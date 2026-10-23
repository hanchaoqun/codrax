package tracecatalog

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestHMC285ScopedSelectionKeepsExcludedNewAndExecutedSeparate(t *testing.T) {
	c := representativeFixture248(t, 4, 3)
	var ids []string
	for _, q := range c.Snapshot().Queries {
		if q.Object.ID == "100" || q.Object.ID == "101" {
			ids = append(ids, q.ID)
		}
	}
	s, err := c.SelectRepresentativesForQueries(context.Background(), RepresentativeOptions{PerCluster: 2}, ids)
	if err != nil {
		t.Fatal(err)
	}
	if s.SelectionScope != "explicit_query_ids" || s.Coverage.Members != 8 || s.Coverage.Selected != 4 || s.Coverage.Excluded != 4 || len(s.ExcludedQueryIDs) != 4 || len(s.NewQueryIDs) != 0 {
		t.Fatalf("wrong scoped roster: %+v", s)
	}
	selected := selected248(s)
	q, _ := c.Query(selected[0])
	for _, a := range c.Snapshot().Artifacts {
		if a.ID == q.ArtifactID {
			bind(t, c, a)
		}
	}
	finish(t, c, q, OutcomeEmpty)
	q, _ = c.Query(s.ExcludedQueryIDs[0])
	finish(t, c, q, OutcomeFailure)
	a := c.Snapshot().Artifacts[0]
	plan(t, c, a.ID, "new", nil)
	now, _ := c.Representatives()
	if now.SelectionID != s.SelectionID || !reflect.DeepEqual(selected248(now), selected) || !reflect.DeepEqual(now.ExcludedQueryIDs, s.ExcludedQueryIDs) || len(now.NewQueryIDs) != 1 || now.Coverage.SelectedOutcomes[OutcomeEmpty] != 1 || now.Coverage.Outcomes[OutcomeFailure] != 0 {
		t.Fatalf("scope/outcome conflation: %+v", now)
	}
	// Snapshot mutation cannot widen the private selection or move exclusions.
	now.ExcludedQueryIDs[0] = "forged"
	frozen, _ := c.Representatives()
	if !reflect.DeepEqual(frozen.ExcludedQueryIDs, s.ExcludedQueryIDs) {
		t.Fatal("detached exclusions mutated live selection")
	}
	// The old explicit whole-catalog action remains available and uses all work.
	all, err := c.SelectRepresentatives(context.Background(), RepresentativeOptions{PerCluster: 2})
	if err != nil || all.SelectionScope != "all_current_queries" || all.Coverage.Members != 13 || all.Coverage.Excluded != 0 || len(all.ExcludedQueryIDs) != 0 {
		t.Fatalf("legacy scope changed: %v %+v", err, all)
	}
}

func TestHMC285ScopedSelectionAdmissionCloneAndCurrentSnapshot(t *testing.T) {
	c := representativeFixture248(t, 2, 2)
	old := select248(t, c, RepresentativeOptions{})
	ids := []string{c.Snapshot().Queries[0].ID}
	for _, invalid := range [][]string{nil, {}, {"unknown"}, {ids[0], "unknown"}} {
		if _, err := c.SelectRepresentativesForQueries(context.Background(), RepresentativeOptions{}, invalid); err == nil {
			t.Fatalf("invalid scope accepted: %v", invalid)
		}
		got, _ := c.Representatives()
		if got.SelectionID != old.SelectionID {
			t.Fatal("invalid scope changed fixed selection")
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.SelectRepresentativesForQueries(canceled, RepresentativeOptions{}, ids); err == nil {
		t.Fatal("canceled selection accepted")
	}
	got, _ := c.Representatives()
	if got.SelectionID != old.SelectionID {
		t.Fatal("canceled selection changed fixed selection")
	}
	s, err := c.SelectRepresentativesForQueries(context.Background(), RepresentativeOptions{}, append(ids, ids...))
	if err != nil || s.Coverage.Members != 1 || s.Coverage.Excluded != 3 {
		t.Fatalf("duplicate exact IDs changed population: %v %+v", err, s)
	}
	fork := c.Clone()
	copy, _ := fork.Representatives()
	if !reflect.DeepEqual(copy.ExcludedQueryIDs, s.ExcludedQueryIDs) || copy.SelectionScope != s.SelectionScope {
		t.Fatal("clone dropped fixed scope")
	}
	// JSON remains only navigation, with no route to restore query permission.
	raw, _ := json.Marshal(s)
	if _, err := DecodeSnapshot(raw); err == nil {
		t.Fatal("selection navigation restored catalog authority")
	}
}
