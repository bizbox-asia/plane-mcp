# Makefile for plane-mcp
#
# Targets:
#   make build                  - Build for current OS/arch
#   make build-all              - Build for all supported platforms
#   make release                - Build all platforms + checksums (local)
#   make github-release         - Build + publish to GitHub (needs VERSION=v1.2.3)
#   make github-release-dry-run - Build only, don't publish
#   make github-release-prerelease - Publish as pre-release
#   make test                   - Run unit tests
#   make smoke                  - Run integration smoke test (requires API key)
#   make clean                  - Remove built binaries
#   make install                - Install to $GOPATH/bin
#   make fmt                    - Format code
#   make lint                   - Run go vet
#
# Environment variables:
#   VERSION  - Release version (default: git describe)
#   GOOS     - Target OS (overridable for cross-compile)
#   GOARCH   - Target arch (overridable for cross-compile)
#   CGO_ENABLED - 0 by default for static binaries

# ---- Configuration ----

BINARY      := plane-mcp
SMOKE_BIN   := plane-mcp-smoke
PACKAGE     := github.com/bizbox-asia/plane-mcp
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE  := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w \
              -X 'main.version=$(VERSION)' \
              -X 'main.commit=$(COMMIT)' \
              -X 'main.buildDate=$(BUILD_DATE)'

# Default to static, no CGO for portable single-binary distribution
export CGO_ENABLED ?= 0

# The parent go.work at the repo root conflicts with this module's
# go.mod. We disable the workspace resolver for all build/test/fmt
# invocations and let the local go.mod take precedence.
GOWORK ?= off
export GOWORK

# Supported targets: each line is "OS/ARCH" or "OS/ARCH/EXT" for
# binaries that need an extension (like Windows .exe). To add a
# new target, just append "OS/ARCH" (or "OS/ARCH/.exe") to the list.
PLATFORMS := \
    darwin/amd64     \
    darwin/arm64     \
    linux/amd64      \
    linux/arm64      \
    linux/386        \
    windows/amd64/ \
    windows/386/  \
    freebsd/amd64    \
    freebsd/arm64

# ---- Source ----

SOURCES := $(shell find cmd internal -name '*.go')
PKG     := ./cmd/plane-mcp
SMOKE   := ./cmd/plane-mcp-smoke

# ---- Default target ----

.PHONY: all
all: build

# ---- Build (current platform) ----

.PHONY: build
build: bin/$(BINARY) bin/$(SMOKE_BIN)

bin/$(BINARY): $(SOURCES) go.mod go.sum
	@mkdir -p bin
	@echo "==> Building $(BINARY) for $(shell go env GOOS)/$(shell go env GOARCH)"
	@go build -trimpath -ldflags="$(LDFLAGS)" -o $@ $(PKG)
	@echo "==> Built $@"
	@ls -lh $@

bin/$(SMOKE_BIN): $(SOURCES) go.mod go.sum
	@mkdir -p bin
	@echo "==> Building $(SMOKE_BIN) for $(shell go env GOOS)/$(shell go env GOARCH)"
	@go build -trimpath -ldflags="$(LDFLAGS)" -o $@ $(SMOKE)
	@echo "==> Built $@"
	@ls -lh $@

# ---- Cross-platform build ----

.PHONY: build-all
build-all:
	@echo "==> Building for all $(words $(PLATFORMS)) platforms"
	@$(foreach platform,$(PLATFORMS), \
		mkdir -p dist/$(platform) && \
		$(MAKE) dist/$(platform)/$(BINARY) GOOS=$(word 1,$(subst /, ,$(platform))) GOARCH=$(word 2,$(subst /, ,$(platform))) && \
		echo "";)

# Pattern rule: dist/<os>/<arch>[/<ext>]/<binary>
# Matches: dist/darwin/arm64/plane-mcp
#          dist/windows/amd64/plane-mc
# Usage: make dist/darwin/arm64/plane-mcp
#        make dist/windows/amd64/plane-mcp
.PHONY: dist/%/$(BINARY)
dist/%/$(BINARY): $(SOURCES) go.mod go.sum
	@$(eval OS_ARCH := $(subst dist/,,$(@D)))
	@$(eval GOOS := $(word 1,$(subst /, ,$(OS_ARCH))))
	@$(eval GOARCH := $(word 2,$(subst /, ,$(OS_ARCH))))
	@echo "==> Building $(BINARY) for $(GOOS)/$(GOARCH)"
	@mkdir -p $(@D)
	@GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 \
		go build -trimpath -ldflags="$(LDFLAGS)" \
		-o $@ $(PKG)
	@echo "==> Built $@ ($(GOOS)/$(GOARCH))"

# ---- Release ----

RELEASE_DIR := dist/release-$(VERSION)

# Return the binary extension for a given OS (empty for unix, .exe for windows).
# Usage: $(call bin_ext,$(GOOS))
define bin_ext
$(if $(filter windows,$(1)),.exe,)
endef

.PHONY: release
release: clean-release
	@echo "==> Building release $(VERSION) for all platforms"
	@mkdir -p $(RELEASE_DIR)
	@$(foreach platform,$(PLATFORMS), \
		$(eval GOOS := $(word 1,$(subst /, ,$(platform)))) \
		$(eval GOARCH := $(word 2,$(subst /, ,$(platform)))) \
		$(eval EXT := $(call bin_ext,$(GOOS))) \
		mkdir -p $(RELEASE_DIR)/$(platform) && \
		GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 \
		go build -trimpath -ldflags="$(LDFLAGS)" \
		-o $(RELEASE_DIR)/$(platform)/$(BINARY)$(EXT) \
		$(PKG) && \
		echo "    -> $(RELEASE_DIR)/$(platform)/$(BINARY)$(EXT)" && \
		chmod +x $(RELEASE_DIR)/$(platform)/$(BINARY)$(EXT);)
	@echo "==> Building checksums.txt"
	@cd $(RELEASE_DIR) && \
		shasum -a 256 $$(find . -type f -name '$(BINARY)*' -not -name '*.sha256' | sort) > checksums.txt
	@echo "==> Release built: $(RELEASE_DIR)/"
	@ls -la $(RELEASE_DIR)/
	@find $(RELEASE_DIR) -type f \( -name '$(BINARY)*' -o -name 'checksums.txt' \) | sort

# Build + publish to GitHub via scripts/release.sh.
# Usage:
#   make github-release VERSION=v1.2.3
#   make github-release-dry-run VERSION=v1.2.3
.PHONY: github-release github-release-dry-run github-release-prerelease
github-release:
	@if [ -z "$(VERSION)" ] || echo "$(VERSION)" | grep -qE '^dev$$|^unknown$$'; then \
		echo "ERROR: VERSION must be set to a real semver (e.g. VERSION=v1.2.3)" >&2; \
		echo "Current VERSION=$(VERSION)" >&2; \
		exit 1; \
	fi
	@./scripts/release.sh $(VERSION)

github-release-dry-run:
	@if [ -z "$(VERSION)" ] || echo "$(VERSION)" | grep -qE '^dev$$|^unknown$$'; then \
		echo "ERROR: VERSION must be set (e.g. VERSION=v1.2.3)" >&2; \
		exit 1; \
	fi
	@./scripts/release.sh $(VERSION) --dry-run

github-release-prerelease:
	@if [ -z "$(VERSION)" ] || echo "$(VERSION)" | grep -qE '^dev$$|^unknown$$'; then \
		echo "ERROR: VERSION must be set (e.g. VERSION=v1.2.3-rc1)" >&2; \
		exit 1; \
	fi
	@./scripts/release.sh $(VERSION) --prerelease

.PHONY: clean-release
clean-release:
	@rm -rf dist/release-*

# ---- Test ----

.PHONY: test
test:
	@echo "==> Running unit tests"
	@go test -race -timeout 60s ./...

.PHONY: test-coverage
test-coverage:
	@echo "==> Running tests with coverage"
	@go test -race -timeout 60s -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -20
	@go tool cover -html=coverage.out -o coverage.html
	@echo "==> Coverage report: coverage.html"

.PHONY: smoke
smoke: bin/$(SMOKE_BIN)
	@if [ -z "$$PLANE_API_KEY" ]; then \
		echo "ERROR: PLANE_API_KEY env var is required for smoke test" >&2; \
		echo "Usage: make smoke" >&2; \
		echo "Or:    PLANE_API_KEY=xxx PLANE_WORKSPACE_SLUG=erp make smoke" >&2; \
		exit 1; \
	fi
	@./bin/$(SMOKE_BIN)

# ---- Install ----

.PHONY: install
install: build
	@echo "==> Installing $(BINARY) to $(GOPATH)/bin"
	@cp bin/$(BINARY) $(GOPATH)/bin/$(BINARY)
	@echo "==> Installed: $(GOPATH)/bin/$(BINARY)"

# ---- Clean ----

.PHONY: clean
clean:
	@rm -rf bin/ dist/
	@echo "==> Cleaned build artifacts"

.PHONY: clean-all
clean-all: clean clean-release
	@rm -f coverage.out coverage.html
	@echo "==> Cleaned all artifacts"

# ---- Lint / Format ----

.PHONY: fmt
fmt:
	@echo "==> Formatting code"
	@go fmt ./...

.PHONY: vet
vet:
	@echo "==> Running go vet"
	@go vet ./...

.PHONY: lint
lint: fmt vet
	@echo "==> Lint complete"

# ---- Info ----

.PHONY: info
info:
	@echo "plane-mcp build info:"
	@echo "  VERSION     = $(VERSION)"
	@echo "  COMMIT      = $(COMMIT)"
	@echo "  BUILD_DATE  = $(BUILD_DATE)"
	@echo "  GOOS/GOARCH = $(shell go env GOOS)/$(shell go env GOARCH)"
	@echo "  Go version  = $(shell go version)"
	@echo "  Package     = $(PACKAGE)"
	@echo "  Platforms   = $(words $(PLATFORMS))"
	@for p in $(PLATFORMS); do echo "    - $$p"; done

# ---- Help ----

.PHONY: help
help:
	@echo "plane-mcp Makefile"
	@echo ""
	@echo "Build targets:"
	@echo "  build          Build for current OS/arch"
	@echo "  build-all      Build for all $(words $(PLATFORMS)) platforms"
	@echo "  release        Build release archives + checksums (local)"
	@echo "  install        Install to \$$GOPATH/bin"
	@echo ""
	@echo "Release targets (publish to GitHub via gh CLI):"
	@echo "  github-release              Build + publish (needs VERSION=v1.2.3)"
	@echo "  github-release-dry-run      Build only, don't publish"
	@echo "  github-release-prerelease   Publish as pre-release"
	@echo "  (or run scripts/release.sh directly for full control)"
	@echo ""
	@echo "Test targets:"
	@echo "  test           Run unit tests"
	@echo "  test-coverage  Run tests with coverage report"
	@echo "  smoke          Run integration smoke test (needs PLANE_API_KEY)"
	@echo ""
	@echo "Utility targets:"
	@echo "  clean          Remove built binaries"
	@echo "  fmt            Format code"
	@echo "  vet            Run go vet"
	@echo "  lint           Run fmt + vet"
	@echo "  info           Print build configuration"
	@echo "  help           Print this help"
	@echo ""
	@echo "Cross-compile examples:"
	@echo "  make dist/darwin/arm64/plane-mcp"
	@echo "  make dist/linux/amd64/plane-mcp"
	@echo "  make dist/windows/amd64/plane-mcp.exe"

.DEFAULT_GOAL := help
