#pragma once
#include <nlohmann/json.hpp>
namespace grimlocker { namespace detail {
    // Convenience helpers
    inline std::string json_str(const nlohmann::json& j, const char* key) {
        return j.value(key, std::string{});
    }
}}
