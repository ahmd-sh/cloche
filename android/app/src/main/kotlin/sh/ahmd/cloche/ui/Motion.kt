package sh.ahmd.cloche.ui

import androidx.compose.animation.AnimatedContentTransitionScope
import androidx.compose.animation.ContentTransform
import androidx.compose.animation.SizeTransform
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.AnimationVector1D
import androidx.compose.animation.core.CubicBezierEasing
import androidx.compose.animation.core.FiniteAnimationSpec
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.layout.Box
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay

object Motion {
    val Sheet = CubicBezierEasing(0.2f, 0f, 0f, 1f)
    val Pen = CubicBezierEasing(0.65f, 0f, 0.35f, 1f)
    const val SHEET = 420
    const val PLOT = 560

    fun <T> effect(): FiniteAnimationSpec<T> = spring(Spring.DampingRatioNoBouncy, Spring.StiffnessMedium)

    fun <T> spatial(): FiniteAnimationSpec<T> = spring(Spring.DampingRatioNoBouncy, Spring.StiffnessMediumLow)

    fun <T> press(down: Boolean): FiniteAnimationSpec<T> =
        if (down) spring(Spring.DampingRatioNoBouncy, Spring.StiffnessHigh) else spring(0.5f, Spring.StiffnessMediumLow)

    fun <T> plot(after: Int = 0): FiniteAnimationSpec<T> = tween(PLOT, after, Pen)
}

fun <S> AnimatedContentTransitionScope<S>.roll(clip: Boolean = false): ContentTransform =
    (slideInVertically(Motion.spatial()) { it / 2 } + fadeIn(Motion.effect())) togetherWith
        (slideOutVertically(Motion.spatial()) { -it / 2 } + fadeOut(Motion.effect())) using SizeTransform(clip)

fun <S> AnimatedContentTransitionScope<S>.swap(): ContentTransform =
    fadeIn(tween(220, 90)) togetherWith fadeOut(tween(90)) using SizeTransform(clip = false)

class Plotted {
    private val done = HashSet<Any>()

    fun first(key: Any): Boolean = done.add(key)
}

val LocalPlotted = staticCompositionLocalOf { Plotted() }

@Composable
fun rememberPlot(key: Any, after: Int = 0): Animatable<Float, AnimationVector1D> {
    val plotted = LocalPlotted.current
    val t = remember(key) { Animatable(if (plotted.first(key)) 0f else 1f) }
    LaunchedEffect(t) {
        if (t.value < 1f) {
            delay(after.toLong())
            t.animateTo(1f, Motion.plot())
        }
    }
    return t
}

fun Modifier.reveal(t: () -> Float): Modifier = graphicsLayer {
    val u = ((t() - 0.4f) / 0.6f).coerceIn(0f, 1f)
    alpha = u
    translationY = (1f - u) * 6.dp.toPx()
}

@Composable
fun Reveal(key: Any, after: Int = 0, modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    val t = rememberPlot(key, after)
    Box(modifier.reveal { t.value }) { content() }
}
