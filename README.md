# mapps

`mapps` is a single Go binary that turns a list of git repositories into a working multi-repo workspace: it clones each repo under `apps/`, generates a root `Makefile` with per-repo and aggregate targets, and writes thin LLM wrapper prompts that build a project map for each app (a `/init` analog) in Claude Code, opencode, or any other coding tool.

The binary does all deterministic work (parsing, cloning, Makefile generation). Building project maps is done by an LLM, driven by the wrapper prompts `mapps` writes into the workspace.

## Install

Linux / macOS (and WSL):

```
curl -fsSL https://raw.githubusercontent.com/rus-lan/multiApps/main/install.sh | sh
```

Windows (PowerShell):

```
irm https://raw.githubusercontent.com/rus-lan/multiApps/main/install.ps1 | iex
```

Both scripts detect OS/arch, download the matching release binary, verify its `checksums.txt` entry before installing, and need no sudo/admin rights. Unix installs to `~/.local/bin`; Windows installs to `%LOCALAPPDATA%\Programs\mapps`. If the install directory isn't already on `PATH`, the script prints the exact line to add to your shell rc (`~/.bashrc`, `~/.zshrc`, `~/.profile`) — on Windows it updates your user `PATH` automatically and asks you to open a new terminal.

Env overrides (set before running the piped command):

- `MAPPS_VERSION` — pin a release instead of installing latest, e.g. `MAPPS_VERSION=v0.1.0 curl -fsSL .../install.sh | sh`.
- `MAPPS_INSTALL_DIR` — unix only, install somewhere other than `~/.local/bin`, e.g. `MAPPS_INSTALL_DIR=/usr/local/bin curl -fsSL .../install.sh | sh`.

Verify the install:

```
mapps version
```

### Alternative — install with Go

```
go install github.com/rus-lan/multiApps/cmd/mapps@latest
```

Requires a Go toolchain on `PATH`.

`mapps` itself runs natively on Linux, macOS, and Windows, has zero third-party dependencies, and shells out to `git` for all repository work — authentication (SSH keys, credential helpers) is entirely `git`'s responsibility; `mapps` never stores or reads tokens. The Makefile it generates is a different story: its targets need GNU `make` and a POSIX shell, so on Windows run the `make ...` workflow from WSL or Git Bash, not plain PowerShell/cmd.

### Uninstall

Unix:

```
rm ~/.local/bin/mapps
```

(or the directory you chose via `MAPPS_INSTALL_DIR`).

Windows (PowerShell):

```
Remove-Item "$env:LOCALAPPDATA\Programs\mapps\mapps.exe"
```

Then optionally remove `%LOCALAPPDATA%\Programs\mapps` from your user `PATH` in the environment-variables settings.

## Quickstart (terminal only)

```
mkdir work && cd work
mapps init git@github.com:you/api.git git@github.com:you/web.git
make list
make build-all
```

This creates `repos.list`, clones both repos into `apps/api` and `apps/web`, generates the root `Makefile`, writes the wrapper prompt files, creates `.gitignore`, and turns the workspace itself into a git repo. No manual file editing is required at any point.

## `repos.list` format

Plain text file in the workspace root, one repo per line:

```
<url> [dir] [branch]
```

- `url` — an ssh or https git URL.
- `dir` (optional) — folder name under `apps/`. Defaults to the repo's basename (the URL's last path segment with a trailing `.git` trimmed).
- `branch` (optional) — branch to check out. Defaults to the remote's default branch.

Full-line `#` comments and blank lines are allowed. Example:

```
# mapps repos list
# format: <url> [dir] [branch]
git@github.com:user/api.git
https://github.com/user/web.git my-web main
```

The file is created automatically (with the header above) on the first `mapps init`. Edit it by hand, or use `mapps add` to append a line and clone it in one step.

## Commands

### `mapps init [<url>...]`

Idempotent — safe to run again at any time:

1. Creates `repos.list` if it does not exist yet.
2. Appends any URLs passed as arguments (skipped silently if already listed).
3. Fails before touching anything if two repos would land in the same `apps/<dir>` (see Troubleshooting).
4. Clones every repo not yet cloned into `apps/`. Full clones, never shallow. Already-cloned repos are **not** pulled — `init` never updates existing clones, only adds missing ones.
5. Skips repos whose remote has no commits yet: prints `repo empty, skipped: <url>`, keeps the line in `repos.list` for a later `init`, and generates no Makefile targets for them. A leftover clone without commits is deleted first (`removed empty clone: apps/<dir>`). Empty repos are not failures — the summary counts them separately: `cloned N, skipped M, empty E, failed K`.
6. Adds `CLAUDE.md` and `AGENTS.md` to each clone's `.git/info/exclude`, so those files never reach the app's own remote.
7. Regenerates the root `Makefile` and creates `mk/` if missing.
8. Writes the three wrapper prompt files (see below).
9. Creates `results/` with an empty `.gitkeep` — the home for LLM-generated artifacts, tracked in the workspace repo.
10. Creates or updates `.gitignore` to ignore `apps/`.
11. Runs `git init` in the workspace root if it is not a git repo yet.

### `mapps add <url> [dir] [branch]`

Appends one repo to `repos.list`, clones it, and regenerates the Makefile. Positional arguments only, no flags. Fails if the URL is already listed, or if the derived/given `dir` collides with an existing entry.

### `mapps rm <name> [--force]`

Removes one repo from the workspace. `<name>` is the folder name under `apps/` (the `dir` of its `repos.list` entry), not a URL. It removes the repo's line from `repos.list` (comments and other lines are preserved), deletes `apps/<name>` and `mk/<name>.mk` if present, and regenerates the Makefile.

Before deleting anything, `rm` refuses — and changes nothing — when the clone has uncommitted changes or unpushed commits (a branch with commits but no upstream counts as unpushed: nothing was ever pushed anywhere). `--force` skips both checks and deletes anyway.

## Make targets

Per repo (`<dir>` = folder name under `apps/`):

| Target | What it does |
|---|---|
| `build-<dir>` | Runs the detected build command from inside `apps/<dir>` |
| `run-<dir>` | Runs the detected run command (foreground) |
| `test-<dir>` | Runs the detected test command |
| `pull-<dir>` | `git -C apps/<dir> pull` |
| `commit-<dir>` | `git -C apps/<dir> add -A && git -C apps/<dir> commit -m "$(MSG)"` |
| `push-<dir>` | `git -C apps/<dir> push` |

Aggregates:

| Target | What it does |
|---|---|
| `build-all` | Runs `build-<dir>` for every repo |
| `pull-all` | Runs `pull-<dir>` for every repo |
| `status` | Short `git status` and current branch for every repo |
| `list` | Dir, branch, and detected stack for every repo |

`commit-<dir>` needs an `MSG`:

```
make commit-api MSG="fix login bug"
```

Running it without `MSG` prints a usage message and stops instead of committing with an empty message.

There is deliberately no `run-all` (foreground process orchestration across many apps is out of scope), no `commit-all`, and no `push-all` (mass commit/push to many remotes at once is too easy to get wrong).

### Stack detection

For each `apps/<dir>`, the first matching marker wins:

| Marker | Stack | build | run | test |
|---|---|---|---|---|
| `go.mod` | go | `go build ./...` | `go run .` | `go test ./...` |
| `package.json` | node | `<pm> run build` | `<pm> run start`/`dev` | `<pm> run test` |
| `Cargo.toml` | rust | `cargo build` | `cargo run` | `cargo test` |
| own `Makefile`/`makefile`/`GNUmakefile` | make | `$(MAKE) build` | `$(MAKE) run` | `$(MAKE) test` |
| none of the above | unknown | placeholder | placeholder | placeholder |

For `node`, the package manager is chosen by lockfile (`pnpm-lock.yaml` → pnpm, `yarn.lock` → yarn, `package-lock.json` or none → npm), and each command falls back to a placeholder when the matching `package.json` script is missing.

### `mk/<dir>.mk` overrides

The root `Makefile` is fully generated and owned by the tool — it is rewritten on every `init`/`add` and should never be hand-edited. Detection gets things wrong sometimes (a `go.mod` in a subfolder, a custom test runner, and so on); the escape hatch is a file per repo under `mk/`, never touched by regeneration:

```make
# mk/api.mk
BUILD_api = go build ./cmd/api
RUN_api = go run ./cmd/api

deploy-api:
	cd apps/api && ./deploy.sh
```

Because `mk/*.mk` is included at the very end of the Makefile with `-include`, a plain variable assignment like `BUILD_api = ...` overrides the generated default with no "overriding recipe" warning (`BUILD_<var>`/`RUN_<var>`/`TEST_<var>` are recursively-expanded make variables, so the last assignment before a recipe runs wins). `mk/*.mk` files can also define entirely new targets, like `deploy-api` above. In the variable name, every character that is not a letter, digit, or `_` is replaced with `_` — so `apps/my-web` uses `BUILD_my_web`.

## Project maps with an LLM

Building a project map for each app is LLM work, so it lives in a prompt, not in the binary. `mapps init` writes the same prompt in three forms:

- **Claude Code** — `.claude/skills/mapps/SKILL.md`. The directory name (`mapps`) is the slash command, so run `/mapps` (optionally `/mapps <url>...` to pass URLs straight to `init`).
- **opencode** — `.opencode/commands/mapps.md`, also invoked as `/mapps`.
- **Any other tool** — `PROMPT-map.md` in the workspace root. Paste its contents into the tool.

The prompt: runs `mapps init` (so the flow works as a single command even in a brand-new workspace), then finds every `apps/<dir>` missing a `CLAUDE.md` and writes one, with this fixed six-section structure:

1. Purpose
2. Stack and entry points
3. Key directories
4. How to build / run / test
5. External dependencies
6. Notable gotchas (only if any were found)

It then copies `CLAUDE.md` to `AGENTS.md` so both exist and are identical, and updates the workspace root `CLAUDE.md` index (one line per app: dir, stack, purpose).

**Why two identical files:** Claude Code reads `CLAUDE.md`; opencode reads `AGENTS.md` as primary and falls back to `CLAUDE.md`. Writing both keeps every tool working from the same map without extra logic.

Maps stay invisible to each app's own remote because `mapps` lists `CLAUDE.md` and `AGENTS.md` in that clone's `.git/info/exclude` at clone time. Maps are a regenerable artifact — they are not committed anywhere, and a fresh `mapps init` plus one run of the wrapper rebuilds any missing ones.

## The `results/` directory

`mapps init` creates `results/` (with an empty `.gitkeep`). Everything an LLM generates from the workspace — a new project scaffolded from the apps, a security-audit report over all apps, comparison docs — goes in there, never into `apps/` or the workspace root; the wrapper prompts instruct the LLM accordingly. Unlike `apps/`, `results/` is tracked in the workspace's git repo, so generated artifacts survive a fresh clone.

## Workspace as a git repo

`mapps init` turns the workspace itself into a git repo (unless it already is one). Tracked:

- `repos.list`
- `Makefile`
- `mk/`
- root `CLAUDE.md`
- `.claude/`
- `.opencode/`
- `PROMPT-map.md`
- `results/`
- `.gitignore`

Ignored: `apps/` — the live nested clones must never enter the workspace's own git history.

**Restoring on a new machine:** clone the workspace repo, run `mapps init` (no URLs needed — they are already in `repos.list`), then run the wrapper prompt once to rebuild any project maps.

## Troubleshooting

- **Clone failed (auth, typo, network):** `mapps` delegates authentication entirely to system `git` — check your SSH keys or credential helper the same way you would for a plain `git clone`. Other repos still get cloned; the failing one stays in `repos.list` and is retried on the next `init`.
- **`dir collision: "..." wanted by line N (...) and line M (...)`:** two repos would derive (or were given) the same folder name under `apps/`. Nothing is cloned when this happens. Fix it by adding an explicit `dir` for one of the two lines in `repos.list`.
- **`repos.list:N: ...` syntax errors:** the line number points at the exact bad line; nothing runs until it is fixed.
- **`apps/<dir> exists but is not a git clone`:** something already occupies that folder and it is not a git repository `mapps` can manage. Remove it (or pick a different `dir`) and re-run `init`.
- **`repo empty, skipped: <url>`:** the remote exists but has no commits yet. Nothing is cloned and the line stays in `repos.list`; the next `init` picks the repo up once something is pushed to it.
- **`apps/<name> has uncommitted changes / unpushed commits`:** `mapps rm` protects local work. Commit and push (or discard) it, or re-run with `--force` to delete anyway.
- **Re-running `init` is always safe.** It never re-clones an existing repo, never auto-pulls, and never duplicates lines in `repos.list` or `.git/info/exclude`.
