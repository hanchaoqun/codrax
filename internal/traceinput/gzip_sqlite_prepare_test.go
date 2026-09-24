package traceinput

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func gzipSQLiteFixture(t *testing.T) (string, []byte, []byte) {
	t.Helper()
	_, decoded := existingSQLiteFixture(t)
	compressed := gzipPreparationBytes(t, decoded)
	path := filepath.Join(t.TempDir(), "compressed 数据.capture")
	writeTestFile(t, path, compressed)
	return path, compressed, decoded
}

func TestPrepareGzipSQLitePublicCompleteMaterialAndDualReceipts(t *testing.T) {
	path, compressed, decoded := gzipSQLiteFixture(t)
	if err := os.Chmod(filepath.Dir(path), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(path), 0o700) })
	anchor := t.TempDir()
	t.Setenv("CODRAX_TRACE_STREAMER", filepath.Join(anchor, "converter-must-not-run"))
	material, err := Prepare(t.Context(), Options{InputPath: path, RuntimeAnchor: anchor, PreviewBytes: 640})
	if err != nil {
		t.Fatal(err)
	}
	if material.SourcePath() != path || material.QueryPath() == path || material.SelfContainedText() || len(material.Preview()) > 640 || strings.Contains(material.Preview(), "tail-business-marker") {
		t.Fatalf("compressed source, full query and preview confused: %v", material)
	}
	idx, err := tracequery.BuildIndex(t.Context(), material.QueryPath())
	if err != nil {
		t.Fatal(err)
	}
	zero := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "zero-time-marker", Limit: 10})
	if len(zero.Events) != 1 || zero.Events[0].Ts != 0 {
		t.Fatalf("zero timestamp lost: %+v", zero.Events)
	}
	tail := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "tail-business-marker", TimeStart: 2.98, TimeEnd: 3, Limit: 10})
	if len(tail.Events) != 1 || tail.TimeStart != 2.98 || tail.TimeEnd != 3 {
		t.Fatalf("full-file tail or explicit scope lost: %+v", tail)
	}
	for _, row := range idx.Events {
		if row.Type == "sched_switch" || row.Type == "sched_wakeup" {
			t.Fatalf("database inventory invented scheduling authority: %+v", row)
		}
	}
	body, err := os.ReadFile(filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt preparationReceipt
	if err := json.Unmarshal(body, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.SourceKind != "gzip" || receipt.Conversion == nil || receipt.Transport != nil {
		t.Fatalf("not an exclusive gzip semantic preparation: %+v", receipt)
	}
	gzip, db := receipt.Conversion.GzipInputProvenance, receipt.Conversion.ExistingTraceDBSource
	outerSHA, innerSHA := sha256.Sum256(compressed), sha256.Sum256(decoded)
	if gzip == nil || db == nil || gzip.DecodedFormat != "sqlite" || db.Path != path ||
		receipt.SourceSHA256 != hex.EncodeToString(outerSHA[:]) || gzip.SourceSHA256 != receipt.SourceSHA256 ||
		gzip.SourceBytes != int64(len(compressed)) || gzip.SourceGeneration != receipt.SourceGeneration ||
		gzip.DecodedSHA256 != hex.EncodeToString(innerSHA[:]) || db.SHA256 != gzip.DecodedSHA256 ||
		db.Bytes != int64(len(decoded)) || db.Bytes != gzip.DecodedBytes || db.Generation == "" || db.Generation != gzip.DecodedGeneration {
		t.Fatalf("compressed and database receipt scales not joined: %+v %+v", gzip, db)
	}
	if err := material.Validate(t.Context(), material.Preview()); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, compressed) {
		t.Fatalf("modified compressed original: %v", err)
	}
	if entries, err := os.ReadDir(filepath.Dir(path)); err != nil || len(entries) != 1 {
		t.Fatalf("created sidecars next to compressed original: %v %v", entries, err)
	}
}

func TestGzipSQLiteSourceReplacementInvalidatesCommitAndWarmMaterial(t *testing.T) {
	for _, phase := range []string{"pending", "warm"} {
		t.Run(phase, func(t *testing.T) {
			path, original, _ := gzipSQLiteFixture(t)
			anchor := t.TempDir()
			options := Options{InputPath: path, RuntimeAnchor: anchor}
			pending, err := Begin(t.Context(), options)
			if err != nil {
				t.Fatal(err)
			}
			defer pending.Discard()
			if phase == "warm" {
				material, err := pending.Commit(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				replaceGzipSQLiteSameBytes(t, path, original)
				if err := material.Validate(t.Context(), material.Preview()); err == nil {
					t.Fatal("same-byte replacement retained compressed input authority")
				}
			} else {
				replaceGzipSQLiteSameBytes(t, path, original)
				if material, err := pending.Commit(t.Context()); err == nil || material != nil {
					t.Fatalf("replaced input committed: %v %v", material, err)
				}
				if dirs, _ := filepath.Glob(filepath.Join(anchor, "trace-input-*")); len(dirs) != 0 {
					t.Fatalf("failed transaction retained output: %v", dirs)
				}
			}
		})
	}
}

func replaceGzipSQLiteSameBytes(t *testing.T, path string, original []byte) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := path + ".replacement"
	writeTestFile(t, replacement, original)
	if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
}
