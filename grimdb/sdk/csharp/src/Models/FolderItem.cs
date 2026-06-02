using System.Text.Json.Serialization;

namespace Grimlocker.Models;

public sealed record FolderItem
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = "";

    [JsonPropertyName("name")]
    public string Name { get; init; } = "";

    [JsonPropertyName("type")]
    public string Kind { get; init; } = "";
}
