package tracequery

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

// Decode the public wire contract independently of the production Go type so
// this regression is executable against the pre-distribution implementation.
type ioRequestDistributionWire struct {
	SampleCount    int     `json:"sample_count"`
	MinMs          float64 `json:"min_ms"`
	MaxMs          float64 `json:"max_ms"`
	MeanMs         float64 `json:"mean_ms"`
	P50Ms          float64 `json:"p50_ms"`
	P90Ms          float64 `json:"p90_ms"`
	P95Ms          float64 `json:"p95_ms"`
	P99Ms          float64 `json:"p99_ms"`
	QuantileMethod string  `json:"quantile_method"`
	SamplePolicy   string  `json:"sample_policy"`
	LatencyCaliber string  `json:"latency_caliber"`
}

func ioRequestDistribution(t *testing.T, row *StorageLatencySummary) *ioRequestDistributionWire {
	t.Helper()
	if row == nil {
		t.Fatal("missing storage latency row")
	}
	data, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Distribution *ioRequestDistributionWire `json:"request_latency_distribution"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	return wire.Distribution
}

func assertIORequestDistribution(t *testing.T, row *StorageLatencySummary, n int, min, max, mean, p50, p90, p95, p99 float64) {
	t.Helper()
	d := ioRequestDistribution(t, row)
	if d == nil {
		t.Fatalf("complete exact-pair population has no request_latency_distribution: %+v", row)
	}
	if d.SampleCount != n || d.SampleCount != row.PairedCount {
		t.Fatalf("sample count=%d, want %d paired=%d", d.SampleCount, n, row.PairedCount)
	}
	for name, pair := range map[string][2]float64{"min": {d.MinMs, min}, "max": {d.MaxMs, max}, "mean": {d.MeanMs, mean}, "p50": {d.P50Ms, p50}, "p90": {d.P90Ms, p90}, "p95": {d.P95Ms, p95}, "p99": {d.P99Ms, p99}} {
		if math.Abs(pair[0]-pair[1]) > 1e-6 {
			t.Errorf("%s=%g, want %g", name, pair[0], pair[1])
		}
	}
	if d.QuantileMethod != "linear_interpolation_n_minus_1" || d.SamplePolicy != "complete_pairs_intersecting_query" || d.LatencyCaliber != "full_request_start_to_completion" {
		t.Errorf("distribution lacks explicit method/population/caliber: %+v", d)
	}
}

func TestIORequestLatencyDistributionUsesFullPopulationBeforeTopEight(t *testing.T) {
	for _, lowScale := range []float64{1, 0.1} {
		t.Run(fmt.Sprintf("lowest_three_scale_%g", lowScale), func(t *testing.T) {
			var body strings.Builder
			for i := 1; i <= 11; i++ {
				duration := float64(i)
				if i <= 3 {
					duration *= lowScale
				}
				fmt.Fprintf(&body, "io-40 (40) [003] .... %.6f: block_rq_issue: 8,0 R 4096 () %d + 8 [io]\n", float64(i), i*8)
				fmt.Fprintf(&body, "irq-2 (2) [003] .... %.6f: block_rq_complete: 8,0 R () %d + 8 [0]\n", float64(i)+duration/1000, i*8)
			}
			idx := buildTraceIndex(t, "io-full-population.systrace", body.String())
			result := Run(idx, Query{View: "window_stats"})
			if result.WindowStats == nil {
				t.Fatalf("public Run omitted window stats: %+v", result)
			}
			stats := result.WindowStats
			if len(stats.IOLatencies) != 8 {
				t.Fatalf("display cap changed: got %d, want 8", len(stats.IOLatencies))
			}
			for _, sample := range stats.IOLatencies {
				if sample.DurationMs < 3.999 {
					t.Fatalf("low tail entered unchanged Top-8 display: %+v", stats.IOLatencies)
				}
			}
			row := storageLatencyRow(stats.StorageLatencyByLayer, "block", blockEndpointFamilyRQ)
			assertIORequestDistribution(t, row, 11, lowScale, 11, (60+6*lowScale)/11, 6, 10, 10.5, 10.9)
		})
	}
}

func TestIORequestLatencyDistributionExistingEndpointFamilies(t *testing.T) {
	cases := []struct {
		name, layer, family, startName, startBody, doneName, doneBody string
	}{
		{"rq", "block", blockEndpointFamilyRQ, "block_rq_issue", "8,0 R 4096 () 123 + 8 [io]", "block_rq_complete", "8,0 R () 123 + 8 [0]"},
		{"bio", "block", blockEndpointFamilyBIO, "block_bio_queue", "8,0 R 123 + 8 [io]", "block_bio_complete", "8,0 R 123 + 8 [0]"},
		{"scsi", "scsi", "scsi_dispatch_cmd", "scsi_dispatch_cmd_start", "dev=12,80 op=read bytes=4096", "scsi_dispatch_cmd_done", "dev=12,80 op=read bytes=4096"},
		{"f2fs", "f2fs", "f2fs_direct_io", "f2fs_direct_IO_enter", "dev=260:136 ino=0x1 pos=0 len=4096 rw=read", "f2fs_direct_IO_exit", "dev=260:136 ino=0x1 pos=0 len=4096 rw=read ret=4096"},
		{"mmc", "mmc", "mmc_request", "mmc_request_start", mmcExactStartBody, "mmc_request_done", mmcDirectDoneBody},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body strings.Builder
			for i, latency := range []float64{1, 3, 9} {
				start := float64(i + 1)
				fmt.Fprintf(&body, "io-40 (40) [003] .... %.6f: %s: %s\n", start, tc.startName, tc.startBody)
				fmt.Fprintf(&body, "io-40 (40) [003] .... %.6f: %s: %s\n", start+latency/1000, tc.doneName, tc.doneBody)
			}
			idx := buildTraceIndex(t, "io-"+tc.name+".systrace", body.String())
			stats := ComputeWindowStats(idx, Query{})
			row := storageLatencyRow(stats.StorageLatencyByLayer, tc.layer, tc.family)
			assertIORequestDistribution(t, row, 3, 1, 9, 13.0/3, 3, 7.8, 8.4, 8.88)
		})
	}
}

func TestIORequestLatencyDistributionAbsentVersusMeasuredZero(t *testing.T) {
	t.Run("unmatched start", func(t *testing.T) {
		idx := buildTraceIndex(t, "io-open.systrace", "io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]\n")
		stats := ComputeWindowStats(idx, Query{})
		row := storageLatencyRow(stats.StorageLatencyByLayer, "block", blockEndpointFamilyRQ)
		if row == nil || row.PairedCount != 0 || row.UnpairedStartCount != 1 {
			t.Fatalf("unmatched endpoint accounting changed: %+v", row)
		}
		if d := ioRequestDistribution(t, row); d != nil {
			t.Fatalf("absence became measured zero: %+v", d)
		}
	})
	t.Run("ordered equal timestamp pair", func(t *testing.T) {
		idx := buildTraceIndex(t, "io-zero.systrace", "io-40 (40) [003] .... 0.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]\n"+
			"irq-2 (2) [003] .... 0.000000: block_rq_complete: 8,0 R () 123 + 8 [0]\n")
		stats := ComputeWindowStats(idx, Query{TimeStartSet: true, TimeEndSet: true})
		row := storageLatencyRow(stats.StorageLatencyByLayer, "block", blockEndpointFamilyRQ)
		assertIORequestDistribution(t, row, 1, 0, 0, 0, 0, 0, 0, 0)
		data, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		// Decode only the nested object: zero-valued measured fields must remain
		// present on the wire instead of becoming indistinguishable from unknown.
		var outer map[string]json.RawMessage
		if err := json.Unmarshal(data, &outer); err != nil {
			t.Fatal(err)
		}
		var distribution map[string]json.RawMessage
		if err := json.Unmarshal(outer["request_latency_distribution"], &distribution); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"sample_count", "min_ms", "max_ms", "mean_ms", "p50_ms", "p90_ms", "p95_ms", "p99_ms"} {
			if _, ok := distribution[key]; !ok {
				t.Errorf("measured zero omitted %s: %s", key, data)
			}
		}
	})
}

func TestIORequestLatencyDistributionRejectsIncompleteAmbiguousOrMixedFamilies(t *testing.T) {
	cases := []struct{ name, body string }{
		{"missing start", "irq-2 (2) [003] .... 1.000000: block_rq_complete: 8,0 R () 123 + 8 [0]\n"},
		{"mixed rq bio", "io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]\nirq-2 (2) [003] .... 1.003000: block_bio_complete: 8,0 R 123 + 8 [0]\n"},
		{"ambiguous rq", "io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]\nio-41 (41) [003] .... 1.001000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]\nirq-2 (2) [003] .... 1.002000: block_rq_complete: 8,0 R () 123 + 8 [0]\nirq-2 (2) [003] .... 1.003000: block_rq_complete: 8,0 R () 123 + 8 [0]\n"},
		{"ambiguous scsi", "io-40 (40) [003] .... 1.000000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096\nio-40 (40) [003] .... 1.001000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096\nio-40 (40) [003] .... 1.002000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096\nio-40 (40) [003] .... 1.003000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx := buildTraceIndex(t, "io-rejected.systrace", tc.body)
			stats := ComputeWindowStats(idx, Query{})
			if len(stats.StorageLatencyByLayer) == 0 {
				t.Fatal("missing endpoint quality rows")
			}
			for i := range stats.StorageLatencyByLayer {
				row := &stats.StorageLatencyByLayer[i]
				if row.PairedCount != 0 || ioRequestDistribution(t, row) != nil {
					t.Fatalf("unproven endpoint topology minted a distribution: %+v", row)
				}
			}
		})
	}
}

func TestIORequestLatencyDistributionNeverCrossesSources(t *testing.T) {
	for _, tc := range []struct{ name, start, done string }{
		{"block", "block_rq_issue: 8,0 R 4096 () 123 + 8 [io]", "block_rq_complete: 8,0 R () 123 + 8 [0]"},
		{"scsi", "scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096", "scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := buildTraceIndex(t, "io-cross-source.systrace", "io-40 (40) [003] .... 1.000000: "+tc.start+"\nio-40 (40) [003] .... 1.003000: "+tc.done+"\n")
			idx.TraceArtifacts = []TraceArtifactSource{
				{SourcePath: "/trace/io-a.systrace", LocalLineCount: 1, VirtualLineBase: 0, CausalCompatible: true},
				{SourcePath: "/trace/io-b.systrace", LocalLineCount: 1, VirtualLineBase: 1, CausalCompatible: true},
			}
			stats := ComputeWindowStats(idx, Query{})
			if len(stats.StorageLatencyByLayer) != 2 {
				t.Fatalf("source-local unmatched rows lost: %+v caveats=%v", stats.StorageLatencyByLayer, stats.Caveats)
			}
			for i := range stats.StorageLatencyByLayer {
				row := &stats.StorageLatencyByLayer[i]
				if row.PairedCount != 0 || ioRequestDistribution(t, row) != nil {
					t.Fatalf("cross-source endpoints minted duration distribution: %+v", row)
				}
			}
		})
	}
}

func TestIORequestLatencyDistributionFullLifetimeForIntersectingQueries(t *testing.T) {
	for _, tc := range []struct{ name, layer, family, start, done string }{
		{"block", "block", blockEndpointFamilyRQ, "block_rq_issue: 8,0 R 4096 () 123 + 8 [io]", "block_rq_complete: 8,0 R () 123 + 8 [0]"},
		{"scsi", "scsi", "scsi_dispatch_cmd", "scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096", "scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := buildTraceIndex(t, "io-window.systrace", "io-40 (40) [003] .... 1.000000: "+tc.start+"\nio-40 (40) [003] .... 3.000000: "+tc.done+"\n")
			for _, query := range []struct {
				name string
				q    Query
			}{
				{"carry through point", Query{TimeStart: 2, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true}},
				{"carry in", Query{TimeStart: 2, TimeEnd: 4, TimeStartSet: true, TimeEndSet: true}},
				{"carry out", Query{TimeStart: .5, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true}},
				{"line domain wins", Query{LineStart: 1, LineEnd: 2, TimeStart: 100, TimeEnd: 101, TimeStartSet: true, TimeEndSet: true}},
			} {
				t.Run(query.name, func(t *testing.T) {
					stats := ComputeWindowStats(idx, query.q)
					row := storageLatencyRow(stats.StorageLatencyByLayer, tc.layer, tc.family)
					assertIORequestDistribution(t, row, 1, 2000, 2000, 2000, 2000, 2000, 2000, 2000)
				})
			}
			for _, q := range []Query{{TimeStartSet: true, TimeEndSet: true}, {TimeStart: 4, TimeEnd: 5, TimeStartSet: true, TimeEndSet: true}} {
				stats := ComputeWindowStats(idx, q)
				if len(stats.StorageLatencyByLayer) != 0 {
					t.Fatalf("nonintersecting/explicit-zero domain retained sample: q=%+v rows=%+v", q, stats.StorageLatencyByLayer)
				}
			}
		})
	}
}

func TestIORequestLatencyDistributionGroupOverflowIsExplicit(t *testing.T) {
	var body strings.Builder
	start := 1.0
	for group := 1; group <= 11; group++ {
		for sample := 0; sample < group; sample++ {
			fmt.Fprintf(&body, "io-40 (40) [003] .... %.6f: block_rq_issue: 8,%d R 4096 () %d + 8 [io]\n", start, group, sample*8)
			fmt.Fprintf(&body, "irq-2 (2) [003] .... %.6f: block_rq_complete: 8,%d R () %d + 8 [0]\n", start+float64(group)/1000, group, sample*8)
			start++
		}
	}
	idx := buildTraceIndex(t, "io-overflow.systrace", body.String())
	result := Run(idx, Query{View: "window_stats"})
	if result.WindowStats == nil {
		t.Fatal("public Run omitted window stats")
	}
	stats := result.WindowStats
	if len(stats.StorageLatencyByLayer) != 8 {
		t.Fatalf("summary display cap changed: got %d rows", len(stats.StorageLatencyByLayer))
	}
	visibleSamples := 0
	for i := range stats.StorageLatencyByLayer {
		row := &stats.StorageLatencyByLayer[i]
		var group int
		if _, err := fmt.Sscanf(row.Dev, "8,%d", &group); err != nil {
			t.Fatal(err)
		}
		latency := float64(group)
		assertIORequestDistribution(t, row, group, latency, latency, latency, latency, latency, latency, latency)
		visibleSamples += row.PairedCount
	}
	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		OverflowGroups      int `json:"storage_latency_overflow_groups"`
		OverflowPairedCount int `json:"storage_latency_overflow_paired_count"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.OverflowGroups != 3 || wire.OverflowPairedCount != 6 || visibleSamples+wire.OverflowPairedCount != 66 {
		t.Fatalf("summary truncation erased population accounting: visible=%d overflow=%+v", visibleSamples, wire)
	}
}

func TestIORequestLatencyDistributionKeepsSourceFamilyDeviceAndOperationSeparate(t *testing.T) {
	type pair struct {
		family, dev, operation string
		latency                float64
	}
	pairs := []pair{
		{blockEndpointFamilyRQ, "8,0", "R", 1},
		{blockEndpointFamilyRQ, "8,0", "R", 7},
		{blockEndpointFamilyRQ, "8,0", "W", 2},
		{blockEndpointFamilyRQ, "8,1", "R", 3},
		{blockEndpointFamilyBIO, "8,0", "R", 4},
		{blockEndpointFamilyRQ, "8,0", "R", 5},
	}
	var body strings.Builder
	for i, p := range pairs {
		startName, doneName := "block_rq_issue", "block_rq_complete"
		startBody := fmt.Sprintf("%s %s 4096 () 123 + 8 [io]", p.dev, p.operation)
		doneBody := fmt.Sprintf("%s %s () 123 + 8 [0]", p.dev, p.operation)
		if p.family == blockEndpointFamilyBIO {
			startName, doneName = "block_bio_queue", "block_bio_complete"
			startBody = fmt.Sprintf("%s %s 123 + 8 [io]", p.dev, p.operation)
			doneBody = fmt.Sprintf("%s %s 123 + 8 [0]", p.dev, p.operation)
		}
		start := float64(i + 1)
		fmt.Fprintf(&body, "io-40 (40) [003] .... %.6f: %s: %s\n", start, startName, startBody)
		fmt.Fprintf(&body, "irq-2 (2) [003] .... %.6f: %s: %s\n", start+p.latency/1000, doneName, doneBody)
	}
	idx := buildTraceIndex(t, "io-separate-groups.systrace", body.String())
	idx.TraceArtifacts = []TraceArtifactSource{
		{SourcePath: "/trace/io-a.systrace", LocalLineCount: 10, VirtualLineBase: 0, CausalCompatible: true},
		{SourcePath: "/trace/io-b.systrace", LocalLineCount: 2, VirtualLineBase: 10, CausalCompatible: true},
	}
	stats := ComputeWindowStats(idx, Query{})
	if len(stats.StorageLatencyByLayer) != 5 {
		t.Fatalf("independent sample domains collapsed: %+v", stats.StorageLatencyByLayer)
	}
	wanted := map[string]float64{
		"/trace/io-a.systrace|block_rq|8,0|R":  0,
		"/trace/io-a.systrace|block_rq|8,0|W":  2,
		"/trace/io-a.systrace|block_rq|8,1|R":  3,
		"/trace/io-a.systrace|block_bio|8,0|R": 4,
		"/trace/io-b.systrace|block_rq|8,0|R":  5,
	}
	for i := range stats.StorageLatencyByLayer {
		row := &stats.StorageLatencyByLayer[i]
		key := strings.Join([]string{row.SourcePath, row.Event, row.Dev, row.Operation}, "|")
		latency, ok := wanted[key]
		if !ok {
			t.Fatalf("unexpected or duplicate sample domain: %+v", row)
		}
		delete(wanted, key)
		if latency == 0 {
			assertIORequestDistribution(t, row, 2, 1, 7, 4, 4, 6.4, 6.7, 6.94)
		} else {
			assertIORequestDistribution(t, row, 1, latency, latency, latency, latency, latency, latency, latency)
		}
	}
	if len(wanted) != 0 {
		t.Fatalf("sample domains disappeared: %v", wanted)
	}
}

func TestIORequestLatencyDistributionAmbiguityAndOpenTailDoNotPolluteRecoveredPairs(t *testing.T) {
	for _, tc := range []struct{ name, layer, family, start, done string }{
		{"block", "block", blockEndpointFamilyRQ, "block_rq_issue: 8,0 R 4096 () 123 + 8 [io]", "block_rq_complete: 8,0 R () 123 + 8 [0]"},
		{"scsi", "scsi", "scsi_dispatch_cmd", "scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096", "scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body strings.Builder
			for _, event := range []struct {
				ts       float64
				endpoint string
			}{
				{1, tc.start}, {1.001, tc.start}, {1.002, tc.done}, {1.003, tc.done},
				{2, tc.start}, {2.003, tc.done}, {3, tc.start},
			} {
				fmt.Fprintf(&body, "io-40 (40) [003] .... %.6f: %s\n", event.ts, event.endpoint)
			}
			idx := buildTraceIndex(t, "io-recovered-pair.systrace", body.String())
			stats := ComputeWindowStats(idx, Query{})
			row := storageLatencyRow(stats.StorageLatencyByLayer, tc.layer, tc.family)
			assertIORequestDistribution(t, row, 1, 3, 3, 3, 3, 3, 3, 3)
			if row.AmbiguousCohortCount != 1 || row.PairingSuppressedCount != 2 || row.UnpairedStartCount != 1 {
				t.Fatalf("distribution discarded endpoint quality disclosure: %+v", row)
			}
		})
	}
}
