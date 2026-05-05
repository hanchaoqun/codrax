package amplifier

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestAmplify_EmptyRMIdentity asserts the Phase 1.1 invariant:
// with no rules registered, calling Amplify on any RequestModel
// returns the model unchanged and a nil observation slice. As
// rules land in Phase 2/3 this test stays meaningful by using a
// minimal RM that does not satisfy any rule's gate.
func TestAmplify_EmptyRMIdentity(t *testing.T) {
	rm := types.RequestModel{}
	got, obs := Amplify(rm)
	if !reflect.DeepEqual(got, rm) {
		t.Errorf("Amplify(empty) mutated the model:\n got=%+v\nwant=%+v", got, rm)
	}
	if obs != nil {
		t.Errorf("Amplify(empty) returned %d observations, want nil", len(obs))
	}
}

// TestAmplify_FullRMNonOverride asserts red line #2: a model with
// every optional slot already populated by the LLM must come back
// untouched. This is the property test that future rules will be
// written against — each new rule must guard `if existing != zero
// { skip }` and this test will catch a regression that drops the
// guard.
func TestAmplify_FullRMNonOverride(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsScalarAnswer:        false,
		},
		SubTopics: []types.SubTopic{
			{Summary: "topic-a", Entities: []string{"A"}},
			{Summary: "topic-b", Entities: []string{"B"}},
		},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"A", "B"},
		},
		TermGraph: types.TermGraph{
			Canonical: []types.CanonicalTerm{
				{Surface: "A", Kind: types.TermSymbol, Confidence: 0.9},
				{Surface: "B", Kind: types.TermSymbol, Confidence: 0.9},
			},
		},
	}
	got, obs := Amplify(rm)
	if !reflect.DeepEqual(got, rm) {
		t.Errorf("Amplify(fully-populated) overrode an LLM-filled slot:\n got=%+v\nwant=%+v", got, rm)
	}
	if obs != nil {
		t.Errorf("Amplify(fully-populated) returned %d observations, want nil", len(obs))
	}
}

// TestAmplify_Idempotent asserts red line #6 (pure function): a
// second call on the output of the first must be a no-op. This
// catches rules that are accidentally NOT additive — e.g. a rule
// that reads its own write and re-fires.
func TestAmplify_Idempotent(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		TermGraph: types.TermGraph{
			Canonical: []types.CanonicalTerm{
				{Surface: "Foo", Kind: types.TermSymbol, Confidence: 0.8},
				{Surface: "Bar", Kind: types.TermSymbol, Confidence: 0.7},
			},
		},
	}
	first, _ := Amplify(rm)
	second, obs2 := Amplify(first)
	if !reflect.DeepEqual(second, first) {
		t.Errorf("Amplify is not idempotent: second pass mutated the model")
	}
	if obs2 != nil {
		t.Errorf("Amplify second pass produced %d observations, want nil", len(obs2))
	}
}

// TestAmplifyPostCompile_NilContract asserts the nil-safety contract:
// callers may pass a nil *AnswerContract during a degraded compile
// path and amplifier must no-op rather than panic.
func TestAmplifyPostCompile_NilContract(t *testing.T) {
	obs := AmplifyPostCompile(types.RequestModel{}, nil)
	if obs != nil {
		t.Errorf("AmplifyPostCompile(nil contract) returned %d observations, want nil", len(obs))
	}
}

// TestAmplifyPostCompile_EmptyIdentity asserts the Phase 1.1
// invariant for the post-compile pass: with no rules registered, a
// pointer-mutated AnswerContract must come back unchanged.
func TestAmplifyPostCompile_EmptyIdentity(t *testing.T) {
	contract := types.AnswerContract{
		MustInclude: []string{"existing"},
		Language:    "go",
	}
	original := types.AnswerContract{
		MustInclude: []string{"existing"},
		Language:    "go",
	}
	obs := AmplifyPostCompile(types.RequestModel{}, &contract)
	if !reflect.DeepEqual(contract, original) {
		t.Errorf("AmplifyPostCompile(empty rules) mutated contract:\n got=%+v\nwant=%+v", contract, original)
	}
	if obs != nil {
		t.Errorf("AmplifyPostCompile(empty rules) returned %d observations, want nil", len(obs))
	}
}

// TestPureFunction_NoRawRequestRead is a structural assertion that
// the Amplify signature does not even ACCEPT a parameter capable of
// leaking RawRequest beyond what RequestModel itself carries. We
// re-state the contract here so that a future change adding a
// reader / ctx parameter shows up as a failing test rather than a
// silent red-line breach. The check uses reflection on the function
// type so the test fails at compile time if the signature shifts.
func TestPureFunction_NoRawRequestRead(t *testing.T) {
	// Amplify must take exactly one argument (RequestModel) and
	// return exactly two values (RequestModel, []Observation).
	fnType := reflect.TypeOf(Amplify)
	if got, want := fnType.NumIn(), 1; got != want {
		t.Errorf("Amplify must take 1 arg, got %d", got)
	}
	if got, want := fnType.NumOut(), 2; got != want {
		t.Errorf("Amplify must return 2 values, got %d", got)
	}
	rmType := reflect.TypeOf(types.RequestModel{})
	if fnType.In(0) != rmType {
		t.Errorf("Amplify arg 0 must be types.RequestModel, got %v", fnType.In(0))
	}
}
