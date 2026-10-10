package orchestrator

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRequiredRuntimeDiagramPostCheckCannotWaiveAvailableNativeRelation(t *testing.T) {
	start, end := 1.0, 1.05
	bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("draw observed handoffs"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
			RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1–1.05"},
		}, AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{Required: true, RequiredKind: types.DiagramFlow}}},
	}
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_transaction_handoffs/events.systrace")
	raw, _ := json.Marshal(map[string]any{"path": path, "view": "transaction_handoffs", "time_start": start, "time_end": end})
	r, err := (&tool.TraceQuery{}).Execute(bus, raw)
	if err != nil || !r.Success {
		t.Fatalf("native query: %v %+v", err, r)
	}
	bus.Mutable.AppendDispatchToolResult(r)
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow,
		DiagramPlan:                   &types.DiagramFacetGraph{Required: true, Kind: types.DiagramFlow, RequireStructuralEdge: true},
		DiagramParticipantObligations: []types.DiagramParticipantHint{{Identity: "Submitter", Role: types.DiagramParticipantIncidentRequired}, {Identity: "Consumer", Role: types.DiagramParticipantIncidentRequired}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "diagram", Kind: types.BlockDiagram,
		Diagram:               &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n Submitter\n Consumer"},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{{Participant: "Submitter", Status: types.DiagramParticipantBoundaryUnproven}, {Participant: "Consumer", Status: types.DiagramParticipantBoundaryUnproven}},
	}}}
	if !tool.DiagramParticipantUnprovenBoundaryShapeComplete(&doc.Blocks[0], view) {
		t.Fatal("fixture must reach old typed boundary waiver")
	}
	vs := validateDiagramEdgeSupportWithRuntimeContext(doc, view, bus)
	if len(vs) != 1 || vs[0].Kind != types.ViolRequiredDiagramEdgeAbsent || vs[0].RepairLocusOverride != types.LocusFinalizer {
		t.Fatalf("post-check must retain the local authoring failure despite unproven role rows: %+v", vs)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	rows := tool.RuntimeDiagramRelations(ledger, &bus.AnalysisIR.RequestModel)
	if len(rows) != 3 {
		t.Fatalf("expected exact native three relations: %+v", rows)
	}
	row := rows[0]
	doc.Blocks[0].Diagram.Body += fmt.Sprintf("\n %s[\"提交记录\"] --> %s[\"消费记录\"]", row.FromNode, row.ToNode)
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: row.FromNode, ToNode: row.ToNode, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity, RelationKind: row.Kind}}
	if vs := validateDiagramEdgeSupportWithRuntimeContext(doc, view, bus); len(vs) != 0 {
		t.Fatalf("one actual native relation should meet minimum visual delivery without inventing role mapping: %+v", vs)
	}
}

func TestRequiredRuntimeDiagramPostCheckUsesTypedRelationScopeInTraceFamily(t *testing.T) {
	start, end := 1.0, 1.05
	bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("画出记录之间的交接"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace, PerfTrace: &types.PerfBundle{}, PredicateAxis: types.AxisFlow,
			RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeRelationAnalysis},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end},
		}, AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{Required: true, RequiredKind: types.DiagramFlow}}},
	}
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_transaction_handoffs/events.systrace")
	raw, _ := json.Marshal(map[string]any{"path": path, "view": "transaction_handoffs", "time_start": start, "time_end": end})
	r, err := (&tool.TraceQuery{}).Execute(bus, raw)
	if err != nil || !r.Success {
		t.Fatalf("native query: %v %+v", err, r)
	}
	bus.Mutable.AppendDispatchToolResult(r)
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	if view.Family != types.QFRootCauseTrace || !view.DiagramPlan.RequireStructuralEdge {
		t.Fatalf("actual compiler must retain family while selecting native relation structure: %+v", view)
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "diagram", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n Submitter\n Consumer"},
	}}}
	vs := validateDiagramEdgeSupportWithRuntimeContext(doc, view, bus)
	if len(vs) != 1 || vs[0].Kind != types.ViolRequiredDiagramEdgeAbsent || vs[0].RepairLocusOverride != types.LocusFinalizer {
		t.Fatalf("production post-check must reject the same native relation omission: %+v", vs)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	rows := tool.RuntimeDiagramRelations(ledger, &bus.AnalysisIR.RequestModel)
	if len(rows) != 3 {
		t.Fatalf("native relation pool changed: %+v", rows)
	}
	row := rows[0]
	doc.Blocks[0].Diagram.Body += fmt.Sprintf("\n %s[\"提交记录\"] --> %s[\"消费记录\"]", row.FromNode, row.ToNode)
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: row.FromNode, ToNode: row.ToNode, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity, RelationKind: row.Kind}}
	if vs := validateDiagramEdgeSupportWithRuntimeContext(doc, view, bus); len(vs) != 0 {
		t.Fatalf("native minimum must not demand synthetic causal or source relations: %+v", vs)
	}
}
