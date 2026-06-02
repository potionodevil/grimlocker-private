using System.Text.Json.Serialization;

namespace Grimlocker.Models;

public sealed record DownloadResult
{
    [JsonPropertyName("data_b64")]
    public string? DataB64 { get; init; }
}
