import Foundation

public struct FolderListing: Codable, Sendable {
    public let folders: [FolderItem]
    public let files: [FileEntry]
}
