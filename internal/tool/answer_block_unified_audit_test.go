package tool

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// G2-4 (post_v2_runtime_gap_remediation, 2026-05-04). Cross-path
// equivalence audits: full emit and patch emit must produce
// byte-equivalent merged docs when fed equivalent inputs. Locks the
// G2 unified-mutation contract at the chokepoint level — if a future
// change reintroduces parallel maintenance the test fails on
// reflect.DeepEqual.

// Both paths are wired through the same NormalizeEmitAnswerBlock +
// AnswerDocumentMutation.Apply. Asserting equivalence here makes the
// chokepoint guarantee structurally enforceable.
func TestUnifiedMutation_FullAndPatchProduceEquivalentMergedDocs(t *testing.T) {
	// Construct a target doc the LLM might emit in two ways:
	//   (A) full emit_answer_document — all blocks named verbatim
	//   (B) patch emit — start from a "draft" prev, then ReplaceBlocks
	//       on every block to reach the same final state
	// The merged result must be reflect.DeepEqual.

	// Step 1: full emit fresh doc.
	fullJSON := json.RawMessage(`{
		"document_model": "v2",
		"blocks": [
			{"id": "s1", "kind": "summary", "surface_role": "principal", "text": "final summary",
			 "facet_ids": ["current_code_path"],
			 "claim_uses": [{"claim_form": "definition_fact", "evidence_id": "ev1"}]
			},
			{"id": "d1", "kind": "diagram",
			 "diagram": {"kind": "flow", "language": "mermaid", "body": "flowchart LR\n  A --> B"},
			 "edge_anchors": [{"from_node": "A", "to_node": "B", "relation_kind": "call", "claim_form": "call_edge"}]
			}
		]
	}`)
	busFull := &types.BusContext{Mutable: &types.MutableState{}}
	fullTool := &EmitAnswerDocument{}
	if res, _ := fullTool.Execute(busFull, fullJSON); !res.Success {
		t.Fatalf("full emit failed: %+v", res)
	}
	fullDoc := busFull.Mutable.AnswerDocumentV2()
	if fullDoc == nil {
		t.Fatal("full emit produced nil doc")
	}

	// Step 2: same final state via patch (start from a draft prev with
	// stale text, then ReplaceBlocks every block to the target shape).
	busPatch := &types.BusContext{Mutable: &types.MutableState{}}
	busPatch.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "stale"},
			{ID: "d1", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Body: "flowchart LR\n  X --> Y"}},
		},
	})
	patchJSON := json.RawMessage(`{
		"replace_blocks": [
			{"id": "s1", "kind": "summary", "surface_role": "principal", "text": "final summary",
			 "facet_ids": ["current_code_path"],
			 "claim_uses": [{"claim_form": "definition_fact", "evidence_id": "ev1"}]
			},
			{"id": "d1", "kind": "diagram",
			 "diagram": {"kind": "flow", "language": "mermaid", "body": "flowchart LR\n  A --> B"},
			 "edge_anchors": [{"from_node": "A", "to_node": "B", "relation_kind": "call", "claim_form": "call_edge"}]
			}
		]
	}`)
	patchTool := &EmitAnswerDocumentPatch{}
	if res, _ := patchTool.Execute(busPatch, patchJSON); !res.Success {
		t.Fatalf("patch emit failed: %+v", res)
	}
	patchDoc := busPatch.Mutable.AnswerDocumentV2()
	if patchDoc == nil {
		t.Fatal("patch emit produced nil doc")
	}

	// Equivalence — sans DocumentModel detail (full sets "v2" on the
	// patch.go ApplyAnswerDocumentV2Patch path too, so they should
	// match verbatim).
	if !reflect.DeepEqual(fullDoc, patchDoc) {
		t.Errorf("full vs patch merged docs DIVERGE — G2 unified mutation invariant broken\nfull:  %+v\npatch: %+v", fullDoc, patchDoc)
	}

	// Specifically lock the EdgeAnchors propagation regression (the
	// G2 motive case): patch path MUST carry edge_anchors on the
	// merged d1 block.
	var patchD1 *types.AnswerBlock
	for i := range patchDoc.Blocks {
		if patchDoc.Blocks[i].ID == "d1" {
			patchD1 = &patchDoc.Blocks[i]
			break
		}
	}
	if patchD1 == nil {
		t.Fatal("patch merged doc missing d1 block")
	}
	if len(patchD1.EdgeAnchors) != 1 || patchD1.EdgeAnchors[0].RelationKind != types.DiagramRelCall {
		t.Errorf("patch path dropped EdgeAnchors (G2-1 regression lock): got %+v", patchD1.EdgeAnchors)
	}
}

// G2-4: Mutation.Summary telemetry surfaces in BOTH emit paths via
// matching log prefixes. We can't easily assert log lines here, but
// we CAN lock the Summary string format used by each path so a
// future operator dashboard can structure-mine without regex
// fragility.
func TestUnifiedMutation_SummaryFormatStableAcrossPaths(t *testing.T) {
	// Full path: ReplaceAll → "replace_all blocks=N citations=M"
	doc := &types.AnswerDocumentV2{DocumentModel: "v2",
		Blocks:    []types.AnswerBlock{{ID: "a"}},
		Citations: []types.Citation{{File: "x.go", Line: 1}},
	}
	fullSummary := types.NewReplaceAllMutation(doc).Summary()
	if fullSummary != "replace_all blocks=1 citations=1" {
		t.Errorf("full summary format drift: %q", fullSummary)
	}

	// Patch path: Partial → "partial unchanged=N replace=M add=K remove=J citations_..."
	patch := &types.AnswerDocumentV2Patch{
		UnchangedBlockIDs: []string{"a"},
		ReplaceBlocks:     []types.AnswerBlock{{ID: "b", Kind: types.BlockSummary}},
	}
	patchSummary := types.NewPartialMutation(patch).Summary()
	if patchSummary != "partial unchanged=1 replace=1 add=0 remove=0 citations_inherited" {
		t.Errorf("patch summary format drift: %q", patchSummary)
	}
}
