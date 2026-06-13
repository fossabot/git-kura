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
```

## `git kura open <key>`

Create the branch and worktree for the given key.

```sh
git kura open 51
git kura open 51 --dry-run
```

If the corresponding branch or worktree already exists, Kura should not create a
conflicting workspace.

`--dry-run` does not create the branch, worktree, or metadata. It prints the
planned worktree as JSON using the same schema as `git kura get <N> --json`.

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
Use `git kura open <key> --dry-run` when you want to inspect the path and branch
that would be created.

This command is designed for both humans and scripts.

For example:

```sh
codex review "$(git kura get 51)"
```

`--root` prints the repository root path. This is useful for scripts that need to
locate files relative to the repository while operating inside a worktree:

```sh
root="$(git kura get 51 --root)"
```

See [output-format.md](output-format.md) for the full metadata schema and output
format reference.

## `git kura close <key>`

Remove the worktree and Kura-managed branch associated with the given key.

```sh
git kura close 51
```

Kura should refuse to remove a worktree when doing so would discard uncommitted
changes unless explicitly instructed.

## `git kura ls`

List all currently open worktrees managed by Kura.

```sh
git kura ls
```

Prints one key per line to standard output, sorted alphabetically. If no
worktrees are currently open, the output is empty and the exit code is 0.

This command is designed for scripts and enumeration:

```sh
for key in $(git kura ls); do
  git kura get "$key" --toon
done
```

## Keys

A key is an opaque, case-sensitive string identifier.

Kura does not parse keys as numbers. Kura does not resolve keys through GitHub,
GitLab, or any issue tracker.

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

## `git kura seal add <path> [path...]`

Add one or more repository-relative file paths to the seal store under the
current key (`GIT_KURA_SEAL_KEY`).

```sh
git kura seal add src/foo.go
git kura seal add src/foo.go tests/foo_test.go
```

Paths are interpreted relative to the repository root regardless of the
current working directory; absolute paths are rejected. All paths are
validated before any change is written — if one path fails, the store is not
modified.

Exits with `seal-conflict` (code 6) if any path is already sealed by a
different key. Exits with `seal-lock-timeout` (code 5) if the store lock
cannot be acquired within the retry timeout.

## `git kura seal remove <path> [path...]`

Remove one or more file paths from the seal store.

```sh
git kura seal remove src/foo.go
git kura seal remove src/foo.go tests/foo_test.go
```

Only the key that originally sealed a path may remove it. Attempting to
remove a path owned by a different key exits with `seal-conflict` (code 6).
Paths not currently in the store are silently skipped (idempotent).

## `git kura seal ls [key]`

List sealed paths recorded in the seal store, one per line:

```txt
<key>	<path>
```

```sh
git kura seal ls          # every sealed path, across all keys
git kura seal ls issue-18 # only paths sealed by issue-18
```

`ls` is a repository-wide inspection command. Unlike `seal add` and
`seal remove`, it does **not** read `GIT_KURA_SEAL_KEY`: running it inside a
`seal enter` session shows the same repository-wide result as running it
outside. To inspect a single key, pass the key as an explicit argument
(validated with the same rules as `seal enter`). See
[`docs/adr/20260612T170922Z_seal-command-current-context-and-scope.md`](adr/20260612T170922Z_seal-command-current-context-and-scope.md)
for the rationale.

The listed scope is the seal store in the Git common dir, shared by all
worktrees of the repository. Paths are repository-root relative with `/`
separators. Output is sorted by key, then by path within a key.

An absent store, an empty store, or a key with no sealed paths all produce
empty output and exit 0. A store that cannot be parsed, has an unsupported
`schemaVersion`, or does not match the store schema is an error.

`ls` is read-only and does not take the store lock, so it is never blocked
by a held `paths.lock`.

## `git kura seal session ls [key]`

List the seal session records for this repository.

```sh
git kura seal session ls           # every recorded session
git kura seal session ls issue-14  # only sessions recorded under issue-14
```

A seal session is created by `git kura seal enter <key>` and normally removed
when the child shell exits. A record can survive an abnormal exit (terminal
crash, killed process, OS shutdown), and `session ls` is how you inspect what
remains.

Output is a table with one row per session:

```txt
key  worktree  parent  child  age  status
```

`status` is one of:

| Status | Meaning |
|--------|---------|
| `active` | parent and child PIDs are alive and within the TTL |
| `stale-candidate (ttl exceeded)` | PIDs are alive but the session is older than the TTL — review it, but it is not removed automatically |
| `stale` | parent and/or child PID is dead — safe to clean |
| `unknown` | PIDs are alive but the child PID was never recorded |

The TTL is a warning signal only; it never causes a session to be removed.
The default is 5 minutes and it is configured with the `GIT_KURA_SESSION_TTL`
environment variable, which accepts a Go duration string (for example `10m`).

Like `seal ls`, `session ls` is a repository-wide inspection command. It does
**not** read `GIT_KURA_SEAL_KEY`, so its output is the same inside and outside
a `seal enter` session. Pass a key argument (validated with the same rules as
`seal enter`) to narrow the listing to one key. The inspected scope is the
session store in the Git common dir, shared by all worktrees of the
repository. Corrupt or unreadable session files are always listed (with the
problem in the `status` column) because they block `seal enter` for their
worktree. See
[`docs/adr/20260612T170922Z_seal-command-current-context-and-scope.md`](adr/20260612T170922Z_seal-command-current-context-and-scope.md)
for the rationale.

## `git kura seal session clean [--apply]`

Remove stale seal sessions from this repository.

```sh
git kura seal session clean          # dry-run: show what would be removed
git kura seal session clean --apply  # actually remove the stale sessions
```

A session is treated as stale **only** when its parent and child PIDs are
confirmed dead (best-effort PID liveness check). TTL expiry alone is never a
reason to delete: a TTL-exceeded session whose PIDs are still alive is kept.
Sessions whose liveness cannot be determined are left in place — when in
doubt, `clean` does nothing.

`session clean` is **dry-run by default**: it prints the sessions it would
remove and exits without changing anything. Pass `--apply` to delete them.
This is a maintenance command and does not read `GIT_KURA_SEAL_KEY`; it
operates on the repository-wide session store resolved from the Git common
dir. Before deleting, it re-reads each record and re-checks liveness, so a
session that was restarted between listing and deletion is not removed.

Corrupt or unreadable session files are reported as a warning and never
deleted automatically — inspect them manually.

## Exit codes

Kura uses stable exit codes so scripts and AI-agent workflows can react
correctly.

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Usage error |
| 3 | Unsafe operation refused |
| 4 | Not found |
| 5 | Seal lock timeout |
| 6 | Seal conflict |

Exit codes 5 and 6 are signalled by `seal add` and `seal remove`. The stderr
message always starts with a stable reason token (`seal-lock-timeout:` or
`seal-conflict:`) that scripts can match without parsing arbitrary text.
