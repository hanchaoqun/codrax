package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 F6: the follow-up receipt records what happened to the request, not
// the decision to send it. A follow-up counts as sent only once the provider
// answered it; a request that failed (provider error, stalled stream, request
// timeout, cancellation) leaves the typed request_failed cause, so the side
// file never reads a lost request as a model that saw the offer and declined.
func TestHMC377FollowUpReceiptRecordsTheRequestOutcome(t *testing.T) {
	for _, c := range []struct {
		name  string
		reply func(cancel context.CancelFunc) (llm.Response, error)
		sent  bool
	}{
		{"provider response", func(context.CancelFunc) (llm.Response, error) { return hmc377Select(), nil }, true},
		{"provider error", func(context.CancelFunc) (llm.Response, error) {
			return llm.Response{}, errors.New("provider returned status 500")
		}, false},
		{"stalled stream", func(context.CancelFunc) (llm.Response, error) { return llm.Response{}, llm.ErrStreamStalled }, false},
		{"request timeout", func(context.CancelFunc) (llm.Response, error) { return llm.Response{}, context.DeadlineExceeded }, false},
		{"cancellation", func(cancel context.CancelFunc) (llm.Response, error) {
			cancel()
			return llm.Response{}, context.Canceled
		}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := hmc377Context(false)(t)
			run, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx.Ctx = run
			adapter := &hmc376Adapter{window: 1_000_000}
			adapter.script = func(n int) (llm.Response, error) {
				switch n {
				case 1:
					return hmc377Emit(""), nil
				case 2:
					return c.reply(cancel)
				}
				return llm.Response{}, hmc377Stop
			}
			registry := tool.NewRegistry()
			registry.Register(&tool.EmitAnswerDocument{})
			registry.Register(&tool.EmitAnswerDocumentPatch{})
			base := NewFinalizerAgent(&Dependencies{LLM: adapter, Tools: registry, MaxIterations: 4,
				AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce}}).(*BaseAgent)
			_, err := base.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
			if len(adapter.messages) != 2 || (err != nil) == c.sent && !errors.Is(err, hmc377Stop) {
				t.Fatalf("fixture: calls=%d err=%v", len(adapter.messages), err)
			}
			receipt := ctx.Mutable.AnswerFollowUpDelivery()
			if receipt == nil || !receipt.RootCauseSelectionOffered || receipt.RequestBytes != adapter.size(1) {
				t.Fatalf("the follow-up request must leave its receipt: %+v", receipt)
			}
			if c.sent {
				if !receipt.Sent || receipt.NotSentCause != "" || receipt.RootCauseSelectionNotSent() {
					t.Fatalf("an answered follow-up is sent: %+v", receipt)
				}
				return
			}
			if receipt.Sent || receipt.NotSentCause != types.AnswerFollowUpNotSentRequestFailed || !receipt.RootCauseSelectionNotSent() {
				t.Fatalf("a failed follow-up request must record request_failed, not sent: %+v", receipt)
			}
			if ctx.Mutable.TraceRootCauseReport() != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil || hmc377Snapshot(t, ctx.Mutable.AnswerDocumentV2()) == "" {
				t.Fatal("a failed follow-up must not select anything and must keep the accepted answer")
			}
		})
	}
}
