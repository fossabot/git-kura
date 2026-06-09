# State Management

Kura manages local worktrees and metadata outside the repository root.

For a repository at:

```txt
<repo-parent>/<repo>
```

Kura stores its local state under:

```txt
<repo-parent>/<repo>.kura/
```

The directory layout is:

```txt
<repo-parent>/<repo>.kura/
  worktrees/
    <key>/
  meta/
    worktrees/
      <key>.json
```

## Worktrees

Each key maps to a deterministic worktree path:

```txt
<repo-parent>/<repo>.kura/worktrees/<key>
```

For example, if the repository is:

```txt
/home/user/project
```

Then issue `51` maps to:

```txt
/home/user/project.kura/worktrees/51
```

Kura keeps worktrees outside the repository root so commands such as `git clean -fdx` in the main repository do not remove them.

## Metadata

Each key also has a metadata file:

```txt
<repo-parent>/<repo>.kura/meta/worktrees/<key>.json
```

The metadata file is created by `git kura open <key>`. It records creation-time state that cannot always be reconstructed later, such as the base branch used when the worktree was opened.

Runtime state such as whether the worktree exists and whether it is dirty is computed when metadata is resolved for structured output.

## Structured Output

`git kura get <key> --json` and `git kura get <key> --toon` are generated from resolved metadata.

Kura must only produce structured output from schema-valid metadata. If metadata is missing or cannot be validated, structured output should fail rather than guessing fields such as `baseBranch`.

Scalar outputs remain deterministic:

```sh
git kura get 51 --path
git kura get 51 --branch
```

These values can be computed from the repository root and key even if stored metadata is missing.

## Cleanup

All Kura-managed local state for a repository lives under one sibling directory:

```txt
<repo-parent>/<repo>.kura/
```

Removing that directory removes Kura-managed worktrees and metadata for that repository.

Do not remove only `meta/` unless you intentionally want to discard Kura metadata. Some metadata, including `baseBranch`, cannot be safely reconstructed from Git after the fact.

## Rationale

Kura does not store state in `<repo>/.git-kura/` because ignored files inside the repository root can be removed by `git clean -fdx`.

Kura also avoids generic sibling names such as `<repo>.worktrees/` because they may collide with other tools or local conventions. The `<repo>.kura/` directory clearly marks the state as Kura-owned and keeps cleanup simple.
