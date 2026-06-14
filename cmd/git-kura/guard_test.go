package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// guardRecordFilePath returns the on-disk guard record location for key in repo.
func guardRecordFilePath(repo, key string) string {
	return filepath.Join(repo, ".git", "kura", "guards", "worktrees", key+".json")
}

func TestGuardAcquireSucceedsInsideWorktree(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	result := cli.gitKura(wt, "guard", "acquire")
	requireExitCode(t, result, 0)
	requireEmptyStdout(t, result)
	requireEmptyStderr(t, result)

	assertPathExists(t, guardRecordFilePath(repo, "key1"))
}

func TestGuardAcquireWritesRecordFields(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	requireExitCode(t, cli.gitKura(wt, "guard", "acquire"), 0)

	record := requireJSONFile(t, guardRecordFilePath(repo, "key1"))
	if record["key"] != "key1" {
		t.Fatalf("record key = %v, want key1", record["key"])
	}
	if _, ok := record["createdAt"].(string); !ok {
		t.Fatalf("record createdAt missing or not a string: %v", record["createdAt"])
	}
	// v0 must not persist updatedAt.
	if _, ok := record["updatedAt"]; ok {
		t.Fatalf("record must not contain updatedAt in v0: %v", record)
	}
	if _, ok := record["worktreePath"].(string); !ok {
		t.Fatalf("record worktreePath missing or not a string: %v", record["worktreePath"])
	}
}

func TestGuardAcquireFromSubdirSharesRecord(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")
	sub := filepath.Join(wt, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// Acquiring from a subdirectory uses the same worktree-derived guard key, so
	// a second acquire from the worktree root conflicts with it.
	requireExitCode(t, cli.gitKura(sub, "guard", "acquire"), 0)
	assertPathExists(t, guardRecordFilePath(repo, "key1"))

	result := cli.gitKura(wt, "guard", "acquire")
	requireExitCode(t, result, exitGuardConflict)
	requireStderrContains(t, result, "guard-active:")
}

func TestGuardAcquireOutsideWorktreeFails(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	// The main checkout is a git repository but not a git-kura managed worktree.
	result := cli.gitKura(repo, "guard", "acquire")
	requireNonZeroExitCode(t, result)
	requireStderrContains(t, result, "managed worktree")
}

func TestGuardReleaseOutsideWorktreeFails(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	result := cli.gitKura(repo, "guard", "release")
	requireNonZeroExitCode(t, result)
	requireStderrContains(t, result, "managed worktree")
}

func TestGuardStatusOutsideWorktreeFails(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	result := cli.gitKura(repo, "guard", "status")
	requireNonZeroExitCode(t, result)
	requireStderrContains(t, result, "managed worktree")
}

func TestGuardAcquireTwiceConflicts(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	requireExitCode(t, cli.gitKura(wt, "guard", "acquire"), 0)

	result := cli.gitKura(wt, "guard", "acquire")
	requireExitCode(t, result, exitGuardConflict)
	requireStderrContains(t, result, "guard-active:")
}

func TestGuardReacquireAfterRelease(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	requireExitCode(t, cli.gitKura(wt, "guard", "acquire"), 0)
	requireExitCode(t, cli.gitKura(wt, "guard", "release"), 0)
	assertPathMissing(t, guardRecordFilePath(repo, "key1"))

	// After release the guard can be acquired again.
	requireExitCode(t, cli.gitKura(wt, "guard", "acquire"), 0)
	assertPathExists(t, guardRecordFilePath(repo, "key1"))
}

func TestGuardReleaseWithoutGuardIsNoOp(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	result := cli.gitKura(wt, "guard", "release")
	requireExitCode(t, result, 0)
	requireStderrContains(t, result, "warning")
}

func TestGuardStatusReportsUnguarded(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	result := cli.gitKura(wt, "guard", "status")
	requireExitCode(t, result, 0)
	requireStdoutContainsLine(t, result, "guarded: false")
	requireStdoutContainsLine(t, result, "key: key1")
	// An unguarded worktree has no createdAt to report.
	if strings.Contains(result.stdout, "createdAt:") {
		t.Fatalf("unguarded status should not print createdAt, got:\n%s", result.stdout)
	}
}

func TestGuardStatusReportsGuarded(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	requireExitCode(t, cli.gitKura(wt, "guard", "acquire"), 0)

	result := cli.gitKura(wt, "guard", "status")
	requireExitCode(t, result, 0)
	requireStdoutContainsLine(t, result, "guarded: true")
	requireStdoutContainsLine(t, result, "key: key1")
	if !strings.Contains(result.stdout, "createdAt:") {
		t.Fatalf("guarded status should print createdAt, got:\n%s", result.stdout)
	}
	// v0 status must not surface updatedAt.
	if strings.Contains(result.stdout, "updatedAt") {
		t.Fatalf("v0 status must not print updatedAt, got:\n%s", result.stdout)
	}
}

// TestGuardAcquireExclusiveCreateBlocksOnExistingRecord verifies acquire relies
// on an exclusive create: any pre-existing record at the guard path, even an
// empty one that was never written by guard acquire, makes acquire fail with
// guard-active rather than overwriting it.
func TestGuardAcquireExclusiveCreateBlocksOnExistingRecord(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	recordPath := guardRecordFilePath(repo, "key1")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	result := cli.gitKura(wt, "guard", "acquire")
	requireExitCode(t, result, exitGuardConflict)
	requireStderrContains(t, result, "guard-active:")
}

// TestGuardAcquireDoesNotAutoDeleteBrokenRecord verifies a corrupted guard
// record blocks acquire and is left untouched (no auto-deletion or rewrite).
func TestGuardAcquireDoesNotAutoDeleteBrokenRecord(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	recordPath := guardRecordFilePath(repo, "key1")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatal(err)
	}
	broken := []byte("{ this is not valid json")
	if err := os.WriteFile(recordPath, broken, 0o644); err != nil {
		t.Fatal(err)
	}

	result := cli.gitKura(wt, "guard", "acquire")
	requireExitCode(t, result, exitGuardConflict)
	requireStderrContains(t, result, "guard-active:")

	// The broken record must remain exactly as it was: not deleted, not rewritten.
	got, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("broken record should still exist: %v", err)
	}
	if string(got) != string(broken) {
		t.Fatalf("broken record was modified: got %q, want %q", got, broken)
	}
}

// TestGuardAcquireConcurrentSingleWinner runs many concurrent acquires against
// the same worktree and asserts exactly one succeeds; the rest exit with
// guard-active (code 8). This exercises the atomic exclusive-create acquire.
func TestGuardAcquireConcurrentSingleWinner(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := cli.openWorktree(t, repo, "key1")

	const n = 8
	var wg sync.WaitGroup
	codes := make([]int, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			codes[i] = cli.gitKura(wt, "guard", "acquire").code
		}(i)
	}
	close(start)
	wg.Wait()

	success, conflict, other := 0, 0, 0
	for _, c := range codes {
		switch c {
		case 0:
			success++
		case exitGuardConflict:
			conflict++
		default:
			other++
		}
	}
	if success != 1 {
		t.Fatalf("want exactly 1 successful acquire, got %d (conflict=%d other=%d codes=%v)", success, conflict, other, codes)
	}
	if other != 0 {
		t.Fatalf("unexpected non-conflict failures: %d (codes=%v)", other, codes)
	}
	if conflict != n-1 {
		t.Fatalf("want %d guard-active conflicts, got %d (codes=%v)", n-1, conflict, codes)
	}
}

// --- in-process tests (exercise the command functions directly) ---

func TestRunGuardDispatch(t *testing.T) {
	if err := runGuard(nil); err == nil {
		t.Fatal("runGuard with no subcommand should error")
	}
	if err := runGuard([]string{"bogus"}); err == nil {
		t.Fatal("runGuard with unknown subcommand should error")
	}
	for _, sub := range []string{"-h", "--help", "acquire", "release", "status"} {
		if err := runGuard([]string{sub, "--help"}); err != nil {
			t.Fatalf("runGuard %q --help: %v", sub, err)
		}
	}
}

func TestRunGuardExecutesSubcommandsInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := openManagedWorktree(t, repo, "key1")

	withWorkingDir(t, wt, func() {
		// Each subcommand is rejected when given an unexpected argument.
		for _, sub := range []string{"acquire", "release", "status"} {
			if err := runGuard([]string{sub, "extra"}); err == nil {
				t.Fatalf("runGuard %q with extra arg should error", sub)
			}
		}

		if err := runGuard([]string{"acquire"}); err != nil {
			t.Fatalf("runGuard acquire: %v", err)
		}
		if _, err := captureStdout(t, func() error { return runGuard([]string{"status"}) }); err != nil {
			t.Fatalf("runGuard status: %v", err)
		}
		if err := runGuard([]string{"release"}); err != nil {
			t.Fatalf("runGuard release: %v", err)
		}
	})
}

func TestParseGuardNoArgs(t *testing.T) {
	if err := parseGuardNoArgs("acquire", nil); err != nil {
		t.Fatalf("parseGuardNoArgs with no args: %v", err)
	}
	if err := parseGuardNoArgs("acquire", []string{"extra"}); err == nil {
		t.Fatal("parseGuardNoArgs should reject an unexpected argument")
	}
}

func TestReadGuardContextOutsideWorktree(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	withWorkingDir(t, repo, func() {
		if _, _, err := readGuardContext(); err == nil {
			t.Fatal("readGuardContext outside a managed worktree should error")
		}
	})
}

func TestReadGuardContextOutsideRepo(t *testing.T) {
	outside := t.TempDir()
	withWorkingDir(t, outside, func() {
		if _, _, err := readGuardContext(); err == nil {
			t.Fatal("readGuardContext outside any git repository should error")
		}
	})
}

func TestMarshalGuardRecordRejectsIncomplete(t *testing.T) {
	if _, err := marshalGuardRecord(guardRecord{Key: "key1"}); err == nil {
		t.Fatal("marshalGuardRecord should reject a record missing worktreePath/createdAt")
	}
	data, err := marshalGuardRecord(guardRecord{Key: "key1", WorktreePath: "/wt", CreatedAt: "2026-06-13T00:00:00Z"})
	if err != nil {
		t.Fatalf("marshalGuardRecord valid record: %v", err)
	}
	if !strings.Contains(string(data), `"key":"key1"`) {
		t.Fatalf("marshalled record missing key: %s", data)
	}
}

func TestCmdGuardLifecycleInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := openManagedWorktree(t, repo, "key1")

	withWorkingDir(t, wt, func() {
		if err := cmdGuardAcquire(); err != nil {
			t.Fatalf("cmdGuardAcquire: %v", err)
		}

		// A second acquire conflicts with exit code 8.
		err := cmdGuardAcquire()
		var xe *exitError
		if !errors.As(err, &xe) || xe.code != exitGuardConflict {
			t.Fatalf("second cmdGuardAcquire exit code = %v, want %d (err: %v)", xe, exitGuardConflict, err)
		}
		if !strings.Contains(err.Error(), "guard-active:") {
			t.Fatalf("conflict error should include stable token: %v", err)
		}

		out, statusErr := captureStdout(t, cmdGuardStatus)
		if statusErr != nil {
			t.Fatalf("cmdGuardStatus: %v", statusErr)
		}
		if !strings.Contains(out, "guarded: true") || !strings.Contains(out, "key: key1") || !strings.Contains(out, "createdAt:") {
			t.Fatalf("guarded status output unexpected:\n%s", out)
		}

		if err := cmdGuardRelease(); err != nil {
			t.Fatalf("cmdGuardRelease: %v", err)
		}

		out, statusErr = captureStdout(t, cmdGuardStatus)
		if statusErr != nil {
			t.Fatalf("cmdGuardStatus after release: %v", statusErr)
		}
		if !strings.Contains(out, "guarded: false") || strings.Contains(out, "createdAt:") {
			t.Fatalf("unguarded status output unexpected:\n%s", out)
		}

		// Release with no guard present is a no-op.
		if err := cmdGuardRelease(); err != nil {
			t.Fatalf("cmdGuardRelease no-op: %v", err)
		}
	})
}

func TestCmdGuardStatusReadsCreatedAtFromRecord(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := openManagedWorktree(t, repo, "key1")

	// Seed a record with a known createdAt and confirm status surfaces it.
	recordPath := guardRecordFilePath(repo, "key1")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordPath, []byte(`{"key":"key1","worktreePath":"`+wt+`","createdAt":"2026-06-13T00:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	withWorkingDir(t, wt, func() {
		out, err := captureStdout(t, cmdGuardStatus)
		if err != nil {
			t.Fatalf("cmdGuardStatus: %v", err)
		}
		if !strings.Contains(out, "createdAt: 2026-06-13T00:00:00Z") {
			t.Fatalf("status should surface stored createdAt:\n%s", out)
		}
	})
}

// TestCmdGuardStatusAndReleaseSurfaceIOErrors verifies that an I/O error other
// than "record absent" is reported rather than mistaken for an unguarded
// worktree. A directory at the record path makes the read and the remove fail.
func TestCmdGuardStatusAndReleaseSurfaceIOErrors(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := openManagedWorktree(t, repo, "key1")

	recordPath := guardRecordFilePath(repo, "key1")
	// A non-empty directory at the record path: ReadFile fails (not NotExist)
	// and Remove fails (directory not empty), exercising both error branches.
	if err := os.MkdirAll(filepath.Join(recordPath, "child"), 0o755); err != nil {
		t.Fatal(err)
	}

	withWorkingDir(t, wt, func() {
		if err := cmdGuardStatus(); err == nil {
			t.Fatal("cmdGuardStatus should error when the record cannot be read")
		}
		if err := cmdGuardRelease(); err == nil {
			t.Fatal("cmdGuardRelease should error when the record cannot be removed")
		}
	})
}

// TestCmdGuardAcquireSurfacesStoreDirError verifies acquire reports a failure
// to create its store directory rather than proceeding. A regular file where
// the guards directory belongs makes MkdirAll fail.
func TestCmdGuardAcquireSurfacesStoreDirError(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := openManagedWorktree(t, repo, "key1")

	guardsDir := filepath.Dir(filepath.Dir(guardRecordFilePath(repo, "key1"))) // <common>/kura/guards
	if err := os.MkdirAll(filepath.Dir(guardsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guardsDir, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	withWorkingDir(t, wt, func() {
		if err := cmdGuardAcquire(); err == nil {
			t.Fatal("cmdGuardAcquire should error when the guard store dir cannot be created")
		}
	})
}

// TestCmdGuardAcquireSurfacesRecordCreateError verifies an exclusive-create
// failure that is not a pre-existing record (here, a permission error in a
// read-only store dir) is reported as an error and not mistaken for a conflict.
func TestCmdGuardAcquireSurfacesRecordCreateError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission tests are Unix-specific")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root: permission restrictions don't apply")
	}
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := openManagedWorktree(t, repo, "key1")

	worktreesDir := filepath.Dir(guardRecordFilePath(repo, "key1")) // <common>/kura/guards/worktrees
	if err := os.MkdirAll(worktreesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(worktreesDir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(worktreesDir, 0o755) }()

	withWorkingDir(t, wt, func() {
		err := cmdGuardAcquire()
		if err == nil {
			t.Fatal("cmdGuardAcquire should error when the record cannot be created")
		}
		// A permission error must not be reported as a guard conflict.
		var xe *exitError
		if errors.As(err, &xe) && xe.code == exitGuardConflict {
			t.Fatalf("permission error must not be reported as guard-active: %v", err)
		}
	})
}

func TestCmdGuardStatusToleratesBrokenRecord(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := openManagedWorktree(t, repo, "key1")

	recordPath := guardRecordFilePath(repo, "key1")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordPath, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	withWorkingDir(t, wt, func() {
		out, err := captureStdout(t, cmdGuardStatus)
		if err != nil {
			t.Fatalf("cmdGuardStatus with broken record: %v", err)
		}
		// Still reports guarded:true with derived key, just no createdAt.
		if !strings.Contains(out, "guarded: true") || !strings.Contains(out, "key: key1") {
			t.Fatalf("broken-record status output unexpected:\n%s", out)
		}
		if strings.Contains(out, "createdAt:") {
			t.Fatalf("broken-record status should omit createdAt:\n%s", out)
		}
	})
}
