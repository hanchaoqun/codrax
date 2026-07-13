package hitraceconv

import (
	"bufio"
	"bytes"
	"container/heap"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
	"unsafe"
)

const defaultTraceDBRowSinkThreshold = 200_000

const (
	defaultTraceDBRowBufferBytes  uint64 = 64 << 20
	defaultTraceDBRowMergeFanIn          = 32
	defaultTraceDBActiveTempBytes uint64 = 4 << 30
	defaultTraceDBLiveTempBytes   uint64 = 8 << 30
	// The controlled buffer allocator keeps both metadata slices at one exact
	// capacity below 2x logical length. The compile-time proof below makes 256
	// bytes/logical row cover both backing arrays and slice headers.
	traceDBBufferedRowMetadataBytes uint64 = 256
	// encoding/json escapes each byte of the three bounded string fields by at
	// most six bytes. Keep additional room for keys and numeric punctuation.
	defaultTraceDBMaxPhysicalRunRowBytes uint64 = 24 << 20
)

// The controlled doubling allocator below guarantees cap < 2*len for every
// non-empty buffer. This compile-time proof includes both backing arrays and
// both slice headers; adding fields to renderedRow or changing an architecture
// cannot silently invalidate the fixed per-row charge.
const traceDBBufferedRowMetadataProofBytes = int(2*(unsafe.Sizeof(renderedRow{})+unsafe.Sizeof(uint64(0))) +
	unsafe.Sizeof([]renderedRow(nil)) + unsafe.Sizeof([]uint64(nil)))

var _ [int(traceDBBufferedRowMetadataBytes) - traceDBBufferedRowMetadataProofBytes]byte

const profilerPairStorageIntegrityFailure = "profiler_pair_storage_integrity_failure"

const (
	// Match the direct full-capture barrier. These are proof-state limits, not
	// input or output row limits: print and other non-pair rows remain unaffected.
	profilerPairBarrierMaxObservations int64 = 4_000_000
	profilerPairBarrierMaxLaneKeys     int64 = 1_000_000
)

type traceDBRowSortStats struct {
	RowsAccepted         int
	RowsWritten          int
	RowsWithheld         int
	PeakBufferedRows     int
	PeakBufferedBytes    uint64
	SpillChunks          int
	TempBytes            int64
	CurrentLiveTempBytes uint64
	PeakLiveTempBytes    uint64
	PeakOpenRunFDs       int
	MergePasses          int
	FirstTSNS            uint64
	LastTSNS             uint64
	ElapsedUS            int64
	FailureReason        string
}

type traceDBRunManifest struct {
	path     string
	size     uint64
	rowCount uint64
	digest   [sha256.Size]byte
	level    uint32
	ordinal  uint64
}

type traceDBRunInputIntegrityError struct{ cause error }

func (err *traceDBRunInputIntegrityError) Error() string {
	if err == nil || err.cause == nil {
		return "trace row run input integrity failure"
	}
	return "trace row run input integrity failure: " + err.cause.Error()
}

func (err *traceDBRunInputIntegrityError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func traceDBRunInputIntegrity(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &traceDBRunInputIntegrityError{cause: err}
}

// traceDBRunInputIntegrityOnly rejects mixed error graphs. A registered input
// may fail while a pending output or cleanup operation independently fails;
// that combination must remain a sorter resource failure rather than being
// laundered into customer-source corruption merely because errors.As finds one
// integrity branch.
func traceDBRunInputIntegrityOnly(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if _, ok := err.(*traceDBRunInputIntegrityError); ok {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		branches := joined.Unwrap()
		if len(branches) == 0 {
			return false
		}
		for _, branch := range branches {
			if !traceDBRunInputIntegrityOnly(branch) {
				return false
			}
		}
		return true
	}
	return false
}

type traceDBRowSinkOptions struct {
	bufferBytes    uint64
	maxRunRowBytes uint64
	mergeFanIn     int
	activeTempCap  uint64
	liveTempCap    uint64
	ops            traceDBRowSinkOps
}

type traceDBRowSinkOps struct {
	createTemp func(string, string) (*os.File, error)
	open       func(string) (*os.File, error)
	stat       func(string) (os.FileInfo, error)
	remove     func(string) error
	removeAll  func(string) error
	fault      func(point, path string) error
}

type traceDBTempArtifact struct {
	path    string
	removed bool
}

func checkedTraceDBUint64Add(left, right uint64) (uint64, bool) {
	if right > math.MaxUint64-left {
		return 0, false
	}
	return left + right, true
}

// traceDBJoinPreservingSingle keeps the historical concrete error identity
// when cleanup/timing adds no second failure. errors.Join(err, nil) wraps err
// in a joinError, which breaks callers that intentionally type-assert the
// single fail-loud invariant.
func traceDBJoinPreservingSingle(primary error, additional ...error) error {
	result := primary
	for _, candidate := range additional {
		if candidate == nil {
			continue
		}
		if result == nil {
			result = candidate
			continue
		}
		result = errors.Join(result, candidate)
	}
	return result
}

func traceDBSorterOperationError(point string, cause error) error {
	if cause == nil {
		return nil
	}
	reason := "trace_row_sort_operation_failed"
	switch point {
	case "create", "open", "stat", "read", "decode", "write", "flush", "close", "remove", "remove_all", "encode":
		reason = "trace_row_sort_run_" + point + "_failed"
	}
	return errors.Join(&traceDBOutputInvariantError{Reason: reason}, cause)
}

func (s traceDBRowSortStats) coverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family:               "sorter",
		Table:                "__systrace_rows__",
		Role:                 "systrace_text_output",
		Found:                true,
		RowsRead:             s.RowsAccepted,
		RowsEmitted:          s.RowsWritten,
		PeakBuffered:         s.PeakBufferedRows,
		PeakBufferedBytes:    s.PeakBufferedBytes,
		SpillChunks:          s.SpillChunks,
		TempBytes:            s.TempBytes,
		CurrentLiveTempBytes: s.CurrentLiveTempBytes,
		PeakLiveTempBytes:    s.PeakLiveTempBytes,
		PeakOpenRunFDs:       s.PeakOpenRunFDs,
		MergePasses:          s.MergePasses,
		ElapsedUS:            s.ElapsedUS,
		Error:                s.FailureReason,
		FieldSources: map[string]string{
			"row_buffer_limits": fmt.Sprintf("%d_bytes+%d_rows",
				defaultTraceDBRowBufferBytes, defaultTraceDBRowSinkThreshold),
			"merge_limits": fmt.Sprintf("%d_input_runs+%d_total_run_fds",
				defaultTraceDBRowMergeFanIn, defaultTraceDBRowMergeFanIn+1),
			"temp_limits": fmt.Sprintf("%d_active_bytes+%d_live_bytes",
				defaultTraceDBActiveTempBytes, defaultTraceDBLiveTempBytes),
		},
	}
}

type traceDBRowSink struct {
	threshold            int
	tempDir              string
	ownDir               string
	rows                 []renderedRow
	rowIngestOrdinals    []uint64
	bufferedBytes        uint64
	nextIngestOrdinal    uint64
	nextRunOrdinal       uint64
	runs                 []traceDBRunManifest
	prepared             bool
	prepareFailure       error
	options              traceDBRowSinkOptions
	operationContext     context.Context
	artifacts            map[string]*traceDBTempArtifact
	openRunFDs           int
	activeTempBytes      uint64
	liveTempBytes        uint64
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
	activePairCensus     profilerPairCensusSet
	pairCensusActive     bool
	legacyPairProof      profilerPairProofDomain
	blockPairProof       profilerPairProofDomain
	pairRowCapacity      int64
	pairRowMappings      map[int]profilerPairRowMapping
	blockLaneClocks      map[string]profilerBlockLaneClock
	pairAuthorityFailure string
	captureLifecycle     profilerCaptureLifecycle
	captureSource        string
	captureBreach        string
	captureSourceFailure string
	allRowsFailClosed    bool
}

type profilerPairRowCensus struct {
	total  int
	byLane map[string]int
}

type profilerPairCensusSet [pairRenderKindCount]profilerPairRowCensus

var profilerCaptureKinds = [...]pairRenderKind{
	pairRenderMMC,
	pairRenderF2FS,
	pairRenderBlock,
}

type profilerPairProofDomain struct {
	maxObservations int64
	maxLaneKeys     int64
	observations    int64
	laneKeys        int64
	failureReason   string
}

type profilerBlockLaneClock struct {
	seq  int
	tsNS uint64
}

// profilerPairRowMapping is the immutable publisher-side semantic provenance
// for one pair-critical row. These five fields must match on readback so kind,
// lane and structured endpoint ownership cannot drift. The sole typed run
// manifest proves every physical spill byte, including tsNS, line and ordinary
// rows which intentionally do not enter this pair-only map.
type profilerPairRowMapping struct {
	kind               pairRenderKind
	lane               string
	table              string
	structuredPair     bool
	profilerEventField int
}

type profilerCaptureLifecycle uint8

const (
	profilerCaptureInactive profilerCaptureLifecycle = iota
	profilerCaptureOpen
	profilerCaptureSealed
)

func defaultTraceDBRowSinkOptions() traceDBRowSinkOptions {
	return traceDBRowSinkOptions{
		bufferBytes:    defaultTraceDBRowBufferBytes,
		maxRunRowBytes: defaultTraceDBMaxPhysicalRunRowBytes,
		mergeFanIn:     defaultTraceDBRowMergeFanIn,
		activeTempCap:  defaultTraceDBActiveTempBytes,
		liveTempCap:    defaultTraceDBLiveTempBytes,
		ops: traceDBRowSinkOps{
			createTemp: os.CreateTemp,
			open:       os.Open,
			stat:       os.Stat,
			remove:     os.Remove,
			removeAll:  os.RemoveAll,
		},
	}
}

func normalizeTraceDBRowSinkOptions(options traceDBRowSinkOptions) (traceDBRowSinkOptions, error) {
	defaults := defaultTraceDBRowSinkOptions()
	if options.bufferBytes == 0 {
		options.bufferBytes = defaults.bufferBytes
	}
	if options.maxRunRowBytes == 0 {
		options.maxRunRowBytes = defaults.maxRunRowBytes
	}
	if options.mergeFanIn == 0 {
		options.mergeFanIn = defaults.mergeFanIn
	}
	if options.activeTempCap == 0 {
		options.activeTempCap = defaults.activeTempCap
	}
	if options.liveTempCap == 0 {
		options.liveTempCap = defaults.liveTempCap
	}
	minimumLive, ok := checkedTraceDBUint64Add(options.activeTempCap, options.activeTempCap)
	if options.mergeFanIn < 2 || options.mergeFanIn > defaultTraceDBRowMergeFanIn ||
		options.bufferBytes == 0 || options.maxRunRowBytes == 0 || options.bufferBytes > options.activeTempCap ||
		options.maxRunRowBytes > uint64(math.MaxInt) || options.activeTempCap < options.maxRunRowBytes ||
		!ok || options.liveTempCap < minimumLive {
		return traceDBRowSinkOptions{}, &traceDBOutputInvariantError{Reason: "trace_row_sort_options_invalid"}
	}
	if options.ops.createTemp == nil {
		options.ops.createTemp = defaults.ops.createTemp
	}
	if options.ops.open == nil {
		options.ops.open = defaults.ops.open
	}
	if options.ops.stat == nil {
		options.ops.stat = defaults.ops.stat
	}
	if options.ops.remove == nil {
		options.ops.remove = defaults.ops.remove
	}
	if options.ops.removeAll == nil {
		options.ops.removeAll = defaults.ops.removeAll
	}
	return options, nil
}

func newTraceDBRowSink(tempDir string, threshold int) (*traceDBRowSink, error) {
	return newTraceDBRowSinkWithOptions(tempDir, threshold, traceDBRowSinkOptions{})
}

func newTraceDBRowSinkWithOptions(tempDir string, threshold int, options traceDBRowSinkOptions) (*traceDBRowSink, error) {
	if threshold <= 0 {
		threshold = defaultTraceDBRowSinkThreshold
	}
	normalized, err := normalizeTraceDBRowSinkOptions(options)
	if err != nil {
		return nil, err
	}
	sink := &traceDBRowSink{
		threshold: threshold, tempDir: tempDir, options: normalized, operationContext: context.Background(),
		artifacts: make(map[string]*traceDBTempArtifact),
		pairRows:  make(map[pairRenderKind]int), pairLaneRows: make(map[pairRenderKind]map[string]int),
		pairTableRows: make(map[pairRenderKind]map[string]map[string]int), poisoned: make(map[pairRenderKind]bool),
		pairTableTotals:      make(map[pairRenderKind]map[string]int),
		poisonedLanes:        make(map[pairRenderKind]map[string]bool),
		opaque:               make(map[pairRenderKind]bool),
		structuredPairRows:   make(map[pairRenderKind]int),
		structuredLaneRows:   make(map[pairRenderKind]map[string]int),
		structuredEventRows:  make(map[pairRenderKind]map[int]int),
		structuredEventLanes: make(map[pairRenderKind]map[int]map[string]int),
		legacyPairProof: profilerPairProofDomain{
			maxObservations: profilerPairBarrierMaxObservations,
			maxLaneKeys:     profilerPairBarrierMaxLaneKeys,
		},
		blockPairProof: profilerPairProofDomain{
			maxObservations: profilerPairBarrierMaxObservations,
			maxLaneKeys:     profilerPairBarrierMaxLaneKeys,
		},
		pairRowCapacity: profilerPairBarrierMaxObservations,
		pairRowMappings: make(map[int]profilerPairRowMapping),
		blockLaneClocks: make(map[string]profilerBlockLaneClock),
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

func (s *traceDBRowSink) bindContext(ctx context.Context) error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	if ctx == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_context_missing"}
	}
	if s.prepared || s.stats.RowsAccepted != 0 || len(s.rows) != 0 || len(s.runs) != 0 {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_context_bind_too_late"}
	}
	s.operationContext = ctx
	return nil
}

func (s *traceDBRowSink) openProfilerCapture(sourcePath string) error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_capture_sink_missing"}
	}
	if s.captureLifecycle != profilerCaptureInactive || s.captureSource != "" ||
		s.stats.RowsAccepted != 0 || len(s.rows) != 0 ||
		len(s.runs) != 0 || len(s.artifacts) != 0 || len(s.rowIngestOrdinals) != 0 || s.bufferedBytes != 0 ||
		s.nextIngestOrdinal != 0 || s.nextRunOrdinal != 0 || s.activeTempBytes != 0 || s.liveTempBytes != 0 ||
		s.prepared || s.prepareFailure != nil || s.pairCensusActive || s.captureSourceFailure != "" {
		return &traceDBOutputInvariantError{Reason: "profiler_capture_open_state_invalid"}
	}
	if strings.TrimSpace(sourcePath) == "" {
		return &traceDBOutputInvariantError{Reason: "profiler_capture_source_namespace_missing"}
	}
	abs, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("resolve profiler capture source namespace: %w", err)
	}
	abs = filepath.Clean(abs)
	physical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("resolve physical profiler capture source namespace: %w", err)
	}
	abs = filepath.Clean(physical)
	if abs == "" || abs == "." {
		return &traceDBOutputInvariantError{Reason: "profiler_capture_source_namespace_invalid"}
	}
	s.captureSource = abs
	s.captureLifecycle = profilerCaptureOpen
	return nil
}

func (s *traceDBRowSink) sealProfilerCapture() error {
	if s != nil && s.operationContext != nil {
		return s.sealProfilerCaptureContext(s.operationContext)
	}
	return s.sealProfilerCaptureContext(context.Background())
}

func (s *traceDBRowSink) sealProfilerCaptureContext(ctx context.Context) error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_capture_sink_missing"}
	}
	if s.captureLifecycle != profilerCaptureOpen {
		return &traceDBOutputInvariantError{Reason: "profiler_capture_seal_state_invalid"}
	}
	if s.captureSource == "" {
		s.recordProfilerCaptureBreach("profiler_capture_source_namespace_lost")
		s.captureLifecycle = profilerCaptureSealed
		return &traceDBOutputInvariantError{Reason: s.captureBreach}
	}
	if s.pairCensusActive {
		s.recordProfilerCaptureBreach("profiler_pair_census_open_at_seal")
		s.captureLifecycle = profilerCaptureSealed
		return &traceDBOutputInvariantError{Reason: s.captureBreach}
	}
	// Spill provenance is part of the capture proof, not an output-time best
	// effort check. Complete its readback before the caller is allowed to open
	// the destination artifact.
	if err := s.prepareForPublication(ctx); err != nil {
		if accountingErr := s.validateProfilerPairAccounting(); accountingErr != nil {
			s.recordProfilerCaptureBreach("profiler_capture_accounting_invalid")
		} else {
			s.recordProfilerCaptureBreach("profiler_pair_storage_validation_failed")
		}
		s.captureLifecycle = profilerCaptureSealed
		return err
	}
	if err := s.validateProfilerPairAccounting(); err != nil {
		s.recordProfilerCaptureBreach("profiler_capture_accounting_invalid")
		s.captureLifecycle = profilerCaptureSealed
		return err
	}
	if s.captureBreach != "" {
		s.captureLifecycle = profilerCaptureSealed
		return &traceDBOutputInvariantError{Reason: s.captureBreach}
	}
	s.captureLifecycle = profilerCaptureSealed
	return nil
}

func (s *traceDBRowSink) profilerMutationAllowed(reason string) bool {
	if s == nil {
		return false
	}
	if s.prepared {
		if s.captureLifecycle != profilerCaptureInactive {
			s.recordProfilerCaptureBreach(reason)
		}
		return false
	}
	if s.captureLifecycle != profilerCaptureSealed {
		return true
	}
	s.recordProfilerCaptureBreach(reason)
	return false
}

func (s *traceDBRowSink) recordProfilerCaptureBreach(reason string) {
	if s == nil {
		return
	}
	if reason == "" {
		reason = "profiler_capture_seal_breach"
	}
	if s.captureBreach == "" {
		s.captureBreach = reason
	}
	s.allRowsFailClosed = true
}

func (s *traceDBRowSink) add(row renderedRow) (result error) {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	defer func() { s.recordSorterFailure(result) }()
	if !s.profilerMutationAllowed("profiler_capture_add_after_seal") {
		reason := s.captureBreach
		if reason == "" {
			reason = "trace_row_sink_add_after_prepare"
		}
		return &traceDBOutputInvariantError{Reason: reason}
	}
	if !traceDBSinglePhysicalLine(row.line, false) {
		return &traceDBOutputInvariantError{Reason: "invalid_rendered_line"}
	}
	if !profilerPairKindValid(row.pairKind) {
		return &traceDBOutputInvariantError{Reason: "invalid_pair_render_kind"}
	}
	if err := validateProfilerEventFieldProvenance(row); err != nil {
		return err
	}
	if err := s.validateActivePairRowCapacity(row); err != nil {
		return err
	}
	if len(row.pairLane) > maxTraceDBSystraceLineBytes || len(row.pairTable) > maxTraceDBSystraceLineBytes ||
		!utf8.ValidString(row.pairLane) || !utf8.ValidString(row.pairTable) {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_metadata_too_large"}
	}
	// Own the exact bytes charged below. Substrings from a much larger source
	// buffer must not retain that backing allocation outside the checked budget.
	row.line = strings.Clone(row.line)
	row.pairLane = strings.Clone(row.pairLane)
	row.pairTable = strings.Clone(row.pairTable)
	rowBytes, ok := traceDBRenderedRowRetainedBytes(row)
	if !ok || s.stats.RowsAccepted == math.MaxInt || s.nextIngestOrdinal == math.MaxUint64 {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_buffer_accounting_overflow"}
	}
	projectedBytes, bytesOK := checkedTraceDBUint64Add(s.bufferedBytes, rowBytes)
	if !bytesOK {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_buffer_accounting_overflow"}
	}
	if len(s.rows) > 0 && projectedBytes > s.options.bufferBytes {
		if err := s.flushChunk(); err != nil {
			return err
		}
		projectedBytes = rowBytes
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
	if len(s.rows) == math.MaxInt {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_buffer_accounting_overflow"}
	}
	nextBufferedRows := len(s.rows) + 1
	if err := s.ensureBufferedCapacity(nextBufferedRows); err != nil {
		return err
	}
	if row.pairKind != pairRenderUnknown {
		s.auditProfilerPairPhysicalRow(row)
	}
	if s.stats.RowsAccepted == 0 || row.tsNS < s.stats.FirstTSNS {
		s.stats.FirstTSNS = row.tsNS
	}
	if row.tsNS > s.stats.LastTSNS {
		s.stats.LastTSNS = row.tsNS
	}
	s.rows = s.rows[:nextBufferedRows]
	s.rowIngestOrdinals = s.rowIngestOrdinals[:nextBufferedRows]
	s.rows[nextBufferedRows-1] = row
	s.rowIngestOrdinals[nextBufferedRows-1] = s.nextIngestOrdinal
	s.nextIngestOrdinal++
	s.bufferedBytes = projectedBytes
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
		trackLane := !s.poisoned[row.pairKind] &&
			(!profilerPairBudgetKind(row.pairKind) || s.observeProfilerPairState(row.pairKind, row.pairLane))
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
	if s.bufferedBytes > s.stats.PeakBufferedBytes {
		s.stats.PeakBufferedBytes = s.bufferedBytes
	}
	if len(s.rows) >= s.threshold || s.bufferedBytes >= s.options.bufferBytes {
		return s.flushChunk()
	}
	return nil
}

func traceDBRenderedRowRetainedBytes(row renderedRow) (uint64, bool) {
	total := traceDBBufferedRowMetadataBytes
	for _, size := range []int{len(row.line), len(row.pairLane), len(row.pairTable)} {
		var ok bool
		total, ok = checkedTraceDBUint64Add(total, uint64(size))
		if !ok {
			return 0, false
		}
	}
	return total, true
}

func traceDBBufferedCapacityBytes(capacity int) (uint64, bool) {
	if capacity < 0 {
		return 0, false
	}
	elementBytes := uint64(unsafe.Sizeof(renderedRow{}) + unsafe.Sizeof(uint64(0)))
	if elementBytes != 0 && uint64(capacity) > math.MaxUint64/elementBytes {
		return 0, false
	}
	total := uint64(capacity) * elementBytes
	return checkedTraceDBUint64Add(total,
		uint64(unsafe.Sizeof([]renderedRow(nil))+unsafe.Sizeof([]uint64(nil))))
}

// ensureBufferedCapacity is the sole backing-array allocator. Exact doubling
// makes the metadata bound independent of the runtime's append growth policy.
func (s *traceDBRowSink) ensureBufferedCapacity(needed int) error {
	if s == nil || needed <= 0 || len(s.rows) != len(s.rowIngestOrdinals) ||
		cap(s.rows) != cap(s.rowIngestOrdinals) || needed <= len(s.rows) {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_buffer_capacity_state_invalid"}
	}
	capacity := cap(s.rows)
	if needed <= capacity {
		return nil
	}
	newCapacity := 1
	if capacity > 0 {
		if capacity > math.MaxInt/2 {
			newCapacity = needed
		} else {
			newCapacity = capacity * 2
		}
	}
	if newCapacity < needed {
		newCapacity = needed
	}
	physicalBytes, ok := traceDBBufferedCapacityBytes(newCapacity)
	if !ok || uint64(needed) > math.MaxUint64/traceDBBufferedRowMetadataBytes ||
		physicalBytes > uint64(needed)*traceDBBufferedRowMetadataBytes {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_buffer_metadata_bound_invalid"}
	}
	rows := make([]renderedRow, len(s.rows), newCapacity)
	copy(rows, s.rows)
	ordinals := make([]uint64, len(s.rowIngestOrdinals), newCapacity)
	copy(ordinals, s.rowIngestOrdinals)
	s.rows = rows
	s.rowIngestOrdinals = ordinals
	return nil
}

func profilerPairKindValid(kind pairRenderKind) bool {
	return kind < pairRenderKindCount
}

func (s *traceDBRowSink) auditProfilerPairPhysicalRow(row renderedRow) {
	if s == nil || row.pairKind == pairRenderUnknown || s.pairAuthorityFailure != "" {
		return
	}
	if row.seq < 0 || row.pairKind == pairRenderBlock && row.pairLane == "" && !s.poisoned[row.pairKind] {
		s.failProfilerPairAuthority("published_row_mapping_missing")
		return
	}
	if int64(len(s.pairRowMappings)) >= s.pairRowCapacity {
		s.failProfilerPairAuthority("shared_row_capacity")
		return
	}
	if _, exists := s.pairRowMappings[row.seq]; exists {
		s.failProfilerPairAuthority("duplicate_published_seq")
		return
	}
	s.pairRowMappings[row.seq] = profilerPairRowMapping{
		kind:               row.pairKind,
		lane:               row.pairLane,
		table:              row.pairTable,
		structuredPair:     row.structuredPair,
		profilerEventField: row.profilerEventField,
	}
	if row.pairKind != pairRenderBlock || row.pairLane == "" || s.poisoned[pairRenderBlock] {
		return
	}
	if previous, found := s.blockLaneClocks[row.pairLane]; found {
		if row.seq <= previous.seq {
			s.failProfilerPairAuthority("block_physical_sequence_regression")
			return
		}
		if row.tsNS < previous.tsNS {
			s.poisonPairLaneRaw(pairRenderBlock, row.pairLane)
		}
	}
	s.blockLaneClocks[row.pairLane] = profilerBlockLaneClock{seq: row.seq, tsNS: row.tsNS}
}

func (mapping profilerPairRowMapping) matches(row renderedRow) bool {
	return mapping.kind == row.pairKind &&
		mapping.lane == row.pairLane &&
		mapping.table == row.pairTable &&
		mapping.structuredPair == row.structuredPair &&
		mapping.profilerEventField == row.profilerEventField
}

// validatePreparedProfilerPairStorage verifies pair mappings while the sole
// authenticated final run is read to complete EOF. It deliberately runs after
// family poison: shared sequence/mapping and physical-storage authority cannot
// be waived by a narrower publication decision.
func (s *traceDBRowSink) validatePreparedProfilerPairStorage(ctx context.Context) error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_pair_sink_missing"}
	}
	if s.stats.RowsAccepted == 0 {
		if len(s.runs) != 0 {
			return traceDBRunInputIntegrity(&traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_row_count_mismatch"})
		}
		return nil
	}
	if len(s.runs) != 1 {
		return traceDBRunInputIntegrity(&traceDBOutputInvariantError{Reason: "trace_row_sort_final_manifest_missing"})
	}
	mappingProofActive := s.pairAuthorityFailure == ""
	seen := make(map[int]struct{}, len(s.pairRowMappings))
	var readRows uint64
	var localizedDuplicateRows uint64
	failStorageAuthority := func(reason string, sourceWide bool) {
		s.failProfilerPairAuthority(reason)
		if sourceWide {
			// A missing/unassociated physical record cannot be distinguished
			// from a pair row whose seq and kind both drifted to ordinary.
			// Suppress the complete source; family-only poison would let that
			// forged ordinary row recreate the very pairing hole we guard.
			s.allRowsFailClosed = true
			if s.captureSourceFailure == "" {
				s.captureSourceFailure = profilerPairStorageIntegrityFailure
			}
		}
	}
	observe := func(row renderedRow) bool {
		if readRows == math.MaxUint64 {
			failStorageAuthority("published_row_storage_count_overflow", true)
			return false
		}
		readRows++
		if !mappingProofActive {
			return false
		}
		expected, mapped := s.pairRowMappings[row.seq]
		if !mapped {
			if row.pairKind != pairRenderUnknown {
				failStorageAuthority("published_row_mapping_missing", true)
			}
			return false
		}
		if _, duplicate := seen[row.seq]; duplicate {
			if localizedDuplicateRows == math.MaxUint64 {
				failStorageAuthority("published_row_storage_count_overflow", true)
				return true
			}
			localizedDuplicateRows++
			failStorageAuthority("duplicate_published_seq", false)
			return true
		}
		seen[row.seq] = struct{}{}
		if !expected.matches(row) {
			failStorageAuthority("published_row_mapping_mismatch", false)
		}
		return true
	}
	reader, err := s.openAuthenticatedRunReader(s.runs[0])
	if err != nil {
		return traceDBRunInputIntegrity(err)
	}
	var rowValidationErr error
	for {
		record, ok, readErr := reader.next(ctx)
		if readErr != nil {
			err = traceDBRunInputIntegrity(readErr)
			break
		}
		if !ok {
			break
		}
		mapped := observe(record.row)
		if !mapped && rowValidationErr == nil {
			rowValidationErr = validateProfilerEventFieldProvenance(record.row)
		}
	}
	err = errors.Join(err, traceDBRunInputIntegrity(reader.close()))
	if err != nil {
		return err
	}
	if rowValidationErr != nil {
		return rowValidationErr
	}
	if mappingProofActive && len(seen) != len(s.pairRowMappings) {
		failStorageAuthority("published_row_mapping_missing", true)
	}
	logicalRows := readRows
	if localizedDuplicateRows > logicalRows {
		failStorageAuthority("published_row_storage_count_invalid", true)
		logicalRows = math.MaxUint64
	} else {
		logicalRows -= localizedDuplicateRows
	}
	if logicalRows != uint64(s.stats.RowsAccepted) {
		failStorageAuthority("published_row_storage_count_mismatch", true)
	}
	return nil
}

type profilerStructuredPairEndpoint struct {
	kind  pairRenderKind
	field int
}

var profilerStructuredPairEndpointRoster = [...]profilerStructuredPairEndpoint{
	{kind: pairRenderBlock, field: 202},
	{kind: pairRenderBlock, field: 204},
	{kind: pairRenderBlock, field: 209},
	{kind: pairRenderBlock, field: 211},
	{kind: pairRenderF2FS, field: 4009},
	{kind: pairRenderF2FS, field: 4010},
	{kind: pairRenderF2FS, field: 4011},
	{kind: pairRenderF2FS, field: 4012},
	{kind: pairRenderMMC, field: 4015},
	{kind: pairRenderMMC, field: 4016},
}

func profilerStructuredPairEventField(kind pairRenderKind, field int) bool {
	for _, endpoint := range profilerStructuredPairEndpointRoster {
		if endpoint.kind == kind && endpoint.field == field {
			return true
		}
	}
	return false
}

func profilerStructuredPairEventFields(kind pairRenderKind) []int {
	fields := make([]int, 0, len(profilerStructuredPairEndpointRoster))
	for _, endpoint := range profilerStructuredPairEndpointRoster {
		if endpoint.kind == kind {
			fields = append(fields, endpoint.field)
		}
	}
	return fields
}

func validateProfilerEventFieldProvenance(row renderedRow) error {
	if !profilerPairKindValid(row.pairKind) {
		return &traceDBOutputInvariantError{Reason: "invalid_pair_render_kind"}
	}
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
	if s == nil || kind == pairRenderUnknown || !profilerPairKindValid(kind) {
		return
	}
	if !s.profilerMutationAllowed("profiler_capture_opaque_after_seal") {
		return
	}
	s.opaque[kind] = true
	if s.pairRows[kind] > 0 {
		s.poisonPairKindRaw(kind)
	}
}

func (s *traceDBRowSink) failCloseAllRows() {
	if s == nil {
		return
	}
	if !s.profilerMutationAllowed("profiler_capture_source_fail_close_after_seal") {
		return
	}
	s.allRowsFailClosed = true
	for _, kind := range []pairRenderKind{
		pairRenderWorkqueue, pairRenderDMAFence, pairRenderMMC, pairRenderF2FS, pairRenderBlock,
	} {
		s.opaque[kind] = true
		s.poisonPairKindRaw(kind)
	}
}

func (s *traceDBRowSink) poisonPairKind(kind pairRenderKind) {
	if s == nil || kind == pairRenderUnknown || !profilerPairKindValid(kind) {
		return
	}
	if !s.profilerMutationAllowed("profiler_capture_family_poison_after_seal") {
		return
	}
	s.poisonPairKindRaw(kind)
}

func (s *traceDBRowSink) poisonPairKindRaw(kind pairRenderKind) {
	if s == nil || kind == pairRenderUnknown || !profilerPairKindValid(kind) {
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
		if s.pairCensusActive {
			s.activePairCensus[kind].byLane = nil
		}
	}
}

func (s *traceDBRowSink) poisonPairLane(kind pairRenderKind, lane string) {
	if s == nil || kind == pairRenderUnknown || !profilerPairKindValid(kind) {
		return
	}
	if !s.profilerMutationAllowed("profiler_capture_lane_poison_after_seal") {
		return
	}
	if lane == "" {
		s.poisonPairKindRaw(kind)
		return
	}
	if s.poisoned[kind] {
		return
	}
	if profilerPairBudgetKind(kind) && !s.observeProfilerPairState(kind, lane) {
		return
	}
	s.poisonPairLaneRaw(kind, lane)
}

func (s *traceDBRowSink) poisonPairLaneRaw(kind pairRenderKind, lane string) {
	if s == nil || kind == pairRenderUnknown || !profilerPairKindValid(kind) || lane == "" || s.poisoned[kind] {
		return
	}
	if s.poisonedLanes[kind] == nil {
		s.poisonedLanes[kind] = make(map[string]bool)
	}
	s.poisonedLanes[kind][lane] = true
}

func profilerPairBudgetKind(kind pairRenderKind) bool {
	return kind == pairRenderMMC || kind == pairRenderF2FS || kind == pairRenderBlock
}

func (s *traceDBRowSink) profilerPairProofDomain(kind pairRenderKind) *profilerPairProofDomain {
	if s == nil || !profilerPairBudgetKind(kind) {
		return nil
	}
	if kind == pairRenderBlock {
		return &s.blockPairProof
	}
	return &s.legacyPairProof
}

func (s *traceDBRowSink) observeProfilerPairState(kind pairRenderKind, lane string) bool {
	domain := s.profilerPairProofDomain(kind)
	if domain == nil || s.poisoned[kind] || domain.failureReason != "" || s.pairAuthorityFailure != "" {
		return false
	}
	if domain.observations >= domain.maxObservations {
		s.failProfilerPairBudget(kind, "observations")
		return false
	}
	domain.observations++
	if lane == "" || s.pairLaneRows[kind][lane] > 0 || s.poisonedLanes[kind][lane] {
		return true
	}
	if domain.laneKeys >= domain.maxLaneKeys {
		s.failProfilerPairBudget(kind, "lane_keys")
		return false
	}
	domain.laneKeys++
	return true
}

func (s *traceDBRowSink) failProfilerPairBudget(kind pairRenderKind, reason string) {
	domain := s.profilerPairProofDomain(kind)
	if domain == nil || domain.failureReason != "" {
		return
	}
	domain.failureReason = reason
	if kind == pairRenderBlock {
		s.poisonPairKindRaw(pairRenderBlock)
		s.blockLaneClocks = nil
		return
	}
	// MMC and F2FS intentionally share one legacy proof domain. Exhausting it
	// closes both families, while the independent Block domain remains usable.
	s.poisonPairKindRaw(pairRenderMMC)
	s.poisonPairKindRaw(pairRenderF2FS)
}

func (s *traceDBRowSink) failProfilerPairAuthority(reason string) {
	if s == nil || reason == "" {
		return
	}
	if s.pairAuthorityFailure == "" {
		s.pairAuthorityFailure = reason
	}
	for _, kind := range []pairRenderKind{
		pairRenderWorkqueue, pairRenderDMAFence, pairRenderMMC, pairRenderF2FS, pairRenderBlock,
	} {
		s.poisonPairKindRaw(kind)
	}
	s.blockLaneClocks = nil
}

func (s *traceDBRowSink) pairKindPoisoned(kind pairRenderKind) bool {
	return s != nil && kind != pairRenderUnknown &&
		(s.poisoned[kind] || len(s.poisonedLanes[kind]) > 0)
}

func (s *traceDBRowSink) withheldPairRows() int {
	total, err := s.withheldPairRowsChecked()
	if err != nil {
		return 0
	}
	return total
}

func (s *traceDBRowSink) withheldPairRowsForKind(kind pairRenderKind) int {
	total, err := s.withheldPairRowsForKindChecked(kind)
	if err != nil {
		return 0
	}
	return total
}

func (s *traceDBRowSink) withheldPairRowsChecked() (int, error) {
	if s == nil {
		return 0, nil
	}
	total := 0
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		count, err := s.withheldPairRowsForKindChecked(kind)
		if err != nil || !checkedProfilerIntAddTo(&total, count) {
			return 0, &traceDBOutputInvariantError{Reason: "profiler_pair_withheld_counter_invalid"}
		}
	}
	return total, nil
}

func (s *traceDBRowSink) withheldPairRowsForKindChecked(kind pairRenderKind) (int, error) {
	if s == nil || kind == pairRenderUnknown || !profilerPairKindValid(kind) {
		return 0, nil
	}
	staged := s.pairRows[kind]
	if staged < 0 {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_pair_staged_counter_negative"}
	}
	if s.poisoned[kind] {
		return staged, nil
	}
	total := 0
	for lane, count := range s.pairLaneRows[kind] {
		if count < 0 {
			return 0, &traceDBOutputInvariantError{Reason: "profiler_pair_lane_counter_negative"}
		}
		if s.poisonedLanes[kind][lane] && !checkedProfilerIntAddTo(&total, count) {
			return 0, &traceDBOutputInvariantError{Reason: "profiler_pair_withheld_counter_overflow"}
		}
	}
	if total > staged {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_pair_withheld_exceeds_staged"}
	}
	return total, nil
}

func (s *traceDBRowSink) withheldStructuredPairRows() int {
	total := 0
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		count, err := s.withheldStructuredPairRowsForKindChecked(kind)
		if err != nil || !checkedProfilerIntAddTo(&total, count) {
			return 0
		}
	}
	return total
}

func (s *traceDBRowSink) withheldStructuredPairRowsForKind(kind pairRenderKind) int {
	total, err := s.withheldStructuredPairRowsForKindChecked(kind)
	if err != nil {
		return 0
	}
	return total
}

func (s *traceDBRowSink) withheldStructuredPairRowsForKindChecked(kind pairRenderKind) (int, error) {
	if s == nil || kind == pairRenderUnknown || !profilerPairKindValid(kind) {
		return 0, nil
	}
	staged := s.structuredPairRows[kind]
	if staged < 0 || staged > s.pairRows[kind] {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_structured_pair_staged_counter_invalid"}
	}
	if s.poisoned[kind] {
		return staged, nil
	}
	total := 0
	for lane, count := range s.structuredLaneRows[kind] {
		if count < 0 {
			return 0, &traceDBOutputInvariantError{Reason: "profiler_structured_pair_lane_counter_negative"}
		}
		if s.poisonedLanes[kind][lane] && !checkedProfilerIntAddTo(&total, count) {
			return 0, &traceDBOutputInvariantError{Reason: "profiler_structured_pair_withheld_counter_overflow"}
		}
	}
	if total > staged {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_structured_pair_withheld_exceeds_staged"}
	}
	return total, nil
}

// withheldStructuredPairRowsForEventField reports only structured profiler
// rows from one exact FtraceEvent field. Text-compatible rows may share the
// same rendered event name, but cannot enter this typed accounting lane.
func (s *traceDBRowSink) withheldStructuredPairRowsForEventField(kind pairRenderKind, field int) int {
	withheld, err := s.withheldStructuredPairRowsForEventFieldChecked(kind, field)
	if err != nil {
		return 0
	}
	return withheld
}

func (s *traceDBRowSink) withheldStructuredPairRowsForEventFieldChecked(kind pairRenderKind, field int) (int, error) {
	if s == nil || !profilerStructuredPairEventField(kind, field) {
		return 0, nil
	}
	totalForField := s.structuredEventRows[kind][field]
	if totalForField < 0 || totalForField > s.structuredPairRows[kind] {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_structured_event_staged_counter_invalid"}
	}
	if s.poisoned[kind] {
		return totalForField, nil
	}
	withheld := 0
	for lane, count := range s.structuredEventLanes[kind][field] {
		if count < 0 {
			return 0, &traceDBOutputInvariantError{Reason: "profiler_structured_event_lane_counter_negative"}
		}
		if !s.poisonedLanes[kind][lane] {
			continue
		}
		if !checkedProfilerIntAddTo(&withheld, count) {
			return 0, &traceDBOutputInvariantError{Reason: "profiler_structured_event_withheld_counter_overflow"}
		}
	}
	if withheld > totalForField {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_structured_event_withheld_exceeds_staged"}
	}
	return withheld, nil
}

func (s *traceDBRowSink) beginPairRowCensus() bool {
	if s == nil || s.pairCensusActive || !s.profilerMutationAllowed("profiler_capture_census_begin_after_seal") {
		return false
	}
	s.activePairCensus = profilerPairCensusSet{}
	s.pairCensusActive = true
	return true
}

func (s *traceDBRowSink) validateActivePairRowCapacity(row renderedRow) error {
	if s == nil || !s.pairCensusActive || !profilerPairBudgetKind(row.pairKind) {
		return nil
	}
	census := &s.activePairCensus[row.pairKind]
	if census.total == math.MaxInt {
		return &traceDBOutputInvariantError{Reason: "profiler_pair_census_total_overflow"}
	}
	if row.pairLane != "" && census.byLane != nil && census.byLane[row.pairLane] == math.MaxInt {
		return &traceDBOutputInvariantError{Reason: "profiler_pair_census_lane_overflow"}
	}
	return nil
}

func (s *traceDBRowSink) accountActivePairRow(row renderedRow, trackLane bool) {
	if s == nil || !s.pairCensusActive || !profilerPairBudgetKind(row.pairKind) {
		return
	}
	census := &s.activePairCensus[row.pairKind]
	census.total++
	if trackLane && row.pairLane != "" {
		if census.byLane == nil {
			census.byLane = make(map[string]int)
		}
		census.byLane[row.pairLane]++
	}
}

func (s *traceDBRowSink) currentPairRowCensus(kind pairRenderKind) profilerPairRowCensus {
	if s == nil || !s.pairCensusActive || !profilerPairBudgetKind(kind) {
		return profilerPairRowCensus{}
	}
	return s.activePairCensus[kind]
}

func (s *traceDBRowSink) endPairRowCensus() profilerPairCensusSet {
	if s == nil || !s.pairCensusActive {
		return profilerPairCensusSet{}
	}
	census := s.activePairCensus
	s.activePairCensus = profilerPairCensusSet{}
	s.pairCensusActive = false
	return census
}

func (s *traceDBRowSink) withheldPairRowsFromCensus(kind pairRenderKind, census profilerPairRowCensus) int {
	total, err := s.withheldPairRowsFromCensusChecked(kind, census)
	if err != nil {
		return 0
	}
	return total
}

func (s *traceDBRowSink) withheldPairRowsFromCensusChecked(kind pairRenderKind, census profilerPairRowCensus) (int, error) {
	if s == nil || kind == pairRenderUnknown || !profilerPairBudgetKind(kind) {
		return 0, nil
	}
	if census.total < 0 {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_pair_publisher_staged_counter_negative"}
	}
	if s.poisoned[kind] {
		return census.total, nil
	}
	total := 0
	for lane, count := range census.byLane {
		if count < 0 {
			return 0, &traceDBOutputInvariantError{Reason: "profiler_pair_publisher_lane_counter_negative"}
		}
		if s.poisonedLanes[kind][lane] && !checkedProfilerIntAddTo(&total, count) {
			return 0, &traceDBOutputInvariantError{Reason: "profiler_pair_publisher_withheld_counter_overflow"}
		}
	}
	if total > census.total {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_pair_publisher_withheld_exceeds_staged"}
	}
	return total, nil
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
	count, err := s.profilerPublishableRows()
	if err != nil {
		return 0
	}
	return count
}

func (s *traceDBRowSink) profilerPublishableRows() (int, error) {
	if s == nil {
		return 0, nil
	}
	if err := s.validateProfilerPairAccounting(); err != nil {
		return 0, err
	}
	if s.allRowsFailClosed {
		return 0, nil
	}
	withheld, err := s.withheldPairRowsChecked()
	if err != nil {
		return 0, err
	}
	return s.stats.RowsAccepted - withheld, nil
}

func (s *traceDBRowSink) validateProfilerPairAccounting() error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_pair_sink_missing"}
	}
	if s.stats.RowsAccepted < 0 {
		return &traceDBOutputInvariantError{Reason: "profiler_rows_accepted_negative"}
	}
	pairRows := 0
	withheldRows := 0
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		staged := s.pairRows[kind]
		if staged < 0 || !checkedProfilerIntAddTo(&pairRows, staged) {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_staged_counter_invalid"}
		}
		withheld, err := s.withheldPairRowsForKindChecked(kind)
		if err != nil || !checkedProfilerIntAddTo(&withheldRows, withheld) {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_withheld_counter_invalid"}
		}
		structuredWithheld, err := s.withheldStructuredPairRowsForKindChecked(kind)
		if err != nil || structuredWithheld > withheld {
			return &traceDBOutputInvariantError{Reason: "profiler_structured_pair_withheld_account_invalid"}
		}
		structuredEvents := 0
		for _, field := range profilerStructuredPairEventFields(kind) {
			count := s.structuredEventRows[kind][field]
			if count < 0 || !checkedProfilerIntAddTo(&structuredEvents, count) {
				return &traceDBOutputInvariantError{Reason: "profiler_structured_event_staged_counter_invalid"}
			}
		}
		if structuredEvents != s.structuredPairRows[kind] {
			return &traceDBOutputInvariantError{Reason: "profiler_structured_event_staged_account_mismatch"}
		}
	}
	if pairRows > s.stats.RowsAccepted || withheldRows > pairRows || withheldRows > s.stats.RowsAccepted {
		return &traceDBOutputInvariantError{Reason: "profiler_pair_cross_total_invalid"}
	}
	for _, domain := range []*profilerPairProofDomain{&s.legacyPairProof, &s.blockPairProof} {
		if domain.maxObservations <= 0 || domain.maxLaneKeys <= 0 || domain.observations < 0 ||
			domain.laneKeys < 0 || domain.observations > domain.maxObservations || domain.laneKeys > domain.maxLaneKeys {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_proof_domain_invalid"}
		}
	}
	if s.pairAuthorityFailure != "" {
		for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
			if !s.poisoned[kind] {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_authority_fail_close_incomplete"}
			}
		}
	}
	if s.captureSourceFailure != "" &&
		(!s.allRowsFailClosed || s.pairAuthorityFailure == "") {
		return &traceDBOutputInvariantError{Reason: "profiler_capture_source_failure_account_invalid"}
	}
	if !s.allRowsFailClosed && s.stats.RowsAccepted-withheldRows < 0 {
		return &traceDBOutputInvariantError{Reason: "profiler_publishable_rows_negative"}
	}
	return nil
}

func (s *traceDBRowSink) rowPublishable(row renderedRow) bool {
	if s == nil || s.allRowsFailClosed {
		return false
	}
	if s.pairAuthorityFailure != "" {
		if _, governed := s.pairRowMappings[row.seq]; governed {
			return false
		}
	}
	return row.pairKind == pairRenderUnknown ||
		(profilerPairKindValid(row.pairKind) && !s.poisoned[row.pairKind] &&
			(row.pairLane == "" || !s.poisonedLanes[row.pairKind][row.pairLane]))
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

func (s *traceDBRowSink) recordSorterFailure(err error) {
	if s == nil || s.stats.FailureReason != "" || err == nil {
		return
	}
	reason, ok := traceDBOutputInvariantReason(err)
	if ok && strings.HasPrefix(reason, "trace_row_sort_") {
		s.stats.FailureReason = reason
	}
}

func (s *traceDBRowSink) accumulateElapsed(start time.Time) error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	elapsed := traceDBCoverageElapsedUS(start)
	if elapsed == 0 {
		return nil
	}
	if s.stats.ElapsedUS < 0 || elapsed > math.MaxInt64-s.stats.ElapsedUS {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_elapsed_overflow"}
	}
	s.stats.ElapsedUS += elapsed
	return nil
}

func (s *traceDBRowSink) validateRunManifestSet(requireFinal bool) error {
	if s == nil || s.stats.RowsAccepted < 0 || s.nextIngestOrdinal != uint64(s.stats.RowsAccepted) ||
		len(s.rows) != 0 || len(s.rowIngestOrdinals) != 0 || s.bufferedBytes != 0 || s.openRunFDs != 0 {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_state_invalid"}
	}
	if requireFinal && s.stats.RowsAccepted > 0 && len(s.runs) != 1 {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_final_manifest_missing"}
	}
	if requireFinal && len(s.runs) == 1 &&
		(s.stats.MergePasses < 0 || uint64(s.stats.MergePasses) > math.MaxUint32 ||
			s.runs[0].level != uint32(s.stats.MergePasses)) {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_final_manifest_level_mismatch"}
	}
	if s.stats.RowsAccepted == 0 && len(s.runs) != 0 {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_row_count_mismatch"}
	}
	seenPaths := make(map[string]struct{}, len(s.runs))
	seenOrdinals := make(map[uint64]struct{}, len(s.runs))
	var totalSize uint64
	var totalRows uint64
	for _, manifest := range s.runs {
		artifact := s.artifacts[manifest.path]
		if manifest.path == "" || manifest.size == 0 || manifest.rowCount == 0 ||
			manifest.ordinal >= s.nextRunOrdinal || artifact == nil || artifact.removed {
			return &traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_invalid"}
		}
		if _, duplicate := seenPaths[manifest.path]; duplicate {
			return &traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_path_duplicate"}
		}
		if _, duplicate := seenOrdinals[manifest.ordinal]; duplicate {
			return &traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_ordinal_duplicate"}
		}
		seenPaths[manifest.path] = struct{}{}
		seenOrdinals[manifest.ordinal] = struct{}{}
		var ok bool
		totalSize, ok = checkedTraceDBUint64Add(totalSize, manifest.size)
		if !ok {
			return &traceDBOutputInvariantError{Reason: "trace_row_sort_run_size_overflow"}
		}
		totalRows, ok = checkedTraceDBUint64Add(totalRows, manifest.rowCount)
		if !ok {
			return &traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_row_count_overflow"}
		}
	}
	if totalRows != uint64(s.stats.RowsAccepted) || totalSize != s.activeTempBytes ||
		totalSize != s.liveTempBytes || s.activeTempBytes > s.options.activeTempCap ||
		s.liveTempBytes > s.options.liveTempCap || s.stats.CurrentLiveTempBytes != s.liveTempBytes {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_accounting_mismatch"}
	}
	return nil
}

func (s *traceDBRowSink) markRegisteredRunStorageIntegrityFailure() {
	if s == nil {
		return
	}
	s.failProfilerPairAuthority("profiler_pair_spill_integrity_mismatch")
	s.allRowsFailClosed = true
	if s.captureSourceFailure == "" {
		s.captureSourceFailure = profilerPairStorageIntegrityFailure
	}
}

func (s *traceDBRowSink) accountAllRowsFailClosed() error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	if !s.allRowsFailClosed {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_all_rows_fail_close_state_invalid"}
	}
	s.stats.RowsWritten = 0
	s.stats.RowsWithheld = s.stats.RowsAccepted
	s.stats.FirstTSNS = 0
	s.stats.LastTSNS = 0
	return s.validateProfilerWrittenAccounting()
}

// accountPreparedNoPublication finalizes the public accounting when every
// accepted row is withheld by source-wide or family/lane authority. Profiler
// callers intentionally skip creating an output artifact in this state, so
// writeTo cannot be relied on to close Accepted=Written+Withheld.
func (s *traceDBRowSink) accountPreparedNoPublication() error {
	if s == nil || !s.prepared {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_no_publication_state_invalid"}
	}
	publishable, err := s.profilerPublishableRows()
	if err != nil {
		return err
	}
	if publishable != 0 {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_no_publication_rows_remain"}
	}
	s.stats.RowsWritten = 0
	s.stats.RowsWithheld = s.stats.RowsAccepted
	s.stats.FirstTSNS = 0
	s.stats.LastTSNS = 0
	return s.validateProfilerWrittenAccounting()
}

func (s *traceDBRowSink) finishRegisteredRunStorageFailure(err error) error {
	if !traceDBRunInputIntegrityOnly(err) {
		return err
	}
	s.markRegisteredRunStorageIntegrityFailure()
	if s.captureLifecycle == profilerCaptureOpen {
		// Profiler has a typed result envelope. Freeze the source-wide empty
		// publication so the caller can surface SourceFailClosed to the customer.
		s.prepared = true
		return s.accountAllRowsFailClosed()
	}
	if accountingErr := s.accountAllRowsFailClosed(); accountingErr != nil {
		return accountingErr
	}
	return &traceDBOutputInvariantError{Reason: profilerPairStorageIntegrityFailure}
}

func (s *traceDBRowSink) prepareForPublication(ctx context.Context) (result error) {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	defer func() { s.recordSorterFailure(result) }()
	if ctx == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_context_missing"}
	}
	if s.prepared {
		return s.prepareFailure
	}
	if s.prepareFailure != nil {
		return s.prepareFailure
	}
	if err := ctx.Err(); err != nil {
		s.prepareFailure = err
		return err
	}
	started := time.Now()
	defer func() {
		elapsedErr := s.accumulateElapsed(started)
		if elapsedErr != nil {
			if result == nil {
				s.prepareFailure = elapsedErr
			}
			result = traceDBJoinPreservingSingle(result, elapsedErr)
		}
	}()
	if err := s.flushChunkContext(ctx); err != nil {
		s.prepareFailure = err
		return err
	}
	if err := s.validateRunManifestSet(false); err != nil {
		if handled := s.finishRegisteredRunStorageFailure(err); handled != nil {
			s.prepareFailure = handled
			return handled
		}
		return nil
	}
	if err := s.mergeRunsLeveled(ctx); err != nil {
		if handled := s.finishRegisteredRunStorageFailure(err); handled != nil {
			s.prepareFailure = handled
			return handled
		}
		return nil
	}
	if err := s.validateRunManifestSet(true); err != nil {
		if handled := s.finishRegisteredRunStorageFailure(err); handled != nil {
			s.prepareFailure = handled
			return handled
		}
		return nil
	}
	if err := s.validatePreparedProfilerPairStorage(ctx); err != nil {
		if handled := s.finishRegisteredRunStorageFailure(err); handled != nil {
			s.prepareFailure = handled
			return handled
		}
		return nil
	}
	if s.captureSourceFailure != "" {
		if accountingErr := s.accountAllRowsFailClosed(); accountingErr != nil {
			s.prepareFailure = accountingErr
			return accountingErr
		}
		if s.captureLifecycle != profilerCaptureOpen {
			err := &traceDBOutputInvariantError{Reason: profilerPairStorageIntegrityFailure}
			s.prepareFailure = err
			return err
		}
	}
	s.prepared = true
	if s.allRowsFailClosed {
		return s.accountAllRowsFailClosed()
	}
	if publishable, err := s.profilerPublishableRows(); err != nil {
		s.prepareFailure = err
		return err
	} else if publishable == 0 {
		if err := s.accountPreparedNoPublication(); err != nil {
			s.prepareFailure = err
			return err
		}
	}
	return nil
}

func (s *traceDBRowSink) writeTo(ctx context.Context, w io.Writer) (stats traceDBRowSortStats, err error) {
	if s == nil {
		return traceDBRowSortStats{}, &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	if ctx == nil {
		return s.stats, &traceDBOutputInvariantError{Reason: "trace_row_sort_context_missing"}
	}
	if w == nil {
		return s.stats, &traceDBOutputInvariantError{Reason: "trace_row_sort_output_missing"}
	}
	if s.captureLifecycle == profilerCaptureOpen {
		return s.stats, &traceDBOutputInvariantError{Reason: "profiler_capture_write_before_seal"}
	}
	if s.captureLifecycle == profilerCaptureSealed && s.captureBreach != "" {
		return s.stats, &traceDBOutputInvariantError{Reason: s.captureBreach}
	}
	if !s.prepared || s.prepareFailure != nil {
		return s.stats, &traceDBOutputInvariantError{Reason: "trace_row_sort_write_before_prepare"}
	}
	if err := ctx.Err(); err != nil {
		return s.stats, err
	}
	if err := s.validateProfilerPairAccounting(); err != nil {
		return s.stats, err
	}
	withheld, err := s.withheldPairRowsChecked()
	if err != nil {
		return s.stats, err
	}
	start := time.Now()
	defer func() {
		err = traceDBJoinPreservingSingle(err, s.accumulateElapsed(start), s.cleanup())
		s.recordSorterFailure(err)
		stats = s.stats
	}()
	s.stats.RowsWritten = 0
	s.stats.RowsWithheld = withheld
	s.stats.FirstTSNS = 0
	s.stats.LastTSNS = 0
	if s.allRowsFailClosed {
		if err := s.accountAllRowsFailClosed(); err != nil {
			return s.stats, err
		}
		// Profiler container captures have a sealed, customer-visible
		// SourceFailClosed disclosure. Inactive sinks (notably SQL export) have
		// no equivalent result envelope, so storage-integrity suppression must
		// return a typed error after accounting and before the first write. A
		// silent empty success would be indistinguishable from a valid no-row
		// query.
		if s.captureLifecycle == profilerCaptureInactive && s.captureSourceFailure != "" {
			return s.stats, &traceDBOutputInvariantError{Reason: profilerPairStorageIntegrityFailure}
		}
		return s.stats, nil
	}
	bw := bufio.NewWriterSize(w, 256*1024)
	if err := writeSystraceHeader(bw); err != nil {
		return s.stats, err
	}
	if s.stats.RowsAccepted == 0 {
		if len(s.runs) != 0 {
			return s.stats, &traceDBOutputInvariantError{Reason: "trace_row_sort_final_manifest_invalid"}
		}
		if err := bw.Flush(); err != nil {
			return s.stats, err
		}
		return s.stats, s.validateProfilerWrittenAccounting()
	}
	if len(s.runs) != 1 {
		return s.stats, &traceDBOutputInvariantError{Reason: "trace_row_sort_final_manifest_invalid"}
	}
	reader, openErr := s.openAuthenticatedRunReader(s.runs[0])
	if openErr != nil {
		return s.stats, traceDBRunInputIntegrity(openErr)
	}
	var readErr error
	for {
		record, ok, nextErr := reader.next(ctx)
		if nextErr != nil {
			readErr = traceDBRunInputIntegrity(nextErr)
			break
		}
		if !ok {
			break
		}
		if !s.rowPublishable(record.row) {
			continue
		}
		if _, writeErr := bw.WriteString(record.row.line); writeErr != nil {
			readErr = writeErr
			break
		}
		if writeErr := bw.WriteByte('\n'); writeErr != nil {
			readErr = writeErr
			break
		}
		s.accountWrittenRow(record.row)
	}
	readErr = errors.Join(readErr, traceDBRunInputIntegrity(reader.close()))
	if readErr != nil {
		return s.stats, readErr
	}
	if err := bw.Flush(); err != nil {
		return s.stats, err
	}
	return s.stats, s.validateProfilerWrittenAccounting()
}

func (s *traceDBRowSink) validateProfilerWrittenAccounting() error {
	if s == nil || s.stats.RowsAccepted < 0 || s.stats.RowsWritten < 0 || s.stats.RowsWithheld < 0 ||
		s.stats.RowsWritten > s.stats.RowsAccepted || s.stats.RowsWithheld > s.stats.RowsAccepted {
		return &traceDBOutputInvariantError{Reason: "profiler_written_account_invalid"}
	}
	total := s.stats.RowsWritten
	if !checkedProfilerIntAddTo(&total, s.stats.RowsWithheld) || total != s.stats.RowsAccepted {
		return &traceDBOutputInvariantError{Reason: "profiler_rows_accepted_written_withheld_mismatch"}
	}
	return nil
}

func (s *traceDBRowSink) flushChunk() (result error) {
	if s == nil || s.operationContext == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_context_missing"}
	}
	started := time.Now()
	defer func() {
		if elapsedErr := s.accumulateElapsed(started); elapsedErr != nil {
			result = traceDBJoinPreservingSingle(result, elapsedErr)
		}
	}()
	return s.flushChunkContext(s.operationContext)
}

type traceDBBufferedRunRow struct {
	row           renderedRow
	ingestOrdinal uint64
}

type traceDBBufferedRows struct {
	rows     []renderedRow
	ordinals []uint64
}

func (rows traceDBBufferedRows) Len() int { return len(rows.rows) }

func (rows traceDBBufferedRows) Less(i, j int) bool {
	return traceDBRunRowLess(
		traceDBBufferedRunRow{row: rows.rows[i], ingestOrdinal: rows.ordinals[i]},
		traceDBBufferedRunRow{row: rows.rows[j], ingestOrdinal: rows.ordinals[j]},
	)
}

func (rows traceDBBufferedRows) Swap(i, j int) {
	rows.rows[i], rows.rows[j] = rows.rows[j], rows.rows[i]
	rows.ordinals[i], rows.ordinals[j] = rows.ordinals[j], rows.ordinals[i]
}

func traceDBRunRowLess(left, right traceDBBufferedRunRow) bool {
	if left.row.tsNS != right.row.tsNS {
		return left.row.tsNS < right.row.tsNS
	}
	if left.row.seq != right.row.seq {
		return left.row.seq < right.row.seq
	}
	return left.ingestOrdinal < right.ingestOrdinal
}

func traceDBChunkRowFor(row traceDBBufferedRunRow) traceDBChunkRow {
	return traceDBChunkRow{
		TSNS: row.row.tsNS, Seq: row.row.seq, IngestOrdinal: row.ingestOrdinal, Line: row.row.line,
		PairKind: row.row.pairKind, PairLane: row.row.pairLane, PairTable: row.row.pairTable,
		StructuredPair: row.row.structuredPair, ProfilerEventField: row.row.profilerEventField,
	}
}

func (s *traceDBRowSink) runFault(point, path string) error {
	if s == nil || s.options.ops.fault == nil {
		return nil
	}
	return traceDBSorterOperationError(point, s.options.ops.fault(point, path))
}

func (s *traceDBRowSink) updateLiveTempStats() {
	if s == nil {
		return
	}
	s.stats.CurrentLiveTempBytes = s.liveTempBytes
	if s.liveTempBytes > s.stats.PeakLiveTempBytes {
		s.stats.PeakLiveTempBytes = s.liveTempBytes
	}
}

func (s *traceDBRowSink) reservePendingRun(size uint64, leaf bool) error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	if leaf {
		active, ok := checkedTraceDBUint64Add(s.activeTempBytes, size)
		if !ok || active > s.options.activeTempCap {
			return &traceDBOutputInvariantError{Reason: "trace_row_sort_active_temp_budget_exceeded"}
		}
	}
	live, ok := checkedTraceDBUint64Add(s.liveTempBytes, size)
	if !ok || live > s.options.liveTempCap {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_live_temp_budget_exceeded"}
	}
	if size > uint64(math.MaxInt64) || s.stats.TempBytes < 0 ||
		uint64(s.stats.TempBytes) > uint64(math.MaxInt64)-size {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_cumulative_temp_overflow"}
	}
	s.liveTempBytes = live
	s.updateLiveTempStats()
	return nil
}

func (s *traceDBRowSink) releasePendingRun(size uint64) error {
	if s == nil || size > s.liveTempBytes {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_live_temp_account_invalid"}
	}
	s.liveTempBytes -= size
	s.updateLiveTempStats()
	return nil
}

func (s *traceDBRowSink) noteRunFDOpen() error {
	if s == nil || s.openRunFDs == math.MaxInt {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_open_fd_counter_overflow"}
	}
	s.openRunFDs++
	if s.openRunFDs > s.stats.PeakOpenRunFDs {
		s.stats.PeakOpenRunFDs = s.openRunFDs
	}
	return nil
}

func (s *traceDBRowSink) noteRunFDClose() error {
	if s == nil || s.openRunFDs <= 0 {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_open_fd_counter_invalid"}
	}
	s.openRunFDs--
	return nil
}

func (s *traceDBRowSink) closeRunFile(file *os.File, path string) error {
	if file == nil {
		return nil
	}
	err := s.runFault("close", path)
	err = errors.Join(err, traceDBSorterOperationError("close", file.Close()), s.noteRunFDClose())
	return err
}

func (s *traceDBRowSink) registerArtifact(path string) error {
	if s == nil || path == "" {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_artifact_path_invalid"}
	}
	if _, exists := s.artifacts[path]; exists {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_artifact_duplicate"}
	}
	s.artifacts[path] = &traceDBTempArtifact{path: path}
	return nil
}

func (s *traceDBRowSink) removeArtifact(path string) error {
	artifact := s.artifacts[path]
	if artifact == nil || artifact.removed {
		return nil
	}
	if err := s.runFault("remove", path); err != nil {
		return err
	}
	err := s.options.ops.remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return traceDBSorterOperationError("remove", err)
	}
	artifact.removed = true
	return nil
}

func (s *traceDBRowSink) createPendingRun(prefix string) (*os.File, string, error) {
	if err := s.runFault("create", s.tempDir); err != nil {
		return nil, "", err
	}
	file, err := s.options.ops.createTemp(s.tempDir, prefix)
	if err != nil {
		return nil, "", traceDBSorterOperationError("create", err)
	}
	path := file.Name()
	if err := s.registerArtifact(path); err != nil {
		// A duplicate path belongs to the already-registered authority. Close
		// this handle but never unlink that path through a second identity.
		return nil, path, errors.Join(err, traceDBSorterOperationError("close", file.Close()))
	}
	if err := s.noteRunFDOpen(); err != nil {
		return nil, path, errors.Join(err, traceDBSorterOperationError("close", file.Close()),
			s.removeArtifact(path))
	}
	return file, path, nil
}

func (s *traceDBRowSink) openManifestRun(manifest traceDBRunManifest) (*os.File, error) {
	if err := s.runFault("open", manifest.path); err != nil {
		return nil, err
	}
	file, err := s.options.ops.open(manifest.path)
	if err != nil {
		return nil, traceDBSorterOperationError("open", err)
	}
	if err := s.noteRunFDOpen(); err != nil {
		return nil, errors.Join(err, traceDBSorterOperationError("close", file.Close()))
	}
	return file, nil
}

func (s *traceDBRowSink) discardPendingRun(file *os.File, path string, reserved uint64, primary error) error {
	if file != nil {
		primary = errors.Join(primary, s.closeRunFile(file, path))
	}
	removeErr := s.removeArtifact(path)
	primary = errors.Join(primary, removeErr)
	if removeErr == nil {
		primary = errors.Join(primary, s.releasePendingRun(reserved))
	}
	return primary
}

func (s *traceDBRowSink) flushChunkContext(ctx context.Context) error {
	if len(s.rows) == 0 {
		return nil
	}
	if s.prepared {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_flush_after_prepare"}
	}
	if len(s.rows) != len(s.rowIngestOrdinals) {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_buffer_ordinal_mismatch"}
	}
	sort.Sort(traceDBBufferedRows{rows: s.rows, ordinals: s.rowIngestOrdinals})
	var encodedSize uint64
	for index, row := range s.rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		runRow := traceDBBufferedRunRow{row: row, ingestOrdinal: s.rowIngestOrdinals[index]}
		raw, err := json.Marshal(traceDBChunkRowFor(runRow))
		if err != nil {
			return traceDBSorterOperationError("encode", err)
		}
		physicalSize, ok := checkedTraceDBUint64Add(uint64(len(raw)), 1)
		if !ok || physicalSize > s.options.maxRunRowBytes {
			return &traceDBOutputInvariantError{Reason: "trace_row_sort_physical_record_too_large"}
		}
		encodedSize, ok = checkedTraceDBUint64Add(encodedSize, physicalSize)
		if !ok {
			return &traceDBOutputInvariantError{Reason: "trace_row_sort_run_size_overflow"}
		}
	}
	if s.nextRunOrdinal == math.MaxUint64 {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_run_ordinal_overflow"}
	}
	if s.stats.SpillChunks == math.MaxInt {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_spill_chunk_counter_overflow"}
	}
	if err := s.reservePendingRun(encodedSize, true); err != nil {
		return err
	}
	f, path, err := s.createPendingRun("rows-leaf-*.jsonl")
	if err != nil {
		return errors.Join(err, s.releasePendingRun(encodedSize))
	}
	digest := sha256.New()
	bw := bufio.NewWriterSize(io.MultiWriter(f, digest), 256*1024)
	for index, row := range s.rows {
		if err := ctx.Err(); err != nil {
			return s.discardPendingRun(f, path, encodedSize, err)
		}
		runRow := traceDBBufferedRunRow{row: row, ingestOrdinal: s.rowIngestOrdinals[index]}
		raw, marshalErr := json.Marshal(traceDBChunkRowFor(runRow))
		if marshalErr != nil {
			return s.discardPendingRun(f, path, encodedSize,
				traceDBSorterOperationError("encode", marshalErr))
		}
		if faultErr := s.runFault("write", path); faultErr != nil {
			return s.discardPendingRun(f, path, encodedSize, faultErr)
		}
		if _, writeErr := bw.Write(raw); writeErr != nil {
			return s.discardPendingRun(f, path, encodedSize,
				traceDBSorterOperationError("write", writeErr))
		}
		if writeErr := bw.WriteByte('\n'); writeErr != nil {
			return s.discardPendingRun(f, path, encodedSize,
				traceDBSorterOperationError("write", writeErr))
		}
	}
	if faultErr := s.runFault("flush", path); faultErr != nil {
		return s.discardPendingRun(f, path, encodedSize, faultErr)
	}
	if err := bw.Flush(); err != nil {
		return s.discardPendingRun(f, path, encodedSize, traceDBSorterOperationError("flush", err))
	}
	if err := s.closeRunFile(f, path); err != nil {
		return s.discardPendingRun(nil, path, encodedSize, err)
	}
	f = nil
	if err := s.runFault("stat", path); err != nil {
		return s.discardPendingRun(nil, path, encodedSize, err)
	}
	info, err := s.options.ops.stat(path)
	if err != nil {
		return s.discardPendingRun(nil, path, encodedSize, traceDBSorterOperationError("stat", err))
	}
	if info.Size() < 0 || uint64(info.Size()) != encodedSize {
		return s.discardPendingRun(nil, path, encodedSize,
			&traceDBOutputInvariantError{Reason: "trace_row_sort_leaf_size_mismatch"})
	}
	var chunkDigest [sha256.Size]byte
	copy(chunkDigest[:], digest.Sum(nil))
	manifest := traceDBRunManifest{
		path: path, size: encodedSize, rowCount: uint64(len(s.rows)), digest: chunkDigest,
		level: 0, ordinal: s.nextRunOrdinal,
	}
	s.nextRunOrdinal++
	s.runs = append(s.runs, manifest)
	s.activeTempBytes += encodedSize
	s.stats.SpillChunks++
	s.stats.TempBytes += int64(encodedSize)
	clear(s.rows)
	s.rows = nil
	s.rowIngestOrdinals = nil
	s.bufferedBytes = 0
	return nil
}

func (s *traceDBRowSink) cleanup() error {
	if s == nil {
		return nil
	}
	var result error
	for path, artifact := range s.artifacts {
		if artifact == nil || artifact.removed {
			continue
		}
		result = errors.Join(result, s.removeArtifact(path))
	}
	if s.ownDir != "" {
		if faultErr := s.runFault("remove_all", s.ownDir); faultErr != nil {
			result = errors.Join(result, faultErr)
		} else if err := s.options.ops.removeAll(s.ownDir); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, traceDBSorterOperationError("remove_all", err))
		}
	}
	if result == nil {
		s.runs = nil
		s.artifacts = nil
		s.rows = nil
		s.rowIngestOrdinals = nil
		s.activeTempBytes = 0
		s.liveTempBytes = 0
		s.openRunFDs = 0
		s.updateLiveTempStats()
	}
	s.recordSorterFailure(result)
	return result
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
	IngestOrdinal      uint64         `json:"ingest_ordinal"`
	Line               string         `json:"line"`
	PairKind           pairRenderKind `json:"pair_kind,omitempty"`
	PairLane           string         `json:"pair_lane,omitempty"`
	PairTable          string         `json:"pair_table,omitempty"`
	StructuredPair     bool           `json:"structured_pair,omitempty"`
	ProfilerEventField int            `json:"profiler_event_field,omitempty"`
}

type traceDBChunkWireRow struct {
	TSNS               *uint64        `json:"ts_ns"`
	Seq                *int           `json:"seq"`
	IngestOrdinal      *uint64        `json:"ingest_ordinal"`
	Line               *string        `json:"line"`
	PairKind           pairRenderKind `json:"pair_kind,omitempty"`
	PairLane           string         `json:"pair_lane,omitempty"`
	PairTable          string         `json:"pair_table,omitempty"`
	StructuredPair     bool           `json:"structured_pair,omitempty"`
	ProfilerEventField int            `json:"profiler_event_field,omitempty"`
}

type traceDBRunRecord struct {
	row           renderedRow
	ingestOrdinal uint64
	raw           []byte
}

func traceDBRunRecordLess(left, right traceDBRunRecord) bool {
	return traceDBRunRowLess(
		traceDBBufferedRunRow{row: left.row, ingestOrdinal: left.ingestOrdinal},
		traceDBBufferedRunRow{row: right.row, ingestOrdinal: right.ingestOrdinal},
	)
}

func traceDBRunRecordKeyEqual(left, right traceDBRunRecord) bool {
	return left.row.tsNS == right.row.tsNS && left.row.seq == right.row.seq &&
		left.ingestOrdinal == right.ingestOrdinal
}

func readTraceDBBoundedJSONLRecord(reader *bufio.Reader, maximum uint64) ([]byte, bool, error) {
	if reader == nil || maximum == 0 || maximum > uint64(math.MaxInt) {
		return nil, false, &traceDBOutputInvariantError{Reason: "trace_row_sort_record_limit_invalid"}
	}
	record := make([]byte, 0, min(int(maximum), 256*1024))
	for {
		fragment, err := reader.ReadSlice('\n')
		projected, ok := checkedTraceDBUint64Add(uint64(len(record)), uint64(len(fragment)))
		if !ok || projected > maximum {
			return nil, false, &traceDBOutputInvariantError{Reason: "trace_row_sort_physical_record_too_large"}
		}
		record = append(record, fragment...)
		switch {
		case err == nil:
			if len(record) == 0 || record[len(record)-1] != '\n' {
				return nil, false, &traceDBOutputInvariantError{Reason: "trace_row_sort_record_not_terminated"}
			}
			return record, true, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(record) == 0:
			return nil, false, nil
		case errors.Is(err, io.EOF):
			return nil, false, &traceDBOutputInvariantError{Reason: "trace_row_sort_record_truncated"}
		default:
			return nil, false, err
		}
	}
}

func decodeTraceDBRunRecord(raw []byte, maximumIngest uint64) (traceDBRunRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire traceDBChunkWireRow
	if err := decoder.Decode(&wire); err != nil {
		return traceDBRunRecord{}, traceDBSorterOperationError("decode", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return traceDBRunRecord{}, &traceDBOutputInvariantError{Reason: "trace_row_sort_record_has_trailing_value"}
		}
		return traceDBRunRecord{}, traceDBSorterOperationError("decode", err)
	}
	if wire.TSNS == nil || wire.Seq == nil || wire.IngestOrdinal == nil || wire.Line == nil {
		return traceDBRunRecord{}, &traceDBOutputInvariantError{Reason: "trace_row_sort_record_required_field_missing"}
	}
	if *wire.IngestOrdinal >= maximumIngest {
		return traceDBRunRecord{}, &traceDBOutputInvariantError{Reason: "trace_row_sort_ingest_ordinal_out_of_range"}
	}
	row := renderedRow{
		tsNS: *wire.TSNS, seq: *wire.Seq, line: *wire.Line, pairKind: wire.PairKind,
		pairLane: wire.PairLane, pairTable: wire.PairTable, structuredPair: wire.StructuredPair,
		profilerEventField: wire.ProfilerEventField,
	}
	if !traceDBSinglePhysicalLine(row.line, false) || !profilerPairKindValid(row.pairKind) ||
		len(row.pairLane) > maxTraceDBSystraceLineBytes || len(row.pairTable) > maxTraceDBSystraceLineBytes ||
		!utf8.ValidString(row.pairLane) || !utf8.ValidString(row.pairTable) {
		return traceDBRunRecord{}, &traceDBOutputInvariantError{Reason: "trace_row_sort_record_invalid"}
	}
	return traceDBRunRecord{row: row, ingestOrdinal: *wire.IngestOrdinal, raw: raw}, nil
}

type traceDBAuthenticatedRunReader struct {
	sink     *traceDBRowSink
	manifest traceDBRunManifest
	file     *os.File
	reader   *bufio.Reader
	proof    hash.Hash
	rowsRead uint64
	previous traceDBRunRecord
	havePrev bool
	verified bool
}

func (s *traceDBRowSink) openAuthenticatedRunReader(manifest traceDBRunManifest) (*traceDBAuthenticatedRunReader, error) {
	if manifest.path == "" || manifest.size == 0 || manifest.rowCount == 0 {
		return nil, &traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_invalid"}
	}
	if err := s.runFault("stat", manifest.path); err != nil {
		return nil, err
	}
	info, err := s.options.ops.stat(manifest.path)
	if err != nil {
		return nil, traceDBSorterOperationError("stat", err)
	}
	if info.Size() < 0 || uint64(info.Size()) != manifest.size {
		return nil, &traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_size_mismatch"}
	}
	file, err := s.openManifestRun(manifest)
	if err != nil {
		return nil, err
	}
	return &traceDBAuthenticatedRunReader{
		sink: s, manifest: manifest, file: file,
		reader: bufio.NewReaderSize(file, 256*1024), proof: sha256.New(),
	}, nil
}

func (reader *traceDBAuthenticatedRunReader) next(ctx context.Context) (traceDBRunRecord, bool, error) {
	if reader == nil || reader.sink == nil || reader.verified {
		return traceDBRunRecord{}, false, &traceDBOutputInvariantError{Reason: "trace_row_sort_reader_state_invalid"}
	}
	if err := ctx.Err(); err != nil {
		return traceDBRunRecord{}, false, err
	}
	if err := reader.sink.runFault("read", reader.manifest.path); err != nil {
		return traceDBRunRecord{}, false, err
	}
	raw, ok, err := readTraceDBBoundedJSONLRecord(reader.reader, reader.sink.options.maxRunRowBytes)
	if err != nil {
		return traceDBRunRecord{}, false, traceDBSorterOperationError("read", err)
	}
	if !ok {
		if reader.rowsRead != reader.manifest.rowCount {
			return traceDBRunRecord{}, false, &traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_row_count_mismatch"}
		}
		var digest [sha256.Size]byte
		copy(digest[:], reader.proof.Sum(nil))
		if digest != reader.manifest.digest {
			return traceDBRunRecord{}, false, &traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_digest_mismatch"}
		}
		reader.verified = true
		return traceDBRunRecord{}, false, nil
	}
	if err := reader.sink.runFault("decode", reader.manifest.path); err != nil {
		return traceDBRunRecord{}, false, err
	}
	if _, err := reader.proof.Write(raw); err != nil {
		return traceDBRunRecord{}, false, traceDBSorterOperationError("read", err)
	}
	record, err := decodeTraceDBRunRecord(raw, uint64(reader.sink.stats.RowsAccepted))
	if err != nil {
		return traceDBRunRecord{}, false, err
	}
	if reader.havePrev && (!traceDBRunRecordLess(reader.previous, record) || traceDBRunRecordKeyEqual(reader.previous, record)) {
		return traceDBRunRecord{}, false, &traceDBOutputInvariantError{Reason: "trace_row_sort_run_order_invalid"}
	}
	if reader.rowsRead == math.MaxUint64 || reader.rowsRead >= reader.manifest.rowCount {
		return traceDBRunRecord{}, false, &traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_row_count_mismatch"}
	}
	reader.rowsRead++
	reader.previous = record
	reader.previous.raw = nil
	reader.havePrev = true
	return record, true, nil
}

func (reader *traceDBAuthenticatedRunReader) close() error {
	if reader == nil || reader.file == nil {
		return nil
	}
	err := reader.sink.closeRunFile(reader.file, reader.manifest.path)
	reader.file = nil
	return err
}

type traceDBRunMergeHeapItem struct {
	record      traceDBRunRecord
	readerIndex int
}

type traceDBRunMergeHeap []traceDBRunMergeHeapItem

func (heapRows traceDBRunMergeHeap) Len() int { return len(heapRows) }

func (heapRows traceDBRunMergeHeap) Less(i, j int) bool {
	if traceDBRunRecordKeyEqual(heapRows[i].record, heapRows[j].record) {
		return heapRows[i].readerIndex < heapRows[j].readerIndex
	}
	return traceDBRunRecordLess(heapRows[i].record, heapRows[j].record)
}

func (heapRows traceDBRunMergeHeap) Swap(i, j int) {
	heapRows[i], heapRows[j] = heapRows[j], heapRows[i]
}

func (heapRows *traceDBRunMergeHeap) Push(value any) {
	*heapRows = append(*heapRows, value.(traceDBRunMergeHeapItem))
}

func (heapRows *traceDBRunMergeHeap) Pop() any {
	old := *heapRows
	last := len(old) - 1
	item := old[last]
	*heapRows = old[:last]
	return item
}

func closeTraceDBAuthenticatedRunReaders(readers []*traceDBAuthenticatedRunReader) error {
	var result error
	for _, reader := range readers {
		result = errors.Join(result, reader.close())
	}
	return result
}

func (s *traceDBRowSink) commitMergedRunAuthority(group []traceDBRunManifest, output traceDBRunManifest) error {
	if s == nil || len(group) < 2 || output.path == "" {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_merge_commit_invalid"}
	}
	start := -1
	for candidate := 0; candidate+len(group) <= len(s.runs); candidate++ {
		matches := true
		for offset := range group {
			if s.runs[candidate+offset] != group[offset] {
				matches = false
				break
			}
		}
		if matches {
			if start >= 0 {
				return &traceDBOutputInvariantError{Reason: "trace_row_sort_merge_commit_ambiguous"}
			}
			start = candidate
		}
	}
	if start < 0 {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_merge_commit_input_missing"}
	}
	committed := make([]traceDBRunManifest, 0, len(s.runs)-len(group)+1)
	committed = append(committed, s.runs[:start]...)
	committed = append(committed, output)
	committed = append(committed, s.runs[start+len(group):]...)
	s.runs = committed
	return nil
}

func (s *traceDBRowSink) mergeRunGroup(ctx context.Context, group []traceDBRunManifest) (traceDBRunManifest, error) {
	if len(group) < 2 || len(group) > s.options.mergeFanIn {
		return traceDBRunManifest{}, &traceDBOutputInvariantError{Reason: "trace_row_sort_merge_group_invalid"}
	}
	var outputSize uint64
	var outputRows uint64
	var maximumLevel uint32
	for _, manifest := range group {
		var ok bool
		outputSize, ok = checkedTraceDBUint64Add(outputSize, manifest.size)
		if !ok {
			return traceDBRunManifest{}, &traceDBOutputInvariantError{Reason: "trace_row_sort_run_size_overflow"}
		}
		outputRows, ok = checkedTraceDBUint64Add(outputRows, manifest.rowCount)
		if !ok {
			return traceDBRunManifest{}, &traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_row_count_overflow"}
		}
		if manifest.level > maximumLevel {
			maximumLevel = manifest.level
		}
	}
	if outputSize == 0 || outputRows == 0 || maximumLevel == math.MaxUint32 ||
		s.nextRunOrdinal == math.MaxUint64 {
		return traceDBRunManifest{}, &traceDBOutputInvariantError{Reason: "trace_row_sort_merge_manifest_overflow"}
	}
	if err := s.reservePendingRun(outputSize, false); err != nil {
		return traceDBRunManifest{}, err
	}
	releaseReservation := func(primary error) error {
		return errors.Join(primary, s.releasePendingRun(outputSize))
	}
	readers := make([]*traceDBAuthenticatedRunReader, 0, len(group))
	mergeHeap := traceDBRunMergeHeap{}
	for index, manifest := range group {
		reader, err := s.openAuthenticatedRunReader(manifest)
		if err != nil {
			return traceDBRunManifest{}, releaseReservation(errors.Join(traceDBRunInputIntegrity(err),
				traceDBRunInputIntegrity(closeTraceDBAuthenticatedRunReaders(readers))))
		}
		readers = append(readers, reader)
		record, ok, err := reader.next(ctx)
		if err != nil {
			return traceDBRunManifest{}, releaseReservation(errors.Join(traceDBRunInputIntegrity(err),
				traceDBRunInputIntegrity(closeTraceDBAuthenticatedRunReaders(readers))))
		}
		if !ok {
			return traceDBRunManifest{}, releaseReservation(errors.Join(
				&traceDBOutputInvariantError{Reason: "trace_row_sort_manifest_empty"},
				traceDBRunInputIntegrity(closeTraceDBAuthenticatedRunReaders(readers))))
		}
		heap.Push(&mergeHeap, traceDBRunMergeHeapItem{record: record, readerIndex: index})
	}
	file, path, err := s.createPendingRun("rows-merge-*.jsonl")
	if err != nil {
		return traceDBRunManifest{}, releaseReservation(errors.Join(err,
			traceDBRunInputIntegrity(closeTraceDBAuthenticatedRunReaders(readers))))
	}
	digest := sha256.New()
	bufferedWriter := bufio.NewWriterSize(io.MultiWriter(file, digest), 256*1024)
	var rowsWritten uint64
	var bytesWritten uint64
	var previous traceDBRunRecord
	havePrevious := false
	var streamErr error
	for mergeHeap.Len() > 0 && streamErr == nil {
		if err := ctx.Err(); err != nil {
			streamErr = err
			break
		}
		item := heap.Pop(&mergeHeap).(traceDBRunMergeHeapItem)
		if havePrevious && (!traceDBRunRecordLess(previous, item.record) ||
			traceDBRunRecordKeyEqual(previous, item.record)) {
			streamErr = traceDBRunInputIntegrity(
				&traceDBOutputInvariantError{Reason: "trace_row_sort_merge_order_invalid"})
			break
		}
		if err := s.runFault("write", path); err != nil {
			streamErr = err
			break
		}
		written, err := bufferedWriter.Write(item.record.raw)
		if err != nil {
			streamErr = traceDBSorterOperationError("write", err)
			break
		}
		if written != len(item.record.raw) {
			streamErr = io.ErrShortWrite
			break
		}
		var ok bool
		bytesWritten, ok = checkedTraceDBUint64Add(bytesWritten, uint64(written))
		if !ok || rowsWritten == math.MaxUint64 {
			streamErr = &traceDBOutputInvariantError{Reason: "trace_row_sort_merge_count_overflow"}
			break
		}
		rowsWritten++
		previous = item.record
		previous.raw = nil
		havePrevious = true
		next, ok, err := readers[item.readerIndex].next(ctx)
		if err != nil {
			streamErr = traceDBRunInputIntegrity(err)
			break
		}
		if ok {
			heap.Push(&mergeHeap, traceDBRunMergeHeapItem{record: next, readerIndex: item.readerIndex})
		}
	}
	readerCloseErr := traceDBRunInputIntegrity(closeTraceDBAuthenticatedRunReaders(readers))
	if streamErr == nil {
		streamErr = readerCloseErr
	} else {
		streamErr = errors.Join(streamErr, readerCloseErr)
	}
	if streamErr == nil {
		streamErr = s.runFault("flush", path)
	}
	if streamErr == nil {
		streamErr = traceDBSorterOperationError("flush", bufferedWriter.Flush())
	}
	closeErr := s.closeRunFile(file, path)
	file = nil
	streamErr = errors.Join(streamErr, closeErr)
	if streamErr != nil {
		return traceDBRunManifest{}, s.discardPendingRun(nil, path, outputSize, streamErr)
	}
	if rowsWritten != outputRows || bytesWritten != outputSize {
		return traceDBRunManifest{}, s.discardPendingRun(nil, path, outputSize,
			&traceDBOutputInvariantError{Reason: "trace_row_sort_merge_output_count_mismatch"})
	}
	if err := s.runFault("stat", path); err != nil {
		return traceDBRunManifest{}, s.discardPendingRun(nil, path, outputSize, err)
	}
	info, err := s.options.ops.stat(path)
	if err != nil {
		return traceDBRunManifest{}, s.discardPendingRun(nil, path, outputSize,
			traceDBSorterOperationError("stat", err))
	}
	if info.Size() < 0 || uint64(info.Size()) != outputSize {
		return traceDBRunManifest{}, s.discardPendingRun(nil, path, outputSize,
			&traceDBOutputInvariantError{Reason: "trace_row_sort_merge_output_size_mismatch"})
	}
	var outputDigest [sha256.Size]byte
	copy(outputDigest[:], digest.Sum(nil))
	output := traceDBRunManifest{
		path: path, size: outputSize, rowCount: outputRows, digest: outputDigest,
		level: maximumLevel + 1, ordinal: s.nextRunOrdinal,
	}
	if outputSize > s.activeTempBytes || outputSize > s.liveTempBytes {
		return traceDBRunManifest{}, s.discardPendingRun(nil, path, outputSize,
			&traceDBOutputInvariantError{Reason: "trace_row_sort_temp_account_invalid"})
	}
	// Commit the authenticated replacement as the sole authority before any
	// input retirement. If removal then fails, the output remains authoritative
	// and every old artifact stays in the cleanup registry; publication still
	// fails, but no partial retirement can destroy the replacement proof.
	if err := s.commitMergedRunAuthority(group, output); err != nil {
		return traceDBRunManifest{}, s.discardPendingRun(nil, path, outputSize, err)
	}
	s.stats.TempBytes += int64(outputSize)
	s.nextRunOrdinal++
	for _, input := range group {
		if err := s.removeArtifact(input.path); err != nil {
			return output, err
		}
		if input.size > s.liveTempBytes {
			return output, &traceDBOutputInvariantError{Reason: "trace_row_sort_temp_account_invalid"}
		}
		s.liveTempBytes -= input.size
		s.updateLiveTempStats()
	}
	if s.liveTempBytes != s.activeTempBytes {
		return output, &traceDBOutputInvariantError{Reason: "trace_row_sort_temp_account_invalid"}
	}
	return output, nil
}

func (s *traceDBRowSink) mergeRunsLeveled(ctx context.Context) error {
	current := append([]traceDBRunManifest(nil), s.runs...)
	for len(current) > 1 {
		if s.stats.MergePasses == math.MaxInt {
			return &traceDBOutputInvariantError{Reason: "trace_row_sort_merge_pass_overflow"}
		}
		nextCapacity := len(current) / s.options.mergeFanIn
		if len(current)%s.options.mergeFanIn != 0 {
			nextCapacity++
		}
		next := make([]traceDBRunManifest, 0, nextCapacity)
		mergedThisPass := false
		for start := 0; start < len(current); {
			end := len(current)
			if len(current)-start > s.options.mergeFanIn {
				end = start + s.options.mergeFanIn
			}
			group := current[start:end]
			if len(group) == 1 {
				next = append(next, group[0])
			} else {
				merged, err := s.mergeRunGroup(ctx, group)
				if err != nil {
					return err
				}
				next = append(next, merged)
				mergedThisPass = true
			}
			start = end
		}
		if !mergedThisPass || len(next) >= len(current) {
			return &traceDBOutputInvariantError{Reason: "trace_row_sort_merge_did_not_progress"}
		}
		s.stats.MergePasses++
		current = next
		s.runs = append(s.runs[:0], current...)
	}
	s.runs = current
	return nil
}
