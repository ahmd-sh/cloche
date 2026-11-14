package sh.ahmd.cloche.ui

import android.os.Build
import android.os.VibrationAttributes
import android.os.VibrationEffect
import android.os.VibrationEffect.Composition.PRIMITIVE_CLICK
import android.os.VibrationEffect.Composition.PRIMITIVE_LOW_TICK
import android.os.VibrationEffect.Composition.PRIMITIVE_QUICK_RISE
import android.os.VibrationEffect.Composition.PRIMITIVE_SLOW_RISE
import android.os.VibrationEffect.Composition.PRIMITIVE_TICK
import android.os.Vibrator
import android.os.VibratorManager
import android.view.HapticFeedbackConstants
import android.view.View
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.platform.LocalView
import kotlin.math.exp
import kotlin.math.pow

class Haptics(private val view: View) {
    private val vibrator: Vibrator? =
        view.context.getSystemService(VibratorManager::class.java)?.defaultVibrator?.takeIf { it.hasVibrator() }

    val rise: Int = vibrator
        ?.takeIf { it.areAllPrimitivesSupported(PRIMITIVE_SLOW_RISE) }
        ?.getPrimitiveDurations(PRIMITIVE_SLOW_RISE)?.firstOrNull()
        ?.takeIf { it > 0 }?.coerceIn(260, 720)
        ?: 420

    fun tick() = play(HapticFeedbackConstants.SEGMENT_TICK)

    fun patter() = play(HapticFeedbackConstants.SEGMENT_FREQUENT_TICK)

    fun key() = play(HapticFeedbackConstants.KEYBOARD_TAP)

    fun confirm() = play(HapticFeedbackConstants.CONFIRM)

    fun reject() = play(HapticFeedbackConstants.REJECT)

    fun group() = compose(HapticFeedbackConstants.CLOCK_TICK, P(PRIMITIVE_CLICK, 0.45f))

    fun ready() = compose(HapticFeedbackConstants.CONFIRM, P(PRIMITIVE_QUICK_RISE, 0.35f), P(PRIMITIVE_CLICK, 0.7f))

    fun arm() = compose(HapticFeedbackConstants.LONG_PRESS, P(PRIMITIVE_LOW_TICK, 0.6f), P(PRIMITIVE_LOW_TICK, 1f, 70))

    fun lock() = compose(
        HapticFeedbackConstants.CONFIRM,
        P(PRIMITIVE_SLOW_RISE, 0.5f), P(PRIMITIVE_CLICK, 1f), P(PRIMITIVE_TICK, 0.4f, 90),
    )

    fun land() = compose(HapticFeedbackConstants.CLOCK_TICK, P(PRIMITIVE_CLICK, 0.8f), P(PRIMITIVE_LOW_TICK, 0.5f, 60))

    fun installed() = compose(HapticFeedbackConstants.CONFIRM, P(PRIMITIVE_TICK, 0.5f), P(PRIMITIVE_CLICK, 1f, 60))

    fun lift() = compose(HapticFeedbackConstants.CLOCK_TICK, P(PRIMITIVE_QUICK_RISE, 0.3f), P(PRIMITIVE_TICK, 0.9f, 20))

    fun clink() = compose(HapticFeedbackConstants.CLOCK_TICK, P(PRIMITIVE_CLICK, 0.5f), P(PRIMITIVE_LOW_TICK, 0.3f, 40))

    fun linkUp() = compose(
        HapticFeedbackConstants.SEGMENT_TICK,
        P(PRIMITIVE_TICK, 0.25f), P(PRIMITIVE_TICK, 0.45f, 90), P(PRIMITIVE_TICK, 0.7f, 90),
    )

    fun slide(millis: Long) {
        val v = vibrator ?: return
        if (millis <= 0) return
        val effect = envelope(v, millis) ?: ticks(v, millis) ?: return play(HapticFeedbackConstants.SEGMENT_TICK)
        v.vibrate(effect, TOUCH)
    }

    private fun envelope(v: Vibrator, millis: Long): VibrationEffect? {
        if (Build.VERSION.SDK_INT < 36 || !v.areEnvelopeEffectsSupported()) return null
        val info = v.envelopeEffectInfo
        val step = maxOf(info.minControlPointDurationMillis, millis / SLIDE_POINTS)
        val points = (millis / step).toInt().coerceIn(2, info.maxSize - 1)
        val e = VibrationEffect.BasicEnvelopeBuilder().setInitialSharpness(SLIDE_SHARPNESS)
        for (i in 1 until points) e.addControlPoint(SLIDE_PEAK * swell(i.toFloat() / points), SLIDE_SHARPNESS, step)
        return e.addControlPoint(0f, SLIDE_SHARPNESS, step).build()
    }

    private fun ticks(v: Vibrator, millis: Long): VibrationEffect? {
        if (!v.areAllPrimitivesSupported(PRIMITIVE_LOW_TICK)) return null
        val c = VibrationEffect.startComposition()
        var at = 0L
        for (u in floatArrayOf(0.08f, 0.22f, 0.42f, 0.68f)) {
            val t = (millis * u).toLong()
            c.addPrimitive(PRIMITIVE_LOW_TICK, swell(u) * 0.6f, (t - at).toInt())
            at = t
        }
        return c.compose()
    }

    private fun swell(u: Float): Float {
        val x = u / SWELL_PEAK
        return (x * exp(1f - x)).toDouble().pow(0.7).toFloat()
    }

    private class P(val id: Int, val scale: Float, val delay: Int = 0)

    private fun compose(fallback: Int, vararg steps: P) {
        val v = vibrator
        if (v == null || !v.areAllPrimitivesSupported(*IntArray(steps.size) { steps[it].id })) return play(fallback)
        val c = VibrationEffect.startComposition()
        for (s in steps) c.addPrimitive(s.id, s.scale, s.delay)
        v.vibrate(c.compose(), TOUCH)
    }

    private fun play(effect: Int) {
        view.performHapticFeedback(effect)
    }
}

private val TOUCH = VibrationAttributes.createForUsage(VibrationAttributes.USAGE_TOUCH)
private const val SLIDE_POINTS = 12L
private const val SLIDE_PEAK = 0.28f
private const val SLIDE_SHARPNESS = 0.2f
private const val SWELL_PEAK = 0.18f

val LocalHaptics = staticCompositionLocalOf<Haptics?> { null }

@Composable
fun haptics(): Haptics {
    val view = LocalView.current
    return LocalHaptics.current ?: remember(view) { Haptics(view) }
}
