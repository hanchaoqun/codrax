package agent

import (
	"bytes"
	"encoding/json"
	"html"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC287OriginalRangeActualFinalizerAndSurfaces(t *testing.T) {
	ctx, results := resourceSelectionPublicContextHMC248(t)
	before := hmc218CausalProjection(t, ctx)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	var metadata *types.RuntimeMeasurementTable
	for _, choice := range view.RuntimeMeasurementContract.Choices() {
		if strings.HasPrefix(choice.ObservationID, "trace_source_metadata:") {
			copy := choice
			metadata = &copy
			break
		}
	}
	if metadata == nil {
		t.Fatal("native original-range metadata did not reach the answer contract")
	}
	assertRange := func(label, text string) {
		t.Helper()
		text = html.UnescapeString(text) // Native Markdown preserves exact field names with entity escaping.
		for _, value := range []string{"trace_range", "source_range", "10000000000", "10100000000"} {
			if !strings.Contains(text, value) {
				t.Errorf("%s lost original range field %q: %s", label, value, text)
			}
		}
	}
	encoded, _ := json.Marshal(metadata)
	assertRange("table", string(encoded))
	actual := dependencyObservationMessages(t, ctx)
	assertRange("actual finalizer messages", actual)
	if metadata.MemberSet != nil || metadata.DisplayPopulation != nil || metadata.HasCompleteNativeDisplayPopulation() || metadata.CoversMemberSet(&ctx.AnalysisIR.RequestModel) {
		t.Fatal("source declaration gained population or completion authority")
	}
	for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
		receipt := &types.AnswerRuntimeMeasurementReceipt{ObservationID: metadata.ObservationID, View: metadata.View}
		if !types.BindRuntimeMeasurementReceipt(receipt, view.RuntimeMeasurementContract) {
			t.Fatal("cannot bind native range display")
		}
		text := render.RenderAnswerDocument(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "range", Kind: kind, RuntimeMeasurement: receipt}}}, "zh")
		assertRange(string(kind), text)
	}
	if !bytes.Equal(before, hmc218CausalProjection(t, ctx)) || ctx.Mutable.TraceRootCauseReport() != nil {
		t.Fatal("range display changed trace causal authority")
	}
	// Serialization cannot restore the private current-source preparation seal.
	wire, _ := json.Marshal(results)
	var restored []types.ToolResult
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: restored})
	for _, choice := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if strings.HasPrefix(choice.ObservationID, "trace_source_metadata:") {
			t.Fatal("JSON result revived native range metadata credential")
		}
	}
}

// Existing resource contracts still assert their exact twelve views/four
// defaults. Source metadata is an independent display-only publication.
func resourceOnlyChoicesHMC287(t *testing.T, choices []types.RuntimeMeasurementTable) []types.RuntimeMeasurementTable {
	t.Helper()
	var out []types.RuntimeMeasurementTable
	metadata := 0
	for _, table := range choices {
		if table.HasNativeResourceFacts() {
			out = append(out, table)
			continue
		}
		if !strings.HasPrefix(table.ObservationID, "trace_source_metadata:") || table.MemberSet != nil || table.HasCompleteNativeDisplayPopulation() {
			t.Fatalf("unexpected non-resource authority: %+v", table)
		}
		metadata++
	}
	if metadata != 2 {
		t.Fatalf("two original sources need two independent metadata views, got %d", metadata)
	}
	return out
}

func resourceOnlyDefaultsHMC287(t *testing.T, ctx *types.AgentContext, contract *types.RuntimeMeasurementContract) int {
	t.Helper()
	count := 0
	for _, receipt := range types.RuntimeNativeFactDisplaySelections(&ctx.AnalysisIR.RequestModel, contract) {
		if !types.BindRuntimeMeasurementReceipt(&receipt, contract) {
			t.Fatal("default cannot bind current native facts")
		}
		if receipt.BoundTable.HasNativeResourceFacts() {
			count++
		}
	}
	return count
}
