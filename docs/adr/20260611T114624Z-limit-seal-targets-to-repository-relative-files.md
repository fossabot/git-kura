# Limit seal targets to repository-relative files

- Status: Accepted
- Created: 2026-06-11T11:46:24Z

## Context

`git kura seal add/remove` must store file paths in the seal store in a form
that is stable across worktrees and host machines. Absolute paths contain
machine-specific prefixes and differ between worktrees; relative paths without
a well-defined base are ambiguous.

## Decision

`git kura seal add/remove` accepts only relative paths as arguments. The rules
are:

- **Absolute paths are rejected** with a usage error.
- **Paths outside the repository root are rejected** with a usage error.
- Relative paths are resolved against the current working directory, then made
  relative to the repository root via `filepath.Rel`.
- `.`, `..`, and redundant separators are cleaned with `filepath.Clean`.
- **Stored paths use forward-slash separators** (`filepath.ToSlash`) for
  cross-platform consistency.
- **Symlinks are not resolved** — the path is stored as given (after cleaning).
- **Non-existent paths are rejected by `seal add`** (the file must exist at
  add time). `seal remove` does not require the file to exist.
- Path normalisation is isolated in `normalizeSealPath`.

## Consequences

- The store is portable across worktrees sharing the same git common directory:
  a path sealed from one worktree is visible from another via the shared
  `seals/paths.json`.
- Users must be inside the repository to run `seal add/remove`, which is
  consistent with the rest of the `git kura` CLI.
- Absolute paths passed by scripts will be rejected; scripts must use relative
  paths or derive them with `git rev-parse --show-prefix`.

## Rejected alternatives

- **Accept absolute paths**: paths differ between machines and worktrees, making
  the store non-portable.
- **Silently convert absolute paths to relative**: the conversion is implicit
  and surprising; explicit rejection is safer.
