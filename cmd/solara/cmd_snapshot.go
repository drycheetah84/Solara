package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"solara/internal/docdb"
	"solara/internal/eos"
	"solara/internal/host"
	"solara/internal/snapshot"
)

func init() { subcommands["snapshot"] = snapshotCmd }

const snapshotUsage = `usage:
  solara snapshot create  [--label L] [--reason R] [--config Release|Debug] [--dir D] [--no-eos]
  solara snapshot list    [--dir D] [--verify]
  solara snapshot verify  <name|latest> [--dir D]
  solara snapshot restore <name|latest> [--config Release|Debug] [--include-eos] [--yes] [--dir D]
  solara snapshot prune   [--keep N] [--force] [--dir D]

Snapshots live in $SOLARA_SNAPSHOT_DIR (default ./snapshots). While Solara is
running it holds the database file lock, so live snapshots come from the
server's scheduler; create/restore from the CLI need the service stopped.`

func snapshotCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, snapshotUsage)
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "create":
		return snapshotCreate(rest)
	case "list":
		return snapshotList(rest)
	case "verify":
		return snapshotVerify(rest)
	case "restore":
		return snapshotRestore(rest)
	case "prune":
		return snapshotPrune(rest)
	case "help", "-h", "--help":
		fmt.Println(snapshotUsage)
		return 0
	}
	fmt.Fprintf(os.Stderr, "unknown snapshot command %q\n%s\n", sub, snapshotUsage)
	return 2
}

// splitPositional lets the snapshot name appear before or after the flags.
func splitPositional(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

func snapshotCreate(args []string) int {
	fs := flag.NewFlagSet("snapshot create", flag.ExitOnError)
	label := fs.String("label", "manual", "snapshot label (keep / pre-restore are protected from prune)")
	reason := fs.String("reason", "manual snapshot (CLI)", "free-text reason stored in the manifest")
	config := fs.String("config", host.DefaultConfiguration, "Release (production) or Debug (development)")
	dir := fs.String("dir", snapshot.DefaultDir(), "snapshots directory")
	noEOS := fs.Bool("no-eos", false, "do not capture sessions.json / lobbies.json")
	_ = fs.Parse(args)

	path, err := host.DatabasePath(*config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	switch err := docdb.ProbeUnlocked(path, 2*time.Second); {
	case errors.Is(err, docdb.ErrDatabaseInUse):
		fmt.Fprintf(os.Stderr, "%s is locked by a running Solara server.\n"+
			"bbolt allows one process per database file, so the CLI cannot read it now.\n"+
			"Live snapshots are taken by the server's scheduler (SOLARA_SNAPSHOT_INTERVAL_HOURS);\n"+
			"stop the service to take an offline snapshot from the CLI.\n", path)
		return 1
	case errors.Is(err, os.ErrNotExist):
		fmt.Fprintf(os.Stderr, "database %s does not exist\n", path)
		return 1
	case err != nil:
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// Opening through the host validates the password exactly like serve.
	db, closeFn, err := host.OpenDatabase(*config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	o := snapshot.Options{Dir: *dir, Label: *label, Reason: *reason}
	if !*noEOS {
		o.EOSDir = eos.BaseDirectory
	}
	snap, err := snapshot.Create(db, o)
	closeFn()
	if err != nil {
		fmt.Fprintln(os.Stderr, "snapshot failed:", err)
		return 1
	}
	v := snapshot.Verify(snap.Path, db.Descriptor().Password)
	fmt.Printf("created %s\n", snap.Path)
	printVerify(v)
	if !v.OK {
		return 1
	}
	return 0
}

func snapshotList(args []string) int {
	fs := flag.NewFlagSet("snapshot list", flag.ExitOnError)
	dir := fs.String("dir", snapshot.DefaultDir(), "snapshots directory")
	verify := fs.Bool("verify", false, "verify every snapshot now instead of showing the cached result")
	_ = fs.Parse(args)
	es, err := snapshot.List(*dir, *verify, host.DatabasePasswordFor)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if len(es) == 0 {
		fmt.Printf("no snapshots in %s\n", *dir)
		return 0
	}
	fmt.Printf("%-40s %-12s %-12s %10s  %-10s %s\n", "NAME", "DB", "LABEL", "SIZE", "VERIFIED", "REASON")
	for _, e := range es {
		if e.Err != nil {
			fmt.Printf("%-40s %-12s %-12s %10s  %-10s %v\n", e.Name, "?", "?", "?", "BROKEN", e.Err)
			continue
		}
		status := "unknown"
		if e.Verify != nil {
			status = "ok"
			if !e.Verify.OK {
				status = "FAILED"
			}
		}
		fmt.Printf("%-40s %-12s %-12s %10s  %-10s %s\n", e.Name, e.Manifest.DBName, e.Manifest.Label, humanSize(e.Size), status, e.Manifest.Reason)
	}
	return 0
}

func snapshotVerify(args []string) int {
	name, rest := splitPositional(args)
	fs := flag.NewFlagSet("snapshot verify", flag.ExitOnError)
	dir := fs.String("dir", snapshot.DefaultDir(), "snapshots directory")
	_ = fs.Parse(rest)
	if name == "" {
		name = fs.Arg(0)
	}
	if name == "" {
		fmt.Fprintln(os.Stderr, "snapshot verify <name|latest>")
		return 2
	}
	p, err := snapshot.Resolve(*dir, name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var pw *string
	if m, err := snapshot.ReadManifest(p); err == nil {
		pw = host.DatabasePasswordFor(m.DBName)
		if m.Encrypted && pw == nil {
			fmt.Println("note: snapshot is encrypted and PRODUCTION_DB_PASSWORD is not configured; decryption will not be checked")
		}
	}
	v := snapshot.Verify(p, pw)
	fmt.Println(p)
	printVerify(v)
	if !v.OK {
		return 1
	}
	return 0
}

func snapshotRestore(args []string) int {
	name, rest := splitPositional(args)
	fs := flag.NewFlagSet("snapshot restore", flag.ExitOnError)
	config := fs.String("config", host.DefaultConfiguration, "Release (production) or Debug (development)")
	dir := fs.String("dir", snapshot.DefaultDir(), "snapshots directory")
	includeEOS := fs.Bool("include-eos", false, "also restore sessions.json / lobbies.json")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	_ = fs.Parse(rest)
	if name == "" {
		name = fs.Arg(0)
	}
	if name == "" {
		fmt.Fprintln(os.Stderr, "snapshot restore <name|latest> [--include-eos] [--yes]")
		return 2
	}
	path, err := host.DatabasePath(*config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	snapDir, err := snapshot.Resolve(*dir, name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	m, err := snapshot.ReadManifest(snapDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	pw := host.DatabasePasswordFor(snapshot.DBNameFromPath(path))
	if !*yes {
		fmt.Printf("Restore %s (created %s, label %q, reason %q)\nover %s", filepath.Base(snapDir), m.CreatedAt, m.Label, m.Reason, path)
		if *includeEOS {
			fmt.Printf(" and the EOS files in %s", eos.BaseDirectory)
		}
		fmt.Print("?\nA pre-restore safety snapshot is taken first. Type 'restore' to continue: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(line) != "restore" {
			fmt.Println("aborted")
			return 1
		}
	}
	res, err := snapshot.Restore(filepath.Base(snapDir), snapshot.RestoreOptions{
		Dir: *dir, DBPath: path, EOSDir: eos.BaseDirectory, IncludeEOS: *includeEOS, Password: pw,
	})
	if res != nil && res.Verify != nil {
		printVerify(res.Verify)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "restore refused:", err)
		return 1
	}
	if res.SafetySnapshot != "" {
		fmt.Printf("safety snapshot: %s\n", res.SafetySnapshot)
	}
	for _, f := range res.Replaced {
		fmt.Printf("restored %s\n", f)
	}
	for _, f := range res.Removed {
		fmt.Printf("removed %s (absent from the snapshot)\n", f)
	}
	fmt.Println("restore complete; start the Solara service again")
	return 0
}

func snapshotPrune(args []string) int {
	fs := flag.NewFlagSet("snapshot prune", flag.ExitOnError)
	dir := fs.String("dir", snapshot.DefaultDir(), "snapshots directory")
	keep := fs.Int("keep", snapshot.DefaultKeep, "number of newest snapshots to keep")
	force := fs.Bool("force", false, "also delete snapshots labelled keep / pre-restore")
	_ = fs.Parse(args)
	deleted, err := snapshot.Prune(*dir, *keep, *force)
	for _, d := range deleted {
		fmt.Printf("deleted %s\n", d)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("%d snapshot(s) deleted\n", len(deleted))
	return 0
}

func printVerify(v *snapshot.VerifyResult) {
	if v.OK {
		fmt.Println("verification: OK")
	} else {
		fmt.Println("verification: FAILED")
	}
	if v.PasswordChecked {
		fmt.Printf("  password decrypts store: %v\n", v.PasswordOK)
	}
	for _, p := range v.Problems {
		fmt.Println("  problem:", p)
	}
	for _, n := range v.Notes {
		fmt.Println("  note:", n)
	}
}

func humanSize(n int64) string {
	const k = 1024
	switch {
	case n >= k*k*k:
		return fmt.Sprintf("%.1f GiB", float64(n)/(k*k*k))
	case n >= k*k:
		return fmt.Sprintf("%.1f MiB", float64(n)/(k*k))
	case n >= k:
		return fmt.Sprintf("%.1f KiB", float64(n)/k)
	}
	return fmt.Sprintf("%d B", n)
}
