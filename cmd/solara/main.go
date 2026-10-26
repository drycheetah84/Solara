// Command solara is the Go port of the Halcyon backend host
// (AUnrealFeatures.Ares/Program.cs + AUnrealFeatures.Hosting.ModuleInitialization).
//
//	solara [serve] [--config Release|Debug]     run the backend (default)
//	solara import-litedb --from <export-dir> [--config ...] [--overwrite]
//	solara check-secrets                          list unconfigured legacy secrets
//
// Startup order reproduces Program.Main exactly:
//  1. ModuleInitialization.Initialize (optional self-elevation, "Main" logger)
//  2. ConfigureServices: database manager (production + development), the
//     active database (Debug -> development, Release -> production), the
//     collection pre-registration in the original order, Mothership.Database
//  3. AddServer registrations: A2StationDb :78, AresDashboard :8080,
//     Mothership :90, EosGateway :50, EosWs :80, HalcyonSocket :9095/:9100
//  4. ServerWatchdog.TakeDeploymentOffline hook, StationAcl.PushAll()
//  5. RunAsync: the DI provider constructs (and thereby starts) every
//     server in registration order, then the host waits forever.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"solara/internal/docdb"
	"solara/internal/host"
	"solara/internal/logging"
	"solara/internal/secrets"
)

func main() {
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		os.Exit(serve(args))
	case "import-litedb":
		os.Exit(importLiteDB(args))
	case "check-secrets":
		if m := secrets.Missing(); len(m) > 0 {
			fmt.Println("missing:", m)
			os.Exit(1)
		}
		fmt.Println("all legacy secrets configured")
	default:
		if fn, ok := subcommands[cmd]; ok {
			os.Exit(fn(args))
		}
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		os.Exit(2)
	}
}

func serve(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	config := fs.String("config", host.DefaultConfiguration, "build configuration to emulate: Release (production.db) or Debug (development.db)")
	_ = fs.Parse(args)

	h, err := host.Build(*config)
	if err != nil {
		logging.Main.Fatal("Host startup failed: {Message}", err.Error())
		return 1
	}
	if err := h.Run(); err != nil {
		logging.Main.Fatal("Host startup failed: {Message}", err.Error())
		h.Shutdown()
		return 1
	}
	// Original: await Task.Delay(-1). Solara additionally shuts down cleanly on
	// SIGINT/SIGTERM so the database file is closed (KNOWN_ISSUES HOST-002).
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	h.Shutdown()
	return 0
}

func importLiteDB(args []string) int {
	fs := flag.NewFlagSet("import-litedb", flag.ExitOnError)
	from := fs.String("from", "", "directory written by tools/litedb-export")
	config := fs.String("config", host.DefaultConfiguration, "Release (production) or Debug (development)")
	overwrite := fs.Bool("overwrite", false, "replace collections that already contain documents")
	_ = fs.Parse(args)
	if *from == "" {
		fmt.Fprintln(os.Stderr, "--from is required")
		return 2
	}
	db, closeFn, err := host.OpenDatabase(*config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer closeFn()
	rep, err := docdb.ImportLiteDBExport(db, *from, *overwrite)
	if rep != nil {
		for name, n := range rep.Collections {
			fmt.Printf("%s: %d documents\n", name, n)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// subcommands holds additional CLI commands registered by other files in this
// package via init() (e.g. snapshot, server-token), so features can add
// commands without editing this dispatcher.
var subcommands = map[string]func(args []string) int{}
