// Port of AUnrealFeatures.EOSSDK/RtcHelper.cs (LiveKit-style HS256 room tokens).
package eos

import (
	"crypto/hmac"
	"crypto/sha256"
	"time"

	"solara/internal/secrets"
	"solara/internal/stj"
)

// SERVER_URL mirrors RtcHelper.SERVER_URL.
const SERVER_URL = "ws://94.72.120.104:7880"

// rtcApiKey / rtcApiSecret mirror the API_KEY / API_SECRET constants (secrets).
func rtcApiKey() string    { return secrets.Get("RTC_API_KEY") }
func rtcApiSecret() string { return secrets.Get("RTC_API_SECRET") }

// RoomName mirrors RtcHelper.RoomName.
func RoomName(lobbyId string) string { return "lobby-" + lobbyId }

// MakeToken mirrors RtcHelper.MakeToken.
func MakeToken(roomName string, puid string) string {
	header := b64url([]byte(`{"alg":"HS256","typ":"JWT"}`))
	now := time.Now()
	exp := now.Add(6 * time.Hour).Unix()
	nbf := now.Add(-1 * time.Second).Unix()
	payload := b64url(stj.MustMarshal(stj.O(
		"iss", rtcApiKey(),
		"sub", puid,
		"jti", puid,
		"exp", exp,
		"nbf", nbf,
		"video", stj.O("room", roomName, "roomJoin", true, "canPublish", true, "canSubscribe", true),
	)))
	sig := b64url(hmacSha256Bytes(header+"."+payload, rtcApiSecret()))
	return header + "." + payload + "." + sig
}

func hmacSha256Bytes(data, key string) []byte {
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(data))
	return m.Sum(nil)
}
