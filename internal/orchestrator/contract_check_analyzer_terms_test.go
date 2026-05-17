package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

type analyzerTermStubOracle struct {
	found map[string]int
}

func (s analyzerTermStubOracle) SymbolExists(name string) (bool, int) {
	tier, ok := s.found[name]
	return ok, tier
}

func (s analyzerTermStubOracle) SymbolExistsFlat(name string) (bool, int) {
	tier, ok := s.found[name]
	return ok, tier
}

func TestRelaxUnsupportedAnalyzerMustIncludeTerms_DemotesUnsupportedFileStem(t *testing.T) {
	mut := types.NewMutableState("x")
	mut.AppendEvidence([]types.EvidenceItem{{
		Source: "internal/orchestrator/contract_check.go",
	}})
	contract := types.AnswerContract{
		MustInclude: []string{"contract_checker", "emit_evidence"},
		MustIncludeTerms: []types.ContractTerm{
			{
				Text:   "contract_checker",
				Kind:   types.ContractTermFileStem,
				Source: types.ContractTermSourceAnalyzerEntity,
			},
			{
				Text:   "emit_evidence",
				Kind:   types.ContractTermToolName,
				Source: types.ContractTermSourceAnalyzerEntity,
			},
		},
	}
	got := relaxUnsupportedAnalyzerMustIncludeTerms(contract, mut, nil)
	if containsString(got.MustInclude, "contract_checker") {
		t.Fatalf("unsupported analyzer file-stem term should be demoted: %+v", got)
	}
	if !containsString(got.MustInclude, "emit_evidence") {
		t.Fatalf("supported tool-name term should remain: %+v", got)
	}
	for _, term := range got.MustIncludeTerms {
		if term.Text == "contract_checker" {
			t.Fatalf("unsupported typed term should be removed: %+v", got.MustIncludeTerms)
		}
	}
}

func TestRelaxUnsupportedAnalyzerMustIncludeTerms_KeepsReliableSymbol(t *testing.T) {
	mut := types.NewMutableState("x")
	contract := types.AnswerContract{
		MustInclude: []string{"runContractCheck"},
		MustIncludeTerms: []types.ContractTerm{{
			Text:   "runContractCheck",
			Kind:   types.ContractTermSymbol,
			Source: types.ContractTermSourceAnalyzerEntity,
		}},
	}
	got := relaxUnsupportedAnalyzerMustIncludeTerms(contract, mut, analyzerTermStubOracle{
		found: map[string]int{"runContractCheck": 1},
	})
	if !containsString(got.MustInclude, "runContractCheck") || len(got.MustIncludeTerms) != 1 {
		t.Fatalf("reliable analyzer symbol should remain: %+v", got)
	}
}

func TestRelaxUnsupportedAnalyzerMustIncludeTerms_DemotesSymbolAbsentFromEvidence(t *testing.T) {
	mut := types.NewMutableState("x")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:         types.EvidenceDirect,
		Subject:      "EvidenceClosure",
		AnchorSymbol: "EvidenceClosure",
		Source:       "internal/types/evidence_closure.go",
	}})
	contract := types.AnswerContract{
		MustInclude: []string{"SymbolOracle"},
		MustIncludeTerms: []types.ContractTerm{{
			Text:   "SymbolOracle",
			Kind:   types.ContractTermSymbol,
			Source: types.ContractTermSourceAnalyzerEntity,
		}},
	}
	got := relaxUnsupportedAnalyzerMustIncludeTerms(contract, mut, analyzerTermStubOracle{
		found: map[string]int{"SymbolOracle": 1},
	})
	if containsString(got.MustInclude, "SymbolOracle") || len(got.MustIncludeTerms) != 0 {
		t.Fatalf("analyzer symbol absent from answer-grade evidence should be demoted despite graph presence: %+v", got)
	}
}

func TestRelaxUnsupportedAnalyzerMustIncludeTerms_KeepsEvidenceSupportedSymbol(t *testing.T) {
	mut := types.NewMutableState("x")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:         types.EvidenceMechanism,
		Subject:      "SymbolOracle",
		AnchorSymbol: "SymbolOracle",
		Source:       "internal/types/symbol_oracle.go",
	}})
	contract := types.AnswerContract{
		MustInclude: []string{"SymbolOracle"},
		MustIncludeTerms: []types.ContractTerm{{
			Text:   "SymbolOracle",
			Kind:   types.ContractTermSymbol,
			Source: types.ContractTermSourceAnalyzerEntity,
		}},
	}
	got := relaxUnsupportedAnalyzerMustIncludeTerms(contract, mut, analyzerTermStubOracle{
		found: map[string]int{"SymbolOracle": 1},
	})
	if !containsString(got.MustInclude, "SymbolOracle") || len(got.MustIncludeTerms) != 1 {
		t.Fatalf("evidence-supported analyzer symbol should remain: %+v", got)
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
