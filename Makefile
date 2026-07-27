# Codrax build system
#
# Tree-sitter grammars require CGO. The native `make` target builds the
# current supported platform binary; Windows build/lowmem deliberately target
# windows/amd64, while `make static` produces a fully static Linux binary.
#
# Notes by host OS:
#   - Linux:   recipes run directly in a POSIX shell. `make static` produces
#              a fully static Linux Codrax parent natively and embeds the
#              independently executed Linux trace_streamer child. The parent
#              is libc-family agnostic; the child still requires glibc >= 2.34.
#   - macOS:   recipes run directly in a POSIX shell. `make` (default), tests,
#              `make cross-darwin*`, and `make eval-patch*` all work with the
#              Xcode Command Line Tools' clang. `make static` fail-louds
#              because Apple does not ship a static libc; for a real Linux
#              static binary install Homebrew's FiloSottile/musl-cross
#              tap and pass `MUSL_CC=x86_64-linux-musl-gcc`. `make release`
#              builds the two darwin legs natively, a distinctly named
#              Linux fully-static-slim leg when the configured static compiler
#              is available, and soft-skips
#              the standard Linux glibc / Windows legs unless their cross
#              compilers are on PATH.
#   - Windows: recipes run in PowerShell so native `make` works without
#              requiring MSYS bash. `make static` delegates to WSL and uses
#              musl-gcc there, because a real Linux static binary is a Linux
#              toolchain job.

override BINARY := codrax
# Build and verification must share the real Go tool authority. Without
# override, `make release GO=true` turns both commands into no-ops and can
# advance to dist cleanup/build bookkeeping without executing either gate.
override GO  := go
# release-strict delegates to the ordinary release recipe. Keep that recursive
# authority real as well: `MAKE=true` must not turn the rebuild into a no-op
# followed by stale-dist existence checks.
override MAKE := make
GOFLAGS      ?=
LDFLAGS      ?=
MUSL_CC      ?= musl-gcc
WSL_GOPROXY  ?= https://proxy.golang.org,direct
WSL_GOSUMDB  ?= sum.golang.org

override DIST_LINUX_AMD64             := dist/$(BINARY)-linux-amd64
override DIST_LINUX_AMD64_STATIC_SLIM := dist/$(BINARY)-linux-amd64-static-slim
override DIST_WINDOWS_AMD64           := dist/$(BINARY)-windows-amd64.exe
override STATIC_SLIM_OUT              := $(BINARY)-static-slim

# Standard/default artifacts use one non-semantic tag solely as an
# unforgeable tag authority. Declare it before the static tag checks so the
# external-only lane can reject an attempted dual identity.
override STANDARD_EMBEDDED_BUILD_TAGS := codrax_embedded_streamer_release

# Ordinary supported-platform builds embed the platform-matched
# trace_streamer payload by default. A fully-static Codrax parent can safely
# carry the Linux payload as opaque bytes because the extracted child is a
# separate executable; the child still owns its glibc >= 2.34 runtime
# requirement. `make static` therefore keeps the embedded tier, while only
# explicitly named static-slim targets opt out. Add project-specific static
# build tags through STATIC_EXTRA_TAGS; neither reserved distribution identity
# may be injected. The old fully-overridable STATIC_TAGS variable is rejected
# fail-loud because it could silently flip either artifact identity.
ifneq ($(origin STATIC_TAGS),undefined)
  $(error STATIC_TAGS is unsafe and no longer supported; use STATIC_EXTRA_TAGS=<comma-separated-extra-tags> (the target fixes embedded vs slim identity))
endif
override comma    := ,
STATIC_EXTRA_TAGS ?=
override STATIC_EMBEDDED_BUILD_TAGS := $(STANDARD_EMBEDDED_BUILD_TAGS)$(if $(strip $(STATIC_EXTRA_TAGS)),$(comma)$(strip $(STATIC_EXTRA_TAGS)))
override STATIC_SLIM_BUILD_TAGS := slim_streamer$(if $(strip $(STATIC_EXTRA_TAGS)),$(comma)$(strip $(STATIC_EXTRA_TAGS)))
ifneq ($(filter $(STANDARD_EMBEDDED_BUILD_TAGS) slim_streamer,$(subst $(comma), ,$(strip $(STATIC_EXTRA_TAGS)))),)
  $(error STATIC_EXTRA_TAGS must not contain reserved build identity tags $(STANDARD_EMBEDDED_BUILD_TAGS) or slim_streamer)
endif
# The tag authority is assigned with override so a Make command-line variable
# cannot replace it with slim_streamer. GOFLAGS is isolated from persistent
# `go env -w` state with GOENV=off, and any caller flags are placed before the
# final -tags value. Artifact verification is always compiled for the host,
# never for an ambient cross-target tuple.
override STREAMER_ARTIFACT_VERIFIER := $(GO) run ./internal/releaseartifact/cmd/verify
override STREAMER_COMMERCIAL_RELEASE_VERIFIER := $(GO) run ./internal/releaseartifact/cmd/verifycommercial
override POSIX_GO_BUILD_ENV := GOENV=off GOFLAGS=
override POSIX_GO_VERIFY_ENV := GOENV=off GOOS= GOARCH= CGO_ENABLED=0 GOFLAGS=

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

# OS selects the recipe language before HOST_OS/HOST_KIND can be fixed. It is
# an environment fact on native Windows, not a cross-build switch; explicit
# cross targets must be used instead of a command-line OS override.
ifeq ($(origin OS),command line)
  $(error OS is a host-detection authority and must not be overridden; use an explicit cross-* target)
endif

ifeq ($(OS),Windows_NT)
  override HOST_OS := windows
  override HOST_KIND := windows
  # Executed inside WSL for the standard Linux release leg.
  LINUX_GLIBC_CC ?= gcc
  override SHELL := powershell.exe
  override .SHELLFLAGS := -NoProfile -Command
  EXEEXT := .exe
  OUT := $(BINARY)$(EXEEXT)
  VERSION_DATE := $(shell powershell -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyyMMdd')")
  # Guard on .git before invoking git: zip-download source trees (the
  # customer build path) have no repository and bare `git status` prints
  # "fatal: not a git repository" noise before the version fallback kicks
  # in. Test-Path skips the subprocess entirely in that case.
  #
  # No `$` may appear in this command: SHELL is powershell.exe, so the
  # $(shell) invocation itself passes through an OUTER PowerShell that
  # expands `$null` (even inside the inner double-quoted -Command string)
  # to an empty token — the inner parser then saw `2>)` and its ERROR TEXT
  # was captured into GIT_DIRTY, corrupting -ldflags (customer witness,
  # 2026-07-09). The Test-Path guard alone is sufficient; no stderr
  # redirect needed.
  GIT_DIRTY := $(shell powershell -NoProfile -Command "if (Test-Path '.git') { if ((git status --porcelain)) { '-dirty' } }")
  GIT_REVISION := $(shell powershell -NoProfile -Command "if (Test-Path '.git') { git rev-parse --short=12 HEAD } else { 'unknown' }")
  BUILD_TIME := $(shell powershell -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')")
  override WSL_REPO := $(shell powershell -NoProfile -Command "'/mnt/' + (Resolve-Path '.').Path.Substring(0,1).ToLower() + (Resolve-Path '.').Path.Substring(2).Replace('\','/')")
  override WINDOWS_GO_ENV := if (-not $$env:GOROOT) { $$goBin = (Get-Command $(GO) -ErrorAction SilentlyContinue).Source; if ($$goBin) { $$goRoot = Join-Path (Split-Path (Split-Path $$goBin -Parent) -Parent) 'lib\go'; if (Test-Path $$goRoot) { $$env:GOROOT = $$goRoot } } }; $$env:CGO_ENABLED='1';
  # Native build/lowmem and every artifact named windows-amd64 consume this
  # one authority. Other architectures remain available via explicit `go
  # build`, where the runtime reports the structured platform gap.
  override WINDOWS_AMD64_GO_ENV := $(WINDOWS_GO_ENV) $$env:GOENV='off'; $$env:GOFLAGS=''; $$env:GOOS='windows'; $$env:GOARCH='amd64';
  # Verifier compilation is host-native even when make inherits a target
  # tuple. Each recipe runs in a fresh PowerShell, so GOROOT discovery must
  # be part of this authority rather than an earlier build line.
  override WINDOWS_VERIFY_GO_ENV := $(WINDOWS_GO_ENV) $$env:GOENV='off'; $$env:GOFLAGS=''; $$env:GOOS=''; $$env:GOARCH=''; $$env:CGO_ENABLED='0';
else
  override SHELL := /bin/sh
  override .SHELLFLAGS := -c
  override HOST_OS := unix
  # HOST_KIND splits unix into darwin vs linux so the static / release
  # / verify-static recipes can take macOS-specific branches without
  # duplicating the whole non-Windows block. Anything that is not
  # macOS (Linux, BSD, …) routes through the linux branch since that
  # is the historical default.
  ifeq ($(shell uname -s 2>/dev/null),Darwin)
    override HOST_KIND := darwin
    # Apple clang is not a Linux/glibc cross compiler. Operators may install
    # and override this explicit tool; release otherwise skips that leg.
    LINUX_GLIBC_CC ?= x86_64-linux-gnu-gcc
  else
    override HOST_KIND := linux
    LINUX_GLIBC_CC ?= gcc
  endif
  EXEEXT :=
  OUT := $(BINARY)
  VERSION_DATE := $(shell date -u '+%Y%m%d')
  GIT_DIRTY := $(shell test -n "$$(git status --porcelain 2>/dev/null)" && echo -dirty)
  GIT_REVISION := $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
  BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
endif

# CalVer-style placeholder until we cut a real semver tag: 0.1.<UTC YYYYMMDD>,
# with -dirty when the working tree has uncommitted changes.
VERSION_BASE := 0.1
VERSION := $(VERSION_BASE).$(strip $(VERSION_DATE))$(strip $(GIT_DIRTY))
BUILD_REVISION := $(strip $(GIT_REVISION))$(strip $(GIT_DIRTY))

LD_VERSION := -X github.com/hanchaoqun/codrax/cmd.version=$(VERSION) -X github.com/hanchaoqun/codrax/cmd.buildTime=$(BUILD_TIME) -X github.com/hanchaoqun/codrax/cmd.buildRevision=$(BUILD_REVISION)

# Platform detection. Ignore persistent GOENV target overrides so a standard
# host build cannot accidentally inherit a previous cross-compilation tuple.
# Explicit make/environment GOOS/GOARCH values still win through ?=.
ifeq ($(HOST_OS),windows)
  GOOS   ?= windows
  GOARCH ?= amd64
else
  GOOS   ?= $(shell GOENV=off GOOS= GOARCH= $(GO) env GOOS)
  GOARCH ?= $(shell GOENV=off GOOS= GOARCH= $(GO) env GOARCH)
endif

override NATIVE_STREAMER_PAYLOAD := none
override NATIVE_LINUX_RUNTIME_ARG :=
ifeq ($(HOST_OS),windows)
  override NATIVE_STREAMER_PAYLOAD := windows-amd64
else ifeq ($(HOST_KIND),linux)
  ifeq ($(GOOS),linux)
    ifeq ($(GOARCH),amd64)
      override NATIVE_STREAMER_PAYLOAD := linux-amd64
      override NATIVE_LINUX_RUNTIME_ARG := --linux-runtime glibc
    endif
  endif
endif

.DEFAULT_GOAL := build

# ---------------------------------------------------------------------------
# Default: build for current platform
# ---------------------------------------------------------------------------
.PHONY: build build-native
build: build-native

ifeq ($(HOST_OS),windows)
# On failure, point straight at the low-memory path: commit-limited hosts
# (customer witness 2026-07-09, VirtualAlloc errno=1455 / "cannot allocate
# memory" during parallel package compiles) need `make lowmem`, and nobody
# reads docs mid-failure. Hint text stays ASCII: GBK consoles mangle UTF-8.
build-native:
	$(WINDOWS_AMD64_GO_ENV) & $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(OUT) .; if (-not $$?) { Write-Host ''; Write-Host 'BUILD FAILED. If the error above mentions out of memory / VirtualAlloc errno=1455: run ''make lowmem'' (serialized low-memory build). See docs/design/revisit_acceptance_pack_20260709.md for details.'; exit 1 }
	$(WINDOWS_VERIFY_GO_ENV) & $(STREAMER_ARTIFACT_VERIFIER) --artifact $(OUT) --repo . --goos windows --goarch amd64 --cgo 1 --payload windows-amd64 --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer; if (-not $$?) { Remove-Item -Force -ErrorAction SilentlyContinue $(OUT); exit 1 }
else
build-native:
	$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(OUT) .
	@$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact $(OUT) --repo . --goos $(GOOS) --goarch $(GOARCH) --cgo 1 --payload $(NATIVE_STREAMER_PAYLOAD) $(NATIVE_LINUX_RUNTIME_ARG) --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer || { rm -f '$(OUT)'; exit 1; }
endif

# ---------------------------------------------------------------------------
# Low-memory build for RAM/pagefile-constrained hosts.
#
# Witness (customer, Windows, 2026-07-09): compiling internal/types ran the
# machine out of commit charge — "VirtualAlloc … failed with errno=1455"
# (ERROR_COMMITMENT_LIMIT, pagefile too small). Peak memory during `go build`
# is (parallel package compiles) x (per-compile heap); internal/types alone
# is the largest unit. This target trades wall-clock for peak memory:
#   -p 1              one package compile at a time
#   -gcflags=all=-c=1 serialize the compiler backend within each package
#   GOGC=50           make each compiler process collect twice as eagerly
# If it still fails with errno=1455, enlarge the Windows pagefile (System
# Properties -> Advanced -> Performance -> Virtual memory) or set
# GOMEMLIMIT (e.g. $env:GOMEMLIMIT='2GiB') before retrying.
# ---------------------------------------------------------------------------
.PHONY: lowmem

ifeq ($(HOST_OS),windows)
lowmem:
	$(WINDOWS_AMD64_GO_ENV) $$env:GOGC='50'; & $(GO) build -p 1 -gcflags=all=-c=1 $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(OUT) .; if (-not $$?) { Write-Host ''; Write-Host 'LOWMEM BUILD FAILED. If still out-of-memory (errno=1455): enlarge the Windows pagefile (System Properties -> Advanced -> Performance -> Virtual memory; set system-managed or >= 16 GB), or set $$env:GOMEMLIMIT=''2GiB'' and retry.'; exit 1 }
	$(WINDOWS_VERIFY_GO_ENV) & $(STREAMER_ARTIFACT_VERIFIER) --artifact $(OUT) --repo . --goos windows --goarch amd64 --cgo 1 --payload windows-amd64 --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer; if (-not $$?) { Remove-Item -Force -ErrorAction SilentlyContinue $(OUT); exit 1 }
else
lowmem:
	$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 GOGC=50 $(GO) build -p 1 -gcflags=all=-c=1 $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(OUT) .
	@$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact $(OUT) --repo . --goos $(GOOS) --goarch $(GOARCH) --cgo 1 --payload $(NATIVE_STREAMER_PAYLOAD) $(NATIVE_LINUX_RUNTIME_ARG) --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer || { rm -f '$(OUT)'; exit 1; }
endif

# ---------------------------------------------------------------------------
# Static builds (fully static Linux parent; embedded by default)
# ---------------------------------------------------------------------------
.PHONY: static static-native static-slim static-slim-native

ifeq ($(HOST_OS),windows)
static:
	if (-not (Get-Command wsl -ErrorAction SilentlyContinue)) { Write-Error 'make static on Windows requires WSL.'; exit 1 }
	& wsl bash -lc "set -euo pipefail; command -v $(MUSL_CC) >/dev/null 2>&1 || { echo 'musl-gcc not found in WSL; install musl-tools first.'; exit 1; }; cd '$(WSL_REPO)' && export GOENV=off GOFLAGS='' GOPROXY='$(WSL_GOPROXY)' GOSUMDB='$(WSL_GOSUMDB)' && make static-native"

static-slim:
	if (-not (Get-Command wsl -ErrorAction SilentlyContinue)) { Write-Error 'make static-slim on Windows requires WSL.'; exit 1 }
	& wsl bash -lc "set -euo pipefail; command -v $(MUSL_CC) >/dev/null 2>&1 || { echo 'musl-gcc not found in WSL; install musl-tools first.'; exit 1; }; cd '$(WSL_REPO)' && export GOENV=off GOFLAGS='' GOPROXY='$(WSL_GOPROXY)' GOSUMDB='$(WSL_GOSUMDB)' && make static-slim-native"

static-native:
	Write-Error 'static-native is a Linux/WSL-only target. Use ''make static'' from Windows.' -ErrorAction Stop

static-slim-native:
	Write-Error 'static-slim-native is a Linux/WSL-only target. Use ''make static-slim'' from Windows.' -ErrorAction Stop
else ifeq ($(HOST_KIND),darwin)
# macOS host: a fully static Linux binary needs musl-gcc + GNU
# binutils, neither of which is in a default macOS toolchain (Apple
# also does not ship libc.a, so a "macOS-static" output isn't
# meaningful in the same sense). Fail loud with a path forward
# instead of half-completing through clang and producing a broken
# artifact.
static:
	@echo "make static targets a fully static Linux binary; macOS hosts cannot build it natively." >&2
	@echo "Options:" >&2
	@echo "  - Native macOS binary: 'make' (default), or 'make cross-darwin' / 'make cross-darwin-arm64'" >&2
	@echo "  - Real Linux static via Homebrew musl-cross:" >&2
	@echo "      brew install FiloSottile/musl-cross/musl-cross" >&2
	@echo "      make static MUSL_CC=x86_64-linux-musl-gcc" >&2
	@echo "  - Or run 'make static' inside a Linux container / VM." >&2
	@exit 1

static-native:
	@echo "static-native is Linux-only; on macOS run 'make' for the native binary, or see 'make static' for cross-static guidance." >&2
	@exit 1

static-slim:
	@echo "make static-slim targets a fully static Linux external-only binary; macOS hosts cannot build it natively." >&2
	@echo "Run it inside a Linux container/VM or use an explicit musl cross-build environment." >&2
	@exit 1

static-slim-native:
	@echo "static-slim-native is Linux-only; run it inside Linux/WSL." >&2
	@exit 1
else
static: static-native
static-slim: static-slim-native

static-native:
	$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 CC=$(MUSL_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -tags '$(STATIC_EMBEDDED_BUILD_TAGS)' -ldflags '-linkmode external -extldflags "-static" $(LD_VERSION) $(LDFLAGS)' -o $(BINARY) .
	@$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact $(BINARY) --repo . --goos linux --goarch amd64 --cgo 1 --payload linux-amd64 --linux-runtime static --require-tags '$(STATIC_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer || { rm -f '$(BINARY)'; exit 1; }

static-slim-native:
	$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 CC=$(MUSL_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -tags '$(STATIC_SLIM_BUILD_TAGS)' -ldflags '-linkmode external -extldflags "-static" $(LD_VERSION) $(LDFLAGS)' -o $(STATIC_SLIM_OUT) .
	@$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact $(STATIC_SLIM_OUT) --repo . --goos linux --goarch amd64 --cgo 1 --payload none --linux-runtime static --require-tags '$(STATIC_SLIM_BUILD_TAGS)' --forbid-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' || { rm -f '$(STATIC_SLIM_OUT)'; exit 1; }
endif

# ---------------------------------------------------------------------------
# Cross-compilation targets
# ---------------------------------------------------------------------------

.PHONY: cross-linux cross-linux-arm64 cross-darwin cross-darwin-arm64 cross-windows

ifeq ($(HOST_OS),windows)
cross-linux:
	if (-not (Get-Command wsl -ErrorAction SilentlyContinue)) { Write-Error 'cross-linux on Windows requires WSL.'; exit 1 }
	New-Item -ItemType Directory -Force dist | Out-Null
	& wsl bash -lc "set -euo pipefail; command -v $(LINUX_GLIBC_CC) >/dev/null 2>&1 || { echo '$(LINUX_GLIBC_CC) not found in WSL.'; exit 1; }; cd '$(WSL_REPO)' && export GOENV=off GOFLAGS='' GOPROXY='$(WSL_GOPROXY)' GOSUMDB='$(WSL_GOSUMDB)' && CGO_ENABLED=1 CC=$(LINUX_GLIBC_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(DIST_LINUX_AMD64) ."
	$(WINDOWS_VERIFY_GO_ENV) & $(STREAMER_ARTIFACT_VERIFIER) --artifact $(DIST_LINUX_AMD64) --repo . --goos linux --goarch amd64 --cgo 1 --payload linux-amd64 --linux-runtime glibc --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer; if (-not $$?) { Remove-Item -Force -ErrorAction SilentlyContinue $(DIST_LINUX_AMD64); exit 1 }

cross-linux-arm64:
	if (-not (Get-Command wsl -ErrorAction SilentlyContinue)) { Write-Error 'cross-linux-arm64 on Windows requires WSL plus aarch64-linux-gnu-gcc.'; exit 1 }
	New-Item -ItemType Directory -Force dist | Out-Null
	& wsl bash -lc "set -euo pipefail; command -v aarch64-linux-gnu-gcc >/dev/null 2>&1 || { echo 'aarch64-linux-gnu-gcc not found in WSL.'; exit 1; }; cd '$(WSL_REPO)' && export GOENV=off GOFLAGS='' GOPROXY='$(WSL_GOPROXY)' GOSUMDB='$(WSL_GOSUMDB)' && CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-linux-arm64 ."
	$(WINDOWS_VERIFY_GO_ENV) & $(STREAMER_ARTIFACT_VERIFIER) --artifact dist/$(BINARY)-linux-arm64 --repo . --goos linux --goarch arm64 --cgo 1 --payload none --linux-runtime glibc --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer; if (-not $$?) { Remove-Item -Force -ErrorAction SilentlyContinue dist/$(BINARY)-linux-arm64; exit 1 }

cross-darwin:
	Write-Error 'cross-darwin from Windows is not configured. Use a macOS host or osxcross in a POSIX environment.' -ErrorAction Stop

cross-darwin-arm64:
	Write-Error 'cross-darwin-arm64 from Windows is not configured. Use a macOS host or osxcross in a POSIX environment.' -ErrorAction Stop

cross-windows:
	New-Item -ItemType Directory -Force dist | Out-Null
	$(WINDOWS_AMD64_GO_ENV) & $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(DIST_WINDOWS_AMD64) .
	$(WINDOWS_VERIFY_GO_ENV) & $(STREAMER_ARTIFACT_VERIFIER) --artifact $(DIST_WINDOWS_AMD64) --repo . --goos windows --goarch amd64 --cgo 1 --payload windows-amd64 --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer; if (-not $$?) { Remove-Item -Force -ErrorAction SilentlyContinue $(DIST_WINDOWS_AMD64); exit 1 }
else
cross-linux:
	@mkdir -p dist
	$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 CC=$(LINUX_GLIBC_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(DIST_LINUX_AMD64) .
	@$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact $(DIST_LINUX_AMD64) --repo . --goos linux --goarch amd64 --cgo 1 --payload linux-amd64 --linux-runtime glibc --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer || { rm -f '$(DIST_LINUX_AMD64)'; exit 1; }

cross-linux-arm64:
	@mkdir -p dist
	$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-linux-arm64 .
	@$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact dist/$(BINARY)-linux-arm64 --repo . --goos linux --goarch arm64 --cgo 1 --payload none --linux-runtime glibc --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer || { rm -f 'dist/$(BINARY)-linux-arm64'; exit 1; }

cross-darwin:
	@mkdir -p dist
	$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-darwin-amd64 .
	@$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact dist/$(BINARY)-darwin-amd64 --repo . --goos darwin --goarch amd64 --cgo 1 --payload none --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer || { rm -f 'dist/$(BINARY)-darwin-amd64'; exit 1; }

cross-darwin-arm64:
	@mkdir -p dist
	$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-darwin-arm64 .
	@$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact dist/$(BINARY)-darwin-arm64 --repo . --goos darwin --goarch arm64 --cgo 1 --payload none --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer || { rm -f 'dist/$(BINARY)-darwin-arm64'; exit 1; }

cross-windows:
	@mkdir -p dist
	$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(DIST_WINDOWS_AMD64) .
	@$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact $(DIST_WINDOWS_AMD64) --repo . --goos windows --goarch amd64 --cgo 1 --payload windows-amd64 --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer || { rm -f '$(DIST_WINDOWS_AMD64)'; exit 1; }
endif

# ---------------------------------------------------------------------------
# Release
# ---------------------------------------------------------------------------
.PHONY: release release-strict release-clean-dist verify-trace-streamer-commercial-release

# GNU Make's -i/--ignore-errors changes a failed prerequisite into apparent
# success and would otherwise let a blocked formal release continue into dist
# cleanup/build. Reject that process-level semantic before any recipe runs.
# A command-line MAKEFLAGS assignment is rejected as well because it can hide
# an active -i bit from the makefile (`make -i release MAKEFLAGS=`).
ifeq ($(origin MAKECMDGOALS),command line)
  $(error MAKECMDGOALS is an automatic authority and must not be overridden)
endif
override FORMAL_RELEASE_GOALS := release release-strict release-clean-dist verify-trace-streamer-commercial-release
ifneq ($(strip $(filter $(FORMAL_RELEASE_GOALS),$(MAKECMDGOALS))),)
  ifeq ($(origin MAKEFLAGS),command line)
    $(error formal release targets do not allow a command-line MAKEFLAGS override)
  endif
  override RELEASE_MAKE_OPTION_WORDS := $(strip $(foreach option,$(MAKEFLAGS),$(if $(findstring =,$(option)),,$(option))))
  override RELEASE_MAKE_SHORT_FLAGS := $(firstword $(filter-out --%,$(RELEASE_MAKE_OPTION_WORDS)))
  ifneq ($(filter --ignore-errors,$(RELEASE_MAKE_OPTION_WORDS)),)
    $(error formal release targets reject make -i/--ignore-errors)
  endif
  ifneq ($(findstring i,$(RELEASE_MAKE_SHORT_FLAGS)),)
    $(error formal release targets reject make -i/--ignore-errors)
  endif
  ifneq ($(filter --just-print --dry-run --recon --touch --question,$(RELEASE_MAKE_OPTION_WORDS)),)
    $(error formal release targets reject execution-suppressing make flags (-n/-t/-q))
  endif
  ifneq ($(findstring n,$(RELEASE_MAKE_SHORT_FLAGS)),)
    $(error formal release targets reject execution-suppressing make flags (-n/-t/-q))
  endif
  ifneq ($(findstring t,$(RELEASE_MAKE_SHORT_FLAGS)),)
    $(error formal release targets reject execution-suppressing make flags (-n/-t/-q))
  endif
  ifneq ($(findstring q,$(RELEASE_MAKE_SHORT_FLAGS)),)
    $(error formal release targets reject execution-suppressing make flags (-n/-t/-q))
  endif
  ifeq ($(HOST_OS),windows)
    override FORMAL_RELEASE_PREFLIGHT_RESULT := $(strip $(shell $(WINDOWS_VERIFY_GO_ENV) & $(STREAMER_COMMERCIAL_RELEASE_VERIFIER) --repo . 2>&1; if ($$?) { Write-Output __CODRAX_COMMERCIAL_PREFLIGHT_APPROVED__ } else { Write-Output __CODRAX_COMMERCIAL_PREFLIGHT_REJECTED__ }))
  else
    override FORMAL_RELEASE_PREFLIGHT_RESULT := $(strip $(shell if $(POSIX_GO_VERIFY_ENV) $(STREAMER_COMMERCIAL_RELEASE_VERIFIER) --repo . 2>&1; then printf '%s\n' __CODRAX_COMMERCIAL_PREFLIGHT_APPROVED__; else printf '%s\n' __CODRAX_COMMERCIAL_PREFLIGHT_REJECTED__; fi))
  endif
  ifneq ($(lastword $(FORMAL_RELEASE_PREFLIGHT_RESULT)),__CODRAX_COMMERCIAL_PREFLIGHT_APPROVED__)
    $(error formal commercial trace_streamer preflight rejected before target scheduling: $(FORMAL_RELEASE_PREFLIGHT_RESULT))
  endif
endif

# Ordinary build/cross targets may carry an audited development payload while
# its dependency-license closure remains unresolved. Formal release targets
# must not. This gate is an ordered prerequisite of release-clean-dist, so it
# fails before dist/ is deleted or any artifact is built. Merely changing a
# status string cannot open it: payload-scoped legal approval, SBOM, dependency
# licenses, notices, and source/build attestation are all hash-bound inputs.
ifeq ($(HOST_OS),windows)
verify-trace-streamer-commercial-release:
	$(WINDOWS_VERIFY_GO_ENV) & $(STREAMER_COMMERCIAL_RELEASE_VERIFIER) --repo .; if (-not $$?) { exit 1 }

release-clean-dist: verify-trace-streamer-commercial-release
	if (Test-Path dist) { Remove-Item -Recurse -Force dist }
else
verify-trace-streamer-commercial-release:
	@$(POSIX_GO_VERIFY_ENV) $(STREAMER_COMMERCIAL_RELEASE_VERIFIER) --repo .

release-clean-dist: verify-trace-streamer-commercial-release
	@rm -rf dist/
endif

ifeq ($(HOST_OS),windows)
release: release-clean-dist
	New-Item -ItemType Directory -Force dist | Out-Null
	$(WINDOWS_AMD64_GO_ENV) & $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(DIST_WINDOWS_AMD64) .
	$(WINDOWS_VERIFY_GO_ENV) & $(STREAMER_ARTIFACT_VERIFIER) --artifact $(DIST_WINDOWS_AMD64) --repo . --goos windows --goarch amd64 --cgo 1 --payload windows-amd64 --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer --commercial-release; if (-not $$?) { Remove-Item -Force -ErrorAction SilentlyContinue $(DIST_WINDOWS_AMD64); exit 1 }
	& wsl bash -lc "set -euo pipefail; command -v $(LINUX_GLIBC_CC) >/dev/null 2>&1 || { echo '$(LINUX_GLIBC_CC) not found in WSL; install a Linux amd64 glibc compiler or set LINUX_GLIBC_CC.'; exit 1; }; cd '$(WSL_REPO)' && export GOENV=off GOFLAGS='' GOPROXY='$(WSL_GOPROXY)' GOSUMDB='$(WSL_GOSUMDB)' && CGO_ENABLED=1 CC=$(LINUX_GLIBC_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(DIST_LINUX_AMD64) ."
	$(WINDOWS_VERIFY_GO_ENV) & $(STREAMER_ARTIFACT_VERIFIER) --artifact $(DIST_LINUX_AMD64) --repo . --goos linux --goarch amd64 --cgo 1 --payload linux-amd64 --linux-runtime glibc --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer --commercial-release; if (-not $$?) { Remove-Item -Force -ErrorAction SilentlyContinue $(DIST_LINUX_AMD64); exit 1 }
	& wsl bash -lc "set -euo pipefail; command -v $(MUSL_CC) >/dev/null 2>&1 || { echo '$(MUSL_CC) not found in WSL; install musl-tools first.'; exit 1; }; cd '$(WSL_REPO)' && export GOENV=off GOFLAGS='' GOPROXY='$(WSL_GOPROXY)' GOSUMDB='$(WSL_GOSUMDB)' && CGO_ENABLED=1 CC=$(MUSL_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -tags '$(STATIC_SLIM_BUILD_TAGS)' -ldflags '-linkmode external -extldflags `"-static`" $(LD_VERSION) $(LDFLAGS)' -o $(DIST_LINUX_AMD64_STATIC_SLIM) ."
	$(WINDOWS_VERIFY_GO_ENV) & $(STREAMER_ARTIFACT_VERIFIER) --artifact $(DIST_LINUX_AMD64_STATIC_SLIM) --repo . --goos linux --goarch amd64 --cgo 1 --payload none --linux-runtime static --require-tags '$(STATIC_SLIM_BUILD_TAGS)' --forbid-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --commercial-release; if (-not $$?) { Remove-Item -Force -ErrorAction SilentlyContinue $(DIST_LINUX_AMD64_STATIC_SLIM); exit 1 }
	Write-Host 'Skipped Darwin artifacts on Windows host.'
	Get-ChildItem dist
else ifeq ($(HOST_KIND),darwin)
# macOS host: build the two darwin legs natively. The standard Linux
# embedded artifact needs an explicit glibc cross compiler; the distinct
# static-slim artifact needs musl-cross. Windows needs mingw-w64. Missing
# cross toolchains are disclosed soft skips so native artifacts still ship;
# release-strict turns the declared five-artifact matrix into a hard gate.
release: release-clean-dist
	@mkdir -p dist
	$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-darwin-amd64 .
	@$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact dist/$(BINARY)-darwin-amd64 --repo . --goos darwin --goarch amd64 --cgo 1 --payload none --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer --commercial-release || { rm -f 'dist/$(BINARY)-darwin-amd64'; exit 1; }
	$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-darwin-arm64 .
	@$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact dist/$(BINARY)-darwin-arm64 --repo . --goos darwin --goarch arm64 --cgo 1 --payload none --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer --commercial-release || { rm -f 'dist/$(BINARY)-darwin-arm64'; exit 1; }
	@if command -v $(LINUX_GLIBC_CC) >/dev/null 2>&1; then \
		$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 CC=$(LINUX_GLIBC_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(DIST_LINUX_AMD64) . && \
		$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact $(DIST_LINUX_AMD64) --repo . --goos linux --goarch amd64 --cgo 1 --payload linux-amd64 --linux-runtime glibc --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer --commercial-release || { rm -f '$(DIST_LINUX_AMD64)'; exit 1; } ; \
	else \
		echo "skip $(DIST_LINUX_AMD64): $(LINUX_GLIBC_CC) not found (set LINUX_GLIBC_CC to a Linux amd64 glibc cross compiler)" >&2 ; \
	fi
	@if command -v $(MUSL_CC) >/dev/null 2>&1; then \
		$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 CC=$(MUSL_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -tags '$(STATIC_SLIM_BUILD_TAGS)' -ldflags '-linkmode external -extldflags "-static" $(LD_VERSION) $(LDFLAGS)' -o $(DIST_LINUX_AMD64_STATIC_SLIM) . && \
		$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact $(DIST_LINUX_AMD64_STATIC_SLIM) --repo . --goos linux --goarch amd64 --cgo 1 --payload none --linux-runtime static --require-tags '$(STATIC_SLIM_BUILD_TAGS)' --forbid-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --commercial-release || { rm -f '$(DIST_LINUX_AMD64_STATIC_SLIM)'; exit 1; } ; \
	else \
		echo "skip $(DIST_LINUX_AMD64_STATIC_SLIM): $(MUSL_CC) not found (brew install FiloSottile/musl-cross/musl-cross to enable)" >&2 ; \
	fi
	@if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then \
		$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(DIST_WINDOWS_AMD64) . && \
		$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact $(DIST_WINDOWS_AMD64) --repo . --goos windows --goarch amd64 --cgo 1 --payload windows-amd64 --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer --commercial-release || { rm -f '$(DIST_WINDOWS_AMD64)'; exit 1; } ; \
	else \
		echo "skip $(DIST_WINDOWS_AMD64): x86_64-w64-mingw32-gcc not found (brew install mingw-w64 to enable)" >&2 ; \
	fi
	@ls -lh dist/
else
release: release-clean-dist
	@mkdir -p dist
	$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 CC=$(LINUX_GLIBC_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(DIST_LINUX_AMD64) .
	@$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact $(DIST_LINUX_AMD64) --repo . --goos linux --goarch amd64 --cgo 1 --payload linux-amd64 --linux-runtime glibc --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer --commercial-release || { rm -f '$(DIST_LINUX_AMD64)'; exit 1; }
	$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 CC=$(MUSL_CC) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -tags '$(STATIC_SLIM_BUILD_TAGS)' -ldflags '-linkmode external -extldflags "-static" $(LD_VERSION) $(LDFLAGS)' -o $(DIST_LINUX_AMD64_STATIC_SLIM) .
	@$(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact $(DIST_LINUX_AMD64_STATIC_SLIM) --repo . --goos linux --goarch amd64 --cgo 1 --payload none --linux-runtime static --require-tags '$(STATIC_SLIM_BUILD_TAGS)' --forbid-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --commercial-release || { rm -f '$(DIST_LINUX_AMD64_STATIC_SLIM)'; exit 1; }
	-$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-darwin-amd64 . && $(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact dist/$(BINARY)-darwin-amd64 --repo . --goos darwin --goarch amd64 --cgo 1 --payload none --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer --commercial-release || { rm -f 'dist/$(BINARY)-darwin-amd64'; exit 1; }
	-$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o dist/$(BINARY)-darwin-arm64 . && $(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact dist/$(BINARY)-darwin-arm64 --repo . --goos darwin --goarch arm64 --cgo 1 --payload none --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer --commercial-release || { rm -f 'dist/$(BINARY)-darwin-arm64'; exit 1; }
	-$(POSIX_GO_BUILD_ENV) CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc $(GO) build $(GOFLAGS) -tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' -ldflags '$(LD_VERSION) $(LDFLAGS)' -o $(DIST_WINDOWS_AMD64) . && $(POSIX_GO_VERIFY_ENV) $(STREAMER_ARTIFACT_VERIFIER) --artifact $(DIST_WINDOWS_AMD64) --repo . --goos windows --goarch amd64 --cgo 1 --payload windows-amd64 --require-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer --commercial-release || { rm -f '$(DIST_WINDOWS_AMD64)'; exit 1; }
	@ls -lh dist/
endif

# Strict means every artifact that this host's release recipe promises as a
# supported/core leg must exist after the build. Darwin can produce its two
# native artifacts plus all three supported cross artifacts when explicit
# toolchains are installed. Linux and Windows do not pretend to have an
# osxcross toolchain: their strict core is linux embedded + linux static-slim
# + windows embedded. Preflight happens before clean/build to avoid a partial
# matrix being mistaken for a successful strict release.
ifeq ($(HOST_OS),windows)
release-strict: verify-trace-streamer-commercial-release
	if (-not (Get-Command wsl -ErrorAction SilentlyContinue)) { Write-Error 'release-strict on Windows requires WSL.'; exit 1 }
	& wsl bash -lc "set -euo pipefail; command -v $(LINUX_GLIBC_CC) >/dev/null; command -v $(MUSL_CC) >/dev/null"; if ($$LASTEXITCODE -ne 0) { Write-Error 'release-strict requires $(LINUX_GLIBC_CC) and $(MUSL_CC) inside WSL.'; exit 1 }
	& $(MAKE) release; if (-not $$?) { Write-Error 'release-strict recursive release failed.'; exit 1 }
	$$missing = @('$(DIST_WINDOWS_AMD64)', '$(DIST_LINUX_AMD64)', '$(DIST_LINUX_AMD64_STATIC_SLIM)') | Where-Object { -not (Test-Path $$_ -PathType Leaf) }; if ($$missing) { Write-Error ('release-strict missing artifacts: ' + ($$missing -join ', ')); exit 1 }
else ifeq ($(HOST_KIND),darwin)
release-strict: verify-trace-streamer-commercial-release
	@for tool in '$(LINUX_GLIBC_CC)' '$(MUSL_CC)' x86_64-w64-mingw32-gcc; do command -v "$$tool" >/dev/null 2>&1 || { echo "release-strict missing tool: $$tool" >&2; exit 1; }; done
	@$(MAKE) release
	@for artifact in 'dist/$(BINARY)-darwin-amd64' 'dist/$(BINARY)-darwin-arm64' '$(DIST_LINUX_AMD64)' '$(DIST_LINUX_AMD64_STATIC_SLIM)' '$(DIST_WINDOWS_AMD64)'; do test -f "$$artifact" || { echo "release-strict missing artifact: $$artifact" >&2; exit 1; }; done
else
release-strict: verify-trace-streamer-commercial-release
	@for tool in '$(LINUX_GLIBC_CC)' '$(MUSL_CC)' x86_64-w64-mingw32-gcc; do command -v "$$tool" >/dev/null 2>&1 || { echo "release-strict missing tool: $$tool" >&2; exit 1; }; done
	@$(MAKE) release
	@for artifact in '$(DIST_LINUX_AMD64)' '$(DIST_LINUX_AMD64_STATIC_SLIM)' '$(DIST_WINDOWS_AMD64)'; do test -f "$$artifact" || { echo "release-strict missing artifact: $$artifact" >&2; exit 1; }; done
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
.PHONY: eval-runner-test eval-data-real eval-patch eval-patch-go eval-patch-c eval-patch-cpp eval-patch-java eval-patch-python eval-github-issues

SAMPLES ?= 3

eval-runner-test:
	bash eval/runner_lib_test.sh

eval-data-real: build
	bash eval/data_real_scenario_gate.sh

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

eval-github-issues: build
	@for case in eval/cases/github_issue_*.case; do \
		echo "=== $$case ==="; \
		bash eval/run.sh $$case $(SAMPLES) || true; \
	done
	@echo "Done. Per-case results under eval/results/."
endif

# ---------------------------------------------------------------------------
# Clean
# ---------------------------------------------------------------------------
.PHONY: clean clean-dist

ifeq ($(HOST_OS),windows)
clean:
	foreach ($$path in @('$(BINARY)', '$(BINARY).exe', '$(BINARY)-staticish.exe', '$(STATIC_SLIM_OUT)')) { if (Test-Path $$path) { Remove-Item -Force $$path } }
	& $(GO) clean -cache

clean-dist:
	if (Test-Path dist) { Remove-Item -Force -Recurse dist }
else
clean:
	rm -f $(BINARY) $(BINARY).exe $(STATIC_SLIM_OUT)
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
else ifeq ($(HOST_KIND),darwin)
# macOS host: 'make static' fails loud (see static target above), so
# the standard verify-static path can't run. If a user produced a
# Linux static binary via musl-cross + 'make static MUSL_CC=…',
# verify it with the linux flow inside a Linux container/VM. On
# macOS the natural verification is otool against the native binary;
# the rule below runs that on whatever $(BINARY) currently exists,
# and reports the macOS-side runtime deps.
verify-static:
	@if [ ! -x "$(BINARY)" ]; then \
		echo "$(BINARY) not found; run 'make' first." >&2 ; \
		exit 1 ; \
	fi
	@echo "macOS hosts cannot produce a fully static binary natively."
	@echo "Reporting native dynamic deps via otool -L $(BINARY):"
	@otool -L $(BINARY) || true

verify-windows-runtime:
	@echo "verify-windows-runtime is intended for Windows-hosted builds."
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
	@echo "HOST_OS:   $(HOST_OS)"
	@echo "HOST_KIND: $(HOST_KIND)"
	@echo "GOOS:      $(GOOS)"
	@echo "GOARCH:    $(GOARCH)"
	@echo "VERSION:   $(VERSION)"
	@echo "GO:        $(shell $(GO) version)"
	@echo "CC:        $(shell { $${CC:-cc} --version 2>/dev/null || cc --version 2>/dev/null; } | head -1)"
ifeq ($(HOST_KIND),darwin)
	@echo "MUSL_CC:   $(shell command -v $(MUSL_CC) 2>/dev/null || echo 'not found (brew install FiloSottile/musl-cross/musl-cross)')"
	@echo "MINGW:     $(shell command -v x86_64-w64-mingw32-gcc 2>/dev/null || echo 'not found (brew install mingw-w64)')"
endif
endif

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------
.PHONY: help
help:
	@echo "Codrax Build System"
	@echo ""
	@echo "Targets:"
	@echo "  build              Build for current supported platform (Windows targets amd64; default)"
	@echo "  static             Fully static Linux Codrax parent with embedded Linux trace_streamer child (child requires glibc >= 2.34)"
	@echo "  static-slim        Fully static Linux external-only Codrax at ./$(STATIC_SLIM_OUT) (no embedded trace_streamer)"
	@echo "  test               Run all tests"
	@echo "  test-v             Run all tests (verbose)"
	@echo "  test-race          Run all tests with the Go race detector (slower; required pre-PR for concurrency-sensitive changes)"
	@echo "  cross-linux        Cross-compile for Linux amd64"
	@echo "  cross-linux-arm64  Cross-compile for Linux arm64"
	@echo "  cross-darwin       Cross-compile for macOS amd64"
	@echo "  cross-darwin-arm64 Cross-compile for macOS arm64 (Apple Silicon)"
	@echo "  cross-windows      Cross-compile for Windows amd64"
	@echo "  release            Build release artifacts into dist/ (commercial trace_streamer evidence gate applies)"
	@echo "  release-strict     Build and require every supported/core artifact for this host"
	@echo "  verify-trace-streamer-commercial-release  Verify payload-scoped legal/SBOM/notices/build evidence"
	@echo "  verify-static      Verify Linux static artifact"
	@echo "  verify-windows-runtime Verify Windows binary avoids MinGW runtime DLLs"
	@echo "  clean              Remove build artifacts"
	@echo "  info               Show build environment"
	@echo "  help               Show this help"
	@echo ""
	@echo "Trace conversion packaging:"
	@echo "  - Native/default Linux amd64 and Windows amd64 builds embed only their matching trace_streamer payload."
	@echo "  - dist/$(BINARY)-linux-amd64 is the standard glibc/default-tag artifact with embedded Linux trace_streamer."
	@echo "  - dist/$(BINARY)-windows-amd64.exe is explicitly built for windows/amd64 with embedded Windows trace_streamer."
	@echo "  - make static keeps the Codrax parent fully static and embeds the independent Linux trace_streamer child."
	@echo "  - make static-slim and dist/$(BINARY)-linux-amd64-static-slim are explicit fully-static external-only artifacts."
	@echo "  - The embedded child still requires glibc >= 2.34 (plus its own shared libraries); this is not a Codrax parent dependency."
	@echo "  - Add static-only project tags with STATIC_EXTRA_TAGS=tag1,tag2; reserved embedded/slim identity tags are rejected."
	@echo "  - macOS and unsupported architectures require a compatible external trace_streamer."
	@echo "  - Formal embedded release currently fails loud while provenance says NOASSERTION/blocked; ordinary development builds remain available."
	@echo ""
	@echo "Windows notes:"
	@echo "  - Native build/lowmem use PowerShell and intentionally target windows/amd64 so the matching payload is always present; tests remain host-native."
	@echo "  - make static and make static-slim delegate to WSL and require musl-gcc there."
	@echo ""
	@echo "macOS notes:"
	@echo "  - 'make' / 'make test' / 'make cross-darwin*' / 'make eval-patch*' work natively (Xcode CLT)."
	@echo "  - 'make static' fail-louds (Apple does not ship a static libc); use 'make' for native or musl-cross for cross-static."
	@echo "  - 'make release' soft-skips unavailable cross legs; standard Linux needs LINUX_GLIBC_CC, static-slim needs MUSL_CC, and Windows needs x86_64-w64-mingw32-gcc."
	@echo "  - 'make release-strict' fails before release when any compiler for the declared five-artifact macOS matrix is missing."
	@echo "  - 'make verify-static' on macOS reports otool deps of the native binary (Linux-static verification needs a Linux host)."
