using System.Text.Json.Serialization;

namespace Grimlocker.Models;

public sealed record FolderListing
{
    [JsonPropertyName("folders")]
    public IReadOnlyList<FolderItem> Folders { get; init; } = [];

    [JsonPropertyName("files")]
    public IReadOnlyList<FileEntry> Files { get; init; } = [];
}
