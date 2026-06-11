package main

import (
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

// acquireSealSession atomically acquires a session record for the given worktree.
//
// It first writes complete JSON to a temp file, then calls os.Link(tmp, final).
// Because os.Link is atomic and fails with EEXIST when the target already exists,
// the final path always contains complete JSON — partial writes are never visible
// to concurrent callers, which eliminates the corrupt-record race that arises when
// O_CREATE|O_EXCL is used directly (empty file visible between create and write).
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

	finalPath := sealSessionPath(sessDir, worktreePath)
	// Per-PID temp path: unique so concurrent callers don't clobber each other's writes.
	tmpPath := finalPath + ".tmp." + strconv.Itoa(parentPID)

	data, _ := json.Marshal(sess)
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return "", sealSession{}, fmt.Errorf("write temp session file: %w", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		// Atomic: succeeds only if finalPath does not yet exist.
		if err := os.Link(tmpPath, finalPath); err == nil {
			os.Remove(tmpPath)
			return finalPath, sess, nil
		} else if !os.IsExist(err) {
			os.Remove(tmpPath)
			return "", sealSession{}, fmt.Errorf("acquire session lock: %w", err)
		}

		// finalPath already exists — always complete JSON (also written via Link).
		existingData, readErr := os.ReadFile(finalPath)
		if readErr != nil {
			// Unreadable: treat as locked, do not delete.
			os.Remove(tmpPath)
			return "", sealSession{}, fmt.Errorf("session is locked or unreadable: %w", readErr)
		}
		var existing sealSession
		if jsonErr := json.Unmarshal(existingData, &existing); jsonErr != nil {
			// Corrupt: treat as locked, do not delete — caller resolves manually.
			os.Remove(tmpPath)
			return "", sealSession{}, fmt.Errorf("session file for this worktree is locked or corrupt; verify state in %s", finalPath)
		}

		if !sessionAlive(existing) {
			os.Remove(finalPath)
			continue // stale — retry
		}

		// Active live session: report conflict.
		os.Remove(tmpPath)
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

	os.Remove(tmpPath)
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
