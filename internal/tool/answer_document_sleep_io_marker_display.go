package tool

// These labels describe the existing SleepIOWaitMS refinement. They do not
// classify an interval or combine the marker and completion-closure accounts.
func runtimeTraceSleepIOMarkerLabel(zh bool) string {
	if zh {
		return "内核 IO 等待标记确认的 S 态等待"
	}
	return "S-state wait confirmed by kernel IO-wait markers"
}

func runtimeTraceSleepIOMarkerBoundary(zh bool) string {
	if zh {
		return "该标记确认的时长已包含在 sleep 中；零值不排除由 IO 完成唤醒独立证明的线程阻塞；两种口径若覆盖同一段时间不得重复计入。"
	}
	return "The marker-confirmed duration is already included in sleep. A zero value does not rule out blocking independently proven by an IO-completion wakeup; if the two measures overlap, do not count the same time interval twice."
}
