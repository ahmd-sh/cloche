package sh.ahmd.cloche.ui

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.offset
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.shrinkVertically
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import sh.ahmd.cloche.link.Link
import sh.ahmd.cloche.link.PairTicket
import sh.ahmd.cloche.link.Tailnet
import sh.ahmd.cloche.link.hex
import sh.ahmd.cloche.store.Computer

private const val CODE = 12

const val TRUST = "Only pair with computers you trust. They can send builds to this phone, and each one installs only when you tap Install."

fun cleanCode(s: String) = s.uppercase().filter { it in 'A'..'Z' || it in '0'..'9' }.take(CODE)

@Composable
fun PairScreen(
    ticket: PairTicket?,
    unpaired: String?,
    busy: Boolean,
    paired: String?,
    error: String?,
    errors: Int,
    tailnetUp: Boolean,
    computers: List<Computer>,
    padding: PaddingValues,
    onPair: (PairTicket, String, String?) -> Unit,
    onBack: (() -> Unit)?,
) {
    val ink = ink
    val haptics = haptics()
    var mac by rememberSaveable(ticket) { mutableStateOf(ticket?.addr ?: "") }
    var code by rememberSaveable(ticket) { mutableStateOf(ticket?.code?.let(::cleanCode) ?: "") }
    val same = ticket?.let { t -> computers.firstOrNull { t.matches(it) } }
    val machine = ticket?.host ?: mac.substringBefore(':').substringBefore('.').trim()
    val clash = if (same != null || machine.isBlank()) null else computers.firstOrNull { it.host.equals(machine, ignoreCase = true) }
    var name by rememberSaveable(ticket) { mutableStateOf(same?.name ?: ticket?.name.orEmpty()) }
    var replace by rememberSaveable(ticket, clash?.id) {
        mutableStateOf(clash != null && Tailnet.hostOf(clash.addr) == Tailnet.hostOf(ticket?.addr ?: mac))
    }
    LaunchedEffect(ticket) {
        val typed = ticket?.code?.let(::cleanCode) ?: return@LaunchedEffect
        code = ""
        delay(380)
        for (c in typed) {
            delay(42)
            code += c
            haptics.patter()
        }
        haptics.ready()
    }
    val ready = !busy && paired == null && mac.isNotBlank() && code.length == CODE
    fun submit() {
        if (!ready) return
        val t = ticket?.takeIf { mac == it.addr && code == cleanCode(it.code) } ?: PairTicket.typed(mac, code.chunked(4).joinToString("-"))
        onPair(t, name, clash?.id?.takeIf { replace })
    }
    val link = remember(paired != null, busy) {
        when {
            paired != null -> Link.Live(System.currentTimeMillis())
            busy -> Link.Connecting
            else -> Link.Unpaired
        }
    }

    Column(Modifier.fillMaxSize().imePadding().verticalScroll(rememberScrollState()).padding(padding)) {
        Masthead("Sheet 00 · Pairing", back = onBack)
        Spacer(Modifier.height(18.dp))
        LinkDiagram(link, ticket?.host ?: mac.substringBefore(':').substringBefore('.').ifBlank { null }, draw = haptics.rise)
        Spacer(Modifier.height(10.dp))
        AnimatedVisibility(paired != null, enter = expandVertically(Motion.spatial()) + fadeIn(), exit = shrinkVertically() + fadeOut()) {
            Column {
                Notice("Paired", "Key ${paired.orEmpty()} is pinned. Builds it signs come here.", ink.live, plot = "paired:$paired", after = haptics.rise)
                Spacer(Modifier.height(18.dp))
            }
        }
        if (unpaired != null && paired == null) {
            Notice("Unpaired", unpaired, ink.redline, plot = "unpaired", after = 200)
            Spacer(Modifier.height(18.dp))
        }
        if (same != null && paired == null) {
            Notice("Paired before", "This phone already has ${same.name} with this key. Pairing again refreshes it.", ink.line, plot = "again", after = 200)
            Spacer(Modifier.height(18.dp))
        }
        if (clash != null && paired == null) {
            Notice(
                "Same name as ${clash.name}",
                "${clash.host} is paired already, with key ${clash.fingerprint}. This may be that computer with a new key, or another one.",
                ink.redline, plot = "clash", after = 200,
            )
            Spacer(Modifier.height(10.dp))
            Choice(listOf(1 to "Replace it", 0 to "Keep both"), if (replace) 1 else 0) { replace = it == 1 }
            Spacer(Modifier.height(18.dp))
        }

        Rule("Steps", after = 100)
        Spacer(Modifier.height(12.dp))
        Reveal("step:1", 200) {
            Step("1", "On your computer, run") {
                T("cloche pair -android", Type.small, ink.line, Modifier.background(ink.line.copy(alpha = 0.08f)).padding(horizontal = 8.dp, vertical = 4.dp))
            }
        }
        Reveal("step:2", 260) {
            Step("2", "Scan the QR code it shows, or type it in:")
        }
        Spacer(Modifier.height(6.dp))
        Reveal("fields", 320) {
            Column {
                Field(
                    "Computer", mac, { mac = it.trim() }, "my-laptop",
                    KeyboardOptions(keyboardType = KeyboardType.Uri, autoCorrectEnabled = false, imeAction = ImeAction.Next),
                )
                CodeField(code, { code = it }, accepted = paired != null, errors = errors, rise = haptics.rise, onDone = ::submit)
                Field(
                    "Name", name, { name = it.take(40) }, ticket?.name ?: "as the computer calls itself",
                    KeyboardOptions(capitalization = KeyboardCapitalization.Words, imeAction = ImeAction.Done),
                )
            }
        }
        if (ticket != null && ticket.display.isNotEmpty() && ticket.display != ticket.host) {
            Spec("Machine", ticket.host, note = "its name on your tailnet")
        }
        ticket?.fingerprint?.let {
            Spec("Key", it.copyOf(8).hex().uppercase().chunked(4).joinToString(" "), note = "from the link")
        }
        Reveal("foot", 380) {
            Column {
                AnimatedVisibility(!tailnetUp && ticket?.addr?.startsWith("127.") != true) {
                    T("Tailscale is off. Turn it on to pair.", Type.small, ink.redline, Modifier.padding(top = 6.dp))
                }
                AnimatedContent(error, transitionSpec = { swap() }, label = "error") { e ->
                    if (e != null) T(e, Type.small, ink.redline, Modifier.fillMaxWidth().padding(top = 6.dp)) else Box(Modifier.fillMaxWidth())
                }
                Spacer(Modifier.height(18.dp))
                T(TRUST, Type.small, ink.dim)
                Spacer(Modifier.height(14.dp))
                PlotButton(
                    when {
                        paired != null -> "Paired"
                        busy -> "Pairing…"
                        else -> "Pair"
                    },
                    ::submit,
                    Modifier.fillMaxWidth(),
                    enabled = ready,
                    tone = if (paired != null) ink.live else null,
                )
                Spacer(Modifier.height(18.dp))
                T("Cloche connects to your computer over Tailscale. Pairing pins its key, and every build must be signed with it.", Type.small, ink.faint)
            }
        }
    }
}

@Composable
private fun CodeField(code: String, onChange: (String) -> Unit, accepted: Boolean, errors: Int, rise: Int, onDone: () -> Unit) {
    val ink = ink
    val haptics = haptics()
    val keyboard = LocalSoftwareKeyboardController.current
    val focus = remember { FocusRequester() }
    var focused by remember { mutableStateOf(false) }
    var wrong by remember { mutableStateOf(false) }
    var field by remember { mutableStateOf(TextFieldValue(code, TextRange(code.length))) }
    if (field.text != code) field = TextFieldValue(code, TextRange(code.length))
    val shake = remember { Animatable(0f) }
    LaunchedEffect(errors) {
        if (errors == 0) return@LaunchedEffect
        wrong = true
        shake.animateTo(0f, spring(0.22f, 1100f), initialVelocity = 520f)
        delay(400)
        wrong = false
    }
    val blink = rememberInfiniteTransition(label = "caret").animateFloat(0f, 1f, infiniteRepeatable(tween(1000, easing = LinearEasing)), label = "blink")
    fun put(next: TextFieldValue) {
        if (next.text != field.text) {
            when {
                next.text.length < field.text.length -> haptics.key()
                next.text.length == CODE && field.text.length < CODE -> haptics.ready()
                next.text.length % 4 == 0 && next.text.length > field.text.length -> haptics.group()
                else -> haptics.key()
            }
        }
        field = next
        if (next.text != code) onChange(next.text)
    }
    Column(Modifier.fillMaxWidth().padding(vertical = 8.dp)) {
        Label("Code")
        Spacer(Modifier.height(2.dp))
        BasicTextField(
            field,
            { raw -> put(typed(field, raw)) },
            Modifier.fillMaxWidth().focusRequester(focus).onFocusChanged { focused = it.isFocused },
            singleLine = true,
            keyboardOptions = KeyboardOptions(
                capitalization = KeyboardCapitalization.Characters,
                autoCorrectEnabled = false,
                keyboardType = KeyboardType.Password,
                imeAction = ImeAction.Done,
            ),
            keyboardActions = KeyboardActions(onDone = { onDone() }),
            decorationBox = {
                val sel = field.selection
                val at = sel.start.coerceIn(0, field.text.length)
                Row(
                    Modifier.fillMaxWidth().graphicsLayer { translationX = shake.value.dp.toPx() },
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    for (g in 0 until 3) {
                        if (g > 0) T("–", Type.field, ink.faint, Modifier.padding(horizontal = 3.dp))
                        for (j in 0 until 4) {
                            val i = g * 4 + j
                            val here = focused && !accepted && sel.collapsed
                            Cell(
                                field.text.getOrNull(i), i,
                                caret = when {
                                    !here -> null
                                    at == i && i == field.text.length -> Caret.Empty
                                    at == i -> Caret.Before
                                    at == CODE && i == CODE - 1 -> Caret.After
                                    else -> null
                                },
                                selected = focused && !sel.collapsed && i >= sel.min && i < sel.max,
                                accepted = accepted, wrong = wrong, rise = rise,
                                blink = { if (blink.value < 0.55f) 1f else 0f },
                                modifier = Modifier.weight(1f),
                                onTap = {
                                    if (accepted) return@Cell
                                    haptics.tick()
                                    val n = field.text.length
                                    put(field.copy(selection = if (i < n) TextRange(i, i + 1) else TextRange(n)))
                                    focus.requestFocus()
                                    keyboard?.show()
                                },
                            )
                        }
                    }
                }
            },
        )
    }
}

internal fun typed(old: TextFieldValue, raw: TextFieldValue): TextFieldValue {
    val upper = raw.text.uppercase()
    val clean = upper.filter { it in 'A'..'Z' || it in '0'..'9' }
    if (clean != upper) return cleanCode(clean).let { TextFieldValue(it, TextRange(it.length)) }
    val p = raw.selection.start
    if (clean.length > CODE && old.text.length == CODE && raw.selection.collapsed && p < clean.length) {
        val over = clean.removeRange(p, minOf(p + clean.length - CODE, clean.length))
        if (over.length == CODE) return TextFieldValue(over, TextRange(p))
    }
    val t = clean.take(CODE)
    return TextFieldValue(t, TextRange(raw.selection.start.coerceAtMost(t.length), raw.selection.end.coerceAtMost(t.length)))
}

private enum class Caret { Empty, Before, After }

@Composable
private fun Cell(
    ch: Char?,
    index: Int,
    caret: Caret?,
    selected: Boolean,
    accepted: Boolean,
    wrong: Boolean,
    rise: Int,
    blink: () -> Float,
    modifier: Modifier,
    onTap: () -> Unit,
) {
    val ink = ink
    val strong = caret != null || selected
    val ruleColor = when {
        accepted -> ink.live
        wrong -> ink.redline
        ch != null || strong -> ink.line
        else -> ink.faint
    }
    val rule by animateColorAsState(ruleColor, if (accepted) tween(140, index * rise / CODE) else Motion.effect(), label = "rule")
    val text by animateColorAsState(
        when {
            accepted -> ink.live
            selected -> ink.paper
            else -> ink.text
        },
        if (accepted) tween(140, index * rise / CODE) else Motion.effect(), label = "text",
    )
    val fill by animateFloatAsState(if (selected) 1f else 0f, Motion.effect(), label = "fill")
    Box(
        modifier
            .height(46.dp)
            .padding(horizontal = 1.5.dp)
            .drawBehind { if (fill > 0f) drawRect(ink.line.copy(alpha = fill), Offset(0f, size.height * 0.12f), Size(size.width, size.height * 0.76f)) }
            .pointerInput(Unit) { detectTapGestures { onTap() } },
        contentAlignment = Alignment.Center,
    ) {
        AnimatedContent(
            ch,
            transitionSpec = { (fadeIn(tween(90)) + scaleIn(spring(0.45f, 700f), initialScale = 1.6f)) togetherWith fadeOut(tween(70)) },
            contentAlignment = Alignment.Center,
            label = "char",
        ) { c ->
            T(c?.toString() ?: " ", Type.field, text)
        }
        val bar = Modifier.width(2.dp).height(22.dp).graphicsLayer { alpha = blink() }.background(ink.line)
        when (caret) {
            Caret.Empty -> Box(bar)
            Caret.Before -> Box(Modifier.align(Alignment.CenterStart).offset(x = (-2.5).dp).then(bar))
            Caret.After -> Box(Modifier.align(Alignment.CenterEnd).offset(x = 2.5.dp).then(bar))
            null -> {}
        }
        Box(Modifier.align(Alignment.BottomCenter).fillMaxWidth().height(if (strong) 2.dp else 1.dp).background(rule))
    }
}

@Composable
private fun Step(n: String, text: String, extra: (@Composable () -> Unit)? = null) {
    val ink = ink
    Row(Modifier.fillMaxWidth().padding(vertical = 6.dp)) {
        T(n, Type.value, ink.line, Modifier.width(26.dp))
        Column(Modifier.weight(1f)) {
            T(text, Type.body, ink.text)
            if (extra != null) {
                Spacer(Modifier.height(6.dp))
                extra()
            }
        }
    }
}
