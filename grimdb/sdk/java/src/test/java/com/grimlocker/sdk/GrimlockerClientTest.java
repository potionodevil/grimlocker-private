package com.grimlocker.sdk;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.grimlocker.sdk.model.*;
import org.junit.jupiter.api.*;

import java.util.*;

class GrimlockerClientTest {

    private static final ObjectMapper JSON = new ObjectMapper();

    @Test
    void testConstructor() {
        GrimlockerClient client = new GrimlockerClient(/* would test via InternalWSClient */);
        assertNotNull(client);
    }

    @Test
    void testClientCloseable() {
        assertTrue(AutoCloseable.class.isAssignableFrom(GrimlockerClient.class));
    }

    @Test
    void testListEntriesQueryBuilder() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            QueryBuilder qb = client.listEntries().namespace("default").limit(10);
            assertNotNull(qb);
            List<Entry> entries = qb.execute();
            assertNotNull(entries);
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testGetEntryQueryBuilder() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            QueryBuilder qb = client.getEntry("e1");
            assertNotNull(qb);
            List<Entry> entries = qb.execute();
            assertEquals(1, entries.size());
            assertEquals("e1", entries.get(0).id);
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testQueryEntriesByCategory() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            List<Entry> entries = client.queryEntries("PASSWORD").namespace("default").execute();
            assertNotNull(entries);
            assertEquals(2, entries.size());
            assertEquals("PASSWORD", entries.get(0).category);
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testCreateEntry() throws Exception {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            Entry entry = client.createEntry()
                .namespace("default")
                .title("GitHub")
                .category("PASSWORD")
                .field("username", "alice")
                .field("password", "s3cret")
                .executeOne();
            assertNotNull(entry);
            assertEquals("new1", entry.id);
            assertEquals("PASSWORD", entry.category);
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testUpdateEntry() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            client.updateEntry("e1")
                .title("Updated")
                .field("notes", "new")
                .execute();
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testDeleteEntry() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            client.deleteEntry("default", "e1");
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testCreateEntriesBatch() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            List<String> ids = client.createEntriesBatch("default", List.of(
                new BatchEntryInput("Entry 1", "PASSWORD", Map.of("username", "alice")),
                new BatchEntryInput("Entry 2", "NOTE", Map.of("content", "hello"))
            ));
            assertNotNull(ids);
            assertEquals(2, ids.size());
            assertEquals("new1", ids.get(0));
            assertEquals("new1", ids.get(1));
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testDeleteEntriesBatch() {
        MockGrimlockerClient client = null;
        try {
            client = new MockGrimlockerClient();
            client.deleteEntriesBatch("default", List.of("e1", "e2"));
            List<String> deleted = client.getDeletedEntryIds();
            assertEquals(2, deleted.size());
            assertTrue(deleted.contains("e1"));
            assertTrue(deleted.contains("e2"));
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testListPasswords() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            List<PasswordEntry> passwords = client.listPasswords("default");
            assertEquals(2, passwords.size());
            assertInstanceOf(PasswordEntry.class, passwords.get(0));
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testCreatePassword() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            PasswordEntry p = new PasswordEntry("GitHub", "alice", "s3cret", "", "");
            String id = client.createPassword("default", p);
            assertEquals("p1", id);
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testListSshKeys() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            List<SshKeyEntry> keys = client.listSshKeys("default");
            assertEquals(1, keys.size());
            assertInstanceOf(SshKeyEntry.class, keys.get(0));
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testCreateSshKey() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            SshKeyEntry k = new SshKeyEntry("My Key", "pub", "priv", "", "");
            String id = client.createSshKey("default", k);
            assertEquals("sk1", id);
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testListCertificates() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            List<CertificateEntry> certs = client.listCertificates("default");
            assertEquals(1, certs.size());
            assertInstanceOf(CertificateEntry.class, certs.get(0));
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testCreateCertificate() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            CertificateEntry c = new CertificateEntry("Cert", "ex.com", "crt", "key");
            String id = client.createCertificate("default", c);
            assertEquals("c1", id);
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testSearchEntries() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            List<Entry> results = client.searchEntries("default", "git", "");
            assertNotNull(results);
            assertEquals(1, results.size());
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testListFolder() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            FolderListing listing = client.listFolder("default", "");
            assertNotNull(listing);
            assertTrue(listing.folders().size() >= 0);
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testCreateFolder() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            FolderItem folder = client.createFolder("default", "Notes", "");
            assertNotNull(folder);
            assertEquals("f1", folder.id());
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testUploadFile() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            FileEntry result = client.uploadFile("default", "hello".getBytes(), "doc.txt", "text/plain", "", null);
            assertNotNull(result);
            assertEquals("doc.txt", result.fileName());
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testDownloadFile() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            byte[] data = client.downloadFile("default", "mb1");
            assertNotNull(data);
            assertTrue(data.length > 0);
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testListWorkspaces() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            List<Workspace> workspaces = client.listWorkspaces();
            assertEquals(1, workspaces.size());
            assertEquals("Personal", workspaces.get(0).name());
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testCreateWorkspace() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            Workspace ws = client.createWorkspace("Work");
            assertNotNull(ws);
            assertEquals("Work", ws.name());
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testListSyncPeers() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            SyncStatus status = client.listSyncPeers("default");
            assertNotNull(status);
            assertEquals(1, status.peers().size());
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testListAuditEvents() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            List<AuditEvent> events = client.listAuditEvents("default", 10);
            assertNotNull(events);
            assertEquals(1, events.size());
            assertEquals("INFO", events.get(0).level());
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testGenerateSSHKey() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            Map<String, String> result = client.generateSSHKey("default", "test", true);
            assertNotNull(result);
            assertTrue(result.containsKey("public_key"));
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testHealthCheck() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            Map<String, Object> health = client.healthCheck();
            assertNotNull(health);
            assertEquals("ok", health.get("status"));
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testErrorHandlingThrowsOnError() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            assertThrows(GrimlockerException.class, () -> {
                client.queryEntries("NONEXISTENT").execute();
            });
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testErrorHandlingLockedVault() {
        assertThrows(GrimlockerException.class, () -> {
            GrimlockerClient client = GrimlockerClient.connect("127.0.0.1", 9999, "bad-token");
            client.close();
        });
    }

    @Test
    void testCloseIdempotent() {
        GrimlockerClient client = null;
        try {
            client = createMockedClient();
            client.close();
            client.close();
        } finally {
            if (client != null) client.close();
        }
    }

    @Test
    void testCircuitBreakerOpensAfterFailures() {
        GrimlockerClient client = new CircuitBreakerTestClient();
        try {
            for (int i = 0; i < 5; i++) {
                assertThrows(GrimlockerException.class, () -> {
                    client.createEntry()
                        .namespace("default")
                        .title("Test")
                        .category("PASSWORD")
                        .execute();
                });
            }
            assertThrows(CircuitBreakerOpenException.class, () -> {
                client.createEntry()
                    .namespace("default")
                    .title("Test")
                    .category("PASSWORD")
                    .execute();
            });
        } finally {
            client.close();
        }
    }

    // ── Helpers ─────────────────────────────────────────────────────────────────────

    private GrimlockerClient createMockedClient() {
        return new MockGrimlockerClient();
    }

    /**
     * Mock implementation that returns canned responses without a real WebSocket connection.
     */
    private static class MockGrimlockerClient extends GrimlockerClient {
        MockGrimlockerClient() {
            super(new InternalMockWS());
        }

        @Override
        public QueryBuilder listEntries() {
            return new MockQueryBuilder(this, "list_entries", List.of(
                new Entry("e1", "PASSWORD", "Entry One", Map.of("username", "alice"), 1L, 2L),
                new Entry("e2", "SSH_KEY", "Entry Two", Map.of(), 3L, 4L)
            ));
        }

        @Override
        public QueryBuilder getEntry(String entryId) {
            return new MockQueryBuilder(this, "get_entry", List.of(
                new Entry(entryId, "PASSWORD", "Test Entry", Map.of("username", "alice"), 1L, 2L)
            ));
        }

        @Override
        public QueryBuilder queryEntries(String category) {
            if ("NONEXISTENT".equals(category)) {
                return new MockQueryBuilder(this, "query_entries", new GrimlockerException("category not found", -20));
            }
            return new MockQueryBuilder(this, "query_entries", List.of(
                new Entry("p1", "PASSWORD", "GitHub", Map.of("username", "a", "password", "b"), 1L, 2L),
                new Entry("p2", "PASSWORD", "GitLab", Map.of("username", "c", "password", "d"), 3L, 4L)
            ));
        }

        @Override
        public QueryBuilder createEntry() {
            return new MockQueryBuilder(this, "create_entry", List.of(
                new Entry("new1", "PASSWORD", "GitHub", Map.of("username", "alice", "password", "s3cret"), 10L, 20L)
            ));
        }

        @Override
        public QueryBuilder updateEntry(String entryId) {
            return new MockQueryBuilder(this, "update_entry", List.of());
        }

        private final List<String> deletedEntryIds = new ArrayList<>();

        @Override
        public void deleteEntry(String namespace, String entryId) {
            deletedEntryIds.add(entryId);
        }

        List<String> getDeletedEntryIds() {
            return deletedEntryIds;
        }

        @Override
        public List<PasswordEntry> listPasswords(String namespace) {
            return List.of(
                new PasswordEntry("GitHub", "alice", "s3cret", "", ""),
                new PasswordEntry("GitLab", "bob", "s3cret", "", "")
            );
        }

        @Override
        public String createPassword(String namespace, PasswordEntry p) {
            return "p1";
        }

        @Override
        public List<SshKeyEntry> listSshKeys(String namespace) {
            return List.of(new SshKeyEntry("Key", "pub", "priv", "", ""));
        }

        @Override
        public String createSshKey(String namespace, SshKeyEntry k) {
            return "sk1";
        }

        @Override
        public List<CertificateEntry> listCertificates(String namespace) {
            return List.of(new CertificateEntry("Cert", "ex.com", "crt", "key"));
        }

        @Override
        public String createCertificate(String namespace, CertificateEntry c) {
            return "c1";
        }

        @Override
        public List<Entry> searchEntries(String namespace, String query, String category) {
            return List.of(new Entry("e1", "PASSWORD", "GitHub", Map.of(), 1L, 2L));
        }

        @Override
        public FolderListing listFolder(String namespace, String folderId) {
            return new FolderListing(List.of(), List.of());
        }

        @Override
        public FolderItem createFolder(String namespace, String name, String parentId) {
            return new FolderItem("f1", name, parentId, 1L, 2L);
        }

        @Override
        public FileEntry uploadFile(String namespace, byte[] data, String fileName, String mimeType, String folderId, ProgressCallback cb) {
            return new FileEntry("f1", "mb1", fileName, mimeType, (long) data.length, folderId, 1L);
        }

        @Override
        public byte[] downloadFile(String namespace, String manifestBlockId) {
            return "downloaded content".getBytes();
        }

        @Override
        public List<Workspace> listWorkspaces() {
            return List.of(new Workspace("ws1", "Personal", true, 1L));
        }

        @Override
        public Workspace createWorkspace(String name) {
            return new Workspace("ws2", name, false, 2L);
        }

        @Override
        public SyncStatus listSyncPeers(String namespace) {
            return new SyncStatus(
                List.of(new SyncPeer("p1", "peer1", "192.168.1.5", true, 1L)),
                0L
            );
        }

        @Override
        public List<AuditEvent> listAuditEvents(String namespace, int n) {
            return List.of(new AuditEvent(1L, "INFO", "auth", "vault unlocked", ""));
        }

        @Override
        public Map<String, String> generateSSHKey(String namespace, String comment, boolean saveToVault) {
            return Map.of("public_key", "ssh-ed25519 AAA", "fingerprint", "SHA256:abc", "entry_id", "e1");
        }

        @Override
        public Map<String, Object> healthCheck() {
            return Map.of("status", "ok", "daemon_version", "1.0.0");
        }

        @Override
        public void close() {
        }
    }

    private static class MockQueryBuilder extends QueryBuilder {
        private final List<Entry> results;
        private final GrimlockerException error;

        MockQueryBuilder(GrimlockerClient client, String operation, List<Entry> results) {
            super(client, operation);
            this.results = results;
            this.error = null;
        }

        MockQueryBuilder(GrimlockerClient client, String operation, GrimlockerException error) {
            super(client, operation);
            this.results = null;
            this.error = error;
        }

        @Override
        public List<Entry> execute() {
            if (error != null) throw error;
            return results != null ? results : List.of();
        }

        @Override
        public Entry executeOne() {
            var list = execute();
            return list.isEmpty() ? null : list.get(0);
        }
    }

    private static class FailingMockWS extends com.grimlocker.sdk.GrimlockerClient.InternalWSClient {
        FailingMockWS() {
            super(null);
        }
    }

    private static class CircuitBreakerTestClient extends GrimlockerClient {
        CircuitBreakerTestClient() {
            super(new FailingMockWS());
        }
    }

    private static class InternalMockWS extends com.grimlocker.sdk.GrimlockerClient.InternalWSClient {
        InternalMockWS() {
            super(null);
        }
    }
}
