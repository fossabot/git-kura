package main

import (
	_ "embed"

	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/tooppoo/git-kura/internal/gitutil"
)

//go:embed schema/session.schema.json
var sealSessionSchemaJSON []byte

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
		os.Remove(tmpPath) //nolint:errcheck
		return finalPath, sess, nil
	} else if !os.IsExist(err) {
		os.Remove(tmpPath) //nolint:errcheck
		return "", sealSession{}, fmt.Errorf("acquire session lock: %w", err)
	}

	// finalPath exists — always complete JSON (written via Link or atomic rename).
	os.Remove(tmpPath) //nolint:errcheck
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
		os.Remove(tmp)
		return err
	}
	return nil
}

func deleteSealSession(path string) {
	os.Remove(path)
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
