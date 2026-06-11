package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/tooppoo/git-kura/internal/gitutil"
)

const sealHelp = `Usage: git kura seal <subcommand> [args]

Manage the current seal key for the active session.

Subcommands:
  enter <key> [-- <command...>]  Start a child shell with GIT_KURA_SEAL_KEY=<key>
  current                        Print the current seal key (GIT_KURA_SEAL_KEY)

Run "git kura seal <subcommand> --help" for subcommand-specific help.`

const sealEnterHelp = `Usage: git kura seal enter <key> [-- <command...>]

Start a child shell with GIT_KURA_SEAL_KEY set to <key>.
The child shell inherits the current environment plus the seal key.
Exit the child shell with 'exit' or Ctrl-D to return.

If -- <command...> is given, run the command without an interactive shell.`

const sealCurrentHelp = `Usage: git kura seal current

Print the value of GIT_KURA_SEAL_KEY.
Exits with non-zero if GIT_KURA_SEAL_KEY is not set.`

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
	default:
		return fmt.Errorf("unknown seal subcommand: %s", args[0])
	}
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

	if err := cmd.Start(); err != nil {
		deleteSealSession(sessPath)
		return fmt.Errorf("seal enter: %w", err)
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
		deleteSealSession(sessPath)
		return errors.Join(fmt.Errorf("seal enter: record child PID in session: %w", err), killErr)
	}

	waitErr := cmd.Wait()
	deleteSealSession(sessPath)

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("seal enter: %w", waitErr)
	}
	return nil
}

func cmdSealCurrent() error {
	key := os.Getenv("GIT_KURA_SEAL_KEY")
	if key == "" {
		return fmt.Errorf("GIT_KURA_SEAL_KEY is not set")
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
