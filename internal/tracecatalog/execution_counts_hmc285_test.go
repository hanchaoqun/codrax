package tracecatalog

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHMC285ExecutionCountsSeparateOriginsRecordsAndAttempts(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "capture"), "trace")
	c := discover(t, root)
	a := c.Snapshot().Artifacts[0]
	bind(t, c, a)
	declare := func(id string) QueryRecord {
		t.Helper()
		q, err := c.RegisterDeclaredQuery(context.Background(), Plan{ArtifactID: a.ID, Object: Object{Kind: "thread", ID: id}, View: "resource_stack"})
		if err != nil {
			t.Fatal(err)
		}
		return q
	}
	failedThenSucceeded := declare("41")
	empty := declare("42")
	_ = declare("43")
	observed := plan(t, c, a.ID, "44", nil)
	finish(t, c, failedThenSucceeded, OutcomeFailure)
	finish(t, c, failedThenSucceeded, OutcomeSuccess)
	finish(t, c, empty, OutcomeEmpty)
	finish(t, c, observed, OutcomeFailure)
	check := func(catalog *Catalog, with, without, attempts, observed int) {
		t.Helper()
		v := catalog.CurrentExecution()
		wire, err := json.Marshal(v.Coverage)
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		if err := json.Unmarshal(wire, &data); err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]int{"query_records_with_attempts": with, "query_records_without_attempts": without, "attempts": attempts, "observed_queries": observed} {
			if got, ok := data[key].(float64); !ok || int(got) != want {
				t.Errorf("%s got=%v want=%d; %s", key, data[key], want, wire)
			}
		}
	}
	check(c, 3, 1, 4, 1)
	check(c.Clone(), 3, 1, 4, 1)
	coverageJSON, _ := json.Marshal(c.CurrentExecution().Coverage)
	var projection struct {
		Attempts map[Outcome]int `json:"attempt_outcomes"`
	}
	if err := json.Unmarshal(coverageJSON, &projection); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projection.Attempts, map[Outcome]int{OutcomeFailure: 2, OutcomeSuccess: 1, OutcomeEmpty: 1}) {
		t.Errorf("attempt outcomes lost retry: %s", coverageJSON)
	}
	// Declaring an already observed query changes its registration origin,
	// not the number of actual attempted records or attempts.
	if _, err := c.RegisterDeclaredQuery(context.Background(), observed.Plan); err != nil {
		t.Fatal(err)
	}
	check(c, 3, 1, 4, 0)
	encoded, _ := json.Marshal(c.Snapshot())
	history, err := DecodeSnapshot(encoded)
	if err != nil {
		t.Fatal(err)
	}
	var plans []Plan
	for _, q := range history.Queries {
		plans = append(plans, q.Plan)
	}
	resumed, err := Resume(context.Background(), history.Root, history, plans)
	if err != nil {
		t.Fatal(err)
	}
	check(resumed, 0, 4, 0, 0)
}

func TestHMC285ExecutionSelectionSnapshotIsDetachedAndCurrent(t *testing.T) {
	c := representativeFixture248(t, 3, 2)
	before := c.CurrentExecution()
	if before.Selection != nil || before.SelectionState() != "not_selected" {
		t.Fatal("discovery invented a selection")
	}
	ids := []string{before.Snapshot.Queries[0].ID}
	s, err := c.SelectRepresentativesForQueries(context.Background(), RepresentativeOptions{}, ids)
	if err != nil {
		t.Fatal(err)
	}
	v := c.CurrentExecution()
	if v.Selection == nil || v.SelectionState() != "selected" || v.Selection.SelectionID != s.SelectionID {
		t.Fatal("atomic execution view omitted its real selection")
	}
	v.Selection.ExcludedQueryIDs[0] = "forged"
	v.Selection.Clusters[0].Members[0].Selected = false
	v.Coverage.AttemptOutcomes[OutcomeSuccess] = 999
	fresh := c.CurrentExecution()
	if !reflect.DeepEqual(fresh.Selection, &s) || len(fresh.Coverage.AttemptOutcomes) != 0 {
		t.Fatal("detached presentation mutated live execution/selection")
	}
	if before.SelectionState() != "not_selected" {
		t.Fatal("prior view changed after selection")
	}
}

func TestHMC285ExportReceiptAttemptCountsAreDetached(t *testing.T) {
	original := ExportReceipt{ExecutionCoverage: ExecutionCoverage{AttemptOutcomes: map[Outcome]int{OutcomeFailure: 1, OutcomeSuccess: 1}}}
	cloned := cloneExportReceipt(original)
	cloned.ExecutionCoverage.AttemptOutcomes[OutcomeFailure] = 99
	if original.ExecutionCoverage.AttemptOutcomes[OutcomeFailure] != 1 {
		t.Fatal("delivery receipt shared mutable attempt counts")
	}
}
