package sh.ahmd.cloche

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import sh.ahmd.cloche.link.FileInfo
import sh.ahmd.cloche.link.Link
import sh.ahmd.cloche.link.Links
import sh.ahmd.cloche.link.Manifest
import sh.ahmd.cloche.link.PairTicket
import sh.ahmd.cloche.store.Computer
import sh.ahmd.cloche.store.Delivery
import sh.ahmd.cloche.store.Saved
import sh.ahmd.cloche.store.Stage
import sh.ahmd.cloche.store.Store
import sh.ahmd.cloche.store.cleanName
import java.io.File
import java.io.IOException

class ComputersTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private fun m(id: String, seq: Long) = Manifest(1, id, seq, "mac", "2026-11-04T18:00:00Z", FileInfo("a.apk", 10, "00"))

    private fun computer(id: String, host: String, key: String, addr: String = "100.101.102.103:41778") =
        Computer(id, host, addr, key, "sealed-$id", "Pixel", 1)

    @Test
    fun readsWhatCloche03Saved() {
        val f = File(tmp.root, "state.json")
        f.writeText(
            """
            {"paired": {"host": "vishingstar", "addr": "100.101.102.103:41778", "key": "AAAA", "secret": "s", "device": "Pixel", "pairedAt": 5},
             "cursor": 11,
             "deliveries": [{"id": "abcdefgh", "seq": 11, "m": {"v": 1, "id": "abcdefgh", "seq": 11, "host": "vishingstar", "pushed_at": "2026-11-04T18:00:00Z", "file": {"name": "a.apk", "size": 10, "sha256": "00"}}, "stage": "Installed", "received": 1, "changed": 1, "reported": "Installed"}],
             "pins": [{"pkg": "sh.ahmd.wallflower", "signer": "aa", "since": 1}]}
            """.trimIndent(),
        )
        val s = Store(f).value
        val c = s.computers.single()
        assertNull(s.paired)
        assertEquals("vishingstar", c.host)
        assertEquals("vishingstar", c.name)
        assertEquals(11L, c.cursor)
        assertEquals("AAAA", c.key)
        assertEquals(c.id, s.deliveries.single().from)
        assertEquals("sh.ahmd.wallflower", s.pins.single().pkg)
        assertEquals(c.id, Store(f).value.computers.single().id)
    }

    @Test
    fun pairingAddsRefreshesAndReplaces() {
        val home = computer("c1", "vishingstar", "K1").copy(cursor = 20, nickname = "Home", review = true)
        var s = Saved(computers = listOf(home))

        s = s.paired(computer("new", "sams-mac", "K2"), head = 3, replace = null)
        assertEquals(listOf("vishingstar", "sams-mac"), s.computers.map { it.host })

        s = s.paired(computer("again", "vishingstar", "K1").copy(nickname = "Home"), head = 7, replace = null)
        val again = s.computer("c1")!!
        assertEquals(2, s.computers.size)
        assertEquals(20L, again.cursor)
        assertTrue(again.review)
        assertEquals("sealed-again", again.secret)

        val d = Delivery("abcdefgh", 4, m("abcdefgh", 4), Stage.Ready, received = 1, changed = 1, from = "c1")
        s = s.copy(deliveries = listOf(d))
        s = s.paired(computer("reset", "vishingstar", "K3"), head = 2, replace = "c1")
        assertNull(s.computer("c1"))
        val reset = s.computers.first { it.key == "K3" }
        assertEquals("reset", reset.id)
        assertEquals(2L, reset.cursor)
        assertTrue(reset.review)
        assertEquals(listOf("K3", "K2"), s.computers.map { it.key })
        assertFalse(s.owed(s.deliveries.single()))

        s = s.paired(computer("twin", "vishingstar", "K4"), head = 1, replace = null)
        assertEquals(3, s.computers.size)
    }

    @Test
    fun anotherAddressCantClaimAPairedKey() {
        val s = Saved(computers = listOf(computer("c1", "vishingstar", "K1")))
        val e = assertThrows(IOException::class.java) { s.paired(computer("x", "sams-mac", "K1", addr = "100.64.0.9:41778"), 1, null) }
        assertTrue(e.message!!, e.message!!.contains("vishingstar"))
        assertEquals(1, s.paired(computer("y", "vishingstar", "K1"), 1, null).computers.size)
    }

    @Test
    fun twoComputersWithOneNameAreToldApart() {
        val a = computer("a", "vishingstar", "AAAA")
        val b = computer("b", "sams-mac", "BBBB").copy(offered = "VISHINGSTAR")
        val s = Saved(computers = listOf(a, b))
        assertTrue(s.label(a).startsWith("vishingstar · "))
        assertTrue(s.label(b).startsWith("VISHINGSTAR · "))
        assertTrue(s.label(a) != s.label(b))
        assertEquals("Work", Saved(computers = listOf(a, b.copy(nickname = "Work"))).label(b.copy(nickname = "Work")))
        assertEquals("Sam's builds", cleanName("  Sam's\u202E builds\u0007 "))
    }

    @Test
    fun unpairingLeavesNothingOwed() {
        val c = computer("c1", "mac", "K1")
        val fetching = Delivery("aaaaaaaa", 1, m("aaaaaaaa", 1), Stage.Fetching, received = 1, changed = 1, from = "c1")
        val installed = Delivery("bbbbbbbb", 2, m("bbbbbbbb", 2), Stage.Installed, received = 1, changed = 1, from = "c1")
        val s = Saved(computers = listOf(c), deliveries = listOf(fetching, installed))
        assertTrue(s.owed(installed))
        val after = s.orphan("c1").copy(computers = emptyList())
        assertEquals(Stage.Failed, after.delivery("aaaaaaaa")?.stage)
        assertTrue(after.deliveries.none { after.owed(it) })
    }

    @Test
    fun reviewFirstTurnsOffOneTap() {
        val c = computer("c1", "mac", "K1")
        val d = Delivery("abcdefgh", 1, m("abcdefgh", 1), Stage.Ready, received = 1, changed = 1, from = "c1")
        assertFalse(Saved(computers = listOf(c)).reviews(d))
        assertTrue(Saved(computers = listOf(c.copy(review = true))).reviews(d))
        assertTrue(Saved(computers = listOf(c)).reviews(d.copy(fresh = true)))
    }

    @Test
    fun namesComeFromTheOwnerThenTheComputer() {
        val c = computer("c1", "vishingstar", "K1")
        assertEquals("vishingstar", c.name)
        assertEquals("Sam's builds", c.copy(offered = "Sam's builds").name)
        assertEquals("Work", c.copy(offered = "Sam's builds", nickname = "Work").name)
    }

    @Test
    fun newerAcrossComputersGoesByArrival() {
        val a = Delivery("aaaaaaaa", 40, m("aaaaaaaa", 40), Stage.Ready, received = 100, changed = 1, from = "c1")
        val b = Delivery("bbbbbbbb", 2, m("bbbbbbbb", 2), Stage.Ready, received = 200, changed = 1, from = "c2")
        assertTrue(b.newerThan(a))
        assertFalse(a.newerThan(b))
        assertTrue(a.copy(seq = 41, received = 100).newerThan(a))
    }

    @Test
    fun unreachableComputersBackOff() {
        assertEquals(1_000L, Links.backoff(1, 0))
        assertEquals(60_000L, Links.backoff(7, 0))
        assertEquals(60_000L, Links.backoff(50, Links.HOUR - 1))
        assertEquals(600_000L, Links.backoff(50, Links.HOUR))
    }

    @Test
    fun theLinkNotificationNamesWhatsDown() {
        val a = computer("a", "vishingstar", "K1")
        val b = computer("b", "sams-mac", "K2").copy(nickname = "Sam")
        val live = Link.Live(1)
        val down = Link.Down("Not answering", 1, 2)
        assertEquals("Not paired", Links.title(emptyList(), emptyMap()))
        assertEquals("Linked to vishingstar", Links.title(listOf(a), mapOf("a" to live)))
        assertEquals("Linked to 2 computers", Links.title(listOf(a, b), mapOf("a" to live, "b" to live)))
        assertEquals("Linked to 1 of 2 · can't reach Sam", Links.title(listOf(a, b), mapOf("a" to live, "b" to down)))
        assertEquals("Can't reach your computers", Links.title(listOf(a, b), mapOf("a" to down, "b" to down)))
        assertEquals("Tailscale is off", Links.title(listOf(a, b), mapOf("a" to Link.NoTailnet)))
        assertEquals(live, Links.overall(listOf(a, b), mapOf("a" to down, "b" to live)))
        assertEquals(Link.Unpaired, Links.overall(emptyList(), emptyMap()))
    }

    @Test
    fun aLinkCarriesTheComputersOwnName() {
        val t = PairTicket.parse("cloche://pair?a=100.101.102.103%3A41778&c=K7Q2-M9XD-3FPA&h=sams-mac&n=Sam%27s%20builds%1B&friend=1")!!
        assertEquals("sams-mac", t.host)
        assertEquals("Sam's builds", t.display)
        assertEquals("Sam's builds", t.name)
        assertTrue(t.friend)
        val plain = PairTicket.parse("cloche://pair?a=100.101.102.103%3A41778&c=K7Q2-M9XD-3FPA&h=mac")!!
        assertEquals("mac", plain.name)
        assertFalse(plain.friend)
    }
}
