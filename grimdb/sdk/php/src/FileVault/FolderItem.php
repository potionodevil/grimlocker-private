<?php declare(strict_types=1);
namespace Grimlocker\FileVault;

class FolderItem
{
    public function __construct(
        public readonly string $id,
        public readonly string $name,
        public readonly string $kind = 'folder',
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            $data['id']   ?? '',
            $data['name'] ?? '',
            'folder',
        );
    }
}
