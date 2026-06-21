package com.grimlocker.sdk

import com.google.gson.Gson
import com.grimlocker.sdk.model.*
import org.junit.jupiter.api.*
import org.junit.jupiter.api.Assertions.*
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.OutputStream
import java.net.HttpURLConnection
import java.net.MalformedURLException
import java.net.URL

class GrimlockerClientTest {

    private lateinit var client: TestClient

    @BeforeEach
    fun setUp() {
        client = TestClient("http://127.0.0.1:36353", "test-token")
    }

    // ── Auth ──────────────────────────────────────────────────────────────────

    @Test
    fun testUnlockVault() {
        client.unlockVault("master-password")
        assertEquals("vault.unlock", client.lastAction)
        assertEquals("master-password", client.lastPayload["password"])
    }

    @Test
    fun testLockVault() {
        client.lockVault()
        assertEquals("vault.logout", client.lastAction)
    }

    @Test
    fun testVaultStatus() {
        val status = client.vaultStatus()
        assertTrue(status.initialized)
        assertTrue(status.unlocked)
    }

    // ── Entries ───────────────────────────────────────────────────────────────

    @Test
    fun testListEntries() {
        val entries = client.listEntries()
        assertEquals(2, entries.size)
        assertEquals("e1", entries[0].id)
        assertEquals("PASSWORD", entries[0].category)
    }

    @Test
    fun testListEntriesByCategory() {
        val entries = client.listEntries("PASSWORD")
        assertEquals(2, entries.size)
        assertEquals("entry.query", client.lastAction)
    }

    @Test
    fun testGetEntry() {
        val entry = client.getEntry("e1")
        assertEquals("e1", entry.id)
        assertEquals("Test Entry", entry.title)
    }

    @Test
    fun testGetEntryNotFound() {
        assertThrows(GrimlockerException::class.java) {
            client.getEntry("nonexistent")
        }
    }

    @Test
    fun testCreateEntry() {
        val entry = client.createEntry("New Entry", "PASSWORD", mapOf("username" to "alice"))
        assertEquals("new1", entry.id)
        assertEquals("PASSWORD", entry.category)
    }

    @Test
    fun testUpdateEntry() {
        client.updateEntry("e1", mapOf("notes" to "updated"))
        assertEquals("entry.update", client.lastAction)
        assertEquals("e1", client.lastPayload["id"])
    }

    @Test
    fun testDeleteEntry() {
        client.deleteEntry("e1")
        assertEquals("entry.delete", client.lastAction)
        assertEquals("e1", client.lastPayload["id"])
    }

    @Test
    fun createEntriesBatch() {
        val entries = listOf(
            mapOf("title" to "Entry 1", "category" to "PASSWORD", "fields" to mapOf("username" to "alice")),
            mapOf("title" to "Entry 2", "category" to "NOTE", "fields" to mapOf("content" to "hello"))
        )
        val ids = client.createEntriesBatch(entries)
        assertEquals(2, ids.size)
        assertEquals("new1", ids[0])
        assertEquals("new1", ids[1])
    }

    @Test
    fun deleteEntriesBatch() {
        client.deleteEntriesBatch(listOf("e1", "e2"))
        assertEquals(listOf("e1", "e2"), client.deletedIds)
    }

    @Test
    fun testSearchEntries() {
        val results = client.searchEntries("git")
        assertEquals(1, results.size)
        assertEquals("e1", results[0].id)
    }

    @Test
    fun testSearchEntriesWithCategory() {
        val results = client.searchEntries("git", "SSH_KEY")
        assertEquals(0, results.size)
    }

    // ── Typed helpers ─────────────────────────────────────────────────────────

    @Test
    fun testListPasswords() {
        val passwords = client.listPasswords()
        assertEquals(2, passwords.size)
        assertEquals("GitHub", passwords[0].title)
    }

    @Test
    fun testCreatePassword() {
        val p = PasswordEntry("GitHub", "alice", "s3cret", "", "")
        val id = client.createPassword(p)
        assertEquals("p1", id)
    }

    @Test
    fun testListSshKeys() {
        val keys = client.listSshKeys()
        assertEquals(1, keys.size)
        assertEquals("Key", keys[0].title)
    }

    @Test
    fun testCreateSshKey() {
        val k = SshKeyEntry("Key", "pubkey", "privkey", "", "")
        val id = client.createSshKey(k)
        assertEquals("sk1", id)
    }

    @Test
    fun testListCertificates() {
        val certs = client.listCertificates()
        assertEquals(1, certs.size)
        assertEquals("Cert", certs[0].title)
    }

    @Test
    fun testCreateCertificate() {
        val c = CertificateEntry("Cert", "ex.com", "crt", "key")
        val id = client.createCertificate(c)
        assertEquals("c1", id)
    }

    // ── File Vault ────────────────────────────────────────────────────────────

    @Test
    fun testListFolder() {
        val listing = client.listFolder("")
        assertNotNull(listing)
    }

    @Test
    fun testListFolderById() {
        val listing = client.listFolder("folder1")
        assertNotNull(listing)
        assertEquals("folder1", client.lastPayload["folder_id"])
    }

    @Test
    fun testCreateFolder() {
        val folder = client.createFolder("Notes", "parent1")
        assertEquals("Notes", folder.name)
        assertEquals("f1", folder.id)
    }

    @Test
    fun testUploadFile() {
        val result = client.uploadFile("hello".toByteArray(), "doc.txt", "text/plain", "")
        assertEquals("doc.txt", result.fileName)
        assertEquals("f1", result.id)
    }

    @Test
    fun testDownloadFile() {
        val data = client.downloadFile("mb1")
        assertEquals("downloaded", String(data))
    }

    // ── Workspaces ────────────────────────────────────────────────────────────

    @Test
    fun testListWorkspaces() {
        val workspaces = client.listWorkspaces()
        assertEquals(1, workspaces.size)
        assertEquals("Personal", workspaces[0].name)
    }

    @Test
    fun testCreateWorkspace() {
        val ws = client.createWorkspace("Work")
        assertEquals("Work", ws.name)
        assertEquals("ws2", ws.id)
    }

    // ── Sync ──────────────────────────────────────────────────────────────────

    @Test
    fun testListSyncPeers() {
        val status = client.listSyncPeers()
        assertEquals(1, status.peers.size)
    }

    @Test
    fun testTriggerSync() {
        client.triggerSync()
        assertEquals("sync.trigger", client.lastAction)
    }

    // ── Audit ─────────────────────────────────────────────────────────────────

    @Test
    fun testListAuditEvents() {
        val events = client.listAuditEvents(10)
        assertEquals(1, events.size)
        assertEquals("INFO", events[0].level)
    }

    // ── Health + Tools ────────────────────────────────────────────────────────

    @Test
    fun testHealthCheck() {
        val health = client.healthCheck()
        assertEquals("ok", health["status"])
    }

    @Test
    fun testGenerateSSHKey() {
        val result = client.generateSSHKey("test", true)
        assertTrue(result.containsKey("public_key"))
        assertTrue(result.containsKey("fingerprint"))
    }

    @Test
    fun testGetRecoveryPhrase() {
        val phrase = client.getRecoveryPhrase("master")
        assertEquals("abandon ability able about above absent absorb abstract...", phrase)
    }

    // ── Error handling ────────────────────────────────────────────────────────

    @Test
    fun testErrorHandlingLockedVault() {
        val client = ErrorTestClient()
        val ex = assertThrows(GrimlockerException::class.java) {
            client.listEntries()
        }
        assertTrue(ex.message?.contains("locked") == true)
    }

    @Test
    fun testErrorHandlingUnauthorized() {
        val client = ErrorTestClient()
        assertThrows(GrimlockerException::class.java) {
            client.getEntry("bad")
        }
    }

    @Test
    fun circuitBreakerOpens() {
        val cbClient = CircuitBreakerTestClient()
        repeat(5) {
            assertThrows(MalformedURLException::class.java) {
                cbClient.listEntries()
            }
        }
        assertThrows(CircuitBreakerOpenException::class.java) {
            cbClient.listEntries()
        }
    }

    // ── Test doubles ──────────────────────────────────────────────────────────

    class TestClient(private val baseUrl: String, private val token: String) : GrimlockerClient(baseUrl, token) {
        var lastAction: String = ""
        var lastPayload: Map<String, Any?> = emptyMap()

        private fun record(action: String, payload: Map<String, Any?>) {
            lastAction = action
            lastPayload = payload
        }

        override fun vaultStatus(): VaultStatus {
            record("vault.status", emptyMap())
            return VaultStatus(true, true, "ok")
        }

        override fun listEntries(category: String?): List<VaultEntry> {
            record(if (category != null) "entry.query" else "entry.list", if (category != null) mapOf("category" to category) else emptyMap())
            return listOf(
                VaultEntry("e1", "PASSWORD", "Entry One", mapOf("username" to "alice"), 1L, 2L),
                VaultEntry("e2", "SSH_KEY", "Entry Two", emptyMap(), 3L, 4L),
            )
        }

        override fun getEntry(id: String): VaultEntry {
            record("entry.read", mapOf("id" to id))
            if (id == "nonexistent") throw GrimlockerException("Entry not found: $id", -10)
            return VaultEntry(id, "PASSWORD", "Test Entry", mapOf("username" to "alice"), 1L, 2L)
        }

        override fun createEntry(title: String, category: String, fields: Map<String, String>): VaultEntry {
            record("entry.create", mapOf("title" to title, "category" to category, "fields" to fields))
            return VaultEntry("new1", category, title, fields, 10L, 20L)
        }

        override fun updateEntry(id: String, fields: Map<String, String>) {
            record("entry.update", mapOf("id" to id, "fields" to fields))
        }

        val deletedIds = mutableListOf<String>()

        override fun deleteEntry(id: String) {
            record("entry.delete", mapOf("id" to id))
            deletedIds.add(id)
        }

        override fun searchEntries(query: String, category: String?): List<VaultEntry> {
            val payload = mutableMapOf<String, Any?>("query" to query)
            if (category != null) payload["category"] = category
            record("entry.search", payload)
            if (category == "SSH_KEY") return emptyList()
            return listOf(VaultEntry("e1", "PASSWORD", "GitHub", emptyMap(), 1L, 2L))
        }

        override fun listPasswords(): List<PasswordEntry> = listOf(
            PasswordEntry("GitHub", "alice", "sec", "", ""),
            PasswordEntry("GitLab", "bob", "sec", "", ""),
        )

        override fun createPassword(p: PasswordEntry): String {
            record("entry.create", mapOf("title" to p.title, "category" to "PASSWORD", "fields" to p.toFields()))
            return "p1"
        }

        override fun listSshKeys(): List<SshKeyEntry> = listOf(
            SshKeyEntry("Key", "pub", "priv", "", "")
        )

        override fun createSshKey(k: SshKeyEntry): String {
            record("entry.create", mapOf("title" to k.title, "category" to "SSH_KEY", "fields" to k.toFields()))
            return "sk1"
        }

        override fun listCertificates(): List<CertificateEntry> = listOf(
            CertificateEntry("Cert", "ex.com", "crt", "key")
        )

        override fun createCertificate(c: CertificateEntry): String {
            record("entry.create", mapOf("title" to c.title, "category" to "CERTIFICATE", "fields" to c.toFields()))
            return "c1"
        }

        override fun listFolder(folderId: String): FolderListing {
            record("file.list_folder", mapOf("folder_id" to folderId))
            return FolderListing(
                listOf(FolderItem("d1", "sub", "folder")),
                listOf(FileEntry("f1", "a.txt", "text/plain", 10, "mb1", ""))
            )
        }

        override fun createFolder(name: String, parentId: String): FolderItem {
            record("file.create_folder", mapOf("name" to name, "parent_id" to parentId))
            return FolderItem("f1", name, "folder")
        }

        override fun uploadFile(data: ByteArray, fileName: String, mimeType: String, folderId: String): FileEntry {
            record("file.ingest", mapOf("file_name" to fileName, "mime_type" to mimeType, "folder_id" to folderId))
            return FileEntry("f1", fileName, mimeType, data.size.toLong(), "mb1", folderId)
        }

        override fun downloadFile(manifestBlockId: String): ByteArray {
            record("file.download", mapOf("manifest_block_id" to manifestBlockId))
            return "downloaded".toByteArray()
        }

        override fun listWorkspaces(): List<Workspace> = listOf(
            Workspace("ws1", "Personal", true, 1L)
        )

        override fun createWorkspace(name: String): Workspace {
            record("workspace.create", mapOf("name" to name))
            return Workspace("ws2", name, false, 2L)
        }

        override fun listSyncPeers(): SyncStatus = SyncStatus(
            listOf(SyncPeer("d1", "peer1", "192.168.1.5", 36352, 1L, null, true)),
            0L, "d1"
        )

        override fun triggerSync() {
            record("sync.trigger", emptyMap())
        }

        override fun listAuditEvents(n: Int): List<AuditEvent> = listOf(
            AuditEvent(1L, "INFO", "auth", "vault unlocked", "", null, null)
        )

        override fun healthCheck(): Map<String, Any?> = mapOf(
            "status" to "ok", "daemon_version" to "1.0.0",
            "vault_initialized" to true, "vault_unlocked" to true
        )

        override fun generateSSHKey(comment: String, saveToVault: Boolean): Map<String, String> {
            record("tool.ssh_keygen", mapOf("comment" to comment, "save_to_vault" to saveToVault))
            return mapOf(
                "public_key" to "ssh-ed25519 AAA...",
                "fingerprint" to "SHA256:abc",
                "entry_id" to "e1"
            )
        }

        override fun getRecoveryPhrase(password: String): String? {
            record("vault.recovery_phrase", mapOf("password" to password))
            return "abandon ability able about above absent absorb abstract..."
        }

        override fun close() {}
    }

    class ErrorTestClient : GrimlockerClient("http://127.0.0.1:36353", "token") {

        override fun listEntries(category: String?): List<VaultEntry> {
            throw GrimlockerException("vault is locked", -101)
        }

        override fun getEntry(id: String): VaultEntry {
            throw GrimlockerException("unauthorized", -102)
        }

        override fun close() {}
    }

    class CircuitBreakerTestClient : GrimlockerClient("unknown://127.0.0.1:1", "token", timeoutMs = 100)
}
