package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The original candidate roster is produced by the real agent installer, not
// supplied by this test. Keep the thirteen independently grounded fixture calls
// so the metadata-only repair cannot succeed by erasing the source diagram.
func b1657ActualInstalledMetadata(t *testing.T) (*types.BusContext, *types.AgentContext, types.ToolResult) {
	t.Helper()
	bus, doc, labels := b1649ThirteenActualPairs(t)
	for i := 1; i <= 13; i++ {
		node := fmt.Sprintf("n%d", i)
		doc.Blocks[1].EdgeAnchors = append(doc.Blocks[1].EdgeAnchors, types.DiagramEdgeAnchor{FromNode: "n0", ToNode: node,
			FromIdentity: "sample.Caller", ToIdentity: fmt.Sprintf("Step%02d", i), RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge, VisibleLabel: labels[node]})
	}
	doc.Blocks[1].Diagram.Body += " participant X as Alpha\n participant Y as Beta\n participant Z as ExistingContext\n X->>Y: unsupported model edge\n"
	for i := 0; i < 3; i++ {
		doc.Blocks[1].EdgeAnchors = append(doc.Blocks[1].EdgeAnchors, types.DiagramEdgeAnchor{FromNode: "X", ToNode: "Y",
			FromIdentity: "Alpha", ToIdentity: "Beta", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge, VisibleLabel: fmt.Sprintf("metadata %d", i)})
	}
	ctx := &types.AgentContext{Mutable: bus.Mutable, AnalysisIR: bus.AnalysisIR, EvidenceItems: bus.EvidenceItems, RepoRoot: bus.RepoRoot}
	raw, _ := json.Marshal(doc)
	initial, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || initial.Success || !installAnswerDocDiagramRelationRepairLease(ctx, bus.Mutable, &initial, false) {
		t.Fatalf("actual initial Emit/installer failed: err=%v result=%+v", err, initial)
	}
	lease := bus.Mutable.AnswerDiagramRelationRepairLease()
	if lease == nil || len(lease.OptionalOrphanCleanups) != 2 {
		t.Fatalf("actual dispatcher did not identify original X/Y candidates: %+v", lease)
	}
	for _, candidate := range lease.OptionalOrphanCleanups {
		if (candidate.ParticipantID != "X" && candidate.ParticipantID != "Y") || candidate.MetadataDependency != nil {
			t.Fatalf("original roster fabricated metadata provenance or unrelated Z: %+v", candidate)
		}
	}
	var selected string
	for _, failure := range lease.Failures {
		if failure.CanRemoveVisibleBodyOccurrence("diagram", "X", "Y", 1, 1) {
			selected = failure.FailureRef
		}
	}
	if selected == "" {
		t.Fatalf("missing actual body removal capability: %+v", lease)
	}
	params, _ := json.Marshal(map[string]any{"diagram_edge_edits": []map[string]string{{"failure_ref": selected, "action": "remove"}}})
	result, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil || result.Success || result.Repair == nil || result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeStagedForRetry {
		t.Fatalf("actual body removal did not stage dependent metadata: err=%v result=%+v", err, result)
	}
	return bus, ctx, result
}

func TestB1657ActualInstallerRetainsOptionalMetadataSource(t *testing.T) {
	for _, mode := range []string{"live", "wire_only", "changed_base"} {
		t.Run(mode, func(t *testing.T) {
			bus, ctx, result := b1657ActualInstalledMetadata(t)
			staged := bus.Mutable.PendingAnswerDocumentPatchBase()
			if mode == "wire_only" {
				wire, _ := json.Marshal(bus.Mutable.AnswerDiagramRelationRepairLease())
				var restored types.AnswerDiagramRelationRepairLease
				if err := json.Unmarshal(wire, &restored); err != nil {
					t.Fatal(err)
				}
				bus.Mutable.SetAnswerDiagramRelationRepairLease(&restored)
			}
			if mode == "changed_base" {
				staged.Blocks[1].EdgeAnchors[13].VisibleLabel = "changed after publication"
				bus.Mutable.SetPendingAnswerDocumentPatchBase(staged)
			}
			if !installAnswerDocDiagramRelationRepairLease(ctx, bus.Mutable, &result, false) {
				t.Fatalf("actual metadata delta did not reinstall its relation lease: %+v", result)
			}
			lease := bus.Mutable.AnswerDiagramRelationRepairLease()
			if mode != "live" {
				if len(lease.OptionalOrphanCleanups) != 0 {
					t.Fatalf("old JSON/stale base reminted private cleanup source: %+v", lease.OptionalOrphanCleanups)
				}
				return
			}
			if len(lease.OptionalOrphanCleanups) != 2 {
				t.Fatalf("agent redispatch erased current optional lineage: %+v", lease)
			}
			for _, candidate := range lease.OptionalOrphanCleanups {
				if !types.AnswerDiagramOrphanMetadataDependencyMatchesBase(staged, candidate, lease) {
					t.Fatalf("redispatch changed exact current source: %+v", candidate)
				}
			}
			if strings.Count(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON], `"decision_optional":true`) != 2 {
				t.Fatal("redispatch dropped read-only optional disclosure")
			}
			hint, ok := answerDocDiagramRelationDeltaPatchHint(&result, true, false)
			if !ok || strings.Count(hint, `"decision_optional":true`) != 2 {
				t.Errorf("actual delta hint dropped optional distinction after JSON roundtrip: %s", hint)
			}
			var schema struct {
				Properties map[string]json.RawMessage `json:"properties"`
			}
			if err := json.Unmarshal((&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx), &schema); err != nil {
				t.Fatal(err)
			}
			participantSchema := string(schema.Properties["diagram_participant_edits"])
			if !strings.Contains(participantSchema, "remove_if_isolated") || !strings.Contains(participantSchema, "retain_as_context") || !strings.Contains(participantSchema, `"X"`) || !strings.Contains(participantSchema, `"Y"`) || strings.Contains(participantSchema, `"Z"`) {
				t.Errorf("actual dispatch hides exact optional actions or broadens to Z: %s", participantSchema)
			}
			var edits []map[string]string
			for _, failure := range lease.Failures {
				edits = append(edits, map[string]string{"failure_ref": failure.FailureRef, "action": "remove"})
			}
			params, _ := json.Marshal(map[string]any{"diagram_edge_edits": edits, "diagram_participant_edits": []map[string]string{{"block_id": "diagram", "participant_id": "X", "action": "remove_if_isolated"}}})
			final, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, params)
			if err != nil || !final.Success {
				t.Fatalf("redispatched optional selection was not publicly executable: err=%v result=%+v", err, final)
			}
			got := bus.Mutable.AnswerDocumentV2()
			if got == nil || len(got.Blocks[1].EdgeAnchors) != 13 || strings.Contains(got.Blocks[1].Diagram.Body, "participant X") || !strings.Contains(got.Blocks[1].Diagram.Body, "participant Y") || !strings.Contains(got.Blocks[1].Diagram.Body, "participant Z") {
				t.Fatalf("model-selected cleanup changed an unselected declaration/native call: %+v", got)
			}
		})
	}
}

func TestB1657InstallerFindsPrivateSourceAcrossBothMutableCarriers(t *testing.T) {
	for _, mode := range []string{"ctx_new", "primary_new", "both_same"} {
		t.Run(mode, func(t *testing.T) {
			bus, ctx, result := b1657ActualInstalledMetadata(t)
			original := bus.Mutable
			other := types.NewMutableState("different carrier")
			other.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "old", Kind: types.BlockSummary, Text: "old accepted"}}})
			primary := other
			if mode == "primary_new" {
				ctx.Mutable = other
				primary = original
			}
			if mode == "both_same" {
				other.SetPendingAnswerDocumentPatchBase(original.PendingAnswerDocumentPatchBase())
				other.SetAnswerDiagramRelationRepairLease(original.AnswerDiagramRelationRepairLease())
			}
			if !installAnswerDocDiagramRelationRepairLease(ctx, primary, &result, false) {
				t.Fatal("actual installer refused the current metadata delta")
			}
			for _, mu := range []*types.MutableState{ctx.Mutable, primary} {
				lease := mu.AnswerDiagramRelationRepairLease()
				if lease == nil || len(lease.OptionalOrphanCleanups) != 2 {
					t.Errorf("exact private source was lost/duplicated by carrier precedence: %+v", lease)
				}
			}
		})
	}
}
