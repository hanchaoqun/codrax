package agent

import "strings"

// answerDocIOCompletionProofMeaning explains a producer-owned proof Boolean;
// it does not evaluate a wakeup, choose an endpoint, or mint a blocking value.
// In particular, false covers absent/ambiguous proof, not a negative event
// observation. Keep the original wire value alongside this reader guidance.
func answerDocIOCompletionProofMeaning(value, lang string) string {
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
	if zh {
		meaning := "该请求的完成唤醒证明未发布或未识别；不能据此判断是否发生唤醒或阻塞"
		switch strings.TrimSpace(value) {
		case "true":
			meaning = "已证明完成事件发出者定向唤醒发送线程"
		case "false":
			meaning = "未形成该请求独立的完成方唤醒发送线程证明；这不等于已证明未唤醒，证据可能缺失或存在歧义"
		}
		return meaning + "。阻塞时长须另有正的闭合区间及测量值；缺少该测量时不能记为零。"
	}
	meaning := "Completion-to-issuer wakeup proof is unpublished or unrecognized for this request; whether wakeup or blocking occurred is not determined"
	switch strings.TrimSpace(value) {
	case "true":
		meaning = "The completion emitter's directed wakeup of the issuing thread is proven"
	case "false":
		meaning = "No independent completion-to-issuer wakeup proof was established for this request; this does not prove that no wakeup occurred, because evidence may be absent or ambiguous"
	}
	return meaning + ". Blocking duration requires a separately published positive closed interval and measurement; a missing measurement is not zero."
}
