package loginput

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustCatalog(t *testing.T, inputs []Input, opts Options) *Catalog {
	t.Helper()
	c, err := Prepare(context.Background(), inputs, opts)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mustQuery(t *testing.T, c *Catalog, q Query) Result {
	t.Helper()
	r, err := c.Query(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func writeSource(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCatalogPhysicalProvenanceRawValuesAndMalformedBoundary(t *testing.T) {
	input := "10-09 01:02:03.123 12 34 I Media: first\r\n  stack1\r\n13-09 01:02:03.123 99 98 E Other: bad date\n  bad stack must not join first\n10-09 01:02:04.123 <6> [9007199.254740993] -;[2] pid=40 tid=41 comm=worker message\nunknown end"
	c := mustCatalog(t, []Input{{Name: "capture.log", Data: []byte(input)}}, Options{})
	r := mustQuery(t, c, Query{})
	if !r.Complete || r.Matched != 5 || len(r.Records) != 5 {
		t.Fatalf("wrong population: %+v", r)
	}
	first := r.Records[0]
	if first.Kind != KindHilog || first.FirstLine != 1 || first.LastLine != 2 || *first.PID != 12 || *first.TID != 34 || first.RawText != "10-09 01:02:03.123 12 34 I Media: first\r\n  stack1\r\n" {
		t.Fatalf("wrong first record: %+v", first)
	}
	if r.Records[1].Status != "malformed" || r.Records[2].Status != "orphan_continuation" {
		t.Fatalf("bad row stole continuation: %+v", r.Records)
	}
	kernel := r.Records[3]
	if kernel.BootTimestampNS != "9007199254740993" || kernel.ClockDomain != "source_boot_unmapped" || *kernel.CPU != 2 || *kernel.PID != 40 || *kernel.TID != 41 || kernel.Comm != "worker" {
		t.Fatalf("boot/context lost: %+v", kernel)
	}
	if first.ClockDomain != "wall_year_and_timezone_unknown" || first.WallTimestamp != "10-09 01:02:03.123" {
		t.Fatalf("invented wall clock: %+v", first)
	}
	var raw []byte
	for _, record := range r.Records {
		if record.ByteStart != int64(len(raw)) || record.ByteEnd-record.ByteStart != int64(len(record.RawBytes)) {
			t.Fatalf("bad byte range: %+v", record)
		}
		raw = append(raw, record.RawBytes...)
	}
	if string(raw) != input {
		t.Fatal("raw bytes changed")
	}
	source := c.Sources()[0]
	hash := sha256.Sum256([]byte(input))
	if source.OriginalSHA256 != hex.EncodeToString(hash[:]) || source.DecodedSHA256 != source.OriginalSHA256 || source.PhysicalLines != 6 || source.MalformedRecords != 1 || source.OrphanContinuations != 1 || source.Records != 5 {
		t.Fatalf("wrong coverage: %+v", source)
	}
	encoded, _ := json.Marshal(kernel)
	if !bytes.Contains(encoded, []byte(`"boot_timestamp_ns":"9007199254740993"`)) {
		t.Fatal(string(encoded))
	}
}

func TestCatalogGzipAndDistinctSameNameSources(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := os.Mkdir(a, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(b, 0700); err != nil {
		t.Fatal(err)
	}
	pathA, pathB := filepath.Join(a, "hilog.txt"), filepath.Join(b, "hilog.txt")
	plain := []byte("10-09 01:02:03.123 1 2 I Tag: one\n")
	compressed := gzipBytes(t, plain)
	writeSource(t, pathA, plain)
	writeSource(t, pathB, compressed)
	alias := filepath.Join(dir, "alias")
	if err := os.Link(pathA, alias); err != nil {
		t.Fatal(err)
	}
	c := mustCatalog(t, []Input{{Path: pathA}, {Path: pathA}, {Path: alias}, {Path: pathB}}, Options{})
	r := mustQuery(t, c, Query{})
	if len(c.Sources()) != 2 || r.Matched != 2 || !r.Complete {
		t.Fatalf("dedup merged independent names or doubled physical sources: %+v", r)
	}
	sources := c.Sources()
	if sources[1].Compression != "gzip" || sources[1].OriginalSHA256 == sources[1].DecodedSHA256 || sources[1].DecodedSHA256 != sources[0].DecodedSHA256 || sources[0].ID == sources[1].ID {
		t.Fatalf("bad compressed identities: %+v", sources)
	}
	if !bytes.Equal(r.Records[0].RawBytes, r.Records[1].RawBytes) {
		t.Fatal("decoded raw bytes lost")
	}
	for path, want := range map[string][]byte{pathA: plain, pathB: compressed} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("source modified: %s", path)
		}
	}
	// Altering a returned metadata slice must not alter catalog bindings.
	sources[0].Aliases[0] = filepath.Join(dir, "nonexistent")
	if !mustQuery(t, c, Query{}).Complete {
		t.Fatal("metadata alias mutated private authority")
	}
}

func TestCatalogMalformedGzipRetainsOtherSourcesAndNoPartialRows(t *testing.T) {
	bad := gzipBytes(t, []byte("valid-looking line\n"))
	bad[len(bad)-8] ^= 0xff
	c := mustCatalog(t, []Input{{Name: "good", Data: []byte("good\n")}, {Name: "broken", Data: bad}}, Options{})
	r := mustQuery(t, c, Query{})
	if r.Complete || len(r.SourceErrors) != 1 || r.Matched != 1 || r.Records[0].RawText != "good\n" {
		t.Fatalf("bad member published or hid healthy: %+v", r)
	}
	allBad, err := Prepare(context.Background(), []Input{{Name: "broken", Data: bad}}, Options{})
	if err == nil || allBad == nil || allBad.Sources()[0].Complete {
		t.Fatal("all-bad input reported success")
	}
}

func TestCatalogPaginationFiltersAndContinuationOverlap(t *testing.T) {
	data := "10-09 01:02:03.123 1 2 I Tag: one\n  continuation\n10-09 01:02:04.123 3 4 E Tag: two\nlast\n"
	c := mustCatalog(t, []Input{{Name: "input", Data: []byte(data)}}, Options{})
	r := mustQuery(t, c, Query{FirstLine: 2, LastLine: 2})
	if r.Matched != 1 || r.Records[0].FirstLine != 1 || r.Records[0].LastLine != 2 {
		t.Fatalf("line overlap lost full record identity: %+v", r)
	}
	r = mustQuery(t, c, Query{Offset: 1, Limit: 1})
	if r.Matched != 3 || r.Returned != 1 || r.Omitted != 2 || r.Records[0].FirstLine != 3 {
		t.Fatalf("wrong page accounting: %+v", r)
	}
	id := int64(3)
	r = mustQuery(t, c, Query{PID: &id, Kinds: []string{KindHilog}, Contains: "two", SourceIDs: []string{c.Sources()[0].ID}})
	if r.Matched != 1 || r.Records[0].FirstLine != 3 {
		t.Fatalf("wrong filtering: %+v", r)
	}
	for _, q := range []Query{{Limit: 1001}, {Offset: -1}, {SourceIDs: []string{"fake"}}, {FirstLine: 3, LastLine: 2}, {Kinds: []string{"runtime"}}} {
		if _, err := c.Query(context.Background(), q); err == nil {
			t.Fatalf("invalid query accepted: %+v", q)
		}
	}
}

func TestCatalogMemoryOwnershipAppendPreviewAndInvalidUTF8(t *testing.T) {
	data := append([]byte("good\n"), []byte{0xff, 0, 0xfe, '\n'}...)
	c := mustCatalog(t, []Input{{Name: "inline", Data: data}}, Options{})
	data[0] = 'X'
	if got := c.Preview(2); got != "go" {
		t.Fatalf("preview=%q", got)
	}
	r := mustQuery(t, c, Query{})
	if r.Matched != 2 || r.Records[0].RawText != "good\n" || r.Records[1].RawText != "" || !bytes.Equal(r.Records[1].RawBytes, []byte{0xff, 0, 0xfe, '\n'}) {
		t.Fatalf("memory or encoding changed: %+v", r)
	}
	merged, err := c.Append(context.Background(), []Input{{Name: "next", Data: []byte("next\n")}, {Name: "inline", Data: append([]byte("good\n"), []byte{0xff, 0, 0xfe, '\n'}...)}})
	if err != nil {
		t.Fatal(err)
	}
	if mustQuery(t, merged, Query{}).Matched != 3 || mustQuery(t, c, Query{}).Matched != 2 {
		t.Fatal("append dropped raw bytes, doubled repeats, or changed receiver")
	}
	encoded, _ := json.Marshal(r.Records[1])
	if !strings.Contains(string(encoded), `"raw_base64":"/wD+Cg=="`) {
		t.Fatal(string(encoded))
	}
}

func TestCatalogGenerationReplacementSameSizeSameMtimeRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.log")
	writeSource(t, path, []byte("old\n"))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	c := mustCatalog(t, []Input{{Path: path}, {Name: "good", Data: []byte("good\n")}}, Options{})
	replacement := path + ".replacement"
	writeSource(t, replacement, []byte("new\n"))
	if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	r := mustQuery(t, c, Query{})
	if r.Complete || r.Matched != 1 || len(r.SourceErrors) != 1 || r.Records[0].RawText != "good\n" {
		t.Fatalf("replacement accepted: %+v", r)
	}
}

func TestCatalogLimitsCancellationAndKnownBinary(t *testing.T) {
	for _, tc := range []struct {
		data []byte
		opts Options
	}{
		{[]byte("12345"), Options{MaxInputBytes: 4}},
		{gzipBytes(t, []byte("12345")), Options{MaxDecodedBytes: 4}},
		{[]byte("12345"), Options{MaxLineBytes: 4}},
		{[]byte("SQLite format 3\x00padding"), Options{}},
		{gzipBytes(t, []byte("PERFILE2payload")), Options{}},
	} {
		c, err := Prepare(context.Background(), []Input{{Name: "limit", Data: tc.data}}, tc.opts)
		if err == nil || c == nil || c.Sources()[0].Complete {
			t.Fatalf("invalid source accepted: %+v", tc)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if c, err := Prepare(ctx, []Input{{Name: "x", Data: []byte("x")}}, Options{}); c != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%+v %v", c, err)
	}
	c := mustCatalog(t, []Input{{Name: "x", Data: []byte("x")}}, Options{})
	if r, err := c.Query(ctx, Query{}); !errors.Is(err, context.Canceled) || len(r.Records) != 0 {
		t.Fatalf("cancel query=%+v %v", r, err)
	}
	if _, err := Prepare(context.Background(), []Input{{Name: "spoof\n# codrax-source: other", Data: []byte("x")}}, Options{}); err == nil {
		t.Fatal("source label injection accepted")
	}
}
