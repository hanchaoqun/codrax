package tool

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func assertStatusPair293(t *testing.T, page publicationPage292) (tracecatalog.Snapshot, tracecatalog.RepresentativeSnapshot) {
	t.Helper()
	if page.Selection == "" || page.SelectionSHA == "" {
		t.Fatal("ordinary status omitted its current same-generation representative snapshot/SHA")
	}
	index, err := tracecatalog.Load(context.Background(), page.Index)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		tracecatalog.RepresentativeSnapshot
		Index string `json:"saved_index"`
		SHA   string `json:"saved_index_sha256"`
	}
	body, err := os.ReadFile(page.Selection)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{page.Index: page.SHA, page.Selection: page.SelectionSHA} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if want != hex.EncodeToString(sum[:]) {
			t.Fatalf("bad publication hash: %s", path)
		}
	}
	if document.Index != page.Index || document.SHA != page.SHA {
		t.Fatal("selection/index generations not bound")
	}
	queries := map[string]tracecatalog.QueryRecord{}
	for _, q := range index.Queries {
		queries[q.ID] = q
	}
	for _, group := range document.Clusters {
		for _, member := range group.Members {
			q, ok := queries[member.QueryID]
			if !ok || q.Outcome != member.Outcome || len(q.Attempts) != member.Attempts {
				t.Fatalf("member not same generation: %+v", member)
			}
			if len(q.Attempts) > 0 {
				got, _ := json.Marshal(member.LatestAttempt)
				want, _ := json.Marshal(q.Attempts[len(q.Attempts)-1])
				if !bytes.Equal(got, want) {
					t.Fatal("latest attempt mismatch")
				}
			}
		}
	}
	return index, document.RepresentativeSnapshot
}

func TestHMC293StatusPublishesCurrentMergedSelection(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	first := publicationStatus292(t, bus, c.ID(), "representatives")
	left, right := bus.ShallowClone(), bus.ShallowClone()
	left.Mutable, right.Mutable = bus.Mutable.ForkForExploreDispatch(), bus.Mutable.ForkForExploreDispatch()
	if out := catalogPublicQuery(t, left, filepath.Join(root, "capture.data"), 101); !out.Success {
		t.Fatal(out.Summary)
	}
	if out := catalogPublicQuery(t, right, filepath.Join(root, "nested/capture.data"), 999); !out.Success {
		t.Fatal(out.Summary)
	}
	before := left.Mutable.TraceCatalogs()[0].Snapshot()
	page := publicationStatus292(t, left, c.ID(), "status")
	index, selection := assertStatusPair293(t, page)
	if len(index.Queries) != 6 || selection.Coverage.Members != 6 {
		t.Fatal("publication lost full roster")
	}
	attempts := 0
	for _, q := range index.Queries {
		attempts += len(q.Attempts)
	}
	if attempts != 2 {
		t.Fatalf("publication lost peer work: %d", attempts)
	}
	if page.Index == first.Index || page.Selection == first.Selection {
		t.Fatal("current status reused pending generation")
	}
	if !reflect.DeepEqual(before, left.Mutable.TraceCatalogs()[0].Snapshot()) {
		t.Fatal("publication installed union into executing fork")
	}
	// Both public navigation shapes must hand out exactly the same generation.
	representative := publicationStatus292(t, left, c.ID(), "representative_status")
	if page != representative {
		t.Fatal("normal and representative status delivered different generations")
	}
}

func TestHMC293StatusKeepsSelectionAndImmutablePairAcrossOrdinaryWork(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	publicationStatus292(t, bus, c.ID(), "representatives")
	first := publicationStatus292(t, bus, c.ID(), "status")
	_, original := assertStatusPair293(t, first)
	oldIndex, _ := os.ReadFile(first.Index)
	oldSelection, _ := os.ReadFile(first.Selection)
	selected := map[string]bool{}
	var unselected string
	for _, g := range original.Clusters {
		for _, m := range g.Members {
			selected[m.QueryID] = m.Selected
			if !m.Selected && filepath.Base(m.Path) == "capture.data" {
				unselected = m.QueryID
			}
		}
	}
	if unselected == "" {
		t.Fatal("no healthy unselected fixture member")
	}
	raw, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": unselected}})
	for i := 0; i < 2; i++ {
		if out, err := (&TraceQuery{}).Execute(bus, raw); err != nil || !out.Success {
			t.Fatalf("unselected retry: %v %s", err, out.Summary)
		}
		if out := catalogPublicQuery(t, bus, filepath.Join(root, "broken.systrace"), 101); out.Success {
			t.Fatal("bad source unexpectedly succeeded")
		}
	}
	if out := catalogPublicQuery(t, bus, filepath.Join(root, "capture.data"), 202); !out.Success {
		t.Fatal(out.Summary)
	}
	page := publicationStatus292(t, bus, c.ID(), "status")
	index, current := assertStatusPair293(t, page)
	if current.SelectionID != original.SelectionID || len(current.NewQueryIDs) != 1 || len(index.Queries) != 7 {
		t.Fatal("ordinary work silently reselected or vanished")
	}
	for _, g := range current.Clusters {
		for _, m := range g.Members {
			if selected[m.QueryID] != m.Selected {
				t.Fatal("selection membership changed")
			}
		}
	}
	attempts := 0
	for _, q := range index.Queries {
		attempts += len(q.Attempts)
	}
	if attempts != 5 {
		t.Fatalf("failed/ordinary attempts lost: %d", attempts)
	}
	if again, _ := os.ReadFile(first.Index); !bytes.Equal(oldIndex, again) {
		t.Fatal("old index rewritten")
	}
	if again, _ := os.ReadFile(first.Selection); !bytes.Equal(oldSelection, again) {
		t.Fatal("old selection rewritten")
	}
	if reuse := publicationStatus292(t, bus, c.ID(), "status"); reuse != page {
		t.Fatal("same content not reused")
	}
	// End-of-page still delivers the full pair, without adding a query call.
	raw, _ = json.Marshal(map[string]any{"action": "status", "catalog_id": c.ID(), "offset": len(index.Artifacts) + len(index.Queries)})
	out, err := (&TraceCatalog{}).Execute(bus, raw)
	if err != nil || !out.Success || len(out.Summary) > 64<<10 {
		t.Fatalf("pagination: %v %s", err, out.Summary)
	}
	var end publicationPage292
	if err := json.Unmarshal([]byte(out.Summary), &end); err != nil {
		t.Fatal(err)
	}
	if end != page {
		t.Fatal("page offset changed publication generation")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(out.Summary), &fields)
	if len(fields["query_calls"]) != 0 {
		t.Fatal("end-of-page invented execution calls")
	}
}

func TestHMC293StatusDoesNotRestoreOrInventSelection(t *testing.T) {
	bus, _ := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	plain := publicationStatus292(t, bus, c.ID(), "status")
	if plain.Selection != "" || plain.SelectionSHA != "" {
		t.Fatal("status selected without an explicit selection")
	}
	publicationStatus292(t, bus, c.ID(), "representatives")
	selected := publicationStatus292(t, bus, c.ID(), "status")
	assertStatusPair293(t, selected)
	other := bus.ShallowClone()
	other.Mutable = types.NewMutableState("fresh run")
	raw, _ := json.Marshal(map[string]any{"action": "status", "catalog_id": c.ID()})
	if out, _ := (&TraceCatalog{}).Execute(other, raw); out.Success {
		t.Fatal("old ID granted current run access")
	}
	raw, _ = json.Marshal(map[string]any{"action": "resume", "root": bus.RepoRoot, "snapshot_path": selected.Index})
	if out, _ := (&TraceCatalog{}).Execute(other, raw); out.Success {
		t.Fatal("historical snapshot bypassed current path authorization")
	}
	other.UserPinnedFiles = []string{selected.Index}
	out, err := (&TraceCatalog{}).Execute(other, raw)
	if err != nil || !out.Success {
		t.Fatalf("ordinary historical resume: %v %s", err, out.Summary)
	}
	var resumed publicationPage292
	_ = json.Unmarshal([]byte(out.Summary), &resumed)
	if resumed.Selection != "" || resumed.SelectionSHA != "" {
		t.Fatal("historical JSON restored selection")
	}
	for _, q := range other.Mutable.TraceCatalogs()[0].Snapshot().Queries {
		if len(q.Attempts) != 0 || q.Outcome != tracecatalog.OutcomeNotExecuted {
			t.Fatal("historical execution restored")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bus.Ctx = ctx
	raw, _ = json.Marshal(map[string]any{"action": "status", "catalog_id": c.ID()})
	if out, _ := (&TraceCatalog{}).Execute(bus, raw); out.Success {
		t.Fatal("canceled status returned successful pair")
	}
}
