package traceinput

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMissingWALPrepareReceiptCommitAndCacheBoundary(t *testing.T) {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		t.Run(suffix, func(t *testing.T) {
			path, _ := existingSQLiteFixture(t)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec("PRAGMA journal_mode=WAL"); err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			coordinator := NewCoordinator(Options{RuntimeAnchor: t.TempDir()})
			material, err := coordinator.Prepare(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			if err = material.Validate(t.Context(), material.Preview()); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json"))
			if err != nil {
				t.Fatal(err)
			}
			var r preparationReceipt
			if err = json.Unmarshal(raw, &r); err != nil {
				t.Fatal(err)
			}
			w := r.Conversion.ExistingTraceDBSource.WAL
			if w == nil || !w.Absent || !validSQLiteWALReceipt(w) {
				t.Fatal("absent receipt lost")
			}
			bad := *w
			bad.Absent = false
			if validSQLiteWALReceipt(&bad) {
				t.Fatal("absence treated as zero-byte file")
			}
			bad = *w
			bad.SHA256 = string(make([]byte, 64))
			if validSQLiteWALReceipt(&bad) {
				t.Fatal("absence given file identity")
			}
			pending, err := Begin(t.Context(), Options{InputPath: path, RuntimeAnchor: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			defer pending.Discard()
			writeTestFile(t, path+suffix, []byte("new source state"))
			if got, err := pending.Commit(t.Context()); err == nil || got != nil {
				t.Fatal("appearance passed publication")
			}
			if err := material.Validate(t.Context(), material.Preview()); err == nil {
				t.Fatal("appearance passed reuse")
			}
			if got, err := coordinator.Prepare(t.Context(), path); err == nil || got != nil {
				t.Fatal("coordinator reused absent-log authority")
			}
		})
	}
}
