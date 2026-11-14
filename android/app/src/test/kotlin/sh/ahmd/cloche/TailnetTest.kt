package sh.ahmd.cloche

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import sh.ahmd.cloche.link.Tailnet
import java.net.InetAddress

class TailnetTest {
    private fun ip(s: String) = InetAddress.getByName(s)

    @Test
    fun tailnetAddresses() {
        assertTrue(Tailnet.contains(ip("100.64.0.1")))
        assertTrue(Tailnet.contains(ip("100.127.255.254")))
        assertTrue(Tailnet.contains(ip("fd7a:115c:a1e0::1")))
        assertFalse(Tailnet.contains(ip("100.128.0.1")))
        assertFalse(Tailnet.contains(ip("100.63.255.255")))
        assertFalse(Tailnet.contains(ip("127.0.0.1")))
        assertFalse(Tailnet.contains(ip("192.168.1.10")))
        assertFalse(Tailnet.contains(ip("8.8.8.8")))
        assertFalse(Tailnet.contains(ip("fd7a:115c:a1e1::1")))
    }

    @Test
    fun literals() {
        assertEquals(ip("100.64.0.1"), Tailnet.literal("100.64.0.1:41778"))
        assertEquals(ip("fd7a:115c:a1e0::1"), Tailnet.literal("[fd7a:115c:a1e0::1]:41778"))
        assertNull(Tailnet.literal("my-laptop:41778"))
        assertNull(Tailnet.literal("evil.example:41778"))
        assertNull(Tailnet.literal("100.64.0.1.evil.example:41778"))
    }

    @Test
    fun loopback() {
        assertTrue(Tailnet.isLoopback("127.0.0.1:41778"))
        assertTrue(Tailnet.isLoopback("localhost:41778"))
        assertTrue(Tailnet.isLoopback("[::1]:41778"))
        assertFalse(Tailnet.isLoopback("100.64.0.1:41778"))
        assertEquals("my-laptop", Tailnet.hostOf("my-laptop:41778"))
        assertEquals("fd7a::1", Tailnet.hostOf("[fd7a::1]:41778"))
    }
}
