package agent

import (
	"bytes"
	"encoding/json"
	"html"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC230BusinessMeasurementActualFinalizerEmitRender(t *testing.T) {
	ctx := hmosBusinessIOFinalizerContext(t)
	// This test requests factual state accounting. The independently tested
	// causal/work-relation gates must not be satisfied by a measurement table.
	rm := &ctx.AnalysisIR.RequestModel
	rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}}
	ctx.Mutable.SetRequestModel(*rm)
	before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	projectionBefore, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))))
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	wanted := map[string]types.RuntimeMeasurementTable{}
	for _, table := range contract.Choices() {
		if strings.HasPrefix(table.Label, "业务区间量测 — ") {
			wanted[table.ObservationID] = table
		}
	}
	if len(wanted) != 4 {
		t.Fatalf("both original views must retain selectors, choices=%d want4", len(wanted))
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, types.TraceNoteKeyRuntimeMeasurement+"={") {
		t.Fatal("private publication JSON leaked into factual handoff")
	}
	for id := range wanted {
		if !strings.Contains(prompt, id) {
			t.Fatal("native selector absent from actual finalizer handoff")
		}
	}
	params, _ := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "两段业务的原始量测如下；这些状态本身不证明响应根因。"}}})
	out, err := (&tool.EmitAnswerDocument{}).Execute(bus, params)
	if err != nil || !out.Success {
		t.Fatalf("emit: %v %s", err, out.Summary)
	}
	doc := ctx.Mutable.AnswerDocumentV2()
	seen := map[string]bool{}
	seenBusiness := map[string]bool{}
	for _, block := range doc.Blocks {
		if block.RuntimeMeasurement == nil {
			continue
		}
		if table, ok := wanted[block.RuntimeMeasurement.ObservationID]; ok {
			if seen[table.ObservationID] || seenBusiness[table.Label] || !block.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(table.Rows, block.RuntimeMeasurement.BoundTable.Rows) {
				t.Fatal("business native facts duplicated or changed at emit")
			}
			seen[table.ObservationID] = true
			seenBusiness[table.Label] = true
		}
	}
	if len(seen) != 2 {
		t.Fatal("default native display lost business partitions")
	}
	visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
	for _, want := range []string{"业务区间量测", "OpenDocument", "LoadDocumentIndex", "1.004500", "1.044500", "运行（ms）", "不能跨嵌套业务相加"} {
		if !strings.Contains(visible, want) {
			t.Errorf("render lost %q", want)
		}
	}
	after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	projectionAfter, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))))
	if !bytes.Equal(before, after) || !bytes.Equal(projectionBefore, projectionAfter) {
		t.Fatal("native display mutated evidence or causal projection")
	}
}
