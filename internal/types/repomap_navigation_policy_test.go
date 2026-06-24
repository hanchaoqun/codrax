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
	if first, ok := got.FirstHopStep(); ok {
		t.Fatalf("source inventory owns its bounded lens surface and must not expose a generic first hop, got %+v", first)
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

func TestCompileRepoMapNavigationPolicy_CallChainShapeAddsCallPathFollowUp(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentTrace,
		PredicateAxis: AxisCall,
		AnalyzerHints: AnalyzerHints{
			Kind:              string(ReqCallChain),
			MentionedEntities: []string{"dispatch"},
		},
	}
	got := CompileRepoMapNavigationPolicy(rm, nil, ExploreLanePlan{})
	if !got.HasRoute(RepoMapNavigationRouteRelationMap) {
		t.Fatalf("call-chain request should keep relation_map route: %+v", got.Steps)
	}
	if !got.HasRoute(RepoMapNavigationRouteCallPath) {
		t.Fatalf("call-chain request should add call_path follow-up step without needing a call-DAG diagram hint: %+v", got.Steps)
	}
	for _, step := range got.Steps {
		if step.Route != RepoMapNavigationRouteRelationMap {
			continue
		}
		found := false
		for _, follow := range step.FollowUps {
			if follow == RepoMapNavigationRouteCallPath {
				found = true
			}
		}
		if !found {
			t.Fatalf("relation_map step should list call_path as a follow-up for call-chain shapes: %+v", step)
		}
	}
}

func TestCompileRepoMapNavigationPolicy_NonCallChainRelationShapeSkipsCallPath(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		PredicateAxis: AxisImplement,
		AnalyzerHints: AnalyzerHints{
			MentionedEntities: []string{"Handler"},
		},
	}
	got := CompileRepoMapNavigationPolicy(rm, nil, ExploreLanePlan{})
	if !got.HasRoute(RepoMapNavigationRouteRelationMap) {
		t.Fatalf("implement-axis request should keep relation_map route: %+v", got.Steps)
	}
	if got.HasRoute(RepoMapNavigationRouteCallPath) {
		t.Fatalf("non-call-chain relation shape must not add call_path: %+v", got.Steps)
	}
}

func TestRepoMapNavigationPolicyWithLocalizationReviewAddsOwnerGapLenses(t *testing.T) {
	review := SourceLocalizationReviewFromTurnA([]string{"pkg/bug.py"}, nil)
	got := RepoMapNavigationPolicyWithLocalizationReview(RepoMapNavigationPolicy{}, &review)
	if !got.HasRoute(RepoMapNavigationRouteFileMap) {
		t.Fatalf("observed source path without owner anchor should require file_map lens: %+v", got.Steps)
	}
	if !got.HasRoute(RepoMapNavigationRouteRelationMap) {
		t.Fatalf("observed source path without owner anchor should require relation_map lens: %+v", got.Steps)
	}
	rendered := got.RenderMarkdownHint("", "")
	if !strings.Contains(rendered, "typed owner-localization gaps") {
		t.Fatalf("rendered localization-gap hint missing typed source:\n%s", rendered)
	}
}

func TestRepoMapNavigationPolicyWithLocalizationReviewSkipsSatisfiedOwner(t *testing.T) {
	review := NormalizeSourceLocalizationReview(SourceLocalizationReview{
		Source:              "test",
		Status:              SourceLocalizationSupported,
		SourcePaths:         []string{"pkg/owner.py"},
		SupportedPaths:      []string{"pkg/owner.py"},
		OwnerSupportedPaths: []string{"pkg/owner.py"},
		Anchors: []SourceLocalizationAnchor{{
			Path:        "pkg/owner.py",
			Role:        SourcePathRoleProduction,
			Kind:        SourceLocalizationAnchorGroundedEvidence,
			Strength:    SourceLocalizationAnchorOwner,
			OwnerSymbol: "Owner.Handle",
		}},
	})
	got := RepoMapNavigationPolicyWithLocalizationReview(RepoMapNavigationPolicy{}, &review)
	if got.HasRoute(RepoMapNavigationRouteFileMap) || got.HasRoute(RepoMapNavigationRouteRelationMap) {
		t.Fatalf("owner-supported localization should not add gap lenses: %+v", got.Steps)
	}
}

func TestCompileRepoMapNavigationPolicy_ArchitectureDiagramProducesSemanticGraph(t *testing.T) {
	rm := RequestModel{
		Intent:      IntentExplain,
		DiagramHint: &DiagramHint{Kind: DiagramArchitecture},
		AnalyzerHints: AnalyzerHints{
			MentionedEntities: []string{"pipeline"},
		},
	}
	got := CompileRepoMapNavigationPolicy(rm, nil, ExploreLanePlan{})
	if !got.HasRoute(RepoMapNavigationRouteSemanticGraph) {
		t.Fatalf("architecture diagram hint should produce semantic_subgraph route: %+v", got.Steps)
	}
	rmFlow := RequestModel{
		Intent:      IntentExplain,
		DiagramHint: &DiagramHint{Kind: DiagramFlow},
	}
	if CompileRepoMapNavigationPolicy(rmFlow, nil, ExploreLanePlan{}).HasRoute(RepoMapNavigationRouteSemanticGraph) {
		t.Fatalf("non-architecture diagram hints must not produce semantic_subgraph")
	}
}

func TestCompileRepoMapNavigationPolicy_ArchitectureScenarioProducesSemanticGraphFirstHop(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
	}
	got := CompileRepoMapNavigationPolicy(rm, nil, ExploreLanePlan{})
	first, ok := got.FirstHopStep()
	if !ok {
		t.Fatalf("architecture explanation should expose a source-navigation first hop: %+v", got.Steps)
	}
	if first.Route != RepoMapNavigationRouteSemanticGraph {
		t.Fatalf("architecture explanation first hop = %s, want semantic_subgraph; steps=%+v", first.Route, got.Steps)
	}
}

func TestCompileRepoMapNavigationPolicy_InventoryStepTeachesImplementersAlternative(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: AnalyzerHints{
			Entities: []string{"Tool"},
		},
	}
	got := CompileRepoMapNavigationPolicy(rm, nil, ExploreLanePlan{})
	if len(got.Steps) == 0 || got.Steps[0].Route != RepoMapNavigationRouteSourceInventory {
		t.Fatalf("enumeration request should keep source_inventory as first soft route: %+v", got.Steps)
	}
	if !strings.Contains(got.Steps[0].When, `view="implementers"`) {
		t.Fatalf("inventory step should name the implementers alternative within the same first hop: %q", got.Steps[0].When)
	}
	rendered := got.RenderMarkdownHint("", "")
	if !strings.Contains(rendered, `view="implementers"`) {
		t.Fatalf("rendered hint should surface the implementers alternative:\n%s", rendered)
	}
	if strings.Contains(rendered, "- `view=\"implementers\"` —") {
		t.Fatalf("implementers must render inside the inventory step, not as a competing first-hop step:\n%s", rendered)
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

func TestRepoMapNavigationCoverageFromReadArtifactsAddsLocalizationGapRoutes(t *testing.T) {
	got := RepoMapNavigationCoverageFromReadArtifacts(&AnalysisIR{}, ExploreLanePlan{}, &TurnAArtifacts{
		ReadFiles: []string{"pkg/bug.py"},
	})
	if got.State != RepoMapNavigationCoverageMissing {
		t.Fatalf("coverage state = %s, want missing for observed-only localization: %+v", got.State, got)
	}
	if !containsRepoMapRoute(got.RequiredRoutes, RepoMapNavigationRouteFileMap) {
		t.Fatalf("localization gap should require file_map route: %+v", got)
	}
	if !containsRepoMapRoute(got.RequiredRoutes, RepoMapNavigationRouteRelationMap) {
		t.Fatalf("localization gap should require relation_map route: %+v", got)
	}
	if !containsRepoMapRoute(got.MissingRoutes, RepoMapNavigationRouteFileMap) ||
		!containsRepoMapRoute(got.MissingRoutes, RepoMapNavigationRouteRelationMap) {
		t.Fatalf("required localization gap routes should be missing before repo_map observations: %+v", got)
	}
}

func TestRepoMapNavigationCoverageFromToolResultsReportsMissingAndCoveredRoutes(t *testing.T) {
	policy := RepoMapNavigationPolicy{Steps: []RepoMapNavigationStep{
		{Route: RepoMapNavigationRouteTaskMap, Purpose: RepoMapNavigationPurposeOrientation},
		{Route: RepoMapNavigationRouteRelationMap, Purpose: RepoMapNavigationPurposeRelation},
	}}
	results := []ToolResult{{
		ToolName: "repo_map",
		Success:  true,
		RawRef:   "blob://repo-map-task",
		Observations: []ObservationRecord{{
			ID:        "repo_map:task#navigation:task_map",
			Origin:    AnswerEvidenceOriginCrossRepoIndex,
			Producer:  "repo_map",
			SourceRef: ObservationSourceRef{Kind: ObservationSourceCrossRepoIndex, RawRef: "blob://repo-map-task"},
			Predicate: RepoMapNavigationObservationPredicate,
			Object:    string(RepoMapNavigationRouteTaskMap),
		}},
	}}

	got := RepoMapNavigationCoverageFromToolResults(policy, results)
	if got.State != RepoMapNavigationCoveragePartial {
		t.Fatalf("coverage state = %s, want partial: %+v", got.State, got)
	}
	if !containsRepoMapRoute(got.CoveredRoutes, RepoMapNavigationRouteTaskMap) {
		t.Fatalf("task_map should be covered: %+v", got)
	}
	if !containsRepoMapRoute(got.MissingRoutes, RepoMapNavigationRouteRelationMap) {
		t.Fatalf("relation_map should be missing: %+v", got)
	}
	if len(got.EvidenceRefs) == 0 || got.EvidenceRefs[0] != "blob://repo-map-task" {
		t.Fatalf("coverage should preserve typed evidence refs: %+v", got.EvidenceRefs)
	}

	results[0].Observations = append(results[0].Observations, ObservationRecord{
		ID:        "repo_map:relation#navigation:relation_map",
		Origin:    AnswerEvidenceOriginCrossRepoIndex,
		Producer:  "repo_map",
		SourceRef: ObservationSourceRef{Kind: ObservationSourceCrossRepoIndex, RawRef: "blob://repo-map-relation"},
		Predicate: RepoMapNavigationObservationPredicate,
		Object:    string(RepoMapNavigationRouteRelationMap),
	})
	got = RepoMapNavigationCoverageFromToolResults(policy, results)
	if got.State != RepoMapNavigationCoverageCovered || len(got.MissingRoutes) != 0 {
		t.Fatalf("coverage should be covered after both routes: %+v", got)
	}
}

func TestRepoMapNavigationCoverageTreatsImplementersAsInventoryAlternative(t *testing.T) {
	policy := RepoMapNavigationPolicy{Steps: []RepoMapNavigationStep{
		{Route: RepoMapNavigationRouteSourceInventory, Purpose: RepoMapNavigationPurposeInventory},
	}}
	got := RepoMapNavigationCoverageFromObservations(policy, []ObservationRecord{{
		ID:        "repo_map:impl#navigation:implementers",
		Origin:    AnswerEvidenceOriginCrossRepoIndex,
		Producer:  "repo_map",
		Predicate: RepoMapNavigationObservationPredicate,
		Object:    string(RepoMapNavigationRouteImplementers),
	}})
	if got.State != RepoMapNavigationCoverageCovered {
		t.Fatalf("implementers should cover the source_inventory first-hop route: %+v", got)
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

func containsRepoMapRoute(values []RepoMapNavigationRoute, want RepoMapNavigationRoute) bool {
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
