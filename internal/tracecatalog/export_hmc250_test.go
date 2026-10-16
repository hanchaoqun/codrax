package tracecatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func exportCoreFixture250(t *testing.T) (*Catalog, Artifact, string) {
	t.Helper()
	root := t.TempDir()
	putFile(t, filepath.Join(root, "capture"), "source bytes")
	c := discover(t, root)
	a := c.Snapshot().Artifacts[0]
	bind(t, c, a)
	return c, a, t.TempDir()
}

func exportCoreAttempt250(t *testing.T, c *Catalog, a Artifact, work, object string, body []byte) QueryRecord {
	t.Helper()
	q := plan(t, c, a.ID, object, nil)
	path := filepath.Join(work, "result-"+object+".json")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	completed, err := c.Complete(context.Background(), q.ID, Completion{Outcome: OutcomeSuccess, SourceRevision: "source-universe:" + a.RevisionID, PayloadRef: path, RawRef: path})
	if err != nil {
		t.Fatal(err)
	}
	c.CaptureExportReferences(context.Background(), q.ID, completed.Attempts[0].ID, work)
	return completed
}

func exportCoreManifest250(t *testing.T, r ExportReceipt) ExportManifest {
	t.Helper()
	data, err := os.ReadFile(r.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var m ExportManifest
	if err = json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCatalogExportCloneMergeAndTransaction250(t *testing.T) {
	c, a, work := exportCoreFixture250(t)
	left, right := c.Clone(), c.Clone()
	exportCoreAttempt250(t, left, a, work, "one", []byte(`{"native":1}`))
	exportCoreAttempt250(t, right, a, work, "two", []byte(`{"native":2}`))
	if err := c.Merge(left); err != nil {
		t.Fatal(err)
	}
	if err := c.Merge(right); err != nil {
		t.Fatal(err)
	}
	before := c.Snapshot()
	r, err := c.Export(context.Background(), work, "merged")
	if err != nil {
		t.Fatal(err)
	}
	m := exportCoreManifest250(t, r)
	if !r.MaterialComplete || len(m.References) != 4 || len(m.Files) != 3 || m.ExecutionCoverage.ObservedQueries != 2 || m.ExecutionCoverage.Basis != "observed_queries_only" {
		t.Fatalf("live clones lost exact refs or claimed expected population: %+v", m)
	}
	if !reflect.DeepEqual(before, c.Snapshot()) {
		t.Fatal("export changed execution state")
	}
	if got, err := os.ReadFile(a.Path); err != nil || string(got) != "source bytes" {
		t.Fatal("source modified")
	}
	// Concurrent publishers must not replace an existing complete package.
	var wg sync.WaitGroup
	results := make(chan ExportReceipt, 2)
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := c.Export(context.Background(), work, "race")
			results <- r
			errors <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	physical, reused := 0, 0
	var packageID string
	for result := range results {
		if result.Reused {
			reused++
		} else {
			physical++
		}
		if packageID != "" && result.PackageID != packageID {
			t.Fatal("same target produced two packages")
		}
		packageID = result.PackageID
	}
	if physical != 1 || reused != 1 {
		t.Fatalf("atomic destination ownership: physical=%d reused=%d", physical, reused)
	}
}

func TestCatalogExportCancelAfterCopyPublishesNothing250(t *testing.T) {
	c, a, work := exportCoreFixture250(t)
	exportCoreAttempt250(t, c, a, work, "one", []byte(`{"native":1}`))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checks := 0
	c.mu.Lock()
	b := c.bindings[a.ID]
	b.validate = func(context.Context) error {
		checks++
		if checks == 2 {
			cancel()
		}
		return nil
	}
	c.bindings[a.ID] = b
	c.mu.Unlock()
	if _, err := c.Export(ctx, work, "cancel-after-copy"); err == nil {
		t.Fatal("canceled publication succeeded")
	}
	entries, err := os.ReadDir(filepath.Join(work, "evidence-exports"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial package or staging visible: %v", entries)
	}
}

func TestCatalogExportRefScopeBudgetAndMissing250(t *testing.T) {
	for _, kind := range []string{"outside_root", "child_symlink", "oversized", "unbound", "truncated"} {
		t.Run(kind, func(t *testing.T) {
			c, a, work := exportCoreFixture250(t)
			q := plan(t, c, a.ID, "one", nil)
			path := filepath.Join(work, "result.json")
			putFile(t, path, `{"native":1}`)
			switch kind {
			case "outside_root":
				path = filepath.Join(t.TempDir(), "outside.json")
				putFile(t, path, `{"outside":true}`)
			case "child_symlink":
				real := filepath.Join(t.TempDir(), "outside.json")
				putFile(t, real, `{"outside":true}`)
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(real, path); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				f, err := os.OpenFile(path, os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				if err = f.Truncate(MaxExportFileBytes + 1); err != nil {
					t.Fatal(err)
				}
				_ = f.Close()
			}
			got, err := c.Complete(context.Background(), q.ID, Completion{Outcome: OutcomeSuccess, SourceRevision: "source-universe:" + a.RevisionID, PayloadRef: path, Truncated: kind == "truncated"})
			if err != nil {
				t.Fatal(err)
			}
			if kind != "unbound" {
				c.CaptureExportReferences(context.Background(), q.ID, got.Attempts[0].ID, work)
			}
			r, err := c.Export(context.Background(), work, "bounded")
			if err != nil {
				t.Fatal(err)
			}
			m := exportCoreManifest250(t, r)
			if m.MaterialComplete {
				t.Fatalf("%s signed complete", kind)
			}
			if kind != "truncated" && (len(m.Files) != 1 || m.References[0].File != "") {
				t.Fatalf("unadmitted bytes copied: %+v", m.References)
			}
		})
	}
}

func TestCatalogExportMaterialBudgetWholeReferences250(t *testing.T) {
	if testing.Short() {
		t.Skip("bounded material budget exercise")
	}
	c, a, work := exportCoreFixture250(t)
	for i := 0; i < 9; i++ {
		exportCoreAttempt250(t, c, a, work, fmt.Sprintf("%02d", i), bytes.Repeat([]byte{byte('a' + i)}, 8<<20))
	}
	r, err := c.Export(context.Background(), work, "budget")
	if err != nil {
		t.Fatal(err)
	}
	m := exportCoreManifest250(t, r)
	if m.MaterialComplete {
		t.Fatal("omitted whole result signed complete")
	}
	var bytesCopied int64
	for _, file := range m.Files {
		if file.Path != "report.md" {
			bytesCopied += file.Bytes
			if file.Bytes != 8<<20 {
				t.Fatal("partial file copied")
			}
		}
	}
	if bytesCopied != MaxExportMaterialBytes {
		t.Fatalf("actual budget boundary=%d", bytesCopied)
	}
	omitted := 0
	for _, ref := range m.References {
		if ref.Status == "omitted_budget" {
			omitted++
			if ref.File != "" {
				t.Fatal("omitted reference has a file")
			}
		}
	}
	if omitted != 2 {
		t.Fatalf("budget omission roles=%d", omitted)
	}
}

func TestCatalogExportPlainManagedOutputAndNoRestore250(t *testing.T) {
	c, a, work := exportCoreFixture250(t)
	exportCoreAttempt250(t, c, a, work, "one", []byte(`{"native":1}`))
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(work, "evidence-exports")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Export(context.Background(), work, "escape"); err == nil {
		t.Fatal("output symlink accepted")
	}
	if files, err := os.ReadDir(outside); err != nil || len(files) != 0 {
		t.Fatal("export escaped managed output")
	}
	data, err := json.Marshal(c.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "exportRefs") || strings.Contains(string(data), "sha256") {
		t.Fatal("private export binding serialized")
	}
}

func TestCatalogExportAtomicNoReplaceExternalTarget250(t *testing.T) {
	path := t.TempDir()
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.Mkdir("stage", 0700); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("stage/complete.json", []byte("keep staged"), 0600); err != nil {
		t.Fatal(err)
	}
	// This does not acquire the exporter lock: it models an unrelated writer
	// creating an EMPTY destination between the Lstat and publication syscall.
	if err := root.Mkdir("occupied", 0700); err != nil {
		t.Fatal(err)
	}
	before, err := root.Stat("occupied")
	if err != nil {
		t.Fatal(err)
	}
	if err := exportPublishNoReplace(root, "stage", "occupied"); err == nil {
		t.Fatal("existing external directory replaced")
	}
	after, err := root.Stat("occupied")
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("external target identity changed")
	}
	if _, err := root.Stat("occupied/complete.json"); !os.IsNotExist(err) {
		t.Fatal("staged bytes leaked into external target")
	}
	if data, err := root.ReadFile("stage/complete.json"); err != nil || string(data) != "keep staged" {
		t.Fatal("failed publication destroyed staging")
	}
}
