package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

type hmc278PressureAdapter struct {
	llm.Adapter
	window, output int
}

func TestHMC278ContextPressureTeachingUsesReservedRatio(t *testing.T) {
	p := contextRequestPressure{MessageBytes: 6000, ToolBytes: 1000, WindowBytes: 10000, OutputTokens: 500, OutputKnown: true}
	got := contextRequestPressureDirective(types.AgentExplorer, p, .9)
	if !strings.Contains(got, "90.0%") || !strings.Contains(got, "500 tokens") || strings.Contains(got, "70.0%") {
		t.Fatalf("reserve and threshold teaching disagree: %s", got)
	}
	p.OutputKnown, p.OutputTokens = false, 0
	if got = contextRequestPressureDirective(types.AgentExplorer, p, .7); !strings.Contains(got, "output limit unknown") || strings.Contains(got, "reserve 0") {
		t.Fatalf("unknown reserve taught as known zero: %s", got)
	}
}

func (a hmc278PressureAdapter) MaxContextTokens() int { return a.window }
func (a hmc278PressureAdapter) MaxOutputTokens() int  { return a.output }

func TestHMC278ContextRequestPressureCompleteAccounting(t *testing.T) {
	messages := []llm.Message{{Role: "user", Content: "source/query identity"}, {Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "emit", Params: []byte(`{"blocks":[]}`)}}}}
	tools := []llm.ToolSchema{{Name: "emit", Description: "same wire catalog", Parameters: []byte(`{"type":"object"}`)}}
	a := hmc278PressureAdapter{window: 10000, output: 1234}
	p := measureContextRequestPressure(a, messages, tools)
	wantPrompt := llm.EstimateRequestBytes(messages, tools)
	if p.promptBytes() != wantPrompt || p.ToolBytes == 0 || p.OutputTokens != 1234 || !p.OutputKnown || p.WindowBytes != 10000*types.BytesPerToken {
		t.Fatalf("wrong complete accounting: %+v", p)
	}
	wantRatio := float64(wantPrompt+1234*types.BytesPerToken) / float64(p.WindowBytes)
	if p.ratio() != wantRatio {
		t.Fatalf("ratio = %v, want %v", p.ratio(), wantRatio)
	}
	tail := llm.Message{Role: "system", Content: "current legal tool surface and reasoning language"}
	withTail := measureContextRequestPressure(a, append(messages, tail), tools)
	if withTail.MessageBytes-p.MessageBytes != llm.EstimateMessagesBytes([]llm.Message{tail}) || withTail.ratio() <= p.ratio() {
		t.Fatal("ephemeral tail not included")
	}
	more := llm.ToolSchema{Name: "patch", Parameters: []byte(`{"type":"object","description":"new repair surface"}`)}
	withPatch := measureContextRequestPressure(a, messages, append(tools, more))
	if withPatch.ToolBytes-p.ToolBytes != llm.EstimateToolSchemasBytes([]llm.ToolSchema{more}) || withPatch.ratio() <= p.ratio() {
		t.Fatal("schema-only growth not included")
	}
}

func TestHMC278ContextRequestPressureUnknownAndPathologicalLimits(t *testing.T) {
	messages := []llm.Message{{Role: "user", Content: "preserved"}}
	for _, output := range []int{0, -1} {
		p := measureContextRequestPressure(hmc278PressureAdapter{window: 100, output: output}, messages, nil)
		if p.OutputKnown || p.OutputTokens != 0 || p.ratio() != float64(p.MessageBytes)/float64(p.WindowBytes) {
			t.Fatalf("unknown output cap was made known: %+v", p)
		}
	}
	for _, window := range []int{0, -1, int(^uint(0) >> 1)} {
		p := measureContextRequestPressure(hmc278PressureAdapter{window: window, output: 10}, messages, nil)
		if p.WindowBytes != 0 || p.ratio() != 0 || p.MessageBytes == 0 {
			t.Fatalf("invalid limit must disable watchdog without dropping messages: %+v", p)
		}
	}
	p := measureContextRequestPressure(nil, messages, nil)
	if p.WindowBytes != 0 || p.OutputKnown || p.MessageBytes == 0 {
		t.Fatalf("nil adapter: %+v", p)
	}
	// Reserve arithmetic must not wrap when an adapter advertises an absurd cap.
	p = measureContextRequestPressure(hmc278PressureAdapter{window: 100, output: int(^uint(0) >> 1)}, nil, nil)
	if p.ratio() < 1 {
		t.Fatalf("output reserve overflow: %+v", p)
	}
}
