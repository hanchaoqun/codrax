package types

import (
	"strconv"
	"testing"
)

func TestLargestExhaustiveMemberSet_RequiresMembersValueExhaustiveSignature(t *testing.T) {
	t.Run("returns largest closed set above threshold", func(t *testing.T) {
		facts := []AnswerAggregateFact{
			{
				Kind:    AnswerAggregateMemberSet,
				Label:   "small set",
				Value:   "5",
				Members: []string{"a", "b", "c", "d", "e"},
			},
			{
				Kind:    AnswerAggregateMemberSet,
				Label:   "big set",
				Value:   "94",
				Members: makeSeqMembers(94),
			},
		}
		got, ok := LargestExhaustiveMemberSet(facts, 30)
		if !ok {
			t.Fatalf("expected exhaustive set match, got none")
		}
		if got.Label != "big set" {
			t.Fatalf("expected largest set, got %q", got.Label)
		}
	})

	t.Run("rejects sample-only count facts even with members", func(t *testing.T) {
		// total_count with members is treated as samples, NOT closed
		// set; deterministic-only would be wrong on it.
		facts := []AnswerAggregateFact{
			{
				Kind:    AnswerAggregateTotalCount,
				Label:   "members",
				Value:   "94",
				Members: makeSeqMembers(94),
			},
		}
		if _, ok := LargestExhaustiveMemberSet(facts, 30); ok {
			t.Fatalf("total_count with members must not trigger deterministic-only")
		}
	})

	t.Run("rejects supporting coverage member sets", func(t *testing.T) {
		facts := []AnswerAggregateFact{
			{
				Kind:    AnswerAggregateMemberSet,
				Label:   "supporting candidate pool",
				Value:   "94",
				Role:    AnswerAggregateRoleSupportingCoverage,
				Members: makeSeqMembers(94),
			},
			{
				Kind:    AnswerAggregateMemberSet,
				Label:   "principal set",
				Value:   "31",
				Role:    AnswerAggregateRolePrincipalAnswer,
				Members: makeSeqMembers(31),
			},
		}
		got, ok := LargestExhaustiveMemberSet(facts, 30)
		if !ok {
			t.Fatalf("expected principal exhaustive set match, got none")
		}
		if got.Label != "principal set" {
			t.Fatalf("supporting coverage set must not drive deterministic review, got %q", got.Label)
		}
	})

	t.Run("rejects mismatched value", func(t *testing.T) {
		facts := []AnswerAggregateFact{
			{
				Kind:    AnswerAggregateMemberSet,
				Value:   "50",
				Members: makeSeqMembers(94),
			},
		}
		if _, ok := LargestExhaustiveMemberSet(facts, 30); ok {
			t.Fatalf("value!=len(members) must not be treated as exhaustive")
		}
	})

	t.Run("rejects below threshold", func(t *testing.T) {
		facts := []AnswerAggregateFact{
			{
				Kind:    AnswerAggregateMemberSet,
				Value:   "20",
				Members: makeSeqMembers(20),
			},
		}
		if _, ok := LargestExhaustiveMemberSet(facts, 30); ok {
			t.Fatalf("20 members must not exceed threshold 30")
		}
	})

	t.Run("threshold 0 disables", func(t *testing.T) {
		facts := []AnswerAggregateFact{
			{
				Kind:    AnswerAggregateMemberSet,
				Value:   "94",
				Members: makeSeqMembers(94),
			},
		}
		if _, ok := LargestExhaustiveMemberSet(facts, 0); ok {
			t.Fatalf("threshold=0 must disable deterministic-only")
		}
	})
}

func TestComputeExhaustiveMemberCoverage_SetEquality(t *testing.T) {
	fact := AnswerAggregateFact{
		Kind:    AnswerAggregateMemberSet,
		Value:   "4",
		Members: []string{"Intent", "Scenario", "Complexity", "QuestionFamily"},
	}

	t.Run("perfect coverage produces no failures", func(t *testing.T) {
		doc := &AnswerDocumentV2{
			Citations: []Citation{
				{File: "internal/types/analysis_ir.go", Line: 10},
				{File: "internal/types/analysis_ir.go", Line: 20},
				{File: "internal/types/analysis_ir.go", Line: 30},
				{File: "internal/types/analysis_ir.go", Line: 40},
			},
			Blocks: []AnswerBlock{{
				ID:          "items",
				Kind:        BlockOrderedList,
				SurfaceRole: SurfacePrincipal,
				FacetIDs:    []string{string(FacetEnumerationItem)},
				Items: []AnswerBlockItem{
					{Label: "Intent", CitationRef: 0},
					{Label: "Scenario", CitationRef: 1},
					{Label: "Complexity", CitationRef: 2},
					{Label: "QuestionFamily", CitationRef: 3},
				},
			}},
		}
		cov := ComputeExhaustiveMemberCoverage(doc, fact)
		if cov.HasFailures() {
			t.Fatalf("perfect coverage should not fail: %+v", cov)
		}
		if cov.PrincipalItemCount != 4 {
			t.Fatalf("expected 4 principal items, got %d", cov.PrincipalItemCount)
		}
	})

	t.Run("matching titled block avoids cross-category unexpected rows", func(t *testing.T) {
		doc := &AnswerDocumentV2{
			Blocks: []AnswerBlock{{
				ID:          "types",
				Kind:        BlockOrderedList,
				Title:       "Types",
				SurfaceRole: SurfacePrincipal,
				Items: []AnswerBlockItem{
					{Label: "Intent", CitationRef: -1},
				},
			}, {
				ID:          "kind-constants",
				Kind:        BlockOrderedList,
				Title:       "Kind constants（4）",
				SurfaceRole: SurfacePrincipal,
				Items: []AnswerBlockItem{
					{Label: "Intent", CitationRef: -1},
					{Label: "Scenario", CitationRef: -1},
					{Label: "Complexity", CitationRef: -1},
					{Label: "QuestionFamily", CitationRef: -1},
				},
			}},
		}
		factWithLabel := fact
		factWithLabel.Label = "Kind constants"
		cov := ComputeExhaustiveMemberCoverage(doc, factWithLabel)
		if cov.HasFailures() {
			t.Fatalf("coverage should ignore sibling category blocks when a matching title exists: %+v", cov)
		}
		if cov.PrincipalItemCount != 4 {
			t.Fatalf("expected 4 matching principal items, got %d", cov.PrincipalItemCount)
		}
	})

	t.Run("missing member flagged", func(t *testing.T) {
		doc := &AnswerDocumentV2{
			Citations: []Citation{{File: "x.go", Line: 1}, {File: "x.go", Line: 2}, {File: "x.go", Line: 3}},
			Blocks: []AnswerBlock{{
				ID:          "items",
				Kind:        BlockOrderedList,
				SurfaceRole: SurfacePrincipal,
				Items: []AnswerBlockItem{
					{Label: "Intent", CitationRef: 0},
					{Label: "Scenario", CitationRef: 1},
					{Label: "Complexity", CitationRef: 2},
				},
			}},
		}
		cov := ComputeExhaustiveMemberCoverage(doc, fact)
		if !cov.HasFailures() {
			t.Fatalf("missing QuestionFamily should fail")
		}
		if len(cov.MissingMembers) != 1 || cov.MissingMembers[0] != "QuestionFamily" {
			t.Fatalf("expected missing=[QuestionFamily], got %+v", cov.MissingMembers)
		}
	})

	t.Run("unexpected item flagged", func(t *testing.T) {
		doc := &AnswerDocumentV2{
			Citations: []Citation{{File: "x.go", Line: 1}, {File: "x.go", Line: 2}, {File: "x.go", Line: 3}, {File: "x.go", Line: 4}, {File: "x.go", Line: 5}},
			Blocks: []AnswerBlock{{
				ID:          "items",
				Kind:        BlockOrderedList,
				SurfaceRole: SurfacePrincipal,
				Items: []AnswerBlockItem{
					{Label: "Intent", CitationRef: 0},
					{Label: "Scenario", CitationRef: 1},
					{Label: "Complexity", CitationRef: 2},
					{Label: "QuestionFamily", CitationRef: 3},
					{Label: "InventedRole", CitationRef: 4},
				},
			}},
		}
		cov := ComputeExhaustiveMemberCoverage(doc, fact)
		if !cov.HasFailures() {
			t.Fatalf("unexpected item should fail")
		}
		if len(cov.UnexpectedItems) != 1 || cov.UnexpectedItems[0] != "InventedRole" {
			t.Fatalf("expected unexpected=[InventedRole], got %+v", cov.UnexpectedItems)
		}
	})

	t.Run("duplicate citation_ref flagged", func(t *testing.T) {
		doc := &AnswerDocumentV2{
			Citations: []Citation{{File: "x.go", Line: 1}, {File: "x.go", Line: 2}},
			Blocks: []AnswerBlock{{
				ID:          "items",
				Kind:        BlockOrderedList,
				SurfaceRole: SurfacePrincipal,
				Items: []AnswerBlockItem{
					{Label: "Intent", CitationRef: 0},
					{Label: "Scenario", CitationRef: 0}, // duplicate
					{Label: "Complexity", CitationRef: 1},
					{Label: "QuestionFamily", CitationRef: 1}, // duplicate
				},
			}},
		}
		cov := ComputeExhaustiveMemberCoverage(doc, fact)
		if len(cov.DuplicateCitations) != 2 {
			t.Fatalf("expected 2 duplicate citation_refs, got %+v", cov.DuplicateCitations)
		}
	})

	t.Run("invalid citation_ref out of range flagged", func(t *testing.T) {
		doc := &AnswerDocumentV2{
			Citations: []Citation{{File: "x.go", Line: 1}}, // only 1 citation
			Blocks: []AnswerBlock{{
				ID:          "items",
				Kind:        BlockOrderedList,
				SurfaceRole: SurfacePrincipal,
				Items: []AnswerBlockItem{
					{Label: "Intent", CitationRef: 0},
					{Label: "Scenario", CitationRef: 5}, // invalid
					{Label: "Complexity", CitationRef: 0},
					{Label: "QuestionFamily", CitationRef: 0},
				},
			}},
		}
		cov := ComputeExhaustiveMemberCoverage(doc, fact)
		if len(cov.InvalidCitationRefs) != 1 || cov.InvalidCitationRefs[0] != 5 {
			t.Fatalf("expected invalid=[5], got %+v", cov.InvalidCitationRefs)
		}
	})

	t.Run("qualified tail relaxation matches", func(t *testing.T) {
		factWithQualified := AnswerAggregateFact{
			Kind:    AnswerAggregateMemberSet,
			Value:   "2",
			Members: []string{"pkg.Foo", "pkg.Bar"},
		}
		doc := &AnswerDocumentV2{
			Citations: []Citation{{File: "x.go", Line: 1}, {File: "x.go", Line: 2}},
			Blocks: []AnswerBlock{{
				ID:          "items",
				Kind:        BlockOrderedList,
				SurfaceRole: SurfacePrincipal,
				Items: []AnswerBlockItem{
					{Label: "Foo", CitationRef: 0}, // matches pkg.Foo via tail
					{Label: "Bar", CitationRef: 1}, // matches pkg.Bar via tail
				},
			}},
		}
		cov := ComputeExhaustiveMemberCoverage(doc, factWithQualified)
		if cov.HasFailures() {
			t.Fatalf("tail-qualified labels should match, got %+v", cov)
		}
	})
}

func TestFormatExhaustiveMissingMembersHint_BoundedDisplay(t *testing.T) {
	t.Run("short list shows all", func(t *testing.T) {
		got := FormatExhaustiveMissingMembersHint([]string{"a", "b", "c"}, 6)
		if got != `"a", "b", "c"` {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("long list truncated with +N more", func(t *testing.T) {
		got := FormatExhaustiveMissingMembersHint([]string{"a", "b", "c", "d", "e", "f", "g", "h"}, 6)
		if got != `"a", "b", "c", "d", "e", "f" (+2 more)` {
			t.Fatalf("got %q", got)
		}
	})
}

func makeSeqMembers(n int) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = "Member" + strconv.Itoa(i)
	}
	return out
}
