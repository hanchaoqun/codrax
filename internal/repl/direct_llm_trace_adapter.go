package repl

import (
	"context"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// NewDirectLLMTraceAdapter wraps direct REPL-side LLM callers so data,
// operation, chitchat, and other non-BaseAgent planners get the same
// transparent request/stream UX as the code-analysis agents. The wrapper is
// render-only: it must never affect routing, gates, evidence, or execution.
func NewDirectLLMTraceAdapter(adapter llm.Adapter, renderer *render.Renderer, agent types.AgentName, stage types.PipelineStage) llm.Adapter {
	if adapter == nil || renderer == nil {
		return adapter
	}
	return &directLLMTraceAdapter{
		inner: adapter,
		emit:  renderer.Emitter(),
		agent: agent,
		stage: stage,
	}
}

type directLLMTraceAdapter struct {
	inner llm.Adapter
	emit  render.EventEmitter
	agent types.AgentName
	stage types.PipelineStage
}

func (a *directLLMTraceAdapter) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	if a == nil || a.inner == nil {
		return llm.Response{}, nil
	}
	telemetry := llm.BuildRequestTelemetry(a.inner, messages, tools)
	now := time.Now()
	if a.emit != nil {
		a.emit(render.Event{
			Kind:                  render.EventAgentThinking,
			Timestamp:             now,
			Agent:                 a.agent,
			Stage:                 a.stage,
			ContextTokensEstimate: telemetry.ContextTokensEstimate,
			ContextWindowTokens:   telemetry.ContextWindowTokens,
			ModelID:               telemetry.ModelID,
		})
	}
	stream := newDirectLLMStreamPreview(a.emit, a.agent, a.stage)
	opts.OnContentDelta = chainStringCallback(opts.OnContentDelta, stream.onDelta)
	opts.OnReasoningDelta = chainStringCallback(opts.OnReasoningDelta, stream.onDelta)
	opts.OnRetry = chainRetryCallback(opts.OnRetry, a.emit, a.agent, a.stage)
	opts.OnFallback = chainFallbackCallback(opts.OnFallback, a.emit, a.agent, a.stage)
	resp, err := a.inner.Chat(ctx, messages, tools, opts)
	stream.flush()
	if a.emit != nil {
		a.emit(render.Event{
			Kind:      render.EventAgentResponse,
			Timestamp: time.Now(),
			Agent:     a.agent,
			Stage:     a.stage,
		})
		if reasoning := directLLMVisibleReasoning(resp); reasoning != "" {
			a.emit(render.Event{
				Kind:      render.EventAgentReasoning,
				Timestamp: time.Now(),
				Agent:     a.agent,
				Stage:     a.stage,
				Reasoning: reasoning,
			})
		}
	}
	return resp, err
}

func (a *directLLMTraceAdapter) ModelID() string {
	if a == nil || a.inner == nil {
		return ""
	}
	return a.inner.ModelID()
}

func (a *directLLMTraceAdapter) MaxContextTokens() int {
	if a == nil || a.inner == nil {
		return 0
	}
	return a.inner.MaxContextTokens()
}

func (a *directLLMTraceAdapter) MaxOutputTokens() int {
	if a == nil || a.inner == nil {
		return 0
	}
	return a.inner.MaxOutputTokens()
}

func (a *directLLMTraceAdapter) RequestTimeout() time.Duration {
	if a == nil || a.inner == nil {
		return 0
	}
	return a.inner.RequestTimeout()
}

func (a *directLLMTraceAdapter) RetryMaxAttempts() int {
	if a == nil || a.inner == nil {
		return 0
	}
	return a.inner.RetryMaxAttempts()
}

type directLLMStreamPreview struct {
	emit       render.EventEmitter
	agent      types.AgentName
	stage      types.PipelineStage
	buf        strings.Builder
	lastEmitAt time.Time
}

func newDirectLLMStreamPreview(emit render.EventEmitter, agent types.AgentName, stage types.PipelineStage) *directLLMStreamPreview {
	return &directLLMStreamPreview{emit: emit, agent: agent, stage: stage}
}

func (b *directLLMStreamPreview) onDelta(delta string) {
	if b == nil || b.emit == nil || delta == "" {
		return
	}
	b.buf.WriteString(delta)
	if time.Since(b.lastEmitAt) < 250*time.Millisecond {
		return
	}
	b.lastEmitAt = time.Now()
	b.emit(render.Event{
		Kind:      render.EventAgentContent,
		Timestamp: b.lastEmitAt,
		Agent:     b.agent,
		Stage:     b.stage,
		Reasoning: b.buf.String(),
	})
}

func (b *directLLMStreamPreview) flush() {
	if b == nil || b.emit == nil || b.buf.Len() == 0 {
		return
	}
	b.emit(render.Event{
		Kind:      render.EventAgentContent,
		Timestamp: time.Now(),
		Agent:     b.agent,
		Stage:     b.stage,
		Reasoning: b.buf.String(),
	})
}

func chainStringCallback(first, second func(string)) func(string) {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	return func(delta string) {
		func() {
			defer func() { _ = recover() }()
			first(delta)
		}()
		func() {
			defer func() { _ = recover() }()
			second(delta)
		}()
	}
}

func directLLMVisibleReasoning(resp llm.Response) string {
	var parts []string
	if reasoning := strings.TrimSpace(resp.ReasoningContent); reasoning != "" {
		parts = append(parts, reasoning)
	}
	thoughts, ordinaryContent := splitVisibleThinkBlocks(resp.Content)
	for _, thought := range thoughts {
		if thought = strings.TrimSpace(thought); thought != "" {
			parts = append(parts, "<think>\n"+thought+"\n</think>")
		}
	}
	if len(resp.ToolCalls) > 0 {
		if ordinaryContent = strings.TrimSpace(ordinaryContent); ordinaryContent != "" {
			parts = append(parts, ordinaryContent)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func chainRetryCallback(first func(int, time.Duration, string), emit render.EventEmitter, agent types.AgentName, stage types.PipelineStage) func(int, time.Duration, string) {
	second := func(attempt int, delay time.Duration, reason string) {
		if emit == nil {
			return
		}
		emit(render.Event{
			Kind:         render.EventAdapterRetry,
			Timestamp:    time.Now(),
			Stage:        stage,
			Agent:        agent,
			RetryAttempt: attempt,
			RetryDelay:   delay,
			RetryReason:  reason,
		})
	}
	if first == nil {
		return second
	}
	return func(attempt int, delay time.Duration, reason string) {
		func() {
			defer func() { _ = recover() }()
			first(attempt, delay, reason)
		}()
		func() {
			defer func() { _ = recover() }()
			second(attempt, delay, reason)
		}()
	}
}

func chainFallbackCallback(first func(string, string, string), emit render.EventEmitter, agent types.AgentName, stage types.PipelineStage) func(string, string, string) {
	second := func(from, to, reason string) {
		if emit == nil {
			return
		}
		emit(render.Event{
			Kind:         render.EventAdapterFallback,
			Timestamp:    time.Now(),
			Stage:        stage,
			Agent:        agent,
			FallbackFrom: from,
			FallbackTo:   to,
			RetryReason:  reason,
		})
	}
	if first == nil {
		return second
	}
	return func(from, to, reason string) {
		func() {
			defer func() { _ = recover() }()
			first(from, to, reason)
		}()
		func() {
			defer func() { _ = recover() }()
			second(from, to, reason)
		}()
	}
}
