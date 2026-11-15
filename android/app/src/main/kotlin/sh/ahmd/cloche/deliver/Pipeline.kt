package sh.ahmd.cloche.deliver

import android.os.Build
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.isActive
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import sh.ahmd.cloche.Cloche
import sh.ahmd.cloche.link.Api
import sh.ahmd.cloche.link.Inbox
import sh.ahmd.cloche.link.Result
import sh.ahmd.cloche.link.Signed
import sh.ahmd.cloche.link.sha256
import sh.ahmd.cloche.store.Delivery
import sh.ahmd.cloche.store.Computer
import sh.ahmd.cloche.store.Saved
import sh.ahmd.cloche.store.cleanName
import sh.ahmd.cloche.store.Stage
import java.io.IOException

data class Kept(val apks: Set<String> = emptySet(), val bytes: Long = 0)

class Pipeline(private val app: Cloche) {
    private val store get() = app.store
    private val _progress = MutableStateFlow<Map<String, Float>>(emptyMap())

    val progress: StateFlow<Map<String, Float>> = _progress.asStateFlow()

    private val _kept = MutableStateFlow(Kept())
    private val looks = Channel<Unit>(Channel.CONFLATED)

    val kept: StateFlow<Kept> = _kept.asStateFlow()

    init {
        app.scope.launch(Dispatchers.IO) {
            for (look in looks) {
                _kept.value = kept()
                app.contentResolver.notifyChange(Shelf.children(), null)
            }
        }
    }

    fun refresh() {
        looks.trySend(Unit)
    }

    fun received(c: Computer, inbox: Inbox): Boolean {
        val now = System.currentTimeMillis()
        var rejected = 0
        val fresh = mutableListOf<Delivery>()
        for (sd in inbox.deliveries) {
            val m = Signed.open(c.keyBytes, sd)
            if (m == null || !DELIVERY_ID.matches(m.id)) {
                rejected++
                continue
            }
            if (store.value.delivery(m.id) != null || fresh.any { it.id == m.id }) continue
            fresh += Delivery(id = m.id, from = c.id, seq = m.seq, m = m, stage = Stage.Fetching, received = now, changed = now)
        }
        val before = store.value.deliveries
        val after = store.update { s ->
            if (s.computer(c.id) == null) return@update s
            val next = s.change(c.id) { it.copy(cursor = inbox.head, offered = cleanName(inbox.name), seen = if (now - it.seen > SEEN) now else it.seen) }
            next.copy(deliveries = trim(next, fresh.sortedByDescending { it.seq } + s.deliveries))
        }
        val kept = after.deliveries.mapTo(HashSet()) { it.id }
        before.filter { it.id !in kept }.forEach { drop(it.id) }
        if (rejected > 0) app.notices.rejected(store.value.label(c), rejected)
        return fresh.isNotEmpty()
    }

    private fun trim(s: Saved, ds: List<Delivery>): List<Delivery> {
        var over = ds.size - KEEP
        if (over <= 0) return ds
        val gone = HashSet<String>()
        for (d in ds.asReversed()) {
            if (over <= 0) break
            val busy = d.stage == Stage.Fetching || d.stage == Stage.Ready || d.stage == Stage.Installing
            if (busy || s.owed(d)) continue
            gone += d.id
            over--
        }
        return ds.filter { it.id !in gone }
    }

    fun pending(id: String): Boolean = store.value.let { s -> s.deliveries.any { it.from == id && (it.stage == Stage.Fetching || s.owed(it)) } }

    suspend fun work(c: Computer, api: Api) {
        for (d in store.value.deliveries.filter { it.from == c.id && it.stage == Stage.Fetching }.sortedBy { it.seq }) fetch(api, d)
        report(c, api)
    }

    fun forgotten(id: String) {
        for (d in store.value.deliveries) if (d.from == id && d.stage == Stage.Failed) app.notices.cancel(d)
        store.update { s -> s.copy(deliveries = s.deliveries.filterNot { it.cleared && !s.owed(it) }) }
    }

    private suspend fun fetch(api: Api, d: Delivery) {
        val apk = app.apkFile(d.id)
        val part = app.partFile(d.id)
        apk.parentFile?.mkdirs()
        if (!apk.exists()) {
            if (holds(d.id)) return hold(d)
            try {
                val whole = until(held(d.id)) { api.download(d.id, part, d.m.file.size) { p -> _progress.update { it + (d.id to p) } } }
                if (whole == null) return hold(d)
            } catch (e: Api.Gone) {
                part.delete()
                settle(d.id, Stage.Failed, "it was gone from the computer before it arrived")
                return
            } catch (e: IOException) {
                val tries = d.tries + 1
                if (tries >= MAX_TRIES) {
                    part.delete()
                    settle(d.id, Stage.Failed, "couldn't download it: ${e.message}")
                    return
                }
                store.update { s -> s.copy(deliveries = s.deliveries.map { if (it.id == d.id) it.copy(tries = tries) else it }) }
                throw e
            } finally {
                _progress.update { it - d.id }
            }
            val sum = withContext(Dispatchers.IO) { sha256(part) }
            if (sum != d.m.file.sha256) {
                part.delete()
                settle(d.id, Stage.Blocked, "its bytes don't match its signed manifest: it was changed on the way")
                return
            }
            if (!part.renameTo(apk)) throw IOException("couldn't keep the file")
            refresh()
        }

        val read = withContext(Dispatchers.IO) { Inspect.read(app, apk, app.iconFile(d.id)) }
        if (read == null) {
            apk.delete()
            refresh()
            settle(d.id, Stage.Blocked, "Android can't read it as an APK")
            return
        }
        val v = Policy.check(read, store.value.pin(read.pkg), Build.VERSION.SDK_INT)
        store.change(d.id) { it.copy(apk = read, fresh = v.fresh) }
        if (!v.ok) {
            apk.delete()
            refresh()
            settle(d.id, Stage.Blocked, v.reason)
            return
        }
        val others = store.value.deliveries.filter { it.id != d.id && it.pkg == read.pkg && it.stage == Stage.Ready }
        if (others.any { it.newerThan(d) }) {
            supersede(d.id)
            return
        }
        others.forEach { supersede(it.id) }
        settle(d.id, Stage.Ready, "")
    }

    private fun holds(id: String) = store.value.let { s -> s.delivery(id)?.let { s.holds(it, app.tailnet.mobile.value) } == true }

    private fun held(id: String) = combine(app.tailnet.mobile, store.state) { mobile, s -> s.delivery(id)?.let { s.holds(it, mobile) } == true }

    private fun hold(d: Delivery) {
        if (store.value.delivery(d.id)?.held == true) return
        store.update { s -> s.copy(deliveries = s.deliveries.map { if (it.id == d.id) it.copy(held = true) else it }) }
        store.value.delivery(d.id)?.let { app.notices.waiting(it) }
    }

    fun downloadNow(id: String) {
        store.update { s -> s.copy(deliveries = s.deliveries.map { if (it.id == id) it.copy(overMobile = true) else it }) }
        store.value.delivery(id)?.let { app.notices.cancel(it) }
        app.work.trySend(Unit)
    }

    fun settle(id: String, stage: Stage, detail: String) {
        store.change(id) { it.copy(stage = stage, detail = detail, asking = false) }
        store.value.delivery(id)?.let { app.notices.show(it) }
    }

    private fun supersede(id: String) {
        app.apkFile(id).delete()
        refresh()
        store.change(id) { it.copy(stage = Stage.Superseded, detail = "a newer build arrived") }
    }

    suspend fun report(c: Computer, api: Api) {
        for (d in store.value.deliveries) {
            if (d.from != c.id) continue
            val wire = d.stage.wire ?: continue
            if (d.reported == d.stage) continue
            val apk = d.apk
            try {
                api.result(
                    d.id,
                    Result(
                        status = wire, detail = d.detail, pkg = apk?.pkg.orEmpty(), versionCode = apk?.versionCode ?: 0,
                        versionName = apk?.versionName.orEmpty(), signer = apk?.signer.orEmpty(),
                    ),
                )
            } catch (e: Api.Gone) {
            }
            store.update { s -> s.copy(deliveries = s.deliveries.map { if (it.id == d.id) it.copy(reported = d.stage) else it }) }
        }
        store.update { s -> s.copy(deliveries = s.deliveries.filterNot { it.cleared && !s.owed(it) }) }
    }

    fun clear(ids: Collection<String>) {
        if (ids.isEmpty()) return
        val now = System.currentTimeMillis()
        val gone = mutableListOf<Delivery>()
        store.update { s ->
            gone.clear()
            s.copy(
                deliveries = s.deliveries.mapNotNull { d ->
                    if (d.id !in ids || !d.clearable) return@mapNotNull d
                    gone += d
                    val e = if (d.stage == Stage.Ready) d.copy(stage = Stage.Declined, detail = "cleared on the phone", changed = now) else d
                    if (s.owed(e)) e.copy(cleared = true) else null
                },
            )
        }
        for (d in gone) {
            app.notices.cancel(d)
            drop(d.id)
        }
        app.work.trySend(Unit)
    }

    suspend fun tidy(now: Long = System.currentTimeMillis()) {
        val days = store.value.clearAfter
        if (days > 0) clear(store.value.deliveries.filter { it.clearable && now - it.changed > days * DAY }.map { it.id })
        withContext(Dispatchers.IO) { sweep() }
    }

    private fun sweep() {
        val files = dirs().flatMap { it.listFiles()?.toList().orEmpty() }
        val owned = store.value.deliveries.filter { !it.cleared }.mapTo(HashSet()) { it.id }
        for (f in files) if (f.nameWithoutExtension !in owned) f.delete()
        refresh()
    }

    fun kept(): Kept {
        val files = dirs().flatMap { it.listFiles()?.toList().orEmpty() }
        return Kept(files.filter { it.extension == "apk" }.mapTo(HashSet()) { it.nameWithoutExtension }, files.sumOf { it.length() })
    }

    fun dropInstalled() {
        for (d in store.value.deliveries) if (d.stage == Stage.Installed) app.apkFile(d.id).delete()
        refresh()
    }

    fun deleteFile(id: String) {
        val d = store.value.delivery(id) ?: return
        if (d.stage == Stage.Fetching || d.stage == Stage.Installing) return
        app.apkFile(id).delete()
        refresh()
        if (d.stage == Stage.Ready) {
            app.notices.cancel(d)
            store.change(id) { it.copy(stage = Stage.Declined, detail = "its file was deleted on the phone") }
            app.work.trySend(Unit)
        }
    }

    private fun dirs() = listOf(app.apkFile("x").parentFile!!, app.iconFile("x").parentFile!!)

    private fun drop(id: String) {
        app.apkFile(id).delete()
        app.partFile(id).delete()
        app.iconFile(id).delete()
        refresh()
    }

    private suspend fun <T> until(stop: Flow<Boolean>, block: suspend () -> T): T? = coroutineScope {
        val job = async { block() }
        val watch = launch {
            stop.first { it }
            job.cancel()
        }
        try {
            job.await()
        } catch (e: CancellationException) {
            if (!isActive) throw e
            null
        } finally {
            watch.cancel()
        }
    }

    private companion object {
        const val KEEP = 100
        const val MAX_TRIES = 8
        const val DAY = 86_400_000L
        const val SEEN = 600_000L
        val DELIVERY_ID = Regex("[a-z2-7]{8}")
    }
}
