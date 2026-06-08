# Output formats

`git kura get` supports scalar and structured output.

## Scalar output

Returns a single value, suitable for shell substitution.

```sh
git kura get 51 --path    # absolute path to the worktree
git kura get 51 --branch  # branch name
```

## Structured output

### JSON (`--json` / `--format json`)

JSON is the canonical machine-readable format. Use it for scripts, tools, and integrations.

```sh
git kura get 51 --json
git kura get 51 --format json
```

Example output:

```json
{
  "schemaVersion": 1,
  "key": "51",
  "kind": "worktree",
  "branch": "issue/51",
  "worktreePath": "/home/user/projects/myrepo-issue-51",
  "repositoryRoot": "/home/user/projects/myrepo",
  "baseBranch": "main",
  "exists": true,
  "dirty": false
}
```

### TOON (`--toon` / `--format toon`)

[TOON](https://github.com/toon-format/toon) is a prompt-friendly, AI-oriented format generated from the same metadata model as JSON.

```sh
git kura get 51 --toon
git kura get 51 --format toon
```

Use TOON when passing workspace context to an LLM prompt or coding agent. JSON remains the compatibility contract for external tools; TOON is not a replacement.

## Metadata schema

Both JSON and TOON output are derived from the same internal model:

| Field | Type | Description |
|-------|------|-------------|
| `schemaVersion` | integer | Schema version of this output |
| `key` | string | The key passed to the command |
| `kind` | string | Workspace kind (e.g. `"worktree"`) |
| `branch` | string | Git branch name associated with the key |
| `worktreePath` | string | Absolute path to the worktree |
| `repositoryRoot` | string | Absolute path to the repository root |
| `baseBranch` | string | Branch the worktree was created from |
| `exists` | boolean | Whether the worktree currently exists on disk |
| `dirty` | boolean | Whether the worktree has uncommitted changes |
