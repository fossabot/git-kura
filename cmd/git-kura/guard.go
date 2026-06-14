package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tooppoo/git-kura/internal/gitutil"
	"github.com/tooppoo/git-kura/internal/worktree"
)

const guardHelp = `Usage: git kura guard <subcommand>

Manage the cooperative worktree guard for the current managed worktree.

A guard is a cooperative lease over a single git-kura managed worktree. It
prevents two agents from starting work in the same worktree at the same time,
where they would share one working tree and index. This is distinct from
"git kura seal", which detects cross-worktree file conflicts.

The guard key is the current managed worktree's key; it is never passed as an
argument. Run these commands from inside the worktree created by
"git kura open <key>" (any subdirectory of it works).

This is a cooperative guard, not an OS-level lock: a process that ignores
git-kura can still use the same worktree.

Subcommands:
  acquire   Acquire the guard for the current worktree
  release   Release the guard for the current worktree
  status    Print the guard status for the current worktree

Run "git kura guard <subcommand> --help" for subcommand-specific help.`

const guardAcquireHelp = `Usage: git kura guard acquire

Acquire the cooperative guard for the current managed worktree.

The guard key is the current managed worktree's key. acquire fails when the
current directory is not inside a managed worktree, or when that worktree's
metadata is missing or inconsistent.

The guard record is created atomically with an exclusive create (O_CREATE |
O_EXCL): there is no check-then-write window, so at most one concurrent acquire
can succeed. If an active guard already exists for the worktree, acquire exits
with guard-active (code 8). A pre-existing guard record is always treated as an
active guard and is never inspected or auto-removed, so a corrupted record also
blocks acquire rather than being silently replaced.`

const guardReleaseHelp = `Usage: git kura guard release

Release the cooperative guard for the current managed worktree.

The guard key is the current managed worktree's key. release fails when the
current directory is not inside a managed worktree, or when that worktree's
metadata is missing or inconsistent.

If a guard record exists it is removed. If no guard record exists, release is a
no-op that prints a warning and exits 0. release does not require a token in
v0: any agent inside the worktree may release its guard.`

const guardStatusHelp = `Usage: git kura guard status

Print the cooperative guard status for the current managed worktree.

The guard key is the current managed worktree's key. status fails when the
current directory is not inside a managed worktree, or when that worktree's
metadata is missing or inconsistent.

Output is one "field: value" line per field:

  guarded: true | false
  key: <key>
  worktreePath: <absolute path>
  createdAt: <RFC3339 timestamp>   (only when guarded)

The key and worktree path are derived from the current worktree, so status
reports a consistent guarded state even when an existing guard record cannot be
parsed.`

func runGuard(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: git kura guard <subcommand>")
	}

	switch args[0] {
	case "-h", "--help":
		fmt.Println(guardHelp)
		return nil
	case "acquire":
		if hasHelpFlag(args[1:]) {
			fmt.Println(guardAcquireHelp)
			return nil
		}
		if err := parseGuardNoArgs("acquire", args[1:]); err != nil {
			return err
		}
		return cmdGuardAcquire()
	case "release":
		if hasHelpFlag(args[1:]) {
			fmt.Println(guardReleaseHelp)
			return nil
		}
		if err := parseGuardNoArgs("release", args[1:]); err != nil {
			return err
		}
		return cmdGuardRelease()
	case "status":
		if hasHelpFlag(args[1:]) {
			fmt.Println(guardStatusHelp)
			return nil
		}
		if err := parseGuardNoArgs("status", args[1:]); err != nil {
			return err
		}
		return cmdGuardStatus()
	default:
		return fmt.Errorf("unknown guard subcommand: %s", args[0])
	}
}

// parseGuardNoArgs rejects every positional argument and option. The guard
// subcommands take no arguments in v0: the guard key is always derived from the
// current managed worktree, never passed in, so an unexpected token must error
// rather than be silently ignored.
func parseGuardNoArgs(sub string, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf("usage: git kura guard %s: unexpected argument %q", sub, args[0])
}

//go:embed schema/guard_record.schema.json
var guardRecordSchemaJSON []byte

var guardRecordSchema = mustCompileGuardRecordSchema()

func mustCompileGuardRecordSchema() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(guardRecordSchemaJSON))
	if err != nil {
		panic(fmt.Sprintf("parse guard record schema: %v", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("guard_record.schema.json", doc); err != nil {
		panic(fmt.Sprintf("add guard record schema resource: %v", err))
	}
	sch, err := c.Compile("guard_record.schema.json")
	if err != nil {
		panic(fmt.Sprintf("compile guard record schema: %v", err))
	}
	return sch
}

// guardRecord is the on-disk record at
// <git-common-dir>/kura/guards/worktrees/<key>.json. v0 intentionally omits
// agent identity, token, heartbeat, and updatedAt fields; the record's mere
// existence marks the worktree as guarded. Schema: schema/guard_record.schema.json.
type guardRecord struct {
	Key          string `json:"key"`
	WorktreePath string `json:"worktreePath"`
	CreatedAt    string `json:"createdAt"`
}

// readGuardContext resolves the current managed worktree, returning its guard
// key and absolute worktree path. It fails when the current directory is not
// inside a managed worktree or that worktree's metadata is inconsistent.
func readGuardContext() (key, worktreePath string, err error) {
	worktreePath, err = gitutil.RepoRoot()
	if err != nil {
		return "", "", fmt.Errorf("not inside a git repository")
	}
	key, err = worktree.CurrentKey(worktreePath)
	if err != nil {
		return "", "", err
	}
	return key, worktreePath, nil
}

// guardRecordPath returns the guard record location for the given worktree key.
// Guards live under the Git common dir so the record is shared by every view of
// the repository, matching where managed worktree state already lives.
func guardRecordPath(worktreePath, key string) (string, error) {
	commonDir, err := gitutil.CommonDir(worktreePath)
	if err != nil {
		return "", fmt.Errorf("get git common dir: %w", err)
	}
	dir := filepath.Join(commonDir, "kura", "guards", "worktrees")
	return filepath.Join(dir, key+".json"), nil
}

// marshalGuardRecord serializes record and verifies it against the guard record
// schema before it is written, so a bug can never persist a record that other
// readers would reject.
func marshalGuardRecord(record guardRecord) ([]byte, error) {
	data, _ := json.Marshal(record)
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse guard record: %w", err)
	}
	if err := guardRecordSchema.Validate(inst); err != nil {
		return nil, fmt.Errorf("refusing to write guard record: %w", err)
	}
	return data, nil
}

func cmdGuardAcquire() error {
	key, worktreePath, err := readGuardContext()
	if err != nil {
		return err
	}

	recordPath, err := guardRecordPath(worktreePath, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		return fmt.Errorf("create guard store dir: %w", err)
	}

	data, err := marshalGuardRecord(guardRecord{
		Key:          key,
		WorktreePath: worktreePath,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}

	// Atomic acquire: an exclusive create is the whole exclusion mechanism.
	// There is no read-then-decide step, so concurrent acquires cannot both
	// succeed, and an existing (even corrupted) record always blocks acquire.
	f, err := os.OpenFile(recordPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return exitCodeError(exitGuardConflict,
				fmt.Errorf("guard-active: this worktree is already guarded"))
		}
		return fmt.Errorf("create guard record: %w", err)
	}
	if _, werr := f.Write(data); werr != nil {
		// Remove the just-created record so a write failure does not leave a
		// guard that blocks every future acquire and release.
		return errors.Join(fmt.Errorf("write guard record: %w", werr), f.Close(), os.Remove(recordPath))
	}
	if cerr := f.Close(); cerr != nil {
		return fmt.Errorf("close guard record: %w", cerr)
	}
	return nil
}

func cmdGuardRelease() error {
	key, worktreePath, err := readGuardContext()
	if err != nil {
		return err
	}

	recordPath, err := guardRecordPath(worktreePath, key)
	if err != nil {
		return err
	}

	if err := os.Remove(recordPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: no active guard to release for key %q\n", key)
			return nil
		}
		return fmt.Errorf("remove guard record: %w", err)
	}
	return nil
}

func cmdGuardStatus() error {
	key, worktreePath, err := readGuardContext()
	if err != nil {
		return err
	}

	recordPath, err := guardRecordPath(worktreePath, key)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(recordPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("guarded: false\nkey: %s\nworktreePath: %s\n", key, worktreePath)
			return nil
		}
		return fmt.Errorf("read guard record: %w", err)
	}

	// The guarded state, key, and worktree path come from the current worktree,
	// so status stays accurate even if the record cannot be parsed. Only
	// createdAt is read from the record, and it is omitted when unavailable.
	fmt.Printf("guarded: true\nkey: %s\nworktreePath: %s\n", key, worktreePath)
	var record guardRecord
	if json.Unmarshal(data, &record) == nil && record.CreatedAt != "" {
		fmt.Printf("createdAt: %s\n", record.CreatedAt)
	}
	return nil
}
