package tracequery

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const cpuUnavailableTraceMarkPrefix = "# codrax_trace_mark_cpu_unavailable/v1"

const (
	TraceMarkCPUStatusUnavailable = "unavailable"

	TraceMarkCPUReasonUnknownStart       = "unknown_start_cpu"
	TraceMarkCPUReasonUnknownEnd         = "unknown_end_cpu"
	TraceMarkCPUReasonSourceTainted      = "tainted_running_cpu_witness"
	TraceMarkCPUReasonLifecycleRejected  = "lifecycle_rejected_running_cpu_witness"
	TraceMarkCPUReasonAliasAmbiguous     = "ambiguous_same_public_tid_scheduler_alias"
	maxCPUUnavailableTraceMarkTextBytes  = 4096
	cpuUnavailableTraceMarkEmptyTextWire = "~"
)

type CPUUnavailableTraceMark struct {
	TimestampNS uint64
	TID         int
	TGID        int
	SpanPID     int
	Action      string
	Comm        string
	Name        string
	Value       string
	Reason      string
}

// FormatCPUUnavailableTraceMark is the sole wire formatter for a marker whose
// callstack identity and timestamps are proven but whose CPU placement is not.
// It is a comment to generic ftrace readers, so no concrete CPU is fabricated;
// Codrax trace_query parses the exact versioned record as typed trace_mark
// evidence with cpu_status=unavailable.
func FormatCPUUnavailableTraceMark(mark CPUUnavailableTraceMark) (string, error) {
	if !validCPUUnavailableTraceMark(mark) {
		return "", fmt.Errorf("invalid cpu-unavailable trace mark")
	}
	return strings.Join([]string{
		cpuUnavailableTraceMarkPrefix,
		"ts_ns=" + strconv.FormatUint(mark.TimestampNS, 10),
		"tid=" + strconv.Itoa(mark.TID),
		"tgid=" + strconv.Itoa(mark.TGID),
		"span_pid=" + strconv.Itoa(mark.SpanPID),
		"action=" + mark.Action,
		"comm=" + encodeCPUUnavailableTraceMarkText(mark.Comm),
		"name=" + encodeCPUUnavailableTraceMarkText(mark.Name),
		"value=" + encodeCPUUnavailableTraceMarkText(mark.Value),
		"reason=" + mark.Reason,
	}, " "), nil
}

func parseCPUUnavailableTraceMark(line string) (CPUUnavailableTraceMark, bool) {
	// This parser is consulted before the common ftrace envelope on every
	// physical row. Reject ordinary rows without Split so the rare typed lane
	// adds zero per-event allocations to scheduler/perf/profiler traces.
	if !strings.HasPrefix(line, cpuUnavailableTraceMarkPrefix+" ") {
		return CPUUnavailableTraceMark{}, false
	}
	parts := strings.Split(line, " ")
	if len(parts) != 11 || parts[0] != "#" || parts[1] != "codrax_trace_mark_cpu_unavailable/v1" {
		return CPUUnavailableTraceMark{}, false
	}
	value := func(index int, prefix string) (string, bool) {
		if !strings.HasPrefix(parts[index], prefix) {
			return "", false
		}
		out := strings.TrimPrefix(parts[index], prefix)
		return out, out != ""
	}
	tsRaw, tsOK := value(2, "ts_ns=")
	tidRaw, tidOK := value(3, "tid=")
	tgidRaw, tgidOK := value(4, "tgid=")
	spanPIDRaw, spanPIDOK := value(5, "span_pid=")
	action, actionOK := value(6, "action=")
	commRaw, commOK := value(7, "comm=")
	nameRaw, nameOK := value(8, "name=")
	valueRaw, valueOK := value(9, "value=")
	reason, reasonOK := value(10, "reason=")
	if !tsOK || !tidOK || !tgidOK || !spanPIDOK || !actionOK || !commOK || !nameOK || !valueOK || !reasonOK {
		return CPUUnavailableTraceMark{}, false
	}
	timestamp, err := strconv.ParseUint(tsRaw, 10, 64)
	if err != nil {
		return CPUUnavailableTraceMark{}, false
	}
	tid, err := strconv.ParseInt(tidRaw, 10, 32)
	if err != nil {
		return CPUUnavailableTraceMark{}, false
	}
	tgid, err := strconv.ParseInt(tgidRaw, 10, 32)
	if err != nil {
		return CPUUnavailableTraceMark{}, false
	}
	spanPID, err := strconv.ParseInt(spanPIDRaw, 10, 32)
	if err != nil {
		return CPUUnavailableTraceMark{}, false
	}
	comm, commOK := decodeCPUUnavailableTraceMarkText(commRaw)
	name, nameOK := decodeCPUUnavailableTraceMarkText(nameRaw)
	markerValue, markerValueOK := decodeCPUUnavailableTraceMarkText(valueRaw)
	if !commOK || !nameOK || !markerValueOK {
		return CPUUnavailableTraceMark{}, false
	}
	mark := CPUUnavailableTraceMark{
		TimestampNS: timestamp,
		TID:         int(tid),
		TGID:        int(tgid),
		SpanPID:     int(spanPID),
		Action:      action,
		Comm:        comm,
		Name:        name,
		Value:       markerValue,
		Reason:      reason,
	}
	return mark, validCPUUnavailableTraceMark(mark)
}

func validCPUUnavailableTraceMark(mark CPUUnavailableTraceMark) bool {
	if mark.TID <= 0 || mark.TGID <= 0 || mark.SpanPID <= 0 ||
		!validCPUUnavailableTraceMarkText(mark.Comm, false) ||
		!validCPUUnavailableTraceMarkText(mark.Name, true) ||
		!validCPUUnavailableTraceMarkText(mark.Value, true) ||
		!validCPUUnavailableTraceMarkReason(mark.Reason) {
		return false
	}
	switch mark.Action {
	case "B":
		return mark.Name != "" && mark.Value == ""
	case "E":
		return mark.Name == "" && mark.Value == ""
	case "S", "F":
		return mark.Name != "" && mark.Value != ""
	default:
		return false
	}
}

func validCPUUnavailableTraceMarkReason(reason string) bool {
	switch reason {
	case TraceMarkCPUReasonUnknownStart,
		TraceMarkCPUReasonUnknownEnd,
		TraceMarkCPUReasonSourceTainted,
		TraceMarkCPUReasonLifecycleRejected,
		TraceMarkCPUReasonAliasAmbiguous:
		return true
	default:
		return false
	}
}

func validCPUUnavailableTraceMarkText(value string, allowEmpty bool) bool {
	if (!allowEmpty && value == "") || len(value) > maxCPUUnavailableTraceMarkTextBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return false
		}
	}
	return true
}

func encodeCPUUnavailableTraceMarkText(value string) string {
	if value == "" {
		return cpuUnavailableTraceMarkEmptyTextWire
	}
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeCPUUnavailableTraceMarkText(value string) (string, bool) {
	if value == cpuUnavailableTraceMarkEmptyTextWire {
		return "", true
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return "", false
	}
	return string(decoded), true
}

func cpuUnavailableTraceMarkEvent(lineNo int, mark CPUUnavailableTraceMark, intern *stringInterner) Event {
	fields := mark.Action + "|" + strconv.Itoa(mark.SpanPID)
	switch mark.Action {
	case "B":
		fields += "|" + mark.Name
	case "S", "F":
		fields += "|" + mark.Name + "|" + mark.Value
	}
	return Event{
		Line:       lineNo,
		Ts:         float64(mark.TimestampNS) / 1e9,
		Type:       EventTraceMark,
		Name:       intern.intern("codrax_trace_mark_cpu_unavailable"),
		Comm:       intern.intern(mark.Comm),
		PID:        mark.TID,
		TGID:       mark.TGID,
		SpanAction: intern.intern(mark.Action),
		SpanPID:    mark.SpanPID,
		SpanName:   intern.intern(mark.Name),
		SpanValue:  intern.intern(mark.Value),
		PluginFields: &PluginFields{
			TraceMarkerCPUStatus: TraceMarkCPUStatusUnavailable,
			TraceMarkerCPUReason: intern.intern(mark.Reason),
		},
		FieldText: intern.intern(fields),
	}
}
