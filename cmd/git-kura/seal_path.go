package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tooppoo/git-kura/internal/gitutil"
)

const sealPathSchemaVersion = 1

// sealStoreLockTimeout is the maximum time to wait for the seal store lock.
// Future: make configurable via GIT_KURA_SEAL_LOCK_TIMEOUT or a config file.
var sealStoreLockTimeout = 5 * time.Second

const sealStoreLockInterval = 100 * time.Millisecond

// sealPathStore is the on-disk record at <git-common-dir>/kura/seals/paths.json.
// Paths maps each repository-relative path (forward-slash) to the key that sealed it.
type sealPathStore struct {
	SchemaVersion int               `json:"schemaVersion"`
	Paths         map[string]string `json:"paths"` // forward-slash repo-relative path → key
}

// sealStorePaths returns the store file and lock file paths for the given repo root.
func sealStorePaths(repoRoot string) (storePath, lockPath string, err error) {
	commonDir, err := gitutil.CommonDir(repoRoot)
	if err != nil {
		return "", "", fmt.Errorf("get git common dir: %w", err)
	}
	dir := filepath.Join(commonDir, "kura", "seals")
	return filepath.Join(dir, "paths.json"), filepath.Join(dir, "paths.lock"), nil
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

// acquireSealLock creates the lock file using atomic O_CREATE|O_EXCL, retrying
// until sealStoreLockTimeout (or GIT_KURA_SEAL_LOCK_TIMEOUT if set).
// Returns a release function that removes the lock file.
func acquireSealLock(lockPath string) (release func(), err error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create seal store dir: %w", err)
	}
	timeout := sealStoreLockTimeout
	if v := os.Getenv("GIT_KURA_SEAL_LOCK_TIMEOUT"); v != "" {
		if d, parseErr := time.ParseDuration(v); parseErr == nil {
			timeout = d
		}
	}
	deadline := time.Now().Add(timeout)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire seal store lock: %w", err)
		}
		if time.Now().After(deadline) {
			return nil, &exitError{
				code: exitSealLockTimeout,
				err:  fmt.Errorf("seal-lock-timeout: failed to acquire seal store lock after %s", timeout),
			}
		}
		time.Sleep(sealStoreLockInterval)
	}
}

// normalizeSealPath converts rawPath to a clean repository-relative path.
// rawPath must be relative; absolute paths are rejected per spec.
// Relative paths are resolved against cwd, then made relative to repoRoot.
// Returns an error if the path escapes the repository.
func normalizeSealPath(repoRoot, rawPath string) (string, error) {
	if filepath.IsAbs(rawPath) {
		return "", fmt.Errorf("path %q must be a relative path", rawPath)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	abs := filepath.Clean(filepath.Join(cwd, rawPath))

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

	storeFile, lockFile, err := sealStorePaths(repoRoot)
	if err != nil {
		return err
	}

	release, err := acquireSealLock(lockFile)
	if err != nil {
		return err
	}
	defer release()

	store, err := readSealStore(storeFile)
	if err != nil {
		return err
	}

	// Validate all paths before modifying the store; partial success is not allowed.
	type entry struct {
		storeKey string
		skip     bool // already sealed under current key — idempotent
	}
	entries := make([]entry, 0, len(rawPaths))
	for _, rawPath := range rawPaths {
		relPath, err := normalizeSealPath(repoRoot, rawPath)
		if err != nil {
			return err
		}
		storeKey := filepath.ToSlash(relPath)

		if _, err := os.Stat(filepath.Join(repoRoot, relPath)); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("path %q does not exist", rawPath)
			}
			return fmt.Errorf("check path: %w", err)
		}

		if existingKey, sealed := store.Paths[storeKey]; sealed {
			if existingKey != key {
				return &exitError{
					code: exitSealConflict,
					err:  fmt.Errorf("seal-conflict: path %q is already sealed by key %q", rawPath, existingKey),
				}
			}
			entries = append(entries, entry{storeKey: storeKey, skip: true})
			continue
		}
		entries = append(entries, entry{storeKey: storeKey})
	}

	changed := false
	for _, e := range entries {
		if e.skip {
			continue
		}
		store.Paths[e.storeKey] = key
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

	storeFile, lockFile, err := sealStorePaths(repoRoot)
	if err != nil {
		return err
	}

	release, err := acquireSealLock(lockFile)
	if err != nil {
		return err
	}
	defer release()

	store, err := readSealStore(storeFile)
	if err != nil {
		return err
	}

	// Validate all paths before modifying the store; partial success is not allowed.
	type entry struct {
		storeKey string
		skip     bool // not in store — idempotent no-op
	}
	entries := make([]entry, 0, len(rawPaths))
	for _, rawPath := range rawPaths {
		relPath, err := normalizeSealPath(repoRoot, rawPath)
		if err != nil {
			return err
		}
		storeKey := filepath.ToSlash(relPath)

		ownerKey, sealed := store.Paths[storeKey]
		if !sealed {
			entries = append(entries, entry{storeKey: storeKey, skip: true})
			continue
		}
		if ownerKey != key {
			return &exitError{
				code: exitSealConflict,
				err:  fmt.Errorf("seal-conflict: path %q is sealed by key %q, not the current key %q", rawPath, ownerKey, key),
			}
		}
		entries = append(entries, entry{storeKey: storeKey})
	}

	changed := false
	for _, e := range entries {
		if e.skip {
			continue
		}
		delete(store.Paths, e.storeKey)
		changed = true
	}
	if !changed {
		return nil
	}
	return writeSealStore(storeFile, store)
}
