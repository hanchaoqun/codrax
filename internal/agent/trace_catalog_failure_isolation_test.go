package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceCatalogIndependentFailureActualAgentBoundary(t *testing.T) {
	for _, tc := range []struct {
		name                                            string
		planned, sibling, bundle, changed, wantIsolated bool
	}{
		{"frozen_independent_members", true, true, false, false, true},
		{"unplanned_directory", false, true, false, false, false},
		{"single_capture", true, false, false, false, false},
		{"bundle_is_one_capture", true, true, true, false, false},
		{"changed_member", true, true, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := toolpkg.NewRegistry()
			toolpkg.RegisterDefaults(registry)
			bus := traceCapabilitiesDiscoveryBus(t, false)
			bus.WorkDir = t.TempDir()
			bus.TraceInputPreparer = traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: t.TempDir()})
			badName := "bad.systrace"
			if tc.bundle {
				badName = "bad.tracebundle.json"
			}
			bad := filepath.Join(bus.RepoRoot, badName)
			if err := os.WriteFile(bad, nil, 0600); err != nil {
				t.Fatal(err)
			}
			good := filepath.Join(bus.RepoRoot, "good.systrace")
			if tc.sibling {
				body := "app-42 (42) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=42 next_prio=120\napp-42 (42) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
				if err := os.WriteFile(good, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
			base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: registry}, &stubEvaluator{})
			allowed := map[string]bool{"trace_catalog": true, "trace_query": true}
			query := map[string]any{"view": "window_stats", "pid": 42, "time_start": 1, "time_end": 1.02}
			discover := map[string]any{"action": "discover", "root": "."}
			if tc.planned {
				discover["queries"] = []any{query}
			}
			raw, _ := json.Marshal(discover)
			result, err := base.executeTool(ctx, llm.ToolCall{Name: "trace_catalog", Params: raw}, allowed)
			if err != nil || result == nil || !result.Success {
				t.Fatalf("discover: %v %+v", err, result)
			}
			// Use the current discovery's exact canonical paths, as advertised.
			// macOS test roots can otherwise spell /private/var as /var.
			for _, member := range ctx.Mutable.TraceCatalogs()[0].Snapshot().Artifacts {
				switch filepath.Base(member.Path) {
				case badName:
					bad = member.Path
				case "good.systrace":
					good = member.Path
				}
			}
			if tc.changed {
				if err := os.WriteFile(bad, []byte{0, 1, 2, 3}, 0600); err != nil {
					t.Fatal(err)
				}
			}
			query["source"], query["path"] = "path", bad
			raw, _ = json.Marshal(query)
			failed, err := base.executeTool(ctx, llm.ToolCall{Name: "trace_query", Params: raw}, allowed)
			if err != nil || failed == nil || failed.Success || failed.Repair == nil || failed.Repair.Metadata["stage"] != types.ToolRepairStageTraceInputAdmission {
				t.Fatalf("fixture must produce a real physical-input failure: %v %+v", err, failed)
			}
			if failed.TraceCatalogIndependentFailure != tc.wantIsolated {
				t.Fatalf("independent failure qualifier: %+v", failed)
			}
			_, terminal := ctx.Mutable.TraceInputAdmissionTerminal(types.StageExplore)
			if terminal == tc.wantIsolated {
				t.Fatalf("global terminal=%t isolated=%t", terminal, tc.wantIsolated)
			}
			// Public JSON carries the failure report, never the run-local exception.
			serialized, _ := json.Marshal(failed)
			var replay types.ToolResult
			if err := json.Unmarshal(serialized, &replay); err != nil || replay.TraceCatalogIndependentFailure {
				t.Fatal("serialized navigation reconstituted independent-failure permission")
			}
			if tc.sibling {
				query["path"] = good
				raw, _ = json.Marshal(query)
				other, err := base.executeTool(ctx, llm.ToolCall{Name: "trace_query", Params: raw}, allowed)
				if err != nil || other == nil || other.Success != tc.wantIsolated {
					t.Fatalf("other member: %v %+v", err, other)
				}
				if tc.wantIsolated && len(other.Observations) == 0 {
					t.Fatal("independent healthy source skipped normal measurement")
				}
			}
		})
	}
}
