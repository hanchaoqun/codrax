package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSealedTraceDBFSExactNameIndependentCursorsAndFrozenStat(t *testing.T) {
	dir, sealed, _ := newSealedTraceDBTestFixture(t, "alpha")
	defer finishSealedTraceDBTestFixture(t, dir, sealed)
	filesystem, err := newSealedTraceDBFS(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := filesystem.Open("other.db"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("exact-one-file FS accepted other name: %v", err)
	}
	first, err := filesystem.Open(sealedTraceDBVirtualName)
	if err != nil {
		t.Fatal(err)
	}
	second, err := filesystem.Open(sealedTraceDBVirtualName)
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	defer first.Close()
	defer second.Close()

	firstSeeker, ok := first.(io.Seeker)
	if !ok {
		t.Fatal("sealed VFS file does not implement io.Seeker")
	}
	if _, err := firstSeeker.Seek(7, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	secondHeader := make([]byte, 16)
	if _, err := io.ReadFull(second, secondHeader); err != nil {
		t.Fatal(err)
	}
	if string(secondHeader) != "SQLite format 3\x00" {
		t.Fatalf("second cursor inherited first seek: %q", secondHeader)
	}
	info, err := second.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Name() != sealedTraceDBVirtualName || info.Size() != sealed.Size() || info.IsDir() || info.Mode().Perm() != 0o400 {
		t.Fatalf("sealed VFS stat drifted: name=%q size=%d mode=%v dir=%t", info.Name(), info.Size(), info.Mode(), info.IsDir())
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Read(make([]byte, 1)); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("closed VFS cursor remained readable: %v", err)
	}
	check := make([]byte, 6)
	if _, err := second.Read(check); err != nil {
		t.Fatalf("closing first cursor affected second: %v", err)
	}
}

func TestOpenTraceDBFromSealedReadOnlyLifecycle(t *testing.T) {
	dir, sealed, displayPath := newSealedTraceDBTestFixture(t, "alpha")
	defer finishSealedTraceDBTestFixture(t, dir, sealed)
	dsn := sqliteSealedReadOnlyDSN("vfs123")
	if !strings.Contains(dsn, "mode=ro") || !strings.Contains(dsn, "vfs=vfs123") || strings.Contains(dsn, "immutable") {
		t.Fatalf("sealed read-only DSN drifted: %q", dsn)
	}
	parsedDSN, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	pragmas := parsedDSN.Query()["_pragma"]
	if !containsString(pragmas, "temp_store(MEMORY)") || !containsString(pragmas, "query_only(1)") {
		t.Fatalf("sealed VFS connection pragmas drifted: dsn=%q pragmas=%v", dsn, pragmas)
	}
	tdb, err := openTraceDBFromSealed(context.Background(), sealed, displayPath)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	if err := tdb.db.QueryRow("SELECT value FROM sealed_fixture").Scan(&got); err != nil || got != "alpha" {
		t.Fatalf("sealed query got=%q err=%v", got, err)
	}
	tdb.db.SetMaxIdleConns(0)
	for attempt := 0; attempt < 2; attempt++ {
		var tempStore, queryOnly int
		if err := tdb.db.QueryRow("PRAGMA temp_store").Scan(&tempStore); err != nil {
			t.Fatalf("query temp_store on replacement connection %d: %v", attempt, err)
		}
		if err := tdb.db.QueryRow("PRAGMA query_only").Scan(&queryOnly); err != nil {
			t.Fatalf("query query_only on replacement connection %d: %v", attempt, err)
		}
		if tempStore != 2 || queryOnly != 1 {
			t.Fatalf("replacement connection %d lost sealed pragmas: temp_store=%d query_only=%d", attempt, tempStore, queryOnly)
		}
	}
	var largest int
	if err := tdb.db.QueryRow(`WITH RECURSIVE seq(value) AS (
		SELECT 1 UNION ALL SELECT value + 1 FROM seq WHERE value < 20000
	) SELECT value FROM seq ORDER BY printf('%08d', value) DESC LIMIT 1`).Scan(&largest); err != nil || largest != 20000 {
		t.Fatalf("sealed VFS in-memory sorter failed: largest=%d err=%v", largest, err)
	}
	if _, err := tdb.db.Exec("UPDATE sealed_fixture SET value='mutated'"); err == nil {
		t.Fatal("sealed VFS unexpectedly allowed SQLite write")
	}
	if err := tdb.close(); err != nil {
		t.Fatal(err)
	}
	if err := tdb.close(); err != nil {
		t.Fatalf("trace DB close is not idempotent: %v", err)
	}
}

func TestOpenTraceDBFromSealedRejectsMissingAuthorityAsLifecycleFailure(t *testing.T) {
	if _, err := openTraceDBFromSealed(context.Background(), nil, "missing.db"); err == nil || !errors.Is(err, errSealedTraceDBAuthority) {
		t.Fatalf("missing sealed authority was not typed hard: %T %v", err, err)
	}
}

func TestExportTraceDBSealedAndPathWrappersRemainByteIdentical(t *testing.T) {
	dir := t.TempDir()
	source := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	privateDir, err := newPrivateConversionDir(dir, "codrax-sealed-export-*")
	if err != nil {
		t.Fatal(err)
	}
	dbPath, err := privateDir.ChildPath(sealedTraceDBVirtualName)
	if err != nil {
		t.Fatal(err)
	}
	copyTestFile(t, source, dbPath)
	pathOutput := filepath.Join(dir, "path.systrace")
	pathLedger, err := newConversionFileLedger(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	pathResult, err := exportTraceDBToSystraceWithLedger(context.Background(), dbPath, pathOutput, pathLedger)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := privateDir.AdoptRegularChild(sealedTraceDBVirtualName, true)
	if err != nil {
		t.Fatal(err)
	}
	sealedOutput := filepath.Join(dir, "sealed.systrace")
	sealedLedger, err := newConversionFileLedger(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	sealedResult, err := exportTraceDBToSystraceFromSealedWithLedger(context.Background(), sealed, dbPath, sealedOutput, sealedLedger)
	if err != nil {
		sealed.Close()
		t.Fatal(err)
	}
	if err := finishSealedConversionFile(sealed, nil); err != nil {
		t.Fatal(err)
	}
	if err := privateDir.FinalizeCleanup(); err != nil {
		t.Fatal(err)
	}
	pathBody, err := os.ReadFile(pathOutput)
	if err != nil {
		t.Fatal(err)
	}
	sealedBody, err := os.ReadFile(sealedOutput)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pathBody, sealedBody) || pathResult.EventsWritten != sealedResult.EventsWritten || pathResult.OutputBytes != sealedResult.OutputBytes {
		t.Fatalf("sealed/path export parity drifted: path=%+v sealed=%+v equal=%t", pathResult, sealedResult, bytes.Equal(pathBody, sealedBody))
	}
}

func TestOpenTraceDBFromSealedConcurrentVFSRegistrationAndClose(t *testing.T) {
	const workers = 8
	type fixture struct {
		dir     *privateConversionDir
		sealed  *sealedConversionFile
		display string
		value   string
	}
	fixtures := make([]fixture, 0, workers)
	for index := 0; index < workers; index++ {
		value := fmt.Sprintf("worker-%d", index)
		dir, sealed, display := newSealedTraceDBTestFixture(t, value)
		fixtures = append(fixtures, fixture{dir: dir, sealed: sealed, display: display, value: value})
	}
	var wait sync.WaitGroup
	errorsFound := make(chan error, workers)
	for _, item := range fixtures {
		item := item
		wait.Add(1)
		go func() {
			defer wait.Done()
			tdb, err := openTraceDBFromSealed(context.Background(), item.sealed, item.display)
			if err != nil {
				errorsFound <- err
				return
			}
			var got string
			queryErr := tdb.db.QueryRow("SELECT value FROM sealed_fixture").Scan(&got)
			closeErr := tdb.close()
			if queryErr != nil || got != item.value || closeErr != nil {
				errorsFound <- fmt.Errorf("value=%q want=%q query=%v close=%v", got, item.value, queryErr, closeErr)
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Errorf("concurrent sealed VFS lifecycle: %v", err)
	}
	for _, item := range fixtures {
		finishSealedTraceDBTestFixture(t, item.dir, item.sealed)
	}
}

func newSealedTraceDBTestFixture(t *testing.T, value string) (*privateConversionDir, *sealedConversionFile, string) {
	t.Helper()
	dir, err := newPrivateConversionDir(t.TempDir(), "codrax-sealed-vfs-*")
	if err != nil {
		t.Fatal(err)
	}
	path, err := dir.ChildPath(sealedTraceDBVirtualName)
	if err != nil {
		dir.FinalizeCleanup()
		t.Fatal(err)
	}
	source := createTraceDBFixture(t, []string{
		"CREATE TABLE sealed_fixture (value TEXT NOT NULL)",
		"INSERT INTO sealed_fixture VALUES ('" + value + "')",
	})
	copyTestFile(t, source, path)
	sealed, err := dir.AdoptRegularChild(sealedTraceDBVirtualName, true)
	if err != nil {
		dir.FinalizeCleanup()
		t.Fatal(err)
	}
	return dir, sealed, path
}

func finishSealedTraceDBTestFixture(t *testing.T, dir *privateConversionDir, sealed *sealedConversionFile) {
	t.Helper()
	if err := sealed.Close(); err != nil {
		t.Errorf("close sealed DB fixture: %v", err)
	}
	if err := dir.FinalizeCleanup(); err != nil {
		t.Errorf("cleanup sealed DB fixture: %v", err)
	}
}

func copyTestFile(t *testing.T, source, destination string) {
	t.Helper()
	body, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
