package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func immutableCatalogFixture251(t *testing.T) (*types.BusContext, *tracecatalog.Catalog) {
	t.Helper()
	bus, root := catalogPublicFixture(t)
	for _, path := range []string{"broken.systrace", "nested/capture.data"} {
		if err := os.Remove(filepath.Join(root, path)); err != nil {
			t.Fatal(err)
		}
	}
	out, err := (&TraceCatalog{}).Execute(bus, []byte(`{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05}]}`))
	if err != nil || !out.Success {
		t.Fatalf("discover: %v %s", err, out.Summary)
	}
	c := bus.Mutable.TraceCatalogs()[0]
	if len(c.Snapshot().Queries) != 1 {
		t.Fatal("expected one exact plan")
	}
	return bus, c
}

func TestHMC251ImmutableNativeResultRetainsStrictExportIdentity(t *testing.T) {
	for _, mutation := range []string{"same_bytes_replacement", "same_size_mtime_restore", "changed_source", "json_history"} {
		t.Run(mutation, func(t *testing.T) {
			bus, c := immutableCatalogFixture251(t)
			q := c.Snapshot().Queries[0]
			out, err := (&TraceQuery{}).Execute(bus, refPublic246(c, q))
			if err != nil || !out.Success || out.ReusedFromRunMemo {
				t.Fatalf("native query: %v %s", err, out.Summary)
			}
			a := c.Snapshot().Queries[0].Attempts[0]
			switch mutation {
			case "same_bytes_replacement", "same_size_mtime_restore":
				data, err := os.ReadFile(a.PayloadRef)
				if err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(a.PayloadRef)
				if err != nil {
					t.Fatal(err)
				}
				if mutation == "same_bytes_replacement" {
					replacement := filepath.Join(bus.WorkDir, "external-replacement")
					if err := os.WriteFile(replacement, data, info.Mode()); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(replacement, a.PayloadRef); err != nil {
						t.Fatal(err)
					}
				} else {
					data[0] ^= 1
					if err := os.WriteFile(a.PayloadRef, data, info.Mode()); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Chtimes(a.PayloadRef, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			case "changed_source":
				source := c.Snapshot().Artifacts[0].Path
				data, err := os.ReadFile(source)
				if err != nil {
					t.Fatal(err)
				}
				// Even identical source bytes do not erase source generation checks.
				if err := os.WriteFile(source, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "json_history":
				bus, c, _ = resumePublic246(t, bus, c)
				if c.CurrentExecution().Coverage.Attempts != 0 {
					t.Fatal("JSON restored current attempts")
				}
			}
			m := exportedManifest250(t, exportCatalogCall250(t, bus, c, "must-not-sign"))
			if m.MaterialComplete {
				t.Fatalf("%s gained complete material", mutation)
			}
			if mutation == "same_bytes_replacement" || mutation == "same_size_mtime_restore" {
				stale := false
				for _, ref := range m.References {
					stale = stale || ref.Status == "stale_reference"
				}
				if !stale {
					t.Fatal("strict result generation no longer enforced")
				}
			}
			if mutation == "json_history" && len(m.References) != 0 {
				t.Fatal("historical JSON borrowed same-byte result references")
			}
		})
	}
}

func TestHMC251ImmutableNativeBytesDoNotBindAnotherSource(t *testing.T) {
	bus, c := exportCatalogFixture250(t)
	var left, right tracecatalog.QueryRecord
	for _, q := range c.Snapshot().Queries {
		if q.Object.ID != "101" {
			continue
		}
		if left.ID == "" {
			left = q
		} else if q.ArtifactID != left.ArtifactID {
			right = q
			break
		}
	}
	if left.ID == "" || right.ID == "" {
		t.Fatal("missing independently admitted sources")
	}
	// Valid source admission and identical file bytes cannot turn a public copied
	// Completion into the private producer witness for another source/attempt.
	if _, err := c.Complete(context.Background(), right.ID, left.Attempts[0].Completion); err == nil {
		t.Fatal("another source's revision was accepted")
	}
	forged := left.Attempts[0].Completion
	forged.SourceRevision = right.Attempts[0].SourceRevision
	if _, err := c.Complete(context.Background(), right.ID, forged); err != nil {
		t.Fatal(err)
	}
	m := exportedManifest250(t, exportCatalogCall250(t, bus, c, "no-cross-source-witness"))
	if m.MaterialComplete {
		t.Fatal("same material signed another source's forged completion")
	}
	missingWitness := false
	for _, ref := range m.References {
		if ref.QueryID == right.ID && ref.Reason == "no current producer reference binding" {
			missingWitness = true
		}
	}
	if !missingWitness {
		t.Fatal("source/attempt authority rejection disappeared")
	}
}

func TestHMC251ImmutableNativeResultTwoForksExport(t *testing.T) {
	// Force the real query's complete textual result into RawRef as well as its
	// JSON PayloadRef. Both must keep their physical generation across attempts.
	old := MaxInlineBytes
	MaxInlineBytes = 1
	t.Cleanup(func() { MaxInlineBytes = old })
	bus, c := immutableCatalogFixture251(t)
	left, right := bus.ShallowClone(), bus.ShallowClone()
	left.Mutable = bus.Mutable.ForkForExploreDispatch()
	right.Mutable = bus.Mutable.ForkForExploreDispatch()
	q := c.Snapshot().Queries[0]
	params, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": q.ID}})
	first, err := (&TraceQuery{}).Execute(left, params)
	if err != nil || !first.Success || first.ReusedFromRunMemo {
		t.Fatalf("first native query: %v %s", err, first.Summary)
	}
	a := left.Mutable.TraceCatalogs()[0].Snapshot().Queries[0].Attempts[0]
	if a.PayloadRef == "" || a.RawRef == "" || a.PayloadRef == a.RawRef {
		t.Fatalf("distinct native material missing: %+v", a)
	}
	before := map[string]filegeneration.Identity{}
	for _, ref := range []string{a.PayloadRef, a.RawRef} {
		before[ref], err = filegeneration.FromPath(ref)
		if err != nil {
			t.Fatal(err)
		}
	}
	second, err := (&TraceQuery{}).Execute(right, params)
	if err != nil || !second.Success || second.ReusedFromRunMemo {
		t.Fatalf("second native query: %v %s", err, second.Summary)
	}
	b := right.Mutable.TraceCatalogs()[0].Snapshot().Queries[0].Attempts[0]
	if a.ID == b.ID || a.PayloadRef != b.PayloadRef || a.RawRef != b.RawRef {
		t.Fatalf("query was skipped or identical material changed identity: first=%+v second=%+v", a, b)
	}
	for ref, identity := range before {
		after, err := filegeneration.FromPath(ref)
		if err != nil || !identity.SameVersion(after) {
			t.Errorf("same-byte result rewrote generation: %s (%v)", ref, err)
		}
	}
	bus.Mutable.MergeExploreFork(left.Mutable)
	bus.Mutable.MergeExploreFork(right.Mutable)
	m := exportedManifest250(t, exportCatalogCall250(t, bus, c, "both-attempts"))
	if !m.MaterialComplete || m.ExecutionCoverage.Attempts != 2 || len(m.References) != 4 {
		t.Fatalf("both live attempts must export: %+v", m)
	}
	for _, ref := range m.References {
		if ref.Status != "copied" {
			t.Errorf("unexported current material: %+v", ref)
		}
	}
}
