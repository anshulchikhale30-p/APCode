# Contributing to APCode

## Development Setup

```sh
git clone https://github.com/apcode/apcode
cd apcode
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

Tag and push triggers GoReleaser:

```sh
git tag v0.2.0
git push origin v0.2.0
# GitHub Actions runs .github/workflows/release.yml -> GoReleaser -> GitHub Release + Homebrew tap
```

Update `npm/package.json` version to match and publish:

```sh
cd npm
npm version 0.2.0
npm publish --access public
```

## Code Style

- Go standard `gofmt` + `go vet`
- Standard library first, no cgo required
- Offline-first: no network calls in core commands except install scripts
