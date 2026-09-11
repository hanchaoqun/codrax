package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This is a protocol regression, not a model replay. The retained A -> B edge
// has an actual ReadFile -> EmitEvidence receipt. X/Y are opaque model-owned
// declarations whose only body edge is explicitly selected for removal; Z was
// already disconnected and must never acquire cleanup authority from proximity.
func b1657MetadataDependency(t *testing.T, withoutSource ...bool) (*types.BusContext, *types.AnswerDocumentV2, *types.AnswerDocumentV2, types.ToolResult, *types.AnswerDiagramRelationRepairLease) {
	t.Helper()
	bus, _ := b1649ActualCall(t)
	doc := b1649Diagram("p.Caller", "Callee")
	doc.Blocks[1].Diagram.Body += "    participant X as Alpha\n    participant Y as Beta\n    participant Z as Unrelated\n    X->>Y: model-selected message\n"
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
		t.Fatalf("initial public relation rejection missing: err=%v result=%+v", err, initial)
	}
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(initial.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatalf("initial public delta: %v result=%+v", err, initial)
	}
	lease := types.NewAnswerDiagramRelationRepairLease(doc, delta.Failures, delta.AllowedAdditions)
	if lease == nil {
		t.Fatalf("initial public failures cannot build lease: %+v", delta)
	}
	var selected types.AnswerDiagramRelationRepairFailure
	for _, failure := range lease.Failures {
		if failure.CanRemoveVisibleBodyOccurrence("diagram", "X", "Y", 1, 1) {
			selected = failure
			break
		}
	}
	if selected.FailureRef == "" {
		t.Fatalf("fixture needs the public remove-capable visible occurrence: %+v", lease.Failures)
	}
	// Install the existing dispatcher-owned candidate shape, not a fabricated
	// orphan authorization: independently check its current incident/ref premise.
	// The agent-only producer is not importable from package tool.
	for _, id := range []string{"X", "Y"} {
		incident, removable := atomicDiagramBaseIncidentEdgesAreRemoveCapableFailures(doc.Blocks[1], id, lease)
		if incident != 1 || !removable {
			t.Fatalf("candidate %s lacks its exact original removal source: %d %v", id, incident, removable)
		}
		lease.OptionalOrphanCleanups = append(lease.OptionalOrphanCleanups, testDiagramOrphanCandidates("diagram", id)...)
	}
	if incident, _ := atomicDiagramBaseIncidentEdgesAreRemoveCapableFailures(doc.Blocks[1], "Z", lease); incident != 0 {
		t.Fatal("unrelated declaration must start disconnected")
	}
	if len(withoutSource) > 0 && withoutSource[0] {
		lease.OptionalOrphanCleanups = nil
	}
	accepted := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "accepted", Kind: types.BlockSummary, Text: "Previously accepted model text."}}}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, accepted)
	bus.Mutable.SetPendingAnswerDocumentPatchBase(doc)
	bus.Mutable.SetAnswerDiagramRelationRepairLease(lease)
	params, _ := json.Marshal(map[string]any{
		"unchanged_block_ids": []string{"summary"},
		"diagram_edge_edits":  []map[string]string{{"failure_ref": selected.FailureRef, "action": "remove"}},
	})
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil || result.Success || result.Repair == nil || result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeStagedForRetry {
		t.Fatalf("body removal must stage the still-invalid metadata dependency: err=%v result=%+v", err, result)
	}
	staged := bus.Mutable.PendingAnswerDocumentPatchBase()
	if staged == nil || strings.Contains(staged.Blocks[1].Diagram.Body, "X->>Y") || len(staged.Blocks[1].EdgeAnchors) < 2 || !reflect.DeepEqual(bus.Mutable.AnswerDocumentV2(), accepted) {
		t.Fatalf("selected body change/stale metadata/accepted ownership premise lost: staged=%+v accepted=%+v", staged, bus.Mutable.AnswerDocumentV2())
	}
	if staged.Blocks[1].Diagram.Body != strings.ReplaceAll(doc.Blocks[1].Diagram.Body, "    X->>Y: model-selected message\n", "") || !reflect.DeepEqual(staged.Blocks[1].EdgeAnchors[0], doc.Blocks[1].EdgeAnchors[0]) {
		t.Fatalf("unselected model declarations or grounded relation changed: %+v", staged)
	}
	t.Logf("public body removal staged: remaining metadata=%d old candidates=%d has_current_lease=%t", len(staged.Blocks[1].EdgeAnchors)-1, len(lease.OptionalOrphanCleanups), bus.Mutable.AnswerDiagramRelationRepairLease() != nil)
	return bus, accepted, staged, result, lease
}

func TestB1657MetadataDependencyKeepsExactOptionalCleanupSource(t *testing.T) {
	bus, _, _, result, source := b1657MetadataDependency(t)
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatalf("actual staged metadata delta: %v", err)
	}
	stale := 0
	for _, failure := range delta.Failures {
		if failure.Issue == diagramCallEdgeIssueAnchorWithoutBodyEdge && failure.FromNode == "X" && failure.ToNode == "Y" {
			stale++
		}
	}
	if stale == 0 {
		t.Fatalf("ordinary validator did not publish the real metadata dependencies: %+v", delta)
	}
	lease := bus.Mutable.AnswerDiagramRelationRepairLease()
	if lease == nil || lease.OrphanDispositionOnly || len(lease.OptionalOrphanCleanups) != len(source.OptionalOrphanCleanups) {
		t.Fatalf("metadata dependency lost original optional cleanup provenance: %d exact source candidates; stale failures=%d lease=%+v", len(source.OptionalOrphanCleanups), stale, lease)
	}
	for _, candidate := range lease.OptionalOrphanCleanups {
		if (candidate.ParticipantID != "X" && candidate.ParticipantID != "Y") || !types.AnswerDiagramOrphanMetadataDependencyMatchesBase(bus.Mutable.PendingAnswerDocumentPatchBase(), candidate, lease) {
			t.Fatalf("current cleanup source borrowed or incomplete: %+v", candidate)
		}
	}
	if strings.Count(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON], `"decision_optional":true`) != 2 {
		t.Fatalf("system roster must mark exactly the two optional decisions: %s", result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON])
	}
}

func TestB1657MetadataCleanupOmissionStillAcceptsWithoutDeletingDeclarations(t *testing.T) {
	bus, _, staged, result, _ := b1657MetadataDependency(t)
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatal(err)
	}
	lease := bus.Mutable.AnswerDiagramRelationRepairLease()
	if lease == nil {
		t.Fatalf("actual metadata delta not executable: %+v", delta)
	}
	var edits []map[string]string
	for _, failure := range lease.Failures {
		if failure.TargetCarrier == types.AnswerDiagramRelationRepairCarrierStaleAnchor && failure.FromNode == "X" && failure.ToNode == "Y" {
			edits = append(edits, map[string]string{"failure_ref": failure.FailureRef, "action": "remove"})
		}
	}
	if len(edits) != len(staged.Blocks[1].EdgeAnchors)-1 {
		t.Fatalf("metadata removal budget does not match current exact occurrences: %+v", lease)
	}
	acceptedBefore, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
	pendingBefore, _ := json.Marshal(bus.Mutable.PendingAnswerDocumentPatchBase())
	unauthorized, _ := json.Marshal(map[string]any{
		"diagram_edge_edits": edits,
		"diagram_participant_edits": []map[string]string{{
			"block_id": "diagram", "participant_id": "Z", "action": "remove_if_isolated",
		}},
	})
	denied, err := (&EmitAnswerDocumentPatch{}).Execute(bus, unauthorized)
	if err != nil || denied.Success || denied.Repair == nil || denied.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeNotStaged {
		t.Fatalf("unrelated pre-existing isolated declaration gained cleanup permission: err=%v result=%+v", err, denied)
	}
	acceptedAfter, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
	pendingAfter, _ := json.Marshal(bus.Mutable.PendingAnswerDocumentPatchBase())
	if string(acceptedAfter) != string(acceptedBefore) || string(pendingAfter) != string(pendingBefore) {
		t.Fatal("unauthorized declaration operation partly committed its sibling metadata removals")
	}
	params, _ := json.Marshal(map[string]any{"diagram_edge_edits": edits})
	cleaned, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil || !cleaned.Success {
		t.Fatalf("omitting new cosmetic choices must not add a required retry: err=%v result=%+v", err, cleaned)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || got.Blocks[1].Diagram.Body != staged.Blocks[1].Diagram.Body || len(got.Blocks[1].EdgeAnchors) != 1 || !reflect.DeepEqual(got.Blocks[1].EdgeAnchors[0], staged.Blocks[1].EdgeAnchors[0]) {
		t.Fatalf("metadata cleanup authored a declaration/edge or damaged the kept native relation: %+v", got)
	}
	if bus.Mutable.PendingAnswerDocumentPatchBase() != nil {
		t.Fatal("successful cleanup must not force a cosmetic staged generation")
	}
}

func TestB1657MetadataCleanupIsModelSelectedAndCanContinueInBatches(t *testing.T) {
	for _, action := range []string{"remove_if_isolated", "retain_as_context"} {
		for _, partial := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/partial=%t", action, partial), func(t *testing.T) {
				bus, _, _, _, _ := b1657MetadataDependency(t)
				lease := bus.Mutable.AnswerDiagramRelationRepairLease()
				if partial {
					oldRef := lease.Failures[0].FailureRef
					params, _ := json.Marshal(map[string]any{"diagram_edge_edits": []map[string]string{{"failure_ref": oldRef, "action": "remove"}}})
					result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
					if err != nil || result.Success || result.Repair == nil || result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeStagedForRetry {
						t.Fatalf("partial metadata repair did not keep exact dependency stage: err=%v result=%+v", err, result)
					}
					lease = bus.Mutable.AnswerDiagramRelationRepairLease()
					if lease == nil || len(lease.Failures) != 1 || len(lease.OptionalOrphanCleanups) != 2 {
						t.Fatalf("partial cleanup lost remaining source: %+v", lease)
					}
					for _, candidate := range lease.OptionalOrphanCleanups {
						if !types.AnswerDiagramOrphanMetadataDependencyMatchesBase(bus.Mutable.PendingAnswerDocumentPatchBase(), candidate, lease) {
							t.Fatalf("partial cleanup retained a stale base/ref: %+v", candidate)
						}
					}
					stagedBefore, _ := json.Marshal(bus.Mutable.PendingAnswerDocumentPatchBase())
					stale, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
					stagedAfter, _ := json.Marshal(bus.Mutable.PendingAnswerDocumentPatchBase())
					if err != nil || stale.Success || string(stagedBefore) != string(stagedAfter) {
						t.Fatalf("consumed metadata ref replay changed the next base: err=%v result=%+v", err, stale)
					}
				}
				staged := bus.Mutable.PendingAnswerDocumentPatchBase()
				var edits []map[string]string
				for i := len(lease.Failures) - 1; i >= 0; i-- {
					edits = append(edits, map[string]string{"failure_ref": lease.Failures[i].FailureRef, "action": "remove"})
				}
				choice := map[string]string{"block_id": "diagram", "participant_id": "X", "action": action}
				if action == "retain_as_context" {
					choice["visible_label"] = "Model-chosen context label"
				}
				params, _ := json.Marshal(map[string]any{"diagram_edge_edits": edits, "diagram_participant_edits": []map[string]string{choice}})
				result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
				if err != nil || !result.Success {
					t.Fatalf("current optional model choice did not execute: err=%v result=%+v", err, result)
				}
				got := bus.Mutable.AnswerDocumentV2()
				if got == nil || len(got.Blocks[1].EdgeAnchors) != 1 || !reflect.DeepEqual(got.Blocks[1].EdgeAnchors[0], staged.Blocks[1].EdgeAnchors[0]) || !reflect.DeepEqual(got.Blocks[0], staged.Blocks[0]) {
					t.Fatalf("optional cleanup changed unrelated native evidence/model text: %+v", got)
				}
				wantBody := strings.Replace(staged.Blocks[1].Diagram.Body, "    participant X as Alpha\n", "", 1)
				if action == "retain_as_context" {
					wantBody = strings.Replace(staged.Blocks[1].Diagram.Body, "    participant X as Alpha", `    participant X as "Model-chosen context label"`, 1)
				}
				if got.Blocks[1].Diagram.Body != wantBody {
					t.Fatalf("declarations other than the selected X changed:\nwant=%s\ngot=%s", wantBody, got.Blocks[1].Diagram.Body)
				}
			})
		}
	}
}

func TestB1657MetadataCleanupCannotStartFromMissingOriginalSource(t *testing.T) {
	bus, _, _, _, _ := b1657MetadataDependency(t, true)
	lease := bus.Mutable.AnswerDiagramRelationRepairLease()
	if lease == nil || len(lease.OptionalOrphanCleanups) != 0 {
		t.Fatalf("metadata alone manufactured original deletion provenance: %+v", lease)
	}
	var edits []map[string]string
	for _, failure := range lease.Failures {
		edits = append(edits, map[string]string{"failure_ref": failure.FailureRef, "action": "remove"})
	}
	before, _ := json.Marshal(bus.Mutable.PendingAnswerDocumentPatchBase())
	params, _ := json.Marshal(map[string]any{"diagram_edge_edits": edits, "diagram_participant_edits": []map[string]string{{"block_id": "diagram", "participant_id": "X", "action": "remove_if_isolated"}}})
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	after, _ := json.Marshal(bus.Mutable.PendingAnswerDocumentPatchBase())
	if err != nil || result.Success || string(before) != string(after) {
		t.Fatalf("source-free declaration deletion was not atomically refused: err=%v result=%+v", err, result)
	}
}
