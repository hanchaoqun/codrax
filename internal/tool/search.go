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
