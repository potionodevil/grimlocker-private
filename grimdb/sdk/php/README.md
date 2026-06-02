# grimlocker/sdk (PHP)

PHP 8.1+ SDK for the Grimlocker daemon. Uses `ext-curl` (standard PHP extension).

## Installation

```
composer require grimlocker/sdk
```

## Quick Start

```php
use Grimlocker\Client;
use Grimlocker\Entries\PasswordEntry;

$client = new Client('http://127.0.0.1:36353', getenv('GRIMLOCKER_TOKEN'));
$client->unlockVault('master-password');

// Passwords
foreach ($client->listPasswords() as $p) {
    echo "{$p->title}: {$p->username}\n";
}
$id = $client->createPassword(new PasswordEntry(title: 'GitHub', username: 'me@example.com', password: 's3cr3t', url: 'https://github.com'));

// File vault
$listing = $client->listFolder();
$folder  = $client->createFolder('Documents');
$file    = $client->uploadFile(file_get_contents('secret.pdf'), 'secret.pdf', 'application/pdf',
    folderId: $folder->id,
    onProgress: fn($sent, $total) => print("$sent/$total bytes\n"));

// Sync + Audit
$sync   = $client->listSyncPeers();
$events = $client->listAuditEvents(n: 20);

// Workspaces
foreach ($client->listWorkspaces() as $w) { echo $w->name . "\n"; }
```
