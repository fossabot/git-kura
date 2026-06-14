package gitutil

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func RepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func HeadBranch(repoRoot string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func DeleteBranch(repoRoot, branch string) error {
	cmd := exec.Command("git", "branch", "-d", branch)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete branch %q: %w\n%s", branch, err, out)
	}
	return nil
}

// BranchExists reports whether refs/heads/<branch> exists in the repository.
// It lets callers treat an already-absent branch as a no-op instead of an error
// (e.g. when cleaning up stale state where the branch was deleted manually).
func BranchExists(repoRoot, branch string) (bool, error) {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repoRoot
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	// git show-ref exits 1 specifically when the ref is absent; fatal errors
	// (e.g. run outside a git repository) exit 128. Only exit 1 is a clean
	// "branch does not exist"; everything else is surfaced as a real error.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("check branch %q: %w", branch, err)
}

// PruneWorktrees removes administrative entries for worktrees whose working
// directory no longer exists. This unblocks branch deletion after a worktree
// directory was removed manually (Git otherwise refuses to delete a branch it
// still considers checked out in the now-missing worktree).
func PruneWorktrees(repoRoot string) error {
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("prune worktrees: %w\n%s", err, out)
	}
	return nil
}

func CommonDir(repoRoot string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	dir := strings.TrimSpace(string(out))
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir), nil
	}
	return filepath.Clean(filepath.Join(repoRoot, dir)), nil
}

func WorktreeDirty(path string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}
