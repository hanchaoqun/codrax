package hitraceconv

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestTraceDBBoundedJSONLReaderReusesBorrowedAndFragmentScratch(t *testing.T) {
	assertAllocationFree := func(t *testing.T, data []byte, bufferSize int, wantRecords int) {
		t.Helper()
		source := bytes.NewReader(data)
		reader := bufio.NewReaderSize(source, bufferSize)
		var scratch []byte
		run := func() {
			source.Reset(data)
			reader.Reset(source)
			count := 0
			for {
				raw, ok, err := readTraceDBBoundedJSONLRecord(reader, uint64(len(data)+1), &scratch)
				if err != nil {
					panic(err)
				}
				if !ok {
					break
				}
				if len(raw) == 0 || raw[len(raw)-1] != '\n' || cap(raw) != len(raw) {
					panic("invalid JSONL lease")
				}
				count++
			}
			if count != wantRecords {
				panic("record count drift")
			}
		}
		run() // Warm any fragmented-record scratch before measuring reuse.
		if allocations := testing.AllocsPerRun(100, run); allocations != 0 {
			t.Fatalf("bounded reader allocated after warmup: allocs/run=%g buffer=%d records=%d scratch_cap=%d",
				allocations, bufferSize, wantRecords, cap(scratch))
		}
	}

	short := bytes.Repeat([]byte("x\n"), 128)
	assertAllocationFree(t, short, 16, 128)

	fragmentedLine := append(bytes.Repeat([]byte{'f'}, 63), '\n')
	fragmented := bytes.Repeat(fragmentedLine, 64)
	assertAllocationFree(t, fragmented, 16, 64)

	source := bytes.NewReader(bytes.Join([][]byte{
		append(bytes.Repeat([]byte{'a'}, 95), '\n'),
		append(bytes.Repeat([]byte{'b'}, 95), '\n'),
	}, nil))
	reader := bufio.NewReaderSize(source, 16)
	var scratch []byte
	first, ok, err := readTraceDBBoundedJSONLRecord(reader, 256, &scratch)
	if err != nil || !ok || len(first) != 96 || cap(first) != len(first) || cap(scratch) < len(first) {
		t.Fatalf("first fragmented lease=%d/%d ok=%t err=%v", len(first), cap(scratch), ok, err)
	}
	firstAddress := &first[0]
	second, ok, err := readTraceDBBoundedJSONLRecord(reader, 256, &scratch)
	if err != nil || !ok || len(second) != 96 || cap(second) != len(second) ||
		&second[0] != firstAddress || second[0] != 'b' {
		t.Fatalf("fragment scratch was not reused: second=%d/%d same=%t ok=%t err=%v",
			len(second), cap(scratch), len(second) > 0 && &second[0] == firstAddress, ok, err)
	}
	_, ok, err = readTraceDBBoundedJSONLRecord(reader, 256, &scratch)
	if err != nil || ok || cap(scratch) < 96 {
		t.Fatalf("EOF discarded reusable scratch: ok=%t err=%v cap=%d", ok, err, cap(scratch))
	}
}

func TestTraceDBBoundedJSONLReaderPreservesStrictErrorBoundaries(t *testing.T) {
	for _, exact := range []struct {
		name       string
		data       string
		bufferSize int
	}{
		{name: "borrowed exact limit", data: "abc\n", bufferSize: 16},
		{name: "fragmented exact limit", data: strings.Repeat("x", 31) + "\n", bufferSize: 16},
	} {
		t.Run(exact.name, func(t *testing.T) {
			reader := bufio.NewReaderSize(strings.NewReader(exact.data), exact.bufferSize)
			var scratch []byte
			raw, ok, err := readTraceDBBoundedJSONLRecord(
				reader, uint64(len(exact.data)), &scratch,
			)
			if err != nil || !ok || len(raw) != len(exact.data) || cap(raw) != len(raw) {
				t.Fatalf("exact physical limit raw=%d/%d ok=%t err=%v",
					len(raw), cap(raw), ok, err)
			}
		})
	}

	for _, test := range []struct {
		name    string
		data    string
		maximum uint64
		want    string
	}{
		{name: "oversize", data: "0123456789\n", maximum: 10,
			want: "trace_row_sort_physical_record_too_large"},
		{name: "truncated", data: "unterminated", maximum: 32,
			want: "trace_row_sort_record_truncated"},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := bufio.NewReaderSize(strings.NewReader(test.data), 16)
			var scratch []byte
			if _, ok, err := readTraceDBBoundedJSONLRecord(reader, test.maximum, &scratch); ok ||
				traceDBBoundedInvariantReason(t, err) != test.want {
				t.Fatalf("strict record boundary ok=%t err=%v want=%q", ok, err, test.want)
			}
		})
	}

	reader := bufio.NewReaderSize(strings.NewReader("x\n"), 16)
	if _, ok, err := readTraceDBBoundedJSONLRecord(reader, 16, nil); ok ||
		traceDBBoundedInvariantReason(t, err) != "trace_row_sort_record_limit_invalid" {
		t.Fatalf("nil scratch authority ok=%t err=%v", ok, err)
	}
}

func TestTraceDBAuthenticatedReaderReusesFragmentScratchAcrossResetAndReleasesOnClose(t *testing.T) {
	sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 8, traceDBRowSinkOptions{
		maxRunRowBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	for index, marker := range []byte{'a', 'b'} {
		if err := sink.add(renderedRow{
			tsNS: uint64(index + 1), seq: index + 1,
			line: strings.Repeat(string(marker), 300<<10),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := sink.flushChunk(); err != nil {
		t.Fatal(err)
	}
	if len(sink.runs) != 1 {
		t.Fatalf("long-record fixture runs=%d want=1", len(sink.runs))
	}
	reader, err := sink.openAuthenticatedRunReader(sink.runs[0])
	if err != nil {
		t.Fatal(err)
	}
	var scratchAddress *byte
	var firstOwnedRow traceDBStoredRow
	for pass := 0; pass < 2; pass++ {
		for rowIndex, marker := range []byte{'a', 'b'} {
			record, ok, readErr := reader.next(context.Background())
			if readErr != nil || !ok || len(record.raw) <= 256<<10 || cap(record.raw) != len(record.raw) ||
				record.row.line[0] != marker {
				t.Fatalf("pass=%d row=%d long record=%d ok=%t err=%v",
					pass, rowIndex, len(record.raw), ok, readErr)
			}
			if scratchAddress == nil {
				scratchAddress = &record.raw[0]
			} else if &record.raw[0] != scratchAddress {
				t.Fatalf("pass=%d row=%d scratch address changed", pass, rowIndex)
			}
			if pass == 0 && rowIndex == 0 {
				firstOwnedRow = record.row
			}
			if pass == 0 && rowIndex == 1 &&
				(len(firstOwnedRow.line) != 300<<10 || firstOwnedRow.line[0] != 'a') {
				t.Fatal("decoded row did not outlive the borrowed raw lease")
			}
		}
		if _, ok, readErr := reader.next(context.Background()); readErr != nil || ok || !reader.verified {
			t.Fatalf("pass=%d EOF ok=%t verified=%t err=%v", pass, ok, reader.verified, readErr)
		}
		if pass == 0 {
			beforeCap := cap(reader.recordScratch)
			if err := reader.reset(); err != nil || cap(reader.recordScratch) != beforeCap {
				t.Fatalf("reset lost scratch cap=%d/%d err=%v", beforeCap, cap(reader.recordScratch), err)
			}
		}
	}
	if cap(reader.recordScratch) < 300<<10 {
		t.Fatalf("long-record scratch cap=%d", cap(reader.recordScratch))
	}
	if err := reader.close(); err != nil {
		t.Fatal(err)
	}
	if reader.recordScratch != nil {
		t.Fatalf("close retained record scratch cap=%d", cap(reader.recordScratch))
	}
}

func TestTraceDBAuthenticatedReaderCloseInvalidatesBufferedRowsAndLeaseState(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	for index := 0; index < 2; index++ {
		if err := sink.add(renderedRow{
			tsNS: uint64(index + 1), seq: index + 1,
			line: "buffered-close-" + string(rune('a'+index)),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := sink.flushChunk(); err != nil {
		t.Fatal(err)
	}
	reader, err := sink.openAuthenticatedRunReader(sink.runs[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := reader.next(context.Background()); err != nil || !ok {
		t.Fatalf("first buffered row ok=%t err=%v", ok, err)
	}
	if err := reader.close(); err != nil {
		t.Fatal(err)
	}
	if err := reader.close(); err != nil {
		t.Fatalf("second close was not idempotent: %v", err)
	}
	if _, ok, err := reader.next(context.Background()); ok ||
		traceDBBoundedInvariantReason(t, err) != "trace_row_sort_reader_state_invalid" {
		t.Fatalf("closed reader returned prefetched row: ok=%t err=%v", ok, err)
	}
	if reader.reader != nil || reader.proof != nil || reader.recordScratch != nil ||
		reader.rowsRead != 0 || reader.havePrev || reader.verified {
		t.Fatalf("close retained reader lease state: %+v", reader)
	}
}

func TestTraceDBMergeConsumesEachBorrowedRawBeforeAdvancingItsReader(t *testing.T) {
	sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 2, traceDBRowSinkOptions{
		maxRunRowBytes: 1 << 20,
		mergeFanIn:     2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	rows := []struct {
		ts     uint64
		marker byte
	}{
		{ts: 1, marker: 'a'}, {ts: 3, marker: 'c'},
		{ts: 2, marker: 'b'}, {ts: 4, marker: 'd'},
	}
	for index, item := range rows {
		if err := sink.add(renderedRow{
			tsNS: item.ts, seq: index + 1,
			line: strings.Repeat(string(item.marker), 256<<10+64),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if len(sink.runs) != 2 {
		t.Fatalf("interleaved merge fixture runs=%d want=2", len(sink.runs))
	}
	if err := sink.prepareForPublication(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sink.runs) != 1 || sink.runs[0].rowCount != 4 {
		t.Fatalf("interleaved merge output=%+v", sink.runs)
	}
	reader, err := sink.openAuthenticatedRunReader(sink.runs[0])
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for {
		record, ok, readErr := reader.next(context.Background())
		if readErr != nil {
			_ = reader.close()
			t.Fatal(readErr)
		}
		if !ok {
			break
		}
		got += string(record.row.line[0])
	}
	if err := reader.close(); err != nil {
		t.Fatal(err)
	}
	if got != "abcd" {
		t.Fatalf("merge advanced a reader before consuming borrowed raw: order=%q", got)
	}
}
