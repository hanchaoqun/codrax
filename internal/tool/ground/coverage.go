package ground

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ParseReadFileBanner is the single canonical parser for the banner
// the read_file tool prepends to every Summary. Two shapes are
// recognised (see internal/tool/builtin.go::ReadFile.Execute):
//
//	[path: showing lines X-Y of Z total]
//	[path: showing all N lines (B bytes); limit=M expanded to full file (...)]
//
// Forced-read trace prefixes (`[forced_read] ` and
// `[forced_read surgical] `, applied by orchestrator/cgec_enforcers
// runForcedReads to mark synthesised reads) are stripped before
// parsing so the prefix does not collide with the banner's
// `[path: ...]` shape. Without this strip, the parser would split on
// the FIRST `: ` it found inside the prefix, producing a malformed
// path key that misses every downstream coverage / line-index lookup.
//
// Returns the repo-relative path verbatim from the banner and the
// [startLine, endLine] line range (1-based inclusive). totalLines is
// Z (or N for the "all" shape) when parseable, 0 otherwise. ok is
// false for any malformed banner so callers skip the entry without
// defensive string checks at the call site.
//
// Lifted out of internal/agent/explorer.go so any package that needs
// to read banner-derived coverage (notably the per-tool gates that
// fire mid-dispatch from internal/tool/) can use the same parser
// instead of reaching into the agent layer.
func ParseReadFileBanner(summary string) (path string, rng types.LineRange, totalLines int, ok bool) {
	return types.ParseReadFileBanner(summary)
}

// ExtractReadCoverage walks tool history and rebuilds the per-file
// read coverage state from read_file banners. Returns:
//
//   - readSet: set of files with at least one observed read_file
//   - readRanges: per-file [Start, End] slices the LLM saw (multi-
//     pagination accumulates; not pre-merged so the caller can hand
//     them to EvidenceClosure.SetReadRanges which runs the canonical
//     mergeLineRanges)
//   - totals: per-file total line count harvested from the banner;
//     0 when the banner did not carry a Z value
//
// repoRoot enables banner-path canonicalisation via
// CanonicalRepoRelative so an absolute / leading-./ banner shape lands
// on the same map key as repo-relative reads from the same file
// (Windows volumes case-insensitive, POSIX slash-form preserved).
// An empty repoRoot is tolerated — the function falls back to a
// slash-form cleanup (matches pre-session-22 behaviour).
//
// SCOPE: read_file only. The discovered-files surface (grep / exec
// path harvesting + isNoisePath filtering) lives in the agent layer
// because it depends on agent-side filters; the closure refresh path
// only needs the read_file projection so this lean walker is enough
// and keeps the coverage helper free of an internal/tool import that
// would create a tool ↔ tool/ground cycle.
func ExtractReadCoverage(history []types.ToolResult, repoRoot string) (
	readSet map[string]bool,
	readRanges map[string][]types.LineRange,
	totals map[string]int,
) {
	return types.ExtractReadCoverage(history, repoRoot)
}

// dispatchHistory aggregates the same three-source tool history
// BuildContext consumes (TurnA artefacts → BusContext.ToolResults →
// in-dispatch buffer). Layered oldest → newest so per-file ranges
// from the live ReAct loop accumulate on top of frozen prior-stage
// reads. nil-tolerant.
//
// Centralised here so RefreshClosureCoverage and BuildContext stay
// in sync — drift between the two would silently re-introduce the
// "gate sees stale ranges" bug this helper exists to prevent.
func dispatchHistory(ctx *types.BusContext) []types.ToolResult {
	if ctx == nil {
		return nil
	}
	var history []types.ToolResult
	if ctx.Mutable != nil {
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil && len(ta.ToolResults) > 0 {
			history = append(history, ta.ToolResults...)
		}
	}
	if len(ctx.ToolResults) > 0 {
		history = append(history, ctx.ToolResults...)
	}
	if ctx.Mutable != nil {
		if disp := ctx.Mutable.DispatchToolResults(); len(disp) > 0 {
			history = append(history, disp...)
		}
	}
	return history
}

// RefreshClosureCoverage rebuilds the EvidenceClosure's per-file read
// ranges and total-line snapshot from the live tool history (TurnA +
// BusContext + in-dispatch buffer). Any gate that reads
// closure.MergedReadLines / FileTotalLines / CoverageRatio /
// HasFullyRead from inside a per-tool Execute (mid-dispatch, before
// the explorer's per-round ParseOutput runs SetReadRanges) MUST call
// this first or it will see ranges frozen at the END of the previous
// dispatch — exactly the staleness that produced the multi-path
// coverage parity gate's "covered 645/4368 lines (15%)" hint that
// repeated unchanged across four iterations after the LLM had read
// 2343 lines.
//
// Atomic per-field via the existing SetReadRanges / SetFileTotalLines
// surface; safe to call any number of times — every call replaces
// the snapshot with the current end-of-history view.
//
// Caller MUST NOT hold the closure mutex.
func RefreshClosureCoverage(ctx *types.BusContext, closure *types.EvidenceClosure) {
	if ctx == nil || closure == nil {
		return
	}
	history := dispatchHistory(ctx)
	if len(history) == 0 {
		return
	}
	repoRoot := ""
	if ctx != nil {
		repoRoot = ctx.RepoRoot
	}
	_, readRanges, totals := ExtractReadCoverage(history, repoRoot)
	if len(readRanges) > 0 {
		closure.SetReadRanges(readRanges)
	}
	if len(totals) > 0 {
		closure.SetFileTotalLines(totals)
	}
}

// canonicalCoveragePath canonicalises a banner path with the same
// rules ground.CanonicalRepoRelative applies (when repoRoot is set)
// so coverage map keys collide with the rest of the closure. With an
// empty repoRoot it falls back to slash-form cleanup so unit tests
// that don't care about repo roots keep working.
func canonicalCoveragePath(path, repoRoot string) string {
	if path == "" {
		return ""
	}
	if repoRoot != "" {
		return CanonicalRepoRelative(path, repoRoot)
	}
	cleaned := strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	return strings.TrimPrefix(cleaned, "./")
}

// forcedReadPrefixes lists every trace prefix runForcedReads
// (orchestrator/cgec_enforcers.go) prepends to a synthesised
// read_file Summary so operators can grep the trace. Order matters:
// the more specific prefix MUST come first so we strip
// `[forced_read surgical]` before matching the shorter
// `[forced_read]`.
var forcedReadPrefixes = []string{
	"[forced_read surgical] ",
	"[forced_read] ",
}

// stripForcedReadPrefix removes any forced-read trace prefix from
// the start of `s`. Returns the suffix verbatim when no prefix
// matches. Centralised so every banner parser (ParseReadFileBanner +
// ground.parseBannerPath) strips the same set of prefixes — drift
// between the two would silently lose surgical-read content from
// the LineIndex while it shows up in coverage, or vice versa.
func stripForcedReadPrefix(s string) string {
	for _, p := range forcedReadPrefixes {
		if strings.HasPrefix(s, p) {
			return s[len(p):]
		}
	}
	return s
}

// StripForcedReadPrefix is the package-public wrapper used by
// ground/ground.go's parseBannerPath so the LineIndex builder
// observes the same prefix-stripping policy as the coverage walker.
func StripForcedReadPrefix(s string) string {
	return stripForcedReadPrefix(s)
}
