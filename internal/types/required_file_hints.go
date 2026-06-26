package types

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/canonpath"
)

// RequiredFileHintCoverageMax bounds automatic current-source reads derived
// from high-confidence analyzer required_file hints. The cap is shared by the
// explorer pre-dispatch seed and the emit_investigation_complete pre-complete
// gate so both layers enforce the same typed contract.
const RequiredFileHintCoverageMax = 4

// SourceInventoryRequiredFileHintCoverageMax is the bounded source-inventory
// variant of RequiredFileHintCoverageMax. Inventory/enumeration tasks often
// need a handful of peer files to avoid false completeness, but the cap must
// remain low enough that routine read tasks are not flooded with pre-reads.
const SourceInventoryRequiredFileHintCoverageMax = 6

// RequiredFileHintCurrentSourceCoverageApplies reports whether high-confidence
// analyzer RequiredFileHints are hard current-source obligations for this
// request. The signal is typed-only: runtime/history requests that also ask for
// current-code verification must cover the hinted files; observation-only
// runtime artifacts must not be polluted with repository reads.
func RequiredFileHintCurrentSourceCoverageApplies(rm RequestModel) bool {
	if len(rm.AnalyzerHints.RequiredFileHints) == 0 ||
		rm.HasObservationOnlyRuntimeArtifact() ||
		(rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource()) {
		return false
	}
	if rm.HasExternalOnlyRuntimeArtifact() && rm.HasRuntimeArtifactCurrentVerificationAnchor() {
		return true
	}
	if RequiredFileHintSourceInventoryCoverageApplies(rm) {
		return true
	}
	return IsHistoryBackedCurrentCodeExplanation(rm)
}

// RequiredFileHintCoverageMaxForRequest returns the shared forced-read cap for
// a request's required-file lane. Callers must use this instead of open-coding a
// cap so pre-dispatch and completion gates stay aligned.
func RequiredFileHintCoverageMaxForRequest(rm RequestModel) int {
	if SourceInventoryRequiredFileCoverageShape(rm) {
		return SourceInventoryRequiredFileHintCoverageMax
	}
	return RequiredFileHintCoverageMax
}

// RequiredFileHintSourceInventoryCoverageApplies reports whether required-file
// hints are completeness-bearing for a typed source inventory / exhaustive
// source enumeration lane. It consumes only RequestModel schema fields. No raw
// request text, model rationale, or final-answer prose can activate it.
func RequiredFileHintSourceInventoryCoverageApplies(rm RequestModel) bool {
	if len(rm.AnalyzerHints.RequiredFileHints) == 0 || rm.HasObservationOnlyRuntimeArtifact() {
		return false
	}
	return SourceInventoryRequiredFileCoverageShape(rm)
}

// SourceInventoryRequiredFileCoverageShape reports whether a request has the
// typed shape where prescan-discovered files may become bounded required-file
// coverage obligations. It intentionally ignores whether hints already exist
// so analyzer normalization can project deterministic prescan paths into the
// hint lane before downstream gates evaluate it.
func SourceInventoryRequiredFileCoverageShape(rm RequestModel) bool {
	if rm.HasObservationOnlyRuntimeArtifact() ||
		(rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource()) {
		return false
	}
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		return true
	}
	return IsTypedSourceEnumerationShape(rm) &&
		(rm.CompletenessObligation.IsActive() || rm.EnumerationBoundary != nil || HasPrincipalCategoryEnumerationMemberLane(rm))
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
// the active checkout and multi-repo set. Seeds must resolve to a regular file
// inside the current repo/active sub-repo; ambiguous, inactive, repo-external,
// parent-traversal, directory, or missing paths return ok=false so callers do
// not enqueue or execute a PendingRead that can never drain.
func QualifyForcedReadSeedPath(ctx *BusContext, raw string) (string, bool) {
	repoRoot := ""
	if ctx != nil {
		repoRoot = ctx.RepoRoot
	}
	canon := CanonicalRequiredFileHintPath(raw, repoRoot)
	if canon == "" {
		return "", false
	}
	if ForcedReadSeedEscapesRepo(canon) {
		return "", false
	}
	if ctx == nil || strings.TrimSpace(repoRoot) == "" {
		return canon, true
	}
	if ctx.MultiGraph == nil {
		return qualifyExistingForcedReadSeed(repoRoot, canon)
	}
	gater, isGater := ctx.MultiGraph.(MultiRepoActiveSetGater)
	if !isGater || gater == nil {
		return qualifyExistingForcedReadSeed(repoRoot, canon)
	}
	gate := gater.ResolveActiveSetPath(ctx, "required_file_hint_seed", canon, func(abs string) bool {
		info, err := os.Stat(abs)
		return err == nil && !info.IsDir()
	})
	if !gate.Allowed {
		return canon, false
	}
	if gate.ResolvedPath == "" {
		return qualifyExistingForcedReadSeed(repoRoot, canon)
	}
	qualified := CanonicalRequiredFileHintPath(gate.ResolvedPath, repoRoot)
	if qualified == "" {
		return "", false
	}
	return qualifyExistingForcedReadSeed(repoRoot, qualified)
}

func qualifyExistingForcedReadSeed(repoRoot, rel string) (string, bool) {
	rel = CanonicalRequiredFileHintPath(rel, repoRoot)
	if rel == "" {
		return "", false
	}
	if ForcedReadSeedEscapesRepo(rel) {
		return "", false
	}
	if strings.TrimSpace(repoRoot) == "" {
		return rel, true
	}
	info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(rel)))
	if err != nil || info.IsDir() || !info.Mode().IsRegular() {
		return "", false
	}
	return rel, true
}

// ForcedReadSeedEscapesRepo reports whether a canonical forced-read seed points
// outside the active checkout. Forced-read producers and executors share this
// helper so historical, absolute-external, and parent-traversal paths cannot
// become hard read debt through a side entrance.
func ForcedReadSeedEscapesRepo(path string) bool {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" {
		return false
	}
	if path == ".." || strings.HasPrefix(path, "../") {
		return true
	}
	if strings.HasPrefix(path, "/") {
		return true
	}
	return len(path) >= 3 && isASCIIAlpha(path[0]) && path[1] == ':' && path[2] == '/'
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
