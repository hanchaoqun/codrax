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

func TestHMC246ManifestResumedFailureActualAgentContinues(t *testing.T) {
	for _, kind := range []string{"resumed_members", "single_member", "bundle", "ordinary_unplanned", "revoked", "changed", "unknown_ref"} {
		t.Run(kind, func(t *testing.T) {
			bus := traceCapabilitiesDiscoveryBus(t, false)
			bus.WorkDir = t.TempDir()
			bus.TraceInputPreparer = traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: t.TempDir()})
			badName := "bad.systrace"
			if kind == "bundle" {
				badName = "bad.tracebundle.json"
			}
			if err := os.WriteFile(filepath.Join(bus.RepoRoot, badName), nil, 0600); err != nil {
				t.Fatal(err)
			}
			if kind != "single_member" {
				body := "app-42 (42) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=42 next_prio=120\napp-42 (42) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
				if err := os.WriteFile(filepath.Join(bus.RepoRoot, "good.systrace"), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			discover := json.RawMessage(`{"queries":[{"view":"window_stats","pid":42,"time_start":1,"time_end":1.02}]}`)
			if kind == "ordinary_unplanned" {
				discover = json.RawMessage(`{}`)
			}
			out, err := (&toolpkg.TraceCatalog{}).Execute(bus, discover)
			if err != nil || !out.Success {
				t.Fatalf("discover: %v %s", err, out.Summary)
			}
			var page struct {
				SavedIndex string `json:"saved_index"`
			}
			if err := json.Unmarshal([]byte(out.Summary), &page); err != nil {
				t.Fatal(err)
			}
			if kind == "ordinary_unplanned" {
				raw, _ := json.Marshal(map[string]any{"source": "path", "path": filepath.Join(bus.RepoRoot, badName), "view": "window_stats", "pid": 42, "time_start": 1, "time_end": 1.02})
				_, _ = (&toolpkg.TraceQuery{}).Execute(bus, raw)
			} else {
				root := bus.Mutable.TraceCatalogs()[0].Snapshot().Root
				bus.Mutable = types.NewMutableState("resume")
				bus.UserPinnedFiles = append(bus.UserPinnedFiles, page.SavedIndex)
				raw, _ := json.Marshal(map[string]any{"action": "resume", "root": root, "snapshot_path": page.SavedIndex})
				out, err = (&toolpkg.TraceCatalog{}).Execute(bus, raw)
				if err != nil || !out.Success {
					t.Fatalf("resume: %v %s", err, out.Summary)
				}
			}
			c := bus.Mutable.TraceCatalogs()[0]
			badID, goodID, badPath := "", "", ""
			for _, a := range c.Snapshot().Artifacts {
				for _, q := range c.Snapshot().Queries {
					if q.ArtifactID != a.ID {
						continue
					}
					if filepath.Base(a.Path) == badName {
						badID = q.ID
						badPath = a.Path
					}
					if filepath.Base(a.Path) == "good.systrace" {
						goodID = q.ID
					}
					if kind != "ordinary_unplanned" && c.IsDeclaredQuery(q.ID) {
						t.Fatal("resume granted declared window")
					}
				}
			}
			if kind == "revoked" {
				bus.RepoRoot = t.TempDir()
			}
			if kind == "changed" {
				if err := os.WriteFile(badPath, []byte("different"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "unknown_ref" {
				badID = "not-current"
			}
			registry := toolpkg.NewRegistry()
			toolpkg.RegisterDefaults(registry)
			base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: registry}, &stubEvaluator{})
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
			allowed := map[string]bool{"trace_catalog": true, "trace_query": true}
			call := func(id string) *types.ToolResult {
				raw, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": id}})
				r, e := base.executeTool(ctx, llm.ToolCall{Name: "trace_query", Params: raw}, allowed)
				if e != nil || r == nil {
					t.Fatalf("call: %v %+v", e, r)
				}
				return r
			}
			failed := call(badID)
			want := kind == "resumed_members"
			if failed.Success || failed.TraceCatalogIndependentFailure != want {
				t.Fatalf("failure isolation=%t want=%t: %+v", failed.TraceCatalogIndependentFailure, want, failed)
			}
			if want {
				if _, terminal := ctx.Mutable.TraceInputAdmissionTerminal(types.StageExplore); terminal {
					t.Fatal("failed member blocked current siblings")
				}
				result := call(goodID)
				if !result.Success || len(result.Observations) == 0 {
					t.Fatalf("healthy sibling not really queried: %+v", result)
				}
			}
			serialized, _ := json.Marshal(failed)
			var replay types.ToolResult
			_ = json.Unmarshal(serialized, &replay)
			if replay.TraceCatalogIndependentFailure {
				t.Fatal("JSON restored isolation authority")
			}
		})
	}
}
