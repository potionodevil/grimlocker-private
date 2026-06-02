<?php declare(strict_types=1);
namespace Grimlocker\Models;

class Workspace
{
    public function __construct(
        public readonly string $id,
        public readonly string $name,
        public readonly bool   $isDefault = false,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            $data['id']         ?? '',
            $data['name']       ?? '',
            $data['is_default'] ?? false,
        );
    }
}
