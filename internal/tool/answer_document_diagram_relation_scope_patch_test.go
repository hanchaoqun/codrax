package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func relationScopePatchTestState() (*types.BusContext, *types.AgentContext, *types.AnswerDocumentV2) {
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "Analyzer"},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "Explorer"},
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{
			Kind: types.DiagramArchitecture, Required: true, Participants: participants,
		},
	}
	ir := &types.AnalysisIR{
		RequestModel: rm,
		AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
			Required:     true,
			RequiredKind: types.DiagramArchitecture,
		}},
	}
	mut := types.NewMutableState("scope patch")
	evidence := []types.EvidenceItem{diagramEvidenceTestCall("Analyzer", "Explorer")}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramArchitecture, Language: "mermaid",
			Body: "flowchart LR\n Analyzer[\"Analyzer\"]\n Explorer[\"Explorer\"]",
		},
		RequestedRelationScope: types.DiagramRelationScopePartialUnproven,
	}}}
	bus := &types.BusContext{Mode: types.ModeRead, AnalysisIR: ir, Mutable: mut, EvidenceItems: evidence}
	agent := &types.AgentContext{Mode: types.ModeRead, AnalysisIR: ir, Mutable: mut, EvidenceItems: evidence}
	return bus, agent, doc
}

func TestPatchRelationScopeSchemaPublishesOnlyLiveTypedStaleRemoval(t *testing.T) {
	_, agent, doc := relationScopePatchTestState()
	agent.Mutable.SetLastRejectedAnswerDocumentV2(doc)
	raw := (&EmitAnswerDocumentPatch{}).ParametersFor(agent)
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	properties, _ := root["properties"].(map[string]any)
	field, _ := properties["diagram_relation_scope_edits"].(map[string]any)
	if field == nil {
		t.Fatalf("typed stale scope mismatch did not publish its local edit: %s", raw)
	}
	encoded, _ := json.Marshal(field)
	for _, want := range []string{`"block_id"`, `"flow"`, `"action"`, `"remove_scope"`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("scope edit schema missing %s: %s", want, encoded)
		}
	}
	if strings.Contains(string(encoded), answerDiagramRelationScopeActionSet) {
		t.Fatalf("stale declaration must not advertise the opposite set action: %s", encoded)
	}
}

func TestPatchRelationScopeSchemaHidesLocalEditWithoutLiveTypedMismatch(t *testing.T) {
	_, agent, doc := relationScopePatchTestState()
	doc.Blocks[0].RequestedRelationScope = types.DiagramRelationScopeUnknown
	agent.Mutable.SetLastRejectedAnswerDocumentV2(doc)
	raw := (&EmitAnswerDocumentPatch{}).ParametersFor(agent)
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	properties, _ := root["properties"].(map[string]any)
	if _, exposed := properties["diagram_relation_scope_edits"]; exposed {
		t.Fatalf("no typed scope mismatch must not increase the model's patch surface: %s", raw)
	}
}

func TestDiagramRelationScopeEditAlternativesAllowOneSetButAllRemovals(t *testing.T) {
	if got := answerDiagramRelationScopeEditMaxItems([]answerDiagramRelationScopeEditCapability{
		{BlockID: "flow-a", Action: answerDiagramRelationScopeActionSet},
		{BlockID: "flow-b", Action: answerDiagramRelationScopeActionSet},
	}); got != 1 {
		t.Fatalf("alternative missing-scope carriers must permit exactly one selection, got %d", got)
	}
	if got := answerDiagramRelationScopeEditMaxItems([]answerDiagramRelationScopeEditCapability{
		{BlockID: "flow-a", Action: answerDiagramRelationScopeActionRemove},
		{BlockID: "flow-b", Action: answerDiagramRelationScopeActionRemove},
	}); got != 2 {
		t.Fatalf("independent stale/duplicate declarations should be removable together, got %d", got)
	}
}

func TestPatchRelationScopeSchemaSurvivesLocalLeaseWithoutWholeTargetReplacement(t *testing.T) {
	_, agent, doc := relationScopePatchTestState()
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "Analyzer", ToNode: "Explorer",
		FromIdentity: "Analyzer", ToIdentity: "Explorer",
		RelationKind: types.DiagramRelCall,
	}}
	agent.Mutable.SetLastRejectedAnswerDocumentV2(doc)
	lease := types.NewAnswerDiagramRelationRepairLease(doc,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "flow", Issue: diagramCallEdgeIssueAnchorWithoutBodyEdge,
			FromNode: "Analyzer", ToNode: "Explorer",
			FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelCall,
		}}, nil)
	if lease == nil || !types.AnswerDiagramRelationRepairLeaseIsLocallyExecutable(lease) {
		t.Fatalf("test setup did not produce an executable local lease: %+v", lease)
	}
	agent.Mutable.SetAnswerDiagramRelationRepairLease(lease)

	raw := (&EmitAnswerDocumentPatch{}).ParametersFor(agent)
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	properties, _ := root["properties"].(map[string]any)
	if _, exposed := properties["diagram_relation_scope_edits"]; !exposed {
		t.Fatalf("typed local scope edit disappeared behind the relation lease: %s", raw)
	}
	if _, exposed := properties["replace_blocks"]; exposed {
		t.Fatalf("sole lease-target diagram must remain unavailable to whole replacement: %s", raw)
	}
}

func TestApplyModelAuthoredDiagramRelationScopeEditPreservesGraph(t *testing.T) {
	bus, _, doc := relationScopePatchTestState()
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"flow"}}
	beforeBody := doc.Blocks[0].Diagram.Body
	if err := applyModelAuthoredDiagramRelationScopeEdits(
		doc,
		patch,
		[]emitAnswerDiagramRelationScopeEdit{{BlockID: "flow", Action: answerDiagramRelationScopeActionRemove}},
		bus,
	); err != nil {
		t.Fatalf("typed stale scope removal failed: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 || patch.ReplaceBlocks[0].RequestedRelationScope != types.DiagramRelationScopeUnknown {
		t.Fatalf("scope edit did not compile one exact field replacement: %+v", patch.ReplaceBlocks)
	}
	if patch.ReplaceBlocks[0].Diagram == nil || patch.ReplaceBlocks[0].Diagram.Body != beforeBody {
		t.Fatalf("scope edit changed model-authored Mermaid content: %+v", patch.ReplaceBlocks[0].Diagram)
	}
	if len(patch.UnchangedBlockIDs) != 0 {
		t.Fatalf("scope-edited block should absorb redundant unchanged id: %v", patch.UnchangedBlockIDs)
	}
	if err := applyModelAuthoredDiagramRelationScopeEdits(
		doc,
		&types.AnswerDocumentV2Patch{},
		[]emitAnswerDiagramRelationScopeEdit{{BlockID: "flow", Action: answerDiagramRelationScopeActionSet}},
		bus,
	); err == nil {
		t.Fatal("opposite or stale scope action must fail closed")
	}
}
