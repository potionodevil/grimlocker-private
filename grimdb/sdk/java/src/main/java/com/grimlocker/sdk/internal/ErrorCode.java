package com.grimlocker.sdk.internal;

import java.util.HashMap;
import java.util.Map;

/** Maps GQL numeric error codes to symbolic names for logging and exceptions. */
public final class ErrorCode {

    private static final Map<Integer, String> NAMES = new HashMap<>();

    static {
        NAMES.put(-1,   "BUS_TIMEOUT");
        NAMES.put(-2,   "INVALID_STORAGE_RESPONSE");
        NAMES.put(-3,   "STORAGE_ERROR");
        NAMES.put(-10,  "MISSING_ENTRY_ID");
        NAMES.put(-11,  "ENTRY_NOT_FOUND");
        NAMES.put(-20,  "CATEGORY_QUERY_FAILED");
        NAMES.put(-30,  "CREATE_FAILED");
        NAMES.put(-31,  "UPDATE_FAILED");
        NAMES.put(-32,  "DELETE_FAILED");
        NAMES.put(-100, "DISPATCHER_UNAVAILABLE");
        NAMES.put(-101, "INVALID_FRAME");
        NAMES.put(-102, "SCHEMA_VALIDATION");
        NAMES.put(-103, "ACL_DENIED");
        NAMES.put(-104, "NOT_A_QUERY_FRAME");
        NAMES.put(-105, "DISPATCH_ERROR");
    }

    private ErrorCode() {}

    public static String nameOf(int code) {
        return NAMES.getOrDefault(code, "UNKNOWN");
    }
}
