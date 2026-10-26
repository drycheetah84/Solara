using LiteDB;
class D { public ObjectId Id {get;set;} = ObjectId.NewObjectId();
  public string? A {get;set;}                 // no init, set ""
  public string B {get;set;} = "init";        // init, set ""
  public string? F {get;set;} = "init";       // nullable init, set ""
  public string G {get;set;} = "init";        // init, set null
  public string? H {get;set;}                 // no init, set "  "
  public string I {get;set;} = "";            // init "", untouched
}
static class P { static string S(string? x) => x==null?"null":"'"+x+"'"; static void Main(){
 var path = Path.GetTempFileName(); File.Delete(path);
 using var db = new LiteDatabase(new ConnectionString(path){Connection=ConnectionType.Direct});
 var c = db.GetCollection<D>("d");
 c.Insert(new D{ A="", B="", F="", G=null!, H="  " });
 Console.WriteLine(db.GetCollection("d").FindAll().First().ToString());
 var r = c.FindAll().First();
 Console.WriteLine($"A={S(r.A)} B={S(r.B)} F={S(r.F)} G={S(r.G)} H={S(r.H)} I={S(r.I)}");
 Console.WriteLine("mapper: EmptyStringToNull=" + BsonMapper.Global.EmptyStringToNull + " TrimWhitespace=" + BsonMapper.Global.TrimWhitespace + " SerializeNullValues=" + BsonMapper.Global.SerializeNullValues);
}}
