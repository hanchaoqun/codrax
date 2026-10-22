package tracequery

import (
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ClockSyncFields preserves one recorded clock-sync observation, not a clock
// transform. HeaderNS is decoded from the physical header, never Event.Ts.
// The producer samples the reference clock before writing the marker; neither
// simultaneity nor a measurement-error bound is recorded by this protocol.
type ClockSyncFields struct {
	HeaderNS    string `json:"header_ns"`
	Field       string `json:"field,omitempty"`
	RawValue    string `json:"raw_value,omitempty"`
	IssueReason string `json:"issue_reason,omitempty"`
}

const clockSyncPrefix = "trace_event_clock_sync: "

func attachClockSyncWitness(event *Event, headerTimestamp, payload string) {
	// Do not reinterpret embedded B/E labels, ordinary prose or a foreign event
	// family as a sync record. Event classification and duration lanes stay put.
	if event.Type != EventUnknown || (event.Name != "tracing_mark_write" && event.Name != "print") || !strings.HasPrefix(payload, clockSyncPrefix) {
		return
	}
	headerNS, ok := parseTraceTimestampNanoseconds(headerTimestamp)
	if !ok {
		return
	}
	witness := &ClockSyncFields{HeaderNS: strconv.FormatUint(headerNS, 10)}
	tail := strings.TrimPrefix(payload, clockSyncPrefix)
	if len(tail) > 128 {
		witness.IssueReason = "clock_sync_payload_exceeds_limit"
	} else {
		field, raw, found := strings.Cut(tail, "=")
		witness.RawValue = tail
		if found && (field == "realtime_ts" || field == "parent_ts") {
			witness.Field, witness.RawValue = field, raw
			valid := false
			if field == "realtime_ts" {
				v, err := strconv.ParseInt(raw, 10, 64)
				valid = err == nil && v >= 0 && strconv.FormatInt(v, 10) == raw
			} else {
				// Preserve the recorded decimal (including trailing zeroes); do
				// not parse it through the producer's lossy floating-point type.
				_, valid = parseTraceTimestampNanoseconds(raw)
			}
			if !valid {
				witness.IssueReason = "invalid_clock_sync_timestamp"
			}
		} else {
			witness.IssueReason = "unsupported_clock_sync_payload"
		}
	}
	if event.PluginFields == nil {
		event.PluginFields = &PluginFields{}
	}
	event.PluginFields.ClockSync = witness
}

func projectClockSyncWitness(w ClockSyncFields) *types.TraceEventSemantics {
	p := traceEventSemanticProjector{value: types.TraceEventSemantics{SchemaVersion: types.TraceEventSemanticsVersion}}
	p.known("clock_sync.protocol", "trace_event_clock_sync")
	p.known("clock_sync.trace_time_ns", w.HeaderNS)
	if w.RawValue != "" {
		p.known("clock_sync.raw_value", w.RawValue)
	}
	key := "clock_sync.reference_value"
	switch w.Field {
	case "realtime_ts":
		key = "clock_sync.realtime_ms"
		p.known("clock_sync.protocol_clock", "CLOCK_REALTIME")
		p.known("clock_sync.protocol_epoch", "Unix epoch")
	case "parent_ts":
		key = "clock_sync.parent_seconds"
		p.known("clock_sync.protocol_clock", "CLOCK_MONOTONIC")
		p.known("clock_sync.protocol_epoch", "unspecified monotonic origin; relationship to the trace clock not established")
	}
	if w.IssueReason != "" {
		p.unknown(key, "invalid", w.IssueReason)
	} else {
		p.known(key, w.RawValue)
	}
	p.unknown("clock_sync.reference_clock", "unavailable", "source_producer_identity_not_verified")
	p.unknown("clock_sync.reference_epoch", "unavailable", "source_producer_identity_not_verified")
	p.unknown("clock_sync.offset_ns", "unavailable", "recorded_pair_not_calibration")
	p.unknown("clock_sync.drift", "unavailable", "not_recorded")
	p.unknown("clock_sync.error_bound_ns", "unavailable", "not_recorded")
	p.unknown("clock_sync.valid_interval", "unavailable", "point_witness_only")
	p.known("clock_sync.mapping_status", "unmapped_observation_only")
	p.known("clock_sync.limitations", "HiTrace/atrace protocol interpretation, not verification of this file's producer identity: reference sampled before marker write, not simultaneous; REALTIME uses truncated milliseconds and parent_ts uses float-formatted seconds. Decimal fidelity is not clock accuracy. No cross-source alignment, interval conversion, drift, error bound or causal authority.")
	return p.finish()
}
