// Oracle for solara/internal/eos: prints real System.Text.Json / helper outputs
// from the original AUnrealFeatures.EOSSDK assembly (net6.0).
using System.Reflection;
using System.Text.Json;
using AUnrealFeatures.EOSSDK;
using AUnrealFeatures.EOSSDK.Models;

static class P
{
    static void Out(string name, string value) => Console.WriteLine(name + "\t" + value.Replace("\r", "\\r").Replace("\n", "\\n"));

    static void Main(string[] args)
    {
        var asm = typeof(EosGatewayServer).Assembly;

        // ── Models ─────────────────────────────────────────────────────────────
        Out("EosLobbyFilterRequest.default", JsonSerializer.Serialize(new EosLobbyFilterRequest()));
        Out("EosLobbyResponse.default", JsonSerializer.Serialize(new EosLobbyResponse()));
        Out("EosLobbySession.default", JsonSerializer.Serialize(new EosLobbySession()));
        Out("EosLobbySettings.default", JsonSerializer.Serialize(new EosLobbySettings()));
        Out("EosLobbyPlatforms.default", JsonSerializer.Serialize(new EosLobbyPlatforms()));
        Out("EosLobbyMemberEntry.default", JsonSerializer.Serialize(new EosLobbyMemberEntry()));
        Out("EosLobbyCreateRequest.default", JsonSerializer.Serialize(new EosLobbyCreateRequest()));
        Out("EosLobbyUpdateRequest.default", JsonSerializer.Serialize(new EosLobbyUpdateRequest()));
        Out("EosFilterCriterion.populated", JsonSerializer.Serialize(new EosFilterCriterion { Key = "bucket", Op = "EQUAL", Value = JsonDocument.Parse("\"A2_4729\"").RootElement }));
        Out("EosFilterRequest.default", JsonSerializer.Serialize(new EosFilterRequest()));
        Out("EosMatchmakingResponse.default", JsonSerializer.Serialize(new EosMatchmakingResponse()));
        Out("EosMatchmakingSession.default", JsonSerializer.Serialize(new EosMatchmakingSession()));
        Out("EosSessionAttributes.default", JsonSerializer.Serialize(new EosSessionAttributes()));
        Out("EosSessionSettings.default", JsonSerializer.Serialize(new EosSessionSettings()));
        Out("EosOAuthTokenResponse.default", JsonSerializer.Serialize(new EosOAuthTokenResponse()));
        Out("EosTelemetryRequest.default", JsonSerializer.Serialize(new EosTelemetryRequest()));
        Out("EosTelemetryEvent.default", JsonSerializer.Serialize(new EosTelemetryEvent()));
        Out("EosTurnCredentialsResponse.default", JsonSerializer.Serialize(new EosTurnCredentialsResponse()));
        var si = new EosSessionInfo { Id = "fixed" };
        Out("EosSessionInfo.default", JsonSerializer.Serialize(si));

        var lobby = new EosLobbySession
        {
            Deployment = "dep", Id = "lid", Bucket = "b<&>", TotalPlayers = 2, OpenPublicPlayers = 8,
            PublicPlayers = new() { "p1", "p2" }, Started = true, LastUpdated = "2025-01-02T03:04:05.678Z",
            Attributes = new() { ["z"] = "1", ["a"] = null, ["ADDRESS_s"] = "1.2.3.4" }, Owner = "p1", OwnerPlatformId = 4000,
            Settings = new EosLobbySettings { Platforms = new EosLobbyPlatforms { AllowedPlatformIds = new[] { 1, 2 } } },
            Lock = "lk",
        };
        Out("EosLobbySession.populated", JsonSerializer.Serialize(lobby));
        Out("EosLobbyResponse.populated", JsonSerializer.Serialize(new EosLobbyResponse { Count = 1, Sessions = new() { lobby } }));
        var me = new EosLobbyMemberEntry { PlatformId = 1, Platform = "steam" };
        me.Data["b"] = true; me.Data["n"] = 5L; me.Data["d"] = 1.5; me.Data["s"] = "x"; me.Data["nul"] = null; me.Data["big"] = 0.1 + 0.2;
        Out("EosLobbyMemberEntry.populated", JsonSerializer.Serialize(me));
        Out("EosOAuthTokenResponse.populated", JsonSerializer.Serialize(new EosOAuthTokenResponse
        {
            AccessToken = "at", ExpiresAt = "2035-01-01T00:00:00.000Z", ExpiresIn = 315360000, Nonce = "n",
            Features = new[] { "A", "B" }, OrganizationId = "o", ProductId = "p", SandboxId = "s", DeploymentId = "d",
            OrganizationUserId = "ou", ProductUserId = "pu", ProductUserIdCreated = false, IdToken = "it", ClientId = "c",
        }));
        Out("EosTurnCredentialsResponse.populated", JsonSerializer.Serialize(new EosTurnCredentialsResponse { Username = "u", Password = "p+/=", Ttl = 3600000, Uris = new[] { "stun:x" } }));
        var full = new EosSessionInfo { Id = "id1", ServerName = "S\u00e9", IpAddress = "1.1.1.1", Port = "7777", MaxPlayers = 12, Bucket = "B", BuildId = 1, ImguiPort = "9", StationId = "st", DeploymentId = "dp", PublicPlayers = new() { "a" } };
        Out("EosSessionInfo.populated", JsonSerializer.Serialize(full));
        Out("Sessions.indented", JsonSerializer.Serialize(new List<EosSessionInfo> { full, si }, new JsonSerializerOptions { WriteIndented = true }));
        Out("Lobbies.indented", JsonSerializer.Serialize(new List<EosLobbySession> { lobby, new EosLobbySession() }, new JsonSerializerOptions { WriteIndented = true }));
        Out("EmptyList.indented", JsonSerializer.Serialize(new List<EosSessionInfo>(), new JsonSerializerOptions { WriteIndented = true }));

        // Deserialize defaults
        Out("EosLobbyCreateRequest.deser", JsonSerializer.Serialize(JsonSerializer.Deserialize<EosLobbyCreateRequest>("{\"settings\":{\"maxPublicPlayers\":4,\"platforms\":{}},\"attributes\":{\"b\":\"1\",\"a\":null}}")));
        Out("EosSessionInfo.deser", JsonSerializer.Serialize(JsonSerializer.Deserialize<EosSessionInfo>("{\"id\":\"x\",\"public_players\":[\"q\"]}")));

        // Telemetry event round trip (JsonElement values)
        var tr = JsonSerializer.Deserialize<EosTelemetryRequest>("{\"Events\":[{\"EventName\":\"e1\",\"EventData\":{ \"k\" : [1, 2.50, {\"x\" : \"\\u00e9\\/\"}], \"a\":null, \"t\": true }}]}")!;
        Out("EosTelemetryEvent.roundtrip", JsonSerializer.Serialize(tr.Events[0]));

        // JsonElement root serialization (DefaultEndpoints)
        string Res(string f) { var n = asm.GetManifestResourceNames().First(x => x.EndsWith(f)); using var s = asm.GetManifestResourceStream(n)!; using var r = new StreamReader(s); return r.ReadToEnd(); }
        Out("default_endpoints", JsonSerializer.Serialize(JsonDocument.Parse(Res("default_endpoints.json")).RootElement));
        Out("custom_endpoints", JsonSerializer.Serialize(JsonDocument.Parse(Res("custom_endpoints.json")).RootElement));

        // Observability records
        var te = new TelemetryEvent("abc", "ev", "{\"x\":1}", new DateTime(2025, 1, 2, 3, 4, 5, DateTimeKind.Utc).AddTicks(1234567));
        Out("TelemetryEvent", JsonSerializer.Serialize(te));
        Out("WsMessage", JsonSerializer.Serialize(new WsMessage("in", "CONNECT", "a:b", "body", new DateTime(2025, 1, 2, 3, 4, 5, DateTimeKind.Utc))));
        var cs = new WsClientSession { SessionId = "sid", Path = "/p", ConnectedAt = new DateTime(2025, 1, 2, 3, 4, 5, DateTimeKind.Utc) };
        cs.AddMessage(new WsMessage("in", "c", "h", "b", DateTime.UtcNow));
        Out("WsClientSession", JsonSerializer.Serialize(cs));

        // JWT helpers
        var jwt = asm.GetType("AUnrealFeatures.EOSSDK.EosJwtFactory")!;
        Out("ProductUserIdFor.alice", (string)jwt.GetMethod("ProductUserIdFor")!.Invoke(null, new object[] { "alice" })!);
        Out("OrgUserIdFor.alice", (string)jwt.GetMethod("OrgUserIdFor")!.Invoke(null, new object[] { "alice" })!);
        Out("ProductUserIdFor.unicode", (string)jwt.GetMethod("ProductUserIdFor")!.Invoke(null, new object[] { "J\u00f6rg" })!);
        Out("ExtractClientId", (string)jwt.GetMethod("ExtractClientId")!.Invoke(null, new object?[] { "Basic " + Convert.ToBase64String(System.Text.Encoding.UTF8.GetBytes("cid:secret")), "fb" })!);
        var gw = typeof(EosGatewayServer);
        var steam = gw.GetMethod("SteamIdFromTicket", BindingFlags.NonPublic | BindingFlags.Static)!;
        Out("SteamIdFromTicket.abc", (string)steam.Invoke(null, new object?[] { "abcdef0123" })!);
        Out("SteamIdFromTicket.null", (string)steam.Invoke(null, new object?[] { null })!);
        Out("SteamIdFromTicket.zz", (string)steam.Invoke(null, new object?[] { "zz" })!);
        var ex = gw.GetMethod("ExtractPuidFromBearer", BindingFlags.NonPublic | BindingFlags.Static)!;
        string B64(string s) => Convert.ToBase64String(System.Text.Encoding.UTF8.GetBytes(s)).TrimEnd('=').Replace('+', '-').Replace('/', '_');
        Out("ExtractPuid.ok", (string)ex.Invoke(null, new object?[] { "Bearer h." + B64("{\"productUserId\":\"0002abc\"}") + ".s" })!);
        Out("ExtractPuid.missing", (string)ex.Invoke(null, new object?[] { "Bearer h." + B64("{\"x\":1}") + ".s" })!);
        Out("ExtractPuid.null", (string)ex.Invoke(null, new object?[] { "Bearer h." + B64("{\"productUserId\":null}") + ".s" })!);
        Out("ExtractPuid.bad", (string)ex.Invoke(null, new object?[] { "Bearer h.!!!.s" })!);
        Out("ExtractPuid.basic", (string)ex.Invoke(null, new object?[] { "Basic x" })!);
        Environment.Exit(0);
    }
}
