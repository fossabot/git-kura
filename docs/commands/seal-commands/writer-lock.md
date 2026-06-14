# Seal store writer lock

This document gives a high-level overview of the *writer lock* that guards the seal store. It covers what the lock protects and where the lock file lives.

## What the lock protects

The path seals recorded by `git kura seal` are kept in a single store that is shared by every worktree of a repository. The mutating commands — `git kura seal claim` and `git kura seal unclaim` — change that shared store.

To keep those changes safe when several processes or agents run at the same time, each mutation takes a **writer lock** before touching the store. The lock is exclusive: only one `seal claim` / `seal unclaim` can hold it at a time, so concurrent mutations are serialized rather than racing against each other.

If the lock is already held, the command waits and retries for a short while. If it still cannot acquire the lock within the timeout, the command fails with the `seal-lock-timeout` reason (exit code 5) instead of proceeding.

Read-only commands (`git kura seal ls`, `git kura seal test`) do **not** take the writer lock, so they are never blocked by a held lock and never block a writer.

## Where the lock file lives

The lock is a file named `paths.lock`, placed next to the seal store under the repository's Git common directory:

```
<git-common-dir>/kura/seals/paths.json   # the seal store
<git-common-dir>/kura/seals/paths.lock   # the writer lock
```

Because the lock lives under the Git common directory, it is shared by all worktrees of the repository. A `seal claim` / `seal unclaim` run from one worktree therefore excludes mutations run from any other worktree of the same repository.

## Related documents

- [seal-commands.md](../seal-commands.md) — classification and scope of the seal commands.
- [commands.md](../../commands.md) — full command reference, including exit codes.
