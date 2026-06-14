# Limit seal targets to repository-relative files

- Status: Partially superseded by [20260614T002323Z_supersede-legacy-seal-command-model.md](20260614T002323Z_supersede-legacy-seal-command-model.md)
- Created: 2026-06-11T11:46:24Z

> **Partially superseded.** The repository-relative path constraints and normalization rules below are still current. The `git kura seal add` / `seal remove` command names are superseded by `seal claim` / `seal unclaim` (the existence-check wording applies to `claim` / `unclaim` respectively). See [20260614T002323Z_supersede-legacy-seal-command-model.md](20260614T002323Z_supersede-legacy-seal-command-model.md).

## Context

`git kura seal add/remove` must store file paths in the seal store in a form that is stable across worktrees and host machines. Absolute paths contain machine-specific prefixes and differ between worktrees; relative paths without a well-defined base are ambiguous.

## Decision

`git kura seal add/remove` accepts only relative paths as arguments. The rules are:

- **Absolute paths are rejected** with a usage error.
- **Paths outside the repository root are rejected** with a usage error.
- **Arguments are interpreted relative to the repository root**, never the caller's working directory, so the same argument always resolves to the same file regardless of where the command is invoked.
- `.`, `..`, and redundant separators are cleaned with `filepath.Clean`.
- **Stored paths use forward-slash separators** (`filepath.ToSlash`) for cross-platform consistency.
- **Symlinks are not resolved** — the path is stored as given (after cleaning).
- **Non-existent paths are rejected by `seal add`** (the file must exist at add time). `seal remove` works even when the file no longer exists, so a seal can be released after the file is deleted or renamed.
- **Directories are not seal targets.** `seal add` rejects directories, and recursive directory sealing is a non-goal.
- Path normalisation is isolated in `normalizeSealPath`.

## Consequences

- The store is portable across worktrees sharing the same git common directory: a path sealed from one worktree is visible from another via the shared `seals/paths.json`.
- Users must be inside the repository to run `seal add/remove`, which is consistent with the rest of the `git kura` CLI.
- Absolute paths passed by scripts will be rejected; scripts must use relative paths or derive them with `git rev-parse --show-prefix`.

## Rejected alternatives

- **Accept absolute paths**: paths differ between machines and worktrees, making the store non-portable.
- **Silently convert absolute paths to relative**: the conversion is implicit and surprising; explicit rejection is safer.
- **Directory (recursive) seals**: seals exist to reserve specific files pin-point; directory seals raise hard questions about recursion scope, files created later, overlap with file seals, and hook-side evaluation, none of which v0 needs to answer.
