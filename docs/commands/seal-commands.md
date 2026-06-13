# Seal commands: context and scope

This document explains the *concepts* behind `git kura seal` — how the commands
are classified and why. For the command reference (usage, arguments, examples),
see the seal sections in [commands.md](../commands.md).

`git kura seal` uses a *seal key* to represent the working context of a process
or agent. The current seal key is derived from the git-kura managed worktree the
command runs in: each worktree is created by `git kura open <key>`, so the key
of the worktree you are in is the current key. This makes the context survive
the fresh shell invocations that agent workflows make, without relying on
process-local state. See
[`docs/adr/2026-06-13T06:46:51Z_seal-worktree-context-and-worktree-guards.md`](../adr/2026-06-13T06:46:51Z_seal-worktree-context-and-worktree-guards.md).

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
valid current seal key. If the command is not run inside a git-kura managed
worktree, or that worktree's metadata is missing or inconsistent, they fail.

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

These commands inspect the project scope by default and must **not** derive a
current key from the worktree. Running them from inside a managed worktree
produces the same project-wide result as running them from the main checkout. A
narrower key scope must be requested explicitly (for example
`git kura seal ls <key>`).

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
