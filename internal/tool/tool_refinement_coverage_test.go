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
		// wantParams pins concrete suggested PreferredParams values (E4
		// over-capacity recovery), wantParamNot pins the anti-echo rule
		// (suggestions must differ from the failing call's params), and
		// forbidParams pins keys that must be absent (event_search never
		// gets a fallback_view — it IS the C3 escape hatch).
		wantParams   map[string]string
		wantParamNot map[string]string
		forbidParams []string
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
			// E4: a view already at its hard cap (root_cause_rank 12) gets a
			// concrete first-segment window split derived from the last
			// emitted row plus the remaining segment, not an echoed limit.
			name: "trace_query root_cause_rank compacted window split",
			hint: traceQueryRefinement(tracequery.Result{
				View:       "root_cause_rank",
				SourcePath: "trace.systrace",
				TimeStart:  1.0,
				TimeEnd:    2.0,
				Caveats:    []string{"root_cause_rank compacted from 30 to 12 candidate(s)"},
				Compactions: []tracequery.ViewCompaction{{
					View:            "root_cause_rank",
					Dimension:       tracequery.CompactionDimensionCandidates,
					Total:           30,
					Emitted:         12,
					LastEmittedTs:   1.4,
					LastEmittedLine: 480,
				}},
			}, tracequery.Query{
				View:      "root_cause_rank",
				PID:       123,
				TimeStart: 1.0,
				TimeEnd:   2.0,
			}, traceQueryParams{
				Source: "path",
				Path:   "trace.systrace",
				View:   "root_cause_rank",
			}, "path"),
			wantReason:    "trace_query_result_compacted",
			wantPreferred: "trace_query",
			wantParams: map[string]string{
				"time_start":    "1.000000",
				"time_end":      "1.400000",
				"next_segment":  "time_start=1.400000 time_end=2.000000",
				"fallback_view": "event_search",
			},
			wantParamNot: map[string]string{"time_end": "2.000000"},
		},
		{
			// E4: below the hard cap the refinement suggests the limit that
			// widens the result — min(total, MaxLimit) — never the echoed
			// capped value.
			name: "trace_query scheduler_latency compacted suggested limit",
			hint: traceQueryRefinement(tracequery.Result{
				View:       "scheduler_latency_stats",
				SourcePath: "trace.systrace",
				TimeStart:  1.0,
				TimeEnd:    2.0,
				Caveats:    []string{"scheduler_latency_stats compacted from 37 to 5 runnable wait interval(s)"},
				Compactions: []tracequery.ViewCompaction{{
					View:            "scheduler_latency_stats",
					Dimension:       tracequery.CompactionDimensionIntervals,
					Total:           37,
					Emitted:         5,
					LastEmittedTs:   1.7,
					LastEmittedLine: 220,
				}},
			}, tracequery.Query{
				View:      "scheduler_latency_stats",
				PID:       123,
				TimeStart: 1.0,
				TimeEnd:   2.0,
				Limit:     5,
			}, traceQueryParams{
				Source: "path",
				Path:   "trace.systrace",
				View:   "scheduler_latency_stats",
				Limit:  FlexInt(5),
			}, "path"),
			wantReason:    "trace_query_result_compacted",
			wantPreferred: "trace_query",
			wantParams: map[string]string{
				"limit":         "20",
				"fallback_view": "event_search",
			},
			wantParamNot: map[string]string{"limit": "5"},
		},
		{
			// E4: streamed event_search compaction publishes a typed record
			// with no " compacted from " caveat — trace_query_result_compacted
			// now fires on the typed record. event_search stays the C3 escape
			// hatch: no fallback_view, and the known matched total becomes the
			// suggested limit.
			name: "trace_query streamed event_search typed compaction",
			hint: traceQueryRefinement(tracequery.Result{
				View:       "event_search",
				SourcePath: "trace.systrace",
				Events: []tracequery.EventView{{
					Event: tracequery.Event{Line: 42, Ts: 1.2, Type: tracequery.EventTraceMark, Name: "frame"},
				}},
				Compactions: []tracequery.ViewCompaction{{
					View:            "event_search",
					Dimension:       tracequery.CompactionDimensionEvents,
					Total:           120,
					Emitted:         40,
					LastEmittedTs:   1.2,
					LastEmittedLine: 42,
				}},
			}, tracequery.Query{
				View:    "event_search",
				Pattern: "frame",
			}, traceQueryParams{
				Source:  "path",
				Path:    "trace.systrace",
				View:    "event_search",
				Pattern: "frame",
			}, "path"),
			wantReason:    "trace_query_result_compacted",
			wantPreferred: "trace_query",
			wantParams:    map[string]string{"limit": "120"},
			forbidParams:  []string{"fallback_view"},
		},
		{
			// E4 anti-echo on composite views: the bundle row has MaxLimit=0
			// but the mirrored sub-view compaction (root_cause_rank, hard cap
			// 12) is what truncated — the widen-vs-split decision must read
			// the truncating view's row and emit a window split; suggesting
			// limit=Total would re-clamp to 12 forever (the identical-echo
			// suggestion loop the adversarial review caught).
			name: "trace_query composite bundle sub-view compaction window split",
			hint: traceQueryRefinement(tracequery.Result{
				View:       "frame_root_cause_bundle",
				SourcePath: "trace.systrace",
				TimeStart:  1.0,
				TimeEnd:    2.0,
				Compactions: []tracequery.ViewCompaction{{
					View:            "root_cause_rank",
					Dimension:       tracequery.CompactionDimensionCandidates,
					Total:           30,
					Emitted:         12,
					LastEmittedTs:   1.4,
					LastEmittedLine: 480,
				}},
			}, tracequery.Query{
				View:      "frame_root_cause_bundle",
				PID:       123,
				TimeStart: 1.0,
				TimeEnd:   2.0,
			}, traceQueryParams{
				Source: "path",
				Path:   "trace.systrace",
				View:   "frame_root_cause_bundle",
			}, "path"),
			wantReason:    "trace_query_result_compacted",
			wantPreferred: "trace_query",
			wantParams: map[string]string{
				"time_start":   "1.000000",
				"time_end":     "1.400000",
				"next_segment": "time_start=1.400000 time_end=2.000000",
			},
			forbidParams: []string{"limit"},
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
			if len(tc.wantParams) == 0 && len(tc.wantParamNot) == 0 && len(tc.forbidParams) == 0 {
				return
			}
			normalized := types.NormalizeToolRefinementHint(*tc.hint)
			for key, want := range tc.wantParams {
				if got := normalized.PreferredParams[key]; got != want {
					t.Fatalf("preferred param %s=%q, want %q in %+v", key, got, want, normalized.PreferredParams)
				}
			}
			for key, echoed := range tc.wantParamNot {
				if got := normalized.PreferredParams[key]; got == echoed {
					t.Fatalf("preferred param %s=%q echoes the failing call (anti-echo violation): %+v", key, got, normalized.PreferredParams)
				}
			}
			for _, key := range tc.forbidParams {
				if got, ok := normalized.PreferredParams[key]; ok {
					t.Fatalf("preferred param %s=%q must be absent: %+v", key, got, normalized.PreferredParams)
				}
			}
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
