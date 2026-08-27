# APCode

![CI](https://github.com/anshulchikhale30-p/APCode/actions/workflows/ci.yml/badge.svg)
![Release](https://github.com/anshulchikhale30-p/APCode/actions/workflows/release.yml/badge.svg)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)
![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey)

**Offline-first AI coding agent for the terminal — everything runs on your laptop.**

> We care about your system. 😄  
> So you can focus on your ideas. 💡  
> Making the most of every bit of your laptop. ⚡

APCode is an open source AI coding agent you install in your terminal. It understands your hardware, recommends the right local model, and runs 100% offline — no cloud APIs, no data leaving your machine.

> **Current release: `v0.1.5`.** Precompiled binaries are published for Windows (amd64/arm64), Linux (amd64/arm64), and macOS Intel/Apple Silicon — no Go toolchain needed. The `install.sh`/`install.ps1` scripts download them automatically; `--binary` works with a local build too. `brew`/`scoop`/`npm`/`winget`/`docker`/`chocolatey` are **future/planned** and not yet published. See `CONTRIBUTING.md` and `docs/release-test.md`.

---

## Install

The easiest way to install APCode is through the install script — just like `apcode`.

### Quick install (recommended)

**macOS & Linux:**

```sh
curl -fsSL https://raw.githubusercontent.com/anshulchikhale30-p/APCode/main/install.sh | bash
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/anshulchikhale30-p/APCode/main/install.sh | bash -s -- --version 0.1.5
```

Install to a custom directory:

```sh
curl -fsSL https://raw.githubusercontent.com/anshulchikhale30-p/APCode/main/install.sh | bash -s -- --dir /usr/local/bin
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/anshulchikhale30-p/APCode/main/install.ps1 | iex
```

Specific version:

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/anshulchikhale30-p/APCode/main/install.ps1))) -Version 0.1.5
# or if you saved it locally:
.\install.ps1 -Version 0.1.5
```

Local binary (no download):

```sh
# Unix
./install.sh --binary ./apcode --dir ~/.apcode/bin
# Windows
.\install.ps1 -Binary .\apcode.exe -InstallDir $HOME\.apcode\bin
```

The installer adds `~/.apcode/bin` to your PATH automatically (skippable with `--no-modify-path`).

Restart your shell or run:

```sh
export PATH="$HOME/.apcode/bin:$PATH"   # bash/zsh
# fish: fish_add_path $HOME/.apcode/bin
```

Verify:

```sh
apcode --version   # APCode 0.1.5
apcode version     # same
apcode --help
apcode             # branded TUI + hardware
```

---

### Other install methods

**Using Go (from source):**

```sh
# From cloned repo (recommended, no registry required)
git clone https://github.com/anshulchikhale30-p/APCode
cd APCode
go build -o apcode ./cmd/apcode
# or install to GOPATH/GOBIN (adds to $(go env GOPATH)/bin)
go install ./cmd/apcode
# then ensure GOPATH/bin is in PATH:
# export PATH="$(go env GOPATH)/bin:$PATH"
```

**Using npm (like `apcode-ai`) — not yet published:**

> `apcode-ai` is prepared in `npm/` but has **not** been published to the npm registry. After `npm publish` in `npm/`, users will be able to:

```sh
npm install -g apcode-ai      # wraps the Go binary, downloads correct platform binary
# bun / pnpm / yarn
bun add -g apcode-ai
pnpm add -g apcode-ai
yarn global add apcode-ai
```

> The package downloads the matching release binary on `postinstall`. No Node runtime needed after install. For now, use the `install.sh`/`install.ps1` scripts.

**Using Homebrew (macOS & Linux) — not yet published / future:**

> Formula `homebrew/apcode.rb` is a template. After a real `v0.1.5` release, a Homebrew tap could be published to `anshulchikhale30-p/homebrew-tap` (not configured for first release). Until then:

```sh
# Build from source locally:
brew install --build-from-source ./homebrew/apcode.rb
# After tap is published (future):
brew install anshulchikhale30-p/tap/apcode
```

> The tap would be updated on every release via GoReleaser if configured — currently not configured for first release.

**Using Scoop (Windows) — not yet published:**

> Manifest `scoop/apcode.json` is a template with `REPLACE_WITH_SHA256_*`. Not yet in Scoop Main. Test locally:

```powershell
# Local test:
scoop install ./scoop/apcode.json
# After bucket is published:
scoop bucket add apcode https://github.com/anshulchikhale30-p/scoop-bucket
scoop install apcode
```

**Using WinGet (Windows) — not yet published:**

> WinGet manifests are prepared in `packaging/winget/` but **not** yet submitted to `microsoft/winget-pkgs`. See `docs/packaging/winget.md` for the exact submission procedure. After acceptance:

```powershell
winget install APCode.APCode
```

> For now, use `install.ps1`.

**Using Chocolatey (Windows) — not yet published / future:**

> Chocolatey package `apcode` is **not yet published** to `chocolatey.org`. After submission, users will be able to:

```powershell
choco install apcode
```

> For now, use `install.ps1`.

**Manual binary download:**

Grab the binary for your platform from [Releases](https://github.com/anshulchikhale30-p/APCode/releases):

| Platform | File |
|---|---|
| Linux amd64 | `apcode_0.1.5_linux_amd64.tar.gz` |
| Linux arm64 | `apcode_0.1.5_linux_arm64.tar.gz` |
| macOS Intel | `apcode_0.1.5_darwin_amd64.tar.gz` |
| macOS Apple Silicon | `apcode_0.1.5_darwin_arm64.tar.gz` |
| Windows amd64 | `apcode_0.1.5_windows_amd64.zip` |
| Windows arm64 | `apcode_0.1.5_windows_arm64.zip` |

Then:

```sh
tar -xzf apcode_*_linux_amd64.tar.gz
sudo mv apcode /usr/local/bin/
apcode --version
```

**Using Docker — planned / not yet published:**

> Docker image `ghcr.io/anshulchikhale30-p/APCode` is **not yet published**. After a real `v0.1.5` GitHub release and `docker build`/`push`, users will be able to:

```sh
docker run --rm -it ghcr.io/anshulchikhale30-p/APCode --help
docker run --rm -it -v $(pwd):/work ghcr.io/anshulchikhale30-p/APCode recommend
```

---

## Quick Start

```sh
# 0. Initialize project (creates .apcode/ + ~/.apcode/config.json)
apcode init                          # in current dir
apcode init --dir ./myproject        # target dir
apcode init --dir . --force          # overwrite existing config

# 1. Run the coding agent — headline feature (plan → act → observe, 16 tools, approval gates, rollback)
apcode run "Add authentication to my Go API"
apcode run "Fix failing tests" --model phi-3-mini-q4 --stream --max-iterations 10 --dir ./myproject --no-color
# Full usage: apcode run <instruction> [--model <id>] [--stream] [--max-iterations N] [--dir <path>] [--no-color]

# 2. Enter interactive mode (like OpenCode)
apcode                          # starts REPL: banner + project + git + runtime + model

# Or check your system non-interactively
apcode --help
apcode --version

# 3. Benchmark your hardware (optional but improves recommendations)
apcode benchmark

# 4. See available models
apcode models
apcode models installed
apcode models info phi-3-mini-q4
apcode models install phi-3-mini-q4  # local stub for offline testing (no remote download yet)
# aliases: apcode models pull <id>  |  apcode models download <id>

# 5. Get hardware-aware recommendation
apcode recommend
apcode recommend --benchmark --preference balanced
apcode recommend --capability code_generation --preference quality

# 6. Inspect project context (offline, no LLM)
apcode context                  # auto-detect root
apcode context --budget 8000 --root ./myproject
apcode search "func main" --dir ./cmd --limit 20

# 7. Check runtime status
apcode runtime

# 8. Run inference (when runtime + model installed)
apcode infer "write a hello world in Go"
apcode infer --stream --model phi-3-mini-q4 "explain this repo"
```

### Interactive REPL

```sh
apcode
# ╭──────────────────────────────────────────────────────────╮
# │                       APCode                             │
# │             Offline AI Coding Agent                     │
# ╰──────────────────────────────────────────────────────────╯
# ✓ Project detected (Go, 88 files)
# ✓ Git repository detected (main)
# ✓ Runtime ready (native)
# ✓ Local model: Qwen2.5-Coder 7B
# Type /help for commands.
# You > explain this project
# APCode > I'll inspect the project structure first...
# You > /help
```

**Slash commands (19, verbatim from `internal/cli/repl.go`):**

```
   /help (/h, /?)        Show this help
   /new (/session)        New session (clears conversation)
   /models                List available models
   /model                 Show the currently selected model
   /runtime (/rt)         Show runtime status
   /status (/st)          Show project and system status
   /benchmark (/bench)    Run hardware benchmark
   /context (/ctx)        Show project context summary
   /files [dir]           List files via the agent's file tool
   /search <query>        Search files in the workspace
   /plan                  Show the plan from the current/last task
   /compact               Compact conversation history
   /permissions           Show the tool permission policy
   /tools                 List every registered agent tool + schema
   /git                   Show git diff + status
   /diff                  Show git diff
   /rollback              Revert the last APCode change set
   /clear (/cls)          Clear screen and redraw welcome
   /exit (/quit, /q)      Exit APCode
```

`Ctrl+C` is graceful (cancels current operation, not crash). Conversation history is kept in memory, so follow-ups understand prior context.

---

## Vision

Instead of requiring a powerful cloud or GPU cluster, APCode understands your hardware and adapts its AI workload to it:

- **Powerful laptop** → stronger model (e.g., 13B)
- **Low-resource laptop** → efficient small model (e.g., 2B–3B)

Everything runs locally. No cloud APIs, no data leaving your machine.

---

## AI Coding Agent

APCode is evolving from a local-LLM REPL into a real terminal coding agent: an iterative loop where the model inspects the project, plans, edits via tools with your approval, and validates its own changes. All offline.

| Capability | Status |
|---|---|
| Agent loop (plan → act → observe → repeat), iteration caps | **IMPLEMENTED** |
| Tool system with JSON schemas + structured errors (16 tools) | **IMPLEMENTED** |
| `read_file` `list_files` `search_files` `write_file` `edit_file` | **IMPLEMENTED** |
| `create_file` (no-clobber) · `delete_file` (approval-gated) · `apply_patch` (unified diff w/ context validation) | **IMPLEMENTED** |
| `project_info` · `run_tests` / `run_build` / `run_lint` (stack auto-detect: Go/Node/Python/Rust) | **IMPLEMENTED** |
| Path security: workspace jail, traversal rejection, symlink escape protection | **IMPLEMENTED** |
| Command classifier: SAFE (auto) / REQUIRES_APPROVAL / BLOCKED | **IMPLEMENTED** |
| Write/delete approval prompts; safe reads automatic | **IMPLEMENTED** |
| Rollback journal (`/rollback` restores last change set) | **IMPLEMENTED** |
| Git awareness (`git_status`, `git_diff`, never auto-commits) | **IMPLEMENTED** |
| Validation after changes + bounded repair loop | **IMPLEMENTED** (repair capped at 2 attempts) |
| REPL slash commands (19, see Interactive REPL below) | **IMPLEMENTED** |
| Context compaction (`/compact`, keeps task anchor + recent turns) | **IMPLEMENTED** (truncate-style summary) |
| Model provider abstraction (`internal/provider`) over native Gemma / llama.cpp / Ollama / mock | **IMPLEMENTED** |
| Structured model output parsing (JSON object/array/fenced/tool-marker formats) | **IMPLEMENTED** |
| Streaming into TUI | **PARTIALLY IMPLEMENTED** (streaming supported in runtime/provider & `apcode run --stream`; REPL still uses non-streaming generate) |
| Automatic plan revision mid-task | **PLANNED** |
| Diff preview of *proposed multi-file* change sets before any apply | **PLANNED** (single-edit previews work today) |
| Prompt-injection hardening for untrusted file content | **PARTIALLY IMPLEMENTED** (repo files are data-only in prompts; no sanitizer layer yet) |
| Secrets redaction in tool output | **PLANNED** |

### Security policy

- The agent can only touch files inside the project root. Absolute paths, `..` traversal, and symlink escapes are rejected.
- Terminal commands are classified: read-only/validation commands run automatically; anything else requires `[y/N]`; system-destructive commands are refused even with approval.
- Deletes always require approval. APCode never runs `git commit` or `git push`.
- Every write/edit/delete is journaled; `/rollback` reverts the most recent APCode change set exactly.

---

## Current Capabilities

- `apcode --version` / `apcode version` — print version (both work, single source `internal/config.Version`)
- `apcode --help` — print usage
- `apcode` — **interactive REPL** (banner + project + git + runtime + model, 19 slash commands, history, tool loop) — offline, no cloud
- `apcode init [--dir <path>] [--force]` — initialize `.apcode` project config + `~/.apcode/config.json` (see `runInit` in `cmd/apcode/main.go`)
- `apcode run "<instruction>" [--model <id>] [--stream] [--max-iterations N] [--dir <path>] [--no-color]` — **flagship coding agent** (plan → act → observe loop, 16 tools, approval gates, rollback) e.g. `apcode run "Add authentication to my Go API"`
- `apcode benchmark` — hardware benchmarks (CPU ops/sec, memory bandwidth, optional storage)
- `apcode models` — list catalog; `models installed` / `models info <id>` / `models install|pull|download <id>` (install creates local 1 MiB stub for offline testing — no remote GGUF download yet; honest placeholder)
- `apcode recommend` — hardware-aware ranking with explanations & uncertainty:
  - `apcode recommend --capability code_generation --preference balanced`
  - `apcode recommend --benchmark` — run benchmark first
  - `apcode recommend --no-color` — disable color
- `apcode context` — local project context gathering (files, languages, tokens) with budget & gitignore
- `apcode search <query>` — offline code search (symbol + text) with `--dir`, `--limit`, `--kind`
- `apcode runtime` — runtime availability check
- `apcode infer <prompt>` — local inference via detected runtime (native/llama.cpp/ollama + mock) with `--model`, `--stream`, `--max-tokens`

---

## Project Structure

```
cmd/apcode/        CLI entry point (welcome, benchmark, models, recommend, context, search, runtime, infer, init, run, REPL)
internal/
    cli/           interactive REPL, 19 slash commands, ProjectContext, conversation history, rollback journal
    agent/         agent loop (plan → act → verify) — invoked via `apcode run`
    provider/      model provider abstraction over runtimes
    benchmark/     real hardware measurement (CPU/memory/storage)
    codeintel/     symbol extraction, search, imports
    config/        version and app constants (ldflags-overridable, current 0.1.5)
    context/       project-context gathering (walk, ignore, metadata)
    git/           git integration
    hardware/      system detection (OS/arch, CPU, RAM, GPU)
    localmodel/    local model file management
    model/         model metadata, registry, BuiltInCatalog
    recommendation/hardware-aware ranking engine
    runtime/       inference runtimes (native, llama.cpp, ollama, mock)
    tools/         agent tools (16 tools: edit, filesystem, search, git, validation)
    tui/           terminal rendering (welcome, benchmark, recommendation, context, search)
    verification/  output verification
models/            reserved for local model artifacts
tests/             cross-package tests
docs/              design notes and roadmap
install.sh         bash installer (like apcode)
install.ps1        PowerShell installer (Windows)
Makefile           build, test, cross-compile
.goreleaser.yaml   release config (binaries, archives, brew tap)
npm/               npm wrapper (apcode-ai) like apcode-ai
homebrew/          Homebrew formula template
scoop/             Scoop manifest template
.github/workflows/ CI + release + install-test
```

---

## Build From Source

Requires [Go](https://go.dev) 1.26+.

```sh
go build -o apcode ./cmd/apcode
./apcode --help

# With version via ldflags (like GoReleaser) — keep in sync with internal/config.Version (0.1.5)
go build -ldflags "-X apcode/internal/config.Version=0.1.5" -o apcode ./cmd/apcode

# Using Make (preferred)
make build          # current platform
make install        # to ~/.apcode/bin
make test           # go test ./...
make release        # cross-compile all platforms to dist/
make clean
make help           # list targets
```

---

## Usage — Detailed

### System Info

```sh
apcode
apcode --no-color
```

### Benchmark

```sh
apcode benchmark
# Measures CPU (deterministic hashing workload), Memory (bandwidth), Storage (optional, disabled by default)
# Ctrl+C cancels cleanly, memory capped at 512 MiB, storage uses temp file + Sync()
```

### Init

```sh
apcode init                          # init current dir: creates .apcode/config.json + ~/.apcode/config.json
apcode init --dir ./myproject        # target directory
apcode init --dir . --force          # overwrite existing config
# Initializes model dir (~/.apcode/models), user config, and project-local .apcode/
```

### Agent (flagship)

```sh
apcode run "Add authentication to my Go API"
apcode run "Fix failing tests" --model phi-3-mini-q4 --stream --max-iterations 10 --dir ./myproject --no-color
# Usage: apcode run <instruction> [--model <id>] [--stream] [--max-iterations N] [--dir <path>] [--no-color]
# The agent loop: understands repo → plans → edits via 16 tools with approval → validates → bounded repair (2 attempts)
```

### Models

```sh
apcode models              # all 6 coding models (CodeLlama 7B/13B, DeepSeek 6.7B, Phi-3 Mini, Gemma 2B, Qwen2.5 7B)
apcode models installed    # installed only (scans ~/.apcode/models)
apcode models info phi-3-mini-q4
apcode models install phi-3-mini-q4   # also: pull / download — same alias
# Current behavior: creates a local 1 MiB deterministic stub file for offline testing
# (no remote GGUF URL configured; satisfies native runtime's non-empty file check).
# Requires approval if RAM check fails; shows progress + checksum, atomic move.
```

### Recommendation

```sh
apcode recommend
apcode recommend --capability reasoning --preference quality
apcode recommend --preference memory --benchmark
apcode recommend --capability tool_calling --preference context --no-color
```

**Inputs:** HardwareProfile + optional benchmark + model metadata + `--capability`/`--preference`.

**Hard constraints:** RAM incompatibility → `HARD:` reject; capability mismatch → filtered.

**Soft scoring (max 100):**

| Weight | Meaning |
|---|---|
| 30 | Capability match (15 partial when no capability) |
| 25 | Memory fit (Good) / 12 when Tight |
| 15 | Efficiency (size buckets) adjusted by preference |
| 10 | Benchmark suitability (fast/slow hw bias) |
| 10 | Context length (+2 for `context` pref) |
| 10 | Installed bonus |

Ranking: FitScore desc → Installed first → smaller size → lexicographic ID. Uncertainty reported.

### Context & Search

```sh
apcode context                          # auto-detect root, prints files/languages/tokens
apcode context --root ./myproject --budget 4000 --ignore "*.log,dist"
apcode search "MySymbol" --dir ./internal --limit 20 --kind function
apcode search "import" --dir . --no-color
```

### Runtime & Inference

```sh
apcode runtime                          # shows runtime+model installed status
apcode infer "hello world"              # auto-picks first compatible installed model
apcode infer --model phi-3-mini-q4 --stream "write a fibonacci function"
apcode infer --max-tokens 256 --prompt "explain apcode"
apcode infer --no-color "test"
```

---

## Benchmarking Notes

- **CPU:** integer hashing mix, configurable workers/work units → ops/sec
- **Memory:** sequential read/write over buffer (default 64 MiB, max 512 MiB, 3 passes) → bytes/sec
- **Storage:** sequential write+read via temp file (default 256 MiB, max 1 GiB, 4 KiB blocks, Sync, disabled by default)
- No GPU benchmark (no reliable cross-platform), no fabricated scores, raw measurements only, `context.Context` cancellation.

---

## Testing

```sh
go test ./...
make test
go test ./... -v -race
go vet ./...
```

Comprehensive deterministic tests for `recommendation`, `benchmark`, `hardware`, `model`, `localmodel`, `runtime`, `context`, `codeintel`, `tools`, `agent`, `tui`.

---

## Dependencies

None beyond Go standard library for core (plus Go toolchain). Install scripts use `curl`/`wget`/`tar`/`unzip` (standard).

---

## Uninstall

```sh
# If installed via install.sh / Makefile
rm -f ~/.apcode/bin/apcode
# Remove PATH line from ~/.bashrc / ~/.zshrc / ~/.config/fish/config.fish:
#   export PATH="$HOME/.apcode/bin:$PATH"
#   or fish_add_path $HOME/.apcode/bin

# npm
npm uninstall -g apcode-ai

# brew
brew uninstall apcode

# scoop
scoop uninstall apcode

# go install
rm -f $(go env GOPATH)/bin/apcode
```

---

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) and `docs/release-test.md` for the clean-install test. PRs welcome!

### Releasing v0.1.5

1. Ensure `internal/config.Version` is `0.1.5` (single source; `npm/package.json`, `homebrew/apcode.rb`, `scoop/apcode.json`, `packaging/winget/*.yaml` are synced on release).
2. `go fmt ./... && go vet ./... && go test ./... -count=1 && go test -race ./...`
3. `go build -ldflags "-X apcode/internal/config.Version=0.1.5" -o /tmp/apcode ./cmd/apcode && /tmp/apcode --version` → `APCode 0.1.5`
4. Commit and push to `main` on GitHub remote (`anshulchikhale30-p/APCode`):
   ```sh
   git remote -v  # should be https://github.com/anshulchikhale30-p/APCode.git
   git add .
   git commit -m "release: v0.1.5"
   git push -u origin main
   ```
5. Tag and push the next release (this triggers the release workflow):
   ```sh
   git tag v0.1.5
   git push origin v0.1.5
   ```
   Pushing `v0.1.5` triggers GitHub Actions (`.github/workflows/release.yml`) which runs GoReleaser (`goreleaser release --clean`) → creates GitHub Release with:
   - `apcode_0.1.5_linux_amd64.tar.gz`, `linux_arm64`, `darwin_amd64`, `darwin_arm64`, `windows_amd64.zip`, `windows_arm64.zip`, `checksums.txt`.
6. After release, update hashes where needed (all **future / not yet published** until verified):
   - `npm`: `cd npm && npm version 0.1.5 && npm publish` (requires `npm login`) — **future**.
   - `homebrew`: GoReleaser would publish to a tap if configured; currently **not configured** for first release, so manually update `homebrew/apcode.rb` hashes from `checksums.txt` if needed — **future**.
   - `scoop`: Update `scoop/apcode.json` hashes and submit to `ScoopInstaller/Main` or your bucket — **future**.
   - `winget`: Follow `docs/packaging/winget.md` to replace `REPLACE_WITH_SHA256_*` and submit to `microsoft/winget-pkgs` — **future**.
   - `docker`: `docker build --build-arg VERSION=0.1.5 -t ghcr.io/anshulchikhale30-p/APCode:0.1.5 . && docker push ghcr.io/anshulchikhale30-p/APCode:0.1.5` — **future / planned**.
   - `chocolatey`: `choco install apcode` — **future / not yet published**.

Until those publishes are verified, **do not claim** `brew install`, `scoop install`, `npm install -g apcode-ai`, `docker run`, `winget install APCode.APCode`, or `choco install apcode` work. The reliable install remains `install.sh`/`install.ps1` via `curl`/`irm` or manual binary from Releases. See `docs/release-test.md` for the full test matrix.

---

## License

MIT — see [LICENSE](./LICENSE).
