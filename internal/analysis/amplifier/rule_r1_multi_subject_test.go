package amplifier

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// makeRMWithSymbols builds a RequestModel with N TermSymbol entries
// at uniform confidence (no sharp gap) and a baseline Intent that
// passes R1's intent gate. Tests then mutate one slot per case to
// drive the rule down a specific gate path. RawRequest stays empty
// so red line #1 is structurally enforced — the test never feeds
// case-specific text to the amplifier.
func makeRMWithSymbols(surfaces ...string) types.RequestModel {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
	}
	for _, s := range surfaces {
		rm.TermGraph.Canonical = append(rm.TermGraph.Canonical, types.CanonicalTerm{
			Surface:    s,
			Kind:       types.TermSymbol,
			Confidence: 0.9,
		})
	}
	return rm
}

// TestR1_FiresOnMultiSubject is the happy path: multiple symbols,
// confidence-equal, intent=Explain, IsScalarAnswer=false → flip
// IsCategoryEnumeration to true.
func TestR1_FiresOnMultiSubject(t *testing.T) {
	rm := makeRMWithSymbols("Foo", "Bar", "Baz")
	got, obs := Amplify(rm)
	if !got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 should fire: IsCategoryEnumeration=%v, want true", got.Predicates.IsCategoryEnumeration)
	}
	if len(obs) != 1 || obs[0].Rule != "R1_multi_subject_predicate" {
		t.Errorf("expected 1 R1 observation, got %+v", obs)
	}
	if obs[0].Field != "predicates.is_category_enumeration" {
		t.Errorf("observation field = %q, want predicates.is_category_enumeration", obs[0].Field)
	}
	if obs[0].Before != "false" || obs[0].After != "true" {
		t.Errorf("observation before/after = %q/%q, want false/true", obs[0].Before, obs[0].After)
	}
}

// TestR1_NoFire_SingleSymbol covers gate #1: topSymbols < 2.
func TestR1_NoFire_SingleSymbol(t *testing.T) {
	rm := makeRMWithSymbols("OnlyOne")
	got, obs := Amplify(rm)
	if got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 must NOT fire on single symbol: IsCategoryEnumeration=true")
	}
	if len(obs) != 0 {
		t.Errorf("expected no observation, got %+v", obs)
	}
}

// TestR1_NoFire_SharpConfidenceGap covers gate #1's sharp-gap
// branch: with > 0.4 confidence delta the helper truncates to one,
// so the rule should not fire even though TWO TermSymbols exist.
func TestR1_NoFire_SharpConfidenceGap(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		TermGraph: types.TermGraph{
			Canonical: []types.CanonicalTerm{
				{Surface: "StrongHit", Kind: types.TermSymbol, Confidence: 1.0},
				{Surface: "WeakHit", Kind: types.TermSymbol, Confidence: 0.3},
			},
		},
	}
	got, _ := Amplify(rm)
	if got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 must NOT fire on sharp confidence gap (1.0 vs 0.3)")
	}
}

// TestR1_NoFire_IntentGate covers gate #3: intents outside the
// allow-list never trigger R1.
func TestR1_NoFire_IntentGate(t *testing.T) {
	denyIntents := []types.Intent{
		types.IntentEnumerate,   // already marks itself
		types.IntentConfigQuery, // scalar lane
		types.IntentReturnValue, // scalar lane
		types.IntentUnknown,     // LLM gave up — don't compound
	}
	for _, intent := range denyIntents {
		rm := makeRMWithSymbols("Foo", "Bar")
		rm.Intent = intent
		got, obs := Amplify(rm)
		if got.Predicates.IsCategoryEnumeration {
			t.Errorf("R1 must NOT fire when Intent=%s", intent)
		}
		if len(obs) != 0 {
			t.Errorf("Intent=%s: expected no observation, got %+v", intent, obs)
		}
	}
}

// TestR1_NoFire_AlreadyTrue covers red line #2: amplifier never
// overrides an LLM-emitted positive.
func TestR1_NoFire_AlreadyTrue(t *testing.T) {
	rm := makeRMWithSymbols("Foo", "Bar")
	rm.Predicates.IsCategoryEnumeration = true // LLM already filled
	got, obs := Amplify(rm)
	if !got.Predicates.IsCategoryEnumeration {
		t.Fatalf("R1 must not flip true→false")
	}
	for _, o := range obs {
		if o.Rule == "R1_multi_subject_predicate" {
			t.Errorf("R1 fired on an already-true model — red line #2 breach")
		}
	}
}

// TestR1_NoFire_ScalarAnswer covers gate #5: typed scalar-answer
// signal is incompatible with enumeration.
func TestR1_NoFire_ScalarAnswer(t *testing.T) {
	rm := makeRMWithSymbols("Foo", "Bar")
	rm.Predicates.IsScalarAnswer = true
	got, _ := Amplify(rm)
	if got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 must NOT fire when IsScalarAnswer=true")
	}
}

// TestR1_NoFire_AllSymbolsEmpty asserts the helper's input
// sanitisation: blank surfaces are dropped, so an all-blank graph
// has zero TermSymbols and the rule cannot fire.
func TestR1_NoFire_AllSymbolsEmpty(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		TermGraph: types.TermGraph{
			Canonical: []types.CanonicalTerm{
				{Surface: "   ", Kind: types.TermSymbol, Confidence: 0.9},
				{Surface: "", Kind: types.TermSymbol, Confidence: 0.9},
			},
		},
	}
	got, _ := Amplify(rm)
	if got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 must NOT fire when all TermSymbol surfaces are blank")
	}
}

// TestR1_NoFire_NonSymbolKinds asserts the helper's kind filter:
// TermConcept / TermConfig are not counted as symbols, so even at
// confidence-equal these alone do not trigger the rule.
func TestR1_NoFire_NonSymbolKinds(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		TermGraph: types.TermGraph{
			Canonical: []types.CanonicalTerm{
				{Surface: "concept-a", Kind: types.TermConcept, Confidence: 0.9},
				{Surface: "concept-b", Kind: types.TermConcept, Confidence: 0.9},
				{Surface: "cfg.key", Kind: types.TermConfig, Confidence: 0.9},
			},
		},
	}
	got, _ := Amplify(rm)
	if got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 must NOT fire when graph has only TermConcept / TermConfig — symbols missing")
	}
}

// TestR1_FiresOnTraceIntent verifies all three allow-listed intents
// (Explain / Trace / RootCause) actually drive R1 through.
func TestR1_FiresOnAllAllowedIntents(t *testing.T) {
	allowIntents := []types.Intent{
		types.IntentExplain,
		types.IntentTrace,
		types.IntentRootCause,
	}
	for _, intent := range allowIntents {
		rm := makeRMWithSymbols("Foo", "Bar")
		rm.Intent = intent
		got, _ := Amplify(rm)
		if !got.Predicates.IsCategoryEnumeration {
			t.Errorf("R1 should fire for Intent=%s", intent)
		}
	}
}

// TestR1_Idempotent re-asserts the package-level invariant after
// R1 lands: a second Amplify pass over the augmented output must
// be a no-op (the gate `if existing != zero { skip }` defends
// against re-fire).
func TestR1_Idempotent(t *testing.T) {
	rm := makeRMWithSymbols("Foo", "Bar", "Baz")
	first, obs1 := Amplify(rm)
	if len(obs1) == 0 {
		t.Fatalf("expected R1 to fire on first pass")
	}
	second, obs2 := Amplify(first)
	if !second.Predicates.IsCategoryEnumeration {
		t.Errorf("second pass dropped IsCategoryEnumeration=true")
	}
	if len(obs2) != 0 {
		t.Errorf("second pass re-fired R1 — idempotency broken: %+v", obs2)
	}
}
