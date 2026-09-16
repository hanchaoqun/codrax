package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A repeated visible invocation exceeds one proved call site's occurrence
// budget, but removing that excess body statement must not consume the unique
// anchor still owned by the first, legal invocation. Exercise the producer's
// actual public repair ref; do not handcraft a private failure or candidate.
func TestB1711PublicRemoveExcessBodyPreservesLegalAnchorAndSiblings(t *testing.T) {
	bus, doc, anchor := standaloneCompleteRowFixture(t)
	anchor.ClaimForm = types.ClaimCallEdge
	doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{anchor}
	diagramAnchor := anchor
	diagramAnchor.FromNode, diagramAnchor.ToNode = "A", "B"
	const first = "  A->>B: delegates appointment scheduling"
	const excess = "  A->>B: repeats an unproved invocation"
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
	result := standaloneCompleteRowExecute(t, bus, doc, false)
	if result.Success || result.Repair == nil {
		t.Fatalf("the unsupported second visible occurrence must be rejected: %+v", result)
	}
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatalf("public emit must publish the exact repair delta: %v; result=%+v", err, result)
	}
	if len(delta.Failures) != 1 {
		t.Fatalf("only the excess second occurrence should fail, not the legal anchor or sibling carriers: %+v", delta.Failures)
	}
	failure := delta.Failures[0]
	if failure.BlockID != "sequence" || failure.Issue != "call_edge_occurrence_unproven" ||
		failure.BodyOccurrence != 2 || failure.FromNode != "A" || failure.ToNode != "B" ||
		failure.FailureRef == "" || !failure.AllowsAction("remove") {
		t.Fatalf("public repair must select only the second visible call: %+v", failure)
	}
	base := bus.Mutable.LastRejectedAnswerDocumentV2()
	if base == nil || bus.Mutable.AnswerDocumentV2() != nil {
		t.Fatal("first rejection must retain a repair base without publishing the invalid document")
	}
	baseSequence := blockByID(t, base, "sequence")
	if len(baseSequence.EdgeAnchors) != 1 || strings.Count(baseSequence.Diagram.Body, "A->>B:") != 2 {
		t.Fatalf("precondition lost the unique anchor or duplicate visible invocation: %+v", baseSequence)
	}
	// Match the normal agent-side handoff using only the public emit's delta.
	lease := types.NewAnswerDiagramRelationRepairLease(base, delta.Failures, delta.AllowedAdditions)
	if lease == nil {
		t.Fatal("public repair delta did not create a live repair lease")
	}
	bus.Mutable.SetAnswerDiagramRelationRepairLease(lease)
	patch := map[string]any{
		"diagram_edge_edits":  []any{map[string]any{"action": "remove", "failure_ref": failure.FailureRef}},
		"unchanged_block_ids": []string{"summary", "path", "other-diagram"},
	}
	result = standaloneCompleteRowExecute(t, bus, patch, true)
	if !result.Success {
		pending := bus.Mutable.PendingAnswerDocumentPatchBase()
		var remaining []types.DiagramEdgeAnchor
		if pending != nil {
			remaining = blockByID(t, pending, "sequence").EdgeAnchors
		}
		t.Fatalf("removing only excess body occurrence 2 must accept in one patch while retaining occurrence 1's unique anchor; remaining_anchors=%+v result=%+v", remaining, result)
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

func TestB1711PublicMultipleExcessBodiesHaveIndependentRemovalRefs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		callSites int
		anchors   int
		reverse   bool
		reply     bool
	}{
		{name: "one_anchor_ascending", callSites: 1, anchors: 1},
		{name: "one_anchor_descending_with_reply", callSites: 1, anchors: 1, reverse: true, reply: true},
		{name: "duplicate_anchors_ascending", callSites: 1, anchors: 3},
		{name: "duplicate_anchors_descending_with_reply", callSites: 1, anchors: 3, reverse: true, reply: true},
		{name: "two_proved_sites_keep_both_anchors", callSites: 2, anchors: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus, doc, anchor := standaloneCompleteRowFixture(t)
			if tc.callSites == 2 {
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
			for i := 1; i <= tc.callSites+2; i++ {
				label := fmt.Sprintf("invocation %d", i)
				line := "\n  A->>B: " + label
				body += line
				if i <= tc.callSites {
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
			result := standaloneCompleteRowExecute(t, bus, doc, false)
			if result.Success || result.Repair == nil {
				t.Fatalf("two unsupported visible occurrences must be rejected: %+v", result)
			}
			var delta types.AnswerDiagramRelationRepairDelta
			if err := json.Unmarshal([]byte(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
				t.Fatal(err)
			}
			if len(delta.Failures) != 2 {
				t.Fatalf("each excess body needs its own public removal capability, even when both share one legal anchor: %+v", delta.Failures)
			}
			refs, occurrences := map[string]bool{}, map[int]bool{}
			for _, failure := range delta.Failures {
				if failure.Issue != "call_edge_occurrence_unproven" || failure.BlockID != "sequence" ||
					failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierVisibleBodyEdge ||
					failure.FailureRef == "" || refs[failure.FailureRef] ||
					failure.BodyOccurrence <= tc.callSites || failure.BodyOccurrence > tc.callSites+2 ||
					occurrences[failure.BodyOccurrence] || len(failure.AllowedActions) != 1 || !failure.AllowsAction("remove") {
					t.Fatalf("excess invocation did not retain its exact independent remove-only capability: %+v", failure)
				}
				refs[failure.FailureRef], occurrences[failure.BodyOccurrence] = true, true
			}
			base := bus.Mutable.LastRejectedAnswerDocumentV2()
			lease := types.NewAnswerDiagramRelationRepairLease(base, delta.Failures, delta.AllowedAdditions)
			bus.Mutable.SetAnswerDiagramRelationRepairLease(lease)
			// The visible-body classification must not mint replacement authority
			// for a source-unproved repetition. Refusal must leave the lease/base live.
			leaseBefore, _ := json.Marshal(bus.Mutable.AnswerDiagramRelationRepairLease())
			bad := map[string]any{"diagram_edge_edits": []any{map[string]any{
				"action": "replace", "failure_ref": delta.Failures[0].FailureRef,
				"edge": map[string]any{"from_node": "A", "to_node": "B", "visible_label": "renamed excess call"},
			}}, "unchanged_block_ids": []string{"summary", "path"}}
			result = standaloneCompleteRowExecute(t, bus, bad, true)
			leaseAfter, _ := json.Marshal(bus.Mutable.AnswerDiagramRelationRepairLease())
			if result.Success || !strings.Contains(result.Summary, "does not allow action=replace") ||
				string(leaseBefore) != string(leaseAfter) || bus.Mutable.AnswerDocumentV2() != nil ||
				!reflect.DeepEqual(base, bus.Mutable.LastRejectedAnswerDocumentV2()) {
				t.Fatalf("unproved occurrence gained replacement authority or changed the live base: %+v", result)
			}
			var edits []any
			for i := range delta.Failures {
				if tc.reverse {
					i = len(delta.Failures) - 1 - i
				}
				edits = append(edits, map[string]any{"action": "remove", "failure_ref": delta.Failures[i].FailureRef})
			}
			result = standaloneCompleteRowExecute(t, bus, map[string]any{
				"diagram_edge_edits": edits, "unchanged_block_ids": []string{"summary", "path"},
			}, true)
			if !result.Success {
				t.Fatalf("both independently selected excess occurrences must close in one patch, regardless of declared edit order: %+v", result)
			}
			got := bus.Mutable.AnswerDocumentV2()
			sequence := blockByID(t, got, "sequence")
			keep := tc.anchors
			if keep > tc.callSites {
				keep = tc.callSites
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

func TestB1711PublicExcessBodyCleanupPreservesDistinctAnchorOwnership(t *testing.T) {
	for _, mode := range []string{"shared_unproved_return", "equivalent_identity_spelling"} {
		t.Run(mode, func(t *testing.T) {
			bus, doc, anchor := standaloneCompleteRowFixture(t)
			anchor.ClaimForm = types.ClaimCallEdge
			doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{anchor}
			anchor.FromNode, anchor.ToNode = "A", "B"
			second := anchor
			second.VisibleLabel = "unproved second invocation"
			switch mode {
			case "shared_unproved_return":
				second.RelationKind = types.DiagramRelReturn
				second.ClaimForm = ""
			case "equivalent_identity_spelling":
				second.FromIdentity = "VisitController::create"
				second.ToIdentity = "VisitService::schedule"
			}
			const legalBody = "sequenceDiagram\n  participant A as VisitController.create\n  participant B as VisitService.schedule\n  A->>B: delegates appointment scheduling"
			doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "sequence", Kind: types.BlockDiagram,
				Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: legalBody + "\n  A->>B: unproved second invocation"},
				EdgeAnchors: []types.DiagramEdgeAnchor{anchor, second}})
			result := standaloneCompleteRowExecute(t, bus, doc, false)
			if result.Success || result.Repair == nil {
				t.Fatalf("the excess invocation must be rejected: %+v", result)
			}
			var delta types.AnswerDiagramRelationRepairDelta
			if err := json.Unmarshal([]byte(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
				t.Fatal(err)
			}
			wantFailures := 1
			if mode == "shared_unproved_return" {
				wantFailures = 2
			}
			if len(delta.Failures) != wantFailures {
				t.Fatalf("fixture must fail only the selected excess body/anchor: %+v", delta.Failures)
			}
			var edits []any
			occurrenceFailure := false
			for _, failure := range delta.Failures {
				if failure.BlockID != "sequence" || failure.BodyOccurrence != 2 || !failure.AllowsAction("remove") ||
					(failure.Issue != "call_edge_occurrence_unproven" && failure.Issue != "return_edge_unproven") {
					t.Fatalf("fixture must not publish a removal of the legal first occurrence: %+v", failure)
				}
				occurrenceFailure = occurrenceFailure || failure.Issue == "call_edge_occurrence_unproven"
				edits = append(edits, map[string]any{"action": "remove", "failure_ref": failure.FailureRef})
			}
			if !occurrenceFailure {
				t.Fatalf("fixture did not reach the occurrence budget boundary: %+v", delta.Failures)
			}
			base := bus.Mutable.LastRejectedAnswerDocumentV2()
			baseSequence := blockByID(t, base, "sequence")
			const excessLine = "\n  A->>B: unproved second invocation"
			if strings.Count(baseSequence.Diagram.Body, excessLine) != 1 || len(baseSequence.EdgeAnchors) != 2 {
				t.Fatalf("rejected base lost the exact excess statement or either anchor: %+v", baseSequence)
			}
			// The public emitter normalizes participant quoting before publishing
			// the rejected base. The patch may remove only this one statement;
			// every other byte must remain exactly as in that actual live base.
			wantBody := strings.Replace(baseSequence.Diagram.Body, excessLine, "", 1)
			bus.Mutable.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(base, delta.Failures, delta.AllowedAdditions))
			result = standaloneCompleteRowExecute(t, bus, map[string]any{
				"diagram_edge_edits": edits, "unchanged_block_ids": []string{"summary", "path"},
			}, true)
			if !result.Success {
				pending := bus.Mutable.PendingAnswerDocumentPatchBase()
				var remaining []types.DiagramEdgeAnchor
				if pending != nil {
					remaining = blockByID(t, pending, "sequence").EdgeAnchors
				}
				t.Fatalf("removing the excess body and its own failed anchor must preserve the first legal anchor; remaining=%+v result=%+v", remaining, result)
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
