package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC284PublicTargetInheritanceDiagnostic(t *testing.T) {
	first := types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 42591, Source: "user_explicit"}
	second := types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 1494, Source: "user_explicit"}
	invalid := types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: types.RuntimeTargetMaxPID + 1, Source: "user_explicit"}
	for _, tc := range []struct {
		name        string
		targets     []types.RuntimeTarget
		hints       bool
		mutableOnly bool
		explicitPID int
		wantNote    string
		wantCount   string
	}{
		{"multiple_typed_with_hints", []types.RuntimeTarget{first, second}, true, false, 0, "typed target candidates did not resolve to one inheritable selector", "matched_events=2"},
		{"multiple_typed_without_hints", []types.RuntimeTarget{first, second}, false, false, 0, "typed target candidates did not resolve to one inheritable selector", "matched_events=2"},
		{"multiple_mutable_targets", []types.RuntimeTarget{first, second}, true, true, 0, "typed target candidates did not resolve to one inheritable selector", "matched_events=2"},
		{"untyped_hints_only", nil, true, false, 0, "only untyped analyzer target strings were present", "matched_events=2"},
		{"invalid_typed_is_not_valid_target", []types.RuntimeTarget{invalid}, true, false, 0, "only untyped analyzer target strings were present", "matched_events=2"},
		{"no_targets", nil, false, false, 0, "", "matched_events=2"},
		{"single_typed", []types.RuntimeTarget{first}, true, false, 0, "trace_query_target_inherited=true", "matched_events=1"},
		{"equivalent_typed", []types.RuntimeTarget{first, first}, true, false, 0, "trace_query_target_inherited=true", "matched_events=1"},
		{"explicit_selector_wins", []types.RuntimeTarget{first, second}, true, false, 1494, "", "matched_events=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "capture.systrace")
			trace := "target-42591 (42591) [004] .... 2.000000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42591 next_prio=52\n" +
				"peer-1494 (1494) [005] .... 2.010000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=peer next_pid=1494 next_prio=20\n"
			if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
				t.Fatal(err)
			}
			rm := types.RequestModel{RuntimeTargets: tc.targets}
			if tc.hints {
				rm.AnalyzerHints.ExactTargets = []string{"42591", "1494"}
			}
			ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("inspect captured facts")}
			ctx.Mutable.SetRequestModel(rm)
			if !tc.mutableOnly {
				ctx.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
			}
			params := map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": 2, "time_end": 2.1, "limit": 10}
			if tc.explicitPID != 0 {
				params["pid"] = tc.explicitPID
			}
			raw, _ := json.Marshal(params)
			result, err := (&TraceQuery{}).Execute(ctx, raw)
			if err != nil || !result.Success {
				t.Fatalf("query was blocked by diagnostic: %v %s", err, result.Summary)
			}
			if !strings.Contains(result.Summary, tc.wantCount) {
				t.Fatalf("diagnostic changed query population; want %s: %s", tc.wantCount, result.Summary)
			}
			if tc.wantNote != "" && !strings.Contains(result.Summary, tc.wantNote) {
				t.Fatalf("incorrect target handoff; want %q: %s", tc.wantNote, result.Summary)
			}
			if strings.HasPrefix(tc.wantNote, "typed target candidates") {
				if strings.Contains(result.Summary, "only untyped analyzer target strings") || !strings.Contains(result.Summary, "unscoped queries remain valid") {
					t.Fatalf("typed targets were erased or broad queries prohibited: %s", result.Summary)
				}
			}
			if tc.wantNote == "" && strings.Contains(result.Summary, "trace_query_target_not_inherited=true") {
				t.Fatalf("unnecessary target warning: %s", result.Summary)
			}
		})
	}
}
