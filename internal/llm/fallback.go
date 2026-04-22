package llm

import "fmt"

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
