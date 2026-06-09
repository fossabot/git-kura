# git-kura

[![CI](https://github.com/tooppoo/git-kura/actions/workflows/ci.yml/badge.svg)](https://github.com/tooppoo/git-kura/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/tooppoo/git-kura/graph/badge.svg?token=5f8XJ77qiN)](https://codecov.io/gh/tooppoo/git-kura)

git-kura is a keyed worktree resolver for Git.

It maps issue, ticket, or task keys to deterministic Git worktrees for humans, AI coding agents, and reviewers.

```sh
git kura open 51              # create a worktree for Issue #51
git kura open 51 --dry-run    # print the worktree that would be created
git kura get 51               # print the open worktree path for Issue #51
git kura get 51 --path        # print the worktree path for Issue #51
git kura get 51 --branch      # print the branch name for Issue #51
git kura get 51 --format json # print workspace metadata as JSON
git kura get 51 --json        # alias of `--format json`
git kura get 51 --format toon # print workspace metadata as TOON for AI prompts
git kura get 51 --toon        # alias of `--format toon`
git kura close 51             # remove the worktree for Issue #51
git kura ls                   # list all open worktrees
```

## Concept

Kura is built around three ideas:

* **Worktree first**: each task gets its own Git worktree.
* **Issue-keyed development**: an issue, ticket, or task number is used as the stable key for the workspace.
* **AI-friendly resolution**: scripts, coding agents, and reviewers can resolve the same key to the same branch and worktree path without guessing.

Kura is intentionally small. It is not a Git client, an AI agent manager, a pull request tool, or a project management tool. Its job is to provide a stable mapping between a task key and a Git worktree.

## Why Kura?

When using Git worktrees with multiple AI coding agents, it is easy to lose track of which worktree belongs to which task.

For example, one agent may implement a change in a task-specific worktree while another tool or reviewer needs to inspect that exact worktree later. If the review target is selected manually, the workflow becomes fragile.

Kura solves this by making the task key the source of truth.

```txt
task key
  -> branch name
  -> worktree path
```

The same key should always resolve to the same workspace.

## Usage

Open a worktree for issue `51`:

```sh
git kura open 51
```

Preview the worktree that would be created:

```sh
git kura open 51 --dry-run
```

Resolve the worktree path:

```sh
cd "$(git kura get 51)"
```

Resolve the branch name:

```sh
git kura get 51 --branch
```

Get machine-readable metadata:

```sh
git kura get 51 --json
git kura get 51 --toon
```

Close work on issue `51`:

```sh
git kura close 51
```

See [docs/commands.md](docs/commands.md) for the command reference and [docs/output-format.md](docs/output-format.md) for structured output formats.

## Installation

### Build from source

Requirements: Go toolchain, Git

```sh
make build
```

This produces `./bin/git-kura`. Place it somewhere on `PATH` so Git can find it as a subcommand:

```sh
cp ./bin/git-kura /usr/local/bin/git-kura
```

## Documentation

* [Commands](docs/commands.md)
* [Output formats](docs/output-format.md)
* [State management](docs/state-management.md)
* [Design](docs/design.md)
* [Architecture decision records](docs/adr/)

## Status

Kura is currently under design.

The command names, output format, and safety behavior may change before the first stable release.

## License

Apache License 2.0.
