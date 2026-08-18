package agent

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// RenderAnswerDocumentWithLastMileSupplements renders the structured V2
// answer + preserved display attachments PLUS every deterministic last-mile
// system supplement the finalizer's parseOutputV2 appends (系统补充 blocks:
// Trace 关键观测核对, 结构化指标核对, read-audit lanes, 输出维度核对).
// Stage bindings are intentionally absent here: their precise authority is
// supplied to the Finalizer prompt and the model remains the answer author.
//
// TRUNC 批 (P1, §29.10-1, 2026-07-09): orchestrator-side FinalAnswer
// re-render paths (first-draft attachment, finalizer auto-repair,
// transient-failure recovery) previously called
// render.RenderAnswerDocumentWithAttachments directly and silently DROPPED
// every supplement from the customer-facing answer — huadong_792 witness:
// the entire 「系统补充：Trace 关键观测核对」 block vanished. Any code
// that overwrites a finalize-stage StageOutput.FinalAnswer MUST render
// through this function (orchestrator side: the
// renderFinalAnswerWithLastMileSupplements chokepoint, pinned by
// TestTRUNCFinalAnswerRerenderChokepointStructural).
func RenderAnswerDocumentWithLastMileSupplements(ctx *types.AgentContext, doc *types.AnswerDocumentV2, attachments []types.AnswerDisplayAttachment, lang string) string {
	e := &answerDocumentEvaluator{language: lang}
	return e.renderAnswerDocumentWithLastMileSupplements(ctx, doc, attachments)
}
