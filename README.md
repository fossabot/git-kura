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

## Quick Start

```sh
curl -fsSL https://raw.githubusercontent.com/tooppoo/git-kura/main/install.sh | sh
```

Installs `git-kura` into `~/.local/bin`. See [docs/installation.md](docs/installation.md) for version pinning, custom install directories, checksum verification, and other installation methods.

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

### curl installer (Linux and macOS)

```sh
curl -fsSL https://raw.githubusercontent.com/tooppoo/git-kura/main/install.sh | sh
```

See [docs/installation.md](docs/installation.md) for options such as `--version`, `--install-dir`, and `--require-signature`.

### Download from GitHub Releases

Download the archive for your OS and CPU architecture from [GitHub Releases](https://github.com/tooppoo/git-kura/releases).

Available release archives:

```txt
git-kura_Linux_x86_64.tar.gz
git-kura_Linux_arm64.tar.gz
git-kura_Darwin_x86_64.tar.gz
git-kura_Darwin_arm64.tar.gz
git-kura_Windows_x86_64.zip
git-kura_Windows_arm64.zip
checksums.txt
```

Current pre-release example:

```sh
VERSION=v0.0.0-alpha
```

#### Linux / macOS

Set `OS` to `Linux` or `Darwin`, and `ARCH` to `x86_64` or `arm64`.

```sh
VERSION=v0.0.0-alpha
OS=Linux
ARCH=x86_64
ARCHIVE="git-kura_${OS}_${ARCH}.tar.gz"

curl -fLO "https://github.com/tooppoo/git-kura/releases/download/${VERSION}/${ARCHIVE}"
curl -fLO "https://github.com/tooppoo/git-kura/releases/download/${VERSION}/checksums.txt"

grep " ${ARCHIVE}$" checksums.txt | sha256sum -c -

tar -xzf "${ARCHIVE}"
chmod +x git-kura
sudo mv git-kura /usr/local/bin/git-kura

git kura -h
```

On macOS, use `shasum` if `sha256sum` is unavailable:

```sh
grep " ${ARCHIVE}$" checksums.txt | shasum -a 256 -c -
```

#### Windows

Set `ARCH` to `x86_64` or `arm64`.

```powershell
$Version = "v0.0.0-alpha"
$Arch = "x86_64"
$Archive = "git-kura_Windows_$Arch.zip"

Invoke-WebRequest "https://github.com/tooppoo/git-kura/releases/download/$Version/$Archive" -OutFile $Archive
Invoke-WebRequest "https://github.com/tooppoo/git-kura/releases/download/$Version/checksums.txt" -OutFile checksums.txt

$Expected = ((Select-String -Path checksums.txt -Pattern " $Archive$").Line -split '\s+')[0].ToLower()
$Actual = (Get-FileHash $Archive -Algorithm SHA256).Hash.ToLower()
if ($Actual -ne $Expected) {
  throw "checksum mismatch: expected $Expected, got $Actual"
}

Expand-Archive $Archive -DestinationPath . -Force
$InstallDir = Join-Path $HOME "bin"
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item .\git-kura.exe (Join-Path $InstallDir "git-kura.exe")

# Add $InstallDir to PATH if it is not already there.
$env:Path = "$InstallDir;$env:Path"
git kura -h
```

Git runs `git-kura` as the external subcommand `git kura` when `git-kura` is available on `PATH`.

Use `git kura -h` for top-level help. `git kura --help` may be handled by Git's manual-page help mechanism instead of being passed directly to `git-kura`.

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

* [Installation](docs/installation.md)
* [Commands](docs/commands.md)
* [Output formats](docs/output-format.md)
* [State management](docs/state-management.md)
* [Design](docs/design.md)
* [Architecture decision records](docs/adr/)

## Status

Kura is currently under design.

The command names, output format, and safety behavior may change before the first stable release.

## License

Apache License 2.0
