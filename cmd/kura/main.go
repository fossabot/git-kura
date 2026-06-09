package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schema/output.schema.json
var outputSchemaJSON []byte

var outputSchema = mustCompileOutputSchema()

func mustCompileOutputSchema() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(outputSchemaJSON))
	if err != nil {
		panic(fmt.Sprintf("parse output schema: %v", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("output.schema.json", doc); err != nil {
		panic(fmt.Sprintf("add output schema resource: %v", err))
	}
	sch, err := c.Compile("output.schema.json")
	if err != nil {
		panic(fmt.Sprintf("compile output schema: %v", err))
	}
	return sch
}

// Entrypoint and dispatch

const topLevelHelp = `Usage: git kura <command> <key> [flags]

Commands:
  get   <key> [flags]  Print worktree path, branch, or structured metadata
  open  <key>          Create a worktree for <key>
  close <key>          Remove the worktree for <key>

Run "git kura <command> --help" for command-specific help.`

const getHelp = `Usage: git kura get <key> [flags]

Print worktree information for <key>.

Flags:
  --path          Print the worktree filesystem path (default)
  --branch        Print the branch name
  --json          Print structured metadata as JSON
  --toon          Print structured metadata as TOML-like text
  --format json   Same as --json
  --format toon   Same as --toon`

const openHelp = `Usage: git kura open <key>

Create a git worktree for <key> on a new branch kura-<key>.`

const closeHelp = `Usage: git kura close <key>

Remove the git worktree for <key>.`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: git kura <command> [key] [flags]")
	}

	switch args[0] {
	case "-h", "--help":
		fmt.Println(topLevelHelp)
		return nil

	case "get":
		if hasHelpFlag(args[1:]) {
			fmt.Println(getHelp)
			return nil
		}
		key, opts, err := parseGetArgs(args[1:])
		if err != nil {
			return err
		}
		return cmdGet(key, opts)

	case "open":
		if hasHelpFlag(args[1:]) {
			fmt.Println(openHelp)
			return nil
		}
		key, err := parseKeyOnlyArgs("open", args[1:])
		if err != nil {
			return err
		}
		return cmdOpen(key)

	case "close":
		if hasHelpFlag(args[1:]) {
			fmt.Println(closeHelp)
			return nil
		}
		key, err := parseKeyOnlyArgs("close", args[1:])
		if err != nil {
			return err
		}
		return cmdClose(key)

	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func hasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

// CLI parsing turns raw argv slices into typed command inputs.
// The command functions below should not inspect raw CLI arguments.

type outputMode string

const (
	outputPath   outputMode = "path"
	outputBranch outputMode = "branch"
	outputJSON   outputMode = "json"
	outputTOON   outputMode = "toon"
)

type getOptions struct {
	OutputMode outputMode
}

func parseGetArgs(args []string) (string, getOptions, error) {
	if len(args) == 0 {
		return "", getOptions{}, fmt.Errorf("usage: git kura get <key> [--path|--branch|--json|--toon|--format <fmt>]")
	}

	key := args[0]
	if err := validateKey(key); err != nil {
		return "", getOptions{}, err
	}

	var mode outputMode
	flags := args[1:]
	for i := 0; i < len(flags); i++ {
		switch flags[i] {
		case "--path":
			if mode != "" {
				return "", getOptions{}, fmt.Errorf("conflict: --%s and --path cannot be used together", mode)
			}
			mode = outputPath
		case "--branch":
			if mode != "" {
				return "", getOptions{}, fmt.Errorf("conflict: --%s and --branch cannot be used together", mode)
			}
			mode = outputBranch
		case "--json":
			if mode != "" {
				return "", getOptions{}, fmt.Errorf("conflict: --%s and --json cannot be used together", mode)
			}
			mode = outputJSON
		case "--toon":
			if mode != "" {
				return "", getOptions{}, fmt.Errorf("conflict: --%s and --toon cannot be used together", mode)
			}
			mode = outputTOON
		case "--format":
			if i+1 >= len(flags) {
				return "", getOptions{}, fmt.Errorf("--format requires a value (json or toon)")
			}
			i++
			fmtVal := flags[i]
			switch fmtVal {
			case "json":
				if mode != "" {
					return "", getOptions{}, fmt.Errorf("conflict: --%s and --format json cannot be used together", mode)
				}
				mode = outputJSON
			case "toon":
				if mode != "" {
					return "", getOptions{}, fmt.Errorf("conflict: --%s and --format toon cannot be used together", mode)
				}
				mode = outputTOON
			default:
				return "", getOptions{}, fmt.Errorf("unknown format %q: valid formats are json, toon", fmtVal)
			}
		default:
			return "", getOptions{}, fmt.Errorf("unknown flag: %s", flags[i])
		}
	}

	if mode == "" {
		mode = outputPath
	}

	return key, getOptions{OutputMode: mode}, nil
}

func parseKeyOnlyArgs(command string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: git kura %s <key>", command)
	}

	key := args[0]
	if err := validateKey(key); err != nil {
		return "", err
	}

	if len(args) > 1 {
		return "", fmt.Errorf("usage: git kura %s <key>: unexpected argument %q", command, args[1])
	}

	return key, nil
}

// Command execution

func cmdGet(key string, opts getOptions) error {
	repoRoot, err := gitRepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}

	branch := branchName(key)
	path, err := worktreePath(repoRoot, key)
	if err != nil {
		return fmt.Errorf("resolve worktree path: %w", err)
	}

	if opts.OutputMode == outputPath {
		fmt.Println(path)
		return nil
	}
	if opts.OutputMode == outputBranch {
		fmt.Println(branch)
		return nil
	}

	// outputJSON and outputTOON require full worktree data.
	_, statErr := os.Stat(path)
	exists := statErr == nil

	dirty := false
	if exists {
		if dirty, err = worktreeDirty(path); err != nil {
			return fmt.Errorf("check worktree status: %w", err)
		}
	}

	var base string
	if exists {
		meta, metaErr := readMetadata(repoRoot, key)
		if metaErr != nil {
			return fmt.Errorf("metadata for %q: %w", key, metaErr)
		}
		base = meta.BaseBranch
	} else {
		if base, err = headBranch(repoRoot); err != nil {
			return fmt.Errorf("get base branch: %w", err)
		}
	}

	data := worktreeJSON{
		SchemaVersion:  1,
		Key:            key,
		Kind:           "worktree",
		Branch:         branch,
		WorktreePath:   path,
		RepositoryRoot: repoRoot,
		BaseBranch:     base,
		Exists:         exists,
		Dirty:          dirty,
	}

	switch opts.OutputMode {
	case outputJSON:
		out, _ := json.Marshal(data)
		inst, _ := jsonschema.UnmarshalJSON(bytes.NewReader(out))
		if err := outputSchema.Validate(inst); err != nil {
			return fmt.Errorf("internal: json output does not conform to schema: %w", err)
		}
		fmt.Println(string(out))
	case outputTOON:
		fmt.Printf(
			"schemaVersion = %d\nkey = %s\nkind = %s\nbranch = %s\npath = %s\nworktreePath = %s\nrepositoryRoot = %s\nbaseBranch = %s\nexists = %v\ndirty = %v\n",
			data.SchemaVersion, data.Key, data.Kind, data.Branch,
			data.WorktreePath, data.WorktreePath,
			data.RepositoryRoot, data.BaseBranch, data.Exists, data.Dirty,
		)
	}

	return nil
}

func cmdOpen(key string) error {
	repoRoot, err := gitRepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}

	path, err := worktreePath(repoRoot, key)
	if err != nil {
		return fmt.Errorf("resolve worktree path: %w", err)
	}
	branch := branchName(key)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create worktree parent: %w", err)
	}

	cmd := exec.Command("git", "worktree", "add", path, "-b", branch, "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %w\n%s", err, out)
	}

	base, err := headBranch(repoRoot)
	if err != nil {
		return fmt.Errorf("get base branch: %w", err)
	}

	meta := metadataFile{BaseBranch: base, WorktreePath: path}
	metaPath, err := metadataPath(repoRoot, key)
	if err != nil {
		return fmt.Errorf("resolve metadata path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
		return fmt.Errorf("create metadata dir: %w", err)
	}
	metaData, _ := json.Marshal(meta)
	if err := os.WriteFile(metaPath, metaData, 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}

	return nil
}

func cmdClose(key string) error {
	repoRoot, err := gitRepoRoot()
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}

	path, err := worktreePath(repoRoot, key)
	if err != nil {
		return fmt.Errorf("resolve worktree path: %w", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	cmd := exec.Command("git", "worktree", "remove", path)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove: %w\n%s", err, out)
	}

	meta, err := metadataPath(repoRoot, key)
	if err != nil {
		return fmt.Errorf("resolve metadata path: %w", err)
	}
	if err := os.Remove(meta); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove metadata: %w", err)
	}

	return nil
}

type metadataFile struct {
	BaseBranch   string `json:"baseBranch"`
	WorktreePath string `json:"worktreePath"`
}

func readMetadata(repoRoot, key string) (metadataFile, error) {
	path, err := metadataPath(repoRoot, key)
	if err != nil {
		return metadataFile{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return metadataFile{}, err
	}
	var meta metadataFile
	if err := json.Unmarshal(data, &meta); err != nil {
		return metadataFile{}, err
	}
	return meta, nil
}

type worktreeJSON struct {
	SchemaVersion  int    `json:"schemaVersion"`
	Key            string `json:"key"`
	Kind           string `json:"kind"`
	Branch         string `json:"branch"`
	WorktreePath   string `json:"worktreePath"`
	RepositoryRoot string `json:"repositoryRoot"`
	BaseBranch     string `json:"baseBranch"`
	Exists         bool   `json:"exists"`
	Dirty          bool   `json:"dirty"`
}

// Git and workspace resolution

func gitRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func headBranch(repoRoot string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitCommonDir(repoRoot string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	dir := strings.TrimSpace(string(out))
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir), nil
	}
	return filepath.Clean(filepath.Join(repoRoot, dir)), nil
}

func worktreeDirty(path string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

func stateDir(repoRoot string) (string, error) {
	commonDir, err := gitCommonDir(repoRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(commonDir, "kura"), nil
}

func worktreePath(repoRoot, key string) (string, error) {
	dir, err := stateDir(repoRoot)
	if err != nil {
		return "", err
	}
	return worktreePathInStateDir(dir, key), nil
}

func metadataPath(repoRoot, key string) (string, error) {
	dir, err := stateDir(repoRoot)
	if err != nil {
		return "", err
	}
	return metadataPathInStateDir(dir, key), nil
}

func worktreePathInStateDir(stateDir, key string) string {
	return filepath.Join(stateDir, "worktrees", key)
}

func metadataPathInStateDir(stateDir, key string) string {
	return filepath.Join(stateDir, "meta", "worktrees", key+".json")
}

func branchName(key string) string {
	return "kura-" + key
}

// Validation

var validKeyRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func validateKey(key string) error {
	if !validKeyRe.MatchString(key) {
		return fmt.Errorf("invalid key %q: key must match [A-Za-z0-9][A-Za-z0-9._-]{0,127}", key)
	}
	if strings.Contains(key, "..") {
		return fmt.Errorf("invalid key %q: key must not contain \"..\"", key)
	}
	if strings.HasSuffix(key, ".") {
		return fmt.Errorf("invalid key %q: key must not end with \".\"", key)
	}
	if strings.HasSuffix(key, ".lock") {
		return fmt.Errorf("invalid key %q: key must not end with \".lock\"", key)
	}
	return nil
}
