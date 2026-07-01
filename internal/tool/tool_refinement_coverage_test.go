package tool

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBroadToolRefinementCoverageMatrix(t *testing.T) {
	longGrepOutput := make([]string, 0, grepGovernorLineEntryThreshold+1)
	for i := 0; i <= grepGovernorLineEntryThreshold; i++ {
		longGrepOutput = append(longGrepOutput, fmt.Sprintf("internal/pkg/file%d.go:%d: match", i, i+1))
	}
	longFileList := make([]string, 0, grepGovernorFileEntryThreshold+1)
	for i := 0; i <= grepGovernorFileEntryThreshold; i++ {
		longFileList = append(longFileList, fmt.Sprintf("src/pkg%d/file.go", i))
	}

	cases := []struct {
		name          string
		hint          *types.ToolRefinementHint
		wantReason    string
		wantPreferred string
	}{
		{
			name:          "exec broad discovery",
			hint:          execCommandRefinement(nil, `find . -name "*.go" | head -200`, "exec_command_timeout", MaxInlineBytes+1),
			wantReason:    "exec_command_timeout",
			wantPreferred: "list_files",
		},
		{
			name: "grep broad result",
			hint: grepBroadResultRefinement(nil, grepToolParams{
				Pattern: "needle",
				Path:    ".",
			}, strings.Join(longGrepOutput, "\n")),
			wantReason:    "grep_result_truncated",
			wantPreferred: "grep",
		},
		{
			name: "read_file truncated prefix",
			hint: readFileResultRefinement(nil,
				"internal/pkg/big.go",
				"internal/pkg/big.go",
				1,
				120,
				300,
				0,
				0,
				true,
			),
			wantReason:    "read_file_result_truncated",
			wantPreferred: "grep",
		},
		{
			name: "read_file trace artifact truncation",
			hint: readFileResultRefinement(nil,
				"record_trace.systrace",
				"record_trace.systrace",
				1,
				120,
				300,
				0,
				0,
				true,
			),
			wantReason:    "read_file_trace_artifact_truncated",
			wantPreferred: "trace_query",
		},
		{
			name: "list_files broad result",
			hint: listFilesBroadResultRefinement(nil, listFilesParams{
				Path:      ".",
				Recursive: true,
			}, longFileList, MaxInlineBytes+1),
			wantReason:    "list_files_result_truncated",
			wantPreferred: "list_files",
		},
		{
			name: "trace_query event limit",
			hint: traceQueryRefinement(tracequery.Result{
				View:       "event_search",
				SourcePath: "trace.systrace",
				Events: []tracequery.EventView{{
					Event: tracequery.Event{Line: 42, Ts: 1.2, Type: tracequery.EventTraceMark, Name: "frame"},
				}},
			}, tracequery.Query{
				View:    "event_search",
				Pattern: "frame",
				Limit:   1,
			}, traceQueryParams{
				Source:  "path",
				Path:    "trace.systrace",
				View:    "event_search",
				Pattern: "frame",
				Limit:   FlexInt(1),
			}, "path"),
			wantReason:    "trace_query_event_search_limit_reached",
			wantPreferred: "trace_query",
		},
		{
			name:          "git diff large output",
			hint:          gitDiffRefinement(gitDiffParams{Ref: "HEAD~1..HEAD"}, MaxInlineBytes+1),
			wantReason:    "git_diff_result_truncated",
			wantPreferred: "git_diff",
		},
		{
			name:          "git show large output",
			hint:          gitShowRefinement(gitShowParams{Ref: "HEAD"}, MaxInlineBytes+1),
			wantReason:    "git_show_result_truncated",
			wantPreferred: "git_show",
		},
		{
			name:          "git log large output",
			hint:          gitLogRefinement(gitLogParams{Count: 80, Format: "full"}, 80, "full", MaxInlineBytes+1),
			wantReason:    "git_log_result_truncated",
			wantPreferred: "git_log",
		},
		{
			name:          "git history wide window",
			hint:          gitHistorySearchRefinement(gitHistorySearchParams{WindowPath: ".", Contains: "needle"}, ".", ".", "recent", 50, 50, MaxInlineBytes+1),
			wantReason:    "git_history_search_window_exhausted",
			wantPreferred: "git_history_search",
		},
		{
			name: "run_tests timeout",
			hint: runTestsRefinement(runTestsParams{TimeoutSeconds: 30}, &runnerPlan{
				Runner: "go",
				Root:   ".",
				Suite:  "./...",
			}, ".", "run_tests_timeout", MaxInlineBytes+1),
			wantReason:    "run_tests_timeout",
			wantPreferred: "run_tests",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertActionableToolRefinement(t, tc.hint, tc.wantReason, tc.wantPreferred)
		})
	}
}

func assertActionableToolRefinement(t *testing.T, hint *types.ToolRefinementHint, wantReason, wantPreferred string) {
	t.Helper()
	if hint == nil {
		t.Fatalf("missing ToolRefinementHint")
	}
	normalized := types.NormalizeToolRefinementHint(*hint)
	if normalized.ReasonCode != wantReason {
		t.Fatalf("reason_code = %q, want %q; hint=%+v", normalized.ReasonCode, wantReason, normalized)
	}
	if normalized.PreferredNextTool != wantPreferred {
		t.Fatalf("preferred_next_tool = %q, want %q; hint=%+v", normalized.PreferredNextTool, wantPreferred, normalized)
	}
	fields := types.ToolRefinementPromptFields(&normalized, types.ToolRefinementPromptFieldOptions{})
	joined := strings.Join(fields, " ")
	for _, want := range []string{
		"refine_action=soft_narrow_if_answer_critical_else_caveat",
		"preferred_tool=" + wantPreferred,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("refinement prompt fields missing %q:\nfields=%v\nhint=%+v", want, fields, normalized)
		}
	}
}
