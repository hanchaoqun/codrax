package traceinput

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

func TestGzipSQLitePreparationRejectsUnboundDatabaseReceipt(t *testing.T) {
	for _, corruption := range []string{"missing", "path", "bytes", "sha", "generation", "outer-scale", "wrong-family", "outer-sha"} {
		t.Run(corruption, func(t *testing.T) {
			path, original, _ := gzipSQLiteFixture(t)
			anchor := t.TempDir()
			prior := filepath.Join(anchor, "prior.systrace")
			writeTestFile(t, prior, []byte("prior publication\n"))
			calls := 0
			material, err := prepare(t.Context(), Options{InputPath: path, RuntimeAnchor: anchor}, func(ctx context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
				calls++
				result, err := hitraceconv.PrepareFile(ctx, opts)
				if err != nil {
					t.Fatalf("real preparation failed before receipt corruption: %v", err)
				}
				db, gzip := result.ExistingTraceDBSource, result.GzipInputProvenance
				if db == nil || gzip == nil || gzip.DecodedFormat != "sqlite" {
					t.Fatal("real SQLite consumption not exercised")
				}
				switch corruption {
				case "missing":
					result.ExistingTraceDBSource = nil
				case "path":
					db.Path += ".different"
				case "bytes":
					db.Bytes++
				case "sha":
					db.SHA256 = strings.Repeat("a", 64)
				case "generation":
					db.Generation += "-changed"
				case "outer-scale":
					db.Bytes, db.SHA256, db.Generation = gzip.SourceBytes, gzip.SourceSHA256, gzip.SourceGeneration
				case "wrong-family":
					gzip.DecodedFormat = "linux_perf_data"
				case "outer-sha":
					gzip.SourceSHA256 = strings.Repeat("b", 64)
				}
				return result, nil
			})
			if material != nil || err == nil || !strings.Contains(err.Error(), "receipt") || calls != 1 {
				t.Fatalf("unbound consumption escaped: material=%v calls=%d err=%v", material, calls, err)
			}
			entries, readErr := os.ReadDir(anchor)
			if readErr != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(prior) {
				t.Fatalf("failed receipt retained outputs/staging: %v %v", entries, readErr)
			}
			if got, err := os.ReadFile(prior); err != nil || string(got) != "prior publication\n" {
				t.Fatalf("cleanup changed preexisting output: %v", err)
			}
			if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("receipt rejection changed original: %v", err)
			}
		})
	}
}
