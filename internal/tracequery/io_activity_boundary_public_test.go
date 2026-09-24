package tracequery

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

func ioActivityRQLine(ts, op string) string {
	return "io-40 (40) [001] .... " + ts + ": block_rq_issue: 8,0 " + op + " 4096 () 8 + 8 [io]\n"
}

func TestIOActivityPublicWindowSelectionAndNativeTail(t *testing.T) {
	idx := buildTraceIndex(t, "selection.systrace", ioActivityRQLine("0", "R")+ioActivityRQLine("0.100", "W"))
	for _, tc := range []struct {
		name       string
		q          Query
		count      int
		rate, tail bool
	}{
		{"explicit_half_open", Query{TimeStart: 0, TimeEnd: .1, TimeStartSet: true, TimeEndSet: true}, 1, true, false},
		{"default_extent_includes_capture_tail", Query{}, 2, true, true},
		{"point_has_inventory_no_rate", Query{TimeStart: .1, TimeEnd: .1, TimeStartSet: true, TimeEndSet: true}, 1, false, false},
		{"line_only", Query{LineStart: 2, LineEnd: 2}, 1, false, false},
		{"line_overrides_conflicting_time", Query{LineStart: 2, LineEnd: 2, TimeStart: 20, TimeEnd: 30, TimeStartSet: true, TimeEndSet: true}, 1, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := ioActivityPublicRun(t, idx, tc.q)
			g := s.Groups[0]
			ioActivityPublicValues(t, g.Values, tc.count, uint64(tc.count)*4096)
			if (g.Rates != nil) != tc.rate || (s.Window != nil) != tc.rate {
				t.Fatalf("unknown denominator became zero or disappeared: %+v", s)
			}
			if !tc.rate && (len(g.Buckets) != 0 || s.WindowUnavailableReason == "") {
				t.Fatalf("point/line forged timeline: %+v", s)
			}
			if tc.rate {
				if s.Window.EndInclusive != tc.tail || len(g.Buckets) != 1 || g.Buckets[0].Window.EndInclusive != tc.tail || g.Buckets[0].Values.EventCount != tc.count {
					t.Fatalf("explicit versus captured tail conflated: %+v", s)
				}
				wire, _ := json.Marshal(s.Window)
				if !strings.Contains(string(wire), `"start_ts":0`) {
					t.Fatal("real zero origin omitted")
				}
			}
		})
	}
	for _, ts := range []string{"0", "1.000000001"} {
		one := buildTraceIndex(t, "one.systrace", ioActivityRQLine(ts, "R"))
		s := ioActivityPublicRun(t, one, Query{})
		if s.Groups[0].Values.EventCount != 1 || s.Groups[0].Rates != nil || s.Window != nil {
			t.Fatalf("single observed endpoint disappeared or invented duration: %+v", s)
		}
	}
}

func TestIOActivityPublicDerivedSpanDoesNotInheritCaptureTail(t *testing.T) {
	for _, captureTail := range []bool{false, true} {
		body := "app-40 (40) [001] .... 1.0: tracing_mark_write: B|40|LoadContent\n" +
			ioActivityRQLine("1.5", "R") +
			"app-40 (40) [001] .... 2.0: tracing_mark_write: E|40\n" + ioActivityRQLine("2.0", "W")
		if !captureTail {
			body += ioActivityRQLine("3.0", "W")
		}
		idx := buildTraceIndex(t, "span-end.systrace", body)
		s := ioActivityPublicRun(t, idx, Query{SpanName: "LoadContent", PID: 40})
		if s.Window == nil || s.Window.StartTs != 1 || s.Window.EndTs != 2 || s.Window.EndInclusive || s.Groups[0].Values.EventCount != 1 {
			t.Fatalf("derived business interval borrowed capture-tail inclusion: %+v", s)
		}
	}
}

func TestIOActivityPublicBucketClampsDecimalBoundariesAndHugeWindows(t *testing.T) {
	idx := buildTraceIndex(t, "buckets.systrace", ioActivityRQLine("0.1", "R")+ioActivityRQLine("0.15", "W")+ioActivityRQLine("0.2", "R"))
	for _, tc := range []struct {
		name          string
		ms, want, end float64
		buckets       uint64
		reason        string
	}{
		{"default", 0, 100, .3, 3, ""},
		{"decimal_fiftieth", 50, 50, .3, 6, ""},
		{"tiny", 1e-100, 1, .3, 300, ""},
		{"large", 1e100, 60000, .3, 1, ""},
		{"nan", math.NaN(), 100, .3, 3, ""},
		{"huge_window", 1, 1, 1e30, 0, "bucket_count_overflow"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := ioActivityPublicRun(t, idx, Query{TimeStart: 0, TimeEnd: tc.end, TimeStartSet: true, TimeEndSet: true, BucketMs: tc.ms})
			g := s.Groups[0]
			if s.BucketMs != tc.want || g.BucketCount != tc.buckets || g.BucketsUnavailableReason != tc.reason || g.Values.EventCount != 3 || g.Rates == nil {
				t.Fatalf("bucket normalization changed complete population: %+v", s)
			}
			if tc.name == "decimal_fiftieth" && (g.Buckets[1].Values.EventCount != 0 || g.Buckets[2].Values.EventCount != 1 || g.Buckets[3].Values.EventCount != 1 || g.Buckets[4].Values.EventCount != 1) {
				t.Fatalf("exact decimal endpoints shifted bucket: %+v", g.Buckets)
			}
		})
	}
	// Real parser timestamps may eventually be wider than an effective bucket.
	// Keep the numeric summary but disclose an unrepresentable timeline.
	wide := buildTraceIndex(t, "precision.systrace", ioActivityRQLine("1000000000000000", "R"))
	s := ioActivityPublicRun(t, wide, Query{TimeStart: 1e15, TimeEnd: 1e15 + 1, TimeStartSet: true, TimeEndSet: true, BucketMs: 1})
	if s.Groups[0].BucketsUnavailableReason != "bucket_width_below_trace_timestamp_resolution" || s.Groups[0].Values.EventCount != 1 || s.Groups[0].Rates == nil {
		t.Fatalf("sub-ULP bucket created bogus duration: %+v", s)
	}
}

func TestIOActivityPublicDirectionsAndUnknownEndpointAdmission(t *testing.T) {
	body := ioActivityRQLine("1.01", "RCVHS") + ioActivityRQLine("1.02", "WCVHS") + ioActivityRQLine("1.03", "RW") + ioActivityRQLine("1.04", "READ") +
		"io-40 (40) [001] .... 1.05: block_rq_issue: 0,0 R 4096 () 1 + 8 [io]\n" +
		"io-40 (40) [001] .... 1.06: block_rq_issue: 8,0 R missing () 1 + 8 [io]\n" +
		"io-40 (40) [001] .... 1.07: mmc_request_start: mmc0 tag=-1 opcode=17 blocks=8 block_size=512 blk_addr=10 extra=1\n" +
		"io-40 (40) [001] .... 1.08: scsi_dispatch_cmd_start: dev=8,0 op=read bytes=4096\n"
	idx := buildTraceIndex(t, "direction.systrace", body)
	s := ioActivityPublicRun(t, idx, Query{TimeStart: 1, TimeEnd: 1.1})
	if s.Coverage.SupportedEndpointCount != 4 || s.Coverage.RejectedEndpointCount != 3 || s.GroupCount != 1 {
		t.Fatalf("unsupported identity/payload counted as real supported IO: %+v", s)
	}
	g := s.Groups[0]
	if g.Directions[0].Values.EventCount != 1 || g.Directions[1].Values.EventCount != 1 || g.Directions[2].Values.EventCount != 2 || g.ReadWrite.EventDenominator != 2 {
		t.Fatalf("noisy op substring classified R/W: %+v", g)
	}
}

func TestIOActivityPublicGroupCapAndLegacyByteIdentity(t *testing.T) {
	var body strings.Builder
	for i := 0; i < IOActivityGroupLimit+3; i++ {
		fmt.Fprintf(&body, "io-40 (40) [001] .... 1.01: block_rq_issue: 8,%d R 4096 () 1 + 8 [io]\n", i)
		fmt.Fprintf(&body, "irq-2 (2) [001] .... 1.02: block_rq_complete: 8,%d R () 1 + 8 [0]\n", i)
	}
	idx := buildTraceIndex(t, "groups.systrace", body.String())
	q := Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.1}
	s := ioActivityPublicRun(t, idx, q)
	if s.GroupCount != (IOActivityGroupLimit+3)*2 || len(s.Groups) != IOActivityGroupLimit || s.OmittedGroups != s.GroupCount-IOActivityGroupLimit {
		t.Fatalf("group display cap became population cap: %+v", s)
	}
	plain := Run(idx, q)
	plain.WindowStats.IOActivity = nil
	old := *idx
	old.Events = append([]Event(nil), idx.Events...)
	for i := range old.Events {
		if bf := old.Events[i].BlockIOFields; bf != nil {
			clone := *bf
			clone.ioActivity = nil
			old.Events[i].BlockIOFields = &clone
		}
	}
	before, _ := json.Marshal(Run(&old, q))
	after, _ := json.Marshal(plain)
	if string(before) != string(after) {
		t.Fatal("independent endpoint collection mutated legacy numeric/pairing/causal faces")
	}
	dead, stop := context.WithCancel(context.Background())
	stop()
	res := Run(idx, q.WithRunContext(dead))
	if res.WindowStats != nil || res.ViewCancellation == nil {
		t.Fatalf("canceled collector published partial population: %+v", res)
	}
}
