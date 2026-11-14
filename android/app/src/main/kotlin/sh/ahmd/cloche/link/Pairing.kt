package sh.ahmd.cloche.link

import android.os.Build
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import sh.ahmd.cloche.BuildConfig
import sh.ahmd.cloche.Cloche
import sh.ahmd.cloche.store.Paired
import java.io.IOException
import java.net.URLDecoder
import java.security.MessageDigest
import java.util.Base64

data class PairTicket(
    val host: String,
    val addr: String,
    val code: String,
    val fingerprint: ByteArray? = null,
) {
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
            return PairTicket(host = q["h"]?.ifBlank { null } ?: addr.substringBeforeLast(':'), addr = addr, code = code, fingerprint = f)
        }

        fun typed(mac: String, code: String): PairTicket {
            val m = mac.trim()
            val addr = if (m.contains(':')) m else "$m:$PORT"
            return PairTicket(host = m.substringBeforeLast(':').substringBefore('.'), addr = addr, code = code.trim())
        }
    }

    override fun equals(other: Any?) = other is PairTicket && host == other.host && addr == other.addr && code == other.code &&
        (fingerprint?.contentEquals(other.fingerprint) ?: (other.fingerprint == null))

    override fun hashCode() = addr.hashCode() * 31 + code.hashCode()
}

object Pairing {
    suspend fun pair(app: Cloche, t: PairTicket): Paired {
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
        val paired = Paired(
            host = res.host.ifEmpty { t.host },
            addr = back,
            key = res.key,
            secret = app.secrets.seal(res.secret),
            device = Build.MODEL,
            pairedAt = System.currentTimeMillis(),
        )
        app.store.update { s ->
            val sameMac = s.paired?.key == paired.key
            s.copy(paired = paired, cursor = if (sameMac) maxOf(s.cursor, res.head) else res.head, unpaired = null)
        }
        app.notices.paired()
        return paired
    }
}
