package repomap

import (
	"testing"

	ctypes "github.com/hanchaoqun/codrax/internal/types"
)

func narrowingTestObservation() ctypes.SourceInventoryObservation {
	return ctypes.SourceInventoryObservation{
		Active:     true,
		Complete:   false,
		Scopes:     []string{"."},
		Provenance: []string{"repo_lens:tool_query", "repo_lens:candidate_budget_truncated"},
		Execution:  &ctypes.SourceInventoryExecutionState{CandidateBudgetTruncated: true},
		Page:       &ctypes.SourceInventoryObservationPage{Limit: 24, Total: 200, Emitted: 24},
		SourceClasses: []ctypes.SourceInventorySourceClassCount{
			{Role: ctypes.SourcePathRoleProduction, Count: 10},
		},
		Sets: []ctypes.SourceInventoryObservationSet{
			{
				Role:     ctypes.AnswerCandidateRoleFunction,
				Complete: false,
				Count:    3,
				Total:    180,
				Members: []ctypes.SourceInventoryObservationMember{
					{Name: "a", File: "internal/tool/a.go"},
					{Name: "b", File: "internal/tool/b.go"},
					{Name: "c", File: "cmd/root.go"},
				},
			},
			{
				Role:     ctypes.AnswerCandidateRoleType,
				Complete: false,
				Count:    1,
				Total:    20,
				Members: []ctypes.SourceInventoryObservationMember{
					{Name: "T", File: "internal/types/context.go"},
				},
			},
		},
	}
}

// TestRepoMapSourceInventoryRefinementNarrowingSuggestions pins F4-T5: the
// candidate-budget-truncated broad branch derives typed scope/roles/top_n
// narrowing rows from the typed observation only (member file directories
// count-desc then name-asc, per-role set totals, page limit).
func TestRepoMapSourceInventoryRefinementNarrowingSuggestions(t *testing.T) {
	obs := narrowingTestObservation()
	hint := repoMapSourceInventoryRefinementWithNarrowing(nil, obs, ctypes.SourceInventoryLensQuery{
		Path:   ".",
		Scopes: []string{"."},
		TopN:   24,
	})
	if hint == nil {
		t.Fatal("expected source-inventory refinement")
	}
	if !hint.CandidateBudgetTruncated {
		t.Fatalf("unexpected hint: %+v", hint)
	}
	got := map[string]ctypes.ToolParamNarrowingSuggestion{}
	for _, s := range hint.ParamNarrowingSuggestions {
		got[s.Param] = s
	}
	// Multiple directory candidates ride the plural `scopes` param
	// (split-string-array schema, verbatim-adoptable comma list) — the
	// singular `scope` is a single-path param and a comma-joined value
	// there would prefix-match zero files.
	scope, ok := got["scopes"]
	if !ok || scope.Priority != 1 || scope.ReasonCode != ctypes.ToolParamNarrowReasonCandidateBudgetTruncated {
		t.Fatalf("missing scopes suggestion: %+v", hint.ParamNarrowingSuggestions)
	}
	if scope.Suggested != "internal/tool,cmd,internal/types" {
		t.Fatalf("scope candidates must be count-desc then name-asc from member files, got %q", scope.Suggested)
	}
	if _, singular := got["scope"]; singular {
		t.Fatalf("multi-candidate suggestion must not use the singular scope param: %+v", hint.ParamNarrowingSuggestions)
	}
	roles, ok := got["roles"]
	if !ok || roles.Priority != 2 || roles.Suggested != "function,type" {
		t.Fatalf("roles suggestion must rank typed per-role counts: %+v", hint.ParamNarrowingSuggestions)
	}
	topN, ok := got["top_n"]
	if !ok || topN.Priority != 3 || topN.Suggested != "24" {
		t.Fatalf("top_n suggestion must carry the page limit: %+v", hint.ParamNarrowingSuggestions)
	}
}

// TestRepoMapSourceInventoryNarrowingGatedByRuntimeSourceAuthority pins that
// the new suggestions route through the same authority gate as the navigation
// refinements: with runtime-source authority active (the
// 2184de6c/5d176481 lane), no source-inventory narrowing rows are attached.
func TestRepoMapSourceInventoryNarrowingGatedByRuntimeSourceAuthority(t *testing.T) {
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
	if !repoMapRuntimeCurrentSourceAvoidsSourceInventoryRefinement(ctx) {
		t.Fatal("test precondition: runtime-source authority gate must be active for this ctx")
	}
	hint := repoMapSourceInventoryRefinementWithNarrowing(ctx, narrowingTestObservation(), ctypes.SourceInventoryLensQuery{
		Path:   ".",
		Scopes: []string{"."},
		TopN:   24,
	})
	if hint == nil {
		t.Fatal("expected source-inventory refinement to survive the gate (only suggestions are withheld)")
	}
	if len(hint.ParamNarrowingSuggestions) != 0 {
		t.Fatalf("authority-gated refinement must not carry source-inventory narrowing suggestions: %+v", hint.ParamNarrowingSuggestions)
	}
}

// TestRepoMapSourceInventoryNarrowingCursorLane pins the page-incomplete lane:
// the cursor row stays last (priority 4) with the page_incomplete reason.
func TestRepoMapSourceInventoryNarrowingCursorLane(t *testing.T) {
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
			Role:    ctypes.AnswerCandidateRoleFunction,
			Count:   1,
			Total:   64,
			Members: []ctypes.SourceInventoryObservationMember{{Name: "main", File: "src/runtime/main.go"}},
		}},
	}
	hint := repoMapSourceInventoryRefinementWithNarrowing(nil, obs, ctypes.SourceInventoryLensQuery{
		Path:   "src/runtime",
		Scopes: []string{"src/runtime"},
		TopN:   24,
	})
	if hint == nil {
		t.Fatal("expected page-incomplete refinement")
	}
	var cursor *ctypes.ToolParamNarrowingSuggestion
	for i := range hint.ParamNarrowingSuggestions {
		if hint.ParamNarrowingSuggestions[i].Param == "cursor" {
			cursor = &hint.ParamNarrowingSuggestions[i]
		}
	}
	if cursor == nil || cursor.Priority != 4 || cursor.Suggested != "24" ||
		cursor.ReasonCode != ctypes.ToolParamNarrowReasonPageIncomplete {
		t.Fatalf("cursor suggestion must stay the priority-4 page-incomplete lane: %+v", hint.ParamNarrowingSuggestions)
	}
}

// TestRepoMapNavigationRefinementFileMapScopeNarrowing pins the navigation
// branches: a broad file_map (non-gated) attaches a scope(1) suggestion
// derived from the typed ViewData item files, while the runtime-authority
// gated branch attaches none.
func TestRepoMapNavigationRefinementFileMapScopeNarrowing(t *testing.T) {
	graph := budgetTestGraph(6000)
	params := ViewParams{Query: "AgentName registry"}
	data := &ViewData{Sections: []ViewSection{{
		Items: []ViewItem{
			{Text: "a", File: "internal/tool/a.go"},
			{Text: "b", File: "internal/tool/b.go"},
			{Text: "c", File: "cmd/root.go"},
		},
	}}}
	hint := repoMapNavigationRefinement(nil, graph, repoMapParams{
		Path:  ".",
		View:  "file_map",
		Query: params.Query,
	}, params, data)
	if hint == nil {
		t.Fatal("expected large file_map refinement")
	}
	if hint.PreferredParams["view"] != "source_inventory" {
		t.Fatalf("expected source_inventory steer: %+v", hint.PreferredParams)
	}
	if len(hint.ParamNarrowingSuggestions) != 1 {
		t.Fatalf("expected exactly one scope suggestion: %+v", hint.ParamNarrowingSuggestions)
	}
	s := hint.ParamNarrowingSuggestions[0]
	if s.Param != "scopes" || s.Priority != 1 || s.Suggested != "internal/tool,cmd" ||
		s.ReasonCode != ctypes.ToolParamNarrowReasonEntriesOverThreshold {
		t.Fatalf("scopes suggestion mismatch: %+v", s)
	}

	gatedCtx := &ctypes.BusContext{
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
	gated := repoMapNavigationRefinement(gatedCtx, graph, repoMapParams{
		Path:  ".",
		View:  "file_map",
		Query: params.Query,
	}, params, data)
	if gated == nil {
		t.Fatal("expected gated file_map refinement")
	}
	if gated.PreferredParams["view"] != "task_map" {
		t.Fatalf("gated branch must avoid source_inventory: %+v", gated.PreferredParams)
	}
	if len(gated.ParamNarrowingSuggestions) != 0 {
		t.Fatalf("gated branch must not carry source-inventory narrowing suggestions: %+v", gated.ParamNarrowingSuggestions)
	}
}
