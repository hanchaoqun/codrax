package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC224MissingComparisonSideActualAgentSingles(t *testing.T) {
	for _, mode := range []string{"bound_pair", "unbound", "outside_pair", "global_cancel", "bundle_atomic"} {
		t.Run(mode, func(t *testing.T) {
			registry := toolpkg.NewRegistry()
			toolpkg.RegisterDefaults(registry)
			bus := traceCapabilitiesDiscoveryBus(t, false)
			bus.WorkDir = t.TempDir()
			bus.TraceInputPreparer = traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: t.TempDir()})
			bad := filepath.Join(bus.RepoRoot, "missing.data")
			if mode == "bundle_atomic" {
				bad = filepath.Join(bus.RepoRoot, "missing.tracebundle.json")
			}
			good := filepath.Join(bus.RepoRoot, "good.systrace")
			body := "app-42 (42) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=42 next_prio=120\napp-42 (42) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
			if err := os.WriteFile(good, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if mode != "unbound" {
				c, err := tracecatalog.DiscoverIndependentPair(context.Background(), [2]string{bad, good})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := bus.Mutable.InstallTraceCatalog(c); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "outside_pair" {
				bad = filepath.Join(bus.RepoRoot, "not_bound.data")
			}
			cancelCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			bus.Ctx = cancelCtx
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
			base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: registry}, &stubEvaluator{})
			allowed := map[string]bool{"trace_query": true}
			query := func(path string) *types.ToolResult {
				raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 42, "time_start": 1, "time_end": 1.02})
				result, err := base.executeTool(ctx, llm.ToolCall{Name: "trace_query", Params: raw}, allowed)
				if err != nil || result == nil {
					t.Fatalf("actual boundary: %v %+v", err, result)
				}
				return result
			}
			failed := query(bad)
			wantIsolated := mode == "bound_pair" || mode == "global_cancel"
			if failed.Success || failed.TraceCatalogIndependentFailure != wantIsolated {
				t.Fatalf("missing side qualifier: %+v", failed)
			}
			_, terminal := ctx.Mutable.TraceInputAdmissionTerminal(types.StageExplore)
			if terminal == wantIsolated {
				t.Fatalf("terminal=%v isolated=%v", terminal, wantIsolated)
			}
			if mode == "global_cancel" {
				cancel()
			}
			other := query(good)
			if other.Success != (mode == "bound_pair") {
				t.Fatalf("healthy subsequent query: %+v", other)
			}
			if mode == "bound_pair" && len(other.Observations) == 0 {
				t.Fatal("healthy source produced no real facts")
			}
		})
	}
}
