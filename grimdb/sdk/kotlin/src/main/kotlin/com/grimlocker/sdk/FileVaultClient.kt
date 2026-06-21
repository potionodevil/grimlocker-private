package com.grimlocker.sdk

import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.grimlocker.sdk.model.*
import java.io.OutputStream
import java.lang.reflect.Type
import java.net.HttpURLConnection
import java.net.URL
import java.util.Base64

class FileVaultClient(
    private val baseUrl: String,
    private val token: String,
    private val timeoutMs: Int = 30_000
) {
    private val gson = Gson()
    private val apiUrl: String = "${baseUrl.trimEnd('/')}/api/v1"

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
        data class DownloadResponse(val data_b64: String?)
        val resp: DownloadResponse = callSingle(
            "file.download",
            mapOf("manifest_block_id" to manifestBlockId)
        )
        val b64 = resp.data_b64
            ?: throw GrimlockerException("Download returned no data")
        return Base64.getDecoder().decode(b64)
    }

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
                    conn.disconnect()
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
}
