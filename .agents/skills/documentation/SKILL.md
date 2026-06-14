---
name: documentation
description: Create or update git-kura documentation. Use when writing README or docs content, creating Architecture Decision Records under docs/adr, changing documented behavior, or checking that documentation changes stay consistent with existing git-kura docs and ADRs.
---

# git-kura Documentation

Use this skill when creating or changing documentation in the git-kura repository.

## Writing Rules

- Write documentation in English unless the target file is already intentionally written in another language.
- Do not hard-wrap prose inside a sentence or paragraph.
- Keep each prose paragraph on a single physical line.
- Use blank lines between paragraphs, headings, lists, tables, and code blocks.
- Lists may use one physical line per item.
- Code blocks may preserve the line breaks required by the code or command output.
- Avoid changing unrelated wording while editing existing documentation.

Bad:

```md
git-kura defense
conflict by multi agent.
it is provided
as git subcommand.
```

Good:

```md
git-kura defense conflict by multi agent.
it is provided as git subcommand.
```

## ADR Creation

- Create ADRs directly under `docs/adr/`.
- Use `docs/adr/TEMPLATE.md` as the starting structure.
- Follow the filename rules in `docs/adr/README.md`.
- Generate the timestamp in UTC with `date -u +"%Y%m%dT%H%M%SZ"`.
- Use the same instant in the filename timestamp and the ADR `Created` field.
- Use a short lowercase kebab-case slug after the timestamp.
- Keep the status value in one of the forms documented in `docs/adr/README.md`.
- Create a new ADR when changing a durable architecture decision; do not rewrite an accepted ADR except for allowed maintenance updates.

## Updating Existing Documentation

Before editing, identify the documented behavior or policy being changed.

Check related files with targeted searches, for example:

```sh
rg -n "keyword|command|field|policy" README.md docs .agents .claude
```

When changing a command, output contract, state rule, worktree rule, safety policy, or agent workflow, check the related user docs, ADRs, and skills for contradictions.

Prefer updating all affected documentation in the same change rather than leaving stale references.

If a contradiction is intentional because behavior changed, update or add the ADR/doc that records the new rule.

## Validation

Run focused checks before finishing:

```sh
git diff --check
rg -n ".{121,}" README.md docs .agents .claude
```

Interpret long-line output carefully: prose should remain on one line, while tables, links, code, and generated examples may legitimately exceed the search threshold.

For ADRs, verify:

- The path is directly under `docs/adr/`.
- The filename matches `YYYYMMDDTHHMMSSZ_short-kebab-case-title.md`.
- The `Created` value matches the filename timestamp.
- Required sections from `docs/adr/README.md` are present.
- The content does not contradict existing ADRs unless it explicitly supersedes them.

Report which files were checked and any remaining uncertainty.
