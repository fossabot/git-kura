# IMPLEMENTATION_MAP（`cmd/git-kura`）

> **これは maintainer 向けの実装対応表である。利用者向けの command reference ではない。**
>
> 各 command の使い方は [docs/commands.md](../../docs/commands.md) や
> [docs/commands/seal-commands.md](../../docs/commands/seal-commands.md) を参照すること。
> この文書は「仕様の概要」「背景となる ADR」「対応する実装 / schema / test」を結びつけ、
> 実装を変更するときに関連する設計判断を見落としにくくすることだけを目的とする。
>
> 仕様の詳細・決定の背景・schema field 一覧・command reference はここに再掲しない。
> いずれも実装 file / ADR / schema file / `docs/` への参照で代替する。

## status の意味

| status | 意味 |
|--------|------|
| `implemented` | 現行実装が ADR の決定どおり |
| `planned` | ADR にあるが未実装 |
| `superseded` | 後続 ADR により完全に置き換えられた |
| `partially superseded` | 決定の一部のみ現行。現行として参照してよい範囲を各項目に明記する |

---

## seal store の管理方法と writer lock

- **概要**: path → key の対応を単一の集中 store に保持し、writer lock で
  concurrent な seal 書き込みの TOCTOU を防ぐ。
- **status**: `partially superseded`
  - 現行として参照してよい範囲: 集中 store の layout
    （`<git-common-dir>/kura/seals/paths.json` + `paths.lock`）と、
    `O_CREATE|O_EXCL` による lock・atomic rename 書き込み・lock timeout の仕組み。
  - 参照してはいけない範囲: ADR 本文の `git kura seal add/remove` という command 名。
    現行の command 名は `seal claim/unclaim`（後述の「claim / unclaim の意味論」を参照）。
- **ADR**: [docs/adr/20260611T114623Z-use-centralized-seal-store.md](../../docs/adr/20260611T114623Z-use-centralized-seal-store.md)
- **実装**: [seal_path.go](seal_path.go)
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

## seal target path の正規化・制約

- **概要**: seal の対象を repository-relative file に限定し、repository root を
  基準に正規化して store に格納する。
- **status**: `partially superseded`
  - 現行として参照してよい範囲: path 制約と正規化規則
    （絶対 path / repository 外の拒否、repository root 基準での解決、
    forward-slash 格納など）。
  - 参照してはいけない範囲: ADR 本文の `git kura seal add/remove` という command 名と、
    `add`/`remove` での存在チェックの記述。現行の command 名は `seal claim/unclaim`。
- **ADR**: [docs/adr/20260611T114624Z-limit-seal-targets-to-repository-relative-files.md](../../docs/adr/20260611T114624Z-limit-seal-targets-to-repository-relative-files.md)
- **実装**: [seal_path.go](seal_path.go)
  - `normalizeSealPath`
  - `cmdSealClaim`
  - `cmdSealUnclaim`
- **test**: [unit_test.go](unit_test.go)
  - `TestNormalizeSealPath*`
  - `TestSealClaimRejectsAbsolutePath` / `TestSealClaimRejectsPathOutsideRepo`
    / `TestSealClaimResolvesPathsFromRepoRootNotCwd`（[integration_test.go](integration_test.go)）

---

## managed worktree 由来の current seal key 解決

- **概要**: current seal key を process-local な環境変数からではなく、
  現在いる git-kura 管理 worktree の identity と metadata から解決する。
- **status**: `implemented`
- **ADR**: [docs/adr/2026-06-13T06:46:51Z_seal-worktree-context-and-worktree-guards.md](../../docs/adr/2026-06-13T06:46:51Z_seal-worktree-context-and-worktree-guards.md)
  - 旧 session-local model（`seal enter` / `GIT_KURA_SEAL_KEY`）は
    [docs/adr/2026-06-11T11:46:22Z_use-session-local-seal-context.md](../../docs/adr/2026-06-11T11:46:22Z_use-session-local-seal-context.md)
    にあるが、この ADR は `superseded`。
- **実装**:
  - [seal_path.go](seal_path.go) — `readSealContext`
  - [../../internal/worktree/worktree.go](../../internal/worktree/worktree.go) — `CurrentKey`
- **schema**: [../../internal/worktree/schema/metadata.schema.json](../../internal/worktree/schema/metadata.schema.json)
- **test**:
  - [unit_test.go](unit_test.go) — `TestReadSealContextInsideWorktree` / `TestReadSealContextOutsideWorktree`
  - [../../internal/worktree/worktree_test.go](../../internal/worktree/worktree_test.go) — `TestCurrentKey*`

---

## claim / unclaim の意味論

- **概要**: 現在の task key が、編集前に repository-relative path の所有権を主張
  （claim）・解放（unclaim）する。別 key が claim 済みの path は cross-worktree
  conflict として拒否する。
- **status**: `partially superseded`
  - 現行として参照してよい範囲: `seal claim` / `seal unclaim` の意味論と、
    current key を worktree から解決する方針。
  - 参照してはいけない範囲: 同 ADR の以下は現行実装ではない。
    - `seal add` / `seal remove` の deprecated alias 維持
      — alias は残さず削除済み（現行 command は `claim` / `unclaim` のみ）。
    - `git kura guard acquire/release/status`（worktree guard）— 未実装（`planned`）。
    - `seal check --staged`（commit 時の staged check）— 未実装（`planned`）。
      現行に存在する近接 command は `seal test <path...>`。
- **ADR**: [docs/adr/2026-06-13T06:46:51Z_seal-worktree-context-and-worktree-guards.md](../../docs/adr/2026-06-13T06:46:51Z_seal-worktree-context-and-worktree-guards.md)
- **関連 Issue**: [#30](https://github.com/tooppoo/git-kura/issues/30)
- **実装**:
  - [seal.go](seal.go) — `runSeal` / `runSealClaim` / `runSealUnclaim`
  - [seal_path.go](seal_path.go) — `cmdSealClaim` / `cmdSealUnclaim` / `sealConflictError`
- **test**:
  - [integration_test.go](integration_test.go) — `TestSealClaim*` / `TestSealUnclaim*`
  - [unit_test.go](unit_test.go) — `TestCmdSealClaim*` / `TestCmdSealUnclaim*` / `TestRunSealClaim*`
