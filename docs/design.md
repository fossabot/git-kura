# Design

Kura is a keyed worktree resolver for Git.

Its core responsibility is intentionally narrow: given a stable key such as an
issue, ticket, or task number, Kura creates, resolves, and removes a
deterministic Git worktree.

## Design principles

### 1. The key is the source of truth

Kura should not require users or agents to remember worktree paths manually. The
task key should be enough.

```sh
git kura get 51 --path
```

### 2. Output should be script-friendly

Commands that return values should be usable in shell scripts without extra
formatting.

```sh
cd "$(git kura get 51 --path)"
```

For structured output, use JSON:

```sh
git kura get 51 --json
```

For AI prompts, Kura provides [TOON](https://github.com/toon-format/toon):

```sh
git kura get 51 --toon
```

### 3. Kura should stay small

Kura should not become an AI session manager, TUI Git client, pull request
orchestrator, or issue tracker client.

Those tools may integrate with Kura, but Kura itself should remain focused on
keyed worktree lifecycle management.

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

## Platform support

Kura supports macOS, Linux, and Windows.

Path handling uses platform-aware APIs. Git branch names and filesystem paths
are treated as distinct:

```txt
branch: issue/51
path:   <repo>-issue-51  (platform path separator)
```
