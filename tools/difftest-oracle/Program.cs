// Differential-test oracle for Solara.
//
// Runs scenario steps through the ORIGINAL Halcyon servers in-process:
//   * HttpListener.Start/Stop are neutralised with Harmony, so constructing the real server
//     classes binds no (privileged) ports; everything else in the constructors runs as-is.
//   * Program.Database / MothershipServer.Database point at a fresh LiteDB file built with the
//     real AstraDatabase class, pre-registered in Program.cs order.
//   * Each step goes through a faithful re-implementation of AstraHttpServer.ListenImpl's
//     pipeline (preprocessors, CORS, OPTIONS) and then the server's real private
//     ProcessRequest (routing, binding, handler), via reflection.
//
// usage: difftest-oracle <scenario.json> <workdir>     (prints one JSON object per step)
using System.Collections.Specialized;
using System.Net;
using System.Reflection;
using System.Text;
using System.Text.Json;
using System.Web;
using HarmonyLib;
using AUnrealFeatures.Hosting.Database;
using AUnrealFeatures.Hosting.Http;
using AUnrealFeatures.Hosting.Http.Actions;
using AUnrealFeatures.Hosting.Http.Interfaces;
using AUnrealFeatures.Hosting.Http.Preprocessors;

static class Oracle
{
    static bool SkipListener() => false; // Harmony prefix: skip original method

    static int Main(string[] args)
    {
        if (args.Length < 2) { Console.Error.WriteLine("usage: difftest-oracle <scenario.json> <workdir>"); return 2; }
        var scenario = JsonDocument.Parse(File.ReadAllBytes(args[0])).RootElement;
        Directory.CreateDirectory(args[1]);
        Directory.SetCurrentDirectory(args[1]);

        var harmony = new Harmony("solara.difftest");
        var skip = new HarmonyMethod(typeof(Oracle).GetMethod(nameof(SkipListener), BindingFlags.NonPublic | BindingFlags.Static));
        harmony.Patch(typeof(HttpListener).GetMethod("Start"), prefix: skip);
        harmony.Patch(typeof(HttpListener).GetMethod("Stop"), prefix: skip);

        var dbPath = Path.Combine(Directory.GetCurrentDirectory(), "difftest.db");
        foreach (var f in new[] { dbPath, dbPath + ".meta" }) if (File.Exists(f)) File.Delete(f);
        var db = new AstraDatabase(new SimpleDatabaseDescriptor { databaseId = 2, name = "difftest", path = dbPath, password = null });
        typeof(AUnrealFeatures.Ares.Program).GetField("_database", BindingFlags.NonPublic | BindingFlags.Static)!.SetValue(null, db);
        typeof(AUnrealFeatures.AAMothership.MothershipServer).GetProperty("Database", BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Static)?.SetValue(null, db);
        PreRegister(db);

        var servers = new Dictionary<string, AstraHttpServer>();
        AstraHttpServer Get(string name)
        {
            if (servers.TryGetValue(name, out var s)) return s;
            var t = AppDomain.CurrentDomain.GetAssemblies().SelectMany(a => { try { return a.GetTypes(); } catch { return Array.Empty<Type>(); } })
                .First(x => x.Name == name && typeof(AstraHttpServer).IsAssignableFrom(x));
            s = (AstraHttpServer)Activator.CreateInstance(t)!;
            return servers[name] = s;
        }
        foreach (var asm in new[] { "AUnrealFeatures.Ares", "AUnrealFeatures.AAMothership", "AUnrealFeatures.EOSSDK", "AUnrealFeatures.HalcyonSocket" })
            Assembly.Load(asm);

        int i = 0;
        foreach (var rawStep in scenario.GetProperty("steps").EnumerateArray())
        {
            var step = JsonDocument.Parse(Subst.Apply(rawStep.GetRawText())).RootElement;
            var server = Get(step.GetProperty("server").GetString()!);
            var res = Run(server, step);
            if (step.TryGetProperty("capture", out var cap) && res.TryGetValue("body", out var bodyObj))
                Subst.Capture(cap, bodyObj as string);
            res["step"] = i++;
            Console.WriteLine(JsonSerializer.Serialize(res));
        }
        return 0;
    }

    static void PreRegister(AstraDatabase db)
    {
        // Program.cs order
        var names = new[] {
            "AUnrealFeatures.Ares.Models.UserDataResponse", "AUnrealFeatures.Ares.Models.RoleResponse", "AUnrealFeatures.Ares.Models.DrafterGrant",
            "AUnrealFeatures.Ares.Models.StationDbObject", "AUnrealFeatures.Ares.Models.DeploymentDbObject", "AUnrealFeatures.Ares.Models.ServerEventDbObject",
            "AUnrealFeatures.Ares.Models.StationEventDbObject", "AUnrealFeatures.Ares.Models.EventSignupDbObject", "AUnrealFeatures.Ares.Models.UserApiKeyDbObject",
            "AUnrealFeatures.AAMothership.Models.MothershipPlayerDbObject", "AUnrealFeatures.AAMothership.Models.MothershipSessionDbObject",
            "AUnrealFeatures.AAMothership.Models.MothershipV2PlayerDbObject", "AUnrealFeatures.AAMothership.Models.MothershipAssociationDbObject",
            "AUnrealFeatures.AAMothership.Models.MothershipLinkDbObject", "AUnrealFeatures.AAMothership.Models.MothershipSharedGroupDbObject",
            "AUnrealFeatures.AAMothership.Models.MothershipReportDbObject", "AUnrealFeatures.AAMothership.Models.MothershipBanDbObject",
            "AUnrealFeatures.AAMothership.Models.MothershipMuteDbObject" };
        var gc = typeof(AstraDatabase).GetMethod("GetCollection")!;
        foreach (var n in names)
        {
            var t = AppDomain.CurrentDomain.GetAssemblies().Select(a => a.GetType(n)).FirstOrDefault(x => x != null)
                ?? Assembly.Load(n.StartsWith("AUnrealFeatures.AAMothership") ? "AUnrealFeatures.AAMothership" : "AUnrealFeatures.Ares").GetTypes().FirstOrDefault(x => x.FullName == n);
            if (t == null) { Console.Error.WriteLine($"[oracle] type not found: {n}"); continue; }
            gc.MakeGenericMethod(t).Invoke(db, new object[] { true });
        }
    }

    static Dictionary<string, object> Run(AstraHttpServer server, JsonElement step)
    {
        var ctx = new FakeContext(step);
        var result = new Dictionary<string, object>();
        IHttpActionResult action = null;
        try
        {
            var preprocessors = (System.Collections.IList)typeof(AstraHttpServer).GetField("_preprocessors", BindingFlags.NonPublic | BindingFlags.Instance)!.GetValue(server)!;
            bool cont = true;
            foreach (IHttpRequestPreprocessor p in preprocessors)
            {
                var r = p.TryPreprocessRequest(ctx.Request, ctx.Response).GetAwaiter().GetResult();
                cont = !r.result.HasFlag(HttpPreprocessorResult.STOP_AFTER);
                if (!cont && action == null) { action = r.actionResult; ctx.Resp.Freeze(); }
            }
            ctx.Response.EnableCors();
            if (action == null)
            {
                if (ctx.Request.Method.Method == "OPTIONS") action = Results.NoContent();
                else if (cont)
                {
                    typeof(AstraHttpServer).GetField("_httpContext", BindingFlags.NonPublic | BindingFlags.Instance)!.SetValue(server, ctx);
                    var task = (Task<IHttpActionResult>)typeof(AstraHttpServer).GetMethod("ProcessRequest", BindingFlags.NonPublic | BindingFlags.Instance)!.Invoke(server, new object[] { ctx })!;
                    action = task.GetAwaiter().GetResult();
                }
            }
            Fill(result, action, ctx);
        }
        catch (Exception ex)
        {
            var inner = ex is TargetInvocationException tie && tie.InnerException != null ? tie.InnerException : ex;
            Fill(result, Results.HtmlDocument(HttpStatusCode.InternalServerError, HtmlDocumentCache.InternalServerErrorDocumentString.Replace("{0}", inner.Message)), ctx);
        }
        return result;
    }

    static void Fill(Dictionary<string, object> r, IHttpActionResult a, FakeContext ctx)
    {
        if (a == null) { r["status"] = 0; return; }
        byte[] body;
        try { body = a.Body; } catch (Exception ex) { r["status"] = 500; r["body"] = "serialization: " + ex.Message; return; }
        r["status"] = (int)a.StatusCode;
        r["contentType"] = a.ContentType;
        r["headers"] = ctx.Resp.HeadersAtApply;
        r["body"] = Encoding.UTF8.GetString(body);
    }
}

sealed class FakeContext : IHttpContext
{
    public FakeContext(JsonElement step)
    {
        Req = new FakeRequest(step);
        Resp = new FakeResponse();
        Session = AstraHttpSession.New(Guid.NewGuid().ToString(), "Default", DateTime.UtcNow.AddMinutes(30), new List<string>(), new Dictionary<string, string>());
    }
    public FakeRequest Req { get; }
    public FakeResponse Resp { get; }
    public IHttpRequest Request => Req;
    public IHttpResponse Response => Resp;
    public IHttpSession Session { get; }
}

// Mirrors AstraHttpRequest semantics on top of scenario data.
sealed class FakeRequest : IHttpRequest
{
    readonly string _path, _query; readonly byte[] _body; readonly WebHeaderCollection _h = new();
    Dictionary<string, object> _json; Dictionary<string, string> _form;
    public FakeRequest(JsonElement step)
    {
        var full = step.GetProperty("path").GetString()!;
        var q = full.IndexOf('?');
        _path = q < 0 ? full : full[..q];
        _query = q < 0 ? "" : full[(q + 1)..];
        Method = new HttpMethod(step.GetProperty("method").GetString()!);
        if (step.TryGetProperty("headers", out var hs)) foreach (var h in hs.EnumerateObject()) _h.Add(h.Name, h.Value.GetString());
        _body = step.TryGetProperty("body", out var b) ? Encoding.UTF8.GetBytes(b.GetString()!) : Array.Empty<byte>();
        _h["Host"] = "localhost";
    }
    public string GetHeaderValue(string key, string defaultValue = null) => Headers.TryGetValue(key, out var v) ? v : defaultValue;
    public string GetQueryParameter(string key, string defaultValue = null) => Queries.TryGetValue(key, out var v) ? v : defaultValue;
    public HttpMethod Method { get; }
    public string Host => "localhost";
    public string RequestId => Guid.NewGuid().ToString();
    public string Uri
    {
        get
        {
            var sb = new StringBuilder(); bool prev = false;
            foreach (var c in _path) { if (c == '/') { if (prev) continue; prev = true; } else prev = false; sb.Append(c); }
            return sb.ToString();
        }
    }
    public Dictionary<string, string> Headers => _h.AllKeys.ToDictionary(k => k!, k => _h[k]!, StringComparer.OrdinalIgnoreCase);
    public Dictionary<string, string> Queries { get { var nv = HttpUtility.ParseQueryString(_query); return nv.AllKeys.ToDictionary(k => k!, k => nv[k])!; } }
    public byte[] Body => _body;
    public Dictionary<string, object> JsonBody
    {
        get
        {
            if (_json == null)
            {
                if (_h["Content-Type"]!.StartsWith("application/json", StringComparison.OrdinalIgnoreCase))
                    _json = JsonSerializer.Deserialize<Dictionary<string, object>>(Body) ?? new Dictionary<string, object>();
                else throw new InvalidOperationException("The request body is not in JSON format.");
            }
            return _json;
        }
    }
    public Dictionary<string, string> FormBody
    {
        get
        {
            if (_form == null)
            {
                if (_h["Content-Type"]!.StartsWith("application/x-www-form-urlencoded", StringComparison.OrdinalIgnoreCase))
                {
                    var nv = HttpUtility.ParseQueryString(Encoding.UTF8.GetString(Body));
                    _form = nv.AllKeys.ToDictionary(k => k!, k => nv[k])!;
                }
                else throw new InvalidOperationException("The request body is not in URL-encoded format.");
            }
            return _form;
        }
    }
    public IPAddress Remote
    {
        get
        {
            if (Headers.ContainsKey("Cf-Connecting-Ip")) return IPAddress.Parse(Headers["Cf-Connecting-Ip"]);
            if (Headers.ContainsKey("X-Real-Ip")) return IPAddress.Parse(Headers["X-Real-Ip"]);
            return IPAddress.Loopback;
        }
    }
    public IPAddress Origin => IPAddress.Loopback;
}

sealed class FakeResponse : IHttpResponse
{
    readonly Dictionary<string, string> _headers = new();
    Dictionary<string, string> _frozen;
    public void Freeze() => _frozen = new Dictionary<string, string>(_headers);
    public Dictionary<string, string> HeadersAtApply => _frozen ?? new Dictionary<string, string>(_headers);
    public HttpStatusCode StatusCode { get; set; } = HttpStatusCode.OK;
    public string ContentType { get; set; }
    public byte[] Content { get; set; } = Array.Empty<byte>();
    public Dictionary<string, string> Headers => _headers;
    public void SetHeader(string key, string value) => _headers[key] = value;
    public void RemoveHeader(string key) => _headers.Remove(key);
    public void SetCookie(Cookie cookie) => _headers["Set-Cookie"] = cookie.ToString();
    public void Redirect(string url) => _headers["Location"] = url;
    public void SetContentString(string content, Encoding encoding = null) => Content = (encoding ?? Encoding.UTF8).GetBytes(content);
    public Task WriteContentAsync(Stream stream) => Task.CompletedTask;
    public void SetJsonContent(object obj) { SetContentString(JsonSerializer.Serialize(obj)); ContentType = "application/json"; }
    public void SetXmlContent(object obj) { }
    public void EnableCors(string origin = "*", string methods = "GET,POST,PUT,DELETE,OPTIONS", string headers = "*")
    { SetHeader("Access-Control-Allow-Origin", origin); SetHeader("Access-Control-Allow-Methods", methods); SetHeader("Access-Control-Allow-Headers", headers); }
    public void SetContentDisposition(string fileName, bool inline = false) => SetHeader("Content-Disposition", $"{(inline ? "inline" : "attachment")}; filename=\"{fileName}\"");
    public void SetCacheControl(int maxAgeSeconds) => SetHeader("Cache-Control", $"max-age={maxAgeSeconds}, public");
    public void ApplyToHttpListenerResponse(IHttpActionResult httpActionResult) { }
}

// ${secret:NAME} -> value from the file named by SOLARA_SECRETS_FILE (KEY=VALUE lines);
// ${var:NAME} -> a value captured from an earlier step's JSON response ("capture": {"NAME": "a.b.c"}).
static class Subst
{
    static readonly Dictionary<string, string> Vars = new();
    static Dictionary<string, string> _secrets;
    static Dictionary<string, string> Secrets()
    {
        if (_secrets != null) return _secrets;
        _secrets = new();
        var f = Environment.GetEnvironmentVariable("SOLARA_SECRETS_FILE");
        if (f != null && File.Exists(f))
            foreach (var line in File.ReadAllLines(f))
            {
                var t = line.Trim(); if (t.Length == 0 || t.StartsWith("#")) continue;
                var i = t.IndexOf('='); if (i > 0) _secrets[t[..i].Trim()] = t[(i + 1)..];
            }
        return _secrets;
    }
    static string Esc(string s) => JsonSerializer.Serialize(s)[1..^1];
    public static string Apply(string json) =>
        System.Text.RegularExpressions.Regex.Replace(json, @"\$\{(secret|var):([A-Za-z0-9_]+)\}", m =>
            Esc(m.Groups[1].Value == "secret" ? (Secrets().TryGetValue(m.Groups[2].Value, out var s) ? s : "")
                                              : (Vars.TryGetValue(m.Groups[2].Value, out var v) ? v : "")));
    public static void Capture(JsonElement cap, string body)
    {
        JsonElement root;
        try { root = JsonDocument.Parse(body ?? "").RootElement; } catch { return; }
        foreach (var p in cap.EnumerateObject())
        {
            var cur = root; bool ok = true;
            foreach (var seg in p.Value.GetString()!.Split('.'))
                if (cur.ValueKind == JsonValueKind.Object && cur.TryGetProperty(seg, out var nx)) cur = nx; else { ok = false; break; }
            if (ok) Vars[p.Name] = cur.ValueKind == JsonValueKind.String ? cur.GetString()! : cur.GetRawText();
        }
    }
}
