package agent

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSchedulerStateAccountingActualFinalizerOpenTailAndClosedMembers(t *testing.T) {
	ctx, result, concurrency := schedulerMeasurementPublicContext(t, "")
	// Request both named threads so the unchanged ten-row family display
	// budget includes the open-tail witness as well as the primary thread.
	ctx.AnalysisIR.RequestModel.RuntimeTargets = append(ctx.AnalysisIR.RequestModel.RuntimeTargets,
		types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 104, Thread: "unfinished", Source: "user_request"})
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies = append(ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies,
		types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactCountOrDuration)
	before, _ := json.Marshal(result)
	var open types.ObservationRecord
	for _, row := range result.Observations {
		if row.Subject == "unfinished-104" && row.Predicate == "state_drilldown" && row.Object == "runnable" {
			open = row
			break
		}
	}
	if open.ID == "" || open.Value != "1.000" || open.Span.StartTs != 1.009 || open.Span.EndTs != 1.01 || len(open.StateAccounting) != 1 {
		t.Fatalf("actual open-tail cumulative row/value/scope lost: %+v", open)
	}
	a := open.StateAccounting[0]
	if a.Caliber != "cumulative_segments" || a.SegmentCount != 1 || a.OpenTailCount != 1 || math.Abs(a.OpenTailMs-1) > 1e-9 || a.ObservedEndCount != 0 || a.EndClippedCount != 0 {
		t.Fatalf("native open tail mislabeled: %+v", a)
	}
	payloadBytes, err := os.ReadFile(open.SourceRef.PayloadRef)
	if err != nil {
		t.Fatal(err)
	}
	var payload tracequery.Result
	if err := json.Unmarshal(payloadBytes, &payload); err != nil || payload.WindowStats == nil {
		t.Fatalf("raw payload: %v", err)
	}
	var native *types.TraceSchedulerStateAccounting
	for _, step := range payload.WindowStats.StateDrilldownPlan {
		if step.Thread.PID == 104 && step.State == "runnable" {
			native = step.Accounting
		}
	}
	if native == nil || !reflect.DeepEqual(*native, a) {
		t.Fatalf("Run -> payload -> tool changed account: %v / %+v", native, a)
	}
	ledger := answerDocObservationLedger(ctx)
	found := false
	expectedSource := open.SourceRef
	expectedSource.ToolCallID = "trace_query[0]" // existing ledger stamp, not a new source identity
	for _, row := range ledger.Records {
		if row.ID == open.ID {
			found = reflect.DeepEqual(row.StateAccounting, open.StateAccounting) && reflect.DeepEqual(row.SourceRef, expectedSource) && row.Span == open.Span && row.Value == open.Value
		}
	}
	if !found {
		t.Fatal("accepted ledger lost native account or changed source/value/window")
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"open_tail=1/1ms", "times are accounted contributions", "measurement_scope=`1.009000..1.010000`", "Cumulative contributions, not continuous/actual endpoints", "unknown-closure states=0"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("actual finalizer lost %q", want)
		}
	}
	if strings.Contains(prompt, "These rows are the exact finite fact families") {
		t.Fatal("open/unknown state accounts presented as universally finite")
	}
	// The existing closed population remains independently bound. Context
	// with an open tail must not enter any confirmed member selection.
	if len(concurrency.Groups) != 2 {
		t.Fatal("closed concurrency groups changed")
	}
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	if len(choices) != 8 {
		t.Fatalf("closed table catalog changed: %d", len(choices))
	}
	for _, table := range choices {
		if table.View != types.RuntimeMeasurementMembers {
			continue
		}
		for _, cells := range table.Rows {
			if strings.Contains(strings.Join(cells, " "), "unfinished") {
				t.Fatal("open tail entered confirmed members")
			}
		}
	}
	for _, zh := range []bool{false, true} {
		line := traceQueryObservationSupplementText(open, zh)
		want := "open_tail=1/1ms"
		if zh {
			want = "开放尾1段/1ms"
		}
		if !strings.Contains(line, want) {
			t.Fatalf("reader appendix lost open caliber: %s", line)
		}
	}
	coverage := types.TraceObservationCoverageFromObservationRecords([]types.ObservationRecord{open})
	if got := renderTraceObservationCoverageForStageReport(coverage); !strings.Contains(got, "open_tail=1/1ms") {
		t.Fatalf("stage report lost closure: %s", got)
	}
	after, _ := json.Marshal(result)
	if string(before) != string(after) || ctx.Mutable.TraceRootCauseReport() != nil {
		t.Fatal("display changed producer or granted root cause")
	}
	// Legacy accepted records lack endpoint metadata. Neither public filled
	// LineEnd nor a plausible prose summary can manufacture physical closure.
	legacy := open
	legacy.StateAccounting = nil
	legacy.Summary = "closed exactly at 1.010, all complete"
	legacy.RichNotes = append([]string{"closure=closed", "observed_end_count=1"}, legacy.RichNotes...)
	legacyResult := result
	legacyResult.Observations = []types.ObservationRecord{legacy}
	ctx.Mutable.ResetDispatchToolResults()
	ctx.Mutable.AppendDispatchToolResult(legacyResult)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{legacyResult}})
	legacyPrompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(legacyPrompt, "closure unknown") || strings.Contains(legacyPrompt, "observed_boundary=1/") || strings.Contains(legacyPrompt, "observation_id=\"") {
		t.Fatal("legacy prose recovered a missing closure or table receipt")
	}
}

func TestSchedulerStateAccountingCompactActualCheckpointKeepsAllStates(t *testing.T) {
	ctx, result, _ := schedulerMeasurementPublicContext(t, "")
	var row types.ObservationRecord
	for _, candidate := range result.Observations {
		if candidate.Predicate == "state_churn" {
			row = candidate
			break
		}
	}
	if row.ID == "" {
		t.Fatal("missing native churn fixture")
	}
	row.StateAccounting = nil
	for i, state := range []string{"running", "runnable", "s_sleep", "d_sleep", "io_wait"} {
		a := types.TraceSchedulerStateAccounting{State: state, Caliber: "cumulative_segments", SegmentCount: 1, ObservedEndCount: 1, ObservedEndMs: 1}
		if i == 3 {
			a.ObservedEndCount = 0
			a.ObservedEndMs = 0
			a.OpenTailCount = 1
			a.OpenTailMs = 1
		}
		if i == 4 {
			a.ObservedEndCount = 0
			a.ObservedEndMs = 0
			a.UnknownClosureCount = 1
			a.UnknownClosureMs = 1
		}
		row.StateAccounting = append(row.StateAccounting, a)
	}
	result.Observations = []types.ObservationRecord{row}
	ctx.Mutable.ResetDispatchToolResults()
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	compact := types.TraceObservationStateAccountingCompact(row)
	if len(compact) > toolHistoryObservationCheckpointNoteMaxLen {
		t.Fatalf("compact exceeds existing shortest budget: %d: %s", len(compact), compact)
	}
	for name, text := range map[string]string{
		"checkpoint":   renderToolHistoryObservationCheckpoint(ctx, 8),
		"160char_note": renderAnswerDocObservationNotes([]string{compact}, 1),
		"initial":      (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil),
	} {
		for _, want := range []string{"Cumulative contributions, not continuous/actual endpoints", "states=5", "open-tail states=1", "unknown-closure states=1"} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s lost %q: %s", name, want, text)
			}
		}
	}
	// Compact summaries count state accounts, not summable overlapping ms.
	if strings.Contains(compact, "5ms") {
		t.Fatal("overlapping state accounts were summed")
	}
}
