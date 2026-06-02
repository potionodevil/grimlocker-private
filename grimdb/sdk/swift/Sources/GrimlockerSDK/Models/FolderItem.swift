import Foundation

public struct FolderItem: Codable, Identifiable, Sendable {
    public let id: String
    public let name: String
    public let kind: String

    enum CodingKeys: String, CodingKey {
        case id, name
        case kind = "type"
    }
}
