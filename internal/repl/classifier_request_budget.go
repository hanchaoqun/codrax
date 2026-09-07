package repl

import (
	"context"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
)

// Configuration presence is meaningful: an operator-selected timeout remains
// a total classification deadline, while the built-in defaults bound only
// non-streaming requests. Setters preserve this distinction even when the
// explicit value happens to equal the default.
var turnPolicyClassifierTimeoutExplicit bool
var singleShotRoutePolicyTimeoutExplicit bool

type classifierCallBudget struct {
	requestTimeout time.Duration
	totalTimeout   time.Duration
}

func classifierBudget(timeout time.Duration, explicit bool) classifierCallBudget {
	if explicit {
		return classifierCallBudget{totalTimeout: timeout}
	}
	return classifierCallBudget{requestTimeout: timeout}
}

func (b classifierCallBudget) context(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if b.totalTimeout > 0 {
		return context.WithTimeout(ctx, b.totalTimeout)
	}
	return ctx, func() {}
}

func chatWithClassifierBudget(ctx context.Context, adapter llm.Adapter, messages []llm.Message, tools []llm.ToolSchema, opts llm.ChatOptions, budget classifierCallBudget) (llm.Response, error) {
	if budget.totalTimeout > 0 {
		return chatWithClassifierHardTimeout(ctx, adapter, messages, tools, opts, budget.totalTimeout)
	}
	return llm.ChatWithInterruptibleRequestBudget(ctx, adapter, messages, tools, opts, budget.requestTimeout)
}

// Only the production classifier promises to enforce the active-adapter
// budget. Unknown classifier implementations keep the dispatcher's bounded
// fallback, including implementations that do not observe context cancellation.
type classifierRequestBudgetOwner interface {
	managesClassifierRequestBudget()
}

func (*llmChitchatClassifier) managesClassifierRequestBudget() {}

func classifierDispatcherTimeout(classifier ChitchatClassifier) time.Duration {
	if !turnPolicyClassifierTimeoutExplicit {
		if _, ownsBudget := classifier.(classifierRequestBudgetOwner); ownsBudget {
			return 0
		}
	}
	return turnPolicyClassifierTimeout
}
