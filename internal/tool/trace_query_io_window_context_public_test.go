package tool

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Public Execute and its stored native result are the prerequisites: these
// assertions concern evidence presentation, not a second IO pairing model.
func TestIOWindowContextPublicEndpointTimes(t *testing.T) {
	for _, tc := range []struct {
		name, body  string
		start, end  float64
		issue, done string
	}{
		{"carry_in", "io-40 (40) [003] .... 0.998000: block_rq_issue: 8,0 R 4096 () 8 + 8 [io]\nirq-2 (2) [003] .... 1.004000: block_rq_complete: 8,0 R () 8 + 8 [0]\n", 1, 1.01, "0.998000", "1.004000"},
		{"carry_out", "io-40 (40) [003] .... 1.008000: block_rq_issue: 8,0 R 4096 () 8 + 8 [io]\nirq-2 (2) [003] .... 1.012000: block_rq_complete: 8,0 R () 8 + 8 [0]\n", 1, 1.01, "1.008000", "1.012000"},
		{"sub_microsecond", "io-40 (40) [003] .... 1.000000100: block_rq_issue: 8,0 R 4096 () 8 + 8 [io]\nirq-2 (2) [003] .... 1.000000400: block_rq_complete: 8,0 R () 8 + 8 [0]\n", 1, 2, "1.0000001", "1.0000004"},
		{"real_zero_start", "io-40 (40) [003] .... 0.000000: block_rq_issue: 8,0 R 4096 () 8 + 8 [io]\nirq-2 (2) [003] .... 0.004000: block_rq_complete: 8,0 R () 8 + 8 [0]\n", 0, .01, "0.000000", "0.004000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, _, row, _ := ioInFlightPrecisionPublicQuery(t, tc.body, tc.start, tc.end, 0, 0)
			data, err := os.ReadFile(row.SourceRef.PayloadRef)
			if err != nil {
				t.Fatal(err)
			}
			var native tracequery.Result
			if err := json.Unmarshal(data, &native); err != nil || native.WindowStats == nil || len(native.WindowStats.IOLatencies) != 1 {
				t.Fatalf("real endpoint-pair prerequisite failed: %v %+v", err, native.WindowStats)
			}
			pair := native.WindowStats.IOLatencies[0]
			if pair.IssueLine != 1 || pair.CompleteLine != 2 || pair.CompleteTs <= pair.IssueTs {
				t.Fatalf("actual parser did not establish both endpoints: %+v", pair)
			}
			latency := traceQuerySummaryLineWithPrefix(result.Summary, "- io_latency ")
			_, evidence, found := strings.Cut(result.Summary, "## Evidence pack\n")
			if !found {
				t.Fatal("public result omitted evidence pack")
			}
			fact := traceQuerySummaryLineWithPrefix(evidence, "- io-40 io_latency ")
			for surface, text := range map[string]string{"latency": latency, "evidence": fact} {
				for _, want := range []string{"issue_ts=" + tc.issue, "complete_ts=" + tc.done, "seconds", "request interval is unclipped", "occupancy uses its intersection with the query window"} {
					if !strings.Contains(text, want) {
						t.Errorf("%s omitted the physical request boundary %q: %s", surface, want, text)
					}
				}
			}
		})
	}
}

func TestIOWindowContextDoesNotInventMissingEndpointTimes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		start, end float64
	}{
		{"unknown_default", 0, 0},
		{"reverse", 2, 1},
		{"nan", math.NaN(), 1},
		{"infinite", 1, math.Inf(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := tracequery.Result{View: "window_stats", WindowStats: &tracequery.WindowStats{
				IOLatencies: []tracequery.IOLatencySummary{{IssueTs: tc.start, CompleteTs: tc.end}},
			}, EvidencePack: []tracequery.EvidenceFact{{Subject: "io", Predicate: "io_latency", StartTs: tc.start, EndTs: tc.end}}}
			text := traceQuerySummary(result, traceQueryParams{}, "runtime_artifact:test", "")
			if strings.Contains(text, "issue_ts=") || strings.Contains(text, "complete_ts=") {
				t.Fatalf("unknown/invalid interval became physical endpoint evidence: %s", text)
			}
		})
	}
}

func TestIOWindowContextPublicCountPopulationMeaning(t *testing.T) {
	const body = "io-40 (40) [003] .... 0.998000: block_rq_issue: 8,0 R 4096 () 8 + 8 [io]\n" +
		"io-40 (40) [003] .... 1.001000: block_rq_issue: 8,0 R 4096 () 16 + 8 [io]\n" +
		"irq-2 (2) [003] .... 1.004000: block_rq_complete: 8,0 R () 8 + 8 [0]\n" +
		"io-40 (40) [003] .... 1.008000: block_rq_issue: 8,0 R 4096 () 24 + 8 [io]\n" +
		"irq-2 (2) [003] .... 1.012000: block_rq_complete: 8,0 R () 24 + 8 [0]\n"
	result, stats, row, compact := ioInFlightPrecisionPublicQuery(t, body, 1, 1.01, 0, 0)
	if stats.Groups[0].AcceptedPairCount != 2 || stats.Groups[0].IssueCount != 2 || stats.Coverage[0].UnpairedStartCount != 1 {
		t.Fatalf("different native populations prerequisite failed: %+v", stats)
	}
	for surface, text := range map[string]string{"public": result.Summary, "typed": row.Summary, "compact": compact} {
		for _, want := range []string{"完整配对请求=2", "范围内发起=2", "非完成次数/覆盖率"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s loses count population meaning %q: %s", surface, want, text)
			}
		}
	}
}

func TestIOWindowContextPublicSharedTeaching(t *testing.T) {
	query := &TraceQuery{}
	var schema struct {
		Properties map[string]struct{ Description string } `json:"properties"`
	}
	if err := json.Unmarshal(query.Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	for surface, text := range map[string]string{"description": query.Description(), "schema": schema.Properties["view"].Description, "matrix": skill.RenderTraceQueryViewMatrix()} {
		if strings.Count(text, skill.TraceIOInFlightTeaching) != 1 {
			t.Errorf("%s duplicates or omits the shared contract", surface)
		}
		for _, want := range []string{"accepted_pair_count counts complete requests intersecting the query, including carry-in/out, not in-window completions", "It and issue_count have different populations: their ratio is not coverage", "Full request residence is unclipped; occupancy uses only the intersection with the selected window"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s omits shared population teaching %q", surface, want)
			}
		}
	}
}
