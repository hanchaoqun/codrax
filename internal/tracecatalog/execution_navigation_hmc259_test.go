package tracecatalog

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestHMC259ExecutionSpecificationGroupsKeepExactAxesAndOrigins(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "a"), "trace a")
	putFile(t, filepath.Join(root, "b"), "trace b")
	c := discover(t, root)
	artifacts := c.Snapshot().Artifacts
	base := Plan{Object: Object{Kind: "thread", ID: "101"}, View: "resource_stack", Window: &Window{StartNS: 0, EndNS: 1000}, Parameters: json.RawMessage(`{"view":"resource_stack","pid":101}`)}
	add := func(a Artifact, p Plan, declared bool) QueryRecord {
		t.Helper()
		p.ArtifactID = a.ID
		var q QueryRecord
		var err error
		if declared {
			q, err = c.RegisterDeclaredQuery(context.Background(), p)
		} else {
			q, err = c.RegisterPlannedQuery(context.Background(), p)
		}
		if err != nil {
			t.Fatal(err)
		}
		return q
	}
	q := add(artifacts[0], base, true)
	add(artifacts[1], base, true)
	variants := []Plan{base, base, base, base, base}
	variants[0].Object = Object{Kind: "process", ID: "101"}
	variants[1].Window = &Window{StartNS: 1, EndNS: 1000}
	inclusive := true
	variants[2].Window = &Window{StartNS: 0, EndNS: 1000, EndInclusive: &inclusive}
	variants[3].Parameters = json.RawMessage(`{"view":"resource_stack","pid":101,"pattern":"a"}`)
	variants[4].Parameters = json.RawMessage(`{"view":"resource_stack","pid":101,"pattern":"b"}`)
	for _, p := range variants {
		add(artifacts[0], p, true)
	}
	// Same query specification on another source is observed-only, not part of
	// the declaration denominator, even when the rest of the coordinates match.
	observed := base
	observed.Parameters = json.RawMessage(`{"view":"resource_stack","pid":101,"path":"other-path","source":"path"}`)
	add(artifacts[1], observed, false)
	finish(t, c, q, OutcomeFailure)
	finish(t, c, q, OutcomeFailure)
	view := c.CurrentExecution()
	groups := view.SpecificationGroups()
	if len(groups) != 7 {
		t.Fatalf("distinct axes/origins collapsed: %+v", groups)
	}
	queries, attempts, doubled, observedGroups := 0, 0, 0, 0
	for _, g := range groups {
		queries += g.Queries
		attempts += g.Attempts
		if g.Queries == 2 {
			doubled++
		}
		if g.Origin == QueryOriginObserved {
			observedGroups++
		}
	}
	if queries != 8 || attempts != 2 || doubled != 1 || observedGroups != 1 {
		t.Fatalf("group census: %+v", groups)
	}
	rows := view.NavigationQueries()
	for i := range rows {
		if len(rows[i].Query.Attempts) != 0 {
			t.Fatal("full retry history leaked into projection")
		}
		if rows[i].Query.ID == q.ID {
			if rows[i].Attempts != 2 || rows[i].AttemptOutcomes[OutcomeFailure] != 2 {
				t.Fatal("failed attempts disappeared")
			}
			rows[i].AttemptOutcomes[OutcomeFailure] = 99
			rows[i].Query.Parameters[0] = 'x'
		}
	}
	if got, _ := c.Query(q.ID); len(got.Attempts) != 2 || got.Parameters[0] != '{' {
		t.Fatal("detached projection mutated live plan")
	}
}
