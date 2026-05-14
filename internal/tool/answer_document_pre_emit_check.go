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
	"strconv"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/analysis/contract"
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
	if doc == nil || view == nil {
		return nil
	}
	var hints []emitFixHint

	// 0. Citation-pool carrier integrity. Run this before semantic
	// member checks so a retry that dropped citations[] or references
	// an out-of-range index gets a direct schema repair instead of a
	// misleading "all principal members are missing" diagnosis.
	if h := preCheckCitationPoolIntegrity(doc); len(h) > 0 {
		hints = append(hints, h...)
	}
	if h := preCheckNegativeCitationBounds(doc); len(h) > 0 {
		hints = append(hints, h...)
	}
	if h := preCheckRuntimeObservationRepoContamination(doc, ctxOpt...); len(h) > 0 {
		hints = append(hints, h...)
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
	if h := preCheckErrorGranularityVerdict(doc, view); len(h) > 0 {
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
	if h := preCheckItemCitationAlignment(doc, view, ctxOpt...); len(h) > 0 {
		hints = append(hints, h...)
	}

	// 5b. Typed item/citation role alignment. An item that explicitly
	// asserts a typed evidence role must cite that same role, not a
	// nearby definition / guard / adjacent item that happens to share
	// one endpoint. The projection lives in types so new role shapes
	// (import/path/span/route/etc.) extend the central contract instead
	// of adding validator-specific patches.
	if h := preCheckCallChainItemCitationRoleAlignment(doc, view, ctxOpt...); len(h) > 0 {
		hints = append(hints, h...)
	}

	// 5c. Principal support member coverage. For enumeration answers,
	// every answer-grade member already selected into the principal
	// support lane must be rendered as a cited item/row; the finalizer
	// should not compress away explorer-emitted members.
	if h := preCheckPrincipalSupportMemberCoverage(doc, ctxOpt...); len(h) > 0 {
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
		hints = append(hints, h...)
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
	if h := preCheckEnumerationLabelGrounding(doc, oracle, ctxOpt...); len(h) > 0 {
		hints = append(hints, h...)
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

func answerDocumentVisibleText(doc *types.AnswerDocumentV2) string {
	if doc == nil {
		return ""
	}
	var b strings.Builder
	for _, block := range doc.Blocks {
		b.WriteString(block.Title)
		b.WriteByte('\n')
		b.WriteString(block.Text)
		b.WriteByte('\n')
		for _, item := range block.Items {
			b.WriteString(item.Label)
			b.WriteByte('\n')
			b.WriteString(item.Text)
			b.WriteByte('\n')
		}
		if block.Diagram != nil {
			b.WriteString(block.Diagram.Body)
			b.WriteByte('\n')
		}
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
		len(block.Items) > 0 ||
		block.Diagram != nil {
		return true
	}
	return false
}

func preCheckItemCitationAlignment(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctxOpt ...*types.BusContext) []emitFixHint {
	if doc == nil || len(ctxOpt) == 0 || ctxOpt[0] == nil || ctxOpt[0].Mutable == nil {
		return nil
	}
	ctx := ctxOpt[0]
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
			if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
				continue
			}
			cit := doc.Citations[item.CitationRef]
			if types.AnswerLocationLabelMatchesCitation(label, cit) {
				continue
			}
			if preEmitCitationSupportsAggregateItem(ctx, label, item.Text, cit) {
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
					candidates: preEmitCandidateCitationLocationsForLabel(ctx, label, 4),
				})
				continue
			}
			evidence, found := preEmitCitedEvidenceItems(ctx, cit)
			if !found {
				if candidates := preEmitCandidateCitationLocationsForAggregateItem(ctx, label, item.Text, 4); len(candidates) > 0 {
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
			if preEmitLabelMatchesAnyEvidenceEndpoint(label, evidence) {
				continue
			}
			if candidates := preEmitCandidateCitationLocationsForAggregateItem(ctx, label, item.Text, 4); len(candidates) > 0 {
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
				candidates: preEmitCandidateCitationLocationsForLabel(ctx, label, 4),
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

func preCheckCallChainItemCitationRoleAlignment(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctxOpt ...*types.BusContext) []emitFixHint {
	if doc == nil || len(ctxOpt) == 0 || ctxOpt[0] == nil || ctxOpt[0].Mutable == nil {
		return nil
	}
	ctx := ctxOpt[0]
	allEvidence := preEmitAnswerEvidenceItems(ctx)
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
			cited, found := preEmitCitedEvidenceItems(ctx, cit)
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
		ExpectedShape: "each item whose visible label/text names a typed evidence role must cite the evidence line for that same role: " +
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
		Field: "blocks[].text OR blocks[].items[].label/text",
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
	principalRefs := types.PrincipalAggregateMemberSetFactRefs(facts)
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
		Field: "blocks[].items[].label/text OR blocks[].text",
		ExpectedShape: "include every model-emitted principal member_set member in the visible answer: " +
			strings.Join(parts, "; "),
		Reason: "the investigation handed off this complete principal member set as structured data; finalization must preserve those model-authored members even when the request family was routed as architecture, scalar, relation, or generic prose.",
	}}
}

func preEmitAggregateMemberSetIsScalarCountSupport(ctx *types.BusContext, fact types.AnswerAggregateFact) bool {
	if ctx == nil || ctx.AnalysisIR == nil || fact.Kind != types.AnswerAggregateMemberSet || len(fact.Members) == 0 {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if !rm.Predicates.IsCountQuestion || !rm.Predicates.IsScalarAnswer {
		return false
	}
	if rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsCrossComponent ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	return rm.Intent == types.IntentReturnValue || rm.AnswerSubject.Kind == types.SubjectNumeric
}

func preCheckAggregateCardinalityConsistency(doc *types.AnswerDocumentV2, ctxOpt ...*types.BusContext) []emitFixHint {
	if doc == nil || len(ctxOpt) == 0 || ctxOpt[0] == nil || ctxOpt[0].Mutable == nil {
		return nil
	}
	refs := types.PrincipalAggregateMemberSetFactRefs(ctxOpt[0].Mutable.StableInvestigationAggregateFacts())
	if len(refs) == 0 {
		return nil
	}
	uniquePrincipalSet := len(refs) == 1
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
		for _, claim := range preEmitAggregateScopedCountClaims(doc, fact, uniquePrincipalSet) {
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
		ExpectedShape: "make every visible count claim for a model-emitted principal member_set equal the member_set cardinality: " +
			strings.Join(parts, "; "),
		Reason: "aggregate_facts.member_set is the authoritative model-authored principal set; final text may display it, but it must not introduce a different count for that same set.",
	}}
}

func preCheckRelationMemberSetAnswerShape(doc *types.AnswerDocumentV2, ctxOpt ...*types.BusContext) []emitFixHint {
	if doc == nil || len(ctxOpt) == 0 || ctxOpt[0] == nil || ctxOpt[0].Mutable == nil {
		return nil
	}
	refs := types.PrincipalRelationMemberSetFactRefs(ctxOpt[0].Mutable.StableInvestigationAggregateFacts())
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

func preEmitAggregateScopedCountClaims(doc *types.AnswerDocumentV2, fact types.AnswerAggregateFact, uniquePrincipalSet bool) []preEmitAggregateCountClaim {
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
		if !preEmitBlockBindsToAggregateCount(block, surface, fact, uniquePrincipalSet) {
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

func preEmitBlockBindsToAggregateCount(block types.AnswerBlock, surface string, fact types.AnswerAggregateFact, uniquePrincipalSet bool) bool {
	if preEmitAggregateDisplayPartAppears(strings.TrimSpace(fact.Label), surface) {
		return true
	}
	if !uniquePrincipalSet {
		return false
	}
	for _, member := range fact.Members {
		if preEmitAggregateMemberAppearsInText(member, surface) {
			return true
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
	return out
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
		itemSurface := strings.TrimSpace(item.Label + "\n" + item.Text)
		if itemSurface == "" {
			continue
		}
		if preEmitAggregateMemberAppearsInText(member, itemSurface) {
			return true
		}
	}
	return preEmitMultiTargetRelationCoveredByStructuredBlock(member, block)
}

func preEmitAggregateMemberAppearsInText(member string, surface string) bool {
	candidates := preEmitAggregateMemberDisplayCandidates(member)
	if len(candidates) == 0 {
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
			itemSurface := strings.TrimSpace(item.Label + "\n" + item.Text)
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
			if preEmitTextContainsAllAggregateParts(item.Label+"\n"+item.Text, left, right) {
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
		appendPreEmitSurface(&b, block.Title)
		appendPreEmitSurface(&b, block.Text)
		for _, item := range block.Items {
			appendPreEmitSurface(&b, item.Label)
			appendPreEmitSurface(&b, item.Text)
		}
		if block.Diagram != nil {
			appendPreEmitSurface(&b, block.Diagram.Body)
		}
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
			if req.Kind != b.Kind || len(req.AcceptableClaimForms) == 0 {
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
		if preEmitNormalizePath(ob.Source) != preEmitNormalizePath(labelFile) {
			return false
		}
		return preEmitNormalizePath(cit.File) == preEmitNormalizePath(labelFile)
	case types.ImpactOutputSites:
		surface, ok := types.ParseAnswerSourceLocationSurface(label)
		if !ok || !types.AnswerSourceLocationSurfaceMatchesCitation(surface, cit) {
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
	if preEmitNormalizeLocation(ob.Location) == want {
		return true
	}
	for _, loc := range ob.EquivalentLocations {
		if preEmitNormalizeLocation(loc) == want {
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

func preEmitAnswerEvidenceItems(ctx *types.BusContext) []types.EvidenceItem {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	var out []types.EvidenceItem
	if artifacts := ctx.Mutable.TurnAArtifacts(); artifacts != nil && len(artifacts.EvidenceItems) > 0 {
		out = append(out, artifacts.EvidenceItems...)
	}
	if emitted := ctx.Mutable.EmittedEvidence(); len(emitted) > 0 {
		out = append(out, emitted...)
	}
	return out
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
	text := strings.TrimSpace(item.Text)
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
	if ctx == nil || ctx.Mutable == nil {
		return nil, false
	}
	file := strings.TrimSpace(cit.File)
	if file == "" || cit.Line <= 0 {
		return nil, false
	}
	var out []types.EvidenceItem
	if artifacts := ctx.Mutable.TurnAArtifacts(); artifacts != nil {
		out = append(out, preEmitCitedEvidenceFromPool(artifacts.EvidenceItems, file, cit.Line)...)
	}
	if emitted := ctx.Mutable.EmittedEvidence(); len(emitted) > 0 {
		out = append(out, preEmitCitedEvidenceFromPool(emitted, file, cit.Line)...)
	}
	return out, len(out) > 0
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

func preEmitCandidateCitationLocationsForLabel(ctx *types.BusContext, label string, limit int) []string {
	if limit <= 0 {
		limit = 4
	}
	var out []string
	seen := make(map[string]bool)
	for _, ev := range preEmitAnswerEvidenceItems(ctx) {
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
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	if limit <= 0 {
		limit = 4
	}
	var out []string
	seen := make(map[string]bool)
	add := func(file string, line int) {
		file = strings.TrimSpace(file)
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
	for _, ref := range types.PrincipalAggregateMemberSetFactRefs(ctx.Mutable.StableInvestigationAggregateFacts()) {
		fact := ref.Fact
		for idx, member := range fact.Members {
			if !preEmitAggregateMemberLabelTextMatches(label, text, member) {
				continue
			}
			if source, line, ok := preEmitAggregateMemberSupportLocation(fact, idx, member); ok {
				add(source, line)
			}
			for _, ev := range preEmitAnswerEvidenceItems(ctx) {
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
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	label = strings.TrimSpace(label)
	text = strings.TrimSpace(text)
	if label == "" || text == "" {
		return false
	}
	for _, ref := range types.PrincipalAggregateMemberSetFactRefs(ctx.Mutable.StableInvestigationAggregateFacts()) {
		fact := ref.Fact
		for idx, member := range fact.Members {
			if !preEmitAggregateMemberLabelTextMatches(label, text, member) {
				continue
			}
			if preEmitAggregateMemberCitationMatches(fact, idx, member, cit) {
				return true
			}
			evidence, found := preEmitCitedEvidenceItems(ctx, cit)
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
	return ok && types.IsCodeIdentitySurface(base)
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
				preEmitCodeSurfaceAppearsVerbatim(part, ev.Snippet) {
				return true
			}
		}
		return true
	}
	part := parts[0]
	return preEmitEvidenceEndpointSupportsToken(ev, part) ||
		preEmitPathSegmentsSupportToken(ev.Source, part) ||
		preEmitCodeSurfaceAppearsVerbatim(part, ev.Snippet)
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
		for _, it := range b.Items {
			if it.CitationRef < 0 || it.CitationRef >= len(doc.Citations) {
				continue
			}
			cite := doc.Citations[it.CitationRef]
			missing := missingSurfaceTermsForItem(it, cite, evidence)
			if len(missing) == 0 {
				continue
			}
			field := fmt.Sprintf("blocks[id=%q].items[id=%q].text", b.ID, it.ID)
			if strings.TrimSpace(it.ID) == "" {
				field = fmt.Sprintf("blocks[id=%q].items[].text", b.ID)
			}
			hints = append(hints, emitFixHint{
				Field: field,
				ExpectedShape: "include these model-emitted surface_terms in the cited item text or label: " +
					strings.Join(missing, ", "),
				Reason: "the investigation explicitly structured these source-visible labels; final answer validation requires preserving them instead of letting the system invent or append them later.",
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
	hay := strings.ToLower(item.Label + "\n" + item.Text)
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
	a = strings.TrimPrefix(strings.TrimSpace(a), "./")
	b = strings.TrimPrefix(strings.TrimSpace(b), "./")
	return a != "" && b != "" && a == b
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
	hay := strings.ToLower(item.Label + "\n" + item.Text)
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
	if doc == nil || oracle == nil {
		return nil
	}
	var ctx *types.BusContext
	if len(ctxOpt) > 0 {
		ctx = ctxOpt[0]
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
				types.AnswerLocationLabelMatchesCitation(label, doc.Citations[it.CitationRef]) {
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
			if ctx != nil && it.CitationRef >= 0 && it.CitationRef < len(doc.Citations) {
				if evidence, found := preEmitCitedEvidenceItems(ctx, doc.Citations[it.CitationRef]); found &&
					preEmitLabelMatchesAnyEvidenceEndpoint(label, evidence) {
					continue
				}
				if preEmitLabelSupportedByAggregateMemberSet(label, it, doc.Citations[it.CitationRef], ctx) {
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
	if label == "" || strings.TrimSpace(item.Text) == "" {
		return false
	}
	facts := ctx.Mutable.StableInvestigationAggregateFacts()
	if len(facts) == 0 {
		return false
	}
	for _, ref := range types.PrincipalAggregateMemberSetFactRefs(facts) {
		fact := ref.Fact
		for _, member := range fact.Members {
			if !preEmitAggregateMemberLabelTextMatches(label, item.Text, member) {
				continue
			}
			return true
		}
	}
	return false
}

func preEmitAggregateMemberLabelTextMatches(label, text, member string) bool {
	member = strings.TrimSpace(member)
	if member == "" {
		return false
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
	evidence, found := preEmitCitedEvidenceItems(ctx, cit)
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
		}
	}
	return false
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
	return strings.TrimSpace(strings.ReplaceAll(cit.File, `\`, `/`)) == strings.TrimSpace(strings.ReplaceAll(loc.File, `\`, `/`)) &&
		cit.Line == loc.LineStart
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
	counts := make(map[types.AnswerBlockKind]int, len(doc.Blocks))
	for _, b := range doc.Blocks {
		counts[b.Kind]++
	}
	var out []emitFixHint
	for _, req := range view.RequiredBlocks {
		if !req.Required {
			continue
		}
		got := counts[req.Kind]
		if got < req.MinCount {
			out = append(out, emitFixHint{
				Field: fmt.Sprintf("blocks[].kind=%s", req.Kind),
				ExpectedShape: fmt.Sprintf(
					"emit at least %d block(s) of kind=%s (currently emitted: %d)",
					req.MinCount, req.Kind, got),
				Reason: strings.TrimSpace(req.Rationale),
			})
			continue
		}
		if req.MaxCount > 0 && got > req.MaxCount {
			out = append(out, emitFixHint{
				Field: fmt.Sprintf("blocks[].kind=%s", req.Kind),
				ExpectedShape: fmt.Sprintf(
					"reduce kind=%s blocks to at most %d (currently emitted: %d)",
					req.Kind, req.MaxCount, got),
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
		if _, ok := reqByKind[r.Kind]; !ok {
			reqByKind[r.Kind] = r
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

// preCheckFacetCoverage mirrors the post-emit validateFacetCoverage
// "required facet has no block declaring its facet_id" branch. We
// gate on req.IsPromoted() the same way the post-emit validator
// does, so SOFT/Optional facets do NOT trigger pre-emit retries.
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
		if req.EffectivePromotionPolicy() == types.PromotionAdvisoryOnly {
			continue
		}
		if !req.IsPromoted() {
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
