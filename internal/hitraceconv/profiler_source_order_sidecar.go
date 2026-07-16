package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
)

const (
	profilerSourceOrderSidecarMagic       = "codrax-pso-side\x00"
	profilerSourceOrderSidecarVersion     = uint16(2)
	profilerSourceOrderSidecarHeaderBytes = uint16(96)
	profilerSourceOrderSidecarRecordBytes = uint16(56)

	profilerSourceOrderSidecarPageBytes   = 64 << 10
	profilerSourceOrderSidecarPageRecords = profilerSourceOrderSidecarPageBytes /
		int(profilerSourceOrderSidecarRecordBytes)
	profilerSourceOrderSidecarRecordPageBytes = profilerSourceOrderSidecarPageRecords *
		int(profilerSourceOrderSidecarRecordBytes)
)

var _ [16 - len(profilerSourceOrderSidecarMagic)]byte
var _ [len(profilerSourceOrderSidecarMagic) - 16]byte

type profilerSourceOrderDisposition uint8

const (
	profilerSourceOrderDispositionInvalid profilerSourceOrderDisposition = iota
	profilerSourceOrderDispositionPublish
	profilerSourceOrderDispositionWithhold
)

func (disposition profilerSourceOrderDisposition) valid() bool {
	return disposition == profilerSourceOrderDispositionPublish ||
		disposition == profilerSourceOrderDispositionWithhold
}

func (disposition profilerSourceOrderDisposition) publishable() bool {
	return disposition == profilerSourceOrderDispositionPublish
}

type profilerSourceOrderSidecarManifest struct {
	path           string
	size           uint64
	rowCount       uint64
	digest         [sha256.Size]byte
	producerRoot   [sha256.Size]byte
	boundRunDigest [sha256.Size]byte
	terminal       profilerTerminalPublicationLedger
}

func (manifest profilerSourceOrderSidecarManifest) present() bool {
	return manifest.path != ""
}

type profilerSourceOrderSidecarHeader struct {
	rowCount       uint64
	producerRoot   [sha256.Size]byte
	boundRunDigest [sha256.Size]byte
}

type profilerSourceOrderSidecarRecord struct {
	ordinalPlusOne uint64
	leaf           [sha256.Size]byte
	provenance     profilerPairRowProvenance
	disposition    profilerSourceOrderDisposition
}

type profilerSourceOrderSidecarAudit struct {
	digest    [sha256.Size]byte
	published uint64
	withheld  uint64
	terminal  profilerTerminalPublicationLedger
}

func profilerSourceOrderSidecarSize(rowCount uint64) (uint64, error) {
	if rowCount == 0 || rowCount > (math.MaxUint64-uint64(profilerSourceOrderSidecarHeaderBytes))/
		uint64(profilerSourceOrderSidecarRecordBytes) {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_size_invalid"}
	}
	size := uint64(profilerSourceOrderSidecarHeaderBytes) +
		rowCount*uint64(profilerSourceOrderSidecarRecordBytes)
	if size > math.MaxInt64 {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_size_invalid"}
	}
	return size, nil
}

func profilerSourceOrderSidecarRecordOffset(ordinal, rowCount uint64) (int64, error) {
	if ordinal >= rowCount || ordinal > (math.MaxUint64-uint64(profilerSourceOrderSidecarHeaderBytes))/
		uint64(profilerSourceOrderSidecarRecordBytes) {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_ordinal_out_of_range"}
	}
	offset := uint64(profilerSourceOrderSidecarHeaderBytes) +
		ordinal*uint64(profilerSourceOrderSidecarRecordBytes)
	if offset > math.MaxInt64 {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_offset_invalid"}
	}
	return int64(offset), nil
}

func encodeProfilerSourceOrderSidecarHeader(header profilerSourceOrderSidecarHeader) [profilerSourceOrderSidecarHeaderBytes]byte {
	var wire [profilerSourceOrderSidecarHeaderBytes]byte
	copy(wire[0:16], profilerSourceOrderSidecarMagic)
	binary.LittleEndian.PutUint16(wire[16:18], profilerSourceOrderSidecarVersion)
	binary.LittleEndian.PutUint16(wire[18:20], profilerSourceOrderProofVersion)
	binary.LittleEndian.PutUint16(wire[20:22], profilerSourceOrderSidecarHeaderBytes)
	binary.LittleEndian.PutUint16(wire[22:24], profilerSourceOrderSidecarRecordBytes)
	binary.LittleEndian.PutUint64(wire[24:32], header.rowCount)
	copy(wire[32:64], header.producerRoot[:])
	copy(wire[64:96], header.boundRunDigest[:])
	return wire
}

func decodeProfilerSourceOrderSidecarHeader(wire []byte) (profilerSourceOrderSidecarHeader, error) {
	if len(wire) != int(profilerSourceOrderSidecarHeaderBytes) ||
		string(wire[0:16]) != profilerSourceOrderSidecarMagic ||
		binary.LittleEndian.Uint16(wire[16:18]) != profilerSourceOrderSidecarVersion ||
		binary.LittleEndian.Uint16(wire[18:20]) != profilerSourceOrderProofVersion ||
		binary.LittleEndian.Uint16(wire[20:22]) != profilerSourceOrderSidecarHeaderBytes ||
		binary.LittleEndian.Uint16(wire[22:24]) != profilerSourceOrderSidecarRecordBytes {
		return profilerSourceOrderSidecarHeader{}, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_header_invalid",
		}
	}
	header := profilerSourceOrderSidecarHeader{rowCount: binary.LittleEndian.Uint64(wire[24:32])}
	copy(header.producerRoot[:], wire[32:64])
	copy(header.boundRunDigest[:], wire[64:96])
	return header, nil
}

func encodeProfilerSourceOrderSidecarRecord(
	ordinal uint64,
	leaf [sha256.Size]byte,
	provenance profilerPairRowProvenance,
	disposition profilerSourceOrderDisposition,
) ([profilerSourceOrderSidecarRecordBytes]byte, error) {
	var wire [profilerSourceOrderSidecarRecordBytes]byte
	err := encodeProfilerSourceOrderSidecarRecordInto(
		wire[:], ordinal, leaf, provenance, disposition,
	)
	return wire, err
}

func encodeProfilerSourceOrderSidecarRecordInto(
	wire []byte,
	ordinal uint64,
	leaf [sha256.Size]byte,
	provenance profilerPairRowProvenance,
	disposition profilerSourceOrderDisposition,
) error {
	if ordinal == math.MaxUint64 || !provenance.storageValid() || !disposition.valid() {
		return &traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_record_invalid",
		}
	}
	if len(wire) != int(profilerSourceOrderSidecarRecordBytes) {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_record_buffer_invalid"}
	}
	clear(wire)
	binary.LittleEndian.PutUint64(wire[0:8], ordinal+1)
	copy(wire[8:40], leaf[:])
	binary.LittleEndian.PutUint32(wire[40:44], provenance.LaneID)
	binary.LittleEndian.PutUint32(wire[44:48], provenance.TextMessageOrdinal)
	wire[48] = byte(provenance.PairKind)
	wire[49] = byte(provenance.EndpointSlot)
	wire[50] = byte(provenance.PublisherSlot)
	wire[51] = byte(provenance.Flags)
	wire[52] = byte(disposition)
	wire[53] = byte(provenance.TraceClass)
	return nil
}

func decodeProfilerSourceOrderSidecarRecord(wire []byte) (profilerSourceOrderSidecarRecord, error) {
	if len(wire) != int(profilerSourceOrderSidecarRecordBytes) || wire[54] != 0 || wire[55] != 0 {
		return profilerSourceOrderSidecarRecord{}, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_record_invalid",
		}
	}
	record := profilerSourceOrderSidecarRecord{
		ordinalPlusOne: binary.LittleEndian.Uint64(wire[0:8]),
		provenance: profilerPairRowProvenance{
			LaneID:             binary.LittleEndian.Uint32(wire[40:44]),
			TextMessageOrdinal: binary.LittleEndian.Uint32(wire[44:48]),
			PairKind:           pairRenderKind(wire[48]),
			EndpointSlot:       profilerPairEndpointSlot(wire[49]),
			PublisherSlot:      profilerPairPublisherSlot(wire[50]),
			Flags:              profilerPairRowProvenanceFlags(wire[51]),
			TraceClass:         profilerTraceClass(wire[53]),
		},
		disposition: profilerSourceOrderDisposition(wire[52]),
	}
	copy(record.leaf[:], wire[8:40])
	if record.ordinalPlusOne == 0 || !record.provenance.storageValid() || !record.disposition.valid() {
		return profilerSourceOrderSidecarRecord{}, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_record_invalid",
		}
	}
	return record, nil
}

func profilerSourceOrderSidecarRecordUnwritten(wire []byte) bool {
	if len(wire) != int(profilerSourceOrderSidecarRecordBytes) {
		return false
	}
	for _, value := range wire {
		if value != 0 {
			return false
		}
	}
	return true
}

func (s *traceDBRowSink) typedProfilerSourceOrderDisposition(
	row traceDBStoredRow,
) (profilerSourceOrderDisposition, error) {
	return s.typedProfilerSourceOrderDispositionForProvenance(row.profilerProvenance())
}

func (s *traceDBRowSink) typedProfilerSourceOrderDispositionForProvenance(
	provenance profilerPairRowProvenance,
) (profilerSourceOrderDisposition, error) {
	if s == nil || s.captureLifecycle == profilerCaptureInactive {
		return profilerSourceOrderDispositionInvalid, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_disposition_state_invalid",
		}
	}
	if !s.profilerStoredProvenanceValid(provenance) {
		return profilerSourceOrderDispositionInvalid, &traceDBOutputInvariantError{
			Reason: "profiler_row_provenance_invalid",
		}
	}
	if s.allRowsFailClosed {
		return profilerSourceOrderDispositionWithhold, nil
	}
	if provenance.PairKind == pairRenderUnknown {
		return profilerSourceOrderDispositionPublish, nil
	}
	if !profilerPairKindValid(provenance.PairKind) {
		return profilerSourceOrderDispositionInvalid, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_disposition_kind_invalid",
		}
	}
	if s.pairAuthorityFailure != "" || s.poisoned[provenance.PairKind] {
		return profilerSourceOrderDispositionWithhold, nil
	}
	if provenance.LaneID == 0 {
		return profilerSourceOrderDispositionPublish, nil
	}
	state, ok := s.pairLaneRegistries[provenance.PairKind].state(provenance.LaneID)
	if !ok {
		return profilerSourceOrderDispositionInvalid, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_disposition_lane_missing",
		}
	}
	if state.poisoned {
		return profilerSourceOrderDispositionWithhold, nil
	}
	return profilerSourceOrderDispositionPublish, nil
}

func (s *traceDBRowSink) createPendingProfilerSourceOrderSidecar() (*os.File, string, error) {
	if err := s.runFault("sidecar_create", s.tempDir); err != nil {
		return nil, "", err
	}
	file, err := s.options.ops.createTemp(s.tempDir, "profiler-source-order-*.bin")
	if err != nil {
		return nil, "", traceDBSorterOperationError("sidecar_create", err)
	}
	path := file.Name()
	if err := s.registerArtifact(path); err != nil {
		return nil, path, errors.Join(err,
			traceDBSorterOperationError("sidecar_close", file.Close()))
	}
	if err := s.noteRunFDOpen(); err != nil {
		return nil, path, errors.Join(err,
			traceDBSorterOperationError("sidecar_close", file.Close()), s.removeArtifact(path))
	}
	return file, path, nil
}

func (s *traceDBRowSink) closeProfilerSourceOrderSidecarFile(file *os.File, path string) error {
	if file == nil {
		return nil
	}
	err := s.runFault("sidecar_close", path)
	err = errors.Join(err, traceDBSorterOperationError("sidecar_close", file.Close()), s.noteRunFDClose())
	return err
}

func (s *traceDBRowSink) profilerSourceOrderSidecarFstat(file *os.File, path string) (os.FileInfo, error) {
	if err := s.runFault("sidecar_fstat", path); err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, traceDBSorterOperationError("sidecar_fstat", err)
	}
	return info, nil
}

func (s *traceDBRowSink) writeProfilerSourceOrderSidecarAt(
	file *os.File,
	path string,
	data []byte,
	offset int64,
) error {
	if err := s.runFault("sidecar_write", path); err != nil {
		return err
	}
	written, err := s.options.ops.writeAt(file, data, offset)
	if err != nil {
		return traceDBSorterOperationError("sidecar_write", err)
	}
	if written != len(data) {
		return traceDBSorterOperationError("sidecar_write", io.ErrShortWrite)
	}
	return nil
}

func (s *traceDBRowSink) readProfilerSourceOrderSidecarAt(
	file *os.File,
	path string,
	data []byte,
	offset int64,
) error {
	if err := s.runFault("sidecar_read", path); err != nil {
		return err
	}
	read, err := s.options.ops.readAt(file, data, offset)
	if err != nil {
		return traceDBSorterOperationError("sidecar_read", err)
	}
	if read != len(data) {
		return traceDBSorterOperationError("sidecar_read", io.ErrUnexpectedEOF)
	}
	return nil
}

func (s *traceDBRowSink) openProfilerSourceOrderSidecar(
	manifest profilerSourceOrderSidecarManifest,
	registered bool,
) (*os.File, error) {
	classify := func(err error) error {
		if registered {
			return traceDBRunInputIntegrity(err)
		}
		return err
	}
	if !manifest.present() || manifest.size == 0 || manifest.rowCount == 0 {
		return nil, classify(&traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_manifest_invalid",
		})
	}
	artifact := s.artifacts[manifest.path]
	if artifact == nil || artifact.removed {
		return nil, classify(&traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_artifact_missing",
		})
	}
	if err := s.runFault("sidecar_open", manifest.path); err != nil {
		return nil, classify(err)
	}
	file, err := s.options.ops.open(manifest.path)
	if err != nil {
		return nil, classify(traceDBSorterOperationError("sidecar_open", err))
	}
	if err := s.noteRunFDOpen(); err != nil {
		return nil, errors.Join(err, traceDBSorterOperationError("sidecar_close", file.Close()))
	}
	info, err := s.profilerSourceOrderSidecarFstat(file, manifest.path)
	if err != nil {
		return nil, traceDBJoinPreservingSingle(
			classify(err), s.closeProfilerSourceOrderSidecarFile(file, manifest.path),
		)
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || uint64(info.Size()) != manifest.size {
		return nil, traceDBJoinPreservingSingle(
			classify(&traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_size_mismatch"}),
			s.closeProfilerSourceOrderSidecarFile(file, manifest.path),
		)
	}
	return file, nil
}

func (s *traceDBRowSink) validateOpenProfilerSourceOrderSidecar(
	ctx context.Context,
	file *os.File,
	manifest profilerSourceOrderSidecarManifest,
	requireDigest bool,
) (profilerSourceOrderSidecarAudit, error) {
	if ctx == nil {
		return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
			Reason: "trace_row_sort_context_missing",
		}
	}
	if err := ctx.Err(); err != nil {
		return profilerSourceOrderSidecarAudit{}, err
	}
	expectedCount, expectedRoot, err := s.expectedProfilerSourceOrderProof()
	if err != nil {
		return profilerSourceOrderSidecarAudit{}, err
	}
	if expectedCount == 0 || expectedCount != manifest.rowCount || expectedRoot != manifest.producerRoot {
		return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_manifest_proof_mismatch",
		}
	}
	expectedSize, err := profilerSourceOrderSidecarSize(expectedCount)
	if err != nil || expectedSize != manifest.size {
		return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_manifest_size_mismatch",
		}
	}

	var headerWire [profilerSourceOrderSidecarHeaderBytes]byte
	if err := s.readProfilerSourceOrderSidecarAt(file, manifest.path, headerWire[:], 0); err != nil {
		return profilerSourceOrderSidecarAudit{}, err
	}
	header, err := decodeProfilerSourceOrderSidecarHeader(headerWire[:])
	if err != nil {
		return profilerSourceOrderSidecarAudit{}, err
	}
	if header.rowCount != manifest.rowCount || header.producerRoot != manifest.producerRoot ||
		header.boundRunDigest != manifest.boundRunDigest {
		return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_header_proof_mismatch",
		}
	}

	physicalProof := sha256.New()
	_, _ = physicalProof.Write(headerWire[:])
	state := profilerSourceOrderInitialState()
	var step [profilerSourceOrderProofStepBytes]byte
	page := new([profilerSourceOrderSidecarRecordPageBytes]byte)
	audit := profilerSourceOrderSidecarAudit{}
	terminalBuilder := profilerTerminalPublicationBuilder{}
	var terminalErr error
	for first := uint64(0); first < manifest.rowCount; {
		if err := ctx.Err(); err != nil {
			return profilerSourceOrderSidecarAudit{}, err
		}
		records := manifest.rowCount - first
		if records > uint64(profilerSourceOrderSidecarPageRecords) {
			records = uint64(profilerSourceOrderSidecarPageRecords)
		}
		bytesInPage := int(records) * int(profilerSourceOrderSidecarRecordBytes)
		offset, err := profilerSourceOrderSidecarRecordOffset(first, manifest.rowCount)
		if err != nil {
			return profilerSourceOrderSidecarAudit{}, err
		}
		if err := s.readProfilerSourceOrderSidecarAt(
			file, manifest.path, page[:bytesInPage], offset,
		); err != nil {
			return profilerSourceOrderSidecarAudit{}, err
		}
		_, _ = physicalProof.Write(page[:bytesInPage])
		for index := uint64(0); index < records; index++ {
			ordinal := first + index
			start := int(index) * int(profilerSourceOrderSidecarRecordBytes)
			recordWire := page[start : start+int(profilerSourceOrderSidecarRecordBytes)]
			if profilerSourceOrderSidecarRecordUnwritten(recordWire) {
				return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
					Reason: "profiler_source_order_sidecar_unwritten_slot",
				}
			}
			record, err := decodeProfilerSourceOrderSidecarRecord(recordWire)
			if err != nil {
				return profilerSourceOrderSidecarAudit{}, err
			}
			if record.ordinalPlusOne != ordinal+1 {
				return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
					Reason: "profiler_source_order_sidecar_dense_ordinal_mismatch",
				}
			}
			state = advanceProfilerSourceOrderState(state, ordinal, record.leaf, &step)
			expectedDisposition, dispositionErr := s.typedProfilerSourceOrderDispositionForProvenance(
				record.provenance,
			)
			if dispositionErr != nil {
				return profilerSourceOrderSidecarAudit{}, dispositionErr
			}
			if expectedDisposition != record.disposition {
				return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
					Reason: "profiler_source_order_sidecar_disposition_mismatch",
				}
			}
			if terminalErr == nil && !terminalBuilder.observe(record.provenance, record.disposition) {
				terminalErr = &traceDBOutputInvariantError{
					Reason: "profiler_terminal_publication_ledger_record_invalid",
				}
			}
			if record.disposition.publishable() {
				if audit.published == math.MaxUint64 {
					return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
						Reason: "profiler_source_order_sidecar_disposition_overflow",
					}
				}
				audit.published++
			} else {
				if audit.withheld == math.MaxUint64 {
					return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
						Reason: "profiler_source_order_sidecar_disposition_overflow",
					}
				}
				audit.withheld++
			}
		}
		first += records
	}
	if terminalProfilerSourceOrderDigest(manifest.rowCount, state) != manifest.producerRoot {
		return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_root_mismatch",
		}
	}
	if terminalErr != nil {
		return profilerSourceOrderSidecarAudit{}, terminalErr
	}
	terminal, terminalOK := terminalBuilder.finish()
	if !terminalOK {
		return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_ledger_message_invalid",
		}
	}
	if err := s.validateProfilerTerminalPublicationLedger(terminal); err != nil {
		return profilerSourceOrderSidecarAudit{}, err
	}
	if terminal.rows.published != audit.published || terminal.rows.withheld != audit.withheld {
		return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_ledger_audit_mismatch",
		}
	}
	audit.terminal = terminal
	// Recheck the already-open file description after the complete bounded
	// read. This is the sidecar EOF proof: an append/truncate racing the initial
	// fstat cannot hide outside the manifest-sized hash domain.
	info, err := s.profilerSourceOrderSidecarFstat(file, manifest.path)
	if err != nil {
		return profilerSourceOrderSidecarAudit{}, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || uint64(info.Size()) != manifest.size {
		return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_size_mismatch",
		}
	}
	copy(audit.digest[:], physicalProof.Sum(nil))
	if requireDigest && audit.digest != manifest.digest {
		return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_digest_mismatch",
		}
	}
	if requireDigest && audit.terminal != manifest.terminal {
		return profilerSourceOrderSidecarAudit{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_ledger_manifest_mismatch",
		}
	}
	if err := ctx.Err(); err != nil {
		return profilerSourceOrderSidecarAudit{}, err
	}
	return audit, nil
}

func (s *traceDBRowSink) openValidatedProfilerSourceOrderSidecar(
	ctx context.Context,
	manifest profilerSourceOrderSidecarManifest,
	requireDigest bool,
) (*os.File, profilerSourceOrderSidecarAudit, error) {
	file, err := s.openProfilerSourceOrderSidecar(manifest, true)
	if err != nil {
		return nil, profilerSourceOrderSidecarAudit{}, err
	}
	audit, err := s.validateOpenProfilerSourceOrderSidecar(ctx, file, manifest, requireDigest)
	if err != nil {
		return nil, profilerSourceOrderSidecarAudit{}, traceDBJoinPreservingSingle(
			traceDBRunInputIntegrity(err),
			s.closeProfilerSourceOrderSidecarFile(file, manifest.path),
		)
	}
	return file, audit, nil
}

// profilerSourceOrderSidecarSemanticMismatch is intentionally narrower than
// the sidecar decoder's full error set. Header/version/size/reserved/sentinel
// corruption during the pre-manifest self-check belongs to local construction.
// Only a zero scatter slot (equal final-run cardinality therefore proves an
// ordinal hole/duplicate) or a dense replay root which disagrees with the
// frozen producer proof is registered-run integrity. Keep this closed list
// explicit so an implementation fault cannot be blamed on the customer source.
func profilerSourceOrderSidecarSemanticMismatch(err error) bool {
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok {
		return false
	}
	switch reason {
	case "profiler_source_order_sidecar_unwritten_slot",
		"profiler_source_order_sidecar_root_mismatch":
		return true
	default:
		return false
	}
}

func (s *traceDBRowSink) discardPendingProfilerSourceOrderSidecar(
	file *os.File,
	path string,
	reserved uint64,
	primary error,
) error {
	if file != nil {
		primary = traceDBJoinPreservingSingle(primary,
			s.closeProfilerSourceOrderSidecarFile(file, path))
	}
	removeErr := s.removeArtifact(path)
	primary = traceDBJoinPreservingSingle(primary, removeErr)
	if removeErr == nil {
		primary = traceDBJoinPreservingSingle(primary, s.releasePendingRun(reserved))
	}
	return primary
}

// profilerSourceOrderSidecarScatterWriter coalesces any contiguous ascending
// or descending ordinal run into a fixed-size ring. Production traces normally
// preserve timestamp/source locality, reducing millions of tiny pwrite calls
// to page-scale writes, while adversarial permutations retain a strict fixed
// memory bound and never acquire a per-row cache.
type profilerSourceOrderSidecarScatterWriter struct {
	sink     *traceDBRowSink
	file     *os.File
	path     string
	rowCount uint64
	buffer   [profilerSourceOrderSidecarRecordPageBytes]byte
	scratch  [profilerSourceOrderSidecarRecordBytes]byte
	head     int
	count    int
	first    uint64
	last     uint64
}

func (writer *profilerSourceOrderSidecarScatterWriter) start(
	ordinal uint64,
	wire []byte,
) {
	writer.head = 0
	writer.count = 1
	writer.first = ordinal
	writer.last = ordinal
	copy(writer.buffer[:int(profilerSourceOrderSidecarRecordBytes)], wire)
}

func (writer *profilerSourceOrderSidecarScatterWriter) flush(ctx context.Context) error {
	if writer == nil || writer.sink == nil || writer.file == nil || writer.path == "" ||
		writer.rowCount == 0 || writer.count < 0 ||
		writer.count > profilerSourceOrderSidecarPageRecords {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_scatter_state_invalid"}
	}
	if writer.count == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	recordBytes := int(profilerSourceOrderSidecarRecordBytes)
	firstRecords := min(writer.count, profilerSourceOrderSidecarPageRecords-writer.head)
	firstOffset, err := profilerSourceOrderSidecarRecordOffset(writer.first, writer.rowCount)
	if err != nil {
		return err
	}
	firstStart := writer.head * recordBytes
	firstEnd := firstStart + firstRecords*recordBytes
	if err := writer.sink.writeProfilerSourceOrderSidecarAt(
		writer.file, writer.path, writer.buffer[firstStart:firstEnd], firstOffset,
	); err != nil {
		return err
	}
	remaining := writer.count - firstRecords
	if remaining > 0 {
		if uint64(firstRecords) > math.MaxUint64-writer.first {
			return &traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_offset_invalid"}
		}
		secondOffset, offsetErr := profilerSourceOrderSidecarRecordOffset(
			writer.first+uint64(firstRecords), writer.rowCount,
		)
		if offsetErr != nil {
			return offsetErr
		}
		if err := writer.sink.writeProfilerSourceOrderSidecarAt(
			writer.file, writer.path, writer.buffer[:remaining*recordBytes], secondOffset,
		); err != nil {
			return err
		}
	}
	writer.count = 0
	writer.head = 0
	return nil
}

func (writer *profilerSourceOrderSidecarScatterWriter) add(
	ctx context.Context,
	ordinal uint64,
	wire []byte,
) error {
	if writer == nil || len(wire) != int(profilerSourceOrderSidecarRecordBytes) {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_scatter_record_invalid"}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if writer.count == 0 {
		writer.start(ordinal, wire)
		return nil
	}
	recordBytes := int(profilerSourceOrderSidecarRecordBytes)
	if writer.count < profilerSourceOrderSidecarPageRecords &&
		writer.last < math.MaxUint64 && ordinal == writer.last+1 {
		index := (writer.head + writer.count) % profilerSourceOrderSidecarPageRecords
		copy(writer.buffer[index*recordBytes:(index+1)*recordBytes], wire)
		writer.last = ordinal
		writer.count++
		return nil
	}
	if writer.count < profilerSourceOrderSidecarPageRecords &&
		writer.first > 0 && ordinal == writer.first-1 {
		writer.head = (writer.head - 1 + profilerSourceOrderSidecarPageRecords) %
			profilerSourceOrderSidecarPageRecords
		copy(writer.buffer[writer.head*recordBytes:(writer.head+1)*recordBytes], wire)
		writer.first = ordinal
		writer.count++
		return nil
	}
	if err := writer.flush(ctx); err != nil {
		return err
	}
	writer.start(ordinal, wire)
	return nil
}

func (writer *profilerSourceOrderSidecarScatterWriter) encodeAndAdd(
	ctx context.Context,
	ordinal uint64,
	leaf [sha256.Size]byte,
	provenance profilerPairRowProvenance,
	disposition profilerSourceOrderDisposition,
) error {
	if writer == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_scatter_state_invalid"}
	}
	if err := encodeProfilerSourceOrderSidecarRecordInto(
		writer.scratch[:], ordinal, leaf, provenance, disposition,
	); err != nil {
		return err
	}
	return writer.add(ctx, ordinal, writer.scratch[:])
}

func (s *traceDBRowSink) buildProfilerSourceOrderSidecar(ctx context.Context) error {
	if s == nil || s.captureLifecycle == profilerCaptureInactive ||
		s.sourceOrderSidecar.present() {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_build_state_invalid"}
	}
	expectedCount, expectedRoot, err := s.expectedProfilerSourceOrderProof()
	if err != nil {
		return err
	}
	if expectedCount == 0 {
		zeroRoot := terminalProfilerSourceOrderDigest(0, profilerSourceOrderInitialState())
		if expectedRoot != zeroRoot || len(s.runs) != 0 {
			return traceDBRunInputIntegrity(&traceDBOutputInvariantError{
				Reason: "profiler_source_order_zero_proof_mismatch",
			})
		}
		return nil
	}
	if len(s.runs) != 1 || s.runs[0].rowCount != expectedCount {
		return traceDBRunInputIntegrity(&traceDBOutputInvariantError{
			Reason: "profiler_source_order_final_run_missing",
		})
	}
	runManifest := s.runs[0]
	size, err := profilerSourceOrderSidecarSize(expectedCount)
	if err != nil {
		return err
	}
	if err := s.reservePendingRun(size, true); err != nil {
		return err
	}
	file, path, err := s.createPendingProfilerSourceOrderSidecar()
	if err != nil {
		return errors.Join(err, s.releasePendingRun(size))
	}
	fail := func(primary error) error {
		return s.discardPendingProfilerSourceOrderSidecar(file, path, size, primary)
	}
	info, err := s.profilerSourceOrderSidecarFstat(file, path)
	if err != nil {
		return fail(err)
	}
	if !info.Mode().IsRegular() || info.Size() != 0 {
		return fail(&traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_initial_file_invalid"})
	}
	if err := s.runFault("sidecar_preallocate", path); err != nil {
		return fail(err)
	}
	if err := s.options.ops.truncate(file, int64(size)); err != nil {
		return fail(traceDBSorterOperationError("sidecar_preallocate", err))
	}
	header := encodeProfilerSourceOrderSidecarHeader(profilerSourceOrderSidecarHeader{
		rowCount: expectedCount, producerRoot: expectedRoot, boundRunDigest: runManifest.digest,
	})
	if err := s.writeProfilerSourceOrderSidecarAt(file, path, header[:], 0); err != nil {
		return fail(err)
	}

	runReader, err := s.openAuthenticatedRunReader(runManifest)
	if err != nil {
		return fail(traceDBRunInputIntegrity(err))
	}
	leafBuilder := newProfilerSourceOrderLeafBuilder()
	scatterWriter := &profilerSourceOrderSidecarScatterWriter{
		sink: s, file: file, path: path, rowCount: expectedCount,
	}
	var rowsRead uint64
	var streamErr error
	for streamErr == nil {
		record, ok, nextErr := runReader.next(ctx)
		if nextErr != nil {
			streamErr = traceDBRunInputIntegrity(nextErr)
			break
		}
		if !ok {
			break
		}
		leaf, leafErr := leafBuilder.leafContext(ctx, record.row, record.ingestOrdinal)
		if leafErr != nil {
			streamErr = leafErr
			break
		}
		disposition, dispositionErr := s.typedProfilerSourceOrderDisposition(record.row)
		if dispositionErr != nil {
			// The producer registries and fixed counters were validated before
			// this pass. A compact run tuple which can no longer resolve against
			// that authority is registered-input drift, just like a leaf/root
			// mismatch; classify it before the row can influence publication.
			streamErr = traceDBRunInputIntegrity(dispositionErr)
			break
		}
		_, offsetErr := profilerSourceOrderSidecarRecordOffset(record.ingestOrdinal, expectedCount)
		if offsetErr != nil {
			streamErr = traceDBRunInputIntegrity(offsetErr)
			break
		}
		if writeErr := scatterWriter.encodeAndAdd(
			ctx, record.ingestOrdinal, leaf, record.row.profilerProvenance(), disposition,
		); writeErr != nil {
			streamErr = writeErr
			break
		}
		if rowsRead == math.MaxUint64 {
			streamErr = &traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_row_count_overflow"}
			break
		}
		rowsRead++
	}
	if streamErr == nil {
		streamErr = scatterWriter.flush(ctx)
	}
	streamErr = traceDBJoinPreservingSingle(streamErr, traceDBRunInputIntegrity(runReader.close()))
	if streamErr != nil {
		return fail(streamErr)
	}
	if rowsRead != expectedCount || s.runs[0] != runManifest {
		return fail(traceDBRunInputIntegrity(&traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_run_count_mismatch",
		}))
	}
	if err := s.closeProfilerSourceOrderSidecarFile(file, path); err != nil {
		file = nil
		return s.discardPendingProfilerSourceOrderSidecar(nil, path, size, err)
	}
	file = nil
	if err := s.runFault("sidecar_stat", path); err != nil {
		return s.discardPendingProfilerSourceOrderSidecar(nil, path, size, err)
	}
	info, err = s.options.ops.stat(path)
	if err != nil {
		return s.discardPendingProfilerSourceOrderSidecar(nil, path, size,
			traceDBSorterOperationError("sidecar_stat", err))
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || uint64(info.Size()) != size {
		return s.discardPendingProfilerSourceOrderSidecar(nil, path, size,
			&traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_size_mismatch"})
	}
	manifest := profilerSourceOrderSidecarManifest{
		path: path, size: size, rowCount: expectedCount,
		producerRoot: expectedRoot, boundRunDigest: runManifest.digest,
	}
	validationFile, validationErr := s.openProfilerSourceOrderSidecar(manifest, false)
	if validationErr != nil {
		return s.discardPendingProfilerSourceOrderSidecar(nil, path, size, validationErr)
	}
	audit, validationErr := s.validateOpenProfilerSourceOrderSidecar(
		ctx, validationFile, manifest, false,
	)
	closeErr := s.closeProfilerSourceOrderSidecarFile(validationFile, path)
	if validationErr != nil {
		if profilerSourceOrderSidecarSemanticMismatch(validationErr) {
			validationErr = traceDBRunInputIntegrity(validationErr)
		}
		validationErr = traceDBJoinPreservingSingle(validationErr, closeErr)
		return s.discardPendingProfilerSourceOrderSidecar(nil, path, size, validationErr)
	}
	if closeErr != nil {
		return s.discardPendingProfilerSourceOrderSidecar(nil, path, size, closeErr)
	}
	manifest.digest = audit.digest
	manifest.terminal = audit.terminal
	s.sourceOrderSidecar = manifest
	s.stats.TempBytes += int64(size)
	s.stats.SourceSidecarLogicalBytes = size
	s.stats.SourceSidecarPhysicalBytes = uint64(info.Size())
	return s.validateRunManifestSet(true)
}

type profilerSourceOrderSidecarRecordCache struct {
	page        [profilerSourceOrderSidecarRecordPageBytes]byte
	exact       [profilerSourceOrderSidecarRecordBytes]byte
	first       uint64
	records     uint64
	last        uint64
	locality    int
	initialized bool
	haveLast    bool
	pageMode    bool
}

type profilerSourceOrderPublicationProof struct {
	sink          *traceDBRowSink
	manifest      profilerSourceOrderSidecarManifest
	sidecar       *os.File
	run           *traceDBAuthenticatedRunReader
	leafBuilder   *profilerSourceOrderLeafBuilder
	cache         profilerSourceOrderSidecarRecordCache
	expectedAudit profilerSourceOrderSidecarAudit
}

func (proof *profilerSourceOrderPublicationProof) close() error {
	if proof == nil {
		return nil
	}
	var result error
	if proof.run != nil {
		result = traceDBJoinPreservingSingle(result, proof.run.close())
		proof.run = nil
	}
	if proof.sidecar != nil {
		result = traceDBJoinPreservingSingle(result,
			proof.sink.closeProfilerSourceOrderSidecarFile(proof.sidecar, proof.manifest.path))
		proof.sidecar = nil
	}
	return result
}

func (proof *profilerSourceOrderPublicationProof) sidecarRecord(
	ctx context.Context,
	ordinal uint64,
) (profilerSourceOrderSidecarRecord, error) {
	if proof == nil || proof.sidecar == nil || ordinal >= proof.manifest.rowCount {
		return profilerSourceOrderSidecarRecord{}, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_publication_state_invalid",
		}
	}
	if err := ctx.Err(); err != nil {
		return profilerSourceOrderSidecarRecord{}, err
	}
	cache := &proof.cache
	var wire []byte
	adjacent := cache.haveLast &&
		(cache.last < math.MaxUint64 && ordinal == cache.last+1 ||
			ordinal < math.MaxUint64 && cache.last == ordinal+1)
	if !adjacent {
		cache.locality = 0
		cache.pageMode = false
	} else if !cache.pageMode && cache.locality < profilerSourceOrderSidecarPageRecords {
		cache.locality++
		if cache.locality == profilerSourceOrderSidecarPageRecords {
			cache.pageMode = true
		}
	}
	if !cache.initialized || ordinal < cache.first || ordinal >= cache.first+cache.records {
		if cache.pageMode {
			cache.first = ordinal / uint64(profilerSourceOrderSidecarPageRecords) *
				uint64(profilerSourceOrderSidecarPageRecords)
			cache.records = proof.manifest.rowCount - cache.first
			if cache.records > uint64(profilerSourceOrderSidecarPageRecords) {
				cache.records = uint64(profilerSourceOrderSidecarPageRecords)
			}
			bytesInPage := int(cache.records) * int(profilerSourceOrderSidecarRecordBytes)
			offset, err := profilerSourceOrderSidecarRecordOffset(cache.first, proof.manifest.rowCount)
			if err != nil {
				return profilerSourceOrderSidecarRecord{}, err
			}
			if err := proof.sink.readProfilerSourceOrderSidecarAt(
				proof.sidecar, proof.manifest.path, cache.page[:bytesInPage], offset,
			); err != nil {
				return profilerSourceOrderSidecarRecord{}, err
			}
			cache.initialized = true
			index := ordinal - cache.first
			start := int(index) * int(profilerSourceOrderSidecarRecordBytes)
			wire = cache.page[start : start+int(profilerSourceOrderSidecarRecordBytes)]
		} else {
			offset, err := profilerSourceOrderSidecarRecordOffset(ordinal, proof.manifest.rowCount)
			if err != nil {
				return profilerSourceOrderSidecarRecord{}, err
			}
			if err := proof.sink.readProfilerSourceOrderSidecarAt(
				proof.sidecar, proof.manifest.path, cache.exact[:], offset,
			); err != nil {
				return profilerSourceOrderSidecarRecord{}, err
			}
			wire = cache.exact[:]
		}
	} else {
		index := ordinal - cache.first
		start := int(index) * int(profilerSourceOrderSidecarRecordBytes)
		wire = cache.page[start : start+int(profilerSourceOrderSidecarRecordBytes)]
	}
	if err := ctx.Err(); err != nil {
		return profilerSourceOrderSidecarRecord{}, err
	}
	record, err := decodeProfilerSourceOrderSidecarRecord(wire)
	if err != nil {
		return profilerSourceOrderSidecarRecord{}, err
	}
	if record.ordinalPlusOne != ordinal+1 {
		return profilerSourceOrderSidecarRecord{}, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_dense_ordinal_mismatch",
		}
	}
	cache.last = ordinal
	cache.haveLast = true
	return record, nil
}

func (proof *profilerSourceOrderPublicationProof) verifyRunRecord(
	ctx context.Context,
	record traceDBRunRecord,
) (profilerSourceOrderDisposition, error) {
	sidecarRecord, err := proof.sidecarRecord(ctx, record.ingestOrdinal)
	if err != nil {
		return profilerSourceOrderDispositionInvalid, err
	}
	leaf, err := proof.leafBuilder.leafContext(ctx, record.row, record.ingestOrdinal)
	if err != nil {
		return profilerSourceOrderDispositionInvalid, err
	}
	if leaf != sidecarRecord.leaf || record.row.profilerProvenance() != sidecarRecord.provenance {
		return profilerSourceOrderDispositionInvalid, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_row_mismatch",
		}
	}
	disposition, err := proof.sink.typedProfilerSourceOrderDisposition(record.row)
	if err != nil {
		return profilerSourceOrderDispositionInvalid, err
	}
	if disposition != sidecarRecord.disposition {
		return profilerSourceOrderDispositionInvalid, &traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_disposition_mismatch",
		}
	}
	return disposition, nil
}

func (s *traceDBRowSink) openProfilerSourceOrderPublicationProof(
	ctx context.Context,
) (*profilerSourceOrderPublicationProof, error) {
	if s == nil || s.captureLifecycle == profilerCaptureInactive || s.stats.RowsAccepted <= 0 ||
		len(s.runs) != 1 || !s.sourceOrderSidecar.present() {
		return nil, &traceDBOutputInvariantError{Reason: "profiler_source_order_publication_state_invalid"}
	}
	manifest := s.sourceOrderSidecar
	if manifest.boundRunDigest != s.runs[0].digest || manifest.rowCount != uint64(s.stats.RowsAccepted) {
		return nil, traceDBRunInputIntegrity(&traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_bound_run_mismatch",
		})
	}
	sidecar, audit, err := s.openValidatedProfilerSourceOrderSidecar(ctx, manifest, true)
	if err != nil {
		return nil, err
	}
	publishableRows, accountingErr := s.profilerPublishableRows()
	if accountingErr != nil || publishableRows < 0 ||
		audit.published != uint64(publishableRows) ||
		audit.withheld != uint64(s.stats.RowsAccepted-publishableRows) {
		if accountingErr == nil {
			accountingErr = &traceDBOutputInvariantError{
				Reason: "profiler_source_order_sidecar_disposition_accounting_mismatch",
			}
		}
		return nil, traceDBJoinPreservingSingle(
			traceDBRunInputIntegrity(accountingErr),
			s.closeProfilerSourceOrderSidecarFile(sidecar, manifest.path),
		)
	}
	proof := &profilerSourceOrderPublicationProof{
		sink: s, manifest: manifest, sidecar: sidecar,
		leafBuilder: newProfilerSourceOrderLeafBuilder(), expectedAudit: audit,
	}
	run, err := s.openAuthenticatedRunReader(s.runs[0])
	if err != nil {
		return nil, traceDBJoinPreservingSingle(traceDBRunInputIntegrity(err), proof.close())
	}
	proof.run = run
	var rowsRead uint64
	for {
		record, ok, nextErr := run.next(ctx)
		if nextErr != nil {
			return nil, traceDBJoinPreservingSingle(traceDBRunInputIntegrity(nextErr), proof.close())
		}
		if !ok {
			break
		}
		if _, verifyErr := proof.verifyRunRecord(ctx, record); verifyErr != nil {
			return nil, traceDBJoinPreservingSingle(traceDBRunInputIntegrity(verifyErr), proof.close())
		}
		rowsRead++
	}
	if rowsRead != manifest.rowCount {
		return nil, traceDBJoinPreservingSingle(traceDBRunInputIntegrity(&traceDBOutputInvariantError{
			Reason: "profiler_source_order_publication_count_mismatch",
		}), proof.close())
	}
	auditAgain, err := s.validateOpenProfilerSourceOrderSidecar(ctx, sidecar, manifest, true)
	if err != nil || auditAgain != audit {
		if err == nil {
			err = &traceDBOutputInvariantError{Reason: "profiler_source_order_sidecar_audit_drift"}
		}
		return nil, traceDBJoinPreservingSingle(traceDBRunInputIntegrity(err), proof.close())
	}
	if err := run.reset(); err != nil {
		return nil, traceDBJoinPreservingSingle(traceDBRunInputIntegrity(err), proof.close())
	}
	proof.cache.initialized = false
	proof.cache.haveLast = false
	proof.cache.pageMode = false
	proof.cache.locality = 0
	return proof, nil
}

func (proof *profilerSourceOrderPublicationProof) validateFinalSidecar(
	ctx context.Context,
) error {
	if proof == nil || proof.sidecar == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_publication_state_invalid"}
	}
	audit, err := proof.sink.validateOpenProfilerSourceOrderSidecar(
		ctx, proof.sidecar, proof.manifest, true,
	)
	if err != nil {
		return traceDBRunInputIntegrity(err)
	}
	if audit != proof.expectedAudit {
		return traceDBRunInputIntegrity(&traceDBOutputInvariantError{
			Reason: "profiler_source_order_sidecar_audit_drift",
		})
	}
	return nil
}
