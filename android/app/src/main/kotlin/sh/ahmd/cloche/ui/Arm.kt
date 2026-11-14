package sh.ahmd.cloche.ui

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.tween
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.unit.dp

@Stable
class Arm internal constructor(private val haptics: Haptics) {
    var armed by mutableStateOf<String?>(null)
        private set
    internal val burn = Animatable(0f)

    fun tap(key: String, then: () -> Unit) {
        if (armed == key) {
            haptics.confirm()
            armed = null
            then()
        } else {
            armed = key
        }
    }

    fun fuse(key: String): Float = if (armed == key) burn.value else 0f

    internal fun disarm() {
        armed = null
    }
}

@Composable
fun rememberArm(): Arm {
    val haptics = haptics()
    val arm = remember { Arm(haptics) }
    LaunchedEffect(arm.armed) {
        if (arm.armed == null) return@LaunchedEffect arm.burn.snapTo(0f)
        haptics.arm()
        arm.burn.snapTo(1f)
        arm.burn.animateTo(0f, tween(3000, easing = LinearEasing))
        arm.disarm()
    }
    return arm
}

@Composable
fun ArmedWord(arm: Arm, key: String, word: String, modifier: Modifier = Modifier, onConfirm: () -> Unit) {
    val ink = ink
    Box(
        modifier
            .press(give = 0.94f) { arm.tap(key, onConfirm) }
            .drawBehind {
                val f = arm.fuse(key)
                if (f > 0f) drawRect(ink.redline, Offset(0f, size.height - 2.dp.toPx()), Size(size.width * f, 2.dp.toPx()))
            }
            .padding(8.dp),
    ) {
        AnimatedContent(if (arm.armed == key) "Tap again" else word, transitionSpec = { roll() }, label = "armed") { Label(it, ink.redline) }
    }
}
