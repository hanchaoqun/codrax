package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQueryRankDisplayAuthorityBothTextFaces(t *testing.T) {
	for _, tc := range []struct {
		name, relevance, want string
		pid                   int
	}{
		{"targetless", "", "background", 0},
		{"targetless_relation", "on_chain", "background", 0},
		{"target_only", "", "background", 41},
		{"adjacent", "adjacent", "adjacent", 41},
		{"on_chain", "on_chain", "chain", 41},
	} {
		t.Run(tc.name, func(t *testing.T) {
			board := &tracequery.RootCauseRankResult{Target: tracequery.ThreadRef{PID: tc.pid}, Items: []tracequery.RootCauseRankItem{{
				Rank: 1, Tier: "primary", Type: "io_latency", Thread: tracequery.ThreadRef{PID: 42},
				ChainRelevance: tc.relevance, IOValueCaliber: "block_rq_issue_to_complete", CumulativeImpactMs: 35, EffectiveImpactMs: 35,
				Summary: "original request interval", LineStart: 10, LineEnd: 20,
			}}}
			before, _ := json.Marshal(board)
			got := traceQuerySummary(tracequery.Result{View: "root_cause_rank", RootCauseRank: board}, traceQueryParams{View: "root_cause_rank"}, "attached_trace", "/tmp/rank.json")
			after, _ := json.Marshal(board)
			if string(before) != string(after) {
				t.Fatal("text presentation mutated the native rank board")
			}
			for _, prefix := range []string{"- root_cause_rank_preview_row ", "- rank=1 "} {
				found := false
				for _, line := range strings.Split(got, "\n") {
					if !strings.HasPrefix(line, prefix) {
						continue
					}
					found = true
					for _, want := range []string{"rank_channel=" + tc.want, "io_value_caliber=block_rq_issue_to_complete", "block request residence", "effective_impact=35.000ms"} {
						if !strings.Contains(line, want) {
							t.Errorf("text face %q lacks %q: %s", prefix, want, line)
						}
					}
				}
				if !found {
					t.Errorf("missing text face %q: %s", prefix, got)
				}
			}
		})
	}
}
