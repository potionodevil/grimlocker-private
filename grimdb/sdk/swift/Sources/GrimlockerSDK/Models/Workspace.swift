import Foundation

public struct Workspace: Codable, Identifiable, Sendable {
    public let id: String
    public let name: String
    public let isDefault: Bool

    enum CodingKeys: String, CodingKey {
        case id, name
        case isDefault = "is_default"
    }
}
