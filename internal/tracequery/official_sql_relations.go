package tracequery

import (
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	frameCallstackRelationPrefix = "# codrax_frame_callstack/v1"
	frameGPURelationPrefix       = "# codrax_frame_gpu/v1"
	perfNAPIAsyncRelationPrefix  = "# codrax_perf_napi_async/v1"
)

// FrameCallstackRelation preserves the official frame_slice.callstack_id
// foreign-key relation. TimestampNS is the frame start; it is only a stable
// time anchor and does not turn this edge into a CPU/thread interval.
type FrameCallstackRelation struct {
	TimestampNS  uint64
	FrameRow     uint32
	CallstackRow uint64
}

// FrameGPURelation preserves one official gpu_slice row. The producer exposes
// frame_row and dur but no GPU start timestamp, CPU, or thread. TimestampNS is
// therefore the referenced frame start only; DurationNS is resource duration,
// never an interval beginning at TimestampNS.
type FrameGPURelation struct {
	TimestampNS uint64
	GPURow      uint32
	FrameRow    uint32
	DurationNS  uint64
}

// PerfNAPIAsyncRelation preserves one official perf_napi_async sample
// correlation. It is a point observation: the producer has no duration.
type PerfNAPIAsyncRelation struct {
	TimestampNS     uint64
	RowID           uint32
	CPU             int
	TID             int
	PID             int
	CallerCallchain uint32
	CalleeCallchain uint32
	PerfSample      uint32
	EventCount      uint64
	EventType       uint64
	TraceID         string
}

func FormatFrameCallstackRelation(relation FrameCallstackRelation) (string, error) {
	return strings.Join([]string{
		frameCallstackRelationPrefix,
		"ts_ns=" + strconv.FormatUint(relation.TimestampNS, 10),
		"frame_row=" + strconv.FormatUint(uint64(relation.FrameRow), 10),
		"callstack_row=" + strconv.FormatUint(relation.CallstackRow, 10),
	}, " "), nil
}

func FormatFrameGPURelation(relation FrameGPURelation) (string, error) {
	return strings.Join([]string{
		frameGPURelationPrefix,
		"ts_ns=" + strconv.FormatUint(relation.TimestampNS, 10),
		"gpu_row=" + strconv.FormatUint(uint64(relation.GPURow), 10),
		"frame_row=" + strconv.FormatUint(uint64(relation.FrameRow), 10),
		"duration_ns=" + strconv.FormatUint(relation.DurationNS, 10),
	}, " "), nil
}

func FormatPerfNAPIAsyncRelation(relation PerfNAPIAsyncRelation) (string, error) {
	if relation.CPU < 0 || relation.CPU > 4095 ||
		relation.TID <= 0 || relation.TID > math.MaxInt32 ||
		relation.PID <= 0 || relation.PID > math.MaxInt32 ||
		relation.TraceID == "" {
		return "", fmt.Errorf("invalid perf-napi-async relation")
	}
	return strings.Join([]string{
		perfNAPIAsyncRelationPrefix,
		"ts_ns=" + strconv.FormatUint(relation.TimestampNS, 10),
		"row_id=" + strconv.FormatUint(uint64(relation.RowID), 10),
		"cpu=" + strconv.Itoa(relation.CPU),
		"tid=" + strconv.Itoa(relation.TID),
		"pid=" + strconv.Itoa(relation.PID),
		"caller_callchain=" + strconv.FormatUint(uint64(relation.CallerCallchain), 10),
		"callee_callchain=" + strconv.FormatUint(uint64(relation.CalleeCallchain), 10),
		"perf_sample=" + strconv.FormatUint(uint64(relation.PerfSample), 10),
		"event_count=" + strconv.FormatUint(relation.EventCount, 10),
		"event_type=" + strconv.FormatUint(relation.EventType, 10),
		"traceid_b64=" + base64.RawURLEncoding.EncodeToString([]byte(relation.TraceID)),
	}, " "), nil
}

func parseFrameCallstackRelation(line string) (FrameCallstackRelation, bool) {
	parts, ok := exactRelationParts(line, frameCallstackRelationPrefix, 5)
	if !ok {
		return FrameCallstackRelation{}, false
	}
	ts, ok := relationUint(parts[2], "ts_ns=", 64)
	if !ok {
		return FrameCallstackRelation{}, false
	}
	frame, ok := relationUint(parts[3], "frame_row=", 32)
	if !ok {
		return FrameCallstackRelation{}, false
	}
	callstack, ok := relationUint(parts[4], "callstack_row=", 64)
	if !ok {
		return FrameCallstackRelation{}, false
	}
	return FrameCallstackRelation{TimestampNS: ts, FrameRow: uint32(frame), CallstackRow: callstack}, true
}

func parseFrameGPURelation(line string) (FrameGPURelation, bool) {
	parts, ok := exactRelationParts(line, frameGPURelationPrefix, 6)
	if !ok {
		return FrameGPURelation{}, false
	}
	ts, ok := relationUint(parts[2], "ts_ns=", 64)
	if !ok {
		return FrameGPURelation{}, false
	}
	gpu, ok := relationUint(parts[3], "gpu_row=", 32)
	if !ok {
		return FrameGPURelation{}, false
	}
	frame, ok := relationUint(parts[4], "frame_row=", 32)
	if !ok {
		return FrameGPURelation{}, false
	}
	duration, ok := relationUint(parts[5], "duration_ns=", 64)
	if !ok {
		return FrameGPURelation{}, false
	}
	return FrameGPURelation{TimestampNS: ts, GPURow: uint32(gpu), FrameRow: uint32(frame), DurationNS: duration}, true
}

func parsePerfNAPIAsyncRelation(line string) (PerfNAPIAsyncRelation, bool) {
	parts, ok := exactRelationParts(line, perfNAPIAsyncRelationPrefix, 13)
	if !ok {
		return PerfNAPIAsyncRelation{}, false
	}
	values := make([]uint64, 8)
	spec := []struct {
		index int
		key   string
		bits  int
	}{
		{2, "ts_ns=", 64},
		{3, "row_id=", 32},
		{7, "caller_callchain=", 32},
		{8, "callee_callchain=", 32},
		{9, "perf_sample=", 32},
		{10, "event_count=", 64},
		{11, "event_type=", 64},
	}
	for i, item := range spec {
		value, valid := relationUint(parts[item.index], item.key, item.bits)
		if !valid {
			return PerfNAPIAsyncRelation{}, false
		}
		values[i] = value
	}
	cpu, ok := relationInt(parts[4], "cpu=", 0, 4095)
	if !ok {
		return PerfNAPIAsyncRelation{}, false
	}
	tid, ok := relationInt(parts[5], "tid=", 1, math.MaxInt32)
	if !ok {
		return PerfNAPIAsyncRelation{}, false
	}
	pid, ok := relationInt(parts[6], "pid=", 1, math.MaxInt32)
	if !ok {
		return PerfNAPIAsyncRelation{}, false
	}
	if !strings.HasPrefix(parts[12], "traceid_b64=") {
		return PerfNAPIAsyncRelation{}, false
	}
	traceBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(parts[12], "traceid_b64="))
	if err != nil || len(traceBytes) == 0 {
		return PerfNAPIAsyncRelation{}, false
	}
	traceID := string(traceBytes)
	relation := PerfNAPIAsyncRelation{
		TimestampNS: values[0], RowID: uint32(values[1]), CPU: cpu, TID: tid, PID: pid,
		CallerCallchain: uint32(values[2]), CalleeCallchain: uint32(values[3]),
		PerfSample: uint32(values[4]), EventCount: values[5], EventType: values[6], TraceID: traceID,
	}
	roundTrip, err := FormatPerfNAPIAsyncRelation(relation)
	return relation, err == nil && roundTrip == line
}

func exactRelationParts(line, prefix string, count int) ([]string, bool) {
	if !strings.HasPrefix(line, prefix+" ") {
		return nil, false
	}
	parts := strings.Split(line, " ")
	return parts, len(parts) == count && parts[0] == "#" && "# "+parts[1] == prefix
}

func relationUint(field, prefix string, bits int) (uint64, bool) {
	if !strings.HasPrefix(field, prefix) {
		return 0, false
	}
	raw := strings.TrimPrefix(field, prefix)
	if raw == "" || (len(raw) > 1 && raw[0] == '0') {
		return 0, false
	}
	value, err := strconv.ParseUint(raw, 10, bits)
	return value, err == nil && strconv.FormatUint(value, 10) == raw
}

func relationInt(field, prefix string, min, max int) (int, bool) {
	value, ok := relationUint(field, prefix, 31)
	if !ok || value < uint64(min) || value > uint64(max) {
		return 0, false
	}
	return int(value), true
}

func frameCallstackRelationEvent(lineNo int, relation FrameCallstackRelation, intern *stringInterner) Event {
	return Event{
		Line: lineNo, Ts: float64(relation.TimestampNS) / 1e9, CPU: -1,
		Type: EventFrameCallstack, Name: intern.intern("codrax_frame_callstack"),
		PluginFields: &PluginFields{FrameCallstack: &FrameCallstackFields{
			TimestampNS: relation.TimestampNS, FrameRow: relation.FrameRow, CallstackRow: relation.CallstackRow,
		}},
		FieldText: intern.intern(fmt.Sprintf("frame_row=%d callstack_row=%d", relation.FrameRow, relation.CallstackRow)),
	}
}

func frameGPURelationEvent(lineNo int, relation FrameGPURelation, intern *stringInterner) Event {
	return Event{
		Line: lineNo, Ts: float64(relation.TimestampNS) / 1e9, CPU: -1,
		Type: EventFrameGPU, Name: intern.intern("codrax_frame_gpu"),
		PluginFields: &PluginFields{FrameGPU: &FrameGPUFields{
			TimestampNS: relation.TimestampNS, GPURow: relation.GPURow,
			FrameRow: relation.FrameRow, DurationNS: relation.DurationNS,
		}},
		FieldText: intern.intern(fmt.Sprintf(
			"gpu_row=%d frame_row=%d duration_ns=%d frame_timestamp_anchor_only=true",
			relation.GPURow, relation.FrameRow, relation.DurationNS,
		)),
	}
}

func perfNAPIAsyncRelationEvent(lineNo int, relation PerfNAPIAsyncRelation, intern *stringInterner) Event {
	return Event{
		Line: lineNo, Ts: float64(relation.TimestampNS) / 1e9, CPU: relation.CPU,
		Type: EventPerfNAPIAsync, Name: intern.intern("codrax_perf_napi_async"),
		Comm: intern.intern("perf-napi-async"), PID: relation.TID, TGID: relation.PID,
		PluginFields: &PluginFields{PerfNAPIAsync: &PerfNAPIAsyncFields{
			TimestampNS: relation.TimestampNS, RowID: relation.RowID, CPU: relation.CPU,
			TID: relation.TID, PID: relation.PID, CallerCallchain: relation.CallerCallchain,
			CalleeCallchain: relation.CalleeCallchain, PerfSample: relation.PerfSample,
			EventCount: relation.EventCount, EventType: relation.EventType, TraceID: intern.intern(relation.TraceID),
		}},
		FieldText: intern.intern(fmt.Sprintf(
			"traceid=%s row_id=%d caller_callchain=%d callee_callchain=%d perf_sample=%d event_count=%d event_type=%d point_only=true",
			relation.TraceID, relation.RowID, relation.CallerCallchain, relation.CalleeCallchain,
			relation.PerfSample, relation.EventCount, relation.EventType,
		)),
	}
}
