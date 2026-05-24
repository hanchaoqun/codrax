// Package tool — answer_document_pre_emit_check.go (P1, 2026-05-10).
//
// Emit-time pre-validation chokepoint. When the LLM calls
// emit_answer_document, this layer runs common structural-compliance
// checks BEFORE the doc lands on Mutable. Each hint is tagged with the
// matching ViolationKind; the executor only hard-rejects kinds that are
// not SoftByDefault in the central registry. Soft hints are logged as
// advisories and later materialized through the normal caveat path.
//
// Pre-emit covers the validator axes that the post-emit chain in
// internal/orchestrator/contract_check.go runs after persist:
//
//   - block_coverage_missing   (validateRequiredBlockCoverage)
//   - principal_claim_use_missing (validatePrincipalClaimUse)
//   - uncertainty_block_missing  (validateUncertaintyBlockPresence)
//   - facet_uncovered          (validateFacetCoverage)
//
// Soft validators remain advisory at the tool boundary. This preserves
// the structured diagnostics for operators without turning uncertain
// shape/citation/member-set observations into same-turn LLM rewrites.
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
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
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
	Kind          types.ViolationKind
}

func tagPreEmitHints(kind types.ViolationKind, hints []emitFixHint) []emitFixHint {
	for i := range hints {
		if hints[i].Kind == "" {
			hints[i].Kind = kind
		}
	}
	return hints
}

func appendPreEmitHints(dst []emitFixHint, kind types.ViolationKind, hints []emitFixHint) []emitFixHint {
	if len(hints) == 0 {
		return dst
	}
	return append(dst, tagPreEmitHints(kind, hints)...)
}

func splitPreEmitHintsByGate(hints []emitFixHint) (hard []emitFixHint, advisory []emitFixHint) {
	for _, hint := range hints {
		if preEmitHintHardByDefault(hint) {
			hard = append(hard, hint)
		} else {
			advisory = append(advisory, hint)
		}
	}
	return hard, advisory
}

func preEmitHintHardByDefault(hint emitFixHint) bool {
	if hint.Kind == "" {
		return true
	}
	if hint.Kind == types.ViolBlockCoverageMissing && preEmitMissingBlockRequiresSameTurnRetry(hint) {
		// The central registry keeps this violation soft for post-emit V2
		// migration telemetry, where retry planning may choose caveat/materialize
		// fallbacks. Pre-emit is different: the model is still inside the same
		// tool call and the Required Answer Blocks contract can carry explicit
		// visual obligations. Letting a missing diagram block ship loses a
		// user-requested surface even though the correction is local.
		return true
	}
	if spec, ok := types.ViolKindSpecFor(hint.Kind); ok {
		return !spec.SoftByDefault
	}
	return types.ViolationProfileFor(hint.Kind, false).RetryEligible
}

func preEmitMissingBlockRequiresSameTurnRetry(hint emitFixHint) bool {
	field := strings.ToLower(strings.TrimSpace(hint.Field))
	shape := strings.ToLower(strings.TrimSpace(hint.ExpectedShape))
	return strings.Contains(field, "blocks[].kind=diagram") ||
		strings.Contains(shape, "kind=diagram")
}

func logSoftPreEmitAdvisory(toolName, label string, hints []emitFixHint) {
	if len(hints) == 0 {
		return
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		toolName = "emit_answer_document"
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = "pre-emit"
	}
	logging.Debug("[%s] %s accepted as soft advisory; no retry requested: %s",
		toolName, label, formatEmitAdvisoryHints(hints))
}

// runPreEmitChecks runs chokepoint checks on the just-built
// (but-not-yet-persisted) document. Returns nil when the shape is
// compliant; otherwise a non-empty list of typed fix hints. The executor
// decides whether each hint is hard or advisory from the central
// ViolationKind registry. view nil → returns nil (no view, no expected
// shape — pre-emit silently passes through and the post-emit chain takes
// over).
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
		return tagPreEmitHints(types.ViolCitation, h)
	}
	if h := preCheckNegativeCitationBounds(doc); len(h) > 0 {
		return tagPreEmitHints(types.ViolAbsenceScopeExceeded, h)
	}
	if h := preCheckRuntimeObservationRepoContamination(doc, ctxOpt...); len(h) > 0 {
		return tagPreEmitHints(types.ViolExternalArtifactUnderdecoded, h)
	}
	if h := preCheckArtifactObservedFrameCitations(doc, ctxOpt...); len(h) > 0 {
		return tagPreEmitHints(types.ViolCitation, h)
	}
	// Carrier visibility is governed by LLM-facing schema/prompt wording and
	// typed row roles, not by post-hoc keyword matching over RawRequest or the
	// model-rendered answer text.

	// 1. Required block kind + count compliance.
	if h := preCheckRequiredBlocks(doc, view); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolBlockCoverageMissing, h)
	}
	if h := preCheckSummaryLeadBlock(doc, view); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolFamilyMismatch, h)
	}

	// 2. Principal-block claim_use presence.
	if h := preCheckPrincipalClaimUse(doc, view); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolPrincipalClaimUseMissing, h)
	}
	if h := preCheckRequiredCandidateRoles(doc, view); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolMissingRequestedRoleUndisclosed, h)
	}
	if h := preCheckRequiredMechanismAnchors(doc, view); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolPrincipalSupportMemberOmitted, h)
	}
	if h := preCheckInactiveTypedDecisionVerdicts(doc, view); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolAcceptance, h)
	}
	if h := preCheckErrorGranularityVerdict(doc, view); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolAcceptance, h)
	}
	if h := preCheckCurrentStatusVerdict(doc, view); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolCurrentStatusVerdictMissing, h)
	}

	// 3. Uncertainty block presence (when contract requires it).
	if h := preCheckUncertaintyBlock(doc, view); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolUncertaintyBlockMissing, h)
	}

	// 4. Required facet coverage.
	if h := preCheckFacetCoverage(doc, view); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolFacetUncovered, h)
	}

	// 5. Per-item label/citation alignment. A symbol-like list label with
	// a citation must name the same cited evidence endpoint; otherwise the
	// rendered answer silently shifts file:line proof across adjacent hops.
	if h := preCheckItemCitationAlignmentWithContext(doc, view, pctx); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolCitation, h)
	}

	// 5b. Typed item/citation role alignment. An item that explicitly
	// asserts a typed evidence role must cite that same role, not a
	// nearby definition / guard / adjacent item that happens to share
	// one endpoint. The projection lives in types so new role shapes
	// (import/path/span/route/etc.) extend the central contract instead
	// of adding validator-specific patches.
	if h := preCheckCallChainItemCitationRoleAlignmentWithContext(doc, view, pctx); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolCitation, h)
	}

	// 5c. Principal support member coverage. For enumeration answers,
	// every answer-grade member already selected into the principal
	// support lane must be rendered as a cited item/row; the finalizer
	// should not compress away explorer-emitted members.
	if h := preCheckPrincipalSupportMemberCoverage(doc, ctxOpt...); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolPrincipalSupportMemberOmitted, h)
	}

	// 2026-05-16 (Fix 1) — multi-repo inactive-scope disclosure.
	// When typed BusContext.PendingSubRepos is non-empty AND the
	// answer is bounded (absent exact target, empty role-locate
	// slate, or scope-limited enumeration), the answer MUST
	// disclose the inactive scope. Pre-emit catches this same
	// dispatch so the model rewrites with disclosure instead of
	// burning a retry round.
	if h := preCheckInactiveScopeDisclosure(doc, ctxOpt...); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolInactiveScopeDisclosureMissing, h)
	}

	// 5d. Model-authored scalar aggregate preservation. A scalar_value
	// aggregate fact is an explicit typed handoff from exploration, not
	// scratchpad prose; the finalizer must carry the value into visible
	// answer blocks instead of remembering it only in thinking.
	if h := preCheckAggregateScalarValueCoverage(doc, ctxOpt...); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolAcceptance, h)
	}
	if h := preCheckAggregateMemberSetCoverage(doc, ctxOpt...); len(h) > 0 {
		if preEmitAggregateMemberSetCoverageHardGate(ctxOpt...) {
			hints = appendPreEmitHints(hints, types.ViolExhaustiveMemberSetCoverageDrift, h)
		} else {
			logSoftPreEmitAdvisory("emit_answer_document", "aggregate member_set coverage", h)
		}
	}
	if h := preCheckAggregateCardinalityConsistency(doc, ctxOpt...); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolCardinalityShort, h)
	}
	if h := preCheckRelationMemberSetAnswerShape(doc, ctxOpt...); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolExhaustiveMemberSetCoverageDrift, h)
	}

	// 5e. Bounded exact absence. Keep the same typed check inside the
	// emit dispatch, but route it through the soft/hard split so the
	// default path can ship with a bounded-scope caveat.
	if h := preCheckAbsenceScopeBound(doc); len(h) > 0 {
		hints = appendPreEmitHints(hints, types.ViolAbsenceScopeExceeded, h)
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
			hints = appendPreEmitHints(hints, types.ViolEnumerationLabelHallucinated, h)
		} else {
			logSoftPreEmitAdvisory("emit_answer_document", "enumeration label grounding", h)
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
			Reason:        "a citation rendered as an absence proof must be reproducible; attached log / trace observations belong in the runtime-observation lane rather than fake current-repo absence citations.",
		}}
	}
	return nil
}

func preCheckRuntimeObservationRepoContamination(doc *types.AnswerDocumentV2, ctxOpt ...*types.BusContext) []emitFixHint {
	if doc == nil || len(ctxOpt) == 0 || ctxOpt[0] == nil {
		return nil
	}
	ctx := ctxOpt[0]
	plan := answerSurfacePlan(ctx)
	if plan == nil || !plan.RuntimeGroundingDisposition.IsActive() ||
		plan.CurrentStatusDiagnosticRequired ||
		plan.CurrentSourceEvidenceOrigin {
		return nil
	}
	if len(doc.Citations) > 0 {
		return []emitFixHint{{
			Field:         "citations[]",
			ExpectedShape: "for an observation-only external runtime artifact answer, omit current-repo citations and keep visible provenance as attached-log / attached-trace observations",
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
		expected := "runtime artifact frame coordinates should stay in observed-artifact rows without current-repo citations"
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

func normalizeCurrentSourceCitationSupplement(doc *types.AnswerDocumentV2, ctx *types.BusContext, pctx *preEmitCheckContext) int {
	if doc == nil || ctx == nil || ctx.AnalysisIR == nil {
		return 0
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || !plan.CurrentSourceEvidenceOrigin || answerDocumentRuntimeObservationOnly(ctx) {
		return 0
	}
	if answerDocumentHasCurrentSourceCitation(doc) {
		return 0
	}
	if pctx == nil {
		pctx = newPreEmitCheckContext(ctx)
	}
	rows := currentSourceCitationSupplementRows(doc, ctx, pctx, 6)
	if len(rows) == 0 {
		return 0
	}
	zh := principalEnumerationPrefersZH(ctx)
	block := types.AnswerBlock{
		ID:       nextCurrentSourceCitationSupplementBlockID(doc),
		Kind:     types.BlockTable,
		Title:    currentSourceCitationSupplementTitle(zh, len(rows)),
		FacetIDs: []string{string(types.FacetCurrentCodePath)},
		Columns:  currentSourceCitationSupplementColumns(zh),
		ClaimUses: []types.RenderedClaimUse{{
			FacetID:   string(types.FacetCurrentCodePath),
			ClaimForm: types.ClaimDefinitionFact,
		}},
	}
	for i, row := range rows {
		ref := appendOrReusePreEmitCitation(doc, types.Citation{
			File:    row.ev.Source,
			Line:    row.ev.LineStart,
			LineEnd: row.ev.LineEnd,
			Scope:   row.ev.Scope,
		})
		if ref < 0 {
			continue
		}
		block.Items = append(block.Items, types.AnswerBlockItem{
			ID:          fmt.Sprintf("source_anchor_%d", i+1),
			Label:       currentSourceCitationSupplementLabel(row.ev),
			Text:        strings.TrimSpace(row.ev.Summary),
			Cells:       currentSourceCitationSupplementCells(row.ev, zh),
			CitationRef: ref,
		})
	}
	if len(block.Items) == 0 {
		return 0
	}
	doc.Blocks = append(doc.Blocks, block)
	return len(block.Items)
}

func answerDocumentHasCurrentSourceCitation(doc *types.AnswerDocumentV2) bool {
	if doc == nil {
		return false
	}
	for _, cit := range doc.Citations {
		file := strings.TrimSpace(cit.File)
		if file != "" && cit.Line > 0 && cit.Scope != types.ScopeNegative {
			return true
		}
	}
	return false
}

type currentSourceCitationSupplementRow struct {
	ev    types.EvidenceItem
	score int
	seq   int
}

func currentSourceCitationSupplementRows(doc *types.AnswerDocumentV2, ctx *types.BusContext, pctx *preEmitCheckContext, limit int) []currentSourceCitationSupplementRow {
	if limit <= 0 {
		limit = 6
	}
	visible := answerDocumentVisibleText(doc)
	required := currentSourceRequiredFileSet(ctx)
	droppedCitationPool := answerDocumentHasOutOfRangeCitationRefs(doc)
	var rows []currentSourceCitationSupplementRow
	seen := map[string]bool{}
	for seq, ev := range pctx.evidenceItems() {
		if !currentSourceEvidenceCitable(ev) {
			continue
		}
		key := strings.ToLower(fmt.Sprintf("%s:%d", strings.TrimSpace(ev.Source), ev.LineStart))
		if seen[key] {
			continue
		}
		if !droppedCitationPool && !currentSourceCitationSupplementVisibleHit(ev, visible) {
			continue
		}
		score := currentSourceCitationSupplementScore(ev, visible, required)
		if score <= 0 {
			continue
		}
		seen[key] = true
		rows = append(rows, currentSourceCitationSupplementRow{ev: ev, score: score, seq: seq})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score > rows[j].score
		}
		return rows[i].seq < rows[j].seq
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func answerDocumentHasOutOfRangeCitationRefs(doc *types.AnswerDocumentV2) bool {
	if doc == nil {
		return false
	}
	for _, block := range doc.Blocks {
		for _, item := range block.Items {
			if item.CitationRef > 0 && item.CitationRef >= len(doc.Citations) {
				return true
			}
		}
	}
	return false
}

func currentSourceCitationSupplementVisibleHit(ev types.EvidenceItem, visible string) bool {
	if preEmitItemSurfaceMentionsEvidence("", visible, ev) {
		return true
	}
	return strings.TrimSpace(ev.Summary) != "" && types.AnswerCodeSurfaceAppearsInText(visible, ev.Summary)
}

func currentSourceEvidenceCitable(ev types.EvidenceItem) bool {
	if ev.GroundingStatus == types.GroundingUngrounded ||
		strings.TrimSpace(ev.Source) == "" ||
		ev.LineStart <= 0 {
		return false
	}
	switch ev.Origin {
	case types.ClaimOriginLog, types.ClaimOriginPerf:
		return false
	}
	switch ev.Scope {
	case types.ScopeLine, types.ScopeLineRange, types.ScopeSection:
		return true
	default:
		return false
	}
}

func currentSourceRequiredFileSet(ctx *types.BusContext) map[string]bool {
	out := map[string]bool{}
	if ctx == nil || ctx.AnalysisIR == nil {
		return out
	}
	for _, hint := range ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints {
		path := strings.TrimSpace(strings.ReplaceAll(hint.Path, `\`, `/`))
		if path == "" {
			continue
		}
		out[strings.ToLower(path)] = true
	}
	return out
}

func currentSourceCitationSupplementScore(ev types.EvidenceItem, visible string, required map[string]bool) int {
	score := 0
	if currentSourceCitationSupplementVisibleHit(ev, visible) {
		score += 100
	}
	source := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(ev.Source, `\`, `/`)))
	if required[source] {
		score += 70
	}
	switch ev.Salience {
	case types.SalienceLoadBearing:
		score += 40
	case types.SalienceExhaustListed:
		score += 30
	case types.SalienceSupporting:
		score += 15
	}
	switch ev.Kind {
	case types.EvidenceDirect, types.EvidenceMechanism, types.EvidenceConditional:
		score += 12
	case types.EvidenceRegistration, types.EvidenceRelationship:
		score += 8
	}
	switch ev.AnchorKind {
	case types.AnchorDefinition, types.AnchorInitializer, types.AnchorAssignment:
		score += 8
	case types.AnchorCall, types.AnchorCondition, types.AnchorReturn:
		score += 5
	}
	if strings.TrimSpace(ev.Summary) != "" {
		score += 5
	}
	return score
}

func currentSourceCitationSupplementTitle(zh bool, n int) string {
	if zh {
		return fmt.Sprintf("系统按已验证证据补充关键源码锚点（%d）", n)
	}
	return fmt.Sprintf("System-verified source anchor supplement (%d)", n)
}

func currentSourceCitationSupplementColumns(zh bool) []string {
	if zh {
		return []string{"位置", "锚点", "说明"}
	}
	return []string{"Location", "Anchor", "Note"}
}

func currentSourceCitationSupplementCells(ev types.EvidenceItem, zh bool) []string {
	return []string{
		currentSourceEvidenceLocation(ev),
		currentSourceCitationSupplementLabel(ev),
		strings.TrimSpace(ev.Summary),
	}
}

func currentSourceEvidenceLocation(ev types.EvidenceItem) string {
	file := strings.TrimSpace(ev.Source)
	if file == "" || ev.LineStart <= 0 {
		return ""
	}
	if ev.LineEnd > ev.LineStart {
		return fmt.Sprintf("%s:%d-%d", file, ev.LineStart, ev.LineEnd)
	}
	return fmt.Sprintf("%s:%d", file, ev.LineStart)
}

func currentSourceCitationSupplementLabel(ev types.EvidenceItem) string {
	for _, s := range []string{ev.AnchorSymbol, ev.Subject, ev.Object, ev.OwnerSymbol} {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return currentSourceEvidenceLocation(ev)
}

func nextCurrentSourceCitationSupplementBlockID(doc *types.AnswerDocumentV2) string {
	base := "current-source-anchor-supplement"
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
		if !preEmitBlockRendersItemSurface(b.Kind) {
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

func preEmitBlockRendersItemSurface(kind types.AnswerBlockKind) bool {
	switch kind {
	case types.BlockSection, types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		return true
	default:
		return false
	}
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
		if !preEmitBlockRendersItemSurface(block.Kind) {
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
			if preEmitBlockPrefersExactDefinitionCitation(*block) {
				if cit, ok := preEmitUniqueExactEndpointDefinitionCitationForLabelWithContext(pctx, label); ok {
					ref := appendOrReusePreEmitCitation(doc, cit)
					if ref >= 0 && item.CitationRef != ref {
						item.CitationRef = ref
						fixed++
					}
					continue
				}
			}
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

func preEmitBlockPrefersExactDefinitionCitation(block types.AnswerBlock) bool {
	for _, claim := range block.ClaimUses {
		if claim.ClaimForm == types.ClaimDefinitionFact {
			return true
		}
		switch claim.FacetID {
		case string(types.FacetEnumerationItem),
			string(types.FacetResolvedLiteralOrSymbol),
			string(types.FacetCurrentCodePath):
			return true
		}
	}
	for _, facet := range block.FacetIDs {
		switch facet {
		case string(types.FacetEnumerationItem),
			string(types.FacetResolvedLiteralOrSymbol),
			string(types.FacetCurrentCodePath):
			return true
		}
	}
	return false
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
		if !preEmitBlockRendersItemSurface(block.Kind) {
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
		if !preEmitBlockRendersItemSurface(block.Kind) {
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
	if doc == nil || ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil ||
		!preEmitAggregateMemberSetCoverageHardGate(ctx) {
		return 0
	}
	if answerDocumentRuntimeObservationOnly(ctx) {
		return 0
	}
	refs := preEmitPrincipalAggregateMemberSetFactRefs(ctx, ctx.Mutable.StableInvestigationAggregateFacts())
	if len(refs) == 0 {
		return 0
	}
	visibleSurface := preEmitVisibleAnswerSurface(doc)
	displayRows := aggregateMemberSetDisplayRowsByFactIndex(ctx)
	zh := principalEnumerationPrefersZH(ctx)
	fixed := 0
	for _, ref := range refs {
		fact := ref.Fact
		rows := displayRows[ref.Index]
		if preEmitAggregateMemberSetIsScalarCountSupport(ctx, fact) ||
			len(fact.Members) == 0 ||
			preEmitAnswerDocumentCoversAggregateMemberSetFact(doc, ctx, fact, visibleSurface) ||
			preEmitAnswerDocumentCoversEnumerationDisplayRows(doc, rows) ||
			preEmitAnswerDocumentHasAuthoredEnumerationCarrier(doc, rows) {
			continue
		}
		if len(fact.Members) == 1 && preEmitPrincipalStructuredBlockClaimsAggregateCategory(doc, fact) {
			continue
		}
		blockIdx := appendAggregateMemberSetCarrierBlock(doc, ref.Index, aggregateMemberSetCarrierTitle(fact, zh))
		if blockIdx < 0 || blockIdx >= len(doc.Blocks) {
			continue
		}
		block := &doc.Blocks[blockIdx]
		block.SurfaceRole = types.SurfacePrincipal
		block.FacetIDs = mergeStringSet(block.FacetIDs, []string{string(types.FacetEnumerationItem)})
		block.ClaimUses = append(block.ClaimUses, types.RenderedClaimUse{
			FacetID:   string(types.FacetEnumerationItem),
			ClaimForm: types.ClaimDefinitionFact,
		})
		block.Text = aggregateMemberSetCarrierText(fact.Label, zh)
		for memberIdx, member := range fact.Members {
			member = strings.TrimSpace(member)
			if member == "" {
				continue
			}
			var row types.EnumerationDisplayRow
			if memberIdx < len(rows) {
				row = rows[memberIdx]
			}
			citationRef := -1
			if row.HasCitation && strings.TrimSpace(row.Source) != "" && row.LineStart > 0 {
				citationRef = appendOrReusePreEmitCitation(doc, types.Citation{File: row.Source, Line: row.LineStart})
			} else if cit, ok := citationForAggregateMemberSetMember(fact, memberIdx, member, ctx); ok {
				citationRef = appendOrReusePreEmitCitation(doc, cit)
			}
			itemText := ""
			if strings.TrimSpace(row.Note) != "" {
				itemText = strings.TrimSpace(row.Note)
			} else if ev, ok := evidenceForAggregateMemberSetMember(fact, memberIdx, member, ctx); ok {
				itemText = aggregateMemberSetEvidenceSummaryText(ev)
			}
			itemLabel := aggregateMemberSetCarrierLabel(member)
			if strings.TrimSpace(row.DisplayLabel) != "" {
				itemLabel = strings.TrimSpace(row.DisplayLabel)
			}
			block.Items = append(block.Items, types.AnswerBlockItem{
				ID:          aggregateMemberSetCarrierItemID(block.ID, memberIdx, member),
				Label:       itemLabel,
				Text:        itemText,
				CitationRef: citationRef,
			})
			fixed++
		}
	}
	return fixed
}

func preEmitAnswerDocumentCoversEnumerationDisplayRows(doc *types.AnswerDocumentV2, rows []types.EnumerationDisplayRow) bool {
	if doc == nil || len(rows) == 0 {
		return false
	}
	for _, row := range rows {
		covered := false
		for _, block := range doc.Blocks {
			if !principalEnumerationBlockCanCarryRows(block) {
				continue
			}
			if principalEnumerationBlockCoversRow(block, doc, row) {
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

func preEmitAnswerDocumentHasAuthoredEnumerationCarrier(doc *types.AnswerDocumentV2, rows []types.EnumerationDisplayRow) bool {
	if doc == nil || len(rows) == 0 {
		return false
	}
	for _, block := range doc.Blocks {
		if preEmitSystemEnumerationRowSupplementBlock(block) || !principalEnumerationBlockCanCarryRows(block) {
			continue
		}
		surface := strings.TrimSpace(types.AnswerBlockVisibleSurface(block))
		if surface == "" && len(block.Items) == 0 {
			continue
		}
		if principalEnumerationBlockHasEnumerationFacet(block) || block.SurfaceRole == types.SurfacePrincipal {
			return true
		}
		for _, row := range rows {
			if principalEnumerationBlockCoversRow(block, doc, row) ||
				principalEnumerationVisibleSurfaceCoversRow(surface, row) {
				return true
			}
		}
	}
	return false
}

func preEmitAnswerDocumentCoversAggregateMemberSetFact(doc *types.AnswerDocumentV2, ctx *types.BusContext, fact types.AnswerAggregateFact, surface string) bool {
	if preEmitStructuredMemberBlockCoversFact(doc, fact) {
		return true
	}
	if doc == nil || len(fact.Members) == 0 || strings.TrimSpace(surface) == "" {
		return false
	}
	for idx, member := range fact.Members {
		if !preEmitAggregateMemberAppearsInDocument(member, doc, surface) {
			return false
		}
		if !preEmitAggregateMemberLocationAppearsInSurface(fact, idx, member, ctx, surface) {
			return false
		}
	}
	return true
}

func preEmitAggregateMemberLocationAppearsInSurface(fact types.AnswerAggregateFact, memberIdx int, member string, ctx *types.BusContext, surface string) bool {
	cit, ok := citationForAggregateMemberSetMember(fact, memberIdx, member, ctx)
	if !ok {
		return preEmitAggregateFactHasOriginSpecificSupport(fact, ctx)
	}
	file := strings.TrimSpace(cit.File)
	if file == "" || cit.Line <= 0 {
		return preEmitAggregateFactHasOriginSpecificSupport(fact, ctx)
	}
	return strings.Contains(surface, fmt.Sprintf("%s:%d", file, cit.Line))
}

func preEmitAggregateFactHasOriginSpecificSupport(fact types.AnswerAggregateFact, ctx *types.BusContext) bool {
	var rm *types.RequestModel
	if ctx != nil && ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	for _, origin := range types.AnswerAggregateFactEvidenceOrigins(fact, rm) {
		if types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
			return true
		}
	}
	return false
}

func preEmitPrincipalStructuredBlockClaimsAggregateCategory(doc *types.AnswerDocumentV2, fact types.AnswerAggregateFact) bool {
	if doc == nil {
		return false
	}
	label := strings.TrimSpace(fact.Label)
	if label == "" {
		return false
	}
	for _, block := range doc.Blocks {
		switch block.Kind {
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		default:
			continue
		}
		if block.SurfaceRole != types.SurfacePrincipal || len(block.Items) == 0 {
			continue
		}
		if !preEmitBlockSharesFacet(block, []string{string(types.FacetEnumerationItem)}) {
			continue
		}
		surface := strings.TrimSpace(block.Title)
		if surface == "" {
			surface = strings.TrimSpace(block.Text)
		}
		if surface == "" {
			continue
		}
		if strings.Contains(strings.ToLower(surface), strings.ToLower(label)) {
			return true
		}
	}
	return false
}

func aggregateMemberSetDisplayRowsByFactIndex(ctx *types.BusContext) map[int][]types.EnumerationDisplayRow {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil {
		return nil
	}
	sets := types.CompileEnumerationDisplaySets(&ctx.AnalysisIR.RequestModel, plan)
	if len(sets) == 0 {
		return nil
	}
	out := make(map[int][]types.EnumerationDisplayRow, len(sets))
	for _, set := range sets {
		if len(set.Rows) > 0 {
			out[set.FactIndex] = set.Rows
		}
	}
	return out
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

func aggregateMemberSetCarrierTitle(fact types.AnswerAggregateFact, zh bool) string {
	label := strings.TrimSpace(fact.Label)
	if label == "" {
		if zh {
			label = "成员清单"
		} else {
			label = "principal member set"
		}
	}
	n := len(fact.Members)
	if zh {
		if n > 0 && !strings.Contains(label, strconv.Itoa(n)) {
			label = fmt.Sprintf("%s（%d）", label, n)
		}
		return "系统按已验证证据补充成员：" + label
	}
	if n > 0 && !strings.Contains(label, strconv.Itoa(n)) {
		label = fmt.Sprintf("%s (%d)", label, n)
	}
	return "System-verified member supplement: " + label
}

func aggregateMemberSetCarrierItemID(blockID string, idx int, member string) string {
	base := strings.TrimSpace(blockID)
	if base == "" {
		base = "aggregate-member-set"
	}
	if suffix := sanitizeRequiredMechanismAnchorID(member); suffix != "" {
		return base + "-" + suffix
	}
	return fmt.Sprintf("%s-%d", base, idx+1)
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

func aggregateMemberSetCarrierText(factLabel string, zh bool) string {
	factLabel = strings.TrimSpace(factLabel)
	if !zh {
		if factLabel == "" {
			return "The following rows come from the accepted structured investigation checklist. The system keeps this supplement separate to preserve a complete enumeration."
		}
		return "The following rows come from the accepted structured investigation checklist for " + factLabel + ". The system keeps this supplement separate to preserve a complete enumeration."
	}
	if factLabel == "" {
		return "以下成员来自已验收的结构化调查清单；系统保留该清单以确保完整枚举。"
	}
	return "以下成员来自已验收的结构化调查清单，分组为：" + factLabel + "；系统保留该清单以确保完整枚举。"
}

func citationForAggregateMemberSetMember(fact types.AnswerAggregateFact, memberIdx int, member string, ctx *types.BusContext) (types.Citation, bool) {
	if aggregateFactSuppressesCurrentSourceLocationFallbacks(fact, ctx) {
		return types.Citation{}, false
	}
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

func evidenceForAggregateMemberSetMember(fact types.AnswerAggregateFact, memberIdx int, member string, ctx *types.BusContext) (types.EvidenceItem, bool) {
	if aggregateFactSuppressesCurrentSourceLocationFallbacks(fact, ctx) {
		return types.EvidenceItem{}, false
	}
	evidence := aggregateMemberSetEvidencePool(ctx)
	if len(evidence) == 0 {
		return types.EvidenceItem{}, false
	}
	candidates := aggregateMemberSetEvidenceCandidates(member)
	if file := aggregateMemberSetMemberFilePrefix(member); file != "" {
		for _, ev := range evidence {
			if !aggregateMemberSetEvidenceLocationUsable(ev) || !aggregateMemberSetPathMatches(file, ev.Source) {
				continue
			}
			if aggregateMemberSetEvidenceMatchesAny(ev, candidates) {
				return ev, true
			}
		}
	}
	if cit, ok := citationForAggregateMemberSetSupportRef(fact, memberIdx, member); ok {
		for _, ev := range evidence {
			if !aggregateMemberSetEvidenceLocationUsable(ev) || !aggregateMemberSetEvidenceMatchesCitation(ev, cit) {
				continue
			}
			if aggregateMemberSetEvidenceMatchesAny(ev, candidates) {
				return ev, true
			}
		}
		for _, ev := range evidence {
			if aggregateMemberSetEvidenceLocationUsable(ev) && aggregateMemberSetEvidenceMatchesCitation(ev, cit) {
				return ev, true
			}
		}
	}
	for _, ev := range evidence {
		if !aggregateMemberSetEvidenceLocationUsable(ev) {
			continue
		}
		if aggregateMemberSetEvidenceMatchesAny(ev, candidates) {
			return ev, true
		}
	}
	return types.EvidenceItem{}, false
}

func aggregateFactSuppressesCurrentSourceLocationFallbacks(fact types.AnswerAggregateFact, ctx *types.BusContext) bool {
	var rm *types.RequestModel
	if ctx != nil && ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	origins := types.AnswerAggregateFactEvidenceOrigins(fact, rm)
	if len(origins) == 0 {
		return false
	}
	return types.AnswerEvidenceOriginsAreOriginSpecificOnly(origins)
}

func aggregateMemberSetEvidenceMatchesCitation(ev types.EvidenceItem, cit types.Citation) bool {
	if strings.TrimSpace(cit.File) == "" || cit.Line <= 0 {
		return false
	}
	if !aggregateMemberSetPathMatches(cit.File, ev.Source) {
		return false
	}
	if ev.LineStart == cit.Line {
		return true
	}
	return ev.LineStart > 0 && ev.LineEnd >= ev.LineStart &&
		cit.Line >= ev.LineStart && cit.Line <= ev.LineEnd
}

func aggregateMemberSetEvidenceSummaryText(ev types.EvidenceItem) string {
	return strings.Join(strings.Fields(strings.TrimSpace(ev.Summary)), " ")
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
	base := strings.TrimSpace(ob.StableItemID())
	if base == "" || base == "support-" {
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
	if ctxOpt[0].AnalysisIR != nil {
		facts = types.NormalizeAggregateFactRolesForRequest(facts, &ctxOpt[0].AnalysisIR.RequestModel)
	}
	surface := preEmitVisibleAnswerSurface(doc)
	if strings.TrimSpace(surface) == "" {
		return nil
	}
	type missingScalar struct {
		kind  types.AnswerAggregateKind
		label string
		value string
		unit  string
		dims  []types.AnswerAggregateDimension
	}
	var missing []missingScalar
	seen := make(map[string]bool)
	for idx, fact := range facts {
		if !preEmitAggregateFactRequiresVisibleValue(ctxOpt[0], facts, idx, fact) {
			continue
		}
		value := strings.TrimSpace(fact.Value)
		if value == "" || preEmitAggregateFactValueAppears(fact, surface) {
			continue
		}
		key := preEmitAggregateFactVisibilityKey(fact)
		if seen[key] {
			continue
		}
		seen[key] = true
		missing = append(missing, missingScalar{
			kind:  fact.Kind,
			label: strings.TrimSpace(fact.Label),
			value: value,
			unit:  strings.TrimSpace(fact.Unit),
			dims:  fact.Dimensions,
		})
	}
	if len(missing) == 0 {
		return nil
	}
	parts := make([]string, 0, len(missing))
	for _, m := range missing {
		parts = append(parts, preEmitAggregateFactExpectedShape(m.kind, m.label, m.value, m.unit, m.dims))
	}
	return []emitFixHint{{
		Field: "blocks[].text OR blocks[].items[].label/text/cells",
		ExpectedShape: "include every principal model-emitted aggregate value in the visible answer surface: " +
			strings.Join(parts, "; "),
		Reason: "the investigation already handed these scalar/count/no-hit values to the final answer as structured data; leaving them only in thinking, citations, or closure notes drops the user's requested literal or boundary.",
	}}
}

func preEmitAggregateFactRequiresVisibleValue(ctx *types.BusContext, facts []types.AnswerAggregateFact, idx int, fact types.AnswerAggregateFact) bool {
	if ctx != nil && ctx.AnalysisIR != nil &&
		types.AggregateFactConflictsWithPrincipalMemberSetCounts(facts, &ctx.AnalysisIR.RequestModel, idx) {
		return false
	}
	if preEmitAggregateFactIsNonExactOriginSupport(ctx, fact) {
		return false
	}
	role := types.NormalizeAnswerAggregateRole(fact.Role)
	switch role {
	case types.AnswerAggregateRolePrincipalAnswer:
		return true
	case types.AnswerAggregateRoleSupportingCoverage, types.AnswerAggregateRoleAuditLedger:
		return false
	}
	switch fact.Kind {
	case types.AnswerAggregateScalar:
		return true
	case types.AnswerAggregateNegativeSearch:
		return ctx != nil && ctx.Mutable != nil &&
			strings.EqualFold(strings.TrimSpace(ctx.Mutable.StableInvestigationResultKind()), "absence")
	case types.AnswerAggregateTotalCount,
		types.AnswerAggregateUniqueCount,
		types.AnswerAggregateGroupedCount,
		types.AnswerAggregateBucketCount,
		types.AnswerAggregateExcluded:
		return preEmitAggregateRequestWantsCountValue(ctx)
	default:
		return false
	}
}

func preEmitAggregateFactIsNonExactOriginSupport(ctx *types.BusContext, fact types.AnswerAggregateFact) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	bindings := types.CompileAnswerClaimBindingsFromAggregateFacts(
		[]types.AnswerAggregateFact{fact},
		&ctx.AnalysisIR.RequestModel,
		&ctx.AnalysisIR.AnswerContract,
	)
	if len(bindings) == 0 {
		return false
	}
	return types.AnswerClaimBindingsHaveOnlyOriginSpecificSupport(bindings) &&
		!types.AnswerClaimBindingsRequestExactOutput(bindings)
}

func preEmitAggregateRequestWantsCountValue(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	return rm.Predicates.IsCountQuestion || rm.Predicates.IsScalarAnswer || rm.Intent == types.IntentReturnValue
}

func preEmitAggregateFactValueAppears(fact types.AnswerAggregateFact, surface string) bool {
	value := strings.TrimSpace(fact.Value)
	if value == "" || !preEmitAggregateScalarValueAppears(value, surface) {
		return false
	}
	if fact.Kind != types.AnswerAggregateNegativeSearch && fact.Kind != types.AnswerAggregateNegativeObservation {
		return true
	}
	return preEmitNegativeSearchFactDimensionsAppear(fact, surface)
}

func preEmitNegativeSearchFactDimensionsAppear(fact types.AnswerAggregateFact, surface string) bool {
	dims := preEmitAggregateDimensionMap(fact.Dimensions)
	repo := dims["repo"]
	if repo != "" && !preEmitAggregateDisplayPartAppears(repo, surface) {
		return false
	}
	query := dims["query"]
	if query == "" {
		query = dims["pattern"]
	}
	if query == "" {
		query = firstNonEmptyPreEmitDim(dims, "target", "predicate")
	}
	if query != "" && !preEmitAggregateSearchPatternAppears(query, surface) {
		return false
	}
	scope := dims["scope"]
	if scope != "" && !strings.EqualFold(scope, repo) && !preEmitAggregateDisplayPartAppears(scope, surface) {
		return false
	}
	return repo != "" || query != "" || scope != ""
}

type aggregateNegativeProofSupplementRow struct {
	kind      types.AnswerAggregateKind
	origin    string
	target    string
	query     string
	scope     string
	searched  string
	details   []aggregateNegativeProofDetail
	citation  types.Citation
	hasCite   bool
	claimForm types.ClaimForm
}

type aggregateNegativeProofDetail struct {
	name  string
	value string
}

func normalizeAggregateNegativeProofSupplement(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil || ctx.Mutable == nil || len(doc.Blocks) >= maxBlocksPerDoc {
		return 0
	}
	facts := ctx.Mutable.StableInvestigationAggregateFacts()
	if len(facts) == 0 {
		return 0
	}
	surface := preEmitVisibleAnswerSurface(doc)
	rows := make([]aggregateNegativeProofSupplementRow, 0)
	seen := map[string]bool{}
	for idx, fact := range facts {
		if fact.Kind != types.AnswerAggregateNegativeSearch && fact.Kind != types.AnswerAggregateNegativeObservation {
			continue
		}
		if !preEmitNegativeAggregateFactRequiresVisibleProof(ctx, facts, idx, fact) {
			continue
		}
		if value := strings.TrimSpace(fact.Value); value != "" && value != "0" {
			continue
		}
		if preEmitNegativeAggregateFactCoveredByStructuredAbsence(doc, fact, surface) {
			continue
		}
		if preEmitAggregateFactValueAppears(fact, surface) {
			continue
		}
		row, ok := aggregateNegativeProofSupplementRowFromFact(fact)
		if !ok {
			continue
		}
		key := strings.Join([]string{string(row.kind), row.origin, row.target, row.query, row.scope}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, row)
		if len(rows) >= 6 {
			break
		}
	}
	if len(rows) == 0 {
		return 0
	}
	zh := principalEnumerationPrefersZH(ctx)
	block := types.AnswerBlock{
		ID:    uniqueAnswerBlockID(doc, "negative-proof-supplement"),
		Kind:  types.BlockCaveat,
		Title: aggregateNegativeProofSupplementTitle(zh),
		Text:  aggregateNegativeProofSupplementText(rows, zh),
	}
	claimForms := map[types.ClaimForm]bool{}
	for _, row := range rows {
		if row.claimForm != "" && !claimForms[row.claimForm] {
			block.ClaimUses = append(block.ClaimUses, types.RenderedClaimUse{ClaimForm: row.claimForm})
			claimForms[row.claimForm] = true
		}
		if row.hasCite {
			appendOrReuseNegativePreEmitCitation(doc, row.citation)
		}
	}
	doc.Blocks = append(doc.Blocks, block)
	return len(rows)
}

func preEmitNegativeAggregateFactCoveredByStructuredAbsence(doc *types.AnswerDocumentV2, fact types.AnswerAggregateFact, surface string) bool {
	if doc == nil || doc.ExactResolution == nil ||
		doc.ExactResolution.Status != types.AnswerExactResolutionAbsent {
		return false
	}
	if fact.Kind != types.AnswerAggregateNegativeSearch &&
		fact.Kind != types.AnswerAggregateNegativeObservation {
		return false
	}
	return preEmitNegativeAggregateFactScopeAndTargetAppear(fact, surface)
}

func preEmitNegativeAggregateFactScopeAndTargetAppear(fact types.AnswerAggregateFact, surface string) bool {
	dims := preEmitAggregateDimensionMap(fact.Dimensions)
	target := firstNonEmptyPreEmitDim(dims, "target", "query", "pattern", "predicate")
	if target == "" {
		target = strings.TrimSpace(fact.Label)
	}
	query := firstNonEmptyPreEmitDim(dims, "query", "pattern", "predicate", "target")
	scope := firstNonEmptyPreEmitDim(dims, "scope", "commit_range", "artifact_id", "trace_window", "tool_result", "source_ref")
	if scope == "" && fact.Kind == types.AnswerAggregateNegativeSearch {
		scope = firstNonEmptyPreEmitDim(dims, "repo")
	}
	seen := false
	if target != "" {
		if !preEmitAggregateSearchPatternAppears(target, surface) {
			return false
		}
		seen = true
	}
	if query != "" && query != target {
		if !preEmitAggregateSearchPatternAppears(query, surface) {
			return false
		}
		seen = true
	}
	if scope != "" {
		if !preEmitAggregateDisplayPartAppears(scope, surface) {
			return false
		}
		seen = true
	}
	return seen
}

func preEmitNegativeAggregateFactRequiresVisibleProof(ctx *types.BusContext, facts []types.AnswerAggregateFact, idx int, fact types.AnswerAggregateFact) bool {
	if ctx != nil && ctx.AnalysisIR != nil &&
		types.AggregateFactConflictsWithPrincipalMemberSetCounts(facts, &ctx.AnalysisIR.RequestModel, idx) {
		return false
	}
	if types.NormalizeAnswerAggregateRole(fact.Role) == types.AnswerAggregateRolePrincipalAnswer {
		return true
	}
	if fact.Kind == types.AnswerAggregateNegativeSearch && ctx != nil && ctx.Mutable != nil &&
		strings.EqualFold(strings.TrimSpace(ctx.Mutable.StableInvestigationResultKind()), "absence") {
		return true
	}
	if ctx != nil && ctx.AnalysisIR != nil {
		contract := types.CompileAnswerIntentContract(ctx.AnalysisIR.RequestModel, &ctx.AnalysisIR.AnswerContract)
		return contract.HasOutput(types.AnswerRequestedOutputAbsence)
	}
	return false
}

func aggregateNegativeProofSupplementRowFromFact(fact types.AnswerAggregateFact) (aggregateNegativeProofSupplementRow, bool) {
	dims := preEmitAggregateDimensionMap(fact.Dimensions)
	row := aggregateNegativeProofSupplementRow{
		kind:     fact.Kind,
		origin:   firstNonEmptyPreEmitDim(dims, "origin", "evidence_origin", "source_ref"),
		target:   firstNonEmptyPreEmitDim(dims, "target", "query", "pattern", "predicate"),
		query:    firstNonEmptyPreEmitDim(dims, "query", "pattern", "predicate", "target"),
		scope:    firstNonEmptyPreEmitDim(dims, "scope", "commit_range", "artifact_id", "trace_window", "tool_result", "source_ref"),
		searched: firstNonEmptyPreEmitDim(dims, "searched_at", "time"),
		details:  aggregateNegativeProofDetails(dims),
	}
	if row.target == "" && strings.TrimSpace(fact.Label) != "" {
		row.target = strings.TrimSpace(fact.Label)
	}
	if row.query == "" {
		row.query = row.target
	}
	if row.scope == "" {
		return aggregateNegativeProofSupplementRow{}, false
	}
	switch fact.Kind {
	case types.AnswerAggregateNegativeSearch:
		repo := firstNonEmptyPreEmitDim(dims, "repo")
		if repo != "" && row.origin == "" {
			row.origin = repo
		}
		if row.query == "" {
			return aggregateNegativeProofSupplementRow{}, false
		}
		row.claimForm = types.ClaimAbsenceFact
		row.citation = types.Citation{
			File:            row.scope,
			Scope:           types.ScopeNegative,
			NegativePattern: row.query,
		}
		row.hasCite = strings.TrimSpace(row.citation.File) != "" && strings.TrimSpace(row.citation.NegativePattern) != ""
	case types.AnswerAggregateNegativeObservation:
		if row.origin == "" || row.target == "" {
			return aggregateNegativeProofSupplementRow{}, false
		}
		row.claimForm = types.ClaimExternalObservation
	default:
		return aggregateNegativeProofSupplementRow{}, false
	}
	if row.searched == "" {
		row.searched = "current_investigation"
	}
	return row, true
}

func aggregateNegativeProofDetails(dims map[string]string) []aggregateNegativeProofDetail {
	if len(dims) == 0 {
		return nil
	}
	names := []string{
		"window_count",
		"window_size",
		"unmatched",
		"order",
		"window_path",
		"diff_path",
		"path",
		"source_ref",
		"tool_result",
		"payload_ref",
		"row_set_ref",
	}
	out := make([]aggregateNegativeProofDetail, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		value := strings.TrimSpace(dims[name])
		if value == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, aggregateNegativeProofDetail{name: name, value: value})
	}
	return out
}

func firstNonEmptyPreEmitDim(dims map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(dims[name]); value != "" {
			return value
		}
	}
	return ""
}

func aggregateNegativeProofSupplementTitle(zh bool) string {
	if zh {
		return "系统按已验证证据补充未命中范围"
	}
	return "System-verified no-hit scope supplement"
}

func aggregateNegativeProofSupplementText(rows []aggregateNegativeProofSupplementRow, zh bool) string {
	var b strings.Builder
	if zh {
		b.WriteString("以下内容来自已验收的结构化零结果证据，仅补充上方回答未显示的检索/观察范围，不替换模型回答：\n")
	} else {
		b.WriteString("The following lines come from accepted structured zero-result evidence and only supplement no-hit scopes not already visible above; they do not replace the model answer:\n")
	}
	for _, row := range rows {
		if zh {
			if row.kind == types.AnswerAggregateNegativeSearch {
				fmt.Fprintf(&b, "- 仓库/范围 `%s` 中检索 `%s`：0 个匹配（%s）%s。\n",
					row.scope, row.query, row.searched, aggregateNegativeProofDetailSuffix(row.details, zh))
			} else {
				fmt.Fprintf(&b, "- `%s` 在 `%s` 中观察 `%s`：0 个匹配（%s）%s。\n",
					aggregateNegativeProofOriginLabel(row.origin, zh), row.scope, row.target, row.searched, aggregateNegativeProofDetailSuffix(row.details, zh))
			}
			continue
		}
		if row.kind == types.AnswerAggregateNegativeSearch {
			fmt.Fprintf(&b, "- Search `%s` in `%s`: 0 matches (%s)%s.\n", row.query, row.scope, row.searched, aggregateNegativeProofDetailSuffix(row.details, zh))
		} else {
			fmt.Fprintf(&b, "- `%s` observed `%s` in `%s`: 0 matches (%s)%s.\n",
				aggregateNegativeProofOriginLabel(row.origin, zh), row.target, row.scope, row.searched, aggregateNegativeProofDetailSuffix(row.details, zh))
		}
	}
	return strings.TrimSpace(b.String())
}

func aggregateNegativeProofDetailSuffix(details []aggregateNegativeProofDetail, zh bool) string {
	if len(details) == 0 {
		return ""
	}
	parts := make([]string, 0, len(details))
	for _, detail := range details {
		if strings.TrimSpace(detail.name) == "" || strings.TrimSpace(detail.value) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", aggregateNegativeProofDetailLabel(detail.name, zh), detail.value))
	}
	if len(parts) == 0 {
		return ""
	}
	if zh {
		return "；范围细节：" + strings.Join(parts, "，")
	}
	return "; scope details: " + strings.Join(parts, ", ")
}

func aggregateNegativeProofDetailLabel(name string, zh bool) string {
	name = strings.TrimSpace(name)
	if !zh {
		switch name {
		case "window_count", "window_size":
			return "window"
		case "unmatched":
			return "unmatched"
		case "order":
			return "order"
		case "window_path":
			return "history path"
		case "diff_path":
			return "diff path"
		case "path":
			return "path"
		case "source_ref":
			return "source"
		case "tool_result":
			return "tool result"
		case "payload_ref":
			return "payload"
		case "row_set_ref":
			return "row set"
		default:
			return name
		}
	}
	switch name {
	case "window_count", "window_size":
		return "检索窗口"
	case "unmatched":
		return "未命中项"
	case "order":
		return "顺序"
	case "window_path":
		return "历史路径"
	case "diff_path":
		return "diff 路径"
	case "path":
		return "路径"
	case "source_ref":
		return "来源"
	case "tool_result":
		return "工具结果"
	case "payload_ref":
		return "载荷"
	case "row_set_ref":
		return "行集"
	default:
		return name
	}
}

func aggregateNegativeProofOriginLabel(origin string, zh bool) string {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		if zh {
			return "外部观察"
		}
		return "observation"
	}
	switch types.AnswerEvidenceOrigin(origin) {
	case types.AnswerEvidenceOriginVCSMetadata:
		if zh {
			return "版本历史"
		}
		return "VCS history"
	case types.AnswerEvidenceOriginVCSDiff:
		if zh {
			return "版本差异"
		}
		return "VCS diff"
	case types.AnswerEvidenceOriginRuntimeArtifact:
		if zh {
			return "运行时附件"
		}
		return "runtime artifact"
	case types.AnswerEvidenceOriginCommandMeasurement:
		if zh {
			return "命令输出"
		}
		return "command output"
	case types.AnswerEvidenceOriginCrossRepoIndex:
		if zh {
			return "多仓索引"
		}
		return "cross-repo index"
	case types.AnswerEvidenceOriginExternalDocument:
		if zh {
			return "外部文档"
		}
		return "external document"
	case types.AnswerEvidenceOriginWebPage:
		if zh {
			return "网页"
		}
		return "web page"
	case types.AnswerEvidenceOriginMCPResource:
		if zh {
			return "MCP 资源"
		}
		return "MCP resource"
	case types.AnswerEvidenceOriginConnectorResource:
		if zh {
			return "连接器资源"
		}
		return "connector resource"
	}
	return origin
}

func appendOrReuseNegativePreEmitCitation(doc *types.AnswerDocumentV2, cit types.Citation) int {
	if doc == nil || cit.Scope != types.ScopeNegative || strings.TrimSpace(cit.NegativePattern) == "" {
		return -1
	}
	file := preEmitNormalizePath(cit.File)
	pattern := strings.TrimSpace(cit.NegativePattern)
	for i, existing := range doc.Citations {
		if existing.Scope == types.ScopeNegative &&
			preEmitNormalizePath(existing.File) == file &&
			strings.TrimSpace(existing.NegativePattern) == pattern {
			return i
		}
	}
	doc.Citations = append(doc.Citations, cit)
	return len(doc.Citations) - 1
}

func preEmitAggregateSearchPatternAppears(pattern, surface string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return true
	}
	if preEmitAggregateDisplayPartAppears(pattern, surface) {
		return true
	}
	for _, token := range strings.FieldsFunc(pattern, func(r rune) bool {
		switch r {
		case '|', ',', '，', ';', '；', '/', '\\', '(', ')', '[', ']', '{', '}', '^', '$', '*', '+', '?':
			return true
		default:
			return unicode.IsSpace(r)
		}
	}) {
		token = strings.TrimSpace(token)
		if len([]rune(token)) < 3 {
			continue
		}
		if preEmitAggregateDisplayPartAppears(token, surface) {
			return true
		}
	}
	return false
}

func preEmitAggregateDimensionMap(dims []types.AnswerAggregateDimension) map[string]string {
	out := make(map[string]string, len(dims))
	for _, dim := range dims {
		name := strings.ToLower(strings.TrimSpace(dim.Name))
		value := strings.TrimSpace(dim.Value)
		if name == "" || value == "" || out[name] != "" {
			continue
		}
		out[name] = value
	}
	return out
}

func preEmitAggregateFactVisibilityKey(fact types.AnswerAggregateFact) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(strings.TrimSpace(string(fact.Kind))))
	b.WriteByte('\x00')
	b.WriteString(strings.ToLower(strings.TrimSpace(fact.Label)))
	b.WriteByte('\x00')
	b.WriteString(strings.ToLower(strings.TrimSpace(fact.Value)))
	b.WriteByte('\x00')
	b.WriteString(strings.ToLower(preEmitAggregateDimensionSummary(fact.Dimensions)))
	return b.String()
}

func preEmitAggregateFactExpectedShape(kind types.AnswerAggregateKind, label, value, unit string, dims []types.AnswerAggregateDimension) string {
	parts := []string{fmt.Sprintf("kind=%q", string(kind)), fmt.Sprintf("value=%q", value)}
	if label != "" {
		parts = append(parts, fmt.Sprintf("label=%q", label))
	}
	if unit != "" {
		parts = append(parts, fmt.Sprintf("unit=%q", unit))
	}
	if dimText := preEmitAggregateDimensionSummary(dims); dimText != "" {
		parts = append(parts, "dimensions=["+dimText+"]")
	}
	return strings.Join(parts, " ")
}

func preEmitAggregateDimensionSummary(dims []types.AnswerAggregateDimension) string {
	if len(dims) == 0 {
		return ""
	}
	parts := make([]string, 0, len(dims))
	for _, dim := range dims {
		name := strings.TrimSpace(dim.Name)
		value := strings.TrimSpace(dim.Value)
		if name == "" || value == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%q", name, value))
	}
	return strings.Join(parts, ", ")
}

func preCheckAggregateMemberSetCoverage(doc *types.AnswerDocumentV2, ctxOpt ...*types.BusContext) []emitFixHint {
	if doc == nil || len(ctxOpt) == 0 || ctxOpt[0] == nil || ctxOpt[0].Mutable == nil {
		return nil
	}
	ctx := ctxOpt[0]
	if answerDocumentRuntimeObservationOnly(ctx) {
		return nil
	}
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
	ctx := ctxOpt[0]
	rm := ctx.AnalysisIR.RequestModel
	if ctx.Mutable != nil && types.HasPrincipalOriginSpecificMemberSetForRequest(&rm, ctx.Mutable.StableInvestigationAggregateFacts()) {
		return true
	}
	return types.RequiresExhaustiveEnumerationMemberSetHandoff(rm) ||
		types.RequiresRelationMemberSetHandoff(rm)
}

func preEmitPrincipalAggregateMemberSetFactRefs(ctx *types.BusContext, facts []types.AnswerAggregateFact) []types.AnswerAggregateFactRef {
	facts = normalizeAggregateFactsForTypedExclusion(ctx, facts)
	facts = types.PruneAggregateMemberSetsByStructuredExclusions(facts)
	if ctx == nil || ctx.AnalysisIR == nil {
		return types.PrincipalAggregateMemberSetFactRefs(facts)
	}
	return types.PrincipalAggregateMemberSetFactRefsForRequest(facts, &ctx.AnalysisIR.RequestModel)
}

func preEmitPrincipalRelationMemberSetFactRefs(ctx *types.BusContext, facts []types.AnswerAggregateFact) []types.AnswerAggregateFactRef {
	facts = normalizeAggregateFactsForTypedExclusion(ctx, facts)
	facts = types.PruneAggregateMemberSetsByStructuredExclusions(facts)
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
	if answerDocumentRuntimeObservationOnly(ctx) {
		return nil
	}
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
		if preEmitSystemMissingMemberSupplementBlock(block) {
			continue
		}
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

func preEmitSystemMissingMemberSupplementBlock(block types.AnswerBlock) bool {
	title := strings.TrimSpace(block.Title)
	return strings.HasPrefix(title, "系统按已验证证据补充缺失成员：") ||
		strings.HasPrefix(title, "System-verified missing member supplement:")
}

func preEmitSystemEnumerationRowSupplementBlock(block types.AnswerBlock) bool {
	title := strings.TrimSpace(block.Title)
	return preEmitSystemMissingMemberSupplementBlock(block) ||
		strings.HasPrefix(title, "系统按已验证证据补充可校验字段：") ||
		strings.HasPrefix(title, "System-verified field supplement:") ||
		strings.HasPrefix(title, "系统按已验证证据补充说明：") ||
		strings.HasPrefix(title, "System-verified note supplement:")
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
	matched := make(map[int]bool, len(fact.Members))
	for _, block := range doc.Blocks {
		switch block.Kind {
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		default:
			continue
		}
		for idx, member := range fact.Members {
			if matched[idx] {
				continue
			}
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
	if preEmitAggregateCommitMemberAppears(member, surface) {
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

func preEmitAggregateCommitMemberAppears(member, surface string) bool {
	memberHash := principalEnumerationLeadingCommitHash(member)
	if memberHash == "" {
		return false
	}
	for _, token := range principalEnumerationCodeTokens(surface) {
		if principalEnumerationCommitHashPrefixMatch(token, memberHash) {
			return true
		}
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

func preEmitUniqueExactEndpointDefinitionCitationForLabelWithContext(pctx *preEmitCheckContext, label string) (types.Citation, bool) {
	if pctx == nil {
		return types.Citation{}, false
	}
	labelKey := preEmitCodeIdentityKey(preEmitDefinitionCitationLabelBase(label))
	if labelKey == "" {
		return types.Citation{}, false
	}
	type candidate struct {
		cit   types.Citation
		score int
	}
	var candidates []candidate
	seen := map[string]bool{}
	for _, ev := range pctx.evidenceItems() {
		if ev.GroundingStatus == types.GroundingUngrounded || ev.Source == "" || ev.LineStart <= 0 {
			continue
		}
		if !preEmitEvidenceExactEndpointKeyMatches(ev, labelKey) {
			continue
		}
		score := preEmitExactEndpointDefinitionScore(ev)
		if score <= 0 {
			continue
		}
		cit := pctx.canonicalCitation(types.Citation{File: ev.Source, Line: ev.LineStart})
		if cit.File == "" || cit.Line <= 0 {
			continue
		}
		key := preEmitCitationLocationKey(cit)
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, candidate{cit: cit, score: score})
	}
	if len(candidates) == 0 {
		return types.Citation{}, false
	}
	bestScore := 0
	for _, c := range candidates {
		if c.score > bestScore {
			bestScore = c.score
		}
	}
	var best []types.Citation
	for _, c := range candidates {
		if c.score == bestScore {
			best = append(best, c.cit)
		}
	}
	if len(best) != 1 {
		return types.Citation{}, false
	}
	return best[0], true
}

func preEmitDefinitionCitationLabelBase(label string) string {
	label = strings.TrimSpace(label)
	if base, _, ok := types.AnswerAggregateDecoratedLabelParts(label); ok {
		return strings.TrimSpace(base)
	}
	return label
}

func preEmitEvidenceExactEndpointKeyMatches(ev types.EvidenceItem, labelKey string) bool {
	if labelKey == "" {
		return false
	}
	for _, endpoint := range []string{ev.Subject, ev.Object, ev.AnchorSymbol, ev.OwnerSymbol} {
		if preEmitCodeIdentityKey(endpoint) == labelKey {
			return true
		}
	}
	return false
}

func preEmitExactEndpointDefinitionScore(ev types.EvidenceItem) int {
	switch ev.AnchorKind {
	case types.AnchorDefinition:
		if ev.Kind == types.EvidenceDirect || ev.Kind == types.EvidenceRegistration {
			return 100
		}
		return 90
	case types.AnchorInitializer, types.AnchorAssignment, types.AnchorImport:
		if ev.Kind == types.EvidenceDirect || ev.Kind == types.EvidenceRegistration {
			return 70
		}
		return 60
	default:
		if ev.Kind == types.EvidenceDirect || ev.Kind == types.EvidenceRegistration {
			return 40
		}
		return 0
	}
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
			Reason: "enumeration item labels should lead with codebase-grounded identifiers; otherwise the post-emit shape contract records a citation-grounding caveat unless this kind is strict-promoted.",
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
	fixed += normalizeImplicitDefinitionClaimUses(doc, view)
	fixed += normalizeAutoRepairableRequiredFacetIDs(doc, view)
	fixed += normalizeAbsentExactResolutionScalarBlocks(doc, view)
	return fixed
}

// normalizeObservedArtifactClaimUseCarriers repairs the non-visible typed
// carrier for runtime/log/trace observations. If the typed bus contract says an
// observed-artifact lane is required and the model already marked a visible
// block with facet_id=observed_artifact_fact, adding
// claim_form=external_observation is schema metadata over existing content. It
// does not author a new answer fact, does not inspect user/model prose, and
// avoids turning a local carrier omission into a generic user-facing acceptance
// caveat.
func normalizeObservedArtifactClaimUseCarriers(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || !types.AnswerRequiresObservedArtifactCarrier(ctx) {
		return 0
	}
	const facet = string(types.FacetObservedArtifactFact)
	fixed := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if !preEmitBlockHasVisiblePayload(*block) {
			continue
		}
		if !preEmitBlockSharesFacet(*block, []string{facet}) {
			continue
		}
		if blockHasObservedArtifactClaimUse(*block) {
			continue
		}
		block.ClaimUses = append(block.ClaimUses, types.RenderedClaimUse{
			ClaimForm: types.ClaimExternalObservation,
			FacetID:   facet,
		})
		fixed++
	}
	return fixed
}

func blockHasObservedArtifactClaimUse(block types.AnswerBlock) bool {
	const facet = string(types.FacetObservedArtifactFact)
	blockDeclaresFacet := false
	for _, id := range block.FacetIDs {
		if strings.TrimSpace(id) == facet {
			blockDeclaresFacet = true
			break
		}
	}
	for _, cu := range block.ClaimUses {
		if cu.ClaimForm != types.ClaimExternalObservation {
			continue
		}
		if strings.TrimSpace(cu.FacetID) == facet || blockDeclaresFacet {
			return true
		}
	}
	return false
}

func normalizeCitationBackedPrincipalClaimUses(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, pctx *preEmitCheckContext) int {
	if doc == nil || view == nil || pctx == nil || pctx.ctx == nil || pctx.ctx.Mutable == nil {
		return 0
	}
	fixed := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if hasAnyClaimUse(*block) || types.AnswerBlockHasActiveTypedDecisionCarrier(*block, view) {
			continue
		}
		if !preEmitBlockRendersItemSurface(block.Kind) || len(block.Items) == 0 {
			continue
		}
		if !preEmitBlockIsPrincipalForView(*block, view) {
			continue
		}
		allowed := preEmitAllowedClaimFormsForBlock(*block, view)
		if len(allowed) == 0 {
			continue
		}
		forms := citationBackedClaimFormsForBlock(*block, doc, pctx, allowed)
		if len(forms) == 0 {
			continue
		}
		facetID := ""
		if len(block.FacetIDs) == 1 {
			facetID = strings.TrimSpace(block.FacetIDs[0])
		}
		for _, form := range forms {
			block.ClaimUses = append(block.ClaimUses, types.RenderedClaimUse{
				FacetID:   facetID,
				ClaimForm: form,
			})
		}
		fixed++
	}
	return fixed
}

func preEmitBlockIsPrincipalForView(block types.AnswerBlock, view *types.AnswerSemanticView) bool {
	if block.SurfaceRole == types.SurfacePrincipal {
		return true
	}
	if block.SurfaceRole != "" || view == nil {
		return false
	}
	for _, req := range append(append([]types.BlockRequirement(nil), view.RequiredBlocks...), view.OptionalBlocks...) {
		if req.SurfaceRoleHint != types.SurfacePrincipal || !req.AcceptsKind(block.Kind) {
			continue
		}
		if len(req.FacetIDs) > 0 && !preEmitBlockSharesFacet(block, req.FacetIDs) {
			continue
		}
		return true
	}
	return false
}

func preEmitAllowedClaimFormsForBlock(block types.AnswerBlock, view *types.AnswerSemanticView) []types.ClaimForm {
	if view == nil {
		return nil
	}
	var out []types.ClaimForm
	seen := map[types.ClaimForm]bool{}
	for _, req := range append(append([]types.BlockRequirement(nil), view.RequiredBlocks...), view.OptionalBlocks...) {
		if !req.AcceptsKind(block.Kind) || len(req.AcceptableClaimForms) == 0 {
			continue
		}
		if len(req.FacetIDs) > 0 && !preEmitBlockSharesFacet(block, req.FacetIDs) {
			continue
		}
		for _, form := range req.AcceptableClaimForms {
			if form == "" || seen[form] {
				continue
			}
			seen[form] = true
			out = append(out, form)
		}
	}
	return out
}

func citationBackedClaimFormsForBlock(block types.AnswerBlock, doc *types.AnswerDocumentV2, pctx *preEmitCheckContext, allowed []types.ClaimForm) []types.ClaimForm {
	if doc == nil || pctx == nil || len(allowed) == 0 {
		return nil
	}
	allowedSet := make(map[types.ClaimForm]bool, len(allowed))
	for _, form := range allowed {
		if form != "" {
			allowedSet[form] = true
		}
	}
	seen := map[types.ClaimForm]bool{}
	var out []types.ClaimForm
	for _, item := range block.Items {
		if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
			continue
		}
		cited, found := pctx.citedEvidenceItems(doc.Citations[item.CitationRef])
		if !found {
			continue
		}
		for _, ev := range cited {
			form := types.ClaimFormOf(ev)
			if form == "" || form == types.ClaimUnknown || !allowedSet[form] || seen[form] {
				continue
			}
			seen[form] = true
			out = append(out, form)
		}
	}
	return out
}

func normalizeAbsentExactResolutionScalarBlocks(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) int {
	if doc == nil || doc.ExactResolution == nil || doc.ExactResolution.Status != types.AnswerExactResolutionAbsent {
		return 0
	}
	if view == nil || view.ExactResolution == nil || len(view.ExactResolution.Targets) != 1 {
		return 0
	}
	if len(doc.Blocks) == 0 {
		return 0
	}
	filtered := doc.Blocks[:0]
	removed := 0
	for _, block := range doc.Blocks {
		if block.Kind == types.BlockScalar {
			removed++
			continue
		}
		filtered = append(filtered, block)
	}
	if removed == 0 {
		return 0
	}
	doc.Blocks = filtered
	return removed
}

// NormalizeAnswerDocumentForRecovery applies the same deterministic
// compatibility repairs used by emit_answer_document, then re-runs the
// pre-emit checks through the same hard/advisory split. It is intentionally
// exported for orchestrator's transient-failure salvage path: a rejected draft
// may be shown to users when it has no currently-hard structural blocker.
func NormalizeAnswerDocumentForRecovery(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctx *types.BusContext) (changed bool, ok bool) {
	if doc == nil || view == nil {
		return false, false
	}
	before, _ := json.Marshal(doc)
	pctx := newPreEmitCheckContext(ctx)
	normalizeAnswerDocumentForPreEmit("answer_document_recovery", doc, view, ctx, pctx)
	after, _ := json.Marshal(doc)
	hints := runPreEmitChecksWithContext(doc, view, preEmitOracleFromCtx(ctx), pctx)
	hardHints, _ := splitPreEmitHintsByGate(hints)
	return !bytes.Equal(before, after), len(hardHints) == 0
}

func normalizeAutoRepairableRequiredFacetIDs(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) int {
	if doc == nil || view == nil || view.FacetCoverage == nil || len(view.FacetCoverage.Required) == 0 {
		return 0
	}
	target := -1
	fixed := 0
	for _, req := range view.FacetCoverage.Required {
		facet := strings.TrimSpace(string(req.Kind))
		if facet == "" || !req.RequiresHardDeclaration() || !preEmitAutoRepairableFacetID(facet) {
			continue
		}
		if answerDocumentDeclaresFacetID(doc, facet) {
			continue
		}
		if target < 0 {
			target = preEmitAutoRepairFacetTarget(doc)
		}
		if target < 0 {
			continue
		}
		doc.Blocks[target].FacetIDs = mergeStringSet(doc.Blocks[target].FacetIDs, []string{facet})
		fixed++
	}
	return fixed
}

func preEmitAutoRepairableFacetID(facet string) bool {
	switch types.AnswerFacetKind(strings.TrimSpace(facet)) {
	case types.FacetCurrentCodePath, types.FacetResolvedLiteralOrSymbol:
		// These are carrier metadata over already-rendered code paths /
		// symbols. Adding the missing facet_id to a visible principal
		// block does not invent a user-visible claim or change the answer
		// shape, and avoids a full finalizer rewrite for non-visible JSON
		// metadata.
		return true
	default:
		// Shape-bearing facets (diagram_spine, enumeration_item, bucket
		// labels, path edges, guards, etc.) stay with the normal typed gate:
		// auto-declaring them could hide a genuinely missing block, row, or
		// diagram.
		return false
	}
}

func answerDocumentDeclaresFacetID(doc *types.AnswerDocumentV2, facet string) bool {
	if doc == nil || facet == "" {
		return false
	}
	for i := range doc.Blocks {
		for _, fid := range doc.Blocks[i].FacetIDs {
			if strings.TrimSpace(fid) == facet {
				return true
			}
		}
		for _, claim := range doc.Blocks[i].ClaimUses {
			if strings.TrimSpace(claim.FacetID) == facet {
				return true
			}
		}
	}
	return false
}

func preEmitAutoRepairFacetTarget(doc *types.AnswerDocumentV2) int {
	if doc == nil || len(doc.Blocks) == 0 {
		return -1
	}
	for i, block := range doc.Blocks {
		if block.Kind == types.BlockCaveat || block.SurfaceRole != types.SurfacePrincipal {
			continue
		}
		if preEmitBlockCanCarryAutoFacet(doc, block) {
			return i
		}
	}
	for i, block := range doc.Blocks {
		if block.Kind == types.BlockCaveat {
			continue
		}
		if preEmitBlockCanCarryAutoFacet(doc, block) {
			return i
		}
	}
	return -1
}

func preEmitBlockCanCarryAutoFacet(doc *types.AnswerDocumentV2, block types.AnswerBlock) bool {
	if !preEmitBlockHasVisiblePayload(block) {
		return false
	}
	if preEmitBlockHasCitationSurface(block) {
		return true
	}
	return doc != nil && len(doc.Citations) > 0
}

func preEmitBlockHasVisiblePayload(block types.AnswerBlock) bool {
	if strings.TrimSpace(block.Title) != "" || strings.TrimSpace(block.Text) != "" || len(block.Items) > 0 {
		return true
	}
	return block.Diagram != nil && strings.TrimSpace(block.Diagram.Body) != ""
}

func preEmitBlockHasCitationSurface(block types.AnswerBlock) bool {
	for _, item := range block.Items {
		if item.CitationRef >= 0 {
			return true
		}
	}
	return false
}

func normalizeImplicitDefinitionClaimUses(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) int {
	if doc == nil || view == nil {
		return 0
	}
	reqByKind := make(map[types.AnswerBlockKind]types.BlockRequirement, len(view.RequiredBlocks))
	for _, r := range view.RequiredBlocks {
		for _, kind := range r.AcceptedKinds() {
			if _, ok := reqByKind[kind]; !ok {
				reqByKind[kind] = r
			}
		}
	}
	fixed := 0
	for i := range doc.Blocks {
		b := &doc.Blocks[i]
		req, ok := reqByKind[b.Kind]
		if !ok || len(req.AcceptableClaimForms) == 0 {
			continue
		}
		isPrincipal := b.SurfaceRole == types.SurfacePrincipal ||
			(b.SurfaceRole == "" && req.SurfaceRoleHint == types.SurfacePrincipal)
		if !isPrincipal || hasAnyClaimUse(*b) || types.AnswerBlockHasActiveTypedDecisionCarrier(*b, view) {
			continue
		}
		if !claimFormAllowed(req.AcceptableClaimForms, types.ClaimDefinitionFact) {
			continue
		}
		switch b.Kind {
		case types.BlockSummary, types.BlockSection:
		default:
			continue
		}
		claim := types.RenderedClaimUse{ClaimForm: types.ClaimDefinitionFact}
		if len(b.FacetIDs) == 1 {
			claim.FacetID = strings.TrimSpace(b.FacetIDs[0])
		}
		b.ClaimUses = append(b.ClaimUses, claim)
		fixed++
	}
	return fixed
}

func claimFormAllowed(forms []types.ClaimForm, want types.ClaimForm) bool {
	for _, form := range forms {
		if form == want {
			return true
		}
	}
	return false
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
		ExpectedShape: "structured answer anchor label(s) must preserve: " + strings.Join(labels, ", ") + ". Add or keep an ordered_list/table item whose label is exactly each missing anchor; use a matching citation_ref when available, otherwise leave it uncited.",
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
	blockIdx := requiredMechanismAnchorBlockIndex(doc)
	if blockIdx < 0 {
		if !shouldMaterializeRequiredMechanismAnchorBlock(view, ctx) {
			return 0
		}
		blockIdx = appendRequiredMechanismAnchorBlock(doc, ctx)
	}
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

func shouldMaterializeRequiredMechanismAnchorBlock(view *types.AnswerSemanticView, ctx *types.BusContext) bool {
	if view == nil || ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	return view.RequiresAnchorSkeleton(ctx.AnalysisIR.RequestModel)
}

func requiredMechanismAnchorBlockIndex(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return -1
	}
	for i, block := range doc.Blocks {
		if block.ID == "required_mechanism_anchors" {
			return i
		}
	}
	return -1
}

func appendRequiredMechanismAnchorBlock(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil {
		return -1
	}
	block := types.AnswerBlock{
		ID:    uniqueRequiredMechanismAnchorBlockID(doc),
		Kind:  types.BlockOrderedList,
		Title: requiredMechanismAnchorBlockTitle(ctx),
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

func requiredMechanismAnchorBlockTitle(ctx *types.BusContext) string {
	if answerDocumentRequiresChinese(requestedAnswerDocumentLanguage(ctx)) {
		return "关键锚点"
	}
	return "Key anchors"
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
// are checked here and then routed through the central hard/advisory split:
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

func emitFixHintsRepair(hints []emitFixHint) *types.ToolRepair {
	if len(hints) == 0 {
		return nil
	}
	fields := make([]string, 0, len(hints))
	kinds := make([]string, 0, len(hints))
	shapes := make([]string, 0, len(hints))
	seenFields := map[string]struct{}{}
	seenKinds := map[string]struct{}{}
	for _, h := range hints {
		if field := strings.TrimSpace(h.Field); field != "" {
			if _, ok := seenFields[field]; !ok {
				seenFields[field] = struct{}{}
				fields = append(fields, field)
			}
		}
		if h.Kind != "" {
			kind := string(h.Kind)
			if _, ok := seenKinds[kind]; !ok {
				seenKinds[kind] = struct{}{}
				kinds = append(kinds, kind)
			}
		}
		if shape := strings.TrimSpace(h.ExpectedShape); shape != "" && len(shapes) < 3 {
			shapes = append(shapes, shape)
		}
	}
	meta := map[string]string{
		"hint_count": strconv.Itoa(len(hints)),
	}
	if len(kinds) > 0 {
		meta["violation_kinds"] = strings.Join(kinds, ",")
	}
	if len(shapes) > 0 {
		meta["expected_shapes"] = strings.Join(shapes, " | ")
	}
	return &types.ToolRepair{
		Code:     "answer_doc_pre_emit_contract",
		Fields:   fields,
		Hint:     "Apply these typed answer-document schema corrections and re-emit the same tool call. Preserve existing answer facts and change only the listed fields or missing block carriers.",
		Metadata: meta,
	}
}

func formatEmitAdvisoryHints(hints []emitFixHint) string {
	if len(hints) == 0 {
		return ""
	}
	const maxHints = 4
	parts := make([]string, 0, min(len(hints), maxHints)+1)
	for i, h := range hints {
		if i >= maxHints {
			parts = append(parts, fmt.Sprintf("+%d more", len(hints)-maxHints))
			break
		}
		field := strings.TrimSpace(h.Field)
		if field == "" {
			field = "answer field"
		}
		action := strings.TrimSpace(h.ExpectedShape)
		if action == "" {
			parts = append(parts, field)
			continue
		}
		parts = append(parts, field+": "+action)
	}
	return strings.Join(parts, "; ")
}
