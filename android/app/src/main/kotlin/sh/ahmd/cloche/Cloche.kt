package sh.ahmd.cloche

import android.app.Application
import android.content.Context
import kotlinx.coroutines.MainScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.update
import sh.ahmd.cloche.deliver.Installer
import sh.ahmd.cloche.deliver.Pipeline
import kotlinx.coroutines.launch
import sh.ahmd.cloche.link.Link
import sh.ahmd.cloche.link.Tailnet
import sh.ahmd.cloche.link.Watchdog
import sh.ahmd.cloche.notify.Notices
import sh.ahmd.cloche.store.Secrets
import sh.ahmd.cloche.store.Store
import java.io.File

class Cloche : Application() {
    lateinit var store: Store
        private set
    lateinit var secrets: Secrets
        private set
    lateinit var notices: Notices
        private set
    lateinit var tailnet: Tailnet
        private set
    lateinit var pipeline: Pipeline
        private set

    val scope = MainScope()

    val links = MutableStateFlow<Map<String, Link>>(emptyMap())

    val kicks = MutableSharedFlow<Unit>(extraBufferCapacity = 1, onBufferOverflow = BufferOverflow.DROP_OLDEST)

    val work = Channel<Unit>(Channel.CONFLATED)

    val started: Boolean get() = ::pipeline.isInitialized

    @Volatile var visible = false

    override fun onCreate() {
        super.onCreate()
        store = Store(File(filesDir, "state.json"))
        secrets = Secrets()
        notices = Notices(this)
        tailnet = Tailnet(this).also { it.start() }
        pipeline = Pipeline(this)
        Installer.watch(this)
        scope.launch {
            delay(3_000)
            Installer.reconcile(this@Cloche)
        }
        pipeline.refresh()
        scope.launch { pipeline.tidy() }
        if (store.value.computers.isNotEmpty()) Watchdog.schedule(this)
    }

    fun kick() {
        kicks.tryEmit(Unit)
        work.trySend(Unit)
    }

    fun forget(id: String, why: String? = null) {
        val c = store.value.computer(id) ?: return
        store.update { s ->
            val rest = s.computers.filter { it.id != id }
            s.orphan(id).copy(computers = rest, unpaired = if (rest.isEmpty()) why ?: s.unpaired else null)
        }
        links.update { it - id }
        pipeline.forgotten(id)
        if (why != null) notices.unpaired(c.name)
        if (store.value.computers.isEmpty()) Watchdog.cancel(this)
    }

    fun apkFile(id: String) = File(filesDir, "apk/$id.apk")
    fun partFile(id: String) = File(filesDir, "apk/$id.part")
    fun iconFile(id: String) = File(filesDir, "icons/$id.png")
}

val Context.cloche: Cloche get() = applicationContext as Cloche
