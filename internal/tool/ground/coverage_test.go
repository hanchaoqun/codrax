package ground

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func readCoverageResult(path string, start, end, total int) types.ToolResult {
	return types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[" + path + ": rendered banner retained for transparency]\n",
		ReadCoverage: &types.ToolReadCoverage{
			Path:       path,
			LineStart:  start,
			LineEnd:    end,
			TotalLines: total,
		},
	}
}

// TestParseReadFileBanner_BothShapes locks the two banner shapes the
// read_file tool emits. Without the "showing all" branch, inline-
// expanded full-file reads (small files where Limit was overridden)
// would be invisible to coverage accounting.
func TestParseReadFileBanner_BothShapes(t *testing.T) {
	cases := []struct {
		name      string
		summary   string
		wantPath  string
		wantStart int
		wantEnd   int
		wantTotal int
		wantOK    bool
	}{
		{
			name:      "showing lines",
			summary:   "[internal/repl/repl.go: showing lines 1382-2047 of 4368 total]\n   1382│ ...\n",
			wantPath:  "internal/repl/repl.go",
			wantStart: 1382,
			wantEnd:   2047,
			wantTotal: 4368,
			wantOK:    true,
		},
		{
			name:      "showing all",
			summary:   "[a.go: showing all 113 lines (4096 bytes); limit=20 expanded to full file (inline-sized; pass offset>0 for explicit paging)]\n     1│ ...",
			wantPath:  "a.go",
			wantStart: 1,
			wantEnd:   113,
			wantTotal: 113,
			wantOK:    true,
		},
		{
			name:    "missing prefix",
			summary: "no banner here",
			wantOK:  false,
		},
		{
			name:    "malformed range",
			summary: "[x.go: showing lines 5-3 of 10 total]\n",
			wantOK:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, rng, total, ok := ParseReadFileBanner(tc.summary)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if path != tc.wantPath {
				t.Errorf("path = %q, want %q", path, tc.wantPath)
			}
			if rng.Start != tc.wantStart || rng.End != tc.wantEnd {
				t.Errorf("range = %d-%d, want %d-%d", rng.Start, rng.End, tc.wantStart, tc.wantEnd)
			}
			if total != tc.wantTotal {
				t.Errorf("total = %d, want %d", total, tc.wantTotal)
			}
		})
	}
}

// TestExtractReadCoverage_AccumulatesAcrossPagination is the core
// invariant: when the LLM paginates a single file across N read_file
// calls, ExtractReadCoverage records every range on the closure's
// per-file slice so SetReadRanges (which calls mergeLineRanges) ends
// up with the union. Failure here would mean the closure
// MergedReadLines stays at the first chunk's count even after the
// LLM read the whole file — the exact stale-coverage failure mode
// the multi-path coverage parity gate hit in production.
func TestExtractReadCoverage_AccumulatesAcrossPagination(t *testing.T) {
	history := []types.ToolResult{
		readCoverageResult("internal/repl/repl.go", 1, 645, 4368),
		readCoverageResult("internal/repl/repl.go", 1382, 2047, 4368),
		readCoverageResult("internal/repl/repl.go", 2048, 2713, 4368),
	}
	readSet, ranges, totals := ExtractReadCoverage(history, "")
	if !readSet["internal/repl/repl.go"] {
		t.Fatalf("readSet missing the file: %+v", readSet)
	}
	if got := len(ranges["internal/repl/repl.go"]); got != 3 {
		t.Fatalf("expected 3 typed coverage ranges accumulated, got %d: %+v", got, ranges["internal/repl/repl.go"])
	}
	if totals["internal/repl/repl.go"] != 4368 {
		t.Errorf("total = %d, want 4368", totals["internal/repl/repl.go"])
	}
}

// TestExtractReadCoverage_IgnoresFailedAndNonReadFile guards against
// inflating coverage with grep / exec_command results or with
// read_file calls that errored out. Banner parser already returns
// !ok on a non-banner Summary; this check defends the outer walk.
func TestExtractReadCoverage_IgnoresFailedAndNonReadFile(t *testing.T) {
	history := []types.ToolResult{
		{
			ToolName: "read_file",
			Success:  false,
			Summary:  "[internal/repl/repl.go: showing lines 1-100 of 4368 total]\n",
			ReadCoverage: &types.ToolReadCoverage{
				Path:       "internal/repl/repl.go",
				LineStart:  1,
				LineEnd:    100,
				TotalLines: 4368,
			},
		},
		{ToolName: "grep", Success: true, Summary: "internal/repl/repl.go:42:foo\n"},
		{ToolName: "exec_command", Success: true, Summary: "internal/repl/repl.go\n"},
	}
	readSet, ranges, totals := ExtractReadCoverage(history, "")
	if len(readSet) != 0 {
		t.Errorf("failed reads + non-read-file results must not populate readSet, got %+v", readSet)
	}
	if len(ranges) != 0 {
		t.Errorf("ranges must stay empty, got %+v", ranges)
	}
	if len(totals) != 0 {
		t.Errorf("totals must stay empty, got %+v", totals)
	}
}

// TestRefreshClosureCoverage_PicksUpInDispatchReads is the integration
// invariant. The motivating bug: a tool that fires mid-dispatch
// (emit_investigation_complete) saw closure.MergedReadLines frozen
// at the END of the previous dispatch's ParseOutput, so reads issued
// in the current ReAct loop were invisible until the next dispatch
// finished. RefreshClosureCoverage must refresh from the live
// DispatchToolResults so any subsequent gate evaluation reads the
// up-to-date per-file range count.
func TestRefreshClosureCoverage_PicksUpInDispatchReads(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	// Seed an older snapshot with only the first read.
	closure.SetReadRanges(map[string][]types.LineRange{
		"internal/repl/repl.go": {{Start: 1, End: 645}},
	})
	closure.SetFileTotalLines(map[string]int{"internal/repl/repl.go": 4368})
	if got := closure.MergedReadLines("internal/repl/repl.go"); got != 645 {
		t.Fatalf("seed coverage = %d, want 645", got)
	}

	// Now simulate the LLM having issued two more reads in the current
	// dispatch — these land in DispatchToolResults but the closure's
	// readRanges are still at the seeded snapshot until refresh.
	mut := types.NewMutableState("test")
	mut.AppendDispatchToolResult(readCoverageResult("internal/repl/repl.go", 1, 645, 4368))
	mut.AppendDispatchToolResult(readCoverageResult("internal/repl/repl.go", 1382, 2047, 4368))
	mut.AppendDispatchToolResult(readCoverageResult("internal/repl/repl.go", 2048, 2713, 4368))
	ctx := &types.BusContext{Mutable: mut}

	RefreshClosureCoverage(ctx, closure)

	got := closure.MergedReadLines("internal/repl/repl.go")
	want := 645 + 666 + 666 // 1-645 + 1382-2047 + 2048-2713
	if got != want {
		t.Fatalf("after refresh, MergedReadLines = %d, want %d (%d-line file, three pagination chunks)", got, want, 4368)
	}
	if total := closure.FileTotalLines("internal/repl/repl.go"); total != 4368 {
		t.Errorf("FileTotalLines = %d, want 4368", total)
	}
}

// TestRefreshClosureCoverage_NilSafe ensures no panic on degenerate
// input. The refresh runs from inside Execute paths that may receive
// a nil ctx / closure / Mutable in tests, so every guard fires.
func TestRefreshClosureCoverage_NilSafe(t *testing.T) {
	RefreshClosureCoverage(nil, nil) // should not panic

	closure := types.NewEvidenceClosure("")
	RefreshClosureCoverage(nil, closure)                 // nil ctx
	RefreshClosureCoverage(&types.BusContext{}, nil)     // nil closure
	RefreshClosureCoverage(&types.BusContext{}, closure) // empty ctx → no-op
}

// TestParseReadFileBanner_StripsForcedReadPrefix pins the load-bearing
// fix for the finalizer-grounder × forced-read death corner: when
// runForcedReads (orchestrator/cgec_enforcers.go) prepends a trace
// prefix like `[forced_read surgical] ` or `[forced_read] ` to the
// read_file Summary, the banner parser must strip the prefix BEFORE
// looking for the path. Without the strip, the parser splits on the
// FIRST `: ` it finds — which lives inside the actual banner's
// `path: showing lines ...` — producing a malformed path key like
// `forced_read surgical] [internal/repl/repl.go` that misses every
// LineIndex / readRanges lookup. The result is that surgical
// recovery reads land in the closure under the wrong key and
// finalizer citations against those lines fail grounding.
func TestParseReadFileBanner_StripsForcedReadPrefix(t *testing.T) {
	cases := []struct {
		name      string
		summary   string
		wantPath  string
		wantStart int
		wantEnd   int
	}{
		{
			name:      "surgical prefix",
			summary:   "[forced_read surgical] [internal/repl/repl.go: showing lines 100-130 of 4368 total]\n   100│ ...\n",
			wantPath:  "internal/repl/repl.go",
			wantStart: 100,
			wantEnd:   130,
		},
		{
			name:      "legacy prefix",
			summary:   "[forced_read] [internal/repl/repl.go: showing lines 1-645 of 4368 total]\n   1│ ...\n",
			wantPath:  "internal/repl/repl.go",
			wantStart: 1,
			wantEnd:   645,
		},
		{
			name:      "no prefix unchanged",
			summary:   "[a.go: showing lines 5-10 of 50 total]\n",
			wantPath:  "a.go",
			wantStart: 5,
			wantEnd:   10,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, rng, _, ok := ParseReadFileBanner(tc.summary)
			if !ok {
				t.Fatalf("expected ok=true on %s; got summary=%q", tc.name, tc.summary)
			}
			if path != tc.wantPath {
				t.Errorf("path = %q, want %q (prefix-strip failed)", path, tc.wantPath)
			}
			if rng.Start != tc.wantStart || rng.End != tc.wantEnd {
				t.Errorf("range = %d-%d, want %d-%d", rng.Start, rng.End, tc.wantStart, tc.wantEnd)
			}
		})
	}
}

// TestExtractReadCoverage_ForcedReadResultsLandInClosure pins the
// integration: a synthesised forced-read ToolResult (with trace
// prefix) flowing through ExtractReadCoverage produces the SAME
// closure entries as a clean LLM-issued read. Without the prefix
// strip, the closure's readRanges would not register the surgical
// recovery, so the next gate pass would re-demand the same lines
// (infinite loop, bounded only by stall detection at 3 wasted
// iterations).
func TestExtractReadCoverage_ForcedReadResultsLandInClosure(t *testing.T) {
	history := []types.ToolResult{
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[forced_read surgical] [internal/repl/repl.go: showing lines 100-130 of 4368 total]\n   100│ x\n",
			ReadCoverage: &types.ToolReadCoverage{
				Path:       "internal/repl/repl.go",
				LineStart:  100,
				LineEnd:    130,
				TotalLines: 4368,
			},
		},
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[forced_read] [internal/repl/repl.go: showing lines 200-230 of 4368 total]\n   200│ y\n",
			ReadCoverage: &types.ToolReadCoverage{
				Path:       "internal/repl/repl.go",
				LineStart:  200,
				LineEnd:    230,
				TotalLines: 4368,
			},
		},
	}
	readSet, ranges, totals := ExtractReadCoverage(history, "")
	if !readSet["internal/repl/repl.go"] {
		t.Fatalf("forced-read with prefix MUST register in readSet; got %+v", readSet)
	}
	if got := len(ranges["internal/repl/repl.go"]); got != 2 {
		t.Errorf("expected 2 forced-read ranges in closure; got %d: %+v", got, ranges)
	}
	if totals["internal/repl/repl.go"] != 4368 {
		t.Errorf("totalLines = %d, want 4368 (typed coverage total survives forced-read summary prefix)", totals["internal/repl/repl.go"])
	}
}

func TestExtractReadCoverage_IgnoresSummaryOnlyReadFileBanner(t *testing.T) {
	history := []types.ToolResult{
		{ToolName: "read_file", Success: true, Summary: "[internal/repl/repl.go: showing lines 1-645 of 4368 total]\n"},
	}
	readSet, ranges, totals := ExtractReadCoverage(history, "")
	if len(readSet) != 0 || len(ranges) != 0 || len(totals) != 0 {
		t.Fatalf("summary-only read_file banner must not populate coverage: set=%+v ranges=%+v totals=%+v", readSet, ranges, totals)
	}
}
