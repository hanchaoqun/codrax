package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSequencePlacementPublic_NewMessageDoesNotRetargetOldOccurrence(t *testing.T) {
	for _, action := range []string{"remove", "replace", "relabel"} {
		for _, newFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s_new_first_%v", action, newFirst), func(t *testing.T) {
				body := "sequenceDiagram\n    participant A\n    participant B\n    A->>B: old\n"
				bus, prev := sequencePlacementPublicFixture(t, body)
				issue := "missing_call_anchor"
				if action == "relabel" {
					issue = diagramTypedRecipeMissingVisibleLabel
					prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelCall, VisibleLabel: "old"}}
					bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
				}
				lease := types.NewAnswerDiagramRelationRepairLease(prev,
					[]types.AnswerDiagramRelationRepairFailure{{BlockID: "diag", Issue: issue, FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelCall, BodyOccurrence: 1}},
					[]types.AnswerDiagramRelationRepairCandidate{{BlockID: "diag", RelationKind: types.DiagramRelPrecedence, FromIdentity: "Analyzer", ToIdentity: "Explorer", FromNodeIDs: []string{"A"}, ToNodeIDs: []string{"B"}, Source: "typed-stage-proof"}})
				bus.Mutable.SetAnswerDiagramRelationRepairLease(lease)
				ref := sequencePlacementPublicRef(t, bus, "A->>B: old")
				addition := fmt.Sprintf(`{"addition_ref":%q,"action":"add","placement_ref":%q,"edge":{"from_node":"A","to_node":"B","visible_label":"new"}}`, lease.AllowedAdditions[0].AdditionRef, ref)
				existing := fmt.Sprintf(`{"failure_ref":%q,"action":%q}`, lease.Failures[0].FailureRef, action)
				if action == "replace" {
					existing = fmt.Sprintf(`{"failure_ref":%q,"action":"replace","edge":{"from_node":"A","to_node":"B","visible_label":"replaced"}}`, lease.Failures[0].FailureRef)
				} else if action == "relabel" {
					existing = fmt.Sprintf(`{"failure_ref":%q,"action":"relabel","visible_label":"replaced"}`, lease.Failures[0].FailureRef)
				}
				edits := existing + "," + addition
				if newFirst {
					edits = addition + "," + existing
				}
				raw := json.RawMessage(`{"diagram_edge_edits":[` + edits + `]}`)
				result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
				var got *types.AnswerDocumentV2
				if action != "remove" {
					// Existing local-scope validation refuses expanding a failed
					// visible pair into two relations. Do not relax that gate for
					// placement; verify public rollback, then the narrower editor.
					if err != nil || result.Success || !strings.Contains(result.Summary, "failed_relation_expanded") || !reflect.DeepEqual(bus.Mutable.AnswerDocumentV2(), prev) {
						t.Fatalf("pair-expansion guard or atomic rollback changed: %v / %+v", err, result)
					}
					var request struct {
						Edits []emitAnswerDiagramEdgeEdit `json:"diagram_edge_edits"`
					}
					if err := json.Unmarshal(raw, &request); err != nil {
						t.Fatal(err)
					}
					patch := &types.AnswerDocumentV2Patch{}
					if err := applyModelAuthoredDiagramAtomicEdits(prev, patch, request.Edits, nil, lease); err != nil {
						t.Fatal(err)
					}
					got, err = types.ApplyAnswerDocumentV2Patch(prev, patch)
					if err != nil {
						t.Fatal(err)
					}
				} else {
					if err != nil || !result.Success {
						t.Fatalf("same-pair atomic patch failed: %v / %+v", err, result)
					}
					got = bus.Mutable.AnswerDocumentV2()
				}
				want := strings.Replace(body, "    A->>B: old\n", "    A->>B: new\n", 1)
				if action != "remove" {
					want += "    A->>B: replaced\n"
				}
				if body := got.Blocks[1].Diagram.Body; body != want {
					t.Fatalf("new insertion changed original occurrence target:\ngot=%q\nwant=%q", body, want)
				}
			})
		}
	}
}

func TestSequencePlacementPublic_LegacyAnchorOccurrenceUsesBaseMapping(t *testing.T) {
	for _, newFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("new_first_%v", newFirst), func(t *testing.T) {
			body := "sequenceDiagram\n    participant A\n    participant B\n    A->>B: first\n    A->>B: second\n"
			bus, prev := sequencePlacementPublicFixture(t, body)
			first := types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelCall, VisibleLabel: "first"}
			second := first
			second.VisibleLabel = "second"
			prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{first, second}
			bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
			ref := sequencePlacementPublicRef(t, bus, "A->>B: first")
			newAnchor := first
			newAnchor.RelationKind, newAnchor.VisibleLabel = types.DiagramRelPrecedence, "new"
			addition := emitAnswerDiagramEdgeEdit{BlockID: "diag", Action: "add", PlacementRef: ref, Edge: &newAnchor}
			removal := emitAnswerDiagramEdgeEdit{BlockID: "diag", Action: "remove", Occurrence: 2, Match: &second}
			edits := []emitAnswerDiagramEdgeEdit{removal, addition}
			if newFirst {
				edits = []emitAnswerDiagramEdgeEdit{addition, removal}
			}
			raw, err := json.Marshal(map[string]any{"diagram_edge_edits": edits})
			if err != nil {
				t.Fatal(err)
			}
			result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
			if err != nil || !result.Success {
				t.Fatalf("legacy source mapping changed: %v / %+v", err, result)
			}
			want := strings.Replace(body, "    A->>B: second\n", "", 1)
			want = strings.Replace(want, "    A->>B: first", "    A->>B: new\n    A->>B: first", 1)
			if got := bus.Mutable.AnswerDocumentV2().Blocks[1].Diagram.Body; got != want {
				t.Fatalf("legacy original occurrence changed:\ngot=%q\nwant=%q", got, want)
			}
		})
	}
}

func TestSequencePlacementPublic_ParticipantBoxesAreDeclarationOnly(t *testing.T) {
	body := "sequenceDiagram\n    box rgb(240,240,240) Workers\n    participant A\n    participant B\n    end\n    A->>B: existing\n"
	bus, prev := sequencePlacementPublicFixture(t, body)
	for _, position := range sequenceInsertionPositions(prev.Blocks[1]) {
		if position.Before == "end" || strings.HasPrefix(position.Before, "participant ") {
			t.Fatalf("published an illegal message gap inside participant box: %+v", position)
		}
	}
	ref := sequencePlacementPublicRef(t, bus, "A->>B: existing")
	addition := bus.Mutable.AnswerDiagramRelationRepairLease().AllowedAdditions[0].AdditionRef
	params := fmt.Sprintf(`{"diagram_edge_edits":[{"addition_ref":%q,"action":"add","placement_ref":%q,"edge":{"from_node":"A","to_node":"B","visible_label":"inserted"}}]}`, addition, ref)
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(params))
	if err != nil || !result.Success {
		t.Fatalf("box-preserving public insertion failed: %v / %+v", err, result)
	}
	want := strings.Replace(body, "    A->>B: existing", "    A->>B: inserted\n    A->>B: existing", 1)
	if got := bus.Mutable.AnswerDocumentV2().Blocks[1].Diagram.Body; got != want {
		t.Fatalf("participant box changed:\ngot=%q\nwant=%q", got, want)
	}
}
