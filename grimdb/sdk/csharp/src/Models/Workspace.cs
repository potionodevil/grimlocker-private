using System.Text.Json.Serialization;

namespace Grimlocker.Models;

public sealed record Workspace
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = "";

    [JsonPropertyName("name")]
    public string Name { get; init; } = "";

    [JsonPropertyName("is_default")]
    public bool IsDefault { get; init; }
}
