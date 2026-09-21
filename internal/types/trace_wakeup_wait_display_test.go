package types

import "testing"

func TestTraceWakeupPreWaitDisplayPreservesPublishedOwnerAndValue(t *testing.T) {
	for _, tc := range []struct{ owner, value, want string }{
		{" transport-300 ", "14.000", "wakee=transport-300 pre_wakeup_wait=14.000ms"},
		{"接收线程-61", "0.001", "wakee=接收线程-61 pre_wakeup_wait=0.001ms"},
		{"", "0.000", "wakee=(identity not published) pre_wakeup_wait=0.000ms"},
	} {
		if got := TraceWakeupPreWaitDisplay(tc.owner, tc.value); got != tc.want {
			t.Fatalf("display reinterpreted owner/value: got %q want %q", got, tc.want)
		}
	}
}
