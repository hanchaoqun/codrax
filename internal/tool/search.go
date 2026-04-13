package tool

import (
	"sync"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// searchBackend holds the detected search command ("rg" or "grep").
// Detected once at first use; cached for the process lifetime.
var (
	searchOnce    sync.Once
	searchCommand string // "rg" or "grep"
	searchPath    string // actual executable path used for the backend
)

// SearchCommand returns "rg" if ripgrep is available, "grep" otherwise.
// The result is cached after the first call.
func SearchCommand() string {
	searchOnce.Do(func() {
		if path := firstRunnablePath("rg", []string{"--version"}, windowsExtraCommandCandidates("rg")...); path != "" {
			searchCommand = "rg"
			searchPath = path
			logging.Info("search backend: ripgrep (%s)", path)
		} else {
			searchCommand = "grep"
			searchPath = firstRunnablePath("grep", []string{"--version"}, windowsExtraCommandCandidates("grep")...)
			if searchPath == "" {
				searchPath = "grep"
				logging.Info("search backend: grep (ripgrep not found)")
			} else {
				logging.Info("search backend: grep (%s)", searchPath)
			}
		}
	})
	return searchCommand
}

// SearchExecutable returns the actual executable path selected for the
// current search backend. When no validated path is available, it falls
// back to the backend name so Unix behavior stays unchanged.
func SearchExecutable() string {
	backend := SearchCommand()
	if searchPath != "" {
		return searchPath
	}
	return backend
}

// UseRipgrep returns true if ripgrep was detected as available.
func UseRipgrep() bool {
	return SearchCommand() == "rg"
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
	"node_modules", "vendor", "__pycache__", ".tox", ".venv", ".mypy_cache",
	".idea", ".vscode",
	"target", "dist", "build", ".gradle",
}

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
	out := make([]string, 0, len(ExcludeDirsAnyLevel)+len(ExcludeDirsRootOnly))
	out = append(out, ExcludeDirsAnyLevel...)
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
