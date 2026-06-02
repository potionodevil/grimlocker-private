import Foundation

public struct SyncStatus: Codable, Sendable {
    public let peers: [SyncPeer]
    public let lastSyncAt: Int64
    public let deviceId: String

    enum CodingKeys: String, CodingKey {
        case peers
        case lastSyncAt = "last_sync_at"
        case deviceId = "device_id"
    }
}
