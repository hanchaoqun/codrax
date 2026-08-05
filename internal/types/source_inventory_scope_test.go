package types

import "testing"

func TestHasBoundedSourceEnumerationScope_TypedRequiredFilesOnly(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SourceScopeProfile: &SourceScopeProfile{
			RequestedScope: SourceScopeProduction,
			Confidence:     0.9,
		},
	}
	files := []string{
		"src/alpha/a.py",
		"src/alpha/b.java",
		"src/alpha/c.ts",
		"src/alpha/d.kt",
		"src/alpha/e.proto",
		"src/alpha/f.cj",
	}

	if !HasBoundedSourceEnumerationScope(rm, files, "") {
		t.Fatal("typed category enumeration with many same-scope source files should be bounded")
	}
	if got := BoundedSourceEnumerationCommonScope(BoundedSourceEnumerationScopeFiles(rm, files, "")); got != "src/alpha" {
		t.Fatalf("common scope = %q, want src/alpha", got)
	}
}

func TestHasBoundedSourceEnumerationScope_RejectsScalarOrBroadRoot(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
			IsScalarAnswer:        true,
		},
	}
	files := []string{
		"a.py",
		"b.java",
		"c.ts",
		"d.kt",
		"e.proto",
		"f.cj",
	}
	if HasBoundedSourceEnumerationScope(rm, files, "") {
		t.Fatal("scalar answer shape must not activate bounded source inventory scope")
	}
	rm.Predicates.IsScalarAnswer = false
	if HasBoundedSourceEnumerationScope(rm, files, "") {
		t.Fatal("root-level file spread must not activate bounded source inventory scope")
	}
}

func TestIsTypedSourceEnumerationShape_ExcludesLookupScalars(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	if !IsTypedSourceEnumerationShape(rm) {
		t.Fatal("plain typed category enumeration should be a source enumeration shape")
	}
	rm.Predicates.IsCountQuestion = true
	if IsTypedSourceEnumerationShape(rm) {
		t.Fatal("count questions must not activate source enumeration shape")
	}
}

func TestIsTypedSourceEnumerationShape_SourceScopeBacksInventoryLane(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentEnumerate,
		AnalyzerHints: AnalyzerHints{Kind: string(ReqEnumeration)},
		SourceScopeProfile: &SourceScopeProfile{
			RequestedScope: SourceScopeAll,
			Confidence:     0.9,
		},
	}
	if !IsTypedSourceEnumerationShape(rm) {
		t.Fatal("source-scope-backed enumeration should preserve the inventory lane even when category predicate is missing")
	}

	rm.Predicates.IsRoleLocateLookup = true
	if IsTypedSourceEnumerationShape(rm) {
		t.Fatal("role lookup should not become a source-inventory lane")
	}
}

func TestSourceInventoryRequiresRepoWideLens_TypedScopeOnly(t *testing.T) {
	rm := RequestModel{
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRolePackage},
		},
		SourceScopeProfile: &SourceScopeProfile{RequestedScope: SourceScopeAll},
	}
	if !SourceInventoryRequiresRepoWideLens(rm) {
		t.Fatal("repo-wide source inventory should require the root lens before narrowing")
	}
	rm.SourceScopeProfile.RequestedScope = SourceScopeUnknown
	if !SourceInventoryRequiresRepoWideLens(rm) {
		t.Fatal("explicit unknown source scope should prefer root inventory over RequiredFiles-derived narrowing")
	}
	rm.SourceScopeProfile = nil
	if !SourceInventoryRequiresRepoWideLens(rm) {
		t.Fatal("missing source-scope profile should prefer root inventory over analyzer RequiredFiles-derived narrowing")
	}
	rm.SourceScopeProfile = &SourceScopeProfile{RequestedScope: SourceScopeProduction}
	if !SourceInventoryRequiresRepoWideLens(rm) {
		t.Fatal("production source scope without explicit auxiliary exclusion should still prefer repo-wide inventory")
	}
	rm.AnswerExclusionPolicy = &AnswerExclusionPolicy{
		IsExclusionRequested:   true,
		ExcludedCandidateRoles: []AnswerCandidateRole{AnswerCandidateRoleFixture, AnswerCandidateRoleExample},
	}
	if SourceInventoryRequiresRepoWideLens(rm) {
		t.Fatal("production source scope with typed auxiliary exclusion may use bounded typed scopes")
	}
	rm.SourceInventoryProfile = nil
	if SourceInventoryRequiresRepoWideLens(rm) {
		t.Fatal("non-source-inventory requests must not activate root inventory policy")
	}
}

func TestSourceInventoryRequiresRepoWideLens_RequestBoundPathScope(t *testing.T) {
	rm := RequestModel{
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
		},
		AnalyzerHints: AnalyzerHints{
			SourceInventoryRequestedPathScopes: []string{"internal/analysis/criterion"},
		},
	}
	if SourceInventoryRequiresRepoWideLens(rm) {
		t.Fatal("request-bound path scope must not be expanded to repo-wide")
	}
	if got := SourceInventoryRequestedPathScopes(rm); len(got) != 1 || got[0] != "internal/analysis/criterion" {
		t.Fatalf("requested path scopes = %#v", got)
	}

	rm.AnalyzerHints.SourceInventoryRequestedPathScopes = []string{".", "../outside"}
	if !SourceInventoryRequiresRepoWideLens(rm) {
		t.Fatal("invalid/root path carriers must not narrow repo-wide authority")
	}
}

func TestSourceInventoryRequiresRepoWideLens_ExactRequestedFileBoundaryWinsOverClass(t *testing.T) {
	rm := RequestModel{
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
		},
		SourceScopeProfile: &SourceScopeProfile{
			RequestedScope: SourceScopeProduction,
			SourceQuotes:   []string{"internal/types/evidence.go"},
		},
		AnalyzerHints: AnalyzerHints{RequiredFileHints: []RequiredFileHint{{
			Path:       "internal/types/evidence.go",
			Confidence: 0.82,
		}}},
	}
	if !SourceInventoryHasExactRequestedFileBoundary(rm) {
		t.Fatal("matching typed source quote and required file should form an exact boundary")
	}
	if SourceInventoryRequiresRepoWideLens(rm) {
		t.Fatal("exact requested file must not be widened to the repository source universe")
	}

	rm.SourceScopeProfile.RequestedScope = SourceScopeAll
	if SourceInventoryHasExactRequestedFileBoundary(rm) {
		t.Fatal("an explicit all-source scope must outrank a sampled exact-file quote")
	}
	if !SourceInventoryRequiresRepoWideLens(rm) {
		t.Fatal("an explicit all-source scope must retain repo-wide semantics")
	}
	rm.SourceScopeProfile.RequestedScope = SourceScopeProduction

	rm.SourceScopeProfile.SourceQuotes = []string{"production files"}
	if SourceInventoryHasExactRequestedFileBoundary(rm) {
		t.Fatal("a source-class phrase is not an exact file boundary")
	}
	if !SourceInventoryRequiresRepoWideLens(rm) {
		t.Fatal("production class scope must retain repo-wide semantics")
	}

	rm.SourceScopeProfile.SourceQuotes = []string{"internal/types/evidence.go"}
	rm.AnalyzerHints.RequiredFileHints = append(rm.AnalyzerHints.RequiredFileHints, RequiredFileHint{
		Path:       "internal/types/context.go",
		Confidence: 0.82,
	})
	if SourceInventoryHasExactRequestedFileBoundary(rm) {
		t.Fatal("a quote/hint set mismatch must fail closed")
	}
}

func TestSourceInventoryRequiresRepoWideLens_UserMentionedExactFileWinsWithoutClassProfile(t *testing.T) {
	rm := RequestModel{
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
		},
		AnalyzerHints: AnalyzerHints{
			RequiredFileHints: []RequiredFileHint{{
				Path:       "internal/types/evidence.go",
				Confidence: 0.95,
			}},
			MentionedEntities: []string{"evidence.go", "internal/types/evidence.go"},
		},
	}
	if !SourceInventoryHasExactRequestedFileBoundary(rm) {
		t.Fatal("deterministically user-mentioned required file should form an exact boundary without a source-class profile")
	}
	if SourceInventoryRequiresRepoWideLens(rm) {
		t.Fatal("nil source class must not widen a provenance-backed exact file")
	}

	rm.AnalyzerHints.RequiredFileHints = append(rm.AnalyzerHints.RequiredFileHints, RequiredFileHint{
		Path:       "internal/types/context.go",
		Confidence: 0.9,
	})
	if SourceInventoryHasExactRequestedFileBoundary(rm) {
		t.Fatal("every required file must have independent user-mentioned provenance")
	}
	if !SourceInventoryRequiresRepoWideLens(rm) {
		t.Fatal("an uncorroborated required hint must keep nil-class inventory repo-wide")
	}

	rm.AnalyzerHints.RequiredFileHints = rm.AnalyzerHints.RequiredFileHints[:1]
	rm.SourceScopeProfile = &SourceScopeProfile{RequestedScope: SourceScopeAll}
	if SourceInventoryHasExactRequestedFileBoundary(rm) {
		t.Fatal("explicit all-source scope must outrank exact mentioned-file provenance")
	}
}
