// Oracle for internal/mothership: captures real .NET 6 / System.Text.Json
// outputs of the original AUnrealFeatures.AAMothership assembly and writes them
// as fixtures (internal/mothership/testdata/oracle.json).
// Usage: dotnet run -- <fixtures.json> <full-token-scratch-file>
// The JWT is signed with the original's baked secret, so only header.body goes
// into the fixture; the complete token goes to the scratch file (outside the repo).
using System.Reflection;
using System.Text.Json;
using AUnrealFeatures.AAMothership;
using AUnrealFeatures.AAMothership.Models;

static class P
{
    static readonly BindingFlags S = BindingFlags.NonPublic | BindingFlags.Static;
    static object Call(string name, params object[] args) => typeof(MothershipServer).GetMethod(name, S)!.Invoke(null, args);

    static void Populate(object o)
    {
        string special = "v<&'" + (char)0xE9 + "+>";
        foreach (var p in o.GetType().GetProperties())
        {
            if (!p.CanWrite || p.Name == "Id") continue;
            var t = p.PropertyType;
            if (t == typeof(string)) p.SetValue(o, p.Name + "_" + special);
            else if (t == typeof(int)) p.SetValue(o, 7);
            else if (t == typeof(long)) p.SetValue(o, 9007199254740993L);
            else if (t == typeof(bool)) p.SetValue(o, !(bool)p.GetValue(o)!);
            else if (t == typeof(DateTime)) p.SetValue(o, new DateTime(2025, 1, 2, 3, 4, 5, DateTimeKind.Utc).AddTicks(1234567));
            else if (t == typeof(List<string>)) p.SetValue(o, new List<string> { "a", "b" });
            else if (t == typeof(Dictionary<string, string>)) p.SetValue(o, new Dictionary<string, string> { { "z", "1" }, { "a", "2" } });
            else throw new Exception("unhandled " + t);
        }
    }

    static void Main(string[] a)
    {
        var outp = new SortedDictionary<string, string>();

        // 1. Models: default-constructed and populated.
        foreach (var t in typeof(MothershipPlayerDbObject).Assembly.GetTypes()
                     .Where(t => t.Namespace == "AUnrealFeatures.AAMothership.Models" && t.IsClass && !t.IsAbstract).OrderBy(t => t.Name))
        {
            var d = Activator.CreateInstance(t)!;
            outp["model.default." + t.Name] = JsonSerializer.Serialize(d, t);
            var f = Activator.CreateInstance(t)!;
            Populate(f);
            outp["model.full." + t.Name] = JsonSerializer.Serialize(f, t);
        }

        // 2. JWT format (header.body only).
        var token = (string)Call("GenerateJwt", new
        {
            sub = "player-1", did = "dev-1", env = "7f3a99dd-5598-4725-98cf-6538d28feb9f",
            externalService = (string)null, externalServiceId = "ext<1>", tid = "f3e9fb19",
            tags = (string)null, orgScopedExternalServiceId = "ext<1>", nbf = 1700000000L, exp = 1700003600L, iat = 1700000000L
        });
        var parts = token.Split('.');
        outp["jwt.headerbody"] = parts[0] + "." + parts[1];
        File.WriteAllText(a[1], token);

        // 3. StableId, Base64Url, quest blob codec.
        outp["stableid"] = (string)Call("StableId", "user-1:Hat_01");
        outp["questblob.encode"] = (string)Call("EncodeQuestBlob", "{\"a\":1}");

        // 4. ParseBody-style dictionary round trip.
        var body = "{ \"b\" : [1, 2.50, {\"x\" : \"y\\u00e9\\/\"}], \"a\": true, \"b\": null , \"n\":-0.0e5}";
        var dict = JsonSerializer.Deserialize<Dictionary<string, JsonElement>>(body)!;
        outp["body.roundtrip"] = JsonSerializer.Serialize(dict);
        outp["body.keys"] = string.Join(",", dict.Keys);
        outp["element.reserialize"] = JsonSerializer.Serialize(new { e = JsonSerializer.Deserialize<JsonElement>("{ \"k\" : [ 1 , \"<\" ] }") });

        // 5. DecodeEquippedItems.
        var inv = (string)Call("EncodeQuestBlob", "{\"equippedItems\":[\"Hat\",\"None\",\"\",\"Hat\",\"Coat\"]}");
        outp["equipped.decode"] = string.Join(",", (List<string>)Call("DecodeEquippedItems", inv));

        // 6. ApplyAutoCompletedQuests (JsonNode + WriteIndented + Environment.NewLine).
        File.WriteAllText(Path.Combine(AppContext.BaseDirectory, "auto_complete_quests.txt"),
            "# comment\n0123456789abcdef0123456789ABCDEF 4\n  fedcba98-7654-3210-fedc-ba9876543210  # trailing\nbad 3\n11111111111111111111111111111111 x\n");
        using var db = new LiteDB.LiteDatabase(new MemoryStream());
        var col = db.GetCollection<MothershipUserDataDbObject>("ud");
        var created = (MothershipUserDataDbObject)Call("ApplyAutoCompletedQuests", col, "user-1", null);
        outp["quests.fromnull.value"] = created.Value;
        outp["quests.fromnull.generation"] = created.Generation.ToString();
        var existingJson = "{\"lastUpdate\":\"2025.01.01-00.00.00\",\"quests\":[{\"iD\":\"0123456789ABCDEF0123456789abcdef\",\"progress\":10,\"completedVersion\":1,\"x\":1.50},null,5,{\"iD\":\"other\",\"progress\":255,\"note\":\"<" + (char)0xE9 + ">\",\"e\":{},\"arr\":[]}],\"savedQuestProgressionVersion\":3,\"extra\":{\"k\":[1,true,null]}}";
        var item = new MothershipUserDataDbObject { DataId = "d1", UserId = "user-1", KeyName = "player_quests", Value = (string)Call("EncodeQuestBlob", existingJson), Generation = 5 };
        col.Insert(item);
        var updated = (MothershipUserDataDbObject)Call("ApplyAutoCompletedQuests", col, "user-1", item);
        outp["quests.existing.value"] = updated.Value;
        outp["quests.existing.generation"] = updated.Generation.ToString();
        outp["newline.length"] = Environment.NewLine.Length.ToString();

        File.WriteAllText(a[0], JsonSerializer.Serialize(outp, new JsonSerializerOptions { WriteIndented = true }));
        Console.WriteLine("wrote " + outp.Count + " fixtures");
    }
}
