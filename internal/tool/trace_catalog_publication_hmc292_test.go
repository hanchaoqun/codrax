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

type publicationPage292 struct {
	Index        string `json:"saved_index"`
	SHA          string `json:"saved_index_sha256"`
	Legacy       string `json:"latest_index_alias"`
	Selection    string `json:"selection_snapshot"`
	SelectionSHA string `json:"selection_snapshot_sha256"`
}

func publicationStatus292(t *testing.T, bus *types.BusContext, id, action string) publicationPage292 {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"action": action, "catalog_id": id})
	out, err := (&TraceCatalog{}).Execute(bus, raw)
	if err != nil || !out.Success {
		t.Fatalf("%s: %v %s", action, err, out.Summary)
	}
	if len(out.Observations) != 0 || out.ReadCoverage != nil || out.TraceQuerySourceRead.Path() != "" {
		t.Fatal("publication minted evidence")
	}
	var page publicationPage292
	if err := json.Unmarshal([]byte(out.Summary), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func TestHMC292PublicRepresentativePublicationSameMergedGeneration(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	publicationStatus292(t, bus, c.ID(), "representatives")
	left, right := bus.ShallowClone(), bus.ShallowClone()
	left.Mutable, right.Mutable = bus.Mutable.ForkForExploreDispatch(), bus.Mutable.ForkForExploreDispatch()
	if out := catalogPublicQuery(t, left, filepath.Join(root, "capture.data"), 101); !out.Success {
		t.Fatal(out.Summary)
	}
	if out := catalogPublicQuery(t, right, filepath.Join(root, "nested/capture.data"), 999); !out.Success {
		t.Fatal(out.Summary)
	}
	localBefore := left.Mutable.TraceCatalogs()[0].Snapshot()
	page := publicationStatus292(t, left, c.ID(), "representative_status")
	index, err := tracecatalog.Load(context.Background(), page.Index)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(page.Selection)
	if err != nil {
		t.Fatal(err)
	}
	var selection struct {
		tracecatalog.RepresentativeSnapshot
		Index string `json:"saved_index"`
		SHA   string `json:"saved_index_sha256"`
	}
	if err := json.Unmarshal(body, &selection); err != nil {
		t.Fatal(err)
	}
	queries := map[string]tracecatalog.QueryRecord{}
	for _, q := range index.Queries {
		queries[q.ID] = q
	}
	for _, g := range selection.Clusters {
		for _, m := range g.Members {
			q, ok := queries[m.QueryID]
			if !ok || m.Attempts != len(q.Attempts) || m.Outcome != q.Outcome {
				t.Fatalf("same-call selection/index differ for %s: selection attempts=%d outcome=%s; index attempts=%d outcome=%s", m.QueryID, m.Attempts, m.Outcome, len(q.Attempts), q.Outcome)
			}
			if len(q.Attempts) > 0 {
				// Embedded RawMessage indentation differs with document nesting;
				// compare every field after JSON whitespace normalization only.
				got, _ := json.Marshal(m.LatestAttempt)
				want, _ := json.Marshal(q.Attempts[len(q.Attempts)-1])
				if !bytes.Equal(got, want) {
					t.Fatal("same-call latest attempt differs")
				}
			}
		}
	}
	indexBytes, err := os.ReadFile(page.Index)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(indexBytes)
	if page.SHA != hex.EncodeToString(hash[:]) || selection.SHA != page.SHA || selection.Index != page.Index {
		t.Fatal("selection lacks exact index content binding")
	}
	if !reflect.DeepEqual(localBefore, left.Mutable.TraceCatalogs()[0].Snapshot()) {
		t.Fatal("delivery union mutated executing fork")
	}
	other := bus.ShallowClone()
	other.Mutable = types.NewMutableState("fresh")
	raw, _ := json.Marshal(map[string]any{"action": "representative_status", "catalog_id": c.ID()})
	if out, _ := (&TraceCatalog{}).Execute(other, raw); out.Success {
		t.Fatal("published JSON restored selection authority")
	}
}

func TestHMC292PublicDeliveredIndexSurvivesRetryAndUnselectedExecution(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	first := publicationStatus292(t, bus, c.ID(), "representatives")
	before, err := os.ReadFile(first.Index)
	if err != nil {
		t.Fatal(err)
	}
	selectionBefore, err := os.ReadFile(first.Selection)
	if err != nil {
		t.Fatal(err)
	}
	var s tracecatalog.RepresentativeSnapshot
	if err := json.Unmarshal(selectionBefore, &s); err != nil {
		t.Fatal(err)
	}
	var unselected string
	for _, g := range s.Clusters {
		for _, m := range g.Members {
			if !m.Selected && filepath.Base(m.Path) == "capture.data" {
				unselected = m.QueryID
			}
		}
	}
	if unselected == "" {
		t.Fatal("fixture lacks unselected healthy query")
	}
	call, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": unselected}})
	for i := 0; i < 2; i++ {
		out, err := (&TraceQuery{}).Execute(bus, call)
		if err != nil || !out.Success {
			t.Fatalf("legal unselected retry: %v %s", err, out.Summary)
		}
	}
	// Ordinary exploration is independent of selection and can add records.
	if out := catalogPublicQuery(t, bus, filepath.Join(root, "capture.data"), 202); !out.Success {
		t.Fatal(out.Summary)
	}
	after, err := os.ReadFile(first.Index)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("delivered saved_index was rewritten by a later legal query")
	}
	second := publicationStatus292(t, bus, c.ID(), "representative_status")
	if second.Index == first.Index || second.SHA == first.SHA {
		t.Fatal("changed execution reused an older content generation")
	}
	if first.Legacy == "" || first.Legacy != second.Legacy || first.Legacy == first.Index {
		t.Fatal("legacy alias is not separately identified")
	}
	latest, err := tracecatalog.Load(context.Background(), second.Legacy)
	if err != nil {
		t.Fatal(err)
	}
	current, err := tracecatalog.Load(context.Background(), second.Index)
	if err != nil || !reflect.DeepEqual(latest, current) {
		t.Fatal("legacy resume alias not compatible")
	}
	reused := publicationStatus292(t, bus, c.ID(), "representative_status")
	if reused.Index != second.Index || reused.Selection != second.Selection {
		t.Fatal("unchanged generation not reused")
	}
	if again, _ := os.ReadFile(first.Selection); !bytes.Equal(again, selectionBefore) {
		t.Fatal("old selection rewritten")
	}
}

func TestHMC292PublicPeerOnlyNavigationCallsExecuteNormally(t *testing.T) {
	for _, action := range []string{"status", "representative_status"} {
		t.Run(action, func(t *testing.T) {
			bus, root := catalogPublicFixture(t)
			c := catalogPublicDiscover(t, bus)
			left, right := bus.ShallowClone(), bus.ShallowClone()
			left.Mutable, right.Mutable = bus.Mutable.ForkForExploreDispatch(), bus.Mutable.ForkForExploreDispatch()
			path := filepath.Join(root, "capture.data")
			canonicalPath, err := filepath.EvalSymlinks(path)
			if err != nil {
				t.Fatal(err)
			}
			if out := catalogPublicQuery(t, right, path, 202); !out.Success {
				t.Fatal(out.Summary)
			}
			peer := right.Mutable.TraceCatalogs()[0]
			var id string
			for _, q := range peer.Snapshot().Queries {
				if q.Object.ID == "202" {
					id = q.ID
				}
			}
			if id == "" {
				t.Fatal("missing peer-only query")
			}
			// In this branch select all three specifications, so the new query is
			// reachable in both the ordinary and representative navigation pages.
			raw, _ := json.Marshal(map[string]any{"action": "representatives", "catalog_id": c.ID(), "representatives": map[string]any{"per_cluster": 5, "max_clusters": 5}})
			if out, err := (&TraceCatalog{}).Execute(right, raw); err != nil || !out.Success {
				t.Fatalf("peer selection: %v %s", err, out.Summary)
			}
			local := left.Mutable.TraceCatalogs()[0]
			if _, ok := local.Query(id); ok {
				t.Fatal("fixture did not isolate peer")
			}
			raw, _ = json.Marshal(map[string]any{"action": action, "catalog_id": c.ID()})
			out, err := (&TraceCatalog{}).Execute(left, raw)
			if err != nil || !out.Success {
				t.Fatalf("page: %v %s", err, out.Summary)
			}
			var page struct {
				Calls map[string]json.RawMessage `json:"query_calls"`
			}
			if err := json.Unmarshal([]byte(out.Summary), &page); err != nil {
				t.Fatal(err)
			}
			call := page.Calls[id]
			if len(call) == 0 {
				t.Fatal("peer-only query missing returned call")
			}
			if _, ok := local.Query(id); ok {
				t.Fatal("publication installed peer plan authority")
			}
			// Revoking directory scope prevents fresh peer navigation. Ordinary
			// explicit file calls are a separate existing path, not root-scoped
			// capabilities and not new permissions introduced by this page.
			revoked := left.ShallowClone()
			revoked.RepoRoot = t.TempDir()
			if result, _ := (&TraceCatalog{}).Execute(revoked, raw); result.Success {
				t.Fatal("peer navigation bypassed catalog directory scope")
			}
			result, err := (&TraceQuery{}).Execute(left, call)
			if err != nil || !result.Success {
				t.Fatalf("returned peer call is not executable: %v %s; call=%s", err, result.Summary, call)
			}
			var p traceQueryParams
			if err := json.Unmarshal(call, &p); err != nil || p.Source != "path" || p.Path != canonicalPath || p.PID.Int() != 202 || p.View != "resource_stack" || !p.TimeStart.Set() || p.TimeStart.Seconds() != 10 || !p.TimeEnd.Set() || p.TimeEnd.Seconds() != 10.05 {
				t.Fatalf("peer call changed exact source/spec: %s", call)
			}
			q, ok := local.Query(id)
			if !ok || len(q.Attempts) != 1 || q.Outcome != tracecatalog.OutcomeSuccess {
				t.Fatal("normal execution did not register actual attempt")
			}
			// A formerly successful peer cannot supply cached source authority
			// after the actual input disappears.
			if err := os.Rename(path, path+".unavailable"); err != nil {
				t.Fatal(err)
			}
			if result, _ := (&TraceQuery{}).Execute(left, call); result.Success {
				t.Fatal("unavailable source borrowed old peer success")
			}
		})
	}
}
