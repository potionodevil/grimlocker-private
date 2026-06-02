<?php declare(strict_types=1);
namespace Grimlocker\Models;

class SyncPeer
{
    public function __construct(
        public readonly string $deviceId  = '',
        public readonly string $host      = '',
        public readonly int    $port      = 0,
        public readonly int    $seenAt    = 0,
        public readonly bool   $reachable = true,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            $data['device_id'] ?? '',
            $data['host']      ?? '',
            $data['port']      ?? 0,
            $data['seen_at']   ?? 0,
            $data['reachable'] ?? true,
        );
    }
}
