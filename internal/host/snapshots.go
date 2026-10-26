package host

import (
	"os"
	"path/filepath"

	"solara/internal/docdb"
	"solara/internal/eos"
	"solara/internal/logging"
	"solara/internal/secrets"
	"solara/internal/snapshot"
)

// StartSnapshotScheduler starts the automatic database snapshots configured
// by SOLARA_SNAPSHOT_* (docs/SNAPSHOTS.md): one "startup" snapshot, then one
// every SOLARA_SNAPSHOT_INTERVAL_HOURS, pruned to SOLARA_SNAPSHOT_KEEP. EOS
// sessions.json / lobbies.json are captured from eos.BaseDirectory. The
// returned stop function waits for an in-progress snapshot; call it before
// the database is closed. Errors are logged, never fatal.
func StartSnapshotScheduler(db *docdb.Database) (stop func()) {
	if db == nil {
		return func() {}
	}
	cfg := snapshot.ConfigFromEnv(logging.New("Snapshots"))
	cfg.EOSDir = eos.BaseDirectory
	s := snapshot.StartScheduler(db, cfg)
	return s.Stop
}

// DatabasePath returns the store file a configuration selects
// ("<working dir>/production.solara.db" for Release), as the builder in
// newManager resolves it.
func DatabasePath(config string) (string, error) {
	name, _, err := dbName(config)
	if err != nil {
		return "", err
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, name+docdb.FileSuffix), nil
}

// DatabasePasswordFor returns the password of a database by name: the
// PRODUCTION_DB_PASSWORD secret for "production" (nil when unset), nil for
// "development" (unencrypted).
func DatabasePasswordFor(name string) *string {
	if name != "production" {
		return nil
	}
	p := secrets.Get("PRODUCTION_DB_PASSWORD")
	if p == "" {
		return nil
	}
	return &p
}
