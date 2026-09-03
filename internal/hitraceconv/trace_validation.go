package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceDBPostvalidationCoverageTable = tracebundle.SystraceReceiptTableSQL

const (
	traceDBPostvalidationCanceled          = "tracequery_postvalidation_canceled"
	traceDBPostvalidationCountMismatch     = "tracequery_postvalidation_count_mismatch"
	traceDBPostvalidationGenerationInvalid = "tracequery_postvalidation_generation_invalid"
	traceDBPostvalidationHeaderInvalid     = "tracequery_postvalidation_header_invalid"
	traceDBPostvalidationParsePanic        = "tracequery_postvalidation_parse_panic"
	traceDBPostvalidationScanFailed        = "tracequery_postvalidation_scan_failed"
	traceDBPostvalidationClockRegression   = "tracequery_postvalidation_clock_regression"
	traceDBPostvalidationUnknownOwnedRow   = "tracequery_postvalidation_unknown_owned_row"
	traceDBPostvalidationUnparsedOwnedRow  = "tracequery_postvalidation_unparsed_owned_row"
	traceDBPostvalidationEventTypeMismatch = "tracequery_postvalidation_event_type_mismatch"
	traceDBPostvalidationEventInvalid      = "tracequery_postvalidation_event_invalid"
	traceDBPostvalidationWireMismatch      = "tracequery_postvalidation_wire_mismatch"
	traceDBPostvalidationZeroRows          = "tracequery_postvalidation_zero_rows"
)

type ownedTraceValidationKind string

const (
	ownedTraceValidationSQL      ownedTraceValidationKind = "sql_systrace"
	ownedTraceValidationBuiltin  ownedTraceValidationKind = "builtin_systrace"
	ownedTraceValidationProfiler ownedTraceValidationKind = "profiler_systrace"
	ownedTraceValidationPerf     ownedTraceValidationKind = "perftrace"
)

type ownedTracePerfProfile string

const (
	ownedTracePerfSimpleperfText  ownedTracePerfProfile = "simpleperf_text"
	ownedTracePerfSimpleperfProto ownedTracePerfProfile = "simpleperf_proto"
	ownedTracePerfHiperfProto     ownedTracePerfProfile = "hiperf_proto"
	ownedTracePerfRaw             ownedTracePerfProfile = "raw_perf"
)

// sourceClock is the single authority for the public capability represented
// by a validated perf receipt. A generic perf receipt may not be re-labelled
// as a different producer with stronger CPU/identity semantics.
func (profile ownedTracePerfProfile) sourceClock() (source, clock string, ok bool) {
	switch profile {
	case ownedTracePerfSimpleperfText:
		return string(tracewire.PerfSampleSourceSimpleperfReportSample), string(tracewire.PerfSampleClockRecord), true
	case ownedTracePerfSimpleperfProto:
		return string(tracewire.PerfSampleSourceSimpleperfReportProto), string(tracewire.PerfSampleClockSimpleperfRecord), true
	case ownedTracePerfHiperfProto:
		return string(tracewire.PerfSampleSourceHiperfProto), string(tracewire.PerfSampleClockMonotonicRaw), true
	case ownedTracePerfRaw:
		return string(tracewire.PerfSampleSourceRawPerfDataFallback), string(tracewire.PerfSampleClockPerfData), true
	default:
		return "", "", false
	}
}

func (profile ownedTracePerfProfile) coverageTable() (string, bool) {
	switch profile {
	case ownedTracePerfSimpleperfText:
		return tracebundle.PerfReceiptTableSimpleperfText, true
	case ownedTracePerfSimpleperfProto:
		return tracebundle.PerfReceiptTableSimpleperfProto, true
	case ownedTracePerfHiperfProto:
		return tracebundle.PerfReceiptTableHiperfProto, true
	case ownedTracePerfRaw:
		return tracebundle.PerfReceiptTableRawPerf, true
	default:
		return "", false
	}
}

func (kind ownedTraceValidationKind) valid() bool {
	switch kind {
	case ownedTraceValidationSQL, ownedTraceValidationBuiltin, ownedTraceValidationProfiler, ownedTraceValidationPerf:
		return true
	default:
		return false
	}
}

// ownedTraceRowDigest binds an exceptional writer-declared row set to exact
// physical coordinates and bytes. It is used only for intentional inventory
// rows (builtin opaque advisory/header-only or provenance-approved profiler
// EventUnknown), never as a substitute for tracequery's semantic event
// classification.
type ownedTraceRowDigest struct {
	Rows   int
	SHA256 [sha256.Size]byte
	Valid  bool
}

type ownedTraceRowDigestBuilder struct {
	h        hash.Hash
	rows     int
	overflow bool
}

func (builder *ownedTraceRowDigestBuilder) add(line int, text string) {
	if builder == nil || builder.overflow || line <= 0 || builder.rows == math.MaxInt {
		if builder != nil {
			builder.overflow = true
		}
		return
	}
	if builder.h == nil {
		builder.h = sha256.New()
	}
	var envelope [16]byte
	binary.BigEndian.PutUint64(envelope[:8], uint64(line))
	binary.BigEndian.PutUint64(envelope[8:], uint64(len(text)))
	_, _ = builder.h.Write(envelope[:])
	_, _ = builder.h.Write([]byte(text))
	builder.rows++
}

func (builder *ownedTraceRowDigestBuilder) finish() ownedTraceRowDigest {
	if builder == nil || builder.overflow {
		return ownedTraceRowDigest{}
	}
	result := ownedTraceRowDigest{Rows: builder.rows, Valid: true}
	if builder.h == nil {
		result.SHA256 = sha256.Sum256(nil)
		return result
	}
	copy(result.SHA256[:], builder.h.Sum(nil))
	return result
}

func ownedTraceRowDigestEqual(expected, observed ownedTraceRowDigest) bool {
	if expected.Rows == 0 && !expected.Valid {
		return observed.Valid && observed.Rows == 0
	}
	return expected.Valid && observed.Valid && expected.Rows == observed.Rows && expected.SHA256 == observed.SHA256
}

type ownedTraceWireDigest struct {
	Bytes  int64
	SHA256 [sha256.Size]byte
	Valid  bool
}

// ownedTraceWireHasher is an io.Writer suitable for io.MultiWriter at the
// producer's single write throat. Callers consume finish only after the real
// writer, buffer flush, sync and close have all succeeded.
type ownedTraceWireHasher struct {
	h        hash.Hash
	bytes    int64
	overflow bool
}

func newOwnedTraceWireHasher() *ownedTraceWireHasher {
	return &ownedTraceWireHasher{h: sha256.New()}
}

func (hasher *ownedTraceWireHasher) Write(data []byte) (int, error) {
	if hasher == nil || hasher.h == nil || hasher.overflow {
		return 0, fmt.Errorf("owned trace wire hasher is unavailable")
	}
	if int64(len(data)) > math.MaxInt64-hasher.bytes {
		hasher.overflow = true
		return 0, fmt.Errorf("owned trace wire byte count overflow")
	}
	written, err := hasher.h.Write(data)
	if err != nil {
		return written, err
	}
	if written != len(data) {
		return written, fmt.Errorf("owned trace wire hash short write: wrote=%d want=%d", written, len(data))
	}
	hasher.bytes += int64(written)
	return written, nil
}

func (hasher *ownedTraceWireHasher) finish() ownedTraceWireDigest {
	if hasher == nil || hasher.h == nil || hasher.overflow {
		return ownedTraceWireDigest{}
	}
	result := ownedTraceWireDigest{Bytes: hasher.bytes, Valid: true}
	copy(result.SHA256[:], hasher.h.Sum(nil))
	return result
}

func ownedTraceWireDigestEqual(expected, observed ownedTraceWireDigest) bool {
	return expected.Valid && observed.Valid && expected.Bytes == observed.Bytes && expected.SHA256 == observed.SHA256
}

type ownedTraceValidationProfile struct {
	Kind                        ownedTraceValidationKind
	PerfProfile                 ownedTracePerfProfile
	CoverageTable               string
	ExpectedRows                int
	ExpectedKnown               int
	ExpectedTypedPreserved      int
	ExpectedSourceRawVisibility int
	ExpectedAdvisory            ownedTraceRowDigest
	ExpectedUnknown             ownedTraceRowDigest
	ExpectedUnparsed            ownedTraceRowDigest
	ExpectedWire                ownedTraceWireDigest
	RequiredEventType           tracequery.EventType
	RequiredPerfSource          string
	RequiredPerfClock           string
	RequirePerfIntegrity        bool
	AllowZeroRows               bool
	RawCaptureCompleteness      RawPerfCaptureCompleteness
	HasRawCaptureCompleteness   bool
	RawCaptureResidual          RawPerfCaptureResidual
	HasRawCaptureResidual       bool
	RawSampleAdmission          RawPerfSampleAdmission
	HasRawSampleAdmission       bool
}

// ownedTraceDBTextRecordSequence closes the integrity that a single-line
// parser cannot: multi-chunk logical-record SHA-256 plus canonical
// schema→row*→receipt table topology. It retains only one SHA state, never a
// source TEXT/BLOB value.
type ownedTraceDBTextRecordSequence struct {
	begun          bool
	open           bool
	carrierVersion int
	lastBlock      int
	lastKind       string
	tableID        int
	lastOrdinal    uint64
	currentKind    string
	currentTable   int
	currentRow     uint64
	currentChunks  int
	nextChunk      int
	currentHash    string
	hasher         hash.Hash
	digestScratch  [sha256.Size]byte
}

// ownedTraceDBRecordSequenceVerdict is the closed outcome of observing one
// parsed row against the typed trace_db record sequence. The zero value is
// deliberately not a member: a verdict that was never minted fails closed as a
// contract break (refusalKind).
type ownedTraceDBRecordSequenceVerdict uint8

const (
	ownedTraceDBRecordSequenceUnset ownedTraceDBRecordSequenceVerdict = iota
	// ownedTraceDBRecordSequenceAccepted: the row is consistent with the
	// sequence — an ordinary row before the typed suffix began, or a typed
	// record/block row that continues the chunk/ordinal/digest contract.
	ownedTraceDBRecordSequenceAccepted
	// ownedTraceDBRecordSequenceForeignRow: an ordinary (non trace_db record)
	// row was published after the typed suffix began. The suffix is the
	// artifact's tail by producer contract; the named row is the foreign one.
	ownedTraceDBRecordSequenceForeignRow
	// ownedTraceDBRecordSequenceContractBreak: a typed trace_db record/block
	// row itself breaks the chunk/ordinal/digest/topology contract.
	ownedTraceDBRecordSequenceContractBreak
)

// refusalKind maps a verdict to the typed witness kind it mints; accepted
// rows mint nothing. An unset verdict cannot be reasoned about and is refused
// as a contract break rather than admitted.
func (verdict ownedTraceDBRecordSequenceVerdict) refusalKind() (TraceEventInvalidKind, bool) {
	switch verdict {
	case ownedTraceDBRecordSequenceAccepted:
		return "", false
	case ownedTraceDBRecordSequenceForeignRow:
		return TraceEventInvalidTraceDBRecordSequenceForeignRow, true
	default:
		return TraceEventInvalidTraceDBRecordSequence, true
	}
}

func (sequence *ownedTraceDBTextRecordSequence) observe(event tracequery.Event) ownedTraceDBRecordSequenceVerdict {
	if sequence == nil {
		return ownedTraceDBRecordSequenceContractBreak
	}
	if event.Type != tracequery.EventTraceDBRecord {
		if sequence.begun {
			return ownedTraceDBRecordSequenceForeignRow
		}
		return ownedTraceDBRecordSequenceAccepted
	}
	if event.PluginFields == nil {
		return ownedTraceDBRecordSequenceContractBreak
	}
	if record := event.PluginFields.TraceDBRecord; record != nil {
		if event.PluginFields.TraceDBBlock != nil ||
			sequence.carrierVersion != 0 && sequence.carrierVersion != 1 {
			return ownedTraceDBRecordSequenceContractBreak
		}
		sequence.carrierVersion = 1
		if !sequence.observeRecord(record) {
			return ownedTraceDBRecordSequenceContractBreak
		}
		return ownedTraceDBRecordSequenceAccepted
	}
	block := event.PluginFields.TraceDBBlock
	if block == nil || sequence.carrierVersion != 0 && sequence.carrierVersion != 2 ||
		block.Block != sequence.lastBlock+1 ||
		block.RecordCount <= 0 || len(block.Records) != block.RecordCount {
		return ownedTraceDBRecordSequenceContractBreak
	}
	sequence.carrierVersion = 2
	for index := range block.Records {
		if !sequence.observeRecord(&block.Records[index]) {
			return ownedTraceDBRecordSequenceContractBreak
		}
	}
	sequence.lastBlock = block.Block
	return ownedTraceDBRecordSequenceAccepted
}

func (sequence *ownedTraceDBTextRecordSequence) observeRecord(record *tracequery.TraceDBRecordFields) bool {
	if sequence == nil || record == nil {
		return false
	}
	if record.PayloadBytes <= 0 || len(record.Payload) != record.PayloadBytes {
		return false
	}
	if record.Chunk == 1 {
		if sequence.open || !sequence.validRecordStart(record) {
			return false
		}
		sequence.begun = true
		sequence.open = true
		sequence.currentKind = record.Kind
		sequence.currentTable = record.TableID
		sequence.currentRow = record.RowOrdinal
		sequence.currentChunks = record.Chunks
		sequence.nextChunk = 1
		sequence.currentHash = record.RecordSHA256
		if sequence.hasher == nil {
			sequence.hasher = sha256.New()
		} else {
			sequence.hasher.Reset()
		}
	}
	if !sequence.open || record.Kind != sequence.currentKind ||
		record.TableID != sequence.currentTable ||
		record.RowOrdinal != sequence.currentRow ||
		record.Chunks != sequence.currentChunks ||
		record.Chunk != sequence.nextChunk ||
		record.RecordSHA256 != sequence.currentHash {
		return false
	}
	if _, err := sequence.hasher.Write(record.Payload); err != nil {
		return false
	}
	sequence.nextChunk++
	if record.Chunk != record.Chunks {
		return true
	}
	digest := sequence.hasher.Sum(sequence.digestScratch[:0])
	if len(digest) != sha256.Size ||
		!traceDBSHA256BytesMatchCanonicalHex(digest, sequence.currentHash) {
		return false
	}
	sequence.open = false
	sequence.lastKind = sequence.currentKind
	sequence.tableID = sequence.currentTable
	sequence.lastOrdinal = sequence.currentRow
	return true
}

func traceDBSHA256BytesMatchCanonicalHex(digest []byte, canonical string) bool {
	if len(digest) != sha256.Size || len(canonical) != sha256.Size*2 {
		return false
	}
	const lowerHex = "0123456789abcdef"
	for index, value := range digest {
		if canonical[index*2] != lowerHex[value>>4] ||
			canonical[index*2+1] != lowerHex[value&0x0f] {
			return false
		}
	}
	return true
}

func (sequence *ownedTraceDBTextRecordSequence) validRecordStart(record *tracequery.TraceDBRecordFields) bool {
	if record == nil {
		return false
	}
	switch sequence.lastKind {
	case "":
		return record.Kind == "schema" && record.TableID == 1 && record.RowOrdinal == 0
	case "schema":
		return record.TableID == sequence.tableID &&
			(record.Kind == "receipt" && record.RowOrdinal == 0 ||
				record.Kind == "row" && record.RowOrdinal == 1)
	case "row":
		return record.TableID == sequence.tableID &&
			(record.Kind == "receipt" && record.RowOrdinal == 0 ||
				record.Kind == "row" && record.RowOrdinal == sequence.lastOrdinal+1)
	case "receipt":
		return record.Kind == "schema" && record.TableID == sequence.tableID+1 &&
			record.RowOrdinal == 0
	default:
		return false
	}
}

func (sequence *ownedTraceDBTextRecordSequence) complete(expectedLines int) bool {
	if expectedLines == 0 {
		return sequence != nil && !sequence.begun && !sequence.open
	}
	return sequence != nil && sequence.begun && !sequence.open &&
		sequence.lastKind == "receipt"
}

func (profile ownedTraceValidationProfile) validate() string {
	if !profile.Kind.valid() || profile.ExpectedRows < 0 || profile.ExpectedKnown < 0 ||
		profile.ExpectedTypedPreserved < 0 || profile.ExpectedSourceRawVisibility < 0 ||
		profile.ExpectedAdvisory.Rows < 0 ||
		profile.ExpectedUnknown.Rows < 0 || profile.ExpectedUnparsed.Rows < 0 {
		return traceDBPostvalidationCountMismatch
	}
	if profile.ExpectedRows == 0 && !profile.AllowZeroRows {
		return traceDBPostvalidationZeroRows
	}
	if profile.ExpectedKnown > math.MaxInt-profile.ExpectedUnknown.Rows ||
		profile.ExpectedKnown+profile.ExpectedUnknown.Rows > math.MaxInt-profile.ExpectedUnparsed.Rows ||
		profile.ExpectedRows != profile.ExpectedKnown+profile.ExpectedUnknown.Rows+profile.ExpectedUnparsed.Rows {
		return traceDBPostvalidationCountMismatch
	}
	if profile.ExpectedAdvisory.Rows > 0 && !profile.ExpectedAdvisory.Valid ||
		profile.ExpectedUnknown.Rows > 0 && !profile.ExpectedUnknown.Valid ||
		profile.ExpectedUnparsed.Rows > 0 && !profile.ExpectedUnparsed.Valid {
		return traceDBPostvalidationCountMismatch
	}
	switch profile.Kind {
	case ownedTraceValidationSQL:
		if profile.PerfProfile != "" || profile.AllowZeroRows || profile.ExpectedRows <= 0 || profile.ExpectedKnown != profile.ExpectedRows ||
			profile.ExpectedTypedPreserved > profile.ExpectedKnown ||
			profile.ExpectedSourceRawVisibility > profile.ExpectedKnown-profile.ExpectedTypedPreserved ||
			profile.ExpectedAdvisory.Rows != 0 || profile.ExpectedUnknown.Rows != 0 || profile.ExpectedUnparsed.Rows != 0 || profile.RequiredEventType != "" ||
			profile.RequirePerfIntegrity || profile.RequiredPerfSource != "" || profile.RequiredPerfClock != "" ||
			profile.HasRawCaptureCompleteness || profile.RawCaptureCompleteness != (RawPerfCaptureCompleteness{}) ||
			profile.HasRawCaptureResidual || profile.RawCaptureResidual != (RawPerfCaptureResidual{}) ||
			profile.HasRawSampleAdmission || profile.RawSampleAdmission != (RawPerfSampleAdmission{}) ||
			profile.CoverageTable != tracebundle.SystraceReceiptTableSQL {
			return traceDBPostvalidationCountMismatch
		}
	case ownedTraceValidationBuiltin:
		advisoryKnown := profile.ExpectedAdvisory.Rows - profile.ExpectedUnknown.Rows
		if profile.PerfProfile != "" || !profile.AllowZeroRows || profile.ExpectedTypedPreserved != 0 || profile.ExpectedSourceRawVisibility != 0 ||
			profile.ExpectedAdvisory.Rows < profile.ExpectedUnknown.Rows ||
			advisoryKnown < 0 || advisoryKnown > profile.ExpectedKnown || profile.RequiredEventType != "" ||
			profile.RequirePerfIntegrity || profile.RequiredPerfSource != "" || profile.RequiredPerfClock != "" ||
			profile.HasRawCaptureCompleteness || profile.RawCaptureCompleteness != (RawPerfCaptureCompleteness{}) ||
			profile.HasRawCaptureResidual || profile.RawCaptureResidual != (RawPerfCaptureResidual{}) ||
			profile.HasRawSampleAdmission || profile.RawSampleAdmission != (RawPerfSampleAdmission{}) ||
			!profile.ExpectedWire.Valid || profile.CoverageTable != tracebundle.SystraceReceiptTableBuiltin {
			return traceDBPostvalidationCountMismatch
		}
	case ownedTraceValidationProfiler:
		if profile.PerfProfile != "" || profile.AllowZeroRows || profile.ExpectedRows <= 0 || profile.ExpectedTypedPreserved != 0 || profile.ExpectedSourceRawVisibility != 0 ||
			profile.ExpectedAdvisory.Rows != 0 || profile.ExpectedUnparsed.Rows != 0 ||
			profile.RequiredEventType != "" || profile.RequirePerfIntegrity || profile.RequiredPerfSource != "" ||
			profile.RequiredPerfClock != "" || profile.HasRawCaptureCompleteness ||
			profile.RawCaptureCompleteness != (RawPerfCaptureCompleteness{}) || profile.HasRawCaptureResidual ||
			profile.RawCaptureResidual != (RawPerfCaptureResidual{}) || profile.HasRawSampleAdmission ||
			profile.RawSampleAdmission != (RawPerfSampleAdmission{}) || !profile.ExpectedWire.Valid ||
			profile.CoverageTable != tracebundle.SystraceReceiptTableProfiler {
			return traceDBPostvalidationCountMismatch
		}
	case ownedTraceValidationPerf:
		requiredSource, requiredClock, validPerfProfile := profile.PerfProfile.sourceClock()
		expectedTable, validCoverageTable := profile.PerfProfile.coverageTable()
		if !validPerfProfile || !validCoverageTable || profile.ExpectedKnown != profile.ExpectedRows ||
			profile.ExpectedTypedPreserved != 0 || profile.ExpectedSourceRawVisibility != 0 ||
			profile.ExpectedAdvisory.Rows != 0 || profile.ExpectedUnknown.Rows != 0 || profile.ExpectedUnparsed.Rows != 0 ||
			profile.RequiredEventType != tracequery.EventPerfSample || !profile.RequirePerfIntegrity ||
			profile.RequiredPerfSource != requiredSource || profile.RequiredPerfClock != requiredClock || !profile.ExpectedWire.Valid ||
			profile.CoverageTable != expectedTable {
			return traceDBPostvalidationCountMismatch
		}
		if profile.PerfProfile != ownedTracePerfRaw {
			if profile.AllowZeroRows || profile.ExpectedRows <= 0 || profile.HasRawCaptureCompleteness ||
				profile.RawCaptureCompleteness != (RawPerfCaptureCompleteness{}) || profile.HasRawCaptureResidual ||
				profile.RawCaptureResidual != (RawPerfCaptureResidual{}) || profile.HasRawSampleAdmission ||
				profile.RawSampleAdmission != (RawPerfSampleAdmission{}) {
				return traceDBPostvalidationCountMismatch
			}
			break
		}
		if !profile.HasRawCaptureCompleteness || !profile.HasRawCaptureResidual || !profile.HasRawSampleAdmission ||
			validateRawPerfCaptureCompleteness(profile.RawCaptureCompleteness) != "" ||
			validateRawPerfCaptureResidual(profile.RawCaptureResidual) != "" ||
			validateRawPerfSampleAdmission(profile.RawSampleAdmission) != "" ||
			profile.RawSampleAdmission.Candidates != profile.RawCaptureCompleteness.SampleRecords.Accepted ||
			profile.RawSampleAdmission.QueryRows > uint64(math.MaxInt) ||
			int(profile.RawSampleAdmission.QueryRows) != profile.ExpectedRows ||
			profile.AllowZeroRows != (profile.ExpectedRows == 0) {
			return traceDBPostvalidationCountMismatch
		}
		if profile.ExpectedRows == 0 {
			hasIssue, err := rawPerfCaptureHasPublicationIssue(profile.RawCaptureCompleteness)
			if err != nil || !hasIssue && !rawPerfSampleAdmissionHasIssue(profile.RawSampleAdmission) {
				return traceDBPostvalidationZeroRows
			}
		}
	}
	return ""
}

type ownedTraceValidationReceipt struct {
	kind                      ownedTraceValidationKind
	perfProfile               ownedTracePerfProfile
	perfSource                string
	perfClock                 string
	sourceIdentity            filegeneration.Identity
	size                      int64
	rows                      int
	known                     int
	authoritativeKnown        int
	advisory                  int
	unknown                   int
	unparsed                  int
	queryReady                bool
	rawCaptureCompleteness    RawPerfCaptureCompleteness
	hasRawCaptureCompleteness bool
	rawCaptureResidual        RawPerfCaptureResidual
	hasRawCaptureResidual     bool
	rawSampleAdmission        RawPerfSampleAdmission
	hasRawSampleAdmission     bool
	coverage                  TraceDBCoverage
	wireSHA256                [sha256.Size]byte
}

// ownedBuiltinAdvisoryEvent is the held-side half of the builtin opaque marker
// receipt. Exact event names keep native non-print rows out of the exception;
// the type roster is the complete output of the existing print-family plugin
// chain for a non-trace-mark payload. EventTraceMark is deliberately absent.
func ownedBuiltinAdvisoryEvent(name string, eventType tracequery.EventType) bool {
	if name != "print" && name != "tracing_mark_write" {
		return false
	}
	switch eventType {
	case tracequery.EventUnknown,
		tracequery.EventAbilityMonitor,
		tracequery.EventXPower,
		tracequery.EventHiSystemEvent,
		tracequery.EventHiLog:
		return true
	default:
		return false
	}
}

type ownedTraceOutputInvariantError struct {
	Reason string
	Cause  error
}

// TraceClockRegressionWitnessError is the bounded, customer-safe first
// witness attached to a generated-output monotonicity failure. It contains no
// trace payload or private staging path; diagnostic-report consumers can
// recover it with errors.As through the provider error graph.
type TraceClockRegressionWitnessError struct {
	PreviousLine         int
	CurrentLine          int
	PreviousTimestampSec float64
	CurrentTimestampSec  float64
	PreviousEventType    tracequery.EventType
	CurrentEventType     tracequery.EventType
}

func (failure *TraceClockRegressionWitnessError) Error() string {
	if failure == nil {
		return "first generated trace timestamp regression"
	}
	return fmt.Sprintf(
		"first generated trace timestamp regression: previous_line=%d previous_timestamp_sec=%.9f previous_event_type=%s current_line=%d current_timestamp_sec=%.9f current_event_type=%s",
		failure.PreviousLine,
		failure.PreviousTimestampSec,
		failure.PreviousEventType,
		failure.CurrentLine,
		failure.CurrentTimestampSec,
		failure.CurrentEventType,
	)
}

// TraceEventInvalidKind names, as a precise typed value, which owned-output
// row contract a refused row broke.
type TraceEventInvalidKind string

const (
	// TraceEventInvalidCarrierSignatureUnderForeignHeader: the row body
	// starts with a reserved codrax carrier wire token (`codrax_<family>/v<N>`)
	// but the row header is not the reserved carrier event name — a producer
	// squatted the reserved namespace (or a carrier wore a semantic name).
	TraceEventInvalidCarrierSignatureUnderForeignHeader TraceEventInvalidKind = "carrier_signature_under_foreign_header"
	// TraceEventInvalidSourceRawVisibilityForeignHeader: a parsed visibility
	// carrier is published under a header other than the reserved name
	// (defense in depth behind the body-signature arm, which witnesses the
	// same row first because a carrier body always starts with the wire).
	TraceEventInvalidSourceRawVisibilityForeignHeader TraceEventInvalidKind = "source_raw_visibility_foreign_header"
	// TraceEventInvalidTraceDBRecordSequence: a typed trace_db record/block
	// row (a `# codrax_trace_db_record/v1` / `# codrax_trace_db_block/v2`
	// comment carrier) itself breaks the chunk/ordinal/digest/topology
	// sequence contract. The named row is that carrier; BodyPrefix shows the
	// bytes after its `# <wire> ` prefix.
	TraceEventInvalidTraceDBRecordSequence TraceEventInvalidKind = "trace_db_record_sequence"
	// TraceEventInvalidTraceDBRecordSequenceForeignRow: an ordinary ftrace row
	// was published after the typed trace_db record suffix began. The typed
	// suffix is the artifact's tail by producer contract, so the foreign row —
	// not the record sequence — is at fault; it is the row named (§40.43
	// F-carrier-2 G).
	TraceEventInvalidTraceDBRecordSequenceForeignRow TraceEventInvalidKind = "trace_db_record_sequence_foreign_row"
	// TraceEventInvalidTraceDBRecordSequenceIncomplete: the typed record
	// sequence ended before the expected count; no single row is at fault
	// (Line is 0).
	TraceEventInvalidTraceDBRecordSequenceIncomplete TraceEventInvalidKind = "trace_db_record_sequence_incomplete"
	// TraceEventInvalidPerfSampleIntegrity: a perf sample row misses the
	// required integrity/source/clock/period contract.
	TraceEventInvalidPerfSampleIntegrity TraceEventInvalidKind = "perf_sample_integrity"
)

// AllTraceEventInvalidKinds is the closed kind set in declaration order —
// the single source the diagnostic-report advertisement census (cmd) binds
// to; a kind declared above but missing here is red in this package's
// census (trace_validation_witness_test.go).
func AllTraceEventInvalidKinds() []TraceEventInvalidKind {
	return []TraceEventInvalidKind{
		TraceEventInvalidCarrierSignatureUnderForeignHeader,
		TraceEventInvalidSourceRawVisibilityForeignHeader,
		TraceEventInvalidTraceDBRecordSequence,
		TraceEventInvalidTraceDBRecordSequenceForeignRow,
		TraceEventInvalidTraceDBRecordSequenceIncomplete,
		TraceEventInvalidPerfSampleIntegrity,
	}
}

// TraceEventInvalidWitnessError is the bounded, customer-safe FIRST witness
// attached to a tracequery_postvalidation_event_invalid refusal: it names the
// refused row (physical line, parsed event name/type) and the first bytes of
// its body so the customer can see which producer wrote it. It carries no
// private staging path; diagnostic-report consumers recover it with
// errors.As through the provider error graph (§40.38 fold-in F8).
type TraceEventInvalidWitnessError struct {
	Kind       TraceEventInvalidKind
	Line       int
	EventName  string
	EventType  tracequery.EventType
	BodyPrefix string
}

func (failure *TraceEventInvalidWitnessError) Error() string {
	if failure == nil {
		return "first generated trace row refused"
	}
	return fmt.Sprintf("first generated trace row refused: kind=%s line=%d event_name=%s event_type=%s body_prefix=%q",
		failure.Kind, failure.Line, failure.EventName, failure.EventType, failure.BodyPrefix)
}

func (failure *ownedTraceOutputInvariantError) Error() string {
	if failure == nil {
		return "owned trace output invariant rejected"
	}
	return "owned trace output invariant rejected: " + failure.Reason
}

func (failure *ownedTraceOutputInvariantError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

func ownedTraceOutputInvariantReason(err error) (reason string, cause error, ok bool) {
	var failure *ownedTraceOutputInvariantError
	if !errors.As(err, &failure) || failure == nil {
		return "", nil, false
	}
	return failure.Reason, failure.Cause, true
}

func newTraceDBPostvalidationCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: tracebundle.SystraceReceiptFamily,
		Table:  traceDBPostvalidationCoverageTable,
		Role:   tracebundle.SystraceReceiptRole,
		Found:  true,
	}
}

// validateSealedSystraceWithTraceQuery is the converter-owned query-ready
// admission gate. It consumes the exact held generation, never reopens the
// public path, and keeps the tracequery parser's event memory at O(1).
//
// The SQL writer always prefixes its rows with the fixed standard systrace
// header. Header bytes are therefore checked separately on the same handle;
// only those exact comment rows may account for UnparsedLines.
func validateSealedSystraceWithTraceQuery(ctx context.Context, source *sealedConversionFile, displayPath string, expectedRows int) (coverage TraceDBCoverage, resultErr error) {
	_, coverage, resultErr = validateSealedSystraceWithTraceQueryReceipt(ctx, source, displayPath, expectedRows)
	return coverage, resultErr
}

func validateSealedSystraceWithTraceQueryReceipt(
	ctx context.Context,
	source *sealedConversionFile,
	displayPath string,
	expectedRows int,
	expectedTypedPreservedValues ...int,
) (receipt ownedTraceValidationReceipt, coverage TraceDBCoverage, resultErr error) {
	expectedTypedPreserved := 0
	if len(expectedTypedPreservedValues) == 1 {
		expectedTypedPreserved = expectedTypedPreservedValues[0]
	} else if len(expectedTypedPreservedValues) > 1 {
		coverage = newTraceDBPostvalidationCoverage()
		coverage.Error = traceDBPostvalidationCountMismatch
		return receipt, coverage, &traceDBOutputInvariantError{Reason: traceDBPostvalidationCountMismatch}
	}
	profile := ownedTraceValidationProfile{
		Kind:                   ownedTraceValidationSQL,
		CoverageTable:          traceDBPostvalidationCoverageTable,
		ExpectedRows:           expectedRows,
		ExpectedKnown:          expectedRows,
		ExpectedTypedPreserved: expectedTypedPreserved,
	}
	receipt, coverage, err := validateOwnedTraceOutput(ctx, source, displayPath, profile)
	if err == nil {
		return receipt, coverage, nil
	}
	if reason, cause, ok := ownedTraceOutputInvariantReason(err); ok {
		return ownedTraceValidationReceipt{}, coverage, &traceDBOutputInvariantError{Reason: reason, Cause: cause}
	}
	return ownedTraceValidationReceipt{}, coverage, err
}

func validateSealedSystraceWithTraceQueryReceiptAndWire(
	ctx context.Context,
	source *sealedConversionFile,
	displayPath string,
	expectedRows int,
	expectedTypedPreserved int,
	expectedSourceRawVisibility int,
	expectedWire ownedTraceWireDigest,
) (receipt ownedTraceValidationReceipt, coverage TraceDBCoverage, resultErr error) {
	if !expectedWire.Valid {
		coverage = newTraceDBPostvalidationCoverage()
		coverage.Error = traceDBPostvalidationWireMismatch
		return receipt, coverage, &traceDBOutputInvariantError{Reason: traceDBPostvalidationWireMismatch}
	}
	profile := ownedTraceValidationProfile{
		Kind:                        ownedTraceValidationSQL,
		CoverageTable:               traceDBPostvalidationCoverageTable,
		ExpectedRows:                expectedRows,
		ExpectedKnown:               expectedRows,
		ExpectedTypedPreserved:      expectedTypedPreserved,
		ExpectedSourceRawVisibility: expectedSourceRawVisibility,
		ExpectedWire:                expectedWire,
	}
	receipt, coverage, err := validateOwnedTraceOutput(ctx, source, displayPath, profile)
	if err == nil {
		return receipt, coverage, nil
	}
	if reason, cause, ok := ownedTraceOutputInvariantReason(err); ok {
		return ownedTraceValidationReceipt{}, coverage, &traceDBOutputInvariantError{Reason: reason, Cause: cause}
	}
	return ownedTraceValidationReceipt{}, coverage, err
}

func validateOwnedTraceOutput(
	ctx context.Context,
	source *sealedConversionFile,
	displayPath string,
	profile ownedTraceValidationProfile,
) (receipt ownedTraceValidationReceipt, coverage TraceDBCoverage, resultErr error) {
	start := time.Now()
	defer func() {
		traceDBSetCoverageElapsed(&coverage, start)
		if resultErr == nil && receipt.kind.valid() {
			receipt.coverage = coverage
		}
	}()
	coverage = newTraceDBPostvalidationCoverage()
	if strings.TrimSpace(profile.CoverageTable) != "" {
		coverage.Table = strings.TrimSpace(profile.CoverageTable)
	}
	if profile.HasRawCaptureCompleteness {
		coverage.RawCaptureCompleteness = cloneRawPerfCaptureCompleteness(profile.RawCaptureCompleteness)
	}
	if profile.HasRawCaptureResidual {
		coverage.RawCaptureResidual = cloneRawPerfCaptureResidual(profile.RawCaptureResidual)
	}
	if profile.HasRawSampleAdmission {
		coverage.RawSampleAdmission = cloneRawPerfSampleAdmission(profile.RawSampleAdmission)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	fail := func(reason string, cause ...error) error {
		coverage.Error = reason
		return &ownedTraceOutputInvariantError{Reason: reason, Cause: traceDBJoinPreservingSingle(nil, cause...)}
	}
	if reason := profile.validate(); reason != "" {
		return receipt, coverage, fail(reason)
	}
	if profile.Kind != ownedTraceValidationPerf {
		if displayPath == "" || displayPath != strings.TrimSpace(displayPath) {
			return receipt, coverage, fail(traceDBPostvalidationGenerationInvalid)
		}
	}
	if source == nil {
		coverage.Found = false
		return receipt, coverage, fail(traceDBPostvalidationGenerationInvalid)
	}
	if err := ctx.Err(); err != nil {
		coverage.Error = traceDBPostvalidationCanceled
		return receipt, coverage, err
	}
	if err := source.Validate(); err != nil {
		coverage.Found = false
		return receipt, coverage, fail(traceDBPostvalidationGenerationInvalid, err)
	}

	headerLines := strings.Count(systraceHeader, "\n")
	callbackCount := 0
	knownCount := 0
	unknownCount := 0
	typedPreservedCount := 0
	sourceRawVisibilityCount := 0
	callbackOverflow := false
	parsedHeaderRow := false
	eventTypeMismatch := false
	eventInvalid := false
	var firstEventInvalid *TraceEventInvalidWitnessError
	// The line observer runs before the event callback for the same physical
	// line (tracequery streamScanReader), so the event-side refusals can name
	// the row text they were minted from.
	var lastObservation tracequery.HeldLineObservation
	refuseEvent := func(kind TraceEventInvalidKind, line int, eventName string, eventType tracequery.EventType, text string) {
		eventInvalid = true
		if firstEventInvalid != nil {
			return
		}
		firstEventInvalid = &TraceEventInvalidWitnessError{
			Kind: kind, Line: line, EventName: eventName, EventType: eventType,
			BodyPrefix: traceEventInvalidWitnessBodyPrefix(text, eventName),
		}
	}
	refuseParsedEvent := func(kind TraceEventInvalidKind, event tracequery.Event) {
		text := ""
		if lastObservation.Line == event.Line {
			text = lastObservation.Text
		}
		refuseEvent(kind, event.Line, event.Name, event.Type, text)
	}
	var typedSequence ownedTraceDBTextRecordSequence
	var advisoryDigest ownedTraceRowDigestBuilder
	var unknownDigest ownedTraceRowDigestBuilder
	var unparsedDigest ownedTraceRowDigestBuilder
	wireHasher := newOwnedTraceWireHasher()
	wireHashFailed := false
	var firstClockRegression *TraceClockRegressionWitnessError
	var previousPositiveTimestampSec float64
	var previousPositiveTimestampLine int
	var previousPositiveTimestampType tracequery.EventType
	var idx *tracequery.Index
	operationReason := ""
	operationErr := source.withOpenFile(func(file *os.File) error {
		if err := ctx.Err(); err != nil {
			operationReason = traceDBPostvalidationCanceled
			return err
		}
		header := make([]byte, len(systraceHeader))
		n, err := file.ReadAt(header, 0)
		if err != nil || n != len(header) || !bytes.Equal(header, []byte(systraceHeader)) {
			operationReason = traceDBPostvalidationHeaderInvalid
			if err != nil {
				return err
			}
			return errors.New("generated systrace header bytes differ from the fixed writer contract")
		}
		idx, err = tracequery.StreamScanHeldFileWithLineObserver(
			ctx,
			file,
			displayPath,
			tracequery.TraceFlavorAuto,
			maxTraceDBSystraceLineBytes,
			func(event tracequery.Event) bool {
				if event.Line <= headerLines {
					parsedHeaderRow = true
					return true
				}
				if event.Ts > 0 {
					if firstClockRegression == nil && previousPositiveTimestampSec > 0 && event.Ts < previousPositiveTimestampSec {
						firstClockRegression = &TraceClockRegressionWitnessError{
							PreviousLine:         previousPositiveTimestampLine,
							CurrentLine:          event.Line,
							PreviousTimestampSec: previousPositiveTimestampSec,
							CurrentTimestampSec:  event.Ts,
							PreviousEventType:    previousPositiveTimestampType,
							CurrentEventType:     event.Type,
						}
					}
					previousPositiveTimestampSec = event.Ts
					previousPositiveTimestampLine = event.Line
					previousPositiveTimestampType = event.Type
				}
				if callbackCount == math.MaxInt {
					callbackOverflow = true
					return true
				}
				callbackCount++
				if event.Type == tracequery.EventUnknown {
					unknownCount++
				} else {
					knownCount++
				}
				if event.Type == tracequery.EventTraceDBRecord {
					typedPreservedCount++
				}
				if event.Type == tracequery.EventSourceRawVisibility {
					sourceRawVisibilityCount++
					// Emission census (colleague_merge_audit §40.13): every
					// visibility carrier passes this single throat, so a carrier
					// published under any header other than the reserved name
					// fails the owned output closed here. The parser stays
					// name-agnostic; the reserved name is a producer contract.
					if event.Name != tracequery.SourceRawVisibilityEventName {
						refuseParsedEvent(TraceEventInvalidSourceRawVisibilityForeignHeader, event)
					}
				}
				if kind, refused := typedSequence.observe(event).refusalKind(); refused {
					refuseParsedEvent(kind, event)
				}
				if profile.RequiredEventType != "" && event.Type != tracequery.EventUnknown && event.Type != profile.RequiredEventType {
					eventTypeMismatch = true
				}
				if profile.RequirePerfIntegrity && event.Type == tracequery.EventPerfSample {
					perf := event.PerfFields
					if perf == nil || perf.PerfTextIntegrity != "" || perf.PerfWeightInvalid || perf.Period <= 0 ||
						(profile.RequiredPerfSource != "" && perf.Source != profile.RequiredPerfSource) ||
						(profile.RequiredPerfClock != "" && perf.Clock != profile.RequiredPerfClock) {
						refuseParsedEvent(TraceEventInvalidPerfSampleIntegrity, event)
					}
				}
				return true
			},
			func(observation tracequery.HeldLineObservation) {
				if _, err := wireHasher.Write([]byte(observation.Text)); err != nil {
					wireHashFailed = true
				}
				if _, err := wireHasher.Write([]byte{'\n'}); err != nil {
					wireHashFailed = true
				}
				if observation.Line <= headerLines {
					if observation.Parsed {
						parsedHeaderRow = true
					}
					return
				}
				lastObservation = observation
				if observation.Parsed && observation.EventType == tracequery.EventUnknown {
					unknownDigest.add(observation.Line, observation.Text)
				}
				// Emission census, body half (§40.13 复核): the `codrax_<family>/v<N>`
				// body signature is reserved to converter carriers, so any parsed
				// row carrying it under a header other than the reserved name is a
				// carrier wearing a semantic identity — refused regardless of which
				// parser lane classified it, present or future family alike.
				if observation.Parsed && observation.EventName != "" && observation.EventName != tracequery.SourceRawVisibilityEventName &&
					traceDBRowBodyCarriesCarrierSignature(observation.Text, observation.EventName) {
					refuseEvent(TraceEventInvalidCarrierSignatureUnderForeignHeader, observation.Line,
						observation.EventName, observation.EventType, observation.Text)
				}
				if observation.Parsed && profile.Kind == ownedTraceValidationBuiltin &&
					ownedBuiltinAdvisoryEvent(observation.EventName, observation.EventType) {
					advisoryDigest.add(observation.Line, observation.Text)
				}
				if !observation.Parsed {
					unparsedDigest.add(observation.Line, observation.Text)
				}
			},
		)
		if err != nil && operationReason == "" {
			operationReason = traceDBPostvalidationScanFailed
		}
		return err
	})
	postScanGenerationErr := source.Validate()
	if operationErr != nil {
		if postScanGenerationErr != nil {
			coverage.Found = false
			return receipt, coverage, fail(traceDBPostvalidationGenerationInvalid, operationErr, postScanGenerationErr)
		}
		if operationReason == traceDBPostvalidationCanceled || errors.Is(operationErr, context.Canceled) || errors.Is(operationErr, context.DeadlineExceeded) {
			coverage.Error = traceDBPostvalidationCanceled
			return receipt, coverage, operationErr
		}
		if operationReason == "" {
			operationReason = traceDBPostvalidationScanFailed
		}
		return receipt, coverage, fail(operationReason, operationErr)
	}
	if postScanGenerationErr != nil {
		coverage.Found = false
		return receipt, coverage, fail(traceDBPostvalidationGenerationInvalid, postScanGenerationErr)
	}
	if err := ctx.Err(); err != nil {
		coverage.Error = traceDBPostvalidationCanceled
		return receipt, coverage, err
	}
	if idx == nil {
		return receipt, coverage, fail(traceDBPostvalidationScanFailed)
	}
	if parsedHeaderRow {
		return receipt, coverage, fail(traceDBPostvalidationHeaderInvalid)
	}
	observedAdvisory := advisoryDigest.finish()
	observedUnknown := unknownDigest.finish()
	observedUnparsed := unparsedDigest.finish()
	observedWire := wireHasher.finish()

	coverage.RowsRead = idx.ScannedLineCount
	coverage.RowsEmitted = callbackCount
	coverage.ColumnsPresent = append(coverage.ColumnsPresent,
		fmt.Sprintf("header_lines=%d", headerLines),
		fmt.Sprintf("profile=%s", profile.Kind),
		fmt.Sprintf("expected_rows=%d", profile.ExpectedRows),
		fmt.Sprintf("expected_known=%d", profile.ExpectedKnown),
		fmt.Sprintf("expected_typed_preserved=%d", profile.ExpectedTypedPreserved),
		fmt.Sprintf("expected_source_raw_visibility=%d", profile.ExpectedSourceRawVisibility),
		fmt.Sprintf("expected_advisory=%d", profile.ExpectedAdvisory.Rows),
		fmt.Sprintf("expected_unknown=%d", profile.ExpectedUnknown.Rows),
		fmt.Sprintf("expected_unparsed=%d", profile.ExpectedUnparsed.Rows),
		fmt.Sprintf("parsed_known=%d", idx.ParsedKnown),
		fmt.Sprintf("parsed_typed_preserved=%d", typedPreservedCount),
		fmt.Sprintf("parsed_source_raw_visibility=%d", sourceRawVisibilityCount),
		fmt.Sprintf("callback_count=%d", callbackCount),
	)
	if idx.FirstTs != 0 || idx.LastTs != 0 {
		coverage.ColumnsPresent = append(coverage.ColumnsPresent,
			fmt.Sprintf("first_ts=%.6f", idx.FirstTs),
			fmt.Sprintf("last_ts=%.6f", idx.LastTs),
		)
	}
	if idx.TraceFlavor != "" {
		coverage.ColumnsPresent = append(coverage.ColumnsPresent, "trace_flavor="+string(idx.TraceFlavor))
	}

	if idx.ParseLinePanics != 0 {
		return receipt, coverage, fail(traceDBPostvalidationParsePanic)
	}
	if idx.ClockRegressions != 0 {
		if firstClockRegression != nil {
			coverage.ColumnsPresent = append(coverage.ColumnsPresent,
				fmt.Sprintf("clock_regression_previous_line=%d", firstClockRegression.PreviousLine),
				fmt.Sprintf("clock_regression_current_line=%d", firstClockRegression.CurrentLine),
				fmt.Sprintf("clock_regression_previous_timestamp_sec=%.9f", firstClockRegression.PreviousTimestampSec),
				fmt.Sprintf("clock_regression_current_timestamp_sec=%.9f", firstClockRegression.CurrentTimestampSec),
				"clock_regression_previous_event_type="+string(firstClockRegression.PreviousEventType),
				"clock_regression_current_event_type="+string(firstClockRegression.CurrentEventType),
			)
			return receipt, coverage, fail(traceDBPostvalidationClockRegression, firstClockRegression)
		}
		return receipt, coverage, fail(traceDBPostvalidationClockRegression)
	}
	if wireHashFailed || !observedWire.Valid || observedWire.Bytes != source.Size() ||
		profile.ExpectedWire.Valid && !ownedTraceWireDigestEqual(profile.ExpectedWire, observedWire) {
		return receipt, coverage, fail(traceDBPostvalidationWireMismatch)
	}
	if !ownedTraceRowDigestEqual(profile.ExpectedUnparsed, observedUnparsed) ||
		idx.UnparsedLines != headerLines+profile.ExpectedUnparsed.Rows {
		return receipt, coverage, fail(traceDBPostvalidationUnparsedOwnedRow)
	}
	if !ownedTraceRowDigestEqual(profile.ExpectedUnknown, observedUnknown) || unknownCount != profile.ExpectedUnknown.Rows {
		return receipt, coverage, fail(traceDBPostvalidationUnknownOwnedRow)
	}
	if !ownedTraceRowDigestEqual(profile.ExpectedAdvisory, observedAdvisory) {
		return receipt, coverage, fail(traceDBPostvalidationUnknownOwnedRow)
	}
	if eventTypeMismatch {
		return receipt, coverage, fail(traceDBPostvalidationEventTypeMismatch)
	}
	if !typedSequence.complete(profile.ExpectedTypedPreserved) {
		refuseEvent(TraceEventInvalidTraceDBRecordSequenceIncomplete, 0, "", "", "")
	}
	if eventInvalid {
		if firstEventInvalid != nil {
			// Row identification rides the same per-row detail lane as the
			// clock-regression witness: typed Cause + coverage columns, so the
			// diagnostic report names the offending producer row.
			coverage.ColumnsPresent = append(coverage.ColumnsPresent,
				"event_invalid_kind="+string(firstEventInvalid.Kind),
				fmt.Sprintf("event_invalid_line=%d", firstEventInvalid.Line),
				"event_invalid_event_name="+firstEventInvalid.EventName,
				"event_invalid_event_type="+string(firstEventInvalid.EventType),
				fmt.Sprintf("event_invalid_body_prefix=%q", firstEventInvalid.BodyPrefix),
			)
			return receipt, coverage, fail(traceDBPostvalidationEventInvalid, firstEventInvalid)
		}
		return receipt, coverage, fail(traceDBPostvalidationEventInvalid)
	}
	if callbackOverflow || profile.ExpectedRows > math.MaxInt-headerLines {
		return receipt, coverage, fail(traceDBPostvalidationCountMismatch)
	}
	expectedLines := headerLines + profile.ExpectedRows
	if idx.Size != source.Size() || len(idx.Events) != 0 || idx.LineCount != expectedLines ||
		idx.ScannedLineCount != expectedLines || idx.ParsedKnown != profile.ExpectedKnown || knownCount != profile.ExpectedKnown ||
		typedPreservedCount != profile.ExpectedTypedPreserved ||
		sourceRawVisibilityCount != profile.ExpectedSourceRawVisibility ||
		idx.TraceDBTextCarrierRows != profile.ExpectedTypedPreserved ||
		callbackCount != profile.ExpectedKnown+profile.ExpectedUnknown.Rows {
		return receipt, coverage, fail(traceDBPostvalidationCountMismatch)
	}
	if profile.Kind != ownedTraceValidationPerf {
		// ArtifactPath is itself a receipt discriminator. Failed diagnostics
		// deliberately remain pathless so they cannot enter receipt selectors.
		coverage.ArtifactPath = displayPath
	}
	authoritativeKnown := profile.ExpectedKnown
	if profile.Kind == ownedTraceValidationBuiltin {
		authoritativeKnown -= profile.ExpectedAdvisory.Rows - profile.ExpectedUnknown.Rows
	} else if profile.Kind == ownedTraceValidationSQL {
		authoritativeKnown -= profile.ExpectedTypedPreserved + profile.ExpectedSourceRawVisibility
	}
	advisory := profile.ExpectedAdvisory.Rows
	if profile.Kind == ownedTraceValidationSQL {
		advisory = profile.ExpectedTypedPreserved + profile.ExpectedSourceRawVisibility
	}
	receipt = ownedTraceValidationReceipt{
		kind: profile.Kind, perfProfile: profile.PerfProfile, perfSource: profile.RequiredPerfSource, perfClock: profile.RequiredPerfClock,
		sourceIdentity: source.identity, size: source.Size(), rows: profile.ExpectedRows,
		known: profile.ExpectedKnown, authoritativeKnown: authoritativeKnown, advisory: advisory,
		unknown: profile.ExpectedUnknown.Rows, unparsed: profile.ExpectedUnparsed.Rows,
		queryReady:             authoritativeKnown > 0,
		rawCaptureCompleteness: profile.RawCaptureCompleteness, hasRawCaptureCompleteness: profile.HasRawCaptureCompleteness,
		rawCaptureResidual: profile.RawCaptureResidual, hasRawCaptureResidual: profile.HasRawCaptureResidual,
		rawSampleAdmission: profile.RawSampleAdmission, hasRawSampleAdmission: profile.HasRawSampleAdmission,
		wireSHA256: observedWire.SHA256,
	}
	return receipt, coverage, nil
}

func validateOwnedTraceValidationReceipt(receipt ownedTraceValidationReceipt) error {
	headerLines := strings.Count(systraceHeader, "\n")
	if receipt.rows < 0 || receipt.known < 0 || receipt.authoritativeKnown < 0 || receipt.advisory < 0 ||
		receipt.unknown < 0 || receipt.unparsed < 0 {
		return fmt.Errorf("owned trace validation receipt has a negative count")
	}
	expectedAuthoritativeKnown := receipt.known
	if receipt.kind == ownedTraceValidationBuiltin {
		if receipt.advisory < receipt.unknown || receipt.advisory-receipt.unknown > receipt.known {
			return fmt.Errorf("owned trace validation receipt has inconsistent builtin advisory accounting")
		}
		expectedAuthoritativeKnown -= receipt.advisory - receipt.unknown
	} else if receipt.kind == ownedTraceValidationSQL {
		if receipt.advisory > receipt.known {
			return fmt.Errorf("owned trace validation receipt has inconsistent SQL preservation accounting")
		}
		expectedAuthoritativeKnown -= receipt.advisory
	} else if receipt.advisory != 0 {
		return fmt.Errorf("owned trace validation receipt attaches builtin advisory rows to another profile")
	}
	if !receipt.kind.valid() || !receipt.sourceIdentity.Initialized() || !receipt.sourceIdentity.Strong() ||
		receipt.size <= 0 || receipt.rows < 0 || receipt.known < 0 || receipt.authoritativeKnown < 0 ||
		receipt.advisory < 0 || receipt.unknown < 0 || receipt.unparsed < 0 ||
		receipt.known > math.MaxInt-receipt.unknown || receipt.known+receipt.unknown > math.MaxInt-receipt.unparsed ||
		receipt.rows > math.MaxInt-headerLines ||
		receipt.rows != receipt.known+receipt.unknown+receipt.unparsed ||
		receipt.authoritativeKnown != expectedAuthoritativeKnown || receipt.queryReady != (receipt.authoritativeKnown > 0) ||
		strings.TrimSpace(receipt.coverage.Error) != "" || !receipt.coverage.Found ||
		receipt.coverage.RowsRead != headerLines+receipt.rows || receipt.coverage.RowsEmitted != receipt.known+receipt.unknown {
		return fmt.Errorf("owned trace validation receipt is incomplete or inconsistent")
	}
	if receipt.kind != ownedTraceValidationPerf {
		if receipt.hasRawCaptureCompleteness || receipt.rawCaptureCompleteness != (RawPerfCaptureCompleteness{}) ||
			receipt.hasRawCaptureResidual || receipt.rawCaptureResidual != (RawPerfCaptureResidual{}) ||
			receipt.hasRawSampleAdmission || receipt.rawSampleAdmission != (RawPerfSampleAdmission{}) ||
			receipt.coverage.RawCaptureCompleteness != nil || receipt.coverage.RawCaptureResidual != nil ||
			receipt.coverage.RawSampleAdmission != nil {
			return fmt.Errorf("owned systrace validation receipt carries raw perf completeness")
		}
		spec, ok := receipt.kind.systraceClaimSpec()
		if !ok || !tracebundle.IsSystraceReceiptCoverage(
			receipt.coverage.Family,
			receipt.coverage.Table,
			receipt.coverage.Role,
			receipt.coverage.ArtifactPath,
		) || receipt.coverage.Table != spec.coverageTable {
			return fmt.Errorf("owned systrace validation receipt has no exact artifact coverage binding")
		}
	} else {
		expectedTable, ok := receipt.perfProfile.coverageTable()
		if !ok || receipt.coverage.Family != tracebundle.PerfReceiptFamily ||
			receipt.coverage.Table != expectedTable || receipt.coverage.Role != tracebundle.PerfReceiptRole ||
			strings.TrimSpace(receipt.coverage.ArtifactPath) != "" {
			return fmt.Errorf("owned perf validation receipt has no exact receipt coverage profile")
		}
	}
	switch receipt.kind {
	case ownedTraceValidationSQL:
		if receipt.rows <= 0 || receipt.known != receipt.rows || receipt.advisory > receipt.known ||
			receipt.authoritativeKnown != receipt.known-receipt.advisory ||
			receipt.unknown != 0 || receipt.unparsed != 0 ||
			receipt.queryReady != (receipt.authoritativeKnown > 0) || !receipt.queryReady {
			return fmt.Errorf("owned trace validation receipt violates its SQL authority profile")
		}
		if receipt.perfProfile != "" || receipt.perfSource != "" || receipt.perfClock != "" {
			return fmt.Errorf("owned trace validation receipt attaches a perf profile to SQL output")
		}
	case ownedTraceValidationPerf:
		expectedSource, expectedClock, ok := receipt.perfProfile.sourceClock()
		if !ok || receipt.perfSource != expectedSource || receipt.perfClock != expectedClock ||
			receipt.known != receipt.rows || receipt.authoritativeKnown != receipt.rows ||
			receipt.unknown != 0 || receipt.unparsed != 0 || receipt.queryReady != (receipt.rows > 0) {
			return fmt.Errorf("owned trace validation receipt has no closed perf profile")
		}
		if receipt.perfProfile != ownedTracePerfRaw {
			if receipt.rows <= 0 || !receipt.queryReady || receipt.hasRawCaptureCompleteness ||
				receipt.rawCaptureCompleteness != (RawPerfCaptureCompleteness{}) || receipt.hasRawCaptureResidual ||
				receipt.rawCaptureResidual != (RawPerfCaptureResidual{}) || receipt.hasRawSampleAdmission ||
				receipt.rawSampleAdmission != (RawPerfSampleAdmission{}) ||
				receipt.coverage.RawCaptureCompleteness != nil || receipt.coverage.RawCaptureResidual != nil ||
				receipt.coverage.RawSampleAdmission != nil {
				return fmt.Errorf("owned nonraw perf receipt carries raw inventory semantics")
			}
			break
		}
		if !receipt.hasRawCaptureCompleteness || receipt.coverage.RawCaptureCompleteness == nil ||
			!receipt.hasRawCaptureResidual || receipt.coverage.RawCaptureResidual == nil ||
			!receipt.hasRawSampleAdmission || receipt.coverage.RawSampleAdmission == nil ||
			validateRawPerfCaptureCompleteness(receipt.rawCaptureCompleteness) != "" ||
			validateRawPerfCaptureResidual(receipt.rawCaptureResidual) != "" ||
			validateRawPerfSampleAdmission(receipt.rawSampleAdmission) != "" ||
			receipt.rawSampleAdmission.Candidates != receipt.rawCaptureCompleteness.SampleRecords.Accepted ||
			receipt.rawSampleAdmission.QueryRows > uint64(math.MaxInt) ||
			int(receipt.rawSampleAdmission.QueryRows) != receipt.rows ||
			*receipt.coverage.RawCaptureCompleteness != receipt.rawCaptureCompleteness ||
			*receipt.coverage.RawCaptureResidual != receipt.rawCaptureResidual ||
			*receipt.coverage.RawSampleAdmission != receipt.rawSampleAdmission {
			return fmt.Errorf("owned raw perf receipt has no exact capture completeness and sample admission binding")
		}
		if receipt.rows == 0 {
			hasIssue, err := rawPerfCaptureHasPublicationIssue(receipt.rawCaptureCompleteness)
			if err != nil || !hasIssue && !rawPerfSampleAdmissionHasIssue(receipt.rawSampleAdmission) {
				return fmt.Errorf("owned raw perf inventory has no deterministic publication issue")
			}
		}
	case ownedTraceValidationBuiltin:
		if receipt.perfProfile != "" || receipt.perfSource != "" || receipt.perfClock != "" {
			return fmt.Errorf("owned trace validation receipt violates its builtin inventory profile")
		}
	case ownedTraceValidationProfiler:
		if receipt.perfProfile != "" || receipt.perfSource != "" || receipt.perfClock != "" || receipt.rows <= 0 ||
			receipt.authoritativeKnown != receipt.known || receipt.unparsed != 0 {
			return fmt.Errorf("owned trace validation receipt violates its profiler provenance profile")
		}
	}
	return nil
}

func validateOwnedTraceReceiptSource(source *sealedConversionFile, receipt ownedTraceValidationReceipt) error {
	if source == nil {
		return fmt.Errorf("owned trace validation source is nil")
	}
	if err := validateOwnedTraceValidationReceipt(receipt); err != nil {
		return err
	}
	if err := source.Validate(); err != nil {
		return fmt.Errorf("validate owned trace receipt source: %w", err)
	}
	if source.Size() != receipt.size || !receipt.sourceIdentity.SameVersion(source.identity) {
		return fmt.Errorf("owned trace validation receipt does not bind the publication source generation")
	}
	return nil
}

func validatePublishedOwnedTraceReceipt(ctx context.Context, publication *retainedTraceDBPublication, receipt ownedTraceValidationReceipt) error {
	if publication == nil {
		return fmt.Errorf("owned trace published validation authority is nil")
	}
	if err := validateOwnedTraceValidationReceipt(receipt); err != nil {
		return err
	}
	publication.mu.Lock()
	defer publication.mu.Unlock()
	if publication.closed || publication.removed || publication.file == nil ||
		!publication.identity.Initialized() || publication.size != receipt.size {
		return fmt.Errorf("owned trace published validation authority is incomplete")
	}
	bytesRead, measuredSHA, measuredIdentity, err := tracebundle.MeasureFile(ctx, publication.file)
	if err != nil {
		return fmt.Errorf("measure owned trace published generation: %w", err)
	}
	if bytesRead != receipt.size || !publication.identity.SameVersion(measuredIdentity) ||
		measuredSHA != hex.EncodeToString(receipt.wireSHA256[:]) {
		return fmt.Errorf("owned trace published generation differs from its held validation receipt")
	}
	return nil
}

// publishValidatedOwnedTraceOutputNoReplace is the only publication throat
// which can carry a held-file semantic receipt into the transaction ledger.
// It rejects validation-A/publication-B substitutions before the platform
// snapshot and binds the successful receipt to the exact public generation.
func publishValidatedOwnedTraceOutputNoReplace(
	ctx context.Context,
	target sealedConversionPublicationTarget,
	source *sealedConversionFile,
	receipt ownedTraceValidationReceipt,
	ledger *conversionFileLedger,
) error {
	if ledger == nil {
		return fmt.Errorf("owned trace validation publication ledger is required")
	}
	bindingPath := strings.TrimSpace(target.finalBindingPath)
	artifactPath := target.FinalPath
	if bindingPath == "" || artifactPath == "" || artifactPath != strings.TrimSpace(artifactPath) {
		return fmt.Errorf("owned trace validation publication binding is incomplete")
	}
	if err := validateOwnedTraceReceiptSource(source, receipt); err != nil {
		return err
	}
	if err := publishSealedConversionFileNoReplaceWithValidation(
		ctx,
		target,
		source,
		ledger,
		func(publication *retainedTraceDBPublication) error {
			return validatePublishedOwnedTraceReceipt(ctx, publication, receipt)
		},
	); err != nil {
		return err
	}
	if err := ledger.recordOwnedTraceValidation(bindingPath, artifactPath, receipt); err != nil {
		return traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(bindingPath))
	}
	return nil
}

// traceDBRowBody returns the body of an ftrace row (the text after
// "<eventName>: ") and whether the header marker was found.
func traceDBRowBody(text, eventName string) (string, bool) {
	marker := " " + eventName + ": "
	idx := strings.Index(text, marker)
	if idx < 0 {
		return "", false
	}
	return text[idx+len(marker):], true
}

// traceDBRowBodyCarriesCarrierSignature reports whether the body of an ftrace
// row starts with a codrax carrier wire token. The grammar is the parser
// package's single source (tracequery.CarrierWireTokenGrammar), the same one
// the producer registry and the structural literal census derive from.
func traceDBRowBodyCarriesCarrierSignature(text, eventName string) bool {
	body, ok := traceDBRowBody(text, eventName)
	if !ok {
		return false
	}
	_, ok = tracequery.CarrierWireToken(body)
	return ok
}

// traceEventInvalidWitnessRowBody returns the producer-written body of a
// refused row for the customer witness: an ftrace row yields the text after
// "<eventName>: "; a comment carrier (`# codrax_<family>/v<N> …`, the form the
// typed trace_db record/block rows take, whose parser name never appears as
// an ftrace marker) yields the bytes after its `# <wire> ` prefix. Every
// witness kind therefore shows producer bytes (§40.43 F-carrier-2 G).
func traceEventInvalidWitnessRowBody(text, eventName string) (string, bool) {
	if strings.HasPrefix(text, tracequery.CarrierCommentLinePrefix) {
		rest := text[len(tracequery.CarrierCommentLinePrefix):]
		if wire, ok := tracequery.CarrierWireToken(rest); ok {
			return strings.TrimPrefix(rest[len(wire):], " "), true
		}
		return rest, true
	}
	return traceDBRowBody(text, eventName)
}

// maxTraceEventInvalidWitnessBodyBytes bounds the row-body excerpt carried by
// TraceEventInvalidWitnessError: at most 64 BYTES OF BODY are taken (on a
// rune boundary) BEFORE escaping — enough to show which producer wrote the
// refused row, never a whole payload. Escaping then rewrites every byte that
// is not valid UTF-8 as a four-byte `\xNN` sequence, so the escaped excerpt
// is at most 4× that budget: maxTraceEventInvalidWitnessExcerptBytes (256
// bytes, reached by a body of 64 invalid bytes). The precise bound is the
// pair (64 B of body, ≤ 256 B escaped), not "64 B" alone (fold-in round
// four, finding P — documentation and pin only, no behaviour change).
const (
	maxTraceEventInvalidWitnessBodyBytes    = 64
	maxTraceEventInvalidWitnessExcerptBytes = 4 * maxTraceEventInvalidWitnessBodyBytes
)

// traceEventInvalidWitnessBodyPrefix returns the bounded, valid-UTF-8 excerpt
// of a refused row's body (traceEventInvalidWitnessExcerpt), or "" when the
// row has no recoverable body.
func traceEventInvalidWitnessBodyPrefix(text, eventName string) string {
	body, ok := traceEventInvalidWitnessRowBody(text, eventName)
	if !ok {
		return ""
	}
	return traceEventInvalidWitnessExcerpt(body)
}

// traceEventInvalidWitnessExcerpt bounds a row body to the witness budget on
// a rune boundary through the shared single source (types.CutPrefixRuneSafe,
// the same cut tracequery uses) and carries bytes that are not valid UTF-8
// escaped strconv.Quote-style (`\xNN`). A non-empty body therefore always
// yields a non-empty, valid-UTF-8 excerpt that still shows the producer's
// bytes — an invalid first byte or a stray byte inside the budget no longer
// collapses the witness (§40.43 F-carrier-2 H). Bound: 64 bytes of body are
// cut before escaping; the escaped result is at most
// maxTraceEventInvalidWitnessExcerptBytes (4× — every invalid byte becomes
// four bytes).
func traceEventInvalidWitnessExcerpt(body string) string {
	cut := types.CutPrefixRuneSafe(body, maxTraceEventInvalidWitnessBodyBytes)
	if cut == "" && body != "" {
		// The shared cut backs off over continuation bytes; only a run of
		// them longer than the budget reaches the start. None of those bytes
		// begins a rune, so the raw budget escapes without splitting one.
		cut = body[:min(len(body), maxTraceEventInvalidWitnessBodyBytes)]
	}
	if utf8.ValidString(cut) {
		return cut
	}
	var excerpt strings.Builder
	excerpt.Grow(len(cut) + 3*maxTraceEventInvalidWitnessBodyBytes)
	for index := 0; index < len(cut); {
		r, size := utf8.DecodeRuneInString(cut[index:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&excerpt, `\x%02x`, cut[index])
		} else {
			excerpt.WriteString(cut[index : index+size])
		}
		index += size
	}
	return excerpt.String()
}
