using System.Text.Json.Serialization;

namespace Grimlocker.Models;

public sealed record FileEntry
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = "";

    [JsonPropertyName("file_name")]
    public string FileName { get; init; } = "";

    [JsonPropertyName("mime_type")]
    public string MimeType { get; init; } = "";

    [JsonPropertyName("total_size")]
    public long TotalSize { get; init; }

    [JsonPropertyName("manifest_block_id")]
    public string ManifestBlockId { get; init; } = "";

    [JsonPropertyName("folder_id")]
    public string FolderId { get; init; } = "";
}
