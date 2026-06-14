---
name: documentation
description: >
  git-kura の README や docs を作成・更新するときに使う documentation skill。
  docs/adr 配下に ADR を作るとき、既存ドキュメントを変更するとき、またはドキュメント間の矛盾を検証するときに必ず参照すること。
  文中で prose を途中改行せず、ADR は docs/adr/TEMPLATE.md と docs/adr/README.md に従う。
---

# git-kura documentation skill

この skill は git-kura リポジトリのドキュメント作成・更新で使う。

## 文章ルール

- 対象ファイルが明示的に別言語で書かれていない限り、ドキュメント本文は英語で書く。
- 文中や段落途中で prose を hard wrap しない。
- prose の段落は 1 段落 1 物理行にする。
- 段落、見出し、リスト、表、コードブロックの間は空行で区切る。
- リストは 1 item 1 物理行でよい。
- コードブロックはコードやコマンド出力に必要な改行を維持してよい。
- 既存ドキュメントを編集するときは、関係ない文言変更を避ける。

NG:

```md
git-kura defense
conflict by multi agent.
it is provided
as git subcommand.
```

OK:

```md
git-kura defense conflict by multi agent.
it is provided as git subcommand.
```

## ADR 作成

- ADR は `docs/adr/` 直下に作る。
- `docs/adr/TEMPLATE.md` を開始テンプレートにする。
- ファイル名は `docs/adr/README.md` の File Naming に従う。
- UTC timestamp は `date -u +"%Y%m%dT%H%M%SZ"` で生成する。
- ファイル名 timestamp と ADR の `Created` は同じ instant にする。
- timestamp の後ろには短い lowercase kebab-case slug を付ける。
- `Status` は `docs/adr/README.md` に定義された形式だけを使う。
- durable な architecture decision を変える場合は新しい ADR を作り、accepted ADR の decision を黙って書き換えない。

## 既存ファイル変更

編集前に、変更対象の behavior や policy を特定する。

関連箇所を targeted search で確認する。

```sh
rg -n "keyword|command|field|policy" README.md docs .agents .claude
```

command、output contract、state rule、worktree rule、safety policy、agent workflow を変える場合は、user docs、ADR、skill が互いに矛盾していないか確認する。

矛盾が見つかった場合は、同じ変更内で関係するドキュメントも更新する。

挙動変更による意図的な矛盾なら、新しいルールを記録する ADR や doc を追加・更新する。

## 検証

完了前に focused check を実行する。

```sh
git diff --check
rg -n ".{121,}" README.md docs .agents .claude
```

long-line の検出結果は文脈で判断する。prose は 1 行を維持する一方で、表、リンク、コード、生成例は長くても妥当な場合がある。

ADR では以下を確認する。

- path が `docs/adr/` 直下である。
- filename が `YYYYMMDDTHHMMSSZ_short-kebab-case-title.md` に一致する。
- `Created` が filename timestamp と一致する。
- `docs/adr/README.md` の required sections がある。
- 明示的に supersede していない既存 ADR と矛盾しない。

最後に、確認したファイルと残っている不確実性を報告する。
