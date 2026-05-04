package types

import (
	"reflect"
	"strings"
	"testing"
)

// G2-2 (post_v2_runtime_gap_remediation, 2026-05-04). Unified
// AnswerDocumentMutation.Apply must be byte-equivalent to the
// legacy paths it lifts onto a single surface.

func TestMutationKind_IsValid(t *testing.T) {
	if !MutationReplaceAll.IsValid() || !MutationPartial.IsValid() {
		t.Error("known kinds must be valid")
	}
	if MutationKind("").IsValid() || MutationKind("foo").IsValid() {
		t.Error("empty/bogus must be invalid")
	}
}

func TestAnswerDocumentMutation_IsValid(t *testing.T) {
	cases := []struct {
		name string
		m    AnswerDocumentMutation
		want bool
	}{
		{"replace happy", AnswerDocumentMutation{Kind: MutationReplaceAll, Replace: &AnswerDocumentV2{}}, true},
		{"replace missing doc", AnswerDocumentMutation{Kind: MutationReplaceAll}, false},
		{"replace with patch", AnswerDocumentMutation{Kind: MutationReplaceAll, Replace: &AnswerDocumentV2{}, Patch: &AnswerDocumentV2Patch{}}, false},
		{"partial happy", AnswerDocumentMutation{Kind: MutationPartial, Patch: &AnswerDocumentV2Patch{}}, true},
		{"partial missing patch", AnswerDocumentMutation{Kind: MutationPartial}, false},
		{"partial with replace", AnswerDocumentMutation{Kind: MutationPartial, Replace: &AnswerDocumentV2{}, Patch: &AnswerDocumentV2Patch{}}, false},
		{"empty kind", AnswerDocumentMutation{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.m.IsValid(); got != c.want {
				t.Errorf("IsValid()=%v, want %v", got, c.want)
			}
		})
	}
}

// Apply ReplaceAll: result is the Replace doc verbatim; prev is
// discarded (not merged).
func TestAnswerDocumentMutation_Apply_ReplaceAll_EquivalentToFullEmit(t *testing.T) {
	prev := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{
		{ID: "old", Kind: BlockSummary, Text: "stale"},
	}}
	fresh := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{
		{ID: "b1", Kind: BlockSummary, Text: "new"},
	}}
	m := NewReplaceAllMutation(fresh)
	got, err := m.Apply(prev)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got != fresh {
		t.Errorf("ReplaceAll must return the Replace doc identity (got %p, want %p)", got, fresh)
	}
}

// Apply Partial: result equals ApplyAnswerDocumentV2Patch(prev,
// Patch). Byte-equivalence of merged doc.
func TestAnswerDocumentMutation_Apply_Partial_EquivalentToPatchApply(t *testing.T) {
	prev := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{
		{ID: "a", Kind: BlockSummary, Text: "summary"},
		{ID: "b", Kind: BlockOrderedList},
	}}
	patch := &AnswerDocumentV2Patch{
		UnchangedBlockIDs: []string{"a"},
		ReplaceBlocks: []AnswerBlock{
			{ID: "b", Kind: BlockOrderedList, Text: "modified",
				Items: []AnswerBlockItem{{ID: "i1", Label: "L"}}},
		},
	}
	wantMerged, err := ApplyAnswerDocumentV2Patch(prev, patch)
	if err != nil {
		t.Fatalf("legacy apply: %v", err)
	}
	gotMerged, err := NewPartialMutation(patch).Apply(prev)
	if err != nil {
		t.Fatalf("mutation apply: %v", err)
	}
	if !reflect.DeepEqual(gotMerged, wantMerged) {
		t.Errorf("Mutation.Apply Partial must equal legacy ApplyAnswerDocumentV2Patch.\nlegacy:  %+v\nmutation: %+v",
			wantMerged, gotMerged)
	}
}

// Invalid mutation shapes return an error from Apply.
func TestAnswerDocumentMutation_Apply_RejectsInvalidShape(t *testing.T) {
	if _, err := (AnswerDocumentMutation{}).Apply(nil); err == nil {
		t.Error("empty kind must error")
	}
	if _, err := (AnswerDocumentMutation{Kind: MutationReplaceAll}).Apply(nil); err == nil {
		t.Error("ReplaceAll missing doc must error")
	}
	if _, err := (AnswerDocumentMutation{Kind: MutationPartial}).Apply(nil); err == nil {
		t.Error("Partial missing patch must error")
	}
}

// Summary format: ReplaceAll case.
func TestAnswerDocumentMutation_Summary_ReplaceAll(t *testing.T) {
	doc := &AnswerDocumentV2{DocumentModel: "v2",
		Blocks:    []AnswerBlock{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}},
		Citations: []Citation{{File: "x.go", Line: 1}, {File: "y.go", Line: 2}},
	}
	m := NewReplaceAllMutation(doc)
	got := m.Summary()
	if got != "replace_all blocks=4 citations=2" {
		t.Errorf("Summary = %q, want \"replace_all blocks=4 citations=2\"", got)
	}
}

func TestAnswerDocumentMutation_Summary_PartialInherited(t *testing.T) {
	patch := &AnswerDocumentV2Patch{
		UnchangedBlockIDs: []string{"a", "b"},
		ReplaceBlocks:     []AnswerBlock{{ID: "c"}},
		RemoveBlockIDs:    []string{"d"},
	}
	got := NewPartialMutation(patch).Summary()
	if got != "partial unchanged=2 replace=1 add=0 remove=1 citations_inherited" {
		t.Errorf("Summary = %q", got)
	}
}

func TestAnswerDocumentMutation_Summary_PartialReplaceCitations(t *testing.T) {
	patch := &AnswerDocumentV2Patch{
		UnchangedBlockIDs: []string{"a"},
		ReplaceCitations:  []Citation{{File: "x.go", Line: 1}, {File: "y.go", Line: 2}, {File: "z.go", Line: 3}},
	}
	got := NewPartialMutation(patch).Summary()
	if !strings.Contains(got, "citations_replaced=3") {
		t.Errorf("Summary missing citations_replaced=3: %q", got)
	}
}

func TestAnswerDocumentMutation_Summary_PartialAppendCitations(t *testing.T) {
	patch := &AnswerDocumentV2Patch{
		UnchangedBlockIDs: []string{"a"},
		AppendCitations:   []Citation{{File: "x.go", Line: 1}},
	}
	got := NewPartialMutation(patch).Summary()
	if !strings.Contains(got, "citations_appended=1") {
		t.Errorf("Summary missing citations_appended=1: %q", got)
	}
}

// R4 lock — Mutation.Summary is INTERNAL telemetry only; it must
// still avoid leaking Go-internal type names so a future
// observability surface that streams it can pipe to LLM-adjacent
// surfaces without scrubbing.
func TestAnswerDocumentMutation_Summary_NoInternalJargon(t *testing.T) {
	cases := []AnswerDocumentMutation{
		NewReplaceAllMutation(&AnswerDocumentV2{}),
		NewPartialMutation(&AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"x"}}),
	}
	for _, m := range cases {
		got := m.Summary()
		for _, banned := range []string{"AnswerDocumentV2", "MutationKind", "AnswerBlock", "RenderedClaimUse"} {
			if strings.Contains(got, banned) {
				t.Errorf("Summary leaks Go type name %q: %q", banned, got)
			}
		}
	}
}
