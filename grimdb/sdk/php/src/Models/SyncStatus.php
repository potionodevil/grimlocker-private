<?php declare(strict_types=1);
namespace Grimlocker\Models;

class SyncStatus
{
    /**
     * @param SyncPeer[] $peers
     */
    public function __construct(
        public readonly array   $peers      = [],
        public readonly int     $lastSyncAt = 0,
        public readonly string  $deviceId   = '',
    ) {}

    public static function fromArray(array $data): self
    {
        $peers = array_map(
            fn(array $p) => SyncPeer::fromArray($p),
            $data['peers'] ?? [],
        );
        return new self(
            $peers,
            $data['last_sync_at'] ?? 0,
            $data['device_id']    ?? '',
        );
    }
}
