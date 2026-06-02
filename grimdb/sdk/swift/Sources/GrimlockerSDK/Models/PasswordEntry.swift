import Foundation

public struct PasswordEntry: Codable, Identifiable, Sendable {
    public var id: String
    public var title: String
    public var username: String
    public var password: String
    public var url: String
    public var notes: String

    enum CodingKeys: String, CodingKey {
        case id, title, username, password, url, notes
    }

    public init(id: String = "", title: String, username: String = "",
                password: String = "", url: String = "", notes: String = "") {
        self.id = id
        self.title = title
        self.username = username
        self.password = password
        self.url = url
        self.notes = notes
    }

    public var fields: [String: String] {
        [
            "username": username,
            "password": password,
            "url": url,
            "notes": notes,
        ]
    }

    public static func from(_ entry: VaultEntry) -> Self {
        .init(
            id: entry.id,
            title: entry.title,
            username: entry.field("username"),
            password: entry.field("password"),
            url: entry.field("url"),
            notes: entry.field("notes")
        )
    }
}
