package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRenderAnswerDocObservationLedgerDisclosesDroppedCategories pins the
// Batch E2 truncation indicator: when the finalize-prompt observation view is
// smaller than the ledger, the render must disclose "(showing N of M ...;
// dropped: origin×count)" instead of a bare count.
func TestRenderAnswerDocObservationLedgerDisclosesDroppedCategories(t *testing.T) {
	mut := types.NewMutableState("enumerate helpers")
	var items []types.EvidenceItem
	for i := 0; i < types.ObservationPromptRecordLimit+8; i++ {
		items = append(items, types.EvidenceItem{
			ID:        fmt.Sprintf("ev-%03d", i),
			Source:    "src/main.go",
			LineStart: i + 1,
			Subject:   fmt.Sprintf("helper%03d", i),
			Summary:   fmt.Sprintf("helper %d definition", i),
		})
	}
	mut.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: items})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentExplain},
		},
	}
	got := renderAnswerDocObservationLedger(ctx)
	wantShowing := fmt.Sprintf("*(showing %d prioritized record(s) of %d total; dropped: current_source×8)*",
		types.ObservationPromptRecordLimit, types.ObservationPromptRecordLimit+8)
	if !strings.Contains(got, wantShowing) {
		t.Fatalf("observation ledger prompt missing dropped-category disclosure %q:\n%s", wantShowing, got)
	}
}

// TestRenderAnswerDocTraceObservationCoverageDisclosesTopTruncation pins the
// Batch E3 indicator: the top-N trace coverage rows must disclose how many
// further trace observations exist beyond the rendered rows.
func TestRenderAnswerDocTraceObservationCoverageDisclosesTopTruncation(t *testing.T) {
	var records []types.ObservationRecord
	for i := 0; i < 8; i++ {
		records = append(records, types.ObservationRecord{
			ID:              fmt.Sprintf("trace:%02d", i),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "critical_blocking",
			ClaimKey:        fmt.Sprintf("critical_blocking:%02d", i),
			Subject:         fmt.Sprintf("thread-%02d", i),
			Summary:         fmt.Sprintf("blocking call %d", i),
		})
	}
	got := renderAnswerDocTraceObservationCoverage(types.ObservationLedger{Records: records})
	if !strings.Contains(got, fmt.Sprintf("- top[%d]", types.TraceCoverageTopObservationPromptLimit)) {
		t.Fatalf("trace coverage should render up to the top-N cap:\n%s", got)
	}
	if strings.Contains(got, fmt.Sprintf("- top[%d]", types.TraceCoverageTopObservationPromptLimit+1)) {
		t.Fatalf("trace coverage rendered beyond the top-N cap:\n%s", got)
	}
	wantDisclosure := fmt.Sprintf("(top view truncated: %d more trace observation(s) beyond the %d shown",
		8-types.TraceCoverageTopObservationPromptLimit, types.TraceCoverageTopObservationPromptLimit)
	if !strings.Contains(got, wantDisclosure) {
		t.Fatalf("trace coverage missing truncation disclosure %q:\n%s", wantDisclosure, got)
	}
	if !strings.Contains(got, "raw trace_query records") {
		t.Fatalf("trace coverage disclosure should point at the raw trace_query records:\n%s", got)
	}
}

// TestRenderStructuredAggregateFactsTruncationDisclosesCategories pins the
// Batch E2 aggregate-fact truncation format: "(showing N of M aggregate facts;
// dropped: kind×count)".
func TestRenderStructuredAggregateFactsTruncationDisclosesCategories(t *testing.T) {
	var facts []types.AnswerAggregateFact
	for i := 0; i < 20; i++ {
		facts = append(facts, types.AnswerAggregateFact{
			Kind:  types.AnswerAggregateTotalCount,
			Label: fmt.Sprintf("count %02d", i),
			Value: "1",
		})
	}
	got := renderStructuredAggregateFacts(facts, 16)
	if !strings.Contains(got, "(showing 16 of 20 aggregate facts; dropped: total_count×4)") {
		t.Fatalf("aggregate fact render missing categorised truncation disclosure:\n%s", got)
	}
}

// TestRenderToolHistoryObservationCheckpointDisclosesDroppedRecords pins the
// checkpoint-layer variant of the Batch E2 truncation indicator.
func TestRenderToolHistoryObservationCheckpointDisclosesDroppedRecords(t *testing.T) {
	mut := types.NewMutableState("checkpoint budget")
	var results []types.ToolResult
	for i := 0; i < 12; i++ {
		results = append(results, types.ToolResult{
			ToolName: "trace_query",
			Success:  true,
			Observations: []types.ObservationRecord{{
				ID:       fmt.Sprintf("trace:%02d", i),
				Origin:   types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query",
				ClaimKey: fmt.Sprintf("wakeup_chain:%02d", i),
				Subject:  fmt.Sprintf("thread-%02d", i),
				Summary:  fmt.Sprintf("wakeup hop %d", i),
			}},
		})
	}
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ctx := &types.AgentContext{Mutable: mut}
	got := renderToolHistoryObservationCheckpoint(ctx, toolHistoryObservationCheckpointRecordLimit)
	if got == "" {
		t.Fatal("expected a rendered observation checkpoint")
	}
	if !strings.Contains(got, fmt.Sprintf("(showing %d of 12 observation record(s)", toolHistoryObservationCheckpointRecordLimit)) {
		t.Fatalf("observation checkpoint missing showing-N-of-M disclosure:\n%s", got)
	}
	if !strings.Contains(got, "dropped: runtime_artifact×4") {
		t.Fatalf("observation checkpoint missing dropped category counts:\n%s", got)
	}
}
