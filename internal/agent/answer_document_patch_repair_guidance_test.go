package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1584PatchRetrySequenceDoesNotInventUnchangedFailureOrFinality(t *testing.T) {
	ctx := ctxWithAnswerPatchBase()
	evaluator := &answerDocumentEvaluator{}
	for i, field := range []string{"diagram_participant_edits", "blocks[].edge_anchors", "diagram_edge_edits[i].edge.from_node", "blocks[].facet_ids"} {
		outcome := types.AnswerDocumentPatchOutcomeStagedForRetry
		if i == 2 {
			outcome = types.AnswerDocumentPatchOutcomeNotStaged
		}
		result := &types.ToolResult{ToolName: "emit_answer_document_patch", Success: false,
			Repair: &types.ToolRepair{Code: "tool_param_misplaced_field", Fields: []string{field},
				Hint:     "Follow the named current schema field without altering unrelated model content.",
				Metadata: map[string]string{types.ToolRepairMetaAnswerDocumentPatchOutcome: outcome}}}
		signal := evaluator.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
		if !signal.HintRequested || !signal.BypassBudget || !signal.BypassThrottle || evaluator.rejectHintsUsed != i+1 {
			t.Fatalf("wording change must preserve real retry routing and counters: %+v", signal)
		}
		if !strings.Contains(signal.Hint, field) {
			t.Fatalf("current precise issue disappeared: %s", signal.Hint)
		}
		if i > 0 && !strings.Contains(signal.Hint, fmt.Sprintf("Repair attempt #%d", i+1)) {
			t.Errorf("retry should report only known cumulative attempt: %s", signal.Hint)
		}
		for _, unsupported := range []string{"SAME issue", "previous fix did not address", "FINAL RETRY", "ships with the violation"} {
			if strings.Contains(signal.Hint, unsupported) {
				t.Errorf("attempt counter alone cannot establish %q: %s", unsupported, signal.Hint)
			}
		}
		want := "exact merged draft is already the live retry base"
		if outcome == types.AnswerDocumentPatchOutcomeNotStaged {
			want = "the live retry base is unchanged"
		}
		if !strings.Contains(signal.Hint, want) {
			t.Fatalf("precise executor state must remain authoritative: %s", signal.Hint)
		}
	}
}

func TestB1584RelationRepairEntryPublishesNestedAddGuidance(t *testing.T) {
	for _, required := range []bool{false, true} {
		ctx := ctxWithAnswerPatchBase()
		const ref = "ra1-current-choice"
		ctx.Mutable.SetAnswerDiagramRelationRepairLease(&types.AnswerDiagramRelationRepairLease{
			Version: 1, Blocks: []types.AnswerDiagramRelationRepairLeaseBlock{{BlockID: "path", Kind: types.BlockDiagram}},
			AllowedAdditions: []types.AnswerDiagramRelationRepairCandidate{{AdditionRef: ref, BlockID: "path",
				RelationKind: types.DiagramRelArgumentFlow, FromIdentity: "input.Kind", ToIdentity: "Factory.resolve", Source: "src/runner.py:10"}},
		})
		evaluator := &answerDocumentEvaluator{diagramRequired: required, mu: ctx.Mutable}
		result := &types.ToolResult{ToolName: "emit_answer_document_patch", Success: false,
			Repair: &types.ToolRepair{Code: types.ToolRepairCodeAnswerDocRelationRepairScope,
				Metadata: map[string]string{
					types.ToolRepairMetaAnswerDocumentPatchOutcome:     types.AnswerDocumentPatchOutcomeStagedForRetry,
					types.ToolRepairMetaDiagramRelationRepairDeltaJSON: `{"version":1,"failures":[],"preserve_unlisted_edges":true,"allowed_additions":[{"addition_ref":"` + ref + `","block_id":"path","relation_kind":"argument_flow","from_identity":"input.Kind","to_identity":"Factory.resolve","source":"src/runner.py:10"}]}`,
				}}}
		signal := evaluator.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
		if !signal.HintRequested || !strings.HasPrefix(signal.HintKey, "answer_doc.patch_relation_repair_scope") {
			t.Fatalf("expected actual relation-repair entry, got %+v", signal)
		}
		for _, want := range []string{"diagram_edge_edits[].edge.{from_node,to_node,visible_label}", "ref-selected", "replace_blocks", "endpoint identities", ref} {
			if !strings.Contains(signal.Hint, want) {
				t.Errorf("required=%v relation retry omitted %q: %s", required, want, signal.Hint)
			}
		}
		if strings.Contains(signal.Hint, "last atomic relation operation was not executable") {
			t.Errorf("an additions-only roster does not prove the staged prior operation failed: %s", signal.Hint)
		}
	}
}
