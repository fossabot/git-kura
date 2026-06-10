package worktree_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tooppoo/git-kura/internal/worktree"
)

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
			if got := worktree.BranchName(tc.key); got != tc.want {
				t.Fatalf("BranchName(%q) = %q, want %q", tc.key, got, tc.want)
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
			if got := worktree.PathInStateDir(tc.stateDir, tc.key); got != tc.want {
				t.Fatalf("PathInStateDir(%q, %q) = %q, want %q", tc.stateDir, tc.key, got, tc.want)
			}
		})
	}
}

func TestMetadataPath(t *testing.T) {
	stateDir := filepath.Join("/home", "user", "repo", ".git", "kura")
	want := filepath.Join("/home", "user", "repo", ".git", "kura", "meta", "worktrees", "51.json")
	if got := worktree.MetadataPathInStateDir(stateDir, "51"); got != want {
		t.Fatalf("MetadataPathInStateDir(%q, 51) = %q, want %q", stateDir, got, want)
	}
}

func TestReadMetadata(t *testing.T) {
	repo := initRepo(t)
	path, err := worktree.MetadataPath(repo, "51")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, `{"repositoryRoot":"/repo","baseBranch":"main","worktreePath":"/tmp/worktree"}`)

	meta, err := worktree.ReadMetadata(repo, "51")
	if err != nil {
		t.Fatalf("ReadMetadata error = %v", err)
	}
	if meta.RepositoryRoot != "/repo" || meta.BaseBranch != "main" || meta.WorktreePath != "/tmp/worktree" {
		t.Fatalf("metadata = %+v, want repositoryRoot /repo, baseBranch main, worktreePath /tmp/worktree", meta)
	}

	writeFile(t, path, `{`)
	if _, err := worktree.ReadMetadata(repo, "51"); err == nil {
		t.Fatal("ReadMetadata invalid JSON error = nil, want error")
	}

	if _, err := worktree.ReadMetadata(repo, "missing"); err == nil {
		t.Fatal("ReadMetadata missing file error = nil, want error")
	}
}

func TestReadStructuredMetadata(t *testing.T) {
	repo := initRepo(t)

	wtPath, err := worktree.Path(repo, "51")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := worktree.MetadataPath(repo, "51")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := worktree.ReadStructuredMetadata(repo, "51", wtPath, false); err == nil {
		t.Fatal("ReadStructuredMetadata unopened key error = nil, want error")
	} else if !strings.Contains(err.Error(), "not open") {
		t.Fatalf("error = %q, want it to mention not open", err.Error())
	}

	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.ReadStructuredMetadata(repo, "51", wtPath, true); err == nil {
		t.Fatal("ReadStructuredMetadata missing metadata error = nil, want error")
	} else if !strings.Contains(err.Error(), "metadata") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %q, want it to mention missing metadata", err.Error())
	}

	if err := os.MkdirAll(filepath.Dir(metadata), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, metadata, `{`)
	if _, err := worktree.ReadStructuredMetadata(repo, "51", wtPath, true); err == nil {
		t.Fatal("ReadStructuredMetadata invalid JSON error = nil, want error")
	} else if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("error = %q, want it to mention invalid", err.Error())
	}

	writeFile(t, metadata, `{"repositoryRoot":"`+repo+`","baseBranch":"main","worktreePath":"`+wtPath+`"}`)
	meta, err := worktree.ReadStructuredMetadata(repo, "51", wtPath, true)
	if err != nil {
		t.Fatalf("ReadStructuredMetadata error = %v", err)
	}
	if meta.RepositoryRoot != repo || meta.BaseBranch != "main" || meta.WorktreePath != wtPath {
		t.Fatalf("metadata = %+v, want repositoryRoot %s, baseBranch main, worktreePath %s", meta, repo, wtPath)
	}

	if _, err := worktree.ReadStructuredMetadata(repo, "51", wtPath, false); err == nil {
		t.Fatal("ReadStructuredMetadata missing worktree error = nil, want error")
	} else if !strings.Contains(err.Error(), "worktree") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %q, want it to mention missing worktree", err.Error())
	}
}

func TestPathHelpersReturnErrorsOutsideRepository(t *testing.T) {
	dir := t.TempDir()

	if _, err := worktree.StateDir(dir); err == nil {
		t.Fatal("StateDir outside git repo error = nil, want error")
	}
	if _, err := worktree.Path(dir, "51"); err == nil {
		t.Fatal("Path outside git repo error = nil, want error")
	}
	if _, err := worktree.MetadataPath(dir, "51"); err == nil {
		t.Fatal("MetadataPath outside git repo error = nil, want error")
	}
}

func initRepo(t *testing.T) string {
	t.Helper()

	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, repo, "init", "-b", "main")
	return repo
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
