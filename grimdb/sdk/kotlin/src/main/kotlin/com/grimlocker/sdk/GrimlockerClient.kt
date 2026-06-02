package com.grimlocker.sdk

import com.google.gson.Gson
import com.google.gson.annotations.SerializedName
import com.google.gson.reflect.TypeToken
import com.grimlocker.sdk.model.*
import java.io.Closeable
import java.io.OutputStream
import java.lang.reflect.Type
import java.net.HttpURLConnection
import java.net.URL
import java.util.Base64

class GrimlockerClient(
    private val baseUrl: String,
    private val token: String,
    private val timeoutMs: Int = 30_000
) : Closeable {

    private val gson = Gson()
    private val apiUrl: String = "${baseUrl.trimEnd('/')}/api/v1"

    private var connection: HttpURLConnection? = null

    // ── Auth ──────────────────────────────────────────────────────────────────

    fun unlockVault(password: String) {
        call("vault.unlock", mapOf("password" to password))
    }

    fun lockVault() {
        call("vault.logout", emptyMap())
    }

    fun vaultStatus(): VaultStatus =
        callSingle("vault.status", emptyMap())

    fun getRecoveryPhrase(password: String): String? {
        val body = callRaw("vault.recovery", mapOf("password" to password))
        val map = gson.fromJson<Map<String, Any?>>(
            body,
            object : TypeToken<Map<String, Any?>>() {}.type
        )
        return map["recovery_phrase"] as? String
    }

    // ── Entries ───────────────────────────────────────────────────────────────

    fun listEntries(category: String? = null): List<VaultEntry> {
        val (action, payload) = if (category != null) {
            "entry.query" to mapOf("category" to category)
        } else {
            "entry.list" to emptyMap<String, Any?>()
        }
        return callList(action, payload, object : TypeToken<List<VaultEntry>>() {}.type)
    }

    fun getEntry(id: String): VaultEntry =
        callSingle("entry.read", mapOf("id" to id))

    fun createEntry(
        title: String,
        category: String,
        fields: Map<String, String>
    ): VaultEntry = callSingle(
        "entry.create",
        mapOf("title" to title, "category" to category, "fields" to fields)
    )

    fun updateEntry(id: String, fields: Map<String, String>) {
        call("entry.update", mapOf("id" to id, "fields" to fields))
    }

    fun deleteEntry(id: String) {
        call("entry.delete", mapOf("id" to id))
    }

    fun searchEntries(query: String, category: String? = null): List<VaultEntry> {
        val payload = mutableMapOf<String, Any?>("query" to query)
        if (category != null) payload["category"] = category
        return callList("entry.search", payload, object : TypeToken<List<VaultEntry>>() {}.type)
    }

    // ── Typed helpers ─────────────────────────────────────────────────────────

    fun listPasswords(): List<PasswordEntry> =
        listEntries("PASSWORD").map { PasswordEntry.fromEntry(it) }

    fun createPassword(p: PasswordEntry): String {
        val entry = createEntry(p.title, "PASSWORD", p.toFields())
        return entry.id
    }

    fun listSshKeys(): List<SshKeyEntry> =
        listEntries("SSH_KEY").map { SshKeyEntry.fromEntry(it) }

    fun createSshKey(k: SshKeyEntry): String {
        val entry = createEntry(k.title, "SSH_KEY", k.toFields())
        return entry.id
    }

    fun listCertificates(): List<CertificateEntry> =
        listEntries("CERTIFICATE").map { CertificateEntry.fromEntry(it) }

    fun createCertificate(c: CertificateEntry): String {
        val entry = createEntry(c.title, "CERTIFICATE", c.toFields())
        return entry.id
    }

    // ── File Vault ────────────────────────────────────────────────────────────

    fun listFolder(folderId: String = ""): FolderListing =
        callSingle("file.list_folder", mapOf("folder_id" to folderId))

    fun createFolder(name: String, parentId: String = ""): FolderItem =
        callSingle("file.create_folder", mapOf("name" to name, "parent_id" to parentId))

    fun renameFolder(id: String, name: String) {
        call("file.rename_folder", mapOf("id" to id, "name" to name))
    }

    fun deleteFolder(id: String) {
        call("file.delete_folder", mapOf("id" to id))
    }

    fun moveFile(manifestBlockId: String, folderId: String) {
        call("file.move", mapOf("manifest_block_id" to manifestBlockId, "folder_id" to folderId))
    }

    fun uploadFile(
        data: ByteArray,
        fileName: String,
        mimeType: String = "application/octet-stream",
        folderId: String = "",
        onProgress: ((UploadProgress) -> Unit)? = null
    ): FileEntry {
        onProgress?.invoke(UploadProgress(bytesSent = 0, totalBytes = data.size.toLong()))
        val dataB64 = Base64.getEncoder().encodeToString(data)
        val entry: FileEntry = callSingle(
            "file.ingest",
            mapOf(
                "file_name" to fileName,
                "mime_type" to mimeType,
                "folder_id" to folderId,
                "data_b64" to dataB64
            )
        )
        onProgress?.invoke(UploadProgress(bytesSent = data.size.toLong(), totalBytes = data.size.toLong()))
        return entry
    }

    fun downloadFile(manifestBlockId: String): ByteArray {
        data class DownloadResponse(
            @SerializedName("data_b64") val dataB64: String?
        )
        val resp: DownloadResponse = callSingle(
            "file.download",
            mapOf("manifest_block_id" to manifestBlockId)
        )
        val b64 = resp.dataB64
            ?: throw GrimlockerException("Download returned no data")
        return Base64.getDecoder().decode(b64)
    }

    // ── Workspaces ────────────────────────────────────────────────────────────

    fun listWorkspaces(): List<Workspace> =
        callList("workspace.list", emptyMap(), object : TypeToken<List<Workspace>>() {}.type)

    fun createWorkspace(name: String): Workspace =
        callSingle("workspace.create", mapOf("name" to name))

    fun switchWorkspace(id: String) {
        call("workspace.switch", mapOf("id" to id))
    }

    fun renameWorkspace(id: String, name: String) {
        call("workspace.rename", mapOf("id" to id, "name" to name))
    }

    fun deleteWorkspace(id: String) {
        call("workspace.delete", mapOf("id" to id))
    }

    // ── Sync ──────────────────────────────────────────────────────────────────

    fun listSyncPeers(): SyncStatus =
        callSingle("sync.list_peers", emptyMap())

    fun triggerSync() {
        call("sync.trigger", emptyMap())
    }

    // ── Audit ─────────────────────────────────────────────────────────────────

    fun listAuditEvents(n: Int = 50): List<AuditEvent> =
        callList("audit.list", mapOf("n" to n), object : TypeToken<List<AuditEvent>>() {}.type)

    // ── Health ────────────────────────────────────────────────────────────────

    fun healthCheck(): VaultStatus = vaultStatus()

    // ── Internal ─────────────────────────────────────────────────────────────

    private fun call(action: String, payload: Map<String, Any?>) {
        doPost(action, payload)
    }

    private inline fun <reified T> callSingle(action: String, payload: Map<String, Any?>): T {
        val body = doPost(action, payload)
        return try {
            gson.fromJson(body, T::class.java)
        } catch (e: GrimlockerException) {
            throw e
        } catch (e: Exception) {
            throw GrimlockerException(-100, "Failed to parse response: ${e.message}")
        }
    }

    private fun <T> callList(action: String, payload: Map<String, Any?>, type: Type): T {
        val body = doPost(action, payload)
        return try {
            gson.fromJson(body, type)
        } catch (e: GrimlockerException) {
            throw e
        } catch (e: Exception) {
            throw GrimlockerException(-100, "Failed to parse response: ${e.message}")
        }
    }

    private fun callRaw(action: String, payload: Map<String, Any?>): String =
        doPost(action, payload)

    private fun doPost(action: String, payload: Map<String, Any?>): String {
        val requestBody = mapOf("action" to action, "payload" to payload)
        val json = gson.toJson(requestBody)

        val url = URL(apiUrl)
        val conn = (url.openConnection() as HttpURLConnection).apply {
            requestMethod = "POST"
            doOutput = true
            connectTimeout = timeoutMs
            readTimeout = timeoutMs
            setRequestProperty("Content-Type", "application/json")
            setRequestProperty("Accept", "application/json")
            setRequestProperty("X-Grimlocker-Token", token)
            setRequestProperty("User-Agent", "GrimlockerSDK-Kotlin/1.0.0")
        }

        this.connection = conn

        try {
            conn.outputStream.use { os: OutputStream ->
                os.write(json.toByteArray(Charsets.UTF_8))
                os.flush()
            }

            val status = conn.responseCode
            val body: String = when {
                status in 200..299 -> {
                    conn.inputStream.bufferedReader(Charsets.UTF_8).use { it.readText() }
                }
                else -> {
                    val errorBody = try {
                        conn.errorStream?.bufferedReader(Charsets.UTF_8)?.use { it.readText() } ?: ""
                    } catch (e: Exception) {
                        ""
                    }
                    parseAndThrow(status, errorBody)
                }
            }

            return body
        } finally {
            this.connection = null
        }
    }

    private fun parseAndThrow(httpStatus: Int, body: String): Nothing {
        val code: Int
        val message: String

        try {
            val errorMap = gson.fromJson<Map<String, Any?>>(
                body,
                object : TypeToken<Map<String, Any?>>() {}.type
            )
            code = (errorMap["error_code"] as? Double)?.toInt() ?: 0
            message = (errorMap["error"] as? String) ?: body.ifEmpty { "HTTP $httpStatus" }
        } catch (e: Exception) {
            code = 0
            message = body.ifEmpty { "HTTP $httpStatus" }
        }

        throw GrimlockerException(code, message)
    }

    override fun close() {
        try {
            connection?.disconnect()
        } catch (_: Exception) {
        }
        connection = null
    }
}
