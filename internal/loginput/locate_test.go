package loginput

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLocateExactSourcePhysicalBytesAndBoundaries(t *testing.T) {
	raw := []byte("header\r\n10-09 01:02:03.123 7 8 I Tag: hello\r\n  continuation\r\nnext row\r\n")
	var gzipBytes bytes.Buffer
	w := gzip.NewWriter(&gzipBytes)
	_, _ = w.Write(raw)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	c, err := Prepare(context.Background(), []Input{{Name: "session.gz", Data: gzipBytes.Bytes()}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	needles := []ExcerptSelector{{Text: "hello\n  continuation"}, {Text: "continuation\nnext row"}, {Text: "no match"}, {Text: ""}}
	got, err := c.Locate(context.Background(), needles)
	if err != nil {
		t.Fatal(err)
	}
	for i, lines := range [][2]int64{{2, 3}, {3, 4}} {
		g := got[i]
		if !g.Verified() || g.Matches != 1 || g.FirstLine != lines[0] || g.LastLine != lines[1] || g.Source.Compression != "gzip" {
			t.Fatalf("locator %d: %+v", i, g)
		}
		if normalizeExcerpt(string(raw[g.ByteStart:g.ByteEnd])) != needles[i].Text {
			t.Fatalf("wrong original byte span: %+v", g)
		}
	}
	if got[0].RecordID == "" || got[1].RecordID != "" || got[2].Status != "not_found" || got[3].Status != "no_evidence" {
		t.Fatalf("record boundary/status: %+v", got)
	}
	encoded, _ := json.Marshal(got[0])
	var restored ExcerptLocation
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Verified() {
		t.Fatal("JSON restored read authority")
	}
	got[0].FirstLine++
	if got[0].Verified() {
		t.Fatal("mutated coordinates retained read authority")
	}
}

func TestLocateDuplicatesFailureAndCancellation(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "change.log")
	if err := os.WriteFile(bad, []byte("different\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Prepare(context.Background(), []Input{{Name: "a/session", Data: []byte("same same\n")}, {Name: "b/session", Data: []byte("same\n")}, {Path: bad}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Locate(context.Background(), []ExcerptSelector{{Text: "same"}, {Text: "same", SourceID: c.Sources()[1].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != "ambiguous" || got[0].Matches != 3 || got[0].Source != nil || !got[1].Verified() {
		t.Fatalf("duplicate selection: %+v", got)
	}
	if err := os.WriteFile(bad, []byte("changed!!\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = c.Locate(context.Background(), []ExcerptSelector{{Text: "same"}, {Text: "same", SourceID: c.Sources()[1].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != "source_unavailable" || got[0].Source != nil || !got[1].Verified() {
		t.Fatalf("failed source gave global uniqueness: %+v", got)
	}
	if _, err := c.Locate(context.Background(), []ExcerptSelector{{Text: "same", SourceID: "fabricated"}}); err == nil {
		t.Fatal("accepted forged source")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Locate(ctx, []ExcerptSelector{{Text: "same"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
}
