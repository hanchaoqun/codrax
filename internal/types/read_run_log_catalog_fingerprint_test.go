package types

import (
	"context"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
)

func TestPreparedLogFingerprintIncludesCompleteSourcesNotPreview(t *testing.T) {
	prepare := func(body string) *loginput.Catalog {
		catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "inline", Data: []byte(body)}}, loginput.Options{})
		if err != nil {
			t.Fatal(err)
		}
		return catalog
	}
	a, b := prepare("head\nfirst-tail\n"), prepare("head\nother-tail\n")
	busA := &BusContext{AttachedLog: "head", AttachedLogCatalog: a}
	busB := &BusContext{AttachedLog: "head", AttachedLogCatalog: b}
	fa, fb := ReadRunAttachmentFingerprintsFromBusContext(busA), ReadRunAttachmentFingerprintsFromBusContext(busB)
	if len(fa) != 1 || len(fb) != 1 || ReadRunAttachmentFingerprintsEqual(fa[0], fb[0]) {
		t.Fatal("same preview collapsed different sources")
	}
	previewOnly := ReadRunAttachmentFingerprintsFromBusContext(&BusContext{AttachedLog: "head"})
	if ReadRunAttachmentFingerprintsEqual(fa[0], previewOnly[0]) {
		t.Fatal("persisted preview can resume fullsource")
	}
	clone := busA.ShallowClone()
	if clone.AttachedLogCatalog != a {
		t.Fatal("parallel fork lost catalog")
	}
}
