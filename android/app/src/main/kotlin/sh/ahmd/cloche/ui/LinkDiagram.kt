package sh.ahmd.cloche.ui

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.clipRect
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.text.TextMeasurer
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import sh.ahmd.cloche.link.Link

const val TRAVEL = 620

@Composable
fun LinkDiagram(link: Link, host: String?, modifier: Modifier = Modifier, pulse: Long? = null, draw: Int = 380, after: Int = 0) {
    val ink = ink
    val haptics = haptics()
    val plotted = LocalPlotted.current
    val measurer = rememberTextMeasurer()
    val live = link is Link.Live
    val down = link is Link.Down || link is Link.NoTailnet
    val first = remember { plotted.first("link") }
    val reach = remember { Animatable(if (live && !first) 1f else 0f) }
    val strike = remember { Animatable(if (down && !first) 1f else 0f) }
    val flash = remember { Animatable(0f) }
    val packet = remember { Animatable(-1f) }
    var wasDown by remember { mutableStateOf(down) }
    var seen by remember { mutableStateOf(pulse ?: Long.MIN_VALUE) }

    LaunchedEffect(down) {
        if (!down) return@LaunchedEffect strike.snapTo(0f)
        wasDown = true
        if (strike.value < 1f) strike.animateTo(1f, tween(440, easing = Motion.Pen))
    }
    LaunchedEffect(live) {
        if (!live) return@LaunchedEffect reach.snapTo(0f)
        if (reach.value >= 1f) return@LaunchedEffect
        if (first) delay(after.toLong())
        if (wasDown) haptics.linkUp()
        wasDown = false
        reach.animateTo(1f, tween(draw, easing = Motion.Pen))
        flash.snapTo(1f)
        flash.animateTo(0f, tween(700))
    }
    LaunchedEffect(pulse) {
        if (pulse == null || pulse <= seen) return@LaunchedEffect
        seen = pulse
        packet.snapTo(0f)
        packet.animateTo(1f, tween(TRAVEL, easing = Motion.Pen))
        haptics.land()
        packet.snapTo(-1f)
        flash.snapTo(1f)
        flash.animateTo(0f, tween(700))
    }

    val motion = if (live || link is Link.Connecting) {
        rememberInfiniteTransition(label = "link")
            .animateFloat(0f, 1f, infiniteRepeatable(tween(if (live) 1500 else 2400, easing = LinearEasing)), label = "flow")
    } else {
        null
    }

    Canvas(modifier.fillMaxWidth().height(96.dp)) {
        val s = 1.dp.toPx()
        val cy = 36.dp.toPx()
        val glow = flash.value

        val pw = 24.dp.toPx()
        val ph = 42.dp.toPx()
        val px = 6.dp.toPx()
        val phone = lerp(ink.line, ink.live, glow)
        drawRoundRect(phone, Offset(px, cy - ph / 2), Size(pw, ph), CornerRadius(4.dp.toPx()), style = Stroke((1.5f + glow) * s))
        drawLine(phone, Offset(px + pw * 0.38f, cy - ph / 2 + 4.dp.toPx()), Offset(px + pw * 0.62f, cy - ph / 2 + 4.dp.toPx()), s)

        val mw = 52.dp.toPx()
        val mh = 33.dp.toPx()
        val mx = size.width - 14.dp.toPx() - mw
        val top = cy - mh / 2 - 3.dp.toPx()
        drawRect(ink.line, Offset(mx, top), Size(mw, mh), style = Stroke(1.5f * s))
        drawRect(ink.line.copy(alpha = 0.35f), Offset(mx + 3.dp.toPx(), top + 3.dp.toPx()), Size(mw - 6.dp.toPx(), mh - 6.dp.toPx()), style = Stroke(s))
        val base = top + mh + 2.5f.dp.toPx()
        drawLine(ink.line, Offset(mx - 8.dp.toPx(), base), Offset(mx + mw + 8.dp.toPx(), base), 1.5f * s)

        val x0 = px + pw + 12.dp.toPx()
        val x1 = mx - 18.dp.toPx()
        val tick = 6.dp.toPx()
        drawLine(ink.faint, Offset(x0, cy - tick), Offset(x0, cy + tick), s)
        drawLine(ink.faint, Offset(x1, cy - tick), Offset(x1, cy + tick), s)
        val mid = (x0 + x1) / 2
        val phase = motion?.value ?: 0f
        when {
            live -> {
                val x = x0 + (x1 - x0) * reach.value
                drawLine(ink.line.copy(alpha = 0.3f), Offset(x0, cy), Offset(x, cy), s)
                val period = 14.dp.toPx()
                val flow = PathEffect.dashPathEffect(floatArrayOf(5.dp.toPx(), period - 5.dp.toPx()), (1f - phase) * period)
                clipRect(left = x0, right = x) { drawLine(ink.live, Offset(x1, cy), Offset(x0, cy), 2 * s, pathEffect = flow) }
                if (reach.value < 1f) drawCircle(ink.live, 2.5f * s, Offset(x, cy))
            }
            link is Link.Connecting -> {
                val period = 8.dp.toPx()
                val reaching = PathEffect.dashPathEffect(floatArrayOf(4.dp.toPx(), 4.dp.toPx()), (1f - phase) * period)
                drawLine(ink.dim, Offset(x0, cy), Offset(x1, cy), s, pathEffect = reaching)
            }
            link is Link.Unpaired -> {
                drawLine(ink.faint, Offset(x0, cy), Offset(x1, cy), s, pathEffect = PathEffect.dashPathEffect(floatArrayOf(2.dp.toPx(), 6.dp.toPx())))
                label(measurer, "?", Offset(mid, cy - 9.dp.toPx()), Type.value.copy(color = ink.dim))
            }
            else -> {
                drawLine(ink.faint, Offset(x0, cy), Offset(x1, cy), s, pathEffect = PathEffect.dashPathEffect(floatArrayOf(4.dp.toPx(), 4.dp.toPx())))
                val r = 5.dp.toPx()
                val a = (strike.value * 2).coerceIn(0f, 1f)
                val b = (strike.value * 2 - 1).coerceIn(0f, 1f)
                if (a > 0f) drawLine(ink.redline, Offset(mid - r, cy - r), Offset(mid - r + 2 * r * a, cy - r + 2 * r * a), 2 * s, StrokeCap.Round)
                if (b > 0f) drawLine(ink.redline, Offset(mid + r, cy - r), Offset(mid + r - 2 * r * b, cy - r + 2 * r * b), 2 * s, StrokeCap.Round)
            }
        }

        val p = packet.value
        if (p >= 0f) {
            val x = x1 - (x1 - x0) * p
            val len = 7.dp.toPx()
            drawLine(ink.live.copy(alpha = 0.25f), Offset(x - len, cy), Offset(x + len, cy), 7 * s, StrokeCap.Round)
            drawLine(ink.text, Offset(x - len / 2, cy), Offset(x + len / 2, cy), 3 * s, StrokeCap.Round)
        }

        val lettering = Type.label.copy(color = ink.dim)
        label(measurer, "PHONE", Offset(px + pw / 2, cy + ph / 2 + 8.dp.toPx()), lettering)
        label(measurer, (host ?: "computer").uppercase(), Offset(mx + mw / 2, base + 8.dp.toPx()), lettering)
        if (link !is Link.Unpaired) label(measurer, "TAILNET", Offset(mid, cy - 20.dp.toPx()), Type.label.copy(color = ink.faint))
    }
}

private fun DrawScope.label(measurer: TextMeasurer, text: String, at: Offset, style: TextStyle) {
    val r = measurer.measure(text, style)
    val x = (at.x - r.size.width / 2).coerceIn(0f, (size.width - r.size.width).coerceAtLeast(0f))
    drawText(r, topLeft = Offset(x, at.y))
}
