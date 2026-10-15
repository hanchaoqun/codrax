package tracecatalog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHMC246ManifestHistoryCapacityCloneAndRoster(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "one.trace")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Discover(ctx, root, DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a := c.Snapshot().Artifacts[0]
	_, err = c.RegisterDeclaredQuery(ctx, Plan{ArtifactID: a.ID, Object: Object{Kind: "thread", ID: "1"}, View: "event_search", Parameters: json.RawMessage(`{"view":"event_search","pid":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.trace"), []byte("must not add"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxHistoricalRuns; i++ {
		s := c.Snapshot()
		plans := []Plan{s.Queries[0].Plan}
		next, err := Resume(ctx, s.Root, s, plans)
		if err != nil {
			t.Fatal(err)
		}
		if len(next.Snapshot().History) != i+1 || len(next.Snapshot().Artifacts) != 1 || next.IsDeclaredQuery(next.Snapshot().Queries[0].ID) {
			t.Fatal("history/roster/declared authority changed")
		}
		if err := c.Merge(next); err != ErrForeignRun {
			t.Fatal("cross-run merge accepted")
		}
		cloned := next.Clone()
		detached := cloned.Snapshot()
		detached.History[0].Queries[0].Parameters[0] = 'X'
		detached.History[0].Artifacts[0].Path = "foreign"
		if !reflect.DeepEqual(next.Snapshot(), cloned.Snapshot()) {
			t.Fatal("history aliases mutable snapshot")
		}
		c = next
	}
	s := c.Snapshot()
	if _, err := Resume(ctx, s.Root, s, []Plan{s.Queries[0].Plan}); err == nil {
		t.Fatal("history silently dropped beyond capacity")
	}
	file := filepath.Join(t.TempDir(), "history.json")
	if err := c.Save(ctx, file); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(ctx, file)
	if err != nil || len(loaded.History) != MaxHistoricalRuns {
		t.Fatalf("full history roundtrip: %v", err)
	}
}

func TestHMC246ManifestDuplicateJSONAndWrongRoot(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "one.trace")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Discover(ctx, root, DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := c.Snapshot()
	if _, err := Resume(ctx, t.TempDir(), s, nil); err == nil {
		t.Fatal("different root accepted")
	}
	data, _ := json.Marshal(s)
	bad := append([]byte(`{"root":"/foreign",`), data[1:]...)
	file := filepath.Join(t.TempDir(), "forged.json")
	if err := os.WriteFile(file, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(ctx, file); err == nil {
		t.Fatal("duplicate root accepted")
	}
	s.Artifacts = append(s.Artifacts, s.Artifacts[0])
	if _, err := Resume(ctx, s.Root, s, nil); err == nil {
		t.Fatal("duplicate historical member accepted")
	}
}

func TestHMC246ManifestExecutedParametersAreDetached(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.trace"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Discover(ctx, root, DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	q, err := c.RegisterPlannedQuery(ctx, Plan{ArtifactID: c.Snapshot().Artifacts[0].ID, Object: Object{Kind: "capture", ID: "all"}, View: "event_search", Parameters: json.RawMessage(`{"view":"event_search"}`)})
	if err != nil {
		t.Fatal(err)
	}
	parameters := json.RawMessage(`{"view":"event_search","time_start":1}`)
	_, err = c.Complete(ctx, q.ID, Completion{Outcome: OutcomeFailure, ExecutedParameters: parameters, Error: &Issue{Code: "failed"}})
	if err != nil {
		t.Fatal(err)
	}
	parameters[0] = 'X'
	clone := c.Clone()
	snapshot := clone.Snapshot()
	snapshot.Queries[0].Attempts[0].ExecutedParameters[0] = 'Y'
	actual, _ := c.Query(q.ID)
	if actual.Attempts[0].ExecutedParameters[0] != '{' {
		t.Fatal("caller mutated persisted execution parameters")
	}
	if err := c.Merge(clone); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "catalog.json")
	if err := c.Save(ctx, file); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(ctx, file)
	if err != nil || !json.Valid(loaded.Queries[0].Attempts[0].ExecutedParameters) {
		t.Fatalf("roundtrip: %v", err)
	}
}
