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
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import sh.ahmd.cloche.link.Link
import sh.ahmd.cloche.link.Links
import sh.ahmd.cloche.store.Computer
import sh.ahmd.cloche.store.Delivery
import sh.ahmd.cloche.store.Saved
import sh.ahmd.cloche.store.Stage

@Composable
fun Home(
    state: Saved,
    links: Map<String, Link>,
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
    val computers = state.computers
    val haptics = haptics()
    val arm = rememberArm()
    var only by rememberSaveable { mutableStateOf<String?>(null) }
    val filter = only?.takeIf { id -> computers.size > 1 && computers.any { it.id == id } }
    val all = remember(state.deliveries, filter) { state.shown.filter { filter == null || it.from == filter } }
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
            val link = Links.overall(computers, links)
            LinkPanel(link, Links.of(computers, links), now, state.deliveries.maxOfOrNull { it.received }) {
                haptics.tick()
                if (link == Link.NoTailnet) onFix("tailscale") else onRetry()
            }
        }
        if (computers.isNotEmpty() && unmet.isNotEmpty()) {
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
                if (computers.size > 1) {
                    Filter(computers, filter) {
                        haptics.tick()
                        only = it
                    }
                    Spacer(Modifier.height(14.dp))
                }
            }
        }
        if (all.isEmpty()) item(key = "empty") { Empty(computers.singleOrNull()?.name ?: computers.find { it.id == filter }?.name, Modifier.animateItem()) }
        items(listed, key = { it.id }) { d ->
            val i = all.indexOf(d)
            val after = if (d.id in here) 200 + 80 * i.coerceIn(0, 5) else TRAVEL - 120
            Sweep(d.clearable, { onClear(listOf(d.id)) }, Modifier.animateItem(fadeInSpec = null, fadeOutSpec = tween(160))) {
                AnimatedContent(d.id in shown, transitionSpec = { swap() }, label = "delivery") { card ->
                    if (card) {
                        Column {
                            val held = state.holds(d, mobile) && d.id !in progress
                            val from = if (computers.size > 1 && filter == null) state.named(d) else null
                            DeliveryCard(d, state.reviews(d), from, d.id in onPhone, held, progress[d.id], now, after, onOpen, onInstall, onLaunch, onDownload)
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
                val one = computers.singleOrNull()
                TitleBlock(
                    listOf(
                        "Drawing" to "Cloche",
                        "Host" to (one?.name ?: if (computers.isEmpty()) "—" else "${computers.size} computers"),
                        "Key" to (one?.fingerprint?.take(9) ?: "—"),
                    ),
                    after = 420,
                )
            }
        }
    }
}

private val Underway = setOf(Stage.Fetching, Stage.Ready, Stage.Installing)

private fun prominent(ds: List<Delivery>): Set<String> {
    val newest = ds.groupBy { it.pkg ?: it.id }.values.mapNotNull { g -> g.reduceOrNull { a, b -> if (b.newerThan(a)) b else a }?.id }
    return newest.toSet() + ds.filter { it.stage in Underway }.map { it.id }
}

@Composable
private fun linkWords(link: Link, host: String, now: Long): Triple<String, Color, String> {
    val ink = ink
    return when (link) {
        is Link.Live -> Triple("Live", ink.live, "Connected since ${clock(link.since)}")
        Link.Connecting -> Triple("Linking", ink.dim, "Connecting to $host")
        Link.NoTailnet -> Triple("No tailnet", ink.redline, "Tailscale is off. Tap to open it.")
        is Link.Down -> Triple("Down", ink.redline, "${link.reason}\n${retry(link, now)}")
        Link.Unpaired -> Triple("Unpaired", ink.redline, "Pair to receive builds")
    }
}

private fun retry(link: Link.Down, now: Long): String {
    val s = (link.retry - now + 999) / 1000
    return when {
        link.retry <= now -> "Retrying now"
        s >= 120 -> "Retrying in ${s / 60} min"
        else -> "Retrying in ${s}s"
    }
}

@Composable
private fun LinkPanel(link: Link, each: List<Pair<Computer, Link>>, now: Long, pulse: Long?, onTap: () -> Unit) {
    val ink = ink
    val one = each.singleOrNull()?.first
    val label = one?.name ?: if (each.isEmpty()) null else "${each.size} computers"
    val t = rememberPlot("panel")
    Column(Modifier.fillMaxWidth().press(onClick = onTap).drafted(ink.line.copy(alpha = 0.7f), drawn = { t.value }).padding(14.dp)) {
        LinkDiagram(link, label, Modifier.reveal { t.value }, pulse = pulse, after = 320)
        Spacer(Modifier.height(6.dp))
        if (each.size <= 1 || link == Link.NoTailnet) {
            val (word, color, line) = linkWords(link, one?.name ?: "your computer", now)
            Row(Modifier.reveal { t.value }, verticalAlignment = Alignment.Top) {
                Tag(word, color, filled = link is Link.Live)
                Spacer(Modifier.width(10.dp))
                T(line, Type.small, ink.dim, Modifier.weight(1f))
            }
        } else {
            for ((c, l) in each) {
                val (word, color, line) = linkWords(l, c.name, now)
                Row(Modifier.reveal { t.value }.padding(vertical = 3.dp), verticalAlignment = Alignment.Top) {
                    Tag(word, color, filled = l is Link.Live)
                    Spacer(Modifier.width(10.dp))
                    Column(Modifier.weight(1f)) {
                        T(c.name, Type.small, ink.text, maxLines = 1)
                        if (l !is Link.Live) T(line, Type.small, ink.dim)
                    }
                }
            }
        }
    }
}

@Composable
private fun Filter(computers: List<Computer>, selected: String?, onSelect: (String?) -> Unit) {
    val ink = ink
    Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()), verticalAlignment = Alignment.CenterVertically) {
        val options = listOf<Pair<String?, String>>(null to "All") + computers.map { it.id to it.name }
        for ((i, o) in options.withIndex()) {
            if (i > 0) Spacer(Modifier.width(8.dp))
            val on = o.first == selected
            Box(Modifier.press(give = 0.94f) { if (!on) onSelect(o.first) }.padding(vertical = 4.dp)) {
                Tag(o.second, if (on) ink.line else ink.dim, filled = on)
            }
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

private fun offer(d: Delivery, review: Boolean, hasFile: Boolean, held: Boolean): String? = when {
    held -> "download"
    d.stage == Stage.Ready && !review -> "install"
    d.stage == Stage.Installing && d.asking -> "install"
    d.stage == Stage.Ready -> "review"
    (d.stage == Stage.Failed || d.stage == Stage.Declined) && hasFile -> "retry"
    d.stage == Stage.Installed && d.pkg != null -> "open"
    else -> null
}

@Composable
private fun DeliveryCard(
    d: Delivery,
    review: Boolean,
    from: String?,
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
                    Tag(word, color, filled = d.stage == Stage.Ready && !review)
                }
            }
            Spacer(Modifier.height(10.dp))
            val version = d.apk?.version ?: d.m.apk?.let { if (it.versionName.isEmpty()) "${it.versionCode}" else "${it.versionName} (${it.versionCode})" }
            T(listOfNotNull(version, ago(now, d.m.pushed)).joinToString(" · "), Type.small, ink.dim)
            if (d.m.note.isNotEmpty()) {
                Spacer(Modifier.height(4.dp))
                T("“${d.m.note}”", Type.body)
            }
            listOfNotNull(d.m.source?.described, from?.let { "from $it" }).takeIf { it.isNotEmpty() }?.let {
                Spacer(Modifier.height(2.dp))
                T(it.joinToString(" · "), Type.small, ink.faint, maxLines = 1)
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
            AnimatedContent(offer(d, review, here, held), transitionSpec = { swap() }, label = "offer") { o ->
                when (o) {
                    "download" -> Action { PlotButton("Download now", { onDownload(d.id) }, Modifier.fillMaxWidth(), primary = false) }
                    "install" -> Action { PlotButton("Install", { onInstall(d.id) }, Modifier.fillMaxWidth()) }
                    "review" -> Action { PlotButton(if (d.fresh) "Review new app" else "Review", { onOpen(d.id) }, Modifier.fillMaxWidth(), primary = false) }
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
