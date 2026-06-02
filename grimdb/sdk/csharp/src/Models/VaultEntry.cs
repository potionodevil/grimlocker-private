using System.Text.Json.Serialization;

namespace Grimlocker.Models;

public record VaultEntry(
    [property: JsonPropertyName("id")]         string Id,
    [property: JsonPropertyName("title")]      string Title,
    [property: JsonPropertyName("category")]   string Category,
    [property: JsonPropertyName("created_at")] long CreatedAt,
    [property: JsonPropertyName("updated_at")] long UpdatedAt,
    [property: JsonPropertyName("fields")]     Dictionary<string, string>? Fields
);
