package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC255SupplementaryResourceCannotClaimRequestedDimension(t *testing.T) {
	ctx := resourceScopeContextHMC255(t)
	ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{Dimensions: []types.RequestedAnswerDimension{{Index: 1, Role: types.RequestedAnswerDimensionMemberSet, Label: "requested owners", Required: true}}}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	checked := 0
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if table.View != types.RuntimeMeasurementMembers || !strings.Contains(table.Label, "补充查询") {
			continue
		}
		checked++
		block := types.AnswerBlock{ID: "facts", Kind: types.BlockTable, RequestedDimensionIndices: []int{1}, RuntimeMeasurement: &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}}
		if !types.BindRuntimeMeasurementReceipt(block.RuntimeMeasurement, view.RuntimeMeasurementContract) {
			t.Fatal("supplementary display lost")
		}
		if _, valid := types.BoundNativeDisplayDimensions(block, view.RuntimeMeasurementContract); valid {
			t.Fatal("supplement covered requested dimension")
		}
		raw, _ := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "Native records only."}, nativeDisplayBlock224(table, 1)}})
		result, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
		if err == nil && result.Success || !strings.Contains(result.Summary, "requested_dimension_indices") {
			t.Fatalf("public emit failed to distinguish display and coverage: %v %s", err, result.Summary)
		}
	}
	if checked != 2 {
		t.Fatalf("checked=%d", checked)
	}
}

func TestHMC255SupplementaryResourceBudgetKeepsRequestedQueriesFirst(t *testing.T) {
	ctx := resourceScopeContextHMC255(t)
	results := ctx.Mutable.TurnAArtifacts().ToolResults
	for _, path := range []string{"captures/capture.data", "captures/nested/capture.data"} {
		raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "resource_stack", "pid": 101, "time_start": 10.001, "time_end": 10.049})
		r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
		if err != nil || !r.Success {
			t.Fatalf("actual extra query: %v %s", err, r.Summary)
		}
		results = append(results, r)
	}
	// Arrival order must not spend the four-query budget on supplements first.
	results = []types.ToolResult{results[1], results[3], results[0], results[2], results[4], results[5]}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ledger := answerDocObservationLedger(ctx)
	shown := renderAnswerDocScopedResourceStacks(ctx, ledger, ledger)
	if len(shown.published) != 4 || !strings.Contains(shown.text, "独立补充查询已展示=0，受预算省略=2") {
		t.Fatalf("budget status wrong: published=%d %s", len(shown.published), shown.text)
	}
	for _, r := range shown.published {
		if r.SourceRef.QueryTargetPID != 101 {
			t.Fatal("supplement displaced requested query")
		}
	}
}

func TestHMC255SupplementaryResourceDoesNotBorrowMatchingID(t *testing.T) {
	ctx := resourceScopeContextHMC255(t)
	ledger := answerDocObservationLedger(ctx)
	var original types.ObservationRecord
	for _, r := range ledger.Records {
		if r.Predicate == types.RuntimeResourceMeasurementPredicate && r.SourceRef.QueryTargetPID == 202 {
			p, ok := tool.DecodeTraceResourceStack(r)
			if ok && len(p.Events) > 0 {
				original = r
				break
			}
		}
	}
	if original.ID == "" {
		t.Fatal("missing exact event query")
	}
	for _, mode := range []string{"source", "payload", "window", "native_value"} {
		t.Run(mode, func(t *testing.T) {
			changed := original
			p, _ := tool.DecodeTraceResourceStack(changed)
			switch mode {
			case "source":
				p.SourcePath += ".copy"
				changed.SourceRef.Path = p.SourcePath
			case "payload":
				changed.SourceRef.PayloadRef += ".copy"
			case "window":
				p.Window.StartTs = 10.001
				changed.SourceRef.QueryWindowStartTs, changed.Span.StartTs = p.Window.StartTs, p.Window.StartTs
			case "native_value":
				p.Events[0].Frames[0].Symbol.Value = "HMC255_FORGED_FRAME"
			}
			changed = resourceSelectionRecordPayloadHMC248(t, changed, p)
			mixed := ledger
			mixed.Records = append(append([]types.ObservationRecord(nil), ledger.Records...), changed)
			for _, c := range resourceStackSupplementaryQueries(ctx, mixed) {
				if reflect.DeepEqual(c.record, changed) && c.supplementary {
					t.Fatalf("%s borrowed current same-ID qualification", mode)
				}
			}
			if got := renderAnswerDocScopedResourceStacks(ctx, mixed, mixed).text; strings.Contains(got, "HMC255_FORGED_FRAME") {
				t.Fatal("conflicting native value reached finalizer")
			}
		})
	}
}

func TestHMC255SupplementaryResourceOriginalCannotBorrowCurrentReceipt(t *testing.T) {
	for _, mode := range []string{"source", "payload", "window", "native_value"} {
		t.Run(mode, func(t *testing.T) {
			ctx := resourceScopeContextHMC255(t)
			results := ctx.Mutable.TurnAArtifacts().ToolResults
			var original types.ObservationRecord
			for _, r := range results[1].Observations {
				if r.Predicate == types.RuntimeResourceMeasurementPredicate {
					original = r
				}
			}
			publication, valid := types.DecodeRuntimeMeasurementPublication(original)
			if !valid {
				t.Fatal("need native original")
			}
			changed := original
			p, _ := tool.DecodeTraceResourceStack(changed)
			switch mode {
			case "source":
				p.SourcePath += ".copy"
				changed.SourceRef.Path = p.SourcePath
			case "payload":
				changed.SourceRef.PayloadRef += ".copy"
			case "window":
				p.Window.StartTs = 10.001
				changed.SourceRef.QueryWindowStartTs, changed.Span.StartTs = p.Window.StartTs, p.Window.StartTs
			case "native_value":
				p.Events[0].Frames[0].Symbol.Value = "HMC255_UNCURRENT_FRAME"
			}
			changed = resourceSelectionRecordPayloadHMC248(t, changed, p)
			changed, valid = types.PublishRuntimeResourceMeasurement(changed, publication.Tables, false)
			if !valid || !types.RuntimeResourceQueryIsSupplementary(changed, &ctx.AnalysisIR.RequestModel) {
				t.Fatal("fixture needs a private native publication with the same observation ID")
			}
			// A native publication without this execution's private read/replay
			// receipt must not borrow the unrelated current same-ID result.
			stale := types.ToolResult{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{changed}}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: append(results, stale)})
			input := types.ObservationLedgerInputFromAgentContext(ctx, 0)
			current := false
			for _, table := range types.BuildRuntimeMeasurementContract(input).Choices() {
				current = current || table.ObservationID == changed.ID
			}
			if !current {
				t.Fatal("fixture must retain a valid same-ID contract entry")
			}
			if types.RuntimeResourceSupplementaryQueryCurrent(input, stale, changed) {
				t.Fatal("stale original borrowed another result's receipt")
			}
			ledger := answerDocObservationLedger(ctx)
			ledger.Records = append(ledger.Records, changed)
			for _, c := range resourceStackSupplementaryQueries(ctx, ledger) {
				if reflect.DeepEqual(c.record, changed) && c.supplementary {
					t.Fatal("same-ID stale original reached supplementary handoff")
				}
			}
		})
	}
}
