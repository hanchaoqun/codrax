package hitraceconv

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const (
	// Raw target records are decoded to completion. Publication consumers need
	// several retained typed subsets, so bound those retained values by an
	// explicit byte budget instead of rejecting every target after an arbitrary
	// row ordinal. The estimate deliberately over-counts string headers and
	// payload bytes; exceeding it withdraws raw publication authority while the
	// decoder still finishes the exact family census.
	maxTraceDBRawDecodeRetainedBytes   = 768 << 20
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

type traceDBRawDMAWaitRecord struct {
	TimestampNS  uint64
	CPU          int
	HeaderPID    int64
	Flags        int64
	PreemptCount int64
	Name         string
	Driver       string
	Timeline     string
	Context      uint64
	Seqno        uint64
}

// traceDBRawDMALifecycleRecord is an exact official point event. It is kept
// separate from wait endpoints because init/destroy/enable/signaled have no
// pairing phase and must never acquire duration semantics.
type traceDBRawDMALifecycleRecord struct {
	TimestampNS  uint64
	CPU          int
	HeaderPID    int64
	Flags        int64
	PreemptCount int64
	Name         string
	Driver       string
	Timeline     string
	Context      uint64
	Seqno        uint64
}

// traceDBRawMarkerRecord retains governed marker endpoint evidence and
// localized carrier failures. PayloadPID is the marker namespace value and is
// never silently rewritten. ZeroPIDUsesHeaderIdentity is true only for the
// exact official OpenHarmony pid/name/start producer, where zero explicitly
// means that the viewer keeps physical common_pid ownership.
// OpenHarmonyStructuredProfile retains that exact producer fact independently.
// OpenHarmonyPrintParserProfile is broader but still exact: the strict decoder
// admitted a print or tracing_mark_write body routed through the official
// PrintEventParser. It authorizes parser-level display normalization only and
// never the structured producer's zero-PID identity semantics. Value is the
// exact async cookie for admitted S/F rows and remains empty for B/E.
type traceDBRawMarkerRecord struct {
	PhysicalOrdinal               int64
	TimestampNS                   uint64
	CPU                           int
	HeaderPID                     int64
	Flags                         int64
	PreemptCount                  int64
	Buffer                        string
	Action                        string
	PayloadPID                    int64
	Name                          string
	Value                         string
	Admitted                      bool
	RejectReason                  string
	OpenHarmonyStructuredProfile  bool
	OpenHarmonyPrintParserProfile bool
	ZeroPIDUsesHeaderIdentity     bool
}

type traceDBSourceRawDecodeAccumulator struct {
	coverage          TraceDBCoverage
	formats           map[int]*traceDBRawDecodeFormatStats
	blockedRecords    []traceDBRawBlockedRecord
	dmaWaitRecords    []traceDBRawDMAWaitRecord
	dmaLifecycle      []traceDBRawDMALifecycleRecord
	markerRecords     []traceDBRawMarkerRecord
	switchLiteRecords []traceDBRawSchedSwitchLiteRecord
	wakeupLiteRecords []traceDBRawSchedWakeupLiteRecord
	wakeupNewNames    []traceDBRawSchedWakeupNewNameRecord
	targetRows        int64
	targetDecoded     int64
	targetFirstTS     uint64
	targetLastTS      uint64
	targetTimestamp   bool
	retainedBytes     int64
	retentionCapped   bool
	nextInfoTailOR    uint64
	nextInfoTailAND   uint64
	nextInfoTailSeen  bool
}

func newTraceDBSourceRawDecodeCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_decode",
		Table:  "__raw_record_decode__",
		Role:   "diagnostic_ledger",
		FieldSources: map[string]string{
			"authority":      "same immutable official input generation, admitted event-format catalog, and structurally validated page/record geometry as source_rawtrace_profile",
			"body_decode":    "closed strict decoders only: sched core, exact sched_switch, exact sched_switch_lite/sched_wakeup_lite, tracing marker, DMA wait endpoints, and official DMA lifecycle point events; generic/legacy fallback renderers never gain RPD authority",
			"geometry":       "bounded exact descriptor field name/offset/size/signed witnesses for closed target formats; field types and print-fmt text are not surfaced",
			"effect":         "this ledger is bounded independent raw-record accounting and RowsEmitted is always zero; retained typed records remain inert unless a separate family-specific census/deduplication/wire gate publishes them",
			"identity":       "strict common_pid/common_flags/common_preempt_count envelope is required per decoded target record; namespace and TGID are not inferred",
			"marker_sync":    "print and tracing_mark_write share tracequery.DecodeTraceMarkEndpointPayload as the sole complete-payload endpoint grammar; exact B/E payloads, admitted S/F payloads, and localized rejected carrier rows are retained without publication, while payload PID remains namespace data",
			"scheduler_lite": "sched_switch_lite retains exact prev/next PID, signed-16 priority, state and the full packed uint64 next_info; known bits render the stable current prefix while nonzero unknown high bits are counted and never guessed as future fields; sched_wakeup_lite retains exact target PID, signed-16 priority and target CPU; both remain internal until a separate DB join proves one-to-one publication authority",
			"limits":         "all structurally scanned target records receive strict body decoding; retained typed recovery records have a conservative 768 MiB in-memory byte budget, at most 64 sorted format/count witnesses and at most 32 fields per target geometry witness are surfaced; retention/format caps withdraw completion while field overflow is explicitly counted",
		},
		Metadata: map[string]string{
			"decode_state":          "unavailable",
			"publication_authority": "retained_records_require_dedicated_family_gate",
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

func (a *traceDBSourceRawDecodeAccumulator) reserveRetained(
	family string,
	fixedBytes int64,
	values ...string,
) bool {
	if a == nil || fixedBytes < 0 {
		return false
	}
	required := fixedBytes
	for _, value := range values {
		if int64(len(value)) > maxTraceDBRawDecodeRetainedBytes-required {
			a.retentionCapped = true
			traceDBAddCoverageMetric(&a.coverage,
				"target_"+traceDBRawDecodeReasonMetric(family)+"_retention_budget_exceeded", 1)
			return false
		}
		required += int64(len(value))
	}
	if a.retainedBytes > maxTraceDBRawDecodeRetainedBytes-required {
		a.retentionCapped = true
		traceDBAddCoverageMetric(&a.coverage,
			"target_"+traceDBRawDecodeReasonMetric(family)+"_retention_budget_exceeded", 1)
		return false
	}
	a.retainedBytes += required
	return true
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
	if directMarkerNameGoverned(format.Name) {
		traceDBAddCoverageMetric(&a.coverage, "target_marker_carrier_records", 1)
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
		if directMarkerNameGoverned(format.Name) {
			// `body` is already the exact result of the sole governed marker
			// decoder in renderEventBodyDecisionWithPair. Do not call that
			// source authority a second time here.
			verdict := tracequery.DecodeTraceMarkEndpointPayload(body)
			if verdict.Recognized {
				status := "rejected"
				if verdict.Admitted {
					status = "admitted"
				}
				traceDBAddCoverageMetric(&a.coverage,
					"target_marker_endpoint_"+strings.ToLower(verdict.Action)+"_"+status, 1)
			} else {
				traceDBAddCoverageMetric(&a.coverage,
					"target_marker_non_endpoint_payloads", 1)
			}
			switch verdict.Action {
			case "B", "E":
				openHarmonyStructuredProfile :=
					directMarkerOpenHarmonyStructuredProfile(event)
				openHarmonyPrintParserProfile :=
					directMarkerOpenHarmonyPrintParserProfile(event)
				zeroPIDUsesHeaderIdentity :=
					verdict.SpanPID == 0 &&
						openHarmonyStructuredProfile
				record := traceDBRawMarkerRecord{
					PhysicalOrdinal: int64(a.coverage.RowsRead),
					TimestampNS:     timestampNS, CPU: cpu, HeaderPID: int64(headerPID),
					Flags: flags, PreemptCount: preemptCount, Buffer: body,
					Action: verdict.Action, PayloadPID: int64(verdict.SpanPID),
					Name: verdict.Name, Admitted: verdict.Admitted,
					RejectReason:                  verdict.InvalidCause,
					OpenHarmonyStructuredProfile:  openHarmonyStructuredProfile,
					OpenHarmonyPrintParserProfile: openHarmonyPrintParserProfile,
					ZeroPIDUsesHeaderIdentity:     zeroPIDUsesHeaderIdentity,
				}
				if !a.reserveRetained(format.Name, 192, record.Buffer, record.Action,
					record.Name, record.Value, record.RejectReason) {
					return
				}
				a.markerRecords = append(a.markerRecords, record)
				traceDBAddCoverageMetric(&a.coverage, "target_marker_sync_records_retained", 1)
				if zeroPIDUsesHeaderIdentity {
					traceDBAddCoverageMetric(&a.coverage,
						"target_marker_sync_zero_pid_header_identity_endpoints", 1)
				}
				if verdict.Admitted {
					traceDBAddCoverageMetric(&a.coverage, "target_marker_sync_endpoints_admitted", 1)
				} else {
					traceDBAddCoverageMetric(&a.coverage, "target_marker_sync_endpoints_rejected", 1)
				}
			case "S", "F":
				if verdict.Admitted {
					record := traceDBRawMarkerRecord{
						PhysicalOrdinal: int64(a.coverage.RowsRead),
						TimestampNS:     timestampNS, CPU: cpu, HeaderPID: int64(headerPID),
						Flags: flags, PreemptCount: preemptCount, Buffer: body,
						Action: verdict.Action, PayloadPID: int64(verdict.SpanPID),
						Name: verdict.Name, Value: verdict.Value, Admitted: true,
					}
					if !a.reserveRetained(format.Name, 192, record.Buffer, record.Action,
						record.Name, record.Value, record.RejectReason) {
						return
					}
					a.markerRecords = append(a.markerRecords, record)
					traceDBAddCoverageMetric(&a.coverage,
						"target_marker_async_records_retained", 1)
				} else {
					record := traceDBRawMarkerRecord{
						PhysicalOrdinal: int64(a.coverage.RowsRead),
						TimestampNS:     timestampNS, CPU: cpu, HeaderPID: int64(headerPID),
						Flags: flags, PreemptCount: preemptCount, Buffer: body,
						Action: verdict.Action, PayloadPID: int64(verdict.SpanPID),
						Name: verdict.Name, RejectReason: "schema_" + verdict.InvalidCause,
					}
					if !a.reserveRetained(format.Name, 192, record.Buffer, record.Action,
						record.Name, record.Value, record.RejectReason) {
						return
					}
					a.markerRecords = append(a.markerRecords, record)
					traceDBAddCoverageMetric(&a.coverage,
						"target_marker_sync_poison_records_retained", 1)
				}
			default:
				traceDBAddCoverageMetric(&a.coverage, "target_marker_non_sync_payloads", 1)
			}
		} else if format.Name == "sched_blocked_reason" {
			blocked, blockedReason := decodeDirectBlockedPayload(event, content)
			if blockedReason != "" {
				traceDBAddCoverageMetric(&a.coverage, "target_sched_blocked_reason_key_capture_failed", 1)
				return
			}
			record := traceDBRawBlockedRecord{
				TimestampNS: timestampNS, CPU: cpu, HeaderPID: int64(headerPID),
				Flags: flags, PreemptCount: preemptCount, TargetTID: blocked.PID,
				IOWait: int64(blocked.IOWait), CallerRaw: blocked.CallerRaw,
				Caller: blocked.Caller, CallerSymbolized: blocked.CallerSymbolized,
				CNodeIndex: blocked.CNodeIndex, CNodeKnown: blocked.CNodeKnown,
				Delay: blocked.Delay, DelayKnown: blocked.DelayKnown,
			}
			if !a.reserveRetained(format.Name, 144, record.Caller) {
				return
			}
			a.blockedRecords = append(a.blockedRecords, record)
		} else if format.Name == "sched_switch_lite" {
			lite, liteReason := decodeTraceDBRawSchedSwitchLite(event)
			if liteReason != "" {
				traceDBAddCoverageMetric(&a.coverage, "target_sched_switch_lite_record_capture_failed", 1)
				return
			}
			lite.TimestampNS, lite.CPU = timestampNS, cpu
			lite.HeaderPID, lite.Flags, lite.PreemptCount = int64(headerPID), flags, preemptCount
			if tail := traceDBRawSchedSwitchLiteNextInfoUnknownTailBits(lite); tail != 0 {
				traceDBAddCoverageMetric(&a.coverage,
					"target_sched_switch_lite_next_info_unknown_tail_bits", 1)
				a.nextInfoTailOR |= tail
				if !a.nextInfoTailSeen {
					a.nextInfoTailAND = tail
					a.nextInfoTailSeen = true
				} else {
					a.nextInfoTailAND &= tail
				}
			}
			if !a.reserveRetained(format.Name, 104) {
				return
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
			if !a.reserveRetained(format.Name, 80) {
				return
			}
			a.wakeupLiteRecords = append(a.wakeupLiteRecords, lite)
		} else if format.Name == "sched_wakeup_new" {
			payload, payloadAdmission, payloadReason :=
				decodeDirectCorePayload(coreDecodeContext{}, event, content)
			if payloadAdmission != bodyAdmitted || payloadReason != "" ||
				payload.Wakeup == nil {
				traceDBAddCoverageMetric(
					&a.coverage, "target_sched_wakeup_new_name_record_capture_failed", 1)
				return
			}
			if payload.Wakeup.Comm == "<...>" {
				traceDBAddCoverageMetric(
					&a.coverage, "target_sched_wakeup_new_name_unavailable", 1)
			} else {
				record := traceDBRawSchedWakeupNewNameRecord{
					TimestampNS: timestampNS,
					TargetTID:   payload.Wakeup.PID,
					Name:        payload.Wakeup.Comm,
				}
				if !a.reserveRetained(format.Name, 48, record.Name) {
					return
				}
				a.wakeupNewNames = append(a.wakeupNewNames, record)
				traceDBAddCoverageMetric(
					&a.coverage, "target_sched_wakeup_new_name_records_retained", 1)
			}
		} else if traceDBRawDMALifecycleName(format.Name) {
			dma, dmaAdmission, dmaReason :=
				decodeDirectDMAFenceLifecycleFields(event, content)
			if dmaAdmission != bodyAdmitted || dmaReason != "" ||
				dma == nil || !dma.DriverKnown || !dma.TimelineKnown ||
				!dma.ContextKnown || !dma.SeqnoKnown ||
				dma.NumberBits != 32 {
				traceDBAddCoverageMetric(&a.coverage,
					"target_dma_fence_lifecycle_record_capture_failed", 1)
				return
			}
			record := traceDBRawDMALifecycleRecord{
				TimestampNS: timestampNS, CPU: cpu,
				HeaderPID: int64(headerPID), Flags: flags,
				PreemptCount: preemptCount, Name: format.Name,
				Driver: dma.Driver, Timeline: dma.Timeline,
				Context: dma.Context, Seqno: dma.Seqno,
			}
			if !a.reserveRetained(format.Name, 112, record.Name,
				record.Driver, record.Timeline) {
				return
			}
			a.dmaLifecycle = append(a.dmaLifecycle, record)
		} else if format.Name == "dma_fence_wait_start" || format.Name == "dma_fence_wait_end" {
			payload, payloadAdmission, payloadReason := decodeDirectPairPayload(event, content)
			if payloadAdmission != bodyAdmitted || payloadReason != "" ||
				payload.Kind != pairRenderDMAFence || payload.Name != format.Name ||
				payload.DMAFence == nil {
				traceDBAddCoverageMetric(&a.coverage, "target_dma_fence_wait_record_capture_failed", 1)
				return
			}
			dma := payload.DMAFence
			if !dma.DriverKnown || !dma.TimelineKnown || !dma.ContextKnown || !dma.SeqnoKnown ||
				dma.NumberBits != 32 {
				traceDBAddCoverageMetric(&a.coverage, "target_dma_fence_wait_record_capture_failed", 1)
				return
			}
			record := traceDBRawDMAWaitRecord{
				TimestampNS: timestampNS, CPU: cpu, HeaderPID: int64(headerPID),
				Flags: flags, PreemptCount: preemptCount, Name: format.Name,
				Driver: dma.Driver, Timeline: dma.Timeline,
				Context: dma.Context, Seqno: dma.Seqno,
			}
			if !a.reserveRetained(format.Name, 112, record.Name,
				record.Driver, record.Timeline) {
				return
			}
			a.dmaWaitRecords = append(a.dmaWaitRecords, record)
		}
	case bodyRejected:
		traceDBAddCoverageMetric(&a.coverage, "target_body_rejected", 1)
		traceDBAddCoverageMetric(&a.coverage, "target_"+metric+"_body_rejected", 1)
		if reason == "" {
			reason = "unspecified"
		}
		traceDBAddCoverageMetric(&a.coverage,
			"target_"+metric+"_reject_"+traceDBRawDecodeReasonMetric(reason), 1)
		if directMarkerNameGoverned(format.Name) {
			record := traceDBRawMarkerRecord{
				PhysicalOrdinal: int64(a.coverage.RowsRead),
				TimestampNS:     timestampNS, CPU: cpu, HeaderPID: int64(headerPID),
				Flags: flags, PreemptCount: preemptCount,
				RejectReason: "carrier_" + reason,
			}
			if !a.reserveRetained(format.Name, 192, record.Buffer, record.Action,
				record.Name, record.Value, record.RejectReason) {
				return
			}
			a.markerRecords = append(a.markerRecords, record)
			traceDBAddCoverageMetric(&a.coverage, "target_marker_carrier_rejections_retained", 1)
		}
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
	case "dma_fence_destroy", "dma_fence_enable_signal",
		"dma_fence_init", "dma_fence_signaled":
		dma, admission, reason :=
			decodeDirectDMAFenceLifecycleFields(event, content)
		if admission != bodyAdmitted {
			return "", admission, reason
		}
		body, ok := renderCanonicalDMAFenceLifecycleFields(dma)
		if !ok {
			return "", bodyRejected, "invalid_dma_fence_lifecycle_body"
		}
		return body, bodyAdmitted, ""
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
	if a.nextInfoTailSeen {
		a.coverage.Metadata["scheduler_lite_next_info_unknown_tail_or"] =
			fmt.Sprintf("0x%016x", a.nextInfoTailOR)
		a.coverage.Metadata["scheduler_lite_next_info_unknown_tail_and"] =
			fmt.Sprintf("0x%016x", a.nextInfoTailAND)
	}
	witnesses, omitted := traceDBRawDecodeFormatWitnesses(catalog, a.formats)
	if len(witnesses) > 0 {
		a.coverage.Metadata["format_record_witnesses"] = strings.Join(witnesses, ",")
	}
	geometry, fieldOmitted := traceDBRawDecodeTargetGeometryWitnesses(catalog)
	if len(geometry) > 0 {
		a.coverage.Metadata["target_format_geometry_witnesses"] = strings.Join(geometry, ",")
	}
	markerGeometry, markerFieldOmitted := traceDBRawDecodeGeometryWitnesses(
		catalog, directMarkerNameGoverned)
	if len(markerGeometry) > 0 {
		a.coverage.Metadata["marker_format_geometry_witnesses"] =
			strings.Join(markerGeometry, ",")
	}
	schedulerLiteGeometry, schedulerLiteFieldOmitted :=
		traceDBRawDecodeTypedGeometryWitnesses(catalog, traceDBRawSchedulerLiteFormat)
	if len(schedulerLiteGeometry) > 0 {
		a.coverage.Metadata["scheduler_lite_format_geometry_witnesses"] =
			strings.Join(schedulerLiteGeometry, ",")
	}
	schedulerWakeupGeometry, schedulerWakeupFieldOmitted :=
		traceDBRawDecodeTypedGeometryWitnesses(catalog, traceDBRawSchedulerWakeupFormat)
	if len(schedulerWakeupGeometry) > 0 {
		a.coverage.Metadata["scheduler_wakeup_format_geometry_witnesses"] =
			strings.Join(schedulerWakeupGeometry, ",")
	}
	blockedGeometry, blockedFieldOmitted :=
		traceDBRawDecodeTypedGeometryWitnesses(catalog, func(name string) bool {
			return name == "sched_blocked_reason"
		})
	if len(blockedGeometry) > 0 {
		a.coverage.Metadata["blocked_reason_format_geometry_witnesses"] =
			strings.Join(blockedGeometry, ",")
	}
	if fieldOmitted > 0 {
		traceDBAddCoverageMetric(&a.coverage, "target_format_geometry_fields_omitted", int64(fieldOmitted))
	}
	if markerFieldOmitted > 0 {
		traceDBAddCoverageMetric(&a.coverage, "marker_format_geometry_fields_omitted", int64(markerFieldOmitted))
	}
	if schedulerLiteFieldOmitted > 0 {
		traceDBAddCoverageMetric(&a.coverage,
			"scheduler_lite_format_geometry_fields_omitted", int64(schedulerLiteFieldOmitted))
	}
	if schedulerWakeupFieldOmitted > 0 {
		traceDBAddCoverageMetric(&a.coverage,
			"scheduler_wakeup_format_geometry_fields_omitted", int64(schedulerWakeupFieldOmitted))
	}
	if blockedFieldOmitted > 0 {
		traceDBAddCoverageMetric(&a.coverage,
			"blocked_reason_format_geometry_fields_omitted", int64(blockedFieldOmitted))
	}
	if omitted > 0 {
		traceDBAddCoverageMetric(&a.coverage, "format_record_witnesses_omitted", int64(omitted))
		a.coverage.Metadata["decode_state"] = "incomplete_format_witness_cap"
		a.coverage.Skipped = "raw record decode ledger incomplete: format_witness_cap_exceeded"
		return a.coverage
	}
	traceDBAddCoverageMetric(&a.coverage, "target_retained_bytes", a.retainedBytes)
	if a.retentionCapped {
		traceDBAddCoverageMetric(&a.coverage, "target_retention_budget_exceeded", 1)
		a.coverage.Metadata["decode_state"] = "incomplete_target_retention_budget"
		a.coverage.Skipped = "raw record decode ledger incomplete: target_retention_byte_budget_exceeded"
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
	return name == "dma_fence_wait_start" || name == "dma_fence_wait_end" ||
		traceDBRawDMALifecycleName(name)
}

func traceDBRawDMALifecycleName(name string) bool {
	switch name {
	case "dma_fence_destroy", "dma_fence_enable_signal",
		"dma_fence_init", "dma_fence_signaled":
		return true
	default:
		return false
	}
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
	return traceDBRawDecodeGeometryWitnesses(catalog, traceDBRawProbeTargetFormat)
}

func traceDBRawDecodeGeometryWitnesses(
	catalog eventFormatCatalog,
	include func(string) bool,
) ([]string, int) {
	return traceDBRawDecodeGeometryWitnessesWithTypes(catalog, include, false)
}

func traceDBRawDecodeTypedGeometryWitnesses(
	catalog eventFormatCatalog,
	include func(string) bool,
) ([]string, int) {
	return traceDBRawDecodeGeometryWitnessesWithTypes(catalog, include, true)
}

func traceDBRawDecodeGeometryWitnessesWithTypes(
	catalog eventFormatCatalog,
	include func(string) bool,
	includeType bool,
) ([]string, int) {
	formats := make([]eventFormat, 0, len(catalog.Formats))
	for _, format := range catalog.Formats {
		if include != nil && include(format.Name) {
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
			witness := fmt.Sprintf("%s@%d:%d:signed=%t",
				name, field.Offset, field.Size, field.Signed)
			if includeType {
				witness += ":type=" +
					traceDBRawDecodeReasonMetric(normalizeFieldType(field.Type))
			}
			fieldWitnesses = append(fieldWitnesses, witness)
		}
		witnesses = append(witnesses, fmt.Sprintf("%s#%d[%s]",
			traceDBRawDecodeFormatWitnessName(format.Name), format.ID, strings.Join(fieldWitnesses, "|")))
	}
	return witnesses, omitted
}

func traceDBRawSchedulerLiteFormat(name string) bool {
	return name == "sched_switch_lite" || name == "sched_wakeup_lite"
}

func traceDBRawSchedulerWakeupFormat(name string) bool {
	return name == "sched_wakeup_lite" || name == "sched_wakeup_new"
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
