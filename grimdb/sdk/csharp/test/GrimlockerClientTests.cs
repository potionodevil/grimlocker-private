using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using Grimlocker.Models;
using Xunit;

namespace Grimlocker.Tests;

public class MockHttpMessageHandler : HttpMessageHandler
{
    private readonly Queue<HttpResponseMessage> _responses;

    public MockHttpMessageHandler(params HttpResponseMessage[] responses)
    {
        _responses = new Queue<HttpResponseMessage>(responses);
    }

    public System.Collections.Generic.IReadOnlyList<HttpRequestMessage> Requests { get; } = new System.Collections.Generic.List<HttpRequestMessage>();

    protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
    {
        ((System.Collections.Generic.List<HttpRequestMessage>)Requests).Add(request);
        var response = _responses.Count > 0 ? _responses.Dequeue() : new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = JsonContent.Create(new { success = true })
        };
        return Task.FromResult(response);
    }
}

public class GrimlockerClientTests
{
    private static readonly JsonSerializerOptions JsonOpts = new() { PropertyNameCaseInsensitive = true };

    private static MockHttpMessageHandler CreateHandler(params (HttpStatusCode status, object body)[] responses)
    {
        var msgs = responses.Select(r => new HttpResponseMessage(r.status)
        {
            Content = JsonContent.Create(r.body)
        }).ToArray();
        return new MockHttpMessageHandler(msgs);
    }

    [Fact]
    public async Task UnlockVaultAsync_SendsCorrectPayload()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { success = true }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        await client.UnlockVaultAsync("master-password");

        var lastRequest = handler.Requests[0];
        var body = await lastRequest.Content!.ReadFromJsonAsync<System.Text.Json.JsonElement>();
        Assert.Equal("vault.unlock", body.GetProperty("action").GetString());
        Assert.Equal("master-password", body.GetProperty("payload").GetProperty("password").GetString());
    }

    [Fact]
    public async Task LockVaultAsync_SendsLogout()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { success = true }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        await client.LockVaultAsync();

        var body = await handler.Requests[0].Content!.ReadFromJsonAsync<JsonElement>();
        Assert.Equal("vault.logout", body.GetProperty("action").GetString());
    }

    [Fact]
    public async Task VaultStatusAsync_ReturnsStatus()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { initialized = true, unlocked = true, status = "ok" }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var status = await client.VaultStatusAsync();

        Assert.True(status.Initialized);
        Assert.True(status.Unlocked);
    }

    [Fact]
    public async Task ListEntriesAsync_WithoutCategory()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new[] {
            new { id = "e1", title = "Entry 1", category = "PASSWORD", created_at = 1L, updated_at = 2L }
        }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var entries = await client.ListEntriesAsync();

        Assert.Single(entries);
        Assert.Equal("e1", entries[0].Id);
    }

    [Fact]
    public async Task ListEntriesAsync_WithCategory()
    {
        var handler = CreateHandler((HttpStatusCode.OK, Array.Empty<object>()));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        await client.ListEntriesAsync("SSH_KEY");

        var body = await handler.Requests[0].Content!.ReadFromJsonAsync<JsonElement>();
        Assert.Equal("entry.query", body.GetProperty("action").GetString());
    }

    [Fact]
    public async Task GetEntryAsync_ReturnsEntry()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { id = "e1", title = "Entry", category = "PASSWORD" }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var entry = await client.GetEntryAsync("e1");

        Assert.Equal("e1", entry.Id);
    }

    [Fact]
    public async Task GetEntryAsync_ThrowsOnNull()
    {
        var handler = CreateHandler((HttpStatusCode.OK, null!));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        await Assert.ThrowsAsync<GrimlockerException>(() => client.GetEntryAsync("bad"));
    }

    [Fact]
    public async Task CreateEntryAsync_ReturnsCreatedEntry()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { id = "new1", title = "New", category = "PASSWORD" }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var entry = await client.CreateEntryAsync("New", "PASSWORD", new Dictionary<string, string> { ["username"] = "alice" });

        Assert.Equal("new1", entry.Id);
    }

    [Fact]
    public async Task UpdateEntryAsync_SendsUpdate()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { success = true }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        await client.UpdateEntryAsync("e1", new Dictionary<string, string> { ["notes"] = "updated" });

        var body = await handler.Requests[0].Content!.ReadFromJsonAsync<JsonElement>();
        Assert.Equal("entry.update", body.GetProperty("action").GetString());
        Assert.Equal("e1", body.GetProperty("payload").GetProperty("id").GetString());
    }

    [Fact]
    public async Task DeleteEntryAsync_SendsDelete()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { success = true }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        await client.DeleteEntryAsync("e1");

        var body = await handler.Requests[0].Content!.ReadFromJsonAsync<JsonElement>();
        Assert.Equal("entry.delete", body.GetProperty("action").GetString());
        Assert.Equal("e1", body.GetProperty("payload").GetProperty("id").GetString());
    }

    [Fact]
    public async Task SearchEntriesAsync_ReturnsResults()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new[] {
            new { id = "e1", title = "GitHub", category = "PASSWORD" }
        }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var results = await client.SearchEntriesAsync("git");

        Assert.Single(results);
    }

    [Fact]
    public async Task ListPasswordsAsync_ReturnsTypedEntries()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new[] {
            new { id = "p1", title = "GitHub", category = "PASSWORD", fields = new Dictionary<string, string> { ["username"] = "a", ["password"] = "b", ["url"] = "", ["notes"] = "" } }
        }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var passwords = await client.ListPasswordsAsync();

        Assert.Single(passwords);
        Assert.Equal("GitHub", passwords[0].Title);
    }

    [Fact]
    public async Task CreatePasswordAsync_ReturnsId()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { id = "p1", title = "GitHub", category = "PASSWORD" }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var p = new Models.PasswordEntry { Title = "GitHub", Username = "alice", Password = "sec" };
        var id = await client.CreatePasswordAsync(p);

        Assert.Equal("p1", id);
    }

    [Fact]
    public async Task ListSshKeysAsync_ReturnsKeys()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new[] {
            new { id = "sk1", title = "Key", category = "SSH_KEY", fields = new Dictionary<string, string> { ["public_key"] = "pk", ["private_key"] = "", ["username"] = "", ["passphrase"] = "", ["comment"] = "" } }
        }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var keys = await client.ListSshKeysAsync();

        Assert.Single(keys);
    }

    [Fact]
    public async Task CreateSshKeyAsync_ReturnsId()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { id = "sk1", title = "Key", category = "SSH_KEY" }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var k = new Models.SshKeyEntry { Title = "Key", PublicKey = "pk", PrivateKey = "priv" };
        var id = await client.CreateSshKeyAsync(k);

        Assert.Equal("sk1", id);
    }

    [Fact]
    public async Task ListCertificatesAsync_ReturnsCerts()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new[] {
            new { id = "c1", title = "Cert", category = "CERTIFICATE", fields = new Dictionary<string, string> { ["domain"] = "ex.com", ["certificate"] = "crt", ["private_key"] = "key" } }
        }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var certs = await client.ListCertificatesAsync();

        Assert.Single(certs);
    }

    [Fact]
    public async Task CreateCertificateAsync_ReturnsId()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { id = "c1", title = "Cert", category = "CERTIFICATE" }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var c = new Models.CertificateEntry { Title = "Cert", Domain = "ex.com", Certificate = "crt", PrivateKey = "key" };
        var id = await client.CreateCertificateAsync(c);

        Assert.Equal("c1", id);
    }

    [Fact]
    public async Task ListFolderAsync_ReturnsListing()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { folders = Array.Empty<object>(), files = Array.Empty<object>() }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var listing = await client.ListFolderAsync();

        Assert.NotNull(listing);
    }

    [Fact]
    public async Task CreateFolderAsync_ReturnsFolder()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { id = "f1", name = "Notes" }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var folder = await client.CreateFolderAsync("Notes");

        Assert.Equal("Notes", folder.Name);
    }

    [Fact]
    public async Task UploadFileAsync_ReturnsFileEntry()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { id = "f1", file_name = "doc.txt", mime_type = "text/plain", total_size = 12, manifest_block_id = "mb1", folder_id = "" }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var result = await client.UploadFileAsync("hello world"u8.ToArray(), "doc.txt", "text/plain");

        Assert.Equal("doc.txt", result.FileName);
    }

    [Fact]
    public async Task DownloadFileAsync_ReturnsData()
    {
        var b64 = Convert.ToBase64String("hello"u8.ToArray());
        var handler = CreateHandler((HttpStatusCode.OK, new { data_b64 = b64 }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var data = await client.DownloadFileAsync("mb1");

        Assert.Equal(5, data.Length);
    }

    [Fact]
    public async Task ListWorkspacesAsync_ReturnsWorkspaces()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new[] {
            new { id = "ws1", name = "Personal", is_default = true }
        }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var workspaces = await client.ListWorkspacesAsync();

        Assert.Single(workspaces);
        Assert.Equal("Personal", workspaces[0].Name);
    }

    [Fact]
    public async Task CreateWorkspaceAsync_ReturnsWorkspace()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { id = "ws2", name = "Work" }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var ws = await client.CreateWorkspaceAsync("Work");

        Assert.Equal("Work", ws.Name);
    }

    [Fact]
    public async Task ListSyncPeersAsync_ReturnsStatus()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { peers = Array.Empty<object>(), last_sync_at = 0L, device_id = "d1" }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var status = await client.ListSyncPeersAsync();

        Assert.NotNull(status);
    }

    [Fact]
    public async Task TriggerSyncAsync_SendsTrigger()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { success = true }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        await client.TriggerSyncAsync();

        var body = await handler.Requests[0].Content!.ReadFromJsonAsync<JsonElement>();
        Assert.Equal("sync.trigger", body.GetProperty("action").GetString());
    }

    [Fact]
    public async Task ListAuditEventsAsync_ReturnsEvents()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new[] {
            new { timestamp = 1L, level = "INFO", module = "auth", message = "unlock", subject_id = "" }
        }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var events = await client.ListAuditEventsAsync();

        Assert.Single(events);
    }

    [Fact]
    public async Task HealthCheckAsync_ReturnsHealth()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { status = "ok", daemon_version = "1.0.0", vault_initialized = true, vault_unlocked = true }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var health = await client.HealthCheckAsync();

        Assert.Equal("ok", health.Status);
    }

    [Fact]
    public async Task GenerateSshKeyAsync_ReturnsKey()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { public_key = "ssh-ed25519 AAA", fingerprint = "SHA256:abc", entry_id = "e1" }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        var result = await client.GenerateSshKeyAsync();

        Assert.Contains("ssh-ed25519", result.PublicKey);
    }

    [Fact]
    public async Task ErrorHandling_ThrowsOnError()
    {
        var handler = CreateHandler((HttpStatusCode.Unauthorized, new { error = "Invalid token" }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        await Assert.ThrowsAsync<GrimlockerException>(() => client.ListEntriesAsync());
    }

    [Fact]
    public async Task MoveFileAsync_SendsMove()
    {
        var handler = CreateHandler((HttpStatusCode.OK, new { success = true }));
        var client = new TestableGrimlockerClient("http://127.0.0.1:36353", "token", handler);

        await client.MoveFileAsync("mb1", "folder1");

        var body = await handler.Requests[0].Content!.ReadFromJsonAsync<JsonElement>();
        Assert.Equal("file.move", body.GetProperty("action").GetString());
    }

    /// <summary>
    /// Testable client that accepts a custom HttpMessageHandler.
    /// </summary>
    private class TestableGrimlockerClient : GrimlockerClient
    {
        public TestableGrimlockerClient(string baseUrl, string token, HttpMessageHandler handler)
            : base(baseUrl, token)
        {
            // Replace the HttpClient's handler
            var httpField = typeof(GrimlockerClient)
                .GetField("_http", System.Reflection.BindingFlags.NonPublic | System.Reflection.BindingFlags.Instance)!;
            var newHttp = new HttpClient(handler);
            newHttp.DefaultRequestHeaders.Add("X-Grimlocker-Token", token);
            httpField.SetValue(this, newHttp);
        }
    }
}
