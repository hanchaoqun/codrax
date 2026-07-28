package hitraceconv

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	maxTraceDBRawDecodeTargetRows      = 250000
	maxTraceDBRawDecodeFormatWitnesses = 64
	maxTraceDBRawDecodeFieldsPerFormat = 32
)

type traceDBRawDecodeFormatStats struct {
	ID      int
	Name    string
	Records int64
}

type traceDBRawBlockedRecord struct {
	TimestampNS      uint64
	CPU              int
	HeaderPID        int64
	Flags            int64
	PreemptCount     int64
	TargetTID        int64
	IOWait           int64
	CallerRaw        uint64
	Caller           string
	CallerSymbolized bool
	CNodeIndex       uint64
	CNodeKnown       bool
	Delay            uint64
	DelayKnown       bool
}

type traceDBSourceRawDecodeAccumulator struct {
	coverage          TraceDBCoverage
	formats           map[int]*traceDBRawDecodeFormatStats
	blockedRecords    []traceDBRawBlockedRecord
	switchLiteRecords []traceDBRawSchedSwitchLiteRecord
	wakeupLiteRecords []traceDBRawSchedWakeupLiteRecord
	targetRows        int64
	targetDecoded     int64
	targetFirstTS     uint64
	targetLastTS      uint64
	targetTimestamp   bool
	decodeCapped      bool
}

func newTraceDBSourceRawDecodeCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_decode",
		Table:  "__raw_record_decode__",
		Role:   "diagnostic_ledger",
		FieldSources: map[string]string{
			"authority":      "same immutable official input generation, admitted event-format catalog, and structurally validated page/record geometry as source_rawtrace_profile",
			"body_decode":    "closed strict decoders only: sched core, exact sched_switch, exact sched_switch_lite/sched_wakeup_lite, tracing marker, and DMA wait endpoints; generic/legacy fallback renderers never gain RPD authority",
			"geometry":       "bounded exact descriptor field name/offset/size/signed witnesses for closed target formats; field types and print-fmt text are not surfaced",
			"effect":         "bounded independent raw-record accounting only; RowsEmitted is always zero and no decoded record is published or merged with trace_streamer output",
			"identity":       "strict common_pid/common_flags/common_preempt_count envelope is required per decoded target record; namespace and TGID are not inferred",
			"scheduler_lite": "sched_switch_lite retains exact prev/next PID, signed-16 priority, state and the full packed uint64 next_info; known bits render the stable current prefix while nonzero unknown high bits are counted and never guessed as future fields; sched_wakeup_lite retains exact target PID, signed-16 priority and target CPU; both remain internal until a separate DB join proves one-to-one publication authority",
			"limits":         "at most 250000 target records receive body decoding, at most 64 sorted format/count witnesses, and at most 32 fields per target geometry witness are surfaced; record/format caps withdraw completion while field overflow is explicitly counted",
		},
		Metadata: map[string]string{
			"decode_state":          "unavailable",
			"publication_authority": "withheld_rpd1_diagnostic_only",
		},
	}
}

func newTraceDBSourceRawDecodeAccumulator() traceDBSourceRawDecodeAccumulator {
	return traceDBSourceRawDecodeAccumulator{
		coverage: newTraceDBSourceRawDecodeCoverage(),
		formats:  map[int]*traceDBRawDecodeFormatStats{},
	}
}

func (a *traceDBSourceRawDecodeAccumulator) setUnavailable(state, reason string, found bool) {
	if a == nil {
		return
	}
	a.coverage.Found = found
	a.coverage.Metadata["decode_state"] = state
	if reason != "" {
		a.coverage.Skipped = reason
	}
}

func (a *traceDBSourceRawDecodeAccumulator) observeUnknownRecord() {
	if a == nil {
		return
	}
	a.coverage.RowsRead++
	traceDBAddCoverageMetric(&a.coverage, "records_without_admitted_format", 1)
}

func (a *traceDBSourceRawDecodeAccumulator) observeRecord(
	format eventFormat,
	content []byte,
	cpu int,
	timestampNS uint64,
) {
	if a == nil {
		return
	}
	a.coverage.RowsRead++
	traceDBAddCoverageMetric(&a.coverage, "records_with_admitted_format", 1)
	stats := a.formats[format.ID]
	if stats == nil {
		stats = &traceDBRawDecodeFormatStats{ID: format.ID, Name: format.Name}
		a.formats[format.ID] = stats
	}
	stats.Records++
	if !traceDBRawProbeTargetFormat(format.Name) {
		return
	}

	a.targetRows++
	metric := traceDBRawDecodeMetricName(format.Name)
	traceDBAddCoverageMetric(&a.coverage, "target_records", 1)
	traceDBAddCoverageMetric(&a.coverage, "target_"+metric+"_records", 1)
	if a.targetDecoded >= maxTraceDBRawDecodeTargetRows {
		a.decodeCapped = true
		return
	}
	a.targetDecoded++
	traceDBAddCoverageMetric(&a.coverage, "target_decode_rows", 1)
	if !a.targetTimestamp || timestampNS < a.targetFirstTS {
		a.targetFirstTS = timestampNS
	}
	if !a.targetTimestamp || timestampNS > a.targetLastTS {
		a.targetLastTS = timestampNS
	}
	a.targetTimestamp = true

	event := decodeEvent(format, content)
	headerPID, flags, preemptCount, envelopeOK := decodeDirectFtraceCommonEnvelope(event)
	if !envelopeOK {
		traceDBAddCoverageMetric(&a.coverage, "target_envelope_rejected", 1)
		traceDBAddCoverageMetric(&a.coverage, "target_"+metric+"_envelope_rejected", 1)
		return
	}
	traceDBAddCoverageMetric(&a.coverage, "target_envelope_admitted", 1)
	traceDBAddCoverageMetric(&a.coverage, "target_"+metric+"_envelope_admitted", 1)

	if !traceDBRawDecodeStrictTarget(format.Name) {
		traceDBAddCoverageMetric(&a.coverage, "target_body_unsupported", 1)
		traceDBAddCoverageMetric(&a.coverage, "target_"+metric+"_body_unsupported", 1)
		return
	}
	body, admission, reason := traceDBRawDecodeTargetBody(event, content, cpu)
	if admission == bodyAdmitted && (body == "" || !traceDBSinglePhysicalLine(body, false)) {
		admission = bodyRejected
		reason = "invalid_strict_body_line"
	}
	switch admission {
	case bodyAdmitted:
		traceDBAddCoverageMetric(&a.coverage, "target_body_admitted", 1)
		traceDBAddCoverageMetric(&a.coverage, "target_"+metric+"_body_admitted", 1)
		if format.Name == "sched_blocked_reason" {
			blocked, blockedReason := decodeDirectBlockedPayload(event, content)
			if blockedReason != "" {
				traceDBAddCoverageMetric(&a.coverage, "target_sched_blocked_reason_key_capture_failed", 1)
				return
			}
			a.blockedRecords = append(a.blockedRecords, traceDBRawBlockedRecord{
				TimestampNS: timestampNS, CPU: cpu, HeaderPID: int64(headerPID),
				Flags: flags, PreemptCount: preemptCount, TargetTID: blocked.PID,
				IOWait: int64(blocked.IOWait), CallerRaw: blocked.CallerRaw,
				Caller: blocked.Caller, CallerSymbolized: blocked.CallerSymbolized,
				CNodeIndex: blocked.CNodeIndex, CNodeKnown: blocked.CNodeKnown,
				Delay: blocked.Delay, DelayKnown: blocked.DelayKnown,
			})
		} else if format.Name == "sched_switch_lite" {
			lite, liteReason := decodeTraceDBRawSchedSwitchLite(event)
			if liteReason != "" {
				traceDBAddCoverageMetric(&a.coverage, "target_sched_switch_lite_record_capture_failed", 1)
				return
			}
			lite.TimestampNS, lite.CPU = timestampNS, cpu
			lite.HeaderPID, lite.Flags, lite.PreemptCount = int64(headerPID), flags, preemptCount
			if traceDBRawSchedSwitchLiteNextInfoUnknownTail(lite) {
				traceDBAddCoverageMetric(&a.coverage,
					"target_sched_switch_lite_next_info_unknown_tail_bits", 1)
			}
			a.switchLiteRecords = append(a.switchLiteRecords, lite)
		} else if format.Name == "sched_wakeup_lite" {
			lite, liteReason := decodeTraceDBRawSchedWakeupLite(event)
			if liteReason != "" {
				traceDBAddCoverageMetric(&a.coverage, "target_sched_wakeup_lite_record_capture_failed", 1)
				return
			}
			lite.TimestampNS, lite.CPU = timestampNS, cpu
			lite.HeaderPID, lite.Flags, lite.PreemptCount = int64(headerPID), flags, preemptCount
			a.wakeupLiteRecords = append(a.wakeupLiteRecords, lite)
		}
	case bodyRejected:
		traceDBAddCoverageMetric(&a.coverage, "target_body_rejected", 1)
		traceDBAddCoverageMetric(&a.coverage, "target_"+metric+"_body_rejected", 1)
		if reason == "" {
			reason = "unspecified"
		}
		traceDBAddCoverageMetric(&a.coverage,
			"target_"+metric+"_reject_"+traceDBRawDecodeReasonMetric(reason), 1)
	default:
		traceDBAddCoverageMetric(&a.coverage, "target_body_unsupported", 1)
		traceDBAddCoverageMetric(&a.coverage, "target_"+metric+"_body_unsupported", 1)
	}
}

func traceDBRawDecodeTargetBody(
	event decodedEvent,
	content []byte,
	cpu int,
) (string, bodyAdmission, string) {
	switch event.format.Name {
	case "sched_switch_lite":
		row, reason := decodeTraceDBRawSchedSwitchLite(event)
		if reason != "" {
			return "", bodyRejected, reason
		}
		return traceDBRawSchedSwitchLiteDiagnosticBody(row), bodyAdmitted, ""
	case "sched_wakeup_lite":
		row, reason := decodeTraceDBRawSchedWakeupLite(event)
		if reason != "" {
			return "", bodyRejected, reason
		}
		return traceDBRawSchedWakeupLiteDiagnosticBody(row), bodyAdmitted, ""
	default:
		return renderEventBodyDecision(coreDecodeContext{}, event, content, cpu)
	}
}

func (a *traceDBSourceRawDecodeAccumulator) finalize(
	catalog eventFormatCatalog,
	profile TraceDBCoverage,
) TraceDBCoverage {
	if a == nil {
		return newTraceDBSourceRawDecodeCoverage()
	}
	a.coverage.Found = profile.Found
	if !profile.Found {
		a.setUnavailable("not_applicable_non_official_profile",
			"official raw record decode ledger not applicable to this envelope", false)
		return a.coverage
	}
	if profile.Metadata["probe_state"] != "complete" ||
		profile.Metadata["page_layout_state"] != "qword_length_cpu_candidate_all_pages" ||
		profile.Metadata["event_format_probe_state"] != "parsed_strict" {
		a.setUnavailable("withheld_profile_not_ready",
			"raw record decode ledger withheld: strict page/format profile is not complete", true)
		return a.coverage
	}
	if profile.Metrics["event_format_ids_poisoned"] > 0 ||
		a.coverage.Metrics["records_without_admitted_format"] > 0 ||
		int64(a.coverage.RowsRead) != profile.Metrics["records_structurally_scanned"] {
		a.setUnavailable("withheld_record_format_accounting_incomplete",
			"raw record decode ledger withheld: record/format accounting is not exact", true)
		return a.coverage
	}

	a.coverage.Metadata["target_formats_absent"] =
		strings.Join(traceDBRawDecodeAbsentTargets(catalog), ",")
	if a.targetTimestamp {
		a.coverage.Metadata["target_first_timestamp_ns"] = strconv.FormatUint(a.targetFirstTS, 10)
		a.coverage.Metadata["target_last_timestamp_ns"] = strconv.FormatUint(a.targetLastTS, 10)
	}
	witnesses, omitted := traceDBRawDecodeFormatWitnesses(catalog, a.formats)
	if len(witnesses) > 0 {
		a.coverage.Metadata["format_record_witnesses"] = strings.Join(witnesses, ",")
	}
	geometry, fieldOmitted := traceDBRawDecodeTargetGeometryWitnesses(catalog)
	if len(geometry) > 0 {
		a.coverage.Metadata["target_format_geometry_witnesses"] = strings.Join(geometry, ",")
	}
	if fieldOmitted > 0 {
		traceDBAddCoverageMetric(&a.coverage, "target_format_geometry_fields_omitted", int64(fieldOmitted))
	}
	if omitted > 0 {
		traceDBAddCoverageMetric(&a.coverage, "format_record_witnesses_omitted", int64(omitted))
		a.coverage.Metadata["decode_state"] = "incomplete_format_witness_cap"
		a.coverage.Skipped = "raw record decode ledger incomplete: format_witness_cap_exceeded"
		return a.coverage
	}
	if a.decodeCapped {
		traceDBAddCoverageMetric(&a.coverage, "target_decode_cap_exceeded", 1)
		a.coverage.Metadata["decode_state"] = "incomplete_target_decode_cap"
		a.coverage.Skipped = "raw record decode ledger incomplete: target_decode_cap_exceeded"
		return a.coverage
	}
	a.coverage.Metadata["decode_state"] = "strict_target_ledger_complete"
	a.coverage.Metadata["decoder_readiness"] = "requires_trace_streamer_family_reconciliation"
	return a.coverage
}

func traceDBRawDecodeStrictTarget(name string) bool {
	if name == "sched_switch" || name == "sched_switch_lite" || name == "sched_wakeup_lite" {
		return true
	}
	if _, governed := coreRenderKindForName(name); governed {
		return true
	}
	if directMarkerNameGoverned(name) {
		return true
	}
	return name == "dma_fence_wait_start" || name == "dma_fence_wait_end"
}

func traceDBRawDecodeMetricName(name string) string {
	switch name {
	case "print", "sched_switch", "sched_switch_lite", "sched_blocked_reason", "trace_vsync", "tracing_mark_write",
		"sched_wakeup_lite",
		"sched_wakeup", "sched_wakeup_new", "sched_waking",
		"dma_fence_destroy", "dma_fence_emit", "dma_fence_enable_signal",
		"dma_fence_init", "dma_fence_signaled", "dma_fence_wait_start", "dma_fence_wait_end":
		return name
	default:
		return "closed_target_other"
	}
}

func traceDBRawDecodeReasonMetric(reason string) string {
	var builder strings.Builder
	for _, value := range reason {
		switch {
		case value >= 'a' && value <= 'z', value >= '0' && value <= '9':
			builder.WriteRune(value)
		case value >= 'A' && value <= 'Z':
			builder.WriteRune(value + ('a' - 'A'))
		default:
			builder.WriteByte('_')
		}
		if builder.Len() >= 80 {
			break
		}
	}
	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "unspecified"
	}
	return result
}

func traceDBRawDecodeTargetNames() []string {
	return []string{
		"dma_fence_destroy",
		"dma_fence_emit",
		"dma_fence_enable_signal",
		"dma_fence_init",
		"dma_fence_signaled",
		"dma_fence_wait_end",
		"dma_fence_wait_start",
		"print",
		"sched_blocked_reason",
		"sched_switch",
		"sched_switch_lite",
		"sched_wakeup",
		"sched_wakeup_lite",
		"sched_wakeup_new",
		"sched_waking",
		"trace_vsync",
		"tracing_mark_write",
	}
}

func traceDBRawDecodeTargetGeometryWitnesses(catalog eventFormatCatalog) ([]string, int) {
	formats := make([]eventFormat, 0, len(catalog.Formats))
	for _, format := range catalog.Formats {
		if traceDBRawProbeTargetFormat(format.Name) {
			formats = append(formats, format)
		}
	}
	sort.Slice(formats, func(i, j int) bool {
		if formats[i].Name != formats[j].Name {
			return formats[i].Name < formats[j].Name
		}
		return formats[i].ID < formats[j].ID
	})
	omitted := 0
	witnesses := make([]string, 0, len(formats))
	for _, format := range formats {
		fields := format.Fields
		if len(fields) > maxTraceDBRawDecodeFieldsPerFormat {
			omitted += len(fields) - maxTraceDBRawDecodeFieldsPerFormat
			fields = fields[:maxTraceDBRawDecodeFieldsPerFormat]
		}
		fieldWitnesses := make([]string, 0, len(fields))
		for _, field := range fields {
			name := traceDBRawDecodeFormatWitnessName(cleanFieldName(field.Name))
			fieldWitnesses = append(fieldWitnesses,
				fmt.Sprintf("%s@%d:%d:signed=%t", name, field.Offset, field.Size, field.Signed))
		}
		witnesses = append(witnesses, fmt.Sprintf("%s#%d[%s]",
			traceDBRawDecodeFormatWitnessName(format.Name), format.ID, strings.Join(fieldWitnesses, "|")))
	}
	return witnesses, omitted
}

func traceDBRawDecodeAbsentTargets(catalog eventFormatCatalog) []string {
	present := make(map[string]bool, len(catalog.Formats))
	for _, format := range catalog.Formats {
		present[format.Name] = true
	}
	var absent []string
	for _, name := range traceDBRawDecodeTargetNames() {
		if !present[name] {
			absent = append(absent, name)
		}
	}
	return absent
}

func traceDBRawDecodeFormatWitnesses(
	catalog eventFormatCatalog,
	stats map[int]*traceDBRawDecodeFormatStats,
) ([]string, int) {
	formats := make([]eventFormat, 0, len(catalog.Formats))
	for _, format := range catalog.Formats {
		formats = append(formats, format)
	}
	sort.Slice(formats, func(i, j int) bool {
		if formats[i].Name != formats[j].Name {
			return formats[i].Name < formats[j].Name
		}
		return formats[i].ID < formats[j].ID
	})
	omitted := 0
	if len(formats) > maxTraceDBRawDecodeFormatWitnesses {
		omitted = len(formats) - maxTraceDBRawDecodeFormatWitnesses
		formats = formats[:maxTraceDBRawDecodeFormatWitnesses]
	}
	witnesses := make([]string, 0, len(formats))
	for _, format := range formats {
		count := int64(0)
		if item := stats[format.ID]; item != nil && item.Name == format.Name {
			count = item.Records
		}
		witnesses = append(witnesses,
			fmt.Sprintf("%s#%d/records=%d", traceDBRawDecodeFormatWitnessName(format.Name), format.ID, count))
	}
	return witnesses, omitted
}

func traceDBRawDecodeFormatWitnessName(name string) string {
	if len(name) > 0 && len(name) <= 128 {
		safe := true
		for _, value := range name {
			if (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') ||
				(value >= '0' && value <= '9') || value == '_' || value == '-' || value == '.' {
				continue
			}
			safe = false
			break
		}
		if safe {
			return name
		}
	}
	sum := sha256.Sum256([]byte(name))
	return fmt.Sprintf("name_sha256_%x_bytes_%d", sum, len(name))
}
