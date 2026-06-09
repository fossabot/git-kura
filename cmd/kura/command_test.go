package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// In-process command tests cover dispatch and command branches without spawning
// the compiled binary. End-to-end CLI behavior is covered by integration tests.

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
		{name: "ls help", args: []string{"ls", "--help"}, want: "Usage: git kura ls"},
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
		{"ls", "unexpected"},
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
		if err == nil {
			t.Fatal("get --path before open error = nil, want error")
		}
		if stdout != "" {
			t.Fatalf("get --path before open stdout = %q, want empty", stdout)
		}

		stdout, err = captureStdout(t, func() error {
			return run([]string{"get", "51", "--branch"})
		})
		if err == nil {
			t.Fatal("get --branch before open error = nil, want error")
		}
		if stdout != "" {
			t.Fatalf("get --branch before open stdout = %q, want empty", stdout)
		}

		stdout, err = captureStdout(t, func() error {
			return run([]string{"get", "51", "--json"})
		})
		if err == nil {
			t.Fatal("get --json before open error = nil, want error")
		}
		if stdout != "" {
			t.Fatalf("get --json before open stdout = %q, want empty", stdout)
		}

		stdout, err = captureStdout(t, func() error {
			return run([]string{"get", "51", "--root"})
		})
		if err == nil {
			t.Fatal("get --root before open error = nil, want error")
		}
		if stdout != "" {
			t.Fatalf("get --root before open stdout = %q, want empty", stdout)
		}

		stdout, err = captureStdout(t, func() error {
			return run([]string{"open", "51", "--dry-run"})
		})
		if err != nil {
			t.Fatalf("open --dry-run error = %v", err)
		}
		dryRun := requireJSONMetadata(t, stdout)
		if dryRun["branch"] != "51" || dryRun["worktreePath"] != expectedWorktreePath(repo, "51") {
			t.Fatalf("dry-run metadata = %+v, want branch 51 and path %s", dryRun, expectedWorktreePath(repo, "51"))
		}

		if err := run([]string{"open", "51"}); err != nil {
			t.Fatalf("open error = %v", err)
		}
		assertPathExists(t, expectedWorktreePath(repo, "51"))
		assertPathExists(t, expectedMetadataPath(repo, "51"))

		stdout, err = captureStdout(t, func() error {
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
		if strings.TrimSpace(stdout) != "51" {
			t.Fatalf("get --branch stdout = %q, want 51", stdout)
		}

		stdout, err = captureStdout(t, func() error {
			return run([]string{"get", "51", "--root"})
		})
		if err != nil {
			t.Fatalf("get --root error = %v", err)
		}
		if strings.TrimSpace(stdout) != repo {
			t.Fatalf("get --root stdout = %q, want %q", stdout, repo)
		}

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
			{"get", "51", "--root"},
			{"get", "51", "--json"},
			{"open", "51"},
			{"close", "51"},
			{"ls"},
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

func TestRunLsIgnoresNonMetadataEntries(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	withWorkingDir(t, repo, func() {
		if err := run([]string{"open", "51"}); err != nil {
			t.Fatalf("open error = %v", err)
		}

		metaDir := filepath.Join(expectedStateDir(repo), "meta", "worktrees")
		writeFile(t, filepath.Join(metaDir, "notjson"), "noise")
		if err := os.Mkdir(filepath.Join(metaDir, "subdir"), 0o755); err != nil {
			t.Fatal(err)
		}

		stdout, err := captureStdout(t, func() error {
			return run([]string{"ls"})
		})
		if err != nil {
			t.Fatalf("ls error = %v", err)
		}
		lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
		if len(lines) != 1 || lines[0] != "51" {
			t.Fatalf("ls stdout = %q, want only line \"51\"", stdout)
		}
	})
}

func TestRunLsInProcess(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	withWorkingDir(t, repo, func() {
		stdout, err := captureStdout(t, func() error {
			return run([]string{"ls"})
		})
		if err != nil {
			t.Fatalf("ls with no worktrees error = %v, want nil", err)
		}
		if strings.TrimSpace(stdout) != "" {
			t.Fatalf("ls with no worktrees stdout = %q, want empty", stdout)
		}

		if err := run([]string{"open", "51"}); err != nil {
			t.Fatalf("open 51 error = %v", err)
		}
		if err := run([]string{"open", "52"}); err != nil {
			t.Fatalf("open 52 error = %v", err)
		}

		stdout, err = captureStdout(t, func() error {
			return run([]string{"ls"})
		})
		if err != nil {
			t.Fatalf("ls error = %v", err)
		}
		for _, key := range []string{"51", "52"} {
			found := false
			for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
				if line == key {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("ls stdout = %q, want line %q", stdout, key)
			}
		}

		if err := run([]string{"close", "51"}); err != nil {
			t.Fatalf("close 51 error = %v", err)
		}

		stdout, err = captureStdout(t, func() error {
			return run([]string{"ls"})
		})
		if err != nil {
			t.Fatalf("ls after close error = %v", err)
		}
		for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
			if line == "51" {
				t.Fatalf("ls after close stdout = %q, want no line 51", stdout)
			}
		}
		found := false
		for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
			if line == "52" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("ls after close stdout = %q, want line 52", stdout)
		}
	})
}
