//go:build unix

package hitraceconv

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestOpenTraceDBFromSealedReconnectNeverReadsReplacementPath(t *testing.T) {
	dir, sealed, path := newSealedTraceDBTestFixture(t, "generation-A")
	defer finishSealedTraceDBTestFixture(t, dir, sealed)
	replacementSource := createTraceDBFixture(t, []string{
		"CREATE TABLE sealed_fixture (value TEXT NOT NULL)",
		"INSERT INTO sealed_fixture VALUES ('generation-B')",
	})
	tdb, err := openTraceDBFromSealed(context.Background(), sealed, path)
	if err != nil {
		t.Fatal(err)
	}
	tdb.db.SetMaxIdleConns(0)
	assertSealedTraceDBValue(t, tdb, "generation-A")
	if err := os.Rename(path, path+".held"); err != nil {
		tdb.close()
		t.Fatal(err)
	}
	copyTestFile(t, replacementSource, path)
	assertSealedTraceDBValue(t, tdb, "generation-A")
	if err := tdb.close(); err != nil {
		t.Fatal(err)
	}
	if err := sealed.Validate(); !errors.Is(err, errSealedConversionFileIdentityChanged) {
		t.Fatalf("replacement path escaped sealed exit gate: %v", err)
	}
}

func assertSealedTraceDBValue(t *testing.T, tdb *traceDB, want string) {
	t.Helper()
	var got string
	if err := tdb.db.QueryRow("SELECT value FROM sealed_fixture").Scan(&got); err != nil || got != want {
		t.Fatalf("sealed reconnect value=%q want=%q err=%v", got, want, err)
	}
}
