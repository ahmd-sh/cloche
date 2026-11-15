package sh.ahmd.cloche.link

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.time.Instant

val json = Json {
    ignoreUnknownKeys = true
    explicitNulls = false
    encodeDefaults = true
}

const val PORT = 41778

@Serializable
data class Manifest(
    val v: Int,
    val id: String,
    val seq: Long,
    val host: String,
    @SerialName("pushed_at") val pushedAt: String,
    val file: FileInfo,
    val apk: ApkInfo? = null,
    val note: String = "",
    val source: Source? = null,
) {
    val pushed: Long get() = runCatching { Instant.parse(pushedAt).toEpochMilli() }.getOrDefault(0)
}

@Serializable
data class FileInfo(val name: String, val size: Long, val sha256: String)

@Serializable
data class ApkInfo(
    @SerialName("package") val pkg: String,
    @SerialName("version_code") val versionCode: Long,
    @SerialName("version_name") val versionName: String = "",
    val label: String = "",
    val signer: String = "",
)

@Serializable
data class Source(
    val dir: String = "",
    val repo: String = "",
    val branch: String = "",
    val commit: String = "",
    val dirty: Boolean = false,
) {
    val described: String
        get() = buildString {
            append(repo.ifEmpty { dir.substringAfterLast('/') })
            if (commit.isNotEmpty()) append("@").append(commit)
            if (dirty) append(" with changes")
            if (branch.isNotEmpty() && branch != "HEAD") append(" on ").append(branch)
        }
}

@Serializable
data class SignedDelivery(val manifest: String, val sig: String)

@Serializable
data class Inbox(val head: Long, val name: String = "", val deliveries: List<SignedDelivery> = emptyList())

@Serializable
data class PairRequest(val code: String, val device: String)

@Serializable
data class PairResponse(val host: String, val name: String = "", val addr: String = "", val key: String, val secret: String, val head: Long)

@Serializable
data class Result(
    val status: String,
    val detail: String = "",
    @SerialName("package") val pkg: String = "",
    @SerialName("version_code") val versionCode: Long = 0,
    @SerialName("version_name") val versionName: String = "",
    val signer: String = "",
)

@Serializable
internal data class Failure(val error: String = "")
