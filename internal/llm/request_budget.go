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

type interruptibleRequestBudgetWait struct{}

// ChatWithInterruptibleRequestBudget additionally preserves a caller's need
// to return on a non-streaming request timeout even if an adapter ignores its
// context. This is an explicit execution policy, not a model option. Wrappers
// carry it to the actual leaf through the original context; stream liveness
// ownership and ordinary synchronous request-budget callers are unchanged.
func ChatWithInterruptibleRequestBudget(ctx context.Context, adapter Adapter, messages []Message, tools []ToolSchema, opts ChatOptions, timeout time.Duration) (Response, error) {
	ctx = context.WithValue(ctx, interruptibleRequestBudgetWait{}, true)
	return ChatWithRequestBudget(ctx, adapter, messages, tools, opts, timeout)
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
	if interruptible, _ := ctx.Value(interruptibleRequestBudgetWait{}).(bool); !interruptible {
		return adapter.Chat(requestCtx, messages, tools, opts)
	}
	// Only the actual non-streaming leaf gets this bounded waiter. A wrapper
	// must never place it around possible streaming fallbacks. The buffered
	// result lets a late, cancellation-ignoring adapter return without blocking
	// the sender; adapters remain responsible for stopping their own I/O.
	type result struct {
		response Response
		err      error
	}
	done := make(chan result, 1)
	go func() {
		response, err := adapter.Chat(requestCtx, messages, tools, opts)
		done <- result{response: response, err: err}
	}()
	select {
	case response := <-done:
		return response.response, response.err
	case <-requestCtx.Done():
		return Response{}, requestCtx.Err()
	}
}
