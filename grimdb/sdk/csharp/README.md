# Grimlocker.SDK (C# / .NET)

.NET 8 SDK for the Grimlocker Zero-Trust Vault. Uses `System.Text.Json` and `HttpClient` — no external dependencies.

## Installation

```
dotnet add package Grimlocker.SDK
```

## Quick Start

```csharp
using Grimlocker;
using Grimlocker.Models;

await using var client = new GrimlockerClient("http://127.0.0.1:36353", Environment.GetEnvironmentVariable("GRIMLOCKER_TOKEN")!);
await client.UnlockVaultAsync("master-password");

// Passwords
var passwords = await client.ListPasswordsAsync();
var id = await client.CreatePasswordAsync(new PasswordEntry("", "GitHub", "me@example.com", "s3cr3t", "https://github.com", ""));

// File Vault
var listing = await client.ListFolderAsync();
var file    = await client.UploadFileAsync(File.ReadAllBytes("secret.pdf"), "secret.pdf", "application/pdf",
    progress: new Progress<(long s, long t)>(p => Console.WriteLine($"{p.s * 100 / p.t}%")));

// Sync + Audit
var sync   = await client.ListSyncPeersAsync();
var events = await client.ListAuditEventsAsync(n: 20);

// Workspaces
var workspaces = await client.ListWorkspacesAsync();
await client.SwitchWorkspaceAsync(workspaces[0].Id);
```

## API Reference

All methods are `async Task`/`async Task<T>` and accept a `CancellationToken`.
See [GrimlockerClient.cs](src/GrimlockerClient.cs) for complete API.
