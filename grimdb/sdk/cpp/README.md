# grimlocker C++ SDK

Header-only C++17 SDK for the Grimlocker daemon. Uses `libcurl` + `nlohmann/json`.

## Installation

### vcpkg
```
vcpkg install curl nlohmann-json
```

### CMake
```cmake
find_package(grimlocker_sdk REQUIRED)
target_link_libraries(my_target PRIVATE grimlocker_sdk)
```

## Quick Start

```cpp
#include <grimlocker/client.hpp>
#include <iostream>

int main() {
    const char* token = std::getenv("GRIMLOCKER_TOKEN");
    grimlocker::Client c("http://127.0.0.1:36353", token);

    c.unlock_vault("master-password");

    // List passwords
    auto passwords = c.list_passwords();
    for (const auto& p : passwords)
        std::cout << p.title << " — " << p.username << "\n";

    // Create a password
    std::string id = c.create_password({ "", "GitHub", "me@example.com", "s3cr3t", "https://github.com", "" });

    // File vault
    auto listing = c.list_folder();
    auto folder  = c.create_folder("Documents");

    // Upload a file
    std::vector<uint8_t> data = { /* ... */ };
    auto file = c.upload_file(data, "secret.bin", "application/octet-stream", folder.id,
        [](grimlocker::UploadProgress p) { std::cout << p.percent() << "%" << std::endl; });

    // Sync + Audit
    auto sync   = c.list_sync_peers();
    auto events = c.list_audit_events(20);

    // Workspaces
    auto workspaces = c.list_workspaces();

    return 0;
}
```

## C API (for FFI)
A `extern "C"` wrapper is available for Python `ctypes`, Node.js native addons, and other FFI consumers. See [grimlocker_c.h](include/grimlocker/grimlocker_c.h).
