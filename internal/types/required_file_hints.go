package types

import (
	"os"
	"strings"

	"github.com/hanchaoqun/codrax/internal/canonpath"
)

// RequiredFileHintCoverageMax bounds automatic current-source reads derived
// from high-confidence analyzer required_file hints. The cap is shared by the
// explorer pre-dispatch seed and the emit_investigation_complete pre-complete
// gate so both layers enforce the same typed contract.
const RequiredFileHintCoverageMax = 4

// RequiredFileHintCurrentSourceCoverageApplies reports whether high-confidence
// analyzer RequiredFileHints are hard current-source obligations for this
// request. The signal is typed-only: runtime/history requests that also ask for
// current-code verification must cover the hinted files; observation-only
// runtime artifacts must not be polluted with repository reads.
func RequiredFileHintCurrentSourceCoverageApplies(rm RequestModel) bool {
	if len(rm.AnalyzerHints.RequiredFileHints) == 0 || rm.HasObservationOnlyRuntimeArtifact() {
		return false
	}
	if rm.HasExternalOnlyRuntimeArtifact() && rm.HasRuntimeArtifactCurrentVerificationAnchor() {
		return true
	}
	return IsHistoryBackedCurrentCodeExplanation(rm)
}

// CanonicalRequiredFileHintPath collapses required_file hint paths to the same
// repo-relative POSIX form used by evidence-closure read and pending-read keys.
func CanonicalRequiredFileHintPath(path, repoRoot string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = canonpath.CanonicalRepoRelative(path, repoRoot)
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}

// QualifyForcedReadSeedPath resolves a required-file/forced-read seed against
// the active multi-repo set. Single-repo callers pass through; ambiguous or
// inactive multi-repo seeds return ok=false so the caller does not enqueue a
// PendingRead that can never drain.
func QualifyForcedReadSeedPath(ctx *BusContext, raw string) (string, bool) {
	repoRoot := ""
	if ctx != nil {
		repoRoot = ctx.RepoRoot
	}
	canon := CanonicalRequiredFileHintPath(raw, repoRoot)
	if canon == "" {
		return "", false
	}
	if ctx == nil || ctx.MultiGraph == nil {
		return canon, true
	}
	gater, isGater := ctx.MultiGraph.(MultiRepoActiveSetGater)
	if !isGater || gater == nil {
		return canon, true
	}
	gate := gater.ResolveActiveSetPath(ctx, "required_file_hint_seed", canon, func(abs string) bool {
		info, err := os.Stat(abs)
		return err == nil && !info.IsDir()
	})
	if !gate.Allowed {
		return canon, false
	}
	if gate.ResolvedPath == "" {
		return canon, true
	}
	qualified := CanonicalRequiredFileHintPath(gate.ResolvedPath, repoRoot)
	if qualified == "" {
		return "", false
	}
	return qualified, true
}
