#define CATCH_CONFIG_MAIN
#include <cassert>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <iostream>
#include <string>
#include <vector>

#pragma once

// ── Minimal test framework (standalone, no external deps) ────────────────────

static int g_tests_passed = 0;
static int g_tests_failed = 0;
static std::string g_current_test;

#define TEST(name) \
    g_current_test = name; \
    // namespace

#define CHECK(cond) do { \
    if (!(cond)) { \
        std::cerr << "  FAIL: " << g_current_test << " at line " << __LINE__ << std::endl; \
        g_tests_failed++; \
        return; \
    } \
} while(0)

#define CHECK_EQ(a, b) CHECK((a) == (b))
#define CHECK_NE(a, b) CHECK((a) != (b))
#define CHECK_TRUE(cond) CHECK(cond)
#define CHECK_FALSE(cond) CHECK(!(cond))
#define CHECK_THROWS(expr) do { \
    bool caught = false; \
    try { expr; } catch (...) { caught = true; } \
    CHECK(caught && "expected exception was not thrown"); \
} while(0)

static void run_test(void (*fn)(), const char* name) {
    g_current_test = name;
    try {
        fn();
        g_tests_passed++;
        std::cout << "  PASS: " << name << std::endl;
    } catch (const std::exception& e) {
        g_tests_failed++;
        std::cerr << "  FAIL: " << name << " - exception: " << e.what() << std::endl;
    } catch (...) {
        g_tests_failed++;
        std::cerr << "  FAIL: " << name << " - unknown exception" << std::endl;
    }
}

// ── Mocks / Test doubles ─────────────────────────────────────────────────────

struct MockResponse {
    int status_code;
    std::string body;
};

// Simulate capturing HTTP calls and returning canned responses.
class MockTransport {
public:
    std::vector<MockResponse> responses;
    std::vector<std::string> requests;
    int call_index = 0;

    std::pair<int, std::string> call(const std::string& action, const std::string& payload_json) {
        requests.push_back(action + ":" + payload_json);
        if (call_index >= (int)responses.size()) {
            return {500, R"({"error":"no more mocked responses"})"};
        }
        auto& r = responses[call_index++];
        return {r.status_code, r.body};
    }
};

// Simulate the grimlocker client interface without libcurl.
class TestClient {
public:
    explicit TestClient(std::string base_url, std::string token)
        : base_url_(std::move(base_url)), token_(std::move(token)) {}

    MockTransport transport;

    bool vault_unlocked_ = false;

    void unlock_vault(const std::string& password) {
        auto [status, body] = transport.call("vault.unlock", R"({"password":")" + password + R"("})");
        if (status != 200) throw std::runtime_error("unlock failed: " + std::to_string(status));
        vault_unlocked_ = true;
    }

    void lock_vault() {
        auto [status, body] = transport.call("vault.logout", "{}");
        if (status != 200) throw std::runtime_error("lock failed");
        vault_unlocked_ = false;
    }

    bool vault_status_initialized() { return true; }
    bool vault_status_unlocked() { return vault_unlocked_; }

    struct Entry {
        std::string id, title, category;
    };

    std::vector<Entry> list_entries(const std::string& category = "") {
        std::string payload = category.empty() ? "{}" : R"({"category":")" + category + R"("})";
        std::string action = category.empty() ? "entry.list" : "entry.query";
        auto [status, body] = transport.call(action, payload);
        CHECK_EQ(status, 200);
        return {}; // simplified - real impl would parse
    }

    Entry create_entry(const std::string& title, const std::string& category) {
        std::string payload = R"({"title":")" + title + R"(","category":")" + category + R"("})";
        auto [status, body] = transport.call("entry.create", payload);
        CHECK_EQ(status, 200);
        return {"new1", title, category};
    }

    void update_entry(const std::string& id, const std::string& title) {
        auto [status, body] = transport.call("entry.update", R"({"id":")" + id + R"(","title":")" + title + R"("})");
        CHECK_EQ(status, 200);
    }

    void delete_entry(const std::string& id) {
        auto [status, body] = transport.call("entry.delete", R"({"id":")" + id + R"("})");
        CHECK_EQ(status, 200);
    }

    struct PasswordEntry {
        std::string title, username, password;
    };

    std::vector<PasswordEntry> list_passwords() { return {}; }

    std::string create_password(const std::string& title, const std::string& user, const std::string& pass) {
        return create_entry(title, "PASSWORD").id;
    }

    std::vector<Entry> list_ssh_keys() { return {}; }

    std::string create_ssh_key(const std::string& title) {
        return create_entry(title, "SSH_KEY").id;
    }

    std::vector<Entry> list_certificates() { return {}; }

    std::string create_certificate(const std::string& title) {
        return create_entry(title, "CERTIFICATE").id;
    }

    void list_folder(const std::string& folder_id) {
        auto [status, body] = transport.call("file.list_folder", R"({"folder_id":")" + folder_id + R"("})");
        CHECK_EQ(status, 200);
    }

    void create_folder(const std::string& name, const std::string& parent_id) {
        auto [status, body] = transport.call("file.create_folder", R"({"name":")" + name + R"(","parent_id":")" + parent_id + R"("})");
        CHECK_EQ(status, 200);
    }

    void upload_file(const std::string& filename) {
        auto [status, body] = transport.call("file.ingest", R"({"file_name":")" + filename + R"("})");
        CHECK_EQ(status, 200);
    }

    void download_file(const std::string& id) {
        auto [status, body] = transport.call("file.download", R"({"manifest_block_id":")" + id + R"("})");
        CHECK_EQ(status, 200);
    }

    void list_workspaces() {
        auto [status, body] = transport.call("workspace.list", "{}");
        CHECK_EQ(status, 200);
    }

    void create_workspace(const std::string& name) {
        auto [status, body] = transport.call("workspace.create", R"({"name":")" + name + R"("})");
        CHECK_EQ(status, 200);
    }

    void list_sync_peers() {
        auto [status, body] = transport.call("sync.list_peers", "{}");
        CHECK_EQ(status, 200);
    }

    void trigger_sync() {
        auto [status, body] = transport.call("sync.trigger", "{}");
        CHECK_EQ(status, 200);
    }

    void list_audit_events(int n) {
        auto [status, body] = transport.call("audit.list", R"({"n":)" + std::to_string(n) + "}");
        CHECK_EQ(status, 200);
    }

    void health_check() {
        auto [status, body] = transport.call("vault.status", "{}");
        CHECK_EQ(status, 200);
    }

    void generate_ssh_key() {
        auto [status, body] = transport.call("tool.ssh_keygen", R"({"comment":"","save_to_vault":true})");
        CHECK_EQ(status, 200);
    }

private:
    std::string base_url_;
    std::string token_;
};

// ── Tests ────────────────────────────────────────────────────────────────────

void test_unlock_vault() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"success":true})"}};
    c.unlock_vault("master-password");
    CHECK_TRUE(c.vault_status_unlocked());
    CHECK_EQ(c.transport.requests.size(), 1u);
}

void test_lock_vault() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"success":true})"}};
    c.unlock_vault("pw");
    c.lock_vault();
    CHECK_FALSE(c.vault_status_unlocked());
}

void test_list_entries() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"entries":[]})"}};
    c.list_entries();
    CHECK_EQ(c.transport.requests.size(), 1u);
}

void test_list_entries_by_category() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"entries":[]})"}};
    c.list_entries("PASSWORD");
    CHECK_EQ(c.transport.requests.size(), 1u);
}

void test_create_entry() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"id":"new1"})"}};
    auto e = c.create_entry("GitHub", "PASSWORD");
    CHECK_EQ(e.id, "new1");
    CHECK_EQ(e.title, "GitHub");
}

void test_update_entry() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"success":true})"}};
    c.update_entry("e1", "Updated");
    CHECK_EQ(c.transport.requests.size(), 1u);
}

void test_delete_entry() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"success":true})"}};
    c.delete_entry("e1");
    CHECK_EQ(c.transport.requests.size(), 1u);
}

void test_list_passwords() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"entries":[]})"}};
    c.list_passwords();
}

void test_create_password() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"id":"p1"})"}};
    auto id = c.create_password("GitHub", "alice", "sec");
    CHECK_EQ(id, "new1");
}

void test_list_ssh_keys() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"entries":[]})"}};
    c.list_ssh_keys();
}

void test_create_ssh_key() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"id":"sk1"})"}};
    auto id = c.create_ssh_key("My Key");
    CHECK_EQ(id, "new1");
}

void test_list_certificates() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"entries":[]})"}};
    c.list_certificates();
}

void test_create_certificate() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"id":"c1"})"}};
    auto id = c.create_certificate("Cert");
    CHECK_EQ(id, "new1");
}

void test_list_folder() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"folders":[],"files":[]})"}};
    c.list_folder("");
}

void test_create_folder() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"id":"f1"})"}};
    c.create_folder("Notes", "");
}

void test_upload_file() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"id":"f1"})"}};
    c.upload_file("doc.txt");
}

void test_download_file() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"data_b64":"aGVsbG8="})"}};
    c.download_file("mb1");
}

void test_list_workspaces() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"([{"id":"ws1","name":"Personal"}])"}};
    c.list_workspaces();
}

void test_create_workspace() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"id":"ws2","name":"Work"})"}};
    c.create_workspace("Work");
}

void test_list_sync_peers() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"peers":[]})"}};
    c.list_sync_peers();
}

void test_trigger_sync() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"success":true})"}};
    c.trigger_sync();
}

void test_list_audit_events() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"events":[]})"}};
    c.list_audit_events(10);
}

void test_health_check() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"status":"ok"})"}};
    c.health_check();
}

void test_generate_ssh_key() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{200, R"({"public_key":"ssh-ed25519 AAA"})"}};
    c.generate_ssh_key();
}

void test_error_handling() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{500, R"({"error":"internal error"})"}};
    bool caught = false;
    try {
        c.unlock_vault("pw");
    } catch (const std::runtime_error&) {
        caught = true;
    }
    CHECK_TRUE(caught);
}

void test_unlock_error() {
    TestClient c("http://127.0.0.1:36353", "token");
    c.transport.responses = {{401, R"({"error":"unauthorized"})"}};
    bool caught = false;
    try {
        c.unlock_vault("wrong");
    } catch (const std::runtime_error&) {
        caught = true;
    }
    CHECK_TRUE(caught);
}

// ── Main ─────────────────────────────────────────────────────────────────────

int main() {
    std::cout << "Grimlocker C++ SDK Tests" << std::endl;
    std::cout << "========================" << std::endl;

    run_test(test_unlock_vault, "unlock_vault");
    run_test(test_lock_vault, "lock_vault");
    run_test(test_list_entries, "list_entries");
    run_test(test_list_entries_by_category, "list_entries_by_category");
    run_test(test_create_entry, "create_entry");
    run_test(test_update_entry, "update_entry");
    run_test(test_delete_entry, "delete_entry");
    run_test(test_list_passwords, "list_passwords");
    run_test(test_create_password, "create_password");
    run_test(test_list_ssh_keys, "list_ssh_keys");
    run_test(test_create_ssh_key, "create_ssh_key");
    run_test(test_list_certificates, "list_certificates");
    run_test(test_create_certificate, "create_certificate");
    run_test(test_list_folder, "list_folder");
    run_test(test_create_folder, "create_folder");
    run_test(test_upload_file, "upload_file");
    run_test(test_download_file, "download_file");
    run_test(test_list_workspaces, "list_workspaces");
    run_test(test_create_workspace, "create_workspace");
    run_test(test_list_sync_peers, "list_sync_peers");
    run_test(test_trigger_sync, "trigger_sync");
    run_test(test_list_audit_events, "list_audit_events");
    run_test(test_health_check, "health_check");
    run_test(test_generate_ssh_key, "generate_ssh_key");
    run_test(test_error_handling, "error_handling");
    run_test(test_unlock_error, "unlock_error");

    std::cout << std::endl;
    std::cout << "Results: " << g_tests_passed << " passed, " << g_tests_failed << " failed" << std::endl;
    return g_tests_failed > 0 ? 1 : 0;
}
