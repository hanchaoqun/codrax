package orchestrator

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/agent"
	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) extractStageHasRequiredWork() bool {
	if o == nil || o.busCtx == nil {
		return true
	}
	ac := ctxbuilder.BuildAgentContext(o.busCtx, types.AgentExtractor, types.StageExtract)
	return agent.ExtractStageHasRequiredWork(ac)
}

func (o *Orchestrator) hasReusableTurnBSlateForFinalize() bool {
	if o == nil || o.busCtx == nil {
		return false
	}
	if len(o.busCtx.AnswerSymbols) > 0 || len(o.busCtx.AnswerChains) > 0 {
		return true
	}
	if o.busCtx.Mutable == nil {
		return false
	}
	if symbols, _ := o.busCtx.Mutable.EmittedAnswerSymbols(); len(symbols) > 0 {
		return true
	}
	return len(o.busCtx.Mutable.EmittedHypothesisVerdicts()) > 0
}

func (o *Orchestrator) reusableTurnBSlateSummary() string {
	if o == nil || o.busCtx == nil {
		return "none"
	}
	mutableSymbols := 0
	verdicts := 0
	if o.busCtx.Mutable != nil {
		if symbols, _ := o.busCtx.Mutable.EmittedAnswerSymbols(); len(symbols) > 0 {
			mutableSymbols = len(symbols)
		}
		verdicts = len(o.busCtx.Mutable.EmittedHypothesisVerdicts())
	}
	return fmt.Sprintf("answer_symbols=%d emitted_answer_symbols=%d answer_chains=%d hypothesis_verdicts=%d",
		len(o.busCtx.AnswerSymbols), mutableSymbols, len(o.busCtx.AnswerChains), verdicts)
}
