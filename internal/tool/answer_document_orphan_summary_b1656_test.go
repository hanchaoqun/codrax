package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the actual relation-only Patch -> staged orphan generation. The
// original roster also names still-connected B/C; only A becomes missing.
func b1656StageOrphans(t *testing.T, blockIDs []string) (types.ToolResult, *types.AnswerDiagramRelationRepairLease) {
	t.Helper()
	base := atomicPatchTestDocument()
	template := base.Blocks[1]
	base.Blocks = base.Blocks[:1]
	var failures []types.AnswerDiagramRelationRepairFailure
	for _, id := range blockIDs {
		block := cloneAtomicDiagramPatchBlock(template)
		block.ID = id
		base.Blocks = append(base.Blocks, block)
		failures = append(failures, types.AnswerDiagramRelationRepairFailure{
			BlockID: id, Issue: "semantic_relation_edge_unproven", FromNode: "A", ToNode: "B",
			FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
		})
	}
	lease := types.NewAnswerDiagramRelationRepairLease(base, failures, nil)
	if lease == nil {
		t.Fatal("missing relation lease fixture")
	}
	for _, id := range blockIDs {
		lease.OptionalOrphanCleanups = append(lease.OptionalOrphanCleanups, testDiagramOrphanCandidates(id, "A", "B", "C")...)
	}
	before, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	mut := types.NewMutableState("orphan diagnostic ownership")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	var edits []map[string]string
	for _, f := range lease.Failures {
		edits = append(edits, map[string]string{"failure_ref": f.FailureRef, "action": "remove"})
	}
	params, err := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": edits})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, params)
	if err != nil || result.Success || result.Repair == nil || result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeStagedForRetry {
		t.Fatalf("expected actual staged orphan exit: err=%v result=%+v", err, result)
	}
	after, _ := json.Marshal(mut.AnswerDocumentV2())
	if string(before) != string(after) {
		t.Fatal("diagnostic stage modified accepted model document")
	}
	live := mut.AnswerDiagramRelationRepairLease()
	if live == nil || !live.OrphanDispositionOnly || len(live.OptionalOrphanCleanups) != len(blockIDs) || len(live.Failures) != 0 || len(live.AllowedAdditions) != 0 {
		t.Fatalf("new lease must contain only current missing participants: %+v", live)
	}
	for _, c := range live.OptionalOrphanCleanups {
		if c.ParticipantID != "A" || !reflect.DeepEqual(c.AllowedActions, testDiagramOrphanCandidates(c.BlockID, "A")[0].AllowedActions) {
			t.Fatalf("source roster changed: %+v", c)
		}
	}
	wantRepair := answerDiagramRelationRepairScopeRepair(live, nil)
	if result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON] != wantRepair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON] {
		t.Fatal("diagnostic changed the complete typed delta")
	}
	pending := mut.PendingAnswerDocumentPatchBase()
	if pending == nil || len(pending.Blocks) != len(base.Blocks) {
		t.Fatal("missing exact staged document")
	}
	for i, original := range base.Blocks {
		expected := cloneAtomicDiagramPatchBlock(original)
		if i > 0 {
			expected.Diagram.Body = strings.Replace(expected.Diagram.Body, "    A->>B: old label\n", "", 1)
			expected.EdgeAnchors = expected.EdgeAnchors[1:]
		}
		if !reflect.DeepEqual(pending.Blocks[i], expected) {
			t.Fatalf("staged block %q changed beyond the selected edge removal", original.ID)
		}
	}
	return result, live
}

func TestB1656StagedOrphanSummaryUsesCurrentExactRoster(t *testing.T) {
	for _, ids := range [][]string{{"diag"}, {"diag-b", "diag-a"}} {
		t.Run(fmt.Sprint(len(ids)), func(t *testing.T) {
			result, live := b1656StageOrphans(t, ids)
			prefix := fmt.Sprintf("diagram relation phase staged; explicit orphan disposition is required for %d participant(s)", len(ids))
			if !strings.Contains(result.Summary, prefix) {
				t.Fatalf("original count prefix changed: %s", result.Summary)
			}
			for _, c := range live.OptionalOrphanCleanups {
				want := fmt.Sprintf("block=%q, participant=%q, allowed_actions=[\"remove_if_isolated\",\"retain_as_context\"]", c.BlockID, c.ParticipantID)
				if !strings.Contains(result.Summary, want) {
					t.Errorf("public summary omitted exact current choice %s: %s", want, result.Summary)
				}
			}
			for _, old := range []string{`participant="B"`, `participant="C"`} {
				if strings.Contains(result.Summary, old) {
					t.Errorf("old connected candidate leaked into current summary: %s", result.Summary)
				}
			}
		})
	}
}

func TestB1656StagedOrphanSummaryBoundsOnlyDisplay(t *testing.T) {
	for _, count := range []int{8, 9} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			ids := make([]string, count)
			for i := range ids {
				ids[i] = fmt.Sprintf("diag-%02d", i)
			}
			result, live := b1656StageOrphans(t, ids)
			if strings.Count(result.Summary, `participant="A"`) != 8 {
				t.Fatalf("bounded summary must show 8 exact rows: %s", result.Summary)
			}
			if (count == 9) != strings.Contains(result.Summary, "1 additional current orphan choice(s) omitted") {
				t.Fatalf("display omission not accurately disclosed: %s", result.Summary)
			}
			if len(live.OptionalOrphanCleanups) != count {
				t.Fatal("display limit altered full lease")
			}
		})
	}
	// Overlong source identifiers are omitted whole, never turned into a new
	// copyable truncated block/participant selector. Their full delta survives.
	result, live := b1656StageOrphans(t, []string{"diagram-" + strings.Repeat("界", 200)})
	if strings.Contains(result.Summary, "block=") || !strings.Contains(result.Summary, "1 additional current orphan choice(s) omitted") || !strings.Contains(result.Summary, "complete current schema and optional_orphan_cleanups") {
		t.Fatalf("oversized selector needs an explicit full-roster route: %s", result.Summary)
	}
	if len(live.OptionalOrphanCleanups[0].BlockID) < 600 {
		t.Fatal("display shortened actual selector")
	}
}

func TestB1656OrphanPreviewDoesNotChooseOrMutate(t *testing.T) {
	for _, lease := range []*types.AnswerDiagramRelationRepairLease{nil, {}, {OrphanDispositionOnly: true}} {
		if got := stagedOrphanDispositionSummary(lease); got != "" {
			t.Fatalf("no current orphan-only roster should invent a choice: %s", got)
		}
	}
	lease := &types.AnswerDiagramRelationRepairLease{OrphanDispositionOnly: true, OptionalOrphanCleanups: []types.AnswerDiagramOrphanCleanupCandidate{{
		BlockID: "diagram\nquoted\"", ParticipantID: "KeepMe",
		AllowedActions: []types.AnswerDiagramOrphanDispositionAction{types.AnswerDiagramOrphanDispositionRetain},
	}}}
	before, _ := json.Marshal(lease)
	got := stagedOrphanDispositionSummary(lease)
	after, _ := json.Marshal(lease)
	if string(before) != string(after) || strings.Contains(got, "remove_if_isolated") || strings.Contains(got, "\n") || !strings.Contains(got, `allowed_actions=["retain_as_context"]`) || !strings.Contains(got, `block="diagram\nquoted\""`) {
		t.Fatalf("preview modified or widened the exact typed choice: %s", got)
	}
	// The exact byte boundary is only a display limit; do not shorten even
	// one byte of a source identifier to squeeze another row into the preview.
	rowBase := len(`{block="", participant="KeepMe", allowed_actions=["retain_as_context"]}`)
	for _, size := range []int{512, 513} {
		lease.OptionalOrphanCleanups[0].BlockID = strings.Repeat("x", size-rowBase)
		got := stagedOrphanDispositionSummary(lease)
		if strings.Contains(got, "block=") != (size == 512) || strings.Contains(got, "omitted") != (size == 513) {
			t.Fatalf("row byte boundary %d not respected: %s", size, got)
		}
	}
}
