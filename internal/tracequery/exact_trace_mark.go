package tracequery

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

const exactTraceMarkPrefix = "# codrax_trace_mark_exact/v1"
const exactTraceMarkOriginPrefix = "# codrax_trace_mark_exact/v2"

// ExactTraceMark losslessly carries marker text or SQL source-name provenance
// that the delimiter-based tracing_mark_write grammar cannot express. It does
// not upgrade the source adapter's timestamp/CPU/identity admission. Generic
// ftrace readers ignore the comment; Codrax reconstructs the typed event.
type ExactTraceMark struct {
	TimestampNS uint64
	CPU         int
	TID         int
	TGID        int
	SpanPID     int
	Action      string
	Comm        string
	Name        string
	Value       string
	NameOrigin  *tracewire.MarkerNameOrigin
}

// FormatExactTraceMark is the sole formatter for the closed exact marker wire.
func FormatExactTraceMark(mark ExactTraceMark) (string, error) {
	if !validExactTraceMark(mark) {
		return "", fmt.Errorf("invalid exact trace mark")
	}
	prefix, suffix := exactTraceMarkPrefix, ""
	if mark.NameOrigin != nil {
		payload, err := tracewire.EncodeMarkerNameOrigin(*mark.NameOrigin)
		if err != nil {
			return "", err
		}
		prefix, suffix = exactTraceMarkOriginPrefix, " name_origin="+payload
	}
	return strings.Join([]string{
		prefix,
		"ts_ns=" + strconv.FormatUint(mark.TimestampNS, 10),
		"cpu=" + strconv.Itoa(mark.CPU),
		"tid=" + strconv.Itoa(mark.TID),
		"tgid=" + strconv.Itoa(mark.TGID),
		"span_pid=" + strconv.Itoa(mark.SpanPID),
		"action=" + mark.Action,
		"comm=" + encodeCPUUnavailableTraceMarkText(mark.Comm),
		"name=" + encodeCPUUnavailableTraceMarkText(mark.Name),
		"value=" + encodeCPUUnavailableTraceMarkText(mark.Value),
	}, " ") + suffix, nil
}

func parseExactTraceMark(line string) (ExactTraceMark, bool) {
	v2 := strings.HasPrefix(line, exactTraceMarkOriginPrefix+" ")
	if !v2 && !strings.HasPrefix(line, exactTraceMarkPrefix+" ") {
		return ExactTraceMark{}, false
	}
	var origin *tracewire.MarkerNameOrigin
	if v2 {
		base, payload, ok := strings.Cut(line, " name_origin=")
		if !ok {
			return ExactTraceMark{}, false
		}
		details, ok := tracewire.DecodeMarkerNameOrigin(payload)
		if !ok {
			return ExactTraceMark{}, false
		}
		origin = &details
		line = exactTraceMarkPrefix + strings.TrimPrefix(base, exactTraceMarkOriginPrefix)
	}
	parts := strings.Split(line, " ")
	if len(parts) != 11 || parts[0] != "#" || parts[1] != "codrax_trace_mark_exact/v1" {
		return ExactTraceMark{}, false
	}
	value := func(index int, prefix string) (string, bool) {
		if !strings.HasPrefix(parts[index], prefix) {
			return "", false
		}
		out := strings.TrimPrefix(parts[index], prefix)
		return out, out != ""
	}
	tsRaw, tsOK := value(2, "ts_ns=")
	cpuRaw, cpuOK := value(3, "cpu=")
	tidRaw, tidOK := value(4, "tid=")
	tgidRaw, tgidOK := value(5, "tgid=")
	spanPIDRaw, spanPIDOK := value(6, "span_pid=")
	action, actionOK := value(7, "action=")
	commRaw, commOK := value(8, "comm=")
	nameRaw, nameOK := value(9, "name=")
	valueRaw, markerValueOK := value(10, "value=")
	if !tsOK || !cpuOK || !tidOK || !tgidOK || !spanPIDOK || !actionOK ||
		!commOK || !nameOK || !markerValueOK {
		return ExactTraceMark{}, false
	}
	timestamp, err := strconv.ParseUint(tsRaw, 10, 64)
	if err != nil {
		return ExactTraceMark{}, false
	}
	cpu, err := strconv.ParseInt(cpuRaw, 10, 32)
	if err != nil {
		return ExactTraceMark{}, false
	}
	tid, err := strconv.ParseInt(tidRaw, 10, 32)
	if err != nil {
		return ExactTraceMark{}, false
	}
	tgid, err := strconv.ParseInt(tgidRaw, 10, 32)
	if err != nil {
		return ExactTraceMark{}, false
	}
	spanPID, err := strconv.ParseInt(spanPIDRaw, 10, 32)
	if err != nil {
		return ExactTraceMark{}, false
	}
	if strconv.FormatUint(timestamp, 10) != tsRaw ||
		strconv.Itoa(int(cpu)) != cpuRaw ||
		strconv.Itoa(int(tid)) != tidRaw ||
		strconv.Itoa(int(tgid)) != tgidRaw ||
		strconv.Itoa(int(spanPID)) != spanPIDRaw {
		return ExactTraceMark{}, false
	}
	comm, commOK := decodeCPUUnavailableTraceMarkText(commRaw)
	name, nameOK := decodeCPUUnavailableTraceMarkText(nameRaw)
	markerValue, valueOK := decodeCPUUnavailableTraceMarkText(valueRaw)
	if !commOK || !nameOK || !valueOK {
		return ExactTraceMark{}, false
	}
	mark := ExactTraceMark{
		TimestampNS: timestamp,
		CPU:         int(cpu),
		TID:         int(tid),
		TGID:        int(tgid),
		SpanPID:     int(spanPID),
		Action:      action,
		Comm:        comm,
		Name:        name,
		Value:       markerValue,
		NameOrigin:  origin,
	}
	return mark, validExactTraceMark(mark)
}

func validExactTraceMark(mark ExactTraceMark) bool {
	if mark.NameOrigin != nil {
		if _, err := tracewire.EncodeMarkerNameOrigin(*mark.NameOrigin); err != nil {
			return false
		}
		label := "AppStartup:startup"
		if n := mark.NameOrigin.Name.Name; n != nil && *n != "" {
			label = "AppStartup:" + *n
		}
		if mark.Action != "B" && mark.Action != "E" || mark.Action == "B" && mark.Name != label {
			return false
		}
	}
	if !validTraceCPUIndex(mark.CPU) || mark.TID <= 0 || mark.TGID <= 0 || mark.SpanPID <= 0 ||
		!validCPUUnavailableTraceMarkText(mark.Comm, false) ||
		!validCPUUnavailableTraceMarkText(mark.Name, true) ||
		!validCPUUnavailableTraceMarkText(mark.Value, true) {
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

func exactTraceMarkEvent(lineNo int, mark ExactTraceMark, intern *stringInterner) Event {
	fields := mark.Action + "|" + strconv.Itoa(mark.SpanPID)
	switch mark.Action {
	case "B":
		fields += "|" + mark.Name
	case "S", "F":
		fields += "|" + mark.Name + "|" + mark.Value
	}
	event := Event{
		Line:       lineNo,
		Ts:         float64(mark.TimestampNS) / 1e9,
		CPU:        mark.CPU,
		Type:       EventTraceMark,
		Name:       intern.intern("codrax_trace_mark_exact"),
		Comm:       intern.intern(mark.Comm),
		PID:        mark.TID,
		TGID:       mark.TGID,
		SpanAction: intern.intern(mark.Action),
		SpanPID:    mark.SpanPID,
		SpanName:   intern.intern(mark.Name),
		SpanValue:  intern.intern(mark.Value),
		FieldText:  intern.intern(fields),
	}
	if mark.NameOrigin != nil {
		event.PluginFields = &PluginFields{MarkerNameOrigin: mark.NameOrigin}
	}
	return event
}
