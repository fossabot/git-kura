package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/tooppoo/git-kura/internal/gitutil"
	"github.com/tooppoo/git-kura/internal/worktree"
)

const sealHelp = `Usage: git kura seal <subcommand> [args]

Manage the current seal key for the active session.

Subcommands:
  enter <key> [-- <command...>]  Start a child shell with GIT_KURA_SEAL_KEY=<key>
  current                        Print the current seal key (GIT_KURA_SEAL_KEY)
  ls [key]                       List sealed paths, optionally filtered by key
  add <path> [path...]            Add paths to the seal store under the current key
  remove <path> [path...]         Remove paths from the seal store under the current key
  session <subcommand>           Inspect and clean seal session records

Run "git kura seal <subcommand> --help" for subcommand-specific help.`

const sealLsHelp = `Usage: git kura seal ls [key]

List sealed paths recorded in the seal store.

Without arguments, lists every sealed path across all keys for the whole
repository (the seal store shared by all worktrees). With a key argument,
lists only the paths sealed by that key.

ls is a repository-wide inspection command: it does NOT read
GIT_KURA_SEAL_KEY, so its output is the same inside and outside a sealed
session. To inspect a single key, pass it explicitly.

Output is one line per sealed path:

  <key>	<path>

Paths are repository-root relative with "/" separators. Lines are sorted
by key, then by path. An empty store produces no output and exits 0.`

const sealEnterHelp = `Usage: git kura seal enter <key> [-- <command...>]

Start a child shell with GIT_KURA_SEAL_KEY set to <key>.
The child shell inherits the current environment plus the seal key.
Exit the child shell with 'exit' or Ctrl-D to return.

If -- <command...> is given, run the command without an interactive shell.`

const sealCurrentHelp = `Usage: git kura seal current

Print the value of GIT_KURA_SEAL_KEY.
Exits with non-zero if GIT_KURA_SEAL_KEY is not set.`

const sealAddHelp = `Usage: git kura seal add <path> [path...]

Add one or more file paths to the seal store under the current key (GIT_KURA_SEAL_KEY).

Paths are interpreted relative to the repository root, regardless of the
current working directory. Absolute paths are rejected.
Exits with error if:
  - GIT_KURA_SEAL_KEY is not set or invalid
  - any path is absolute or outside the repository
  - any path does not exist or is a directory
  - any path is already sealed under a different key

If a path is already sealed under the current key, it is skipped (idempotent).`

const sealRemoveHelp = `Usage: git kura seal remove <path> [path...]

Remove one or more file paths from the seal store under the current key (GIT_KURA_SEAL_KEY).

Paths are interpreted relative to the repository root, regardless of the
current working directory. Absolute paths are rejected.
Exits with error if:
  - GIT_KURA_SEAL_KEY is not set or invalid
  - any path is absolute or outside the repository
  - any path is sealed under a different key

Paths not currently in the seal store are skipped (idempotent).`

type sealEnterArgs struct {
	Key     string
	Command []string
}

func argsBeforeDoubleDash(args []string) []string {
	for i, a := range args {
		if a == "--" {
			return args[:i]
		}
	}
	return args
}

func runSeal(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: git kura seal <subcommand> [args]")
	}

	switch args[0] {
	case "-h", "--help":
		fmt.Println(sealHelp)
		return nil
	case "enter":
		if hasHelpFlag(argsBeforeDoubleDash(args[1:])) {
			fmt.Println(sealEnterHelp)
			return nil
		}
		a, err := parseSealEnterArgs(args[1:])
		if err != nil {
			return err
		}
		return cmdSealEnter(a)
	case "current":
		if hasHelpFlag(args[1:]) {
			fmt.Println(sealCurrentHelp)
			return nil
		}
		if err := parseSealCurrentArgs(args[1:]); err != nil {
			return err
		}
		return cmdSealCurrent()
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
	case "session":
		return runSealSession(args[1:])
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
// ls never consults GIT_KURA_SEAL_KEY: inspection scope must not depend on
// the caller's session. It also reads the store without acquiring
// paths.lock, so a held lock never blocks listing.
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

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func parseSealEnterArgs(args []string) (sealEnterArgs, error) {
	if len(args) == 0 {
		return sealEnterArgs{}, fmt.Errorf("usage: git kura seal enter <key> [-- <command...>]")
	}

	key := args[0]
	if err := validateKey(key); err != nil {
		return sealEnterArgs{}, err
	}

	rest := args[1:]
	if len(rest) == 0 {
		return sealEnterArgs{Key: key}, nil
	}
	if rest[0] != "--" {
		return sealEnterArgs{}, fmt.Errorf("usage: git kura seal enter <key> [-- <command...>]: unexpected argument %q", rest[0])
	}
	if len(rest) < 2 {
		return sealEnterArgs{}, fmt.Errorf("usage: git kura seal enter <key> -- <command...>: command required after --")
	}
	return sealEnterArgs{Key: key, Command: rest[1:]}, nil
}

func parseSealCurrentArgs(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("usage: git kura seal current: unexpected argument %q", args[0])
	}
	return nil
}

func cmdSealEnter(a sealEnterArgs) error {
	repoRoot, err := gitutil.RepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}

	sessDir, err := sealSessionDir(repoRoot)
	if err != nil {
		return err
	}

	sessPath, sess, err := acquireSealSession(sessDir, repoRoot, a.Key, os.Getpid())
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	if len(a.Command) > 0 {
		cmd = exec.Command(a.Command[0], a.Command[1:]...)
	} else {
		cmd = exec.Command(detectShell())
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "GIT_KURA_SEAL_KEY="+a.Key)

	wtPath, wtErr := worktree.Path(repoRoot, a.Key)
	if wtErr != nil {
		wtPath = "(unknown)"
	}
	fmt.Printf("[kura] entered seal: %s\nworktree: %s\nrun `git kura seal current` to inspect\ntype `exit` to leave\n", a.Key, wtPath)

	if err := cmd.Start(); err != nil {
		return errors.Join(fmt.Errorf("seal enter: %w", err), deleteSealSession(sessPath))
	}

	sess.ChildPID = cmd.Process.Pid
	if err := updateSealSession(sessPath, sess); err != nil {
		// Session file can't reflect the child PID.  A dead parent with child_pid=0
		// looks stale to future seal enter callers even while the child shell runs.
		// Abort: kill the child so the user can retry with a consistent state.
		killErr := cmd.Process.Kill()
		// ExitError after Kill is expected (process exits by signal); surface other errors.
		if waitErr := cmd.Wait(); waitErr != nil && !errors.As(waitErr, new(*exec.ExitError)) {
			killErr = errors.Join(killErr, fmt.Errorf("wait for killed child: %w", waitErr))
		}
		return errors.Join(fmt.Errorf("seal enter: record child PID in session: %w", err), killErr, deleteSealSession(sessPath))
	}

	waitErr := cmd.Wait()
	deleteErr := deleteSealSession(sessPath)

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return errors.Join(fmt.Errorf("seal enter: %w", waitErr), deleteErr)
	}
	return deleteErr
}

func cmdSealCurrent() error {
	key := os.Getenv("GIT_KURA_SEAL_KEY")
	if key == "" {
		return fmt.Errorf("not in sealed session (GIT_KURA_SEAL_KEY not set), run 'git kura seal enter <key>' to start one")
	}
	fmt.Println(key)
	return nil
}

func detectShell() string {
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("pwsh"); err == nil {
			return "pwsh"
		}
		return "cmd.exe"
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "sh"
}
