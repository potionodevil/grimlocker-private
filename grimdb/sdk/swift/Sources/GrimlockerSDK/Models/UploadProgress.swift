import Foundation

public struct UploadProgress: Codable, Sendable {
    public let bytesSent: Int64
    public let totalBytes: Int64

    enum CodingKeys: String, CodingKey {
        case bytesSent = "bytes_sent"
        case totalBytes = "total_bytes"
    }

    public var percent: Double {
        totalBytes > 0 ? Double(bytesSent) * 100.0 / Double(totalBytes) : 100.0
    }
}
