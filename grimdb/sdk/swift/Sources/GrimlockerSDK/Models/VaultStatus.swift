import Foundation

public struct VaultStatus: Codable, Sendable {
    public let initialized: Bool
    public let unlocked: Bool
    public let status: String

    enum CodingKeys: String, CodingKey {
        case initialized, unlocked, status
    }
}
