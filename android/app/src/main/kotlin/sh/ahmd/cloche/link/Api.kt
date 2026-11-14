package sh.ahmd.cloche.link

import android.net.Network
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL

class Api(private val net: Network?, private val addr: String, private val secret: String?) {
    class Unpaired(message: String) : IOException(message)

    class Gone(message: String) : IOException(message)

    class Refused(val code: Int, message: String) : IOException(message)

    suspend fun inbox(after: Long, wait: Int, connected: () -> Unit): Inbox =
        open("/v1/inbox?after=$after&wait=$wait", readTimeout = (wait + 30) * 1000).call { c ->
            c.connect()
            connected()
            check(c)
            json.decodeFromString<Inbox>(c.inputStream.use { it.readBytes() }.decodeToString())
        }

    suspend fun pair(req: PairRequest): PairResponse =
        open("/v1/pair").call { c ->
            post(c, json.encodeToString(PairRequest.serializer(), req))
            check(c)
            json.decodeFromString<PairResponse>(c.inputStream.use { it.readBytes() }.decodeToString())
        }

    suspend fun result(id: String, r: Result) {
        open("/v1/result/$id").call { c ->
            post(c, json.encodeToString(Result.serializer(), r))
            check(c)
        }
    }

    suspend fun download(id: String, part: File, size: Long, progress: (Float) -> Unit) {
        open("/v1/apk/$id", readTimeout = 60_000).call { c ->
            val have = part.length().takeIf { it in 1 until size } ?: 0L
            if (have > 0) c.setRequestProperty("Range", "bytes=$have-")
            check(c)
            val append = have > 0 && c.responseCode == HttpURLConnection.HTTP_PARTIAL
            var done = if (append) have else 0L
            FileOutputStream(part, append).use { out ->
                c.inputStream.use { input ->
                    val buf = ByteArray(64 * 1024)
                    var told = 0L
                    while (true) {
                        val n = input.read(buf)
                        if (n < 0) break
                        done += n
                        if (done > size) throw IOException("more bytes than its manifest says")
                        out.write(buf, 0, n)
                        if (done - told > 128 * 1024) {
                            progress(done.toFloat() / size)
                            told = done
                        }
                    }
                }
            }
            if (done != size) throw IOException("got $done of $size bytes")
        }
    }

    private fun open(path: String, readTimeout: Int = 30_000): HttpURLConnection {
        val url = URL("http://$addr$path")
        val c = (net?.openConnection(url) ?: url.openConnection()) as HttpURLConnection
        c.connectTimeout = 15_000
        c.readTimeout = readTimeout
        c.useCaches = false
        c.instanceFollowRedirects = false
        secret?.let { c.setRequestProperty("Authorization", "Bearer $it") }
        return c
    }

    private fun post(c: HttpURLConnection, body: String) {
        c.requestMethod = "POST"
        c.doOutput = true
        c.setRequestProperty("Content-Type", "application/json")
        c.outputStream.use { it.write(body.encodeToByteArray()) }
    }

    private fun check(c: HttpURLConnection) {
        val code = c.responseCode
        if (code in 200..299) return
        val msg = runCatching { json.decodeFromString<Failure>(c.errorStream.use { it.readBytes() }.decodeToString()).error }
            .getOrNull()?.ifEmpty { null } ?: "HTTP $code"
        throw when (code) {
            401 -> Unpaired(msg)
            404 -> Gone(msg)
            else -> Refused(code, msg)
        }
    }
}

private suspend fun <T> HttpURLConnection.call(block: (HttpURLConnection) -> T): T = coroutineScope {
    val conn = this@call
    val watch = launch(start = CoroutineStart.UNDISPATCHED) {
        try {
            awaitCancellation()
        } finally {
            conn.disconnect()
        }
    }
    try {
        withContext(Dispatchers.IO) { block(conn) }
    } finally {
        watch.cancel()
    }
}
