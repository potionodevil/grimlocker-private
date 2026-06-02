import XCTest
@testable import GrimlockerSDK

final class MockURLProtocol: URLProtocol {
    static var requestHandler: ((URLRequest) throws -> (HTTPURLResponse, Data))?

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func stopLoading() {}
    override func startLoading() {
        guard let handler = MockURLProtocol.requestHandler else {
            fatalError("requestHandler not set")
        }
        do {
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }
}

final class GrimlockerClientTests: XCTestCase {
    var client: GrimlockerClient!

    static let baseURL = URL(string: "http://127.0.0.1:36353")!

    override func setUp() {
        super.setUp()
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [MockURLProtocol.self]
        client = GrimlockerClient(baseURL: Self.baseURL, token: "test-token")
    }

    override func tearDown() {
        MockURLProtocol.requestHandler = nil
        super.tearDown()
    }

    private func stubResponse(status: Int = 200, body: Data) {
        MockURLProtocol.requestHandler = { _ in
            let response = HTTPURLResponse(
                url: Self.baseURL.appendingPathComponent("api/v1"),
                statusCode: status,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            return (response, body)
        }
    }

    private func stubJSON(_ json: String, status: Int = 200) {
        stubResponse(status: status, body: json.data(using: .utf8)!)
    }

    private func encodeJSON(_ obj: Encodable) -> Data {
        try! JSONEncoder().encode(obj)
    }

    // ── Auth ──────────────────────────────────────────────────────────────────

    func testUnlockVault() async throws {
        stubJSON(#"{"success":true}"#)
        try await client.unlockVault(password: "master-password")
    }

    func testLockVault() async throws {
        stubJSON(#"{"success":true}"#)
        try await client.lockVault()
    }

    func testVaultStatus() async throws {
        stubJSON(#"{"initialized":true,"unlocked":true,"status":"ok"}"#)
        let status = try await client.vaultStatus()
        XCTAssertTrue(status.initialized)
        XCTAssertTrue(status.unlocked)
    }

    // ── Entries ───────────────────────────────────────────────────────────────

    func testListEntries() async throws {
        stubJSON(#"{"entries":[{"id":"e1","title":"Entry 1","category":"PASSWORD","created_at":1,"updated_at":2}],"total_count":1}"#)
        let entries = try await client.listEntries()
        XCTAssertEqual(entries.count, 1)
        XCTAssertEqual(entries[0].id, "e1")
    }

    func testListEntriesByCategory() async throws {
        stubJSON(#"{"entries":[]}"#)
        let entries = try await client.listEntries(category: "SSH_KEY")
        XCTAssertEqual(entries.count, 0)
    }

    func testGetEntry() async throws {
        stubJSON(#"{"id":"e1","title":"Entry","category":"PASSWORD"}"#)
        let entry = try await client.getEntry(id: "e1")
        XCTAssertEqual(entry.id, "e1")
        XCTAssertEqual(entry.title, "Entry")
    }

    func testCreateEntry() async throws {
        stubJSON(#"{"id":"new1","title":"New","category":"PASSWORD"}"#)
        let entry = try await client.createEntry(title: "New", category: "PASSWORD", fields: ["username": "alice"])
        XCTAssertEqual(entry.id, "new1")
    }

    func testUpdateEntry() async throws {
        stubJSON(#"{"success":true}"#)
        try await client.updateEntry(id: "e1", fields: ["notes": "updated"])
    }

    func testDeleteEntry() async throws {
        stubJSON(#"{"success":true}"#)
        try await client.deleteEntry(id: "e1")
    }

    func testSearchEntries() async throws {
        stubJSON(#"{"entries":[{"id":"e1","title":"GitHub","category":"PASSWORD"}],"total_count":1}"#)
        let results = try await client.searchEntries(query: "git")
        XCTAssertEqual(results.count, 1)
    }

    func testSearchEntriesWithCategory() async throws {
        stubJSON(#"{"entries":[]}"#)
        let results = try await client.searchEntries(query: "git", category: "SSH_KEY")
        XCTAssertEqual(results.count, 0)
    }

    // ── Typed helpers ─────────────────────────────────────────────────────────

    func testListPasswords() async throws {
        stubJSON(#"{"entries":[{"id":"p1","title":"GitHub","category":"PASSWORD","fields":{"username":"a","password":"b","url":"","notes":""}}],"total_count":1}"#)
        let passwords = try await client.listPasswords()
        XCTAssertEqual(passwords.count, 1)
        XCTAssertEqual(passwords[0].title, "GitHub")
    }

    func testCreatePassword() async throws {
        stubJSON(#"{"id":"p1","title":"GitHub","category":"PASSWORD"}"#)
        let p = PasswordEntry(title: "GitHub", username: "alice", password: "sec")
        let id = try await client.createPassword(p)
        XCTAssertEqual(id, "p1")
    }

    func testListSshKeys() async throws {
        stubJSON(#"{"entries":[{"id":"sk1","title":"Key","category":"SSH_KEY","fields":{"public_key":"pk","private_key":"","username":"","passphrase":"","comment":""}}],"total_count":1}"#)
        let keys = try await client.listSshKeys()
        XCTAssertEqual(keys.count, 1)
    }

    func testCreateSshKey() async throws {
        stubJSON(#"{"id":"sk1","title":"Key","category":"SSH_KEY"}"#)
        let k = SshKeyEntry(title: "Key", publicKey: "pk", privateKey: "priv", username: "")
        let id = try await client.createSshKey(k)
        XCTAssertEqual(id, "sk1")
    }

    func testListCertificates() async throws {
        stubJSON(#"{"entries":[{"id":"c1","title":"Cert","category":"CERTIFICATE","fields":{"domain":"ex.com","certificate":"crt","private_key":"key"}}],"total_count":1}"#)
        let certs = try await client.listCertificates()
        XCTAssertEqual(certs.count, 1)
    }

    func testCreateCertificate() async throws {
        stubJSON(#"{"id":"c1","title":"Cert","category":"CERTIFICATE"}"#)
        let c = CertificateEntry(title: "Cert", domain: "ex.com", certificate: "crt", privateKey: "key")
        let id = try await client.createCertificate(c)
        XCTAssertEqual(id, "c1")
    }

    // ── File Vault ────────────────────────────────────────────────────────────

    func testListFolder() async throws {
        stubJSON(#"{"folders":[{"id":"d1","name":"sub","type":"folder"}],"files":[{"id":"f1","file_name":"a.txt","mime_type":"text/plain","total_size":10,"manifest_block_id":"mb1","folder_id":""}]}"#)
        let listing = try await client.listFolder()
        XCTAssertEqual(listing.folders.count, 1)
        XCTAssertEqual(listing.files.count, 1)
    }

    func testCreateFolder() async throws {
        stubJSON(#"{"id":"f1","name":"Notes"}"#)
        let folder = try await client.createFolder(name: "Notes", parentID: "p1")
        XCTAssertEqual(folder.name, "Notes")
    }

    func testUploadFile() async throws {
        stubJSON(#"{"id":"f1","file_name":"doc.txt","mime_type":"text/plain","total_size":5,"manifest_block_id":"mb1","folder_id":""}"#)
        let data = "hello".data(using: .utf8)!
        let result = try await client.uploadFile(data: data, fileName: "doc.txt", mimeType: "text/plain")
        XCTAssertEqual(result.fileName, "doc.txt")
    }

    func testDownloadFile() async throws {
        let b64 = Data("hello".utf8).base64EncodedString()
        stubJSON(#"{"data_b64":"\#(b64)"}"#)
        let data = try await client.downloadFile(manifestBlockID: "mb1")
        XCTAssertEqual(data.count, 5)
    }

    // ── Workspaces ────────────────────────────────────────────────────────────

    func testListWorkspaces() async throws {
        stubJSON(#"{"workspaces":[{"id":"ws1","name":"Personal","is_default":true}]}"#)
        let workspaces = try await client.listWorkspaces()
        XCTAssertEqual(workspaces.count, 1)
        XCTAssertEqual(workspaces[0].name, "Personal")
    }

    func testCreateWorkspace() async throws {
        stubJSON(#"{"id":"ws2","name":"Work"}"#)
        let ws = try await client.createWorkspace(name: "Work")
        XCTAssertEqual(ws.name, "Work")
    }

    // ── Sync ──────────────────────────────────────────────────────────────────

    func testListSyncPeers() async throws {
        stubJSON(#"{"peers":[{"device_id":"d1","host":"192.168.1.5","port":36352,"seen_at":1}],"last_sync_at":0,"device_id":"d1"}"#)
        let status = try await client.listSyncPeers()
        XCTAssertEqual(status.peers.count, 1)
    }

    func testTriggerSync() async throws {
        stubJSON(#"{"success":true}"#)
        try await client.triggerSync()
    }

    // ── Audit ─────────────────────────────────────────────────────────────────

    func testListAuditEvents() async throws {
        stubJSON(#"{"events":[{"timestamp":1,"level":"INFO","module":"auth","message":"unlock","subject_id":""}]}"#)
        let events = try await client.listAuditEvents()
        XCTAssertEqual(events.count, 1)
        XCTAssertEqual(events[0].level, "INFO")
    }

    // ── Health ────────────────────────────────────────────────────────────────

    func testHealthCheck() async throws {
        stubJSON(#"{"status":"ok","daemon_version":"1.0.0","vault_initialized":true,"vault_unlocked":true}"#)
        let health = try await client.healthCheck()
        XCTAssertEqual(health.status, "ok")
    }

    // ── Tools ─────────────────────────────────────────────────────────────────

    func testGenerateSshKey() async throws {
        stubJSON(#"{"public_key":"ssh-ed25519 AAA","fingerprint":"SHA256:abc","entry_id":"e1"}"#)
        let result = try await client.generateSSHKey(comment: "test", saveToVault: true)
        XCTAssertTrue(result.publicKey.hasPrefix("ssh-ed25519"))
    }

    func testGetRecoveryPhrase() async throws {
        stubJSON(#"{"recovery_phrase":"abandon ability able about above absent"}"#)
        let phrase = try await client.getRecoveryPhrase(password: "master")
        XCTAssertTrue(phrase.hasPrefix("abandon"))
    }

    // ── Error handling ───────────────────────────────────────────────────────

    func testErrorHandlingUnauthorized() async throws {
        stubJSON(#"{"error":"unauthorized"}"#, status: 401)
        do {
            let _ = try await client.listEntries()
            XCTFail("expected error")
        } catch let error as GrimlockerError {
            XCTAssertEqual(error.statusCode, 401)
        }
    }

    func testErrorHandlingInternalError() async throws {
        stubJSON(#"{"error":"internal server error"}"#, status: 500)
        do {
            let _ = try await client.vaultStatus()
            XCTFail("expected error")
        } catch let error as GrimlockerError {
            XCTAssertEqual(error.statusCode, 500)
        }
    }
}
