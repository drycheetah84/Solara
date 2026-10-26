// Ports the Meta graph integrations of AUnrealFeatures.AAMothership/MothershipServer.cs:
// AttestationResult, VerifyQuestAttestation, VerifyMetaUserNonce,
// VerifyMetaUserNonceWith and BanDevice, plus the shared NonceHttp client.
package mothership

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"solara/internal/hosting"
	"solara/internal/stj"
)

// graphBaseURL is https://graph.oculus.com (tests point it at an httptest server).
var graphBaseURL = "https://graph.oculus.com"

// dotnetTransport approximates the .NET 6 HttpClient defaults on the wire:
// HTTP/1.1 only, no automatic Accept-Encoding.
func dotnetTransport() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DisableCompression = true
	t.ForceAttemptHTTP2 = false
	t.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	return t
}

// metaTransport is shared by every graph call (tests may replace it).
var metaTransport = dotnetTransport()

// nonceHttp mirrors NonceHttp: one shared HttpClient with a 4s timeout.
var nonceHttp = &http.Client{Timeout: 4 * time.Second, Transport: metaTransport}

// newHttp8 mirrors `new HttpClient { Timeout = TimeSpan.FromSeconds(8) }`.
func newHttp8() *http.Client {
	return &http.Client{Timeout: 8 * time.Second, Transport: metaTransport}
}

// escapeDataString mirrors Uri.EscapeDataString (RFC 3986 unreserved kept, %XX upper-case).
func escapeDataString(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// formUrlEncode mirrors FormUrlEncodedContent (EscapeDataString with %20 -> '+').
func formUrlEncode(kv ...string) string {
	var sb strings.Builder
	for i := 0; i < len(kv); i += 2 {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(url.QueryEscape(kv[i]))
		sb.WriteByte('=')
		sb.WriteString(url.QueryEscape(kv[i+1]))
	}
	return sb.String()
}

// httpErrorMessage renders an HttpClient failure as its .NET exception message.
func httpErrorMessage(err error, timeoutSec int) string {
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return fmt.Sprintf("The request was canceled due to the configured HttpClient.Timeout of %d seconds elapsing.", timeoutSec)
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

// send performs a request and reads the whole body as a string
// (resp.Content.ReadAsStringAsync(); the status code is not checked).
func send(client *http.Client, req *http.Request, timeoutSec int) (string, error) {
	req.Header.Set("User-Agent", "") // HttpClient sends no User-Agent by default
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New(httpErrorMessage(err, timeoutSec))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.New(httpErrorMessage(err, timeoutSec))
	}
	return utf8String(body), nil
}

// attestationResult mirrors the private AttestationResult record.
type attestationResult struct {
	Ok                  bool
	Message             string
	Nonce               string
	Exp                 int64
	PackageId           string
	AppIntegrity        string
	DeviceIntegrity     string
	RawClaims           string
	Timestamp           int64
	UniqueId            string
	SecurityPendingDays int
	DeviceBanned        bool
	CertDigestsCsv      string
}

func failedAttestation(msg, raw string) attestationResult {
	return attestationResult{Ok: false, Message: msg, RawClaims: raw}
}

// strProp mirrors `x.TryGetProperty(name, out var v) ? v.GetString() ?? "" : ""`.
func strProp(el stj.Element, name string) string {
	if v, ok := el.TryGetProperty(name); ok {
		if p := v.GetString(); p != nil {
			return *p
		}
	}
	return ""
}

// verifyQuestAttestation mirrors VerifyQuestAttestation: verifies a Quest
// attestation token against graph.oculus.com/platform_integrity/verify and
// decodes its claims. Never panics.
func verifyQuestAttestation(attestationToken string) (res attestationResult) {
	defer func() {
		if p := recover(); p != nil {
			res = failedAttestation("exception: "+hosting.ExceptionMessage(p), "")
		}
	}()
	accessToken := "OC|" + META_APP_ID + "|" + metaAppSecret()
	u := graphBaseURL + "/platform_integrity/verify" +
		"?token=" + escapeDataString(attestationToken) +
		"&access_token=" + escapeDataString(accessToken)

	// NOTE: the server processes serially -- keep this short.
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return failedAttestation("exception: "+err.Error(), "")
	}
	text, err := send(newHttp8(), req, 8)
	if err != nil {
		return failedAttestation("exception: "+err.Error(), "")
	}

	doc, err := stj.Parse([]byte(text))
	if err != nil {
		return failedAttestation("exception: "+err.Error(), "")
	}
	data, ok := doc.TryGetProperty("data")
	if !ok || data.ValueKind() != stj.Array || data.GetArrayLength() == 0 {
		return failedAttestation("malformed response", text)
	}

	first := data.EnumerateArray()[0]
	message := strProp(first, "message")
	cl, hasClaims := stj.Element{}, false
	if message == "success" {
		cl, hasClaims = first.TryGetProperty("claims")
	}
	if message != "success" || !hasClaims {
		if message == "" {
			message = "no message"
		}
		return failedAttestation(message, text)
	}

	clStr := ""
	if p := cl.GetString(); p != nil {
		clStr = *p
	}
	claimsJson := decodeBase64UrlToString(clStr)
	root, err := stj.Parse([]byte(claimsJson))
	if err != nil {
		return failedAttestation("exception: "+err.Error(), "")
	}

	nonce, exp, ts := "", int64(0), int64(0)
	if rd, ok := root.TryGetProperty("request_details"); ok {
		nonce = strProp(rd, "nonce")
		if e, ok := rd.TryGetProperty("exp"); ok {
			exp = e.GetInt64()
		}
		if t, ok := rd.TryGetProperty("timestamp"); ok {
			ts = t.GetInt64()
		}
	}
	pkg, appI, certCsv := "", "", ""
	if app, ok := root.TryGetProperty("app_state"); ok {
		pkg = strProp(app, "package_id")
		appI = strProp(app, "app_integrity_state")
		if cd, ok := app.TryGetProperty("package_cert_sha256_digest"); ok && cd.ValueKind() == stj.Array {
			var parts []string
			for _, x := range cd.EnumerateArray() {
				if x.ValueKind() == stj.String {
					parts = append(parts, x.Str())
				}
			}
			certCsv = strings.Join(parts, ",")
		}
	}
	devI, uid, pending := "", "", 0
	if dev, ok := root.TryGetProperty("device_state"); ok {
		devI = strProp(dev, "device_integrity_state")
		uid = strProp(dev, "unique_id")
		if s, ok := dev.TryGetProperty("security_update_pending_days"); ok {
			pending = int(s.GetInt32())
		}
	}
	// device_ban section is present at root ONLY when the device is banned.
	banned := false
	if db2, ok := root.TryGetProperty("device_ban"); ok && db2.ValueKind() == stj.Object {
		ib, ok := db2.TryGetProperty("is_banned")
		banned = ok && (ib.ValueKind() == stj.True || (ib.ValueKind() == stj.String && ib.Str() == "true"))
	}

	return attestationResult{Ok: true, Message: "success", Nonce: nonce, Exp: exp, PackageId: pkg,
		AppIntegrity: appI, DeviceIntegrity: devI, RawClaims: claimsJson, Timestamp: ts, UniqueId: uid,
		SecurityPendingDays: pending, DeviceBanned: banned, CertDigestsCsv: certCsv}
}

// verifyMetaUserNonce mirrors VerifyMetaUserNonce: validates a Meta UserProof
// nonce for userId, caching a pass for NONCE_CACHE_TTL_SEC.
func verifyMetaUserNonce(userId, nonce string) (bool, string) {
	if userId == "" || nonce == "" {
		return false, "missing userId/nonce"
	}
	now := time.Now().Unix()
	stateMu.Lock()
	exp := nonceOkCache[userId]
	if exp > now {
		stateMu.Unlock()
		return true, "cache" // no I/O
	}
	if exp != 0 && exp <= now {
		delete(nonceOkCache, userId)
	}
	stateMu.Unlock()
	ok, raw := verifyMetaUserNonceWith("OC|"+META_APP_ID+"|"+metaAppSecret(), userId, nonce)
	if ok {
		stateMu.Lock()
		nonceOkCache[userId] = now + NONCE_CACHE_TTL_SEC
		stateMu.Unlock()
	}
	return ok, "app=meta " + raw
}

// verifyMetaUserNonceWith mirrors VerifyMetaUserNonceWith: POST
// graph.oculus.com/user_nonce_validate (form: access_token, nonce, user_id).
func verifyMetaUserNonceWith(accessToken, userId, nonce string) (ok bool, raw string) {
	defer func() {
		if p := recover(); p != nil {
			ok, raw = false, "exception: "+hosting.ExceptionMessage(p)
		}
	}()
	form := formUrlEncode("access_token", accessToken, "nonce", nonce, "user_id", userId)
	req, err := http.NewRequest(http.MethodPost, graphBaseURL+"/user_nonce_validate", strings.NewReader(form))
	if err != nil {
		return false, "exception: " + err.Error()
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	text, err := send(nonceHttp, req, 4)
	if err != nil {
		return false, "exception: " + err.Error()
	}
	doc, err := stj.Parse([]byte(text))
	if err != nil {
		return false, "exception: " + err.Error()
	}
	v, has := doc.TryGetProperty("is_valid")
	ok = has && (v.ValueKind() == stj.True || (v.ValueKind() == stj.String && strings.ToLower(v.Str()) == "true"))
	return ok, text
}

// banDevice mirrors BanDevice: fires a Meta device ban for uniqueId. The
// idempotency check runs synchronously (as the async method did up to its
// first await); the network call runs in the background (fire-and-forget).
func banDevice(uniqueId string, minutes int, reason string) {
	if uniqueId == "" {
		return
	}
	if !tryAdd(bannedUniqueIds, uniqueId) {
		return // already banned this session
	}
	go func() {
		defer func() {
			if p := recover(); p != nil {
				authLog(fmt.Sprintf("[QUEST-BAN] unique_id=%s error: %s", uniqueId, hosting.ExceptionMessage(p)))
			}
		}()
		accessToken := "OC|" + META_APP_ID + "|" + metaAppSecret()
		u := graphBaseURL + "/platform_integrity/device_ban?method=POST" +
			"&unique_id=" + escapeDataString(uniqueId) +
			fmt.Sprintf("&is_banned=true&remaining_time_in_minute=%d", minutes) +
			"&access_token=" + escapeDataString(accessToken)
		req, err := http.NewRequest(http.MethodPost, u, nil)
		if err != nil {
			authLog(fmt.Sprintf("[QUEST-BAN] unique_id=%s error: %s", uniqueId, err.Error()))
			return
		}
		text, err := send(newHttp8(), req, 8)
		if err != nil {
			authLog(fmt.Sprintf("[QUEST-BAN] unique_id=%s error: %s", uniqueId, err.Error()))
			return
		}
		authLog(fmt.Sprintf("[QUEST-BAN] unique_id=%s minutes=%d reason='%s' -> %s", uniqueId, minutes, reason, text))
	}()
}
