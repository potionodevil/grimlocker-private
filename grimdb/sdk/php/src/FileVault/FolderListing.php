<?php declare(strict_types=1);
namespace Grimlocker\FileVault;

class FolderListing
{
    /**
     * @param FolderItem[] $folders
     * @param FileEntry[]  $files
     */
    public function __construct(
        public readonly array $folders,
        public readonly array $files,
    ) {}

    public static function fromArray(array $data): self
    {
        $folders = array_map(
            fn(array $f) => FolderItem::fromArray($f),
            $data['folders'] ?? [],
        );
        $files = array_map(
            fn(array $f) => FileEntry::fromArray($f),
            $data['files'] ?? [],
        );
        return new self($folders, $files);
    }
}
