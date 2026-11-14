package sh.ahmd.cloche

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import sh.ahmd.cloche.deliver.Policy
import sh.ahmd.cloche.link.FileInfo
import sh.ahmd.cloche.link.Manifest
import sh.ahmd.cloche.store.Apk
import sh.ahmd.cloche.store.Delivery
import sh.ahmd.cloche.store.Paired
import sh.ahmd.cloche.store.Pin
import sh.ahmd.cloche.store.Saved
import sh.ahmd.cloche.store.Stage
import sh.ahmd.cloche.store.Store
import java.io.File

class PolicyTest {
    private val apk = Apk(
        pkg = "sh.ahmd.wallflower", label = "wallflower", versionCode = 5, versionName = "0.1.0",
        signer = "aa".repeat(32), minSdk = 34, targetSdk = 37,
    )

    @Test
    fun aNewAppNeedsALook() {
        val v = Policy.check(apk, pin = null, sdk = 37)
        assertTrue(v.ok)
        assertTrue(v.fresh)
    }

    @Test
    fun anUpdateSignedAsInstalledGoesOnATap() {
        val v = Policy.check(apk.copy(installedCode = 5, installedSameSigner = true), pin = null, sdk = 37)
        assertTrue(v.ok)
        assertFalse(v.fresh)
    }

    @Test
    fun aPinnedAppNeedsNoLookOnceUninstalled() {
        val v = Policy.check(apk, Pin(apk.pkg, apk.signer, 0), sdk = 37)
        assertTrue(v.ok)
        assertFalse(v.fresh)
    }

    @Test
    fun refusesAnotherSigner() {
        assertFalse(Policy.check(apk.copy(installedCode = 5, installedSameSigner = false), null, 37).ok)
        val v = Policy.check(apk.copy(signer = "bb".repeat(32)), Pin(apk.pkg, apk.signer, 0), 37)
        assertFalse(v.ok)
        assertTrue(v.reason, v.reason.startsWith("signed with a different key"))
    }

    @Test
    fun refusesADowngradeAndTooNewAnAndroid() {
        assertFalse(Policy.check(apk.copy(installedCode = 6, installedSameSigner = true), null, 37).ok)
        assertFalse(Policy.check(apk.copy(minSdk = 38), null, 37).ok)
    }

    @Test
    fun paired() {
        assertTrue(Paired("m", "127.0.0.1:41778", "", "", "", 0).loopback)
        assertFalse(Paired("m", "100.101.102.103:41778", "", "", "", 0).loopback)
    }

    @get:Rule
    val tmp = TemporaryFolder()

    @Test
    fun storeKeepsWhatItsGiven() {
        val f = File(tmp.root, "state.json")
        val s = Store(f)
        val m = Manifest(1, "abc", 3, "mac", "2026-11-04T18:00:00Z", FileInfo("a.apk", 10, "00"))
        s.update { it.copy(cursor = 3, deliveries = listOf(Delivery("abc", 3, m, Stage.Fetching, received = 1, changed = 1))) }
        s.change("abc") { it.copy(stage = Stage.Ready, apk = apk) }
        s.flush()
        val again = Store(f).value
        assertEquals(3L, again.cursor)
        assertEquals(Stage.Ready, again.delivery("abc")?.stage)
        assertEquals("wallflower", again.delivery("abc")?.label)
    }

    @Test
    fun buildsWaitForWiFiOnlyWhenTheyShould() {
        val m = Manifest(1, "abc", 3, "mac", "2026-11-04T18:00:00Z", FileInfo("a.apk", 10, "00"))
        val d = Delivery("abc", 3, m, Stage.Fetching, received = 1, changed = 1)
        val waits = Saved(mobileData = false)
        assertTrue(waits.holds(d, mobile = true))
        assertFalse(waits.holds(d, mobile = false))
        assertFalse(waits.holds(d.copy(overMobile = true), mobile = true))
        assertFalse(waits.holds(d.copy(stage = Stage.Ready), mobile = true))
        assertFalse(Saved().holds(d, mobile = true))
    }

    @Test
    fun storeStartsOverFromAnUnreadableFile() {
        val f = File(tmp.root, "state.json").apply { writeText("{not json") }
        assertEquals(0L, Store(f).value.cursor)
    }
}
