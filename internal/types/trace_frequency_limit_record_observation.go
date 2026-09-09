package types

import (
	"fmt"
	"strings"
)

// FormatTraceFrequencyLimitRecordObservation describes two different read
// sets already carried by one witness: the current-query record census and
// one selected record's bounds. It does not reconstruct rows, infer duration,
// or grant positive policy authority to a missing/nonpositive upper bound.
// Callers retain their existing CPU/query/source coordinate display separately.
func FormatTraceFrequencyLimitRecordObservation(w TraceFrequencyLimitAuthority, lang string) string {
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
	var observation string
	if zh {
		observation = fmt.Sprintf("此 CPU 当前查询范围内有效策略记录共 %d 条；正上限最低的一条代表记录的策略范围为 %d–%d kHz。", w.LimitRowCount, w.MinFrequencyKHz, w.MaxFrequencyKHz)
		if w.MaxFrequencyKHz <= 0 {
			observation = fmt.Sprintf("此 CPU 当前查询范围内有效策略记录共 %d 条；所给记录的范围为 %d–%d kHz，未提供正的策略上限，不能据此断言没有策略。", w.LimitRowCount, w.MinFrequencyKHz, w.MaxFrequencyKHz)
		}
	} else {
		observation = fmt.Sprintf("%d valid frequency-policy records for this CPU in the current query scope; one representative record with the lowest positive upper bound has policy bounds %d–%d kHz. ", w.LimitRowCount, w.MinFrequencyKHz, w.MaxFrequencyKHz)
		if w.MaxFrequencyKHz <= 0 {
			observation = fmt.Sprintf("%d valid frequency-policy records for this CPU in the current query scope; the supplied bounds are %d–%d kHz. No positive policy ceiling is supplied; this does not prove policy absence. ", w.LimitRowCount, w.MinFrequencyKHz, w.MaxFrequencyKHz)
		}
	}
	return observation + TraceFrequencyLimitRecordCaliber(lang)
}

// TraceFrequencyLimitRecordCaliber is display-only teaching, shared by the
// numeric guidance, reader cards and system-owned policy annotation. Query
// scope includes any line/range filters; it is not a claim of full capture.
func TraceFrequencyLimitRecordCaliber(lang string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh") {
		return "总条数不是这组上下限重复出现的次数；代表记录的行号/时间只定位这一次事件，不证明整窗保持该策略或策略持续时长。"
	}
	return "The total is not a repetition count for this min/max pair. The representative line/time locates one event, not whole-window constancy or policy duration."
}
