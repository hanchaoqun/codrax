package tracequery

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEventSearchScanScopeSeparatesSelectorObservedAndMatched(t *testing.T) {
	path := writeTraceMarkIntegrityTrace(t, "scope.trace",
		traceMarkTestLine("writer", 10, 1, "I|20|needle"),
		"writer-10 (10) [001] .... 1.500000: unrecognized_vendor_event: value=1",
		traceMarkTestLine("writer", 11, 2, "I|20|other"),
		traceMarkTestLine("writer", 10, 3, "I|20|outside"))
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		q       Query
		matched int
	}{
		{"filtered", Query{TimeStart: .5, TimeEnd: 2.5, Pattern: "needle"}, 1},
		{"all", Query{TimeStart: .5, TimeEnd: 2.5}, 3},
		{"subject", Query{TimeStart: .5, TimeEnd: 2.5, PID: 11}, 1},
		{"empty_matches", Query{TimeStart: .5, TimeEnd: 2.5, Pattern: "missing"}, 0},
		{"empty_window", Query{TimeStart: 10, TimeEnd: 20}, 0},
		{"line_precedence", Query{TimeStart: 10, TimeEnd: 20, LineStart: 1, LineEnd: 1}, 1},
		{"inclusive_edge", Query{TimeStart: 1, TimeEnd: 1}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.q
			q.View = "event_search"
			q.Limit = 1
			indexed := Run(idx, q)
			streamed, err := StreamEventSearch(context.Background(), path, q)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range []Result{indexed, streamed} {
				c := r.EventSearchCoverage
				if c == nil || c.ScanScope == nil || !types.ValidateTraceEventSearchScanScope(c.ScanScope) || c.MatchedTotal != tc.matched {
					t.Fatalf("invalid census: %+v", c)
				}
				s := c.ScanScope
				if q.LineStart > 0 {
					if s.TimeStartApplied || s.TimeEndApplied || s.LineStart != 1 || s.LineEnd != 1 {
						t.Fatalf("line/time drift: %+v", s)
					}
				} else if !s.TimeStartApplied || !s.TimeEndApplied || s.TimeStart != q.TimeStart || s.TimeEnd != q.TimeEnd {
					t.Fatalf("selector replaced by envelope: %+v", s)
				}
				if tc.name == "filtered" && (c.ScopeTimeStart != 1 || c.ScopeTimeEnd != 2 || c.MatchedTimeStart != 1 || c.MatchedTimeEnd != 1) {
					t.Fatalf("envelopes conflated: %+v", c)
				}
				if tc.name == "empty_window" && s.ObservedCount != 0 {
					t.Fatalf("empty scan fabricated observation: %+v", s)
				}
				if tc.name == "empty_matches" && s.ObservedCount == 0 {
					t.Fatal("no match erased observed population")
				}
				wire, _ := json.Marshal(c)
				var restored EventSearchCoverage
				if err := json.Unmarshal(wire, &restored); err != nil || !reflect.DeepEqual(c, &restored) {
					t.Fatal("scope does not roundtrip")
				}
			}
			if tc.name == "filtered" && (indexed.EventSearchCoverage.ScanScope.ObservedBasis != "parsed_events" || streamed.EventSearchCoverage.ScanScope.ObservedBasis != "physical_timestamp_rows" || streamed.EventSearchCoverage.ScanScope.ObservedCount != 3) {
				t.Fatal("parse/scanned population conflated")
			}
		})
	}
	stopCtx, stop := context.WithCancel(context.Background())
	stop()
	if r := Run(idx, Query{View: "event_search"}.WithRunContext(stopCtx)); r.EventSearchCoverage != nil {
		t.Fatal("cancellation published complete scope")
	}
	if _, err := StreamEventSearch(stopCtx, path, Query{View: "event_search"}); err == nil {
		t.Fatal("stream cancellation lost")
	}
}

func TestEventSearchScanScopeZeroAndLegacyAppliedBounds(t *testing.T) {
	path := writeTraceMarkIntegrityTrace(t, "zero.trace", traceMarkTestLine("writer", 10, 0, "I|20|zero"))
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	result := Run(idx, Query{View: "event_search", TimeStartSet: true, TimeEndSet: true})
	c := result.EventSearchCoverage
	if c == nil || c.ScanScope == nil || c.ScanScope.ObservedCount != 1 || c.ScopeTimeStart != 0 || c.ScopeTimeEnd != 0 || c.ScanScope.TimeEndApplied {
		t.Fatalf("zero or actual legacy bound changed: %+v", c)
	}
}
