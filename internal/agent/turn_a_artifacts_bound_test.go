package agent

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func boundTestToolResult(name string, success bool, summaryLen int) types.ToolResult {
	return types.ToolResult{
		ToolName: name,
		Success:  success,
		Summary:  strings.Repeat("x", summaryLen),
	}
}

// TestBoundTurnAToolResultsUnderCapsIsIdentity pins the stable-path guarantee:
// slices inside both caps are returned unchanged (same backing array).
func TestBoundTurnAToolResultsUnderCapsIsIdentity(t *testing.T) {
	in := []types.ToolResult{
		boundTestToolResult("grep", true, 100),
		boundTestToolResult("read_file", true, 100),
	}
	out := boundTurnAToolResults(in, turnAToolResultsWindowCountCap, turnAToolResultsWindowByteCap)
	if len(out) != len(in) || &out[0] != &in[0] {
		t.Fatalf("under-cap slice must be returned unchanged")
	}
}

// TestBoundTurnAToolResultsDropsOldestFirstKeepingOrder pins the count cap:
// oldest entries are dropped first and chronological order is kept.
func TestBoundTurnAToolResultsDropsOldestFirstKeepingOrder(t *testing.T) {
	var in []types.ToolResult
	for i := 0; i < 10; i++ {
		r := boundTestToolResult("grep", true, 10)
		r.RawRef = fmt.Sprintf("ref-%02d", i)
		in = append(in, r)
	}
	out := boundTurnAToolResults(in, 4, 1<<20)
	if len(out) != 4 {
		t.Fatalf("count cap not applied: got %d", len(out))
	}
	for i, r := range out {
		want := fmt.Sprintf("ref-%02d", 6+i)
		if r.RawRef != want {
			t.Fatalf("order/oldest-first drifted at %d: got %s want %s", i, r.RawRef, want)
		}
	}
}

// TestBoundTurnAToolResultsByteCapDropsOldestFirst pins the byte cap.
func TestBoundTurnAToolResultsByteCapDropsOldestFirst(t *testing.T) {
	in := []types.ToolResult{
		boundTestToolResult("grep", true, 4000),
		boundTestToolResult("read_file", true, 4000),
		boundTestToolResult("repo_map", true, 4000),
	}
	// Budget fits roughly one entry (4000 + overhead); the newest must win.
	out := boundTurnAToolResults(in, 100, 5000)
	if len(out) != 1 || out[0].ToolName != "repo_map" {
		t.Fatalf("byte cap should keep only the newest entry, got %d entries (%v)", len(out), out)
	}
}

// TestBoundTurnAToolResultsPreservesSuccessfulInvestigationResult pins the
// gate-preservation rule: when the budget would drop the only successful
// investigation-class result, value-ordered retention keeps it (at its
// chronological position) so InvestigationStructurallyEmpty cannot flip from
// false to true.
func TestBoundTurnAToolResultsPreservesSuccessfulInvestigationResult(t *testing.T) {
	in := []types.ToolResult{
		boundTestToolResult("grep", true, 10), // the only successful investigation-class result
	}
	for i := 0; i < 8; i++ {
		in = append(in, boundTestToolResult("read_file", false, 10))
	}
	out := boundTurnAToolResults(in, 4, 1<<20)
	if len(out) != 4 {
		t.Fatalf("expected the count cap to hold with the preserved result inside it, got %d", len(out))
	}
	if out[0].ToolName != "grep" || !out[0].Success {
		t.Fatalf("successful investigation result not preserved at head: %+v", out[0])
	}

	before := InvestigationStructurallyEmpty(&types.TurnAArtifacts{ToolResults: in}, nil)
	after := InvestigationStructurallyEmpty(&types.TurnAArtifacts{ToolResults: out}, nil)
	if before != after {
		t.Fatalf("bound flipped InvestigationStructurallyEmpty: before=%v after=%v", before, after)
	}
	if after {
		t.Fatalf("investigation with a successful grep must not be structurally empty")
	}
}

// TestMergeTurnAArtifactsWithPriorBoundsConcatenatedToolResults pins that the
// cross-window merge applies the merged caps to the concatenation.
func TestMergeTurnAArtifactsWithPriorBoundsConcatenatedToolResults(t *testing.T) {
	var prior, current types.TurnAArtifacts
	for i := 0; i < turnAToolResultsMergedCountCap; i++ {
		prior.ToolResults = append(prior.ToolResults, boundTestToolResult("grep", true, 10))
	}
	for i := 0; i < 50; i++ {
		r := boundTestToolResult("read_file", true, 10)
		r.RawRef = fmt.Sprintf("current-%02d", i)
		current.ToolResults = append(current.ToolResults, r)
	}
	merged := mergeTurnAArtifactsWithPrior(&prior, current)
	if len(merged.ToolResults) != turnAToolResultsMergedCountCap {
		t.Fatalf("merged slice not bounded: got %d want %d", len(merged.ToolResults), turnAToolResultsMergedCountCap)
	}
	last := merged.ToolResults[len(merged.ToolResults)-1]
	if last.RawRef != "current-49" {
		t.Fatalf("newest current-window entry must survive the merge bound, got %q", last.RawRef)
	}
}

// TestMergeTurnAArtifactsWithPriorKeepsTraceObservationsOverGrepNoise is the
// Batch E1 acceptance pin at the merge level: a window sequence with a grep
// flood and deterministic trace_query observations must, after the over-cap
// merge, keep every trace observation, drop grep noise first, and record the
// truncation for checkpoint disclosure.
func TestMergeTurnAArtifactsWithPriorKeepsTraceObservationsOverGrepNoise(t *testing.T) {
	traceResult := func(id string) types.ToolResult {
		return types.ToolResult{
			ToolName: "trace_query",
			Success:  true,
			Summary:  "typed trace observations " + id,
			Observations: []types.ObservationRecord{{
				ID:       id,
				Origin:   types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query",
				Summary:  "wakeup chain hop",
			}},
		}
	}
	var prior, current types.TurnAArtifacts
	for i := 0; i < 5; i++ {
		prior.ToolResults = append(prior.ToolResults, traceResult(fmt.Sprintf("trace-%d", i)))
	}
	for i := 0; i < turnAToolResultsMergedCountCap+20; i++ {
		r := boundTestToolResult("grep", true, 10)
		r.RawRef = fmt.Sprintf("grep-%03d", i)
		current.ToolResults = append(current.ToolResults, r)
	}

	merged := mergeTurnAArtifactsWithPrior(&prior, current)
	if len(merged.ToolResults) != turnAToolResultsMergedCountCap {
		t.Fatalf("merged slice not bounded: got %d want %d", len(merged.ToolResults), turnAToolResultsMergedCountCap)
	}
	traceKept := 0
	for _, r := range merged.ToolResults {
		if r.ToolName == "trace_query" {
			traceKept++
		}
	}
	if traceKept != 5 {
		t.Fatalf("deterministic trace_query observations must survive the grep flood, kept %d of 5", traceKept)
	}
	if merged.ToolResults[0].ToolName != "trace_query" {
		t.Fatalf("chronological order must be preserved (prior trace results first): %+v", merged.ToolResults[0])
	}
	if !merged.ToolResultTruncation.Active() {
		t.Fatal("merged truncation summary must be recorded for checkpoint disclosure")
	}
	if merged.ToolResultTruncation.ByTool["grep"] != merged.ToolResultTruncation.Dropped ||
		merged.ToolResultTruncation.ByTool["trace_query"] != 0 {
		t.Fatalf("grep noise must be dropped first: %+v", merged.ToolResultTruncation)
	}
}

// TestMergeTurnAArtifactsWithPriorUnderCapsKeepsFullConcat pins the default
// path: typical (under-cap) merges keep the full concatenation, unchanged.
func TestMergeTurnAArtifactsWithPriorUnderCapsKeepsFullConcat(t *testing.T) {
	prior := types.TurnAArtifacts{ToolResults: []types.ToolResult{boundTestToolResult("grep", true, 10)}}
	current := types.TurnAArtifacts{ToolResults: []types.ToolResult{boundTestToolResult("read_file", true, 10)}}
	merged := mergeTurnAArtifactsWithPrior(&prior, current)
	if len(merged.ToolResults) != 2 ||
		merged.ToolResults[0].ToolName != "grep" || merged.ToolResults[1].ToolName != "read_file" {
		t.Fatalf("under-cap merge changed the concatenation: %+v", merged.ToolResults)
	}
}

// TestExplorerCaptureSiteUsesBoundedToolResults is the structural pin for the
// snapshot-time bound: the TurnAArtifacts capture in explorer.go must route
// ToolResults through boundTurnAToolResultsWithTruncation (per-window caps +
// truncation disclosure) so the bound cannot be silently dropped by a
// refactor.
func TestExplorerCaptureSiteUsesBoundedToolResults(t *testing.T) {
	src, err := os.ReadFile("explorer.go")
	if err != nil {
		t.Fatal(err)
	}
	idx := strings.Index(string(src), "windowToolResults, windowToolTruncation := boundTurnAToolResultsWithTruncation(")
	if idx < 0 {
		t.Fatalf("bounded per-window capture call not found in explorer.go")
	}
	window := string(src)[idx:]
	if end := strings.Index(window, "// Cross-window accumulation"); end > 0 {
		window = window[:end]
	}
	if !strings.Contains(window, "toolResults, turnAToolResultsWindowCountCap, turnAToolResultsWindowByteCap)") {
		t.Fatalf("capture site no longer applies the per-window caps:\n%s", window)
	}
	if !strings.Contains(window, "ToolResults:                      windowToolResults") ||
		!strings.Contains(window, "ToolResultTruncation:             windowToolTruncation") {
		t.Fatalf("capture literal no longer consumes the bounded results + truncation summary:\n%s", window)
	}
}
