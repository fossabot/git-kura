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
[`docs/adr/20260613T064651Z_seal-worktree-context-and-worktree-guards.md`](../adr/20260613T064651Z_seal-worktree-context-and-worktree-guards.md).

Seal commands are classified by their effect and by whether their meaning
depends on the current seal key. This asymmetry is an intentional design
decision: read-only is not the deciding factor — semantic dependence on the
working context is. See
[`docs/adr/20260612T170922Z_seal-command-current-context-and-scope.md`](../adr/20260612T170922Z_seal-command-current-context-and-scope.md)
for the full rationale.

> Of these, `seal claim`, `seal unclaim`, `seal test`, and `seal ls` are
> implemented in the current release. `seal doctor` is specified in the ADR but
> not yet implemented. `seal session ls` / `seal session clean` belonged to the
> session-local model that has since been withdrawn; the worktree-guard commands
> take over that role. See
> [`docs/adr/20260614T002323Z_supersede-legacy-seal-command-model.md`](../adr/20260614T002323Z_supersede-legacy-seal-command-model.md).

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
| `git kura seal claim <file...>` | claim paths for the current key (mutation) |
| `git kura seal unclaim <file...>` | release the current key's claim (mutation) |
| `git kura seal test <file...>` | context-validation (read-only) |

`seal test` is read-only, but it answers whether the given files may be handled
in the *current* working context, so it is grouped with the mutation commands
and requires a current key in v0. With a valid current key, unsealed files and
files claimed by the current key are allowed, while files claimed by another key
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
| `git kura seal doctor` | project-wide integrity check (specified, not yet implemented) |

`seal doctor` is project-wide and read-only in v0: it must not modify seal
state and does not provide `seal doctor --fix`. It is specified in the ADR but
not yet implemented.

> `seal session ls` and `seal session clean` previously belonged to this
> classification, operating on repository-level session records. The
> session-local model they served has been withdrawn (see the
> [supersession ADR](../adr/20260614T002323Z_supersede-legacy-seal-command-model.md)),
> so there is currently no maintenance command; same-worktree coordination is
> handled by the (deferred) worktree-guard commands instead.
