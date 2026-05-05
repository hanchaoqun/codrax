package amplifier

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// makeRMWithEntities builds a RequestModel with N AnalyzerHints
// entries and a baseline Intent that passes R1's intent gate. Tests
// then mutate one slot per case to drive the rule down a specific
// gate path. RawRequest stays empty so red line #1 is structurally
// enforced — the test never feeds case-specific text to amplifier.
func makeRMWithEntities(entities ...string) types.RequestModel {
	return types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: append([]string(nil), entities...),
		},
	}
}

// TestR1_FiresOnMultiEntity is the happy path: multiple LLM-named
// entities, intent=Explain, IsScalarAnswer=false → flip
// IsCategoryEnumeration to true.
func TestR1_FiresOnMultiEntity(t *testing.T) {
	rm := makeRMWithEntities("Foo", "Bar", "Baz")
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

// TestR1_NoFire_SingleEntity covers gate #1: distinctEntityCount < 2.
func TestR1_NoFire_SingleEntity(t *testing.T) {
	rm := makeRMWithEntities("OnlyOne")
	got, obs := Amplify(rm)
	if got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 must NOT fire on single entity")
	}
	if len(obs) != 0 {
		t.Errorf("expected no observation, got %+v", obs)
	}
}

// TestR1_NoFire_DuplicateEntities covers gate #1's distinct-count
// branch: the same entity repeated N times is one subject, not N.
// Case-folding + trim mean "Foo" / "FOO" / "  Foo  " collapse to
// one key.
func TestR1_NoFire_DuplicateEntities(t *testing.T) {
	rm := makeRMWithEntities("Foo", "FOO", "  foo  ")
	got, _ := Amplify(rm)
	if got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 must NOT fire when entities collapse to one distinct subject")
	}
}

// TestR1_NoFire_BlankEntries asserts that blank entries are dropped
// from the count: an LLM that emits ["", " ", "Foo"] has only one
// real subject.
func TestR1_NoFire_BlankEntries(t *testing.T) {
	rm := makeRMWithEntities("Foo", "", "   ")
	got, _ := Amplify(rm)
	if got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 must NOT fire when only one entity survives blank-entry filter")
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
		rm := makeRMWithEntities("Foo", "Bar")
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
	rm := makeRMWithEntities("Foo", "Bar")
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
	rm := makeRMWithEntities("Foo", "Bar")
	rm.Predicates.IsScalarAnswer = true
	got, _ := Amplify(rm)
	if got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 must NOT fire when IsScalarAnswer=true")
	}
}

// TestR1_FiresOnAllAllowedIntents verifies all three allow-listed
// intents (Explain / Trace / RootCause) actually drive R1 through.
func TestR1_FiresOnAllAllowedIntents(t *testing.T) {
	allowIntents := []types.Intent{
		types.IntentExplain,
		types.IntentTrace,
		types.IntentRootCause,
	}
	for _, intent := range allowIntents {
		rm := makeRMWithEntities("Foo", "Bar")
		rm.Intent = intent
		got, _ := Amplify(rm)
		if !got.Predicates.IsCategoryEnumeration {
			t.Errorf("R1 should fire for Intent=%s", intent)
		}
	}
}

// TestR1_FiresOnQfArchShape covers the empirical case that drove
// the Phase 2.1-fix-2 redesign: a Chinese question where the LLM
// emits stage names as Entities even though the question text never
// mentions them. Pre-fix this returned with R1 inactive; post-fix
// R1 must flip IsCategoryEnumeration to true.
func TestR1_FiresOnQfArchShape(t *testing.T) {
	rm := makeRMWithEntities("StageAnalyze", "StageExplore", "StageExtract", "StageFinalize")
	got, obs := Amplify(rm)
	if !got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 should fire on the qf_arch shape: 4 entities + intent=explain + everything-false predicates")
	}
	r1Count := 0
	for _, o := range obs {
		if o.Rule == "R1_multi_subject_predicate" {
			r1Count++
		}
	}
	if r1Count != 1 {
		t.Fatalf("expected exactly 1 R1 observation, got %d (full obs: %+v)", r1Count, obs)
	}
}

// TestR1_NoFire_AxisCollapseAlignment_SingleComponentSubTopics
// covers gate #6 (Phase 3.2-fix): when the LLM has emitted ≥2
// SubTopics on a single-component question, R1 must not flip cat
// to true because that combo will trigger the downstream
// axis_collapse gate. Without this guard the analyzer enters a
// retry storm and the eval fails (empirical: 2026-05-05 qf_arch
// run-1 with R2 producing same-domain SubTopics).
func TestR1_NoFire_AxisCollapseAlignment_SingleComponentSubTopics(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"Foo", "Bar", "Baz"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "topic-1", Entities: []string{"Foo"}},
			{Summary: "topic-2", Entities: []string{"Bar"}},
		},
		Predicates: types.SemanticPredicates{
			IsCrossComponent: false, // single-component
		},
	}
	got, _ := Amplify(rm)
	if got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 must NOT fire when LLM has ≥2 SubTopics + !IsCrossComponent " +
			"(would trigger axis_collapse downstream)")
	}
}

// TestR1_FiresOnCrossComponentMultiSubTopic verifies that the
// IsCrossComponent=true escape clause works: when the LLM marked
// the question as cross-component, axis_collapse will not trigger
// regardless of SubTopic domain count, so R1 may fire safely. This
// is the m1a empirical shape (multi-axis question with explorer +
// extractor across packages).
func TestR1_FiresOnCrossComponentMultiSubTopic(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"AgentExplorer", "AgentExtractor", "TurnA"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "topic-1", Entities: []string{"AgentExplorer"}},
			{Summary: "topic-2", Entities: []string{"AgentExtractor"}},
		},
		Predicates: types.SemanticPredicates{
			IsCrossComponent: true, // m1a-shape: spans subsystems
		},
	}
	got, _ := Amplify(rm)
	if !got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 should fire on multi-axis cross-component question (axis_collapse won't trigger)")
	}
}

// TestR1_Idempotent re-asserts the package-level invariant after
// R1 lands: a second Amplify pass over the augmented output must
// be a no-op (the gate `if existing != zero { skip }` defends
// against re-fire).
func TestR1_Idempotent(t *testing.T) {
	rm := makeRMWithEntities("Foo", "Bar", "Baz")
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
