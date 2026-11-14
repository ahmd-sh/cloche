package sh.ahmd.cloche.notify

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.graphics.BitmapFactory
import android.graphics.drawable.Icon
import android.os.Build
import android.os.Bundle
import android.os.VibrationEffect
import sh.ahmd.cloche.Cloche
import sh.ahmd.cloche.MainActivity
import sh.ahmd.cloche.R
import sh.ahmd.cloche.link.Link
import sh.ahmd.cloche.link.LinkService
import sh.ahmd.cloche.store.Delivery
import sh.ahmd.cloche.store.Paired
import sh.ahmd.cloche.store.Stage

class Notices(private val app: Cloche) {
    private val nm = app.getSystemService(NotificationManager::class.java)

    init {
        nm.createNotificationChannels(
            listOf(
                NotificationChannel(BUILDS, "Builds", NotificationManager.IMPORTANCE_HIGH).apply {
                    description = "Builds ready to install"
                    if (Build.VERSION.SDK_INT >= 36) {
                        setVibrationEffect(VibrationEffect.createWaveform(longArrayOf(0, 14, 90, 22), intArrayOf(0, 110, 0, 255), -1))
                    } else {
                        vibrationPattern = longArrayOf(0, 14, 90, 22)
                    }
                },
                NotificationChannel(INSTALLED, "Installed", NotificationManager.IMPORTANCE_LOW).apply {
                    description = "Builds that finished installing while Cloche was closed"
                },
                NotificationChannel(PROBLEMS, "Problems", NotificationManager.IMPORTANCE_DEFAULT).apply {
                    description = "Refused or failed builds, and pairing problems"
                },
                NotificationChannel(LINK, "Link", NotificationManager.IMPORTANCE_MIN).apply {
                    description = "When Cloche connects to your computer, and when it loses it"
                    setShowBadge(false)
                },
            ),
        )
    }

    fun show(d: Delivery) {
        when (d.stage) {
            Stage.Ready -> ready(d)
            Stage.Blocked -> problem(d, "Refused a ${d.label} build", d.detail)
            Stage.Failed -> problem(d, "Couldn't install ${d.label}", d.detail)
            Stage.Installed -> installed(d)
            Stage.Declined, Stage.Superseded -> cancel(d)
            else -> {}
        }
    }

    private fun ready(d: Delivery) {
        val apk = d.apk ?: return
        val build = "${mb(d.m.file.size)} · from ${d.m.host}"
        val b = base(BUILDS, d)
            .setContentTitle(if (d.fresh) "New app: ${apk.label}" else "${apk.label} ${apk.versionName}".trim())
            .setContentText(d.m.note.ifEmpty { if (d.fresh) "Review it before installing" else build })
            .setStyle(Notification.BigTextStyle().bigText(listOf(d.m.note, build).filter { it.isNotEmpty() }.joinToString("\n")))
            .setSubText(d.m.source?.described ?: d.m.host)
            .setPublicVersion(Notification.Builder(app, BUILDS).setSmallIcon(R.drawable.ic_cloche).setContentTitle("A build is ready").build())
            .setVisibility(Notification.VISIBILITY_PRIVATE)
        if (d.fresh) {
            b.addAction(Notification.Action.Builder(null, "Review", open(d.id, install = false)).build())
        } else {
            b.addAction(Notification.Action.Builder(null, "Install", open(d.id, install = true)).build())
        }
        if (app.visible) b.setGroup(QUIET).setGroupAlertBehavior(Notification.GROUP_ALERT_SUMMARY)
        nm.notify(tag(d), BUILD, b.build())
    }

    fun waiting(d: Delivery) {
        val version = d.m.apk?.versionName.orEmpty()
        val b = base(BUILDS, d)
            .setContentTitle("${d.label} $version".trim())
            .setContentText("Waiting for Wi-Fi · ${mb(d.m.file.size)}")
            .setSubText(d.m.source?.described ?: d.m.host)
            .setGroup(QUIET)
            .setGroupAlertBehavior(Notification.GROUP_ALERT_SUMMARY)
            .addAction(Notification.Action.Builder(null, "Download now", open(d.id, install = false, download = true)).build())
        nm.notify(tag(d), BUILD, b.build())
    }

    private fun installed(d: Delivery) {
        if (app.visible) {
            cancel(d)
            return
        }
        drop(d, PROBLEM)
        val pkg = d.apk?.pkg ?: return
        val b = base(INSTALLED, d)
            .setContentTitle("${d.label} installed")
            .setContentText(d.apk.version)
            .setTimeoutAfter(15 * 60_000)
        app.packageManager.getLaunchIntentForPackage(pkg)?.let { launch ->
            val pi = PendingIntent.getActivity(app, pkg.hashCode(), launch, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
            b.setContentIntent(pi).addAction(Notification.Action.Builder(null, "Open", pi).build())
        }
        nm.notify(tag(d), BUILD, b.build())
    }

    private fun problem(d: Delivery, title: String, detail: String) {
        drop(d, BUILD)
        nm.notify(
            tag(d), PROBLEM,
            base(PROBLEMS, d).setContentTitle(title).setContentText(detail).setStyle(Notification.BigTextStyle().bigText(detail)).build(),
        )
    }

    fun confirm(id: String) {
        val d = app.store.value.delivery(id) ?: return
        val again = open(d.id, install = true)
        nm.notify(
            tag(d), BUILD,
            base(BUILDS, d)
                .setContentTitle("Confirm installing ${d.label}")
                .setContentText("Android needs your OK to install it")
                .setContentIntent(again)
                .addAction(Notification.Action.Builder(null, "Install", again).build())
                .setGroup(QUIET)
                .setGroupAlertBehavior(Notification.GROUP_ALERT_SUMMARY)
                .build(),
        )
    }

    fun rejected(host: String, count: Int) {
        nm.notify(
            REJECTED,
            Notification.Builder(app, PROBLEMS)
                .setSmallIcon(R.drawable.ic_cloche)
                .setContentTitle(if (count == 1) "Dropped an unsigned delivery" else "Dropped $count unsigned deliveries")
                .setContentText("It wasn't signed by $host's key")
                .setContentIntent(home())
                .build(),
        )
    }

    fun paired() {
        nm.cancel(UNPAIRED)
    }

    fun unpaired(host: String) {
        nm.notify(
            UNPAIRED,
            Notification.Builder(app, PROBLEMS)
                .setSmallIcon(R.drawable.ic_cloche)
                .setContentTitle("$host unpaired this phone")
                .setContentText("Open Cloche to pair again")
                .setContentIntent(home())
                .setAutoCancel(true)
                .build(),
        )
    }

    fun cancel(d: Delivery) {
        drop(d, BUILD)
        drop(d, PROBLEM)
    }

    private fun drop(d: Delivery, id: Int) {
        val tag = tag(d)
        val mine = nm.activeNotifications.any { it.tag == tag && it.id == id && it.notification.extras.getString(DELIVERY).let { o -> o == null || o == d.id } }
        if (mine) nm.cancel(tag, id)
    }

    fun relink() {
        if (LinkService.running) nm.notify(LINK_ID, link(app.link.value, app.store.value.paired))
    }

    fun link(link: Link, paired: Paired?): Notification {
        val host = paired?.host ?: "your computer"
        val title = when (link) {
            is Link.Live -> "Linked to $host"
            Link.Connecting -> "Connecting to $host"
            Link.NoTailnet -> "Tailscale is off"
            is Link.Down -> "Can't reach $host"
            Link.Unpaired -> "Not paired"
        }
        return Notification.Builder(app, LINK)
            .setSmallIcon(R.drawable.ic_cloche)
            .setContentTitle(title)
            .setContentIntent(home())
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setShowWhen(false)
            .build()
    }

    private fun base(channel: String, d: Delivery): Notification.Builder {
        val b = Notification.Builder(app, channel)
            .setSmallIcon(R.drawable.ic_cloche)
            .setContentIntent(open(d.id, install = false))
            .setAutoCancel(true)
            .addExtras(Bundle().apply { putString(DELIVERY, d.id) })
        val icon = app.iconFile(d.id)
        if (icon.exists()) BitmapFactory.decodeFile(icon.path)?.let { b.setLargeIcon(Icon.createWithBitmap(it)) }
        return b
    }

    private fun tag(d: Delivery) = d.pkg ?: d.id

    private fun open(id: String, install: Boolean, download: Boolean = false): PendingIntent =
        PendingIntent.getActivity(
            app, (id + install + download).hashCode(),
            Intent().setComponent(MainActivity.notified(app))
                .putExtra(MainActivity.EXTRA_OPEN, id)
                .putExtra(MainActivity.EXTRA_INSTALL, install)
                .putExtra(MainActivity.EXTRA_DOWNLOAD, download)
                .addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )

    private fun home(): PendingIntent =
        PendingIntent.getActivity(app, 0, Intent(app, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)

    companion object {
        const val BUILDS = "builds"
        const val INSTALLED = "installed"
        const val PROBLEMS = "problems"
        const val LINK = "link"
        private const val QUIET = "quiet"

        private const val DELIVERY = "sh.ahmd.cloche.delivery"

        const val LINK_ID = 1
        const val BUILD = 2
        const val PROBLEM = 3
        const val REJECTED = 4
        const val UNPAIRED = 5

        fun mb(bytes: Long) = "%.1f MB".format(bytes / 1048576.0)
    }
}
