package sh.ahmd.cloche.store

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.serialization.Serializable
import sh.ahmd.cloche.link.Manifest
import sh.ahmd.cloche.BuildConfig
import sh.ahmd.cloche.link.Signed
import sh.ahmd.cloche.link.Tailnet
import sh.ahmd.cloche.link.json
import java.io.File
import java.io.FileNotFoundException
import java.io.FileOutputStream
import java.io.IOException
import java.util.Base64
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicReference

@Serializable
data class Paired(
    val host: String,
    val addr: String,
    val key: String,
    val secret: String,
    val device: String,
    val pairedAt: Long,
) {
    val keyBytes: ByteArray get() = Base64.getDecoder().decode(key)
    val fingerprint: String get() = Signed.fingerprint(keyBytes)

    val loopback: Boolean
        get() = BuildConfig.DEBUG && Tailnet.isLoopback(addr)
}

@Serializable
enum class Stage(val wire: String?) {
    Fetching(null),
    Ready("ready"),
    Blocked("blocked"),
    Installing("installing"),
    Installed("installed"),
    Failed("failed"),
    Declined("declined"),
    Superseded("superseded");

    val installable: Boolean get() = this == Ready || this == Failed || this == Declined
}

@Serializable
data class Apk(
    val pkg: String,
    val label: String,
    val versionCode: Long,
    val versionName: String,
    val signer: String,
    val minSdk: Int,
    val targetSdk: Int,
    val installedCode: Long? = null,
    val installedName: String? = null,
    val installedSameSigner: Boolean? = null,
) {
    val shortSigner: String get() = signer.take(16).uppercase().chunked(4).joinToString(" ")
    val version: String get() = if (versionName.isEmpty()) "$versionCode" else "$versionName ($versionCode)"
}

@Serializable
data class Delivery(
    val id: String,
    val seq: Long,
    val m: Manifest,
    val stage: Stage,
    val detail: String = "",
    val apk: Apk? = null,
    val fresh: Boolean = false,
    val received: Long,
    val changed: Long,
    val reported: Stage? = null,
    val tries: Int = 0,
    val cleared: Boolean = false,
    val overMobile: Boolean = false,
    val held: Boolean = false,
    val session: Int = 0,
    val asking: Boolean = false,
) {
    val clearable: Boolean get() = !cleared && stage != Stage.Fetching && stage != Stage.Installing

    val label: String get() = apk?.label ?: m.apk?.label?.ifEmpty { null } ?: m.apk?.pkg ?: m.file.name
    val pkg: String? get() = apk?.pkg ?: m.apk?.pkg
}

@Serializable
data class Pin(val pkg: String, val signer: String, val since: Long)

@Serializable
data class Saved(
    val paired: Paired? = null,
    val cursor: Long = 0,
    val deliveries: List<Delivery> = emptyList(),
    val pins: List<Pin> = emptyList(),
    val unpaired: String? = null,
    val clearAfter: Int = 7,
    val folded: Boolean = false,
    val keepInstalled: Boolean = false,
    val mobileData: Boolean = true,
) {
    fun delivery(id: String) = deliveries.firstOrNull { it.id == id }

    fun holds(d: Delivery, mobile: Boolean): Boolean = mobile && !mobileData && !d.overMobile && d.stage == Stage.Fetching

    val shown: List<Delivery> get() = deliveries.filter { !it.cleared }
    fun pin(pkg: String) = pins.firstOrNull { it.pkg == pkg }
}

class Store(private val file: File) {
    private val _state = MutableStateFlow(load())
    val state: StateFlow<Saved> = _state.asStateFlow()
    val value: Saved get() = _state.value

    private val unwritten = AtomicReference<Saved?>(null)
    private val writer = Executors.newSingleThreadExecutor { Thread(it, "cloche-store").apply { isDaemon = true } }

    @Synchronized
    fun update(f: (Saved) -> Saved): Saved {
        val next = f(_state.value)
        if (next != _state.value) {
            _state.value = next
            if (unwritten.getAndSet(next) == null) writer.execute { unwritten.getAndSet(null)?.let(::save) }
        }
        return next
    }

    fun flush() {
        writer.submit {}.get()
    }

    fun change(id: String, f: (Delivery) -> Delivery) = update { s ->
        s.copy(deliveries = s.deliveries.map { d -> if (d.id == id) f(d).let { if (it != d) it.copy(changed = System.currentTimeMillis()) else it } else d })
    }

    private fun load(): Saved = try {
        json.decodeFromString<Saved>(file.readText())
    } catch (e: FileNotFoundException) {
        Saved()
    } catch (e: Exception) {
        file.renameTo(File(file.path + ".unreadable"))
        Saved()
    }

    private fun save(s: Saved) {
        val next = File(file.path + ".new")
        try {
            FileOutputStream(next).use { out ->
                out.write(json.encodeToString(Saved.serializer(), s).encodeToByteArray())
                out.fd.sync()
            }
            if (!next.renameTo(file)) throw IOException("couldn't replace $file")
        } catch (e: IOException) {
            next.delete()
            System.err.println("cloche: can't save: $e")
        }
    }
}
