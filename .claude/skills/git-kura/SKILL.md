---
name: git-kura
description: >
  git-kura リポジトリ自体を開発・レビューするときに使う worktree 管理 skill。
  「〇〇 issue / key を実装して」「〇〇 key をレビューして」という依頼を受けたとき、
  または worktree を切る・移動する・閉じる操作が必要なときに必ず参照すること。
  worktree path は推測せず、必ず git-kura コマンドで解決する。
---

# git-kura worktree skill (dogfood / maintainer 用)

このリポジトリ自体を `git-kura` で開発するための skill。
**worktree path を手で推測したり、`git worktree add` を直接使ったりしない。**
task key を source of truth として、すべての worktree 操作を `git kura` 経由で行う。

---

## 基本原則

- `git kura` が利用可能な前提で動く（未インストールなら `make build && cp ./bin/git-kura /usr/local/bin/git-kura` を案内する）
- worktree path は **必ず** `git kura get <key>` で解決する
- main checkout を task の作業場所として使わない
- `merge` / `rebase` / `push` / `close` は明示指示があるときだけ実行する

---

## ワークフロー 1：開発依頼を受けたとき

**トリガー**: 「issue N を実装して」「key X の機能を作って」など

```
1. key を確認する
   - 依頼文から key を特定する（例: issue番号、feature名）
   - 不明なら確認する

2. worktree を開く
   git kura open <key>
   # → branch と worktree を決定論的に作成

3. worktree に移動して作業開始
   cd "$(git kura get <key>)"
   # この中で実装・コミットを行う

4. 作業完了後、明示指示があれば push / merge
   # 指示なしでは実行しない
```

---

## ワークフロー 2：レビュー依頼を受けたとき

**トリガー**: 「key X をレビューして」「〇〇 の変更を確認して」など

```
1. key を確認する
   - 依頼文から key を特定する
   - 不明なら確認する

2. key が open 済みか確認する
   git kura ls
   # open されていなければ依頼者に確認し、必要なら git kura open <key>

3. worktree を解決して移動
   cd "$(git kura get <key>)"

4. レビュー開始
   - diff / log / test 結果を確認する
   - AI prompt 向けコンテキストが必要なら: git kura get <key> --toon
   - script 向け metadata が必要なら:     git kura get <key> --json
```

---

## worktree 終了時の safety check

`git kura close <key>` を実行する前に **必ず** 以下を確認する。

```bash
cd "$(git kura get <key>)"
git status --short
```

以下のいずれかがある場合は、明示確認なしに close しない。

- uncommitted changes
- untracked files（意図的なものか確認）
- unpushed commits
- 未解決の merge / rebase 判断

---

## コマンドリファレンス（よく使うもの）

| 目的 | コマンド |
|------|---------|
| worktree を作成 | `git kura open <key>` |
| worktree path を解決 | `git kura get <key>` |
| branch 名を解決 | `git kura get <key> --branch` |
| AI prompt 向け context | `git kura get <key> --toon` |
| script 向け metadata | `git kura get <key> --json` |
| repo root を取得 | `git kura get <key> --root` |
| open 中の一覧 | `git kura ls` |
| worktree を閉じる | `git kura close <key>` ※safety check 後 |
| dry-run で確認 | `git kura open <key> --dry-run` |

exit code: 0=成功 / 1=一般エラー / 2=使い方エラー / 3=unsafe拒否 / 4=not found

---

## Claude Code との関係

Claude Code の `--worktree` mode と git-kura 管理 worktree を **混同しない**。

推奨運用:
```bash
git kura open <key>
cd "$(git kura get <key>)"
claude          # git-kura が作成した worktree の中で Claude Code を起動
```

Claude Code に worktree を作らせるのではなく、git-kura が解決した worktree の中で起動する。
