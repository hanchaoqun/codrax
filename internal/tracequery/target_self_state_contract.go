package tracequery

// TargetSelfStateRankingBoundaryText is the single customer-facing wording
// source for the SYM-2 target-self split. The split itself remains owned by
// rootCauseItemIsTargetWaitSymptomType and the causal-token registry:
// wait-on-counterpart rows are drill-down symptoms, while decomposable self
// runnable/D/IO/compute-supply rows may compete under their typed caliber.
//
// Keep this text semantic rather than token-enumerative. It teaches readers
// how to interpret the typed rows; it neither admits a row nor changes rank.
func TargetSelfStateRankingBoundaryText(zh bool) string {
	if zh {
		return "目标线程自身的休眠、等锁或等对端属于待下钻的症状，不直接占根因排序席；唤醒后的可运行等待、已确认的 D/IO 阻塞及有正向提升量的运行供给仍可按其 typed 证据进入排序"
	}
	return "the target's own sleep, lock wait, and peer wait are drill-down symptoms and do not directly take root-cause seats; post-wakeup runnable delay, typed D/IO blocking, and a positive compute-supply opportunity may still enter the ranking under their typed evidence"
}
