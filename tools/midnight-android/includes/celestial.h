#pragma once
#include <string>

// Celestial (client half of Solara's Quest anticheat).
//
// On load it hashes the installed OBB once, then asks Mothership for a short-lived "trusted" token
// (POST /v1/celestial/token; the server half is internal/mothership/celestial.go). The curl URL
// hook appends that token to the Quest login request so the server lets the login through.
//
// It only hashes a file and makes an HTTPS request — it does not inspect or modify the game process.
namespace Celestial {
    // Start the background fetch/refresh. Called from JNI_OnLoad (needs the JavaVM).
    void Start();

    // The current token, waiting up to timeoutMs for the first fetch. "" if none yet.
    std::string Token(int timeoutMs);

    // True for the login path the server gates (so the hook knows to append the token).
    bool NeedsToken(const std::string& pathAndQuery);
}
