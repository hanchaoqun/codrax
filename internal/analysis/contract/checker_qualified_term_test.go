package contract

// QNO F1 pins (2026-07-05): a symbol-kind must_include term carrying a
// scope qualifier ("gate.Run") must be enforced on the ANSWER SURFACE,
// never satisfied by citation stems. The historical vacuum: R3b pinned
// "gate.Run" with the INFERRED kind, InferContractTermKind routed the
// dotted spelling to file_stem, and contractFileStemCoveredByCitation
// keys ["gate.run","gate"] were covered by ANY stem-`gate` citation —
// under the Go pkg/pkg.go convention the s1a probe form (answer never
// names Run, cites gate/gate.go) shipped with 0 violations.

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func qualifiedSymbolContract() types.AnswerContract {
	return types.AnswerContract{
		MustIncludeTerms: []types.ContractTerm{{
			Text:   "gate.Run",
			Kind:   types.ContractTermSymbol,
			Source: types.ContractTermSourceAnalyzerEntity,
		}},
	}
}

// The probe form: answer discusses the package, cites gate/gate.go,
// never names Run in any spelling → MUST violate (≥1). The contrast
// case documents the pre-F1 vacuum staying exactly where it was for
// genuinely file-shaped terms: the same probe under an INFERRED kind
// (file_stem) is auto-satisfied by the stem-gate citation.
func TestQualifiedSymbolTerm_ProbeFormViolates(t *testing.T) {
	probe := Answer{
		Text:      "The gate package coordinates the completion checks and the retry budget.",
		Citations: []Citation{{File: "internal/analysis/gate/gate.go", Line: 12}},
	}

	viols := checkMustIncludeOracle(probe, qualifiedSymbolContract(), nil)
	if len(viols) < 1 {
		t.Fatalf("probe form (no Run mention, stem-gate citation) must violate a symbol-kind gate.Run term; got %d violations", len(viols))
	}
	if viols[0].Kind != ViolMustInclude {
		t.Fatalf("violation kind = %v, want ViolMustInclude (soft-by-default lane unchanged)", viols[0].Kind)
	}

	// Contrast: legacy string pin → inferred file_stem kind → citation
	// coverage applies. This is the pinned mechanism of the OLD vacuum;
	// if this ever starts violating, the file_stem citation contract
	// changed and F1's premise should be re-audited.
	inferred := types.AnswerContract{MustInclude: []string{"gate.Run"}}
	if v := checkMustIncludeOracle(probe, inferred, nil); len(v) != 0 {
		t.Fatalf("inferred file_stem term should stay citation-satisfiable (contrast pin); got %d violations", len(v))
	}
}

// Acceptance forms — the user's dotted spelling, its case variant, the
// bare trailing segment, and sentence-final punctuation all count as
// naming the subject: 0 violations.
func TestQualifiedSymbolTerm_AcceptedSpellings(t *testing.T) {
	c := qualifiedSymbolContract()
	for _, text := range []string{
		"The completion gate is driven by gate.Run before anything retries.",
		"The completion gate is driven by Gate.Run (method expression).",
		"Only the Run method decides whether the loop terminates.",
		"Everything funnels into Run.",      // sentence-final dot merges into the run
		"Everything funnels into gate.Run.", // qualified + sentence-final dot
	} {
		if v := checkMustIncludeOracle(Answer{Text: text}, c, nil); len(v) != 0 {
			t.Errorf("text %q should satisfy the gate.Run symbol term; got %d violations: %+v", text, len(v), v)
		}
	}
}

// Rejection forms — whole-token equality keeps the ORIGINAL s1a
// failure caught: sibling identifiers and prose containing the tail as
// a substring do not satisfy.
func TestQualifiedSymbolTerm_RejectedSpellings(t *testing.T) {
	c := qualifiedSymbolContract()
	for _, text := range []string{
		"The answer resolves everything through RunWith and never mentions the asked subject.",
		"The runtime keeps running until the budget expires.",
		"gate.RunWith is the sibling entry point.",
	} {
		if v := checkMustIncludeOracle(Answer{Text: text}, c, nil); len(v) != 1 {
			t.Errorf("text %q must NOT satisfy the gate.Run symbol term (whole-token rule); got %d violations", text, len(v))
		}
	}
}

// The tail acceptance is include-side only: contractTermHit itself is
// byte-stable so must_exclude does not widen (a forbidden "gate.Run"
// is not "present" just because the answer says bare Run).
func TestQualifiedSymbolTerm_MustExcludeNotWidened(t *testing.T) {
	c := types.AnswerContract{
		MustExcludeTerms: []types.ContractTerm{{
			Text: "gate.Run",
			Kind: types.ContractTermSymbol,
		}},
	}
	draft := Answer{Text: "Only the Run method decides the outcome."}
	if v := checkMustExcludeOracle(draft, c, nil); len(v) != 0 {
		t.Fatalf("must_exclude must not adopt the include-side tail acceptance; got %d violations", len(v))
	}
	verbatim := Answer{Text: "gate.Run decides the outcome."}
	if v := checkMustExcludeOracle(verbatim, c, nil); len(v) != 1 {
		t.Fatalf("verbatim forbidden spelling must still hit must_exclude; got %d violations", len(v))
	}
}

// Unqualified symbol terms never enter the tail-acceptance path — their
// semantics stay owned by contractTermHit (including the oracle
// hallucination gate).
func TestQualifiedSymbolTerm_UnqualifiedTermsUntouched(t *testing.T) {
	if qualifiedSymbolTermHit("anything at all", "RunWith") {
		t.Fatal("single-segment term must return false from qualifiedSymbolTermHit")
	}
	if !qualifiedSymbolTermHit("calls mod::Type::method here", "mod::Type::method") {
		t.Fatal(":: qualified verbatim spelling should hit via whole-token equality")
	}
	if !qualifiedSymbolTermHit("the method segment appears alone", "mod::Type::method") {
		t.Fatal("bare trailing segment of a :: qualified term should hit")
	}
}
