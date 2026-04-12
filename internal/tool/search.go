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

// ExcludeDirs is the single authoritative list of directories that all
// search operations (grep tool, keyword search, file coverage analysis)
// skip. Centralised here so the three call sites stay in sync.
//
// Categories:
//   - VCS internals: .git, .hg, .svn
//   - Dependency trees: node_modules, vendor, __pycache__, .tox
//   - Runtime artifacts: logs, memory (codrax's own output dirs)
//   - Build output: target (Rust/Java), dist, build
//   - Eval artifacts: eval/results/* full transcripts contain the
//     test question verbatim and would contaminate keyword search
//     for the very test they're produced from. Excluding the whole
//     `eval` tree is safe because case files use a separate runtime.
var ExcludeDirs = []string{
	".git", ".hg", ".svn",
	"node_modules", "vendor", "__pycache__", ".tox",
	"logs", "memory", "eval",
	"target", "dist", "build",
}
