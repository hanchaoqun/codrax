package tool

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceQueryScopeCoverageParams(t *testing.T, raw string) traceQueryParams {
	t.Helper()
	var params traceQueryParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		t.Fatal(err)
	}
	return params
}

func TestTraceQueryFullArtifactScopeCoverageAllowsRelationFiltersButNotDerivedWindows(t *testing.T) {
	result := tracequery.Result{
		View:          "thread_timeline",
		SourcePath:    "/trace/customer.systrace",
		IndexWindowed: false,
		TimeStart:     34579.450627,
		TimeEnd:       34579.595184,
	}
	params := traceQueryScopeCoverageParams(t, `{
		"view":"thread_timeline",
		"pid":59566,
		"event_types":["sched_switch"],
		"limit":8
	}`)
	record, ok := traceQueryFullArtifactScopeCoverageObservation(
		result, params, "attached_trace", "payload.json", "raw.txt", time.Unix(1, 0),
	)
	if !ok {
		t.Fatal("unbounded canonical query did not mint full-artifact scope coverage")
	}
	if record.Predicate != types.RuntimeArtifactScopeCoveragePredicate ||
		record.Object != string(types.RuntimeArtifactScopeFullArtifact) ||
		record.Scope != string(types.RuntimeArtifactScopeFullArtifact) ||
		record.Value != "thread_timeline" {
		t.Fatalf("unexpected coverage record: %+v", record)
	}
	if coverage := types.CompileRuntimeArtifactScopeCoverage(
		types.ObservationLedger{Records: []types.ObservationRecord{record}}, nil,
	); !coverage.FullArtifact() {
		t.Fatalf("producer record did not survive the unified consumer: %+v", coverage)
	}

	rejects := []struct {
		name   string
		raw    string
		result tracequery.Result
	}{
		{"time bound", `{"view":"thread_timeline","pid":59566,"time_start":1,"time_end":2}`, result},
		{"line bound", `{"view":"thread_timeline","pid":59566,"line_start":10,"line_end":20}`, result},
		{"pattern derived", `{"view":"frame_window","pattern":"frame-42"}`, result},
		{"span derived", `{"view":"span_window","span_name":"RenderFrame"}`, result},
		{"recipe derived", `{"view":"root_cause_rank","recipe_name":"jank"}`, result},
		{"windowed index", `{"view":"thread_timeline","pid":59566}`, func() tracequery.Result {
			windowed := result
			windowed.IndexWindowed = true
			return windowed
		}()},
	}
	for _, tc := range rejects {
		t.Run(tc.name, func(t *testing.T) {
			p := traceQueryScopeCoverageParams(t, tc.raw)
			if got, ok := traceQueryFullArtifactScopeCoverageObservation(
				tc.result, p, "attached_trace", "payload.json", "raw.txt", time.Unix(1, 0),
			); ok {
				t.Fatalf("bounded/derived query minted full coverage: %+v", got)
			}
		})
	}
}
