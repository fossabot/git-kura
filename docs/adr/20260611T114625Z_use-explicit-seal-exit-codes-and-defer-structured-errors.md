# Use explicit seal exit codes and defer structured error output

- Status: Accepted
- Created: 2026-06-11T11:46:25Z

## Context

Agent Skills and shell scripts driving `git kura seal add/remove` need to distinguish "the store lock could not be acquired" from "the path is sealed by another key" — the first is retryable, the second requires coordination.
The top-level command runner previously exited `1` for every error, making the two cases indistinguishable without parsing stderr text.

## Decision

- Introduce dedicated exit codes, recorded in the docs/commands.md table:
  - `5` — seal lock timeout
  - `6` — seal conflict
- Introduce an error type (`exitError`, built via the `exitCodeError` constructor) that carries an exit code through the command runner; `main` unwraps it with `errors.As` and exits with that code. Errors without a code keep exiting `1`.
- stderr messages for these cases start with a stable reason token (`seal-lock-timeout:` / `seal-conflict:`) as a secondary, human-readable discriminator.
- Tests assert that the exit code alone is sufficient to distinguish the cases.
- Structured error output (JSON / TOON) is **not** implemented in this issue.
  When it is introduced later, single and multiple conflicts must share the same shape: always a `details` array, e.g.

  ```json
  {
    "schemaVersion": 1,
    "kind": "error",
    "code": "seal-conflict",
    "message": "one or more paths are already sealed by another key",
    "currentKey": "issue-18",
    "details": [
      { "path": "src/foo.go", "sealedBy": "issue-19" }
    ]
  }
  ```

## Consequences

- Scripts branch on `$?` (`5` → retry/wait, `6` → pick another file or key) without depending on stderr string parsing.
- The plain-text conflict error already lists every conflicting path with its sealing key, so the future `details` array is a formatting change, not an information change.
- The exit code table becomes part of the CLI's compatibility surface; codes must not be renumbered.

## Rejected alternatives

- **Structured (JSON/TOON) error output now**: useful, but the priority for this issue is a minimal `add/remove` implementation with fixed exit codes.
- **stderr-token-only discrimination**: forces every caller to parse text; exit codes are simpler and locale/format-stable.
