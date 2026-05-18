// Package tool — answer_document_pre_emit_check.go (P1, 2026-05-10).
//
// Emit-time pre-validation chokepoint. When the LLM calls
// emit_answer_document, this layer runs the four most common
// structural-compliance checks BEFORE the doc lands on Mutable.
// Violations are surfaced as a structured user-vocab fix-list
// inside failEmit's rejection envelope — the LLM retries within
// the same dispatch (BaseAgent.emitAnswerDocumentRejectSignal
// captures !LastToolResult.Success and re-prompts), so the cost is
// one tool-call round trip, NOT a full repair-loop iteration with
// orchestrator dispatch overhead.
//
// Pre-emit only covers the four STRICT-classified validator axes
// that the post-emit chain in internal/orchestrator/contract_check.go
// runs after persist:
//
//   - block_coverage_missing   (validateRequiredBlockCoverage)
//   - principal_claim_use_missing (validatePrincipalClaimUse)
//   - uncertainty_block_missing  (validateUncertaintyBlockPresence)
//   - facet_uncovered          (validateFacetCoverage)
//
// SOFT validators (richness regression / inline-code density / etc.)
// stay in the post-emit chain — they're advisory in nature and
// pre-empting them at emit time would add noise without reducing
// load-bearing failures.
//
// Forensic anchor: May-9 sweep showed block_coverage_missing +
// facet_uncovered + uncertainty_block + principal_claim_use_missing
// account for ~69% of repair-loop activations. Pre-emit chokepoint
// converts those from a full orchestrator retry round (3-5min) to
// a same-call tool retry (~5s).
//
// Architecture:
//
//   - The four checks are intentionally LIGHTWEIGHT re-implementations
//     of the post-emit logic (not copies of the full validator
//     functions). Pre-emit needs only "is the structural shape
//     compliant?" — it does NOT need the full types.Violation envelope
//     (cluster keys / suspected roots / repair locus mapping). That
//     distinction is the post-emit chain's job.
//   - Output is []emitFixHint with user-vocab Field+ExpectedShape
//     strings the failEmit rejection turns into a numbered list the
//     LLM can mechanically apply (matches the design doc's
//     "structured fix-list" surface).
//   - Empty fix-list → ApplyAndPersistMutation runs unchanged.
//   - This is a NEW chokepoint, not a replacement; the post-emit
//     chain remains the load-bearing correctness layer for SOFT
//     advisory cases + cluster-routed repair planning.
package tool

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/types"
)

// preEmitOracleFromCtx pulls the SymbolOracle the orchestrator
// stashed on Mutable. Returns nil when:
//   - ctx or Mutable is nil (unit-test / no-bus paths)
//   - the orchestrator hasn't wired the oracle yet
//
// Tools in internal/tool can't import internal/tool/repomap to
// build the oracle directly (cycle: repomap → tool → repomap), so
// the orchestrator constructs once during explorer wire-up and
// Mutable.SetSymbolOracle ferries it across the package boundary.
// Mirrors the SetCrossRepoOracle pattern at
// internal/tool/ground/ground.go:23.
//
// 2026-05-10 P1.
func preEmitOracleFromCtx(ctx *types.BusContext) types.SymbolOracle {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	return ctx.Mutable.SymbolOracle()
}

type preEmitCheckContext struct {
	ctx       *types.BusContext
	groundCtx *ground.Context
	evidence  *preEmitEvidenceIndex
}

type preEmitEvidenceIndex struct {
	items  []types.EvidenceItem
	byFile map[string][]types.EvidenceItem
}

func newPreEmitCheckContext(ctxOpt ...*types.BusContext) *preEmitCheckContext {
	var ctx *types.BusContext
	if len(ctxOpt) > 0 {
		ctx = ctxOpt[0]
	}
	return &preEmitCheckContext{ctx: ctx}
}

func (c *preEmitCheckContext) ctxArgs() []*types.BusContext {
	if c == nil || c.ctx == nil {
		return nil
	}
	return []*types.BusContext{c.ctx}
}

func (c *preEmitCheckContext) evidenceItems() []types.EvidenceItem {
	if c == nil || c.ctx == nil || c.ctx.Mutable == nil {
		return nil
	}
	if c.evidence == nil {
		c.evidence = newPreEmitEvidenceIndex(c)
	}
	return c.evidence.items
}

func (c *preEmitCheckContext) citedEvidenceItems(cit types.Citation) ([]types.EvidenceItem, bool) {
	if c == nil || c.ctx == nil || c.ctx.Mutable == nil {
		return nil, false
	}
	if c.evidence == nil {
		c.evidence = newPreEmitEvidenceIndex(c)
	}
	return c.evidence.citedEvidenceItems(c.canonicalCitation(cit))
}

func (c *preEmitCheckContext) canonicalPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if c != nil && c.ctx != nil {
		if c.groundCtx == nil {
			c.groundCtx = ground.BuildContext(c.ctx)
		}
		if canon := ground.CanonicalContextPath(c.groundCtx, raw); canon != "" {
			return canon
		}
	}
	return ground.CanonicalRepoRelative(raw, "")
}

func (c *preEmitCheckContext) canonicalCitation(cit types.Citation) types.Citation {
	cit.File = c.canonicalPath(cit.File)
	return cit
}

func newPreEmitEvidenceIndex(pctx *preEmitCheckContext) *preEmitEvidenceIndex {
	idx := &preEmitEvidenceIndex{}
	var ctx *types.BusContext
	if pctx != nil {
		ctx = pctx.ctx
	}
	if ctx == nil || ctx.Mutable == nil {
		return idx
	}
	var raw []types.EvidenceItem
	if artifacts := ctx.Mutable.TurnAArtifacts(); artifacts != nil && len(artifacts.EvidenceItems) > 0 {
		raw = append(raw, artifacts.EvidenceItems...)
	}
	if emitted := ctx.Mutable.EmittedEvidence(); len(emitted) > 0 {
		raw = append(raw, emitted...)
	}
	if len(raw) == 0 {
		return idx
	}
	idx.byFile = make(map[string][]types.EvidenceItem)
	idx.items = make([]types.EvidenceItem, 0, len(raw))
	for _, ev := range raw {
		ev.Source = pctx.canonicalPath(ev.Source)
		idx.items = append(idx.items, ev)
		file := strings.TrimSpace(ev.Source)
		if file == "" {
			continue
		}
		idx.byFile[file] = append(idx.byFile[file], ev)
	}
	return idx
}

func (idx *preEmitEvidenceIndex) citedEvidenceItems(cit types.Citation) ([]types.EvidenceItem, bool) {
	if idx == nil {
		return nil, false
	}
	file := strings.TrimSpace(cit.File)
	if file == "" || cit.Line <= 0 {
		return nil, false
	}
	candidates := idx.byFile[file]
	if len(candidates) == 0 {
		return nil, false
	}
	out := preEmitCitedEvidenceFromPool(candidates, file, cit.Line)
	return out, len(out) > 0
}

// emitFixHint is the user-facing fix instruction surfaced when a
// pre-emit check fails. Formatted by formatEmitFixHints into a
// rejection prose string that the LLM reads and acts on within the
// same tool dispatch.
//
// Field is the schema-level token (block.facet_ids /
// block.claim_uses / etc.) — matches the user-section vocab the
// finalizer skill prompt teaches. ExpectedShape names what the LLM
// should emit; Reason is the one-sentence "why" so the LLM
// understands the structural requirement, not just the literal fix.
type emitFixHint struct {
	Field         string
	ExpectedShape string
	Reason        string
}

// runPreEmitChecks runs the STRICT chokepoint checks on the
// just-built (but-not-yet-persisted) document. Returns nil when the
// shape is compliant; otherwise a non-empty list of fix hints the
// caller wraps in failEmit. view nil → returns nil (no view, no
// expected shape — pre-emit silently passes through and the
// post-emit chain takes over).
//
// oracle is OPTIONAL — when nil, the enum-label grounding check
// (P1 2026-05-10) silently passes; the post-emit chain still
// catches hallucinations via validateEnumerationItemLabelHallucination.
// Non-nil oracle activates the same gate at the chokepoint to avoid
// burning a full repair-loop round on a fixable label hallucination.
func runPreEmitChecks(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, oracle types.SymbolOracle, ctxOpt ...*types.BusContext) []emitFixHint {
	return runPreEmitChecksWithContext(doc, view, oracle, newPreEmitCheckContext(ctxOpt...))
}

func runPreEmitChecksWithContext(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, oracle types.SymbolOracle, pctx *preEmitCheckContext) []emitFixHint {
	if doc == nil || view == nil {
		return nil
	}
	var hints []emitFixHint
	ctxOpt := pctx.ctxArgs()

	// 0. Citation-pool carrier integrity. Run this before semantic
	// member checks so a retry that dropped citations[] or references
	// an out-of-range index gets a direct schema repair instead of a
	// misleading "all principal members are missing" diagnosis.
	if h := preCheckCitationPoolIntegrity(doc); len(h) > 0 {
		return h
	}
	if h := preCheckNegativeCitationBounds(doc); len(h) > 0 {
		return h
	}
	if h := preCheckRuntimeObservationRepoContamination(doc, ctxOpt...); len(h) > 0 {
		return h
	}
	if h := preCheckArtifactObservedFrameCitations(doc, ctxOpt...); len(h) > 0 {
		return h
	}
	// Carrier visibility is governed by LLM-facing schema/prompt wording and
	// typed row roles, not by post-hoc keyword matching over RawRequest or the
	// model-rendered answer text.

	// 1. Required block kind + count compliance.
	if h := preCheckRequiredBlocks(doc, view); len(h) > 0 {
		hints = append(hints, h...)
	}
	if h := preCheckSummaryLeadBlock(doc, view); len(h) > 0 {
		hints = append(hints, h...)
	}

	// 2. Principal-block claim_use presence.
	if h := preCheckPrincipalClaimUse(doc, view); len(h) > 0 {
		hints = append(hints, h...)
	}
	if h := preCheckRequiredCandidateRoles(doc, view); len(h) > 0 {
		hints = append(hints, h...)
	}
	if h := preCheckRequiredMechanismAnchors(doc, view); len(h) > 0 {
		hints = append(hints, h...)
	}
	if h := preCheckInactiveTypedDecisionVerdicts(doc, view); len(h) > 0 {
		hints = append(hints, h...)
	}
	if h := preCheckErrorGranularityVerdict(doc, view); len(h) > 0 {
		hints = append(hints, h...)
	}
	if h := preCheckCurrentStatusVerdict(doc, view); len(h) > 0 {
		hints = append(hints, h...)
	}

	// 3. Uncertainty block presence (when contract requires it).
	if h := preCheckUncertaintyBlock(doc, view); len(h) > 0 {
		hints = append(hints, h...)
	}

	// 4. Required facet coverage.
	if h := preCheckFacetCoverage(doc, view); len(h) > 0 {
		hints = append(hints, h...)
	}

	// 5. Per-item label/citation alignment. A symbol-like list label with
	// a citation must name the same cited evidence endpoint; otherwise the
	// rendered answer silently shifts file:line proof across adjacent hops.
	if h := preCheckItemCitationAlignmentWithContext(doc, view, pctx); len(h) > 0 {
		hints = append(hints, h...)
	}

	// 5b. Typed item/citation role alignment. An item that explicitly
	// asserts a typed evidence role must cite that same role, not a
	// nearby definition / guard / adjacent item that happens to share
	// one endpoint. The projection lives in types so new role shapes
	// (import/path/span/route/etc.) extend the central contract instead
	// of adding validator-specific patches.
	if h := preCheckCallChainItemCitationRoleAlignmentWithContext(doc, view, pctx); len(h) > 0 {
		hints = append(hints, h...)
	}

	// 5c. Principal support member coverage. For enumeration answers,
	// every answer-grade member already selected into the principal
	// support lane must be rendered as a cited item/row; the finalizer
	// should not compress away explorer-emitted members.
	if h := preCheckPrincipalSupportMemberCoverage(doc, ctxOpt...); len(h) > 0 {
		hints = append(hints, h...)
	}

	// 2026-05-16 (Fix 1) — multi-repo inactive-scope disclosure.
	// When typed BusContext.PendingSubRepos is non-empty AND the
	// answer is bounded (absent exact target, empty role-locate
	// slate, or scope-limited enumeration), the answer MUST
	// disclose the inactive scope. Pre-emit catches this same
	// dispatch so the model rewrites with disclosure instead of
	// burning a retry round.
	if h := preCheckInactiveScopeDisclosure(doc, ctxOpt...); len(h) > 0 {
		hints = append(hints, h...)
	}

	// 5d. Model-authored scalar aggregate preservation. A scalar_value
	// aggregate fact is an explicit typed handoff from exploration, not
	// scratchpad prose; the finalizer must carry the value into visible
	// answer blocks instead of remembering it only in thinking.
	if h := preCheckAggregateScalarValueCoverage(doc, ctxOpt...); len(h) > 0 {
		hints = append(hints, h...)
	}
	if h := preCheckAggregateMemberSetCoverage(doc, ctxOpt...); len(h) > 0 {
		if preEmitAggregateMemberSetCoverageHardGate(ctxOpt...) {
			hints = append(hints, h...)
		} else {
			logging.Warning("[emit_answer_document] aggregate member_set coverage advisory not hard-rejected: %s", formatEmitFixHints(h))
		}
	}
	if h := preCheckAggregateCardinalityConsistency(doc, ctxOpt...); len(h) > 0 {
		hints = append(hints, h...)
	}
	if h := preCheckRelationMemberSetAnswerShape(doc, ctxOpt...); len(h) > 0 {
		hints = append(hints, h...)
	}

	// 5e. Bounded exact absence. Keep the absence hard gate inside
	// the emit dispatch too, so missing negative-scope citations are
	// repaired before the doc reaches the orchestrator retry loop.
	if h := preCheckAbsenceScopeBound(doc); len(h) > 0 {
		hints = append(hints, h...)
	}
	// Multi-repo absence disclosure is prompted from typed PendingSubRepos /
	// exact_resolution state. This pre-emit chokepoint must not inspect the
	// model's rendered prose for path-name keywords to decide control flow.

	// 6. Enumeration item label grounding (P1 2026-05-10).
	// Catches the hallucinated identifier-shape labels that drove
	// 70% of post-emit repair-loop violations in the 2026-05-10
	// sweep digest. Mirrors validateEnumerationItemLabelHallucination
	// at the chokepoint so the LLM gets the fix hint inside the
	// SAME dispatch instead of paying a full retry round.
	if h := preCheckEnumerationLabelGroundingWithContext(doc, oracle, pctx); len(h) > 0 {
		if preEmitEnumerationLabelGroundingHardGate(pctx) {
			hints = append(hints, h...)
		} else {
			logging.Warning("[emit_answer_document] enumeration label grounding advisory not hard-rejected: %s", formatEmitFixHints(h))
		}
	}

	return hints
}

func preCheckCitationPoolIntegrity(doc *types.AnswerDocumentV2) []emitFixHint {
	if doc == nil {
		return nil
	}
	maxRef := -1
	refCount := 0
	for _, block := range doc.Blocks {
		for _, item := range block.Items {
			if item.CitationRef < 0 {
				continue
			}
			refCount++
			if item.CitationRef > maxRef {
				maxRef = item.CitationRef
			}
		}
	}
	if refCount == 0 {
		return nil
	}
	if len(doc.Citations) > maxRef {
		return nil
	}
	expected := "preserve / emit a top-level citations[] pool with at least " +
		fmt.Sprintf("%d entries so every non-negative blocks[].items[].citation_ref resolves to an existing citation object", maxRef+1)
	if len(doc.Citations) == 0 {
		expected = "preserve / emit the top-level citations[] pool; this payload contains cited items but no citations[] entries"
	}
	return []emitFixHint{{
		Field:         "citations[]",
		ExpectedShape: expected,
		Reason:        "citation_ref is an index into the model-emitted citations[] array; semantic coverage checks cannot run correctly until the carrier's citation pool is structurally complete.",
	}}
}

func preCheckNegativeCitationBounds(doc *types.AnswerDocumentV2) []emitFixHint {
	if doc == nil {
		return nil
	}
	for _, c := range doc.Citations {
		if c.Scope != types.ScopeNegative {
			continue
		}
		if strings.TrimSpace(c.NegativePattern) != "" {
			continue
		}
		return []emitFixHint{{
			Field:         "citations[]",
			ExpectedShape: "remove negative-scope citations that are not bounded absence proofs, or add `negative_pattern` naming the exact query whose zero matches prove the absence",
			Reason:        "a citation rendered as an absence proof must be reproducible; attached log / trace observations should use citation_ref=-1 instead of a fake negative citation.",
		}}
	}
	return nil
}

func preCheckRuntimeObservationRepoContamination(doc *types.AnswerDocumentV2, ctxOpt ...*types.BusContext) []emitFixHint {
	if doc == nil || len(ctxOpt) == 0 || ctxOpt[0] == nil {
		return nil
	}
	ctx := ctxOpt[0]
	plan := types.BuildAnswerSurfacePlanForBusContext(ctx)
	if plan == nil || !plan.RuntimeGroundingDisposition.IsActive() || plan.CurrentStatusDiagnosticRequired {
		return nil
	}
	if len(doc.Citations) > 0 {
		return []emitFixHint{{
			Field:         "citations[]",
			ExpectedShape: "for an observation-only external runtime artifact answer, omit current-repo citations and set runtime-observation items to `citation_ref=-1`",
			Reason:        "this request asks what the attached log / trace observed; current-repo citations would imply the checkout produced or proves the external artifact.",
		}}
	}
	return nil
}

func preCheckArtifactObservedFrameCitations(doc *types.AnswerDocumentV2, ctxOpt ...*types.BusContext) []emitFixHint {
	if doc == nil || len(doc.Citations) == 0 || len(ctxOpt) == 0 || ctxOpt[0] == nil {
		return nil
	}
	plan := types.BuildAnswerSurfacePlanForBusContext(ctxOpt[0])
	if plan == nil || !plan.IsCrashSourcedRootCause() {
		return nil
	}
	for i, cit := range doc.Citations {
		observed, current, ok := citationMatchesDriftedArtifactFrame(plan, cit)
		if !ok {
			continue
		}
		expected := "runtime artifact frame coordinates should stay in observed-artifact rows with `citation_ref=-1`"
		if current != "" {
			expected += "; cite the current grounded source anchor instead: " + current
		}
		return []emitFixHint{{
			Field:         fmt.Sprintf("citations[%d]", i),
			ExpectedShape: expected,
			Reason:        fmt.Sprintf("the citation points at observed artifact coordinate %s, which is not the current source proof for this checkout.", observed),
		}}
	}
	return nil
}

func citationMatchesDriftedArtifactFrame(plan *types.AnswerSurfacePlan, cit types.Citation) (observed string, current string, ok bool) {
	file := normalizeArtifactCitationPath(cit.File)
	if plan == nil || file == "" || cit.Line <= 0 {
		return "", "", false
	}
	for _, seed := range plan.ExternalObservationSeeds {
		if !types.ExternalObservationSeedIsFrame(seed) {
			continue
		}
		seedFile := normalizeArtifactCitationPath(seed.File)
		if seedFile == "" || seed.Line <= 0 || seedFile != file || seed.Line != cit.Line {
			continue
		}
		observed = fmt.Sprintf("%s:%d", seedFile, seed.Line)
		anchoredFile := normalizeArtifactCitationPath(seed.AnchoredFile)
		if anchoredFile == "" || seed.AnchoredLine <= 0 {
			continue
		}
		current = fmt.Sprintf("%s:%d", anchoredFile, seed.AnchoredLine)
		if anchoredFile == file && seed.AnchoredLine == cit.Line {
			return "", "", false
		}
		return observed, current, true
	}
	for _, anchor := range append(append([]types.LogSourceDriftAnchor(nil), plan.LogObservedAnchors...), plan.LogSourceDriftAnchors...) {
		anchorFile := normalizeArtifactCitationPath(anchor.File)
		if anchorFile == "" || anchor.ObservedLine <= 0 || anchorFile != file || anchor.ObservedLine != cit.Line {
			continue
		}
		if anchor.AnchoredLine > 0 {
			current = fmt.Sprintf("%s:%d", anchorFile, anchor.AnchoredLine)
			if anchor.AnchoredLine == cit.Line {
				return "", "", false
			}
		}
		return fmt.Sprintf("%s:%d", anchorFile, anchor.ObservedLine), current, true
	}
	return "", "", false
}

func normalizeArtifactCitationPath(path string) string {
	return strings.TrimSpace(strings.ReplaceAll(path, `\`, `/`))
}

func answerDocumentVisibleText(doc *types.AnswerDocumentV2) string {
	if doc == nil {
		return ""
	}
	var b strings.Builder
	for _, block := range doc.Blocks {
		b.WriteString(types.AnswerBlockVisibleSurface(block))
		b.WriteByte('\n')
	}
	for _, caveat := range doc.Caveats {
		b.WriteString(caveat)
		b.WriteByte('\n')
	}
	return b.String()
}

func preCheckSummaryLeadBlock(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	if doc == nil || view == nil || !viewRequiresSummaryBlock(view) || len(doc.Blocks) == 0 {
		return nil
	}
	firstRenderable := -1
	summaryAt := -1
	for i, block := range doc.Blocks {
		if !answerBlockHasRenderableSurface(block) {
			continue
		}
		if firstRenderable < 0 {
			firstRenderable = i
		}
		if block.Kind == types.BlockSummary && summaryAt < 0 {
			summaryAt = i
		}
	}
	if summaryAt < 0 || firstRenderable < 0 || summaryAt == firstRenderable {
		return nil
	}
	return []emitFixHint{{
		Field:         "blocks[]",
		ExpectedShape: "place the required summary block first in blocks[] before principal lists, tables, sections, diagrams, or caveats",
		Reason:        "the summary block is the answer lead-in; rendering it after detail blocks makes the user-facing answer read backwards even when the facts and citations are correct.",
	}}
}

func viewRequiresSummaryBlock(view *types.AnswerSemanticView) bool {
	if view == nil {
		return false
	}
	for _, req := range view.RequiredBlocks {
		if req.Kind == types.BlockSummary && req.Required {
			return true
		}
	}
	return false
}

func answerBlockHasRenderableSurface(block types.AnswerBlock) bool {
	if strings.TrimSpace(block.Text) != "" ||
		strings.TrimSpace(block.Title) != "" ||
		len(block.Columns) > 0 ||
		len(block.Items) > 0 ||
		block.Diagram != nil {
		return true
	}
	return false
}

func preCheckItemCitationAlignment(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctxOpt ...*types.BusContext) []emitFixHint {
	return preCheckItemCitationAlignmentWithContext(doc, view, newPreEmitCheckContext(ctxOpt...))
}

func preCheckItemCitationAlignmentWithContext(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, pctx *preEmitCheckContext) []emitFixHint {
	if doc == nil || pctx == nil || pctx.ctx == nil || pctx.ctx.Mutable == nil {
		return nil
	}
	type mismatch struct {
		blockID    string
		itemID     string
		label      string
		cite       string
		candidates []string
	}
	var mismatches []mismatch
	for _, b := range doc.Blocks {
		switch b.Kind {
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		default:
			continue
		}
		if preEmitBlockUsesNonSymbolLabelSurface(b, view) {
			continue
		}
		for _, item := range b.Items {
			label := strings.TrimSpace(item.Label)
			if label == "" || !preEmitLabelNeedsCitationAlignment(label) {
				continue
			}
			text := preEmitItemNonLabelSurface(item)
			if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
				continue
			}
			cit := pctx.canonicalCitation(doc.Citations[item.CitationRef])
			if types.AnswerLocationLabelMatchesCitation(label, cit) {
				continue
			}
			if preEmitCitationEnclosingFunctionSupportsLabel(label, cit) {
				continue
			}
			if preEmitCitationEnclosingFunctionConflictsWithQualifiedLabel(label, cit) {
				mismatches = append(mismatches, mismatch{
					blockID:    b.ID,
					itemID:     item.ID,
					label:      label,
					cite:       fmt.Sprintf("%s:%d", strings.TrimSpace(cit.File), cit.Line),
					candidates: preEmitCandidateCitationLocationsForLabelWithContext(pctx, label, 4),
				})
				continue
			}
			if preEmitCitationSupportsAggregateItemWithContext(pctx, label, text, cit) {
				continue
			}
			if surface, ok := types.ParseAnswerSourceLocationSurface(label); ok {
				if types.AnswerSourceLocationSurfaceMatchesCitation(surface, cit) {
					continue
				}
				mismatches = append(mismatches, mismatch{
					blockID:    b.ID,
					itemID:     item.ID,
					label:      label,
					cite:       fmt.Sprintf("%s:%d", strings.TrimSpace(cit.File), cit.Line),
					candidates: preEmitCandidateCitationLocationsForLabelWithContext(pctx, label, 4),
				})
				continue
			}
			evidence, found := pctx.citedEvidenceItems(cit)
			if !found {
				if candidates := preEmitCandidateCitationLocationsForAggregateItemWithContext(pctx, label, text, 4); len(candidates) > 0 {
					mismatches = append(mismatches, mismatch{
						blockID:    b.ID,
						itemID:     item.ID,
						label:      label,
						cite:       fmt.Sprintf("%s:%d", strings.TrimSpace(cit.File), cit.Line),
						candidates: candidates,
					})
				}
				continue
			}
			if preEmitDecoratedItemLabelMatchesAnyEvidenceEndpoint(label, text, evidence) ||
				preEmitLabelMatchesAnyEvidenceEndpoint(label, evidence) {
				continue
			}
			if candidates := preEmitCandidateCitationLocationsForAggregateItemWithContext(pctx, label, text, 4); len(candidates) > 0 {
				mismatches = append(mismatches, mismatch{
					blockID:    b.ID,
					itemID:     item.ID,
					label:      label,
					cite:       fmt.Sprintf("%s:%d", strings.TrimSpace(cit.File), cit.Line),
					candidates: candidates,
				})
				continue
			}
			mismatches = append(mismatches, mismatch{
				blockID:    b.ID,
				itemID:     item.ID,
				label:      label,
				cite:       fmt.Sprintf("%s:%d", strings.TrimSpace(cit.File), cit.Line),
				candidates: preEmitCandidateCitationLocationsForLabelWithContext(pctx, label, 4),
			})
		}
	}
	if len(mismatches) == 0 {
		return nil
	}
	parts := make([]string, 0, len(mismatches))
	for _, m := range mismatches {
		part := fmt.Sprintf("block=%q item=%q label=%q current_citation=%s", m.blockID, m.itemID, m.label, m.cite)
		if len(m.candidates) > 0 {
			part += " candidate_citations=[" + strings.Join(m.candidates, ", ") + "]"
		} else {
			part += " candidate_citations=[]"
		}
		parts = append(parts, part)
	}
	return []emitFixHint{{
		Field: "blocks[].items[].citation_ref",
		ExpectedShape: "each symbol-like item label must cite the evidence line whose subject/object/anchor names that same label; each source-location label must cite that exact file:line. current_citation is INVALID, not a target. Use a candidate_citations entry when present, or change the label to an endpoint actually present at current_citation: " +
			strings.Join(parts, "; "),
		Reason: "list item labels and citation_ref values must stay aligned; adjacent call-chain hops or nearby source locations cannot borrow each other's citations.",
	}}
}

func normalizeItemCitationRefsByUniqueLabelCitation(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctx *types.BusContext) int {
	return normalizeItemCitationRefsByUniqueLabelCitationWithContext(doc, view, ctx, newPreEmitCheckContext(ctx))
}

func normalizeItemCitationRefsByUniqueLabelCitationWithContext(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctx *types.BusContext, pctx *preEmitCheckContext) int {
	if doc == nil || ctx == nil || ctx.Mutable == nil {
		return 0
	}
	fixed := 0
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		switch block.Kind {
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		default:
			continue
		}
		if preEmitBlockUsesNonSymbolLabelSurface(*block, view) {
			continue
		}
		for ii := range block.Items {
			item := &block.Items[ii]
			label := strings.TrimSpace(item.Label)
			if label == "" || !preEmitLabelNeedsCitationAlignment(label) {
				continue
			}
			text := preEmitItemNonLabelSurface(*item)
			if item.CitationRef >= 0 && item.CitationRef < len(doc.Citations) &&
				preEmitItemCitationAlignedWithContext(pctx, label, text, doc.Citations[item.CitationRef]) {
				continue
			}
			match := preEmitUniqueCitationIndex(doc.Citations, item.CitationRef, func(cit types.Citation) bool {
				return preEmitItemCitationStrictlyAlignedWithContext(pctx, label, text, cit)
			})
			if match < 0 {
				match = preEmitUniqueCitationIndex(doc.Citations, item.CitationRef, func(cit types.Citation) bool {
					return preEmitItemCitationAlignedWithContext(pctx, label, text, cit)
				})
			}
			if match >= 0 {
				item.CitationRef = match
				fixed++
				continue
			}
			if cit, ok := preEmitPreferredCandidateCitationForItemWithContext(pctx, label, text); ok {
				item.CitationRef = appendOrReusePreEmitCitation(doc, cit)
				fixed++
			}
		}
	}
	return fixed
}

func normalizeOutOfRangeItemCitationRefsByEvidenceSurfaceWithContext(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctx *types.BusContext, pctx *preEmitCheckContext) int {
	if doc == nil || ctx == nil || ctx.Mutable == nil {
		return 0
	}
	if pctx == nil {
		pctx = newPreEmitCheckContext(ctx)
	}
	fixed := 0
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		switch block.Kind {
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		default:
			continue
		}
		if preEmitBlockUsesNonSymbolLabelSurface(*block, view) {
			continue
		}
		for ii := range block.Items {
			item := &block.Items[ii]
			if item.CitationRef < len(doc.Citations) {
				continue
			}
			label := strings.TrimSpace(item.Label)
			text := preEmitItemNonLabelSurface(*item)
			if label == "" && strings.TrimSpace(text) == "" {
				continue
			}
			if cit, ok := preEmitPreferredCandidateCitationForItemWithContext(pctx, label, text); ok {
				item.CitationRef = appendOrReusePreEmitCitation(doc, cit)
				fixed++
				continue
			}
			if cit, ok := preEmitUniqueEvidenceCitationForItemSurfaceWithContext(pctx, label, text); ok {
				item.CitationRef = appendOrReusePreEmitCitation(doc, cit)
				fixed++
			}
		}
	}
	return fixed
}

func preEmitUniqueEvidenceCitationForItemSurfaceWithContext(pctx *preEmitCheckContext, label, text string) (types.Citation, bool) {
	if pctx == nil || pctx.ctx == nil || pctx.ctx.Mutable == nil {
		return types.Citation{}, false
	}
	label = strings.TrimSpace(label)
	text = strings.TrimSpace(text)
	if label == "" && text == "" {
		return types.Citation{}, false
	}
	var out []types.Citation
	seen := make(map[string]bool)
	for _, ev := range pctx.evidenceItems() {
		if ev.GroundingStatus == types.GroundingUngrounded ||
			strings.TrimSpace(ev.Source) == "" ||
			ev.LineStart <= 0 {
			continue
		}
		if !preEmitItemSurfaceMentionsEvidence(label, text, ev) {
			continue
		}
		cit := pctx.canonicalCitation(types.Citation{
			File:          ev.Source,
			Line:          ev.LineStart,
			LineEnd:       ev.LineEnd,
			Scope:         ev.Scope,
			SectionPath:   ev.SectionPath,
			FileRoleLabel: ev.FileRoleLabel,
		})
		if cit.File == "" || cit.Line <= 0 {
			continue
		}
		key := preEmitCitationLocationKey(cit)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, cit)
	}
	if len(out) != 1 {
		return types.Citation{}, false
	}
	return out[0], true
}

func preEmitItemSurfaceMentionsEvidence(label, text string, ev types.EvidenceItem) bool {
	surface := strings.TrimSpace(strings.Join([]string{label, text}, "\n"))
	if surface == "" {
		return false
	}
	for _, endpoint := range []string{ev.Subject, ev.Object, ev.AnchorSymbol, ev.OwnerSymbol} {
		if preEmitCodeSurfaceAppearsVerbatim(endpoint, surface) {
			return true
		}
	}
	for _, term := range ev.SurfaceTerms {
		if preEmitCodeSurfaceAppearsVerbatim(term, surface) {
			return true
		}
	}
	if preEmitSourceSurfaceMatchesLabel(label, ev.Source) {
		return true
	}
	return false
}

func preEmitSourceSurfaceMatchesLabel(label, source string) bool {
	label = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(label, `\`, `/`)))
	source = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(source, `\`, `/`)))
	if label == "" || source == "" {
		return false
	}
	if source == label || strings.HasSuffix(source, "/"+label) {
		return true
	}
	base := filepath.Base(label)
	if base == "." || base == "/" || base == "" {
		return false
	}
	return strings.Contains(base, ".") && filepath.Base(source) == base
}

func preEmitUniqueCitationIndex(citations []types.Citation, exclude int, matches func(types.Citation) bool) int {
	if len(citations) == 0 || matches == nil {
		return -1
	}
	match := -1
	for i, cit := range citations {
		if i == exclude {
			continue
		}
		if !matches(cit) {
			continue
		}
		if match >= 0 {
			return -1
		}
		match = i
	}
	return match
}

func normalizeQualifiedItemLabelsByUniqueEnclosingFunction(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) int {
	if doc == nil || len(doc.Citations) == 0 {
		return 0
	}
	fixed := 0
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		switch block.Kind {
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		default:
			continue
		}
		if preEmitBlockUsesNonSymbolLabelSurface(*block, view) {
			continue
		}
		for ii := range block.Items {
			item := &block.Items[ii]
			label := strings.TrimSpace(item.Label)
			if label == "" || !preEmitLabelNeedsCitationAlignment(label) {
				continue
			}
			_, member, ok := preEmitQualifiedCodeSurfaceParts(label)
			if !ok {
				continue
			}
			candidateIndex, candidateLabel, ok := preEmitUniqueEnclosingFunctionForMemberMention(doc.Citations, member, preEmitItemNonLabelSurface(*item))
			if !ok || candidateLabel == "" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(candidateLabel), label) && item.CitationRef == candidateIndex {
				continue
			}
			item.Label = candidateLabel
			item.CitationRef = candidateIndex
			fixed++
		}
	}
	return fixed
}

func preEmitUniqueEnclosingFunctionForMemberMention(citations []types.Citation, member, text string) (int, string, bool) {
	memberKey := preEmitCodeIdentityKey(member)
	if memberKey == "" || strings.TrimSpace(text) == "" {
		return -1, "", false
	}
	type candidate struct {
		index int
		label string
	}
	var candidates []candidate
	seen := make(map[string]bool)
	for i, cit := range citations {
		fn := preEmitNormalizeCallableSurface(cit.EnclosingFunction)
		if fn == "" {
			continue
		}
		_, fnMember, ok := preEmitQualifiedCodeSurfaceParts(fn)
		if !ok || preEmitCodeIdentityKey(fnMember) != memberKey {
			continue
		}
		if !preEmitCodeSurfaceAppearsVerbatim(fn, text) {
			continue
		}
		key := strings.ToLower(fn)
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, candidate{index: i, label: fn})
	}
	if len(candidates) != 1 {
		return -1, "", false
	}
	return candidates[0].index, candidates[0].label, true
}

func preEmitUniqueCandidateCitationForItem(ctx *types.BusContext, label, text string) (types.Citation, bool) {
	return preEmitUniqueCandidateCitationForItemWithContext(newPreEmitCheckContext(ctx), label, text)
}

func preEmitUniqueCandidateCitationForItemWithContext(pctx *preEmitCheckContext, label, text string) (types.Citation, bool) {
	var out []types.Citation
	seen := make(map[string]bool)
	add := func(cit types.Citation) {
		cit = pctx.canonicalCitation(cit)
		if cit.File == "" || cit.Line <= 0 {
			return
		}
		key := preEmitCitationLocationKey(cit)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, cit)
	}
	for _, loc := range preEmitCandidateCitationLocationsForAggregateItemWithContext(pctx, label, text, 8) {
		if cit, ok := parsePreEmitCitationLocation(loc); ok {
			add(cit)
		}
	}
	for _, loc := range preEmitCandidateCitationLocationsForLabelWithContext(pctx, label, 8) {
		if cit, ok := parsePreEmitCitationLocation(loc); ok {
			add(cit)
		}
	}
	if len(out) != 1 {
		return types.Citation{}, false
	}
	return out[0], true
}

func preEmitPreferredCandidateCitationForItemWithContext(pctx *preEmitCheckContext, label, text string) (types.Citation, bool) {
	if cit, ok := preEmitUniqueCandidateCitationForItemWithContext(pctx, label, text); ok {
		return cit, true
	}
	for _, loc := range preEmitCandidateCitationLocationsForAggregateItemWithContext(pctx, label, text, 1) {
		if cit, ok := parsePreEmitCitationLocation(loc); ok {
			return cit, true
		}
	}
	for _, loc := range preEmitCandidateCitationLocationsForLabelWithContext(pctx, label, 1) {
		if cit, ok := parsePreEmitCitationLocation(loc); ok {
			return cit, true
		}
	}
	return types.Citation{}, false
}

func appendOrReusePreEmitCitation(doc *types.AnswerDocumentV2, cit types.Citation) int {
	if doc == nil {
		return -1
	}
	want := preEmitCitationLocationKey(cit)
	for i, existing := range doc.Citations {
		if preEmitCitationLocationKey(existing) == want || preEmitCitationSameLocation(existing, cit) {
			return i
		}
	}
	doc.Citations = append(doc.Citations, cit)
	return len(doc.Citations) - 1
}

func parsePreEmitCitationLocation(loc string) (types.Citation, bool) {
	loc = strings.TrimSpace(loc)
	idx := strings.LastIndex(loc, ":")
	if idx <= 0 || idx >= len(loc)-1 {
		return types.Citation{}, false
	}
	line, err := strconv.Atoi(strings.TrimSpace(loc[idx+1:]))
	if err != nil || line <= 0 {
		return types.Citation{}, false
	}
	file := strings.TrimSpace(loc[:idx])
	if file == "" {
		return types.Citation{}, false
	}
	return types.Citation{File: file, Line: line}, true
}

func preEmitItemCitationAligned(ctx *types.BusContext, label, text string, cit types.Citation) bool {
	return preEmitItemCitationAlignedWithContext(newPreEmitCheckContext(ctx), label, text, cit)
}

func preEmitItemCitationAlignedWithContext(pctx *preEmitCheckContext, label, text string, cit types.Citation) bool {
	if preEmitItemCitationStrictlyAlignedWithContext(pctx, label, text, cit) {
		return true
	}
	evidence, found := pctx.citedEvidenceItems(cit)
	return found && preEmitDecoratedItemLabelMatchesAnyEvidenceEndpoint(label, text, evidence)
}

func preEmitItemCitationStrictlyAlignedWithContext(pctx *preEmitCheckContext, label, text string, cit types.Citation) bool {
	cit = pctx.canonicalCitation(cit)
	if types.AnswerLocationLabelMatchesCitation(label, cit) {
		return true
	}
	if preEmitCitationEnclosingFunctionSupportsLabel(label, cit) {
		return true
	}
	if preEmitCitationEnclosingFunctionConflictsWithQualifiedLabel(label, cit) {
		return false
	}
	if preEmitCitationSupportsAggregateItemWithContext(pctx, label, text, cit) {
		return true
	}
	if surface, ok := types.ParseAnswerSourceLocationSurface(label); ok {
		return types.AnswerSourceLocationSurfaceMatchesCitation(surface, cit)
	}
	evidence, found := pctx.citedEvidenceItems(cit)
	return found && preEmitLabelMatchesAnyEvidenceEndpoint(label, evidence)
}

func preCheckCallChainItemCitationRoleAlignment(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctxOpt ...*types.BusContext) []emitFixHint {
	return preCheckCallChainItemCitationRoleAlignmentWithContext(doc, view, newPreEmitCheckContext(ctxOpt...))
}

func preCheckCallChainItemCitationRoleAlignmentWithContext(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, pctx *preEmitCheckContext) []emitFixHint {
	if doc == nil || pctx == nil || pctx.ctx == nil || pctx.ctx.Mutable == nil {
		return nil
	}
	ctx := pctx.ctx
	allEvidence := pctx.evidenceItems()
	if len(allEvidence) == 0 {
		return nil
	}
	type mismatch struct {
		blockID string
		itemID  string
		edge    string
		got     string
		want    string
	}
	var mismatches []mismatch
	for _, b := range doc.Blocks {
		forms := preEmitBlockCitationRoleForms(b, view)
		if len(forms) == 0 {
			continue
		}
		for _, item := range b.Items {
			if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
				continue
			}
			cit := doc.Citations[item.CitationRef]
			if preEmitItemMatchesSourceLocationPrincipalMember(ctx, item, cit) {
				continue
			}
			expected, ok := preEmitClaimRoleMentionedByItemSurface(item, forms, allEvidence)
			if !ok {
				continue
			}
			cited, found := pctx.citedEvidenceItems(cit)
			if found && types.EvidenceSetContainsSameClaimRole(cited, expected) {
				continue
			}
			mismatches = append(mismatches, mismatch{
				blockID: b.ID,
				itemID:  item.ID,
				edge:    types.EvidenceClaimRoleName(expected),
				got:     fmt.Sprintf("%s:%d", strings.TrimSpace(cit.File), cit.Line),
				want:    types.EvidenceClaimRoleLocation(expected),
			})
		}
	}
	if len(mismatches) == 0 {
		return nil
	}
	parts := make([]string, 0, len(mismatches))
	for _, m := range mismatches {
		parts = append(parts, fmt.Sprintf("block=%q item=%q asserts %s but cites %s; cite %s", m.blockID, m.itemID, m.edge, m.got, m.want))
	}
	return []emitFixHint{{
		Field: "blocks[].items[].citation_ref",
		ExpectedShape: "each item whose visible label/text/cells name a typed evidence role must cite the evidence line for that same role: " +
			strings.Join(parts, "; "),
		Reason: "item citations must support the typed role asserted by the item; definition lines or adjacent items cannot stand in for a different role named in the item surface.",
	}}
}

func preCheckPrincipalSupportMemberCoverage(doc *types.AnswerDocumentV2, ctxOpt ...*types.BusContext) []emitFixHint {
	if doc == nil || len(ctxOpt) == 0 || ctxOpt[0] == nil {
		return nil
	}
	supportPlan := types.BuildAnswerSupportPlanForBusContext(ctxOpt[0])
	if labelDrift := types.ChangeImpactFileOutputLabelDrift(doc, supportPlan); labelDrift != nil {
		return []emitFixHint{{
			Field: "blocks[].items[].label",
			ExpectedShape: "this change-impact answer requested files as the principal output, so each principal item label must be the file path, not a file:line site. Use exactly " +
				changeImpactFileLabelsForHint(labelDrift),
			Reason: "file:line anchors are supporting evidence for each file member; rendering sites as the principal labels changes the user's requested output surface and lets stale prose counts drift from the typed file set.",
		}}
	}
	if narrowing := types.ChangeImpactPrincipalNarrowing(doc, supportPlan); narrowing != nil {
		return []emitFixHint{{
			Field: "blocks[].items[]",
			ExpectedShape: "the current request is a change-impact affected-site question for " +
				changeImpactTargetForHint(narrowing) +
				"; principal list/table items cannot be narrowed to direct assignment evidence only. Include typed affected-site members for non-assignment roles already present in the support lane, citing matching file:line anchors: " +
				changeImpactMissingMembersForHint(narrowing),
			Reason: "direct assignments are only one affected-site role; the investigation emitted non-assignment affected sites as principal evidence, so the final answer must preserve them instead of shrinking the user's affected-site criterion.",
		}}
	}
	missing := types.MissingPrincipalSupportMembers(doc, supportPlan)
	if len(missing) == 0 {
		return nil
	}
	parts := make([]string, 0, len(missing))
	for _, m := range missing {
		label := strings.TrimSpace(m.Label)
		if label == "" {
			label = m.Location
		}
		parts = append(parts, fmt.Sprintf("%q at %s", label, m.LocationHint()))
		if len(parts) >= 6 {
			break
		}
	}
	return []emitFixHint{{
		Field: "blocks[].items[].citation_ref",
		ExpectedShape: "include one principal ordered_list / bullet_list / table item for each principal support evidence member, citing a matching typed file:line: " +
			strings.Join(parts, "; "),
		Reason: "the investigation already emitted these as answer-grade principal evidence; the final answer must preserve the members or add a cited caveat item for a real exclusion instead of relying on system-added caveats.",
	}}
}

func normalizePrincipalSupportMemberCarriers(doc *types.AnswerDocumentV2, supportPlan *types.AnswerSupportPlan) int {
	if doc == nil || supportPlan == nil {
		return 0
	}
	missing := types.MissingPrincipalSupportMembers(doc, supportPlan)
	if len(missing) == 0 {
		return 0
	}
	fixed := 0
	for _, ob := range missing {
		if !principalSupportMemberVisibleForCarrierNormalization(doc, ob) {
			continue
		}
		cit, ok := citationForPrincipalSupportMember(ob)
		if !ok {
			continue
		}
		ref := appendOrReusePreEmitCitation(doc, cit)
		if ref < 0 {
			continue
		}
		if normalizeExistingPrincipalSupportMemberItem(doc, ob, ref) {
			fixed++
			continue
		}
		if appendHiddenPrincipalSupportMemberItem(doc, ob, ref) {
			fixed++
		}
	}
	return fixed
}

func principalSupportMemberVisibleForCarrierNormalization(doc *types.AnswerDocumentV2, ob types.AnswerSupportMemberObligation) bool {
	if doc == nil {
		return false
	}
	for _, block := range doc.Blocks {
		if !preEmitBlockCanCarryPrincipalSupportMember(block) {
			continue
		}
		for _, item := range block.Items {
			if types.AnswerTextMentionsSupportMember(types.AnswerBlockItemVisibleSurface(item), ob) {
				return true
			}
		}
		if strings.TrimSpace(block.Text) != "" &&
			types.AnswerTextMentionsSupportMember(types.AnswerBlockVisibleSurface(block), ob) {
			return true
		}
	}
	return false
}

func normalizeAggregateMemberSetCarriers(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	// See normalizePrincipalSupportMemberCarriers: aggregate facts can
	// justify checks or caveats, but the system must not materialize a
	// new principal list/table on the model's behalf.
	return 0
}

func appendAggregateMemberSetCarrierBlock(doc *types.AnswerDocumentV2, factIdx int, label string) int {
	if doc == nil {
		return -1
	}
	title := strings.TrimSpace(label)
	if title == "" {
		title = "Principal member set"
	}
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:    nextAggregateMemberSetBlockID(doc, factIdx, title),
		Kind:  types.BlockOrderedList,
		Title: title,
	})
	return len(doc.Blocks) - 1
}

func nextAggregateMemberSetBlockID(doc *types.AnswerDocumentV2, factIdx int, label string) string {
	base := "aggregate-member-set"
	if suffix := sanitizeRequiredMechanismAnchorID(label); suffix != "" {
		base += "-" + suffix
	} else if factIdx >= 0 {
		base += fmt.Sprintf("-%d", factIdx+1)
	}
	used := make(map[string]bool, len(doc.Blocks))
	for _, block := range doc.Blocks {
		if id := strings.TrimSpace(block.ID); id != "" {
			used[id] = true
		}
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		id := fmt.Sprintf("%s-%d", base, i)
		if !used[id] {
			return id
		}
	}
}

func nextAggregateMemberSetItemID(block types.AnswerBlock, label string) string {
	base := "member-" + sanitizeRequiredMechanismAnchorID(label)
	if base == "member-" {
		base = "member"
	}
	used := make(map[string]bool, len(block.Items))
	for _, item := range block.Items {
		if id := strings.TrimSpace(item.ID); id != "" {
			used[id] = true
		}
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		id := fmt.Sprintf("%s-%d", base, i)
		if !used[id] {
			return id
		}
	}
}

func aggregateMemberSetCarrierLabel(member string) string {
	candidates := preEmitAggregateMemberDisplayCandidates(member)
	if len(candidates) > 0 {
		return strings.TrimSpace(candidates[0])
	}
	return strings.TrimSpace(member)
}

func aggregateMemberSetCarrierText(factLabel string) string {
	factLabel = strings.TrimSpace(factLabel)
	if factLabel == "" {
		return "来自已验收的调查成员清单。"
	}
	return "来自已验收的调查成员清单：" + factLabel + "。"
}

func citationForAggregateMemberSetMember(fact types.AnswerAggregateFact, memberIdx int, member string, ctx *types.BusContext) (types.Citation, bool) {
	member = strings.TrimSpace(member)
	if member == "" {
		return types.Citation{}, false
	}
	if _, location, ok := types.ParseAnswerSupportRefMemberLocation(member); ok && location.File != "" && location.LineStart > 0 {
		return types.Citation{File: location.File, Line: location.LineStart}, true
	}
	if location, ok := types.ParseAnswerSourceLocationSurface(member); ok && location.File != "" && location.LineStart > 0 {
		return types.Citation{File: location.File, Line: location.LineStart}, true
	}
	if cit, ok := citationForAggregateMemberSetSupportRef(fact, memberIdx, member); ok {
		return cit, true
	}
	if cit, ok := citationForAggregateMemberSetEvidence(member, ctx); ok {
		return cit, true
	}
	return types.Citation{}, false
}

func citationForAggregateMemberSetSupportRef(fact types.AnswerAggregateFact, memberIdx int, member string) (types.Citation, bool) {
	if len(fact.SupportRefs) == 0 {
		return types.Citation{}, false
	}
	memberKey := strings.ToLower(strings.TrimSpace(member))
	var bare []types.AnswerSourceLocationSurface
	var generic []types.AnswerSourceLocationSurface
	for _, ref := range fact.SupportRefs {
		refMember, location, ok := types.ParseAnswerSupportRefMemberLocation(ref)
		if !ok || location.File == "" || location.LineStart <= 0 {
			continue
		}
		if strings.TrimSpace(refMember) == "" {
			bare = append(bare, location)
			continue
		}
		if types.AnswerSupportRefLabelIsGeneric(refMember) {
			generic = append(generic, location)
			continue
		}
		if strings.EqualFold(strings.TrimSpace(refMember), memberKey) ||
			aggregateSupportRefLabelMatchesMember(refMember, member) {
			return types.Citation{File: location.File, Line: location.LineStart}, true
		}
	}
	if len(generic) == len(fact.Members) && memberIdx >= 0 && memberIdx < len(generic) {
		location := generic[memberIdx]
		return types.Citation{File: location.File, Line: location.LineStart}, true
	}
	if len(generic) == 1 && len(fact.Members) == 1 {
		location := generic[0]
		return types.Citation{File: location.File, Line: location.LineStart}, true
	}
	if len(bare) == len(fact.Members) && memberIdx >= 0 && memberIdx < len(bare) {
		location := bare[memberIdx]
		return types.Citation{File: location.File, Line: location.LineStart}, true
	}
	if len(bare) == 1 && len(fact.Members) == 1 {
		location := bare[0]
		return types.Citation{File: location.File, Line: location.LineStart}, true
	}
	return types.Citation{}, false
}

func aggregateSupportRefLabelMatchesMember(refMember, member string) bool {
	refMember = strings.TrimSpace(refMember)
	member = strings.TrimSpace(member)
	if refMember == "" || member == "" {
		return false
	}
	for _, candidate := range preEmitAggregateMemberDisplayCandidates(member) {
		if strings.EqualFold(refMember, candidate) {
			return true
		}
	}
	if tail := types.NormalizedSurfaceSymbolTail(member); tail != "" {
		return strings.EqualFold(types.NormalizedSurfaceSymbolTail(refMember), tail)
	}
	return false
}

func citationForAggregateMemberSetEvidence(member string, ctx *types.BusContext) (types.Citation, bool) {
	evidence := aggregateMemberSetEvidencePool(ctx)
	if len(evidence) == 0 {
		return types.Citation{}, false
	}
	if file := aggregateMemberSetMemberFilePrefix(member); file != "" {
		for _, ev := range evidence {
			if aggregateMemberSetEvidenceLocationUsable(ev) && aggregateMemberSetPathMatches(file, ev.Source) {
				return types.Citation{File: ev.Source, Line: ev.LineStart}, true
			}
		}
	}
	candidates := aggregateMemberSetEvidenceCandidates(member)
	for _, ev := range evidence {
		if !aggregateMemberSetEvidenceLocationUsable(ev) {
			continue
		}
		if aggregateMemberSetEvidenceMatchesAny(ev, candidates) {
			return types.Citation{File: ev.Source, Line: ev.LineStart}, true
		}
	}
	return types.Citation{}, false
}

func aggregateMemberSetEvidencePool(ctx *types.BusContext) []types.EvidenceItem {
	if ctx == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []types.EvidenceItem
	appendItems := func(items []types.EvidenceItem) {
		for _, item := range items {
			key := fmt.Sprintf("%s:%d:%s", strings.TrimSpace(item.Source), item.LineStart, strings.TrimSpace(item.ID))
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, item)
		}
	}
	if plan := types.BuildAnswerSurfacePlanForBusContext(ctx); plan != nil {
		appendItems(plan.SurfaceEvidence)
	}
	if ctx.Mutable != nil {
		appendItems(ctx.Mutable.EmittedEvidence())
	}
	appendItems(ctx.EvidenceItems)
	return out
}

func aggregateMemberSetEvidenceLocationUsable(ev types.EvidenceItem) bool {
	return strings.TrimSpace(ev.Source) != "" &&
		ev.LineStart > 0 &&
		ev.GroundingStatus != types.GroundingUngrounded
}

func aggregateMemberSetMemberFilePrefix(member string) string {
	member = strings.TrimSpace(member)
	if base, _, ok := types.AnswerAggregateDecoratedLabelParts(member); ok {
		base = strings.TrimSpace(base)
		if ext := filepath.Ext(base); base != "" && ext != "" &&
			(types.IsCodeOrConfigPathExtension(ext) || strings.Contains(base, "/")) {
			return strings.ReplaceAll(base, `\`, `/`)
		}
	}
	idx := strings.Index(member, ": ")
	if idx <= 0 {
		return ""
	}
	file := strings.TrimSpace(member[:idx])
	ext := filepath.Ext(file)
	if file == "" || ext == "" {
		return ""
	}
	if !types.IsCodeOrConfigPathExtension(ext) && !strings.Contains(file, "/") {
		return ""
	}
	return strings.ReplaceAll(file, `\`, `/`)
}

func aggregateMemberSetPathMatches(memberFile, evidenceFile string) bool {
	memberFile = strings.Trim(strings.TrimSpace(strings.ReplaceAll(memberFile, `\`, `/`)), "./")
	evidenceFile = strings.Trim(strings.TrimSpace(strings.ReplaceAll(evidenceFile, `\`, `/`)), "./")
	if memberFile == "" || evidenceFile == "" {
		return false
	}
	return memberFile == evidenceFile || strings.HasSuffix(evidenceFile, "/"+memberFile)
}

func aggregateMemberSetEvidenceCandidates(member string) []string {
	var out []string
	out = append(out, preEmitAggregateMemberDisplayCandidates(member)...)
	if idx := strings.Index(member, ": "); idx > 0 {
		right := strings.TrimSpace(member[idx+2:])
		if right != "" {
			out = append(out, right)
			if paren := strings.IndexAny(right, "（("); paren > 0 {
				out = append(out, strings.TrimSpace(right[:paren]))
			}
		}
	}
	if label, _, ok := types.ParseAnswerSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
		out = append(out, label)
	}
	if tail := types.NormalizedSurfaceSymbolTail(member); tail != "" {
		out = append(out, tail)
	}
	return dedupPreEmitStringCandidates(out)
}

func aggregateMemberSetEvidenceMatchesAny(ev types.EvidenceItem, candidates []string) bool {
	if len(candidates) == 0 {
		return false
	}
	hay := strings.Join([]string{
		ev.AnchorSymbol,
		ev.Subject,
		ev.Object,
		ev.OwnerSymbol,
		strings.Join(ev.SurfaceTerms, "\n"),
		ev.Snippet,
		ev.Summary,
	}, "\n")
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if types.AnswerCodeSurfaceAppearsInText(hay, candidate) || strings.EqualFold(candidate, strings.TrimSpace(ev.AnchorSymbol)) {
			return true
		}
	}
	return false
}

func principalSupportMemberCarrierBlockIndex(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return -1
	}
	for i, block := range doc.Blocks {
		if !preEmitBlockCanCarryPrincipalSupportMember(block) {
			continue
		}
		if block.SurfaceRole == types.SurfacePrincipal || len(block.Items) > 0 {
			return i
		}
	}
	for i, block := range doc.Blocks {
		if preEmitBlockCanCarryPrincipalSupportMember(block) {
			return i
		}
	}
	return -1
}

func appendPrincipalSupportMemberCarrierBlock(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return -1
	}
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:       nextPrincipalSupportMemberBlockID(doc),
		Kind:     types.BlockBulletList,
		FacetIDs: []string{string(types.FacetEnumerationItem)},
	})
	return len(doc.Blocks) - 1
}

func normalizeExistingPrincipalSupportMemberItem(doc *types.AnswerDocumentV2, ob types.AnswerSupportMemberObligation, citationRef int) bool {
	if doc == nil || citationRef < 0 {
		return false
	}
	for bi := range doc.Blocks {
		if !preEmitBlockCanCarryPrincipalSupportMember(doc.Blocks[bi]) {
			continue
		}
		for ii := range doc.Blocks[bi].Items {
			item := &doc.Blocks[bi].Items[ii]
			if !types.AnswerTextMentionsSupportMember(types.AnswerBlockItemVisibleSurface(*item), ob) {
				continue
			}
			changed := false
			if item.CitationRef != citationRef {
				item.CitationRef = citationRef
				changed = true
			}
			if strings.TrimSpace(item.ID) == "" {
				item.ID = nextPrincipalSupportMemberItemID(doc.Blocks[bi], ob)
				changed = true
			}
			return changed
		}
	}
	return false
}

func appendHiddenPrincipalSupportMemberItem(doc *types.AnswerDocumentV2, ob types.AnswerSupportMemberObligation, citationRef int) bool {
	if doc == nil || citationRef < 0 {
		return false
	}
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		if !preEmitBlockCanCarryPrincipalSupportMember(*block) {
			continue
		}
		// When a table/list block has an authored markdown/table text
		// surface, the renderer treats block.Text as authoritative and
		// does not render items[] as a second table. Adding an item here is
		// therefore a hidden citation sidecar for already-visible content,
		// not a new user-facing claim.
		if strings.TrimSpace(block.Text) == "" {
			continue
		}
		if !types.AnswerTextMentionsSupportMember(types.AnswerBlockVisibleSurface(*block), ob) {
			continue
		}
		block.Items = append(block.Items, types.AnswerBlockItem{
			ID:          nextPrincipalSupportMemberItemID(*block, ob),
			Label:       principalSupportMemberItemLabel(ob),
			CitationRef: citationRef,
		})
		return true
	}
	return false
}

func preEmitBlockCanCarryPrincipalSupportMember(block types.AnswerBlock) bool {
	switch block.Kind {
	case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		return true
	default:
		return false
	}
}

func citationForPrincipalSupportMember(ob types.AnswerSupportMemberObligation) (types.Citation, bool) {
	if source := strings.TrimSpace(ob.Source); source != "" && ob.LineStart > 0 {
		return types.Citation{File: source, Line: ob.LineStart}, true
	}
	for _, location := range append([]string{ob.Location}, ob.EquivalentLocations...) {
		if cit, ok := parsePreEmitCitationLocation(location); ok {
			return cit, true
		}
	}
	return types.Citation{}, false
}

func principalSupportMemberItemLabel(ob types.AnswerSupportMemberObligation) string {
	if label := strings.TrimSpace(ob.Label); label != "" {
		return label
	}
	if len(ob.SurfaceTerms) > 0 {
		if term := strings.TrimSpace(ob.SurfaceTerms[0]); term != "" {
			return term
		}
	}
	return strings.TrimSpace(ob.Location)
}

func principalSupportMemberItemText(ob types.AnswerSupportMemberObligation) string {
	return strings.TrimSpace(ob.LocationHint())
}

func nextPrincipalSupportMemberBlockID(doc *types.AnswerDocumentV2) string {
	base := "principal-support-members"
	used := make(map[string]bool, len(doc.Blocks))
	for _, block := range doc.Blocks {
		if id := strings.TrimSpace(block.ID); id != "" {
			used[id] = true
		}
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		id := fmt.Sprintf("%s-%d", base, i)
		if !used[id] {
			return id
		}
	}
}

func nextPrincipalSupportMemberItemID(block types.AnswerBlock, ob types.AnswerSupportMemberObligation) string {
	label := principalSupportMemberItemLabel(ob)
	if label == "" {
		label = ob.Location
	}
	base := "support-" + sanitizeRequiredMechanismAnchorID(label)
	if base == "support-" {
		base = "support-member"
	}
	used := make(map[string]bool, len(block.Items))
	for _, item := range block.Items {
		if id := strings.TrimSpace(item.ID); id != "" {
			used[id] = true
		}
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		id := fmt.Sprintf("%s-%d", base, i)
		if !used[id] {
			return id
		}
	}
}

// preCheckInactiveScopeDisclosure observes the same typed inactive-scope
// obligation as the orchestrator but does not reject. The boundary is a
// system/runtime fact, not a model-authored answer claim; when missing, the
// orchestrator appends a system-channel supplemental note after the answer.
func preCheckInactiveScopeDisclosure(doc *types.AnswerDocumentV2, ctxOpt ...*types.BusContext) []emitFixHint {
	if doc == nil || len(ctxOpt) == 0 || ctxOpt[0] == nil {
		return nil
	}
	obligation := types.BuildInactiveScopeDisclosureObligationFromBus(ctxOpt[0], doc)
	if !obligation.Active() {
		return nil
	}
	if types.AnswerDocumentDisclosesInactiveScope(doc, obligation) {
		return nil
	}
	logging.Info("[emit_answer_document] inactive-scope disclosure deferred to system caveat: pending=%q reason=%s",
		strings.Join(obligation.PendingRootRels, ","), obligation.Reason)
	return nil
}

func preCheckAggregateScalarValueCoverage(doc *types.AnswerDocumentV2, ctxOpt ...*types.BusContext) []emitFixHint {
	if doc == nil || len(ctxOpt) == 0 || ctxOpt[0] == nil || ctxOpt[0].Mutable == nil {
		return nil
	}
	facts := ctxOpt[0].Mutable.StableInvestigationAggregateFacts()
	if len(facts) == 0 {
		return nil
	}
	surface := preEmitVisibleAnswerSurface(doc)
	if strings.TrimSpace(surface) == "" {
		return nil
	}
	type missingScalar struct {
		label string
		value string
	}
	var missing []missingScalar
	seen := make(map[string]bool)
	for _, fact := range facts {
		if fact.Kind != types.AnswerAggregateScalar {
			continue
		}
		value := strings.TrimSpace(fact.Value)
		if value == "" || preEmitAggregateScalarValueAppears(value, surface) {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		missing = append(missing, missingScalar{
			label: strings.TrimSpace(fact.Label),
			value: value,
		})
	}
	if len(missing) == 0 {
		return nil
	}
	parts := make([]string, 0, len(missing))
	for _, m := range missing {
		if m.label != "" {
			parts = append(parts, fmt.Sprintf("label=%q value=%q", m.label, m.value))
		} else {
			parts = append(parts, fmt.Sprintf("value=%q", m.value))
		}
	}
	return []emitFixHint{{
		Field: "blocks[].text OR blocks[].items[].label/text/cells",
		ExpectedShape: "include every model-emitted scalar_value aggregate in the visible answer surface: " +
			strings.Join(parts, "; "),
		Reason: "the investigation already handed these scalar values to the final answer as structured data; leaving them only in thinking, citations, or closure notes drops the user's requested literal.",
	}}
}

func preCheckAggregateMemberSetCoverage(doc *types.AnswerDocumentV2, ctxOpt ...*types.BusContext) []emitFixHint {
	if doc == nil || len(ctxOpt) == 0 || ctxOpt[0] == nil || ctxOpt[0].Mutable == nil {
		return nil
	}
	ctx := ctxOpt[0]
	facts := ctx.Mutable.StableInvestigationAggregateFacts()
	if len(facts) == 0 {
		return nil
	}
	principalRefs := preEmitPrincipalAggregateMemberSetFactRefs(ctx, facts)
	if len(principalRefs) == 0 {
		return nil
	}
	surface := preEmitVisibleAnswerSurface(doc)
	if strings.TrimSpace(surface) == "" {
		return nil
	}
	type missingMember struct {
		label  string
		member string
	}
	var missing []missingMember
	seen := make(map[string]bool)
	for _, ref := range principalRefs {
		fact := ref.Fact
		if preEmitAggregateMemberSetIsScalarCountSupport(ctx, fact) {
			continue
		}
		for _, member := range fact.Members {
			if preEmitAggregateMemberAppearsInDocument(member, doc, surface) {
				continue
			}
			candidates := preEmitAggregateMemberDisplayCandidates(member)
			if len(candidates) == 0 {
				continue
			}
			key := strings.ToLower(candidates[0])
			if seen[key] {
				continue
			}
			seen[key] = true
			missing = append(missing, missingMember{
				label:  strings.TrimSpace(fact.Label),
				member: candidates[0],
			})
		}
	}
	if len(missing) == 0 {
		return nil
	}
	parts := make([]string, 0, len(missing))
	for _, m := range missing {
		if m.label != "" {
			parts = append(parts, fmt.Sprintf("label=%q member=%q", m.label, m.member))
		} else {
			parts = append(parts, fmt.Sprintf("member=%q", m.member))
		}
		if len(parts) >= 12 {
			break
		}
	}
	if len(missing) > len(parts) {
		parts = append(parts, fmt.Sprintf("... %d more omitted member(s)", len(missing)-len(parts)))
	}
	return []emitFixHint{{
		Field: "blocks[].items[].label/text/cells OR blocks[].text",
		ExpectedShape: "include every model-emitted principal member_set member in the visible answer: " +
			strings.Join(parts, "; "),
		Reason: "the investigation handed off this complete principal member set as structured data; finalization must preserve those model-authored members even when the request family was routed as architecture, scalar, relation, or generic prose.",
	}}
}

func preEmitAggregateMemberSetIsScalarCountSupport(ctx *types.BusContext, fact types.AnswerAggregateFact) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	return types.AggregateMemberSetIsScalarCountSupport(&rm, fact)
}

func preEmitAggregateMemberSetCoverageHardGate(ctxOpt ...*types.BusContext) bool {
	if len(ctxOpt) == 0 || ctxOpt[0] == nil || ctxOpt[0].AnalysisIR == nil {
		return true
	}
	rm := ctxOpt[0].AnalysisIR.RequestModel
	return types.RequiresExhaustiveEnumerationMemberSetHandoff(rm) ||
		types.RequiresRelationMemberSetHandoff(rm)
}

func preEmitPrincipalAggregateMemberSetFactRefs(ctx *types.BusContext, facts []types.AnswerAggregateFact) []types.AnswerAggregateFactRef {
	if ctx == nil || ctx.AnalysisIR == nil {
		return types.PrincipalAggregateMemberSetFactRefs(facts)
	}
	return types.PrincipalAggregateMemberSetFactRefsForRequest(facts, &ctx.AnalysisIR.RequestModel)
}

func preEmitPrincipalRelationMemberSetFactRefs(ctx *types.BusContext, facts []types.AnswerAggregateFact) []types.AnswerAggregateFactRef {
	if ctx == nil || ctx.AnalysisIR == nil {
		return types.PrincipalRelationMemberSetFactRefs(facts)
	}
	return types.PrincipalRelationMemberSetFactRefsForRequest(facts, &ctx.AnalysisIR.RequestModel)
}

func preCheckAggregateCardinalityConsistency(doc *types.AnswerDocumentV2, ctxOpt ...*types.BusContext) []emitFixHint {
	if doc == nil || len(ctxOpt) == 0 || ctxOpt[0] == nil || ctxOpt[0].Mutable == nil {
		return nil
	}
	ctx := ctxOpt[0]
	refs := preEmitAggregateCardinalityFactRefs(ctx, ctx.Mutable.StableInvestigationAggregateFacts())
	if len(refs) == 0 {
		return nil
	}
	type mismatch struct {
		label    string
		expected int
		got      int
		blockID  string
	}
	var mismatches []mismatch
	for _, ref := range refs {
		fact := ref.Fact
		expected := len(fact.Members)
		if expected == 0 {
			continue
		}
		if declared, ok := preEmitParseAggregateFactCount(fact.Value); ok && declared != expected {
			mismatches = append(mismatches, mismatch{
				label:    strings.TrimSpace(fact.Label),
				expected: expected,
				got:      declared,
				blockID:  fmt.Sprintf("aggregate_facts[%d].value", ref.Index),
			})
			continue
		}
		for _, claim := range preEmitAggregateScopedCountClaims(doc, fact, ref.MemberBindingMin) {
			if claim.value == expected {
				continue
			}
			mismatches = append(mismatches, mismatch{
				label:    strings.TrimSpace(fact.Label),
				expected: expected,
				got:      claim.value,
				blockID:  claim.blockID,
			})
		}
	}
	if len(mismatches) == 0 {
		return nil
	}
	parts := make([]string, 0, len(mismatches))
	seen := map[string]bool{}
	for _, m := range mismatches {
		key := fmt.Sprintf("%s\x00%d\x00%d\x00%s", strings.ToLower(m.label), m.expected, m.got, m.blockID)
		if seen[key] {
			continue
		}
		seen[key] = true
		label := m.label
		if label == "" {
			label = "principal member_set"
		}
		parts = append(parts, fmt.Sprintf("label=%q expected_count=%d visible_count=%d surface=%s", label, m.expected, m.got, m.blockID))
		if len(parts) >= 8 {
			break
		}
	}
	return []emitFixHint{{
		Field: "blocks[].text/count claims",
		ExpectedShape: "make every visible count claim for a model-emitted aggregate member list equal the aggregate cardinality: " +
			strings.Join(parts, "; "),
		Reason: "aggregate_facts carries the authoritative model-authored member cardinality; final text may display it, but it must not introduce a different count for that same set.",
	}}
}

type preEmitAggregateCardinalityRef struct {
	Index            int
	Fact             types.AnswerAggregateFact
	MemberBindingMin int
}

func preEmitAggregateCardinalityFactRefs(ctx *types.BusContext, facts []types.AnswerAggregateFact) []preEmitAggregateCardinalityRef {
	if len(facts) == 0 {
		return nil
	}
	principalRefs := preEmitPrincipalAggregateMemberSetFactRefs(ctx, facts)
	out := make([]preEmitAggregateCardinalityRef, 0, len(principalRefs))
	principalByIndex := make(map[int]bool, len(principalRefs))
	uniquePrincipalSet := len(principalRefs) == 1
	for _, ref := range principalRefs {
		principalByIndex[ref.Index] = true
		min := 0
		if uniquePrincipalSet {
			min = 1
		}
		out = append(out, preEmitAggregateCardinalityRef{
			Index:            ref.Index,
			Fact:             ref.Fact,
			MemberBindingMin: min,
		})
	}
	if ctx == nil || ctx.AnalysisIR == nil {
		return out
	}
	rm := ctx.AnalysisIR.RequestModel
	for idx, fact := range facts {
		if principalByIndex[idx] || !types.AggregateMemberSetIsScalarCountSupport(&rm, fact) {
			continue
		}
		out = append(out, preEmitAggregateCardinalityRef{
			Index:            idx,
			Fact:             fact,
			MemberBindingMin: 0,
		})
	}
	for _, ref := range types.PrincipalAggregateMemberSetFactRefs(facts) {
		if principalByIndex[ref.Index] || !preEmitNarrativeAggregateCountCanBindByMembers(ref.Fact) {
			continue
		}
		min := 2
		if len(ref.Fact.Members) < min {
			min = len(ref.Fact.Members)
		}
		out = append(out, preEmitAggregateCardinalityRef{
			Index:            ref.Index,
			Fact:             ref.Fact,
			MemberBindingMin: min,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func preEmitNarrativeAggregateCountCanBindByMembers(fact types.AnswerAggregateFact) bool {
	switch fact.Kind {
	case types.AnswerAggregateGroupedCount, types.AnswerAggregateBucketCount:
	default:
		return false
	}
	if len(fact.Members) == 0 {
		return false
	}
	for _, member := range fact.Members {
		member = strings.TrimSpace(member)
		if member == "" {
			return false
		}
		if _, ok := types.ParseAnswerFilePathSurface(member); ok {
			continue
		}
		if _, ok := types.ParseAnswerSourceLocationSurface(member); ok {
			continue
		}
		return true
	}
	return false
}

func preCheckRelationMemberSetAnswerShape(doc *types.AnswerDocumentV2, ctxOpt ...*types.BusContext) []emitFixHint {
	if doc == nil || len(ctxOpt) == 0 || ctxOpt[0] == nil || ctxOpt[0].Mutable == nil {
		return nil
	}
	ctx := ctxOpt[0]
	refs := preEmitPrincipalRelationMemberSetFactRefs(ctx, ctx.Mutable.StableInvestigationAggregateFacts())
	if len(refs) == 0 {
		return nil
	}
	var missing []string
	for _, ref := range refs {
		fact := ref.Fact
		if len(fact.Members) <= 1 {
			continue
		}
		if preEmitStructuredMemberBlockCoversFact(doc, fact) {
			continue
		}
		label := strings.TrimSpace(fact.Label)
		if label == "" {
			label = fmt.Sprintf("aggregate_facts[%d]", ref.Index)
		}
		missing = append(missing, fmt.Sprintf("label=%q members=%d", label, len(fact.Members)))
	}
	if len(missing) == 0 {
		return nil
	}
	return []emitFixHint{{
		Field: "blocks[kind=ordered_list|bullet_list|table].items[]",
		ExpectedShape: "render every multi-member relation principal member_set as direct list/table rows before mechanism explanation: " +
			strings.Join(missing, "; "),
		Reason: "relation lookups ask for qualifying members plus proof; a mechanism-only paragraph or compressed prose can hide members and recreate the original off-topic architecture answer.",
	}}
}

type preEmitAggregateCountClaim struct {
	value   int
	blockID string
}

func preEmitParseAggregateFactCount(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func preEmitAggregateScopedCountClaims(doc *types.AnswerDocumentV2, fact types.AnswerAggregateFact, memberBindingMin int) []preEmitAggregateCountClaim {
	if doc == nil {
		return nil
	}
	expected := len(fact.Members)
	if expected == 0 {
		return nil
	}
	var out []preEmitAggregateCountClaim
	for _, block := range doc.Blocks {
		surface := strings.TrimSpace(block.Title + "\n" + block.Text)
		if surface == "" {
			continue
		}
		if !preEmitBlockBindsToAggregateCount(block, surface, fact, memberBindingMin) {
			continue
		}
		for _, value := range preEmitScopedCountValues(surface, expected) {
			out = append(out, preEmitAggregateCountClaim{
				value:   value,
				blockID: preEmitBlockCountSurfaceID(block),
			})
		}
	}
	return out
}

func preEmitBlockBindsToAggregateCount(block types.AnswerBlock, surface string, fact types.AnswerAggregateFact, memberBindingMin int) bool {
	if preEmitAggregateDisplayPartAppears(strings.TrimSpace(fact.Label), surface) {
		return true
	}
	if memberBindingMin > 0 {
		matched := 0
		for _, member := range fact.Members {
			if preEmitAggregateMemberAppearsInText(member, surface) {
				matched++
				if matched >= memberBindingMin {
					return true
				}
			}
		}
	}
	switch block.Kind {
	case types.BlockScalar:
		return true
	default:
		return false
	}
}

func preEmitBlockCountSurfaceID(block types.AnswerBlock) string {
	id := strings.TrimSpace(block.ID)
	if id == "" {
		id = string(block.Kind)
	}
	return fmt.Sprintf("blocks[id=%q kind=%q]", id, block.Kind)
}

func preEmitScopedCountValues(surface string, expected int) []int {
	values := preEmitCountLikeIntegers(surface)
	if len(values) == 0 {
		return nil
	}
	if len(values) == 1 {
		return values
	}
	for _, value := range values {
		if value == expected {
			return nil
		}
	}
	return []int{values[0]}
}

func preEmitCountLikeIntegers(surface string) []int {
	var out []int
	for i := 0; i < len(surface); {
		if surface[i] < '0' || surface[i] > '9' {
			i++
			continue
		}
		start := i
		for i < len(surface) && surface[i] >= '0' && surface[i] <= '9' {
			i++
		}
		end := i
		if preEmitIntegerLooksLikeSourceOrIdentifier(surface, start, end) {
			continue
		}
		value, err := strconv.Atoi(surface[start:end])
		if err != nil {
			continue
		}
		out = append(out, value)
	}
	out = append(out, preEmitChineseCountLikeIntegers(surface)...)
	return out
}

func preEmitChineseCountLikeIntegers(surface string) []int {
	runes := []rune(surface)
	var out []int
	for i := 0; i < len(runes); {
		if !preEmitIsChineseCountRune(runes[i]) {
			i++
			continue
		}
		start := i
		for i < len(runes) && preEmitIsChineseCountRune(runes[i]) {
			i++
		}
		end := i
		j := end
		for j < len(runes) && unicode.IsSpace(runes[j]) {
			j++
		}
		if j >= len(runes) || !preEmitIsChineseCountClassifier(runes[j]) {
			continue
		}
		if start > 0 && (preEmitIsASCIIAlnum(runes[start-1]) || runes[start-1] == '_' || runes[start-1] == '/' || runes[start-1] == '.') {
			continue
		}
		value, ok := preEmitParseChineseCountToken(string(runes[start:end]))
		if !ok {
			continue
		}
		out = append(out, value)
	}
	return out
}

func preEmitIsChineseCountRune(r rune) bool {
	switch r {
	case '零', '〇', '一', '二', '两', '三', '四', '五', '六', '七', '八', '九', '十':
		return true
	default:
		return false
	}
}

func preEmitIsChineseCountClassifier(r rune) bool {
	switch r {
	case '层', '个', '项', '类', '种', '条', '组', '段', '轮', '次', '处', '份':
		return true
	default:
		return false
	}
}

func preEmitParseChineseCountToken(token string) (int, bool) {
	runes := []rune(strings.TrimSpace(token))
	if len(runes) == 0 || len(runes) > 3 {
		return 0, false
	}
	if len(runes) == 1 {
		return preEmitChineseDigitValue(runes[0])
	}
	tenAt := -1
	for i, r := range runes {
		if r == '十' {
			tenAt = i
			break
		}
	}
	if tenAt < 0 {
		return 0, false
	}
	left := 1
	if tenAt > 0 {
		var ok bool
		left, ok = preEmitChineseDigitValue(runes[tenAt-1])
		if !ok || left == 0 {
			return 0, false
		}
	}
	right := 0
	if tenAt+1 < len(runes) {
		var ok bool
		right, ok = preEmitChineseDigitValue(runes[tenAt+1])
		if !ok {
			return 0, false
		}
	}
	return left*10 + right, true
}

func preEmitChineseDigitValue(r rune) (int, bool) {
	switch r {
	case '零', '〇':
		return 0, true
	case '一':
		return 1, true
	case '二', '两':
		return 2, true
	case '三':
		return 3, true
	case '四':
		return 4, true
	case '五':
		return 5, true
	case '六':
		return 6, true
	case '七':
		return 7, true
	case '八':
		return 8, true
	case '九':
		return 9, true
	case '十':
		return 10, true
	default:
		return 0, false
	}
}

func preEmitIntegerLooksLikeSourceOrIdentifier(surface string, start, end int) bool {
	if start > 0 {
		prev := surface[start-1]
		if prev == ':' || prev == '/' || prev == '\\' || prev == '.' || prev == '-' || prev == '#' ||
			prev == '_' || prev >= 'A' && prev <= 'Z' || prev >= 'a' && prev <= 'z' {
			return true
		}
	}
	if end < len(surface) {
		next := surface[end]
		if next == ':' || next == '/' || next == '\\' || next == '.' || next == '-' ||
			next == '_' || next >= 'A' && next <= 'Z' || next >= 'a' && next <= 'z' {
			return true
		}
	}
	if strings.HasPrefix(surface[end:], "行") {
		return true
	}
	prefix := strings.ToLower(strings.TrimSpace(surface[:start]))
	last := ""
	for i := len(prefix) - 1; i >= 0; i-- {
		ch := prefix[i]
		if ch >= 'a' && ch <= 'z' {
			continue
		}
		last = prefix[i+1:]
		break
	}
	if last == "" {
		last = prefix
	}
	switch last {
	case "line", "lines", "ln", "l":
		return true
	default:
		return false
	}
}

func preEmitStructuredMemberBlockCoversFact(doc *types.AnswerDocumentV2, fact types.AnswerAggregateFact) bool {
	if doc == nil || len(fact.Members) == 0 {
		return false
	}
	for _, block := range doc.Blocks {
		switch block.Kind {
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		default:
			continue
		}
		matched := make(map[int]bool, len(fact.Members))
		for idx, member := range fact.Members {
			if preEmitStructuredBlockCoversAggregateMember(block, member) {
				matched[idx] = true
			}
		}
		if len(matched) == len(fact.Members) {
			return true
		}
	}
	return false
}

func preEmitStructuredBlockCoversAggregateMember(block types.AnswerBlock, member string) bool {
	for _, item := range block.Items {
		itemSurface := types.AnswerBlockItemVisibleSurface(item)
		if itemSurface == "" {
			continue
		}
		if preEmitAggregateMemberAppearsInText(member, itemSurface) {
			return true
		}
	}
	if block.Kind == types.BlockTable && strings.TrimSpace(block.Text) != "" &&
		preEmitAggregateMemberAppearsInText(member, block.Text) {
		return true
	}
	return preEmitMultiTargetRelationCoveredByStructuredBlock(member, block)
}

func preEmitAggregateMemberAppearsInText(member string, surface string) bool {
	candidates := preEmitAggregateMemberDisplayCandidates(member)
	if len(candidates) == 0 {
		return true
	}
	if preEmitDecoratedAggregateMemberAppearsInText(member, surface) {
		return true
	}
	if preEmitAnyAggregateMemberAppears(candidates, surface) {
		return true
	}
	for _, relationSurface := range preEmitAggregateMemberRelationSurfaces(member) {
		left, right, ok := types.AnswerAggregateMemberRelationParts(relationSurface)
		if !ok {
			continue
		}
		if preEmitTextContainsAllAggregateParts(surface, left, right) {
			return true
		}
	}
	if preEmitMultiTargetRelationAppearsInText(member, surface) {
		return true
	}
	return false
}

func preEmitAggregateMemberAppearsInDocument(member string, doc *types.AnswerDocumentV2, surface string) bool {
	candidates := preEmitAggregateMemberDisplayCandidates(member)
	if len(candidates) == 0 {
		return true
	}
	if preEmitDecoratedAggregateMemberAppearsInText(member, surface) {
		return true
	}
	if preEmitAnyAggregateMemberAppears(candidates, surface) {
		return true
	}
	for _, relationSurface := range preEmitAggregateMemberRelationSurfaces(member) {
		left, right, ok := types.AnswerAggregateMemberRelationParts(relationSurface)
		if !ok {
			continue
		}
		if preEmitRelationPartsAppearInSameAnswerUnit(left, right, doc) {
			return true
		}
	}
	if preEmitMultiTargetRelationAppearsInStructuredDocument(member, doc) {
		return true
	}
	return false
}

func preEmitMultiTargetRelationAppearsInStructuredDocument(member string, doc *types.AnswerDocumentV2) bool {
	if doc == nil {
		return false
	}
	for _, block := range doc.Blocks {
		switch block.Kind {
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		default:
			continue
		}
		if preEmitMultiTargetRelationCoveredByStructuredBlock(member, block) {
			return true
		}
	}
	return false
}

func preEmitMultiTargetRelationCoveredByStructuredBlock(member string, block types.AnswerBlock) bool {
	left, targets, ok := preEmitAggregateMemberMultiTargetRelationParts(member)
	if !ok {
		return false
	}
	for _, target := range targets {
		covered := false
		for _, item := range block.Items {
			itemSurface := types.AnswerBlockItemVisibleSurface(item)
			if preEmitTextContainsAllAggregateParts(itemSurface, left, target) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func preEmitMultiTargetRelationAppearsInText(member string, surface string) bool {
	left, targets, ok := preEmitAggregateMemberMultiTargetRelationParts(member)
	if !ok {
		return false
	}
	if !preEmitAggregateDisplayPartAppears(left, surface) {
		return false
	}
	for _, target := range targets {
		if !preEmitAggregateDisplayPartAppears(target, surface) {
			return false
		}
	}
	return true
}

func preEmitAggregateMemberMultiTargetRelationParts(member string) (left string, targets []string, ok bool) {
	for _, surface := range preEmitAggregateMemberRelationSurfaces(member) {
		left, targets, ok = preEmitMultiTargetRelationPartsFromSurface(surface)
		if ok {
			return left, targets, true
		}
	}
	return "", nil, false
}

func preEmitMultiTargetRelationPartsFromSurface(surface string) (left string, targets []string, ok bool) {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return "", nil, false
	}
	if _, _, parsed := types.AnswerAggregateMemberRelationParts(surface); parsed {
		return "", nil, false
	}
	left, right, ok := preEmitRawRelationSurfaceSplit(surface)
	if !ok {
		return "", nil, false
	}
	rawTargets := preEmitSplitMultiRelationTargets(right)
	if len(rawTargets) < 2 {
		return "", nil, false
	}
	for _, target := range rawTargets {
		target = strings.Trim(target, "` ")
		if target == "" {
			continue
		}
		_, _, parsed := types.AnswerAggregateMemberRelationParts(left + " → " + target)
		if !parsed {
			return "", nil, false
		}
		targets = append(targets, target)
	}
	if len(targets) < 2 {
		return "", nil, false
	}
	return left, dedupPreEmitStringCandidates(targets), true
}

func preEmitRawRelationSurfaceSplit(surface string) (left, right string, ok bool) {
	for _, sep := range []string{"→", "->", "=>"} {
		if strings.Count(surface, sep) != 1 {
			continue
		}
		idx := strings.Index(surface, sep)
		left = strings.TrimSpace(surface[:idx])
		right = strings.TrimSpace(surface[idx+len(sep):])
		if left != "" && right != "" {
			return left, right, true
		}
	}
	if strings.Count(surface, ":") == 1 {
		idx := strings.Index(surface, ":")
		if idx > 0 && idx < len(surface)-1 {
			before := rune(surface[idx-1])
			after := rune(surface[idx+1])
			if unicode.IsSpace(before) || unicode.IsSpace(after) {
				left = strings.TrimSpace(surface[:idx])
				right = strings.TrimSpace(surface[idx+1:])
				if left != "" && right != "" {
					return left, right, true
				}
			}
		}
	}
	return "", "", false
}

func preEmitSplitMultiRelationTargets(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	replacer := strings.NewReplacer(
		"+", "\x00",
		"&", "\x00",
		",", "\x00",
		"，", "\x00",
		"、", "\x00",
		"/", "\x00",
		" 和 ", "\x00",
		" 与 ", "\x00",
		" and ", "\x00",
		" AND ", "\x00",
	)
	parts := strings.Split(replacer.Replace(raw), "\x00")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func preEmitAggregateMemberRelationSurfaces(member string) []string {
	member = strings.TrimSpace(member)
	if member == "" {
		return nil
	}
	var out []string
	add := func(surface string) {
		surface = strings.TrimSpace(surface)
		if surface == "" {
			return
		}
		out = append(out, surface)
		for _, candidate := range types.AnswerAggregateMemberDisplayCandidates(surface) {
			out = append(out, candidate)
		}
	}
	add(member)
	if label, _, ok := types.ParseAnswerSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
		add(label)
	}
	for _, sep := range []string{" @ ", "\t", " | "} {
		if idx := strings.Index(member, sep); idx > 0 {
			prefix := strings.TrimSpace(member[:idx])
			if prefix != "" {
				add(prefix)
			}
		}
	}
	return dedupPreEmitStringCandidates(out)
}

func preEmitRelationPartsAppearInSameAnswerUnit(left, right string, doc *types.AnswerDocumentV2) bool {
	if doc == nil || strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	for _, block := range doc.Blocks {
		if preEmitTextContainsAllAggregateParts(block.Title+"\n"+block.Text, left, right) {
			return true
		}
		for _, item := range block.Items {
			if preEmitTextContainsAllAggregateParts(types.AnswerBlockItemVisibleSurface(item), left, right) {
				return true
			}
		}
		if block.Diagram != nil && preEmitTextContainsAllAggregateParts(block.Diagram.Body, left, right) {
			return true
		}
	}
	for _, caveat := range doc.Caveats {
		if preEmitTextContainsAllAggregateParts(caveat, left, right) {
			return true
		}
	}
	return false
}

func preEmitTextContainsAllAggregateParts(text, left, right string) bool {
	return preEmitAggregateDisplayPartAppears(left, text) &&
		preEmitAggregateDisplayPartAppears(right, text)
}

func preEmitVisibleAnswerSurface(doc *types.AnswerDocumentV2) string {
	if doc == nil {
		return ""
	}
	var b strings.Builder
	for _, block := range doc.Blocks {
		appendPreEmitSurface(&b, types.AnswerBlockVisibleSurface(block))
	}
	for _, caveat := range doc.Caveats {
		appendPreEmitSurface(&b, caveat)
	}
	return b.String()
}

func preEmitAggregateMemberDisplayCandidates(member string) []string {
	member = strings.TrimSpace(member)
	if member == "" {
		return nil
	}
	out := types.AnswerAggregateMemberDisplayCandidates(member)
	if len(out) == 0 {
		out = []string{member}
	}
	if label, _, ok := types.ParseAnswerSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
		out = append(out, types.AnswerAggregateMemberDisplayCandidates(label)...)
	}
	for _, sep := range []string{" @ ", "\t", " | "} {
		if idx := strings.Index(member, sep); idx > 0 {
			prefix := strings.TrimSpace(member[:idx])
			if prefix != "" {
				out = append(out, types.AnswerAggregateMemberDisplayCandidates(prefix)...)
			}
		}
	}
	return dedupPreEmitStringCandidates(out)
}

func dedupPreEmitStringCandidates(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

func preEmitAnyAggregateMemberAppears(candidates []string, surface string) bool {
	for _, candidate := range candidates {
		if preEmitAggregateScalarValueAppears(candidate, surface) {
			return true
		}
	}
	return false
}

func appendPreEmitSurface(b *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	b.WriteString(text)
}

func preEmitAggregateScalarValueAppears(value, surface string) bool {
	value = strings.TrimSpace(value)
	surface = strings.TrimSpace(surface)
	if value == "" || surface == "" {
		return false
	}
	if types.CodeSurfaceAppearsAsToken(value, surface) {
		return true
	}
	return preEmitDisplaySurfaceAppears(value, surface)
}

func preEmitAggregateDisplayPartAppears(value, surface string) bool {
	if preEmitAggregateScalarValueAppears(value, surface) {
		return true
	}
	return preEmitDisplaySurfaceAppearsFold(value, surface)
}

func preEmitDisplaySurfaceAppears(value, surface string) bool {
	if preEmitDisplaySurfaceAppearsStrict(value, surface) {
		return true
	}
	// Fallback for typographic style variance only — full-width
	// punctuation ↔ half-width and ASCII-letter|digit ↔ CJK boundary
	// whitespace. Member identity tokens still must appear in the
	// surface (no substring-search relaxation). Per
	// docs/design/post_phase2a_forensic_followups.md §2.3.
	nv := preEmitNormalizeForMixedCJK(value)
	ns := preEmitNormalizeForMixedCJK(surface)
	if nv == value && ns == surface {
		return false
	}
	return preEmitDisplaySurfaceAppearsStrict(nv, ns)
}

func preEmitDisplaySurfaceAppearsStrict(value, surface string) bool {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	surface = strings.Join(strings.Fields(strings.TrimSpace(surface)), " ")
	if value == "" || surface == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(surface[start:], value)
		if idx < 0 {
			return false
		}
		pos := start + idx
		if preEmitDisplaySurfaceBoundary(surface, pos-1) &&
			preEmitDisplaySurfaceBoundary(surface, pos+len(value)) {
			return true
		}
		start = pos + len(value)
		if start >= len(surface) {
			return false
		}
	}
}

func preEmitDisplaySurfaceAppearsFold(value, surface string) bool {
	value = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
	surface = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(surface)), " "))
	if value == "" || surface == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(surface[start:], value)
		if idx < 0 {
			return false
		}
		pos := start + idx
		if preEmitDisplaySurfaceBoundary(surface, pos-1) &&
			preEmitDisplaySurfaceBoundary(surface, pos+len(value)) {
			return true
		}
		start = pos + len(value)
		if start >= len(surface) {
			return false
		}
	}
}

func preEmitDisplaySurfaceBoundary(s string, idx int) bool {
	if idx < 0 || idx >= len(s) {
		return true
	}
	r := rune(s[idx])
	return !(r == '_' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z')
}

// preEmitNormalizeForMixedCJK relaxes the strict member-surface
// oracle for typographic variance that arises when an investigator
// emits a member with EN punctuation + ASCII-style spacing while the
// finalizer renders the same identity in zh prose. The rewrite is
// closed-set — no token is dropped, no substring relaxation, only
// display-style folding. Pure-ASCII identifiers (e.g. gate.RunWith)
// pass through byte-identical so the strict comparator semantics
// remain unchanged on that surface.
//
// Mappings:
//   - full-width punctuation → half-width: （）：！，。
//   - optional whitespace between an ASCII letter|digit and a CJK
//     letter → removed (so "vs 正文" ≡ "vs正文")
//   - optional whitespace between an identifier-char (ASCII alnum or
//     CJK letter) and an opening '(' → removed; required for
//     "Foo (qualifier)" ≡ "Foo（qualifier）" after zh-paren strip,
//     since EN typography puts a space before '(' while zh does not.
//     '(' is the only opening paren in the closed punctuation set
//     above; ')' is NOT included on either side to avoid eroding
//     external word boundaries.
//   - multi-space collapsed; leading/trailing trimmed
//
// docs/design/post_phase2a_forensic_followups.md §2.3 (2026-05-17).
func preEmitNormalizeForMixedCJK(s string) string {
	if s == "" {
		return s
	}
	var pb strings.Builder
	pb.Grow(len(s))
	for _, r := range s {
		switch r {
		case '（':
			pb.WriteByte('(')
		case '）':
			pb.WriteByte(')')
		case '：':
			pb.WriteByte(':')
		case '！':
			pb.WriteByte('!')
		case '，':
			pb.WriteByte(',')
		case '。':
			pb.WriteByte('.')
		default:
			pb.WriteRune(r)
		}
	}
	runes := []rune(pb.String())
	out := make([]rune, 0, len(runes))
	i := 0
	for i < len(runes) {
		r := runes[i]
		if !unicode.IsSpace(r) {
			out = append(out, r)
			i++
			continue
		}
		j := i
		for j < len(runes) && unicode.IsSpace(runes[j]) {
			j++
		}
		hasPrev := len(out) > 0
		hasNext := j < len(runes)
		if hasPrev && hasNext {
			prev := out[len(out)-1]
			next := runes[j]
			if !preEmitIsTypographicFoldBoundary(prev, next) {
				out = append(out, ' ')
			}
		}
		i = j
	}
	return string(out)
}

func preEmitIsTypographicFoldBoundary(a, b rune) bool {
	// ASCII letter|digit ↔ CJK letter (per spec).
	if (preEmitIsASCIIAlnum(a) && preEmitIsCJKLetter(b)) ||
		(preEmitIsCJKLetter(a) && preEmitIsASCIIAlnum(b)) {
		return true
	}
	// identifier-char ↔ '(' — EN-vs-zh paren-spacing asymmetry.
	// Asymmetric on close-paren side to preserve external word
	// boundary "(foo) bar" so a substring search for "(foo)" still
	// has ' ' as the right-side boundary char.
	if a == '(' && (preEmitIsASCIIAlnum(b) || preEmitIsCJKLetter(b)) {
		return true
	}
	if b == '(' && (preEmitIsASCIIAlnum(a) || preEmitIsCJKLetter(a)) {
		return true
	}
	return false
}

func preEmitIsASCIIAlnum(r rune) bool {
	return (r >= '0' && r <= '9') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= 'a' && r <= 'z')
}

func preEmitIsCJKLetter(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func changeImpactFileLabelsForHint(diag *types.ChangeImpactFileOutputLabelDiagnostic) string {
	if diag == nil || len(diag.MissingLabels) == 0 {
		return "one file-path label per typed file obligation"
	}
	parts := make([]string, 0, len(diag.MissingLabels))
	for _, member := range diag.MissingLabels {
		label := strings.TrimSpace(member.Source)
		if label == "" {
			label = strings.TrimSpace(member.Label)
		}
		if label == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%q", label))
		if len(parts) >= 8 {
			break
		}
	}
	if len(parts) == 0 {
		return "one file-path label per typed file obligation"
	}
	if diag.ExpectedCount > len(parts) {
		parts = append(parts, fmt.Sprintf("... (%d typed file members total)", diag.ExpectedCount))
	}
	return strings.Join(parts, ", ")
}

func changeImpactTargetForHint(diag *types.ChangeImpactNarrowingDiagnostic) string {
	if diag == nil || strings.TrimSpace(diag.Target) == "" {
		return "the requested target"
	}
	return diag.Target
}

func changeImpactMissingMembersForHint(diag *types.ChangeImpactNarrowingDiagnostic) string {
	if diag == nil {
		return ""
	}
	parts := make([]string, 0, len(diag.MissingMembers))
	for _, m := range diag.MissingMembers {
		label := strings.TrimSpace(m.Label)
		if label == "" {
			label = m.Location
		}
		form := string(m.ClaimForm)
		if form == "" {
			form = "principal_evidence"
		}
		parts = append(parts, fmt.Sprintf("%q (%s at %s)", label, form, m.LocationHint()))
		if len(parts) >= 6 {
			break
		}
	}
	return strings.Join(parts, "; ")
}

func preCheckAbsenceScopeBound(doc *types.AnswerDocumentV2) []emitFixHint {
	if doc == nil || doc.ExactResolution == nil || doc.ExactResolution.Status != types.AnswerExactResolutionAbsent {
		return nil
	}
	for _, c := range doc.Citations {
		if c.Scope == types.ScopeNegative && strings.TrimSpace(c.NegativePattern) != "" {
			return nil
		}
	}
	return []emitFixHint{{
		Field:         "citations[]",
		ExpectedShape: "when exact_resolution.status is absent, include at least one citation with scope=\"negative\" and a non-empty negative_pattern that names the bounded search/query proving absence.",
		Reason:        "an exact absence answer needs a typed negative-scope proof; a normal file:line citation or vague prose cannot bound what was searched.",
	}}
}

func preEmitBlockCitationRoleForms(b types.AnswerBlock, view *types.AnswerSemanticView) []types.ClaimForm {
	switch b.Kind {
	case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
	default:
		return nil
	}
	var forms []types.ClaimForm
	for _, cu := range b.ClaimUses {
		forms = append(forms, cu.ClaimForm)
	}
	if len(forms) > 0 {
		return types.ClaimFormsSupportingCitationRoleAlignment(forms)
	}
	if view != nil {
		for _, req := range append(append([]types.BlockRequirement(nil), view.RequiredBlocks...), view.OptionalBlocks...) {
			if !req.AcceptsKind(b.Kind) || len(req.AcceptableClaimForms) == 0 {
				continue
			}
			if len(req.FacetIDs) > 0 && !preEmitBlockSharesFacet(b, req.FacetIDs) {
				continue
			}
			forms = append(forms, req.AcceptableClaimForms...)
		}
	}
	if containsBlockFacet(b, types.FacetPrincipalPathEdge) {
		forms = append(forms, types.ClaimCallEdge)
	}
	if view != nil && (view.Family == types.QFCallChain || view.Family == types.QFRootCauseTrace) &&
		containsBlockFacet(b, types.FacetCurrentCodePath) {
		forms = append(forms, types.ClaimCallEdge)
	}
	return types.ClaimFormsSupportingCitationRoleAlignment(forms)
}

func preEmitItemMatchesSourceLocationPrincipalMember(ctx *types.BusContext, item types.AnswerBlockItem, cit types.Citation) bool {
	if ctx == nil {
		return false
	}
	plan := types.BuildAnswerSupportPlanForBusContext(ctx)
	if plan == nil || plan.ChangeImpactProfile == nil || !plan.ChangeImpactProfile.Active() {
		return false
	}
	label := strings.TrimSpace(item.Label)
	if label == "" {
		return false
	}
	switch plan.ChangeImpactProfile.RequestedOutput {
	case types.ImpactOutputFiles:
		if !types.AnswerFilePathLabelMatchesCitation(label, cit) {
			return false
		}
	case types.ImpactOutputSites:
		if !types.AnswerSourceLocationLabelMatchesCitation(label, cit) {
			return false
		}
	default:
		return false
	}
	for _, ob := range types.PrincipalSupportMemberObligations(plan) {
		if preEmitSourcePrincipalObligationMatchesItem(ob, label, cit, plan.ChangeImpactProfile.RequestedOutput) {
			return true
		}
	}
	return false
}

func preEmitSourcePrincipalObligationMatchesItem(ob types.AnswerSupportMemberObligation, label string, cit types.Citation, output types.ImpactRequestedOutput) bool {
	switch output {
	case types.ImpactOutputFiles:
		labelFile, ok := types.ParseAnswerFilePathSurface(label)
		if !ok {
			return false
		}
		if !preEmitPathMatches(ob.Source, labelFile) {
			return false
		}
		return preEmitPathMatches(cit.File, labelFile)
	case types.ImpactOutputSites:
		surface, ok := types.ParseAnswerSourceLocationSurface(label)
		if !ok || !preEmitCitationMatchesSourceLocation(cit, surface) {
			return false
		}
		return preEmitSupportObligationHasCitation(ob, cit)
	default:
		return false
	}
}

func preEmitSupportObligationHasCitation(ob types.AnswerSupportMemberObligation, cit types.Citation) bool {
	want := preEmitCitationLocationKey(cit)
	if want == "" {
		return false
	}
	if preEmitNormalizeLocation(ob.Location) == want || preEmitLocationMatchesCitation(ob.Location, cit) {
		return true
	}
	for _, loc := range ob.EquivalentLocations {
		if preEmitNormalizeLocation(loc) == want || preEmitLocationMatchesCitation(loc, cit) {
			return true
		}
	}
	return false
}

func preEmitCitationLocationKey(cit types.Citation) string {
	file := preEmitNormalizePath(cit.File)
	if file == "" || cit.Line <= 0 {
		return ""
	}
	return file + ":" + fmt.Sprintf("%d", cit.Line)
}

func preEmitNormalizeLocation(location string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(location, `\`, `/`)))
}

func preEmitNormalizePath(path string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(path, `\`, `/`)))
}

func preEmitPathMatches(a, b string) bool {
	a = preEmitNormalizePath(strings.TrimPrefix(strings.TrimSpace(a), "./"))
	b = preEmitNormalizePath(strings.TrimPrefix(strings.TrimSpace(b), "./"))
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.HasSuffix(a, "/"+b) || strings.HasSuffix(b, "/"+a)
}

func preEmitCitationSameLocation(a, b types.Citation) bool {
	return a.Line > 0 && a.Line == b.Line && preEmitPathMatches(a.File, b.File)
}

func preEmitLocationMatchesCitation(location string, cit types.Citation) bool {
	location = strings.TrimSpace(location)
	if location == "" || cit.Line <= 0 {
		return false
	}
	idx := strings.LastIndex(location, ":")
	if idx <= 0 || idx >= len(location)-1 {
		return false
	}
	line, err := strconv.Atoi(strings.TrimSpace(location[idx+1:]))
	if err != nil || line != cit.Line {
		return false
	}
	return preEmitPathMatches(location[:idx], cit.File)
}

func preEmitAnswerEvidenceItems(ctx *types.BusContext) []types.EvidenceItem {
	return newPreEmitCheckContext(ctx).evidenceItems()
}

func preEmitBlockSharesFacet(b types.AnswerBlock, facets []string) bool {
	if len(facets) == 0 {
		return false
	}
	for _, facet := range facets {
		facet = strings.TrimSpace(facet)
		if facet == "" {
			continue
		}
		for _, id := range b.FacetIDs {
			if strings.TrimSpace(id) == facet {
				return true
			}
		}
		for _, cu := range b.ClaimUses {
			if strings.TrimSpace(cu.FacetID) == facet {
				return true
			}
		}
	}
	return false
}

func preEmitClaimRoleMentionedByItemSurface(item types.AnswerBlockItem, forms []types.ClaimForm, evidence []types.EvidenceItem) (types.EvidenceItem, bool) {
	label := strings.TrimSpace(item.Label)
	if label == "" {
		for _, cell := range item.Cells {
			if strings.TrimSpace(cell) != "" {
				label = strings.TrimSpace(cell)
				break
			}
		}
	}
	text := preEmitItemNonLabelSurface(item)
	if label == "" && text == "" {
		return types.EvidenceItem{}, false
	}
	for _, ev := range evidence {
		if types.EvidenceClaimRoleAssertedByAnswerSurface(ev, forms, label, text) {
			return ev, true
		}
	}
	return types.EvidenceItem{}, false
}

func preEmitItemNonLabelSurface(item types.AnswerBlockItem) string {
	var parts []string
	if text := strings.TrimSpace(item.Text); text != "" {
		parts = append(parts, text)
	}
	for _, cell := range item.Cells {
		cell = strings.TrimSpace(cell)
		if cell != "" {
			parts = append(parts, cell)
		}
	}
	return strings.Join(parts, "\n")
}

func preEmitBlockUsesNonSymbolLabelSurface(b types.AnswerBlock, view *types.AnswerSemanticView) bool {
	for _, cu := range b.ClaimUses {
		if cu.ClaimForm.UsesNonSymbolLabelSurface() {
			return true
		}
	}
	return view != nil && view.Family == types.QFConfigPrecedence &&
		containsBlockFacet(b, types.FacetConfigPrecedenceRole)
}

func containsBlockFacet(b types.AnswerBlock, facet types.AnswerFacetKind) bool {
	want := string(facet)
	for _, id := range b.FacetIDs {
		if strings.TrimSpace(id) == want {
			return true
		}
	}
	for _, cu := range b.ClaimUses {
		if strings.TrimSpace(cu.FacetID) == want {
			return true
		}
	}
	return false
}

func preEmitCitedEvidenceItems(ctx *types.BusContext, cit types.Citation) ([]types.EvidenceItem, bool) {
	return newPreEmitCheckContext(ctx).citedEvidenceItems(cit)
}

func preEmitCitedEvidenceFromPool(items []types.EvidenceItem, file string, line int) []types.EvidenceItem {
	var out []types.EvidenceItem
	for _, ev := range items {
		if ev.GroundingStatus == types.GroundingUngrounded {
			continue
		}
		if strings.TrimSpace(ev.Source) != file {
			continue
		}
		if line < ev.LineStart {
			continue
		}
		lineEnd := ev.LineEnd
		if lineEnd <= 0 {
			lineEnd = ev.LineStart
		}
		if line > lineEnd {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func preEmitLabelMatchesAnyEvidenceEndpoint(label string, evidence []types.EvidenceItem) bool {
	for _, ev := range evidence {
		if preEmitLabelMatchesEvidenceEndpoint(label, ev) {
			return true
		}
	}
	return false
}

func preEmitDecoratedItemLabelMatchesAnyEvidenceEndpoint(label, text string, evidence []types.EvidenceItem) bool {
	for _, ev := range evidence {
		if preEmitDecoratedItemLabelMatchesEvidence(label, text, ev) {
			return true
		}
	}
	return false
}

func preEmitCandidateCitationLocationsForLabel(ctx *types.BusContext, label string, limit int) []string {
	return preEmitCandidateCitationLocationsForLabelWithContext(newPreEmitCheckContext(ctx), label, limit)
}

func preEmitCandidateCitationLocationsForLabelWithContext(pctx *preEmitCheckContext, label string, limit int) []string {
	if limit <= 0 {
		limit = 4
	}
	var out []string
	seen := make(map[string]bool)
	for _, ev := range pctx.evidenceItems() {
		if ev.GroundingStatus == types.GroundingUngrounded {
			continue
		}
		if !preEmitLabelMatchesEvidenceEndpoint(label, ev) {
			continue
		}
		file := strings.TrimSpace(ev.Source)
		if file == "" || ev.LineStart <= 0 {
			continue
		}
		loc := fmt.Sprintf("%s:%d", file, ev.LineStart)
		if seen[loc] {
			continue
		}
		seen[loc] = true
		out = append(out, loc)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func preEmitCandidateCitationLocationsForAggregateItem(ctx *types.BusContext, label, text string, limit int) []string {
	return preEmitCandidateCitationLocationsForAggregateItemWithContext(newPreEmitCheckContext(ctx), label, text, limit)
}

func preEmitCandidateCitationLocationsForAggregateItemWithContext(pctx *preEmitCheckContext, label, text string, limit int) []string {
	if pctx == nil || pctx.ctx == nil || pctx.ctx.Mutable == nil {
		return nil
	}
	ctx := pctx.ctx
	if limit <= 0 {
		limit = 4
	}
	var out []string
	seen := make(map[string]bool)
	add := func(file string, line int) {
		file = pctx.canonicalPath(file)
		if file == "" || line <= 0 || len(out) >= limit {
			return
		}
		loc := fmt.Sprintf("%s:%d", file, line)
		key := strings.ToLower(strings.ReplaceAll(loc, `\`, `/`))
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, loc)
	}
	for _, ref := range preEmitPrincipalAggregateMemberSetFactRefs(ctx, ctx.Mutable.StableInvestigationAggregateFacts()) {
		fact := ref.Fact
		for idx, member := range fact.Members {
			if !preEmitAggregateMemberLabelTextMatches(label, text, member) {
				continue
			}
			if source, line, ok := preEmitAggregateMemberSupportLocation(fact, idx, member); ok {
				add(source, line)
			}
			for _, ev := range pctx.evidenceItems() {
				if ev.GroundingStatus == types.GroundingUngrounded || ev.Source == "" || ev.LineStart <= 0 {
					continue
				}
				if preEmitEvidenceSupportsAggregateMemberCitation(ev, member) {
					add(ev.Source, ev.LineStart)
				}
			}
		}
	}
	return out
}

func preEmitCitationSupportsAggregateItem(ctx *types.BusContext, label, text string, cit types.Citation) bool {
	return preEmitCitationSupportsAggregateItemWithContext(newPreEmitCheckContext(ctx), label, text, cit)
}

func preEmitCitationSupportsAggregateItemWithContext(pctx *preEmitCheckContext, label, text string, cit types.Citation) bool {
	if pctx == nil || pctx.ctx == nil || pctx.ctx.Mutable == nil {
		return false
	}
	cit = pctx.canonicalCitation(cit)
	ctx := pctx.ctx
	label = strings.TrimSpace(label)
	text = strings.TrimSpace(text)
	if label == "" || text == "" {
		return false
	}
	for _, ref := range preEmitPrincipalAggregateMemberSetFactRefs(ctx, ctx.Mutable.StableInvestigationAggregateFacts()) {
		fact := ref.Fact
		for idx, member := range fact.Members {
			if !preEmitAggregateMemberLabelTextMatches(label, text, member) {
				continue
			}
			if preEmitAggregateMemberCitationMatches(fact, idx, member, cit) {
				return true
			}
			evidence, found := pctx.citedEvidenceItems(cit)
			if !found {
				continue
			}
			for _, ev := range evidence {
				if preEmitEvidenceSupportsAggregateMemberCitation(ev, member) {
					return true
				}
			}
		}
	}
	return false
}

func preEmitLabelMatchesEvidenceEndpoint(label string, ev types.EvidenceItem) bool {
	if preEmitDecoratedLabelMatchesEvidence(label, ev) {
		return true
	}
	if matched, handled := preEmitDisplayLabelCodeSurfacesMatchEvidence(label, ev); handled {
		return matched
	}
	if matched, handled := preEmitQualifiedCodeSurfaceMatchesEvidence(label, ev); handled {
		return matched
	}
	if _, _, ok := types.AnswerAggregateDecoratedLabelParts(label); ok {
		return false
	}
	for _, endpoint := range []string{ev.Subject, ev.Object, ev.AnchorSymbol, ev.OwnerSymbol} {
		if preEmitCodeSurfaceMatches(label, endpoint) {
			return true
		}
	}
	for _, term := range ev.SurfaceTerms {
		if preEmitCodeSurfaceAppearsVerbatim(label, term) {
			return true
		}
	}
	if preEmitCodeSurfaceAppearsVerbatim(label, ev.Snippet) {
		return true
	}
	return false
}

func preEmitLabelNeedsCitationAlignment(label string) bool {
	label = strings.TrimSpace(label)
	if label == "" {
		return false
	}
	if types.IsCodeIdentitySurface(label) {
		return true
	}
	base, _, ok := types.AnswerAggregateDecoratedLabelParts(label)
	if ok && types.IsCodeIdentitySurface(base) {
		return true
	}
	return len(preEmitExplicitDisplayCodeSurfaces(label)) > 0
}

func preEmitDecoratedLabelMatchesEvidence(label string, ev types.EvidenceItem) bool {
	base, qualifier, ok := types.AnswerAggregateDecoratedLabelParts(label)
	if !ok {
		return false
	}
	if !preEmitEvidenceEndpointSupportsToken(ev, base) {
		return false
	}
	return preEmitEvidenceSupportsDecoratorQualifier(ev, qualifier)
}

func preEmitDecoratedItemLabelMatchesEvidence(label, text string, ev types.EvidenceItem) bool {
	base, qualifier, ok := types.AnswerAggregateDecoratedLabelParts(label)
	if !ok {
		return false
	}
	if !preEmitEvidenceEndpointSupportsToken(ev, base) {
		return false
	}
	if preEmitEvidenceSupportsDecoratorQualifier(ev, qualifier) {
		return true
	}
	// Parenthetical qualifiers on ordered-list/table item labels are
	// often reader-facing disambiguators ("fast path", "retry", "slow
	// path"), not source-code identifiers. Once the code identity before
	// the parenthesis is grounded by the cited evidence, treat non-code
	// qualifiers as display context instead of forcing an LLM retry just
	// because the exact prose is not present on the source line. Code-like
	// qualifiers ("subject", "SubExplorer", "Foo.Bar") still need evidence
	// support so scoped same-name symbols do not drift.
	if types.IsCodeIdentitySurface(qualifier) || len(preEmitExplicitDisplayCodeSurfaces(qualifier)) > 0 {
		return false
	}
	return true
}

func preEmitEvidenceSupportsDecoratorQualifier(ev types.EvidenceItem, qualifier string) bool {
	parts := preEmitDecoratorQualifierParts(qualifier)
	if len(parts) == 0 {
		return false
	}
	// A single qualifier is usually a package/type/scope disambiguator
	// ("Score (subject)", "New (Classifier)") and must be grounded by the
	// cited line or source path. Multi-part qualifiers often describe method
	// families ("Engine (New + Submit/Apply)") and are enforced by relation
	// left-scope matching when present, not by requiring all siblings on one
	// definition line.
	if len(parts) > 1 {
		for _, part := range parts {
			if preEmitEvidenceEndpointSupportsToken(ev, part) ||
				preEmitPathSegmentsSupportToken(ev.Source, part) ||
				preEmitEvidenceTextContainsQualifier(ev, part) {
				return true
			}
		}
		return true
	}
	part := parts[0]
	return preEmitEvidenceEndpointSupportsToken(ev, part) ||
		preEmitPathSegmentsSupportToken(ev.Source, part) ||
		preEmitEvidenceTextContainsQualifier(ev, part)
}

func preEmitEvidenceTextContainsQualifier(ev types.EvidenceItem, qualifier string) bool {
	qualifier = strings.TrimSpace(qualifier)
	if qualifier == "" {
		return false
	}
	if preEmitCodeSurfaceAppearsVerbatim(qualifier, ev.Snippet) ||
		preEmitCodeSurfaceAppearsVerbatim(qualifier, ev.Summary) {
		return true
	}
	if preEmitTextContainsLoose(ev.Snippet, qualifier) || preEmitTextContainsLoose(ev.Summary, qualifier) {
		return true
	}
	return preEmitSurfaceTermsSupportToken(ev.SurfaceTerms, qualifier)
}

func preEmitSurfaceTermsSupportToken(terms []string, token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	for _, term := range terms {
		if preEmitCodeSurfaceAppearsVerbatim(token, term) || preEmitTextContainsLoose(term, token) {
			return true
		}
	}
	return false
}

func preEmitTextContainsLoose(text, needle string) bool {
	text = strings.TrimSpace(text)
	needle = strings.TrimSpace(needle)
	if text == "" || needle == "" {
		return false
	}
	if strings.Contains(text, needle) {
		return true
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(needle))
}

func preEmitDecoratorQualifierParts(qualifier string) []string {
	qualifier = strings.TrimSpace(qualifier)
	if qualifier == "" {
		return nil
	}
	raw := strings.FieldsFunc(qualifier, func(r rune) bool {
		switch r {
		case '/', '+', ',', '，', '、', '&', '|':
			return true
		default:
			return false
		}
	})
	var out []string
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func preEmitDisplayLabelCodeSurfacesMatchEvidence(label string, ev types.EvidenceItem) (bool, bool) {
	if types.IsCodeIdentitySurface(strings.TrimSpace(label)) {
		return false, false
	}
	surfaces := preEmitExplicitDisplayCodeSurfaces(label)
	if len(surfaces) == 0 {
		return false, false
	}
	for _, surface := range surfaces {
		if preEmitCodeSurfaceMatchesEvidence(surface, ev) {
			return true, true
		}
	}
	return false, true
}

func preEmitExplicitDisplayCodeSurfaces(label string) []string {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}
	var out []string
	if base, qualifier, ok := types.AnswerAggregateDecoratedLabelParts(label); ok &&
		(!types.IsCodeIdentitySurface(base) || preEmitDecoratedBaseLooksDisplayProse(base)) {
		out = appendCodeIdentitySurface(out, qualifier)
		for _, part := range preEmitDecoratorQualifierParts(qualifier) {
			out = appendCodeIdentitySurface(out, part)
		}
	}
	for _, surface := range preEmitBacktickCodeSurfaces(label) {
		out = appendCodeIdentitySurface(out, surface)
	}
	return out
}

func preEmitBacktickCodeSurfaces(label string) []string {
	var out []string
	for {
		start := strings.Index(label, "`")
		if start < 0 {
			return out
		}
		rest := label[start+1:]
		end := strings.Index(rest, "`")
		if end < 0 {
			return out
		}
		out = append(out, rest[:end])
		label = rest[end+1:]
	}
}

func appendCodeIdentitySurface(out []string, surface string) []string {
	surface = strings.Trim(strings.TrimSpace(surface), "`'\" ")
	if surface == "" || !types.IsCodeIdentitySurface(surface) {
		return out
	}
	key := strings.ToLower(surface)
	for _, existing := range out {
		if strings.ToLower(existing) == key {
			return out
		}
	}
	return append(out, surface)
}

func preEmitDecoratedBaseLooksDisplayProse(base string) bool {
	base = strings.TrimSpace(base)
	if base == "" {
		return false
	}
	for _, r := range base {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			return true
		}
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func preEmitCodeSurfaceMatchesEvidence(surface string, ev types.EvidenceItem) bool {
	if matched, handled := preEmitQualifiedCodeSurfaceMatchesEvidence(surface, ev); handled {
		return matched
	}
	for _, endpoint := range []string{ev.Subject, ev.Object, ev.AnchorSymbol, ev.OwnerSymbol} {
		if preEmitCodeSurfaceMatches(surface, endpoint) {
			return true
		}
	}
	for _, term := range ev.SurfaceTerms {
		if preEmitCodeSurfaceAppearsVerbatim(surface, term) {
			return true
		}
	}
	return preEmitCodeSurfaceAppearsVerbatim(surface, ev.Snippet)
}

func preEmitQualifiedCodeSurfaceMatchesEvidence(surface string, ev types.EvidenceItem) (bool, bool) {
	owner, member, ok := preEmitQualifiedCodeSurfaceParts(surface)
	if !ok {
		return false, false
	}
	if preEmitEvidenceEndpointSupportsExactSurface(ev, surface) {
		return true, true
	}
	if preEmitCodeSurfaceAppearsVerbatim(surface, ev.Summary) &&
		preEmitEvidenceEndpointSupportsToken(ev, member) &&
		preEmitQualifiedOwnerSupportedByEvidence(ev, owner, member) {
		return true, true
	}
	ownerOK := preEmitQualifiedOwnerSupportedByEvidence(ev, owner, member)
	memberOK := preEmitEvidenceEndpointSupportsToken(ev, member) ||
		preEmitCodeSurfaceAppearsVerbatim(member, ev.Snippet) ||
		preEmitPathSegmentsSupportToken(ev.Source, member)
	return ownerOK && memberOK, true
}

func preEmitQualifiedOwnerSupportedByEvidence(ev types.EvidenceItem, owner, member string) bool {
	if preEmitEvidenceEndpointSupportsToken(ev, owner) ||
		preEmitCodeSurfaceAppearsVerbatim(owner, ev.Snippet) {
		return true
	}
	if !preEmitPathSegmentsSupportToken(ev.Source, owner) {
		return false
	}
	return !preEmitEvidenceSnippetConflictsWithQualifiedOwner(ev.Snippet, owner, member)
}

func preEmitEvidenceSnippetConflictsWithQualifiedOwner(snippet, owner, member string) bool {
	snippet = strings.TrimSpace(snippet)
	if snippet == "" || !preEmitCodeSurfaceAppearsVerbatim(member, snippet) ||
		preEmitCodeSurfaceAppearsVerbatim(owner, snippet) {
		return false
	}
	recvOwner := preEmitGoReceiverOwnerForMethodSnippet(snippet, member)
	return recvOwner != "" && preEmitCodeIdentityKey(recvOwner) != preEmitCodeIdentityKey(owner)
}

func preEmitGoReceiverOwnerForMethodSnippet(snippet, member string) string {
	snippet = strings.TrimSpace(snippet)
	member = strings.Trim(strings.TrimSpace(member), "`'\" ")
	if snippet == "" || member == "" || !strings.HasPrefix(snippet, "func (") {
		return ""
	}
	closeIdx := strings.Index(snippet, ")")
	if closeIdx <= len("func (") {
		return ""
	}
	receiver := strings.TrimSpace(snippet[len("func ("):closeIdx])
	fields := strings.Fields(receiver)
	if len(fields) == 0 {
		return ""
	}
	owner := strings.Trim(strings.TrimSpace(fields[len(fields)-1]), "*")
	rest := strings.TrimSpace(snippet[closeIdx+1:])
	if !strings.HasPrefix(rest, member) {
		return ""
	}
	return owner
}

func preEmitCitationEnclosingFunctionSupportsLabel(label string, cit types.Citation) bool {
	fn := strings.TrimSpace(cit.EnclosingFunction)
	if fn == "" {
		return false
	}
	if preEmitEnclosingFunctionSurfaceMatches(label, fn) {
		return true
	}
	for _, surface := range preEmitExplicitDisplayCodeSurfaces(label) {
		if preEmitEnclosingFunctionSurfaceMatches(surface, fn) {
			return true
		}
	}
	return false
}

func preEmitCitationEnclosingFunctionConflictsWithQualifiedLabel(label string, cit types.Citation) bool {
	_, member, qualified := preEmitQualifiedCodeSurfaceParts(label)
	if !qualified {
		return false
	}
	fn := preEmitNormalizeCallableSurface(cit.EnclosingFunction)
	if fn == "" || preEmitEnclosingFunctionSurfaceMatches(label, fn) {
		return false
	}
	_, fnMember, fnQualified := preEmitQualifiedCodeSurfaceParts(fn)
	return fnQualified && preEmitCodeIdentityKey(fnMember) == preEmitCodeIdentityKey(member)
}

func preEmitEnclosingFunctionSurfaceMatches(surface, fn string) bool {
	surface = preEmitNormalizeCallableSurface(surface)
	fn = preEmitNormalizeCallableSurface(fn)
	if surface == "" || fn == "" || !types.IsCodeIdentitySurface(surface) || !types.IsCodeIdentitySurface(fn) {
		return false
	}
	if strings.EqualFold(surface, fn) {
		return true
	}
	owner, member, qualified := preEmitQualifiedCodeSurfaceParts(surface)
	fnOwner, fnMember, fnQualified := preEmitQualifiedCodeSurfaceParts(fn)
	if qualified {
		return fnQualified &&
			preEmitCodeIdentityKey(owner) == preEmitCodeIdentityKey(fnOwner) &&
			preEmitCodeIdentityKey(member) == preEmitCodeIdentityKey(fnMember)
	}
	if fnQualified {
		return preEmitCodeIdentityKey(surface) == preEmitCodeIdentityKey(fnMember)
	}
	return preEmitCodeIdentityKey(surface) == preEmitCodeIdentityKey(fn)
}

func preEmitNormalizeCallableSurface(surface string) string {
	surface = strings.Trim(strings.TrimSpace(surface), "`'\" ")
	if surface == "" {
		return ""
	}
	if strings.HasPrefix(surface, "(*") {
		if idx := strings.Index(surface, ")."); idx > 0 {
			recv := strings.Trim(strings.TrimSpace(surface[1:idx]), "* ")
			if recv != "" {
				return recv + surface[idx+1:]
			}
		}
	}
	if strings.HasPrefix(surface, "(") {
		if idx := strings.Index(surface, ")."); idx > 0 {
			recv := strings.TrimSpace(surface[1:idx])
			if recv != "" {
				return recv + surface[idx+1:]
			}
		}
	}
	return strings.TrimPrefix(surface, "*")
}

func preEmitEvidenceEndpointSupportsExactSurface(ev types.EvidenceItem, surface string) bool {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return false
	}
	for _, endpoint := range []string{ev.Subject, ev.Object, ev.AnchorSymbol, ev.OwnerSymbol} {
		if strings.EqualFold(strings.TrimSpace(endpoint), surface) {
			return true
		}
	}
	for _, term := range ev.SurfaceTerms {
		if preEmitCodeSurfaceAppearsVerbatim(surface, term) {
			return true
		}
	}
	return preEmitCodeSurfaceAppearsVerbatim(surface, ev.Snippet)
}

func preEmitQualifiedCodeSurfaceParts(surface string) (owner string, member string, ok bool) {
	surface = strings.Trim(strings.TrimSpace(surface), "`'\" ")
	if surface == "" || !types.IsCodeIdentitySurface(surface) {
		return "", "", false
	}
	if _, ok := types.ParseAnswerSourceLocationSurface(surface); ok {
		return "", "", false
	}
	if _, ok := types.ParseAnswerFilePathSurface(surface); ok {
		return "", "", false
	}
	if strings.Count(surface, "::") == 1 && !strings.Contains(surface, "/") {
		parts := strings.Split(surface, "::")
		owner = strings.TrimSpace(parts[0])
		member = strings.TrimSpace(parts[1])
	} else if strings.Count(surface, ".") == 1 && !strings.Contains(surface, "/") {
		parts := strings.Split(surface, ".")
		owner = strings.TrimSpace(parts[0])
		member = strings.TrimSpace(parts[1])
	} else {
		return "", "", false
	}
	if owner == "" || member == "" || !types.IsCodeIdentitySurface(owner) || !types.IsCodeIdentitySurface(member) {
		return "", "", false
	}
	return owner, member, true
}

func preEmitCodeSurfaceAppearsVerbatim(label, text string) bool {
	label = strings.TrimSpace(label)
	text = strings.TrimSpace(text)
	if label == "" || text == "" || !types.IsCodeIdentitySurface(label) {
		return false
	}
	return strings.Contains(text, label)
}

func preEmitCodeSurfaceMatches(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if strings.EqualFold(a, b) {
		return true
	}
	if types.IsCodeIdentitySurface(a) && types.IsCodeIdentitySurface(b) {
		aFold := strings.ToLower(strings.TrimSpace(a))
		bFold := strings.ToLower(strings.TrimSpace(b))
		if strings.Contains(aFold, bFold) || strings.Contains(bFold, aFold) {
			return true
		}
	}
	aTail := types.NormalizedSurfaceSymbolTail(a)
	bTail := types.NormalizedSurfaceSymbolTail(b)
	return aTail != "" && bTail != "" && strings.EqualFold(aTail, bTail)
}

func preCheckModelSurfaceTerms(doc *types.AnswerDocumentV2, ctx *types.BusContext) []emitFixHint {
	if doc == nil || ctx == nil {
		return nil
	}
	evidence := modelSurfaceTermEvidence(ctx)
	if len(evidence) == 0 {
		return nil
	}
	var hints []emitFixHint
	for _, b := range doc.Blocks {
		if b.Kind != types.BlockOrderedList && b.Kind != types.BlockBulletList && b.Kind != types.BlockTable {
			continue
		}
		if b.Kind == types.BlockTable && strings.TrimSpace(b.Text) != "" {
			continue
		}
		for _, it := range b.Items {
			if it.CitationRef < 0 || it.CitationRef >= len(doc.Citations) {
				continue
			}
			cite := doc.Citations[it.CitationRef]
			missing := missingSurfaceTermsForItem(it, cite, evidence)
			if len(missing) == 0 {
				continue
			}
			field := fmt.Sprintf("blocks[id=%q].items[id=%q].label/text/cells", b.ID, it.ID)
			if strings.TrimSpace(it.ID) == "" {
				field = fmt.Sprintf("blocks[id=%q].items[].label/text/cells", b.ID)
			}
			hints = append(hints, emitFixHint{
				Field: field,
				ExpectedShape: "include these model-emitted surface_terms in the cited item label, text, or table cells: " +
					strings.Join(missing, ", "),
				Reason: "the investigation explicitly structured these source-visible labels; preserve them when they are relevant to the visible answer instead of relying on downstream synthesis to infer them.",
			})
		}
	}
	return hints
}

func modelSurfaceTermEvidence(ctx *types.BusContext) []types.EvidenceItem {
	var pool []types.EvidenceItem
	if ctx != nil && ctx.Mutable != nil {
		pool = append(pool, ctx.Mutable.EmittedEvidence()...)
	}
	if ctx != nil {
		pool = append(pool, ctx.EvidenceItems...)
	}
	out := make([]types.EvidenceItem, 0, len(pool))
	seen := make(map[string]bool, len(pool))
	for _, ev := range pool {
		if ev.Producer != EmitEvidenceProducer || len(ev.SurfaceTerms) == 0 {
			continue
		}
		key := ev.ID
		if key == "" {
			key = ev.Source + "\x00" + fmt.Sprint(ev.LineStart) + "\x00" + strings.Join(ev.SurfaceTerms, "\x00")
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ev)
	}
	return out
}

func missingSurfaceTermsForItem(item types.AnswerBlockItem, cite types.Citation, evidence []types.EvidenceItem) []string {
	if strings.TrimSpace(cite.File) == "" {
		return nil
	}
	hay := strings.ToLower(types.AnswerBlockItemVisibleSurface(item))
	seen := make(map[string]bool)
	var missing []string
	for _, ev := range evidence {
		if !sameSurfaceTermSource(ev.Source, cite.File) {
			continue
		}
		if !surfaceTermLineClose(ev, cite) {
			continue
		}
		if !surfaceTermEvidenceAppliesToItem(ev, item) {
			continue
		}
		for _, term := range ev.SurfaceTerms {
			term = strings.TrimSpace(term)
			if term == "" || strings.Contains(hay, strings.ToLower(term)) {
				continue
			}
			if !types.SurfaceTermShouldBeRequiredForEvidence(term, ev, item.Label) {
				continue
			}
			key := strings.ToLower(term)
			if seen[key] {
				continue
			}
			seen[key] = true
			missing = append(missing, term)
		}
	}
	return missing
}

func sameSurfaceTermSource(a, b string) bool {
	return preEmitPathMatches(a, b)
}

func surfaceTermLineClose(ev types.EvidenceItem, cite types.Citation) bool {
	if cite.Line <= 0 || ev.LineStart <= 0 {
		return true
	}
	end := ev.LineEnd
	if end < ev.LineStart {
		end = ev.LineStart
	}
	return cite.Line >= ev.LineStart-8 && cite.Line <= end+8
}

func surfaceTermEvidenceAppliesToItem(ev types.EvidenceItem, item types.AnswerBlockItem) bool {
	hay := strings.ToLower(types.AnswerBlockItemVisibleSurface(item))
	for _, key := range []string{ev.Subject, ev.AnchorSymbol, ev.Object, ev.OwnerSymbol} {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if strings.Contains(hay, strings.ToLower(key)) {
			return true
		}
	}
	return strings.TrimSpace(ev.Subject) == "" && strings.TrimSpace(ev.AnchorSymbol) == ""
}

// preCheckEnumerationLabelGrounding mirrors the post-emit
// validateEnumerationItemLabelHallucination check at the chokepoint:
// every list-item's leading identifier must resolve to a Tier 1-2
// codebase symbol via the typed graph SymbolOracle. When the
// validator finds hallucinated labels, it returns one fix hint per
// affected block so the LLM can revise within the same dispatch
// instead of paying a full repair-loop round downstream.
//
// Trigger conditions (mirror the post-emit gate to keep precision
// contracts identical):
//
//   - oracle != nil (caller-side gate; nil → silently no-op so
//     unit-test paths and no-graph runs keep working)
//   - block kind ∈ { ordered_list, bullet_list, table } — only
//     surfaces with "named symbol list" semantics
//   - leading identifier shape (labelLeadingSymbolIdentifier mirror)
//     length ≥ 10 chars to avoid stdlib-helper false positives
//   - oracle returns Tier=0 (not found) OR Tier ≥ 3 (low-confidence
//     parse) — same gate as contract.must_include / extractor_match
//
// Cross-language: the SymbolOracle is populated by repomap which
// covers all 12+ languages; this check is language-agnostic at
// the typed-API level.
//
// 2026-05-10 P1.
func preCheckEnumerationLabelGrounding(doc *types.AnswerDocumentV2, oracle types.SymbolOracle, ctxOpt ...*types.BusContext) []emitFixHint {
	return preCheckEnumerationLabelGroundingWithContext(doc, oracle, newPreEmitCheckContext(ctxOpt...))
}

func preEmitEnumerationLabelGroundingHardGate(pctx *preEmitCheckContext) bool {
	if pctx == nil || pctx.ctx == nil || pctx.ctx.AnalysisIR == nil {
		return true
	}
	rm := pctx.ctx.AnalysisIR.RequestModel
	if types.RequiresExhaustiveEnumerationMemberSetHandoff(rm) ||
		types.RequiresRelationMemberSetHandoff(rm) {
		return true
	}
	return types.HasPrincipalCategoryEnumerationMemberLane(rm)
}

func preCheckEnumerationLabelGroundingWithContext(doc *types.AnswerDocumentV2, oracle types.SymbolOracle, pctx *preEmitCheckContext) []emitFixHint {
	if doc == nil || oracle == nil {
		return nil
	}
	var ctx *types.BusContext
	if pctx != nil {
		ctx = pctx.ctx
	}
	var hints []emitFixHint
	for _, b := range doc.Blocks {
		if b.Kind != types.BlockOrderedList && b.Kind != types.BlockBulletList && b.Kind != types.BlockTable {
			continue
		}
		var hallucinatedIdents []string
		seen := make(map[string]bool)
		for _, it := range b.Items {
			label := strings.TrimSpace(it.Label)
			if label == "" {
				continue
			}
			if ctx != nil && it.CitationRef >= 0 && it.CitationRef < len(doc.Citations) &&
				types.AnswerLocationLabelMatchesCitation(label, pctx.canonicalCitation(doc.Citations[it.CitationRef])) {
				continue
			}
			ident := preEmitLabelLeadingIdentifier(label)
			if len(ident) < preEmitLabelLengthFloor {
				continue
			}
			if !contract.IsIdentifierShaped(ident) {
				continue
			}
			if preEmitLabelSupportedByQuestionBucket(label, ctx) {
				continue
			}
			if preEmitLabelSupportedByRuntimeArtifact(label, ctx) {
				continue
			}
			if preEmitLabelSupportedByAggregateMemberSet(label, it, types.Citation{}, ctx) {
				continue
			}
			if ctx != nil && it.CitationRef >= 0 && it.CitationRef < len(doc.Citations) {
				if evidence, found := pctx.citedEvidenceItems(doc.Citations[it.CitationRef]); found &&
					preEmitLabelMatchesAnyEvidenceEndpoint(label, evidence) {
					continue
				}
			}
			if preEmitLabelStartsWithQualifiedCodeIdentity(label) {
				continue
			}
			found, tier := oracle.SymbolExistsFlat(ident)
			if found && tier < 3 {
				continue
			}
			if seen[ident] {
				continue
			}
			seen[ident] = true
			hallucinatedIdents = append(hallucinatedIdents, ident)
		}
		if len(hallucinatedIdents) == 0 {
			continue
		}
		// Shape: one fix hint per affected block carrying every
		// hallucinated identifier so the LLM sees the full repair
		// scope without separate hint lines per item.
		hints = append(hints, emitFixHint{
			Field: fmt.Sprintf("blocks[id=%q].items[].label", b.ID),
			ExpectedShape: fmt.Sprintf(
				"replace these item label leading identifiers with names that exist in the codebase OR drop the items: %s",
				strings.Join(hallucinatedIdents, ", "),
			),
			Reason: "enumeration item labels must lead with codebase-grounded identifiers; the post-emit shape contract rejects fabricated names and forces a full repair retry.",
		})
	}
	return hints
}

func preEmitLabelSupportedByAggregateMemberSet(label string, item types.AnswerBlockItem, cit types.Citation, ctx *types.BusContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return false
	}
	facts := ctx.Mutable.StableInvestigationAggregateFacts()
	if len(facts) == 0 {
		return false
	}
	for _, ref := range preEmitPrincipalAggregateMemberSetFactRefs(ctx, facts) {
		fact := ref.Fact
		for _, member := range fact.Members {
			if preEmitAggregateMemberLabelMatches(label, member) {
				return true
			}
			if surface := preEmitItemNonLabelSurface(item); surface != "" &&
				preEmitAggregateMemberLabelTextMatches(label, surface, member) {
				return true
			}
		}
	}
	return false
}

func preEmitAggregateMemberLabelMatches(label, member string) bool {
	label = strings.TrimSpace(label)
	member = strings.TrimSpace(member)
	if label == "" || member == "" {
		return false
	}
	for _, surface := range preEmitAggregateMemberDisplayCandidates(member) {
		if preEmitTypedLabelTokenSupportsLabel(surface, label) ||
			preEmitTypedLabelTokenSupportsLabel(label, surface) {
			return true
		}
	}
	for _, surface := range preEmitAggregateMemberRelationSurfaces(member) {
		left, right, ok := preEmitAggregateMemberLabelRelationParts(surface)
		if !ok {
			continue
		}
		if preEmitTypedLabelTokenSupportsLabel(left, label) ||
			preEmitTypedLabelTokenSupportsLabel(right, label) {
			return true
		}
		for _, rightDisplay := range preEmitAggregateMemberDisplayCandidates(right) {
			if preEmitTypedLabelTokenSupportsLabel(rightDisplay, label) ||
				preEmitTypedLabelTokenSupportsLabel(label, rightDisplay) {
				return true
			}
		}
	}
	return false
}

func preEmitAggregateMemberLabelTextMatches(label, text, member string) bool {
	member = strings.TrimSpace(member)
	if member == "" {
		return false
	}
	if preEmitDecoratedAggregateMemberAppearsInText(member, strings.Join([]string{label, text}, "\n")) {
		return true
	}
	for _, candidate := range preEmitAggregateMemberDisplayCandidates(member) {
		if preEmitTypedLabelTokenSupportsLabel(candidate, label) ||
			preEmitTypedLabelTokenSupportsLabel(label, candidate) {
			return true
		}
	}
	for _, surface := range preEmitAggregateMemberRelationSurfaces(member) {
		left, right, ok := preEmitAggregateMemberLabelRelationParts(surface)
		if !ok {
			continue
		}
		if !preEmitTypedLabelTokenSupportsLabel(left, label) {
			for _, rightDisplay := range preEmitAggregateMemberDisplayCandidates(right) {
				if preEmitTypedLabelTokenSupportsLabel(rightDisplay, label) &&
					preEmitAggregateDisplayPartAppears(left, text) {
					return true
				}
			}
			continue
		}
		for _, rightDisplay := range preEmitAggregateMemberDisplayCandidates(right) {
			if preEmitAggregateDisplayPartAppears(rightDisplay, text) {
				return true
			}
		}
	}
	return false
}

func preEmitDecoratedAggregateMemberAppearsInText(member, surface string) bool {
	base, qualifier, ok := types.AnswerAggregateDecoratedLabelParts(member)
	if !ok {
		return false
	}
	if !preEmitAggregateDisplayPartAppears(base, surface) &&
		!types.CodeSurfaceAppearsAsToken(base, surface) {
		return false
	}
	return preEmitDecoratedQualifierAppearsInText(qualifier, surface)
}

func preEmitDecoratedQualifierAppearsInText(qualifier, surface string) bool {
	qualifier = strings.TrimSpace(qualifier)
	if qualifier == "" {
		return true
	}
	if preEmitAggregateDisplayPartAppears(qualifier, surface) ||
		types.CodeSurfaceAppearsAsToken(qualifier, surface) {
		return true
	}
	parts := preEmitDecoratedQualifierParts(qualifier)
	if len(parts) <= 1 {
		return false
	}
	for _, part := range parts {
		if !preEmitAggregateDisplayPartAppears(part, surface) &&
			!types.CodeSurfaceAppearsAsToken(part, surface) {
			return false
		}
	}
	return true
}

func preEmitDecoratedQualifierParts(qualifier string) []string {
	fields := strings.FieldsFunc(qualifier, func(r rune) bool {
		switch r {
		case '/', '|', ',', '，', ';', '；':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func preEmitAggregateMemberLabelRelationParts(member string) (left string, right string, ok bool) {
	if left, right, ok := types.AnswerAggregateMemberRelationParts(member); ok {
		return left, right, true
	}
	member = strings.TrimSpace(strings.Trim(member, "`\"' "))
	if strings.Count(member, ".") != 1 || strings.ContainsAny(member, `/\`) {
		return "", "", false
	}
	parts := strings.Split(member, ".")
	left, right = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if !preEmitAggregateSimpleRelationPartOK(left) || !preEmitAggregateSimpleRelationPartOK(right) {
		return "", "", false
	}
	return left, right, true
}

func preEmitAggregateSimpleRelationPartOK(part string) bool {
	part = strings.TrimSpace(part)
	if part == "" {
		return false
	}
	hasAlphaNum := false
	for _, r := range part {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9':
			hasAlphaNum = true
		case r == '_' || r == '-' || r == '$':
		default:
			return false
		}
	}
	return hasAlphaNum
}

func preEmitAggregateMemberCitationMatches(fact types.AnswerAggregateFact, memberIdx int, member string, cit types.Citation) bool {
	if strings.TrimSpace(cit.File) == "" || cit.Line <= 0 {
		return false
	}
	if surface, ok := types.ParseAnswerSourceLocationSurface(member); ok {
		return preEmitCitationMatchesSourceLocation(cit, surface)
	}
	if label, loc, ok := preEmitAggregateSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
		return preEmitCitationMatchesSourceLocation(cit, loc)
	}
	memberKey := strings.ToLower(strings.TrimSpace(member))
	var bareRefs []types.AnswerSourceLocationSurface
	var genericRefs []types.AnswerSourceLocationSurface
	for _, ref := range fact.SupportRefs {
		refMember, loc, ok := preEmitAggregateSupportRefMemberLocation(ref)
		if !ok {
			continue
		}
		if refMember == "" {
			bareRefs = append(bareRefs, loc)
			continue
		}
		if types.AnswerSupportRefLabelIsGeneric(refMember) {
			genericRefs = append(genericRefs, loc)
			if preEmitCitationMatchesSourceLocation(cit, loc) {
				return true
			}
			continue
		}
		if strings.ToLower(refMember) == memberKey && preEmitCitationMatchesSourceLocation(cit, loc) {
			return true
		}
	}
	if len(genericRefs) == len(fact.Members) && memberIdx >= 0 && memberIdx < len(genericRefs) {
		return preEmitCitationMatchesSourceLocation(cit, genericRefs[memberIdx])
	}
	if len(genericRefs) == 1 && len(fact.Members) == 1 {
		return preEmitCitationMatchesSourceLocation(cit, genericRefs[0])
	}
	if len(bareRefs) == len(fact.Members) && memberIdx >= 0 && memberIdx < len(bareRefs) {
		return preEmitCitationMatchesSourceLocation(cit, bareRefs[memberIdx])
	}
	if len(bareRefs) == 1 {
		return preEmitCitationMatchesSourceLocation(cit, bareRefs[0])
	}
	return false
}

func preEmitAggregateMemberSupportLocation(fact types.AnswerAggregateFact, memberIdx int, member string) (source string, line int, ok bool) {
	if surface, parsed := types.ParseAnswerSourceLocationSurface(member); parsed {
		return surface.File, surface.LineStart, true
	}
	if label, loc, parsed := preEmitAggregateSupportRefMemberLocation(member); parsed && strings.TrimSpace(label) != "" {
		return loc.File, loc.LineStart, true
	}
	memberKey := strings.ToLower(strings.TrimSpace(member))
	var bareRefs []types.AnswerSourceLocationSurface
	var genericRefs []types.AnswerSourceLocationSurface
	for _, ref := range fact.SupportRefs {
		refMember, loc, parsed := preEmitAggregateSupportRefMemberLocation(ref)
		if !parsed {
			continue
		}
		if strings.TrimSpace(refMember) == "" {
			bareRefs = append(bareRefs, loc)
			continue
		}
		if types.AnswerSupportRefLabelIsGeneric(refMember) {
			genericRefs = append(genericRefs, loc)
			continue
		}
		if strings.ToLower(strings.TrimSpace(refMember)) == memberKey {
			return loc.File, loc.LineStart, true
		}
	}
	if len(genericRefs) == len(fact.Members) && memberIdx >= 0 && memberIdx < len(genericRefs) {
		loc := genericRefs[memberIdx]
		return loc.File, loc.LineStart, true
	}
	if len(genericRefs) == 1 && len(fact.Members) == 1 {
		loc := genericRefs[0]
		return loc.File, loc.LineStart, true
	}
	if len(bareRefs) == len(fact.Members) && memberIdx >= 0 && memberIdx < len(bareRefs) {
		loc := bareRefs[memberIdx]
		return loc.File, loc.LineStart, true
	}
	if len(bareRefs) == 1 {
		loc := bareRefs[0]
		return loc.File, loc.LineStart, true
	}
	return "", 0, false
}

func preEmitCitationMatchesAggregateEvidence(ctx *types.BusContext, member string, cit types.Citation) bool {
	return preEmitCitationMatchesAggregateEvidenceWithContext(newPreEmitCheckContext(ctx), member, cit)
}

func preEmitCitationMatchesAggregateEvidenceWithContext(pctx *preEmitCheckContext, member string, cit types.Citation) bool {
	evidence, found := pctx.citedEvidenceItems(cit)
	if !found {
		return false
	}
	for _, ev := range evidence {
		if preEmitEvidenceSupportsAggregateMemberCitation(ev, member) {
			return true
		}
	}
	return false
}

func preEmitEvidenceSupportsAggregateMemberCitation(ev types.EvidenceItem, member string) bool {
	if left, right, ok := preEmitAggregateMemberLabelRelationParts(member); ok {
		return preEmitEvidenceEndpointSupportsAnyAggregateCandidate(ev, right) &&
			preEmitEvidenceOrSourceSupportsRelationLeft(ev, left)
	}
	if preEmitDecoratedLabelMatchesEvidence(member, ev) {
		return true
	}
	return preEmitEvidenceSupportsAggregateMember(ev, member)
}

func preEmitEvidenceSupportsAggregateMember(ev types.EvidenceItem, member string) bool {
	if left, right, ok := preEmitAggregateMemberLabelRelationParts(member); ok {
		return preEmitEvidenceEndpointSupportsAnyAggregateCandidate(ev, left) &&
			preEmitEvidenceEndpointSupportsAnyAggregateCandidate(ev, right)
	}
	if preEmitDecoratedLabelMatchesEvidence(member, ev) {
		return true
	}
	for _, candidate := range preEmitAggregateMemberDisplayCandidates(member) {
		if preEmitLabelMatchesEvidenceEndpoint(candidate, ev) {
			return true
		}
	}
	return false
}

func preEmitEvidenceEndpointSupportsAnyAggregateCandidate(ev types.EvidenceItem, member string) bool {
	for _, candidate := range preEmitAggregateMemberDisplayCandidates(member) {
		if preEmitEvidenceEndpointSupportsToken(ev, candidate) {
			return true
		}
	}
	return false
}

func preEmitEvidenceOrSourceSupportsRelationLeft(ev types.EvidenceItem, left string) bool {
	left = strings.TrimSpace(left)
	if left == "" {
		return false
	}
	if preEmitEvidenceEndpointSupportsToken(ev, left) {
		return true
	}
	return preEmitPathSegmentsSupportToken(ev.Source, left)
}

func preEmitPathSegmentsSupportToken(path string, token string) bool {
	path = strings.TrimSpace(strings.ReplaceAll(path, `\`, `/`))
	token = strings.TrimSpace(token)
	if path == "" || token == "" {
		return false
	}
	want := strings.ToLower(strings.Trim(token, "`\"' "))
	wantKey := preEmitCodeIdentityKey(token)
	for _, segment := range strings.Split(path, "/") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		base := segment
		if dot := strings.LastIndex(base, "."); dot > 0 {
			base = base[:dot]
		}
		for _, candidate := range []string{segment, base} {
			if strings.ToLower(candidate) == want {
				return true
			}
			if wantKey != "" && preEmitCodeIdentityKey(candidate) == wantKey {
				return true
			}
		}
	}
	return false
}

func preEmitCodeIdentityKey(surface string) string {
	surface = strings.Trim(strings.TrimSpace(surface), "`\"' ")
	if surface == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(surface))
	for _, r := range surface {
		switch {
		case r == '_' || r == '-':
			continue
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func preEmitEvidenceEndpointSupportsToken(ev types.EvidenceItem, token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	for _, endpoint := range []string{ev.Subject, ev.Object, ev.AnchorSymbol, ev.OwnerSymbol} {
		if preEmitTypedLabelTokenSupportsLabel(endpoint, token) ||
			preEmitTypedLabelTokenSupportsLabel(token, endpoint) {
			return true
		}
	}
	for _, term := range ev.SurfaceTerms {
		if preEmitTypedLabelTokenSupportsLabel(term, token) ||
			preEmitTypedLabelTokenSupportsLabel(token, term) {
			return true
		}
	}
	return preEmitCodeSurfaceAppearsVerbatim(token, ev.Snippet)
}

func preEmitAggregateSupportRefMemberLocation(ref string) (member string, location types.AnswerSourceLocationSurface, ok bool) {
	return types.ParseAnswerSupportRefMemberLocation(ref)
}

func preEmitCitationMatchesSourceLocation(cit types.Citation, loc types.AnswerSourceLocationSurface) bool {
	return cit.Line == loc.LineStart && preEmitPathMatches(cit.File, loc.File)
}

func preEmitLabelSupportedByQuestionBucket(label string, ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	for _, bucket := range ctx.AnalysisIR.RequestModel.QuestionStructure().Buckets {
		if preEmitTypedLabelTokenSupportsLabel(bucket.Label, label) {
			return true
		}
	}
	return false
}

func preEmitLabelSupportedByRuntimeArtifact(label string, ctx *types.BusContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	if bundle := ctx.Mutable.LogTriage(); bundle != nil {
		if preEmitRuntimeLabelSupportedByLogBundle(label, bundle) {
			return true
		}
	}
	if perf := ctx.Mutable.PerfTrace(); perf != nil {
		for _, frame := range perf.LogFrames() {
			if preEmitRuntimeLabelSupportedByFrame(label, frame) {
				return true
			}
		}
	}
	return false
}

func preEmitRuntimeLabelSupportedByLogBundle(label string, bundle *types.LogBundle) bool {
	if bundle == nil {
		return false
	}
	for _, signal := range bundle.Meta.Signals {
		if preEmitTypedLabelTokenSupportsLabel(string(signal), label) {
			return true
		}
	}
	matched := false
	types.WalkLogErrors(bundle, func(err *types.LogError) {
		if matched || err == nil {
			return
		}
		if preEmitTypedLabelTokenSupportsLabel(err.Type, label) ||
			preEmitTypedLabelTokenSupportsLabel(err.Message, label) {
			matched = true
		}
	})
	if matched {
		return true
	}
	types.WalkLogFrames(bundle, func(frame types.LogFrame) {
		if matched {
			return
		}
		if preEmitRuntimeLabelSupportedByFrame(label, frame) {
			matched = true
		}
	})
	return matched
}

func preEmitRuntimeLabelSupportedByFrame(label string, frame types.LogFrame) bool {
	return preEmitTypedLabelTokenSupportsLabel(frame.Lang, label) ||
		preEmitTypedLabelTokenSupportsLabel(frame.Func, label) ||
		preEmitTypedLabelTokenSupportsLabel(frame.Pkg, label) ||
		preEmitTypedLabelTokenSupportsLabel(frame.File, label) ||
		preEmitTypedLabelTokenSupportsLabel(frame.Raw, label)
}

func preEmitTypedLabelTokenSupportsLabel(token, label string) bool {
	token = strings.TrimSpace(token)
	label = strings.TrimSpace(label)
	if token == "" || label == "" {
		return false
	}
	t := strings.ToLower(token)
	l := strings.ToLower(label)
	if l == t {
		return true
	}
	if strings.HasPrefix(l, t) && preEmitLabelBoundaryAfter(l, len(t)) {
		return true
	}
	if strings.HasPrefix(t, l) && preEmitLabelBoundaryAfter(t, len(l)) {
		return true
	}
	return false
}

func preEmitLabelBoundaryAfter(s string, n int) bool {
	if n >= len(s) {
		return true
	}
	if s[n] >= 0x80 {
		return true
	}
	switch s[n] {
	case ' ', '\t', '\r', '\n', '.', ':', '/', '\\', '-', '_', '(', ')', '[', ']', '{', '}', ',', ';':
		return true
	}
	return false
}

// preEmitLabelLeadingIdentifier extracts the leading
// identifier-shape token from a label, mirroring
// labelLeadingSymbolIdentifier at internal/orchestrator/contract_check_block.go:1782.
// Kept local to avoid an internal/orchestrator dependency in the
// internal/tool package (would create an import cycle).
//
// Behaviour parity (verbatim mirror, see post-emit comment for
// rationale): returns "" when the label is prose ("Step 1: ..."),
// when the leading run is too short, or when no identifier prefix
// is present.
func preEmitLabelLeadingIdentifier(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	// Identifier scan: collect leading [A-Za-z_][A-Za-z0-9_.]* run.
	end := 0
	for i, r := range label {
		if i == 0 {
			if !(r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
				return ""
			}
			end = 1
			continue
		}
		if r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' {
			end = i + 1
			continue
		}
		break
	}
	if end == 0 {
		return ""
	}
	return label[:end]
}

// preEmitLabelStartsWithQualifiedCodeIdentity mirrors the post-emit
// contract checker's qualified-identity bypass. Package/module/
// namespace-qualified labels are validated through citation alignment
// and evidence endpoints; the declaration oracle is only precise for
// standalone identifiers.
func preEmitLabelStartsWithQualifiedCodeIdentity(label string) bool {
	token := preEmitLeadingCodeIdentityToken(label)
	if token == "" {
		return false
	}
	if strings.HasPrefix(token, "@") {
		return true
	}
	return strings.Contains(token, ".") ||
		strings.Contains(token, "::") ||
		strings.Contains(token, "/") ||
		strings.Contains(token, `\`)
}

func preEmitLeadingCodeIdentityToken(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	end := 0
	for end < len(label) {
		c := label[end]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_', c == '.', c == ':', c == '/', c == '\\', c == '-', c == '@':
			end++
			continue
		default:
			goto done
		}
	}
done:
	if end == 0 {
		return ""
	}
	token := strings.Trim(label[:end], ":-")
	if token == "" || !types.IsCodeIdentitySurface(token) {
		return ""
	}
	return token
}

// preEmitLabelLengthFloor mirrors labelHallucinationGateLengthFloor
// at internal/orchestrator/contract_check_block.go:1825. Identifiers
// shorter than this are stdlib-helper false positives (Sprintf,
// Println, New, Get) — too noisy to gate on. Kept local for the same
// import-cycle reason as preEmitLabelLeadingIdentifier.
const preEmitLabelLengthFloor = 10

// preCheckRequiredBlocks mirrors the post-emit
// validateRequiredBlockCoverage core logic (block kind+count) but
// emits user-vocab fix-list entries instead of types.Violation.
func preCheckRequiredBlocks(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	if len(view.RequiredBlocks) == 0 {
		return nil
	}
	var out []emitFixHint
	for _, req := range view.RequiredBlocks {
		if !req.Required {
			continue
		}
		got := types.CountAnswerBlocksForRequirement(doc.Blocks, req)
		kindLabel := req.AcceptedKindsLabel()
		if kindLabel == "" {
			kindLabel = string(req.Kind)
		}
		if got < req.MinCount {
			out = append(out, emitFixHint{
				Field: fmt.Sprintf("blocks[].kind=%s", req.Kind),
				ExpectedShape: fmt.Sprintf(
					"emit at least %d block(s) of kind=%s (currently emitted: %d)",
					req.MinCount, kindLabel, got),
				Reason: strings.TrimSpace(req.Rationale),
			})
			continue
		}
		if req.MaxCount > 0 && got > req.MaxCount {
			out = append(out, emitFixHint{
				Field: fmt.Sprintf("blocks[].kind=%s", req.Kind),
				ExpectedShape: fmt.Sprintf(
					"reduce kind=%s blocks to at most %d (currently emitted: %d)",
					kindLabel, req.MaxCount, got),
				Reason: strings.TrimSpace(req.Rationale),
			})
		}
	}
	return out
}

// preCheckPrincipalClaimUse mirrors validatePrincipalClaimUse's
// "principal block lacks claim_use annotation" branch + the
// 2026-05-04 single-AcceptableClaimForm relaxation that lets blocks
// with structural grounding (facet_ids non-empty + at least one
// item with citation_ref >= 0) skip the explicit claim_uses
// annotation. Returning a fix hint here directs the LLM to emit
// claim_uses[] before persist; without this the LLM ships and we
// pay a full orchestrator retry round.
func preCheckPrincipalClaimUse(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	reqByKind := make(map[types.AnswerBlockKind]types.BlockRequirement, len(view.RequiredBlocks))
	for _, r := range view.RequiredBlocks {
		for _, kind := range r.AcceptedKinds() {
			if _, ok := reqByKind[kind]; !ok {
				reqByKind[kind] = r
			}
		}
	}
	var out []emitFixHint
	for _, b := range doc.Blocks {
		req, ok := reqByKind[b.Kind]
		if !ok {
			continue
		}
		if len(req.AcceptableClaimForms) == 0 {
			continue
		}
		isPrincipal := b.SurfaceRole == types.SurfacePrincipal ||
			(b.SurfaceRole == "" && req.SurfaceRoleHint == types.SurfacePrincipal)
		if !isPrincipal {
			continue
		}
		if hasAnyClaimUse(b) {
			continue
		}
		if types.AnswerBlockHasActiveTypedDecisionCarrier(b, view) {
			continue
		}
		// Single-form relaxation: when the contract declares exactly
		// one AcceptableClaimForm AND the block already carries
		// structural grounding, the LLM's emit is unambiguously
		// correct even without the explicit claim_uses[]. Skip
		// the fix hint in that path.
		if len(req.AcceptableClaimForms) == 1 && hasStructuralGrounding(b) {
			continue
		}
		formNames := make([]string, 0, len(req.AcceptableClaimForms))
		for _, f := range req.AcceptableClaimForms {
			formNames = append(formNames, fmt.Sprintf("%q", string(f)))
		}
		out = append(out, emitFixHint{
			Field: fmt.Sprintf("blocks[id=%q].claim_uses", b.ID),
			ExpectedShape: fmt.Sprintf(
				"add a one-element claim_uses[] array on this principal block; claim_form must be one of [%s]",
				strings.Join(formNames, ", ")),
			Reason: "principal blocks need a claim annotation so the answer's typed payload can be matched to its evidence shape",
		})
	}
	return out
}

// normalizeViewCompatibleAnswerDocument applies deterministic compatibility
// repairs that are fully implied by the typed AnswerSemanticView. The normalizer
// is deliberately narrow: it may remove fields from inactive typed lanes, but it
// must not infer semantic answer content from user prose or model prose.
func normalizeViewCompatibleAnswerDocument(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) int {
	if doc == nil || view == nil {
		return 0
	}
	fixed := 0
	fixed += normalizeInactiveTypedDecisionVerdictFields(doc, view)
	fixed += normalizeExcessRequiredSummaryBlocks(doc, view)
	return fixed
}

func normalizeInactiveTypedDecisionVerdictFields(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) int {
	if doc == nil || view == nil {
		return 0
	}
	errorGranularityActive := view.ErrorGranularityProfile != nil && view.ErrorGranularityProfile.Active()
	currentStatusActive := view.CurrentStatusDiagnostic != nil && view.CurrentStatusDiagnostic.Required
	fixed := 0
	for i := range doc.Blocks {
		if doc.Blocks[i].Kind != types.BlockDecision {
			continue
		}
		if !errorGranularityActive && doc.Blocks[i].ErrorGranularityVerdict != "" {
			doc.Blocks[i].ErrorGranularityVerdict = ""
			fixed++
		}
		if !currentStatusActive && doc.Blocks[i].CurrentStatusVerdict != "" {
			doc.Blocks[i].CurrentStatusVerdict = ""
			fixed++
		}
	}
	return fixed
}

func normalizeExcessRequiredSummaryBlocks(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) int {
	if doc == nil || view == nil || !viewRequiresSingleSummaryBlock(view) {
		return 0
	}
	lead := -1
	fixed := 0
	for i := 0; i < len(doc.Blocks); i++ {
		if doc.Blocks[i].Kind != types.BlockSummary {
			continue
		}
		if lead < 0 {
			lead = i
			continue
		}
		mergeSummaryBlock(&doc.Blocks[lead], doc.Blocks[i])
		doc.Blocks = append(doc.Blocks[:i], doc.Blocks[i+1:]...)
		i--
		fixed++
	}
	return fixed
}

func viewRequiresSingleSummaryBlock(view *types.AnswerSemanticView) bool {
	if view == nil {
		return false
	}
	for _, req := range view.RequiredBlocks {
		if req.Required && req.Kind == types.BlockSummary && req.MaxCount == 1 {
			return true
		}
	}
	return false
}

func mergeSummaryBlock(dst *types.AnswerBlock, src types.AnswerBlock) {
	if dst == nil {
		return
	}
	if strings.TrimSpace(dst.Title) == "" {
		dst.Title = src.Title
	}
	dst.Text = mergeSummaryText(dst.Text, src.Text)
	dst.Items = append(dst.Items, src.Items...)
	dst.FacetIDs = mergeStringSet(dst.FacetIDs, src.FacetIDs)
	dst.ClaimUses = mergeRenderedClaimUses(dst.ClaimUses, src.ClaimUses)
	dst.EdgeAnchors = mergeDiagramEdgeAnchors(dst.EdgeAnchors, src.EdgeAnchors)
	if dst.SurfaceRole == "" {
		dst.SurfaceRole = src.SurfaceRole
	}
}

func mergeSummaryText(a, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	switch {
	case a == "":
		return b
	case b == "":
		return a
	case a == b:
		return a
	default:
		return a + "\n\n" + b
	}
}

func mergeStringSet(dst, src []string) []string {
	seen := make(map[string]bool, len(dst)+len(src))
	out := make([]string, 0, len(dst)+len(src))
	for _, value := range append(append([]string(nil), dst...), src...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func mergeRenderedClaimUses(dst, src []types.RenderedClaimUse) []types.RenderedClaimUse {
	seen := make(map[types.RenderedClaimUse]bool, len(dst)+len(src))
	out := make([]types.RenderedClaimUse, 0, len(dst)+len(src))
	for _, value := range append(append([]types.RenderedClaimUse(nil), dst...), src...) {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func mergeDiagramEdgeAnchors(dst, src []types.DiagramEdgeAnchor) []types.DiagramEdgeAnchor {
	seen := make(map[types.DiagramEdgeAnchor]bool, len(dst)+len(src))
	out := make([]types.DiagramEdgeAnchor, 0, len(dst)+len(src))
	for _, value := range append(append([]types.DiagramEdgeAnchor(nil), dst...), src...) {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func preCheckRequiredCandidateRoles(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	if doc == nil || view == nil || len(view.RequiredCandidateRoles) == 0 {
		return nil
	}
	missing := types.MissingRequiredCandidateRoles(doc, view.RequiredCandidateRoles)
	if len(missing) == 0 {
		return nil
	}
	roles := make([]string, 0, len(missing))
	for _, role := range missing {
		roles = append(roles, string(role))
	}
	return []emitFixHint{{
		Field: "blocks[].items[].candidate_role",
		ExpectedShape: "principal scalar/list/table item(s) must carry `candidate_role` for the requested answer role(s): " +
			strings.Join(roles, ", "),
		Reason: "the typed request contract requires these positive answer roles; prose-only wording or adjacent roles cannot satisfy a structural role-binding request.",
	}}
}

func preCheckRequiredMechanismAnchors(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	if doc == nil || view == nil || len(view.RequiredMechanismAnchors) == 0 {
		return nil
	}
	missing := types.MissingRequiredMechanismAnchors(doc, view.RequiredMechanismAnchors)
	if len(missing) == 0 {
		return nil
	}
	labels := make([]string, 0, len(missing))
	for _, anchor := range missing {
		labels = append(labels, anchor.Text)
	}
	return []emitFixHint{{
		Field:         "blocks[].items[].label",
		ExpectedShape: "structured answer anchor label(s) must preserve: " + strings.Join(labels, ", ") + ". Add or keep an ordered_list/table item whose label is exactly each missing anchor; use a matching citation_ref when available, otherwise citation_ref=-1.",
		Reason:        "the typed mechanism-anchor contract requires exact endpoint anchors in structured fields; summary prose alone cannot satisfy this boundary.",
	}}
}

func normalizeRequiredMechanismAnchorCarriers(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctx *types.BusContext) int {
	return normalizeRequiredMechanismAnchorCarriersWithContext(doc, view, ctx, newPreEmitCheckContext(ctx))
}

func normalizeRequiredMechanismAnchorCarriersWithContext(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctx *types.BusContext, pctx *preEmitCheckContext) int {
	if doc == nil || view == nil || len(view.RequiredMechanismAnchors) == 0 {
		return 0
	}
	missing := types.MissingRequiredMechanismAnchors(doc, view.RequiredMechanismAnchors)
	if len(missing) == 0 {
		return 0
	}
	blockIdx := ensureRequiredMechanismAnchorBlock(doc)
	if blockIdx < 0 || blockIdx >= len(doc.Blocks) {
		return 0
	}
	added := 0
	for _, anchor := range missing {
		label := strings.TrimSpace(anchor.Text)
		if label == "" {
			continue
		}
		item := types.AnswerBlockItem{
			ID:          nextRequiredMechanismAnchorItemID(doc.Blocks[blockIdx], label),
			Label:       label,
			CitationRef: citationRefForRequiredMechanismAnchorWithContext(doc, pctx, label),
		}
		doc.Blocks[blockIdx].Items = append(doc.Blocks[blockIdx].Items, item)
		added++
	}
	return added
}

func ensureRequiredMechanismAnchorBlock(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return -1
	}
	for i, block := range doc.Blocks {
		if block.ID == "required_mechanism_anchors" {
			return i
		}
	}
	block := types.AnswerBlock{
		ID:    uniqueRequiredMechanismAnchorBlockID(doc),
		Kind:  types.BlockOrderedList,
		Title: "Key anchors",
		FacetIDs: []string{
			string(types.FacetCurrentCodePath),
		},
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimDefinitionFact,
			FacetID:   string(types.FacetCurrentCodePath),
		}},
	}
	doc.Blocks = append(doc.Blocks, block)
	return len(doc.Blocks) - 1
}

func uniqueRequiredMechanismAnchorBlockID(doc *types.AnswerDocumentV2) string {
	const base = "required_mechanism_anchors"
	if doc == nil {
		return base
	}
	used := make(map[string]bool, len(doc.Blocks))
	for _, block := range doc.Blocks {
		id := strings.TrimSpace(block.ID)
		if id != "" {
			used[id] = true
		}
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		id := fmt.Sprintf("%s_%d", base, i)
		if !used[id] {
			return id
		}
	}
}

func nextRequiredMechanismAnchorItemID(block types.AnswerBlock, label string) string {
	base := "anchor_" + sanitizeRequiredMechanismAnchorID(label)
	if base == "anchor_" {
		base = "anchor"
	}
	used := make(map[string]bool, len(block.Items))
	for _, item := range block.Items {
		id := strings.TrimSpace(item.ID)
		if id != "" {
			used[id] = true
		}
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		id := fmt.Sprintf("%s_%d", base, i)
		if !used[id] {
			return id
		}
	}
}

func sanitizeRequiredMechanismAnchorID(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range label {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func citationRefForRequiredMechanismAnchor(doc *types.AnswerDocumentV2, ctx *types.BusContext, label string) int {
	return citationRefForRequiredMechanismAnchorWithContext(doc, newPreEmitCheckContext(ctx), label)
}

func citationRefForRequiredMechanismAnchorWithContext(doc *types.AnswerDocumentV2, pctx *preEmitCheckContext, label string) int {
	if doc == nil {
		return -1
	}
	for i, cit := range doc.Citations {
		if preEmitItemCitationAlignedWithContext(pctx, label, "", cit) {
			return i
		}
	}
	for _, loc := range preEmitCandidateCitationLocationsForLabelWithContext(pctx, label, 4) {
		file, line, ok := parsePreEmitLocation(loc)
		if !ok {
			continue
		}
		for i, cit := range doc.Citations {
			if cit.Line == line && preEmitPathMatches(cit.File, file) {
				return i
			}
		}
		doc.Citations = append(doc.Citations, types.Citation{File: file, Line: line})
		return len(doc.Citations) - 1
	}
	return -1
}

func parsePreEmitLocation(loc string) (string, int, bool) {
	loc = strings.TrimSpace(loc)
	if loc == "" {
		return "", 0, false
	}
	idx := strings.LastIndex(loc, ":")
	if idx <= 0 || idx == len(loc)-1 {
		return "", 0, false
	}
	file := strings.TrimSpace(loc[:idx])
	lineRaw := strings.TrimSpace(loc[idx+1:])
	line, err := strconv.Atoi(lineRaw)
	if err != nil || line <= 0 || file == "" {
		return "", 0, false
	}
	return file, line, true
}

func preCheckInactiveTypedDecisionVerdicts(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	if doc == nil || view == nil {
		return nil
	}
	errorGranularityActive := view.ErrorGranularityProfile != nil && view.ErrorGranularityProfile.Active()
	currentStatusActive := view.CurrentStatusDiagnostic != nil && view.CurrentStatusDiagnostic.Required
	for _, block := range doc.Blocks {
		if block.Kind != types.BlockDecision {
			continue
		}
		id := strings.TrimSpace(block.ID)
		if !errorGranularityActive && block.ErrorGranularityVerdict != "" {
			field := "blocks[kind=decision].error_granularity_verdict"
			if id != "" {
				field = fmt.Sprintf("blocks[id=%q].error_granularity_verdict", id)
			}
			return []emitFixHint{{
				Field:         field,
				ExpectedShape: "omit `error_granularity_verdict` unless the typed error-granularity contract is active for this request",
				Reason:        "typed decision verdict fields are lane-specific; an inactive failure-scope verdict must not share the current-status decision surface.",
			}}
		}
		if !currentStatusActive && block.CurrentStatusVerdict != "" {
			field := "blocks[kind=decision].current_status_verdict"
			if id != "" {
				field = fmt.Sprintf("blocks[id=%q].current_status_verdict", id)
			}
			return []emitFixHint{{
				Field:         field,
				ExpectedShape: "omit `current_status_verdict` unless the typed current-status diagnostic contract is active for this request",
				Reason:        "typed decision verdict fields are lane-specific; an inactive current-status verdict must not share unrelated decision surfaces.",
			}}
		}
	}
	return nil
}

func preCheckErrorGranularityVerdict(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	if doc == nil || view == nil || view.ErrorGranularityProfile == nil || !view.ErrorGranularityProfile.Active() {
		return nil
	}
	if !types.MissingErrorGranularityVerdict(doc, view.ErrorGranularityProfile) {
		if verdict, mismatch := types.ErrorGranularityVerdictOptionMismatch(doc, view.ErrorGranularityProfile); mismatch {
			options := make([]string, 0, len(view.ErrorGranularityProfile.RequestedVerdictOptions))
			for _, option := range view.ErrorGranularityProfile.RequestedVerdictOptions {
				options = append(options, string(option))
			}
			options = append(options, string(types.ErrorGranularityNotEnoughEvidence))
			return []emitFixHint{{
				Field: "blocks[].error_granularity_verdict",
				ExpectedShape: fmt.Sprintf(
					"replace `error_granularity_verdict=%s` with one of the request's typed verdict options: %s",
					verdict, strings.Join(options, ", ")),
				Reason: "the typed failure-scope request contract listed explicit verdict alternatives; use the most specific supported option instead of a broader umbrella verdict.",
			}}
		}
		return nil
	}
	return []emitFixHint{{
		Field: "blocks[].error_granularity_verdict",
		ExpectedShape: "principal `decision` block must set `error_granularity_verdict` to one of: " +
			strings.Join(errorGranularityVerdictValues(), ", "),
		Reason: "the typed failure-scope request contract requires a canonical decision verdict enum; prose-only wording does not satisfy it.",
	}}
}

func preCheckCurrentStatusVerdict(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	if doc == nil || view == nil || view.CurrentStatusDiagnostic == nil || !view.CurrentStatusDiagnostic.Required {
		return nil
	}
	if !types.MissingCurrentStatusVerdict(doc, view.CurrentStatusDiagnostic) {
		return nil
	}
	return []emitFixHint{{
		Field: "blocks[].current_status_verdict",
		ExpectedShape: "principal `decision` block must set `current_status_verdict` to one of: " +
			strings.Join(currentStatusVerdictValues(view.CurrentStatusDiagnostic), ", "),
		Reason: "the typed current-status diagnostic contract requires a canonical decision verdict enum; prose-only wording does not satisfy it.",
	}}
}

func currentStatusVerdictValues(contract *types.CurrentStatusDiagnosticContract) []string {
	allowed := types.CurrentStatusAllowedVerdicts(contract)
	out := make([]string, 0, len(allowed))
	for _, verdict := range allowed {
		out = append(out, string(verdict))
	}
	return out
}

// preCheckUncertaintyBlock checks the contract's uncertainty rules.
// Mirrors validateUncertaintyBlockPresence's load-bearing path:
// when an UncertaintyRule fires (resolved fact: a layer the user
// asked about has no grounded binding) and no caveat block sits in
// the doc, surface a fix hint.
//
// Conservative pre-emit shape: we fire the hint only when the
// view's UncertaintyRules slice is non-empty AND the doc has zero
// caveat blocks. The full post-emit validator runs the per-rule
// trigger logic; here we just want the LLM to emit the caveat at
// all, which is the most common miss.
func preCheckUncertaintyBlock(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	if len(view.UncertaintyRules) == 0 {
		return nil
	}
	hasCaveat := false
	for _, b := range doc.Blocks {
		if b.Kind == types.BlockCaveat {
			hasCaveat = true
			break
		}
	}
	if hasCaveat {
		return nil
	}
	return []emitFixHint{{
		Field:         "blocks[].kind=caveat",
		ExpectedShape: "emit a caveat block disclosing what was searched and what remained uncertain",
		Reason:        "the question's contract carries uncertainty rules that require explicit boundary disclosure",
	}}
}

// preCheckFacetCoverage mirrors the emit-time hard subset of the
// post-emit facet coverage check. Only facets that are hard by template
// are rejected here. Evidence-sufficient SOFT facets remain advisory:
// their absence may be recorded later, but it must not burn a finalizer
// retry when the answer already cites the underlying code.
func preCheckFacetCoverage(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	if view.FacetCoverage == nil || len(view.FacetCoverage.Required) == 0 {
		return nil
	}
	covered := make(map[string]bool, 8)
	for _, b := range doc.Blocks {
		for _, fid := range b.FacetIDs {
			covered[strings.TrimSpace(fid)] = true
		}
		for _, cu := range b.ClaimUses {
			if cu.FacetID != "" {
				covered[strings.TrimSpace(cu.FacetID)] = true
			}
		}
	}
	var out []emitFixHint
	for _, req := range view.FacetCoverage.Required {
		if !req.RequiresHardDeclaration() {
			continue
		}
		kind := strings.TrimSpace(string(req.Kind))
		if kind == "" || covered[kind] {
			continue
		}
		out = append(out, emitFixHint{
			Field: "blocks[].facet_ids OR blocks[].claim_uses[].facet_id",
			ExpectedShape: fmt.Sprintf(
				"declare facet_id=%q on at least one block whose payload covers this facet",
				kind),
			Reason: "the question's contract requires this facet to be anchored on the answer surface",
		})
	}
	return out
}

// hasAnyClaimUse reports whether the block carries at least one
// block-level claim_uses[] entry. Mirrors the post-emit
// blockHasClaimUse helper but stays package-local in tool/.
func hasAnyClaimUse(b types.AnswerBlock) bool {
	return len(b.ClaimUses) > 0
}

// hasStructuralGrounding reports whether the block already carries
// the structural grounding the single-AcceptableClaimForm relaxation
// recognises (facet_ids non-empty + at least one item with
// citation_ref >= 0). Mirrors hasGroundedCoverage in
// internal/orchestrator/contract_check_block.go.
func hasStructuralGrounding(b types.AnswerBlock) bool {
	if len(b.FacetIDs) == 0 {
		return false
	}
	for _, item := range b.Items {
		if item.CitationRef >= 0 {
			return true
		}
	}
	return false
}

// formatEmitFixHints renders a fix-hint list as a human-readable
// numbered list embedded in the failEmit rejection prose. The LLM
// reads it inside the tool-result and re-emits within the same
// dispatch (BaseAgent's emit-rejection signal handling, see
// emitAnswerDocumentRejectSignal in internal/agent).
//
// Red-line audit (R3/R4/R5/R6/R7/SST/CN+EN-only):
//   - R3 typed: each Field is a schema field name the LLM emits;
//     ExpectedShape names a concrete action; Reason gives the
//     structural why
//   - R4 generic: no project / case names; templated
//   - R5: rejection envelope, never writes answer body
//   - R6 no internal vocab: schema field names + user-vocab prose;
//     "the answer document" is the user-visible carrier name (already
//     in skill prompt); no ViolKind / ClusterKey / SuspectedRoot /
//     "orchestrator" / "pre-emit" architectural terms surface
//   - R7: actionable next steps the LLM can mechanically apply;
//     explicitly names the "same tool call" retry budget so the
//     LLM doesn't escalate to a heavier rewrite path
//   - SST: matches the existing failEmit message shape (e.g.
//     "blocks[3]: id is required ...")
//   - CN+EN-only: prompt is English, matches the surrounding
//     failEmit envelopes used by NormalizeEmitAnswerBlock
func formatEmitFixHints(hints []emitFixHint) string {
	if len(hints) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("The answer document does not yet meet the structural contract for this question. Apply each correction below and re-emit emit_answer_document in the SAME tool turn — this rejection is light-weight and does NOT consume a heavier rewrite round, just re-call with the fixes:\n\n")
	for i, h := range hints {
		fmt.Fprintf(&b, "  %d. Field: `%s`\n", i+1, h.Field)
		fmt.Fprintf(&b, "     Action: %s\n", h.ExpectedShape)
		if reason := strings.TrimSpace(h.Reason); reason != "" {
			fmt.Fprintf(&b, "     Why: %s\n", reason)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
