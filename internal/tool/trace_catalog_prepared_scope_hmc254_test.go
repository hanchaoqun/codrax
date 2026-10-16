package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC254PreparedQueriesKeepChangedScopesAndFailuresIndependent(t *testing.T) {
	bus, c := exportCatalogFixture250(t)
	m := bus.TraceInputPreparer.PreparedMaterials()[0]
	original, ok := c.ArtifactForPath(m.SourcePath())
	if !ok {
		t.Fatal("original absent")
	}
	for _, args := range []map[string]any{
		{"source": "path", "path": m.QueryPath(), "view": "resource_stack", "pid": 101, "time_start": 9, "time_end": 11},
		{"source": "path", "path": m.QueryPath(), "view": "resource_stack", "pid": 202, "time_start": 10, "time_end": 10.05},
	} {
		out := hmc17NamedQuery(t, bus, args)
		if !out.Success {
			t.Fatalf("changed scope failed: %s", out.Summary)
		}
	}
	// A real source-selection repair occurs after preparation, before the
	// native query. Record the failed invocation, never an empty observation.
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RawRequest: "list all captured events", RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
	}}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": m.QueryPath(), "view": "event_search", "time_start": 10, "time_end": 10.05,
		"event_field_filters": []map[string]string{{"field": "appid", "op": "eq", "value": "42"}}})
	for range 2 {
		out, err := (&TraceQuery{}).Execute(bus, args)
		if err != nil || out.Success || out.Repair == nil || out.Repair.Code != types.ToolRepairCodeTraceEventSelectionSourceRequired {
			t.Fatalf("expected real source-selection repair: %v %+v", err, out)
		}
	}
	current := c.CurrentExecution()
	if current.Coverage.DeclaredPlans != 4 || current.Coverage.ObservedQueries != 3 || current.Coverage.Attempts != 8 {
		t.Fatalf("changed scopes collapsed into declared plans: %+v", current.Coverage)
	}
	for _, q := range current.Snapshot.Queries {
		if current.QueryOrigins[q.ID] != tracecatalog.QueryOriginObserved {
			continue
		}
		if q.ArtifactID != original.ID || q.Window == nil {
			t.Fatalf("query lost original identity/window: %+v", q)
		}
		if q.View == "event_search" {
			if len(q.Attempts) != 2 || q.Attempts[0].ID == q.Attempts[1].ID || q.Outcome != tracecatalog.OutcomeFailure || q.Attempts[1].Error.Code != types.ToolRepairCodeTraceEventSelectionSourceRequired || q.Attempts[1].PayloadRef != "" {
				t.Fatalf("failed invocation was erased or claimed evidence: %+v", q)
			}
		} else if q.Object.ID == "101" {
			if q.Window.StartNS != 9_000_000_000 || q.Window.EndNS != 11_000_000_000 {
				t.Fatalf("explicit changed window lost: %+v", q)
			}
		} else if q.Object.ID != "202" || q.Window.StartNS != 10_000_000_000 || q.Window.EndNS != 10_050_000_000 {
			t.Fatalf("independent object/window lost: %+v", q)
		}
	}
}

func TestHMC254PreparedQueriesShareLiveForkLineage(t *testing.T) {
	bus, c := exportCatalogFixture250(t)
	m := bus.TraceInputPreparer.PreparedMaterials()[0]
	alias := filepath.Join(t.TempDir(), "capture.tracebundle.json")
	if err := os.Symlink(m.QueryPath(), alias); err != nil {
		t.Fatal(err)
	}
	forks := []*types.BusContext{bus.ShallowClone(), bus.ShallowClone()}
	type result struct {
		out types.ToolResult
		err error
	}
	done := make(chan result, len(forks))
	for i, fork := range forks {
		fork.Mutable, fork.WorkDir = bus.Mutable.ForkForExploreDispatch(), t.TempDir()
		path := m.QueryPath()
		if i == 1 {
			path = alias
		}
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "resource_stack", "pid": 101, "time_start": 10, "time_end": 10.05})
		go func() {
			out, err := (&TraceQuery{}).Execute(fork, params)
			done <- result{out, err}
		}()
	}
	for range forks {
		got := <-done
		if got.err != nil || !got.out.Success {
			t.Fatalf("fork query failed: %v %s", got.err, got.out.Summary)
		}
	}
	for _, fork := range forks {
		bus.Mutable.MergeExploreFork(fork.Mutable)
	}
	current := bus.Mutable.TraceCatalogs()[0].CurrentExecution()
	if current.Coverage.Attempts != 6 || current.Coverage.ObservedQueries != 0 {
		t.Fatalf("fork attempts lost or invented plans: %+v", current.Coverage)
	}
	mf := exportedManifest250(t, exportCatalogCall250(t, bus, c, "fork-attempts"))
	if mf.ExecutionCoverage.Attempts != 6 || !mf.MaterialComplete {
		t.Fatalf("live-lineage export missed prepared-path attempts: %+v", mf.ExecutionCoverage)
	}
}
