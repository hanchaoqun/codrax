package hitraceconv

import (
	"bufio"
	"container/heap"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"sort"
	"time"
)

const defaultTraceDBRowSinkThreshold = 200_000

const (
	// Match the direct full-capture barrier. These are proof-state limits, not
	// input or output row limits: print and other non-pair rows remain unaffected.
	profilerPairBarrierMaxObservations int64 = 4_000_000
	profilerPairBarrierMaxLaneKeys     int64 = 1_000_000
)

type traceDBRowSortStats struct {
	RowsAccepted     int
	RowsWritten      int
	RowsWithheld     int
	PeakBufferedRows int
	SpillChunks      int
	TempBytes        int64
	FirstTSNS        uint64
	LastTSNS         uint64
	ElapsedUS        int64
}

func (s traceDBRowSortStats) coverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family:       "sorter",
		Table:        "__systrace_rows__",
		Role:         "systrace_text_output",
		Found:        true,
		RowsRead:     s.RowsAccepted,
		RowsEmitted:  s.RowsWritten,
		PeakBuffered: s.PeakBufferedRows,
		SpillChunks:  s.SpillChunks,
		TempBytes:    s.TempBytes,
		ElapsedUS:    s.ElapsedUS,
	}
}

type traceDBRowSink struct {
	threshold            int
	tempDir              string
	ownDir               string
	rows                 []renderedRow
	chunks               []string
	stats                traceDBRowSortStats
	pairRows             map[pairRenderKind]int
	pairLaneRows         map[pairRenderKind]map[string]int
	pairTableRows        map[pairRenderKind]map[string]map[string]int
	pairTableTotals      map[pairRenderKind]map[string]int
	poisoned             map[pairRenderKind]bool
	poisonedLanes        map[pairRenderKind]map[string]bool
	opaque               map[pairRenderKind]bool
	structuredPairRows   map[pairRenderKind]int
	structuredLaneRows   map[pairRenderKind]map[string]int
	structuredEventRows  map[pairRenderKind]map[int]int
	structuredEventLanes map[pairRenderKind]map[int]map[string]int
	activePairCensus     map[pairRenderKind]*profilerPairRowCensus
	pairObservationLimit int64
	pairLaneLimit        int64
	pairObservations     int64
	pairUniqueLanes      int64
	pairBudgetFailed     bool
	pairBudgetFailure    string
	allRowsFailClosed    bool
}

type profilerPairRowCensus struct {
	total  int
	byLane map[string]int
}

func newTraceDBRowSink(tempDir string, threshold int) (*traceDBRowSink, error) {
	if threshold <= 0 {
		threshold = defaultTraceDBRowSinkThreshold
	}
	sink := &traceDBRowSink{
		threshold: threshold, tempDir: tempDir,
		pairRows: make(map[pairRenderKind]int), pairLaneRows: make(map[pairRenderKind]map[string]int),
		pairTableRows: make(map[pairRenderKind]map[string]map[string]int), poisoned: make(map[pairRenderKind]bool),
		pairTableTotals:      make(map[pairRenderKind]map[string]int),
		poisonedLanes:        make(map[pairRenderKind]map[string]bool),
		opaque:               make(map[pairRenderKind]bool),
		structuredPairRows:   make(map[pairRenderKind]int),
		structuredLaneRows:   make(map[pairRenderKind]map[string]int),
		structuredEventRows:  make(map[pairRenderKind]map[int]int),
		structuredEventLanes: make(map[pairRenderKind]map[int]map[string]int),
		pairObservationLimit: profilerPairBarrierMaxObservations,
		pairLaneLimit:        profilerPairBarrierMaxLaneKeys,
	}
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
	if !traceDBSinglePhysicalLine(row.line, false) {
		return &traceDBOutputInvariantError{Reason: "invalid_rendered_line"}
	}
	if err := validateProfilerEventFieldProvenance(row); err != nil {
		return err
	}
	if row.profilerEventField != 0 {
		if s.structuredEventRows[row.pairKind][row.profilerEventField] == math.MaxInt {
			return &traceDBOutputInvariantError{Reason: "profiler_structured_event_counter_overflow"}
		}
		if row.pairLane != "" &&
			s.structuredEventLanes[row.pairKind][row.profilerEventField][row.pairLane] == math.MaxInt {
			return &traceDBOutputInvariantError{Reason: "profiler_structured_event_lane_counter_overflow"}
		}
	}
	if s.stats.RowsAccepted == 0 || row.tsNS < s.stats.FirstTSNS {
		s.stats.FirstTSNS = row.tsNS
	}
	if row.tsNS > s.stats.LastTSNS {
		s.stats.LastTSNS = row.tsNS
	}
	s.rows = append(s.rows, row)
	s.stats.RowsAccepted++
	if row.pairKind != pairRenderUnknown {
		s.pairRows[row.pairKind]++
		if row.pairTable != "" {
			if s.pairTableTotals[row.pairKind] == nil {
				s.pairTableTotals[row.pairKind] = make(map[string]int)
			}
			s.pairTableTotals[row.pairKind][row.pairTable]++
		}
		if s.opaque[row.pairKind] {
			s.poisonPairKind(row.pairKind)
		}
		trackLane := !profilerPairBudgetKind(row.pairKind) ||
			(!s.poisoned[row.pairKind] && s.observeProfilerPairState(row.pairKind, row.pairLane))
		if trackLane && row.pairLane != "" {
			if s.pairLaneRows[row.pairKind] == nil {
				s.pairLaneRows[row.pairKind] = make(map[string]int)
			}
			s.pairLaneRows[row.pairKind][row.pairLane]++
		}
		if trackLane && row.pairTable != "" {
			if s.pairTableRows[row.pairKind] == nil {
				s.pairTableRows[row.pairKind] = make(map[string]map[string]int)
			}
			if s.pairTableRows[row.pairKind][row.pairTable] == nil {
				s.pairTableRows[row.pairKind][row.pairTable] = make(map[string]int)
			}
			s.pairTableRows[row.pairKind][row.pairTable][row.pairLane]++
		}
		s.accountActivePairRow(row, trackLane)
		if row.structuredPair {
			s.structuredPairRows[row.pairKind]++
			if trackLane && row.pairLane != "" {
				if s.structuredLaneRows[row.pairKind] == nil {
					s.structuredLaneRows[row.pairKind] = make(map[string]int)
				}
				s.structuredLaneRows[row.pairKind][row.pairLane]++
			}
			if row.profilerEventField != 0 {
				if s.structuredEventRows[row.pairKind] == nil {
					s.structuredEventRows[row.pairKind] = make(map[int]int)
				}
				s.structuredEventRows[row.pairKind][row.profilerEventField]++
				if trackLane && row.pairLane != "" {
					if s.structuredEventLanes[row.pairKind] == nil {
						s.structuredEventLanes[row.pairKind] = make(map[int]map[string]int)
					}
					if s.structuredEventLanes[row.pairKind][row.profilerEventField] == nil {
						s.structuredEventLanes[row.pairKind][row.profilerEventField] = make(map[string]int)
					}
					s.structuredEventLanes[row.pairKind][row.profilerEventField][row.pairLane]++
				}
			}
		}
	}
	if len(s.rows) > s.stats.PeakBufferedRows {
		s.stats.PeakBufferedRows = len(s.rows)
	}
	if len(s.rows) >= s.threshold {
		return s.flushChunk()
	}
	return nil
}

func profilerStructuredPairEventField(kind pairRenderKind, field int) bool {
	switch kind {
	case pairRenderMMC:
		return field == 4015 || field == 4016
	case pairRenderF2FS:
		return field >= 4009 && field <= 4012
	default:
		return false
	}
}

func validateProfilerEventFieldProvenance(row renderedRow) error {
	if row.profilerEventField == 0 {
		if row.structuredPair {
			return &traceDBOutputInvariantError{Reason: "profiler_structured_pair_missing_event_field"}
		}
		return nil
	}
	if !row.structuredPair {
		return &traceDBOutputInvariantError{Reason: "profiler_event_field_without_structured_pair"}
	}
	if !profilerStructuredPairEventField(row.pairKind, row.profilerEventField) {
		return &traceDBOutputInvariantError{Reason: "profiler_event_field_pair_kind_mismatch"}
	}
	return nil
}

func (s *traceDBRowSink) markPairCaptureOpaque(kind pairRenderKind) {
	if s == nil || kind == pairRenderUnknown {
		return
	}
	s.opaque[kind] = true
	if s.pairRows[kind] > 0 {
		s.poisonPairKind(kind)
	}
}

func (s *traceDBRowSink) failCloseAllRows() {
	if s == nil {
		return
	}
	s.allRowsFailClosed = true
	for _, kind := range []pairRenderKind{
		pairRenderWorkqueue, pairRenderDMAFence, pairRenderMMC, pairRenderF2FS,
	} {
		s.opaque[kind] = true
		s.poisonPairKind(kind)
	}
}

func (s *traceDBRowSink) poisonPairKind(kind pairRenderKind) {
	if s == nil || kind == pairRenderUnknown {
		return
	}
	s.poisoned[kind] = true
	if profilerPairBudgetKind(kind) {
		// Whole-family publication now reads only the scalar totals. Drop all
		// subordinate maps immediately and never grow them again.
		s.pairLaneRows[kind] = nil
		s.pairTableRows[kind] = nil
		s.poisonedLanes[kind] = nil
		s.structuredLaneRows[kind] = nil
		s.structuredEventLanes[kind] = nil
		if census := s.activePairCensus[kind]; census != nil {
			census.byLane = nil
		}
	}
}

func (s *traceDBRowSink) poisonPairLane(kind pairRenderKind, lane string) {
	if s == nil || kind == pairRenderUnknown {
		return
	}
	if lane == "" {
		s.poisonPairKind(kind)
		return
	}
	if s.poisoned[kind] {
		return
	}
	if profilerPairBudgetKind(kind) && !s.observeProfilerPairState(kind, lane) {
		return
	}
	if s.poisonedLanes[kind] == nil {
		s.poisonedLanes[kind] = make(map[string]bool)
	}
	s.poisonedLanes[kind][lane] = true
}

func profilerPairBudgetKind(kind pairRenderKind) bool {
	return kind == pairRenderMMC || kind == pairRenderF2FS
}

func (s *traceDBRowSink) observeProfilerPairState(kind pairRenderKind, lane string) bool {
	if s == nil || !profilerPairBudgetKind(kind) || s.poisoned[kind] || s.pairBudgetFailed {
		return false
	}
	if s.pairObservations >= s.pairObservationLimit {
		s.failProfilerPairBudget("observations")
		return false
	}
	s.pairObservations++
	if lane == "" || s.pairLaneRows[kind][lane] > 0 || s.poisonedLanes[kind][lane] {
		return true
	}
	if s.pairUniqueLanes >= s.pairLaneLimit {
		s.failProfilerPairBudget("lane_keys")
		return false
	}
	s.pairUniqueLanes++
	return true
}

func (s *traceDBRowSink) failProfilerPairBudget(reason string) {
	if s == nil || s.pairBudgetFailed {
		return
	}
	s.pairBudgetFailed = true
	s.pairBudgetFailure = reason
	// Resource pressure invalidates the completeness proof, not just the row
	// that crossed a cap. Close both profiler pair-critical families before any
	// output so the other family cannot become a source-dependent rescue path.
	s.poisonPairKind(pairRenderMMC)
	s.poisonPairKind(pairRenderF2FS)
}

func (s *traceDBRowSink) pairKindPoisoned(kind pairRenderKind) bool {
	return s != nil && kind != pairRenderUnknown &&
		(s.poisoned[kind] || len(s.poisonedLanes[kind]) > 0)
}

func (s *traceDBRowSink) withheldPairRows() int {
	if s == nil {
		return 0
	}
	total := 0
	for kind := range s.pairRows {
		total += s.withheldPairRowsForKind(kind)
	}
	return total
}

func (s *traceDBRowSink) withheldPairRowsForKind(kind pairRenderKind) int {
	if s == nil || kind == pairRenderUnknown {
		return 0
	}
	if s.poisoned[kind] {
		return s.pairRows[kind]
	}
	total := 0
	for lane, count := range s.pairLaneRows[kind] {
		if s.poisonedLanes[kind][lane] {
			total += count
		}
	}
	return total
}

func (s *traceDBRowSink) withheldStructuredPairRows() int {
	if s == nil {
		return 0
	}
	total := 0
	for kind := range s.structuredPairRows {
		total += s.withheldStructuredPairRowsForKind(kind)
	}
	return total
}

func (s *traceDBRowSink) withheldStructuredPairRowsForKind(kind pairRenderKind) int {
	if s == nil || kind == pairRenderUnknown {
		return 0
	}
	if s.poisoned[kind] {
		return s.structuredPairRows[kind]
	}
	total := 0
	for lane, count := range s.structuredLaneRows[kind] {
		if s.poisonedLanes[kind][lane] {
			total += count
		}
	}
	return total
}

// withheldStructuredPairRowsForEventField reports only structured profiler
// rows from one exact FtraceEvent field. Text-compatible rows may share the
// same rendered event name, but cannot enter this typed accounting lane.
func (s *traceDBRowSink) withheldStructuredPairRowsForEventField(kind pairRenderKind, field int) int {
	if s == nil || !profilerStructuredPairEventField(kind, field) {
		return 0
	}
	totalForField := s.structuredEventRows[kind][field]
	if s.poisoned[kind] {
		return totalForField
	}
	withheld := 0
	for lane, count := range s.structuredEventLanes[kind][field] {
		if !s.poisonedLanes[kind][lane] {
			continue
		}
		if !checkedProfilerIntAddTo(&withheld, count) {
			// Counts are disjoint subsets of totalForField, so this is reachable
			// only after internal state corruption. Fail closed for this field.
			return totalForField
		}
	}
	return withheld
}

func (s *traceDBRowSink) beginPairRowCensus() bool {
	if s == nil || s.activePairCensus != nil {
		return false
	}
	s.activePairCensus = make(map[pairRenderKind]*profilerPairRowCensus)
	return true
}

func (s *traceDBRowSink) accountActivePairRow(row renderedRow, trackLane bool) {
	if s == nil || s.activePairCensus == nil || row.pairKind == pairRenderUnknown {
		return
	}
	census := s.activePairCensus[row.pairKind]
	if census == nil {
		census = &profilerPairRowCensus{byLane: make(map[string]int)}
		s.activePairCensus[row.pairKind] = census
	}
	census.total++
	if trackLane && row.pairLane != "" {
		census.byLane[row.pairLane]++
	}
}

func (s *traceDBRowSink) currentPairRowCensus(kind pairRenderKind) profilerPairRowCensus {
	if s == nil || s.activePairCensus == nil || s.activePairCensus[kind] == nil {
		return profilerPairRowCensus{}
	}
	return *s.activePairCensus[kind]
}

func (s *traceDBRowSink) endPairRowCensus() (profilerPairRowCensus, profilerPairRowCensus) {
	mmc := s.currentPairRowCensus(pairRenderMMC)
	f2fs := s.currentPairRowCensus(pairRenderF2FS)
	if s != nil {
		s.activePairCensus = nil
	}
	return mmc, f2fs
}

func (s *traceDBRowSink) withheldPairRowsFromCensus(kind pairRenderKind, census profilerPairRowCensus) int {
	if s == nil || kind == pairRenderUnknown {
		return 0
	}
	if s.poisoned[kind] {
		return census.total
	}
	total := 0
	for lane, count := range census.byLane {
		if s.poisonedLanes[kind][lane] {
			total += count
		}
	}
	return total
}

func (s *traceDBRowSink) withheldPairRowsForTable(kind pairRenderKind, table string) int {
	if s == nil || kind == pairRenderUnknown || table == "" {
		return 0
	}
	lanes := s.pairTableRows[kind][table]
	if s.poisoned[kind] {
		return s.pairTableTotals[kind][table]
	}
	total := 0
	for lane, count := range lanes {
		if s.poisonedLanes[kind][lane] {
			total += count
		}
	}
	return total
}

func (s *traceDBRowSink) publishableRows() int {
	if s == nil {
		return 0
	}
	if s.allRowsFailClosed {
		return 0
	}
	count := s.stats.RowsAccepted - s.withheldPairRows()
	if count < 0 {
		return 0
	}
	return count
}

func (s *traceDBRowSink) rowPublishable(row renderedRow) bool {
	return !s.allRowsFailClosed &&
		(row.pairKind == pairRenderUnknown ||
			(!s.poisoned[row.pairKind] && (row.pairLane == "" || !s.poisonedLanes[row.pairKind][row.pairLane])))
}

func (s *traceDBRowSink) accountWrittenRow(row renderedRow) {
	if s.stats.RowsWritten == 0 || row.tsNS < s.stats.FirstTSNS {
		s.stats.FirstTSNS = row.tsNS
	}
	if s.stats.RowsWritten == 0 || row.tsNS > s.stats.LastTSNS {
		s.stats.LastTSNS = row.tsNS
	}
	s.stats.RowsWritten++
}

func (s *traceDBRowSink) writeTo(ctx context.Context, w io.Writer) (stats traceDBRowSortStats, err error) {
	start := time.Now()
	defer func() {
		s.stats.ElapsedUS = traceDBCoverageElapsedUS(start)
		stats = s.stats
	}()
	defer s.cleanup()
	s.stats.RowsWritten = 0
	s.stats.RowsWithheld = s.withheldPairRows()
	s.stats.FirstTSNS = 0
	s.stats.LastTSNS = 0
	if s.allRowsFailClosed {
		s.stats.RowsWithheld = s.stats.RowsAccepted
		return s.stats, nil
	}
	if len(s.chunks) == 0 {
		published := s.rows[:0]
		for _, row := range s.rows {
			if !s.rowPublishable(row) {
				continue
			}
			published = append(published, row)
		}
		sortRenderedRows(published)
		err := writeRows(w, published)
		if err == nil {
			for _, row := range published {
				s.accountWrittenRow(row)
			}
		}
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
		if s.rowPublishable(item.row) {
			if _, err := bw.WriteString(item.row.line); err != nil {
				return s.stats, err
			}
			if err := bw.WriteByte('\n'); err != nil {
				return s.stats, err
			}
			s.accountWrittenRow(item.row)
		}
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
		if err := enc.Encode(traceDBChunkRow{
			TSNS: row.tsNS, Seq: row.seq, Line: row.line, PairKind: row.pairKind,
			PairLane: row.pairLane, PairTable: row.pairTable, StructuredPair: row.structuredPair,
			ProfilerEventField: row.profilerEventField,
		}); err != nil {
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
	TSNS               uint64         `json:"ts_ns"`
	Seq                int            `json:"seq"`
	Line               string         `json:"line"`
	PairKind           pairRenderKind `json:"pair_kind,omitempty"`
	PairLane           string         `json:"pair_lane,omitempty"`
	PairTable          string         `json:"pair_table,omitempty"`
	StructuredPair     bool           `json:"structured_pair,omitempty"`
	ProfilerEventField int            `json:"profiler_event_field,omitempty"`
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
	row := renderedRow{
		tsNS: item.TSNS, seq: item.Seq, line: item.Line, pairKind: item.PairKind,
		pairLane: item.PairLane, pairTable: item.PairTable, structuredPair: item.StructuredPair,
		profilerEventField: item.ProfilerEventField,
	}
	if err := validateProfilerEventFieldProvenance(row); err != nil {
		return renderedRow{}, false, err
	}
	return row, true, nil
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
