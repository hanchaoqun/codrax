package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) appendAnswerDisplayAttachment(out *agent.StageOutput, att types.AnswerDisplayAttachment) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || out == nil {
		return
	}
	if strings.TrimSpace(att.Body) == "" {
		return
	}
	attachments := append(o.busCtx.Mutable.AnswerDisplayAttachments(), att)
	if doc := o.busCtx.Mutable.AnswerDocumentV2(); doc != nil {
		// B882: post-finalize producers share the emit path's typed accepted-
		// document attachment boundary; rejected model prose stays telemetry.
		attachments = tool.FilterAcceptedAnswerDisplayAttachments(doc, attachments)
		o.busCtx.Mutable.SetAnswerDisplayAttachments(attachments)
		// TRUNC 批 (§29.10-1): this overwrite used to call
		// render.RenderAnswerDocumentWithAttachments directly — dropping
		// both the finalizer's authority hedging AND every last-mile
		// 系统补充 block from the customer-facing answer (huadong_792
		// witness: the whole trace_query 关键观测核对 block vanished
		// exactly on the first-draft-attachment path). Mirror
		// parseOutputV2: hedge the defensive clone, then render through
		// the last-mile chokepoint.
		render.ApplyAuthorityHedging(doc, finalizerAutoRepairAuthorityEvidencePool(o.busCtx), o.busCtx.Language)
		out.FinalAnswer = o.renderFinalAnswerWithLastMileSupplements(doc, attachments)
		return
	}
	// A recovered output has no accepted document to re-render. Isolate the
	// new reference before appending it, and persist only the cleaned carrier;
	// a different StageOutput must not revive the retained raw first draft.
	cleaned := tool.IsolateAnswerDisplayAttachmentsForRender(o.busCtx, []types.AnswerDisplayAttachment{att}, o.busCtx.Language)
	attachments = append(o.busCtx.Mutable.AnswerDisplayAttachments(), cleaned...)
	o.busCtx.Mutable.SetAnswerDisplayAttachments(tool.IsolateAnswerDisplayAttachmentsForRender(o.busCtx, attachments, o.busCtx.Language))
	for _, display := range cleaned {
		out.FinalAnswer = strings.TrimRight(out.FinalAnswer, "\n") + "\n\n---\n\n**" +
			strings.TrimSpace(display.Title) + "**\n\n" + strings.TrimSpace(display.Body) + "\n\n---\n"
	}
	out.FinalAnswer = tool.DeduplicateDiagramIsolationNotice(out.FinalAnswer, o.busCtx.Language)
}
