package worktree

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tooppoo/git-kura/internal/gitutil"
)

type MetadataFile struct {
	RepositoryRoot string `json:"repositoryRoot"`
	BaseBranch     string `json:"baseBranch"`
	WorktreePath   string `json:"worktreePath"`
}

func BranchName(key string) string {
	return key
}

func StateDir(repoRoot string) (string, error) {
	commonDir, err := gitutil.CommonDir(repoRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(commonDir, "kura"), nil
}

func Path(repoRoot, key string) (string, error) {
	dir, err := StateDir(repoRoot)
	if err != nil {
		return "", err
	}
	return PathInStateDir(dir, key), nil
}

func PathInStateDir(stateDir, key string) string {
	return filepath.Join(stateDir, "worktrees", key)
}

func MetadataPath(repoRoot, key string) (string, error) {
	dir, err := StateDir(repoRoot)
	if err != nil {
		return "", err
	}
	return MetadataPathInStateDir(dir, key), nil
}

func MetadataPathInStateDir(stateDir, key string) string {
	return filepath.Join(stateDir, "meta", "worktrees", key+".json")
}

func ReadMetadata(repoRoot, key string) (MetadataFile, error) {
	path, err := MetadataPath(repoRoot, key)
	if err != nil {
		return MetadataFile{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return MetadataFile{}, err
	}
	var meta MetadataFile
	if err := json.Unmarshal(data, &meta); err != nil {
		return MetadataFile{}, err
	}
	return meta, nil
}

func ReadStructuredMetadata(repoRoot, key, worktreePath string, worktreeExists bool) (MetadataFile, error) {
	metaPath, err := MetadataPath(repoRoot, key)
	if err != nil {
		return MetadataFile{}, fmt.Errorf("resolve metadata path: %w", err)
	}

	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			if worktreeExists {
				return MetadataFile{}, fmt.Errorf("metadata for key %q is missing; worktree exists at %s, but Kura cannot reconstruct creation-time metadata such as baseBranch", key, worktreePath)
			}
			return MetadataFile{}, fmt.Errorf("worktree for key %q is not open; run \"git kura open %s\" first", key, key)
		}
		return MetadataFile{}, fmt.Errorf("read metadata for key %q: %w", key, err)
	}

	var meta MetadataFile
	if err := json.Unmarshal(data, &meta); err != nil {
		return MetadataFile{}, fmt.Errorf("metadata for key %q is invalid: %w", key, err)
	}
	if !worktreeExists {
		return MetadataFile{}, fmt.Errorf("worktree for key %q is missing; metadata exists at %s, but expected worktree at %s", key, metaPath, worktreePath)
	}

	return meta, nil
}
