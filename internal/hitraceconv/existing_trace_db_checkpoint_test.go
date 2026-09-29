package hitraceconv

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func checkpointWALCapture(t *testing.T, kind string) string {
	t.Helper()
	path, db := existingWALFixture(t)
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	if kind != "empty" {
		for _, q := range []string{"PRAGMA cache_size=1", "BEGIN", "CREATE TABLE uncommitted (v BLOB)", "INSERT INTO uncommitted SELECT zeroblob(100000)"} {
			if _, err := db.Exec(q); err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() { _, _ = db.Exec("ROLLBACK") })
	}
	// The normal SQLite reader is an independent visibility oracle while the
	// writer holds its uncommitted transaction. It must still see the checkpoint.
	oracle, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer oracle.Close()
	var name string
	if err := oracle.QueryRow("SELECT name FROM process WHERE ipid=1").Scan(&name); err != nil || name != "wal-process" {
		t.Fatalf("oracle=%q %v", name, err)
	}
	var pending int
	if err := oracle.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='uncommitted'").Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("oracle sees pending data: %d %v", pending, err)
	}
	inputs := walSourceBytes(t, path)
	if kind != "empty" && len(inputs["-wal"]) <= 32 {
		t.Fatal("no real uncommitted spill")
	}
	if kind == "header" {
		inputs["-wal"] = inputs["-wal"][:32]
	}
	capture := filepath.Join(t.TempDir(), "capture.db")
	for suffix, data := range inputs {
		if err := os.WriteFile(capture+suffix, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return capture
}

func TestPrepareCheckpointOnlyWALPublicQueryAndReuse(t *testing.T) {
	for _, kind := range []string{"empty", "header", "uncommitted"} {
		t.Run(kind, func(t *testing.T) {
			path := checkpointWALCapture(t, kind)
			before := walSourceBytes(t, path)
			for suffix := range before {
				if err := os.Chmod(path+suffix, 0400); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chmod(filepath.Dir(path), 0500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = os.Chmod(filepath.Dir(path), 0700)
				for suffix := range before {
					_ = os.Chmod(path+suffix, 0600)
				}
			})
			result, err := PrepareExistingTraceDB(t.Context(), existingTraceDBOptions(t, path))
			if err != nil {
				t.Fatal(err)
			}
			r := result.ExistingTraceDBSource
			if r == nil || r.WAL == nil || !r.WAL.CheckpointOnly || r.WAL.CommitFrame != 0 || r.WAL.SnapshotBytes != r.Bytes || r.WAL.SHA256 == "" || r.WAL.SnapshotSHA256 == "" {
				t.Fatalf("missing checkpoint provenance: %+v", r)
			}
			if err := ValidateExistingTraceDBReceipt(t.Context(), path, r); err != nil {
				t.Fatal(err)
			}
			idx, err := tracequery.BuildIndex(t.Context(), result.OutputPath)
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
				t.Fatalf("checkpoint data lost: %d", count)
			}
			body, err := os.ReadFile(result.OutputPath)
			if err != nil || !bytes.Contains(body, []byte("wal-thread")) || bytes.Contains(body, []byte("uncommitted")) {
				t.Fatalf("wrong committed visibility: %v", err)
			}
			for suffix, original := range before {
				got, err := os.ReadFile(path + suffix)
				if err != nil || !bytes.Equal(original, got) {
					t.Fatalf("source %s changed: %v", suffix, err)
				}
			}
			_ = os.Chmod(path+"-wal", 0600)
			if err := os.WriteFile(path+"-wal", append(before["-wal"], 0), 0600); err != nil {
				t.Fatal(err)
			}
			if err := ValidateExistingTraceDBReceipt(t.Context(), path, r); err == nil {
				t.Fatal("WAL-only change reused checkpoint material")
			}
		})
	}
}

func TestCheckpointWALRejectsDamageAndPublicationChanges(t *testing.T) {
	for _, change := range []string{"bad-main-count", "bad-counter", "short-header", "checksum", "cancel", "append"} {
		t.Run(change, func(t *testing.T) {
			path := checkpointWALCapture(t, "header")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch change {
			case "bad-main-count", "bad-counter":
				b, _ := os.ReadFile(path)
				if change == "bad-main-count" {
					binary.BigEndian.PutUint32(b[28:32], 1)
				} else {
					b[92] ^= 1
				}
				if err := os.WriteFile(path, b, 0600); err != nil {
					t.Fatal(err)
				}
			case "short-header":
				if err := os.Truncate(path+"-wal", 16); err != nil {
					t.Fatal(err)
				}
			case "checksum":
				b, _ := os.ReadFile(path + "-wal")
				b[24] ^= 1
				if err := os.WriteFile(path+"-wal", b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			opts := existingTraceDBOptions(t, path)
			opts.Progress = func(p ProgressEvent) {
				if p.Stage == "existing_trace_db_snapshot" && p.Status == ProgressStatusComplete {
					if change == "cancel" {
						cancel()
					}
					if change == "append" {
						if err := os.WriteFile(path+"-wal", nil, 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			result, err := PrepareExistingTraceDB(ctx, opts)
			assertExistingTraceDBFailureClean(t, opts, result, err)
		})
	}
}
