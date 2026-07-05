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

// analyzerTermQualifiedStubOracle layers the QualifiedSymbolOracle
// extension over the base stub (QNO F1, 2026-07-05).
type analyzerTermQualifiedStubOracle struct {
	analyzerTermStubOracle
	qualified map[string]int
}

func (s analyzerTermQualifiedStubOracle) QualifiedSymbolExists(name string) (bool, int) {
	tier, ok := s.qualified[name]
	return ok, tier
}

// QNO F1 (2026-07-05): with Kind forced to symbol by R3b, a dotted
// user spelling would previously be demoted here — SymbolExists /
// SymbolExistsFlat never resolve "gate.Run" — moving the pin vacuum
// from the checker into this drop. The qualified extension keeps the
// pin alive when the symbol genuinely resolves; oracles without the
// extension (or unresolvable spellings) still demote honestly.
func TestRelaxUnsupportedAnalyzerMustIncludeTerms_QualifiedSymbolLanes(t *testing.T) {
	dotted := types.AnswerContract{
		MustInclude: []string{"gate.Run"},
		MustIncludeTerms: []types.ContractTerm{{
			Text:   "gate.Run",
			Kind:   types.ContractTermSymbol,
			Source: types.ContractTermSourceAnalyzerEntity,
		}},
	}

	// (a) No evidence signals + qualified-capable oracle → kept.
	mut := types.NewMutableState("x")
	got := relaxUnsupportedAnalyzerMustIncludeTerms(dotted, mut, analyzerTermQualifiedStubOracle{
		qualified: map[string]int{"gate.Run": 1},
	})
	if !containsString(got.MustInclude, "gate.Run") || len(got.MustIncludeTerms) != 1 {
		t.Fatalf("qualified-resolvable dotted symbol pin must survive: %+v", got)
	}

	// (b) Extension present but the spelling does not resolve → demoted
	// (honest miss, no fuzzy rescue).
	got = relaxUnsupportedAnalyzerMustIncludeTerms(dotted, mut, analyzerTermQualifiedStubOracle{
		qualified: map[string]int{"other.Run": 1},
	})
	if containsString(got.MustInclude, "gate.Run") || len(got.MustIncludeTerms) != 0 {
		t.Fatalf("unresolvable dotted symbol pin must demote: %+v", got)
	}

	// (c) Oracle without the extension → demoted (legacy posture,
	// production graph/multigraph oracles always carry the extension).
	got = relaxUnsupportedAnalyzerMustIncludeTerms(dotted, mut, analyzerTermStubOracle{
		found: map[string]int{"Run": 1},
	})
	if containsString(got.MustInclude, "gate.Run") {
		t.Fatalf("extension-less oracle must not rescue dotted spellings: %+v", got)
	}

	// (d) Evidence anchors the bare tail → supported through the shared
	// trailing-segment key, no oracle needed.
	mutEv := types.NewMutableState("x")
	mutEv.AppendEvidence([]types.EvidenceItem{{
		Kind:         types.EvidenceMechanism,
		Subject:      "Run",
		AnchorSymbol: "Run",
		Source:       "internal/analysis/gate/gate.go",
	}})
	got = relaxUnsupportedAnalyzerMustIncludeTerms(dotted, mutEv, analyzerTermStubOracle{})
	if !containsString(got.MustInclude, "gate.Run") || len(got.MustIncludeTerms) != 1 {
		t.Fatalf("tail-anchored dotted symbol pin must survive via evidence keys: %+v", got)
	}

	// (e) Evidence anchors only the SIBLING identifier → demoted; the
	// tail key rule is whole-key equality, never substring (s1a guard).
	mutSib := types.NewMutableState("x")
	mutSib.AppendEvidence([]types.EvidenceItem{{
		Kind:         types.EvidenceMechanism,
		Subject:      "RunWith",
		AnchorSymbol: "RunWith",
		Source:       "internal/analysis/gate/gate.go",
	}})
	got = relaxUnsupportedAnalyzerMustIncludeTerms(dotted, mutSib, analyzerTermStubOracle{})
	if containsString(got.MustInclude, "gate.Run") {
		t.Fatalf("sibling-only evidence must not support the dotted pin: %+v", got)
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
