// Ground-truth capture for the HalcyonSocket port (Solara internal/halcyonsocket).
// Runs the ORIGINAL AgentHub / ServerWatchdog assemblies and prints what they do.
// Usage: dotnet run -- <mode>   modes: dto | hub | wd | fast | misc
// Non-ASCII characters are written as ~XXXX (4 hex digits) and expanded by X().
using System.Net.Sockets;
using System.Reflection;
using System.Text;
using System.Text.Json;
using System.Text.RegularExpressions;
using AUnrealFeatures.HalcyonSocket;

var mode = args.Length > 0 ? args[0] : "dto";
var real = Console.Out;
var cap = new StringWriter();
void Out(string s) => real.WriteLine(s);
void Flush() { foreach (var l in cap.ToString().Split('\n', StringSplitOptions.RemoveEmptyEntries)) real.WriteLine("CONSOLE " + Esc(l.TrimEnd('\r'))); cap.GetStringBuilder().Clear(); }

if (mode == "dto")
{
    var dt = new DateTime(2026, 10, 25, 12, 34, 56, 789, DateTimeKind.Utc).AddTicks(1234);
    Out(JsonSerializer.Serialize(new WatchdogRow()));
    Out(JsonSerializer.Serialize(new WatchdogRow { deployment_id = "d<1>", name = X("N~00e9"), box = "b", pid = 7, players = -1, uptime_minutes = 3, last_heartbeat = dt, dispatch_age_ms = ulong.MaxValue, tick_age_ms = 5, verdict = "v", restarts = 2, last_restart = dt, retired = true, retired_reason = "r" }));
    Out(JsonSerializer.Serialize(new WatchdogStatusResult()));
    Out(JsonSerializer.Serialize(new SpinUpResult()));
    Out(JsonSerializer.Serialize(new SpinUpBody()));
    Out(JsonSerializer.Serialize(new WatchdogBody()));
    Out(JsonSerializer.Serialize(new SuccessResult()));
    Out(JsonSerializer.Serialize(new AgentInfo { box = "x", capacity = 2, running = 1, free = 1, last_seen = dt }));
    Out(JsonSerializer.Serialize(new AgentListResult()));
    Out(JsonSerializer.Serialize(new { type = "spinup", id = "abc", map = "M&<'>", region = X("~00fc+~4e2d"), args = "-a=\"q\" \\ \t" }));
    Out(JsonSerializer.Serialize(new { type = "kill", pid = "12", reason = "engine-frozen" }));
    foreach (var b in new[] { "", "null", "{}", "[]", "{\"box\":5}", "{\"Box\":\"x\",\"box\":\"y\",\"map\":null}", "{\"deployment_id\":\"a\",\"reason\":null}" })
    {
        string r;
        try { var x = JsonSerializer.Deserialize<SpinUpBody>(Encoding.UTF8.GetBytes(b)); r = x == null ? "null" : $"box={x.box ?? "<null>"} map={x.map ?? "<null>"}"; }
        catch (Exception e) { r = "EX " + e.GetType().Name; }
        Out($"SpinUpBody[{b}] => {r}");
    }
    return;
}

if (mode == "misc")
{
    var re = new Regex(@"-DashboardDeploymentId=(\S+)", RegexOptions.IgnoreCase);
    foreach (var a in new[] { "-x -DashboardDeploymentId=abc -y", X("-dashboarddeploymentid=Q1~00a0tail"), X("-DASHBOARDDEPLOYMENTID=~2003x"), X("-DashboardDeploymentId=a~0085b"), X("-Dashboard~017fDeploymentId=no"), "-DashboardDeploymentId=", "-DashboardDeploymentId=one -DashboardDeploymentId=two", X("-DashboardDeploymentId=x~200by") })
    {
        var m = re.Match(a);
        Out($"RE[{Esc(a)}] => {(m.Success ? Esc(m.Groups[1].Value) : "<nomatch>")}");
    }
    var ens = typeof(ServerWatchdog).GetMethod("EnsureDeploymentArg", BindingFlags.NonPublic | BindingFlags.Static)!;
    foreach (var a in new[] { "", "  -a  ", "-dashboarddeploymentid=x", X("-Dashboard~0130DeploymentId=x") })
        Out($"ENSURE[{Esc(a)}] => {Esc((string)ens.Invoke(null, new object[] { a, "DEP" })!)}");
    foreach (var s in new[] { "12", " +12 ", "-0", "1\0\0", "1 \0", "2147483648", "-2147483648", "", "1.0", X("~0661"), "\t7\n" })
        Out($"TRYPARSE[{Esc(s)}] => {(int.TryParse(s, out var v) ? v.ToString() : "fail")}");
    foreach (var d in new[] { 0.5, 1.5, 2.5, -0.4, 179.9996, 12345.5 })
        Out($"F0[{d:R}] => {d:F0}");
    var names = new[] { "b", "B", "a", "", X("~d83d~de00"), X("~ff21"), "aa", "Z" };
    Out("ORDER => " + string.Join("|", names.OrderBy(x => x).Select(Esc)));
    Out("TRIM => " + Esc(X(" ~00a0~3000x~2028\t ").Trim()));
    var shortM = typeof(ServerWatchdog).GetMethod("Short", BindingFlags.NonPublic | BindingFlags.Static)!;
    foreach (var s in new[] { "abc", "0123456789", X("abcdefg~d83d~de00z") })
        Out($"SHORT[{Esc(s)}] => {Esc((string)shortM.Invoke(null, new object[] { s })!)}");
    return;
}

if (mode == "hub")
{
    Console.SetOut(cap);
    var hub = new AgentHub(19100);
    hub.OnProbeResult = (b, p, al) => Console.WriteLine($"CB probe_result box={b} pid={p} alive={al}");
    hub.OnSpunUp = (r, p, s, b) => Console.WriteLine($"CB spunup req={r} pid={p} status={s} box={b}");
    hub.OnExited = (b, p, c) => Console.WriteLine($"CB exited box={b} pid={p} code={c}");
    hub.Start();
    var cl = new TcpClient(); cl.Connect("127.0.0.1", 19100);
    var ns = cl.GetStream();
    var rd = new StreamReader(ns, new UTF8Encoding(false));
    void Send(string s) { var b = Encoding.UTF8.GetBytes(s); ns.Write(b, 0, b.Length); ns.Flush(); Thread.Sleep(150); }
    void SendBytes(byte[] b) { ns.Write(b, 0, b.Length); ns.Flush(); Thread.Sleep(150); }
    void Agents() { foreach (var a in hub.Agents) Console.WriteLine($"AGENT box={a.Box} cap={a.Capacity} running={a.Running} free={a.Free}"); }
    Thread.Sleep(150);
    Agents();
    Send("{\"type\":\"hel"); Send("lo\",\"box\":\"boxA\",\"capacity\":2,\"running\":0}\r\n");
    Agents();
    Console.WriteLine("PICK null => " + (hub.Pick(null)?.Box ?? "<none>"));
    Console.WriteLine("PICK empty => " + (hub.Pick("")?.Box ?? "<none>"));
    Console.WriteLine("PICK boxA => " + (hub.Pick("boxA")?.Box ?? "<none>"));
    Console.WriteLine("PICK boxZ => " + (hub.Pick("boxZ")?.Box ?? "<none>"));
    Console.WriteLine("PROBE => " + hub.Probe("boxA", "123")); Console.WriteLine("WIRE " + rd.ReadLine());
    Console.WriteLine("KILL => " + hub.Kill("boxA", "123", "engine-frozen <&>")); Console.WriteLine("WIRE " + rd.ReadLine());
    Console.WriteLine("PROBEALL => " + hub.ProbeAll("9")); Console.WriteLine("WIRE " + rd.ReadLine());
    Console.WriteLine("KILL none => " + hub.Kill("nobox", "1", "x"));
    Send("{\"type\":\"heartbeat\",\"running\":\"1\"}\n"); Agents();
    Send("{\"type\":\"heartbeat\",\"running\":1}\n"); Agents();
    Send("{\"type\":\"hello\",\"box\":\"boxB\",\"capacity\":\"x\",\"running\":0}\n"); Agents();
    Send("{\"type\":\"hello\",\"box\":\"boxB\",\"capacity\":2.5,\"running\":2}\n"); Agents();
    Send("{\"type\":\"hello\",\"box\":\"boxB\",\"capacity\":3,\"running\":true}\n"); Agents();
    Send("[1]\n garbage \n\n   \n{\"type\":\"probe_result\",\"pid\":77,\"alive\":true}\n{\"type\":\"probe_result\",\"pid\":\"78\",\"alive\":\"true\"}\n");
    Send("{\"type\":\"spunup\",\"id\":\"r1\",\"pid\":\"4242\",\"status\":\"ok\",\"msg\":\"launched\"}\n{\"type\":\"spunup\",\"id\":true,\"status\":null}\n");
    Send("{\"type\":\"exited\",\"pid\":\"4242\",\"code\":\"killed\",\"running\":1}\n{\"type\":\"exited\",\"pid\":\"5\",\"code\":\"x\",\"running\":\"1\"}\n");
    Send("{\"type\":\"heartbeat\",\"running\":0}{\"type\":\"x\"}\n");
    Agents();
    // a multi-byte char split across two reads -> each part decoded alone
    var hb = Encoding.UTF8.GetBytes("{\"type\":\"hello\",\"box\":\"caf" + X("~00e9") + "\",\"capacity\":1,\"running\":0}\n");
    int cut = Array.IndexOf(hb, (byte)0xC3) + 1;
    SendBytes(hb[..cut]); SendBytes(hb[cut..]); Agents();
    SendBytes(new byte[] { 0x7b, 0x22, 0x74, 0x79, 0x70, 0x65, 0x22, 0x3a, 0x22, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x22, 0x2c, 0x22, 0x62, 0x6f, 0x78, 0x22, 0x3a, 0x22, 0xE2, 0x82, 0x41, 0xF0, 0x9F, 0x80, 0xC0, 0x22, 0x7d, 0x0a }); Agents();
    cl.Close(); Thread.Sleep(300);
    Agents();
    Console.WriteLine("END");
    Console.SetOut(real); Flush();
    return;
}

if (mode == "wd" || mode == "fast")
{
    // Knobs are static readonly: set env before the type initializer runs.
    Environment.SetEnvironmentVariable("HALCYON_WD_TICK_MS", "100000000");
    if (mode == "fast")
    {
        Environment.SetEnvironmentVariable("HALCYON_WD_HB_GRACE_MS", "40");
        Environment.SetEnvironmentVariable("HALCYON_WD_SILENT_MS", "80");
        Environment.SetEnvironmentVariable("HALCYON_WD_COOLDOWN_MS", "1");
    }
    Console.SetOut(cap);
    var hub = new AgentHub(mode == "wd" ? 19101 : 19102);
    hub.Start();
    var cl = new TcpClient(); cl.Connect("127.0.0.1", mode == "wd" ? 19101 : 19102);
    var ns = cl.GetStream();
    var rd = new StreamReader(ns, new UTF8Encoding(false));
    var b0 = Encoding.UTF8.GetBytes("{\"type\":\"hello\",\"box\":\"boxA\",\"capacity\":4,\"running\":0}\n"); ns.Write(b0, 0, b0.Length);
    Thread.Sleep(200);
    ServerWatchdog.TakeDeploymentOffline = (d, r) => Console.WriteLine($"HOOK offline {d} {r}");
    var wd = new ServerWatchdog(hub, (b, m, r, a, n) => { Console.WriteLine($"SPINUP box={b ?? "<null>"} map={m ?? "<null>"} region={r ?? "<null>"} args={a ?? "<null>"} name={n ?? "<null>"}"); return new SpinUpResult { success = true, request_id = "REQ", agent = b ?? "" }; });
    hub.OnSpunUp = wd.OnSpunUp; hub.OnExited = wd.OnExited; hub.OnProbeResult = wd.OnProbeResult;
    Thread.Sleep(300); // let the constructor's first Evaluate pass run on an empty set
    var eval = typeof(ServerWatchdog).GetMethod("Evaluate", BindingFlags.NonPublic | BindingFlags.Instance)!;
    void Ev() { Console.WriteLine("-- evaluate"); eval.Invoke(wd, null); }
    void Dump() { foreach (var w in wd.Servers.OrderBy(x => x.DeploymentId, StringComparer.Ordinal)) Console.WriteLine($"STATE {w.DeploymentId} box={w.Box} pid={w.Pid} verdict={w.LastVerdict} restarts={w.RestartsInWindow} retired={w.Retired} reason={w.RetiredReason} hb={(w.LastHeartbeat != null)} name={w.Name}"); }
    string ReadWire() { ns.ReadTimeout = 500; try { return rd.ReadLine() ?? "<eof>"; } catch { return "<none>"; } }

    if (mode == "wd")
    {
        wd.OnSpinUpRequested("dep-frozen-0001", "req1", "boxA", "mapA", null, "-x", "Alpha");
        wd.OnSpunUp("req1", "4242", "ok", "boxA");
        wd.OnSpunUp("req1", "4242", "ok", "boxA"); // second: no mapping
        wd.OnSpinUpRequested("dep-healthy-0002", "req2", "boxA", null, "eu", "", "  ");
        wd.OnSpunUp("req2", " 77 ", "ok", "boxA");
        wd.OnSpinUpRequested("dep-start-0003", "req3", "boxA", "m", "r", "-DashboardDeploymentId=dep-start-0003", "Gamma");
        wd.OnSpinUpRequested("dep-fail-0004", "req4", "boxA", "m", "r", "", "Delta");
        wd.OnSpunUp("req4", "", "error", "boxA");
        wd.OnSpinUpRequested("dep-nobox-0005", "req5", "boxZ", "m", "r", "", "Eps");
        wd.OnHeartbeat("dep-frozen-0001", 4242, 1, 100000, 95000, 7200000, 3);
        wd.OnHeartbeat("dep-healthy-0002", 77, 1, 10, 10, 7260000, 5);
        wd.OnHeartbeat("dep-adopt-0006", 999, 1, 10, 10, 60000, 0);
        wd.OnHeartbeat("  ", 1, 1, 1, 1, 1, 1);
        Ev(); Dump();
        Console.WriteLine("WIRE " + ReadWire());
        Console.WriteLine("WIRE " + ReadWire());
        wd.OnProbeResult("boxA", "999", true);
        Ev(); Dump();
        Console.WriteLine("STOP adopt => " + wd.Stop("dep-adopt-0006", "station deleted"));
        Console.WriteLine("WIRE " + ReadWire());
        Console.WriteLine("STOP none => " + wd.Stop("nope", "x"));
        Console.WriteLine("STOP start => " + wd.Stop("dep-start-0003", "x"));
        Console.WriteLine("STOP healthy => " + wd.Stop("dep-healthy-0002", "y"));
        Console.WriteLine("WIRE " + ReadWire());
        Console.WriteLine("RETIRE => " + wd.Retire("dep-nobox-0005", "manual") + " " + wd.Retire("zzz", "manual"));
        Console.WriteLine("RESUME => " + wd.Resume("dep-nobox-0005") + " " + wd.Resume("zzz"));
        // exited path
        wd.OnSpinUpRequested("dep-exit-0007", "req7", "boxA", "m7", "r7", "-p", "Seven");
        wd.OnSpunUp("req7", "55", "ok", "boxA");
        wd.OnExited("boxA", "56", "1");
        wd.OnExited("boxA", "55", "1");
        Console.WriteLine("WIRE " + ReadWire());
        wd.OnExited("boxA", "55", "1");
        Ev(); Dump();
        Console.WriteLine("FORGET => " + wd.Forget("dep-exit-0007") + " " + wd.Forget("dep-exit-0007"));
    }
    else
    {
        // fast knobs: grace 40ms, silent 80ms, cooldown 1ms
        wd.OnSpinUpRequested("dep-silent-0001", "r1", "boxA", "m", "r", "-a", "S1");
        wd.OnSpunUp("r1", "100", "ok", "boxA");
        wd.OnHeartbeat("dep-silent-0001", 100, 1, 10, 10, 60000, 1);
        Ev(); Dump();
        Thread.Sleep(50); Ev(); Dump();          // quiet -- waiting
        Thread.Sleep(100); Ev(); Dump();         // probe sent
        Console.WriteLine("WIRE " + ReadWire());
        Ev(); Dump();                            // probe already sent: nothing new
        wd.OnProbeResult("boxA", "100", true);
        Ev(); Dump();                            // silent -> restart
        Console.WriteLine("WIRE " + ReadWire());
        // crash loop -> GIVING UP
        for (int i = 0; i < 4; i++)
        {
            wd.OnSpinUpRequested("dep-loop-0002", "L" + i, "boxA", "m", "r", "-b", "Loop");
            wd.OnSpunUp("L" + i, (200 + i).ToString(), "ok", "boxA");
            Thread.Sleep(5);
            wd.OnExited("boxA", (200 + i).ToString(), "-1");
            Console.WriteLine("WIRE " + ReadWire());
        }
        Dump();
    }
    Console.SetOut(real); Flush();
    return;
}

static string X(string s) => Regex.Replace(s, "~([0-9a-fA-F]{4})", m => ((char)Convert.ToInt32(m.Groups[1].Value, 16)).ToString());
static string Esc(string s) { var sb = new StringBuilder(); foreach (var c in s) { if (c < 0x20 || c > 0x7e) sb.Append($"<{(int)c:X4}>"); else sb.Append(c); } return sb.ToString(); }
