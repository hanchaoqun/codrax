package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Only the host capability changes publication timing. Stage is not authority.
func TestHMC253PipelineExportDefersPhysicalPublication(t *testing.T) {
	bus, c := exportCatalogFixture250(t)
	bus.PipelineStage = types.StageExplore
	bus.PipelineExports = tracecatalog.NewPipelineExports(bus.DeliveryRoot)
	got := exportCatalogCall250(t, bus, c, "requested-capture")
	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(got.Summary), &result); err != nil || !got.Success || result.Status != "pending_publication" {
		t.Fatalf("pipeline export published before terminal current-set closure: err=%v success=%v result=%s", err, got.Success, got.Summary)
	}
	root, err := bus.DeliveryRoot.Directory(context.Background(), false)
	if err != nil || root != "" {
		t.Fatalf("accepted intent created a physical delivery directory: %q %v", root, err)
	}
}

func pendingRequest253(t *testing.T, result types.ToolResult) tracecatalog.ExportRequest {
	t.Helper()
	var request tracecatalog.ExportRequest
	if err := json.Unmarshal([]byte(result.Summary), &request); err != nil || !result.Success || request.IntentID == "" {
		t.Fatalf("accepted request: %v %+v", err, result)
	}
	return request
}

func TestHMC253PendingExportLateForkUnionAndSameTarget(t *testing.T) {
	bus, c := exportCatalogFixture250(t)
	bus.PipelineExports = tracecatalog.NewPipelineExports(bus.DeliveryRoot)
	left, right := bus.ShallowClone(), bus.ShallowClone()
	left.Mutable, right.Mutable = bus.Mutable.ForkForExploreDispatch(), bus.Mutable.ForkForExploreDispatch()
	first := pendingRequest253(t, exportCatalogCall250(t, left, c, "original-target"))
	if again := pendingRequest253(t, exportCatalogCall250(t, right, c, first.Target)); !reflect.DeepEqual(first, again) {
		t.Fatal("same live intent did not deduplicate across branches")
	}
	// The other branch actually executes additional attempts, including empty
	// native results. No fixed attempt count or success-only terminal condition.
	for _, q := range right.Mutable.TraceCatalogs()[0].Snapshot().Queries {
		raw, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": q.ID}})
		out, err := (&TraceQuery{}).Execute(right, raw)
		if err != nil || !out.Success {
			t.Fatalf("late query: %v %s", err, out.Summary)
		}
	}
	bus.Mutable.MergeExploreFork(left.Mutable)
	bus.Mutable.MergeExploreFork(right.Mutable)
	current := bus.Mutable.TraceCatalogs()[0].CurrentExecution()
	rows, err := MaterializePendingTraceCatalogExports(bus)
	if err != nil || len(rows) != 1 || rows[0].IntentID != first.IntentID || rows[0].Target != first.Target || rows[0].Status != "published" || rows[0].Delivery == nil || !rows[0].Delivery.CurrentMatch {
		t.Fatalf("terminal original delivery: %+v %v", rows, err)
	}
	receipt := rows[0].Delivery.Receipt
	wire, _ := json.Marshal(receipt)
	manifest := exportedManifest250(t, types.ToolResult{Success: true, Summary: string(wire)})
	// Manifest formatting expands RawMessage whitespace. Compare the exact
	// canonical JSON fields, not formatting bytes nested inside Go raw slices.
	actualQueries, _ := json.Marshal(manifest.Catalog.Queries)
	expectedQueries, _ := json.Marshal(current.Snapshot.Queries)
	if string(actualQueries) != string(expectedQueries) || !reflect.DeepEqual(manifest.ExecutionCoverage, current.Coverage) {
		t.Fatal("terminal package did not preserve exact final query/attempt union")
	}
	for _, file := range manifest.Files {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(receipt.ManifestPath), file.Path))
		if err != nil || int64(len(data)) != file.Bytes || dispatchReadVersionSHA(string(data)) != file.SHA256 {
			t.Fatalf("bound file bytes/hash mismatch: %+v %v", file, err)
		}
	}
	before, _ := os.ReadFile(receipt.ManifestPath)
	again, err := MaterializePendingTraceCatalogExports(bus)
	if err != nil || len(again) != 1 || again[0].Delivery == nil || again[0].Delivery.Receipt.PackageID != receipt.PackageID || !reflect.DeepEqual(current, bus.Mutable.TraceCatalogs()[0].CurrentExecution()) {
		t.Fatal("terminal replay changed package or executed a query")
	}
	if late := exportCatalogCall250(t, bus, c, "invented-late-name"); late.Success {
		t.Fatal("sealed run accepted a replacement target")
	}
	// A later real attempt cannot make the prior frozen publication current.
	q := current.Snapshot.Queries[0]
	raw, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": q.ID}})
	if out, err := (&TraceQuery{}).Execute(bus, raw); err != nil || !out.Success {
		t.Fatal(err, out.Summary)
	}
	stale := pendingRequest253(t, exportCatalogCall250(t, bus, c, first.Target))
	if stale.Status != "failed" || stale.Delivery == nil || stale.Delivery.CurrentMatch || stale.IntentID != first.IntentID {
		t.Fatalf("stale receipt or target identity upgraded: %+v", stale)
	}
	after, _ := os.ReadFile(receipt.ManifestPath)
	if string(before) != string(after) {
		t.Fatal("frozen bytes overwritten")
	}
}
