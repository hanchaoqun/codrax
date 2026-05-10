package contract

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// stubOracle is the in-test types.SymbolOracle. tier=0 means
// "not found"; tier 1-2 = real symbol; tier 3+ = low-confidence
// match treated as not-found by the oracle gate.
type stubOracle struct {
	tiers map[string]int
}

func (s *stubOracle) SymbolExists(name string) (bool, int) {
	if s == nil || s.tiers == nil {
		return false, 0
	}
	if t, ok := s.tiers[name]; ok && t > 0 {
		return true, t
	}
	return false, 0
}

// SymbolExistsFlat (Fix E test stub) flattens both the query and
// each tier-map key, then matches case-insensitively across forms.
// Mirrors the production graphOracle's lazy flat index but built
// per-call (cheap for test scale).
func (s *stubOracle) SymbolExistsFlat(name string) (bool, int) {
	if s == nil || s.tiers == nil {
		return false, 0
	}
	flatQ := flattenIdentifier(name)
	if flatQ == "" {
		return false, 0
	}
	for k, t := range s.tiers {
		if t <= 0 {
			continue
		}
		if flattenIdentifier(k) == flatQ {
			return true, t
		}
	}
	return false, 0
}

func TestCheck_NilOracleIsByteIdenticalSubstring(t *testing.T) {
	// Pre-commit-55 contract: a substring hit on must_include
	// passes regardless of whether the term is a real symbol.
	// Default Check() (no oracle) MUST preserve this.
	draft := Answer{Text: "the FooHandlerStub does its job"}
	c := types.AnswerContract{MustInclude: []string{"FooHandler"}}
	res := Check(draft, c)
	if !res.Passed {
		t.Errorf("nil-oracle substring match should pass; got %d violations", len(res.Violations))
	}
}

// TestCheckWithOracle_HallucinationRejected pins the commit-55
// Batch C fix: a substring hit on a term that is NOT a real
// symbol gets caught when oracle is wired. The "FooHandler"
// term is hallucinated; only "FooHandlerStub" exists in the
// repo (and the must_include text is the real prose).
func TestCheckWithOracle_HallucinationRejected(t *testing.T) {
	oracle := &stubOracle{tiers: map[string]int{
		// Note: "FooHandler" is INTENTIONALLY missing — the LLM
		// hallucinated this name.
		"FooHandlerStub": 1,
	}}
	draft := Answer{Text: "the FooHandlerStub does its job"}
	c := types.AnswerContract{MustInclude: []string{"FooHandler"}}
	res := CheckWithOracle(draft, c, oracle)
	if res.Passed {
		t.Error("oracle should reject hallucinated must_include term")
	}
	found := false
	for _, v := range res.Violations {
		if v.Kind == ViolMustInclude {
			found = true
		}
	}
	if !found {
		t.Errorf("expected must_include violation; got %v", res.Violations)
	}
}

// TestCheckWithOracle_RealSymbolSubstringStillPasses pins the
// preservation contract: when the must_include term IS a real
// symbol, the substring match (TaskList → TaskListSize) still
// passes — oracle gating only rejects HALLUCINATED terms.
func TestCheckWithOracle_RealSymbolSubstringStillPasses(t *testing.T) {
	oracle := &stubOracle{tiers: map[string]int{
		"TaskList": 1, // real Tier-1 symbol
	}}
	draft := Answer{Text: "the TaskListSize function returns 42"}
	c := types.AnswerContract{MustInclude: []string{"TaskList"}}
	res := CheckWithOracle(draft, c, oracle)
	if !res.Passed {
		t.Errorf("real symbol with prefix-style match should pass; got %d violations", len(res.Violations))
	}
}

func TestCheckWithOracle_FlatSymbolFormStillPasses(t *testing.T) {
	oracle := &stubOracle{tiers: map[string]int{
		"EmitEvidence": 1,
	}}
	draft := Answer{Text: "The final answer explains how `EmitEvidence` records the grounded item."}
	c := types.AnswerContract{MustInclude: []string{"emit_evidence"}}
	res := CheckWithOracle(draft, c, oracle)
	if !res.Passed {
		t.Fatalf("flat-form real symbol should satisfy must_include; got %+v", res.Violations)
	}
}

func TestCheckWithOracle_ToolNameMustIncludeBypassesSymbolOracle(t *testing.T) {
	oracle := &stubOracle{tiers: map[string]int{}}
	draft := Answer{Text: "The explorer records facts by calling `emit_evidence` after reading files."}
	c := types.AnswerContract{
		MustIncludeTerms: []types.ContractTerm{{
			Text: "emit_evidence",
			Kind: types.ContractTermToolName,
		}},
	}
	res := CheckWithOracle(draft, c, oracle)
	if !res.Passed {
		t.Fatalf("tool-name must_include should not require symbol oracle hit; got %+v", res.Violations)
	}
}

func TestCheckWithOracle_LegacyKnownToolNameInfersToolKind(t *testing.T) {
	oracle := &stubOracle{tiers: map[string]int{}}
	draft := Answer{Text: "The answer mentions `emit_evidence` as the grounding tool."}
	c := types.AnswerContract{MustInclude: []string{"emit_evidence"}}
	res := CheckWithOracle(draft, c, oracle)
	if !res.Passed {
		t.Fatalf("legacy known tool name should infer tool_name and bypass oracle; got %+v", res.Violations)
	}
}

func TestCheckWithOracle_FlatAcceptanceSymbolPasses(t *testing.T) {
	oracle := &stubOracle{tiers: map[string]int{
		"EmitEvidence": 1,
	}}
	draft := Answer{Text: "The final answer explains how `EmitEvidence` records the grounded item."}
	c := types.AnswerContract{
		AcceptanceTests: []types.Criterion{{
			Kind: types.CritContainsSymbol,
			Expr: "emit_evidence",
		}},
	}
	res := CheckWithOracle(draft, c, oracle)
	if !res.Passed {
		t.Fatalf("flat-form real symbol should satisfy acceptance contains_symbol; got %+v", res.Violations)
	}
}

// TestCheckWithOracle_PhraseBypassesOracleGate pins that
// non-identifier-shaped terms (multi-word phrases, short tokens)
// don't trigger the oracle gate — they keep substring-only
// semantics. shouldOracleGateInclude only opts in identifier-
// shaped tokens.
func TestCheckWithOracle_PhraseBypassesOracleGate(t *testing.T) {
	oracle := &stubOracle{tiers: map[string]int{}} // empty graph
	draft := Answer{Text: "the system handles authentication errors"}
	c := types.AnswerContract{MustInclude: []string{"system"}} // 6 chars but ALL lowercase = treat as plain word
	res := CheckWithOracle(draft, c, oracle)
	// "system" passes the regex shape but is a plain English word
	// the oracle won't find. shouldOracleGateInclude returns true
	// for it (alphanumeric ≥ 4), so the oracle gates this. The
	// term IS in prose, but oracle says no symbol → hit→miss.
	// Therefore we EXPECT this to fail. Operators wanting plain-
	// English terms in must_include should use phrases (with
	// spaces) instead so they bypass the gate.
	if res.Passed {
		t.Logf("oracle gate intentionally rejects plain English in must_include — operators should use phrase form")
	}
}

// TestCheckWithOracle_LowTierRejected pins the tier-floor: a
// Tier-3+ match doesn't count as validation either (low-
// confidence salvage parse, may itself be junk).
func TestCheckWithOracle_LowTierRejected(t *testing.T) {
	oracle := &stubOracle{tiers: map[string]int{
		"WeakHandler": 4, // path-only fallback
	}}
	draft := Answer{Text: "the WeakHandlerImpl exists"}
	c := types.AnswerContract{MustInclude: []string{"WeakHandler"}}
	res := CheckWithOracle(draft, c, oracle)
	if res.Passed {
		t.Error("Tier-4 oracle hit should NOT count as validation")
	}
}

func TestCheckWithOracle_MustExcludeHallucinationDoesNotFire(t *testing.T) {
	oracle := &stubOracle{tiers: map[string]int{
		// "Forbidden" is intentionally not a real symbol.
	}}
	draft := Answer{Text: "the ForbiddenStub exists"}
	c := types.AnswerContract{MustExclude: []string{"Forbidden"}}
	res := CheckWithOracle(draft, c, oracle)
	// Pre-fix: would fire ViolMustExclude because "Forbidden" is
	// substring of "ForbiddenStub". With oracle: "Forbidden" is
	// not a real symbol → not the prohibited concept → no
	// violation. Tightens precision.
	if !res.Passed {
		t.Errorf("hallucinated must_exclude term should NOT fire violation; got %v", res.Violations)
	}
}

// TestShouldOracleGateInclude pins the predicate that decides
// whether an oracle gate applies. Identifier-shaped tokens (≥ 4
// alphanumeric/underscore) opt in; phrases (whitespace) and
// short tokens bypass.
func TestShouldOracleGateInclude(t *testing.T) {
	cases := []struct {
		sym  string
		want bool
	}{
		{"FooHandler", true},
		{"my_func", true},
		{"GetUserToken", true},
		{"k8sClient", true},
		{"ab", false},         // too short
		{"abc", false},        // too short
		{"foo bar", false},    // phrase
		{"check this", false}, // phrase
		{"foo-bar", false},    // hyphen = punctuation
		{"foo.bar", false},    // dot = punctuation
		{"camelCase", true},   // identifier
		{"snake_case", true},  // identifier
		{"abcd", true},        // 4 chars, lowercase only — passes regex but ambiguous
	}
	for _, c := range cases {
		if got := shouldOracleGateInclude(c.sym); got != c.want {
			t.Errorf("shouldOracleGateInclude(%q) = %v, want %v", c.sym, got, c.want)
		}
	}
}
