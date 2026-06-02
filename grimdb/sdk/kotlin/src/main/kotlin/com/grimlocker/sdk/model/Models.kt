package com.grimlocker.sdk.model

import com.google.gson.annotations.SerializedName

data class VaultEntry(
    val id: String,
    val title: String,
    val category: String,
    val fields: Map<String, String>? = null,
    @SerializedName("created_at") val createdAt: Long = 0,
    @SerializedName("updated_at") val updatedAt: Long = 0
) {
    fun field(key: String): String = fields?.get(key) ?: ""
}

data class PasswordEntry(
    val title: String,
    val username: String = "",
    val password: String = "",
    val url: String = "",
    val notes: String = "",
    val id: String = ""
) {
    fun toFields(): Map<String, String> = mapOf(
        "username" to username,
        "password" to password,
        "url" to url,
        "notes" to notes
    )

    companion object {
        fun fromEntry(e: VaultEntry): PasswordEntry = PasswordEntry(
            title = e.title,
            username = e.field("username"),
            password = e.field("password"),
            url = e.field("url"),
            notes = e.field("notes"),
            id = e.id
        )
    }
}

data class SshKeyEntry(
    val title: String,
    @SerializedName("public_key") val publicKey: String = "",
    @SerializedName("private_key") val privateKey: String = "",
    val username: String = "",
    val passphrase: String = "",
    val id: String = ""
) {
    fun toFields(): Map<String, String> = mapOf(
        "public_key" to publicKey,
        "private_key" to privateKey,
        "username" to username,
        "passphrase" to passphrase
    )

    companion object {
        fun fromEntry(e: VaultEntry): SshKeyEntry = SshKeyEntry(
            title = e.title,
            publicKey = e.field("public_key"),
            privateKey = e.field("private_key"),
            username = e.field("username"),
            passphrase = e.field("passphrase"),
            id = e.id
        )
    }
}

data class CertificateEntry(
    val title: String,
    val domain: String = "",
    val certificate: String = "",
    @SerializedName("private_key") val privateKey: String = "",
    val id: String = ""
) {
    fun toFields(): Map<String, String> = mapOf(
        "domain" to domain,
        "certificate" to certificate,
        "private_key" to privateKey
    )

    companion object {
        fun fromEntry(e: VaultEntry): CertificateEntry = CertificateEntry(
            title = e.title,
            domain = e.field("domain"),
            certificate = e.field("certificate"),
            privateKey = e.field("private_key"),
            id = e.id
        )
    }
}

data class FileEntry(
    val id: String,
    @SerializedName("file_name") val fileName: String,
    @SerializedName("mime_type") val mimeType: String,
    @SerializedName("total_size") val totalSize: Long,
    @SerializedName("manifest_block_id") val manifestBlockId: String,
    @SerializedName("folder_id") val folderId: String = ""
)

data class FolderItem(
    val id: String,
    val name: String,
    val type: String
)

data class FolderListing(
    val folders: List<FolderItem> = emptyList(),
    val files: List<FileEntry> = emptyList()
)

data class UploadProgress(
    val bytesSent: Long,
    val totalBytes: Long
) {
    val percent: Double
        get() = if (totalBytes > 0) bytesSent.toDouble() * 100.0 / totalBytes.toDouble() else 100.0
}

data class Workspace(
    val id: String,
    val name: String,
    @SerializedName("is_default") val isDefault: Boolean = false
)

data class SyncPeer(
    @SerializedName("device_id") val deviceId: String,
    val host: String,
    val port: Int,
    @SerializedName("seen_at") val seenAt: Long,
    val reachable: Boolean? = null
)

data class SyncStatus(
    val peers: List<SyncPeer> = emptyList(),
    @SerializedName("last_sync_at") val lastSyncAt: Long = 0,
    @SerializedName("device_id") val deviceId: String = ""
)

data class AuditEvent(
    val timestamp: Long,
    val level: String,
    val module: String,
    val message: String,
    @SerializedName("subject_id") val subjectId: String? = null
)

data class VaultStatus(
    val initialized: Boolean = false,
    val unlocked: Boolean = false,
    val status: String = ""
)
