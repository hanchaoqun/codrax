package tracecatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func publicationFixture292(t *testing.T) (*Catalog, QueryRecord, string) {
	t.Helper()
	root := t.TempDir()
	putFile(t, filepath.Join(root, "capture"), "original capture")
	c := discover(t, root)
	a := c.Snapshot().Artifacts[0]
	bind(t, c, a)
	q := plan(t, c, a.ID, "101", &Window{StartNS: 10, EndNS: 50})
	if _, err := c.SelectRepresentatives(context.Background(), RepresentativeOptions{}); err != nil {
		t.Fatal(err)
	}
	return c, q, filepath.Join(t.TempDir(), "catalog.json")
}

func assertPublication292(t *testing.T, p Publication) {
	t.Helper()
	b, err := os.ReadFile(p.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotContentSHA(b) != p.IndexSHA256 {
		t.Fatal("index hash differs")
	}
	loaded, err := Load(context.Background(), p.IndexPath)
	if err != nil || !reflect.DeepEqual(loaded, p.Execution.Snapshot) {
		t.Fatalf("index/view differs: %v", err)
	}
	if p.Execution.Selection == nil {
		return
	}
	b, err = os.ReadFile(p.SelectionPath)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotContentSHA(b) != p.SelectionSHA256 {
		t.Fatal("selection hash differs")
	}
	var selection struct {
		Index string `json:"saved_index"`
		SHA   string `json:"saved_index_sha256"`
		RepresentativeSnapshot
	}
	if err := json.Unmarshal(b, &selection); err != nil {
		t.Fatal(err)
	}
	// Normalize indentation of the embedded parameters, not any factual field.
	gotSelection, _ := json.Marshal(selection.RepresentativeSnapshot)
	wantSelection, _ := json.Marshal(p.Execution.Selection)
	if selection.Index != p.IndexPath || selection.SHA != p.IndexSHA256 || !bytes.Equal(gotSelection, wantSelection) {
		t.Fatal("selection and index/view generation differ")
	}
	queries := map[string]QueryRecord{}
	for _, q := range loaded.Queries {
		queries[q.ID] = q
	}
	for _, g := range selection.Clusters {
		for _, m := range g.Members {
			q := queries[m.QueryID]
			if m.Outcome != q.Outcome || m.Attempts != len(q.Attempts) {
				t.Fatal("selection/query attempt states diverged")
			}
			if len(q.Attempts) > 0 && !reflect.DeepEqual(m.LatestAttempt, &q.Attempts[len(q.Attempts)-1]) {
				t.Fatal("latest attempt differs")
			}
		}
	}
}

func TestHMC292PublicationConcurrentForksReuseAndNoAuthority(t *testing.T) {
	c, q, path := publicationFixture292(t)
	left, right := c.Clone(), c.Clone()
	finish(t, left, q, OutcomeFailure)
	finish(t, right, q, OutcomeSuccess)
	var wg sync.WaitGroup
	results := make(chan Publication, 2)
	errs := make(chan error, 2)
	for _, fork := range []*Catalog{left, right} {
		wg.Add(1)
		go func(fork *Catalog) {
			defer wg.Done()
			p, err := fork.SavePublication(context.Background(), path)
			results <- p
			errs <- err
		}(fork)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for p := range results {
		assertPublication292(t, p)
	}
	final, err := c.SavePublication(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	assertPublication292(t, final)
	if final.Execution.Coverage.Attempts != 2 || final.Execution.Snapshot.Queries[0].Outcome != OutcomeSuccess {
		t.Fatal("complete retry history/current outcome lost")
	}
	if row, _ := c.Query(q.ID); row.Outcome != OutcomeNotExecuted || len(row.Attempts) != 0 {
		t.Fatal("delivery union leaked into parent authority")
	}
	indexGeneration, err := filegeneration.FromPath(final.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	selectionGeneration, err := filegeneration.FromPath(final.SelectionPath)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := c.SavePublication(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]filegeneration.Identity{final.IndexPath: indexGeneration, final.SelectionPath: selectionGeneration} {
		got, err := filegeneration.FromPath(name)
		if err != nil || !want.SameVersion(got) {
			t.Fatal("same-content publication rewrote physical generation")
		}
	}
	if repeated.IndexPath != final.IndexPath || repeated.SelectionPath != final.SelectionPath {
		t.Fatal("same bytes not reused")
	}
	// Legacy alias is a separate inode, not a writable hardlink to delivery.
	aliasInfo, _ := os.Stat(path)
	indexInfo, _ := os.Stat(final.IndexPath)
	if os.SameFile(aliasInfo, indexInfo) {
		t.Fatal("legacy alias can mutate immutable index")
	}
	oldBytes, _ := os.ReadFile(final.IndexPath)
	putFile(t, path, "forged legacy history")
	if _, err := c.SavePublication(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(final.IndexPath); !bytes.Equal(got, oldBytes) {
		t.Fatal("legacy content affected frozen index")
	}
	finish(t, right, q, OutcomeFailure)
	changed, err := right.SavePublication(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	assertPublication292(t, changed)
	if changed.IndexPath == final.IndexPath || changed.Execution.Snapshot.Queries[0].Outcome != OutcomeFailure || changed.Execution.Coverage.Attempts != 3 {
		t.Fatal("new failure borrowed older success")
	}
	if got, _ := os.ReadFile(final.IndexPath); !bytes.Equal(got, oldBytes) {
		t.Fatal("later retry rewrote delivered generation")
	}
	// Historical loading still only permits explicit re-admission; every old
	// attempt is history, and neither selection nor success becomes current.
	history, err := Load(context.Background(), final.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := Resume(context.Background(), history.Root, history, []Plan{q.Plan})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resumed.Representatives(); ok {
		t.Fatal("history restored representative authority")
	}
	if got := resumed.Snapshot().Queries[0]; len(got.Attempts) != 0 || got.Outcome != OutcomeNotExecuted {
		t.Fatal("history restored current execution")
	}
}

func TestHMC292PublicationRejectsCanceledAndAlteredDestinations(t *testing.T) {
	for _, mode := range []string{"cancel", "index_bytes", "selection_bytes", "index_symlink", "selection_symlink", "alias_directory"} {
		t.Run(mode, func(t *testing.T) {
			c, _, path := publicationFixture292(t)
			first, err := c.SavePublication(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			ctx := context.Background()
			switch mode {
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "index_bytes":
				putFile(t, first.IndexPath, "tampered index")
			case "selection_bytes":
				putFile(t, first.SelectionPath, "tampered selection")
			case "index_symlink", "selection_symlink":
				victim := first.IndexPath
				if mode == "selection_symlink" {
					victim = first.SelectionPath
				}
				if err := os.Rename(victim, victim+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(victim+".original", victim); err != nil {
					t.Fatal(err)
				}
			case "alias_directory":
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if p, err := c.SavePublication(ctx, path); err == nil || p.IndexPath != "" {
				t.Fatalf("%s published success: %+v %v", mode, p, err)
			} else if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if mode != "alias_directory" {
				if got, _ := os.ReadFile(path); !bytes.Equal(got, before) {
					t.Fatal("failed publication replaced legacy alias")
				}
			}
			entries, _ := os.ReadDir(filepath.Dir(path))
			for _, e := range entries {
				if len(e.Name()) > 0 && e.Name()[0] == '.' {
					t.Fatal("temporary publication leaked")
				}
			}
		})
	}
}
