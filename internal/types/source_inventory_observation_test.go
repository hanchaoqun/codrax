package types

import (
	"strings"
	"testing"
)

func TestSourceInventoryObservationFromAdvisory_NormalizesCountAndAmbiguity(t *testing.T) {
	advisory := SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"src"},
		Provenance:   []string{"repomap_graph"},
		Sets: []SourceInventoryAdvisorySet{{
			Role:     AnswerCandidateRolePackage,
			Complete: true,
			Candidates: []SourceInventoryAdvisoryCandidate{{
				Member:       "alpha",
				Key:          "src/alpha",
				SupportRef:   "src/alpha",
				SurfaceTerms: []string{"@Route"},
				File:         "src/alpha",
				Language:     "python",
				Attributes: []SourceInventoryAdvisoryAttribute{
					{Member: "run_alpha", Key: "run_alpha", SupportRef: "run_alpha: src/alpha/a.py:7", SurfaceTerms: []string{"@Entry"}, Role: AnswerCandidateRoleFunction, File: "src/alpha/a.py", Line: 7},
					{Member: "build_alpha", Key: "build_alpha", SupportRef: "build_alpha: src/alpha/build.py:11", Role: AnswerCandidateRoleFunction, File: "src/alpha/build.py", Line: 11},
				},
			}},
		}},
	}

	got := SourceInventoryObservationFromAdvisory(advisory)
	if !got.IsActive() || !got.AdvisoryOnly || !got.Complete {
		t.Fatalf("observation flags = %+v", got)
	}
	if len(got.Sets) != 1 || got.Sets[0].Count != 1 || len(got.Sets[0].Members) != 1 {
		t.Fatalf("observation set/count = %+v", got.Sets)
	}
	member := got.Sets[0].Members[0]
	if member.CoverageState != SourceInventoryCoverageObserved {
		t.Fatalf("member coverage = %q", member.CoverageState)
	}
	if strings.Join(member.SurfaceTerms, ",") != "@Route" {
		t.Fatalf("member surface terms not preserved: %+v", member.SurfaceTerms)
	}
	if len(member.Attributes) != 2 {
		t.Fatalf("attributes = %+v", member.Attributes)
	}
	if strings.Join(member.Attributes[0].SurfaceTerms, ",") != "@Entry" {
		t.Fatalf("attribute surface terms not preserved: %+v", member.Attributes[0].SurfaceTerms)
	}
	for _, attr := range member.Attributes {
		if attr.CoverageState != SourceInventoryCoverageAmbiguous ||
			attr.Ambiguity != "one_of_many_candidate_attributes" {
			t.Fatalf("attribute ambiguity not preserved: %+v", attr)
		}
	}
}

func TestSourceInventoryObservation_CloneAndMergePreservesCountInvariant(t *testing.T) {
	prior := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Lens:         []string{"members"},
		Page: &SourceInventoryObservationPage{
			Offset:     0,
			Limit:      1,
			Total:      2,
			Emitted:    1,
			NextCursor: "1",
		},
		Execution: &SourceInventoryExecutionState{
			AttributesDeferred: true,
		},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Count:    99,
			Members: []SourceInventoryObservationMember{{
				Name: "Run",
				Key:  "Run",
				Role: AnswerCandidateRoleFunction,
				File: "src/run.py",
				Line: 7,
			}},
		}},
	}
	current := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Lens:         []string{"count"},
		Page: &SourceInventoryObservationPage{
			Offset:     10,
			Limit:      5,
			Total:      20,
			Emitted:    5,
			NextCursor: "15",
		},
		Execution: &SourceInventoryExecutionState{
			Budgeted:                 true,
			CandidateBudgetTruncated: true,
		},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Members: []SourceInventoryObservationMember{{
				Name: "Build",
				Key:  "Build",
				Role: AnswerCandidateRoleFunction,
				File: "src/build.py",
				Line: 11,
			}},
		}},
	}

	cloned := CloneSourceInventoryObservation(prior)
	if cloned.Sets[0].Count != 1 {
		t.Fatalf("clone did not normalize count: %+v", cloned.Sets[0])
	}
	merged := MergeSourceInventoryObservation(prior, current)
	if len(merged.Sets) != 1 || merged.Sets[0].Count != 2 || len(merged.Sets[0].Members) != 2 {
		t.Fatalf("merge set/count = %+v", merged.Sets)
	}
	if len(merged.Lens) != 2 || merged.Lens[0] != "members" || merged.Lens[1] != "count" {
		t.Fatalf("merge lens order = %+v", merged.Lens)
	}
	if merged.Page == nil || merged.Page.NextCursor != "15" || merged.Page.Total != 20 {
		t.Fatalf("merge should preserve the latest typed page state: %+v", merged.Page)
	}
	if merged.Execution == nil || !merged.Execution.Budgeted || !merged.Execution.CandidateBudgetTruncated {
		t.Fatalf("merge should preserve typed execution budget state: %+v", merged.Execution)
	}
	cloned.Page.NextCursor = "mutated"
	cloned.Execution = &SourceInventoryExecutionState{}
	if prior.Page == nil || prior.Page.NextCursor != "1" || prior.Execution == nil || !prior.Execution.AttributesDeferred {
		t.Fatalf("clone mutation leaked into prior observation: page=%+v execution=%+v", prior.Page, prior.Execution)
	}
}

func TestSourceInventoryObservation_MergeKeepsSameFileSameNameDistinctLines(t *testing.T) {
	prior := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: true,
			Members: []SourceInventoryObservationMember{{
				Name:          "Cart",
				Role:          AnswerCandidateRoleType,
				File:          "src/cart/Cart.cj",
				Line:          30,
				SurfaceTerms:  []string{"extend", "extend Cart"},
				CoverageState: SourceInventoryCoverageObserved,
			}},
		}},
	}
	current := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: true,
			Members: []SourceInventoryObservationMember{{
				Name:          "Cart",
				Role:          AnswerCandidateRoleType,
				File:          "src/cart/Cart.cj",
				Line:          14,
				SurfaceTerms:  []string{"public class", "public class Cart"},
				CoverageState: SourceInventoryCoverageObserved,
			}},
		}},
	}

	merged := MergeSourceInventoryObservation(prior, current)
	if len(merged.Sets) != 1 || len(merged.Sets[0].Members) != 2 || merged.Sets[0].Count != 2 {
		t.Fatalf("same-name declarations on distinct lines must remain distinct rows: %+v", merged.Sets)
	}
	byLine := map[int]SourceInventoryObservationMember{}
	for _, member := range merged.Sets[0].Members {
		byLine[member.Line] = member
	}
	if strings.Contains(strings.Join(byLine[30].SurfaceTerms, "\n"), "public class") {
		t.Fatalf("line 30 extend row inherited class terms: %+v", byLine[30])
	}
	if strings.Contains(strings.Join(byLine[14].SurfaceTerms, "\n"), "extend") {
		t.Fatalf("line 14 class row inherited extend terms: %+v", byLine[14])
	}
}

func TestSourceInventoryObservation_MergeCompleteLensSupersedesEarlierTruncatedSameScope(t *testing.T) {
	prior := SourceInventoryObservation{
		Active:     true,
		Complete:   false,
		Scopes:     []string{"."},
		Provenance: []string{"repo_lens:tool_query", "repo_lens:candidate_budget_truncated"},
		Lens:       []string{"source_inventory", "count"},
		Page: &SourceInventoryObservationPage{
			Offset:     0,
			Limit:      1,
			Total:      2,
			Emitted:    1,
			NextCursor: "1",
			Complete:   false,
		},
		Execution: &SourceInventoryExecutionState{
			Budgeted:                 true,
			CandidateBudgetTruncated: true,
		},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: false,
			Total:    2,
			Members: []SourceInventoryObservationMember{{
				Name: "defaultHeader",
				Key:  "defaultHeader",
				Role: AnswerCandidateRoleFunction,
				File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets",
				Line: 8,
			}, {
				Name: "GlobalCard",
				Key:  "GlobalCard",
				Role: AnswerCandidateRoleFunction,
				File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets",
				Line: 26,
			}},
		}, {
			Role:     AnswerCandidateRoleMethod,
			Complete: false,
		}},
	}
	current := SourceInventoryObservation{
		Active:     true,
		Complete:   true,
		Scopes:     []string{"."},
		Provenance: []string{"repo_lens:tool_query"},
		Lens:       []string{"source_inventory", "count"},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Total:    2,
			Members: []SourceInventoryObservationMember{{
				Name: "defaultHeader",
				Key:  "defaultHeader",
				Role: AnswerCandidateRoleFunction,
				File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets",
				Line: 8,
			}, {
				Name: "GlobalCard",
				Key:  "GlobalCard",
				Role: AnswerCandidateRoleFunction,
				File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets",
				Line: 26,
			}},
		}},
	}

	merged := MergeSourceInventoryObservation(prior, current)
	if !merged.Complete {
		t.Fatalf("complete same-scope rerun should clear stale incomplete state: %+v", merged)
	}
	if merged.Page != nil {
		t.Fatalf("complete rerun should clear stale pagination debt: %+v", merged.Page)
	}
	if merged.Execution != nil && merged.Execution.CandidateBudgetTruncated {
		t.Fatalf("complete rerun should clear stale candidate-budget debt: %+v", merged.Execution)
	}
	var functionSet SourceInventoryObservationSet
	for _, set := range merged.Sets {
		if set.Role == AnswerCandidateRoleFunction {
			functionSet = set
			break
		}
	}
	if !functionSet.Complete || functionSet.Count != 2 {
		t.Fatalf("function set should be complete after covering rerun: %+v", functionSet)
	}
	if sourceInventoryCompletionObservationIncomplete(merged) {
		t.Fatalf("merged observation should not be completion-incomplete: %+v", merged)
	}
}

func TestSourceInventoryObservationFromAdvisory_PreservesCompleteZeroSetLens(t *testing.T) {
	obs := SourceInventoryObservationFromAdvisory(SourceInventoryAdvisory{
		Active:   true,
		Complete: true,
		Scopes:   []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources"},
		Provenance: []string{
			"repo_lens:tool_query",
		},
		Sets: []SourceInventoryAdvisorySet{{
			Role:     AnswerCandidateRoleType,
			Complete: true,
			Total:    0,
		}},
	})
	if !obs.IsActive() {
		t.Fatalf("complete zero source-inventory observation should stay active: %+v", obs)
	}
	if len(obs.Sets) != 1 || obs.Sets[0].Role != AnswerCandidateRoleType || !obs.Sets[0].Complete || obs.Sets[0].Count != 0 || obs.Sets[0].Total != 0 {
		t.Fatalf("complete zero set was not preserved: %+v", obs.Sets)
	}
	if len(obs.CompleteLenses) != 1 {
		t.Fatalf("complete zero set should produce a complete lens, got %+v", obs.CompleteLenses)
	}
	lens := obs.CompleteLenses[0]
	if lens.Role != AnswerCandidateRoleType || lens.Count != 0 || lens.Total != 0 {
		t.Fatalf("zero complete lens has wrong role/count: %+v", lens)
	}
	if len(lens.SourceClasses) != 1 || lens.SourceClasses[0] != SourcePathRoleThirdParty {
		t.Fatalf("zero complete lens should carry source class from scope, got %+v", lens.SourceClasses)
	}
}

func TestSourceInventoryObservation_MergePreservesScopedCompleteLensProof(t *testing.T) {
	prior := SourceInventoryObservation{
		Active:     true,
		Complete:   false,
		Scopes:     []string{"."},
		Provenance: []string{"repo_lens:tool_query", "repo_lens:candidate_budget_truncated"},
		Execution:  &SourceInventoryExecutionState{Budgeted: true, CandidateBudgetTruncated: true},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: false,
			Total:    200,
			Members: []SourceInventoryObservationMember{{
				Name:     "Run",
				Key:      "Run",
				Role:     AnswerCandidateRoleFunction,
				File:     "cmd/root.go",
				Line:     10,
				Language: "go",
			}},
		}},
	}
	current := SourceInventoryObservation{
		Active:     true,
		Complete:   true,
		Scopes:     []string{"internal/thirdparty/tree-sitter-arkts/corpus/sources"},
		Provenance: []string{"repo_lens:tool_query", "repo_lens:scopes"},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Members: []SourceInventoryObservationMember{{
				Name:     "GlobalCard",
				Key:      "GlobalCard",
				Role:     AnswerCandidateRoleFunction,
				File:     "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets",
				Line:     26,
				Language: "arkts",
			}},
		}},
	}

	merged := MergeSourceInventoryObservation(prior, current)
	if !sourceInventoryCompletionObservationIncomplete(merged) {
		t.Fatalf("merged observation should still remember broader incomplete root debt: %+v", merged)
	}
	found := false
	for _, lens := range merged.CompleteLenses {
		if lens.Role != AnswerCandidateRoleFunction {
			continue
		}
		if len(lens.Languages) == 1 && lens.Languages[0] == "arkts" &&
			len(lens.SourceClasses) == 1 && lens.SourceClasses[0] == SourcePathRoleThirdParty {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("complete scoped ArkTS lens proof should survive role-level merge: %+v", merged.CompleteLenses)
	}
}

func TestSourceInventoryObservation_ClassUniverseCanBeActiveWithoutMemberRows(t *testing.T) {
	classOnly := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Lens:         []string{"source_class_universe"},
		SourceClasses: []SourceInventorySourceClassCount{{
			Role:     SourcePathRoleThirdParty,
			Count:    2,
			Complete: true,
		}},
	}
	got := CloneSourceInventoryObservation(classOnly)
	if !got.IsActive() || len(got.Sets) != 0 || len(got.SourceClasses) != 1 {
		t.Fatalf("class universe observation should stay active without member rows: %+v", got)
	}

	withMembers := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Members: []SourceInventoryObservationMember{{
				Name: "Run",
				Key:  "Run",
				Role: AnswerCandidateRoleFunction,
				File: "src/run.py",
			}},
		}},
	}
	merged := MergeSourceInventoryObservation(classOnly, withMembers)
	if !merged.IsActive() || len(merged.Sets) != 1 || len(merged.SourceClasses) != 1 {
		t.Fatalf("merge should preserve member rows and source classes: %+v", merged)
	}
	if merged.SourceClasses[0].Role != SourcePathRoleThirdParty || merged.SourceClasses[0].Count != 2 {
		t.Fatalf("source classes not preserved: %+v", merged.SourceClasses)
	}
}

func TestSourceInventoryObservation_SourceClassSamplesCloneMergeAndNormalize(t *testing.T) {
	prior := SourceInventoryObservation{
		Active: true,
		SourceClasses: []SourceInventorySourceClassCount{{
			Role:       SourcePathRoleThirdParty,
			Count:      1,
			Complete:   true,
			Samples:    []string{" internal/thirdparty/a.cj ", `internal\thirdparty\a.cj`, "internal/thirdparty/b.cj"},
			Provenance: []string{"repo_lens"},
		}},
	}
	current := SourceInventoryObservation{
		Active: true,
		SourceClasses: []SourceInventorySourceClassCount{{
			Role:       SourcePathRoleThirdParty,
			Count:      3,
			Complete:   true,
			Samples:    []string{"internal/thirdparty/b.cj", "internal/thirdparty/c.cj"},
			Provenance: []string{"repo_truth"},
		}},
	}

	cloned := CloneSourceInventoryObservation(prior)
	cloned.SourceClasses[0].Samples[0] = "mutated"
	if prior.SourceClasses[0].Samples[0] == "mutated" {
		t.Fatalf("source-class sample clone leaked into prior observation: %+v", prior.SourceClasses[0].Samples)
	}
	merged := MergeSourceInventoryObservation(prior, current)
	if len(merged.SourceClasses) != 1 {
		t.Fatalf("source classes = %+v", merged.SourceClasses)
	}
	got := strings.Join(merged.SourceClasses[0].Samples, ",")
	if got != "internal/thirdparty/a.cj,internal/thirdparty/b.cj,internal/thirdparty/c.cj" {
		t.Fatalf("merged samples = %q", got)
	}
	if merged.SourceClasses[0].Count != 3 {
		t.Fatalf("merged count = %d, want 3", merged.SourceClasses[0].Count)
	}
}

func TestSourceInventoryObservation_SourceClassCompletenessCanBeSuperseded(t *testing.T) {
	prior := SourceInventoryObservation{
		Active: true,
		SourceClasses: []SourceInventorySourceClassCount{{
			Role:     SourcePathRoleThirdParty,
			Count:    3,
			Complete: false,
			Samples:  []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/01_basic.cj"},
		}},
	}
	current := SourceInventoryObservation{
		Active: true,
		SourceClasses: []SourceInventorySourceClassCount{{
			Role:     SourcePathRoleThirdParty,
			Count:    3,
			Complete: true,
			Samples:  []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj"},
		}},
	}

	merged := MergeSourceInventoryObservation(prior, current)
	if len(merged.SourceClasses) != 1 || !merged.SourceClasses[0].Complete {
		t.Fatalf("equal-or-broader complete source-class census should supersede old incomplete row: %+v", merged.SourceClasses)
	}
}

func TestSourceInventoryCompletionObservationIncompleteIgnoresZeroCountRole(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"."},
		Lens:     []string{"source_inventory", "count"},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Members: []SourceInventoryObservationMember{{
				Name: "GlobalCard",
				Key:  "GlobalCard",
				Role: AnswerCandidateRoleFunction,
				File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets",
				Line: 26,
			}},
		}, {
			Role:     AnswerCandidateRoleMethod,
			Complete: false,
			Count:    0,
			Total:    0,
		}},
	}

	if sourceInventoryCompletionObservationIncomplete(obs) {
		t.Fatalf("zero-count incomplete role must not block completion: %+v", obs)
	}
	if got := sourceInventoryCompletionIncompleteSets(obs); len(got) != 0 {
		t.Fatalf("zero-count incomplete role should not surface as follow-up role debt: %+v", got)
	}
}

func TestSourceInventoryExactAbsenceNeedsInventoryProof(t *testing.T) {
	profile := &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
	}
	observation := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Lens:         []string{"source_class_universe", "count"},
		SourceClasses: []SourceInventorySourceClassCount{{
			Role:     SourcePathRoleThirdParty,
			Count:    1,
			Complete: true,
		}},
	}
	summary, blocked := SourceInventoryExactAbsenceNeedsInventoryProof(profile, observation)
	if !blocked || !strings.Contains(summary, "thirdparty:1") || !strings.Contains(summary, "function") {
		t.Fatalf("open source class universe should block exact absence, summary=%q blocked=%t", summary, blocked)
	}

	observation.Sets = []SourceInventoryObservationSet{{
		Role:     AnswerCandidateRoleFunction,
		Complete: true,
		Count:    0,
	}}
	summary, blocked = SourceInventoryExactAbsenceNeedsInventoryProof(profile, observation)
	if blocked || summary != "" {
		t.Fatalf("complete empty principal source-inventory set should close absence, summary=%q blocked=%t", summary, blocked)
	}
}

func TestSourceInventoryExactAbsenceNeedsInventoryProof_BlocksTruncatedZeroSet(t *testing.T) {
	profile := &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
	}
	closedZero := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Lens:         []string{"source_class_universe", "count"},
		SourceClasses: []SourceInventorySourceClassCount{{
			Role:     SourcePathRoleThirdParty,
			Count:    1,
			Complete: true,
		}},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
		}},
	}
	if summary, blocked := SourceInventoryExactAbsenceNeedsInventoryProof(profile, closedZero); blocked || summary != "" {
		t.Fatalf("complete untruncated zero set should close absence, summary=%q blocked=%t", summary, blocked)
	}

	for _, tc := range []struct {
		name string
		mut  func(*SourceInventoryObservation)
		want string
	}{
		{
			name: "observation incomplete",
			mut: func(obs *SourceInventoryObservation) {
				obs.Complete = false
			},
			want: "observation_incomplete",
		},
		{
			name: "candidate budget truncated",
			mut: func(obs *SourceInventoryObservation) {
				obs.Execution = &SourceInventoryExecutionState{
					Budgeted:                 true,
					CandidateBudgetTruncated: true,
				}
			},
			want: "candidate_budget_truncated",
		},
		{
			name: "page incomplete",
			mut: func(obs *SourceInventoryObservation) {
				obs.Page = &SourceInventoryObservationPage{
					Offset:     0,
					Limit:      10,
					Total:      11,
					Emitted:    10,
					NextCursor: "10",
					Complete:   false,
				}
			},
			want: "page_incomplete",
		},
		{
			name: "source class incomplete",
			mut: func(obs *SourceInventoryObservation) {
				obs.SourceClasses[0].Complete = false
			},
			want: "source_class_universe_incomplete",
		},
	} {
		obs := CloneSourceInventoryObservation(closedZero)
		tc.mut(&obs)
		summary, blocked := SourceInventoryExactAbsenceNeedsInventoryProof(profile, obs)
		if !blocked || !strings.Contains(summary, tc.want) {
			t.Fatalf("%s: truncated/incomplete zero set should not close absence, summary=%q blocked=%t", tc.name, summary, blocked)
		}
	}
}

func TestMutableState_SourceInventoryAdvisoryMaintainsObservation(t *testing.T) {
	mut := NewMutableState("source inventory")
	mut.SetSourceInventoryAdvisory(SourceInventoryAdvisory{
		Active: true,
		Sets: []SourceInventoryAdvisorySet{{
			Role: AnswerCandidateRoleFunction,
			Candidates: []SourceInventoryAdvisoryCandidate{{
				Member:     "Run",
				Key:        "Run",
				SupportRef: "Run: src/run.py:7",
				Role:       AnswerCandidateRoleFunction,
				File:       "src/run.py",
				Line:       7,
			}},
		}},
	})
	if got := mut.SourceInventoryObservation(); !got.IsActive() ||
		len(got.Sets) != 1 || got.Sets[0].Count != 1 ||
		got.Sets[0].Members[0].Name != "Run" {
		t.Fatalf("observation not maintained with advisory: %+v", got)
	}
	mut.ClearSourceInventoryAdvisory()
	if got := mut.SourceInventoryObservation(); got.IsActive() {
		t.Fatalf("clear advisory should clear observation: %+v", got)
	}
}

func TestTurnAArtifacts_SourceInventoryObservationBackfilledFromAdvisory(t *testing.T) {
	mut := NewMutableState("source inventory")
	mut.SetTurnAArtifacts(TurnAArtifacts{
		SourceInventoryAdvisory: SourceInventoryAdvisory{
			Active: true,
			Sets: []SourceInventoryAdvisorySet{{
				Role: AnswerCandidateRoleFile,
				Candidates: []SourceInventoryAdvisoryCandidate{{
					Member:     "src/run.py",
					Key:        "src/run.py",
					SupportRef: "src/run.py",
					Role:       AnswerCandidateRoleFile,
					File:       "src/run.py",
					Language:   "python",
				}},
			}},
		},
	})
	got := mut.TurnAArtifacts()
	if got == nil || !got.SourceInventoryObservation.IsActive() {
		t.Fatalf("turn A observation not backfilled: %+v", got)
	}
	if got.SourceInventoryObservation.Sets[0].Count != 1 ||
		got.SourceInventoryObservation.Sets[0].Members[0].Name != "src/run.py" {
		t.Fatalf("turn A observation = %+v", got.SourceInventoryObservation)
	}
}

func TestSourceInventoryObservationFromMutableReadsClosureUniverse(t *testing.T) {
	mut := NewMutableState("source inventory")
	mut.EvidenceClosure().RecordSourceInventoryObservation(SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Lens:     []string{"source_class_universe", "count"},
		SourceClasses: []SourceInventorySourceClassCount{{
			Role:       SourcePathRoleThirdParty,
			Count:      6,
			Complete:   true,
			Provenance: []string{"source_class_universe:git_tracked_or_filesystem"},
		}},
	})

	got := SourceInventoryObservationFromMutable(mut)
	if !got.IsActive() || len(got.SourceClasses) != 1 ||
		got.SourceClasses[0].Role != SourcePathRoleThirdParty ||
		got.SourceClasses[0].Count != 6 {
		t.Fatalf("merged observation should read closure source-class universe: %+v", got)
	}

	mut.ResetTurnAArtifacts()
	if got := SourceInventoryObservationFromMutable(mut); got.IsActive() {
		t.Fatalf("ResetTurnAArtifacts must clear closure-backed source inventory to prevent stale universe leakage: %+v", got)
	}
}

func TestSourceInventorySourceClassesCompleteRequiresPositiveKnownClasses(t *testing.T) {
	if SourceInventorySourceClassesComplete(nil) {
		t.Fatal("nil classes must not be complete")
	}
	if SourceInventorySourceClassesComplete([]SourceInventorySourceClassCount{{
		Role: SourcePathRoleUnknown, Count: 1, Complete: true,
	}}) {
		t.Fatal("unknown role must not make the universe complete")
	}
	if SourceInventorySourceClassesComplete([]SourceInventorySourceClassCount{{
		Role: SourcePathRoleProduction, Count: 0, Complete: true,
	}}) {
		t.Fatal("zero-count class must not make the universe complete")
	}
	if SourceInventorySourceClassesComplete([]SourceInventorySourceClassCount{{
		Role: SourcePathRoleProduction, Count: 2, Complete: false,
	}}) {
		t.Fatal("incomplete positive class must not be complete")
	}
	if !SourceInventorySourceClassesComplete([]SourceInventorySourceClassCount{{
		Role: SourcePathRoleProduction, Count: 2, Complete: true,
	}}) {
		t.Fatal("positive known complete class should complete the universe")
	}
}

func TestSourceInventoryLensExecutedSeparatesClassUniverseSeed(t *testing.T) {
	classOnly := SourceInventoryObservation{
		Active: true,
		Lens:   []string{"source_class_universe", "count"},
		SourceClasses: []SourceInventorySourceClassCount{{
			Role:     SourcePathRoleProduction,
			Count:    1,
			Complete: true,
		}},
	}
	if SourceInventoryLensExecuted(classOnly) {
		t.Fatalf("class-universe-only seed must not count as executable lens: %+v", classOnly)
	}
	toolQuery := classOnly
	toolQuery.Provenance = []string{"repo_lens:tool_query"}
	if !SourceInventoryLensExecuted(toolQuery) {
		t.Fatalf("repo_map source_inventory query should count as executable lens: %+v", toolQuery)
	}
	withMembers := classOnly
	withMembers.Sets = []SourceInventoryObservationSet{{
		Role:     AnswerCandidateRoleFunction,
		Complete: true,
		Count:    1,
		Members:  []SourceInventoryObservationMember{{Name: "Handle"}},
	}}
	if SourceInventoryLensExecuted(withMembers) {
		t.Fatalf("member-set observation without executable provenance must not count as lens execution: %+v", withMembers)
	}
	analyzePrescan := withMembers
	analyzePrescan.Provenance = []string{
		SourceInventoryProvenanceRepoLensToolQuery,
		SourceInventoryProvenanceStageAnalyze,
	}
	if SourceInventoryLensExecuted(analyzePrescan) {
		t.Fatalf("analyze-stage prescan must remain advisory and not suppress explore lens probe: %+v", analyzePrescan)
	}
	exploreLens := analyzePrescan
	exploreLens.Provenance = []string{
		SourceInventoryProvenanceRepoLensToolQuery,
		SourceInventoryProvenanceStageExplore,
	}
	if !SourceInventoryLensExecuted(exploreLens) {
		t.Fatalf("explore-stage source-inventory observation should count as executable lens: %+v", exploreLens)
	}
	listFilesSupport := withMembers
	listFilesSupport.Provenance = []string{"tool:list_files:direct", "repo_lens:query_roles"}
	if SourceInventoryLensExecuted(listFilesSupport) {
		t.Fatalf("list_files/path-discovery support rows must not suppress executable repo_map source_inventory lens: %+v", listFilesSupport)
	}
}
