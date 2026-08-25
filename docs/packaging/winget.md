# WinGet Packaging for APCode

This guide explains how to prepare, test, and submit APCode to the [Microsoft WinGet](https://learn.microsoft.com/en-us/windows/package-manager/) community repository (`microsoft/winget-pkgs`). WinGet is **not yet published** for APCode — the following is the exact procedure to make `winget install APCode.APCode` work after a real `v0.1.0` GitHub release.

## 1. Prerequisites

- A real GitHub release `v0.1.0` at `https://github.com/anshulchikhale30-p/APCode/releases/tag/v0.1.0` with assets:
  - `apcode_0.1.0_windows_amd64.zip`
  - `apcode_0.1.0_windows_arm64.zip` (now built via GoReleaser)
  - `checksums.txt` (SHA256)
- [WinGet](https://aka.ms/getwinget) installed (`winget --version`)
- Optional: [wingetcreate](https://github.com/microsoft/winget-create) (`wingetcreate --version`)

## 2. PackageIdentifier

We use:

- **PackageIdentifier:** `APCode.APCode` (Publisher `APCode`, Package `APCode`)
- **Publisher:** `APCode`
- **PackageName:** `APCode`
- Alternatives considered: `apcode.apcode` (lowercase) — WinGet is case-insensitive, but `APCode.APCode` matches the repo `anshulchikhale30-p/APCode` and the Go `AppName`.

If you fork or change the GitHub org, update `PackageIdentifier` accordingly (e.g., `YourOrg.APCode`).

## 3. Manifest Files

Templates are in `packaging/winget/`:

- `APCode.APCode.yaml` — merged single-file manifest (WinGet 1.6+, simplest)
- `APCode.APCode.installer.yaml` — split installer manifest
- `APCode.APCode.locale.en-US.yaml` — split locale manifest
- `version.yaml` — split version manifest

For submission, either submit the single merged file or the three split files under `manifests/a/APCode/APCode/0.1.0/`. Do not submit both styles at once.

**All manifests currently contain `REPLACE_WITH_SHA256_*` placeholders.** Do not submit with placeholders.

## 4. Calculate Hashes

After a real release, download each zip and compute SHA256:

### PowerShell (Windows)

```powershell
Invoke-WebRequest -Uri https://github.com/anshulchikhale30-p/APCode/releases/download/v0.1.0/apcode_0.1.0_windows_amd64.zip -OutFile apcode_0.1.0_windows_amd64.zip
Get-FileHash apcode_0.1.0_windows_amd64.zip -Algorithm SHA256
# Same for arm64
Invoke-WebRequest -Uri https://github.com/anshulchikhale30-p/APCode/releases/download/v0.1.0/apcode_0.1.0_windows_arm64.zip -OutFile apcode_0.1.0_windows_arm64.zip
Get-FileHash apcode_0.1.0_windows_arm64.zip -Algorithm SHA256
# Also verify against checksums.txt
Invoke-WebRequest -Uri https://github.com/anshulchikhale30-p/APCode/releases/download/v0.1.0/checksums.txt -OutFile checksums.txt
Get-Content checksums.txt | Select-String "windows"
```

### Bash (Linux/macOS)

```sh
curl -LO https://github.com/anshulchikhale30-p/APCode/releases/download/v0.1.0/apcode_0.1.0_windows_amd64.zip
sha256sum apcode_0.1.0_windows_amd64.zip
curl -LO https://github.com/anshulchikhale30-p/APCode/releases/download/v0.1.0/checksums.txt
grep windows_amd64 checksums.txt
```

Replace **both** occurrences in each manifest:

- `REPLACE_WITH_SHA256_WINDOWS_AMD64`
- `REPLACE_WITH_SHA256_WINDOWS_ARM64`

Do not invent hashes; copy exactly the 64-char hex from the download or `checksums.txt`.

## 5. Test the Installer

### Validate manifest

```powershell
winget validate packaging/winget/APCode.APCode.yaml
# Or for split files:
winget validate manifests/a/APCode/APCode/0.1.0/
```

Fix any schema errors reported. Required fields: `PackageIdentifier`, `PackageVersion`, `InstallerType`, `InstallerUrl`, `InstallerSha256`, `Commands`.

### Test install from local manifest (before submission)

```powershell
# Install directly from local manifest (no submission needed)
winget install --manifest packaging/winget/APCode.APCode.yaml

# Verify
apcode --version
apcode version
apcode --help
apcode models
where.exe apcode  # should be %LOCALAPPDATA%\Microsoft\WinGet\Links\apcode.exe or shims

# Uninstall / test upgrade
winget uninstall APCode.APCode
# Re-install to test idempotency
winget install --manifest packaging/winget/APCode.APCode.yaml --force
```

For `zip` + `portable` installers, WinGet extracts `apcode.exe` and creates a shim at `%LOCALAPPDATA%\Microsoft\WinGet\Links\`. Ensure `apcode` is in PATH after install (WinGet adds its Links folder to PATH).

If using `wingetcreate`:

```powershell
wingetcreate update APCode.APCode --version 0.1.0 --urls https://github.com/anshulchikhale30-p/APCode/releases/download/v0.1.0/apcode_0.1.0_windows_amd64.zip https://github.com/anshulchikhale30-p/APCode/releases/download/v0.1.0/apcode_0.1.0_windows_arm64.zip --manifest-dir manifests
```

But verify the generated manifest still uses `portable` + `NestedInstallerFiles`.

## 6. Submit to microsoft/winget-pkgs

1. Fork https://github.com/microsoft/winget-pkgs
2. Clone your fork:
   ```sh
   git clone https://github.com/YOUR_GITHUB_USERNAME/winget-pkgs
   cd winget-pkgs
   ```
3. Create branch:
   ```sh
   git checkout -b apcode-0.1.0
   ```
4. Copy manifests to:
   ```
   manifests/a/APCode/APCode/0.1.0/APCode.APCode.yaml
   manifests/a/APCode/APCode/0.1.0/APCode.APCode.installer.yaml
   manifests/a/APCode/APCode/0.1.0/APCode.APCode.locale.en-US.yaml
   # If using merged manifest, just the single file is enough; CI will split.
   ```
   The easiest is to copy the merged manifest as `APCode.APCode.yaml` and let the WinGet CI bot split it, or provide the three split files as generated above.

5. Validate locally:
   ```powershell
   winget validate manifests/a/APCode/APCode/0.1.0/
   ```

6. Commit and push:
   ```sh
   git add manifests/a/APCode/APCode/0.1.0/
   git commit -m "New package: APCode.APCode version 0.1.0"
   git push origin apcode-0.1.0
   ```

7. Open a Pull Request against `microsoft/winget-pkgs:master`. The title should be `New package: APCode.APCode version 0.1.0`.

8. Wait for WinGet validation bot (checks URLs, hashes, manifest schema). Fix any bot comments.

9. After merge, it takes a few hours for `winget install APCode.APCode` to work from any Windows machine:
   ```powershell
   winget search APCode
   winget install APCode.APCode
   apcode --version
   ```

Do **not** claim `winget install APCode.APCode` works until the PR is merged and `winget search` finds it.

## 7. Update Documentation After Publication

Once published, update:

- `README.md` — change the WinGet section from “future” to:
  ```powershell
  winget install APCode.APCode
  ```
- `packaging/winget/*.yaml` — replace placeholders with real hashes and commit.

## 8. Troubleshooting

- **Hash mismatch:** Re-download the zip and re-run `Get-FileHash`; ensure you are hashing the exact file from the release, not a re-zipped file.
- **URL 404:** Ensure `v0.1.0` tag exists and assets are named exactly `apcode_0.1.0_windows_amd64.zip` (see `.goreleaser.yaml` `name_template`).
- **Portable not in PATH:** WinGet’s portable installer adds `%LOCALAPPDATA%\Microsoft\WinGet\Links` to PATH; restart terminal.
- **Validation error `Missing Agreement`:** Add `License` and `LicenseUrl` (already present).

## 9. References

- WinGet manifest schema: https://github.com/microsoft/winget-pkgs#manifests
- `wingetcreate` docs: https://github.com/microsoft/winget-create
- GoReleaser release naming: `.goreleaser.yaml` `archives.name_template`
