# Solara porting guide (C# Halcyon → Go)

Read this completely before porting any server. It is the shared contract that lets
modules be ported in parallel and still link together.

Original source (read-only snapshot): `reference/halcyon/backend/src/` (never edit it).
Authoritative route table (from reflection over the built original): `docs/original_routes.tsv`
(`server<TAB>METHOD<TAB>route<TAB>C# method name`, in registration order).

## 0. Tooling hazard — read first

The tool layer that writes files **decodes `\u` followed by four hex digits into the literal
character** in anything you write (Write/Edit/Bash heredocs). Never type that sequence in Go
source. If you need a unicode escape in a Go string, use a `\x..` UTF-8 byte escape, `\U0000XXXX`
(8 digits is safe), `string(rune(0xXXXX))`, or build it at runtime (`"\\" + "u"`). Check with
`grep -n` afterwards if in doubt.

## 1. Packages and naming (deterministic, so modules can reference each other before they exist)

| C# | Go |
|---|---|
| `AUnrealFeatures.AAMothership` (MothershipServer) | `solara/internal/mothership` |
| `AUnrealFeatures.AAMothership.Models` | `solara/internal/mothership/models` |
| `AUnrealFeatures.Ares` (Program statics, Servers/*) | `solara/internal/ares` |
| `AUnrealFeatures.Ares.Models` | `solara/internal/ares/models` |
| `AUnrealFeatures.EOSSDK` | `solara/internal/eos` |
| `AUnrealFeatures.EOSSDK.Models` | `solara/internal/eos/models` |
| `AUnrealFeatures.HalcyonSocket` | `solara/internal/halcyonsocket` |

* C# class/struct/enum names → **same** Go type names. C# property names → **same** Go field names.
* A C# `public static` member `Class.Member` of a server/helper class → Go package-level identifier
  named `Member` in that class's package (e.g. `EosGatewayServer.UpsertSession(...)` →
  `eos.UpsertSession(...)`, `HalcyonSocketServer.PendingNames` → `halcyonsocket.PendingNames`,
  `Program.Database` → `ares.Database`, `MothershipServer.Database` → `mothership.Database`).
  If two classes in one package have a clashing static name, prefix with the class name
  (`StationAclPush`) and say so in your report. Static helper classes (StationAcl, SpecLevelsStore,
  NetvarDumpStore, EosJwtFactory, LobbyWsHelper, ...) follow the same rule: their statics become
  package-level identifiers named exactly like the C# member (exported).
* Keep parameter order and meaning identical to the C# signature. Map C# types:
  `string`→`string` (or `*string` when null is observable), `int`→`int`, `long`→`int64`,
  `double`→`float64`, `float`→`float32`, `bool`→`bool`, `DateTime`→`dotnet.DateTime`,
  `DateTime?`→`*dotnet.DateTime`, `List<T>`→`[]T`, `Dictionary<string,T>`→`map[string]T`
  (or `stj.Obj` when its JSON key order is observable and insertion-ordered),
  `JsonElement`→`stj.Element`, `object`→`any`, `Guid`→`string`.
* Instance state of a server class → fields on the Go server struct. Static state → package-level
  vars guarded by a `sync.Mutex`/`sync.RWMutex` where the original used locks or concurrent
  collections (ConcurrentDictionary → map + mutex, preserving semantics).

## 2. Models (JSON and storage contract — ALL FIELDS MUST STAY THE SAME)

* One Go struct per C# model, same name, fields in the **same declaration order**.
* JSON name: `json:"<JsonPropertyName>"` when the C# property has `[JsonPropertyName]`,
  otherwise `json:"<PropertyName>"` (STJ default policy = exact C# name). `[JsonIgnore]` →
  `json:"-"`. `[JsonIgnore(Condition = WhenWritingNull)]` → `json:"name,omitnull"`;
  `WhenWritingDefault` → `json:"name,omitdefault"`. Never add `omitempty` otherwise.
* Nullability: a C# reference-type property **without** a non-null initializer (e.g.
  `public string Foo { get; set; }`) can be null and serializes as `null` → use `*string`,
  `*T`, nil slice/map. With an initializer (`= ""`, `= new()`) use the value type and set it
  in the constructor. Nullable value types (`int?`, `bool?`, `DateTime?`) → pointers.
* Every model gets a constructor `func NewX() *X` that applies the C# property initializers
  (e.g. `Config: map[string]string{}`, `Name: ""`). Stored models pass it to
  `docdb.GetCollection`.
* Stored models (deriving from `AstraDbObject`) embed `docdb.AstraDbObject` **as the first field**
  (it provides `Id`, JSON-ignored, stored as `_id`). Storage member names are the Go field names
  (= C# property names); override with `bson:"Name"` only if a Go field name must differ.
  `[JsonIgnore]` fields are still stored (LiteDB ignores JsonIgnore).
* Inheritance between models: embed the base struct (untagged) — the STJ encoder emits a type's
  own fields first, then inherited ones, matching .NET reflection order. Verify with the oracle
  if order matters.
* Getter-only computed properties that STJ serializes → implement as a field filled before
  serialization, or a custom `MarshalSTJ`. Read-only (getter-only) properties are not set on
  deserialize: tag `json:"name,readonly"`.

## 3. JSON (package `solara/internal/stj`)

* `stj.Marshal/MustMarshal/MarshalString` == `JsonSerializer.Serialize(x)` with default options
  (byte-exact: escaping, number formatting, DateTime formats). `hosting.Ok(x)` serializes for you.
* C# anonymous objects `new { a = 1, b = "x" }` → `stj.O("a", 1, "b", "x")` (ordered!). Never use
  a Go map where the original produced an anonymous object or a struct (order matters for byte
  equality). `Dictionary<string, X>` built by insertion → `stj.Obj` with `.Set(k, v)`.
* `JsonSerializer.Deserialize<T>(bytes[, opts])` → `stj.Unmarshal(bytes, &t[, stj.Options{CaseInsensitive: true}])`
  (case-sensitive by default; null into value types errors; unknown props ignored).
  A thrown JsonException → `stj.MustUnmarshal` (panics) **only** where the C# did not catch it.
* `JsonDocument.Parse(x).RootElement` → `stj.MustParse(x)` / `stj.Parse(x)`; `JsonElement` API
  → `stj.Element` (`GetProperty`, `TryGetProperty`, `GetString` (returns *string), `Str()`,
  `GetInt32`, `TryGetInt32`, `GetBoolean`, `EnumerateArray`, `EnumerateObject`, `ValueKind`,
  `GetRawText`, `ToString`). Methods panic with the same .NET exception types as the original.
* Custom `JsonSerializerOptions` (e.g. `PropertyNamingPolicy = CamelCase`, `WriteIndented`,
  `DefaultIgnoreCondition`) are **not** supported by the encoder — if the original uses them,
  produce the equivalent output explicitly (e.g. a struct with camelCase tags, or `stj.O(...)`),
  and note it in your report.

## 4. HTTP (package `solara/internal/hosting`)

* A server: `s := hosting.NewServer("MothershipServer", HOSTNAME, PORT)` then register every
  endpoint **in exactly the order of `docs/original_routes.tsv`** for that server:
  `s.Handle("GET", "<uri as in the attribute>", "<C# method name>", handler)`. `Start()` binds.
  Provide `func New<Server>() (*<Server>, error)` that builds + starts (constructor semantics:
  the original started listening in its constructor; return the bind error).
* Handler: `func(c *hosting.Context) hosting.Result`. Return `hosting.Ok(obj)`,
  `hosting.BadRequest("text")`, `hosting.NotFound(nil)`, `hosting.Unauthorized(obj)`,
  `hosting.Configurable(code, ctype, content)`, `hosting.HtmlDocument(code, html)`, etc. — the
  same factory the C# called (content-type rules are reproduced: nil → text/plain empty body,
  string/number/bool → text/plain `ToString()`, `[]byte` → octet-stream, else JSON).
  `Results.NoContent()` → `hosting.NoContent()`.
* Route params: `c.Param("name")` (raw). Original then did `Convert.ChangeType(value, paramType)`:
  string → as is; int → `c.RouteInt("name")` (FormatException → 500 page).
* Parameter binding attributes: `[FromQuery] string x` → `c.QueryString("x")` ("" when missing),
  `[FromQuery] int n` → `c.QueryInt("n")` (0 when missing/invalid), `[FromHeader]` →
  `c.HeaderString("x")`, `[FromBody] string x` → `c.BodyString("x")`; complex `[FromBody] T`
  → BindComplexTypeFromBody semantics (instance of T, for each public property look up
  JsonBody[DataMember name or property name] and convert `value.ToString()` with the type
  converter — use `hosting.Bind*` helpers). The attribute's `Name` overrides the param name
  (empty Name ⇒ param name... note: the attributes default Name to "" and the binder uses
  `attr.Name ?? param.Name`, so an empty string name is used as-is — check each usage).
* Request: `c.Request.Body()`, `.JsonBody()` (an Object `stj.Element`; panics like the
  original), `.FormBody()`, `.Header(name)` (case-insensitive), `.GetHeaderValue(name, def)`,
  `.Query(key)`, `.Queries()`, `.GetQueryParameter(key, def)`, `.Uri()`, `.Remote()`,
  `.Method()`, `.Host()`, `.ContentType()`. Response: `c.Response.SetHeader`, `.SetCookie`, ...
* Exceptions: let unexpected failures panic (the hosting layer turns them into the original 500
  HTML page "Error while processing <MethodName>..."). Use `dotnet.Throw(type, msg)` to raise a
  specific .NET exception where the original threw one. Where the C# has try/catch, reproduce
  the catch behaviour exactly (use `defer/recover` or explicit error checks).
* Requests are processed one at a time per server (the hosting layer holds a mutex), exactly
  like the original listener loop, so handler code may rely on that.
* WebSocket endpoints (`HandleWebSocketAsync` overrides): set `s.WebSocket = func(w, r) bool`,
  upgrade with `github.com/gorilla/websocket` (`websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}`),
  and port the message loop. Subprotocol negotiation must match `AcceptWebSocketAsync(subProtocol)`.
* `HttpClient` calls → `net/http` with identical URLs, methods, headers, bodies, timeouts.
* `Task.Run`/fire-and-forget → goroutines; timers/background loops → goroutines with tickers;
  keep intervals identical. `Console.WriteLine(...)` → `logging.Console(...)` with identical text.
  Serilog `Logger.Information("tmpl {X}", x)` → `s.Logger.Information("tmpl {X}", x)`.

## 5. Persistence (package `solara/internal/docdb`)

* `database.GetCollection<T>(true)` → `docdb.GetCollection[models.T](db, true, models.NewT)`.
* `FindAll()` → `.FindAll()` ([]*T in _id order). `Find(pred)`, `FindOne(pred)`, `Count(pred)`,
  `Exists(pred)`, `DeleteMany(pred)`, `Insert`, `Update` (bool), `Upsert` (bool inserted),
  `Delete(id)`, `DeleteAll()`, `FindById(id)`.
* **CRITICAL collation rule**: predicates passed to LiteDB (`Find/FindOne/Count/Exists/DeleteMany`
  lambdas) are evaluated by LiteDB with its default collation, which compares strings
  **case-insensitively** (verified with the original runtime: `FindOne(x => x.Name == "alice")`
  finds "Alice"; `x.Tags.Contains("red")` matches "Red"; `StartsWith` too). Inside such
  predicates use `docdb.EqFold(a, b)`, `docdb.ContainsFold(list, x)`, `docdb.HasPrefixFold`.
  Predicates applied in memory after `FindAll()` (LINQ: `.FindAll().Where(...)`,
  `.FirstOrDefault(...)`) are ordinary C# **case-sensitive** comparisons — use `==` there.
  Numeric/bool comparisons are the same either way.
* LiteDB returns new objects each call; DateTime values come back with Kind=Local and
  millisecond precision — the codec already does this.

## 6. .NET semantics helpers (package `solara/internal/dotnet`)

`dotnet.DateTime` (`UtcNow()`, `Now()`, `AddMinutes`, `Sub`, `Before/After`, `FormatO()` for
`ToString("o")`, `Format("yyyy-MM-dd")`, `String()` for `ToString()`, `ToUnixSeconds()`…),
`dotnet.NewGuid()` (`Guid.NewGuid().ToString()`), `dotnet.NewGuidN()` (`ToString("N")`),
`dotnet.CreateRandomString`, `dotnet.RandomNext(min,max)`, `dotnet.FormatDouble`,
`dotnet.FormatBool` (`"True"/"False"`), `dotnet.Throw`. String interpolation of a bool in C#
gives `True/False`; of a double uses `FormatDouble`; of a DateTime uses `.String()`.
`string.Equals(a,b,OrdinalIgnoreCase)` → `strings.EqualFold`. `StringComparison.Ordinal` → `==`.

## 7. Secrets

Hard-coded secrets in the original (API keys, JWT secrets, Meta app secret, DB password) must
NOT be copied into Go source, tests, docs or logs. Read them from `secrets.Get("NAME")`
(package `solara/internal/secrets`; it loads `config/legacy-secrets.env`, generated from the
original source and git-ignored, then environment overrides). Use the exact C# constant name
as NAME. The configured names are exactly: `JWT_SECRET`, `META_APP_SECRET`, `AUTOMATION_KEY`
(MothershipServer), `SERVER_API_KEY` (the legacy literal fallback inside
`MothershipServer.LoadServerApiKey`; SpecLevelsStore has its own LoadServerApiKey — use the same
name for its literal fallback if it has one), `ServerMasterKey`, `QUEST_APP_SECRET`
(A2StationDbServer), `TURN_SECRET` (EosGatewayServer), `RTC_API_KEY`/`RTC_API_SECRET`
(RtcHelper `API_KEY`/`API_SECRET`), `PRODUCTION_DB_PASSWORD` (Program.cs). Aliases such as
`RIFT_APP_SECRET = META_APP_SECRET` stay aliases. Where the original
already read an env var first (e.g. `HALCYON_SERVER_API_KEY`), keep that precedence and only
use `secrets.Get` for the baked fallback. Non-secret constants (ports, ids, URLs) stay in code.

## 8. Fidelity rules

* Port every endpoint body, branch, fallback, log line and quirk. No stubs, no TODOs, no
  "simplified" logic. Do not fix bugs; if you see one, keep it and list it in your report.
* Keep C# comments that explain behaviour (shortened is fine) so reviewers can map code.
* Each Go file starts with a comment naming the C# file(s) it ports.
* Write `_test.go` tests for: route table equality with `docs/original_routes.tsv` for your
  server(s), every model's JSON shape (field names/order/nullability), and representative
  handler behaviour using `httptest` against the server's `hosting.Server` (call
  `s.ServeHTTP(rec, req)` directly; no real ports needed).
* Run `gofmt -w`, `go vet ./...` and `go test ./internal/<yourpkg>/...` before finishing.
