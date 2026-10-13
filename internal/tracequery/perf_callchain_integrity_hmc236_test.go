package tracequery

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestPerfCallchainLegacyViewsKeepFrameIntegrityHMC236(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{ViewPerfSamplingTree, "perf_stats", "window_stats", "trace_perf_bundle", "perf_timeline"} {
		t.Run(view, func(t *testing.T) {
			r := Run(idx, Query{View: view, PID: 11, TimeStart: 1, TimeEnd: 2})
			if r.PerfSamplingTree == nil || r.PerfSamplingTree.SampleCount != 5 || r.PerfSamplingTree.CohortCount != 3 {
				t.Fatal("native population changed")
			}
			for _, c := range r.PerfSamplingTree.Cohorts {
				if c.Event == "cycles" && c.Producer == "raw_perfdata_fallback" && (c.TotalWeight != 35 || c.StackWeight != 30 || c.UnavailableWeight != 5) {
					t.Fatalf("native frame coverage changed: %+v", c)
				}
				if c.Event == "cycles" && c.Producer == "raw_perfdata_fallback" {
					weights := map[string]int64{}
					for _, n := range c.Nodes {
						weights[n.DSO] = n.InclusiveWeight
						if n.Symbol == "work" && n.SelfWeight != n.InclusiveWeight || n.Depth == 0 && (n.Symbol != "sys;root" || n.InclusiveWeight != 30) {
							t.Fatalf("literal root or exact leaf weight changed: %+v", n)
						}
					}
					if weights["/app/A.so"] != 10 || weights["/app/B.so"] != 20 {
						t.Fatalf("flat A=15 leaked into exact tree: %v", weights)
					}
				}
			}
			stats := r.PerfStats
			if stats == nil && r.WindowStats != nil {
				stats = r.WindowStats.PerfSamples
			}
			if stats == nil {
				return // Standalone tree/timeline do not synthesize a flat context.
			}
			for _, c := range stats.Cohorts {
				if c.Event != "cycles" || c.Scope.Producer != "raw_perfdata_fallback" {
					continue
				}
				if c.TotalPeriod != 35 || c.SampleCount != 3 {
					t.Fatal("unknown full stack was removed from the denominator")
				}
				if c.TopSymbols[1].DSO != "/app/A.so" || c.TopSymbols[1].Period != 15 {
					t.Fatal("flat observed symbol weight must retain the legacy sample")
				}
				if len(c.TopCallchains) != 3 {
					t.Fatalf("exact and legacy paths merged: %+v", c.TopCallchains)
				}
				for _, hot := range c.TopCallchains {
					if hot.CallchainIdentity == nil || hot.Callchain != "sys;root;work" {
						t.Fatalf("path identity or original label lost: %+v", hot)
					}
					if hot.Period == 5 {
						if hot.CallchainIdentity.Form != "opaque_flat_label" || hot.CallchainIdentity.Reason != "legacy_frame_boundaries_unavailable" || hot.CallchainIdentity.ID != "" {
							t.Fatalf("unknown stack gained an exact identity: %+v", hot)
						}
					} else if hot.CallchainIdentity.Form != "exact_frames" || hot.CallchainIdentity.FrameCount != 2 || hot.CallchainIdentity.ID == "" {
						t.Fatalf("exact path lost typed identity: %+v", hot)
					}
				}
				if c.Quality.CallchainKnownCount != 2 || c.Quality.CallchainUnknownCount != 1 {
					t.Fatalf("producer symbolized tag claimed exact frame coverage: %+v", c.Quality)
				}
				statusWeight := map[string]int64{}
				for _, status := range c.Quality.FrameStatuses {
					statusWeight[status.Value] = status.Period
				}
				if statusWeight["exact_frames"] != 30 || statusWeight["legacy_frame_boundaries_unavailable"] != 5 || c.Quality.CallchainStatuses[0].Period != 35 {
					t.Fatalf("typed frame coverage or producer labels lost: %+v", c.Quality)
				}
			}
		})
	}
}

func TestPerfCallchainExactBoundariesDoNotMeanCompleteUnwindHMC236(t *testing.T) {
	frames := []tracewire.PerfFrame{{IP: "0xabc", SourceRef: "partial:1", SourceDepth: 4}}
	body := fmt.Sprintf("worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=7 event=cycles symbol=work dso=a.so callchain=\"unresolved\" source=raw callchain_status=partial frames_v1=%s\n", tracewire.QuotePerfKVValue(tracewire.EncodePerfFrames(frames, "available")))
	r := Run(buildTraceIndex(t, "partial.perftrace", body), Query{View: "perf_stats", PID: 11, TimeStart: 1, TimeEnd: 2})
	q := r.PerfStats.Quality
	if q.CallchainKnownCount != 1 || q.CallchainUnknownCount != 0 || q.CallchainStatuses[0].Value != "partial" || q.FrameStatuses[0].Value != "exact_frames" {
		t.Fatalf("producer partial declaration and exact recorded boundary must coexist: %+v", q)
	}
	if !strings.Contains(strings.Join(q.Caveats, " "), "not proof of exact frames or complete unwinding") || r.PerfStats.TopCallchains[0].CallchainIdentity.FrameCount != 1 {
		t.Fatal("available carrier gained a complete-unwind claim or invented frames")
	}
}

func TestPerfCallchainIdentityUsesFullLiteralFramesHMC236(t *testing.T) {
	var body string
	frames := []tracewire.PerfFrame{{Symbol: "sys;root", DSO: "sys.so", IP: "0x1", SourceRef: "f0"}, {Symbol: "work", DSO: "a.so", IP: "0x2", SourceRef: "f1", SourceDepth: 1}}
	add := func(weight int, fs []tracewire.PerfFrame) {
		body += fmt.Sprintf("worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=%d event=cycles symbol=work dso=a.so callchain=\"sys;root;work\" source=raw callchain_status=symbolized frames_v1=%s\n", weight, tracewire.QuotePerfKVValue(tracewire.EncodePerfFrames(fs, "available")))
	}
	add(2, frames)
	frames[0].SourceRef = "other-record"
	add(3, frames) // Occurrence coordinates are not path identity.
	frames[1].IP = "0x3"
	add(7, frames)
	frames[0].Symbol = "sys"
	frames = append(frames[:1:1], tracewire.PerfFrame{Symbol: "root", DSO: "sys.so", IP: "0x4", SourceRef: "f2", SourceDepth: 1}, frames[1])
	add(11, frames) // Same flat label, different literal frame boundaries.
	idx := buildTraceIndex(t, "identity.perftrace", body)
	r := Run(idx, Query{View: "perf_stats", PID: 11, TimeStart: 1, TimeEnd: 2})
	paths := r.PerfStats.TopCallchains
	var weights []int
	for _, p := range paths {
		if p.CallchainIdentity == nil || p.CallchainIdentity.Form != "exact_frames" {
			t.Fatalf("missing exact identity: %+v", p)
		}
		weights = append(weights, int(p.Period))
	}
	sort.Ints(weights)
	if fmt.Sprint(weights) != "[5 7 11]" {
		t.Fatalf("path identity lost DSO/IP/frame boundaries or used source coordinates: %v", weights)
	}
}

func TestPerfCallchainIntegrityFailureKindsHMC236(t *testing.T) {
	cases := []struct{ carrier, reason string }{
		{"", "legacy_frame_boundaries_unavailable"},
		{"{broken", "malformed_frame_carrier"},
		{tracewire.EncodePerfFrames(nil, "missing"), "missing"},
		{tracewire.EncodePerfFrames(nil, "source_depths_unavailable"), "source_depths_unavailable"},
		{tracewire.EncodePerfFrames(nil, "frame_budget_or_metadata_unavailable"), "frame_budget_or_metadata_unavailable"},
	}
	var body strings.Builder
	for i, tc := range cases {
		fmt.Fprintf(&body, "worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=%d event=cycles symbol=work dso=a.so callchain=\"root;work\" source=raw callchain_status=symbolized frames_v1=%s\n", i+1, tracewire.QuotePerfKVValue(tc.carrier))
	}
	idx := buildTraceIndex(t, "unknown.perftrace", body.String())
	r := Run(idx, Query{View: "perf_stats", PID: 11, TimeStart: 1, TimeEnd: 2})
	if r.PerfStats.TotalPeriod != 15 || r.PerfStats.TopSymbols[0].Period != 15 || r.PerfStats.Quality.CallchainKnownCount != 0 || r.PerfStats.Quality.CallchainUnknownCount != 5 {
		t.Fatalf("unknown records disappeared or gained complete paths: %+v", r.PerfStats)
	}
	if len(r.PerfStats.TopCallchains) != len(cases) {
		t.Fatal("different unknown reasons merged")
	}
	for _, tc := range cases {
		if r.PerfSamplingTree.Cohorts[0].UnavailableReasons[tc.reason] != 1 {
			t.Fatalf("tree/flat failure classification diverged for %s", tc.reason)
		}
	}
}
