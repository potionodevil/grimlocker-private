package com.grimlocker.sdk

class GrimlockerException(
    val errorCode: Int,
    message: String
) : RuntimeException(message) {

    constructor(message: String) : this(0, message)

    override fun toString(): String = "GrimlockerException(code=$errorCode, message='$message')"

    companion object {
        fun nameOf(code: Int): String = when (code) {
            -1   -> "BUS_ERROR"
            -2   -> "STORAGE_ERROR"
            -3   -> "NOT_FOUND"
            -10  -> "ENTRY_NOT_FOUND"
            -20  -> "CATEGORY_ERROR"
            -30  -> "CREATE_FAILED"
            -31  -> "UPDATE_FAILED"
            -32  -> "DELETE_FAILED"
            -100 -> "PROTOCOL_ERROR"
            -101 -> "AUTH_REQUIRED"
            -102 -> "PERMISSION_DENIED"
            -103 -> "INVALID_REQUEST"
            -104 -> "TIMEOUT"
            else -> "UNKNOWN"
        }
    }
}
