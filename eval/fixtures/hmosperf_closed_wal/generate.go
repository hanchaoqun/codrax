//go:build ignore

// Run from the repository root: go run eval/fixtures/hmosperf_closed_wal/generate.go
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	out := "eval/fixtures/hmosperf_closed_wal/capture.data"
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		panic("refusing to overwrite fixture")
	}
	work, err := os.MkdirTemp("", "codrax-closed-wal-fixture-")
	must(err)
	defer os.RemoveAll(work)
	source := filepath.Join(work, "capture.db")
	db, err := sql.Open("sqlite", source)
	must(err)
	db.SetMaxOpenConns(1)
	base, err := os.ReadFile("eval/fixtures/hmosperf_referenced_dictionary/capture.sql")
	must(err)
	for _, query := range []string{string(base), "PRAGMA journal_mode=WAL",
		"UPDATE app_startup SET end_time=1048000000 WHERE start_time=1040000000",
		"INSERT INTO data_dict VALUES (301, 'RestoreTabs')",
		"INSERT INTO app_startup VALUES (1064000000, 1072000000, 301, 1)"} {
		_, err := db.Exec(query)
		must(err)
	}
	must(db.Close())
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(source + suffix); !os.IsNotExist(err) {
			panic("fixture still has auxiliaries")
		}
	}
	body, err := os.ReadFile(source)
	must(err)
	if body[18] != 2 || body[19] != 2 {
		panic("fixture lost WAL mode")
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	must(err)
	_, err = f.Write(body)
	must(err)
	must(f.Close())
	fmt.Printf("normal close: %d-byte WAL-mode main, no auxiliaries\n", len(body))
}
