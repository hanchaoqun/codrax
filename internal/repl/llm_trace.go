package repl

import (
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
)

// replLLMCallTrace is render-only telemetry for direct REPL LLM calls that do
// not go through BaseAgent. It must never drive routing, gating, or execution.
type replLLMCallTrace struct {
	Scope      string
	Reasoning  string
	ToolName   string
	ToolParams json.RawMessage
}

type replLLMTraceProvider interface {
	LastReplLLMTrace() replLLMCallTrace
}

func traceFromLLMResponse(scope string, resp llm.Response) replLLMCallTrace {
	trace := replLLMCallTrace{
		Scope:     strings.TrimSpace(scope),
		Reasoning: strings.TrimSpace(resp.ReasoningContent),
	}
	if len(resp.ToolCalls) > 0 {
		trace.ToolName = strings.TrimSpace(resp.ToolCalls[0].Name)
		trace.ToolParams = append(json.RawMessage(nil), resp.ToolCalls[0].Params...)
	}
	return trace
}
