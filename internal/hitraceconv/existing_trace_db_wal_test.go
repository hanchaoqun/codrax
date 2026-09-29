package hitraceconv

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func existingWALFixture(t *testing.T) (string, *sql.DB) {
	t.Helper()
	path := existingTraceDBFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	for _, query := range []string{"PRAGMA journal_mode=WAL", "PRAGMA wal_autocheckpoint=0", "UPDATE process SET name='wal-process'", "UPDATE thread SET name='wal-thread' WHERE itid=2", "INSERT INTO sched_slice VALUES (3000000,1000000,1,'S',120,2)"} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	return path, db
}

func walSourceBytes(t *testing.T, path string) map[string][]byte {
	t.Helper()
	out := make(map[string][]byte)
	for _, suffix := range []string{"", "-wal", "-shm"} {
		body, err := os.ReadFile(path + suffix)
		if err != nil {
			t.Fatal(err)
		}
		out[suffix] = body
	}
	return out
}

func TestPrepareExistingWALCommittedReadOnlySnapshot(t *testing.T) {
	path, db := existingWALFixture(t)
	// Keep a live uncommitted transaction on the actual writer. Its future
	// process name must not leak into the prepared trace.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("UPDATE process SET name='uncommitted-process'"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("UPDATE thread SET name='uncommitted-thread' WHERE itid=2"); err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
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
	if r == nil || r.WAL == nil || r.WAL.CommitFrame < 1 || r.WAL.SnapshotSHA256 == r.SHA256 || r.WAL.SHA256 == "" {
		t.Fatalf("missing source/image distinction: %+v", r)
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
		t.Fatalf("committed WAL scheduler update lost, got %d", count)
	}
	body, _ := os.ReadFile(result.OutputPath)
	if !bytes.Contains(body, []byte("wal-thread")) || bytes.Contains(body, []byte("uncommitted-thread")) {
		t.Fatal("wrong transaction visible")
	}
	for suffix, original := range before {
		got, _ := os.ReadFile(path + suffix)
		if !bytes.Equal(original, got) {
			t.Fatalf("source %q was modified", suffix)
		}
	}
}

func TestPrepareExistingWALGenerationCancellationAndDamage(t *testing.T) {
	for _, change := range []string{"commit", "replace", "cancel", "journal", "checksum", "short-frame", "page-budget"} {
		t.Run(change, func(t *testing.T) {
			path, db := existingWALFixture(t)
			opts := existingTraceDBOptions(t, path)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if change == "checksum" || change == "short-frame" || change == "page-budget" {
				body, err := os.ReadFile(path + "-wal")
				if err != nil {
					t.Fatal(err)
				}
				switch change {
				case "checksum":
					body[70] ^= 1
				case "short-frame":
					body = body[:len(body)-1]
				case "page-budget":
					binary.BigEndian.PutUint32(body[8:12], 123)
				}
				if err := os.WriteFile(path+"-wal", body, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				fired := false
				opts.Progress = func(e ProgressEvent) {
					if fired || e.Stage != "existing_trace_db_snapshot" || e.Status != ProgressStatusComplete {
						return
					}
					fired = true
					switch change {
					case "commit":
						if _, err := db.Exec("UPDATE process SET name='new-generation'"); err != nil {
							t.Fatal(err)
						}
					case "replace":
						body, _ := os.ReadFile(path + "-wal")
						replacement := filepath.Join(t.TempDir(), "wal")
						if err := os.WriteFile(replacement, body, 0600); err != nil {
							t.Fatal(err)
						}
						if err := os.Rename(replacement, path+"-wal"); err != nil {
							t.Fatal(err)
						}
					case "cancel":
						cancel()
					case "journal":
						if err := os.WriteFile(path+"-journal", nil, 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			result, err := PrepareExistingTraceDB(ctx, opts)
			assertExistingTraceDBFailureClean(t, opts, result, err)
			if change == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel lost: %v", err)
			}
		})
	}
}

func TestExistingWALReaderSQLiteOracleAndUncommittedFrame(t *testing.T) {
	path, db := existingWALFixture(t)
	// Force cache spill to create real uncommitted frames, not just dirty
	// connection-local pages. SQLite's reader remains the independent oracle.
	for _, q := range []string{"PRAGMA cache_size=1", "BEGIN", "CREATE TABLE uncommitted (v BLOB)", "INSERT INTO uncommitted SELECT zeroblob(100000)"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	defer db.Exec("ROLLBACK")
	main, err := openConversionInputAuthority(path)
	if err != nil {
		t.Fatal(err)
	}
	defer main.Close()
	v, err := openExistingWALView(t.Context(), main)
	if err != nil {
		t.Fatal(err)
	}
	defer v.wal.Close()
	if v.receipt.CommitFrame >= (v.wal.Size()-32)/(v.pageSize+24) {
		t.Fatal("fixture did not create uncommitted WAL tail")
	}
	dir, err := newRuntimePrivateConversionDir(t.TempDir(), "wal-oracle-*")
	if err != nil {
		t.Fatal(err)
	}
	defer dir.FinalizeCleanup()
	lease, err := newExternalToolInputLeaseWithProgress(t.Context(), v, dir, "image.db", externalToolInputSnapshotOnly, nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := sealExternalToolInputSnapshot(t.Context(), lease, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer sealed.Close()
	read, err := openTraceDBFromSealed(t.Context(), sealed, path)
	if err != nil {
		t.Fatal(err)
	}
	defer read.close()
	var name, integrity string
	var count int
	if err := read.db.QueryRow("SELECT name FROM process WHERE ipid=1").Scan(&name); err != nil || name != "wal-process" {
		t.Fatalf("wrong committed content %q: %v", name, err)
	}
	if err := read.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='uncommitted'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("uncommitted schema visible: %d %v", count, err)
	}
	if err := read.db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("bad image %q %v", integrity, err)
	}
}

func TestExistingWALPageGeometryChecksumOrdersAndResetTail(t *testing.T) {
	for _, pageSize := range []int{512, 4096, 65536} {
		for _, big := range []bool{false, true} {
			t.Run(fmt.Sprintf("page-%d-big-%v", pageSize, big), func(t *testing.T) {
				path := existingTraceDBFixture(t)
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				db.SetMaxOpenConns(1)
				for _, q := range []string{fmt.Sprintf("PRAGMA page_size=%d", pageSize), "VACUUM", "PRAGMA journal_mode=WAL", "PRAGMA wal_autocheckpoint=0", "UPDATE thread SET name='wal-thread' WHERE itid=2"} {
					if _, err := db.Exec(q); err != nil {
						t.Fatal(err)
					}
				}
				body, err := os.ReadFile(path + "-wal")
				if err != nil {
					t.Fatal(err)
				}
				var order binary.ByteOrder = binary.LittleEndian
				magic := uint32(0x377f0682)
				if big {
					order = binary.BigEndian
					magic = 0x377f0683
				}
				binary.BigEndian.PutUint32(body[:4], magic)
				s0, s1 := existingWALChecksum(order, body[:24], 0, 0)
				binary.BigEndian.PutUint32(body[24:28], s0)
				binary.BigEndian.PutUint32(body[28:32], s1)
				for off := 32; off < len(body); off += pageSize + 24 {
					f := body[off : off+pageSize+24]
					s0, s1 = existingWALChecksum(order, f[:8], s0, s1)
					s0, s1 = existingWALChecksum(order, f[24:], s0, s1)
					binary.BigEndian.PutUint32(f[16:20], s0)
					binary.BigEndian.PutUint32(f[20:24], s1)
				}
				// A complete old-salt tail is legal after reset and must not be
				// read as the current transaction, even if it claims a huge size.
				old := append([]byte{}, body[len(body)-pageSize-24:]...)
				old[8] ^= 1
				binary.BigEndian.PutUint32(old[4:8], 0xfffffffe)
				body = append(body, old...)
				if err := os.WriteFile(path+"-wal", body, 0600); err != nil {
					t.Fatal(err)
				}
				opts := existingTraceDBOptions(t, path)
				res, err := PrepareExistingTraceDB(t.Context(), opts)
				if err != nil {
					t.Fatal(err)
				}
				out, err := os.ReadFile(res.OutputPath)
				if err != nil || !bytes.Contains(out, []byte("wal-thread")) {
					t.Fatalf("committed row lost: %v", err)
				}
			})
		}
	}
}
