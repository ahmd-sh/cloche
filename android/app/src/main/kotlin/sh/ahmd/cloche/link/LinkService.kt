package sh.ahmd.cloche.link

import android.app.NotificationManager
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.Network
import android.util.Log
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.cancel
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.mapNotNull
import kotlinx.coroutines.flow.merge
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.transformLatest
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import sh.ahmd.cloche.Cloche
import sh.ahmd.cloche.cloche
import sh.ahmd.cloche.notify.Notices
import sh.ahmd.cloche.store.Paired
import java.io.EOFException
import java.net.ConnectException
import java.net.ProtocolException
import java.net.SocketException
import java.net.NoRouteToHostException
import java.net.SocketTimeoutException
import java.net.UnknownHostException

class LinkService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private lateinit var app: Cloche
    private var foreground = false
    private var polling: Job? = null
    private var working: Job? = null

    override fun onBind(intent: Intent?) = null

    @OptIn(ExperimentalCoroutinesApi::class)
    override fun onCreate() {
        super.onCreate()
        app = cloche
        instance = this
        val nm = getSystemService(NotificationManager::class.java)
        scope.launch {
            app.link
                .mapNotNull { it.up }
                .distinctUntilChanged()
                .transformLatest { up ->
                    if (!up) delay(GRACE)
                    emit(up)
                }
                .distinctUntilChanged()
                .collect { if (foreground) nm.notify(Notices.LINK_ID, app.notices.link(app.link.value, app.store.value.paired)) }
        }
        scope.launch { app.tailnet.mobile.collect { if (!it) app.work.trySend(Unit) } }
        scope.launch {
            while (true) {
                app.pipeline.tidy()
                delay(HOUR)
            }
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (!foreground) {
            try {
                startForeground(Notices.LINK_ID, app.notices.link(app.link.value, app.store.value.paired), ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
                foreground = true
            } catch (e: Exception) {
                Log.w(TAG, "can't keep the link from here", e)
                stopSelf()
                return START_NOT_STICKY
            }
        }
        Watchdog.schedule(this)
        ensure()
        return START_STICKY
    }

    private fun ensure() {
        if (polling?.isActive != true) polling = scope.launch { poll() }
        if (working?.isActive != true) working = scope.launch { work() }
        app.kick()
    }

    override fun onDestroy() {
        instance = null
        scope.cancel()
        super.onDestroy()
    }

    private suspend fun poll() {
        var failures = 0
        var downSince = 0L
        var liveSince = 0L
        var answered = false
        fun live() {
            if (liveSince == 0L) liveSince = System.currentTimeMillis()
            app.link.value = Link.Live(liveSince)
        }
        while (true) {
            val paired = app.store.value.paired
            if (paired == null) {
                app.link.value = Link.Unpaired
                stopSelf()
                return
            }
            val net = if (paired.loopback) null else app.tailnet.vpn.value
            if (!paired.loopback && net == null) {
                app.link.value = Link.NoTailnet
                liveSince = 0
                answered = false
                app.tailnet.vpn.first { it != null }
                continue
            }
            if (app.link.value !is Link.Live && app.link.value !is Link.Down) app.link.value = Link.Connecting
            val api = Api(net, paired.addr, app.secrets.open(paired.secret))
            try {
                val inbox = whileSame(net, paired) {
                    api.inbox(app.store.value.cursor, if (answered) WAIT else 0) { if (answered) live() }
                } ?: continue
                answered = true
                live()
                failures = 0
                downSince = 0
                app.pipeline.received(paired, inbox)
                if (app.pipeline.pending()) app.work.trySend(Unit)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Api.Unpaired) {
                if (app.store.value.paired != paired) continue
                app.store.update { it.copy(paired = null, unpaired = e.message) }
                app.notices.unpaired(paired.host)
                app.link.value = Link.Unpaired
                Watchdog.cancel(this)
                stopSelf()
                return
            } catch (e: Exception) {
                liveSince = 0
                answered = false
                failures++
                if (downSince == 0L) downSince = System.currentTimeMillis()
                val wait = BACKOFF[minOf(failures - 1, BACKOFF.lastIndex)] * 1000L
                app.link.value = Link.Down(reason(e), downSince, System.currentTimeMillis() + wait)
                withTimeoutOrNull(wait) { merge(app.kicks.receiveAsFlow(), app.tailnet.vpn.drop(1)).first() }
            }
        }
    }

    private suspend fun work() {
        var failures = 0
        for (signal in app.work) {
            val paired = app.store.value.paired ?: continue
            val net = if (paired.loopback) null else app.tailnet.vpn.value ?: continue
            try {
                app.pipeline.work(Api(net, paired.addr, app.secrets.open(paired.secret)))
                failures = 0
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                failures++
                Log.i(TAG, "work: ${e.message}")
                if (e !is Api.Unpaired) {
                    val wait = BACKOFF[minOf(failures - 1, BACKOFF.lastIndex)] * 1000L
                    scope.launch {
                        delay(wait)
                        app.work.trySend(Unit)
                    }
                }
            }
        }
    }

    private suspend fun <T> whileSame(net: Network?, paired: Paired, block: suspend () -> T): T? = coroutineScope {
        val job = async { block() }
        val watch = launch {
            merge(
                app.tailnet.vpn.mapNotNull { if (net != null && it != net) Unit else null },
                app.store.state.mapNotNull { if (it.paired != paired) Unit else null },
            ).first()
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

    companion object {
        private const val TAG = "cloche"
        const val WAIT = 270
        private val BACKOFF = longArrayOf(1, 2, 5, 10, 20, 30, 60)

        private const val GRACE = 20_000L
        private const val HOUR = 3_600_000L

        @Volatile private var instance: LinkService? = null

        val running: Boolean get() = instance != null

        fun start(context: Context) {
            instance?.ensure() ?: context.startForegroundService(Intent(context, LinkService::class.java))
        }

        fun reason(e: Exception): String = when (e) {
            is ConnectException -> "Nothing's listening. Is cloche running?"
            is NoRouteToHostException -> "No route to the computer"
            is SocketTimeoutException -> "Not answering. Asleep or offline?"
            is UnknownHostException -> "Can't find it by name"
            is Api.Refused -> e.message.orEmpty()
            is SocketException, is EOFException, is ProtocolException -> "The connection dropped"
            else -> if (e.message?.startsWith("unexpected end of stream") == true) "The connection dropped" else e.message ?: e.javaClass.simpleName
        }
    }
}

class BootReceiver : android.content.BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED && intent.action != Intent.ACTION_MY_PACKAGE_REPLACED) return
        if (context.cloche.store.value.paired != null) {
            runCatching { LinkService.start(context) }.onFailure { Log.w("cloche", "couldn't start the link", it) }
        }
    }
}

private val Link.up: Boolean?
    get() = when (this) {
        is Link.Live -> true
        is Link.Down, Link.NoTailnet -> false
        Link.Connecting, Link.Unpaired -> null
    }
