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

func TestHMC253PendingExportPrivateHostAndFiniteFailures(t *testing.T) {
	for _, kind := range []string{"standalone_finalize", "json_capability", "zero_capability", "replaced_root", "invalid_target", "cancelled_accept", "cancelled_terminal", "revoked_source", "new_run", "external_target"} {
		t.Run(kind, func(t *testing.T) {
			bus, c := exportCatalogFixture250(t)
			bus.PipelineStage = types.StageFinalize // A model-visible string cannot defer export.
			if kind == "standalone_finalize" {
				_ = exportedManifest250(t, exportCatalogCall250(t, bus, c, "standalone"))
				return
			}
			bus.PipelineExports = tracecatalog.NewPipelineExports(bus.DeliveryRoot)
			target := "requested"
			switch kind {
			case "zero_capability":
				bus.PipelineExports = &tracecatalog.PipelineExports{}
			case "json_capability":
				wire, _ := json.Marshal(bus.PipelineExports)
				bus.PipelineExports = &tracecatalog.PipelineExports{}
				if err := json.Unmarshal(wire, bus.PipelineExports); err != nil {
					t.Fatal(err)
				}
			case "replaced_root":
				bus.DeliveryRoot = tracecatalog.NewManagedExportRoot(t.TempDir())
			case "invalid_target":
				target = "../escape"
			case "cancelled_accept":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				bus.Ctx = ctx
			}
			out := exportCatalogCall250(t, bus, c, target)
			if kind == "zero_capability" || kind == "json_capability" || kind == "replaced_root" || kind == "invalid_target" || kind == "cancelled_accept" {
				if out.Success {
					t.Fatalf("invalid host request accepted: %s", out.Summary)
				}
				return
			}
			accepted := pendingRequest253(t, out)
			switch kind {
			case "cancelled_terminal":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				bus.Ctx = ctx
			case "revoked_source":
				bus.RepoRoot, bus.UserPinnedFiles = t.TempDir(), nil
			case "new_run":
				bus.Mutable = types.NewMutableState("another run cannot consume an old private intent")
			case "external_target":
				dir, err := bus.DeliveryRoot.Directory(context.Background(), true)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(dir, "evidence-exports", target), 0700); err != nil {
					t.Fatal(err)
				}
			}
			rows, err := MaterializePendingTraceCatalogExports(bus)
			if err != nil || len(rows) != 1 || rows[0].IntentID != accepted.IntentID || rows[0].Delivery != nil || rows[0].ReasonCode == "" || rows[0].Status == "published" {
				t.Fatalf("finite failure signed delivery: %+v %v", rows, err)
			}
			if kind == "cancelled_terminal" && rows[0].Status != "cancelled" {
				t.Fatal("cancellation lost")
			}
			// The same terminal invocation cannot turn into a retry after the
			// cancellation/authority condition is changed by the caller.
			bus.Ctx = context.Background()
			again, err := MaterializePendingTraceCatalogExports(bus)
			if err != nil || len(again) != 1 || again[0].Status != rows[0].Status || again[0].ReasonCode != rows[0].ReasonCode {
				t.Fatal("terminal failure was retried")
			}
		})
	}
}

func TestHMC253PendingExportHonestIncompleteAndNoIntent(t *testing.T) {
	bus, _ := catalogPublicFixture(t)
	bus.PipelineExports = tracecatalog.NewPipelineExports(bus.DeliveryRoot)
	if rows, err := MaterializePendingTraceCatalogExports(bus); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	if dir, err := bus.DeliveryRoot.Directory(context.Background(), false); err != nil || dir != "" {
		t.Fatal("empty terminal created a directory")
	}
	bus.PipelineExports = tracecatalog.NewPipelineExports(bus.DeliveryRoot)
	c := catalogPublicDiscover(t, bus)
	pendingRequest253(t, exportCatalogCall250(t, bus, c, "honest-unexecuted"))
	rows, err := MaterializePendingTraceCatalogExports(bus)
	if err != nil || len(rows) != 1 || rows[0].Status != "published" || rows[0].Delivery == nil || rows[0].Delivery.Receipt.MaterialComplete || rows[0].Delivery.Receipt.ExecutionCoverage.Attempts != 0 {
		t.Fatalf("unexecuted plans upgraded: %+v %v", rows, err)
	}
}
