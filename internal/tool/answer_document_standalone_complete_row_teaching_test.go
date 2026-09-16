package tool

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// An explicit citation range is intentionally broader than the call evidence:
// it does not select an exact evidence ID or an additions-only capability. The
// public emitter must therefore teach the ordinary complete-block repair.
func standaloneCompleteRowFixture(t *testing.T) (*types.BusContext, *types.AnswerDocumentV2, types.DiagramEdgeAnchor) {
	t.Helper()
	repo, err := filepath.Abs("../../eval/fixtures/java-layered-service")
	if err != nil {
		t.Fatal(err)
	}
	const source = "src/main/java/com/clinic/web/VisitController.java"
	evidence := []types.EvidenceItem{{
		ID: "complete-row-call", Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		Subject: "VisitController.create", Object: "VisitService.schedule", Predicate: "calls",
		Source: source, LineStart: 18, LineEnd: 18, AnchorKind: types.AnchorCall,
		AnchorSymbol: "schedule", OwnerSymbol: "VisitController.create",
		Snippet: "return service.schedule(petId, reason);", GroundingStatus: types.GroundingGrounded,
		Origin: types.ClaimOriginCurrentRepo,
	}}
	mut := types.NewMutableState("Explain the verified request delegation")
	mut.SetRepoRoot(repo)
	mut.AppendEvidence(evidence)
	bus := &types.BusContext{RepoRoot: repo, WorkDir: t.TempDir(), Mutable: mut, EvidenceItems: evidence,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2",
		Citations: []types.Citation{{File: source, Line: 14, LineEnd: 19, Scope: types.ScopeLineRange}},
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "The HTTP entry delegates appointment scheduling."},
			{ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
				Title: "Request delegation", Text: "Keep this model-authored explanation unchanged.",
				FacetIDs:  []string{string(types.FacetPrincipalPathEdge), string(types.FacetCurrentCodePath)},
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, FacetID: string(types.FacetPrincipalPathEdge)}},
				Items: []types.AnswerBlockItem{{ID: "step", Label: "HTTP entry",
					Text: "Delegates appointment scheduling to the service.", CitationRef: 0}},
			},
		}}
	anchor := types.DiagramEdgeAnchor{FromNode: "HTTP entry", ToNode: "Appointment service",
		FromIdentity: "VisitController.create", ToIdentity: "VisitService.schedule",
		RelationKind: types.DiagramRelCall, VisibleLabel: "delegates appointment scheduling"}
	return bus, doc, anchor
}

func standaloneCompleteRowExecute(t *testing.T, bus *types.BusContext, payload any, patch bool) types.ToolResult {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	before := string(raw)
	var result types.ToolResult
	if patch {
		result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
	} else {
		result, err = (&EmitAnswerDocument{}).Execute(bus, raw)
	}
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(payload)
	if err != nil || string(after) != before {
		t.Fatalf("public execution changed model input: err=%v before=%s after=%s", err, before, after)
	}
	return result
}

func TestB1707StandaloneCompleteRowTeachingPublicEmitPatch(t *testing.T) {
	bus, doc, anchor := standaloneCompleteRowFixture(t)
	result := standaloneCompleteRowExecute(t, bus, doc, false)
	if result.Success || result.Repair == nil ||
		!strings.Contains(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationFailureIssues], diagramStandaloneRelationClaimHasNoAnchor) {
		t.Fatalf("empty principal relation must request its missing complete row: %+v", result)
	}
	var action string
	for _, line := range strings.Split(result.Summary, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Action: ") && strings.Contains(line, "edge_anchors is empty") {
			action = line
			break
		}
	}
	if !strings.Contains(action, "complete same-direction typed recipe") || strings.Contains(action, "when it publishes addition_ref") {
		t.Fatalf("fixture did not reach the public ordinary full-row teaching branch: %+v", result)
	}
	const completeFields = "from_node, to_node, relation_kind, from_identity, to_identity, and visible_label"
	if !strings.Contains(action, completeFields) {
		t.Errorf("public complete-row repair teaching must list all six required fields together: %s", action)
	}
	if !strings.Contains(action, "visible_label must be a concise model-authored business phrase") {
		t.Errorf("public complete-row teaching must connect the visible label to a model-authored business phrase: %s", action)
	}
	if bus.Mutable.AnswerDocumentV2() != nil {
		t.Fatal("rejected empty relation was published")
	}

	// Follow the old five-field 'complete' recipe verbatim. Identity proof is
	// now adequate, but its separately rendered relation label is still owed.
	withoutLabel := anchor
	withoutLabel.VisibleLabel = ""
	doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{withoutLabel}
	patch := types.AnswerDocumentV2Patch{ReplaceBlocks: []types.AnswerBlock{doc.Blocks[1]}, UnchangedBlockIDs: []string{"summary"}}
	result = standaloneCompleteRowExecute(t, bus, patch, true)
	if result.Success || result.Repair == nil ||
		!strings.Contains(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationFailureIssues], diagramStandaloneRelationMissingVisibleLabel) {
		t.Fatalf("old five-field recipe must still expose its independent visible-label debt: %+v", result)
	}
	if strings.Contains(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationFailureIssues], diagramStandaloneRelationIdentityMissing) {
		t.Fatalf("complete identities must not be lost while teaching the visible label: %+v", result)
	}

	doc.Blocks[1].EdgeAnchors[0] = anchor
	patch.ReplaceBlocks = []types.AnswerBlock{doc.Blocks[1]}
	result = standaloneCompleteRowExecute(t, bus, patch, true)
	if !result.Success {
		t.Fatalf("six-field model-authored row must close the public repair: %+v", result)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 || !reflect.DeepEqual(got.Blocks[1].EdgeAnchors, doc.Blocks[1].EdgeAnchors) ||
		got.Blocks[1].Text != doc.Blocks[1].Text || got.Blocks[1].Title != doc.Blocks[1].Title ||
		got.Blocks[1].Items[0].Text != doc.Blocks[1].Items[0].Text || got.Blocks[1].Items[0].Label != doc.Blocks[1].Items[0].Label ||
		got.Blocks[0].Text != doc.Blocks[0].Text {
		t.Fatalf("repair changed model-authored content or exact typed relation: %+v", got)
	}
}

func TestB1707StandaloneCompleteRowTeachingDoesNotGrantEvidence(t *testing.T) {
	for _, mode := range []string{"unsupported_endpoint", "ungrounded_evidence", "unsupported_diagram"} {
		t.Run(mode, func(t *testing.T) {
			bus, doc, anchor := standaloneCompleteRowFixture(t)
			doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{anchor}
			switch mode {
			case "unsupported_endpoint":
				doc.Blocks[1].EdgeAnchors[0].ToIdentity = "UnrelatedService.run"
			case "ungrounded_evidence":
				bus.EvidenceItems[0].GroundingStatus = types.GroundingUngrounded
				bus.Mutable = types.NewMutableState("same model claim without citable proof")
				bus.Mutable.SetRepoRoot(bus.RepoRoot)
				bus.Mutable.AppendEvidence(bus.EvidenceItems)
			case "unsupported_diagram":
				doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "diagram", Kind: types.BlockDiagram,
					Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid",
						Body: "sequenceDiagram\n  participant A as VisitController.create\n  participant B as UnrelatedService.run\n  A->>B: delegates\n"},
					EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall,
						FromIdentity: "VisitController.create", ToIdentity: "UnrelatedService.run", VisibleLabel: "delegates"}},
				})
			}
			result := standaloneCompleteRowExecute(t, bus, doc, false)
			if result.Success || result.Repair == nil ||
				!strings.Contains(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationFailureIssues], diagramCallEdgeIssueNoEvidence) {
				t.Fatalf("complete visible fields must not grant unsupported %s authority: %+v", mode, result)
			}
			if bus.Mutable.AnswerDocumentV2() != nil {
				t.Fatal("unsupported relation reached accepted answer")
			}
		})
	}
}

func TestB1707StandaloneCompleteRowTeachingLeavesDiagramLabelOptional(t *testing.T) {
	for _, mode := range []string{"diagram_anchor", "standalone_sibling_edge"} {
		t.Run(mode, func(t *testing.T) {
			bus, doc, anchor := standaloneCompleteRowFixture(t)
			anchor.ClaimForm = types.ClaimCallEdge
			if mode == "standalone_sibling_edge" {
				anchor.FromNode, anchor.ToNode, anchor.VisibleLabel = "A", "B", ""
			}
			doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{anchor}
			diagramAnchor := anchor
			diagramAnchor.FromNode, diagramAnchor.ToNode, diagramAnchor.VisibleLabel = "A", "B", ""
			doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "diagram", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid",
					Body: "sequenceDiagram\n  participant A as \"VisitController.create\"\n  participant B as \"VisitService.schedule\"\n  A->>B: delegates appointment scheduling"},
				EdgeAnchors: []types.DiagramEdgeAnchor{diagramAnchor},
			})
			result := standaloneCompleteRowExecute(t, bus, doc, false)
			if !result.Success {
				t.Fatalf("standalone teaching must not require an optional diagram/sibling anchor label: %+v", result)
			}
			got := bus.Mutable.AnswerDocumentV2()
			if got == nil || len(got.Blocks) != 3 {
				t.Fatalf("accepted diagram missing: %+v", got)
			}
			for _, index := range []int{1, 2} {
				if !reflect.DeepEqual(got.Blocks[index].EdgeAnchors, doc.Blocks[index].EdgeAnchors) {
					t.Errorf("block %s anchors were rewritten: got=%+v want=%+v", doc.Blocks[index].ID,
						got.Blocks[index].EdgeAnchors, doc.Blocks[index].EdgeAnchors)
				}
			}
			if got.Blocks[2].Diagram.Body != doc.Blocks[2].Diagram.Body {
				t.Errorf("diagram body was rewritten: got=%q want=%q", got.Blocks[2].Diagram.Body, doc.Blocks[2].Diagram.Body)
			}
		})
	}
}
