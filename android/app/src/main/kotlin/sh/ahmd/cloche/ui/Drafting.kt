package sh.ahmd.cloche.ui

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import kotlin.math.min

fun Modifier.gridPaper(ink: Ink): Modifier = drawBehind {
    drawRect(ink.paper)
    val step = 8.dp.toPx()
    var i = 0
    var x = 0f
    while (x <= size.width) {
        drawLine(if (i % 5 == 0) ink.gridMajor else ink.grid, Offset(x, 0f), Offset(x, size.height), 1f)
        x += step
        i++
    }
    i = 0
    var y = 0f
    while (y <= size.height) {
        drawLine(if (i % 5 == 0) ink.gridMajor else ink.grid, Offset(0f, y), Offset(size.width, y), 1f)
        y += step
        i++
    }
}

fun Modifier.sheetEdge(): Modifier = drawBehind {
    val w = 18.dp.toPx()
    drawRect(
        Brush.horizontalGradient(listOf(Color.Transparent, Color.Black.copy(alpha = 0.2f)), startX = -w, endX = 0f),
        Offset(-w, 0f), Size(w, size.height),
    )
}

fun Modifier.drafted(
    color: Color,
    dash: Float = 0f,
    marks: Boolean = true,
    weight: Dp = 1.dp,
    drawn: (() -> Float)? = null,
): Modifier = drawBehind {
    val t = drawn?.invoke() ?: 1f
    if (t <= 0f) return@drawBehind
    val s = weight.toPx()
    val effect = if (dash > 0.01f) PathEffect.dashPathEffect(floatArrayOf(5.dp.toPx(), 4.dp.toPx() * dash)) else null
    if (t >= 1f) {
        drawRect(color, Offset(s / 2, s / 2), Size(size.width - s, size.height - s), style = Stroke(s, pathEffect = effect))
    } else {
        trace(color, s, t, effect)
    }
    val m = ((t - 0.75f) / 0.25f).coerceIn(0f, 1f)
    if (marks && m > 0f) cornerMarks(color.copy(alpha = color.alpha * m), s, 5.dp.toPx())
}

private fun DrawScope.trace(color: Color, s: Float, t: Float, effect: PathEffect?) {
    val o = s / 2
    val r = size.width - o
    val b = size.height - o
    val w = r - o
    val h = b - o
    val d = t * (w + h)
    fun line(a: Offset, z: Offset) = drawLine(color, a, z, s, StrokeCap.Square, effect)
    line(Offset(o, o), Offset(o + min(d, w), o))
    if (d > w) line(Offset(r, o), Offset(r, o + (d - w)))
    line(Offset(o, o), Offset(o, o + min(d, h)))
    if (d > h) line(Offset(o, b), Offset(o + (d - h), b))
}

private fun DrawScope.cornerMarks(color: Color, s: Float, m: Float) {
    val o = s / 2
    val w = size.width - o
    val h = size.height - o
    for ((cx, cy, dx, dy) in listOf(floatArrayOf(o, o, -1f, -1f), floatArrayOf(w, o, 1f, -1f), floatArrayOf(o, h, -1f, 1f), floatArrayOf(w, h, 1f, 1f))) {
        drawLine(color, Offset(cx, cy), Offset(cx + dx * m, cy), s)
        drawLine(color, Offset(cx, cy), Offset(cx, cy + dy * m), s)
    }
}

fun Modifier.ground(solid: Boolean, color: Color, drawn: () -> Float = { 1f }): Modifier =
    if (!solid) this else drawBehind { drawRect(color.copy(alpha = color.alpha * 0.85f * drawn())) }

fun Modifier.press(enabled: Boolean = true, give: Float = 0.985f, onClick: () -> Unit): Modifier = composed {
    val source = remember { MutableInteractionSource() }
    val pressed by source.collectIsPressedAsState()
    val ink = ink
    val shade by animateFloatAsState(if (pressed) 0.08f else 0f, Motion.effect(), label = "shade")
    val scale by animateFloatAsState(if (pressed) give else 1f, Motion.press(pressed), label = "give")
    this
        .graphicsLayer {
            scaleX = scale
            scaleY = scale
        }
        .clickable(source, indication = null, enabled = enabled, role = Role.Button, onClick = onClick)
        .drawBehind { if (shade > 0f) drawRect(ink.line.copy(alpha = shade)) }
}

@Composable
fun PlotButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    primary: Boolean = true,
    danger: Boolean = false,
    enabled: Boolean = true,
    tone: Color? = null,
    fuse: (() -> Float)? = null,
    quiet: Boolean = false,
) {
    val ink = ink
    val haptics = haptics()
    val source = remember { MutableInteractionSource() }
    val pressed by source.collectIsPressedAsState()
    val base = tone ?: if (danger) ink.redline else ink.line
    val c by animateColorAsState(if (enabled || tone != null) base else base.copy(alpha = 0.4f), Motion.effect(), label = "ink")
    val solid = primary || ink.solid(base)
    val fill by animateFloatAsState(
        when {
            solid && pressed -> 0.82f
            solid -> 1f
            pressed -> 0.12f
            else -> 0f
        },
        Motion.effect(), label = "fill",
    )
    val scale by animateFloatAsState(if (pressed) 0.97f else 1f, Motion.press(pressed), label = "give")
    Box(
        modifier
            .graphicsLayer {
                scaleX = scale
                scaleY = scale
            }
            .heightIn(min = 48.dp)
            .clickable(source, indication = null, enabled = enabled, role = Role.Button) {
                if (!quiet) if (primary) haptics.confirm() else haptics.tick()
                onClick()
            }
            .drawBehind {
                if (fill > 0f) drawRect(c.copy(alpha = c.alpha * fill))
                val f = fuse?.invoke() ?: 0f
                if (f > 0f) drawRect(c, Offset(0f, size.height - 3.dp.toPx()), Size(size.width * f, 3.dp.toPx()))
            }
            .drafted(c, marks = !primary)
            .padding(horizontal = 18.dp, vertical = 12.dp),
        contentAlignment = Alignment.Center,
    ) {
        AnimatedContent(text.uppercase(), transitionSpec = { roll() }, contentAlignment = Alignment.Center, label = "button") { word ->
            T(word, Type.button, if (solid) ink.on(c) else c, maxLines = 1)
        }
    }
}

@Composable
fun Tag(text: String, color: Color, modifier: Modifier = Modifier, filled: Boolean = false) {
    val ink = ink
    val c by animateColorAsState(color, Motion.effect(), label = "ink")
    val fill by animateFloatAsState(if (filled || ink.solid(color)) 1f else 0f, Motion.effect(), label = "fill")
    Box(
        modifier
            .drawBehind { if (fill > 0f) drawRect(c.copy(alpha = c.alpha * fill)) }
            .drafted(c, marks = false)
            .padding(horizontal = 6.dp, vertical = 2.dp),
        contentAlignment = Alignment.Center,
    ) {
        AnimatedContent(text.uppercase(), transitionSpec = { roll(clip = true) }, contentAlignment = Alignment.Center, label = "tag") { word ->
            T(word, Type.label, lerp(c, ink.on(c), fill), maxLines = 1)
        }
    }
}

@Composable
fun Rule(
    label: String,
    modifier: Modifier = Modifier,
    right: String? = null,
    plot: Any = "rule:$label",
    after: Int = 0,
    folded: Boolean = false,
    onFold: (() -> Unit)? = null,
    trailing: (@Composable () -> Unit)? = null,
) {
    val ink = ink
    val t = rememberPlot(plot, after)
    val haptics = haptics()
    val turn by animateFloatAsState(if (folded) -90f else 0f, Motion.spatial(), label = "caret")
    Row(modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        if (onFold != null) {
            Row(
                Modifier.press(give = 0.96f) {
                    haptics.tick()
                    onFold()
                }.padding(vertical = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                T("▾", Type.small, ink.dim, Modifier.graphicsLayer { rotationZ = turn })
                Spacer(Modifier.width(6.dp))
                Label(label, ink.dim)
            }
        } else {
            Label(label, ink.dim)
        }
        Spacer(Modifier.width(10.dp))
        Box(
            Modifier.weight(1f).height(1.dp).drawBehind {
                drawRect(ink.line.copy(alpha = 0.5f), size = Size(size.width * t.value, size.height))
            },
        )
        if (right != null) {
            Spacer(Modifier.width(10.dp))
            AnimatedContent(right, transitionSpec = { roll() }, label = "count") { Label(it, ink.faint) }
        }
        if (trailing != null) {
            Spacer(Modifier.width(2.dp))
            trailing()
        }
    }
}

@Composable
fun Dimension(label: String, modifier: Modifier = Modifier, color: Color = ink.dim, filled: Float? = null, busy: Boolean = false, file: Boolean? = null) {
    val ink = ink
    val fill by animateFloatAsState(filled ?: 1f, spring(Spring.DampingRatioNoBouncy, Spring.StiffnessLow), label = "fill")
    val shown by animateFloatAsState(if (filled != null || busy) 1f else 0f, tween(500), label = "shown")
    val shuttle = if (busy) {
        rememberInfiniteTransition(label = "shuttle")
            .animateFloat(0f, 1f, infiniteRepeatable(tween(900, easing = Motion.Pen), RepeatMode.Reverse), label = "carriage")
    } else {
        null
    }
    val from: () -> Float = if (shuttle != null) ({ shuttle.value * 0.78f }) else ({ 0f })
    val to: () -> Float = if (shuttle != null) ({ shuttle.value * 0.78f + 0.22f }) else ({ fill })
    val head = filled != null && !busy
    val pen = ink.live
    Row(modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        DimensionHalf(
            start = true, color, pen, { shown }, head,
            { (from() * 2).coerceIn(0f, 1f) }, { (to() * 2).coerceIn(0f, 1f) },
            Modifier.weight(1f),
        )
        Row(Modifier.padding(horizontal = 8.dp), verticalAlignment = Alignment.CenterVertically) {
            if (file != null) {
                FileMark(file)
                Spacer(Modifier.width(6.dp))
            }
            T(label, Type.small, color, maxLines = 1)
        }
        DimensionHalf(
            start = false, color, pen, { shown }, head,
            { (from() * 2 - 1).coerceIn(0f, 1f) }, { (to() * 2 - 1).coerceIn(0f, 1f) },
            Modifier.weight(1f),
        )
    }
}

@Composable
private fun DimensionHalf(
    start: Boolean,
    color: Color,
    pen: Color,
    shown: () -> Float,
    head: Boolean,
    from: () -> Float,
    to: () -> Float,
    modifier: Modifier,
) {
    Canvas(modifier.height(12.dp)) {
        val s = 1.dp.toPx()
        val y = size.height / 2
        val a = 5.dp.toPx()
        val end = if (start) 0f else size.width
        val dir = if (start) 1f else -1f
        drawLine(color, Offset(end, 0f), Offset(end, size.height), s)
        drawLine(color, Offset(0f, y), Offset(size.width, y), s)
        drawLine(color, Offset(end, y), Offset(end + dir * a, y - a * 0.45f), s)
        drawLine(color, Offset(end, y), Offset(end + dir * a, y + a * 0.45f), s)
        val alpha = shown()
        if (alpha <= 0f) return@Canvas
        val f0 = from()
        val f1 = to()
        if (f1 > f0) drawLine(pen.copy(alpha = alpha), Offset(size.width * f0, y), Offset(size.width * f1, y), 3 * s)
        if (head && f1 > 0f && f1 < 1f) drawCircle(pen.copy(alpha = alpha), 3 * s, Offset(size.width * f1, y))
    }
}

@Composable
fun Spec(label: String, value: String, note: String? = null, noteColor: Color = ink.faint, valueColor: Color = ink.text) {
    Row(Modifier.fillMaxWidth().padding(vertical = 7.dp)) {
        Label(label, ink.dim, Modifier.width(84.dp).padding(top = 3.dp))
        Column(Modifier.weight(1f)) {
            T(value, Type.value, valueColor)
            if (!note.isNullOrEmpty()) T(note, Type.small, noteColor)
        }
    }
}

@Composable
fun TitleBlock(cells: List<Pair<String, String>>, modifier: Modifier = Modifier, plot: Any = "title", after: Int = 0) {
    val ink = ink
    val t = rememberPlot(plot, after)
    Row(modifier.fillMaxWidth().height(IntrinsicSize.Min).drafted(ink.line, marks = false, drawn = { t.value })) {
        cells.forEachIndexed { i, (k, v) ->
            if (i > 0) {
                Box(
                    Modifier.fillMaxHeight().width(1.dp).drawBehind {
                        drawRect(ink.line, size = Size(size.width, size.height * t.value))
                    },
                )
            }
            Column(
                Modifier.weight(if (i == 0) 1.3f else 1f).reveal { t.value }.padding(horizontal = 8.dp, vertical = 6.dp),
                verticalArrangement = Arrangement.spacedBy(2.dp),
            ) {
                Label(k, ink.faint)
                T(v, Type.small, ink.text, maxLines = 1)
            }
        }
    }
}

@Composable
fun Masthead(sheet: String, modifier: Modifier = Modifier, back: (() -> Unit)? = null, action: String? = null, onAction: () -> Unit = {}) {
    val ink = ink
    val t = rememberPlot("masthead:$sheet")
    Column(modifier.fillMaxWidth()) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            if (back != null) {
                Box(Modifier.press(give = 0.9f, onClick = back).padding(end = 12.dp, top = 6.dp, bottom = 6.dp)) { T("←", Type.title, ink.line) }
            }
            Mark()
            Spacer(Modifier.width(10.dp))
            T("CLOCHE", Type.mark, ink.text, Modifier.weight(1f))
            if (action != null) {
                Box(Modifier.press(give = 0.94f, onClick = onAction).padding(vertical = 8.dp, horizontal = 4.dp)) { Label(action, ink.line) }
            }
        }
        Spacer(Modifier.height(2.dp))
        Label(sheet, ink.faint)
        Spacer(Modifier.height(10.dp))
        Box(Modifier.fillMaxWidth().height(2.dp).drawBehind { drawRect(ink.line, size = Size(size.width * t.value, size.height)) })
        Spacer(Modifier.height(2.dp))
        Box(
            Modifier.fillMaxWidth().height(1.dp).drawBehind {
                val u = ((t.value - 0.15f) / 0.85f).coerceIn(0f, 1f)
                drawRect(ink.line.copy(alpha = 0.6f), size = Size(size.width * u, size.height))
            },
        )
    }
}
