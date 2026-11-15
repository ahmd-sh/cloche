package sh.ahmd.cloche.ui

import androidx.activity.compose.PredictiveBackHandler
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.ContentTransform
import androidx.compose.animation.core.SeekableTransitionState
import androidx.compose.animation.core.rememberTransition
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleOut
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.launch
import sh.ahmd.cloche.Cloche
import sh.ahmd.cloche.link.Api
import sh.ahmd.cloche.link.Link
import sh.ahmd.cloche.link.LinkService
import sh.ahmd.cloche.link.PairTicket
import sh.ahmd.cloche.link.Pairing
import sh.ahmd.cloche.store.Delivery
import sh.ahmd.cloche.store.Stage

sealed interface Opened {
    data class Delivery(val id: String, val install: Boolean, val download: Boolean = false) : Opened
    data class Pair(val ticket: PairTicket) : Opened
}

interface Host {
    fun install(id: String)
    fun launch(pkg: String)
    fun showFiles()
    fun fix(what: String)
    fun paired()
    fun unpair(id: String)
    fun checks(): Checks
}

private const val FIRST = "pair:first"

private fun depth(sheet: String) = when {
    sheet == FIRST -> -1
    sheet == "home" -> 0
    sheet == "pair" -> 2
    else -> 1
}

private const val FRESH = 8_000L

@Composable
fun ClocheApp(app: Cloche, opened: Flow<Opened>, host: Host, resumes: Int) {
    val view = LocalView.current
    val haptics = remember(view) { Haptics(view) }
    val plotted = remember { Plotted() }
    val offline by remember {
        combine(app.tailnet.vpn, app.store.state) { vpn, s -> vpn == null && (s.computers.isEmpty() || s.computers.any { !it.loopback }) }
            .distinctUntilChanged()
    }.collectAsStateWithLifecycle(app.tailnet.vpn.value == null)
    ClocheTheme(offline) {
        CompositionLocalProvider(LocalHaptics provides haptics, LocalPlotted provides plotted) {
            Sheets(app, opened, host, resumes, haptics)
        }
    }
}

@Composable
private fun Sheets(app: Cloche, opened: Flow<Opened>, host: Host, resumes: Int, haptics: Haptics) {
    val ink = ink
    val state by app.store.state.collectAsStateWithLifecycle()
    val links by app.links.collectAsStateWithLifecycle()
    val progress by app.pipeline.progress.collectAsStateWithLifecycle()
    val vpn by app.tailnet.vpn.collectAsStateWithLifecycle()
    val mobile by app.tailnet.mobile.collectAsStateWithLifecycle()
    val fast = links.values.any { it is Link.Down } || state.deliveries.any { System.currentTimeMillis() - it.received < 90_000 }
    val now = rememberNow(if (fast) 1000 else 30_000)
    val scope = rememberCoroutineScope()
    val checks = remember(resumes, vpn, state.computers.size) { host.checks() }
    val kept by app.pipeline.kept.collectAsStateWithLifecycle()
    val list = rememberLazyListState()

    var screen by rememberSaveable { mutableStateOf("home") }
    var ticket by remember { mutableStateOf<PairTicket?>(null) }
    var pairing by remember { mutableStateOf(false) }
    var pairError by remember { mutableStateOf<String?>(null) }
    var pairErrors by remember { mutableIntStateOf(0) }
    var firstPairing by remember { mutableStateOf(false) }
    var celebrating by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(opened) {
        opened.collect { o ->
            when (o) {
                is Opened.Delivery -> {
                    screen = "build:${o.id}"
                    val d = app.store.value.delivery(o.id)
                    val asking = d?.stage == Stage.Installing && d.asking
                    if (o.install && d != null && ((d.stage == Stage.Ready && !app.store.value.reviews(d)) || asking)) host.install(o.id)
                    if (o.download && d != null && d.stage == Stage.Fetching) app.pipeline.downloadNow(o.id)
                }
                is Opened.Pair -> {
                    ticket = o.ticket
                    pairError = null
                    screen = "pair"
                }
            }
        }
    }

    fun pair(t: PairTicket, name: String, replace: String?) = scope.launch {
        firstPairing = state.computers.isEmpty()
        pairing = true
        pairError = null
        try {
            celebrating = Pairing.pair(app, t, name, replace).fingerprint
            haptics.lock()
            delay(haptics.rise + 900L)
            firstPairing = false
            screen = "home"
            host.paired()
            delay(Motion.SHEET + 100L)
            ticket = null
        } catch (e: CancellationException) {
            throw e
        } catch (e: Api.Refused) {
            pairError = e.message
            pairErrors++
            haptics.reject()
        } catch (e: Exception) {
            pairError = LinkService.reason(e)
            pairErrors++
            haptics.reject()
        } finally {
            pairing = false
            firstPairing = false
            celebrating = null
        }
    }

    val key = if (state.computers.isEmpty() || firstPairing) FIRST else screen
    Feel(state.deliveries, haptics, key)
    fun clear(ids: List<String>) {
        app.pipeline.clear(ids)
    }
    val nav = remember { SeekableTransitionState(key) }
    val transition = rememberTransition(nav, label = "sheets")
    LaunchedEffect(key) {
        if (nav.currentState != key || nav.targetState != key) {
            haptics.slide(Motion.SHEET.toLong())
            nav.animateTo(key)
        }
    }
    PredictiveBackHandler(enabled = key != "home" && key != FIRST) { events ->
        val from = key
        try {
            events.collect { e -> nav.seekTo(e.progress, targetState = "home") }
            screen = "home"
            if (from == "pair") scope.launch {
                delay(Motion.SHEET.toLong())
                ticket = null
            }
        } catch (e: CancellationException) {
            scope.launch { nav.animateTo(from) }
            throw e
        }
    }

    val padding = PaddingValues(horizontal = 20.dp, vertical = 16.dp)
    val back = { screen = "home" }
    Box(Modifier.fillMaxSize().background(ink.paper)) {
        transition.AnimatedContent(
            Modifier.fillMaxSize(),
            transitionSpec = { sheets(depth(targetState) - depth(initialState)) },
        ) { k ->
            Box(Modifier.fillMaxSize().sheetEdge().gridPaper(ink).windowInsetsPadding(WindowInsets.safeDrawing)) {
                when {
                    k == FIRST || k == "pair" -> PairScreen(
                        ticket, state.unpaired, pairing, celebrating, pairError, pairErrors, vpn != null,
                        computers = state.computers,
                        padding = padding, onPair = ::pair,
                        onBack = if (k == FIRST) null else back,
                    )
                    k.startsWith("build:") -> {
                        val id = k.removePrefix("build:")
                        Detail(
                            state.delivery(id)?.takeUnless { it.cleared }, state, id in kept.apks, mobile, progress[id], now, padding, back, host::install, host::launch,
                            onClear = { clear(listOf(it)); screen = "home" },
                            onDownload = app.pipeline::downloadNow,
                            onShowFiles = host::showFiles,
                            onDeleteFile = app.pipeline::deleteFile,
                        )
                    }
                    k == "settings" -> {
                        SettingsScreen(
                            state, links, checks, kept, padding, now,
                            onBack = back,
                            onFix = host::fix,
                            onPairAnother = {
                                ticket = null
                                pairError = null
                                screen = "pair"
                            },
                            onUnpair = { id -> host.unpair(id) },
                            onRename = { id, name -> app.store.update { s -> s.change(id) { it.copy(nickname = name.trim().take(40).takeUnless { n -> n == it.offered.ifEmpty { it.host } }.orEmpty()) } } },
                            onReview = { id, on -> app.store.update { s -> s.change(id) { it.copy(review = on) } } },
                            onForget = { pkg -> app.store.update { st -> st.copy(pins = st.pins.filter { it.pkg != pkg }) } },
                            onClearAfter = { days ->
                                app.store.update { it.copy(clearAfter = days) }
                                app.scope.launch { app.pipeline.tidy() }
                            },
                            onKeepInstalled = { keep ->
                                app.store.update { it.copy(keepInstalled = keep) }
                                if (!keep) app.pipeline.dropInstalled()
                            },
                            onMobileData = { on ->
                                app.store.update { it.copy(mobileData = on) }
                                if (on) app.work.trySend(Unit)
                            },
                            onClearAll = { clear(state.shown.filter { it.clearable }.map { it.id }) },
                        )
                    }
                    else -> Home(
                        state, links, checks, mobile, kept.apks, progress, now, padding, list,
                        onOpen = { screen = "build:$it" },
                        onInstall = host::install,
                        onLaunch = host::launch,
                        onDownload = app.pipeline::downloadNow,
                        onRetry = { app.kick() },
                        onFix = host::fix,
                        onFold = { app.store.update { it.copy(folded = !it.folded) } },
                        onClear = ::clear,
                        onSettings = { screen = "settings" },
                    )
                }
            }
        }
    }
}

private fun sheets(dir: Int): ContentTransform {
    val slide = tween<IntOffset>(Motion.SHEET, easing = Motion.Sheet)
    val fade = tween<Float>(Motion.SHEET, easing = Motion.Sheet)
    return when {
        dir > 0 -> (slideInHorizontally(slide) { it } togetherWith (slideOutHorizontally(slide) { -it / 4 } + fadeOut(fade, 0.35f)))
            .apply { targetContentZIndex = 1f }
        dir < 0 -> ((slideInHorizontally(slide) { -it / 4 } + fadeIn(fade, 0.35f)) togetherWith (slideOutHorizontally(slide) { it } + scaleOut(fade, 0.94f)))
            .apply { targetContentZIndex = -1f }
        else -> fadeIn(tween(220, 90)) togetherWith fadeOut(tween(90))
    }
}

@Composable
private fun Feel(deliveries: List<Delivery>, haptics: Haptics, sheet: String) {
    val seen = remember { HashMap<String, Stage>() }
    LaunchedEffect(deliveries) {
        val now = System.currentTimeMillis()
        for (d in deliveries) {
            val was = seen.put(d.id, d.stage)
            if (d.cleared || was == null || was == d.stage || now - d.changed > FRESH) continue
            when (d.stage) {
                Stage.Ready -> if (sheet != "home" && sheet != "build:${d.id}") haptics.tick()
                Stage.Installed -> haptics.installed()
                Stage.Blocked, Stage.Failed -> haptics.reject()
                else -> {}
            }
        }
    }
}
