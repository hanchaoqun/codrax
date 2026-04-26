package tool

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
)

var (
	gitProbeOnce  sync.Once
	gitProbeOk    bool
	gitProbePath  string
	gitProbeVerOK string // trimmed `git --version` output; "" when probe failed

	// lintEnabled gates the V5 lint validator family. Default true:
	// the validator is opt-out (operators who want strict-tests-only
	// behaviour set codrax.yaml :: pipeline_lint_enabled: false). The
	// individual per-language helpers ALSO short-circuit when their
	// toolchain (ruff / gofmt) is missing, so the master switch is
	// the operator's hard veto, not a "is anything installed" probe.
	lintEnabled = true
)

// LintEnabled reports whether V5 lint validators should run. See
// validatePlanLint for the consumer side. Defaults to true; operators
// set codrax.yaml :: pipeline_lint_enabled: false to disable
// (typically when a project's existing code carries lint debt and the
// new-files-only filter still feels too noisy).
func LintEnabled() bool { return lintEnabled }

// SetLintEnabled installs the operator's yaml override. Called once
// from cmd/root.go's settings-resolution block alongside the other
// pipeline knobs.
func SetLintEnabled(b bool) { lintEnabled = b }

// GitAvailable reports whether `git --version` succeeded during the
// one-shot startup probe. Cached for the process lifetime so REPL
// banner, probe logger, and any future consumer share one answer.
// Safe to call before LogCapabilities (forces the probe on demand).
func GitAvailable() bool {
	runGitProbe()
	return gitProbeOk
}

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
	runGitProbe()
	if gitProbeOk {
		logging.Info("git backend: %s (%s)", gitProbePath, gitProbeVerOK)
	} else {
		switch runtime.GOOS {
		case "windows":
			logging.Warning("git not found on PATH — repomap scanning falls back to filesystem walk; git_diff / git_log tools disabled. Install Git for Windows (https://git-scm.com/download/win) to restore.")
		case "darwin":
			logging.Warning("git not found on PATH — repomap scanning falls back to filesystem walk; git_diff / git_log tools disabled. Install with `xcode-select --install` or `brew install git`.")
		default:
			logging.Warning("git not found on PATH — repomap scanning falls back to filesystem walk; git_diff / git_log tools disabled. Install via your distro package manager (apt/yum/apk install git).")
		}
	}

	// V5 lint per-language availability snapshot. One INFO line
	// summarising which languages are active + which are skipped
	// (binary missing). Operators audit in-place via the startup
	// banner; no need to grep emit_change_plan.go.
	if !LintEnabled() {
		logging.Info("V5 lint validator: DISABLED via codrax.yaml :: pipeline_lint_enabled")
		return
	}
	stats := LintLanguagesEnabled()
	var active, missing []string
	for _, st := range stats {
		if st.Available {
			active = append(active, st.Name)
		} else {
			missing = append(missing, st.Name+" ("+st.Binary+")")
		}
	}
	if len(active) == 0 {
		logging.Warning("V5 lint validator: NO language toolchains found on PATH — all lint checks will silently skip. Install at least one of: %s",
			strings.Join(linterListForLog(stats), ", "))
		return
	}
	logging.Info("V5 lint validator: active for %d/%d languages — active=[%s]; missing=[%s]",
		len(active), len(stats), strings.Join(active, ", "), strings.Join(missing, ", "))

	// V6 project-aware lint snapshot. Same shape as V5 but the
	// "active" list reflects which language toolchains are on PATH —
	// the V6 trigger ALSO requires the project to carry the
	// language's manifest file (oh-package.json5 / cjpm.toml), which
	// is per-Run state and not knowable at startup. The banner is
	// purely about toolchain presence so operators know what V6
	// COULD lint if a matching project shows up.
	pstats := ProjectLintLanguagesEnabled()
	var pactive, pmissing []string
	for _, st := range pstats {
		if st.Available {
			pactive = append(pactive, st.Name)
		} else {
			pmissing = append(pmissing, st.Name+" ("+st.Binary+")")
		}
	}
	if len(pactive) == 0 {
		logging.Info("V6 project lint validator: 0/%d toolchains on PATH — missing=[%s]. Install the relevant toolchain (DevEco / Cangjie SDK) when working on a HarmonyOS / Cangjie project.",
			len(pstats), strings.Join(pmissing, ", "))
	} else {
		logging.Info("V6 project lint validator: %d/%d toolchains on PATH — active=[%s]; missing=[%s]. V6 fires only when the repo also contains the matching manifest (oh-package.json5 / cjpm.toml).",
			len(pactive), len(pstats), strings.Join(pactive, ", "), strings.Join(pmissing, ", "))
	}
}

// linterListForLog formats the per-language install hints as a
// short comma-separated list for the WARN-when-nothing-active log
// line. Kept tiny so the warning stays scannable.
func linterListForLog(stats []LintLangStatus) []string {
	out := make([]string, 0, len(stats))
	for _, st := range stats {
		out = append(out, st.Name+":"+st.Binary)
	}
	return out
}

// runGitProbe tries `git --version` with a 2s timeout and caches the
// outcome in gitProbeOk / gitProbePath / gitProbeVerOK. Safe to call
// repeatedly; only the first call actually probes.
func runGitProbe() {
	gitProbeOnce.Do(func() {
		path, err := exec.LookPath("git")
		if err != nil || path == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, path, "--version").Output()
		if err != nil {
			logging.Warning("git detected at %s but `git --version` failed: %v", path, err)
			return
		}
		version := ""
		if len(out) > 0 {
			version = string(out)
			if n := len(version); n > 0 && version[n-1] == '\n' {
				version = version[:n-1]
			}
		}
		gitProbeOk = true
		gitProbePath = path
		gitProbeVerOK = version
	})
}
