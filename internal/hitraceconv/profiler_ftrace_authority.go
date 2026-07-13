package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type profilerFtracePayloadDisposition uint8

const (
	profilerFtracePayloadNotStructured profilerFtracePayloadDisposition = iota
	profilerFtracePayloadStructured
	profilerFtracePayloadMalformed
)

type profilerPairAdmission struct {
	Kind      pairRenderKind
	Governed  bool
	Admitted  bool
	LaneKnown bool
	Lane      string
	// HeaderOwnerKnown records the precise physical emitter proof separately
	// from the semantic key; idle TID 0 is known, malformed/absent is not.
	HeaderOwnerKnown bool
	Verdict          tracequery.PairingEndpointVerdict
}

func (a profilerPairAdmission) poison(sink *traceDBRowSink) {
	if !a.Governed || sink == nil {
		return
	}
	if (a.Kind == pairRenderF2FS || a.Kind == pairRenderBlock) && a.LaneKnown {
		sink.poisonPairLane(a.Kind, a.Lane)
		return
	}
	sink.poisonPairKind(a.Kind)
}

const profilerTracePluginResultMaxField = 8

// profilerTracePluginResult is the single top-level TracePluginResult
// authority shared by metadata summarization and row rendering. The first
// pass retains one bounded view of the physical payload plus fixed-width
// census/provenance only. Repeated values are consumed by the checked second
// pass visitor below; keeping per-occurrence slice headers here would let a
// legal short-record frame amplify before the bounded row sink is reached.
type profilerTracePluginResult struct {
	Disposition        profilerFtracePayloadDisposition
	PairFamilies       pairCriticalFormatFamilyMask
	PairCaptureOpaque  bool
	payload            []byte
	KnownOccurrences   [profilerTracePluginResultMaxField + 1]uint64
	PayloadOccurrences [profilerTracePluginResultMaxField + 1]uint64
	VersionOccurrences uint64
	Issues             profilerTracePluginIssueCensus
	IssueOverflow      bool
}

func decodeProfilerTracePluginResult(data []byte) profilerTracePluginResult {
	result, err := decodeProfilerTracePluginResultContext(context.Background(), data)
	if err != nil {
		// A background context cannot be cancelled. Keep the compatibility
		// wrapper fail-closed if that invariant is ever violated.
		return profilerTracePluginResult{
			Disposition:       profilerFtracePayloadMalformed,
			PairCaptureOpaque: true,
			IssueOverflow:     true,
		}
	}
	return result
}

func decodeProfilerTracePluginResultContext(ctx context.Context, payload []byte) (profilerTracePluginResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var out profilerTracePluginResult
	data := payload
	recognized := false
	malformed := false
	checkpoint := uint64(0)
	for len(data) > 0 {
		if err := profilerProtoContextCheckpoint(ctx, &checkpoint); err != nil {
			return profilerTracePluginResult{}, err
		}
		key, n, ok := consumeProtoVarint(data)
		if !ok {
			malformed = true
			break
		}
		data = data[n:]
		fieldNumber := key >> 3
		if fieldNumber < 1 || fieldNumber > (1<<29)-1 {
			malformed = true
			break
		}
		field := int(fieldNumber)
		wire := int(key & 0x7)
		known := field == 1 || field == 2 || field == 5 || field == 6 || field == 7 || field == 8
		if known {
			// Attribute a truncated or unsupported-wire field to the official
			// envelope as soon as its complete key identifies a known member.
			recognized = true
			if out.KnownOccurrences[field] == math.MaxUint64 {
				return profilerTracePluginResult{}, &traceDBOutputInvariantError{Reason: "profiler_trace_plugin_known_field_census_overflow"}
			}
			out.KnownOccurrences[field]++
			if field == 7 {
				if out.VersionOccurrences == math.MaxUint64 {
					return profilerTracePluginResult{}, &traceDBOutputInvariantError{Reason: "profiler_trace_plugin_version_census_overflow"}
				}
				out.VersionOccurrences++
			}
		}

		var raw []byte
		switch wire {
		case 0:
			_, consumed, valid := consumeProtoVarint(data)
			if !valid {
				malformed = true
			} else {
				data = data[consumed:]
			}
		case 1:
			if len(data) < 8 {
				malformed = true
			} else {
				raw = data[:8]
				data = data[8:]
			}
		case 2:
			length, consumed, valid := consumeProtoVarint(data)
			if !valid {
				malformed = true
			} else if length > uint64(len(data[consumed:])) {
				if field == 2 {
					families, familyErr := profilerPairFamiliesFromCPUDetailContext(ctx, data[consumed:])
					if familyErr != nil {
						return profilerTracePluginResult{}, familyErr
					}
					out.PairFamilies |= families
				}
				malformed = true
			} else {
				raw = data[consumed : consumed+int(length)]
				data = data[consumed+int(length):]
			}
		case 5:
			if len(data) < 4 {
				malformed = true
			} else {
				raw = data[:4]
				data = data[4:]
			}
		default:
			malformed = true
		}
		if malformed {
			break
		}
		if !known {
			continue
		}

		if wire != 2 {
			if kind, ok := profilerTracePluginWrongWireIssue(field); !ok || !out.Issues.observe(kind, 1) {
				out.IssueOverflow = true
			}
			continue
		}
		if field == 2 {
			families, familyErr := profilerPairFamiliesFromCPUDetailContext(ctx, raw)
			if familyErr != nil {
				return profilerTracePluginResult{}, familyErr
			}
			out.PairFamilies |= families
		}
		if out.PayloadOccurrences[field] == math.MaxUint64 {
			return profilerTracePluginResult{}, &traceDBOutputInvariantError{Reason: "profiler_trace_plugin_field_census_overflow"}
		}
		out.PayloadOccurrences[field]++
	}
	if malformed {
		if !recognized {
			return profilerTracePluginResult{Disposition: profilerFtracePayloadNotStructured}, nil
		}
		out.Disposition = profilerFtracePayloadMalformed
		out.PairCaptureOpaque = true
		out.payload = nil
		out.KnownOccurrences = [profilerTracePluginResultMaxField + 1]uint64{}
		out.PayloadOccurrences = [profilerTracePluginResultMaxField + 1]uint64{}
		out.VersionOccurrences = 0
		if !out.Issues.observe(profilerTracePluginIssueMalformedWire, 1) {
			out.IssueOverflow = true
		}
		return out, nil
	}
	if !recognized {
		out.Disposition = profilerFtracePayloadNotStructured
		return out, nil
	}
	if out.VersionOccurrences > 1 {
		if !out.Issues.observeVersionDuplicate(out.VersionOccurrences - 1) {
			out.IssueOverflow = true
		}
	}
	out.Disposition = profilerFtracePayloadStructured
	out.payload = payload
	return out, nil
}

type profilerTracePluginResultVisitor func(field int, raw []byte) error

// visitProfilerTracePluginResult is the only repeated top-level value lane.
// It runs only after the authority pass reached EOF and verifies that the
// immutable payload still matches the first pass census.
func visitProfilerTracePluginResult(ctx context.Context, result profilerTracePluginResult, visit profilerTracePluginResultVisitor) error {
	if result.Disposition != profilerFtracePayloadStructured {
		return nil
	}
	if result.VersionOccurrences != result.KnownOccurrences[7] {
		return &traceDBOutputInvariantError{Reason: "profiler_trace_plugin_version_census_drift"}
	}
	for _, field := range [...]int{1, 2, 5, 6, 7, 8} {
		if result.PayloadOccurrences[field] > result.KnownOccurrences[field] {
			return &traceDBOutputInvariantError{Reason: "profiler_trace_plugin_payload_census_invalid"}
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var knownObserved [profilerTracePluginResultMaxField + 1]uint64
	var payloadObserved [profilerTracePluginResultMaxField + 1]uint64
	var callbackErr error
	checkpoint := uint64(0)
	err := walkProtoFields(result.payload, func(field int, wire int, raw []byte, _ uint64) error {
		if err := profilerProtoContextCheckpoint(ctx, &checkpoint); err != nil {
			return err
		}
		if field < 1 || field > profilerTracePluginResultMaxField ||
			(field != 1 && field != 2 && field != 5 && field != 6 && field != 7 && field != 8) {
			return nil
		}
		if knownObserved[field] == math.MaxUint64 {
			return &traceDBOutputInvariantError{Reason: "profiler_trace_plugin_visitor_census_overflow"}
		}
		knownObserved[field]++
		if wire != 2 {
			return nil
		}
		if payloadObserved[field] == math.MaxUint64 {
			return &traceDBOutputInvariantError{Reason: "profiler_trace_plugin_visitor_payload_census_overflow"}
		}
		payloadObserved[field]++
		if field == 7 && result.VersionOccurrences != 1 {
			return nil
		}
		if visit != nil {
			callbackErr = visit(field, raw)
			return callbackErr
		}
		return nil
	})
	if err != nil {
		if callbackErr != nil {
			return callbackErr
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return &traceDBOutputInvariantError{Reason: "profiler_trace_plugin_visitor_parse_drift"}
	}
	if knownObserved != result.KnownOccurrences || payloadObserved != result.PayloadOccurrences {
		return &traceDBOutputInvariantError{Reason: "profiler_trace_plugin_visitor_census_drift"}
	}
	return nil
}

func profilerProtoContextCheckpoint(ctx context.Context, checkpoint *uint64) error {
	if ctx == nil {
		return nil
	}
	if checkpoint == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_proto_checkpoint_nil"}
	}
	(*checkpoint)++
	// Poll the first occurrence and then every 256 occurrences. This bounds
	// cancellation latency without adding a channel select to every tiny field.
	if *checkpoint == 1 || *checkpoint&255 == 0 {
		return ctx.Err()
	}
	return nil
}

func walkProfilerProtoFieldsContext(ctx context.Context, data []byte, visit func(field int, wire int, raw []byte, value uint64) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	checkpoint := uint64(0)
	err := walkProtoFields(data, func(field int, wire int, raw []byte, value uint64) error {
		if err := profilerProtoContextCheckpoint(ctx, &checkpoint); err != nil {
			return err
		}
		if visit != nil {
			return visit(field, wire, raw, value)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return ctx.Err()
}

func visitProfilerTracePluginResultEventsContext(ctx context.Context, result profilerTracePluginResult, visit func(profilerFtraceEventRecord) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if result.Disposition == profilerFtracePayloadMalformed {
		return visitProfilerTracePluginMalformedResult(result, visit)
	}
	return visitProfilerTracePluginResult(ctx, result, func(field int, raw []byte) error {
		if field != 2 {
			return nil
		}
		authority, err := auditProfilerFtraceCPUDetail(ctx, raw)
		if err != nil {
			return err
		}
		return visitProfilerFtraceCPUDetailEvents(ctx, authority, visit)
	})
}

func visitProfilerTracePluginMalformedResult(result profilerTracePluginResult, visit func(profilerFtraceEventRecord) error) error {
	if result.PairFamilies == 0 && !result.PairCaptureOpaque {
		return nil
	}
	record := profilerFtraceEventRecord{
		Field: profilerFtraceCPUDetailEnvelopeField, PairFamilies: result.PairFamilies,
		PairCaptureOpaque: result.PairCaptureOpaque,
	}
	if err := record.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeTracePluginMalformedWire); err != nil {
		return err
	}
	if visit != nil {
		return visit(record)
	}
	return nil
}

func profilerTracePluginResultCoverage(result profilerTracePluginResult) []TraceDBCoverage {
	if result.Issues.empty() {
		return nil
	}
	total, ok := result.Issues.totalOccurrences()
	if !ok {
		return nil
	}
	rowsRead, ok := profilerContainerCountToInt(total)
	if !ok {
		return nil
	}
	fields := map[string]string{
		"schema_profile": "TracePluginResult repeated fields 1/2/5/6/8 and singular version field 7",
	}
	result.Issues.appendFieldSources(fields)
	return []TraceDBCoverage{{
		Family:       "builtin_modern_ftrace:trace_plugin_envelope",
		Table:        "__trace_plugin_envelope__",
		Role:         "unsupported_input",
		Found:        true,
		RowsRead:     rowsRead,
		Skipped:      result.Issues.summary(),
		FieldSources: fields,
	}}
}

// addStrictSystraceRowsFromBytes is the whole-payload physical-text
// compatibility lane for an exact ftrace-plugin. Text origin may outrank an
// overlapping protobuf grammar; source election is separate from exact pair
// admission. All rows are staged first, so one malformed fragment prevents
// partial publication while an origin-proven exact endpoint retains capture
// provenance.
func addStrictSystraceRowsFromBytes(data []byte, seq *int, sink *traceDBRowSink) (int, bool, error) {
	if len(data) == 0 {
		return 0, false, nil
	}
	return addProfilerStrictSystraceStage(stageProfilerStrictSystracePayload(data), seq, sink)
}

type profilerStrictSystracePayloadStage struct {
	scan          profilerStrictSystracePayloadScan
	rows          []renderedRow
	rejectedPairs []profilerPairAdmission
}

func stageProfilerStrictSystracePayload(data []byte) profilerStrictSystracePayloadStage {
	stage, _ := stageProfilerStrictSystracePayloadContext(context.Background(), data)
	return stage
}

func stageProfilerStrictSystracePayloadContext(ctx context.Context, data []byte) (profilerStrictSystracePayloadStage, error) {
	var stage profilerStrictSystracePayloadStage
	scan, err := scanProfilerStrictSystracePayloadContext(ctx, data, func(row renderedRow, pair profilerPairAdmission) {
		if pair.Governed && !pair.Admitted {
			stage.rejectedPairs = append(stage.rejectedPairs, pair)
		}
		stage.rows = append(stage.rows, row)
	})
	if err != nil {
		return profilerStrictSystracePayloadStage{}, err
	}
	stage.scan = scan
	return stage, nil
}

func addProfilerStrictSystraceStage(stage profilerStrictSystracePayloadStage, seq *int, sink *traceDBRowSink) (int, bool, error) {
	if stage.scan.rejected {
		if sink == nil {
			return 0, false, fmt.Errorf("strict systrace row sink is nil")
		}
		for _, kind := range profilerCaptureKinds {
			if stage.scan.observed[kind] && (kind != pairRenderBlock || stage.scan.originText) {
				sink.poisonPairKind(kind)
			}
		}
		// This is the exact ftrace-plugin compatibility lane. Once the whole
		// payload fails complete text classification, unread bytes may instead
		// be a malformed structured envelope. Record delayed opacity even when
		// no exact text header was recoverable; it suppresses nothing unless the
		// same physical source actually contains MMC or F2FS endpoints.
		sink.markPairCaptureOpaque(pairRenderMMC)
		sink.markPairCaptureOpaque(pairRenderF2FS)
		if stage.scan.originText && stage.scan.observed[pairRenderBlock] {
			sink.markPairCaptureOpaque(pairRenderBlock)
		}
		return 0, false, nil
	}
	if len(stage.rows) == 0 {
		return 0, false, nil
	}
	if sink == nil || seq == nil {
		return 0, false, fmt.Errorf("strict systrace row sink or sequence is nil")
	}
	for _, pair := range stage.rejectedPairs {
		pair.poison(sink)
	}
	for index := range stage.rows {
		stage.rows[index].seq = *seq
		if err := sink.add(stage.rows[index]); err != nil {
			return index, true, err
		}
		(*seq)++
	}
	return len(stage.rows), true, nil
}

type profilerStrictSystracePayloadScan struct {
	observed      [pairRenderKindCount]bool
	rejected      bool
	originDecided bool
	originText    bool
}

// scanProfilerStrictSystracePayload is the shared, non-publishing authority
// for strict legacy text classification. The first non-empty, non-comment
// physical header elects origin once; comments are only a preamble and later
// headers cannot rescue anonymous metadata. Exact endpoint admission remains a
// separate authority. Callers may stage candidate rows, but may publish none
// when any physical fragment fails the complete-text gate.
func scanProfilerStrictSystracePayload(data []byte, visit func(renderedRow, profilerPairAdmission)) profilerStrictSystracePayloadScan {
	scan, _ := scanProfilerStrictSystracePayloadContext(context.Background(), data, visit)
	return scan
}

func scanProfilerStrictSystracePayloadContext(ctx context.Context, data []byte,
	visit func(renderedRow, profilerPairAdmission),
) (profilerStrictSystracePayloadScan, error) {
	var scan profilerStrictSystracePayloadScan
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return scan, err
	}
	processed := uint64(0)
	pending := uint64(0)
	for start := 0; start < len(data); {
		end := start
		for end < len(data) && data[end] != '\n' {
			end++
			pending++
			if pending == profilerContextByteCheckpointBytes {
				if err := profilerByteContextCheckpoint(ctx, &processed, pending); err != nil {
					return profilerStrictSystracePayloadScan{}, err
				}
				pending = 0
			}
		}
		part := data[start:end]
		if len(part) > 0 && part[len(part)-1] == '\r' {
			part = part[:len(part)-1]
		}
		if len(part) > 0 && (part[0] == ' ' || part[len(part)-1] == ' ') {
			var trimErr error
			part, trimErr = profilerTrimASCIISpacesBytesContext(ctx, part)
			if trimErr != nil {
				return profilerStrictSystracePayloadScan{}, trimErr
			}
		}
		if end < len(data) {
			start = end + 1
			pending++
			if pending == profilerContextByteCheckpointBytes {
				if err := profilerByteContextCheckpoint(ctx, &processed, pending); err != nil {
					return profilerStrictSystracePayloadScan{}, err
				}
				pending = 0
			}
		} else {
			start = len(data)
		}
		if len(part) == 0 {
			continue
		}
		// A leading '#' owns the compatibility comment namespace. It can
		// never be an ftrace task prefix or pair endpoint, even when the bytes
		// after it happen to form a complete physical Block header. Valid
		// comments are preamble; invalid/oversized comments still reject the
		// strict payload, but must do so before any endpoint census can mint
		// provenance.
		commentPrefix := part[0] == '#'
		if part[0] == '\t' {
			var commentErr error
			commentPrefix, commentErr = profilerTextCommentPrefixContext(ctx, part)
			if commentErr != nil {
				return profilerStrictSystracePayloadScan{}, commentErr
			}
		}
		if commentPrefix {
			physical, err := profilerPhysicalRunesSafeBytesContext(ctx, part)
			if err != nil {
				return profilerStrictSystracePayloadScan{}, err
			}
			if len(part) > maxProfilerTextLineBytes || !physical {
				scan.rejected = true
			}
			continue
		}
		if len(part) > maxProfilerTextLineBytes {
			kind, governed, _ := profilerTextPairCensus(part)
			physical, err := profilerPhysicalRunesSafeBytesContext(ctx, part)
			if err != nil {
				return profilerStrictSystracePayloadScan{}, err
			}
			if !scan.originDecided {
				scan.originDecided = true
				// A bounded exact governed endpoint is itself precise text
				// provenance after the comment namespace has been excluded. This
				// keeps an oversized endpoint with an unsafe suffix from becoming
				// an invisible hole, while anonymous unsafe bytes remain non-text.
				if physical {
					scan.originText, _ = profilerTextPhysicalHeaderCensus(part)
				} else {
					_, scan.originText = profilerTextLeadingPairCensus(part)
				}
			}
			if governed {
				scan.observed[kind] = true
			}
			scan.rejected = true
			continue
		}
		physicalLine, err := profilerSinglePhysicalLineBytesContext(ctx, part, false)
		if err != nil {
			return profilerStrictSystracePayloadScan{}, err
		}
		if !physicalLine {
			// A non-comment row can remain precise endpoint provenance even when
			// a trailing control/invalid rune makes the complete row unpublishable.
			// Use only the bounded exact physical-header census, then reject, so a
			// valid endpoint on either side cannot bridge this capture hole.
			kind, governed, _ := profilerTextPairCensus(part)
			if !scan.originDecided {
				scan.originDecided = true
				_, scan.originText = profilerTextLeadingPairCensus(part)
			}
			if governed {
				scan.observed[kind] = true
			}
			scan.rejected = true
			continue
		}
		line, err := profilerCloneBytesStringContext(ctx, part)
		if err != nil {
			return profilerStrictSystracePayloadScan{}, err
		}
		if !scan.originDecided {
			scan.originDecided = true
			_, physicalHeader := tracequery.ProbePhysicalFtraceHeader(line)
			scan.originText = physicalHeader
		}
		if profilerTextPairNormalizationCollision(line) {
			// A compatibility ParseLine row that normalizes a missing/shifted
			// event delimiter into an exact endpoint must not be published as an
			// ungoverned row for downstream pairing. It is a near-name inventory
			// row, so drop it locally without poisoning any pair family.
			continue
		}
		pair := profilerTextPairAdmission(line)
		if pair.Governed {
			scan.observed[pair.Kind] = true
		}
		ts, ok := strictSystraceLineTimestampNS(line)
		if !ok {
			scan.rejected = true
			continue
		}
		row := renderedRow{tsNS: ts, line: line}
		if pair.Governed {
			row.pairKind = pair.Kind
			row.pairLane = pair.Lane
		}
		if visit != nil && scan.originText {
			visit(row, pair)
		}
		if err := ctx.Err(); err != nil {
			return profilerStrictSystracePayloadScan{}, err
		}
	}
	if err := profilerByteContextCheckpoint(ctx, &processed, pending); err != nil {
		return profilerStrictSystracePayloadScan{}, err
	}
	if err := ctx.Err(); err != nil {
		return profilerStrictSystracePayloadScan{}, err
	}
	return scan, nil
}

// profilerTextCommentPrefix is a classification-only negative gate. Leading
// ASCII spaces are already trimmed by the caller; tabs are accepted here only
// so an invalid tab-indented '#...' record cannot be reinterpreted by ftrace's
// whitespace-tolerant header regex. The rune validator still rejects that
// payload, and no other control character is skipped.
func profilerTextCommentPrefix(data []byte) bool {
	found, _ := profilerTextCommentPrefixContext(context.Background(), data)
	return found
}

func profilerTextCommentPrefixContext(ctx context.Context, data []byte) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	processed := uint64(0)
	pending := uint64(0)
	for len(data) > 0 && (data[0] == ' ' || data[0] == '\t') {
		data = data[1:]
		pending++
		if pending == profilerContextByteCheckpointBytes {
			if err := profilerByteContextCheckpoint(ctx, &processed, pending); err != nil {
				return false, err
			}
			pending = 0
		}
	}
	if err := profilerByteContextCheckpoint(ctx, &processed, pending); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return len(data) > 0 && data[0] == '#', nil
}

// profilerTextPairNormalizationCollision detects only a precise parser-shape
// collision: the compatibility parser resolved an exact pair endpoint, while
// the physical header did not contain that same byte-exact `name:` token.
// No prose/prefix heuristic participates, and near rows never poison a family.
func profilerTextPairNormalizationCollision(line string) bool {
	if !strings.Contains(line, "mmc_request_") && !strings.Contains(line, "f2fs_") &&
		!strings.Contains(line, "block_rq_") && !strings.Contains(line, "block_bio_") {
		return false
	}
	event, parsed := tracequery.ParseLine(1, line, nil)
	if !parsed {
		return false
	}
	if _, governed := profilerPairKindForExactName(event.Name); !governed {
		return false
	}
	physicalName, exact := tracequery.ProbeExactEventNamePrefix(line)
	return !exact || physicalName != event.Name
}

// profilerTextPairAdmission is the sole bridge for profiler text compatibility
// publishers into the same source-wide pair barrier as structured protobuf
// rows.  Recognition is byte-exact and the body verdict comes from
// tracequery's canonical endpoint authority; prose/substrings never guess a
// family.
func profilerTextPairAdmission(line string) profilerPairAdmission {
	if !strings.Contains(line, "mmc_request_") && !strings.Contains(line, "f2fs_") &&
		!strings.Contains(line, "block_rq_") && !strings.Contains(line, "block_bio_") {
		return profilerPairAdmission{}
	}
	physicalName, exactHeader := tracequery.ProbeExactEventNamePrefix(line)
	kind, governed := profilerPairKindForExactName(physicalName)
	if !exactHeader || !governed {
		return profilerPairAdmission{}
	}
	ev, ok := tracequery.ParseLine(1, line, nil)
	if ok {
		if ev.Name != physicalName {
			return profilerPairAdmission{}
		}
		verdict := tracequery.FingerprintPairingEvent(ev)
		admitted := verdict.Recognized && verdict.KeyKnown && verdict.PayloadAdmitted &&
			verdict.EmitterKnown && verdict.EmitterAdmitted
		return profilerPairAdmission{
			Kind: kind, Governed: true, Admitted: admitted,
			LaneKnown: verdict.KeyKnown && verdict.EmitterKnown && verdict.SemanticKey != "", Lane: verdict.SemanticKey,
			HeaderOwnerKnown: verdict.EmitterKnown, Verdict: verdict,
		}
	}
	return profilerPairAdmission{Kind: kind, Governed: true}
}

func profilerPairKindForExactName(name string) (pairRenderKind, bool) {
	switch name {
	case "mmc_request_start", "mmc_request_done":
		return pairRenderMMC, true
	case "f2fs_sync_file_enter", "f2fs_sync_file_exit", "f2fs_direct_IO_enter", "f2fs_direct_IO_exit",
		"f2fs_write_begin", "f2fs_write_end":
		return pairRenderF2FS, true
	case "block_bio_queue", "block_bio_complete", "block_rq_issue", "block_rq_complete":
		return pairRenderBlock, true
	default:
		return pairRenderUnknown, false
	}
}

const profilerTextPairHeaderProbeBytes = 4096

// profilerTextPhysicalHeaderCensus proves only the physical ftrace envelope
// from a fixed prefix. It grants no endpoint identity: malformed scalar rows
// and missing/shifted event delimiters may establish text origin, while exact
// pair admission remains solely in profilerTextPairAdmission/Census.
func profilerTextPhysicalHeaderCensus(part []byte) (bool, bool) {
	probe := part
	truncated := false
	if len(probe) > profilerTextPairHeaderProbeBytes {
		probe = probe[:profilerTextPairHeaderProbeBytes]
		truncated = true
	}
	_, complete := tracequery.ProbePhysicalFtraceHeader(string(probe))
	return complete, truncated
}

// profilerTextPairCensus uses at most a bounded header prefix. A complete
// header provides exact family provenance; an unterminated oversized prefix
// is marked opaque so a later exact MMC endpoint cannot bridge it.
func profilerTextPairCensus(part []byte) (pairRenderKind, bool, bool) {
	probe := part
	truncated := false
	if len(probe) > profilerTextPairHeaderProbeBytes {
		probe = probe[:profilerTextPairHeaderProbeBytes]
		truncated = true
	}
	name, complete := tracequery.ProbeExactEventNamePrefix(string(probe))
	if complete {
		if kind, governed := profilerPairKindForExactName(name); governed {
			return kind, true, false
		}
		return pairRenderUnknown, false, false
	}
	return pairRenderUnknown, false, truncated
}

// profilerTextLeadingPairCensus is narrower than the general hole census: it
// can elect text origin for an unsafe first row only when the exact governed
// endpoint header physically begins the bounded prefix. It must never use the
// loose rightmost-header fallback across protobuf/control metadata.
func profilerTextLeadingPairCensus(part []byte) (pairRenderKind, bool) {
	probe := part
	if len(probe) > profilerTextPairHeaderProbeBytes {
		probe = probe[:profilerTextPairHeaderProbeBytes]
	}
	name, complete := tracequery.ProbeLeadingExactEventNamePrefix(string(probe))
	if !complete {
		return pairRenderUnknown, false
	}
	return profilerPairKindForExactName(name)
}

func strictSystraceLineTimestampNS(line string) (uint64, bool) {
	if _, ok := tracequery.ParseLine(1, line, nil); !ok {
		return 0, false
	}
	return tracequery.ParseLineTimestampNS(line)
}

func profilerTracePluginIssueSummary(issues []string) string {
	counts := make(map[string]int)
	for _, issue := range issues {
		if issue = strings.TrimSpace(issue); issue != "" {
			counts[issue]++
		}
	}
	return traceDBCountSummary(counts)
}
