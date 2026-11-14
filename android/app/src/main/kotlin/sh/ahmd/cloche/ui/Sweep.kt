package sh.ahmd.cloche.ui

import androidx.compose.animation.core.animate
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.foundation.gestures.draggable
import androidx.compose.foundation.gestures.rememberDraggableState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import kotlin.math.abs
import kotlin.math.sign

private const val PAST = 0.36f

@Composable
fun Sweep(enabled: Boolean, onSwept: () -> Unit, modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    val ink = ink
    val haptics = haptics()
    var x by remember { mutableFloatStateOf(0f) }
    var width by remember { mutableFloatStateOf(1f) }
    var past by remember { mutableStateOf(false) }
    val right by remember { derivedStateOf { x > 0f } }
    val drag = rememberDraggableState { d ->
        x += d
        val now = abs(x) > width * PAST
        if (now != past) {
            past = now
            if (now) haptics.tick() else haptics.patter()
        }
    }
    Box(
        modifier
            .onSizeChanged { width = it.width.toFloat().coerceAtLeast(1f) }
            .draggable(
                drag, Orientation.Horizontal, enabled,
                onDragStopped = { v ->
                    val away = past || (abs(v) > 2400f && sign(v) == sign(x))
                    if (away) {
                        haptics.slide(200)
                        animate(x, sign(x) * width * 1.15f, v, tween(200, easing = Motion.Sheet)) { value, _ -> x = value }
                        onSwept()
                        delay(800)
                        animate(x, 0f, 0f, spring(0.75f, 400f)) { value, _ -> x = value }
                    } else {
                        animate(x, 0f, v, spring(0.75f, 600f)) { value, _ -> x = value }
                    }
                    past = false
                },
            ),
    ) {
        Row(
            Modifier.matchParentSize().graphicsLayer {
                val off = ((abs(x) - width * 0.8f) / (width * 0.3f)).coerceIn(0f, 1f)
                alpha = (abs(x) / (width * PAST)).coerceIn(0f, 1f) * (1f - off)
            }.padding(horizontal = 14.dp),
            horizontalArrangement = if (right) Arrangement.Start else Arrangement.End,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Label(if (past) "Let go" else "Clear", ink.redline)
        }
        Box(
            Modifier.graphicsLayer {
                translationX = x
                rotationZ = x / width * 1.5f
                alpha = 1f - (abs(x) / width * 0.7f).coerceIn(0f, 0.7f)
            },
        ) { content() }
    }
}
