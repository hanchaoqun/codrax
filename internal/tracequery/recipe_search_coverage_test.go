package tracequery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRecipeSearchPreservesSubqueryCoverage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locator.systrace")
	body := "app-20 (20) [001] .... 1.000000: tracing_mark_write: I|20|phase\n" +
		"app-20 (20) [001] .... 2.000000: tracing_mark_write: I|20|phase\n" +
		"app-20 (20) [001] .... 3.000000: tracing_mark_write: I|20|phase\n" +
		"app-20 (20) [001] .... 4.000000: tracing_mark_write: I|20|other\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		q         Query
		matched   int
		compacted bool
	}{
		{"capped", Query{Pattern: "phase", Limit: 1}, 3, true},
		{"exact_limit", Query{Pattern: "phase", Limit: 3}, 3, false},
		{"uncapped", Query{Pattern: "phase", Limit: 8}, 3, false},
		{"empty", Query{Pattern: "absent", Limit: 1}, 0, false},
		{"time_window", Query{Pattern: "phase", Limit: 1, TimeStart: 1.5, TimeEnd: 3.5}, 2, true},
		{"line_window_overrides_time", Query{Pattern: "phase", Limit: 1, LineStart: 2, LineEnd: 3, TimeStart: 100, TimeEnd: 200}, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.q
			q.View = "event_search"
			direct := Run(idx, q)
			q.View, q.RecipeName = "recipe", "span_locate"
			composed := Run(idx, q)
			if composed.EventSearchCoverage == nil || !composed.EventSearchCoverage.EnumerationComplete || composed.EventSearchCoverage.MatchedTotal != tc.matched {
				t.Fatalf("recipe discarded child accounting: %+v", composed.EventSearchCoverage)
			}
			if !reflect.DeepEqual(direct.EventSearchCoverage, composed.EventSearchCoverage) || !reflect.DeepEqual(direct.Events, composed.Events) {
				t.Fatalf("recipe changed child envelope: direct=%+v recipe=%+v", direct.EventSearchCoverage, composed.EventSearchCoverage)
			}
			for _, result := range []Result{direct, composed} {
				if hasEventSearchCompaction(result.Compactions) != tc.compacted {
					t.Fatalf("%s compaction disagrees with complete census: %+v", result.View, result.Compactions)
				}
				for _, compact := range result.Compactions {
					if compact.View == FallbackViewEventSearch && compact.Total != tc.matched {
						t.Fatalf("lost exact matched total: %+v", compact)
					}
				}
			}
			armed, stop := context.WithCancel(context.Background())
			defer stop()
			if got := Run(idx, q.WithRunContext(armed)); !reflect.DeepEqual(got, composed) {
				t.Fatal("unfired cancellation changed recipe result")
			}
			stop()
			if got := Run(idx, q.WithRunContext(armed)); got.EventSearchCoverage != nil || len(got.Events) != 0 || got.ViewCancellation == nil {
				t.Fatal("canceled recipe published a complete search face")
			}
		})
	}
}

func TestRecipeSearchCancellationRetainsOnlyCompletedFaces(t *testing.T) {
	var body strings.Builder
	// Exceed the cooperative 64Ki scan sampling interval; a smaller fixture
	// never samples cancellation inside the exhaustive census.
	const rows = 70000
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&body, "app-20 (20) [001] .... %.6f: tracing_mark_write: I|20|phase\n", 1+float64(i)/1000)
	}
	idx := buildTraceIndex(t, "recipe-search-cancel.systrace", body.String())
	q := Query{View: "recipe", RecipeName: "span_locate", Pattern: "phase", Limit: 1}
	baseline := Run(idx, q)
	if baseline.EventSearchCoverage == nil || baseline.EventSearchCoverage.MatchedTotal != rows {
		t.Fatal("fixture lacks a complete search baseline")
	}
	sawRetainedDisplay := false
	for cut := 1; cut <= 60; cut++ {
		res := Run(idx, q.WithRunContext(newRunCancelAfterN(cut)))
		if len(res.Events) != 0 && !reflect.DeepEqual(res.Events, baseline.Events) {
			t.Fatalf("cut=%d published partial or changed search rows", cut)
		}
		if res.EventSearchCoverage != nil && !reflect.DeepEqual(res.EventSearchCoverage, baseline.EventSearchCoverage) {
			t.Fatalf("cut=%d published incomplete enumeration as complete: %+v", cut, res.EventSearchCoverage)
		}
		if len(res.Events) == 1 && res.EventSearchCoverage == nil {
			sawRetainedDisplay = true
			if res.ViewCancellation == nil || !hasEventSearchCompaction(res.Compactions) {
				t.Fatalf("cut=%d retained capped rows without cancellation/unknown-total disclosure: %+v", cut, res.Compactions)
			}
			for _, compact := range res.Compactions {
				if compact.View == FallbackViewEventSearch && compact.Total != 0 {
					t.Fatalf("cut=%d invented a complete matched total: %+v", cut, compact)
				}
			}
		}
		if res.ViewCancellation == nil {
			break
		}
	}
	if !sawRetainedDisplay {
		t.Fatal("cancellation sweep did not exercise the completed-display/incomplete-census boundary")
	}
}

func TestSearchCoverageUsesEffectiveSpanWindow(t *testing.T) {
	idx := buildTraceIndex(t, "span-scope.systrace", "app-20 (20) [001] .... 1.000000: tracing_mark_write: I|20|phase\n"+
		"app-20 (20) [001] .... 2.000000: tracing_mark_write: B|20|OpenDocument\n"+
		"app-20 (20) [001] .... 2.500000: tracing_mark_write: I|20|phase\n"+
		"app-20 (20) [001] .... 3.000000: tracing_mark_write: E|20\n"+
		"app-20 (20) [001] .... 4.000000: tracing_mark_write: I|20|phase\n"+
		"app-20 (20) [001] .... 5.000000: tracing_mark_write: I|20|other\n")
	for _, tc := range []struct {
		name       string
		q          Query
		start, end float64
		matched    int
	}{
		{"derived", Query{}, 2, 3, 1},
		{"explicit_start", Query{TimeStart: 1.5}, 2, 3, 1},
		{"explicit_end", Query{TimeEnd: 3.5}, 2, 3, 1},
		{"empty_matches", Query{Pattern: "missing"}, 2, 3, 0},
		{"line_window_owns_scope", Query{LineStart: 1, LineEnd: 6}, 1, 5, 3},
	} {
		for _, view := range []string{"event_search", "recipe"} {
			t.Run(tc.name+"/"+view, func(t *testing.T) {
				q := tc.q
				q.View, q.RecipeName, q.SpanName, q.Limit = view, "span_locate", "OpenDocument", 10
				if q.Pattern == "" {
					q.Pattern = "phase"
				}
				res := Run(idx, q)
				coverage := res.EventSearchCoverage
				if coverage == nil || coverage.ScopeKind != EventSearchScopeSelectedWindow ||
					coverage.ScopeTimeStart != tc.start || coverage.ScopeTimeEnd != tc.end ||
					coverage.MatchedTotal != tc.matched || len(res.Events) != tc.matched {
					t.Fatalf("effective window misreported: coverage=%+v events=%d query=%.3f..%.3f", coverage, len(res.Events), res.TimeStart, res.TimeEnd)
				}
			})
		}
	}
}
