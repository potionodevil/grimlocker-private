<?php
declare(strict_types=1);

namespace Grimlocker;

use Grimlocker\Entries\{Entry, PasswordEntry, SshKeyEntry, CertificateEntry};
use Grimlocker\FileVault\{FileVaultClient, FolderListing, FolderItem, FileEntry};
use Grimlocker\Models\{Workspace, SyncStatus, AuditEvent};
use Grimlocker\Exception\GrimlockerException;
use Grimlocker\Exception\CircuitBreakerOpenException;

/**
 * HTTP client for the Grimlocker daemon /api/v1 endpoint.
 *
 * @example
 * $client = new Grimlocker\Client('http://127.0.0.1:36353', getenv('GRIMLOCKER_TOKEN'));
 * $client->unlockVault('master-password');
 * $passwords = $client->listPasswords();
 */
class Client
{
    private string $baseUrl;
    private string $token;
    private int    $timeout;
    private int    $failureCount = 0;
    private ?float $circuitOpenUntil = null;

    private const MAX_RETRIES = 3;
    private const BASE_DELAY_MS = 100;
    private const CIRCUIT_FAILURE_THRESHOLD = 5;
    private const CIRCUIT_OPEN_SECONDS = 30;

    public function __construct(string $baseUrl, string $token, int $timeout = 30)
    {
        $this->baseUrl = rtrim($baseUrl, '/');
        $this->token   = $token;
        $this->timeout = $timeout;
    }

    // ── Auth ──────────────────────────────────────────────────────────────────

    public function unlockVault(string $password): void
    {
        $this->call('vault.unlock', ['password' => $password]);
    }

    public function lockVault(): void
    {
        $this->call('vault.logout', []);
    }

    /** @return array{initialized: bool, unlocked: bool, status: string} */
    public function vaultStatus(): array
    {
        return $this->call('vault.status', []);
    }

    // ── Entries ───────────────────────────────────────────────────────────────

    /** @return Entry[] */
    public function listEntries(?string $category = null): array
    {
        $action  = $category ? 'entry.query' : 'entry.list';
        $payload = $category ? ['category' => $category] : [];
        return $this->parseEntries($this->call($action, $payload));
    }

    public function getEntry(string $id): Entry
    {
        $entries = $this->parseEntries($this->call('entry.read', ['id' => $id]));
        if (empty($entries)) throw new GrimlockerException("Entry not found: $id", -10);
        return $entries[0];
    }

    /** @param array<string,string> $fields */
    public function createEntry(string $title, string $category, array $fields = []): Entry
    {
        $entries = $this->parseEntries($this->call('entry.create', compact('title', 'category', 'fields')));
        if (empty($entries)) throw new GrimlockerException("Create returned no entry", -30);
        return $entries[0];
    }

    /** @param array<string,string> $fields */
    public function updateEntry(string $id, array $fields): void
    {
        $this->call('entry.update', compact('id', 'fields'));
    }

    public function deleteEntry(string $id): void
    {
        $this->call('entry.delete', ['id' => $id]);
    }

    /** @param array<int,array{title:string,category:string,fields?:array<string,string>}> $entries */
    public function createEntriesBatch(array $entries): array
    {
        $ids = [];
        foreach ($entries as $entry) {
            $created = $this->createEntry($entry['title'], $entry['category'], $entry['fields'] ?? []);
            $ids[] = $created->id;
        }
        return $ids;
    }

    /** @param string[] $ids */
    public function deleteEntriesBatch(array $ids): void
    {
        foreach ($ids as $id) {
            $this->deleteEntry($id);
        }
    }

    /** @return Entry[] */
    public function searchEntries(string $query, ?string $category = null): array
    {
        $payload = array_filter(['query' => $query, 'category' => $category]);
        return $this->parseEntries($this->call('entry.search', $payload));
    }

    // ── Typed helpers ─────────────────────────────────────────────────────────

    /** @return PasswordEntry[] */
    public function listPasswords(): array
    {
        return array_map(fn(Entry $e) => PasswordEntry::fromEntry($e), $this->listEntries('PASSWORD'));
    }

    public function createPassword(PasswordEntry $p): string
    {
        return $this->createEntry($p->title, 'PASSWORD', $p->toFields())->id;
    }

    /** @return SshKeyEntry[] */
    public function listSshKeys(): array
    {
        return array_map(fn(Entry $e) => SshKeyEntry::fromEntry($e), $this->listEntries('SSH_KEY'));
    }

    public function createSshKey(SshKeyEntry $k): string
    {
        return $this->createEntry($k->title, 'SSH_KEY', $k->toFields())->id;
    }

    /** @return CertificateEntry[] */
    public function listCertificates(): array
    {
        return array_map(fn(Entry $e) => CertificateEntry::fromEntry($e), $this->listEntries('CERTIFICATE'));
    }

    public function createCertificate(CertificateEntry $c): string
    {
        return $this->createEntry($c->title, 'CERTIFICATE', $c->toFields())->id;
    }

    // ── File Vault ────────────────────────────────────────────────────────────

    public function listFolder(string $folderId = ''): FolderListing
    {
        return FolderListing::fromArray($this->call('file.list_folder', ['folder_id' => $folderId]));
    }

    public function createFolder(string $name, string $parentId = ''): FolderItem
    {
        $r = $this->call('file.create_folder', ['name' => $name, 'parent_id' => $parentId]);
        return new FolderItem($r['id'] ?? '', $name, 'folder');
    }

    public function renameFolder(string $id, string $name): void
    {
        $this->call('file.rename_folder', compact('id', 'name'));
    }

    public function deleteFolder(string $id): void
    {
        $this->call('file.delete_folder', ['id' => $id]);
    }

    public function moveFile(string $manifestBlockId, string $folderId): void
    {
        $this->call('file.move', ['manifest_block_id' => $manifestBlockId, 'folder_id' => $folderId]);
    }

    /**
     * @param callable|null $onProgress fn(int $bytesSent, int $totalBytes): void
     */
    public function uploadFile(
        string $data, string $fileName,
        string $mimeType = 'application/octet-stream',
        string $folderId = '',
        ?callable $onProgress = null
    ): FileEntry {
        if ($onProgress) ($onProgress)(0, strlen($data));
        $r = $this->call('file.ingest', [
            'file_name' => $fileName, 'mime_type' => $mimeType,
            'folder_id' => $folderId, 'data_b64'  => base64_encode($data),
        ]);
        if ($onProgress) ($onProgress)(strlen($data), strlen($data));
        return FileEntry::fromArray($r);
    }

    public function downloadFile(string $manifestBlockId): string
    {
        $r = $this->call('file.download', ['manifest_block_id' => $manifestBlockId]);
        return base64_decode($r['data_b64'] ?? '');
    }

    // ── Workspaces ────────────────────────────────────────────────────────────

    /** @return Workspace[] */
    public function listWorkspaces(): array
    {
        $data = $this->call('workspace.list', []);
        return array_map(fn($w) => Workspace::fromArray($w), is_array($data) ? $data : []);
    }

    public function createWorkspace(string $name): Workspace
    {
        return Workspace::fromArray($this->call('workspace.create', compact('name')));
    }

    public function switchWorkspace(string $id): void { $this->call('workspace.switch', compact('id')); }
    public function renameWorkspace(string $id, string $name): void { $this->call('workspace.rename', compact('id', 'name')); }
    public function deleteWorkspace(string $id): void { $this->call('workspace.delete', compact('id')); }

    // ── Sync ──────────────────────────────────────────────────────────────────

    public function listSyncPeers(): SyncStatus
    {
        return SyncStatus::fromArray($this->call('sync.list_peers', []));
    }

    public function triggerSync(): void { $this->call('sync.trigger', []); }

    // ── Audit ─────────────────────────────────────────────────────────────────

    /** @return AuditEvent[] */
    public function listAuditEvents(int $n = 50): array
    {
        $data = $this->call('audit.list', ['n' => $n]);
        return array_map(fn($e) => AuditEvent::fromArray($e), is_array($data) ? $data : []);
    }

    // ── Health ────────────────────────────────────────────────────────────────

    public function generateSSHKey(string $comment = '', bool $saveToVault = true): array
    {
        return $this->call('tool.ssh_keygen', ['comment' => $comment, 'save_to_vault' => $saveToVault]);
    }

    public function getRecoveryPhrase(string $password): string
    {
        $r = $this->call('vault.recovery_phrase', ['password' => $password]);
        return $r['recovery_phrase'] ?? $r['phrase'] ?? '';
    }

    public function healthCheck(): array { return $this->vaultStatus(); }

    // ── Internal ─────────────────────────────────────────────────────────────

    private function call(string $action, array $payload): array
    {
        $now = microtime(true);
        if ($this->circuitOpenUntil !== null && $now < $this->circuitOpenUntil) {
            throw new CircuitBreakerOpenException('Circuit breaker is open');
        }
        $this->circuitOpenUntil = null;

        $lastError = null;

        for ($attempt = 0; $attempt <= self::MAX_RETRIES; $attempt++) {
            $ch = curl_init($this->baseUrl . '/api/v1');
            $body = json_encode(['action' => $action, 'payload' => $payload]);

            curl_setopt_array($ch, [
                CURLOPT_POST           => true,
                CURLOPT_POSTFIELDS     => $body,
                CURLOPT_RETURNTRANSFER => true,
                CURLOPT_TIMEOUT        => $this->timeout,
                CURLOPT_HTTPHEADER     => [
                    'Content-Type: application/json',
                    'X-Grimlocker-Token: ' . $this->token,
                ],
            ]);

            $resp    = curl_exec($ch);
            $code    = curl_getinfo($ch, CURLINFO_HTTP_CODE);
            $curlErr = curl_error($ch);
            curl_close($ch);

            if ($curlErr) {
                $this->recordFailure();
                $lastError = new GrimlockerException("cURL error: $curlErr");
                if ($attempt === self::MAX_RETRIES) {
                    throw $lastError;
                }
                $delay = min(self::BASE_DELAY_MS * (1 << $attempt), 2000);
                usleep($delay * 1000);
                continue;
            }

            $data = json_decode($resp ?: '{}', true);
            if (json_last_error() !== JSON_ERROR_NONE) {
                $this->recordFailure();
                $lastError = new GrimlockerException("Invalid JSON response");
                if ($attempt === self::MAX_RETRIES) {
                    throw $lastError;
                }
                $delay = min(self::BASE_DELAY_MS * (1 << $attempt), 2000);
                usleep($delay * 1000);
                continue;
            }

            if ($code < 200 || $code >= 300) {
                $ec  = $data['error_code'] ?? 0;
                $msg = $data['error'] ?? "HTTP $code";
                $lastError = new GrimlockerException(GrimlockerException::nameOf($ec) . ": $msg", $ec);

                if ($code >= 400 && $code < 500) {
                    $this->failureCount = 0;
                    throw $lastError;
                }

                if ($code >= 500) {
                    $this->recordFailure();
                    if ($attempt === self::MAX_RETRIES) {
                        throw $lastError;
                    }
                    $delay = min(self::BASE_DELAY_MS * (1 << $attempt), 2000);
                    usleep($delay * 1000);
                    continue;
                }

                throw $lastError;
            }

            $this->failureCount = 0;
            return $data;
        }

        throw $lastError ?? new GrimlockerException("Request failed after retries");
    }

    private function recordFailure(): void
    {
        $this->failureCount++;
        if ($this->failureCount >= self::CIRCUIT_FAILURE_THRESHOLD) {
            $this->circuitOpenUntil = microtime(true) + self::CIRCUIT_OPEN_SECONDS;
        }
    }

    /** @return Entry[] */
    private function parseEntries(array $data): array
    {
        $arr = isset($data['entries']) ? $data['entries'] : (array_is_list($data) ? $data : []);
        return array_map(fn($e) => Entry::fromArray($e), $arr);
    }
}
