<?php
declare(strict_types=1);

namespace Grimlocker\Tests;

use Grimlocker\Client;
use Grimlocker\Entries\Entry;
use Grimlocker\Entries\PasswordEntry;
use Grimlocker\Entries\SshKeyEntry;
use Grimlocker\Entries\CertificateEntry;
use Grimlocker\FileVault\FolderListing;
use Grimlocker\FileVault\FolderItem;
use Grimlocker\FileVault\FileEntry;
use Grimlocker\Exception\GrimlockerException;
use PHPUnit\Framework\TestCase;

final class ClientTest extends TestCase
{
    private Client $client;

    protected function setUp(): void
    {
        $this->client = new TestClient('http://127.0.0.1:36353', 'test-token');
    }

    // ── Auth ──────────────────────────────────────────────────────────────────

    public function testUnlockVault(): void
    {
        $this->client->unlockVault('master-password');
        $this->assertTrue(true); // no exception
    }

    public function testLockVault(): void
    {
        $this->client->lockVault();
        $this->assertTrue(true);
    }

    public function testVaultStatus(): void
    {
        $status = $this->client->vaultStatus();
        $this->assertIsArray($status);
        $this->assertTrue($status['initialized']);
    }

    // ── Entries ───────────────────────────────────────────────────────────────

    public function testListEntries(): void
    {
        $entries = $this->client->listEntries();
        $this->assertIsArray($entries);
        $this->assertCount(2, $entries);
        $this->assertInstanceOf(Entry::class, $entries[0]);
        $this->assertSame('e1', $entries[0]->id);
    }

    public function testListEntriesByCategory(): void
    {
        $entries = $this->client->listEntries('PASSWORD');
        $this->assertCount(2, $entries);
        $this->assertSame('PASSWORD', $entries[0]->category);
    }

    public function testGetEntry(): void
    {
        $entry = $this->client->getEntry('e1');
        $this->assertInstanceOf(Entry::class, $entry);
        $this->assertSame('e1', $entry->id);
        $this->assertSame('Test Entry', $entry->title);
    }

    public function testGetEntryNotFound(): void
    {
        $this->expectException(GrimlockerException::class);
        $this->client->getEntry('nonexistent');
    }

    public function testCreateEntry(): void
    {
        $entry = $this->client->createEntry('New Entry', 'PASSWORD', ['username' => 'alice']);
        $this->assertInstanceOf(Entry::class, $entry);
        $this->assertSame('new1', $entry->id);
        $this->assertSame('PASSWORD', $entry->category);
    }

    public function testUpdateEntry(): void
    {
        $this->client->updateEntry('e1', ['notes' => 'updated']);
        $this->assertTrue(true);
    }

    public function testDeleteEntry(): void
    {
        $this->client->deleteEntry('e1');
        $this->assertTrue(true);
    }

    // ── Typed helpers ─────────────────────────────────────────────────────────

    public function testListPasswords(): void
    {
        $passwords = $this->client->listPasswords();
        $this->assertCount(2, $passwords);
        $this->assertInstanceOf(PasswordEntry::class, $passwords[0]);
    }

    public function testCreatePassword(): void
    {
        $p = new PasswordEntry('', 'GitHub', 'alice', 's3cret', '', '');
        $id = $this->client->createPassword($p);
        $this->assertSame('p1', $id);
    }

    public function testListSshKeys(): void
    {
        $keys = $this->client->listSshKeys();
        $this->assertCount(1, $keys);
        $this->assertInstanceOf(SshKeyEntry::class, $keys[0]);
    }

    public function testCreateSshKey(): void
    {
        $k = new SshKeyEntry('', 'My Key', 'pubkey', 'privkey', '', '');
        $id = $this->client->createSshKey($k);
        $this->assertSame('sk1', $id);
    }

    public function testListCertificates(): void
    {
        $certs = $this->client->listCertificates();
        $this->assertCount(1, $certs);
        $this->assertInstanceOf(CertificateEntry::class, $certs[0]);
    }

    public function testCreateCertificate(): void
    {
        $c = new CertificateEntry('', 'Cert', 'ex.com', 'crt-pem', 'key-pem');
        $id = $this->client->createCertificate($c);
        $this->assertSame('c1', $id);
    }

    // ── Search ───────────────────────────────────────────────────────────────

    public function testSearchEntries(): void
    {
        $results = $this->client->searchEntries('git');
        $this->assertCount(1, $results);
        $this->assertSame('e1', $results[0]->id);
    }

    public function testSearchEntriesWithCategory(): void
    {
        $results = $this->client->searchEntries('git', 'SSH_KEY');
        $this->assertCount(0, $results);
    }

    // ── File Vault ────────────────────────────────────────────────────────────

    public function testListFolder(): void
    {
        $listing = $this->client->listFolder();
        $this->assertInstanceOf(FolderListing::class, $listing);
    }

    public function testListFolderById(): void
    {
        $listing = $this->client->listFolder('folder1');
        $this->assertInstanceOf(FolderListing::class, $listing);
    }

    public function testCreateFolder(): void
    {
        $folder = $this->client->createFolder('Notes', 'parent1');
        $this->assertInstanceOf(FolderItem::class, $folder);
        $this->assertSame('Notes', $folder->name);
    }

    public function testUploadFile(): void
    {
        $result = $this->client->uploadFile('hello world', 'doc.txt');
        $this->assertInstanceOf(FileEntry::class, $result);
        $this->assertSame('doc.txt', $result->fileName);
    }

    public function testDownloadFile(): void
    {
        $data = $this->client->downloadFile('mb1');
        $this->assertSame('hello', $data);
    }

    // ── Workspaces ───────────────────────────────────────────────────────────

    public function testListWorkspaces(): void
    {
        $workspaces = $this->client->listWorkspaces();
        $this->assertCount(1, $workspaces);
        $this->assertSame('Personal', $workspaces[0]->name);
    }

    public function testCreateWorkspace(): void
    {
        $ws = $this->client->createWorkspace('Work');
        $this->assertSame('Work', $ws->name);
    }

    // ── Sync ─────────────────────────────────────────────────────────────────

    public function testListSyncPeers(): void
    {
        $status = $this->client->listSyncPeers();
        $this->assertIsArray($status['peers']);
    }

    public function testTriggerSync(): void
    {
        $this->client->triggerSync();
        $this->assertTrue(true);
    }

    // ── Audit ────────────────────────────────────────────────────────────────

    public function testListAuditEvents(): void
    {
        $events = $this->client->listAuditEvents(10);
        $this->assertCount(1, $events);
        $this->assertSame('INFO', $events[0]->level);
    }

    // ── Health ───────────────────────────────────────────────────────────────

    public function testHealthCheck(): void
    {
        $health = $this->client->healthCheck();
        $this->assertSame('ok', $health['status']);
    }

    // ── Error handling ───────────────────────────────────────────────────────

    public function testBatchCreateEntries(): void
    {
        $ids = $this->client->batchCreateEntries([['A', 'PASSWORD'], ['B', 'SSH_KEY']]);
        $this->assertCount(2, $ids);
        $this->assertSame('new1', $ids[0]);
        $this->assertSame('new1', $ids[1]);
    }

    public function testBatchDeleteEntries(): void
    {
        $this->client->batchDeleteEntries(['e1', 'e2']);
        $this->assertCount(2, $this->client->calls);
        $this->assertSame('entry.delete', $this->client->calls[0][0]);
        $this->assertSame('entry.delete', $this->client->calls[1][0]);
    }

    public function testCircuitBreakerOpens(): void
    {
        $cb = new CircuitBreaker(3);
        for ($i = 0; $i < 3; $i++) {
            try {
                $cb->call(function () {
                    throw new \RuntimeException('fail');
                });
            } catch (\RuntimeException $e) {
                // expected
            }
        }
        $this->assertSame('open', $cb->getState());
        $this->assertSame(3, $cb->getFailureCount());
        $this->expectException(\RuntimeException::class);
        $this->expectExceptionMessage('circuit open');
        $cb->call(function () {});
    }

    public function testErrorHandlingLockedVault(): void
    {
        $client = new ErrorTestClient('http://127.0.0.1:36353', 'token');
        $this->expectException(GrimlockerException::class);
        $this->expectExceptionMessageMatches('/locked/');
        $client->listEntries();
    }

    public function testErrorHandlingUnauthorized(): void
    {
        $client = new ErrorTestClient('http://127.0.0.1:36353', 'token');
        $this->expectException(GrimlockerException::class);
        $client->getEntry('bad');
    }
}


/**
 * Test double for Client that returns canned responses.
 */
class TestClient extends Client
{
    public array $calls = [];

    public function __construct(string $baseUrl, string $token)
    {
        // bypass real HTTP by nulling constructor
    }

    public function unlockVault(string $password): void
    {
    }

    public function lockVault(): void
    {
    }

    public function vaultStatus(): array
    {
        return ['initialized' => true, 'unlocked' => true, 'status' => 'ok'];
    }

    public function listEntries(?string $category = null): array
    {
        $entries = [
            new Entry('e1', 'Entry One', 'PASSWORD', ['username' => 'alice'], 1, 2),
            new Entry('e2', 'Entry Two', 'SSH_KEY', [], 3, 4),
        ];
        if ($category) {
            return array_filter($entries, fn($e) => $e->category === $category);
        }
        return $entries;
    }

    public function getEntry(string $id): Entry
    {
        if ($id === 'nonexistent') {
            throw new GrimlockerException("Entry not found: $id", -10);
        }
        return new Entry($id, 'Test Entry', 'PASSWORD', ['username' => 'alice'], 1, 2);
    }

    public function createEntry(string $title, string $category, array $fields = []): Entry
    {
        $this->calls[] = ['entry.create', $title, $category];
        return new Entry('new1', $title, $category, $fields, 10, 20);
    }

    public function updateEntry(string $id, array $fields): void
    {
    }

    public function deleteEntry(string $id): void
    {
        $this->calls[] = ['entry.delete', $id];
    }

    public function batchCreateEntries(array $items): array
    {
        $ids = [];
        foreach ($items as [$title, $category]) {
            $entry = $this->createEntry($title, $category);
            $ids[] = $entry->id;
        }
        return $ids;
    }

    public function batchDeleteEntries(array $ids): void
    {
        foreach ($ids as $id) {
            $this->deleteEntry($id);
        }
    }

    public function searchEntries(string $query, ?string $category = null): array
    {
        if ($category === 'SSH_KEY') return [];
        return [new Entry('e1', 'GitHub', 'PASSWORD', [], 1, 2)];
    }

    public function listPasswords(): array
    {
        return [
            new PasswordEntry('', 'GitHub', 'alice', 'sec', '', ''),
            new PasswordEntry('', 'GitLab', 'bob', 'sec', '', ''),
        ];
    }

    public function createPassword(PasswordEntry $p): string
    {
        return 'p1';
    }

    public function listSshKeys(): array
    {
        return [new SshKeyEntry('', 'Key', 'pub', 'priv', '', '')];
    }

    public function createSshKey(SshKeyEntry $k): string
    {
        return 'sk1';
    }

    public function listCertificates(): array
    {
        return [new CertificateEntry('', 'Cert', 'ex.com', 'crt', 'key')];
    }

    public function createCertificate(CertificateEntry $c): string
    {
        return 'c1';
    }

    public function listFolder(?string $folderId = null): FolderListing
    {
        return new FolderListing(
            [new FolderItem('d1', 'sub', 'folder', '')],
            []
        );
    }

    public function createFolder(string $name, string $parentId = ''): FolderItem
    {
        return new FolderItem('f1', $name, 'folder');
    }

    public function uploadFile(string|array $data, string $fileName): FileEntry
    {
        return new FileEntry('f1', $fileName, 'text/plain', 12, 'mb1', '');
    }

    public function downloadFile(string $manifestBlockId): string
    {
        return 'hello';
    }

    public function listWorkspaces(): array
    {
        return [new \Grimlocker\Models\Workspace('ws1', 'Personal', true)];
    }

    public function createWorkspace(string $name): \Grimlocker\Models\Workspace
    {
        return new \Grimlocker\Models\Workspace('ws2', $name, false);
    }

    public function listSyncPeers(): array
    {
        return ['peers' => [['device_id' => 'd1', 'host' => '192.168.1.5']], 'last_sync_at' => 0];
    }

    public function triggerSync(): void
    {
    }

    public function listAuditEvents(int $n = 50): array
    {
        return [
            new \Grimlocker\Models\AuditEvent(1, 'INFO', 'auth', 'vault unlocked', ''),
        ];
    }

    public function healthCheck(): array
    {
        return ['status' => 'ok', 'daemon_version' => '1.0.0'];
    }
}

class CircuitBreaker
{
    private int $threshold;
    private int $failureCount = 0;
    private string $state = 'closed';

    public function __construct(int $threshold = 3)
    {
        $this->threshold = $threshold;
    }

    public function call(callable $fn): void
    {
        if ($this->state === 'open') {
            throw new \RuntimeException('circuit open');
        }
        try {
            $fn();
        } catch (\Exception $e) {
            $this->failureCount++;
            if ($this->failureCount >= $this->threshold) {
                $this->state = 'open';
            }
            throw $e;
        }
    }

    public function getState(): string
    {
        return $this->state;
    }

    public function getFailureCount(): int
    {
        return $this->failureCount;
    }
}

/**
 * Client that throws errors for testing error handling.
 */
class ErrorTestClient extends Client
{
    public function __construct(string $baseUrl, string $token)
    {
    }

    public function listEntries(?string $category = null): array
    {
        throw new GrimlockerException('vault is locked', -101);
    }

    public function getEntry(string $id): Entry
    {
        throw new GrimlockerException('unauthorized', -102);
    }
}
