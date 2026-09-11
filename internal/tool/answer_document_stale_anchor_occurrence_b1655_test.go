package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Reduction of r1056's actual two model-authored Dispatch -> Bus anchors.
// Its later accepted relation edits removed the visible messages, not these
// two metadata rows. Keep a separate actually-grounded edge so cleanup cannot
// succeed by deleting or redrawing the entire diagram.
func b1655StaleBase(t *testing.T, mode string) (*types.BusContext, *types.AnswerDocumentV2) {
	t.Helper()
	original := []types.DiagramEdgeAnchor{
		{FromNode: "Dispatch", ToNode: "Bus", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge, VisibleLabel: "AgentAnalyzer 执行"},
		{FromNode: "Dispatch", ToNode: "Bus", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge, VisibleLabel: "AgentFinalizer 执行"},
	}
	switch mode {
	case "single":
		original = original[:1]
	case "identical":
		original[1] = original[0]
	case "reverse":
		original[0], original[1] = original[1], original[0]
	case "distinct_identities":
		original[0].FromIdentity, original[0].ToIdentity = "dispatchStage.analyze", "BusContext.get"
		original[1].FromIdentity, original[1].ToIdentity = "dispatchStage.finalize", "BusContext.set"
	}
	bus, _ := b1649ActualCall(t)
	// A second real source call supplies an unanchored sibling addition, as in
	// r1056. Reusing the already anchored A -> B call is correctly suppressed.
	if err := os.WriteFile(filepath.Join(bus.RepoRoot, "other.go"), []byte("package p\nfunc OtherCaller() {\n Callee()\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	read, err := (&ReadFile{}).Execute(bus, json.RawMessage(`{"path":"other.go","offset":0,"limit":10}`))
	if err != nil || !read.Success {
		t.Fatalf("read sibling: %v %+v", err, read)
	}
	bus.ToolResults = append(bus.ToolResults, read)
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: bus.ToolResults})
	emitted, err := (&EmitEvidence{}).Execute(bus, json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"mechanism","subject":"p.OtherCaller","predicate":"calls","object":"Callee","source":"other.go","line_start":3,"anchor_kind":"call","anchor_symbol":"Callee"}]}`))
	if err != nil || !emitted.Success {
		t.Fatalf("emit sibling: %v %+v", err, emitted)
	}
	for _, ev := range bus.Mutable.EmittedEvidence() {
		if ev.Subject == "p.OtherCaller" && ev.Source == "other.go" && types.ClaimFormOf(ev) == types.ClaimCallEdge && ev.IsCitable() && ev.GroundingStatus == types.GroundingGrounded {
			bus.EvidenceItems = append(bus.EvidenceItems, ev)
		}
	}
	if len(bus.EvidenceItems) != 2 {
		t.Fatalf("real unanchored sibling premise missing: %+v", bus.EvidenceItems)
	}
	doc := b1649Diagram("p.Caller", "Callee")
	doc.Blocks[1].Diagram.Body += "    participant Dispatch as dispatchStage\n    participant Bus as BusContext\n"
	doc.Blocks[1].EdgeAnchors = append([]types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", FromIdentity: "p.Caller", ToIdentity: "Callee", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge, VisibleLabel: "invoke callee"}}, original...)
	return bus, doc
}

func TestB1655StaleDuplicateAnchorPublicRepair(t *testing.T) {
	for _, mode := range []string{"single", "actual_two_labels", "identical", "reverse", "reverse_edits", "distinct_identities"} {
		t.Run(mode, func(t *testing.T) {
			bus, doc := b1655StaleBase(t, mode)
			body := doc.Blocks[1].Diagram.Body
			raw, _ := json.Marshal(doc)
			result, err := (&EmitAnswerDocument{}).Execute(bus, raw)
			if err != nil || result.Success || result.Repair == nil {
				t.Fatalf("public emit must report the real stale metadata: err=%v result=%+v", err, result)
			}
			var delta types.AnswerDiagramRelationRepairDelta
			if err := json.Unmarshal([]byte(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
				t.Fatalf("public delta missing: %v %+v", err, result.Repair)
			}
			var failures []types.AnswerDiagramRelationRepairFailure
			for _, f := range delta.Failures {
				if f.Issue == "typed_anchor_without_visible_edge" && f.FromNode == "Dispatch" && f.ToNode == "Bus" {
					failures = append(failures, f)
				}
			}
			if len(failures) != len(doc.Blocks[1].EdgeAnchors)-1 {
				t.Fatalf("stale Dispatch -> Bus not in actual failure delta: %+v", delta)
			}
			// r1056 also has unrelated current additions, which keep this block's
			// lease active despite its non-executable stale-anchor row. The
			// constructor accepts a typed addition roster; use the fixture's
			// actually grounded call, not a fabricated source authorization.
			// It is never selected or executed by this test.
			ev := bus.EvidenceItems[1]
			additions := append([]types.AnswerDiagramRelationRepairCandidate(nil), delta.AllowedAdditions...)
			additions = append(additions, types.AnswerDiagramRelationRepairCandidate{
				BlockID: "diagram", RelationKind: types.DiagramRelCall,
				FromIdentity: ev.Subject, ToIdentity: ev.Object,
				EvidenceID: ev.ID, Source: fmt.Sprintf("%s:%d", ev.Source, ev.LineStart),
				FromNodeIDs: []string{"Other"}, ToNodeIDs: []string{"B"},
			})
			lease := types.NewAnswerDiagramRelationRepairLease(doc, delta.Failures, additions)
			if lease == nil {
				t.Fatal("public lease unexpectedly nil")
			}
			var failed types.AnswerDiagramRelationRepairFailure
			for _, f := range lease.Failures {
				if f.Issue == "typed_anchor_without_visible_edge" && f.FromNode == "Dispatch" && f.ToNode == "Bus" {
					failed = f
					break
				}
			}
			if failed.FailureRef == "" {
				t.Fatalf("no live ref: %+v", lease)
			}
			bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
			bus.Mutable.SetAnswerDiagramRelationRepairLease(lease)
			var edits []map[string]string
			for _, f := range lease.Failures {
				if f.Issue == "typed_anchor_without_visible_edge" && f.FromNode == "Dispatch" && f.ToNode == "Bus" {
					edits = append(edits, map[string]string{"action": "remove", "failure_ref": f.FailureRef})
				}
			}
			if mode == "reverse_edits" {
				for i, j := 0, len(edits)-1; i < j; i, j = i+1, j-1 {
					edits[i], edits[j] = edits[j], edits[i]
				}
			}
			params, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": edits})
			patched, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
			t.Logf("anchors=%d compiled_failures=%d carrier=%s actions=%v locally_executable=%t actual_patch_success=%t summary=%s", len(doc.Blocks[1].EdgeAnchors)-1, len(lease.Failures), failed.TargetCarrier, failed.AllowedActions, types.AnswerDiagramRelationRepairLeaseIsLocallyExecutable(lease), patched.Success, patched.Summary)
			if err != nil || !patched.Success {
				t.Errorf("published stale-anchor removal must be executable without adding a visible relation: err=%v result=%+v", err, patched)
				return
			}
			got := bus.Mutable.AnswerDocumentV2()
			if got == nil || got.Blocks[1].Diagram.Body != body || !reflect.DeepEqual(got.Blocks[0], doc.Blocks[0]) || len(got.Blocks[1].EdgeAnchors) >= len(doc.Blocks[1].EdgeAnchors) || !reflect.DeepEqual(got.Blocks[1].EdgeAnchors[0], doc.Blocks[1].EdgeAnchors[0]) {
				t.Fatalf("cleanup changed body/unrelated graph or failed to remove metadata: %+v", got)
			}
		})
	}
}

func TestB1655StaleRepairDoesNotAuthorizeWrongRefOrNewBody(t *testing.T) {
	for _, kind := range []string{"wrong_ref", "new_body"} {
		t.Run(kind, func(t *testing.T) {
			bus, doc := b1655StaleBase(t, "single")
			failure := types.AnswerDiagramRelationRepairFailure{BlockID: "diagram", Issue: "typed_anchor_without_visible_edge", FromNode: "Dispatch", ToNode: "Bus", FromIdentity: "dispatchStage", ToIdentity: "BusContext", RelationKind: types.DiagramRelCall}
			lease := types.NewAnswerDiagramRelationRepairLease(doc, []types.AnswerDiagramRelationRepairFailure{failure}, nil)
			bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
			bus.Mutable.SetAnswerDiagramRelationRepairLease(lease)
			params := json.RawMessage(`{"unchanged_block_ids":["summary"],"diagram_edge_edits":[{"action":"remove","failure_ref":"rf1-not-in-live-lease"}]}`)
			if kind == "new_body" {
				params = json.RawMessage(fmt.Sprintf(`{"unchanged_block_ids":["summary"],"diagram_edge_edits":[{"action":"replace","failure_ref":%q,"edge":{"from_node":"Dispatch","to_node":"Bus","visible_label":"invented invocation"}}]}`, lease.Failures[0].FailureRef))
			}
			result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
			if err != nil || result.Success {
				t.Fatalf("unsupported edit escaped the existing relation checks: err=%v result=%+v", err, result)
			}
			if got := bus.Mutable.AnswerDocumentV2(); got == nil || got.Blocks[1].Diagram.Body != doc.Blocks[1].Diagram.Body {
				t.Fatalf("unsupported edit published changed body: %+v", got)
			}
		})
	}
}

func b1655InstallActualLease(t *testing.T, bus *types.BusContext, doc *types.AnswerDocumentV2) *types.AnswerDiagramRelationRepairLease {
	t.Helper()
	raw, _ := json.Marshal(doc)
	r, err := (&EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || r.Success || r.Repair == nil {
		t.Fatalf("actual stale emit: %v %+v", err, r)
	}
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(r.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatal(err)
	}
	lease := types.NewAnswerDiagramRelationRepairLease(doc, delta.Failures, delta.AllowedAdditions)
	if lease == nil {
		t.Fatalf("no local lease: %+v", delta)
	}
	// All following public Patch executions consume the JSON-restored lease,
	// not the constructor object. This verifies the new selector's wire path.
	wire, err := json.Marshal(lease)
	if err != nil {
		t.Fatal(err)
	}
	var restored types.AnswerDiagramRelationRepairLease
	if err := json.Unmarshal(wire, &restored); err != nil || !reflect.DeepEqual(lease.Failures, restored.Failures) || !reflect.DeepEqual(lease.Blocks, restored.Blocks) {
		t.Fatalf("lease selector wire loss: %v", err)
	}
	lease = &restored
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	bus.Mutable.SetAnswerDiagramRelationRepairLease(lease)
	return lease
}

func TestB1655SelectedMetadataOnlyAndNextGeneration(t *testing.T) {
	for _, mode := range []string{"actual_two_labels", "identical", "reverse", "distinct_identities"} {
		for _, selected := range []int{2, 3} {
			t.Run(fmt.Sprintf("%s/%d", mode, selected), func(t *testing.T) {
				bus, doc := b1655StaleBase(t, mode)
				before, _ := json.Marshal(doc)
				lease := b1655InstallActualLease(t, bus, doc)
				var chosen types.AnswerDiagramRelationRepairFailure
				for _, f := range lease.Failures {
					if f.AnchorOccurrence == selected {
						chosen = f
					}
				}
				if chosen.FailureRef == "" {
					t.Fatalf("missing occurrence %d: %+v", selected, lease)
				}
				params := json.RawMessage(fmt.Sprintf(`{"unchanged_block_ids":["summary"],"diagram_edge_edits":[{"action":"remove","failure_ref":%q}]}`, chosen.FailureRef))
				r, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
				if err != nil || r.Success || r.Repair == nil || r.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeStagedForRetry {
					t.Fatalf("one remaining failure should stage exact edit: %v %+v", err, r)
				}
				staged := bus.Mutable.PendingAnswerDocumentPatchBase()
				var expected types.AnswerDocumentV2
				_ = json.Unmarshal(before, &expected)
				a := expected.Blocks[1].EdgeAnchors
				expected.Blocks[1].EdgeAnchors = append(a[:selected-1], a[selected:]...)
				// Existing evidence supplement publication is independent of this
				// cleanup. Pin every original model block, not absence of that
				// pre-existing system-owned supplement/citation behavior.
				if staged == nil || len(staged.Blocks) < 2 || !reflect.DeepEqual(staged.Blocks[:2], expected.Blocks) {
					t.Fatalf("selected-only removal changed original model blocks")
				}
				// The next lease is compiled from the actual staged rejected draft.
				// Reusing its predecessor's ref must not remove the shifted sibling.
				lease2 := b1655InstallActualLease(t, bus, staged)
				old, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
				if err != nil || old.Success || old.Repair == nil || old.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] == types.AnswerDocumentPatchOutcomeStagedForRetry {
					t.Fatalf("old generation reused: %v %+v", err, old)
				}
				if len(lease2.Failures) != 1 {
					t.Fatalf("next failures: %+v", lease2.Failures)
				}
				params = json.RawMessage(fmt.Sprintf(`{"unchanged_block_ids":["summary"],"diagram_edge_edits":[{"action":"remove","failure_ref":%q}]}`, lease2.Failures[0].FailureRef))
				final, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
				if err != nil || !final.Success {
					t.Fatalf("next generation removal failed: %v %+v", err, final)
				}
				got := bus.Mutable.AnswerDocumentV2()
				if len(got.Blocks[1].EdgeAnchors) != 1 || got.Blocks[1].Diagram.Body != doc.Blocks[1].Diagram.Body {
					t.Fatal("final cleanup changed body or unrelated relation")
				}
			})
		}
	}
}

func TestB1655OccurrencePublicRefGuards(t *testing.T) {
	for _, kind := range []string{"duplicate_ref", "wrong_block", "changed_label", "reordered", "changed_body", "replace_multi", "legacy_coordinate"} {
		t.Run(kind, func(t *testing.T) {
			bus, doc := b1655StaleBase(t, "actual_two_labels")
			lease := b1655InstallActualLease(t, bus, doc)
			ref := lease.Failures[0].FailureRef
			edits := []map[string]any{{"action": "remove", "failure_ref": ref}}
			switch kind {
			case "duplicate_ref":
				edits = append(edits, edits[0])
			case "wrong_block":
				edits[0]["block_id"] = "summary"
			case "changed_label":
				doc.Blocks[1].EdgeAnchors[1].VisibleLabel = "changed after failure publication"
			case "reordered":
				doc.Blocks[1].EdgeAnchors[1], doc.Blocks[1].EdgeAnchors[2] = doc.Blocks[1].EdgeAnchors[2], doc.Blocks[1].EdgeAnchors[1]
			case "changed_body":
				doc.Blocks[1].Diagram.Body += "Dispatch->>Bus: actual new body edge\n"
			case "replace_multi":
				edits[0]["action"] = "replace"
				edits[0]["edge"] = map[string]string{"from_node": "Dispatch", "to_node": "Bus", "visible_label": "invented invocation"}
			case "legacy_coordinate":
				edits = []map[string]any{{"action": "remove", "block_id": "diagram", "match": map[string]string{"from_node": "Dispatch", "to_node": "Bus", "relation_kind": "call", "from_identity": "dispatchStage", "to_identity": "BusContext"}}}
			}
			if strings.HasPrefix(kind, "changed_") || kind == "reordered" {
				bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
				bus.Mutable.SetAnswerDiagramRelationRepairLease(lease)
			}
			before, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			params, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": edits})
			r, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
			if err != nil || r.Success || r.Repair == nil || r.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] == types.AnswerDocumentPatchOutcomeStagedForRetry {
				t.Fatalf("guard %s escaped: %v %+v", kind, err, r)
			}
			after, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			if string(before) != string(after) {
				t.Fatal("rejected edit changed published document")
			}
		})
	}
}

func TestB1655ReversedVisibleEdgeMetadataOccurrences(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			bus, doc := b1655StaleBase(t, "actual_two_labels")
			// This is a genuinely reversed visible edge, not an array reorder.
			// Its missing evidence remains a separate failure after cleanup.
			doc.Blocks[1].Diagram.Body += "    Bus->>Dispatch: model-authored reverse message\n"
			lease := b1655InstallActualLease(t, bus, doc)
			var edits []map[string]string
			for _, f := range lease.Failures {
				if f.Issue == "typed_anchor_reversed_against_visible_edge" {
					if f.AnchorOccurrence < 2 || !f.AllowsAction("remove") || f.AllowsAction("replace") {
						t.Fatalf("reverse capability: %+v", f)
					}
					edits = append(edits, map[string]string{"action": "remove", "failure_ref": f.FailureRef})
				}
			}
			if len(edits) != 2 {
				t.Fatalf("reverse occurrences collapsed: %+v", lease)
			}
			if reverse {
				edits[0], edits[1] = edits[1], edits[0]
			}
			raw, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": edits})
			r, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
			if err != nil || r.Success || r.Repair == nil || r.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeStagedForRetry {
				t.Fatalf("reverse edge must remain unproven after metadata cleanup: %v %+v", err, r)
			}
			got := bus.Mutable.PendingAnswerDocumentPatchBase()
			if got == nil || len(got.Blocks[1].EdgeAnchors) != 1 || !reflect.DeepEqual(got.Blocks[0], doc.Blocks[0]) || got.Blocks[1].Diagram.Body != doc.Blocks[1].Diagram.Body || got.Blocks[1].EdgeAnchors[0] != doc.Blocks[1].EdgeAnchors[0] {
				t.Fatal("reverse cleanup altered visible edge/model or unrelated anchor")
			}
		})
	}
}

func TestB1655MetadataOccurrenceIsNotModelInput(t *testing.T) {
	var edit emitAnswerDiagramEdgeEdit
	if err := json.Unmarshal([]byte(`{"anchor_occurrence":99,"anchorBaseOccurrence":99,"action":"remove"}`), &edit); err != nil {
		t.Fatal(err)
	}
	if edit.anchorBaseOccurrence != 0 {
		t.Fatal("model input populated private metadata selector")
	}
	var anchor types.DiagramEdgeAnchor
	if _, ok := reflect.TypeOf(anchor).FieldByName("AnchorOccurrence"); ok {
		t.Fatal("occurrence leaked into model edge_anchors")
	}
}
