package gitutil_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tooppoo/git-kura/internal/gitutil"
)

func TestGitHelpersReturnErrors(t *testing.T) {
	dir := t.TempDir()

	if _, err := gitutil.HeadBranch(dir); err == nil {
		t.Fatal("HeadBranch outside git repo error = nil, want error")
	}
	if _, err := gitutil.WorktreeDirty(dir); err == nil {
		t.Fatal("WorktreeDirty outside git repo error = nil, want error")
	}
}

func TestGitCommonDirSupportsLinkedWorktree(t *testing.T) {
	repo := initRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")

	gitCmd(t, repo, "worktree", "add", "-b", "linked", linked)

	commonDir, err := gitutil.CommonDir(linked)
	if err != nil {
		t.Fatalf("CommonDir linked worktree error = %v", err)
	}
	want := filepath.Join(repo, ".git")
	if commonDir != want {
		t.Fatalf("CommonDir linked worktree = %q, want %q", commonDir, want)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()

	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, repo, "init", "-b", "main")
	gitCmd(t, repo, "config", "user.email", "kura-test@example.com")
	gitCmd(t, repo, "config", "user.name", "Kura Test")

	tracked := filepath.Join(repo, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, repo, "add", "tracked.txt")
	gitCmd(t, repo, "commit", "-m", "initial")
	return repo
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
