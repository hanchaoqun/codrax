package repl

import (
	"context"
	"strconv"
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
	if ctx == nil {
		ctx = context.Background()
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
	opts.OnContentDelta = chainStringCallback(opts.OnContentDelta, stream.onDeltaWithContext(ctx))
	opts.OnReasoningDelta = chainStringCallback(opts.OnReasoningDelta, stream.onDeltaWithContext(ctx))
	opts.OnToolCallDelta = chainToolCallCallback(opts.OnToolCallDelta, stream.onToolCallDeltaWithContext(ctx))
	opts.OnRetry = chainRetryCallback(opts.OnRetry, a.emit, a.agent, a.stage)
	opts.OnFallback = chainFallbackCallback(opts.OnFallback, a.emit, a.agent, a.stage)
	resp, err := a.inner.Chat(ctx, messages, tools, opts)
	if ctx.Err() != nil {
		return resp, err
	}
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
	emit          render.EventEmitter
	agent         types.AgentName
	stage         types.PipelineStage
	buf           strings.Builder
	lastEmitAt    time.Time
	toolArgBytes  map[int]int
	toolLastNames map[int]string
}

func newDirectLLMStreamPreview(emit render.EventEmitter, agent types.AgentName, stage types.PipelineStage) *directLLMStreamPreview {
	return &directLLMStreamPreview{emit: emit, agent: agent, stage: stage}
}

func (b *directLLMStreamPreview) onDelta(delta string) {
	if b == nil || b.emit == nil || delta == "" {
		return
	}
	b.buf.WriteString(delta)
	b.emitLivePreview(b.buf.String())
}

func (b *directLLMStreamPreview) onDeltaWithContext(ctx context.Context) func(string) {
	return func(delta string) {
		if ctx != nil && ctx.Err() != nil {
			return
		}
		b.onDelta(delta)
	}
}

func (b *directLLMStreamPreview) onToolCallDelta(index int, name string, argsChunk string) {
	if b == nil || b.emit == nil || argsChunk == "" {
		return
	}
	if b.toolArgBytes == nil {
		b.toolArgBytes = make(map[int]int)
	}
	if b.toolLastNames == nil {
		b.toolLastNames = make(map[int]string)
	}
	b.toolArgBytes[index] += len(argsChunk)
	if strings.TrimSpace(name) != "" {
		b.toolLastNames[index] = strings.TrimSpace(name)
	}
	label := b.toolLastNames[index]
	if label == "" {
		label = "tool_call"
	}
	b.emitLivePreview(formatDirectToolCallStreamPreview(label, b.toolArgBytes[index]))
}

func (b *directLLMStreamPreview) onToolCallDeltaWithContext(ctx context.Context) func(int, string, string) {
	return func(index int, name string, argsChunk string) {
		if ctx != nil && ctx.Err() != nil {
			return
		}
		b.onToolCallDelta(index, name, argsChunk)
	}
}

func (b *directLLMStreamPreview) emitLivePreview(preview string) {
	if b == nil || b.emit == nil || strings.TrimSpace(preview) == "" {
		return
	}
	now := time.Now()
	if !b.lastEmitAt.IsZero() && now.Sub(b.lastEmitAt) < 250*time.Millisecond {
		return
	}
	b.lastEmitAt = now
	b.emit(render.Event{
		Kind:      render.EventAgentContent,
		Timestamp: b.lastEmitAt,
		Agent:     b.agent,
		Stage:     b.stage,
		Reasoning: preview,
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

func chainToolCallCallback(first, second func(int, string, string)) func(int, string, string) {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	return func(index int, name string, argsChunk string) {
		func() {
			defer func() { _ = recover() }()
			first(index, name, argsChunk)
		}()
		func() {
			defer func() { _ = recover() }()
			second(index, name, argsChunk)
		}()
	}
}

func formatDirectToolCallStreamPreview(name string, bytes int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "tool_call"
	}
	if bytes < 1024 {
		return "tool_call " + name + " args " + strconv.Itoa(bytes) + " B"
	}
	kib := float64(bytes) / 1024.0
	if kib < 100 {
		return "tool_call " + name + " args " + strconv.FormatFloat(kib, 'f', 1, 64) + " KiB"
	}
	return "tool_call " + name + " args " + strconv.Itoa(int(kib+0.5)) + " KiB"
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
