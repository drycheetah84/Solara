// CLI for per-server identity tokens (docs/SERVER_IDENTITY.md, SEC-SID):
//
//	solara server-token issue  --deployment ID [--box B] [--station S] [--idle-hours N] [--max-days N]
//	solara server-token list   [--all]
//	solara server-token revoke <prefix|id> | --deployment ID   [--reason R]
//	solara server-token prune  [--grace-hours N]
//	solara agent-token  issue  --box NAME [--idle-days N] [--max-days N]
//	solara agent-token  list   [--all]
//	solara agent-token  revoke <prefix|id> [--reason R]
//
// Every command takes --config Release|Debug and opens the database the
// server uses. The database is exclusively locked by a running `solara
// serve`; while it runs, list/revoke through the dashboard API instead
// (GET/DELETE /api/server-tokens). An issued token is printed exactly once.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"solara/internal/docdb"
	"solara/internal/host"
	"solara/internal/serverid"
)

func init() {
	subcommands["server-token"] = func(args []string) int { return tokenCmd(serverid.KindServer, args) }
	subcommands["agent-token"] = func(args []string) int { return tokenCmd(serverid.KindAgent, args) }
}

// tokOpenTimeout bounds the wait for the database file lock.
const tokOpenTimeout = 10 * time.Second

func tokenCmd(kind string, args []string) int {
	name := kind + "-token"
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "usage: solara %s issue|list|revoke%s [flags] (see docs/SERVER_IDENTITY.md)\n", name, map[bool]string{true: "|prune"}[kind == serverid.KindServer])
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "issue":
		return tokIssue(kind, rest)
	case "list":
		return tokList(kind, rest)
	case "revoke":
		return tokRevoke(kind, rest)
	case "prune":
		if kind == serverid.KindServer {
			return tokPrune(rest)
		}
	}
	fmt.Fprintf(os.Stderr, "unknown %s command %q\n", name, sub)
	return 2
}

// tokOpen opens the configured database with a timeout (a running server
// holds the file lock) and points the token store at it.
func tokOpen(config string) (func(), error) {
	type res struct {
		db    *docdb.Database
		close func()
		err   error
	}
	ch := make(chan res, 1)
	go func() {
		db, c, err := host.OpenDatabase(config)
		ch <- res{db, c, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		serverid.SetDatabase(r.db)
		return r.close, nil
	case <-time.After(tokOpenTimeout):
		return nil, errors.New("the database is locked (is `solara serve` running?) -- stop it, or use the dashboard API GET/DELETE /api/server-tokens")
	}
}

// tokSplit takes a leading positional argument (flag stops at the first one).
func tokSplit(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

func tokIssue(kind string, args []string) int {
	fs := flag.NewFlagSet(kind+"-token issue", flag.ContinueOnError)
	config := fs.String("config", host.DefaultConfiguration, "Release (production) or Debug (development)")
	dep := fs.String("deployment", "", "deployment id the server token is bound to (server tokens)")
	box := fs.String("box", "", "box name (required for agent tokens)")
	station := fs.String("station", "", "optional station id binding (server tokens)")
	idleH := fs.Float64("idle-hours", 0, "idle timeout in hours (0 = default)")
	idleD := fs.Float64("idle-days", 0, "idle timeout in days (0 = default)")
	maxD := fs.Float64("max-days", 0, "absolute lifetime in days (0 = default)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	switch kind {
	case serverid.KindServer:
		if strings.TrimSpace(*dep) == "" {
			fmt.Fprintln(os.Stderr, "--deployment is required")
			return 2
		}
	case serverid.KindAgent:
		if strings.TrimSpace(*box) == "" {
			fmt.Fprintln(os.Stderr, "--box is required")
			return 2
		}
		if *dep != "" || *station != "" {
			fmt.Fprintln(os.Stderr, "agent tokens are bound to a box only (no --deployment/--station)")
			return 2
		}
	}
	ttl := serverid.TTLs{
		Idle: time.Duration(*idleH*float64(time.Hour)) + time.Duration(*idleD*float64(24*time.Hour)),
		Max:  time.Duration(*maxD * float64(24*time.Hour)),
	}
	closeFn, err := tokOpen(*config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer closeFn()
	tok, rec, err := serverid.IssueSpec(serverid.Spec{Kind: kind, DeploymentId: *dep, StationId: *station, Box: *box, TTLs: ttl})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("issued %s token %s (id %s)\n", kind, rec.Prefix, rec.IdHex())
	fmt.Printf("  idle expiry %s, absolute expiry %s\n", serverid.FormatMs(rec.ExpiresAt), serverid.FormatMs(rec.AbsoluteExpiry))
	fmt.Println()
	fmt.Println(tok)
	fmt.Println()
	fmt.Println("This is the only time the token is shown; it is stored as a hash only.")
	if kind == serverid.KindAgent {
		fmt.Println("Put it in the agent config (HalcyonAgent: AgentToken = '...' in server.config.psd1).")
	} else {
		fmt.Println("Pass it to the game as -ServerApiKey=<token> -DashboardApiKey=<token> (spin-ups do this automatically).")
	}
	return 0
}

func tokList(kind string, args []string) int {
	fs := flag.NewFlagSet(kind+"-token list", flag.ContinueOnError)
	config := fs.String("config", host.DefaultConfiguration, "Release (production) or Debug (development)")
	all := fs.Bool("all", false, "include revoked and expired tokens")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	closeFn, err := tokOpen(*config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer closeFn()
	list, err := serverid.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	now := time.Now().UTC().UnixMilli()
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "PREFIX\tID\tDEPLOYMENT\tSTATION\tBOX\tISSUED\tLAST SEEN\tEXPIRES\tSTATE")
	n := 0
	for _, r := range list {
		if r.Kind != kind {
			continue
		}
		state := "active"
		switch {
		case r.Revoked():
			state = "revoked: " + derefS(r.RevokeReason)
		case now >= r.ExpiresAt || now >= r.AbsoluteExpiry:
			state = "expired"
		}
		if !*all && state != "active" {
			continue
		}
		n++
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Prefix, r.IdHex(), dash(r.Deployment()), dash(r.Station()), dash(r.BoxName()),
			serverid.FormatMs(r.IssuedAt), serverid.FormatMs(r.LastSeen), serverid.FormatMs(min(r.ExpiresAt, r.AbsoluteExpiry)), state)
	}
	_ = w.Flush()
	fmt.Printf("%d %s token(s)\n", n, kind)
	return 0
}

func tokRevoke(kind string, args []string) int {
	target, rest := tokSplit(args)
	fs := flag.NewFlagSet(kind+"-token revoke", flag.ContinueOnError)
	config := fs.String("config", host.DefaultConfiguration, "Release (production) or Debug (development)")
	dep := fs.String("deployment", "", "revoke every server token of this deployment")
	reason := fs.String("reason", "revoked with the CLI", "reason recorded with the revocation")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if target == "" && fs.NArg() > 0 {
		target = fs.Arg(0)
	}
	if (target == "") == (*dep == "") || (*dep != "" && kind != serverid.KindServer) {
		fmt.Fprintf(os.Stderr, "usage: solara %s-token revoke <prefix|id>%s\n", kind, map[bool]string{true: " | --deployment ID"}[kind == serverid.KindServer])
		return 2
	}
	closeFn, err := tokOpen(*config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer closeFn()
	if *dep != "" {
		n, err := serverid.Default.RevokeDeployment(*dep, *reason)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("revoked %d server token(s) of deployment %s\n", n, *dep)
		return 0
	}
	// resolve first so a server-token command never revokes an agent token (and vice versa)
	list, err := serverid.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var hit []*serverid.ServerTokenDbObject
	for _, r := range list {
		if r.Kind == kind && (r.Prefix == target || r.IdHex() == strings.ToLower(target)) {
			hit = append(hit, r)
		}
	}
	switch len(hit) {
	case 0:
		fmt.Fprintf(os.Stderr, "no %s token matches %q\n", kind, target)
		return 1
	case 1:
	default:
		fmt.Fprintf(os.Stderr, "%q matches %d tokens; use the id\n", target, len(hit))
		return 1
	}
	r, err := serverid.Revoke(hit[0].IdHex(), *reason)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("revoked %s token %s (id %s)\n", kind, r.Prefix, r.IdHex())
	return 0
}

func tokPrune(args []string) int {
	fs := flag.NewFlagSet("server-token prune", flag.ContinueOnError)
	config := fs.String("config", host.DefaultConfiguration, "Release (production) or Debug (development)")
	grace := fs.Float64("grace-hours", 24*7, "keep revoked/expired records this long (for auditing)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	closeFn, err := tokOpen(*config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer closeFn()
	n, err := serverid.Prune(time.Duration(*grace * float64(time.Hour)))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("pruned %d token record(s)\n", n)
	return 0
}

func derefS(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
