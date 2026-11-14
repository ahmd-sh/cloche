package sh.ahmd.cloche

import android.Manifest
import android.animation.Animator
import android.animation.AnimatorListenerAdapter
import android.animation.AnimatorSet
import android.animation.ObjectAnimator
import android.app.NotificationManager
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.graphics.Color
import android.os.Bundle
import android.os.PowerManager
import android.provider.DocumentsContract
import android.provider.Settings
import android.util.Log
import android.graphics.drawable.GradientDrawable
import android.view.Gravity
import android.view.View
import android.widget.FrameLayout
import android.view.animation.PathInterpolator
import android.window.SplashScreenView
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.setValue
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.launch
import sh.ahmd.cloche.deliver.Installer
import sh.ahmd.cloche.deliver.Shelf
import sh.ahmd.cloche.link.LinkService
import sh.ahmd.cloche.link.PairTicket
import sh.ahmd.cloche.link.Watchdog
import sh.ahmd.cloche.ui.Checks
import sh.ahmd.cloche.ui.ClocheApp
import sh.ahmd.cloche.ui.Haptics
import sh.ahmd.cloche.ui.Host
import sh.ahmd.cloche.ui.Opened

class MainActivity : ComponentActivity(), Host {
    private val app get() = cloche
    private val opened = Channel<Opened>(Channel.BUFFERED)
    private val openedFlow = opened.receiveAsFlow()
    private var resumes by mutableIntStateOf(0)

    private var waiting: String? = null

    private val notifyPermission = registerForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        if (granted) app.notices.relink()
        resumes++
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge(SystemBarStyle.dark(Color.TRANSPARENT), SystemBarStyle.dark(Color.TRANSPARENT))
        setContent {
            ClocheApp(app, openedFlow, this, resumes)
        }
        if (savedInstanceState == null) handle(intent)
        splashScreen.setOnExitAnimationListener(::lift)
    }

    private fun lift(splash: SplashScreenView) {
        try {
            liftOff(splash)
        } catch (e: RuntimeException) {
            Log.w("cloche", "splash", e)
            splash.remove()
        }
    }

    private fun liftOff(splash: SplashScreenView) {
        val wait = (splash.iconAnimationStart?.toEpochMilli()?.plus(splash.iconAnimationDuration?.toMillis() ?: 0) ?: 0) - System.currentTimeMillis()
        val ease = PathInterpolator(0.2f, 0f, 0f, 1f)
        val lip = View(this).apply {
            background = GradientDrawable(GradientDrawable.Orientation.TOP_BOTTOM, intArrayOf(0x00000000, 0x59000000))
            alpha = 0f
        }
        splash.addView(lip, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, (36 * resources.displayMetrics.density).toInt(), Gravity.BOTTOM))
        val icon: View? = splash.iconView
        val parts = buildList<Animator> {
            if (icon != null) {
                add(ObjectAnimator.ofFloat(icon, View.TRANSLATION_Y, 0f, -splash.height * 0.06f).setDuration(260))
                add(ObjectAnimator.ofFloat(icon, View.ALPHA, 1f, 0f).setDuration(220))
            }
            add(ObjectAnimator.ofFloat(lip, View.ALPHA, 0f, 1f).setDuration(120))
            add(
                ObjectAnimator.ofFloat(splash, View.TRANSLATION_Y, 0f, -splash.height.toFloat()).apply {
                    duration = 520
                    startDelay = 90
                    interpolator = ease
                },
            )
        }
        val up = AnimatorSet().apply {
            playTogether(parts)
            startDelay = if (icon != null) wait.coerceIn(0, 900) else 0
            doOnStart { Haptics(window.decorView).lift() }
            doOnEnd { splash.remove() }
        }
        up.start()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handle(intent)
    }

    private fun handle(intent: Intent) {
        intent.data?.toString()?.let(PairTicket::parse)?.let {
            opened.trySend(Opened.Pair(it))
            return
        }
        if (intent.component != notified(this)) return
        intent.getStringExtra(EXTRA_OPEN)?.let {
            opened.trySend(Opened.Delivery(it, intent.getBooleanExtra(EXTRA_INSTALL, false), intent.getBooleanExtra(EXTRA_DOWNLOAD, false)))
        }
    }

    override fun onStart() {
        super.onStart()
        app.visible = true
        if (app.store.value.paired != null) LinkService.start(this) else app.kick()
    }

    override fun onResume() {
        super.onResume()
        resumes++
        Installer.reconcile(app)
        val id = waiting
        if (id != null && packageManager.canRequestPackageInstalls()) {
            waiting = null
            install(id)
        }
    }

    override fun onStop() {
        super.onStop()
        app.visible = false
    }

    override fun install(id: String) {
        if (!packageManager.canRequestPackageInstalls()) {
            waiting = id
            fix("installs")
            return
        }
        Installer.prompt(app, id)?.let { if (runCatching { startActivity(it) }.isSuccess) return }
        app.scope.launch { Installer.install(app, id) }
    }

    override fun showFiles() {
        val root = Shelf.root()
        val tries = listOf(
            Intent(Intent.ACTION_VIEW).setDataAndType(root, DocumentsContract.Root.MIME_TYPE_ITEM),
            Intent(Intent.ACTION_VIEW, root),
        )
        for (intent in tries) {
            intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            if (runCatching { startActivity(intent) }.isSuccess) return
        }
    }

    override fun launch(pkg: String) {
        packageManager.getLaunchIntentForPackage(pkg)?.let { startActivity(it) }
    }

    override fun fix(what: String) {
        val pkg = Uri.parse("package:$packageName")
        val intent = when (what) {
            "notifications" -> {
                if (!asked(ASKED_NOTIFY)) {
                    notifyPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
                    return
                }
                Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, packageName)
            }
            "installs" -> Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, pkg)
            "battery" -> Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS, pkg)
            "tailscale" -> packageManager.getLaunchIntentForPackage(TAILSCALE) ?: return
            else -> return
        }
        runCatching { startActivity(intent) }
    }

    override fun paired() {
        LinkService.start(this)
        if (checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED && !asked(ASKED_NOTIFY)) {
            notifyPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
    }

    override fun unpair() {
        app.store.update { it.copy(paired = null, cursor = 0, unpaired = null) }
        stopService(Intent(this, LinkService::class.java))
        Watchdog.cancel(this)
    }

    override fun checks() = Checks(
        notifications = checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED &&
            getSystemService(NotificationManager::class.java).areNotificationsEnabled(),
        installs = packageManager.canRequestPackageInstalls(),
        battery = getSystemService(PowerManager::class.java).isIgnoringBatteryOptimizations(packageName),
        tailnet = app.tailnet.vpn.value != null,
    )

    private fun asked(key: String): Boolean {
        val prefs = getSharedPreferences("cloche", MODE_PRIVATE)
        if (prefs.getBoolean(key, false)) return true
        prefs.edit().putBoolean(key, true).apply()
        return false
    }

    companion object {
        const val EXTRA_OPEN = "sh.ahmd.cloche.open"
        const val EXTRA_INSTALL = "sh.ahmd.cloche.install"
        const val EXTRA_DOWNLOAD = "sh.ahmd.cloche.download"
        private const val ASKED_NOTIFY = "asked_notify"

        fun notified(context: Context) = ComponentName(context, "sh.ahmd.cloche.Notified")
        private const val TAILSCALE = "com.tailscale.ipn"
    }
}

private fun Animator.doOnStart(f: () -> Unit) = addListener(object : AnimatorListenerAdapter() {
    override fun onAnimationStart(animation: Animator) = f()
})

private fun Animator.doOnEnd(f: () -> Unit) = addListener(object : AnimatorListenerAdapter() {
    override fun onAnimationEnd(animation: Animator) = f()
})
