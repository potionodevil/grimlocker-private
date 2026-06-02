import Foundation

public struct FileEntry: Codable, Identifiable, Sendable {
    public let id: String
    public let fileName: String
    public let mimeType: String
    public let totalSize: Int64
    public let manifestBlockId: String
    public let folderId: String

    enum CodingKeys: String, CodingKey {
        case id
        case fileName = "file_name"
        case mimeType = "mime_type"
        case totalSize = "total_size"
        case manifestBlockId = "manifest_block_id"
        case folderId = "folder_id"
    }
}
