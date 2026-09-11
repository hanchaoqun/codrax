package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The source and public protocol match b1657MetadataDependency, but the model
// owns a flowchart. This does not treat a flow arrow as a new relation kind: the
// retained A -> B call has the actual ReadFile -> EmitEvidence receipt, while
// the unproved X -> Y call is explicitly selected for removal. Z starts alone.
func b1657FlowMetadataDependency(t *testing.T) (*types.BusContext, *types.AnswerDocumentV2) {
	t.Helper()
	bus, _ := b1649ActualCall(t)
	doc := b1649Diagram("p.Caller", "Callee")
	doc.Citations = []types.Citation{{File: "calls.go", Line: 4, LineEnd: 4, Scope: types.ScopeLine, Quote: "Callee()"}}
	doc.Blocks[1].Diagram = &types.AnswerDiagramBlock{
		Kind: types.DiagramFlow, Language: "mermaid",
		Body: "flowchart LR\n  A[p.Caller]\n  B[Callee]\n  X[Alpha]\n  Y[Beta]\n  Z[Unrelated]\n  A -->|invoke callee| B\n  X -->|model-selected message| Y\n",
	}
	doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "A", ToNode: "B", FromIdentity: "p.Caller", ToIdentity: "Callee",
		RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge, VisibleLabel: "invoke callee",
	}}
	for i := 0; i < 3; i++ {
		doc.Blocks[1].EdgeAnchors = append(doc.Blocks[1].EdgeAnchors, types.DiagramEdgeAnchor{
			FromNode: "X", ToNode: "Y", FromIdentity: "Alpha", ToIdentity: "Beta",
			RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
			VisibleLabel: fmt.Sprintf("model metadata %d", i),
		})
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := (&EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || initial.Success || initial.Repair == nil {
		t.Fatalf("flow fixture must reach actual relation rejection: err=%v result=%+v", err, initial)
	}
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(initial.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatalf("public initial flow delta: %v", err)
	}
	lease := types.NewAnswerDiagramRelationRepairLease(doc, delta.Failures, delta.AllowedAdditions)
	if lease == nil {
		t.Fatalf("public flow rejection cannot supply a lease: %+v", delta)
	}
	var selected types.AnswerDiagramRelationRepairFailure
	for _, failure := range lease.Failures {
		if failure.CanRemoveVisibleBodyOccurrence("diagram", "X", "Y", 1, 1) {
			selected = failure
			break
		}
	}
	if selected.FailureRef == "" {
		t.Fatalf("no exact flow body removal in public delta: %+v", lease)
	}
	// Package tool cannot import the agent-only candidate producer. As in the
	// sequence regression, install its existing shape only after verifying both
	// candidates' original incident edge is the public remove-capable failure.
	for _, id := range []string{"X", "Y"} {
		incident, removable := atomicDiagramBaseIncidentEdgesAreRemoveCapableFailures(doc.Blocks[1], id, lease)
		if incident != 1 || !removable {
			t.Fatalf("flow candidate %s has no exact removal source: %d %v", id, incident, removable)
		}
		lease.OptionalOrphanCleanups = append(lease.OptionalOrphanCleanups, testDiagramOrphanCandidates("diagram", id)...)
	}
	if incident, _ := atomicDiagramBaseIncidentEdgesAreRemoveCapableFailures(doc.Blocks[1], "Z", lease); incident != 0 {
		t.Fatal("Z must be pre-existing isolated context")
	}
	accepted := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "accepted", Kind: types.BlockSummary, Text: "Previously accepted flow answer."}}}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, accepted)
	bus.Mutable.SetPendingAnswerDocumentPatchBase(doc)
	bus.Mutable.SetAnswerDiagramRelationRepairLease(lease)
	params, _ := json.Marshal(map[string]any{
		"unchanged_block_ids": []string{"summary"},
		"diagram_edge_edits":  []map[string]string{{"failure_ref": selected.FailureRef, "action": "remove"}},
	})
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil || result.Success || result.Repair == nil || result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeStagedForRetry {
		t.Fatalf("flow body removal must stage its remaining metadata: err=%v result=%+v", err, result)
	}
	staged := bus.Mutable.PendingAnswerDocumentPatchBase()
	wantBody := strings.Replace(doc.Blocks[1].Diagram.Body, "  X -->|model-selected message| Y\n", "", 1)
	if staged == nil || staged.Blocks[1].Diagram.Kind != types.DiagramFlow || staged.Blocks[1].Diagram.Body != wantBody ||
		len(staged.Blocks[1].EdgeAnchors) != 3 || !reflect.DeepEqual(staged.Blocks[1].EdgeAnchors[0], doc.Blocks[1].EdgeAnchors[0]) ||
		!reflect.DeepEqual(staged.Blocks[0], doc.Blocks[0]) || !reflect.DeepEqual(bus.Mutable.AnswerDocumentV2(), accepted) {
		t.Fatalf("flow stage changed unselected model content or lost its two metadata dependencies: %+v", staged)
	}
	if err := json.Unmarshal([]byte(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatalf("actual staged flow delta: %v", err)
	}
	current := bus.Mutable.AnswerDiagramRelationRepairLease()
	if current == nil || current.OrphanDispositionOnly || len(current.Failures) != 2 || len(current.OptionalOrphanCleanups) != 2 {
		t.Fatalf("flow metadata dependencies lost their optional source: delta=%+v lease=%+v", delta, current)
	}
	for _, failure := range current.Failures {
		if failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierStaleAnchor || failure.FromNode != "X" || failure.ToNode != "Y" {
			t.Fatalf("flow dependency was widened to another relation: %+v", failure)
		}
	}
	for _, candidate := range current.OptionalOrphanCleanups {
		if (candidate.ParticipantID != "X" && candidate.ParticipantID != "Y") || !types.AnswerDiagramOrphanMetadataDependencyMatchesBase(staged, candidate, current) {
			t.Fatalf("flow optional choice lost exact current-base provenance: %+v", candidate)
		}
	}
	if strings.Count(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON], `"decision_optional":true`) != 2 {
		t.Fatalf("flow public delta must disclose exactly two optional decisions: %+v", delta)
	}
	t.Logf("flow public remove staged two stale anchors and two optional source-bound declarations")
	return bus, staged
}

func TestB1657FlowMetadataDependencyPublicChoices(t *testing.T) {
	for _, action := range []string{"omit", "remove_if_isolated", "retain_as_context"} {
		t.Run(action, func(t *testing.T) {
			bus, staged := b1657FlowMetadataDependency(t)
			lease := bus.Mutable.AnswerDiagramRelationRepairLease()
			var edits []map[string]string
			for _, failure := range lease.Failures {
				edits = append(edits, map[string]string{"failure_ref": failure.FailureRef, "action": "remove"})
			}
			// The schema must publish the optional branch before the model can
			// choose it. Inspect parsed branch identities, not prose mentions.
			var schema struct {
				Properties map[string]struct {
					Items struct {
						OneOf []struct {
							Properties map[string]struct {
								Enum []string `json:"enum"`
							} `json:"properties"`
						} `json:"oneOf"`
					} `json:"items"`
				} `json:"properties"`
			}
			raw := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: bus.Mutable})
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			choices := make(map[string]bool)
			for _, branch := range schema.Properties["diagram_participant_edits"].Items.OneOf {
				ids, actions := branch.Properties["participant_id"].Enum, branch.Properties["action"].Enum
				if len(ids) != 1 || len(actions) != 1 || (ids[0] != "X" && ids[0] != "Y") {
					t.Fatalf("schema broadened flow cleanup beyond exact source nodes: %+v", branch)
				}
				choices[ids[0]+"/"+actions[0]] = true
			}
			for _, id := range []string{"X", "Y"} {
				for _, choice := range []string{"remove_if_isolated", "retain_as_context"} {
					if !choices[id+"/"+choice] {
						t.Errorf("actual schema omitted executable optional flow choice %s/%s", id, choice)
					}
				}
			}
			if action == "omit" {
				acceptedBefore, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
				pendingBefore, _ := json.Marshal(staged)
				bad, _ := json.Marshal(map[string]any{"diagram_edge_edits": edits, "diagram_participant_edits": []map[string]string{{"block_id": "diagram", "participant_id": "Z", "action": "remove_if_isolated"}}})
				denied, err := (&EmitAnswerDocumentPatch{}).Execute(bus, bad)
				if err != nil || denied.Success || denied.Repair == nil || denied.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeNotStaged {
					t.Fatalf("old isolated Z gained removal authority: err=%v result=%+v", err, denied)
				}
				acceptedAfter, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
				pendingAfter, _ := json.Marshal(bus.Mutable.PendingAnswerDocumentPatchBase())
				if string(acceptedBefore) != string(acceptedAfter) || string(pendingBefore) != string(pendingAfter) {
					t.Fatal("rejected Z edit partly applied sibling metadata removals")
				}
			}
			params := map[string]any{"diagram_edge_edits": edits}
			wantBody := staged.Blocks[1].Diagram.Body
			if action != "omit" {
				choice := map[string]string{"block_id": "diagram", "participant_id": "X", "action": action}
				wantBody = strings.Replace(wantBody, "  X[Alpha]\n", "", 1)
				if action == "retain_as_context" {
					choice["visible_label"] = "Model-selected context"
					wantBody = strings.Replace(staged.Blocks[1].Diagram.Body, "  X[Alpha]", `  X["Model-selected context"]`, 1)
				}
				params["diagram_participant_edits"] = []map[string]string{choice}
			}
			raw, _ = json.Marshal(params)
			result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
			if err != nil || !result.Success {
				t.Fatalf("flow metadata cleanup with choice %s did not publish: err=%v result=%+v", action, err, result)
			}
			got := bus.Mutable.AnswerDocumentV2()
			if got == nil || got.Blocks[1].Diagram.Kind != types.DiagramFlow || got.Blocks[1].Diagram.Body != wantBody ||
				len(got.Blocks[1].EdgeAnchors) != 1 || !reflect.DeepEqual(got.Blocks[1].EdgeAnchors[0], staged.Blocks[1].EdgeAnchors[0]) ||
				!reflect.DeepEqual(got.Blocks[0], staged.Blocks[0]) || !reflect.DeepEqual(got.Citations, staged.Citations) {
				t.Fatalf("flow choice changed another declaration, native relation, model text, or citation: %+v", got)
			}
			if bus.Mutable.PendingAnswerDocumentPatchBase() != nil {
				t.Fatal("completed metadata cleanup imposed a cosmetic retry")
			}
		})
	}
}
