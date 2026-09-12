package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The public relation edit genuinely stages before the orphan-only patch.
// In the live witness the recipe used n3/n4 while the model used RP/RES;
// unique-topology metadata completion must not become an unlisted model edit.
func TestB1647cPublicBusinessNodeOrphanReceipt(t *testing.T) {
	for _, shape := range []string{"legacy_only", "legacy_only_with_evidence"} {
		for _, businessIDs := range []bool{false, true} {
			for _, action := range []string{"retain_as_context", "remove_if_isolated"} {
				t.Run(fmt.Sprintf("%s/business=%t/%s", shape, businessIDs, action), func(t *testing.T) {
					bus, accepted, staged, recipe := b1647StageOrphan(t, shape)
					if businessIDs {
						recipe.FromNode, recipe.ToNode = "n3", "n4"
					}
					bus.Mutable.SetFinalizerTypedRelationRecipeAnchors([]types.DiagramEdgeAnchor{recipe})
					lease := bus.Mutable.AnswerDiagramRelationRepairLease()
					beforeLease, _ := json.Marshal(lease)
					beforeStage, _ := json.Marshal(staged)
					schema := string((&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: bus.Mutable, EvidenceItems: bus.EvidenceItems}))
					if !strings.Contains(schema, `"diagram_participant_edits"`) || !strings.Contains(schema, `"`+action+`"`) || strings.Contains(schema, `"diagram_edge_edits"`) {
						t.Fatalf("exact orphan-only public capability missing: %s", schema)
					}
					edit := map[string]any{"block_id": "diag", "participant_id": "D", "action": action}
					if action == "retain_as_context" {
						edit["visible_label"] = "OldCallee (context only)"
					}
					raw, _ := json.Marshal(map[string]any{"diagram_participant_edits": []any{edit}})
					result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
					if err != nil || !result.Success {
						t.Fatalf("current orphan choice must not be blamed for system-only inherited identity completion: err=%v summary=%s", err, result.Summary)
					}
					got := bus.Mutable.AnswerDocumentV2()
					if got == nil || !reflect.DeepEqual(got.Blocks[0], accepted.Blocks[0]) || len(got.Citations) < len(accepted.Citations) || !reflect.DeepEqual(got.Citations[:len(accepted.Citations)], accepted.Citations) {
						t.Fatalf("unselected model content/citations changed: %+v", got)
					}
					if b1647MessageLines(got.Blocks[1].Diagram.Body) != b1647MessageLines(staged.Blocks[1].Diagram.Body) || len(got.Blocks[1].EdgeAnchors) != len(staged.Blocks[1].EdgeAnchors) {
						t.Fatal("identity comparison must preserve every model message and anchor occurrence")
					}
					for i, anchor := range got.Blocks[1].EdgeAnchors {
						if anchor.HasEndpointIdentityPair() && (anchor.FromIdentity != "Caller" || anchor.ToIdentity != "Callee") {
							t.Fatalf("non-receipted identity invented: %+v", anchor)
						}
						want := staged.Blocks[1].EdgeAnchors[i]
						anchor.FromIdentity, anchor.ToIdentity = want.FromIdentity, want.ToIdentity
						if bus.AnalysisIR != nil {
							want.ClaimForm = types.ClaimCallEdge
						}
						if anchor != want {
							t.Fatalf("non-identity metadata changed: got=%+v want=%+v", anchor, want)
						}
					}
					if action == "remove_if_isolated" && strings.Contains(got.Blocks[1].Diagram.Body, "participant D") {
						t.Fatal("selected orphan removal was lost")
					}
					if action == "retain_as_context" && !strings.Contains(got.Blocks[1].Diagram.Body, "OldCallee (context only)") {
						t.Fatal("selected context label was lost")
					}
					afterLease, _ := json.Marshal(lease)
					afterStage, _ := json.Marshal(staged)
					if string(beforeLease) != string(afterLease) || string(beforeStage) != string(afterStage) || bus.Mutable.AnswerDiagramRelationRepairLease() != nil || bus.Mutable.PendingAnswerDocumentPatchBase() != nil {
						t.Fatal("private receipts changed immutable inputs or failed to consume the accepted retry generation")
					}
				})
			}
		}
	}
}

func TestB1647cIdentityReceiptPreservesExactOccurrences(t *testing.T) {
	for _, shape := range []string{"empty", "from_only", "to_only", "qualified", "mixed_occurrences"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", shape, reverse), func(t *testing.T) {
				bus, _, doc, recipe := b1647StageOrphan(t, "legacy_only")
				lease := bus.Mutable.AnswerDiagramRelationRepairLease()
				anchor := &doc.Blocks[1].EdgeAnchors[0]
				wantFixed := 1
				switch shape {
				case "from_only":
					anchor.FromIdentity = "Caller"
				case "to_only":
					anchor.ToIdentity = "Callee"
				case "qualified":
					anchor.FromIdentity, anchor.ToIdentity = "Caller (source)", "Callee (source)"
				case "mixed_occurrences":
					anchor.VisibleLabel = "first model message"
					second := *anchor
					second.FromIdentity, second.ToIdentity = "Caller", "Callee"
					second.VisibleLabel, second.ClaimForm = "second model message", types.ClaimCallEdge
					doc.Blocks[1].EdgeAnchors = append(doc.Blocks[1].EdgeAnchors, second, *anchor)
					doc.Blocks[1].Diagram.Body += " A->>B: second model message\n A->>B: first model message\n"
					wantFixed = 2
				}
				if reverse {
					anchors := doc.Blocks[1].EdgeAnchors
					for i, j := 0, len(anchors)-1; i < j; i, j = i+1, j-1 {
						anchors[i], anchors[j] = anchors[j], anchors[i]
					}
				}
				// This white-box positive models an exact historical partial or
				// qualified baseline; unlike the old partial_baseline negative,
				// both the source snapshot and immutable lease own the same tuple.
				lease.Blocks[0].BaseAnchors = append([]types.DiagramEdgeAnchor(nil), doc.Blocks[1].EdgeAnchors...)
				original, _ := json.Marshal(doc)
				leaseJSON, _ := json.Marshal(lease)
				before := snapshotDiagramAnchorIdentities(doc)
				if n := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, []types.DiagramEdgeAnchor{recipe}); n != wantFixed {
					t.Fatalf("real normalizer did not exercise %s: n=%d anchors=%+v", shape, n, doc.Blocks[1].EdgeAnchors)
				}
				receipt := recordDiagramAnchorIdentityRepair(before, doc)
				if n := stabilizeUnlistedRelationLeaseAnchorIdentities(doc, lease, receipt); n != wantFixed {
					t.Fatalf("only this invocation's exact occurrences may be restored: n=%d want=%d", n, wantFixed)
				}
				after, _ := json.Marshal(doc)
				if string(original) != string(after) {
					t.Fatal("identity receipt changed model bytes or moved identity between occurrence labels/claims")
				}
				if issues := types.ValidateAnswerDiagramRelationRepairLease(lease, doc); len(issues) != 0 {
					t.Fatalf("exact restored lease comparison must remain valid: %+v", issues)
				}
				afterLease, _ := json.Marshal(lease)
				if string(leaseJSON) != string(afterLease) {
					t.Fatal("comparison modified the immutable lease")
				}
			})
		}
	}
}

func TestB1647cIdentityReceiptCannotHideModelOrReceiptDrift(t *testing.T) {
	for _, change := range []string{"missing_receipt", "after_identity_drift", "before_label_drift", "after_claim_drift", "different_block", "duplicate_receipt_occurrence", "model_prechanged_identity"} {
		t.Run(change, func(t *testing.T) {
			bus, _, doc, recipe := b1647StageOrphan(t, "legacy_only")
			lease := bus.Mutable.AnswerDiagramRelationRepairLease()
			if change == "model_prechanged_identity" {
				doc.Blocks[1].EdgeAnchors[0].FromIdentity = "OtherCaller"
			}
			before := snapshotDiagramAnchorIdentities(doc)
			normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, []types.DiagramEdgeAnchor{recipe})
			receipt := recordDiagramAnchorIdentityRepair(before, doc)
			switch change {
			case "missing_receipt":
				receipt = diagramAnchorIdentityRepairReceipt{}
			case "after_identity_drift":
				doc.Blocks[1].EdgeAnchors[0].ToIdentity = "OtherCallee"
			case "before_label_drift":
				receipt.before["diag"][0].VisibleLabel = "different occurrence"
			case "after_claim_drift":
				receipt.after["diag"][0].ClaimForm = types.ClaimCallEdge
			case "different_block":
				receipt.before["other"] = receipt.before["diag"]
				delete(receipt.before, "diag")
			case "duplicate_receipt_occurrence":
				receipt.before["diag"] = append(receipt.before["diag"], receipt.before["diag"][0])
			}
			raw, _ := json.Marshal(doc)
			if n := stabilizeUnlistedRelationLeaseAnchorIdentities(doc, lease, receipt); n != 0 {
				t.Fatalf("missing/stale/nonmatching receipt must not neutralize a model change: n=%d", n)
			}
			after, _ := json.Marshal(doc)
			if string(raw) != string(after) || len(types.ValidateAnswerDiagramRelationRepairLease(lease, doc)) == 0 {
				t.Fatal("original scope rejection or exact candidate bytes changed")
			}
		})
	}
}

func TestB1647cPublicOrphanReceiptDoesNotGrantCallEvidence(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing_evidence=%t", missing), func(t *testing.T) {
			bus, accepted, _, recipe := b1647StageOrphan(t, "legacy_only_with_evidence")
			recipe.FromNode, recipe.ToNode = "n3", "n4"
			if missing {
				bus.EvidenceItems = nil
			} else {
				recipe.ToIdentity = "UnprovenMethod"
			}
			bus.Mutable.SetFinalizerTypedRelationRecipeAnchors([]types.DiagramEdgeAnchor{recipe})
			raw := json.RawMessage(`{"diagram_participant_edits":[{"action":"remove_if_isolated","block_id":"diag","participant_id":"D"}]}`)
			result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
			if err != nil || result.Success {
				t.Fatalf("a normalization receipt is not executable-call evidence: err=%v result=%+v", err, result)
			}
			wantIssue := "edge_anchor_node_identity_conflict"
			if missing {
				wantIssue = "call_edge_unproven"
			}
			if strings.Contains(result.Summary, "unlisted_relation_") || !strings.Contains(result.Summary, wantIssue) {
				t.Fatalf("orphan scope should pass, then the unchanged ordinary call gate must refuse: %s", result.Summary)
			}
			if !reflect.DeepEqual(accepted, bus.Mutable.AnswerDocumentV2()) {
				t.Fatal("unproved normalized relation was published")
			}
		})
	}
}
