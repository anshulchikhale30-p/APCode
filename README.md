# APCode

**Offline-first AI coding agent for the terminal — like OpenCode, but everything runs on your laptop.**

> We care about your system. 😄  
> So you can focus on your ideas. 💡  
> Making the most of every bit of your laptop. ⚡

APCode is an open source AI coding agent you install in your terminal. It understands your hardware, recommends the right local model, and runs 100% offline — no cloud APIs, no data leaving your machine.

> **v0.1.0 is ready for release, but no GitHub Release exists yet.** The `install.sh`/`install.ps1` scripts work today via `--binary` with a local build. Remote `curl | bash` / `irm | iex` and `brew`/`scoop`/`npm`/`winget` will work **after** a real `v0.1.0` tag is pushed and GoReleaser publishes artifacts. See `CONTRIBUTING.md` and `docs/packaging/winget.md`.

---

## Install

The easiest way to install APCode is through the install script — just like `opencode`.

### Quick install (recommended)

**macOS & Linux:**

```sh
curl -fsSL https://raw.githubusercontent.com/apcode/apcode/main/install.sh | bash
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/apcode/apcode/main/install.sh | bash -s -- --version 0.1.0
```

Install to a custom directory:

```sh
curl -fsSL https://raw.githubusercontent.com/apcode/apcode/main/install.sh | bash -s -- --dir /usr/local/bin
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/apcode/apcode/main/install.ps1 | iex
```

Specific version:

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/apcode/apcode/main/install.ps1))) -Version 0.1.0
# or if you saved it locally:
.\install.ps1 -Version 0.1.0
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
apcode --version   # APCode 0.1.0
apcode version     # same
apcode --help
apcode             # branded TUI + hardware
```

---

### Other install methods

**Using Go (from source):**

```sh
# From cloned repo (recommended, no registry required)
git clone https://github.com/apcode/apcode
cd apcode
go build -o apcode ./cmd/apcode
# or install to GOPATH/GOBIN (adds to $(go env GOPATH)/bin)
go install ./cmd/apcode
# then ensure GOPATH/bin is in PATH:
# export PATH="$(go env GOPATH)/bin:$PATH"
```

**Using npm (like `opencode-ai`) — not yet published:**

> `apcode-ai` is prepared in `npm/` but has **not** been published to the npm registry. After `npm publish` in `npm/`, users will be able to:

```sh
npm install -g apcode-ai      # wraps the Go binary, downloads correct platform binary
# bun / pnpm / yarn
bun add -g apcode-ai
pnpm add -g apcode-ai
yarn global add apcode-ai
```

> The package downloads the matching release binary on `postinstall`. No Node runtime needed after install. For now, use the `install.sh`/`install.ps1` scripts.

**Using Homebrew (macOS & Linux) — not yet published:**

> Formula `homebrew/apcode.rb` is a template. After a real `v0.1.0` release, GoReleaser publishes to `apcode/homebrew-tap`. Until then:

```sh
# Build from source locally:
brew install --build-from-source ./homebrew/apcode.rb
# After tap is published:
brew install apcode/tap/apcode
```

> The tap `apcode/tap` will be updated on every release via GoReleaser.

**Using Scoop (Windows) — not yet published:**

> Manifest `scoop/apcode.json` is a template with `REPLACE_WITH_SHA256_*`. Not yet in Scoop Main. Test locally:

```powershell
# Local test:
scoop install ./scoop/apcode.json
# After bucket is published:
scoop bucket add apcode https://github.com/apcode/scoop-bucket
scoop install apcode
```

**Using WinGet (Windows) — not yet published:**

> WinGet manifests are prepared in `packaging/winget/` but **not** yet submitted to `microsoft/winget-pkgs`. See `docs/packaging/winget.md` for the exact submission procedure. After acceptance:

```powershell
winget install APCode.APCode
```

> For now, use `install.ps1`.

**Using Chocolatey (Windows) — future:**

```powershell
choco install apcode
```

**Manual binary download:**

Grab the binary for your platform from [Releases](https://github.com/apcode/apcode/releases) (after `v0.1.0` is published):

| Platform | File |
|---|---|
| Linux amd64 | `apcode_0.1.0_linux_amd64.tar.gz` |
| Linux arm64 | `apcode_0.1.0_linux_arm64.tar.gz` |
| macOS Intel | `apcode_0.1.0_darwin_amd64.tar.gz` |
| macOS Apple Silicon | `apcode_0.1.0_darwin_arm64.tar.gz` |
| Windows amd64 | `apcode_0.1.0_windows_amd64.zip` |
| Windows arm64 | `apcode_0.1.0_windows_arm64.zip` |

Then:

```sh
tar -xzf apcode_*_linux_amd64.tar.gz
sudo mv apcode /usr/local/bin/
apcode --version
```

**Using Docker:**

```sh
docker run --rm -it ghcr.io/apcode/apcode --help
docker run --rm -it -v $(pwd):/work ghcr.io/apcode/apcode recommend
```

---

## Quick Start

```sh
# 1. Check your system
apcode                          # welcome banner + OS/CPU/RAM/GPU + version

# 2. Benchmark your hardware (optional but improves recommendations)
apcode benchmark

# 3. See available models
apcode models
apcode models info phi-3-mini-q4

# 4. Get hardware-aware recommendation
apcode recommend
apcode recommend --benchmark --preference balanced
apcode recommend --capability code_generation --preference quality

# 5. Inspect project context (offline, no LLM)
apcode context                  # auto-detect root
apcode context --budget 8000 --root ./myproject
apcode search "func main" --dir ./cmd --limit 20

# 6. Check runtime status
apcode runtime

# 7. Run inference (when runtime + model installed)
apcode infer "write a hello world in Go"
apcode infer --stream --model phi-3-mini-q4 "explain this repo"
```

---

## Vision

Instead of requiring a powerful cloud or GPU cluster, APCode understands your hardware and adapts its AI workload to it:

- **Powerful laptop** → stronger model (e.g., 13B)
- **Low-resource laptop** → efficient small model (e.g., 2B–3B)

Everything runs locally. No cloud APIs, no data leaving your machine.

---

## Current Capabilities

- `apcode --version` / `apcode version` — print version (both work, single source `internal/config.Version`)
- `apcode --help` — print usage
- `apcode` — welcome banner + system info (OS, arch, CPU, RAM, GPU, version, offline status)
- `apcode benchmark` — hardware benchmarks (CPU ops/sec, memory bandwidth, optional storage)
- `apcode models` — list catalog (`models installed`, `models info <id>`)
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
cmd/apcode/        CLI entry point (welcome, benchmark, models, recommend, context, search, runtime, infer)
internal/
    agent/         agent loop (plan → act → verify)
    benchmark/     real hardware measurement (CPU/memory/storage)
    codeintel/     symbol extraction, search, imports
    config/        version and app constants (ldflags-overridable)
    context/       project-context gathering (walk, ignore, metadata)
    git/           git integration
    hardware/      system detection (OS/arch, CPU, RAM, GPU)
    localmodel/    local model file management
    model/         model metadata, registry, BuiltInCatalog
    recommendation/hardware-aware ranking engine
    runtime/       inference runtimes (native, llama.cpp, ollama, mock)
    tools/         agent tools (edit, filesystem, search, git diff)
    tui/           terminal rendering (welcome, benchmark, recommendation, context, search)
    verification/  output verification
models/            reserved for local model artifacts
tests/             cross-package tests
docs/              design notes and roadmap
install.sh         bash installer (like opencode)
install.ps1        PowerShell installer (Windows)
Makefile           build, test, cross-compile
.goreleaser.yaml   release config (binaries, archives, brew tap)
npm/               npm wrapper (apcode-ai) like opencode-ai
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

# With version via ldflags (like GoReleaser)
go build -ldflags "-X apcode/internal/config.Version=0.1.0" -o apcode ./cmd/apcode

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

### Models

```sh
apcode models              # all 6 coding models (CodeLlama 7B/13B, DeepSeek 6.7B, Phi-3 Mini, Gemma 2B, Qwen2.5 7B)
apcode models installed    # installed only (scans ~/.apcode/models)
apcode models info phi-3-mini-q4
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

### Releasing v0.1.0

1. Ensure `internal/config.Version` is `0.1.0` (single source; `npm/package.json`, `homebrew/apcode.rb`, `scoop/apcode.json`, `packaging/winget/*.yaml` are synced on release).
2. `go fmt ./... && go vet ./... && go test ./... -count=1 && go test -race ./...`
3. `go build -ldflags "-X apcode/internal/config.Version=0.1.0" -o /tmp/apcode ./cmd/apcode && /tmp/apcode --version` → `APCode 0.1.0`
4. Commit and push to `main` on the real GitHub remote (currently `apcode/apcode` is a placeholder — must match `git remote -v`):
   ```sh
   git init # if needed
   git remote add origin https://github.com/<YOUR_ORG>/apcode.git
   git add .
   git commit -m "release: v0.1.0"
   git push -u origin main
   ```
5. Tag and push:
   ```sh
   git tag v0.1.0
   git push origin v0.1.0
   ```
   GitHub Actions `release.yml` runs GoReleaser → creates GitHub Release with:
   - `apcode_0.1.0_linux_amd64.tar.gz`, `linux_arm64`, `darwin_amd64`, `darwin_arm64`, `windows_amd64.zip`, `windows_arm64.zip`, `checksums.txt`.
6. After release, update hashes where needed:
   - `npm`: `cd npm && npm version 0.1.0 && npm publish` (requires `npm login`).
   - `homebrew`: GoReleaser auto-updates `apcode/homebrew-tap` if the `brews` repository exists and `GITHUB_TOKEN` has access; otherwise manually update `homebrew/apcode.rb` hashes from `checksums.txt`.
   - `scoop`: Update `scoop/apcode.json` hashes and submit to `ScoopInstaller/Main` or your bucket.
   - `winget`: Follow `docs/packaging/winget.md` to replace `REPLACE_WITH_SHA256_*` and submit to `microsoft/winget-pkgs`.
   - `docker`: `docker build --build-arg VERSION=0.1.0 -t ghcr.io/<ORG>/apcode:0.1.0 . && docker push ghcr.io/<ORG>/apcode:0.1.0`

Until those publishes are verified, **do not claim** `brew install`, `scoop install`, `npm install -g apcode-ai`, or `winget install APCode.APCode` work. The reliable install remains `install.sh`/`install.ps1` via `curl`/`irm` or manual binary from Releases. See `docs/release-test.md` for the full test matrix.

---

## License

MIT — see [LICENSE](./LICENSE).
