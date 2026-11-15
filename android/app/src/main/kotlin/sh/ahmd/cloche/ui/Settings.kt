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
import androidx.compose.foundation.background
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import sh.ahmd.cloche.BuildConfig
import sh.ahmd.cloche.deliver.Kept
import sh.ahmd.cloche.link.Link
import sh.ahmd.cloche.store.Computer
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

private val Updates = listOf(0 to "One tap", 1 to "Review first")

@Composable
fun SettingsScreen(
    state: Saved,
    links: Map<String, Link>,
    checks: Checks,
    kept: Kept,
    padding: PaddingValues,
    now: Long,
    onBack: () -> Unit,
    onFix: (String) -> Unit,
    onPairAnother: () -> Unit,
    onUnpair: (String) -> Unit,
    onRename: (String, String) -> Unit,
    onReview: (String, Boolean) -> Unit,
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

        Rule("Computers", right = state.computers.size.takeIf { it > 1 }?.toString(), after = 160)
        Spacer(Modifier.height(6.dp))
        for ((i, c) in state.computers.withIndex()) {
            if (i > 0) {
                Spacer(Modifier.height(14.dp))
                Box(Modifier.fillMaxWidth().height(1.dp).background(ink.line.copy(alpha = 0.25f)))
                Spacer(Modifier.height(8.dp))
            }
            ComputerBlock(c, state.label(c), links[c.id], now, arm, onRename, onReview, onUnpair)
        }
        Spacer(Modifier.height(14.dp))
        PlotButton("Pair another computer", onPairAnother, Modifier.fillMaxWidth(), primary = false)
        Spacer(Modifier.height(10.dp))
        T(TRUST, Type.small, ink.faint)
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
            "Cloche ${BuildConfig.VERSION_NAME} delivers builds from your computers to this phone over Tailscale. " +
                "Each build is signed by the computer that sent it and verified by Cloche before you can install it. " +
                "Updates must be signed with the same key as the app they replace.",
            Type.small, ink.faint,
        )
    }
}

@Composable
private fun ComputerBlock(
    c: Computer,
    label: String,
    link: Link?,
    now: Long,
    arm: Arm,
    onRename: (String, String) -> Unit,
    onReview: (String, Boolean) -> Unit,
    onUnpair: (String) -> Unit,
) {
    val ink = ink
    var draft by remember(c.id, c.nickname) { mutableStateOf(c.nickname) }
    val seen = when (link) {
        is Link.Live -> "connected since ${clock(link.since)}"
        is Link.Down -> link.reason
        Link.NoTailnet -> "Tailscale is off"
        else -> if (c.seen > 0) "last answered ${ago(now, c.seen)}" else "connecting"
    }
    Field(
        "Name", draft, { draft = it.take(40) }, c.offered.ifEmpty { c.host },
        KeyboardOptions(capitalization = KeyboardCapitalization.Words, imeAction = ImeAction.Done),
        onDone = { if (draft != c.nickname) onRename(c.id, draft) },
    )
    Spec("Machine", c.host, note = c.offered.takeIf { it.isNotEmpty() && it != c.host }?.let { "calls itself $it" })
    Spec("Link", seen, valueColor = if (link is Link.Down || link == Link.NoTailnet) ink.redline else ink.text)
    Spec("Address", c.addr)
    Spec("Key", c.fingerprint, note = "as cloche status shows it")
    Spec("Paired", clock(c.pairedAt), note = "as ${c.device}")
    Spacer(Modifier.height(10.dp))
    Label("Updates from it", ink.dim)
    Spacer(Modifier.height(8.dp))
    Choice(Updates, if (c.review) 1 else 0) { onReview(c.id, it == 1) }
    Spacer(Modifier.height(6.dp))
    T(
        if (c.review) "Every build opens for review before it installs." else "An update to an app it installed before takes one tap from the notification. New apps open for review.",
        Type.small, ink.faint,
    )
    Spacer(Modifier.height(10.dp))
    val key = "unpair:${c.id}"
    PlotButton(
        if (arm.armed == key) "Tap again" else "Unpair $label",
        { arm.tap(key) { onUnpair(c.id) } },
        Modifier.fillMaxWidth(), primary = false, danger = true, quiet = true,
        fuse = { arm.fuse(key) },
    )
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
fun Choice(options: List<Pair<Int, String>>, selected: Int, onSelect: (Int) -> Unit) {
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
