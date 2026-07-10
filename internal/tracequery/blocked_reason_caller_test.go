package tracequery

import (
	"strings"
	"testing"
)

func TestBlockedReasonCallerSeparatesSemanticSymbolFromRawAddress(t *testing.T) {
	tests := []struct {
		name   string
		fields string
		want   string
	}{
		{name: "legacy raw address", fields: "pid=562 iowait=1 caller=0x69680100fffe0000", want: "unknown"},
		{name: "uppercase raw address", fields: "pid=562 iowait=1 caller=0XABCDEF", want: "unknown"},
		{name: "explicit opaque overrides caller", fields: "pid=562 iowait=1 caller=forged_name caller_raw=0x1234 caller_quality=opaque", want: "unknown"},
		{name: "converter opaque", fields: "pid=562 iowait=1 caller=unknown caller_raw=0x1234 caller_quality=opaque", want: "unknown"},
		{name: "symbol with offsets", fields: "pid=562 iowait=1 caller=worker_thread+0x534/0x820", want: "worker_thread+0x534/0x820"},
		{name: "symbolized quality", fields: "pid=562 iowait=1 caller=schedule_timeout+0x10/0x20[kernel] caller_raw=0x41424344 caller_quality=symbolized", want: "schedule_timeout+0x10/0x20[kernel]"},
		{name: "missing caller", fields: "pid=562 iowait=1", want: "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line := "worker-20 (20) [004] .... 1.000000: sched_blocked_reason: " + tc.fields
			ev, ok := ParseLine(1, line, newStringInterner())
			if !ok || ev.Type != EventSchedBlockedReason {
				t.Fatalf("blocked reason rejected: ok=%t event=%+v", ok, ev)
			}
			if ev.WakeePID != 562 || ev.IOWait != 1 || ev.Reason != tc.want {
				t.Fatalf("parsed pid=%d iowait=%d reason=%q, want pid=562 iowait=1 reason=%q", ev.WakeePID, ev.IOWait, ev.Reason, tc.want)
			}
			if !strings.Contains(ev.FieldText, tc.fields) {
				t.Fatalf("raw caller provenance was not retained: %q", ev.FieldText)
			}
		})
	}
}

func TestBlockedReasonRawAddressesFoldIntoOneUnknownReasonBucket(t *testing.T) {
	idx := buildTraceIndex(t, "blocked-raw-address.systrace", strings.Join([]string{
		"worker-20 (20) [004] .... 1.000000: sched_blocked_reason: pid=562 iowait=1 caller=0x11111111",
		"worker-20 (20) [004] .... 1.001000: sched_blocked_reason: pid=562 iowait=1 caller=0x22222222",
		"worker-20 (20) [004] .... 1.002000: sched_blocked_reason: pid=562 iowait=1 caller=f2fs_wait_on_block+0x10/0x20",
		"",
	}, "\n"))
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.999, TimeEnd: 1.003})
	if len(stats.BlockedReasons) != 2 {
		t.Fatalf("raw addresses fragmented blocked-reason buckets: %+v", stats.BlockedReasons)
	}
	counts := map[string]int{}
	for _, reason := range stats.BlockedReasons {
		counts[reason.Reason] = reason.Count
	}
	if counts["unknown"] != 2 || counts["f2fs_wait_on_block+0x10/0x20"] != 1 {
		t.Fatalf("unexpected blocked-reason grouping: %+v", counts)
	}
}
