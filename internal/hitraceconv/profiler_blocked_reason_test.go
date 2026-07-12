package hitraceconv

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestProfilerStructuredSchedBlockedReasonUsesCanonicalCallerQuality(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "blocked-reason.htrace")
	payload := protoMessage(2,
		protoVarint(1, 3),
		syntheticTracePluginFtraceEvent(1_000, 562, 562, "worker", 4002, protoPayload(
			protoVarint(1, 562),
			protoVarint(2, 0x41424344),
			protoVarint(3, 1),
			protoBytes(4, []byte("schedule_timeout+0x10/0x20[kernel]")),
		)),
		syntheticTracePluginFtraceEvent(2_000, 563, 563, "worker2", 4002, protoPayload(
			protoVarint(1, 563),
			protoVarint(2, 0x11223344),
			protoVarint(3, 0),
		)),
		syntheticTracePluginFtraceEvent(3_000, 564, 564, "worker3", 4002, protoPayload(
			protoVarint(1, 564),
			protoVarint(2, 0x55667788),
			protoVarint(3, 1),
			protoBytes(4, []byte("forged\nsecond-row")),
		)),
	)
	body := syntheticProfilerTraceFile(syntheticProfilerPluginData("ftrace-plugin", payload))
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"), TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatalf("convert blocked reason fixture: %v", err)
	}
	if result.EventsWritten != 3 {
		t.Fatalf("blocked reason rows=%d, want 3: %+v", result.EventsWritten, result)
	}
	converted, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(converted)
	for _, want := range []string{
		"sched_blocked_reason: pid=562 iowait=1 caller=schedule_timeout+0x10/0x20[kernel]",
		"sched_blocked_reason: pid=563 iowait=0 caller=unknown caller_raw=0x11223344 caller_quality=opaque",
		"sched_blocked_reason: pid=564 iowait=1 caller=unknown caller_raw=0x55667788 caller_quality=opaque",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("converted output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "forged\nsecond-row") || strings.Contains(text, "caller=0x") {
		t.Fatalf("unsafe/raw caller became semantic caller or injected a line:\n%s", text)
	}
	if strings.Contains(text, "caller_quality=symbolized") {
		t.Fatalf("structured symbolized caller bypassed the shared canonical renderer:\n%s", text)
	}

	idx, err := tracequery.BuildIndex(context.Background(), result.OutputPath)
	if err != nil {
		t.Fatalf("tracequery parse blocked reason output: %v", err)
	}
	var reasons []string
	for _, event := range idx.Events {
		if event.Type == tracequery.EventSchedBlockedReason {
			reasons = append(reasons, event.Reason)
		}
	}
	if len(reasons) != 3 || reasons[0] != "schedule_timeout+0x10/0x20[kernel]" || reasons[1] != "unknown" || reasons[2] != "unknown" {
		t.Fatalf("semantic blocked callers=%v, want [symbol unknown unknown]", reasons)
	}

	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		TraceCoverage []TraceDBCoverage `json:"trace_coverage"`
	}
	if err := json.Unmarshal(bundle, &meta); err != nil {
		t.Fatal(err)
	}
	if !coverageHasEmitted(meta.TraceCoverage, "builtin_modern_ftrace:sched", "sched_blocked_reason", 3) {
		t.Fatalf("blocked reason structured coverage missing: %+v", meta.TraceCoverage)
	}
}
