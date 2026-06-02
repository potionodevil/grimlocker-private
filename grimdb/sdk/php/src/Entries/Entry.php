<?php declare(strict_types=1);
namespace Grimlocker\Entries;

/**
 * Generic vault entry holding structured key/value fields.
 *
 * Returned by entry.list / entry.read / entry.search.  For typed access
 * use the specialised entry classes (PasswordEntry, SshKeyEntry, …).
 */
class Entry
{
    /**
     * @param array<string,string> $fields
     */
    public function __construct(
        public readonly string $id,
        public readonly string $title,
        public readonly string $category,
        public readonly array  $fields,
        public readonly int    $createdAt = 0,
        public readonly int    $updatedAt = 0,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            $data['id']         ?? '',
            $data['title']      ?? '',
            $data['category']   ?? '',
            $data['fields']     ?? [],
            $data['created_at'] ?? 0,
            $data['updated_at'] ?? 0,
        );
    }

    public function field(string $key): string
    {
        return $this->fields[$key] ?? '';
    }
}
