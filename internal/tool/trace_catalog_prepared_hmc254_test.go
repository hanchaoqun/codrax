package tool

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

// The original and derived coordinates are exercised through the real public
// tools. A result being reusable by content does not erase a new invocation.
func TestHMC254PreparedQueriesRegisterOriginalAttempts(t *testing.T) {
	bus, c := exportCatalogFixture250(t)
	before := c.CurrentExecution()
	if len(bus.TraceInputPreparer.PreparedMaterials()) != 2 {
		t.Fatal("fixture did not prepare the two independent original captures")
	}
	for _, material := range bus.TraceInputPreparer.PreparedMaterials() {
		for _, pid := range []int{101, 999} {
			out := catalogPublicQuery(t, bus, material.QueryPath(), pid)
			if !out.Success {
				t.Fatalf("derived query failed: %s", out.Summary)
			}
		}
	}
	after := c.CurrentExecution()
	if after.Coverage.Attempts != before.Coverage.Attempts+4 {
		t.Fatalf("four real prepared-path queries disappeared from original execution: before=%+v after=%+v", before.Coverage, after.Coverage)
	}
	if len(after.Snapshot.Queries) != len(before.Snapshot.Queries) || after.Coverage.ObservedQueries != 0 {
		t.Fatal("coordinate aliases changed the original query identities")
	}
	for _, q := range after.Snapshot.Queries {
		if len(q.Attempts) != 2 || q.Attempts[0].ID == q.Attempts[1].ID || q.Attempts[0].PayloadRef != q.Attempts[1].PayloadRef {
			t.Fatalf("same-payload attempt was lost or copied: %+v", q)
		}
		if q.Object.ID == "999" && q.Outcome != tracecatalog.OutcomeEmpty {
			t.Fatalf("known empty query lost its status: %+v", q)
		}
	}
	manifest := exportedManifest250(t, exportCatalogCall250(t, bus, c, "prepared-attempts"))
	want, _ := json.Marshal(after.Snapshot.Queries)
	got, _ := json.Marshal(manifest.Catalog.Queries)
	if string(want) != string(got) || manifest.ExecutionCoverage.Attempts != after.Coverage.Attempts || !manifest.MaterialComplete {
		t.Fatal("publication did not preserve the same complete attempt collection")
	}
}
