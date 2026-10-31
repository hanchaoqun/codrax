package tracecatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func TestHMC328RepresentativePublicationIncludesUnsavedLiveBranches(t *testing.T) {
	c, q, path := publicationFixture292(t)
	left, right := c.Clone(), c.Clone()
	finish(t, left, q, OutcomeFailure)
	finish(t, right, q, OutcomeSuccess)
	before := c.CurrentExecution()
	// Neither branch saves: failed intermediate persistence must not erase a
	// real attempt from the terminal union.
	p, err := c.SaveRepresentativePublication(context.Background(), path, func(*Catalog) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	assertPublication292(t, p)
	if p.Execution.Coverage.Attempts != 2 || p.Execution.Coverage.AttemptOutcomes[OutcomeFailure] != 1 || p.Execution.Coverage.AttemptOutcomes[OutcomeSuccess] != 1 {
		t.Fatalf("live attempts lost: %+v", p.Execution.Coverage)
	}
	if !reflect.DeepEqual(before, c.CurrentExecution()) {
		t.Fatal("terminal union installed peer execution state")
	}
	versions := map[string]filegeneration.Identity{}
	for _, name := range []string{path, p.IndexPath, p.SelectionPath} {
		version, err := filegeneration.FromPath(name)
		if err != nil {
			t.Fatal(err)
		}
		versions[name] = version
	}
	again, err := right.SaveRepresentativePublication(context.Background(), path, func(*Catalog) error { return nil })
	if err != nil || again.IndexPath != p.IndexPath || again.SelectionPath != p.SelectionPath {
		t.Fatalf("same live union not reused: %v", err)
	}
	for name, want := range versions {
		got, err := filegeneration.FromPath(name)
		if err != nil || !want.SameVersion(got) {
			t.Fatalf("same terminal publication rewrote %s", name)
		}
	}
	oldIndex, _ := os.ReadFile(p.IndexPath)
	// The alias is neither an input nor an authority source.
	putFile(t, path, "forged latest index")
	again, err = c.SaveRepresentativePublication(context.Background(), path, func(*Catalog) error { return nil })
	if err != nil || again.IndexPath != p.IndexPath {
		t.Fatalf("disk alias influenced terminal data: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "forged latest index" {
		t.Fatal("same terminal view rewrote alias instead of only verifying immutable objects")
	}
	finish(t, left, q, OutcomeEmpty)
	next, err := c.SaveRepresentativePublication(context.Background(), path, func(*Catalog) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	assertPublication292(t, next)
	if next.IndexPath == p.IndexPath || next.Execution.Coverage.Attempts != 3 || next.Execution.Selection.SelectionID != p.Execution.Selection.SelectionID || next.Execution.Snapshot.Queries[0].Outcome != OutcomeEmpty {
		t.Fatal("legal retry lost attempt or reselected")
	}
	if got, _ := os.ReadFile(p.IndexPath); !bytes.Equal(got, oldIndex) {
		t.Fatal("later retry overwrote old immutable index")
	}
}

func TestHMC328RepresentativePublicationRejectsInvalidTerminalGeneration(t *testing.T) {
	for _, mode := range []string{"cancelled", "unauthorized", "source_changed", "save_failure", "tampered_index", "concurrent_attempt", "branch_overflow"} {
		t.Run(mode, func(t *testing.T) {
			c, q, path := publicationFixture292(t)
			finish(t, c, q, OutcomeSuccess)
			p, err := c.SaveRepresentativePublication(context.Background(), path, func(*Catalog) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			authorize := func(*Catalog) error { return nil }
			switch mode {
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "unauthorized":
				authorize = func(*Catalog) error { return errors.New("revoked current source authority") }
			case "source_changed":
				putFile(t, c.Snapshot().Artifacts[0].Path, "replaced input")
			case "save_failure":
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "tampered_index":
				putFile(t, p.IndexPath, "different bytes")
			case "concurrent_attempt":
				fork := c.Clone()
				authorize = func(*Catalog) error { finish(t, fork, q, OutcomeFailure); return nil }
			case "branch_overflow":
				for i := 0; i < MaxDeliveryBranches; i++ {
					fork := c.Clone()
					finish(t, fork, q, OutcomeEmpty)
				}
			}
			failed, err := c.SaveRepresentativePublication(ctx, path, authorize)
			if err == nil || failed.IndexPath != "" || failed.SelectionPath != "" || failed.IndexSHA256 != "" || failed.SelectionSHA256 != "" {
				t.Fatalf("invalid generation returned successful delivery: %+v %v", failed, err)
			}
		})
	}
}

func TestHMC328RepresentativePublicationDoesNotRestoreSelection(t *testing.T) {
	c, q, path := publicationFixture292(t)
	finish(t, c, q, OutcomeSuccess)
	p, err := c.SaveRepresentativePublication(context.Background(), path, func(*Catalog) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	history, err := Load(context.Background(), p.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := Resume(context.Background(), history.Root, history, []Plan{q.Plan})
	if err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(t.TempDir(), "current.json")
	got, err := resumed.SaveRepresentativePublication(context.Background(), newPath, func(*Catalog) error { return nil })
	if err == nil || got.IndexPath != "" {
		t.Fatal("historical JSON granted current representative delivery")
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatal("no-selection call wrote navigation")
	}
	if q := resumed.Snapshot().Queries[0]; len(q.Attempts) != 0 || q.Outcome != OutcomeNotExecuted {
		t.Fatal("history restored current attempts")
	}
}

func TestHMC328RepresentativePublicationRevalidatesAfterPublishing(t *testing.T) {
	for _, mode := range []string{"source_changed", "authorization_revoked"} {
		t.Run(mode, func(t *testing.T) {
			c, q, path := publicationFixture292(t)
			finish(t, c, q, OutcomeSuccess)
			calls := 0
			p, err := c.SaveRepresentativePublication(context.Background(), path, func(*Catalog) error {
				calls++
				if calls == 2 {
					// The original alias exists only after the publication step;
					// exercise the gap between publication and the final receipt.
					if _, err := os.Stat(path); err != nil {
						t.Fatal("post-publication boundary was not reached")
					}
					if mode == "source_changed" {
						putFile(t, c.Snapshot().Artifacts[0].Path, "source changed during delivery")
					} else {
						return errors.New("authorization revoked during delivery")
					}
				}
				return nil
			})
			if calls != 2 || err == nil || p.IndexPath != "" || p.SelectionPath != "" {
				t.Fatalf("post-publication change certified: calls=%d receipt=%+v err=%v", calls, p, err)
			}
			files, err := filepath.Glob(filepath.Join(filepath.Dir(path), "representatives-*.json"))
			if err != nil || len(files) != 1 {
				t.Fatal("failure erased the immutable historical object")
			}
		})
	}
}

func TestHMC328RepresentativeReceiptShapeIsNavigationOnly(t *testing.T) {
	// A new field requires an explicit owner decision: this receipt must not
	// silently grow native, read, causal or whole-analysis completion authority.
	want := map[string]string{
		"Kind": "kind", "Status": "status", "CatalogID": "catalog_id", "SelectionID": "selection_id,omitempty",
		"IndexPath": "saved_index,omitempty", "IndexSHA256": "saved_index_sha256,omitempty",
		"SelectionPath": "selection_snapshot,omitempty", "SelectionSHA256": "selection_snapshot_sha256,omitempty",
		"ExecutionCoverage": "execution_coverage,omitempty", "RepresentativeCoverage": "representative_coverage,omitempty", "Reason": "reason,omitempty",
	}
	typ := reflect.TypeOf(RepresentativeDelivery{})
	if typ.NumField() != len(want) {
		t.Fatal("classify the new terminal receipt field before publication")
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag, ok := want[field.Name]
		if !ok || field.Tag.Get("json") != tag {
			t.Fatalf("unclassified terminal receipt field %s", field.Name)
		}
	}
	body, err := json.Marshal(RepresentativeDelivery{Kind: "trace_representative_delivery/v1", Status: "failed", CatalogID: "catalog", Reason: "failure"})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 4 || fields["saved_index"] != nil || fields["selection_snapshot"] != nil {
		t.Fatal("failed receipt published a successful object shape")
	}
}
