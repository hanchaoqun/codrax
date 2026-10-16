package tracecatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestHMC253PipelineExportsConcurrentSealingAndDetachedAudit(t *testing.T) {
	c, a, work := exportCoreFixture250(t)
	exportCoreAttempt250(t, c, a, work, "first", []byte(`{"native":1}`))
	root := NewManagedExportRoot(t.TempDir())
	p := NewPipelineExports(root)
	const callers = 8
	var wg sync.WaitGroup
	ids := make(chan string, callers)
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := p.Accept(context.Background(), root, c.Clone(), "shared-request")
			ids <- r.IntentID
			errs <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	identity := ""
	for id := range ids {
		if identity != "" && id != identity {
			t.Fatal("concurrent accepts created multiple identities")
		}
		identity = id
	}
	results := make(chan []ExportRequest, callers)
	errs = make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := p.Materialize(context.Background(), root, func(*Catalog) error { return nil })
			results <- rows
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	packageID := ""
	for rows := range results {
		if len(rows) != 1 || rows[0].Status != "published" || rows[0].Delivery == nil || !rows[0].Delivery.CurrentMatch {
			t.Fatalf("terminal rows: %+v", rows)
		}
		r := rows[0].Delivery.Receipt
		if packageID != "" && packageID != r.PackageID {
			t.Fatal("concurrent terminal calls published twice")
		}
		packageID = r.PackageID
		rows[0].Delivery.Receipt.ExecutionCoverage.ObservedOutcomes[OutcomeSuccess] = 999
	}
	rows, err := p.Requests(context.Background(), root, func(*Catalog) error { return nil })
	if err != nil || rows[0].Delivery.Receipt.ExecutionCoverage.ObservedOutcomes[OutcomeSuccess] != 1 {
		t.Fatal("audit aliases private receipt")
	}
	dir, err := root.Directory(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "evidence-exports"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "shared-request" {
		t.Fatal("multiple/staging packages", entries, err)
	}
}

func TestHMC253PipelineExportsLineageAndMultipleTargets(t *testing.T) {
	c, _, _ := exportCoreFixture250(t)
	root := NewManagedExportRoot(t.TempDir())
	p := NewPipelineExports(root)
	for _, target := range []string{"primary", "additional"} {
		if _, err := p.Accept(context.Background(), root, c, target); err != nil {
			t.Fatal(err)
		}
	}
	foreign := discover(t, c.Snapshot().Root)
	if _, err := p.Accept(context.Background(), root, foreign, "primary"); err == nil {
		t.Fatal("public catalog ID borrowed live lineage")
	}
	wire, _ := json.Marshal(p)
	var decoded PipelineExports
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, err := decoded.Materialize(context.Background(), root, func(*Catalog) error { return nil }); err == nil {
		t.Fatal("serialized capability materialized")
	}
	rows, err := p.Materialize(context.Background(), root, func(*Catalog) error { return fmt.Errorf("current authority revoked") })
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	for _, r := range rows {
		if r.Status != "failed" || r.ReasonCode != "source_authorization" || r.Delivery != nil {
			t.Fatal("failed target lost separate identity", r)
		}
	}
	if dir, err := root.Directory(context.Background(), false); err != nil || dir != "" {
		t.Fatal("failed requests created output")
	}
}
