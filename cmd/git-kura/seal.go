package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"text/tabwriter"
	"time"

	"github.com/tooppoo/git-kura/internal/gitutil"
	"github.com/tooppoo/git-kura/internal/worktree"
)

const sealHelp = `Usage: git kura seal <subcommand> [args]

Manage the current seal key for the active session.

Subcommands:
  enter <key> [-- <command...>]  Start a child shell with GIT_KURA_SEAL_KEY=<key>
  current                        Print the current seal key (GIT_KURA_SEAL_KEY)
  ls                             List all recorded seal sessions
  release                        Remove stale seal sessions
  add <path> [path...]            Add paths to the seal store under the current key
  remove <path> [path...]         Remove paths from the seal store under the current key

Run "git kura seal <subcommand> --help" for subcommand-specific help.`

const sealLsHelp = `Usage: git kura seal ls

List all recorded seal sessions for this repository.

Displays key, worktree path, parent PID, child PID, age, and status.
Sessions whose PIDs are dead are shown as stale.
Sessions exceeding the TTL but with live PIDs are shown as stale-candidate.

TTL is configured via GIT_KURA_SESSION_TTL (e.g. "10m"). Default: 5m.`

const sealReleaseHelp = `Usage: git kura seal release

Remove stale seal sessions from this repository.

A session is removed only when its parent and child PIDs are confirmed dead.
TTL-exceeded sessions are NOT removed unless their PIDs are also dead.
Sessions with unknown liveness are never removed.`

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
		if len(args) > 1 {
			return fmt.Errorf("usage: git kura seal ls: unexpected argument %q", args[1])
		}
		return cmdSealLs()
	case "release":
		if hasHelpFlag(args[1:]) {
			fmt.Println(sealReleaseHelp)
			return nil
		}
		if len(args) > 1 {
			return fmt.Errorf("usage: git kura seal release: unexpected argument %q", args[1])
		}
		return cmdSealRelease()
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

func cmdSealLs() error {
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

	ttl := sessionTTL()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "key\tworktree\tparent\tchild\tage\tstatus"); err != nil {
		return err
	}
	for _, r := range records {
		if r.Err != nil {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				"(unknown)", r.Path, "-", "-", "-", r.Err.Error(),
			); err != nil {
				return err
			}
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

func cmdSealRelease() error {
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

	var errs []error
	removed := 0

	if len(stale) == 0 {
		fmt.Println("No stale sessions found.")
	} else {
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
	}

	if len(corrupt) > 0 {
		fmt.Println("Warning: corrupt or unreadable session file(s) skipped — inspect manually:")
		for _, r := range corrupt {
			fmt.Printf("  %s\n", r.Path)
		}
	}

	return errors.Join(errs...)
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
