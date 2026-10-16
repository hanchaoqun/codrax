package tool

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

func TestHMC253PendingExportNeverRenamesOrOverwritesFrozenTarget(t *testing.T) {
	bus, c := exportCatalogFixture250(t)
	frozen := exportCatalogCall250(t, bus, c, "original")
	_ = exportedManifest250(t, frozen)
	var receipt tracecatalog.ExportReceipt
	if err := json.Unmarshal([]byte(frozen.Summary), &receipt); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(receipt.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	bus.PipelineExports = tracecatalog.NewPipelineExports(bus.DeliveryRoot)
	original := pendingRequest253(t, exportCatalogCall250(t, bus, c, "original"))
	additional := pendingRequest253(t, exportCatalogCall250(t, bus, c, "additional-explicit"))
	q := c.Snapshot().Queries[0]
	raw, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": q.ID}})
	if out, err := (&TraceQuery{}).Execute(bus, raw); err != nil || !out.Success {
		t.Fatal(err, out.Summary)
	}
	rows, err := MaterializePendingTraceCatalogExports(bus)
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	states := map[string]tracecatalog.ExportRequest{}
	for _, r := range rows {
		states[r.IntentID] = r
	}
	old, newTarget := states[original.IntentID], states[additional.IntentID]
	if old.Target != "original" || old.Status != "failed" || old.ReasonCode == "" || newTarget.Status != "published" || newTarget.Delivery == nil || newTarget.Target != "additional-explicit" {
		t.Fatalf("extra target substituted original completion: %+v", rows)
	}
	after, err := os.ReadFile(receipt.ManifestPath)
	if err != nil || string(before) != string(after) {
		t.Fatal("frozen target was overwritten")
	}
}
