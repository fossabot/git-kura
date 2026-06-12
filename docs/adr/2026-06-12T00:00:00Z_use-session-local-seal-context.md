# Use Session-local Seal Context for `git kura seal`

* Status: Accepted
* Created: 2026-06-12T00:00:00Z

## Context

`git-kura` supports keyed Git worktree workflows.

When multiple agents or processes edit files in parallel, especially across multiple Git worktrees, conflicts can remain hidden until a later merge or integration step. A worktree isolates the working directory, but it does not prevent two agents from editing the same path in different worktrees. It also does not prevent two agents from accidentally using the same worktree and therefore sharing the same working tree and index.

`git kura seal` introduces an advisory mechanism for managing files under a task-specific key.

A seal key represents the current work context, such as:

```txt
issue-12
agent-x
claude-issue-34
```

Several operation models were considered:

1. Pass the key explicitly to every operation, for example `git kura seal add <key> <files...>`.
2. Store a current key per worktree.
3. Enter a session-local context with `git kura seal enter <key>` and let subsequent commands use that current key.

This ADR records the decision to use a session-local current key model.

Related issues:

* [#9](https://github.com/tooppoo/git-kura/issues/9) defines the overall `git kura seal` command set.
* [#13](https://github.com/tooppoo/git-kura/issues/13) defines `git kura seal enter <key>` and the worktree session guard.
* [#14](https://github.com/tooppoo/git-kura/issues/14) defines `git kura seal session ls` and `git kura seal session clean`.

## Decision

`git kura seal` uses a session-local current key.

The user enters a seal context with:

```sh
git kura seal enter <key>
```

For example:

```sh
git kura seal enter issue-12
```

`git kura seal enter <key>` starts a child shell with `GIT_KURA_SEAL_KEY=<key>` in its environment.

Conceptually, this is similar to:

```sh
GIT_KURA_SEAL_KEY=issue-12 "$SHELL"
```

Within that child shell, `git kura seal` commands read the current key from `GIT_KURA_SEAL_KEY`.

```sh
git kura seal current
git kura seal add src/foo.ts
git kura seal remove src/foo.ts
git kura seal test src/foo.ts
```

`git kura seal add` and `git kura seal remove` do not accept a key argument. They require a current seal key.

If no current seal key is available, `add` and `remove` fail.

`git kura seal leave` is not provided. Since `enter` starts a child shell, the user leaves the seal context by exiting that shell with `exit` or Ctrl-D.

## Session-local Current Key

The current key is represented by the `GIT_KURA_SEAL_KEY` environment variable.

This is session-local rather than worktree-local.

The process structure is:

```txt
user shell
  └── git kura seal enter issue-12
        └── child shell with GIT_KURA_SEAL_KEY=issue-12
              ├── git kura seal add ...
              ├── git commit
              └── pre-commit hook
```

The current key is inherited by child processes launched from the seal shell.

This matters for hooks. A `pre-commit` hook launched from inside the seal session can read `GIT_KURA_SEAL_KEY` and validate staged files against the current key.

## Worktree Session Guard

The environment variable alone is not sufficient.

`GIT_KURA_SEAL_KEY` is local to a process tree. It cannot, by itself, detect that another agent has entered the same worktree from another shell.

For example:

```sh
# agent x
cd worktree-a
git kura seal enter agent-x

# agent y
cd worktree-a
git kura seal enter agent-y
```

Both agents would have different process-local environment variables, but they would share the same working tree and index.

Therefore, `git kura seal enter <key>` also creates a worktree session record.

The session record is stored under the repository's Git common directory and is shared by all worktrees of the repository.

`enter` performs the following steps:

1. Resolve the current worktree.
2. Check the shared session store.
3. If the same worktree already has an active session for another key, fail.
4. If no active session exists, create a session record.
5. Start a child shell with `GIT_KURA_SEAL_KEY=<key>`.
6. Wait for the child shell to exit.
7. Remove the session record after the child shell exits.

The session record is not the source of the current key for commands inside the shell.

The environment variable is the source of the current key. The session record exists to detect concurrent entry into the same worktree.

## Stale Sessions

A session record may remain after abnormal termination.

Normal termination works as follows:

```txt
git-kura enter process
  └── child shell
        └── exit
git-kura removes the session record
```

However, the session record can remain if:

* the `git-kura` process is killed;
* the terminal is closed unexpectedly;
* the machine shuts down;
* the process crashes;
* the process is forcefully terminated on Windows.

This is an accepted trade-off.

`git-kura` does not attempt strict process supervision in v0. Heartbeat is intentionally not used.

The reasons are:

* v0 only needs early detection of accidental concurrent worktree entry;
* heartbeat would introduce additional design surface around update intervals, crash handling, clock drift, and cross-platform behavior;
* strict supervision can be reconsidered later if the workflow needs it.

Session records should include at least:

```json
{
  "key": "issue-12",
  "worktree": "/path/to/worktree-a",
  "parent_pid": 12345,
  "child_pid": 12346,
  "started_at": "2026-06-12T00:00:00Z"
}
```

Stale session handling follows these rules:

* PID checks are best-effort.
* PID absence may indicate a stale session.
* PID presence is not a complete proof of activity, because PIDs can be reused.
* TTL is used only as a warning signal.
* TTL must not be used as the sole reason for deleting a session.
* The default TTL is about five minutes.
* The TTL should be configurable.
* Session inspection and cleanup are handled by `git kura seal session ls` and `git kura seal session clean`.

## Alternatives Considered

### Pass the key explicitly to each operation

Example:

```sh
git kura seal add issue-12 src/foo.ts
git kura seal remove issue-12 src/foo.ts
```

This was rejected.

It is explicit, but it is repetitive and error-prone. The user or agent must pass the correct key on every operation.

This is especially problematic for agents. An agent needs a reliable way to determine "which context am I currently operating in?" without relying only on conversation context. If the conversation is compressed, truncated, or partially lost, the agent may lose track of the intended key.

A session-local current key externalizes the current context into the process environment. The agent can recover the context by running:

```sh
git kura seal current
```

This makes the active work context observable from the execution environment rather than only from prompt history.

### Store the current key per worktree

This was rejected.

A worktree-local current key can model this situation:

```txt
worktree-a -> issue-12
worktree-b -> issue-13
```

However, it fails when multiple agents or processes use the same worktree.

If agent x enters `agent-x` and agent y later enters `agent-y` in the same worktree, a worktree-local current key would be overwritten. It cannot distinguish concurrent process sessions within the same worktree.

The current key is therefore session-local, not worktree-local.

### Use only an environment variable without a session guard

This was rejected as insufficient.

An environment variable lets each process tree know its own current key, but it does not expose that state to other process trees.

That means it cannot detect two agents entering the same worktree.

The accepted design combines:

1. a session-local current key via `GIT_KURA_SEAL_KEY`; and
2. a shared worktree session guard stored under the Git common directory.

## Consequences

### Positive Consequences

* `git kura seal add` and `git kura seal remove` remain simple.
* The current work context is externally observable through `git kura seal current`.
* Agents can recover their current key after prompt compression or context loss.
* Key argument mistakes are reduced.
* Pre-commit hooks can inherit `GIT_KURA_SEAL_KEY`.
* The session guard detects accidental concurrent entry into the same worktree.
* The design keeps seal ownership and session entry separate: the environment provides current context, while the shared session store detects worktree-level concurrency.

### Negative Consequences

* `git kura seal enter` is more complex than a simple environment-variable wrapper.
* `enter` must create, wait on, and clean up a session record.
* Stale session records can remain after abnormal termination.
* Cross-platform shell launching and PID checks require careful implementation.
* `git kura seal leave` is not available; users must exit the child shell.

### Neutral Consequences

* TTL is a warning mechanism, not a deletion mechanism.
* Heartbeat is intentionally deferred.
* `git kura seal session ls` and `git kura seal session clean` are separate commands rather than part of the core `enter` flow.
