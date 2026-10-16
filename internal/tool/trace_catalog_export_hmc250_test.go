package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func exportCatalogFixture250(t *testing.T) (*types.BusContext, *tracecatalog.Catalog) {
	t.Helper()
	bus, root := catalogPublicFixture(t)
	if err := os.Remove(filepath.Join(root, "broken.systrace")); err != nil {
		t.Fatal(err)
	}
	out, err := (&TraceCatalog{}).Execute(bus, []byte(`{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05},{"view":"resource_stack","pid":999,"time_start":10,"time_end":10.05}]}`))
	if err != nil || !out.Success {
		t.Fatalf("discover: %v %s", err, out.Summary)
	}
	c := bus.Mutable.TraceCatalogs()[0]
	for _, q := range c.Snapshot().Queries {
		raw, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": q.ID}})
		got, err := (&TraceQuery{}).Execute(bus, raw)
		if err != nil || !got.Success {
			t.Fatalf("query: %v %s", err, got.Summary)
		}
	}
	return bus, c
}

func exportedManifest250(t *testing.T, out types.ToolResult) tracecatalog.ExportManifest {
	t.Helper()
	if !out.Success {
		t.Fatalf("export refused: %s", out.Summary)
	}
	var receipt tracecatalog.ExportReceipt
	if err := json.Unmarshal([]byte(out.Summary), &receipt); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(receipt.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest tracecatalog.ExportManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if !receipt.PhysicalSuccess || receipt.MaterialComplete != manifest.MaterialComplete || manifest.NativeAvailability != "unknown" || manifest.AnalysisCompletion != "not_asserted" {
		t.Fatalf("publication axes: %s", out.Summary)
	}
	return manifest
}

func TestTraceCatalogExportPartialAndHistorical250(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	for _, item := range []struct {
		path string
		pid  int
	}{{"capture.data", 101}, {"nested/capture.data", 999}, {"broken.systrace", 101}} {
		_ = catalogPublicQuery(t, bus, filepath.Join(root, item.path), item.pid)
	}
	m := exportedManifest250(t, exportCatalogCall250(t, bus, c, "partial-v1"))
	if m.MaterialComplete || m.ExecutionCoverage.PlannedOutcomes[tracecatalog.OutcomeNotExecuted] != 3 || m.ExecutionCoverage.PlannedOutcomes[tracecatalog.OutcomeFailure] != 1 || m.ExecutionCoverage.PlannedOutcomes[tracecatalog.OutcomeEmpty] != 1 {
		t.Fatalf("partial outcomes hidden: %+v", m.ExecutionCoverage)
	}
	saved, err := saveTraceCatalog(bus, c)
	if err != nil {
		t.Fatal(err)
	}
	fresh := bus.ShallowClone()
	fresh.Mutable = types.NewMutableState("resume current")
	fresh.WorkDir = t.TempDir()
	fresh.UserPinnedFiles = []string{saved}
	if got := exportCatalogCall250(t, fresh, c, "old-id"); got.Success {
		t.Fatal("historical id was accepted")
	}
	resume, _ := json.Marshal(map[string]string{"action": "resume", "root": root, "snapshot_path": saved})
	got, err := (&TraceCatalog{}).Execute(fresh, resume)
	if err != nil || !got.Success {
		t.Fatalf("resume: %v %s", err, got.Summary)
	}
	current := fresh.Mutable.TraceCatalogs()[0]
	m = exportedManifest250(t, exportCatalogCall250(t, fresh, current, "resumed-pending"))
	if m.MaterialComplete || len(m.References) != 0 || m.ExecutionCoverage.Attempts != 0 || m.ExecutionCoverage.ResumedPlans != 6 || len(m.Catalog.History) != 1 {
		t.Fatalf("history restored material/current state: %+v", m)
	}
}

func TestTraceCatalogExportReferenceIntegrity250(t *testing.T) {
	for _, mutation := range []string{"missing", "same_size_rewrite", "source_changed", "unbound_replay"} {
		t.Run(mutation, func(t *testing.T) {
			bus, c := exportCatalogFixture250(t)
			q := c.Snapshot().Queries[0]
			a := q.Attempts[0]
			switch mutation {
			case "missing":
				if err := os.Remove(a.PayloadRef); err != nil {
					t.Fatal(err)
				}
			case "same_size_rewrite":
				info, err := os.Stat(a.PayloadRef)
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(a.PayloadRef)
				if err != nil {
					t.Fatal(err)
				}
				data[0] ^= 1
				if err = os.WriteFile(a.PayloadRef, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err = os.Chtimes(a.PayloadRef, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			case "source_changed":
				for _, source := range c.Snapshot().Artifacts {
					if source.ID == q.ArtifactID {
						if err := os.WriteFile(source.Path, []byte("changed"), 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
			case "unbound_replay":
				// A Complete call with copied public fields is not the live producer
				// reference hook, even when its source was bound by an earlier call.
				if _, err := c.Complete(context.Background(), q.ID, a.Completion); err != nil {
					t.Fatal(err)
				}
			}
			m := exportedManifest250(t, exportCatalogCall250(t, bus, c, "integrity"))
			if m.MaterialComplete {
				t.Fatalf("%s signed complete", mutation)
			}
			if mutation == "same_size_rewrite" {
				found := false
				for _, ref := range m.References {
					found = found || ref.Status == "stale_reference"
				}
				if !found {
					t.Fatal("changed reference lost typed status")
				}
			}
		})
	}
}

func TestTraceCatalogExportTargetAuthorityAndCancellation250(t *testing.T) {
	bus, c := exportCatalogFixture250(t)
	for _, name := range []string{"", "../source", "/absolute", "nested/package", "a\\b", ".hidden"} {
		if got := exportCatalogCall250(t, bus, c, name); got.Success {
			t.Fatalf("invalid target accepted %q", name)
		}
	}
	first := exportCatalogCall250(t, bus, c, "keep")
	m := exportedManifest250(t, first)
	var r tracecatalog.ExportReceipt
	_ = json.Unmarshal([]byte(first.Summary), &r)
	before, _ := os.ReadFile(r.ManifestPath)
	second := exportCatalogCall250(t, bus, c, "keep")
	var reused tracecatalog.ExportReceipt
	_ = json.Unmarshal([]byte(second.Summary), &reused)
	if !second.Success || !reused.Reused || reused.PackageID != r.PackageID || reused.ManifestSHA256 != r.ManifestSHA256 {
		t.Fatalf("unchanged live target did not share its verified receipt: %s", second.Summary)
	}
	after, _ := os.ReadFile(r.ManifestPath)
	if string(before) != string(after) || m.PackageID == "" {
		t.Fatal("old output changed")
	}
	deliveryRoot, err := bus.DeliveryRoot.Directory(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(deliveryRoot, "evidence-exports", "external")
	if err := os.Mkdir(external, 0700); err != nil {
		t.Fatal(err)
	}
	if got := exportCatalogCall250(t, bus, c, "external"); got.Success {
		t.Fatal("external target overwritten or adopted")
	}
	for _, raw := range []string{`{"action":"status","export":{"target":"x"}}`, `{"action":"export","catalog_id":"` + c.ID() + `","export":{"target":"x","overwrite":true}}`, `{"action":"export","catalog_id":"` + c.ID() + `","root":".","export":{"target":"x"}}`} {
		got, err := (&TraceCatalog{}).Execute(bus, []byte(raw))
		if err == nil && got.Success {
			t.Fatalf("mixed/unknown args accepted %s", raw)
		}
	}
	denied := bus.ShallowClone()
	denied.RepoRoot = t.TempDir()
	denied.UserPinnedFiles = nil
	if got := exportCatalogCall250(t, denied, c, "revoked"); got.Success {
		t.Fatal("current authorization revocation ignored")
	}
	canceled := bus.ShallowClone()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled.Ctx = ctx
	if got := exportCatalogCall250(t, canceled, c, "canceled"); got.Success {
		t.Fatal("canceled publication succeeded")
	}
	if _, err := os.Stat(filepath.Join(deliveryRoot, "evidence-exports", "canceled")); !os.IsNotExist(err) {
		t.Fatal("canceled package visible")
	}
	entries, err := os.ReadDir(filepath.Join(deliveryRoot, "evidence-exports"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".stage-") || strings.HasSuffix(entry.Name(), ".lock") {
			t.Fatal("staging or lock leaked")
		}
	}
}

func exportCatalogCall250(t *testing.T, bus *types.BusContext, c *tracecatalog.Catalog, target string) types.ToolResult {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"action": "export", "catalog_id": c.ID(), "export": map[string]string{"target": target}})
	out, err := (&TraceCatalog{}).Execute(bus, raw)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTraceCatalogExportPublicVersionedPackage250(t *testing.T) {
	bus, c := exportCatalogFixture250(t)
	out := exportCatalogCall250(t, bus, c, "capture-evidence-v1")
	if !out.Success {
		t.Fatalf("public export unavailable: %s", out.Summary)
	}
	var result struct {
		ManifestPath     string `json:"manifest_path"`
		PhysicalSuccess  bool   `json:"physical_success"`
		MaterialComplete bool   `json:"material_complete"`
	}
	if err := json.Unmarshal([]byte(out.Summary), &result); err != nil {
		t.Fatal(err)
	}
	if !result.PhysicalSuccess || !result.MaterialComplete {
		t.Fatalf("export axes: %s", out.Summary)
	}
	data, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SchemaVersion      int    `json:"schema_version"`
		NativeAvailability string `json:"native_availability"`
		Files              []struct {
			Path, SHA256 string
			Bytes        int64
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 1 || manifest.NativeAvailability != "unknown" || len(manifest.Files) == 0 {
		t.Fatalf("manifest: %s", data)
	}
	for _, f := range manifest.Files {
		body, err := os.ReadFile(filepath.Join(filepath.Dir(result.ManifestPath), f.Path))
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(body)
		if hex.EncodeToString(hash[:]) != f.SHA256 || int64(len(body)) != f.Bytes {
			t.Fatalf("copied reference mismatch: %+v", f)
		}
	}
	if len(out.Observations) != 0 || out.ReadCoverage != nil || out.TraceQuerySourceRead.Path() != "" {
		t.Fatal("export minted runtime/read evidence")
	}
}
