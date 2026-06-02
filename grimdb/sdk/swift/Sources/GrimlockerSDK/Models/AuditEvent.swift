import Foundation

public struct AuditEvent: Codable, Identifiable, Sendable {
    public var id: Int64 { timestamp }
    public let timestamp: Int64
    public let level: String
    public let module: String
    public let message: String
    public let subjectId: String?

    enum CodingKeys: String, CodingKey {
        case timestamp, module, message, level
        case subjectId = "subject_id"
    }
}
