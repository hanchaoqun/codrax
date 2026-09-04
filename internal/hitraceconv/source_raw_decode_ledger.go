package hitraceconv

import (
	"crypto/sha256"
	"encoding/base64"
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

const (
	traceDBRawRetentionMarker         = "marker"
	traceDBRawRetentionBlocked        = "blocked_reason"
	traceDBRawRetentionSwitchLite     = "sched_switch_lite"
	traceDBRawRetentionWakeupLite     = "sched_wakeup_lite"
	traceDBRawRetentionWakeupName     = "sched_wakeup_new_name"
	traceDBRawRetentionDMAWait        = "dma_wait"
	traceDBRawRetentionDMALifecycle   = "dma_lifecycle"
	traceDBRawRetentionBlock          = "block_storage"
	traceDBRawRetentionWorkqueue      = "workqueue"
	traceDBRawRetentionFilemap        = "filemap"
	traceDBRawRetentionF2FS           = "f2fs"
	traceDBRawRetentionFamilyComplete = "complete"
)

var traceDBRawRetentionFamilyBudgets = map[string]int64{
	traceDBRawRetentionMarker:       408 << 20,
	traceDBRawRetentionBlocked:      56 << 20,
	traceDBRawRetentionSwitchLite:   128 << 20,
	traceDBRawRetentionWakeupLite:   72 << 20,
	traceDBRawRetentionWakeupName:   16 << 20,
	traceDBRawRetentionDMAWait:      8 << 20,
	traceDBRawRetentionDMALifecycle: 8 << 20,
	traceDBRawRetentionBlock:        16 << 20,
	traceDBRawRetentionWorkqueue:    8 << 20,
	traceDBRawRetentionFilemap:      32 << 20,
	traceDBRawRetentionF2FS:         16 << 20,
}

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

// traceDBRawBlockRecord is one byte-exact block request/BIO event recovered
// from the immutable official raw page. HeaderPID remains the physical
// common_pid namespace value; it is not rewritten to a host PID or TGID.
// Body is the output of the sole governed direct block decoder and excludes
// the stable event-name prefix.
type traceDBRawBlockRecord struct {
	PhysicalOrdinal int64
	TimestampNS     uint64
	CPU             int
	HeaderPID       int64
	Flags           int64
	PreemptCount    int64
	Name            string
	Body            string
}

// traceDBRawExactRecord is one canonical point or endpoint produced by an
// existing strict descriptor decoder. Family is a closed typed identity, not
// an event-name prefix inferred at publication time.
type traceDBRawExactRecord struct {
	PhysicalOrdinal int64
	TimestampNS     uint64
	CPU             int
	HeaderPID       int64
	Flags           int64
	PreemptCount    int64
	Family          string
	Name            string
	Body            string
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
	blockRecords      []traceDBRawBlockRecord
	exactRecords      []traceDBRawExactRecord
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
	retainedByFamily  map[string]int64
	cappedFamilies    map[string]bool
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
			"body_decode":    "closed strict decoders only: sched core, exact sched_switch, exact sched_switch_lite/sched_wakeup_lite, tracing marker, exact block request/BIO, workqueue, filemap/page-cache, F2FS, DMA wait endpoints, and official DMA lifecycle point events; generic/legacy fallback renderers never gain RPD authority",
			"geometry":       "bounded exact descriptor field name/offset/size/signed witnesses for closed target formats; field types and print-fmt text are not surfaced",
			"effect":         "this ledger is bounded independent raw-record accounting and RowsEmitted is always zero; retained typed records remain inert unless a separate family-specific census/deduplication/wire gate publishes them",
			"identity":       "strict common_pid/common_flags/common_preempt_count envelope is required per decoded target record; namespace and TGID are not inferred",
			"marker_sync":    "print and tracing_mark_write share tracequery.DecodeTraceMarkEndpointPayload as the sole complete-payload endpoint grammar; exact B/E payloads, admitted S/F payloads, and localized rejected carrier rows are retained without publication, while payload PID remains namespace data",
			"scheduler_lite": "sched_switch_lite retains exact prev/next PID, signed-16 priority, state and the full packed uint64 next_info; known bits render the stable current prefix while nonzero unknown high bits are counted and never guessed as future fields; sched_wakeup_lite retains exact target PID, signed-16 priority and target CPU; both remain internal until a separate DB join proves one-to-one publication authority",
			"limits":         "all structurally scanned target records receive strict body decoding; retained typed recovery records have conservative per-family budgets totaling at most 768 MiB, at most 64 sorted format/count witnesses and at most 32 fields per target geometry witness are surfaced; retention/format caps withdraw completion while field overflow is explicitly counted",
		},
		Metadata: map[string]string{
			"decode_state":          traceDBRawDecodeStateUnavailable,
			"publication_authority": "retained_records_require_dedicated_family_gate",
		},
	}
}

func newTraceDBSourceRawDecodeAccumulator() traceDBSourceRawDecodeAccumulator {
	return traceDBSourceRawDecodeAccumulator{
		coverage:         newTraceDBSourceRawDecodeCoverage(),
		formats:          map[int]*traceDBRawDecodeFormatStats{},
		retainedByFamily: map[string]int64{},
		cappedFamilies:   map[string]bool{},
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
	formatName string,
	fixedBytes int64,
	values ...string,
) bool {
	if a == nil || fixedBytes < 0 {
		return false
	}
	family := traceDBRawRetentionFamily(formatName)
	limit := traceDBRawRetentionFamilyBudgets[family]
	if limit <= 0 {
		a.retentionCapped = true
		a.cappedFamilies[family] = true
		traceDBAddCoverageMetric(&a.coverage,
			"target_"+family+"_retention_budget_exceeded", 1)
		return false
	}
	required := fixedBytes
	for _, value := range values {
		if int64(len(value)) > limit-required {
			a.retentionCapped = true
			a.cappedFamilies[family] = true
			traceDBAddCoverageMetric(&a.coverage,
				"target_"+family+"_retention_budget_exceeded", 1)
			return false
		}
		required += int64(len(value))
	}
	if a.retainedByFamily[family] > limit-required {
		a.retentionCapped = true
		a.cappedFamilies[family] = true
		traceDBAddCoverageMetric(&a.coverage,
			"target_"+family+"_retention_budget_exceeded", 1)
		return false
	}
	a.retainedByFamily[family] += required
	a.retainedBytes += required
	return true
}

func traceDBRawRetentionFamily(name string) string {
	switch {
	case directMarkerNameGoverned(name):
		return traceDBRawRetentionMarker
	case name == "sched_blocked_reason":
		return traceDBRawRetentionBlocked
	case name == "sched_switch_lite":
		return traceDBRawRetentionSwitchLite
	case name == "sched_wakeup_lite":
		return traceDBRawRetentionWakeupLite
	case name == "sched_wakeup_new":
		return traceDBRawRetentionWakeupName
	case name == "dma_fence_wait_start" || name == "dma_fence_wait_end":
		return traceDBRawRetentionDMAWait
	case traceDBRawDMALifecycleName(name):
		return traceDBRawRetentionDMALifecycle
	case directBlockNameGoverned(name):
		return traceDBRawRetentionBlock
	case directPairNameGoverned(name) && name != "dma_fence_wait_start" && name != "dma_fence_wait_end":
		return traceDBRawRetentionWorkqueue
	case directFilemapNameGoverned(name):
		return traceDBRawRetentionFilemap
	case directF2FSNameGoverned(name):
		return traceDBRawRetentionF2FS
	default:
		return traceDBRawDecodeReasonMetric(name)
	}
}

// traceDBRawDecodeCensusComplete reads the decode_state through the single
// gate table (source_raw_lane_gate.go): complete means the state's gate kind
// is Ready, never a spelling test.
func traceDBRawDecodeCensusComplete(coverage TraceDBCoverage) bool {
	return traceDBRawDecodeStateGates[coverage.Metadata["decode_state"]] == traceDBSourceRawGateReady
}

func traceDBRawDecodeFamilyComplete(coverage TraceDBCoverage, family string) bool {
	if !traceDBRawDecodeCensusComplete(coverage) {
		return false
	}
	state, present := coverage.Metadata["retention_"+family+"_state"]
	return !present || state == traceDBRawRetentionFamilyComplete
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
		traceDBAddCoverageMetric(&a.coverage, "visibility_candidate_records", 1)
		event := decodeEvent(format, content)
		if _, _, _, ok := decodeDirectFtraceCommonEnvelope(event); !ok {
			traceDBAddCoverageMetric(&a.coverage, "visibility_envelope_rejected", 1)
			return
		}
		traceDBAddCoverageMetric(&a.coverage, "visibility_envelope_admitted", 1)
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
		} else if directBlockNameGoverned(format.Name) {
			if directBlockPairEndpointName(format.Name) {
				verdict := tracequery.DecodePairingEndpoint(
					format.Name, body, int64(headerPID))
				if !verdict.Recognized ||
					verdict.Family != tracequery.PairingEndpointBlock ||
					!verdict.KeyKnown || !verdict.PayloadAdmitted ||
					!verdict.EmitterKnown || !verdict.EmitterAdmitted {
					traceDBAddCoverageMetric(&a.coverage,
						"target_block_storage_record_capture_failed", 1)
					return
				}
			}
			record := traceDBRawBlockRecord{
				PhysicalOrdinal: int64(a.coverage.RowsRead),
				TimestampNS:     timestampNS, CPU: cpu, HeaderPID: int64(headerPID),
				Flags: flags, PreemptCount: preemptCount,
				Name: format.Name, Body: body,
			}
			if !a.reserveRetained(format.Name, 112, record.Name, record.Body) {
				return
			}
			a.blockRecords = append(a.blockRecords, record)
			traceDBAddCoverageMetric(&a.coverage,
				"target_block_storage_records_retained", 1)
		} else if family := traceDBRawExactRecoveryFamily(format.Name); family != "" {
			if traceDBRawExactRecoveryPairEndpoint(format.Name) {
				verdict := tracequery.DecodePairingEndpoint(
					format.Name, body, int64(headerPID))
				if !traceDBRawExactRecoveryVerdictMatches(
					family, format.Name, body, int64(headerPID), verdict) {
					traceDBAddCoverageMetric(&a.coverage,
						"target_"+family+"_record_capture_failed", 1)
					return
				}
			}
			record := traceDBRawExactRecord{
				PhysicalOrdinal: int64(a.coverage.RowsRead),
				TimestampNS:     timestampNS, CPU: cpu, HeaderPID: int64(headerPID),
				Flags: flags, PreemptCount: preemptCount,
				Family: family, Name: format.Name, Body: body,
			}
			if !a.reserveRetained(format.Name, 128,
				record.Family, record.Name, record.Body) {
				return
			}
			a.exactRecords = append(a.exactRecords, record)
			traceDBAddCoverageMetric(&a.coverage,
				"target_"+family+"_records_retained", 1)
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
		a.setUnavailable(traceDBRawDecodeStateNotApplicableNonOfficialProfile,
			"official raw record decode ledger not applicable to this envelope", false)
		return a.coverage
	}
	if profile.Metadata["probe_state"] != "complete" ||
		profile.Metadata["page_layout_state"] != "qword_length_cpu_candidate_all_pages" ||
		profile.Metadata["event_format_probe_state"] != "parsed_strict" {
		a.setUnavailable(traceDBRawDecodeStateWithheldProfileNotReady,
			"raw record decode ledger withheld: strict page/format profile is not complete", true)
		return a.coverage
	}
	if profile.Metrics["event_format_ids_poisoned"] > 0 ||
		a.coverage.Metrics["records_without_admitted_format"] > 0 ||
		int64(a.coverage.RowsRead) != profile.Metrics["records_structurally_scanned"] {
		a.setUnavailable(traceDBRawDecodeStateWithheldRecordFormatAccountingIncomplete,
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
	hmfsGeometry, hmfsFieldOmitted :=
		traceDBRawDecodeTypedGeometryWitnesses(catalog, traceDBRawHMFSFormat)
	if len(hmfsGeometry) > 0 {
		a.coverage.Metadata["hmfs_format_geometry_witnesses"] =
			strings.Join(hmfsGeometry, ",")
	}
	if hmfsPrint := traceDBRawHMFSPrintFormatWitnesses(catalog); len(hmfsPrint) > 0 {
		a.coverage.Metadata["hmfs_print_format_base64_witnesses"] =
			strings.Join(hmfsPrint, ",")
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
	if hmfsFieldOmitted > 0 {
		traceDBAddCoverageMetric(&a.coverage,
			"hmfs_format_geometry_fields_omitted", int64(hmfsFieldOmitted))
	}
	if omitted > 0 {
		traceDBAddCoverageMetric(&a.coverage, "format_record_witnesses_omitted", int64(omitted))
		a.coverage.Metadata["decode_state"] = traceDBRawDecodeStateIncompleteFormatWitnessCap
		a.coverage.Skipped = "raw record decode ledger incomplete: format_witness_cap_exceeded"
		return a.coverage
	}
	traceDBAddCoverageMetric(&a.coverage, "target_retained_bytes", a.retainedBytes)
	for family := range traceDBRawRetentionFamilyBudgets {
		traceDBAddCoverageMetric(&a.coverage,
			"target_"+family+"_retained_bytes", a.retainedByFamily[family])
		state := traceDBRawRetentionFamilyComplete
		if a.cappedFamilies[family] {
			state = "incomplete_byte_budget"
		}
		a.coverage.Metadata["retention_"+family+"_state"] = state
	}
	if a.retentionCapped {
		traceDBAddCoverageMetric(&a.coverage, "target_retention_budget_exceeded", 1)
		a.coverage.Metadata["decode_state"] =
			traceDBRawDecodeStateCompleteWithFamilyRetentionWithdrawal
		a.coverage.Skipped =
			"raw record decode census complete; one or more family recovery stores were withheld by retained-byte budget"
		return a.coverage
	}
	a.coverage.Metadata["decode_state"] = traceDBRawDecodeStateComplete
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
	if directBlockNameGoverned(name) {
		return true
	}
	if traceDBRawExactRecoveryFamily(name) != "" {
		return true
	}
	return name == "dma_fence_wait_start" || name == "dma_fence_wait_end" ||
		traceDBRawDMALifecycleName(name)
}

func traceDBRawExactRecoveryFamily(name string) string {
	switch {
	case name == "workqueue_execute_start" || name == "workqueue_execute_end":
		return traceDBRawRetentionWorkqueue
	case directFilemapNameGoverned(name):
		return traceDBRawRetentionFilemap
	case directF2FSNameGoverned(name):
		return traceDBRawRetentionF2FS
	default:
		return ""
	}
}

func traceDBRawExactRecoveryPairEndpoint(name string) bool {
	return name == "workqueue_execute_start" || name == "workqueue_execute_end" ||
		directF2FSNameGoverned(name)
}

func traceDBRawExactRecoveryTargetNames() []string {
	return []string{
		"file_check_and_advance_wb_err",
		"filemap_set_wb_err",
		"f2fs_direct_IO_enter",
		"f2fs_direct_IO_exit",
		"f2fs_sync_file_enter",
		"f2fs_sync_file_exit",
		"f2fs_write_begin",
		"f2fs_write_end",
		"mm_filemap_add_to_page_cache",
		"mm_filemap_delete_from_page_cache",
		"workqueue_execute_end",
		"workqueue_execute_start",
	}
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

func traceDBRawBlockTargetNames() []string {
	return []string{
		"block_bio_complete",
		"block_bio_queue",
		"block_bio_remap",
		"block_rq_complete",
		"block_rq_insert",
		"block_rq_issue",
		"block_rq_remap",
	}
}

// traceDBRawBlockNameGoverned is exact membership in the source block target
// registry. No prefix or case folding: the SQL raw class "block_storage" is
// wider than this set (MMC/UFS/SCSI endpoints), and only these names may be
// superseded by a complete source block family.
func traceDBRawBlockNameGoverned(name string) bool {
	for _, target := range traceDBRawBlockTargetNames() {
		if name == target {
			return true
		}
	}
	return false
}

// traceDBRawSourceSupersession registers one "complete source family
// supersedes normalized SQLite raw rows" rule. The suppression set of the rule
// is exactly Governed, the family's own governed event-name set from this
// ledger; it is never a SQL raw class. Every consumer of the rule — the DB
// exporter's row suppression and observation zeroing, the DB-side publishable
// census, and the source lane's overlap check — reads this registry by name.
type traceDBRawSourceSupersession struct {
	// Family is the retention family key shared with the decode ledger
	// (retention_<Family>_state, target_<Family>_records_retained).
	Family string
	// Reason is the stable wire token used in coverage Skipped reasons and
	// FieldSources keys; it is pinned by existing receipts and tests.
	Reason string
	// Governed is exact event-name membership in the family's source target set.
	Governed func(name string) bool
	// Eligible reports whether the immutable source inventory holds a complete,
	// fully admitted family that owns publication of the governed names.
	Eligible func(inventory *traceDBSourceNameInventory) bool
	// DBSupersedesSource marks the inverse policy (§40.42 ④a): the DB lane keeps
	// publication authority and the SOURCE family is withheld when any DB row
	// of a governed name was publishable. Such an entry never enters the
	// exporter's superseding map, but its governed names are still counted so
	// the withhold check reads exact names, not the SQL raw class.
	DBSupersedesSource bool
}

// RowReason is the DB raw class Skipped reason for a superseded governed row.
func (s traceDBRawSourceSupersession) RowReason() string {
	return "superseded_complete_source_raw_" + s.Reason + "_family"
}

// PublishableMetric names the DB raw coverage metric counting rows of governed
// event names the DB lane still holds publication authority over. It is 0 by
// construction whenever the family is eligible and is the precise witness the
// source lane's overlap check reads instead of the class-wide RowsEmitted.
func (s traceDBRawSourceSupersession) PublishableMetric() string {
	return "source_governed_" + s.Family + "_rows_publishable"
}

// PrecedenceField is the schema coverage FieldSources disclosure key.
func (s traceDBRawSourceSupersession) PrecedenceField() string {
	return s.Reason + "_source_precedence"
}

var traceDBRawSourceSupersessions = buildTraceDBRawSourceSupersessions()

func buildTraceDBRawSourceSupersessions() []traceDBRawSourceSupersession {
	out := []traceDBRawSourceSupersession{{
		Family:   traceDBRawRetentionBlock,
		Reason:   "block",
		Governed: traceDBRawBlockNameGoverned,
		Eligible: traceDBRawBlockFamilyAuthorityEligible,
	}}
	for _, family := range traceDBRawExactRecoveryFamilies {
		out = append(out, traceDBRawSourceSupersession{
			Family: family,
			Reason: family,
			Governed: func(name string) bool {
				return traceDBRawExactRecoveryFamily(name) == family
			},
			Eligible: func(inventory *traceDBSourceNameInventory) bool {
				return traceDBRawExactFamilyAuthorityEligible(inventory, family)
			},
		})
	}
	// DMA wait / lifecycle: DB rows keep authority; the source family is
	// withheld only on an overlap of its exact governed names (never on the
	// dma_fence SQL class, which also carries DB-only names).
	for _, family := range []string{traceDBRawRetentionDMAWait, traceDBRawRetentionDMALifecycle} {
		out = append(out, traceDBRawSourceSupersession{
			Family:             family,
			Reason:             family,
			Governed:           func(name string) bool { return traceDBRawRetentionFamily(name) == family },
			Eligible:           func(*traceDBSourceNameInventory) bool { return false },
			DBSupersedesSource: true,
		})
	}
	return out
}

// traceDBRawSourceSupersessionFor returns the registry entry whose governed
// set contains name. Governed sets are disjoint (pinned by census), so the
// first hit is the only hit.
func traceDBRawSourceSupersessionFor(name string) (traceDBRawSourceSupersession, bool) {
	for _, entry := range traceDBRawSourceSupersessions {
		if entry.Governed(name) {
			return entry, true
		}
	}
	return traceDBRawSourceSupersession{}, false
}

func traceDBRawSourceSupersessionForFamily(family string) (traceDBRawSourceSupersession, bool) {
	for _, entry := range traceDBRawSourceSupersessions {
		if entry.Family == family {
			return entry, true
		}
	}
	return traceDBRawSourceSupersession{}, false
}

// traceDBRawSourceGovernedRowsPublishable sums, over every DB raw-ftrace
// coverage item, the rows of the entry's governed event names the DB lane
// kept publication authority over. It replaces the class-keyed RowsEmitted
// overlap comparison: rows of ungoverned names sharing a SQL raw class never
// count as an overlap with the source family.
func traceDBRawSourceGovernedRowsPublishable(
	dbRawCoverage []TraceDBCoverage,
	entry traceDBRawSourceSupersession,
) int64 {
	total := int64(0)
	for _, item := range dbRawCoverage {
		if item.Family == "raw_ftrace" {
			total += item.Metrics[entry.PublishableMetric()]
		}
	}
	return total
}

func traceDBRawDecodeMetricName(name string) string {
	switch name {
	case "print", "sched_switch", "sched_switch_lite", "sched_blocked_reason", "trace_vsync", "tracing_mark_write",
		"sched_wakeup_lite",
		"sched_wakeup", "sched_wakeup_new", "sched_waking",
		"block_bio_complete", "block_bio_queue", "block_bio_remap",
		"block_rq_complete", "block_rq_insert", "block_rq_issue", "block_rq_remap",
		"file_check_and_advance_wb_err", "filemap_set_wb_err",
		"f2fs_direct_IO_enter", "f2fs_direct_IO_exit",
		"f2fs_sync_file_enter", "f2fs_sync_file_exit",
		"f2fs_write_begin", "f2fs_write_end",
		"mm_filemap_add_to_page_cache", "mm_filemap_delete_from_page_cache",
		"workqueue_execute_end", "workqueue_execute_start",
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
	names := append([]string{}, traceDBRawBlockTargetNames()...)
	names = append(names, traceDBRawExactRecoveryTargetNames()...)
	return append(names,
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
	)
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

func traceDBRawHMFSFormat(name string) bool {
	switch name {
	case "hmfs_readpage", "hmfs_readpages", "hmfs_sync", "hmfs_sync_exit",
		"hmfs_sync_file_enter", "hmfs_sync_file_exit", "hmfs_writepage",
		"hmfs_writepages":
		return true
	default:
		return false
	}
}

func traceDBRawHMFSPrintFormatWitnesses(catalog eventFormatCatalog) []string {
	formats := make([]eventFormat, 0, 8)
	for _, format := range catalog.Formats {
		if traceDBRawHMFSFormat(format.Name) {
			formats = append(formats, format)
		}
	}
	sort.Slice(formats, func(i, j int) bool {
		if formats[i].Name != formats[j].Name {
			return formats[i].Name < formats[j].Name
		}
		return formats[i].ID < formats[j].ID
	})
	out := make([]string, 0, len(formats))
	for _, format := range formats {
		value := format.PrintFmt
		if len(value) > 4096 {
			value = ""
		}
		out = append(out, fmt.Sprintf("%s#%d/bytes=%d/base64=%s",
			traceDBRawDecodeFormatWitnessName(format.Name), format.ID,
			len(format.PrintFmt), base64.RawStdEncoding.EncodeToString([]byte(value))))
	}
	return out
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
