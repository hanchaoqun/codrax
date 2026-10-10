package loginput

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCatalogQueryStablePaginationAtByteLimit(t *testing.T) {
	// A page must be a prefix of matching rows, never silently skip a large
	// record and return a later small record under the same Offset.
	data := "first\n" + strings.Repeat("large", 100) + "\nlast\n"
	c := mustCatalog(t, []Input{{Name: "source", Data: []byte(data)}}, Options{MaxResultBytes: 800})
	r := mustQuery(t, c, Query{})
	if r.Matched != 3 || r.Returned != 1 || !r.ResultByteLimitReached || r.Records[0].FirstLine != 1 {
		t.Fatalf("non-prefix byte-bounded page: %+v", r)
	}
}

func TestCatalogQueryMetadataCannotMutateReceipt(t *testing.T) {
	dir := t.TempDir()
	path, alias := filepath.Join(dir, "source"), filepath.Join(dir, "alias")
	writeSource(t, path, []byte("source\n"))
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	c := mustCatalog(t, []Input{{Path: path}, {Path: alias}}, Options{})
	r := mustQuery(t, c, Query{})
	r.Sources[0].Aliases[0] = filepath.Join(dir, "missing")
	if next := mustQuery(t, c, Query{}); !next.Complete {
		t.Fatalf("returned source metadata changed catalog receipt: %+v", next)
	}
}

func TestCatalogIndentedMalformedHeaderDoesNotStealFollowingStack(t *testing.T) {
	data := "10-09 01:02:03.123 1 2 I Tag: good\n 13-09 01:02:03.123 99 100 E Other: bad\n  following stack\n"
	c := mustCatalog(t, []Input{{Name: "input", Data: []byte(data)}}, Options{})
	r := mustQuery(t, c, Query{})
	if r.Matched != 3 || r.Records[0].LastLine != 1 || r.Records[1].Status != "malformed" || r.Records[2].Status != "orphan_continuation" {
		t.Fatalf("malformed header was stolen as healthy continuation: %+v", r)
	}
}

func TestCatalogAliasRebindingAndDuringReadMutationFailClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source")
	alias := filepath.Join(dir, "alias")
	writeSource(t, path, []byte("first\nsecond\n"))
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	c := mustCatalog(t, []Input{{Path: path}, {Path: alias}}, Options{})
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	writeSource(t, alias, []byte("rebound\n"))
	r := mustQuery(t, c, Query{})
	if r.Complete || r.Matched != 0 || len(r.SourceErrors) != 1 {
		t.Fatalf("rebound alias retained authority: %+v", r)
	}
	clean := mustCatalog(t, []Input{{Path: path}}, Options{})
	s := clean.sources[0]
	changed := false
	err := scanSource(context.Background(), &s, clean.opts, func(Record) {
		if !changed {
			changed = true
			writeSource(t, path, []byte("changed bytes\n"))
		}
	}, true)
	if err == nil || s.summary.Complete {
		t.Fatalf("mutation during held read accepted: %v %+v", err, s.summary)
	}
}

func TestCatalogConcurrentReadOwnershipAndNoSuffixFormatInference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-really-gzip.gz")
	writeSource(t, path, []byte("plain unchanged\n"))
	c := mustCatalog(t, []Input{{Path: path}}, Options{})
	if c.Sources()[0].Compression != "none" {
		t.Fatal("extension invented compression")
	}
	var wait sync.WaitGroup
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for j := 0; j < 4; j++ {
				r, err := c.Query(context.Background(), Query{})
				if err != nil || !r.Complete || r.Matched != 1 {
					t.Errorf("concurrent query: %+v %v", r, err)
					return
				}
				r.Records[0].RawBytes[0] = 'X'
				if !bytes.Equal(mustQuery(t, c, Query{}).Records[0].RawBytes, []byte("plain unchanged\n")) {
					t.Error("result bytes mutated catalog")
				}
			}
		}()
	}
	wait.Wait()
}

func TestCatalogLateRecordOutsidePreviewAndNonPrintablePreview(t *testing.T) {
	data := []byte(strings.Repeat("background\n", 3000) + "10-09 01:02:03.123 77 78 E Tag: late-important\n")
	c := mustCatalog(t, []Input{{Name: "complete", Data: data}}, Options{})
	if strings.Contains(c.Preview(65536), "late-important") {
		t.Fatal("fixture no longer outside cached preview")
	}
	r := mustQuery(t, c, Query{Contains: "late-important"})
	if !r.Complete || r.Matched != 1 || r.Records[0].FirstLine != 3001 || *r.Records[0].PID != 77 {
		t.Fatalf("query incorrectly limited to preview: %+v", r)
	}
	unsafe := mustCatalog(t, []Input{{Name: "control", Data: []byte("escape\x1b[2Jnul\x00\n")}}, Options{})
	if preview := unsafe.Preview(65536); strings.ContainsAny(preview, "\x00\x1b") || !strings.Contains(preview, "exact bytes") {
		t.Fatalf("unsafe/implicit display: %q", preview)
	}
	if !bytes.Equal(mustQuery(t, unsafe, Query{}).Records[0].RawBytes, []byte("escape\x1b[2Jnul\x00\n")) {
		t.Fatal("safe preview mutated complete bytes")
	}
}

func TestCatalogPartialDecimalTimestampAndCancelDuringScan(t *testing.T) {
	c := mustCatalog(t, []Input{{Name: "raw", Data: []byte("02-29 01:02:03.1 0 0 I Tag: year unknown\n[9007199.2547409931] too precise\n")}}, Options{})
	r := mustQuery(t, c, Query{})
	if r.Records[0].Status != "parsed" || r.Records[0].WallTimestamp != "02-29 01:02:03.1" || r.Records[1].Status != "malformed" || r.Records[1].BootTimestampNS != "" {
		t.Fatalf("timestamp guessed/rounded: %+v", r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := c.sources[0]
	err := scanSource(ctx, &s, c.opts, func(Record) { cancel() }, true)
	if !errors.Is(err, context.Canceled) || s.summary.Complete {
		t.Fatalf("canceled scan accepted: %v %+v", err, s.summary)
	}
}

func TestCatalogPreviewSeparatesUnterminatedSourcesAndFailureExplainsCause(t *testing.T) {
	c := mustCatalog(t, []Input{{Name: "first", Data: []byte("without newline")}, {Name: "second", Data: []byte("second content")}}, Options{})
	if preview := c.Preview(65536); !strings.Contains(preview, "without newline\n# codrax-source: second") {
		t.Fatalf("source boundaries merged: %q", preview)
	}
	_, err := Prepare(context.Background(), []Input{{Name: "bad.db", Data: []byte("SQLite format 3\x00")}}, Options{})
	if err == nil || !strings.Contains(err.Error(), "sqlite") || !strings.Contains(err.Error(), "bad.db") {
		t.Fatalf("all-bad diagnostics hid cause: %v", err)
	}
}
