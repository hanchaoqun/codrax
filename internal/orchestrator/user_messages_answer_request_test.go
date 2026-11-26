package orchestrator

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The reader-facing wording of an answer request that was too large to send.
// Spelled out here on purpose: the tests pin the wire text, and the product
// tables may not drift from it silently.
const (
	refusalNoticeZH     = "无法继续：这一问的证据量超过了一次答案请求能携带的量，这次请求没有发给模型，也没有写出答案。可以缩小问题范围后重试；分析 Trace 时，可缩小时间窗，或减少要问的线程"
	refusalNoticeEN     = "Cannot continue: the evidence for this question is larger than one answer request can carry, so the request was not sent to the model and no answer was written. Narrow the question and retry; for a trace, narrow the time window or ask about fewer threads"
	refusalStepNoticeZH = "这一问的证据量超过了一次答案请求能携带的量，这次请求没有发给模型"
	refusalStepNoticeEN = "The evidence for this question is larger than one answer request can carry, so the request was not sent to the model"
)

// readerForbiddenWords are internal names the reader must never be shown for
// this case: stage, limit and mechanism vocabulary.
var readerForbiddenWords = []string{
	"allocation", "budget", "finalizer", "finalize", "request plan", "content-byte", "byte", "guard", "agent ",
	"分配", "预算", "成文", "字节", "阶段",
}

func TestAnswerRequestNotSentNoticeIsInTheReadersTerms(t *testing.T) {
	for _, tc := range []struct {
		lang, step, notice string
		actions            []string
	}{
		{"zh", refusalStepNoticeZH, refusalNoticeZH, []string{"缩小问题范围", "缩小时间窗", "减少要问的线程"}},
		{"en", refusalStepNoticeEN, refusalNoticeEN, []string{"Narrow the question", "narrow the time window", "ask about fewer threads"}},
	} {
		if got := answerRequestNotSentMessage(tc.lang); got != tc.step {
			t.Errorf("%s step notice\n got: %s\nwant: %s", tc.lang, got, tc.step)
		}
		got := answerRequestNotSentFailureMessage(tc.lang)
		if got != tc.notice {
			t.Errorf("%s notice\n got: %s\nwant: %s", tc.lang, got, tc.notice)
		}
		for _, action := range tc.actions {
			if !strings.Contains(got, action) {
				t.Errorf("%s notice must tell the reader they can %q: %s", tc.lang, action, got)
			}
		}
		for _, text := range []string{tc.step, tc.notice} {
			for _, word := range readerForbiddenWords {
				if strings.Contains(strings.ToLower(text), word) {
					t.Errorf("reader notice %q contains the internal name %q", text, word)
				}
			}
			if strings.ContainsAny(text, "0123456789") {
				t.Errorf("reader notice %q carries a figure; sizes stay in the log", text)
			}
		}
	}
	// An unset language reads as Chinese, like every other notice.
	if got := answerRequestNotSentFailureMessage(""); got != answerRequestNotSentFailureMessage("zh") && got != answerRequestNotSentFailureMessage("en") {
		t.Errorf("an unset language must fall into one of the two tables: %s", got)
	}
}

// The typed refusal is recognised before any string probe of the error text:
// a byte figure that happens to contain "429" is not a rate limit, and one
// that contains a transport word is not a dropped connection.
func TestRefusedAnswerRequestMessageIsChosenBeforeStringProbes(t *testing.T) {
	wrapped := fmt.Errorf("agent finalizer execution: %w", refusedAnswerRequest())
	if !strings.Contains(wrapped.Error(), "429") {
		t.Fatal("fixture must carry the digits 429 in its byte figure")
	}
	for lang, want := range map[string]string{"zh": refusalNoticeZH, "en": refusalNoticeEN} {
		if got := forcedFinalizeFailureMessage(wrapped, lang); got != want {
			t.Errorf("%s message\n got: %s\nwant: %s", lang, got, want)
		}
	}
	// Every string arm of the chooser, wrapped around the typed refusal: the
	// typed arm still wins.
	for _, probe := range []string{"unexpected EOF", "no deployments available", "rate limit", "context canceled", "deadline exceeded", "timeout"} {
		err := fmt.Errorf("%s: %w", probe, refusedAnswerRequest())
		if got := forcedFinalizeFailureMessage(err, "en"); got != refusalNoticeEN {
			t.Errorf("probe %q outranked the typed refusal: %s", probe, got)
		}
	}
	// The string arms keep their meaning for errors that are not a refusal.
	if got := forcedFinalizeFailureMessage(errors.New("upstream returned 429"), "en"); !strings.Contains(got, "rate-limited") {
		t.Errorf("a real rate limit must still read as one: %s", got)
	}
	if got := forcedFinalizeFailureMessage(errors.New("read stream: unexpected EOF"), "en"); got == refusalNoticeEN {
		t.Errorf("a dropped connection is not a refused request: %s", got)
	}
}

// Through the real scheduler: whichever way the run ends on a refused answer
// request, the reader is told what happened and what they can do.
func TestRunEndingOnARefusedAnswerRequestTellsTheReaderWhatToDo(t *testing.T) {
	for _, tc := range []struct {
		name, lang, want string
		opts             refusalRunOptions
		dispatches       int
	}{
		{"zh, forced dispatch not run", "zh", refusalNoticeZH, refusalRunOptions{}, 1},
		{"en, forced dispatch not run", "en", refusalNoticeEN, refusalRunOptions{}, 1},
		{"zh, rebuilt and refused again", "zh", refusalNoticeZH, refusalRunOptions{
			repoFiles: lateReadRepo,
			finalizer: func(call int, ctx *types.AgentContext) (*agent.StageOutput, error) {
				if call == 1 {
					pendingLateRead(ctx)
				}
				return nil, refusedAnswerRequest()
			},
		}, 2},
		{"en, refused after a repair request", "en", refusalNoticeEN, refusalRunOptions{
			finalizer: func(call int, _ *types.AgentContext) (*agent.StageOutput, error) {
				if call == 1 {
					return nil, &agent.AnswerContextByteBudgetError{ActualBytes: 777916, AllocatedBytes: 700000, RequestsSent: 1}
				}
				return nil, refusedAnswerRequest()
			},
		}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.opts.language = tc.lang
			run := runRefusalScenario(t, tc.opts)
			if run.finalizerCalls != tc.dispatches {
				t.Fatalf("fixture: answer stage dispatched %d times, want %d", run.finalizerCalls, tc.dispatches)
			}
			if got := run.bus.TaskState.LastError; got != tc.want {
				t.Errorf("reader notice\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}
