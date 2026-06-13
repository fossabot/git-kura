package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tooppoo/git-kura/internal/gitutil"
)

const sealHelp = `Usage: git kura seal <subcommand> [args]

Manage path claims in the repository-wide seal store.

A claim records that the current task — identified by the git-kura managed
worktree you are in — intends to edit a path. This lets conflicting edits
across tasks/worktrees be detected before they reach a merge.

Subcommands:
  ls [key]                       List claimed paths, optionally filtered by key
  claim <path> [path...]         Claim paths for the current key
  unclaim <path> [path...]       Release the current key's claim on paths
  test <path> [path...]          Check paths against the current seal context

Run "git kura seal <subcommand> --help" for subcommand-specific help.`

const sealLsHelp = `Usage: git kura seal ls [key]

List claimed paths recorded in the seal store.

Without arguments, lists every claimed path across all keys for the whole
repository (the seal store shared by all worktrees). With a key argument,
lists only the paths claimed by that key.

ls is a repository-wide inspection command: it does not derive a current key
from the worktree, so its output is the same regardless of where it is run.
To inspect a single key, pass it explicitly.

Output is one line per claimed path:

  <key>	<path>

Paths are repository-root relative with "/" separators. Lines are sorted
by key, then by path. An empty store produces no output and exits 0.`

const sealClaimHelp = `Usage: git kura seal claim <path> [path...]

Claim one or more file paths for the current key in the seal store.

Paths are interpreted relative to the repository root, regardless of the
current working directory. Absolute paths are rejected.
Exits with error if:
  - no current seal key is available (see "Current key" below)
  - any path is absolute or outside the repository
  - any path does not exist or is a directory
  - any path is already claimed by a different key

If a path is already claimed by the current key, it is skipped (idempotent).

Current key:
  The current key is derived from the git-kura managed worktree you are in:
  run this command from inside the worktree created by "git kura open <key>"
  and that worktree's key becomes the current key. It fails when the current
  directory is not inside a managed worktree, or when that worktree's
  metadata is missing or inconsistent.`

const sealUnclaimHelp = `Usage: git kura seal unclaim <path> [path...]

Release the current key's claim on one or more file paths in the seal store.

Paths are interpreted relative to the repository root, regardless of the
current working directory. Absolute paths are rejected.
Exits with error if:
  - no current seal key is available (see "Current key" below)
  - any path is absolute or outside the repository
  - any path is claimed by a different key

Paths not currently claimed are skipped (idempotent).

Current key:
  The current key is derived from the git-kura managed worktree you are in:
  run this command from inside the worktree created by "git kura open <key>"
  and that worktree's key becomes the current key. It fails when the current
  directory is not inside a managed worktree, or when that worktree's
  metadata is missing or inconsistent.`

const sealTestHelp = `Usage: git kura seal test <path> [path...]

Check whether one or more paths may be handled in the current seal context.

seal test is read-only: it never modifies the seal store and never takes the
store lock. It answers a single question — given the current key, is every
listed path safe to edit?

Paths are interpreted relative to the repository root, regardless of the
current working directory. Absolute paths and paths outside the repository are
rejected. A path inside the repository that does not exist yet is treated as
unclaimed, so seal test can be used to check a file before creating it.

A path is safe when it is unclaimed, or already claimed by the current key. A
path claimed by a different key is a conflict. seal test exits 0 only when every
path is safe; if any path conflicts it exits with seal-conflict (code 6) and
reports each conflicting path and the key that claims it.

This command takes no options in v0: --all, --unsealed, and --staged are not
defined and are rejected.

Current key:
  The current key is derived from the git-kura managed worktree you are in:
  run this command from inside the worktree created by "git kura open <key>"
  and that worktree's key becomes the current key. It fails when the current
  directory is not inside a managed worktree, or when that worktree's
  metadata is missing or inconsistent.`

func runSeal(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: git kura seal <subcommand> [args]")
	}

	switch args[0] {
	case "-h", "--help":
		fmt.Println(sealHelp)
		return nil
	case "ls":
		if hasHelpFlag(args[1:]) {
			fmt.Println(sealLsHelp)
			return nil
		}
		key, err := parseSealLsArgs(args[1:])
		if err != nil {
			return err
		}
		return cmdSealLs(key)
	case "claim":
		return runSealClaim(args[1:])
	case "unclaim":
		return runSealUnclaim(args[1:])
	case "test":
		return runSealTest(args[1:])
	default:
		return fmt.Errorf("unknown seal subcommand: %s", args[0])
	}
}

func runSealClaim(args []string) error {
	if hasHelpFlag(args) {
		fmt.Println(sealClaimHelp)
		return nil
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: git kura seal claim <path> [path...]")
	}
	return cmdSealClaim(args)
}

func runSealUnclaim(args []string) error {
	if hasHelpFlag(args) {
		fmt.Println(sealUnclaimHelp)
		return nil
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: git kura seal unclaim <path> [path...]")
	}
	return cmdSealUnclaim(args)
}

func runSealTest(args []string) error {
	if hasHelpFlag(args) {
		fmt.Println(sealTestHelp)
		return nil
	}
	paths, err := parseSealTestArgs(args)
	if err != nil {
		return err
	}
	return cmdSealTest(paths)
}

// parseSealTestArgs requires at least one positional path and rejects any
// option. seal test is intentionally option-free in v0: the not-yet-defined
// --all / --unsealed / --staged modes must error rather than be silently
// ignored, so a future release can add them without changing behavior.
func parseSealTestArgs(args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: git kura seal test <path> [path...]")
	}
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			return nil, fmt.Errorf("usage: git kura seal test <path> [path...]: unknown option %q", a)
		}
	}
	return args, nil
}

// parseSealLsArgs accepts at most one positional key argument. Options are
// rejected: ls is intentionally option-free in v0 so that future flags such
// as --format can be added without ambiguity.
func parseSealLsArgs(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if strings.HasPrefix(args[0], "-") {
		return "", fmt.Errorf("usage: git kura seal ls [key]: unknown option %q", args[0])
	}
	if len(args) > 1 {
		return "", fmt.Errorf("usage: git kura seal ls [key]: unexpected argument %q", args[1])
	}
	if err := validateKey(args[0]); err != nil {
		return "", err
	}
	return args[0], nil
}

// cmdSealLs lists claimed paths from the path seal store as "<key>\t<path>"
// lines, sorted by key then path. An empty filterKey lists every key.
// Per docs/adr/20260612T170922Z_seal-command-current-context-and-scope.md,
// ls is always repository-wide: its scope must not depend on the caller's
// current worktree. It also reads the store without acquiring paths.lock,
// so a held lock never blocks listing.
func cmdSealLs(filterKey string) error {
	repoRoot, err := gitutil.RepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}
	storeFile, _, err := pathsSealStore(repoRoot)
	if err != nil {
		return err
	}
	store, err := readSealStore(storeFile)
	if err != nil {
		return err
	}

	paths := make([]string, 0, len(store.Paths))
	for p, entry := range store.Paths {
		if filterKey != "" && entry.Key != filterKey {
			continue
		}
		paths = append(paths, p)
	}
	sort.Slice(paths, func(i, j int) bool {
		ki, kj := store.Paths[paths[i]].Key, store.Paths[paths[j]].Key
		if ki != kj {
			return ki < kj
		}
		return paths[i] < paths[j]
	})

	var b strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&b, "%s\t%s\n", store.Paths[p].Key, p)
	}
	_, err = os.Stdout.WriteString(b.String())
	return err
}
