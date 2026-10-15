package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func checkpointRead247(t *testing.T) (*types.BusContext, string) {
	t.Helper()
	bus, root := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	if r := catalogPublicQuery(t, bus, filepath.Join(root, "capture.data"), 101); !r.Success {
		t.Fatal(r.Summary)
	}
	checkpoint, err := saveTraceCatalog(bus, c)
	if err != nil {
		t.Fatal(err)
	}
	next := bus.ShallowClone()
	next.Mutable = types.NewMutableState("continue saved work")
	next.UserPinnedFiles = []string{checkpoint}
	return next, checkpoint
}

func readCheckpoint247(t *testing.T, bus *types.BusContext, path string) types.ToolResult {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"path": path})
	out, err := (&ReadFile{}).Execute(bus, raw)
	if err != nil || !out.Success {
		t.Fatalf("read: %v %s", err, out.Summary)
	}
	return out
}

func TestHMC247CheckpointReadToCurrentNativeExecution(t *testing.T) {
	bus, path := checkpointRead247(t)
	out := readCheckpoint247(t, bus, path)
	if out.Refinement == nil || out.Refinement.PreferredNextTool != "trace_catalog" || out.Refinement.PreferredParams["action"] != "resume" {
		t.Fatalf("complete checkpoint read has no typed resume navigation: %+v", out.Refinement)
	}
	if len(bus.Mutable.TraceCatalogs()) != 0 || len(out.Observations) != 0 || out.TraceQuerySourceRead.Path() != "" {
		t.Fatal("navigation installed a live plan or restored evidence")
	}
	raw, _ := json.Marshal(out.Refinement.PreferredParams)
	if err := toolparam.Validate(raw, (&TraceCatalog{}).Parameters()); err != nil {
		t.Fatal(err)
	}
	resumed, err := (&TraceCatalog{}).Execute(bus, raw)
	if err != nil || !resumed.Success {
		t.Fatalf("resume: %v %s", err, resumed.Summary)
	}
	c := bus.Mutable.TraceCatalogs()[0]
	for _, q := range c.Snapshot().Queries {
		if q.Outcome != tracecatalog.OutcomeNotExecuted || len(q.Attempts) != 0 || c.IsDeclaredQuery(q.ID) {
			t.Fatal("old execution/window restored")
		}
	}
	q := planFor246(t, c, "capture.data", "101")
	actual, err := (&TraceQuery{}).Execute(bus, refPublic246(c, q))
	if err != nil || !actual.Success || len(actual.Observations) == 0 {
		t.Fatalf("not a native query: %v %s", err, actual.Summary)
	}
}

func TestHMC247CheckpointInvalidAndUnauthorizedDoNotOfferResume(t *testing.T) {
	for _, kind := range []string{"foreign_root", "partial_json", "duplicate_key", "unsupported_schema", "ordinary_json"} {
		t.Run(kind, func(t *testing.T) {
			bus, path := checkpointRead247(t)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "foreign_root":
				bus.RepoRoot = t.TempDir()
			case "partial_json":
				data = data[:len(data)/2]
			case "duplicate_key":
				data = append([]byte(`{"navigation_only":true,`), data[1:]...)
			case "unsupported_schema":
				data = []byte(strings.Replace(string(data), `"schema_version": 1`, `"schema_version": 999`, 1))
			case "ordinary_json":
				data = []byte(`{"root":"/tmp","action":"resume"}`)
			}
			if kind != "foreign_root" {
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			out := readCheckpoint247(t, bus, path)
			if out.Refinement != nil && out.Refinement.PreferredNextTool == "trace_catalog" {
				t.Fatalf("%s offered executable recovery: %+v", kind, out.Refinement)
			}
			if len(bus.Mutable.TraceCatalogs()) != 0 {
				t.Fatal("read installed history")
			}
		})
	}
}

func TestHMC247CheckpointPagedAndOrdinaryReadsKeepTheirMeaning(t *testing.T) {
	bus, path := checkpointRead247(t)
	for _, fields := range []map[string]any{{"path": path, "line_offset": 1, "limit": 2}, {"path": path, "line_offset": 0, "limit": 1}} {
		raw, _ := json.Marshal(fields)
		out, err := (&ReadFile{}).Execute(bus, raw)
		if err != nil || !out.Success {
			t.Fatal(err, out.Summary)
		}
		// Explicit page limits can expand only when the normal reader itself
		// proves an inline-sized complete read; never infer this from the call.
		if out.EnumerationAuthority.Status == "incomplete" && strings.Contains(out.Summary, "Checkpoint navigation") {
			t.Fatal("partial output gained complete navigation")
		}
	}
	ordinary := filepath.Join(bus.RepoRoot, "ordinary.json")
	if err := os.WriteFile(ordinary, []byte(`{"status":"success","root":"/tmp"}`), 0600); err != nil {
		t.Fatal(err)
	}
	out := readCheckpoint247(t, bus, ordinary)
	if out.Refinement != nil && out.Refinement.ReasonCode == "historical_catalog_navigation" {
		t.Fatal("ordinary JSON classified by words")
	}
}

func checkpointAction247(t *testing.T, out types.ToolResult) json.RawMessage {
	t.Helper()
	line := strings.SplitN(out.Summary, "\n", 2)[0]
	const prefix = "Checkpoint navigation (not query evidence): "
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("missing complete navigation: %s", line)
	}
	var navigation struct {
		NextCall   json.RawMessage               `json:"next_call"`
		Historical struct{ Plans, Attempts int } `json:"historical_snapshot"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, prefix)), &navigation); err != nil {
		t.Fatal(err)
	}
	if navigation.Historical.Plans != 6 || navigation.Historical.Attempts != 1 {
		t.Fatalf("historical counts wrong: %+v", navigation)
	}
	return navigation.NextCall
}

func TestHMC247CheckpointActionExactPathsAndRechecks(t *testing.T) {
	for _, kind := range []string{"escaped_path", "long_path", "revoked_after_read", "changed_after_read"} {
		t.Run(kind, func(t *testing.T) {
			bus, oldPath := checkpointRead247(t)
			data, err := os.ReadFile(oldPath)
			if err != nil {
				t.Fatal(err)
			}
			dir := bus.RepoRoot
			if kind == "long_path" {
				dir = filepath.Join(dir, strings.Repeat("a", 120), strings.Repeat("b", 120))
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(dir, "checkpoint,part=\"old\".json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			out := readCheckpoint247(t, bus, path)
			action := checkpointAction247(t, out)
			var args map[string]string
			if err := json.Unmarshal(action, &args); err != nil {
				t.Fatal(err)
			}
			canonical, err := filepath.EvalSymlinks(path)
			if err != nil {
				t.Fatal(err)
			}
			if args["snapshot_path"] != canonical {
				t.Fatalf("path was clipped or reparsed: %s", action)
			}
			if kind == "long_path" && out.Refinement.PreferredNextTool != "" {
				t.Fatal("long path entered truncated generic refinement")
			}
			if kind == "revoked_after_read" {
				bus.RepoRoot = t.TempDir()
			}
			if kind == "changed_after_read" {
				if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			r, err := (&TraceCatalog{}).Execute(bus, action)
			want := kind == "escaped_path" || kind == "long_path"
			if err != nil || r.Success != want {
				t.Fatalf("normal resume admission bypassed: %v %s", err, r.Summary)
			}
		})
	}
}

func TestHMC247CatalogRolesCountsAndWrongCoordinate(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	out, err := (&TraceCatalog{}).Execute(bus, []byte(`{}`))
	if err != nil || !out.Success {
		t.Fatal(err, out.Summary)
	}
	var page map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out.Summary), &page); err != nil {
		t.Fatal(err)
	}
	if _, ok := page["query_calls"]; ok {
		t.Fatal("empty discovery created executable tasks")
	}
	if !strings.Contains(string(page["records"]), `"record_kind":"candidate_artifact"`) {
		t.Fatal("artifact role lost")
	}
	c := bus.Mutable.TraceCatalogs()[0]
	bad, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": c.Snapshot().Artifacts[0].ID}})
	r, err := (&TraceQuery{}).Execute(bus, bad)
	if err != nil || r.Success || !strings.Contains(r.Summary, "candidate artifact, not a planned query") {
		t.Fatalf("wrong coordinate not explained: %v %s", err, r.Summary)
	}
	if len(c.Snapshot().Queries) != 0 {
		t.Fatal("wrong ID minted a plan")
	}
	for i := 0; i < 2; i++ {
		if r := catalogPublicQuery(t, bus, filepath.Join(root, "capture.data"), 101); !r.Success {
			t.Fatal(r.Summary)
		}
	}
	counts := traceCatalogCounts(c.Snapshot().Queries)
	if counts.Plans != 1 || counts.Attempts != 2 || counts.ArtifactObjects != 1 || counts.Outcomes[tracecatalog.OutcomeSuccess] != 1 {
		t.Fatalf("different count units combined: %+v", counts)
	}
}

func TestHMC247CatalogCountAxesDoNotImplyMemberCompletion(t *testing.T) {
	bus, _ := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	first := c.Snapshot().Queries[0]
	for _, delta := range []struct {
		view string
		end  int64
	}{{"event_search", first.Window.EndNS}, {first.View, first.Window.EndNS + 1000000}} {
		p := first.Plan
		p.View = delta.view
		w := *p.Window
		w.EndNS = delta.end
		p.Window = &w
		p.Parameters = json.RawMessage(`{"view":"` + delta.view + `"}`)
		if _, err := c.RegisterPlannedQuery(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	counts := traceCatalogCounts(c.Snapshot().Queries)
	// Same TID in three files remains three members. Extra views/windows
	// add plans, not members, and do not prove that any member is complete.
	if counts.Plans != 8 || counts.ArtifactObjects != 6 || counts.Attempts != 0 || counts.Outcomes[tracecatalog.OutcomeNotExecuted] != 8 {
		t.Fatalf("scope/count axes conflated: %+v", counts)
	}
	b, _ := json.Marshal(counts)
	if strings.Contains(string(b), "completed") {
		t.Fatal("member completion was invented")
	}
}

func TestHMC247CheckpointOutputBudgetWithholdsAction(t *testing.T) {
	bus, path := checkpointRead247(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Valid JSON with excessive formatting still reads within the file bound,
	// but the model sees only a page/preview: do not attach complete navigation.
	data = append([]byte(strings.Repeat("\n", 10000)), data...)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	out := readCheckpoint247(t, bus, path)
	if strings.Contains(out.Summary, "Checkpoint navigation") || (out.Refinement != nil && out.Refinement.PreferredNextTool == "trace_catalog") {
		t.Fatal("clipped read gained complete navigation")
	}
	if _, err := tracecatalog.DecodeSnapshot([]byte(strings.Repeat(" ", tracecatalog.MaxSnapshotBytes+1))); err == nil {
		t.Fatal("snapshot byte bound widened")
	}
}

func TestHMC247CatalogPaginationAndCanceledNavigation(t *testing.T) {
	bus, path := checkpointRead247(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := tracecatalog.DecodeSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	end, err := traceCatalogPage(s, path, len(s.Artifacts)+len(s.Queries))
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]json.RawMessage
	if err := json.Unmarshal([]byte(end), &p); err != nil {
		t.Fatal(err)
	}
	if _, ok := p["next_call"]; ok || !strings.Contains(string(p["execution"]), "no more records") {
		t.Fatal("terminal page invented another navigation action", end)
	}
	// A page can have a subset of plans; never issue calls for absent rows.
	for len(s.Queries) < 35 {
		s.Queries = append(s.Queries, s.Queries[0])
		s.Queries[len(s.Queries)-1].ID = string(rune('a' + len(s.Queries)))
	}
	for _, offset := range []int{0, 16, 32} {
		page, err := traceCatalogPage(s, path, offset)
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Records []struct {
				Kind string `json:"record_kind"`
				ID   string `json:"id"`
			}
			Calls map[string]json.RawMessage `json:"query_calls"`
		}
		if err := json.Unmarshal([]byte(page), &v); err != nil {
			t.Fatal(err)
		}
		plans := map[string]bool{}
		for _, r := range v.Records {
			if r.Kind == "current_query_plan" {
				plans[r.ID] = true
			}
		}
		if len(plans) != len(v.Calls) {
			t.Fatal("calls do not match current page")
		}
		for id := range v.Calls {
			if !plans[id] {
				t.Fatal("call from unshown plan")
			}
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	bus.Ctx = canceled
	out := types.ToolResult{ToolName: "read_file", Success: true, Summary: "original"}
	finishTraceCatalogReadNavigation(bus, path, data, true, &out)
	if out.Refinement != nil || out.Summary != "original" {
		t.Fatal("canceled read decorated navigation")
	}
}
