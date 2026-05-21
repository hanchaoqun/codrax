package types

import (
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestNormalizeAnswerAggregateFacts_DedupesAndTrims(t *testing.T) {
	in := []AnswerAggregateFact{
		{
			Kind:  AnswerAggregateUniqueCount,
			Label: "  unique files  ",
			Value: " 2 ",
			Unit:  " files ",
			Dimensions: []AnswerAggregateDimension{
				{Name: " scope ", Value: " production "},
				{Name: "scope", Value: "production"},
			},
			Members: []string{" internal/a.go ", "internal/a.go", "internal/b.cpp"},
		},
		{
			Kind:  AnswerAggregateUniqueCount,
			Label: "unique files",
			Value: "2",
			Unit:  "files",
			Dimensions: []AnswerAggregateDimension{
				{Name: "scope", Value: "production"},
			},
			Members: []string{"internal/a.go", "internal/b.cpp"},
		},
	}
	got, err := NormalizeAnswerAggregateFacts(in)
	if err != nil {
		t.Fatalf("NormalizeAnswerAggregateFacts returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("deduped facts len=%d, want 1: %+v", len(got), got)
	}
	if got[0].Label != "unique files" || got[0].Value != "2" || got[0].Unit != "files" {
		t.Fatalf("fact not trimmed: %+v", got[0])
	}
	if len(got[0].Dimensions) != 1 || got[0].Dimensions[0].Name != "scope" || got[0].Dimensions[0].Value != "production" {
		t.Fatalf("dimensions not normalized: %+v", got[0].Dimensions)
	}
	if len(got[0].Members) != 2 || got[0].Members[1] != "internal/b.cpp" {
		t.Fatalf("members not normalized: %+v", got[0].Members)
	}
}

func TestNormalizeAnswerAggregateFacts_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		in   []AnswerAggregateFact
	}{
		{name: "kind", in: []AnswerAggregateFact{{Kind: "derived_guess", Label: "x", Value: "1"}}},
		{name: "label", in: []AnswerAggregateFact{{Kind: AnswerAggregateTotalCount, Value: "1"}}},
		{name: "value", in: []AnswerAggregateFact{{Kind: AnswerAggregateTotalCount, Label: "x"}}},
		{name: "count value unit drift", in: []AnswerAggregateFact{{Kind: AnswerAggregateTotalCount, Label: "x", Value: "3 files"}}},
		{name: "member cardinality", in: []AnswerAggregateFact{{Kind: AnswerAggregateBucketCount, Label: "runtime bucket", Value: "3", Members: []string{"a.go:1", "b.go:2"}}}},
		{name: "member set requires members", in: []AnswerAggregateFact{{Kind: AnswerAggregateMemberSet, Label: "enum types", Value: "2"}}},
		{name: "member set noninteger value", in: []AnswerAggregateFact{{Kind: AnswerAggregateMemberSet, Label: "enum types", Value: "two", Members: []string{"Intent"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeAnswerAggregateFacts(tc.in); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestNormalizeAnswerAggregateFacts_AcceptsMemberSet(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "public string enum types",
		Value:   "4",
		Unit:    "types",
		Members: []string{"Intent", "Scenario", "Complexity", "QuestionFamily"},
		SupportRefs: []string{
			"internal/types/analysis_ir.go:642",
			"internal/types/facet_plan.go:9",
		},
	}})
	if err != nil {
		t.Fatalf("member_set aggregate should validate: %v", err)
	}
	if len(got) != 1 || got[0].Kind != AnswerAggregateMemberSet || len(got[0].Members) != 4 {
		t.Fatalf("member_set not preserved: %+v", got)
	}
}

func TestNormalizeAnswerAggregateFacts_PreservesRoleAndProvenance(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:       AnswerAggregateMemberSet,
		Label:      "files inspected",
		Value:      "1",
		Role:       "coverage",
		Provenance: " command:rg ",
		Members:    []string{"internal/agent/explorer.go"},
	}})
	if err != nil {
		t.Fatalf("member_set aggregate should validate: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d facts, want 1", len(got))
	}
	if got[0].Role != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("role = %q, want supporting_coverage", got[0].Role)
	}
	if got[0].Provenance != "command:rg" {
		t.Fatalf("provenance = %q, want trimmed command:rg", got[0].Provenance)
	}
}

func TestEffectiveCitationFloorForPrincipalMemberSets_CapsSingletonExactSet(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
			IsRelationalLookup:    true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqEnumeration)},
	}
	facts := []AnswerAggregateFact{{
		Kind:        AnswerAggregateMemberSet,
		Label:       "default subagent names",
		Value:       "1",
		Role:        AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"explorer"},
		SupportRefs: []string{"internal/agent/sub_explorer.go:31"},
	}}

	got, ok := EffectiveCitationFloorForPrincipalMemberSets(3, facts, rm)
	if !ok || got != 1 {
		t.Fatalf("effective floor = %d,%v; want 1,true", got, ok)
	}
}

func TestEffectiveCitationFloorForPrincipalMemberSets_DoesNotCapSupportNarrativeSet(t *testing.T) {
	rm := &RequestModel{
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Complexity: ComplexityModerate,
		Predicates: SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: AnalyzerHints{
			Kind: string(ReqMechanism),
		},
	}
	facts := []AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "architecture components",
		Value:   "1",
		Role:    AnswerAggregateRoleSupportingCoverage,
		Members: []string{"explorer"},
	}}

	got, ok := EffectiveCitationFloorForPrincipalMemberSets(3, facts, rm)
	if ok || got != 3 {
		t.Fatalf("support narrative set must not cap floor, got %d,%v", got, ok)
	}
}

func TestNormalizeAnswerAggregateFacts_AcceptsNegativeSearch(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:  AnswerAggregateNegativeSearch,
		Label: "frameworks/base ressched interface search",
		Dimensions: []AnswerAggregateDimension{
			{Name: "repository", Value: "frameworks/base"},
			{Name: "search_pattern", Value: "ResSched|IRemoteBroker|SAMR"},
			{Name: "search_path", Value: "frameworks/base"},
			{Name: "searched_in", Value: "explore iteration 20"},
			{Name: "matches", Value: "0"},
		},
	}})
	if err != nil {
		t.Fatalf("negative_search aggregate should validate: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d facts, want 1", len(got))
	}
	fact := got[0]
	if fact.Kind != AnswerAggregateNegativeSearch || fact.Value != "0" || fact.Unit != "matches" {
		t.Fatalf("negative_search not normalized: %+v", fact)
	}
	dims := aggregateDimensionMap(fact.Dimensions)
	for _, name := range []string{"repo", "pattern", "scope", "searched_at", "result_count"} {
		if dims[name] == "" {
			t.Fatalf("missing canonical dimension %q in %+v", name, fact.Dimensions)
		}
	}
}

func TestNormalizeAnswerAggregateFacts_AcceptsNegativeObservation(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:  AnswerAggregateNegativeObservation,
		Label: "history search found no backport commits",
		Dimensions: []AnswerAggregateDimension{
			{Name: "evidence_origin", Value: "vcs_metadata"},
			{Name: "target", Value: "Backport Foo"},
			{Name: "commit_range", Value: "HEAD~50..HEAD"},
			{Name: "matches", Value: "0"},
		},
	}})
	if err != nil {
		t.Fatalf("negative_observation aggregate should validate: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d facts, want 1", len(got))
	}
	fact := got[0]
	if fact.Kind != AnswerAggregateNegativeObservation || fact.Value != "0" || fact.Unit != "matches" {
		t.Fatalf("negative_observation not normalized: %+v", fact)
	}
	dims := aggregateDimensionMap(fact.Dimensions)
	for _, name := range []string{"origin", "target", "commit_range", "scope", "searched_at", "result_count"} {
		if dims[name] == "" {
			t.Fatalf("missing canonical dimension %q in %+v", name, fact.Dimensions)
		}
	}
}

func TestNormalizeAnswerAggregateFacts_NormalizesNonRepoNegativeSearchToObservation(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:  AnswerAggregateNegativeSearch,
		Label: "log has no fatal error",
		Dimensions: []AnswerAggregateDimension{
			{Name: "origin", Value: "runtime_artifact"},
			{Name: "pattern", Value: "FATAL"},
			{Name: "artifact_id", Value: "attached-log"},
			{Name: "result_count", Value: "0"},
		},
	}})
	if err != nil {
		t.Fatalf("legacy non-repo negative_search should normalize: %v", err)
	}
	if len(got) != 1 || got[0].Kind != AnswerAggregateNegativeObservation {
		t.Fatalf("got %+v, want one negative_observation", got)
	}
}

func TestNormalizeAnswerAggregateFacts_RejectsNegativeObservationWithoutNonRepoOrigin(t *testing.T) {
	_, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:  AnswerAggregateNegativeObservation,
		Label: "missing symbol",
		Dimensions: []AnswerAggregateDimension{
			{Name: "origin", Value: "current_source"},
			{Name: "target", Value: "ShapeValue"},
			{Name: "scope", Value: "internal/types"},
			{Name: "result_count", Value: "0"},
		},
	}})
	if err == nil {
		t.Fatal("expected negative_observation without non-repo origin to reject")
	}
}

func TestMergeAnswerAggregateFacts_UnionsCompatibleMemberSets(t *testing.T) {
	got := MergeAnswerAggregateFacts(
		[]AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "public functions",
			Value:       "2",
			Role:        AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"Eval", "EvalAll"},
			SupportRefs: []string{"eval.go:15", "eval.go:36"},
		}},
		[]AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "public functions",
			Value:       "2",
			Role:        AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"EvalAll", "RegisteredKinds"},
			SupportRefs: []string{"eval.go:36", "grammar.go:106"},
		}},
	)
	if len(got) != 1 {
		t.Fatalf("merged facts = %+v, want one compatible member_set", got)
	}
	if got[0].Value != "3" {
		t.Fatalf("merged value = %q, want cardinality 3", got[0].Value)
	}
	wantMembers := []string{"Eval", "EvalAll", "RegisteredKinds"}
	if strings.Join(got[0].Members, ",") != strings.Join(wantMembers, ",") {
		t.Fatalf("merged members = %+v, want %+v", got[0].Members, wantMembers)
	}
	if len(got[0].SupportRefs) != 3 || got[0].SupportRefs[2] != "grammar.go:106" {
		t.Fatalf("support refs not aligned with merged members: %+v", got[0].SupportRefs)
	}
}

func TestMergeAnswerAggregateFacts_RemovesStaleCountQualifierFromMemberSetLabel(t *testing.T) {
	members := []string{
		"KindSymbolPresent", "KindNoCallSites", "KindAnswerSetBounded", "KindAnswerSetUnbounded",
		"KindMultipleResolutionChains", "KindUserClauseUnresolved", "KindUntrustedReachesSink",
		"KindInvariantBroken", "KindNoRelevantEvidence", "KindSignalPresent", "KindHasEnoughFacts",
		"KindAllHypothesesDecided", "KindContractSatisfied", "KindBudgetExhausted", "KindEvidenceCount",
		"KindCitationCountGE", "KindContainsSymbol", "KindRegexMatch", "KindCounterfactualBranchesDecided",
		"KindRelationAbsent", "KindPlanReady", "KindPatchApplies", "KindTestsPass", "KindNoRegression",
		"KindExternalArtifactDecoded",
	}
	got := MergeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "Kind const block 成员（读模式 20 个）",
		Value:   "20",
		Role:    AnswerAggregateRolePrincipalAnswer,
		Members: members,
	}})
	if len(got) != 1 {
		t.Fatalf("facts=%+v, want one", got)
	}
	if got[0].Value != "25" {
		t.Fatalf("value=%q, want 25", got[0].Value)
	}
	if got[0].Label != "Kind const block 成员" {
		t.Fatalf("stale count qualifier should be removed from display label, got %q", got[0].Label)
	}
	if normalizeAggregateMemberSetLabelCardinality("HTTP2 handlers", 3) != "HTTP2 handlers" {
		t.Fatal("semantic digits outside count qualifiers must be preserved")
	}
	if normalizeAggregateMemberSetLabelCardinality("Kind const block 全部成员（25个）", 25) != "Kind const block 全部成员（25个）" {
		t.Fatal("matching count qualifier should be preserved")
	}
}

func TestMergeAnswerAggregateFacts_KeepsDistinctMemberSetBuckets(t *testing.T) {
	got := MergeAnswerAggregateFacts(
		[]AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "public types",
			Value:   "1",
			Members: []string{"Kind"},
		}},
		[]AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "public functions",
			Value:   "1",
			Members: []string{"Eval"},
		}},
	)
	if len(got) != 2 {
		t.Fatalf("distinct labels must stay separate; got %+v", got)
	}
}

func TestMergeAnswerAggregateFacts_DemotesImplicitSupersetShadowedByExplicitPrincipal(t *testing.T) {
	got := MergeAnswerAggregateFacts(
		[]AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "functions",
			Value:   "4",
			Members: []string{"Eval", "EvalAll", "dispatch", "parseIntThreshold"},
		}},
		[]AnswerAggregateFact{{
			Kind:    AnswerAggregateGroupedCount,
			Label:   "public functions",
			Value:   "2",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Eval", "EvalAll"},
		}},
	)
	if len(got) != 2 {
		t.Fatalf("merged facts = %+v, want two facts", got)
	}
	if got[0].Role != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("implicit member_set superset should be supporting once explicit principal subset exists: %+v", got[0])
	}
	if got[1].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("explicit principal fact should stay principal: %+v", got[1])
	}
}

func TestMergeAnswerAggregateFacts_DemotesCompatiblePrincipalSubset(t *testing.T) {
	got := MergeAnswerAggregateFacts(
		[]AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "Kind 常量成员",
			Value:       "2",
			Role:        AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"KindSymbolPresent", "KindNoCallSites"},
			SupportRefs: []string{"grammar.go:29", "grammar.go:30"},
		}},
		[]AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "Kind常量完整清单",
			Value:       "3",
			Role:        AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"KindSymbolPresent", "KindNoCallSites", "KindAnswerSetBounded"},
			SupportRefs: []string{"grammar.go:29", "grammar.go:30", "grammar.go:31"},
		}},
	)

	if len(got) != 2 {
		t.Fatalf("facts len=%d want 2: %+v", len(got), got)
	}
	if got[0].Role != AnswerAggregateRoleSupportingCoverage ||
		!strings.Contains(got[0].Provenance, "demoted:shadowed_by_principal_member_set_superset") {
		t.Fatalf("smaller compatible set should be supporting: %+v", got[0])
	}
	if got[1].Role != AnswerAggregateRolePrincipalAnswer || got[1].Value != "3" {
		t.Fatalf("larger compatible set should remain principal: %+v", got[1])
	}
	refs := PrincipalAggregateMemberSetFactRefs(got)
	if len(refs) != 1 || len(refs[0].Fact.Members) != 3 {
		t.Fatalf("principal refs should expose only the larger set: %+v", refs)
	}
}

func TestMergeAnswerAggregateFacts_KeepsDistinctPrincipalSubsetBuckets(t *testing.T) {
	got := MergeAnswerAggregateFacts(
		[]AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "public types",
			Value:   "2",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Kind", "Env"},
		}},
		[]AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "public symbols",
			Value:   "3",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Kind", "Env", "Eval"},
		}},
	)

	if len(got) != 2 {
		t.Fatalf("facts len=%d want 2: %+v", len(got), got)
	}
	if got[0].Role != AnswerAggregateRolePrincipalAnswer || got[1].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("different structured buckets must remain principal: %+v", got)
	}
}

func TestNormalizeAnswerAggregateFacts_NegativeSearchRejectsNonZeroAndMissingQuery(t *testing.T) {
	cases := []struct {
		name string
		in   AnswerAggregateFact
	}{
		{
			name: "nonzero",
			in: AnswerAggregateFact{
				Kind:  AnswerAggregateNegativeSearch,
				Label: "frameworks/base ressched search",
				Value: "2",
				Dimensions: []AnswerAggregateDimension{
					{Name: "repo", Value: "frameworks/base"},
					{Name: "pattern", Value: "ResSched"},
					{Name: "scope", Value: "frameworks/base"},
				},
			},
		},
		{
			name: "missing query",
			in: AnswerAggregateFact{
				Kind:  AnswerAggregateNegativeSearch,
				Label: "frameworks/base ressched search",
				Value: "0",
				Dimensions: []AnswerAggregateDimension{
					{Name: "repo", Value: "frameworks/base"},
					{Name: "scope", Value: "frameworks/base"},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{tc.in}); err == nil {
				t.Fatalf("expected negative_search validation error")
			}
		})
	}
}

func TestPrincipalAggregateMemberSetFactRefsForRequest_DemotesPathCoverageForArchitecture(t *testing.T) {
	facts := []AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "files inspected",
			Value:   "2",
			Members: []string{"internal/agent/explorer.go", "internal/agent/agent.go"},
		},
		{
			Kind:    AnswerAggregateTotalCount,
			Label:   "core files",
			Value:   "2",
			Unit:    "files",
			Members: []string{"internal/agent/explorer.go", "internal/agent/agent.go"},
		},
	}
	arch := RequestModel{
		Intent:      IntentExplain,
		Scenario:    ScenarioArchitectureExplain,
		Complexity:  ComplexityComplex,
		Predicates:  SemanticPredicates{IsCrossComponent: true},
		DiagramHint: &DiagramHint{Kind: DiagramArchitecture},
	}
	if got := PrincipalAggregateMemberSetFactRefsForRequest(facts, &arch); len(got) != 0 {
		t.Fatalf("architecture path coverage set should not be principal, got %+v", got)
	}

	enum := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all files"},
	}
	if got := PrincipalAggregateMemberSetFactRefsForRequest(facts, &enum); len(got) != 2 {
		t.Fatalf("enumeration path set should remain principal, got %+v", got)
	}
}

func TestPrincipalAggregateMemberSetFactRefsForRequest_ExplicitRoleOverridesHeuristic(t *testing.T) {
	arch := RequestModel{
		Intent:      IntentExplain,
		Scenario:    ScenarioArchitectureExplain,
		Complexity:  ComplexityComplex,
		Predicates:  SemanticPredicates{IsCrossComponent: true},
		DiagramHint: &DiagramHint{Kind: DiagramArchitecture},
	}
	principal := []AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "files requested by the answer",
		Value:   "1",
		Role:    AnswerAggregateRolePrincipalAnswer,
		Members: []string{"internal/agent/explorer.go"},
	}}
	if got := PrincipalAggregateMemberSetFactRefsForRequest(principal, &arch); len(got) != 1 || got[0].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("explicit principal role should override path-coverage heuristic, got %+v", got)
	}

	support := []AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "supporting files",
		Value:   "1",
		Role:    AnswerAggregateRoleSupportingCoverage,
		Members: []string{"internal/agent/explorer.go"},
	}}
	enum := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all files"},
	}
	if got := PrincipalAggregateMemberSetFactRefsForRequest(support, &enum); len(got) != 0 {
		t.Fatalf("explicit supporting_coverage role must not satisfy principal enumeration, got %+v", got)
	}
}

func TestPrincipalAggregateMemberSetFactRefsForRequest_DemotesNarrativeGroupedCountsForArchitecture(t *testing.T) {
	facts := []AnswerAggregateFact{{
		Kind:    AnswerAggregateGroupedCount,
		Label:   "codrax 四层架构成员",
		Value:   "4",
		Unit:    "layers",
		Members: []string{"Ground (ground/ground.go)", "Gate (analysis/gate/gate.go)", "Reviewer (orchestrator/)", "Contract (orchestrator/contract_check.go)"},
	}}
	arch := RequestModel{
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Complexity: ComplexityComplex,
		Predicates: SemanticPredicates{IsCrossComponent: true},
	}
	if got := PrincipalAggregateMemberSetFactRefsForRequest(facts, &arch); len(got) != 0 {
		t.Fatalf("architecture grouped_count should remain support context, got %+v", got)
	}

	enum := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "全部架构成员"},
	}
	if got := PrincipalAggregateMemberSetFactRefsForRequest(facts, &enum); len(got) != 1 {
		t.Fatalf("enumeration grouped_count should remain principal, got %+v", got)
	}
}

func TestNormalizeAnswerAggregateFacts_CanonicalizesMemberSetValueFromMembers(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "pipeline stages",
		Unit:    "stages",
		Members: []string{"analyze", "explore", "extract", "finalize"},
	}})
	if err != nil {
		t.Fatalf("member_set without value should validate from structured members: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d facts, want 1: %+v", len(got), got)
	}
	if got[0].Value != "4" {
		t.Fatalf("member_set value = %q, want canonical len(members)=4", got[0].Value)
	}
	if len(got[0].Members) != 4 {
		t.Fatalf("members not preserved: %+v", got[0].Members)
	}
}

func TestNormalizeAnswerAggregateFacts_CanonicalizesNumericMemberSetValueDrift(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "pipeline stages",
		Value:   "3",
		Unit:    "stages",
		Members: []string{"analyze", "explore", "extract", "finalize"},
	}})
	if err != nil {
		t.Fatalf("member_set with numeric value drift should canonicalize from structured members: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d facts, want 1: %+v", len(got), got)
	}
	if got[0].Value != "4" {
		t.Fatalf("member_set value = %q, want canonical len(members)=4", got[0].Value)
	}
	if len(got[0].Members) != 4 {
		t.Fatalf("members not preserved: %+v", got[0].Members)
	}
}

func TestNormalizeAnswerAggregateFacts_DedupesEquivalentMemberSetDisplayVariants(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "subpackage directory and entry function",
			Value:   "3",
			Members: []string{"aggregator/Aggregate", "compiler/Compile", "Type::Member"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "all subpackages and entry functions",
			Value:   "3",
			Members: []string{"compiler: Compile", "aggregator -> Aggregate", "Type/Member"},
		},
	})
	if err != nil {
		t.Fatalf("equivalent member_set display variants should validate: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("equivalent member sets should dedupe to one fact, got %+v", got)
	}
	if got[0].Label != "subpackage directory and entry function" {
		t.Fatalf("dedupe should preserve first model-authored display fact, got %+v", got[0])
	}
	candidates := AnswerAggregateMemberDisplayCandidates("compiler: Compile")
	if !stringSliceContains(candidates, "compiler → Compile") || !stringSliceContains(candidates, "compiler/Compile") {
		t.Fatalf("colon relation display should expose equivalent renderings, got %+v", candidates)
	}
}

func TestNormalizeAnswerAggregateFacts_SeparatesMemberIdentityFromSupportLocation(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:  AnswerAggregateMemberSet,
		Label: "exported_types",
		Value: "3",
		Members: []string{
			"Kind: internal/analysis/criterion/grammar.go:26",
			"Kind",
			"Kind (grammar.go:26)",
			"Env: internal/analysis/criterion/grammar.go:124",
			"Env",
			"Result: internal/analysis/criterion/grammar.go:184",
			"Result",
		},
		SupportRefs: []string{
			"",
			"Kind @ internal/analysis/criterion/grammar.go:26",
			"",
			"",
			"Env @ internal/analysis/criterion/grammar.go:124",
			"",
			"Result @ internal/analysis/criterion/grammar.go:184",
		},
	}})
	if err != nil {
		t.Fatalf("member/support location variants should normalize: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d facts, want 1: %+v", len(got), got)
	}
	if got[0].Value != "3" || strings.Join(got[0].Members, ",") != "Kind,Env,Result" {
		t.Fatalf("support locations leaked into member identity: %+v", got[0])
	}
	if len(got[0].SupportRefs) != 3 ||
		got[0].SupportRefs[0] != "Kind @ internal/analysis/criterion/grammar.go:26" ||
		got[0].SupportRefs[1] != "Env @ internal/analysis/criterion/grammar.go:124" ||
		got[0].SupportRefs[2] != "Result @ internal/analysis/criterion/grammar.go:184" {
		t.Fatalf("support refs not aligned after identity normalization: %+v", got[0].SupportRefs)
	}
}

func TestNormalizeAnswerAggregateFacts_DedupesQualifiedRelationMemberSetVariants(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "subpackage directory and entry function",
			Value:   "3",
			Members: []string{"aggregator → Aggregate", "declarative → Classify", "hint → Compose"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "all subpackages and entry functions",
			Value:   "3",
			Members: []string{"hint → Composer.Compose", "aggregator → Aggregator.Aggregate", "declarative → Classifier.Classify"},
		},
	})
	if err != nil {
		t.Fatalf("qualified relation member variants should validate: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("qualified relation variants should dedupe to one fact, got %+v", got)
	}
	candidates := AnswerAggregateMemberDisplayCandidates("aggregator → Aggregator.Aggregate")
	if !stringSliceContains(candidates, "aggregator → Aggregate") ||
		!stringSliceContains(candidates, "aggregator/Aggregate") {
		t.Fatalf("qualified relation display should expose tail renderings, got %+v", candidates)
	}
}

func TestPrincipalAggregateMemberSetFactRefs_PrefersSupportedRelationAlternateSameLeftAxis(t *testing.T) {
	refs := PrincipalAggregateMemberSetFactRefs([]AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "subpackage entry functions",
			Value:   "2",
			Members: []string{"aggregator → Aggregator.Aggregate", "compiler → Compile"},
			SupportRefs: []string{
				"internal/analysis/aggregator/aggregator.go:132",
				"internal/analysis/compiler/compile.go:37",
			},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "internal/analysis package entries",
			Value:   "2",
			Members: []string{"aggregator.Aggregate", "compiler.Compile"},
		},
	})
	if len(refs) != 1 {
		t.Fatalf("unsupported alternate relation set with same left axis should be support-only, got %+v", refs)
	}
	if refs[0].Index != 0 || refs[0].Fact.Label != "subpackage entry functions" {
		t.Fatalf("complete support-ref relation set should remain principal, got %+v", refs[0])
	}
}

func TestPrincipalAggregateMemberSetFactRefs_KeepsFullySupportedSameLeftAxisSets(t *testing.T) {
	refs := PrincipalAggregateMemberSetFactRefs([]AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "subpackage entry functions",
			Value:   "2",
			Members: []string{"aggregator → Aggregator.Aggregate", "compiler → Compile"},
			SupportRefs: []string{
				"internal/analysis/aggregator/aggregator.go:132",
				"internal/analysis/compiler/compile.go:37",
			},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "subpackage constructors",
			Value:   "2",
			Members: []string{"aggregator → New", "compiler → New"},
			SupportRefs: []string{
				"internal/analysis/aggregator/aggregator.go:112",
				"internal/analysis/compiler/compiler.go:20",
			},
		},
	})
	if len(refs) != 2 {
		t.Fatalf("two fully supported relation attributes over the same left axis should both stay principal, got %+v", refs)
	}
}

func TestAnswerAggregateMemberDisplayCandidates_RelationDecoratorVariants(t *testing.T) {
	candidates := AnswerAggregateMemberDisplayCandidates("declarative → New (Classifier)")
	if !stringSliceContains(candidates, "declarative → New (Classifier)") ||
		!stringSliceContains(candidates, "declarative → New(Classifier)") ||
		!stringSliceContains(candidates, "declarative/New(Classifier)") {
		t.Fatalf("decorated relation member should expose spaced and compact displays, got %+v", candidates)
	}
	left, right, ok := AnswerAggregateMemberRelationParts("priority → Score(priority)")
	if !ok || left != "priority" || right != "Score(priority)" {
		t.Fatalf("decorated relation should parse as relation parts, got left=%q right=%q ok=%v", left, right, ok)
	}
	left, right, ok = AnswerAggregateMemberRelationParts("patcher → Engine (New + Submit/Apply)")
	if !ok || left != "patcher" || right != "Engine (New + Submit/Apply)" {
		t.Fatalf("decorated relation with qualifier list should parse, got left=%q right=%q ok=%v", left, right, ok)
	}
	candidates = AnswerAggregateMemberDisplayCandidates("patcher → Engine (New + Submit/Apply)")
	if !stringSliceContains(candidates, "patcher → Engine (New + Submit/Apply)") ||
		!stringSliceContains(candidates, "patcher → Engine(New + Submit/Apply)") ||
		!stringSliceContains(candidates, "patcher/Engine(New + Submit/Apply)") {
		t.Fatalf("decorated relation with slash qualifier should expose display variants, got %+v", candidates)
	}
	base, qualifier, ok := AnswerAggregateDecoratedLabelParts("Score (subject)")
	if !ok || base != "Score" || qualifier != "subject" {
		t.Fatalf("standalone decorated label should expose qualifier, got base=%q qualifier=%q ok=%v", base, qualifier, ok)
	}
}

func TestAnswerAggregateMemberDisplayCandidates_SourceLineDecoratorIsSupportSurface(t *testing.T) {
	candidates := AnswerAggregateMemberDisplayCandidates("aggregator → Aggregate (line 132)")
	if !stringSliceContains(candidates, "aggregator → Aggregate") ||
		!stringSliceContains(candidates, "aggregator/Aggregate") {
		t.Fatalf("source-line decorator should expose relation identity without the line support surface, got %+v", candidates)
	}
	if stringSliceContains(AnswerAggregateMemberDisplayCandidates("declarative → New (Classifier)"), "declarative → New") {
		t.Fatal("non-location relation decorators must remain identity-bearing")
	}
	if !stringSliceContains(AnswerAggregateMemberDisplayCandidates("prescan → ClassifyToken (行 29)"), "prescan → ClassifyToken") {
		t.Fatal("Chinese source-line decorators should also be treated as support surfaces")
	}
	if !stringSliceContains(AnswerAggregateMemberDisplayCandidates("compiler → Compile (lines 37-39)"), "compiler → Compile") {
		t.Fatal("source-line ranges should also be treated as support surfaces")
	}
	if !stringSliceContains(AnswerAggregateMemberDisplayCandidates("risk → Evaluate (L22)"), "risk → Evaluate") {
		t.Fatal("compact line decorators should also be treated as support surfaces")
	}
}

func TestAnswerAggregateMemberDisplayCandidates_CompactSourceSupportRef(t *testing.T) {
	candidates := AnswerAggregateMemberDisplayCandidates("analyzerEvaluator@internal/agent/analyzer.go:887")
	if !stringSliceContains(candidates, "analyzerEvaluator@internal/agent/analyzer.go:887") {
		t.Fatalf("compact support-ref member should preserve the model-authored surface, got %+v", candidates)
	}
	if !stringSliceContains(candidates, "analyzerEvaluator") {
		t.Fatalf("compact support-ref member should expose its principal label for downstream display, got %+v", candidates)
	}
}

func TestAnswerAggregateMemberDisplayCandidates_ViaSourceSupportRef(t *testing.T) {
	candidates := AnswerAggregateMemberDisplayCandidates("explorer (via SubExplorer.Name @ internal/agent/sub_explorer.go:31)")
	if !stringSliceContains(candidates, "explorer") {
		t.Fatalf("via support-ref member should expose its principal label for downstream display, got %+v", candidates)
	}
	if !stringSliceContains(candidates, "explorer (via SubExplorer.Name @ internal/agent/sub_explorer.go:31)") {
		t.Fatalf("via support-ref member should preserve the model-authored surface, got %+v", candidates)
	}
}

func TestAnswerAggregateMemberRelationParts_CompactDotAcrossSupportedLanguages(t *testing.T) {
	samples := map[string]string{
		repotypes.LangGo:         "compiler.Compile",
		repotypes.LangPython:     "module.run",
		repotypes.LangJavaScript: "Controller.handle",
		repotypes.LangTypeScript: "Service.resolve",
		repotypes.LangJava:       "Service.handle",
		repotypes.LangKotlin:     "Service.handle",
		repotypes.LangRust:       "crate::run",
		repotypes.LangC:          "module.init_module",
		repotypes.LangCpp:        "Parser::Parse",
		repotypes.LangRuby:       "Admin::call",
		repotypes.LangSwift:      "Service.start",
		repotypes.LangLua:        "M.render",
		repotypes.LangProto:      "UserService.ListUsers",
		repotypes.LangArkTS:      "EntryComponent.build",
		repotypes.LangCangjie:    "Service::run",
	}
	for _, lang := range repotypes.SupportedReadLanguages() {
		sample, ok := samples[lang]
		if !ok {
			t.Fatalf("missing compact relation sample for supported language %q", lang)
		}
		left, right, parsed := AnswerAggregateMemberRelationParts(sample)
		if !parsed {
			t.Fatalf("%s compact sample did not parse as relation member: %q", lang, sample)
		}
		if left == "" || right == "" {
			t.Fatalf("%s compact sample parsed empty relation part: left=%q right=%q", lang, left, right)
		}
	}
	for _, sample := range []string{"package.submodule.run", "internal/types/foo.go", "github.com/acme/pkg"} {
		if left, right, ok := AnswerAggregateMemberRelationParts(sample); ok {
			t.Fatalf("ambiguous/path-like compact surface %q must stay literal, got left=%q right=%q", sample, left, right)
		}
	}
	candidates := AnswerAggregateMemberDisplayCandidates("compiler.Compile")
	if !stringSliceContains(candidates, "compiler → Compile") ||
		!stringSliceContains(candidates, "compiler/Compile") {
		t.Fatalf("compact relation should expose split display candidates, got %+v", candidates)
	}
}

func TestAnswerAggregateMemberDisplayCandidates_AllSupportedLanguageQualifiers(t *testing.T) {
	samples := map[string]string{
		repotypes.LangGo:         "compiler → compiler.Compile",
		repotypes.LangPython:     "module → package.submodule.run",
		repotypes.LangJavaScript: "module → Controller.handle",
		repotypes.LangTypeScript: "service → App.Service.resolve",
		repotypes.LangJava:       "service → com.example.Service.handle",
		repotypes.LangKotlin:     "service → com.example.Service.handle",
		repotypes.LangRust:       "crate → crate::module::run",
		repotypes.LangC:          "module → init_module",
		repotypes.LangCpp:        "namespace → core::Parser::Parse",
		repotypes.LangRuby:       "module → Admin::Users.call",
		repotypes.LangSwift:      "module → App.Service.start",
		repotypes.LangLua:        "module → M.render",
		repotypes.LangProto:      "service → api.v1.UserService.ListUsers",
		repotypes.LangArkTS:      "component → EntryComponent.build",
		repotypes.LangCangjie:    "package → pkg::Service::run",
	}
	for _, lang := range repotypes.SupportedReadLanguages() {
		sample, ok := samples[lang]
		if !ok {
			t.Fatalf("missing aggregate relation qualifier sample for supported language %q", lang)
		}
		left, right, parsed := AnswerAggregateMemberRelationParts(sample)
		if !parsed {
			t.Fatalf("%s sample did not parse as relation member: %q", lang, sample)
		}
		if left == "" || right == "" {
			t.Fatalf("%s sample parsed empty relation part: left=%q right=%q", lang, left, right)
		}
		candidates := AnswerAggregateMemberDisplayCandidates(sample)
		if len(candidates) == 0 {
			t.Fatalf("%s sample produced no display candidates: %q", lang, sample)
		}
		if tail := NormalizedSurfaceSymbolTail(right); tail != "" && tail != strings.ToLower(right) {
			if !stringSliceContainsFold(candidates, left+" → "+tail) {
				t.Fatalf("%s sample should expose tail relation candidate %q, got %+v", lang, left+" → "+tail, candidates)
			}
		}
	}
}

func TestAnswerAggregateMemberRelationParts_AllSupportedLanguageSignatures(t *testing.T) {
	samples := map[string]struct {
		member string
		base   string
	}{
		repotypes.LangGo:         {"analysis → New(cfg Config) *Aggregator", "New"},
		repotypes.LangPython:     {"module -> run(config: Config) -> Result", "run"},
		repotypes.LangJavaScript: {"service → resolve(input): Promise<Result>", "resolve"},
		repotypes.LangTypeScript: {"service → resolve<T>(input: T): Promise<T>", "resolve"},
		repotypes.LangJava:       {"service → handle(Request req): Response", "handle"},
		repotypes.LangKotlin:     {"service → handle(request: Request): Response", "handle"},
		repotypes.LangRust:       {"crate -> run<T>(input: T) -> Result<()>", "run"},
		repotypes.LangC:          {"module → init_module(int argc, char **argv)", "init_module"},
		repotypes.LangCpp:        {"parser → Parser::Parse(std::string_view input) -> Result", "Parser::Parse"},
		repotypes.LangRuby:       {"service → call(payload)", "call"},
		repotypes.LangSwift:      {"service → start(request: Request) async throws -> Response", "start"},
		repotypes.LangLua:        {"module → render(ctx)", "render"},
		repotypes.LangProto:      {"service → ListUsers(ListUsersRequest) returns (ListUsersResponse)", "ListUsers"},
		repotypes.LangArkTS:      {"component → build(): void", "build"},
		repotypes.LangCangjie:    {"service → run(input: String): Result<Unit>", "run"},
	}
	for _, lang := range repotypes.SupportedReadLanguages() {
		sample, ok := samples[lang]
		if !ok {
			t.Fatalf("missing signature relation sample for supported language %q", lang)
		}
		left, right, parsed := AnswerAggregateMemberRelationParts(sample.member)
		if !parsed {
			t.Fatalf("%s signature sample did not parse as relation member: %q", lang, sample.member)
		}
		if left == "" || right == "" {
			t.Fatalf("%s signature sample parsed empty relation part: left=%q right=%q", lang, left, right)
		}
		candidates := AnswerAggregateMemberDisplayCandidates(sample.member)
		if !stringSliceContainsFold(candidates, left+" → "+sample.base) {
			t.Fatalf("%s signature sample should expose callable base candidate %q, got %+v", lang, left+" → "+sample.base, candidates)
		}
	}
}

func TestNormalizeAnswerAggregateFacts_DoesNotCollapseAmbiguousQualifiedRelationTails(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "overloaded handlers",
			Value:   "2",
			Members: []string{"api → HTTP.Handle", "api → GRPC.Handle"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "ambiguous handler surface",
			Value:   "1",
			Members: []string{"api → Handle"},
		},
	})
	if err != nil {
		t.Fatalf("ambiguous qualified relation members should still validate: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ambiguous qualified tails must not collapse into one member set, got %+v", got)
	}
}

func stringSliceContains(in []string, want string) bool {
	for _, got := range in {
		if got == want {
			return true
		}
	}
	return false
}

func stringSliceContainsFold(in []string, want string) bool {
	for _, got := range in {
		if strings.EqualFold(got, want) {
			return true
		}
	}
	return false
}

func TestNormalizeAnswerAggregateFacts_DoesNotCollapseDistinctPathMembers(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "production files",
			Value:   "1",
			Members: []string{"src/main.cpp"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "production files alternate",
			Value:   "1",
			Members: []string{"src → main.cpp"},
		},
	})
	if err != nil {
		t.Fatalf("path-like member sets should still validate: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("path-like and relation-like members must remain distinct, got %+v", got)
	}
}

func TestNormalizeAnswerAggregateFacts_NonMemberSetStillRequiresValue(t *testing.T) {
	_, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:    AnswerAggregateTotalCount,
		Label:   "pipeline stages",
		Members: []string{"analyze", "explore", "extract", "finalize"},
	}})
	if err == nil {
		t.Fatal("total_count without value must still reject; members may be samples")
	}
}

func TestNormalizeAnswerAggregateFacts_DerivesBucketCountValueFromMembers(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:    AnswerAggregateBucketCount,
		Label:   "runAnalyzePhase 成功条件",
		Unit:    "条件项",
		Members: []string{"err == nil", "out != nil", "out.Error == \"\"", "out.AnalysisIR != nil"},
	}})
	if err != nil {
		t.Fatalf("bucket_count with exact members should derive value: %v", err)
	}
	if len(got) != 1 || got[0].Value != "4" {
		t.Fatalf("bucket_count value = %+v, want derived value 4", got)
	}
}

func TestNormalizeAnswerAggregateFacts_AcceptsGroupedAndBucketCounts(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{
		{
			Kind:  AnswerAggregateGroupedCount,
			Label: "language=arkts",
			Value: "2",
			Unit:  "locations",
			Dimensions: []AnswerAggregateDimension{
				{Name: "language", Value: "ArkTS"},
				{Name: "bucket", Value: "runtime"},
			},
			Members: []string{"entry/src/main/ets/pages/Index.ets:12", "entry/src/main/ets/pages/Index.ets:18"},
		},
		{
			Kind:  AnswerAggregateBucketCount,
			Label: "native bucket",
			Value: "2",
			Unit:  "locations",
			Dimensions: []AnswerAggregateDimension{
				{Name: "language", Value: "C++"},
				{Name: "bucket", Value: "native"},
			},
			Members: []string{"src/native/foo.cpp:20", "src/native/foo.h:8"},
		},
		{
			Kind:  AnswerAggregateUniqueCount,
			Label: "native bucket files",
			Value: "2",
			Unit:  "files",
			Dimensions: []AnswerAggregateDimension{
				{Name: "bucket", Value: "native"},
			},
			Members: []string{"src/native/foo.cpp", "src/native/foo.h"},
		},
	})
	if err != nil {
		t.Fatalf("grouped/bucket aggregate facts should validate: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d facts, want 3: %+v", len(got), got)
	}
}

func TestNormalizeAnswerAggregateFacts_FileLineMembersRequireUniqueFileFact(t *testing.T) {
	_, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:  AnswerAggregateTotalCount,
		Label: "production locations",
		Value: "4",
		Members: []string{
			"internal/agent/analyzer.go:1903",
			"internal/orchestrator/contract_check.go:63",
			"internal/orchestrator/orchestrator.go:6362",
			"internal/orchestrator/orchestrator.go:6494",
		},
	}})
	if err == nil {
		t.Fatal("expected missing unique_count companion to reject")
	}

	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{
		{
			Kind:  AnswerAggregateTotalCount,
			Label: "production locations",
			Value: "4",
			Members: []string{
				"internal/agent/analyzer.go:1903",
				"internal/orchestrator/contract_check.go:63",
				"internal/orchestrator/orchestrator.go:6362",
				"internal/orchestrator/orchestrator.go:6494",
			},
		},
		{
			Kind:  AnswerAggregateUniqueCount,
			Label: "distinct files",
			Value: "3",
			Unit:  "files",
			Members: []string{
				"internal/agent/analyzer.go",
				"internal/orchestrator/contract_check.go",
				"internal/orchestrator/orchestrator.go",
			},
		},
	})
	if err != nil {
		t.Fatalf("unique_count companion should satisfy file-set aggregate: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d facts, want 2: %+v", len(got), got)
	}
}

func TestPrincipalAggregateMemberSetFactRefs_RelationSetSubsumesLeftAxis(t *testing.T) {
	facts := []AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "packages",
			Value:   "3",
			Members: []string{"aggregator", "declarative", "priority"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "package entry functions",
			Value:   "3",
			Members: []string{"aggregator → Aggregate", "declarative → New (Classifier)", "priority → Score(priority)"},
		},
	}

	got := PrincipalAggregateMemberSetFactRefs(facts)
	if len(got) != 1 {
		t.Fatalf("left-axis member set should be coverage context, got %+v", got)
	}
	if got[0].Index != 1 || got[0].Fact.Label != "package entry functions" {
		t.Fatalf("principal slate should preserve the richer relation fact and original index, got %+v", got[0])
	}
}

func TestPrincipalAggregateMemberSetFactRefs_DoesNotSuppressRightAxisOrDifferentDimensions(t *testing.T) {
	facts := []AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "entry functions",
			Value:   "2",
			Members: []string{"Aggregate", "Compile"},
		},
		{
			Kind:  AnswerAggregateMemberSet,
			Label: "production package entries",
			Value: "2",
			Dimensions: []AnswerAggregateDimension{{
				Name:  "scope",
				Value: "production",
			}},
			Members: []string{"aggregator → Aggregate", "compiler → Compile"},
		},
		{
			Kind:  AnswerAggregateMemberSet,
			Label: "test packages",
			Value: "2",
			Dimensions: []AnswerAggregateDimension{{
				Name:  "scope",
				Value: "tests",
			}},
			Members: []string{"aggregator", "compiler"},
		},
	}

	got := PrincipalAggregateMemberSetFactRefs(facts)
	if len(got) != 3 {
		t.Fatalf("right-axis and different-dimension sets should remain principal, got %+v", got)
	}
}

func TestProjectPrincipalAggregateFactsOntoCompleteAnswerSymbols_PrunesConflictingCountLedgersAtSource(t *testing.T) {
	facts := []AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "Kind constants",
			Value:   "2",
			Members: []string{"KindSymbolPresent", "KindNoCallSites"},
			SupportRefs: []string{
				"KindSymbolPresent: internal/analysis/criterion/grammar.go:29",
				"KindNoCallSites: internal/analysis/criterion/grammar.go:30",
			},
		},
		{
			Kind:    AnswerAggregateGroupedCount,
			Label:   "functions",
			Value:   "3",
			Members: []string{"Eval", "EvalAll", "parseIntThreshold"},
			SupportRefs: []string{
				"Eval: internal/analysis/criterion/eval.go:15",
				"EvalAll: internal/analysis/criterion/eval.go:36",
				"parseIntThreshold: internal/analysis/criterion/eval.go:190",
			},
		},
		{
			Kind:    AnswerAggregateGroupedCount,
			Label:   "internal evaluators",
			Value:   "2",
			Members: []string{"evalSymbolPresent", "evalNoCallSites"},
			SupportRefs: []string{
				"evalSymbolPresent: internal/analysis/criterion/eval.go:434",
				"evalNoCallSites: internal/analysis/criterion/eval.go:481",
			},
		},
	}
	symbols := []AnswerSymbol{
		{Name: "KindSymbolPresent"},
		{Name: "KindNoCallSites"},
		{Name: "Eval"},
		{Name: "EvalAll"},
	}

	got := ProjectPrincipalAggregateFactsOntoCompleteAnswerSymbols(
		facts,
		&RequestModel{Intent: IntentEnumerate},
		symbols,
		CompletenessComplete,
	)

	if len(got) != 3 {
		t.Fatalf("facts len=%d want 3", len(got))
	}
	if got[1].Value != "2" || strings.Join(got[1].Members, ",") != "Eval,EvalAll" {
		t.Fatalf("conflicting function aggregate was not projected onto complete answer symbols: %+v", got[1])
	}
	if len(got[1].SupportRefs) != 2 || strings.Contains(strings.Join(got[1].SupportRefs, ","), "parseIntThreshold") {
		t.Fatalf("support_refs should remain aligned with kept members: %+v", got[1].SupportRefs)
	}
	if got[2].Role != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("non-overlapping stale aggregate should be demoted to supporting coverage: %+v", got[2])
	}
}

func TestProjectPrincipalAggregateFactsOntoCompleteAnswerSymbols_PreservesExplicitPrincipalInventory(t *testing.T) {
	facts := []AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "Kind constants",
			Value:   "3",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"KindPlanReady", "KindPatchApplies", "KindTestsPass"},
			SupportRefs: []string{
				"KindPlanReady: internal/analysis/criterion/grammar.go:54",
				"KindPatchApplies: internal/analysis/criterion/grammar.go:55",
				"KindTestsPass: internal/analysis/criterion/grammar.go:56",
			},
		},
	}
	symbols := []AnswerSymbol{
		{Name: "KindPlanReady"},
		{Name: "KindTestsPass"},
	}

	got := ProjectPrincipalAggregateFactsOntoCompleteAnswerSymbols(
		facts,
		&RequestModel{Intent: IntentEnumerate},
		symbols,
		CompletenessComplete,
	)

	if len(got) != 1 {
		t.Fatalf("facts len=%d want 1", len(got))
	}
	if got[0].Role != AnswerAggregateRolePrincipalAnswer ||
		got[0].Value != "3" ||
		strings.Join(got[0].Members, ",") != "KindPlanReady,KindPatchApplies,KindTestsPass" {
		t.Fatalf("explicit principal member_set must not be pruned by a partial answer-symbol slate: %+v", got[0])
	}
	if len(got[0].SupportRefs) != 3 {
		t.Fatalf("support_refs should remain aligned with explicit principal members: %+v", got[0].SupportRefs)
	}
}

func TestProjectPrincipalAggregateFactsOntoCompleteAnswerSymbols_DemotesConflictingMemberSetNotCoveredByCompleteSymbols(t *testing.T) {
	facts := []AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "公开函数成员",
			Value:   "5",
			Members: []string{"Eval", "EvalAll", "IsRegistered", "RegisteredKinds", "parseIntThreshold"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "公开函数完整清单",
			Value:   "5",
			Members: []string{"Eval", "EvalAll", "IsRegistered", "RegisteredKinds", "SetExternalArtifactFloor"},
		},
	}
	symbols := []AnswerSymbol{
		{Name: "Eval"},
		{Name: "EvalAll"},
		{Name: "IsRegistered"},
		{Name: "RegisteredKinds"},
		{Name: "SetExternalArtifactFloor"},
	}

	got := ProjectPrincipalAggregateFactsOntoCompleteAnswerSymbols(
		facts,
		&RequestModel{Intent: IntentEnumerate},
		symbols,
		CompletenessComplete,
	)

	if got[0].Role != AnswerAggregateRoleSupportingCoverage ||
		!strings.Contains(got[0].Provenance, "demoted:conflicts_with_complete_answer_symbol_slate") {
		t.Fatalf("stale same-size member_set should be support: %+v", got[0])
	}
	if AnswerAggregateFactRoleForRequest(got[1], &RequestModel{Intent: IntentEnumerate}) != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("answer-symbol-covered member_set should stay principal: %+v", got[1])
	}
}

func TestProjectPrincipalAggregateFactsOntoCompleteAnswerSymbols_DemotesDuplicateProjectedSet(t *testing.T) {
	facts := []AnswerAggregateFact{
		{
			Kind:    AnswerAggregateGroupedCount,
			Label:   "functions",
			Value:   "2",
			Members: []string{"Eval", "EvalAll"},
		},
		{
			Kind:    AnswerAggregateGroupedCount,
			Label:   "internal helpers",
			Value:   "3",
			Members: []string{"Eval", "EvalAll", "parseIntThreshold"},
		},
	}
	symbols := []AnswerSymbol{{Name: "Eval"}, {Name: "EvalAll"}}

	got := ProjectPrincipalAggregateFactsOntoCompleteAnswerSymbols(
		facts,
		&RequestModel{Intent: IntentEnumerate},
		symbols,
		CompletenessComplete,
	)
	if got[0].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("first matching principal set should stay principal: %+v", got[0])
	}
	if got[1].Role != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("duplicate projected principal set should be demoted: %+v", got[1])
	}
}

func TestPruneAggregateMemberSetsByStructuredExclusions(t *testing.T) {
	facts := []AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "public functions",
			Value:   "5",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Eval", "EvalAll", "IsRegistered", "RegisteredKinds", "SetExternalArtifactFloor"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "public function symbols",
			Value:   "8",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Eval (eval.go:15)", "EvalAll (eval.go:36)", "dispatch (eval.go:51)", "IsRegistered (grammar.go:100)", "RegisteredKinds (grammar.go:106)", "SetExternalArtifactFloor (eval.go:982)", "parseComparison (eval.go:918)", "parseIntThreshold (eval.go:190)"},
			SupportRefs: []string{
				"Eval: internal/analysis/criterion/eval.go:15",
				"EvalAll: internal/analysis/criterion/eval.go:36",
				"dispatch: internal/analysis/criterion/eval.go:51",
				"IsRegistered: internal/analysis/criterion/grammar.go:100",
				"RegisteredKinds: internal/analysis/criterion/grammar.go:106",
				"SetExternalArtifactFloor: internal/analysis/criterion/eval.go:982",
				"parseComparison: internal/analysis/criterion/eval.go:918",
				"parseIntThreshold: internal/analysis/criterion/eval.go:190",
			},
		},
		{
			Kind:    AnswerAggregateExcluded,
			Label:   "excluded unexported functions",
			Value:   "3",
			Members: []string{"dispatch", "parseComparison", "parseIntThreshold"},
		},
	}

	got := PruneAggregateMemberSetsByStructuredExclusions(facts)
	if got[1].Value != "5" {
		t.Fatalf("value=%q want 5: %+v", got[1].Value, got[1])
	}
	joined := strings.Join(got[1].Members, ",")
	for _, banned := range []string{"dispatch", "parseComparison", "parseIntThreshold"} {
		if strings.Contains(joined, banned) || strings.Contains(strings.Join(got[1].SupportRefs, ","), banned) {
			t.Fatalf("excluded member %q leaked after prune: %+v", banned, got[1])
		}
	}
	if !strings.Contains(joined, "SetExternalArtifactFloor") {
		t.Fatalf("allowed exported member was lost: %+v", got[1])
	}
}

func TestProjectPrincipalAggregateFactsOntoCompleteAnswerSymbols_IgnoresCategoryLevelSlateWithoutMemberOverlap(t *testing.T) {
	facts := []AnswerAggregateFact{
		{
			Kind:    AnswerAggregateGroupedCount,
			Label:   "public types",
			Value:   "3",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Kind", "Env", "Result"},
		},
		{
			Kind:    AnswerAggregateGroupedCount,
			Label:   "public functions",
			Value:   "4",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"IsRegistered", "RegisteredKinds", "Eval", "EvalAll"},
		},
	}
	symbols := []AnswerSymbol{
		{Name: "types", Kind: KindLiteral},
		{Name: "functions", Kind: KindLiteral},
		{Name: "Kind constants", Kind: KindLiteral},
	}

	got := ProjectPrincipalAggregateFactsOntoCompleteAnswerSymbols(
		facts,
		&RequestModel{Intent: IntentEnumerate},
		symbols,
		CompletenessComplete,
	)

	if len(got) != len(facts) {
		t.Fatalf("facts len=%d want %d", len(got), len(facts))
	}
	for i := range got {
		if got[i].Role != AnswerAggregateRolePrincipalAnswer || strings.Join(got[i].Members, ",") != strings.Join(facts[i].Members, ",") {
			t.Fatalf("category-level answer symbols must not demote or prune principal members: got[%d]=%+v", i, got[i])
		}
	}
}

func TestBuildAnswerSurfacePlan_ProjectsAggregateFactsAfterCompleteAnswerSymbols(t *testing.T) {
	mut := NewMutableState("public functions")
	mut.SetInvestigationAggregateFacts([]AnswerAggregateFact{{
		Kind:    AnswerAggregateGroupedCount,
		Label:   "functions",
		Value:   "3",
		Members: []string{"Eval", "EvalAll", "parseIntThreshold"},
	}})
	mut.SetInvestigationComplete("done")
	mut.RetainInvestigationAggregateFacts()
	mut.SetEmittedAnswerSymbols([]AnswerSymbol{
		{Name: "Eval", Kind: KindFunction},
		{Name: "EvalAll", Kind: KindFunction},
	}, CompletenessComplete)

	plan := BuildAnswerSurfacePlan(&AnalysisIR{RequestModel: RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}}, mut, nil, nil, nil, nil)
	if plan == nil || len(plan.StableAggregateFacts) != 1 {
		t.Fatalf("plan aggregate facts = %+v", plan)
	}
	got := plan.StableAggregateFacts[0]
	if got.Value != "2" || strings.Join(got.Members, ",") != "Eval,EvalAll" {
		t.Fatalf("BuildAnswerSurfacePlan must project stale aggregate facts at source, got %+v", got)
	}
}

func TestPrincipalAggregateMemberSetFactRefsForRequest_ScalarHistoryCountTreatsMembersAsSupport(t *testing.T) {
	facts := []AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "matching commits",
		Value:   "2",
		Role:    AnswerAggregateRolePrincipalAnswer,
		Members: []string{"commit a", "commit b"},
	}}
	rm := RequestModel{
		Predicates: SemanticPredicates{
			IsScalarAnswer:  true,
			IsCountQuestion: true,
			IsHistoryLookup: true,
		},
	}

	if got := PrincipalAggregateMemberSetFactRefsForRequest(facts, &rm); len(got) != 0 {
		t.Fatalf("history count member_set should remain support-only even if explicitly marked principal: %+v", got)
	}
	if AggregateMemberSetIsScalarCountSupport(&rm, facts[0]) != true {
		t.Fatalf("scalar history count member_set should be classified as count support")
	}

	rm.Predicates.IsCategoryEnumeration = true
	if got := PrincipalAggregateMemberSetFactRefsForRequest(facts, &rm); len(got) != 1 {
		t.Fatalf("category enumeration should still allow principal member_set rows, got %+v", got)
	}
}

func TestNormalizeAggregateFactRolesForRequest_DemotesScalarCountMemberSetAtSource(t *testing.T) {
	facts := []AnswerAggregateFact{{
		Kind:       AnswerAggregateMemberSet,
		Label:      "non-test Go files",
		Value:      "2",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "command:find internal/tool -name '*.go' | xargs wc -l",
		Unit:       "files",
		Members:    []string{"internal/tool/a.go", "internal/tool/b.go"},
	}, {
		Kind:       AnswerAggregateScalar,
		Label:      "total lines",
		Value:      "42",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "command:find internal/tool -name '*.go' | xargs wc -l | tail -1",
		Unit:       "lines",
	}}
	rm := RequestModel{
		Intent: IntentReturnValue,
		Predicates: SemanticPredicates{
			IsScalarAnswer:  true,
			IsCountQuestion: true,
		},
	}

	got := NormalizeAggregateFactRolesForRequest(facts, &rm)
	if got[0].Role != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("member_set role = %q, want supporting_coverage: %+v", got[0].Role, got[0])
	}
	if !strings.Contains(got[0].Provenance, "demoted:scalar_count_support_member_set") {
		t.Fatalf("demotion provenance missing: %+v", got[0])
	}
	if got[1].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("scalar answer role changed unexpectedly: %+v", got[1])
	}

	rm.Predicates.IsCategoryEnumeration = true
	got = NormalizeAggregateFactRolesForRequest(facts, &rm)
	if got[0].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("category enumeration must preserve principal member_set role, got %+v", got[0])
	}
}

func TestNormalizeAggregateFactRolesForRequest_DemotesMechanismMemberSetAtSource(t *testing.T) {
	facts := []AnswerAggregateFact{{
		Kind:  AnswerAggregateMemberSet,
		Label: "analyze 阶段重试退出路径",
		Value: "4",
		Role:  AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			`Error=="" 且 AnalysisIR!=nil → 成功退出`,
			`autoCorrectAnalyzerStageOutput 成功 → 清除 Error 并成功退出`,
			`retry storm（同 fingerprint ≥ ceil(max/2) 次）→ 提前退出降级路径`,
			`重试预算耗尽 → 降级路径（安装最小非零 AnalysisIR）`,
		},
	}}
	rm := RequestModel{
		Intent:        IntentExplain,
		Scenario:      ScenarioArchitectureExplain,
		Complexity:    ComplexityComplex,
		PredicateAxis: AxisCondition,
		AnalyzerHints: AnalyzerHints{Kind: string(ReqMechanism)},
		SubTopics: []SubTopic{
			{Summary: "retry budget", Entities: []string{"MaxRetriesPerStage"}},
			{Summary: "stage output", Entities: []string{"StageOutput"}},
			{Summary: "fallback", Entities: []string{"AnalysisIR"}},
		},
	}

	got := NormalizeAggregateFactRolesForRequest(facts, &rm)
	if got[0].Role != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("single-topic mechanism member_set should be support context, got %+v", got[0])
	}
	if !strings.Contains(got[0].Provenance, "demoted:mechanism_narrative_support_member_set") {
		t.Fatalf("mechanism demotion provenance missing: %+v", got[0])
	}
	if refs := PrincipalAggregateMemberSetFactRefsForRequest(got, &rm); len(refs) != 0 {
		t.Fatalf("mechanism support member_set must not create principal enumeration rows, got %+v", refs)
	}

	rm.EnumerationBoundary = &RequestedEnumerationBoundary{DeclaredCount: 4, SourceQuote: "4 条退出路径"}
	got = NormalizeAggregateFactRolesForRequest(facts, &rm)
	if got[0].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("explicit bounded mechanism set should remain principal, got %+v", got[0])
	}
}

func TestNormalizeAggregateFactRolesForRequest_DemotesArchitectureNarrativeMemberSetAtSource(t *testing.T) {
	facts := []AnswerAggregateFact{{
		Kind:       AnswerAggregateMemberSet,
		Label:      "read-mode agents",
		Value:      "3",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "explorer aggregate",
		Members:    []string{"AgentAnalyzer", "AgentExplorer", "AgentFinalizer"},
		SupportRefs: []string{
			"AgentAnalyzer @ internal/types/enums.go:33",
			"AgentExplorer @ internal/types/enums.go:34",
			"AgentFinalizer @ internal/types/enums.go:36",
		},
	}}
	rm := RequestModel{
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Complexity: ComplexityComplex,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true, // analyzer drift must not turn architecture context into a hard set
			IsCrossComponent:      true,
		},
		DiagramHint: &DiagramHint{Kind: DiagramFlow},
		SubTopics: []SubTopic{
			{Summary: "pipeline stages", Entities: []string{"StageAnalyze", "StageExplore"}},
			{Summary: "agent roles", Entities: []string{"AgentAnalyzer", "AgentExplorer"}},
			{Summary: "final answer", Entities: []string{"AgentFinalizer"}},
		},
	}

	got := NormalizeAggregateFactRolesForRequest(facts, &rm)
	if got[0].Role != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("architecture narrative member_set should be support context, got %+v", got[0])
	}
	if !strings.Contains(got[0].Provenance, "demoted:architecture_narrative_support_member_set") {
		t.Fatalf("architecture demotion provenance missing: %+v", got[0])
	}
	if refs := PrincipalAggregateMemberSetFactRefsForRequest(got, &rm); len(refs) != 0 {
		t.Fatalf("architecture support member_set must not create principal enumeration rows, got %+v", refs)
	}

	rm.CompletenessObligation = &CompletenessObligation{Required: true, SourceQuote: "列出全部 agents"}
	got = NormalizeAggregateFactRolesForRequest(facts, &rm)
	if got[0].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("explicit architecture member obligation should remain principal, got %+v", got[0])
	}
}

func TestNormalizeAggregateFactRolesForRequest_DemotesNonScalarHistoryScalars(t *testing.T) {
	facts := []AnswerAggregateFact{{
		Kind:       AnswerAggregateScalar,
		Label:      "最近一次合入特性",
		Value:      "Tighten explorer guidance and evidence grounding",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "git log --merges -1 --oneline",
	}, {
		Kind:       AnswerAggregateScalar,
		Label:      "merge commit",
		Value:      "aa27be48",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "git log --merges -1 --oneline",
	}, {
		Kind:       AnswerAggregateMemberSet,
		Label:      "feature commit",
		Value:      "1",
		Members:    []string{"af683718 Tighten explorer guidance and evidence grounding"},
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "git log --merges -1 --oneline",
	}, {
		Kind:       AnswerAggregateGroupedCount,
		Label:      "修改文件数",
		Value:      "18",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "git show --stat",
	}}
	rm := RequestModel{
		Intent: IntentExplain,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
	}

	got := NormalizeAggregateFactRolesForRequest(facts, &rm)
	for _, fact := range got {
		if fact.Role != AnswerAggregateRoleSupportingCoverage {
			t.Fatalf("non-scalar history metadata should become support metadata, got %+v", fact)
		}
		if !strings.Contains(fact.Provenance, "demoted:history_metadata_support") {
			t.Fatalf("demotion provenance missing: %+v", fact)
		}
	}

	rm.Predicates.IsScalarAnswer = true
	got = NormalizeAggregateFactRolesForRequest(facts, &rm)
	if got[0].Role != AnswerAggregateRolePrincipalAnswer || got[1].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("true scalar history lookup must keep principal scalar facts, got %+v", got)
	}

	rm.Predicates.IsScalarAnswer = false
	rm.Intent = IntentEnumerate
	got = NormalizeAggregateFactRolesForRequest(facts, &rm)
	if got[2].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("explicit history list/enumeration should preserve principal member_set facts, got %+v", got[2])
	}

	rm.Intent = IntentExplain
	rm.Buckets = []QuestionBucket{{Label: "commit A", Index: 1}, {Label: "commit B", Index: 2}}
	got = NormalizeAggregateFactRolesForRequest(facts, &rm)
	if got[2].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("explicit bucketed history comparison should preserve principal member_set facts, got %+v", got[2])
	}
	rm.Buckets = nil
	rm.Predicates.IsCrossComponent = true
	rm.Scenario = ScenarioArchitectureExplain
	rm.AnalyzerHints = AnalyzerHints{Kind: string(ReqMechanism)}
	rm.EnumerationBoundary = &RequestedEnumerationBoundary{DeclaredCount: 1, SourceQuote: "一次"}
	got = NormalizeAggregateFactRolesForRequest(facts, &rm)
	for _, fact := range got {
		if fact.Role != AnswerAggregateRoleSupportingCoverage {
			t.Fatalf("history + current-code mechanism metadata should become support, got %+v", fact)
		}
	}
}

func TestPrincipalAggregateMemberSetFactRefsForRequest_HistoryMechanismTreatsExplicitSetsAsSupport(t *testing.T) {
	facts := []AnswerAggregateFact{{
		Kind:       AnswerAggregateMemberSet,
		Label:      "相关 commit",
		Value:      "2",
		Members:    []string{"40f36f93 Accept scalar aggregate count handoffs", "e4f15aa1 Treat scalar count boundaries as scope"},
		Role:       AnswerAggregateRolePrincipalAnswer,
		Provenance: "git_history_search",
	}}
	rm := RequestModel{
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Complexity: ComplexityComplex,
		Predicates: SemanticPredicates{
			IsHistoryLookup:  true,
			IsCrossComponent: true,
		},
		AnalyzerHints:       AnalyzerHints{Kind: string(ReqMechanism)},
		EnumerationBoundary: &RequestedEnumerationBoundary{DeclaredCount: 1, SourceQuote: "一次"},
	}
	if got := PrincipalAggregateMemberSetFactRefsForRequest(facts, &rm); len(got) != 0 {
		t.Fatalf("history + current-code mechanism support sets must not become principal rows, got %+v", got)
	}

	rm.Intent = IntentEnumerate
	if got := PrincipalAggregateMemberSetFactRefsForRequest(facts, &rm); len(got) != 1 {
		t.Fatalf("explicit history enumeration must still preserve principal member_set rows, got %+v", got)
	}
}

func TestDropPartialAggregateExcludedLists_KeepsCountAndOmitsNonExactList(t *testing.T) {
	facts := []AnswerAggregateFact{{
		Kind:       AnswerAggregateExcluded,
		Label:      "排除的测试文件",
		Value:      "116",
		Provenance: "find internal/tool -name \"*_test.go\"",
		Excluded:   []string{"116 个 *_test.go 文件"},
	}}

	got, err := NormalizeAnswerAggregateFacts(facts)
	if err != nil {
		t.Fatalf("partial excluded_count should normalize, not reject: %v", err)
	}
	if len(got) != 1 || got[0].Value != "116" {
		t.Fatalf("normalized excluded_count = %+v", got)
	}
	if len(got[0].Excluded) != 0 {
		t.Fatalf("partial excluded list should be omitted, got %+v", got[0].Excluded)
	}
	if !strings.Contains(got[0].Provenance, "normalized:partial_excluded_list_omitted") {
		t.Fatalf("normalization provenance missing: %+v", got[0])
	}

	exact, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:     AnswerAggregateExcluded,
		Label:    "排除项",
		Value:    "1",
		Excluded: []string{"internal/tool/foo_test.go"},
	}})
	if err != nil {
		t.Fatalf("exact excluded_count should still pass: %v", err)
	}
	if len(exact[0].Excluded) != 1 {
		t.Fatalf("exact excluded list should be preserved, got %+v", exact[0])
	}
}

func TestBuildAnswerSurfacePlan_DoesNotNarrowAcceptedMemberSetWithIncompleteAnswerSymbols(t *testing.T) {
	mut := NewMutableState("public constants")
	mut.SetInvestigationAggregateFacts([]AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "Kind 常量成员",
		Value:   "3",
		Members: []string{"KindSymbolPresent", "KindNoCallSites", "KindExternalArtifactDecoded"},
	}})
	mut.SetInvestigationComplete("done")
	mut.RetainInvestigationAggregateFacts()
	mut.SetEmittedAnswerSymbols([]AnswerSymbol{
		{Name: "KindSymbolPresent", Kind: KindConst},
		{Name: "KindNoCallSites", Kind: KindConst},
	}, CompletenessComplete)

	plan := BuildAnswerSurfacePlan(&AnalysisIR{RequestModel: RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}}, mut, nil, nil, nil, nil)

	if plan == nil || len(plan.StableAggregateFacts) != 1 {
		t.Fatalf("plan aggregate facts = %+v", plan)
	}
	got := plan.StableAggregateFacts[0]
	if got.Value != "3" || strings.Join(got.Members, ",") != "KindSymbolPresent,KindNoCallSites,KindExternalArtifactDecoded" {
		t.Fatalf("complete member_set must remain authoritative over incomplete answer symbols: %+v", got)
	}
}

func TestBuildAnswerSurfacePlan_DemotesConflictingParallelCountFacts(t *testing.T) {
	mut := NewMutableState("parallel enumeration facts")
	mut.SetInvestigationAggregateFacts([]AnswerAggregateFact{
		{
			Kind:  AnswerAggregateScalar,
			Label: "func 数量",
			Value: "8",
			Role:  AnswerAggregateRolePrincipalAnswer,
		},
		{
			Kind:  AnswerAggregateScalar,
			Label: "Kind const block 数量",
			Value: "1",
			Role:  AnswerAggregateRolePrincipalAnswer,
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "公开函数",
			Value:   "5",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Eval", "EvalAll", "IsRegistered", "RegisteredKinds", "SetExternalArtifactFloor"},
		},
	})
	mut.SetInvestigationComplete("done")
	mut.RetainInvestigationAggregateFacts()

	plan := BuildAnswerSurfacePlan(&AnalysisIR{RequestModel: RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{
			Required:    true,
			SourceQuote: "all public symbols",
		},
	}}, mut, nil, nil, nil, nil)
	if plan == nil || len(plan.StableAggregateFacts) != 3 {
		t.Fatalf("plan aggregate facts = %+v", plan)
	}
	if got := plan.StableAggregateFacts[0]; got.Role != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("conflicting stale count should be demoted to support, got %+v", got)
	}
	if got := plan.StableAggregateFacts[1]; got.Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("independent boundary count of 1 should remain principal, got %+v", got)
	}
}

func TestBuildAnswerSurfacePlan_DemotesSingletonMetadataMemberSetOutsideCompleteSymbols(t *testing.T) {
	mut := NewMutableState("enumeration facts with count-basis metadata")
	mut.SetInvestigationAggregateFacts([]AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "类型成员",
			Value:   "2",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Kind", "Env"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "函数成员",
			Value:   "2",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Eval", "EvalAll"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "Kind const block 数量",
			Value:   "1",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"const () block @ grammar.go:28-66"},
		},
	})
	mut.SetInvestigationComplete("done")
	mut.RetainInvestigationAggregateFacts()
	mut.SetEmittedAnswerSymbols([]AnswerSymbol{
		{Name: "Kind", Kind: KindType},
		{Name: "Env", Kind: KindType},
		{Name: "Eval", Kind: KindFunction},
		{Name: "EvalAll", Kind: KindFunction},
	}, CompletenessComplete)

	ir := &AnalysisIR{RequestModel: RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{
			Required:    true,
			SourceQuote: "all public symbols by category",
		},
	}}
	plan := BuildAnswerSurfacePlan(ir, mut, nil, nil, nil, nil)
	if plan == nil {
		t.Fatal("plan is nil")
	}
	refs := PrincipalAggregateMemberSetFactRefsForRequest(plan.StableAggregateFacts, &ir.RequestModel)
	if len(refs) != 2 {
		t.Fatalf("only actual answer member sets should remain principal, got %+v", refs)
	}
	for _, fact := range plan.StableAggregateFacts {
		if fact.Label == "Kind const block 数量" && fact.Role != AnswerAggregateRoleSupportingCoverage {
			t.Fatalf("singleton count-basis metadata must be support context, got %+v", fact)
		}
	}
}

func TestBuildAnswerSurfacePlan_DemotesSameRoleSubBucketsWhenBroadSupersetExists(t *testing.T) {
	mut := NewMutableState("Kind 常量三分类枚举")
	mut.SetInvestigationAggregateFacts(MergeAnswerAggregateFacts(
		[]AnswerAggregateFact{
			{
				Kind:    AnswerAggregateMemberSet,
				Label:   "Kind 常量成员（读模式）",
				Value:   "2",
				Role:    AnswerAggregateRolePrincipalAnswer,
				Members: []string{"KindSymbolPresent", "KindNoCallSites"},
			},
			{
				Kind:    AnswerAggregateMemberSet,
				Label:   "Kind 常量成员（写模式）",
				Value:   "2",
				Role:    AnswerAggregateRolePrincipalAnswer,
				Members: []string{"KindPlanReady", "KindPatchApplies"},
			},
		},
		[]AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "Kind 常量",
			Value:   "4",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"KindSymbolPresent", "KindNoCallSites", "KindPlanReady", "KindPatchApplies"},
		}},
	))
	mut.SetInvestigationComplete("done")
	mut.RetainInvestigationAggregateFacts()

	ir := &AnalysisIR{RequestModel: RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}}
	plan := BuildAnswerSurfacePlan(ir, mut, nil, nil, nil, nil)
	if plan == nil {
		t.Fatal("plan is nil")
	}
	refs := PrincipalAggregateMemberSetFactRefsForRequest(plan.StableAggregateFacts, &ir.RequestModel)
	if len(refs) != 1 {
		t.Fatalf("only broad same-role superset should remain principal, got %+v", refs)
	}
	if got := refs[0].Fact.Label; got != "Kind 常量" {
		t.Fatalf("principal set label = %q, want broad Kind 常量; refs=%+v", got, refs)
	}
	for _, fact := range plan.StableAggregateFacts {
		if strings.Contains(fact.Label, "读模式") || strings.Contains(fact.Label, "写模式") {
			if fact.Role != AnswerAggregateRoleSupportingCoverage {
				t.Fatalf("same-role sub-bucket should be support when broad superset exists: %+v", fact)
			}
		}
	}
}

func TestMutableState_StableInvestigationAggregateFactsRetention(t *testing.T) {
	mu := NewMutableState("q")
	facts := []AnswerAggregateFact{{
		Kind:    AnswerAggregateBucketCount,
		Label:   "CLI bucket",
		Value:   "2",
		Unit:    "items",
		Members: []string{"--foo", "--bar"},
	}}
	mu.SetInvestigationAggregateFacts(facts)
	if got := mu.StableInvestigationAggregateFacts(); len(got) != 0 {
		t.Fatalf("downgraded/current facts must not be stable before completion: %+v", got)
	}
	mu.SetInvestigationComplete("done")
	mu.RetainInvestigationAggregateFacts()
	mu.ResetInvestigationComplete()

	got := mu.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Label != "CLI bucket" || len(got[0].Members) != 2 {
		t.Fatalf("stable aggregate facts not retained across reset: %+v", got)
	}
	got[0].Members[0] = "mutated"
	again := mu.StableInvestigationAggregateFacts()
	if again[0].Members[0] != "--foo" {
		t.Fatalf("StableInvestigationAggregateFacts must return a defensive copy: %+v", again)
	}

	// A later rejected/downgraded closure may decode a new aggregate payload
	// before its gates fail. That in-flight payload must never replace the
	// accepted stable pool until the completion flag is set and the tool retains
	// it after all gates pass.
	mu.SetInvestigationAggregateFacts([]AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "stale downgraded payload",
		Value:   "1",
		Members: []string{"stale"},
	}})
	stable := mu.StableInvestigationAggregateFacts()
	if len(stable) != 1 || stable[0].Label != "CLI bucket" || stable[0].Members[0] != "--foo" {
		t.Fatalf("downgraded in-flight aggregate facts polluted stable pool: %+v", stable)
	}
}

func TestMutableState_MergeExploreForkUnionsStableAggregateFacts(t *testing.T) {
	parent := NewMutableState("q")
	fork1 := parent.ForkForExploreDispatch()
	fork2 := parent.ForkForExploreDispatch()

	fork1.SetInvestigationAggregateFacts([]AnswerAggregateFact{{
		Kind:        AnswerAggregateMemberSet,
		Label:       "public functions",
		Value:       "2",
		Role:        AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"Eval", "EvalAll"},
		SupportRefs: []string{"eval.go:15", "eval.go:36"},
	}})
	fork1.SetInvestigationComplete("branch one complete")
	fork1.RetainInvestigationAggregateFacts()

	fork2.SetInvestigationAggregateFacts([]AnswerAggregateFact{{
		Kind:        AnswerAggregateMemberSet,
		Label:       "public functions",
		Value:       "2",
		Role:        AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"EvalAll", "RegisteredKinds"},
		SupportRefs: []string{"eval.go:36", "grammar.go:106"},
	}})
	fork2.SetInvestigationComplete("branch two complete")
	fork2.RetainInvestigationAggregateFacts()

	parent.MergeExploreFork(fork1)
	parent.MergeExploreFork(fork2)

	got := parent.StableInvestigationAggregateFacts()
	if len(got) != 1 {
		t.Fatalf("stable aggregate facts = %+v, want one merged member_set", got)
	}
	want := []string{"Eval", "EvalAll", "RegisteredKinds"}
	if strings.Join(got[0].Members, ",") != strings.Join(want, ",") {
		t.Fatalf("stable merged members = %+v, want %+v", got[0].Members, want)
	}
}

func TestReconcilePrincipalAggregateMemberSetSupersets_PromotesFilteredSuperset(t *testing.T) {
	facts := []AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "公开函数（func）",
			Value:   "3",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Eval", "EvalAll", "SetExternalArtifactFloor"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "公开函数 (Func)",
			Value:   "5",
			Role:    AnswerAggregateRoleSupportingCoverage,
			Members: []string{"IsRegistered", "RegisteredKinds", "Eval", "EvalAll", "SetExternalArtifactFloor"},
		},
	}

	got := ReconcilePrincipalAggregateMemberSetSupersets(facts)
	if len(got) != 2 {
		t.Fatalf("facts=%+v", got)
	}
	if AnswerAggregateFactRoleForRequest(got[0], nil) != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("narrower set should yield after typed filtering: %+v", got)
	}
	if AnswerAggregateFactRoleForRequest(got[1], nil) != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("supported superset should become principal: %+v", got)
	}
	if strings.Join(got[1].Members, ",") != "IsRegistered,RegisteredKinds,Eval,EvalAll,SetExternalArtifactFloor" {
		t.Fatalf("superset members changed: %+v", got[1])
	}
}
