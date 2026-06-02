<?php declare(strict_types=1);
namespace Grimlocker\Entries;

class SshKeyEntry
{
    public function __construct(
        public readonly string $id         = '',
        public readonly string $title      = '',
        public readonly string $publicKey  = '',
        public readonly string $privateKey = '',
        public readonly string $username   = '',
        public readonly string $passphrase = '',
    ) {}

    public static function fromEntry(Entry $e): self
    {
        return new self(
            $e->id,
            $e->title,
            $e->field('public_key'),
            $e->field('private_key'),
            $e->field('username'),
            $e->field('passphrase'),
        );
    }

    /** @return array<string,string> */
    public function toFields(): array
    {
        return [
            'public_key'  => $this->publicKey,
            'private_key' => $this->privateKey,
            'username'    => $this->username,
            'passphrase'  => $this->passphrase,
        ];
    }
}
