package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceIDRepairFixture() (*types.AnswerDocumentV2, []types.EvidenceItem, types.RequestModel) {
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "The CLI asks the API client to fetch the user."},
		{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n    participant CLI as run (CLI)\n    participant API as ApiClient.fetchUser\n",
		}},
	}}
	ev := diagramEvidenceTestCall("run", "ApiClient.fetchUser")
	ev.ID, ev.Source, ev.LineStart, ev.LineEnd = "ev-cli-fetch", "packages/cli/src/main.ts", 12, 12
	return doc, []types.EvidenceItem{ev}, types.RequestModel{Intent: types.IntentExplain, PredicateAxis: types.AxisCall}
}

func sourceIDRepairCandidates(doc *types.AnswerDocumentV2, evidence []types.EvidenceItem, rm types.RequestModel) []types.AnswerDiagramRelationRepairCandidate {
	return diagramRelationRepairAllowedAdditions(doc, rm, evidence, nil, []string{"diagram"}, []preEmitStandaloneRelationRepairCandidate{{
		relation: types.DiagramRelCall, from: "run", to: "ApiClient.fetchUser", evidenceID: "ev-cli-fetch",
		source: "packages/cli/src/main.ts:12", blockIDs: []string{"diagram"},
	}}, 8)
}

// r1030: the model's initial wrong metadata is its own error. The retry
// producer must not additionally turn exact CLI/API declarations into cli/api.
func TestB1593DiagramSourceID_ProducerLeaseSchemaAndExecution(t *testing.T) {
	doc, evidence, rm := sourceIDRepairFixture()
	before, _ := json.Marshal(doc)
	allowed := sourceIDRepairCandidates(doc, evidence, rm)
	if len(allowed) != 1 {
		t.Fatalf("wanted exact relation candidate, got %+v", allowed)
	}
	if !slices.Contains(allowed[0].FromNodeIDs, "CLI") || !slices.Contains(allowed[0].ToNodeIDs, "API") ||
		slices.Contains(allowed[0].FromNodeIDs, "cli") || slices.Contains(allowed[0].ToNodeIDs, "api") {
		t.Errorf("executable IDs must preserve source syntax: %+v", allowed[0])
	}
	mut := types.NewMutableState("source node IDs")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	mut.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(doc, nil, allowed))
	lease := mut.AnswerDiagramRelationRepairLease()
	if lease == nil || len(lease.AllowedAdditions) != 1 {
		t.Fatalf("candidate lost in lease: %+v", lease)
	}
	schema := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut})
	if !json.Valid(schema) || !strings.Contains(string(schema), "CLI") || !strings.Contains(string(schema), "API") {
		t.Errorf("repair schema lacks exact declared IDs: %s", schema)
	}
	// Copy the producer's own short declaration aliases, as the model did in
	// r1030. This exercises the live reference resolver and persistence path.
	from, to := "", ""
	for _, id := range lease.AllowedAdditions[0].FromNodeIDs {
		if strings.EqualFold(id, "CLI") {
			from = id
		}
	}
	for _, id := range lease.AllowedAdditions[0].ToNodeIDs {
		if strings.EqualFold(id, "API") {
			to = id
		}
	}
	params := fmt.Sprintf(`{"unchanged_block_ids":["summary"],"diagram_edge_edits":[{"action":"add","addition_ref":%q,"edge":{"from_node":%q,"to_node":%q,"visible_label":"fetch user"}}]}`, lease.AllowedAdditions[0].AdditionRef, from, to)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut, EvidenceItems: evidence}, json.RawMessage(params))
	if err != nil || !res.Success {
		t.Fatalf("published capability failed its own executor: err=%v result=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || !strings.Contains(got.Blocks[1].Diagram.Body, "CLI->>API: fetch user") || strings.Contains(got.Blocks[1].Diagram.Body, "cli->>api") {
		t.Errorf("copying producer IDs invented disconnected implicit nodes: %+v", got)
	}
	if mismatches := DiagramCallEdgeEvidenceMismatches(got, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(mismatches) != 0 {
		t.Errorf("ordinary typed relation validator rejected exact repair: %+v", mismatches)
	}
	after, _ := json.Marshal(doc)
	if string(before) != string(after) {
		t.Fatal("producer/executor changed the immutable source document")
	}
}

func TestB1593DiagramSourceID_WrongCaseIsNotExecutablePermission(t *testing.T) {
	doc, _, _ := sourceIDRepairFixture()
	candidate := &types.AnswerDiagramRelationRepairCandidate{BlockID: "diagram", RelationKind: types.DiagramRelCall,
		FromIdentity: "run", ToIdentity: "ApiClient.fetchUser", FromNodeIDs: []string{"CLI"}, ToNodeIDs: []string{"API"}, Source: "packages/cli/src/main.ts:12"}
	for _, pair := range [][2]string{{"CLI", "API"}, {"cli", "API"}, {"CLI", "api"}, {"CLI", "Api"}} {
		err := validateAtomicDiagramAdditionEndpointBindings(&doc.Blocks[1], &types.DiagramEdgeAnchor{
			FromNode: pair[0], ToNode: pair[1], FromIdentity: "run", ToIdentity: "ApiClient.fetchUser", RelationKind: types.DiagramRelCall,
		}, candidate, nil)
		if (err == nil) != (pair == [2]string{"CLI", "API"}) {
			t.Errorf("case-sensitive permission %v: err=%v", pair, err)
		}
		if pair == [2]string{"CLI", "API"} {
			continue
		}
		mut := types.NewMutableState("wrong-case repair")
		mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
		mut.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(doc, nil, []types.AnswerDiagramRelationRepairCandidate{*candidate}))
		lease := mut.AnswerDiagramRelationRepairLease()
		if lease == nil || len(lease.AllowedAdditions) != 1 {
			t.Fatalf("missing exact capability: %+v", lease)
		}
		before, _ := json.Marshal(mut.AnswerDocumentV2())
		params := fmt.Sprintf(`{"unchanged_block_ids":["summary"],"diagram_edge_edits":[{"action":"add","addition_ref":%q,"edge":{"from_node":%q,"to_node":%q,"visible_label":"fetch user"}}]}`, lease.AllowedAdditions[0].AdditionRef, pair[0], pair[1])
		res, execErr := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, json.RawMessage(params))
		if execErr != nil || res.Success {
			t.Errorf("actual wrong-case patch must fail before writing: %v err=%v res=%+v", pair, execErr, res)
		}
		after, _ := json.Marshal(mut.AnswerDocumentV2())
		if string(before) != string(after) {
			t.Errorf("rejected %v patch changed visible answer", pair)
		}
	}
}

func TestB1593DiagramSourceID_ActualFullEmitPublishesExactRepair(t *testing.T) {
	doc, evidence, rm := sourceIDRepairFixture()
	rm.Intent = types.IntentTrace
	doc.Blocks[1].Diagram.Body += "    CLI->>API: fetch user\n"
	doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "run", ToNode: "ApiClient.fetchUser", RelationKind: types.DiagramRelCall, VisibleLabel: "fetch user"}}
	bus := &types.BusContext{Mutable: types.NewMutableState("show source relation"), EvidenceItems: evidence, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	res, err := (&EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || res.Success {
		t.Fatalf("initial wrong metadata must be returned to model: err=%v res=%+v", err, res)
	}
	// The ordinary pre-emit result is the actual producer of the repair delta;
	// it must publish source IDs, not just pass a helper-only test.
	if res.Repair == nil {
		t.Fatal("full emission omitted typed repair")
	}
	var delta diagramRelationRepairDelta
	if err := json.Unmarshal([]byte(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatal(err)
	}
	if len(delta.Failures) != 2 || len(delta.AllowedAdditions) != 1 {
		t.Fatalf("actual full-emit delta lost executable relation: %+v", delta)
	}
	got := delta.AllowedAdditions[0]
	if got.FromIdentity != "run" || got.ToIdentity != "ApiClient.fetchUser" ||
		!slices.Contains(got.FromNodeIDs, "CLI") || !slices.Contains(got.ToNodeIDs, "API") ||
		slices.Contains(got.FromNodeIDs, "cli") || slices.Contains(got.ToNodeIDs, "api") {
		t.Fatalf("actual full emission published incorrect exact IDs: %+v", got)
	}
	t.Logf("actual full-emit typed delta: %+v", delta)
}

func TestB1593DiagramSourceID_ExactDeclarationsAndCaseCollision(t *testing.T) {
	for _, fixture := range []struct {
		name, body string
		kind       types.DiagramKind
	}{
		{"sequence", "sequenceDiagram\nparticipant API as ApiClient.fetchUser\nparticipant api as Other.fetchUser\n", types.DiagramSequence},
		{"flow", "flowchart LR\nAPI[ApiClient.fetchUser]\napi[Other.fetchUser]\n", types.DiagramFlow},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			doc, _, _ := sourceIDRepairFixture()
			doc.Blocks[1].Diagram.Body, doc.Blocks[1].Diagram.Kind = fixture.body, fixture.kind
			got := explicitDiagramEndpointDeclarations(doc, "diagram")
			want := []explicitDiagramEndpointDeclaration{{ID: "API", Label: "ApiClient.fetchUser"}, {ID: "api", Label: "Other.fetchUser"}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("case-distinct source declarations collapsed: got=%+v want=%+v", got, want)
			}
		})
	}
	doc, _, _ := sourceIDRepairFixture()
	lease := types.NewAnswerDiagramRelationRepairLease(doc, nil, []types.AnswerDiagramRelationRepairCandidate{{
		BlockID: "diagram", RelationKind: types.DiagramRelCall, FromIdentity: "run", ToIdentity: "ApiClient.fetchUser", Source: "a.ts:12",
		FromNodeIDs: []string{"CLI", "cli"}, ToNodeIDs: []string{"API", "api"},
	}})
	if lease == nil || !reflect.DeepEqual(lease.AllowedAdditions[0].FromNodeIDs, []string{"CLI", "cli"}) || !reflect.DeepEqual(lease.AllowedAdditions[0].ToNodeIDs, []string{"API", "api"}) {
		t.Errorf("lease collapsed independently selectable exact IDs: %+v", lease)
	}
	decls := []mermaidcompat.NodeDecl{{Ident: "API", Label: "ApiClient.fetchUser"}, {Ident: "api", Label: "ApiClient.fetchUser"}}
	if id, ok, ambiguous := atomicSequenceUniqueDeclaredTypedNode("ApiClient.fetchUser", "ApiClient.fetchUser", types.DiagramRelCall, decls, nil); ok || !ambiguous {
		t.Errorf("technical endpoint guessed between case-distinct source IDs: id=%q ok=%v ambiguous=%v", id, ok, ambiguous)
	}
}

func TestB1593DiagramSourceID_DeclarationScopeDoesNotInventChoices(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []string
	}{
		{"same exact repeated", "sequenceDiagram\nparticipant API as ApiClient.fetchUser\nparticipant API as ApiClient.fetchUser\n", []string{"API"}},
		{"conflicting exact declaration", "sequenceDiagram\nparticipant API as ApiClient.fetchUser\nparticipant API as Other.fetchUser\n", nil},
		{"two exact aliases same identity", "sequenceDiagram\nparticipant API as ApiClient.fetchUser\nparticipant api as ApiClient.fetchUser\n", nil},
		{"message payload is not a declaration", "sequenceDiagram\nCLI->>API: ApiClient.fetchUser(run)\n", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, evidence, _ := sourceIDRepairFixture()
			doc.Blocks[1].Diagram.Body = tc.body
			row := types.AnswerDiagramRelationRepairCandidate{BlockID: "diagram", FromIdentity: "run", ToIdentity: "ApiClient.fetchUser"}
			bindDiagramRelationRepairCandidateExistingTypedNodeIDs(&row, doc, evidence)
			if !reflect.DeepEqual(row.ToNodeIDs, tc.want) {
				t.Errorf("source alias qualification guessed: got=%v want=%v", row.ToNodeIDs, tc.want)
			}
			row.ToNodeIDs = nil
			bindDiagramRelationRepairCandidateExistingTypedNodeIDs(&row, doc, nil)
			if len(row.ToNodeIDs) != 0 {
				t.Fatal("source declaration minted relation evidence")
			}
		})
	}
	doc, _, rm := sourceIDRepairFixture()
	doc.Blocks[1].Diagram.Body = "sequenceDiagram\nparticipant API as ApiClient.fetchUser\nparticipant api as ApiClient.fetchUser\n"
	rm.DiagramHint = &types.DiagramHint{Participants: []types.DiagramParticipantHint{{Identity: "ApiClient.fetchUser", Role: types.DiagramParticipantIncidentRequired}}}
	if got := diagramParticipantExactVisibleEndpointIDs(doc, rm, "ApiClient.fetchUser", "diagram"); !reflect.DeepEqual(got, []string{"API", "api"}) {
		t.Fatalf("typed participant choices collapsed distinct source nodes: %v", got)
	}
}

func TestB1593DiagramSourceID_RuntimeTemporalKeepsIndependentAuthority(t *testing.T) {
	pctx := reportLocalTemporalTraceDiagramTestContext()
	pctx.ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "frame", Kind: types.BlockDiagram,
		Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\nparticipant UI\nparticipant RS\nUI->>RS: frame sequence\n"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "UI", ToNode: "RS", FromIdentity: "UI", ToIdentity: "RS", RelationKind: types.DiagramRelTemporal}},
	}}}
	before, _ := json.Marshal(doc)
	if hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, &types.AnswerSemanticView{Family: types.QFRootCauseTrace}, pctx); len(hints) != 0 {
		t.Fatalf("report-local runtime temporal fact entered source repair contract: %+v", hints)
	}
	after, _ := json.Marshal(doc)
	if string(before) != string(after) {
		t.Fatal("runtime diagram content changed")
	}
}
