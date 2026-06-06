package com.grimlocker.sdk;

/** Thrown when the circuit breaker is open and requests are being rejected. */
public class CircuitBreakerOpenException extends GrimlockerException {
    public CircuitBreakerOpenException(String message) {
        super(message);
    }
}
