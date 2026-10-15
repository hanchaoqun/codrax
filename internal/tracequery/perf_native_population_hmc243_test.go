package tracequery

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestHMC243NativePopulationPrivateBoundary(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	index, err := BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	r := Run(index, Query{View: "perf_stats", PID: 11, TimeStart: 1, TimeEnd: 2})
	if p, _, ok := NativePerfPopulation(r); !ok || len(p.Cohorts) != 3 {
		t.Fatal("native unfiltered fact population unavailable")
	}
	if _, _, ok := FilteredPerfPopulation(r); ok {
		t.Fatal("unfiltered source gained filtered fallback")
	}
	for _, mode := range []string{"json", "generation", "source", "window", "cohort", "capture"} {
		copy := r
		copy.TraceArtifacts = append([]TraceArtifactSource(nil), r.TraceArtifacts...)
		p, _ := json.Marshal(r.PerfStats)
		var perf PerfContext
		_ = json.Unmarshal(p, &perf)
		copy.PerfStats = &perf
		switch mode {
		case "json":
			wire, _ := json.Marshal(r)
			copy = Result{}
			_ = json.Unmarshal(wire, &copy)
		case "generation":
			copy.TraceArtifacts[0].SourceModUnixNano++
		case "source":
			copy.SourcePath += ".other"
		case "window":
			copy.TimeEnd++
		case "cohort":
			copy.PerfStats.Cohorts[0].Scope.SourceGeneration += ".other"
		case "capture":
			copy.TraceArtifacts[0].CaptureID = "other"
		}
		if _, _, ok := NativePerfPopulation(copy); ok {
			t.Fatalf("%s minted native population witness", mode)
		}
	}
}
