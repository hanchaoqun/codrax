package tracequery

import (
	"strings"
	"testing"
	"unsafe"
)

func TestOfficialSQLRelationsWireRoundTrip(t *testing.T) {
	frameCallstack := FrameCallstackRelation{TimestampNS: 11, FrameRow: 0, CallstackRow: 7}
	callstackLine, err := FormatFrameCallstackRelation(frameCallstack)
	if err != nil {
		t.Fatal(err)
	}
	frameGPU := FrameGPURelation{TimestampNS: 12, GPURow: 1, FrameRow: 0, DurationNS: 99}
	gpuLine, err := FormatFrameGPURelation(frameGPU)
	if err != nil {
		t.Fatal(err)
	}
	napi := PerfNAPIAsyncRelation{
		TimestampNS: 13, RowID: 2, CPU: 0, TID: 20, PID: 10,
		CallerCallchain: 3, CalleeCallchain: 4, PerfSample: 5,
		EventCount: 6, EventType: 7, TraceID: "0xabc|带 空格",
	}
	napiLine, err := FormatPerfNAPIAsyncRelation(napi)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{callstackLine, gpuLine, napiLine} {
		if strings.Contains(line, "[000]") {
			t.Fatalf("typed SQL relation fabricated a physical ftrace envelope: %q", line)
		}
	}
	interner := newStringInterner()
	callstackEvent, ok := ParseLine(1, callstackLine, interner)
	if !ok || callstackEvent.Type != EventFrameCallstack || callstackEvent.CPU != -1 ||
		callstackEvent.PluginFields == nil || callstackEvent.PluginFields.FrameCallstack == nil ||
		callstackEvent.PluginFields.FrameCallstack.FrameRow != 0 ||
		callstackEvent.PluginFields.FrameCallstack.CallstackRow != 7 {
		t.Fatalf("frame-callstack relation drifted: %+v", callstackEvent)
	}
	gpuEvent, ok := ParseLine(2, gpuLine, interner)
	if !ok || gpuEvent.Type != EventFrameGPU || gpuEvent.CPU != -1 ||
		gpuEvent.PluginFields == nil || gpuEvent.PluginFields.FrameGPU == nil ||
		gpuEvent.PluginFields.FrameGPU.DurationNS != 99 {
		t.Fatalf("frame-GPU relation drifted: %+v", gpuEvent)
	}
	napiEvent, ok := ParseLine(3, napiLine, interner)
	if !ok || napiEvent.Type != EventPerfNAPIAsync || napiEvent.CPU != 0 ||
		napiEvent.PID != 20 || napiEvent.TGID != 10 ||
		napiEvent.PluginFields == nil || napiEvent.PluginFields.PerfNAPIAsync == nil ||
		napiEvent.PluginFields.PerfNAPIAsync.TraceID != napi.TraceID ||
		napiEvent.PluginFields.PerfNAPIAsync.PerfSample != 5 {
		t.Fatalf("perf NAPI relation drifted: %+v", napiEvent)
	}
	for line, wantTS := range map[string]uint64{
		callstackLine: 11, gpuLine: 12, napiLine: 13,
	} {
		if got, ok := ParseLineTimestampNS(line); !ok || got != wantTS {
			t.Fatalf("timestamp for %q=(%d,%t), want (%d,true)", line, got, ok, wantTS)
		}
	}
	if got := eventSideTableBytes(&callstackEvent); got !=
		int64(unsafe.Sizeof(PluginFields{}))+int64(unsafe.Sizeof(FrameCallstackFields{})) {
		t.Fatalf("frame-callstack side-table bytes=%d", got)
	}
}

func TestOfficialSQLRelationsClosedWire(t *testing.T) {
	napi, err := FormatPerfNAPIAsyncRelation(PerfNAPIAsyncRelation{
		TimestampNS: 1, CPU: 0, TID: 1, PID: 1, TraceID: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		strings.Replace(napi, "ts_ns=1", "ts_ns=01", 1),
		strings.Replace(napi, "cpu=0", "cpu=-1", 1),
		strings.Replace(napi, "traceid_b64=eA", "traceid_b64=eA==", 1),
		napi + " extra=x",
		strings.Replace(napi, "/v1", "/v2", 1),
	} {
		if _, ok := ParseLine(1, line, newStringInterner()); ok {
			t.Fatalf("accepted non-canonical relation: %q", line)
		}
	}
}
