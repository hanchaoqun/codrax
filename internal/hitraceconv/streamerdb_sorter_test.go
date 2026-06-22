package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceDBRowSinkWritesInMemoryRowsSorted(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []renderedRow{
		{tsNS: 3000, seq: 3, line: "third"},
		{tsNS: 1000, seq: 1, line: "first"},
		{tsNS: 1000, seq: 2, line: "second"},
	} {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &out)
	if err != nil {
		t.Fatalf("write rows: %v", err)
	}
	if stats.SpillChunks != 0 || stats.RowsWritten != 3 || stats.PeakBufferedRows != 3 {
		t.Fatalf("unexpected in-memory stats: %+v", stats)
	}
	body := out.String()
	if !strings.Contains(body, systraceHeader) || strings.Index(body, "first") > strings.Index(body, "second") || strings.Index(body, "second") > strings.Index(body, "third") {
		t.Fatalf("rows not sorted with header:\n%s", body)
	}
	coverage := stats.coverage()
	if coverage.RowsRead != 3 || coverage.RowsEmitted != 3 || coverage.PeakBuffered != 3 {
		t.Fatalf("coverage mismatch: %+v", coverage)
	}
}

func TestTraceDBRowSinkSpillsMergesAndCleansChunks(t *testing.T) {
	dir := t.TempDir()
	sink, err := newTraceDBRowSink(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []renderedRow{
		{tsNS: 5000, seq: 5, line: "five"},
		{tsNS: 1000, seq: 1, line: "one"},
		{tsNS: 3000, seq: 3, line: "three"},
		{tsNS: 2000, seq: 2, line: "two"},
		{tsNS: 4000, seq: 4, line: "four"},
	} {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	if len(sink.chunks) == 0 {
		t.Fatal("expected forced spill chunks before write")
	}
	chunks := append([]string(nil), sink.chunks...)
	var out bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &out)
	if err != nil {
		t.Fatalf("write spilled rows: %v", err)
	}
	if stats.SpillChunks < 2 || stats.RowsWritten != 5 || stats.PeakBufferedRows > 2 || stats.TempBytes == 0 {
		t.Fatalf("unexpected spill stats: %+v", stats)
	}
	body := out.String()
	last := -1
	for _, token := range []string{"one", "two", "three", "four", "five"} {
		pos := strings.Index(body, token)
		if pos < 0 || pos < last {
			t.Fatalf("merged output order mismatch token=%s last=%d pos=%d\n%s", token, last, pos, body)
		}
		last = pos
	}
	for _, path := range chunks {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("chunk %s should be cleaned, stat err=%v", path, err)
		}
	}
}

func TestTraceDBRowSinkCleansChunksOnWriteError(t *testing.T) {
	dir := t.TempDir()
	sink, err := newTraceDBRowSink(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []renderedRow{
		{tsNS: 1000, seq: 1, line: "one"},
		{tsNS: 2000, seq: 2, line: "two"},
	} {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	chunks := append([]string(nil), sink.chunks...)
	_, err = sink.writeTo(context.Background(), failingWriter{})
	if err == nil {
		t.Fatal("expected write failure")
	}
	for _, path := range chunks {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("chunk %s should be cleaned after failure, stat err=%v", path, statErr)
		}
	}
}

func TestTraceDBRowSinkOutputRoundTripsThroughTraceQuery(t *testing.T) {
	dir := t.TempDir()
	sink, err := newTraceDBRowSink(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	rows := []renderedRow{
		{tsNS: 2_000_000_000, seq: 2, line: "         app-20   (   20) [001] .... 2.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53"},
		{tsNS: 2_050_000_000, seq: 3, line: "         app-20   (   20) [001] .... 2.050000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120"},
		{tsNS: 1_900_000_000, seq: 1, line: "       other-30   (   30) [000] .... 1.900000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001"},
	}
	for _, row := range rows {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	outPath := filepath.Join(dir, "out.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	stats, writeErr := sink.writeTo(context.Background(), out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatalf("write trace: %v", writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if stats.RowsWritten != 3 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse sorted DB output: %v", err)
	}
	if len(idx.Events) != 3 {
		t.Fatalf("tracequery event count mismatch: %+v", idx.Events)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("forced write failure")
}
