package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tooppoo/git-kura/internal/gitutil"
)

const sealSessionHelp = `Usage: git kura seal session <subcommand>

Inspect and clean seal session records for this repository.

Subcommands:
  ls [key]   List recorded seal sessions, optionally filtered by key
  clean      Remove stale seal sessions (dry-run unless --apply is given)

Run "git kura seal session <subcommand> --help" for subcommand-specific help.`

const sealSessionLsHelp = `Usage: git kura seal session ls [key]

List seal sessions recorded for this repository.

Displays key, worktree path, parent PID, child PID, age, and status.
Sessions whose PIDs are dead are shown as stale.
Sessions exceeding the TTL but with live PIDs are shown as stale-candidate.

session ls is a repository-wide inspection command: it does NOT read
GIT_KURA_SEAL_KEY, so its output is the same inside and outside a sealed
session. The inspected scope is the session store in the Git common dir,
shared by all worktrees of the repository. Pass a key argument to list only
the sessions recorded under that key.

TTL is configured via GIT_KURA_SESSION_TTL (e.g. "10m"). Default: 5m.`

const sealSessionCleanHelp = `Usage: git kura seal session clean [--apply]

Remove stale seal sessions from this repository.

A session is treated as stale only when its parent and child PIDs are
confirmed dead. TTL-exceeded sessions are NOT removed unless their PIDs are
also dead. Sessions with unknown liveness are never removed.

session clean is dry-run by default: it prints the sessions that would be
removed and exits without modifying anything. Pass --apply to actually
remove the stale sessions.

session clean does not read GIT_KURA_SEAL_KEY; it operates on the
repository-wide session store resolved from the Git common dir.`

func runSealSession(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: git kura seal session <subcommand> [args]")
	}
	switch args[0] {
	case "-h", "--help":
		fmt.Println(sealSessionHelp)
		return nil
	case "ls":
		if hasHelpFlag(args[1:]) {
			fmt.Println(sealSessionLsHelp)
			return nil
		}
		key, err := parseSealSessionLsArgs(args[1:])
		if err != nil {
			return err
		}
		return cmdSealSessionLs(key)
	case "clean":
		if hasHelpFlag(args[1:]) {
			fmt.Println(sealSessionCleanHelp)
			return nil
		}
		apply, err := parseSealSessionCleanArgs(args[1:])
		if err != nil {
			return err
		}
		return cmdSealSessionClean(apply)
	default:
		return fmt.Errorf("unknown seal session subcommand: %s", args[0])
	}
}

// parseSealSessionLsArgs accepts at most one positional key argument. Like
// seal ls, session ls is intentionally option-free in v0 so future flags can
// be added without ambiguity.
func parseSealSessionLsArgs(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if strings.HasPrefix(args[0], "-") {
		return "", fmt.Errorf("usage: git kura seal session ls [key]: unknown option %q", args[0])
	}
	if len(args) > 1 {
		return "", fmt.Errorf("usage: git kura seal session ls [key]: unexpected argument %q", args[1])
	}
	if err := validateKey(args[0]); err != nil {
		return "", err
	}
	return args[0], nil
}

// parseSealSessionCleanArgs reports whether --apply was given. session clean is
// dry-run by default (see ADR 20260612T170922Z); deletion requires --apply.
func parseSealSessionCleanArgs(args []string) (bool, error) {
	apply := false
	for _, a := range args {
		switch a {
		case "--apply":
			apply = true
		default:
			return false, fmt.Errorf("usage: git kura seal session clean [--apply]: unexpected argument %q", a)
		}
	}
	return apply, nil
}

// cmdSealSessionLs lists every seal session record (one row per worktree),
// optionally narrowed to filterKey. Per ADR 20260612T170922Z it never consults
// GIT_KURA_SEAL_KEY: inspection scope must not depend on the caller's session.
func cmdSealSessionLs(filterKey string) error {
	repoRoot, err := gitutil.RepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}
	sessDir, err := sealSessionDir(repoRoot)
	if err != nil {
		return err
	}
	records, err := readAllSealSessions(sessDir)
	if err != nil {
		return err
	}

	// Stable, deterministic order for humans, scripts, and tests. Corrupt
	// records (no Key) sort by path so they are still listed predictably.
	sort.Slice(records, func(i, j int) bool {
		ki, kj := records[i].Session.Key, records[j].Session.Key
		if ki != kj {
			return ki < kj
		}
		return records[i].Path < records[j].Path
	})

	ttl := sessionTTL()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "key\tworktree\tparent\tchild\tage\tstatus"); err != nil {
		return err
	}
	for _, r := range records {
		if r.Err != nil {
			// A corrupt record has no usable key, so it cannot be filtered by
			// key. Always surface it: it blocks seal enter for its worktree.
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				"(unknown)", r.Path, "-", "-", "-", r.Err.Error(),
			); err != nil {
				return err
			}
			continue
		}
		if filterKey != "" && r.Session.Key != filterKey {
			continue
		}
		age := time.Since(r.Session.StartedAt).Truncate(time.Second)
		if _, err := fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\t%s\n",
			r.Session.Key,
			r.Session.WorktreePath,
			r.Session.ParentPID,
			r.Session.ChildPID,
			formatAge(age),
			sessionStatusStr(r.Session, ttl),
		); err != nil {
			return err
		}
	}
	return w.Flush()
}

// cmdSealSessionClean removes stale seal sessions (PIDs confirmed dead). It is
// dry-run by default and only deletes when apply is true, per ADR
// 20260612T170922Z. TTL-exceeded sessions with live PIDs are never removed.
func cmdSealSessionClean(apply bool) error {
	repoRoot, err := gitutil.RepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}
	sessDir, err := sealSessionDir(repoRoot)
	if err != nil {
		return err
	}
	records, err := readAllSealSessions(sessDir)
	if err != nil {
		return err
	}

	var stale, corrupt []sealSessionFile
	for _, r := range records {
		switch {
		case r.Err != nil:
			corrupt = append(corrupt, r)
		case !sessionAlive(r.Session):
			stale = append(stale, r)
		}
	}
	sort.Slice(stale, func(i, j int) bool { return stale[i].Path < stale[j].Path })

	var errs []error

	if len(stale) == 0 {
		fmt.Println("No stale sessions found.")
	} else if apply {
		removed := 0
		fmt.Println("Removing stale sessions:")
		for _, r := range stale {
			age := time.Since(r.Session.StartedAt).Truncate(time.Second)
			fmt.Printf("  %s (key: %s, age: %s)\n", r.Path, r.Session.Key, formatAge(age))
			ok, removeErr := removeStaleSession(r.Path, r.Session)
			if removeErr != nil {
				errs = append(errs, removeErr)
			} else if ok {
				removed++
			}
		}
		fmt.Printf("Removed %d stale session(s).\n", removed)
	} else {
		fmt.Println("Stale sessions that would be removed (dry-run; re-run with --apply to remove):")
		for _, r := range stale {
			age := time.Since(r.Session.StartedAt).Truncate(time.Second)
			fmt.Printf("  %s (key: %s, age: %s)\n", r.Path, r.Session.Key, formatAge(age))
		}
		fmt.Printf("%d stale session(s) would be removed. Re-run with --apply to remove them.\n", len(stale))
	}

	if len(corrupt) > 0 {
		fmt.Println("Warning: corrupt or unreadable session file(s) skipped — inspect manually:")
		for _, r := range corrupt {
			fmt.Printf("  %s\n", r.Path)
		}
	}

	return errors.Join(errs...)
}
