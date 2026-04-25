package llm

import (
	"fmt"
	"time"
)

// FallbackAdapter tries adapters in order until one succeeds.
type FallbackAdapter struct {
	adapters []Adapter
}

func NewFallbackAdapter(adapters ...Adapter) *FallbackAdapter {
	return &FallbackAdapter{adapters: adapters}
}

func (f *FallbackAdapter) Chat(messages []Message, tools []ToolSchema, opts ChatOptions) (Response, error) {
	var lastErr error
	for _, a := range f.adapters {
		resp, err := a.Chat(messages, tools, opts)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return Response{}, fmt.Errorf("all LLM adapters failed: %w", lastErr)
}

func (f *FallbackAdapter) ModelID() string {
	if len(f.adapters) > 0 {
		return f.adapters[0].ModelID()
	}
	return "none"
}

func (f *FallbackAdapter) MaxContextTokens() int {
	if len(f.adapters) > 0 {
		return f.adapters[0].MaxContextTokens()
	}
	return 0
}

// MaxOutputTokens / RequestTimeout / RetryMaxAttempts delegate to the
// first wrapped adapter. The fallback stack is for failover, not for
// heterogeneous sizing — operators are expected to wrap adapters with
// compatible knobs, so reporting the head's values is correct.
func (f *FallbackAdapter) MaxOutputTokens() int {
	if len(f.adapters) > 0 {
		return f.adapters[0].MaxOutputTokens()
	}
	return 0
}

func (f *FallbackAdapter) RequestTimeout() time.Duration {
	if len(f.adapters) > 0 {
		return f.adapters[0].RequestTimeout()
	}
	return 0
}

func (f *FallbackAdapter) RetryMaxAttempts() int {
	if len(f.adapters) > 0 {
		return f.adapters[0].RetryMaxAttempts()
	}
	return 0
}
