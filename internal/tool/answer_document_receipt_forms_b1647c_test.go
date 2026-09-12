package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The starting document is a saved legacy fixture, not a claim that modern
// full emit accepts an unresolved partial identity. Both retry generations
// below are produced by public Patch execution; no normalization receipt or
// orphan-only lease is supplied by this test. Source authority is obtained
// through the actual ReadFile -> EmitEvidence helper used by B1647.
func TestB1647cPublicOrphanNormalizesLegalIdentityForms(t *testing.T) {
	for _, tc := range []struct {
		name, from, to, action string
	}{
		{"from_only", "Caller", "", "remove_if_isolated"},
		{"to_only", "", "Callee", "retain_as_context"},
		{"display_qualified", "Caller (source)", "Callee (source)", "remove_if_isolated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evidence, repo := b1647TwoCallOccurrences(t)
			prev := atomicPatchTestDocument()
			prev.Blocks[1].Diagram.Body = "sequenceDiagram\n participant A as Caller\n participant B as Callee\n participant C as OldCaller\n participant D as OldCallee\n A->>B: keep\n C->>D: remove\n"
			prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{
				{FromNode: "A", ToNode: "B", FromIdentity: tc.from, ToIdentity: tc.to, RelationKind: types.DiagramRelCall, VisibleLabel: "keep"},
				{FromNode: "C", ToNode: "D", FromIdentity: "OldCaller", ToIdentity: "OldCallee", RelationKind: types.DiagramRelCall, VisibleLabel: "remove"},
			}
			ev := evidence[0]
			prev.Citations = []types.Citation{{File: ev.Source, Line: ev.LineStart, LineEnd: ev.LineEnd, Scope: ev.Scope, Quote: ev.Snippet}}
			lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
				BlockID: "diag", Issue: "call_edge_unproven", FromNode: "C", ToNode: "D", FromIdentity: "OldCaller", ToIdentity: "OldCallee", RelationKind: types.DiagramRelCall, BodyOccurrence: 1,
			}}, nil)
			if lease == nil || len(lease.Failures) != 1 {
				t.Fatalf("initial removal lease missing: %+v", lease)
			}
			lease.OptionalOrphanCleanups = testDiagramOrphanCandidates("diag", "D")
			mut := types.NewMutableState("B1647c public inherited identity forms")
			bus := &types.BusContext{Mutable: mut, RepoRoot: repo, WorkDir: repo, AnalysisIR: &types.AnalysisIR{}, EvidenceItems: evidence}
			mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
			mut.SetAnswerDiagramRelationRepairLease(lease)
			mut.SetFinalizerTypedRelationRecipeAvailable(true)
			mut.SetFinalizerTypedRelationRecipeAnchors([]types.DiagramEdgeAnchor{{
				FromNode: "n3", ToNode: "n4", FromIdentity: "Caller", ToIdentity: "Callee", RelationKind: types.DiagramRelCall,
			}})
			accepted := mut.AnswerDocumentV2()
			view := types.BuildAnswerSemanticViewForBusContext(bus)
			if view == nil || view.Family == types.QFRootCauseTrace {
				t.Fatal("ordinary source relation validation must be active")
			}
			first := json.RawMessage(fmt.Sprintf(`{"diagram_edge_edits":[{"action":"remove","failure_ref":%q}]}`, lease.Failures[0].FailureRef))
			result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, first)
			if err != nil || result.Success || result.Repair == nil || result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeStagedForRetry {
				t.Fatalf("first public removal must stage an orphan choice: err=%v result=%+v", err, result)
			}
			staged, orphanLease := mut.PendingAnswerDocumentPatchBase(), mut.AnswerDiagramRelationRepairLease()
			if staged == nil || orphanLease == nil || !orphanLease.OrphanDispositionOnly || len(orphanLease.Failures) != 0 || len(orphanLease.AllowedAdditions) != 0 || len(orphanLease.OptionalOrphanCleanups) != 1 || orphanLease.OptionalOrphanCleanups[0].ParticipantID != "D" {
				t.Fatalf("second generation must be actual orphan-only state: %+v", orphanLease)
			}
			if !reflect.DeepEqual(accepted, mut.AnswerDocumentV2()) || len(staged.Blocks[1].EdgeAnchors) != 1 || staged.Blocks[1].EdgeAnchors[0] != prev.Blocks[1].EdgeAnchors[0] || !reflect.DeepEqual(staged.Blocks[1].EdgeAnchors, orphanLease.Blocks[0].BaseAnchors) {
				t.Fatal("stage must retain the exact inherited partial/qualified baseline without publishing it")
			}
			stageJSON, _ := json.Marshal(staged)
			leaseJSON, _ := json.Marshal(orphanLease)
			schema := string((&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut, EvidenceItems: evidence}))
			if !strings.Contains(schema, `"diagram_participant_edits"`) || !strings.Contains(schema, `"`+tc.action+`"`) || strings.Contains(schema, `"diagram_edge_edits"`) {
				t.Fatal("orphan retry must not publish a relation-edit escape")
			}
			choice := map[string]any{"block_id": "diag", "participant_id": "D", "action": tc.action}
			if tc.action == "retain_as_context" {
				choice["visible_label"] = "OldCallee (context only)"
			}
			raw, _ := json.Marshal(map[string]any{"diagram_participant_edits": []any{choice}})
			result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
			if err != nil || !result.Success {
				t.Fatalf("legal inherited form must survive actual receipt and ordinary gate: err=%v result=%+v", err, result)
			}
			got := mut.AnswerDocumentV2()
			if got == nil || !reflect.DeepEqual(got.Blocks[0], accepted.Blocks[0]) || len(got.Citations) < len(accepted.Citations) || !reflect.DeepEqual(got.Citations[:len(accepted.Citations)], accepted.Citations) {
				t.Fatal("orphan choice changed unselected model prose or original citations")
			}
			want := staged.Blocks[1].EdgeAnchors[0]
			want.FromIdentity, want.ToIdentity, want.ClaimForm = "Caller", "Callee", types.ClaimCallEdge
			if len(got.Blocks[1].EdgeAnchors) != 1 || got.Blocks[1].EdgeAnchors[0] != want || b1647MessageLines(got.Blocks[1].Diagram.Body) != b1647MessageLines(staged.Blocks[1].Diagram.Body) {
				t.Fatal("only legal hidden identity normalization may accompany the orphan choice")
			}
			if issues := DiagramCallEdgeEvidenceMismatches(got, view, evidence); len(issues) != 0 {
				t.Fatalf("final visible relation must retain real grounded call authority: %+v", issues)
			}
			if tc.action == "remove_if_isolated" && strings.Contains(got.Blocks[1].Diagram.Body, "participant D") || tc.action == "retain_as_context" && !strings.Contains(got.Blocks[1].Diagram.Body, "OldCallee (context only)") {
				t.Fatal("the selected orphan disposition did not persist")
			}
			afterStage, _ := json.Marshal(staged)
			afterLease, _ := json.Marshal(orphanLease)
			if string(stageJSON) != string(afterStage) || string(leaseJSON) != string(afterLease) || mut.PendingAnswerDocumentPatchBase() != nil || mut.AnswerDiagramRelationRepairLease() != nil {
				t.Fatal("accepted retry changed captured inputs or retained a stale generation")
			}
		})
	}
}
