package loginput

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadRecordFullSourceReceiptAndByteBudgetIndependence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	data := []byte("first full original record\nsecond\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	catalog, err := Prepare(context.Background(), []Input{{Path: path}}, Options{MaxResultBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	page, err := catalog.Query(context.Background(), Query{})
	if err != nil || page.Returned != 0 || !page.ResultByteLimitReached {
		t.Fatal("fixture must exhaust query page budget", page, err)
	}
	id := catalog.Sources()[0].ID + ":L1-1"
	record, source, err := catalog.ReadRecord(context.Background(), id)
	if err != nil || !source.Complete || !bytes.Equal(record.RawBytes, []byte("first full original record\n")) {
		t.Fatal(record, source, err)
	}
	if _, _, err := catalog.ReadRecord(context.Background(), id+"x"); err == nil {
		t.Fatal("non-native ID accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if record, _, err := catalog.ReadRecord(canceled, id); !errors.Is(err, context.Canceled) || record.ID != "" {
		t.Fatal("canceled read returned record", record, err)
	}
	if err := os.WriteFile(path, []byte("first full original record\nCHANGED\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if record, _, err := catalog.ReadRecord(context.Background(), id); err == nil || record.ID != "" {
		t.Fatal("changed tail source accepted unchanged early record", record, err)
	}
}
