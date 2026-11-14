package sh.ahmd.cloche.link

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.io.IOException
import java.net.Inet6Address
import java.net.InetAddress

class Tailnet(context: Context) {
    private val cm = context.getSystemService(ConnectivityManager::class.java)
    private val _vpn = MutableStateFlow<Network?>(null)
    val vpn: StateFlow<Network?> = _vpn.asStateFlow()
    private val _mobile = MutableStateFlow(false)

    val mobile: StateFlow<Boolean> = _mobile.asStateFlow()

    fun start() {
        cm.activeNetwork?.let { n ->
            val caps = cm.getNetworkCapabilities(n)
            if (caps?.hasTransport(NetworkCapabilities.TRANSPORT_VPN) == true) _vpn.value = n
            _mobile.value = caps.mobile
        }
        cm.registerDefaultNetworkCallback(object : ConnectivityManager.NetworkCallback() {
            private var current: Network? = null

            override fun onCapabilitiesChanged(network: Network, caps: NetworkCapabilities) {
                current = network
                _mobile.value = caps.mobile
            }

            override fun onLost(network: Network) {
                if (network == current) _mobile.value = false
            }
        })
        val request = NetworkRequest.Builder()
            .clearCapabilities()
            .addTransportType(NetworkCapabilities.TRANSPORT_VPN)
            .build()
        cm.registerNetworkCallback(request, object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                _vpn.value = network
            }

            override fun onLost(network: Network) {
                _vpn.compareAndSet(network, null)
            }
        })
    }

    companion object {
        fun hostOf(addr: String): String = when {
            addr.startsWith("[") -> addr.substringAfter('[').substringBefore(']')
            addr.count { it == ':' } == 1 -> addr.substringBeforeLast(':')
            else -> addr
        }

        private fun portOf(addr: String): String? = when {
            addr.startsWith("[") -> addr.substringAfter("]:", "").ifEmpty { null }
            addr.count { it == ':' } == 1 -> addr.substringAfterLast(':')
            else -> null
        }

        fun isLoopback(addr: String) = hostOf(addr) in setOf("127.0.0.1", "localhost", "::1")

        fun literal(addr: String): InetAddress? {
            val h = hostOf(addr)
            val numeric = Regex("""\d{1,3}(\.\d{1,3}){3}""").matches(h) || (h.contains(':') && Regex("[0-9A-Fa-f:.]+").matches(h))
            return if (numeric) runCatching { InetAddress.getByName(h) }.getOrNull() else null
        }

        fun contains(ip: InetAddress): Boolean {
            val b = ip.address
            return when (b.size) {
                4 -> b[0] == 100.toByte() && (b[1].toInt() and 0xc0) == 64
                16 -> b.copyOf(6).contentEquals(byteArrayOf(0xfd.toByte(), 0x7a, 0x11, 0x5c, 0xa1.toByte(), 0xe0.toByte()))
                else -> false
            }
        }

        fun resolve(net: Network, addr: String): String {
            val host = hostOf(addr)
            val port = portOf(addr) ?: PORT.toString()
            val ips = literal(addr)?.let { listOf(it) }
                ?: runCatching { net.getAllByName(host).toList() }.getOrElse { throw IOException("Can't find $host on your tailnet") }
            val ip = ips.firstOrNull(::contains) ?: throw IOException("$host isn't on your tailnet")
            return if (ip is Inet6Address) "[${ip.hostAddress}]:$port" else "${ip.hostAddress}:$port"
        }
    }
}

private val NetworkCapabilities?.mobile: Boolean
    get() = this != null && hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) &&
        !hasTransport(NetworkCapabilities.TRANSPORT_WIFI) && !hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET)

sealed interface Link {
    data object Unpaired : Link
    data object NoTailnet : Link
    data object Connecting : Link

    data class Live(val since: Long) : Link

    data class Down(val reason: String, val since: Long, val retry: Long) : Link
}
