// Emits the original route table exactly as AstraHttpServer.ScanForServerBasedEndpoints
// builds it: Type.GetMethods() (public only, reflection order) with an HttpEndpointAttribute,
// route "/" + Uri.Trim('/'). Output: server<TAB>METHOD<TAB>route<TAB>methodName
using System.Reflection;
using AUnrealFeatures.Hosting.Http.Attributes;
var asms = new[]{"AUnrealFeatures.AAMothership","AUnrealFeatures.Ares","AUnrealFeatures.EOSSDK","AUnrealFeatures.HalcyonSocket"};
foreach (var a in asms) {
  var asm = Assembly.Load(a);
  foreach (var t in asm.GetTypes().Where(t => t.IsSubclassOf(typeof(AUnrealFeatures.Hosting.Http.AstraHttpServer))).OrderBy(t=>t.Name)) {
    foreach (var m in t.GetMethods()) {
      var at = m.GetCustomAttribute<HttpEndpointAttribute>();
      if (at == null) continue;
      Console.WriteLine($"{t.Name}\t{at.Method.Method}\t/{at.Uri.Trim('/')}\t{m.Name}");
    }
    // endpoints declared on non-public methods are never registered; list them for the audit
    foreach (var m in t.GetMethods(BindingFlags.NonPublic|BindingFlags.Instance|BindingFlags.Static)) {
      var at = m.GetCustomAttribute<HttpEndpointAttribute>();
      if (at != null) Console.Error.WriteLine($"UNREGISTERED(non-public)\t{t.Name}\t{at.Method.Method}\t{at.Uri}\t{m.Name}");
    }
  }
}
