package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func closedWALCapture(t *testing.T) string {
	t.Helper()
	path, db := existingWALFixture(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(path + suffix); !os.IsNotExist(err) {
			t.Fatalf("not a closed capture: %s %v", suffix, err)
		}
	}
	return path
}

func TestPrepareMissingWALCheckpointReadOnlyAndReuse(t *testing.T) {
	path := closedWALCapture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if before[18] != 2 || before[19] != 2 {
		t.Fatal("fixture lost WAL header")
	}
	if err := os.Chmod(path, 0400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(path), 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(path), 0700); _ = os.Chmod(path, 0600) })
	r, err := PrepareExistingTraceDB(t.Context(), existingTraceDBOptions(t, path))
	if err != nil {
		t.Fatal(err)
	}
	if r.ExistingTraceDBSource == nil || r.ExistingTraceDBSource.WAL == nil || !r.ExistingTraceDBSource.WAL.CheckpointOnly {
		t.Fatal("missing checkpoint receipt")
	}
	if !strings.Contains(strings.Join(r.Caveats, " "), "historical completeness") {
		t.Fatal("missing-log history presented as complete")
	}
	if err := ValidateExistingTraceDBReceipt(t.Context(), path, r.ExistingTraceDBSource); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(t.Context(), r.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, e := range idx.Events {
		if e.Type == tracequery.EventSchedSwitch {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("checkpoint events=%d", count)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatal("source sidecars created", err)
	}
	_ = os.Chmod(filepath.Dir(path), 0700)
	if err := os.WriteFile(path+"-wal", nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExistingTraceDBReceipt(t.Context(), path, r.ExistingTraceDBSource); err == nil {
		t.Fatal("new WAL did not invalidate absent-WAL receipt")
	}
}

func TestMissingWALRejectsAuxiliaryAppearanceAndCancellation(t *testing.T) {
	for _, suffix := range []string{"-wal", "-shm", "-journal", "cancel", "replace-main", "dangling-wal", "bad-count"} {
		t.Run(suffix, func(t *testing.T) {
			path := closedWALCapture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			opts := existingTraceDBOptions(t, path)
			if suffix == "dangling-wal" {
				if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), path+"-wal"); err != nil {
					t.Fatal(err)
				}
			} else if suffix == "bad-count" {
				b, _ := os.ReadFile(path)
				b[28] ^= 1
				if err := os.WriteFile(path, b, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				opts.Progress = func(p ProgressEvent) {
					if p.Stage == "existing_trace_db_snapshot" && p.Status == ProgressStatusComplete {
						if suffix == "cancel" {
							cancel()
						} else if suffix == "replace-main" {
							body, err := os.ReadFile(path)
							if err != nil {
								t.Fatal(err)
							}
							if err := os.Rename(path, path+".old"); err != nil {
								t.Fatal(err)
							}
							if err := os.WriteFile(path, body, 0600); err != nil {
								t.Fatal(err)
							}
						} else if err := os.WriteFile(path+suffix, nil, 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			r, err := PrepareExistingTraceDB(ctx, opts)
			assertExistingTraceDBFailureClean(t, opts, r, err)
		})
	}
}

func TestMissingWALAliasNamespaceStaysBound(t *testing.T) {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		t.Run(suffix, func(t *testing.T) {
			path := closedWALCapture(t)
			alias := filepath.Join(t.TempDir(), "capture.db")
			if err := os.Symlink(path, alias); err != nil {
				t.Fatal(err)
			}
			r, err := PrepareExistingTraceDB(t.Context(), existingTraceDBOptions(t, alias))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(alias+suffix, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if err := ValidateExistingTraceDBReceipt(t.Context(), alias, r.ExistingTraceDBSource); err == nil {
				t.Fatal("requested alias namespace was not revalidated")
			}
		})
	}
}
