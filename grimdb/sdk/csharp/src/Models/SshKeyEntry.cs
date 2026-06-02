namespace Grimlocker.Models;

public sealed record SshKeyEntry
{
    public string Title { get; init; } = "";
    public string PublicKey { get; init; } = "";
    public string PrivateKey { get; init; } = "";
    public string Username { get; init; } = "";
    public string Passphrase { get; init; } = "";
    public string Id { get; init; } = "";

    public Dictionary<string, string> ToFields() => new()
    {
        ["public_key"] = PublicKey, ["private_key"] = PrivateKey,
        ["username"] = Username, ["passphrase"] = Passphrase
    };

    public static SshKeyEntry FromEntry(VaultEntry e) => new()
    {
        Id = e.Id, Title = e.Title,
        PublicKey = e.Fields?.GetValueOrDefault("public_key") ?? "",
        PrivateKey = e.Fields?.GetValueOrDefault("private_key") ?? "",
        Username = e.Fields?.GetValueOrDefault("username") ?? "",
        Passphrase = e.Fields?.GetValueOrDefault("passphrase") ?? ""
    };
}
