package sh.ahmd.cloche.deliver

import android.content.Context
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.drawable.Drawable
import sh.ahmd.cloche.link.hex
import sh.ahmd.cloche.link.sha256
import sh.ahmd.cloche.store.Apk
import java.io.File

object Inspect {
    fun read(context: Context, file: File, icon: File?): Apk? {
        val pm = context.packageManager
        val info = pm.getPackageArchiveInfo(file.path, PackageManager.PackageInfoFlags.of(PackageManager.GET_SIGNING_CERTIFICATES.toLong()))
            ?: return null
        val ai = info.applicationInfo ?: return null
        ai.sourceDir = file.path
        ai.publicSourceDir = file.path
        val certs = info.signingInfo?.apkContentsSigners?.map { it.toByteArray() }.orEmpty()
        if (certs.isEmpty()) return null

        val installed = try {
            pm.getPackageInfo(info.packageName, PackageManager.PackageInfoFlags.of(0))
        } catch (e: PackageManager.NameNotFoundException) {
            null
        }
        val sameSigner = installed?.let { certs.all { c -> pm.hasSigningCertificate(info.packageName, c, PackageManager.CERT_INPUT_RAW_X509) } }
        if (icon != null) runCatching { save(ai.loadIcon(pm), icon) }

        return Apk(
            pkg = info.packageName,
            label = ai.loadLabel(pm).toString(),
            versionCode = info.longVersionCode,
            versionName = info.versionName.orEmpty(),
            signer = certs.map { sha256(it).hex() }.sorted().joinToString(","),
            minSdk = ai.minSdkVersion,
            targetSdk = ai.targetSdkVersion,
            installedCode = installed?.longVersionCode,
            installedName = installed?.versionName,
            installedSameSigner = sameSigner,
        )
    }

    private fun save(d: Drawable, to: File) {
        val px = 192
        val bmp = Bitmap.createBitmap(px, px, Bitmap.Config.ARGB_8888)
        d.setBounds(0, 0, px, px)
        d.draw(Canvas(bmp))
        to.parentFile?.mkdirs()
        to.outputStream().use { bmp.compress(Bitmap.CompressFormat.PNG, 100, it) }
    }
}
