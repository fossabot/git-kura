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

func TestRepoRootReturnsPath(t *testing.T) {
	root, err := gitutil.RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot error = %v", err)
	}
	if root == "" {
		t.Fatal("RepoRoot = empty, want non-empty path")
	}
}

func TestHeadBranchReturnsCurrentBranch(t *testing.T) {
	repo := initRepo(t)

	branch, err := gitutil.HeadBranch(repo)
	if err != nil {
		t.Fatalf("HeadBranch error = %v", err)
	}
	if branch != "main" {
		t.Fatalf("HeadBranch = %q, want %q", branch, "main")
	}
}

func TestDeleteBranch(t *testing.T) {
	t.Run("deletes existing branch", func(t *testing.T) {
		repo := initRepo(t)
		gitCmd(t, repo, "branch", "to-delete")

		if err := gitutil.DeleteBranch(repo, "to-delete"); err != nil {
			t.Fatalf("DeleteBranch error = %v", err)
		}
	})

	t.Run("returns error for non-existent branch", func(t *testing.T) {
		repo := initRepo(t)

		if err := gitutil.DeleteBranch(repo, "no-such-branch"); err == nil {
			t.Fatal("DeleteBranch non-existent branch error = nil, want error")
		}
	})
}

func TestWorktreeDirty(t *testing.T) {
	t.Run("clean worktree returns false", func(t *testing.T) {
		repo := initRepo(t)

		dirty, err := gitutil.WorktreeDirty(repo)
		if err != nil {
			t.Fatalf("WorktreeDirty error = %v", err)
		}
		if dirty {
			t.Fatal("WorktreeDirty clean repo = true, want false")
		}
	})

	t.Run("untracked file returns true", func(t *testing.T) {
		repo := initRepo(t)
		if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}

		dirty, err := gitutil.WorktreeDirty(repo)
		if err != nil {
			t.Fatalf("WorktreeDirty error = %v", err)
		}
		if !dirty {
			t.Fatal("WorktreeDirty untracked file = false, want true")
		}
	})
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
