<?php declare(strict_types=1);
namespace Grimlocker\Exception;

class GrimlockerException extends \RuntimeException {
    public function __construct(string $message, int $code = 0, ?\Throwable $previous = null) {
        parent::__construct($message, $code, $previous);
    }
    public static function nameOf(int $code): string {
        return match($code) {
            -1 => 'BUS_ERROR', -2 => 'STORAGE_ERROR', -3 => 'NOT_FOUND',
            -10 => 'ENTRY_NOT_FOUND', -20 => 'CATEGORY_ERROR',
            -30 => 'CREATE_FAILED', -31 => 'UPDATE_FAILED', -32 => 'DELETE_FAILED',
            -100 => 'PROTOCOL_ERROR', -101 => 'AUTH_REQUIRED',
            -102 => 'PERMISSION_DENIED', -103 => 'INVALID_REQUEST', default => 'UNKNOWN',
        };
    }
}
