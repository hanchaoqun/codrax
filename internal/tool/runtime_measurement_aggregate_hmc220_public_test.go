package tool

import (
	"encoding/json"
	"html"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A normal investigation may inspect one capture before asking for a pair.
// Both public results retain the same producer table; collection order must
// not make that table ambiguous or remove it from the emitted final answer.
func TestHMC220NativeMeasurementSingleThenPairPublic(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		ctx, pair := hmc220PairPublicFixture(t)
		report, ok := pair.RuntimeMeasurementPair.Report()
		if !ok {
			t.Fatal("missing accepted pair report")
		}
		single, err := (&TraceQuery{}).Execute(ctx, report.Sides[0].Request)
		if err != nil || !single.Success {
			t.Fatalf("single: %v / %s", err, single.Summary)
		}
		results := []types.ToolResult{single, pair}
		if reverse {
			results[0], results[1] = results[1], results[0]
		}
		rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
		rm.Language, rm.PerfTrace = "en", &types.PerfBundle{}
		ctx.AnalysisIR.RequestModel = rm
		ctx.Mutable.SetRequestModel(rm)
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
		choices := types.BuildAnswerSemanticViewForBusContext(ctx).RuntimeMeasurementContract.Choices()
		if len(choices) != 4 {
			t.Fatalf("single + pair removed shared native facts (reverse=%t): got %d choices, want 4", reverse, len(choices))
		}
		out := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": []any{
			map[string]any{"id": "interpretation", "kind": "summary", "text": "The unavailable side does not prove a zero value."},
		}}, false)
		if !out.Success {
			t.Fatal(out.Summary)
		}
		tables, rows := 0, 0
		for _, block := range ctx.Mutable.AnswerDocumentV2().Blocks {
			if block.RuntimeMeasurement != nil && block.RuntimeMeasurement.IsBound() {
				tables++
				rows += len(block.RuntimeMeasurement.BoundTable.Rows)
			}
		}
		if tables != 2 || rows != 15 {
			t.Fatalf("summary-only final answer lost or duplicated producer rows: %d tables / %d rows", tables, rows)
		}
		visible := html.UnescapeString(render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "en"))
		for _, exact := range []string{"9007199254740993", "未知（NULL）", "failed", "baseline", "current"} {
			if !strings.Contains(visible, exact) {
				t.Errorf("final answer missing %q", exact)
			}
		}
		// The merged selection must still be a pure receipt, not retyped data.
		encoded, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
		if strings.Contains(string(encoded), "9007199254740993") {
			t.Fatal("producer rows leaked into model-owned document payload")
		}
	}
}

func TestHMC220NativeMeasurementPairPublicationConflictPublic(t *testing.T) {
	ctx, pair := hmc220PairPublicFixture(t)
	report, _ := pair.RuntimeMeasurementPair.Report()
	single, err := (&TraceQuery{}).Execute(ctx, report.Sides[0].Request)
	if err != nil || !single.Success {
		t.Fatalf("single: %v / %s", err, single.Summary)
	}
	for i := range single.Observations {
		publication, ok := types.DecodeRuntimeMeasurementPublication(single.Observations[i])
		if !ok {
			continue
		}
		publication.Tables[0].Rows[0][0] = "conflicting source value"
		encoded, _ := json.Marshal(publication)
		single.Observations[i].RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(encoded)}
	}
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
	rm.PerfTrace = &types.PerfBundle{}
	ctx.AnalysisIR.RequestModel = rm
	ctx.Mutable.SetRequestModel(rm)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{single, pair}})
	choices := types.BuildAnswerSemanticViewForBusContext(ctx).RuntimeMeasurementContract.Choices()
	if len(choices) != 1 || choices[0].ObservationID != report.ID {
		t.Fatalf("one conflicting view left sibling views selectable: %+v", choices)
	}
	out := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": []any{
		map[string]any{"id": "lead", "kind": "summary", "text": "Conflicting values cannot be resolved from the remaining views."},
	}}, false)
	if !out.Success {
		t.Fatal(out.Summary)
	}
	visible := html.UnescapeString(render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "en"))
	if strings.Contains(visible, "9007199254740993") || strings.Contains(visible, "conflicting source value") {
		t.Fatal("final answer selected a version of an ambiguous publication")
	}
}
