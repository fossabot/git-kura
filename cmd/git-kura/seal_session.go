package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tooppoo/git-kura/internal/gitutil"
)

const defaultSessionTTL = 5 * time.Minute

const sealSessionSchemaVersion = 1

// sealSession is the on-disk record written to <git-common-dir>/kura/sessions/*.json.
// See schema/session.schema.json for the full field specification.
type sealSession struct {
	SchemaVersion int       `json:"schemaVersion"`
	Key           string    `json:"key"`
	WorktreePath  string    `json:"worktree"`
	ParentPID     int       `json:"parent_pid"`
	ChildPID      int       `json:"child_pid"`
	StartedAt     time.Time `json:"started_at"`
}

func sealSessionDir(repoRoot string) (string, error) {
	commonDir, err := gitutil.CommonDir(repoRoot)
	if err != nil {
		return "", fmt.Errorf("get git common dir: %w", err)
	}
	return filepath.Join(commonDir, "kura", "sessions"), nil
}

// sealSessionPath returns the session file path for the given worktree.
// One file per worktree means that the O_CREATE|O_EXCL in acquireSealSession
// covers the entire check-then-create as a single atomic operation, which
// eliminates the scan-then-create race that would occur if separate paths
// were used per process.
func sealSessionPath(sessDir, worktreePath string) string {
	h := sha256.Sum256([]byte(worktreePath))
	return filepath.Join(sessDir, hex.EncodeToString(h[:8])+".json")
}

// acquireSealSession atomically acquires a session record for the given worktree.
//
// It writes complete JSON to a per-PID temp file, then calls os.Link(tmp, final).
// os.Link is atomic and fails with EEXIST when the target already exists, so the
// final path always contains complete JSON — partial writes are never visible.
//
// Stale sessions (dead PIDs) are NOT removed automatically. Any unconditional
// removal of finalPath races with concurrent callers that may have linked a new
// live session between the stale read and the remove call.  The caller must
// remove the session file manually after verifying it is truly orphaned.
func acquireSealSession(sessDir, worktreePath, key string, parentPID int) (string, sealSession, error) {
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		return "", sealSession{}, fmt.Errorf("create session store: %w", err)
	}

	sess := sealSession{
		SchemaVersion: sealSessionSchemaVersion,
		Key:           key,
		WorktreePath:  worktreePath,
		ParentPID:     parentPID,
		// ChildPID is 0 (sentinel) until cmd.Start() returns the child PID.
		// The session must be acquired before the child is launched to prevent
		// a race between concurrent seal enter calls for the same worktree.
		ChildPID:  0,
		StartedAt: time.Now(),
	}

	finalPath := sealSessionPath(sessDir, worktreePath)
	// Per-PID temp path so concurrent callers don't clobber each other's writes.
	tmpPath := finalPath + ".tmp." + strconv.Itoa(parentPID)

	data, _ := json.Marshal(sess)
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return "", sealSession{}, fmt.Errorf("write temp session file: %w", err)
	}

	if err := os.Link(tmpPath, finalPath); err == nil {
		return finalPath, sess, os.Remove(tmpPath)
	} else if !os.IsExist(err) {
		return "", sealSession{}, errors.Join(fmt.Errorf("acquire session lock: %w", err), os.Remove(tmpPath))
	}

	// finalPath exists — always complete JSON (written via Link or atomic rename).
	if removeErr := os.Remove(tmpPath); removeErr != nil {
		return "", sealSession{}, fmt.Errorf("acquire session: clean up temp file: %w", removeErr)
	}
	existingData, readErr := os.ReadFile(finalPath)
	if readErr != nil {
		return "", sealSession{}, fmt.Errorf("session is locked or unreadable: %w", readErr)
	}
	var existing sealSession
	if jsonErr := json.Unmarshal(existingData, &existing); jsonErr != nil {
		return "", sealSession{}, fmt.Errorf("session file for this worktree is locked or corrupt; verify state in %s", finalPath)
	}

	age := time.Since(existing.StartedAt).Truncate(time.Second)

	if !sessionAlive(existing) {
		return "", sealSession{}, fmt.Errorf(
			"worktree %q has a stale seal session for key %q "+
				"(started %v ago, parent PID %d and child PID %d appear dead); "+
				"remove it manually to proceed: %s",
			worktreePath, existing.Key, age, existing.ParentPID, existing.ChildPID, finalPath,
		)
	}

	if existing.Key == key {
		return "", sealSession{}, fmt.Errorf(
			"seal session for key %q is already active for this worktree (started %v ago, parent PID %d)",
			key, age, existing.ParentPID,
		)
	}
	msg := fmt.Sprintf(
		"worktree %q already has an active seal session for key %q (started %v ago, parent PID %d, child PID %d)",
		worktreePath, existing.Key, age, existing.ParentPID, existing.ChildPID,
	)
	if age > defaultSessionTTL {
		msg += " [TTL exceeded — may be stale; verify PIDs manually]"
	}
	return "", sealSession{}, fmt.Errorf("%s", msg)
}

// updateSealSession atomically replaces the session file using a temp-then-rename
// strategy. os.Rename on POSIX is atomic (rename(2)); on Windows it is best-effort
// but still avoids the truncate-before-write window that os.WriteFile would leave.
func updateSealSession(path string, sess sealSession) error {
	data, _ := json.Marshal(sess)
	tmp := path + ".tmp." + strconv.Itoa(sess.ParentPID)
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	return nil
}

func deleteSealSession(path string) error {
	return os.Remove(path)
}

func sessionAlive(s sealSession) bool {
	if !pidAlive(s.ParentPID) {
		return false
	}
	if s.ChildPID != 0 && !pidAlive(s.ChildPID) {
		return false
	}
	return true
}

// sessionTTL returns the configured session TTL.
// It reads GIT_KURA_SESSION_TTL (a Go duration string, e.g. "10m") and falls
// back to defaultSessionTTL when unset or invalid.
func sessionTTL() time.Duration {
	if v := os.Getenv("GIT_KURA_SESSION_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultSessionTTL
}

const (
	sessionStatusActive         = "active"
	sessionStatusStaleCandidate = "stale-candidate (ttl exceeded)"
	sessionStatusStale          = "stale"
	sessionStatusUnknown        = "unknown"
)

// sessionStatusStr classifies a seal session for human display.
// Priority: dead PIDs → stale; TTL exceeded → stale-candidate; child not yet
// recorded → unknown; otherwise active.
func sessionStatusStr(s sealSession, ttl time.Duration) string {
	if !sessionAlive(s) {
		return sessionStatusStale
	}
	if time.Since(s.StartedAt) > ttl {
		return sessionStatusStaleCandidate
	}
	if s.ChildPID == 0 {
		return sessionStatusUnknown
	}
	return sessionStatusActive
}

type sealSessionFile struct {
	Path    string
	Session sealSession
	// Err is non-nil when the file could not be read or contains invalid JSON.
	// A corrupt session file blocks seal enter for its worktree, so callers must
	// surface it rather than silently ignoring it.
	Err error
}

// readAllSealSessions returns all session records (valid and invalid) found in sessDir.
// Unreadable files are included with Err set to describe the problem; corrupt JSON files
// are included with Err = "corrupt". Non-existent directory returns nil, nil.
func readAllSealSessions(sessDir string) ([]sealSessionFile, error) {
	entries, err := os.ReadDir(sessDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session dir: %w", err)
	}
	var records []sealSessionFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(sessDir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			records = append(records, sealSessionFile{Path: path, Err: fmt.Errorf("unreadable: %w", readErr)})
			continue
		}
		var sess sealSession
		if jsonErr := json.Unmarshal(data, &sess); jsonErr != nil {
			records = append(records, sealSessionFile{Path: path, Err: fmt.Errorf("corrupt")})
			continue
		}
		records = append(records, sealSessionFile{Path: path, Session: sess})
	}
	return records, nil
}

// removeStaleSession removes path only when it still contains the same stale session
// that was read earlier. It re-reads and re-checks liveness immediately before removal
// to prevent deleting a live session that raced into the same path after the initial scan.
// Returns (true, nil) if removed, (false, nil) if skipped, or (false, err) on I/O failure.
func removeStaleSession(path string, original sealSession) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("re-read session before removal: %w", err)
	}
	var current sealSession
	if err := json.Unmarshal(data, &current); err != nil {
		// File is now corrupt; leave it for manual inspection
		return false, nil
	}
	// A different session was created at this path since the initial scan
	if !current.StartedAt.Equal(original.StartedAt) {
		return false, nil
	}
	// Re-check liveness — the session might have been restarted
	if sessionAlive(current) {
		return false, nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("remove stale session: %w", err)
	}
	return true, nil
}
