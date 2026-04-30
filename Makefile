# Codrax build system
#
# Tree-sitter grammars require CGO. The native `make` target builds the
# current platform's binary; `make static` produces a fully static Linux
# binary.
#
# Notes by host OS:
#   - Linux/macOS: recipes run directly in a POSIX shell.
#   - Windows: recipes run in PowerShell so native `make` works without
#     requiring MSYS bash. `make static` delegates to WSL and uses musl-gcc
#     there, because a real Linux static binary is a Linux toolchain job.

BINARY       := codrax
GO           := go
GOFLAGS      ?=
LDFLAGS      ?=
MUSL_CC      ?= musl-gcc
WSL_GOPROXY  ?= https://proxy.golang.org,direct
WSL_GOSUMDB  ?= sum.golang.org

# CGO_CFLAGS injection silences gcc/clang/musl-gcc warnings from the
# vendored tree-sitter parser packages. Specifically, tree-sitter-lua
# parser.c embeds a literal `"\0"` string at index 254 (parser-
# generator artifact, harmless at runtime) which trips:
#
#   parser.c:254:17: warning: null character(s) preserved in literal
#     254 |   [anon_sym_] = " ",
#         |                 ^
#
# That specific warning comes from libcpp and is NOT addressable via
# `-Wno-null-character` (gcc rejects the flag with "unrecognized
# command-line option"). The only certain suppressor is `-w` which
# silences all warnings for vendored C compilation.
#
# Scope: the project has zero hand-written C — every CGO surface is
# vendored tree-sitter or the cgo runtime. Suppressing all warnings
# in vendored C means we lose nothing we own; real bugs surface in
# Go code via `go vet` and `go build` errors anyway.
#
# `export CGO_CFLAGS` makes every spawned compile recipe (build,
# static, cross-*, test, eval) inherit the flag without per-line
# edits. The append form preserves any user-supplied CGO_CFLAGS so
# an operator chasing a real warning can override via env.
export CGO_CFLAGS := $(CGO_CFLAGS) -w

ifeq ($(OS),Windows_NT)
  HOST_OS := windows
  SHELL := powershell.exe
  .SHELLFLAGS := -NoProfile -Command
  EXEEXT := .exe
  OUT := $(BINARY)$(EXEEXT)
  VERSION_DATE := $(shell powershell -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyyMMdd')")
  GIT_DIRTY := $(shell powershell -NoProfile -Command "if ((git status --porcelain 2>$$null)) { '-dirty' }")
  BUILD_TIME := $(shell powershell -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')")
  WSL_REPO := $(shell powershell -NoProfile -Command "'/mnt/' + (Resolve-Path '.').Path.Substring(0,1).ToLower() + (Resolve-Path '.').Path.Substring(2).Replace('\','/')")
else
  HOST_OS := unix
  EXEEXT :=
  OUT := $(BINARY)
  VERSION_DATE := $(shell date -u '+%Y%m%d')
  GIT_DIRTY := $(shell test -n "$$(git status --porcelain 2>/dev/null)" && echo -dirty)
  BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
endif

# CalVer-style placeholder until we cut a real semver tag: 0.1.<UTC YYYYMMDD>,
# with -dirty when the working tree has uncommitted changes.
VERSION_BASE := 0.1
VERSION := $(VERSION_BASE).$(strip $(VERSION_DATE))$(strip $(GIT_DIRTY))

LD_VERSION := -X github.com/hanchaoqun/codrax/cmd.version=$(VERSION) -X github.com/hanchaoqun/codrax/cmd.buildTime=$(BUILD_TIME)

# Platform detection
GOOS   ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)

.DEFAULT_GOAL := build

# ---------------------------------------------------------------------------
# Default: build for current platform
# ---------------------------------------------------------------------------
.PHONY: build build-native
build: build-native

ifeq ($(HOST_OS),windows)
build-native:
	$$env:CGO_ENABLED='1'; & $(GO) build $(GOFLAGS) -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(OUT) .
else
build-native:
	CGO_ENABLED=1 $(GO) build $(GOFLAGS) -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(OUT) .
endif

# ---------------------------------------------------------------------------
# Static build (fully static Linux binary)
# ---------------------------------------------------------------------------
.PHONY: static static-native

ifeq ($(HOST_OS),windows)
static:
	if (-not (Get-Command wsl -ErrorAction SilentlyContinue)) { Write-Error 'make static on Windows requires WSL.'; exit 1 }
	& wsl bash -lc "set -euo pipefail; command -v $(MUSL_CC) >/dev/null 2>&1 || { echo 'musl-gcc not found in WSL; install musl-tools first.'; exit 1; }; cd '$(WSL_REPO)' && export GOPROXY='$(WSL_GOPROXY)' GOSUMDB='$(WSL_GOSUMDB)' && make static-native"

static-native:
	Write-Error 'static-native is a Linux/WSL-only target. Use ''make static'' from Windows.' -ErrorAction Stop
else
static: static-native

static-native:
	CGO_ENABLED=1 CC=$(MUSL_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags '-linkmode external -extldflags "-static" $(LD_VERSION) $(LDFLAGS)' -o $(BINARY) .
endif

# ---------------------------------------------------------------------------
# Cross-compilation targets
# ---------------------------------------------------------------------------

.PHONY: cross-linux cross-linux-arm64 cross-darwin cross-darwin-arm64 cross-windows

ifeq ($(HOST_OS),windows)
cross-linux:
	if (-not (Get-Command wsl -ErrorAction SilentlyContinue)) { Write-Error 'cross-linux on Windows requires WSL.'; exit 1 }
	New-Item -ItemType Directory -Force dist | Out-Null
	& wsl bash -lc "set -euo pipefail; cd '$(WSL_REPO)' && export GOPROXY='$(WSL_GOPROXY)' GOSUMDB='$(WSL_GOSUMDB)' && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-linux-amd64 ."

cross-linux-arm64:
	if (-not (Get-Command wsl -ErrorAction SilentlyContinue)) { Write-Error 'cross-linux-arm64 on Windows requires WSL plus aarch64-linux-gnu-gcc.'; exit 1 }
	New-Item -ItemType Directory -Force dist | Out-Null
	& wsl bash -lc "set -euo pipefail; command -v aarch64-linux-gnu-gcc >/dev/null 2>&1 || { echo 'aarch64-linux-gnu-gcc not found in WSL.'; exit 1; }; cd '$(WSL_REPO)' && export GOPROXY='$(WSL_GOPROXY)' GOSUMDB='$(WSL_GOSUMDB)' && CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc $(GO) build $(GOFLAGS) -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-linux-arm64 ."

cross-darwin:
	Write-Error 'cross-darwin from Windows is not configured. Use a macOS host or osxcross in a POSIX environment.' -ErrorAction Stop

cross-darwin-arm64:
	Write-Error 'cross-darwin-arm64 from Windows is not configured. Use a macOS host or osxcross in a POSIX environment.' -ErrorAction Stop

cross-windows:
	New-Item -ItemType Directory -Force dist | Out-Null
	$$env:CGO_ENABLED='1'; & $(GO) build $(GOFLAGS) -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-windows-amd64.exe .
else
cross-linux:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-linux-amd64 .

cross-linux-arm64:
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc $(GO) build $(GOFLAGS) -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-linux-arm64 .

cross-darwin:
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-darwin-amd64 .

cross-darwin-arm64:
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-darwin-arm64 .

cross-windows:
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc $(GO) build $(GOFLAGS) -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-windows-amd64.exe .
endif

# ---------------------------------------------------------------------------
# Release
# ---------------------------------------------------------------------------
.PHONY: release

ifeq ($(HOST_OS),windows)
release: clean-dist
	New-Item -ItemType Directory -Force dist | Out-Null
	$$env:CGO_ENABLED='1'; & $(GO) build $(GOFLAGS) -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-windows-amd64.exe .
	& wsl bash -lc "set -euo pipefail; command -v $(MUSL_CC) >/dev/null 2>&1 || { echo 'musl-gcc not found in WSL; install musl-tools first.'; exit 1; }; cd '$(WSL_REPO)' && export GOPROXY='$(WSL_GOPROXY)' GOSUMDB='$(WSL_GOSUMDB)' && CGO_ENABLED=1 CC=$(MUSL_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags '-linkmode external -extldflags `"-static`" $(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-linux-amd64 ."
	Write-Host 'Skipped Darwin artifacts on Windows host.'
	Get-ChildItem dist
else
release: clean-dist
	@mkdir -p dist
	CGO_ENABLED=1 CC=$(MUSL_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags '-linkmode external -extldflags "-static" $(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-linux-amd64 .
	-CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-darwin-amd64 .
	-CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-darwin-arm64 .
	-CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc $(GO) build $(GOFLAGS) -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-windows-amd64.exe .
	@ls -lh dist/
endif

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------
.PHONY: test test-v test-race

ifeq ($(HOST_OS),windows)
test:
	$$env:CGO_ENABLED='1'; & $(GO) test ./...

test-v:
	$$env:CGO_ENABLED='1'; & $(GO) test -v ./...

test-race:
	$$env:CGO_ENABLED='1'; & $(GO) test -race ./...
else
test:
	CGO_ENABLED=1 $(GO) test ./...

test-v:
	CGO_ENABLED=1 $(GO) test -v ./...

# test-race surfaces concurrent-access bugs the regular test pass
# misses (file-lock races, mutex misuse, shared-map writes
# without proper sync). The write-mode hardening sweep added
# several concurrency-sensitive surfaces (BaselineCache mtime
# guard, plan_critic dedup mutex, MutableState.* slot updates
# under load) that should be exercised under the race detector
# at least pre-PR. Run cost: ~4× slower than `make test`; not
# default because most contributor-iteration cycles don't need
# it. CI should run this on every PR.
#
# Pre-existing races in exec_supervisor.waitWithKillTimeout
# (cmd.Wait called concurrently in two goroutines) and the
# grandchild-kill test (strings.Builder shared with os/exec
# stdout-copy goroutine) were fixed in commit 17 — entire
# codebase is now race-clean.
test-race:
	CGO_ENABLED=1 $(GO) test -race ./...
endif

# ---------------------------------------------------------------------------
# kind=patch end-to-end eval suite
#
# Drives the real LLM through plan→apply→verify against five language
# fixtures (C, C++, Go, Java, Python) each carrying a one-line typo
# that breaks compilation. The case files (eval/cases/patch_*_typo.case)
# require:
#   - $PROVIDERS_CONFIG pointing at a providers.yaml with valid LLM
#     credentials (the case shells out to ./codrax)
#   - codrax.yaml (or env equivalents) with write_enabled: true
#
# Each case runs with the harness default N=3 samples; per-run logs
# land under eval/results/<case-id>-<timestamp>/ for inspection. The
# suite is intentionally shell-driven (not `go test`) because real
# LLM calls have no place in `make test`.
#
# Override sample count: `make eval-patch SAMPLES=5`. Run a single
# case via `bash eval/run.sh eval/cases/patch_go_typo.case 3`.
# ---------------------------------------------------------------------------
.PHONY: eval-patch eval-patch-go eval-patch-c eval-patch-cpp eval-patch-java eval-patch-python

SAMPLES ?= 3

ifeq ($(HOST_OS),windows)
eval-patch:
	@echo "eval-patch is unix-only (run.sh requires bash); use WSL or run cases manually"
	@exit 1
else
eval-patch: build
	@for case in eval/cases/patch_*_typo.case; do \
		echo "=== $$case ==="; \
		bash eval/run.sh $$case $(SAMPLES) || true; \
	done
	@echo "Done. Per-case results under eval/results/."

eval-patch-go: build
	bash eval/run.sh eval/cases/patch_go_typo.case $(SAMPLES)

eval-patch-c: build
	bash eval/run.sh eval/cases/patch_c_typo.case $(SAMPLES)

eval-patch-cpp: build
	bash eval/run.sh eval/cases/patch_cpp_typo.case $(SAMPLES)

eval-patch-java: build
	bash eval/run.sh eval/cases/patch_java_typo.case $(SAMPLES)

eval-patch-python: build
	bash eval/run.sh eval/cases/patch_python_typo.case $(SAMPLES)
endif

# ---------------------------------------------------------------------------
# Clean
# ---------------------------------------------------------------------------
.PHONY: clean clean-dist

ifeq ($(HOST_OS),windows)
clean:
	Remove-Item -Force -ErrorAction SilentlyContinue $(BINARY), $(BINARY).exe, $(BINARY)-staticish.exe
	& $(GO) clean -cache

clean-dist:
	Remove-Item -Force -Recurse -ErrorAction SilentlyContinue dist
else
clean:
	rm -f $(BINARY) $(BINARY).exe
	$(GO) clean -cache

clean-dist:
	rm -rf dist/
endif

# ---------------------------------------------------------------------------
# Verify
# ---------------------------------------------------------------------------

# Verify targets
.PHONY: verify-static verify-windows-runtime

ifeq ($(HOST_OS),windows)
verify-static: static
	& wsl bash -lc "cd '$(WSL_REPO)' && file $(BINARY) && ldd $(BINARY) 2>&1 || true"

verify-windows-runtime: build
	$$dlls = (& objdump -p $(OUT) | Select-String 'DLL Name' | Out-String); Write-Host $$dlls; if ($$dlls -match 'libgcc|libstdc\+\+|libwinpthread') { Write-Error 'Unexpected MinGW runtime DLL dependency detected.' -ErrorAction Stop } else { Write-Host 'OK: no MinGW runtime DLL dependencies detected.' }
else
verify-static: static
	@file $(BINARY) | grep -q "statically linked" && \
		echo "OK: statically linked" || \
		echo "FAIL: NOT statically linked"
	@ldd $(BINARY) 2>&1 | grep -q "not a dynamic" && \
		echo "OK: no dynamic dependencies" || \
		ldd $(BINARY)

verify-windows-runtime:
	@echo "verify-windows-runtime is intended for Windows-hosted builds."
endif

# ---------------------------------------------------------------------------
# Info
# ---------------------------------------------------------------------------
.PHONY: info

ifeq ($(HOST_OS),windows)
info:
	Write-Host "HOST_OS: $(HOST_OS)"
	Write-Host "GOOS:    $(GOOS)"
	Write-Host "GOARCH:  $(GOARCH)"
	Write-Host "VERSION: $(VERSION)"
	& $(GO) version
	Write-Host "CC:"
	try { & gcc --version | Select-Object -First 1 } catch { Write-Host 'gcc not found' }
else
info:
	@echo "HOST_OS: $(HOST_OS)"
	@echo "GOOS:    $(GOOS)"
	@echo "GOARCH:  $(GOARCH)"
	@echo "VERSION: $(VERSION)"
	@echo "GO:      $(shell $(GO) version)"
	@echo "CC:      $(shell $(CC) --version 2>/dev/null | head -1 || echo 'not found')"
endif

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------
.PHONY: help
help:
	@echo "Codrax Build System"
	@echo ""
	@echo "Targets:"
	@echo "  build              Build for current platform (default)"
	@echo "  static             Fully static Linux binary"
	@echo "  test               Run all tests"
	@echo "  test-v             Run all tests (verbose)"
	@echo "  test-race          Run all tests with the Go race detector (slower; required pre-PR for concurrency-sensitive changes)"
	@echo "  cross-linux        Cross-compile for Linux amd64"
	@echo "  cross-linux-arm64  Cross-compile for Linux arm64"
	@echo "  cross-darwin       Cross-compile for macOS amd64"
	@echo "  cross-darwin-arm64 Cross-compile for macOS arm64 (Apple Silicon)"
	@echo "  cross-windows      Cross-compile for Windows amd64"
	@echo "  release            Build release artifacts into dist/"
	@echo "  verify-static      Verify Linux static artifact"
	@echo "  verify-windows-runtime Verify Windows binary avoids MinGW runtime DLLs"
	@echo "  clean              Remove build artifacts"
	@echo "  info               Show build environment"
	@echo "  help               Show this help"
	@echo ""
	@echo "Windows notes:"
	@echo "  - Native build/test use PowerShell recipes."
	@echo "  - make static delegates to WSL and requires musl-gcc there."
