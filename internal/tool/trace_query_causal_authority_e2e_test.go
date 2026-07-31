package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestTraceCausalPublicationAuthorityGenericCrossArtifactCoverageQuery is the
// production-path pin for the real E2 comparison shape. Generic trace
// coverage/sample questions may observe semantic spans while collecting
// window statistics, but a background semantic optimization point is not an
// independently proved root cause and must not mint the full causal report.
//
// The test deliberately executes the same six trace_query calls as the eval
// rather than hand-constructing a projection. This catches compiler-side
// duplication of one semantic row into another projection bucket.
func TestTraceCausalPublicationAuthorityGenericCrossArtifactCoverageQuery(t *testing.T) {
	fixtures := map[string]string{
		"frame.systrace": "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace",
		"short.systrace": "../../eval/fixtures/real_traces/donghu_short_excerpt.systrace",
	}
	dir := t.TempDir()
	for dst, src := range fixtures {
		raw, err := os.ReadFile(src)
		if err != nil {
			t.Skipf("real trace fixture unavailable: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, dst), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx := &types.BusContext{
		RepoRoot: dir,
		WorkDir:  dir,
		Mutable:  types.NewMutableState("compare trace coverage and sampled lanes"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioGeneric,
			Predicates: types.SemanticPredicates{
				IsCrossComponent: true,
			},
		}},
	}
	calls := []string{
		`{"source":"path","path":"frame.systrace","view":"window_stats"}`,
		`{"source":"path","path":"short.systrace","view":"window_stats"}`,
		`{"source":"path","path":"short.systrace","view":"event_search","event_types":["trace_mark"],"pattern":"VSync"}`,
		`{"source":"path","path":"frame.systrace","view":"event_search","event_types":["trace_mark"],"pattern":"VSync"}`,
		`{"source":"path","path":"short.systrace","view":"event_search","event_types":["cpu_frequency"],"pattern":"cpu_frequency"}`,
		`{"source":"path","path":"frame.systrace","view":"event_search","event_types":["cpu_frequency"],"pattern":"cpu_frequency","limit":10}`,
	}
	for _, params := range calls {
		result, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(params))
		if err != nil {
			t.Fatalf("trace_query %s: %v", params, err)
		}
		if !result.Success {
			t.Fatalf("trace_query %s unsuccessful: %s", params, result.Summary)
		}
		ctx.ToolResults = append(ctx.ToolResults, result)
	}
	maxFrequencyRows := 0
	maxClockSetRateRows := 0
	for _, result := range ctx.ToolResults {
		if authority := result.TraceEvidenceAuthority; authority != nil {
			maxFrequencyRows = max(maxFrequencyRows, authority.FrequencyTransitionEventCount)
			maxClockSetRateRows = max(maxClockSetRateRows, authority.FrequencyClockSetRateEventCount)
		}
	}
	if maxFrequencyRows == 0 || maxClockSetRateRows == 0 {
		t.Fatalf("real mixed-frequency fixture must keep cpu_frequency and clock_set_rate on separate non-empty typed lanes: cpu_frequency_rows=%d clock_set_rate_events=%d",
			maxFrequencyRows, maxClockSetRateRows)
	}

	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(
		ctx, types.ObservationExtractLedgerEvidenceLimit,
	))
	set := types.CompileTraceCausalProjectionSet(ledger)
	if runtimeTraceProjectionSetHasPublicationGradeCausalRows(set) {
		t.Fatalf("generic cross-artifact coverage query must not gain causal publication authority from background semantic rows: %+v", set.Projections)
	}
	if runtimeTraceCausalProjectionMaterializationAllowed(ctx, set) {
		t.Fatal("generic cross-artifact coverage query must not materialize Trace causal projection")
	}
	if runtimeTraceFullReportMaterializationAllowed(ctx) {
		t.Fatal("generic cross-artifact coverage query must not inherit the full runtime trace report")
	}

	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "typed comparison result",
		}},
	}
	if _, err := persistMergedAnswerDocument(
		ctx, "emit_answer_document", types.MutationReplaceAll, "cross-artifact coverage", doc, time.Now(),
	); err != nil {
		t.Fatalf("persist generic comparison answer: %v", err)
	}
	for _, block := range doc.Blocks {
		if RuntimeTraceSystemBlock(block) {
			t.Fatalf("generic cross-artifact coverage query gained unrelated runtime report block %q", block.ID)
		}
	}
}
