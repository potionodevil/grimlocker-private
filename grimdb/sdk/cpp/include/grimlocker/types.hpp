#pragma once
#include <string>
#include <vector>
#include <map>
#include <optional>
#include <cstdint>

namespace grimlocker {

struct VaultEntry {
    std::string id;
    std::string title;
    std::string category;
    std::map<std::string, std::string> fields;
    int64_t created_at = 0;
    int64_t updated_at = 0;

    std::string field(const std::string& key) const {
        auto it = fields.find(key);
        return it != fields.end() ? it->second : "";
    }
};

struct PasswordEntry {
    std::string id, title, username, password, url, notes;
    static PasswordEntry from_entry(const VaultEntry& e) {
        return { e.id, e.title, e.field("username"), e.field("password"), e.field("url"), e.field("notes") };
    }
    std::map<std::string, std::string> to_fields() const {
        return { {"username", username}, {"password", password}, {"url", url}, {"notes", notes} };
    }
};

struct SshKeyEntry {
    std::string id, title, public_key, private_key, username, passphrase, comment;
    static SshKeyEntry from_entry(const VaultEntry& e) {
        return { e.id, e.title, e.field("public_key"), e.field("private_key"),
                 e.field("username"), e.field("passphrase"), e.field("comment") };
    }
    std::map<std::string, std::string> to_fields() const {
        return { {"public_key", public_key}, {"private_key", private_key},
                 {"username", username}, {"passphrase", passphrase}, {"comment", comment} };
    }
};

struct CertificateEntry {
    std::string id, title, domain, certificate, private_key;
    static CertificateEntry from_entry(const VaultEntry& e) {
        return { e.id, e.title, e.field("domain"), e.field("certificate"), e.field("private_key") };
    }
    std::map<std::string, std::string> to_fields() const {
        return { {"domain", domain}, {"certificate", certificate}, {"private_key", private_key} };
    }
};

struct FileEntry {
    std::string id, file_name, mime_type, manifest_block_id, folder_id;
    uint64_t total_size = 0;
};

struct FolderItem {
    std::string id, name, kind; // kind: "folder" | "file"
};

struct FolderListing {
    std::vector<FolderItem> folders;
    std::vector<FileEntry>  files;
};

struct Workspace {
    std::string id, name;
    bool is_default = false;
};

struct SyncPeer {
    std::string device_id, host;
    int port = 0;
    int64_t seen_at = 0;
    bool reachable = true;
};

struct SyncStatus {
    std::vector<SyncPeer> peers;
    int64_t last_sync_at = 0;
    std::string device_id;
};

struct AuditEvent {
    int64_t timestamp = 0;
    std::string level, module, message;
    std::optional<std::string> subject_id;
};

struct VaultStatus {
    bool initialized = false;
    bool unlocked    = false;
    std::string status;
};

struct UploadProgress {
    uint64_t bytes_sent   = 0;
    uint64_t total_bytes  = 0;
    double percent() const { return total_bytes ? (bytes_sent * 100.0 / total_bytes) : 100.0; }
};

struct SSHKeyResult {
    std::string public_key;
    std::string fingerprint;
    std::string entry_id;
};

} // namespace grimlocker
