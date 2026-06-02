<?php declare(strict_types=1);
namespace Grimlocker\FileVault;

class FileEntry
{
    public function __construct(
        public readonly string $id,
        public readonly string $fileName,
        public readonly string $mimeType,
        public readonly int    $totalSize,
        public readonly string $manifestBlockId,
        public readonly string $folderId,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            $data['id']                ?? '',
            $data['file_name']         ?? '',
            $data['mime_type']         ?? '',
            $data['total_size']        ?? 0,
            $data['manifest_block_id'] ?? '',
            $data['folder_id']         ?? '',
        );
    }
}
