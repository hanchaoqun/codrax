package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletedAsyncIntervalWireAndTraceSpanProjection(t *testing.T) {
	line, err := FormatCompletedAsyncInterval(CompletedAsyncInterval{
		StartTimestampNS: 1_000_000_123,
		EndTimestampNS:   1_005_000_456,
		SourceRow:        77,
		TID:              17267,
		TGID:             17267,
		SpanPID:          37722,
		StartCPU:         3,
		StartCPUStatus:   TraceAsyncIntervalCPUStatusKnown,
		Comm:             ".ugc.aweme.lite",
		Name:             "async|pipeline",
		Cookie:           "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	event, ok := ParseLine(7, line, newStringInterner())
	if !ok || event.Type != EventTraceAsyncInterval || event.CPU != 3 ||
		event.PID != 17267 || event.TGID != 17267 || event.SpanPID != 37722 ||
		event.SpanName != "async|pipeline" || event.SpanValue != "0" ||
		event.PluginFields == nil || event.PluginFields.AsyncInterval == nil {
		t.Fatalf("completed interval did not reconstruct: ok=%t event=%+v", ok, event)
	}
	typed := event.PluginFields.AsyncInterval
	if typed.SourceRow != 77 || typed.StartTimestampNS != 1_000_000_123 ||
		typed.EndTimestampNS != 1_005_000_456 ||
		typed.StartCPUStatus != TraceAsyncIntervalCPUStatusKnown ||
		typed.FinishEmitterStatus != "unavailable" || typed.FinishCPUStatus != "unavailable" {
		t.Fatalf("typed interval authority drifted: %+v", typed)
	}
	if ts, ok := ParseLineTimestampNS(line); !ok || ts != 1_000_000_123 {
		t.Fatalf("exact interval timestamp=%d ok=%t", ts, ok)
	}

	path := filepath.Join(t.TempDir(), "completed-async.systrace")
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	spans, _, caveats := computeTraceMarks(idx, Query{TimeStart: 1.002, TimeEnd: 1.004}, 8)
	if len(spans) != 1 {
		t.Fatalf("typed interval missing from trace spans: spans=%+v caveats=%v", spans, caveats)
	}
	span := spans[0]
	if span.Kind != "async" || span.Name != "async|pipeline" || span.SpanPID != 37722 ||
		span.DurationMs < 1.999999 || span.DurationMs > 2.000001 ||
		span.ActualDurationMs < 5.000332 || span.ActualDurationMs > 5.000334 ||
		span.StartLine != 1 || span.EndLine != 1 {
		t.Fatalf("typed interval projection drifted: %+v", span)
	}

	windowed, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 1.002, TimeEnd: 1.004, TimeStartSet: true, TimeEndSet: true,
		AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(windowed.Events) != 1 || windowed.Events[0].Type != EventTraceAsyncInterval {
		t.Fatalf("carry-in typed interval was dropped by window gate: %+v", windowed.Events)
	}
}

func TestCompletedAsyncIntervalCPUUnavailableAndMalformedWire(t *testing.T) {
	interval := CompletedAsyncInterval{
		StartTimestampNS: 2_000,
		EndTimestampNS:   4_000,
		SourceRow:        9,
		TID:              101,
		TGID:             100,
		SpanPID:          100,
		StartCPU:         -1,
		StartCPUStatus:   TraceMarkCPUStatusUnavailable,
		StartCPUReason:   TraceMarkCPUReasonUnknownStart,
		Comm:             "worker",
		Name:             "async-work",
		Cookie:           "9",
	}
	line, err := FormatCompletedAsyncInterval(interval)
	if err != nil {
		t.Fatal(err)
	}
	event, ok := ParseLine(1, line, newStringInterner())
	if !ok || event.CPU != -1 || event.PluginFields == nil ||
		event.PluginFields.TraceMarkerCPUStatus != TraceMarkCPUStatusUnavailable ||
		event.PluginFields.TraceMarkerCPUReason != TraceMarkCPUReasonUnknownStart {
		t.Fatalf("CPU-unavailable interval lost typed status: ok=%t event=%+v", ok, event)
	}
	parsed, ok := parseCompletedAsyncInterval(line)
	if !ok || parsed != interval {
		t.Fatalf("CPU-unavailable interval round trip=%+v ok=%t", parsed, ok)
	}

	invalid := interval
	invalid.EndTimestampNS = invalid.StartTimestampNS - 1
	if _, err := FormatCompletedAsyncInterval(invalid); err == nil {
		t.Fatal("end-before-start interval was formatted")
	}
	for _, mutation := range []string{
		strings.Replace(line, "end_ns=4000", "end_ns=1999", 1),
		strings.Replace(line, "start_cpu=~", "start_cpu=3", 1),
		strings.Replace(line, "cpu_reason=unknown_start_cpu", "cpu_reason=~", 1),
	} {
		if _, ok := parseCompletedAsyncInterval(mutation); ok {
			t.Fatalf("malformed completed interval parsed: %q", mutation)
		}
	}
}

func TestCompletedAsyncIntervalWindowDiscoveryKeepsSingleRowAuthority(t *testing.T) {
	line, err := FormatCompletedAsyncInterval(CompletedAsyncInterval{
		StartTimestampNS: 3_000_000_000,
		EndTimestampNS:   3_020_000_000,
		SourceRow:        88,
		TID:              101,
		TGID:             100,
		SpanPID:          100,
		StartCPU:         2,
		StartCPUStatus:   TraceAsyncIntervalCPUStatusKnown,
		Comm:             "worker",
		Name:             "async-verify",
		Cookie:           "11",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := writeWindowDiscoveryTrace(t, line)
	request := traceMarkCarryRequest(3.005, 3.010, 2, WindowDiscoveryFamilyTraceAsync)
	request.MaxWindowMs = 50
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Windows) != 1 || result.Windows[0].Kind != "typed_interval" ||
		result.Windows[0].CoreStartTs != 3 || result.Windows[0].CoreEndTs != 3.02 {
		t.Fatalf("typed interval discovery window drifted: %+v", result)
	}
	var candidate *WindowDiscoveryCandidate
	for i := range result.Candidates {
		if result.Candidates[i].Kind == "typed_interval" {
			candidate = &result.Candidates[i]
			break
		}
	}
	if candidate == nil || candidate.EndpointCount != 1 || candidate.StartEndpoint == nil ||
		candidate.EndEndpoint != nil || candidate.StartEndpoint.Line != 1 ||
		candidate.PairingStatus != WindowDiscoveryPairingCompleteExact {
		t.Fatalf("typed interval discovery fabricated an endpoint: %+v", result.Candidates)
	}
	if len(result.Families) != 1 || result.Families[0].CompletedIntervalCount != 1 ||
		result.Families[0].CompletedPairCount != 0 {
		t.Fatalf("typed interval was counted as an endpoint pair: %+v", result.Families)
	}
}
