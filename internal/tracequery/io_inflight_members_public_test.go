package tracequery

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func ioInFlightAssertMemberPartition(t *testing.T, group IOInFlightGroup) {
	t.Helper()
	if len(group.Members)+group.OmittedMembers+group.MemberWitnessUnavailableCount != group.AcceptedPairCount {
		t.Fatalf("member partition does not equal accepted pairs: %+v", group)
	}
	if group.OmittedMembers < 0 || group.MemberWitnessUnavailableCount < 0 {
		t.Fatalf("negative member disclosure: %+v", group)
	}
	seen := map[string]bool{}
	for i, m := range group.Members {
		if m.ID == "" || seen[m.ID] || m.SourcePath != group.SourcePath || m.IssueLine <= 0 || m.CompleteLine <= 0 || m.IssueLocalLine <= 0 || m.CompleteLocalLine <= 0 {
			t.Fatalf("member lacks unique native endpoint provenance: %+v", m)
		}
		seen[m.ID] = true
		if i > 0 && ioInFlightMemberLess(m, group.Members[i-1]) {
			t.Fatal("member display borrowed latency rank instead of physical instance order")
		}
	}
}

func TestIOInFlightMembersPublicAcceptedPopulation(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_io_inflight/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "window_stats", PID: 40, TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true}
	result := Run(idx, q)
	if result.WindowStats == nil {
		t.Fatalf("public query failed: %+v", result)
	}
	stats := ioInflightPublicDecode(t, *result.WindowStats)
	want := map[string]struct {
		issues, peak        int
		mean, busy, area    float64
		lines, done, owners []int
	}{
		"block_rq|8,0|R":  {6, 2, 1.4, 10, 14, []int{3, 6, 16, 18}, []int{11, 15, 21, 23}, []int{40, 41, 40, 41}},
		"block_rq|8,0|W":  {2, 1, .6, 6, 6, []int{5, 14}, []int{7, 19}, []int{60, 60}},
		"block_rq|8,1|R":  {1, 1, .4, 4, 4, []int{8}, []int{17}, []int{70}},
		"block_bio|8,0|R": {1, 1, 1, 10, 10, []int{4}, []int{22}, []int{50}},
	}
	if len(stats.Groups) != len(want) {
		t.Fatalf("population changed: %+v", stats.Groups)
	}
	for _, g := range stats.Groups {
		key := strings.Join([]string{g.EndpointFamily, g.Dev, g.Operation}, "|")
		w, ok := want[key]
		if !ok {
			t.Fatalf("unexpected independent population %q", key)
		}
		ioInflightPublicValues(t, g, len(w.lines), w.issues, w.peak, w.mean, w.busy, w.area)
		ioInFlightAssertMemberPartition(t, g)
		if g.OmittedMembers != 0 || g.MemberWitnessUnavailableCount != 0 || len(g.Members) != len(w.lines) {
			t.Fatalf("admitted native endpoints lost witnesses: %+v", g)
		}
		var area float64
		for i, m := range g.Members {
			if m.IssueLine != w.lines[i] || m.IssueLocalLine != w.lines[i] || m.CompleteLine != w.done[i] || m.CompleteLocalLine != w.done[i] || m.IssueThread.PID != w.owners[i] || m.WindowContributionMs == nil {
				t.Errorf("wrong contributor or endpoint instance: %+v", m)
			}
			area += *m.WindowContributionMs
			// Physical completion owner is the emitter (80/82/83/84), not
			// its parenthesized TGID=2, and not the request's issue owner.
			if m.CompleteThread.TGID != 2 || m.CompleteThread.PID < 80 {
				t.Errorf("completion ownership rewritten: %+v", m.CompleteThread)
			}
		}
		if math.Abs(area-g.Values.RequestMs) > 1e-6 {
			t.Fatalf("complete member contribution did not reconcile: %g vs %g", area, g.Values.RequestMs)
		}
	}
	// Neither ambiguous sector 9000 nor the unclosed sector 10000 becomes
	// a member; issue_count intentionally still includes their starts.
	if result.RootCauseRank != nil {
		t.Fatal("measurement acquired root-cause permission")
	}
}

func TestIOInFlightMembersPublicProjectionAndZero(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		pair                                 ioInflightPublicPair
		start, end, clippedStart, clippedEnd float64
	}{
		{"carry_in", ioInflightPublicPair{.998, 1.004}, 1, 1.01, 1, 1.004},
		{"carry_out", ioInflightPublicPair{1.006, 1.014}, 1, 1.01, 1.006, 1.01},
		{"both_sides", ioInflightPublicPair{.998, 1.014}, 1, 1.01, 1, 1.01},
		{"zero_origin_ns", ioInflightPublicPair{0, .000000001}, 0, .000000010, 0, .000000001},
		{"zero_duration", ioInflightPublicPair{0, 0}, 0, .01, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stats := ioInflightPublicStats(t, ioInflightPublicRQ(tc.pair), tc.start, tc.end)
			g := ioInflightPublicDecode(t, stats).Groups[0]
			ioInFlightAssertMemberPartition(t, g)
			if len(g.Members) != 1 {
				t.Fatalf("paired member not retained: %+v", g)
			}
			m := g.Members[0]
			if m.ActualStartTs != tc.pair.start || m.ActualEndTs != tc.pair.end || m.WindowContribution == nil || m.WindowContribution.StartTs != tc.clippedStart || m.WindowContribution.EndTs != tc.clippedEnd || m.WindowContributionMs == nil || math.Abs(*m.WindowContributionMs-(tc.clippedEnd-tc.clippedStart)*1000) > 1e-10 {
				t.Fatalf("actual versus window contribution conflated: %+v", m)
			}
			wire, _ := json.Marshal(m)
			if tc.pair.start == 0 && !strings.Contains(string(wire), `"actual_start_ts":0`) || tc.pair.end == 0 && !strings.Contains(string(wire), `"actual_end_ts":0`) {
				t.Fatalf("real zero omitted from wire: %s", wire)
			}
		})
	}
}

func TestIOInFlightMembersPublicWindowUnavailable(t *testing.T) {
	idx := buildTraceIndex(t, "line-point.systrace", ioInflightPublicRQ(ioInflightPublicPair{1, 1.003}))
	for _, q := range []Query{
		{View: "window_stats", LineStart: 1, LineEnd: 2},
		{View: "window_stats", LineStart: 1, LineEnd: 2, TimeStart: 50, TimeEnd: 60, TimeStartSet: true, TimeEndSet: true},
		{View: "window_stats", TimeStart: 1.002, TimeEnd: 1.002, TimeStartSet: true, TimeEndSet: true},
	} {
		result := Run(idx, q)
		g := ioInflightPublicDecode(t, *result.WindowStats).Groups[0]
		ioInFlightAssertMemberPartition(t, g)
		if len(g.Members) != 1 || g.Members[0].WindowContribution != nil || g.Members[0].WindowContributionMs != nil || g.Values != nil {
			t.Fatalf("missing selected denominator was rendered as measured zero: %+v", g)
		}
		if g.Members[0].ActualStartTs != 1 || g.Members[0].ActualEndTs != 1.003 {
			t.Fatal("line/point selection lost accepted physical interval")
		}
	}
}

func TestIOInFlightMembersPublicFullPopulationBeforeCaps(t *testing.T) {
	var pairs []ioInflightPublicPair
	for i := 0; i < 20; i++ {
		start := 1 + float64(i*2+1)/1000
		pairs = append(pairs, ioInflightPublicPair{start, start + .001})
	}
	// True peak and 30 request-ms are entirely beyond retained witnesses.
	for i := 0; i < 3; i++ {
		pairs = append(pairs, ioInflightPublicPair{1.070, 1.080})
	}
	idx := buildTraceIndex(t, "caps.systrace", ioInflightPublicRQ(pairs...))
	q := Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.100, Limit: 1}
	before, _ := json.Marshal(idx.Events)
	result := Run(idx, q)
	g := ioInflightPublicDecode(t, *result.WindowStats).Groups[0]
	ioInflightPublicValues(t, g, 23, 23, 3, .5, 30, 50)
	ioInFlightAssertMemberPartition(t, g)
	if len(result.WindowStats.IOLatencies) != 8 || len(g.Members) != IOInFlightMemberLimit || g.OmittedMembers != 7 || g.MemberWitnessUnavailableCount != 0 || len(g.Segments) != 16 || g.OmittedSegments == 0 {
		t.Fatalf("independent caps or omission counts changed: %+v", g)
	}
	for i, m := range g.Members {
		if m.IssueLocalLine != 2*i+1 || m.CompleteLocalLine != 2*i+2 || m.ActualStartTs >= 1.070 {
			t.Fatalf("latency Top-N contaminated chronological witnesses: %+v", m)
		}
	}
	// Native successful pairs are the only input; the census shared with
	// latency details is not mutated to obtain the new member order.
	legacy := computeBlockIOLatencies(idx, q, 8)
	if !reflect.DeepEqual(result.WindowStats.IOLatencies, legacy.latencies) {
		t.Fatal("new members changed legacy latency output")
	}
	result.WindowStats.IOInFlight.Groups[0].Members[0].IssueThread.Comm = "mutated"
	*result.WindowStats.IOInFlight.Groups[0].Members[0].WindowContributionMs = 999
	again := Run(idx, q)
	if !reflect.DeepEqual(g, again.WindowStats.IOInFlight.Groups[0]) {
		t.Fatal("member result aliases a later query or query index")
	}
	after, _ := json.Marshal(idx.Events)
	if string(before) != string(after) {
		t.Fatal("read-only witness projection mutated source events")
	}
}

func TestIOInFlightMembersPublicGenericStorage(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 2; i++ {
		fmt.Fprintf(&body, "storage-worker-40 (90) [003] .... %.9f: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096\n", 1.001+float64(i)*.004)
		fmt.Fprintf(&body, "storage-done-40 (90) [003] .... %.9f: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096\n", 1.003+float64(i)*.004)
	}
	stats := ioInflightPublicStats(t, body.String(), 1, 1.01)
	g := ioInflightPublicDecode(t, stats).Groups[0]
	ioInflightPublicValues(t, g, 2, 2, 1, .4, 4, 4)
	ioInFlightAssertMemberPartition(t, g)
	if g.Layer != "scsi" || g.EndpointFamily != "scsi_dispatch_cmd" || len(g.Members) != 2 {
		t.Fatalf("generic success endpoint path not used: %+v", g)
	}
	for i, m := range g.Members {
		if m.IssueLocalLine != 2*i+1 || m.CompleteLocalLine != 2*i+2 || m.IssueThread.Comm != "storage-worker" || m.CompleteThread.Comm != "storage-done" || m.IssueThread.PID != 40 || m.CompleteThread.PID != 40 || m.IssueThread.TGID != 90 {
			t.Fatalf("generic pair owner/physical identity was reconstructed incorrectly: %+v", m)
		}
	}
}
