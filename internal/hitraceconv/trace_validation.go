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

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
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
	Kind                 ownedTraceValidationKind
	PerfProfile          ownedTracePerfProfile
	CoverageTable        string
	ExpectedRows         int
	ExpectedKnown        int
	ExpectedAdvisory     ownedTraceRowDigest
	ExpectedUnknown      ownedTraceRowDigest
	ExpectedUnparsed     ownedTraceRowDigest
	ExpectedWire         ownedTraceWireDigest
	RequiredEventType    tracequery.EventType
	RequiredPerfSource   string
	RequiredPerfClock    string
	RequirePerfIntegrity bool
	AllowZeroRows        bool
}

func (profile ownedTraceValidationProfile) validate() string {
	if !profile.Kind.valid() || profile.ExpectedRows < 0 || profile.ExpectedKnown < 0 ||
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
			profile.ExpectedAdvisory.Rows != 0 || profile.ExpectedUnknown.Rows != 0 || profile.ExpectedUnparsed.Rows != 0 || profile.RequiredEventType != "" ||
			profile.RequirePerfIntegrity || profile.RequiredPerfSource != "" || profile.RequiredPerfClock != "" ||
			profile.CoverageTable != tracebundle.SystraceReceiptTableSQL {
			return traceDBPostvalidationCountMismatch
		}
	case ownedTraceValidationBuiltin:
		advisoryKnown := profile.ExpectedAdvisory.Rows - profile.ExpectedUnknown.Rows
		if profile.PerfProfile != "" || !profile.AllowZeroRows || profile.ExpectedAdvisory.Rows < profile.ExpectedUnknown.Rows ||
			advisoryKnown < 0 || advisoryKnown > profile.ExpectedKnown || profile.RequiredEventType != "" ||
			profile.RequirePerfIntegrity || profile.RequiredPerfSource != "" || profile.RequiredPerfClock != "" ||
			!profile.ExpectedWire.Valid || profile.CoverageTable != tracebundle.SystraceReceiptTableBuiltin {
			return traceDBPostvalidationCountMismatch
		}
	case ownedTraceValidationProfiler:
		if profile.PerfProfile != "" || profile.AllowZeroRows || profile.ExpectedRows <= 0 || profile.ExpectedAdvisory.Rows != 0 || profile.ExpectedUnparsed.Rows != 0 ||
			profile.RequiredEventType != "" || profile.RequirePerfIntegrity || profile.RequiredPerfSource != "" ||
			profile.RequiredPerfClock != "" || !profile.ExpectedWire.Valid ||
			profile.CoverageTable != tracebundle.SystraceReceiptTableProfiler {
			return traceDBPostvalidationCountMismatch
		}
	case ownedTraceValidationPerf:
		requiredSource, requiredClock, validPerfProfile := profile.PerfProfile.sourceClock()
		if !validPerfProfile || profile.AllowZeroRows || profile.ExpectedRows <= 0 || profile.ExpectedKnown != profile.ExpectedRows ||
			profile.ExpectedAdvisory.Rows != 0 || profile.ExpectedUnknown.Rows != 0 || profile.ExpectedUnparsed.Rows != 0 ||
			profile.RequiredEventType != tracequery.EventPerfSample || !profile.RequirePerfIntegrity ||
			profile.RequiredPerfSource != requiredSource || profile.RequiredPerfClock != requiredClock || !profile.ExpectedWire.Valid {
			return traceDBPostvalidationCountMismatch
		}
	}
	return ""
}

type ownedTraceValidationReceipt struct {
	kind               ownedTraceValidationKind
	perfProfile        ownedTracePerfProfile
	perfSource         string
	perfClock          string
	sourceIdentity     filegeneration.Identity
	size               int64
	rows               int
	known              int
	authoritativeKnown int
	advisory           int
	unknown            int
	unparsed           int
	queryReady         bool
	coverage           TraceDBCoverage
	wireSHA256         [sha256.Size]byte
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
) (receipt ownedTraceValidationReceipt, coverage TraceDBCoverage, resultErr error) {
	profile := ownedTraceValidationProfile{
		Kind:          ownedTraceValidationSQL,
		CoverageTable: traceDBPostvalidationCoverageTable,
		ExpectedRows:  expectedRows,
		ExpectedKnown: expectedRows,
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
	callbackOverflow := false
	parsedHeaderRow := false
	eventTypeMismatch := false
	eventInvalid := false
	var advisoryDigest ownedTraceRowDigestBuilder
	var unknownDigest ownedTraceRowDigestBuilder
	var unparsedDigest ownedTraceRowDigestBuilder
	wireHasher := newOwnedTraceWireHasher()
	wireHashFailed := false
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
				if profile.RequiredEventType != "" && event.Type != tracequery.EventUnknown && event.Type != profile.RequiredEventType {
					eventTypeMismatch = true
				}
				if profile.RequirePerfIntegrity && event.Type == tracequery.EventPerfSample {
					perf := event.PerfFields
					if perf == nil || perf.PerfTextIntegrity != "" || perf.PerfWeightInvalid || perf.Period <= 0 ||
						(profile.RequiredPerfSource != "" && perf.Source != profile.RequiredPerfSource) ||
						(profile.RequiredPerfClock != "" && perf.Clock != profile.RequiredPerfClock) {
						eventInvalid = true
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
				if observation.Parsed && observation.EventType == tracequery.EventUnknown {
					unknownDigest.add(observation.Line, observation.Text)
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
		fmt.Sprintf("expected_advisory=%d", profile.ExpectedAdvisory.Rows),
		fmt.Sprintf("expected_unknown=%d", profile.ExpectedUnknown.Rows),
		fmt.Sprintf("expected_unparsed=%d", profile.ExpectedUnparsed.Rows),
		fmt.Sprintf("parsed_known=%d", idx.ParsedKnown),
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
	if eventInvalid {
		return receipt, coverage, fail(traceDBPostvalidationEventInvalid)
	}
	if callbackOverflow || profile.ExpectedRows > math.MaxInt-headerLines {
		return receipt, coverage, fail(traceDBPostvalidationCountMismatch)
	}
	expectedLines := headerLines + profile.ExpectedRows
	if idx.Size != source.Size() || len(idx.Events) != 0 || idx.LineCount != expectedLines ||
		idx.ScannedLineCount != expectedLines || idx.ParsedKnown != profile.ExpectedKnown || knownCount != profile.ExpectedKnown ||
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
	}
	receipt = ownedTraceValidationReceipt{
		kind: profile.Kind, perfProfile: profile.PerfProfile, perfSource: profile.RequiredPerfSource, perfClock: profile.RequiredPerfClock,
		sourceIdentity: source.identity, size: source.Size(), rows: profile.ExpectedRows,
		known: profile.ExpectedKnown, authoritativeKnown: authoritativeKnown, advisory: profile.ExpectedAdvisory.Rows,
		unknown: profile.ExpectedUnknown.Rows, unparsed: profile.ExpectedUnparsed.Rows,
		queryReady: authoritativeKnown > 0, wireSHA256: observedWire.SHA256,
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
		spec, ok := receipt.kind.systraceClaimSpec()
		if !ok || !tracebundle.IsSystraceReceiptCoverage(
			receipt.coverage.Family,
			receipt.coverage.Table,
			receipt.coverage.Role,
			receipt.coverage.ArtifactPath,
		) || receipt.coverage.Table != spec.coverageTable {
			return fmt.Errorf("owned systrace validation receipt has no exact artifact coverage binding")
		}
	} else if strings.TrimSpace(receipt.coverage.ArtifactPath) != "" {
		return fmt.Errorf("owned perf validation receipt unexpectedly carries systrace artifact coverage")
	}
	switch receipt.kind {
	case ownedTraceValidationSQL, ownedTraceValidationPerf:
		if receipt.rows <= 0 || receipt.known != receipt.rows || receipt.authoritativeKnown != receipt.rows ||
			receipt.unknown != 0 || receipt.unparsed != 0 || !receipt.queryReady {
			return fmt.Errorf("owned trace validation receipt violates its strict-known profile")
		}
		if receipt.kind == ownedTraceValidationPerf {
			expectedSource, expectedClock, ok := receipt.perfProfile.sourceClock()
			if !ok || receipt.perfSource != expectedSource || receipt.perfClock != expectedClock {
				return fmt.Errorf("owned trace validation receipt has no closed perf profile")
			}
		} else if receipt.perfProfile != "" || receipt.perfSource != "" || receipt.perfClock != "" {
			return fmt.Errorf("owned trace validation receipt attaches a perf profile to SQL output")
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
