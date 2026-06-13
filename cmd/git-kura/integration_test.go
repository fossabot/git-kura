package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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

// --- seal claim / unclaim integration tests ---

func TestSealClaimOutsideWorktreeFails(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	// The main checkout is a git repository but not a git-kura managed worktree.
	result := cli.gitKura(repo, "seal", "claim", "tracked.txt")
	requireNonZeroExitCode(t, result)
	requireStderrContains(t, result, "managed worktree")
}

func TestSealUnclaimOutsideWorktreeFails(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	result := cli.gitKura(repo, "seal", "unclaim", "tracked.txt")
	requireNonZeroExitCode(t, result)
	requireStderrContains(t, result, "managed worktree")
}

func TestSealClaimSucceeds(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	result := cli.gitKura(wt, "seal", "claim", "tracked.txt")
	requireExitCode(t, result, 0)
	requireEmptyStdout(t, result)
	requireEmptyStderr(t, result)
}

func TestSealClaimIsIdempotent(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	requireExitCode(t, cli.gitKura(wt, "seal", "claim", "tracked.txt"), 0)
	result := cli.gitKura(wt, "seal", "claim", "tracked.txt")
	requireExitCode(t, result, 0)
}

func TestSealClaimRejectsDifferentKey(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt1 := cli.openWorktree(t, repo, "key1")
	wt2 := cli.openWorktree(t, repo, "key2")

	requireExitCode(t, cli.gitKura(wt1, "seal", "claim", "tracked.txt"), 0)

	// The lock is NOT held here: the rejection below is purely a cross-key
	// seal conflict, not lock contention.
	requireNoSealLock(t, repo)

	result := cli.gitKura(wt2, "seal", "claim", "tracked.txt")
	requireExitCode(t, result, exitSealConflict)
	requireStderrContains(t, result, "seal-conflict:")
	requireStderrContains(t, result, "key1")
}

func TestSealUnclaimRejectsDifferentKey(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt1 := cli.openWorktree(t, repo, "key1")
	wt2 := cli.openWorktree(t, repo, "key2")

	requireExitCode(t, cli.gitKura(wt1, "seal", "claim", "tracked.txt"), 0)

	// The lock is NOT held here: the rejection below is purely a cross-key
	// seal conflict, not lock contention.
	requireNoSealLock(t, repo)

	result := cli.gitKura(wt2, "seal", "unclaim", "tracked.txt")
	requireExitCode(t, result, exitSealConflict)
	requireStderrContains(t, result, "seal-conflict:")
	requireStderrContains(t, result, "key1")
}

func TestSealClaimConflictListsAllSealedPaths(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	commitFile(t, repo, "second.txt", "content\n")
	wt1 := cli.openWorktree(t, repo, "key1")
	wt2 := cli.openWorktree(t, repo, "key2")
	wt3 := cli.openWorktree(t, repo, "key3")

	requireExitCode(t, cli.gitKura(wt1, "seal", "claim", "tracked.txt"), 0)
	requireExitCode(t, cli.gitKura(wt2, "seal", "claim", "second.txt"), 0)
	requireNoSealLock(t, repo)

	result := cli.gitKura(wt3, "seal", "claim", "tracked.txt", "second.txt")
	requireExitCode(t, result, exitSealConflict)
	requireStderrContains(t, result, "tracked.txt")
	requireStderrContains(t, result, "key1")
	requireStderrContains(t, result, "second.txt")
	requireStderrContains(t, result, "key2")
}

func TestSealClaimRejectsNonExistentFile(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	result := cli.gitKura(wt, "seal", "claim", "nosuchfile.txt")
	requireNonZeroExitCode(t, result)
	requireStderrContains(t, result, "nosuchfile.txt")
}

func TestSealClaimResolvesPathsFromRepoRootNotCwd(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt1 := cli.openWorktree(t, repo, "key1")
	wt2 := cli.openWorktree(t, repo, "key2")
	sub := filepath.Join(wt1, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// Run from a subdirectory of the worktree: "tracked.txt" must resolve to the
	// file at the worktree root, not sub/tracked.txt (which does not exist). The
	// current key is still derived from the worktree, not the subdirectory.
	requireExitCode(t, cli.gitKura(sub, "seal", "claim", "tracked.txt"), 0)

	// The path sealed from the subdirectory is the root file: a different key
	// is rejected when targeting it from another worktree.
	result := cli.gitKura(wt2, "seal", "claim", "tracked.txt")
	requireExitCode(t, result, exitSealConflict)
	requireStderrContains(t, result, "key1")
}

func TestSealClaimRejectsAbsolutePath(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	absPath := filepath.Join(wt, "tracked.txt")
	result := cli.gitKura(wt, "seal", "claim", absPath)
	requireNonZeroExitCode(t, result)
}

func TestSealClaimRejectsPathOutsideRepo(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	result := cli.gitKura(wt, "seal", "claim", "../outside.txt")
	requireNonZeroExitCode(t, result)
}

// sealLockFilePath resolves the seal store lock file path for a repository.
func sealLockFilePath(t *testing.T, repo string) string {
	t.Helper()
	commonDir := strings.TrimSpace(git(t, repo, "rev-parse", "--git-common-dir"))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(repo, commonDir)
	}
	return filepath.Join(commonDir, "kura", "seals", "paths.lock")
}

// requireNoSealLock asserts the seal store lock is not held, so a subsequent
// failure cannot be caused by lock contention.
func requireNoSealLock(t *testing.T, repo string) {
	t.Helper()
	lockPath := sealLockFilePath(t, repo)
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("seal store lock %s should not exist (stat err: %v)", lockPath, err)
	}
}

func TestSealClaimLockTimeout(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	// Hold the lock manually.
	lockPath := sealLockFilePath(t, repo)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(lockPath) }()

	env := filterEnv(append(os.Environ(), "PATH="+cli.envPath), "GIT_KURA_SEAL_LOCK_TIMEOUT")
	env = append(env, "GIT_KURA_SEAL_LOCK_TIMEOUT=100ms")
	cmd := exec.Command("git", "kura", "seal", "claim", "tracked.txt")
	cmd.Dir = wt
	cmd.Env = env
	result := runCommand(cmd)

	requireExitCode(t, result, exitSealLockTimeout)
	requireStderrContains(t, result, "seal-lock-timeout:")
}

func TestSealUnclaimIsIdempotentWhenNotSealed(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	result := cli.gitKura(wt, "seal", "unclaim", "tracked.txt")
	requireExitCode(t, result, 0)
}

func TestSealUnclaimRemovesPath(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt1 := cli.openWorktree(t, repo, "key1")
	wt2 := cli.openWorktree(t, repo, "key2")

	requireExitCode(t, cli.gitKura(wt1, "seal", "claim", "tracked.txt"), 0)

	result := cli.gitKura(wt1, "seal", "unclaim", "tracked.txt")
	requireExitCode(t, result, 0)

	// After removal, a different key can seal the same path
	result2 := cli.gitKura(wt2, "seal", "claim", "tracked.txt")
	requireExitCode(t, result2, 0)
}

func TestSealClaimMultiplePaths(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	commitFile(t, repo, "second.txt", "content\n")
	wt := cli.openWorktree(t, repo, "key1")

	requireExitCode(t, cli.gitKura(wt, "seal", "claim", "tracked.txt"), 0)
	requireExitCode(t, cli.gitKura(wt, "seal", "claim", "second.txt"), 0)
}

func TestSealClaimWorksAcrossWorktrees(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt51 := cli.openWorktree(t, repo, "51")
	wt52 := cli.openWorktree(t, repo, "52")

	// tracked.txt is committed and present in every worktree.
	result := cli.gitKura(wt51, "seal", "claim", "tracked.txt")
	requireExitCode(t, result, 0)

	// The shared store prevents a different worktree's key from sealing the
	// same path.
	result2 := cli.gitKura(wt52, "seal", "claim", "tracked.txt")
	requireNonZeroExitCode(t, result2)
	requireStderrContains(t, result2, "51")
}

func TestSealClaimMissingPathArg(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	result := cli.gitKura(repo, "seal", "claim")
	requireNonZeroExitCode(t, result)
}

func TestSealUnclaimMissingPathArg(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	result := cli.gitKura(repo, "seal", "unclaim")
	requireNonZeroExitCode(t, result)
}

// --- removed seal add / remove subcommand integration tests ---

// TestSealAddSubcommandRemoved asserts the old "seal add" verb is gone: it is
// not an alias of "claim", it is simply an unknown subcommand.
func TestSealAddSubcommandRemoved(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	result := cli.gitKura(wt, "seal", "add", "tracked.txt")
	requireNonZeroExitCode(t, result)
	requireStderrContains(t, result, "unknown seal subcommand")

	// Nothing was written to the store.
	ls := cli.gitKura(repo, "seal", "ls")
	requireExitCode(t, ls, 0)
	requireEmptyStdout(t, ls)
}

// TestSealRemoveSubcommandRemoved asserts the old "seal remove" verb is gone.
func TestSealRemoveSubcommandRemoved(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	result := cli.gitKura(wt, "seal", "remove", "tracked.txt")
	requireNonZeroExitCode(t, result)
	requireStderrContains(t, result, "unknown seal subcommand")
}

// --- seal ls integration tests ---

func TestSealLsListsAllKeysIgnoringCurrentWorktree(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	commitFile(t, repo, "second.txt", "content\n")
	wt1 := cli.openWorktree(t, repo, "key1")
	wt2 := cli.openWorktree(t, repo, "key2")

	requireExitCode(t, cli.gitKura(wt2, "seal", "claim", "second.txt"), 0)
	requireExitCode(t, cli.gitKura(wt1, "seal", "claim", "tracked.txt"), 0)

	// ls is repository-wide: the current worktree's key must not narrow the
	// output, even when ls is run from inside a managed worktree.
	result := cli.gitKura(wt1, "seal", "ls")
	requireExitCode(t, result, 0)
	requireEmptyStderr(t, result)
	if want := "key1\ttracked.txt\nkey2\tsecond.txt\n"; result.stdout != want {
		t.Fatalf("stdout = %q, want %q", result.stdout, want)
	}
}

func TestSealLsFiltersByKeyArgument(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	commitFile(t, repo, "second.txt", "content\n")
	wt1 := cli.openWorktree(t, repo, "key1")
	wt2 := cli.openWorktree(t, repo, "key2")

	requireExitCode(t, cli.gitKura(wt1, "seal", "claim", "tracked.txt"), 0)
	requireExitCode(t, cli.gitKura(wt2, "seal", "claim", "second.txt"), 0)

	result := cli.gitKura(repo, "seal", "ls", "key2")
	requireExitCode(t, result, 0)
	if want := "key2\tsecond.txt\n"; result.stdout != want {
		t.Fatalf("stdout = %q, want %q", result.stdout, want)
	}
}

func TestSealLsSeesStoreFromWorktree(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "51")

	requireExitCode(t, cli.gitKura(wt, "seal", "claim", "tracked.txt"), 0)

	// The store is shared via the git common dir, so ls shows the same
	// repository-wide result from the main checkout and from the worktree.
	for _, dir := range []string{repo, wt} {
		result := cli.gitKura(dir, "seal", "ls")
		requireExitCode(t, result, 0)
		if want := "51\ttracked.txt\n"; result.stdout != want {
			t.Fatalf("stdout in %s = %q, want %q", dir, result.stdout, want)
		}
	}
}

func TestSealLsEmptyStoreSucceeds(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	result := cli.gitKura(repo, "seal", "ls")
	requireExitCode(t, result, 0)
	requireEmptyStdout(t, result)
	requireEmptyStderr(t, result)
}

func TestSealLsHelpFlag(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	result := cli.gitKura(repo, "seal", "ls", "--help")
	requireExitCode(t, result, 0)
	if !strings.Contains(result.stdout, "Usage: git kura seal ls [key]") {
		t.Fatalf("help output = %s, want usage line", result.stdout)
	}
}

func TestSealClaimHelpFlag(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	result := cli.gitKura(repo, "seal", "claim", "--help")
	requireExitCode(t, result, 0)
	if !strings.Contains(result.stdout, "managed worktree") {
		t.Fatalf("help output should describe worktree-derived key resolution: %s", result.stdout)
	}
}

func TestSealUnclaimHelpFlag(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	result := cli.gitKura(repo, "seal", "unclaim", "--help")
	requireExitCode(t, result, 0)
	if !strings.Contains(result.stdout, "managed worktree") {
		t.Fatalf("help output should describe worktree-derived key resolution: %s", result.stdout)
	}
}
