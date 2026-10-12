package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeDisplayBindingContext224(t *testing.T, query string) (*types.AgentContext, types.RuntimeMeasurementTable) {
	t.Helper()
	var inputs []loginput.Input
	for _, relative := range []string{"app/session.log", "kernel/session.log.gz"} {
		path, err := filepath.Abs("../../eval/fixtures/hmosperf_log_sources/" + relative)
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, loginput.Input{Path: path})
	}
	catalog, err := loginput.Prepare(context.Background(), inputs, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, LogTriage: &types.LogBundle{},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{
			{Index: 1, Role: types.RequestedAnswerDimensionMemberSet, Label: "record roster", Required: true},
			{Index: 2, Role: types.RequestedAnswerDimensionMemberSet, Label: "record ownership", Required: true},
		}},
	}
	mu := types.NewMutableState("Show the recorded events and their process/thread ownership")
	mu.SetRequestModel(rm)
	dir := t.TempDir()
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "en", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: mu,
		AttachedLogCatalog: catalog, AttachedLog: catalog.Preview(8), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}}
	result, err := (&tool.LogQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(query))
	if err != nil || !result.Success {
		t.Fatalf("query: %v %s", err, result.Summary)
	}
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if table.View == types.RuntimeMeasurementMembers {
			return ctx, table
		}
	}
	t.Fatal("native log table missing")
	return nil, types.RuntimeMeasurementTable{}
}

func TestHMC224NativeDisplayDimensionBindingPublic(t *testing.T) {
	ctx, table := nativeDisplayBindingContext224(t, `{}`)
	execute := func(payload any, patch bool) types.ToolResult {
		params, _ := json.Marshal(payload)
		var out types.ToolResult
		var err error
		if patch {
			out, err = (&tool.EmitAnswerDocumentPatch{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), params)
		} else {
			out, err = (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), params)
		}
		if err != nil || !out.Success {
			t.Fatalf("emit/patch: %v %s", err, out.Summary)
		}
		return out
	}
	execute(map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "The native rows retain their original source and identity."}}}, false)
	if len(table.Rows) != 9 {
		t.Fatalf("want nine rows: %d", len(table.Rows))
	}
	if missing := missingRequestedAnswerDimensionsInDocument(ctx, ctx.Mutable.AnswerDocumentV2()); len(missing) != 2 {
		t.Fatalf("unassigned table auto-claimed dimensions: %+v", missing)
	}
	e := &answerDocumentEvaluator{mu: ctx.Mutable, language: "en"}
	if sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop}); !sig.HintRequested || !strings.Contains(sig.Hint, "requested_dimension_indices") || strings.Contains(sig.Hint, "add_facet_id") {
		t.Fatalf("missing binding needs a metadata-only native hint: %s", sig.Hint)
	}
	selector := map[string]any{"id": "records", "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}, "requested_dimension_indices": []int{1, 2}}
	execute(map[string]any{"add_blocks": []any{selector}, "unchanged_block_ids": []string{"lead"}}, true)
	if missing := missingRequestedAnswerDimensionsInDocument(ctx, ctx.Mutable.AnswerDocumentV2()); len(missing) != 0 {
		t.Fatalf("native display binding ignored: %+v; blocks=%+v choices=%v", missing, ctx.Mutable.AnswerDocumentV2().Blocks, types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.NativeDisplayDimensionChoices())
	}
	rows, tables := 0, 0
	for _, block := range ctx.Mutable.AnswerDocumentV2().Blocks {
		if block.RuntimeMeasurement == nil {
			continue
		}
		rows += len(block.RuntimeMeasurement.BoundTable.Rows)
		tables++
		if !reflect.DeepEqual(block.RuntimeMeasurement.BoundTable.Rows, table.Rows) || len(block.Items) != 0 || block.Text != "" {
			t.Fatal("binding reconstructed producer rows")
		}
	}
	if rows != 9 || tables != 1 {
		t.Fatalf("metadata-only binding duplicated data: %d/%d", rows, tables)
	}
	if table.CoversMemberSet(&ctx.AnalysisIR.RequestModel) || ctx.Mutable.IsInvestigationComplete() {
		t.Fatal("display became completion")
	}
	ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions.Dimensions = append(ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions.Dimensions,
		types.RequestedAnswerDimension{Index: 3, Role: types.RequestedAnswerDimensionSourceLocation, Label: "implementation locations", Required: true},
		types.RequestedAnswerDimension{Index: 4, Role: types.RequestedAnswerDimensionCausalAttribution, Label: "proven cause", Required: true})
	// A new typed analysis gets a fresh per-agent presentation cache.
	ctx = &types.AgentContext{RepoRoot: ctx.RepoRoot, WorkDir: ctx.WorkDir, Language: ctx.Language, AgentName: ctx.AgentName, Stage: ctx.Stage, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR, AttachedLogCatalog: ctx.AttachedLogCatalog, AttachedLog: ctx.AttachedLog}
	missing := missingRequestedAnswerDimensionsInDocument(ctx, ctx.Mutable.AnswerDocumentV2())
	if len(missing) != 2 || missing[0].Index != 3 || missing[1].Index != 4 {
		t.Fatalf("native display consumed independent obligations: %+v", missing)
	}
}
