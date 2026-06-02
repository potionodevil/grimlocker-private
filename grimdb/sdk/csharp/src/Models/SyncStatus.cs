using System.Text.Json.Serialization;

namespace Grimlocker.Models;

public record SyncPeer(
    [property: JsonPropertyName("device_id")] string DeviceId,
    [property: JsonPropertyName("host")]      string Host,
    [property: JsonPropertyName("port")]      int Port,
    [property: JsonPropertyName("seen_at")]   long SeenAt,
    [property: JsonPropertyName("reachable")] bool? Reachable
);

public record SyncStatus(
    [property: JsonPropertyName("peers")]        IReadOnlyList<SyncPeer> Peers,
    [property: JsonPropertyName("last_sync_at")] long LastSyncAt,
    [property: JsonPropertyName("device_id")]    string DeviceId
);

public record VaultStatus(
    [property: JsonPropertyName("initialized")] bool Initialized,
    [property: JsonPropertyName("unlocked")]    bool Unlocked,
    [property: JsonPropertyName("status")]      string Status
);
