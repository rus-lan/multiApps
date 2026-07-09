# mapps — Specification

CLI tool for building and managing a multi-repo workspace, designed to work with LLM coding tools (Claude Code, opencode, and others).

## Concept

A single Go binary (`mapps`) does all deterministic work: reads a list of git repositories, clones them into `apps/`, and generates a root Makefile. The LLM-dependent part — building a project map for each repo (analog of `/init`) — is driven by thin wrapper prompts that the binary itself generates for each supported tool.

## Architecture

- **Core:** Go CLI, single static binary. No LLM calls inside the binary.
- **LLM wrappers:** prompt files embedded in the binary via `go:embed`, written into the workspace by `mapps init`:
  - `.claude/skills/mapps/SKILL.md` — Claude Code skill
  - opencode command file (exact path/format to confirm during implementation)
  - `PROMPT-map.md` — generic prompt text for any other tool
- The wrapper is self-sufficient: it runs `mapps init` itself if needed, so the full flow works as one command inside the LLM tool (e.g. `/mapps <url>...`).

## Input: `repos.list`

Plain text file in the workspace root. One repository per line:

```
<url> [dir] [branch]
# comments allowed
git@github.com:user/api.git
https://github.com/user/web.git my-web main
```

- `url` — ssh or https git URL.
- `dir` — optional folder name under `apps/` (default: repo basename).
- `branch` — optional branch to check out (default: remote default branch).

The file is created automatically on first `mapps init` — a terminal-only workflow with no manual file editing must be possible.

## Commands

### `mapps init [<url>...]`

Idempotent. On every run:

1. Creates `repos.list` if missing; appends any URLs passed as arguments.
2. Creates `apps/`.
3. Clones every repo from `repos.list` that is not yet cloned (**full clones**, not shallow — repos are committed to and pushed). Already-cloned repos are skipped (no auto-pull).
4. Adds `CLAUDE.md` and `AGENTS.md` to each clone's `.git/info/exclude`.
5. Generates the root `Makefile` and `mk/` include dir.
6. Writes the LLM wrapper files (skill, opencode command, generic prompt).
7. Creates `.gitignore` with `apps/`; runs `git init` in the workspace root if it is not a git repo yet.

### `mapps add <url> [dir] [branch]`

Appends the repo to `repos.list`, clones it, regenerates the Makefile. Positional arguments, no flag-string parsing.

## Makefile generation

- The root `Makefile` is **fully generated and owned by the tool** — never hand-edited. Regeneration on `add` is always safe.
- Custom/override targets live in `mk/<dir>.mk` files, included by the root Makefile and **never touched** by regeneration.
- Build/run/test commands come from deterministic stack detection:
  - `go.mod` → `go build ./...` / `go run .` / `go test ./...`
  - `package.json` → scripts from it (npm/pnpm/yarn by lockfile)
  - `Cargo.toml` → `cargo build` / `cargo run` / `cargo test`
  - repo has its own `Makefile` → delegate to it
  - unknown stack → placeholder target that prints "define in mk/<dir>.mk"
- Git targets are stack-independent and always generated.

### Target set

Per repo (`<dir>` = folder name under `apps/`):

```
build-<dir>
run-<dir>              # foreground
test-<dir>
pull-<dir>
commit-<dir>           # git add -A && git commit -m "$(MSG)"
push-<dir>
```

Aggregates:

```
build-all
pull-all
status                 # short git status + current branch for every repo
list                   # dir, branch, detected stack
```

Deliberately **not** generated: `run-all` (foreground process management is out of scope), `commit-all`/`push-all` (mass commit/push to many remotes is too dangerous).

## Project maps (the `/init` analog)

- Built by the LLM wrapper, not the binary. The wrapper walks `apps/*`, and for every repo **missing `CLAUDE.md`** explores the code (parallel subagents allowed) and writes the map. This makes map building idempotent and incremental after `add`.
- Two identical files per repo: `apps/<dir>/CLAUDE.md` and `apps/<dir>/AGENTS.md` (Claude Code reads the first, opencode the second; confirm exact reader matrix during implementation).
- Files stay invisible to the repo's remote thanks to `.git/info/exclude` (set by the CLI at clone time).
- A root workspace `CLAUDE.md` holds the index: app list, stack, one line per app. The wrapper updates it.
- Maps are written in **English**, fixed structure:
  1. Purpose (1–2 sentences)
  2. Stack and entry points
  3. Key directories
  4. How to build / run / test (consistent with Makefile targets)
  5. External dependencies: DBs, queues, env vars, configs
  6. Notable gotchas, if found
- If detection produced wrong build/run commands, the wrapper may write corrections into `mk/<dir>.mk`.
- Maps are a regenerable artifact: they are not tracked anywhere. On a fresh machine, after `mapps init`, the wrapper rebuilds missing maps.

## Workspace as a git repo

Tracked: `repos.list`, `Makefile`, `mk/`, root `CLAUDE.md`, `.claude/`, `PROMPT-map.md`, `.gitignore`.
Ignored: `apps/` (nested live clones must never enter workspace git).
Result: cloning the workspace repo on a new machine + `mapps init` restores everything except maps.

## Error handling

| Situation | Behavior |
|---|---|
| A repo fails to clone (access, typo, network) | Continue with the rest; print summary `N ok, M failed: <list with reasons>`; non-zero exit. Failed entries stay in `repos.list` — next `init` retries. |
| Folder name collision (two repos with the same basename) | Fail immediately before any cloning; suggest setting an explicit `dir`. No auto-suffixing. |
| Syntax error in `repos.list` | Fail fast with the line number; do nothing. |

Authentication is fully delegated to system `git` (ssh keys, credential helpers). The tool stores no tokens.

## Naming & distribution

- CLI name: `mapps`. Skill trigger: `/mapps`.
- Source lives in this repository.
- Install: `go install github.com/<user>/mapps@latest`. Binary releases (goreleaser) — post-v1.

## Out of scope (v1)

- `run-all` / process orchestration / docker-compose
- Shallow clones
- Auth configuration of any kind
- Auto-pull of existing clones on `init`
- Binary releases via goreleaser
- Mirroring maps into a tracked folder
