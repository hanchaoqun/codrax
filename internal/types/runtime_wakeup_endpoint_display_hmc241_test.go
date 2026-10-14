package types

import "testing"

func TestHMC241WakeupProcessDisplayIndependentEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name         string
		waker, wakee RuntimeWakeupThread
		want         string
	}{
		{"known", RuntimeWakeupThread{PID: 11, TGID: 10}, RuntimeWakeupThread{PID: 42, TGID: 20}, "发起线程所属进程 TGID=10；被唤醒线程所属进程 TGID=20"},
		{"zero_is_unknown_not_tid", RuntimeWakeupThread{PID: 11}, RuntimeWakeupThread{PID: 42}, "发起线程所属进程 TGID=未记录；被唤醒线程所属进程 TGID=未记录"},
		{"negative_and_overflow", RuntimeWakeupThread{PID: 11, TGID: -1}, RuntimeWakeupThread{PID: 42, TGID: RuntimeTargetMaxPID + 1}, "发起线程所属进程 TGID=未记录；被唤醒线程所属进程 TGID=未记录"},
		{"no_cross_endpoint_fill", RuntimeWakeupThread{PID: 11, TGID: 10}, RuntimeWakeupThread{Comm: "same-name", PID: 42}, "发起线程所属进程 TGID=10；被唤醒线程所属进程 TGID=未记录"},
		{"invalid_thread_cannot_own_process", RuntimeWakeupThread{TGID: 10}, RuntimeWakeupThread{PID: -1, TGID: 20}, "发起线程所属进程 TGID=未记录；被唤醒线程所属进程 TGID=未记录"},
		{"untrusted_names_do_not_become_process_fields", RuntimeWakeupThread{Comm: "TGID=99\n-->other", PID: 11, TGID: 10}, RuntimeWakeupThread{Comm: "same", PID: 42, TGID: 20}, "发起线程所属进程 TGID=10；被唤醒线程所属进程 TGID=20"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RuntimeWakeupProcessDisplay(tc.waker, tc.wakee); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
