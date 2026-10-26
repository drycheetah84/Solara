// DB round-trip oracle for internal/mothership: inserts every persisted
// AUnrealFeatures.AAMothership.Models type into a real LiteDB 5.0.21 database
// (BsonMapper defaults: TrimWhitespace, EmptyStringToNull), reads it back with
// FindAll().First() and records JsonSerializer.Serialize(readBack).
// Usage: dotnet run -- <fixtures.json>
// Also prints LiteDB predicate semantics for "" / padded / null strings.
using System.Reflection;
using System.Text.Json;
using JsonSerializer = System.Text.Json.JsonSerializer;
using LiteDB;
using AUnrealFeatures.AAMothership.Models;

static class P
{
    static readonly string Special = "v<&'" + (char)0xE9 + ">";

    // Mixed population rule (mirrored by populateMixed in the Go test):
    // the k-th string property (declaration order) gets
    //   k%4==0: ""   k%4==1: "  pad_<Name>  "   k%4==2: initializer kept   k%4==3: "<Name>_" + Special
    static void Populate(object o)
    {
        int k = 0;
        foreach (var p in o.GetType().GetProperties())
        {
            if (!p.CanWrite || p.Name == "Id") continue;
            var t = p.PropertyType;
            if (t == typeof(string))
            {
                switch (k++ % 4)
                {
                    case 0: p.SetValue(o, ""); break;
                    case 1: p.SetValue(o, "  pad_" + p.Name + "  "); break;
                    case 2: break;
                    case 3: p.SetValue(o, p.Name + "_" + Special); break;
                }
            }
            else if (t == typeof(int)) p.SetValue(o, 7);
            else if (t == typeof(long)) p.SetValue(o, 9007199254740993L);
            else if (t == typeof(bool)) p.SetValue(o, !(bool)p.GetValue(o)!);
            else if (t == typeof(DateTime)) p.SetValue(o, new DateTime(2025, 1, 2, 3, 4, 5, DateTimeKind.Utc).AddTicks(1234567));
            else if (t == typeof(List<string>)) p.SetValue(o, new List<string> { "", "  x  ", "y", "   " });
            else if (t == typeof(Dictionary<string, string>)) p.SetValue(o, new Dictionary<string, string> { { "z", "" }, { "a", " 2 " }, { "m", "m" }, { "w", "  " } });
            else throw new Exception("unhandled " + t);
        }
    }

    static object RoundTrip(LiteDatabase db, Type t, object o)
    {
        var m = typeof(LiteDatabase).GetMethods().First(x => x.Name == "GetCollection" && x.IsGenericMethod && x.GetParameters().Length == 2
                                                           && x.GetParameters()[0].ParameterType == typeof(string));
        var col = m.MakeGenericMethod(t).Invoke(db, new object[] { "c" + Guid.NewGuid().ToString("N"), BsonAutoId.ObjectId })!;
        col.GetType().GetMethod("Insert", new[] { t })!.Invoke(col, new[] { o });
        var all = (System.Collections.IEnumerable)col.GetType().GetMethod("FindAll")!.Invoke(col, null)!;
        return all.Cast<object>().First();
    }

    static void Main(string[] a)
    {
        var outp = new SortedDictionary<string, string>();
        var path = Path.Combine(Path.GetTempPath(), "oracle-msdb-" + Guid.NewGuid().ToString("N") + ".db");
        using (var db = new LiteDatabase(new ConnectionString(path) { Connection = ConnectionType.Direct }))
        {
            foreach (var t in typeof(MothershipPlayerDbObject).Assembly.GetTypes()
                         .Where(t => t.Namespace == "AUnrealFeatures.AAMothership.Models" && t.IsClass && !t.IsAbstract).OrderBy(t => t.Name))
            {
                outp["rt.default." + t.Name] = JsonSerializer.Serialize(RoundTrip(db, t, Activator.CreateInstance(t)!), t);
                var f = Activator.CreateInstance(t)!;
                Populate(f);
                outp["rt.mixed." + t.Name] = JsonSerializer.Serialize(RoundTrip(db, t, f), t);
            }

            // Predicate semantics (console only).
            var c = db.GetCollection<MothershipSessionDbObject>("pred");
            c.Insert(new MothershipSessionDbObject { Token = "Abc", PlayerId = "p1" });
            c.Insert(new MothershipSessionDbObject { Token = "", PlayerId = "p2" });
            c.Insert(new MothershipSessionDbObject { Token = "  pad  ", PlayerId = "p3" });
            foreach (var q in new[] { "", " ", "abc", " Abc ", "pad", "  pad  " })
            {
                var r = c.Find(x => x.Token == q).Select(x => x.PlayerId);
                Console.WriteLine($"Token == '{q}' -> [{string.Join(",", r)}]");
            }
            string nul = null;
            Console.WriteLine($"Token == null -> [{string.Join(",", c.Find(x => x.Token == nul).Select(x => x.PlayerId))}]");
        }
        File.Delete(path);
        File.WriteAllText(a[0], JsonSerializer.Serialize(outp, new JsonSerializerOptions { WriteIndented = true }));
        Console.WriteLine("wrote " + outp.Count + " fixtures");
    }
}
