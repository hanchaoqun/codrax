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

// TestR2_FiresOnSinglePrefixFamily covers the qf_arch shape: four
// stage entities sharing the prefix "Stage" should derive four
// SubTopics, one per stage.
func TestR2_FiresOnSinglePrefixFamily(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore", "StageExtract", "StageFinalize"},
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

// TestR2_FiresOnMultiFamily covers the m1a shape: 8 entities
// split across 2 affix families (Agent*, emit_*). Every family
// member should yield one SubTopic.
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
// be a no-op because R2's gate #1 (SubTopics empty) is no longer
// satisfied.
func TestR2_Idempotent(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore", "StageExtract"},
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

// TestR2_DropsNonIdentifiers covers the case where AnalyzerHints
// contains a mix of identifiers and prose phrases. R2 should
// affix-group only the identifiers and emit a clean SubTopic set.
func TestR2_DropsNonIdentifiers(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{
				"StageAnalyze", "StageExplore",
				"some prose phrase", "另一个 中文短语", "config",
			},
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
