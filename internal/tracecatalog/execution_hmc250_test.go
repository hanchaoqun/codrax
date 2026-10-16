package tracecatalog

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHMC250ExecutionOriginsCloneMergeAndHistory(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "a"), "trace a")
	putFile(t, filepath.Join(root, "nested", "a"), "trace b")
	c := discover(t, root)
	a := c.Snapshot().Artifacts[0]
	bind(t, c, a)
	observed := plan(t, c, a.ID, "101", nil)
	finish(t, c, observed, OutcomeFailure)
	finish(t, c, observed, OutcomeSuccess)
	declared, err := c.RegisterDeclaredQuery(context.Background(), Plan{ArtifactID: a.ID, Object: Object{Kind: "thread", ID: "202"}, View: "resource_stack"})
	if err != nil {
		t.Fatal(err)
	}
	v := c.CurrentExecution()
	if v.Coverage.DeclaredPlans != 1 || v.Coverage.ObservedQueries != 1 || v.Coverage.Attempts != 2 || v.Coverage.PlannedOutcomes[OutcomeNotExecuted] != 1 || v.Coverage.ObservedOutcomes[OutcomeSuccess] != 1 {
		t.Fatalf("wrong census: %+v", v.Coverage)
	}
	if v.QueryOrigins[observed.ID] != QueryOriginObserved || v.QueryOrigins[declared.ID] != QueryOriginDeclared {
		t.Fatal("registration source lost")
	}
	before := c.Snapshot()
	if got := c.Clone().CurrentExecution(); !reflect.DeepEqual(got, v) {
		t.Fatal("live clone lost current registration")
	}
	v.QueryOrigins[observed.ID] = QueryOriginDeclared
	v.Coverage.PlannedOutcomes[OutcomeSuccess] = 100
	v.Snapshot.Queries[0].Outcome = OutcomeEmpty
	if !reflect.DeepEqual(c.Snapshot(), before) || c.CurrentExecution().QueryOrigins[observed.ID] != QueryOriginObserved {
		t.Fatal("display mutated current state")
	}
	fork := c.Clone()
	if _, err := fork.RegisterDeclaredQuery(context.Background(), observed.Plan); err != nil {
		t.Fatal(err)
	}
	if err := c.Merge(fork); err != nil {
		t.Fatal(err)
	}
	if got := c.CurrentExecution().Coverage; got.DeclaredPlans != 2 || got.ObservedQueries != 0 || got.Attempts != 2 {
		t.Fatalf("live merge: %+v", got)
	}
	encoded, err := json.Marshal(c.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	old, err := DecodeSnapshot(encoded)
	if err != nil {
		t.Fatal(err)
	}
	plans := make([]Plan, len(old.Queries))
	for i, q := range old.Queries {
		plans[i] = q.Plan
	}
	resumed, err := Resume(context.Background(), old.Root, old, plans)
	if err != nil {
		t.Fatal(err)
	}
	r := resumed.CurrentExecution()
	if r.Coverage.ResumedPlans != 2 || r.Coverage.DeclaredPlans != 0 || r.Coverage.Attempts != 0 || r.Coverage.PlannedOutcomes[OutcomeNotExecuted] != 2 {
		t.Fatalf("history restored current success: %+v", r.Coverage)
	}
	for _, q := range r.Snapshot.Queries {
		if r.QueryOrigins[q.ID] != QueryOriginResumed || resumed.IsDeclaredQuery(q.ID) {
			t.Fatal("resume restored declaration/window permission")
		}
	}
	if err := c.Merge(resumed); err == nil {
		t.Fatal("foreign run origins merged")
	}
}
