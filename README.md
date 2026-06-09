# git-kura

[![CI](https://github.com/tooppoo/git-kura/actions/workflows/ci.yml/badge.svg)](https://github.com/tooppoo/git-kura/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/tooppoo/git-kura/graph/badge.svg?token=5f8XJ77qiN)](https://codecov.io/gh/tooppoo/git-kura)

`git-kura` is a keyed worktree resolver for Git.

It maps issue, ticket, task or feature keys to deterministic Git worktrees for humans, AI coding agents, and reviewers.

`git-kura` provides following commands.

```sh
git kura open fizz              # create a worktree and branch for key "fizz"
git kura open fizz --dry-run    # print the worktree that would be created
git kura get fizz               # print the open worktree path for "fizz"
git kura get fizz --path        # print the worktree path for "fizz"
git kura get fizz --branch      # print the branch name for "fizz"
git kura get fizz --root        # print the repository root path
git kura get fizz --format json # print workspace metadata as JSON
git kura get fizz --json        # alias of `--format json`
git kura get fizz --format toon # print workspace metadata as TOON for AI prompts
git kura get fizz --toon        # alias of `--format toon`
git kura close fizz             # remove the worktree for "fizz"
git kura ls                     # list all open worktrees
```

## Walkthrough

```sh
git kura open fizz-feature             # create a worktree and a branch in a repository
cd $(git kura get fizz-feature)        # move to the worktree

# edit, save and commit in the worktree

cd $(git kura get fizz-feature --root) # move to the repository root

git merge fizz-feature                 # merge changes to main stream

git kura close fizz-feature            # clean up worktree and branch
```

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

## Concept

Kura（`蔵`） is built around four ideas:

* **Keyed workspaces**: any stable key — an issue number, ticket ID, task name, feature slug, or review target — can identify one workspace.
* **Worktree isolation**: each key gets its own Git worktree and branch.
* **A small local kura**: Kura（`蔵`） keeps keyed worktrees and metadata in a repository-local store, so the right workspace can be put away, found, and cleaned up without relying on memory.
* **Deterministic resolution**: humans, scripts, AI coding agents, and reviewers can resolve the same key to the same workspace without guessing.

Kura is intentionally small. It is not a Git client, an AI agent manager, a pull request tool, an issue tracker client, or a project management tool. Its job is to provide a stable mapping between a key and a local Git worktree.

## Usage

Open a worktree for issue `fizz`:

```sh
git kura open fizz
```

Preview the worktree that would be created:

```sh
git kura open fizz --dry-run
```

Resolve the worktree path:

```sh
cd "$(git kura get fizz)"
```

Resolve the branch name:

```sh
git kura get fizz --branch
```

Resolve the repository root path:

```sh
cd $(git kura get fizz --root)
```

Get machine-readable metadata:

```sh
git kura get fizz --json
git kura get fizz --toon
```

Close work on issue `fizz`:

```sh
git kura close fizz
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
