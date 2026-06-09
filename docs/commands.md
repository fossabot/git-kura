# Commands

Kura is invoked as a Git subcommand:

```sh
git kura <command> [arguments]
```

## `git kura open <key>`

Create the branch and worktree for the given key.

```sh
git kura open 51
```

If the corresponding branch or worktree already exists, Kura should not create a
conflicting workspace.

## `git kura get <key>`

Resolve the branch or worktree associated with the given key.

```sh
git kura get 51 --path
git kura get 51 --branch
git kura get 51 --json
git kura get 51 --toon
```

This command is designed for both humans and scripts.

For example:

```sh
codex review "$(git kura get 51 --path)"
```

See [output-format.md](output-format.md) for the full metadata schema and output
format reference.

## `git kura close <key>`

Remove the worktree associated with the given key.

```sh
git kura close 51
```

Kura should refuse to remove a worktree when doing so would discard uncommitted
changes unless explicitly instructed.

## Keys

A key is an opaque, case-sensitive string identifier.

Kura does not parse keys as numbers. Kura does not resolve keys through GitHub,
GitLab, or any issue tracker.

In v0, a key must match:

```txt
[A-Za-z0-9][A-Za-z0-9._-]{0,127}
```

Additionally, Kura rejects keys that:

* are `"."` or `".."`
* contain `".."`
* start with `"."`
* end with `"."`
* end with `".lock"`
* contain path separators `"/"` or `"\"`
* contain whitespace
* contain control characters
* contain shell metacharacters
* contain Git ref expression syntax such as `"@{"`

## Exit codes

Kura uses stable exit codes so scripts and AI-agent workflows can react
correctly.

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Usage error |
| 3 | Unsafe operation refused |
| 4 | Not found |
