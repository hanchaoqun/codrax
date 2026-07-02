package types

import (
	"strings"
	"testing"
)

func TestToolRefinementPromptFieldsRenderSharedSoftNarrowSurface(t *testing.T) {
	refinement := &ToolRefinementHint{
		ReasonCode:               "grep_result_truncated",
		ResultTruncated:          true,
		CandidateBudgetTruncated: true,
		UniverseExcludedReason:   "configured_search_exclude_roots",
		PreferredNextTool:        "repo_map",
		PreferredParams: map[string]string{
			"view":  "task_map",
			"query": "Owner",
		},
		RequiredFields:         []string{"query", "scope"},
		NextCursor:             "50",
		SkippedLargeCandidates: []string{"trace.systrace", "big.log"},
		ExcludedRoots:          []string{".codrax", "node_modules"},
		TopSourceClasses:       []SourcePathRole{SourcePathRoleProduction, SourcePathRoleTest},
		ParamNarrowingSuggestions: []ToolParamNarrowingSuggestion{
			{Param: "scope", Priority: 1, Suggested: "internal/tool", ReasonCode: ToolParamNarrowReasonCandidateBudgetTruncated},
			{Param: "roles", Priority: 2, Suggested: "function,method", ReasonCode: ToolParamNarrowReasonCandidateBudgetTruncated},
			{Param: "top_n", Priority: 3, Suggested: "50", ReasonCode: ToolParamNarrowReasonCandidateBudgetTruncated},
		},
	}

	got := strings.Join(ToolRefinementPromptFields(refinement, ToolRefinementPromptFieldOptions{
		FlagField:   "flags",
		ActionField: "action",
	}), " ")
	for _, want := range []string{
		"flags=result_truncated,candidate_budget_truncated",
		"excluded_reason=configured_search_exclude_roots",
		"action=soft_narrow_if_answer_critical_else_caveat",
		"preferred_tool=repo_map",
		"preferred_params=query=Owner,view=task_map",
		"required_fields=query,scope",
		"next_cursor=50",
		"skipped_large=trace.systrace,big.log",
		"excluded_roots=.codrax,node_modules",
		"top_source_classes=production,test",
		"narrow_params=scope(1: internal/tool)>roles(2: function,method)>top_n(3: 50)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered fields missing %q:\n%s", want, got)
		}
	}
}

// TestToolRefinementPromptFieldsBoundsParamNarrowingSuggestions pins the
// presentation-only item cap on the rendered narrowing rows (prompt-bloat
// guard: the hint keeps its full normalized set, the render shows a bounded
// prefix).
func TestToolRefinementPromptFieldsBoundsParamNarrowingSuggestions(t *testing.T) {
	refinement := &ToolRefinementHint{
		ParamNarrowingSuggestions: []ToolParamNarrowingSuggestion{
			{Param: "a", Priority: 1, Suggested: "1", ReasonCode: "entries_over_threshold"},
			{Param: "b", Priority: 2, Suggested: "2", ReasonCode: "entries_over_threshold"},
			{Param: "c", Priority: 3, ReasonCode: "entries_over_threshold"},
		},
	}
	got := strings.Join(ToolRefinementPromptFields(refinement, ToolRefinementPromptFieldOptions{
		ParamSuggestionLimit: 2,
	}), " ")
	if !strings.Contains(got, "narrow_params=a(1: 1)>b(2: 2)") {
		t.Fatalf("bounded narrow_params missing:\n%s", got)
	}
	if strings.Contains(got, "c(3") {
		t.Fatalf("item cap must bound rendered suggestions:\n%s", got)
	}
	// Empty Suggested renders as param(priority) without a dangling colon.
	all := strings.Join(ToolRefinementPromptFields(refinement, ToolRefinementPromptFieldOptions{}), " ")
	if !strings.Contains(all, ">c(3)") {
		t.Fatalf("empty Suggested should render bare param(priority):\n%s", all)
	}
}

func TestToolRefinementPromptFieldsCanRenderHistoricalQuotedLabels(t *testing.T) {
	refinement := &ToolRefinementHint{
		ResultTruncated:   true,
		PreferredNextTool: "read_file",
		PreferredParams: map[string]string{
			"path":        "src/app.go",
			"line_offset": "40",
		},
		RequiredFields: []string{"path", "line_offset"},
		NextCursor:     "80",
	}

	got := strings.Join(ToolRefinementPromptFields(refinement, ToolRefinementPromptFieldOptions{
		HistoricalProducerLabels: true,
		QuoteValues:              true,
	}), " ")
	for _, want := range []string{
		"refine_flags=`result_truncated`",
		"refine_action=`soft_narrow_if_answer_critical_else_caveat`",
		"prior_stage_preferred_tool=`read_file`",
		"prior_stage_preferred_params=`line_offset=40,path=src/app.go`",
		"prior_stage_required_fields=`path,line_offset`",
		"prior_stage_next_cursor=`80`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("historical quoted fields missing %q:\n%s", want, got)
		}
	}
}
