package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSealCurrentPrintsEnvVar(t *testing.T) {
	cli := newTestCLI(t)
	dir := t.TempDir()

	// Not set → non-zero exit, empty stdout, error on stderr
	unset := cli.gitKuraWithSealKey(dir, "", "seal", "current")
	requireNonZeroExitCode(t, unset)
	requireEmptyStdout(t, unset)
	requireStderrContains(t, unset, "GIT_KURA_SEAL_KEY")

	// Set → exit 0, prints key
	set := cli.gitKuraWithSealKey(dir, "my-key", "seal", "current")
	requireExitCode(t, set, 0)
	requireStdoutLine(t, set, "my-key")
	requireCleanValueStdout(t, set)
}

func TestSealCurrentWorksOutsideRepository(t *testing.T) {
	cli := newTestCLI(t)
	outside := t.TempDir()

	result := cli.gitKuraWithSealKey(outside, "outer-key", "seal", "current")
	requireExitCode(t, result, 0)
	requireStdoutLine(t, result, "outer-key")
}

func TestSealEnterSetsGitKuraSealKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TODO: add Windows-specific seal enter test with pwsh/cmd.exe")
	}

	cli := newTestCLI(t)
	dir := t.TempDir()

	// Use -- mode: run `git kura seal current` inside the sealed context
	result := cli.gitKuraWithSealKey(dir, "",
		"seal", "enter", "test-key", "--", "git", "kura", "seal", "current")
	requireExitCode(t, result, 0)
	requireStdoutLine(t, result, "test-key")
}

func TestSealEnterWorksOutsideRepository(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TODO: add Windows-specific seal enter test with pwsh/cmd.exe")
	}

	cli := newTestCLI(t)
	outside := t.TempDir()

	result := cli.gitKuraWithSealKey(outside, "",
		"seal", "enter", "outside-key", "--", "git", "kura", "seal", "current")
	requireExitCode(t, result, 0)
	requireStdoutLine(t, result, "outside-key")
}

func TestSealEnterOverridesSealKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TODO: add Windows-specific seal enter test with pwsh/cmd.exe")
	}

	cli := newTestCLI(t)
	dir := t.TempDir()

	// Even if GIT_KURA_SEAL_KEY is already set, enter overrides it in the child
	result := cli.gitKuraWithSealKey(dir, "old-key",
		"seal", "enter", "new-key", "--", "git", "kura", "seal", "current")
	requireExitCode(t, result, 0)
	requireStdoutLine(t, result, "new-key")
}

// Integration tests exercise the git-kura binary through Git's subcommand
// dispatch against real temporary repositories.

func TestRepositoryContext(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "ls succeeds in repository", args: []string{"ls"}},
		{name: "open succeeds in repository", args: []string{"open", "51"}},
		{name: "get succeeds in repository", args: []string{"get", "51", "--path"}},
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
		{name: "ls fails outside repository", args: []string{"ls"}},
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
	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)

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

	missing := cli.gitKura(repo, "get", "51", "--path")
	requireNonZeroExitCode(t, missing)
	requireEmptyStdout(t, missing)
	requireStderrContains(t, missing, "not open")

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
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

func TestGetDefaultRequiresOpenWorktree(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	beforeOpen := cli.gitKura(repo, "get", "51")
	requireNonZeroExitCode(t, beforeOpen)
	requireEmptyStdout(t, beforeOpen)
	requireStderrContains(t, beforeOpen, "not open")

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
	requireStdoutLine(t, cli.gitKura(repo, "get", "51"), expectedWorktreePath(repo, "51"))

	requireExitCode(t, cli.gitKura(repo, "close", "51"), 0)
	afterClose := cli.gitKura(repo, "get", "51")
	requireNonZeroExitCode(t, afterClose)
	requireEmptyStdout(t, afterClose)
	requireStderrContains(t, afterClose, "not open")

	pathAfterClose := cli.gitKura(repo, "get", "51", "--path")
	requireNonZeroExitCode(t, pathAfterClose)
	requireEmptyStdout(t, pathAfterClose)
	requireStderrContains(t, pathAfterClose, "not open")
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

	missing := cli.gitKura(repo, "get", "51", "--branch")
	requireNonZeroExitCode(t, missing)
	requireEmptyStdout(t, missing)
	requireStderrContains(t, missing, "not open")

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
	result := cli.gitKura(repo, "get", "51", "--branch")
	requireExitCode(t, result, 0)
	requireStdoutLine(t, result, "51")
	requireCleanValueStdout(t, result)

	invalid := cli.gitKura(repo, "get", "../x", "--branch")
	requireNonZeroExitCode(t, invalid)
	requireEmptyStdout(t, invalid)

	outside := cli.gitKura(t.TempDir(), "get", "51", "--branch")
	requireNonZeroExitCode(t, outside)
	requireEmptyStdout(t, outside)
}

func TestGetRoot(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	missing := cli.gitKura(repo, "get", "51", "--root")
	requireNonZeroExitCode(t, missing)
	requireEmptyStdout(t, missing)
	requireStderrContains(t, missing, "not open")

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
	result := cli.gitKura(repo, "get", "51", "--root")
	requireExitCode(t, result, 0)
	requireStdoutLine(t, result, repo)
	requireCleanValueStdout(t, result)

	invalid := cli.gitKura(repo, "get", "../x", "--root")
	requireNonZeroExitCode(t, invalid)
	requireEmptyStdout(t, invalid)

	outside := cli.gitKura(t.TempDir(), "get", "51", "--root")
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

func TestCloseAllowsReopenWithSameKey(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
	requireExitCode(t, cli.gitKura(repo, "close", "51"), 0)
	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)

	assertPathExists(t, expectedWorktreePath(repo, "51"))
	assertPathExists(t, expectedMetadataPath(repo, "51"))
}

func TestOpenCreatesMetadata(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)

	metadataPath := expectedMetadataPath(repo, "51")
	assertPathExists(t, metadataPath)

	metadata := requireJSONFile(t, metadataPath)
	if metadata["repositoryRoot"] != repo {
		t.Fatalf("metadata repositoryRoot = %v, want %s", metadata["repositoryRoot"], repo)
	}
	if metadata["baseBranch"] != "main" {
		t.Fatalf("metadata baseBranch = %v, want main", metadata["baseBranch"])
	}
	if metadata["worktreePath"] != expectedWorktreePath(repo, "51") {
		t.Fatalf("metadata worktreePath = %v, want %s", metadata["worktreePath"], expectedWorktreePath(repo, "51"))
	}
}

func TestOpenDryRunPrintsPlannedWorktree(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	result := cli.gitKura(repo, "open", "51", "--dry-run")
	requireExitCode(t, result, 0)
	requireEmptyStderr(t, result)
	requireConformsToOutputSchema(t, result.stdout)

	metadata := requireJSONMetadata(t, result.stdout)
	if metadata["branch"] != "51" {
		t.Fatalf("dry-run branch = %v, want 51", metadata["branch"])
	}
	if metadata["worktreePath"] != expectedWorktreePath(repo, "51") {
		t.Fatalf("dry-run worktreePath = %v, want %s", metadata["worktreePath"], expectedWorktreePath(repo, "51"))
	}
	if metadata["baseBranch"] != "main" {
		t.Fatalf("dry-run baseBranch = %v, want main", metadata["baseBranch"])
	}
	if metadata["exists"] != false {
		t.Fatalf("dry-run exists = %v, want false", metadata["exists"])
	}
	if metadata["dirty"] != false {
		t.Fatalf("dry-run dirty = %v, want false", metadata["dirty"])
	}
	assertPathMissing(t, expectedWorktreePath(repo, "51"))
	assertPathMissing(t, expectedMetadataPath(repo, "51"))
}

func TestOpenStoresWorktreeAndMetadataInGitCommonDir(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)

	wantStateDir := filepath.Join(repo, ".git", "kura")
	wantWorktreePath := filepath.Join(wantStateDir, "worktrees", "51")
	wantMetadataPath := filepath.Join(wantStateDir, "meta", "worktrees", "51.json")

	assertPathExists(t, wantWorktreePath)
	assertPathExists(t, wantMetadataPath)
	requireStdoutLine(t, cli.gitKura(repo, "get", "51", "--path"), wantWorktreePath)

	metadata := requireJSONFile(t, wantMetadataPath)
	if metadata["worktreePath"] != wantWorktreePath {
		t.Fatalf("metadata worktreePath = %v, want %s", metadata["worktreePath"], wantWorktreePath)
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
	requireStderrContains(t, jsonResult, "missing")

	toonResult := cli.gitKura(repo, "get", "51", "--toon")
	requireNonZeroExitCode(t, toonResult)
	requireEmptyStdout(t, toonResult)
	requireStderrContains(t, toonResult, "metadata")
	requireStderrContains(t, toonResult, "missing")

	pathResult := cli.gitKura(repo, "get", "51", "--path")
	requireNonZeroExitCode(t, pathResult)
	requireEmptyStdout(t, pathResult)
	requireStderrContains(t, pathResult, "metadata")

	branchResult := cli.gitKura(repo, "get", "51", "--branch")
	requireNonZeroExitCode(t, branchResult)
	requireEmptyStdout(t, branchResult)
	requireStderrContains(t, branchResult, "metadata")
}

func TestGetStructuredOutputFailsWhenWorktreeIsMissing(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
	if err := os.RemoveAll(expectedWorktreePath(repo, "51")); err != nil {
		t.Fatal(err)
	}

	jsonResult := cli.gitKura(repo, "get", "51", "--json")
	requireNonZeroExitCode(t, jsonResult)
	requireEmptyStdout(t, jsonResult)
	requireStderrContains(t, jsonResult, "worktree")
	requireStderrContains(t, jsonResult, "missing")
	requireStderrContains(t, jsonResult, "metadata exists")
}

func TestGetStructuredOutputFailsForUnopenedKey(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	requireExitCode(t, cli.gitKura(repo, "open", "1"), 0)

	jsonResult := cli.gitKura(repo, "get", "2", "--json")
	requireNonZeroExitCode(t, jsonResult)
	requireEmptyStdout(t, jsonResult)
	requireStderrContains(t, jsonResult, "not open")
	requireStderrContains(t, jsonResult, "git kura open 2")

	toonResult := cli.gitKura(repo, "get", "2", "--toon")
	requireNonZeroExitCode(t, toonResult)
	requireEmptyStdout(t, toonResult)
	requireStderrContains(t, toonResult, "not open")
	requireStderrContains(t, toonResult, "git kura open 2")
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

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
	result := cli.gitKura(repo, "get", "51", "--json")
	requireExitCode(t, result, 0)

	requireConformsToOutputSchema(t, result.stdout)
}

func TestLsNoOpenWorktrees(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	result := cli.gitKura(repo, "ls")
	requireExitCode(t, result, 0)
	requireEmptyStdout(t, result)
	requireEmptyStderr(t, result)
}

func TestLsListsOpenWorktrees(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
	requireExitCode(t, cli.gitKura(repo, "open", "FEAT-1"), 0)

	result := cli.gitKura(repo, "ls")
	requireExitCode(t, result, 0)
	requireEmptyStderr(t, result)
	requireStdoutContainsLine(t, result, "51")
	requireStdoutContainsLine(t, result, "FEAT-1")
}

func TestLsShowsOnlyOpenWorktrees(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	requireExitCode(t, cli.gitKura(repo, "open", "51"), 0)
	requireExitCode(t, cli.gitKura(repo, "open", "52"), 0)
	requireExitCode(t, cli.gitKura(repo, "close", "51"), 0)

	result := cli.gitKura(repo, "ls")
	requireExitCode(t, result, 0)
	requireStdoutContainsLine(t, result, "52")
	requireStdoutNotContainsLine(t, result, "51")
}
