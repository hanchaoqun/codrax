package tracecatalog

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

func hmc314NavigationFixture(t *testing.T) (*Catalog, []QueryRecord) {
	t.Helper()
	root := t.TempDir()
	putFile(t, filepath.Join(root, "one", "capture"), "first source")
	putFile(t, filepath.Join(root, "two", "capture"), "second source")
	c := discover(t, root)
	var queries []QueryRecord
	for _, a := range c.Snapshot().Artifacts {
		bind(t, c, a)
		q, err := c.RegisterDeclaredQuery(context.Background(), Plan{ArtifactID: a.ID, Object: Object{Kind: "thread", ID: "101", Scope: "process:7"}, View: "event_search", Window: &Window{StartNS: 10, EndNS: 50}, Parameters: json.RawMessage(`{"view":"event_search","pid":101,"limit":40}`)})
		if err != nil {
			t.Fatal(err)
		}
		queries = append(queries, q)
	}
	if _, err := c.SelectRepresentatives(context.Background(), RepresentativeOptions{PerCluster: 1}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		finish(t, c, queries[0], OutcomeSuccess)
	}
	finish(t, c, queries[1], OutcomeFailure)
	finish(t, c, queries[1], OutcomeSuccess)
	return c, queries
}

func TestHMC314RepresentativeNavigationExactGeneration(t *testing.T) {
	c, queries := hmc314NavigationFixture(t)
	// A successful extra query on the same source/thread is not a selected
	// event search: its resource view and endpoint contract stay independent.
	exclusive := false
	extra, err := c.RegisterPlannedQuery(context.Background(), Plan{ArtifactID: queries[0].ArtifactID, Object: queries[0].Object, View: "resource_stack", Window: &Window{StartNS: 10, EndNS: 50, EndInclusive: &exclusive}, Parameters: json.RawMessage(`{"view":"resource_stack","pid":101,"limit":40}`)})
	if err != nil {
		t.Fatal(err)
	}
	finish(t, c, extra, OutcomeSuccess)
	before := c.CurrentExecution()
	encodedBefore, _ := json.Marshal(before)
	rows := before.RepresentativeNavigation()
	if len(rows) != 2 {
		t.Fatalf("extra query polluted selected scope: %+v", rows)
	}
	for i, q := range queries {
		r := rows[q.ID]
		want := map[Outcome]int{OutcomeSuccess: 3}
		if i == 1 {
			want = map[Outcome]int{OutcomeFailure: 1, OutcomeSuccess: 1}
		}
		if !reflect.DeepEqual(r.AttemptOutcomes, want) || r.Origin != QueryOriginDeclared || r.Specification.View != "event_search" || r.Specification.Window.EndInclusive != nil || r.Specification.Object != q.Object {
			t.Fatalf("source/query/endpoint/execution identity changed: %+v", r)
		}
		data, _ := json.Marshal(r)
		var restored RepresentativeQueryNavigation
		if err := json.Unmarshal(data, &restored); err != nil || !reflect.DeepEqual(restored, r) {
			t.Fatalf("navigation JSON changed coordinates: %v", err)
		}
	}
	finish(t, c, queries[0], OutcomeFailure)
	after := c.CurrentExecution()
	if rows[queries[0].ID].AttemptOutcomes[OutcomeFailure] != 0 || after.RepresentativeNavigation()[queries[0].ID].AttemptOutcomes[OutcomeFailure] != 1 {
		t.Fatal("historical and current generations mixed")
	}
	// A mismatched old selection cannot borrow a newer index's outcome history.
	after.Selection = before.Selection
	if _, ok := after.RepresentativeNavigation()[queries[0].ID]; ok {
		t.Fatal("mixed generation restored a current execution summary")
	}
	if len(after.RepresentativeNavigation()) != 1 {
		t.Fatal("unaffected source was lost")
	}
	row := rows[queries[0].ID]
	row.AttemptOutcomes[OutcomeSuccess] = 99
	row.Specification.Parameters[0] = '!'
	row.Specification.Window.StartNS = 123
	encodedAfter, _ := json.Marshal(before)
	if string(encodedAfter) != string(encodedBefore) {
		t.Fatal("projection mutated detached original history")
	}
}

func TestHMC314RepresentativeNavigationRejectsIdentityBorrowing(t *testing.T) {
	c, _ := hmc314NavigationFixture(t)
	cases := map[string]func(*ExecutionView){
		"catalog": func(e *ExecutionView) { e.Selection.CatalogID = "foreign" },
		"query":   func(e *ExecutionView) { e.Selection.Clusters[0].Members[0].QueryID = "foreign" },
		"artifact": func(e *ExecutionView) {
			e.Selection.Clusters[0].Members[0].ArtifactID = e.Selection.Clusters[0].Members[1].ArtifactID
		},
		"artifact_revision": func(e *ExecutionView) { e.Selection.Clusters[0].Members[0].ArtifactRevision = "older" },
		"source_revision":   func(e *ExecutionView) { e.Selection.Clusters[0].Members[0].SourceRevision = "older" },
		"source_path": func(e *ExecutionView) {
			e.Selection.Clusters[0].Members[0].Path = e.Selection.Clusters[0].Members[1].Path
		},
		"source_status":   func(e *ExecutionView) { e.Selection.Clusters[0].Members[0].SourceStatus = "stale" },
		"outcome":         func(e *ExecutionView) { e.Selection.Clusters[0].Members[0].Outcome = OutcomeEmpty },
		"attempt_count":   func(e *ExecutionView) { e.Selection.Clusters[0].Members[0].Attempts++ },
		"attempt_id":      func(e *ExecutionView) { e.Selection.Clusters[0].Members[0].LatestAttempt.ID = "older" },
		"missing_attempt": func(e *ExecutionView) { e.Selection.Clusters[0].Members[0].LatestAttempt = nil },
		"object":          func(e *ExecutionView) { e.Selection.Clusters[0].Specification.Object.Kind = "process" },
		"thread":          func(e *ExecutionView) { e.Selection.Clusters[0].Specification.Object.ID = "202" },
		"scope":           func(e *ExecutionView) { e.Selection.Clusters[0].Specification.Object.Scope = "process:8" },
		"view":            func(e *ExecutionView) { e.Selection.Clusters[0].Specification.View = "resource_stack" },
		"window":          func(e *ExecutionView) { e.Selection.Clusters[0].Specification.Window.StartNS++ },
		"endpoint":        func(e *ExecutionView) { v := false; e.Selection.Clusters[0].Specification.Window.EndInclusive = &v },
		"filter": func(e *ExecutionView) {
			e.Selection.Clusters[0].Specification.Parameters = json.RawMessage(`{"view":"event_search","pid":101,"limit":1}`)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			e := c.CurrentExecution()
			id := e.Selection.Clusters[0].Members[0].QueryID
			mutate(&e)
			if _, ok := e.RepresentativeNavigation()[id]; ok {
				t.Fatal("foreign identity/history was borrowed")
			}
			if len(c.CurrentExecution().RepresentativeNavigation()) != 2 {
				t.Fatal("navigation mutation changed live catalog")
			}
		})
	}
}
