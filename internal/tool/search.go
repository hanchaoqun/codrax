package tool

import (
	"os/exec"
	"sync"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// searchBackend holds the detected search command ("rg" or "grep").
// Detected once at first use; cached for the process lifetime.
var (
	searchOnce    sync.Once
	searchCommand string // "rg" or "grep"
)

// SearchCommand returns "rg" if ripgrep is available, "grep" otherwise.
// The result is cached after the first call.
func SearchCommand() string {
	searchOnce.Do(func() {
		if path, err := exec.LookPath("rg"); err == nil && path != "" {
			searchCommand = "rg"
			logging.Info("search backend: ripgrep (%s)", path)
		} else {
			searchCommand = "grep"
			logging.Info("search backend: grep (ripgrep not found)")
		}
	})
	return searchCommand
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
var ExcludeDirs = []string{
	".git", ".hg", ".svn",
	"node_modules", "vendor", "__pycache__", ".tox",
	"logs", "memory",
	"target", "dist", "build",
}
