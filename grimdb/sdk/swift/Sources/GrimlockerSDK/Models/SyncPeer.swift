import Foundation

public struct SyncPeer: Codable, Identifiable, Sendable {
    public var id: String { deviceId }
    public let deviceId: String
    public let host: String
    public let port: Int
    public let seenAt: Int64
    public let reachable: Bool?

    enum CodingKeys: String, CodingKey {
        case deviceId = "device_id"
        case host
        case port
        case seenAt = "seen_at"
        case reachable
    }
}
