package sh.ahmd.cloche.ui

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import sh.ahmd.cloche.link.Link
import sh.ahmd.cloche.store.Delivery
import sh.ahmd.cloche.store.Paired
import sh.ahmd.cloche.store.Saved
import sh.ahmd.cloche.store.Stage

@Composable
fun Home(
    state: Saved,
    link: Link,
    checks: Checks,
    mobile: Boolean,
    onPhone: Set<String>,
    progress: Map<String, Float>,
    now: Long,
    padding: PaddingValues,
    list: LazyListState,
    onOpen: (String) -> Unit,
    onInstall: (String) -> Unit,
    onLaunch: (String) -> Unit,
    onDownload: (String) -> Unit,
    onRetry: () -> Unit,
    onFix: (String) -> Unit,
    onFold: () -> Unit,
    onClear: (List<String>) -> Unit,
    onSettings: () -> Unit,
) {
    val paired = state.paired
    val haptics = haptics()
    val arm = rememberArm()
    val all = remember(state.deliveries) { state.shown }
    val shown = remember(all) { prominent(all) }
    val here = remember { all.map { it.id }.toSet() }
    val listed = if (state.folded) all.filter { it.stage in Underway } else all
    val hidden = all.size - listed.size
    val clearable = all.filter { it.clearable }
    val unmet = Needs.filter { it.key != "tailscale" && !checks.met(it.key) }
    LazyColumn(Modifier.fillMaxSize(), list, contentPadding = padding) {
        item(key = "head") {
            Masthead("Sheet 01 · Builds", action = "Settings", onAction = onSettings)
            Spacer(Modifier.height(18.dp))
            LinkPanel(link, paired, now, all.maxOfOrNull { it.seq }) {
                haptics.tick()
                if (link == Link.NoTailnet) onFix("tailscale") else onRetry()
            }
        }
        if (paired != null && unmet.isNotEmpty()) {
            item(key = "needs") { NeedsPanel(unmet, onFix, Modifier.animateItem()) }
        }
        item(key = "rule") {
            Column(Modifier.animateItem()) {
                Spacer(Modifier.height(26.dp))
                Rule(
                    "Deliveries",
                    right = all.size.takeIf { it > 0 }?.toString(),
                    after = 140,
                    folded = state.folded,
                    onFold = onFold.takeIf { all.isNotEmpty() },
                    trailing = if (clearable.isNotEmpty() && !state.folded) {
                        { ArmedWord(arm, "clear", "Clear") { onClear(clearable.map { it.id }) } }
                    } else {
                        null
                    },
                )
                Spacer(Modifier.height(14.dp))
            }
        }
        if (all.isEmpty()) item(key = "empty") { Empty(paired?.host, Modifier.animateItem()) }
        items(listed, key = { it.id }) { d ->
            val i = all.indexOf(d)
            val after = if (d.id in here) 200 + 80 * i.coerceIn(0, 5) else TRAVEL - 120
            Sweep(d.clearable, { onClear(listOf(d.id)) }, Modifier.animateItem(fadeInSpec = null, fadeOutSpec = tween(160))) {
                AnimatedContent(d.id in shown, transitionSpec = { swap() }, label = "delivery") { card ->
                    if (card) {
                        Column {
                            val held = state.holds(d, mobile) && d.id !in progress
                            DeliveryCard(d, d.id in onPhone, held, progress[d.id], now, after, onOpen, onInstall, onLaunch, onDownload)
                            Spacer(Modifier.height(14.dp))
                        }
                    } else {
                        DeliveryRow(d, d.id in onPhone, now, onOpen)
                    }
                }
            }
        }
        if (hidden > 0) {
            item(key = "folded") {
                Box(Modifier.animateItem().fillMaxWidth().press { onFold() }.padding(vertical = 10.dp)) {
                    T(if (hidden == 1) "1 more, folded away" else "$hidden more, folded away", Type.small, ink.faint)
                }
            }
        }
        item(key = "title") {
            Column(Modifier.animateItem()) {
                Spacer(Modifier.height(28.dp))
                TitleBlock(
                    listOf(
                        "Drawing" to "Cloche",
                        "Host" to (paired?.host ?: "—"),
                        "Key" to (paired?.fingerprint?.take(9) ?: "—"),
                    ),
                    after = 420,
                )
            }
        }
    }
}

private val Underway = setOf(Stage.Fetching, Stage.Ready, Stage.Installing)

private fun prominent(ds: List<Delivery>): Set<String> {
    val newest = ds.groupBy { it.pkg ?: it.id }.values.mapNotNull { g -> g.maxByOrNull { it.seq }?.id }
    return newest.toSet() + ds.filter { it.stage in Underway }.map { it.id }
}

@Composable
private fun LinkPanel(link: Link, paired: Paired?, now: Long, pulse: Long?, onTap: () -> Unit) {
    val ink = ink
    val host = paired?.host ?: "your computer"
    val (word, color, line) = when (link) {
        is Link.Live -> Triple("Live", ink.live, "Connected since ${clock(link.since)}")
        Link.Connecting -> Triple("Linking", ink.dim, "Connecting to $host")
        Link.NoTailnet -> Triple("No tailnet", ink.redline, "Tailscale is off. Tap to open it.")
        is Link.Down -> Triple(
            "Down", ink.redline,
            "${link.reason}\n${if (link.retry <= now) "Retrying now" else "Retrying in ${(link.retry - now + 999) / 1000}s"}",
        )
        Link.Unpaired -> Triple("Unpaired", ink.redline, "Pair to receive builds")
    }
    val t = rememberPlot("panel")
    Column(Modifier.fillMaxWidth().press(onClick = onTap).drafted(ink.line.copy(alpha = 0.7f), drawn = { t.value }).padding(14.dp)) {
        LinkDiagram(link, paired?.host, Modifier.reveal { t.value }, pulse = pulse, after = 320)
        Spacer(Modifier.height(6.dp))
        Row(Modifier.reveal { t.value }, verticalAlignment = Alignment.Top) {
            Tag(word, color, filled = link is Link.Live)
            Spacer(Modifier.width(10.dp))
            T(line, Type.small, ink.dim, Modifier.weight(1f))
        }
    }
}

@Composable
private fun NeedsPanel(unmet: List<Need>, onFix: (String) -> Unit, modifier: Modifier) {
    val ink = ink
    val t = rememberPlot("needs", 200)
    val solid = ink.solid(ink.redline)
    Column(modifier.padding(top = 14.dp).fillMaxWidth().ground(solid, ink.redline) { t.value }.drafted(ink.redline, drawn = { t.value }).padding(horizontal = 14.dp, vertical = 8.dp)) {
        for (need in unmet) {
            Row(Modifier.reveal { t.value }.padding(vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    T(need.name, Type.value)
                    T(need.unmet, Type.small, if (solid) ink.dim else ink.redline)
                }
                Spacer(Modifier.width(8.dp))
                Box(Modifier.press(give = 0.94f) { onFix(need.key) }.padding(8.dp)) { Label(need.action, ink.line) }
            }
        }
    }
}

@Composable
private fun Empty(host: String?, modifier: Modifier) {
    val ink = ink
    val t = rememberPlot("empty", 200)
    Column(modifier.fillMaxWidth().drafted(ink.line.copy(alpha = 0.6f), dash = 1f, drawn = { t.value }).padding(18.dp)) {
        Column(Modifier.reveal { t.value }) {
            Canvas(Modifier.size(64.dp, 52.dp).align(Alignment.CenterHorizontally)) { cloche(ink.line, 1.5.dp.toPx()) }
            Spacer(Modifier.height(14.dp))
            T("Nothing under the cloche.", Type.value)
            Spacer(Modifier.height(8.dp))
            T("In a session on ${host ?: "your computer"}, tell the agent to use cloche, or run", Type.small, ink.dim)
            Spacer(Modifier.height(8.dp))
            T("cloche push app.apk -m \"what changed\"", Type.small, ink.line, Modifier.fillMaxWidth().background(ink.line.copy(alpha = 0.08f)).padding(8.dp))
            Spacer(Modifier.height(8.dp))
            T("Builds show up here, with a notification. Tap one to install it.", Type.small, ink.dim)
        }
    }
}

private fun offer(d: Delivery, hasFile: Boolean, held: Boolean): String? = when {
    held -> "download"
    d.stage == Stage.Ready && !d.fresh -> "install"
    d.stage == Stage.Installing && d.asking -> "install"
    d.stage == Stage.Ready -> "review"
    (d.stage == Stage.Failed || d.stage == Stage.Declined) && hasFile -> "retry"
    d.stage == Stage.Installed && d.pkg != null -> "open"
    else -> null
}

@Composable
private fun DeliveryCard(
    d: Delivery,
    here: Boolean,
    held: Boolean,
    progress: Float?,
    now: Long,
    after: Int,
    onOpen: (String) -> Unit,
    onInstall: (String) -> Unit,
    onLaunch: (String) -> Unit,
    onDownload: (String) -> Unit,
) {
    val ink = ink
    val t = rememberPlot(d.id, after)
    val (word, color) = stageWord(d, held)
    val first = remember(d.id) { d.stage }
    val border by animateColorAsState(
        when (d.stage) {
            Stage.Blocked, Stage.Failed -> ink.redline
            Stage.Ready, Stage.Fetching, Stage.Installing -> ink.line
            else -> ink.line.copy(alpha = 0.5f)
        },
        Motion.effect(), label = "border",
    )
    val dash by animateFloatAsState(if (d.stage == Stage.Fetching) 1f else 0f, tween(600), label = "dash")
    Column(Modifier.fillMaxWidth().press { onOpen(d.id) }.drafted(border, dash = dash, drawn = { t.value }).padding(14.dp)) {
        Column(Modifier.reveal { t.value }) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Served(d, 40.dp) { t.value }
                Spacer(Modifier.width(12.dp))
                Column(Modifier.weight(1f)) {
                    T(d.label, Type.value, maxLines = 1)
                    T(d.pkg ?: d.m.file.name, Type.small, ink.dim, maxLines = 1)
                }
                Spacer(Modifier.width(8.dp))
                if (d.stage == Stage.Installed) {
                    Stamp(word, color, thump = first != Stage.Installed)
                } else {
                    Tag(word, color, filled = d.stage == Stage.Ready && !d.fresh)
                }
            }
            Spacer(Modifier.height(10.dp))
            val version = d.apk?.version ?: d.m.apk?.let { if (it.versionName.isEmpty()) "${it.versionCode}" else "${it.versionName} (${it.versionCode})" }
            T(listOfNotNull(version, ago(now, d.m.pushed)).joinToString(" · "), Type.small, ink.dim)
            if (d.m.note.isNotEmpty()) {
                Spacer(Modifier.height(4.dp))
                T("“${d.m.note}”", Type.body)
            }
            d.m.source?.let {
                Spacer(Modifier.height(2.dp))
                T(it.described, Type.small, ink.faint, maxLines = 1)
            }
            if (d.detail.isNotEmpty() && d.stage in setOf(Stage.Blocked, Stage.Failed, Stage.Declined)) {
                Spacer(Modifier.height(6.dp))
                T(d.detail, Type.small, if (d.stage == Stage.Declined) ink.dim else ink.redline)
            }
            Spacer(Modifier.height(12.dp))
            when (d.stage) {
                Stage.Fetching -> Dimension(
                    when {
                        progress != null -> "${(progress * 100).toInt()}% of ${mb(d.m.file.size)}"
                        held -> "waiting for Wi-Fi · ${mb(d.m.file.size)}"
                        else -> "waiting · ${mb(d.m.file.size)}"
                    },
                    filled = progress ?: 0f,
                )
                Stage.Installing -> if (d.asking) {
                    Dimension("waiting for your OK · ${mb(d.m.file.size)}", color = ink.line)
                } else {
                    Dimension("installing · ${mb(d.m.file.size)}", color = ink.line, busy = true)
                }
                else -> Dimension("${mb(d.m.file.size)} · ${if (here) "on phone" else "not on phone"}", file = here)
            }
            AnimatedContent(offer(d, here, held), transitionSpec = { swap() }, label = "offer") { o ->
                when (o) {
                    "download" -> Action { PlotButton("Download now", { onDownload(d.id) }, Modifier.fillMaxWidth(), primary = false) }
                    "install" -> Action { PlotButton("Install", { onInstall(d.id) }, Modifier.fillMaxWidth()) }
                    "review" -> Action { PlotButton("Review new app", { onOpen(d.id) }, Modifier.fillMaxWidth(), primary = false) }
                    "retry" -> Action { PlotButton("Try again", { onInstall(d.id) }, Modifier.fillMaxWidth(), primary = false) }
                    "open" -> Action { PlotButton("Open", { d.pkg?.let(onLaunch) }, Modifier.fillMaxWidth(), primary = false) }
                    else -> Box(Modifier.fillMaxWidth())
                }
            }
        }
    }
}

@Composable
private fun Action(content: @Composable () -> Unit) {
    Column {
        Spacer(Modifier.height(14.dp))
        content()
    }
}

@Composable
private fun DeliveryRow(d: Delivery, here: Boolean, now: Long, onOpen: (String) -> Unit) {
    val ink = ink
    val (word, color) = stageWord(d)
    Row(Modifier.fillMaxWidth().press { onOpen(d.id) }.padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
        T(listOfNotNull(d.label, d.apk?.versionName).joinToString(" "), Type.small, ink.dim, Modifier.weight(1f), maxLines = 1)
        T(ago(now, d.m.pushed), Type.small, ink.faint)
        Spacer(Modifier.width(10.dp))
        FileMark(here)
        Spacer(Modifier.width(8.dp))
        Label(word, color)
    }
}
