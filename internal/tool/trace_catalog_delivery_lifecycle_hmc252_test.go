package tool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHMC252TraceCatalogExportRequiresHostDeliveryScope(t *testing.T) {
	bus, c := exportCatalogFixture250(t)
	bus.DeliveryRoot = nil
	if r := exportCatalogCall250(t, bus, c, "capture-review"); r.Success {
		t.Fatal("export silently borrowed disposable WorkDir")
	}
	if _, err := os.Stat(filepath.Join(bus.WorkDir, "evidence-exports")); !os.IsNotExist(err) {
		t.Fatalf("failed export created a temporary deliverable: %v", err)
	}
	if deliveries, err := AuthorizedTraceCatalogDeliveries(bus); err != nil || len(deliveries) != 0 {
		t.Fatalf("missing host restored publication: %v %+v", err, deliveries)
	}
}
