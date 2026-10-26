// Port of AUnrealFeatures.EOSSDK/EosJwtFactory.cs.
//
// Generates self-signed RS256 JWTs that match the EOS token structure.
// The EOS SDK validates format and expiry; signature verification against
// EOS's real public key is skipped for local private-server use.
package eos

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"strings"
	"sync"
	"time"

	"solara/internal/dotnet"
	"solara/internal/stj"
)

const jwtKID = "2022-06-14T06:17:57.047928700Z"

// One RSA key pair per process lifetime (regenerated on restart — fine for local use)
var (
	rsaOnce sync.Once
	rsaKey  *rsa.PrivateKey
)

func jwtRsa() *rsa.PrivateKey {
	rsaOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		rsaKey = k
	})
	return rsaKey
}

// Features mirrors EosJwtFactory.Features.
var Features = []string{
	"Achievements", "AntiCheat", "Connect", "Leaderboards", "Lobbies",
	"Matchmaking", "Metrics", "Notifications", "PlayerDataStorage",
	"PlayerReports", "ProgressionSnapshot", "Sanctions", "Stats",
	"TitleStorage", "Voice",
}

// ── Public factory methods ────────────────────────────────────────────────

// UserToken — returned for grant_type=external_auth. The C# optional
// parameters default to idp "oculus_app_id", platformId "0", platformType "other".
func UserToken(clientId, productId, sandboxId, deploymentId, orgId, productUserId, orgUserId,
	nonce, displayName, clientIp, idp, platformId, platformType string) string {
	iat := time.Now().Unix()
	exp := iat + 3600*24*365*10 // 10 years

	account := stj.O("idp", idp, "displayName", displayName, "id", platformId, "plf", platformType)
	if idp == "oculus_app_id" {
		account.Set("oculusOrgScopedId", "0")
	}

	payload := stj.O(
		"clientId", clientId,
		"productId", productId,
		"iss", "eos",
		"env", "prod",
		"nonce", nonce,
		"organizationId", orgId,
		"features", Features,
		"productUserId", productUserId,
		"organizationUserId", orgUserId,
		"clientIp", clientIp,
		"deploymentId", deploymentId,
		"sandboxId", sandboxId,
		"tokenType", "userToken",
		"exp", exp,
		"iat", iat,
		"account", account,
		"jti", dotnet.NewGuidN(),
	)
	return jwtSign(payload)
}

// ClientToken — returned for grant_type=client_credentials.
func ClientToken(clientId, productId, sandboxId, deploymentId, orgId string) string {
	iat := time.Now().Unix()
	exp := iat + 3600 // 1 hour, matching real EOS

	payload := stj.O(
		"clientId", clientId,
		"productId", productId,
		"iss", "eos",
		"env", "prod",
		"organizationId", orgId,
		"features", Features,
		"deploymentId", deploymentId,
		"sandboxId", sandboxId,
		"tokenType", "clientToken",
		"exp", exp,
		"iat", iat,
		"jti", dotnet.NewGuidN(),
	)
	return jwtSign(payload)
}

// ── ID token (Connect) ────────────────────────────────────────────────────

// IdToken — included in the user token response alongside access_token.
// Optional parameters default to idp "oculus_app_id", platformId "0", platformType "other".
func IdToken(clientId, productId, sandboxId, deploymentId, productUserId, idp, platformId, platformType string) string {
	iat := time.Now().Unix()
	exp := iat + 3600*24*365*10

	payload := stj.O(
		"aud", clientId,
		"sub", productUserId,
		"pfsid", sandboxId,
		"act", stj.O("pltfm", platformType, "eaid", platformId, "eat", idp),
		"pfdid", deploymentId,
		"iss", "https://api.epicgames.dev/auth/v1/oauth",
		"exp", exp,
		"tokenType", "idToken",
		"iat", iat,
		"pfpid", productId,
		"jti", dotnet.NewGuidN(),
	)
	return jwtSign(payload)
}

// ── Helpers ───────────────────────────────────────────────────────────────

const jwtHeaderJSON = `{"kid":"` + jwtKID + `","typ":"JWT","alg":"RS256"}`

func jwtSign(payload stj.Obj) string {
	payloadJSON := stj.MustMarshal(payload)

	header64 := b64url([]byte(jwtHeaderJSON))
	payload64 := b64url(payloadJSON)
	message := header64 + "." + payload64

	h := sha256.Sum256([]byte(message))
	sig, err := rsa.SignPKCS1v15(nil, jwtRsa(), crypto.SHA256, h[:])
	if err != nil {
		panic(err)
	}
	return message + "." + b64url(sig)
}

// b64url mirrors Convert.ToBase64String(b).TrimEnd('=').Replace('+','-').Replace('/','_').
func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// ── JWKS ─────────────────────────────────────────────────────────────────

// GetJwks mirrors EosJwtFactory.GetJwks (public key of the process key pair).
func GetJwks() any {
	k := jwtRsa()
	n := k.N.FillBytes(make([]byte, (k.N.BitLen()+7)/8))
	e := big.NewInt(int64(k.E)).Bytes()
	return stj.O("keys", []stj.Obj{stj.O(
		"kty", "RSA",
		"use", "sig",
		"alg", "RS256",
		"kid", jwtKID,
		"n", b64url(n),
		"e", b64url(e),
	)})
}

// jwtPublicKey exposes the verification key (tests).
func jwtPublicKey() *rsa.PublicKey { return &jwtRsa().PublicKey }

// ── Deterministic product user ID from display name ───────────────────────

// ProductUserIdFor generates a stable product_user_id for a given display name:
// SHA-1 of "puid:"+name, formatted like real EOS IDs (0002... prefix).
func ProductUserIdFor(displayName string) string {
	h := sha1.Sum([]byte("puid:" + displayName))
	return "0002" + strings.ToLower(hex.EncodeToString(h[:]))[:28]
}

// OrgUserIdFor mirrors EosJwtFactory.OrgUserIdFor.
func OrgUserIdFor(displayName string) string {
	h := sha1.Sum([]byte("ouid:" + displayName))
	return "0001" + strings.ToLower(hex.EncodeToString(h[:]))[:28]
}

// ExtractClientId extracts the client_id from the Authorization: Basic ...
// header. Returns the fallback value if the header is missing or malformed.
func ExtractClientId(authHeader string, fallback string) string {
	if authHeader == "" || !hasPrefixFold(authHeader, "Basic ") {
		return fallback
	}
	b, err := fromBase64String(authHeader[6:])
	if err != nil {
		return fallback
	}
	decoded := utf8GetString(b)
	return strings.Split(decoded, ":")[0]
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// fromBase64String mirrors Convert.FromBase64String: whitespace (space, tab,
// CR, LF) is ignored and padding is required.
func fromBase64String(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		}
		return r
	}, s)
	return base64.StdEncoding.DecodeString(s)
}
