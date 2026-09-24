package tracediag

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestDeferredResourceBulkCannotEvictPublicRankDetail(t *testing.T) {
	_, path, _ := writeRunFixtures(t, runTestScript)
	idx, err := tracequery.BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	res := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", PID: 20, TimeStart: 1, TimeEnd: 1.6, TimeStartSet: true, TimeEndSet: true})
	if res.RootCauseRank == nil || res.WindowStats == nil || res.WindowStats.SchedulerConcurrency == nil {
		t.Fatalf("fixture lacks real rank/background: %+v", res)
	}
	before, _ := json.Marshal(res)
	bounded := renderStepBody(&Step{View: "root_cause_rank", effMaxLines: 200}, stepOutcome{result: &res})
	text := strings.Join(bounded.lines, "\n")
	for _, want := range []string{"明细 root_cause_rank:", "root_cause_rank.items[0]"} {
		if !strings.Contains(text, want) {
			t.Fatalf("causal detail evicted under unchanged 200-line cap: missing=%q\n%s", want, text)
		}
	}
	baseline := res
	stats := *res.WindowStats
	stats.IOInFlight, stats.SchedulerConcurrency, stats.BusinessTree, stats.IOActivity = nil, nil, nil, nil
	baseline.WindowStats = &stats
	var core, full []string
	renderNonEventResultDetail(&baseline, func(s string) { core = append(core, s) })
	renderNonEventResultDetail(&res, func(s string) { full = append(full, s) })
	if len(full) <= len(core) || !reflect.DeepEqual(full[:len(core)], core) {
		t.Fatal("optional background changed/evicted ordinary sibling detail prefix")
	}
	if !strings.Contains(strings.Join(full[len(core):], "\n"), "window_stats.scheduler_concurrency.groups") {
		t.Fatal("deferral silently dropped native background")
	}
	after, _ := json.Marshal(res)
	if string(before) != string(after) {
		t.Fatal("display scheduling mutated native result")
	}
}

func TestDeferredResourceBulkRetainsAllLinesAndNestedCausalDetail(t *testing.T) {
	for _, family := range []string{"io_inflight", "scheduler_concurrency", "business_tree", "io_activity"} {
		t.Run(family, func(t *testing.T) {
			stats := &tracequery.WindowStats{Window: tracequery.TimeWindow{StartTs: 1, EndTs: 2}}
			switch family {
			case "io_inflight":
				stats.IOInFlight = &tracequery.IOInFlightStats{Population: "accepted_complete_pairs", IssuerScope: "all_issuers"}
			case "scheduler_concurrency":
				stats.SchedulerConcurrency = &tracequery.SchedulerConcurrencyStats{Population: "accepted_closed_intervals", ThreadScope: "all_positive_tids"}
			case "business_tree":
				stats.BusinessTree = &tracequery.TraceMarkerTreeStats{Coverage: "unknown_prefix"}
			case "io_activity":
				stats.IOActivity = &tracequery.IOActivityStats{Population: "observed_endpoint_events"}
			}
			rank := &tracequery.RootCauseRankResult{Items: []tracequery.RootCauseRankItem{{Rank: 1, Summary: "proven on-chain cause"}}}
			res := &tracequery.Result{View: "frame_root_cause_bundle", WindowStats: stats,
				FrameRootCauseBundle: &tracequery.FrameRootCauseBundle{RootCauseRank: rank}}
			var scheduled, previous []string
			renderNonEventResultDetail(res, func(s string) { scheduled = append(scheduled, s) })
			// Prior local-only policy is retained as a lossless content oracle:
			// this change may reorder, never delete or synthesize a detail line.
			renderResultDetailWithPolicy(res, func(s string) { previous = append(previous, s) }, &nonEventDetailPolicy)
			text := strings.Join(scheduled, "\n")
			causePos := strings.Index(text, "frame_root_cause_bundle.root_cause_rank.items[0]")
			bulkPos := strings.Index(text, "window_stats."+family+":")
			if causePos < 0 || bulkPos < causePos {
				t.Fatalf("nested causal detail must precede %s:\n%s", family, text)
			}
			sort.Strings(scheduled)
			sort.Strings(previous)
			if !reflect.DeepEqual(scheduled, previous) {
				t.Fatalf("deferral changed the complete line population: got=%v want=%v", scheduled, previous)
			}
			full := renderStepBody(&Step{View: res.View, effMaxLines: 1000}, stepOutcome{result: res})
			for _, cap := range []int{1, 5, len(full.lines)} {
				bounded := renderStepBody(&Step{View: res.View, effMaxLines: cap}, stepOutcome{result: res})
				if bounded.total != full.total || !reflect.DeepEqual(bounded.lines, full.lines[:len(bounded.lines)]) {
					t.Fatalf("cap %d changed output census or prefix: bounded=%+v full=%+v", cap, bounded, full)
				}
			}
		})
	}
}

func TestDeferredOnlyPayloadKeepsOneHeaderAndPerRenderQueue(t *testing.T) {
	res := &tracequery.Result{WindowStats: &tracequery.WindowStats{
		SchedulerConcurrency: &tracequery.SchedulerConcurrencyStats{Population: "accepted_closed_intervals"},
	}}
	var first []string
	renderNonEventResultDetail(res, func(s string) { first = append(first, s) })
	for i := 0; i < 3; i++ {
		var next []string
		renderNonEventResultDetail(res, func(s string) { next = append(next, s) })
		if !reflect.DeepEqual(first, next) || strings.Count(strings.Join(next, "\n"), "明细 window_stats:") != 1 {
			t.Fatalf("deferred-only header/queue leaked across rendering: %v", next)
		}
	}
}
