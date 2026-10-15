package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC248RepresentativesPublicQueryAndCoverage(t *testing.T) {
	bus, _ := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	raw, _ := json.Marshal(map[string]any{"action": "representatives", "catalog_id": c.ID(), "representatives": map[string]any{"per_cluster": 1, "max_clusters": 2, "priority": "largest_input_first"}})
	out, err := (&TraceCatalog{}).Execute(bus, raw)
	if err != nil || !out.Success {
		t.Fatalf("public representative selection unavailable: %v %s", err, out.Summary)
	}
	var page struct {
		SelectionID string                     `json:"selection_id"`
		QueryCalls  map[string]json.RawMessage `json:"query_calls"`
		Coverage    struct {
			Members  int `json:"members"`
			Selected int `json:"selected"`
			Budget   int `json:"budget_not_selected"`
		} `json:"coverage"`
	}
	if err := json.Unmarshal([]byte(out.Summary), &page); err != nil {
		t.Fatal(err)
	}
	if page.SelectionID == "" || page.Coverage.Members != 6 || page.Coverage.Selected != 2 || page.Coverage.Budget != 4 || len(page.QueryCalls) != 2 {
		t.Fatalf("representative roster/coverage lost: %s", out.Summary)
	}
	if len(out.Observations) != 0 || out.ReadCoverage != nil || out.TraceQuerySourceRead.Path() != "" {
		t.Fatal("representative navigation minted evidence or source authority")
	}
	for _, call := range page.QueryCalls {
		result, err := (&TraceQuery{}).Execute(bus, call)
		if err != nil || !result.Success {
			t.Fatalf("returned call not executable: %v %s", err, result.Summary)
		}
	}
	counts := map[tracecatalog.Outcome]int{}
	for _, q := range c.Snapshot().Queries {
		counts[q.Outcome]++
	}
	if counts[tracecatalog.OutcomeSuccess] != 1 || counts[tracecatalog.OutcomeEmpty] != 1 || counts[tracecatalog.OutcomeNotExecuted] != 4 {
		t.Fatalf("selection pretended to execute unselected members: %+v", counts)
	}
	raw, _ = json.Marshal(map[string]any{"action": "representative_status", "catalog_id": c.ID()})
	status, err := (&TraceCatalog{}).Execute(bus, raw)
	if err != nil || !status.Success {
		t.Fatalf("representative comparison status unavailable: %v %s", err, status.Summary)
	}
	var current struct {
		SelectionID string `json:"selection_id"`
		Coverage    struct {
			SelectedOutcomes map[string]int `json:"selected_outcomes"`
		} `json:"coverage"`
	}
	if err := json.Unmarshal([]byte(status.Summary), &current); err != nil || current.SelectionID != page.SelectionID || current.Coverage.SelectedOutcomes["success"] != 1 || current.Coverage.SelectedOutcomes["empty"] != 1 {
		t.Fatalf("current outcomes or frozen selection changed: %v %s", err, status.Summary)
	}
}

func TestHMC248RepresentativePublicAdmissionAndNavigationOnly(t *testing.T) {
	bus, _ := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	for _, fields := range []map[string]any{
		{"action": "representative_status"},
		{"action": "status", "representatives": map[string]any{}},
		{"action": "representatives", "root": "."},
		{"action": "representatives", "snapshot_path": "old.json"},
		{"action": "representatives", "queries": []any{map[string]any{"view": "event_search"}}},
		{"action": "representatives", "offset": 1},
		{"action": "representatives", "representatives": map[string]any{"per_cluster": 6}},
		{"action": "representatives", "representatives": map[string]any{"priority": "model_similarity"}},
		{"action": "representatives", "representatives": map[string]any{"cluster_label": "same cause"}},
	} {
		fields["catalog_id"] = c.ID()
		raw, _ := json.Marshal(fields)
		out, err := (&TraceCatalog{}).Execute(bus, raw)
		if out.Success {
			t.Fatalf("invalid admission accepted: %s err=%v result=%s", raw, err, out.Summary)
		}
		if _, ok := c.Representatives(); ok {
			t.Fatal("failed admission changed fixed selection")
		}
	}
	raw, _ := json.Marshal(map[string]any{"action": "representatives", "catalog_id": c.ID()})
	for _, mode := range []string{"foreign_run", "revoked_root"} {
		other := bus.ShallowClone()
		if mode == "foreign_run" {
			other.Mutable = types.NewMutableState("fresh")
		} else {
			other.RepoRoot = t.TempDir()
		}
		out, _ := (&TraceCatalog{}).Execute(other, raw)
		if out.Success {
			t.Fatalf("%s acquired selection authority", mode)
		}
	}
	if _, ok := c.Representatives(); ok {
		t.Fatal("foreign or revoked request mutated current selection")
	}
	out, err := (&TraceCatalog{}).Execute(bus, raw)
	if err != nil || !out.Success {
		t.Fatalf("valid current selection: %v %s", err, out.Summary)
	}
	var page struct {
		Snapshot string `json:"selection_snapshot"`
		SHA      string `json:"selection_snapshot_sha256"`
		Catalog  string `json:"saved_index"`
	}
	if err := json.Unmarshal([]byte(out.Summary), &page); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(page.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != page.SHA || page.Snapshot == page.Catalog {
		t.Fatal("independent navigation snapshot is not exact")
	}
	if _, err := tracecatalog.DecodeSnapshot(data); err == nil {
		t.Fatal("selection JSON accepted as restorable catalog")
	}
	var snapshot tracecatalog.RepresentativeSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil || !snapshot.NavigationOnly || snapshot.Coverage.Members != 6 {
		t.Fatalf("full snapshot: %v %+v", err, snapshot)
	}
	if len(bus.Mutable.TraceQueryBlobRefs()) != 0 {
		t.Fatal("selection snapshot became runtime query payload")
	}
}

func TestHMC248RepresentativePublicPaginationAndExactCalls(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	body, err := os.ReadFile(filepath.Join(root, "capture.data"))
	if err != nil {
		t.Fatal(err)
	}
	// Exact physical names never become grouping criteria or delimiter syntax.
	for _, name := range []string{"comma,equals=quote\".data", strings.Repeat("long", 40) + ".data", "third.data", "fourth.data"} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	out, err := (&TraceCatalog{}).Execute(bus, []byte(`{"queries":[{"view":"resource_stack","pid":101},{"view":"resource_stack","pid":999},{"view":"resource_stack","pid":202}]}`))
	if err != nil || !out.Success {
		t.Fatalf("discover: %v %s", err, out.Summary)
	}
	c := bus.Mutable.TraceCatalogs()[0]
	raw, _ := json.Marshal(map[string]any{"action": "representatives", "catalog_id": c.ID(), "representatives": map[string]any{"per_cluster": 2, "max_clusters": 2}})
	seen, selected, pages := map[string]bool{}, map[string]bool{}, 0
	for len(raw) > 0 {
		if err := toolparam.Validate(raw, (&TraceCatalog{}).Parameters()); err != nil {
			t.Fatalf("returned page call not schema-valid: %v", err)
		}
		out, err = (&TraceCatalog{}).Execute(bus, raw)
		if err != nil || !out.Success || len(out.Summary) > 64<<10 {
			t.Fatalf("whole page: %v %s", err, out.Summary)
		}
		var page struct {
			Records []struct {
				Kind     string                            `json:"record_kind"`
				Member   tracecatalog.RepresentativeMember `json:"member"`
				Coverage map[string]json.RawMessage        `json:"coverage"`
			} `json:"records"`
			Calls map[string]json.RawMessage `json:"query_calls"`
			Next  json.RawMessage            `json:"next_call"`
			Total int                        `json:"total_records"`
		}
		if err := json.Unmarshal([]byte(out.Summary), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Records) > 16 || len(page.Records) == 0 {
			t.Fatal("invalid navigation page size")
		}
		visible := map[string]bool{}
		for _, r := range page.Records {
			if r.Kind == "query_specification_cluster" {
				for _, inappropriate := range []string{"candidates", "clusters", "unmatched_candidates", "new_queries_not_in_selection"} {
					if _, ok := r.Coverage[inappropriate]; ok {
						t.Fatalf("cluster published inapplicable global zero %s", inappropriate)
					}
				}
				continue
			}
			if r.Kind != "cluster_member" {
				continue
			}
			m := r.Member
			if seen[m.QueryID] || m.Path == "" || m.ArtifactRevision == "" {
				t.Fatalf("member identity lost/duplicated: %+v", m)
			}
			seen[m.QueryID] = true
			if m.Selected {
				selected[m.QueryID] = true
				visible[m.QueryID] = true
			}
		}
		if len(visible) != len(page.Calls) {
			t.Fatal("calls do not match current page selected members")
		}
		for id, call := range page.Calls {
			if !visible[id] {
				t.Fatal("foreign/hidden query call")
			}
			if err := toolparam.Validate(call, (&TraceQuery{}).Parameters()); err != nil {
				t.Fatalf("call schema: %v %s", err, call)
			}
			var ref struct {
				Ref struct {
					CatalogID string `json:"catalog_id"`
					QueryID   string `json:"query_id"`
				} `json:"catalog_query_ref"`
			}
			if err := json.Unmarshal(call, &ref); err != nil || ref.Ref.CatalogID != c.ID() || ref.Ref.QueryID != id {
				t.Fatal("call ID changed")
			}
		}
		pages++
		raw = page.Next
	}
	if pages < 2 || len(seen) != 21 || len(selected) != 4 {
		t.Fatalf("coverage pagination: pages=%d members=%d selected=%d", pages, len(seen), len(selected))
	}
}
