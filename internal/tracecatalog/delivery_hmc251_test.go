package tracecatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestCatalogDeliveryLiveForkUnionAndExactReuse251(t *testing.T) {
	c, a, work := exportCoreFixture250(t)
	left, right := c.Clone(), c.Clone()
	exportCoreAttempt250(t, left, a, work, "one", []byte(`{"native":1}`))
	exportCoreAttempt250(t, right, a, work, "two", []byte(`{"native":2}`))
	before := left.Snapshot()
	first, err := left.Export(context.Background(), work, "explicit-package")
	if err != nil {
		t.Fatal(err)
	}
	m := exportCoreManifest250(t, first)
	if !first.MaterialComplete || m.ExecutionCoverage.Attempts != 2 || len(m.Catalog.Queries) != 2 {
		t.Fatalf("export omitted a completed live sibling: %+v", m.ExecutionCoverage)
	}
	second, err := right.Export(context.Background(), work, "explicit-package")
	if err != nil {
		t.Fatalf("identical current delivery was not reusable: %v", err)
	}
	if first.PackageID != second.PackageID || first.ManifestSHA256 != second.ManifestSHA256 {
		t.Fatal("same explicit delivery produced another frozen package")
	}
	entries, err := os.ReadDir(filepath.Join(work, "evidence-exports"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("duplicate or partial publications: %v %v", entries, err)
	}
	if !reflect.DeepEqual(before, left.Snapshot()) {
		t.Fatal("export union changed caller query authority/state")
	}
}

func TestCatalogDeliveryChangesDoNotReuseCompletion251(t *testing.T) {
	for _, change := range []string{"new_attempt", "new_plan", "declared_origin", "representatives", "source_changed", "reference_changed", "failed_retry"} {
		t.Run(change, func(t *testing.T) {
			c, a, work := exportCoreFixture250(t)
			q := exportCoreAttempt250(t, c, a, work, "one", []byte(`{"native":1}`))
			fork := c.Clone()
			first, err := c.Export(context.Background(), work, "explicit")
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "new_attempt", "failed_retry":
				completion := Completion{Outcome: OutcomeFailure, Error: &Issue{Code: "actual_query_failed"}}
				if change == "new_attempt" {
					completion = Completion{Outcome: OutcomeEmpty, SourceRevision: "source-universe:" + a.RevisionID, PayloadRef: q.Attempts[0].PayloadRef}
				}
				newq, err := fork.Complete(context.Background(), q.ID, completion)
				if err != nil {
					t.Fatal(err)
				}
				fork.CaptureExportReferences(context.Background(), q.ID, newq.Attempts[len(newq.Attempts)-1].ID, work)
			case "new_plan":
				plan(t, fork, a.ID, "two", nil)
			case "declared_origin":
				if _, err := fork.RegisterDeclaredQuery(context.Background(), q.Plan); err != nil {
					t.Fatal(err)
				}
			case "representatives":
				if _, err := fork.SelectRepresentatives(context.Background(), RepresentativeOptions{}); err != nil {
					t.Fatal(err)
				}
			case "source_changed":
				putFile(t, a.Path, "source bytes changed")
			case "reference_changed":
				putFile(t, q.Attempts[0].PayloadRef, `{"native":1}`)
			}
			_, err = c.Export(context.Background(), work, "explicit")
			var conflict *ExportConflictError
			if !errors.As(err, &conflict) || !conflict.Delivery.PackageValid || conflict.Delivery.CurrentMatch || conflict.Delivery.Receipt.PackageID != first.PackageID {
				t.Fatalf("old completion reused or audit lost: %#v %v", conflict, err)
			}
			rows, err := fork.ExportDeliveries(context.Background(), work)
			if err != nil || len(rows) != 1 || rows[0].CurrentMatch || !rows[0].PackageValid {
				t.Fatalf("current audit: %+v %v", rows, err)
			}
			if change == "failed_retry" {
				newq, err := fork.Complete(context.Background(), q.ID, Completion{Outcome: OutcomeSuccess, SourceRevision: "source-universe:" + a.RevisionID, PayloadRef: q.Attempts[0].PayloadRef})
				if err != nil {
					t.Fatal(err)
				}
				fork.CaptureExportReferences(context.Background(), q.ID, newq.Attempts[len(newq.Attempts)-1].ID, work)
				second, err := fork.Export(context.Background(), work, "explicit-new-version")
				if err != nil {
					t.Fatal(err)
				}
				m := exportCoreManifest250(t, second)
				if m.ExecutionCoverage.Attempts != 3 || m.MaterialComplete {
					t.Fatalf("actual retry lost failure history or inferred complete: %+v", m)
				}
			}
		})
	}
}

func TestCatalogDeliveryPackageAndPrivateBoundary251(t *testing.T) {
	for _, change := range []string{"manifest", "report", "result", "package_symlink", "foreign_run", "historical_resume"} {
		t.Run(change, func(t *testing.T) {
			c, a, work := exportCoreFixture250(t)
			exportCoreAttempt250(t, c, a, work, "one", []byte(`{"native":1}`))
			first, err := c.Export(context.Background(), work, "explicit")
			if err != nil {
				t.Fatal(err)
			}
			manifest := exportCoreManifest250(t, first)
			switch change {
			case "manifest":
				putFile(t, first.ManifestPath, "{}")
			case "report":
				putFile(t, first.ReportPath, "changed report")
			case "result":
				putFile(t, filepath.Join(filepath.Dir(first.ManifestPath), manifest.Files[0].Path), "changed result")
			case "package_symlink":
				original := filepath.Dir(first.ManifestPath)
				moved := original + "-moved"
				if err := os.Rename(original, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, original); err != nil {
					t.Fatal(err)
				}
			case "foreign_run":
				c = discover(t, c.Snapshot().Root)
			case "historical_resume":
				data, _ := json.Marshal(c.Snapshot())
				old, err := DecodeSnapshot(data)
				if err != nil {
					t.Fatal(err)
				}
				c, err = Resume(context.Background(), old.Root, old, []Plan{old.Queries[0].Plan})
				if err != nil {
					t.Fatal(err)
				}
			}
			rows, err := c.ExportDeliveries(context.Background(), work)
			if err != nil {
				t.Fatal(err)
			}
			if change == "foreign_run" || change == "historical_resume" {
				if len(rows) != 0 {
					t.Fatal("disk/history minted a private delivery")
				}
			} else if len(rows) != 1 || rows[0].PackageValid || rows[0].CurrentMatch || rows[0].Status != "package_invalid" {
				t.Fatalf("tampered package certified: %+v", rows)
			}
			if _, err := c.Export(context.Background(), work, "explicit"); err == nil {
				t.Fatal("untrusted existing package reused")
			}
		})
	}
}

func TestCatalogDeliveryConcurrencyCancellationAndDetachedReceipt251(t *testing.T) {
	c, a, work := exportCoreFixture250(t)
	exportCoreAttempt250(t, c, a, work, "one", []byte(`{"native":1}`))
	const n = 8
	var wg sync.WaitGroup
	receipts := make(chan ExportReceipt, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		fork := c.Clone()
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := fork.Export(context.Background(), work, "shared")
			receipts <- r
			errs <- err
		}()
	}
	wg.Wait()
	close(receipts)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	physical, reused := 0, 0
	packageID := ""
	for r := range receipts {
		if r.Reused {
			reused++
		} else {
			physical++
		}
		if packageID != "" && packageID != r.PackageID {
			t.Fatal("different frozen packages")
		}
		packageID = r.PackageID
	}
	if physical != 1 || reused != n-1 {
		t.Fatalf("physical=%d reused=%d", physical, reused)
	}
	rows, err := c.ExportDeliveries(context.Background(), work)
	if err != nil {
		t.Fatal(err)
	}
	rows[0].Receipt.ExecutionCoverage.ObservedOutcomes[OutcomeSuccess] = 999
	again, err := c.Export(context.Background(), work, "shared")
	if err != nil || again.ExecutionCoverage.ObservedOutcomes[OutcomeSuccess] != 1 {
		t.Fatalf("receipt alias: %+v %v", again, err)
	}
	if _, err := c.Export(context.Background(), work, "another-explicit-target"); err != nil {
		t.Fatal("explicit multi-target denied", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Export(ctx, work, "shared"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel ignored: %v", err)
	}
	if _, err := c.ExportDeliveries(ctx, work); !errors.Is(err, context.Canceled) {
		t.Fatalf("audit cancel ignored: %v", err)
	}
	if _, err := c.Export(context.Background(), work, "../escape"); err == nil {
		t.Fatal("target escape accepted")
	}
}

func TestCatalogDeliveryBudgetAndReadOnlyNoop251(t *testing.T) {
	c, a, work := exportCoreFixture250(t)
	if rows, err := c.ExportDeliveries(context.Background(), "/definitely/unavailable"); err != nil || len(rows) != 0 {
		t.Fatalf("no publication should be cheap no-op: %v %v", rows, err)
	}
	for i := 0; i < MaxDeliveryBranches; i++ {
		fork := c.Clone()
		plan(t, fork, a.ID, fmt.Sprint(i), nil)
	}
	if _, err := c.Export(context.Background(), work, "bounded"); err == nil {
		t.Fatal("branch overflow certified partial union")
	}
	if _, err := os.Stat(filepath.Join(work, "evidence-exports", "bounded")); !os.IsNotExist(err) {
		t.Fatalf("overflow published: %v", err)
	}
}

func TestCatalogDeliveryChangedDuringPublicationAndFailureRetry251(t *testing.T) {
	c, a, work := exportCoreFixture250(t)
	exportCoreAttempt250(t, c, a, work, "one", []byte(`{"native":1}`))
	fork := c.Clone()
	checks := 0
	c.mu.Lock()
	b := c.bindings[a.ID]
	b.validate = func(context.Context) error {
		checks++
		if checks == 2 {
			plan(t, fork, a.ID, "new-during-copy", nil)
		}
		return nil
	}
	c.bindings[a.ID] = b
	c.mu.Unlock()
	if _, err := c.Export(context.Background(), work, "explicit"); err == nil {
		t.Fatal("concurrent new plan omitted from published revision")
	}
	if _, err := os.Stat(filepath.Join(work, "evidence-exports", "explicit")); !os.IsNotExist(err) {
		t.Fatal("changed transaction published")
	}
	if rows, err := c.ExportDeliveries(context.Background(), work); err != nil || len(rows) != 0 {
		t.Fatalf("failed publication minted receipt: %+v %v", rows, err)
	}
	// Same explicit target is retryable; the new registered plan remains pending.
	r, err := c.Export(context.Background(), work, "explicit")
	if err != nil {
		t.Fatal(err)
	}
	m := exportCoreManifest250(t, r)
	if r.Reused || r.MaterialComplete || len(m.Catalog.Queries) != 2 || m.ExecutionCoverage.Attempts != 1 {
		t.Fatalf("failed export blocked retry or fabricated new query: %+v", m)
	}
}

func TestCatalogDeliveryPersistenceClonesAreNotExecutionBranches251(t *testing.T) {
	c, a, work := exportCoreFixture250(t)
	exportCoreAttempt250(t, c, a, work, "one", []byte(`{"native":1}`))
	path := filepath.Join(work, "navigation.json")
	for i := 0; i < MaxDeliveryBranches+1; i++ {
		if err := c.Save(context.Background(), path); err != nil {
			t.Fatal(err)
		}
	}
	r, err := c.Export(context.Background(), work, "explicit")
	if err != nil || !r.MaterialComplete {
		t.Fatalf("persistence clones exhausted execution branch budget: %+v %v", r, err)
	}
}

func TestCatalogDeliveryProducerPublicationPrecedesPostWriteValidation251(t *testing.T) {
	c, a, work := exportCoreFixture250(t)
	exportCoreAttempt250(t, c, a, work, "one", []byte(`{"native":1}`))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var frozen *deliveryPublication
	r, err := c.exportNew(ctx, work, "explicit", nil, func(receipt ExportReceipt, files []ExportFile) {
		frozen = &deliveryPublication{fingerprint: c.deliveryFingerprint(), receipt: cloneExportReceipt(receipt), files: append([]ExportFile(nil), files...)}
		cancel()
	})
	if err != nil || frozen == nil || !r.PhysicalSuccess {
		t.Fatalf("successful publication did not retain its producer record: %+v %v", r, err)
	}
	if _, err := validateDeliveryPackage(ctx, frozen); !errors.Is(err, context.Canceled) {
		t.Fatalf("post-publication cancellation was not distinct: %v", err)
	}
	d := c.inspectDelivery(context.Background(), frozen)
	if !d.PackageValid || !d.CurrentMatch {
		t.Fatalf("new caller cannot independently audit previously published bytes: %+v", d)
	}
}
