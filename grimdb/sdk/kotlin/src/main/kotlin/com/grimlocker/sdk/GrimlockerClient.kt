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

    // ── Circuit breaker state ─────────────────────────────────────────────────

    private var consecutiveFailures = 0
    private var circuitOpenUntil: Long = 0
    private var isProbe = false

    private fun onSuccess() {
        consecutiveFailures = 0
        circuitOpenUntil = 0
        isProbe = false
    }

    private fun onFailure() {
        if (isProbe) {
            circuitOpenUntil = System.currentTimeMillis() + 30_000
            isProbe = false
            return
        }
        consecutiveFailures++
        if (consecutiveFailures >= 5) {
            circuitOpenUntil = System.currentTimeMillis() + 30_000
            consecutiveFailures = 0
        }
    }

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
        val body = callRaw("vault.recovery_phrase", mapOf("password" to password))
        val map = gson.fromJson<Map<String, Any?>>(
            body,
            object : TypeToken<Map<String, Any?>>() {}.type
        )
        return map["recovery_phrase"] as? String ?: map["phrase"] as? String
    }

    fun generateSSHKey(comment: String = "", saveToVault: Boolean = true): SSHKeyResult {
        return callSingle("tool.ssh_keygen", mapOf("comment" to comment, "save_to_vault" to saveToVault))
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

    fun createEntriesBatch(entries: List<Map<String, Any?>>): List<String> {
        return entries.map {
            val title = it["title"] as String
            val category = it["category"] as String
            @Suppress("UNCHECKED_CAST")
            val fields = it["fields"] as Map<String, String>
            createEntry(title, category, fields).id
        }
    }

    fun deleteEntriesBatch(ids: List<String>) {
        for (id in ids) {
            deleteEntry(id)
        }
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

    val fileVault: FileVaultClient by lazy {
        FileVaultClient(baseUrl, token, timeoutMs)
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
        if (circuitOpenUntil > 0) {
            if (System.currentTimeMillis() < circuitOpenUntil) {
                throw CircuitBreakerOpenException()
            }
            isProbe = true
        }

        var attempt = 0
        var delayMs = 100L
        var lastException: Exception? = null

        while (true) {
            try {
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

                    if (status in 200..299) {
                        val body = conn.inputStream.bufferedReader(Charsets.UTF_8).use { it.readText() }
                        onSuccess()
                        return body
                    }

                    val errorBody = try {
                        conn.errorStream?.bufferedReader(Charsets.UTF_8)?.use { it.readText() } ?: ""
                    } catch (e: Exception) {
                        ""
                    }

                    if (status in 400..499) {
                        parseAndThrow(status, errorBody)
                    }

                    lastException = GrimlockerException("HTTP $status: $errorBody")
                } finally {
                    this.connection = null
                }
            } catch (e: GrimlockerException) {
                onFailure()
                throw e
            } catch (e: java.net.SocketException) {
                lastException = e
            } catch (e: java.net.UnknownHostException) {
                lastException = e
            } catch (e: java.net.ConnectException) {
                lastException = e
            } catch (e: java.net.SocketTimeoutException) {
                lastException = e
            } catch (e: Exception) {
                onFailure()
                throw e
            }

            if (attempt >= 3) break
            Thread.sleep(delayMs.coerceAtMost(2000))
            delayMs *= 2
            attempt++
        }

        onFailure()
        throw lastException ?: Exception("Request failed after retries")
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
