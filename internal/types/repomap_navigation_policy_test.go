package types

import (
	"strings"
	"testing"
)

func TestCompileRepoMapNavigationPolicy_SourceInventory(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqEnumeration),
			Entities: []string{"Kind"},
		},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
		},
	}
	got := CompileRepoMapNavigationPolicy(rm, nil, ExploreLanePlan{})
	if !got.HasRoute(RepoMapNavigationRouteSourceInventory) {
		t.Fatalf("source inventory request should prefer source_inventory route: %+v", got)
	}
	if len(got.Steps) == 0 || got.Steps[0].Route != RepoMapNavigationRouteSourceInventory {
		t.Fatalf("source inventory request should make source_inventory the first soft route, got %+v", got.Steps)
	}
	if !containsRepoMapPolicyTerm(got.QueryTerms, "Kind") {
		t.Fatalf("query terms should preserve typed entity, got %+v", got.QueryTerms)
	}
}

func TestCompileRepoMapNavigationPolicy_RelationFlow(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentTrace,
		PredicateAxis: AxisCall,
		AnalyzerHints: AnalyzerHints{
			Kind:              string(ReqCallChain),
			MentionedEntities: []string{"io_uring"},
		},
		DiagramHint: &DiagramHint{Kind: DiagramFlow},
	}
	got := CompileRepoMapNavigationPolicy(rm, nil, ExploreLanePlan{})
	if !got.HasRoute(RepoMapNavigationRouteTaskMap) {
		t.Fatalf("known entity flow request should use task_map for narrowing: %+v", got)
	}
	if !got.HasRoute(RepoMapNavigationRouteRelationMap) {
		t.Fatalf("call-chain request should use relation_map route: %+v", got)
	}
}

func TestCompileRepoMapNavigationPolicy_ChangeImpact(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		ChangeImpactProfile: &ChangeImpactProfile{
			IsChangeImpact:  true,
			Target:          "ProviderAuth",
			RequestedOutput: ImpactOutputSites,
		},
	}
	got := CompileRepoMapNavigationPolicy(rm, nil, ExploreLanePlan{})
	if !got.HasRoute(RepoMapNavigationRouteEditImpact) {
		t.Fatalf("change impact request should expose edit_impact route: %+v", got)
	}
	if !containsRepoMapPolicyTerm(got.QueryTerms, "ProviderAuth") {
		t.Fatalf("change impact target should be a query term, got %+v", got.QueryTerms)
	}
}

func TestCompileRepoMapNavigationPolicy_MixedExternalCurrentSource(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			SourceQuotes:                        []string{"当前代码"},
			TargetTerms:                         []string{"auth.set"},
		},
	}
	lanes := ExploreLanePlan{Lanes: []ExploreLane{
		{Origin: AnswerEvidenceOriginVCSDiff, Label: "diff"},
		{Origin: AnswerEvidenceOriginCurrentSource, Label: "current"},
	}}
	got := CompileRepoMapNavigationPolicy(rm, nil, lanes)
	if !got.HasRoute(RepoMapNavigationRouteTaskMap) {
		t.Fatalf("mixed external/current-source request should keep current-source narrowing route: %+v", got)
	}
	if !containsRepoMapPolicyTerm(got.QueryTerms, "auth.set") {
		t.Fatalf("current-source target terms should enter query terms, got %+v", got.QueryTerms)
	}
}

func TestCompileRepoMapNavigationPolicy_ExternalObservationCurrentSourceOrigins(t *testing.T) {
	rm := RequestModel{Intent: IntentExplain}
	lanes := ExploreLanePlan{Lanes: []ExploreLane{
		{Origin: AnswerEvidenceOriginMCPResource, Label: "mcp"},
		{Origin: AnswerEvidenceOriginWebPage, Label: "web"},
		{Origin: AnswerEvidenceOriginConnectorResource, Label: "connector"},
		{Origin: AnswerEvidenceOriginCurrentSource, Label: "current source"},
	}}
	got := CompileRepoMapNavigationPolicy(rm, nil, lanes)
	if !containsRepoMapPolicyStepPurpose(got.Steps, RepoMapNavigationPurposeExternalCurrent) {
		t.Fatalf("external observation + current-source lanes should expose current-source verification route: %+v", got)
	}
}

func TestCompileRepoMapNavigationPolicy_PartitionsSubTopicsBucketsAndLanes(t *testing.T) {
	rm := RequestModel{
		AnalyzerHints: AnalyzerHints{
			PrimaryScopes: []string{"frameworks/base", "hm_z/foundation/resourceschedule/ressched"},
		},
		SubTopics: []SubTopic{
			{Summary: "read side", Entities: []string{"Reader"}, Scopes: []string{"pkg/read"}},
		},
		Buckets: []QuestionBucket{
			{Label: "write side", Anchors: []string{"Writer"}, Index: 1},
		},
	}
	lanes := ExploreLanePlan{Lanes: []ExploreLane{
		{Origin: AnswerEvidenceOriginCurrentSource, Label: "current source", DimensionLabels: []string{"当前关键代码"}},
	}}
	got := CompileRepoMapNavigationPolicy(rm, nil, lanes)
	if len(got.Partitions) != 4 {
		t.Fatalf("expected sub-topic, bucket, and lane partitions, got %+v", got.Partitions)
	}
	if !containsRepoMapPolicyScope(got.Partitions, "frameworks/base") {
		t.Fatalf("expected active sub-repo scopes to be preserved as partitions, got %+v", got.Partitions)
	}
	rendered := got.RenderMarkdownHint("", "")
	if !containsRepoMapPolicySubstring(rendered, "set repo_map `path` to that sub-repo") {
		t.Fatalf("rendered hint should teach multi-repo relative path usage, got:\n%s", rendered)
	}
}

func TestCompileRepoMapNavigationPolicy_PrefersExactCodeSurfacesForQuery(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentTrace,
		PredicateAxis: AxisCall,
		AnalyzerHints: AnalyzerHints{
			Kind:              string(ReqCallChain),
			ExactTargets:      []string{"SubAgentRegistry"},
			MentionedEntities: []string{"agent", "SubAgent"},
			PrimaryEntities:   []string{"dispatcher", "dispatchStage"},
			Keywords:          []string{"调用", "call", "dispatch", "agent", "subagent", "propose_sub_agents"},
		},
		DiagramHint: &DiagramHint{Kind: DiagramFlow},
	}

	got := CompileRepoMapNavigationPolicy(rm, nil, ExploreLanePlan{})
	for _, want := range []string{"SubAgentRegistry", "SubAgent", "dispatchStage", "propose_sub_agents"} {
		if !containsRepoMapPolicyTerm(got.QueryTerms, want) {
			t.Fatalf("exact code surface %q should be preferred, got %+v", want, got.QueryTerms)
		}
	}
	for _, broad := range []string{"agent", "call", "调用", "dispatch", "dispatcher", "subagent"} {
		if containsRepoMapPolicyTerm(got.QueryTerms, broad) {
			t.Fatalf("broad/non-code query term %q should not outrank exact code surfaces, got %+v", broad, got.QueryTerms)
		}
	}
	rendered := got.RenderMarkdownHint("", "")
	for _, want := range []string{"Prefer concise exact code surfaces as `query`", "Do not paste a natural-language sentence"} {
		if !containsRepoMapPolicySubstring(rendered, want) {
			t.Fatalf("rendered hint missing exact-query teaching %q:\n%s", want, rendered)
		}
	}
}

func TestCompileRepoMapNavigationPolicy_FallsBackWhenOnlyBroadTermsExist(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		AnalyzerHints: AnalyzerHints{
			Entities: []string{"scheduler"},
			Keywords: []string{"runtime"},
		},
	}

	got := CompileRepoMapNavigationPolicy(rm, nil, ExploreLanePlan{})
	if !containsRepoMapPolicyTerm(got.QueryTerms, "scheduler") ||
		!containsRepoMapPolicyTerm(got.QueryTerms, "runtime") {
		t.Fatalf("policy should keep broad terms as a fallback when no exact code surfaces exist, got %+v", got.QueryTerms)
	}
}

func containsRepoMapPolicyTerm(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsRepoMapPolicyStepPurpose(values []RepoMapNavigationStep, want RepoMapNavigationPurpose) bool {
	for _, value := range values {
		if value.Purpose == want {
			return true
		}
	}
	return false
}

func containsRepoMapPolicyScope(values []RepoMapNavigationPartition, want string) bool {
	for _, part := range values {
		for _, scope := range part.Scopes {
			if scope == want {
				return true
			}
		}
	}
	return false
}

func containsRepoMapPolicySubstring(value, want string) bool {
	return strings.Contains(value, want)
}
