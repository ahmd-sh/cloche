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
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.mapNotNull
import kotlinx.coroutines.flow.merge
import kotlinx.coroutines.flow.transformLatest
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import sh.ahmd.cloche.Cloche
import sh.ahmd.cloche.cloche
import sh.ahmd.cloche.notify.Notices
import sh.ahmd.cloche.store.Computer
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
    private var watching: Job? = null
    private var dispatching: Job? = null
    private val stations = HashMap<String, Station>()

    private class Station(val poll: Job, val work: Job, val wake: Channel<Unit>) {
        fun stop() {
            poll.cancel()
            work.cancel()
            wake.close()
        }
    }

    override fun onBind(intent: Intent?) = null

    @OptIn(ExperimentalCoroutinesApi::class)
    override fun onCreate() {
        super.onCreate()
        app = cloche
        instance = this
        val nm = getSystemService(NotificationManager::class.java)
        scope.launch {
            combine(app.links, app.store.state) { links, s -> Links.up(s.computers, links) to Links.title(s.computers, links) }
                .distinctUntilChanged()
                .transformLatest { (up, title) ->
                    if (up == false) delay(GRACE)
                    emit(title)
                }
                .distinctUntilChanged()
                .collect { if (foreground) nm.notify(Notices.LINK_ID, app.notices.link()) }
        }
        scope.launch { app.tailnet.mobile.collect { if (!it) app.work.trySend(Unit) } }
        scope.launch {
            while (true) {
                app.pipeline.tidy()
                delay(Links.HOUR)
            }
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (!foreground) {
            try {
                startForeground(Notices.LINK_ID, app.notices.link(), ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
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
        if (watching?.isActive != true) watching = scope.launch { watch() }
        if (dispatching?.isActive != true) dispatching = scope.launch { for (signal in app.work) stations.values.forEach { it.wake.trySend(Unit) } }
        app.kick()
    }

    override fun onDestroy() {
        instance = null
        stations.values.forEach { it.stop() }
        stations.clear()
        scope.cancel()
        super.onDestroy()
    }

    private suspend fun watch() {
        app.store.state.map { s -> s.computers.map { it.id } }.distinctUntilChanged().collect { ids ->
            for (id in stations.keys - ids.toSet()) stations.remove(id)?.stop()
            app.links.update { links -> links.filterKeys { it in ids } }
            if (ids.isEmpty()) {
                stopSelf()
                return@collect
            }
            for (id in ids) {
                if (id in stations) continue
                val wake = Channel<Unit>(Channel.CONFLATED)
                stations[id] = Station(scope.launch { poll(id) }, scope.launch { work(id, wake) }, wake)
                wake.trySend(Unit)
            }
        }
    }

    private fun set(id: String, link: Link) {
        app.links.update { if (app.store.value.computer(id) != null) it + (id to link) else it - id }
    }

    private suspend fun poll(id: String) {
        var failures = 0
        var downSince = 0L
        var liveSince = 0L
        var answered = false
        fun live() {
            if (liveSince == 0L) liveSince = System.currentTimeMillis()
            set(id, Link.Live(liveSince))
        }
        while (true) {
            val c = app.store.value.computer(id) ?: return
            val net = if (c.loopback) null else app.tailnet.vpn.value
            if (!c.loopback && net == null) {
                set(id, Link.NoTailnet)
                liveSince = 0
                answered = false
                app.tailnet.vpn.first { it != null }
                continue
            }
            val now = app.links.value[id]
            if (now !is Link.Live && now !is Link.Down) set(id, Link.Connecting)
            val api = Api(net, c.addr, app.secrets.open(c.secret))
            try {
                val inbox = whileSame(net, c) {
                    api.inbox(c.cursor, if (answered) WAIT else 0) { if (answered) live() }
                } ?: continue
                answered = true
                live()
                failures = 0
                downSince = 0
                app.pipeline.received(c, inbox)
                if (app.pipeline.pending(id)) stations[id]?.wake?.trySend(Unit)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Api.Unpaired) {
                if (!c.sameLink(app.store.value.computer(id))) continue
                app.forget(id, e.message)
                return
            } catch (e: Exception) {
                liveSince = 0
                answered = false
                failures++
                val at = System.currentTimeMillis()
                if (downSince == 0L) downSince = at
                val wait = Links.backoff(failures, at - downSince)
                set(id, Link.Down(reason(e), downSince, at + wait))
                withTimeoutOrNull(wait) { merge(app.kicks, app.tailnet.vpn.drop(1)).first() }
            }
        }
    }

    private suspend fun work(id: String, wake: Channel<Unit>) {
        var failures = 0
        for (signal in wake) {
            val c = app.store.value.computer(id) ?: return
            if (!app.pipeline.pending(id)) continue
            val net = if (c.loopback) null else app.tailnet.vpn.value ?: continue
            try {
                app.pipeline.work(c, Api(net, c.addr, app.secrets.open(c.secret)))
                failures = 0
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                failures++
                Log.i(TAG, "work for ${c.name}: ${e.message}")
                if (e !is Api.Unpaired) {
                    scope.launch {
                        delay(Links.backoff(failures, 0))
                        wake.trySend(Unit)
                    }
                }
            }
        }
    }

    private suspend fun <T> whileSame(net: Network?, c: Computer, block: suspend () -> T): T? = coroutineScope {
        val job = async { block() }
        val watch = launch {
            merge(
                app.tailnet.vpn.mapNotNull { if (net != null && it != net) Unit else null },
                app.store.state.mapNotNull { if (!c.sameLink(it.computer(c.id))) Unit else null },
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

        private const val GRACE = 20_000L

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
        if (context.cloche.store.value.computers.isNotEmpty()) {
            runCatching { LinkService.start(context) }.onFailure { Log.w("cloche", "couldn't start the link", it) }
        }
    }
}
