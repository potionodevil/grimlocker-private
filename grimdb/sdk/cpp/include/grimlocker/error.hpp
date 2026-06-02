#pragma once
#include <stdexcept>
#include <string>

namespace grimlocker {

class GrimlockerError : public std::runtime_error {
public:
    int error_code;

    GrimlockerError(const std::string& msg, int code = 0)
        : std::runtime_error(msg), error_code(code) {}

    static const char* name_of(int code) {
        switch (code) {
            case -1:   return "BUS_ERROR";
            case -2:   return "STORAGE_ERROR";
            case -3:   return "NOT_FOUND";
            case -10:  return "ENTRY_NOT_FOUND";
            case -20:  return "CATEGORY_ERROR";
            case -30:  return "CREATE_FAILED";
            case -31:  return "UPDATE_FAILED";
            case -32:  return "DELETE_FAILED";
            case -100: return "PROTOCOL_ERROR";
            case -101: return "AUTH_REQUIRED";
            case -102: return "PERMISSION_DENIED";
            case -103: return "INVALID_REQUEST";
            case -104: return "TIMEOUT";
            default:   return "UNKNOWN";
        }
    }
};

} // namespace grimlocker
