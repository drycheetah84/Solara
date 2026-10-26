// Oracle for the Ares port: runs the ORIGINAL built assemblies and prints the
// exact System.Text.Json bytes / helper results the Go tests compare against.
//   dotnet run -- models   > internal/ares/models/testdata/models.tsv
//   dotnet run -- fuzzy    < queries  (query<TAB>choice|choice|...) -> matched values
//   dotnet run -- helpers  > internal/ares/testdata/helpers.tsv
using System.Reflection;
using System.Text.Json;
using AUnrealFeatures.Ares.Models;
using AUnrealFeatures.Ares.Servers;

Console.OutputEncoding = new System.Text.UTF8Encoding(false);
Console.InputEncoding = new System.Text.UTF8Encoding(false);
var mode = args.Length > 0 ? args[0] : "models";
if (mode == "models") Models();
else if (mode == "fuzzy") Fuzzy();
else if (mode == "helpers") Helpers();
else if (mode == "specindex") SpecIndex();

static void Emit(string name, object o) => Console.WriteLine(name + "\t" + JsonSerializer.Serialize(o, o.GetType()));

static void Models()
{
    var d1 = new DateTime(2024, 1, 2, 3, 4, 5, DateTimeKind.Utc).AddTicks(1234567);
    var d2 = new DateTime(2023, 12, 31, 23, 59, 59, DateTimeKind.Unspecified);
    var cfg = new Dictionary<string, string> { ["a"] = "1", ["b"] = "x<y>&'\"" };
    var ban = new BanRequest { UserId = "u1", StationId = "s1", Expiration = d1, Reason = "r", Revoked = true };
    var role = new RoleResponse { RoleId = "r1", StationId = "s1", RoleName = "Admin", RoleDescription = "", Permissions = new() { "admin", "station:read" } };
    var udr = new UserDataResponse { UserId = "u1", Username = "Bob", DiscordId = "d", Platform = "quest", CreatedAt = d1, LastLogin = d2, Roles = new() { "r1" }, Bans = new() { ban } };
    var sdr = new ServerDeploymentResponse { DeploymentId = "dep", StationId = "st", DeploymentName = "n", Region = "NAE", IpAddress = "1.2.3.4:7777", Version = "22284", CreatedAt = d1, Online = true, LastEventAt = d2, PlayerCount = 3, Config = cfg };
    var dex = new DeploymentExpanded { IpAddress = "ip", DeploymentName = "dn", Region = "r", Version = "v", StationId = "st", CreatedAt = d1, Online = true, LastEvent = d2, PlayerCount = 7, Config = cfg };
    var fsr = new FleetStationResponse { StationId = "a", FleetId = "b", SessionId = "c", StationName = "d", Region = "e", Ip = "f", Version = "g", DeploymentCl = "h", Created = d1, Online = true, LastEvent = d2, PlayerCount = 2, Disabled = false, Config = new Dictionary<string, object> { ["flag"] = true, ["num"] = 1.5, ["s"] = "str" }, DistrictPopulations = null };
    var sev = new ServerEventResponse { Index = 1, EventType = "t", DeploymentId = "dep", EventData = "{}", Timestamp = d1 };
    var sr = new StationResponse { StationId = "st", StationName = "nm", CreatedAt = d1, Deployments = new() { sdr }, Config = cfg };
    var fr = new FleetResponse { FleetId = "f", FleetName = "fn", Created = d1, Online = true, Stations = new() { fsr }, Config = new Dictionary<string, object> { ["x"] = 2.0 } };

    var all = new (string, object, object)[]
    {
        ("ApiKeyResponse", new ApiKeyResponse { ApiKey = "k" }, new ApiKeyResponse()),
        ("AssignPermsBody", new AssignPermsBody { Permissions = new() { "a", "b" } }, new AssignPermsBody()),
        ("BanRequest", ban, new BanRequest()),
        ("BanResponse", new BanResponse { Bans = new() { ban } }, new BanResponse()),
        ("ClientLoginDeployment", new ClientLoginDeployment { DeploymentId = "d", StationId = "s", StationName = "sn", DeploymentName = "dn", IpAddress = "ip", IsWhitelist = true, IsPublic = true, IsJoinable = true }, new ClientLoginDeployment()),
        ("ClientLoginRequest", new ClientLoginRequest { Nonce = "n", PlatformId = "p", Username = "u", Platform = "q", Device = "d" }, new ClientLoginRequest()),
        ("ClientLoginResponse", new ClientLoginResponse { Success = true, ApiKey = "k", OrgScopedId = "o", ServerDeployments = new() { new ClientLoginDeployment() } }, new ClientLoginResponse()),
        ("ClientLoginV2Response", new ClientLoginV2Response { Success = true, ApiKey = "k", OrgScopedId = "o" }, new ClientLoginV2Response()),
        ("CreateDeploymentResponse", new CreateDeploymentResponse { ApiKey = "k", DeploymentId = "d" }, new CreateDeploymentResponse()),
        ("CreateRoleRequest", new CreateRoleRequest { RoleName = "n", RoleDescription = "d" }, new CreateRoleRequest()),
        ("CreateStationRequest", new CreateStationRequest { StationName = "n", StationId = "i" }, new CreateStationRequest()),
        ("CreateStationResponse", new CreateStationResponse { StationId = "i" }, new CreateStationResponse()),
        ("DeploymentDbObject", new DeploymentDbObject { DeploymentId = "d", StationId = "s", DeploymentName = "n", IpAddress = "ip", Region = "r", Version = "v", CreatedAt = d1, Online = true, LastEvent = d2, PlayerCount = 4, Config = cfg, ApiKeys = new() { "k" } }, new DeploymentDbObject()),
        ("DeploymentRequest", new DeploymentRequest { IpAddress = "ip", DeploymentName = "dn", Region = "r", Version = "v", StationId = "s" }, new DeploymentRequest()),
        ("DeploymentExpanded", dex, new DeploymentExpanded()),
        ("DeploymentsResponsePage", Page<DeploymentExpanded, DeploymentsResponsePage>(new() { dex }, p => p.StationId = "st"), new DeploymentsResponsePage()),
        ("EventSignupDbObject", new EventSignupDbObject { SignupId = "s", EventId = "e", UserId = "u", Timestamp = d1, Data = "x" }, new EventSignupDbObject()),
        ("EventSignupResponse", new EventSignupResponse { UserSignedUp = true, SignupId = "s" }, new EventSignupResponse()),
        ("FleetResponse", fr, new FleetResponse()),
        ("FleetResponsePage", Page<FleetResponse, FleetResponsePage>(new() { fr }, null), new FleetResponsePage()),
        ("FleetStationResponse", fsr, new FleetStationResponse()),
        ("ValidationError", new ValidationError { Locations = new() { "body" }, Message = "m", Type = "t" }, new ValidationError()),
        ("HTTPValidationError", new HTTPValidationError { Details = new() { new ValidationError() } }, new HTTPValidationError()),
        ("LogInServerRequest", new LogInServerRequest { UserId = "u", Username = "n", Platform = "p", Device = "d" }, new LogInServerRequest()),
        ("UserPermission", new UserPermission { StationId = "s", Permission = "p" }, new UserPermission()),
        ("LogInServerResponse", new LogInServerResponse { ApiKey = "k", User = udr, Permissions = new() { new UserPermission { StationId = "global", Permission = "admin" } } }, new LogInServerResponse()),
        ("PageData", new PageData { TotalItems = 1, ItemCount = 2, PageSize = 3, Page = 4, Pages = 5 }, new PageData()),
        ("RegionResponse", new RegionResponse { ContinentCode = "EU", CountryCode = "DE" }, new RegionResponse()),
        ("RegisterServerRequest", new RegisterServerRequest { Ip = "1", Port = "2", DeploymentId = "3", ServerName = "4", Region = "5", ImguiPort = "6", MaxPlayers = 7 }, new RegisterServerRequest()),
        ("RegisterServerResponse", new RegisterServerResponse { Success = true, StationId = "s", DeploymentId = "d" }, new RegisterServerResponse()),
        ("UpdatePlayerCountRequest", new UpdatePlayerCountRequest { DeploymentId = "d", PlayerCount = 3 }, new UpdatePlayerCountRequest()),
        ("ServerHeartbeatRequest", new ServerHeartbeatRequest { DeploymentId = "d", Pid = 1, Seq = 18446744073709551615UL, DispatchAgeMs = 2, TickAgeMs = 3, UptimeMs = 4, Players = 5 }, new ServerHeartbeatRequest()),
        ("RoleData", new RoleData { RoleId = "r", StationId = "s", RoleName = "n", RoleDescription = "d" }, new RoleData()),
        ("RoleResponse", role, new RoleResponse()),
        ("RolesResponse", new RolesResponse { Roles = new() { role } }, new RolesResponse()),
        ("ServerDeploymentResponse", sdr, new ServerDeploymentResponse()),
        ("ServerDeploymentResponsePage", Page<ServerDeploymentResponse, ServerDeploymentResponsePage>(new() { sdr }, null), new ServerDeploymentResponsePage()),
        ("ServerEventDbObject", new ServerEventDbObject { DeploymentId = "d", EventType = "t", EventData = "x", Timestamp = d2 }, new ServerEventDbObject()),
        ("ServerEventRequest", new ServerEventRequest { EventType = "t", EventData = "x" }, new ServerEventRequest()),
        ("ServerEventResponse", sev, new ServerEventResponse()),
        ("ServerEventResponsePage", Page<ServerEventResponse, ServerEventResponsePage>(new() { sev, sev }, null), new ServerEventResponsePage()),
        ("StationDbObject", new StationDbObject { StationId = "s", StationName = "n", CreatedAt = d1, Online = true, LastOnline = d2, Config = cfg }, new StationDbObject()),
        ("StationDeploymentResponse", new StationDeploymentResponse { StationId = "s", Deployments = new() { dex } }, new StationDeploymentResponse()),
        ("StationEventDbObject", new StationEventDbObject { EventId = "e", StationId = "s", DeploymentId = "d", Title = "t", Description = "de", StartTime = d1, Duration = 60, Public = true, SignupsOpen = true, Config = cfg }, new StationEventDbObject()),
        ("StationEventSignupInfo", new StationEventSignupInfo { SignupId = "s", UserId = "u", Timestamp = d1, Data = "x" }, new StationEventSignupInfo()),
        ("StationEventSignups", new StationEventSignups { Signups = new() { new StationEventSignupInfo() } }, new StationEventSignups()),
        ("StationEvent", new StationEvent { EventId = "e", Title = "t", Description = "d", StartTime = d1, Duration = 5, Public = true, SignupsOpen = true, StationId = "s", DeploymentId = "" }, new StationEvent()),
        ("StationEvents", new StationEvents { Events = new() { new StationEvent() } }, new StationEvents()),
        ("StationPerm", new StationPerm { StationId = "s", Permission = "p" }, new StationPerm()),
        ("StationResponse", sr, new StationResponse()),
        ("StationResponsePage", Page<StationResponse, StationResponsePage>(new() { sr }, null), new StationResponsePage()),
        ("StationsPageV2", new StationsPageV2 { FleetId = "", Items = new() { fsr } }, new StationsPageV2()),
        ("SuccessBoolean", new SuccessBoolean { Success = true }, new SuccessBoolean()),
        ("UpdateStationRequest", new UpdateStationRequest { StationName = "n" }, new UpdateStationRequest()),
        ("UpdateStationResponse", new UpdateStationResponse { StationName = "n" }, new UpdateStationResponse()),
        ("UserApiKeyDbObject", new UserApiKeyDbObject { UserId = "u", ApiKey = "k" }, new UserApiKeyDbObject()),
        ("UserData", new UserData { UserId = "u", Username = "n", DiscordId = "d", Platform = "p", CreatedAt = d1, LastLogin = d2 }, new UserData()),
        ("UserDataResponse", udr, new UserDataResponse()),
        ("UserDataV1Response", UserDataV1Response.FromStorage(udr, new() { role }, true), new UserDataV1Response()),
        ("UserDataV1ResponseNoBans", UserDataV1Response.FromStorage(udr, null, false), new UserDataV1Response()),
        ("UserRequest", new UserRequest { UserId = "u", Username = "n", DiscordId = "d", Platform = "p" }, new UserRequest()),
        ("UserRoleByNameRequest", new UserRoleByNameRequest { RoleId = "r", Username = "u", ExpiresInHours = 3 }, new UserRoleByNameRequest()),
        ("UserRoleByNameResponse", new UserRoleByNameResponse { Success = true, UserExists = true }, new UserRoleByNameResponse()),
        ("UsersResponse", new UsersResponse { Users = new() { udr } }, new UsersResponse()),
        ("UsersResponsePage", Page<UserDataResponse, UsersResponsePage>(new() { udr }, null), new UsersResponsePage()),
        ("DrafterGrant", new DrafterGrant { UserId = "u", Scope = "global", GrantedAt = d1 }, new DrafterGrant { GrantedAt = d2 }),
        ("SpecEntry", new SpecLevelsStore.Entry { Autoload = true, Loaded = true, Updated = "x", Size = 3, ServerLoaded = new() { "d" }, ServerSeen = "y" }, new SpecLevelsStore.Entry()),
    };
    foreach (var (n, full, empty) in all) { Emit(n, full); Emit(n + ".empty", empty); }

    // PagedResponse.Create paging arithmetic
    var items = Enumerable.Range(1, 7).Select(i => new ServerEventResponse { Index = i, Timestamp = d2 }).ToList();
    foreach (var (ps, pg) in new[] { (3, 1), (3, 3), (3, 9), (100, 1), (7, 2), (1, 7) })
        Emit($"Create.{ps}.{pg}", PagedResponse<ServerEventResponse>.Create<ServerEventResponsePage>(items, ps, pg));
    Emit("Create.empty", PagedResponse<ServerEventResponse>.Create<ServerEventResponsePage>(new(), 10, 5));
}

static TPage Page<T, TPage>(List<T> items, Action<TPage> fill) where TPage : PagedResponse<T>
{
    var p = (TPage)PagedResponse<T>.Create<TPage>(items, 10, 1);
    fill?.Invoke(p);
    return p;
}

static void Fuzzy()
{
    string line;
    while ((line = Console.ReadLine()) != null)
    {
        var parts = line.Split('\t');
        var choices = parts[1].Split('|');
        var r = FuzzySharp.Process.ExtractTop(parts[0], choices).Select(x => $"{x.Value}:{x.Score}");
        Console.WriteLine(string.Join("|", r));
    }
}

static void Helpers()
{
    var t = typeof(A2StationDbServer);
    var bf = BindingFlags.NonPublic | BindingFlags.Public | BindingFlags.Static;
    var toTyped = t.GetMethod("ToTypedConfig", bf)!;
    foreach (var v in new[] { "true", "False", "1.5", "  2 ", "1e3", "0x10", "$1,000.50", "(5)", "abc", "", "NaN", "-Infinity", "1,234", "TRUE ", " true", "∞", "1_000", ",1", "1,,2", "1.", ".5", "-.5e-2", "1e", "+-1", "5-", "(5", "¤5", "Infinity", "infinity", "-nan", " NaN ", "1e400", "1 000", "1\x0000", "+5", "5+", "--5", "1e+", "1.5.2", "12,345.6", "1,2.3,4", "(5)-", "-(5)", "  ", "\t7\n", "1E2", "0.1", "123456789012345678901234567890", "4.9e-324", "2e-400", "-0", "true\x0000", "\x0000true", "False\t" })
    {
        var res = (Dictionary<string, object>)toTyped.Invoke(null, new object[] { new Dictionary<string, string> { ["k"] = v } });
        string js;
        try { js = JsonSerializer.Serialize(res); }
        catch (Exception e) { js = "EXC " + res["k"].GetType().Name + " " + res["k"] + " " + e.GetType().Name; }
        Console.WriteLine("typed\t" + v + "\t" + js);
    }
    var pbb = t.GetMethod("PublicBoardBase", bf)!;
    foreach (var h in new[] { "", "localhost", "127.0.0.1", "10.1.1.1", "192.168.0.2", "172.16.4.4", "172.17.0.1", "169.254.1.1", "example.com", "Example.COM", "::1", "LOCALHOST" })
        Console.WriteLine("board\t" + h + "\t" + pbb.Invoke(null, new object[] { h }));
    var san = t.GetMethod("SanitiseForFileName", bf)!;
    foreach (var s in new[] { "abc-DEF_12", "a b/c", "été", "日本", "x.y" })
        Console.WriteLine("sanitise\t" + s + "\t" + san.Invoke(null, new object[] { s }));
    var st = new StationDbObject { StationId = "s", Config = new() { ["nv.module.3.Speed"] = "\"12\"", ["nv.world.config/player/brakeStrength"] = " 0.5\t", ["nv.module.bad"] = "x", ["nv.module.*.Mode"] = "DRIFT", ["other"] = "y", ["nv.zzz"] = "q", ["nv.module.x."] = "z" } };
    Console.WriteLine("buildtext\t" + JsonSerializer.Serialize(NetvarOverridesFile.BuildText(st)));
    Console.WriteLine("buildtext.null\t" + JsonSerializer.Serialize(NetvarOverridesFile.BuildText(null)));
    foreach (var raw in new[] { "My%20Level", " ok_name-1 ", "bad/name", "%ZZ", "a%2Fb", new string('a', 64), new string('a', 65), "" })
        Console.WriteLine("cleanname\t" + raw + "\t" + (SpecLevelsStore.CleanName(raw) ?? "<null>"));
    foreach (var raw in new[] { "a, b;c\n d,,", "", " , " })
        Console.WriteLine("aclparse\t" + JsonSerializer.Serialize(raw) + "\t" + JsonSerializer.Serialize(StationAcl.Parse(raw).ToList()));
    foreach (var id in new[] { "abc", "a.b", "", new string('x', 129), "A-z_0" })
        Console.WriteLine("validid\t" + id + "\t" + DrafterPermissions.ValidId(id));
}

static void SpecIndex()
{
    var idx = new Dictionary<string, SpecLevelsStore.Entry>(StringComparer.Ordinal)
    {
        ["Zeta level"] = new SpecLevelsStore.Entry { Autoload = true, Loaded = false, Updated = "2024-01-02T03:04:05.1234567Z", Size = 12, ServerLoaded = new() { "dep-1", "dep<2>" }, ServerSeen = "" },
        ["alpha"] = new SpecLevelsStore.Entry(),
    };
    var opts = new JsonSerializerOptions { WriteIndented = true };
    Console.WriteLine(JsonSerializer.Serialize(JsonSerializer.Serialize(idx, opts)));
    Console.WriteLine(JsonSerializer.Serialize(JsonSerializer.Serialize(new Dictionary<string, SpecLevelsStore.Entry>(), opts)));
}
