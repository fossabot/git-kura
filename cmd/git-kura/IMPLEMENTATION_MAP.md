# IMPLEMENTATION_MAP (`cmd/git-kura`)

> **This is an implementation cross-reference for maintainers. It is not a user-facing command reference.**
>
> For how to use each command, see [docs/commands.md](../../docs/commands.md) and
> [docs/commands/seal-commands.md](../../docs/commands/seal-commands.md).
> The sole purpose of this document is to connect each "spec overview", "background ADR", and
> "corresponding implementation / schema / test", so that related design decisions are less
> likely to be overlooked when changing the implementation.
>
> Detailed specs, decision rationale, schema field listings, and command references are not
> repeated here. All of them are deferred to references to the implementation files / ADRs /
> schema files / `docs/`.

## Meaning of status

| status | meaning |
|--------|---------|
| `implemented` | The current implementation matches the ADR's decision |
| `planned` | Described in an ADR but not yet implemented |
| `superseded` | Completely replaced by a later ADR |
| `partially superseded` | Only part of the decision is current. The range that may be referenced as current is stated explicitly in each item |

For each `partially superseded` item, the authoritative source for which clauses are current and
which clauses were replaced is
[docs/adr/20260614T002323Z_supersede-legacy-seal-command-model.md](../../docs/adr/20260614T002323Z_supersede-legacy-seal-command-model.md).
Each item below summarizes that range by mapping it onto the implementation.

---

## Management of the seal store and the writer lock

- **Overview**: Hold the path → key mapping in a single centralized store, and use a writer lock
  to prevent the TOCTOU of concurrent seal writes.
- **status**: `partially superseded`
  - Range that may be referenced as current: the centralized store layout
    (`<git-common-dir>/kura/seals/paths.json` + `paths.lock`), and the mechanisms of
    locking via `O_CREATE|O_EXCL`, writing via atomic rename, and the lock timeout.
  - Range that must not be referenced: the command names `git kura seal add/remove` in the ADR
    body. The current command names are `seal claim/unclaim` (see "Semantics of claim / unclaim"
    below).
- **ADR**: [docs/adr/20260611T114623Z-use-centralized-seal-store.md](../../docs/adr/20260611T114623Z-use-centralized-seal-store.md)
- **Implementation**: [seal_path.go](seal_path.go)
  - `pathsSealStore`
  - `readSealStore`
  - `writeSealStore`
  - `acquireSealLock`
- **schema**: [schema/seal_store.schema.json](schema/seal_store.schema.json)
- **test**: [unit_test.go](unit_test.go)
  - `TestReadSealStore*` / `TestWriteSealStore*` / `TestWriteReadSealStoreRoundtrip`
  - `TestWrittenSealStoreConformsToSchema`
  - `TestPathsSealStoreOutsideRepo`
  - `TestAcquireSealLock*` / `TestSealLockReleaseReportsRemoveFailure`

---

## Normalization and constraints of the seal target path

- **Overview**: Restrict seal targets to repository-relative files, normalize them relative to
  the repository root, and store them in the store.
- **status**: `partially superseded`
  - Range that may be referenced as current: the path constraints and normalization rules
    (rejecting absolute paths / paths outside the repository, resolving relative to the
    repository root, storing with forward slashes, etc.).
  - Range that must not be referenced: the command names `git kura seal add/remove` in the ADR
    body and the description of existence checks in `add`/`remove`. The current command names are
    `seal claim/unclaim`.
- **ADR**: [docs/adr/20260611T114624Z-limit-seal-targets-to-repository-relative-files.md](../../docs/adr/20260611T114624Z-limit-seal-targets-to-repository-relative-files.md)
- **Implementation**: [seal_path.go](seal_path.go)
  - `normalizeSealPath`
  - `cmdSealClaim`
  - `cmdSealUnclaim`
- **test**: [unit_test.go](unit_test.go)
  - `TestNormalizeSealPath*`
  - `TestSealClaimRejectsAbsolutePath` / `TestSealClaimRejectsPathOutsideRepo`
    / `TestSealClaimResolvesPathsFromRepoRootNotCwd` ([integration_test.go](integration_test.go))

---

## Resolving the current seal key from the managed worktree

- **Overview**: Resolve the current seal key not from a process-local environment variable, but
  from the identity and metadata of the git-kura managed worktree you are currently in.
- **status**: `implemented`
- **ADR**: [docs/adr/2026-06-13T06:46:51Z_seal-worktree-context-and-worktree-guards.md](../../docs/adr/2026-06-13T06:46:51Z_seal-worktree-context-and-worktree-guards.md)
  - The old session-local model (`seal enter` / `GIT_KURA_SEAL_KEY`) is in
    [docs/adr/2026-06-11T11:46:22Z_use-session-local-seal-context.md](../../docs/adr/2026-06-11T11:46:22Z_use-session-local-seal-context.md),
    but that ADR is `superseded`.
- **Implementation**:
  - [seal_path.go](seal_path.go) — `readSealContext`
  - [../../internal/worktree/worktree.go](../../internal/worktree/worktree.go) — `CurrentKey`
- **schema**: [../../internal/worktree/schema/metadata.schema.json](../../internal/worktree/schema/metadata.schema.json)
- **test**:
  - [unit_test.go](unit_test.go) — `TestReadSealContextInsideWorktree` / `TestReadSealContextOutsideWorktree`
  - [../../internal/worktree/worktree_test.go](../../internal/worktree/worktree_test.go) — `TestCurrentKey*`

---

## Semantics of claim / unclaim

- **Overview**: The current task key asserts (claim) or releases (unclaim) ownership of a
  repository-relative path before editing. A path already claimed by a different key is rejected
  as a cross-worktree conflict.
- **status**: `partially superseded`
  - Range that may be referenced as current: the semantics of `seal claim` / `seal unclaim`, and
    the policy of resolving the current key from the worktree.
  - Range that must not be referenced: the following in the same ADR are not the current
    implementation.
    - Keeping deprecated aliases `seal add` / `seal remove`
      — the aliases were not kept and have been removed (the current commands are only
      `claim` / `unclaim`).
    - `git kura guard acquire/release/status` (worktree guard) — not implemented (`planned`).
    - `seal check --staged` (staged check at commit time) — not implemented (`planned`).
      The closest existing command is `seal test <path...>`.
- **ADR**: [docs/adr/2026-06-13T06:46:51Z_seal-worktree-context-and-worktree-guards.md](../../docs/adr/2026-06-13T06:46:51Z_seal-worktree-context-and-worktree-guards.md)
- **Related Issue**: [#30](https://github.com/tooppoo/git-kura/issues/30)
- **Implementation**:
  - [seal.go](seal.go) — `runSeal` / `runSealClaim` / `runSealUnclaim`
  - [seal_path.go](seal_path.go) — `cmdSealClaim` / `cmdSealUnclaim` / `sealConflictError`
- **test**:
  - [integration_test.go](integration_test.go) — `TestSealClaim*` / `TestSealUnclaim*`
  - [unit_test.go](unit_test.go) — `TestCmdSealClaim*` / `TestCmdSealUnclaim*` / `TestRunSealClaim*`

---

## Context scope of seal test / seal ls

- **Overview**: Even among read-only commands, the degree of dependence on context is treated
  differently. `seal test` is a validation that depends on the current key (current-dependent)
  and fails if there is no current key. `seal ls` is a repository-wide inspection
  (current-independent): it does not use the current key as the default display scope, and any
  filtering is specified explicitly via a key argument.
- **status**: `partially superseded`
  - Range that may be referenced as current: the read-only-vs-mutation /
    current-dependent-vs-current-independent classification, and the fact that it applies to
    `seal test` (current-dependent) and `seal ls` (repository-wide).
  - Range that must not be referenced: the mechanism for establishing the current key assumed by
    the same ADR. Establishment via `seal enter` / `GIT_KURA_SEAL_KEY`, as well as
    `seal session ls/clean` and `seal doctor`, are not the current implementation (withdrawn /
    not implemented). The current key derives from the worktree (see "Resolving the current seal
    key from the managed worktree"). The names `seal add/remove` in the body are also
    `claim/unclaim` in the current implementation.
- **ADR**: [docs/adr/20260612T170922Z_seal-command-current-context-and-scope.md](../../docs/adr/20260612T170922Z_seal-command-current-context-and-scope.md)
  (Status: Partially superseded. The note at the top defines the current range.)
- **Implementation**:
  - [seal.go](seal.go) — `runSealTest` / `parseSealLsArgs` / `cmdSealLs`
  - [seal_path.go](seal_path.go) — `cmdSealTest`
- **test**:
  - [unit_test.go](unit_test.go) — `TestCmdSealTest*` / `TestRunSealTest*`
  - [command_test.go](command_test.go) — `TestCmdSealLs*`
  - [integration_test.go](integration_test.go) — `TestSealTest*` / `TestSealLs*`
