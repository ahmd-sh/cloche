package sh.ahmd.cloche

import kotlinx.serialization.Serializable
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import sh.ahmd.cloche.link.Inbox
import sh.ahmd.cloche.link.PairTicket
import sh.ahmd.cloche.link.Signed
import sh.ahmd.cloche.link.json
import sh.ahmd.cloche.link.sha256
import java.io.File
import java.security.KeyPairGenerator
import java.security.spec.ECGenParameterSpec
import java.util.Base64

class WireTest {
    @Serializable
    data class Golden(val key: String, val fingerprint: String, val inbox: Inbox)

    private val golden = json.decodeFromString<Golden>(File(System.getProperty("testdata"), "golden.json").readText())
    private val key = Base64.getDecoder().decode(golden.key)
    private val delivery = golden.inbox.deliveries.single()

    @Test
    fun opensWhatTheMacSigned() {
        val m = Signed.open(key, delivery)
        assertNotNull(m)
        m!!
        assertEquals(1, m.v)
        assertEquals(1L, m.seq)
        assertEquals("mac", m.host)
        assertEquals("Sessions: rename inline", m.note)
        assertEquals(10L, m.file.size)
        assertEquals("app-debug.apk", m.file.name)
        assertEquals("app@020c4dd with changes on main", m.source?.described)
        assertTrue(m.pushed > 0)
    }

    @Test
    fun fingerprintAgrees() = assertEquals(golden.fingerprint, Signed.fingerprint(key))

    @Test
    fun refusesAChangedManifest() {
        val bytes = Base64.getDecoder().decode(delivery.manifest)
        val changed = String(bytes).replace("Sessions: rename inline", "Sessions: rename inlinf")
        assertNull(Signed.open(key, delivery.copy(manifest = Base64.getEncoder().encodeToString(changed.toByteArray()))))
    }

    @Test
    fun refusesAnotherKey() {
        val other = KeyPairGenerator.getInstance("EC").apply { initialize(ECGenParameterSpec("secp256r1")) }.generateKeyPair()
        assertNull(Signed.open(other.public.encoded, delivery))
    }

    @Test
    fun refusesGarbage() {
        assertNull(Signed.open(key, delivery.copy(sig = "not base64!")))
        assertNull(Signed.open(key, delivery.copy(sig = Base64.getEncoder().encodeToString(ByteArray(70)))))
    }

    @Test
    fun readsThePairingLink() {
        val f = Base64.getUrlEncoder().withoutPadding().encodeToString(sha256(key))
        val t = PairTicket.parse("cloche://pair?a=100.101.102.103%3A41778&c=KFHS-RKHA-Y2JY&f=$f&h=my-laptop")
        assertNotNull(t)
        assertEquals("my-laptop", t!!.host)
        assertEquals("100.101.102.103:41778", t.addr)
        assertEquals("KFHS-RKHA-Y2JY", t.code)
        assertTrue(sha256(key).contentEquals(t.fingerprint))
        assertNull(PairTicket.parse("cloche://pair?h=x"))
        assertNull(PairTicket.parse("https://example.com/pair?a=1&c=2"))
    }

    @Test
    fun aMangledPairingLinkIsNoLink() {
        assertNull(PairTicket.parse("cloche://pair?a=100.1.2.3%G1&c=KFHS"))
        assertNull(PairTicket.parse("cloche://pair?a=100.1.2.3:41778&c=KFHS%"))
    }

    @Test
    fun readsATypedMac() {
        assertEquals("my-laptop:41778", PairTicket.typed(" my-laptop ", "x").addr)
        assertEquals("100.1.2.3:9", PairTicket.typed("100.1.2.3:9", "x").addr)
        assertEquals("my-laptop", PairTicket.typed("my-laptop.tail1234.ts.net", "x").host)
    }
}
