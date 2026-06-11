package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func sealSessionFilePath(sessDir string, parentPID int) string {
	return filepath.Join(sessDir, fmt.Sprintf("%d.json", parentPID))
}

// checkAndCleanSessions scans the session store for active sessions on the
// given worktree. Stale sessions (dead PIDs) are removed. Returns an error
// if a live session with a different key exists for the same worktree.
func checkAndCleanSessions(sessDir, worktreePath, key string) error {
	entries, err := os.ReadDir(sessDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read session store: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		sessPath := filepath.Join(sessDir, entry.Name())
		data, err := os.ReadFile(sessPath)
		if err != nil {
			continue
		}

		var s sealSession
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}

		if s.WorktreePath != worktreePath {
			continue
		}

		if !sessionAlive(s) {
			os.Remove(sessPath)
			continue
		}

		if s.Key != key {
			age := time.Since(s.StartedAt).Truncate(time.Second)
			msg := fmt.Sprintf(
				"worktree %q already has an active seal session for key %q (started %v ago, parent PID %d, child PID %d)",
				worktreePath, s.Key, age, s.ParentPID, s.ChildPID,
			)
			if age > defaultSessionTTL {
				msg += " [TTL exceeded — may be stale; verify PIDs manually]"
			}
			return fmt.Errorf("%s", msg)
		}
	}

	return nil
}

// createSealSession atomically creates a session record file using O_CREATE|O_EXCL.
// If a file with the same PID already exists and is stale (PID reuse), it is
// removed and creation is retried once.
func createSealSession(sessDir string, sess sealSession) (string, error) {
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		return "", fmt.Errorf("create session store: %w", err)
	}

	path := sealSessionFilePath(sessDir, sess.ParentPID)

	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			data, _ := json.Marshal(sess)
			_, writeErr := f.Write(data)
			f.Close()
			if writeErr != nil {
				os.Remove(path)
				return "", fmt.Errorf("write session record: %w", writeErr)
			}
			return path, nil
		}

		if !os.IsExist(err) {
			return "", fmt.Errorf("create session record: %w", err)
		}

		// File already exists: only remove it if the recorded PIDs are dead (PID reuse).
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", fmt.Errorf("read existing session record: %w", readErr)
		}
		var existing sealSession
		if jsonErr := json.Unmarshal(data, &existing); jsonErr != nil || sessionAlive(existing) {
			return "", fmt.Errorf("session record for PID %d already exists and is not stale", sess.ParentPID)
		}
		os.Remove(path)
	}

	return "", fmt.Errorf("could not create session record after retry")
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
