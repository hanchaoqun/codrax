package llm

import (
	"context"
	"time"
)

// RequestBudgetAdapter is implemented by adapter wrappers that can forward a
// per-call evaluator budget to the provider actually handling each attempt.
// It is separate from StreamingLivenessReporter: a mixed fallback stack need
// not advertise that every possible leg has streaming watchdogs.
type RequestBudgetAdapter interface {
	ChatWithRequestBudget(context.Context, []Message, []ToolSchema, ChatOptions, time.Duration) (Response, error)
}

// ChatWithRequestBudget applies the evaluator's optional wall budget only to
// the active non-streaming adapter. Wrappers forward the same typed budget to
// their actual child instead of preemptively timing the entire fallback chain.
// Each entered non-streaming leg receives its own budget; the adapter's own
// request timeout and any earlier caller deadline/cancellation remain intact.
// A streaming liveness owner receives the original context, so heartbeat,
// reasoning and tool-call bytes can outlive this budget without interruption.
func ChatWithRequestBudget(ctx context.Context, adapter Adapter, messages []Message, tools []ToolSchema, opts ChatOptions, timeout time.Duration) (Response, error) {
	if timeout <= 0 {
		return adapter.Chat(ctx, messages, tools, opts)
	}
	if wrapper, ok := adapter.(RequestBudgetAdapter); ok {
		return wrapper.ChatWithRequestBudget(ctx, messages, tools, opts, timeout)
	}
	if streaming, ok := adapter.(StreamingLivenessReporter); ok && streaming.StreamingLivenessWatchdogEnabled() {
		return adapter.Chat(ctx, messages, tools, opts)
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return adapter.Chat(requestCtx, messages, tools, opts)
}
