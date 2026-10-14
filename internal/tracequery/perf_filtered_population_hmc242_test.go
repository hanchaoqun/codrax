package tracequery

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilteredPerfPopulationPrivateWitnessHMC242(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	idx, err := BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "window_stats", PID: 11, TimeStart: 1, TimeEnd: 2, Pattern: "dso=/app/A.so", EventTypes: []EventType{EventPerfSample}}
	native := Run(idx, q)
	p, actual, ok := FilteredPerfPopulation(native)
	if !ok || p.SampleCount != 4 || len(p.Cohorts) != 3 {
		t.Fatalf("native own population absent: ok=%t p=%+v", ok, p)
	}
	if actual.Pattern != q.Pattern || len(actual.EventTypes) != 1 {
		t.Fatal("effective filter missing")
	}
	actual.EventTypes[0] = EventSchedSwitch
	_, again, ok := FilteredPerfPopulation(native)
	if !ok || again.EventTypes[0] != EventPerfSample {
		t.Fatal("returned query mutated private receipt")
	}
	for _, name := range []string{"json", "source", "generation", "capture", "kind", "schema", "window", "view", "population", "cohort_generation", "cohort_filter_id", "cancel"} {
		t.Run(name, func(t *testing.T) {
			r := native
			r.TraceArtifacts = append([]TraceArtifactSource(nil), native.TraceArtifacts...)
			if r.WindowStats != nil {
				s := *r.WindowStats
				r.WindowStats = &s
				b, _ := json.Marshal(s.PerfSamples)
				var c PerfContext
				_ = json.Unmarshal(b, &c)
				r.WindowStats.PerfSamples = &c
			}
			switch name {
			case "json":
				b, _ := json.Marshal(native)
				r = Result{}
				_ = json.Unmarshal(b, &r)
			case "source":
				r.SourcePath += ".other"
			case "generation":
				r.TraceArtifacts[0].SourceModUnixNano++
			case "capture":
				r.TraceArtifacts[0].CaptureID = strings.Repeat("a", 64)
			case "kind":
				r.TraceArtifacts[0].Kind = "other"
			case "schema":
				r.TraceArtifacts[0].BundleSchema = "other"
			case "window":
				r.TimeEnd++
			case "view":
				r.View = "perf_stats"
			case "population":
				r.WindowStats.PerfSamples.Cohorts[0].TotalPeriod++
			case "cohort_generation":
				r.WindowStats.PerfSamples.Cohorts[0].Scope.SourceGeneration += "other"
			case "cohort_filter_id":
				r.WindowStats.PerfSamples.Cohorts[0].Scope.ID += "other"
			case "cancel":
				r.ViewCancellation = &ViewCancellation{View: r.View, Reason: "canceled"}
			}
			if _, _, ok := FilteredPerfPopulation(r); ok {
				t.Fatal("changed/copied population minted receipt")
			}
		})
	}
	q.Pattern = ""
	q.EventTypes = nil
	if _, _, ok := FilteredPerfPopulation(Run(idx, q)); ok {
		t.Fatal("unfiltered lane acquired new fallback")
	}
}
