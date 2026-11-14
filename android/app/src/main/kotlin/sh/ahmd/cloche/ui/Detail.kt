package sh.ahmd.cloche.ui

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.expandHorizontally
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkHorizontally
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.getValue
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import sh.ahmd.cloche.store.Delivery
import sh.ahmd.cloche.store.Saved
import sh.ahmd.cloche.store.Stage

@Composable
fun Detail(
    d: Delivery?,
    state: Saved,
    here: Boolean,
    mobile: Boolean,
    progress: Float?,
    now: Long,
    padding: PaddingValues,
    onBack: () -> Unit,
    onInstall: (String) -> Unit,
    onLaunch: (String) -> Unit,
    onClear: (String) -> Unit,
    onDownload: (String) -> Unit,
    onShowFiles: () -> Unit,
    onDeleteFile: (String) -> Unit,
) {
    val ink = ink
    val arm = rememberArm()
    val first = remember(d?.id) { d?.stage }
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(padding)) {
        Masthead("Sheet 01 · ${d?.label ?: "Build"}", back = onBack)
        Spacer(Modifier.height(20.dp))
        if (d == null) {
            T("This delivery was cleared.", Type.body, ink.dim)
            return@Column
        }
        val apk = d.apk
        val pin = apk?.let { state.pin(it.pkg) }
        val held = state.holds(d, mobile) && progress == null
        val (word, color) = stageWord(d, held)
        val id = d.id
        var line = 0
        fun next() = 180 + 40 * line++

        Reveal("head:$id", 60) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Served(d, 56.dp)
                Spacer(Modifier.width(14.dp))
                Column(Modifier.weight(1f)) {
                    T(d.label, Type.title, maxLines = 2)
                    T(d.pkg ?: d.m.file.name, Type.small, ink.dim)
                }
                Spacer(Modifier.width(8.dp))
                if (d.stage == Stage.Installed) {
                    Stamp(word, color, thump = first != Stage.Installed)
                } else {
                    Tag(word, color, filled = d.stage == Stage.Ready && !d.fresh)
                }
            }
        }
        Spacer(Modifier.height(18.dp))

        val notice = when {
            d.stage == Stage.Blocked -> "refused"
            d.stage == Stage.Failed -> "failed"
            d.stage == Stage.Ready && d.fresh -> "new"
            d.stage == Stage.Fetching -> "fetching"
            d.stage == Stage.Installing && d.asking -> "asking"
            else -> null
        }
        AnimatedContent(notice, transitionSpec = { swap() }, label = "notice") { n ->
            when (n) {
                "refused" -> Notice("Refused", d.detail, ink.redline, plot = "refused:$id", after = 120)
                "failed" -> Notice("Failed", d.detail, ink.redline, plot = "failed:$id", after = 120)
                "new" -> Notice(
                    "New app",
                    "It isn't on this phone yet. Installing it pins its signer, and later builds must be signed with the same key.",
                    ink.line, plot = "new:$id", after = 120,
                )
                "asking" -> Notice("Waiting for you", "Android needs your OK to install it.", ink.line, plot = "asking:$id", after = 120)
                "fetching" -> Dimension(
                    progress?.let { "${(it * 100).toInt()}% of ${mb(d.m.file.size)}" }
                        ?: if (held) "waiting for Wi-Fi · ${mb(d.m.file.size)}" else "waiting to download · ${mb(d.m.file.size)}",
                    filled = progress ?: 0f,
                )
                else -> Box(Modifier.fillMaxWidth())
            }
        }
        Spacer(Modifier.height(18.dp))

        Rule("Specification", right = if (apk != null) "read on the phone" else "as the computer read it", plot = "spec:$id", after = 140)
        Spacer(Modifier.height(6.dp))
        val macPkg = d.m.apk?.pkg
        Reveal("$id:${line}", next()) {
            Spec(
                "Package", apk?.pkg ?: macPkg ?: "—",
                note = if (apk != null && macPkg != null && macPkg != apk.pkg) "the computer read it as $macPkg" else null,
                noteColor = ink.redline,
            )
        }
        Reveal("$id:${line}", next()) {
            Spec(
                "Version", apk?.version ?: d.m.apk?.let { "${it.versionName} (${it.versionCode})" } ?: "—",
                note = when {
                    apk == null -> null
                    d.stage == Stage.Installed -> "installed ${clock(d.changed)}"
                    apk.installedCode == null -> "not on this phone"
                    else -> "replaces ${apk.installedName.orEmpty()} (${apk.installedCode})".replace("  ", " ")
                },
            )
        }
        Reveal("$id:${line}", next()) {
            Spec(
                "Signer", apk?.shortSigner ?: "—",
                note = when {
                    apk == null -> null
                    pin != null && pin.signer == apk.signer -> "pinned since ${clock(pin.since)}"
                    pin != null -> "doesn't match the pinned ${pin.signer.take(16).uppercase().chunked(4).joinToString(" ")}"
                    apk.installedSameSigner == true -> "matches the installed app"
                    apk.installedSameSigner == false -> "doesn't match the installed app"
                    else -> "pinned when installed"
                },
                noteColor = if (pin != null && pin.signer != apk.signer || apk?.installedSameSigner == false) ink.redline else ink.faint,
            )
        }
        Reveal("$id:${line}", next()) { Spec("Size", mb(d.m.file.size)) }
        Reveal("$id:${line}", next()) {
            FileRow(
                d, here, arm,
                note = when {
                    here && d.stage == Stage.Installed -> "kept after installing"
                    here -> "opens in Files"
                    d.stage == Stage.Fetching -> if (held) "waiting for Wi-Fi" else "downloading"
                    d.stage == Stage.Installed && !state.keepInstalled -> "deleted after installing"
                    d.stage == Stage.Superseded -> "deleted when a newer build arrived"
                    d.stage == Stage.Blocked -> "deleted when it was refused"
                    else -> "deleted"
                },
                onShow = onShowFiles,
                onDelete = { onDeleteFile(d.id) },
            )
        }
        Reveal("$id:${line}", next()) {
            Spec(
                "SHA-256", d.m.file.sha256.take(32).chunked(4).joinToString(" ") + " …",
                note = if (apk != null) "verified" else null,
            )
        }
        if (apk != null) Reveal("$id:${line}", next()) { Spec("Android", "targets ${apk.targetSdk} · needs ${apk.minSdk}") }

        Spacer(Modifier.height(14.dp))
        Rule("Origin", plot = "origin:$id", after = next())
        Spacer(Modifier.height(6.dp))
        Reveal("$id:${line}", next()) { Spec("Computer", d.m.host, note = state.paired?.let { "signed by ${it.fingerprint}" }) }
        d.m.source?.let { Reveal("$id:${line}", next()) { Spec("Source", it.described, note = it.dir) } }
        Reveal("$id:${line}", next()) { Spec("Pushed", clock(d.m.pushed), note = "received ${ago(now, d.received)}") }
        Reveal("$id:${line}", next()) { Spec("Delivery", "#${d.seq}", note = "as cloche status numbers it") }
        if (d.m.note.isNotEmpty()) Reveal("$id:${line}", next()) { Spec("Note", "“${d.m.note}”") }

        Spacer(Modifier.height(24.dp))
        val offer = when {
            held -> "download"
            d.stage == Stage.Ready -> if (d.fresh) "install new" else "install"
            d.stage == Stage.Installing -> if (d.asking) "install" else "installing"
            (d.stage == Stage.Failed || d.stage == Stage.Declined) && here -> "retry"
            d.stage == Stage.Installed && d.pkg != null -> "open"
            else -> null
        }
        Reveal("offer:$id", next()) {
            AnimatedContent(offer, transitionSpec = { swap() }, label = "offer") { o ->
                when (o) {
                    "download" -> PlotButton("Download now", { onDownload(d.id) }, Modifier.fillMaxWidth(), primary = false)
                    "install", "install new" -> PlotButton(if (o == "install new") "Install new app" else "Install", { onInstall(d.id) }, Modifier.fillMaxWidth())
                    "installing" -> Dimension("installing", color = ink.line, busy = true, modifier = Modifier.padding(vertical = 18.dp))
                    "retry" -> PlotButton("Try again", { onInstall(d.id) }, Modifier.fillMaxWidth(), primary = false)
                    "open" -> PlotButton("Open ${d.label}", { d.pkg?.let(onLaunch) }, Modifier.fillMaxWidth(), primary = false)
                    else -> Box(Modifier.fillMaxWidth())
                }
            }
        }
        if (d.clearable) {
            Spacer(Modifier.height(10.dp))
            Reveal("clear:$id", next()) {
                Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                    ArmedWord(arm, "clear", "Clear this delivery") { onClear(d.id) }
                }
            }
        }
        Spacer(Modifier.height(8.dp))
    }
}

@Composable
private fun FileRow(d: Delivery, here: Boolean, arm: Arm, note: String?, onShow: () -> Unit, onDelete: () -> Unit) {
    val ink = ink
    val key = "file"
    val armed = arm.armed == key
    val bin by animateColorAsState(if (armed) ink.redline else ink.dim, Motion.effect(), label = "bin")
    Row(Modifier.fillMaxWidth().padding(vertical = 7.dp), verticalAlignment = Alignment.CenterVertically) {
        Label("File", ink.dim, Modifier.width(84.dp))
        Column(Modifier.weight(1f)) {
            AnimatedContent(here, transitionSpec = { roll() }, label = "file") { on ->
                if (on) {
                    T("On this phone", Type.value.copy(textDecoration = TextDecoration.Underline), ink.line, Modifier.press(give = 0.97f, onClick = onShow))
                } else {
                    T("Not on this phone", Type.value)
                }
            }
            if (!note.isNullOrEmpty()) T(note, Type.small, ink.faint)
        }
        if (here && d.stage != Stage.Fetching && d.stage != Stage.Installing) {
            AnimatedVisibility(armed, enter = fadeIn() + expandHorizontally(), exit = fadeOut() + shrinkHorizontally()) {
                Label("Tap again", ink.redline, Modifier.padding(end = 4.dp))
            }
            Box(
                Modifier
                    .press(give = 0.9f) { arm.tap(key, onDelete) }
                    .drawBehind {
                        val f = arm.fuse(key)
                        if (f > 0f) drawRect(ink.redline, Offset(0f, size.height - 2.dp.toPx()), Size(size.width * f, 2.dp.toPx()))
                    }
                    .padding(10.dp),
            ) {
                Canvas(Modifier.size(16.dp, 18.dp)) { bin(bin, 1.4.dp.toPx()) }
            }
        }
    }
}
