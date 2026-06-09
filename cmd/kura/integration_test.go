package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type cliResult struct {
	stdout string
	stderr string
	code   int
}

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
			assertPathMissing(t, filepath.Join(filepath.Dir(outside), filepath.Base(outside)+".worktrees"))
			assertPathMissing(t, filepath.Join(outside, ".git-kura.toml"))
		})
	}
}

func TestKeyValidationAcceptsOpaqueCaseSensitiveKeys(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	for _, key := range []string{
		"51",
		"051",
		"ABC-123",
		"abc-123",
		"task-51",
		"bugfix_login",
		"release-2026-06",
	} {
		t.Run(key, func(t *testing.T) {
			branch := cli.gitKura(repo, "get", key, "--branch")
			requireExitCode(t, branch, 0)
			requireStdoutLine(t, branch, "kura-"+key)

			path := cli.gitKura(repo, "get", key, "--path")
			requireExitCode(t, path, 0)
			requireStdoutLine(t, path, expectedWorktreePath(repo, key))
		})
	}

	requireStdoutLine(t, cli.gitKura(repo, "get", "51", "--branch"), "kura-51")
	requireStdoutLine(t, cli.gitKura(repo, "get", "051", "--branch"), "kura-051")
	requireStdoutLine(t, cli.gitKura(repo, "get", "ABC-123", "--branch"), "kura-ABC-123")
	requireStdoutLine(t, cli.gitKura(repo, "get", "abc-123", "--branch"), "kura-abc-123")
}

func TestKeyValidationRejectsUnsafeKeysWithoutFilesystemChanges(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	for _, key := range []string{
		"../x",
		"..\\x",
		"/a/b",
		`C:\temp\x`,
		"a/b",
		`a\b`,
		".",
		".git",
		".git/config",
		"feature..x",
		"feature.lock",
		"feature.",
		"@{upstream}",
		"a b",
		"a\tb",
		"a\nb",
		"a\x01b",
		"$(rm -rf .)",
		"`rm -rf .`",
		"a; rm -rf .",
		"a && rm -rf .",
		"a | cat",
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
			assertPathMissing(t, filepath.Join(filepath.Dir(repo), filepath.Base(repo)+".worktrees", key))
		})
	}
}

func TestDeterministicResolution(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	for _, tc := range []struct {
		key    string
		branch string
	}{
		{key: "51", branch: "kura-51"},
		{key: "051", branch: "kura-051"},
		{key: "ABC-123", branch: "kura-ABC-123"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			for i := 0; i < 3; i++ {
				requireStdoutLine(t, cli.gitKura(repo, "get", tc.key, "--branch"), tc.branch)
				requireStdoutLine(t, cli.gitKura(repo, "get", tc.key, "--path"), expectedWorktreePath(repo, tc.key))
			}
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

	commandSubstitution := cli.shell(repo, `cd "$(git kura get 51 --path)"`)
	requireExitCode(t, commandSubstitution, 0)

	invalid := cli.gitKura(repo, "get", "../x", "--path")
	requireNonZeroExitCode(t, invalid)
	requireEmptyStdout(t, invalid)

	outside := cli.gitKura(t.TempDir(), "get", "51", "--path")
	requireNonZeroExitCode(t, outside)
	requireEmptyStdout(t, outside)
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

func TestGetStructuredOutputOptions(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	jsonShort := cli.gitKura(repo, "get", "51", "--json")
	jsonFormat := cli.gitKura(repo, "get", "51", "--format", "json")
	requireExitCode(t, jsonShort, 0)
	requireExitCode(t, jsonFormat, 0)
	if jsonShort.stdout != jsonFormat.stdout {
		t.Fatalf("--json and --format json differ\n--json: %s\n--format json: %s", jsonShort.stdout, jsonFormat.stdout)
	}
	metadata := requireJSONMetadata(t, jsonShort.stdout)
	if metadata["branch"] != "kura-51" {
		t.Fatalf("json branch = %v, want kura-51", metadata["branch"])
	}
	if metadata["path"] != expectedWorktreePath(repo, "51") {
		t.Fatalf("json path = %v, want %s", metadata["path"], expectedWorktreePath(repo, "51"))
	}

	toonShort := cli.gitKura(repo, "get", "51", "--toon")
	toonFormat := cli.gitKura(repo, "get", "51", "--format", "toon")
	requireExitCode(t, toonShort, 0)
	requireExitCode(t, toonFormat, 0)
	if toonShort.stdout != toonFormat.stdout {
		t.Fatalf("--toon and --format toon differ\n--toon: %s\n--format toon: %s", toonShort.stdout, toonFormat.stdout)
	}
	requireTOONMetadata(t, toonShort.stdout, "kura-51", expectedWorktreePath(repo, "51"))

	unknown := cli.gitKura(repo, "get", "51", "--format", "xml")
	requireNonZeroExitCode(t, unknown)
	requireStderrContains(t, unknown, "format")
	requireStderrContains(t, unknown, "json")
	requireStderrContains(t, unknown, "toon")
}

func TestGetOutputOptionConflicts(t *testing.T) {
	cli := newTestCLI(t)
	repo := cli.initRepo(t)

	for _, args := range [][]string{
		{"get", "51", "--path", "--branch"},
		{"get", "51", "--json", "--toon"},
		{"get", "51", "--path", "--json"},
		{"get", "51", "--branch", "--json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			result := cli.gitKura(repo, args...)
			requireNonZeroExitCode(t, result)
			requireEmptyStdout(t, result)
			requireStderrContains(t, result, "conflict")
		})
	}
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

func (c *testCLI) shell(dir, script string) cliResult {
	c.t.Helper()
	cmd := exec.Command("sh", "-c", script)
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
	return filepath.Join(filepath.Dir(repo), filepath.Base(repo)+".worktrees", key)
}

func printableName(value string) string {
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
	for _, forbidden := range []string{":", "\"", "'", "warning"} {
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

func requireJSONMetadata(t *testing.T, output string) map[string]any {
	t.Helper()
	var metadata map[string]any
	if err := json.Unmarshal([]byte(output), &metadata); err != nil {
		t.Fatalf("json output is not parseable: %v\n%s", err, output)
	}
	return metadata
}

func requireTOONMetadata(t *testing.T, output, branch, path string) {
	t.Helper()
	if strings.TrimSpace(output) == "" {
		t.Fatal("toon output is empty")
	}
	for _, want := range []string{"branch", branch, "path", path} {
		if !strings.Contains(output, want) {
			t.Fatalf("toon output = %q, want it to contain %q", output, want)
		}
	}
}
