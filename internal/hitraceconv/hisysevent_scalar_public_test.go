package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestHiSysObservationStrictScalarsPreserveValidSiblings(t *testing.T) {
	for _, binaryProvider := range []bool{false, true} {
		name := "existing_sqlite"
		if binaryProvider {
			name = "binary_provider"
		}
		t.Run(name, func(t *testing.T) { testHiSysStrictScalars(t, binaryProvider) })
	}
}

func testHiSysStrictScalars(t *testing.T, binaryProvider bool) {
	statements := append(traceDBSyncSpanIntegrationBaseStatements(),
		"INSERT INTO data_dict VALUES (81, 'SYS'), (82, 'EVENT')",
		// No affinity: exercise the actual stored SQLite classes.
		"CREATE TABLE hisys_all_event (ts, tid, domain_id, event_name_id, contents)",
		"INSERT INTO hisys_all_event VALUES (0, 0, 81, 82, 'zero'), (1, NULL, 81, 82, 'null'), (9007199254740993, 100, 81, 82, 'exact')",
		"INSERT INTO hisys_all_event VALUES (2, '100', 81, 82, 'text'), (3, CAST(100 AS REAL), 81, 82, 'real'), (4, X'313030', 81, 82, 'blob'), (5, -1, 81, 82, 'negative'), (6, 2147483648, 81, 82, 'range')",
		"INSERT INTO hisys_all_event VALUES (NULL, 100, 81, 82, 'bad'), ('7', 100, 81, 82, 'bad'), (CAST(8 AS REAL), 100, 81, 82, 'bad'), (X'39', 100, 81, 82, 'bad'), (-1, 100, 81, 82, 'bad')")
	source := createTraceDBFixture(t, statements)
	before, _ := os.ReadFile(source)
	dir := t.TempDir()
	opts := Options{InputPath: source, OutputPath: filepath.Join(dir, "out.systrace")}
	var r Result
	var err error
	if binaryProvider {
		opts.InputPath = filepath.Join(dir, "capture.htrace")
		if err := os.WriteFile(opts.InputPath, []byte("modern profiler payload"), 0o600); err != nil {
			t.Fatal(err)
		}
		opts.TraceEngine, opts.TraceStreamerPath = traceEngineTraceStreamer, writeFakeTraceStreamer(t, dir, 0)
		t.Setenv("TRACE_STREAMER_FIXTURE_DB", source)
		r, err = ConvertFile(context.Background(), opts)
	} else {
		r, err = PrepareExistingTraceDB(context.Background(), opts)
	}
	if err != nil {
		t.Fatalf("one bad scalar must not abort valid siblings: %v", err)
	}
	body, err := os.ReadFile(r.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	var rows []tracewire.HiSysEvent
	for _, line := range strings.Split(string(body), "\n") {
		if row, ok := tracewire.ParseHiSysEventObservation(line); ok {
			rows = append(rows, row)
		}
	}
	if len(rows) != 8 || rows[7].TimestampNS != 9007199254740993 || rows[0].SourceTID == nil || *rows[0].SourceTID != 0 || rows[1].SourceTID != nil {
		t.Fatalf("valid zero/NULL/exact integer lost: %+v", rows)
	}
	for i := 2; i <= 6; i++ {
		if rows[i].SourceTID != nil || rows[i].SourceTIDRaw == nil {
			t.Fatalf("invalid TID coerced into authority: %+v", rows[i])
		}
	}
	c := coverageForTable(r.TraceDBCoverage, "hisys_all_event")
	if c == nil || c.RowsRead != 13 || c.RowsEmitted != 8 || !strings.Contains(c.Skipped, "invalid_timestamp=5") || !strings.Contains(c.Skipped, "invalid_source_tid=5") {
		t.Fatalf("incomplete scalar accounting: %+v", c)
	}
	after, _ := os.ReadFile(source)
	if string(before) != string(after) {
		t.Fatal("source database changed")
	}
}
