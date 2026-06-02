using System.Text.Json.Serialization;

namespace Grimlocker.Models;

public sealed record AuditEvent
{
    [JsonPropertyName("timestamp")]
    public long Timestamp { get; init; }

    [JsonPropertyName("level")]
    public string Level { get; init; } = "";

    [JsonPropertyName("module")]
    public string Module { get; init; } = "";

    [JsonPropertyName("message")]
    public string Message { get; init; } = "";

    [JsonPropertyName("subject_id")]
    public string? SubjectId { get; init; }
}
