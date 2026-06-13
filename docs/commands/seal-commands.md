# Seal commands

`git kura seal` uses a *seal key* to represent the working context of a process
or agent. `git kura seal enter <key>` establishes the current seal key by
setting `GIT_KURA_SEAL_KEY` for a child shell and its descendants, so the
current key is process-local and session-local rather than a repository-wide
persistent setting.

Seal commands are classified by their effect and by whether their meaning
depends on the current seal key. This asymmetry is an intentional design
decision: read-only is not the deciding factor — semantic dependence on the
working context is. See
[`docs/adr/20260612T170922Z_seal-command-current-context-and-scope.md`](../adr/20260612T170922Z_seal-command-current-context-and-scope.md)
for the full rationale.

> Of these, only `seal add`, `seal remove`, and `seal ls` are implemented in
> the current release. `seal test`, `seal doctor`, and `seal session
> ls`/`clean` describe the intended v0 design recorded in the ADR.

## Project scope

The *project scope* is the seal state associated with the Git common dir
resolved from the current working directory. It is the state shared by all
worktrees of that repository. Current-independent inspection commands operate
on this project scope by default.

## Current-dependent commands

These commands are semantically tied to the active work context and require a
valid current seal key. If `GIT_KURA_SEAL_KEY` is unset, empty, or invalid,
they fail.

| Command | Effect |
|---------|--------|
| `git kura seal add <file...>` | change path seal ownership (mutation) |
| `git kura seal remove <file...>` | change path seal ownership (mutation) |
| `git kura seal test <file...>` | context-validation (read-only) |

`seal test` is read-only, but it answers whether the given files may be handled
in the *current* working context, so it is grouped with the mutation commands
and requires a current key in v0. With a valid current key, unsealed files and
files sealed by the current key are allowed, while files sealed by another key
are rejected. v0 does not provide a project-wide validation mode (no
`--all` / `--unsealed`).

## Current-independent inspection commands

These commands inspect the project scope by default and must **not** consult
`GIT_KURA_SEAL_KEY`. Running them inside a `seal enter <key>` session produces
the same project-wide result as running them outside. A narrower key scope must
be requested explicitly (for example `git kura seal ls <key>`).

| Command | Notes |
|---------|-------|
| `git kura seal ls` | lists project-wide path seals; ignores the current key |
| `git kura seal session ls` | lists project-wide session records |
| `git kura seal doctor` | project-wide integrity check |

`seal doctor` is project-wide and read-only in v0: it must not modify seal
state and does not provide `seal doctor --fix`.

## Current-independent maintenance commands

`git kura seal session clean` operates on repository-level session records, not
on path seal ownership for the current working context, so it does not use the
current seal key as a mutation guard. Instead it is controlled by explicit
safety flags: it is dry-run by default and deletes records only with explicit
flags such as `--apply`, while wider modes such as `--force` stay explicit and
are never implied by the current seal key.

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
[`docs/adr/20260612T170922Z_seal-command-current-context-and-scope.md`](../adr/20260612T170922Z_seal-command-current-context-and-scope.md)
for the rationale.

The listed scope is the seal store in the Git common dir, shared by all
worktrees of the repository. Paths are repository-root relative with `/`
separators. Output is sorted by key, then by path within a key.

An absent store, an empty store, or a key with no sealed paths all produce
empty output and exit 0. A store that cannot be parsed, has an unsupported
`schemaVersion`, or does not match the store schema is an error.

`ls` is read-only and does not take the store lock, so it is never blocked
by a held `paths.lock`.
