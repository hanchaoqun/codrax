package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

var samplingSelectionViewsHMC235 = []string{"perf_sampling_tree", "perf_stats", "window_stats", "trace_perf_bundle", "perf_timeline"}

func TestSamplingSelectionPublicAliasesHMC235(t *testing.T) {
	for _, view := range samplingSelectionViewsHMC235 {
		for _, selector := range []map[string]any{
			{"pid": 11}, {"thread": "document-worker"}, {"thread": "11"},
			{"thread": "document-worker-11"}, {"thread": "document-worker-11 (tid=11)"},
			{"thread": "document-worker [11]"}, {"thread": "thread=document-worker tid=11"},
			{"pid": 11, "thread": "document-worker-11 (tid=11)"},
		} {
			t.Run(fmt.Sprintf("%s/%v", view, selector), func(t *testing.T) {
				selector["view"] = view
				r := samplingSelectionPublicRecord(t, selector)
				before, _ := json.Marshal(r)
				p, ok := DecodeTracePerfSamplingTree(r)
				if !ok {
					t.Fatalf("native producer/outer selection binding disagrees: %+v", r.SourceRef)
				}
				if p.Status != "available" || p.SampleCount != 5 || p.CohortCount != 3 || p.OmittedCohorts != 0 {
					t.Fatalf("selector changed the source population: %+v", p)
				}
				if subject, selected := TraceQueryAggregateSelectionSubject(r); !selected || subject != "document-worker-11" {
					t.Fatalf("consistent native selection lost display identity: %q %t", subject, selected)
				}
				weights := map[string]int64{}
				for _, cohort := range p.Cohorts {
					weights[cohort.Producer+"/"+cohort.Event] = cohort.TotalWeight
					if cohort.SourcePath != r.SourceRef.Path || cohort.SourceGeneration == "" || cohort.ThreadIdentity == nil || cohort.ThreadIdentity.TID != 11 {
						t.Fatalf("selector lost source/thread generation: %+v", cohort)
					}
					if cohort.Producer == "raw_perfdata_fallback" && cohort.Event == "cycles" && (cohort.StackWeight != 30 || cohort.UnavailableWeight != 5) {
						t.Fatalf("unknown stack disappeared from denominator: %+v", cohort)
					}
				}
				if weights["raw_perfdata_fallback/cycles"] != 35 || weights["simpleperf_report_sample/cycles"] != 7 || weights["raw_perfdata_fallback/instructions"] != 100 {
					t.Fatalf("selector mixed independent sample domains: %v", weights)
				}
				text := TracePerfSamplingTreeText(p)
				for _, literal := range []string{`"sys;root"`, "/app/A.so", "/app/B.so"} {
					if !strings.Contains(text, literal) {
						t.Fatalf("native frame boundary/source lost %q: %s", literal, text)
					}
				}
				if r.Role != types.AnswerAggregateRoleSupportingCoverage {
					t.Fatal("sampling selection gained causal/principal authority")
				}
				after, _ := json.Marshal(r)
				if !bytes.Equal(before, after) {
					t.Fatal("display selection mutated the original native record")
				}
			})
		}
	}
}

func TestSamplingSelectionPublicConflictsHMC235(t *testing.T) {
	for _, view := range samplingSelectionViewsHMC235 {
		for _, selector := range []map[string]any{
			{"pid": 11, "thread": "document-worker (tid=12)"},
			{"thread": "document-worker [12] (tid=11)"},
			{"thread": "document-worker tid=11 pid=12"},
		} {
			t.Run(fmt.Sprintf("%s/%v", view, selector), func(t *testing.T) {
				selector["view"] = view
				r := samplingSelectionPublicRecord(t, selector)
				if subject, selected := TraceQueryAggregateSelectionSubject(r); selected {
					t.Fatalf("conflicting native selector was laundered into target %q", subject)
				}
				p, ok := DecodeTracePerfSamplingTree(r)
				if !ok || p.Status != "unavailable" || p.SampleCount != 0 || len(p.Cohorts) != 0 || p.Reason != "invalid_thread_selector" {
					t.Fatalf("conflicting selector needs a source-bound unavailable receipt: ok=%t p=%+v", ok, p)
				}
				raw, err := os.ReadFile(r.SourceRef.PayloadRef)
				if err != nil {
					t.Fatal(err)
				}
				var payload tracequery.Result
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatal(err)
				}
				if payload.PerfStats != nil && payload.PerfStats.SampleCount != 0 || payload.WindowStats != nil && payload.WindowStats.PerfSamples != nil && payload.WindowStats.PerfSamples.SampleCount != 0 {
					t.Fatal("flat sampling statistics bypassed the invalid selector")
				}
				if payload.PerfTimeline != nil && len(payload.PerfTimeline.Buckets) != 0 {
					t.Fatal("timeline sampling statistics bypassed the invalid selector")
				}
			})
		}
	}
}

func TestSamplingSelectionPublicNumericCommHMC235(t *testing.T) {
	path := filepath.Join(t.TempDir(), "numeric-name.perftrace")
	data := "worker-3-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 thread_comm=worker-3 sample_weight=3 event=cycles symbol=work source=raw\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	for _, view := range samplingSelectionViewsHMC235 {
		for _, selector := range []map[string]any{{"thread": "worker-3 (tid=11)"}, {"pid": 11, "thread": "worker-3"}, {"thread": "worker-3-11 (tid=11)"}} {
			selector["view"], selector["path"] = view, path
			r := samplingSelectionPublicRecord(t, selector)
			p, ok := DecodeTracePerfSamplingTree(r)
			if !ok || p.Status != "available" || p.SampleCount != 1 || p.TargetPID != 11 || p.TargetThread != "worker-3" {
				t.Fatalf("literal numeric comm was treated as a conflicting TID: %v %+v", selector, p)
			}
			if subject, selected := TraceQueryAggregateSelectionSubject(r); !selected || subject != "worker-3-11" {
				t.Fatalf("canonical name was reparsed as another TID: %v %q", selector, subject)
			}
		}
	}
}

func TestSamplingSelectionPublicScopeBoundariesHMC235(t *testing.T) {
	for _, view := range samplingSelectionViewsHMC235 {
		for _, selector := range []map[string]any{
			{"pid": 12}, {"thread": "document"},
			{"pid": 11, "time_start": 2, "time_end": 2.5}, {"pid": 11, "pattern": "work"},
		} {
			t.Run(fmt.Sprintf("%s/%v", view, selector), func(t *testing.T) {
				selector["view"] = view
				r := samplingSelectionPublicRecord(t, selector)
				if subject, selected := TraceQueryAggregateSelectionSubject(r); selected {
					t.Fatalf("empty/fuzzy/unsupported selection gained target authority: %q", subject)
				}
				p, ok := DecodeTracePerfSamplingTree(r)
				if !ok {
					t.Fatal("scope boundary lost its honest source-bound receipt")
				}
				if selector["pid"] == 12 || selector["time_start"] == 2 {
					if p.SampleCount != 0 || len(p.Cohorts) != 0 {
						t.Fatal("empty selection fell back to another thread/window")
					}
				}
			})
		}
		// Process scope is rejected before native sampling acquisition. Preserve
		// that existing public boundary rather than requiring a success receipt.
		dir := t.TempDir()
		ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("sampling")}
		path, err := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
		if err != nil {
			t.Fatal(err)
		}
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "target_scope": "process", "pid": 11, "time_start": 1, "time_end": 2})
		result, err := (&TraceQuery{}).Execute(ctx, params)
		if err != nil || result.Success || len(result.Observations) != 0 {
			t.Fatalf("unsupported process scope gained sampling evidence: %v %+v", err, result)
		}
	}
	path := filepath.Join(t.TempDir(), "two-threads.perftrace")
	data := "worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 thread_comm=worker sample_weight=3 event=cycles symbol=work source=raw\n" +
		"worker-12 (10) [000] .... 1.2: perf_sample: pid=10 tid=12 thread_comm=worker sample_weight=5 event=cycles symbol=work source=raw\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	for _, view := range samplingSelectionViewsHMC235 {
		r := samplingSelectionPublicRecord(t, map[string]any{"view": view, "path": path, "thread": "worker"})
		p, ok := DecodeTracePerfSamplingTree(r)
		if !ok || p.SampleCount != 2 || p.CohortCount != 2 {
			t.Fatalf("multi-TID search lost its distinct cohorts: %+v", p)
		}
		if subject, selected := TraceQueryAggregateSelectionSubject(r); selected {
			t.Fatalf("shared name borrowed one TID's identity: %q", subject)
		}
	}
}
