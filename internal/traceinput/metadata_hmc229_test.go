package traceinput

import (
	"bytes"
	"database/sql"
	"os"
	"testing"
)

func TestHMC229PreparedMetadataRejectsWALOnlyCommit(t *testing.T) {
	path, _ := existingSQLiteFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, query := range []string{"PRAGMA journal_mode=WAL", "PRAGMA wal_autocheckpoint=0", "CREATE TABLE device_info(physical_width,physical_height,physical_frame_rate)", "INSERT INTO device_info VALUES(1440,2560,120)"} {
		if _, err := db.Exec(query); err != nil {
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
	fields, err := material.SourceMetadata(t.Context())
	if err != nil || len(fields) < 3 || fields[0].Value != "1440" {
		t.Fatal(fields, err)
	}
	if _, err := db.Exec("UPDATE device_info SET physical_width=1080"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("fixture changed main DB", err)
	}
	if _, err := material.SourceMetadata(t.Context()); err == nil {
		t.Fatal("WAL-only replacement reused metadata")
	}
}
