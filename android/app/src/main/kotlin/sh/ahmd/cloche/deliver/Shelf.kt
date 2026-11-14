package sh.ahmd.cloche.deliver

import android.content.res.AssetFileDescriptor
import android.database.Cursor
import android.database.MatrixCursor
import android.graphics.Point
import android.net.Uri
import android.os.CancellationSignal
import android.os.ParcelFileDescriptor
import android.provider.DocumentsContract
import android.provider.DocumentsContract.Document
import android.provider.DocumentsContract.Root
import android.provider.DocumentsProvider
import sh.ahmd.cloche.R
import sh.ahmd.cloche.cloche
import java.io.File
import java.io.FileNotFoundException

class Shelf : DocumentsProvider() {
    override fun onCreate() = true

    private val app get() = context!!.cloche
    private fun dir() = app.apkFile("x").parentFile!!

    override fun queryRoots(projection: Array<out String>?): Cursor =
        MatrixCursor(projection ?: ROOT_COLUMNS).apply {
            newRow().apply {
                add(Root.COLUMN_ROOT_ID, ROOT)
                add(Root.COLUMN_DOCUMENT_ID, DIR)
                add(Root.COLUMN_TITLE, "Cloche")
                add(Root.COLUMN_SUMMARY, "Builds kept on this phone")
                add(Root.COLUMN_ICON, R.mipmap.ic_launcher)
                add(Root.COLUMN_FLAGS, Root.FLAG_LOCAL_ONLY)
                add(Root.COLUMN_MIME_TYPES, APK)
                add(Root.COLUMN_AVAILABLE_BYTES, dir().usableSpace)
            }
        }

    override fun queryDocument(documentId: String, projection: Array<out String>?): Cursor =
        MatrixCursor(projection ?: DOCUMENT_COLUMNS).apply {
            if (documentId == DIR) folder(this) else file(this, apk(documentId))
        }

    override fun queryChildDocuments(parentDocumentId: String, projection: Array<out String>?, sortOrder: String?): Cursor {
        if (parentDocumentId != DIR) throw FileNotFoundException(parentDocumentId)
        return MatrixCursor(projection ?: DOCUMENT_COLUMNS).apply {
            dir().listFiles { f -> f.extension == "apk" }?.sortedByDescending { it.lastModified() }?.forEach { file(this, it) }
            setNotificationUri(context!!.contentResolver, children())
        }
    }

    override fun openDocument(documentId: String, mode: String, signal: CancellationSignal?): ParcelFileDescriptor {
        if (mode != "r") throw UnsupportedOperationException("Cloche's builds are read only")
        return ParcelFileDescriptor.open(apk(documentId), ParcelFileDescriptor.MODE_READ_ONLY)
    }

    override fun openDocumentThumbnail(documentId: String, sizeHint: Point?, signal: CancellationSignal?): AssetFileDescriptor {
        val icon = app.iconFile(documentId).takeIf { it.exists() } ?: throw FileNotFoundException(documentId)
        return AssetFileDescriptor(ParcelFileDescriptor.open(icon, ParcelFileDescriptor.MODE_READ_ONLY), 0, icon.length())
    }

    private fun folder(c: MatrixCursor) {
        c.newRow().apply {
            add(Document.COLUMN_DOCUMENT_ID, DIR)
            add(Document.COLUMN_DISPLAY_NAME, "Builds")
            add(Document.COLUMN_MIME_TYPE, Document.MIME_TYPE_DIR)
            add(Document.COLUMN_FLAGS, 0)
            add(Document.COLUMN_SIZE, null)
            add(Document.COLUMN_LAST_MODIFIED, dir().lastModified())
        }
    }

    private fun file(c: MatrixCursor, f: File) {
        val id = f.nameWithoutExtension
        c.newRow().apply {
            add(Document.COLUMN_DOCUMENT_ID, id)
            add(Document.COLUMN_DISPLAY_NAME, name(id))
            add(Document.COLUMN_MIME_TYPE, APK)
            add(Document.COLUMN_FLAGS, if (app.iconFile(id).exists()) Document.FLAG_SUPPORTS_THUMBNAIL else 0)
            add(Document.COLUMN_SIZE, f.length())
            add(Document.COLUMN_LAST_MODIFIED, f.lastModified())
        }
    }

    private fun apk(id: String): File {
        if (!id.matches(ID)) throw FileNotFoundException(id)
        return app.apkFile(id).takeIf { it.exists() } ?: throw FileNotFoundException(id)
    }

    private fun name(id: String): String = names()[id] ?: "$id.apk"

    private fun names(): Map<String, String> {
        if (!app.started) return emptyMap()
        val here = dir().listFiles { f -> f.extension == "apk" }.orEmpty().mapTo(HashSet()) { it.nameWithoutExtension }
        val taken = HashMap<String, Int>()
        return app.store.value.deliveries.filter { it.id in here }.sortedBy { it.seq }.associate { d ->
            val version = d.apk?.versionName?.ifEmpty { null } ?: d.apk?.versionCode?.toString()
            val base = listOfNotNull(d.label, version).joinToString("-").replace(Regex("[\\\\/:*?\"<>|\\s]+"), "_")
            val n = taken.merge(base, 1, Int::plus)!!
            d.id to if (n == 1) "$base.apk" else "$base ($n).apk"
        }
    }

    companion object {
        const val AUTHORITY = "sh.ahmd.cloche.shelf"
        private const val ROOT = "cloche"
        private const val DIR = "builds"
        private const val APK = "application/vnd.android.package-archive"
        private val ID = Regex("[A-Za-z0-9_-]+")

        private val ROOT_COLUMNS = arrayOf(
            Root.COLUMN_ROOT_ID, Root.COLUMN_DOCUMENT_ID, Root.COLUMN_TITLE, Root.COLUMN_SUMMARY,
            Root.COLUMN_ICON, Root.COLUMN_FLAGS, Root.COLUMN_MIME_TYPES, Root.COLUMN_AVAILABLE_BYTES,
        )
        private val DOCUMENT_COLUMNS = arrayOf(
            Document.COLUMN_DOCUMENT_ID, Document.COLUMN_DISPLAY_NAME, Document.COLUMN_MIME_TYPE,
            Document.COLUMN_FLAGS, Document.COLUMN_SIZE, Document.COLUMN_LAST_MODIFIED,
        )

        fun children(): Uri = DocumentsContract.buildChildDocumentsUri(AUTHORITY, DIR)

        fun root(): Uri = DocumentsContract.buildRootUri(AUTHORITY, ROOT)
    }
}
