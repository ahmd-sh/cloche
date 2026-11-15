package sh.ahmd.cloche.link

import android.os.Build
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import sh.ahmd.cloche.BuildConfig
import sh.ahmd.cloche.Cloche
import sh.ahmd.cloche.store.Computer
import sh.ahmd.cloche.store.newComputerId
import java.io.IOException
import java.net.URLDecoder
import java.security.MessageDigest
import java.util.Base64

data class PairTicket(
    val host: String,
    val addr: String,
    val code: String,
    val fingerprint: ByteArray? = null,
    val display: String = "",
    val friend: Boolean = false,
) {
    val name: String get() = display.ifEmpty { host }

    fun matches(c: Computer) = fingerprint?.let { MessageDigest.isEqual(it, sha256(c.keyBytes)) } == true

    companion object {
        fun parse(link: String): PairTicket? {
            if (!link.startsWith("cloche://pair")) return null
            val q = runCatching {
                link.substringAfter('?', "").split('&').mapNotNull {
                    val k = it.substringBefore('=', "")
                    if (k.isEmpty()) null else k to URLDecoder.decode(it.substringAfter('='), "UTF-8")
                }.toMap()
            }.getOrNull() ?: return null
            val addr = q["a"]?.takeIf { it.isNotBlank() } ?: return null
            val code = q["c"]?.takeIf { it.isNotBlank() } ?: return null
            val f = q["f"]?.let { runCatching { Base64.getUrlDecoder().decode(it) }.getOrNull() }
            return PairTicket(
                host = q["h"]?.ifBlank { null } ?: addr.substringBeforeLast(':'), addr = addr, code = code, fingerprint = f,
                display = q["n"].orEmpty().filter { !it.isISOControl() }.trim().take(40), friend = q["friend"] == "1",
            )
        }

        fun typed(mac: String, code: String): PairTicket {
            val m = mac.trim()
            val addr = if (m.contains(':')) m else "$m:$PORT"
            return PairTicket(host = m.substringBeforeLast(':').substringBefore('.'), addr = addr, code = code.trim())
        }
    }

    override fun equals(other: Any?) = other is PairTicket && host == other.host && addr == other.addr && code == other.code &&
        display == other.display && friend == other.friend &&
        (fingerprint?.contentEquals(other.fingerprint) ?: (other.fingerprint == null))

    override fun hashCode() = addr.hashCode() * 31 + code.hashCode()
}

object Pairing {
    suspend fun pair(app: Cloche, t: PairTicket, name: String, replace: String?): Computer {
        val dev = BuildConfig.DEBUG && Tailnet.isLoopback(t.addr)
        val net = if (dev) null else app.tailnet.vpn.value ?: throw IOException("Tailscale is off on this phone")
        val addr = if (net == null) t.addr else withContext(Dispatchers.IO) { Tailnet.resolve(net, t.addr) }
        val res = Api(net, addr, null).pair(PairRequest(t.code, Build.MODEL))
        val key = Base64.getDecoder().decode(res.key)
        if (t.fingerprint != null && !MessageDigest.isEqual(sha256(key), t.fingerprint)) {
            throw IOException("The computer's key doesn't match the pairing link")
        }
        val back = res.addr.ifEmpty { addr }
        if (!dev && Tailnet.literal(back)?.let(Tailnet::contains) != true) {
            throw IOException("The computer answered with an address outside your tailnet")
        }
        val sealed = app.secrets.seal(res.secret)
        val host = res.host.ifEmpty { t.host }
        val offered = res.name.filter { !it.isISOControl() }.trim().take(40)
        val wanted = name.trim().take(40)
        val nickname = if (wanted.isEmpty() || wanted == offered.ifEmpty { host }) "" else wanted
        val now = System.currentTimeMillis()
        val c = Computer(
            id = newComputerId(), host = host, addr = back, key = res.key, secret = sealed, device = Build.MODEL,
            pairedAt = now, cursor = res.head, offered = offered, nickname = nickname, seen = now,
        )
        val after = app.store.update { it.paired(c, res.head, replace) }
        app.notices.paired()
        return after.computers.first { it.key == res.key }
    }
}
