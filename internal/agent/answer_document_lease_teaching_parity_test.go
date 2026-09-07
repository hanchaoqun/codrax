package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This exercises the real producer/evaluator/executor boundary. A grounded
// visible edge without metadata must not be taught whole-block replacement
// while the same dispatch has installed an atomic-only relation lease.
func TestB1609GroundedAnchorTeachingFollowsExecutableLease(t *testing.T) {
	for _, required := range []bool{false, true} {
		for _, patchReject := range []bool{false, true} {
			name := "optional/full"
			if required {
				name = "required/full"
			}
			if patchReject {
				name = strings.TrimSuffix(name, "full") + "patch"
			}
			t.Run(name, func(t *testing.T) {
				bus, ctx, rejected := b1609GroundedAnchorFixture(t, patchReject)
				e := &answerDocumentEvaluator{diagramRequired: required}
				var signal LoopSignal
				if patchReject {
					signal = e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: &rejected})
				} else {
					signal = e.emitAnswerDocumentRejectSignal(ctx, LoopObservation{LastToolResult: &rejected})
				}
				lease := bus.Mutable.AnswerDiagramRelationRepairLease()
				if lease == nil || len(lease.Failures) != 1 {
					t.Fatalf("real reject must install exactly one executable failure: %+v", lease)
				}
				if !signal.HintRequested || !strings.Contains(signal.Hint, "diagram_edge_edits") ||
					strings.Contains(signal.Hint, "replace only the listed diagram block(s)") ||
					strings.Contains(signal.Hint, "Set each listed block's `edge_anchors`") {
					t.Errorf("live atomic lease conflicts with retry teaching: key=%s\n%s", signal.HintKey, signal.Hint)
				}
				for _, want := range []string{"only their anchor metadata is missing", "Prefer keeping every such edge", "schema-published attach pair", "permitted replace with the same visible content"} {
					if !strings.Contains(signal.Hint, want) {
						t.Errorf("grounded-only soft preservation guidance lost %q: %s", want, signal.Hint)
					}
				}
				failure := lease.Failures[0]
				if !failure.AllowsAction("replace") || !strings.Contains(signal.Hint, failure.FailureRef) {
					t.Fatalf("grounded repair must publish the exact executable replacement ref: %+v\n%s", failure, signal.Hint)
				}
				b1609AssertPublishedAtomicBranch(t, ctx, failure.FailureRef)

				// A whole replacement is still forbidden; fixing teaching must not
				// relax the executor or publish even a semantically identical block.
				base := bus.Mutable.LastRejectedAnswerDocumentV2()
				if pending := bus.Mutable.PendingAnswerDocumentPatchBase(); pending != nil {
					base = pending
				}
				if base == nil {
					t.Fatal("rejected model draft must remain the retry base")
				}
				whole, _ := json.Marshal(map[string]any{"replace_blocks": []types.AnswerBlock{base.Blocks[1]}, "unchanged_block_ids": []string{"summary"}})
				blocked, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, whole)
				if err != nil || blocked.Success || !strings.Contains(blocked.Summary, "whole_replace_not_authorized") {
					t.Fatalf("atomic lease must still reject a whole-block replacement: err=%v result=%+v", err, blocked)
				}

				// The model chooses the existing edge and retains its exact visible
				// wording. Only the schema-published ref supplies hidden metadata.
				params, _ := json.Marshal(map[string]any{"diagram_edge_edits": []any{map[string]any{
					"failure_ref": failure.FailureRef, "action": "replace",
					"edge": map[string]any{"from_node": "B", "to_node": "C", "visible_label": "model second"},
				}}, "unchanged_block_ids": []string{"summary"}})
				accepted, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, params)
				if err != nil || !accepted.Success {
					t.Fatalf("exact published atomic repair must execute: err=%v result=%+v", err, accepted)
				}
				got := bus.Mutable.AnswerDocumentV2()
				if got == nil || len(got.Blocks) != len(base.Blocks) || !reflect.DeepEqual(got.Blocks[0], base.Blocks[0]) ||
					got.Blocks[1].Diagram.Body != base.Blocks[1].Diagram.Body || len(got.Blocks[1].EdgeAnchors) != 2 {
					t.Fatalf("metadata repair changed model prose, topology, or unrelated carriers: got=%+v base=%+v", got, base)
				}
				if !reflect.DeepEqual(got.Blocks[1].EdgeAnchors[0], base.Blocks[1].EdgeAnchors[0]) ||
					!reflect.DeepEqual(got.Blocks[2:], base.Blocks[2:]) || !reflect.DeepEqual(got.Citations, base.Citations) {
					t.Fatal("atomic metadata repair changed an unlisted anchor, sibling, or citation")
				}
			})
		}
	}
}

func TestB1609GroundedOnlyPreservationAdviceDoesNotClaimMixedFailuresAreGrounded(t *testing.T) {
	_, _, rejected := b1609GroundedAnchorFixture(t, false)
	rejected.Repair.Metadata[types.ToolRepairMetaDiagramRelationFailureIssues] += ",call_edge_unproven"
	hint, ok := answerDocDiagramRelationDeltaPatchHint(&rejected, false, false)
	if !ok || strings.Contains(hint, "The listed edges are already grounded") {
		t.Fatalf("mixed failure roster must not acquire metadata-only preservation advice: ok=%v hint=%s", ok, hint)
	}
}

func b1609AssertPublishedAtomicBranch(t *testing.T, ctx *types.AgentContext, failureRef string) {
	t.Helper()
	for _, schema := range finalizerSchemaTestAgent().buildToolSchemas(finalizerSchemaTestSkill(), ctx) {
		if schema.Name != "emit_answer_document_patch" {
			continue
		}
		var root map[string]any
		if err := json.Unmarshal(schema.Parameters, &root); err != nil {
			t.Fatal(err)
		}
		props := root["properties"].(map[string]any)
		if replacement, ok := props["replace_blocks"].(map[string]any); ok {
			items := replacement["items"].(map[string]any)["properties"].(map[string]any)
			for _, id := range items["id"].(map[string]any)["enum"].([]any) {
				if id == "sequence" {
					t.Fatal("live schema widened the leased diagram into whole replacement")
				}
			}
		}
		branches := props["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
		for _, raw := range branches {
			branch := raw.(map[string]any)["properties"].(map[string]any)
			ref, ok := branch["failure_ref"].(map[string]any)
			if !ok || !reflect.DeepEqual(ref["enum"], []any{failureRef}) ||
				!reflect.DeepEqual(branch["action"].(map[string]any)["enum"], []any{"replace"}) {
				continue
			}
			edge := branch["edge"].(map[string]any)
			if edge["type"] != "object" {
				t.Fatal("replacement edge must be a native nested object")
			}
			for _, field := range []string{"from_node", "to_node", "visible_label"} {
				if _, ok := edge["properties"].(map[string]any)[field]; !ok {
					t.Fatalf("current replacement schema lost edge.%s", field)
				}
			}
			return
		}
		t.Fatal("current dispatch schema did not publish the ref/action taught by the hint")
	}
	t.Fatal("current dispatch omitted the patch schema")
}

func TestB1609GroundedAnchorTeachingRetainsLegacyNoLeaseReplacement(t *testing.T) {
	for _, patchReject := range []bool{false, true} {
		name := "full"
		if patchReject {
			name = "patch"
		}
		t.Run(name, func(t *testing.T) {
			bus, ctx, rejected := b1609GroundedAnchorFixture(t, patchReject)
			// Older producers have the validated complete anchor array but no
			// executable delta. Do not invent a lease or retire their legal path.
			delete(rejected.Repair.Metadata, types.ToolRepairMetaDiagramRelationRepairDeltaJSON)
			e := &answerDocumentEvaluator{}
			var signal LoopSignal
			if patchReject {
				signal = e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: &rejected})
			} else {
				signal = e.emitAnswerDocumentRejectSignal(ctx, LoopObservation{LastToolResult: &rejected})
			}
			if bus.Mutable.AnswerDiagramRelationRepairLease() != nil || !signal.HintRequested ||
				!strings.Contains(signal.Hint, "replace only the listed diagram block(s)") {
				t.Fatalf("no-lease metadata recovery lost its legal legacy branch: %+v", signal)
			}
			base := bus.Mutable.LastRejectedAnswerDocumentV2()
			if pending := bus.Mutable.PendingAnswerDocumentPatchBase(); pending != nil {
				base = pending
			}
			var rows []answerDocGroundedAnchorPatchRow
			if err := json.Unmarshal([]byte(rejected.Repair.Metadata[types.ToolRepairMetaDiagramGroundedAnchorPatchJSON]), &rows); err != nil || len(rows) != 1 {
				t.Fatalf("real metadata patch array missing: err=%v rows=%+v", err, rows)
			}
			replacement := base.Blocks[1]
			replacement.EdgeAnchors = rows[0].EdgeAnchors
			params, _ := json.Marshal(map[string]any{"replace_blocks": []types.AnswerBlock{replacement}, "unchanged_block_ids": []string{"summary"}})
			result, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, params)
			if err != nil || !result.Success {
				t.Fatalf("legacy complete-anchor replacement must remain executable without a lease: err=%v result=%+v", err, result)
			}
			got := bus.Mutable.AnswerDocumentV2()
			if got == nil || got.Blocks[1].Diagram.Body != base.Blocks[1].Diagram.Body ||
				!reflect.DeepEqual(got.Blocks[0], base.Blocks[0]) || len(got.Blocks[1].EdgeAnchors) != 2 {
				t.Fatal("legacy metadata repair changed model content")
			}
		})
	}
}

func b1609GroundedAnchorFixture(t *testing.T, patchReject bool) (*types.BusContext, *types.AgentContext, types.ToolResult) {
	t.Helper()
	mut := types.NewMutableState("explain the selected operations")
	evidence := []types.EvidenceItem{
		{ID: "edge-ab", Kind: types.EvidenceRelationship, Subject: "Alpha.Run", Predicate: "calls", Object: "Beta.Run", Source: "sample.go", LineStart: 10, AnchorKind: types.AnchorCall, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded, Producer: types.EvidenceProducerExplorerEmitEvidence},
		{ID: "edge-bc", Kind: types.EvidenceRelationship, Subject: "Beta.Run", Predicate: "calls", Object: "Gamma.Run", Source: "sample.go", LineStart: 20, AnchorKind: types.AnchorCall, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded, Producer: types.EvidenceProducerExplorerEmitEvidence},
	}
	mut.AppendEvidence(evidence)
	ir := &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain, PredicateAxis: types.AxisCall}}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir, EvidenceItems: evidence}
	ctx := &types.AgentContext{Mutable: mut, AnalysisIR: ir, EvidenceItems: evidence}
	raw := json.RawMessage(`{"blocks":[{"id":"summary","kind":"summary","text":"model explanation"},{"id":"sequence","kind":"diagram","diagram":{"kind":"sequence","language":"mermaid","body":"sequenceDiagram\n  participant A as Alpha.Run\n  participant B as Beta.Run\n  participant C as Gamma.Run\n  A->>B: model first\n  B->>C: model second\n"},"edge_anchors":[{"from_node":"A","to_node":"B","from_identity":"Alpha.Run","to_identity":"Beta.Run","relation_kind":"call","visible_label":"model first"}]}]}`)
	result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || result.Success || !answerDocumentRejectOnlyGroundedMissingCallAnchors(&result) {
		t.Fatalf("real full emitter must publish grounded metadata-only repair: err=%v result=%+v", err, result)
	}
	if patchReject {
		result, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"unchanged_block_ids":["summary","sequence"]}`))
		if err != nil || result.Success || !answerDocumentRejectOnlyGroundedMissingCallAnchors(&result) {
			t.Fatalf("real patch emitter must preserve the same metadata-only defect: err=%v result=%+v", err, result)
		}
	}
	return bus, ctx, result
}
