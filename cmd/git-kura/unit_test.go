package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

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

	t.Run("--root produces root output mode", func(t *testing.T) {
		_, opts, err := parseGetArgs([]string{"51", "--root"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.OutputMode != outputRoot {
			t.Fatalf("OutputMode = %q, want %q", opts.OutputMode, outputRoot)
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
		{"51", "--path", "--path"},
		{"51", "--path", "--branch"},
		{"51", "--path", "--root"},
		{"51", "--root", "--path"},
		{"51", "--root", "--branch"},
		{"51", "--root", "--json"},
		{"51", "--root", "--toon"},
		{"51", "--json", "--toon"},
		{"51", "--path", "--json"},
		{"51", "--branch", "--json"},
		{"51", "--path", "--format", "json"},
		{"51", "--branch", "--format", "toon"},
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

func TestParseOpenArgs(t *testing.T) {
	t.Run("valid key succeeds", func(t *testing.T) {
		key, opts, err := parseOpenArgs([]string{"51"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "51" {
			t.Fatalf("key = %q, want 51", key)
		}
		if opts.DryRun {
			t.Fatal("DryRun = true, want false")
		}
	})

	t.Run("dry-run flag", func(t *testing.T) {
		key, opts, err := parseOpenArgs([]string{"51", "--dry-run"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "51" {
			t.Fatalf("key = %q, want 51", key)
		}
		if !opts.DryRun {
			t.Fatal("DryRun = false, want true")
		}
	})

	t.Run("no key is usage error", func(t *testing.T) {
		_, _, err := parseOpenArgs([]string{})
		if err == nil {
			t.Fatal("expected error for missing key, got nil")
		}
	})

	t.Run("extra argument is error", func(t *testing.T) {
		_, _, err := parseOpenArgs([]string{"51", "--extra"})
		if err == nil {
			t.Fatal("expected error for extra argument, got nil")
		}
	})

	t.Run("invalid key is error", func(t *testing.T) {
		_, _, err := parseOpenArgs([]string{"../x"})
		if err == nil {
			t.Fatal("expected error for invalid key, got nil")
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

func TestPrintJSONRejectsInvalidData(t *testing.T) {
	if err := printJSON(worktreeJSON{}); err == nil {
		t.Fatal("printJSON invalid data error = nil, want error")
	}
}

func TestPrintTOONFormat(t *testing.T) {
	data := worktreeJSON{
		SchemaVersion:  1,
		Key:            "test-51",
		Kind:           "worktree",
		Branch:         "test-51",
		WorktreePath:   "/repo/.git/kura/worktrees/test-51",
		RepositoryRoot: "/repo",
		BaseBranch:     "main",
		Exists:         true,
		Dirty:          false,
	}

	stdout, err := captureStdout(t, func() error { return printTOON(data) })
	if err != nil {
		t.Fatalf("printTOON error = %v", err)
	}

	for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		if !strings.Contains(line, ": ") {
			t.Errorf("line %q does not use ': ' separator", line)
		}
	}
}

func TestPrintTOONFields(t *testing.T) {
	data := worktreeJSON{
		SchemaVersion:  1,
		Key:            "test-51",
		Kind:           "worktree",
		Branch:         "test-51",
		WorktreePath:   "/repo/.git/kura/worktrees/test-51",
		RepositoryRoot: "/repo",
		BaseBranch:     "main",
		Exists:         true,
		Dirty:          false,
	}

	stdout, err := captureStdout(t, func() error { return printTOON(data) })
	if err != nil {
		t.Fatalf("printTOON error = %v", err)
	}

	for field, want := range map[string]string{
		"schemaVersion":  "schemaVersion: 1",
		"key":            "key: test-51",
		"kind":           "kind: worktree",
		"branch":         "branch: test-51",
		"worktreePath":   "worktreePath: /repo/.git/kura/worktrees/test-51",
		"repositoryRoot": "repositoryRoot: /repo",
		"baseBranch":     "baseBranch: main",
		"exists":         "exists: true",
		"dirty":          "dirty: false",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("field %q: stdout does not contain %q\nfull output:\n%s", field, want, stdout)
		}
	}

	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 9 {
		t.Errorf("line count = %d, want 9\nfull output:\n%s", len(lines), stdout)
	}
}

func TestRequireCleanValueStdoutAcceptsWindowsPath(t *testing.T) {
	requireCleanValueStdout(t, cliResult{stdout: `C:\repo.kura\worktrees\51` + "\n"})
}

func TestParseSealEnterArgs(t *testing.T) {
	t.Run("valid key with no command", func(t *testing.T) {
		a, err := parseSealEnterArgs([]string{"issue-12"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.Key != "issue-12" {
			t.Fatalf("Key = %q, want %q", a.Key, "issue-12")
		}
		if len(a.Command) != 0 {
			t.Fatalf("Command = %v, want empty", a.Command)
		}
	})

	t.Run("-- command returns key and command", func(t *testing.T) {
		a, err := parseSealEnterArgs([]string{"issue-12", "--", "echo", "hi"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.Key != "issue-12" {
			t.Fatalf("Key = %q, want %q", a.Key, "issue-12")
		}
		if len(a.Command) != 2 || a.Command[0] != "echo" || a.Command[1] != "hi" {
			t.Fatalf("Command = %v, want [echo hi]", a.Command)
		}
	})

	t.Run("no key is usage error", func(t *testing.T) {
		_, err := parseSealEnterArgs([]string{})
		if err == nil {
			t.Fatal("expected error for missing key, got nil")
		}
	})

	t.Run("invalid key is error", func(t *testing.T) {
		_, err := parseSealEnterArgs([]string{"../x"})
		if err == nil {
			t.Fatal("expected error for invalid key, got nil")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "key") {
			t.Fatalf("error %q does not mention key", err.Error())
		}
	})

	t.Run("extra argument without -- is error", func(t *testing.T) {
		_, err := parseSealEnterArgs([]string{"issue-12", "extra"})
		if err == nil {
			t.Fatal("expected error for extra argument, got nil")
		}
	})

	t.Run("-- with no command is error", func(t *testing.T) {
		_, err := parseSealEnterArgs([]string{"issue-12", "--"})
		if err == nil {
			t.Fatal("expected error for -- with no command, got nil")
		}
	})
}

func TestArgsBeforeDoubleDash(t *testing.T) {
	for _, tc := range []struct {
		input []string
		want  []string
	}{
		{input: []string{}, want: []string{}},
		{input: []string{"a", "b"}, want: []string{"a", "b"}},
		{input: []string{"a", "--", "b"}, want: []string{"a"}},
		{input: []string{"--", "b"}, want: []string{}},
		{input: []string{"a", "--", "--help"}, want: []string{"a"}},
	} {
		got := argsBeforeDoubleDash(tc.input)
		if len(got) != len(tc.want) {
			t.Fatalf("argsBeforeDoubleDash(%v) = %v, want %v", tc.input, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("argsBeforeDoubleDash(%v)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

func TestParseSealCurrentArgs(t *testing.T) {
	t.Run("no args succeeds", func(t *testing.T) {
		if err := parseSealCurrentArgs([]string{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("extra argument is error", func(t *testing.T) {
		if err := parseSealCurrentArgs([]string{"extra"}); err == nil {
			t.Fatal("expected error for extra argument, got nil")
		}
	})
}

func TestCmdSealCurrentPrintsKey(t *testing.T) {
	t.Setenv("GIT_KURA_SEAL_KEY", "test-key-123")
	stdout, err := captureStdout(t, cmdSealCurrent)
	if err != nil {
		t.Fatalf("cmdSealCurrent error = %v, want nil", err)
	}
	if strings.TrimSpace(stdout) != "test-key-123" {
		t.Fatalf("stdout = %q, want %q", stdout, "test-key-123")
	}
}

func TestCmdSealCurrentFailsWhenUnset(t *testing.T) {
	t.Setenv("GIT_KURA_SEAL_KEY", "")
	if err := cmdSealCurrent(); err == nil {
		t.Fatal("cmdSealCurrent error = nil, want error when GIT_KURA_SEAL_KEY is not set")
	}
}

func TestDetectShellUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell detection is not tested on Windows")
	}

	t.Run("uses SHELL env var when set", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/myshell")
		if got := detectShell(); got != "/bin/myshell" {
			t.Fatalf("detectShell() = %q, want %q", got, "/bin/myshell")
		}
	})

	t.Run("falls back to sh when SHELL not set", func(t *testing.T) {
		t.Setenv("SHELL", "")
		if got := detectShell(); got != "sh" {
			t.Fatalf("detectShell() = %q, want sh", got)
		}
	})
}

// deadPIDForTest starts a short-lived process and returns its PID after it
// finishes.  The PID is guaranteed dead for the duration of the test (barring
// unlikely OS PID reuse).
func deadPIDForTest(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("deadPIDForTest: %v", err)
	}
	return cmd.Process.Pid
}

// writeSealSessionFile writes a session JSON file at sealSessionPath(sessDir, sess.WorktreePath)
// and registers a cleanup to remove it after the test.
func writeSealSessionFile(t *testing.T, sessDir string, sess sealSession) {
	t.Helper()
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := sealSessionPath(sessDir, sess.WorktreePath)
	data, _ := json.Marshal(sess)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(path) }) //nolint:errcheck
}

func TestAcquireSealSessionSucceeds(t *testing.T) {
	dir := t.TempDir()

	path, _, err := acquireSealSession(dir, "/wt-a", "key1", os.Getpid())
	if err != nil {
		t.Fatalf("acquireSealSession error = %v, want nil", err)
	}
	t.Cleanup(func() { os.Remove(path) }) //nolint:errcheck

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("session file missing after acquire: %v", err)
	}
}

func TestAcquireSealSessionDifferentWorktreesDoNotConflict(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	// Two different session directories (simulating different repos) must not conflict.
	pathA, _, err := acquireSealSession(dirA, "/wt-a", "key1", os.Getpid())
	if err != nil {
		t.Fatalf("acquire for wt-a error = %v", err)
	}
	t.Cleanup(func() { os.Remove(pathA) }) //nolint:errcheck

	pathB, _, err := acquireSealSession(dirB, "/wt-b", "key2", os.Getpid())
	if err != nil {
		t.Fatalf("acquire for wt-b error = %v", err)
	}
	t.Cleanup(func() { os.Remove(pathB) }) //nolint:errcheck
}

func TestAcquireSealSessionConflictDifferentKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PID liveness uses kill(0) which is Unix-specific")
	}

	dir := t.TempDir()
	writeSealSessionFile(t, dir, sealSession{
		Key: "key2", WorktreePath: "/conflict-wt",
		ParentPID: os.Getpid(), StartedAt: time.Now(),
	})

	_, _, err := acquireSealSession(dir, "/conflict-wt", "key1", os.Getpid())
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "key2") {
		t.Fatalf("error %q does not mention conflicting key", err.Error())
	}
}

func TestAcquireSealSessionConflictSameKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PID liveness uses kill(0) which is Unix-specific")
	}

	dir := t.TempDir()
	writeSealSessionFile(t, dir, sealSession{
		Key: "key1", WorktreePath: "/same-key-wt",
		ParentPID: os.Getpid(), StartedAt: time.Now(),
	})

	_, _, err := acquireSealSession(dir, "/same-key-wt", "key1", os.Getpid())
	if err == nil {
		t.Fatal("expected conflict error for same-key active session, got nil")
	}
	if !strings.Contains(err.Error(), "key1") {
		t.Fatalf("error %q does not mention key", err.Error())
	}
}

func TestAcquireSealSessionTTLWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PID liveness uses kill(0) which is Unix-specific")
	}

	dir := t.TempDir()
	writeSealSessionFile(t, dir, sealSession{
		Key: "old-key", WorktreePath: "/ttl-wt",
		ParentPID: os.Getpid(),
		StartedAt: time.Now().Add(-10 * time.Minute),
	})

	_, _, err := acquireSealSession(dir, "/ttl-wt", "new-key", os.Getpid())
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "ttl") {
		t.Fatalf("error %q does not mention TTL", err.Error())
	}
}

func TestCmdSealEnterFailsOutsideGitRepo(t *testing.T) {
	withWorkingDir(t, t.TempDir(), func() {
		if err := cmdSealEnter(sealEnterArgs{Key: "key1", Command: []string{"true"}}); err == nil {
			t.Fatal("cmdSealEnter outside git repo error = nil, want error")
		}
	})
}

func TestSessionAliveDeadChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PID liveness uses kill(0) which is Unix-specific")
	}
	deadPID := deadPIDForTest(t)
	sess := sealSession{ParentPID: os.Getpid(), ChildPID: deadPID}
	if sessionAlive(sess) {
		t.Fatal("sessionAlive = true for dead child PID, want false")
	}
}

func TestAcquireSealSessionCorruptFile(t *testing.T) {
	dir := t.TempDir()
	finalPath := sealSessionPath(dir, "/wt-corrupt")
	if err := os.WriteFile(finalPath, []byte("not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := acquireSealSession(dir, "/wt-corrupt", "key", os.Getpid())
	if err == nil {
		t.Fatal("corrupt session file: error = nil, want error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "corrupt") {
		t.Fatalf("error %q does not mention 'corrupt'", err.Error())
	}
}

func TestPidAliveZeroPid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific zero-PID test")
	}
	if pidAlive(0) {
		t.Fatal("pidAlive(0) = true, want false")
	}
	if pidAlive(-1) {
		t.Fatal("pidAlive(-1) = true, want false")
	}
}

func TestSealSessionDirOutsideRepo(t *testing.T) {
	_, err := sealSessionDir(t.TempDir())
	if err == nil {
		t.Fatal("sealSessionDir outside git repo: error = nil, want error")
	}
}

func TestAcquireSealSessionReadFileError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks may require elevated privileges on Windows")
	}
	dir := t.TempDir()
	finalPath := sealSessionPath(dir, "/wt-broken-link")
	// Broken symlink: directory entry exists (causes EEXIST for Link)
	// but os.ReadFile follows the symlink and gets an error (target absent).
	if err := os.Symlink(finalPath+".gone", finalPath); err != nil {
		t.Fatal(err)
	}
	_, _, err := acquireSealSession(dir, "/wt-broken-link", "key", os.Getpid())
	if err == nil {
		t.Fatal("broken symlink at session path: error = nil, want error")
	}
}

func TestAcquireSealSessionMkdirAllFails(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can always mkdir; skip permission test")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o755) }) //nolint:errcheck
	sessDir := filepath.Join(parent, "sessions")
	_, _, err := acquireSealSession(sessDir, "/wt", "key", os.Getpid())
	if err == nil {
		t.Fatal("MkdirAll in unwritable parent: error = nil, want error")
	}
}

func TestAcquireSealSessionWriteTmpFails(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can always write; skip permission test")
	}
	sessDir := t.TempDir()
	if err := os.Chmod(sessDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(sessDir, 0o755) }) //nolint:errcheck
	_, _, err := acquireSealSession(sessDir, "/wt", "key", os.Getpid())
	if err == nil {
		t.Fatal("WriteFile in non-writable sessDir: error = nil, want error")
	}
}

func TestAcquireSealSessionStaleReportsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PID liveness uses kill(0) which is Unix-specific")
	}

	dir := t.TempDir()
	deadPID := deadPIDForTest(t)

	writeSealSessionFile(t, dir, sealSession{
		Key: "key2", WorktreePath: "/stale-wt",
		ParentPID: deadPID, StartedAt: time.Now(),
	})
	stalePath := sealSessionPath(dir, "/stale-wt")

	_, _, err := acquireSealSession(dir, "/stale-wt", "key1", os.Getpid())
	if err == nil {
		t.Fatal("expected stale-session error, got nil")
	}
	// Error must include the file path so users know what to delete manually.
	if !strings.Contains(err.Error(), stalePath) {
		t.Fatalf("error %q does not contain session file path %q", err.Error(), stalePath)
	}
	// Stale file must not have been deleted automatically.
	if _, statErr := os.Stat(stalePath); statErr != nil {
		t.Fatalf("stale session file was unexpectedly deleted: %v", statErr)
	}
}

func TestUpdateSealSessionRenameFails(t *testing.T) {
	dir := t.TempDir()
	// Place a directory at the target path so os.Rename(tmp, path) fails.
	path := filepath.Join(dir, "session")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	sess := sealSession{Key: "key", WorktreePath: "/wt", ParentPID: os.Getpid(), StartedAt: time.Now()}
	if err := updateSealSession(path, sess); err == nil {
		t.Fatal("updateSealSession with path=directory: error = nil, want error")
	}
}

func TestSessionStatusStr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PID liveness uses kill(0) which is Unix-specific")
	}

	ttl := 5 * time.Minute

	t.Run("active when pids alive and within ttl", func(t *testing.T) {
		sess := sealSession{ParentPID: os.Getpid(), ChildPID: os.Getpid(), StartedAt: time.Now()}
		if got := sessionStatusStr(sess, ttl); got != sessionStatusActive {
			t.Fatalf("sessionStatusStr = %q, want %q", got, sessionStatusActive)
		}
	})

	t.Run("stale-candidate when pids alive but ttl exceeded", func(t *testing.T) {
		sess := sealSession{
			ParentPID: os.Getpid(), ChildPID: os.Getpid(),
			StartedAt: time.Now().Add(-10 * time.Minute),
		}
		if got := sessionStatusStr(sess, ttl); got != sessionStatusStaleCandidate {
			t.Fatalf("sessionStatusStr = %q, want %q", got, sessionStatusStaleCandidate)
		}
	})

	t.Run("stale when parent pid dead", func(t *testing.T) {
		dead := deadPIDForTest(t)
		sess := sealSession{ParentPID: dead, StartedAt: time.Now()}
		if got := sessionStatusStr(sess, ttl); got != sessionStatusStale {
			t.Fatalf("sessionStatusStr = %q, want %q", got, sessionStatusStale)
		}
	})

	t.Run("unknown when child pid not yet recorded", func(t *testing.T) {
		sess := sealSession{ParentPID: os.Getpid(), ChildPID: 0, StartedAt: time.Now()}
		if got := sessionStatusStr(sess, ttl); got != sessionStatusUnknown {
			t.Fatalf("sessionStatusStr = %q, want %q", got, sessionStatusUnknown)
		}
	})
}

func TestSessionTTLEnvVar(t *testing.T) {
	t.Run("valid duration is used", func(t *testing.T) {
		t.Setenv("GIT_KURA_SESSION_TTL", "10m")
		if got := sessionTTL(); got != 10*time.Minute {
			t.Fatalf("sessionTTL() = %v, want 10m", got)
		}
	})

	t.Run("invalid duration falls back to default", func(t *testing.T) {
		t.Setenv("GIT_KURA_SESSION_TTL", "notaduration")
		if got := sessionTTL(); got != defaultSessionTTL {
			t.Fatalf("sessionTTL() = %v, want %v (default)", got, defaultSessionTTL)
		}
	})

	t.Run("unset falls back to default", func(t *testing.T) {
		t.Setenv("GIT_KURA_SESSION_TTL", "")
		if got := sessionTTL(); got != defaultSessionTTL {
			t.Fatalf("sessionTTL() = %v, want %v (default)", got, defaultSessionTTL)
		}
	})

	t.Run("zero duration falls back to default", func(t *testing.T) {
		t.Setenv("GIT_KURA_SESSION_TTL", "0s")
		if got := sessionTTL(); got != defaultSessionTTL {
			t.Fatalf("sessionTTL() = %v, want %v (default)", got, defaultSessionTTL)
		}
	})
}

func TestReadAllSealSessionsNonExistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	records, err := readAllSealSessions(dir)
	if err != nil {
		t.Fatalf("readAllSealSessions non-existent dir error = %v, want nil", err)
	}
	if len(records) != 0 {
		t.Fatalf("len(records) = %d, want 0", len(records))
	}
}

func TestReadAllSealSessionsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	records, err := readAllSealSessions(dir)
	if err != nil {
		t.Fatalf("readAllSealSessions empty dir error = %v, want nil", err)
	}
	if len(records) != 0 {
		t.Fatalf("len(records) = %d, want 0", len(records))
	}
}

func TestReadAllSealSessionsReadDirError(t *testing.T) {
	parent := t.TempDir()
	sessDir := filepath.Join(parent, "sessions")
	// Place a regular file at sessDir so os.ReadDir fails with a non-NotExist error
	if err := os.WriteFile(sessDir, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readAllSealSessions(sessDir)
	if err == nil {
		t.Fatal("readAllSealSessions(file instead of dir) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "read session dir") {
		t.Fatalf("error %q does not mention 'read session dir'", err.Error())
	}
}

func TestReadAllSealSessionsSkipsNonJSONEntries(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "metadata.txt"), []byte("noise"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	records, err := readAllSealSessions(dir)
	if err != nil {
		t.Fatalf("readAllSealSessions error = %v, want nil", err)
	}
	if len(records) != 0 {
		t.Fatalf("len(records) = %d, want 0 (non-JSON and dir skipped)", len(records))
	}
}

func TestReadAllSealSessionsIncludesUnreadableAsError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can always read; skip permission test")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "unreadable.json")
	if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) }) //nolint:errcheck

	records, err := readAllSealSessions(dir)
	if err != nil {
		t.Fatalf("readAllSealSessions error = %v, want nil", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1 (unreadable file included as error)", len(records))
	}
	if records[0].Err == nil {
		t.Fatal("unreadable record Err = nil, want non-nil")
	}
	if !strings.Contains(records[0].Err.Error(), "unreadable") {
		t.Fatalf("Err = %q, want it to contain 'unreadable'", records[0].Err.Error())
	}
}

func TestReadAllSealSessionsWithSession(t *testing.T) {
	dir := t.TempDir()
	sess := sealSession{Key: "test-key", WorktreePath: "/wt", ParentPID: os.Getpid(), StartedAt: time.Now()}
	writeSealSessionFile(t, dir, sess)

	records, err := readAllSealSessions(dir)
	if err != nil {
		t.Fatalf("readAllSealSessions error = %v, want nil", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].Session.Key != "test-key" {
		t.Fatalf("Session.Key = %q, want %q", records[0].Session.Key, "test-key")
	}
}

func TestReadAllSealSessionsIncludesCorruptAsError(t *testing.T) {
	dir := t.TempDir()
	corruptPath := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corruptPath, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	valid := sealSession{Key: "valid", WorktreePath: "/wt-valid", ParentPID: os.Getpid(), StartedAt: time.Now()}
	writeSealSessionFile(t, dir, valid)

	records, err := readAllSealSessions(dir)
	if err != nil {
		t.Fatalf("readAllSealSessions error = %v, want nil", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2 (valid + corrupt)", len(records))
	}

	var foundValid, foundCorrupt bool
	for _, r := range records {
		if r.Err == nil && r.Session.Key == "valid" {
			foundValid = true
		}
		if r.Err != nil && r.Path == corruptPath {
			foundCorrupt = true
			if !strings.Contains(r.Err.Error(), "corrupt") {
				t.Fatalf("corrupt record Err = %q, want it to contain 'corrupt'", r.Err.Error())
			}
		}
	}
	if !foundValid {
		t.Fatal("valid session not found in records")
	}
	if !foundCorrupt {
		t.Fatal("corrupt session not found in records")
	}
}

func TestRemoveStaleSessionFileGone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gone.json")
	sess := sealSession{Key: "k", WorktreePath: "/wt", ParentPID: 999, StartedAt: time.Now()}

	ok, err := removeStaleSession(path, sess)
	if err != nil {
		t.Fatalf("removeStaleSession (file gone) error = %v, want nil", err)
	}
	if ok {
		t.Fatal("removeStaleSession (file gone) removed = true, want false")
	}
}

func TestRemoveStaleSessionStartedAtChanged(t *testing.T) {
	dir := t.TempDir()
	original := sealSession{Key: "k", WorktreePath: "/wt", ParentPID: 1, StartedAt: time.Now().Add(-1 * time.Minute)}
	newer := sealSession{Key: "k2", WorktreePath: "/wt", ParentPID: os.Getpid(), StartedAt: time.Now()}
	writeSealSessionFile(t, dir, newer)
	path := sealSessionPath(dir, "/wt")

	ok, err := removeStaleSession(path, original)
	if err != nil {
		t.Fatalf("removeStaleSession (startedAt changed) error = %v, want nil", err)
	}
	if ok {
		t.Fatal("removeStaleSession (startedAt changed) removed = true, want false (live session protected)")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("live session was unexpectedly deleted: %v", statErr)
	}
}

func TestRemoveStaleSessionNowCorrupt(t *testing.T) {
	dir := t.TempDir()
	original := sealSession{Key: "k", WorktreePath: "/wt", ParentPID: 1, StartedAt: time.Now()}
	path := sealSessionPath(dir, "/wt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := removeStaleSession(path, original)
	if err != nil {
		t.Fatalf("removeStaleSession (now corrupt) error = %v, want nil", err)
	}
	if ok {
		t.Fatal("removeStaleSession (now corrupt) removed = true, want false (corrupt left for manual inspection)")
	}
}

func TestRemoveStaleSessionNowAlive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PID liveness uses kill(0) which is Unix-specific")
	}

	dir := t.TempDir()
	startedAt := time.Now()
	dead := deadPIDForTest(t)
	// original had a dead PID; a live session with the same StartedAt now occupies the path
	original := sealSession{Key: "k", WorktreePath: "/wt-race", ParentPID: dead, StartedAt: startedAt}
	current := sealSession{Key: "k", WorktreePath: "/wt-race", ParentPID: os.Getpid(), ChildPID: os.Getpid(), StartedAt: startedAt}
	writeSealSessionFile(t, dir, current)
	path := sealSessionPath(dir, "/wt-race")

	ok, err := removeStaleSession(path, original)
	if err != nil {
		t.Fatalf("removeStaleSession (now alive) error = %v, want nil", err)
	}
	if ok {
		t.Fatal("removeStaleSession (now alive) removed = true, want false (live session protected)")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("live session was unexpectedly deleted: %v", statErr)
	}
}

func TestRemoveStaleSessionReadError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can always read; skip permission test")
	}

	dir := t.TempDir()
	original := sealSession{Key: "k", WorktreePath: "/wt-noperm", ParentPID: 1, StartedAt: time.Now()}
	path := sealSessionPath(dir, "/wt-noperm")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) }) //nolint:errcheck

	_, err := removeStaleSession(path, original)
	if err == nil {
		t.Fatal("removeStaleSession (unreadable) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "re-read session") {
		t.Fatalf("error %q does not mention 're-read session'", err.Error())
	}
}

func TestRemoveStaleSessionStillStale(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PID liveness uses kill(0) which is Unix-specific")
	}

	dir := t.TempDir()
	dead := deadPIDForTest(t)
	sess := sealSession{Key: "k", WorktreePath: "/wt-stale", ParentPID: dead, StartedAt: time.Now()}
	writeSealSessionFile(t, dir, sess)
	path := sealSessionPath(dir, "/wt-stale")

	ok, err := removeStaleSession(path, sess)
	if err != nil {
		t.Fatalf("removeStaleSession (still stale) error = %v, want nil", err)
	}
	if !ok {
		t.Fatal("removeStaleSession (still stale) removed = false, want true")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("stale session not deleted (stat err = %v)", statErr)
	}
}

func TestFormatAge(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{30 * time.Second, "30s"},
		{59 * time.Second, "59s"},
		{time.Minute, "1m"},
		{5 * time.Minute, "5m"},
		{59 * time.Minute, "59m"},
		{time.Hour, "1h"},
		{2*time.Hour + 30*time.Minute, "2h"},
	} {
		if got := formatAge(tc.d); got != tc.want {
			t.Errorf("formatAge(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// --- seal path store unit tests ---

func TestNormalizeSealPathAbsolute(t *testing.T) {
	root := t.TempDir()
	path, err := normalizeSealPath(root, filepath.Join(root, "src", "foo.go"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != filepath.Join("src", "foo.go") {
		t.Fatalf("got %q, want %q", path, filepath.Join("src", "foo.go"))
	}
}

func TestNormalizeSealPathRelative(t *testing.T) {
	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()

	path, err := normalizeSealPath(root, "src/foo.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != filepath.Join("src", "foo.go") {
		t.Fatalf("got %q, want %q", path, filepath.Join("src", "foo.go"))
	}
}

func TestNormalizeSealPathEscapesRepo(t *testing.T) {
	root := t.TempDir()
	_, err := normalizeSealPath(root, filepath.Join(root, "..", "escape.go"))
	if err == nil {
		t.Fatal("expected error for path outside repo, got nil")
	}
}

func TestNormalizeSealPathRepoRootItself(t *testing.T) {
	root := t.TempDir()
	path, err := normalizeSealPath(root, root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "." {
		t.Fatalf("got %q, want %q", path, ".")
	}
}

func TestReadSealPathStoreNotExist(t *testing.T) {
	store, err := readSealPathStore(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if len(store.Paths) != 0 {
		t.Fatalf("expected empty paths, got %v", store.Paths)
	}
}

func TestReadWriteSealPathStoreRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key1.json")
	original := sealPathStore{
		SchemaVersion: sealPathSchemaVersion,
		Key:           "key1",
		Paths:         []string{"src/a.go", "internal/b.go"},
	}
	if err := writeSealPathStore(path, original); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := readSealPathStore(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Key != original.Key || len(got.Paths) != len(original.Paths) {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", got, original)
	}
}

func TestFindKeyForPathNotFound(t *testing.T) {
	key, err := findKeyForPath(t.TempDir(), "src/foo.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "" {
		t.Fatalf("got %q, want empty", key)
	}
}

func TestFindKeyForPathFindsKey(t *testing.T) {
	dir := t.TempDir()
	store := sealPathStore{SchemaVersion: sealPathSchemaVersion, Key: "key1", Paths: []string{"src/foo.go"}}
	if err := writeSealPathStore(sealPathStoreFile(dir, "key1"), store); err != nil {
		t.Fatalf("write: %v", err)
	}
	key, err := findKeyForPath(dir, "src/foo.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "key1" {
		t.Fatalf("got %q, want %q", key, "key1")
	}
}

func TestFindKeyForPathNotInAnyStore(t *testing.T) {
	dir := t.TempDir()
	store := sealPathStore{SchemaVersion: sealPathSchemaVersion, Key: "key1", Paths: []string{"src/bar.go"}}
	if err := writeSealPathStore(sealPathStoreFile(dir, "key1"), store); err != nil {
		t.Fatalf("write: %v", err)
	}
	key, err := findKeyForPath(dir, "src/foo.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "" {
		t.Fatalf("got %q, want empty", key)
	}
}
