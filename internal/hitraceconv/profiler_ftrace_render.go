package hitraceconv

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type profilerFtraceEventRecord struct {
	CPU                int64
	TSNS               uint64
	TGID               int64
	PID                int64
	HeaderOwnerKnown   bool
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
	return renderProfilerFtraceStructuredResultWithEnvelopeCoverage(result, seq, sink, true, nil)
}

// The TraceFile container owns one cross-frame envelope diagnostic ledger.
// Direct renderer callers retain the compatibility coverage above; the
// container path suppresses only that duplicate top-level row and leaves all
// typed event coverage and pair behavior byte-for-byte shared.
func renderProfilerFtraceStructuredResultForContainer(result profilerTracePluginResult, seq *int, sink *traceDBRowSink) (int, profilerFtraceEventBatchCensus, error) {
	var batch profilerFtraceEventBatchCensus
	rows, _, err := renderProfilerFtraceStructuredResultWithEnvelopeCoverage(result, seq, sink, false, &batch)
	return rows, batch, err
}

func renderProfilerFtraceStructuredResultWithEnvelopeCoverage(result profilerTracePluginResult, seq *int, sink *traceDBRowSink, includeEnvelopeCoverage bool, batch *profilerFtraceEventBatchCensus) (int, []TraceDBCoverage, error) {
	var topLevelCoverage []TraceDBCoverage
	if includeEnvelopeCoverage {
		topLevelCoverage = profilerTracePluginResultCoverage(result)
	}
	events, err := profilerTracePluginResultEvents(result)
	if err != nil {
		return 0, topLevelCoverage, err
	}
	var coverageByField map[int]*TraceDBCoverage
	var degradationsByField map[int]map[string]int
	if batch == nil {
		coverageByField = map[int]*TraceDBCoverage{}
		degradationsByField = map[int]map[string]int{}
	}
	rows := 0
	for _, event := range events {
		if event.PairCaptureOpaque {
			sink.markPairCaptureOpaque(pairRenderMMC)
			sink.markPairCaptureOpaque(pairRenderF2FS)
		}
		mmcGoverned := event.Field == 4015 || event.Field == 4016
		mmcObserved := event.PairFamilies&pairCriticalFormatFamilyMMC != 0
		f2fsGoverned := event.Field >= 4009 && event.Field <= 4012
		f2fsObserved := event.PairFamilies&pairCriticalFormatFamilyF2FS != 0
		f2fsPair := profilerStructuredF2FSPairFamily(event.Field)
		if f2fsGoverned {
			_, _, _, f2fsPair = decodeProfilerAuxPayloadWithPairAdmission(event)
		}
		if mmcObserved && !mmcGoverned {
			// Multiple/wrong-wire/late-malformed oneofs can clear Field, but the
			// exact field number already seen remains precise family provenance.
			sink.poisonPairKind(pairRenderMMC)
		}
		if f2fsObserved && !f2fsGoverned {
			sink.poisonPairKind(pairRenderF2FS)
		}
		var coverage *TraceDBCoverage
		if batch == nil {
			coverage = profilerFtraceEventRenderCoverage(coverageByField, event.Field)
			coverage.RowsRead++
		} else if !batch.observeRead(event.Field) {
			return rows, topLevelCoverage, &traceDBOutputInvariantError{Reason: "profiler_event_batch_counter_overflow"}
		}
		var name, body string
		var ok bool
		var issues []profilerFtraceEventIssue
		var auditErr error
		name, body, ok, issues, auditErr = renderProfilerFtraceEventBodyWithTypedAudit(event)
		if auditErr != nil {
			return rows, topLevelCoverage, auditErr
		}
		if !profilerFtraceEventIssueVerdictValid(event.Field, ok, issues) {
			return rows, topLevelCoverage, &traceDBOutputInvariantError{Reason: "profiler_event_issue_verdict_invalid"}
		}
		if batch == nil {
			labels, labelsOK := profilerFtraceEventIssueLabels(event.Field, issues)
			if !labelsOK {
				return rows, topLevelCoverage, &traceDBOutputInvariantError{Reason: "profiler_event_issue_label_invalid"}
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
				return rows, topLevelCoverage, &traceDBOutputInvariantError{Reason: "profiler_event_issue_census_overflow"}
			}
		}
		if !ok {
			if mmcGoverned || mmcObserved {
				sink.poisonPairKind(pairRenderMMC)
			}
			if f2fsGoverned {
				f2fsPair.poison(sink)
			} else if f2fsObserved {
				sink.poisonPairKind(pairRenderF2FS)
			}
			if coverage != nil && coverage.Skipped == "" {
				coverage.Skipped = "structured ftrace renderer pending"
			}
			continue
		}
		task := firstNonEmpty(event.Comm, "unknown")
		row, err := prepareTraceDBRenderedRowWithTraceFlags(int64(event.TSNS), *seq, task, event.PID,
			event.TGID, event.CPU, event.CommonFlags, event.CommonPreemptCount, name+": "+body)
		if err != nil {
			return rows, append(topLevelCoverage, profilerFtraceEventRenderCoverageList(coverageByField)...), err
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
			row.pairLane = f2fsPair.Lane
			row.pairTable = name
			row.structuredPair = true
			row.profilerEventField = event.Field
			pair := profilerTextPairAdmission(row.line)
			if !f2fsPair.LaneKnown || !pair.Governed || pair.Kind != pairRenderF2FS ||
				!pair.Admitted || !pair.LaneKnown || pair.Lane != f2fsPair.Lane {
				sink.poisonPairKind(pairRenderF2FS)
			}
		}
		if err := sink.add(row); err != nil {
			return rows, append(topLevelCoverage, profilerFtraceEventRenderCoverageList(coverageByField)...), err
		}
		(*seq)++
		rows++
		if coverage != nil {
			coverage.RowsEmitted++
		} else if !batch.observeEmitted(event.Field) {
			return rows, topLevelCoverage, &traceDBOutputInvariantError{Reason: "profiler_event_batch_emitted_counter_overflow"}
		}
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
	return rows, append(topLevelCoverage, profilerFtraceEventRenderCoverageList(coverageByField)...), nil
}

func decodeProfilerFtraceStructuredEvents(data []byte) ([]profilerFtraceEventRecord, error) {
	return profilerTracePluginResultEvents(decodeProfilerTracePluginResult(data))
}

func decodeProfilerFtraceCPUDetailEvents(data []byte) ([]profilerFtraceEventRecord, error) {
	var cpu uint64
	cpuCount := 0
	cpuWrongWire := false
	var eventPayloads [][]byte
	eventWrongWire := 0
	overwriteCount := 0
	overwriteWrongWire := false
	var out []profilerFtraceEventRecord
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			cpuCount++
			if wire != 0 {
				cpuWrongWire = true
				break
			}
			cpu = v
		case 2:
			if wire != 2 {
				eventWrongWire++
				break
			}
			eventPayloads = append(eventPayloads, raw)
		case 3:
			overwriteCount++
			if wire != 0 {
				overwriteWrongWire = true
			}
		}
		return nil
	})
	if err != nil {
		record := profilerFtraceEventRecord{
			Field:             profilerFtraceCPUDetailEnvelopeField,
			PairFamilies:      profilerPairFamiliesFromCPUDetail(data),
			PairCaptureOpaque: true,
		}
		if issueErr := record.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeCPUDetailMalformedWire); issueErr != nil {
			return nil, issueErr
		}
		return []profilerFtraceEventRecord{record}, nil
	}
	var cpuIssueKind profilerFtraceEventIssueKind
	cpuIssuePresent := true
	switch {
	case cpuCount > 1:
		cpuIssueKind = profilerFtraceEventIssueEnvelopeCPUDuplicate
	case cpuWrongWire:
		cpuIssueKind = profilerFtraceEventIssueEnvelopeCPUWrongWire
	case cpu > uint64(maxTraceDBCPUIndex):
		cpuIssueKind = profilerFtraceEventIssueEnvelopeCPUOutOfRange
	default:
		cpuIssuePresent = false
	}
	// FtraceParser and FlowController always call set_cpu. Proto3 omits an
	// exact zero, so absence is CPU 0 only for this pinned producer profile.
	for _, raw := range eventPayloads {
		event, decodeErr := decodeProfilerFtraceEventRecord(cpu, raw)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if cpuIssuePresent {
			if issueErr := event.appendEnvelopeIssue(cpuIssueKind); issueErr != nil {
				return nil, issueErr
			}
		}
		out = append(out, event)
	}
	if len(eventPayloads) == 0 && cpuIssuePresent {
		record := profilerFtraceEventRecord{Field: profilerFtraceCPUDetailEnvelopeField}
		if issueErr := record.appendEnvelopeIssue(cpuIssueKind); issueErr != nil {
			return nil, issueErr
		}
		out = append(out, record)
	}
	if eventWrongWire > 0 {
		record := profilerFtraceEventRecord{
			Field:             profilerFtraceCPUDetailEnvelopeField,
			PairCaptureOpaque: true,
		}
		if issueErr := record.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeEventContainerWrongWire); issueErr != nil {
			return nil, issueErr
		}
		out = append(out, record)
	}
	if overwriteCount > 1 || overwriteWrongWire {
		record := profilerFtraceEventRecord{Field: profilerFtraceCPUDetailEnvelopeField}
		if issueErr := record.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeOverwriteInvalid); issueErr != nil {
			return nil, issueErr
		}
		out = append(out, record)
	}
	return out, nil
}

func profilerPairFamiliesFromCPUDetail(data []byte) pairCriticalFormatFamilyMask {
	var families pairCriticalFormatFamilyMask
	for len(data) > 0 {
		key, consumed, ok := consumeProtoVarint(data)
		if !ok {
			break
		}
		data = data[consumed:]
		fieldNumber := key >> 3
		if fieldNumber < 1 || fieldNumber > (1<<29)-1 {
			return families
		}
		field, wire := int(fieldNumber), int(key&0x7)
		switch wire {
		case 0:
			_, consumed, ok = consumeProtoVarint(data)
			if !ok {
				return families
			}
			data = data[consumed:]
		case 1:
			if len(data) < 8 {
				return families
			}
			data = data[8:]
		case 2:
			length, lengthBytes, valid := consumeProtoVarint(data)
			if !valid {
				return families
			}
			data = data[lengthBytes:]
			available := uint64(len(data))
			payloadLength := length
			if payloadLength > available {
				payloadLength = available
			}
			if field == 2 {
				families |= profilerPairFamiliesFromEventPayload(data[:int(payloadLength)])
			}
			if length > available {
				return families
			}
			data = data[int(length):]
		case 5:
			if len(data) < 4 {
				return families
			}
			data = data[4:]
		default:
			return families
		}
	}
	return families
}

func profilerPairFamiliesFromEventPayload(data []byte) pairCriticalFormatFamilyMask {
	var families pairCriticalFormatFamilyMask
	for len(data) > 0 {
		key, consumed, ok := consumeProtoVarint(data)
		if !ok {
			break
		}
		data = data[consumed:]
		fieldNumber := key >> 3
		if fieldNumber < 1 || fieldNumber > (1<<29)-1 {
			return families
		}
		field, wire := int(fieldNumber), int(key&0x7)
		families |= profilerPairFamilyForField(field)
		switch wire {
		case 0:
			_, consumed, ok = consumeProtoVarint(data)
			if !ok {
				return families
			}
			data = data[consumed:]
		case 1:
			if len(data) < 8 {
				return families
			}
			data = data[8:]
		case 2:
			length, lengthBytes, valid := consumeProtoVarint(data)
			if !valid {
				return families
			}
			data = data[lengthBytes:]
			if length > uint64(len(data)) {
				return families
			}
			data = data[int(length):]
		case 5:
			if len(data) < 4 {
				return families
			}
			data = data[4:]
		default:
			return families
		}
	}
	return families
}

type profilerProtoEnvelopeField struct {
	count     int
	wrongWire bool
	uintValue uint64
	bytes     []byte
}

func decodeProfilerFtraceEventRecord(cpu uint64, data []byte) (profilerFtraceEventRecord, error) {
	record := profilerFtraceEventRecord{CPU: int64(cpu)}
	var timestamp, tgid, comm, common profilerProtoEnvelopeField
	oneofCount := 0
	oneofWrongWire := false
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
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
		pid, ownerKnown, flags, preempt, commonErr := decodeProfilerFtraceCommonFields(&record, common.bytes)
		if commonErr != nil {
			return profilerFtraceEventRecord{}, commonErr
		}
		record.PID = pid
		record.HeaderOwnerKnown = ownerKnown
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
	switch field {
	case 4009, 4010, 4011, 4012:
		return pairCriticalFormatFamilyF2FS
	case 4015, 4016:
		return pairCriticalFormatFamilyMMC
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

func decodeProfilerFtraceCommonFields(record *profilerFtraceEventRecord, data []byte) (pid int64, ownerKnown bool,
	flags, preempt int64, err error) {
	if record == nil {
		return 0, false, 0, 0, &traceDBOutputInvariantError{Reason: "profiler_event_common_record_nil"}
	}
	var fields [5]profilerProtoEnvelopeField
	walkErr := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
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
		if !appendIssue(profilerFtraceEventIssueEnvelopeCommonFieldsMalformedWire) {
			return 0, false, 0, 0, &traceDBOutputInvariantError{Reason: "profiler_event_common_issue_schema_invalid"}
		}
		return 0, false, 0, 0, nil
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
			return 0, false, 0, 0, &traceDBOutputInvariantError{Reason: "profiler_event_common_issue_schema_invalid"}
		}
	}
	if fields[1].count == 1 && !fields[1].wrongWire && fields[1].uintValue > math.MaxUint16 {
		if !appendIssue(profilerFtraceEventIssueEnvelopeCommonTypeSourceWidth) {
			return 0, false, 0, 0, &traceDBOutputInvariantError{Reason: "profiler_event_common_issue_schema_invalid"}
		}
	}
	if fields[2].count == 1 && !fields[2].wrongWire && fields[2].uintValue > math.MaxUint8 {
		if !appendIssue(profilerFtraceEventIssueEnvelopeCommonFlagsSourceWidth) {
			return 0, false, 0, 0, &traceDBOutputInvariantError{Reason: "profiler_event_common_issue_schema_invalid"}
		}
	}
	if fields[3].count == 1 && !fields[3].wrongWire && fields[3].uintValue > math.MaxUint8 {
		if !appendIssue(profilerFtraceEventIssueEnvelopeCommonPreemptCountSourceWidth) {
			return 0, false, 0, 0, &traceDBOutputInvariantError{Reason: "profiler_event_common_issue_schema_invalid"}
		}
	}
	if fields[4].count == 1 && !fields[4].wrongWire && fields[4].uintValue > math.MaxInt32 {
		if !appendIssue(profilerFtraceEventIssueEnvelopeCommonPIDOutOfRange) {
			return 0, false, 0, 0, &traceDBOutputInvariantError{Reason: "profiler_event_common_issue_schema_invalid"}
		}
	}
	ownerKnown = fields[4].count <= 1 && !fields[4].wrongWire && fields[4].uintValue <= math.MaxInt32
	return int64(fields[4].uintValue), ownerKnown, int64(fields[2].uintValue), int64(fields[3].uintValue), nil
}

func renderProfilerFtraceEventBody(event profilerFtraceEventRecord) (string, string, bool) {
	switch event.Field {
	case 202, 204, 205, 209, 210, 211, 212:
		name, body, ok, _ := renderProfilerBlockEvent(event)
		return name, body, ok
	case 410:
		// Field 410 is clk.proto ClkSetRateFormat{name, rate}; unlike the
		// power event at field 2002 it has no cpu_id. Keep the established
		// clock_set_rate text alias, but never synthesize a CPU dimension.
		return "clock_set_rate", fmt.Sprintf("%s state=%d", protoString(event.Payload, 1), protoUint(event.Payload, 2)), true
	case 2002:
		parts := []string{protoString(event.Payload, 1), fmt.Sprintf("state=%d", protoUint(event.Payload, 2))}
		cpuID, state, _ := protoScalarUint(event.Payload, 3)
		switch state {
		case protoScalarAbsent:
			// FtraceEventProcessor always sets ClockSetRateFormat.cpu_id.
			// Proto3 omits an exact zero scalar from the wire, so absence in
			// this pinned producer profile is authoritative CPU 0.
			parts = appendClockSetRateCPU(parts, 0)
		case protoScalarPresent:
			parts = appendClockSetRateCPU(parts, cpuID)
		}
		return "clock_set_rate", strings.Join(parts, " "), true
	case 2417:
		body := fmt.Sprintf("prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d",
			protoString(event.Payload, 1), protoInt(event.Payload, 2), protoInt(event.Payload, 3),
			linuxPrevState(protoUint(event.Payload, 4)), protoString(event.Payload, 5), protoInt(event.Payload, 6), protoInt(event.Payload, 7))
		nextInfo, state, _ := protoScalarUint(event.Payload, 8)
		// The pinned producer writes MaxUint64 when the source format has no
		// next_info. Proto3 omits an exact packed zero, so wire absence is the
		// authoritative zero tuple only for this exact field-2417 profile.
		if state == protoScalarAbsent {
			nextInfo = 0
			state = protoScalarPresent
		}
		if state == protoScalarPresent && nextInfo != ^uint64(0) {
			body += " next_info=" + formatHarmonySchedInfo(nextInfo, true)
		}
		return "sched_switch", body, true
	default:
		return "", "", false
	}
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

func protoUint(data []byte, field int) uint64 {
	var out uint64
	_ = walkProtoFields(data, func(f int, wire int, raw []byte, v uint64) error {
		if f == field && wire == 0 {
			out = v
		}
		_ = raw
		return nil
	})
	return out
}

func protoInt(data []byte, field int) int64 {
	return int64(protoUint(data, field))
}

func protoString(data []byte, field int) string {
	var out string
	_ = walkProtoFields(data, func(f int, wire int, raw []byte, v uint64) error {
		if f == field && wire == 2 {
			out = string(raw)
		}
		_ = v
		return nil
	})
	return out
}

func renderProfilerFtraceEventBodyWithAudit(event profilerFtraceEventRecord) (string, string, bool, []string) {
	envelopeIssues, envelopeErr := event.checkedEnvelopeIssues()
	if envelopeErr != nil {
		return "", "", false, nil
	}
	if len(envelopeIssues) > 0 {
		labels, valid := profilerFtraceEventIssueLabels(event.Field, envelopeIssues)
		if !valid {
			return "", "", false, nil
		}
		return "", "", false, labels
	}
	corePayload, admission, reason, degradations := decodeProfilerCorePayload(event)
	switch admission {
	case bodyAdmitted:
		body, ok := renderCanonicalCorePayload(corePayload)
		if !ok {
			return "", "", false, []string{"invalid_canonical_core_payload"}
		}
		if !profilerCanonicalLineValid(event, corePayload.Name, body) {
			return "", "", false, []string{"invalid_canonical_core_line"}
		}
		return corePayload.Name, body, true, degradations
	case bodyRejected:
		return "", "", false, []string{reason}
	}
	auxPayload, admission, reason := decodeProfilerAuxPayload(event)
	switch admission {
	case bodyAdmitted:
		body, ok := renderCanonicalProfilerAuxPayload(auxPayload)
		if !ok {
			return "", "", false, []string{"invalid_canonical_aux_payload"}
		}
		if !profilerCanonicalLineValid(event, auxPayload.Name, body) {
			return "", "", false, []string{"invalid_canonical_aux_line"}
		}
		return auxPayload.Name, body, true, append([]string(nil), auxPayload.Degradations...)
	case bodyRejected:
		return "", "", false, []string{reason}
	}
	filemapPayload, admission, reason := decodeProfilerFilemapPayload(event)
	switch admission {
	case bodyAdmitted:
		body, ok := renderCanonicalFilemapPayload(filemapPayload)
		if !ok {
			return "", "", false, []string{"invalid_canonical_filemap_payload"}
		}
		if !profilerCanonicalLineValid(event, filemapPayload.Name, body) {
			return "", "", false, []string{"invalid_canonical_filemap_line"}
		}
		return filemapPayload.Name, body, true, nil
	case bodyRejected:
		return "", "", false, []string{reason}
	}
	if _, _, blockEvent := blockRenderKindForProfilerField(event.Field); blockEvent {
		return renderProfilerBlockEvent(event)
	}
	coreOK, degradations := profilerFtraceCoreWireAudit(event)
	if !coreOK {
		return "", "", false, degradations
	}
	name, body, ok := renderProfilerFtraceEventBody(event)
	if !ok {
		return name, body, ok, nil
	}
	switch event.Field {
	case 2002:
		cpuID, state, reason := protoScalarUint(event.Payload, 3)
		if state == protoScalarInvalid {
			degradations = append(degradations, "cpu_id_"+reason)
		} else if state == protoScalarPresent && cpuID > uint64(maxTraceDBCPUIndex) {
			degradations = append(degradations, "cpu_id_out_of_range")
		}
	case 2417:
		_, state, reason := protoScalarUint(event.Payload, 8)
		if state == protoScalarInvalid {
			degradations = append(degradations, "next_info_"+reason)
		}
	}
	return name, body, ok, degradations
}

// B2-b migrates producer families one at a time. Envelope issues already enter
// through the exact typed arm below; the strict legacy bridge remains only for
// producer families whose typed migration is not complete yet.
func renderProfilerFtraceEventBodyWithTypedAudit(event profilerFtraceEventRecord) (string, string, bool, []profilerFtraceEventIssue, error) {
	envelopeIssues, envelopeErr := event.checkedEnvelopeIssues()
	if envelopeErr != nil {
		return "", "", false, nil, envelopeErr
	}
	if len(envelopeIssues) > 0 {
		issues := append([]profilerFtraceEventIssue(nil), envelopeIssues...)
		if profilerFtraceEventSlot(event.Field) == profilerFtraceUnknownEventSlot {
			if event.Field < 100 || event.Field > profilerFtraceUnknownEventAggregateField {
				return "", "", false, nil, &traceDBOutputInvariantError{Reason: "profiler_event_field_domain_invalid"}
			}
			unmapped, valid := profilerFtraceEventFixedIssue(event.Field, profilerFtraceEventIssueUnmappedField)
			if !valid {
				return "", "", false, nil, &traceDBOutputInvariantError{Reason: "profiler_event_unmapped_issue_invalid"}
			}
			issues = append(issues, unmapped)
		}
		return "", "", false, issues, nil
	}
	name, body, ok, reasons := renderProfilerFtraceEventBodyWithAudit(event)
	legacySource := profilerFtraceEventDegradationFieldAudit
	switch {
	case profilerStructuredCoreSchemas[event.Field] != nil:
		legacySource = profilerFtraceEventDegradationCorePayload
	case profilerStructuredAuxSchemas[event.Field] != nil:
		legacySource = profilerFtraceEventDegradationAuxPayload
	case event.Field == 1000 || event.Field == 1001:
		legacySource = profilerFtraceEventDegradationFilemapPayload
	default:
		if _, _, block := blockRenderKindForProfilerField(event.Field); block {
			legacySource = profilerFtraceEventDegradationBlockPayload
		} else if _, known := profilerFtraceEventDescriptors[event.Field]; known {
			legacySource = profilerFtraceEventDegradationWireAudit
		} else {
			legacySource = profilerFtraceEventDegradationUnmappedField
		}
	}
	if profilerFtraceEventSlot(event.Field) == profilerFtraceUnknownEventSlot {
		if event.Field < 100 || event.Field > profilerFtraceUnknownEventAggregateField {
			return "", "", false, nil, &traceDBOutputInvariantError{Reason: "profiler_event_field_domain_invalid"}
		}
		// Preserve exact envelope issues: their typed Kind+PayloadField does not
		// depend on the compacted unknown field ID. Unmapped identity is appended
		// below as the mandatory dominant hard issue for the unknown slot.
		name, body, ok = "", "", false
	}
	if !ok && len(reasons) == 0 {
		if profilerFtraceEventSlot(event.Field) == profilerFtraceUnknownEventSlot {
			// The mandatory unmapped issue is appended after any envelope issues.
		} else if legacySource == profilerFtraceEventDegradationUnmappedField {
			reasons = []string{"unmapped structured ftrace event field"}
		} else {
			return "", "", false, nil, &traceDBOutputInvariantError{Reason: "profiler_event_missing_typed_issue"}
		}
	}
	issues := make([]profilerFtraceEventIssue, 0, len(reasons))
	for _, reason := range reasons {
		issue, valid := profilerFtraceEventIssueFromLegacy(event.Field, legacySource, reason)
		if !valid {
			return "", "", false, nil, &traceDBOutputInvariantError{Reason: "profiler_event_legacy_issue_unmapped"}
		}
		issues = append(issues, issue)
	}
	if profilerFtraceEventSlot(event.Field) == profilerFtraceUnknownEventSlot {
		unmapped, valid := profilerFtraceEventIssueFromLegacy(event.Field,
			profilerFtraceEventDegradationUnmappedField, "unmapped structured ftrace event field")
		if !valid {
			return "", "", false, nil, &traceDBOutputInvariantError{Reason: "profiler_event_unmapped_issue_invalid"}
		}
		issues = append(issues, unmapped)
	}
	return name, body, ok, issues, nil
}

func profilerFtraceCoreWireAudit(event profilerFtraceEventRecord) (bool, []string) {
	var stringFields []int
	var scalarFields []int
	switch event.Field {
	case 410, 2002:
		stringFields = []int{1}
		scalarFields = []int{2}
	case 2417:
		stringFields = []int{1, 5}
		scalarFields = []int{2, 3, 4, 6, 7}
	default:
		return true, nil
	}
	var reasons []string
	for _, field := range stringFields {
		value, state, reason := protoScalarString(event.Payload, field)
		validValue := state == protoScalarPresent && traceDBSinglePhysicalLine(value, false)
		if validValue && (event.Field == 410 || event.Field == 2002) && field == 1 {
			validValue = traceDBSingleToken(value)
		}
		if state == protoScalarAbsent || (state == protoScalarPresent && !validValue) {
			reasons = append(reasons, fmt.Sprintf("core_field%d_missing_or_invalid", field))
			continue
		}
		if state == protoScalarInvalid {
			reasons = append(reasons, fmt.Sprintf("core_field%d_%s", field, reason))
		}
	}
	for _, field := range scalarFields {
		_, state, reason := protoScalarUint(event.Payload, field)
		// Proto3 scalar absence is the exact default zero under these pinned
		// generated producer profiles; only malformed/ambiguous wire is a
		// missing core authority.
		if state == protoScalarInvalid {
			reasons = append(reasons, fmt.Sprintf("core_field%d_%s", field, reason))
		}
	}
	if event.Field == 2417 && len(reasons) == 0 {
		for _, field := range []int{2, 6} {
			value, state, _ := protoScalarUint(event.Payload, field)
			if state == protoScalarPresent && value > math.MaxInt32 {
				reasons = append(reasons, fmt.Sprintf("core_field%d_out_of_range", field))
			}
		}
		for _, field := range []int{3, 7} {
			value, state, _ := protoScalarUint(event.Payload, field)
			if state == protoScalarPresent {
				signed := int64(value)
				if signed < math.MinInt32 || signed > math.MaxInt32 {
					reasons = append(reasons, fmt.Sprintf("core_field%d_out_of_range", field))
				}
			}
		}
	}
	return len(reasons) == 0, reasons
}

type protoScalarState uint8

const (
	protoScalarAbsent protoScalarState = iota
	protoScalarPresent
	protoScalarInvalid
)

// protoScalarUint reads one singular proto scalar without collapsing three
// distinct states: proto3 default omission, a present value (including an
// explicitly encoded zero), and malformed/ambiguous input. Callers interpret
// the default only when their pinned producer profile proves it.
func protoScalarUint(data []byte, field int) (uint64, protoScalarState, string) {
	var value uint64
	count := 0
	wrongWire := false
	err := walkProtoFields(data, func(f int, wire int, raw []byte, v uint64) error {
		if f != field {
			return nil
		}
		count++
		if wire != 0 {
			wrongWire = true
			return nil
		}
		value = v
		_ = raw
		return nil
	})
	if err != nil {
		return 0, protoScalarInvalid, "malformed_wire"
	}
	if wrongWire {
		return 0, protoScalarInvalid, "wrong_wire"
	}
	if count > 1 {
		return 0, protoScalarInvalid, "duplicate"
	}
	if count == 0 {
		return 0, protoScalarAbsent, ""
	}
	return value, protoScalarPresent, ""
}

func protoScalarString(data []byte, field int) (string, protoScalarState, string) {
	var value string
	count := 0
	wrongWire := false
	err := walkProtoFields(data, func(f int, wire int, raw []byte, v uint64) error {
		if f != field {
			return nil
		}
		count++
		if wire != 2 {
			wrongWire = true
			return nil
		}
		value = string(raw)
		_ = v
		return nil
	})
	if err != nil {
		return "", protoScalarInvalid, "malformed_wire"
	}
	if wrongWire {
		return "", protoScalarInvalid, "wrong_wire"
	}
	if count > 1 {
		return "", protoScalarInvalid, "duplicate"
	}
	if count == 0 {
		return "", protoScalarAbsent, ""
	}
	return value, protoScalarPresent, ""
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
