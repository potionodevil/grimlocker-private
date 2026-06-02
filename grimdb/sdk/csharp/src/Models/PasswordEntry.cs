namespace Grimlocker.Models;

public sealed record PasswordEntry
{
    public string Title { get; init; } = "";
    public string Username { get; init; } = "";
    public string Password { get; init; } = "";
    public string Url { get; init; } = "";
    public string Notes { get; init; } = "";
    public string Id { get; init; } = "";

    public Dictionary<string, string> ToFields() => new()
    {
        ["username"] = Username, ["password"] = Password,
        ["url"] = Url, ["notes"] = Notes
    };

    public static PasswordEntry FromEntry(VaultEntry e) => new()
    {
        Id = e.Id, Title = e.Title,
        Username = e.Fields?.GetValueOrDefault("username") ?? "",
        Password = e.Fields?.GetValueOrDefault("password") ?? "",
        Url = e.Fields?.GetValueOrDefault("url") ?? "",
        Notes = e.Fields?.GetValueOrDefault("notes") ?? ""
    };
}
