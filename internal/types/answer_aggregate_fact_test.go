package types

import "testing"

func TestNormalizeAnswerAggregateFacts_DedupesAndTrims(t *testing.T) {
	in := []AnswerAggregateFact{
		{
			Kind:  AnswerAggregateUniqueCount,
			Label: "  unique files  ",
			Value: " 3 ",
			Unit:  " files ",
			Dimensions: []AnswerAggregateDimension{
				{Name: " scope ", Value: " production "},
				{Name: "scope", Value: "production"},
			},
			Members: []string{" internal/a.go ", "internal/a.go", "internal/b.cpp"},
		},
		{
			Kind:  AnswerAggregateUniqueCount,
			Label: "unique files",
			Value: "3",
			Unit:  "files",
			Dimensions: []AnswerAggregateDimension{
				{Name: "scope", Value: "production"},
			},
			Members: []string{"internal/a.go", "internal/b.cpp"},
		},
	}
	got, err := NormalizeAnswerAggregateFacts(in)
	if err != nil {
		t.Fatalf("NormalizeAnswerAggregateFacts returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("deduped facts len=%d, want 1: %+v", len(got), got)
	}
	if got[0].Label != "unique files" || got[0].Value != "3" || got[0].Unit != "files" {
		t.Fatalf("fact not trimmed: %+v", got[0])
	}
	if len(got[0].Dimensions) != 1 || got[0].Dimensions[0].Name != "scope" || got[0].Dimensions[0].Value != "production" {
		t.Fatalf("dimensions not normalized: %+v", got[0].Dimensions)
	}
	if len(got[0].Members) != 2 || got[0].Members[1] != "internal/b.cpp" {
		t.Fatalf("members not normalized: %+v", got[0].Members)
	}
}

func TestNormalizeAnswerAggregateFacts_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		in   []AnswerAggregateFact
	}{
		{name: "kind", in: []AnswerAggregateFact{{Kind: "derived_guess", Label: "x", Value: "1"}}},
		{name: "label", in: []AnswerAggregateFact{{Kind: AnswerAggregateTotalCount, Value: "1"}}},
		{name: "value", in: []AnswerAggregateFact{{Kind: AnswerAggregateTotalCount, Label: "x"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeAnswerAggregateFacts(tc.in); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestMutableState_StableInvestigationAggregateFactsRetention(t *testing.T) {
	mu := NewMutableState("q")
	facts := []AnswerAggregateFact{{
		Kind:    AnswerAggregateBucketCount,
		Label:   "CLI bucket",
		Value:   "2",
		Unit:    "items",
		Members: []string{"--foo", "--bar"},
	}}
	mu.SetInvestigationAggregateFacts(facts)
	if got := mu.StableInvestigationAggregateFacts(); len(got) != 0 {
		t.Fatalf("downgraded/current facts must not be stable before completion: %+v", got)
	}
	mu.SetInvestigationComplete("done")
	mu.RetainInvestigationAggregateFacts()
	mu.ResetInvestigationComplete()

	got := mu.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Label != "CLI bucket" || len(got[0].Members) != 2 {
		t.Fatalf("stable aggregate facts not retained across reset: %+v", got)
	}
	got[0].Members[0] = "mutated"
	again := mu.StableInvestigationAggregateFacts()
	if again[0].Members[0] != "--foo" {
		t.Fatalf("StableInvestigationAggregateFacts must return a defensive copy: %+v", again)
	}
}
