# Compatibility matrix

Per-endpoint and per-model rows (with Go locations) are in MIGRATION_INVENTORY.md
(API-001…282, MODEL-*). This matrix maps each contract **class** to how it is preserved and
which test proves it.

| Contract | Original | Solara mechanism | Verification |
|---|---|---|---|
| Ports / bind addresses | 90, 78, 8080, 50, 80, 9095 (HTTP.sys `+`), 9100 TCP | same constants, all interfaces | e2e run (VALIDATION_REPORT §4) |
| Route set & precedence | reflection order, first match | registration in `docs/original_routes.tsv` order | per-server route-table tests (282/282) |
| Path handling | AbsolutePath, `//` collapsed, `{p}` = `[^/]+`, case-sensitive | `hosting.compileRoute`, `CollapseSlashes` | hosting tests; sweeps |
| Method mismatch | 404 (not 405) | same | hosting tests (HOST-001) |
| CORS / OPTIONS | `*` headers after preprocessors; OPTIONS → 204 | same order | hosting tests; live check |
| Error page | 500 HTML `HtmlDocumentCache` + exception | byte-identical asset + type/message | hosting tests; sweeps (stack trace normalised) |
| Content types | text/plain (null/primitive), application/json, octet-stream, text/html | same rules | hosting tests; sweeps |
| JSON field names/order/nullability | STJ defaults + `[JsonPropertyName]` | struct tags, field order, pointer nullability | oracle byte-equality per model |
| JSON escaping & numbers | `"`-style escapes, non-ASCII escaped, .NET double/float formats | `stj` encoder | `stj` oracle tests |
| Dates | ISO with trimmed 7-digit fraction, `Z`/offset/none by Kind | `dotnet.DateTime` | oracle tests; round-trips |
| Request JSON binding | case-sensitive, strict types, JsonElement values | `stj` decoder / `Element` | `stj` tests; sweeps |
| Query/form/header binding | NameValueCollection + TypeConverter semantics | `hosting` binding helpers | hosting tests |
| Auth: Mothership player JWT (HS256) | JWT_SECRET | same secret (externalised), same layout | JWT oracle + original-signed token verified in Go |
| Auth: server/automation/master keys | `x-server-api-key`, `x-automation-key`, `x-api-key` | same headers, same values | sweeps-authed |
| Auth: dashboard | admin key / `a2dash_session` HMAC JWT, `DASHBOARD_AUTH_ENFORCE` | same | preprocessor tests; live check |
| Auth: EOS tokens | RS256 per-process key, JWKS | same | sign/verify + JWKS shape tests |
| Meta attestation | graph.oculus.com calls, enforcement flags, bans | same requests/flags | fake-Meta tests (live API not exercised) |
| Agent TCP protocol (:9100) | newline JSON, message set, reaper 90 s/15 s | same | replay vs .NET oracle |
| WebSockets (EOS) | STOMP v12, lobby protocol | gorilla/websocket with .NET-compatible framing/close codes | end-to-end WS tests |
| Persistence model | LiteDB: names `_<md5>_<type>`, `_id` ObjectId order, nulls omitted, `""`→null, trim, ms/Local dates, IgnoreCase predicates | `docdb` codec + collection | round-trip oracles (47 models); import test |
| Existing data | LiteDB files | export + import tool | encrypted fixture test |
| Persistence sidecars | `.meta` BinaryWriter format | same format | docdb tests |
| On-disk stores | sessions/lobbies JSON, netvar dumps/overrides, spec levels, boards | same paths/formats | package tests (byte-equal outputs) |
| Configuration | env vars, `server_api_key.txt`, data files, compiled constants | same names/precedence; baked secrets externalised | docs/CONFIGURATION.md; check-secrets |
| Logging | Serilog console + daily files | same templates/paths | manual review |
| Startup/shutdown | DI construction order; no graceful stop | same order; graceful stop added (HOST-002) | e2e run |
