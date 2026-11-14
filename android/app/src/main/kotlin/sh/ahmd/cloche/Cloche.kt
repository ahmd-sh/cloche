package sh.ahmd.cloche

import android.app.Application
import android.content.Context
import kotlinx.coroutines.MainScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
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

    val link = MutableStateFlow<Link>(Link.Connecting)

    val kicks = Channel<Unit>(Channel.CONFLATED)

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
        if (store.value.paired != null) Watchdog.schedule(this)
    }

    fun kick() {
        kicks.trySend(Unit)
        work.trySend(Unit)
    }

    fun apkFile(id: String) = File(filesDir, "apk/$id.apk")
    fun partFile(id: String) = File(filesDir, "apk/$id.part")
    fun iconFile(id: String) = File(filesDir, "icons/$id.png")
}

val Context.cloche: Cloche get() = applicationContext as Cloche
