package amplifier

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func collectR2Observations(obs []Observation) []Observation {
	var out []Observation
	for _, o := range obs {
		if o.Rule == "R2_typed_name_parity_subtopics" {
			out = append(out, o)
		}
	}
	return out
}

// TestR2_FiresOnSinglePrefixFamily_CrossComponent covers the
// safe-fire path: four stage entities sharing the prefix "Stage"
// in a CROSS-COMPONENT question (legitimately spans subsystems)
// should derive four SubTopics, one per stage. axis_collapse won't
// trigger because IsCrossComponent=true bypasses gate condition #2.
func TestR2_FiresOnSinglePrefixFamily_CrossComponent(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore", "StageExtract", "StageFinalize"},
		},
		Predicates: types.SemanticPredicates{
			IsCrossComponent: true, // bypasses axis_collapse alignment gate
		},
	}
	got, obs := Amplify(rm)
	r2 := collectR2Observations(obs)
	if len(r2) != 1 {
		t.Fatalf("expected 1 R2 observation, got %d (%+v)", len(r2), r2)
	}
	if len(got.SubTopics) != 4 {
		t.Errorf("expected 4 SubTopics derived, got %d (%+v)", len(got.SubTopics), got.SubTopics)
	}
	wantSurface := map[string]bool{"StageAnalyze": true, "StageExplore": true, "StageExtract": true, "StageFinalize": true}
	for _, st := range got.SubTopics {
		if !wantSurface[st.Summary] {
			t.Errorf("unexpected SubTopic surface %q", st.Summary)
		}
		if len(st.Entities) != 1 || st.Entities[0] != st.Summary {
			t.Errorf("SubTopic %q must have Entities=[%q], got %+v", st.Summary, st.Summary, st.Entities)
		}
	}
}

// TestR2_FiresOnMultiFamily covers the m1a shape: 6 entities
// split across 2 affix families (Agent*, emit_*) in a cross-
// component question. Every family member should yield one
// SubTopic.
func TestR2_FiresOnMultiFamily(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{
				"AgentExplorer", "AgentExtractor",
				"emit_analysis", "emit_evidence",
				"emit_answer_symbol", "emit_hypothesis_verdict",
			},
		},
		Predicates: types.SemanticPredicates{
			IsCrossComponent: true, // m1a-shape: spans subsystems
		},
	}
	got, obs := Amplify(rm)
	r2 := collectR2Observations(obs)
	if len(r2) != 1 {
		t.Fatalf("expected 1 R2 observation, got %+v", r2)
	}
	if len(got.SubTopics) != 6 {
		t.Errorf("expected 6 SubTopics (Agent*[2] + emit_*[4]), got %d (%+v)",
			len(got.SubTopics), got.SubTopics)
	}
}

// TestR2_NoFire_SingleComponent_SinglePrefixFamily is the
// counterpart of the previous test and the canonical qf_arch
// failure shape: single-component + single-axis enumeration with
// 4 same-prefix entities. R2 must NOT fire because the derived
// SubTopics would all share a domain and trigger axis_collapse.
func TestR2_NoFire_SingleComponent_SinglePrefixFamily(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore", "StageExtract", "StageFinalize"},
		},
		Predicates: types.SemanticPredicates{
			// After R1 flips IsCategoryEnumeration to true (because the
			// test input meets all R1 gates), R2 must skip because
			// IsCrossComponent=false.
			IsCrossComponent: false,
		},
	}
	got, obs := Amplify(rm)
	r2 := collectR2Observations(obs)
	if len(r2) != 0 {
		t.Errorf("R2 must NOT fire on single-component single-axis enumeration: %+v", r2)
	}
	if len(got.SubTopics) != 0 {
		t.Errorf("R2 must NOT derive SubTopics in this scenario, got %+v", got.SubTopics)
	}
}

// TestR2_NoFire_LLMAlreadyFilledSubTopics covers red line #2:
// amplifier never overrides an LLM-emitted SubTopics list.
func TestR2_NoFire_LLMAlreadyFilledSubTopics(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore", "StageExtract", "StageFinalize"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "user-supplied topic", Entities: []string{"X"}},
		},
	}
	got, obs := Amplify(rm)
	r2 := collectR2Observations(obs)
	if len(r2) != 0 {
		t.Errorf("R2 must NOT fire when SubTopics is already populated — red line #2 breach")
	}
	if len(got.SubTopics) != 1 || got.SubTopics[0].Summary != "user-supplied topic" {
		t.Errorf("R2 mutated user-supplied SubTopics: %+v", got.SubTopics)
	}
}

// TestR2_NoFire_SingleEntity covers gate #2.
func TestR2_NoFire_SingleEntity(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{Entities: []string{"OnlyOne"}},
	}
	got, obs := Amplify(rm)
	r2 := collectR2Observations(obs)
	if len(r2) != 0 {
		t.Errorf("R2 must NOT fire on single-entity input")
	}
	if len(got.SubTopics) != 0 {
		t.Errorf("expected no derived SubTopics, got %+v", got.SubTopics)
	}
}

// TestR2_NoFire_NoCommonAffix covers gate #3: entities with no
// shared 4-char prefix or suffix produce no qualifying group.
func TestR2_NoFire_NoCommonAffix(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"FooBar", "QuxBaz", "Whirlpool"},
		},
	}
	got, obs := Amplify(rm)
	r2 := collectR2Observations(obs)
	if len(r2) != 0 {
		t.Errorf("R2 must NOT fire when no entities share a 4-char affix")
	}
	if len(got.SubTopics) != 0 {
		t.Errorf("expected no SubTopics, got %+v", got.SubTopics)
	}
}

// TestR2_NoFire_AffixTooShort covers the minAffixLen guard:
// 3-char common prefix is below threshold and must not group.
func TestR2_NoFire_AffixTooShort(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"GetX", "GetY"}, // shared "Get" = 3 chars only
		},
	}
	got, obs := Amplify(rm)
	r2 := collectR2Observations(obs)
	if len(r2) != 0 {
		t.Errorf("R2 must NOT fire on 3-char shared affix (below 4-char threshold)")
	}
	if len(got.SubTopics) != 0 {
		t.Errorf("expected no SubTopics, got %+v", got.SubTopics)
	}
}

// TestR2_NoFire_ProseConcepts covers the isIdentifierLike filter:
// all-lowercase prose words ("stage", "agent") that pass the
// simple char-class check still get rejected because they have
// no separator (underscore / dot) and no uppercase letter.
func TestR2_NoFire_ProseConcepts(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"stage", "stages", "stagex"},
		},
	}
	got, obs := Amplify(rm)
	r2 := collectR2Observations(obs)
	if len(r2) != 0 {
		t.Errorf("R2 must NOT fire on lowercase prose words")
	}
	if len(got.SubTopics) != 0 {
		t.Errorf("expected no SubTopics, got %+v", got.SubTopics)
	}
}

// TestR2_FiresOnSnakeCase confirms snake_case identifiers do
// pass the isIdentifierLike filter even when fully lowercase.
func TestR2_FiresOnSnakeCase(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"emit_analysis", "emit_evidence", "emit_verdict"},
		},
	}
	got, _ := Amplify(rm)
	if len(got.SubTopics) != 3 {
		t.Errorf("expected 3 SubTopics for emit_* family, got %d (%+v)", len(got.SubTopics), got.SubTopics)
	}
}

// TestR2_GroupSizeUpperCap asserts that a group of 9+ entities is
// dropped (probably global pattern, not a question subtopic).
func TestR2_GroupSizeUpperCap(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{
				"TestFoo", "TestBar", "TestBaz", "TestQux", "TestWhirl",
				"TestEcho", "TestFox", "TestZulu", "TestYankee",
			}, // 9 Test* entities — over maxGroupSize of 8
		},
	}
	got, obs := Amplify(rm)
	r2 := collectR2Observations(obs)
	if len(r2) != 0 {
		t.Errorf("R2 must NOT fire on group of 9 (cap is 8)")
	}
	if len(got.SubTopics) != 0 {
		t.Errorf("expected no SubTopics, got %+v", got.SubTopics)
	}
}

// TestR2_Idempotent: a second pass over the augmented output must
// be a no-op because R2's red-line #1 (SubTopics empty) is no
// longer satisfied. Uses IsCrossComponent=true to pass the
// axis-collapse alignment gate so R2 actually fires on first pass.
func TestR2_Idempotent(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore", "StageExtract"},
		},
		Predicates: types.SemanticPredicates{
			IsCrossComponent: true, // bypass axis-collapse alignment gate
		},
	}
	first, obs1 := Amplify(rm)
	if len(collectR2Observations(obs1)) == 0 {
		t.Fatalf("expected R2 to fire on first pass")
	}
	second, obs2 := Amplify(first)
	r2second := collectR2Observations(obs2)
	if len(r2second) != 0 {
		t.Errorf("R2 fired on the second pass — idempotency broken: %+v", r2second)
	}
	if len(second.SubTopics) != len(first.SubTopics) {
		t.Errorf("second pass mutated SubTopics: %d → %d", len(first.SubTopics), len(second.SubTopics))
	}
}

// TestR2_NoFire_AxisCollapseAlignment_EnumerationSingleComponent
// covers gate #2 (Phase 3.2-fix): when LLM (or R1) marks the
// question as single-axis enumeration, R2 must not derive
// SubTopics from a same-affix family because the downstream
// axis_collapse gate will reject same-domain SubTopics under
// IsCategoryEnumeration. The right answer shape is empty
// SubTopics + finalizer ordered_list. Empirical: 2026-05-05
// qf_arch run-1 — 4 Stage* entities + LLM cat=true +
// !IsCrossComponent → R2 derived 4 same-domain SubTopics →
// axis_collapse → 4 retries exhausted → eval FAIL.
func TestR2_NoFire_AxisCollapseAlignment_EnumerationSingleComponent(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore", "StageExtract", "StageFinalize"},
		},
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      false,
		},
	}
	got, obs := Amplify(rm)
	if len(got.SubTopics) != 0 {
		t.Errorf("R2 must NOT fire when IsCategoryEnumeration=true && !IsCrossComponent " +
			"(would trigger axis_collapse downstream)")
	}
	for _, o := range obs {
		if o.Rule == "R2_typed_name_parity_subtopics" {
			t.Errorf("unexpected R2 firing: %+v", o)
		}
	}
}

func TestR2_NoFire_ScalarAnswerTypedPredicateWins(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentReturnValue,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"emit_analysis", "emit_answer_document"},
		},
		Predicates: types.SemanticPredicates{
			IsScalarAnswer: true,
		},
	}
	got, obs := Amplify(rm)
	if len(got.SubTopics) != 0 {
		t.Fatalf("R2 must not derive SubTopics when IsScalarAnswer=true, got %+v", got.SubTopics)
	}
	for _, o := range obs {
		if o.Rule == "R2_typed_name_parity_subtopics" {
			t.Fatalf("unexpected R2 firing for scalar answer: %+v", o)
		}
	}
}

// TestR2_FiresOnCrossComponentEnumeration verifies the escape
// clause: when IsCrossComponent=true the question legitimately
// spans subsystems, axis_collapse won't fire on single-domain
// SubTopics, so R2 may safely derive subtopics from an affix
// family. m1a-shape (cross-component multi-agent question).
func TestR2_FiresOnCrossComponentEnumeration(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"AgentExplorer", "AgentExtractor"},
		},
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      true,
		},
	}
	got, _ := Amplify(rm)
	if len(got.SubTopics) == 0 {
		t.Errorf("R2 should fire on cross-component enumeration (axis_collapse won't trigger)")
	}
}

// TestR2_DropsNonIdentifiers covers the case where AnalyzerHints
// contains a mix of identifiers and prose phrases. R2 should
// affix-group only the identifiers and emit a clean SubTopic set.
// Uses IsCrossComponent=true to bypass axis-collapse alignment.
func TestR2_DropsNonIdentifiers(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{
				"StageAnalyze", "StageExplore",
				"some prose phrase", "另一个 中文短语", "config",
			},
		},
		Predicates: types.SemanticPredicates{
			IsCrossComponent: true, // bypass axis-collapse alignment gate
		},
	}
	got, obs := Amplify(rm)
	r2 := collectR2Observations(obs)
	if len(r2) != 1 {
		t.Fatalf("expected 1 R2 observation, got %+v", r2)
	}
	if len(got.SubTopics) != 2 {
		t.Errorf("expected 2 Stage* SubTopics, got %d (%+v)", len(got.SubTopics), got.SubTopics)
	}
}
