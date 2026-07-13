package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type profilerFtraceEventRecord struct {
	CPU                int64
	TSNS               uint64
	TGID               int64
	PID                int64
	HeaderOwnerKnown   bool
	HeaderOwnerPresent bool
	CommonFlags        int64
	CommonPreemptCount int64
	Comm               string
	Field              int
	Payload            []byte
	EnvelopeIssueCount uint8
	EnvelopeIssues     [profilerFtraceEnvelopeIssuesPerEvent]profilerFtraceEventIssue
	PairFamilies       pairCriticalFormatFamilyMask
	PairCaptureOpaque  bool
}

const profilerFtraceCPUDetailEnvelopeField = -1
const profilerFtraceEnvelopeIssuesPerEvent = 9

func (record *profilerFtraceEventRecord) appendEnvelopeIssue(kind profilerFtraceEventIssueKind) error {
	if record == nil || kind > profilerFtraceEventIssueEnvelopeIdentityIncomplete {
		return &traceDBOutputInvariantError{Reason: "profiler_event_envelope_issue_kind_invalid"}
	}
	issue, ok := profilerFtraceEventFixedIssue(record.Field, kind)
	if !ok {
		return &traceDBOutputInvariantError{Reason: "profiler_event_envelope_issue_schema_invalid"}
	}
	if int(record.EnvelopeIssueCount) > len(record.EnvelopeIssues) {
		return &traceDBOutputInvariantError{Reason: "profiler_event_envelope_issue_count_invalid"}
	}
	for index := 0; index < int(record.EnvelopeIssueCount); index++ {
		if record.EnvelopeIssues[index] == issue {
			return &traceDBOutputInvariantError{Reason: "profiler_event_envelope_issue_duplicate"}
		}
	}
	if int(record.EnvelopeIssueCount) == len(record.EnvelopeIssues) {
		return &traceDBOutputInvariantError{Reason: "profiler_event_envelope_issue_overflow"}
	}
	record.EnvelopeIssues[int(record.EnvelopeIssueCount)] = issue
	record.EnvelopeIssueCount++
	return nil
}

func (record *profilerFtraceEventRecord) checkedEnvelopeIssues() ([]profilerFtraceEventIssue, error) {
	if record == nil || int(record.EnvelopeIssueCount) > len(record.EnvelopeIssues) {
		return nil, &traceDBOutputInvariantError{Reason: "profiler_event_envelope_issue_count_invalid"}
	}
	issues := record.EnvelopeIssues[:int(record.EnvelopeIssueCount)]
	for index, issue := range issues {
		if issue.sourceClass() != profilerFtraceEventDegradationEnvelope || !issue.validFor(record.Field) {
			return nil, &traceDBOutputInvariantError{Reason: "profiler_event_envelope_issue_invalid"}
		}
		for prior := 0; prior < index; prior++ {
			if issues[prior] == issue {
				return nil, &traceDBOutputInvariantError{Reason: "profiler_event_envelope_issue_duplicate"}
			}
		}
	}
	return issues, nil
}

func renderProfilerFtraceStructuredRows(data []byte, seq *int, sink *traceDBRowSink) (int, []TraceDBCoverage, error) {
	return renderProfilerFtraceStructuredResult(decodeProfilerTracePluginResult(data), seq, sink)
}

func renderProfilerFtraceStructuredResult(result profilerTracePluginResult, seq *int, sink *traceDBRowSink) (int, []TraceDBCoverage, error) {
	return renderProfilerFtraceStructuredResultWithEnvelopeCoverageContext(context.Background(), result, seq, sink, true, nil)
}

// The TraceFile container owns one cross-frame envelope diagnostic ledger.
// Direct renderer callers retain the compatibility coverage above; the
// container path suppresses only that duplicate top-level row and leaves all
// typed event coverage and pair behavior byte-for-byte shared.
func renderProfilerFtraceStructuredResultForContainer(result profilerTracePluginResult, seq *int, sink *traceDBRowSink) (int, profilerFtraceEventBatchCensus, error) {
	return renderProfilerFtraceStructuredResultForContainerContext(context.Background(), result, seq, sink)
}

func renderProfilerFtraceStructuredResultForContainerContext(ctx context.Context, result profilerTracePluginResult, seq *int, sink *traceDBRowSink) (int, profilerFtraceEventBatchCensus, error) {
	var batch profilerFtraceEventBatchCensus
	rows, _, err := renderProfilerFtraceStructuredResultWithEnvelopeCoverageContext(ctx, result, seq, sink, false, &batch)
	return rows, batch, err
}

func renderProfilerFtraceStructuredResultForContainerFusedContext(ctx context.Context, result profilerTracePluginResult,
	seq *int, sink *traceDBRowSink,
) (int, profilerFtraceEventBatchCensus, profilerFtraceSummary, bool, error) {
	var batch profilerFtraceEventBatchCensus
	rows, _, summary, recognized, err := renderProfilerFtraceStructuredResultConsumerContext(ctx, result, seq, sink, false, &batch, true)
	return rows, batch, summary, recognized, err
}

func renderProfilerFtraceStructuredResultWithEnvelopeCoverage(result profilerTracePluginResult, seq *int, sink *traceDBRowSink, includeEnvelopeCoverage bool, batch *profilerFtraceEventBatchCensus) (int, []TraceDBCoverage, error) {
	return renderProfilerFtraceStructuredResultWithEnvelopeCoverageContext(context.Background(), result, seq, sink, includeEnvelopeCoverage, batch)
}

func renderProfilerFtraceStructuredResultWithEnvelopeCoverageContext(ctx context.Context, result profilerTracePluginResult, seq *int, sink *traceDBRowSink, includeEnvelopeCoverage bool, batch *profilerFtraceEventBatchCensus) (int, []TraceDBCoverage, error) {
	rows, coverage, _, _, err := renderProfilerFtraceStructuredResultConsumerContext(ctx, result, seq, sink, includeEnvelopeCoverage, batch, false)
	return rows, coverage, err
}

func renderProfilerFtraceStructuredResultConsumerContext(ctx context.Context, result profilerTracePluginResult, seq *int,
	sink *traceDBRowSink, includeEnvelopeCoverage bool, batch *profilerFtraceEventBatchCensus, summarize bool,
) (int, []TraceDBCoverage, profilerFtraceSummary, bool, error) {
	var topLevelCoverage []TraceDBCoverage
	if includeEnvelopeCoverage {
		topLevelCoverage = profilerTracePluginResultCoverage(result)
	}
	var coverageByField map[int]*TraceDBCoverage
	var degradationsByField map[int]map[string]int
	if batch == nil {
		coverageByField = map[int]*TraceDBCoverage{}
		degradationsByField = map[int]map[string]int{}
	}
	rows := 0
	renderEvent := func(event profilerFtraceEventRecord) error {
		if event.PairCaptureOpaque {
			sink.markPairCaptureOpaque(pairRenderMMC)
			sink.markPairCaptureOpaque(pairRenderF2FS)
			if event.PairFamilies&pairCriticalFormatFamilyBlock != 0 {
				sink.markPairCaptureOpaque(pairRenderBlock)
			}
		}
		mmcGoverned := profilerStructuredPairEventField(pairRenderMMC, event.Field)
		mmcObserved := event.PairFamilies&pairCriticalFormatFamilyMMC != 0
		f2fsGoverned := profilerStructuredPairEventField(pairRenderF2FS, event.Field)
		f2fsObserved := event.PairFamilies&pairCriticalFormatFamilyF2FS != 0
		blockPair := profilerStructuredBlockPairFamily(event.Field)
		blockGoverned := blockPair.Governed
		blockObserved := event.PairFamilies&pairCriticalFormatFamilyBlock != 0
		if mmcObserved && !mmcGoverned {
			// Multiple/wrong-wire/late-malformed oneofs can clear Field, but the
			// exact field number already seen remains precise family provenance.
			sink.poisonPairKind(pairRenderMMC)
		}
		if f2fsObserved && !f2fsGoverned {
			sink.poisonPairKind(pairRenderF2FS)
		}
		if blockObserved && !blockGoverned {
			sink.poisonPairKind(pairRenderBlock)
		}
		var coverage *TraceDBCoverage
		if batch == nil {
			coverage = profilerFtraceEventRenderCoverage(coverageByField, event.Field)
			coverage.RowsRead++
		} else if !batch.observeRead(event.Field) {
			return &traceDBOutputInvariantError{Reason: "profiler_event_batch_counter_overflow"}
		}
		var name, body string
		var ok bool
		var issues []profilerFtraceEventIssue
		var typedPair profilerPairAdmission
		var auditErr error
		name, body, ok, issues, typedPair, auditErr = renderProfilerFtraceEventBodyWithTypedAuditAndPair(event)
		if auditErr != nil {
			return auditErr
		}
		if !profilerFtraceEventIssueVerdictValid(event.Field, ok, issues) {
			return &traceDBOutputInvariantError{Reason: "profiler_event_issue_verdict_invalid"}
		}
		if batch == nil {
			labels, labelsOK := profilerFtraceEventIssueLabels(event.Field, issues)
			if !labelsOK {
				return &traceDBOutputInvariantError{Reason: "profiler_event_issue_label_invalid"}
			}
			degradations := make([]string, 0, len(labels))
			for index, label := range labels {
				// Direct unknown-event coverage historically uses its fixed table
				// description rather than a counted degradation token. Keep that
				// display shape while the typed Unmapped issue remains authoritative
				// for the container census and verdict.
				if issues[index].Kind != profilerFtraceEventIssueUnmappedField {
					degradations = append(degradations, label)
				}
			}
			if len(degradations) > 0 {
				counts := degradationsByField[event.Field]
				if counts == nil {
					counts = map[string]int{}
					degradationsByField[event.Field] = counts
				}
				for _, reason := range degradations {
					counts[reason]++
				}
			}
		} else {
			if len(issues) > 0 && !batch.observeIssues(event.Field, ok, issues) {
				return &traceDBOutputInvariantError{Reason: "profiler_event_issue_census_overflow"}
			}
		}
		if !ok {
			if mmcGoverned || mmcObserved {
				sink.poisonPairKind(pairRenderMMC)
			}
			if f2fsGoverned {
				typedPair.poison(sink)
			} else if f2fsObserved {
				sink.poisonPairKind(pairRenderF2FS)
			}
			if blockGoverned {
				typedPair.poison(sink)
			} else if blockObserved {
				sink.poisonPairKind(pairRenderBlock)
			}
			if coverage != nil && coverage.Skipped == "" {
				coverage.Skipped = "structured ftrace renderer pending"
			}
			return nil
		}
		task := firstNonEmpty(event.Comm, "unknown")
		row, err := prepareTraceDBRenderedRowWithTraceFlags(int64(event.TSNS), *seq, task, event.PID,
			event.TGID, event.CPU, event.CommonFlags, event.CommonPreemptCount, name+": "+body)
		if err != nil {
			return err
		}
		if mmcGoverned {
			row.pairKind = pairRenderMMC
			row.pairTable = name
			row.structuredPair = true
			row.profilerEventField = event.Field
			pair := profilerTextPairAdmission(row.line)
			if !pair.Governed || pair.Kind != pairRenderMMC || !pair.Admitted {
				sink.poisonPairKind(pairRenderMMC)
			}
		}
		if f2fsGoverned {
			row.pairKind = pairRenderF2FS
			row.pairLane = typedPair.Lane
			row.pairTable = name
			row.structuredPair = true
			row.profilerEventField = event.Field
			pair := profilerTextPairAdmission(row.line)
			if !typedPair.LaneKnown || !pair.Governed || pair.Kind != pairRenderF2FS ||
				!pair.Admitted || !pair.LaneKnown || pair.Lane != typedPair.Lane {
				sink.poisonPairKind(pairRenderF2FS)
			}
		}
		if blockGoverned {
			row.pairKind = pairRenderBlock
			row.pairLane = typedPair.Lane
			row.pairTable = name
			row.structuredPair = true
			row.profilerEventField = event.Field
			wireVerdict := tracequery.DecodePairingEndpoint(name, body, event.PID)
			if !typedPair.LaneKnown || !typedPair.HeaderOwnerKnown || wireVerdict != typedPair.Verdict {
				sink.poisonPairKind(pairRenderBlock)
			} else if !typedPair.Admitted {
				typedPair.poison(sink)
			}
		}
		if err := sink.add(row); err != nil {
			return err
		}
		(*seq)++
		rows++
		if coverage != nil {
			coverage.RowsEmitted++
		} else if !batch.observeEmitted(event.Field) {
			return &traceDBOutputInvariantError{Reason: "profiler_event_batch_emitted_counter_overflow"}
		}
		return nil
	}
	summary, recognized, err := consumeProfilerTracePluginResultContext(ctx, result, summarize, renderEvent)
	if err != nil {
		return rows, append(topLevelCoverage, profilerFtraceEventRenderCoverageList(coverageByField)...), summary, false, err
	}
	for field, counts := range degradationsByField {
		coverage := profilerFtraceEventRenderCoverage(coverageByField, field)
		coverage.Skipped = traceDBCountSummary(counts)
		if coverage.FieldSources == nil {
			coverage.FieldSources = map[string]string{}
		}
		for reason, count := range counts {
			coverage.FieldSources["degraded_"+reason+"_rows"] = strconv.Itoa(count)
		}
	}
	return rows, append(topLevelCoverage, profilerFtraceEventRenderCoverageList(coverageByField)...), summary, recognized, nil
}

type profilerFtraceCPUDetailAuthority struct {
	Malformed               bool
	CPU                     uint64
	CPUOccurrences          uint64
	CPUWrongWire            bool
	EventOccurrences        uint64
	EventPayloadOccurrences uint64
	EventWrongWire          uint64
	Overwrite               uint64
	OverwriteOccurrences    uint64
	OverwriteWrongWire      bool
	PairFamilies            pairCriticalFormatFamilyMask
	payload                 []byte
}

func auditProfilerFtraceCPUDetail(ctx context.Context, data []byte) (profilerFtraceCPUDetailAuthority, error) {
	var authority profilerFtraceCPUDetailAuthority
	err := walkProfilerProtoFieldsContext(ctx, data, func(field int, wire int, _ []byte, value uint64) error {
		switch field {
		case 1:
			if authority.CPUOccurrences == math.MaxUint64 {
				return &traceDBOutputInvariantError{Reason: "profiler_cpu_detail_cpu_census_overflow"}
			}
			authority.CPUOccurrences++
			if wire != 0 {
				authority.CPUWrongWire = true
				return nil
			}
			authority.CPU = value
		case 2:
			if authority.EventOccurrences == math.MaxUint64 {
				return &traceDBOutputInvariantError{Reason: "profiler_cpu_detail_event_census_overflow"}
			}
			authority.EventOccurrences++
			if wire != 2 {
				if authority.EventWrongWire == math.MaxUint64 {
					return &traceDBOutputInvariantError{Reason: "profiler_cpu_detail_event_wrong_wire_census_overflow"}
				}
				authority.EventWrongWire++
				return nil
			}
			if authority.EventPayloadOccurrences == math.MaxUint64 {
				return &traceDBOutputInvariantError{Reason: "profiler_cpu_detail_event_payload_census_overflow"}
			}
			authority.EventPayloadOccurrences++
		case 3:
			if authority.OverwriteOccurrences == math.MaxUint64 {
				return &traceDBOutputInvariantError{Reason: "profiler_cpu_detail_overwrite_census_overflow"}
			}
			authority.OverwriteOccurrences++
			if wire != 0 {
				authority.OverwriteWrongWire = true
				return nil
			}
			authority.Overwrite = value
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return profilerFtraceCPUDetailAuthority{}, err
		}
		if _, invariant := traceDBOutputInvariantReason(err); invariant {
			return profilerFtraceCPUDetailAuthority{}, err
		}
		families, familyErr := profilerPairFamiliesFromCPUDetailContext(ctx, data)
		if familyErr != nil {
			return profilerFtraceCPUDetailAuthority{}, familyErr
		}
		return profilerFtraceCPUDetailAuthority{
			Malformed: true, PairFamilies: families,
		}, nil
	}
	authority.payload = data
	return authority, nil
}

func (authority profilerFtraceCPUDetailAuthority) cpuIssue() (profilerFtraceEventIssueKind, bool) {
	switch {
	case authority.CPUOccurrences > 1:
		return profilerFtraceEventIssueEnvelopeCPUDuplicate, true
	case authority.CPUWrongWire:
		return profilerFtraceEventIssueEnvelopeCPUWrongWire, true
	case authority.CPU > uint64(maxTraceDBCPUIndex):
		return profilerFtraceEventIssueEnvelopeCPUOutOfRange, true
	default:
		return 0, false
	}
}

func visitProfilerFtraceCPUDetailEvents(ctx context.Context, authority profilerFtraceCPUDetailAuthority, visit func(profilerFtraceEventRecord) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	callback := func(record profilerFtraceEventRecord) error {
		if visit != nil {
			return visit(record)
		}
		return nil
	}
	if authority.Malformed {
		record := profilerFtraceEventRecord{
			Field:             profilerFtraceCPUDetailEnvelopeField,
			PairFamilies:      authority.PairFamilies,
			PairCaptureOpaque: true,
		}
		if err := record.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeCPUDetailMalformedWire); err != nil {
			return err
		}
		return callback(record)
	}
	if authority.EventPayloadOccurrences > authority.EventOccurrences ||
		authority.EventWrongWire != authority.EventOccurrences-authority.EventPayloadOccurrences {
		return &traceDBOutputInvariantError{Reason: "profiler_cpu_detail_event_census_invalid"}
	}

	cpuIssue, cpuIssuePresent := authority.cpuIssue()
	var eventObserved, eventPayloadObserved uint64
	var callbackErr error
	err := walkProfilerProtoFieldsContext(ctx, authority.payload, func(field int, wire int, raw []byte, _ uint64) error {
		if field != 2 {
			return nil
		}
		if eventObserved == math.MaxUint64 {
			return &traceDBOutputInvariantError{Reason: "profiler_cpu_detail_visitor_event_census_overflow"}
		}
		eventObserved++
		if wire != 2 {
			return nil
		}
		if eventPayloadObserved == math.MaxUint64 {
			return &traceDBOutputInvariantError{Reason: "profiler_cpu_detail_visitor_payload_census_overflow"}
		}
		eventPayloadObserved++
		event, decodeErr := decodeProfilerFtraceEventRecordContext(ctx, authority.CPU, raw)
		if decodeErr != nil {
			return decodeErr
		}
		if cpuIssuePresent {
			if issueErr := event.appendEnvelopeIssue(cpuIssue); issueErr != nil {
				return issueErr
			}
		}
		callbackErr = callback(event)
		return callbackErr
	})
	if err != nil {
		if callbackErr != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if _, invariant := traceDBOutputInvariantReason(err); invariant {
			return err
		}
		return &traceDBOutputInvariantError{Reason: "profiler_cpu_detail_visitor_parse_drift"}
	}
	if eventObserved != authority.EventOccurrences || eventPayloadObserved != authority.EventPayloadOccurrences {
		return &traceDBOutputInvariantError{Reason: "profiler_cpu_detail_visitor_census_drift"}
	}
	if eventPayloadObserved == 0 && cpuIssuePresent {
		record := profilerFtraceEventRecord{Field: profilerFtraceCPUDetailEnvelopeField}
		if err := record.appendEnvelopeIssue(cpuIssue); err != nil {
			return err
		}
		if err := callback(record); err != nil {
			return err
		}
	}
	if authority.EventWrongWire > 0 {
		record := profilerFtraceEventRecord{
			Field:             profilerFtraceCPUDetailEnvelopeField,
			PairCaptureOpaque: true,
		}
		if err := record.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeEventContainerWrongWire); err != nil {
			return err
		}
		if err := callback(record); err != nil {
			return err
		}
	}
	if authority.OverwriteOccurrences > 1 || authority.OverwriteWrongWire {
		record := profilerFtraceEventRecord{Field: profilerFtraceCPUDetailEnvelopeField}
		if err := record.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeOverwriteInvalid); err != nil {
			return err
		}
		if err := callback(record); err != nil {
			return err
		}
	}
	return nil
}

func profilerPairFamiliesFromCPUDetail(data []byte) pairCriticalFormatFamilyMask {
	families, _ := profilerPairFamiliesFromCPUDetailContext(context.Background(), data)
	return families
}

func profilerPairFamiliesFromCPUDetailContext(ctx context.Context, data []byte) (pairCriticalFormatFamilyMask, error) {
	var families pairCriticalFormatFamilyMask
	checkpoint := uint64(0)
	for len(data) > 0 {
		if err := profilerProtoContextCheckpoint(ctx, &checkpoint); err != nil {
			return families, err
		}
		key, consumed, ok := consumeProtoVarint(data)
		if !ok {
			break
		}
		data = data[consumed:]
		fieldNumber := key >> 3
		if fieldNumber < 1 || fieldNumber > (1<<29)-1 {
			return families, nil
		}
		field, wire := int(fieldNumber), int(key&0x7)
		switch wire {
		case 0:
			_, consumed, ok = consumeProtoVarint(data)
			if !ok {
				return families, nil
			}
			data = data[consumed:]
		case 1:
			if len(data) < 8 {
				return families, nil
			}
			data = data[8:]
		case 2:
			length, lengthBytes, valid := consumeProtoVarint(data)
			if !valid {
				return families, nil
			}
			data = data[lengthBytes:]
			available := uint64(len(data))
			payloadLength := length
			if payloadLength > available {
				payloadLength = available
			}
			if field == 2 {
				eventFamilies, err := profilerPairFamiliesFromEventPayloadContext(ctx, data[:int(payloadLength)])
				if err != nil {
					return families, err
				}
				families |= eventFamilies
			}
			if length > available {
				return families, nil
			}
			data = data[int(length):]
		case 5:
			if len(data) < 4 {
				return families, nil
			}
			data = data[4:]
		default:
			return families, nil
		}
	}
	return families, nil
}

func profilerPairFamiliesFromEventPayload(data []byte) pairCriticalFormatFamilyMask {
	families, _ := profilerPairFamiliesFromEventPayloadContext(context.Background(), data)
	return families
}

func profilerPairFamiliesFromEventPayloadContext(ctx context.Context, data []byte) (pairCriticalFormatFamilyMask, error) {
	var families pairCriticalFormatFamilyMask
	checkpoint := uint64(0)
	for len(data) > 0 {
		if err := profilerProtoContextCheckpoint(ctx, &checkpoint); err != nil {
			return families, err
		}
		key, consumed, ok := consumeProtoVarint(data)
		if !ok {
			break
		}
		data = data[consumed:]
		fieldNumber := key >> 3
		if fieldNumber < 1 || fieldNumber > (1<<29)-1 {
			return families, nil
		}
		field, wire := int(fieldNumber), int(key&0x7)
		families |= profilerPairFamilyForField(field)
		switch wire {
		case 0:
			_, consumed, ok = consumeProtoVarint(data)
			if !ok {
				return families, nil
			}
			data = data[consumed:]
		case 1:
			if len(data) < 8 {
				return families, nil
			}
			data = data[8:]
		case 2:
			length, lengthBytes, valid := consumeProtoVarint(data)
			if !valid {
				return families, nil
			}
			data = data[lengthBytes:]
			if length > uint64(len(data)) {
				return families, nil
			}
			data = data[int(length):]
		case 5:
			if len(data) < 4 {
				return families, nil
			}
			data = data[4:]
		default:
			return families, nil
		}
	}
	return families, nil
}

type profilerProtoEnvelopeField struct {
	count     int
	wrongWire bool
	uintValue uint64
	bytes     []byte
}

func decodeProfilerFtraceEventRecord(cpu uint64, data []byte) (profilerFtraceEventRecord, error) {
	return decodeProfilerFtraceEventRecordContext(context.Background(), cpu, data)
}

func decodeProfilerFtraceEventRecordContext(ctx context.Context, cpu uint64, data []byte) (profilerFtraceEventRecord, error) {
	record := profilerFtraceEventRecord{CPU: int64(cpu)}
	var timestamp, tgid, comm, common profilerProtoEnvelopeField
	oneofCount := 0
	oneofWrongWire := false
	err := walkProfilerProtoFieldsContext(ctx, data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			profilerProtoEnvelopeSetUint(&timestamp, wire, v)
		case 2:
			profilerProtoEnvelopeSetUint(&tgid, wire, v)
		case 3:
			profilerProtoEnvelopeSetBytes(&comm, wire, raw)
		case 50:
			profilerProtoEnvelopeSetBytes(&common, wire, raw)
		default:
			if field >= 100 {
				record.PairFamilies |= profilerPairFamilyForField(field)
				oneofCount++
				if wire != 2 {
					oneofWrongWire = true
					break
				}
				record.Field = field
				record.Payload = raw
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return profilerFtraceEventRecord{}, err
		}
		if _, invariant := traceDBOutputInvariantReason(err); invariant {
			return profilerFtraceEventRecord{}, err
		}
		var decodeErr *protoFieldDecodeError
		if errors.As(err, &decodeErr) && decodeErr.FieldKnown {
			// A complete oneof key is precise family provenance even when its
			// value is truncated/unsupported and the callback never ran. The
			// closed mapper deliberately excludes Block inventory fields.
			record.PairFamilies |= profilerPairFamilyForField(decodeErr.Field)
		}
		record.Field = 0
		record.Payload = nil
		record.PairCaptureOpaque = true
		if issueErr := record.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeEventMalformedWire); issueErr != nil {
			return profilerFtraceEventRecord{}, issueErr
		}
		return record, nil
	}
	var oneofIssueKind profilerFtraceEventIssueKind
	oneofIssuePresent := true
	switch {
	case oneofCount == 0:
		record.Field = 0
		record.Payload = nil
		record.PairCaptureOpaque = true
		oneofIssueKind = profilerFtraceEventIssueEnvelopeOneofMissing
	case oneofCount > 1:
		record.Field = 0
		record.Payload = nil
		oneofIssueKind = profilerFtraceEventIssueEnvelopeOneofMultiple
	case oneofWrongWire:
		record.Field = 0
		record.Payload = nil
		oneofIssueKind = profilerFtraceEventIssueEnvelopeOneofWrongWire
	default:
		oneofIssuePresent = false
	}
	if kind, present := profilerProtoEnvelopeOptionalIssueKind(timestamp,
		profilerFtraceEventIssueEnvelopeTimestampDuplicate,
		profilerFtraceEventIssueEnvelopeTimestampWrongWire); present {
		if issueErr := record.appendEnvelopeIssue(kind); issueErr != nil {
			return profilerFtraceEventRecord{}, issueErr
		}
	} else {
		record.TSNS = timestamp.uintValue
		if record.TSNS > math.MaxInt64 {
			if issueErr := record.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeTimestampOutOfRange); issueErr != nil {
				return profilerFtraceEventRecord{}, issueErr
			}
		}
	}
	if kind, present := profilerProtoEnvelopeOptionalIssueKind(tgid,
		profilerFtraceEventIssueEnvelopeTGIDDuplicate,
		profilerFtraceEventIssueEnvelopeTGIDWrongWire); present {
		if issueErr := record.appendEnvelopeIssue(kind); issueErr != nil {
			return profilerFtraceEventRecord{}, issueErr
		}
	} else if tgid.uintValue > math.MaxInt32 {
		if issueErr := record.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeTGIDOutOfRange); issueErr != nil {
			return profilerFtraceEventRecord{}, issueErr
		}
	} else {
		record.TGID = int64(tgid.uintValue)
	}
	if kind, present := profilerProtoEnvelopeOptionalIssueKind(comm,
		profilerFtraceEventIssueEnvelopeCommDuplicate,
		profilerFtraceEventIssueEnvelopeCommWrongWire); present {
		if issueErr := record.appendEnvelopeIssue(kind); issueErr != nil {
			return profilerFtraceEventRecord{}, issueErr
		}
	} else {
		record.Comm = string(comm.bytes)
		if comm.count == 1 && !traceDBSinglePhysicalLine(record.Comm, true) {
			if issueErr := record.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeCommInvalid); issueErr != nil {
				return profilerFtraceEventRecord{}, issueErr
			}
		}
	}
	if kind, present := profilerProtoEnvelopeRequiredIssueKind(common,
		profilerFtraceEventIssueEnvelopeCommonFieldsMissing,
		profilerFtraceEventIssueEnvelopeCommonFieldsDuplicate,
		profilerFtraceEventIssueEnvelopeCommonFieldsWrongWire); present {
		if issueErr := record.appendEnvelopeIssue(kind); issueErr != nil {
			return profilerFtraceEventRecord{}, issueErr
		}
	} else {
		pid, ownerKnown, ownerPresent, flags, preempt, commonErr := decodeProfilerFtraceCommonFieldsContext(ctx, &record, common.bytes)
		if commonErr != nil {
			return profilerFtraceEventRecord{}, commonErr
		}
		record.PID = pid
		record.HeaderOwnerKnown = ownerKnown
		record.HeaderOwnerPresent = ownerPresent
		record.CommonFlags = flags
		record.CommonPreemptCount = preempt
	}
	if oneofIssuePresent {
		if issueErr := record.appendEnvelopeIssue(oneofIssueKind); issueErr != nil {
			return profilerFtraceEventRecord{}, issueErr
		}
	}
	// Upstream only resolves/sets TGID for a non-idle PID and may honestly
	// leave it at proto3 zero when the process map has no answer. Preserve that
	// as the systrace "-----" TGID, never fabricate TGID=TID. The inverse
	// shape (idle PID with a positive TGID) is not producer-reachable.
	if record.HeaderOwnerKnown && record.PID == 0 && record.TGID != 0 {
		if issueErr := record.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeIdentityIncomplete); issueErr != nil {
			return profilerFtraceEventRecord{}, issueErr
		}
	}
	return record, nil
}

func profilerPairFamilyForField(field int) pairCriticalFormatFamilyMask {
	switch {
	case profilerStructuredPairEventField(pairRenderF2FS, field):
		return pairCriticalFormatFamilyF2FS
	case profilerStructuredPairEventField(pairRenderMMC, field):
		return pairCriticalFormatFamilyMMC
	case profilerStructuredPairEventField(pairRenderBlock, field):
		return pairCriticalFormatFamilyBlock
	default:
		return 0
	}
}

func profilerProtoEnvelopeSetUint(field *profilerProtoEnvelopeField, wire int, value uint64) {
	field.count++
	if wire != 0 {
		field.wrongWire = true
		return
	}
	field.uintValue = value
}

func profilerProtoEnvelopeSetBytes(field *profilerProtoEnvelopeField, wire int, value []byte) {
	field.count++
	if wire != 2 {
		field.wrongWire = true
		return
	}
	field.bytes = value
}

func profilerProtoEnvelopeOptionalIssueKind(field profilerProtoEnvelopeField,
	duplicateKind, wrongWireKind profilerFtraceEventIssueKind) (profilerFtraceEventIssueKind, bool) {
	if field.count > 1 {
		return duplicateKind, true
	}
	if field.wrongWire {
		return wrongWireKind, true
	}
	return 0, false
}

func profilerProtoEnvelopeRequiredIssueKind(field profilerProtoEnvelopeField,
	missingKind, duplicateKind, wrongWireKind profilerFtraceEventIssueKind) (profilerFtraceEventIssueKind, bool) {
	if field.count == 0 {
		return missingKind, true
	}
	return profilerProtoEnvelopeOptionalIssueKind(field, duplicateKind, wrongWireKind)
}

func decodeProfilerFtraceCommonFields(record *profilerFtraceEventRecord, data []byte) (pid int64, ownerKnown, ownerPresent bool,
	flags, preempt int64, err error) {
	return decodeProfilerFtraceCommonFieldsContext(context.Background(), record, data)
}

func decodeProfilerFtraceCommonFieldsContext(ctx context.Context, record *profilerFtraceEventRecord, data []byte) (pid int64, ownerKnown, ownerPresent bool,
	flags, preempt int64, err error) {
	if record == nil {
		return 0, false, false, 0, 0, &traceDBOutputInvariantError{Reason: "profiler_event_common_record_nil"}
	}
	var fields [5]profilerProtoEnvelopeField
	walkErr := walkProfilerProtoFieldsContext(ctx, data, func(field int, wire int, raw []byte, v uint64) error {
		if field >= 1 && field <= 4 {
			profilerProtoEnvelopeSetUint(&fields[field], wire, v)
		}
		_ = raw
		return nil
	})
	appendIssue := func(kind profilerFtraceEventIssueKind) bool {
		return record.appendEnvelopeIssue(kind) == nil
	}
	if walkErr != nil {
		if errors.Is(walkErr, context.Canceled) || errors.Is(walkErr, context.DeadlineExceeded) {
			return 0, false, false, 0, 0, walkErr
		}
		if _, invariant := traceDBOutputInvariantReason(walkErr); invariant {
			return 0, false, false, 0, 0, walkErr
		}
		if !appendIssue(profilerFtraceEventIssueEnvelopeCommonFieldsMalformedWire) {
			return 0, false, false, 0, 0, &traceDBOutputInvariantError{Reason: "profiler_event_common_issue_schema_invalid"}
		}
		return 0, false, false, 0, 0, nil
	}
	for _, item := range []struct {
		field         int
		duplicateKind profilerFtraceEventIssueKind
		wrongWireKind profilerFtraceEventIssueKind
	}{
		{field: 1, duplicateKind: profilerFtraceEventIssueEnvelopeCommonTypeDuplicate, wrongWireKind: profilerFtraceEventIssueEnvelopeCommonTypeWrongWire},
		{field: 2, duplicateKind: profilerFtraceEventIssueEnvelopeCommonFlagsDuplicate, wrongWireKind: profilerFtraceEventIssueEnvelopeCommonFlagsWrongWire},
		{field: 3, duplicateKind: profilerFtraceEventIssueEnvelopeCommonPreemptCountDuplicate, wrongWireKind: profilerFtraceEventIssueEnvelopeCommonPreemptCountWrongWire},
		{field: 4, duplicateKind: profilerFtraceEventIssueEnvelopeCommonPIDDuplicate, wrongWireKind: profilerFtraceEventIssueEnvelopeCommonPIDWrongWire},
	} {
		if kind, present := profilerProtoEnvelopeOptionalIssueKind(fields[item.field], item.duplicateKind, item.wrongWireKind); present && !appendIssue(kind) {
			return 0, false, false, 0, 0, &traceDBOutputInvariantError{Reason: "profiler_event_common_issue_schema_invalid"}
		}
	}
	if fields[1].count == 1 && !fields[1].wrongWire && fields[1].uintValue > math.MaxUint16 {
		if !appendIssue(profilerFtraceEventIssueEnvelopeCommonTypeSourceWidth) {
			return 0, false, false, 0, 0, &traceDBOutputInvariantError{Reason: "profiler_event_common_issue_schema_invalid"}
		}
	}
	if fields[2].count == 1 && !fields[2].wrongWire && fields[2].uintValue > math.MaxUint8 {
		if !appendIssue(profilerFtraceEventIssueEnvelopeCommonFlagsSourceWidth) {
			return 0, false, false, 0, 0, &traceDBOutputInvariantError{Reason: "profiler_event_common_issue_schema_invalid"}
		}
	}
	if fields[3].count == 1 && !fields[3].wrongWire && fields[3].uintValue > math.MaxUint8 {
		if !appendIssue(profilerFtraceEventIssueEnvelopeCommonPreemptCountSourceWidth) {
			return 0, false, false, 0, 0, &traceDBOutputInvariantError{Reason: "profiler_event_common_issue_schema_invalid"}
		}
	}
	if fields[4].count == 1 && !fields[4].wrongWire && fields[4].uintValue > math.MaxInt32 {
		if !appendIssue(profilerFtraceEventIssueEnvelopeCommonPIDOutOfRange) {
			return 0, false, false, 0, 0, &traceDBOutputInvariantError{Reason: "profiler_event_common_issue_schema_invalid"}
		}
	}
	ownerKnown = fields[4].count <= 1 && !fields[4].wrongWire && fields[4].uintValue <= math.MaxInt32
	ownerPresent = fields[4].count == 1 && !fields[4].wrongWire
	return int64(fields[4].uintValue), ownerKnown, ownerPresent, int64(fields[2].uintValue), int64(fields[3].uintValue), nil
}

func renderProfilerFtraceEventBody(event profilerFtraceEventRecord) (string, string, bool) {
	name, body, ok, _, err := renderProfilerFtraceEventBodyWithTypedAudit(event)
	if err != nil {
		return "", "", false
	}
	return name, body, ok
}

func safeProfilerBlockedCaller(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" || value != raw || len(value) > 512 {
		return "", false
	}
	for _, r := range value {
		// caller= is one systrace key/value token. Whitespace, equals, pipes and
		// controls would either truncate the parser-visible reason or inject a
		// second trace field/line, so such payloads fail closed to opaque.
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == '=' || r == '|' {
			return "", false
		}
	}
	return value, true
}

func renderProfilerFtraceEventBodyWithAudit(event profilerFtraceEventRecord) (string, string, bool, []string) {
	name, body, ok, issues, err := renderProfilerFtraceEventBodyWithTypedAudit(event)
	if err != nil {
		return "", "", false, nil
	}
	// The typed ledger always carries UnmappedField for conservation. The
	// pre-B2 compatibility surface historically exposed only independently
	// actionable envelope labels for unknown events, and no label for a plain
	// unsupported field. Preserve that byte contract without reversing text
	// into authority or changing the production typed verdict.
	if profilerFtraceEventSlot(event.Field) == profilerFtraceUnknownEventSlot {
		visible := make([]profilerFtraceEventIssue, 0, len(issues))
		for _, issue := range issues {
			if issue.Kind != profilerFtraceEventIssueUnmappedField {
				visible = append(visible, issue)
			}
		}
		issues = visible
	}
	labels, labelsOK := profilerFtraceEventIssueLabels(event.Field, issues)
	if !labelsOK {
		return "", "", false, nil
	}
	return name, body, ok, labels
}

// Every structured producer enters through this typed choke. Compatibility
// callers may render labels afterward, but no production path reverses text
// back into issue authority.
func renderProfilerFtraceEventBodyWithTypedAudit(event profilerFtraceEventRecord) (string, string, bool, []profilerFtraceEventIssue, error) {
	name, body, ok, issues, _, err := renderProfilerFtraceEventBodyWithTypedAuditAndPair(event)
	return name, body, ok, issues, err
}

func renderProfilerFtraceEventBodyWithTypedAuditAndPair(event profilerFtraceEventRecord) (
	string, string, bool, []profilerFtraceEventIssue, profilerPairAdmission, error,
) {
	pair := profilerStructuredF2FSPairFamily(event.Field)
	var blockPayload blockRenderPayload
	var blockAdmission bodyAdmission
	var blockIssues profilerFtraceBlockIssueSet
	blockHandled := false
	if _, _, blockGoverned := blockRenderKindForProfilerField(event.Field); blockGoverned {
		var blockErr error
		blockPayload, blockAdmission, blockIssues, blockHandled, blockErr =
			decodeProfilerBlockPayloadWithTypedAuditInto(event, &pair)
		if blockErr != nil {
			return "", "", false, nil, pair, blockErr
		}
	}
	var auxResult profilerFtraceAuxTypedResult
	auxDecoded := false
	if profilerStructuredAuxSchemas[event.Field] != nil {
		var auxErr error
		auxResult, auxErr = decodeProfilerAuxPayloadWithTypedAudit(event)
		pair, auxDecoded = auxResult.Pair, true
		if auxErr != nil {
			return "", "", false, nil, pair, auxErr
		}
		if !auxResult.Handled {
			return "", "", false, nil, pair,
				&traceDBOutputInvariantError{Reason: "profiler_aux_typed_renderer_unhandled"}
		}
	}
	envelopeIssues, envelopeErr := event.checkedEnvelopeIssues()
	if envelopeErr != nil {
		return "", "", false, nil, pair, envelopeErr
	}
	if len(envelopeIssues) > 0 {
		issues := append([]profilerFtraceEventIssue(nil), envelopeIssues...)
		if profilerFtraceEventSlot(event.Field) == profilerFtraceUnknownEventSlot {
			if event.Field < 100 || event.Field > profilerFtraceUnknownEventAggregateField {
				return "", "", false, nil, pair, &traceDBOutputInvariantError{Reason: "profiler_event_field_domain_invalid"}
			}
			unmapped, valid := profilerFtraceEventFixedIssue(event.Field, profilerFtraceEventIssueUnmappedField)
			if !valid {
				return "", "", false, nil, pair, &traceDBOutputInvariantError{Reason: "profiler_event_unmapped_issue_invalid"}
			}
			issues = append(issues, unmapped)
		}
		return "", "", false, issues, pair, nil
	}
	if name, body, ok, issues, handled, coreErr := renderProfilerFtraceCoreEventWithTypedAudit(event); handled {
		return name, body, ok, issues, pair, coreErr
	}
	if profilerStructuredCoreSchemas[event.Field] != nil {
		return "", "", false, nil, pair, &traceDBOutputInvariantError{Reason: "profiler_core_typed_renderer_unhandled"}
	}
	if auxDecoded {
		name, body, ok, issues, auxErr := finalizeProfilerFtraceAuxEventWithTypedAudit(event, auxResult)
		return name, body, ok, issues, pair, auxErr
	}
	if name, body, ok, issues, handled, filemapErr := renderProfilerFtraceFilemapEventWithTypedAudit(event); handled {
		return name, body, ok, issues, pair, filemapErr
	}
	if event.Field == 1000 || event.Field == 1001 {
		return "", "", false, nil, pair,
			&traceDBOutputInvariantError{Reason: "profiler_filemap_typed_renderer_unhandled"}
	}
	if blockHandled {
		name, body, ok, issues, blockFinalizeErr :=
			finalizeProfilerFtraceBlockEventWithTypedAudit(event, blockPayload, blockAdmission, blockIssues)
		return name, body, ok, issues, pair, blockFinalizeErr
	}
	if _, _, block := blockRenderKindForProfilerField(event.Field); block {
		return "", "", false, nil, pair,
			&traceDBOutputInvariantError{Reason: "profiler_block_typed_renderer_unhandled"}
	}
	if name, body, ok, issues, handled, genericErr := renderProfilerFtraceGenericEventWithTypedAudit(event); handled {
		return name, body, ok, issues, pair, genericErr
	}
	if profilerFtraceEventSlot(event.Field) == profilerFtraceUnknownEventSlot {
		if event.Field < 100 || event.Field > profilerFtraceUnknownEventAggregateField {
			return "", "", false, nil, pair, &traceDBOutputInvariantError{Reason: "profiler_event_field_domain_invalid"}
		}
		unmapped, valid := profilerFtraceEventFixedIssue(event.Field, profilerFtraceEventIssueUnmappedField)
		if !valid {
			return "", "", false, nil, pair, &traceDBOutputInvariantError{Reason: "profiler_event_unmapped_issue_invalid"}
		}
		return "", "", false, []profilerFtraceEventIssue{unmapped}, pair, nil
	}
	return "", "", false, nil, pair,
		&traceDBOutputInvariantError{Reason: "profiler_event_typed_renderer_unhandled"}
}

const profilerFtraceGenericIssuesPerEvent = 7

type profilerFtraceGenericIssueSet struct {
	Count  uint8
	Issues [profilerFtraceGenericIssuesPerEvent]profilerFtraceEventIssue
}

func (set *profilerFtraceGenericIssueSet) validate(eventField int) error {
	if set == nil || int(set.Count) > len(set.Issues) {
		return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_count_invalid"}
	}
	var payloadFields [9]bool
	severitySet := false
	var severity profilerFtraceEventIssueSeverity
	displayCount := 0
	issueArm := uint8(0)
	for index, issue := range set.Issues {
		if index >= int(set.Count) {
			if issue != (profilerFtraceEventIssue{}) {
				return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_count_invalid"}
			}
			continue
		}
		if issue.Kind < profilerFtraceEventIssueWirePayloadMalformedWire ||
			issue.Kind > profilerFtraceEventIssueWireInvalidCanonicalLine ||
			!issue.validFor(eventField) || issue.Severity != issue.expectedSeverity() {
			return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_schema_invalid"}
		}
		for prior := 0; prior < index; prior++ {
			if set.Issues[prior] == issue {
				return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_duplicate"}
			}
		}
		if issue.Kind == profilerFtraceEventIssueWirePayloadMalformedWire ||
			issue.Kind == profilerFtraceEventIssueWireInvalidCanonicalLine {
			if set.Count != 1 {
				return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_arm_invalid"}
			}
		}
		currentArm := uint8(0)
		switch issue.Kind {
		case profilerFtraceEventIssueWirePayloadMalformedWire,
			profilerFtraceEventIssueWireInvalidCanonicalLine:
			currentArm = 1 // sole whole-message/canonical failure
		case profilerFtraceEventIssueWireFieldMalformedWire:
			currentArm = 2 // sole localized structural endpoint failure
			if set.Count != 1 {
				return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_arm_invalid"}
			}
		case profilerFtraceEventIssueWireFieldWrongWire,
			profilerFtraceEventIssueWireFieldDuplicate,
			profilerFtraceEventIssueWireFieldMissingOrInvalid:
			currentArm = 3 // completed-scan hard audit
		case profilerFtraceEventIssueWireFieldOutOfRange:
			currentArm = 4 // range audit runs only after a clean hard audit
		case profilerFtraceEventIssueWireCPUIDMalformedWire,
			profilerFtraceEventIssueWireCPUIDWrongWire,
			profilerFtraceEventIssueWireCPUIDDuplicate,
			profilerFtraceEventIssueWireCPUIDOutOfRange,
			profilerFtraceEventIssueWireNextInfoMalformedWire,
			profilerFtraceEventIssueWireNextInfoWrongWire,
			profilerFtraceEventIssueWireNextInfoDuplicate:
			currentArm = 5 // the single admitted CPU/next-info display issue
		default:
			return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_schema_invalid"}
		}
		if issueArm != 0 && issueArm != currentArm {
			return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_arm_invalid"}
		}
		issueArm = currentArm
		if int(issue.PayloadField) >= len(payloadFields) || payloadFields[issue.PayloadField] {
			return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_endpoint_conflict"}
		}
		payloadFields[issue.PayloadField] = true
		if severitySet && issue.Severity != severity {
			return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_arm_invalid"}
		}
		severity, severitySet = issue.Severity, true
		if issue.Severity == profilerFtraceEventIssueAdmittedDisplay {
			displayCount++
			if displayCount > 1 {
				return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_arm_invalid"}
			}
		}
	}
	return nil
}

func (set *profilerFtraceGenericIssueSet) add(eventField int, issue profilerFtraceEventIssue) error {
	if err := set.validate(eventField); err != nil {
		return err
	}
	if issue.Kind < profilerFtraceEventIssueWirePayloadMalformedWire ||
		issue.Kind > profilerFtraceEventIssueWireInvalidCanonicalLine ||
		!issue.validFor(eventField) || issue.Severity != issue.expectedSeverity() {
		return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_schema_invalid"}
	}
	for index := 0; index < int(set.Count); index++ {
		if set.Issues[index] == issue {
			return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_duplicate"}
		}
	}
	if int(set.Count) == len(set.Issues) {
		return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_overflow"}
	}
	candidate := *set
	candidate.Issues[int(candidate.Count)] = issue
	candidate.Count++
	if err := candidate.validate(eventField); err != nil {
		return err
	}
	*set = candidate
	return nil
}

func (set *profilerFtraceGenericIssueSet) addFixed(eventField int, kind profilerFtraceEventIssueKind) error {
	issue, ok := profilerFtraceEventFixedIssue(eventField, kind)
	if !ok {
		return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_schema_invalid"}
	}
	return set.add(eventField, issue)
}

func (set *profilerFtraceGenericIssueSet) addPayload(eventField int, kind profilerFtraceEventIssueKind, payloadField int) error {
	issue, ok := profilerFtraceEventPayloadIssue(eventField, kind, payloadField)
	if !ok {
		return &traceDBOutputInvariantError{Reason: "profiler_generic_issue_schema_invalid"}
	}
	return set.add(eventField, issue)
}

func (set *profilerFtraceGenericIssueSet) checked(eventField int) ([]profilerFtraceEventIssue, error) {
	if err := set.validate(eventField); err != nil {
		return nil, err
	}
	return append([]profilerFtraceEventIssue(nil), set.Issues[:int(set.Count)]...), nil
}

type profilerFtraceGenericFieldRole uint8

const (
	profilerFtraceGenericFieldUnknown profilerFtraceGenericFieldRole = iota
	profilerFtraceGenericFieldHardString
	profilerFtraceGenericFieldHardUint
	profilerFtraceGenericFieldDisplayUint
)

type profilerFtraceGenericFieldState struct {
	Count       uint8 // saturates at two: absent, singular, or duplicate
	WrongWire   bool
	Malformed   bool
	UintValue   uint64
	StringValue string
}

func profilerFtraceGenericFieldSchema(eventField, payloadField int) (profilerFtraceGenericFieldRole, int, bool) {
	switch eventField {
	case 410, 2002:
		switch payloadField {
		case 1:
			return profilerFtraceGenericFieldHardString, 2, true
		case 2:
			return profilerFtraceGenericFieldHardUint, 0, true
		case 3:
			if eventField == 2002 {
				return profilerFtraceGenericFieldDisplayUint, 0, true
			}
		}
	case 2417:
		switch payloadField {
		case 1, 5:
			return profilerFtraceGenericFieldHardString, 2, true
		case 2, 3, 4, 6, 7:
			return profilerFtraceGenericFieldHardUint, 0, true
		case 8:
			return profilerFtraceGenericFieldDisplayUint, 0, true
		}
	}
	return profilerFtraceGenericFieldUnknown, 0, false
}

// renderProfilerFtraceGenericEventWithTypedAudit is the single parser and
// renderer authority for profiler fields 410, 2002, and 2417. Every endpoint
// is observed during one wire walk; issue publication happens only afterward
// in schema order, so source order cannot change diagnostics or precedence.
func renderProfilerFtraceGenericEventWithTypedAudit(event profilerFtraceEventRecord) (name, body string, ok bool, issues []profilerFtraceEventIssue, handled bool, err error) {
	switch event.Field {
	case 410, 2002, 2417:
		handled = true
	default:
		return "", "", false, nil, false, nil
	}

	var fields [9]profilerFtraceGenericFieldState
	walkErr := walkProtoFields(event.Payload, func(payloadField int, wire int, raw []byte, value uint64) error {
		role, expectedWire, known := profilerFtraceGenericFieldSchema(event.Field, payloadField)
		if !known {
			return nil
		}
		state := &fields[payloadField]
		if state.Count < 2 {
			state.Count++
		}
		if wire != expectedWire {
			state.WrongWire = true
			return nil
		}
		if role == profilerFtraceGenericFieldHardString {
			state.StringValue = string(raw)
		} else {
			state.UintValue = value
		}
		return nil
	})
	if walkErr != nil {
		var decodeErr *protoFieldDecodeError
		if !errors.As(walkErr, &decodeErr) {
			return "", "", false, nil, true, &traceDBOutputInvariantError{Reason: "profiler_generic_wire_error_untyped"}
		}
		role, _, known := profilerFtraceGenericFieldSchema(event.Field, decodeErr.Field)
		localizedFailure := decodeErr.Failure == protoFieldDecodeMalformedValue ||
			decodeErr.Failure == protoFieldDecodeUnsupportedWire
		if !localizedFailure || !decodeErr.FieldKnown || !known ||
			(role == profilerFtraceGenericFieldDisplayUint && !decodeErr.Terminal) {
			var set profilerFtraceGenericIssueSet
			if addErr := set.addFixed(event.Field, profilerFtraceEventIssueWirePayloadMalformedWire); addErr != nil {
				return "", "", false, nil, true, addErr
			}
			issues, checkErr := set.checked(event.Field)
			return "", "", false, issues, true, checkErr
		}
		if role != profilerFtraceGenericFieldDisplayUint {
			// A structural failure ends the scan. Returning only its localized
			// hard endpoint prevents fields beyond that physical boundary from
			// being falsely minted as missing or defaulted observations.
			var set profilerFtraceGenericIssueSet
			if addErr := set.addPayload(event.Field, profilerFtraceEventIssueWireFieldMalformedWire, decodeErr.Field); addErr != nil {
				return "", "", false, nil, true, addErr
			}
			issues, checkErr := set.checked(event.Field)
			return "", "", false, issues, true, checkErr
		}
		fields[decodeErr.Field].Malformed = true
	}

	var set profilerFtraceGenericIssueSet
	stringFields := [2]int{1, 0}
	stringFieldCount := 1
	scalarFields := [5]int{2, 0, 0, 0, 0}
	scalarFieldCount := 1
	if event.Field == 2417 {
		stringFields = [2]int{1, 5}
		stringFieldCount = 2
		scalarFields = [5]int{2, 3, 4, 6, 7}
		scalarFieldCount = 5
	}
	for index := 0; index < stringFieldCount; index++ {
		payloadField := stringFields[index]
		state := fields[payloadField]
		var kind profilerFtraceEventIssueKind
		issuePresent := true
		switch {
		case state.Malformed:
			kind = profilerFtraceEventIssueWireFieldMalformedWire
		case state.WrongWire:
			kind = profilerFtraceEventIssueWireFieldWrongWire
		case state.Count > 1:
			kind = profilerFtraceEventIssueWireFieldDuplicate
		case state.Count == 0:
			kind = profilerFtraceEventIssueWireFieldMissingOrInvalid
		case (event.Field == 410 || event.Field == 2002) && !traceDBSingleToken(state.StringValue):
			kind = profilerFtraceEventIssueWireFieldMissingOrInvalid
		case event.Field == 2417 && !traceDBSinglePhysicalLine(state.StringValue, false):
			kind = profilerFtraceEventIssueWireFieldMissingOrInvalid
		default:
			issuePresent = false
		}
		if issuePresent {
			if addErr := set.addPayload(event.Field, kind, payloadField); addErr != nil {
				return "", "", false, nil, true, addErr
			}
		}
	}
	for index := 0; index < scalarFieldCount; index++ {
		payloadField := scalarFields[index]
		state := fields[payloadField]
		var kind profilerFtraceEventIssueKind
		issuePresent := true
		switch {
		case state.Malformed:
			kind = profilerFtraceEventIssueWireFieldMalformedWire
		case state.WrongWire:
			kind = profilerFtraceEventIssueWireFieldWrongWire
		case state.Count > 1:
			kind = profilerFtraceEventIssueWireFieldDuplicate
		default:
			issuePresent = false
		}
		if issuePresent {
			if addErr := set.addPayload(event.Field, kind, payloadField); addErr != nil {
				return "", "", false, nil, true, addErr
			}
		}
	}
	// Proto3 scalar absence is the exact default zero under these pinned
	// generated producer profiles. Range checks run only after all wire and
	// required-string authority is clean, preserving the established ordering.
	if event.Field == 2417 && set.Count == 0 {
		for _, payloadField := range [2]int{2, 6} {
			state := fields[payloadField]
			if state.Count == 1 && state.UintValue > math.MaxInt32 {
				if addErr := set.addPayload(event.Field, profilerFtraceEventIssueWireFieldOutOfRange, payloadField); addErr != nil {
					return "", "", false, nil, true, addErr
				}
			}
		}
		for _, payloadField := range [2]int{3, 7} {
			state := fields[payloadField]
			signed := int64(state.UintValue)
			if state.Count == 1 && (signed < math.MinInt32 || signed > math.MaxInt32) {
				if addErr := set.addPayload(event.Field, profilerFtraceEventIssueWireFieldOutOfRange, payloadField); addErr != nil {
					return "", "", false, nil, true, addErr
				}
			}
		}
	}
	if set.Count > 0 {
		issues, checkErr := set.checked(event.Field)
		return "", "", false, issues, true, checkErr
	}

	displayValue := uint64(0)
	displayValid := event.Field == 2002 || event.Field == 2417
	if displayValid {
		displayField := 3
		malformedKind := profilerFtraceEventIssueWireCPUIDMalformedWire
		wrongKind := profilerFtraceEventIssueWireCPUIDWrongWire
		duplicateKind := profilerFtraceEventIssueWireCPUIDDuplicate
		if event.Field == 2417 {
			displayField = 8
			malformedKind = profilerFtraceEventIssueWireNextInfoMalformedWire
			wrongKind = profilerFtraceEventIssueWireNextInfoWrongWire
			duplicateKind = profilerFtraceEventIssueWireNextInfoDuplicate
		}
		state := fields[displayField]
		displayValue = state.UintValue
		var kind profilerFtraceEventIssueKind
		issuePresent := true
		switch {
		case state.Malformed:
			kind = malformedKind
		case state.WrongWire:
			kind = wrongKind
		case state.Count > 1:
			kind = duplicateKind
		case event.Field == 2002 && state.Count == 1 && state.UintValue > uint64(maxTraceDBCPUIndex):
			kind = profilerFtraceEventIssueWireCPUIDOutOfRange
		default:
			issuePresent = false
		}
		if issuePresent {
			displayValid = false
			if addErr := set.addFixed(event.Field, kind); addErr != nil {
				return "", "", false, nil, true, addErr
			}
		}
	}

	switch event.Field {
	case 410:
		name = "clock_set_rate"
		body = fmt.Sprintf("%s state=%d", fields[1].StringValue, fields[2].UintValue)
	case 2002:
		name = "clock_set_rate"
		parts := []string{fields[1].StringValue, fmt.Sprintf("state=%d", fields[2].UintValue)}
		if displayValid {
			parts = appendClockSetRateCPU(parts, displayValue)
		}
		body = strings.Join(parts, " ")
	case 2417:
		name = "sched_switch"
		body = fmt.Sprintf("prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d",
			fields[1].StringValue, int64(fields[2].UintValue), int64(fields[3].UintValue),
			linuxPrevState(fields[4].UintValue), fields[5].StringValue, int64(fields[6].UintValue), int64(fields[7].UintValue))
		// MaxUint64 is the producer's missing sentinel. Wire absence is the
		// authoritative packed zero tuple only for this exact field profile.
		if displayValid && displayValue != math.MaxUint64 {
			body += " next_info=" + formatHarmonySchedInfo(displayValue, true)
		}
	}
	if !profilerCanonicalLineValid(event, name, body) {
		var canonicalSet profilerFtraceGenericIssueSet
		if addErr := canonicalSet.addFixed(event.Field, profilerFtraceEventIssueWireInvalidCanonicalLine); addErr != nil {
			return "", "", false, nil, true, addErr
		}
		issues, checkErr := canonicalSet.checked(event.Field)
		return "", "", false, issues, true, checkErr
	}
	issues, checkErr := set.checked(event.Field)
	return name, body, true, issues, true, checkErr
}

func profilerFtraceEventRenderCoverage(coverageByField map[int]*TraceDBCoverage, field int) *TraceDBCoverage {
	if coverage, ok := coverageByField[field]; ok {
		return coverage
	}
	if field == 0 {
		coverage := TraceDBCoverage{
			Family:  "builtin_modern_ftrace:envelope",
			Table:   "__event_envelope__",
			Role:    "unsupported_input",
			Found:   true,
			Skipped: "malformed or ambiguous structured ftrace event envelope",
			FieldSources: map[string]string{
				"schema_profile": "TracePluginResult.ftrace_cpu_detail -> FtraceCpuDetailMsg -> exactly one FtraceEvent oneof payload",
			},
		}
		coverageByField[field] = &coverage
		return &coverage
	}
	if field == profilerFtraceCPUDetailEnvelopeField {
		coverage := TraceDBCoverage{
			Family:  "builtin_modern_ftrace:cpu_detail_envelope",
			Table:   "__cpu_detail_envelope__",
			Role:    "unsupported_input",
			Found:   true,
			Skipped: "malformed or ambiguous FtraceCpuDetailMsg envelope",
			FieldSources: map[string]string{
				"schema_profile": "FtraceCpuDetailMsg{cpu=1 singular,event=2 repeated,overwrite=3 singular}",
			},
		}
		coverageByField[field] = &coverage
		return &coverage
	}
	desc, ok := profilerFtraceEventDescriptors[field]
	coverage := TraceDBCoverage{Found: true, Role: "query_ready_export"}
	if ok {
		coverage.Family = "builtin_modern_ftrace:" + desc.Family
		coverage.Table = desc.Name
		switch field {
		case 202:
			coverage.FieldSources = map[string]string{
				"schema_profile":   "block.proto BlockBioCompleteFormat{dev=1,sector=2,nr_sector=3,error=4,rwbs=5}",
				"pairing_identity": "dev+rwbs+sector+nr_sector; error never substitutes",
				"scalar_presence":  "pinned producer set_* profile: proto3 scalar absence is exact zero; malformed/wrong-wire/duplicate fail closed",
			}
		case 204:
			coverage.FieldSources = map[string]string{
				"schema_profile":   "block.proto BlockBioQueueFormat{dev=1,sector=2,nr_sector=3,rwbs=4,comm=5}",
				"pairing_identity": "dev+rwbs+sector+nr_sector; comm never substitutes",
				"scalar_presence":  "pinned producer set_* profile: proto3 scalar absence is exact zero; malformed/wrong-wire/duplicate fail closed",
			}
		case 205:
			coverage.FieldSources = map[string]string{
				"schema_profile":   "block.proto BlockBioRemapFormat{dev=1,sector=2,nr_sector=3,old_dev=4,old_sector=5,rwbs=6}",
				"pairing_identity": "remap inventory only; no other field substitutes",
				"scalar_presence":  "pinned producer set_* profile: proto3 scalar absence is exact zero; malformed/wrong-wire/duplicate fail closed",
			}
		case 209:
			coverage.FieldSources = map[string]string{
				"schema_profile":   "block.proto BlockRqCompleteFormat{dev=1,sector=2,nr_sector=3,error=4,rwbs=5,cmd=6}",
				"pairing_identity": "dev+rwbs+sector+nr_sector; bytes/cmd/comm/error never substitute",
				"scalar_presence":  "pinned producer set_* profile: proto3 scalar absence is exact zero; malformed/wrong-wire/duplicate fail closed",
			}
		case 210, 211:
			coverage.FieldSources = map[string]string{
				"schema_profile":   "block.proto BlockRqInsert/IssueFormat{dev=1,sector=2,nr_sector=3,bytes=4,rwbs=5,comm=6,cmd=7}",
				"pairing_identity": "dev+rwbs+sector+nr_sector; bytes/cmd/comm/error never substitute",
				"scalar_presence":  "pinned producer set_* profile: proto3 scalar absence is exact zero; malformed/wrong-wire/duplicate fail closed",
			}
		case 212:
			coverage.FieldSources = map[string]string{
				"schema_profile":   "block.proto BlockRqRemapFormat{dev=1,sector=2,nr_sector=3,old_dev=4,old_sector=5,nr_bios=6,rwbs=7}",
				"pairing_identity": "remap inventory only; bytes/cmd/comm/error never substitute",
				"scalar_presence":  "pinned producer set_* profile: proto3 scalar absence is exact zero; malformed/wrong-wire/duplicate fail closed",
			}
		case 410:
			coverage.FieldSources = map[string]string{
				"schema_profile": "clk.proto ClkSetRateFormat{name=1,rate=2}",
				"cpu_id":         "not present in field-410 schema; omitted",
			}
		case 2002:
			coverage.FieldSources = map[string]string{
				"schema_profile": "power.proto ClockSetRateFormat{name=1,state=2,cpu_id=3}",
				"cpu_id":         "field3 uint64; proto3 wire absence is CPU0 under the pinned producer; valid range 0..4095",
			}
		case 2417:
			coverage.FieldSources = map[string]string{
				"schema_profile": "sched.proto SchedSwitchFormat{prev=1..4,next=5..7,next_info=8}",
				"next_info":      "field8 packed uint64; proto3 wire absence is exact zero; MaxUint64 is producer missing sentinel",
			}
		case 1000, 1001:
			coverage.FieldSources = map[string]string{
				"schema_profile": "filemap proto exposes pfn=1,i_ino=2,index=3,s_dev=4; 6.6.30 adds order=5 uint32",
				"page_pointer":   "not present in profiler schema; page token omitted",
				"order":          "field5 is audited at source range 0..255 and intentionally not displayed or projected as bytes",
			}
		case 1109:
			coverage.FieldSources = map[string]string{
				"schema_profile":    "ftrace.proto PrintFormat{ip=1,buf=2}; default tracing_mark_write alias intentionally omits ip",
				"payload_authority": "field2 buf after at-most-one official trailing-LF normalization; arbitrary safe prose remains valid",
			}
		case 4009:
			coverage.FieldSources = map[string]string{
				"schema_profile": "f2fs.proto F2fsSyncFileEnterFormat{dev=1,ino=2,pino=3,mode=4,size=5,nlink=6,blocks=7,advise=8}",
				"operation":      "exact event name; no synthetic rw field",
				"dev_t":          "field1 uint64 carrier; pinned Linux dev_t is uint32 with 12-bit major/20-bit minor; zero and overflow reject",
			}
		case 4010:
			coverage.FieldSources = map[string]string{
				"schema_profile": "f2fs.proto F2fsSyncFileExitFormat{dev=1,ino=2,cp_reason=3,datasync=4,ret=5}",
				"operation":      "exact event name; cp_reason is never projected as offset",
				"dev_t":          "field1 uint64 carrier; pinned Linux dev_t is uint32 with 12-bit major/20-bit minor; zero and overflow reject",
			}
		case 4011:
			coverage.FieldSources = map[string]string{
				"schema_profile": "f2fs.proto F2fsWriteBeginFormat{dev=1,ino=2,pos=3,len=4,flags=5}",
				"flags_presence": "field5 is set by the default parser and never set by 6.6.30; proto3 may omit exact zero, so wire absence is profile-ambiguous and omitted, never defaulted",
				"operation":      "exact event name; tracequery derives write without a synthetic rw field",
				"dev_t":          "field1 uint64 carrier; pinned Linux dev_t is uint32 with 12-bit major/20-bit minor; zero and overflow reject",
			}
		case 4012:
			coverage.FieldSources = map[string]string{
				"schema_profile": "f2fs.proto F2fsWriteEndFormat{dev=1,ino=2,pos=3,len=4,copied=5}",
				"operation":      "exact event name; tracequery derives write without a synthetic rw field",
				"dev_t":          "field1 uint64 carrier; pinned Linux dev_t is uint32 with 12-bit major/20-bit minor; zero and overflow reject",
			}
		case 4015:
			coverage.FieldSources = map[string]string{
				"schema_profile":      "mmc.proto MmcRequestDoneFormat at field 4015 (developtools_profiler 5bc8ef5)",
				"pairing_identity":    "mrq/tag/request metadata are inventory only; current coarse pairing key remains source/layer/base/emitter",
				"response_provenance": "cmd/stop/sbc carriers are wire/unique audited and never rendered; source u32[4] content above 16 bytes is field-scoped dropped/degraded because ParseStrField loses the shape",
			}
		case 4016:
			coverage.FieldSources = map[string]string{
				"schema_profile":   "mmc.proto MmcRequestStartFormat at field 4016 (developtools_profiler 5bc8ef5)",
				"pairing_identity": "mrq/tag/request metadata are inventory only; current coarse pairing key remains source/layer/base/emitter",
			}
		}
	} else {
		coverage.Family = "builtin_modern_ftrace:unknown"
		coverage.Table = fmt.Sprintf("event_field:%d", field)
		coverage.Role = "unsupported_input"
		coverage.Skipped = "unmapped structured ftrace event field"
	}
	coverageByField[field] = &coverage
	return &coverage
}

func profilerFtraceEventRenderCoverageList(coverageByField map[int]*TraceDBCoverage) []TraceDBCoverage {
	fields := make([]int, 0, len(coverageByField))
	for field := range coverageByField {
		fields = append(fields, field)
	}
	sort.Ints(fields)
	out := make([]TraceDBCoverage, 0, len(fields))
	for _, field := range fields {
		out = append(out, *coverageByField[field])
	}
	return out
}
