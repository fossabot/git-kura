package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tooppoo/git-kura/internal/gitutil"
)

const sealHelp = `Usage: git kura seal <subcommand> [args]

Manage sealed paths in the repository-wide seal store.

Subcommands:
  ls [key]                       List sealed paths, optionally filtered by key
  add <path> [path...]            Add paths to the seal store under the current key
  remove <path> [path...]         Remove paths from the seal store under the current key

Run "git kura seal <subcommand> --help" for subcommand-specific help.`

const sealLsHelp = `Usage: git kura seal ls [key]

List sealed paths recorded in the seal store.

Without arguments, lists every sealed path across all keys for the whole
repository (the seal store shared by all worktrees). With a key argument,
lists only the paths sealed by that key.

ls is a repository-wide inspection command: it does not derive a current key
from the worktree, so its output is the same regardless of where it is run.
To inspect a single key, pass it explicitly.

Output is one line per sealed path:

  <key>	<path>

Paths are repository-root relative with "/" separators. Lines are sorted
by key, then by path. An empty store produces no output and exits 0.`

const sealAddHelp = `Usage: git kura seal add <path> [path...]

Add one or more file paths to the seal store under the current key.

Paths are interpreted relative to the repository root, regardless of the
current working directory. Absolute paths are rejected.
Exits with error if:
  - no current seal key is available (see "Current key" below)
  - any path is absolute or outside the repository
  - any path does not exist or is a directory
  - any path is already sealed under a different key

If a path is already sealed under the current key, it is skipped (idempotent).

Current key:
  The current key is derived from the git-kura managed worktree you are in:
  run this command from inside the worktree created by "git kura open <key>"
  and that worktree's key becomes the current key. It fails when the current
  directory is not inside a managed worktree, or when that worktree's
  metadata is missing or inconsistent.`

const sealRemoveHelp = `Usage: git kura seal remove <path> [path...]

Remove one or more file paths from the seal store under the current key.

Paths are interpreted relative to the repository root, regardless of the
current working directory. Absolute paths are rejected.
Exits with error if:
  - no current seal key is available (see "Current key" below)
  - any path is absolute or outside the repository
  - any path is sealed under a different key

Paths not currently in the seal store are skipped (idempotent).

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
	case "add":
		if hasHelpFlag(args[1:]) {
			fmt.Println(sealAddHelp)
			return nil
		}
		if len(args) < 2 {
			return fmt.Errorf("usage: git kura seal add <path> [path...]")
		}
		return cmdSealAdd(args[1:])
	case "remove":
		if hasHelpFlag(args[1:]) {
			fmt.Println(sealRemoveHelp)
			return nil
		}
		if len(args) < 2 {
			return fmt.Errorf("usage: git kura seal remove <path> [path...]")
		}
		return cmdSealRemove(args[1:])
	default:
		return fmt.Errorf("unknown seal subcommand: %s", args[0])
	}
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

// cmdSealLs lists sealed paths from the path seal store as "<key>\t<path>"
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
