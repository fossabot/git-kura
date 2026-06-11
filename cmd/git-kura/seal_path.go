package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tooppoo/git-kura/internal/gitutil"
)

const sealPathSchemaVersion = 1

type sealPathStore struct {
	SchemaVersion int      `json:"schemaVersion"`
	Key           string   `json:"key"`
	Paths         []string `json:"paths"`
}

func sealPathDir(repoRoot string) (string, error) {
	commonDir, err := gitutil.CommonDir(repoRoot)
	if err != nil {
		return "", fmt.Errorf("get git common dir: %w", err)
	}
	return filepath.Join(commonDir, "kura", "seals"), nil
}

func sealPathStoreFile(sealDir, key string) string {
	return filepath.Join(sealDir, key+".json")
}

func readSealPathStore(path string) (sealPathStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sealPathStore{}, nil
		}
		return sealPathStore{}, fmt.Errorf("read seal path store: %w", err)
	}
	var store sealPathStore
	if err := json.Unmarshal(data, &store); err != nil {
		return sealPathStore{}, fmt.Errorf("parse seal path store: %w", err)
	}
	return store, nil
}

func writeSealPathStore(path string, store sealPathStore) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create seal path store dir: %w", err)
	}
	data, _ := json.Marshal(store)
	return os.WriteFile(path, data, 0o644)
}

// normalizeSealPath converts rawPath to a clean repository-relative path.
// It resolves relative paths against cwd, then makes them relative to repoRoot.
// Returns an error if the path escapes the repository.
func normalizeSealPath(repoRoot, rawPath string) (string, error) {
	abs := rawPath
	if !filepath.IsAbs(rawPath) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
		abs = filepath.Join(cwd, rawPath)
	}
	abs = filepath.Clean(abs)

	rel, err := filepath.Rel(repoRoot, abs)
	if err != nil {
		return "", fmt.Errorf("resolve path relative to repo root: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %q is outside the repository root", rawPath)
	}
	return rel, nil
}

// findKeyForPath searches all seal stores and returns the key that seals the
// given repo-relative path. Returns "" if no key seals that path.
func findKeyForPath(sealDir, repoRelPath string) (string, error) {
	entries, err := os.ReadDir(sealDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read seal dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		storePath := filepath.Join(sealDir, entry.Name())
		store, err := readSealPathStore(storePath)
		if err != nil {
			continue
		}
		for _, p := range store.Paths {
			if p == repoRelPath {
				return store.Key, nil
			}
		}
	}
	return "", nil
}

func cmdSealAdd(rawPath string) error {
	key := os.Getenv("GIT_KURA_SEAL_KEY")
	if key == "" {
		return fmt.Errorf("not in sealed session (GIT_KURA_SEAL_KEY not set), run 'git kura seal enter <key>' to start one")
	}

	repoRoot, err := gitutil.RepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}

	relPath, err := normalizeSealPath(repoRoot, rawPath)
	if err != nil {
		return err
	}

	absPath := filepath.Join(repoRoot, relPath)
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path %q does not exist", rawPath)
		}
		return fmt.Errorf("check path: %w", err)
	}

	sealDir, err := sealPathDir(repoRoot)
	if err != nil {
		return err
	}

	existingKey, err := findKeyForPath(sealDir, relPath)
	if err != nil {
		return err
	}
	if existingKey != "" && existingKey != key {
		return fmt.Errorf("path %q is already sealed under key %q", rawPath, existingKey)
	}

	storePath := sealPathStoreFile(sealDir, key)
	store, err := readSealPathStore(storePath)
	if err != nil {
		return err
	}

	for _, p := range store.Paths {
		if p == relPath {
			return nil
		}
	}

	store.SchemaVersion = sealPathSchemaVersion
	store.Key = key
	store.Paths = append(store.Paths, relPath)

	return writeSealPathStore(storePath, store)
}

func cmdSealRemove(rawPath string) error {
	key := os.Getenv("GIT_KURA_SEAL_KEY")
	if key == "" {
		return fmt.Errorf("not in sealed session (GIT_KURA_SEAL_KEY not set), run 'git kura seal enter <key>' to start one")
	}

	repoRoot, err := gitutil.RepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}

	relPath, err := normalizeSealPath(repoRoot, rawPath)
	if err != nil {
		return err
	}

	sealDir, err := sealPathDir(repoRoot)
	if err != nil {
		return err
	}

	storePath := sealPathStoreFile(sealDir, key)
	store, err := readSealPathStore(storePath)
	if err != nil {
		return err
	}

	newPaths := make([]string, 0, len(store.Paths))
	for _, p := range store.Paths {
		if p != relPath {
			newPaths = append(newPaths, p)
		}
	}

	if len(newPaths) == len(store.Paths) {
		return nil
	}

	if len(newPaths) == 0 {
		if err := os.Remove(storePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove empty seal store: %w", err)
		}
		return nil
	}

	store.Paths = newPaths
	return writeSealPathStore(storePath, store)
}
