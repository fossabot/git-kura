# Use a centralized path seal store

- Status: Accepted
- Created: 2026-06-11T11:46:23Z

## Context

`git kura seal add/remove` must prevent two concurrent agent sessions from
claiming the same file path under different seal keys. An early design stored
one JSON file per key under `<git-common-dir>/kura/seals/<key>.json`. With
that layout, detecting cross-key conflicts required scanning all per-key files
on every operation, and a TOCTOU race between the scan and the write could
allow two writers to each claim the same path.

## Decision

v0 uses a single, centralized store:

```txt
<git-common-dir>/kura/seals/paths.json   — path → key map
<git-common-dir>/kura/seals/paths.lock   — writer lock (atomic create)
```

`paths.json` holds a `map[string]string` from repository-relative path
(forward-slash separator) to seal key.

`add/remove` acquire `paths.lock` via `O_CREATE|O_EXCL` before reading the
store, validate all requested paths against the in-memory map, then write the
updated store with a temp-file atomic rename, and release the lock.

If the lock file already exists the operation retries for up to 5 seconds
(fixed in v0; future: configurable via `GIT_KURA_SEAL_LOCK_TIMEOUT` or a
config file). Timeout exits with code 5 and a `seal-lock-timeout:` stderr
prefix. Cross-key conflict exits with code 6 and a `seal-conflict:` prefix.

Multiple paths are fully validated before any store mutation; partial success
is not possible.

## Consequences

- Cross-key conflict detection is a single map lookup — no directory scan.
- The lock eliminates TOCTOU races between concurrent `seal add/remove` calls.
- A stale lock file (process killed without releasing) blocks writers until
  manually removed. Automatic stale-lock cleanup is deferred to a future issue.
- v0 prioritises atomicity and explainability over high-concurrency throughput,
  which is appropriate: seal operations are infrequent compared to reads.

## Rejected alternatives

- **1 path = 1 file**: requires scanning all files for conflict detection and
  does not eliminate TOCTOU races without per-file locking.
- **OS-level advisory locks (flock/fcntl)**: platform-inconsistent behaviour
  across Linux, macOS, and Windows; lock files are more portable and visible.
