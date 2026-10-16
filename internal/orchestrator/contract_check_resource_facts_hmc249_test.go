package orchestrator

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC249ResourceFactDiagramDoesNotSupplyRelationships(t *testing.T) {
	dir := t.TempDir()
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("resource facts"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain, RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet}}}
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_catalog_objects/captures/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "resource_stack", "pid": 101, "time_start": 10, "time_end": 10.05})
	result, err := (&tool.TraceQuery{}).Execute(bus, args)
	if err != nil || !result.Success {
		t.Fatalf("query: %v %s", err, result.Summary)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	contract := types.BuildAnswerSemanticViewForBusContext(bus).RuntimeMeasurementContract
	count := 0
	for _, table := range contract.Choices() {
		if !table.HasNativeResourceFacts() {
			continue
		}
		count++
		for _, caliber := range []types.TraceCausalClaimCaliber{"", types.TraceCausalClaimNoConclusion} {
			ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
			if !types.BindRuntimeMeasurementReceipt(ref, contract) {
				t.Fatal("native bind")
			}
			block := types.AnswerBlock{ID: "resource-facts", Kind: types.BlockDiagram, RuntimeMeasurement: ref, TraceCausalClaimCaliber: caliber}
			if !isBoundNativeEventFactDiagram(block) {
				t.Fatal("fixed facts not recognized")
			}
			view := diagramRequiredView(types.DiagramSequence)
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{block}}
			if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 1 || vs[0].Kind != types.ViolDiagramEdgeUnsupported {
				t.Fatal("fact rows satisfied relationship obligation")
			}
			doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "ordinary", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Body: "flowchart TB\nA-->B"}})
			if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 1 || !strings.Contains(vs[0].Detail, "ordinary") {
				t.Fatal("native rows masked another diagram")
			}
			block.RuntimeMeasurement = ref.Clone()
			block.RuntimeMeasurement.BoundTable.Rows[0][0] = "other"
			if isBoundNativeEventFactDiagram(block) {
				t.Fatal("modified table retained pure-fact qualification")
			}
		}
	}
	if count != 2 {
		t.Fatalf("expected two native views, got%d", count)
	}
}
