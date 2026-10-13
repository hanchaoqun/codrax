package orchestrator

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeEventReviewFixtureHMC236(t *testing.T) *types.AnswerRuntimeMeasurementReceipt {
	t.Helper()
	dir := t.TempDir()
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet}}
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("event records"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	bus.Mutable.SetRequestModel(rm)
	path, err := filepath.Abs("../../eval/fixtures/trace_query_jank_field_inventory/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "limit": 100})
	result, err := (&tool.TraceQuery{}).Execute(bus, args)
	if err != nil || !result.Success {
		t.Fatalf("native query: %v / %s", err, result.Summary)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if table.EventData == nil {
			continue
		}
		ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View,
			EventSelection: &types.RuntimeEventSelection{FieldKeys: []string{"event.trace_time_seconds", "jank.appid", "jank.start_ts_ns"}}}
		if types.BindRuntimeMeasurementReceipt(ref, view.RuntimeMeasurementContract) {
			return ref
		}
	}
	t.Fatal("native event selection unavailable")
	return nil
}

func TestNativeEventDiagramDoesNotHideOrFulfilRelationsHMC236(t *testing.T) {
	native := types.AnswerBlock{ID: "native", Kind: types.BlockDiagram, RuntimeMeasurement: nativeEventReviewFixtureHMC236(t)}
	if !isBoundNativeEventFactDiagram(native) {
		t.Fatal("native fact chart not recognized")
	}
	view := diagramRequiredView(types.DiagramSequence)
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{native}}
	if violations := validateDiagramEdgeSupport(doc, view); len(violations) != 1 || violations[0].Kind != types.ViolDiagramEdgeUnsupported {
		t.Fatalf("fact chart satisfied required relation: %+v", violations)
	}
	view.DiagramPlan.Required = false
	if violations := validateDiagramEdgeSupport(doc, view); len(violations) != 0 {
		t.Fatalf("fact chart acquired relation obligation: %+v", violations)
	}
	view.DiagramPlan.Required = true
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "ordinary", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Body: "flowchart TB\nA-->B"}})
	if violations := validateDiagramEdgeSupport(doc, view); len(violations) != 1 || !strings.Contains(violations[0].Detail, "ordinary") {
		t.Fatalf("leading native chart hid invalid sibling: %+v", violations)
	}
	for name, mutate := range map[string]func(*types.AnswerBlock){
		"unbound":           func(b *types.AnswerBlock) { b.RuntimeMeasurement.BoundTable = nil },
		"modified selector": func(b *types.AnswerBlock) { b.RuntimeMeasurement.EventSelection.RowIDs = []string{"event-999"} },
		"model diagram": func(b *types.AnswerBlock) {
			b.Diagram = &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Body: "sequenceDiagram\nA->>B: call"}
		},
		"model text":       func(b *types.AnswerBlock) { b.Text = "model cause" },
		"relation sidecar": func(b *types.AnswerBlock) { b.EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B"}} },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := native
			candidate.RuntimeMeasurement = native.RuntimeMeasurement.Clone()
			mutate(&candidate)
			if isBoundNativeEventFactDiagram(candidate) {
				t.Fatal("impure/unbound chart gained exemption")
			}
		})
	}
}

func TestReviewerUsesSameNativeFactSurfacesHMC236(t *testing.T) {
	ref := nativeEventReviewFixtureHMC236(t)
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, RuntimeMeasurement: ref},
		{ID: "table", Kind: types.BlockTable, RuntimeMeasurement: ref.Clone()},
		{ID: "diagram", Kind: types.BlockDiagram, RuntimeMeasurement: ref.Clone()},
	}}
	if !shouldReviewConsistencyV2(doc) {
		t.Fatal("native summary/table became invisible to reviewer scheduling")
	}
	for _, lang := range []string{"en", "zh"} {
		for _, includeDiagrams := range []bool{false, true} {
			surface := buildReviewerAnswerSurface(doc, nil, lang, includeDiagrams)
			exact := func(block types.AnswerBlock) string {
				return render.StripAuthorityArtifacts(render.RenderAnswerDocument(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{block}}, lang))
			}
			wantBody := exact(doc.Blocks[1])
			if includeDiagrams {
				wantBody += exact(doc.Blocks[2])
			}
			if surface.Summary != exact(doc.Blocks[0]) || surface.Body != wantBody || !strings.Contains(surface.FullMarkdown, "9007199254740993") {
				t.Fatalf("reviewer and visible native surfaces diverged (lang=%s diagrams=%v): %+v", lang, includeDiagrams, surface)
			}
		}
	}
	doc.Blocks[0].RuntimeMeasurement = ref.Clone()
	doc.Blocks[0].RuntimeMeasurement.BoundTable = nil
	surface := buildReviewerAnswerSurface(doc, nil, "en", true)
	if !strings.Contains(surface.Summary, "not bound") || strings.Contains(surface.Summary, "9007199254740993") {
		t.Fatal("reviewer revived stale facts or hid failed binding")
	}
}

func TestReviewerExistingMeasurementTableStillVisibleHMC236(t *testing.T) {
	table := types.RuntimeMeasurementTable{ObservationID: "legacy", View: types.RuntimeMeasurementSummary, Columns: []string{"CPU", "Value"}, Rows: [][]string{{"cpu2", "17 ms"}}}
	ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
	if !types.BindRuntimeMeasurementReceipt(ref, &types.RuntimeMeasurementContract{Tables: []types.RuntimeMeasurementTable{table}}) {
		t.Fatal("legacy fixture failed")
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{Kind: types.BlockTable, RuntimeMeasurement: ref}}}
	if surface := buildReviewerAnswerSurface(doc, nil, "en", false); !strings.Contains(surface.Body, "cpu2") || !strings.Contains(surface.Body, "17 ms") {
		t.Fatalf("legacy native table hidden: %+v", surface)
	}
}
