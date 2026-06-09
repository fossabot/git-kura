package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestBranchName(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want string
	}{
		{"51", "51"},
		{"ABC-123", "ABC-123"},
		{"release-2026-06", "release-2026-06"},
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
		stateDir string
		key      string
		want     string
	}{
		{
			stateDir: filepath.Join("/home", "user", "repo", ".git", "kura"),
			key:      "51",
			want:     filepath.Join("/home", "user", "repo", ".git", "kura", "worktrees", "51"),
		},
		{
			stateDir: filepath.Join("/home", "user", "myproject", ".git", "kura"),
			key:      "feature",
			want:     filepath.Join("/home", "user", "myproject", ".git", "kura", "worktrees", "feature"),
		},
	} {
		t.Run(tc.key, func(t *testing.T) {
			if got := worktreePathInStateDir(tc.stateDir, tc.key); got != tc.want {
				t.Fatalf("worktreePathInStateDir(%q, %q) = %q, want %q", tc.stateDir, tc.key, got, tc.want)
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

func TestGitHelpersReturnErrors(t *testing.T) {
	dir := t.TempDir()

	if _, err := headBranch(dir); err == nil {
		t.Fatal("headBranch outside git repo error = nil, want error")
	}
	if _, err := worktreeDirty(dir); err == nil {
		t.Fatal("worktreeDirty outside git repo error = nil, want error")
	}
}

func TestGitCommonDirSupportsLinkedWorktree(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")

	git(t, repo, "worktree", "add", "-b", "linked", linked)

	commonDir, err := gitCommonDir(linked)
	if err != nil {
		t.Fatalf("gitCommonDir linked worktree error = %v", err)
	}
	want := filepath.Join(repo, ".git")
	if commonDir != want {
		t.Fatalf("gitCommonDir linked worktree = %q, want %q", commonDir, want)
	}
}

func TestReadMetadata(t *testing.T) {
	repo := initUnitRepo(t)
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

func TestReadStructuredMetadata(t *testing.T) {
	repo := initUnitRepo(t)
	worktree := expectedWorktreePath(repo, "51")
	metadata := expectedMetadataPath(repo, "51")

	if _, err := readStructuredMetadata(repo, "51", worktree, false); err == nil {
		t.Fatal("readStructuredMetadata unopened key error = nil, want error")
	} else if !strings.Contains(err.Error(), "not open") {
		t.Fatalf("error = %q, want it to mention not open", err.Error())
	}

	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := readStructuredMetadata(repo, "51", worktree, true); err == nil {
		t.Fatal("readStructuredMetadata missing metadata error = nil, want error")
	} else if !strings.Contains(err.Error(), "metadata") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %q, want it to mention missing metadata", err.Error())
	}

	if err := os.MkdirAll(filepath.Dir(metadata), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, metadata, `{`)
	if _, err := readStructuredMetadata(repo, "51", worktree, true); err == nil {
		t.Fatal("readStructuredMetadata invalid JSON error = nil, want error")
	} else if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("error = %q, want it to mention invalid", err.Error())
	}

	writeFile(t, metadata, `{"baseBranch":"main","worktreePath":"`+worktree+`"}`)
	meta, err := readStructuredMetadata(repo, "51", worktree, true)
	if err != nil {
		t.Fatalf("readStructuredMetadata error = %v", err)
	}
	if meta.BaseBranch != "main" || meta.WorktreePath != worktree {
		t.Fatalf("metadata = %+v, want main and %s", meta, worktree)
	}

	if _, err := readStructuredMetadata(repo, "51", worktree, false); err == nil {
		t.Fatal("readStructuredMetadata missing worktree error = nil, want error")
	} else if !strings.Contains(err.Error(), "worktree") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %q, want it to mention missing worktree", err.Error())
	}
}

func TestPathHelpersReturnErrorsOutsideRepository(t *testing.T) {
	dir := t.TempDir()

	if _, err := stateDir(dir); err == nil {
		t.Fatal("stateDir outside git repo error = nil, want error")
	}
	if _, err := worktreePath(dir, "51"); err == nil {
		t.Fatal("worktreePath outside git repo error = nil, want error")
	}
	if _, err := metadataPath(dir, "51"); err == nil {
		t.Fatal("metadataPath outside git repo error = nil, want error")
	}
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

func TestMetadataPath(t *testing.T) {
	stateDir := filepath.Join("/home", "user", "repo", ".git", "kura")
	want := filepath.Join("/home", "user", "repo", ".git", "kura", "meta", "worktrees", "51.json")
	if got := metadataPathInStateDir(stateDir, "51"); got != want {
		t.Fatalf("metadataPathInStateDir(%q, 51) = %q, want %q", stateDir, got, want)
	}
}

func initUnitRepo(t *testing.T) string {
	t.Helper()

	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-b", "main")
	return repo
}

func TestRequireCleanValueStdoutAcceptsWindowsPath(t *testing.T) {
	requireCleanValueStdout(t, cliResult{stdout: `C:\repo.kura\worktrees\51` + "\n"})
}
