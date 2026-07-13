package hitraceconv

import (
	"bufio"
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
)

const defaultTraceDBRowSinkThreshold = 200_000

const profilerPairStorageIntegrityFailure = "profiler_pair_storage_integrity_failure"

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
	chunkDigests         map[string][sha256.Size]byte
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
// lane and structured endpoint ownership cannot drift. The separate per-chunk
// SHA-256 proves every physical spill byte, including tsNS, line and ordinary
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

func newTraceDBRowSink(tempDir string, threshold int) (*traceDBRowSink, error) {
	if threshold <= 0 {
		threshold = defaultTraceDBRowSinkThreshold
	}
	sink := &traceDBRowSink{
		threshold: threshold, tempDir: tempDir,
		chunkDigests: make(map[string][sha256.Size]byte),
		pairRows:     make(map[pairRenderKind]int), pairLaneRows: make(map[pairRenderKind]map[string]int),
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

func (s *traceDBRowSink) openProfilerCapture(sourcePath string) error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_capture_sink_missing"}
	}
	if s.captureLifecycle != profilerCaptureInactive || s.captureSource != "" ||
		s.stats.RowsAccepted != 0 || len(s.rows) != 0 || len(s.chunks) != 0 || len(s.chunkDigests) != 0 ||
		s.pairCensusActive || s.captureSourceFailure != "" {
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
	if err := s.prepareAndValidateProfilerPairStorage(); err != nil {
		s.recordProfilerCaptureBreach("profiler_pair_storage_validation_failed")
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

func (s *traceDBRowSink) add(row renderedRow) error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	if !s.profilerMutationAllowed("profiler_capture_add_after_seal") {
		return &traceDBOutputInvariantError{Reason: s.captureBreach}
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
	if row.pairKind != pairRenderUnknown {
		s.auditProfilerPairPhysicalRow(row)
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
	if len(s.rows) >= s.threshold {
		return s.flushChunk()
	}
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

// prepareAndValidateProfilerPairStorage freezes any pending spill tail, then
// verifies two independent layers in one read: pair rows must match the
// publisher's semantic map, and every chunk must match its write-time SHA-256
// over the complete JSONL byte stream. It deliberately runs after family
// poison: shared sequence/mapping and physical-storage authority cannot be
// waived by a narrower publication decision.
func (s *traceDBRowSink) prepareAndValidateProfilerPairStorage() error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_pair_sink_missing"}
	}
	if len(s.chunks) > 0 && len(s.rows) > 0 {
		if err := s.flushChunk(); err != nil {
			return err
		}
	}
	mappingProofActive := s.pairAuthorityFailure == ""
	seen := make(map[int]struct{}, len(s.pairRowMappings))
	var readRows int64
	var localizedDuplicateRows int64
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
		if readRows == math.MaxInt64 {
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
			if localizedDuplicateRows == math.MaxInt64 {
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

	spillIntegrityDrift := false
	markUnreadableSpill := func() {
		// Once a chunk has been registered by flushChunk, failure to reopen,
		// decode/read, or close that exact artifact is an at-rest integrity
		// failure. Do not return the raw filesystem/JSON error: sealed Profiler
		// captures must finish sealing so the result envelope can disclose
		// SourceFailClosed, while inactive sinks translate the same state into a
		// typed error before output. Errors before registration (for example a
		// pending-tail flush failure) deliberately keep their original boundary.
		spillIntegrityDrift = true
		s.failProfilerPairAuthority("profiler_pair_spill_integrity_mismatch")
		s.allRowsFailClosed = true
		if s.captureSourceFailure == "" {
			s.captureSourceFailure = profilerPairStorageIntegrityFailure
		}
	}
	if len(s.chunks) == 0 {
		for _, row := range s.rows {
			mapped := observe(row)
			if !mapped {
				if err := validateProfilerEventFieldProvenance(row); err != nil {
					return err
				}
			}
		}
	} else {
		for _, path := range s.chunks {
			reader, err := openTraceDBChunkProofReader(path)
			if err != nil {
				markUnreadableSpill()
				continue
			}
			var rowValidationErr error
			for {
				row, ok, readErr := reader.nextRaw()
				if readErr != nil {
					markUnreadableSpill()
					break
				}
				if !ok {
					break
				}
				mapped := observe(row)
				if !mapped {
					if err := validateProfilerEventFieldProvenance(row); err != nil {
						rowValidationErr = err
						break
					}
				}
			}
			actualDigest, digestKnown := reader.proofDigest()
			expectedDigest, expectedKnown := s.chunkDigests[path]
			if !digestKnown || !expectedKnown || actualDigest != expectedDigest {
				spillIntegrityDrift = true
				s.allRowsFailClosed = true
				if s.captureSourceFailure == "" {
					s.captureSourceFailure = profilerPairStorageIntegrityFailure
				}
			}
			if err := reader.close(); err != nil {
				markUnreadableSpill()
			}
			if rowValidationErr != nil {
				// Schema/provenance failures are not storage failures. Preserve
				// their precise typed boundary unless the same registered chunk
				// independently failed its digest/read/close proof.
				if !spillIntegrityDrift {
					return rowValidationErr
				}
			}
		}
	}
	if mappingProofActive && len(seen) != len(s.pairRowMappings) {
		failStorageAuthority("published_row_mapping_missing", true)
	}
	logicalRows := readRows
	if localizedDuplicateRows > logicalRows {
		failStorageAuthority("published_row_storage_count_invalid", true)
		logicalRows = -1
	} else {
		logicalRows -= localizedDuplicateRows
	}
	if logicalRows != int64(s.stats.RowsAccepted) {
		failStorageAuthority("published_row_storage_count_mismatch", true)
	}
	if spillIntegrityDrift {
		// Preserve a more precise mapping/duplicate/count reason already
		// observed during the same raw readback; otherwise expose the chunk
		// proof as the shared authority failure.
		s.failProfilerPairAuthority("profiler_pair_spill_integrity_mismatch")
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

func (s *traceDBRowSink) writeTo(ctx context.Context, w io.Writer) (stats traceDBRowSortStats, err error) {
	if s == nil {
		return traceDBRowSortStats{}, &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	if s.captureLifecycle == profilerCaptureOpen {
		return s.stats, &traceDBOutputInvariantError{Reason: "profiler_capture_write_before_seal"}
	}
	if s.captureLifecycle == profilerCaptureSealed && s.captureBreach != "" {
		return s.stats, &traceDBOutputInvariantError{Reason: s.captureBreach}
	}
	if s.captureLifecycle == profilerCaptureInactive {
		if err := s.prepareAndValidateProfilerPairStorage(); err != nil {
			return s.stats, err
		}
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
		s.stats.ElapsedUS = traceDBCoverageElapsedUS(start)
		stats = s.stats
	}()
	defer s.cleanup()
	s.stats.RowsWritten = 0
	s.stats.RowsWithheld = withheld
	s.stats.FirstTSNS = 0
	s.stats.LastTSNS = 0
	if s.allRowsFailClosed {
		s.stats.RowsWithheld = s.stats.RowsAccepted
		if err := s.validateProfilerWrittenAccounting(); err != nil {
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
		if err != nil {
			return s.stats, err
		}
		return s.stats, s.validateProfilerWrittenAccounting()
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
		row, ok, err := s.nextProfilerChunkRow(reader)
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
		row, ok, err := s.nextProfilerChunkRow(readers[item.readerIndex])
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
	return s.stats, s.validateProfilerWrittenAccounting()
}

// nextProfilerChunkRow keeps readback mapping authority ahead of the derived
// event-field validator. Once a mapped row has already caused a global
// authority failure, its possibly drifted metadata is consumed only so the
// row can be withheld; it must not be reclassified as a narrower schema error.
func (s *traceDBRowSink) nextProfilerChunkRow(reader *traceDBChunkReader) (renderedRow, bool, error) {
	row, ok, err := reader.nextRaw()
	if err != nil || !ok {
		return row, ok, err
	}
	if s != nil && s.pairAuthorityFailure != "" {
		if _, mapped := s.pairRowMappings[row.seq]; mapped {
			return row, true, nil
		}
	}
	if err := validateProfilerEventFieldProvenance(row); err != nil {
		return renderedRow{}, false, err
	}
	return row, true, nil
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
	digest := sha256.New()
	bw := bufio.NewWriterSize(io.MultiWriter(f, digest), 256*1024)
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
	var chunkDigest [sha256.Size]byte
	copy(chunkDigest[:], digest.Sum(nil))
	if s.chunkDigests == nil {
		s.chunkDigests = make(map[string][sha256.Size]byte)
	}
	s.chunkDigests[path] = chunkDigest
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
	s.chunkDigests = nil
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
	file  *os.File
	dec   *json.Decoder
	proof hash.Hash
}

func openTraceDBChunkReader(path string) (*traceDBChunkReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &traceDBChunkReader{file: f, dec: json.NewDecoder(f)}, nil
}

func openTraceDBChunkProofReader(path string) (*traceDBChunkReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	proof := sha256.New()
	return &traceDBChunkReader{file: f, dec: json.NewDecoder(io.TeeReader(f, proof)), proof: proof}, nil
}

func (r *traceDBChunkReader) proofDigest() ([sha256.Size]byte, bool) {
	if r == nil || r.proof == nil {
		return [sha256.Size]byte{}, false
	}
	var digest [sha256.Size]byte
	copy(digest[:], r.proof.Sum(nil))
	return digest, true
}

func (r *traceDBChunkReader) next() (renderedRow, bool, error) {
	row, ok, err := r.nextRaw()
	if err != nil || !ok {
		return row, ok, err
	}
	if err := validateProfilerEventFieldProvenance(row); err != nil {
		return renderedRow{}, false, err
	}
	return row, true, nil
}

func (r *traceDBChunkReader) nextRaw() (renderedRow, bool, error) {
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
