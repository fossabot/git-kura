package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tooppoo/git-kura/internal/gitutil"
)

const defaultSessionTTL = 5 * time.Minute

type sealSession struct {
	Key          string    `json:"key"`
	WorktreePath string    `json:"worktree"`
	ParentPID    int       `json:"parent_pid"`
	ChildPID     int       `json:"child_pid"`
	StartedAt    time.Time `json:"started_at"`
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

// acquireSealSession atomically creates a session record for the given
// worktree using O_CREATE|O_EXCL on a path that is scoped to the worktree.
// Because all concurrent callers for the same worktree compete for the same
// filename, exactly one of them can create it; the rest observe the file and
// either receive a conflict error (live session) or retry once after cleaning
// up a stale session (dead PIDs).
func acquireSealSession(sessDir, worktreePath, key string, parentPID int) (string, sealSession, error) {
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		return "", sealSession{}, fmt.Errorf("create session store: %w", err)
	}

	sess := sealSession{
		Key:          key,
		WorktreePath: worktreePath,
		ParentPID:    parentPID,
		ChildPID:     0,
		StartedAt:    time.Now(),
	}
	path := sealSessionPath(sessDir, worktreePath)

	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			data, _ := json.Marshal(sess)
			_, writeErr := f.Write(data)
			f.Close()
			if writeErr != nil {
				os.Remove(path)
				return "", sealSession{}, fmt.Errorf("write session record: %w", writeErr)
			}
			return path, sess, nil
		}

		if !os.IsExist(err) {
			return "", sealSession{}, fmt.Errorf("create session record: %w", err)
		}

		// File exists: inspect it.
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", sealSession{}, fmt.Errorf("read existing session: %w", readErr)
		}
		var existing sealSession
		if jsonErr := json.Unmarshal(data, &existing); jsonErr != nil {
			// Corrupt record — remove and retry.
			os.Remove(path)
			continue
		}

		if !sessionAlive(existing) {
			// Stale — remove and retry.
			os.Remove(path)
			continue
		}

		// Active live session: report conflict.
		age := time.Since(existing.StartedAt).Truncate(time.Second)
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

	return "", sealSession{}, fmt.Errorf("could not acquire session record after retry")
}

func updateSealSession(path string, sess sealSession) error {
	data, _ := json.Marshal(sess)
	return os.WriteFile(path, data, 0o644)
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
