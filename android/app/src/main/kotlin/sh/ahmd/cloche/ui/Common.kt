package sh.ahmd.cloche.ui

import android.graphics.BitmapFactory
import android.text.format.DateUtils
import androidx.compose.animation.animateColorAsState
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import kotlinx.coroutines.delay
import sh.ahmd.cloche.cloche
import sh.ahmd.cloche.store.Delivery
import sh.ahmd.cloche.store.Stage
import java.text.DateFormat
import java.util.Date

@Composable
fun rememberNow(period: Long = 1000): Long {
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val now by produceState(System.currentTimeMillis(), period) {
        lifecycle.repeatOnLifecycle(Lifecycle.State.STARTED) {
            while (true) {
                value = System.currentTimeMillis()
                delay(period - value % period)
            }
        }
    }
    return now
}

fun ago(now: Long, t: Long): String {
    val s = (now - t) / 1000
    return when {
        t <= 0 -> "—"
        s < 5 -> "just now"
        s < 60 -> "${s}s ago"
        s < 3600 -> "${s / 60}m ago"
        s < 86400 -> "${s / 3600}h ago"
        else -> clock(t)
    }
}

fun clock(t: Long): String =
    if (DateUtils.isToday(t)) DateFormat.getTimeInstance(DateFormat.SHORT).format(Date(t))
    else DateFormat.getDateTimeInstance(DateFormat.MEDIUM, DateFormat.SHORT).format(Date(t))

fun mb(bytes: Long) = "%.1f MB".format(bytes / 1048576.0)

@Composable
fun stageWord(d: Delivery, held: Boolean = false): Pair<String, Color> {
    val ink = ink
    return when (d.stage) {
        Stage.Fetching -> (if (held) "Waiting" else "Arriving") to ink.dim
        Stage.Ready -> (if (d.fresh) "New app" else "Ready") to ink.line
        Stage.Blocked -> "Refused" to ink.redline
        Stage.Installing -> (if (d.asking) "Waiting" else "Installing") to ink.line
        Stage.Installed -> "Installed" to ink.live
        Stage.Failed -> "Failed" to ink.redline
        Stage.Declined -> "Declined" to ink.dim
        Stage.Superseded -> "Superseded" to ink.faint
    }
}

@Composable
fun AppIcon(d: Delivery, size: Dp, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val ink = ink
    val bmp = remember(d.id, d.apk) {
        context.cloche.iconFile(d.id).takeIf { it.exists() }?.let { BitmapFactory.decodeFile(it.path)?.asImageBitmap() }
    }
    if (bmp != null) {
        Image(bmp, null, modifier.size(size))
    } else {
        Box(modifier.size(size).drafted(ink.line.copy(alpha = 0.6f), marks = false), contentAlignment = Alignment.Center) {
            T(d.label.take(1).uppercase(), Type.value, ink.line)
        }
    }
}

@Composable
fun Notice(title: String, body: String, color: Color, modifier: Modifier = Modifier, plot: Any? = null, after: Int = 0) {
    val t = if (plot != null) rememberPlot(plot, after) else null
    val c by animateColorAsState(color, Motion.effect(), label = "ink")
    val solid = ink.solid(color)
    Column(modifier.fillMaxWidth().ground(solid, c) { t?.value ?: 1f }.drafted(c, drawn = t?.let { { it.value } }).padding(12.dp)) {
        Column(Modifier.reveal { t?.value ?: 1f }) {
            Label(title, if (solid) ink.text else c)
            Spacer(Modifier.height(4.dp))
            T(body, Type.small, ink.text)
        }
    }
}

@Composable
fun Field(
    label: String,
    value: String,
    onChange: (String) -> Unit,
    placeholder: String,
    keyboard: KeyboardOptions = KeyboardOptions.Default,
    modifier: Modifier = Modifier,
) {
    val ink = ink
    Column(modifier.fillMaxWidth().padding(vertical = 8.dp)) {
        Label(label)
        Spacer(Modifier.height(6.dp))
        BasicTextField(
            value, onChange,
            Modifier.fillMaxWidth(),
            textStyle = Type.field.copy(color = ink.text),
            cursorBrush = SolidColor(ink.line),
            singleLine = true,
            keyboardOptions = keyboard,
            decorationBox = { inner ->
                Box {
                    if (value.isEmpty()) T(placeholder, Type.field, ink.faint)
                    inner()
                }
            },
        )
        Spacer(Modifier.height(4.dp))
        Box(Modifier.fillMaxWidth().height(1.dp).background(ink.line))
    }
}
