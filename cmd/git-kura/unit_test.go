package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
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

// --- seal path store unit tests ---

func TestNormalizeSealPathAbsoluteRejected(t *testing.T) {
	root := t.TempDir()
	_, err := normalizeSealPath(root, filepath.Join(root, "src", "foo.go"))
	if err == nil {
		t.Fatal("expected error for absolute path, got nil")
	}
}

func TestNormalizeSealPathRootRelative(t *testing.T) {
	root := t.TempDir()
	path, err := normalizeSealPath(root, "src/foo.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != filepath.Join("src", "foo.go") {
		t.Fatalf("got %q, want %q", path, filepath.Join("src", "foo.go"))
	}
}

func TestNormalizeSealPathIgnoresWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// Even when the caller's cwd is a subdirectory, the argument is resolved
	// against the repository root, not the cwd.
	withWorkingDir(t, sub, func() {
		path, err := normalizeSealPath(root, "src/foo.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != filepath.Join("src", "foo.go") {
			t.Fatalf("got %q, want %q", path, filepath.Join("src", "foo.go"))
		}
	})
}

func TestNormalizeSealPathEscapesRepo(t *testing.T) {
	root := t.TempDir()
	_, err := normalizeSealPath(root, "../escape.go")
	if err == nil {
		t.Fatal("expected error for path outside repo, got nil")
	}
}

func TestNormalizeSealPathDotDotPrefixInsideRepo(t *testing.T) {
	root := t.TempDir()
	// A file like "..foo/bar" starts with ".." but is inside the repo.
	path, err := normalizeSealPath(root, "..foo/bar")
	if err != nil {
		t.Fatalf("unexpected error for path inside repo starting with '..': %v", err)
	}
	if path != filepath.Join("..foo", "bar") {
		t.Fatalf("got %q, want %q", path, filepath.Join("..foo", "bar"))
	}
}

func TestNormalizeSealPathRepoRootItself(t *testing.T) {
	root := t.TempDir()
	path, err := normalizeSealPath(root, ".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "." {
		t.Fatalf("got %q, want %q", path, ".")
	}
}

func TestReadSealStoreNotExist(t *testing.T) {
	store, err := readSealStore(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if len(store.Paths) != 0 {
		t.Fatalf("expected empty paths, got %v", store.Paths)
	}
}

func TestReadSealStoreCorrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "seals.json"), []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readSealStore(filepath.Join(dir, "seals.json"))
	if err == nil {
		t.Fatal("expected error for corrupt store, got nil")
	}
}

func TestReadSealStoreRejectsSchemaViolations(t *testing.T) {
	for name, content := range map[string]string{
		// valid JSON, but entries must be objects with a "key" field
		"bare string entry": `{"schemaVersion":1,"paths":{"src/a.go":"key1"}}`,
		// unsupported schema version
		"wrong schemaVersion": `{"schemaVersion":2,"paths":{}}`,
		// missing required fields
		"missing paths": `{"schemaVersion":1}`,
		// empty key violates minLength
		"empty key": `{"schemaVersion":1,"paths":{"src/a.go":{"key":""}}}`,
		// unknown top-level field violates additionalProperties
		"unknown field": `{"schemaVersion":1,"paths":{},"extra":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "paths.json")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := readSealStore(path); err == nil {
				t.Fatalf("expected schema validation error for %s, got nil", name)
			}
		})
	}
}

func TestWriteSealStoreRejectsSchemaViolations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "paths.json")
	// An empty key violates the schema's minLength constraint; the write
	// must be refused and nothing persisted.
	err := writeSealStore(path, sealPathStore{
		Paths: map[string]sealEntry{"src/a.go": {Key: ""}},
	})
	if err == nil {
		t.Fatal("expected schema validation error, got nil")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("store file should not exist after refused write (stat err: %v)", statErr)
	}
}

func TestWriteSealStoreNormalizesNilPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "paths.json")
	if err := writeSealStore(path, sealPathStore{}); err != nil {
		t.Fatalf("write with nil Paths should normalize to empty map: %v", err)
	}
	got, err := readSealStore(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Paths == nil || len(got.Paths) != 0 {
		t.Fatalf("expected empty paths map, got %+v", got.Paths)
	}
}

func TestReadSealStoreUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission tests are Unix-specific")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root: permission restrictions don't apply")
	}
	dir := t.TempDir()
	storePath := filepath.Join(dir, "seals.json")
	if err := os.WriteFile(storePath, []byte(`{"schemaVersion":1,"paths":{}}`), 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(storePath, 0o644) }()
	_, err := readSealStore(storePath)
	if err == nil {
		t.Fatal("expected error for unreadable file, got nil")
	}
}

func TestWriteReadSealStoreRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "paths.json")
	original := sealPathStore{
		Paths: map[string]sealEntry{
			"src/a.go":      {Key: "key1"},
			"internal/b.go": {Key: "key2"},
		},
	}
	if err := writeSealStore(path, original); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := readSealStore(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got.Paths) != len(original.Paths) {
		t.Fatalf("round-trip length mismatch: got %d, want %d", len(got.Paths), len(original.Paths))
	}
	if got.Paths["src/a.go"].Key != "key1" || got.Paths["internal/b.go"].Key != "key2" {
		t.Fatalf("round-trip content mismatch: got %+v", got.Paths)
	}
	if got.SchemaVersion != sealPathSchemaVersion {
		t.Fatalf("schema version: got %d, want %d", got.SchemaVersion, sealPathSchemaVersion)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestWrittenSealStoreConformsToSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "paths.json")
	store := sealPathStore{
		Paths: map[string]sealEntry{
			"src/a.go": {Key: "key1"},
		},
	}
	if err := writeSealStore(path, store); err != nil {
		t.Fatalf("write: %v", err)
	}

	schemaDoc, err := jsonschema.UnmarshalJSON(strings.NewReader(readFileString(t, filepath.Join("schema", "seal_store.schema.json"))))
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("seal_store.schema.json", schemaDoc); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	schema, err := compiler.Compile("seal_store.schema.json")
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}

	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(readFileString(t, path)))
	if err != nil {
		t.Fatalf("parse written store: %v", err)
	}
	if err := schema.Validate(inst); err != nil {
		t.Fatalf("written store does not conform to schema: %v", err)
	}
}

func TestWriteSealStoreMkdirAllFails(t *testing.T) {
	dir := t.TempDir()
	// Create a file where MkdirAll expects to create a directory
	if err := os.WriteFile(filepath.Join(dir, "not-a-dir"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeSealStore(filepath.Join(dir, "not-a-dir", "paths.json"), sealPathStore{Paths: map[string]sealEntry{}})
	if err == nil {
		t.Fatal("expected error when MkdirAll cannot create dir, got nil")
	}
}

func TestPathsSealStoreOutsideRepo(t *testing.T) {
	_, _, err := pathsSealStore(t.TempDir())
	if err == nil {
		t.Fatal("pathsSealStore outside git repo: error = nil, want error")
	}
}

func TestExitCodeValuesMatchDocs(t *testing.T) {
	// Keep in sync with the exit code table in docs/commands.md.
	if exitSuccess != 0 || exitGeneralError != 1 || exitUsageError != 2 ||
		exitUnsafeRefused != 3 || exitNotFound != 4 ||
		exitSealLockTimeout != 5 || exitSealConflict != 6 {
		t.Fatal("exit code constants must match the table in docs/commands.md")
	}
}

func TestExitCodeErrorNilPassthrough(t *testing.T) {
	if err := exitCodeError(exitSealConflict, nil); err != nil {
		t.Fatalf("exitCodeError(code, nil) = %v, want nil", err)
	}
	err := exitCodeError(exitSealConflict, errors.New("boom"))
	var xe *exitError
	if !errors.As(err, &xe) || xe.code != exitSealConflict {
		t.Fatalf("expected exitError with code %d, got: %v", exitSealConflict, err)
	}
}

func TestAcquireSealLockBasic(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "paths.lock")

	release, err := acquireSealLock(lockPath)
	if err != nil {
		t.Fatalf("acquireSealLock: %v", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file should exist: %v", err)
	}
	release()
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatal("lock file should be removed after release")
	}
}

func TestAcquireSealLockTimeout(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "paths.lock")

	// Hold the lock by creating the file manually.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(lockPath) }()

	// Use a short timeout for the test.
	orig := sealStoreLockTimeout
	sealStoreLockTimeout = 150 * time.Millisecond
	defer func() { sealStoreLockTimeout = orig }()

	_, err = acquireSealLock(lockPath)
	if err == nil {
		t.Fatal("expected lock-timeout error, got nil")
	}
	var xe *exitError
	if !errors.As(err, &xe) || xe.code != exitSealLockTimeout {
		t.Fatalf("expected exitError with code %d, got: %v", exitSealLockTimeout, err)
	}
	if !strings.Contains(err.Error(), "seal-lock-timeout:") {
		t.Fatalf("expected 'seal-lock-timeout:' prefix in error: %s", err.Error())
	}
}

func TestAcquireSealLockTimeoutFromEnv(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "paths.lock")

	// Hold the lock so acquisition must time out.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(lockPath) }()

	t.Setenv("GIT_KURA_SEAL_LOCK_TIMEOUT", "50ms")
	start := time.Now()
	_, err = acquireSealLock(lockPath)
	if err == nil {
		t.Fatal("expected lock-timeout error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("env timeout not honored: took %s", elapsed)
	}
	if !strings.Contains(err.Error(), "50ms") {
		t.Fatalf("expected timeout from env in message, got: %s", err.Error())
	}
}

func TestAcquireSealLockUnwritableDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission tests are Unix-specific")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root: permission restrictions don't apply")
	}
	dir := filepath.Join(t.TempDir(), "seals")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()

	_, err := acquireSealLock(filepath.Join(dir, "paths.lock"))
	if err == nil {
		t.Fatal("expected error creating lock in unwritable dir, got nil")
	}
	var xe *exitError
	if errors.As(err, &xe) {
		t.Fatalf("permission error must not be reported as lock timeout: %v", err)
	}
}

func TestWriteSealStoreUnwritableDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission tests are Unix-specific")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root: permission restrictions don't apply")
	}
	dir := filepath.Join(t.TempDir(), "seals")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()

	err := writeSealStore(filepath.Join(dir, "paths.json"), sealPathStore{Paths: map[string]sealEntry{}})
	if err == nil {
		t.Fatal("expected error writing store in unwritable dir, got nil")
	}
}

func TestSealLockReleaseReportsRemoveFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission tests are Unix-specific")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root: permission restrictions don't apply")
	}
	dir := filepath.Join(t.TempDir(), "seals")
	lockPath := filepath.Join(dir, "paths.lock")

	release, err := acquireSealLock(lockPath)
	if err != nil {
		t.Fatalf("acquireSealLock: %v", err)
	}

	// Make the directory read-only so the lock file cannot be removed.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()

	release() // must not panic; reports the failure on stderr

	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file should still exist after failed release: %v", err)
	}
}

func TestReadSealContextInsideWorktree(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := openManagedWorktree(t, repo, "key1")

	withWorkingDir(t, wt, func() {
		key, err := readSealContext()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "key1" {
			t.Fatalf("got %q, want %q", key, "key1")
		}
	})
}

func TestReadSealContextOutsideWorktree(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	// The main checkout is a git repository but not a managed worktree.
	withWorkingDir(t, repo, func() {
		if _, err := readSealContext(); err == nil {
			t.Fatal("expected error outside a managed worktree, got nil")
		}
	})
}

// --- cmdSealClaim / cmdSealUnclaim in-process tests (need a real git repo) ---

func TestCmdSealClaimAndRemoveInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := openManagedWorktree(t, repo, "key1")

	withWorkingDir(t, wt, func() {
		if err := cmdSealClaim([]string{"tracked.txt"}); err != nil {
			t.Fatalf("cmdSealClaim: %v", err)
		}
		// idempotent: same key, same path
		if err := cmdSealClaim([]string{"tracked.txt"}); err != nil {
			t.Fatalf("cmdSealClaim idempotent: %v", err)
		}
		if err := cmdSealUnclaim([]string{"tracked.txt"}); err != nil {
			t.Fatalf("cmdSealUnclaim: %v", err)
		}
		// idempotent: not present
		if err := cmdSealUnclaim([]string{"tracked.txt"}); err != nil {
			t.Fatalf("cmdSealUnclaim idempotent: %v", err)
		}
	})
}

func TestCmdSealClaimMultiplePathsInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	commitFile(t, repo, "second.txt", "content\n")
	commitFile(t, repo, "third.txt", "content\n")
	wt1 := openManagedWorktree(t, repo, "key1")
	wt2 := openManagedWorktree(t, repo, "key2")

	withWorkingDir(t, wt1, func() {
		if err := cmdSealClaim([]string{"tracked.txt", "second.txt", "third.txt"}); err != nil {
			t.Fatalf("cmdSealClaim multiple paths: %v", err)
		}
	})
	// All three should be blocked for a different key
	withWorkingDir(t, wt2, func() {
		if err := cmdSealClaim([]string{"second.txt"}); err == nil {
			t.Fatal("expected conflict for second.txt, got nil")
		}
	})
}

func TestCmdSealClaimRejectsDifferentKeyInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt1 := openManagedWorktree(t, repo, "key1")
	wt2 := openManagedWorktree(t, repo, "key2")

	withWorkingDir(t, wt1, func() {
		if err := cmdSealClaim([]string{"tracked.txt"}); err != nil {
			t.Fatalf("cmdSealClaim: %v", err)
		}
	})

	withWorkingDir(t, wt2, func() {
		err := cmdSealClaim([]string{"tracked.txt"})
		if err == nil {
			t.Fatal("expected error when adding path under different key, got nil")
		}
		var xe *exitError
		if !errors.As(err, &xe) || xe.code != exitSealConflict {
			t.Fatalf("expected exitError code %d, got: %v", exitSealConflict, err)
		}
		if !strings.Contains(err.Error(), "seal-conflict:") {
			t.Fatalf("expected 'seal-conflict:' prefix, got: %s", err.Error())
		}
	})
}

func TestCmdSealUnclaimRejectsDifferentKeyInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt1 := openManagedWorktree(t, repo, "key1")
	wt2 := openManagedWorktree(t, repo, "key2")

	withWorkingDir(t, wt1, func() {
		if err := cmdSealClaim([]string{"tracked.txt"}); err != nil {
			t.Fatalf("cmdSealClaim: %v", err)
		}
	})

	withWorkingDir(t, wt2, func() {
		err := cmdSealUnclaim([]string{"tracked.txt"})
		if err == nil {
			t.Fatal("expected error when removing path owned by different key, got nil")
		}
		var xe *exitError
		if !errors.As(err, &xe) || xe.code != exitSealConflict {
			t.Fatalf("expected exitError code %d, got: %v", exitSealConflict, err)
		}
		if !strings.Contains(err.Error(), "seal-conflict:") {
			t.Fatalf("expected 'seal-conflict:' prefix, got: %s", err.Error())
		}
	})

	// key1's seal must still be intact
	withWorkingDir(t, wt1, func() {
		if err := cmdSealClaim([]string{"tracked.txt"}); err != nil {
			t.Fatalf("seal should still be owned by key1 after failed removal: %v", err)
		}
	})
}

func TestCmdSealClaimReportsAllConflictsInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	commitFile(t, repo, "second.txt", "content\n")
	commitFile(t, repo, "third.txt", "content\n")
	wt1 := openManagedWorktree(t, repo, "key1")
	wt2 := openManagedWorktree(t, repo, "key2")
	wt3 := openManagedWorktree(t, repo, "key3")
	wt4 := openManagedWorktree(t, repo, "key4")

	withWorkingDir(t, wt1, func() {
		if err := cmdSealClaim([]string{"tracked.txt"}); err != nil {
			t.Fatalf("cmdSealClaim tracked.txt: %v", err)
		}
	})
	withWorkingDir(t, wt2, func() {
		if err := cmdSealClaim([]string{"second.txt"}); err != nil {
			t.Fatalf("cmdSealClaim second.txt: %v", err)
		}
	})

	// key3 tries to add all three: the error must list both conflicting
	// paths with the keys that seal them.
	withWorkingDir(t, wt3, func() {
		err := cmdSealClaim([]string{"tracked.txt", "second.txt", "third.txt"})
		if err == nil {
			t.Fatal("expected conflict error, got nil")
		}
		msg := err.Error()
		for _, want := range []string{"seal-conflict:", "tracked.txt", "key1", "second.txt", "key2"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("conflict error missing %q: %s", want, msg)
			}
		}
	})

	// All-or-nothing: third.txt must not have been sealed.
	withWorkingDir(t, wt4, func() {
		if err := cmdSealClaim([]string{"third.txt"}); err != nil {
			t.Fatalf("third.txt should not have been claimed by the failed claim: %v", err)
		}
	})
}

func TestCmdSealClaimRejectsDirectoryInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := openManagedWorktree(t, repo, "key1")
	if err := os.Mkdir(filepath.Join(wt, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	withWorkingDir(t, wt, func() {
		err := cmdSealClaim([]string{"subdir"})
		if err == nil {
			t.Fatal("expected error for directory target, got nil")
		}
		if !strings.Contains(err.Error(), "directory") {
			t.Fatalf("expected directory rejection message, got: %v", err)
		}
	})
}

func TestCmdSealClaimNonExistentFileInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := openManagedWorktree(t, repo, "key1")

	withWorkingDir(t, wt, func() {
		if err := cmdSealClaim([]string{"nosuchfile.txt"}); err == nil {
			t.Fatal("expected error for non-existent file, got nil")
		}
	})
}

func TestCmdSealClaimOutsideRepoInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := openManagedWorktree(t, repo, "key1")

	withWorkingDir(t, wt, func() {
		if err := cmdSealClaim([]string{"../outside.txt"}); err == nil {
			t.Fatal("expected error for path outside repo, got nil")
		}
	})
}

func TestCmdSealClaimFailsOutsideGitRepo(t *testing.T) {
	withWorkingDir(t, t.TempDir(), func() {
		if err := cmdSealClaim([]string{"tracked.txt"}); err == nil {
			t.Fatal("expected error outside git repo, got nil")
		}
	})
}

func TestCmdSealUnclaimFailsOutsideGitRepo(t *testing.T) {
	withWorkingDir(t, t.TempDir(), func() {
		if err := cmdSealUnclaim([]string{"tracked.txt"}); err == nil {
			t.Fatal("expected error outside git repo, got nil")
		}
	})
}

func TestCmdSealClaimFailsOutsideManagedWorktree(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	// A plain git checkout that is not a managed worktree must be rejected.
	withWorkingDir(t, repo, func() {
		if err := cmdSealClaim([]string{"tracked.txt"}); err == nil {
			t.Fatal("expected error outside a managed worktree, got nil")
		}
	})
}

func TestCmdSealUnclaimOutsideRepoInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := openManagedWorktree(t, repo, "key1")

	withWorkingDir(t, wt, func() {
		if err := cmdSealUnclaim([]string{"../outside.txt"}); err == nil {
			t.Fatal("expected error for path outside repo, got nil")
		}
	})
}

func TestCmdSealUnclaimAllowsDifferentKeyAfterRemovalInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt1 := openManagedWorktree(t, repo, "key1")
	wt2 := openManagedWorktree(t, repo, "key2")

	withWorkingDir(t, wt1, func() {
		if err := cmdSealClaim([]string{"tracked.txt"}); err != nil {
			t.Fatalf("cmdSealClaim: %v", err)
		}
		if err := cmdSealUnclaim([]string{"tracked.txt"}); err != nil {
			t.Fatalf("cmdSealUnclaim: %v", err)
		}
	})
	// After removal, a different key can now seal the same path
	withWorkingDir(t, wt2, func() {
		if err := cmdSealClaim([]string{"tracked.txt"}); err != nil {
			t.Fatalf("cmdSealClaim after removal: %v", err)
		}
	})
}

func TestCmdSealUnclaimFromMultiPathStoreInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	commitFile(t, repo, "second.txt", "content\n")
	wt1 := openManagedWorktree(t, repo, "key1")
	wt2 := openManagedWorktree(t, repo, "key2")

	withWorkingDir(t, wt1, func() {
		if err := cmdSealClaim([]string{"tracked.txt", "second.txt"}); err != nil {
			t.Fatalf("cmdSealClaim: %v", err)
		}
		if err := cmdSealUnclaim([]string{"tracked.txt"}); err != nil {
			t.Fatalf("cmdSealUnclaim tracked.txt: %v", err)
		}
	})
	// second.txt is still sealed under key1
	withWorkingDir(t, wt2, func() {
		if err := cmdSealClaim([]string{"second.txt"}); err == nil {
			t.Fatal("expected conflict error for second.txt still sealed under key1, got nil")
		}
	})
}

func TestRunSealClaimUnclaimInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	wt := openManagedWorktree(t, repo, "key1")

	withWorkingDir(t, wt, func() {
		if err := run([]string{"seal", "claim", "tracked.txt"}); err != nil {
			t.Fatalf("seal claim via run: %v", err)
		}
		if err := run([]string{"seal", "unclaim", "tracked.txt"}); err != nil {
			t.Fatalf("seal unclaim via run: %v", err)
		}
	})
}

func TestRunSealClaimMultiplePathsInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	commitFile(t, repo, "second.txt", "content\n")
	wt := openManagedWorktree(t, repo, "key1")

	withWorkingDir(t, wt, func() {
		if err := run([]string{"seal", "claim", "tracked.txt", "second.txt"}); err != nil {
			t.Fatalf("seal claim multiple paths via run: %v", err)
		}
		if err := run([]string{"seal", "unclaim", "tracked.txt", "second.txt"}); err != nil {
			t.Fatalf("seal unclaim multiple paths via run: %v", err)
		}
	})
}

func TestRunSealClaimMissingArgInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	// The missing-argument check runs before the seal context is resolved, so
	// it fails regardless of the current worktree.
	withWorkingDir(t, repo, func() {
		if err := run([]string{"seal", "claim"}); err == nil {
			t.Fatal("expected error for missing path arg, got nil")
		}
		if err := run([]string{"seal", "unclaim"}); err == nil {
			t.Fatal("expected error for missing path arg, got nil")
		}
	})
}

func TestRunSealClaimUnclaimHelpInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	withWorkingDir(t, repo, func() {
		stdout, err := captureStdout(t, func() error {
			return run([]string{"seal", "claim", "--help"})
		})
		if err != nil {
			t.Fatalf("seal claim --help: %v", err)
		}
		if !strings.Contains(stdout, "managed worktree") {
			t.Fatalf("seal claim --help should describe worktree-derived key resolution: %s", stdout)
		}

		stdout, err = captureStdout(t, func() error {
			return run([]string{"seal", "unclaim", "--help"})
		})
		if err != nil {
			t.Fatalf("seal unclaim --help: %v", err)
		}
		if !strings.Contains(stdout, "managed worktree") {
			t.Fatalf("seal unclaim --help should describe worktree-derived key resolution: %s", stdout)
		}
	})
}
