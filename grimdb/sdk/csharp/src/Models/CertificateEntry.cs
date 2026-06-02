namespace Grimlocker.Models;

public sealed record CertificateEntry
{
    public string Title { get; init; } = "";
    public string Domain { get; init; } = "";
    public string Certificate { get; init; } = "";
    public string PrivateKey { get; init; } = "";
    public string Id { get; init; } = "";

    public Dictionary<string, string> ToFields() => new()
    {
        ["domain"] = Domain, ["certificate"] = Certificate, ["private_key"] = PrivateKey
    };

    public static CertificateEntry FromEntry(VaultEntry e) => new()
    {
        Id = e.Id, Title = e.Title,
        Domain = e.Fields?.GetValueOrDefault("domain") ?? "",
        Certificate = e.Fields?.GetValueOrDefault("certificate") ?? "",
        PrivateKey = e.Fields?.GetValueOrDefault("private_key") ?? ""
    };
}
