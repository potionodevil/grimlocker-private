using System.Net.Http.Json;
using System.Text.Json;
using Grimlocker.Models;

namespace Grimlocker;

/// <summary>
/// Async HTTP client for the Grimlocker vault daemon.
/// Uses the /api/v1 JSON endpoint — no external dependencies.
/// </summary>
/// <example>
/// <code>
/// await using var client = new GrimlockerClient("http://127.0.0.1:36353", token);
/// await client.UnlockVaultAsync("master-password");
/// var passwords = await client.ListPasswordsAsync();
/// </code>
/// </example>
public class GrimlockerClient : IAsyncDisposable
{
    private readonly HttpClient   _http;
    private readonly string       _baseUrl;
    private static readonly JsonSerializerOptions _json = new()
        { PropertyNameCaseInsensitive = true };

    private readonly object _circuitLock = new();
    private int _consecutiveFailures = 0;
    private DateTimeOffset _circuitOpenUntil = DateTimeOffset.MinValue;

    /// <param name="baseUrl">Daemon base URL, e.g. "http://127.0.0.1:36353"</param>
    /// <param name="token">GRIMLOCKER_TOKEN from daemon stdout</param>
    public GrimlockerClient(string baseUrl, string token)
    {
        _baseUrl = baseUrl.TrimEnd('/');
        _http = new HttpClient();
        _http.DefaultRequestHeaders.Add("X-Grimlocker-Token", token);
    }

    // ── Auth ──────────────────────────────────────────────────────────────────

    public Task UnlockVaultAsync(string password, CancellationToken ct = default)
        => CallAsync("vault.unlock", new { password }, ct);

    public Task LockVaultAsync(CancellationToken ct = default)
        => CallAsync("vault.logout", new { }, ct);

    public async Task<VaultStatus> VaultStatusAsync(CancellationToken ct = default)
        => await CallAsync<VaultStatus>("vault.status", new { }, ct);

    // ── Entries ───────────────────────────────────────────────────────────────

    public async Task<IReadOnlyList<VaultEntry>> ListEntriesAsync(string? category = null, CancellationToken ct = default)
    {
        var payload = category is null ? new { } as object : new { category };
        var action  = category is null ? "entry.list" : "entry.query";
        return await CallAsync<List<VaultEntry>>(action, payload, ct) ?? [];
    }

    public async Task<VaultEntry> GetEntryAsync(string id, CancellationToken ct = default)
        => await CallAsync<VaultEntry>("entry.read", new { id }, ct)
           ?? throw new GrimlockerException($"Entry not found: {id}", -10);

    public async Task<VaultEntry> CreateEntryAsync(string title, string category,
        Dictionary<string, string> fields, CancellationToken ct = default)
        => await CallAsync<VaultEntry>("entry.create", new { title, category, fields }, ct)
           ?? throw new GrimlockerException("Create returned no entry", -30);

    public Task UpdateEntryAsync(string id, Dictionary<string, string> fields, CancellationToken ct = default)
        => CallAsync("entry.update", new { id, fields }, ct);

    public Task DeleteEntryAsync(string id, CancellationToken ct = default)
        => CallAsync("entry.delete", new { id }, ct);

    public async Task<IReadOnlyList<string>> CreateEntriesBatchAsync(IReadOnlyList<BatchEntryInput> items, CancellationToken ct = default)
    {
        var ids = new List<string>(items.Count);
        foreach (var item in items)
        {
            var entry = await CreateEntryAsync(item.Title, item.Category, item.Fields, ct);
            ids.Add(entry.Id);
        }
        return ids;
    }

    public async Task DeleteEntriesBatchAsync(IReadOnlyList<string> ids, CancellationToken ct = default)
    {
        foreach (var id in ids)
        {
            await DeleteEntryAsync(id, ct);
        }
    }

    public async Task<IReadOnlyList<VaultEntry>> SearchEntriesAsync(string query, string? category = null, CancellationToken ct = default)
        => await CallAsync<List<VaultEntry>>("entry.search", new { query, category }, ct) ?? [];

    // ── Typed helpers ─────────────────────────────────────────────────────────

    public async Task<IReadOnlyList<PasswordEntry>> ListPasswordsAsync(CancellationToken ct = default)
    {
        var entries = await ListEntriesAsync("PASSWORD", ct);
        return entries.Select(PasswordEntry.FromEntry).ToList();
    }

    public async Task<string> CreatePasswordAsync(PasswordEntry p, CancellationToken ct = default)
    {
        var entry = await CreateEntryAsync(p.Title, "PASSWORD", p.ToFields(), ct);
        return entry.Id;
    }

    public async Task<IReadOnlyList<SshKeyEntry>> ListSshKeysAsync(CancellationToken ct = default)
    {
        var entries = await ListEntriesAsync("SSH_KEY", ct);
        return entries.Select(SshKeyEntry.FromEntry).ToList();
    }

    public async Task<string> CreateSshKeyAsync(SshKeyEntry k, CancellationToken ct = default)
    {
        var entry = await CreateEntryAsync(k.Title, "SSH_KEY", k.ToFields(), ct);
        return entry.Id;
    }

    public async Task<IReadOnlyList<CertificateEntry>> ListCertificatesAsync(CancellationToken ct = default)
    {
        var entries = await ListEntriesAsync("CERTIFICATE", ct);
        return entries.Select(CertificateEntry.FromEntry).ToList();
    }

    public async Task<string> CreateCertificateAsync(CertificateEntry c, CancellationToken ct = default)
    {
        var entry = await CreateEntryAsync(c.Title, "CERTIFICATE", c.ToFields(), ct);
        return entry.Id;
    }

    // ── File Vault ────────────────────────────────────────────────────────────

    public async Task<FolderListing> ListFolderAsync(string? folderId = null, CancellationToken ct = default)
        => await CallAsync<FolderListing>("file.list_folder", new { folder_id = folderId ?? "" }, ct)
           ?? new FolderListing { Folders = [], Files = [] };

    public async Task<FolderItem> CreateFolderAsync(string name, string? parentId = null, CancellationToken ct = default)
        => await CallAsync<FolderItem>("file.create_folder", new { name, parent_id = parentId ?? "" }, ct)
           ?? throw new GrimlockerException("Folder creation failed");

    public Task RenameFolderAsync(string id, string name, CancellationToken ct = default)
        => CallAsync("file.rename_folder", new { id, name }, ct);

    public Task DeleteFolderAsync(string id, CancellationToken ct = default)
        => CallAsync("file.delete_folder", new { id }, ct);

    public Task MoveFileAsync(string manifestBlockId, string folderId, CancellationToken ct = default)
        => CallAsync("file.move", new { manifest_block_id = manifestBlockId, folder_id = folderId }, ct);

    public async Task<FileEntry> UploadFileAsync(
        byte[] data, string fileName, string mimeType,
        string? folderId = null,
        IProgress<(long sent, long total)>? progress = null,
        CancellationToken ct = default)
    {
        progress?.Report((0, data.Length));
        var dataB64 = Convert.ToBase64String(data);
        var entry = await CallAsync<FileEntry>("file.ingest", new
        {
            file_name = fileName, mime_type = mimeType,
            folder_id = folderId ?? "", data_b64 = dataB64
        }, ct) ?? throw new GrimlockerException("Upload returned no entry");
        progress?.Report((data.Length, data.Length));
        return entry;
    }

    public async Task<byte[]> DownloadFileAsync(string manifestBlockId, CancellationToken ct = default)
    {
        var result = await CallAsync<DownloadResult>("file.download",
            new { manifest_block_id = manifestBlockId }, ct);
        if (result?.DataB64 is null) throw new GrimlockerException("Download returned no data");
        return Convert.FromBase64String(result.DataB64);
    }

    // ── Workspaces ────────────────────────────────────────────────────────────

    public async Task<IReadOnlyList<Workspace>> ListWorkspacesAsync(CancellationToken ct = default)
        => await CallAsync<List<Workspace>>("workspace.list", new { }, ct) ?? [];

    public async Task<Workspace> CreateWorkspaceAsync(string name, CancellationToken ct = default)
        => await CallAsync<Workspace>("workspace.create", new { name }, ct)
           ?? throw new GrimlockerException("Workspace creation failed");

    public Task SwitchWorkspaceAsync(string id, CancellationToken ct = default)
        => CallAsync("workspace.switch", new { id }, ct);

    public Task RenameWorkspaceAsync(string id, string name, CancellationToken ct = default)
        => CallAsync("workspace.rename", new { id, name }, ct);

    public Task DeleteWorkspaceAsync(string id, CancellationToken ct = default)
        => CallAsync("workspace.delete", new { id }, ct);

    // ── Sync ──────────────────────────────────────────────────────────────────

    public async Task<SyncStatus> ListSyncPeersAsync(CancellationToken ct = default)
        => await CallAsync<SyncStatus>("sync.list_peers", new { }, ct)
           ?? new SyncStatus([], 0, "");

    public Task TriggerSyncAsync(CancellationToken ct = default)
        => CallAsync("sync.trigger", new { }, ct);

    // ── Audit ─────────────────────────────────────────────────────────────────

    public async Task<IReadOnlyList<AuditEvent>> ListAuditEventsAsync(int n = 50, CancellationToken ct = default)
        => await CallAsync<List<AuditEvent>>("audit.list", new { n }, ct) ?? [];

    // ── Tools ─────────────────────────────────────────────────────────────────

    public async Task<SshKeyResult> GenerateSshKeyAsync(string comment = "", bool saveToVault = true, CancellationToken ct = default)
        => await CallAsync<SshKeyResult>("tool.ssh_keygen", new { comment, save_to_vault = saveToVault }, ct)
           ?? throw new GrimlockerException("SSH key generation failed");

    public async Task<string> GetRecoveryPhraseAsync(string password, CancellationToken ct = default)
    {
        var result = await CallAsync<RecoveryResult>("vault.recovery_phrase", new { password }, ct);
        return result?.Phrase ?? throw new GrimlockerException("Recovery phrase retrieval failed");
    }

    // ── Health ────────────────────────────────────────────────────────────────

    public async Task<VaultStatus> HealthCheckAsync(CancellationToken ct = default)
        => await VaultStatusAsync(ct);

    // ── Internal ─────────────────────────────────────────────────────────────

    private async Task CallAsync(string action, object payload, CancellationToken ct)
    {
        await ExecuteWithRetryAsync(action, payload, ct);
    }

    private async Task<T?> CallAsync<T>(string action, object payload, CancellationToken ct)
    {
        var body = await ExecuteWithRetryAsync(action, payload, ct);
        return JsonSerializer.Deserialize<T>(body, _json);
    }

    private async Task<string> ExecuteWithRetryAsync(string action, object payload, CancellationToken ct)
    {
        lock (_circuitLock)
        {
            if (DateTimeOffset.UtcNow < _circuitOpenUntil)
                throw new CircuitBreakerOpenException("circuit breaker is open");
        }

        for (int attempt = 0; attempt < 4; attempt++)
        {
            string body = "";
            int statusCode = 0;
            bool isNetworkError = false;

            try
            {
                using var resp = await _http.PostAsJsonAsync($"{_baseUrl}/api/v1",
                    new { action, payload }, _json, ct);
                statusCode = (int)resp.StatusCode;
                body = await resp.Content.ReadAsStringAsync(ct);
            }
            catch (OperationCanceledException)
            {
                throw;
            }
            catch (HttpRequestException)
            {
                isNetworkError = true;
            }

            if (!isNetworkError && statusCode >= 200 && statusCode < 300)
            {
                lock (_circuitLock) { _consecutiveFailures = 0; }
                return body;
            }

            bool isClientError = !isNetworkError && statusCode >= 400 && statusCode < 500;

            if (isClientError || attempt == 3)
            {
                lock (_circuitLock)
                {
                    _consecutiveFailures++;
                    if (_consecutiveFailures >= 5)
                        _circuitOpenUntil = DateTimeOffset.UtcNow.AddSeconds(30);
                }

                if (isNetworkError)
                    throw new GrimlockerException($"network error ({action})", -104);

                int code = 0;
                string msg = body;
                try
                {
                    using var doc = JsonDocument.Parse(body);
                    code = doc.RootElement.TryGetProperty("error_code", out var c) ? c.GetInt32() : 0;
                    msg = doc.RootElement.TryGetProperty("error", out var e) ? e.GetString() ?? body : body;
                }
                catch { /* ignore parse errors */ }
                throw new GrimlockerException($"{GrimlockerException.NameOf(code)} ({action}): {msg}", code);
            }

            int delayMs = Math.Min(2000, 100 * (1 << attempt));
            await Task.Delay(delayMs, ct);
        }

        throw new GrimlockerException("retry exhaustion", -104);
    }

    public async ValueTask DisposeAsync() => _http.Dispose();

    private record DownloadResult([property: System.Text.Json.Serialization.JsonPropertyName("data_b64")] string? DataB64);
    private record RecoveryResult([property: System.Text.Json.Serialization.JsonPropertyName("phrase")] string? Phrase);
}

public class CircuitBreakerOpenException : GrimlockerException
{
    public CircuitBreakerOpenException(string message) : base(message, -200) { }
}

public sealed record BatchEntryInput(string Title, string Category, Dictionary<string, string> Fields);

public sealed record SshKeyResult
{
    [System.Text.Json.Serialization.JsonPropertyName("public_key")]
    public string PublicKey { get; init; } = "";
    [System.Text.Json.Serialization.JsonPropertyName("fingerprint")]
    public string Fingerprint { get; init; } = "";
    [System.Text.Json.Serialization.JsonPropertyName("entry_id")]
    public string? EntryId { get; init; }
}
