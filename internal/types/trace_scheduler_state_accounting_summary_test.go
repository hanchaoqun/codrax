package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSchedulerStateAccountingSummaryRetainsAllClosureClasses(t *testing.T) {
	mixed := TraceSchedulerStateAccounting{State: "runnable", Caliber: "cumulative_segments", SegmentCount: 3,
		ObservedEndCount: 1, ObservedEndMs: 2, OpenTailCount: 1, OpenTailMs: 1, UnknownClosureCount: 1, UnknownClosureMs: 4,
		StartClippedCount: 1, EndClippedCount: 1, BoundaryContinuationCount: 1}
	for _, tc := range []struct {
		name    string
		account *TraceSchedulerStateAccounting
		want    string
	}{
		{"mixed", &mixed, "account=cumulative(boundary=1,open_tail=1/1ms,unknown=1/4ms)"},
		{"nil", nil, "account=cumulative(closure=unknown)"},
		{"malformed", &TraceSchedulerStateAccounting{State: "running", Caliber: "cumulative_segments", SegmentCount: 1}, "account=cumulative(closure=unknown)"},
		{"closed", &TraceSchedulerStateAccounting{State: "running", Caliber: "cumulative_segments", SegmentCount: 2, ObservedEndCount: 2, ObservedEndMs: 2}, "account=cumulative(boundary=2)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := json.Marshal(tc.account)
			if got := TraceSchedulerStateAccountingSummary(tc.account); got != tc.want || len(got) > 80 {
				t.Fatalf("wrong or verbose summary: %s", got)
			}
			after, _ := json.Marshal(tc.account)
			if string(before) != string(after) {
				t.Fatal("summary changed native times/clipping/continuations")
			}
		})
	}
	var accounts []TraceSchedulerStateAccounting
	for _, state := range []string{"running", "runnable", "s_sleep", "d_sleep", "io_wait"} {
		a := mixed
		a.State = state
		accounts = append(accounts, a)
	}
	before, _ := json.Marshal(accounts)
	got := TraceSchedulerStateAccountsSummary(accounts)
	for _, a := range accounts {
		if !strings.Contains(got, a.State+":boundary=1,open_tail=1/1ms,unknown=1/4ms") {
			t.Fatalf("state account collapsed: %s", got)
		}
	}
	if strings.Count(got, "cumulative") != 1 || len(got) >= len(TraceSchedulerStateAccountsMeaning(accounts, false))/3 {
		t.Fatalf("repeated per-state teaching still dominates: %s", got)
	}
	after, _ := json.Marshal(accounts)
	if string(before) != string(after) {
		t.Fatal("multi-state summary changed complete DTO")
	}
}
