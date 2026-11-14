package sh.ahmd.cloche.ui

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.CubicBezierEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.requiredSize
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.withTransform
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import sh.ahmd.cloche.store.Delivery
import sh.ahmd.cloche.store.Stage

fun DrawScope.cloche(
    color: Color,
    stroke: Float,
    lift: Float = 0f,
    tilt: Float = 0f,
    lid: Float = 1f,
    plate: Float = 1f,
    shine: Float = 0.55f,
    fill: Color? = null,
) {
    val w = size.width
    val h = size.height
    val base = h * 0.84f
    if (plate > 0f) {
        drawLine(color.copy(alpha = color.alpha * plate), Offset(w * 0.02f, base + stroke * 1.4f), Offset(w * 0.98f, base + stroke * 1.4f), stroke, StrokeCap.Round)
        drawLine(color.copy(alpha = color.alpha * plate * 0.55f), Offset(w * 0.16f, base + stroke * 3.4f), Offset(w * 0.84f, base + stroke * 3.4f), stroke * 0.7f, StrokeCap.Round)
    }
    if (lid <= 0f) return
    val c = color.copy(alpha = color.alpha * lid)
    val l = w * 0.1f
    val r = w * 0.9f
    val top = h * 0.24f
    val dome = Path().apply {
        moveTo(l, base)
        cubicTo(l, top + (base - top) * 0.3f, w * 0.27f, top, w / 2, top)
        cubicTo(w * 0.73f, top, r, top + (base - top) * 0.3f, r, base)
    }
    val glint = Path().apply {
        moveTo(w * 0.22f, base - (base - top) * 0.38f)
        cubicTo(w * 0.23f, top + (base - top) * 0.3f, w * 0.29f, top + (base - top) * 0.16f, w * 0.37f, top + (base - top) * 0.1f)
    }
    val knob = top - h * 0.06f
    withTransform({
        translate(0f, -lift * (base - top) * 1.15f)
        rotate(tilt * lift, Offset(w / 2, base))
    }) {
        if (fill != null) drawPath(dome, fill.copy(alpha = fill.alpha * lid))
        drawPath(dome, c, style = Stroke(stroke, cap = StrokeCap.Round, join = StrokeJoin.Round))
        drawLine(c, Offset(l - stroke, base), Offset(r + stroke, base), stroke, StrokeCap.Round)
        drawLine(c, Offset(w / 2, top), Offset(w / 2, knob), stroke)
        drawCircle(c, w * 0.055f, Offset(w / 2, knob - w * 0.04f))
        if (shine > 0f) drawPath(glint, c.copy(alpha = c.alpha * shine), style = Stroke(stroke * 0.8f, cap = StrokeCap.Round))
    }
}

@Composable
fun Mark(modifier: Modifier = Modifier) {
    val ink = ink
    val haptics = haptics()
    val scope = rememberCoroutineScope()
    val lid = remember { Animatable(0f) }
    Canvas(
        modifier.size(22.dp, 20.dp).pointerInput(Unit) {
            detectTapGestures {
                if (lid.isRunning) return@detectTapGestures
                scope.launch {
                    haptics.lift()
                    lid.animateTo(1f, tween(280, easing = Motion.Sheet))
                    delay(520)
                    lid.animateTo(0f, tween(200, easing = CubicBezierEasing(0.5f, 0f, 1f, 1f)))
                    haptics.clink()
                }
            }
        },
    ) {
        val s = 1.5.dp.toPx()
        val peek = lid.value
        if (peek > 0f) {
            val b = size.width * 0.26f
            val o = Offset((size.width - b) / 2, size.height * 0.84f - b)
            val c = ink.live.copy(alpha = peek)
            drawRect(c, o, Size(b, b), style = Stroke(s))
            drawLine(c.copy(alpha = peek * 0.6f), o, o + Offset(b, b), s * 0.7f)
        }
        cloche(ink.text, s, lift = peek * 0.75f, tilt = -16f, fill = ink.paper)
    }
}

@Composable
fun Served(d: Delivery, size: Dp, plotted: () -> Float = { 1f }) {
    val ink = ink
    val haptics = haptics()
    val covered = d.stage == Stage.Fetching
    val lid = remember(d.id) { Animatable(if (covered) 0f else 1f) }
    val glint = if (covered) {
        rememberInfiniteTransition(label = "glint")
            .animateFloat(0.25f, 0.85f, infiniteRepeatable(tween(1100, easing = Motion.Pen), RepeatMode.Reverse), label = "glint")
    } else {
        null
    }
    LaunchedEffect(covered) {
        if (covered || lid.value >= 1f) return@LaunchedEffect
        snapshotFlow { plotted() }.first { it >= 1f }
        delay(140)
        haptics.lift()
        lid.animateTo(1f, tween(720, easing = Motion.Sheet))
    }
    Box(Modifier.size(size), contentAlignment = Alignment.Center) {
        AppIcon(
            d, size,
            Modifier.graphicsLayer {
                val u = ((lid.value - 0.12f) / 0.45f).coerceIn(0f, 1f)
                alpha = u
                scaleX = 0.82f + 0.18f * u
                scaleY = 0.82f + 0.18f * u
            },
        )
        Canvas(Modifier.requiredSize(size * 1.3f, size * 1.2f)) {
            val u = lid.value
            if (u >= 1f) return@Canvas
            val fade = 1f - ((u - 0.35f) / 0.5f).coerceIn(0f, 1f)
            cloche(ink.line, 1.5.dp.toPx(), lift = u, tilt = -12f, lid = fade, plate = 1f - u, shine = glint?.value ?: 0.55f, fill = ink.paper)
        }
    }
}

@Composable
fun FileMark(here: Boolean, modifier: Modifier = Modifier) {
    val ink = ink
    Canvas(modifier.size(9.dp, 11.dp)) {
        val s = 1.dp.toPx()
        val c = size.width * 0.38f
        val page = Path().apply {
            moveTo(s / 2, s / 2)
            lineTo(size.width - c, s / 2)
            lineTo(size.width - s / 2, c)
            lineTo(size.width - s / 2, size.height - s / 2)
            lineTo(s / 2, size.height - s / 2)
            close()
        }
        if (here) {
            drawPath(page, ink.line.copy(alpha = 0.25f))
            drawPath(page, ink.line, style = Stroke(s, join = StrokeJoin.Round))
            drawLine(ink.line, Offset(size.width - c, s / 2), Offset(size.width - c, c), s)
            drawLine(ink.line, Offset(size.width - c, c), Offset(size.width - s / 2, c), s)
        } else {
            drawPath(page, ink.faint, style = Stroke(s, pathEffect = PathEffect.dashPathEffect(floatArrayOf(2.dp.toPx(), 1.5.dp.toPx()))))
        }
    }
}

fun DrawScope.bin(color: Color, stroke: Float) {
    val w = size.width
    val h = size.height
    drawLine(color, Offset(w * 0.08f, h * 0.2f), Offset(w * 0.92f, h * 0.2f), stroke, StrokeCap.Round)
    drawPath(
        Path().apply {
            moveTo(w * 0.36f, h * 0.2f)
            lineTo(w * 0.36f, h * 0.06f)
            lineTo(w * 0.64f, h * 0.06f)
            lineTo(w * 0.64f, h * 0.2f)
        },
        color, style = Stroke(stroke, join = StrokeJoin.Round),
    )
    drawPath(
        Path().apply {
            moveTo(w * 0.18f, h * 0.2f)
            lineTo(w * 0.26f, h * 0.94f)
            lineTo(w * 0.74f, h * 0.94f)
            lineTo(w * 0.82f, h * 0.2f)
        },
        color, style = Stroke(stroke, join = StrokeJoin.Round),
    )
    drawLine(color, Offset(w * 0.42f, h * 0.36f), Offset(w * 0.43f, h * 0.8f), stroke * 0.8f, StrokeCap.Round)
    drawLine(color, Offset(w * 0.58f, h * 0.36f), Offset(w * 0.57f, h * 0.8f), stroke * 0.8f, StrokeCap.Round)
}

@Composable
fun Stamp(text: String, color: Color, thump: Boolean, modifier: Modifier = Modifier) {
    val ink = ink
    val solid = ink.solid(color)
    val down = remember { Animatable(if (thump) 0f else 1f) }
    val spread = remember { Animatable(if (thump) 0f else 1f) }
    LaunchedEffect(Unit) {
        if (down.value >= 1f) return@LaunchedEffect
        down.animateTo(1f, tween(90, easing = CubicBezierEasing(0.55f, 0f, 1f, 0.45f)))
        spread.animateTo(1f, tween(320, easing = Motion.Sheet))
    }
    Box(
        modifier
            .graphicsLayer {
                val u = down.value
                val k = 1f + (1f - u) * 0.8f
                scaleX = k
                scaleY = k
                alpha = (u * 1.6f).coerceAtMost(1f)
                rotationZ = -5f
            }
            .drawBehind {
                val s = 1.5.dp.toPx()
                if (solid) drawRect(color)
                drawRect(color, Offset(s / 2, s / 2), Size(size.width - s, size.height - s), style = Stroke(s))
                val i = 2.5.dp.toPx()
                val inner = if (solid) ink.text.copy(alpha = 0.5f) else color.copy(alpha = color.alpha * 0.8f)
                drawRect(inner, Offset(i, i), Size(size.width - 2 * i, size.height - 2 * i), style = Stroke(0.8.dp.toPx()))
                val p = spread.value
                if (p in 0.001f..0.999f) {
                    val e = p * 7.dp.toPx()
                    drawRect(color.copy(alpha = (1f - p) * 0.5f), Offset(-e, -e), Size(size.width + 2 * e, size.height + 2 * e), style = Stroke(s * (1f - p) + 0.5f))
                }
            }
            .padding(horizontal = 9.dp, vertical = 4.dp),
    ) {
        T(text.uppercase(), Type.label.copy(fontWeight = FontWeight.SemiBold), if (solid) ink.text else color, maxLines = 1)
    }
}
