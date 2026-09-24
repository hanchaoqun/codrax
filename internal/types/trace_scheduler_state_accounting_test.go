package types

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestSchedulerStateAccountingDisplayKeepsUnknownAndNonContinuous(t *testing.T) {
	valid := &TraceSchedulerStateAccounting{State: "runnable", Caliber: "cumulative_segments", SegmentCount: 3,
		ObservedEndCount: 1, ObservedEndMs: 2, OpenTailCount: 1, OpenTailMs: 1,
		UnknownClosureCount: 1, UnknownClosureMs: 4, StartClippedCount: 1, EndClippedCount: 1, BoundaryContinuationCount: 1}
	for _, zh := range []bool{false, true} {
		got := TraceSchedulerStateAccountingMeaning(valid, zh)
		for _, want := range map[bool][]string{false: {"cumulative_segments=3", "open_tail=1/1ms", "closure_unknown=1/4ms", "observed_boundary=1/2ms", "start/known_end_clipped=1/1", "boundary_continuations=1", "not a continuous interval"}, true: {"就绪累计3段", "开放尾1段/1ms", "闭合未知1段/4ms", "已观测分段终点1段/2ms", "范围内计入量", "分段终点不等于状态终止"}}[zh] {
			if !strings.Contains(got, want) {
				t.Fatalf("missing %q in %s", want, got)
			}
		}
	}
	for name, change := range map[string]func(*TraceSchedulerStateAccounting){
		"caliber":                 func(v *TraceSchedulerStateAccounting) { v.Caliber = "closed" },
		"count":                   func(v *TraceSchedulerStateAccounting) { v.SegmentCount = 4 },
		"continuation":            func(v *TraceSchedulerStateAccounting) { v.BoundaryContinuationCount = 2 },
		"negative":                func(v *TraceSchedulerStateAccounting) { v.OpenTailMs = -1 },
		"nan":                     func(v *TraceSchedulerStateAccounting) { v.ObservedEndMs = math.NaN() },
		"zero_count_nonzero_time": func(v *TraceSchedulerStateAccounting) { v.SegmentCount--; v.OpenTailCount = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			v := *valid
			change(&v)
			if got := TraceSchedulerStateAccountingMeaning(&v, false); !strings.Contains(got, "closure unknown") || strings.Contains(got, "open_tail=0") {
				t.Fatalf("invalid metadata invented a closed/zero account: %s", got)
			}
		})
	}
	if got := TraceSchedulerStateAccountingMeaning(nil, false); !strings.Contains(got, "closure unknown") {
		t.Fatal(got)
	}
}

func TestSchedulerStateAccountingLedgerProjectionAndCopy(t *testing.T) {
	a := TraceSchedulerStateAccounting{State: "runnable", Caliber: "cumulative_segments", SegmentCount: 1, OpenTailCount: 1, OpenTailMs: 1}
	row := ObservationRecord{ID: "native-state", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", Role: AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard, ProvenanceLane: ObservationProvenanceArtifactSpan,
		SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: "/trace", PayloadRef: "/payload", QueryScopeID: "original-window"},
		Span:      ObservationSpan{StartTs: 1.009, EndTs: 1.01, LineStart: 15, LineEnd: 15}, Predicate: "state_drilldown", Subject: "unfinished-104", Object: "runnable", Value: "1.000", Unit: "ms", StateAccounting: []TraceSchedulerStateAccounting{a}, RichNotes: []string{"source=top_runnable", "chain_required=true", "recursive=false"}}
	result := ToolResult{ToolName: "trace_query", Success: true, Observations: []ObservationRecord{row}}
	before, _ := json.Marshal(result)
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{result}})
	if len(ledger.Records) != 1 || !reflect.DeepEqual(ledger.Records[0].StateAccounting, row.StateAccounting) {
		t.Fatalf("native accounting lost: %+v", ledger)
	}
	projected := ProjectObservationPromptRecords(ledger.Records, nil, nil, DefaultObservationPromptProjectionOptions(1))
	if len(projected) != 1 || !strings.HasPrefix(projected[0].Span, "accounting_scope:") || len(projected[0].Notes) == 0 || !strings.Contains(projected[0].Notes[0], "open-tail states=1") {
		t.Fatalf("projection lost first-class caliber: %+v", projected)
	}
	coverage := BuildTraceObservationCoverage(ledger)
	if len(coverage.TopObservations) != 1 || !strings.Contains(coverage.TopObservations[0].StateAccountingMeaning, "open_tail=1/1ms") {
		t.Fatalf("coverage lost account: %+v", coverage)
	}
	after, _ := json.Marshal(result)
	if string(before) != string(after) {
		t.Fatal("display changed original producer")
	}
	ledger.Records[0].StateAccounting[0].OpenTailCount = 99
	if result.Observations[0].StateAccounting[0].OpenTailCount != 1 {
		t.Fatal("ledger aliases producer accounting")
	}
	m := NewMutableState("accounting")
	m.AppendDispatchToolResult(result)
	result.Observations[0].StateAccounting[0].OpenTailCount = 55
	got := m.DispatchToolResults()
	if got[0].Observations[0].StateAccounting[0].OpenTailCount != 1 {
		t.Fatal("dispatch retained caller-owned metadata")
	}
	got[0].Observations[0].StateAccounting[0].OpenTailCount = 66
	if m.DispatchToolResults()[0].Observations[0].StateAccounting[0].OpenTailCount != 1 {
		t.Fatal("dispatch getter exposed mutable metadata")
	}
	row.StateAccounting = nil
	if got := TraceObservationStateAccountingMeaning(row, false); !strings.Contains(got, "closure unknown") {
		t.Fatal(got)
	}
	row.Producer = "emit_investigation_complete"
	row.StateAccounting = []TraceSchedulerStateAccounting{a}
	if got := TraceObservationStateAccountingMeaning(row, false); got != "" {
		t.Fatalf("non-native data used as native account: %s", got)
	}
}
