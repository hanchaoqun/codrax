package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Enter through the public native producer and stop at the real Finalizer
// adapter. The recap must carry the producer's relation, not recompute it from
// two raw numbers whose ordering differs by trace platform and proof caliber.
func TestHMC273WakeupPriorityPublicFinalizer(t *testing.T) {
	for _, tc := range []struct {
		name, flavor, relation, caliber string
		waker, wakee                    int
		openWaker                       bool
	}{
		{"harmony_lower", "harmony_hitrace", "lower_priority_waker", "closed_range_stable", 20, 52, false},
		{"harmony_higher", "harmony_hitrace", "higher_priority_waker", "closed_range_stable", 140, 52, false},
		{"harmony_equal", "harmony_hitrace", "same_priority", "closed_range_stable", 52, 52, false},
		{"generic_raw", "generic_ftrace", "raw_priority_uninterpreted", "closed_range_stable", 20, 52, false},
		{"harmony_kernel_raw", "harmony_hitrace", "raw_priority_uninterpreted", "closed_range_stable", 160, 52, false},
		{"nearest_is_not_ordering", "harmony_hitrace", "", "advisory_nearest", 20, 52, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			trace := fmt.Sprintf(`# tracer: nop
       idle-0 (0) [002] .... 4.999000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=%d
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=%d prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 prio=%d target_cpu=001
`, tc.waker, tc.wakee, tc.wakee)
			if !tc.openWaker {
				trace += fmt.Sprintf("     worker-200 (200) [002] .... 5.005500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=%d prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120\n", tc.waker)
			}
			trace += fmt.Sprintf("       idle-0 (0) [001] .... 5.006000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=%d\n", tc.wakee)
			path := filepath.Join(dir, "priority.ftrace")
			if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
				t.Fatal(err)
			}
			start, end := 4.998, 5.012
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "wakeup_chain", "pid": 100, "time_start": start, "time_end": end, "trace_flavor": tc.flavor})
			result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !result.Success {
				t.Fatalf("public query: %v %s", err, result.Summary)
			}
			var edge *types.ObservationRecord
			for i := range result.Observations {
				r := &result.Observations[i]
				if r.Predicate == "wakeup_chain_edge" && r.Subject == "worker-200" && r.Object == "app-100" {
					edge = r
				}
			}
			if edge == nil {
				t.Fatal("public native edge missing; no vacuous handoff pass")
			}
			if got := hmc273Note(edge.RichNotes, "priority_relation"); got != tc.relation {
				t.Fatalf("native relation: got %q want %q; %v", got, tc.relation, edge.RichNotes)
			}
			if got := hmc273Note(edge.RichNotes, types.TraceNoteKeyPriorityRelationCaliber); got != tc.caliber {
				t.Fatalf("native caliber: got %q want %q; %v", got, tc.caliber, edge.RichNotes)
			}
			ctx := tracePrincipalValueAuthorityTestContext("app-100", 100, result.Observations)
			ctx.RepoRoot, ctx.WorkDir, ctx.Language = dir, dir, "zh"
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			rm := &ctx.AnalysisIR.RequestModel
			rm.Intent = types.IntentExplain
			rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactDirectWaker}}
			rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "4.998000..5.012000"}
			ctx.Mutable.SetRequestModel(*rm)
			ctx = ctxbuilder.BuildAgentContext(types.ToolBusContext(ctx, types.AgentFinalizer), types.AgentFinalizer, types.StageFinalize)
			before, _ := json.Marshal(result)
			projection := hmc218CausalProjection(t, ctx)
			messages := dependencyObservationMessages(t, ctx)
			row := ""
			for _, line := range strings.Split(messages, "\n") {
				if strings.HasPrefix(line, "- principal_wakeup_edge_roles:") && strings.Contains(line, "waker=`worker-200`; wakee=`app-100`") {
					row = line
				}
			}
			if row == "" {
				t.Fatal("actual Finalizer messages lack the target-bound role recap")
			}
			for _, key := range []string{types.TraceNoteKeyWakerPriority, types.TraceNoteKeyWakeePriority, types.TraceNoteKeyWakerPriorityArtifactSource, types.TraceNoteKeyWakeePriorityArtifactSource} {
				if value := hmc273Note(edge.RichNotes, key); value != "" && !strings.Contains(row, key+"=`"+value+"`") {
					t.Errorf("actual role recap lost native endpoint field %s=%q: %s", key, value, row)
				}
			}
			for _, want := range []string{"priority_relation_caliber=`" + tc.caliber + "`", "waker_cpu=2; wakee_target_cpu=1", "role_binding=`exact_row_local`; conclusion_owner=`model`"} {
				if !strings.Contains(row, want) {
					t.Errorf("actual role recap lost %q: %s", want, row)
				}
			}
			if tc.relation != "" {
				if !strings.Contains(row, "priority_relation=`"+tc.relation+"`") {
					t.Errorf("native relation exists but actual role recap lost it: %s", row)
				}
			} else if strings.Contains(row, "priority_relation=`") {
				t.Errorf("weak native evidence acquired an ordering: %s", row)
			}
			after, _ := json.Marshal(result)
			if !bytes.Equal(before, after) || !bytes.Equal(projection, hmc218CausalProjection(t, ctx)) {
				t.Fatal("role handoff mutated native result or causal projection")
			}
		})
	}
}

func hmc273Note(notes []string, key string) string {
	for _, note := range notes {
		if value, ok := strings.CutPrefix(note, key+"="); ok {
			return value
		}
	}
	return ""
}
