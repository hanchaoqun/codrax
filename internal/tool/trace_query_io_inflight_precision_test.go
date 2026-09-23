package tool

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const ioInFlightPublicNumberPattern = `[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?`

// Real parsing/public execution precedes every display assertion. These cases
// distinguish nonzero sub-microsecond measurements from genuine measured zero;
// they do not change the existing endpoint population or require exact tails.
func TestIOInFlightPublicPrecisionAndScope(t *testing.T) {
	const tiny = "io-40 (40) [003] .... 1.000000100: block_rq_issue: 8,0 R 4096 () 8 + 8 [io]\n" +
		"irq-2 (2) [003] .... 1.000000400: block_rq_complete: 8,0 R () 8 + 8 [0]\n"
	for _, tc := range []struct {
		name       string
		start, end float64
	}{
		{"tiny_selected_window", 1.0000001, 1.0000004},
		{"tiny_occupancy_in_wide_window", 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, stats, row, compact := ioInFlightPrecisionPublicQuery(t, tiny, tc.start, tc.end, 0, 0)
			g := stats.Groups[0]
			if stats.Window == nil || stats.Window.StartTs != tc.start || stats.Window.EndTs != tc.end || g.Values == nil || g.Values.PeakRequests != 1 || g.Values.BusyMs <= 0 || g.Values.BusyMs >= .001 {
				t.Fatalf("native fixture did not prove a tiny positive interval: %+v", stats)
			}
			for surface, text := range map[string]string{"summary": ioInFlightPrecisionSummary(t, result.Summary), "typed": row.Summary + " " + strings.Join(row.RichNotes, " "), "semantic": compact} {
				for key, want := range map[string]float64{"peak_requests": 1, "mean_requests": g.Values.MeanRequests, "busy_ms": g.Values.BusyMs, "request_ms": g.Values.RequestMs} {
					ioInFlightPrecisionNumber(t, surface, text, key, want)
				}
				window := regexp.MustCompile(`selected_window=(` + ioInFlightPublicNumberPattern + `)\.\.(` + ioInFlightPublicNumberPattern + `)`).FindStringSubmatch(text)
				if len(window) != 3 {
					t.Errorf("%s omitted the measured window: %s", surface, text)
				} else {
					start, _ := strconv.ParseFloat(window[1], 64)
					end, _ := strconv.ParseFloat(window[2], 64)
					if start != tc.start || end != tc.end || end <= start {
						t.Errorf("%s rounded/collapsed the measured window: %s", surface, window[0])
					}
				}
				segments := regexp.MustCompile(`\[(`+ioInFlightPublicNumberPattern+`),(`+ioInFlightPublicNumberPattern+`)\)(?: seconds)?\s*:\s*1`).FindAllStringSubmatch(text, -1)
				if len(segments) == 0 {
					t.Errorf("%s omitted the first positive depth segment: %s", surface, text)
				}
				for _, segment := range segments {
					start, _ := strconv.ParseFloat(segment[1], 64)
					end, _ := strconv.ParseFloat(segment[2], 64)
					if start != 1.0000001 || end != 1.0000004 || end <= start {
						t.Errorf("%s changed the positive segment: %s", surface, segment[0])
					}
				}
			}
		})
	}
	t.Run("line_selected_population", func(t *testing.T) {
		body := "io-40 (40) [003] .... 1.001000: block_rq_issue: 8,0 R 4096 () 8 + 8 [io]\n" +
			"irq-2 (2) [003] .... 1.003000: block_rq_complete: 8,0 R () 8 + 8 [0]\n" +
			"io-40 (40) [003] .... 1.006000: block_rq_issue: 8,0 R 4096 () 16 + 8 [io]\n" +
			"irq-2 (2) [003] .... 1.008000: block_rq_complete: 8,0 R () 16 + 8 [0]\n"
		result, stats, row, compact := ioInFlightPrecisionPublicQuery(t, body, 1, 1.01, 3, 4)
		g := stats.Groups[0]
		if stats.LineStart != 3 || stats.LineEnd != 4 || g.AcceptedPairCount != 1 || !row.SourceRef.QueryLineRangeKnown || row.SourceRef.QueryLineStart != 3 || row.SourceRef.QueryLineEnd != 4 {
			t.Fatalf("native fixture did not preserve line-filtered pairing: %+v / %+v", stats, row.SourceRef)
		}
		for surface, text := range map[string]string{"summary": ioInFlightPrecisionSummary(t, result.Summary), "typed": row.Summary + " " + strings.Join(row.RichNotes, " "), "semantic": compact} {
			if !strings.Contains(text, "lines=3..4") {
				t.Errorf("%s hides the line-selected population behind a time window: %s", surface, text)
			}
			if !strings.Contains(text, "line_bounds_take_precedence") {
				t.Errorf("%s omitted the line-domain denominator boundary: %s", surface, text)
			}
		}
	})
}

func TestIOInFlightPublicUnavailableReason(t *testing.T) {
	const issue = "io-40 (40) [003] .... 1.001000: block_rq_issue: 8,0 R 4096 () 8 + 8 [io]\n"
	const done = "irq-2 (2) [003] .... 1.003000: block_rq_complete: 8,0 R () 8 + 8 [0]\n"
	for _, tc := range []struct {
		name, body, reason string
		start, end         float64
	}{
		{"missing_completion", issue, "no_accepted_complete_pairs", 1, 1.01},
		{"point_window", issue + done, "finite_positive_time_window_not_determined", 1.002, 1.002},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, stats, row, compact := ioInFlightPrecisionPublicQuery(t, tc.body, tc.start, tc.end, 0, 0)
			g := stats.Groups[0]
			if g.Values != nil || g.ValuesUnavailableReason != tc.reason || row.Value != "" {
				t.Fatalf("native fixture did not preserve unknown versus zero: %+v / %+v", g, row)
			}
			for surface, text := range map[string]string{"summary": ioInFlightPrecisionSummary(t, result.Summary), "typed": row.Summary + " " + strings.Join(row.RichNotes, " "), "semantic": compact} {
				if !strings.Contains(text, tc.reason) {
					t.Errorf("%s lost the exact unavailability reason %s: %s", surface, tc.reason, text)
				}
				if strings.Contains(text, "peak_requests=0") || strings.Contains(text, "mean_requests=0") {
					t.Errorf("%s published unknown occupancy as measured zero: %s", surface, text)
				}
			}
		})
	}
}

func ioInFlightPrecisionPublicQuery(t *testing.T, body string, start, end float64, lineStart, lineEnd int) (types.ToolResult, *tracequery.IOInFlightStats, types.ObservationRecord, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "precision.systrace")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	physicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("Describe measured IO occupancy and its scope")}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": start, "time_end": end, "line_start": lineStart, "line_end": lineEnd})
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("real TraceQuery execution failed: %v %+v", err, result)
	}
	var row types.ObservationRecord
	for _, candidate := range result.Observations {
		if candidate.Predicate == "io_inflight" {
			if row.ID != "" {
				t.Fatal("fixture unexpectedly produced multiple occupancy groups")
			}
			row = candidate
		}
	}
	if row.ID == "" || row.SourceRef.Path != physicalPath || row.SourceRef.PayloadRef == "" || row.Role != types.AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("real producer omitted source-bound noncausal occupancy: %+v", row)
	}
	data, err := os.ReadFile(row.SourceRef.PayloadRef)
	if err != nil {
		t.Fatal(err)
	}
	var native tracequery.Result
	if err := json.Unmarshal(data, &native); err != nil || native.WindowStats == nil || native.WindowStats.IOInFlight == nil || len(native.WindowStats.IOInFlight.Groups) != 1 {
		t.Fatalf("actual JSON payload prerequisite failed: %v %+v", err, native.WindowStats)
	}
	projected := types.ProjectObservationPromptRecords([]types.ObservationRecord{row}, nil, nil, types.SemanticReviewObservationPromptProjectionOptions(1))
	if len(projected) != 1 || len(projected[0].Notes) > 6 {
		t.Fatalf("shared compact projection exceeded its unchanged budget: %+v", projected)
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || string(unchanged) != body {
		t.Fatal("query changed original trace bytes")
	}
	return result, native.WindowStats.IOInFlight, row, projected[0].Summary + " " + strings.Join(projected[0].Notes, " ")
}

func ioInFlightPrecisionNumber(t *testing.T, surface, text, key string, want float64) {
	t.Helper()
	match := regexp.MustCompile(`\b` + regexp.QuoteMeta(key) + `=(` + ioInFlightPublicNumberPattern + `)`).FindStringSubmatch(text)
	if len(match) != 2 {
		t.Errorf("%s omitted %s: %s", surface, key, text)
		return
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil || value <= 0 || math.Abs(value-want) > math.Abs(want)*1e-7 {
		t.Errorf("%s changed positive %s: got %q want %g", surface, key, match[1], want)
	}
}

func ioInFlightPrecisionSummary(t *testing.T, summary string) string {
	t.Helper()
	start := strings.Index(summary, "- io_inflight ")
	if start < 0 {
		t.Fatal("actual public summary omitted the occupancy section")
	}
	return summary[start:]
}
