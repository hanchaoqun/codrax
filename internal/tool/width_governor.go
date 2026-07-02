package tool

import (
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/width"
)

// width_governor.go is the package-tool façade over the central width
// governor (internal/tool/width) — the same re-export pattern as
// source_inventory_exec_budget.go over the sourceinventory kernel. The leaf
// package is load-bearing: it lets internal/tool/repomap (which imports
// package tool) and package tool share one cap table without a cycle, and it
// keeps the table stdlib+engine-only.
//
// Production code reads the live accessors below so the one-shot
// SetWidthGovernor startup overrides take effect. The two compatibility
// constants at the bottom exist only so tests that construct oversized inputs
// from the historical constant names keep compiling; they are pinned to the
// width defaults and must not be used on new code paths.

// SetWidthGovernor is the controlled startup entry point for the codrax.yaml
// tool_width_* prefix group. cmd/root.go calls it once after loading the
// runtime config; non-positive values keep the code default (the same
// contract as SetBlobLimits). Cross-field sanity clamps performed by the
// governor are surfaced as WARN logs.
func SetWidthGovernor(o width.Overrides) {
	for _, note := range width.Set(o) {
		logging.Warning("[config] tool_width sanity: %s", note)
	}
}

// --- grep producer caps -----------------------------------------------------

func grepWidthLineEntryThreshold() int    { return width.Current().Grep.LineEntryThreshold }
func grepWidthFileEntryThreshold() int    { return width.Current().Grep.FileEntryThreshold }
func grepWidthByteThreshold() int         { return width.Current().Grep.ByteThreshold }
func grepWidthLineProductionCap() int     { return width.Current().Grep.LineProductionCap }
func grepWidthFileProductionCap() int     { return width.Current().Grep.FileProductionCap }
func grepWidthAuxiliaryCap() int          { return width.Current().Grep.AuxiliaryCap }
func grepWidthOtherCap() int              { return width.Current().Grep.OtherCap }
func grepWidthLineWindowHintMax() int     { return width.Current().Grep.LineWindowHintMax }
func grepWidthLineWindowHalfSpan() int    { return width.Current().Grep.LineWindowHalfSpan }
func grepWidthDirScanMaxFileBytes() int64 { return width.Current().Grep.DirScanMaxFileBytes }

// --- list_files caps (explicit named aliases of the grep values) ------------

func listFilesWidthEntryThreshold() int { return width.Current().ListFilesEntryThreshold() }
func listFilesWidthByteThreshold() int  { return width.Current().ListFilesByteThreshold() }

// --- shared path-discovery candidate cap -------------------------------------

func toolWidthPathDiscoveryCandidateFileLimit() int {
	return width.Current().PathDiscoveryCandidateFileLimit
}

// --- read_file paging window --------------------------------------------------

func readFileWidthPageWindowDefault() int { return width.Current().ReadFile.PageWindowDefault }
func readFileWidthPageWindowMax() int     { return width.Current().ReadFile.PageWindowMax }

// --- trace_query tool-side caps ------------------------------------------------

func traceQueryWidthStreamSearchMinLimit() int {
	return width.Current().TraceQuery.StreamSearchMinLimit
}

func traceQueryWidthTypedEvidenceFactCap() int {
	return width.Current().TraceQuery.TypedEvidenceFactCap
}

func traceQueryWidthTypedFamilyRowCap() int {
	return width.Current().TraceQuery.TypedFamilyRowCap
}

func traceQueryWidthStateDrilldownSummaryCap() int {
	return width.Current().TraceQuery.StateDrilldownSummaryCap
}

// traceQueryWidthEventSearchDefaultLimit is the effective event_search default
// limit: the tool_width_trace_query_event_search_limit override when set,
// otherwise the tracequery capacity table's own default (E4 — referenced, not
// duplicated).
func traceQueryWidthEventSearchDefaultLimit() int {
	return width.Current().TraceQueryEventSearchDefaultLimit()
}

// traceQueryWidthEventSearchLimitOverride returns the raw operator override
// (0 = none). The tool layer applies it on outgoing event_search queries that
// carry no explicit limit, leaving the engine capacity table untouched.
func traceQueryWidthEventSearchLimitOverride() int {
	return width.Current().TraceQuery.EventSearchLimitOverride
}

// --- test-construction compatibility constants --------------------------------

// grepGovernorFileEntryThreshold / nativeGrepMaxFileBytes are pinned to the
// governor defaults for tests that build oversized fixtures from the
// historical names. Production code must use the live grepWidth* accessors.
const (
	grepGovernorFileEntryThreshold = width.DefaultGrepFileEntryThreshold
	nativeGrepMaxFileBytes         = width.DefaultGrepDirScanMaxFileBytes
)
