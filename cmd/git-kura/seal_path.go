package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tooppoo/git-kura/internal/gitutil"
)

const sealPathSchemaVersion = 1

//go:embed schema/seal_store.schema.json
var sealStoreSchemaJSON []byte

var sealStoreSchema = mustCompileSealStoreSchema()

func mustCompileSealStoreSchema() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(sealStoreSchemaJSON))
	if err != nil {
		panic(fmt.Sprintf("parse seal store schema: %v", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("seal_store.schema.json", doc); err != nil {
		panic(fmt.Sprintf("add seal store schema resource: %v", err))
	}
	sch, err := c.Compile("seal_store.schema.json")
	if err != nil {
		panic(fmt.Sprintf("compile seal store schema: %v", err))
	}
	return sch
}

// validateSealStoreJSON checks that raw store JSON conforms to
// schema/seal_store.schema.json.
func validateSealStoreJSON(data []byte) error {
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("parse seal store: %w", err)
	}
	if err := sealStoreSchema.Validate(inst); err != nil {
		return fmt.Errorf("seal store does not conform to schema: %w", err)
	}
	return nil
}

// sealStoreLockTimeout is the maximum time to wait for the seal store lock.
// Future: make configurable via GIT_KURA_SEAL_LOCK_TIMEOUT or a config file.
var sealStoreLockTimeout = 5 * time.Second

const sealStoreLockInterval = 100 * time.Millisecond

// sealEntry records how a path is sealed. It is a struct rather than a bare
// key string so future fields (e.g. sealedAt, agent) can be added without a
// breaking schema change. Schema: schema/seal_store.schema.json.
type sealEntry struct {
	Key string `json:"key"`
}

// sealPathStore is the on-disk record at <git-common-dir>/kura/seals/paths.json.
// Paths maps each repository-relative path (forward-slash) to its seal entry.
type sealPathStore struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Paths         map[string]sealEntry `json:"paths"`
}

// pathsSealStore returns the store file and lock file locations for the given
// repo root.
func pathsSealStore(repoRoot string) (storePath, lockPath string, err error) {
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
			return sealPathStore{Paths: make(map[string]sealEntry)}, nil
		}
		return sealPathStore{}, fmt.Errorf("read seal store: %w", err)
	}
	// Validate before unmarshalling so a hand-edited or corrupted store is
	// rejected instead of being silently coerced into the Go struct.
	if err := validateSealStoreJSON(data); err != nil {
		return sealPathStore{}, fmt.Errorf("read seal store %s: %w", path, err)
	}
	var store sealPathStore
	if err := json.Unmarshal(data, &store); err != nil {
		return sealPathStore{}, fmt.Errorf("parse seal store: %w", err)
	}
	if store.Paths == nil {
		store.Paths = make(map[string]sealEntry)
	}
	return store, nil
}

func writeSealStore(path string, store sealPathStore) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create seal store dir: %w", err)
	}
	store.SchemaVersion = sealPathSchemaVersion
	if store.Paths == nil {
		store.Paths = make(map[string]sealEntry)
	}
	data, _ := json.Marshal(store)
	// Validate before writing so a bug can never persist a store that other
	// readers (or the future commit hook) would reject.
	if err := validateSealStoreJSON(data); err != nil {
		return fmt.Errorf("refusing to write seal store: %w", err)
	}
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
// Returns a release function that removes the lock file. If removal fails the
// lock would block all future seal commands, so the failure is reported on
// stderr with the lock path so the user can remove it manually.
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
			return func() {
				if removeErr := os.Remove(lockPath); removeErr != nil && !os.IsNotExist(removeErr) {
					fmt.Fprintf(os.Stderr,
						"warning: failed to release seal store lock %s: %v\nremove the file manually or subsequent seal commands will time out\n",
						lockPath, removeErr)
				}
			}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire seal store lock: %w", err)
		}
		if time.Now().After(deadline) {
			return nil, exitCodeError(exitSealLockTimeout,
				fmt.Errorf("seal-lock-timeout: failed to acquire seal store lock after %s", timeout))
		}
		time.Sleep(sealStoreLockInterval)
	}
}

// normalizeSealPath converts rawPath to a clean repository-relative path.
// rawPath must be relative and is interpreted relative to the repository
// root — never the caller's working directory — so the same argument always
// resolves to the same file. Returns an error for absolute paths and paths
// that escape the repository.
func normalizeSealPath(repoRoot, rawPath string) (string, error) {
	if filepath.IsAbs(rawPath) {
		return "", fmt.Errorf("path %q must be relative to the repository root", rawPath)
	}
	abs := filepath.Clean(filepath.Join(repoRoot, filepath.FromSlash(rawPath)))

	rel, err := filepath.Rel(repoRoot, abs)
	if err != nil {
		return "", fmt.Errorf("resolve path relative to repo root: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the repository root", rawPath)
	}
	return rel, nil
}

func readSealContext() (string, error) {
	key := os.Getenv("GIT_KURA_SEAL_KEY")
	if key == "" {
		return "", fmt.Errorf("not in sealed session (GIT_KURA_SEAL_KEY not set), run 'git kura seal enter <key>' to start one")
	}
	if err := validateKey(key); err != nil {
		return "", fmt.Errorf("GIT_KURA_SEAL_KEY is invalid: %w", err)
	}
	return key, nil
}

// sealConflict records one path that could not be added/removed because it is
// sealed by a key other than the current one.
type sealConflict struct {
	path     string // path as given by the user
	sealedBy string // key that currently seals the path
}

// sealConflictError builds the seal-conflict error listing every conflicting
// path and the key that seals it, so the user can see all blockers at once.
func sealConflictError(conflicts []sealConflict) error {
	parts := make([]string, 0, len(conflicts))
	for _, c := range conflicts {
		parts = append(parts, fmt.Sprintf("path %q is already sealed by key %q", c.path, c.sealedBy))
	}
	return exitCodeError(exitSealConflict,
		fmt.Errorf("seal-conflict: %s", strings.Join(parts, "; ")))
}

func cmdSealAdd(rawPaths []string) error {
	key, err := readSealContext()
	if err != nil {
		return err
	}

	repoRoot, err := gitutil.RepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}

	storeFile, lockFile, err := pathsSealStore(repoRoot)
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

	// Validate all paths before modifying the store; partial success is not
	// allowed. Cross-key conflicts are collected so the error reports every
	// conflicting path with the key that seals it.
	var conflicts []sealConflict
	toAdd := make([]string, 0, len(rawPaths))
	for _, rawPath := range rawPaths {
		relPath, err := normalizeSealPath(repoRoot, rawPath)
		if err != nil {
			return err
		}
		storeKey := filepath.ToSlash(relPath)

		info, err := os.Stat(filepath.Join(repoRoot, relPath))
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("path %q does not exist", rawPath)
			}
			return fmt.Errorf("check path: %w", err)
		}
		// Only files can be sealed; directory seals are out of scope (see
		// docs/adr/20260611T114624Z-limit-seal-targets-to-repository-relative-files.md).
		if info.IsDir() {
			return fmt.Errorf("path %q is a directory; only files can be sealed", rawPath)
		}

		if entry, sealed := store.Paths[storeKey]; sealed {
			if entry.Key != key {
				conflicts = append(conflicts, sealConflict{path: rawPath, sealedBy: entry.Key})
			}
			// Already sealed under the current key: idempotent, nothing to write.
			continue
		}
		toAdd = append(toAdd, storeKey)
	}

	if len(conflicts) > 0 {
		return sealConflictError(conflicts)
	}
	if len(toAdd) == 0 {
		return nil
	}
	for _, storeKey := range toAdd {
		store.Paths[storeKey] = sealEntry{Key: key}
	}
	return writeSealStore(storeFile, store)
}

func cmdSealRemove(rawPaths []string) error {
	key, err := readSealContext()
	if err != nil {
		return err
	}

	repoRoot, err := gitutil.RepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}

	storeFile, lockFile, err := pathsSealStore(repoRoot)
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

	// Validate all paths before modifying the store; partial success is not
	// allowed. Cross-key conflicts are collected so the error reports every
	// conflicting path with the key that seals it.
	var conflicts []sealConflict
	toRemove := make([]string, 0, len(rawPaths))
	for _, rawPath := range rawPaths {
		relPath, err := normalizeSealPath(repoRoot, rawPath)
		if err != nil {
			return err
		}
		storeKey := filepath.ToSlash(relPath)

		entry, sealed := store.Paths[storeKey]
		if !sealed {
			// Removing a path that was never sealed: idempotent no-op.
			continue
		}
		if entry.Key != key {
			conflicts = append(conflicts, sealConflict{path: rawPath, sealedBy: entry.Key})
			continue
		}
		toRemove = append(toRemove, storeKey)
	}

	if len(conflicts) > 0 {
		return sealConflictError(conflicts)
	}
	if len(toRemove) == 0 {
		return nil
	}
	for _, storeKey := range toRemove {
		delete(store.Paths, storeKey)
	}
	return writeSealStore(storeFile, store)
}
