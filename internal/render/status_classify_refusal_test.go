package render

import (
	"testing"
	"time"
)

// An answer request that was too large to send has no model response behind
// it and cannot be retried into a different result. Its notice must read as a
// failed step on both surfaces, never as "model response error, re-composing".
func TestAnswerRequestNotSentNoticeReadsAsAFailedStep(t *testing.T) {
	for _, notice := range []string{
		"这一问的证据量超过了一次答案请求能携带的量，这次请求没有发给模型",
		"The evidence for this question is larger than one answer request can carry, so the request was not sent to the model",
		"无法继续：这一问的证据量超过了一次答案请求能携带的量，这次请求没有发给模型，也没有写出答案。可以缩小时间窗，或减少要问的线程后重试",
		"Cannot continue: the evidence for this question is larger than one answer request can carry, so the request was not sent to the model and no answer was written. Narrow the time window, or ask about fewer threads, and retry",
	} {
		if got := classifyEventError(notice); got != statusErrorFatal {
			t.Errorf("event classification = %q, want fatal: %s", got, notice)
		}
		glyph, state := nonTTYEndErrorShape(notice)
		if glyph != string(glyphFatal) || state != stagePhraseFailed {
			t.Errorf("non-TTY step line = %s/%v, want the failed shape: %s", glyph, state, notice)
		}
		row := &taskRow{stage: "finalize", errorMsg: notice, endTime: time.Now()}
		if got := classifyStatusError(row); got != statusErrorFatal {
			t.Errorf("row classification = %q, want fatal: %s", got, notice)
		}
		// No canned phrase replaces the notice: the reader sees what
		// happened and what to do, in the notice's own words.
		for _, lang := range []string{"zh", "en"} {
			if phrase := fatalDetailPhrase(row, lang); phrase != "" {
				t.Errorf("the notice must be shown as written, not as %q", phrase)
			}
		}
	}
	// The retry wording stays for a failure that did come from a response.
	if got := classifyEventError("agent finalizer execution: read stream: schema decode failed"); got != statusErrorRecoverable {
		t.Errorf("an unrelated step error keeps its classification, got %q", got)
	}
}
