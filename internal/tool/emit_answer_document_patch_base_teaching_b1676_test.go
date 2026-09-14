package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1676Base(label string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: label, Kind: types.BlockSummary, Text: label + " model-authored text"}}}
}

func TestB1676PatchUsableBaseControls(t *testing.T) {
	for _, tc := range []struct {
		name, want                        string
		accepted, retry, rejected, staged bool
	}{
		{name: "accepted", want: "accepted", accepted: true},
		{name: "retry snapshot", want: "retry", retry: true},
		{name: "rejected without prior success", want: "rejected", rejected: true},
		{name: "staged without prior success", want: "staged", staged: true},
		{name: "staged precedes every fallback", want: "staged", accepted: true, retry: true, rejected: true, staged: true},
		{name: "accepted precedes snapshots", want: "accepted", accepted: true, retry: true, rejected: true},
		{name: "retry precedes rejected full", want: "retry", retry: true, rejected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mut := types.NewMutableState("repair the answer")
			// These public setters model the four independently owned draft
			// carriers; only patch Execute is under test, not their producers.
			if tc.accepted {
				mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, b1676Base("accepted"))
			}
			if tc.retry {
				raw, err := json.Marshal(b1676Base("retry"))
				if err != nil {
					t.Fatal(err)
				}
				mut.SetRetryState(&types.RetryState{Attempt: 1, PrevEmitJSON: raw})
			}
			if tc.rejected {
				mut.SetLastRejectedAnswerDocumentV2(b1676Base("rejected"))
			}
			if tc.staged {
				mut.StageAnswerDocumentPatchGeneration(b1676Base("staged"), nil)
			}
			params, err := json.Marshal(map[string]any{"unchanged_block_ids": []string{tc.want}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, params)
			if err != nil || !result.Success {
				t.Fatalf("usable %s must permit a patch without a successful full-emit prerequisite: result=%+v err=%v", tc.want, result, err)
			}
			doc := mut.AnswerDocumentV2()
			if doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].ID != tc.want || doc.Blocks[0].Text != tc.want+" model-authored text" {
				t.Fatalf("wrong base selected or inherited content changed: %+v", doc)
			}
		})
	}
}

func TestB1676FullEmitFirstAndCompleteRewriteRemainAvailable(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("write the answer")}
	full := &EmitAnswerDocument{}
	for _, text := range []string{"first complete answer", "completely rewritten answer"} {
		params, err := json.Marshal(map[string]any{"blocks": []map[string]any{{"id": "answer", "kind": "summary", "text": text}}})
		if err != nil {
			t.Fatal(err)
		}
		result, err := full.Execute(bus, params)
		if err != nil || !result.Success {
			t.Fatalf("first/full rewrite should remain legal: result=%+v err=%v", result, err)
		}
		if doc := bus.Mutable.AnswerDocumentV2(); doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].Text != text {
			t.Fatalf("full answer did not replace prior content: %+v", doc)
		}
	}
}

func TestB1676NoBaseDiagnosticDoesNotRequirePriorSuccess(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("first answer")}
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"unchanged_block_ids":["answer"]}`))
	if err != nil || result.Success || bus.Mutable.AnswerDocumentV2() != nil {
		t.Fatalf("missing base must still reject without publishing: result=%+v err=%v", result, err)
	}
	lower := strings.ToLower(result.Summary)
	if strings.Contains(lower, "after a successful") {
		t.Errorf("diagnostic incorrectly denies the usable rejected/staged draft lanes: %s", result.Summary)
	}
	if strings.Contains(lower, "resubmit the complete intended patch") {
		t.Errorf("no addressable base exists, so retry guidance must not require resubmitting a patch: %s", result.Summary)
	}
	for _, want := range []string{"no previous emit", "structured", "draft", "accepted", "rejected", "staged", "first", "emit_answer_document"} {
		if !strings.Contains(lower, want) {
			t.Errorf("missing-base diagnostic must explain the actual prerequisite %q: %s", want, result.Summary)
		}
	}
}

func TestB1676UnavailableBaseDoesNotMintPatchTransactionGuidance(t *testing.T) {
	bad := types.NewMutableState("recover a structurally unaddressable draft")
	bad.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "same", Kind: types.BlockSummary, Text: "first model block"},
		{ID: "same", Kind: types.BlockSummary, Text: "second model block"},
	}})
	for name, bus := range map[string]*types.BusContext{
		"nil context":                nil,
		"no mutable state":           {},
		"no prior draft":             {Mutable: types.NewMutableState("first answer")},
		"duplicate block identities": {Mutable: bad},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"unchanged_block_ids":["same"]}`))
			if err != nil || result.Success {
				t.Fatalf("unavailable base must remain a failed patch: result=%+v err=%v", result, err)
			}
			if result.Repair != nil && result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != "" {
				t.Fatal("no usable base must not acquire a transaction outcome that teaches resubmitting a patch")
			}
			if strings.Contains(result.Summary, "Resubmit the complete intended patch") {
				t.Fatal("cannot resubmit a patch without an addressable base")
			}
		})
	}
}

func TestB1676UsableBaseStructuralRejectRetainsTransactionalGuidance(t *testing.T) {
	mut := types.NewMutableState("repair an existing answer")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, b1676Base("accepted"))
	before, err := json.Marshal(mut.AnswerDocumentV2())
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, json.RawMessage(`{"unchanged_block_ids":["missing"]}`))
	if err != nil || result.Success {
		t.Fatalf("unknown block must remain rejected: result=%+v err=%v", result, err)
	}
	for _, want := range []string{"live retry base is unchanged", "Resubmit the complete intended patch"} {
		if !strings.Contains(result.Summary, want) {
			t.Errorf("usable-base rollback lost valid patch retry guidance %q: %s", want, result.Summary)
		}
	}
	if result.Repair == nil || result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeNotStaged {
		t.Fatalf("usable-base structural rejection lost the typed transaction outcome: %+v", result.Repair)
	}
	after, err := json.Marshal(mut.AnswerDocumentV2())
	if err != nil || string(before) != string(after) || mut.PendingAnswerDocumentPatchBase() != nil {
		t.Fatal("structurally rejected patch changed accepted/staged content")
	}
}
