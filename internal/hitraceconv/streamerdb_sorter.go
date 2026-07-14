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
	// encoding/json escapes each byte of the sole stored line field by at most
	// six bytes. Keep additional room for keys and numeric punctuation.
	defaultTraceDBMaxPhysicalRunRowBytes uint64 = 24 << 20
)

// The controlled doubling allocator below guarantees cap < 2*len for every
// non-empty buffer. This compile-time proof includes both backing arrays and
// both slice headers; adding fields to traceDBStoredRow or changing an architecture
// cannot silently invalidate the fixed per-row charge.
const traceDBBufferedRowMetadataProofBytes = int(2*(unsafe.Sizeof(traceDBStoredRow{})+unsafe.Sizeof(uint64(0))) +
	unsafe.Sizeof([]traceDBStoredRow(nil)) + unsafe.Sizeof([]uint64(nil)))

var _ [int(traceDBBufferedRowMetadataBytes) - traceDBBufferedRowMetadataProofBytes]byte

const profilerPairStorageIntegrityFailure = "profiler_pair_storage_integrity_failure"

const (
	// Match the direct full-capture barrier. These are proof-state limits, not
	// input or output row limits: print and other non-pair rows remain unaffected.
	profilerPairBarrierMaxObservations int64 = 4_000_000
	profilerPairBarrierMaxLaneKeys     int64 = 1_000_000
)

type traceDBRowSortStats struct {
	RowsAccepted               int
	RowsWritten                int
	RowsWithheld               int
	PeakBufferedRows           int
	PeakBufferedBytes          uint64
	SpillChunks                int
	TempBytes                  int64
	CurrentLiveTempBytes       uint64
	PeakLiveTempBytes          uint64
	PeakOpenRunFDs             int
	MergePasses                int
	SourceSidecarLogicalBytes  uint64
	SourceSidecarPhysicalBytes uint64
	FirstTSNS                  uint64
	LastTSNS                   uint64
	ElapsedUS                  int64
	FailureReason              string
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
	// Never put a second integrity wrapper around an existing graph. In
	// particular, wrapping Join(integrity, cleanup-error) would make the exact
	// type check below misclassify a mixed failure as source corruption.
	if traceDBRunInputIntegrityPresent(err) {
		return err
	}
	return &traceDBRunInputIntegrityError{cause: err}
}

func traceDBRunInputIntegrityPresent(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(*traceDBRunInputIntegrityError); ok {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, branch := range joined.Unwrap() {
			if traceDBRunInputIntegrityPresent(branch) {
				return true
			}
		}
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return traceDBRunInputIntegrityPresent(wrapped.Unwrap())
	}
	return false
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
	if wrapped, ok := err.(*traceDBRunInputIntegrityError); ok {
		// A normal wrapper may contain errors.Join(invariant, OS-cause) from a
		// single authenticated read operation. Only reject a nested wrapper
		// whose cause already contains a separately classified integrity branch;
		// that shape can only be an attempted laundering of a mixed graph.
		return wrapped.cause != nil &&
			(!traceDBRunInputIntegrityPresent(wrapped.cause) ||
				traceDBRunInputIntegrityOnly(wrapped.cause))
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
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return traceDBRunInputIntegrityOnly(wrapped.Unwrap())
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
	truncate   func(*os.File, int64) error
	writeAt    func(*os.File, []byte, int64) (int, error)
	readAt     func(*os.File, []byte, int64) (int, error)
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
	case "create", "open", "stat", "fstat", "seek", "read", "decode", "write", "flush", "close", "remove", "remove_all", "encode":
		reason = "trace_row_sort_run_" + point + "_failed"
	default:
		if strings.HasPrefix(point, "sidecar_") {
			reason = "trace_row_sort_" + point + "_failed"
		}
	}
	return errors.Join(&traceDBOutputInvariantError{Reason: reason}, cause)
}

func (s traceDBRowSortStats) coverage() TraceDBCoverage {
	coverage := TraceDBCoverage{
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
	if s.SourceSidecarLogicalBytes != 0 || s.SourceSidecarPhysicalBytes != 0 {
		coverage.FieldSources["profiler_source_order_sidecar"] = fmt.Sprintf(
			"%d_logical_bytes+%d_physical_bytes",
			s.SourceSidecarLogicalBytes, s.SourceSidecarPhysicalBytes,
		)
	}
	return coverage
}

type traceDBRowSink struct {
	threshold            int
	tempDir              string
	ownDir               string
	rows                 []traceDBStoredRow
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
	poisoned             map[pairRenderKind]bool
	opaque               map[pairRenderKind]bool
	structuredPairRows   map[pairRenderKind]int
	pairFixedLedger      profilerPairFixedLedger
	pairLaneRegistries   [pairRenderKindCount]profilerPairLaneRegistry
	activePairCensus     profilerPairCensusSet
	pairCensusActive     bool
	activePairPublisher  profilerPairPublisherSlot
	textMessageActive    bool
	activeTextMessage    uint32
	activeTextRows       int
	nextTextMessage      uint32
	legacyPairProof      profilerPairProofDomain
	blockPairProof       profilerPairProofDomain
	pairAuthorityFailure string
	captureLifecycle     profilerCaptureLifecycle
	captureSource        string
	captureBreach        string
	captureSourceFailure string
	allRowsFailClosed    bool
	inactiveOrdinaryOnly bool
	profilerSourceProof  profilerSourceOrderProof
	sourceOrderSidecar   profilerSourceOrderSidecarManifest
}

type profilerPairRowCensus struct {
	total int
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

// traceDBProfilerEventDelta is the fixed-width pair proof mutation staged by
// one structured or strict-text FtraceEvent. A producer may build it freely,
// but only the sorter event transaction applies it after every cancellable
// validation, allocation, and completed-prefix spill has succeeded.
type traceDBProfilerEventDelta struct {
	opaqueKinds [pairRenderKindCount]bool
	poisonKinds [pairRenderKindCount]bool
	poisonLanes [pairRenderKindCount]string
}

func (delta traceDBProfilerEventDelta) apply(sink *traceDBRowSink) {
	if sink == nil {
		return
	}
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		if delta.opaqueKinds[kind] {
			sink.markPairCaptureOpaque(kind)
		}
	}
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		if delta.poisonKinds[kind] {
			sink.poisonPairKind(kind)
			continue
		}
		if lane := delta.poisonLanes[kind]; lane != "" {
			sink.poisonPairLane(kind, lane)
		}
	}
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
			truncate: func(file *os.File, size int64) error {
				return file.Truncate(size)
			},
			writeAt: func(file *os.File, data []byte, offset int64) (int, error) {
				return file.WriteAt(data, offset)
			},
			readAt: func(file *os.File, data []byte, offset int64) (int, error) {
				return file.ReadAt(data, offset)
			},
			remove:    os.Remove,
			removeAll: os.RemoveAll,
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
	if options.ops.truncate == nil {
		options.ops.truncate = defaults.ops.truncate
	}
	if options.ops.writeAt == nil {
		options.ops.writeAt = defaults.ops.writeAt
	}
	if options.ops.readAt == nil {
		options.ops.readAt = defaults.ops.readAt
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

// newTraceDBInactiveOrdinaryRowSink is the production constructor for SQL and
// generic sources. Those sources have no Profiler capture lifecycle and may
// retain only ordinary rows; pair provenance is accepted solely after an
// explicit Profiler capture open (or by source-neutral unit fixtures).
func newTraceDBInactiveOrdinaryRowSink(tempDir string, threshold int) (*traceDBRowSink, error) {
	sink, err := newTraceDBRowSink(tempDir, threshold)
	if err != nil {
		return nil, err
	}
	sink.inactiveOrdinaryOnly = true
	return sink, nil
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
		pairRows:  make(map[pairRenderKind]int), poisoned: make(map[pairRenderKind]bool),
		opaque: make(map[pairRenderKind]bool), structuredPairRows: make(map[pairRenderKind]int),
		legacyPairProof: profilerPairProofDomain{
			maxObservations: profilerPairBarrierMaxObservations,
			maxLaneKeys:     profilerPairBarrierMaxLaneKeys,
		},
		blockPairProof: profilerPairProofDomain{
			maxObservations: profilerPairBarrierMaxObservations,
			maxLaneKeys:     profilerPairBarrierMaxLaneKeys,
		},
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

func (s *traceDBRowSink) profilerCapturePreOpenStatePristine() bool {
	if s == nil || s.artifacts == nil || len(s.pairRows) != 0 || len(s.poisoned) != 0 ||
		len(s.opaque) != 0 || len(s.structuredPairRows) != 0 || !s.pairFixedLedger.pristine() ||
		s.legacyPairProof.observations != 0 || s.legacyPairProof.laneKeys != 0 ||
		s.legacyPairProof.failureReason != "" || s.blockPairProof.observations != 0 ||
		s.blockPairProof.laneKeys != 0 || s.blockPairProof.failureReason != "" ||
		s.sourceOrderSidecar != (profilerSourceOrderSidecarManifest{}) {
		return false
	}
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		registry := s.pairLaneRegistries[kind]
		census := s.activePairCensus[kind]
		if len(registry.byKey) != 0 || len(registry.keys) != 0 || len(registry.states) != 0 ||
			census.total != 0 {
			return false
		}
	}
	return true
}

func (s *traceDBRowSink) openProfilerCapture(sourcePath string) error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_capture_sink_missing"}
	}
	if s.inactiveOrdinaryOnly {
		return &traceDBOutputInvariantError{Reason: "profiler_capture_ordinary_sink_forbidden"}
	}
	if s.captureLifecycle != profilerCaptureInactive || s.captureSource != "" ||
		s.stats.RowsAccepted != 0 || len(s.rows) != 0 ||
		!s.profilerCapturePreOpenStatePristine() || len(s.runs) != 0 || len(s.artifacts) != 0 ||
		len(s.rowIngestOrdinals) != 0 || s.bufferedBytes != 0 ||
		s.nextIngestOrdinal != 0 || s.nextRunOrdinal != 0 || s.activeTempBytes != 0 || s.liveTempBytes != 0 ||
		s.prepared || s.prepareFailure != nil || s.pairCensusActive || s.textMessageActive ||
		s.activePairPublisher != profilerPairPublisherNone || s.activeTextMessage != 0 ||
		s.activeTextRows != 0 || s.nextTextMessage != 0 || s.captureBreach != "" ||
		s.captureSourceFailure != "" || s.pairAuthorityFailure != "" || s.allRowsFailClosed ||
		!s.profilerSourceProof.pristine() {
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
	s.profilerSourceProof.activate()
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
	if s.pairCensusActive || s.textMessageActive {
		s.recordProfilerCaptureBreach("profiler_pair_census_open_at_seal")
		s.captureLifecycle = profilerCaptureSealed
		return &traceDBOutputInvariantError{Reason: s.captureBreach}
	}
	if err := s.validateProfilerSourceOrderProof(); err != nil {
		s.recordProfilerCaptureBreach("profiler_source_order_proof_invalid")
		s.captureLifecycle = profilerCaptureSealed
		return err
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
	if s.captureLifecycle == profilerCaptureSealed {
		s.recordProfilerCaptureBreach(reason)
		return false
	}
	if s.profilerSourceProof.frozen || s.profilerSourceProof.retired {
		if s.captureLifecycle != profilerCaptureInactive {
			s.recordProfilerCaptureBreach("profiler_capture_mutation_after_source_freeze")
		}
		return false
	}
	return true
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

func (s *traceDBRowSink) add(row renderedRow) error {
	ctx := context.Background()
	if s != nil && s.operationContext != nil {
		ctx = s.operationContext
	}
	return s.addContext(ctx, row, nil, false)
}

// addProfilerEventContext defers a threshold-triggered spill until the next
// event (or the consumer tail). That makes the current event's linearization
// point the final ctx poll immediately before its fixed pair delta and row
// accounting commit; no cancellable I/O remains after that point.
func (s *traceDBRowSink) addProfilerEventContext(ctx context.Context, row renderedRow, delta traceDBProfilerEventDelta) error {
	return s.addContext(ctx, row, &delta, true)
}

func validateProfilerRowSequenceRange(seq *int, count int) error {
	if seq == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_row_sequence_missing"}
	}
	next := *seq
	if count < 0 || !checkedProfilerIntAddTo(&next, count) {
		return &traceDBOutputInvariantError{Reason: "profiler_row_sequence_invalid"}
	}
	return nil
}

// addSequencedProfilerEventContext is the sole Profiler row/sequence commit
// authority. It assigns the display sequence itself, lets addContext commit the
// fixed event delta, provenance, census and row after its final Context poll,
// then advances the caller sequence as the no-fail tail of the same event.
// Future source-order proof must join this linearization point rather than add
// another publisher-local sequence or digest transaction.
func (s *traceDBRowSink) addSequencedProfilerEventContext(ctx context.Context, seq *int,
	row renderedRow, delta traceDBProfilerEventDelta,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	if err := validateProfilerRowSequenceRange(seq, 1); err != nil {
		return err
	}
	if row.seq != 0 {
		return &traceDBOutputInvariantError{Reason: "profiler_row_sequence_preassigned"}
	}
	next := *seq
	if !checkedProfilerIntAddTo(&next, 1) {
		return &traceDBOutputInvariantError{Reason: "profiler_row_sequence_invalid"}
	}
	row.seq = *seq
	if err := s.addProfilerEventContext(ctx, row, delta); err != nil {
		return err
	}
	*seq = next
	return nil
}

func (s *traceDBRowSink) commitProfilerEventDeltaContext(ctx context.Context, delta traceDBProfilerEventDelta) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	if s.inactiveOrdinaryOnly {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_inactive_profiler_mutation"}
	}
	if !s.profilerMutationAllowed("profiler_capture_event_delta_after_seal") {
		reason := s.captureBreach
		if reason == "" {
			reason = "trace_row_sink_add_after_prepare"
		}
		return &traceDBOutputInvariantError{Reason: reason}
	}
	if delta != (traceDBProfilerEventDelta{}) {
		if err := s.preflightProfilerPairFixedMutation(nil, &delta); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	delta.apply(s)
	return nil
}

func (s *traceDBRowSink) flushTriggeredProfilerEventContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	if len(s.rows) < s.threshold && s.bufferedBytes < s.options.bufferBytes {
		return nil
	}
	return s.flushChunkWithContext(ctx)
}

func (s *traceDBRowSink) addContext(ctx context.Context, row renderedRow, eventDelta *traceDBProfilerEventDelta,
	deferTriggeredFlush bool,
) (result error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	defer func() {
		s.profilerSourceProof.abortPreparedRow()
		s.recordSorterFailure(result)
	}()
	if !s.profilerMutationAllowed("profiler_capture_add_after_seal") {
		reason := s.captureBreach
		if reason == "" {
			reason = "trace_row_sink_add_after_prepare"
		}
		return &traceDBOutputInvariantError{Reason: reason}
	}
	if s.inactiveOrdinaryOnly && (eventDelta != nil || !row.profilerNeutral() ||
		s.pairCensusActive || s.textMessageActive ||
		s.activePairPublisher != profilerPairPublisherNone || s.activeTextMessage != 0) {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_inactive_nonordinary_row"}
	}
	if err := s.validateProfilerSourceOrderProof(); err != nil {
		return err
	}
	if s.profilerSourceProof.frozen {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_proof_frozen"}
	}
	if deferTriggeredFlush && (len(s.rows) >= s.threshold || s.bufferedBytes >= s.options.bufferBytes) {
		if err := s.flushChunkWithContext(ctx); err != nil {
			return err
		}
	}
	lineValid, err := profilerSinglePhysicalLineStringContext(ctx, row.line, false)
	if err != nil {
		return err
	}
	if !lineValid {
		return &traceDBOutputInvariantError{Reason: "invalid_rendered_line"}
	}
	if !profilerPairKindValid(row.pairKind) {
		return &traceDBOutputInvariantError{Reason: "invalid_pair_render_kind"}
	}
	if !profilerPairBudgetKind(row.pairKind) && (row.pairLane != "" || row.pairTable != "") {
		return &traceDBOutputInvariantError{Reason: "profiler_nonbudget_pair_metadata_forbidden"}
	}
	if row.profilerLaneID != 0 {
		return &traceDBOutputInvariantError{Reason: "profiler_pair_lane_id_preassigned"}
	}
	if row.profilerPublisherSlot != profilerPairPublisherNone || row.profilerTextMessageOrdinal != 0 ||
		row.profilerProvenanceFlags != 0 {
		return &traceDBOutputInvariantError{Reason: "profiler_row_provenance_preassigned"}
	}
	if err := validateProfilerEventFieldProvenance(row); err != nil {
		return err
	}
	if err := s.stageProfilerPairRowProvenance(&row); err != nil {
		return err
	}
	if s.pairCensusActive && profilerPairBudgetKind(row.pairKind) &&
		s.activePairCensus[row.pairKind].total == math.MaxInt {
		return &traceDBOutputInvariantError{Reason: "profiler_pair_census_total_overflow"}
	}
	if len(row.pairLane) > maxTraceDBSystraceLineBytes || len(row.pairTable) > maxTraceDBSystraceLineBytes {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_metadata_too_large"}
	}
	laneValid, err := profilerUTF8StringValidContext(ctx, row.pairLane)
	if err != nil {
		return err
	}
	tableValid, err := profilerUTF8StringValidContext(ctx, row.pairTable)
	if err != nil {
		return err
	}
	if !laneValid || !tableValid {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_metadata_too_large"}
	}
	// Own the exact bytes charged below. Substrings from a much larger source
	// buffer must not retain that backing allocation outside the checked budget.
	row.line, err = profilerCloneStringContext(ctx, row.line)
	if err != nil {
		return err
	}
	row.pairLane, err = profilerCloneStringContext(ctx, row.pairLane)
	if err != nil {
		return err
	}
	if row.profilerEndpointSlot == profilerPairEndpointNone {
		row.pairTable, err = profilerCloneStringContext(ctx, row.pairTable)
		if err != nil {
			return err
		}
	}
	rowBytes, ok := traceDBStoredRowRetainedBytes(compactTraceDBStoredRow(row))
	if !ok || s.stats.RowsAccepted == math.MaxInt || s.nextIngestOrdinal == math.MaxUint64 {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_buffer_accounting_overflow"}
	}
	projectedBytes, bytesOK := checkedTraceDBUint64Add(s.bufferedBytes, rowBytes)
	if !bytesOK {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_buffer_accounting_overflow"}
	}
	if len(s.rows) > 0 && projectedBytes > s.options.bufferBytes {
		if err := s.flushChunkWithContext(ctx); err != nil {
			return err
		}
		projectedBytes = rowBytes
	}
	if len(s.rows) == math.MaxInt {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_buffer_accounting_overflow"}
	}
	if s.textMessageActive && s.activeTextRows == math.MaxInt {
		return &traceDBOutputInvariantError{Reason: "profiler_text_message_row_counter_overflow"}
	}
	nextBufferedRows := len(s.rows) + 1
	if err := s.ensureBufferedCapacity(nextBufferedRows); err != nil {
		return err
	}
	if row.pairKind != pairRenderUnknown ||
		eventDelta != nil && *eventDelta != (traceDBProfilerEventDelta{}) {
		if err := s.preflightProfilerPairFixedMutation(&row, eventDelta); err != nil {
			return err
		}
	}
	if s.profilerSourceProof.active {
		if err := s.profilerSourceProof.prepareRowContext(ctx, row, s.nextIngestOrdinal); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if eventDelta != nil {
		eventDelta.apply(s)
	}
	trackLane := false
	if row.pairKind != pairRenderUnknown {
		if s.opaque[row.pairKind] {
			s.poisonPairKind(row.pairKind)
		}
		trackLane = !s.poisoned[row.pairKind] &&
			(!profilerPairBudgetKind(row.pairKind) || s.observeProfilerPairStateOwned(row.pairKind, row.pairLane))
		if trackLane && profilerPairBudgetKind(row.pairKind) && row.pairLane != "" {
			laneID, found := s.pairLaneRegistries[row.pairKind].idFor(row.pairLane)
			if !found {
				s.failProfilerPairAuthority("pair_lane_registry_row_missing")
				trackLane = false
			} else {
				canonicalLane, canonicalFound := s.pairLaneRegistries[row.pairKind].key(laneID)
				if !canonicalFound {
					s.failProfilerPairAuthority("pair_lane_registry_key_missing")
					trackLane = false
				} else {
					row.profilerLaneID = laneID
					row.pairLane = canonicalLane
				}
			}
		}
		s.commitProfilerBlockLaneClock(row)
		// The Block clock can fail the source-wide authority after trackLane was
		// computed. That reset deliberately clears every exact-lane registry;
		// never let the stale pre-reset decision recreate a subordinate lane
		// account for the current withheld row.
		if s.poisoned[row.pairKind] || s.pairAuthorityFailure != "" {
			trackLane = false
		}
	}
	if row.pairKind != pairRenderUnknown {
		trackLane = s.commitProfilerPairFixedRow(row, trackLane)
	}
	if s.stats.RowsAccepted == 0 || row.tsNS < s.stats.FirstTSNS {
		s.stats.FirstTSNS = row.tsNS
	}
	if row.tsNS > s.stats.LastTSNS {
		s.stats.LastTSNS = row.tsNS
	}
	s.rows = s.rows[:nextBufferedRows]
	s.rowIngestOrdinals = s.rowIngestOrdinals[:nextBufferedRows]
	storedRow := compactTraceDBStoredRow(row)
	s.rows[nextBufferedRows-1] = storedRow
	s.rowIngestOrdinals[nextBufferedRows-1] = s.nextIngestOrdinal
	s.nextIngestOrdinal++
	s.bufferedBytes = projectedBytes
	s.stats.RowsAccepted++
	if s.profilerSourceProof.prepared {
		s.profilerSourceProof.commitPreparedRow(row.profilerProvenance())
	}
	if s.textMessageActive {
		s.activeTextRows++
	}
	if row.pairKind != pairRenderUnknown {
		s.pairRows[row.pairKind]++
		if s.pairCensusActive && profilerPairBudgetKind(row.pairKind) {
			s.activePairCensus[row.pairKind].total++
		}
		if row.structuredPair {
			s.structuredPairRows[row.pairKind]++
		}
	}
	if len(s.rows) > s.stats.PeakBufferedRows {
		s.stats.PeakBufferedRows = len(s.rows)
	}
	if s.bufferedBytes > s.stats.PeakBufferedBytes {
		s.stats.PeakBufferedBytes = s.bufferedBytes
	}
	if !deferTriggeredFlush && (len(s.rows) >= s.threshold || s.bufferedBytes >= s.options.bufferBytes) {
		return s.flushChunkWithContext(ctx)
	}
	return nil
}

func traceDBStoredRowRetainedBytes(row traceDBStoredRow) (uint64, bool) {
	total := traceDBBufferedRowMetadataBytes
	var ok bool
	lineBytes := len(row.line)
	total, ok = checkedTraceDBUint64Add(total, uint64(lineBytes))
	if !ok {
		return 0, false
	}
	return total, true
}

func traceDBBufferedCapacityBytes(capacity int) (uint64, bool) {
	if capacity < 0 {
		return 0, false
	}
	elementBytes := uint64(unsafe.Sizeof(traceDBStoredRow{}) + unsafe.Sizeof(uint64(0)))
	if elementBytes != 0 && uint64(capacity) > math.MaxUint64/elementBytes {
		return 0, false
	}
	total := uint64(capacity) * elementBytes
	return checkedTraceDBUint64Add(total,
		uint64(unsafe.Sizeof([]traceDBStoredRow(nil))+unsafe.Sizeof([]uint64(nil))))
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
	rows := make([]traceDBStoredRow, len(s.rows), newCapacity)
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

func (s *traceDBRowSink) commitProfilerBlockLaneClock(row renderedRow) {
	if s == nil || row.pairKind != pairRenderBlock || row.pairLane == "" ||
		s.pairAuthorityFailure != "" || s.poisoned[pairRenderBlock] {
		return
	}
	state, stateOK := s.pairLaneRegistries[pairRenderBlock].state(row.profilerLaneID)
	if !stateOK {
		s.failProfilerPairAuthority("block_lane_registry_missing")
		return
	}
	if state.blockClockSeen {
		if row.seq <= state.lastBlockSeq {
			s.failProfilerPairAuthority("block_physical_sequence_regression")
			return
		}
		if row.tsNS < state.lastBlockTSNS {
			s.poisonPairLaneRaw(pairRenderBlock, row.pairLane)
		}
	} else if state.lastBlockSeq != 0 || state.lastBlockTSNS != 0 {
		s.failProfilerPairAuthority("block_lane_registry_clock_residue")
		return
	}
	state.blockClockSeen = true
	state.lastBlockSeq = row.seq
	state.lastBlockTSNS = row.tsNS
}

func profilerStructuredPairEventField(kind pairRenderKind, field int) bool {
	slot, ok := profilerPairEndpointForStructuredField(field)
	if !ok {
		return false
	}
	descriptor, ok := slot.descriptor()
	return ok && descriptor.kind == kind
}

func profilerStructuredPairEventFields(kind pairRenderKind) []int {
	fields := make([]int, 0, len(profilerPairEndpointRoster))
	for _, endpoint := range profilerPairEndpointRoster {
		if endpoint.kind == kind && endpoint.structuredField != 0 {
			fields = append(fields, endpoint.structuredField)
		}
	}
	return fields
}

func (s *traceDBRowSink) stageProfilerPairRowProvenance(row *renderedRow) error {
	if row == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_row_provenance_missing"}
	}
	if !profilerPairKindValid(row.pairKind) {
		return &traceDBOutputInvariantError{Reason: "invalid_pair_render_kind"}
	}
	if s != nil && s.pairCensusActive {
		if !s.activePairPublisher.valid() {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_publisher_slot_invalid"}
		}
		if row.profilerPublisherSlot != profilerPairPublisherNone &&
			row.profilerPublisherSlot != s.activePairPublisher {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_publisher_slot_mismatch"}
		}
		row.profilerPublisherSlot = s.activePairPublisher
	}
	if s != nil && s.textMessageActive {
		if s.activeTextMessage == 0 || s.activePairPublisher == profilerPairPublisherNone ||
			s.activePairPublisher == profilerPairPublisherSession || row.structuredPair {
			return &traceDBOutputInvariantError{Reason: "profiler_text_message_provenance_invalid"}
		}
		row.profilerTextMessageOrdinal = s.activeTextMessage
		row.profilerProvenanceFlags = profilerPairRowProvenanceText
	} else if row.profilerTextMessageOrdinal != 0 ||
		row.profilerProvenanceFlags&profilerPairRowProvenanceText != 0 {
		return &traceDBOutputInvariantError{Reason: "profiler_text_message_provenance_outside_message"}
	}
	if row.structuredPair {
		slot, ok := profilerPairEndpointForStructuredField(row.profilerEventField)
		if !ok {
			return &traceDBOutputInvariantError{Reason: "profiler_structured_pair_event_slot_missing"}
		}
		descriptor, _ := slot.descriptor()
		if descriptor.kind != row.pairKind {
			return &traceDBOutputInvariantError{Reason: "profiler_event_field_pair_kind_mismatch"}
		}
		if row.profilerEndpointSlot != profilerPairEndpointNone && row.profilerEndpointSlot != slot {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_endpoint_slot_mismatch"}
		}
		if row.pairTable != "" && row.pairTable != descriptor.name {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_endpoint_table_mismatch"}
		}
		row.pairTable = descriptor.name
		row.profilerEndpointSlot = slot
		row.profilerProvenanceFlags = profilerPairRowProvenanceStructured
	} else if row.profilerEventField != 0 ||
		row.profilerProvenanceFlags&profilerPairRowProvenanceStructured != 0 {
		return &traceDBOutputInvariantError{Reason: "profiler_event_field_without_structured_pair"}
	}
	if profilerPairBudgetKind(row.pairKind) && !row.structuredPair {
		if row.profilerEndpointSlot == profilerPairEndpointNone &&
			s != nil && s.pairCensusActive && s.activePairPublisher != profilerPairPublisherNone {
			return &traceDBOutputInvariantError{Reason: "profiler_text_pair_endpoint_slot_missing"}
		}
		if row.profilerEndpointSlot == profilerPairEndpointNone && row.pairTable != "" {
			slot, ok := profilerPairEndpointForName(row.pairTable)
			if !ok {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_endpoint_table_unknown"}
			}
			row.profilerEndpointSlot = slot
		}
		if row.profilerEndpointSlot != profilerPairEndpointNone {
			descriptor, ok := row.profilerEndpointSlot.descriptor()
			if !ok || descriptor.kind != row.pairKind {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_endpoint_kind_mismatch"}
			}
			if row.pairTable != "" && row.pairTable != descriptor.name {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_endpoint_table_mismatch"}
			}
			row.pairTable = descriptor.name
		}
	}
	return validateProfilerPairRowProvenance(*row)
}

func validateProfilerPairRowProvenance(row renderedRow) error {
	provenance := row.profilerProvenance()
	if !provenance.valid() || provenance.PairKind != row.pairKind {
		return &traceDBOutputInvariantError{Reason: "profiler_row_provenance_invalid"}
	}
	if provenance.LaneID != 0 && row.pairLane == "" {
		return &traceDBOutputInvariantError{Reason: "profiler_row_provenance_lane_mismatch"}
	}
	if row.structuredPair != (provenance.Flags&profilerPairRowProvenanceStructured != 0) {
		return &traceDBOutputInvariantError{Reason: "profiler_row_provenance_structured_mismatch"}
	}
	if row.structuredPair {
		slot, ok := profilerPairEndpointForStructuredField(row.profilerEventField)
		if !ok || slot != provenance.EndpointSlot {
			return &traceDBOutputInvariantError{Reason: "profiler_row_provenance_event_mismatch"}
		}
	}
	if provenance.EndpointSlot != profilerPairEndpointNone && row.pairTable != "" {
		descriptor, ok := provenance.EndpointSlot.descriptor()
		if !ok || descriptor.name != row.pairTable {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_endpoint_table_mismatch"}
		}
	}
	return nil
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
	if s == nil || s.inactiveOrdinaryOnly || kind == pairRenderUnknown || !profilerPairKindValid(kind) {
		return
	}
	if !s.profilerMutationAllowed("profiler_capture_opaque_after_seal") {
		return
	}
	plan, ok := s.pairFixedLedger.planMarkOpaque(kind)
	if !ok {
		s.failProfilerPairFixedLedger("profiler_pair_fixed_ledger_opaque_invalid")
		return
	}
	plan.apply(&s.pairFixedLedger)
	s.opaque[kind] = true
	if s.pairRows[kind] > 0 {
		s.poisonPairKindRaw(kind)
	}
}

func (s *traceDBRowSink) failCloseAllRows() {
	if s == nil || s.inactiveOrdinaryOnly {
		return
	}
	if !s.profilerMutationAllowed("profiler_capture_source_fail_close_after_seal") {
		return
	}
	s.allRowsFailClosed = true
	for _, kind := range []pairRenderKind{
		pairRenderWorkqueue, pairRenderDMAFence, pairRenderMMC, pairRenderF2FS, pairRenderBlock,
	} {
		plan, ok := s.pairFixedLedger.planMarkOpaque(kind)
		if !ok {
			s.failProfilerPairFixedLedger("profiler_pair_fixed_ledger_opaque_invalid")
			return
		}
		plan.apply(&s.pairFixedLedger)
		s.opaque[kind] = true
		s.poisonPairKindRaw(kind)
	}
}

func (s *traceDBRowSink) poisonPairKind(kind pairRenderKind) {
	if s == nil || s.inactiveOrdinaryOnly || kind == pairRenderUnknown || !profilerPairKindValid(kind) {
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
	plan, ok := s.pairFixedLedger.planPoisonFamily(kind)
	if !ok {
		s.failProfilerPairFixedLedger("profiler_pair_fixed_ledger_family_poison_invalid")
		return
	}
	plan.apply(&s.pairFixedLedger)
	s.poisonPairKindLegacyRaw(kind)
}

func (s *traceDBRowSink) poisonPairKindLegacyRaw(kind pairRenderKind) {
	if s == nil || kind == pairRenderUnknown || !profilerPairKindValid(kind) {
		return
	}
	s.poisoned[kind] = true
	if profilerPairBudgetKind(kind) {
		// Whole-family publication reads the fixed ledger. Exact lane state is
		// no longer needed once every staged row is withheld.
		s.pairLaneRegistries[kind].reset()
	}
}

func (s *traceDBRowSink) poisonPairLane(kind pairRenderKind, lane string) {
	if s == nil || s.inactiveOrdinaryOnly || kind == pairRenderUnknown || !profilerPairKindValid(kind) {
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
	if !profilerPairBudgetKind(kind) {
		// Workqueue/DMA do not carry a compact exact-lane identity. A coarse
		// lane rejection therefore closes the whole typed family instead of
		// manufacturing a string-only quarantine that publication cannot see.
		s.poisonPairKindRaw(kind)
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
	if !profilerPairBudgetKind(kind) {
		s.poisonPairKindRaw(kind)
		return
	}
	id, ok := s.pairLaneRegistries[kind].idFor(lane)
	if !ok {
		s.failProfilerPairAuthority("pair_lane_registry_missing")
		return
	}
	state, ok := s.pairLaneRegistries[kind].state(id)
	if !ok {
		s.failProfilerPairAuthority("pair_lane_registry_state_missing")
		return
	}
	plan, ok := s.pairFixedLedger.planPoisonLane(kind, *state)
	if !ok {
		s.failProfilerPairFixedLedger("profiler_pair_fixed_ledger_lane_poison_invalid")
		return
	}
	plan.apply(&s.pairFixedLedger, state)
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
	return s.observeProfilerPairStateWithOwnership(kind, lane, false)
}

func (s *traceDBRowSink) observeProfilerPairStateOwned(kind pairRenderKind, lane string) bool {
	return s.observeProfilerPairStateWithOwnership(kind, lane, true)
}

func (s *traceDBRowSink) observeProfilerPairStateWithOwnership(kind pairRenderKind, lane string, owned bool) bool {
	domain := s.profilerPairProofDomain(kind)
	if domain == nil || s.poisoned[kind] || domain.failureReason != "" || s.pairAuthorityFailure != "" {
		return false
	}
	if domain.observations >= domain.maxObservations {
		s.failProfilerPairBudget(kind, "observations")
		return false
	}
	domain.observations++
	if lane == "" {
		return true
	}
	if _, found := s.pairLaneRegistries[kind].idFor(lane); found {
		return true
	}
	if domain.laneKeys >= domain.maxLaneKeys {
		s.failProfilerPairBudget(kind, "lane_keys")
		return false
	}
	domain.laneKeys++
	var ok bool
	if owned {
		_, ok = s.pairLaneRegistries[kind].internOwned(lane)
	} else {
		_, ok = s.pairLaneRegistries[kind].intern(lane)
	}
	if !ok {
		s.failProfilerPairAuthority("pair_lane_registry_capacity")
		return false
	}
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
}

func (s *traceDBRowSink) pairKindPoisoned(kind pairRenderKind) bool {
	if s == nil || kind == pairRenderUnknown || !profilerPairKindValid(kind) {
		return false
	}
	family, ok := s.pairFixedLedger.family(kind)
	if !ok || family.poisoned {
		return true
	}
	for _, state := range s.pairLaneRegistries[kind].states {
		if state.poisoned {
			return true
		}
	}
	return false
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
	family, ok := s.pairFixedLedger.family(kind)
	if !ok {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_invalid"}
	}
	return family.withheld, nil
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
	family, ok := s.pairFixedLedger.family(kind)
	if !ok {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_invalid"}
	}
	return family.structuredWithheld, nil
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
	slot, ok := profilerPairEndpointForStructuredField(field)
	if !ok {
		return 0, nil
	}
	counts, ok := s.pairFixedLedger.endpoint(slot)
	if !ok {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_invalid"}
	}
	return counts.structuredWithheld, nil
}

func (s *traceDBRowSink) beginPairRowCensus() bool {
	return s.beginPairRowCensusForPublisher(profilerPairPublisherNone)
}

func (s *traceDBRowSink) beginPairRowCensusForPublisher(publisher profilerPairPublisherSlot) bool {
	if s == nil || s.inactiveOrdinaryOnly || s.pairCensusActive ||
		!s.profilerMutationAllowed("profiler_capture_census_begin_after_seal") {
		return false
	}
	if !publisher.valid() {
		return false
	}
	s.activePairCensus = profilerPairCensusSet{}
	s.pairCensusActive = true
	s.activePairPublisher = publisher
	return true
}

func (s *traceDBRowSink) beginProfilerTextMessage() bool {
	if s == nil || !s.pairCensusActive || s.textMessageActive ||
		!s.activePairPublisher.textCapable() || s.nextTextMessage == math.MaxUint32 ||
		!s.profilerMutationAllowed("profiler_text_message_begin_after_seal") {
		return false
	}
	s.textMessageActive = true
	s.activeTextMessage = s.nextTextMessage + 1
	s.activeTextRows = 0
	return true
}

func (s *traceDBRowSink) endProfilerTextMessage(expectedRows int) error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_text_message_end_state_invalid"}
	}
	if s.inactiveOrdinaryOnly {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_inactive_profiler_mutation"}
	}
	if !s.textMessageActive || expectedRows < 0 || s.activeTextRows != expectedRows ||
		s.activeTextMessage == 0 || s.activeTextMessage != s.nextTextMessage+1 {
		s.recordProfilerCaptureBreach("profiler_text_message_end_state_invalid")
		s.abortProfilerTextMessage()
		return &traceDBOutputInvariantError{Reason: "profiler_text_message_end_state_invalid"}
	}
	if expectedRows > 0 {
		s.nextTextMessage = s.activeTextMessage
	}
	s.textMessageActive = false
	s.activeTextMessage = 0
	s.activeTextRows = 0
	return nil
}

func (s *traceDBRowSink) abortProfilerTextMessage() {
	if s == nil || s.inactiveOrdinaryOnly || !s.textMessageActive {
		return
	}
	if s.activeTextRows > 0 && s.activeTextMessage > s.nextTextMessage {
		s.nextTextMessage = s.activeTextMessage
	}
	s.textMessageActive = false
	s.activeTextMessage = 0
	s.activeTextRows = 0
}

func (s *traceDBRowSink) abortPairRowCensus() {
	if s == nil || s.inactiveOrdinaryOnly || !s.pairCensusActive {
		return
	}
	s.abortProfilerTextMessage()
	s.activePairCensus = profilerPairCensusSet{}
	s.pairCensusActive = false
	s.activePairPublisher = profilerPairPublisherNone
}

func (s *traceDBRowSink) endPairRowCensus() profilerPairCensusSet {
	if s == nil || s.inactiveOrdinaryOnly || !s.pairCensusActive {
		return profilerPairCensusSet{}
	}
	if s.textMessageActive {
		s.recordProfilerCaptureBreach("profiler_text_message_open_at_census_end")
		return profilerPairCensusSet{}
	}
	census := s.activePairCensus
	s.activePairCensus = profilerPairCensusSet{}
	s.pairCensusActive = false
	s.activePairPublisher = profilerPairPublisherNone
	return census
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
	if err := s.validateProfilerSourceOrderProof(); err != nil {
		return err
	}
	if err := s.validateProfilerPairLaneRegistryParity(); err != nil {
		return err
	}
	if s.stats.RowsAccepted < 0 {
		return &traceDBOutputInvariantError{Reason: "profiler_rows_accepted_negative"}
	}
	pairRows := 0
	withheldRows := 0
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		family, ok := s.pairFixedLedger.family(kind)
		if !ok || !checkedProfilerIntAddTo(&pairRows, family.staged) {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_staged_counter_invalid"}
		}
		if !checkedProfilerIntAddTo(&withheldRows, family.withheld) {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_withheld_counter_invalid"}
		}
		if family.structuredWithheld > family.withheld {
			return &traceDBOutputInvariantError{Reason: "profiler_structured_pair_withheld_account_invalid"}
		}
		structuredEvents := 0
		for _, field := range profilerStructuredPairEventFields(kind) {
			slot, slotOK := profilerPairEndpointForStructuredField(field)
			counts, countsOK := s.pairFixedLedger.endpoint(slot)
			if !slotOK || !countsOK || !checkedProfilerIntAddTo(&structuredEvents, counts.structured) {
				return &traceDBOutputInvariantError{Reason: "profiler_structured_event_staged_counter_invalid"}
			}
		}
		if structuredEvents != family.structured {
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

func (s *traceDBRowSink) validateProfilerPairLaneRegistryParity() error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_pair_sink_missing"}
	}
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		registry := &s.pairLaneRegistries[kind]
		if !profilerPairBudgetKind(kind) {
			if len(registry.byKey) != 0 || len(registry.keys) != 0 || len(registry.states) != 0 {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_lane_registry_nonprofiler_state"}
			}
			continue
		}
		family, ok := s.pairFixedLedger.family(kind)
		if !ok {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_ledger_invalid"}
		}
		if family.poisoned {
			if len(registry.byKey) != 0 || len(registry.keys) != 0 || len(registry.states) != 0 {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_lane_registry_family_reset_mismatch"}
			}
			continue
		}
		if len(registry.byKey) != len(registry.states) || len(registry.keys) != len(registry.states) {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_lane_registry_cardinality_mismatch"}
		}
		var laneTotals [profilerPairFamilyEndpointCapacity]profilerPairFixedCounts
		var poisonedTotals [profilerPairFamilyEndpointCapacity]profilerPairFixedCounts
		for index, lane := range registry.keys {
			if lane == "" {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_lane_registry_empty_key"}
			}
			id := uint32(index + 1)
			mappedID, mapped := registry.byKey[lane]
			if !mapped || mappedID != id {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_lane_registry_id_mismatch"}
			}
			state, ok := registry.state(id)
			if !ok || !state.endpointCountsValid(kind) {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_lane_state_invalid"}
			}
			rows, _, totalsOK := state.endpointTotals(kind)
			if !totalsOK || rows == 0 && !state.poisoned {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_lane_registry_orphan"}
			}
			if kind == pairRenderBlock {
				if !state.blockClockSeen && (state.lastBlockSeq != 0 || state.lastBlockTSNS != 0) {
					return &traceDBOutputInvariantError{Reason: "profiler_block_lane_registry_clock_residue"}
				}
			} else if state.blockClockSeen || state.lastBlockSeq != 0 || state.lastBlockTSNS != 0 {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_lane_registry_foreign_clock"}
			}
			count, _ := profilerPairFamilyEndpointCount(kind)
			for ordinal := uint8(0); ordinal < count; ordinal++ {
				laneCounts := state.endpointCounts[ordinal]
				delta := profilerPairFixedCounts{
					staged: int(laneCounts.rows), structured: int(laneCounts.structuredRows),
				}
				if !addProfilerPairFixedCounts(&laneTotals[ordinal], delta) {
					return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_lane_total_invalid"}
				}
				if state.poisoned && !addProfilerPairFixedCounts(&poisonedTotals[ordinal], delta) {
					return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_lane_withheld_invalid"}
				}
			}
		}
		first, count, rangeOK := profilerPairFamilyEndpointRange(kind)
		if !rangeOK {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_endpoint_range_invalid"}
		}
		var poisonedFamily profilerPairFixedCounts
		for ordinal := uint8(0); ordinal < count; ordinal++ {
			fixed := s.pairFixedLedger.endpoints[first+profilerPairEndpointSlot(ordinal)]
			lanes := laneTotals[ordinal]
			poisoned := poisonedTotals[ordinal]
			if lanes.staged > fixed.staged || lanes.structured > fixed.structured {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_lane_exceeds_endpoint"}
			}
			if poisoned.staged != fixed.withheld || poisoned.structured != fixed.structuredWithheld {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_withheld_lane_mismatch"}
			}
			if !addProfilerPairFixedCounts(&poisonedFamily, poisoned) {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_lane_withheld_invalid"}
			}
		}
		if poisonedFamily.staged != family.withheld ||
			poisonedFamily.structured != family.structuredWithheld {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_fixed_withheld_family_mismatch"}
		}
	}
	return s.validateProfilerPairFixedLedgerParity()
}

func (s *traceDBRowSink) rowPublishable(row traceDBStoredRow) bool {
	if s == nil || s.allRowsFailClosed {
		return false
	}
	provenance := row.profilerProvenance()
	if !provenance.valid() || s.pairAuthorityFailure != "" && provenance.PairKind != pairRenderUnknown {
		return false
	}
	if provenance.PairKind == pairRenderUnknown {
		return true
	}
	if !profilerPairKindValid(provenance.PairKind) || s.poisoned[provenance.PairKind] {
		return false
	}
	if provenance.LaneID == 0 {
		return true
	}
	state, ok := s.pairLaneRegistries[provenance.PairKind].state(provenance.LaneID)
	return ok && !state.poisoned
}

func (s *traceDBRowSink) accountWrittenRow(row traceDBStoredRow) {
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
	sidecarBytes := uint64(0)
	if s.sourceOrderSidecar.present() {
		manifest := s.sourceOrderSidecar
		artifact := s.artifacts[manifest.path]
		expectedSidecarSize, sidecarSizeErr := profilerSourceOrderSidecarSize(manifest.rowCount)
		if s.captureLifecycle == profilerCaptureInactive || s.stats.RowsAccepted <= 0 || len(s.runs) != 1 ||
			sidecarSizeErr != nil || manifest.rowCount != uint64(s.stats.RowsAccepted) ||
			manifest.size != expectedSidecarSize || manifest.boundRunDigest != s.runs[0].digest ||
			manifest.producerRoot != s.profilerSourceProof.expectedRoot ||
			manifest.rowCount != s.profilerSourceProof.expectedCount || artifact == nil || artifact.removed ||
			s.stats.SourceSidecarLogicalBytes != manifest.size ||
			s.stats.SourceSidecarPhysicalBytes != manifest.size {
			return &traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_manifest_invalid"}
		}
		sidecarBytes = manifest.size
	} else if s.stats.SourceSidecarLogicalBytes != 0 ||
		s.stats.SourceSidecarPhysicalBytes != 0 {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_manifest_missing"}
	}
	expectedLive, liveOK := checkedTraceDBUint64Add(totalSize, sidecarBytes)
	activeWithSidecar, activeOK := checkedTraceDBUint64Add(s.activeTempBytes, sidecarBytes)
	if totalRows != uint64(s.stats.RowsAccepted) || totalSize != s.activeTempBytes ||
		!liveOK || expectedLive != s.liveTempBytes || !activeOK ||
		activeWithSidecar > s.options.activeTempCap || s.activeTempBytes > s.options.activeTempCap ||
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

// authenticatePreparedFinalRun closes the single-leaf gap where an inactive
// SQL/generic sink has nothing to merge. validateRunManifestSet authenticates
// manifest shape, not file bytes; consume the final registered run to EOF in
// preflight so digest/order/record damage is found before output is opened.
// The reader retains one record only and therefore replaces no retired
// per-row map, bitmap, or seen set.
func (s *traceDBRowSink) authenticatePreparedFinalRun(ctx context.Context) error {
	if s == nil || ctx == nil || s.captureLifecycle != profilerCaptureInactive {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_preflight_auth_state_invalid"}
	}
	if s.stats.RowsAccepted == 0 {
		if len(s.runs) != 0 {
			return traceDBRunInputIntegrity(&traceDBOutputInvariantError{
				Reason: "trace_row_sort_preflight_auth_zero_run_mismatch",
			})
		}
		return nil
	}
	if len(s.runs) != 1 || s.runs[0].rowCount != uint64(s.stats.RowsAccepted) {
		return traceDBRunInputIntegrity(&traceDBOutputInvariantError{
			Reason: "trace_row_sort_preflight_auth_manifest_mismatch",
		})
	}
	reader, err := s.openAuthenticatedRunReader(s.runs[0])
	if err != nil {
		return traceDBRunInputIntegrity(err)
	}
	var readErr error
	for readErr == nil {
		_, ok, nextErr := reader.next(ctx)
		if nextErr != nil {
			readErr = traceDBRunInputIntegrity(nextErr)
			break
		}
		if !ok {
			break
		}
	}
	return traceDBJoinPreservingSingle(readErr, traceDBRunInputIntegrity(reader.close()))
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
	if err := s.validateProfilerSourceOrderProof(); err != nil {
		s.prepareFailure = err
		return err
	}
	if s.captureLifecycle != profilerCaptureInactive {
		if err := s.profilerSourceProof.freezeExpected(); err != nil {
			s.prepareFailure = err
			return err
		}
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
	if s.captureLifecycle == profilerCaptureInactive {
		if err := s.authenticatePreparedFinalRun(ctx); err != nil {
			if handled := s.finishRegisteredRunStorageFailure(err); handled != nil {
				s.prepareFailure = handled
				return handled
			}
			return nil
		}
	}
	// The typed/legacy accounting oracle must fail before B-c derives a
	// terminal disposition from either side. sealProfilerCapture historically
	// performed this check after prepare; sidecar construction now consumes the
	// same state, so preserve the earlier authoritative reason here as well.
	if s.captureLifecycle != profilerCaptureInactive {
		if err := s.validateProfilerPairAccounting(); err != nil {
			s.prepareFailure = err
			return err
		}
	}
	if s.captureLifecycle != profilerCaptureInactive {
		if err := s.buildProfilerSourceOrderSidecar(ctx); err != nil {
			if handled := s.finishRegisteredRunStorageFailure(err); handled != nil {
				s.prepareFailure = handled
				return handled
			}
			return nil
		}
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
	var sourceOrderPublication *profilerSourceOrderPublicationProof
	var standaloneReader *traceDBAuthenticatedRunReader
	defer func() {
		var publicationCloseErr error
		if sourceOrderPublication != nil {
			publicationCloseErr = sourceOrderPublication.close()
		} else if standaloneReader != nil {
			publicationCloseErr = traceDBRunInputIntegrity(standaloneReader.close())
		}
		err = traceDBJoinPreservingSingle(err, publicationCloseErr, s.accumulateElapsed(start), s.cleanup())
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
	if s.stats.RowsAccepted == 0 {
		if len(s.runs) != 0 || s.sourceOrderSidecar.present() {
			return s.stats, &traceDBOutputInvariantError{Reason: "trace_row_sort_final_manifest_invalid"}
		}
		if err := writeSystraceHeader(bw); err != nil {
			return s.stats, err
		}
		if err := bw.Flush(); err != nil {
			return s.stats, err
		}
		return s.stats, s.validateProfilerWrittenAccounting()
	}
	if len(s.runs) != 1 {
		return s.stats, &traceDBOutputInvariantError{Reason: "trace_row_sort_final_manifest_invalid"}
	}
	var reader *traceDBAuthenticatedRunReader
	if s.captureLifecycle != profilerCaptureInactive {
		sourceOrderPublication, err = s.openProfilerSourceOrderPublicationProof(ctx)
		if err != nil {
			return s.stats, err
		}
		reader = sourceOrderPublication.run
	} else {
		var openErr error
		reader, openErr = s.openAuthenticatedRunReader(s.runs[0])
		if openErr != nil {
			return s.stats, traceDBRunInputIntegrity(openErr)
		}
		standaloneReader = reader
	}
	if err := writeSystraceHeader(bw); err != nil {
		return s.stats, err
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
		publishable := false
		if sourceOrderPublication != nil {
			disposition, verifyErr := sourceOrderPublication.verifyRunRecord(ctx, record)
			if verifyErr != nil {
				readErr = traceDBRunInputIntegrity(verifyErr)
				break
			}
			publishable = disposition.publishable()
		} else {
			publishable = s.rowPublishable(record.row)
		}
		if !publishable {
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
	if readErr == nil && sourceOrderPublication != nil {
		readErr = sourceOrderPublication.validateFinalSidecar(ctx)
	}
	if sourceOrderPublication != nil {
		readErr = traceDBJoinPreservingSingle(readErr, sourceOrderPublication.close())
	} else {
		readErr = traceDBJoinPreservingSingle(readErr, traceDBRunInputIntegrity(reader.close()))
	}
	if readErr != nil {
		return s.stats, readErr
	}
	// A close/final-audit fault hook may cancel after the last row poll. Keep
	// cancellation identity and stop before the buffered publication is flushed.
	if err := ctx.Err(); err != nil {
		return s.stats, err
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
	return s.flushChunkWithContext(s.operationContext)
}

func (s *traceDBRowSink) flushChunkWithContext(ctx context.Context) (result error) {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	defer func() {
		if elapsedErr := s.accumulateElapsed(started); elapsedErr != nil {
			result = traceDBJoinPreservingSingle(result, elapsedErr)
		}
	}()
	return s.flushChunkContext(ctx)
}

type traceDBBufferedRunRow struct {
	row           traceDBStoredRow
	ingestOrdinal uint64
}

type traceDBBufferedRows struct {
	rows     []traceDBStoredRow
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
		ProfilerProvenance: row.row.profilerProvenance(),
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
		primary = traceDBJoinPreservingSingle(primary, s.closeRunFile(file, path))
	}
	removeErr := s.removeArtifact(path)
	primary = traceDBJoinPreservingSingle(primary, removeErr)
	if removeErr == nil {
		primary = traceDBJoinPreservingSingle(primary, s.releasePendingRun(reserved))
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
	if err := ctx.Err(); err != nil {
		return s.discardPendingRun(nil, path, encodedSize, err)
	}
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
	// Cleanup is a terminal lifecycle transition even when a filesystem
	// removal later fails. Freeze the accepted prefix first so a caller cannot
	// resume mutation against a partially removed artifact set; a retry may
	// finish physical cleanup but can never change the expected source proof.
	if s.profilerSourceProof.active && !s.profilerSourceProof.frozen && !s.profilerSourceProof.retired {
		result = errors.Join(result, s.profilerSourceProof.freezeExpected())
	}
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
		s.sourceOrderSidecar = profilerSourceOrderSidecarManifest{}
		s.activeTempBytes = 0
		s.liveTempBytes = 0
		s.openRunFDs = 0
		s.profilerSourceProof.retire()
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

func sortTraceDBStoredRows(rows []traceDBStoredRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].tsNS == rows[j].tsNS {
			return rows[i].seq < rows[j].seq
		}
		return rows[i].tsNS < rows[j].tsNS
	})
}

type traceDBChunkRow struct {
	Line               string                    `json:"line"`
	TSNS               uint64                    `json:"ts_ns"`
	IngestOrdinal      uint64                    `json:"ingest_ordinal"`
	Seq                int                       `json:"seq"`
	ProfilerProvenance profilerPairRowProvenance `json:"p"`
}

type traceDBChunkWireRow struct {
	Line               *string                    `json:"line"`
	TSNS               *uint64                    `json:"ts_ns"`
	Seq                *int                       `json:"seq"`
	IngestOrdinal      *uint64                    `json:"ingest_ordinal"`
	ProfilerProvenance *profilerPairRowProvenance `json:"p"`
}

type traceDBRunRecord struct {
	row           traceDBStoredRow
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

func readTraceDBBoundedJSONLRecord(
	reader *bufio.Reader,
	maximum uint64,
	scratch *[]byte,
) ([]byte, bool, error) {
	if reader == nil || maximum == 0 || maximum > uint64(math.MaxInt) || scratch == nil {
		return nil, false, &traceDBOutputInvariantError{Reason: "trace_row_sort_record_limit_invalid"}
	}
	*scratch = (*scratch)[:0]
	record := *scratch
	retainScratch := func() {
		*scratch = record[:0]
	}
	for {
		fragment, err := reader.ReadSlice('\n')
		projected, ok := checkedTraceDBUint64Add(uint64(len(record)), uint64(len(fragment)))
		if !ok || projected > maximum {
			retainScratch()
			return nil, false, &traceDBOutputInvariantError{Reason: "trace_row_sort_physical_record_too_large"}
		}
		// A record which fits in bufio's fixed reader buffer needs no second
		// allocation. Its bytes share the same lease as a scratch-backed record:
		// the caller must consume them before the next read/reset/close.
		if len(record) == 0 && err == nil {
			if len(fragment) == 0 || fragment[len(fragment)-1] != '\n' {
				return nil, false, &traceDBOutputInvariantError{Reason: "trace_row_sort_record_not_terminated"}
			}
			return fragment[:len(fragment):len(fragment)], true, nil
		}
		record = append(record, fragment...)
		retainScratch()
		switch {
		case err == nil:
			if len(record) == 0 || record[len(record)-1] != '\n' {
				return nil, false, &traceDBOutputInvariantError{Reason: "trace_row_sort_record_not_terminated"}
			}
			return record[:len(record):len(record)], true, nil
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
	if wire.ProfilerProvenance == nil {
		return traceDBRunRecord{}, &traceDBOutputInvariantError{Reason: "trace_row_sort_record_required_field_missing"}
	}
	provenance := *wire.ProfilerProvenance
	if *wire.IngestOrdinal >= maximumIngest {
		return traceDBRunRecord{}, &traceDBOutputInvariantError{Reason: "trace_row_sort_ingest_ordinal_out_of_range"}
	}
	row := traceDBStoredRow{
		tsNS: *wire.TSNS, seq: *wire.Seq, line: *wire.Line, provenance: provenance,
	}
	if !traceDBSinglePhysicalLine(row.line, false) || !provenance.valid() {
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
	// recordScratch is allocated lazily only for a physical record which spans
	// bufio fragments, then reused for every later fragmented record/pass.
	recordScratch []byte
	rowsRead      uint64
	previous      traceDBRunRecord
	havePrev      bool
	verified      bool
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
	if err := s.runFault("fstat", manifest.path); err != nil {
		return nil, traceDBJoinPreservingSingle(
			traceDBRunInputIntegrity(err), s.closeRunFile(file, manifest.path),
		)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, traceDBJoinPreservingSingle(
			traceDBRunInputIntegrity(traceDBSorterOperationError("fstat", err)),
			s.closeRunFile(file, manifest.path),
		)
	}
	if !openedInfo.Mode().IsRegular() || openedInfo.Size() < 0 || uint64(openedInfo.Size()) != manifest.size {
		return nil, traceDBJoinPreservingSingle(
			traceDBRunInputIntegrity(&traceDBOutputInvariantError{
				Reason: "trace_row_sort_manifest_opened_size_mismatch",
			}),
			s.closeRunFile(file, manifest.path),
		)
	}
	return &traceDBAuthenticatedRunReader{
		sink: s, manifest: manifest, file: file,
		reader: bufio.NewReaderSize(file, 256*1024), proof: sha256.New(),
	}, nil
}

// reset replays one already-authenticated run through the same opened file
// description. B-c uses the first pass as a publication preflight and the
// second as the actual output pass; closing and reopening by path between the
// two would reintroduce a replace-by-path window.
func (reader *traceDBAuthenticatedRunReader) reset() error {
	if reader == nil || reader.sink == nil || reader.file == nil || reader.reader == nil ||
		reader.proof == nil || !reader.verified {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_reader_reset_state_invalid"}
	}
	if err := reader.sink.runFault("seek", reader.manifest.path); err != nil {
		return err
	}
	offset, err := reader.file.Seek(0, io.SeekStart)
	if err != nil {
		return traceDBSorterOperationError("seek", err)
	}
	if offset != 0 {
		return &traceDBOutputInvariantError{Reason: "trace_row_sort_reader_reset_offset_invalid"}
	}
	reader.reader.Reset(reader.file)
	reader.proof.Reset()
	reader.rowsRead = 0
	reader.previous = traceDBRunRecord{}
	reader.havePrev = false
	reader.verified = false
	reader.recordScratch = reader.recordScratch[:0]
	return nil
}

// next returns a borrowed record. record.raw is a read-only, capacity-fenced
// view which aliases this reader's bufio buffer or recordScratch and remains
// valid only until the next next, reset, or close call on this same reader.
// record.row owns its decoded values and outlives that raw lease. Callers must
// consume raw first; merge keeps at most one outstanding record per reader and
// advances it only after writing that record.
func (reader *traceDBAuthenticatedRunReader) next(ctx context.Context) (traceDBRunRecord, bool, error) {
	if reader == nil || reader.sink == nil || reader.file == nil || reader.reader == nil ||
		reader.proof == nil || reader.verified {
		return traceDBRunRecord{}, false, &traceDBOutputInvariantError{Reason: "trace_row_sort_reader_state_invalid"}
	}
	if err := ctx.Err(); err != nil {
		return traceDBRunRecord{}, false, err
	}
	if err := reader.sink.runFault("read", reader.manifest.path); err != nil {
		return traceDBRunRecord{}, false, err
	}
	raw, ok, err := readTraceDBBoundedJSONLRecord(
		reader.reader, reader.sink.options.maxRunRowBytes, &reader.recordScratch,
	)
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
	reader.reader = nil
	reader.proof = nil
	reader.recordScratch = nil
	reader.rowsRead = 0
	reader.previous = traceDBRunRecord{}
	reader.havePrev = false
	reader.verified = false
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
