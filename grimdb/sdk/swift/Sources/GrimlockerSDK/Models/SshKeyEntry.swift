import Foundation

public struct SshKeyEntry: Codable, Identifiable, Sendable {
    public var id: String
    public var title: String
    public var publicKey: String
    public var privateKey: String
    public var username: String
    public var passphrase: String

    enum CodingKeys: String, CodingKey {
        case id, title
        case publicKey = "public_key"
        case privateKey = "private_key"
        case username
        case passphrase
    }

    public init(id: String = "", title: String, publicKey: String = "",
                privateKey: String = "", username: String = "",
                passphrase: String = "") {
        self.id = id
        self.title = title
        self.publicKey = publicKey
        self.privateKey = privateKey
        self.username = username
        self.passphrase = passphrase
    }

    public var fields: [String: String] {
        [
            "public_key": publicKey,
            "private_key": privateKey,
            "username": username,
            "passphrase": passphrase,
        ]
    }

    public static func from(_ entry: VaultEntry) -> Self {
        .init(
            id: entry.id,
            title: entry.title,
            publicKey: entry.field("public_key"),
            privateKey: entry.field("private_key"),
            username: entry.field("username"),
            passphrase: entry.field("passphrase")
        )
    }
}
