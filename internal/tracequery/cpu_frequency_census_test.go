package tracequery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// RFC #71 pin battery (real_trace_campaign_20260705 §8.2 c4): the synthetic
// twin of the customer shape — 90 cpu_frequency rows across cpus 3-5 on an
// 11-tier ladder where every 807000kHz row sits AFTER the 40-row
// chronological display cut. The census must still carry all 11 tiers with
// the true 807000..2189000 boundary; the display face alone enumerates only
// the upper tiers (the exact wrong-range trap the model fell into).

// rfcC4UpperTiers are the 9 tiers visible inside the first 40 chronological
// rows (the truncated enumeration face the c4 answer was built on).
var rfcC4UpperTiers = []int{1090000, 1224000, 1325000, 1418000, 1517000, 1618000, 1749000, 2000000, 2189000}

// writeRFCC4FrequencyTrace writes the synthetic 90-row ladder + 2 trailing
// cpu_frequency_limits rows and returns the path plus the expected per-tier
// row counts.
func writeRFCC4FrequencyTrace(t *testing.T) (string, map[int64]int) {
	t.Helper()
	var b strings.Builder
	counts := map[int64]int{}
	row := 0
	emit := func(khz int) {
		row++
		cpu := 3 + (row % 3)
		counts[int64(khz)]++
		fmt.Fprintf(&b, "  tppmgr-5850 ( 5850) [00%d] .... %.6f: cpu_frequency: state=%d cpu_id=%d\n",
			cpu, 34579.500000+float64(row)*0.0001, khz, cpu)
	}
	// Rows 1-40: upper 9 tiers only (the truncated display face).
	for i := 0; i < 40; i++ {
		emit(rfcC4UpperTiers[i%len(rfcC4UpperTiers)])
	}
	// Rows 41-84: upper tiers + 965000 (still above the display cut).
	midTiers := append([]int{965000}, rfcC4UpperTiers...)
	for i := 0; i < 44; i++ {
		emit(midTiers[i%len(midTiers)])
	}
	// Rows 85-90: the 807000 boundary tier — entirely past the cut.
	for i := 0; i < 6; i++ {
		emit(807000)
	}
	// Trailing limits rows: compat-matched by event_types=[cpu_frequency],
	// counted separately, never a ladder tier.
	fmt.Fprintf(&b, "  tppmgr-5850 ( 5850) [003] .... %.6f: cpu_frequency_limits: min=500000 max=2400000 cpu_id=3\n", 34579.600000)
	fmt.Fprintf(&b, "  tppmgr-5850 ( 5850) [004] .... %.6f: cpu_frequency_limits: min=500000 max=2400000 cpu_id=4\n", 34579.600100)
	path := filepath.Join(t.TempDir(), "rfc_c4_freq.systrace")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, counts
}

func TestStreamEventSearchCPUFrequencyCensusSurvivesChronologicalTruncation(t *testing.T) {
	path, counts := writeRFCC4FrequencyTrace(t)
	res, err := StreamEventSearch(context.Background(), path, Query{EventTypes: []EventType{EventCPUFrequency}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != sharedDefaultResultLimit {
		t.Fatalf("expected the default %d-row chronological display cut, got %d rows", sharedDefaultResultLimit, len(res.Events))
	}
	// Trap-shape sanity: the display face must NOT contain the boundary tier.
	for _, ev := range res.Events {
		if ev.Frequency == 807000 {
			t.Fatalf("fixture broken: 807000 leaked into the displayed rows (line %d)", ev.Line)
		}
	}
	census := res.CPUFrequencyCensus
	if census == nil {
		t.Fatal("expected a pre-truncation cpu_frequency census on the truncated result")
	}
	if census.MatchedFrequencyRows != 90 || census.DisplayedFrequencyRows != 40 {
		t.Fatalf("census counts wrong: matched=%d displayed=%d", census.MatchedFrequencyRows, census.DisplayedFrequencyRows)
	}
	if census.FrequencyLimitRows != 2 {
		t.Fatalf("expected 2 compat-matched cpu_frequency_limits rows, got %d", census.FrequencyLimitRows)
	}
	if len(census.Tiers) != 11 {
		t.Fatalf("expected all 11 ladder tiers in the census, got %d: %+v", len(census.Tiers), census.Tiers)
	}
	if census.MinKHz != 807000 || census.MaxKHz != 2189000 {
		t.Fatalf("census boundary wrong: %d..%d (the 807000 boundary is the c4 pin)", census.MinKHz, census.MaxKHz)
	}
	for i, tier := range census.Tiers {
		if i > 0 && census.Tiers[i-1].FrequencyKHz >= tier.FrequencyKHz {
			t.Fatalf("tiers not strictly ascending at %d: %+v", i, census.Tiers)
		}
		if want := counts[tier.FrequencyKHz]; tier.Rows != want {
			t.Fatalf("tier %d row count %d != generated %d", tier.FrequencyKHz, tier.Rows, want)
		}
		if tier.FrequencyKHz == 807000 && tier.Rows != 6 {
			t.Fatalf("807000 must carry its 6 hidden rows, got %d", tier.Rows)
		}
	}
	if !reflect.DeepEqual(census.CPUs, []int{3, 4, 5}) {
		t.Fatalf("census cpu set wrong: %v", census.CPUs)
	}
	if len(res.EvidencePack) == 0 || res.EvidencePack[0].Predicate != "frequency_tier_census" {
		t.Fatalf("census evidence fact must lead the evidence pack: %+v", res.EvidencePack[:min(len(res.EvidencePack), 2)])
	}
	if res.EvidencePack[0].Object != "807000..2189000kHz" {
		t.Fatalf("census fact object must carry the true boundary: %q", res.EvidencePack[0].Object)
	}
	if !strings.Contains(res.EvidencePack[0].Summary, "807000×6") {
		t.Fatalf("census fact summary must list the hidden boundary tier: %q", res.EvidencePack[0].Summary)
	}
}

func TestStreamEventSearchCPUFrequencyCensusAbsentWhenNotTruncated(t *testing.T) {
	path, _ := writeRFCC4FrequencyTrace(t)
	res, err := StreamEventSearch(context.Background(), path, Query{EventTypes: []EventType{EventCPUFrequency}, Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 92 {
		t.Fatalf("expected all 92 rows (90 freq + 2 limits) without the cap, got %d", len(res.Events))
	}
	if res.CPUFrequencyCensus != nil {
		t.Fatalf("census must stay nil when the display face is complete (byte-stability lane): %+v", res.CPUFrequencyCensus)
	}
	for _, fact := range res.EvidencePack {
		if fact.Predicate == "frequency_tier_census" {
			t.Fatalf("no census fact may appear on a non-truncated result")
		}
	}
}

// TestIndexedEventSearchCPUFrequencyCensusTwinParity pins the indexed
// fallback path (typed-field pattern matching) to the exact census the
// streaming path publishes — the two admission twins must not drift.
func TestIndexedEventSearchCPUFrequencyCensusTwinParity(t *testing.T) {
	path, _ := writeRFCC4FrequencyTrace(t)
	streamRes, err := StreamEventSearch(context.Background(), path, Query{EventTypes: []EventType{EventCPUFrequency}})
	if err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	indexedRes := Run(idx, Query{View: "event_search", EventTypes: []EventType{EventCPUFrequency}})
	if indexedRes.CPUFrequencyCensus == nil {
		t.Fatal("indexed event_search must publish the same census lane")
	}
	if !reflect.DeepEqual(streamRes.CPUFrequencyCensus, indexedRes.CPUFrequencyCensus) {
		t.Fatalf("stream/indexed census twins drifted:\nstream:  %+v\nindexed: %+v",
			streamRes.CPUFrequencyCensus, indexedRes.CPUFrequencyCensus)
	}
}

func TestCPUFrequencyCensusRequiresFrequencyEventType(t *testing.T) {
	path, _ := writeRFCC4FrequencyTrace(t)
	// Same truncation, no cpu_frequency in event_types: the census is the
	// frequency-enumeration face's truth row, not a general event census.
	res, err := StreamEventSearch(context.Background(), path, Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 10 {
		t.Fatalf("expected truncated generic search, got %d rows", len(res.Events))
	}
	if res.CPUFrequencyCensus != nil {
		t.Fatalf("census must stay nil without an explicit cpu_frequency event_types filter")
	}
}

// TestCPUFrequencyCensusHonorsWindowPredicate pins census admission to the
// SAME window predicate as the display face: rows outside the line window
// must not inflate the ladder (the census claims a window census, not a
// whole-file census).
func TestCPUFrequencyCensusHonorsWindowPredicate(t *testing.T) {
	path, _ := writeRFCC4FrequencyTrace(t)
	res, err := StreamEventSearch(context.Background(), path, Query{
		EventTypes: []EventType{EventCPUFrequency},
		LineStart:  1,
		LineEnd:    40,
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	census := res.CPUFrequencyCensus
	if census == nil {
		t.Fatal("expected census on the truncated line window")
	}
	if census.MatchedFrequencyRows != 40 || census.DisplayedFrequencyRows != 10 {
		t.Fatalf("window census counts wrong: matched=%d displayed=%d", census.MatchedFrequencyRows, census.DisplayedFrequencyRows)
	}
	if len(census.Tiers) != len(rfcC4UpperTiers) {
		t.Fatalf("line-window census must only carry in-window tiers: %+v", census.Tiers)
	}
	if census.MinKHz != 1090000 {
		t.Fatalf("in-window boundary must reflect the window, got min=%d", census.MinKHz)
	}
	for _, tier := range census.Tiers {
		if tier.FrequencyKHz == 807000 {
			t.Fatalf("807000 sits outside the line window and must not enter this census")
		}
	}
}

// TestFormatCPUFrequencyCensusTiersFoldKeepsEndpoints pins the defensive
// over-cap fold to a boundary-preserving shape: the census exists for
// range/boundary claims, so min and max tiers must survive any fold.
func TestFormatCPUFrequencyCensusTiersFoldKeepsEndpoints(t *testing.T) {
	tiers := make([]CPUFrequencyCensusTier, 0, 30)
	for i := 0; i < 30; i++ {
		tiers = append(tiers, CPUFrequencyCensusTier{FrequencyKHz: int64(500000 + i*100000), Rows: i + 1})
	}
	out := FormatCPUFrequencyCensusTiers(tiers, 24)
	if !strings.Contains(out, "500000×1") {
		t.Fatalf("fold dropped the min endpoint: %q", out)
	}
	if !strings.Contains(out, "3400000×30") {
		t.Fatalf("fold dropped the max endpoint: %q", out)
	}
	if !strings.Contains(out, "mid tier(s) folded; ladder endpoints retained") {
		t.Fatalf("fold must disclose itself: %q", out)
	}
	// No-fold shape: every tier verbatim, no fold marker.
	full := FormatCPUFrequencyCensusTiers(tiers[:5], 24)
	if strings.Contains(full, "folded") || strings.Count(full, "×") != 5 {
		t.Fatalf("under-cap listing must be verbatim: %q", full)
	}
}
