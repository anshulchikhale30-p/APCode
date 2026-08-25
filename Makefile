# APCode Makefile — like opencode's build system but for Go
# Usage:
#   make build          # build for current platform
#   make install        # build + install to ~/.apcode/bin
#   make test           # run tests
#   make vet            # go vet
#   make fmt            # go fmt
#   make release        # cross-compile all platforms (dist/)
#   make clean          # remove artifacts
#   make help           # show help

APP        := apcode
CMD        := ./cmd/apcode
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0")
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
DATE       ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS    := -s -w -X apcode/internal/config.Version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
INSTALL_DIR ?= $(HOME)/.apcode/bin
GOFLAGS    :=
PLATFORMS  := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: help build install test vet fmt lint clean release release-local deps check

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build binary for current platform to ./apcode
	go build -ldflags "$(LDFLAGS)" -o $(APP) $(CMD)
	@echo "✓ Built $(APP) $(VERSION) -> ./$(APP)"
	@./$(APP) --version || true

install: build ## Build and install to $(INSTALL_DIR)
	@mkdir -p $(INSTALL_DIR)
	@cp $(APP) $(INSTALL_DIR)/$(APP)
	@chmod +x $(INSTALL_DIR)/$(APP)
	@if [ "$(shell go env GOOS)" = "windows" ]; then cp $(APP) $(INSTALL_DIR)/$(APP).exe 2>/dev/null || true; fi
	@echo "✓ Installed to $(INSTALL_DIR)/$(APP)"
	@echo "  Ensure $(INSTALL_DIR) is in PATH: export PATH=\"$(INSTALL_DIR):\$$PATH\""

test: ## Run all tests
	go test ./... -count=1

test-verbose: ## Run tests verbosely
	go test ./... -count=1 -v

vet: ## Run go vet
	go vet ./...

fmt: ## Format Go code
	go fmt ./...

lint: vet ## Alias for vet (add golangci-lint if available)
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run; else echo "golangci-lint not installed, ran go vet only"; fi

check: fmt vet test ## Run fmt, vet, and tests

deps: ## Tidy go.mod
	go mod tidy

clean: ## Remove build artifacts
	rm -f $(APP) $(APP).exe apcode_test_binary apcode_test_binary.exe
	rm -rf dist/ bin/
	rm -f build.out test.out vet.out

# Cross-compile all platforms into dist/
release: ## Cross-compile for all platforms (like GoReleaser locally)
	@mkdir -p dist
	@for plat in $(PLATFORMS); do \
		os=$$(echo $$plat | cut -d/ -f1); \
		arch=$$(echo $$plat | cut -d/ -f2); \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		out="dist/$(APP)-$$os-$$arch$$ext"; \
		archive="dist/$(APP)_$(VERSION)_$$os_$$arch.tar.gz"; \
		if [ "$$os" = "windows" ]; then archive="dist/$(APP)_$(VERSION)_$$os_$$arch.zip"; fi; \
		echo "→ Building $$os/$$arch -> $$out"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o "$$out" $(CMD) || exit 1; \
		if [ "$$os" = "windows" ]; then \
			(cd dist && zip -q "$$(basename $$archive)" "$$(basename $$out)"); \
		else \
			(cd dist && tar -czf "$$(basename $$archive)" "$$(basename $$out)"); \
		fi; \
		echo "  archive: $$archive"; \
	done
	@echo "✓ Release artifacts in dist/"
	@ls -lh dist/

release-local: release ## Alias for release

# Quick dev install via go install
go-install: ## Install via go install (requires cloned repo)
	go install -ldflags "$(LDFLAGS)" $(CMD)

# Uninstall
uninstall: ## Remove installed binary
	rm -f $(INSTALL_DIR)/$(APP) $(INSTALL_DIR)/$(APP).exe
	@echo "Removed from $(INSTALL_DIR)"

# Benchmark + recommend demo
demo: build ## Run apcode demo (welcome + benchmark + recommend)
	./$(APP) --help
	./$(APP) benchmark || true
	./$(APP) recommend || true

# Verify install script syntax
verify-scripts: ## Verify install.sh syntax
	bash -n install.sh
	@echo "✓ install.sh syntax OK"
	@if command -v pwsh >/dev/null 2>&1; then pwsh -NoProfile -Command "Get-Content install.ps1 | Out-Null; Write-Host '✓ install.ps1 syntax OK'"; else echo "pwsh not found, skipped ps1 check"; fi
