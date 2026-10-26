// DB round-trip oracle for internal/ares: inserts every persisted
// AUnrealFeatures.Ares model (and Servers.DrafterGrant) into a real LiteDB
// 5.0.21 database (BsonMapper defaults: TrimWhitespace, EmptyStringToNull),
// reads it back with FindAll().First() and prints
// "<case>\t<JsonSerializer.Serialize(readBack)>" lines.
// Usage: dotnet run -- > ../../internal/ares/testdata/dbroundtrip.tsv
// The Go side (internal/ares/dbroundtrip_test.go) builds the same instances.
// Dictionary keys are inserted in ordinal order (Go maps serialize sorted).
using System.Text.Json;
using LiteDB;
using AUnrealFeatures.Ares.Models;
using AUnrealFeatures.Ares.Servers;
using JsonSerializer = System.Text.Json.JsonSerializer;

static class P
{
    static readonly DateTime D = new DateTime(2025, 1, 2, 3, 4, 5, DateTimeKind.Utc).AddTicks(1234567);
    static readonly string Special = "v<&'" + (char)0xE9 + ">";

    static Dictionary<string, string> Cfg() => new()
    {
        ["a"] = " 2 ", ["m"] = "m", ["n"] = null, ["s"] = Special, ["w"] = "  ", ["z"] = "",
    };

    static List<string> Lst() => new() { "", "  x  ", "y", "   ", null, Special };

    static LiteDatabase db;

    static void Rt<T>(string name, T o)
    {
        var col = db.GetCollection<T>("c" + Guid.NewGuid().ToString("N"));
        col.Insert(o);
        var back = col.FindAll().First();
        Console.WriteLine(name + "\t" + JsonSerializer.Serialize(back));
    }

    static void Main()
    {
        Console.OutputEncoding = new System.Text.UTF8Encoding(false);
        var path = Path.Combine(Path.GetTempPath(), "oracle-aresdb-" + Guid.NewGuid().ToString("N") + ".db");
        using (db = new LiteDatabase(new ConnectionString(path) { Connection = ConnectionType.Direct }))
        {
            Rt("default.DeploymentDbObject", new DeploymentDbObject());
            Rt("mixed.DeploymentDbObject", new DeploymentDbObject
            {
                DeploymentId = "", StationId = "  st  ", DeploymentName = null, IpAddress = "1.2.3.4:7777", Region = "   ",
                Version = Special, CreatedAt = D, Online = true, LastEvent = D, PlayerCount = 7, Config = Cfg(),
                ApiKeys = new() { "k1" },
            });
            Rt("default.ServerEventDbObject", new ServerEventDbObject());
            Rt("mixed.ServerEventDbObject", new ServerEventDbObject { DeploymentId = "  d  ", EventType = "", EventData = "{\"x\":1}", Timestamp = D });
            Rt("default.StationEventDbObject", new StationEventDbObject());
            Rt("mixed.StationEventDbObject", new StationEventDbObject
            {
                EventId = "e", StationId = "", DeploymentId = "  ", Title = " T ", Description = "", StartTime = D, Duration = 7,
                Public = true, SignupsOpen = true, Config = Cfg(),
            });
            Rt("default.EventSignupDbObject", new EventSignupDbObject());
            Rt("mixed.EventSignupDbObject", new EventSignupDbObject { SignupId = "", EventId = " e ", UserId = "u", Timestamp = D, Data = "" });
            Rt("default.RoleResponse", new RoleResponse());
            Rt("mixed.RoleResponse", new RoleResponse { RoleId = "r", StationId = " s ", RoleName = "Admin", RoleDescription = "", Permissions = Lst() });
            Rt("default.StationDbObject", new StationDbObject());
            Rt("mixed.StationDbObject", new StationDbObject { StationId = "s", StationName = "", CreatedAt = D, Online = true, LastOnline = D, Config = Cfg() });
            Rt("default.UserDataResponse", new UserDataResponse());
            Rt("mixed.UserDataResponse", new UserDataResponse
            {
                UserId = " u ", Username = "", DiscordId = null, Platform = "quest", CreatedAt = D, LastLogin = D, Roles = Lst(),
                Bans = new() { new BanRequest { UserId = "", StationId = " s ", Expiration = D, Reason = "", Revoked = true }, new BanRequest() },
            });
            Rt("default.UserApiKeyDbObject", new UserApiKeyDbObject());
            Rt("mixed.UserApiKeyDbObject", new UserApiKeyDbObject { UserId = "", ApiKey = " k " });
            // DrafterGrant only ever stores ValidId values (see drafter_permissions.go).
            Rt("valid.DrafterGrant", new DrafterGrant { UserId = "user_1", Scope = "global", GrantedAt = D });
        }
        File.Delete(path);
    }
}
