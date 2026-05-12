package amplifier

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func collectR3Observations(obs []Observation) []Observation {
	var out []Observation
	for _, o := range obs {
		if o.Rule == "R3_typed_identifier_mustinclude" {
			out = append(out, o)
		}
	}
	return out
}

// TestR3_FiresOnEnumeration covers the canonical case: cat=true +
// 4 identifier-shaped entities → all 4 added to MustInclude, one
// observation emitted.
func TestR3_FiresOnEnumeration(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore", "StageExtract", "StageFinalize"},
		},
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	contract := types.AnswerContract{}
	obs := AmplifyPostCompile(rm, &contract)
	r3 := collectR3Observations(obs)
	if len(r3) != 1 {
		t.Fatalf("expected 1 R3 observation, got %d (%+v)", len(r3), r3)
	}
	if len(contract.MustInclude) != 4 {
		t.Errorf("expected 4 MustInclude entries, got %d (%+v)", len(contract.MustInclude), contract.MustInclude)
	}
	want := map[string]bool{"StageAnalyze": true, "StageExplore": true, "StageExtract": true, "StageFinalize": true}
	for _, m := range contract.MustInclude {
		if !want[m] {
			t.Errorf("unexpected MustInclude entry %q", m)
		}
	}
}

// TestR3_NoFire_NotEnumeration covers gate #1: when
// IsCategoryEnumeration=false the rule must not fire even with
// many identifier-shaped entities.
func TestR3_NoFire_NotEnumeration(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore", "StageExtract"},
		},
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: false,
		},
	}
	contract := types.AnswerContract{}
	obs := AmplifyPostCompile(rm, &contract)
	r3 := collectR3Observations(obs)
	if len(r3) != 0 {
		t.Errorf("R3 must NOT fire when IsCategoryEnumeration=false, got %+v", r3)
	}
	if len(contract.MustInclude) != 0 {
		t.Errorf("R3 must not modify MustInclude on no-fire, got %+v", contract.MustInclude)
	}
}

// TestR3_NoFire_EmptyEntities covers the empty-entity no-op path.
func TestR3_NoFire_EmptyEntities(t *testing.T) {
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
	}
	contract := types.AnswerContract{}
	obs := AmplifyPostCompile(rm, &contract)
	r3 := collectR3Observations(obs)
	if len(r3) != 0 {
		t.Errorf("R3 must NOT fire on empty entity list, got %+v", r3)
	}
}

// TestR3_DropsProseEntities covers gate #3: the isIdentifierLike
// filter rejects lowercase prose words. Mixed input must result in
// only identifier-shaped entries pinned.
func TestR3_DropsProseEntities(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{
				"StageAnalyze", "StageExplore",
				"stage", "agent", "概念性短语", "config",
			},
		},
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
	}
	contract := types.AnswerContract{}
	obs := AmplifyPostCompile(rm, &contract)
	r3 := collectR3Observations(obs)
	if len(r3) != 1 {
		t.Fatalf("expected 1 R3 observation, got %+v", r3)
	}
	if len(contract.MustInclude) != 2 {
		t.Errorf("expected 2 identifier-shaped MustInclude entries, got %d (%+v)",
			len(contract.MustInclude), contract.MustInclude)
	}
}

func TestR3_PinsLowercaseMembersForExhaustiveEnumeration(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{
				"aggregator",
				"compiler",
				"findings_validator",
				"com.example.api",
				"react-dom",
				"@scope/pkg",
				"foo::bar",
				"packages/core",
			},
		},
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		CompletenessObligation: &types.CompletenessObligation{
			Required:    true,
			SourceQuote: "all packages",
		},
	}
	contract := types.AnswerContract{}
	obs := AmplifyPostCompile(rm, &contract)
	r3 := collectR3Observations(obs)
	if len(r3) != 1 {
		t.Fatalf("expected 1 R3 observation, got %+v", r3)
	}
	if len(contract.MustInclude) != 8 {
		t.Fatalf("expected 8 cross-language package/module members to be pinned, got %d (%+v)",
			len(contract.MustInclude), contract.MustInclude)
	}
	kinds := map[string]types.ContractTermKind{}
	for _, term := range contract.MustIncludeTerms {
		kinds[term.Text] = term.Kind
	}
	for _, want := range []string{
		"aggregator",
		"compiler",
		"findings_validator",
		"com.example.api",
		"react-dom",
		"@scope/pkg",
		"foo::bar",
		"packages/core",
	} {
		found := false
		for _, got := range contract.MustInclude {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing pinned member %q in %+v", want, contract.MustInclude)
		}
		if kinds[want] != types.ContractTermFileStem {
			t.Fatalf("member %q kind=%q, want file_stem; terms=%+v", want, kinds[want], contract.MustIncludeTerms)
		}
	}
}

func TestR3_PinsLowercaseMembersWithRequiredFileHints(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"aggregator", "compiler"},
			RequiredFileHints: []types.RequiredFileHint{
				{Path: "internal/analysis/aggregator/aggregator.go", Confidence: 0.9},
				{Path: "internal/analysis/compiler/compile.go", Confidence: 0.9},
			},
		},
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
	}
	contract := types.AnswerContract{}
	obs := AmplifyPostCompile(rm, &contract)
	r3 := collectR3Observations(obs)
	if len(r3) != 1 {
		t.Fatalf("expected 1 R3 observation, got %+v", r3)
	}
	if len(contract.MustInclude) != 2 {
		t.Fatalf("expected required-file-hinted lowercase members to be pinned, got %+v", contract.MustInclude)
	}
	for _, term := range contract.MustIncludeTerms {
		if term.Kind != types.ContractTermFileStem {
			t.Fatalf("required-file-hinted member %q kind=%q, want file_stem", term.Text, term.Kind)
		}
	}
}

func TestR3_NoFire_RelationalEnumerationEntitiesAreSearchHints(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{
				"SubAgent",
				"ProposeSubAgents",
				"SubAgentRuntime",
				"SubExplorer",
				"RegisterDefaultSubAgents",
				"capability_surface",
			},
		},
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsRelationalLookup:    true,
		},
	}
	contract := types.AnswerContract{}
	obs := AmplifyPostCompile(rm, &contract)
	r3 := collectR3Observations(obs)
	if len(r3) != 0 {
		t.Fatalf("R3 must not hard-pin relation-shaped enumeration entities, got %+v", r3)
	}
	if len(contract.MustInclude) != 0 {
		t.Fatalf("relation target/helper entities must remain soft hints, got MustInclude=%+v", contract.MustInclude)
	}
	if len(contract.MustIncludeTerms) != 0 {
		t.Fatalf("relation target/helper entities must not become typed hard terms, got %+v", contract.MustIncludeTerms)
	}
}

// TestR3_DedupesAgainstExisting covers gate #4: entities already
// present in MustInclude must not be re-added.
func TestR3_DedupesAgainstExisting(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore", "StageExtract"},
		},
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
	}
	contract := types.AnswerContract{
		MustInclude: []string{"StageAnalyze"}, // already pinned by template
	}
	obs := AmplifyPostCompile(rm, &contract)
	r3 := collectR3Observations(obs)
	if len(r3) != 1 {
		t.Fatalf("expected 1 R3 observation, got %+v", r3)
	}
	if len(contract.MustInclude) != 3 {
		t.Errorf("expected MustInclude=3 (1 existing + 2 added), got %d (%+v)",
			len(contract.MustInclude), contract.MustInclude)
	}
	// Verify no duplicate of StageAnalyze was added.
	count := 0
	for _, m := range contract.MustInclude {
		if m == "StageAnalyze" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("StageAnalyze duplicated %d times; dedupe failed", count)
	}
}

// TestR3_DedupeIsCaseFolded covers the case-insensitive dedupe:
// "stageAnalyze" in entity list should be treated as duplicate of
// "StageAnalyze" already in MustInclude, even though the surface
// strings differ.
func TestR3_DedupeIsCaseFolded(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"stageAnalyze", "StageExplore"},
		},
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
	}
	contract := types.AnswerContract{
		MustInclude: []string{"StageAnalyze"},
	}
	obs := AmplifyPostCompile(rm, &contract)
	r3 := collectR3Observations(obs)
	if len(r3) != 1 {
		t.Fatalf("expected 1 R3 observation, got %+v", r3)
	}
	if len(contract.MustInclude) != 2 {
		t.Errorf("expected MustInclude=2 (StageAnalyze case-folded dedupe), got %d (%+v)",
			len(contract.MustInclude), contract.MustInclude)
	}
}

// TestR3_NoFire_AllProse covers gate #3 boundary: when every entity
// is prose, the rule must not fire and MustInclude stays untouched.
func TestR3_NoFire_AllProse(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"stage", "agent", "concept"},
		},
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
	}
	contract := types.AnswerContract{
		MustInclude: []string{"existing"},
	}
	obs := AmplifyPostCompile(rm, &contract)
	r3 := collectR3Observations(obs)
	if len(r3) != 0 {
		t.Errorf("R3 must NOT fire when no entities are identifier-shaped, got %+v", r3)
	}
	if len(contract.MustInclude) != 1 || contract.MustInclude[0] != "existing" {
		t.Errorf("MustInclude was modified despite no-fire, got %+v", contract.MustInclude)
	}
}

// TestR3_Idempotent: a second AmplifyPostCompile pass over the
// already-augmented contract must be a no-op (gate #4 dedupe).
func TestR3_Idempotent(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore"},
		},
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
	}
	contract := types.AnswerContract{}
	obs1 := AmplifyPostCompile(rm, &contract)
	if len(collectR3Observations(obs1)) == 0 {
		t.Fatalf("expected R3 to fire on first pass")
	}
	obs2 := AmplifyPostCompile(rm, &contract)
	if len(collectR3Observations(obs2)) != 0 {
		t.Errorf("R3 fired on second pass — idempotency broken: %+v", obs2)
	}
	if len(contract.MustInclude) != 2 {
		t.Errorf("second pass mutated MustInclude: %+v", contract.MustInclude)
	}
}

func TestR3_PopulatesTypedMustIncludeTerms(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"emit_evidence", "StageAnalyze"},
		},
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
	}
	contract := types.AnswerContract{}
	obs := AmplifyPostCompile(rm, &contract)
	if len(collectR3Observations(obs)) == 0 {
		t.Fatalf("expected R3 to fire")
	}
	if len(contract.MustIncludeTerms) != 2 {
		t.Fatalf("MustIncludeTerms len=%d, want 2: %+v", len(contract.MustIncludeTerms), contract.MustIncludeTerms)
	}
	kinds := map[string]types.ContractTermKind{}
	for _, term := range contract.MustIncludeTerms {
		kinds[term.Text] = term.Kind
	}
	if kinds["emit_evidence"] != types.ContractTermToolName {
		t.Fatalf("emit_evidence kind = %q, want tool_name; terms=%+v", kinds["emit_evidence"], contract.MustIncludeTerms)
	}
	if kinds["StageAnalyze"] != types.ContractTermSymbol {
		t.Fatalf("StageAnalyze kind = %q, want symbol; terms=%+v", kinds["StageAnalyze"], contract.MustIncludeTerms)
	}
}
