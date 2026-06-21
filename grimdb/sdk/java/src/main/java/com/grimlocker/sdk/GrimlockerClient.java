package com.grimlocker.sdk;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.grimlocker.sdk.internal.ErrorCode;
import com.grimlocker.sdk.internal.GQLFrame;
import com.grimlocker.sdk.model.*;
import org.java_websocket.client.WebSocketClient;
import org.java_websocket.handshake.ServerHandshake;

import java.net.URI;
import java.nio.ByteBuffer;
import java.util.*;
import java.util.concurrent.*;

/**
 * Grimlocker Java SDK — high-level client for the GQL binary protocol.
 *
 * <h2>Quick Start</h2>
 * <pre>{@code
 * try (GrimlockerClient client = GrimlockerClient.connect("127.0.0.1", 41753, token)) {
 *
 *     // List all entries
 *     List<Entry> entries = client.listEntries()
 *         .namespace("default")
 *         .limit(20)
 *         .execute();
 *
 *     // Create a password entry
 *     Entry created = client.createEntry()
 *         .namespace("default")
 *         .title("GitHub")
 *         .category("PASSWORD")
 *         .field("username", "alice")
 *         .field("password", "s3cr3t")
 *         .field("url", "https://github.com")
 *         .executeOne();
 * }
 * }</pre>
 *
 * <p>All queries are binary-encoded — no injection risk. The wire protocol is
 * identical to the Go/Python SDKs.
 */
public class GrimlockerClient implements AutoCloseable {

    private static final ObjectMapper JSON = new ObjectMapper();
    private static final int TIMEOUT_MS = 30_000;

    private final InternalWSClient ws;

    private int consecutiveFailures = 0;
    private long circuitOpenUntil = 0L;
    private final Object circuitLock = new Object();

    private GrimlockerClient(InternalWSClient ws) {
        this.ws = ws;
    }

    /**
     * Connects to the Grimlocker daemon and returns a ready-to-use client.
     *
     * @param host  daemon host (e.g. "127.0.0.1")
     * @param port  daemon WebSocket port
     * @param token session token printed by the daemon on startup
     * @throws GrimlockerException on connection failure
     */
    public static GrimlockerClient connect(String host, int port, String token) {
        String uri = "ws://" + host + ":" + port + "/ws?token=" + token;
        try {
            InternalWSClient ws = new InternalWSClient(new URI(uri));
            if (!ws.connectBlocking(10, TimeUnit.SECONDS)) {
                throw new GrimlockerException("connection timed out to " + uri);
            }
            ws.pendingResponse.poll(5, TimeUnit.SECONDS);
            return new GrimlockerClient(ws);
        } catch (GrimlockerException e) {
            throw e;
        } catch (Exception e) {
            throw new GrimlockerException("connect failed: " + e.getMessage(), e);
        }
    }

    /** Returns a builder for list_entries queries. */
    public QueryBuilder listEntries() {
        return new QueryBuilder(this, "list_entries");
    }

    /** Returns a builder for get_entry queries. */
    public QueryBuilder getEntry(String entryId) {
        return new QueryBuilder(this, "get_entry").entryId(entryId);
    }

    /** Returns a builder for query_entries (category filter) queries. */
    public QueryBuilder queryEntries(String category) {
        return new QueryBuilder(this, "query_entries").category(category);
    }

    /** Returns a builder for create_entry mutations. */
    public QueryBuilder createEntry() {
        return new QueryBuilder(this, "create_entry");
    }

    /** Returns a builder for update_entry mutations. Requires entryId() to be set. */
    public QueryBuilder updateEntry(String entryId) {
        return new QueryBuilder(this, "update_entry").entryId(entryId);
    }

    /** Deletes an entry by ID. */
    public void deleteEntry(String namespace, String entryId) {
        executeQuery("delete_entry", namespace, entryId, "", "", Collections.emptyMap(), 0, 0);
    }

    /** Creates multiple entries in a batch and returns their IDs. */
    public List<String> createEntriesBatch(String namespace, List<BatchEntryInput> inputs) {
        List<String> ids = new ArrayList<>(inputs.size());
        for (BatchEntryInput input : inputs) {
            List<Entry> results = createEntry()
                .namespace(namespace)
                .title(input.title)
                .category(input.category)
                .fields(input.fields)
                .execute();
            if (!results.isEmpty()) {
                ids.add(results.get(0).id);
            }
        }
        return ids;
    }

    /** Deletes multiple entries in a batch. */
    public void deleteEntriesBatch(String namespace, List<String> entryIds) {
        for (String entryId : entryIds) {
            deleteEntry(namespace, entryId);
        }
    }

    // --- Typed helpers ---

    public List<PasswordEntry> listPasswords(String namespace) {
        QueryBuilder qb = queryEntries("PASSWORD").namespace(namespace);
        List<Entry> raw = qb.execute();
        List<PasswordEntry> out = new ArrayList<>(raw.size());
        for (Entry e : raw) out.add(PasswordEntry.fromEntry(e));
        return out;
    }

    public String createPassword(String namespace, PasswordEntry p) {
        List<Entry> results = createEntry()
            .namespace(namespace)
            .title(p.title != null ? p.title : "")
            .category("PASSWORD")
            .fields(p.toFields())
            .execute();
        if (results.isEmpty()) throw new GrimlockerException("createPassword returned no entry");
        return results.get(0).id;
    }

    public List<SshKeyEntry> listSshKeys(String namespace) {
        QueryBuilder qb = queryEntries("SSH_KEY").namespace(namespace);
        List<Entry> raw = qb.execute();
        List<SshKeyEntry> out = new ArrayList<>(raw.size());
        for (Entry e : raw) out.add(SshKeyEntry.fromEntry(e));
        return out;
    }

    public String createSshKey(String namespace, SshKeyEntry k) {
        List<Entry> results = createEntry()
            .namespace(namespace)
            .title(k.title != null ? k.title : "")
            .category("SSH_KEY")
            .fields(k.toFields())
            .execute();
        if (results.isEmpty()) throw new GrimlockerException("createSshKey returned no entry");
        return results.get(0).id;
    }

    public List<CertificateEntry> listCertificates(String namespace) {
        QueryBuilder qb = queryEntries("CERTIFICATE").namespace(namespace);
        List<Entry> raw = qb.execute();
        List<CertificateEntry> out = new ArrayList<>(raw.size());
        for (Entry e : raw) out.add(CertificateEntry.fromEntry(e));
        return out;
    }

    public String createCertificate(String namespace, CertificateEntry c) {
        List<Entry> results = createEntry()
            .namespace(namespace)
            .title(c.title != null ? c.title : "")
            .category("CERTIFICATE")
            .fields(c.toFields())
            .execute();
        if (results.isEmpty()) throw new GrimlockerException("createCertificate returned no entry");
        return results.get(0).id;
    }

    /**
     * Searches entries by a text query filtered by category.
     *
     * @param namespace workspace namespace
     * @param query     search text (matched against title and fields)
     * @param category  category filter (empty for all categories)
     * @return matching entries
     */
    public List<Entry> searchEntries(String namespace, String query, String category) {
        Map<String, String> fields = new HashMap<>();
        if (query != null && !query.isEmpty()) fields.put("search", query);
        return executeQuery("search_entries", namespace, "", category != null ? category : "", "",
                            fields, 50, 0);
    }

    // --- File Vault ---

    /**
     * Lists the contents of a folder in the File Vault.
     *
     * @param folderId folder ID, or empty string for root
     */
    public FolderListing listFolder(String folderId) {
        String json = "{\"folder_id\":\"" + (folderId != null ? folderId : "") + "\"}";
        String resp = executeJsonCommand("file.list_folder", "default", json);
        try {
            return JSON.readValue(resp, FolderListing.class);
        } catch (Exception e) {
            throw new GrimlockerException("parse FolderListing failed: " + e.getMessage(), e);
        }
    }

    /**
     * Creates a new folder in the File Vault.
     *
     * @param name     folder name
     * @param parentId parent folder ID, or empty string for root
     * @return the created folder
     */
    public FolderItem createFolder(String name, String parentId) {
        Map<String, String> payload = new HashMap<>();
        payload.put("name", name != null ? name : "");
        payload.put("parent_id", parentId != null ? parentId : "");
        String json = toJsonString(payload);
        String resp = executeJsonCommand("file.create_folder", "default", json);
        try {
            return JSON.readValue(resp, FolderItem.class);
        } catch (Exception e) {
            throw new GrimlockerException("parse FolderItem failed: " + e.getMessage(), e);
        }
    }

    /** Renames a folder in the File Vault. */
    public void renameFolder(String folderId, String name) {
        Map<String, String> payload = new HashMap<>();
        payload.put("id", folderId != null ? folderId : "");
        payload.put("name", name != null ? name : "");
        executeJsonCommand("file.rename_folder", "default", toJsonString(payload));
    }

    /** Deletes a folder from the File Vault. */
    public void deleteFolder(String folderId) {
        String json = "{\"id\":\"" + (folderId != null ? folderId : "") + "\"}";
        executeJsonCommand("file.delete_folder", "default", json);
    }

    /** Moves a file to a different folder. */
    public void moveFile(String manifestBlockId, String folderId) {
        Map<String, String> payload = new HashMap<>();
        payload.put("manifest_block_id", manifestBlockId != null ? manifestBlockId : "");
        payload.put("folder_id", folderId != null ? folderId : "");
        executeJsonCommand("file.move", "default", toJsonString(payload));
    }

    /**
     * Uploads a file to the File Vault.
     *
     * @param namespace workspace namespace
     * @param data      file content bytes
     * @param fileName  file name
     * @param mimeType  MIME type
     * @param folderId  target folder ID, or empty for root
     * @return the created FileEntry
     */
    public FileEntry uploadFile(String namespace, byte[] data, String fileName, String mimeType, String folderId) {
        String encoded = Base64.getEncoder().encodeToString(data);
        Map<String, String> payload = new HashMap<>();
        payload.put("file_name", fileName != null ? fileName : "untitled");
        payload.put("mime_type", mimeType != null ? mimeType : "application/octet-stream");
        payload.put("data", encoded);
        payload.put("folder_id", folderId != null ? folderId : "");
        String json = toJsonString(payload);
        String resp = executeJsonCommand("file.ingest", namespace != null ? namespace : "default", json);
        try {
            return JSON.readValue(resp, FileEntry.class);
        } catch (Exception e) {
            throw new GrimlockerException("parse FileEntry failed: " + e.getMessage(), e);
        }
    }

    /**
     * Downloads a file from the File Vault.
     *
     * @param manifestBlockId the manifest block ID of the file to download
     * @return file content bytes
     */
    public byte[] downloadFile(String manifestBlockId) {
        String json = "{\"manifest_block_id\":\"" + (manifestBlockId != null ? manifestBlockId : "") + "\"}";
        String resp = executeJsonCommand("file.download", "default", json);
        try {
            JsonPayload wrapper = JSON.readValue(resp, JsonPayload.class);
            if (wrapper.data == null) return new byte[0];
            return Base64.getDecoder().decode(wrapper.data);
        } catch (Exception e) {
            throw new GrimlockerException("downloadFile failed: " + e.getMessage(), e);
        }
    }

    /** Gets the upload progress for a file ingestion operation. */
    public UploadProgress getUploadProgress(String manifestBlockId) {
        String json = "{\"manifest_block_id\":\"" + (manifestBlockId != null ? manifestBlockId : "") + "\"}";
        String resp = executeJsonCommand("file.upload_progress", "default", json);
        try {
            return JSON.readValue(resp, UploadProgress.class);
        } catch (Exception e) {
            throw new GrimlockerException("parse UploadProgress failed: " + e.getMessage(), e);
        }
    }

    // --- Workspaces ---

    /** Lists all available workspaces. */
    public List<Workspace> listWorkspaces() {
        String resp = executeJsonCommand("workspace.list", "default", "{}");
        try {
            return JSON.readValue(resp, new TypeReference<List<Workspace>>() {});
        } catch (Exception e) {
            throw new GrimlockerException("parse Workspace list failed: " + e.getMessage(), e);
        }
    }

    /** Creates a new workspace. */
    public Workspace createWorkspace(String name) {
        String json = "{\"name\":\"" + (name != null ? name : "") + "\"}";
        String resp = executeJsonCommand("workspace.create", "default", json);
        try {
            return JSON.readValue(resp, Workspace.class);
        } catch (Exception e) {
            throw new GrimlockerException("parse Workspace failed: " + e.getMessage(), e);
        }
    }

    /** Switches the active workspace to the given ID. */
    public void switchWorkspace(String id) {
        String json = "{\"id\":\"" + (id != null ? id : "") + "\"}";
        executeJsonCommand("workspace.switch", "default", json);
    }

    /** Renames a workspace. */
    public void renameWorkspace(String id, String name) {
        Map<String, String> payload = new HashMap<>();
        payload.put("id", id != null ? id : "");
        payload.put("name", name != null ? name : "");
        executeJsonCommand("workspace.rename", "default", toJsonString(payload));
    }

    /** Deletes a workspace by ID. */
    public void deleteWorkspace(String id) {
        String json = "{\"id\":\"" + (id != null ? id : "") + "\"}";
        executeJsonCommand("workspace.delete", "default", json);
    }

    // --- Sync + Audit ---

    /** Lists sync peers and current sync status. */
    public SyncStatus listSyncPeers() {
        String resp = executeJsonCommand("sync.list_peers", "default", "{}");
        try {
            return JSON.readValue(resp, SyncStatus.class);
        } catch (Exception e) {
            throw new GrimlockerException("parse SyncStatus failed: " + e.getMessage(), e);
        }
    }

    /** Triggers a full vault synchronization with connected peers. */
    public void triggerSync() {
        executeJsonCommand("sync.trigger", "default", "{}");
    }

    /** Lists the most recent audit events. */
    public List<AuditEvent> listAuditEvents(int n) {
        String json = "{\"n\":" + Math.max(1, n) + "}";
        String resp = executeJsonCommand("audit.list", "default", json);
        try {
            return JSON.readValue(resp, new TypeReference<List<AuditEvent>>() {});
        } catch (Exception e) {
            throw new GrimlockerException("parse AuditEvent list failed: " + e.getMessage(), e);
        }
    }

    // --- Health + Tools ---

    /** Sends a health check ping to the daemon. Returns "OK" on success. */
    public String healthCheck() {
        String resp = executeJsonCommand("system.health", "default", "{}");
        try {
            JsonPayload p = JSON.readValue(resp, JsonPayload.class);
            return p.status != null ? p.status : "UNKNOWN";
        } catch (Exception e) {
            return "ERROR: " + e.getMessage();
        }
    }

    /**
     * Generates a new Ed25519 SSH key pair, optionally saving the private key to the vault.
     *
     * @param comment     key comment (e.g. "user@host")
     * @param saveToVault if true, the private key is stored as a vault entry
     * @return JSON string with {public_key, fingerprint, entry_id}
     */
    public String generateSSHKey(String comment, boolean saveToVault) {
        Map<String, Object> payload = new HashMap<>();
        payload.put("comment", comment != null ? comment : "");
        payload.put("save_to_vault", saveToVault);
        return executeJsonCommand("tool.ssh_gen", "default", toJsonStringGeneric(payload));
    }

    /**
     * Retrieves the recovery phrase required to recreate the MVK.
     *
     * @param password the current vault password (for verification)
     * @return the BIP39 recovery phrase
     */
    public String getRecoveryPhrase(String password) {
        String json = "{\"password\":\"" + (password != null ? password : "") + "\"}";
        String resp = executeJsonCommand("tool.recovery_phrase", "default", json);
        try {
            JsonPayload p = JSON.readValue(resp, JsonPayload.class);
            return p.phrase != null ? p.phrase : "";
        } catch (Exception e) {
            throw new GrimlockerException("getRecoveryPhrase failed: " + e.getMessage(), e);
        }
    }

    // --- Builder pattern ---

    /** Returns a new Builder for creating a GrimlockerClient. */
    public static Builder builder() {
        return new Builder();
    }

    public static class Builder {
        private String host = "127.0.0.1";
        private int port = 41753;
        private String token;

        public Builder host(String host) { this.host = host; return this; }
        public Builder port(int port)   { this.port = port; return this; }
        public Builder token(String t)  { this.token = t; return this; }

        public GrimlockerClient connect() {
            return GrimlockerClient.connect(host, port, token);
        }
    }

    // --- Internal execution ---

    private byte[] sendFrameWithRetry(byte[] frame) {
        synchronized (circuitLock) {
            if (System.currentTimeMillis() < circuitOpenUntil) {
                throw new CircuitBreakerOpenException("circuit breaker is open");
            }
        }

        for (int attempt = 0; attempt < 4; attempt++) {
            byte[] response = null;
            boolean isNetworkError = false;

            try {
                ws.send(frame);
                response = ws.pendingResponse.poll(TIMEOUT_MS, TimeUnit.MILLISECONDS);
                if (response == null) {
                    isNetworkError = true;
                }
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw new GrimlockerException("interrupted while waiting for response", e);
            } catch (Exception e) {
                isNetworkError = true;
            }

            if (!isNetworkError && response != null) {
                synchronized (circuitLock) {
                    consecutiveFailures = 0;
                }
                return response;
            }

            if (attempt == 3) {
                synchronized (circuitLock) {
                    consecutiveFailures++;
                    if (consecutiveFailures >= 5) {
                        circuitOpenUntil = System.currentTimeMillis() + 30_000L;
                    }
                }
                throw new GrimlockerException("GQL request failed after " + attempt + " retries");
            }

            long delayMs = 100L * (1L << attempt);
            if (delayMs > 2000) delayMs = 2000;
            try {
                Thread.sleep(delayMs);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw new GrimlockerException("interrupted during retry delay", e);
            }
        }

        throw new GrimlockerException("retry exhaustion");
    }

    List<Entry> executeQuery(
            String operation,
            String namespace,
            String entryId,
            String category,
            String title,
            Map<String, String> fields,
            int limit,
            int offset) {

        byte[] frame = GQLFrame.encodeQuery(operation, namespace, entryId, category, title, fields, limit, offset);

        try {
            byte[] response = sendFrameWithRetry(frame);

            byte opcode = GQLFrame.readOpcode(response);
            byte[] jsonPayload = GQLFrame.readPayload(response);

            if (opcode == GQLFrame.OPCODE_ERROR) {
                GQLResultRaw err = JSON.readValue(jsonPayload, GQLResultRaw.class);
                throw new GrimlockerException(err.errorCode,
                    ErrorCode.nameOf(err.errorCode), err.errorMsg);
            }

            GQLResultRaw result = JSON.readValue(jsonPayload, GQLResultRaw.class);
            if (!result.success) {
                throw new GrimlockerException(result.errorCode,
                    ErrorCode.nameOf(result.errorCode), result.errorMsg);
            }
            return result.entries == null ? Collections.emptyList() : result.entries;

        } catch (GrimlockerException e) {
            throw e;
        } catch (Exception e) {
            throw new GrimlockerException("execute failed: " + e.getMessage(), e);
        }
    }

    /**
     * Sends an arbitrary JSON payload command through the GQL binary protocol.
     * Used for file vault, workspace, sync, audit, and tool operations.
     *
     * @return the raw JSON response body as a String
     */
    String executeJsonCommand(String operation, String namespace, String jsonPayload) {
        byte[] frame = GQLFrame.encodeJsonPayload(operation, namespace, jsonPayload);

        try {
            byte[] response = sendFrameWithRetry(frame);

            byte opcode = GQLFrame.readOpcode(response);
            byte[] jsonBody = GQLFrame.readPayload(response);

            if (opcode == GQLFrame.OPCODE_ERROR) {
                GQLResultRaw err = JSON.readValue(jsonBody, GQLResultRaw.class);
                throw new GrimlockerException(err.errorCode,
                    ErrorCode.nameOf(err.errorCode), err.errorMsg);
            }

            return new String(jsonBody, java.nio.charset.StandardCharsets.UTF_8);

        } catch (GrimlockerException e) {
            throw e;
        } catch (Exception e) {
            throw new GrimlockerException("executeJsonCommand failed: " + operation + " \u2014 " + e.getMessage(), e);
        }
    }

    private static String toJsonString(Map<String, String> map) {
        StringBuilder sb = new StringBuilder("{");
        boolean first = true;
        for (Map.Entry<String, String> e : map.entrySet()) {
            if (!first) sb.append(",");
            sb.append("\"").append(escapeJson(e.getKey())).append("\":\"")
              .append(escapeJson(e.getValue())).append("\"");
            first = false;
        }
        sb.append("}");
        return sb.toString();
    }

    private static String toJsonStringGeneric(Map<String, Object> map) {
        try {
            return JSON.writeValueAsString(map);
        } catch (Exception e) {
            throw new GrimlockerException("JSON marshal failed: " + e.getMessage(), e);
        }
    }

    private static String escapeJson(String s) {
        if (s == null) return "";
        return s.replace("\\", "\\\\")
                .replace("\"", "\\\"")
                .replace("\n", "\\n")
                .replace("\r", "\\r")
                .replace("\t", "\\t");
    }

    @Override
    public void close() {
        try {
            ws.closeBlocking();
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
    }

    // --- Internal types ---

    @JsonIgnoreProperties(ignoreUnknown = true)
    private static class GQLResultRaw {
        @JsonProperty("success")    public boolean success;
        @JsonProperty("entries")    public List<Entry> entries;
        @JsonProperty("error_code") public int errorCode;
        @JsonProperty("error_msg")  public String errorMsg = "";
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    private static class JsonPayload {
        @JsonProperty("data")   public String data;
        @JsonProperty("status") public String status;
        @JsonProperty("phrase") public String phrase;
    }

    private static class InternalWSClient extends WebSocketClient {
        final BlockingQueue<byte[]> pendingResponse = new LinkedBlockingQueue<>();

        InternalWSClient(URI uri) {
            super(uri);
        }

        @Override
        public void onOpen(ServerHandshake hs) {}

        @Override
        public void onMessage(String msg) {
            pendingResponse.offer(msg.getBytes());
        }

        @Override
        public void onMessage(ByteBuffer bytes) {
            byte[] arr = new byte[bytes.remaining()];
            bytes.get(arr);
            pendingResponse.offer(arr);
        }

        @Override
        public void onClose(int code, String reason, boolean remote) {}

        @Override
        public void onError(Exception e) {}
    }
}
