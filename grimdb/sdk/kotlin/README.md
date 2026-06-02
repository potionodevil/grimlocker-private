# Grimlocker SDK — Kotlin

Kotlin/JVM SDK for the Grimlocker Zero-Trust Vault daemon. Uses `HttpURLConnection` and Gson — only one external dependency (Gson for JSON).

## Installation

### Gradle (Groovy DSL)

```groovy
repositories {
    mavenCentral()
}
dependencies {
    implementation 'com.grimlocker:grimlocker-sdk-kotlin:1.0.0'
}
```

### Gradle (Kotlin DSL)

```kotlin
repositories {
    mavenCentral()
}
dependencies {
    implementation("com.grimlocker:grimlocker-sdk-kotlin:1.0.0")
}
```

### Maven

```xml
<dependency>
    <groupId>com.grimlocker</groupId>
    <artifactId>grimlocker-sdk-kotlin</artifactId>
    <version>1.0.0</version>
</dependency>
```

## Requirements

- JVM 17+
- Kotlin 2.0.0+

## Quick Start

```kotlin
import com.grimlocker.sdk.GrimlockerClient
import com.grimlocker.sdk.model.*

fun main() {
    // The token is printed by the daemon at startup: GRIMLOCKER_TOKEN=...
    val client = GrimlockerClient("http://127.0.0.1:36353", System.getenv("GRIMLOCKER_TOKEN")!!)

    client.use { c ->
        // Unlock the vault
        c.unlockVault("master-password")

        // List all entries
        val entries = c.listEntries()
        println("${entries.size} entries found")

        // Create a password
        val id = c.createPassword(PasswordEntry(
            title = "GitHub",
            username = "me@example.com",
            password = "s3cr3t",
            url = "https://github.com"
        ))
        println("Created entry: $id")

        // List passwords
        val passwords = c.listPasswords()
        passwords.forEach { println("${it.title} — ${it.username}") }

        // Search entries
        val results = c.searchEntries("github")

        // Update entry fields
        c.updateEntry(id, mapOf("notes" to "Updated notes"))

        // Delete entry
        c.deleteEntry(id)

        // File vault operations
        val listing = c.listFolder()
        val folder = c.createFolder("documents")
        val file = c.uploadFile(
            data = "secret content".toByteArray(),
            fileName = "secret.txt",
            onProgress = { p -> println("${String.format("%.0f", p.percent)}%") }
        )
        val downloaded = c.downloadFile(file.manifestBlockId)
        c.moveFile(file.manifestBlockId, folder.id)
        c.deleteFolder(folder.id)

        // Workspaces
        val workspaces = c.listWorkspaces()
        val ws = c.createWorkspace("Personal")
        c.switchWorkspace(ws.id)
        c.renameWorkspace(ws.id, "Work")
        c.deleteWorkspace(ws.id)

        // Sync
        val syncStatus = c.listSyncPeers()
        println("Peers: ${syncStatus.peers.size}, device: ${syncStatus.deviceId}")
        c.triggerSync()

        // Audit log
        val events = c.listAuditEvents(20)
        events.forEach { println("[${it.level}] ${it.module}: ${it.message}") }

        // Vault status
        val status = c.vaultStatus()
        println("Initialized: ${status.initialized}, Unlocked: ${status.unlocked}")

        // Recovery phrase
        val phrase = c.getRecoveryPhrase("master-password")
        println("Recovery phrase: $phrase")

        // Lock
        c.lockVault()
    }
}
```

## Error Handling

```kotlin
import com.grimlocker.sdk.GrimlockerException

try {
    client.unlockVault("wrong-password")
} catch (e: GrimlockerException) {
    println("Error ${e.errorCode} (${GrimlockerException.nameOf(e.errorCode)}): ${e.message}")
    when (e.errorCode) {
        -101 -> println("Authentication required")
        -103 -> println("Invalid request")
        else -> println("Unknown error")
    }
}
```

## API Reference

### GrimlockerClient

| Method | Description |
|--------|-------------|
| `unlockVault(password)` | Unlock vault with master password |
| `lockVault()` | Lock the vault / logout |
| `vaultStatus()` | Get vault status (initialized, unlocked) |
| `getRecoveryPhrase(password)` | Get recovery phrase |
| `listEntries(category?)` | List all vault entries, optionally filtered by category |
| `getEntry(id)` | Read a single entry by ID |
| `createEntry(title, category, fields)` | Create a new vault entry |
| `updateEntry(id, fields)` | Update entry fields |
| `deleteEntry(id)` | Delete entry by ID |
| `searchEntries(query, category?)` | Search entries |
| `listPasswords()` | List password entries |
| `createPassword(p)` | Create a password entry, returns ID |
| `listSshKeys()` | List SSH key entries |
| `createSshKey(k)` | Create an SSH key entry, returns ID |
| `listCertificates()` | List certificate entries |
| `createCertificate(c)` | Create a certificate entry, returns ID |
| `listFolder(folderId?)` | List folder contents |
| `createFolder(name, parentId?)` | Create a subfolder |
| `renameFolder(id, name)` | Rename a folder |
| `deleteFolder(id)` | Delete a folder |
| `moveFile(manifestBlockId, folderId)` | Move a file to a folder |
| `uploadFile(data, fileName, ...)` | Upload a file to the vault |
| `downloadFile(manifestBlockId)` | Download a file from the vault |
| `listWorkspaces()` | List all workspaces |
| `createWorkspace(name)` | Create a new workspace |
| `switchWorkspace(id)` | Switch to another workspace |
| `renameWorkspace(id, name)` | Rename a workspace |
| `deleteWorkspace(id)` | Delete a workspace |
| `listSyncPeers()` | List sync peers and status |
| `triggerSync()` | Trigger an immediate sync cycle |
| `listAuditEvents(n?)` | Fetch last n audit events (default 50) |
| `healthCheck()` | Alias for `vaultStatus()` |

## Supported Languages

| Language | Package | Status |
|----------|---------|--------|
| C# / .NET | `Grimlocker.SDK` | ✅ Available |
| TypeScript | `@grimlocker/sdk` | ✅ Available |
| Swift | `GrimlockerSDK` | ✅ Available |
| Kotlin | `com.grimlocker:grimlocker-sdk-kotlin` | ✅ This package |
