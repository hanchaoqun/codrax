package tool

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestLegacySamplingScopeSurvivesSummaryAndObservations(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	r := tracequery.Run(idx, tracequery.Query{View: "window_stats", PID: 11, TimeStart: 1, TimeEnd: 2})
	text := traceQuerySummary(r, traceQueryParams{View: r.View}, "path", "/tmp/native-result.json")
	for _, cohort := range r.WindowStats.PerfSamples.Cohorts {
		if cohort.Scope == nil {
			t.Fatal("missing native scope")
		}
		if !strings.Contains(text, "cohort_id="+cohort.Scope.ID) || !strings.Contains(text, "producer="+cohort.Scope.Producer) {
			t.Fatalf("summary loses exact cohort scope: %s", text)
		}
	}
	rows := traceQueryTypedObservations(r, path, "/tmp/native-result.json", "/tmp/native-result.txt", "", time.Unix(1, 0))
	count := 0
	claims := map[string]bool{}
	for _, row := range rows {
		if row.Predicate != "perf_sample_top_symbol" {
			continue
		}
		count++
		if strings.Contains(row.ID, "#evidence_fact:") {
			if !strings.Contains(row.Summary, "denominator=") || !strings.Contains(row.Summary, "producer=") {
				t.Fatalf("generic evidence lost denominator/source: %+v", row)
			}
			continue
		}
		if !strings.Contains(row.Summary, "cohort_id=") || !strings.Contains(row.Summary, "denominator_weight=") || !strings.Contains(row.ClaimKey, "cohort=") {
			t.Fatalf("old observation can detach percentage from scope: %+v", row)
		}
		if claims[row.ClaimKey] {
			t.Fatalf("same symbol in independent DSO/cohort shares claim: %s", row.ClaimKey)
		}
		claims[row.ClaimKey] = true
	}
	if count < 3 {
		t.Fatalf("missing separate producer/event observations: %d", count)
	}
	for _, row := range rows {
		if row.Predicate != "perf_sample_top_symbol" || strings.Contains(row.ID, "#evidence_fact:") {
			continue
		}
		projected := types.ProjectObservationPromptRecords([]types.ObservationRecord{row}, nil, nil, types.DefaultObservationPromptProjectionOptions(1))
		if len(projected) != 1 || !strings.Contains(projected[0].Summary, "denominator_weight=") || !strings.Contains(projected[0].Summary, "producer=") {
			t.Fatalf("compact finalizer loses source/denominator: %+v", projected)
		}
	}
}
