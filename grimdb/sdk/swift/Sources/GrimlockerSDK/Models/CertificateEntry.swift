import Foundation

public struct CertificateEntry: Codable, Identifiable, Sendable {
    public var id: String
    public var title: String
    public var domain: String
    public var certificate: String
    public var privateKey: String

    enum CodingKeys: String, CodingKey {
        case id, title, domain, certificate
        case privateKey = "private_key"
    }

    public init(id: String = "", title: String, domain: String = "",
                certificate: String = "", privateKey: String = "") {
        self.id = id
        self.title = title
        self.domain = domain
        self.certificate = certificate
        self.privateKey = privateKey
    }

    public var fields: [String: String] {
        [
            "domain": domain,
            "certificate": certificate,
            "private_key": privateKey,
        ]
    }

    public static func from(_ entry: VaultEntry) -> Self {
        .init(
            id: entry.id,
            title: entry.title,
            domain: entry.field("domain"),
            certificate: entry.field("certificate"),
            privateKey: entry.field("private_key")
        )
    }
}
