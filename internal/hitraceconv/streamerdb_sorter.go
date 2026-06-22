package hitraceconv

import (
	"bufio"
	"container/heap"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
)

const defaultTraceDBRowSinkThreshold = 200_000

type traceDBRowSortStats struct {
	RowsAccepted     int
	RowsWritten      int
	PeakBufferedRows int
	SpillChunks      int
	TempBytes        int64
}

func (s traceDBRowSortStats) coverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family:       "sorter",
		Table:        "__systrace_rows__",
		Found:        true,
		RowsRead:     s.RowsAccepted,
		RowsEmitted:  s.RowsWritten,
		PeakBuffered: s.PeakBufferedRows,
		SpillChunks:  s.SpillChunks,
		TempBytes:    s.TempBytes,
	}
}

type traceDBRowSink struct {
	threshold int
	tempDir   string
	ownDir    string
	rows      []renderedRow
	chunks    []string
	stats     traceDBRowSortStats
}

func newTraceDBRowSink(tempDir string, threshold int) (*traceDBRowSink, error) {
	if threshold <= 0 {
		threshold = defaultTraceDBRowSinkThreshold
	}
	sink := &traceDBRowSink{threshold: threshold, tempDir: tempDir}
	if sink.tempDir == "" {
		dir, err := os.MkdirTemp("", "codrax-tracedb-sort-*")
		if err != nil {
			return nil, err
		}
		sink.tempDir = dir
		sink.ownDir = dir
	}
	return sink, nil
}

func (s *traceDBRowSink) add(row renderedRow) error {
	s.rows = append(s.rows, row)
	s.stats.RowsAccepted++
	if len(s.rows) > s.stats.PeakBufferedRows {
		s.stats.PeakBufferedRows = len(s.rows)
	}
	if len(s.rows) >= s.threshold {
		return s.flushChunk()
	}
	return nil
}

func (s *traceDBRowSink) writeTo(ctx context.Context, w io.Writer) (traceDBRowSortStats, error) {
	defer s.cleanup()
	if len(s.chunks) == 0 {
		sortRenderedRows(s.rows)
		err := writeRows(w, s.rows)
		s.stats.RowsWritten = len(s.rows)
		return s.stats, err
	}
	if err := s.flushChunk(); err != nil {
		return s.stats, err
	}
	bw := bufio.NewWriterSize(w, 256*1024)
	if err := writeSystraceHeader(bw); err != nil {
		return s.stats, err
	}
	readers := make([]*traceDBChunkReader, 0, len(s.chunks))
	for _, path := range s.chunks {
		reader, err := openTraceDBChunkReader(path)
		if err != nil {
			closeTraceDBChunkReaders(readers)
			return s.stats, err
		}
		readers = append(readers, reader)
	}
	defer closeTraceDBChunkReaders(readers)
	h := traceDBRowHeap{}
	for idx, reader := range readers {
		row, ok, err := reader.next()
		if err != nil {
			return s.stats, err
		}
		if ok {
			heap.Push(&h, traceDBRowHeapItem{row: row, readerIndex: idx})
		}
	}
	for h.Len() > 0 {
		if err := ctx.Err(); err != nil {
			return s.stats, err
		}
		item := heap.Pop(&h).(traceDBRowHeapItem)
		if _, err := bw.WriteString(item.row.line); err != nil {
			return s.stats, err
		}
		if err := bw.WriteByte('\n'); err != nil {
			return s.stats, err
		}
		s.stats.RowsWritten++
		row, ok, err := readers[item.readerIndex].next()
		if err != nil {
			return s.stats, err
		}
		if ok {
			heap.Push(&h, traceDBRowHeapItem{row: row, readerIndex: item.readerIndex})
		}
	}
	if err := bw.Flush(); err != nil {
		return s.stats, err
	}
	return s.stats, nil
}

func (s *traceDBRowSink) flushChunk() error {
	if len(s.rows) == 0 {
		return nil
	}
	sortRenderedRows(s.rows)
	f, err := os.CreateTemp(s.tempDir, "rows-*.jsonl")
	if err != nil {
		return err
	}
	path := f.Name()
	bw := bufio.NewWriterSize(f, 256*1024)
	enc := json.NewEncoder(bw)
	for _, row := range s.rows {
		if err := enc.Encode(traceDBChunkRow{TSNS: row.tsNS, Seq: row.seq, Line: row.line}); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return err
		}
	}
	if err := bw.Flush(); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	s.chunks = append(s.chunks, path)
	s.stats.SpillChunks++
	s.stats.TempBytes += info.Size()
	s.rows = s.rows[:0]
	return nil
}

func (s *traceDBRowSink) cleanup() {
	for _, path := range s.chunks {
		_ = os.Remove(path)
	}
	s.chunks = nil
	if s.ownDir != "" {
		_ = os.RemoveAll(s.ownDir)
	}
}

func sortRenderedRows(rows []renderedRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].tsNS == rows[j].tsNS {
			return rows[i].seq < rows[j].seq
		}
		return rows[i].tsNS < rows[j].tsNS
	})
}

type traceDBChunkRow struct {
	TSNS uint64 `json:"ts_ns"`
	Seq  int    `json:"seq"`
	Line string `json:"line"`
}

type traceDBChunkReader struct {
	file *os.File
	dec  *json.Decoder
}

func openTraceDBChunkReader(path string) (*traceDBChunkReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &traceDBChunkReader{file: f, dec: json.NewDecoder(f)}, nil
}

func (r *traceDBChunkReader) next() (renderedRow, bool, error) {
	var item traceDBChunkRow
	err := r.dec.Decode(&item)
	if errors.Is(err, io.EOF) {
		return renderedRow{}, false, nil
	}
	if err != nil {
		return renderedRow{}, false, err
	}
	return renderedRow{tsNS: item.TSNS, seq: item.Seq, line: item.Line}, true, nil
}

func (r *traceDBChunkReader) close() error {
	if r == nil || r.file == nil {
		return nil
	}
	return r.file.Close()
}

func closeTraceDBChunkReaders(readers []*traceDBChunkReader) {
	for _, reader := range readers {
		_ = reader.close()
	}
}

type traceDBRowHeapItem struct {
	row         renderedRow
	readerIndex int
}

type traceDBRowHeap []traceDBRowHeapItem

func (h traceDBRowHeap) Len() int { return len(h) }

func (h traceDBRowHeap) Less(i, j int) bool {
	if h[i].row.tsNS == h[j].row.tsNS {
		return h[i].row.seq < h[j].row.seq
	}
	return h[i].row.tsNS < h[j].row.tsNS
}

func (h traceDBRowHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *traceDBRowHeap) Push(x any) {
	*h = append(*h, x.(traceDBRowHeapItem))
}

func (h *traceDBRowHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}
