package context_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc285CatalogCall(t *testing.T, bus *types.BusContext, params any) string {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	out, err := (&tool.TraceCatalog{}).Execute(bus, raw)
	if err != nil || !out.Success {
		t.Fatalf("catalog call: %v %s", err, out.Summary)
	}
	return out.Summary
}

func hmc285ContextCoverage(t *testing.T, content string) tracecatalog.ExecutionCoverage {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		if raw, ok := strings.CutPrefix(line, "current_execution="); ok {
			var coverage tracecatalog.ExecutionCoverage
			if err := json.Unmarshal([]byte(raw), &coverage); err != nil {
				t.Fatal(err)
			}
			return coverage
		}
	}
	t.Fatalf("current execution missing: %s", content)
	return tracecatalog.ExecutionCoverage{}
}

func TestHMC285PublicSelectionExecutionHandoff(t *testing.T) {
	bus := hmc259CatalogBus(t)
	c := bus.Mutable.TraceCatalogs()[0]
	stages := []types.PipelineStage{types.StageExplore, types.StageExtract, types.StageFinalize}
	check := func(selected bool, attempted, pending, attempts int) {
		t.Helper()
		for _, stage := range stages {
			content := hmc259CatalogContext(t, bus, stage)
			wantState := `representative_selection_state="not_selected"`
			if selected {
				wantState = `representative_selection_state="selected"`
			}
			if !strings.Contains(content, wantState) || len(content) > 24<<10 {
				t.Fatalf("%s: missing state or budget violation: %s", stage, content)
			}
			coverage := hmc285ContextCoverage(t, content)
			if coverage.QueriesWithAttempts != attempted || coverage.QueriesWithoutAttempts != pending || coverage.Attempts != attempts {
				t.Fatalf("%s: records/attempts conflated: %+v", stage, coverage)
			}
			if !reflect.DeepEqual(coverage, c.CurrentExecution().Coverage) {
				t.Fatalf("%s: context changed typed execution coverage", stage)
			}
		}
	}
	check(false, 0, 4, 0)
	status := hmc285CatalogCall(t, bus, map[string]any{"action": "status", "catalog_id": c.ID()})
	if !strings.Contains(status, `"representative_selection_state":"not_selected"`) {
		t.Fatalf("discovery/status invented a selection: %s", status)
	}
	query := func(raw json.RawMessage) {
		t.Helper()
		out, err := (&tool.TraceQuery{}).Execute(bus, raw)
		if err != nil || !out.Success {
			t.Fatalf("native query: %v %s", err, out.Summary)
		}
	}
	// Earlier exploration is a real successful call, but has a different exact
	// window from the two requested comparison specifications below.
	prior, _ := json.Marshal(map[string]any{"source": "path", "path": c.Snapshot().Artifacts[0].Path, "view": "resource_stack", "pid": 101, "time_start": 10, "time_end": 10.1})
	query(prior)
	check(false, 1, 4, 1)
	var oldQueryID string
	for id, origin := range c.CurrentExecution().QueryOrigins {
		if origin == tracecatalog.QueryOriginObserved {
			oldQueryID = id
		}
	}
	if oldQueryID == "" {
		t.Fatal("direct native exploration was not registered")
	}
	combined := hmc285CatalogCall(t, bus, map[string]any{
		"action": "discover", "representatives": map[string]any{"per_cluster": 1},
		"queries": []map[string]any{
			{"view": "resource_stack", "pid": 101, "time_start": 10, "time_end": 10.05},
			{"view": "resource_stack", "pid": 999, "time_start": 10, "time_end": 10.05},
		},
	})
	var page struct {
		Calls map[string]json.RawMessage `json:"query_calls"`
	}
	if err := json.Unmarshal([]byte(combined), &page); err != nil {
		t.Fatal(err)
	}
	selection, ok := c.Representatives()
	if !ok || selection.SelectionScope != "explicit_query_ids" || selection.Coverage.Members != 4 || selection.Coverage.Selected != 2 || selection.Coverage.Clusters != 2 || len(page.Calls) != 2 || !reflect.DeepEqual(selection.ExcludedQueryIDs, []string{oldQueryID}) {
		t.Fatalf("combined selection changed requested population: %+v / %s", selection, combined)
	}
	selectedIDs := map[string]bool{}
	var successID, unselectedEmptyID string
	for _, group := range selection.Clusters {
		var params struct {
			PID int `json:"pid"`
		}
		if err := json.Unmarshal(group.Specification.Parameters, &params); err != nil {
			t.Fatal(err)
		}
		if params.PID != 101 && params.PID != 999 {
			t.Fatalf("nonrequested target entered comparison: %s", group.Specification.Parameters)
		}
		for _, member := range group.Members {
			if member.Selected {
				selectedIDs[member.QueryID] = true
				if params.PID == 101 {
					successID = member.QueryID
				}
			} else if params.PID == 999 {
				unselectedEmptyID = member.QueryID
			}
		}
	}
	for _, stage := range stages {
		content := hmc259CatalogContext(t, bus, stage)
		calls := hmc259Calls(t, content)
		if len(calls) != 3 || len(calls[oldQueryID]) == 0 || !strings.Contains(content, "queries_outside_selection_scope_displayed=1 omitted=0") || !strings.Contains(content, `"new_queries_not_in_selection":0`) {
			t.Fatalf("%s: old exploration mixed with selected or later work: %s", stage, content)
		}
		for id := range selectedIDs {
			if string(calls[id]) != string(page.Calls[id]) {
				t.Fatalf("%s: exact selected query call lost for %s", stage, id)
			}
		}
	}
	// Execute the exact calls emitted through the stage context, not reconstructed
	// source/target arguments. Repeating one call must not create a new record.
	calls := hmc259Calls(t, hmc259CatalogContext(t, bus, types.StageExtract))
	for id := range selectedIDs {
		query(calls[id])
	}
	query(calls[successID])
	check(true, 3, 2, 4)
	coverage := c.CurrentExecution().Coverage
	if coverage.DeclaredPlans != 4 || coverage.ObservedQueries != 1 || coverage.AttemptOutcomes[tracecatalog.OutcomeSuccess] != 3 || coverage.AttemptOutcomes[tracecatalog.OutcomeEmpty] != 1 || coverage.PlannedOutcomes[tracecatalog.OutcomeNotExecuted] != 2 {
		t.Fatalf("retry/empty/latest-record outcomes conflated: %+v", coverage)
	}
	// Unselected is navigation priority, not a query prohibition. Ordinary status
	// still returns its exact call; reading either status must not rerank.
	status = hmc285CatalogCall(t, bus, map[string]any{"action": "status", "catalog_id": c.ID()})
	if err := json.Unmarshal([]byte(status), &page); err != nil || len(page.Calls[unselectedEmptyID]) == 0 {
		t.Fatalf("unselected current work became inaccessible: %v %s", err, status)
	}
	query(page.Calls[unselectedEmptyID])
	check(true, 4, 1, 5)
	hmc285CatalogCall(t, bus, map[string]any{"action": "representative_status", "catalog_id": c.ID()})
	current, _ := c.Representatives()
	if current.SelectionID != selection.SelectionID || current.Coverage.SelectedOutcomes[tracecatalog.OutcomeSuccess] != 1 || current.Coverage.SelectedOutcomes[tracecatalog.OutcomeEmpty] != 1 || current.Coverage.Outcomes[tracecatalog.OutcomeEmpty] != 2 {
		t.Fatalf("status/query reranked selection or promoted unselected outcome: %+v", current)
	}
	// A later discover adds independently executable work without retroactively
	// expanding the fixed selection or reclassifying earlier exclusions.
	hmc285CatalogCall(t, bus, map[string]any{"action": "discover", "queries": []map[string]any{{"view": "resource_stack", "pid": 202, "time_start": 10, "time_end": 10.05}}})
	current, _ = c.Representatives()
	if current.SelectionID != selection.SelectionID || len(current.NewQueryIDs) != 2 || !reflect.DeepEqual(current.ExcludedQueryIDs, []string{oldQueryID}) {
		t.Fatalf("later registration altered selection scope: %+v", current)
	}
	before, _ := json.Marshal(c.CurrentExecution())
	check(true, 4, 3, 5)
	for _, stage := range stages {
		content := hmc259CatalogContext(t, bus, stage)
		if !strings.Contains(content, "queries_outside_selection_scope_displayed=1 omitted=0") || !strings.Contains(content, "outside_selection_queries_displayed=2 omitted=0") {
			t.Fatalf("%s: excluded exploration and later additions collapsed: %s", stage, content)
		}
	}
	after, _ := json.Marshal(c.CurrentExecution())
	if string(before) != string(after) {
		t.Fatal("context projection mutated the atomic execution/selection view")
	}
}

func TestHMC285SelectionBudgetRetainsRealStateAndDetachedSnapshot(t *testing.T) {
	bus := hmc259CatalogBus(t)
	c := bus.Mutable.TraceCatalogs()[0]
	params, _ := json.Marshal(map[string]any{"view": "event_search", "pattern": strings.Repeat("long", 7800)})
	large, err := c.RegisterDeclaredQuery(context.Background(), tracecatalog.Plan{ArtifactID: c.Snapshot().Artifacts[0].ID, Object: tracecatalog.Object{Kind: "capture", ID: "all"}, View: "event_search", Parameters: params})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := c.SelectRepresentativesForQueries(context.Background(), tracecatalog.RepresentativeOptions{PerCluster: 1}, []string{large.ID})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(c.CurrentExecution())
	for _, stage := range []types.PipelineStage{types.StageExplore, types.StageExtract, types.StageFinalize} {
		content := hmc259CatalogContext(t, bus, stage)
		for _, want := range []string{`representative_selection_state="selected"`, selection.SelectionID, "comparison_groups_displayed=0 omitted=1", "selected_member_records_displayed=0 omitted=1"} {
			if !strings.Contains(content, want) {
				t.Fatalf("%s: budget hid actual selection state %q: %s", stage, want, content)
			}
		}
		if len(content) > 24<<10 || strings.Contains(content, `representative_selection_state="not_selected"`) || strings.Contains(content, strings.Repeat("long", 20)) {
			t.Fatal("budget changed selection state or split a query specification")
		}
	}
	// CurrentExecution publishes detached maps, slices and selection parameters.
	// Modifying consumer-side navigation never restores or changes authority.
	detached := c.CurrentExecution()
	detached.Selection.ExcludedQueryIDs[0] = "forged"
	detached.Selection.Clusters[0].Members[0].QueryID = "forged"
	detached.Selection.Clusters[0].Specification.Parameters[0] = '!'
	detached.Coverage.AttemptOutcomes[tracecatalog.OutcomeSuccess] = 99
	detached.QueryOrigins[large.ID] = tracecatalog.QueryOriginObserved
	detached.Snapshot.Queries[0].ID = "forged"
	after, _ := json.Marshal(c.CurrentExecution())
	if string(before) != string(after) {
		t.Fatal("consumer mutated live execution or representative selection")
	}
}
