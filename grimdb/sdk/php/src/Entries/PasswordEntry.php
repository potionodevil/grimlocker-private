<?php declare(strict_types=1);
namespace Grimlocker\Entries;

class PasswordEntry
{
    public function __construct(
        public readonly string $id       = '',
        public readonly string $title    = '',
        public readonly string $username = '',
        public readonly string $password = '',
        public readonly string $url      = '',
        public readonly string $notes    = '',
    ) {}

    public static function fromEntry(Entry $e): self
    {
        return new self(
            $e->id,
            $e->title,
            $e->field('username'),
            $e->field('password'),
            $e->field('url'),
            $e->field('notes'),
        );
    }

    /** @return array<string,string> */
    public function toFields(): array
    {
        return [
            'username' => $this->username,
            'password' => $this->password,
            'url'      => $this->url,
            'notes'    => $this->notes,
        ];
    }
}
