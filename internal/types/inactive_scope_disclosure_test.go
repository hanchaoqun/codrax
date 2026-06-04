package types

import "testing"

func TestScopeDisclosureKind_EnumIntegrity(t *testing.T) {
	for _, k := range AllScopeDisclosureKinds() {
		if !k.IsValid() {
			t.Fatalf("AllScopeDisclosureKinds entry %q reports invalid", k)
		}
	}
	if !ScopeDisclosureUnknown.IsValid() {
		t.Fatalf("empty value must be valid")
	}
	if got, ok := NormalizeScopeDisclosureKind("not_a_real_value"); ok || got != ScopeDisclosureUnknown {
		t.Fatalf("invalid raw value must normalize to (Unknown,false), got (%q,%t)", got, ok)
	}
	if got, ok := NormalizeScopeDisclosureKind("  inactive_scope_named  "); !ok || got != ScopeDisclosureInactiveScopeNamed {
		t.Fatalf("trim+match failed: got (%q,%t)", got, ok)
	}
}

func TestBuildInactiveScopeDisclosureObligation_SingleRepoShortCircuits(t *testing.T) {
	// No PendingSubRepos → not required regardless of intent.
	busCtx := &BusContext{}
	doc := &AnswerDocumentV2{}
	got := BuildInactiveScopeDisclosureObligationFromBus(busCtx, doc)
	if got.Active() {
		t.Fatalf("single-repo posture must not activate obligation")
	}
}

func TestBuildInactiveScopeDisclosureObligation_AbsenceFires(t *testing.T) {
	busCtx := &BusContext{
		PendingSubRepos: []string{"repo-tools-py"},
		AnalysisIR: &AnalysisIR{
			RequestModel: RequestModel{
				Intent: IntentExplain,
				AnalyzerHints: AnalyzerHints{
					ExactTargets: []string{"process_request"},
				},
			},
		},
	}
	doc := &AnswerDocumentV2{
		ExactResolution: &AnswerExactResolution{
			Status: AnswerExactResolutionAbsent,
		},
	}
	got := BuildInactiveScopeDisclosureObligationFromBus(busCtx, doc)
	if !got.Active() {
		t.Fatalf("expected Active, got %+v", got)
	}
	if got.Reason != InactiveScopeReasonExactAbsentInActive {
		t.Fatalf("reason=%q, want %q", got.Reason, InactiveScopeReasonExactAbsentInActive)
	}
	if len(got.PendingRootRels) != 1 || got.PendingRootRels[0] != "repo-tools-py" {
		t.Fatalf("PendingRootRels=%+v", got.PendingRootRels)
	}
	if got.RequestedSubject != "process_request" {
		t.Fatalf("RequestedSubject=%q", got.RequestedSubject)
	}
}

func TestBuildInactiveScopeDisclosureObligation_ComparisonSkipsInactiveDisclosure(t *testing.T) {
	busCtx := &BusContext{
		PendingSubRepos: []string{"inactive-c"},
		AnalysisIR: &AnalysisIR{
			RequestModel: RequestModel{
				Intent:     IntentExplain,
				RawRequest: "compare repo-a and repo-b",
				Predicates: SemanticPredicates{IsCrossComponent: true},
				SubTopics: []SubTopic{
					{Summary: "a", Entities: []string{"repo-a"}},
					{Summary: "b", Entities: []string{"repo-b"}},
				},
				AnalyzerHints: AnalyzerHints{
					MentionedEntities: []string{"repo-a", "repo-b"},
					ExactTargets:      []string{"repo-a", "repo-b"},
				},
			},
		},
	}
	doc := &AnswerDocumentV2{
		ExactResolution: &AnswerExactResolution{Status: AnswerExactResolutionAbsent},
	}
	got := BuildInactiveScopeDisclosureObligationFromBus(busCtx, doc)
	if got.Active() {
		t.Fatalf("comparison answers must not require inactive-scope disclosure for unrelated pending repos: %+v", got)
	}
}

func TestBuildInactiveScopeDisclosureObligation_RoleLocateEmptyPrincipalFires(t *testing.T) {
	busCtx := &BusContext{
		PendingSubRepos: []string{"repo-tools-py"},
		AnalysisIR: &AnalysisIR{
			RequestModel: RequestModel{
				Intent: IntentExplain,
				Predicates: SemanticPredicates{
					IsRoleLocateLookup: true,
				},
				AnalyzerHints: AnalyzerHints{
					ExactTargets: []string{"process_request"},
				},
			},
		},
	}
	// Doc has no principal items.
	doc := &AnswerDocumentV2{
		Blocks: []AnswerBlock{
			{ID: "s1", Kind: BlockSummary, Text: "no match found"},
		},
	}
	got := BuildInactiveScopeDisclosureObligationFromBus(busCtx, doc)
	if !got.Active() {
		t.Fatalf("role-locate + empty principal must fire, got %+v", got)
	}
	if got.Reason != InactiveScopeReasonLocatePrincipalEmpty {
		t.Fatalf("reason=%q", got.Reason)
	}
}

func TestBuildInactiveScopeDisclosureObligation_PopulatedPrincipalSkips(t *testing.T) {
	busCtx := &BusContext{
		PendingSubRepos: []string{"repo-tools-py"},
		AnalysisIR: &AnalysisIR{
			RequestModel: RequestModel{
				Intent: IntentEnumerate,
				Predicates: SemanticPredicates{
					IsCategoryEnumeration: true,
				},
			},
		},
	}
	// Doc has principal items → not bounded.
	doc := &AnswerDocumentV2{
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockOrderedList,
			SurfaceRole: SurfacePrincipal,
			Items:       []AnswerBlockItem{{Label: "Foo", CitationRef: 0}},
		}},
	}
	got := BuildInactiveScopeDisclosureObligationFromBus(busCtx, doc)
	if got.Active() {
		t.Fatalf("populated principal slate must not activate obligation, got %+v", got)
	}
}

func TestBuildInactiveScopeDisclosureObligation_PrincipalScalarSkips(t *testing.T) {
	busCtx := &BusContext{
		PendingSubRepos: []string{"repo-tools-py"},
		AnalysisIR: &AnalysisIR{
			RequestModel: RequestModel{
				Intent: IntentExplain,
				Predicates: SemanticPredicates{
					IsRoleLocateLookup: true,
					IsScalarAnswer:     true,
				},
				AnalyzerHints: AnalyzerHints{
					ExactTargets: []string{"answer_value"},
				},
			},
		},
	}
	doc := &AnswerDocumentV2{
		Blocks: []AnswerBlock{{
			ID:          "value",
			Kind:        BlockScalar,
			SurfaceRole: SurfacePrincipal,
			Text:        "42",
		}},
	}
	got := BuildInactiveScopeDisclosureObligationFromBus(busCtx, doc)
	if got.Active() {
		t.Fatalf("principal scalar answer must not activate inactive-scope disclosure, got %+v", got)
	}
}

func TestBuildInactiveScopeDisclosureObligation_PrincipalSummaryExactMatchSkips(t *testing.T) {
	busCtx := &BusContext{
		PendingSubRepos: []string{"repo-tools-py"},
		AnalysisIR: &AnalysisIR{
			RequestModel: RequestModel{
				Intent: IntentExplain,
				Predicates: SemanticPredicates{
					IsRoleLocateLookup: true,
				},
			},
		},
	}
	doc := &AnswerDocumentV2{
		ExactResolution: &AnswerExactResolution{Status: AnswerExactResolutionExactMatch},
		Blocks: []AnswerBlock{{
			ID:          "summary",
			Kind:        BlockSummary,
			SurfaceRole: SurfacePrincipal,
			Text:        "repo-stub-rust/src/lib.rs:8 returns 42.",
		}},
	}
	got := BuildInactiveScopeDisclosureObligationFromBus(busCtx, doc)
	if got.Active() {
		t.Fatalf("principal exact-match summary must not activate inactive-scope disclosure, got %+v", got)
	}
}

func TestAnswerDocumentDisclosesInactiveScope_TypedFieldSatisfies(t *testing.T) {
	obligation := InactiveScopeDisclosureObligation{
		Required:        true,
		Reason:          InactiveScopeReasonExactAbsentInActive,
		PendingRootRels: []string{"repo-tools-py"},
	}
	doc := &AnswerDocumentV2{
		Blocks: []AnswerBlock{
			{ID: "c", Kind: BlockCaveat, Text: "ambiguous", ScopeDisclosure: ScopeDisclosureRequiresWorkspaceAdjust},
		},
	}
	if !AnswerDocumentDisclosesInactiveScope(doc, obligation) {
		t.Fatalf("typed ScopeDisclosure field must satisfy obligation")
	}
}

func TestAnswerDocumentDisclosesInactiveScope_NamedRootRelSatisfies(t *testing.T) {
	obligation := InactiveScopeDisclosureObligation{
		Required:        true,
		Reason:          InactiveScopeReasonExactAbsentInActive,
		PendingRootRels: []string{"repo-tools-py"},
	}
	doc := &AnswerDocumentV2{
		Blocks: []AnswerBlock{
			{ID: "c", Kind: BlockCaveat, Text: "process_request may live in repo-tools-py; please /repos focus to include it."},
		},
	}
	if !AnswerDocumentDisclosesInactiveScope(doc, obligation) {
		t.Fatalf("inline RootRel mention must satisfy obligation")
	}
}

func TestAnswerDocumentDisclosesInactiveScope_BasenameSatisfies(t *testing.T) {
	obligation := InactiveScopeDisclosureObligation{
		Required:        true,
		PendingRootRels: []string{"repo-tools-py"},
	}
	doc := &AnswerDocumentV2{
		Blocks: []AnswerBlock{
			{ID: "c", Kind: BlockCaveat, Text: "process_request is in tools-py."},
		},
	}
	if !AnswerDocumentDisclosesInactiveScope(doc, obligation) {
		t.Fatalf("basename form 'tools-py' must satisfy obligation when RootRel='repo-tools-py'")
	}
}

func TestAnswerDocumentDisclosesInactiveScope_NoDisclosureFails(t *testing.T) {
	obligation := InactiveScopeDisclosureObligation{
		Required:        true,
		PendingRootRels: []string{"repo-tools-py"},
	}
	doc := &AnswerDocumentV2{
		Blocks: []AnswerBlock{
			{ID: "s", Kind: BlockSummary, Text: "process_request does not exist in the active sub-repos."},
		},
	}
	if AnswerDocumentDisclosesInactiveScope(doc, obligation) {
		t.Fatalf("answer without RootRel name and without typed ScopeDisclosure must NOT satisfy obligation")
	}
}

func TestAnswerDocumentDisclosesInactiveScope_NonActiveObligationShortCircuits(t *testing.T) {
	obligation := InactiveScopeDisclosureObligation{Required: false}
	if AnswerDocumentDisclosesInactiveScope(&AnswerDocumentV2{}, obligation) {
		t.Fatalf("inactive obligation must never report satisfied")
	}
}
