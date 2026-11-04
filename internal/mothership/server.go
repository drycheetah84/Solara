// Ports AUnrealFeatures.AAMothership/MothershipServer.cs: class constants,
// static state, the static constructor (INSECURE_TESTING banner),
// LoadServerApiKey, the constructor and the endpoint table (reflection order,
// see docs/original_routes.tsv).
package mothership

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/logging"
	"solara/internal/secrets"
)

const (
	HOSTNAME       = "localhost"
	PORT           = 90
	ENVIRONMENT_ID = "7f3a99dd-5598-4725-98cf-6538d28feb9f"
	TENANT_ID      = "f3e9fb19"

	// Halcyon Meta app: used to verify Quest attestation + user-proof nonces
	// against Meta's graph API. All server-to-server requests use this app;
	// the RIFT_* names are aliases of the META_* values.
	META_APP_ID         = "2236834297114138"
	RIFT_APP_ID         = META_APP_ID
	RIFT_FED_APP_ID     = META_APP_ID
	NONCE_CACHE_TTL_SEC = int64(12 * 3600)
	QUEST_BAN_MINUTES   = 7 * 24 * 60 // 7 days
)

// Secrets the original baked in as constants (never stored in Solara source).
func jwtSecret() string     { return secrets.Get("JWT_SECRET") }
func metaAppSecret() string { return secrets.Get("META_APP_SECRET") }
func automationKey() string { return secrets.Get("AUTOMATION_KEY") }

// RIFT_APP_SECRET / RIFT_FED_APP_SECRET are aliases of META_APP_SECRET.
func riftAppSecret() string    { return metaAppSecret() }
func riftFedAppSecret() string { return metaAppSecret() }

// Database mirrors MothershipServer.Database (set by the host before servers start).
var Database *docdb.Database

// Static readonly configuration, computed once by staticInit (the C# static
// field initializers + static constructor).
var (
	staticOnce sync.Once

	INSECURE_TESTING           bool
	QUEST_ENFORCE              bool
	RIFT_ENFORCE               bool
	QUEST_EXPECTED_PACKAGE     string
	QUEST_EXPECTED_CERT_SHA    string
	ALLOW_NONSTORE             bool
	QUEST_REQUIRE_STORE        bool
	QUEST_REQUIRE_DEVICE_BASIC bool
	QUEST_REQUIRE_PATCHED      bool
	QUEST_BAN_ON_TAMPER        bool
	SERVER_API_KEY             string
	serverKeySource            string
)

func envSwitch(name string) bool {
	switch os.Getenv(name) {
	case "1", "true", "TRUE", "yes":
		return true
	}
	return false
}

// staticInit runs the C# static initializers and static constructor once
// ("on first touch of the class").
func staticInit() {
	staticOnce.Do(func() {
		INSECURE_TESTING = envSwitch("MOTHERSHIP_INSECURE")
		QUEST_ENFORCE = !INSECURE_TESTING && true
		RIFT_ENFORCE = !INSECURE_TESTING && false
		if INSECURE_TESTING {
			QUEST_EXPECTED_PACKAGE, QUEST_EXPECTED_CERT_SHA = "", ""
		} else {
			QUEST_EXPECTED_PACKAGE = "com.AnotherSaxaphone.GayTwo"
			QUEST_EXPECTED_CERT_SHA = "db0b8c3dffbe2aa9ce36b5bdd83884052d26e685c2d372befabffbb2d88a846a"
		}
		ALLOW_NONSTORE = envSwitch("MOTHERSHIP_ALLOW_NONSTORE")
		QUEST_REQUIRE_STORE = !INSECURE_TESTING && !ALLOW_NONSTORE && true
		QUEST_REQUIRE_DEVICE_BASIC = !INSECURE_TESTING && true
		QUEST_REQUIRE_PATCHED = false
		QUEST_BAN_ON_TAMPER = !INSECURE_TESTING && true
		SERVER_API_KEY, serverKeySource = LoadServerApiKey()

		// static MothershipServer(): shouts if the auth bypass is on.
		if !INSECURE_TESTING {
			return
		}
		logging.Console("")
		logging.Console("  ##############################################################################")
		logging.Console("  ##  THIS MOTHERSHIP IS IN INSECURE TESTING MODE (MOTHERSHIP_INSECURE=1)     ##")
		logging.Console("  ##                                                                          ##")
		logging.Console("  ##  Meta attestation, StoreRecognized, device-integrity, package-id and     ##")
		logging.Console("  ##  signing-cert checks are ALL DISABLED. Any client can claim any          ##")
		logging.Console("  ##  identity. Use only on a LAN box you control. Never expose this.         ##")
		logging.Console("  ##############################################################################")
		logging.Console("")
	})
}

// baseDirOverride replaces AppContext.BaseDirectory (tests).
var baseDirOverride string

// baseDirectory mirrors AppContext.BaseDirectory (directory of the executable).
func baseDirectory() string {
	if baseDirOverride != "" {
		return baseDirOverride
	}
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// LoadServerApiKey mirrors MothershipServer.LoadServerApiKey(out source): env
// HALCYON_SERVER_API_KEY, then server_api_key.txt next to the executable, then
// the legacy baked fallback (secrets SERVER_API_KEY). Returns (key, source).
func LoadServerApiKey() (string, string) {
	if env := os.Getenv("HALCYON_SERVER_API_KEY"); strings.TrimSpace(env) != "" {
		return strings.TrimSpace(env), "env HALCYON_SERVER_API_KEY"
	}
	// try { ... } catch { fall through to the legacy literal }: I/O errors fall through.
	path := filepath.Join(baseDirectory(), "server_api_key.txt")
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		if raw, err := os.ReadFile(path); err == nil {
			k := strings.TrimSpace(strings.TrimPrefix(string(raw), "\xef\xbb\xbf"))
			if len(k) > 0 {
				return k, path
			}
		}
	}
	return secrets.Get("SERVER_API_KEY"), "LEGACY BAKED FALLBACK (set server_api_key.txt)"
}

// fileLog mirrors s_fileLog: this server's file logger, used by the static
// helpers once the constructor ran.
var fileLog atomic.Pointer[logging.Logger]

// consoleWriteLine mirrors Console.WriteLine(string) (text verbatim).
func consoleWriteLine(line string) { logging.Console(line) }

// authLog mirrors MothershipServer.AuthLog: console + file logger warning.
func authLog(line string) {
	logging.Console(line)
	if l := fileLog.Load(); l != nil {
		l.Warning("{AuthLine}", line)
	}
}

// MothershipServer mirrors the C# server class.
type MothershipServer struct {
	HTTP *hosting.Server
}

// NewMothershipServer mirrors `new MothershipServer()`: builds the endpoint
// table, starts listening on :90 (base constructor), then records the file
// logger and logs where the server key came from.
func NewMothershipServer() (*MothershipServer, error) {
	s := newMothershipServer()
	if err := s.HTTP.Start(); err != nil {
		return nil, err
	}
	s.afterStart()
	return s, nil
}

func newMothershipServer() *MothershipServer {
	staticInit()
	s := &MothershipServer{HTTP: hosting.NewServer("MothershipServer", HOSTNAME, PORT)}
	s.register()
	return s
}

func (s *MothershipServer) afterStart() {
	fileLog.Store(s.HTTP.Logger)
	authLog(fmt.Sprintf("[AUTH] server api key loaded from %s (len=%d)", serverKeySource, utf16Len(SERVER_API_KEY)))
	go backfillQuestRewards()
	go backfillPlayerNames()
}

// ---- active-session allowlist (Tier 1 join gate) ---------------------------

var (
	stateMu sync.Mutex
	// _pendingQuestAuth: userId -> issued attestation nonce.
	pendingQuestAuth = map[string]string{}
	// _usedQuestNonces: single-use guard (anti-replay).
	usedQuestNonces = map[string]byte{}
	// _bannedUniqueIds: unique_ids already device-banned this process.
	bannedUniqueIds = map[string]byte{}
	// _activeSessions: id -> expiry (unix seconds).
	activeSessions = map[string]int64{}
	// _nonceOkCache: userId -> expiry (unix seconds).
	nonceOkCache = map[string]int64{}
)

func tryAdd(m map[string]byte, k string) bool {
	stateMu.Lock()
	defer stateMu.Unlock()
	if _, ok := m[k]; ok {
		return false
	}
	m[k] = 1
	return true
}

// markActiveSession mirrors MarkActiveSession(hours, params ids).
func markActiveSession(hours int, ids ...string) {
	exp := time.Now().UTC().Add(time.Duration(hours) * time.Hour).Unix()
	stateMu.Lock()
	defer stateMu.Unlock()
	for _, id := range ids {
		if id != "" {
			activeSessions[id] = exp
		}
	}
}

// MarkDashboardSession mirrors MothershipServer.MarkDashboardSession(string? id):
// marks the org-scoped id the dashboard login hands the client as an active
// session (12h). C# null and "" are both ignored (string.IsNullOrEmpty), so
// the parameter is a plain string.
func MarkDashboardSession(id string) {
	staticInit()
	markActiveSession(12, id)
}

// isActiveSession mirrors IsActiveSession (lazily evicts expired entries).
func isActiveSession(id string) bool {
	if id == "" {
		return false
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	exp, ok := activeSessions[id]
	if !ok {
		return false
	}
	if exp >= time.Now().Unix() {
		return true
	}
	delete(activeSessions, id)
	return false
}

// playerPersisted mirrors PlayerPersisted: FindAll().Any(...) (in-memory LINQ,
// case-sensitive).
func playerPersisted(id string) bool {
	if id == "" {
		return false
	}
	col := v2Players()
	if col == nil {
		return false
	}
	for _, p := range col.FindAll() {
		if eqS(p.ExternalProviderId, id) || eqS(p.PlayerId, id) {
			return true
		}
	}
	return false
}
