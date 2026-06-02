using System.Text.Json.Serialization;

namespace Grimlocker.Models;

public sealed record UploadProgress
{
    [JsonPropertyName("bytes_sent")]
    public long BytesSent { get; init; }

    [JsonPropertyName("total_bytes")]
    public long TotalBytes { get; init; }
}
