package tracediag

// render_clamp_diag_test.go — TDIAG 留观② (§28.13 反射单行字节无界,
// 2026-07-09): the reflection detail walker's joined slice/map tokens pass
// through clampToken — a pathological engine payload can no longer mint an
// unbounded single report line (per-element clamping alone still joined into
// an unbounded line).

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestReflectionJoinedTokensAreByteCapped(t *testing.T) {
	long := make([]string, 0, 200)
	for i := 0; i < 200; i++ {
		long = append(long, "元素宽字符串abcdefghijklmnop")
	}
	got := formatScalarSlice(reflect.ValueOf(long))
	if len(got) > maxRenderedTokenBytes+len("…(截断)") {
		t.Fatalf("joined slice token must be byte-capped: %d bytes", len(got))
	}
	if !strings.HasSuffix(got, "…(截断)") {
		t.Fatalf("the cut must be marked: %q", got[len(got)-32:])
	}

	m := map[string]string{}
	for i := 0; i < 200; i++ {
		m[strings.Repeat("k", 10)+string(rune('a'+i%26))+string(rune('0'+i/26))] = "值值值值值值值值值值"
	}
	gotMap := formatMapTokens(reflect.ValueOf(m))
	if len(gotMap) > maxRenderedTokenBytes+len("…(截断)") {
		t.Fatalf("joined map token must be byte-capped: %d bytes", len(gotMap))
	}
	if !strings.HasSuffix(gotMap, "…(截断)") {
		t.Fatalf("the map cut must be marked: %q", gotMap[len(gotMap)-32:])
	}

	// Short payloads stay byte-identical (no marker, no clamp).
	if got := formatScalarSlice(reflect.ValueOf([]int{1, 2, 3})); got != "[1, 2, 3]" {
		t.Fatalf("short slice must render whole: %q", got)
	}
}

func TestEventSearchFrequencyCensusDisplayedRowsUseStrictSamples(t *testing.T) {
	result := &tracequery.Result{
		Events: []tracequery.EventView{
			{Event: tracequery.Event{Type: tracequery.EventCPUFrequency, Name: "cpu_frequency", Frequency: 1800000, CPUForField: 1, CPUForFieldValid: true}},
			{Event: tracequery.Event{Type: tracequery.EventCPUFrequency, Name: "cpu_frequency", Frequency: 0, CPUForField: 1, CPUForFieldValid: true}},
			{Event: tracequery.Event{Type: tracequery.EventCPUFrequency, Name: "cpu_frequency", Frequency: 2200000, CPUForField: 1, CPUForFieldValid: true, CPUInputInvalid: true}},
			{Event: tracequery.Event{Type: tracequery.EventCPUFrequency, Name: "clock_set_rate", ClockName: "cpu-cluster.0", Frequency: 1400000, CPUForField: 1, CPUForFieldValid: true}},
		},
		CPUFrequencyCensus: &tracequery.CPUFrequencyCensus{
			MatchedFrequencyRows: 5,
			Tiers:                []tracequery.CPUFrequencyCensusTier{{FrequencyKHz: 1800000, Rows: 5}},
			MinKHz:               1800000,
			MaxKHz:               1800000,
			CPUs:                 []int{1},
		},
	}
	lines := eventSearchDiagnosticLines(result, len(result.Events), len(result.Events), false)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "matched_frequency_rows=5 displayed_frequency_rows=1") {
		t.Fatalf("zero/malformed/reclassified clock row entered displayed frequency census:\n%s", joined)
	}
}
