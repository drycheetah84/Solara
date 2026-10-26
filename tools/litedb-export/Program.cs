// usage: litedb-export <path-to.db> <out-dir>
// The database password, if any, is read from the LITEDB_PASSWORD environment variable
// (never from the command line, so it does not land in shell history).
using LiteDB;
if (args.Length != 2) { Console.Error.WriteLine("usage: litedb-export <db-file> <out-dir>"); return 2; }
var src = args[0]; var outDir = args[1];
if (!File.Exists(src)) { Console.Error.WriteLine($"not found: {src}"); return 2; }
Directory.CreateDirectory(outDir);
var cs = new ConnectionString(src) { Connection = ConnectionType.Direct, ReadOnly = true };
var pw = Environment.GetEnvironmentVariable("LITEDB_PASSWORD");
if (!string.IsNullOrEmpty(pw)) cs.Password = pw;
using var db = new LiteDatabase(cs);
var manifest = new List<string>();
foreach (var name in db.GetCollectionNames().OrderBy(n => n, StringComparer.Ordinal)) {
  var col = db.GetCollection(name);
  var file = Path.Combine(outDir, name + ".jsonl");
  int n = 0;
  using (var w = new StreamWriter(file, false, new System.Text.UTF8Encoding(false))) {
    foreach (var doc in col.FindAll()) { w.Write(LiteDB.JsonSerializer.Serialize(doc)); w.Write('\n'); n++; }
  }
  manifest.Add($"{name}\t{n}");
  Console.WriteLine($"{name}: {n} documents");
}
File.WriteAllLines(Path.Combine(outDir, "manifest.tsv"), manifest);
var meta = src + ".meta";
if (File.Exists(meta)) File.Copy(meta, Path.Combine(outDir, "collections.meta"), true);
return 0;
