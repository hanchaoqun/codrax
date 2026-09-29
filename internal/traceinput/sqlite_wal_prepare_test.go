package traceinput

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestPrepareWALUsesCommittedRowsAndInvalidatesOnWALOnlyCommit(t *testing.T) {
	path, _ := existingSQLiteFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA wal_autocheckpoint=0", "CREATE TABLE wal_only (value TEXT)", "INSERT INTO wal_only VALUES ('committed-row')"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	material, err := Prepare(t.Context(), Options{InputPath: path, RuntimeAnchor: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := material.Validate(t.Context(), material.Preview()); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(t.Context(), material.QueryPath())
	if err != nil || len(idx.Events) == 0 {
		t.Fatalf("not query-ready: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r preparationReceipt
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	if r.Conversion == nil || r.Conversion.ExistingTraceDBSource == nil || r.Conversion.ExistingTraceDBSource.WAL == nil || r.Conversion.ExistingTraceDBSource.WAL.SnapshotSHA256 == "" {
		t.Fatal("missing composite provenance")
	}
	if _, err := db.Exec("INSERT INTO wal_only VALUES ('later-commit')"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("fixture must change WAL only, not main DB")
	}
	if err := material.Validate(t.Context(), material.Preview()); err == nil {
		t.Fatal("cached material ignored changed WAL")
	}
}

func TestPrepareCheckpointOnlyWALReceiptAndReuse(t *testing.T) {
	path, _ := existingSQLiteFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA journal_mode=WAL", "CREATE TABLE checkpointed (value TEXT)", "INSERT INTO checkpointed VALUES ('committed-row')", "PRAGMA wal_checkpoint(TRUNCATE)"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	material, err := Prepare(t.Context(), Options{InputPath: path, RuntimeAnchor: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := material.Validate(t.Context(), material.Preview()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r preparationReceipt
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	if r.Conversion == nil || r.Conversion.ExistingTraceDBSource == nil || r.Conversion.ExistingTraceDBSource.WAL == nil || !r.Conversion.ExistingTraceDBSource.WAL.CheckpointOnly {
		t.Fatal("checkpoint basis lost in serialized receipt")
	}
	wal := r.Conversion.ExistingTraceDBSource.WAL
	if wal.Bytes != 0 || wal.CommitFrame != 0 {
		t.Fatalf("invented commit: %+v", wal)
	}
	wal.CheckpointOnly = false
	source := r.Conversion.ExistingTraceDBSource
	if err := validatePreparedSQLiteReceipt("sqlite", source.Path, source.Bytes, source.SHA256, source.Generation, *r.Conversion); err == nil {
		t.Fatal("ambiguous zero-commit receipt accepted")
	}
	if _, err := db.Exec("INSERT INTO checkpointed VALUES ('new-commit')"); err != nil {
		t.Fatal(err)
	}
	if err := material.Validate(t.Context(), material.Preview()); err == nil {
		t.Fatal("empty-WAL material reused after first commit")
	}
}
