package types

import "testing"

func TestRelaxAnalyzerEntityMustIncludeWithAggregateMemberSet_DemotesAnalyzerCandidates(t *testing.T) {
	contract := AnswerContract{
		MustInclude: []string{"Intent", "StaleCandidate", "explicit phrase"},
		MustIncludeTerms: []ContractTerm{
			{
				Text:   "Intent",
				Kind:   ContractTermSymbol,
				Source: ContractTermSourceAnalyzerEntity,
			},
			{
				Text:   "StaleCandidate",
				Kind:   ContractTermSymbol,
				Source: ContractTermSourceAnalyzerEntity,
			},
			{
				Text: "explicit phrase",
				Kind: ContractTermUserPhrase,
			},
		},
	}
	facts := []AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "verified enum members",
		Value:   "1",
		Members: []string{"Intent"},
	}}

	got := RelaxAnalyzerEntityMustIncludeWithAggregateMemberSet(contract, facts)
	if len(got.MustIncludeTerms) != 1 || got.MustIncludeTerms[0].Text != "explicit phrase" {
		t.Fatalf("analyzer terms should be demoted after member_set handoff, got %+v", got.MustIncludeTerms)
	}
	if len(got.MustInclude) != 1 || got.MustInclude[0] != "explicit phrase" {
		t.Fatalf("legacy analyzer pins should be removed with their typed source, got %+v", got.MustInclude)
	}
}

func TestRelaxAnalyzerEntityMustIncludeWithAggregateMemberSet_NoMemberSetNoOp(t *testing.T) {
	contract := AnswerContract{
		MustInclude: []string{"Intent"},
		MustIncludeTerms: []ContractTerm{{
			Text:   "Intent",
			Kind:   ContractTermSymbol,
			Source: ContractTermSourceAnalyzerEntity,
		}},
	}
	got := RelaxAnalyzerEntityMustIncludeWithAggregateMemberSet(contract, nil)
	if len(got.MustIncludeTerms) != 1 || len(got.MustInclude) != 1 {
		t.Fatalf("without member_set, analyzer pins remain the hard early contract: %+v", got)
	}
}
