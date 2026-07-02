package orchestrator

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestBuildExploreTransientRetryCheckpointHintDisclosesToolResultTruncation
// pins the Batch E1 disclosure lane: when window budgets dropped tool
// results, the transient-retry checkpoint must say so with category counts.
func TestBuildExploreTransientRetryCheckpointHintDisclosesToolResultTruncation(t *testing.T) {
	mut := types.NewMutableState("disclose truncation")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"internal/a.go"},
		ToolResultTruncation: &types.ToolResultTruncationSummary{
			Dropped: 14,
			ByTool:  map[string]int{"grep": 12, "read_file": 2},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	got := o.buildExploreTransientRetryCheckpointHint()
	if !strings.Contains(got, "tool results truncated by window budget: 14 dropped (grep×12, read_file×2)") {
		t.Fatalf("transient-retry checkpoint missing tool-result truncation disclosure:\n%s", got)
	}
}

// TestBuildExploreFactRetryContinuationHintDisclosesToolResultTruncation is
// the fact-retry continuation companion pin.
func TestBuildExploreFactRetryContinuationHintDisclosesToolResultTruncation(t *testing.T) {
	mut := types.NewMutableState("disclose truncation")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"internal/a.go"},
		ToolResultTruncation: &types.ToolResultTruncationSummary{
			Dropped: 3,
			ByTool:  map[string]int{"grep": 3},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	got := o.buildExploreFactRetryContinuationHint(nil)
	if !strings.Contains(got, "tool results truncated by window budget: 3 dropped (grep×3)") {
		t.Fatalf("fact-retry continuation missing tool-result truncation disclosure:\n%s", got)
	}
}

// TestTransientRetryTypedObservationSummaryUsesFullLedgerStatistics pins the
// Batch E2 origins fix: checkpoint origin counts must reflect the FULL
// ledger, not the previous 24-evidence bounded view.
func TestTransientRetryTypedObservationSummaryUsesFullLedgerStatistics(t *testing.T) {
	bus := &types.BusContext{}
	for i := 0; i < 40; i++ {
		bus.EvidenceItems = append(bus.EvidenceItems, types.EvidenceItem{
			ID:        fmt.Sprintf("log-ev-%02d", i),
			Source:    "app.log",
			LineStart: i + 1,
			Origin:    types.ClaimOriginLog,
			Subject:   fmt.Sprintf("error-%02d", i),
			Summary:   fmt.Sprintf("log line %d", i),
		})
	}

	got := transientRetryTypedObservationSummary(bus)
	if !strings.Contains(got, "runtime_artifact:40") {
		t.Fatalf("origin summary should count the full ledger (40 runtime-artifact records), got %q", got)
	}
}
