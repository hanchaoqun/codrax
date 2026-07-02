// Package width is the central width governor for tool result production:
// the single source of truth for the per-tool raw-output width/entry caps
// that decide when a tool result is compacted and when a typed refinement
// hint fires (Batch F4, 2026-07-03).
//
// Four-layer composition — this package is layer 1 and must reference, not
// absorb, the other three:
//
//  1. tool width governor (this package) — producer raw-output caps: how many
//     entries/bytes a tool inlines before compaction + refinement guidance.
//  2. blob offload (internal/tool/blob.go MaxInlineBytes/preview*) — transport
//     layer, already yaml-wired via SetBlobLimits.
//  3. observation view budgets (internal/types/observation_view_budgets.go,
//     E2) — prompt-render layer for typed observation ledgers.
//  4. Turn A window bounds (E1) — tool-result window budgets at handoff.
//
// Also deliberately OUT of this table (each already single-sourced with its
// own wiring): the sourceinventory kernel budget (internal/tool/sourceinventory
// re-exported by source_inventory_exec_budget.go), the per-view trace_query
// result-row capacity table (internal/tracequery/view_capacity.go, E4 — this
// package REFERENCES its exported constants/functions instead of duplicating
// them), and the agent-side explore per-tool invocation caps (a call-count
// budget, different axis).
//
// Contract (same style as observation_view_budgets.go): consumers MUST
// reference this table instead of re-hardcoding literals so the producer caps
// cannot silently drift apart again. All caps here are SOFT-guidance drivers
// (compaction + refinement hints); the only precise flags downstream logic may
// branch on remain ToolRefinementHint.ResultTruncated /
// CandidateBudgetTruncated — never the numeric cap values themselves.
package width

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Default cap values, verbatim from their historical declaration sites
// (builtin.go grepGovernor* block, nativegrep.go, trace_query.go,
// repomap/tool.go). TestWidthDefaultsPinned pins every value; changing one is
// a deliberate test edit, not a refactor side effect.
const (
	DefaultGrepLineEntryThreshold  = 80
	DefaultGrepFileEntryThreshold  = 120
	DefaultGrepByteThreshold       = 16 * 1024
	DefaultGrepLineProductionCap   = 48
	DefaultGrepFileProductionCap   = 80
	DefaultGrepAuxiliaryCap        = 24
	DefaultGrepOtherCap            = 16
	DefaultGrepLineWindowHintMax   = 4
	DefaultGrepLineWindowHalfSpan  = 20
	DefaultGrepDirScanMaxFileBytes = 4 << 20

	DefaultPathDiscoveryCandidateFileLimit = 256

	DefaultReadFilePageWindowDefault = 100
	DefaultReadFilePageWindowMax     = 200

	DefaultTraceQueryStreamSearchMinLimit     = 20
	DefaultTraceQueryTypedEvidenceFactCap     = 64
	DefaultTraceQueryTypedFamilyRowCap        = 32
	DefaultTraceQueryStateDrilldownSummaryCap = 5

	DefaultRepoMapBroadNavigationMaxRows   = 96
	DefaultRepoMapBroadNavigationScanLimit = 4096
	DefaultRepoMapBroadRootDefaultTopN     = 50
	DefaultRepoMapBroadRootMaxTopN         = 100
)

// GrepCaps bounds grep raw-output production (and, via the explicit ListFiles*
// aliases on Caps, list_files enumeration — historically a silent reuse of the
// grep values at the list_files call site).
type GrepCaps struct {
	// LineEntryThreshold / FileEntryThreshold / ByteThreshold decide when a
	// broad grep result is compacted and a refinement hint fires (line-output
	// mode vs files_only mode share the byte threshold).
	LineEntryThreshold int
	FileEntryThreshold int
	ByteThreshold      int
	// LineProductionCap / FileProductionCap / AuxiliaryCap / OtherCap bound
	// the inline rows kept per relevance partition after compaction.
	LineProductionCap int
	FileProductionCap int
	AuxiliaryCap      int
	OtherCap          int
	// LineWindowHintMax / LineWindowHalfSpan shape the follow-up line-window
	// hints computed from production matches on runtime artifacts.
	LineWindowHintMax  int
	LineWindowHalfSpan int
	// DirScanMaxFileBytes caps per-file bytes on broad directory scans when
	// the caller does not specify a limit (explicit single-file searches
	// stream without this cap).
	DirScanMaxFileBytes int64
}

// ReadFileCaps bounds the read_file paging-window suggestion emitted when an
// inline read is clamped by the transport budget.
type ReadFileCaps struct {
	PageWindowDefault int
	PageWindowMax     int
}

// TraceQueryCaps bounds tool-side trace_query surfaces. The per-view result
// row capacity table stays in internal/tracequery (E4 ruling: engine-internal
// clamps need it); EventSearchLimitOverride is the only bridge — a non-zero
// operator override for the event_search default limit that the tool layer
// applies on the outgoing query, leaving the engine table untouched.
type TraceQueryCaps struct {
	// StreamSearchMinLimit floors the auto-window event_search probe limit.
	StreamSearchMinLimit int
	// TypedEvidenceFactCap bounds published evidence-pack observation rows.
	TypedEvidenceFactCap int
	// TypedFamilyRowCap bounds every other per-family typed observation list.
	TypedFamilyRowCap int
	// StateDrilldownSummaryCap bounds rendered state_drilldown summary rows.
	StateDrilldownSummaryCap int
	// EventSearchLimitOverride, when > 0, replaces the engine's event_search
	// default result limit (tracequery capacity table) for calls that did not
	// set an explicit limit. Zero means "engine default".
	EventSearchLimitOverride int
}

// RepoMapCaps bounds repo_map broad-navigation and broad-root lens output.
type RepoMapCaps struct {
	BroadNavigationMaxRows   int
	BroadNavigationScanLimit int
	BroadRootDefaultTopN     int
	BroadRootMaxTopN         int
}

// Caps is the full width-governor table.
type Caps struct {
	Grep GrepCaps
	// PathDiscoveryCandidateFileLimit bounds typed path-discovery candidate
	// lists on grep/list_files/repo_map results (single source; the historical
	// repomap-side duplicate is deleted).
	PathDiscoveryCandidateFileLimit int
	ReadFile                        ReadFileCaps
	TraceQuery                      TraceQueryCaps
	RepoMap                         RepoMapCaps
}

// ListFilesEntryThreshold / ListFilesByteThreshold are the explicit named
// aliases of the grep values that list_files historically reused silently:
// list_files enumeration compaction follows grep's files_only thresholds by
// design, and the alias form keeps that coupling readable and un-forkable.
func (c Caps) ListFilesEntryThreshold() int { return c.Grep.FileEntryThreshold }
func (c Caps) ListFilesByteThreshold() int  { return c.Grep.ByteThreshold }

// TraceQueryEventSearchDefaultLimit is the effective event_search default
// result limit: the operator override when set, otherwise the engine's own
// capacity-table default (E4 single source — referenced, never duplicated).
func (c Caps) TraceQueryEventSearchDefaultLimit() int {
	if c.TraceQuery.EventSearchLimitOverride > 0 {
		return c.TraceQuery.EventSearchLimitOverride
	}
	return tracequery.ViewCapacityFor(tracequery.FallbackViewEventSearch).DefaultLimit
}

// Defaults returns the code-default table.
func Defaults() Caps {
	return Caps{
		Grep: GrepCaps{
			LineEntryThreshold:  DefaultGrepLineEntryThreshold,
			FileEntryThreshold:  DefaultGrepFileEntryThreshold,
			ByteThreshold:       DefaultGrepByteThreshold,
			LineProductionCap:   DefaultGrepLineProductionCap,
			FileProductionCap:   DefaultGrepFileProductionCap,
			AuxiliaryCap:        DefaultGrepAuxiliaryCap,
			OtherCap:            DefaultGrepOtherCap,
			LineWindowHintMax:   DefaultGrepLineWindowHintMax,
			LineWindowHalfSpan:  DefaultGrepLineWindowHalfSpan,
			DirScanMaxFileBytes: DefaultGrepDirScanMaxFileBytes,
		},
		PathDiscoveryCandidateFileLimit: DefaultPathDiscoveryCandidateFileLimit,
		ReadFile: ReadFileCaps{
			PageWindowDefault: DefaultReadFilePageWindowDefault,
			PageWindowMax:     DefaultReadFilePageWindowMax,
		},
		TraceQuery: TraceQueryCaps{
			StreamSearchMinLimit:     DefaultTraceQueryStreamSearchMinLimit,
			TypedEvidenceFactCap:     DefaultTraceQueryTypedEvidenceFactCap,
			TypedFamilyRowCap:        DefaultTraceQueryTypedFamilyRowCap,
			StateDrilldownSummaryCap: DefaultTraceQueryStateDrilldownSummaryCap,
		},
		RepoMap: RepoMapCaps{
			BroadNavigationMaxRows:   DefaultRepoMapBroadNavigationMaxRows,
			BroadNavigationScanLimit: DefaultRepoMapBroadNavigationScanLimit,
			BroadRootDefaultTopN:     DefaultRepoMapBroadRootDefaultTopN,
			BroadRootMaxTopN:         DefaultRepoMapBroadRootMaxTopN,
		},
	}
}

var current = Defaults()

// Current returns a copy of the active table.
func Current() Caps { return current }

// Overrides carries the operator-tunable primary width dimensions
// (codrax.yaml tool_width_* prefix group). Non-positive values keep the code
// default — the same contract as tool.SetBlobLimits. Only these nine
// dimensions are exposed; every other cap stays code-only in the table.
type Overrides struct {
	GrepLineEntryThreshold      int
	GrepFileEntryThreshold      int
	GrepByteThreshold           int
	GrepProductionCap           int // applies to both line and file production caps
	GrepDirScanMaxFileBytes     int64
	PathDiscoveryCandidateLimit int
	ReadFilePageWindowMax       int
	TraceQueryEventSearchLimit  int
	RepoMapNavigationMaxRows    int
}

// Set applies the startup overrides one-shot and returns advisory notes for
// any cross-field sanity clamp it performed (production caps must not exceed
// their entry thresholds; the read_file default window must not exceed the
// window max). Notes are advisory log text for the caller — never gate input.
func Set(o Overrides) []string {
	caps := Defaults()
	if o.GrepLineEntryThreshold > 0 {
		caps.Grep.LineEntryThreshold = o.GrepLineEntryThreshold
	}
	if o.GrepFileEntryThreshold > 0 {
		caps.Grep.FileEntryThreshold = o.GrepFileEntryThreshold
	}
	if o.GrepByteThreshold > 0 {
		caps.Grep.ByteThreshold = o.GrepByteThreshold
	}
	if o.GrepProductionCap > 0 {
		caps.Grep.LineProductionCap = o.GrepProductionCap
		caps.Grep.FileProductionCap = o.GrepProductionCap
	}
	if o.GrepDirScanMaxFileBytes > 0 {
		caps.Grep.DirScanMaxFileBytes = o.GrepDirScanMaxFileBytes
	}
	if o.PathDiscoveryCandidateLimit > 0 {
		caps.PathDiscoveryCandidateFileLimit = o.PathDiscoveryCandidateLimit
	}
	if o.ReadFilePageWindowMax > 0 {
		caps.ReadFile.PageWindowMax = o.ReadFilePageWindowMax
	}
	if o.TraceQueryEventSearchLimit > 0 {
		caps.TraceQuery.EventSearchLimitOverride = o.TraceQueryEventSearchLimit
	}
	if o.RepoMapNavigationMaxRows > 0 {
		caps.RepoMap.BroadNavigationMaxRows = o.RepoMapNavigationMaxRows
	}
	var notes []string
	if caps.Grep.LineProductionCap > caps.Grep.LineEntryThreshold {
		notes = append(notes, fmt.Sprintf("grep line production cap %d clamped to line entry threshold %d", caps.Grep.LineProductionCap, caps.Grep.LineEntryThreshold))
		caps.Grep.LineProductionCap = caps.Grep.LineEntryThreshold
	}
	if caps.Grep.FileProductionCap > caps.Grep.FileEntryThreshold {
		notes = append(notes, fmt.Sprintf("grep file production cap %d clamped to file entry threshold %d", caps.Grep.FileProductionCap, caps.Grep.FileEntryThreshold))
		caps.Grep.FileProductionCap = caps.Grep.FileEntryThreshold
	}
	if caps.ReadFile.PageWindowDefault > caps.ReadFile.PageWindowMax {
		notes = append(notes, fmt.Sprintf("read_file page window default %d clamped to page window max %d", caps.ReadFile.PageWindowDefault, caps.ReadFile.PageWindowMax))
		caps.ReadFile.PageWindowDefault = caps.ReadFile.PageWindowMax
	}
	current = caps
	return notes
}

// Reset restores the code defaults (test helper; production calls Set once at
// startup and never resets).
func Reset() { current = Defaults() }
