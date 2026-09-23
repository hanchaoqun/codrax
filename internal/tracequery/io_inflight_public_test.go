package tracequery

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
)

// These fixtures enter through the real text parser and public query surface.
// Distinct sectors prove distinct requests; issue counts alone cannot prove
// concurrency, and no hand-built Event or synthesized pairing receipt is used.
type ioInflightPublicPair struct{ start, end float64 }

func ioInflightPublicRQ(pairs ...ioInflightPublicPair) string {
	type row struct {
		ts   float64
		text string
	}
	var rows []row
	for i, p := range pairs {
		rows = append(rows, row{p.start, fmt.Sprintf("io-40 (40) [003] .... %.9f: block_rq_issue: 8,0 R 4096 () %d + 8 [io]\n", p.start, i*8)})
		rows = append(rows, row{p.end, fmt.Sprintf("irq-2 (2) [003] .... %.9f: block_rq_complete: 8,0 R () %d + 8 [0]\n", p.end, i*8)})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ts < rows[j].ts })
	var body strings.Builder
	for _, row := range rows {
		body.WriteString(row.text)
	}
	return body.String()
}

func ioInflightPublicStats(t *testing.T, body string, start, end float64) WindowStats {
	t.Helper()
	idx := buildTraceIndex(t, "inflight.systrace", body)
	result := Run(idx, Query{View: "window_stats", TimeStart: start, TimeEnd: end, TimeStartSet: true, TimeEndSet: true})
	if result.WindowStats == nil {
		t.Fatalf("public Run omitted window stats: %+v", result)
	}
	return *result.WindowStats
}

// The first RED used an independent JSON wire struct before these public
// types existed; subsequent cases decode the same real serialized surface.
func ioInflightPublicDecode(t *testing.T, stats WindowStats) *IOInFlightStats {
	t.Helper()
	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	var outer struct {
		InFlight *IOInFlightStats `json:"io_inflight"`
	}
	if err := json.Unmarshal(data, &outer); err != nil {
		t.Fatal(err)
	}
	if outer.InFlight == nil {
		t.Fatal("accepted real IO pairs have no io_inflight public measurement")
	}
	if outer.InFlight.Population != "accepted_complete_pairs" || outer.InFlight.IssuerScope != "all_issuers" {
		t.Fatalf("measurement omitted its population/scope: %+v", outer.InFlight)
	}
	return outer.InFlight
}

func ioInflightPublicValues(t *testing.T, g IOInFlightGroup, pairs, issues, peak int, mean, busy, requestMS float64) {
	t.Helper()
	if g.Values == nil || g.AcceptedPairCount != pairs || g.IssueCount != issues || g.Values.PeakRequests != peak {
		t.Fatalf("pair/issue count, availability or overlap drifted: %+v", g)
	}
	for key, pair := range map[string][2]float64{"mean": {g.Values.MeanRequests, mean}, "busy": {g.Values.BusyMs, busy}, "request_ms": {g.Values.RequestMs, requestMS}} {
		if math.IsNaN(pair[0]) || math.IsInf(pair[0], 0) || math.Abs(pair[0]-pair[1]) > 1e-6 {
			t.Errorf("%s=%g want=%g", key, pair[0], pair[1])
		}
	}
}

func TestIOInflightPublicSerialVersusOverlap(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		pairs                 []ioInflightPublicPair
		peak                  int
		mean, busy, requestMS float64
	}{
		{"serial", []ioInflightPublicPair{{1.001, 1.003}, {1.005, 1.007}}, 1, .4, 4, 4},
		{"overlap", []ioInflightPublicPair{{1.001, 1.005}, {1.003, 1.007}}, 2, .8, 6, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stats := ioInflightPublicStats(t, ioInflightPublicRQ(tc.pairs...), 1, 1.01)
			// Existing accepted-pair output must work before testing the new
			// contract: malformed fixture input is not a valid product RED.
			if len(stats.IOLatencies) != 2 || len(stats.StorageLatencyByLayer) != 1 || stats.StorageLatencyByLayer[0].PairedCount != 2 {
				t.Fatalf("fixture did not produce two exact native pairs: %+v", stats.StorageLatencyByLayer)
			}
			wire := ioInflightPublicDecode(t, stats)
			if len(wire.Groups) != 1 || wire.Groups[0].Values == nil {
				t.Fatalf("one complete group required: %+v", wire)
			}
			ioInflightPublicValues(t, wire.Groups[0], 2, 2, tc.peak, tc.mean, tc.busy, tc.requestMS)
		})
	}
}

func TestIOInflightPublicWindowProjectionAndZero(t *testing.T) {
	for _, tc := range []struct {
		name            string
		pairs           []ioInflightPublicPair
		start, end      float64
		issues, peak    int
		mean, requestMS float64
	}{
		{"carry_in", []ioInflightPublicPair{{.998, 1.004}}, 1, 1.010, 0, 1, .4, 4},
		{"carry_out", []ioInflightPublicPair{{1.006, 1.014}}, 1, 1.010, 1, 1, .4, 4},
		{"spans_whole_window", []ioInflightPublicPair{{.998, 1.014}}, 1, 1.010, 0, 1, 1, 10},
		{"equal_boundary_no_false_overlap", []ioInflightPublicPair{{1.001, 1.003}, {1.003, 1.005}}, 1, 1.010, 2, 1, .4, 4},
		{"zero_duration_pair", []ioInflightPublicPair{{1.004, 1.004}}, 1, 1.010, 1, 0, 0, 0},
		{"real_zero_origin", []ioInflightPublicPair{{0, .004}}, 0, .010, 1, 1, .4, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stats := ioInflightPublicStats(t, ioInflightPublicRQ(tc.pairs...), tc.start, tc.end)
			wire := ioInflightPublicDecode(t, stats)
			if wire.Window == nil || wire.Window.StartTs != tc.start || wire.Window.EndTs != tc.end || len(wire.Groups) != 1 {
				t.Fatalf("determined query window or group lost: %+v", wire)
			}
			ioInflightPublicValues(t, wire.Groups[0], len(tc.pairs), tc.issues, tc.peak, tc.mean, tc.requestMS, tc.requestMS)
			g := wire.Groups[0]
			if g.OmittedSegments == 0 {
				area, busy, last := 0.0, 0.0, tc.start
				for _, seg := range g.Segments {
					if math.Abs(seg.StartTs-last) > 1e-12 || seg.EndTs <= seg.StartTs || seg.Requests < 0 {
						t.Fatalf("segment discontinuity/invalid depth: %+v", g.Segments)
					}
					ms := (seg.EndTs - seg.StartTs) * 1000
					area += ms * float64(seg.Requests)
					if seg.Requests > 0 {
						busy += ms
					}
					last = seg.EndTs
				}
				if math.Abs(last-tc.end) > 1e-12 || math.Abs(area-tc.requestMS) > 1e-6 || math.Abs(busy-tc.requestMS) > 1e-6 {
					t.Fatalf("segments disagree with full-window values: %+v", g)
				}
			}
			data, err := json.Marshal(g.Values)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"peak_requests", "mean_requests", "busy_ms", "request_ms"} {
				if !strings.Contains(string(data), `"`+key+`":`) {
					t.Fatalf("measured zero disappeared from JSON: %s", data)
				}
			}
		})
	}
}

func TestIOInflightPublicFullPopulationSurvivesTopEight(t *testing.T) {
	var pairs []ioInflightPublicPair
	for i := 0; i < 11; i++ {
		start := 1 + float64(i*4+1)/1000
		pairs = append(pairs, ioInflightPublicPair{start, start + .001})
	}
	stats := ioInflightPublicStats(t, ioInflightPublicRQ(pairs...), 1, 1.100)
	if len(stats.IOLatencies) != 8 || len(stats.StorageLatencyByLayer) != 1 || stats.StorageLatencyByLayer[0].PairedCount != 11 {
		t.Fatalf("fixture must retain original Top-8 display and complete population: %+v", stats.StorageLatencyByLayer)
	}
	wire := ioInflightPublicDecode(t, stats)
	if len(wire.Groups) != 1 || wire.OmittedGroups != 0 {
		t.Fatalf("same group must not truncate its numeric population: %+v", wire)
	}
	ioInflightPublicValues(t, wire.Groups[0], 11, 11, 1, .11, 11, 11)
	if len(wire.Groups[0].Segments) != 16 || wire.Groups[0].OmittedSegments != 7 {
		t.Fatalf("bounded segments must disclose the remaining 23-piece profile without changing totals: %+v", wire.Groups[0])
	}
}
