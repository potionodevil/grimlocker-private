#pragma once
/**
 * grimlocker/client.hpp — Header-only C++17 SDK for the Grimlocker daemon.
 *
 * Uses libcurl for HTTP transport and nlohmann/json for JSON.
 * Requires: libcurl, nlohmann/json (both available via vcpkg or Conan).
 *
 * Usage:
 *   #include <grimlocker/client.hpp>
 *   grimlocker::Client c("http://127.0.0.1:36353", token);
 *   c.unlock_vault("password");
 *   auto passwords = c.list_passwords();
 */

#include "types.hpp"
#include "error.hpp"
#include <curl/curl.h>
#include <nlohmann/json.hpp>
#include <functional>
#include <sstream>
#include <cstring>
#include <vector>
#include <chrono>
#include <thread>

namespace grimlocker {

namespace detail {
    static size_t write_cb(char* ptr, size_t size, size_t nmemb, std::string* data) {
        data->append(ptr, size * nmemb);
        return size * nmemb;
    }

    static std::vector<uint8_t> base64_decode(const std::string& encoded) {
        static const std::string chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
        std::vector<int> T(256, -1);
        for (int i = 0; i < 64; i++) T[(unsigned char)chars[i]] = i;

        std::vector<uint8_t> out;
        int val = 0, valb = -8;
        for (unsigned char c : encoded) {
            if (T[c] == -1) break;
            val = (val << 6) + T[c];
            valb += 6;
            if (valb >= 0) {
                out.push_back(uint8_t((val >> valb) & 0xFF));
                valb -= 8;
            }
        }
        return out;
    }
} // namespace detail

struct CircuitBreakerOpenException : GrimlockerError {
    CircuitBreakerOpenException() : GrimlockerError("circuit breaker is open", -200) {}
};

struct BatchEntryInput {
    std::string title;
    std::string category;
    std::map<std::string, std::string> fields;
};

class Client {
public:
    /**
     * @param base_url  Daemon URL, e.g. "http://127.0.0.1:36353"
     * @param token     GRIMLOCKER_TOKEN from daemon stdout
     */
    Client(std::string base_url, std::string token)
        : base_url_(std::move(base_url)), token_(std::move(token))
    {
        curl_global_init(CURL_GLOBAL_DEFAULT);
    }

    ~Client() { curl_global_cleanup(); }

    // ── Auth ──────────────────────────────────────────────────────────────────

    void unlock_vault(const std::string& password) {
        call("vault.unlock", { {"password", password} });
    }
    void lock_vault() { call("vault.logout", {}); }
    VaultStatus vault_status() {
        auto j = call("vault.status", {});
        return { j.value("initialized", false), j.value("unlocked", false), j.value("status", std::string{}) };
    }

    // ── Entries ───────────────────────────────────────────────────────────────

    std::vector<VaultEntry> list_entries(const std::string& category = "") {
        nlohmann::json p = nlohmann::json::object();
        std::string action = "entry.list";
        if (!category.empty()) { p["category"] = category; action = "entry.query"; }
        return parse_entries(call(action, p));
    }

    VaultEntry get_entry(const std::string& id) {
        auto j = call("entry.read", { {"id", id} });
        auto entries = parse_entries(j);
        if (entries.empty()) throw GrimlockerError("Entry not found: " + id, -10);
        return entries.front();
    }

    VaultEntry create_entry(const std::string& title, const std::string& category,
                            const std::map<std::string, std::string>& fields) {
        nlohmann::json f = fields;
        auto j = call("entry.create", { {"title", title}, {"category", category}, {"fields", f} });
        auto entries = parse_entries(j);
        if (entries.empty()) throw GrimlockerError("Create returned no entry", -30);
        return entries.front();
    }

    void update_entry(const std::string& id, const std::map<std::string, std::string>& fields) {
        nlohmann::json f = fields;
        call("entry.update", { {"id", id}, {"fields", f} });
    }

    void delete_entry(const std::string& id) {
        call("entry.delete", { {"id", id} });
    }

    std::vector<std::string> create_entries_batch(const std::vector<BatchEntryInput>& items) {
        std::vector<std::string> ids;
        ids.reserve(items.size());
        for (const auto& item : items) {
            ids.push_back(create_entry(item.title, item.category, item.fields).id);
        }
        return ids;
    }

    void delete_entries_batch(const std::vector<std::string>& ids) {
        for (const auto& id : ids) {
            delete_entry(id);
        }
    }

    std::vector<VaultEntry> search_entries(const std::string& query, const std::string& category = "") {
        nlohmann::json p = { {"query", query} };
        if (!category.empty()) p["category"] = category;
        return parse_entries(call("entry.search", p));
    }

    // ── Typed helpers ─────────────────────────────────────────────────────────

    std::vector<PasswordEntry> list_passwords() {
        auto entries = list_entries("PASSWORD");
        std::vector<PasswordEntry> out;
        for (const auto& e : entries) out.push_back(PasswordEntry::from_entry(e));
        return out;
    }

    std::string create_password(const PasswordEntry& p) {
        return create_entry(p.title, "PASSWORD", p.to_fields()).id;
    }

    std::vector<SshKeyEntry> list_ssh_keys() {
        auto entries = list_entries("SSH_KEY");
        std::vector<SshKeyEntry> out;
        for (const auto& e : entries) out.push_back(SshKeyEntry::from_entry(e));
        return out;
    }

    std::string create_ssh_key(const SshKeyEntry& k) {
        return create_entry(k.title, "SSH_KEY", k.to_fields()).id;
    }

    std::vector<CertificateEntry> list_certificates() {
        auto entries = list_entries("CERTIFICATE");
        std::vector<CertificateEntry> out;
        for (const auto& e : entries) out.push_back(CertificateEntry::from_entry(e));
        return out;
    }

    std::string create_certificate(const CertificateEntry& c) {
        return create_entry(c.title, "CERTIFICATE", c.to_fields()).id;
    }

    // ── File Vault ────────────────────────────────────────────────────────────

    FolderListing list_folder(const std::string& folder_id = "") {
        auto j = call("file.list_folder", { {"folder_id", folder_id} });
        FolderListing out;
        for (const auto& f : j.value("folders", nlohmann::json::array())) {
            out.folders.push_back({ f.value("id",""), f.value("name",""), "folder" });
        }
        for (const auto& f : j.value("files", nlohmann::json::array())) {
            out.files.push_back({ f.value("id",""), f.value("file_name",""),
                f.value("mime_type",""), f.value("manifest_block_id",""), f.value("folder_id",""),
                f.value("total_size", uint64_t{0}) });
        }
        return out;
    }

    FolderItem create_folder(const std::string& name, const std::string& parent_id = "") {
        auto j = call("file.create_folder", { {"name", name}, {"parent_id", parent_id} });
        return { j.value("id",""), j.value("name", name), "folder" };
    }

    void rename_folder(const std::string& id, const std::string& name) {
        call("file.rename_folder", { {"id", id}, {"name", name} });
    }

    void delete_folder(const std::string& id) {
        call("file.delete_folder", { {"id", id} });
    }

    void move_file(const std::string& manifest_block_id, const std::string& folder_id) {
        call("file.move", { {"manifest_block_id", manifest_block_id}, {"folder_id", folder_id} });
    }

    std::vector<uint8_t> download_file(const std::string& manifest_block_id) {
        auto j = call("file.download", { {"manifest_block_id", manifest_block_id} });
        std::string b64 = j.value("data_b64", std::string{});
        return detail::base64_decode(b64);
    }

    FileEntry upload_file(const std::vector<uint8_t>& data, const std::string& filename,
                          const std::string& mime_type = "application/octet-stream",
                          const std::string& folder_id = "",
                          std::function<void(UploadProgress)> on_progress = nullptr) {
        if (on_progress) on_progress({ 0, data.size() });
        // Base64-encode data
        static const char* b64chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
        std::string b64;
        b64.reserve(((data.size() + 2) / 3) * 4);
        for (size_t i = 0; i < data.size(); i += 3) {
            uint32_t n  = (uint32_t)data[i] << 16;
            if (i+1 < data.size()) n |= (uint32_t)data[i+1] << 8;
            if (i+2 < data.size()) n |= (uint32_t)data[i+2];
            b64 += b64chars[(n >> 18) & 63];
            b64 += b64chars[(n >> 12) & 63];
            b64 += (i+1 < data.size()) ? b64chars[(n >> 6) & 63] : '=';
            b64 += (i+2 < data.size()) ? b64chars[n & 63] : '=';
        }
        auto j = call("file.ingest", {
            {"file_name", filename}, {"mime_type", mime_type},
            {"folder_id", folder_id}, {"data_b64", b64}
        });
        if (on_progress) on_progress({ data.size(), data.size() });
        return { j.value("id",""), j.value("file_name", filename), j.value("mime_type", mime_type),
                 j.value("manifest_block_id",""), j.value("folder_id",""),
                 j.value("total_size", (uint64_t)data.size()) };
    }

    // ── Workspaces ────────────────────────────────────────────────────────────

    std::vector<Workspace> list_workspaces() {
        auto j = call("workspace.list", {});
        std::vector<Workspace> out;
        if (j.is_array()) {
            for (const auto& w : j)
                out.push_back({ w.value("id",""), w.value("name",""), w.value("is_default",false) });
        }
        return out;
    }

    Workspace create_workspace(const std::string& name) {
        auto j = call("workspace.create", { {"name", name} });
        return { j.value("id",""), j.value("name", name), false };
    }

    void switch_workspace(const std::string& id) { call("workspace.switch", { {"id", id} }); }
    void rename_workspace(const std::string& id, const std::string& name) {
        call("workspace.rename", { {"id", id}, {"name", name} });
    }
    void delete_workspace(const std::string& id) { call("workspace.delete", { {"id", id} }); }

    // ── Sync ──────────────────────────────────────────────────────────────────

    SyncStatus list_sync_peers() {
        auto j = call("sync.list_peers", {});
        SyncStatus s;
        s.device_id    = j.value("device_id", std::string{});
        s.last_sync_at = j.value("last_sync_at", int64_t{0});
        for (const auto& p : j.value("peers", nlohmann::json::array()))
            s.peers.push_back({ p.value("device_id",""), p.value("host",""),
                p.value("port",0), p.value("seen_at",int64_t{0}), p.value("reachable",true) });
        return s;
    }

    void trigger_sync() { call("sync.trigger", {}); }

    // ── Audit ─────────────────────────────────────────────────────────────────

    std::vector<AuditEvent> list_audit_events(int n = 50) {
        auto j = call("audit.list", { {"n", n} });
        std::vector<AuditEvent> out;
        if (j.is_array()) {
            for (const auto& e : j)
                out.push_back({ e.value("timestamp", int64_t{0}), e.value("level",""),
                    e.value("module",""), e.value("message",""),
                    e.contains("subject_id") ? std::optional<std::string>(e["subject_id"].get<std::string>()) : std::nullopt });
        }
        return out;
    }

    // ── Tools ─────────────────────────────────────────────────────────────────

    SSHKeyResult generate_ssh_key(const std::string& comment = "", bool save_to_vault = true) {
        auto j = call("tool.ssh_keygen", { {"comment", comment}, {"save_to_vault", save_to_vault} });
        return { j.value("public_key", std::string{}), j.value("fingerprint", std::string{}), j.value("entry_id", std::string{}) };
    }

    std::string get_recovery_phrase(const std::string& password) {
        auto j = call("vault.recovery_phrase", { {"password", password} });
        return j.value("recovery_phrase", j.value("phrase", std::string{}));
    }

    // ── Health ────────────────────────────────────────────────────────────────

    VaultStatus health_check() { return vault_status(); }

private:
    std::string base_url_, token_;
    int consecutive_failures_ = 0;
    std::chrono::steady_clock::time_point circuit_open_until_;

    nlohmann::json call(const std::string& action, const nlohmann::json& payload) {
        auto now = std::chrono::steady_clock::now();
        if (now < circuit_open_until_) {
            throw CircuitBreakerOpenException();
        }

        std::string url = base_url_ + "/api/v1";
        std::string body = nlohmann::json{ {"action", action}, {"payload", payload} }.dump();
        std::string resp;
        long http_code = 0;
        CURLcode last_res = CURLE_OK;

        for (int attempt = 0; attempt < 4; ++attempt) {
            resp.clear();
            http_code = 0;
            CURL* curl = curl_easy_init();
            if (!curl) {
                last_res = CURLE_FAILED_INIT;
            } else {
                struct curl_slist* headers = nullptr;
                headers = curl_slist_append(headers, "Content-Type: application/json");
                std::string auth_hdr = "X-Grimlocker-Token: " + token_;
                headers = curl_slist_append(headers, auth_hdr.c_str());

                curl_easy_setopt(curl, CURLOPT_URL,            url.c_str());
                curl_easy_setopt(curl, CURLOPT_POSTFIELDS,     body.c_str());
                curl_easy_setopt(curl, CURLOPT_HTTPHEADER,     headers);
                curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION,  detail::write_cb);
                curl_easy_setopt(curl, CURLOPT_WRITEDATA,      &resp);
                curl_easy_setopt(curl, CURLOPT_TIMEOUT,        30L);

                last_res = curl_easy_perform(curl);
                curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &http_code);
                curl_slist_free_all(headers);
                curl_easy_cleanup(curl);
            }

            if (last_res == CURLE_OK && http_code >= 200 && http_code < 300) {
                consecutive_failures_ = 0;
                break;
            }

            bool is_network_error = (last_res != CURLE_OK);
            bool is_client_error  = (last_res == CURLE_OK && http_code >= 400 && http_code < 500);

            if (is_client_error || attempt == 3) {
                consecutive_failures_++;
                if (consecutive_failures_ >= 5) {
                    circuit_open_until_ = std::chrono::steady_clock::now() + std::chrono::seconds(30);
                }
                if (last_res != CURLE_OK) {
                    throw GrimlockerError(std::string("curl error: ") + curl_easy_strerror(last_res));
                }
                auto j = nlohmann::json::parse(resp, nullptr, false);
                if (j.is_discarded()) throw GrimlockerError("invalid JSON response");
                int code = j.value("error_code", 0);
                std::string msg = j.value("error", std::string("HTTP ") + std::to_string(http_code));
                throw GrimlockerError(std::string(GrimlockerError::name_of(code)) + ": " + msg, code);
            }

            long delay_ms = 100L * (1L << attempt);
            if (delay_ms > 2000) delay_ms = 2000;
            std::this_thread::sleep_for(std::chrono::milliseconds(delay_ms));
        }

        auto j = nlohmann::json::parse(resp, nullptr, false);
        if (j.is_discarded()) throw GrimlockerError("invalid JSON response");
        return j;
    }

    std::vector<VaultEntry> parse_entries(const nlohmann::json& j) {
        std::vector<VaultEntry> out;
        auto arr = j.is_array() ? j : j.value("entries", nlohmann::json::array());
        for (const auto& e : arr) {
            VaultEntry entry;
            entry.id         = e.value("id", "");
            entry.title      = e.value("title", "");
            entry.category   = e.value("category", "");
            entry.created_at = e.value("created_at", int64_t{0});
            entry.updated_at = e.value("updated_at", int64_t{0});
            if (e.contains("fields") && e["fields"].is_object()) {
                for (const auto& [k, v] : e["fields"].items())
                    entry.fields[k] = v.get<std::string>();
            }
            out.push_back(std::move(entry));
        }
        return out;
    }
};

} // namespace grimlocker
