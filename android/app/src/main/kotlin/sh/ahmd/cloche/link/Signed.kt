package sh.ahmd.cloche.link

import kotlinx.serialization.SerializationException
import java.io.File
import java.security.GeneralSecurityException
import java.security.KeyFactory
import java.security.MessageDigest
import java.security.Signature
import java.security.spec.X509EncodedKeySpec
import java.util.Base64

object Signed {
    fun open(key: ByteArray, d: SignedDelivery): Manifest? {
        val bytes: ByteArray
        val sig: ByteArray
        try {
            bytes = Base64.getDecoder().decode(d.manifest)
            sig = Base64.getDecoder().decode(d.sig)
        } catch (e: IllegalArgumentException) {
            return null
        }
        val ok = try {
            val pub = KeyFactory.getInstance("EC").generatePublic(X509EncodedKeySpec(key))
            Signature.getInstance("SHA256withECDSA").run {
                initVerify(pub)
                update(bytes)
                verify(sig)
            }
        } catch (e: GeneralSecurityException) {
            false
        }
        if (!ok) return null
        return try {
            json.decodeFromString<Manifest>(bytes.decodeToString())
        } catch (e: SerializationException) {
            null
        } catch (e: IllegalArgumentException) {
            null
        }
    }

    fun fingerprint(key: ByteArray): String =
        sha256(key).copyOf(8).hex().uppercase().chunked(4).joinToString(" ")
}

fun sha256(b: ByteArray): ByteArray = MessageDigest.getInstance("SHA-256").digest(b)

fun sha256(f: File): String {
    val md = MessageDigest.getInstance("SHA-256")
    f.inputStream().use { input ->
        val buf = ByteArray(256 * 1024)
        while (true) {
            val n = input.read(buf)
            if (n < 0) break
            md.update(buf, 0, n)
        }
    }
    return md.digest().hex()
}

fun ByteArray.hex(): String = joinToString("") { "%02x".format(it) }
