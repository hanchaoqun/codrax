package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The run's final notice when the answer was not formed for another reason
// than a request that could not be sent at all: the product's general text.
const (
	answerNotFormedNoticeZH = "无法继续：最终答案生成失败"
	answerNotFormedNoticeEN = "Cannot continue: failed to generate the final answer"
)

// refusedRepairRequest is the byte guard's refusal of a request after the
// dispatch had already sent requests: a repair round whose history grew
// past what one request can carry. The first request of that dispatch did
// carry the evidence and reached the model. The byte figure contains the
// digits 429, so a string probe for a rate-limit status would misread it.
func refusedRepairRequest(sent int) error {
	return &agent.AnswerContextByteBudgetError{ActualBytes: 742912, AllocatedBytes: 700000, RequestsSent: sent}
}

// A refused request is told to the reader as "not sent, the evidence does not
// fit one request" only when the run's refusal record says nothing of the
// answer stage was sent — the derivation the root-cause side file's reason
// reads. A refused repair or follow-up request is not: the evidence had
// reached the model, the step reads as recoverable because the forced
// dispatch follows, and a run that still ends without an answer gets the
// general terminal text, chosen by the typed refusal before any string probe.
func TestARefusedRepairRequestIsNotToldAsUnsent(t *testing.T) {
	for _, tc := range []struct {
		lang, stepNotice, notice, failedLabel, retryLabel, cue, generic string
	}{
		{"zh", refusalStepNoticeZH, refusalNoticeZH, "未能生成最终答案", "模型响应出错,正在重新撰写答案", "4/4 正在生成最终答案", answerNotFormedNoticeZH},
		{"en", refusalStepNoticeEN, refusalNoticeEN, "Could not compose answer", "Model response error, re-composing the final answer", "4/4 Composing the final answer", answerNotFormedNoticeEN},
	} {
		t.Run(tc.lang+", repair round refused, forced dispatch answers", func(t *testing.T) {
			run := runRefusalScenario(t, refusalRunOptions{
				language: tc.lang,
				finalizer: func(call int, _ *types.AgentContext) (*agent.StageOutput, error) {
					if call == 1 {
						return nil, refusedRepairRequest(1)
					}
					return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "Source 在 src.go:1 定义。"}, nil
				},
			})
			if run.finalizerCalls != 2 || !strings.Contains(run.bus.Mutable.Result(), "Source") || run.bus.TaskState.LastError != "" {
				t.Fatalf("fixture: the forced dispatch must run and answer: calls=%d result=%q lastError=%q",
					run.finalizerCalls, run.bus.Mutable.Result(), run.bus.TaskState.LastError)
			}
			assertRepairRefusalStepLine(t, run, tc.stepNotice, tc.failedLabel, tc.retryLabel)
			if got := strings.Count(run.rendered, tc.cue); got != 2 {
				t.Errorf("both dispatches ran and both must be announced, got %d cue(s):\n%s", got, run.rendered)
			}
		})

		t.Run(tc.lang+", repair rounds refused on both dispatches", func(t *testing.T) {
			run := runRefusalScenario(t, refusalRunOptions{
				language: tc.lang,
				finalizer: func(call int, _ *types.AgentContext) (*agent.StageOutput, error) {
					return nil, refusedRepairRequest(call)
				},
			})
			if run.finalizerCalls != 2 || run.bus.Mutable.Result() != "" {
				t.Fatalf("fixture: two dispatches, no answer: calls=%d result=%q", run.finalizerCalls, run.bus.Mutable.Result())
			}
			assertRepairRefusalStepLine(t, run, tc.stepNotice, tc.failedLabel, tc.retryLabel)
			if got := run.bus.TaskState.LastError; got != tc.generic {
				t.Errorf("terminal notice of a run whose requests reached the model\n got: %s\nwant: %s", got, tc.generic)
			}
			if !strings.Contains(run.log, "forced finalize failed after 1 dispatched attempt(s)") {
				t.Errorf("the forced dispatch ran once:\n%s", lastLogLines(run.log, 6))
			}
		})

		// The forced dispatch's own first request is refused, but the
		// dispatch before it had sent its request: the record's kind is not
		// "first request", the side file keeps the transcript reason, and
		// the reader is not told that nothing was sent either.
		t.Run(tc.lang+", first request of the forced dispatch refused after a sent dispatch", func(t *testing.T) {
			run := runRefusalScenario(t, refusalRunOptions{
				language: tc.lang,
				finalizer: func(call int, _ *types.AgentContext) (*agent.StageOutput, error) {
					if call == 1 {
						return nil, refusedRepairRequest(1)
					}
					return nil, refusedAnswerRequest()
				},
			})
			if run.finalizerCalls != 2 || run.bus.Mutable.Result() != "" {
				t.Fatalf("fixture: two dispatches, no answer: calls=%d result=%q", run.finalizerCalls, run.bus.Mutable.Result())
			}
			if refusal := run.bus.Mutable.FinalizerRequestRefusal(); refusal == nil || refusal.Kind != types.FinalizerRefusalLaterRequest || refusal.RequestsSent != 0 {
				t.Fatalf("fixture: the standing refusal must be a later request with nothing sent by its own dispatch: %+v", refusal)
			}
			if got := run.bus.TaskState.LastError; got != tc.generic {
				t.Errorf("terminal notice after a dispatch that had sent its request\n got: %s\nwant: %s", got, tc.generic)
			}
		})
	}
}

// assertRepairRefusalStepLine: the answer step of a refused repair round keeps
// the recoverable shape and its own error text, never the unsent notice.
func assertRepairRefusalStepLine(t *testing.T, run *refusalRun, stepNotice, failedLabel, retryLabel string) {
	t.Helper()
	if strings.Contains(run.rendered, stepNotice) {
		t.Errorf("a refused repair request is told as a request that was never sent:\n%s", run.rendered)
	}
	if strings.Contains(run.rendered, failedLabel+" · ") {
		t.Errorf("the step is drawn as failed although the forced dispatch follows:\n%s", run.rendered)
	}
	// The recoverable glyph and label, then the dispatch's own error text
	// (the stage wraps it), exactly as for any other failed dispatch.
	raw := refusedRepairRequest(1).Error()
	recoverable := 0
	for _, line := range strings.Split(run.rendered, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "↻ "+retryLabel+" · ") {
			recoverable++
			if !strings.HasSuffix(line, raw) {
				t.Errorf("the recoverable step line must carry the dispatch's own error text %q: %s", raw, line)
			}
		}
	}
	if recoverable != 1 {
		t.Errorf("the answer step must read as recoverable once (↻ %s · …), found %d:\n%s", retryLabel, recoverable, run.rendered)
	}
}
