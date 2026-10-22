package agent

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Optional optimization only: the owner must attest that this dispatch has
// already accepted an answer and is spending its one advisory repair turn.
type acceptedAnswerPressureKeeper interface {
	keepAcceptedAnswerUnderPressure(*types.AgentContext) bool
}

type contextRequestPressure struct {
	MessageBytes, ToolBytes, WindowBytes int
	OutputTokens                         int
	OutputKnown                          bool
}

func measureContextRequestPressure(adapter llm.Adapter, messages []llm.Message, tools []llm.ToolSchema) contextRequestPressure {
	p := contextRequestPressure{MessageBytes: llm.EstimateMessagesBytes(messages), ToolBytes: llm.EstimateToolSchemasBytes(tools)}
	if adapter == nil {
		return p
	}
	window := adapter.MaxContextTokens()
	if window > 0 && window <= int(^uint(0)>>1)/types.BytesPerToken {
		p.WindowBytes = window * types.BytesPerToken
	}
	if output := adapter.MaxOutputTokens(); output > 0 {
		p.OutputTokens, p.OutputKnown = output, true
	}
	return p
}

func (p contextRequestPressure) promptBytes() int { return p.MessageBytes + p.ToolBytes }

func (p contextRequestPressure) ratio() float64 {
	if p.WindowBytes <= 0 {
		return 0
	}
	return (float64(p.MessageBytes) + float64(p.ToolBytes) + float64(p.OutputTokens)*types.BytesPerToken) / float64(p.WindowBytes)
}

func contextRequestPressureDirective(name types.AgentName, p contextRequestPressure, hard float64) string {
	reserve := "output limit unknown; no exact output reserve is available"
	if p.OutputKnown {
		reserve = fmt.Sprintf("known output reserve %d tokens", p.OutputTokens)
	}
	why := fmt.Sprintf("Request messages %d bytes + tool schemas %d bytes; %s. Estimated combined pressure %.1f%% of %d window bytes ≥ hard threshold %.1f%%; another iteration risks a provider 400.", p.MessageBytes, p.ToolBytes, reserve, p.ratio()*100, p.WindowBytes, hard*100)
	return renderContextPressureDirective(name, why)
}

func (e *answerDocumentEvaluator) keepAcceptedAnswerUnderPressure(ctx *types.AgentContext) bool {
	if e == nil || !e.postEmitAdvisoryDelivered || ctx == nil || ctx.Stage != types.StageFinalize || ctx.Mutable == nil || e.mu != ctx.Mutable {
		return false
	}
	doc := ctx.Mutable.AnswerDocumentV2()
	return doc != nil && len(doc.Blocks) > 0
}
