package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tooppoo/git-kura/internal/gitutil"
)

const sealPathSchemaVersion = 1

// sealPathStore is the on-disk record written to <git-common-dir>/kura/seals.json.
// Paths maps each repository-relative file path to the key that sealed it.
type sealPathStore struct {
	SchemaVersion int               `json:"schemaVersion"`
	Paths         map[string]string `json:"paths"` // repo-relative path → key
}

func sealStoreFile(repoRoot string) (string, error) {
	commonDir, err := gitutil.CommonDir(repoRoot)
	if err != nil {
		return "", fmt.Errorf("get git common dir: %w", err)
	}
	return filepath.Join(commonDir, "kura", "seals.json"), nil
}

func readSealStore(path string) (sealPathStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sealPathStore{Paths: make(map[string]string)}, nil
		}
		return sealPathStore{}, fmt.Errorf("read seal store: %w", err)
	}
	var store sealPathStore
	if err := json.Unmarshal(data, &store); err != nil {
		return sealPathStore{}, fmt.Errorf("parse seal store: %w", err)
	}
	if store.Paths == nil {
		store.Paths = make(map[string]string)
	}
	return store, nil
}

func writeSealStore(path string, store sealPathStore) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create seal store dir: %w", err)
	}
	store.SchemaVersion = sealPathSchemaVersion
	data, _ := json.Marshal(store)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write seal store: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return errors.Join(fmt.Errorf("commit seal store: %w", err), os.Remove(tmp))
	}
	return nil
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
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the repository root", rawPath)
	}
	return rel, nil
}

// sealContext reads and validates GIT_KURA_SEAL_KEY, returning the key or an error.
func sealContext() (string, error) {
	key := os.Getenv("GIT_KURA_SEAL_KEY")
	if key == "" {
		return "", fmt.Errorf("not in sealed session (GIT_KURA_SEAL_KEY not set), run 'git kura seal enter <key>' to start one")
	}
	if err := validateKey(key); err != nil {
		return "", fmt.Errorf("GIT_KURA_SEAL_KEY is invalid: %w", err)
	}
	return key, nil
}

func cmdSealAdd(rawPaths []string) error {
	key, err := sealContext()
	if err != nil {
		return err
	}

	repoRoot, err := gitutil.RepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}

	storeFile, err := sealStoreFile(repoRoot)
	if err != nil {
		return err
	}

	store, err := readSealStore(storeFile)
	if err != nil {
		return err
	}

	changed := false
	for _, rawPath := range rawPaths {
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

		if existingKey, sealed := store.Paths[relPath]; sealed {
			if existingKey != key {
				return fmt.Errorf("path %q is already sealed under key %q", rawPath, existingKey)
			}
			continue // idempotent: already sealed under same key
		}

		store.Paths[relPath] = key
		changed = true
	}

	if !changed {
		return nil
	}
	return writeSealStore(storeFile, store)
}

func cmdSealRemove(rawPaths []string) error {
	key, err := sealContext()
	if err != nil {
		return err
	}

	repoRoot, err := gitutil.RepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}

	storeFile, err := sealStoreFile(repoRoot)
	if err != nil {
		return err
	}

	store, err := readSealStore(storeFile)
	if err != nil {
		return err
	}

	changed := false
	for _, rawPath := range rawPaths {
		relPath, err := normalizeSealPath(repoRoot, rawPath)
		if err != nil {
			return err
		}

		ownerKey, sealed := store.Paths[relPath]
		if !sealed {
			continue // idempotent: path not in store
		}
		if ownerKey != key {
			return fmt.Errorf("path %q is sealed under key %q, not the current key %q", rawPath, ownerKey, key)
		}

		delete(store.Paths, relPath)
		changed = true
	}

	if !changed {
		return nil
	}
	return writeSealStore(storeFile, store)
}
