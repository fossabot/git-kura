# git-kura

git-kura is a keyed worktree resolver for Git.

It maps issue, ticket, or task keys to deterministic Git worktrees for humans, AI coding agents, and reviewers.

```sh
git kura open 51              # create a worktree for Issue #51
git kura get 51               # print workspace metadata for Issue #51
git kura get 51 --path        # print the worktree path for Issue #51
git kura get 51 --branch      # print the branch name for Issue #51
git kura get 51 --format json # print workspace metadata as JSON
git kura get 51 --json        # alias of `--format json`
git kura get 51 --format toon # print workspace metadata as TOON for AI prompts
git kura get 51 --toon        # alias of `--format toon`
git kura close 51             # remove the worktree for Issue #51
```

## Concept

Kura is built around three ideas:

* **Worktree first**: each task gets its own Git worktree.
* **Issue-keyed development**: an issue, ticket, or task number is used as the stable key for the workspace.
* **AI-friendly resolution**: scripts, coding agents, and reviewers can resolve the same key to the same branch and worktree path without guessing.

Kura is intentionally small. It is not a Git client, an AI agent manager, a pull request tool, or a project management tool. Its job is to provide a stable mapping between a task key and a Git worktree.

## Why Kura?

When using Git worktrees with multi AI coding agents, it is easy to lose track of which worktree belongs to which task.

For example, one agent may implement a change in a task-specific worktree while another tool or reviewer needs to inspect that exact worktree later. If the review target is selected manually, the workflow becomes fragile.

Kura solves this by making the task key the source of truth.

```txt
task key
  -> branch name
  -> worktree path
```

The same key should always resolve to the same workspace.

## Example

Open worktree on issue `51`:

```sh
git kura open 51
```

Resolve the worktree path for issue `51`:

```sh
cd "$(git kura get 51 --path)"
```

Resolve the branch name:

```sh
git kura get 51 --branch
```

Get machine-readable metadata:

```sh
git kura get 51 --json
```

End work on issue `51`:

```sh
git kura end 51
```

## Commands

### `git kura open <key>`

Create the branch and worktree for the given key.

```sh
git kura start 51
```

If the corresponding branch or worktree already exists, Kura should not create a conflicting workspace.

### `git kura get <key>`

Resolve the branch or worktree associated with the given key.

```sh
git kura get 51 --path
git kura get 51 --branch
git kura get 51 --json
```

This command is designed for both humans and scripts.

For example:

```sh
codex review "$(git kura get 51 --path)"
```

### `git kura close <key>`

Remove the worktree associated with the given key.

```sh
git kura end 51
```

Kura should refuse to remove a worktree when doing so would discard uncommitted changes unless explicitly instructed.

## Design principles

### 1. The key is the source of truth

Kura should not require users or agents to remember worktree paths manually. The task key should be enough.

```sh
git kura get 51 --path
```

### 2. Output should be script-friendly

Commands that return values should be usable in shell scripts without extra formatting.

```sh
cd "$(git kura get 51 --path)"
```

For structured output, use JSON:

```sh
git kura get 51 --json
```

For AI prompts, `kura` provide [TOON](https://github.com/toon-format/toon):

```sh
git kura get 51 --toon
```

### 3. Kura should stay small

Kura should not become an AI session manager, TUI Git client, pull request orchestrator, or issue tracker client.

Those tools may integrate with Kura, but Kura itself should remain focused on keyed worktree lifecycle management.

### 4. Safety over convenience

Removing a worktree should be conservative.

Kura should check for conditions such as:

* uncommitted changes
* untracked files
* missing worktree paths
* branch/worktree mismatches
* dirty submodules, if applicable

When in doubt, Kura should stop and explain what must be resolved manually.

## Non-goals

Kura does not aim to:

* manage AI coding sessions
* create or review pull requests
* replace GitHub CLI, GitLab CLI, or other issue tracker tools
* provide a full Git TUI
* infer the correct worktree from natural language
* classify or evaluate task priority

Kura only manages the mapping between a key and a Git worktree.

## Installation

Installation instructions will depend on the distribution method.

For example:

```sh
# TODO
```

## Status

Kura is currently under design.

The command names, output format, and safety behavior may change before the first stable release.

## License

Apache License 2.0.
