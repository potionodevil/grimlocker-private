import Foundation

/// Actor-based Grimlocker client — thread-safe by Swift's actor model.
///
/// ```swift
/// let client = GrimlockerClient(baseURL: URL(string: "http://127.0.0.1:36353")!,
///                               token: ProcessInfo.processInfo.environment["GRIMLOCKER_TOKEN"]!)
/// try await client.unlockVault(password: "master-password")
/// let passwords = try await client.listPasswords()
/// ```
public actor GrimlockerClient {
    private let baseURL: URL
    private let session: URLSession
    private let decoder = JSONDecoder()
    private let encoder = JSONEncoder()

    public init(baseURL: URL, token: String, timeout: TimeInterval = 30) {
        self.baseURL = baseURL
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = timeout
        config.httpAdditionalHeaders = [
            "X-Grimlocker-Token": token,
            "Content-Type": "application/json",
        ]
        self.session = URLSession(configuration: config)
    }

    // ── Auth ──────────────────────────────────────────────────────────────────

    public func unlockVault(password: String) async throws {
        try await call("vault.unlock", payload: ["password": password])
    }

    public func lockVault() async throws {
        try await call("vault.logout", payload: [:] as [String: String])
    }

    public func vaultStatus() async throws -> VaultStatus {
        try await call("vault.status", payload: [:] as [String: String])
    }

    // ── Entries ───────────────────────────────────────────────────────────────

    public func listEntries(category: String? = nil) async throws -> [VaultEntry] {
        if let cat = category {
            return try await call("entry.query", payload: ["category": cat])
        }
        return try await call("entry.list", payload: [:] as [String: String])
    }

    public func getEntry(id: String) async throws -> VaultEntry {
        try await call("entry.read", payload: ["id": id])
    }

    public func createEntry(title: String, category: String, fields: [String: String]) async throws -> VaultEntry {
        try await call("entry.create", payload: ["title": title, "category": category, "fields": fields])
    }

    public func updateEntry(id: String, fields: [String: String]) async throws {
        try await call("entry.update", payload: ["id": id, "fields": fields])
    }

    public func deleteEntry(id: String) async throws {
        try await call("entry.delete", payload: ["id": id])
    }

    public func searchEntries(query: String, category: String? = nil) async throws -> [VaultEntry] {
        var p: [String: String] = ["query": query]
        if let c = category { p["category"] = c }
        return try await call("entry.search", payload: p)
    }

    // ── Typed helpers ─────────────────────────────────────────────────────────

    public func listPasswords() async throws -> [PasswordEntry] {
        try await listEntries(category: "PASSWORD").map(PasswordEntry.from)
    }

    public func createPassword(_ p: PasswordEntry) async throws -> String {
        try await createEntry(title: p.title, category: "PASSWORD", fields: p.fields).id
    }

    public func listSshKeys() async throws -> [SshKeyEntry] {
        try await listEntries(category: "SSH_KEY").map(SshKeyEntry.from)
    }

    public func createSshKey(_ k: SshKeyEntry) async throws -> String {
        try await createEntry(title: k.title, category: "SSH_KEY", fields: k.fields).id
    }

    public func listCertificates() async throws -> [CertificateEntry] {
        try await listEntries(category: "CERTIFICATE").map(CertificateEntry.from)
    }

    public func createCertificate(_ c: CertificateEntry) async throws -> String {
        try await createEntry(title: c.title, category: "CERTIFICATE", fields: c.fields).id
    }

    // ── File Vault ────────────────────────────────────────────────────────────

    public func listFolder(folderID: String = "") async throws -> FolderListing {
        try await call("file.list_folder", payload: ["folder_id": folderID])
    }

    public func createFolder(name: String, parentID: String = "") async throws -> FolderItem {
        try await call("file.create_folder", payload: ["name": name, "parent_id": parentID])
    }

    public func renameFolder(id: String, name: String) async throws {
        try await call("file.rename_folder", payload: ["id": id, "name": name])
    }

    public func deleteFolder(id: String) async throws {
        try await call("file.delete_folder", payload: ["id": id])
    }

    public func moveFile(manifestBlockID: String, folderID: String) async throws {
        try await call("file.move", payload: ["manifest_block_id": manifestBlockID, "folder_id": folderID])
    }

    public func uploadFile(
        _ data: Data, fileName: String,
        mimeType: String = "application/octet-stream",
        folderID: String = "",
        onProgress: ((UploadProgress) -> Void)? = nil
    ) async throws -> FileEntry {
        onProgress?(UploadProgress(bytesSent: 0, totalBytes: Int64(data.count)))
        let entry: FileEntry = try await call("file.ingest", payload: [
            "file_name": fileName, "mime_type": mimeType,
            "folder_id": folderID, "data_b64": data.base64EncodedString(),
        ])
        onProgress?(UploadProgress(bytesSent: Int64(data.count), totalBytes: Int64(data.count)))
        return entry
    }

    public func downloadFile(manifestBlockID: String) async throws -> Data {
        struct Resp: Decodable { let dataB64: String; enum CodingKeys: String, CodingKey { case dataB64 = "data_b64" } }
        let r: Resp = try await call("file.download", payload: ["manifest_block_id": manifestBlockID])
        guard let d = Data(base64Encoded: r.dataB64) else { throw GrimlockerError.decodingError("base64 decode failed") }
        return d
    }

    // ── Workspaces ────────────────────────────────────────────────────────────

    public func listWorkspaces() async throws -> [Workspace] {
        try await call("workspace.list", payload: [:] as [String: String])
    }

    public func createWorkspace(name: String) async throws -> Workspace {
        try await call("workspace.create", payload: ["name": name])
    }

    public func switchWorkspace(id: String) async throws {
        try await call("workspace.switch", payload: ["id": id])
    }

    public func renameWorkspace(id: String, name: String) async throws {
        try await call("workspace.rename", payload: ["id": id, "name": name])
    }

    public func deleteWorkspace(id: String) async throws {
        try await call("workspace.delete", payload: ["id": id])
    }

    // ── Sync ──────────────────────────────────────────────────────────────────

    public func listSyncPeers() async throws -> SyncStatus {
        try await call("sync.list_peers", payload: [:] as [String: String])
    }

    public func triggerSync() async throws {
        try await call("sync.trigger", payload: [:] as [String: String])
    }

    // ── Audit ─────────────────────────────────────────────────────────────────

    public func listAuditEvents(n: Int = 50) async throws -> [AuditEvent] {
        try await call("audit.list", payload: ["n": n])
    }

    // ── Health ────────────────────────────────────────────────────────────────

    public func healthCheck() async throws -> VaultStatus { try await vaultStatus() }

    // ── Internal ─────────────────────────────────────────────────────────────

    @discardableResult
    private func call<Payload: Encodable, Response: Decodable>(
        _ action: String, payload: Payload
    ) async throws -> Response {
        var req = URLRequest(url: baseURL.appendingPathComponent("api/v1"))
        req.httpMethod = "POST"
        req.httpBody = try encoder.encode(["action": AnyEncodable(action), "payload": AnyEncodable(payload)])

        let (data, resp) = try await session.data(for: req)
        guard let http = resp as? HTTPURLResponse else { throw GrimlockerError.network("non-HTTP response") }

        if http.statusCode < 200 || http.statusCode >= 300 {
            if let err = try? decoder.decode(APIError.self, from: data) {
                throw GrimlockerError.daemon(code: err.errorCode ?? 0, message: err.error ?? "HTTP \(http.statusCode)")
            }
            throw GrimlockerError.network("HTTP \(http.statusCode)")
        }
        return try decoder.decode(Response.self, from: data)
    }

    @discardableResult
    private func call<Payload: Encodable>(_ action: String, payload: Payload) async throws -> Void {
        let _: EmptyResponse = try await call(action, payload: payload)
    }

    private struct APIError: Decodable {
        let error: String?
        let errorCode: Int?
        enum CodingKeys: String, CodingKey { case error; case errorCode = "error_code" }
    }
    private struct EmptyResponse: Decodable {}
}

// MARK: - Error

public enum GrimlockerError: Error, LocalizedError {
    case network(String)
    case daemon(code: Int, message: String)
    case decodingError(String)

    public var errorDescription: String? {
        switch self {
        case .network(let m):            return "Network error: \(m)"
        case .daemon(let c, let m):      return "Daemon error (\(c)): \(m)"
        case .decodingError(let m):      return "Decoding error: \(m)"
        }
    }
}

// Type-erased Encodable for mixed-type dictionaries
private struct AnyEncodable: Encodable {
    let value: Any
    init(_ v: Any) { value = v }
    func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        switch value {
        case let v as String:  try c.encode(v)
        case let v as Int:     try c.encode(v)
        case let v as Bool:    try c.encode(v)
        case let v as [String: String]: try c.encode(v)
        case let v as [String: Int]:    try c.encode(v)
        case let v as Encodable:
            let wrapper = EncodableWrapper(v)
            try wrapper.encode(to: encoder)
        default: try c.encodeNil()
        }
    }
}
private struct EncodableWrapper: Encodable {
    let base: Encodable
    init(_ base: Encodable) { self.base = base }
    func encode(to encoder: Encoder) throws { try base.encode(to: encoder) }
}
