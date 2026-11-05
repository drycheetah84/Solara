// Ports the private helpers of AUnrealFeatures.AAMothership/MothershipServer.cs:
// NewId, Expiration*, CtEquals, IsTrustedServer/IsAdmin/IsClient/IsAuthorized,
// ParseBody/Str/Int/Bool/StrList, GenerateJwt, GetUserIdFromToken, Base64Url,
// DecodeBase64UrlToString, Json/JsonAnon/JsonError and GenerateSessionJwt.
package mothership

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/stj"
)

// col mirrors Database.GetCollection<T>(true)!.
func col[T any](newT func() *T) *docdb.Collection[T] {
	return docdb.GetCollection[T](Database, true, newT)
}

// newId mirrors NewId(): Guid.NewGuid().ToString().
func newId() string { return dotnet.NewGuid() }

// expirationSec / expirationMs mirror ExpirationSec / ExpirationMs (unused in
// the original as well).
func expirationSec(hours int) int64 {
	return time.Now().UTC().Add(time.Duration(hours) * time.Hour).Unix()
}
func expirationMs(hours int) int64 {
	return time.Now().UTC().Add(time.Duration(hours) * time.Hour).UnixMilli()
}

func utcNowO() string { return dotnet.UtcNow().FormatO() }

// fixedTimeEquals mirrors CryptographicOperations.FixedTimeEquals on UTF-8 bytes.
func fixedTimeEquals(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// ctEquals mirrors CtEquals(a, b): a must be non-empty.
func ctEquals(a string, present bool, b string) bool {
	return present && a != "" && fixedTimeEquals(a, b)
}

// isTrustedServer mirrors IsTrustedServer: the real SERVER_API_KEY in x-server-api-key.
// Solara: a valid per-server token also qualifies; the static key is subject
// to the strict/compat policy (serverid_auth.go).
func isTrustedServer(req *hosting.Request) bool {
	if serverTokenFromRequest(req) != nil {
		return true
	}
	k, _ := req.Header("x-server-api-key")
	if k == "" || strings.HasPrefix(SERVER_API_KEY, "<FILL_ME") {
		return false
	}
	return fixedTimeEquals(k, SERVER_API_KEY) && staticServerKeyOK(req)
}

// isAdmin mirrors IsAdmin: the real server key or automation key.
// Solara: a valid per-server token stands in for the server key.
func isAdmin(req *hosting.Request) bool {
	if serverTokenFromRequest(req) != nil {
		return true
	}
	sk, sok := req.Header("x-server-api-key")
	if ctEquals(sk, sok, SERVER_API_KEY) && staticServerKeyOK(req) {
		return true
	}
	ak, aok := req.Header("x-automation-key")
	return ctEquals(ak, aok, automationKey())
}

// isClient mirrors IsClient: a valid, signature-checked player JWT.
func isClient(req *hosting.Request) bool { return getUserIdFromToken(req) != "" }

// isAuthorized mirrors IsAuthorized.
func isAuthorized(req *hosting.Request) bool { return isAdmin(req) || isClient(req) }

// jsonBody mirrors the Dictionary<string, JsonElement> ParseBody returns:
// keys in first-insertion order, last duplicate value wins.
type jsonBody struct {
	keys []string
	vals map[string]stj.Element
}

// parseBody mirrors ParseBody: Deserialize<Dictionary<string, JsonElement>>(Body)
// ?? new(); any failure yields an empty dictionary.
func parseBody(req *hosting.Request) jsonBody {
	b := jsonBody{vals: map[string]stj.Element{}}
	el, err := stj.Parse(req.Body())
	if err != nil || el.ValueKind() != stj.Object {
		return b
	}
	for _, p := range el.EnumerateObject() {
		if _, ok := b.vals[p.Name]; !ok {
			b.keys = append(b.keys, p.Name)
		}
		b.vals[p.Name] = p.Value
	}
	return b
}

func (b jsonBody) TryGetValue(key string) (stj.Element, bool) {
	v, ok := b.vals[key]
	return v, ok
}

// Keys mirrors body.Keys.
func (b jsonBody) Keys() []string { return b.keys }

// MarshalSTJ mirrors serializing the Dictionary<string, JsonElement>: every
// element is re-written (JsonElement.WriteTo), not copied verbatim.
func (b jsonBody) MarshalSTJ() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range b.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		stj.WriteString(&buf, k)
		buf.WriteByte(':')
		writeElement(&buf, b.vals[k], false, 0)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// str mirrors Str(b, key).
func str(b jsonBody, key string) string {
	if e, ok := b.vals[key]; ok && e.ValueKind() == stj.String {
		return e.Str()
	}
	return ""
}

// intOr mirrors Int(b, key, def): GetInt32 throws FormatException for a
// non-Int32 number (uncaught in the original -> 500).
func intOr(b jsonBody, key string, def int) int {
	if e, ok := b.vals[key]; ok && e.ValueKind() == stj.Number {
		return int(e.GetInt32())
	}
	return def
}

// boolOr mirrors Bool(b, key, def).
func boolOr(b jsonBody, key string, def bool) bool {
	e, ok := b.vals[key]
	if !ok {
		return def
	}
	switch e.ValueKind() {
	case stj.True:
		return true
	case stj.False:
		return false
	}
	return def
}

// strList mirrors StrList(b, key).
func strList(b jsonBody, key string) []string {
	out := []string{}
	e, ok := b.vals[key]
	if !ok || e.ValueKind() != stj.Array {
		return out
	}
	for _, x := range e.EnumerateArray() {
		if x.ValueKind() == stj.String {
			out = append(out, x.Str())
		}
	}
	return out
}

// base64Url mirrors Base64Url: base64, '=' trimmed, URL alphabet.
func base64Url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// fromBase64String mirrors Convert.FromBase64String (whitespace ignored;
// FormatException on invalid input).
func fromBase64String(s string) []byte {
	clean := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		}
		return r
	}, s)
	b, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		dotnet.Throw("System.FormatException", "The input is not a valid Base-64 string as it contains a non-base 64 character, more than two padding characters, or an illegal character among the padding characters.")
	}
	return b
}

// padBase64Url converts base64url to base64 and pads as the original did
// (only lengths %4 == 2 or 3 are padded).
func padBase64Url(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "-", "+"), "_", "/")
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return s
}

// utf8String mirrors Encoding.UTF8.GetString: invalid bytes become U+FFFD.
func utf8String(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	for len(b) > 0 {
		r, n := utf8.DecodeRune(b)
		if r == utf8.RuneError && n <= 1 {
			sb.WriteRune(utf8.RuneError)
			b = b[1:]
			continue
		}
		sb.Write(b[:n])
		b = b[n:]
	}
	return sb.String()
}

// decodeBase64UrlToString mirrors DecodeBase64UrlToString.
func decodeBase64UrlToString(input string) string {
	return utf8String(fromBase64String(padBase64Url(input)))
}

func hmacBase64Url(signing string) string {
	h := hmac.New(sha256.New, []byte(jwtSecret()))
	h.Write([]byte(signing))
	return base64Url(h.Sum(nil))
}

// generateJwt mirrors GenerateJwt(object payload): HS256 over
// base64url({"alg":"HS256","typ":"JWT"}) + "." + base64url(payload JSON).
func generateJwt(payload any) string {
	header := base64Url(stj.MustMarshal(stj.O("alg", "HS256", "typ", "JWT")))
	body := base64Url(stj.MustMarshal(payload))
	signing := header + "." + body
	return signing + "." + hmacBase64Url(signing)
}

// getUserIdFromToken mirrors GetUserIdFromToken: validates the HS256
// signature of x-mothership-token, honours "exp", returns "sub" or "".
func getUserIdFromToken(req *hosting.Request) (sub string) {
	token, _ := req.Header("x-mothership-token")
	if token == "" {
		return ""
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	expected := hmacBase64Url(parts[0] + "." + parts[1])
	if !fixedTimeEquals(expected, parts[2]) {
		return ""
	}
	defer func() {
		if recover() != nil {
			sub = "" // catch { return ""; }
		}
	}()
	raw := fromBase64String(padBase64Url(parts[1]))
	root, err := stj.Parse(raw)
	if err != nil {
		return ""
	}
	if exp, ok := root.TryGetProperty("exp"); ok && exp.GetInt64() < time.Now().Unix() {
		return ""
	}
	if s, ok := root.TryGetProperty("sub"); ok {
		if p := s.GetString(); p != nil {
			// Solara: a globally banned player's tokens stop working (our own servers are exempt; globalban.go)
			if !isTrustedServer(req) && activeBan(*p) != nil {
				return ""
			}
			return *p
		}
	}
	return ""
}

// nullable renders a C# string? (nil -> null).
func nullable(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// generateSessionJwt mirrors GenerateSessionJwt(playerId, externalService, externalServiceId).
func generateSessionJwt(playerId string, externalService, externalServiceId *string) string {
	now := time.Now()
	expSec := now.Add(24 * time.Hour).Unix()
	return generateJwt(stj.O(
		"sub", playerId,
		"did", newId(),
		"env", ENVIRONMENT_ID,
		"externalService", nullable(externalService),
		"externalServiceId", nullable(externalServiceId),
		"tid", TENANT_ID,
		"tags", nil,
		"orgScopedExternalServiceId", nullable(externalServiceId),
		"nbf", now.Unix(),
		"exp", expSec,
		"iat", now.Unix(),
	))
}

// jsonT mirrors Json<T>(obj, code) and jsonAnon mirrors JsonAnon(obj, code):
// Results.Configurable(code, "application/json", SerializeToUtf8Bytes(obj)).
func jsonT(obj any, code int) hosting.Result {
	return hosting.Configurable(code, "application/json", stj.MustMarshal(obj))
}

func jsonAnon(obj any, code int) hosting.Result { return jsonT(obj, code) }

// jsonError mirrors JsonError(code, name, message, details, keyName).
func jsonError(code int, name, message, details string, keyName *string) hosting.Result {
	kn := ""
	if keyName != nil {
		kn = *keyName
	}
	return jsonAnon(stj.O(
		"statusCode", code,
		"name", name,
		"message", message,
		"details", details,
		"key_name", kn,
		"error", message,
	), code)
}

// unauthorized is the repeated `JsonAnon(new { error = "Unauthorized", code = 401 }, 401)`.
func unauthorized() hosting.Result {
	return jsonAnon(stj.O("error", "Unauthorized", "code", 401), 401)
}

// errorResult is the repeated `JsonAnon(new { error = msg }, code)`.
func errorResult(msg string, code int) hosting.Result {
	return jsonAnon(stj.O("error", msg), code)
}

// utf16Len mirrors string.Length.
func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// leftUTF16 mirrors s[..n] on a C# string (n <= Length).
func leftUTF16(s string, n int) string {
	u := utf16.Encode([]rune(s))
	if n > len(u) {
		n = len(u)
	}
	return string(utf16.Decode(u[:n]))
}

// strp returns a pointer to s.
func strp(s string) *string { return &s }
