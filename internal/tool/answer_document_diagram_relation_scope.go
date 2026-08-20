package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// DiagramRequestedRelationScopeIssue is a closed structural mismatch between
// the typed request-spine coverage and the model-authored diagram disclosure.
// It reads no request, answer, or Mermaid prose.
type DiagramRequestedRelationScopeIssue string

const (
	DiagramRequestedRelationScopeMissing   DiagramRequestedRelationScopeIssue = "missing_partial_unproven_scope"
	DiagramRequestedRelationScopeStale     DiagramRequestedRelationScopeIssue = "stale_partial_unproven_scope"
	DiagramRequestedRelationScopeDuplicate DiagramRequestedRelationScopeIssue = "duplicate_partial_unproven_scope"
)

type DiagramRequestedRelationScopeMismatch struct {
	BlockID string
	Issue   DiagramRequestedRelationScopeIssue
}

// DiagramRequestedRelationScopeMismatches compares one model-authored typed
// scope field against the same parser-owned relation component calculation
// used by participant coverage and candidate publication. The disclosure is
// required only for the narrow partial-spine shape: at least one requested
// participant is covered by the request-scoped provider, but the complete
// incident participant slate is not. No document prose or visible edge label
// participates, and the result cannot authorize a missing bridge.
func DiagramRequestedRelationScopeMismatches(
	doc *types.AnswerDocumentV2,
	view *types.AnswerSemanticView,
	rm types.RequestModel,
	evidence []types.EvidenceItem,
	stagePrecedence ...stageauthority.PrecedenceRelation,
) []DiagramRequestedRelationScopeMismatch {
	if doc == nil || view == nil || view.DiagramPlan == nil || !view.DiagramPlan.Required ||
		view.RelationAxis != types.AxisFlow || types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace ||
		!answerDocumentContainsDiagramPayload(doc) {
		return nil
	}
	obligations := diagramIncidentParticipantObligations(view)
	if len(obligations) < 2 {
		return nil
	}
	allSurfaces := make([][]string, 0, len(obligations))
	for _, obligation := range obligations {
		surfaces := []string{strings.TrimSpace(obligation.Identity)}
		for _, resolved := range types.DiagramParticipantIdentitySurfaces(rm, obligation) {
			if !diagramParticipantSurfaceListContainsExact(surfaces, resolved) {
				surfaces = append(surfaces, resolved)
			}
		}
		allSurfaces = append(allSurfaces, surfaces)
	}
	scope := buildFlowParticipantRelationScope(rm, obligations, allSurfaces, evidence, stagePrecedence)
	required := scope.requestScopedSubsetIncomplete

	type declaration struct{ blockID string }
	declarations := make([]declaration, 0, 1)
	for _, block := range doc.Blocks {
		if block.RequestedRelationScope == types.DiagramRelationScopeUnknown {
			continue
		}
		// Normalization already rejects non-diagram and unknown enum shapes.
		// Keep this checker fail-closed for programmatic/test documents too.
		if block.Kind == types.BlockDiagram && block.Diagram != nil &&
			block.RequestedRelationScope == types.DiagramRelationScopePartialUnproven {
			declarations = append(declarations, declaration{blockID: block.ID})
		}
	}
	if required && len(declarations) == 0 {
		return []DiagramRequestedRelationScopeMismatch{{Issue: DiagramRequestedRelationScopeMissing}}
	}
	if !required && len(declarations) > 0 {
		out := make([]DiagramRequestedRelationScopeMismatch, 0, len(declarations))
		for _, declaration := range declarations {
			out = append(out, DiagramRequestedRelationScopeMismatch{
				BlockID: declaration.blockID, Issue: DiagramRequestedRelationScopeStale,
			})
		}
		return out
	}
	if len(declarations) > 1 {
		out := make([]DiagramRequestedRelationScopeMismatch, 0, len(declarations)-1)
		for _, declaration := range declarations[1:] {
			out = append(out, DiagramRequestedRelationScopeMismatch{
				BlockID: declaration.blockID, Issue: DiagramRequestedRelationScopeDuplicate,
			})
		}
		return out
	}
	return nil
}

func preCheckDiagramRequestedRelationScope(
	doc *types.AnswerDocumentV2,
	view *types.AnswerSemanticView,
	pctx *preEmitCheckContext,
) []emitFixHint {
	if pctx == nil || pctx.ctx == nil || pctx.ctx.AnalysisIR == nil {
		return nil
	}
	evidence := DiagramEvidenceForValidation(pctx.ctx, doc, view, pctx.evidenceItems())
	evidence = preEmitEvidenceWithExactTypedDiagramRelations(doc, pctx.ctx, evidence)
	mismatches := DiagramRequestedRelationScopeMismatches(
		doc, view, pctx.ctx.AnalysisIR.RequestModel, evidence,
		diagramVerifiedReadModeStagePrecedence(pctx.ctx, view)...,
	)
	if len(mismatches) == 0 {
		return nil
	}
	parts := make([]string, 0, len(mismatches))
	for _, mismatch := range mismatches {
		part := "issue=" + string(mismatch.Issue)
		if strings.TrimSpace(mismatch.BlockID) != "" {
			part = fmt.Sprintf("block=%q %s", mismatch.BlockID, part)
		}
		parts = append(parts, part)
	}
	return []emitFixHint{{
		Field:      "blocks[kind=diagram].requested_relation_scope",
		HardSignal: preEmitHardSignalTypedDiagramParticipantCoverage,
		OffendingBlockKinds: []types.AnswerBlockKind{
			types.BlockDiagram,
		},
		ExpectedShape: "Typed requested-relation scope mismatch: " + strings.Join(parts, "; ") +
			". When the typed request-spine authority is partial, put requested_relation_scope=partial_unproven on exactly one model-authored diagram block that presents the requested relation. Preserve all proved local edges and exact anchors, but do not invent a bridge. Remove this field when the typed authority is not partial, and remove duplicate declarations. The field is block-level, never nested inside diagram; it is rendered as reader-facing coverage language, so do not repeat the raw enum in prose or labels.",
		Reason: "the parser-owned request-scoped relation component covers only a strict subset of the typed incident participant slate; per-participant local incidence cannot prove one complete requested relation",
	}}
}

func DiagramRequestedRelationScopeMismatchesWithRuntimeContext(
	ctx *types.BusContext,
	doc *types.AnswerDocumentV2,
	view *types.AnswerSemanticView,
	evidence []types.EvidenceItem,
) []DiagramRequestedRelationScopeMismatch {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	evidence = DiagramEvidenceForValidation(ctx, doc, view, evidence)
	evidence = preEmitEvidenceWithExactTypedDiagramRelations(doc, ctx, evidence)
	return DiagramRequestedRelationScopeMismatches(
		doc, view, ctx.AnalysisIR.RequestModel, evidence,
		diagramVerifiedReadModeStagePrecedence(ctx, view)...,
	)
}
