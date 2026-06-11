package main

import (
	"os"
	"runtime"
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
		key, cmd, err := parseSealEnterArgs([]string{"issue-12"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "issue-12" {
			t.Fatalf("key = %q, want %q", key, "issue-12")
		}
		if len(cmd) != 0 {
			t.Fatalf("command = %v, want empty", cmd)
		}
	})

	t.Run("-- command returns key and command", func(t *testing.T) {
		key, cmd, err := parseSealEnterArgs([]string{"issue-12", "--", "echo", "hi"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "issue-12" {
			t.Fatalf("key = %q, want %q", key, "issue-12")
		}
		if len(cmd) != 2 || cmd[0] != "echo" || cmd[1] != "hi" {
			t.Fatalf("command = %v, want [echo hi]", cmd)
		}
	})

	t.Run("no key is usage error", func(t *testing.T) {
		_, _, err := parseSealEnterArgs([]string{})
		if err == nil {
			t.Fatal("expected error for missing key, got nil")
		}
	})

	t.Run("invalid key is error", func(t *testing.T) {
		_, _, err := parseSealEnterArgs([]string{"../x"})
		if err == nil {
			t.Fatal("expected error for invalid key, got nil")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "key") {
			t.Fatalf("error %q does not mention key", err.Error())
		}
	})

	t.Run("extra argument without -- is error", func(t *testing.T) {
		_, _, err := parseSealEnterArgs([]string{"issue-12", "extra"})
		if err == nil {
			t.Fatal("expected error for extra argument, got nil")
		}
	})

	t.Run("-- with no command is error", func(t *testing.T) {
		_, _, err := parseSealEnterArgs([]string{"issue-12", "--"})
		if err == nil {
			t.Fatal("expected error for -- with no command, got nil")
		}
	})
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
	prev, had := os.LookupEnv("GIT_KURA_SEAL_KEY")
	os.Unsetenv("GIT_KURA_SEAL_KEY")
	t.Cleanup(func() {
		if had {
			os.Setenv("GIT_KURA_SEAL_KEY", prev)
		} else {
			os.Unsetenv("GIT_KURA_SEAL_KEY")
		}
	})
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
		prev, had := os.LookupEnv("SHELL")
		os.Unsetenv("SHELL")
		t.Cleanup(func() {
			if had {
				os.Setenv("SHELL", prev)
			} else {
				os.Unsetenv("SHELL")
			}
		})
		if got := detectShell(); got != "sh" {
			t.Fatalf("detectShell() = %q, want sh", got)
		}
	})
}
