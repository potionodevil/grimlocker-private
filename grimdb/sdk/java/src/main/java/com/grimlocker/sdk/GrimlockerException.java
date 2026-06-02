package com.grimlocker.sdk;

/** Thrown when the Grimlocker daemon returns an error or the connection fails. */
public class GrimlockerException extends RuntimeException {

    private final int errorCode;

    public GrimlockerException(String message) {
        super(message);
        this.errorCode = 0;
    }

    public GrimlockerException(int errorCode, String errorName, String errorMsg) {
        super(errorName + " (" + errorCode + "): " + errorMsg);
        this.errorCode = errorCode;
    }

    public GrimlockerException(String message, Throwable cause) {
        super(message, cause);
        this.errorCode = 0;
    }

    /** The GQL numeric error code, or 0 if this is a connection/protocol error. */
    public int getErrorCode() {
        return errorCode;
    }
}
