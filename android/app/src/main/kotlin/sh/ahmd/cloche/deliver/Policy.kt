package sh.ahmd.cloche.deliver

import sh.ahmd.cloche.store.Apk
import sh.ahmd.cloche.store.Pin

object Policy {
    data class Verdict(val ok: Boolean, val reason: String = "", val fresh: Boolean = false)

    fun check(apk: Apk, pin: Pin?, sdk: Int): Verdict = when {
        apk.minSdk > sdk ->
            Verdict(false, "needs Android API ${apk.minSdk}; this phone has $sdk")
        pin != null && pin.signer != apk.signer ->
            Verdict(false, "signed with a different key from the builds Cloche installed before (${short(pin.signer)}, now ${short(apk.signer)})")
        apk.installedSameSigner == false ->
            Verdict(false, "signed with a different key from the installed app")
        apk.installedCode != null && apk.versionCode < apk.installedCode ->
            Verdict(false, "older than the installed app (version code ${apk.versionCode} < ${apk.installedCode})")
        else ->
            Verdict(true, fresh = apk.installedCode == null && pin == null)
    }

    private fun short(signer: String) = signer.take(8).uppercase().chunked(4).joinToString(" ")
}
