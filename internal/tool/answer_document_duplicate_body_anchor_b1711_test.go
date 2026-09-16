package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Restore a synthetic version-1 checkpoint fixture containing historical
// occurrence failures. This does not call today's producer or endorse the old
// static-call-site count gate: static sites do not bound dynamic invocations.
// The public lease compiler and JSON round trip exercise compatibility, then
// the real patch tool must preserve ownership and run all current validators.
func b1711RestoreLegacyOccurrenceLease(t *testing.T, bus *types.BusContext, doc *types.AnswerDocumentV2, historical []types.AnswerDiagramRelationRepairFailure) (*types.AnswerDocumentV2, *types.AnswerDiagramRelationRepairLease) {
	t.Helper()
	lease := types.NewAnswerDiagramRelationRepairLease(doc, historical, nil)
	if lease == nil || lease.Version != 1 || !types.AnswerDiagramRelationRepairLeaseIsLocallyExecutable(lease) {
		t.Fatalf("historical fixture must compile into an executable version-1 lease: %+v", lease)
	}
	raw, err := json.Marshal(lease)
	if err != nil {
		t.Fatal(err)
	}
	var restored types.AnswerDiagramRelationRepairLease
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	bus.Mutable.SetLastRejectedAnswerDocumentV2(doc)
	bus.Mutable.SetAnswerDiagramRelationRepairLease(&restored)
	if bus.Mutable.AnswerDocumentV2() != nil || bus.Mutable.PendingAnswerDocumentPatchBase() != nil {
		t.Fatal("historical rejected fixture must not be installed as accepted or staged output")
	}
	return bus.Mutable.LastRejectedAnswerDocumentV2(), bus.Mutable.AnswerDiagramRelationRepairLease()
}

func b1711LegacyOccurrenceFailure(anchor types.DiagramEdgeAnchor, occurrence int) types.AnswerDiagramRelationRepairFailure {
	return types.AnswerDiagramRelationRepairFailure{
		BlockID: "sequence", Issue: "call_edge_occurrence_unproven", RelationKind: types.DiagramRelCall,
		FromNode: anchor.FromNode, ToNode: anchor.ToNode,
		FromIdentity: anchor.FromIdentity, ToIdentity: anchor.ToIdentity, BodyOccurrence: occurrence,
	}
}

func TestB1711LegacyOccurrenceLeaseRemovePreservesAnchorAndSiblings(t *testing.T) {
	bus, doc, anchor := standaloneCompleteRowFixture(t)
	anchor.ClaimForm = types.ClaimCallEdge
	doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{anchor}
	diagramAnchor := anchor
	diagramAnchor.FromNode, diagramAnchor.ToNode = "A", "B"
	const first = "  A->>B: delegates appointment scheduling"
	const excess = "  A->>B: repeated appointment scheduling"
	doc.Blocks = append(doc.Blocks,
		types.AnswerBlock{ID: "sequence", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid",
				Body: "sequenceDiagram\n  participant A as \"VisitController.create\"\n  participant B as \"VisitService.schedule\"\n" + first + "\n" + excess},
			EdgeAnchors: []types.DiagramEdgeAnchor{diagramAnchor},
		},
		types.AnswerBlock{ID: "other-diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid",
				Body: "sequenceDiagram\n  participant A as \"VisitController.create\"\n  participant B as \"VisitService.schedule\"\n" + first},
			EdgeAnchors: []types.DiagramEdgeAnchor{diagramAnchor},
		},
	)
	beforeEvidence, err := json.Marshal(bus.Mutable.EmittedEvidence())
	if err != nil {
		t.Fatal(err)
	}
	base, lease := b1711RestoreLegacyOccurrenceLease(t, bus, doc,
		[]types.AnswerDiagramRelationRepairFailure{b1711LegacyOccurrenceFailure(diagramAnchor, 2)})
	if len(lease.Failures) != 1 {
		t.Fatalf("historical fixture must retain its one selected occurrence: %+v", lease.Failures)
	}
	failure := lease.Failures[0]
	if failure.BlockID != "sequence" || failure.Issue != "call_edge_occurrence_unproven" ||
		failure.BodyOccurrence != 2 || failure.FromNode != "A" || failure.ToNode != "B" ||
		failure.FailureRef == "" || !failure.AllowsAction("remove") {
		t.Fatalf("historical repair must select only the second visible call: %+v", failure)
	}
	baseSequence := blockByID(t, base, "sequence")
	if len(baseSequence.EdgeAnchors) != 1 || strings.Count(baseSequence.Diagram.Body, "A->>B:") != 2 {
		t.Fatalf("precondition lost the unique anchor or duplicate visible invocation: %+v", baseSequence)
	}
	patch := map[string]any{
		"diagram_edge_edits":  []any{map[string]any{"action": "remove", "failure_ref": failure.FailureRef}},
		"unchanged_block_ids": []string{"summary", "path", "other-diagram"},
	}
	result := standaloneCompleteRowExecute(t, bus, patch, true)
	if !result.Success {
		pending := bus.Mutable.PendingAnswerDocumentPatchBase()
		var remaining []types.DiagramEdgeAnchor
		if pending != nil {
			remaining = blockByID(t, pending, "sequence").EdgeAnchors
		}
		t.Fatalf("removing only historical selection 2 must accept in one patch while retaining occurrence 1's unique anchor; remaining_anchors=%+v result=%+v", remaining, result)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("successful correction was not published")
	}
	sequence := blockByID(t, got, "sequence")
	wantBody := strings.Replace(baseSequence.Diagram.Body, "\n"+excess, "", 1)
	if sequence.Diagram.Body != wantBody || !strings.Contains(sequence.Diagram.Body, first) ||
		strings.Contains(sequence.Diagram.Body, excess) || strings.Count(sequence.Diagram.Body, "A->>B:") != 1 {
		t.Fatalf("atomic correction must remove only the excess model statement:\n got=%s\nwant=%s", sequence.Diagram.Body, wantBody)
	}
	if !reflect.DeepEqual(sequence.EdgeAnchors, baseSequence.EdgeAnchors) {
		t.Fatalf("the legal first invocation lost or changed its original unique anchor: got=%+v want=%+v", sequence.EdgeAnchors, baseSequence.EdgeAnchors)
	}
	for _, id := range []string{"summary", "path", "other-diagram"} {
		if !reflect.DeepEqual(blockByID(t, got, id), blockByID(t, base, id)) {
			t.Errorf("atomic body cleanup changed unrelated model-authored block %q", id)
		}
	}
	afterEvidence, err := json.Marshal(bus.Mutable.EmittedEvidence())
	if err != nil || string(afterEvidence) != string(beforeEvidence) {
		t.Fatalf("repair changed source evidence: err=%v", err)
	}
}

func TestB1711LegacyOccurrenceLeaseMultipleBodiesHaveIndependentRemovalRefs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		keepCalls int
		anchors   int
		reverse   bool
		reply     bool
	}{
		{name: "one_anchor_ascending", keepCalls: 1, anchors: 1},
		{name: "one_anchor_descending_with_reply", keepCalls: 1, anchors: 1, reverse: true, reply: true},
		{name: "duplicate_anchors_ascending", keepCalls: 1, anchors: 3},
		{name: "duplicate_anchors_descending_with_reply", keepCalls: 1, anchors: 3, reverse: true, reply: true},
		{name: "two_retained_invocations_keep_both_anchors", keepCalls: 2, anchors: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus, doc, anchor := standaloneCompleteRowFixture(t)
			if tc.keepCalls == 2 {
				calls, repo := b1647TwoCallOccurrences(t)
				bus.RepoRoot = repo
				bus.Mutable = types.NewMutableState("Explain both source-proved invocations")
				bus.Mutable.SetRepoRoot(repo)
				bus.Mutable.AppendEvidence(calls)
				bus.EvidenceItems = calls
				doc.Citations = []types.Citation{{File: "calls.go", Line: 3, LineEnd: 6, Scope: types.ScopeLineRange}}
				anchor.FromIdentity, anchor.ToIdentity = "Caller", "Callee"
			}
			anchor.ClaimForm = types.ClaimCallEdge
			doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{anchor}
			anchor.FromNode, anchor.ToNode = "A", "B"
			header := fmt.Sprintf("sequenceDiagram\n  participant A as %q\n  participant B as %q", anchor.FromIdentity, anchor.ToIdentity)
			body, wantBody := header, header
			var anchors []types.DiagramEdgeAnchor
			for i := 1; i <= tc.keepCalls+2; i++ {
				label := fmt.Sprintf("invocation %d", i)
				line := "\n  A->>B: " + label
				body += line
				if i <= tc.keepCalls {
					wantBody += line
				}
				if tc.reply && i == 1 {
					const reply = "\n  B-->>A: result of the legal invocation"
					body += reply
					wantBody += reply
				}
				if i <= tc.anchors {
					row := anchor
					row.VisibleLabel = label
					anchors = append(anchors, row)
				}
			}
			doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "sequence", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: body}, EdgeAnchors: anchors})
			base, lease := b1711RestoreLegacyOccurrenceLease(t, bus, doc, []types.AnswerDiagramRelationRepairFailure{
				b1711LegacyOccurrenceFailure(anchor, tc.keepCalls+1), b1711LegacyOccurrenceFailure(anchor, tc.keepCalls+2),
			})
			if len(lease.Failures) != 2 {
				t.Fatalf("historical body selections need independent removal capabilities even when they share one retained anchor: %+v", lease.Failures)
			}
			refs, occurrences := map[string]bool{}, map[int]bool{}
			for _, failure := range lease.Failures {
				if failure.Issue != "call_edge_occurrence_unproven" || failure.BlockID != "sequence" ||
					failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierVisibleBodyEdge ||
					failure.FailureRef == "" || refs[failure.FailureRef] ||
					failure.BodyOccurrence <= tc.keepCalls || failure.BodyOccurrence > tc.keepCalls+2 ||
					occurrences[failure.BodyOccurrence] || len(failure.AllowedActions) != 1 || !failure.AllowsAction("remove") {
					t.Fatalf("historical selection lost its exact independent remove-only capability: %+v", failure)
				}
				refs[failure.FailureRef], occurrences[failure.BodyOccurrence] = true, true
			}
			// Restoring an old remove-only capability must not mint replacement
			// authority. Refusal must leave the historical lease and base live.
			leaseBefore, _ := json.Marshal(bus.Mutable.AnswerDiagramRelationRepairLease())
			bad := map[string]any{"diagram_edge_edits": []any{map[string]any{
				"action": "replace", "failure_ref": lease.Failures[0].FailureRef,
				"edge": map[string]any{"from_node": "A", "to_node": "B", "visible_label": "renamed excess call"},
			}}, "unchanged_block_ids": []string{"summary", "path"}}
			result := standaloneCompleteRowExecute(t, bus, bad, true)
			leaseAfter, _ := json.Marshal(bus.Mutable.AnswerDiagramRelationRepairLease())
			if result.Success || !strings.Contains(result.Summary, "does not allow action=replace") ||
				string(leaseBefore) != string(leaseAfter) || bus.Mutable.AnswerDocumentV2() != nil ||
				!reflect.DeepEqual(base, bus.Mutable.LastRejectedAnswerDocumentV2()) {
				t.Fatalf("historical occurrence gained replacement authority or changed the live base: %+v", result)
			}
			var edits []any
			for i := range lease.Failures {
				if tc.reverse {
					i = len(lease.Failures) - 1 - i
				}
				edits = append(edits, map[string]any{"action": "remove", "failure_ref": lease.Failures[i].FailureRef})
			}
			result = standaloneCompleteRowExecute(t, bus, map[string]any{
				"diagram_edge_edits": edits, "unchanged_block_ids": []string{"summary", "path"},
			}, true)
			if !result.Success {
				t.Fatalf("both historical selections must close in one patch, regardless of declared edit order: %+v", result)
			}
			got := bus.Mutable.AnswerDocumentV2()
			sequence := blockByID(t, got, "sequence")
			keep := tc.anchors
			if keep > tc.keepCalls {
				keep = tc.keepCalls
			}
			if sequence.Diagram.Body != wantBody || !reflect.DeepEqual(sequence.EdgeAnchors, anchors[:keep]) {
				t.Fatalf("cleanup did not preserve exactly the legal calls, reply, and their original anchors: anchors=%+v body=%s", sequence.EdgeAnchors, sequence.Diagram.Body)
			}
			for _, id := range []string{"summary", "path"} {
				if !reflect.DeepEqual(blockByID(t, got, id), blockByID(t, base, id)) {
					t.Errorf("atomic cleanup changed unrelated block %q", id)
				}
			}
		})
	}
}

func TestB1711LegacyOccurrenceLeaseCleanupPreservesDistinctAnchorOwnership(t *testing.T) {
	for _, mode := range []string{"shared_unproved_return", "equivalent_identity_spelling"} {
		t.Run(mode, func(t *testing.T) {
			bus, doc, anchor := standaloneCompleteRowFixture(t)
			anchor.ClaimForm = types.ClaimCallEdge
			doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{anchor}
			anchor.FromNode, anchor.ToNode = "A", "B"
			second := anchor
			second.VisibleLabel = "repeated appointment scheduling"
			switch mode {
			case "shared_unproved_return":
				second.RelationKind = types.DiagramRelReturn
				second.ClaimForm = ""
			case "equivalent_identity_spelling":
				second.FromIdentity = "VisitController::create"
				second.ToIdentity = "VisitService::schedule"
			}
			const legalBody = "sequenceDiagram\n  participant A as \"VisitController.create\"\n  participant B as \"VisitService.schedule\"\n  A->>B: delegates appointment scheduling"
			const selectedLine = "\n  A->>B: repeated appointment scheduling"
			doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "sequence", Kind: types.BlockDiagram,
				Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: legalBody + selectedLine},
				EdgeAnchors: []types.DiagramEdgeAnchor{anchor, second}})
			historical := []types.AnswerDiagramRelationRepairFailure{b1711LegacyOccurrenceFailure(second, 2)}
			wantFailures := 1
			if mode == "shared_unproved_return" {
				wantFailures = 2
				historical = append(historical, types.AnswerDiagramRelationRepairFailure{
					BlockID: "sequence", Issue: "return_edge_unproven", RelationKind: types.DiagramRelReturn,
					FromNode: second.FromNode, ToNode: second.ToNode,
					FromIdentity: second.FromIdentity, ToIdentity: second.ToIdentity, BodyOccurrence: 2,
				})
			}
			base, lease := b1711RestoreLegacyOccurrenceLease(t, bus, doc, historical)
			if len(lease.Failures) != wantFailures {
				t.Fatalf("historical fixture must retain only its selected body/anchor capabilities: %+v", lease.Failures)
			}
			var edits []any
			occurrenceFailure := false
			for _, failure := range lease.Failures {
				if failure.BlockID != "sequence" || failure.BodyOccurrence != 2 || !failure.AllowsAction("remove") ||
					(failure.Issue != "call_edge_occurrence_unproven" && failure.Issue != "return_edge_unproven") {
					t.Fatalf("historical fixture must not permit removal of the retained first occurrence: %+v", failure)
				}
				occurrenceFailure = occurrenceFailure || failure.Issue == "call_edge_occurrence_unproven"
				edits = append(edits, map[string]any{"action": "remove", "failure_ref": failure.FailureRef})
			}
			if !occurrenceFailure {
				t.Fatalf("fixture lost the historical occurrence failure: %+v", lease.Failures)
			}
			baseSequence := blockByID(t, base, "sequence")
			if strings.Count(baseSequence.Diagram.Body, selectedLine) != 1 || len(baseSequence.EdgeAnchors) != 2 {
				t.Fatalf("restored base lost the exact selected statement or either anchor: %+v", baseSequence)
			}
			// The historical base already contains normalized participant quoting.
			// Only the selected statement may change; all other bytes must survive.
			wantBody := strings.Replace(baseSequence.Diagram.Body, selectedLine, "", 1)
			result := standaloneCompleteRowExecute(t, bus, map[string]any{
				"diagram_edge_edits": edits, "unchanged_block_ids": []string{"summary", "path"},
			}, true)
			if !result.Success {
				pending := bus.Mutable.PendingAnswerDocumentPatchBase()
				var remaining []types.DiagramEdgeAnchor
				if pending != nil {
					remaining = blockByID(t, pending, "sequence").EdgeAnchors
				}
				t.Fatalf("removing the historical selection and its own anchor must preserve the first retained anchor; remaining=%+v result=%+v", remaining, result)
			}
			got := bus.Mutable.AnswerDocumentV2()
			sequence := blockByID(t, got, "sequence")
			if sequence.Diagram.Body != wantBody || !reflect.DeepEqual(sequence.EdgeAnchors, baseSequence.EdgeAnchors[:1]) {
				t.Fatalf("body cleanup changed the first legal invocation or left excess metadata: anchors=%+v\ngot=%q\nwant=%q", sequence.EdgeAnchors, sequence.Diagram.Body, wantBody)
			}
			for _, id := range []string{"summary", "path"} {
				if !reflect.DeepEqual(blockByID(t, got, id), blockByID(t, base, id)) {
					t.Errorf("selected body cleanup changed unrelated block %q", id)
				}
			}
		})
	}
}
