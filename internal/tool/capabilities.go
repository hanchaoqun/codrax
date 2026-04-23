package tool

import (
	"context"
	"os/exec"
	"runtime"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// LogCapabilities forces a one-shot probe of every external binary
// codrax touches and emits one log line per backend. Each missing
// binary comes with a concrete install hint so operators do not
// have to cross-reference the docs when something breaks at runtime.
//
// Safe to call multiple times — individual detectors are already
// sync.Once-guarded; the extra calls just re-log whatever was cached.
//
// Call from initApp AFTER logging is wired and BEFORE the first
// pipeline run. Must NOT abort on any miss: codrax degrades rather
// than refusing to start. A deliberately noisy log on the main path
// is the whole point of this probe.
func LogCapabilities() {
	// Search backend (rg / grep / native). SearchCommand's own
	// sync.Once already emits a line on first call; we call it here
	// so the banner order is deterministic at startup instead of
	// firing on the first grep deep inside a stage.
	_ = SearchCommand()

	// Shell (sh / bash / cmd on Windows; sh on Unix).
	path, args := shellSpec()
	logging.Info("shell backend: %s %v", path, args)

	// Git — required for repomap scan speed and for git_diff / git_log.
	// When missing, the repomap scanner falls back to filepath.Walk
	// and the two user-facing git tools return structured errors.
	if probeGit() {
		// Already-logged in probeGit; nothing more to do.
		return
	}
	switch runtime.GOOS {
	case "windows":
		logging.Warning("git not found on PATH — repomap scanning falls back to filesystem walk; git_diff / git_log tools disabled. Install Git for Windows (https://git-scm.com/download/win) to restore.")
	case "darwin":
		logging.Warning("git not found on PATH — repomap scanning falls back to filesystem walk; git_diff / git_log tools disabled. Install with `xcode-select --install` or `brew install git`.")
	default:
		logging.Warning("git not found on PATH — repomap scanning falls back to filesystem walk; git_diff / git_log tools disabled. Install via your distro package manager (apt/yum/apk install git).")
	}
}

// probeGit runs `git --version` with a 2s timeout and, if it
// succeeds, logs the version. Returns true when git is usable.
func probeGit() bool {
	path, err := exec.LookPath("git")
	if err != nil || path == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		logging.Warning("git detected at %s but `git --version` failed: %v", path, err)
		return false
	}
	// git --version prints a single line like "git version 2.43.0"
	version := ""
	if len(out) > 0 {
		version = string(out)
		if n := len(version); n > 0 && version[n-1] == '\n' {
			version = version[:n-1]
		}
	}
	logging.Info("git backend: %s (%s)", path, version)
	return true
}
