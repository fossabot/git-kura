# Seal Command Current Context and Scope

- Status: Partially superseded
- Created: 2026-06-12T17:09:22Z
- Amended: 2026-06-13

> **Partially superseded.** The read-only-vs-mutation classification this ADR
> establishes still holds, and it now governs the implemented commands: `seal ls`
> is repository-wide and ignores the current key, while `seal claim` /
> `seal unclaim` (the renamed `seal add` / `seal remove`) and `seal test` require
> a current seal key. However, the mechanism described here for *establishing*
> that key — `git kura seal enter <key>` setting `GIT_KURA_SEAL_KEY` for a child
> shell — has been withdrawn, along with the `seal session ls` /
> `seal session clean` inspection and maintenance commands, in
> [#29](https://github.com/tooppoo/git-kura/issues/29). The current key is derived
> from the active git-kura managed worktree per
> [`2026-06-13T06:46:51Z_seal-worktree-context-and-worktree-guards.md`](2026-06-13T06:46:51Z_seal-worktree-context-and-worktree-guards.md).
> `GIT_KURA_SEAL_KEY` no longer participates in current-key resolution at all:
> [#32](https://github.com/tooppoo/git-kura/issues/32) moved `claim` / `unclaim`
> onto worktree-derived keys, and [#20](https://github.com/tooppoo/git-kura/issues/20)
> implemented `seal test` the same way. Wherever this ADR conditions behavior on
> `GIT_KURA_SEAL_KEY` being set, unset, or invalid, read it as "the worktree-derived
> current key" instead. Treat the `seal enter` / `seal session ...` references and
> the `seal add` / `seal remove` names below as historical context, not current
> design.

## Context

`git kura seal` uses a seal key to represent the working context of a process or agent.

`git kura seal enter <key>` establishes the current seal key by setting `GIT_KURA_SEAL_KEY` for a child shell and its descendant processes. This makes the current key process-local and session-local, rather than a repository-wide persistent setting.

The seal command group has several kinds of commands:

- commands that change path seal ownership, such as `seal add` and `seal remove`
- commands that validate whether a path may be handled in a working context, such as `seal test`
- commands that inspect repository-wide state, such as `seal ls`, `seal session ls`, and `seal doctor`
- maintenance commands that modify diagnostic records, such as `seal session clean`

These commands should not all use the current seal key in the same way.

For mutation and context-validation commands, using the wrong key is dangerous because it can make an agent believe that it is allowed to edit or stage paths under the wrong working context. These commands are semantically tied to the active work context.

For inspection commands, implicit dependence on `GIT_KURA_SEAL_KEY` makes output harder to reason about. The same command would show different scopes depending on the caller's environment, which is inconvenient for humans, scripts, and agents. Inspection commands are more useful when they show repository-wide state by default and require explicit arguments for narrower scopes.

This decision affects public CLI behavior, command documentation, help text, and future seal command design. It therefore needs to be recorded as an ADR rather than only discussed in issues.

Related issues:

- [#9: git kura seal コマンドを追加する](https://github.com/tooppoo/git-kura/issues/9)
- [#14: git kura seal session ls / clean を追加する](https://github.com/tooppoo/git-kura/issues/14)
- [#18: git kura seal add / remove を実装する](https://github.com/tooppoo/git-kura/issues/18)
- [#19: git kura seal ls で path seal を一覧できるようにする](https://github.com/tooppoo/git-kura/issues/19)

## Decision

Seal commands are classified by their effect and by whether their meaning depends on a working context.

### Current-dependent commands

Commands that change path seal ownership must require a current seal key.

This includes:

```sh
git kura seal add <file...>
git kura seal remove <file...>
```

If `GIT_KURA_SEAL_KEY` is unset, empty, or invalid, these commands must fail.

Context-validation commands also depend on the current seal key by default.

This includes:

```sh
git kura seal test <file...>
```

Although `seal test` is read-only, it answers whether the specified files may be handled in the current working context. It is therefore grouped with the commands that prepare or protect mutations.

In v0, `seal test <file...>` must fail when there is no valid current seal key. The project-wide validation mode is not part of v0. Options such as `seal test --all <file...>` or `seal test --unsealed <file...>` may be reconsidered later, but they are not included in this decision.

When a valid current key exists, `seal test <file...>` uses that key as the validation context:

- unsealed files are allowed
- files sealed by the current key are allowed
- files sealed by another key are rejected

### Current-independent inspection commands

Inspection commands must not depend on `GIT_KURA_SEAL_KEY` by default.

This includes:

```sh
git kura seal ls
git kura seal session ls
git kura seal doctor
```

These commands inspect repository-wide state by default.

The repository-wide project scope is defined as the Git common dir resolved from the current working directory. In other words, the project scope is the state associated with the current Git repository's common dir, including state shared by multiple worktrees of that repository.

When a narrower key scope is supported, the key must be specified explicitly.

For example:

```sh
git kura seal ls <key>
git kura seal session ls <key>
```

`seal ls` must not use `GIT_KURA_SEAL_KEY` to decide its default display scope. Running `seal ls` inside a shell created by `seal enter <key>` must still show repository-wide path seal state unless a key argument is explicitly provided.

`seal doctor` is repository-wide in v0. It must not modify seal state, and v0 does not support `seal doctor --fix`.

### Current-independent maintenance commands

Maintenance commands that operate on repository-level diagnostic records do not use the current seal key merely because they mutate state.

This includes:

```sh
git kura seal session clean
```

`session clean` operates on session records, not on path seal ownership for the current working context. Its safety model is therefore different from `seal add` and `seal remove`.

For v0, `session clean` must be dry-run by default. It may delete records only when explicit mutation flags such as `--apply` are used. Wider cleanup modes such as `--force` must remain explicit and must not be implied by the current seal key.

### Documentation requirement

Help text and command documentation must explicitly describe this asymmetry.

At minimum, documentation should explain that:

- path seal mutation commands require a current seal key
- context-validation commands such as `seal test` also require a current seal key in v0
- inspection commands do not use the current seal key by default
- inspection commands inspect the repository-wide state resolved from the Git common dir
- narrower inspection scopes must be requested explicitly
- maintenance commands use their own safety flags instead of relying on current seal context

## Alternatives Considered

### Make all seal commands use the current key by default

This would make `seal ls` show only the current key's paths when `GIT_KURA_SEAL_KEY` is set.

This was rejected because it makes read-only inspection depend on process-local environment state. The same command would produce different output depending on whether it is executed inside a shell created by `seal enter <key>`. That is convenient in some interactive cases, but it is less deterministic for scripts, tests, and agents.

The project may add explicit key-scoped inspection commands or arguments where useful, but the default inspection scope should remain repository-wide.

### Make all commands current-independent and require explicit key arguments

This would allow commands such as:

```sh
git kura seal add <key> <file...>
git kura seal remove <key> <file...>
```

This was rejected because mutation commands should be tied to the current work context established by `seal enter <key>`. Passing the key as an ordinary argument makes it easier for scripts or agents to accidentally modify seal ownership under the wrong key while still operating in another context.

For mutation commands, requiring a current key makes the active context explicit at the process level and reduces accidental cross-context operations.

### Let `seal test` fall back to project-wide deny validation without a current key

One possible design was:

```sh
git kura seal test <file...>
```

with no current key, the command would succeed only if all files were unsealed by any key.

This was rejected for v0 because it makes `seal test` ambiguous. Sometimes it would mean "can these files be handled in the current context?" and sometimes it would mean "are these files globally unsealed?" depending on whether `GIT_KURA_SEAL_KEY` is set.

For v0, `seal test` is defined as a context-validation command. It therefore requires a valid current key. Project-wide validation may be introduced later under an explicit option, but it is not part of v0.

### Make `doctor` support key-scoped checks in v0

This was rejected for v0.

`seal doctor` is an integrity check for repository-wide seal state. Some problems, such as malformed store files, invalid schema versions, normalized path duplication, or repository-wide consistency issues, are not naturally scoped to one key. A key-scoped `doctor` could hide global corruption and give a false impression of safety.

For v0, `doctor` remains repository-wide and read-only.

## Consequences

### Positive Consequences

- Mutation commands cannot silently fall back to another scope when there is no current key.
- `seal test` has a single v0 meaning: validate files against the current seal context.
- Inspection commands produce deterministic default output independent of `GIT_KURA_SEAL_KEY`.
- `seal ls` and similar commands remain useful for humans, scripts, and agents that need a repository-wide view.
- The project has a clear rule for future commands: classify them by effect and semantic context dependency, not merely by whether they are read-only.
- Repository-level maintenance can use safety mechanisms such as dry-run and `--apply` instead of misusing the current seal key as a generic mutation guard.

### Negative Consequences

- Users inside `git kura seal enter <key>` may initially expect `seal ls` to show only the current key. Documentation and help text must make the repository-wide default explicit.
- Users who want current-key inspection must specify the key explicitly, for example by running `git kura seal ls <key>`.
- `seal test` cannot be used in v0 as a project-wide "is this globally unsealed?" check without entering a seal context.
- The command set is intentionally asymmetric, so the rationale must be documented to avoid appearing inconsistent.

### Neutral Consequences

- `seal ls` and `seal test` intentionally follow different context rules even though both are read-only.
- `session clean` is current-independent even though it can mutate repository-level records.
- Future project-wide validation modes should be introduced through explicit options or separate commands, not by changing the default behavior of `seal test`.
- The expected scale is local multi-agent or multi-process usage on a developer machine, not high-traffic remote service usage. Repository-wide inspection is therefore acceptable for v0.
