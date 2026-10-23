package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func scopedRepresentativesBus285(t *testing.T) *types.BusContext {
	t.Helper()
	root, work := t.TempDir(), t.TempDir()
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.data", "b.data", "c.data", "d.data"} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return &types.BusContext{RepoRoot: root, WorkDir: work, Mutable: types.NewMutableState("compare captures"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(work, "prepared")})}
}

func catalogCall285(t *testing.T, bus *types.BusContext, raw string) map[string]json.RawMessage {
	t.Helper()
	if err := toolparam.Validate([]byte(raw), (&TraceCatalog{}).Parameters()); err != nil {
		t.Fatal(err)
	}
	out, err := (&TraceCatalog{}).Execute(bus, []byte(raw))
	if err != nil || !out.Success {
		t.Fatalf("catalog: %v %s", err, out.Summary)
	}
	var page map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out.Summary), &page); err != nil {
		t.Fatal(err)
	}
	if len(out.Observations) != 0 || out.ReadCoverage != nil || out.TraceQuerySourceRead.Path() != "" {
		t.Fatal("selection minted evidence")
	}
	return page
}

func TestHMC285DiscoverExplicitRepresentativesScopesCurrentQuerySpecs(t *testing.T) {
	bus := scopedRepresentativesBus285(t)
	initial := catalogCall285(t, bus, `{"queries":[{"view":"event_search","event_field_filters":[{"field":"appid","op":"eq","value":101}],"time_start":10,"time_end":10.05},{"view":"thread_timeline","time_start":10,"time_end":10.05}]}`)
	if string(initial["representative_selection_state"]) != `"not_selected"` {
		t.Fatal("discovery state did not distinguish actual selection")
	}
	c := bus.Mutable.TraceCatalogs()[0]
	if _, selected := c.Representatives(); selected {
		t.Fatal("discovery automatically selected")
	}
	out, err := (&TraceQuery{}).Execute(bus, []byte(`{"source":"path","path":"a.data","view":"resource_stack","pid":101,"time_start":0,"time_end":100}`))
	if err != nil || !out.Success {
		t.Fatalf("ordinary exploration: %v %s", err, out.Summary)
	}
	before := c.CurrentExecution()
	page := catalogCall285(t, bus, `{"action":"discover","queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05},{"view":"resource_stack","pid":202,"time_start":10,"time_end":10.05}],"representatives":{"per_cluster":2,"max_clusters":2}}`)
	s, ok := c.Representatives()
	if !ok || len(s.Clusters) != 2 || s.Coverage.Members != 8 || s.Coverage.Selected != 4 || s.Coverage.BudgetNotSelected != 4 || len(s.NewQueryIDs) != 0 {
		t.Fatalf("explicit same-call specs contaminated by prior work: %+v", s)
	}
	if len(c.Snapshot().Queries) != 17 || c.CurrentExecution().Coverage.Attempts != before.Coverage.Attempts {
		t.Fatal("selection dropped or executed existing work")
	}
	var snapshotPath string
	if err := json.Unmarshal(page["selection_snapshot"], &snapshotPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Excluded []string `json:"excluded_query_ids"`
	}
	if err := json.Unmarshal(data, &saved); err != nil || len(saved.Excluded) != 9 {
		t.Fatalf("prior outside-scope roster lost: %v %s", err, data)
	}
	var calls map[string]json.RawMessage
	if err := json.Unmarshal(page["query_calls"], &calls); err != nil || len(calls) != 4 {
		t.Fatalf("selected calls: %v %s", err, page["query_calls"])
	}
	for _, call := range calls {
		got, err := (&TraceQuery{}).Execute(bus, call)
		if err != nil || !got.Success {
			t.Fatalf("normal selected query: %v %s", err, got.Summary)
		}
	}
	statusArgs, _ := json.Marshal(map[string]any{"action": "representative_status", "catalog_id": c.ID()})
	catalogCall285(t, bus, string(statusArgs))
	ordinaryStatus, _ := json.Marshal(map[string]any{"action": "status", "catalog_id": c.ID()})
	statusPage := catalogCall285(t, bus, string(ordinaryStatus))
	if string(statusPage["representative_selection_state"]) != `"selected"` || len(statusPage["representative_status_call"]) == 0 {
		t.Fatal("ordinary status omitted fixed-selection navigation")
	}
	after, _ := c.Representatives()
	if after.SelectionID != s.SelectionID || after.Coverage.SelectedOutcomes[tracecatalog.OutcomeSuccess] != 4 || after.Coverage.Outcomes[tracecatalog.OutcomeNotExecuted] != 4 {
		t.Fatalf("frozen execution/unselected state: %+v", after.Coverage)
	}
	// Explicit narrow selection must not deny ordinary work outside that set.
	var excludedCall json.RawMessage
	for _, row := range c.CurrentExecution().NavigationQueries() {
		if row.Query.ID == saved.Excluded[0] {
			excludedCall, _ = json.Marshal(row.QueryCall)
		}
	}
	if len(excludedCall) == 0 {
		t.Fatal("outside-scope query disappeared")
	}
	out, err = (&TraceQuery{}).Execute(bus, excludedCall)
	if err != nil || !out.Success {
		t.Fatalf("outside selection denied: %v %s", err, out.Summary)
	}
	catalogCall285(t, bus, `{"queries":[{"view":"resource_stack","pid":303,"time_start":10,"time_end":10.05}]}`)
	newer, _ := c.Representatives()
	if newer.SelectionID != s.SelectionID || len(newer.NewQueryIDs) != 4 || !reflect.DeepEqual(newer.Options, s.Options) {
		t.Fatalf("new plans reranked or old exclusions called new: %+v", newer)
	}
}

func TestHMC285ExplicitDiscoverSelectionRequiresPlansWithoutChangingExistingSelection(t *testing.T) {
	bus := scopedRepresentativesBus285(t)
	catalogCall285(t, bus, `{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05}]}`)
	c := bus.Mutable.TraceCatalogs()[0]
	args, _ := json.Marshal(map[string]any{"action": "representatives", "catalog_id": c.ID()})
	catalogCall285(t, bus, string(args))
	old, _ := c.Representatives()
	oldQueries := len(c.Snapshot().Queries)
	for _, raw := range []string{
		`{"action":"discover","representatives":{"per_cluster":2}}`,
		`{"action":"discover","offset":1,"queries":[{"view":"resource_stack","pid":202}],"representatives":{}}`,
		`{"action":"discover","queries":[{"view":"resource_stack","path":"a.data"}],"representatives":{}}`,
		`{"action":"discover","queries":[{"view":"resource_stack"}],"representatives":{"per_cluster":6}}`,
		`{"action":"discover","queries":[{"pid":202}],"representatives":{}}`,
	} {
		out, _ := (&TraceCatalog{}).Execute(bus, []byte(raw))
		if out.Success {
			t.Fatalf("invalid combined selection accepted: %s", raw)
		}
		current, _ := c.Representatives()
		if current.SelectionID != old.SelectionID {
			t.Fatal("failed admission changed selection")
		}
		if len(c.Snapshot().Queries) != oldQueries {
			t.Fatal("invalid combined action installed default/partial plans")
		}
	}
}
