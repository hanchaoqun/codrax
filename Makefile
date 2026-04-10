# Codrax build system
#
# Tree-sitter grammars require CGO. For fully static binaries on Linux,
# we link against musl via musl-gcc.
#
# Prerequisites:
#   Linux (Debian/Ubuntu): sudo apt-get install gcc musl-tools
#   Linux (Alpine):        apk add build-base musl-dev
#   macOS:                 xcode-select --install  (or brew install gcc)
#   Windows:               install mingw-w64 via MSYS2 or scoop
#
# Usage:
#   make                 — build for current platform
#   make static          — fully static musl-linked binary (Linux only)
#   make test            — run all tests
#   make release         — build release binaries for all platforms
#   make cross-linux     — cross-compile for Linux from any host
#   make cross-darwin    — cross-compile for macOS from any host
#   make cross-windows   — cross-compile for Windows from any host

BINARY     := codrax
GO         := go
GOFLAGS    ?=
LDFLAGS    ?=
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LD_VERSION := -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)

# Platform detection
GOOS       ?= $(shell $(GO) env GOOS)
GOARCH     ?= $(shell $(GO) env GOARCH)

# musl-gcc for static Linux builds
MUSL_CC    ?= musl-gcc

# Output binary name with platform suffix for cross builds
ifeq ($(GOOS),windows)
  OUT := $(BINARY).exe
else
  OUT := $(BINARY)
endif

# ---------------------------------------------------------------------------
# Default: build for current platform
# ---------------------------------------------------------------------------
.PHONY: build
build:
	CGO_ENABLED=1 $(GO) build $(GOFLAGS) \
		-ldflags '$(LD_VERSION) $(LDFLAGS)' \
		-o $(OUT) .

# ---------------------------------------------------------------------------
# Static build (Linux only, musl libc)
# ---------------------------------------------------------------------------
.PHONY: static
static:
	CGO_ENABLED=1 CC=$(MUSL_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) \
		-ldflags '-linkmode external -extldflags "-static" $(LD_VERSION) $(LDFLAGS)' \
		-o $(BINARY) .

# ---------------------------------------------------------------------------
# Cross-compilation targets
# ---------------------------------------------------------------------------

# Linux amd64 (dynamic)
.PHONY: cross-linux
cross-linux:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) \
		-ldflags '$(LD_VERSION) $(LDFLAGS)' \
		-o dist/$(BINARY)-linux-amd64 .

# Linux arm64 (dynamic)
.PHONY: cross-linux-arm64
cross-linux-arm64:
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc $(GO) build $(GOFLAGS) \
		-ldflags '$(LD_VERSION) $(LDFLAGS)' \
		-o dist/$(BINARY)-linux-arm64 .

# macOS amd64
.PHONY: cross-darwin
cross-darwin:
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) \
		-ldflags '$(LD_VERSION) $(LDFLAGS)' \
		-o dist/$(BINARY)-darwin-amd64 .

# macOS arm64 (Apple Silicon)
.PHONY: cross-darwin-arm64
cross-darwin-arm64:
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) \
		-ldflags '$(LD_VERSION) $(LDFLAGS)' \
		-o dist/$(BINARY)-darwin-arm64 .

# Windows amd64
.PHONY: cross-windows
cross-windows:
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc $(GO) build $(GOFLAGS) \
		-ldflags '$(LD_VERSION) $(LDFLAGS)' \
		-o dist/$(BINARY)-windows-amd64.exe .

# ---------------------------------------------------------------------------
# Release: build all supported platforms
# ---------------------------------------------------------------------------
.PHONY: release
release: clean-dist
	@mkdir -p dist
	@echo "=== Building Linux amd64 (static) ==="
	CGO_ENABLED=1 CC=$(MUSL_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) \
		-ldflags '-linkmode external -extldflags "-static" $(LD_VERSION) $(LDFLAGS)' \
		-o dist/$(BINARY)-linux-amd64 .
	@echo "=== Building macOS amd64 ==="
	-CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) \
		-ldflags '$(LD_VERSION) $(LDFLAGS)' \
		-o dist/$(BINARY)-darwin-amd64 .
	@echo "=== Building macOS arm64 ==="
	-CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) \
		-ldflags '$(LD_VERSION) $(LDFLAGS)' \
		-o dist/$(BINARY)-darwin-arm64 .
	@echo "=== Building Windows amd64 ==="
	-CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc $(GO) build $(GOFLAGS) \
		-ldflags '$(LD_VERSION) $(LDFLAGS)' \
		-o dist/$(BINARY)-windows-amd64.exe .
	@echo "=== Release build complete ==="
	@ls -lh dist/

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------
.PHONY: test
test:
	CGO_ENABLED=1 $(GO) test ./...

.PHONY: test-v
test-v:
	CGO_ENABLED=1 $(GO) test -v ./...

# ---------------------------------------------------------------------------
# Clean
# ---------------------------------------------------------------------------
.PHONY: clean
clean:
	rm -f $(BINARY) $(BINARY).exe
	$(GO) clean -cache

.PHONY: clean-dist
clean-dist:
	rm -rf dist/

# ---------------------------------------------------------------------------
# Verify
# ---------------------------------------------------------------------------

# Verify the binary is statically linked (Linux only)
.PHONY: verify-static
verify-static: static
	@file $(BINARY) | grep -q "statically linked" && \
		echo "OK: statically linked" || \
		echo "FAIL: NOT statically linked"
	@ldd $(BINARY) 2>&1 | grep -q "not a dynamic" && \
		echo "OK: no dynamic dependencies" || \
		ldd $(BINARY)

# Show build info
.PHONY: info
info:
	@echo "GOOS:    $(GOOS)"
	@echo "GOARCH:  $(GOARCH)"
	@echo "VERSION: $(VERSION)"
	@echo "GO:      $(shell $(GO) version)"
	@echo "CC:      $(shell $(CC) --version 2>/dev/null | head -1 || echo 'not found')"

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------
.PHONY: help
help:
	@echo "Codrax Build System"
	@echo ""
	@echo "Targets:"
	@echo "  build              Build for current platform (default)"
	@echo "  static             Fully static Linux binary (musl, amd64)"
	@echo "  test               Run all tests"
	@echo "  test-v             Run all tests (verbose)"
	@echo "  cross-linux        Cross-compile for Linux amd64"
	@echo "  cross-linux-arm64  Cross-compile for Linux arm64"
	@echo "  cross-darwin       Cross-compile for macOS amd64"
	@echo "  cross-darwin-arm64 Cross-compile for macOS arm64 (Apple Silicon)"
	@echo "  cross-windows      Cross-compile for Windows amd64"
	@echo "  release            Build all platforms into dist/"
	@echo "  verify-static      Verify binary is statically linked"
	@echo "  clean              Remove build artifacts"
	@echo "  info               Show build environment"
	@echo "  help               Show this help"
	@echo ""
	@echo "Cross-compilation prerequisites:"
	@echo "  Linux arm64:  apt-get install gcc-aarch64-linux-gnu"
	@echo "  Windows:      apt-get install gcc-mingw-w64-x86-64"
	@echo "  Static Linux: apt-get install musl-tools"
	@echo "  macOS:        requires macOS host or osxcross"
