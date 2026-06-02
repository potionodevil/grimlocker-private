import Foundation

public struct VaultEntry: Codable, Identifiable, Sendable {
    public let id: String
    public let title: String
    public let category: String
    public let fields: [String: String]?
    public let createdAt: Int64
    public let updatedAt: Int64

    enum CodingKeys: String, CodingKey {
        case id, title, category, fields
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }

    public func field(_ key: String) -> String { fields?[key] ?? "" }
}
