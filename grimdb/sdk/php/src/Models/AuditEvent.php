<?php declare(strict_types=1);
namespace Grimlocker\Models;

class AuditEvent
{
    public function __construct(
        public readonly int     $timestamp = 0,
        public readonly string  $level     = '',
        public readonly string  $module    = '',
        public readonly string  $message   = '',
        public readonly ?string $subjectId = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            $data['timestamp']  ?? 0,
            $data['level']      ?? '',
            $data['module']     ?? '',
            $data['message']    ?? '',
            $data['subject_id'] ?? null,
        );
    }
}
