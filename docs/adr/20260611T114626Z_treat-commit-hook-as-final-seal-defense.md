# Treat commit hook as final seal defense

- Status: Accepted
- Created: 2026-06-11T11:46:26Z

## Context

The seal store prevents concurrent agent sessions from editing the same files, but `git kura seal add/remove` is only effective when callers use it. A defense is needed for the case where a session commits changes to a path that another key has sealed, without ever consulting the seal store.

## Decision

- The commit hook itself is **not** implemented in this issue.
- The overall seal design keeps the commit hook as the final line of defense:
  - The hook will reject a commit when a staged path is sealed by a key other than the current one (`GIT_KURA_SEAL_KEY`).
  - Unsealed staged paths are allowed at this stage.
- Defense in depth: Agent Skills and `seal add/remove` provide early detection; the commit hook provides final enforcement.
- If real `git-kura` + multi-agent operation shows that the cooperative seal mechanism alone leaves too many collisions, a future **strict mode** may be considered:

  ```txt
  staged paths must be sealed by the current key, or the commit is rejected
  ```

## Consequences

- v0 seals are a cooperative reservation system, not mandatory access control; sessions that skip `seal add` are caught only at commit time once the hook exists.
- The centralized store layout (`seals/paths.json`) was chosen partly so the future hook can check all staged paths with a single file read.

## Rejected alternatives

- **Strict mode from the start**: too disruptive for normal human workflows; every edit would require sealing first.
- **Hook-only enforcement (no `seal add/remove`)**: detection at commit time is too late; agents would waste work on files they cannot commit.
