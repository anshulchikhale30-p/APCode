# Contributing to APCode

## Development Setup

```sh
git clone https://github.com/anshulchikhale30-p/APCode
cd APCode
go version  # requires 1.26+
make build
./apcode --help
```

## Make Targets

| Command | Description |
|---|---|
| `make build` | Build for current platform |
| `make test` | Run tests |
| `make vet` | Go vet |
| `make release` | Cross-compile all platforms into `dist/` |
| `make install` | Install to `~/.apcode/bin` |
| `make clean` | Clean artifacts |
| `make verify-scripts` | Check install.sh / install.ps1 syntax |

## Install Script Testing

```sh
# Test bash installer locally (no network)
go build -o /tmp/apcode ./cmd/apcode
bash install.sh --binary /tmp/apcode --dir /tmp/apcode-test --no-modify-path
/tmp/apcode-test/apcode --version

# PowerShell (Windows)
go build -o C:\Temp\apcode.exe ./cmd/apcode
.\install.ps1 -Binary C:\Temp\apcode.exe -InstallDir C:\Temp\apcode-test -NoModifyPath
C:\Temp\apcode-test\apcode.exe --version
```

## Releasing

Tag and push triggers GoReleaser (GitHub Actions `release.yml` → `goreleaser release --clean` → GitHub Release with binaries + checksums):

```sh
git tag v0.1.1
git push origin v0.1.1
# Pushing v0.1.1 triggers .github/workflows/release.yml -> GoReleaser -> GitHub Release
```

Update `npm/package.json` version to match and publish (future, not required for first release):

```sh
cd npm
npm version 0.1.1
npm publish --access public
```

## Code Style

- Go standard `gofmt` + `go vet`
- Standard library first, no cgo required
- Offline-first: no network calls in core commands except install scripts
