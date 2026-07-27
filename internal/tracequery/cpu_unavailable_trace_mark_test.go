package tracequery

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCPUUnavailableTraceMarkWireRoundTrip(t *testing.T) {
	mark := CPUUnavailableTraceMark{
		TimestampNS: 1_234_567_890,
		TID:         33410,
		TGID:        69326,
		SpanPID:     32788,
		Action:      "B",
		Comm:        "ss.hm.ugc.aweme",
		Name:        "H:Frame Work 空格",
		Reason:      TraceMarkCPUReasonUnknownStart,
	}
	line, err := FormatCPUUnavailableTraceMark(mark)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(line, "[000]") || !strings.HasPrefix(line, cpuUnavailableTraceMarkPrefix+" ") {
		t.Fatalf("wire row fabricated a CPU or lost its exact prefix: %q", line)
	}
	ts, ok := ParseLineTimestampNS(line)
	if !ok || ts != mark.TimestampNS {
		t.Fatalf("timestamp parse=(%d,%t) want (%d,true)", ts, ok, mark.TimestampNS)
	}
	ev, ok := ParseLine(7, line, newStringInterner())
	if !ok || ev.Type != EventTraceMark || ev.Line != 7 || ev.PID != mark.TID || ev.TGID != mark.TGID ||
		ev.SpanPID != mark.SpanPID || ev.SpanAction != "B" || ev.SpanName != mark.Name ||
		ev.PluginFields == nil || ev.PluginFields.TraceMarkerCPUStatus != TraceMarkCPUStatusUnavailable ||
		ev.PluginFields.TraceMarkerCPUReason != mark.Reason {
		t.Fatalf("typed marker round-trip drifted: %+v", ev)
	}
	if ev.CPU != 0 {
		t.Fatalf("unavailable CPU must remain unset, got %d", ev.CPU)
	}
}

func TestCPUUnavailableTraceMarkRejectsNonCanonicalOrOpenEndedWire(t *testing.T) {
	base, err := FormatCPUUnavailableTraceMark(CPUUnavailableTraceMark{
		TimestampNS: 10,
		TID:         11,
		TGID:        12,
		SpanPID:     13,
		Action:      "S",
		Comm:        "worker",
		Name:        "async",
		Value:       "42",
		Reason:      TraceMarkCPUReasonAliasAmbiguous,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		strings.Replace(base, "action=S", "action=X", 1),
		strings.Replace(base, "reason="+TraceMarkCPUReasonAliasAmbiguous, "reason=future_guess", 1),
		strings.Replace(base, "comm=d29ya2Vy", "comm=d29ya2Vy=", 1),
		strings.Replace(base, " ts_ns=10", "  ts_ns=10", 1),
		base + " extra=x",
		strings.Replace(base, "value=NDI", "value=~", 1),
	} {
		if _, ok := ParseLine(1, line, newStringInterner()); ok {
			t.Fatalf("accepted non-canonical wire row: %q", line)
		}
	}
	if _, err := FormatCPUUnavailableTraceMark(CPUUnavailableTraceMark{
		TimestampNS: 1, TID: 1, TGID: 1, SpanPID: 1, Action: "B",
		Comm: "bad\ncomm", Name: "span", Reason: TraceMarkCPUReasonUnknownEnd,
	}); err == nil {
		t.Fatal("formatter accepted a multi-line comm")
	}
}

func TestCPUUnavailableTraceMarkPreservesSpanAndTypedCaveat(t *testing.T) {
	begin, err := FormatCPUUnavailableTraceMark(CPUUnavailableTraceMark{
		TimestampNS: 1_000_000_000,
		TID:         33410,
		TGID:        69326,
		SpanPID:     32788,
		Action:      "B",
		Comm:        "ss.hm.ugc.aweme",
		Name:        "Frame Task",
		Reason:      TraceMarkCPUReasonLifecycleRejected,
	})
	if err != nil {
		t.Fatal(err)
	}
	end, err := FormatCPUUnavailableTraceMark(CPUUnavailableTraceMark{
		TimestampNS: 1_005_000_000,
		TID:         33410,
		TGID:        69326,
		SpanPID:     32788,
		Action:      "E",
		Comm:        "ss.hm.ugc.aweme",
		Reason:      TraceMarkCPUReasonLifecycleRejected,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cpu-unavailable.systrace")
	if err := os.WriteFile(path, []byte("# tracer: nop\n"+begin+"\n"+end+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	spans, _, caveats := computeTraceMarks(idx, Query{TimeStart: 0.9, TimeEnd: 1.1}, 16)
	if len(spans) != 1 || spans[0].Name != "Frame Task" || spans[0].SpanPID != 32788 ||
		math.Abs(spans[0].DurationMs-5) > 1e-9 || spans[0].CPUStatus != TraceMarkCPUStatusUnavailable ||
		spans[0].CPUReason != TraceMarkCPUReasonLifecycleRejected {
		t.Fatalf("typed unavailable span was not preserved: spans=%+v caveats=%+v", spans, caveats)
	}
	joined := strings.Join(caveats, "\n")
	if !strings.Contains(joined, "trace_span_cpu_status=unavailable spans=1") ||
		!strings.Contains(joined, "must not be used for per-CPU attribution") {
		t.Fatalf("missing typed CPU caveat: %v", caveats)
	}
}
