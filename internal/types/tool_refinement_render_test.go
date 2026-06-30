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
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered fields missing %q:\n%s", want, got)
		}
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
