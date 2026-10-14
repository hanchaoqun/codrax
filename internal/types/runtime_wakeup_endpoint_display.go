package types

import "strconv"

// RuntimeWakeupProcessDisplay is a display-only projection of the independently
// published endpoint identities. Callers must use a producer edge or a validated
// RuntimeWakeupEvent; labels, neighboring records and the other endpoint cannot
// supply a missing process. This must not replace RuntimeWakeupThreadLabel in
// claim keys, subject/object matching or relation credentials.
func RuntimeWakeupProcessDisplay(waker, wakee RuntimeWakeupThread) string {
	process := func(thread RuntimeWakeupThread) string {
		if thread.PID <= 0 || thread.PID > RuntimeTargetMaxPID || thread.TGID <= 0 || thread.TGID > RuntimeTargetMaxPID {
			return "未记录"
		}
		return strconv.Itoa(thread.TGID)
	}
	return "发起线程所属进程 TGID=" + process(waker) + "；被唤醒线程所属进程 TGID=" + process(wakee)
}
