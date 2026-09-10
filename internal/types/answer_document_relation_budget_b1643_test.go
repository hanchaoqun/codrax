package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func b1643BudgetDocument(anchors ...DiagramEdgeAnchor) *AnswerDocumentV2 {
	return &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{{
		ID: "diagram", Kind: BlockDiagram, EdgeAnchors: anchors,
		Diagram: &AnswerDiagramBlock{Body: "sequenceDiagram\n    Logger->>Sink: model-owned message\n"},
	}}}
}

func b1643BudgetAnchor(identity string, relation DiagramRelationKind) DiagramEdgeAnchor {
	return DiagramEdgeAnchor{FromNode: "Logger", ToNode: "Sink", FromIdentity: "Logger.log",
		ToIdentity: identity, RelationKind: relation, VisibleLabel: "model-owned " + identity}
}

func b1643BudgetFailure(anchor DiagramEdgeAnchor) AnswerDiagramRelationRepairFailure {
	return AnswerDiagramRelationRepairFailure{BlockID: "diagram", Issue: "edge_anchor_node_identity_conflict",
		FromNode: anchor.FromNode, ToNode: anchor.ToNode, FromIdentity: anchor.FromIdentity,
		ToIdentity: anchor.ToIdentity, RelationKind: anchor.RelationKind, BodyOccurrence: 2}
}

func TestB1643BudgetUsesExactPriorAnchorSelection(t *testing.T) {
	for _, relation := range []DiagramRelationKind{DiagramRelCall, DiagramRelAssignment} {
		for _, selectedFirst := range []bool{false, true} {
			t.Run(string(relation)+"/selected_first="+map[bool]string{false: "false", true: "true"}[selectedFirst], func(t *testing.T) {
				other := b1643BudgetAnchor("Sink.flush", DiagramRelCall)
				selected := b1643BudgetAnchor("Sink.write", relation)
				anchors := []DiagramEdgeAnchor{other, selected}
				if selectedFirst {
					anchors = []DiagramEdgeAnchor{selected, other}
				}
				base := b1643BudgetDocument(anchors...)
				failure := b1643BudgetFailure(selected)
				lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{failure, failure}, nil)
				if lease == nil || len(lease.Failures) != 1 || lease.Failures[0].TargetCarrier != AnswerDiagramRelationRepairCarrierPriorAnchor {
					t.Fatalf("expected one exact prior carrier: %+v", lease)
				}
				matches := AnswerDiagramRelationRepairFailureAnchorCandidates(lease.Failures[0], anchors)
				if len(matches) != 1 || !reflect.DeepEqual(matches[0], selected) {
					t.Fatalf("executor selector did not choose selected tuple: %+v", matches)
				}
				before, _ := json.Marshal(lease)
				replacement := selected
				replacement.FromNode, replacement.ToNode = "Logger.log", "Sink.write"
				if got := ValidateAnswerDiagramRelationRepairLease(lease, b1643BudgetDocument(other, replacement)); len(got) != 0 {
					t.Fatalf("exact selected replacement lost its one-row budget: %+v", got)
				}
				for name, result := range map[string][]DiagramEdgeAnchor{
					"unselected-removal-is-not-a-donor":        {selected, replacement},
					"retained-selected-is-not-a-donor":         {other, selected, replacement},
					"duplicate-failure-does-not-double-budget": {other, replacement, replacement},
				} {
					got := ValidateAnswerDiagramRelationRepairLease(lease, b1643BudgetDocument(result...))
					if len(got) == 0 {
						t.Errorf("%s unexpectedly acquired budget", name)
					}
				}
				after, _ := json.Marshal(lease)
				if string(before) != string(after) {
					t.Fatal("budget checking mutated immutable lease")
				}
			})
		}
	}
}

func TestB1643BudgetPreservesCarrierLanes(t *testing.T) {
	selected := b1643BudgetAnchor("Sink.write", DiagramRelCall)
	other := b1643BudgetAnchor("Sink.flush", DiagramRelCall)
	for _, tc := range []struct {
		name, issue string
		carrier     AnswerDiagramRelationRepairTargetCarrier
		action      string
	}{
		{"stale", "typed_anchor_without_visible_edge", AnswerDiagramRelationRepairCarrierStaleAnchor, "replace"},
		{"metadata", "missing_relation_identity", AnswerDiagramRelationRepairCarrierPriorAnchorMetadata, "remove"},
		{"label", "diagram_visible_label_mismatch", AnswerDiagramRelationRepairCarrierLabelPair, "relabel"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := b1643BudgetFailure(selected)
			failure.Issue = tc.issue
			if tc.carrier == AnswerDiagramRelationRepairCarrierPriorAnchorMetadata {
				failure.TargetCarrier = tc.carrier
			}
			lease := NewAnswerDiagramRelationRepairLease(b1643BudgetDocument(other, selected), []AnswerDiagramRelationRepairFailure{failure}, nil)
			if lease == nil || len(lease.Failures) != 1 || lease.Failures[0].TargetCarrier != tc.carrier || !lease.Failures[0].AllowsAction(tc.action) {
				t.Fatalf("carrier capability changed: %+v", lease)
			}
			result := []DiagramEdgeAnchor{other}
			if tc.action != "remove" {
				changed := selected
				changed.VisibleLabel = "new model label"
				if tc.action == "replace" {
					changed.FromNode, changed.ToNode = "Logger.log", "Sink.write"
				}
				result = append(result, changed)
			}
			if got := ValidateAnswerDiagramRelationRepairLease(lease, b1643BudgetDocument(result...)); len(got) != 0 {
				t.Fatalf("existing %s operation failed accounting: %+v", tc.action, got)
			}
		})
	}

	t.Run("body-only-and-repeated-occurrences", func(t *testing.T) {
		for _, repeat := range []bool{false, true} {
			base := b1643BudgetDocument()
			failure := b1643BudgetFailure(selected)
			failure.Issue = "missing_call_anchor"
			if repeat {
				base = b1643BudgetDocument(selected, selected)
				failure.Issue = "call_edge_unproven"
			}
			lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{failure, failure}, nil)
			if lease == nil || len(lease.Failures) != 1 || lease.Failures[0].TargetCarrier != AnswerDiagramRelationRepairCarrierVisibleBodyEdge {
				t.Fatalf("body capability changed: %+v", lease)
			}
			if repeat && lease.Failures[0].AllowsAction("replace") {
				t.Fatal("repeated occurrence remove-only lane gained replacement permission")
			}
			if got := ValidateAnswerDiagramRelationRepairLease(lease, b1643BudgetDocument(selected)); len(got) != 0 {
				t.Fatalf("existing body-only addition or one-occurrence removal failed: %+v", got)
			}
			if !repeat {
				if got := ValidateAnswerDiagramRelationRepairLease(lease, b1643BudgetDocument(selected, selected)); len(got) == 0 {
					t.Fatal("duplicate body-only ref increased its one missing-anchor budget")
				}
			}
		}
	})
}

func TestB1643UnresolvedPriorIsNotMissingBodyBudget(t *testing.T) {
	selected := b1643BudgetAnchor("Sink.write", DiagramRelCall)
	failure := b1643BudgetFailure(selected)
	failure.TargetCarrier = AnswerDiagramRelationRepairCarrierPriorAnchor
	failure.AllowedActions = []AnswerDiagramRelationRepairAction{AnswerDiagramRelationRepairActionReplace}
	replacement := selected
	replacement.FromNode, replacement.ToNode = "Logger.log", "Sink.write"
	otherRelation := selected
	otherRelation.RelationKind = DiagramRelAssignment
	for _, tc := range []struct {
		name           string
		base, result   []DiagramEdgeAnchor
		candidateCount int
	}{
		{"missing-prior", []DiagramEdgeAnchor{otherRelation}, []DiagramEdgeAnchor{otherRelation, replacement}, 0},
		{"ambiguous-prior", []DiagramEdgeAnchor{selected, selected}, []DiagramEdgeAnchor{selected, replacement}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A malformed/older persisted capability must not turn failure to
			// resolve its claimed prior anchor into an extra missing-body slot.
			lease := &AnswerDiagramRelationRepairLease{Version: 1, Failures: []AnswerDiagramRelationRepairFailure{failure},
				Blocks: []AnswerDiagramRelationRepairLeaseBlock{{BlockID: "diagram", Kind: BlockDiagram, BaseAnchors: tc.base}}}
			if got := len(AnswerDiagramRelationRepairFailureAnchorCandidates(failure, tc.base)); got != tc.candidateCount {
				t.Fatalf("invalid test premise: candidates=%d want=%d", got, tc.candidateCount)
			}
			if got := ValidateAnswerDiagramRelationRepairLease(lease, b1643BudgetDocument(tc.result...)); len(got) != 1 || got[0].Issue != "failed_relation_expanded" {
				t.Fatalf("unresolved prior gained a replacement slot: %+v", got)
			}
		})
	}
	t.Run("legacy-no-carrier", func(t *testing.T) {
		failure.TargetCarrier = AnswerDiagramRelationRepairCarrierUnknown
		failure.AllowedActions = nil
		lease := &AnswerDiagramRelationRepairLease{Version: 1, Failures: []AnswerDiagramRelationRepairFailure{failure},
			Blocks: []AnswerDiagramRelationRepairLeaseBlock{{BlockID: "diagram", Kind: BlockDiagram, BaseAnchors: []DiagramEdgeAnchor{selected}}}}
		if got := ValidateAnswerDiagramRelationRepairLease(lease, b1643BudgetDocument(replacement)); len(got) != 0 {
			t.Fatalf("legacy accounting lane changed: %+v", got)
		}
	})
}
