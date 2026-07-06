package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// RFC #71 renderer pins (real_trace_campaign_20260705 §8.2 c4): the
// pre-truncation cpu_frequency census must reach the model-facing banner on
// the SAME face as the truncated row list, with the 807000 boundary tier and
// the exhaustiveness clause intact; the window_stats freq_residency fold
// must disclose the distinct-tier boundary it used to hide inside "+N".

func rfcC4CensusFixture() *tracequery.CPUFrequencyCensus {
	tiers := []tracequery.CPUFrequencyCensusTier{
		{FrequencyKHz: 807000, Rows: 6, CPUs: []int{3, 4, 5}},
		{FrequencyKHz: 965000, Rows: 4, CPUs: []int{3, 4}},
		{FrequencyKHz: 1090000, Rows: 12, CPUs: []int{3, 4, 5}},
		{FrequencyKHz: 1224000, Rows: 10, CPUs: []int{3, 4, 5}},
		{FrequencyKHz: 1325000, Rows: 9, CPUs: []int{3, 5}},
		{FrequencyKHz: 1418000, Rows: 9, CPUs: []int{4, 5}},
		{FrequencyKHz: 1517000, Rows: 9, CPUs: []int{3, 4}},
		{FrequencyKHz: 1618000, Rows: 9, CPUs: []int{3, 5}},
		{FrequencyKHz: 1749000, Rows: 9, CPUs: []int{4}},
		{FrequencyKHz: 2000000, Rows: 8, CPUs: []int{3, 4, 5}},
		{FrequencyKHz: 2189000, Rows: 5, CPUs: []int{3, 4, 5}},
	}
	return &tracequery.CPUFrequencyCensus{
		MatchedFrequencyRows:   90,
		DisplayedFrequencyRows: 40,
		FrequencyLimitRows:     2,
		CPUs:                   []int{3, 4, 5},
		Tiers:                  tiers,
		MinKHz:                 807000,
		MaxKHz:                 2189000,
		LineStart:              11623,
		LineEnd:                12789,
		Summary:                "full-window census of all 90 matched cpu_frequency row(s)",
	}
}

func TestTraceQuerySummaryRendersCPUFrequencyCensusOnTruncatedFace(t *testing.T) {
	result := tracequery.Result{
		View: "event_search",
		Events: []tracequery.EventView{
			{Event: tracequery.Event{Line: 11000, Ts: 34579.5001, Type: tracequery.EventCPUFrequency, Frequency: 1090000}, Raw: "cpu_frequency: state=1090000 cpu_id=3"},
		},
		CPUFrequencyCensus: rfcC4CensusFixture(),
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "event_search"}, "attached_trace", "")
	for _, want := range []string{
		"cpu_frequency_census(频点普查)",
		"matched_rows=90",
		"displayed_rows=40",
		"limit_rows=2",
		"cpus=3,4,5",
		"distinct_khz=11",
		"range=807000..2189000kHz",
		"lines=11623-12789",
		"BEFORE the chronological display truncation",
		"exhaustive for this window",
		"cpu_frequency_census_tiers khz×rows=807000×6,965000×4",
		"2189000×5",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("banner missing census clause %q:\n%s", want, summary)
		}
	}
}

// TestTraceQuerySummaryOmitsCensusRowWhenAbsent pins the additive-only
// discipline: results without a census render zero census bytes.
func TestTraceQuerySummaryOmitsCensusRowWhenAbsent(t *testing.T) {
	result := tracequery.Result{
		View: "event_search",
		Events: []tracequery.EventView{
			{Event: tracequery.Event{Line: 11000, Ts: 34579.5001, Type: tracequery.EventCPUFrequency, Frequency: 1090000}, Raw: "cpu_frequency: state=1090000 cpu_id=3"},
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "event_search"}, "attached_trace", "")
	if strings.Contains(summary, "cpu_frequency_census") {
		t.Fatalf("census row must not render without a census:\n%s", summary)
	}
}

func TestTraceFrequencyResidencyFoldDisclosesDistinctTierBoundary(t *testing.T) {
	// The c4 shape: 30 chronological residency segments, the first four all
	// upper tiers, 807000 buried in the "+26" fold.
	items := []tracequery.CPUFrequencyResidency{
		{Frequency: 1090000, DurationMs: 1.396},
		{Frequency: 2189000, DurationMs: 0.170},
		{Frequency: 1224000, DurationMs: 0.510},
		{Frequency: 1618000, DurationMs: 1.221},
	}
	ladder := []int{1090000, 2189000, 1224000, 1618000, 965000, 1325000, 1418000, 1517000, 1749000, 2000000, 807000}
	for i := 0; i < 26; i++ {
		items = append(items, tracequery.CPUFrequencyResidency{Frequency: ladder[i%len(ladder)], DurationMs: 0.1})
	}
	out := traceFrequencyResidencySummary(items)
	for _, want := range []string{
		" freq_residency=1090000kHz/1.396ms,2189000kHz/0.170ms,1224000kHz/0.510ms,1618000kHz/1.221ms,+26",
		" distinct_khz=11",
		" range=807000..2189000kHz",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("folded residency summary missing %q: %q", want, out)
		}
	}
}

// TestTraceFrequencyResidencyUnfoldedStaysByteIdentical pins the legacy
// bytes for ≤4-segment lines: the census suffix exists to undo a fold, never
// to restate a complete face.
func TestTraceFrequencyResidencyUnfoldedStaysByteIdentical(t *testing.T) {
	items := []tracequery.CPUFrequencyResidency{
		{Frequency: 1090000, DurationMs: 1.396},
		{Frequency: 2189000, DurationMs: 0.170},
		{Frequency: 1224000, DurationMs: 0.510},
		{Frequency: 1618000, DurationMs: 1.221},
	}
	got := traceFrequencyResidencySummary(items)
	want := " freq_residency=1090000kHz/1.396ms,2189000kHz/0.170ms,1224000kHz/0.510ms,1618000kHz/1.221ms"
	if got != want {
		t.Fatalf("unfolded residency line drifted:\n got: %q\nwant: %q", got, want)
	}
}
