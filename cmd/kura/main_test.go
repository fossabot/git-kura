package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// Integration tests exercise the git-kura binary through Git's subcommand
// dispatch against real temporary repositories.

func TestRepositoryContext(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "get succeeds in repository", args: []string{"get", "51", "--path"}},
		{name: "open succeeds in repository", args: []string{"open", "51"}},
		{name: "close succeeds in repository", args: []string{"close", "51"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := cli.gitKura(repo, tc.args...)
			requireExitCode(t, result, 0)
		})
	}

	outside := t.TempDir()
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "get path fails outside repository", args: []string{"get", "51", "--path"}},
		{name: "get branch fails outside repository", args: []string{"get", "51", "--branch"}},
		{name: "get json fails outside repository", args: []string{"get", "51", "--json"}},
		{name: "open fails outside repository", args: []string{"open", "51"}},
		{name: "close fails outside repository", args: []string{"close", "51"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := cli.gitKura(outside, tc.args...)
			requireNonZeroExitCode(t, result)
			requireEmptyStdout(t, result)
			requireStderrContains(t, result, "repository")
			assertPathMissing(t, filepath.Join(outside, ".git"))
			assertPathMissing(t, expectedStateDir(outside))
			assertPathMissing(t, filepath.Join(outside, ".git-kura.toml"))
		})
	}
}

func TestKeyValidationRejectsUnsafeKeysWithoutFilesystemChanges(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	for _, key := range []string{
		"../x",
		"@{upstream}",
	} {
		t.Run(printableName(key), func(t *testing.T) {
			before := gitRefs(t, repo)
			result := cli.gitKura(repo, "open", key)
			requireNonZeroExitCode(t, result)
			requireEmptyStdout(t, result)
			requireStderrContains(t, result, "key")
			if after := gitRefs(t, repo); after != before {
				t.Fatalf("git refs changed for invalid key %q\nbefore:\n%s\nafter:\n%s", key, before, after)
			}
			assertPathMissing(t, expectedWorktreePath(repo, key))
		})
	}
}

func TestGetPathIsStateIndependentAndScriptFriendly(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	want := expectedWorktreePath(repo, "51")

	for _, mutate := range []struct {
		name string
		fn   func(t *testing.T)
	}{
		{name: "clean", fn: func(t *testing.T) {}},
		{name: "unstaged changes", fn: func(t *testing.T) { appendFile(t, filepath.Join(repo, "tracked.txt"), "unstaged\n") }},
		{name: "staged changes", fn: func(t *testing.T) {
			writeFile(t, filepath.Join(repo, "staged.txt"), "staged\n")
			git(t, repo, "add", "staged.txt")
		}},
		{name: "untracked file", fn: func(t *testing.T) { writeFile(t, filepath.Join(repo, "untracked.txt"), "untracked\n") }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			mutate.fn(t)
			result := cli.gitKura(repo, "get", "51", "--path")
			requireExitCode(t, result, 0)
			requireStdoutLine(t, result, want)
			requireCleanValueStdout(t, result)
		})
	}
}

func TestGetPath(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	result := cli.gitKura(repo, "get", "51", "--path")
	requireExitCode(t, result, 0)
	requireStdoutLine(t, result, expectedWorktreePath(repo, "51"))
	requireCleanValueStdout(t, result)

	invalid := cli.gitKura(repo, "get", "../x", "--path")
	requireNonZeroExitCode(t, invalid)
	requireEmptyStdout(t, invalid)

	outside := cli.gitKura(t.TempDir(), "get", "51", "--path")
	requireNonZeroExitCode(t, outside)
	requireEmptyStdout(t, outside)
}

func TestGetPathCommandSubstitutionPOSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell command substitution is covered by the Windows-specific test")
	}

	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
	result := cli.posixShell(repo, `cd "$(git kura get 51 --path)"`)
	requireExitCode(t, result, 0)
}

func TestGetPathCommandSubstitutionWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows command substitution is covered on Windows")
	}

	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
	result := cli.windowsCommand(repo, `for /f "delims=" %p in ('git kura get 51 --path') do cd /d "%p"`)
	requireExitCode(t, result, 0)
}

func TestGetBranch(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	result := cli.gitKura(repo, "get", "51", "--branch")
	requireExitCode(t, result, 0)
	requireStdoutLine(t, result, "kura-51")
	requireCleanValueStdout(t, result)

	invalid := cli.gitKura(repo, "get", "../x", "--branch")
	requireNonZeroExitCode(t, invalid)
	requireEmptyStdout(t, invalid)

	outside := cli.gitKura(t.TempDir(), "get", "51", "--branch")
	requireNonZeroExitCode(t, outside)
	requireEmptyStdout(t, outside)
}

func TestCloseRemovesWorktreeAndMetadata(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
	metadataPath := expectedMetadataPath(repo, "51")
	assertPathExists(t, expectedWorktreePath(repo, "51"))
	assertPathExists(t, metadataPath)

	requireExitCode(t, cli.gitKura(repo, "close", "51"), 0)
	assertPathMissing(t, expectedWorktreePath(repo, "51"))
	assertPathMissing(t, metadataPath)
}

func TestOpenCreatesMetadata(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)

	metadataPath := expectedMetadataPath(repo, "51")
	assertPathExists(t, metadataPath)

	metadata := requireJSONFile(t, metadataPath)
	if metadata["baseBranch"] != "main" {
		t.Fatalf("metadata baseBranch = %v, want main", metadata["baseBranch"])
	}
	if metadata["worktreePath"] != expectedWorktreePath(repo, "51") {
		t.Fatalf("metadata worktreePath = %v, want %s", metadata["worktreePath"], expectedWorktreePath(repo, "51"))
	}
}

func TestGetStructuredOutputUsesOpenTimeBaseBranch(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
	git(t, repo, "checkout", "-b", "later")

	result := cli.gitKura(repo, "get", "51", "--json")
	requireExitCode(t, result, 0)

	metadata := requireJSONMetadata(t, result.stdout)
	if metadata["baseBranch"] != "main" {
		t.Fatalf("json baseBranch = %v, want open-time base branch main", metadata["baseBranch"])
	}
}

func TestGetStructuredOutputFailsWhenMetadataIsMissing(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
	if err := os.Remove(expectedMetadataPath(repo, "51")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	jsonResult := cli.gitKura(repo, "get", "51", "--json")
	requireNonZeroExitCode(t, jsonResult)
	requireEmptyStdout(t, jsonResult)
	requireStderrContains(t, jsonResult, "metadata")

	toonResult := cli.gitKura(repo, "get", "51", "--toon")
	requireNonZeroExitCode(t, toonResult)
	requireEmptyStdout(t, toonResult)
	requireStderrContains(t, toonResult, "metadata")

	requireStdoutLine(t, cli.gitKura(repo, "get", "51", "--path"), expectedWorktreePath(repo, "51"))
	requireStdoutLine(t, cli.gitKura(repo, "get", "51", "--branch"), "kura-51")
}

func TestGetTOONOutputContainsMetadataFields(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
	result := cli.gitKura(repo, "get", "51", "--toon")
	requireExitCode(t, result, 0)

	for _, want := range []string{
		"schemaVersion",
		"key",
		"kind",
		"branch",
		"worktreePath",
		"repositoryRoot",
		"baseBranch",
		"exists",
		"dirty",
	} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("toon output = %q, want it to contain field %q", result.stdout, want)
		}
	}
}

func TestGetJSONOutputConformsToSchema(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	result := cli.gitKura(repo, "get", "51", "--json")
	requireExitCode(t, result, 0)

	requireConformsToOutputSchema(t, result.stdout)
}

// Unit tests cover pure parsing and validation helpers without creating Git
// repositories or invoking the compiled binary.

func TestValidateKeyAcceptsValidKeys(t *testing.T) {
	for _, key := range []string{
		"51",
		"051",
		"ABC-123",
		"abc-123",
		"task-51",
		"bugfix_login",
		"release-2026-06",
		"a",
		"Z",
		"0",
	} {
		t.Run(key, func(t *testing.T) {
			if err := validateKey(key); err != nil {
				t.Fatalf("validateKey(%q) = %v, want nil", key, err)
			}
		})
	}
}

func TestValidateKeyRejectsInvalidKeys(t *testing.T) {
	for _, key := range []string{
		"../x",
		".git",
		"feature..x",
		"feature.lock",
		"feature.",
		"a/b",
		"a b",
		"",
		".hidden",
		"@{upstream}",
	} {
		t.Run(printableName(key), func(t *testing.T) {
			if err := validateKey(key); err == nil {
				t.Fatalf("validateKey(%q) = nil, want error", key)
			}
		})
	}
}

func TestBranchName(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want string
	}{
		{"51", "kura-51"},
		{"ABC-123", "kura-ABC-123"},
		{"release-2026-06", "kura-release-2026-06"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			if got := branchName(tc.key); got != tc.want {
				t.Fatalf("branchName(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestWorktreePath(t *testing.T) {
	for _, tc := range []struct {
		repoRoot string
		key      string
		want     string
	}{
		{
			repoRoot: filepath.Join("/home", "user", "repo"),
			key:      "51",
			want:     filepath.Join("/home", "user", "repo.kura", "worktrees", "51"),
		},
		{
			repoRoot: filepath.Join("/home", "user", "myproject"),
			key:      "feature",
			want:     filepath.Join("/home", "user", "myproject.kura", "worktrees", "feature"),
		},
	} {
		t.Run(tc.key, func(t *testing.T) {
			if got := worktreePath(tc.repoRoot, tc.key); got != tc.want {
				t.Fatalf("worktreePath(%q, %q) = %q, want %q", tc.repoRoot, tc.key, got, tc.want)
			}
		})
	}
}

func TestParseGetArgs(t *testing.T) {
	t.Run("default output mode is path", func(t *testing.T) {
		key, opts, err := parseGetArgs([]string{"51"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "51" {
			t.Fatalf("key = %q, want %q", key, "51")
		}
		if opts.OutputMode != outputPath {
			t.Fatalf("OutputMode = %q, want %q", opts.OutputMode, outputPath)
		}
	})

	t.Run("--path produces path output mode", func(t *testing.T) {
		_, opts, err := parseGetArgs([]string{"51", "--path"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.OutputMode != outputPath {
			t.Fatalf("OutputMode = %q, want %q", opts.OutputMode, outputPath)
		}
	})

	t.Run("--branch produces branch output mode", func(t *testing.T) {
		_, opts, err := parseGetArgs([]string{"51", "--branch"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.OutputMode != outputBranch {
			t.Fatalf("OutputMode = %q, want %q", opts.OutputMode, outputBranch)
		}
	})

	t.Run("--json and --format json produce same output mode", func(t *testing.T) {
		_, shortOpts, err := parseGetArgs([]string{"51", "--json"})
		if err != nil {
			t.Fatalf("--json: unexpected error: %v", err)
		}
		_, formatOpts, err := parseGetArgs([]string{"51", "--format", "json"})
		if err != nil {
			t.Fatalf("--format json: unexpected error: %v", err)
		}
		if shortOpts.OutputMode != formatOpts.OutputMode {
			t.Fatalf("--json mode %q != --format json mode %q", shortOpts.OutputMode, formatOpts.OutputMode)
		}
		if shortOpts.OutputMode != outputJSON {
			t.Fatalf("OutputMode = %q, want %q", shortOpts.OutputMode, outputJSON)
		}
	})

	t.Run("--toon and --format toon produce same output mode", func(t *testing.T) {
		_, shortOpts, err := parseGetArgs([]string{"51", "--toon"})
		if err != nil {
			t.Fatalf("--toon: unexpected error: %v", err)
		}
		_, formatOpts, err := parseGetArgs([]string{"51", "--format", "toon"})
		if err != nil {
			t.Fatalf("--format toon: unexpected error: %v", err)
		}
		if shortOpts.OutputMode != formatOpts.OutputMode {
			t.Fatalf("--toon mode %q != --format toon mode %q", shortOpts.OutputMode, formatOpts.OutputMode)
		}
		if shortOpts.OutputMode != outputTOON {
			t.Fatalf("OutputMode = %q, want %q", shortOpts.OutputMode, outputTOON)
		}
	})

	t.Run("unknown format is error", func(t *testing.T) {
		_, _, err := parseGetArgs([]string{"51", "--format", "xml"})
		if err == nil {
			t.Fatal("expected error for unknown format, got nil")
		}
		for _, want := range []string{"format", "json", "toon"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q does not mention %q", err.Error(), want)
			}
		}
	})

	t.Run("missing format value is error", func(t *testing.T) {
		_, _, err := parseGetArgs([]string{"51", "--format"})
		if err == nil {
			t.Fatal("expected error for missing format value, got nil")
		}
		if !strings.Contains(err.Error(), "--format") {
			t.Fatalf("error %q does not mention --format", err.Error())
		}
	})

	for _, args := range [][]string{
		{"51", "--path", "--branch"},
		{"51", "--json", "--toon"},
		{"51", "--path", "--json"},
		{"51", "--branch", "--json"},
	} {
		args := args
		t.Run("conflict: "+strings.Join(args[1:], " "), func(t *testing.T) {
			_, _, err := parseGetArgs(args)
			if err == nil {
				t.Fatal("expected conflict error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "conflict") {
				t.Fatalf("error %q does not mention 'conflict'", err.Error())
			}
		})
	}

	t.Run("no key is usage error", func(t *testing.T) {
		_, _, err := parseGetArgs([]string{})
		if err == nil {
			t.Fatal("expected error for missing key, got nil")
		}
	})

	t.Run("unknown flag is error", func(t *testing.T) {
		_, _, err := parseGetArgs([]string{"51", "--unknown"})
		if err == nil {
			t.Fatal("expected error for unknown flag, got nil")
		}
	})

	t.Run("invalid key is error", func(t *testing.T) {
		_, _, err := parseGetArgs([]string{"../x"})
		if err == nil {
			t.Fatal("expected error for invalid key, got nil")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "key") {
			t.Fatalf("error %q does not mention 'key'", err.Error())
		}
	})
}

func TestParseKeyOnlyArgs(t *testing.T) {
	t.Run("valid key succeeds", func(t *testing.T) {
		key, err := parseKeyOnlyArgs("open", []string{"51"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "51" {
			t.Fatalf("key = %q, want %q", key, "51")
		}
	})

	t.Run("no key is usage error", func(t *testing.T) {
		_, err := parseKeyOnlyArgs("open", []string{})
		if err == nil {
			t.Fatal("expected error for missing key, got nil")
		}
	})

	t.Run("extra argument is error", func(t *testing.T) {
		_, err := parseKeyOnlyArgs("open", []string{"51", "--extra"})
		if err == nil {
			t.Fatal("expected error for extra argument, got nil")
		}
	})

	t.Run("invalid key is error", func(t *testing.T) {
		_, err := parseKeyOnlyArgs("open", []string{"../x"})
		if err == nil {
			t.Fatal("expected error for invalid key, got nil")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "key") {
			t.Fatalf("error %q does not mention 'key'", err.Error())
		}
	})
}

// In-process command tests cover dispatch and command branches without spawning
// the compiled binary. End-to-end CLI behavior is covered above.

func TestRunHelpAndUsage(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "top-level short help", args: []string{"-h"}, want: "Usage: git kura"},
		{name: "top-level long help", args: []string{"--help"}, want: "Usage: git kura"},
		{name: "get help", args: []string{"get", "--help"}, want: "Usage: git kura get"},
		{name: "open help", args: []string{"open", "--help"}, want: "Usage: git kura open"},
		{name: "close help", args: []string{"close", "--help"}, want: "Usage: git kura close"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, err := captureStdout(t, func() error {
				return run(tc.args)
			})
			if err != nil {
				t.Fatalf("run(%v) error = %v, want nil", tc.args, err)
			}
			if !strings.Contains(stdout, tc.want) {
				t.Fatalf("stdout = %q, want it to contain %q", stdout, tc.want)
			}
		})
	}

	for _, args := range [][]string{
		{},
		{"unknown"},
	} {
		t.Run(strings.Join(append([]string{"error"}, args...), " "), func(t *testing.T) {
			if err := run(args); err == nil {
				t.Fatalf("run(%v) error = nil, want error", args)
			}
		})
	}
}

func TestRunArgumentErrors(t *testing.T) {
	for _, args := range [][]string{
		{"get"},
		{"get", "51", "--format"},
		{"open", "51", "--extra"},
		{"close", "51", "--extra"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if err := run(args); err == nil {
				t.Fatalf("run(%v) error = nil, want error", args)
			}
		})
	}
}

func TestRunCommandsInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	withWorkingDir(t, repo, func() {
		stdout, err := captureStdout(t, func() error {
			return run([]string{"get", "51", "--path"})
		})
		if err != nil {
			t.Fatalf("get --path error = %v", err)
		}
		if strings.TrimSpace(stdout) != expectedWorktreePath(repo, "51") {
			t.Fatalf("get --path stdout = %q, want %q", stdout, expectedWorktreePath(repo, "51"))
		}

		stdout, err = captureStdout(t, func() error {
			return run([]string{"get", "51", "--branch"})
		})
		if err != nil {
			t.Fatalf("get --branch error = %v", err)
		}
		if strings.TrimSpace(stdout) != "kura-51" {
			t.Fatalf("get --branch stdout = %q, want kura-51", stdout)
		}

		stdout, err = captureStdout(t, func() error {
			return run([]string{"get", "51", "--json"})
		})
		if err != nil {
			t.Fatalf("get --json before open error = %v", err)
		}
		metadata := requireJSONMetadata(t, stdout)
		if metadata["exists"] != false {
			t.Fatalf("exists = %v, want false before open", metadata["exists"])
		}

		if err := run([]string{"open", "51"}); err != nil {
			t.Fatalf("open error = %v", err)
		}
		assertPathExists(t, expectedWorktreePath(repo, "51"))
		assertPathExists(t, expectedMetadataPath(repo, "51"))

		stdout, err = captureStdout(t, func() error {
			return run([]string{"get", "51", "--toon"})
		})
		if err != nil {
			t.Fatalf("get --toon error = %v", err)
		}
		for _, want := range []string{"schemaVersion", "worktreePath", "baseBranch", "exists"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("toon stdout = %q, want it to contain %q", stdout, want)
			}
		}

		if err := run([]string{"close", "51"}); err != nil {
			t.Fatalf("close error = %v", err)
		}
		assertPathMissing(t, expectedWorktreePath(repo, "51"))
		assertPathMissing(t, expectedMetadataPath(repo, "51"))
	})
}

func TestRunCommandErrorPathsInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	withWorkingDir(t, repo, func() {
		if err := run([]string{"close", "missing"}); err != nil {
			t.Fatalf("close missing worktree error = %v, want nil", err)
		}

		if err := run([]string{"open", "51"}); err != nil {
			t.Fatalf("open error = %v", err)
		}
		if err := run([]string{"open", "51"}); err == nil {
			t.Fatal("duplicate open error = nil, want error")
		}

		appendFile(t, filepath.Join(expectedWorktreePath(repo, "51"), "tracked.txt"), "dirty\n")
		stdout, err := captureStdout(t, func() error {
			return run([]string{"get", "51", "--json"})
		})
		if err != nil {
			t.Fatalf("get --json dirty error = %v", err)
		}
		metadata := requireJSONMetadata(t, stdout)
		if metadata["dirty"] != true {
			t.Fatalf("dirty = %v, want true", metadata["dirty"])
		}
	})
}

func TestRunCommandsOutsideRepositoryInProcess(t *testing.T) {
	outside := t.TempDir()

	withWorkingDir(t, outside, func() {
		for _, args := range [][]string{
			{"get", "51", "--path"},
			{"get", "51", "--json"},
			{"open", "51"},
			{"close", "51"},
		} {
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				if err := run(args); err == nil {
					t.Fatalf("run(%v) error = nil, want error", args)
				}
			})
		}
	})
}

func TestRunStructuredOutputRequiresMetadataForExistingWorktree(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	withWorkingDir(t, repo, func() {
		if err := run([]string{"open", "51"}); err != nil {
			t.Fatalf("open error = %v", err)
		}
		if err := os.Remove(expectedMetadataPath(repo, "51")); err != nil {
			t.Fatal(err)
		}
		if err := run([]string{"get", "51", "--json"}); err == nil {
			t.Fatal("get --json with missing metadata error = nil, want error")
		}
		if err := run([]string{"get", "51", "--toon"}); err == nil {
			t.Fatal("get --toon with missing metadata error = nil, want error")
		}
		if err := run([]string{"close", "51"}); err != nil {
			t.Fatalf("close with missing metadata error = %v, want nil", err)
		}
	})
}

func TestGitHelpersReturnErrors(t *testing.T) {
	dir := t.TempDir()

	if _, err := headBranch(dir); err == nil {
		t.Fatal("headBranch outside git repo error = nil, want error")
	}
	if _, err := worktreeDirty(dir); err == nil {
		t.Fatal("worktreeDirty outside git repo error = nil, want error")
	}
}

func TestReadMetadata(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	path := expectedMetadataPath(repo, "51")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, `{"baseBranch":"main","worktreePath":"/tmp/worktree"}`)

	meta, err := readMetadata(repo, "51")
	if err != nil {
		t.Fatalf("readMetadata error = %v", err)
	}
	if meta.BaseBranch != "main" || meta.WorktreePath != "/tmp/worktree" {
		t.Fatalf("metadata = %+v, want main and /tmp/worktree", meta)
	}

	writeFile(t, path, `{`)
	if _, err := readMetadata(repo, "51"); err == nil {
		t.Fatal("readMetadata invalid JSON error = nil, want error")
	}

	if _, err := readMetadata(repo, "missing"); err == nil {
		t.Fatal("readMetadata missing file error = nil, want error")
	}
}

func TestMetadataPath(t *testing.T) {
	repo := filepath.Join("/home", "user", "repo")
	want := filepath.Join("/home", "user", "repo.kura", "meta", "worktrees", "51.json")
	if got := metadataPath(repo, "51"); got != want {
		t.Fatalf("metadataPath(%q, 51) = %q, want %q", repo, got, want)
	}
}

func TestRequireCleanValueStdoutAcceptsWindowsPath(t *testing.T) {
	requireCleanValueStdout(t, cliResult{stdout: `C:\repo.kura\worktrees\51` + "\n"})
}

// Test helpers

type cliResult struct {
	stdout string
	stderr string
	code   int
}

type testCLI struct {
	t       *testing.T
	binDir  string
	envPath string
}

func newTestCLI(t *testing.T) *testCLI {
	t.Helper()

	binDir := t.TempDir()
	bin := filepath.Join(binDir, "git-kura")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build git-kura: %v\n%s", err, output)
	}

	envPath := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	return &testCLI{t: t, binDir: binDir, envPath: envPath}
}

func (c *testCLI) initRepo(t *testing.T) string {
	t.Helper()

	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.email", "kura-test@example.com")
	git(t, repo, "config", "user.name", "Kura Test")
	writeFile(t, filepath.Join(repo, "tracked.txt"), "initial\n")
	git(t, repo, "add", "tracked.txt")
	git(t, repo, "commit", "-m", "initial")
	return repo
}

func (c *testCLI) gitKura(dir string, args ...string) cliResult {
	c.t.Helper()
	return c.run(dir, append([]string{"kura"}, args...)...)
}

func (c *testCLI) posixShell(dir, script string) cliResult {
	c.t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+c.envPath)
	return runCommand(cmd)
}

func (c *testCLI) windowsCommand(dir, script string) cliResult {
	c.t.Helper()
	cmd := exec.Command("cmd", "/C", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+c.envPath)
	return runCommand(cmd)
}

func (c *testCLI) run(dir string, args ...string) cliResult {
	c.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+c.envPath)
	return runCommand(cmd)
}

func runCommand(cmd *exec.Cmd) cliResult {
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	code := 0
	if err := cmd.Run(); err != nil {
		code = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		}
	}
	return cliResult{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func withWorkingDir(t *testing.T, dir string, fn func()) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(old); err != nil {
			t.Fatal(err)
		}
	}()
	fn()
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write

	fnErr := fn()

	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = old

	out, err := io.ReadAll(read)
	if err != nil {
		t.Fatal(err)
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	return string(out), fnErr
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func gitRefs(t *testing.T, repo string) string {
	t.Helper()
	return git(t, repo, "show-ref", "--heads")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendFile(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

func expectedWorktreePath(repo, key string) string {
	return filepath.Join(expectedStateDir(repo), "worktrees", key)
}

func expectedMetadataPath(repo, key string) string {
	return filepath.Join(expectedStateDir(repo), "meta", "worktrees", key+".json")
}

func expectedStateDir(repo string) string {
	return filepath.Join(filepath.Dir(repo), filepath.Base(repo)+".kura")
}

func printableName(value string) string {
	if value == "" {
		return "(empty)"
	}
	return strings.Map(func(r rune) rune {
		switch r {
		case '\x00', '\n', '\t', '/', '\\':
			return '_'
		default:
			return r
		}
	}, value)
}

func requireExitCode(t *testing.T, result cliResult, want int) {
	t.Helper()
	if result.code != want {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", result.code, want, result.stdout, result.stderr)
	}
}

func requireNonZeroExitCode(t *testing.T, result cliResult) {
	t.Helper()
	if result.code == 0 {
		t.Fatalf("exit code = 0, want non-zero\nstdout:\n%s\nstderr:\n%s", result.stdout, result.stderr)
	}
}

func requireEmptyStdout(t *testing.T, result cliResult) {
	t.Helper()
	if result.stdout != "" {
		t.Fatalf("stdout = %q, want empty", result.stdout)
	}
}

func requireStdoutLine(t *testing.T, result cliResult, want string) {
	t.Helper()
	requireExitCode(t, result, 0)
	if strings.TrimSuffix(result.stdout, "\n") != want {
		t.Fatalf("stdout = %q, want %q\nstderr:\n%s", result.stdout, want+"\n", result.stderr)
	}
}

func requireCleanValueStdout(t *testing.T, result cliResult) {
	t.Helper()
	if strings.Count(result.stdout, "\n") != 1 {
		t.Fatalf("stdout should contain exactly one line, got %q", result.stdout)
	}
	for _, forbidden := range []string{": ", "\"", "'", "warning"} {
		if strings.Contains(strings.ToLower(result.stdout), forbidden) {
			t.Fatalf("stdout contains non-value text %q in %q", forbidden, result.stdout)
		}
	}
}

func requireStderrContains(t *testing.T, result cliResult, want string) {
	t.Helper()
	if !strings.Contains(strings.ToLower(result.stderr), strings.ToLower(want)) {
		t.Fatalf("stderr = %q, want it to contain %q", result.stderr, want)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s exists, want missing", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s should exist: %v", path, err)
	}
}

func requireJSONMetadata(t *testing.T, output string) map[string]any {
	t.Helper()
	var metadata map[string]any
	if err := json.Unmarshal([]byte(output), &metadata); err != nil {
		t.Fatalf("json output is not parseable: %v\n%s", err, output)
	}
	return metadata
}

func requireJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json file %s: %v", path, err)
	}
	return requireJSONMetadata(t, string(data))
}

func requireConformsToOutputSchema(t *testing.T, jsonOutput string) {
	t.Helper()

	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(jsonOutput))
	if err != nil {
		t.Fatalf("parse json output: %v\noutput: %s", err, jsonOutput)
	}

	if err := outputSchema.Validate(inst); err != nil {
		t.Fatalf("json output does not conform to schema:\n%v\noutput: %s", err, jsonOutput)
	}
}
