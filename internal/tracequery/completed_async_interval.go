package tracequery

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const completedAsyncIntervalPrefix = "# codrax_trace_async_interval/v1"

const TraceAsyncIntervalCPUStatusKnown = "known"

// CompletedAsyncInterval is one converter-authored copy of an official
// TraceStreamer callstack (ts,dur,cookie) row. It asserts a completed logical
// interval and the source emitter at the start only. No finish emitter or
// finish CPU is present in this wire, so neither can be inferred by consumers.
type CompletedAsyncInterval struct {
	StartTimestampNS uint64
	EndTimestampNS   uint64
	SourceRow        uint64
	TID              int
	TGID             int
	SpanPID          int
	StartCPU         int
	StartCPUStatus   string
	StartCPUReason   string
	Comm             string
	Name             string
	Cookie           string
}

// FormatCompletedAsyncInterval is the sole formatter for the closed completed
// async interval wire.
func FormatCompletedAsyncInterval(interval CompletedAsyncInterval) (string, error) {
	if !validCompletedAsyncInterval(interval) {
		return "", fmt.Errorf("invalid completed async interval")
	}
	startCPU := cpuUnavailableTraceMarkEmptyTextWire
	cpuReason := cpuUnavailableTraceMarkEmptyTextWire
	if interval.StartCPUStatus == TraceAsyncIntervalCPUStatusKnown {
		startCPU = strconv.Itoa(interval.StartCPU)
	} else {
		cpuReason = interval.StartCPUReason
	}
	return strings.Join([]string{
		completedAsyncIntervalPrefix,
		"ts_ns=" + strconv.FormatUint(interval.StartTimestampNS, 10),
		"end_ns=" + strconv.FormatUint(interval.EndTimestampNS, 10),
		"source_row=" + strconv.FormatUint(uint64(interval.SourceRow), 10),
		"tid=" + strconv.Itoa(interval.TID),
		"tgid=" + strconv.Itoa(interval.TGID),
		"span_pid=" + strconv.Itoa(interval.SpanPID),
		"start_cpu=" + startCPU,
		"cpu_reason=" + cpuReason,
		"comm=" + encodeCPUUnavailableTraceMarkText(interval.Comm),
		"name=" + encodeCPUUnavailableTraceMarkText(interval.Name),
		"cookie=" + encodeCPUUnavailableTraceMarkText(interval.Cookie),
	}, " "), nil
}

func parseCompletedAsyncInterval(line string) (CompletedAsyncInterval, bool) {
	if !strings.HasPrefix(line, completedAsyncIntervalPrefix+" ") {
		return CompletedAsyncInterval{}, false
	}
	parts := strings.Split(line, " ")
	if len(parts) != 13 || parts[0] != "#" || parts[1] != "codrax_trace_async_interval/v1" {
		return CompletedAsyncInterval{}, false
	}
	value := func(index int, prefix string) (string, bool) {
		if !strings.HasPrefix(parts[index], prefix) {
			return "", false
		}
		out := strings.TrimPrefix(parts[index], prefix)
		return out, out != ""
	}
	startRaw, startOK := value(2, "ts_ns=")
	endRaw, endOK := value(3, "end_ns=")
	sourceRaw, sourceOK := value(4, "source_row=")
	tidRaw, tidOK := value(5, "tid=")
	tgidRaw, tgidOK := value(6, "tgid=")
	spanPIDRaw, spanPIDOK := value(7, "span_pid=")
	startCPURaw, startCPUOK := value(8, "start_cpu=")
	cpuReasonRaw, cpuReasonOK := value(9, "cpu_reason=")
	commRaw, commOK := value(10, "comm=")
	nameRaw, nameOK := value(11, "name=")
	cookieRaw, cookieOK := value(12, "cookie=")
	if !startOK || !endOK || !sourceOK || !tidOK || !tgidOK || !spanPIDOK ||
		!startCPUOK || !cpuReasonOK || !commOK || !nameOK || !cookieOK {
		return CompletedAsyncInterval{}, false
	}
	start, err := strconv.ParseUint(startRaw, 10, 64)
	if err != nil || strconv.FormatUint(start, 10) != startRaw {
		return CompletedAsyncInterval{}, false
	}
	end, err := strconv.ParseUint(endRaw, 10, 64)
	if err != nil || strconv.FormatUint(end, 10) != endRaw {
		return CompletedAsyncInterval{}, false
	}
	sourceRow, err := strconv.ParseUint(sourceRaw, 10, 64)
	if err != nil || strconv.FormatUint(sourceRow, 10) != sourceRaw {
		return CompletedAsyncInterval{}, false
	}
	tid, err := strconv.ParseInt(tidRaw, 10, 32)
	if err != nil || strconv.Itoa(int(tid)) != tidRaw {
		return CompletedAsyncInterval{}, false
	}
	tgid, err := strconv.ParseInt(tgidRaw, 10, 32)
	if err != nil || strconv.Itoa(int(tgid)) != tgidRaw {
		return CompletedAsyncInterval{}, false
	}
	spanPID, err := strconv.ParseInt(spanPIDRaw, 10, 32)
	if err != nil || strconv.Itoa(int(spanPID)) != spanPIDRaw {
		return CompletedAsyncInterval{}, false
	}
	startCPU := -1
	cpuStatus, cpuReason := TraceMarkCPUStatusUnavailable, cpuReasonRaw
	if startCPURaw != cpuUnavailableTraceMarkEmptyTextWire {
		cpu, err := strconv.ParseInt(startCPURaw, 10, 32)
		if err != nil || strconv.Itoa(int(cpu)) != startCPURaw ||
			cpuReasonRaw != cpuUnavailableTraceMarkEmptyTextWire {
			return CompletedAsyncInterval{}, false
		}
		startCPU, cpuStatus, cpuReason = int(cpu), TraceAsyncIntervalCPUStatusKnown, ""
	} else if cpuReasonRaw == cpuUnavailableTraceMarkEmptyTextWire {
		return CompletedAsyncInterval{}, false
	}
	comm, commOK := decodeCPUUnavailableTraceMarkText(commRaw)
	name, nameOK := decodeCPUUnavailableTraceMarkText(nameRaw)
	cookie, cookieOK := decodeCPUUnavailableTraceMarkText(cookieRaw)
	if !commOK || !nameOK || !cookieOK {
		return CompletedAsyncInterval{}, false
	}
	interval := CompletedAsyncInterval{
		StartTimestampNS: start,
		EndTimestampNS:   end,
		SourceRow:        sourceRow,
		TID:              int(tid),
		TGID:             int(tgid),
		SpanPID:          int(spanPID),
		StartCPU:         startCPU,
		StartCPUStatus:   cpuStatus,
		StartCPUReason:   cpuReason,
		Comm:             comm,
		Name:             name,
		Cookie:           cookie,
	}
	return interval, validCompletedAsyncInterval(interval)
}

func validCompletedAsyncInterval(interval CompletedAsyncInterval) bool {
	if interval.EndTimestampNS < interval.StartTimestampNS ||
		interval.SourceRow == 0 ||
		interval.TID <= 0 || interval.TID > math.MaxInt32 ||
		interval.TGID <= 0 || interval.TGID > math.MaxInt32 ||
		interval.SpanPID <= 0 || interval.SpanPID > math.MaxInt32 ||
		!validCPUUnavailableTraceMarkText(interval.Comm, false) ||
		!validCPUUnavailableTraceMarkText(interval.Name, false) ||
		!validCPUUnavailableTraceMarkText(interval.Cookie, false) {
		return false
	}
	switch interval.StartCPUStatus {
	case TraceAsyncIntervalCPUStatusKnown:
		return validTraceCPUIndex(interval.StartCPU) && interval.StartCPUReason == ""
	case TraceMarkCPUStatusUnavailable:
		return interval.StartCPU == -1 && validCPUUnavailableTraceMarkReason(interval.StartCPUReason)
	default:
		return false
	}
}

func completedAsyncIntervalEvent(lineNo int, interval CompletedAsyncInterval, intern *stringInterner) Event {
	cpu := interval.StartCPU
	plugin := &PluginFields{
		AsyncInterval: &TraceAsyncIntervalFields{
			SourceRow:           interval.SourceRow,
			StartTimestampNS:    interval.StartTimestampNS,
			EndTimestampNS:      interval.EndTimestampNS,
			StartCPUStatus:      intern.intern(interval.StartCPUStatus),
			StartCPUReason:      intern.intern(interval.StartCPUReason),
			FinishEmitterStatus: "unavailable",
			FinishCPUStatus:     "unavailable",
		},
	}
	if interval.StartCPUStatus == TraceMarkCPUStatusUnavailable {
		cpu = -1
		plugin.TraceMarkerCPUStatus = TraceMarkCPUStatusUnavailable
		plugin.TraceMarkerCPUReason = intern.intern(interval.StartCPUReason)
	}
	return Event{
		Line:         lineNo,
		Ts:           float64(interval.StartTimestampNS) / 1e9,
		CPU:          cpu,
		Type:         EventTraceAsyncInterval,
		Name:         intern.intern("codrax_trace_async_interval"),
		Comm:         intern.intern(interval.Comm),
		PID:          interval.TID,
		TGID:         interval.TGID,
		SpanPID:      interval.SpanPID,
		SpanName:     intern.intern(interval.Name),
		SpanValue:    intern.intern(interval.Cookie),
		PluginFields: plugin,
		FieldText: intern.intern(fmt.Sprintf(
			"source_row=%d end_ts_ns=%d cookie=%s finish_emitter_status=unavailable finish_cpu_status=unavailable",
			interval.SourceRow, interval.EndTimestampNS, interval.Cookie,
		)),
	}
}
