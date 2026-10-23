package agent

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the public native query and actual finalizer, not a copied JSON
// carrier. The original inclusive envelope includes the excluded right edge.
func TestHMC286BroadResourceWindowActualFinalizer(t *testing.T) {
	for _, targets := range []bool{true, false} {
		name := "window_only"
		if targets {
			name = "two_targets"
		}
		t.Run(name, func(t *testing.T) { testBroadResourceWindowActualFinalizerHMC286(t, targets) })
	}
}

func testBroadResourceWindowActualFinalizerHMC286(t *testing.T, targets bool) {
	ctx, _ := resourceSelectionPublicContextHMC248(t)
	if !targets {
		ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
		ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	}
	ctx.AgentName, ctx.Stage = types.AgentExplorer, types.StageExplore
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"path":"captures/capture.data","view":"resource_stack"}`))
	if err != nil || !r.Success {
		t.Fatalf("broad native query: %v %s", err, r.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	ctx.AgentName, ctx.Stage = types.AgentFinalizer, types.StageFinalize
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	var nativeID string
	for _, observation := range r.Observations {
		if p, ok := tool.DecodeTraceResourceStack(observation); ok {
			if p.MatchedEvents != 5 || len(p.Events) != 5 || !p.Window.EndInclusive {
				t.Fatalf("fixture lost broad population: %+v", p)
			}
			nativeID = observation.ID
		}
	}
	if nativeID == "" {
		t.Fatal("native resource observation missing")
	}
	presentation := renderAnswerDocScopedResourceStacks(ctx, answerDocObservationLedger(ctx), answerDocObservationLedger(ctx))
	for _, want := range []string{"10005000000ns", "10015000000ns", "10025000000ns", "10040000000ns", "tid=101", "tid=202", "原查询匹配事件=5"} {
		if !strings.Contains(presentation.text, want) {
			t.Errorf("finite display lost %q", want)
		}
	}
	if strings.Contains(presentation.text, "10050000000ns") {
		t.Error("right-boundary member escaped the requested half-open window")
	}
	if len(presentation.published) != 1 {
		t.Errorf("published original query count=%d", len(presentation.published))
	}
	actual := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(actual, "10015000000ns") || !strings.Contains(actual, "原查询匹配事件=5") {
		t.Error("actual finalizer omitted source-bound window member details")
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	counts := map[types.RuntimeMeasurementView]int{}
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if table.ObservationID != nativeID {
			continue
		}
		counts[table.View] = len(table.Rows)
		if table.HasCompleteNativeDisplayPopulation() || table.CoversMemberSet(&ctx.AnalysisIR.RequestModel) {
			t.Error("finite display gained query completion or requested-population authority")
		}
		if table.View == types.RuntimeMeasurementSummary || table.View == types.RuntimeMeasurementMembers {
			for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
				receipt := &types.AnswerRuntimeMeasurementReceipt{ObservationID: nativeID, View: table.View}
				if !types.BindRuntimeMeasurementReceipt(receipt, view.RuntimeMeasurementContract) {
					t.Fatalf("cannot bind projected %s", table.View)
				}
				text := render.RenderAnswerDocument(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "native", Kind: kind, RuntimeMeasurement: receipt}}}, "zh")
				if !strings.Contains(text, "10015000000") || strings.Contains(text, "10050000000") || strings.Contains(text, "-->") {
					t.Errorf("%s/%s lost target 202, added right edge, or invented relations", kind, table.View)
				}
			}
		}
	}
	if counts[types.RuntimeMeasurementSummary] != 4 || counts[types.RuntimeMeasurementMembers] != 11 {
		t.Errorf("same-source table members/frames: %v", counts)
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if !bytes.Equal(before, after) {
		t.Fatal("display changed original ledger")
	}
}
