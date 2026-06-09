# State Management

Kura manages local worktrees and metadata under Git's common directory.

For a repository whose Git common directory is:

```txt
<git-common-dir>
```

Kura stores its local state under:

```txt
<git-common-dir>/kura/
```

The directory layout is:

```txt
<git-common-dir>/kura/
  worktrees/
    <key>/
  meta/
    worktrees/
      <key>.json
```

## Worktrees

Each key maps to a deterministic worktree path:

```txt
<git-common-dir>/kura/worktrees/<key>
```

For example, if the repository is:

```txt
/home/user/project
```

And its Git common directory is:

```txt
/home/user/project/.git
```

Then issue `51` maps to:

```txt
/home/user/project/.git/kura/worktrees/51
```

Kura keeps worktrees outside the working tree so commands such as `git clean -fdx` in the main repository do not remove them. Git does not clean files under its own `.git` directory as working tree content.

## Metadata

Each key also has a metadata file:

```txt
<git-common-dir>/kura/meta/worktrees/<key>.json
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

All Kura-managed local state for a repository lives under one directory:

```txt
<git-common-dir>/kura/
```

Removing that directory removes Kura-managed worktrees and metadata for that repository.

Do not remove only `meta/` unless you intentionally want to discard Kura metadata. Some metadata, including `baseBranch`, cannot be safely reconstructed from Git after the fact.

## Rationale

Kura does not store state in `<repo>/.git-kura/` because ignored files inside the repository root can be removed by `git clean -fdx`.

Kura also avoids sibling directories such as `<repo>.kura/` because some development environments, including devcontainers, allow writes inside the repository but not to the repository parent directory. Git's common directory survives `git clean`, is local to the repository, and is writable in those environments.
