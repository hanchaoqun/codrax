package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func catalogPublicFixture(t *testing.T) (*types.BusContext, string) {
	t.Helper()
	root := t.TempDir()
	work := t.TempDir()
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"capture.data", "nested/capture.data"} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "broken.systrace"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	return &types.BusContext{RepoRoot: root, WorkDir: work, Mutable: types.NewMutableState("inspect captures"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(work, "prepared")})}, root
}

func catalogPublicDiscover(t *testing.T, bus *types.BusContext) *tracecatalog.Catalog {
	t.Helper()
	out, err := (&TraceCatalog{}).Execute(bus, []byte(`{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05},{"view":"resource_stack","pid":999,"time_start":10,"time_end":10.05}]}`))
	if err != nil || !out.Success {
		t.Fatalf("discover: %v %s", err, out.Summary)
	}
	if len(out.Observations) != 0 || out.ReadCoverage != nil || out.TraceQuerySourceRead.Path() != "" {
		t.Fatal("catalog minted evidence/read authority")
	}
	c := bus.Mutable.TraceCatalogs()[0]
	s := c.Snapshot()
	if len(s.Artifacts) != 3 || len(s.Queries) != 6 || !s.Discovery.Complete {
		t.Fatalf("roster not complete: %+v", s)
	}
	return c
}

func catalogPublicQuery(t *testing.T, bus *types.BusContext, path string, pid int) types.ToolResult {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "resource_stack", "pid": pid, "time_start": 10, "time_end": 10.05})
	out, err := (&TraceQuery{}).Execute(bus, raw)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTraceCatalogPublicSameNamesObjectsStatusesAndPersistence(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	// Execute in an order different from both file and object discovery order.
	empty := catalogPublicQuery(t, bus, filepath.Join(root, "nested/capture.data"), 999)
	failure := catalogPublicQuery(t, bus, filepath.Join(root, "broken.systrace"), 101)
	success := catalogPublicQuery(t, bus, filepath.Join(root, "capture.data"), 101)
	if !empty.Success || failure.Success || !success.Success || !failure.TraceCatalogIndependentFailure {
		t.Fatalf("outcomes: empty=%+v failed=%+v success=%+v", empty, failure, success)
	}
	counts := map[tracecatalog.Outcome]int{}
	for _, q := range c.Snapshot().Queries {
		counts[q.Outcome]++
		if q.Outcome == tracecatalog.OutcomeSuccess || q.Outcome == tracecatalog.OutcomeEmpty {
			if len(q.Attempts) != 1 || q.Attempts[0].PayloadRef == "" || q.Attempts[0].SourceRevision == "" {
				t.Fatalf("missing original result identity: %+v", q)
			}
			if q.Window == nil || q.Window.StartNS != 10_000_000_000 || q.Window.EndNS != 10_050_000_000 {
				t.Fatalf("window loss: %+v", q)
			}
			if q.Window.EndInclusive != nil {
				t.Fatal("argument bounds guessed native endpoint inclusion")
			}
		}
	}
	if counts[tracecatalog.OutcomeSuccess] != 1 || counts[tracecatalog.OutcomeEmpty] != 1 || counts[tracecatalog.OutcomeFailure] != 1 || counts[tracecatalog.OutcomeNotExecuted] != 3 {
		t.Fatalf("status collapse: %+v", counts)
	}
	ref, err := saveTraceCatalog(bus, c)
	if err != nil {
		t.Fatal(err)
	}
	historical, err := tracecatalog.Load(context.Background(), ref)
	if err != nil || len(historical.Queries) != 6 || !historical.NavigationOnly {
		t.Fatalf("saved catalog: %v %+v", err, historical)
	}
	other := bus.ShallowClone()
	other.Mutable = types.NewMutableState("another run")
	raw, _ := json.Marshal(map[string]any{"action": "status", "catalog_id": c.ID()})
	if out, _ := (&TraceCatalog{}).Execute(other, raw); out.Success {
		t.Fatal("historical ID restored live authority")
	}
	if len(other.Mutable.TraceQueryBlobRefs()) != 0 {
		t.Fatal("historical index registered payload access")
	}
	// A retry never erases the original failure.
	_ = catalogPublicQuery(t, bus, filepath.Join(root, "broken.systrace"), 101)
	for _, q := range c.Snapshot().Queries {
		if q.Outcome == tracecatalog.OutcomeFailure && len(q.Attempts) != 2 {
			t.Fatalf("failure history lost: %+v", q)
		}
	}
}

func TestTraceCatalogPublicStaleSourceAndUnplannedFailure(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	path := filepath.Join(root, "capture.data")
	if out := catalogPublicQuery(t, bus, path, 101); !out.Success {
		t.Fatal(out.Summary)
	}
	if err := os.WriteFile(path, []byte("replaced"), 0600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"action": "status", "catalog_id": c.ID()})
	out, err := (&TraceCatalog{}).Execute(bus, raw)
	if err != nil || !out.Success {
		t.Fatalf("status: %v %s", err, out.Summary)
	}
	for _, q := range c.Snapshot().Queries {
		a, _ := c.ArtifactForPath(path)
		if q.ArtifactID == a.ID && q.Outcome != tracecatalog.OutcomeStale {
			t.Fatalf("stale source revived: %+v", q)
		}
	}
	fail := catalogPublicQuery(t, bus, filepath.Join(root, "broken.systrace"), 202)
	if fail.Success || fail.TraceCatalogIndependentFailure {
		t.Fatal("unplanned input failure gained member-local exception")
	}
	if retry := catalogPublicQuery(t, bus, filepath.Join(root, "broken.systrace"), 202); retry.TraceCatalogIndependentFailure {
		t.Fatal("repeating an unplanned query manufactured a declared plan")
	}
}

func TestTraceCatalogPublicCancellationDoesNotRecordSuccess(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	p := traceQueryParams{Source: "path", Path: filepath.Join(root, "capture.data"), View: "resource_stack"}
	raw := json.RawMessage(`{"view":"resource_stack"}`)
	tickets := traceCatalogBeginQuery(bus, p, raw)
	if len(tickets) != 1 {
		t.Fatal("missing ticket")
	}
	out := types.ToolResult{Success: true, TraceViewCancellation: &types.TraceViewCancellation{}}
	traceCatalogFinishQuery(bus, tickets, &out, nil)
	for _, q := range c.Snapshot().Queries {
		if q.ID == tickets[0].queryID {
			if q.Outcome != tracecatalog.OutcomeCanceled || len(q.Attempts) != 1 || q.Attempts[0].Error == nil {
				t.Fatalf("canceled result was not recorded: %+v %s", q, out.Summary)
			}
			return
		}
	}
	t.Fatal("canceled query vanished")
}

func TestTraceCatalogArgumentBoundsDoNotInventViewBoundaryContract(t *testing.T) {
	for _, view := range []string{"event_search", "resource_stack", "window_stats"} {
		raw, _ := json.Marshal(map[string]any{"view": view, "time_start": 10, "time_end": 10.05})
		var p traceQueryParams
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatal(err)
		}
		plan, err := traceCatalogQueryPlan(p, raw, "artifact")
		if err != nil || plan.Window == nil || plan.Window.EndInclusive != nil {
			t.Fatalf("%s guessed boundary: %+v %v", view, plan, err)
		}
		encoded, _ := json.Marshal(plan.Window)
		if strings.Contains(string(encoded), "end_inclusive") {
			t.Fatal("unknown became false on JSON wire")
		}
	}
}

func TestTraceCatalogPublicAuthorizationAndSaveFailure(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	outside := t.TempDir()
	raw, _ := json.Marshal(map[string]any{"root": outside})
	if out, _ := (&TraceCatalog{}).Execute(bus, raw); out.Success {
		t.Fatal("external root implicitly authorized")
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if out, _ := (&TraceCatalog{}).Execute(bus, []byte(`{"root":"escape"}`)); out.Success {
		t.Fatal("symlink escaped scope")
	}
	badWork := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(badWork, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	bus.WorkDir = badWork
	out, _ := (&TraceCatalog{}).Execute(bus, []byte(`{}`))
	if out.Success || !strings.Contains(out.Summary, "persistence failed") {
		t.Fatalf("false persisted result: %+v", out)
	}
	got, _ := os.ReadFile(badWork)
	if string(got) != "keep" {
		t.Fatal("save damaged existing file")
	}
}
