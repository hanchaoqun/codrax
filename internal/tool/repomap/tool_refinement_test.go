package repomap

import (
	"testing"

	ctypes "github.com/hanchaoqun/codrax/internal/types"
)

func TestRepoMapNavigationRefinementRelationMapBroadFallback(t *testing.T) {
	graph := budgetTestGraph(1000)
	params := ViewParams{}
	hint := repoMapNavigationRefinement(nil, graph, repoMapParams{
		Path: ".",
		View: "relation_map",
	}, params, GenerateViewData(graph, "relation_map", params))
	if hint == nil {
		t.Fatal("expected broad relation_map refinement")
	}
	if hint.ReasonCode != "repo_map_relation_map_broad_fallback" || !hint.ResultTruncated {
		t.Fatalf("unexpected hint: %+v", hint)
	}
	if hint.PreferredNextTool != "repo_map" || hint.PreferredParams["view"] != "relation_map" {
		t.Fatalf("preferred route mismatch: %+v", hint)
	}
	if !sameStringSetForRepoMapTest(hint.RequiredFields, []string{"query", "scope", "sources"}) {
		t.Fatalf("required fields = %+v", hint.RequiredFields)
	}
}

func TestRepoMapNavigationRefinementTaskMapMissingQuery(t *testing.T) {
	graph := budgetTestGraph(1000)
	params := ViewParams{}
	hint := repoMapNavigationRefinement(nil, graph, repoMapParams{
		Path: ".",
		View: "task_map",
	}, params, GenerateViewData(graph, "task_map", params))
	if hint == nil {
		t.Fatal("expected broad task_map refinement")
	}
	if hint.ReasonCode != "repo_map_task_map_missing_query" || !hint.ResultTruncated {
		t.Fatalf("unexpected hint: %+v", hint)
	}
	if !sameStringSetForRepoMapTest(hint.RequiredFields, []string{"query"}) {
		t.Fatalf("required fields = %+v", hint.RequiredFields)
	}
}

func TestRepoMapNavigationRefinementLargeOverviewPrefersTaskMapQuery(t *testing.T) {
	graph := budgetTestGraph(6000)
	params := ViewParams{}
	hint := repoMapNavigationRefinement(nil, graph, repoMapParams{
		Path: ".",
		View: "overview",
	}, params, GenerateViewData(graph, "overview", params))
	if hint == nil {
		t.Fatal("expected large overview refinement")
	}
	if hint.ReasonCode != "repo_map_overview_large_scope" || !hint.ResultTruncated {
		t.Fatalf("unexpected hint: %+v", hint)
	}
	if hint.PreferredParams["view"] != "task_map" {
		t.Fatalf("large overview should prefer task_map next: %+v", hint.PreferredParams)
	}
	if !sameStringSetForRepoMapTest(hint.RequiredFields, []string{"query"}) {
		t.Fatalf("required fields = %+v", hint.RequiredFields)
	}
}

func TestRepoMapNavigationRefinementLargeFileMapDefaultsToSourceInventory(t *testing.T) {
	graph := budgetTestGraph(6000)
	params := ViewParams{Query: "AgentName registry"}
	hint := repoMapNavigationRefinement(nil, graph, repoMapParams{
		Path:  ".",
		View:  "file_map",
		Query: params.Query,
	}, params, GenerateViewData(graph, "file_map", params))
	if hint == nil {
		t.Fatal("expected large file_map refinement")
	}
	if hint.ReasonCode != "repo_map_file_map_large_scope" || !hint.ResultTruncated {
		t.Fatalf("unexpected hint: %+v", hint)
	}
	if hint.PreferredParams["view"] != "source_inventory" {
		t.Fatalf("ordinary large file_map should still prefer source_inventory narrowing, got %+v", hint.PreferredParams)
	}
	if !sameStringSetForRepoMapTest(hint.RequiredFields, []string{"scope", "roles"}) {
		t.Fatalf("required fields = %+v", hint.RequiredFields)
	}
}

func TestRepoMapNavigationRefinementMixedRuntimeCurrentSourceAvoidsSourceInventory(t *testing.T) {
	graph := budgetTestGraph(6000)
	params := ViewParams{Query: "trace_query hitrace parsing"}
	ctx := &ctypes.BusContext{
		AnalysisIR: &ctypes.AnalysisIR{RequestModel: ctypes.RequestModel{
			PerfTrace: &ctypes.PerfBundle{
				Observations: []ctypes.PerfObservation{{Kind: "span_duration"}},
			},
			CurrentSourceExplanationProfile: &ctypes.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				Modes:                               []ctypes.CurrentSourceExplanationMode{ctypes.CurrentSourceExplanationExplainCurrentMechanism},
				SourceQuotes:                        []string{"current source explains trace parsing"},
				Confidence:                          0.95,
			},
		}},
	}
	hint := repoMapNavigationRefinement(ctx, graph, repoMapParams{
		Path:  ".",
		View:  "file_map",
		Query: params.Query,
	}, params, GenerateViewData(graph, "file_map", params))
	if hint == nil {
		t.Fatal("expected large file_map refinement")
	}
	if hint.ReasonCode != "repo_map_file_map_large_scope" || !hint.ResultTruncated {
		t.Fatalf("unexpected hint: %+v", hint)
	}
	if got := hint.PreferredParams["view"]; got != "task_map" {
		t.Fatalf("mixed runtime/current-source file_map should avoid source_inventory refinement, got view=%q params=%+v", got, hint.PreferredParams)
	}
	if got := hint.PreferredParams["top_n"]; got != "12" {
		t.Fatalf("mixed runtime/current-source refinement should keep owner narrowing bounded, got top_n=%q params=%+v", got, hint.PreferredParams)
	}
	if _, bad := hint.PreferredParams["include_attributes"]; bad {
		t.Fatalf("non-inventory refinement must not carry source_inventory-only params: %+v", hint.PreferredParams)
	}
	if len(hint.RequiredFields) != 0 {
		t.Fatalf("query-backed task_map refinement should not require source_inventory fields, got %+v", hint.RequiredFields)
	}
}

func TestRepoMapNavigationRefinementRuntimeSourceAuthorityAvoidsSourceInventoryWithoutStaticShape(t *testing.T) {
	graph := budgetTestGraph(6000)
	params := ViewParams{Query: "trace parser mechanism"}
	ctx := &ctypes.BusContext{
		TurnRouteHint: ctypes.TurnRouteHint{
			Source:          "mixed",
			NeedsRepoAccess: true,
		},
		AnalysisIR: &ctypes.AnalysisIR{RequestModel: ctypes.RequestModel{
			Intent: ctypes.IntentRootCause,
			CurrentSourceExplanationProfile: &ctypes.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				Modes:                               []ctypes.CurrentSourceExplanationMode{ctypes.CurrentSourceExplanationExplainCurrentMechanism},
				SourceQuotes:                        []string{"current parser mechanism"},
				Confidence:                          0.95,
			},
		}},
		ToolResults: []ctypes.ToolResult{{
			ToolName: "trace_query",
			Success:  true,
			Observations: []ctypes.ObservationRecord{{
				ID:              "trace_query:window#root_cause_rank:1",
				Origin:          ctypes.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				GroundingPolicy: ctypes.ClaimGroundingHard,
				SourceRef:       ctypes.ObservationSourceRef{Kind: ctypes.ObservationSourceRuntimeArtifact, RawRef: "trace.systrace"},
				Summary:         "trace_query ranked the target-thread delay",
			}},
		}},
	}
	if ctypes.MixedRuntimeCurrentSourceRequiredFileCoverageShape(ctx.AnalysisIR.RequestModel) {
		t.Fatal("fixture must not depend on the legacy static mixed shape")
	}
	hint := repoMapNavigationRefinement(ctx, graph, repoMapParams{
		Path:  ".",
		View:  "file_map",
		Query: params.Query,
	}, params, GenerateViewData(graph, "file_map", params))
	if hint == nil {
		t.Fatal("expected large file_map refinement")
	}
	if got := hint.PreferredParams["view"]; got != "task_map" {
		t.Fatalf("authority-backed runtime/source file_map should avoid source_inventory, got view=%q params=%+v", got, hint.PreferredParams)
	}
	if _, bad := hint.PreferredParams["include_attributes"]; bad {
		t.Fatalf("non-inventory refinement must not carry source_inventory-only params: %+v", hint.PreferredParams)
	}
}

func TestRepoMapNavigationRefinementSoftCurrentSourceWithoutRuntimeCarrierUsesSourceInventory(t *testing.T) {
	graph := budgetTestGraph(6000)
	params := ViewParams{Query: "parser mechanism"}
	ctx := &ctypes.BusContext{
		AnalysisIR: &ctypes.AnalysisIR{RequestModel: ctypes.RequestModel{
			Intent: ctypes.IntentExplain,
			CurrentSourceExplanationProfile: &ctypes.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				Modes:                               []ctypes.CurrentSourceExplanationMode{ctypes.CurrentSourceExplanationExplainCurrentMechanism},
				SourceQuotes:                        []string{"current parser mechanism"},
				Confidence:                          0.95,
			},
		}},
	}
	hint := repoMapNavigationRefinement(ctx, graph, repoMapParams{
		Path:  ".",
		View:  "file_map",
		Query: params.Query,
	}, params, GenerateViewData(graph, "file_map", params))
	if hint == nil {
		t.Fatal("expected large file_map refinement")
	}
	if got := hint.PreferredParams["view"]; got != "source_inventory" {
		t.Fatalf("source-only file_map should keep source_inventory narrowing, got view=%q params=%+v", got, hint.PreferredParams)
	}
	if !sameStringSetForRepoMapTest(hint.RequiredFields, []string{"scope", "roles"}) {
		t.Fatalf("source-only refinement should request source_inventory fields, got %+v", hint.RequiredFields)
	}
}

func TestRepoMapNavigationRefinementTargetedViewsStaySilent(t *testing.T) {
	graph := budgetTestGraph(6000)
	for _, view := range []string{"call_path", "edit_impact", "implementers"} {
		params := ViewParams{Query: "Runnable", TargetFile: "src/app.go", EntryPoint: "cmd/app.go"}
		hint := repoMapNavigationRefinement(nil, graph, repoMapParams{
			Path: ".",
			View: view,
		}, params, GenerateViewData(graph, view, params))
		if hint != nil {
			t.Fatalf("%s is a targeted lookup and should stay silent, got %+v", view, hint)
		}
	}
}

func TestRepoMapSourceInventoryRefinementRequiresScopeForBroadBudgetTruncation(t *testing.T) {
	obs := ctypes.SourceInventoryObservation{
		Active:     true,
		Complete:   false,
		Scopes:     []string{"."},
		Provenance: []string{"repo_lens:tool_query", "repo_lens:candidate_budget_truncated"},
		Execution:  &ctypes.SourceInventoryExecutionState{CandidateBudgetTruncated: true},
		SourceClasses: []ctypes.SourceInventorySourceClassCount{
			{Role: ctypes.SourcePathRoleProduction, Count: 10},
			{Role: ctypes.SourcePathRoleThirdParty, Count: 3},
		},
		Sets: []ctypes.SourceInventoryObservationSet{{
			Role:     ctypes.AnswerCandidateRoleFunction,
			Complete: false,
			Count:    1,
			Total:    200,
			Members:  []ctypes.SourceInventoryObservationMember{{Name: "main", File: "cmd/main.go"}},
		}},
	}
	hint := repoMapSourceInventoryRefinement(obs, ctypes.SourceInventoryLensQuery{
		Path:   ".",
		Scopes: []string{"."},
		Roles: []ctypes.AnswerCandidateRole{
			ctypes.AnswerCandidateRoleFunction,
			ctypes.AnswerCandidateRoleType,
		},
		TopN: 24,
	})
	if hint == nil {
		t.Fatal("expected source-inventory refinement")
	}
	if hint.ReasonCode != "source_inventory_candidate_budget_truncated" || !hint.CandidateBudgetTruncated {
		t.Fatalf("unexpected hint: %+v", hint)
	}
	if hint.PreferredNextTool != "repo_map" || hint.PreferredParams["view"] != "source_inventory" {
		t.Fatalf("preferred route mismatch: %+v", hint)
	}
	if _, ok := hint.PreferredParams["scope"]; ok {
		t.Fatalf("broad budget truncation should require a narrower scope, not repeat root scope: %+v", hint.PreferredParams)
	}
	if _, ok := hint.PreferredParams["cursor"]; ok {
		t.Fatalf("broad budget truncation should not prefer cursor paging over narrowing: %+v", hint.PreferredParams)
	}
	if len(hint.RequiredFields) != 1 || hint.RequiredFields[0] != "scope" {
		t.Fatalf("required fields = %+v", hint.RequiredFields)
	}
	if len(hint.TopSourceClasses) != 2 || hint.TopSourceClasses[1] != ctypes.SourcePathRoleThirdParty {
		t.Fatalf("top source classes = %+v", hint.TopSourceClasses)
	}
}

func TestRepoMapSourceInventoryRefinementCarriesCursorForScopedPage(t *testing.T) {
	obs := ctypes.SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Scopes:   []string{"src/runtime"},
		Page: &ctypes.SourceInventoryObservationPage{
			Offset:     0,
			Limit:      24,
			Total:      64,
			Emitted:    24,
			NextCursor: "24",
			Complete:   false,
		},
		Sets: []ctypes.SourceInventoryObservationSet{{
			Role:     ctypes.AnswerCandidateRoleType,
			Complete: false,
			Count:    24,
			Total:    64,
			Members:  []ctypes.SourceInventoryObservationMember{{Name: "Runtime", File: "src/runtime/runtime.ts"}},
		}},
	}
	hint := repoMapSourceInventoryRefinement(obs, ctypes.SourceInventoryLensQuery{
		Path:   ".",
		Scopes: []string{"src/runtime"},
		Roles:  []ctypes.AnswerCandidateRole{ctypes.AnswerCandidateRoleType},
		TopN:   24,
	})
	if hint == nil {
		t.Fatal("expected cursor refinement")
	}
	if hint.ReasonCode != "source_inventory_page_incomplete" || hint.CandidateBudgetTruncated {
		t.Fatalf("unexpected hint: %+v", hint)
	}
	if hint.PreferredParams["scope"] != "src/runtime" || hint.PreferredParams["cursor"] != "24" {
		t.Fatalf("preferred params = %+v", hint.PreferredParams)
	}
	if len(hint.RequiredFields) != 0 {
		t.Fatalf("scoped pagination should not require a new scope: %+v", hint.RequiredFields)
	}
}

func sameStringSetForRepoMapTest(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]int, len(got))
	for _, item := range got {
		seen[item]++
	}
	for _, item := range want {
		if seen[item] == 0 {
			return false
		}
		seen[item]--
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}
