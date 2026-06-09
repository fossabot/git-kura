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
git kura open 51 --dry-run
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

`git kura open <key> --dry-run` uses the same JSON schema. In dry-run output,
`baseBranch` is the current branch, `exists` is `false`, and `dirty` is `false`.

### TOON (`--toon` / `--format toon`)

[TOON](https://github.com/toon-format/toon) is a prompt-friendly, AI-oriented format generated from the same metadata model as JSON.

```sh
git kura get 51 --toon
git kura get 51 --format toon
```

Example output:

```toon
schemaVersion: 1
key: fizz
kind: worktree
branch: fizz
worktreePath: /workspaces/git-kura/.git/kura/worktrees/fizz
repositoryRoot: /workspaces/git-kura
baseBranch: main
exists: true
dirty: false
```

Use TOON when passing workspace context to an LLM prompt or coding agent. JSON remains the compatibility contract for external tools; TOON is not a replacement.

## Metadata schema

The JSON Schema is defined in [`cmd/kura/schema/output.schema.json`](../cmd/kura/schema/output.schema.json)
and is embedded in the binary for runtime output validation.
