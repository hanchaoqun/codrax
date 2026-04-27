package tool

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// searchBackend holds the detected search command.
// Detected once at first use; cached for the process lifetime.
//
// Tri-state:
//   - "rg"     — ripgrep found and runnable
//   - "grep"   — GNU/BSD grep found and runnable (or blind-call fallback)
//   - "native" — neither found; use the Go-native scanner in nativegrep.go
//
// The "native" state guarantees codrax keeps working on minimal
// environments (distroless/scratch containers, stripped CI images,
// Windows without Git-for-Windows) at the cost of slower scans.
var (
	searchOnce    sync.Once
	searchCommand string // "rg" / "grep" / "native"
	searchPath    string // actual executable path used for the backend; "" when native
)

// SearchCommand returns the selected search backend identifier.
// The result is cached after the first call.
func SearchCommand() string {
	searchOnce.Do(func() {
		if path := firstRunnablePath("rg", []string{"--version"}, windowsExtraCommandCandidates("rg")...); path != "" {
			searchCommand = "rg"
			searchPath = path
			logging.Info("search backend: ripgrep (%s)", path)
			return
		}
		if path := firstRunnablePath("grep", []string{"--version"}, windowsExtraCommandCandidates("grep")...); path != "" {
			searchCommand = "grep"
			searchPath = path
			logging.Info("search backend: grep (%s; ripgrep not found — install ripgrep for faster scans)", path)
			return
		}
		searchCommand = "native"
		searchPath = ""
		logging.Warning("search backend: native Go scanner (neither ripgrep nor grep found on PATH — install ripgrep for faster scans; codrax falls back to a pure-Go regex walker)")
	})
	return searchCommand
}

// SearchExecutable returns the actual executable path selected for the
// current search backend. Returns "" when the backend is native —
// callers MUST check UseNativeGrep() before shelling out.
func SearchExecutable() string {
	SearchCommand()
	return searchPath
}

// UseRipgrep returns true if ripgrep was detected as available.
func UseRipgrep() bool {
	return SearchCommand() == "rg"
}

// UseNativeGrep returns true when neither rg nor grep is on PATH and
// the Go-native scanner must be used. Callers in this package and in
// internal/agent check this to avoid exec.CommandContext on an empty
// SearchExecutable(), which would fail to start.
func UseNativeGrep() bool {
	return SearchCommand() == "native"
}

// ExcludeDirs is the single authoritative list of directories that
// every search operation (grep tool, keyword search, file coverage
// analysis) skips. Centralised here so all call sites stay in sync.
//
// Two overlapping categories, exposed separately so callers can
// pick the semantics they need:
//
//   - ExcludeDirsAnyLevel entries match at any directory depth.
//     They are dependency trees, VCS internals, build output, and
//     IDE state — directories that are structurally never part of
//     the codebase regardless of where they appear.
//
//   - ExcludeDirsRootOnly entries match only at the repo root.
//     They are runtime artifacts ("memory", "logs") and eval
//     fixtures ("eval") — names whose semantics are "this special
//     top-level folder codrax maintains", and which a nested
//     `internal/memory/` package must NOT be accidentally matched
//     against.
//
// ExcludeDirs is the concatenation, retained for backwards
// compatibility with existing ripgrep-glob and filepath.WalkDir
// callers that accept the old "any-level" semantics. New code
// that runs inside nested paths (e.g. the repomap scanner) should
// prefer ExcludeDirsAnyLevel + an explicit root-segment check.
var ExcludeDirsAnyLevel = []string{
	".git", ".hg", ".svn",
	".codrax", // codrax's own per-repo state (logs / blob / worktrees / plans). Showed up in `list_files recursive=false` output and confused LLMs into thinking it was project state.
	"node_modules", "vendor", "__pycache__", ".tox", ".venv", "venv", ".mypy_cache", ".pytest_cache",
	".idea", ".vscode", ".vs",
	"target", "dist", "build", ".gradle", ".cargo",
	".next", ".nuxt", ".turbo", // common JS framework output dirs
	".pnpm-store", // alternative to node_modules
}

// ExcludeDirPatternsAnyLevel extends the exact-name directory list
// with wildcard/prefix patterns for transient cache roots that are
// created by tooling inside a repo checkout. These are infrastructure
// artifacts, not project content:
//   - .gotmp*   — per-run temp roots used by codrax/go tooling
//   - .gocache* — local Go build/module cache roots parked in-repo
//
// Kept central so shell-backed search and Go-native walkers apply the
// same policy instead of drifting.
var ExcludeDirPatternsAnyLevel = func() []string {
	out := make([]string, 0, len(ExcludeDirsAnyLevel)+2)
	out = append(out, ExcludeDirsAnyLevel...)
	out = append(out, ".gotmp*", ".gocache*")
	return out
}()

// ExcludeDirsRootOnly entries are matched only when they sit at
// position 0 of a relative path. See ExcludeDirsAnyLevel for
// rationale.
var ExcludeDirsRootOnly = []string{
	"logs", "memory", "eval",
}

// ExcludeDirs is the flat union of any-level and root-only lists.
// Kept for existing callers that treat the whole set as any-level
// (GrepTool, keyword_search, explorer). Those sites accept some
// false-positive exclusion of nested "memory"/"logs" directories
// as an acceptable trade-off; scan paths that can't tolerate it
// (repomap.scanner) build their own predicate from the two slices.
var ExcludeDirs = func() []string {
	out := make([]string, 0, len(ExcludeDirPatternsAnyLevel)+len(ExcludeDirsRootOnly))
	out = append(out, ExcludeDirPatternsAnyLevel...)
	out = append(out, ExcludeDirsRootOnly...)
	return out
}()

// ExcludeDirsAnyLevelSet is the map form of ExcludeDirsAnyLevel,
// built once at package init for O(1) membership checks.
var ExcludeDirsAnyLevelSet = func() map[string]bool {
	m := make(map[string]bool, len(ExcludeDirsAnyLevel))
	for _, d := range ExcludeDirsAnyLevel {
		m[d] = true
	}
	return m
}()

// ExcludeDirsRootOnlySet is the map form of ExcludeDirsRootOnly.
var ExcludeDirsRootOnlySet = func() map[string]bool {
	m := make(map[string]bool, len(ExcludeDirsRootOnly))
	for _, d := range ExcludeDirsRootOnly {
		m[d] = true
	}
	return m
}()

// DirNameMatchesExcludePattern reports whether name matches any exact
// or glob-style directory exclude pattern. Patterns are the same ones
// passed to grep/rg (`node_modules`, `.gotmp*`, ...), so shell-backed
// and Go-native scans stay aligned.
func DirNameMatchesExcludePattern(name string, patterns []string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if pattern == name {
			return true
		}
		if strings.ContainsAny(pattern, "*?[") {
			if matched, err := filepath.Match(pattern, name); err == nil && matched {
				return true
			}
		}
	}
	return false
}

// IsExcludedDirName reports whether a single directory basename should
// be excluded at any depth.
func IsExcludedDirName(name string) bool {
	return DirNameMatchesExcludePattern(name, ExcludeDirPatternsAnyLevel)
}

// IsExcludedRelativePath reports whether a repo-relative path should be
// skipped by repo search / scan layers. Root-only exclusions are only
// applied to the first segment; any-level exclusions apply to every
// directory segment.
func IsExcludedRelativePath(relPath string) bool {
	if IsWindowsReservedDevicePath(relPath) {
		return true
	}
	normalized := strings.TrimSpace(strings.ReplaceAll(relPath, `\`, `/`))
	if normalized == "" {
		return false
	}
	parts := strings.Split(normalized, "/")
	if len(parts) > 0 && ExcludeDirsRootOnlySet[parts[0]] {
		return true
	}
	for _, part := range parts {
		if IsExcludedDirName(part) {
			return true
		}
	}
	return false
}

// windowsReservedDeviceNames are DOS device basenames that behave like
// pseudo-files on Windows (`nul`, `con`, `prn`, `aux`, `com1`...).
// Repositories can contain these names when authored on other
// platforms, but ripgrep / grep / WalkDir on Windows will treat them
// as device handles rather than ordinary source files, causing noisy
// search failures before the pipeline even reaches grounded reads.
var windowsReservedDeviceNames = []string{
	"con", "prn", "aux", "nul",
	"com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9",
	"lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9",
}

var windowsReservedDeviceSet = func() map[string]bool {
	m := make(map[string]bool, len(windowsReservedDeviceNames))
	for _, name := range windowsReservedDeviceNames {
		m[name] = true
	}
	return m
}()

// IsWindowsReservedDevicePath reports whether name resolves to a
// Windows reserved device basename such as `nul` or `con`.
//
// The check is intentionally OS-gated: on Linux/macOS a file literally
// named `nul` is valid project content and must remain searchable. On
// Windows, however, these names break external search tools and native
// walkers alike, so codrax treats them as search/list noise files.
func IsWindowsReservedDevicePath(name string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	base := strings.ToLower(strings.TrimSpace(filepath.Base(name)))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return false
	}
	if idx := strings.IndexByte(base, '.'); idx >= 0 {
		base = base[:idx]
	}
	return windowsReservedDeviceSet[base]
}

// ReservedDeviceRipgrepGlobs returns ripgrep glob patterns that skip
// Windows reserved device basenames at repo root and nested paths.
func ReservedDeviceRipgrepGlobs() []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	out := make([]string, 0, len(windowsReservedDeviceNames)*4)
	for _, name := range windowsReservedDeviceNames {
		out = append(out,
			name,
			name+".*",
			"**/"+name,
			"**/"+name+".*",
		)
	}
	return out
}

// ReservedDeviceGrepExcludes returns GNU grep basename exclude globs
// for Windows reserved device files.
func ReservedDeviceGrepExcludes() []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	out := make([]string, 0, len(windowsReservedDeviceNames)*2)
	for _, name := range windowsReservedDeviceNames {
		out = append(out, name, name+".*")
	}
	return out
}
