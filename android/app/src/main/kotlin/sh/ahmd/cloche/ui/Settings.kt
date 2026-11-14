package sh.ahmd.cloche.ui

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.expandHorizontally
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.shrinkHorizontally
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.unit.dp
import sh.ahmd.cloche.BuildConfig
import sh.ahmd.cloche.deliver.Kept
import sh.ahmd.cloche.store.Saved

data class Checks(val notifications: Boolean, val installs: Boolean, val battery: Boolean, val tailnet: Boolean) {
    val ok: Boolean get() = notifications && installs && battery && tailnet
}

data class Need(val key: String, val name: String, val met: String, val unmet: String, val action: String)

val Needs = listOf(
    Need("tailscale", "Tailscale", "Connected", "Disconnected", "Open"),
    Need("notifications", "Notifications", "For pushed builds", "Off: builds arrive without a notification", "Allow"),
    Need("installs", "Install apps", "To install pushed builds", "Needed to install builds", "Allow"),
    Need("battery", "Disable battery optimization", "Keeps Cloche running reliably in the background", "Android may stop Cloche in the background", "Allow"),
)

fun Checks.met(key: String) = when (key) {
    "tailscale" -> tailnet
    "notifications" -> notifications
    "installs" -> installs
    "battery" -> battery
    else -> true
}

private val OnMobile = listOf(1 to "Download", 0 to "Wait for Wi-Fi")

private val AfterInstall = listOf(0 to "Delete file", 1 to "Keep file")

private val Keep = listOf(0 to "Never", 1 to "1 day", 7 to "1 week", 30 to "1 month")

@Composable
fun SettingsScreen(
    state: Saved,
    checks: Checks,
    kept: Kept,
    padding: PaddingValues,
    onBack: () -> Unit,
    onFix: (String) -> Unit,
    onPairAgain: () -> Unit,
    onUnpair: () -> Unit,
    onForget: (String) -> Unit,
    onClearAfter: (Int) -> Unit,
    onKeepInstalled: (Boolean) -> Unit,
    onMobileData: (Boolean) -> Unit,
    onClearAll: () -> Unit,
) {
    val ink = ink
    val arm = rememberArm()
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(padding)) {
        Masthead("Sheet 02 · Settings", back = onBack)
        Spacer(Modifier.height(22.dp))

        Rule("Checks", after = 100)
        Spacer(Modifier.height(6.dp))
        for (need in Needs) Check(need, checks.met(need.key), onFix)
        Spacer(Modifier.height(18.dp))

        val paired = state.paired
        Rule("Computer", after = 160)
        Spacer(Modifier.height(6.dp))
        if (paired != null) {
            Spec("Host", paired.host)
            Spec("Address", paired.addr)
            Spec("Key", paired.fingerprint, note = "as cloche status shows it")
            Spec("Paired", clock(paired.pairedAt), note = "as ${paired.device}")
            Spacer(Modifier.height(12.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                PlotButton("Pair again", onPairAgain, Modifier.weight(1f), primary = false)
                PlotButton(
                    if (arm.armed == "unpair") "Tap again" else "Unpair",
                    { arm.tap("unpair", onUnpair) },
                    Modifier.weight(1f), primary = false, danger = true, quiet = true,
                    fuse = { arm.fuse("unpair") },
                )
            }
        }
        Spacer(Modifier.height(22.dp))

        Rule("Downloads", after = 200)
        Spacer(Modifier.height(10.dp))
        Label("On mobile data", ink.dim)
        Spacer(Modifier.height(8.dp))
        Choice(OnMobile, if (state.mobileData) 1 else 0) { onMobileData(it == 1) }
        Spacer(Modifier.height(22.dp))

        Rule("Storage", after = 220)
        Spacer(Modifier.height(6.dp))
        val apks = kept.apks.size
        Spec("On phone", if (apks == 0) "No APKs" else if (apks == 1) "1 APK" else "$apks APKs", note = if (kept.bytes >= 50_000) mb(kept.bytes) else null)
        Spacer(Modifier.height(8.dp))
        Label("After installing", ink.dim)
        Spacer(Modifier.height(8.dp))
        Choice(AfterInstall, if (state.keepInstalled) 1 else 0) { onKeepInstalled(it == 1) }
        Spacer(Modifier.height(16.dp))
        Label("Clear deliveries after", ink.dim)
        Spacer(Modifier.height(8.dp))
        Choice(Keep, state.clearAfter, onClearAfter)
        Spacer(Modifier.height(6.dp))
        T("Clearing a delivery deletes its file.", Type.small, ink.faint)
        Spacer(Modifier.height(12.dp))
        val clearable = state.shown.count { it.clearable }
        PlotButton(
            if (arm.armed == "clear") "Tap again" else "Clear all deliveries",
            { arm.tap("clear", onClearAll) },
            Modifier.fillMaxWidth(), primary = false, danger = true, quiet = true, enabled = clearable > 0,
            fuse = { arm.fuse("clear") },
        )
        Spacer(Modifier.height(22.dp))

        Rule("Pinned signers", right = state.pins.size.takeIf { it > 0 }?.toString(), after = 280)
        Spacer(Modifier.height(6.dp))
        if (state.pins.isEmpty()) {
            T("None yet. An app's signer is pinned when Cloche first installs it, and its later builds must match.", Type.small, ink.faint)
        }
        for (pin in state.pins.sortedBy { it.pkg }) {
            Row(Modifier.fillMaxWidth().padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    T(pin.pkg, Type.value, maxLines = 1)
                    T("${pin.signer.take(16).uppercase().chunked(4).joinToString(" ")} · since ${clock(pin.since)}", Type.small, ink.dim, maxLines = 1)
                }
                ArmedWord(arm, "forget:${pin.pkg}", "Forget") { onForget(pin.pkg) }
            }
        }
        Spacer(Modifier.height(22.dp))

        Rule("About", after = 340)
        Spacer(Modifier.height(8.dp))
        T(
            "Cloche ${BuildConfig.VERSION_NAME} delivers builds from your computer to this phone over Tailscale. " +
                "Each build is signed by your computer and verified by Cloche before you can install it. " +
                "Updates must be signed with the same key as the app they replace.",
            Type.small, ink.faint,
        )
    }
}

@Composable
private fun Check(need: Need, ok: Boolean, onFix: (String) -> Unit) {
    val ink = ink
    val haptics = haptics()
    var was by remember { mutableStateOf(ok) }
    LaunchedEffect(ok) {
        if (ok && !was) haptics.confirm()
        was = ok
    }
    Row(Modifier.fillMaxWidth().padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
        AnimatedContent(
            ok,
            Modifier.width(26.dp),
            transitionSpec = { (scaleIn(spring(0.45f, 600f), 0.2f) + fadeIn()) togetherWith fadeOut(tween(90)) },
            label = "check",
        ) { good ->
            T(if (good) "✓" else "✕", Type.value, if (good && ink.solid(ink.live)) ink.line else if (good) ink.live else ink.redline)
        }
        Column(Modifier.weight(1f)) {
            T(need.name, Type.value)
            AnimatedContent(if (ok) need.met else need.unmet, transitionSpec = { roll() }, label = "why") {
                T(it, Type.small, if (ok) ink.faint else ink.redline)
            }
        }
        AnimatedVisibility(!ok, enter = fadeIn() + expandHorizontally(), exit = fadeOut() + shrinkHorizontally()) {
            Row {
                Spacer(Modifier.width(8.dp))
                Box(Modifier.press(give = 0.94f) { onFix(need.key) }.padding(8.dp)) { Label(need.action, ink.line) }
            }
        }
    }
}

@Composable
private fun Choice(options: List<Pair<Int, String>>, selected: Int, onSelect: (Int) -> Unit) {
    val ink = ink
    val haptics = haptics()
    val index = options.indexOfFirst { it.first == selected }.coerceAtLeast(0)
    val at by animateFloatAsState(index.toFloat(), Motion.spatial(), label = "choice")
    Row(
        Modifier
            .fillMaxWidth()
            .height(40.dp)
            .drawBehind {
                val w = size.width / options.size
                drawRect(ink.line, Offset(w * at, 0f), Size(w, size.height))
                for (i in 1 until options.size) {
                    drawLine(ink.line.copy(alpha = 0.5f), Offset(w * i, 0f), Offset(w * i, size.height), 1.dp.toPx())
                }
            }
            .drafted(ink.line, marks = false),
    ) {
        options.forEachIndexed { i, (value, name) ->
            val c by animateColorAsState(if (i == index) ink.paper else ink.line, Motion.effect(), label = "ink")
            Box(
                Modifier.weight(1f).fillMaxHeight().press(give = 0.96f) {
                    if (value != selected) {
                        haptics.tick()
                        onSelect(value)
                    }
                },
                contentAlignment = Alignment.Center,
            ) { Label(name, c) }
        }
    }
}
