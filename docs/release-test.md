# Release Test — Clean Install

Reproducible procedure to verify `v0.1.0` on a clean machine **without Go installed**. Mirrors the user experience for Windows, Linux, and macOS. Use a fresh VM, container, or a new user profile.

## Prerequisites

- Clean VM/container or new user account (no prior `apcode` in `PATH`, no `~/.apcode`).
- Network access to `github.com` and `raw.githubusercontent.com`.
- For binary test: download the correct artifact from `https://github.com/anshulchikhale30-p/APCode/releases/tag/v0.1.0`.

> Replace `anshulchikhale30-p/APCode` with the actual GitHub org/repo if different. The `v0.1.0` tag must exist before testing download flows. For pre-release testing, use `--binary` with a local build.

---

## Windows (PowerShell)

### 1. Install via `install.ps1` (no Go required)

```powershell
# Option A: Remote install (requires release published)
irm https://raw.githubusercontent.com/anshulchikhale30-p/APCode/main/install.ps1 | iex

# Option B: Specific version
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/anshulchikhale30-p/APCode/main/install.ps1))) -Version 0.1.0

# Option C: Local binary (for pre-release QA, no network)
go build -o $env:TEMP\apcode.exe ./cmd/apcode   # build on a dev machine, copy to clean VM
.\install.ps1 -Binary $env:TEMP\apcode.exe -InstallDir $env:USERPROFILE\.apcode\bin -NoModifyPath
```

The installer prints:
- `Installing apcode version: X.Y.Z for windows/amd64` (or `arm64`)
- `Installed to C:\Users\<user>\.apcode\bin\apcode.exe`
- `Verified: APCode X.Y.Z`
- `Added ... to PATH (restart terminal)` if `--NoModifyPath` not used.

If you used `-NoModifyPath`, manually add to PATH for this session:

```powershell
$env:Path += ";$env:USERPROFILE\.apcode\bin"
```

### 2. Verify PATH

```powershell
Get-Command apcode | Format-List Source
# Should be C:\Users\<user>\.apcode\bin\apcode.exe
where.exe apcode
$env:Path -split ";" | Select-String "apcode"
[Environment]::GetEnvironmentVariable("Path","User") -split ";" | Select-String "apcode"
```

### 3. Run commands

```powershell
apcode --version
# APCode 0.1.0

apcode version
# APCode 0.1.0  (subcommand)

apcode --help
# shows Usage, Flags

apcode
# branded TUI: APCode banner + OS/arch/CPU/RAM/GPU + version + offline mode

apcode --no-color
# same but without ANSI colors (check no escape codes)

apcode models
# 6 models, not installed

apcode models info phi-3-mini-q4
# detailed info

apcode recommend
# ranked recommendation, Fit Score, Reasons, Warnings

apcode recommend --no-color --preference speed --capability code_generation

apcode benchmark
# CPU complete, Memory complete, Storage unavailable (disabled by default), Duration
```

### 4. Verify `benchmark` respects cancellation

```powershell
# Run and Ctrl+C within 1 sec — should exit cleanly, not hang
apcode benchmark
```

### 5. Test `search` and `context` (offline, no LLM)

```powershell
apcode context
apcode search "func main" --dir ./cmd --limit 5
```

### 6. Test `runtime` and `infer` (mock)

```powershell
apcode runtime
# Runtime: installed (native) but Model: not installed

# With no model, infer should give clear error:
apcode infer "hello"
# -> Model: not installed, hint to use `apcode models`
```

### 7. Uninstall / Reinstall (idempotency)

```powershell
# Uninstall
Remove-Item -Recurse -Force $env:USERPROFILE\.apcode\bin
# Remove PATH entry (if added): edit User PATH via
# [Environment]::SetEnvironmentVariable("Path", (([Environment]::GetEnvironmentVariable("Path","User") -split ";" | Where-Object { $_ -ne "$env:USERPROFILE\.apcode\bin" }) -join ";"), "User")

# Re-install via --binary should succeed
.\install.ps1 -Binary $env:TEMP\apcode.exe -InstallDir $env:USERPROFILE\.apcode\bin -NoModifyPath
apcode --version  # still 0.1.0

# Re-running installer when same version already installed should be idempotent:
.\install.ps1 -Binary $env:TEMP\apcode.exe -InstallDir $env:USERPROFILE\.apcode\bin -NoModifyPath
# -> "Version 0.1.0 already installed at ..."
```

---

## Linux / macOS (bash)

> No Go required for binary install. Use a clean container: `docker run --rm -it ubuntu:24.04 bash` or `debian:bookworm`.

### 1. Install via `install.sh`

```sh
# Remote (requires release)
curl -fsSL https://raw.githubusercontent.com/anshulchikhale30-p/APCode/main/install.sh | bash

# Specific version
curl -fsSL https://raw.githubusercontent.com/anshulchikhale30-p/APCode/main/install.sh | bash -s -- --version 0.1.0

# Custom dir
curl -fsSL https://raw.githubusercontent.com/anshulchikhale30-p/APCode/main/install.sh | bash -s -- --dir /usr/local/bin

# Local binary (pre-release)
go build -o /tmp/apcode ./cmd/apcode   # on dev machine, copy to clean VM
bash install.sh --binary /tmp/apcode --dir /tmp/apcode-bin --no-modify-path
export PATH="/tmp/apcode-bin:$PATH"
```

The installer:
- Detects `linux`/`darwin` + `amd64`/`arm64` (handles `aarch64`→`arm64`, `x86_64`→`amd64`, Rosetta on darwin)
- Verifies `curl`/`wget`, `tar`/`unzip`, optional `sha256sum`/`shasum` checksum
- Downloads `apcode_<version>_<os>_<arch>.tar.gz` and `checksums.txt` (if available)
- Extracts, `mv apcode /tmp/apcode-bin/apcode`, `chmod 755`
- Prints `✓ Installed apcode to /tmp/apcode-bin/apcode` and `✓ Verified: APCode 0.1.0`
- Adds `export PATH="$INSTALL_DIR:$PATH"` to `~/.bashrc`/`~/.zshrc` unless `--no-modify-path`

If `--no-modify-path`, manually:

```sh
export PATH="$HOME/.apcode/bin:$PATH"  # or /tmp/apcode-bin for test
```

### 2. Verify PATH

```sh
which apcode
command -v apcode
echo $PATH | tr ':' '\n' | grep apcode
grep -r "apcode" ~/.bashrc ~/.zshrc 2>/dev/null | head
```

### 3. Run commands

```sh
apcode --version           # APCode 0.1.0
apcode version             # APCode 0.1.0
apcode --help | head -n 30
apcode                     # banner + hardware + version
apcode --no-color          # no ANSI
apcode models
apcode models installed    # 0 installed
apcode models info gemma-2b-q4
apcode recommend
apcode recommend --benchmark  # runs benchmark then ranks
apcode recommend --no-color --preference memory
apcode benchmark
apcode context --budget 4000
apcode search "func main" --dir ./cmd --limit 5
apcode runtime
apcode infer "hello"  # should error clearly with no model
```

### 4. Verify benchmark cancellation

```sh
timeout 2 apcode benchmark || true  # should not hang, Ctrl+C equivalent
```

### 5. Uninstall / Reinstall

```sh
rm -rf ~/.apcode/bin/apcode /tmp/apcode-bin
# Remove PATH line from ~/.bashrc:
# grep -v "apcode" ~/.bashrc > /tmp/bashrc.new && mv /tmp/bashrc.new ~/.bashrc
bash install.sh --binary /tmp/apcode --dir /tmp/apcode-bin --no-modify-path
/tmp/apcode-bin/apcode --version
# Re-run should be idempotent:
bash install.sh --binary /tmp/apcode --dir /tmp/apcode-bin --no-modify-path
# -> "Version 0.1.0 already installed at /tmp/apcode-bin/apcode"
```

---

## npm (all platforms, requires Node)

> The `apcode-ai` npm package is **not yet published** to the npm registry. The following is how it will work after `npm publish` in `npm/`.

```sh
npm install -g apcode-ai
apcode --version
apcode --help

# Verify shim
node $(npm root -g)/apcode-ai/bin/apcode.js --version
ls -lh $(npm root -g)/apcode-ai/bin/
cat $(npm root -g)/apcode-ai/bin/checksums.txt 2>/dev/null || true

# Update
npm update -g apcode-ai

# Uninstall
npm uninstall -g apcode-ai
which apcode || echo "not found (expected)"
```

For local testing without registry:

```sh
cd npm
npm pack  # creates apcode-ai-0.1.0.tgz
npm install -g ./apcode-ai-0.1.0.tgz
apcode --version
```

---

## Scoop (Windows)

> Scoop manifest `scoop/apcode.json` is a template with `REPLACE_WITH_SHA256_*`. It is not yet in the main Scoop bucket. Test locally:

```powershell
scoop install ./scoop/apcode.json
apcode --version
scoop uninstall apcode
```

After publication to `https://github.com/ScoopInstaller/Main` or a custom bucket `anshulchikhale30-p/scoop-bucket`, users will:

```powershell
scoop bucket add apcode https://github.com/anshulchikhale30-p/scoop-bucket
scoop install apcode
```

---

## Homebrew (macOS/Linux)

> Formula `homebrew/apcode.rb` is a template; `anshulchikhale30-p/homebrew-tap` must exist and be populated by GoReleaser. Not yet published.

```sh
brew install --build-from-source ./homebrew/apcode.rb
apcode --version
brew test apcode
brew uninstall apcode

# After tap published:
brew install anshulchikhale30-p/tap/apcode
```

---

## WinGet (Windows)

> See `docs/packaging/winget.md`. Manifests in `packaging/winget/` contain `REPLACE_WITH_SHA256_*` and are **not yet submitted** to `microsoft/winget-pkgs`.

Local test (after real release and hash replacement):

```powershell
winget validate packaging/winget/APCode.APCode.yaml
winget install --manifest packaging/winget/APCode.APCode.yaml
apcode --version
winget uninstall APCode.APCode
```

After acceptance, users will:

```powershell
winget install APCode.APCode
```

---

## Docker (no install)

```sh
docker build -t apcode:0.1.0 --build-arg VERSION=0.1.0 .
docker run --rm apcode:0.1.0 --version
docker run --rm apcode:0.1.0 --help
docker run --rm -v $(pwd):/work apcode:0.1.0 recommend
docker run --rm -v $(pwd):/work apcode:0.1.0 search "func main" --dir /work
```

---

## Expected Outputs

- `apcode --version` and `apcode version` both print `APCode 0.1.0` (single line, no extra logging).
- `apcode` prints branded banner, hardware profile, `APCode version: 0.1.0`, `Offline mode: enabled`.
- `apcode --no-color` strips ANSI.
- `apcode models` → 6 models.
- `apcode recommend` → `Phi-3 Mini` or `Gemma 2B` depending on RAM (on 16GiB, `Phi-3` ranked top due to context length), with `Fit Score:`.
- `apcode benchmark` → `CPU: complete`, `Memory: complete`, `Storage: unavailable` (or `complete` if enabled), no hangs.
- `apcode runtime` → `Runtime: installed` (native) + `Model: not installed` on clean machine.
- All commands exit 0; `--help` prints `Usage:`.

If any command fails or prints the wrong version, the release is not ready.
