package sh.ahmd.cloche.ui

import androidx.compose.foundation.text.BasicText
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.graphics.luminance
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.runtime.getValue
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.padding
import sh.ahmd.cloche.R

@Immutable
data class Ink(
    val paper: Color,
    val grid: Color,
    val gridMajor: Color,
    val line: Color,
    val text: Color,
    val dim: Color,
    val faint: Color,
    val redline: Color,
    val live: Color,
)

val Blueprint = Ink(
    paper = Color(0xFF0F3D6E),
    grid = Color(0x12FFFFFF),
    gridMajor = Color(0x26FFFFFF),
    line = Color(0xFFDCEBFF),
    text = Color(0xFFF2F7FF),
    dim = Color(0xFFADC6E5),
    faint = Color(0xFF7598C2),
    redline = Color(0xFFFF8F7E),
    live = Color(0xFF9FF0D5),
)

val Offline = Ink(
    paper = Color(0xFF7E2F2B),
    grid = Color(0x14FFFFFF),
    gridMajor = Color(0x29FFFFFF),
    line = Color(0xFFFFE2DC),
    text = Color(0xFFFFF5F2),
    dim = Color(0xFFEDC0B8),
    faint = Color(0xFFC98F86),
    redline = Color(0xFF561613),
    live = Color(0xFF561613),
)

fun lerp(a: Ink, b: Ink, t: Float): Ink = if (t <= 0f) a else if (t >= 1f) b else Ink(
    lerp(a.paper, b.paper, t), lerp(a.grid, b.grid, t), lerp(a.gridMajor, b.gridMajor, t),
    lerp(a.line, b.line, t), lerp(a.text, b.text, t), lerp(a.dim, b.dim, t), lerp(a.faint, b.faint, t),
    lerp(a.redline, b.redline, t), lerp(a.live, b.live, t),
)

fun Ink.solid(c: Color): Boolean = c.luminance() < paper.luminance()

fun Ink.on(c: Color): Color = if (solid(c)) text else paper

val LocalInk = staticCompositionLocalOf { Blueprint }

val ink: Ink
    @Composable @ReadOnlyComposable get() = LocalInk.current

val Plex = FontFamily(
    Font(R.font.plex_mono_regular, FontWeight.Normal),
    Font(R.font.plex_mono_medium, FontWeight.Medium),
    Font(R.font.plex_mono_semibold, FontWeight.SemiBold),
)

object Type {
    val label = TextStyle(fontFamily = Plex, fontWeight = FontWeight.Medium, fontSize = 10.sp, lineHeight = 14.sp, letterSpacing = 1.5.sp)
    val small = TextStyle(fontFamily = Plex, fontSize = 11.sp, lineHeight = 16.sp)
    val body = TextStyle(fontFamily = Plex, fontSize = 13.sp, lineHeight = 19.sp)
    val value = TextStyle(fontFamily = Plex, fontWeight = FontWeight.Medium, fontSize = 14.sp, lineHeight = 20.sp)
    val title = TextStyle(fontFamily = Plex, fontWeight = FontWeight.SemiBold, fontSize = 20.sp, lineHeight = 26.sp)
    val mark = TextStyle(fontFamily = Plex, fontWeight = FontWeight.SemiBold, fontSize = 15.sp, lineHeight = 20.sp, letterSpacing = 5.sp)
    val button = TextStyle(fontFamily = Plex, fontWeight = FontWeight.SemiBold, fontSize = 12.sp, lineHeight = 16.sp, letterSpacing = 2.5.sp)
    val field = TextStyle(fontFamily = Plex, fontWeight = FontWeight.Medium, fontSize = 17.sp, lineHeight = 24.sp, letterSpacing = 1.sp)
}

@Composable
fun ClocheTheme(offline: Boolean = false, content: @Composable () -> Unit) {
    val t by animateFloatAsState(if (offline) 1f else 0f, tween(900, easing = Motion.Sheet), label = "sheet")
    CompositionLocalProvider(LocalInk provides lerp(Blueprint, Offline, t), content = content)
}

@Composable
fun T(
    text: String,
    style: TextStyle = Type.body,
    color: Color = ink.text,
    modifier: Modifier = Modifier,
    maxLines: Int = Int.MAX_VALUE,
    align: TextAlign = TextAlign.Unspecified,
) {
    val ink = ink
    if (ink.solid(color)) {
        BasicText(
            text, modifier.background(color).padding(horizontal = 4.dp),
            style.copy(color = ink.text, textAlign = align), overflow = TextOverflow.Ellipsis, maxLines = maxLines,
        )
    } else {
        BasicText(text, modifier, style.copy(color = color, textAlign = align), overflow = TextOverflow.Ellipsis, maxLines = maxLines)
    }
}

@Composable
fun Label(text: String, color: Color = ink.dim, modifier: Modifier = Modifier) {
    T(text.uppercase(), Type.label, color, modifier, maxLines = 1)
}
