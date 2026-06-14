# Commands

`git-kura` is invoked as a Git subcommand:

```sh
git kura <command> [arguments]
```

`git-kura` provides following commands.

```sh
git kura open fizz              # create a worktree and branch for key "fizz"
git kura open fizz --dry-run    # print the worktree that would be created
git kura get fizz               # print the open worktree path for "fizz"
git kura get fizz --path        # print the worktree path for "fizz"
git kura get fizz --branch      # print the branch name for "fizz"
git kura get fizz --root        # print the repository root path
git kura get fizz --format json # print workspace metadata as JSON
git kura get fizz --json        # alias of `--format json`
git kura get fizz --format toon # print workspace metadata as TOON for AI prompts
git kura get fizz --toon        # alias of `--format toon`
git kura close fizz             # remove the worktree for "fizz"
git kura ls                     # list all open worktrees
git kura seal claim <path...>   # claim paths for the current seal key
git kura seal unclaim <path...> # release the current seal key's claim on paths
git kura seal test <path...>    # check paths against the current seal context
git kura seal ls [key]          # list claimed paths (project-wide by default)
git kura seal doctor            # validate the project-wide seal store
```

## `git kura open <key>`

Create the branch and worktree for the given key.

```sh
git kura open 51
git kura open 51 --dry-run
```

If the corresponding branch or worktree already exists, Kura should not create a conflicting workspace.

`--dry-run` does not create the branch, worktree, or metadata. It prints the planned worktree as JSON using the same schema as `git kura get <N> --json`.

## `git kura get <key>`

Resolve the branch or worktree associated with the given key.

```sh
git kura get 51
git kura get 51 --path
git kura get 51 --branch
git kura get 51 --root
git kura get 51 --json
git kura get 51 --toon
```

`git kura get <key>` and all output flags require the key to be currently open.
Use `git kura open <key> --dry-run` when you want to inspect the path and branch that would be created.

This command is designed for both humans and scripts.

For example:

```sh
codex review "$(git kura get 51)"
```

`--root` prints the repository root path. This is useful for scripts that need to locate files relative to the repository while operating inside a worktree:

```sh
root="$(git kura get 51 --root)"
```

See [output-format.md](output-format.md) for the full metadata schema and output format reference.

## `git kura close <key>`

Remove the worktree and Kura-managed branch associated with the given key.

```sh
git kura close 51
```

Kura should refuse to remove a worktree when doing so would discard uncommitted changes unless explicitly instructed.

## `git kura ls`

List all currently open worktrees managed by Kura.

```sh
git kura ls
```

Prints one key per line to standard output, sorted alphabetically. If no worktrees are currently open, the output is empty and the exit code is 0.

This command is designed for scripts and enumeration:

```sh
for key in $(git kura ls); do
  git kura get "$key" --toon
done
```

## Keys

A key is an opaque, case-sensitive string identifier.

Kura does not parse keys as numbers. Kura does not resolve keys through GitHub, GitLab, or any issue tracker.

In v0, a key must match:

```txt
[A-Za-z0-9][A-Za-z0-9._-]{0,127}
```

Additionally, Kura rejects keys that:

* are `"."` or `".."`
* contain `".."`
* start with `"."`
* end with `"."`
* end with `".lock"`
* contain path separators `"/"` or `"\"`
* contain whitespace
* contain control characters
* contain shell metacharacters
* contain Git ref expression syntax such as `"@{"`

## Seal commands

`git kura seal` manages *path seals* scoped to a seal key.
The command reference is below. For the concepts behind these commands — how they are classified, the meaning of *project scope*, and which commands depend on the current seal key — see [Seal commands: context and scope](commands/seal-commands.md).

## `git kura seal claim <path> [path...]`

Claim one or more repository-relative file paths for the current key in the seal store. Claiming records that the current task intends to edit those paths so conflicting edits across tasks/worktrees are detected before merge.

The current key is derived from the git-kura managed worktree you run the command in. Move into the worktree created by `git kura open <key>` and that worktree's key becomes the current key:

```sh
cd "$(git kura get issue-18)"
git kura seal claim src/foo.go
git kura seal claim src/foo.go tests/foo_test.go
```

`seal claim` fails when it is not run inside a managed worktree, or when that worktree's metadata is missing or inconsistent.

Paths are interpreted relative to the repository root regardless of the current working directory; absolute paths are rejected. All paths are validated before any change is written — if one path fails, the store is not modified.

Exits with `seal-conflict` (code 6) if any path is already claimed by a different key. Exits with `seal-lock-timeout` (code 5) if the store lock cannot be acquired within the retry timeout.

## `git kura seal unclaim <path> [path...]`

Release the current key's claim on one or more file paths in the seal store.

```sh
git kura seal unclaim src/foo.go
git kura seal unclaim src/foo.go tests/foo_test.go
```

Only the key that claimed a path may release it. Attempting to unclaim a path claimed by a different key exits with `seal-conflict` (code 6). Paths not currently claimed are silently skipped (idempotent).

## `git kura seal test <path> [path...]`

Check whether one or more repository-relative paths may be handled in the current seal context, without modifying the store. `seal test` answers a single question: given the current key, is every listed path safe to edit?

The current key is derived from the git-kura managed worktree you run the command in, exactly like `seal claim` / `seal unclaim`:

```sh
cd "$(git kura get issue-18)"
git kura seal test src/foo.go
git kura seal test src/foo.go tests/foo_test.go
```

`seal test` fails when it is not run inside a managed worktree, or when that worktree's metadata is missing or inconsistent. This context error is distinct from a seal conflict. `GIT_KURA_SEAL_KEY` is **not** consulted for current-key resolution and does not affect the result.

A path is safe when it is unclaimed, or already claimed by the current key. A path claimed by a different key is a conflict. Paths are interpreted relative to the repository root regardless of the current working directory; absolute paths and paths outside the repository are rejected. A path inside the repository that does not exist yet is treated as unclaimed, so `seal test` can check a file before it is created.

`seal test` exits 0 only when every path is safe. If any path conflicts it exits with `seal-conflict` (code 6) and reports each conflicting path with the key that claims it. `seal test` is read-only: it does not modify the store and does not take the store lock, so it is never blocked by a held `paths.lock`.

In v0 `seal test` takes no options. `--all`, `--unsealed`, and `--staged` are not defined and are rejected; project-wide validation (checking paths without a current key) is intentionally out of scope.

## `git kura seal ls [key]`

List claimed paths recorded in the seal store, one per line:

```txt
<key>	<path>
```

```sh
git kura seal ls          # every claimed path, across all keys
git kura seal ls issue-18 # only paths claimed by issue-18
```

`ls` is a repository-wide inspection command. Unlike `seal claim` and `seal unclaim`, it does **not** derive a current key from the worktree: its output is the same whether it runs from the main checkout or from inside a managed worktree. To inspect a single key, pass the key as an explicit argument (validated with the same key rules). See [`docs/adr/20260612T170922Z_seal-command-current-context-and-scope.md`](adr/20260612T170922Z_seal-command-current-context-and-scope.md) for the rationale.

The listed scope is the seal store in the Git common dir, shared by all worktrees of the repository. Paths are repository-root relative with `/` separators. Output is sorted by key, then by path within a key.

An absent store, an empty store, or a key with no claimed paths all produce empty output and exit 0. A store that cannot be parsed, has an unsupported `schemaVersion`, or does not match the store schema is an error.

`ls` is read-only and does not take the store lock, so it is never blocked by a held `paths.lock`.

## `git kura seal doctor`

Validate the project-wide path seal store for the Git repository resolved from the current working directory.

```sh
git kura seal doctor
```

`doctor` is a repository-wide inspection command. It does **not** derive a current key from the worktree, does **not** read git-kura worktree metadata, and does **not** consult `GIT_KURA_SEAL_KEY`. It can run from the main checkout, from a git-kura managed worktree, or from any other directory inside the Git repository.

An absent `paths.json` is treated as an empty store and succeeds. A healthy store exits 0 and prints nothing to stdout.

`doctor` validates the store file structure, `schemaVersion`, entry keys, repository-relative path syntax, `/` path separators, paths that escape the repository root, and paths that would duplicate another entry after normalization. It does not check whether stored paths currently exist in the working tree, whether they are files or directories, or where symlinks point.

If the store is malformed or inconsistent, `doctor` exits with `seal-doctor-error` (code 7) and reports every problematic store entry it finds on stderr, so all issues can be fixed in a single pass. `doctor` is read-only: it does not modify `paths.json`, does not take `paths.lock`, and does not create, remove, or rewrite a lock file.

In v0 `seal doctor` takes no arguments and no options. `git kura seal doctor <key>`, `git kura seal doctor --fix`, and other options are usage errors.

## Exit codes

Kura uses stable exit codes so scripts and AI-agent workflows can react correctly.

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Usage error |
| 3 | Unsafe operation refused |
| 4 | Not found |
| 5 | Seal lock timeout |
| 6 | Seal conflict |
| 7 | Seal doctor error |

Exit code 5 is signalled by `seal claim` and `seal unclaim`. Exit code 6 is signalled by `seal claim`, `seal unclaim`, and `seal test`. Exit code 7 is signalled by `seal doctor` when the seal store fails integrity validation. The stderr message always starts with a stable reason token (`seal-lock-timeout:`, `seal-conflict:`, or `seal-doctor-error:`) that scripts can match without parsing arbitrary text.
