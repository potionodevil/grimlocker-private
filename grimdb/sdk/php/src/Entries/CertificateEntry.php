<?php declare(strict_types=1);
namespace Grimlocker\Entries;

class CertificateEntry
{
    public function __construct(
        public readonly string $id          = '',
        public readonly string $title       = '',
        public readonly string $domain      = '',
        public readonly string $certificate = '',
        public readonly string $privateKey  = '',
    ) {}

    public static function fromEntry(Entry $e): self
    {
        return new self(
            $e->id,
            $e->title,
            $e->field('domain'),
            $e->field('certificate'),
            $e->field('private_key'),
        );
    }

    /** @return array<string,string> */
    public function toFields(): array
    {
        return [
            'domain'      => $this->domain,
            'certificate' => $this->certificate,
            'private_key' => $this->privateKey,
        ];
    }
}
