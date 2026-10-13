package agent

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This is a deterministic context test, not evidence that a live model chose
// the right branch or naturally retried a failed query.
func TestSamplingFinalizerKeepsExactTreeAndFlatAuditFactsHMC237(t *testing.T) {
	ctx := samplingSelectionAnalysisContext(t)
	params := json.RawMessage(`{"source":"attached_trace","view":"perf_stats","pid":11,"time_start":1,"time_end":2}`)
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("public stats query: %v %+v", err, result)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal(ledger)
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		`producer="raw_perfdata_fallback" event="cycles"`,
		`producer="raw_perfdata_fallback" event="instructions"`,
		`producer="simpleperf_report_sample" event="cycles"`,
		"total_weight=35 stack_weight=30 unavailable_weight=5",
		"total_weight=7 stack_weight=7 unavailable_weight=0",
		"total_weight=100 stack_weight=100 unavailable_weight=0",
		`symbol="sys;root" dso="/system/lib.so" ip="0x1" inclusive=30 self=0`,
		`symbol="work" dso="/app/A.so" ip="0x2" inclusive=10 self=10`,
		`symbol="work" dso="/app/B.so" ip="0x3" inclusive=20 self=20`,
		"cohort_percent=85.714286", "cohort_percent=28.571429", "cohort_percent=57.142857",
		`unavailable_stacks=1 reasons={"legacy_frame_boundaries_unavailable":1}`,
		"Self weight is the last recorded frame, not proof of complete unwinding.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("finalizer lost native frame boundary %q", want)
		}
	}
	if strings.Contains(prompt, "inclusive=900") || strings.Contains(prompt, "total_weight=935") || strings.Contains(prompt, "old merged cycles=42") {
		t.Fatal("out-of-window or superseded pre-triage facts leaked into the finalizer")
	}
	if decided, allowed := types.RuntimeTraceReportShapeAuthority(&ctx.AnalysisIR.RequestModel); !decided || allowed {
		t.Fatal("sampling display gained causal report authority")
	}
	// The named-target prompt may omit flat rows in favor of the native tree.
	// That presentation choice must not reduce A's recorded-field weight from
	// 15 to 10 or strip the distinct caliber from the lossless audit records.
	flat, generic := 0, 0
	for _, record := range ledger.Records {
		if record.Predicate != "perf_sample_top_symbol" || record.Object != "/app/A.so" {
			continue
		}
		if record.Value == "15" {
			flat++
			if !strings.Contains(record.Summary, "denominator_weight=35") || !strings.Contains(strings.Join(record.RichNotes, " "), "perf_value_basis=recorded_sample_fields_not_prefix_node") {
				t.Errorf("native flat sample weight lost its independent caliber: %+v", record)
			}
		}
		if strings.Contains(record.Summary, "sample_weight=15 denominator=35") {
			generic++
			if !strings.Contains(record.Summary, "sample_weight=15 denominator=35 value_basis=recorded_sample_fields_not_prefix_node") {
				t.Errorf("generic flat evidence lost its independent caliber: %+v", record)
			}
		}
	}
	if flat != 1 || generic != 1 {
		t.Fatalf("expected both exact public flat observation routes, got native=%d generic=%d", flat, generic)
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if !bytes.Equal(before, after) {
		t.Fatal("finalizer projection rewrote the lossless audit ledger")
	}
}
