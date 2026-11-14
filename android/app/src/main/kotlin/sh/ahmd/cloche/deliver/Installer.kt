package sh.ahmd.cloche.deliver

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.content.pm.PackageManager
import android.os.Build
import android.os.Handler
import android.os.Looper
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import sh.ahmd.cloche.Cloche
import sh.ahmd.cloche.cloche
import sh.ahmd.cloche.store.Delivery
import sh.ahmd.cloche.store.Pin
import sh.ahmd.cloche.store.Stage
import java.util.concurrent.ConcurrentHashMap

object Installer {
    const val EXTRA_ID = "sh.ahmd.cloche.delivery"

    private const val COMMITTED = 0.8f

    private val prompts = ConcurrentHashMap<String, Intent>()

    private val starting: MutableSet<String> = ConcurrentHashMap.newKeySet()

    private val abandoned: MutableSet<Int> = ConcurrentHashMap.newKeySet()

    fun prompt(app: Cloche, id: String): Intent? =
        prompts[id]?.takeIf { app.store.value.delivery(id)?.let { it.stage == Stage.Installing && it.asking } == true }

    suspend fun install(app: Cloche, id: String) = withContext(Dispatchers.IO) {
        val d = app.store.value.delivery(id) ?: return@withContext
        if (!d.stage.installable && !(d.stage == Stage.Installing && d.asking)) return@withContext
        if (!starting.add(id)) return@withContext
        try {
            start(app, d)
        } finally {
            starting.remove(id)
        }
        app.work.trySend(Unit)
    }

    private fun start(app: Cloche, d: Delivery) {
        val id = d.id
        val file = app.apkFile(id)
        if (!file.exists()) {
            app.pipeline.settle(id, Stage.Failed, "its file is gone from the phone")
            return
        }
        val apk = Inspect.read(app, file, null)
        if (apk == null) {
            app.pipeline.settle(id, Stage.Blocked, "Android can't read it as an APK")
            return
        }
        val v = Policy.check(apk, app.store.value.pin(apk.pkg), Build.VERSION.SDK_INT)
        app.store.change(id) { it.copy(apk = apk) }
        if (!v.ok) {
            app.pipeline.settle(id, Stage.Blocked, v.reason)
            return
        }
        app.notices.cancel(d)
        try {
            val pi = app.packageManager.packageInstaller
            for (s in pi.mySessions) {
                if (s.appPackageName != apk.pkg) continue
                if (runCatching { pi.abandonSession(s.sessionId) }.isFailure) continue
                abandoned += s.sessionId
                val other = app.store.value.deliveries.firstOrNull { it.id != id && it.session == s.sessionId && it.stage == Stage.Installing }
                if (other != null) {
                    forget(other.id)
                    app.pipeline.settle(other.id, Stage.Superseded, "a newer build was installed")
                }
            }
            prompts.remove(id)
            val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL).apply {
                setAppPackageName(apk.pkg)
                setSize(file.length())
                setInstallReason(PackageManager.INSTALL_REASON_USER)
                setRequireUserAction(
                    if (apk.pkg == app.packageName) PackageInstaller.SessionParams.USER_ACTION_REQUIRED
                    else PackageInstaller.SessionParams.USER_ACTION_NOT_REQUIRED,
                )
            }
            val sid = pi.createSession(params)
            app.store.change(id) { it.copy(stage = Stage.Installing, detail = "", session = sid, asking = false) }
            pi.openSession(sid).use { s ->
                file.inputStream().use { input ->
                    s.openWrite("base.apk", 0, file.length()).use { out ->
                        input.copyTo(out, 256 * 1024)
                        s.fsync(out)
                    }
                }
                val done = Intent(app, InstallReceiver::class.java).putExtra(EXTRA_ID, id)
                val pending = PendingIntent.getBroadcast(app, sid, done, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE)
                s.commit(pending.intentSender)
            }
        } catch (e: Exception) {
            app.pipeline.settle(id, Stage.Failed, e.message ?: "couldn't start installing")
        }
    }

    fun watch(app: Cloche) {
        app.packageManager.packageInstaller.registerSessionCallback(object : PackageInstaller.SessionCallback() {
            override fun onCreated(sessionId: Int) {}
            override fun onBadgingChanged(sessionId: Int) {}
            override fun onActiveChanged(sessionId: Int, active: Boolean) {}
            override fun onFinished(sessionId: Int, success: Boolean) {}

            override fun onProgressChanged(sessionId: Int, progress: Float) {
                if (progress <= COMMITTED + 0.01f) return
                val d = app.store.value.deliveries.firstOrNull { it.session == sessionId && it.stage == Stage.Installing && it.asking } ?: return
                forget(d.id)
                app.store.change(d.id) { it.copy(asking = false) }
                app.notices.cancel(d)
            }
        }, Handler(Looper.getMainLooper()))
    }

    internal fun current(app: Cloche, id: String, sid: Int): Boolean {
        if (sid in abandoned) return false
        val d = app.store.value.delivery(id) ?: return false
        return d.session == 0 || sid <= 0 || d.session == sid
    }

    internal fun asking(app: Cloche, id: String, prompt: Intent) {
        prompts[id] = prompt
        app.store.change(id) { it.copy(asking = true) }
    }

    internal fun forget(id: String) {
        prompts.remove(id)
    }

    fun reconcile(app: Cloche) {
        val pm = app.packageManager
        val pi = pm.packageInstaller
        val before = app.store.value
        for (d in before.deliveries) {
            if (d.cleared || d.id in starting) continue
            if (d.stage != Stage.Installing && !d.stage.installable) continue
            val apk = d.apk ?: continue
            val info = try {
                pm.getPackageInfo(apk.pkg, PackageManager.PackageInfoFlags.of(0))
            } catch (e: PackageManager.NameNotFoundException) {
                null
            }
            val since = if (d.stage == Stage.Installing) d.changed - 5_000 else d.received
            if (info != null && info.longVersionCode == apk.versionCode && info.lastUpdateTime >= since && signedAs(pm, apk.pkg, apk.signer)) {
                installed(app, d.id)
                continue
            }
            if (d.stage != Stage.Installing) continue
            val session = d.session.takeIf { it != 0 }?.let { pi.getSessionInfo(it) }
            if (session != null && session.isCommitted) continue
            if (session != null) runCatching { pi.abandonSession(session.sessionId) }.onSuccess { abandoned += session.sessionId }
            forget(d.id)
            app.pipeline.settle(d.id, Stage.Failed, "the install was interrupted")
        }
        if (app.store.value != before) app.work.trySend(Unit)
    }

    private fun signedAs(pm: PackageManager, pkg: String, signer: String): Boolean = signer.split(',').all { hex ->
        val cert = runCatching { hex.chunked(2).map { it.toInt(16).toByte() }.toByteArray() }.getOrNull() ?: return@all false
        pm.hasSigningCertificate(pkg, cert, PackageManager.CERT_INPUT_SHA256)
    }

    fun installed(app: Cloche, id: String) {
        forget(id)
        if (!app.store.value.keepInstalled && app.apkFile(id).delete()) app.pipeline.refresh()
        val apk = app.store.value.delivery(id)?.apk
        if (apk != null) {
            app.store.update { s ->
                if (s.pin(apk.pkg) != null) s else s.copy(pins = s.pins + Pin(apk.pkg, apk.signer, System.currentTimeMillis()))
            }
        }
        app.store.change(id) { it.copy(fresh = false) }
        app.pipeline.settle(id, Stage.Installed, "")
    }

    internal fun failure(status: Int, message: String?): String {
        val what = when (status) {
            PackageInstaller.STATUS_FAILURE_BLOCKED -> "Android blocked it"
            PackageInstaller.STATUS_FAILURE_CONFLICT -> "it conflicts with the installed app"
            PackageInstaller.STATUS_FAILURE_INCOMPATIBLE -> "it isn't compatible with this phone"
            PackageInstaller.STATUS_FAILURE_INVALID -> "Android found it invalid"
            PackageInstaller.STATUS_FAILURE_STORAGE -> "there isn't room for it"
            PackageInstaller.STATUS_FAILURE_TIMEOUT -> "it timed out"
            else -> "it failed"
        }
        return if (message.isNullOrBlank()) what else "$what: $message"
    }
}

class InstallReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val app = context.cloche
        val id = intent.getStringExtra(Installer.EXTRA_ID) ?: return
        if (!Installer.current(app, id, intent.getIntExtra(PackageInstaller.EXTRA_SESSION_ID, 0))) return
        when (val status = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)) {
            PackageInstaller.STATUS_PENDING_USER_ACTION -> {
                val confirm = intent.getParcelableExtra(Intent.EXTRA_INTENT, Intent::class.java) ?: return
                Installer.asking(app, id, confirm)
                val shown = app.visible && runCatching { context.startActivity(Intent(confirm).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) }.isSuccess
                if (!shown) app.notices.confirm(id)
            }
            PackageInstaller.STATUS_SUCCESS -> Installer.installed(app, id)
            PackageInstaller.STATUS_FAILURE_ABORTED -> {
                Installer.forget(id)
                app.pipeline.settle(id, Stage.Declined, "cancelled at Android's prompt")
            }
            else -> {
                Installer.forget(id)
                app.pipeline.settle(id, Stage.Failed, Installer.failure(status, intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE)))
            }
        }
        app.work.trySend(Unit)
    }
}
