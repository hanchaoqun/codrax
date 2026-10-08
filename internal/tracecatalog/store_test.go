package tracecatalog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestAtomicNavigationSnapshotRoundTripAndNoAuthorityRestore(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "one/trace"), "one")
	putFile(t, filepath.Join(root, "two/trace"), "two")
	c := discover(t, root)
	artifacts := c.Snapshot().Artifacts
	for _, a := range artifacts {
		bind(t, c, a)
	}
	q := plan(t, c, artifacts[0].ID, "101", &Window{StartNS: 10_000_000_000, EndNS: 10_050_000_000})
	finish(t, c, q, OutcomeFailure)
	finish(t, c, q, OutcomeSuccess)
	plan(t, c, artifacts[1].ID, "101", nil)
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := c.Save(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, c.Snapshot()) {
		t.Fatalf("roundtrip differs\n got=%+v\nwant=%+v", loaded, c.Snapshot())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"sourceCheck", "validate", "runIdentity", "device", "inode"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("private authority leaked: %q", forbidden)
		}
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatalf("snapshot permissions: %v %v", info, err)
	}
	// Loading is a data-only type. Same ID from independent discovery still
	// cannot merge or resume live authority, even in this same process.
	if err := c.Merge(discover(t, root)); !errors.Is(err, ErrForeignRun) {
		t.Fatal("historical identity restored authority")
	}
}

func TestConcurrentForkPublicationsMergeWithoutRestoringDiskAuthority(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "trace"), "trace")
	c := discover(t, root)
	a := c.Snapshot().Artifacts[0]
	bind(t, c, a)
	q1 := plan(t, c, a.ID, "101", nil)
	q2 := plan(t, c, a.ID, "202", nil)
	one, two := c.Clone(), c.Clone()
	finish(t, one, q1, OutcomeFailure)
	finish(t, two, q2, OutcomeSuccess)
	path := filepath.Join(t.TempDir(), "catalog.json")
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, fork := range []*Catalog{one, two} {
		wg.Add(1)
		go func(fork *Catalog) { defer wg.Done(); errs <- fork.Save(context.Background(), path) }(fork)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	// Saving the untouched parent must not roll back prior live publications.
	if err := c.Save(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	s, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]Outcome{}
	for _, q := range s.Queries {
		states[q.ID] = q.Outcome
	}
	if states[q1.ID] != OutcomeFailure || states[q2.ID] != OutcomeSuccess {
		t.Fatalf("fork publication dropped attempts: %+v", states)
	}
	if q, _ := c.Query(q1.ID); q.Outcome != OutcomeNotExecuted {
		t.Fatal("publication union mutated fork-isolated parent")
	}
	// A forged on-disk row must not be adopted by the next same-run save.
	s.Queries[0].Attempts = nil
	s.Queries[0].Outcome = OutcomeNotExecuted
	body, _ := json.Marshal(s)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range loaded.Queries {
		if q.ID == q1.ID && q.Outcome != OutcomeFailure {
			t.Fatal("disk history overrode current-run publication")
		}
	}
}

func TestSaveCancelOrErrorDoesNotReplacePreviousSnapshotOrSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "trace")
	putFile(t, source, "original source")
	c := discover(t, root)
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := c.Save(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	plan(t, c, c.Snapshot().Artifacts[0].ID, "101", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Save(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel not returned: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("canceled save replaced history")
	}
	if err := c.Save(context.Background(), source); err == nil {
		t.Fatal("catalog overwrote original source")
	}
	if got, _ := os.ReadFile(source); string(got) != "original source" {
		t.Fatal("source changed")
	}
	if err := c.Save(context.Background(), filepath.Join(t.TempDir(), "missing", "catalog.json")); err == nil {
		t.Fatal("missing output directory masked")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("private temp leaked: %+v", entries)
	}
}

func TestLoadRejectsForgedIdentityDuplicateAndNonNavigationEnvelope(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "trace"), "trace")
	c := discover(t, root)
	a := c.Snapshot().Artifacts[0]
	plan(t, c, a.ID, "101", nil)
	for _, tc := range []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{"non-navigation", func(s *Snapshot) { s.NavigationOnly = false }},
		{"query-key", func(s *Snapshot) { s.Queries[0].Object.ID = "202" }},
		{"source-revision", func(s *Snapshot) { s.Queries[0].ArtifactRevision = "different" }},
		{"duplicate-artifact", func(s *Snapshot) { s.Artifacts = append(s.Artifacts, s.Artifacts[0]) }},
		{"duplicate-query", func(s *Snapshot) { s.Queries = append(s.Queries, s.Queries[0]) }},
		{"outside-root", func(s *Snapshot) { s.Root = filepath.Join(root, "unrelated") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := c.Snapshot()
			tc.mutate(&s)
			data, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "forged.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(context.Background(), path); err == nil {
				t.Fatal("malformed history accepted")
			}
		})
	}
}

func TestSnapshotSizeLimitPreservesPriorHistory(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "trace"), "trace")
	c := discover(t, root)
	a := c.Snapshot().Artifacts[0]
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := c.Save(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	for i := 0; i < 300; i++ {
		p := Plan{ArtifactID: a.ID, Object: Object{Kind: "thread", ID: strings.Repeat("x", i+1)}, View: "event_search", Parameters: json.RawMessage(`{"literal":"` + strings.Repeat("x", 32000) + `"}`)}
		if _, err := c.RegisterPlannedQuery(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Save(context.Background(), path); err == nil {
		t.Fatal("oversized snapshot silently published")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("failed bounded save replaced prior history")
	}
}
