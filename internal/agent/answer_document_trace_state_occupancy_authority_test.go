package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func traceStateOccupancyTestProjection(state string, notes ...string) types.TraceCausalProjection {
	return types.CompileTraceCausalProjection(types.ObservationLedger{Records: []types.ObservationRecord{traceStateOccupancyTestRecord(state, notes...)}})
}

func traceStateOccupancyTestRecord(state string, notes ...string) types.ObservationRecord {
	base := []string{"rank=1", "tier=primary", "chain_relevance=on_chain", "dominant_state=" + state,
		"impact_ms=7.405", "effective_impact_ms=7.405", "cumulative_impact_ms=8.294", "selected_window=10.000000..10.100000"}
	base = append(base, notes...)
	return types.ObservationRecord{
		ID: "trace_query:fixture#root_cause_rank:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "fixture.trace", ArtifactKind: "trace"},
		Span:      types.ObservationSpan{StartTs: 10, EndTs: 10.1, LineStart: 1, LineEnd: 10},
		Predicate: "root_cause_primary", ClaimKey: "root_cause_primary:worker-200", Subject: "worker-200",
		Object: "priority_inversion_candidate", Value: "7.405", Unit: "ms", RichNotes: base,
	}
}

func TestTraceReaderStateOccupancyUsesPublishedStateNotPricedImpact(t *testing.T) {
	for _, tc := range []struct {
		state, note string
		measured    float64
	}{
		{"running", "running=8.294", 8.294},
		{"fragmented_running", "running=3.299", 3.299},
		{"runnable", "runnable=0.109", 0.109},
		{"s_sleep", "sleep=3.324", 3.324},
		{"d_sleep", "d_state=3.598", 3.598},
		{"io_wait", "io_wait=1.759", 1.759},
	} {
		t.Run(tc.state, func(t *testing.T) {
			projection := traceStateOccupancyTestProjection(tc.state, tc.note)
			if len(projection.PrimaryRootCauses) == 0 {
				t.Fatal("compiler did not retain the test rank seat")
			}
			set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}}
			before, _ := json.Marshal(set)
			for _, lang := range []string{"zh", "en"} {
				got := renderTraceFinalReaderDecisionCards(set, nil, lang, nil, types.TraceWakeupTargetCPUIntegrity{}, false)
				want := fmt.Sprintf("measured %.3f ms", tc.measured)
				wrong := "measured 7.405 ms"
				if lang == "zh" {
					want = fmt.Sprintf("已测 %.3f 毫秒", tc.measured)
					wrong = "已测 7.405 毫秒"
				}
				if !strings.Contains(got, want) || strings.Contains(got, wrong) {
					t.Fatalf("state measurement was substituted by a priced amount:\n%s", got)
				}
				if !strings.Contains(got, "7.405") {
					t.Fatal("repair candidate value was lost")
				}
			}
			after, _ := json.Marshal(set)
			if string(before) != string(after) {
				t.Fatal("fact rendering mutated the projection")
			}
		})
	}
}

func TestTraceReaderMissingStateOccupancyDoesNotInventMeasuredZero(t *testing.T) {
	for _, state := range []string{"running", "runnable", "s_sleep", "d_sleep", "io_wait", "unknown"} {
		t.Run(state, func(t *testing.T) {
			set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{traceStateOccupancyTestProjection(state)}}
			got := renderTraceFinalReaderDecisionCards(set, nil, "zh", nil, types.TraceWakeupTargetCPUIntegrity{}, false)
			if strings.Contains(got, "已测 7.405") || strings.Contains(got, "已测 0.000") || !strings.Contains(got, "原始状态占用未提供") {
				t.Fatalf("missing state measurement was not disclosed honestly:\n%s", got)
			}
		})
	}
}

func TestTraceReaderPublishedZeroStateOccupancyIsNotMissing(t *testing.T) {
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{traceStateOccupancyTestProjection("running", "running=0.000")}}
	got := renderTraceFinalReaderDecisionCards(set, nil, "zh", nil, types.TraceWakeupTargetCPUIntegrity{}, false)
	if !strings.Contains(got, "已测 0.000 毫秒") || strings.Contains(got, "原始状态占用未提供") {
		t.Fatalf("published zero lost its presence:\n%s", got)
	}
}

func TestTraceStateOccupancyProductionFinalizerHandoffKeepsRawAndPricedAxes(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(false)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 200, Thread: "worker-200", Source: "user_explicit"}}
	row := traceStateOccupancyTestRecord("running", "running=8.294", "runnable=0.109", "sleep=3.324", "d_state=3.598", "io_wait=0.000",
		"supply_fold_deficit_ms=7.296", "supply_fold_ideal_ms=0.998", "fold_basis=known=8.294ms,unknown=0.000ms")
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, TraceEvidenceAuthority: &types.TraceEvidenceAuthority{View: "root_cause_rank"}, Observations: []types.ObservationRecord{row},
	}}})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"measured_state_occupancy=8.294ms", "effective_attribution=7.405ms", "folded_running_total=8.294ms",
		"kind=`running`; window_projection=8.294ms", "running time, measured 8.294 ms", "eliminable impact 7.405 ms",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("production prompt missed original/priced ruler %q:\n%s", want, prompt)
		}
	}
	for _, wrong := range []string{"measured_state_occupancy=7.405ms", "running time, measured 7.405", "kind=`running`; window_projection=7.405ms"} {
		if strings.Contains(prompt, wrong) {
			t.Fatalf("production prompt substituted priced impact as wall clock %q", wrong)
		}
	}
	if strings.LastIndex(prompt, "running time, measured 8.294 ms") < strings.LastIndex(prompt, "## Submission Checklist") {
		t.Fatal("reader fact card was not wired to the finalizer tail")
	}
}

func TestTraceWaitCallsiteCardDoesNotBorrowAnotherStateMeasurement(t *testing.T) {
	projection := traceStateOccupancyTestProjection("d_state", "d_state=3.598", "io_wait=1.759", "blocked_reason_caller=wait_site")
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}}
	got := renderTraceFinalReaderDecisionCards(set, nil, "zh", nil, types.TraceWakeupTargetCPUIntegrity{}, false)
	marker := strings.Index(got, "- 链上等待证据边界")
	if marker < 0 {
		t.Fatal("callsite card missing")
	}
	if !strings.Contains(got[marker:], "已测 3.598 毫秒") || strings.Contains(got[marker:], "已测 1.759 毫秒") {
		t.Fatalf("D-state label borrowed IO partition's duration:\n%s", got[marker:])
	}
}

func TestTraceMeasuredSpanRequiresOriginalObservationNotRankLabels(t *testing.T) {
	rank := types.TraceCausalProjectionNode{Predicate: "root_cause_primary", Unit: "ms", SemanticClass: "jit_compile", SpanName: "JitCompile", ImpactMS: 3, EffectiveImpactMS: 3, ActualImpactMS: 10}
	if _, ok := traceFinalMeasuredStateOccupancy(rank); ok {
		t.Fatal("rank labels do not turn priced chain participation into original span duration")
	}
	original := rank
	original.Predicate = "trace_semantic_span"
	original.ImpactMS = 12
	original.FamilyMemberCount = 4 // Producer-owned family duration, not a display fold.
	if value, ok := traceFinalMeasuredStateOccupancy(original); !ok || value != 12 {
		t.Fatalf("original span duration lost: (%v,%v)", value, ok)
	}
	for _, unit := range []string{"", "us", "composite_score"} {
		wrongUnit := original
		wrongUnit.Unit = unit
		if _, ok := traceFinalMeasuredStateOccupancy(wrongUnit); ok {
			t.Fatalf("non-ms original span acquired millisecond authority: %q", unit)
		}
	}
	fold := original
	fold.MergedCount = 2
	if _, ok := traceFinalMeasuredStateOccupancy(fold); ok {
		t.Fatal("display-fold seed acquired whole-population span authority")
	}
	original.StateKind = "running"
	if _, ok := traceFinalMeasuredStateOccupancy(original); ok {
		t.Fatal("span duration cannot stand in for a missing running state account")
	}
}
