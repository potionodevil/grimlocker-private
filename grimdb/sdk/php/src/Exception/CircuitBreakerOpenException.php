<?php declare(strict_types=1);
namespace Grimlocker\Exception;

class CircuitBreakerOpenException extends GrimlockerException {
    public function __construct(string $message = 'Circuit breaker is open', ?\Throwable $previous = null) {
        parent::__construct($message, 0, $previous);
    }
}
