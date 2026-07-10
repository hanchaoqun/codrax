package tracequery

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraceTimestampNonFiniteRangeAndDerivedOverflowFailClosed(t *testing.T) {
	overflowDecimal := strings.Repeat("9", 400) + ".0"
	// This decimal parses to a finite float, but is outside the representation
	// envelope that makes every seconds->milliseconds duration derivation safe.
	unsafeFiniteDecimal := "1" + strings.Repeat("0", 305) + ".0"
	for _, raw := range []string{"NaN", "Inf", "+Inf", overflowDecimal, unsafeFiniteDecimal} {
		line := "app-20 (20) [001] .... " + raw + ": sched_wakeup: comm=app pid=20 prio=20 target_cpu=001"
		if ts, ok := parseLineTimestamp(line); ok {
			t.Fatalf("invalid timestamp %q admitted by window/ordering parser: %v", raw, ts)
		}
		if ev, ok := ParseLine(1, line, newStringInterner()); ok {
			t.Fatalf("invalid timestamp %q entered the event index: %+v", raw, ev)
		}
	}

	path := filepath.Join(t.TempDir(), "finite-time.systrace")
	content := strings.Join([]string{
		`app-20 (20) [001] .... 1.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`,
		"app-20 (20) [001] .... " + overflowDecimal + `: sched_wakeup: comm=bad pid=21 prio=20 target_cpu=001`,
		"app-20 (20) [001] .... " + unsafeFiniteDecimal + `: sched_wakeup: comm=bad pid=22 prio=20 target_cpu=001`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || idx.Events[0].WakeePID != 20 {
		t.Fatalf("range-overflow timestamps polluted the index: %+v", idx.Events)
	}

	if duration, ok := traceDurationMilliseconds(1, 1.0025); !ok || math.Abs(duration-2.5) > 1e-9 {
		t.Fatalf("legal timestamp duration regressed: duration=%v ok=%v", duration, ok)
	}
	for _, bounds := range [][2]float64{
		{0, math.Inf(1)},
		{math.Inf(-1), 0},
		{0, math.NaN()},
		{-math.MaxFloat64, math.MaxFloat64},
	} {
		if duration, ok := traceDurationMilliseconds(bounds[0], bounds[1]); ok || duration != 0 {
			t.Fatalf("non-finite/overflowing derived duration admitted: bounds=%v duration=%v ok=%v", bounds, duration, ok)
		}
	}
}

func TestHeaderCPUDecimalOverflowCannotCollapseToCPUZero(t *testing.T) {
	hugeCPU := strings.Repeat("9", 200)
	line := "app-20 (20) [" + hugeCPU + `] .... 1.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`
	if ev, ok := ParseLine(1, line, newStringInterner()); ok {
		t.Fatalf("overflowing header CPU entered the index as cpu=%d: %+v", ev.CPU, ev)
	}
	if failures := cpuInputValidationFailures(1, line); len(failures) != 1 || failures[0].Field != "header_cpu" {
		t.Fatalf("overflowing header CPU lost its typed audit witness: %+v", failures)
	}
	valid := `app-20 (20) [000] .... 1.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`
	if ev, ok := ParseLine(2, valid, newStringInterner()); !ok || ev.CPU != 0 {
		t.Fatalf("legal header CPU0 regressed: ok=%v ev=%+v", ok, ev)
	}
}

func TestResourceLatencyNonFiniteAndUnitOverflowRowsAreRejected(t *testing.T) {
	overflowDecimal := strings.Repeat("9", 400)
	lines := []string{
		`app-20 (20) [001] .... 2.000000: bio_latency: op=R path=/data/valid.db latency_us=2500 bytes=4096`,
		`app-20 (20) [001] .... 2.001000: bio_latency: op=R path=/data/nan.db latency_ms=NaN bytes=4096`,
		`app-20 (20) [001] .... 2.002000: bio_latency: op=R path=/data/inf.db latency_ms=Inf bytes=4096`,
		`app-20 (20) [001] .... 2.003000: bio_latency: op=R path=/data/ninf.db latency_ms=-Inf bytes=4096`,
		"app-20 (20) [001] .... 2.004000: bio_latency: op=R path=/data/overflow.db latency_us=" + overflowDecimal + " bytes=4096",
	}
	idx := buildTraceIndex(t, "invalid-resource-latency.systrace", strings.Join(lines, "\n")+"\n")
	if len(idx.Events) != 1 || idx.Events[0].ResourceFields == nil || idx.Events[0].ResourceFields.Path != "/data/valid.db" {
		t.Fatalf("malformed latency rows entered the index: %+v", idx.Events)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.9, TimeEnd: 2.1, Limit: 20})
	if len(stats.BIOResources) != 1 || stats.BIOResources[0].Path != "/data/valid.db" || math.Abs(stats.BIOResources[0].TotalLatencyMs-2.5) > 1e-9 {
		t.Fatalf("malformed latency rows entered resource evidence: %+v", stats.BIOResources)
	}
	rank := BuildRootCauseRank(idx, Query{TimeStart: 1.9, TimeEnd: 2.1, Limit: 20})
	for _, item := range rank.Items {
		if strings.Contains(item.Summary, "/data/nan.db") || strings.Contains(item.Summary, "/data/inf.db") || strings.Contains(item.Summary, "/data/overflow.db") {
			t.Fatalf("malformed latency row entered root-cause ranking: %+v", item)
		}
	}

	for name, kv := range map[string]map[string]string{
		"milliseconds": {"latency_ms": "2.5"},
		"microseconds": {"latency_us": "2500"},
		"nanoseconds":  {"latency_ns": "2500000"},
	} {
		if got, ok := parseLatencyMsChecked(kv); !ok || math.Abs(got-2.5) > 1e-9 {
			t.Fatalf("legal %s latency regressed: got=%v ok=%v", name, got, ok)
		}
	}
	for _, raw := range []string{"NaN", "Inf", "-Inf", "-1", "1e309", overflowDecimal} {
		if got, ok := parseLatencyMsChecked(map[string]string{"duration_ms": raw}); ok || got != 0 {
			t.Fatalf("non-finite/overflow latency %q admitted: got=%v ok=%v", raw, got, ok)
		}
	}
}
