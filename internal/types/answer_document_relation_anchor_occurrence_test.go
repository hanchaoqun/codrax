package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func b1655OccurrenceFixture() (*AnswerDocumentV2, AnswerDiagramRelationRepairFailure) {
	a := DiagramEdgeAnchor{FromNode: "Dispatch", ToNode: "Bus", RelationKind: DiagramRelCall, VisibleLabel: "first model wording"}
	b := a
	b.VisibleLabel = "second model wording"
	return &AnswerDocumentV2{Blocks: []AnswerBlock{{ID: "seq", Kind: BlockDiagram,
		Diagram:     &AnswerDiagramBlock{Kind: "sequence", Body: "sequenceDiagram\nparticipant Dispatch\nparticipant Bus\n"},
		EdgeAnchors: []DiagramEdgeAnchor{a, b},
	}}}, AnswerDiagramRelationRepairFailure{BlockID: "seq", Issue: "typed_anchor_without_visible_edge", RelationKind: DiagramRelCall, FromNode: "Dispatch", ToNode: "Bus", AnchorOccurrence: 2}
}

func TestB1655OccurrenceSelectorAndLegacy(t *testing.T) {
	base, failure := b1655OccurrenceFixture()
	for _, tc := range []struct {
		name   string
		mutate func(*AnswerDiagramRelationRepairFailure)
		want   int
	}{
		{"exact second", func(*AnswerDiagramRelationRepairFailure) {}, 1},
		{"legacy ambiguous", func(f *AnswerDiagramRelationRepairFailure) { f.AnchorOccurrence = 0 }, 2},
		{"negative", func(f *AnswerDiagramRelationRepairFailure) { f.AnchorOccurrence = -1 }, 0},
		{"out of range", func(f *AnswerDiagramRelationRepairFailure) { f.AnchorOccurrence = 3 }, 0},
		{"wrong pair", func(f *AnswerDiagramRelationRepairFailure) { f.ToNode = "Other" }, 0},
		{"wrong relation", func(f *AnswerDiagramRelationRepairFailure) { f.RelationKind = DiagramRelReturn }, 0},
		{"wrong issue", func(f *AnswerDiagramRelationRepairFailure) { f.Issue = "call_edge_unproven" }, 0},
		{"body is not metadata", func(f *AnswerDiagramRelationRepairFailure) { f.BodyOccurrence = 2 }, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := failure
			tc.mutate(&f)
			got := AnswerDiagramRelationRepairFailureBaseAnchorCandidates(base, f)
			if len(got) != tc.want || (tc.want == 1 && got[0] != base.Blocks[0].EdgeAnchors[1]) {
				t.Fatalf("selection=%+v", got)
			}
		})
	}
	legacy := failure
	legacy.AnchorOccurrence = 0
	if got := AssignAnswerDiagramRelationRepairFailureRefs(base, []AnswerDiagramRelationRepairFailure{legacy})[0]; got.TargetCarrier != AnswerDiagramRelationRepairCarrierUnknown || len(got.AllowedActions) != 0 {
		t.Fatalf("legacy ambiguity widened: %+v", got)
	}
	base.Blocks[0].EdgeAnchors = base.Blocks[0].EdgeAnchors[:1]
	if got := AssignAnswerDiagramRelationRepairFailureRefs(base, []AnswerDiagramRelationRepairFailure{legacy})[0]; !got.AllowsAction("remove") || !got.AllowsAction("replace") {
		t.Fatalf("legacy unique regressed: %+v", got)
	}
}

func TestB1655OccurrenceLeaseJSONAndGeneration(t *testing.T) {
	base, second := b1655OccurrenceFixture()
	first := second
	first.AnchorOccurrence = 1
	lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{first, second, first}, nil)
	if lease == nil || len(lease.Failures) != 2 || lease.Failures[0].FailureRef == lease.Failures[1].FailureRef {
		t.Fatalf("occurrences collapsed: %+v", lease)
	}
	for _, f := range lease.Failures {
		if !f.AllowsAction("remove") || f.AllowsAction("replace") || !AnswerDiagramRelationRepairOccurrenceMatchesBase(base, f) {
			t.Fatalf("capability/snapshot: %+v", f)
		}
	}
	raw, _ := json.Marshal(lease)
	var restored AnswerDiagramRelationRepairLease
	if err := json.Unmarshal(raw, &restored); err != nil || !reflect.DeepEqual(lease.Failures, restored.Failures) || !reflect.DeepEqual(lease.Blocks, restored.Blocks) {
		t.Fatalf("lease round trip: %v", err)
	}
	encoded, _ := json.Marshal(&restored)
	if string(encoded) != string(raw) {
		t.Fatal("restored lease changed wire bytes")
	}
	for _, f := range restored.Failures {
		if !AnswerDiagramRelationRepairOccurrenceMatchesBase(base, f) {
			t.Fatal("round trip lost live selector")
		}
	}
	for _, mutation := range []string{"reorder", "label", "body", "identity"} {
		t.Run(mutation, func(t *testing.T) {
			changed, _ := b1655OccurrenceFixture()
			switch mutation {
			case "reorder":
				changed.Blocks[0].EdgeAnchors[0], changed.Blocks[0].EdgeAnchors[1] = changed.Blocks[0].EdgeAnchors[1], changed.Blocks[0].EdgeAnchors[0]
			case "label":
				changed.Blocks[0].EdgeAnchors[1].VisibleLabel = "changed opaque bytes"
			case "body":
				changed.Blocks[0].Diagram.Body += "Dispatch->>Bus: newly authored edge\n"
			case "identity":
				changed.Blocks[0].EdgeAnchors[0].FromIdentity = "OtherCaller"
			}
			for _, f := range lease.Failures {
				if AnswerDiagramRelationRepairOccurrenceMatchesBase(changed, f) {
					t.Fatalf("stale %s ref authorized", mutation)
				}
			}
		})
	}
}

func TestB1655OccurrenceDoesNotMultiplyRemovalOrReplacementBudget(t *testing.T) {
	base, f := b1655OccurrenceFixture()
	lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{f, f}, nil)
	if lease == nil || len(lease.Failures) != 1 {
		t.Fatal("duplicate failure must not add budget")
	}
	for _, tc := range []struct {
		name   string
		rows   []DiagramEdgeAnchor
		wantOK bool
	}{
		{"selected only", base.Blocks[0].EdgeAnchors[:1], true},
		{"both same tuple", nil, false},
		{"unselected same pair other relation", []DiagramEdgeAnchor{{FromNode: "Dispatch", ToNode: "Bus", RelationKind: DiagramRelReturn}}, false},
		{"replace is not granted", []DiagramEdgeAnchor{base.Blocks[0].EdgeAnchors[0], {FromNode: "Bus", ToNode: "Dispatch", RelationKind: DiagramRelCall}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := &AnswerDocumentV2{Blocks: []AnswerBlock{{ID: "seq", Kind: BlockDiagram, Diagram: base.Blocks[0].Diagram, EdgeAnchors: tc.rows}}}
			v := ValidateAnswerDiagramRelationRepairLease(lease, result)
			if (len(v) == 0) != tc.wantOK {
				t.Fatalf("scope violations=%+v", v)
			}
		})
	}
}
