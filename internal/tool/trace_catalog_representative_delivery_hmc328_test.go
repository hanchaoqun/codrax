package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func representativeDeliveryPairHMC328(t *testing.T, rows []tracecatalog.RepresentativeDelivery) (tracecatalog.Snapshot, tracecatalog.RepresentativeSnapshot) {
	t.Helper()
	if len(rows) != 1 || rows[0].Status != "published" || rows[0].ExecutionCoverage == nil || rows[0].RepresentativeCoverage == nil {
		t.Fatalf("missing typed terminal receipt: %+v", rows)
	}
	r := rows[0]
	index, selection := assertStatusPair293(t, publicationPage292{Index: r.IndexPath, SHA: r.IndexSHA256, Selection: r.SelectionPath, SelectionSHA: r.SelectionSHA256})
	if r.CatalogID != index.ID || r.SelectionID != selection.SelectionID {
		t.Fatal("typed receipt is another generation")
	}
	return index, selection
}

func TestHMC328RepresentativeTerminalRetainsFailuresRetriesAndUnselected(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	first := publicationStatus292(t, bus, c.ID(), "representatives")
	_, selected := assertStatusPair293(t, first)
	var unselected string
	for _, g := range selected.Clusters {
		for _, m := range g.Members {
			if !m.Selected && filepath.Base(m.Path) == "capture.data" {
				unselected = m.QueryID
			}
		}
	}
	if unselected == "" {
		t.Fatal("fixture lacks healthy unselected member")
	}
	call, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": unselected}})
	for i := 0; i < 2; i++ {
		if out, err := (&TraceQuery{}).Execute(bus, call); err != nil || !out.Success {
			t.Fatalf("unselected legal retry: %v %s", err, out.Summary)
		}
	}
	if out := catalogPublicQuery(t, bus, filepath.Join(root, "broken.systrace"), 101); out.Success {
		t.Fatal("bad source unexpectedly succeeded")
	}
	if out := catalogPublicQuery(t, bus, filepath.Join(root, "capture.data"), 202); !out.Success {
		t.Fatal(out.Summary)
	}
	rows := MaterializeTraceCatalogRepresentativeDeliveries(bus)
	index, current := representativeDeliveryPairHMC328(t, rows)
	if rows[0].ExecutionCoverage.Attempts != 4 || rows[0].ExecutionCoverage.AttemptOutcomes[tracecatalog.OutcomeFailure] != 1 || current.SelectionID != selected.SelectionID || len(current.NewQueryIDs) != 1 || len(index.Queries) != 7 {
		t.Fatalf("actual work collapsed: %+v %+v", rows[0], current)
	}
	found := false
	for _, g := range current.Clusters {
		for _, m := range g.Members {
			if m.QueryID == unselected {
				found = true
				if m.Selected || m.Attempts != 2 {
					t.Fatal("unselected execution promoted membership or lost retry")
				}
			}
		}
	}
	if !found {
		t.Fatal("unselected query disappeared")
	}
	if len(bus.Mutable.TraceCatalogs()) != 1 {
		t.Fatal("terminal navigation created a new executing catalog")
	}
}

func TestHMC328RepresentativeTerminalTypedFailureDoesNotReturnPaths(t *testing.T) {
	for _, mode := range []string{"cancelled", "revoked", "source_changed", "save_failure", "empty_current_state", "historical_resume"} {
		t.Run(mode, func(t *testing.T) {
			bus, root := catalogPublicFixture(t)
			c := catalogPublicDiscover(t, bus)
			selected := publicationStatus292(t, bus, c.ID(), "representatives")
			if out := catalogPublicQuery(t, bus, filepath.Join(root, "capture.data"), 101); !out.Success {
				t.Fatal(out.Summary)
			}
			switch mode {
			case "cancelled":
				cancelled, cancel := context.WithCancel(context.Background())
				cancel()
				bus.Ctx = cancelled
			case "revoked":
				bus.RepoRoot = t.TempDir()
				bus.UserPinnedFiles = nil
			case "source_changed":
				if err := os.WriteFile(filepath.Join(root, "capture.data"), []byte("replacement"), 0600); err != nil {
					t.Fatal(err)
				}
			case "save_failure":
				broken := filepath.Join(t.TempDir(), "not-directory")
				if err := os.WriteFile(broken, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
				bus.WorkDir = broken
			case "empty_current_state":
				bus.Mutable = types.NewMutableState("new run")
			case "historical_resume":
				bus.Mutable = types.NewMutableState("resumed run")
				bus.UserPinnedFiles = []string{selected.Index}
				raw, _ := json.Marshal(map[string]any{"action": "resume", "root": root, "snapshot_path": selected.Index})
				out, err := (&TraceCatalog{}).Execute(bus, raw)
				if err != nil || !out.Success {
					t.Fatalf("resume setup: %v %s", err, out.Summary)
				}
			}
			rows := MaterializeTraceCatalogRepresentativeDeliveries(bus)
			if mode == "empty_current_state" || mode == "historical_resume" {
				if len(rows) != 0 {
					t.Fatal("historical identity restored terminal selection")
				}
				return
			}
			if len(rows) != 1 || rows[0].Status == "published" || rows[0].Reason == "" || rows[0].IndexPath != "" || rows[0].SelectionPath != "" || rows[0].IndexSHA256 != "" || rows[0].SelectionSHA256 != "" || rows[0].ExecutionCoverage != nil || rows[0].RepresentativeCoverage != nil {
				t.Fatalf("failure minted successful receipt: %+v", rows)
			}
			if mode == "cancelled" && rows[0].Status != "cancelled" {
				t.Fatal("cancellation was renamed")
			}
		})
	}
}
