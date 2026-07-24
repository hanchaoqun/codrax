package tool

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/reasoninggraph"
	"github.com/hanchaoqun/codrax/internal/sourceowner"
	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_mutation_runtime.go — B4 v3 (2026-05-04). The
// single write closure for V2 answer-document emits. Both full
// (emit_answer_document) and patch (emit_answer_document_patch)
// paths converge here so merged-doc validation, persistence, and
// telemetry are byte-identical between the two submission shapes.
//
// Pre-v3 design split the path:
//   - executeAnswerDocumentV2          → SetAnswerDocumentV2(merged)
//   - EmitAnswerDocumentPatch.Execute  → SetAnswerDocumentV2FromPatch(merged)
//                                       + per-block id-uniqueness check
//                                       + per-block diagram-payload check
//
// v3 collapses the validation + setter + Summary + telemetry into
// ApplyAndPersistMutation. The two callers now only differ in how
// they BUILD the AnswerDocumentMutation: ReplaceAll for full,
// Partial for patch.
//
// Maintenance contract:
//   - Every callsite that wants to write a V2 doc to MutableState
//     MUST go through ApplyAndPersistMutation. Direct calls to
//     SetAnswerDocumentV2WithMutation are reserved for tests / fake
//     constructors.
//   - Adding a merged-doc invariant (block-id uniqueness, diagram
//     payload, max blocks, future ones) lives here so both paths
//     pick it up automatically.

// ApplyAndPersistMutation is the unified write closure.
//
// Behaviour:
//  1. Apply the typed AnswerDocumentMutation onto prev (mutation.Apply
//     handles ReplaceAll vs Partial dispatch).
//  2. Validate merged doc invariants:
//     - every block has a non-empty id
//     - block ids are unique
//     - kind=diagram blocks carry a non-nil Diagram payload
//     - explicit diagram payloads with a stale non-diagram discriminator
//     are normalized to kind=diagram before validation
//     (More invariants extend this list — keep them merged-doc-shape
//     checks rather than per-emit-input checks so both paths share
//     them.)
//  3. Persist via ctx.Mutable.SetAnswerDocumentV2WithMutation(merged,
//     mutation.Kind). LastEmitFromPatch flag is set automatically.
//  4. Log mutation.Summary() at info level for operator audit.
//  5. Return ToolResult{Summary: "<toolName> accepted V2 carrier:
//     <mutation.Summary()>"} on success; failEmit on any rejection.
//
// All callers (executeAnswerDocumentV2 + EmitAnswerDocumentPatch.Execute)
// share this surface; their job is reduced to building the typed
// AnswerDocumentMutation.
func ApplyAndPersistMutation(
	ctx *types.BusContext,
	toolName string,
	mutation types.AnswerDocumentMutation,
	prev *types.AnswerDocumentV2,
	now time.Time,
) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return failEmit(toolName, now,
			"%s requires a writable context", toolName)
	}

	merged, err := mutation.Apply(prev)
	if err != nil {
		return failEmitWithRepair(toolName, now, answerDocumentMutationRepair(err),
			"mutation apply rejected: %v", err)
	}
	if merged == nil {
		return failEmit(toolName, now,
			"mutation apply produced a nil document — internal error")
	}
	return persistMergedAnswerDocument(ctx, toolName, mutation.Kind, mutation.Summary(), merged, now)
}

func answerDocumentMutationRepair(err error) *types.ToolRepair {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "replace_citations and append_citations are mutually exclusive"):
		return &types.ToolRepair{
			Code:   "answer_doc_patch_citation_mode_conflict",
			Fields: []string{"replace_citations", "append_citations"},
			Hint:   "Re-emit `emit_answer_document_patch` with exactly one citation-pool operation: use `append_citations` when only adding citations, or `replace_citations` only when every citation_ref-bearing block is also replaced/removed. If many citations change, switch to a full `emit_answer_document` payload.",
		}
	case strings.Contains(msg, "add_blocks[") && strings.Contains(msg, "already exists in previous emit"):
		return &types.ToolRepair{
			Code:   "answer_doc_patch_existing_block",
			Fields: []string{"add_blocks", "replace_blocks", "unchanged_block_ids"},
			Hint:   "Move any block id that already exists in the previous emit out of `add_blocks`. Put the edited block in `replace_blocks`, or list it in `unchanged_block_ids` when it should stay byte-identical.",
		}
	case strings.Contains(msg, "replace_citations cannot preserve citation-bearing block"):
		return &types.ToolRepair{
			Code:   "answer_doc_patch_replace_citations_with_preserved_blocks",
			Fields: []string{"replace_citations", "append_citations", "replace_blocks", "remove_block_ids"},
			Hint:   "Do not replace the citation pool while preserving old blocks that still contain citation_ref values. Use `append_citations`, replace/remove every citation-bearing block, or switch to a full `emit_answer_document` payload with a complete zero-based citation pool.",
		}
	default:
		return nil
	}
}

func persistMergedAnswerDocument(
	ctx *types.BusContext,
	toolName string,
	kind types.MutationKind,
	mutationSummary string,
	merged *types.AnswerDocumentV2,
	now time.Time,
) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return failEmit(toolName, now,
			"%s requires a writable context", toolName)
	}
	if merged == nil {
		return failEmit(toolName, now,
			"mutation apply produced a nil document — internal error")
	}
	if canonicalizeSummaryLeadBlock(merged) {
		logging.Info("[%s] canonicalized summary block to lead position before persist", toolName)
	}
	if view := types.BuildAnswerSemanticViewForBusContext(ctx); view != nil {
		if fixed := normalizeViewCompatibleAnswerDocument(merged, view); fixed > 0 {
			logging.Warning("[%s] repaired %d view-compatible typed lane field(s) before persist", toolName, fixed)
		}
	}
	if fixed := normalizeMergedDiagramPayloadKinds(merged); fixed > 0 {
		logging.Warning("[%s] repaired %d diagram block discriminator(s) before persist", toolName, fixed)
	}
	// Runtime-trace IDs are a reserved system namespace. The model still owns
	// arbitrary block IDs, so normalize an exact collision before any
	// idempotence/order/evidence consumer sees the merged document. The
	// internal SystemGeneratedKind marker is json:"-" and therefore the only
	// authority that lets a reserved ID retain its spelling.
	if fixed := normalizeRuntimeTraceReservedBlockIDCollisions(merged); fixed > 0 {
		logging.Warning("[%s] renamed %d model-authored runtime-trace reserved block id collision(s) before materialization", toolName, fixed)
	}
	if fixed := normalizePriorityInversionCandidateAnswerSurface(merged, ctx); fixed > 0 {
		logging.Warning("[%s] repaired %d priority-inversion claim(s) to typed authority-calibrated wording before persist", toolName, fixed)
	}
	if fixed := normalizeRuntimeTraceLowCoverageRootCauseSurface(merged, ctx); fixed > 0 {
		logging.Warning("[%s] weakened %d whole-frame root-cause claim(s) to low-coverage candidate wording before persist", toolName, fixed)
	}
	if materializeRuntimeTraceArithmeticRelationCaveat(merged, ctx) {
		logging.Info("[%s] materialized runtime trace arithmetic relation caveat without rewriting model prose", toolName)
	}
	if materializeRuntimeTraceFrequencyAuthorityCaveat(merged, ctx) {
		logging.Info("[%s] materialized runtime trace frequency transition authority caveat", toolName)
	}
	if materializeRuntimeTraceCausalProjectionBlock(merged, ctx) {
		logging.Info("[%s] materialized runtime trace causal projection from structured trace observations", toolName)
	}
	if materializeRuntimeTraceSemanticOptimizationBlock(merged, ctx) {
		logging.Info("[%s] materialized runtime trace deterministic optimization points from typed semantic spans", toolName)
	}
	if materializeRuntimeTraceMetricSnapshotBlock(merged, ctx) {
		logging.Info("[%s] materialized runtime trace metric snapshot from structured observation notes", toolName)
	}
	if materializeRuntimeTraceNextStepsBlock(merged, ctx) {
		logging.Info("[%s] materialized runtime trace next-step block from structured observation notes", toolName)
	}
	if materializeRuntimeTracePerfQualityBlock(merged, ctx) {
		logging.Info("[%s] materialized runtime trace perf quality block from structured observation notes", toolName)
	}
	if fixed := normalizeHarmonyPriorityAnswerSurface(merged, ctx); fixed > 0 {
		logging.Warning("[%s] repaired %d Harmony priority class surface(s) from typed prio/class facts", toolName, fixed)
	}
	if materializeRuntimeTraceObservationBlock(merged, ctx) {
		logging.Info("[%s] materialized runtime trace observation block from structured perf facts", toolName)
	}
	if materializeRuntimeTraceSupplementDisclosureCaveat(merged, ctx) {
		logging.Info("[%s] stamped the system trace supplement disclosure caveat (SUPP-CORE single-line provenance)", toolName)
	}
	normalizeAnswerDocumentRowsBeforePersist(toolName, ctx, merged)
	if stamped := stampReadOwnerAnchorsFromTurnA(ctx, merged); stamped > 0 {
		logging.Info("[%s] stamped %d read owner anchor(s) from typed source localization", toolName, stamped)
	}
	if stampReadNavigationCoverageFromTurnA(ctx, merged) {
		logging.Info("[%s] stamped read repo_map navigation coverage from typed TurnA observations", toolName)
	}
	if stampReadLocalizerFollowup(ctx, merged) {
		logging.Info("[%s] stamped read localizer follow-up from typed localization/navigation state", toolName)
	}
	if stampReadReasoningGraph(ctx, merged) {
		logging.Info("[%s] stamped read reasoning graph summary from typed read artifacts", toolName)
	}
	if fixed := normalizeHarmonyPriorityAnswerSurface(merged, ctx); fixed > 0 {
		logging.Warning("[%s] repaired %d late Harmony priority class surface(s) from typed prio/class facts", toolName, fixed)
	}
	if moved := normalizeRuntimeTraceReportHierarchy(merged); moved > 0 {
		logging.Info("[%s] reordered %d runtime trace report block(s) into decision-first hierarchy", toolName, moved)
	}
	if deduped := dedupeVisibleAnswerBlocks(merged); deduped > 0 {
		logging.Warning("[%s] dropped %d duplicate visible answer block(s) before persist", toolName, deduped)
	}
	// QCE GAP-B (2026-07-05): the pre-persist row normalization above can
	// mint fresh bare file:line citations too (same appendOrReuse shape as
	// the pre-emit chain). Same gated backfill as the chain end so
	// system-rebuilt references persist with a source-line quote.
	if answerDocumentHasQuotelessCurrentSourceCitation(merged, ctx) {
		if fixed := normalizeCurrentSourceCitationQuotes(merged, ctx); fixed > 0 {
			logging.Warning("[%s] backfilled %d quoteless citation quote(s) from current source before persist", toolName, fixed)
		}
	}
	// XGAP-FIX ⑤ (§29.104.8): runtime-artifact citation quote check —
	// healthy-path wiring of the independent arm (detect → disclose, never
	// reject). Current-source quotes above are deterministically
	// backfilled; artifact quotes could not be, so mismatches are
	// disclosed instead.
	if flagged := verifyRuntimeArtifactCitationQuotes(merged, ctx); flagged > 0 {
		logging.Warning("[%s] disclosed %d runtime-artifact citation quote mismatch(es) before persist", toolName, flagged)
	}
	// QCE GAP-A (2026-07-05): this is the LAST content-mutating point
	// before persist (row normalization above can still prune items;
	// dedupe can drop whole blocks). Materialize the detach disclosure
	// HERE from the typed records the pre-emit chain ferried over, so the
	// caveat wording is computed from each item's actual final presence
	// in the merged document — disposal and wording physically share one
	// signal. Take() consumes the records so a failed persist cannot
	// leak them into an unrelated later persist.
	materializeDetachedCitationRefCaveats(merged, ctx, ctx.Mutable.TakePendingDetachedCitationDisclosures())
	// SPR #72 (RTC §8.3): stamp the current-status verdict evidence
	// downgrade from the origin-lane observation ledger. Must run after
	// every block-mutating pass above so the stamp targets the final
	// decision block; the block's own verdict field is never modified
	// (audit position).
	if stampCurrentStatusVerdictEvidenceDowngrade(ctx, merged) {
		logging.Info("[%s] current-status verdict downgraded to not-evaluable disclosure: origin-lane ledger has zero current_source evidence this run (original verdict retained for audit)", toolName)
	}
	if vErr := validateMergedV2Doc(merged); vErr != nil {
		return failEmit(toolName, now, "%s", vErr.Error())
	}

	ctx.Mutable.SetAnswerDocumentV2WithMutation(kind, merged)
	ctx.Mutable.SetAnswerDisplayAttachments(filterAcceptedAnswerDisplayAttachments(merged, ctx.Mutable.AnswerDisplayAttachments()))
	logging.Info("[%s] mutation: %s", toolName, mutationSummary)

	return types.ToolResult{
		ToolName: toolName,
		Success:  true,
		Summary: fmt.Sprintf(
			"%s accepted: %s%s",
			toolName, mutationSummary,
			summarizeV2Blocks(merged.Blocks)),
		Timestamp: now,
	}, nil
}

func normalizeAnswerDocumentRowsBeforePersist(toolName string, ctx *types.BusContext, doc *types.AnswerDocumentV2) {
	if doc == nil || ctx == nil {
		return
	}
	itemsBefore := answerDocumentStructuredItemCount(doc)
	if fixed := compileEnumerationDisplayTableRows(doc, ctx); fixed > 0 {
		logging.Warning("[%s] compiled %d deterministic enumeration table row(s) from accepted principal evidence handoff before persist", toolName, fixed)
	}
	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed > 0 {
		logging.Warning("[%s] normalized %d principal enumeration block(s) from accepted evidence-rich row contract before persist", toolName, fixed)
	}
	if fixed := normalizeAggregateMemberSetCarriers(doc, ctx); fixed > 0 {
		logging.Warning("[%s] materialized %d principal aggregate member row(s) from accepted exhaustive enumeration handoff before persist", toolName, fixed)
	}
	if itemsAfter := answerDocumentStructuredItemCount(doc); itemsAfter < itemsBefore {
		if fixed := normalizeUnusedCitationPoolEntries(doc); fixed > 0 {
			logging.Warning("[%s] pruned/remapped %d unused citation-pool slot(s) after pre-persist answer-row normalization", toolName, fixed)
		}
	}
}

func answerDocumentStructuredItemCount(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return 0
	}
	count := 0
	for _, block := range doc.Blocks {
		count += len(block.Items)
	}
	return count
}

func dedupeVisibleAnswerBlocks(doc *types.AnswerDocumentV2) int {
	if doc == nil || len(doc.Blocks) < 2 {
		return 0
	}
	out := make([]types.AnswerBlock, 0, len(doc.Blocks))
	seen := make(map[string]bool, len(doc.Blocks))
	dropped := 0
	for _, block := range doc.Blocks {
		keys := visibleAnswerBlockDedupeKeys(block, doc.Citations)
		if anyVisibleAnswerBlockDedupeKeySeen(seen, keys) {
			dropped++
			continue
		}
		for _, key := range keys {
			seen[key] = true
		}
		out = append(out, block)
	}
	if dropped > 0 {
		doc.Blocks = out
	}
	return dropped
}

func visibleAnswerBlockDedupeKeys(block types.AnswerBlock, citations []types.Citation) []string {
	keys := make([]string, 0, 2)
	if key := exactVisibleAnswerBlockDedupeKey(block, citations); key != "" {
		keys = append(keys, "exact\x00"+key)
	}
	if key := semanticPrincipalAnswerBlockDedupeKey(block, citations); key != "" {
		keys = append(keys, "semantic_principal\x00"+key)
	}
	return keys
}

func anyVisibleAnswerBlockDedupeKeySeen(seen map[string]bool, keys []string) bool {
	for _, key := range keys {
		if seen[key] {
			return true
		}
	}
	return false
}

func exactVisibleAnswerBlockDedupeKey(block types.AnswerBlock, citations []types.Citation) string {
	if block.Kind == types.BlockDiagram {
		return ""
	}
	if !types.AnswerBlockKindRendersStructuredItems(block.Kind) || len(block.Items) == 0 {
		return ""
	}
	surface := strings.TrimSpace(types.AnswerBlockVisibleSurface(block))
	if surface == "" {
		return ""
	}
	return string(block.Kind) + "\x00" +
		string(block.SurfaceRole) + "\x00" +
		strings.Join(block.FacetIDs, "\x1f") + "\x00" +
		strings.Join(answerBlockClaimUseKeys(block.ClaimUses), "\x1f") + "\x00" +
		exactVisibleAnswerBlockCitationKey(block, citations) + "\x00" +
		surface
}

func exactVisibleAnswerBlockCitationKey(block types.AnswerBlock, citations []types.Citation) string {
	if len(block.Items) == 0 {
		return ""
	}
	out := make([]string, 0, len(block.Items))
	for _, item := range block.Items {
		key := answerBlockItemCitationDedupeKey(item.CitationRef, citations)
		if key == "" {
			key = fmt.Sprintf("ref:%d", item.CitationRef)
		}
		out = append(out, key)
	}
	return strings.Join(out, "\x1f")
}

func semanticPrincipalAnswerBlockDedupeKey(block types.AnswerBlock, citations []types.Citation) string {
	if block.SurfaceRole != types.SurfacePrincipal {
		return ""
	}
	switch block.Kind {
	case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
	default:
		return ""
	}
	if len(block.Items) == 0 {
		return ""
	}
	rows := make([]string, 0, len(block.Items))
	for _, item := range block.Items {
		row := semanticPrincipalAnswerItemDedupeKey(item, citations)
		if row == "" {
			return ""
		}
		rows = append(rows, row)
	}
	return string(block.Kind) + "\x00" +
		string(block.SurfaceRole) + "\x00" +
		strings.Join(sortedAnswerBlockStrings(block.FacetIDs), "\x1f") + "\x00" +
		strings.Join(sortedAnswerBlockStrings(answerBlockClaimUseKeys(block.ClaimUses)), "\x1f") + "\x00" +
		strings.Join(answerBlockNormalizedSurfaces(block.Columns), "\x1f") + "\x00" +
		strings.Join(rows, "\x1f")
}

func semanticPrincipalAnswerItemDedupeKey(item types.AnswerBlockItem, citations []types.Citation) string {
	label := normalizeAnswerBlockDedupeSurface(item.Label)
	if label == "" {
		return ""
	}
	citation := answerBlockItemCitationDedupeKey(item.CitationRef, citations)
	if citation == "" {
		return ""
	}
	return string(item.CandidateRole) + "\x1e" +
		label + "\x1e" +
		normalizeAnswerBlockDedupeSurface(item.Text) + "\x1e" +
		strings.Join(answerBlockNormalizedSurfaces(item.Cells), "\x1d") + "\x1e" +
		citation
}

func answerBlockItemCitationDedupeKey(ref int, citations []types.Citation) string {
	if ref < 0 || ref >= len(citations) {
		return ""
	}
	cit := citations[ref]
	file := normalizeAnswerBlockDedupeFile(cit.File)
	if file == "" || cit.Line <= 0 {
		return ""
	}
	end := cit.LineEnd
	if end < cit.Line {
		end = 0
	}
	if end > cit.Line {
		return fmt.Sprintf("%s:%d-%d", file, cit.Line, end)
	}
	return fmt.Sprintf("%s:%d", file, cit.Line)
}

func answerBlockNormalizedSurfaces(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		if s := normalizeAnswerBlockDedupeSurface(raw); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func normalizeAnswerBlockDedupeSurface(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && strings.HasPrefix(raw, "`") && strings.HasSuffix(raw, "`") {
		raw = strings.TrimSpace(strings.Trim(raw, "`"))
	}
	return strings.Join(strings.Fields(raw), " ")
}

func normalizeAnswerBlockDedupeFile(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	for strings.HasPrefix(raw, "./") {
		raw = strings.TrimPrefix(raw, "./")
	}
	return raw
}

func sortedAnswerBlockStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func answerBlockClaimUseKeys(in []types.RenderedClaimUse) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, claim := range in {
		out = append(out, string(claim.ClaimForm)+"\x1e"+claim.FacetID+"\x1e"+claim.EvidenceID)
	}
	return out
}

func stampReadOwnerAnchorsFromTurnA(ctx *types.BusContext, doc *types.AnswerDocumentV2) int {
	if ctx == nil || ctx.Mutable == nil || doc == nil {
		return 0
	}
	if readFinalAnswerSourceSupplementsNotRequired(ctx) {
		doc.ReadOwnerAnchors = nil
		doc.ReadSourceLocalization = nil
		return 0
	}
	turnA := ctx.Mutable.TurnAArtifacts()
	if turnA == nil {
		return 0
	}
	var review *types.SourceLocalizationReview
	if types.SourceLocalizationReviewHasSignal(turnA.SourceLocalization) {
		review = turnA.SourceLocalization
	} else {
		derived := types.SourceLocalizationReviewFromTurnA(turnA.ReadFiles, turnA.EvidenceItems)
		if types.SourceLocalizationReviewHasSignal(&derived) {
			review = &derived
		}
	}
	if review == nil {
		doc.ReadOwnerAnchors = nil
		doc.ReadSourceLocalization = nil
		return 0
	}
	repoRoot := strings.TrimSpace(ctx.RepoRoot)
	if repoRoot == "" {
		repoRoot = strings.TrimSpace(ctx.MainRepoRoot)
	}
	if repoRoot != "" {
		enriched := sourceowner.EnrichSourceLocalizationReview(repoRoot, *review, "read_final_structural_owner")
		review = &enriched
	}
	normalizedReview := types.NormalizeSourceLocalizationReview(*review)
	doc.ReadSourceLocalization = types.CloneSourceLocalizationReviewPtr(&normalizedReview)
	view := types.OwnerAnchorViewFromSourceLocalizationReview(*review, 0)
	items := make([]types.OwnerAnchorViewItem, 0, minInt(len(view.Items), 12))
	for _, item := range view.Items {
		if !readFinalAnswerOwnerAnchorItem(item) {
			continue
		}
		items = append(items, item)
		if len(items) >= 12 {
			break
		}
	}
	doc.ReadOwnerAnchors = types.NormalizeOwnerAnchorView(types.OwnerAnchorView{Items: items}, 12).Items
	return len(doc.ReadOwnerAnchors)
}

func stampReadNavigationCoverageFromTurnA(ctx *types.BusContext, doc *types.AnswerDocumentV2) bool {
	if ctx == nil || ctx.Mutable == nil || doc == nil || ctx.AnalysisIR == nil {
		return false
	}
	if readFinalAnswerSourceSupplementsNotRequired(ctx) {
		doc.ReadNavigationCoverage = nil
		return false
	}
	if types.RuntimeArtifactReadSourceNavigationNotRequiredForBusContext(ctx) {
		doc.ReadNavigationCoverage = nil
		return false
	}
	turnA := ctx.Mutable.TurnAArtifacts()
	if turnA == nil {
		return false
	}
	coverage := types.RepoMapNavigationCoverageFromReadArtifacts(ctx.AnalysisIR, ctx.ExploreLanePlan, turnA)
	coverage = types.NormalizeRepoMapNavigationCoverage(coverage)
	if coverage.State == "" || coverage.State == types.RepoMapNavigationCoverageNotRequired {
		doc.ReadNavigationCoverage = nil
		return false
	}
	doc.ReadNavigationCoverage = &coverage
	return true
}

func readFinalAnswerSourceSupplementsNotRequired(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	return types.RuntimeArtifactReadSourceSupplementsNotRequiredForBusContext(ctx)
}

func stampReadLocalizerFollowup(ctx *types.BusContext, doc *types.AnswerDocumentV2) bool {
	if ctx == nil || doc == nil {
		return false
	}
	followup := types.DeriveReadLocalizerFollowup(doc.ReadSourceLocalization, doc.ReadNavigationCoverage)
	if followup == nil {
		doc.ReadLocalizerFollowup = nil
		return false
	}
	doc.ReadLocalizerFollowup = followup
	return true
}

func stampReadReasoningGraph(ctx *types.BusContext, doc *types.AnswerDocumentV2) bool {
	if ctx == nil || doc == nil {
		return false
	}
	summary := reasoninggraph.AnswerReasoningGraphSummaryFromReadAnswerDocument(ctx, doc)
	if summary == nil {
		doc.ReadReasoningGraph = nil
		return false
	}
	doc.ReadReasoningGraph = summary
	return true
}

func readFinalAnswerOwnerAnchorItem(item types.OwnerAnchorViewItem) bool {
	normalized := types.NormalizeOwnerAnchorView(types.OwnerAnchorView{Items: []types.OwnerAnchorViewItem{item}}, 1)
	if len(normalized.Items) == 0 {
		return false
	}
	item = normalized.Items[0]
	if item.Path == "" || types.SourcePathRoleIsAuxiliary(item.Role) || item.Kind == types.SourceLocalizationAnchorScope {
		return false
	}
	switch item.Strength {
	case types.SourceLocalizationAnchorOwner, types.SourceLocalizationAnchorSupporting:
	default:
		return false
	}
	return item.EvidenceRef != nil ||
		strings.TrimSpace(item.OwnerSymbol) != "" ||
		strings.TrimSpace(item.Subject) != "" ||
		strings.TrimSpace(item.AnchorSymbol) != ""
}

func filterAcceptedAnswerDisplayAttachments(doc *types.AnswerDocumentV2, in []types.AnswerDisplayAttachment) []types.AnswerDisplayAttachment {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.AnswerDisplayAttachment, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, att := range in {
		if !answerDisplayAttachmentSurvivesAcceptedDoc(doc, att) {
			continue
		}
		key := att.Hash
		if key == "" {
			key = answerDisplayAttachmentHash(att.Kind, att.Language, att.Body)
			att.Hash = key
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, att)
	}
	return out
}

// FilterAcceptedAnswerDisplayAttachments exposes the same accepted-document
// attachment boundary used by the emit tools. Once a structured
// AnswerDocumentV2 has been accepted, markdown/text recovered from failed
// drafts is retry telemetry, not an additional answer carrier. Diagram
// attachments survive because they can represent visible content that did not
// fit the structured document.
func FilterAcceptedAnswerDisplayAttachments(doc *types.AnswerDocumentV2, in []types.AnswerDisplayAttachment) []types.AnswerDisplayAttachment {
	return filterAcceptedAnswerDisplayAttachments(doc, in)
}

func answerDisplayAttachmentSurvivesAcceptedDoc(doc *types.AnswerDocumentV2, att types.AnswerDisplayAttachment) bool {
	switch strings.TrimSpace(att.Kind) {
	case types.AnswerDisplayAttachmentDiagram:
		return strings.TrimSpace(att.Body) != ""
	case types.AnswerDisplayAttachmentMarkdown, types.AnswerDisplayAttachmentText:
		return false
	default:
		return strings.TrimSpace(att.Body) != "" && doc == nil
	}
}

// validateMergedV2Doc runs the merged-doc invariants both write
// paths must enforce. Returns nil on success or a structured error
// the caller surfaces via failEmit.
//
// Invariants:
//   - every block has a non-empty id (after trim)
//   - block ids are unique within the doc
//   - diagram payloads appear only on kind=diagram blocks
//   - kind=diagram blocks carry a non-nil Diagram payload
//   - max blocks: documents with > maxBlocksPerDoc are rejected
func validateMergedV2Doc(doc *types.AnswerDocumentV2) error {
	if doc == nil {
		return fmt.Errorf("merged doc is nil")
	}
	if len(doc.Blocks) == 0 {
		// The full-emit path rejects empty blocks up front; this is the
		// shared backstop for the patch path, where remove_block_ids
		// covering every block would otherwise persist an EMPTY document
		// past the non-softenable empty-blocks gate. No system-side
		// mutation empties a doc (materialize*/normalize* writers only
		// append or rewrite in place), so an empty merge is always a
		// model-authored patch error.
		return fmt.Errorf("merged doc has no blocks; a patch may not remove every block — keep or replace at least one block (unchanged_block_ids preserves prior blocks byte-identical)")
	}
	if len(doc.Blocks) > maxBlocksPerDoc {
		return fmt.Errorf("merged doc has %d blocks; maximum is %d",
			len(doc.Blocks), maxBlocksPerDoc)
	}
	seenIDs := make(map[string]bool, len(doc.Blocks))
	for i, b := range doc.Blocks {
		id := strings.TrimSpace(b.ID)
		if id == "" {
			return fmt.Errorf("merged blocks[%d]: id is empty", i)
		}
		if seenIDs[id] {
			// No index in this message: the merged doc's physical
			// layout includes system-inserted blocks (split diagram
			// halves, prev-doc blocks on the patch path), so a
			// position here does not correspond to the model's own
			// emission — the id is the stable handle the model can
			// act on.
			return fmt.Errorf("merged doc: duplicate id %q (each block must have a unique id)",
				id)
		}
		seenIDs[id] = true
		if b.Diagram != nil && b.Kind != types.BlockDiagram {
			return fmt.Errorf("merged blocks[%d]: diagram payload is only valid when kind=diagram; replace the block with kind=diagram or remove the sibling diagram object from kind=%q", i, b.Kind)
		}
		if b.Kind == types.BlockDiagram && b.Diagram == nil {
			return fmt.Errorf("merged blocks[%d]: kind=diagram requires the sibling `diagram` object {kind: <flow|sequence|architecture|call_dag>, language: \"mermaid\", body: <raw mermaid source>}. If you removed it on a patch retry, restore it on `replace_blocks`; do not move the diagram body into the block-level `text` field", i)
		}
	}
	return nil
}

func normalizeMergedDiagramPayloadKinds(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return 0
	}
	fixed := 0
	for i := range doc.Blocks {
		if doc.Blocks[i].Diagram != nil && doc.Blocks[i].Kind != types.BlockDiagram {
			doc.Blocks[i].Kind = types.BlockDiagram
			fixed++
		}
	}
	return fixed
}

// Runtime trace causal projection block-id family. Every system-emitted block
// of the section derives from the base id; the idempotence guard and the cap
// degrade below key on these exact spellings (F2, adversarial review
// 2026-07-04) — keep construction and guard in lockstep via the constants.
const (
	runtimeTraceCausalProjectionBlockIDBase    = "runtime_trace_causal_projection"
	runtimeTraceCausalProjectionCompareBlockID = runtimeTraceCausalProjectionBlockIDBase + "_compare"
	// runtimeTraceCausalProjectionCompareNotesBlockID (PTV8-LAD L6, 2026-07-08)
	// is the overview's 对比注记明细 sibling — emitted ONLY when the layered
	// table notes fold past the visible cap (the full set must stay reachable
	// whole). Registered in the family guard + degrade skip below.
	runtimeTraceCausalProjectionCompareNotesBlockID  = runtimeTraceCausalProjectionCompareBlockID + "_notes"
	runtimeTraceCausalProjectionCoverageBlockID      = runtimeTraceCausalProjectionBlockIDBase + "_coverage"
	runtimeTraceCausalProjectionPartitionBlockID     = runtimeTraceCausalProjectionBlockIDBase + "_partition"
	runtimeTraceCausalProjectionArtifactBlockIDInfix = "_a"
)

// answerDocumentHasRuntimeTraceCausalProjectionBlock reports whether ANY block
// of the projection family is already present (F2b idempotence). The former
// guard checked only the base / _a1 / _coverage ids, so a document holding
// e.g. only the _compare table (cap-degrade leftovers) or an _a2 section
// escaped the guard and the section was emitted twice.
func answerDocumentHasRuntimeTraceCausalProjectionBlock(doc *types.AnswerDocumentV2) bool {
	for _, block := range doc.Blocks {
		if RuntimeTraceSystemBlock(block) &&
			runtimeTraceCausalProjectionFamilyBlockID(strings.TrimSpace(block.ID)) {
			return true
		}
	}
	return false
}

// runtimeTraceCausalProjectionFamilyBlockID is the precise prefix+suffix
// pattern of the system-emitted projection block-id family: the base id with
// an exact known suffix, or the per-artifact form base+"_a<digits>" with an
// exact known sub-suffix. Arbitrary "runtime_trace_causal_projection*" ids
// (e.g. model-authored lookalikes with a non-numeric artifact tag) do NOT
// match — the guard reads the exact spellings this file constructs, nothing
// looser.
func runtimeTraceCausalProjectionFamilyBlockID(id string) bool {
	rest, ok := strings.CutPrefix(id, runtimeTraceCausalProjectionBlockIDBase)
	if !ok {
		return false
	}
	switch rest {
	// DFULL (SEM-LEAD 收编微批, 2026-07-10): "_detail_full" joined both
	// switches — the builder has emitted idPrefix+"_detail_full" (因果投影
	// 明细无损块) since the PTV8 detail redesign while this guard never
	// listed it, so (F2b) a document holding only the _detail_full residue
	// escaped the idempotence gate and the section could emit twice, and
	// (PSG §25(b)) the system-authored block was excluded from the evidence
	// feed face — its engine numerals could not ground prose (false-positive
	// repair rounds) while its text was scanned as model prose.
	case "", "_detail", "_detail_full", "_evidence", "_compare", "_compare_notes", "_coverage", "_partition":
		return true
	}
	digits, ok := strings.CutPrefix(rest, runtimeTraceCausalProjectionArtifactBlockIDInfix)
	if !ok {
		return false
	}
	i := 0
	for i < len(digits) && digits[i] >= '0' && digits[i] <= '9' {
		i++
	}
	if i == 0 {
		return false
	}
	switch digits[i:] {
	case "", "_detail", "_detail_full", "_evidence":
		return true
	}
	return false
}

// RuntimeTraceSystemBlockID reports whether id is one of the EXACT block-id
// spellings this file's system-injected runtime-trace surfaces construct:
// the causal-projection family (runtimeTraceCausalProjectionFamilyBlockID),
// the facts / semantic-optimizations blocks, and the metric-snapshot /
// next-step / perf-quality blocks with their numbered forms. Same discipline
// as the projection-family guard: arbitrary "runtime_trace_*" lookalikes
// (model-authored ids that merely share the prefix) do NOT match.
//
// This helper classifies the RESERVED ID namespace only. It deliberately does
// not establish system provenance: a model can emit any JSON block ID. Hard
// consumers must call RuntimeTraceSystemBlock, which additionally requires the
// unforgeable in-memory SystemGeneratedKind marker.
func RuntimeTraceSystemBlockID(id string) bool {
	if runtimeTraceCausalProjectionFamilyBlockID(id) {
		return true
	}
	switch id {
	case "runtime_trace_facts",
		"runtime_trace_semantic_optimizations",
		"runtime_trace_metric_snapshot",
		"runtime_trace_perf_quality":
		return true
	}
	for _, base := range []string{
		"runtime_trace_metric_snapshot_",
		"runtime_trace_next_step_",
		"runtime_trace_perf_quality_",
	} {
		if digits, ok := strings.CutPrefix(id, base); ok {
			if digits == "" {
				return false
			}
			for i := 0; i < len(digits); i++ {
				if digits[i] < '0' || digits[i] > '9' {
					return false
				}
			}
			return true
		}
	}
	return false
}

// RuntimeTraceSystemBlock reports whether block is an authenticated
// deterministic runtime-trace evidence surface. Exact spelling prevents a
// loose-prefix lookalike; the json:"-" marker prevents a model from minting an
// exact reserved spelling and thereby suppressing a real system block or
// laundering prose scalars into the evidence lane.
func RuntimeTraceSystemBlock(block types.AnswerBlock) bool {
	return block.SystemGeneratedKind.IsRuntimeTraceSupplement() &&
		RuntimeTraceSystemBlockID(strings.TrimSpace(block.ID))
}

func markRuntimeTraceSystemBlock(block *types.AnswerBlock) {
	if block == nil {
		return
	}
	block.SystemGeneratedKind = types.AnswerSystemGeneratedRuntimeTrace
}

func markRuntimeTraceSystemBlocks(blocks []types.AnswerBlock) {
	for i := range blocks {
		markRuntimeTraceSystemBlock(&blocks[i])
	}
}

// normalizeRuntimeTraceReservedBlockIDCollisions preserves model-authored
// content while taking exact reserved IDs out of the system namespace. The
// rename is deterministic, stable in input order, and collision-free. It runs
// after a full/patch mutation has merged and before every runtime-trace
// materializer, so both write paths share one choke point.
func normalizeRuntimeTraceReservedBlockIDCollisions(doc *types.AnswerDocumentV2) int {
	if doc == nil || len(doc.Blocks) == 0 {
		return 0
	}
	used := make(map[string]bool, len(doc.Blocks))
	for _, block := range doc.Blocks {
		if id := strings.TrimSpace(block.ID); id != "" {
			used[id] = true
		}
	}
	renamed := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		id := strings.TrimSpace(block.ID)
		if id == "" || !RuntimeTraceSystemBlockID(id) || RuntimeTraceSystemBlock(*block) {
			continue
		}
		base := "model_" + id
		candidate := base
		for suffix := 2; used[candidate]; suffix++ {
			candidate = fmt.Sprintf("%s_%d", base, suffix)
		}
		block.ID = candidate
		used[candidate] = true
		renamed++
	}
	return renamed
}

// runtimeTraceCausalProjectionDegradeLeadBlock picks the cap-degrade survivor
// (F2a): the FIRST block that is not the comparison overview. The compare
// table's cells summarize the per-artifact sections ("详情见各工件分段"), so
// degrading TO it would publish a table whose referenced sections were just
// dropped — the first projection section lead is the safest minimum surface.
func runtimeTraceCausalProjectionDegradeLeadBlock(cluster []types.AnswerBlock) *types.AnswerBlock {
	for i := range cluster {
		switch cluster[i].ID {
		case runtimeTraceCausalProjectionCompareBlockID, runtimeTraceCausalProjectionCompareNotesBlockID:
			// PTV8-LAD L6: the notes-detail sibling annotates the overview —
			// degrading TO it would publish notes for a table that was just
			// dropped (same reasoning as skipping the overview itself).
			continue
		}
		return &cluster[i]
	}
	return nil
}

// runtimeTraceCausalProjectionClusterWithinBudget keeps the most useful
// self-contained subset of a projection cluster. Artifact/solo causal leads
// are mandatory. A comparison overview is retained only when every artifact
// lead fits (its cells point to those sections). The original order is
// preserved, and the already tiered cluster order makes compact metrics beat
// full attributes/evidence for any remaining slots.
func runtimeTraceCausalProjectionClusterWithinBudget(cluster []types.AnswerBlock, budget int) []types.AnswerBlock {
	if budget <= 0 || len(cluster) == 0 {
		return nil
	}
	if len(cluster) <= budget {
		return cluster
	}
	selected := make(map[int]bool, budget)
	leadIndexes := make([]int, 0, 2)
	compareIndex := -1
	for i, block := range cluster {
		id := strings.TrimSpace(block.ID)
		switch {
		case id == runtimeTraceCausalProjectionCompareBlockID:
			compareIndex = i
		case runtimeTraceCausalProjectionStandaloneLeadBlockID(id):
			leadIndexes = append(leadIndexes, i)
		}
	}
	if len(leadIndexes) == 0 {
		if lead := runtimeTraceCausalProjectionDegradeLeadBlock(cluster); lead != nil {
			return []types.AnswerBlock{*lead}
		}
		return nil
	}
	for _, idx := range leadIndexes {
		if len(selected) >= budget {
			break
		}
		selected[idx] = true
	}
	// The overview is useful only when all referenced artifact sections fit.
	if len(selected) == len(leadIndexes) && compareIndex >= 0 && len(selected) < budget {
		selected[compareIndex] = true
	}
	for i := range cluster {
		if len(selected) >= budget {
			break
		}
		if selected[i] || i == compareIndex {
			continue
		}
		selected[i] = true
	}
	out := make([]types.AnswerBlock, 0, len(selected))
	for i, block := range cluster {
		if selected[i] {
			out = append(out, block)
		}
	}
	return out
}

func runtimeTraceCausalProjectionStandaloneLeadBlockID(id string) bool {
	if id == runtimeTraceCausalProjectionBlockIDBase || id == runtimeTraceCausalProjectionCoverageBlockID {
		return true
	}
	rest, ok := strings.CutPrefix(id, runtimeTraceCausalProjectionBlockIDBase+runtimeTraceCausalProjectionArtifactBlockIDInfix)
	if !ok || rest == "" {
		return false
	}
	for i := 0; i < len(rest); i++ {
		if rest[i] < '0' || rest[i] > '9' {
			return false
		}
	}
	return true
}

func runtimeTraceCriticalFollowupHeadroom(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil {
		return 0
	}
	headroom := 0
	if !answerDocumentHasRuntimeTraceSystemBlockID(doc, "runtime_trace_semantic_optimizations") &&
		runtimeTraceSemanticOptimizationAvailable(ctx) {
		headroom++
	}
	if !answerDocumentHasRuntimeTraceSystemBlockID(doc, "runtime_trace_metric_snapshot") &&
		len(runtimeTraceMetricSnapshotItems(doc, ctx)) > 0 {
		headroom++
	}
	if !answerDocumentHasNextStepsBlock(doc) && len(runtimeTraceNextStepItems(doc, ctx)) > 0 {
		headroom++
	}
	return headroom
}

func runtimeTraceSemanticOptimizationAvailable(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	set := types.CompileTraceCausalProjectionSet(ledger)
	for _, projection := range set.Projections {
		if len(projection.SemanticSpans) > 0 {
			return true
		}
	}
	return false
}

func answerDocumentHasRuntimeTraceSystemBlockID(doc *types.AnswerDocumentV2, id string) bool {
	if doc == nil {
		return false
	}
	id = strings.TrimSpace(id)
	for _, block := range doc.Blocks {
		if strings.TrimSpace(block.ID) == id && RuntimeTraceSystemBlock(block) {
			return true
		}
	}
	return false
}

func materializeRuntimeTraceCausalProjectionBlock(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil || ctx.Mutable == nil {
		return false
	}
	if len(doc.Blocks) >= maxBlocksPerDoc {
		logging.Warning("[answer_document] runtime trace causal projection block skipped: document already at the %d-block cap", maxBlocksPerDoc)
		return false
	}
	if answerDocumentHasRuntimeTraceCausalProjectionBlock(doc) {
		return false
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	ledger := types.CompileObservationLedger(input)
	set := types.CompileTraceCausalProjectionSet(ledger)
	lang := requestedAnswerDocumentLanguage(ctx)
	focus := runtimeTraceProjUserFocusFromBusContext(ctx)
	var cluster []types.AnswerBlock
	if len(set.Projections) > 1 {
		// CMP-1: multi-artifact ledger — one projection section per trace
		// artifact (per-artifact tree/table/evidence/scale) plus, for typed
		// comparison shapes, a compact cross-artifact overview table.
		cluster = runtimeTraceCausalProjectionMultiCluster(set, ledger, ctx, lang, focus)
	} else {
		var projection types.TraceCausalProjection
		if len(set.Projections) == 1 {
			projection = set.Projections[0]
		}
		cluster = runtimeTraceCausalProjectionCluster(projection, lang, focus)
		// F2c: a multi-identity ledger can still land here when only ONE
		// partition compiled Active — the unattributed/omitted caveat must
		// render exactly as on the multi lane instead of silently dropping.
		// Pure single-artifact ledgers carry zero counters → nil block →
		// byte-identity preserved.
		if len(cluster) > 0 {
			if block := runtimeTraceProjPartitionCaveatBlock(set, runtimeTraceCausalProjectionUseChinese(lang)); block != nil {
				cluster = append(cluster, *block)
			}
		}
	}
	if len(cluster) == 0 {
		if block := runtimeTraceCausalProjectionCoverageBlock(input, lang); block != nil {
			markRuntimeTraceSystemBlock(block)
			insertAt := answerDocumentInsertionIndexBeforeCaveat(doc)
			doc.Blocks = append(doc.Blocks, types.AnswerBlock{})
			copy(doc.Blocks[insertAt+1:], doc.Blocks[insertAt:])
			doc.Blocks[insertAt] = *block
			return true
		}
		return false
	}
	markRuntimeTraceSystemBlocks(cluster)
	// Reserve the ACTUAL follow-up decision surfaces before consuming the
	// remaining block budget. This closes the exact-fit shape where a complete
	// projection cluster filled slot 64 and the unconditional semantic/action/
	// metric blocks were then silently skipped. Causal leads win first; compact
	// metrics win next; lossless details/evidence are the first rows trimmed.
	available := maxBlocksPerDoc - len(doc.Blocks)
	if available <= 0 {
		logging.Warning("[answer_document] runtime trace causal projection block skipped: document already at the %d-block cap", maxBlocksPerDoc)
		return false
	}
	reserve := runtimeTraceCriticalFollowupHeadroom(doc, ctx)
	if reserve < 0 {
		reserve = 0
	}
	if reserve > available-1 {
		reserve = available - 1 // never sacrifice the last causal lead slot
	}
	clusterBudget := available - reserve
	if clusterBudget <= 0 {
		logging.Warning("[answer_document] runtime trace causal projection block skipped: document already at the %d-block cap", maxBlocksPerDoc)
		return false
	}
	if len(cluster) > clusterBudget {
		cluster = runtimeTraceCausalProjectionClusterWithinBudget(cluster, clusterBudget)
		if len(cluster) == 0 {
			logging.Warning("[answer_document] runtime trace causal projection block skipped: no self-contained lead fits the remaining block budget")
			return false
		}
	}
	insertAt := answerDocumentInsertionIndexBeforeCaveat(doc)
	tail := append([]types.AnswerBlock(nil), doc.Blocks[insertAt:]...)
	doc.Blocks = append(doc.Blocks[:insertAt], cluster...)
	doc.Blocks = append(doc.Blocks, tail...)
	return true
}

// runtimeTraceCausalProjectionCluster builds the presentation v3 block cluster
// (docs/design/trace_projection_presentation_v3_20260702.md): a lead block
// whose Text carries the fact-only conclusion + window anchor + tree reading
// note + the MAIN monospace ```text tree (target-anchored, four edge kinds,
// window-scaled bars, ⚠/⊘ inline), then the T10 key-metric table + per-node
// lossless blocks, then a
// file-grouped evidence index. Zero mermaid — the fence is byte-identical
// across HTML / markdown / terminal. Every projection node appears exactly
// twice (one tree row + one table row).
func runtimeTraceCausalProjectionCluster(projection types.TraceCausalProjection, lang string, focus runtimeTraceProjUserFocus) []types.AnswerBlock {
	return runtimeTraceCausalProjectionClusterFor(projection, lang, focus,
		runtimeTraceCausalProjectionBlockIDBase, "")
}

// runtimeTraceCausalProjectionClusterFor is the id/label-parametrized section
// builder behind runtimeTraceCausalProjectionCluster (CMP-1 multi-artifact
// rendering). The single-projection caller passes the legacy id prefix and an
// empty artifactLabel, so its output stays byte-identical; the multi-artifact
// path emits one section per trace artifact ("Trace 因果投影 — <basename>")
// with per-artifact block ids, tree, detail table, evidence index (fresh E#
// numbering) and bar scale.
func runtimeTraceCausalProjectionClusterFor(projection types.TraceCausalProjection, lang string, focus runtimeTraceProjUserFocus, idPrefix, artifactLabel string) []types.AnswerBlock {
	if !projection.Active() {
		return nil
	}
	zh := runtimeTraceCausalProjectionUseChinese(lang)
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	// CMP-7a: the flat-fallback shape (no ≥2-node wakeup path — the same
	// condition that leaves model.Target empty) must not label audit summaries
	// or 因果位置 cells "on-chain" under a header that says the chain could not
	// be traced. Computed here so evidence entries added during the model build
	// already carry it.
	evidence.flatChain = len(runtimeTraceCausalProjectionCleanPath(projection.WakeupPath)) < 2
	model := buildRuntimeTraceProjTreeModel(projection, evidence, zh)
	// PTV8-LAD L7 (§24.14 补2): the single-artifact lane is the caller's own
	// id-prefix fact — the D-4 tree-head deviation line renders only here (the
	// comparison overview carries its own folded per-side note; 批名即界).
	model.SoloArtifact = idPrefix == runtimeTraceCausalProjectionBlockIDBase
	runtimeTraceProjApplyUserFocus(&model, focus)
	fence := runtimeTraceProjTreeFence(model, zh)
	if fence == "" {
		return nil
	}
	// ELIM-1 ◎ 窗内可消除量总览 (RANK-U Stage 2, 2026-07-13): its own typed
	// fence in the lead block (rollback = drop this append). Render CALL
	// order stays tree→overview→lead so the ◎ legend mark reaches the
	// dynamic legend; only the CONCATENATION order below places it first.
	//
	// EVOLUTION RECORD (user ruling 2026-07-13, RANK-U Stage 2 mid-batch):
	// the overview renders BEFORE the projection tree (先执摘后细节 — the
	// executive summary leads, details follow); the GREENLIT draft's
	// 「树 fence 后/明细表前」 placement is superseded. E#/seat pointers keep
	// their semantics as FORWARD references into the tree/board below — the
	// evidence ordinals are allocated at model build, never at fence render,
	// so the assembly order is position-independent; the preview anchor
	// transformer pairs the overview with its FOLLOWING tree fence.
	elimFence := runtimeTraceProjElimOverviewFence(projection, model, zh)
	titleSuffix := ""
	if label := strings.TrimSpace(artifactLabel); label != "" {
		titleSuffix = " — " + label
	}
	claimUses := []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}}
	facets := []string{"observed_artifact_fact"}
	leadText := runtimeTraceProjLeadText(projection, model, lang, zh)
	if elimFence != "" {
		leadText += "\n\n" + elimFence
	}
	leadText += "\n\n" + fence
	out := []types.AnswerBlock{{
		ID:          idPrefix,
		Kind:        types.BlockSection,
		Title:       runtimeTraceCausalProjectionTitle(lang) + titleSuffix,
		Text:        leadText,
		SurfaceRole: types.SurfacePrincipal,
		ClaimUses:   claimUses,
		FacetIDs:    facets,
	}}
	if columns, rows := runtimeTraceProjDetailTable(model, zh); len(rows) > 0 {
		title := "因果投影关键指标"
		if !zh {
			title = "Causal Projection Key Metrics"
		}
		// Customer 2026-07-03: the legend was one run-on paragraph and
		// unreadable — itemized, one short definition per line, kept plain.
		// PTV4 T10: the table is the (a) key-metric surface (≤6 columns);
		// the (b) vertical blocks below carry every qualitative attribute.
		// PTV5 C28/C29 (#68): the 窗口投影/链上累计 definitions land on the
		// duration layer instead of defining 投影 by 投影 — direction of the
		// cumulative verified against the producer semantics (a node's
		// cumulative contains its drill-down sub-chain toward the target;
		// see runtimeTraceProjDepth1Cumulative's containment doc). PTV5 C41:
		// 全 roster → 全部成员清单 (no half-English).
		// PTV8-RCR-B (UXA 域B #2-#10 + verify 修正稿, 2026-07-08). EVOLUTION
		// RECORD: 「用户窗口/聚合行为/循环定义的有效归因/底层状态/口径=列/
		// 投影窗口/无损块」全部按域B改造表落地;⊘/⚠ 与树图例同词并互指;
		// 有效归因定义 = D#29 修正稿 (不引已退役的 gated 词,不写
		// 「可能小于窗口投影」——cmp_01 E29 反例).
		// C2 值词库教学批 (§29.104.16.1 M5②, 裁定③ §29.104.17, 2026-07-17).
		// EVOLUTION RECORD:
		//  ① 链上累计 gains the no-sub-chain arm — a row WITHOUT a drill-down
		//     sub-chain carries no sub-chain share, so its cumulative IS the
		//     row's own in-window account (display compile: an absent
		//     cumulative note falls back to the row's own projection;
		//     engine rank lanes mint cumulative from the row's own account) —
		//     and the anti-「直达」 clause: 「链上」 names the accumulation
		//     direction, never an extra direct-conduction claim (witness
		//     cust_span_vs_prio: the model coined 「直达」 from four names
		//     sharing one value).
		//  ② 有效归因 gains the eff↔cum relation clause. Verified multi-lane
		//     reality — NO single ≤ inequality is honest: eff==cum is the
		//     non-semantic default (one measurement, two names; the tree face
		//     folds the redundant 链上累计 copy, tree.go PTV6-D(c)); 折算/
		//     链上计入 lanes sit below cum; 发生段账目 (§29.112 件D) and
		//     承自等待区间 lanes sit above it. The clause therefore teaches
		//     same-value = one measurement and defers direction to the row's
		//     caliber word, with both directions named as examples only.
		//  ③ the wire-mapping line (C2③) bridges the four duration columns to
		//     the trace_query row fields the model greps in the blob
		//     (impact= / cumulative_impact= / effective_impact= /
		//     actual_impact=; JSON impact_ms / cumulative_impact_ms /
		//     effective_impact_ms / actual_impact_ms). projected_impact_ms is
		//     deliberately NOT equated with 窗口投影 (semantic family rows
		//     publish a narrower intersection there).
		//  ④ the 置信 line lands 裁定③ verbatim scope: the tier word is each
		//     evidence lane's numeric confidence folded through fixed
		//     thresholds, never a cross-row evidence-strength comparison.
		// DISPLAY-HYG 二轮 catalog B4 (§29.104.18.1, 2026-07-17): the ⊘/⚠
		// glossary lines were the table legend's only UNCONDITIONAL glyph
		// entries — a report whose body carries zero ⊘/⚠ still defined both
		// (dead-entry witness 20260717-101345:381). They now gate on the SAME
		// emission-site marks the tree legend consumes (NEW-7 discipline; the
		// fence renders above, so the marks are final here): ⊘ on the
		// undrillable mark; ⚠ on the actual-scope annotation family (⚠实际/
		// ⚠跨窗/超出发生段/区间未发布 — the line teaches the whole family,
		// including the two non-⚠ contrast words).
		undrillableLegend := model.Marks.has(runtimeTraceProjMarkUndrillable)
		actualScopeLegend := model.Marks.has(runtimeTraceProjMarkCrossWindow) ||
			model.Marks.has(runtimeTraceProjMarkCrossWindowNoActual) ||
			model.Marks.has(runtimeTraceProjMarkActualBeyondEpisode) ||
			model.Marks.has(runtimeTraceProjMarkActualNoInterval)
		lines := []string{
			tracefence.AuxColumnGlossaryMarker,
			"- 窗口投影 = 该节点的状态落在分析窗内的时长;跨线程聚合行按跨线程累计计量(非墙钟,单元格已标注)。",
			"- 链上累计 = 该节点及其下钻子链沿唤醒链累计到关注线程的投影时长;无下钻子链的行不含子链份额,该值即该行自身账目的窗内投影;「链上」指沿唤醒链累计的方向,不是「直达关注线程」的额外传导声明。",
			"- 有效归因 = 该行计入根因排序的影响时长;与窗口投影不同时,行内口径词(全额/折算/单次最大等)说明取值方式;与链上累计同值时为同一测量的两个名目(非两项独立证据),异值时取值方式见行内口径词/标注(如 折算/链上计入 取小,发生段账目/承自等待区间 可高于链上累计)。",
			"- 实际状态 = 该状态的真实完整时长,可跨出分析窗(此时带 ⚠);合并行该列为合并种子单次成员的实际值(标注 单次成员),非族合计。",
			"- 与 trace_query 行字段对照:窗口投影 对应 impact=(JSON impact_ms);链上累计 对应 cumulative_impact=(cumulative_impact_ms);有效归因 对应 effective_impact=(effective_impact_ms);实际状态 对应 actual_impact=(actual_impact_ms);一行内多字段同值 = 同一测量的多个名目,不构成相互印证。",
			"- 「—」 = 该列对此节点无值。",
		}
		if undrillableLegend {
			lines = append(lines, "- ⊘ = 窗口内无匹配唤醒事件(sched_wakeup),链止于此(同树内 ⊘链止)。")
		}
		if actualScopeLegend {
			lines = append(lines, "- ⚠ = 实际状态区间确证跨出分析窗(同树内 ⚠实际Xms);仅超出该行自身发生段而未跨分析窗时标注(超出发生段,窗内),区间未随数据发布时标注(区间未发布),均不作跨窗声明。")
		}
		lines = append(lines,
			"- 背景行仅作环境压力证据,不计入链上归因。",
			"- 置信 = 置信档(高/中/低):各证据来源的数值置信按固定阈值折词,不同来源基准不同,不作跨行证据强度比较。",
			"- 本表只列时长与置信;每个节点的类型、因果位置、关系、影响形态、合并成员清单与完整名称,见下方「因果投影明细」。",
		)
		if !zh {
			lines = []string{
				"Column calibers:",
				"- window projection = the duration of the node's state inside the analysis window; cross-thread aggregate rows measure a cross-thread cumulative (not wall clock; cells carry the annotation).",
				"- chain total = the projected duration this node plus its drill-down sub-chain accumulate toward the focused thread along the wakeup chain; a row WITHOUT a drill-down sub-chain carries no sub-chain share — its chain total is the row's own in-window account, and \"chain\" names the accumulation direction, not an extra direct-conduction claim toward the focused thread.",
				"- attribution = the impact duration this row contributes to the root-cause ranking; when it differs from the window projection, the row's caliber word (in full / discounted / single max …) says how it was taken; when it equals the chain total the two are one measurement under two names (never two independent proofs), and when they differ the row's caliber word/annotation governs (e.g. discounted / on-chain-counted sit below, occurrence-segment account / inherited-from-wait-interval may sit above the chain total).",
				"- actual state = the state's true full duration; it may extend beyond the analysis window (then marked ⚠); on a merged row this column is the merge seed's single-member actual (marked single member), never the family total.",
				"- field mapping to trace_query rows: window projection ↔ impact= (JSON impact_ms); chain total ↔ cumulative_impact= (cumulative_impact_ms); attribution ↔ effective_impact= (effective_impact_ms); actual state ↔ actual_impact= (actual_impact_ms); several fields of one row sharing one value = one measurement under several names, never mutual corroboration.",
				"- “—” = no value in this column for this node.",
			}
			if undrillableLegend {
				lines = append(lines, "- ⊘ = no matching wakeup event (sched_wakeup) in the window; the chain ends there (same as the tree's ⊘chain-ends mark).")
			}
			if actualScopeLegend {
				lines = append(lines, "- ⚠ = the actual interval provably crosses the analysis window (same as the tree's ⚠actual mark); an overshoot beyond the row's own episode that stays inside the window is marked (beyond own episode, inside window), and an unpublished interval is marked (interval unpublished) — neither claims a window crossing.")
			}
			lines = append(lines,
				"- Background rows are context-pressure evidence only, never counted into the chain attribution.",
				"- confidence = the confidence tier (high/mid/low): each evidence source's numeric confidence folded through fixed thresholds; sources use different baselines, so the tier never compares evidence strength across rows.",
				"- This table lists durations and confidence only; each node's type, causal position, relation, impact shape, merged-member roster and full name live in the Causal Projection Detail below.",
			)
		}
		// PTV5 C33/C34 (#68): the ×N-form and dual-seat notations get legend
		// rows exactly when the table shows them (gated flags from the same
		// detail rows the table renders) — every other render stays
		// byte-stable.
		if flags := runtimeTraceProjDetailTableLegendFlagsFor(model, zh); flags.mergedSum || flags.mergedMax || flags.mergedWindowMax || flags.mergedDedup || flags.multiSeat || flags.family || flags.selfSymptom || flags.allZeroFold || flags.stanzaChainTotal || flags.gatedProjection ||
			flags.scoreIOPressure || flags.scoreBlockIO || flags.countEquivalent || flags.countClamp || flags.businessSpanMention {
			// DISP-2 G3 表列口径 (§27.2, 2026-07-09): the ◇/▒ stanza row's
			// gated line — present exactly when a stanza row with a cumulative
			// value is on the table (its 链上累计 cell is "—": the column means
			// on-chain accumulation, which an off-chain seat does not make).
			if flags.stanzaChainTotal {
				if zh {
					lines = append(lines, "- ◇/▒ 区段行不在唤醒链上,「链上累计」列为 —;其累计时长以 累计(跨线程) 口径显示于树区段行(与窗口投影相等时即窗口投影列数值)。")
				} else {
					lines = append(lines, "- ◇/▒ stanza rows are off the wakeup chain, so their chain-total cell is “—”; their cumulative shows on the stanza rows under the cross-thread cum caliber (equal to the window-projection cell when the two match).")
				}
			}
			// GATED-CAL 件1③ (§29.104.16.1 M3-c, 2026-07-16): the inversion-seat
			// window-projection qualifier — present exactly when a cell wears
			// the 构成,见明细 annotation (same typed predicate as the cell; the
			// column's 状态落窗时长 promise does not hold on those cells).
			if flags.gatedProjection {
				if zh {
					lines = append(lines, "- 优先级反转席的「窗口投影」列为该席发布的构成值(runnable(全额)+running(折算),非纯状态落窗时长),单元格已标注 构成,见明细;构成拆解见该行分解行与明细块。")
				} else {
					lines = append(lines, "- an inversion seat's window-projection cell is the seat's PUBLISHED composite value (runnable in full + discounted running — not a pure in-window state duration); the cell carries the composite, see the detail blocks annotation, and the split lives on the row's breakdown line and its detail block.")
				}
			}
			// DISP-2 G19 (§27.5, 2026-07-09): the all-zero fold note's gated
			// line — present exactly when the 窗内无有效时长 token is on the
			// table (the shape never claims a member MAX).
			if flags.allZeroFold {
				if zh {
					lines = append(lines, "- 窗内无有效时长 = 该折叠行全部成员在窗内均无可计量时长,不作取最大声明;成员见下方明细与证据索引。")
				} else {
					lines = append(lines, "- no in-window effective duration = every member of that fold carries no measurable in-window duration, so no member-MAX claim is made; members live in the detail blocks and the evidence index.")
				}
			}
			// GAP-B G6 (§27.3, 2026-07-09): the wait-symptom self row's gated
			// line — present exactly when a target_self_state row is on the
			// table (its 有效归因 cell is "—": the row never enters the
			// root-cause ranking; 自因四态 self rows keep their value).
			if flags.selfSymptom {
				if zh {
					lines = append(lines, "- 关注线程自身的等待症状行不参与根因排序,「有效归因」列为 —;其根因看对端/上游链上行。")
				} else {
					lines = append(lines, "- the focused thread's own wait-symptom rows never enter the root-cause ranking, so their attribution cell is “—”; their cause lives on the counterpart/upstream chain rows.")
				}
			}
			// RCM-2 D3 (§24.7.1②/§24.10): the family-merge token's gated line —
			// present exactly when a ×N合计/×N成员最大 family row is on the table.
			if flags.family {
				if zh {
					lines = append(lines, "- N次合计 = 同一线程同类 N 段合并为一个参赛项,数值为同线程墙钟合计(重叠段取并集;跨线程仍不可加和);N次成员最大 = 重叠无法逐段核销时取成员最大(下界)。成员清单与区分键见下方明细。")
				} else {
					lines = append(lines, "- n=N total = N same-kind segments of ONE thread merged into one contender; the value is the same-thread wall-clock total (overlaps as their interval union; across threads wall clock still never sums). n=N member max = the member MAX lower bound when overlap cannot be deducted. Member rosters and distinguishing keys live in the detail blocks.")
				}
			}
			if flags.mergedSum || flags.mergedMax || flags.mergedWindowMax || flags.mergedDedup {
				var parts []string
				if zh {
					if flags.mergedSum {
						// PTV8-RCR-B (UXA 域B 漏审 S3): the 同一(线程,原因)
						// scope clause matches the tree's sum entry (同词).
						parts = append(parts, "N次(a~b) = 同一(线程,原因)的 N 次实例合并,数值为总和")
					}
					if flags.mergedMax {
						// PTV8-RCR-B (UXA 域B #11 REVISE): canonical 墙钟跨线程不可加和.
						parts = append(parts, "N线程取最大(单项a~b) = 跨线程折叠,数值取成员最大(墙钟跨线程不可加和)")
					}
					if flags.mergedWindowMax {
						// §21 CWD: the cross-window MAX form gets its own gated
						// line — the sum line's 数值为总和 must never gloss it.
						parts = append(parts, "N次跨窗取最大(单项a~b) = 查询窗互相重叠,数值取成员最大(互相重叠的查询窗量值不可求和)")
					}
					if flags.mergedDedup {
						parts = append(parts, "N次同值 = 同一测量重复发布,数值即那一次")
					}
					lines = append(lines, "- "+strings.Join(parts, ";")+"。")
				} else {
					if flags.mergedSum {
						parts = append(parts, "n=N(a~b) = N merged instances, the value is the SUM")
					}
					if flags.mergedMax {
						parts = append(parts, "N-thread max(each a~b) = cross-thread fold, the value is the member MAX (wall clock never sums)")
					}
					if flags.mergedWindowMax {
						parts = append(parts, "n=N cross-window max(each a~b) = overlapping query windows, the value is the member MAX (overlapping-window magnitudes never sum)")
					}
					if flags.mergedDedup {
						parts = append(parts, "n=N same-value = one measurement published N times, the value IS that one")
					}
					lines = append(lines, "- "+strings.Join(parts, "; ")+".")
				}
			}
			if flags.multiSeat {
				if zh {
					lines = append(lines, "- 双席/多席 = 同一节点同时出现在多个区段,表内只列一行、数值不重复计,记号列出所在区段;各区段属性见下方「因果投影明细」。")
				} else {
					lines = append(lines, "- dual-/multi-seat = one node appears in several stanzas; the table lists it once and never double counts, the glyphs name the stanzas — per-stanza attributes live in the Causal Projection Detail below.")
				}
			}
			// SCORE-DERIV (§29.104.22.1 user ruling 2026-07-17, 「阅读参考」
			// formula entries): each renders exactly when its word face is on
			// the render (flags read the value faces' own typed predicates —
			// 承诺面双向); the WEIGHT CONSTANTS are deliberately hidden
			// (加权/固定系数 markers only — values live in the code and
			// docs/design/score_derivation_20260717.md, never on the report
			// face; the entry lines carry ZERO digits by construction and the
			// pins scan for none). The block_io formula names the PUBLISHED
			// rank value's three terms (query.go block_io_by_inode mint) —
			// the ledger's four-term :12041 form is the internal SORT score
			// and never publishes, so echoing its page-cache term here would
			// over-claim (documented in the design doc; 委托默认处置).
			if flags.scoreIOPressure {
				if zh {
					lines = append(lines, "- 综合评分(io_pressure) = 最大单事件块/存储延迟 + iowait 阻塞次数(加权) + D态/iowait 墙钟 + 页缓存事件(加权) + 文件IO事件与字节(加权):跨单位合成分,加权系数为固定常量(报告不列数值);非墙钟,不参与汇排。")
				} else {
					lines = append(lines, "- composite score (io_pressure) = max single-event block/storage latency + iowait blocked count (weighted) + D-state/iowait wall clock + page-cache events (weighted) + file-IO events and bytes (weighted): a cross-unit blend with fixed weight constants (values not listed in the report); not wall clock, not ranked here.")
				}
			}
			if flags.scoreBlockIO {
				if zh {
					lines = append(lines, "- 综合评分(block_io) = 最大块延迟 + 最大存储延迟 + 文件IO字节(加权):跨单位合成分,加权系数为固定常量(报告不列数值);非墙钟,不参与汇排。")
				} else {
					lines = append(lines, "- composite score (block_io) = max block latency + max storage latency + file-IO bytes (weighted): a cross-unit blend with fixed weight constants (values not listed in the report); not wall clock, not ranked here.")
				}
			}
			if flags.countEquivalent {
				// DISPLAY-HYG 第二轮 (§29.123 P3②, 2026-07-17): the count
				// family has TWO producers (page_cache_churn count form +
				// file-IO hotspot advisory form, which adds a weighted
				// byte-volume term) — the entry names the second form so the
				// formula promise stays complete; still zero digits.
				if zh {
					lines = append(lines, "- 计数当量 = 事件数 × 固定当量系数(文件IO热点形另含字节量加权项;系数均不列数值);非墙钟,不参与汇排。")
				} else {
					lines = append(lines, "- count equivalent (计数当量) = event count × a fixed equivalence coefficient (the file-IO hotspot form adds a weighted byte-volume term; values not listed); not wall clock, not ranked here.")
				}
			}
			if flags.countClamp {
				if zh {
					lines = append(lines, "- 超上限截断 = 计数当量按窗长固定比例设上限,超出即按上限发布(原始和随行供对照);非墙钟,不参与汇排。")
				} else {
					lines = append(lines, "- over-limit clamp (超上限截断) = the count equivalent is capped at a fixed fraction of the window length and publishes the cap when exceeded (the raw sum rides along for cross-checking); not wall clock, not ranked here.")
				}
			}
			// AXIOM-V2 护栏③ (排序键定义句, user rulings 2026-07-18 —
			// SCORE-DERIV 先例: 键可见常量隐,零数字): renders exactly when a
			// fix-direction word face or a cross-direction mutual clause is on
			// the render (the same emission-site marks the tree legend
			// consumes; 承诺面双向).
			if flags.fixDirection {
				// The channel word rides the tracefence constant (UXG-1 M1
				// discipline: no new hand mirror of the seat-channel bytes).
				if zh {
					lines = append(lines, "- "+tracefence.SeatChannelChainZH+"键 = 各席折算后可消除的提升空间(即 有效归因):跨修复方向同一口径下可比、不可相加(同段重叠收益不叠加,见行内互指句);修向 = 修复方向归类(registry 属性轴),不改变排序与数值。")
				} else {
					lines = append(lines, "- "+tracefence.SeatChannelChainEN+" key = each seat's post-conversion eliminable headroom (i.e. attribution): comparable across fix directions on one caliber, never additive (same-segment overlap gains do not add — see the in-row mutual clauses); fix-direction = a repair-direction class (registry attribute axis) that changes no ordering and no value.")
				}
			}
			// SPANVIS-1 件4 阅读参考层 (user ruling 2026-07-19; SCORE-DERIV
			// 先例 — 教读法不替判): the ◈ dual-lever reading reference,
			// rendered exactly when the ◈ word face is on the render (same
			// emission-site mark as the legend entry; 承诺面双向). The
			// sentence teaches how to READ the typed trio (单次最大/次数/
			// 合计) — it never judges any row as 过于频繁 (树面零判词,
			// §29.131 既裁).
			if flags.businessSpanMention {
				if zh {
					lines = append(lines, "- ◈ 业务span提示行(阅读参考):次数多而单次小→业务流程/调用次数方向;单次长→单次运行时长方向;三数(单次最大/次数/合计)均为窗内墙钟原始值,仅业务视角提示,不参与根因排序,不参与汇排。")
				} else {
					lines = append(lines, "- ◈ business span lead rows (reading reference): many occurrences with a small single occurrence → look toward the business flow / call count; a long single occurrence → look toward one run's duration; the trio (max single / count / total) are raw in-window wall-clock values — business-view leads only, not in root-cause ranking, not ranked here.")
				}
			}
		}
		// VS-1 (§7.8): the discount caliber is explained ONLY when a periodic
		// row is actually on the table — non-periodic renders stay byte-stable.
		if runtimeTraceProjModelHasPeriodicRow(model) {
			if zh {
				lines = append(lines, "- 周期性信号源行:有效归因 = runnable 全额 + 信号迟到量;期内睡眠为正常节拍,不计入有效归因(窗口投影保留原始值)。")
			} else {
				lines = append(lines, "- periodic signal source rows: attribution = runnable in full + signal lateness; in-period sleep is normal cadence and never counts (the window projection keeps the raw value).")
			}
		}
		text := strings.Join(lines, "\n")
		out = append(out, types.AnswerBlock{
			ID:          idPrefix + "_detail",
			Kind:        types.BlockTable,
			Title:       title + titleSuffix,
			Text:        text,
			Columns:     columns,
			Items:       rows,
			SurfaceRole: types.SurfacePrincipal,
			ClaimUses:   claimUses,
			FacetIDs:    facets,
		})
	}
	// PTV4 T10 (b): the per-node vertical lossless blocks — the hard floor
	// surface: every item the tree demotes or omits is reachable here whole
	// (full names carry NO cell caps).
	if fullText := runtimeTraceProjDetailFullText(model, zh); strings.TrimSpace(fullText) != "" {
		// UXG-1 M1 (2026-07-12): the generated-chapter titles come from the
		// tracefence single source — the preview section transformer
		// (traceAuditHeadingClass) matches the same constants.
		title := tracefence.SectionDetailZH
		// 复核收窄: the blocks cover every DATA-bearing rendered node; folded
		// transit hops carry no data row and live on the 省略行 roster — the
		// intro must not over-promise them. PTV5 C42 (#68): 省略行 roster →
		// 省略行清单 (no half-English). PTV6-C ruling C (#73): the trailing
		// "与原始 trace_query 记录" pointer is retired — the roster is the
		// in-answer surface; the intermediate record file is not a user-facing
		// pointer target.
		// Catalog B13 (DISPLAY-HYG 二轮, §29.104.18.1, 2026-07-17): the block
		// order deliberately mirrors the tree lanes (runtimeTraceProjDetailRows
		// — 三面同序 with the tree and the key-metric table), so [E#] is NOT
		// consecutive here; the intro says so and points E#-lookups at the
		// evidence index, whose entries ARE in E# order by construction
		// (ordinals mint in index insertion order).
		// C8PROSE-1 (§29.164 残余清单收账, 2026-07-20): system-minted prose
		// intro — depth-0 clause marks go full-width per the C8 regime;
		// parenthetical interiors keep half-width; the EN face is native.
		intro := "每个节点一块，给出树和指标表中省略或压缩的全部属性；名称不截断；属性完全相同的同名节点共用一块(标题并列各自编号)。树中折叠的中间线程见树内省略行清单。块序与树区自上而下一致(非按 [E#] 连续编号)；按 [E#] 查找请用下方「" + tracefence.SectionEvidenceZH + "」区(按编号排列)。"
		if !zh {
			title = tracefence.SectionDetailEN
			intro = "One block per node, carrying every attribute the tree or the key-metric table demotes or compresses; names are never truncated; identical same-name nodes share one block (evidence numbers side by side in the heading). Folded transit hops live on the tree's omitted-row roster. Blocks follow the tree's top-down order (not consecutive [E#] numbering); to look up a specific [E#], use the " + tracefence.SectionEvidenceEN + " section below (listed in number order)."
		}
		out = append(out, types.AnswerBlock{
			ID:          idPrefix + "_detail_full",
			Kind:        types.BlockSection,
			Title:       title + titleSuffix,
			Text:        intro + "\n\n" + fullText,
			SurfaceRole: types.SurfacePrincipal,
			ClaimUses:   claimUses,
			FacetIDs:    facets,
		})
	}
	if intro, items := runtimeTraceProjEvidenceBlockParts(evidence, zh); len(items) > 0 {
		title := tracefence.SectionEvidenceZH
		if !zh {
			title = tracefence.SectionEvidenceEN
		}
		// NEW-9 (adversarial re-review 2026-07-04): typed truncation disclosure.
		// When any trace_query record of THIS artifact carried the producer's
		// capacity_truncated note (per-view row budget cut the result tail), the
		// index header says so instead of presenting the roster as exhaustive.
		if projection.CapacityTruncated {
			// PTV6-C ruling C (#73): the truncation disclosure states the fact
			// without deflecting to the intermediate record file — the cut
			// tails were never collected, so no coordinate exists to give.
			// C8PROSE-1 (§29.164 残余清单收账, 2026-07-20): the appended
			// disclosure sentence follows the C8 prose regime (depth-0 comma
			// full-width; the half-width `:` stays per the DISPHYG-3 precedent).
			if zh {
				intro += " 部分查询结果超过单次返回上限:各自排序靠前的部分完整保留，超限的尾部行不在本索引内。"
			} else {
				intro += " Some query results exceeded the per-call return limit: the top of each result's own ordering is fully kept; the over-limit tail rows are not in this index."
			}
		}
		out = append(out, types.AnswerBlock{
			ID:        idPrefix + "_evidence",
			Kind:      types.BlockBulletList,
			Title:     title + titleSuffix,
			Text:      intro,
			Items:     items,
			ClaimUses: claimUses,
			FacetIDs:  facets,
		})
	}
	return out
}

// runtimeTraceCausalProjectionMultiCluster renders the CMP-1 multi-artifact
// layout: [compare overview (deterministic ≥2-active-projection gate, NEW-2)]
// + one section cluster per artifact projection + [partition caveat]. Every
// per-artifact section reuses the SINGLE-artifact section builder verbatim
// (no parallel subsystem) with per-artifact ids, titles, E# numbering and bar
// scale.
func runtimeTraceCausalProjectionMultiCluster(set types.TraceCausalProjectionSet, ledger types.ObservationLedger, ctx *types.BusContext, lang string, focus runtimeTraceProjUserFocus) []types.AnswerBlock {
	zh := runtimeTraceCausalProjectionUseChinese(lang)
	var leads, details []types.AnswerBlock
	if runtimeTraceProjComparisonShape(len(set.Projections)) {
		// PTV8-LAD L6: the plural form carries the 对比注记明细 sibling when
		// the layered table notes folded past the visible cap. The compact
		// overview is a decision surface; its lossless note sibling belongs to
		// the later detail tier, after every artifact's root-cause lead.
		for _, block := range runtimeTraceProjCompareOverviewBlocks(set.Projections, ledger, lang, zh, focus) {
			if block.ID == runtimeTraceCausalProjectionCompareBlockID {
				leads = append(leads, block)
			} else {
				details = append(details, block)
			}
		}
	}
	// DISP-3 (§29.10-3 用户裁定, real_trace_campaign_20260705.md, 2026-07-09):
	// when several "Trace 因果投影" sections coexist, every projection TREE
	// section (lead block = 头/覆盖句/树读法/图例/fence + its 关键指标 table)
	// renders first, in artifact order; the 因果明细 blocks (逐节点完整属性 +
	// their per-artifact evidence indexes) follow, in the same artifact order —
	// the former per-artifact interleave buried tree 2 under detail 1. Pure
	// block REORDER on the builder's own typed block ids (block CONTENT stays
	// byte-identical; E# cross-references are per-artifact and
	// position-independent by construction).
	//
	// EVOLUTION RECORD (审计 #63/#6 回裁, §29.25 处置委托 + §29.26 待主会话落账,
	// 2026-07-10). §29.10-3 用户裁定原文: "投影树(含头/覆盖句/关键指标)依次全部
	// 优先显示,因果明细依次殿后" — the 关键指标 table is INSIDE each projection's
	// priority unit; §29.18 ② 验收句: "总览→各投影 lead+关键指标依次→各明细+证据
	// 索引依次→partition caveat 殿后". A remote batch (e920a5d8) split the
	// sectionPrefix+"_detail" key-metric tables into a separate middle tier
	// (a1,a2,a1_detail,a2_detail) and flipped the DISP-3 pin without citing a
	// re-adjudication of §29.10-3 — reverted here to the adjudicated paired
	// grouping (a1,a1_detail,a2,a2_detail). The system supplements' stable
	// insertion boundary is the first LOSSLESS detail block
	// (answerDocumentInsertionIndexBeforeRuntimeTraceDetails — "_detail" is
	// deliberately NOT a boundary id), so decision surfaces never wedge between
	// a tree and its own key-metric table.
	for i, projection := range set.Projections {
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = strings.TrimSpace(projection.ArtifactPath)
		}
		sectionPrefix := fmt.Sprintf("%s%s%d", runtimeTraceCausalProjectionBlockIDBase,
			runtimeTraceCausalProjectionArtifactBlockIDInfix, i+1)
		section := runtimeTraceCausalProjectionClusterFor(projection, lang, focus, sectionPrefix, label)
		for _, block := range section {
			switch block.ID {
			case sectionPrefix, sectionPrefix + "_detail":
				leads = append(leads, block)
			default:
				details = append(details, block)
			}
		}
	}
	out := append(leads, details...)
	if len(out) == 0 {
		return nil
	}
	if block := runtimeTraceProjPartitionCaveatBlock(set, zh); block != nil {
		out = append(out, *block)
	}
	return out
}

// runtimeTraceProjComparisonShape is the comparison-form gate for the
// cross-artifact overview table and the comparison next-step rows (CMP-1
// 对比总览门, as re-adjudicated by NEW-2 — §7.6 对比场景客户回访 2026-07-04):
// ≥2 compiled ACTIVE per-artifact projections, nothing else. The former
// second conjunct (the analyzer's historical_regression / is_cross_component
// boolean) was an LLM-emitted classification with run-to-run variance — the
// SAME two-trace question rendered the overview on one run and silently
// dropped the whole table + supply column + comparison next-steps on the
// rerun (§7.6: complex/5-entity vs moderate/3-entity classification of one
// question). 精确信号红线: this hard-ish display fork now reads ONLY the
// deterministic partition count. ≥2 compiled projections is the system's own
// fact that two artifacts were really analysed; the overview is a pure
// per-artifact fact roll-up, harmless even when the question was not phrased
// as a comparison. The analyzer predicate survives ONLY where no comparison
// data exists and user intent is the missing half of the gate — the
// single-sided sampling hint (runtimeTraceNextStepUnsampledComparisonHint,
// predicate ∧ preflight census ≥2) and the prompt-tier skill directive.
func runtimeTraceProjComparisonShape(projectionCount int) bool {
	return projectionCount >= 2
}

// runtimeTraceProjCompareNoteClass is the PTV8-LAD L6 (§24.19 留账 / §24.8
// 重要度分层总则) importance layer of one comparison-overview ⚠ note. Order IS
// importance (lower = more important): a note that says two cells cannot be
// read against each other outranks a note naming a value's window base, which
// outranks a contextual disclosure. Assigned at each note's build site
// (precise emission identity, never text sniffing).
type runtimeTraceProjCompareNoteClass int

const (
	runtimeTraceProjCompareNoteCaliberConflict runtimeTraceProjCompareNoteClass = iota // 口径矛盾类: cells not directly comparable
	runtimeTraceProjCompareNoteWindowBase                                              // 窗基类: value normalized/sourced off the analysis window
	runtimeTraceProjCompareNoteDisclosure                                              // 披露类: contextual construction facts
)

// runtimeTraceProjCompareNote is one classed table note.
type runtimeTraceProjCompareNote struct {
	Class runtimeTraceProjCompareNoteClass
	Text  string
}

// runtimeTraceProjCompareNoteVisibleCap is the L6 fold threshold (§24.19 留账
// 建议值 4): up to this many notes render under the table intro; beyond it the
// lowest-importance tail folds into one pointer line and the FULL layered set
// rides the 对比注记明细 block (重要信息永不省略 — 低重要度先折叠).
const runtimeTraceProjCompareNoteVisibleCap = 4

// runtimeTraceProjCompareLayerNotes applies the L6 layering: stable-sort by
// importance class (same class stays adjacent, in-class build order kept),
// then fold past the visible cap. Returns the table-face lines and, when the
// fold fired, the full layered line set for the detail block (nil otherwise).
func runtimeTraceProjCompareLayerNotes(notes []runtimeTraceProjCompareNote, zh bool) ([]string, []string) {
	sorted := append([]runtimeTraceProjCompareNote(nil), notes...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Class < sorted[j].Class })
	lines := make([]string, 0, len(sorted))
	for _, note := range sorted {
		lines = append(lines, note.Text)
	}
	if len(lines) <= runtimeTraceProjCompareNoteVisibleCap {
		return lines, nil
	}
	visible := append([]string(nil), lines[:runtimeTraceProjCompareNoteVisibleCap]...)
	folded := len(lines) - runtimeTraceProjCompareNoteVisibleCap
	if zh {
		visible = append(visible, fmt.Sprintf("⚠ 其余 %d 条注记见下方「对比注记明细」", folded))
	} else {
		visible = append(visible, fmt.Sprintf("⚠ %d more note(s) live in the Comparison Note Detail below", folded))
	}
	return visible, lines
}

// runtimeTraceProjCompareNotesDetailBlock is the L6 lossless carrier: the FULL
// layered note set (including the entries already shown above the table),
// rendered as its own section right after the overview so the folded tail is
// reachable whole (全量入明细面). Emitted ONLY when the fold fired.
func runtimeTraceProjCompareNotesDetailBlock(all []string, zh bool) *types.AnswerBlock {
	if len(all) == 0 {
		return nil
	}
	title := "对比注记明细"
	// C8PROSE-1 (§29.164 残余清单收账「等」面, 2026-07-20): the section intro
	// is a system-minted prose sentence — depth-0 comma goes full-width; the
	// joined note lines below keep their own institutional bytes untouched.
	intro := "对比总览的全部注记(含总览下已显示的条目)，按重要度分层排序:口径矛盾 > 窗基 > 披露。"
	if !zh {
		title = "Comparison Note Detail"
		intro = "Every comparison-overview note (including the ones already shown under the table), layered by importance: caliber conflicts > window bases > disclosures."
	}
	return &types.AnswerBlock{
		ID:          runtimeTraceCausalProjectionCompareNotesBlockID,
		Kind:        types.BlockSection,
		Title:       title,
		Text:        intro + "\n\n" + strings.Join(all, "\n"),
		SurfaceRole: types.SurfacePrincipal,
		ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}},
		FacetIDs:    []string{"observed_artifact_fact"},
	}
}

// runtimeTraceProjCompareOverviewBlock is the single-block view of the
// overview builder (existing consumers/tests); the notes-detail sibling, when
// the L6 fold fires, is emitted by the plural form below.
func runtimeTraceProjCompareOverviewBlock(projections []types.TraceCausalProjection, ledger types.ObservationLedger, lang string, zh bool, focus runtimeTraceProjUserFocus) *types.AnswerBlock {
	blocks := runtimeTraceProjCompareOverviewBlocks(projections, ledger, lang, zh, focus)
	if len(blocks) == 0 {
		return nil
	}
	return &blocks[0]
}

// runtimeTraceProjCompareOverviewBlocks assembles the compact cross-artifact
// comparison table from typed fields only (CMP-1 §7.2 对比总览层): per artifact
// the V1-lane primary root cause, the target symptom duration, the on-chain
// attributed amount, the dominant cross-thread background pressure (F3/§7.3
// 裁定2: normalized density leads the cell, the raw cross-thread sum sits in
// the parenthetical note — never a naked cpu·ms figure), the optional
// compute-supply column (F5 downstream: only when EVERY artifact published
// the typed compute_supply_balance observation) and the projection window;
// unequal normalization windows (>10%) force a closing note row. No prose
// reasoning; every cell is assembled from typed fields.
//
// COV-2 (§24.14 B-2/B-5/D-1/D-4, real_trace_campaign_20260705.md, 2026-07-08):
// the four value columns (主根因/症状/链上已归因/供给) reuse the background
// column's 窗基 disclosure lane — whenever a cell's source query window
// provably differs from its side's analysis window the note names the bases
// (the cmp_78_01 primary cells sat on 81ms-vs-1645ms windows, a 20× silent
// base split; the supply share read 22× wrong against the 分析窗 column).
// The focus parameter feeds ONLY the D-4 user-window deviation notes (the
// existing display-only typed window lane); absence changes nothing.
//
// PTV8-LAD L6 (§24.19 留账 → 完整分层, 2026-07-08): the table notes are
// classed at their build sites (口径矛盾 > 窗基 > 披露), layered by importance
// with same-class entries adjacent, and folded past the visible cap — the
// full set then rides the 对比注记明细 sibling block (second return element).
func runtimeTraceProjCompareOverviewBlocks(projections []types.TraceCausalProjection, ledger types.ObservationLedger, lang string, zh bool, focus runtimeTraceProjUserFocus) []types.AnswerBlock {
	if len(projections) < 2 {
		return nil
	}
	supplyCells, supplyWindows := runtimeTraceProjCompareSupplyCells(projections, ledger, zh)
	// DCS E6 F3b (ledger §23.1 ruling ③, 2026-07-08): per-artifact top
	// deterministic optimization span cells (typed SemanticSpans data via the
	// shared LEAD-SEM selector). The column renders only when at least one
	// artifact actually carries a data-bearing semantic span — an all-"—"
	// column would widen every non-compile comparison for nothing.
	optimizationCells := make([]string, len(projections))
	hasOptimization := false
	for i, projection := range projections {
		optimizationCells[i] = runtimeTraceProjCompareOptimizationCell(buildRuntimeTraceProjTreeModel(projection, nil, zh), zh)
		if optimizationCells[i] != "—" {
			hasOptimization = true
		}
	}
	// PTV8-RCR-B (UXA 域D #3, 2026-07-08). EVOLUTION RECORD: 工件→trace 文件
	// (内部词), rank=1→根因排序#1 (根因族), on-chain 已归因→链上已归因
	// (归因族), 投影窗→分析窗 (窗族). EN face keeps its established words.
	// PTV8-RCR-C (§24.14 D-5 退役词, 2026-07-08). EVOLUTION RECORD: the B#3
	// 目标→关注线程 family sweep left this header behind — 目标症状时长 →
	// 关注线程症状时长 (禁词 pin 补录).
	// COV-2 (§24.14 B-2, 2026-07-08). EVOLUTION RECORD: 链上已归因 →
	// 链上已归因(单项最大) — the §24.15 C4 caliber word reaches the column
	// header: the cell value is runtimeTraceProjDepth1Cumulative's largest
	// single depth-1 caliber, never a Σ (墙钟不可加和), and the bare header
	// read as a total.
	columns := []string{"trace 文件", "主根因(" + tracefence.SeatChannelChainZH + "#1)", "关注线程症状时长", "链上已归因(单项最大)", "背景压力"}
	if !zh {
		columns = []string{"Trace file", "Primary root cause (" + tracefence.SeatChannelChainEN + " #1)", "Focused-thread symptom duration", "On-chain attributed (single largest)", "Background pressure"}
	}
	if hasOptimization {
		if zh {
			columns = append(columns, "确定性优化点")
		} else {
			columns = append(columns, "Deterministic optimization")
		}
	}
	if supplyCells != nil {
		if zh {
			columns = append(columns, "算力供给(归一化)")
		} else {
			columns = append(columns, "Compute supply (normalized)")
		}
	}
	if zh {
		columns = append(columns, "分析窗")
	} else {
		columns = append(columns, "Analysis window")
	}
	dash := "—"
	msCell := func(v float64) string {
		if v <= 0 {
			return dash
		}
		return fmt.Sprintf("%.3fms", v)
	}
	rows := make([]types.AnswerBlockItem, 0, len(projections)+1)
	var densityWindows []float64
	var densityBases []string
	densityBaseDiffersFromProjWindow := false
	// §21 CWD item D (§11-N3 修向(a) 披露半, cmp_01 revisit 2026-07-07): the
	// per-side on-chain numerator + the typed chain-not-run flag feed the
	// anchor-quality asymmetry note below. Collected from the SAME models the
	// row cells render from, so the note can never disagree with the cells.
	sideLabels := make([]string, 0, len(projections))
	sideAttributed := make([]float64, 0, len(projections))
	sideChainNotRun := make([]bool, 0, len(projections))
	// COV-2 (§24.14 B-1/B-2/B-5/D-1, 2026-07-08): per-column window-base and
	// symptom-caliber collection — gathered from the SAME models/values the
	// row cells render from, so the notes can never disagree with the cells.
	primaryBases := make([]runtimeTraceProjCompareCellWindowBase, 0, len(projections))
	symptomBases := make([]runtimeTraceProjCompareCellWindowBase, 0, len(projections))
	chainBases := make([]runtimeTraceProjCompareCellWindowBase, 0, len(projections))
	supplyBases := make([]runtimeTraceProjCompareCellWindowBase, 0, len(projections))
	symptomArms := make([]runtimeTraceProjCompareSymptomArm, 0, len(projections))
	for i, projection := range projections {
		model := buildRuntimeTraceProjTreeModel(projection, nil, zh)
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = dash
		}
		attributed := runtimeTraceProjDepth1Cumulative(model)
		sideLabels = append(sideLabels, label)
		sideAttributed = append(sideAttributed, attributed)
		sideChainNotRun = append(sideChainNotRun, model.WakeupChainRecommendedNotRun)
		// COV-2 (§24.14 B-2): the primary cell's source window is the elected
		// lead node's own typed query window (same runtimeTraceProjLeadSelect
		// surface the cell renders from); a multi-window merged lead is a
		// positive cross-window attestation. No lead / no identity → no claim.
		primaryLead, _ := runtimeTraceProjLeadSelect(projection, model)
		primaryBases = append(primaryBases, runtimeTraceProjCompareNodeWindowBase(projection, primaryLead))
		// COV-2 (§24.14 B-2): the on-chain column shares the coverage lane's
		// window consensus (chain data rows + self rows — the exact rows the
		// depth-1 numerator reads); a zero cell claims nothing.
		var chainBase runtimeTraceProjCompareCellWindowBase
		if attributed > 0 {
			cws, cwe, chainOK, chainCross := runtimeTraceProjCoverageWindowConsensus(model)
			chainBase = runtimeTraceProjCompareWindowBaseFrom(projection, cws, cwe, chainOK, chainCross)
		}
		chainBases = append(chainBases, chainBase)
		symptomCell, symptomArm, symptomBase := runtimeTraceProjCompareTargetSymptomCell(projection, model, zh)
		symptomArms = append(symptomArms, symptomArm)
		symptomBases = append(symptomBases, symptomBase)
		if supplyCells != nil {
			supplyBases = append(supplyBases, runtimeTraceProjCompareSupplyWindowBase(projection, supplyWindows[i]))
		}
		window := dash
		if types.TraceCausalProjectionWindowPresent(projection.WindowStartTs, projection.WindowEndTs) {
			window = fmt.Sprintf("%.3f~%.3fs", projection.WindowStartTs, projection.WindowEndTs)
		} else if zh {
			window = "未采集"
		} else {
			window = "not captured"
		}
		pressureCell, densityWindow := runtimeTraceProjCompareBackgroundPressureCell(model, zh)
		if densityWindow > 0 {
			densityWindows = append(densityWindows, densityWindow)
			densityBases = append(densityBases, fmt.Sprintf("%.3fms", densityWindow))
			projWindowMS := (projection.WindowEndTs - projection.WindowStartTs) * 1000
			if projWindowMS <= 0 || math.Abs(densityWindow-projWindowMS) > 1.0 {
				densityBaseDiffersFromProjWindow = true
			}
		}
		cells := []string{
			runtimeTraceCausalProjectionMarkdownSafe(label),
			runtimeTraceCausalProjectionMarkdownSafe(runtimeTraceProjComparePrimaryCell(projection, model, zh)),
			symptomCell,
			msCell(attributed),
			runtimeTraceCausalProjectionMarkdownSafe(pressureCell),
		}
		if hasOptimization {
			cells = append(cells, runtimeTraceCausalProjectionMarkdownSafe(optimizationCells[i]))
		}
		if supplyCells != nil {
			cells = append(cells, runtimeTraceCausalProjectionMarkdownSafe(supplyCells[i]))
		}
		cells = append(cells, window)
		rows = append(rows, types.AnswerBlockItem{
			Cells:       cells,
			CitationRef: -1,
		})
	}
	// F3 forced note (§7.3 裁定2): when the per-artifact normalization windows
	// differ by more than 10%, say so directly under the table — a reader
	// comparing the density figures must know they were normalized over
	// unequal windows. Rendered as the table's last row so it sits after the
	// data on every surface.
	// PTV8-RCR-B (UXA 域D layout-L1, 2026-07-08). EVOLUTION RECORD: the three
	// disclosure notes used to ride the table's first cell with a tail of
	// empty cells ("| ⚠ … |  |  |  |") — they now render as text lines under
	// the table intro (same facts, same order, out of the grid).
	// PTV8-LAD L6: every note carries its importance class from here on.
	var tableNotes []runtimeTraceProjCompareNote
	appendNote := func(class runtimeTraceProjCompareNoteClass, text string) {
		tableNotes = append(tableNotes, runtimeTraceProjCompareNote{Class: class, Text: text})
	}
	if runtimeTraceProjCompareWindowsUnequal(densityWindows) {
		// §21 CWD 复核 F3-note fix: the legacy sentence asserts the projection
		// windows themselves differ — only true when every density base IS its
		// side's projection window (±1ms, F-2 tolerance family). When any side
		// normalized over a different window (cross-window-max member window /
		// row query window) that claim is false for the table the reader sees;
		// name the actual bases instead so the densities stay recomputable.
		note := "⚠ 两侧分析窗长度不等,背景压力已按各自窗长归一化"
		if !zh {
			note = "⚠ Analysis-window lengths differ; background pressure is normalized per analysis window"
		}
		if densityBaseDiffersFromProjWindow {
			bases := strings.Join(densityBases, " / ")
			if zh {
				note = "⚠ 背景压力已按各自数值所在窗归一化(窗基: " + bases + ")"
			} else {
				note = "⚠ Background pressure is normalized over each figure's own window (bases: " + bases + ")"
			}
		}
		appendNote(runtimeTraceProjCompareNoteWindowBase, note)
	}
	// COV-2 (§24.14 B-2/B-5/D-1, real_trace_campaign_20260705.md, 2026-07-08):
	// the four value columns reuse the background column's 窗基 disclosure
	// lane. Precise typed signals only (row query-window endpoints vs the
	// side's analysis-window endpoints, ±1ms F-2 tolerance; the multi-window
	// merged marker); every-side-same-base shapes emit NOTHING.
	// COV-2 复核 F3 (注洪泛最小合并, 2026-07-08): fired lanes whose per-side
	// base tuples are BYTE-identical merge into one line naming both columns
	// (the symptom and on-chain lanes routinely share one consensus tuple);
	// full layered note design stays with the LAD batch.
	windowBaseLanes := []struct {
		zhLabel, enLabel string
		bases            []runtimeTraceProjCompareCellWindowBase
	}{
		{"主根因", "Primary root cause", primaryBases},
		{"关注线程症状时长", "Focused-thread symptom duration", symptomBases},
		{"链上已归因(单项最大)", "On-chain attributed (single largest)", chainBases},
	}
	windowBaseMerged := make([]bool, len(windowBaseLanes))
	for li := range windowBaseLanes {
		if windowBaseMerged[li] || !runtimeTraceProjCompareCellWindowBasesFired(windowBaseLanes[li].bases) {
			continue
		}
		tuple := runtimeTraceProjCompareCellWindowBaseTuple(windowBaseLanes[li].bases, zh)
		label := windowBaseLanes[li].zhLabel
		if !zh {
			label = windowBaseLanes[li].enLabel
		}
		for lj := li + 1; lj < len(windowBaseLanes); lj++ {
			if windowBaseMerged[lj] || !runtimeTraceProjCompareCellWindowBasesFired(windowBaseLanes[lj].bases) {
				continue
			}
			if runtimeTraceProjCompareCellWindowBaseTuple(windowBaseLanes[lj].bases, zh) != tuple {
				continue
			}
			windowBaseMerged[lj] = true
			if zh {
				label += "、" + windowBaseLanes[lj].zhLabel
			} else {
				label += ", " + windowBaseLanes[lj].enLabel
			}
		}
		windowBaseMerged[li] = true
		if note := runtimeTraceProjCompareCellWindowBaseNote(label, windowBaseLanes[li].bases, zh); note != "" {
			appendNote(runtimeTraceProjCompareNoteWindowBase, note)
		}
	}
	if supplyCells != nil {
		// COV-2 (§24.14 B-5/D-1): the supply share is normalized over the
		// observation's OWN query window (~81/93ms) — beside a 1800/1645ms
		// 分析窗 column that base must be named (the 22× misread witness).
		// The 算力供给 literal stays inside this whitelisted function
		// (semantic_wording_lint_test.go delivery-lane rule).
		supplyLabel := "算力供给"
		if !zh {
			supplyLabel = "Compute supply"
		}
		if note := runtimeTraceProjCompareShareWindowBaseNote(supplyLabel, supplyBases, zh); note != "" {
			appendNote(runtimeTraceProjCompareNoteWindowBase, note)
		}
	}
	// COV-2 (§24.14 B-1 修向, 2026-07-08): when the symptom column mixes the
	// two caliber arms (state statistics vs wakeup-chain sampled — the
	// cmp_78_01 3.262-vs-470.071 two-orders-of-magnitude face), say the two
	// sides cannot be read straight across. Same-arm shapes emit NOTHING.
	if note := runtimeTraceProjCompareSymptomCaliberNote(symptomArms, zh); note != "" {
		// L6: 口径矛盾类 — "cannot be read straight across" outranks every base
		// note, so the layering seats it first.
		appendNote(runtimeTraceProjCompareNoteCaliberConflict, note)
	}
	// RTC-2 (real_trace_campaign_20260705.md §4 案 e2, 批 #67): when the
	// artifacts' typed time-base spans are pairwise disjoint (envelope Span ∪
	// anchor window per partition — NOT an anchor consumer, see the F1
	// distinction on types.TraceCausalProjectionTimeBasesDisjoint), close the
	// table with the disclosure note: unrelated clock bases cannot be aligned
	// on one shared timeline, so cross-trace reading must stay relative to
	// each artifact's own window. Pure-arithmetic soft guidance; single
	// partition / any intersection / any span-less projection emits NOTHING.
	if types.TraceCausalProjectionTimeBasesDisjoint(projections) {
		if note := runtimeTraceProjCompareDisjointTimeBaseNote(projections, zh); note != "" {
			appendNote(runtimeTraceProjCompareNoteDisclosure, note)
		}
	}
	// §21 CWD item D (§11-N3 修向(a) 披露半, cmp_01 revisit 2026-07-07): when
	// one side's anchor window carries on-chain attribution and another's
	// carries none, the overview rows compare a drilled window against an
	// undrilled one with no disclosure — the 6.0-vs-7.0 specimen read "有因 vs
	// 无因" off a pure anchor-quality asymmetry. Precise signals only: the
	// per-side depth-1 numerator (>0 existence) and the typed
	// WakeupChainRecommendedNotRun flag; symmetric shapes emit NOTHING.
	if note := runtimeTraceProjCompareChainAsymmetryNote(sideLabels, sideAttributed, sideChainNotRun, zh); note != "" {
		appendNote(runtimeTraceProjCompareNoteDisclosure, note)
	}
	// COV-2 (§24.14 D-4 ±10% 容差裁定, 2026-07-08): per-side analysis-window
	// length vs the user's requested window length — >±10% deviation on ANY
	// side discloses BOTH sides (对读基不同构必须可见); all-within-tolerance
	// or no typed user window emits NOTHING.
	for _, note := range runtimeTraceProjCompareUserWindowDeviationNotes(projections, focus, zh) {
		appendNote(runtimeTraceProjCompareNoteDisclosure, note)
	}
	// PTV5 C15 (#68): no internal jargon on the user panel — "typed" out, and
	// the retired LLM-predicate framing ("对比形态判定") with it (NEW-2 made
	// the gate a deterministic partition count).
	title := tracefence.SectionProjectionZH + "对比总览"
	// C8PROSE-1 (§29.164 残余清单收账, 2026-07-20): prose intro under the C8
	// regime — depth-0 commas full-width, half-width `:` stays.
	text := "跨 trace 对比总览:数值来自各份 trace 独立的投影，跨线程累计值带单位标注，详情见各 trace 分段。"
	if !zh {
		title = tracefence.SectionProjectionEN + " Comparison Overview"
		text = "Cross-trace comparison overview: every value comes from an independent projection of each trace file; cross-thread cumulative values carry their unit annotation. Details live in the per-trace-file sections."
	}
	// PTV8-LAD L6: layer by importance (同类相邻), fold past the visible cap;
	// the folded set rides the 对比注记明细 sibling whole.
	visibleNotes, allNotes := runtimeTraceProjCompareLayerNotes(tableNotes, zh)
	if len(visibleNotes) > 0 {
		text += "\n" + strings.Join(visibleNotes, "\n")
	}
	blocks := []types.AnswerBlock{{
		ID:          runtimeTraceCausalProjectionCompareBlockID,
		Kind:        types.BlockTable,
		Title:       title,
		Text:        text,
		Columns:     columns,
		Items:       rows,
		SurfaceRole: types.SurfacePrincipal,
		ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}},
		FacetIDs:    []string{"observed_artifact_fact"},
	}}
	if detail := runtimeTraceProjCompareNotesDetailBlock(allNotes, zh); detail != nil {
		blocks = append(blocks, *detail)
	}
	return blocks
}

// runtimeTraceProjCompareTargetSymptomCell renders the comparison overview's
// 关注线程症状时长 cell (P0-A2 §18.C, F1 裁定张力 resolution). The primary caliber is
// the F1 state-segment aggregate (runtimeTraceProjTargetSymptomMS) — hop-view
// wall clock is DELIBERATELY excluded there to avoid double counting, and that
// exclusion is untouched here.
//
// F1 裁定张力 resolution (chosen implementation): when the state-view aggregate
// is empty (a hop-only target — both self rows are causal_hop views, e.g. q9's
// single sleep re-described as a wakeup hop) BUT a hop-view sleep magnitude does
// exist, the cell does NOT silently fall to "—" against a tree whose coverage
// line already publishes that sleep. Instead it shows the obtainable hop-view
// sleep magnitude WITH an explicit view-caliber annotation ("唤醒链视图目标睡眠,
// 非状态段聚合") so the reader can never read it as the same F1 state-segment
// caliber the OTHER artifacts' cells carry. This is not "hop-only sleep == the
// symptom denominator" (that would violate F1 double-count protection); it is an
// honest, caliber-labeled fallback that keeps this cell consistent with the tree
// coverage line (both mark the hop-view source). MAX, never Σ — the hop-only
// value comes from runtimeTraceProjHopOnlyTargetSleep which already takes the
// single largest hop-view sleep row.
//
// COV-2 (§24.14 B-1 + §24.12 D-P0, real_trace_campaign_20260705.md,
// 2026-07-08). EVOLUTION RECORD: the state arm's bare "%.3fms" retires — the
// cmp_78_01 flagship face read 3.262ms (a two-row state aggregate whose
// population had collapsed against a 456.725ms excluded hop view) beside
// 470.071ms (annotated hop caliber) with no caliber word: two orders of
// magnitude, one column, one silent existence gate. Both arms now carry
// caliber words, and the state arm's gate is the coverage sentence's OWN
// dominance form (§24.15 C3 census 同款: crossBase=排除>0 form fork;
// 非 crossBase=单项最大>入 cell 合计) plus the §21 CWD window consensus (a
// cross-window numerator never pairs with a single-window caliber claim).
// Precise typed signals only; the returns feed the table-level 窗基 note and
// the B-1 两侧口径不同 note (same values the cell renders — never re-derived).
func runtimeTraceProjCompareTargetSymptomCell(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, zh bool) (string, runtimeTraceProjCompareSymptomArm, runtimeTraceProjCompareCellWindowBase) {
	if symptom := runtimeTraceProjTargetSymptomMS(model); symptom > 0 {
		ws, we, okWin, crossBase := runtimeTraceProjCoverageWindowConsensus(model)
		base := runtimeTraceProjCompareWindowBaseFrom(projection, ws, we, okWin, crossBase)
		excluded, excludedMax, _ := runtimeTraceProjSymptomDenominatorCensus(projection, model)
		var cell string
		switch {
		case crossBase && excluded > 0:
			if zh {
				cell = fmt.Sprintf("%.3fms(仅计入分析窗内直接等待;另有 %d 条关注线程状态行未计入,单项最大 %.3fms;链上/自身数据横跨多个查询窗)", symptom, excluded, excludedMax)
			} else {
				cell = fmt.Sprintf("%.3fms (direct waits inside the analysis window only; %d more focused-thread state row(s) uncounted, single largest %.3fms; the chain/self data spans multiple query windows)", symptom, excluded, excludedMax)
			}
		case crossBase:
			if zh {
				cell = fmt.Sprintf("%.3fms(状态统计;链上/自身数据横跨多个查询窗)", symptom)
			} else {
				cell = fmt.Sprintf("%.3fms (state statistics; the chain/self data spans multiple query windows)", symptom)
			}
		case excluded > 0 && excludedMax > symptom:
			cell = fmt.Sprintf("%.3fms(仅计入分析窗内直接等待;另有 %d 条关注线程状态行未计入,单项最大 %.3fms)", symptom, excluded, excludedMax)
			if !zh {
				cell = fmt.Sprintf("%.3fms (direct waits inside the analysis window only; %d more focused-thread state row(s) uncounted, single largest %.3fms)", symptom, excluded, excludedMax)
			}
		default:
			cell = fmt.Sprintf("%.3fms(全窗状态统计)", symptom)
			if !zh {
				cell = fmt.Sprintf("%.3fms (whole-window state statistics)", symptom)
			}
		}
		return cell, runtimeTraceProjCompareSymptomArmState, base
	}
	if hopSleep, hopWinStart, hopWinEnd := runtimeTraceProjHopOnlyTargetSleep(model); hopSleep > 0 {
		// PTV8-RCR-C (§24.14 D-5 退役词, 2026-07-08). EVOLUTION RECORD: the
		// PTV8-RCR-B pass rewrote this parenthetical without applying its own
		// B#3 目标→关注线程 family — the survivor 目标睡眠 retires here (禁词
		// pin 补录; EN face keeps its established words).
		// §29.183 G8 复核补点: the hop window feeds from the WindowPresent-gated
		// runtimeTraceProjHopOnlyTargetSleep — a rebased [0,end] hop window
		// keeps its window-base cell like any other.
		base := runtimeTraceProjCompareWindowBaseFrom(projection, hopWinStart, hopWinEnd,
			types.TraceCausalProjectionWindowPresent(hopWinStart, hopWinEnd), false)
		if zh {
			return fmt.Sprintf("%.3fms(唤醒链采样到的关注线程睡眠合计,非全窗状态统计)", hopSleep), runtimeTraceProjCompareSymptomArmHop, base
		}
		return fmt.Sprintf("%.3fms (wakeup-chain-view focused-thread sleep, not a state-segment aggregate)", hopSleep), runtimeTraceProjCompareSymptomArmHop, base
	}
	return "—", runtimeTraceProjCompareSymptomArmNone, runtimeTraceProjCompareCellWindowBase{}
}

// runtimeTraceProjCompareSymptomArm is the typed caliber arm the symptom cell
// actually rendered (COV-2 §24.14 B-1): the B-1 mixed-caliber note fires on
// the precise arm enum, never on cell-text sniffing.
type runtimeTraceProjCompareSymptomArm int

const (
	runtimeTraceProjCompareSymptomArmNone runtimeTraceProjCompareSymptomArm = iota
	runtimeTraceProjCompareSymptomArmState
	runtimeTraceProjCompareSymptomArmHop
)

// runtimeTraceProjCompareSymptomCaliberNote renders the COV-2 (§24.14 B-1 修向)
// mixed-caliber note: the symptom column carries BOTH a state-statistics cell
// and a wakeup-chain-sampled cell — different calibers that must not be read
// straight across (cmp_78_01: 3.262 vs 470.071, two口径 two orders of
// magnitude on the flagship face). Precise arm enums only; all-same-arm (or
// any-dash) shapes where no two rendered arms differ emit "".
func runtimeTraceProjCompareSymptomCaliberNote(arms []runtimeTraceProjCompareSymptomArm, zh bool) string {
	hasState, hasHop := false, false
	for _, arm := range arms {
		switch arm {
		case runtimeTraceProjCompareSymptomArmState:
			hasState = true
		case runtimeTraceProjCompareSymptomArmHop:
			hasHop = true
		}
	}
	if !hasState || !hasHop {
		return ""
	}
	if zh {
		return "⚠ 关注线程症状时长列两侧口径不同(全窗状态统计 / 唤醒链采样合计),不可直接对读"
	}
	return "⚠ The focused-thread-symptom column mixes two calibers (whole-window state statistics / wakeup-chain sampled); the two sides cannot be read straight across"
}

// runtimeTraceProjCompareCellWindowBase is one comparison-overview cell's
// typed window-base verdict (COV-2 §24.14 B-2/B-5/D-1): where the cell's
// value actually came from, relative to that side's analysis window. Absence
// of a window identity never claims anything (known=false, mismatch=false);
// a positive multi-window attestation (cross) is always a mismatch — a
// cross-window magnitude has no single base to reconcile with the 分析窗
// column.
type runtimeTraceProjCompareCellWindowBase struct {
	known    bool
	cross    bool
	baseMS   float64
	mismatch bool
}

// runtimeTraceProjCompareWindowBaseFrom builds a cell window base from typed
// query-window endpoints + the consensus cross flag. Mismatch is the §21-CWD
// precise endpoint comparison (runtimeTraceProjCoverageWindowBaseMismatch, F-2
// ±1ms tolerance; no analysis window → no claim).
func runtimeTraceProjCompareWindowBaseFrom(projection types.TraceCausalProjection, ws, we float64, ok, cross bool) runtimeTraceProjCompareCellWindowBase {
	if cross {
		return runtimeTraceProjCompareCellWindowBase{cross: true, mismatch: true}
	}
	if !ok || !types.TraceCausalProjectionWindowPresent(ws, we) {
		return runtimeTraceProjCompareCellWindowBase{}
	}
	return runtimeTraceProjCompareCellWindowBase{
		known:    true,
		baseMS:   (we - ws) * 1000,
		mismatch: runtimeTraceProjCoverageWindowBaseMismatch(projection, ws, we),
	}
}

// runtimeTraceProjCompareNodeWindowBase reads one node's own typed query
// window as a cell base (the primary column's source: the elected lead row).
// A multi-window merged row positively attests >1 windows while the
// aggregator zeroes its row identity — same typed key as the %-faces
// (runtimeTraceProjMultiWindowMergedRow, zero new signals). nil → no claim.
func runtimeTraceProjCompareNodeWindowBase(projection types.TraceCausalProjection, node *types.TraceCausalProjectionNode) runtimeTraceProjCompareCellWindowBase {
	if node == nil {
		return runtimeTraceProjCompareCellWindowBase{}
	}
	if runtimeTraceProjMultiWindowMergedRow(*node) {
		return runtimeTraceProjCompareCellWindowBase{cross: true, mismatch: true}
	}
	return runtimeTraceProjCompareWindowBaseFrom(projection, node.QueryWindowStartTs, node.QueryWindowEndTs,
		types.TraceCausalProjectionWindowPresent(node.QueryWindowStartTs, node.QueryWindowEndTs), false)
}

// runtimeTraceProjCompareSupplyWindowBase builds the supply column's cell base
// from the observation's own typed window_ms note (a length, not endpoints —
// the same 1.0ms tolerance the background density lane applies to its
// length-vs-length comparison). Missing analysis window → no claim.
func runtimeTraceProjCompareSupplyWindowBase(projection types.TraceCausalProjection, windowMS float64) runtimeTraceProjCompareCellWindowBase {
	if windowMS <= 0 {
		return runtimeTraceProjCompareCellWindowBase{}
	}
	projWindowMS := (projection.WindowEndTs - projection.WindowStartTs) * 1000
	return runtimeTraceProjCompareCellWindowBase{
		known:    true,
		baseMS:   windowMS,
		mismatch: projWindowMS > 0 && math.Abs(windowMS-projWindowMS) > 1.0,
	}
}

// runtimeTraceProjCompareCellWindowBaseString renders one side's base slot for
// the 窗基 note: the named window length when it provably differs, 分析窗 when
// it provably matches, the positive multi-window phrase, or 未标注 when the
// cell's rows carried no window identity (absence never guesses).
func runtimeTraceProjCompareCellWindowBaseString(base runtimeTraceProjCompareCellWindowBase, zh bool) string {
	switch {
	case base.cross:
		if zh {
			return "横跨多个查询窗"
		}
		return "multiple query windows"
	case base.known && base.mismatch:
		return fmt.Sprintf("%.3fms", base.baseMS)
	case base.known:
		if zh {
			return "分析窗"
		}
		return "analysis window"
	default:
		if zh {
			return "未标注"
		}
		return "unlabeled"
	}
}

// runtimeTraceProjCompareCellWindowBasesFired reports whether ≥1 side's cell
// value provably came from a window other than that side's analysis window —
// the single firing predicate of the 窗基 note lanes.
func runtimeTraceProjCompareCellWindowBasesFired(bases []runtimeTraceProjCompareCellWindowBase) bool {
	for _, base := range bases {
		if base.mismatch {
			return true
		}
	}
	return false
}

// runtimeTraceProjCompareCellWindowBaseTuple renders the per-side base slots
// joined in row order — the note body AND the F3 merge key (byte-identical
// tuples fold into one note line).
func runtimeTraceProjCompareCellWindowBaseTuple(bases []runtimeTraceProjCompareCellWindowBase, zh bool) string {
	parts := make([]string, 0, len(bases))
	for _, base := range bases {
		parts = append(parts, runtimeTraceProjCompareCellWindowBaseString(base, zh))
	}
	return strings.Join(parts, " / ")
}

// runtimeTraceProjCompareCellWindowBaseNote renders one value column's 窗基
// note (COV-2 §24.14 B-2/B-5/D-1 — the background column's existing 窗基
// disclosure mechanism generalized per-CLASS): fires exactly when ≥1 side's
// cell value provably came from a window other than that side's analysis
// window; every-side-same-base (or no-identity) shapes emit "".
func runtimeTraceProjCompareCellWindowBaseNote(column string, bases []runtimeTraceProjCompareCellWindowBase, zh bool) string {
	if !runtimeTraceProjCompareCellWindowBasesFired(bases) {
		return ""
	}
	tuple := runtimeTraceProjCompareCellWindowBaseTuple(bases, zh)
	if zh {
		return "⚠ " + column + "数值来自各自所在查询窗(窗基: " + tuple + "),不可按分析窗折算"
	}
	return "⚠ " + column + " values come from each side's own query window (bases: " + tuple + "); do not scale them against the analysis window"
}

// runtimeTraceProjCompareShareWindowBaseNote is the share-caliber variant of
// the 窗基 note for percentage cells (the supply column's 占其查询窗 share):
// same firing rule, normalization wording (mirrors the background density
// note's 已按各自数值所在窗归一化 form).
func runtimeTraceProjCompareShareWindowBaseNote(column string, bases []runtimeTraceProjCompareCellWindowBase, zh bool) string {
	if !runtimeTraceProjCompareCellWindowBasesFired(bases) {
		return ""
	}
	tuple := runtimeTraceProjCompareCellWindowBaseTuple(bases, zh)
	if zh {
		return "⚠ " + column + "占比已按各自查询窗归一化(窗基: " + tuple + "),非分析窗占比"
	}
	return "⚠ " + column + " shares are normalized over each side's own query window (bases: " + tuple + "), not over the analysis window"
}

// runtimeTraceProjUserWindowDeviationPct is the ONE §24.14 D-4 judging helper
// (±10% 容差裁定, pure arithmetic): the signed percentage deviation of an
// analysis-window length from the user's stated window length, plus whether it
// exceeds the ±10% disclosure threshold. Shared by the comparison face's
// folded note (COV-2) and the single-artifact tree-head line (PTV8-LAD L7) so
// the two disclosure lanes can never judge differently.
func runtimeTraceProjUserWindowDeviationPct(winMS, userMS float64) (float64, bool) {
	deviation := (winMS - userMS) / userMS * 100
	return deviation, math.Abs(deviation) > 10
}

// runtimeTraceProjCompareUserWindowDeviationNotes renders the COV-2 (§24.14
// D-4 ±10% 容差裁定, 2026-07-08) per-side analysis-window-vs-requested-length
// disclosure: when ANY side's analysis-window length deviates from the user's
// typed requested-window length by more than ±10%, EVERY windowed side
// discloses its own deviation (对读基不同构必须可见 — the cmp_78_01 6.0 side
// analyzed 1645ms against a requested 884ms, a silent 1.86× stretch under a
// cross-side percentage comparison). Pure arithmetic on the existing typed
// display-only user-window lane (runtimeTraceProjUserWindowFromEntities); no
// derivable user window, no analysis window, or all sides within ±10% → nil
// (absence never guesses; ≤10% 静默).
func runtimeTraceProjCompareUserWindowDeviationNotes(projections []types.TraceCausalProjection, focus runtimeTraceProjUserFocus, zh bool) []string {
	userStart, userEnd, ok := runtimeTraceProjUserWindowFromEntities(focus.Entities)
	if !ok {
		return nil
	}
	userMS := (userEnd - userStart) * 1000
	if userMS <= 0 {
		return nil
	}
	type deviationSide struct {
		label string
		winMS float64
	}
	var sides []deviationSide
	triggered := false
	for _, projection := range projections {
		if !types.TraceCausalProjectionWindowPresent(projection.WindowStartTs, projection.WindowEndTs) {
			continue // no analysis window — this side never judges and never discloses
		}
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = strings.TrimSpace(projection.ArtifactPath)
		}
		if label == "" {
			label = "—"
		}
		winMS := (projection.WindowEndTs - projection.WindowStartTs) * 1000
		sides = append(sides, deviationSide{label: label, winMS: winMS})
		if _, beyond := runtimeTraceProjUserWindowDeviationPct(winMS, userMS); beyond {
			triggered = true
		}
	}
	if !triggered {
		return nil
	}
	// COV-2 复核 F3+F4 (2026-07-08): one ⚠ line, one clause per side (the
	// requested length named once, in the first clause); %.1f so a 10.3%
	// deviation never displays as the silence threshold's own "10%".
	clauses := make([]string, 0, len(sides))
	for i, side := range sides {
		deviation, _ := runtimeTraceProjUserWindowDeviationPct(side.winMS, userMS)
		label := runtimeTraceCausalProjectionMarkdownSafe(side.label)
		first := i == 0
		switch {
		case deviation > 10:
			switch {
			case zh && first:
				clauses = append(clauses, fmt.Sprintf("%s:分析窗 %.3fms,较你指定的 %.3fms 长 %.1f%%", label, side.winMS, userMS, deviation))
			case zh:
				clauses = append(clauses, fmt.Sprintf("%s:分析窗 %.3fms,长 %.1f%%", label, side.winMS, deviation))
			case first:
				clauses = append(clauses, fmt.Sprintf("%s: analysis window %.3fms is %.1f%% longer than your requested %.3fms", label, side.winMS, deviation, userMS))
			default:
				clauses = append(clauses, fmt.Sprintf("%s: analysis window %.3fms is %.1f%% longer", label, side.winMS, deviation))
			}
		case deviation < -10:
			switch {
			case zh && first:
				clauses = append(clauses, fmt.Sprintf("%s:分析窗 %.3fms,较你指定的 %.3fms 短 %.1f%%", label, side.winMS, userMS, -deviation))
			case zh:
				clauses = append(clauses, fmt.Sprintf("%s:分析窗 %.3fms,短 %.1f%%", label, side.winMS, -deviation))
			case first:
				clauses = append(clauses, fmt.Sprintf("%s: analysis window %.3fms is %.1f%% shorter than your requested %.3fms", label, side.winMS, -deviation, userMS))
			default:
				clauses = append(clauses, fmt.Sprintf("%s: analysis window %.3fms is %.1f%% shorter", label, side.winMS, -deviation))
			}
		default:
			switch {
			case zh && first:
				clauses = append(clauses, fmt.Sprintf("%s:分析窗 %.3fms,与你指定的 %.3fms 偏差 %.1f%%(±10%% 内)", label, side.winMS, userMS, math.Abs(deviation)))
			case zh:
				clauses = append(clauses, fmt.Sprintf("%s:分析窗 %.3fms,偏差 %.1f%%(±10%% 内)", label, side.winMS, math.Abs(deviation)))
			case first:
				clauses = append(clauses, fmt.Sprintf("%s: analysis window %.3fms is within %.1f%% of your requested %.3fms", label, side.winMS, math.Abs(deviation), userMS))
			default:
				clauses = append(clauses, fmt.Sprintf("%s: analysis window %.3fms is within %.1f%%", label, side.winMS, math.Abs(deviation)))
			}
		}
	}
	if zh {
		return []string{"⚠ " + strings.Join(clauses, ";") + ":窗口按数据边界对齐构造"}
	}
	return []string{"⚠ " + strings.Join(clauses, "; ") + ": the window is constructed by aligning to data boundaries"}
}

// runtimeTraceProjComparePrimaryCell mirrors the conclusion line's selection
// (engine typed rank first, single-instance attribution fallback, merged SUM
// never participates, RN-3(a) on-chain fallback with its short note) into one
// compact table cell — the same runtimeTraceProjLeadSelect surface, so the
// cell can never name a different node than the artifact's conclusion line.
func runtimeTraceProjComparePrimaryCell(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, zh bool) string {
	primary, lane := runtimeTraceProjLeadSelect(projection, model)
	if primary != nil && lane == runtimeTraceProjLeadLaneSemanticFallback {
		// §21 LEAD-SEM (cmp_01 A①): the semantic tier-4 lead flows through the
		// SAME single-source wording as the conclusion line — an optimization-
		// span statement, never a 主根因 claim (负向 pin: no "主根因:" prefix).
		return runtimeTraceProjSemanticLeadText(*primary, model, zh)
	}
	fallbackNote := ""
	if lane == runtimeTraceProjLeadLaneOnChainFallback {
		fallbackNote = runtimeTraceProjLeadFallbackNote(model, zh)
	}
	if primary == nil {
		// §21 LEAD-SEM L3 (cmp_01 A②): the 背景压力段 pointer renders only
		// when the background stanza is non-empty (same defensive check as
		// the conclusion line's 未定位 branch).
		// DCS E6 F3b (ledger §23.1 ruling ③): the zero-chain cell additionally
		// discloses the artifact's top deterministic optimization span with a
		// pointer at the 优化点 column — LEAD-SEM 协调: the semantic-fallback
		// lane above already NAMES the span in its own wording and returns
		// before this branch, so the presence note can never double-write.
		presence := runtimeTraceProjCompareOptimizationPresenceNote(model, zh)
		// SYM (§24.13 裁定一): the same single-source symptom disclosure the
		// conclusion line's honest-fallback lanes carry — ranked target-self
		// rows disclose the target's own wait/lock-hold magnitude next to the
		// 未定位 verdict ("" on every legacy shape).
		selfNote := runtimeTraceProjTargetSelfSymptomNote(model, zh)
		if zh {
			if len(model.Background) > 0 {
				return "未定位到链上主根因(见背景压力段)" + presence + selfNote
			}
			return "未定位到链上主根因" + presence + selfNote
		}
		if len(model.Background) > 0 {
			return "no on-chain primary (see background stanza)" + presence + selfNote
		}
		return "no on-chain primary" + presence + selfNote
	}
	name := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(*primary, zh))
	// §29.30 (2026-07-11): a self-cause crown speaks the SAME head morpheme +
	// category word as the conclusion line (runtimeTraceProjSelfCauseCrownState
	// single source — the cell can never name an external thread the
	// conclusion refused to name). The cell keeps its compact magnitude
	// grammar.
	selfState, selfCategory := runtimeTraceProjSelfCauseCrownState(*primary, projection, model, zh)
	if selfState != "" {
		name = runtimeTraceProjSelfCauseCrownName(selfState, zh)
	}
	cause := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(*primary, zh))
	if selfState != "" {
		cause = selfCategory
	}
	if primary.IsAggregateMetric() {
		cause = ""
	}
	cell := name
	if cause != "" {
		if cell != "" {
			cell += " · "
		}
		cell += cause
	}
	if primary.MergedCount > 1 && primary.MergedMaxMS > 0 {
		// V1: the ×N SUM never publishes as the headline hard fact.
		// VS-1 F2 (adversarial review 2026-07-04): the periodic override rides
		// the SAME helper as the conclusion line on BOTH value branches — a
		// periodic fold's single-max is still cadence-dominated raw sleep.
		if ms := runtimeTraceProjPeriodicHeadlineMS(*primary, primary.MergedMaxMS); primary.PeriodicSource {
			return cell + runtimeTraceProjPeriodicCompareCellSuffix(ms, zh) + fallbackNote
		}
		if zh {
			cell += fmt.Sprintf(" 单次最大 %.3fms(共%d次)", primary.MergedMaxMS, primary.MergedCount)
		} else {
			cell += fmt.Sprintf(" single max %.3fms (of %d)", primary.MergedMaxMS, primary.MergedCount)
		}
		return cell + fallbackNote
	}
	ms := primary.CumulativeImpactMS
	if ms <= 0 {
		ms = runtimeTraceProjNodeDisplayImpact(*primary)
	}
	// VS-1 F2: same shared override as the conclusion line (never a second
	// implementation); a periodic primary's cell states the discounted value
	// (0.000 included) with the caliber note.
	ms = runtimeTraceProjPeriodicHeadlineMS(*primary, ms)
	if primary.PeriodicSource {
		return cell + runtimeTraceProjPeriodicCompareCellSuffix(ms, zh) + fallbackNote
	}
	if ms > 0 {
		cell += fmt.Sprintf(" %.3fms", ms)
	}
	return cell + fallbackNote
}

// runtimeTraceProjCompareOptimizationCell (DCS E6 F3b, ledger §23.1 ruling ③)
// renders one artifact's 确定性优化点 column cell — the top semantic span (the
// shared LEAD-SEM selector) with its magnitude and, when the C00/E5 gates
// allow one, its window share. Typed SemanticSpans data only; "—" when the
// artifact carries no data-bearing semantic span.
func runtimeTraceProjCompareOptimizationCell(model runtimeTraceProjTreeModel, zh bool) string {
	node, ms, ok := runtimeTraceProjSemanticTopSpan(model)
	if !ok {
		return "—"
	}
	// RCM-2 D3 (§24.10 witness cell 「类校验 ×14 合计7.124ms(占其查询窗9%)」):
	// a family top span names the class + ×N and qualifies the magnitude with
	// the family caliber stem — the pre-RCM cell showed ONE member's 2.424ms
	// and hid the family's 7.124ms magnitude. Shared wording source with the
	// presence note and the conclusion fallback (零链括注同步); non-family
	// spans stay byte-identical.
	name, valueCell := runtimeTraceProjSemanticCellParts(*node, ms, zh)
	cell := fmt.Sprintf("%s %s", name, valueCell)
	if share := runtimeTraceProjSemanticSpanShareText(*node, ms, model, zh); share != "" {
		if zh {
			cell += "(" + share + ")"
		} else {
			cell += " (" + share + ")"
		}
	}
	return cell
}

// runtimeTraceProjCompareOptimizationPresenceNote is the zero-chain primary
// cell's 括注 (DCS E6 F3b): the artifact located no on-chain primary but DOES
// carry a deterministic optimization span — say so next to the 未定位 verdict
// and point at the 优化点 column. "" when no data-bearing semantic span
// exists (the note never guesses).
func runtimeTraceProjCompareOptimizationPresenceNote(model runtimeTraceProjTreeModel, zh bool) string {
	node, ms, ok := runtimeTraceProjSemanticTopSpan(model)
	if !ok {
		return ""
	}
	// RCM-2 D3 零链括注同步: same family name/value wording source as the
	// 确定性优化点 column cell (one helper, never a drifting copy).
	name, valueCell := runtimeTraceProjSemanticCellParts(*node, ms, zh)
	share := runtimeTraceProjSemanticSpanShareText(*node, ms, model, zh)
	if zh {
		if share != "" {
			return fmt.Sprintf("(存在确定性优化点: %s %s %s,见优化点列)", name, valueCell, share)
		}
		return fmt.Sprintf("(存在确定性优化点: %s %s,见优化点列)", name, valueCell)
	}
	if share != "" {
		return fmt.Sprintf(" (deterministic optimization point present: %s %s, %s; see the optimization column)", name, valueCell, share)
	}
	return fmt.Sprintf(" (deterministic optimization point present: %s %s; see the optimization column)", name, valueCell)
}

// runtimeTraceProjPeriodicCompareCellSuffix renders a periodic primary's
// magnitude in the comparison-overview cell: the discounted attribution from
// runtimeTraceProjPeriodicHeadlineMS plus the short caliber note (F2 pin:
// "0.176ms(周期性,期内睡眠不计)"). Formatting only — the value NEVER comes
// from here.
func runtimeTraceProjPeriodicCompareCellSuffix(ms float64, zh bool) string {
	if zh {
		return fmt.Sprintf(" %.3fms(周期性,期内睡眠不计)", ms)
	}
	return fmt.Sprintf(" %.3fms (periodic; in-period sleep excluded)", ms)
}

// runtimeTraceProjCompareBackgroundPressureCell picks the LARGEST cross-thread
// cumulative aggregate row of the artifact's background stanza and renders it
// per §7.3 裁定2 (F3, adversarial review 2026-07-04): the CMP-9 normalized
// density is the PRIMARY cell content — the only cross-window-comparable
// reading — and the raw cross-thread sum moves into the parenthetical note
// with its CMP-3 unit annotation. Without a precise window the raw value +
// annotation stay (never an estimated density). The second return is the
// normalization window actually used (0 = no density), feeding the F3
// unequal-window note. "—" when the artifact published no such aggregate.
func runtimeTraceProjCompareBackgroundPressureCell(model runtimeTraceProjTreeModel, zh bool) (string, float64) {
	var best *types.TraceCausalProjectionNode
	bestValue := 0.0
	var compositeBest *types.TraceCausalProjectionNode
	compositeBestValue := 0.0
	for i := range model.Background {
		node := &model.Background[i].Node
		if !runtimeTraceProjCrossThreadAggregateType(*node) {
			continue
		}
		v := runtimeTraceProjNodeDisplayImpact(*node)
		// RANKDIS-M18: a composite score and a thread/cpu-ms aggregate have
		// no common ordering ruler. Duration aggregates retain this panel's
		// established selection/density lane; the composite arm is an honest
		// fallback only when that lane has no positive candidate.
		if runtimeTraceProjCompositeValueCaliber(*node) {
			if v > 0 && (compositeBest == nil || v > compositeBestValue) {
				compositeBest, compositeBestValue = node, v
			}
			continue
		}
		if v > 0 && (best == nil || v > bestValue) {
			best, bestValue = node, v
		}
	}
	if best == nil || bestValue <= 0 {
		if compositeBest != nil && compositeBestValue > 0 {
			return runtimeTraceProjCompositeScoreValueText(compositeBestValue, zh), 0
		}
		// COV-2 (§24.14 D-3, real_trace_campaign_20260705.md, 2026-07-08): the
		// closed-set type gate swallowed every non-aggregate background row —
		// the cmp_78_01 6.0 cell rendered "—" while its own tree stanza
		// published two 91.940ms trace_span background rank rows, a
		// face-to-face contradiction. When the closed set misses but the
		// stanza carries data-bearing background rows, show the top row with
		// its caliber word instead of a fabricated "—".
		if cell, ok := runtimeTraceProjCompareBackgroundTopRowCell(model, zh); ok {
			return cell, 0
		}
		return "—", 0
	}
	windowMS := runtimeTraceProjCrossThreadDensityWindowMS(*best, model.WindowMS, model.WindowMS > 0)
	if windowMS <= 0 {
		return fmt.Sprintf("%.3fms", bestValue) +
			runtimeTraceProjCrossThreadAggregateSuffix(*best, model.WindowMS, model.WindowMS > 0, zh), 0
	}
	density := bestValue / windowMS
	queueDepth := runtimeTraceProjCrossThreadQueueDepthToken(*best)
	concurrency := runtimeTraceProjCrossThreadConcurrencyToken(*best)
	// §21 CWD (cmp_01 revisit 2026-07-07): a cross-window MAX row's value is
	// the single largest member, not a Σ — the parenthetical label must not
	// call it 累计/cumulative-of-N. Non-max rows keep the wording verbatim.
	// PTV8-RCR-B (UXA 域D #19, 2026-07-08). EVOLUTION RECORD: the zh cell said
	// 「累计 …ms,跨线程累计」 — one caliber twice in one parenthetical. The
	// value label itself now carries 跨线程累计; the cross-window-MAX arm (a
	// different label) keeps the full note (it had no duplication).
	valueLabel, enValueLabel := "跨线程累计", "cumulative"
	zhSuffix := ",非墙钟"
	if best.MergedCrossWindowMax {
		valueLabel, enValueLabel = "跨窗取最大", "cross-window max"
		zhSuffix = ",跨线程累计,非墙钟"
	}
	var cell string
	switch {
	case queueDepth && zh:
		cell = fmt.Sprintf("≈平均排队深度 %.1f(%s %.3fms%s)", density, valueLabel, bestValue, zhSuffix)
	case queueDepth:
		cell = fmt.Sprintf("≈avg queue depth %.1f (%s %.3fms, cross-thread, not wall clock)", density, enValueLabel, bestValue)
	case concurrency && zh:
		// PTV6-D (d): the irq-family density word — mirrored with the stanza
		// suffix fork so both surfaces speak one semantics.
		cell = fmt.Sprintf("≈窗内并发 %.1f×(%s %.3fms%s)", density, valueLabel, bestValue, zhSuffix)
	case concurrency:
		cell = fmt.Sprintf("≈avg concurrency %.1f× (%s %.3fms, cross-thread, not wall clock)", density, enValueLabel, bestValue)
	case zh:
		cell = fmt.Sprintf("≈均值 %.1f(%s %.3fms%s)", density, valueLabel, bestValue, zhSuffix)
	default:
		cell = fmt.Sprintf("≈mean %.1f (%s %.3fms, cross-thread, not wall clock)", density, enValueLabel, bestValue)
	}
	return cell, windowMS
}

// runtimeTraceProjCompareBackgroundTopRowCell is the COV-2 (§24.14 D-3,
// 2026-07-08) fallback arm of the comparison overview's 背景压力 cell: the
// cross-thread-aggregate closed set found no positive row, but the background
// stanza DOES carry data-bearing rows (e.g. trace_span background rank rows).
// The cell then shows the largest single background row with an explicit
// caliber word instead of contradicting the stanza with "—". Returns ok=false
// when no such row exists (the honest "—" keeps its no-data meaning).
//
// COV-2 复核 F1 (2026-07-08): the value lane mirrors the census (§24.15 C3
// 同款) so the 单项最大 word can never label a Σ — a MergedCount>1 row
// publishes its single-instance MergedMaxMS (a merged row without one has no
// single-item magnitude and never competes); PeriodicSource rows stay out
// (cadence-dominated raw magnitudes). The 非跨线程累计 claim is gated on the
// typed display-impact source: a cumulative/effective-sourced value IS the
// cross-thread cum its own tree stanza row labels 累计(跨线程) — the cell
// speaks the SAME shared word (runtimeTraceProjCrossThreadCumWord, no second
// literal), never the face-to-face contradiction.
func runtimeTraceProjCompareBackgroundTopRowCell(model runtimeTraceProjTreeModel, zh bool) (string, bool) {
	var best *types.TraceCausalProjectionNode
	bestValue := 0.0
	bestSingleInstance := false
	for i := range model.Background {
		node := &model.Background[i].Node
		if runtimeTraceProjCrossThreadAggregateType(*node) {
			continue // the aggregate arm already had its shot (no positive value)
		}
		if node.PeriodicSource {
			continue // 复核 F1: raw cadence-dominated magnitudes never lead this cell
		}
		v, source := runtimeTraceProjNodeDisplayImpactSource(*node)
		singleInstance := source == runtimeTraceProjImpactSourceWindow
		if runtimeTraceProjFamilyRow(*node) {
			// RCM-2 D1/D3 (F6): a family contender competes with its PUBLISHED
			// participation value and wears the family caliber word below —
			// never 单项最大 (it is a same-thread total) and never the
			// cross-thread word (the F6 mislabel this batch retires).
			v = runtimeTraceProjFamilyPublishedMS(*node)
			singleInstance = false
		} else if node.MergedCount > 1 {
			if node.MergedMaxMS <= 0 {
				continue // Σ-only merged row: no single-item magnitude to publish
			}
			v = node.MergedMaxMS // census 同款: single-instance max, never the Σ
			singleInstance = true
		}
		if v > bestValue {
			best, bestValue, bestSingleInstance = node, v, singleInstance
		}
	}
	if best == nil || bestValue <= 0 {
		return "", false
	}
	name := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(*best, zh))
	cause := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(*best, zh))
	cell := name
	if cause != "" {
		if cell != "" {
			cell += " · "
		}
		cell += cause
	}
	if runtimeTraceProjFamilyRow(*best) {
		// RCM-2 D3: the count chip + the family caliber word (shared single
		// source; unknown calibers make no claim — the count is still truth).
		cell += runtimeTraceProjMergeCountChip(best.FamilyMemberCount, zh)
		if cell != "" {
			cell += " "
		}
		word, _, ok := runtimeTraceProjFamilyCaliberWord(*best, zh)
		switch {
		case ok && zh:
			return cell + fmt.Sprintf("%.3fms(背景行最大,%s)", bestValue, word), true
		case ok:
			return cell + fmt.Sprintf("%.3fms (largest background row, %s)", bestValue, word), true
		case zh:
			return cell + fmt.Sprintf("%.3fms(背景行最大)", bestValue), true
		default:
			return cell + fmt.Sprintf("%.3fms (largest background row)", bestValue), true
		}
	}
	if cell != "" {
		cell += " "
	}
	if !bestSingleInstance {
		// cumulative/effective-sourced value: the same word its tree row carries.
		if zh {
			return cell + fmt.Sprintf("%.3fms(背景行单项最大,%s)", bestValue, runtimeTraceProjCrossThreadCumWord(true)), true
		}
		return cell + fmt.Sprintf("%.3fms (largest single background row, %s)", bestValue, runtimeTraceProjCrossThreadCumWord(false)), true
	}
	if zh {
		return cell + fmt.Sprintf("%.3fms(背景行单项最大,非跨线程累计)", bestValue), true
	}
	return cell + fmt.Sprintf("%.3fms (largest single background row, not a cross-thread cumulative)", bestValue), true
}

// runtimeTraceProjCompareWindowsUnequal is the F3 precise inequality check
// over the per-artifact normalization windows actually used by the background
// cells: ≥2 densities rendered AND (max-min)/max > 0.1 — the exact
// |w1-w2|/max > 10% comparison on the two-artifact shape, never a fuzzy
// tolerance.
func runtimeTraceProjCompareWindowsUnequal(windows []float64) bool {
	if len(windows) < 2 {
		return false
	}
	min, max := windows[0], windows[0]
	for _, w := range windows[1:] {
		if w < min {
			min = w
		}
		if w > max {
			max = w
		}
	}
	return max > 0 && (max-min)/max > 0.1
}

// runtimeTraceProjCompareDisjointTimeBaseNote renders the RTC-2 disjoint
// time-base note row for the comparison overview: each artifact's typed
// time-base envelope verbatim, then the user-word guidance (never "envelope"/
// "partition" jargon). Callers gate on
// types.TraceCausalProjectionTimeBasesDisjoint; the defensive "" on a missing
// span can only trip if the two calls ever diverge.
func runtimeTraceProjCompareDisjointTimeBaseNote(projections []types.TraceCausalProjection, zh bool) string {
	spans := make([]string, 0, len(projections))
	for _, projection := range projections {
		start, end, ok := projection.TimeBaseSpan()
		if !ok {
			return ""
		}
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = strings.TrimSpace(projection.ArtifactPath)
		}
		if label == "" {
			label = "—"
		}
		spans = append(spans, fmt.Sprintf("%s %.3f~%.3fs",
			runtimeTraceCausalProjectionMarkdownSafe(label), start, end))
	}
	if zh {
		subject := "两份 trace 时间基准不相交"
		if len(projections) > 2 {
			subject = "各份 trace 时间基准两两不相交"
		}
		return fmt.Sprintf("⚠ %s(%s),不可直接在同一时间轴对齐;对比请以各自窗口内相对指标为准",
			subject, strings.Join(spans, ","))
	}
	subject := "The two trace files' time bases do not overlap"
	if len(projections) > 2 {
		subject = "The trace files' time bases are pairwise disjoint"
	}
	return fmt.Sprintf("⚠ %s (%s); they cannot be aligned directly on one shared timeline — compare relative metrics within each trace file's own window",
		subject, strings.Join(spans, ", "))
}

// runtimeTraceProjCompareChainAsymmetryNote renders the §21-CWD anchor-quality
// asymmetry note row (item D; §11-N3 修向(a) 披露半, cmp_01 revisit
// 2026-07-07): fires exactly when ≥1 side's anchor window carries NO on-chain
// attribution while ≥1 other side's carries some — the cmp_01 shape (6.0 锚窗
// 100ms 无链/平铺树 vs 7.0 锚窗有链 94.466ms) where the overview's 主根因 /
// on-chain 列 read as "有因 vs 无因" off a pure evidence-depth asymmetry.
// Precise typed signals only: the per-side depth-1 numerator (>0 existence
// comparison — the SAME value the on-chain cell renders) and the typed
// WakeupChainRecommendedNotRun flag naming why the chainless side is
// chainless. Symmetric shapes (all >0 or all ==0) return "" and the table
// stays byte-identical. Soft disclosure only — no gate consumes this.
func runtimeTraceProjCompareChainAsymmetryNote(labels []string, attributed []float64, chainNotRun []bool, zh bool) string {
	if len(labels) < 2 || len(labels) != len(attributed) || len(labels) != len(chainNotRun) {
		return ""
	}
	var zeroSides []string
	posLabel, posMax := "", 0.0
	for i, label := range labels {
		if attributed[i] > 0 {
			if attributed[i] > posMax {
				posLabel, posMax = label, attributed[i]
			}
			continue
		}
		side := runtimeTraceCausalProjectionMarkdownSafe(label)
		if chainNotRun[i] {
			if zh {
				side += "(该侧唤醒链下钻未执行)"
			} else {
				side += " (its wakeup-chain drilldown did not run)"
			}
		}
		zeroSides = append(zeroSides, side)
	}
	if len(zeroSides) == 0 || posMax <= 0 {
		return ""
	}
	sep := "、"
	if !zh {
		sep = ", "
	}
	if zh {
		return fmt.Sprintf("⚠ 两侧分析窗证据深度不对称:%s 的分析窗内无链上归因;%s 的分析窗内链上已归因 %.3fms。主根因/链上已归因两列不可直接逐行对比",
			strings.Join(zeroSides, sep), runtimeTraceCausalProjectionMarkdownSafe(posLabel), posMax)
	}
	return fmt.Sprintf("⚠ Anchor-window quality is asymmetric: %s has no on-chain attribution inside its anchor window; %s attributed %.3fms on-chain there. Evidence depth differs — do not read the primary / on-chain columns straight across",
		strings.Join(zeroSides, sep), runtimeTraceCausalProjectionMarkdownSafe(posLabel), posMax)
}

// runtimeTraceProjCompareSupplyCells builds the per-artifact 算力供给 cells of
// the comparison overview (CMP-C F5 downstream, adversarial review
// 2026-07-04). Source: the typed compute_supply_balance ledger observation per
// artifact — ClaimKey "compute_supply_balance", deterministic trace_query
// producer, rich notes supply_ratio= / idle_mismatch_ms= / window_ms= (the
// CMP-8/CMP-10 cross-lane contract). The supply ratio is already normalized
// (delivered/nominal cpu·ms) and renders as a percentage; the idle mismatch
// renders in wall-clock ms plus its share of that artifact's OWN supply
// window — the normalized caliber that stays comparable across unequal
// windows. Returns nil (whole column absent) when ANY projection lacks the
// observation or a parseable required value: a half-filled or estimated
// supply column would fabricate exactly the comparison data this table exists
// to ground. The second return is the per-artifact typed window_ms actually
// used as the share denominator, feeding the COV-2 窗基 note.
//
// COV-2 (§24.14 B-5/D-1, 2026-07-08). EVOLUTION RECORD: 占窗 → 占其查询窗
// (DCS E5 同措辞, tree.go 占其查询窗N% 先例): the share's denominator is the
// supply observation's OWN query window (~81/93ms in cmp_78_01), not the
// row's 分析窗 (1800/1645ms) — the bare 占窗 beside the 分析窗 column read
// 22× wrong.
func runtimeTraceProjCompareSupplyCells(projections []types.TraceCausalProjection, ledger types.ObservationLedger, zh bool) ([]string, []float64) {
	cells := make([]string, 0, len(projections))
	windows := make([]float64, 0, len(projections))
	for _, projection := range projections {
		record, ok := runtimeTraceProjSupplyBalanceRecordForArtifact(ledger, projection)
		if !ok {
			return nil, nil
		}
		ratio, okRatio := runtimeTraceProjSupplyNoteFloat(record, types.TraceNoteKeySupplyRatio)
		idle, okIdle := runtimeTraceProjSupplyNoteFloat(record, types.TraceNoteKeyIdleMismatchMS)
		window, okWindow := runtimeTraceProjSupplyNoteFloat(record, types.TraceNoteKeyWindowMS)
		if !okRatio || !okIdle || !okWindow || window <= 0 || ratio < 0 || idle < 0 {
			return nil, nil
		}
		share := idle / window * 100
		if zh {
			cells = append(cells, fmt.Sprintf("供给率 %.1f%% · 就绪积压时核闲置 %.3fms(占其查询窗 %.1f%%)", ratio*100, idle, share))
		} else {
			cells = append(cells, fmt.Sprintf("supply ratio %.1f%% · idle mismatch %.3fms (%.1f%% of its query window)", ratio*100, idle, share))
		}
		windows = append(windows, window)
	}
	return cells, windows
}

// runtimeTraceProjSupplyBalanceRecordForArtifact finds THIS artifact's typed
// compute_supply_balance observation: exact ClaimKey, deterministic-query
// producer (chokepoint classifier — run-suffixed trace_query ids included),
// and the partitioner's own record→projection artifact attribution (canonical
// path / suffix-alias / id-label matching — never a substring heuristic).
// First ledger match wins (deterministic ledger order).
func runtimeTraceProjSupplyBalanceRecordForArtifact(ledger types.ObservationLedger, projection types.TraceCausalProjection) (types.ObservationRecord, bool) {
	for _, record := range ledger.Records {
		if record.ClaimKey != "compute_supply_balance" {
			continue
		}
		if !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
			continue
		}
		if !types.TraceCausalProjectionRecordMatchesArtifact(record, projection) {
			continue
		}
		return record, true
	}
	return types.ObservationRecord{}, false
}

// runtimeTraceProjSupplyNoteFloat parses one typed key=value rich note off a
// compute_supply_balance record. Missing or unparseable → not-ok; the caller
// drops the whole supply column instead of estimating.
func runtimeTraceProjSupplyNoteFloat(record types.ObservationRecord, key string) (float64, bool) {
	raw := runtimeTraceObservationRichNoteValue(record.RichNotes, key)
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// runtimeTraceProjPartitionCaveatBlock renders the CMP-1 partition caveat: the
// unattributed-observation count ("N 条观测无工件归属,未纳入投影") and the
// artifacts omitted by the partition cap. Nil when there is nothing to say —
// single-artifact ledgers always take that path (byte-identity).
func runtimeTraceProjPartitionCaveatBlock(set types.TraceCausalProjectionSet, zh bool) *types.AnswerBlock {
	var parts []string
	// C8PROSE-1 (§29.164 残余清单收账, 2026-07-20): both caveat sentences are
	// system-minted prose — depth-0 clause marks go full-width (the half-width
	// `:` before the roster stays; the 、-joined label roster is untouched).
	if set.UnattributedObservationCount > 0 {
		if zh {
			parts = append(parts, fmt.Sprintf("%d 条观测无法归属到任一 trace 文件，未纳入投影。", set.UnattributedObservationCount))
		} else {
			parts = append(parts, fmt.Sprintf("%d observation(s) carried no trace-file identity and were left out of every projection.", set.UnattributedObservationCount))
		}
	}
	if len(set.OmittedArtifactLabels) > 0 {
		if zh {
			parts = append(parts, fmt.Sprintf("trace 文件分区数超过上限，仅保留观测最多的 %d 个；未展示: %s。",
				len(set.Projections), strings.Join(set.OmittedArtifactLabels, "、")))
		} else {
			parts = append(parts, fmt.Sprintf("Trace-file partitions exceeded the cap; the %d with the most observations are shown. Omitted: %s.",
				len(set.Projections), strings.Join(set.OmittedArtifactLabels, ", ")))
		}
	}
	if len(parts) == 0 {
		return nil
	}
	title := tracefence.SectionProjectionZH + "分区边界"
	if !zh {
		title = tracefence.SectionProjectionEN + " Partition Boundary"
	}
	return &types.AnswerBlock{
		ID:        runtimeTraceCausalProjectionPartitionBlockID,
		Kind:      types.BlockCaveat,
		Title:     title,
		Text:      strings.Join(parts, " "),
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}},
		FacetIDs:  []string{"observed_artifact_fact", "uncertainty_boundary"},
	}
}

// runtimeTraceProjUserFocusFromBusContext extracts the typed analyzer entity
// context for the R2 root-label comparison: AnalyzerHints.Entities ∪
// ExactTargets verbatim (never RawRequest — the typed lanes are the only
// permitted carriers). Nil context / IR → empty focus → every consumer fails
// open to legacy behavior.
func runtimeTraceProjUserFocusFromBusContext(ctx *types.BusContext) runtimeTraceProjUserFocus {
	var focus runtimeTraceProjUserFocus
	if ctx == nil || ctx.AnalysisIR == nil {
		return focus
	}
	hints := ctx.AnalysisIR.RequestModel.AnalyzerHints
	seen := map[string]bool{}
	for _, entity := range append(append([]string(nil), hints.Entities...), hints.ExactTargets...) {
		entity = strings.TrimSpace(entity)
		if entity == "" || seen[entity] {
			continue
		}
		seen[entity] = true
		focus.Entities = append(focus.Entities, entity)
	}
	return focus
}

func runtimeTraceCausalProjectionCleanPath(path []string) []string {
	out := make([]string, 0, len(path))
	for _, item := range path {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

// PTV8-LAD L1 (§24.11 维度A, 2026-07-08). EVOLUTION RECORD: the former
// runtimeTraceCausalProjectionRepeatingPath detector (index-0-anchored FULL
// path periodicity, path[i]==path[i%period] for every i) is RETIRED here —
// its shape never matched the real pathology (a repeating segment in the
// MIDDLE of a mixed path returned (0,0), so the huadong_78 ladder rendered
// with zero ×N disclosures), and its note could only ride the first fold row.
// The run-length lane in runtimeTraceProjFoldSegments (tree.go) detects
// mid-path cycles directly; a fully periodic path is subsumed (its folded
// middle is itself a run).

func runtimeTraceCausalProjectionCoverageBlock(input types.ObservationLedgerInput, lang string) *types.AnswerBlock {
	zh := runtimeTraceCausalProjectionUseChinese(lang)
	reasons := runtimeTraceCausalProjectionCoverageReasons(input.ToolResults, zh)
	authority := runtimeTraceCoverageAuthority(input.ToolResults)
	if len(reasons) == 0 && !authority.causalUnproven && !authority.enumerationIncomplete && len(authority.lifecycleBoundaries) == 0 {
		return nil
	}
	authorityText := runtimeTraceCoverageAuthorityText(authority, zh)
	text := runtimeTraceCausalProjectionCoverageText(reasons, zh)
	if len(authority.lifecycleBoundaries) > 0 {
		// Lifecycle suppression is the precise engine-side cause of the empty
		// causal/frame face. Seat it before generic exploration/refinement
		// reasons so a same-window retry is not presented as the primary cure.
		text = authorityText + text
	} else {
		text += authorityText
	}
	title := tracefence.SectionProjectionZH + "覆盖边界"
	if !zh {
		title = tracefence.SectionProjectionEN + " Coverage Boundary"
	}
	return &types.AnswerBlock{
		ID:        runtimeTraceCausalProjectionCoverageBlockID,
		Kind:      types.BlockCaveat,
		Title:     title,
		Text:      text,
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}},
		FacetIDs:  []string{"observed_artifact_fact", "uncertainty_boundary"},
	}
}

type runtimeTraceCoverageAuthorityBoundary struct {
	causalUnproven        bool
	frameUnproven         bool
	frameEvidenceStatus   string
	enumerationIncomplete bool
	compactedViews        []string
	enumerationBoundaries []types.ToolEnumerationBoundary
	lifecycleBoundaries   []types.TraceLifecycleBoundaryAuthority
}

func runtimeTraceCoverageAuthority(results []types.ToolResult) runtimeTraceCoverageAuthorityBoundary {
	var out runtimeTraceCoverageAuthorityBoundary
	seenViews := map[string]bool{}
	seenBoundaries := map[types.ToolEnumerationBoundary]bool{}
	for _, result := range results {
		toolName := strings.TrimSpace(result.ToolName)
		if toolName == "trace_query" && result.TraceEvidenceAuthority != nil {
			authority := result.TraceEvidenceAuthority
			if authority.CausalConclusion == "unproven" {
				out.causalUnproven = true
			}
			if authority.FrameEvidenceStatus == "absent" || authority.FrameEvidenceStatus == "unavailable" {
				out.frameUnproven = true
				if out.frameEvidenceStatus == "" || authority.FrameEvidenceStatus == "unavailable" {
					out.frameEvidenceStatus = authority.FrameEvidenceStatus
				}
			}
			out.lifecycleBoundaries = append(out.lifecycleBoundaries, authority.LifecycleBoundaries...)
		}
		enumerationInScope := toolName == "trace_query" ||
			(toolName == "read_file" && result.RuntimeArtifactRead != nil)
		if !enumerationInScope || result.EnumerationAuthority == nil {
			continue
		}
		if result.EnumerationAuthority.Status == "incomplete" {
			out.enumerationIncomplete = true
			for _, boundary := range result.EnumerationAuthority.Boundaries {
				if !seenBoundaries[boundary] {
					seenBoundaries[boundary] = true
					out.enumerationBoundaries = append(out.enumerationBoundaries, boundary)
				}
				view := strings.TrimSpace(boundary.Scope)
				if view == "" || seenViews[view] {
					continue
				}
				seenViews[view] = true
				out.compactedViews = append(out.compactedViews, view)
			}
		}
	}
	sort.Strings(out.compactedViews)
	sort.Slice(out.enumerationBoundaries, func(i, j int) bool {
		a, b := out.enumerationBoundaries[i], out.enumerationBoundaries[j]
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		if a.Dimension != b.Dimension {
			return a.Dimension < b.Dimension
		}
		if a.Emitted != b.Emitted {
			return a.Emitted < b.Emitted
		}
		return a.Total < b.Total
	})
	sort.Slice(out.lifecycleBoundaries, func(i, j int) bool {
		if out.lifecycleBoundaries[i].BoundaryTs != out.lifecycleBoundaries[j].BoundaryTs {
			return out.lifecycleBoundaries[i].BoundaryTs < out.lifecycleBoundaries[j].BoundaryTs
		}
		return out.lifecycleBoundaries[i].ConflictTID < out.lifecycleBoundaries[j].ConflictTID
	})
	return out
}

func runtimeTraceCoverageAuthorityText(authority runtimeTraceCoverageAuthorityBoundary, zh bool) string {
	var parts []string
	if len(authority.lifecycleBoundaries) > 0 {
		boundaries := runtimeTraceLifecycleBoundariesText(authority.lifecycleBoundaries)
		if zh {
			parts = append(parts, "生命周期抑制: suppression_reason=thread_incarnation_conflict，"+boundaries+"；应按给出的 boundary/selector/process-scope 建议恢复证据，不能把同窗重复探索或通用限流当成首要原因")
		} else {
			parts = append(parts, "Lifecycle suppression: suppression_reason=thread_incarnation_conflict, "+boundaries+"; recover evidence with the published boundary/selector/process-scope remedies rather than treating same-window exploration or generic limits as the primary cause")
		}
	}
	if authority.causalUnproven {
		if zh {
			if authority.frameUnproven {
				parts = append(parts, "证据权限: frame_causality=unproven，frame_evidence_status="+firstNonEmpty(authority.frameEvidenceStatus, "absent")+"；未获得可绑定到目标的 frame/deadline 证据或 typed causal row，调度、IO、频率观察只能描述窗口背景，不能证明具体丢帧因果")
			} else {
				parts = append(parts, "证据权限: causal_conclusion=unproven；当前没有 typed causal row，背景观察不能升级为确定根因")
			}
		} else if authority.frameUnproven {
			parts = append(parts, "Evidence authority: frame_causality=unproven, frame_evidence_status="+firstNonEmpty(authority.frameEvidenceStatus, "absent")+"; no target-bound frame/deadline evidence or typed causal row was produced, so scheduler, IO, and frequency observations describe window context but do not prove a specific frame-drop cause")
		} else {
			parts = append(parts, "Evidence authority: causal_conclusion=unproven; without a typed causal row, background observations cannot be promoted to a definite root cause")
		}
	}
	if authority.enumerationIncomplete {
		views := strings.Join(authority.compactedViews, ",")
		if views == "" {
			views = "unknown"
		}
		boundaries := runtimeTraceEnumerationBoundariesText(authority.enumerationBoundaries)
		if zh {
			parts = append(parts, "枚举权限: enumeration_status=incomplete，compacted_views="+views+"，boundaries="+boundaries+"；达到上限或分页只返回的行只能作为样本或下界，不能支撑“全部/仅有/总计/共N/最大/最小”结论")
		} else {
			parts = append(parts, "Enumeration authority: enumeration_status=incomplete, compacted_views="+views+", boundaries="+boundaries+"; capped or paged rows are examples or lower bounds and cannot support all/only/total/exact-count/max/min claims")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, "。") + "。"
}

func runtimeTraceLifecycleBoundariesText(boundaries []types.TraceLifecycleBoundaryAuthority) string {
	if len(boundaries) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(boundaries))
	for _, boundary := range boundaries {
		parts = append(parts, fmt.Sprintf("tid=%d boundary_line=%d boundary_ts=%.6f scope=%s affects_target=%t affected_lanes=%s preserved_lanes=%s frame_ownership_status=%s candidate_selectors=%s suggested_queries=%s",
			boundary.ConflictTID, boundary.BoundaryLine, boundary.BoundaryTs,
			firstNonEmpty(strings.TrimSpace(boundary.Scope), "unknown"),
			boundary.AffectsTarget, strings.Join(boundary.AffectedLanes, ","),
			strings.Join(boundary.PreservedLanes, ","),
			firstNonEmpty(strings.TrimSpace(boundary.FrameOwnershipStatus), "not_applicable"),
			strings.Join(boundary.CandidateSelectors, ","),
			strings.Join(boundary.SuggestedQueries, "|")))
	}
	return strings.Join(parts, ";")
}

func runtimeTraceEnumerationBoundariesText(boundaries []types.ToolEnumerationBoundary) string {
	if len(boundaries) == 0 {
		return "unknown"
	}
	parts := make([]string, 0, len(boundaries))
	for _, boundary := range boundaries {
		scope := strings.TrimSpace(boundary.Scope)
		if scope == "" {
			scope = "unknown"
		}
		total := "unknown"
		if boundary.TotalKnown {
			total = strconv.Itoa(boundary.Total)
		}
		parts = append(parts, fmt.Sprintf("%s/%s:emitted=%d,total=%s",
			scope, firstNonEmpty(strings.TrimSpace(boundary.Dimension), "rows"), boundary.Emitted, total))
	}
	return strings.Join(parts, "|")
}

func runtimeTraceCausalProjectionCoverageReasons(results []types.ToolResult, zh bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, result := range results {
		if strings.TrimSpace(result.ToolName) != "trace_query" {
			continue
		}
		if !result.Success {
			reason := "trace_query_failed"
			if zh {
				reason = "trace_query 执行失败"
			}
			if !seen[reason] {
				seen[reason] = true
				out = append(out, reason)
			}
		}
		if result.Refinement != nil {
			hint := types.NormalizeToolRefinementHint(*result.Refinement)
			if reason := runtimeTraceCausalProjectionCoverageReasonLabel(hint.ReasonCode, zh); reason != "" && !seen[reason] {
				seen[reason] = true
				out = append(out, reason)
			}
		}
		if result.Repair != nil {
			code := strings.TrimSpace(result.Repair.Code)
			if code != "" {
				reason := "repair_code=" + code
				if zh {
					reason = "修复码=" + code
				}
				if !seen[reason] {
					seen[reason] = true
					out = append(out, reason)
				}
			}
		}
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func runtimeTraceCausalProjectionCoverageReasonLabel(code string, zh bool) string {
	switch strings.TrimSpace(code) {
	case "trace_query_heavy_view_requires_scope":
		// PTV5 C37 (#68): zh 面不夹 heavy view/bounded 工程词;view 名与参数名
		// (span/pattern) 保留=操作指引。
		if zh {
			return "大 trace 的重量级视图查询需要限定时间/行/span/pattern 范围"
		}
		return "heavy trace view requires a bounded time/line/span/pattern scope"
	case "trace_query_index_event_limit":
		if zh {
			return "trace 索引事件预算触顶"
		}
		return "trace index event budget was reached"
	case "trace_query_result_compacted":
		if zh {
			return "trace_query 结果已压缩"
		}
		return "trace_query result was compacted"
	case "trace_query_event_search_limit_reached":
		if zh {
			return "event_search 结果达到条数上限"
		}
		return "event_search reached its result limit"
	case "trace_query_recipe_discovery_needs_scope":
		if zh {
			return "recipe discovery 需要更精确范围"
		}
		return "recipe discovery needs a narrower scope"
	case "trace_query_recipe_discovery_marker_window":
		if zh {
			return "recipe discovery 建议使用 marker 附近窗口"
		}
		return "recipe discovery recommends a marker-local window"
	case "trace_query_auto_window_candidate":
		if zh {
			return "trace_query 已给出候选自动窗口"
		}
		return "trace_query produced an auto-window candidate"
	default:
		code = strings.TrimSpace(code)
		if code == "" {
			return ""
		}
		return "reason_code=" + code
	}
}

func runtimeTraceCausalProjectionCoverageText(reasons []string, zh bool) string {
	if zh {
		// PTV8-RCR-B 收尾 (UXA 域D #23/#24 族, 2026-07-08). EVOLUTION RECORD:
		// 本轮→本报告;"应按 trace_query 的有界参数继续补 X" 工具语法 →
		// 客户可执行的追问句式(token 括注保留).
		// C8PROSE-1 (§29.164 残余清单收账, 2026-07-20): coverage prose under
		// the C8 regime — depth-0 clause marks full-width (the 结构化原因
		// roster joint already spoke the full-width ；and stays; parenthetical
		// tool-token interiors keep half-width).
		text := "本报告已获得 trace_query 的结构化执行记录，但没有产出有数据支撑的 root_cause/wakeup_chain/semantic 行，因此未生成分层因果表。"
		if len(reasons) > 0 {
			text += " 结构化原因: " + strings.Join(reasons, "；") + "。"
		}
		text += " 这不是“没有背景影响”的结论；只表示当前证据没有给出可审计的因果/背景统计，可追问一次根因/窗口/交互统计分析(root_cause_rank、window_stats 或 interaction_stats)补齐。"
		return text
	}
	text := "This report has structured trace_query execution records, but no data-backed root_cause/wakeup_chain/semantic rows were produced, so the layered causal table was not generated."
	if len(reasons) > 0 {
		text += " Typed reason: " + strings.Join(reasons, "; ") + "."
	}
	text += " This does not prove there was no background influence; it only means this report lacks auditable causal/background statistics. Ask a follow-up root-cause/window/interaction statistics analysis (root_cause_rank, window_stats or interaction_stats) to fill it in."
	return text
}

type runtimeTraceCausalProjectionEvidenceIndex struct {
	order []runtimeTraceCausalProjectionEvidenceEntry
	seen  map[string]string
	// flatChain mirrors the section's flat-fallback shape (no ≥2-node wakeup
	// path, so the tree renders "按层级平铺"): the audit summary must not claim
	// on-chain causality next to a header that says the chain could not be
	// traced (CMP-7a). Set by the cluster builder before the model build.
	flatChain bool
	// hasMergedEvidence flips when any added node carried MergedEvidenceIDs —
	// gates the PTV5 C35 (#68) E#(+N) intro half-sentence, so rosters without
	// the notation stay byte-identical.
	hasMergedEvidence bool
}

type runtimeTraceCausalProjectionEvidenceEntry struct {
	ID  string
	Ref string
	// Window is the node's own trace window rendered as "[start–end s]" when the
	// source observation exposed one — the preferred display locator (§7.30
	// 裁定6): a time window locates evidence for the reader where an 800k-line
	// range does not. The full path:line locator stays in the raw record.
	Window  string
	Details string
	// SyntheticLine marks an ABSENCE observation (typed missing_wakeup lane):
	// its line span is the sleep interval's bookkeeping, not a trace row that
	// contains the observation — there is no sched_wakeup row to point at. The
	// display locator keeps only the artifact name (CMP-7b, customer compare
	// audit 2026-07-03 §7: "…systrace:44" read as a real row); the raw record
	// keeps the interval lines untouched.
	SyntheticLine bool
	// FamilyAudit (RCM-2 D4, 2026-07-08) marks an entry standing for an engine
	// family merge: its member_count/member_fold_caliber audit tokens are
	// load-bearing (the E# stands for N members), so the display's audit
	// ceiling widens for exactly these entries instead of cutting the family
	// accounting at the 96-rune boundary. Typed flag from the node, never a
	// substring probe on the composed details.
	FamilyAudit bool
	// SameValueAudit (DIAG A1, §28.11-3(a), 2026-07-09) marks an entry whose
	// node carries the µs-tie fold-member disclosure: its
	// same_value_members/same_value_lines tokens are the double-attribution
	// witness the customer verifies line ranges from, so the ceiling widens
	// exactly like FamilyAudit. Typed flag from the node.
	SameValueAudit bool
	// AbsorbedAudit (SELF-ALL rider, 2026-07-13) marks an absorbed chain-lane
	// entry (G1 §27.2-G1): its absorbed_into=<family key> pointer token is
	// load-bearing (信息守恒第三面 — the E# is only self-explaining through the
	// family pointer), and the family key grew a typed proof-basis lane
	// dimension (rootCauseFamilyFoldLaneKey "on_chain|<basis>"), so the 96-rune
	// ceiling would part-boundary-drop the pointer. Widens exactly like
	// FamilyAudit. Typed flag from the node.
	AbsorbedAudit bool
	// SupplementAudit (修复轮 件5, 2026-07-14): the entry's record was
	// minted by the SUPP-CORE system supplement — its
	// origin=system_supplement provenance token is load-bearing on the
	// audit face, so the ceiling widens exactly like FamilyAudit.
	SupplementAudit bool
}

func newRuntimeTraceCausalProjectionEvidenceIndex() *runtimeTraceCausalProjectionEvidenceIndex {
	return &runtimeTraceCausalProjectionEvidenceIndex{seen: map[string]string{}}
}

// has reports whether the node already holds an E# — i.e. the tag was
// allocated by the model walk and therefore EXISTS in the rendered evidence
// index. 修复轮 D1 (冷读 2026-07-12, donghu r2 witness): the optimization
// table re-tags spans through the same walk-rebuilt index; a span the walk
// never rendered would mint a FRESH number past the printed index (「证据
// E39」 with the index ending at E38 — a dangling pointer on a deterministic
// surface). Consumers use this to fall back to an inline locator instead.
func (idx *runtimeTraceCausalProjectionEvidenceIndex) has(node types.TraceCausalProjectionNode) bool {
	if idx == nil {
		return false
	}
	ref := runtimeTraceCausalProjectionEvidenceRef(node)
	if strings.TrimSpace(ref) == "" {
		return false
	}
	key := strings.TrimSpace(ref) + "\x00" + runtimeTraceCausalProjectionNodeKey(node)
	return idx.seen[key] != ""
}

func (idx *runtimeTraceCausalProjectionEvidenceIndex) add(node types.TraceCausalProjectionNode, zh bool) string {
	if idx == nil {
		return ""
	}
	ref := runtimeTraceCausalProjectionEvidenceRef(node)
	if strings.TrimSpace(ref) == "" {
		return ""
	}
	key := strings.TrimSpace(ref) + "\x00" + runtimeTraceCausalProjectionNodeKey(node)
	if id := idx.seen[key]; id != "" {
		return id
	}
	id := fmt.Sprintf("E%d", len(idx.order)+1)
	idx.seen[key] = id
	if len(node.MergedEvidenceIDs) > 0 {
		idx.hasMergedEvidence = true
	}
	window := ""
	if types.TraceCausalProjectionWindowPresent(node.StartTs, node.EndTs) {
		window = fmt.Sprintf("[%.3f~%.3fs]", node.StartTs, node.EndTs)
	}
	idx.order = append(idx.order, runtimeTraceCausalProjectionEvidenceEntry{
		ID:              id,
		Ref:             strings.TrimSpace(ref),
		Window:          window,
		Details:         runtimeTraceCausalProjectionAuditDetail(node, zh, idx.flatChain),
		SyntheticLine:   node.Undrillable(),
		FamilyAudit:     node.FamilyMemberCount > 1,
		SameValueAudit:  len(node.SameValueMembers) > 0,
		AbsorbedAudit:   node.AbsorbedByRankFamily,
		SupplementAudit: node.SystemSupplement,
	})
	return id
}

func runtimeTraceCausalProjectionEvidenceText(zh bool) string {
	// PTV6-C ruling C (#73, 用户裁定 2026-07-06): the intermediate trace_query
	// record file is no longer a user-facing locator authority — the index
	// itself carries the trace source coordinates (line/time spans of the
	// user's persistent trace artifact).
	// PTV8-RCR-B (UXA 域C #1 + #2 verify 修正稿, 2026-07-08). EVOLUTION
	// RECORD: 「主表/短证据 ID/结构化审计摘要」内部口径词 → 自解释;审计
	// token 七词得图例句闭环(token 本身零改动,§22.2.1 审计车道原文保留).
	// RCM-2 D4 (2026-07-08). EVOLUTION RECORD: the audit-token legend sentence
	// gains the member_* family tokens (引 §24.10/§24.22 — family entries now
	// carry member_count/member_fold_caliber; token 本身零改动).
	// DIAG A1 (§28.11-3(a), 2026-07-09). EVOLUTION RECORD: the audit-token
	// legend sentence gains the same_value_* pair — a cross-thread take-MAX
	// fold whose members tie the published MAX to the µs names those members
	// and their line intervals (token 本身零改动,§22.2.1 审计车道原文保留).
	// WF-2 件④ (2026-07-14). EVOLUTION RECORD: the sentence gains the
	// origin=system_supplement token (SUPP-CORE 修复轮 件5 provenance face) —
	// the member_*/same_value_* extension precedent; token 本身零改动.
	// C8PROSE-1 (§29.164 残余清单收账, 2026-07-20): depth-0 clause marks go
	// full-width per the C8 regime; the audit k=v token faces, their 、-joined
	// roster and every parenthetical interior keep half-width byte-identically
	// (共享词面 token 单点纪律 — the tokens mirror the trace_query wire).
	if zh {
		return "正文用 E1、E2 等编号引用证据；本索引给出每条证据在 trace 中的位置(行号或时间区间)与审计字段。" +
			"审计字段为 trace_query 原文 token，便于回溯核对:tier=证据层级、causality=因果位置、rank=根因排序、confidence=置信度、predicate=判定类型、span=span 名、merged_*=合并明细、member_*=同线程家族合并明细、same_value_*=跨线程取最大折叠中同值到微秒的成员及各自行区间(供核对是否同段)、origin=记录出处(system_supplement=成文前确定性补采所得,非模型查询)；其余字段同为原文 token。"
	}
	return "The answer cites evidence by the E1/E2 numbers; this index gives each entry's location in the trace (line or time span) and its audit fields. " +
		"Audit fields are raw trace_query tokens kept for cross-checking: tier = evidence tier, causality = causal position, rank = root-cause rank, confidence = confidence, predicate = judgment kind, span = span name, merged_* = merge detail, member_* = same-thread family-merge detail, same_value_* = members of a cross-thread take-MAX fold whose values tie to the µs, with each member's own line interval (to check whether they are one segment), origin = record provenance (system_supplement = collected by the deterministic pre-report supplement, not a model query); any other field is likewise a raw token."
}

func runtimeTraceCausalProjectionPriorityCell(node types.TraceCausalProjectionNode, zh bool) string {
	switch {
	case node.IsContextOnlyRow():
		return runtimeTraceCausalProjectionContextLayerCell(node, zh)
	case node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span",
		strings.TrimSpace(node.SemanticClass) != "",
		strings.TrimSpace(node.Tier) == "deterministic_optimization":
		// DCS E1 display word (ledger §23.1, 2026-07-08): an on-chain
		// deterministic-optimization row wears the SAME 确定优化 priority word
		// as the semantic observation lane — one 确定性优化 display family,
		// and never the on_chain 重点关注 root-cause word.
		//
		// EVOLUTION RECORD (审计 #60/#66, §29.25 处置委托 + §29.26 待主会话
		// 落账, 2026-07-10). §29.7-2 ① 原文: "tier 词'确定性优化候选'身份保留".
		// The engine retired the deterministic_optimization tier mint (追认:
		// on-chain semantic rows enter the ordinary primary/secondary/tertiary
		// election, types.go RootCauseTier record) — post-retirement rank-lane
		// semantic rows carry predicate root_cause_primary/…, so the tier arm
		// alone no longer covers them and they fell to 主要关注, losing the
		// adjudicated identity. The typed SemanticClass token now carries the
		// identity (this arm precedes the primary arm — pre-retirement,
		// semantic rows could NEVER wear 主要关注 because their predicate was
		// root_cause_deterministic_optimization); the tier arm stays for
		// legacy persisted records.
		if zh {
			return "确定优化"
		}
		return "optimize"
	case node.Role == types.TraceCausalRolePrimaryRootCause || strings.HasPrefix(strings.TrimSpace(node.Predicate), "root_cause_primary"):
		if zh {
			return "主要关注"
		}
		return "primary focus"
	case node.IsTargetSelfStateRow():
		// SYM (§24.13 裁定一): the target-self rank row wears its own
		// symptom-band word — never the on_chain 重点关注 root-cause word.
		if zh {
			return "自身状态"
		}
		return "self state"
	case strings.TrimSpace(node.ChainRelevance) == "on_chain":
		if zh {
			return "重点关注"
		}
		return "important"
	case strings.TrimSpace(node.ChainRelevance) == "adjacent":
		if zh {
			return "邻近参考"
		}
		return "adjacent context"
	default:
		if zh {
			return "支撑参考"
		}
		return "supporting context"
	}
}

func runtimeTraceCausalProjectionLayerCell(node types.TraceCausalProjectionNode, zh bool) string {
	if node.IsContextOnlyRow() {
		return runtimeTraceCausalProjectionContextLayerCell(node, zh)
	}
	if node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span" {
		if zh {
			return "确定性优化点"
		}
		return "semantic"
	}
	if strings.TrimSpace(node.Tier) == "deterministic_optimization" ||
		strings.TrimSpace(node.SemanticClass) != "" {
		// DCS E1 display word (ledger §23.1): the on-chain optimization row
		// speaks the 确定性优化点 layer word, aligned with the semantic
		// observation lane above — never 链上 root-cause layer wording.
		// EVOLUTION RECORD (审计 #60/#66, §29.25/§29.26, 2026-07-10): with the
		// deterministic_optimization tier mint retired (§29.7-2 全权参赛追认),
		// the typed SemanticClass token carries the adjudicated tier-word
		// identity for post-retirement rank-lane semantic rows; the tier arm
		// stays for legacy persisted records.
		if zh {
			return "确定性优化点"
		}
		return "deterministic optimization"
	}
	if node.IsTargetSelfStateRow() {
		// SYM (§24.13 裁定一): the target-self rank row speaks the SAME
		// 关注线程自身 word the self row-kind lane already uses (UXA B#24
		// family) — never the 主根因/链上 root-cause layer words. Load-bearing
		// on flat shapes where the row renders outside the SelfRows lane.
		if zh {
			return "关注线程自身"
		}
		return "the focused thread itself"
	}
	if node.Role == types.TraceCausalRolePrimaryRootCause || strings.HasPrefix(strings.TrimSpace(node.Predicate), "root_cause_primary") {
		if zh {
			return "主根因"
		}
		return "primary"
	}
	switch strings.TrimSpace(node.ChainRelevance) {
	case "on_chain":
		// PTV5 C30 (#68): the zh panel says 链上 (every sibling branch already
		// speaks zh); the EN face keeps its established on-chain product word.
		// The CMP-7a flat wrapper (runtimeTraceProjCausalPositionLayerCell)
		// matches BOTH spellings.
		if zh {
			return "链上"
		}
		return "on-chain"
	case "adjacent":
		if zh {
			return "邻近链"
		}
		return "adjacent"
	case "background":
		if zh {
			return "背景"
		}
		return "background"
	default:
		if zh {
			return "支撑"
		}
		return "support"
	}
}

// runtimeTraceCausalProjectionContextLayerCell keeps the context-only tier
// orthogonal to causal relevance: a zero-effective background row must never
// be promoted by wording into chain membership. Exact relevance enum only.
func runtimeTraceCausalProjectionContextLayerCell(node types.TraceCausalProjectionNode, zh bool) string {
	switch strings.TrimSpace(node.ChainRelevance) {
	case "on_chain":
		if zh {
			return "链路上下文"
		}
		return "chain context"
	case "adjacent":
		if zh {
			return "邻近上下文"
		}
		return "adjacent context"
	case "background":
		if zh {
			return "背景上下文"
		}
		return "background context"
	default:
		if zh {
			return "上下文"
		}
		return "context"
	}
}

func runtimeTraceCausalProjectionActionCell(node types.TraceCausalProjectionNode, zh bool) string {
	word, _ := runtimeTraceCausalProjectionActionCellWithFamily(node, zh)
	return word
}

// runtimeTraceCausalProjectionActionCellWithFamily returns the action word
// together with the scheduler-state FAMILY it restates ("" = the cell carries
// non-state information: sleep drill guidance, candidate words, optimization
// points). PTV6-C 第三标本修 (b3, 2026-07-06): the family is the typed dedupe
// signal for the #6 absorption net — the tag builder suppresses a pure
// restatement when the same family is already spoken by the row's state tag
// or its cause word, judged on typed tokens only.
func runtimeTraceCausalProjectionActionCellWithFamily(node types.TraceCausalProjectionNode, zh bool) (string, string) {
	causeKind := strings.TrimSpace(strings.ToLower(firstNonEmptyAnswerString(node.Object, node.Predicate)))
	stateKind := strings.TrimSpace(strings.ToLower(node.StateKind))
	if stateKind == "" {
		stateKind = causeKind
	}
	if node.IsSleepState() {
		// PTV8-RCR-C (§24.12 C11 睡眠症状注三态统一, 2026-07-08). EVOLUTION
		// RECORD: one tree spoke three sleep-note states for no principled
		// reason (cmp_78_01: E10 睡眠症状→查上游 / E8 裸 睡眠症状 / E13-E17
		// 无注). The note is now GUIDANCE or nothing — one word, one condition:
		// it renders exactly when a known upstream exists to chase (the A#25
		// gate still suppresses it when that upstream is already a rendered
		// row). The bare 睡眠症状 restatement (the row already wears ☾ + the
		// sleep state word) and the ·缺唤醒边 variant (the ⊘链止 keep-mark and
		// the 无唤醒记录 lane already state that fact) are retired.
		if !node.Undrillable() && strings.TrimSpace(node.DrilldownTarget) != "" {
			if zh {
				return "睡眠症状→查上游", ""
			}
			return "sleep symptom→upstream", ""
		}
		return "", ""
	}
	if causeKind == "compute_supply" {
		// b3 (b): the former 执行/算力 converges onto the canonical running
		// word (the 算力 lane word leaves the demand-side action cell — §7.4);
		// family=running so a same-family state tag still absorbs it.
		return runtimeTraceCausalProjectionStateActionWord("running", zh), "running"
	}
	if node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span" {
		class := strings.TrimSpace(node.SemanticClass)
		if zh {
			if class != "" {
				class = runtimeTraceCausalProjectionDisplayCauseName(class, true)
				return tracefence.ActionWordZH + "·" + runtimeTraceCausalProjectionCompactCellText(class, 22), ""
			}
			return "确定性优化点", ""
		}
		if class != "" {
			return tracefence.ActionWordENShort + "·" + runtimeTraceCausalProjectionCompactCellText(class, 22), ""
		}
		return tracefence.ActionWordEN, ""
	}
	if word := runtimeTraceCausalProjectionStateActionWord(stateKind, zh); word != "" {
		return word, runtimeTraceProjActionJointFamily(stateKind)
	}
	// F5 (§22 PTV7-SPN, 用户裁定 2026-07-07): a diagnostic-lane row (registry
	// Lane==diagnostic — trace_gap/unknown_state/state_churn/…) is a
	// data-quality marker, never a cause candidate — the generic candidate
	// chip stays off; trace_gap rows carry their own inline disclosure
	// (窗内无调度数据·链止) from the metric-parts builder. The ×N(0.000–0.000)
	// all-zero fold row gets the same treatment (typed numeric shape).
	if runtimeTraceProjDiagnosticLaneNode(node) || runtimeTraceProjAllZeroFoldRow(node) {
		return "", ""
	}
	// PTV8-RCR-A (§24.2, 2026-07-08). EVOLUTION RECORD: the generic 候选根因 /
	// candidate cause fallthrough chip is RETIRED tree-wide (空话 chip 全树
	// 退役) — the cause-node 行2 「类别·根因排序#N·置信」 carries the ranking
	// identity, and rows without any cause word simply carry none (the detail
	// table's 影响形态 cell stays the lossless shape home).
	return "", ""
}

// runtimeTraceCausalProjectionStateActionWord is the SINGLE home for the
// action words that merely restate a scheduler state (extracted from the
// ActionCell switch for PTV6-C #6, #73 标本归因 2026-07-06): when the row's
// state tag already speaks the same state family (StateKindLabel), the tag
// builder suppresses this restatement (近义收敛, 全词一处 — the family
// judgment is typed, so PTV7's word collapse does not move it). PTV7 (#74,
// 用户裁定 2026-07-06): the action words ARE the canonical state tokens on
// both faces — the tag / action / state-derived-cause lanes now share ONE
// token set, so absorption-net word-table leaks (the b3 算力 class, the Q4
// 调度等待 collision class) are structurally gone. Rows whose state tag
// carries OTHER information (lock / candidate words) keep their action cell
// untouched.
func runtimeTraceCausalProjectionStateActionWord(stateKind string, zh bool) string {
	_ = zh // PTV7: state words are face-invariant tokens.
	switch strings.TrimSpace(strings.ToLower(stateKind)) {
	case "running":
		// b3 (b) (第三标本, 2026-07-06): canonical convergence onto the 裁定4
		// running word — 执行/算力 put the compute-delivery lane word (算力,
		// §7.4) on demand-side rows and dodged the #6 absorption net whenever
		// the state tag was absent.
		return "running"
	case "runnable":
		// PTV5 Q4 (#68 用户裁定 2026-07-05) ruled the former 调度/优先级 word
		// out (word-table collision with the inversion evidence wording); PTV7
		// replaces its 调度等待 successor with the state token itself — a
		// runnable token cannot collide with any inversion product word.
		return "runnable"
	case "d_state", "io_wait", "d_sleep", "uninterruptible_sleep":
		// The joint blocking family keeps one honest two-sided word.
		return "D-state/iowait"
	}
	return ""
}

func runtimeTraceCausalProjectionImpactShapeCell(node types.TraceCausalProjectionNode, zh bool) string {
	cell, _ := runtimeTraceCausalProjectionImpactShapeCellTyped(node, zh)
	return cell
}

// runtimeTraceCausalProjectionImpactShapeCellTyped is the shape cell plus a
// PRECISE typed signal: generic=true ONLY when the cell fell through every
// typed branch to the category fallback word (候选影响 / candidate impact).
// PTV6-D (b) (#75 标本归因 #10, 2026-07-06): the tree fence suppresses that
// category word per row (the class semantics ride a dedicated legend entry via
// NEW-7); the detail table keeps the full cell — the signal is the branch
// itself, never a display-string comparison.
func runtimeTraceCausalProjectionImpactShapeCellTyped(node types.TraceCausalProjectionNode, zh bool) (string, bool) {
	// §7.30.3 D1: lock-contention rows carry a typed BlockingKind — the
	// duration is a blocked wait on a lock, so it always renders with that
	// semantic label instead of a bare number or a generic candidate word.
	// BLK §15.C ②: a holder-subject rank row was never blocked — its duration
	// is time spent HOLDING the lock while the waiter starved, so the shape
	// word says 持锁, not 阻塞 (same typed gate as the HOLD name).
	if strings.TrimSpace(node.BlockingKind) != "" {
		if node.BlockingSubjectIsHolder {
			if zh {
				return "锁竞争·持锁", false
			}
			return "lock contention · holder", false
		}
		if zh {
			return "锁竞争·阻塞", false
		}
		return "lock contention · blocked", false
	}
	// XERR1-FIX 件2 (§29.104.3/.4): the payload-less blocking_span basis rows
	// speak their own precise shape word — the converged row IS a measured
	// wait-segment total; the envelope fallback makes no blocking claim.
	// Basis-less nodes fall through byte-identically.
	if strings.TrimSpace(node.BlockingKind) == "" {
		switch strings.TrimSpace(node.BlockingValueBasis) {
		case tracequery.BlockingValueBasisWaitSegments:
			if zh {
				return "阻塞等待(span∩窗等待段合计)", false
			}
			return "blocking wait (span∩window wait-segment total)", false
		case tracequery.BlockingValueBasisSpanEnvelope:
			if zh {
				return "span 包络(含运行)", false
			}
			return "span envelope (includes running)", false
		}
	}
	// §7.30.3 D3: an inversion row's impact is the R5d gated COMPOSITE
	// (runnable full + discounted weak-core running) — no single scheduler
	// state may claim it. PTV6-C ruling B (#73, 用户裁定 2026-07-06): the
	// former dedicated "反转影响" shape word is DELETED — the cell speaks the
	// cause FULL word instead (优先级反转候选 / the raw token on EN), so a hop
	// row whose Object is a state token still carries the inversion identity,
	// while rows whose name already shows the full word dedupe the tag away
	// (#12 全词保障 + 全词一处). PTV8-RCR-A (§24 ②): the composition now
	// rides the cause node's 行3 「=」breakdown (the D3 影响构成 tag is
	// retired); the branch still returns early so the composite NEVER falls
	// through to a single-state claim; the other shape-cell-wins forms
	// (锁竞争·阻塞, 候选影响 …) are untouched.
	//
	// A5 反转词位单源 (sweep M8 §29.104.16.1, 2026-07-17). EVOLUTION RECORD:
	// this arm keyed on the candidate lane only and hard-spelled the candidate
	// word, while a third word 「runnable调度候选」 lived on the causeKind
	// switch below for the runnable-overlap token — the flag row and the value
	// row of ONE family split words. Both now speak the ONE per-token composer
	// (typelabels bytes; EN keeps the raw wire token), and the runnable-overlap
	// token's row returns early here too — a typed inversion-family row never
	// falls to a bare single-state claim.
	if word, ok := runtimeTraceProjInversionFamilyWord(node, zh); ok {
		return word, false
	}
	state := strings.TrimSpace(strings.ToLower(node.StateKind))
	switch state {
	case "running":
		if zh {
			return "running / CPU执行", false
		}
		return "running / CPU execution", false
	case "runnable":
		if zh {
			return "runnable / 等待调度", false
		}
		return "runnable / waiting for CPU", false
	case "sleep", "s_sleep", "sleep_wait":
		if zh {
			return "sleep / 等待唤醒", false
		}
		return "sleep / waiting for wakeup", false
	case "d_state", "d_sleep", "uninterruptible_sleep":
		if zh {
			return "D-state / 不可中断等待", false
		}
		return "D-state / uninterruptible wait", false
	case "io_wait":
		if zh {
			return "iowait / IO等待", false
		}
		return "iowait", false
	}
	causeKind := strings.TrimSpace(strings.ToLower(firstNonEmptyAnswerString(node.Object, node.Predicate)))
	switch causeKind {
	case "compute_supply":
		if zh {
			return "CPU供给候选", false
		}
		return "CPU-supply candidate", false
	case "block_io_by_inode", "io_latency":
		if zh {
			return "IO阻塞候选", false
		}
		return "IO-blocking candidate", false
	}
	if node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span" {
		label := "语义优化span"
		if !zh {
			label = "semantic optimization span"
		}
		// §7.30.2 C4: the typed semantic class (class_verification / jit_compile
		// / shader_compile / …) rides the shape cell so the deterministic class
		// survives on the lossless table AND as the tree row's leading tag even
		// when the B4 width cap elides the secondary action/host tags.
		if class := strings.TrimSpace(node.SemanticClass); class != "" {
			class = runtimeTraceCausalProjectionDisplayCauseName(class, zh)
			label += "·" + runtimeTraceCausalProjectionCompactCellText(class, 22)
		}
		return label, false
	}
	// H20: rows that would fall back to the generic candidate word but carry a
	// typed aggregate-activity kind (irq_burst / irq_activity / page_cache_churn)
	// render their typed shape instead. TypeToken (the verbatim producer enum)
	// first; the same token riding the Object cause lane second. Labels live in
	// the typelabels helper file — never scattered strings.
	if label := runtimeTraceAggregateTypeShapeLabel(node.TypeToken, zh); label != "" {
		return label, false
	}
	if label := runtimeTraceAggregateTypeShapeLabel(causeKind, zh); label != "" {
		return label, false
	}
	// PTV6-C #3 (#73, 标本归因 2026-07-06): a row whose typed TypeToken itself
	// carries scheduler-state semantics (d_state_or_io_wait 等) states that
	// state family instead of the generic candidate word — the producer
	// published the state on the type lane, so the row is NOT stateless (the
	// ◦ 无主导态 chip stays silent through the same typed class gate).
	// Display-layer mapping only; the producer's StateKind lane is untouched.
	if class := runtimeTraceCausalProjectionTypeTokenStateClass(node); class != "" {
		// DSTATE-REFINE arm a (件③): the shape word consumes the typed
		// refined-D proof (merged class → unambiguous d_state).
		return runtimeTraceCausalProjectionTypeTokenStateWord(runtimeTraceCausalProjectionRefinedStateClass(node, class), zh), false
	}
	// PTV6-D (b) → PTV8-RCR-B (UXA 域B #21, 2026-07-08). EVOLUTION RECORD:
	// the 「候选影响」 class word shared 候选 with the retired 候选根因 while
	// meaning something unrelated — the generic arm now self-describes
	// (未分类); the tree fence keeps suppressing it per row (legend entry
	// 无类型词的行 carries the class).
	if zh {
		return "未分类(该行无具体状态/类型词)", true
	}
	return "unclassified (no concrete state/type word on this row)", true
}

func runtimeTraceCausalProjectionAuditDetail(node types.TraceCausalProjectionNode, zh bool, flatChain bool) string {
	var parts []string
	if tier := strings.TrimSpace(node.Tier); tier != "" {
		parts = append(parts, "tier="+tier)
	}
	if causality := strings.TrimSpace(node.Causality); causality != "" {
		// CMP-7a: in the flat-fallback shape the tree header states the chain
		// could not be traced upstream — an audit summary claiming on-chain
		// causality right below it is self-contradictory. Display face only
		// (typed token, both languages); the raw observation keeps its verbatim
		// causality note.
		if flatChain && (causality == "on_wakeup_chain" || causality == "on_dependency_chain") {
			parts = append(parts, "chain_shape=flat_untraceable")
		} else {
			parts = append(parts, "causality="+causality)
		}
	}
	if node.Rank > 0 {
		parts = append(parts, fmt.Sprintf("rank=%d", node.Rank))
	}
	if node.Confidence > 0 {
		parts = append(parts, fmt.Sprintf("confidence=%.2f", node.Confidence))
	}
	// SUPP-CORE 修复轮 件5 / 冷读 SC-F1 (2026-07-14): the audit face's typed
	// provenance token — this row's record was minted by the SYSTEM
	// supplement's deterministic re-run, not a model dispatch. Pure render
	// token from the typed node flag (no wire note key — R2' exempt), seated
	// EARLY like the member tokens so free-length predicate/span parts can
	// never push it off the audit ceiling. The user-visible total disclosure
	// stays the single caveat line (R5); this is the per-record audit/replay
	// granularity face.
	if node.SystemSupplement {
		parts = append(parts, "origin=system_supplement")
	}
	// RCM-2 D4 + 复核 F-3 (2026-07-08): the engine family merge's audit tokens
	// — the index entry says it stands for N same-thread members and names the
	// combining ruler (member keys/roster stay on the detail stanza and the
	// raw record). Seated right AFTER confidence, BEFORE the free-length
	// predicate/span parts: the worst REAL prefix (tier=deterministic_
	// optimization + causality=adjacent_to_wakeup_chain + rank + confidence)
	// plus both member tokens measures ≤160 display runes, so the family
	// accounting can never fall off the widened FamilyAudit ceiling — pinned
	// by TestRCM2AuditTokensSurviveWorstCasePrefix.
	if node.FamilyMemberCount > 1 {
		parts = append(parts, fmt.Sprintf("member_count=%d", node.FamilyMemberCount))
		if caliber := strings.TrimSpace(node.FamilyFoldCaliber); caliber != "" {
			parts = append(parts, "member_fold_caliber="+caliber)
		}
	}
	// DIAG A1 (§28.11-3(a) G12, 2026-07-09): the cross-thread take-MAX fold's
	// µs-tie disclosure — subjects + per-member line intervals as two audit
	// tokens (merged_ids family style), seated EARLY like the RCM member
	// tokens so the free-length predicate/span parts can never push the tie
	// witness off the audit ceiling. Same-value entries also widen the
	// ceiling (SameValueAudit flag, FamilyAudit precedent).
	if len(node.SameValueMembers) > 0 {
		subjects := make([]string, 0, len(node.SameValueMembers))
		lines := make([]string, 0, len(node.SameValueMembers))
		for _, member := range node.SameValueMembers {
			subjects = append(subjects, member.Subject)
			lines = append(lines, fmt.Sprintf("%d-%d", member.LineStart, member.LineEnd))
		}
		parts = append(parts, "same_value_members="+strings.Join(subjects, ","))
		parts = append(parts, "same_value_lines="+strings.Join(lines, ","))
	}
	// G1 跨车道对账 (§27.2-G1, 2026-07-09): an absorbed chain-lane entry keeps
	// its family pointer on the audit face (第三面无损 — the raw observation
	// carries the same typed note verbatim; this token makes the E# roster
	// self-explaining without opening the raw record).
	if node.AbsorbedByRankFamily {
		if key := strings.TrimSpace(node.AbsorbedInto); key != "" {
			parts = append(parts, "absorbed_into="+key)
		} else {
			parts = append(parts, "absorbed_by_rank_family=true")
		}
	}
	if pred := strings.TrimSpace(node.Predicate); pred != "" {
		parts = append(parts, "predicate="+pred)
	}
	// F2 (§22 PTV7-SPN): the parsed span name joins the audit summary —
	// SpanName non-empty rows only (typed note; the evidence index was the
	// one surface with the name in hand and nowhere to show it).
	if name := strings.TrimSpace(node.SpanName); name != "" {
		parts = append(parts, "span="+name)
	}
	// Aggregation provenance (presentation v3 §6): every merged observation id
	// stays auditable from the roster entry.
	if node.MergedCount > 1 {
		parts = append(parts, fmt.Sprintf("merged_count=%d", node.MergedCount))
		// G12-ENG (§29.1, 2026-07-09): valueless-member accounting on the audit
		// face — merged_count alone let the E23 fold read as two same-value
		// observations when one member carried no duration at all.
		if node.MergedValuelessCount > 0 {
			parts = append(parts, fmt.Sprintf("merged_valueless=%d", node.MergedValuelessCount))
		}
	}
	// F5: the duplicate-publication fold (ONE measurement republished N times)
	// is typed provenance distinct from both the R1 same-fact merge and the R2
	// ×N SUM — without this field the evidence index (third surface) could not
	// tell the fold kinds apart.
	if node.DuplicatePublications > 1 {
		parts = append(parts, fmt.Sprintf("duplicate_publications=%d", node.DuplicatePublications))
	}
	if len(node.MergedEvidenceIDs) > 0 {
		parts = append(parts, "merged_ids="+strings.Join(node.MergedEvidenceIDs, ","))
	}
	if len(parts) == 0 {
		if zh {
			return "结构化 trace_query 观测"
		}
		return "structured trace_query observation"
	}
	return strings.Join(parts, " · ")
}

func runtimeTraceCausalProjectionCanonicalNode(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func runtimeTraceCausalProjectionNodeKey(node types.TraceCausalProjectionNode) string {
	if id := strings.TrimSpace(node.EvidenceID); id != "" {
		return id
	}
	return strings.ToLower(strings.TrimSpace(node.Subject)) + "\x00" +
		strings.ToLower(strings.TrimSpace(node.Object)) + "\x00" +
		strings.ToLower(strings.TrimSpace(node.Predicate))
}

func runtimeTraceCausalProjectionPrimaryRoots(projection types.TraceCausalProjection) []types.TraceCausalProjectionNode {
	if len(projection.PrimaryRootCauses) > 0 {
		return projection.PrimaryRootCauses
	}
	if projection.PrimaryRootCause == nil {
		return nil
	}
	return []types.TraceCausalProjectionNode{*projection.PrimaryRootCause}
}

func runtimeTraceCausalProjectionUseChinese(lang string) bool {
	lang = strings.ToLower(strings.TrimSpace(lang))
	return lang == "" || answerDocumentRequiresChinese(lang)
}

func runtimeTraceCausalProjectionTitle(lang string) string {
	// UX-ANCHOR 件a (§29.61.7, 2026-07-14): the title bytes come from the
	// tracefence table-④ single source — the preview lead-segment decorator
	// keys its lead-scope boundary on the same constant (byte-identical).
	if runtimeTraceCausalProjectionUseChinese(lang) {
		return tracefence.SectionProjectionZH
	}
	return tracefence.SectionProjectionEN
}

func runtimeTraceCausalProjectionNodeSubjectCell(node types.TraceCausalProjectionNode, zh bool) string {
	if node.IsAggregateMetric() {
		return runtimeTraceCausalProjectionCompactCellText(runtimeTraceCausalProjectionAggregateMetricName(node, zh), 44)
	}
	// RNB-5B 件⑦: the micro anchored-seat fold names its own family on the
	// (a) table too (三面同词 with the tree 行1 and the detail block).
	if node.MicroAnchorFold {
		return runtimeTraceCausalProjectionCompactCellText(
			runtimeTraceProjMicroAnchorFoldName(node, zh)+runtimeTraceProjMergedSubjectsSuffix(node, zh), 96)
	}
	// §7.30.3 D1: contention rows lead with the typed lock semantics (holder
	// included when the payload named one) instead of a peer/type word. The
	// holder label is the load-bearing information — the lossless table keeps
	// it whole (a 96-rune ceiling only guards against degenerate payloads).
	if blocking := runtimeTraceCausalProjectionBlockingName(node, zh); blocking != "" {
		if runtimeTraceCausalProjectionKnownSubject(node.Subject) {
			subject := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(node, zh))
			return runtimeTraceCausalProjectionCompactCellText(subject, 28) + " / " + runtimeTraceCausalProjectionCompactCellText(blocking, 96)
		}
		return runtimeTraceCausalProjectionCompactCellText(blocking, 96)
	}
	subject := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(node, zh))
	// D2: the zh table's Node/cause column shows the concise label; the raw
	// token stays lossless in the dedicated 类型 column.
	object := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(node, zh))
	// F1 (§22 PTV7-SPN P0): the span-name consumption gate is SpanName
	// non-empty (shared helper) — the semantic gate's only remaining job here
	// is the wider objectLimit split below.
	objectLimit := 22
	if runtimeTraceCausalProjectionSemanticSpanRow(node) {
		objectLimit = 36
	}
	// SEM-LEAD (§29.7-2 ④, ledger real_trace_campaign_20260705.md,
	// 2026-07-10): a semantic FAMILY row's node cell speaks the typed class
	// word (词值同源 with the tree 行1; 792-textup witness: the (a) table read
	// "Texture upload(15573) 1140x1856 ×11合计" — one member's span name
	// impersonating the ×11 family). Members stay lossless on the (b) roster.
	if runtimeTraceProjFamilyRow(node) && strings.TrimSpace(node.SemanticClass) != "" &&
		runtimeTraceProjFamilySemanticClassWord(node, zh) != "" {
		object = runtimeTraceProjFamilySemanticClassWord(node, zh)
	} else if runtimeTraceCausalProjectionSemanticSpanRow(node) && strings.TrimSpace(node.SpanName) != "" {
		object = strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(node.SpanName, zh))
	} else if spanWord := runtimeTraceCausalProjectionSpanNameObjectWord(node, zh); spanWord != "" {
		object = spanWord
		// 用户裁定 2026-07-07 ("trace span" 专用名词不翻译) 连锁: the composite
		// "name(type word)" wider than this cell's rune budget drops the
		// type-word garnish and keeps the BARE real name — truncating the
		// composite would cut into the name/type token ("H:ReceiveVsync(trace …")
		// and is strictly worse than the name alone. A bare name still over
		// budget keeps the existing rune truncation below. Pure length
		// comparison (precise signal); the tree row (36-budget name lane) and
		// the lossless block keep the full composite.
		if len([]rune(object)) > objectLimit {
			if bare := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(node.SpanName, zh)); bare != "" {
				object = bare
			}
		}
	}
	switch {
	case subject != "" && object != "":
		return runtimeTraceCausalProjectionCompactCellText(subject, 28) + " / " + runtimeTraceCausalProjectionCompactCellText(object, objectLimit)
	case subject != "":
		return runtimeTraceCausalProjectionCompactCellText(subject, 44)
	case object != "":
		return runtimeTraceCausalProjectionCompactCellText(object, 44)
	default:
		// F4 (§22 PTV7-SPN, C39 漏面): the on-chain overflow fold names its
		// count + roster on THIS surface too — mirroring the tree row and the
		// lossless block (P2a rider 件1 lockstep, §29.55.3 2026-07-13: the
		// dedup form 其余N项(折叠); the lane lives on the tree face's edge word
		// and the detail block's 因果位置 line); and the subject/object-less
		// fallback speaks zh on the zh panel (the EN placeholder stays
		// EN-face only — "(unnamed causal node)" per RUN2FIX-A 复核 CR-4).
		if node.OnChainOverflowFold {
			if zh {
				return runtimeTraceCausalProjectionCompactCellText(
					fmt.Sprintf("其余 %d 项(折叠)%s", node.MergedCount, runtimeTraceProjMergedSubjectsSuffix(node, zh)), 44)
			}
			return runtimeTraceCausalProjectionCompactCellText(
				fmt.Sprintf("%d more (folded)%s", node.MergedCount, runtimeTraceProjMergedSubjectsSuffix(node, zh)), 44)
		}
		// Catalog B9 (DISPLAY-HYG 二轮): member preview beats the opaque
		// placeholder on this face too (same single source as the detail
		// block's full-name fallback).
		if preview := runtimeTraceProjAnonymousNodePreviewName(node, zh); preview != "" {
			return runtimeTraceCausalProjectionCompactCellText(preview, 44)
		}
		if zh {
			return "(未命名因果节点)"
		}
		return "(unnamed causal node)"
	}
}

func runtimeTraceCausalProjectionDisplayNodeName(raw string, zh bool) string {
	raw = strings.TrimSpace(raw)
	if runtimeTraceCausalProjectionUnknownSentinel(raw) {
		// Raw-string lane: no node context, so the wording stays generic.
		return runtimeTraceCausalProjectionUnresolvedPeerText("", zh)
	}
	// RN-4 (§7.9 runnable 主导场景审计 2026-07-04): the systrace comm-truncation
	// placeholder "<...>-N" is a SYSTEM-generated token (the kernel never
	// recorded the comm), not a thread name — rendering it verbatim read as
	// line noise. Display-only rewrite through the single R1 wording helper;
	// canonical/grouping keys keep the raw subject untouched.
	if tid, ok := runtimeTraceCausalProjectionCommTruncatedTid(raw); ok {
		return runtimeTraceCausalProjectionUnrecordedThreadText(tid, zh)
	}
	return raw
}

// runtimeTraceCausalProjectionCommTruncatedTid matches the exact systrace
// comm-truncation subject shape "<...>-N": the name part VERBATIM equal to
// the "<...>" truncation token plus the tokenizer's "-tid" suffix (pure
// digits). A character-class check on a system-generated marker — never a
// prose heuristic; any real thread name (even one containing dots or angle
// brackets elsewhere) fails the exact-prefix or pure-digit test.
func runtimeTraceCausalProjectionCommTruncatedTid(raw string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(raw), "<...>-")
	if !ok || rest == "" {
		return "", false
	}
	for _, r := range rest {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return rest, true
}

// runtimeTraceCausalProjectionUnrecordedThreadText is THE single wording home
// (R1 措辞族, beside runtimeTraceCausalProjectionUnresolvedPeerText) for the
// RN-4 comm-truncation display: the trace recorded the tid but not the name.
// Display-lane only — never a match key or behavior gate.
func runtimeTraceCausalProjectionUnrecordedThreadText(tid string, zh bool) string {
	if zh {
		return "线程名未记录(tid " + tid + ")"
	}
	return "unnamed thread (tid " + tid + ")"
}

// runtimeTraceCausalProjectionUnknownSentinel reports the exact data-layer
// unknown-thread sentinel (typed token match, never a prose heuristic).
func runtimeTraceCausalProjectionUnknownSentinel(raw string) bool {
	switch runtimeTraceCausalProjectionCanonicalNode(raw) {
	case "unknown-thread", "unknown":
		return true
	}
	return false
}

// runtimeTraceCausalProjectionUnresolvedPeerText is THE single home for the
// unknown-thread sentinel wording (customer 2026-07-03: "未定位线程" was
// unreadable). kind is the row's typed kind token from
// runtimeTraceCausalProjectionUnresolvedPeerKind ("" = no typed kind, generic
// wording). Display-lane only — never used as a match key or behavior gate.
func runtimeTraceCausalProjectionUnresolvedPeerText(kind string, zh bool) string {
	switch kind {
	case "blocking_span":
		if zh {
			return "阻塞等待(对端未解析)"
		}
		return "blocking wait (peer unresolved)"
	case runtimeTraceBlockingSpanKindWaitSegments:
		// XERR1-FIX 件2 (§29.104.4): the converged row's value IS the waiter's
		// proven wait segments, so the 阻塞等待 word stays true (值真词才真).
		if zh {
			return "阻塞等待(对端未解析)"
		}
		return "blocking wait (peer unresolved)"
	case runtimeTraceBlockingSpanKindEnvelope:
		// 件2 词面退路: the value is still the span envelope — the word must
		// not claim a blocking wait.
		if zh {
			return "span 包络(含运行;对端未解析)"
		}
		return "span envelope (includes running; peer unresolved)"
	case "d_state_or_io_wait":
		// PTV7 (#74): the state compound speaks the canonical tokens; the
		// peer-relation frame stays localized.
		if zh {
			return "D-state/iowait(对端未解析)"
		}
		return "D-state/iowait (peer unresolved)"
	case "d_state_refined":
		// DSTATE-REFINE arm a (件③): the coverage-proven refined form.
		if zh {
			return "D-state(对端未解析)"
		}
		return "D-state (peer unresolved)"
	default:
		if zh {
			return "对端线程未解析"
		}
		return "unresolved wait peer"
	}
}

// runtimeTraceBlockingSpanKind* (XERR1-FIX 件2, §29.104.4, 2026-07-15): the
// basis-forked kind vocabulary of a payload-less blocking_span row. The fork
// key is the producer's typed blocking_value_basis note — wait_segments keeps
// the 阻塞等待 word family (the value was converged to the waiter's proven
// wait segments) with the peer DEMOTED to 「span 期间最后唤醒者(推断)」;
// span_envelope retreats the whole word face to 「span 包络(含运行)」. A
// basis-less node (payload-typed rows, legacy artifacts) keeps the legacy
// "blocking_span" kind and every legacy byte.
const (
	runtimeTraceBlockingSpanKindWaitSegments = "blocking_span_wait_segments"
	runtimeTraceBlockingSpanKindEnvelope     = "blocking_span_envelope"
)

// runtimeTraceCausalProjectionBlockingSpanBasisKind applies the 件2 basis fork
// to a base "blocking_span" kind. Exact typed enum match; unknown basis values
// keep the legacy kind (fail-open to legacy wording).
//
// XERR1-EXT 件2 (§29.104.17 裁定⑤, 2026-07-16): payload-TYPED rows
// (BlockingKind!="") now carry the typed basis too (their VALUE converged),
// but their word family stays the lock vocabulary (锁竞争·阻塞/持锁, 对端 /
// holder words) — the kind fork is payload-less-only, same typed gate as
// runtimeTraceProjBlockingSpanBasisImpactForm (one contract, two forks).
func runtimeTraceCausalProjectionBlockingSpanBasisKind(node types.TraceCausalProjectionNode, kind string) string {
	if kind != "blocking_span" || strings.TrimSpace(node.BlockingKind) != "" {
		return kind
	}
	switch strings.TrimSpace(node.BlockingValueBasis) {
	case tracequery.BlockingValueBasisWaitSegments:
		return runtimeTraceBlockingSpanKindWaitSegments
	case tracequery.BlockingValueBasisSpanEnvelope:
		return runtimeTraceBlockingSpanKindEnvelope
	}
	return kind
}

// runtimeTraceCausalProjectionPeerKindToken classifies one producer-side typed
// token into the peer-relation kind vocabulary shared by the unresolved AND
// resolved peer wordings (PTV6-C #7 — one token table, two wording arms).
// io_latency joins for the RESOLVED arm only (an unresolved io_latency peer
// keeps the generic wording exactly as before — the unresolved lane never
// consulted io_latency).
func runtimeTraceCausalProjectionPeerKindToken(token string) string {
	switch runtimeTraceCausalProjectionCanonicalNode(token) {
	case "blocking_span":
		return "blocking_span"
	case "d_state_or_io_wait":
		return "d_state_or_io_wait"
	}
	return ""
}

// runtimeTraceCausalProjectionUnresolvedPeerKind derives the typed kind that
// specializes the unresolved-peer wording: an EXACT typed-token match on the
// node's verbatim TypeToken ("type=" rich note), Predicate, or Object — all
// producer-side typed enums, never model prose. "" when none matches (callers
// fall back to the generic wording).
func runtimeTraceCausalProjectionUnresolvedPeerKind(node types.TraceCausalProjectionNode) string {
	for _, token := range []string{node.TypeToken, node.Predicate, node.Object} {
		if kind := runtimeTraceCausalProjectionPeerKindToken(token); kind != "" {
			// XERR1-FIX 件2: the payload-less blocking_span kind forks on the
			// typed value basis (one fork point, every wording arm follows).
			return runtimeTraceCausalProjectionBlockingSpanBasisKind(node, kind)
		}
	}
	return ""
}

// runtimeTraceCausalProjectionResolvedPeerKind derives the typed kind for the
// RESOLVED peer-relation wording (#7): the type lanes ONLY (TypeToken, then
// Predicate) — the Object slot holds the resolved peer thread itself, so it
// never votes here. io_latency is a member of this arm (the ruling specimen:
// io_latency rows whose peer resolved to udk-irq-1-63).
func runtimeTraceCausalProjectionResolvedPeerKind(node types.TraceCausalProjectionNode) string {
	for _, token := range []string{node.TypeToken, node.Predicate} {
		if kind := runtimeTraceCausalProjectionPeerKindToken(token); kind != "" {
			// XERR1-FIX 件2: same basis fork as the unresolved arm (one fork
			// point, two wording lanes).
			return runtimeTraceCausalProjectionBlockingSpanBasisKind(node, kind)
		}
		if runtimeTraceCausalProjectionCanonicalNode(token) == "io_latency" {
			return "io_latency"
		}
	}
	return ""
}

// runtimeTraceCausalProjectionResolvedPeerText is the resolved twin of
// runtimeTraceCausalProjectionUnresolvedPeerText and lives in the SAME wording
// home (PTV6-C #7, #73 标本归因 2026-07-06): a resolved peer thread name never
// occupies the cause word slot bare — the relation form says what the wait IS
// and who the peer is ("IO等待(对端 udk-irq-1-63)"). Display-lane only.
func runtimeTraceCausalProjectionResolvedPeerText(kind, peer string, zh bool) string {
	switch kind {
	case "blocking_span":
		if zh {
			return "阻塞等待(对端 " + peer + ")"
		}
		return "blocking wait (peer " + peer + ")"
	case runtimeTraceBlockingSpanKindWaitSegments:
		// XERR1-FIX 件2: a payload-less row's counterpart is a wakeup-edge
		// sample, never a payload-confirmed 对端 — the peer word demotes to
		// 最后唤醒者(推断) while the converged value keeps 阻塞等待 true.
		if zh {
			return "阻塞等待(span 期间最后唤醒者(推断) " + peer + ")"
		}
		return "blocking wait (last waker during span, inferred: " + peer + ")"
	case runtimeTraceBlockingSpanKindEnvelope:
		if zh {
			return "span 包络(含运行;span 期间最后唤醒者(推断) " + peer + ")"
		}
		return "span envelope (includes running; last waker during span, inferred: " + peer + ")"
	case "d_state_or_io_wait":
		// PTV7 (#74): same canonical compound as the unresolved arm (同形).
		if zh {
			return "D-state/iowait(对端 " + peer + ")"
		}
		return "D-state/iowait (peer " + peer + ")"
	case "d_state_refined":
		// DSTATE-REFINE arm a (件③, 同形 twin of the unresolved arm).
		if zh {
			return "D-state(对端 " + peer + ")"
		}
		return "D-state (peer " + peer + ")"
	case "io_latency":
		if zh {
			return "IO等待(对端 " + peer + ")"
		}
		return "IO wait (peer " + peer + ")"
	}
	return peer
}

// runtimeTraceCausalProjectionPeerRelationShortWord (§29.58.5 ③, 2026-07-13)
// is the STATE-WORD HEAD of the peer-relation forms above — the short form a
// dedup fold row's 行1 keeps when the width fit cannot hold the full relation
// word (「主体 · IO等待 2次同值」 主行三要素). SAME wording home as the two
// composers (one word source — never a string cut of the composed form).
// "" for unknown kinds (labels are never fabricated).
func runtimeTraceCausalProjectionPeerRelationShortWord(kind string, zh bool) string {
	switch kind {
	case "blocking_span", runtimeTraceBlockingSpanKindWaitSegments:
		if zh {
			return "阻塞等待"
		}
		return "blocking wait"
	case runtimeTraceBlockingSpanKindEnvelope:
		// XERR1-FIX 件2 词面退路: never a blocking claim on the envelope basis.
		if zh {
			return "span 包络(含运行)"
		}
		return "span envelope (includes running)"
	case "d_state_or_io_wait":
		return "D-state/iowait"
	case "d_state_refined":
		return "D-state"
	case "io_latency":
		if zh {
			return "IO等待"
		}
		return "IO wait"
	}
	return ""
}

// runtimeTraceCausalProjectionKnownSubject reports whether a subject names a
// real (possibly partial, pid-only) thread — not empty and not the
// unknown-thread sentinel. [Low 修正轮 2026-07-06] single authority: this is a
// pure delegate to the exported types gate (the same one the near-lane folds
// consume) — the former local twin is gone; the two layers can never drift
// (pin: TestPTV6KnownSubjectSingleAuthority).
func runtimeTraceCausalProjectionKnownSubject(raw string) bool {
	return types.TraceCausalProjectionKnownSubject(raw)
}

// runtimeTraceCausalProjectionInversionRow reports whether this node publishes
// the R5d GATED COMPOSITE impact (§7.30.3 D3) — exact typed token match on the
// row's cause type. Only these rows replace the single-state tag with the
// dedicated inversion-impact label and the gated composition split.
func runtimeTraceCausalProjectionInversionRow(node types.TraceCausalProjectionNode) bool {
	// PTV5 Q4 (#68 用户裁定 2026-07-05): the typed node field is the primary
	// signal (hop rows carry the candidacy note while their Object holds the
	// dominant state); the Object-token lane stays for root_cause rows whose
	// Object rides the type token.
	return node.PriorityInversionCandidate ||
		runtimeTraceCausalProjectionCanonicalNode(node.Object) == "priority_inversion_candidate"
}

// runtimeTraceCausalProjectionBlockingName renders the typed lock-contention
// semantics (§7.30.3 D1): the row is a wait ON A LOCK, and when the structured
// payload named the owner, the holder is displayed with its thread label
// (name-tid). Empty when the node carries no typed BlockingKind.
func runtimeTraceCausalProjectionBlockingName(node types.TraceCausalProjectionNode, zh bool) string {
	if strings.TrimSpace(node.BlockingKind) == "" {
		return ""
	}
	// P0-E 锁车道修3 (§24.9-C F5): an INFERRED holder identity (closing
	// wakeup edge / ns-span derivation — typed holder_source lanes) carries a
	// short 推断 qualifier on the row face; the detail stanza's 持有者来历
	// line carries the full origin sentence. Payload-direct holders render
	// unchanged.
	inferred := runtimeTraceProjBlockingHolderInferred(node)
	// BLK §15.C: the resolved rank lock row's subject IS the holder — render a
	// HOLD ("持锁阻塞了 <waiter>"), never the reversed lock-WAIT that the
	// waiter-subject critical_blocking row already carries for the SAME physical
	// lock. The waiter here is BlockingPeer (the row subject's contention
	// counterpart).
	if node.BlockingSubjectIsHolder {
		waiter := strings.TrimSpace(node.BlockingPeer)
		if zh {
			switch {
			case waiter != "" && inferred:
				return "持锁阻塞(等待方 " + waiter + ";持有者推断)"
			case waiter != "":
				return "持锁阻塞(等待方 " + waiter + ")"
			case inferred:
				return "持锁阻塞(持有者推断)"
			}
			return "持锁阻塞"
		}
		switch {
		case waiter != "" && inferred:
			return "holds lock, blocking " + waiter + " (holder inferred)"
		case waiter != "":
			return "holds lock, blocking " + waiter
		case inferred:
			return "holds lock (holder inferred)"
		}
		return "holds lock"
	}
	name := "锁竞争等待"
	if !zh {
		name = "lock contention wait"
	}
	if peer := strings.TrimSpace(node.BlockingPeer); peer != "" {
		switch {
		case zh && inferred:
			name += "(持有者 " + peer + "·推断)"
		case zh:
			name += "(持有者 " + peer + ")"
		case inferred:
			name += " (owner " + peer + ", inferred)"
		default:
			name += " (owner " + peer + ")"
		}
	}
	return name
}

// runtimeTraceProjBlockingHolderInferred is the ONE typed predicate the three
// P0-E 锁车道修3 disclosure faces share: the row's resolved holder identity
// came from an INFERENCE lane (the waiter's closing wakeup edge or the LCK-2
// ns-span derivation), never the payload-direct origin. Exact typed enum
// match on the producer's holder_source note — prose never participates.
func runtimeTraceProjBlockingHolderInferred(node types.TraceCausalProjectionNode) bool {
	if strings.TrimSpace(node.BlockingKind) == "" {
		return false
	}
	switch node.BlockingHolderSource {
	case tracequery.CounterpartSourceWakeupEdge, tracequery.CounterpartSourceNsSpanDerivation:
		return true
	default:
		return false
	}
}

// runtimeTraceCausalProjectionDisplaySubjectName is the node-aware subject
// display (§7.30 裁定2): typed aggregate-metric rows show the metric semantics
// (there is no thread to resolve), sentinel subjects render the typed
// unresolved-peer wording (specialized by the row's kind token when one is
// present), and every other subject — including the data layer's pid=1234
// partial identities — renders verbatim.
func runtimeTraceCausalProjectionDisplaySubjectName(node types.TraceCausalProjectionNode, zh bool) string {
	if node.IsAggregateMetric() {
		return runtimeTraceCausalProjectionAggregateMetricName(node, zh)
	}
	if runtimeTraceCausalProjectionUnknownSentinel(node.Subject) {
		return runtimeTraceCausalProjectionUnresolvedPeerText(runtimeTraceCausalProjectionUnresolvedPeerKind(node), zh)
	}
	return runtimeTraceCausalProjectionDisplayNodeName(node.Subject, zh)
}

// runtimeTraceCausalProjectionAggregateMetricName maps a typed aggregate-metric
// row to its metric semantic name, keyed on the row's typed Object (the
// root-cause type word). Unlisted metric types keep the generic aggregate label
// with the type word preserved.
func runtimeTraceCausalProjectionAggregateMetricName(node types.TraceCausalProjectionNode, zh bool) string {
	metric := strings.TrimSpace(strings.ToLower(firstNonEmptyAnswerString(node.Object, node.Predicate)))
	switch metric {
	case "io_pressure":
		if strings.TrimSpace(node.IOPressureEvidenceQuality) == tracequery.IOPressureEvidenceQualityActivityMarkerOnly {
			if zh {
				return "窗口IO活动标记(聚合)"
			}
			return "window IO activity markers (aggregate)"
		}
		if zh {
			return "窗口IO压力(聚合)"
		}
		return "window IO pressure (aggregate)"
	case "cpu_pressure":
		if zh {
			return "CPU竞争压力(聚合)"
		}
		return "CPU contention pressure (aggregate)"
	case "supply_pressure":
		if zh {
			return runtimeTraceSupplyPressureDisplayLabel(true) + "·聚合"
		}
		return runtimeTraceSupplyPressureDisplayLabel(false) + " · aggregate"
	}
	if metric == "" {
		metric = types.TraceCausalSubjectKindAggregateMetric
	}
	if zh {
		return "窗口聚合指标(" + metric + ")"
	}
	return "window aggregate metric (" + metric + ")"
}

// runtimeTraceCausalProjectionAuditCellText truncates an evidence-index audit
// summary at its " · " PART boundaries (§22 PTV7-SPN F2): the legacy 72-rune
// mid-token cut shipped "confidenc…" / "predicate=wakeup_causal_i…" — the
// confidence value and the predicate name both lost. Both offered halves of
// the fix land together, with reasons: the cap rises to 96 (the §7.30.3
// blocking-name ceiling precedent — 96 alone still cuts inside long predicate
// tokens) AND the cut backs off to whole parts (part-boundary alone at 72
// would drop the confidence part this fix exists to surface). A degenerate
// over-wide single part falls back to the legacy rune cut (never returns an
// empty cell). The trailing "…" states that tail parts were dropped; the full
// summary stays lossless on the raw observation record.
func runtimeTraceCausalProjectionAuditCellText(raw string, maxRunes int) string {
	raw = strings.TrimSpace(raw)
	if maxRunes <= 0 || len([]rune(raw)) <= maxRunes {
		return runtimeTraceCausalProjectionCompactCellText(raw, maxRunes)
	}
	const sep = " · "
	kept := ""
	for _, part := range strings.Split(raw, sep) {
		trial := part
		if kept != "" {
			trial = kept + sep + part
		}
		// +1 reserves the "…" truncation marker's slot.
		if len([]rune(trial))+1 > maxRunes {
			break
		}
		kept = trial
	}
	if kept == "" {
		return runtimeTraceCausalProjectionCompactCellText(raw, maxRunes)
	}
	return runtimeTraceCausalProjectionMarkdownSafe(kept + "…")
}

func runtimeTraceCausalProjectionCompactCellText(raw string, maxRunes int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || maxRunes <= 0 {
		return runtimeTraceCausalProjectionMarkdownSafe(raw)
	}
	runes := []rune(raw)
	if len(runes) <= maxRunes {
		return runtimeTraceCausalProjectionMarkdownSafe(raw)
	}
	if maxRunes <= 1 {
		return "…"
	}
	return runtimeTraceCausalProjectionMarkdownSafe(string(runes[:maxRunes-1]) + "…")
}

func runtimeTraceCausalProjectionMarkdownSafe(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// EVOLUTION RECORD (WF-range 用户裁定, SMR-1 批 2026-07-12): the SINGLE
	// tilde is no longer escaped — "~" is the adjudicated VALUE-RANGE glyph
	// (×3(4.426~6.768ms); the en-dash misread as minus in arithmetic-dense
	// reports), and both render faces already treat a single "~" as literal
	// (min-run-2 strikethrough ruling 2026-07-05, internal/render +
	// internal/markdownext). Double "~~" keeps its GFM escape.
	return strings.ReplaceAll(raw, "~~", "\\~\\~")
}

func runtimeTraceCausalProjectionEvidenceRef(node types.TraceCausalProjectionNode) string {
	for _, ref := range node.SupportRefs {
		if s := strings.TrimSpace(ref); s != "" {
			return s
		}
	}
	if node.LineStart <= 0 {
		return ""
	}
	if node.LineEnd > node.LineStart {
		return fmt.Sprintf("lines=%d-%d", node.LineStart, node.LineEnd)
	}
	return fmt.Sprintf("line=%d", node.LineStart)
}

func runtimeTraceCausalProjectionEvidenceDisplayRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "line=") || strings.HasPrefix(ref, "lines=") {
		return runtimeTraceCausalProjectionMarkdownSafe(ref)
	}
	pathPart, suffix := runtimeTraceCausalProjectionSplitLineSuffix(ref)
	if strings.TrimSpace(pathPart) == "" {
		return runtimeTraceCausalProjectionCompactCellText(ref, 64)
	}
	tail := runtimeTraceCausalProjectionPathTail(pathPart, 1)
	if tail == "" {
		return runtimeTraceCausalProjectionCompactCellText(ref, 64)
	}
	tail = strings.TrimPrefix(tail, "…/")
	if suffix != "" {
		const maxLocatorRunes = 64
		suffixRunes := len([]rune(suffix))
		tailLimit := maxLocatorRunes - suffixRunes
		if tailLimit < 12 {
			tailLimit = 12
		}
		return runtimeTraceCausalProjectionCompactCellText(tail, tailLimit) + runtimeTraceCausalProjectionMarkdownSafe(suffix)
	}
	return runtimeTraceCausalProjectionCompactCellText(tail, 64)
}

// runtimeTraceCausalProjectionEvidenceDisplayRefWithWindow prefers the
// "basename [start–end s]" locator when the node exposed its own trace window
// (§7.30 裁定6 — a time window locates evidence for the reader where an
// 800k-line range does not); without a window it falls back to the
// basename:line-range display. The full raw locator stays in the original
// trace_query record either way.
func runtimeTraceCausalProjectionEvidenceDisplayRefWithWindow(ref, window string) string {
	window = strings.TrimSpace(window)
	if window == "" {
		return runtimeTraceCausalProjectionEvidenceDisplayRef(ref)
	}
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "line=") || strings.HasPrefix(ref, "lines=") {
		return runtimeTraceCausalProjectionMarkdownSafe(window)
	}
	pathPart, _ := runtimeTraceCausalProjectionSplitLineSuffix(ref)
	tail := strings.TrimPrefix(runtimeTraceCausalProjectionPathTail(pathPart, 1), "…/")
	if tail == "" {
		return runtimeTraceCausalProjectionMarkdownSafe(window)
	}
	return runtimeTraceCausalProjectionCompactCellText(tail, 48) + " " + runtimeTraceCausalProjectionMarkdownSafe(window)
}

// runtimeTraceCausalProjectionBareLineRef reports whether ref is the naked
// line=N / lines=N-M form emitted when a node carried no SupportRefs path
// (literal prefix + digit/dash character-class check) and returns the numeric
// range part.
func runtimeTraceCausalProjectionBareLineRef(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	for _, prefix := range []string{"lines=", "line="} {
		if !strings.HasPrefix(ref, prefix) {
			continue
		}
		lineRange := strings.TrimPrefix(ref, prefix)
		if runtimeTraceCausalProjectionLineSuffix(lineRange) {
			return lineRange, true
		}
	}
	return "", false
}

// runtimeTraceCausalProjectionSoleArtifactPath returns the single distinct
// artifact path shared by every path-carrying locator in the roster, or ""
// when none exists or more than one artifact appears (H19 display fallback
// applies only when the artifact is unambiguous).
func runtimeTraceCausalProjectionSoleArtifactPath(entries []runtimeTraceCausalProjectionEvidenceEntry) string {
	shared := ""
	for _, entry := range entries {
		if _, bare := runtimeTraceCausalProjectionBareLineRef(entry.Ref); bare {
			continue
		}
		pathPart, suffix := runtimeTraceCausalProjectionSplitLineSuffix(entry.Ref)
		if suffix == "" || strings.TrimSpace(pathPart) == "" {
			continue
		}
		if shared == "" {
			shared = pathPart
			continue
		}
		if pathPart != shared {
			return ""
		}
	}
	return shared
}

func runtimeTraceCausalProjectionSplitLineSuffix(ref string) (string, string) {
	for i := len(ref) - 1; i >= 0; i-- {
		if ref[i] != ':' {
			continue
		}
		suffix := ref[i+1:]
		if runtimeTraceCausalProjectionLineSuffix(suffix) {
			return strings.TrimSpace(ref[:i]), ref[i:]
		}
	}
	return ref, ""
}

func runtimeTraceCausalProjectionLineSuffix(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	seenDigit := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			seenDigit = true
			continue
		}
		if ch == '-' && seenDigit {
			continue
		}
		return false
	}
	return seenDigit
}

func runtimeTraceCausalProjectionPathTail(path string, components int) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	normalized := strings.ReplaceAll(path, "\\", "/")
	trimmed := strings.Trim(normalized, "/")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	if components <= 0 {
		components = 1
	}
	if len(parts) > components {
		parts = parts[len(parts)-components:]
		return "…/" + strings.Join(parts, "/")
	}
	return strings.Join(parts, "/")
}

// materializeRuntimeTraceSemanticOptimizationBlock renders the deterministic
// semantic optimization points (class verification / JIT / shader / runtime
// compile / texture upload / explicit GC-pause spans) that the projection carries as typed
// SemanticSpans with a semantic_class lane) as an UNCONDITIONAL system typed
// block in the answer body (§7.30.2 C4a). Before this block, those spans only
// surfaced inside the causal-projection cluster and the body relied on the
// model choosing to repeat them. Typed data only — name / host thread /
// effective cost / E# evidence — the system never ghost-writes prose
// recommendations (v3 system-block pattern, same family as the metric
// snapshot / next-steps blocks). No spans → no block.
func materializeRuntimeTraceSemanticOptimizationBlock(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil || ctx.Mutable == nil {
		return false
	}
	// 复核 R3: every pass recomputes the at-cap skip disclosure from the
	// document's CURRENT state — a stale "表未插入" caveat never ships beside
	// an inserted table, after the doc dropped below the cap, or after a
	// zh↔en language flip (the skip arm below re-appends when still true).
	runtimeTraceSemanticOptimizationSkipCaveatReconcile(doc)
	if answerDocumentHasRuntimeTraceSystemBlockID(doc, "runtime_trace_semantic_optimizations") {
		return false
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	ledger := types.CompileObservationLedger(input)
	set := types.CompileTraceCausalProjectionSet(ledger)
	lang := requestedAnswerDocumentLanguage(ctx)
	zh := runtimeTraceCausalProjectionUseChinese(lang)
	var columns []string
	var rows []types.AnswerBlockItem
	if len(set.Projections) > 1 {
		// CMP-1: the projection cluster renders per-artifact sections with
		// per-artifact E# numbering, so this block rebuilds each artifact's
		// numbering the same way and qualifies the tag with the artifact label
		// ("<basename> E3") — an unqualified E# would be ambiguous across
		// sections.
		for _, projection := range set.Projections {
			if len(projection.SemanticSpans) == 0 {
				continue
			}
			evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
			model := buildRuntimeTraceProjTreeModel(projection, evidence, zh)
			cols, sectionRows := runtimeTraceSemanticOptimizationParts(projection, evidence, model.WindowMS, zh)
			columns = cols
			label := strings.TrimSpace(projection.ArtifactLabel)
			for _, row := range sectionRows {
				if label != "" && len(row.Cells) == 6 && row.Cells[5] != "—" {
					row.Cells[5] = runtimeTraceCausalProjectionMarkdownSafe(label) + " " + row.Cells[5]
				}
				rows = append(rows, row)
			}
		}
	} else {
		var projection types.TraceCausalProjection
		if len(set.Projections) == 1 {
			projection = set.Projections[0]
		}
		if len(projection.SemanticSpans) == 0 {
			return false
		}
		// Rebuild the projection cluster's deterministic evidence numbering so the
		// E# tags in this block match the cluster's evidence index (same ledger,
		// same compile, same model walk — pure and deterministic).
		evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
		model := buildRuntimeTraceProjTreeModel(projection, evidence, zh)
		columns, rows = runtimeTraceSemanticOptimizationParts(projection, evidence, model.WindowMS, zh)
	}
	if len(rows) == 0 {
		return false
	}
	if !reserveRuntimeTraceSemanticOptimizationBlockSlot(doc) {
		// 审计 #58: never evict model content — skip the system table and
		// disclose the skip on the document caveat lane (typed, idempotent).
		runtimeTraceSemanticOptimizationSkipCaveat(doc, zh)
		logging.Warning("[answer_document] runtime trace semantic optimization block skipped: no replaceable slot at the %d-block cap", maxBlocksPerDoc)
		return false
	}
	// PTV6-C ruling C (#73): the grounding clause points at the report's own
	// evidence index (trace line/time coordinates) — never the intermediate
	// trace_query record file.
	title := tracefence.SectionOptimizationZH
	// C8PROSE-1 (§29.164 残余清单收账, 2026-07-20): prose intro — the depth-0
	// semicolon goes full-width; the parenthetical span-class roster keeps its
	// half-width interior comma.
	text := "trace 中的确定性语义优化 span(类校验/JIT编译/着色器编译/运行时编译/纹理上传/GC暂停等,来自 typed semantic_class 通道):每行都是可直接落地的优化点；时长与 E# 证据均可经证据索引定位到 trace 行号区间。"
	if !zh {
		title = tracefence.SectionOptimizationEN
		text = "Deterministic semantic optimization spans found in the trace (class verification / JIT / shader compilation / texture upload / explicit GC pauses, from the typed semantic_class channel): each row is a directly actionable optimization point; durations and E# tags resolve to trace line spans via the evidence index."
	}
	block := types.AnswerBlock{
		ID:      "runtime_trace_semantic_optimizations",
		Kind:    types.BlockTable,
		Title:   title,
		Text:    text,
		Columns: columns,
		Items:   rows,
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimExternalObservation,
		}},
		FacetIDs: []string{"observed_artifact_fact"},
	}
	markRuntimeTraceSystemBlock(&block)
	insertAt := answerDocumentInsertionIndexBeforeRuntimeTraceDetails(doc)
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{})
	copy(doc.Blocks[insertAt+1:], doc.Blocks[insertAt:])
	doc.Blocks[insertAt] = block
	return true
}

// reserveRuntimeTraceSemanticOptimizationBlockSlot enforces the semantic
// mention obligation even when a model-authored document arrives exactly at
// the block cap. The deterministic optimization table is a decision surface,
// so one SYSTEM-authenticated lossless detail/evidence block yields before it
// (their data stays reachable through the projection cluster's other faces).
//
// EVOLUTION RECORD (审计 #58, §29.25 处置委托 + §29.26 待主会话落账,
// 2026-07-10): the former fallback evicted the LAST non-system block with no
// Kind guard — a model-authored caveat/decision/summary could vanish from the
// customer answer with only a log line (系统不可代替 LLM 写用户面板答案 red
// line; disclosure blocks are load-bearing per §29.14/§29.17). Model blocks
// are now NEVER evicted: with no replaceable system slot the system skips its
// own table insertion and the caller discloses the skip through the document
// caveat lane instead of silently deleting model content.
func reserveRuntimeTraceSemanticOptimizationBlockSlot(doc *types.AnswerDocumentV2) bool {
	if doc == nil {
		return false
	}
	if len(doc.Blocks) < maxBlocksPerDoc {
		return true
	}
	removeAt := -1
	for i := len(doc.Blocks) - 1; i > 0; i-- {
		block := doc.Blocks[i]
		if RuntimeTraceSystemBlock(block) &&
			runtimeTraceCausalProjectionDetailBlockID(strings.TrimSpace(block.ID)) {
			removeAt = i
			break
		}
	}
	if removeAt < 0 {
		return false
	}
	removedID := strings.TrimSpace(doc.Blocks[removeAt].ID)
	copy(doc.Blocks[removeAt:], doc.Blocks[removeAt+1:])
	doc.Blocks = doc.Blocks[:len(doc.Blocks)-1]
	logging.Warning("[answer_document] evicted lower-priority block %q to preserve the mandatory semantic optimization surface at the %d-block cap", removedID, maxBlocksPerDoc)
	return true
}

// runtimeTraceSemanticOptimizationSkipCaveatText is the single wording source
// of the at-cap skip disclosure (both language forms; exact strings double as
// the reconcile identity below).
func runtimeTraceSemanticOptimizationSkipCaveatText(zh bool) string {
	// C8PROSE-1 (§29.164 残余清单收账, 2026-07-20): caveat prose — depth-0
	// semicolon full-width. Mint and reconcile share THIS single wording
	// source, so the identity key evolves atomically on both sides.
	if zh {
		return fmt.Sprintf("文档已达 %d 块上限且无可让位的系统明细块:确定性优化点汇总表未插入；语义优化 span(类校验/JIT/着色器编译/Texture upload/GC暂停等)仍完整保留在 trace 因果投影区块中。", maxBlocksPerDoc)
	}
	return fmt.Sprintf("The document is at the %d-block cap with no replaceable system detail block: the Deterministic Optimization Points summary table was not inserted; the semantic optimization spans remain fully available inside the trace causal projection sections.", maxBlocksPerDoc)
}

// runtimeTraceSemanticOptimizationSkipCaveatReconcile removes every existing
// skip disclosure (BOTH language forms — the language-agnostic identity key,
// so a zh↔en language flip never double-mints). 复核 R3 (§29.25 处置委托 +
// §29.26 待主会话落账, 2026-07-10), the §29.24 C15 hedge-marker
// upsert/reconcile precedent: each materialize pass recomputes the disclosure
// from the document's CURRENT state — when a later pass finds headroom (blocks
// trimmed below the cap, or a system detail became evictable) and inserts the
// table, the stale "未插入" caveat is removed instead of shipping beside the
// table it denies.
func runtimeTraceSemanticOptimizationSkipCaveatReconcile(doc *types.AnswerDocumentV2) {
	if doc == nil || len(doc.Caveats) == 0 {
		return
	}
	kept := doc.Caveats[:0]
	for _, caveat := range doc.Caveats {
		if caveat == runtimeTraceSemanticOptimizationSkipCaveatText(true) ||
			caveat == runtimeTraceSemanticOptimizationSkipCaveatText(false) {
			continue
		}
		kept = append(kept, caveat)
	}
	doc.Caveats = kept
}

// runtimeTraceSemanticOptimizationSkipCaveat discloses the at-cap skip on the
// document caveat lane (upsert: reconcile-remove both language forms, then
// append the current language once). The wording points the reader at the
// projection cluster, which still carries every semantic span losslessly; the
// system never deletes model panel content to make room (审计 #58).
func runtimeTraceSemanticOptimizationSkipCaveat(doc *types.AnswerDocumentV2, zh bool) {
	if doc == nil {
		return
	}
	runtimeTraceSemanticOptimizationSkipCaveatReconcile(doc)
	doc.Caveats = append(doc.Caveats, runtimeTraceSemanticOptimizationSkipCaveatText(zh))
}

// --- SUPP-CORE (DISPATCH-IND 批1, 2026-07-14) supplement disclosure --------
//
// runtimeTraceSupplementDisclosurePrefixZH/EN are the reserved single-line
// disclosure prefixes for the post-explore deterministic trace supplement
// (R5 ruling: ONE total line on the document caveat lane, no per-face
// labels). The prefix doubles as the reconcile identity so repeated
// mutation passes upsert exactly one line (both language forms removed
// before the current-language append — semantic-optimization skip caveat
// precedent).
const (
	runtimeTraceSupplementDisclosurePrefixZH = "系统补采:"
	runtimeTraceSupplementDisclosurePrefixEN = "System supplement:"
)

// runtimeTraceSupplementViewZHLabel maps the supplement's CLOSED engine-view
// set (traceSupplementViews — exactly two tokens today) to user-facing zh
// display words. root_cause_rank speaks the established channel word
// (tracefence single source — UXG-1 F2 tripwire: never a hand-copied
// literal). Unmapped tokens pass through verbatim (D4 rule: no claim, never
// a guess).
func runtimeTraceSupplementViewZHLabel(view string) string {
	switch strings.TrimSpace(view) {
	case "root_cause_rank":
		return tracefence.SeatChannelChainZH
	case "critical_blocking_calls":
		return "关键阻塞调用"
	}
	return ""
}

// runtimeTraceSupplementViewList renders the disclosure line's view list. The
// zh face speaks 中文视图名（raw_token） — the D4 combined-form precedent
// (label（token）, tree ⌗ 口径行), keeping the raw token inline for audit
// fidelity while the Chinese word carries the reading; the EN face keeps the
// raw tokens (existing EN token convention).
func runtimeTraceSupplementViewList(views []string, zh bool) string {
	if !zh {
		return strings.Join(views, ", ")
	}
	parts := make([]string, 0, len(views))
	for _, view := range views {
		if label := runtimeTraceSupplementViewZHLabel(view); label != "" && label != view {
			parts = append(parts, label+"（"+view+"）")
			continue
		}
		parts = append(parts, view)
	}
	return strings.Join(parts, "、")
}

// runtimeTraceSupplementDisclosureText renders the single disclosure line
// from the typed supplement meta (never model-authored text — §29.57 typed
// backfill discipline). Three forms (P1 budget fuses, 2026-07-14):
//   - executed: the standard re-run line;
//   - executed + duration_budget_exceeded: the re-run line plus the honest
//     "remaining views not re-run over the duration budget" tail —
//     completed views are kept, the partial skip is disclosed;
//   - window_span_exceeded: nothing ran — the user named this window, so
//     the line says so and points at narrowing the window (never a silent
//     truncation, never a guessed sub-window);
//   - canceled (SUPP-CANCEL, 2026-07-14): the duration budget's context
//     deadline canceled ≥1 view IN-view. Canceled-only (zero recorded
//     views) gets its own sentence; a mixed run keeps the executed line and
//     appends the canceled tail — completed faces recorded, unfinished
//     parts discarded whole.
//
// EVOLUTION RECORD (§29.71 残留3 词面终稿, WF-2 词面批 2026-07-14; supersedes
// the DISPATCH-IND provisional form):
//   - 「装配期确定性补跑」→「成文前确定性补跑」/ EN "deterministic
//     assembly-time re-run" → "deterministic pre-report re-run" — 装配期/
//     assembly-time named an internal pipeline phase; the reader-visible
//     fact is that the re-run happened before this report was written
//     (零内部管线词 discipline).
//   - raw view tokens (root_cause_rank) → zh 视图名（token） via
//     runtimeTraceSupplementViewList (D4 label（token） precedent); EN keeps
//     the raw tokens.
//   - three-form audit: shared skeleton kept (prefix + 补跑/未补跑 + views +
//     window/reason); the span-skip form's 「补跑窗长预算」 vs the partial
//     form's 「时长预算」 name two DIFFERENT budgets and deliberately stay
//     distinct words.
func runtimeTraceSupplementDisclosureText(meta *types.SystemTraceSupplementMeta, zh bool) string {
	if meta == nil || (len(meta.Views) == 0 && meta.SkipReason != types.TraceSupplementReasonWindowSpanExceeded && !meta.CensusLite && len(meta.CanceledViews) == 0) {
		return ""
	}
	// G4-ENGINE (2026-07-20): the windowless D-state fallback lane carries
	// no derived window — every window clause below must speak the honest
	// whole-trace caliber instead of a fabricated 0.000000..0.000000 span.
	windowClause := func() string {
		if meta.WindowlessFallback {
			if zh {
				return "全 trace 无时间窗——本次调查未确定统一分析时间窗"
			}
			return "whole trace, windowless — no consistent analysis window was established"
		}
		if zh {
			return fmt.Sprintf("窗 %.6f..%.6f", meta.WindowStart, meta.WindowEnd)
		}
		return fmt.Sprintf("window %.6f..%.6f", meta.WindowStart, meta.WindowEnd)
	}
	// SUPP-CANCEL (2026-07-14) canceled-only form: the cancellation canceled
	// every attempted view before any complete face was recorded — nothing
	// ran to completion, and the user-named window must hear that honestly
	// (禁裸丢). Partial aggregates were discarded whole by the engine
	// (禁半账), so the sentence claims no re-run. SUPP-HYG P3-D (2026-07-14,
	// ATOMIC zh/en fork): the caller-abort reason (canceled_by_caller) names
	// the caller's cancellation and recommends a plain re-run — it must never
	// blame the duration budget (which did not fire) nor advise narrowing the
	// window.
	if len(meta.Views) == 0 && meta.SkipReason != types.TraceSupplementReasonWindowSpanExceeded && !meta.CensusLite && len(meta.CanceledViews) > 0 {
		if meta.SkipReason == types.TraceSupplementReasonCanceledByCaller {
			if zh {
				refill := "重新运行可补齐该窗结果"
				if meta.WindowlessFallback {
					refill = "重新运行可补齐结果"
				}
				return fmt.Sprintf("%s 未完成成文前确定性补跑——%s在执行中被本次运行的取消信号中止，未采信任何部分结果(%s)；%s",
					runtimeTraceSupplementDisclosurePrefixZH, runtimeTraceSupplementViewList(meta.CanceledViews, true), windowClause(), refill)
			}
			return fmt.Sprintf("%s pre-report re-run incomplete — %s canceled mid-run by this run's cancellation signal; no partial aggregates were kept (%s); re-run to fill it in",
				runtimeTraceSupplementDisclosurePrefixEN, runtimeTraceSupplementViewList(meta.CanceledViews, false), windowClause())
		}
		if zh {
			refill := "缩小时间窗后可补齐该窗结果"
			if meta.WindowlessFallback {
				// G4-ENGINE: there is no window to narrow on the windowless
				// lane — the honest advice is to provide one.
				refill = "提供明确时间窗后可补齐结果"
			}
			return fmt.Sprintf("%s 未完成成文前确定性补跑——%s超 %g 秒时长预算在执行中被取消，未采信任何部分结果(%s)；%s",
				runtimeTraceSupplementDisclosurePrefixZH, runtimeTraceSupplementViewList(meta.CanceledViews, true), meta.DurationBudgetS, windowClause(), refill)
		}
		refill := "narrow the time window to fill it in"
		if meta.WindowlessFallback {
			refill = "provide an explicit time window to fill it in"
		}
		return fmt.Sprintf("%s pre-report re-run incomplete — %s canceled mid-run over the %gs duration budget; no partial aggregates were kept (%s); %s",
			runtimeTraceSupplementDisclosurePrefixEN, runtimeTraceSupplementViewList(meta.CanceledViews, false), meta.DurationBudgetS, windowClause(), refill)
	}
	// SA-F2 / C-lite (DISPATCH-IND 批4 + 修复轮 件2, 2026-07-14): the
	// lightweight census arm's disclosure. Three shapes: lite-ONLY (nothing
	// windowed ran — standalone sentence below), windowed views + lite
	// adjunct (tail clause on the windowed line), span-skip + lite (tail
	// clause on the span line). Every form states exactly what ran — one
	// whole-trace literal search minting the generator census — never a
	// window/target claim for the lite pass itself.
	liteTail := ""
	if meta.CensusLite {
		pattern := strings.TrimSpace(meta.CensusLitePattern)
		if len(meta.Views) == 0 && meta.SkipReason != types.TraceSupplementReasonWindowSpanExceeded {
			// Lite-only sentence is LANE-NEUTRAL by design (修复轮 件2): the
			// arm fires on derivation failures AND on the families_present /
			// execution_failed lanes, so it claims only what ran — never why
			// the windowed re-run stayed absent.
			if zh {
				return fmt.Sprintf("%s 成文前对全 trace 补跑 VSync/帧节拍发生器轻量普查(event_search·%s, 无时间窗)",
					runtimeTraceSupplementDisclosurePrefixZH, pattern)
			}
			return fmt.Sprintf("%s ran a lightweight whole-trace VSync/frame-pacing generator census pre-report (event_search, %q, windowless)",
				runtimeTraceSupplementDisclosurePrefixEN, pattern)
		}
		if zh {
			liteTail = fmt.Sprintf(";另对全 trace 补跑 VSync/帧节拍发生器轻量普查(event_search·%s)", pattern)
		} else {
			liteTail = fmt.Sprintf("; also ran a lightweight whole-trace VSync/frame-pacing generator census (event_search, %q)", pattern)
		}
	}
	target := strings.TrimSpace(meta.TargetThread)
	switch {
	case target != "" && meta.TargetPID > 0:
		// name-tid label form; labels that already carry the "-<tid>" tail
		// (".ugc.aweme.lite-17267") are kept verbatim, never doubled.
		if !strings.HasSuffix(target, fmt.Sprintf("-%d", meta.TargetPID)) {
			target = fmt.Sprintf("%s-%d", target, meta.TargetPID)
		}
	case target == "" && meta.TargetPID > 0:
		target = fmt.Sprintf("pid %d", meta.TargetPID)
	}
	if meta.SkipReason == types.TraceSupplementReasonWindowSpanExceeded {
		span := meta.WindowEnd - meta.WindowStart
		if zh {
			return fmt.Sprintf("%s 未补跑 %s——窗 %.6f..%.6f 跨度 %.3f 秒超出补跑窗长预算 %g 秒；缩小时间窗后可补齐该窗结果",
				runtimeTraceSupplementDisclosurePrefixZH, runtimeTraceSupplementViewList(meta.SkippedViews, true), meta.WindowStart, meta.WindowEnd, span, meta.WindowBudgetS) + liteTail
		}
		return fmt.Sprintf("%s %s not re-run — window %.6f..%.6f spans %.3fs, over the %gs span budget; narrow the time window to fill it in",
			runtimeTraceSupplementDisclosurePrefixEN, runtimeTraceSupplementViewList(meta.SkippedViews, false), meta.WindowStart, meta.WindowEnd, span, meta.WindowBudgetS) + liteTail
	}
	if zh {
		line := fmt.Sprintf("%s 成文前确定性补跑 %s(%s, 目标 %s)",
			runtimeTraceSupplementDisclosurePrefixZH, runtimeTraceSupplementViewList(meta.Views, true), windowClause(), target)
		if meta.SkipReason == types.TraceSupplementReasonDurationBudgetExceeded && len(meta.SkippedViews) > 0 {
			line += "；超时长预算未补跑 " + runtimeTraceSupplementViewList(meta.SkippedViews, true)
		}
		if len(meta.CanceledViews) > 0 {
			// SUPP-HYG P3-D (ATOMIC zh/en fork): caller abort vs duration
			// budget — the tail must name the cancellation that actually
			// happened.
			if meta.SkipReason == types.TraceSupplementReasonCanceledByCaller {
				line += fmt.Sprintf("；其中 %s 被本次运行的取消信号中止，仅已完成的完整结果被记录，未完成部分整弃",
					runtimeTraceSupplementViewList(meta.CanceledViews, true))
			} else {
				line += fmt.Sprintf("；其中 %s 在 %g 秒时长预算处被取消，仅已完成的完整结果被记录，未完成部分整弃",
					runtimeTraceSupplementViewList(meta.CanceledViews, true), meta.DurationBudgetS)
			}
		}
		return line + liteTail
	}
	line := fmt.Sprintf("%s deterministic pre-report re-run of %s (%s, target %s)",
		runtimeTraceSupplementDisclosurePrefixEN, runtimeTraceSupplementViewList(meta.Views, false), windowClause(), target)
	if meta.SkipReason == types.TraceSupplementReasonDurationBudgetExceeded && len(meta.SkippedViews) > 0 {
		line += "; not re-run over the duration budget: " + runtimeTraceSupplementViewList(meta.SkippedViews, false)
	}
	if len(meta.CanceledViews) > 0 {
		if meta.SkipReason == types.TraceSupplementReasonCanceledByCaller {
			line += fmt.Sprintf("; %s canceled by this run's cancellation signal — only fully-completed results were recorded, unfinished parts were discarded whole",
				runtimeTraceSupplementViewList(meta.CanceledViews, false))
		} else {
			line += fmt.Sprintf("; %s canceled at the %gs duration budget — only fully-completed results were recorded, unfinished parts were discarded whole",
				runtimeTraceSupplementViewList(meta.CanceledViews, false), meta.DurationBudgetS)
		}
	}
	return line + liteTail
}

// materializeRuntimeTraceSupplementDisclosureCaveat upserts the supplement
// disclosure onto the document caveat lane when the supplement executed this
// run OR was budget-skipped on the user-named window (window_span_exceeded —
// P1 honest disclosure). The silent fail-open family (no target / no window
// / inconsistent windows / disabled / cold budget) leaves the document
// byte-identical. Precise trigger: typed meta present with a non-empty
// executed view list or the span-skip reason.
func materializeRuntimeTraceSupplementDisclosureCaveat(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil || ctx.Mutable == nil {
		return false
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || (len(meta.Views) == 0 && meta.SkipReason != types.TraceSupplementReasonWindowSpanExceeded && !meta.CensusLite && len(meta.CanceledViews) == 0) {
		return false
	}
	kept := doc.Caveats[:0]
	for _, caveat := range doc.Caveats {
		trimmed := strings.TrimSpace(caveat)
		if strings.HasPrefix(trimmed, runtimeTraceSupplementDisclosurePrefixZH) ||
			strings.HasPrefix(trimmed, runtimeTraceSupplementDisclosurePrefixEN) {
			continue
		}
		kept = append(kept, caveat)
	}
	doc.Caveats = kept
	zh := runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx))
	doc.Caveats = append(doc.Caveats, runtimeTraceSupplementDisclosureText(meta, zh))
	return true
}

// runtimeTraceSemanticOptimizationParts builds the ZH/EN-symmetric table rows
// for the deterministic optimization block: span name, typed semantic class,
// host thread, effective cost (EffectiveImpactMS with the display-impact
// fallback), and the shared E# evidence tag. CitationRef=-1 on every
// system-injected row (red-line invariant).
// runtimeTraceSemanticSpanInlineLocator renders a span's own trace locator
// (time window first, line span fallback) for surfaces that must not mint a
// fresh E# (修复轮 D1). "" when the span carries neither coordinate.
func runtimeTraceSemanticSpanInlineLocator(span types.TraceCausalProjectionNode, zh bool) string {
	if types.TraceCausalProjectionWindowPresent(span.StartTs, span.EndTs) {
		return fmt.Sprintf("[%.3f~%.3fs]", span.StartTs, span.EndTs)
	}
	if span.LineStart > 0 && span.LineEnd >= span.LineStart {
		if zh {
			return fmt.Sprintf("行 %d–%d", span.LineStart, span.LineEnd)
		}
		return fmt.Sprintf("lines %d–%d", span.LineStart, span.LineEnd)
	}
	return ""
}

func runtimeTraceSemanticOptimizationParts(projection types.TraceCausalProjection, evidence *runtimeTraceCausalProjectionEvidenceIndex, windowMS float64, zh bool) ([]string, []types.AnswerBlockItem) {
	// 占窗% (RANK-U Stage 2 rider, §29.61 d, caliber ruling ⑤ 2026-07-13):
	// the C4 table gains a window-share column — basis = THE SAME published
	// effective-cost value the ms cell prints (one field, one value source;
	// the member_sum basis was rejected: it would split the row across two
	// calibers) divided by the analysis-window length. Semantic-class rows
	// only (typed SemanticClass gate — the whole table is semantic, but
	// member/fold subordinate rows and class-less spans render "—"), and a
	// merged row straddling MULTIPLE query windows renders "—" (§21.1 CWD-2 ①:
	// no single window base — never a cross-window numerator over one anchor
	// denominator). Legality: semantic eff is pure wall clock (union /
	// intersection calibers, zero supply-discount component), so the §29.27
	// discounted-value percentage ban does not bind here.
	columns := []string{tracefence.ActionWordZH, "类别", "宿主线程", "有效成本", "占窗%", "证据"}
	if !zh {
		columns = []string{"Optimization point", "Class", "Host thread", "Effective cost", "% of window", "Evidence"}
	}
	dash := "—"
	windowShare := func(span types.TraceCausalProjectionNode, cost float64) string {
		if strings.TrimSpace(span.SemanticClass) == "" || windowMS <= 0 || cost <= 0 ||
			runtimeTraceProjMultiWindowMergedRow(span) {
			return dash
		}
		return fmt.Sprintf("%.1f%%", cost/windowMS*100)
	}
	var rows []types.AnswerBlockItem
	for _, span := range projection.SemanticSpans {
		name := strings.TrimSpace(span.SpanName)
		if name == "" {
			name = strings.TrimSpace(span.Object)
		}
		if name == "" {
			continue
		}
		classToken := strings.TrimSpace(span.SemanticClass)
		class := classToken
		if classToken == "" {
			class = dash
		} else if zh {
			class = runtimeTraceCausalProjectionDisplayCauseName(classToken, true)
		}
		host := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(span.Subject, zh))
		if host == "" {
			host = dash
		}
		cost := span.EffectiveImpactMS
		if cost <= 0 {
			cost = runtimeTraceProjNodeDisplayImpact(span)
		}
		costCell := dash
		if cost > 0 {
			costCell = fmt.Sprintf("%.3fms", cost)
		}
		// 修复轮 D1 (冷读 donghu r2 「证据 E39」悬空, 2026-07-12): an E# renders
		// ONLY when the model walk already allocated it (the tag then exists in
		// the printed evidence index). A span the walk never rendered falls
		// back to its own inline trace locator — groundable directly, never a
		// pointer past the index's last row.
		tag := ""
		if evidence.has(span) {
			tag = runtimeTraceProjEvidenceTag(span, evidence, zh)
		} else {
			tag = runtimeTraceSemanticSpanInlineLocator(span, zh)
		}
		if tag == "" {
			tag = dash
		}
		// RCM-2 D5 (C4 块 family 分组, §24.10): a family contender renders as
		// ONE header row (类型词 ×N + 合计 cost — the participation magnitude
		// the pre-RCM table hid behind one member's value) followed by capped
		// member rows and a COUNTED fold row (计数折叠, never a silent cut).
		if runtimeTraceProjFamilyRow(span) {
			famName, famCost := runtimeTraceProjSemanticCellParts(span, runtimeTraceProjFamilyPublishedMS(span), zh)
			rows = append(rows, types.AnswerBlockItem{
				Cells: []string{
					runtimeTraceCausalProjectionMarkdownSafe(famName),
					class,
					runtimeTraceCausalProjectionMarkdownSafe(host),
					famCost,
					windowShare(span, runtimeTraceProjFamilyPublishedMS(span)),
					tag,
				},
				CitationRef: -1,
			})
			// SPANTOP-1 件5 (§29.131 归口 XLANE-2 备案「优化点表成员重复列示」,
			// user ruling 2026-07-18): the member rows left this table — the
			// detail stanza holds the complete inventory and the tree face adds
			// the top-3 constituent block WHEN its typed gates pass (两面互指,
			// 不再第三面抄成员). ONE counted subordinate pointer row replaces
			// them (roster 折叠必带计数披露 red line: the count and the detail
			// destination stay explicit; the 列N clause keeps the honest-excerpt
			// disclosure whenever the detail roster is itself bounded below the
			// member count). The row makes NO claim about the tree face: the
			// tree's fail-open lanes (periodic / gated-composite / dual-caliber
			// imbalance) legally render ZERO member sub-rows, so a 「树面列前K项」
			// clause here was a false pointer on exactly those forms (SPANTOP-1
			// 双复核 C4, 2026-07-18 — the clause was removed; the tree top-3
			// block is self-evident when present).
			// EVOLUTION RECORD (P2a rider 件4 §29.58.2 F4 → 用户裁定 2026-07-14
			// → SPANTOP-1): ↳ subordinate connector retained; the per-member
			// cells retired with the batch.
			var pointerCell string
			if len(span.FamilyMemberRoster) == span.FamilyMemberCount {
				if zh {
					pointerCell = fmt.Sprintf("%s 成员共%d项:全清单见因果投影明细(本表不另列)", tracefence.GlyphSubordinate, span.FamilyMemberCount)
				} else {
					pointerCell = fmt.Sprintf("%s %d members: full inventory in the causal projection detail (not re-listed here)", tracefence.GlyphSubordinate, span.FamilyMemberCount)
				}
			} else {
				if zh {
					pointerCell = fmt.Sprintf("%s 成员共%d项:因果投影明细列%d项(本表不另列)", tracefence.GlyphSubordinate, span.FamilyMemberCount, len(span.FamilyMemberRoster))
				} else {
					pointerCell = fmt.Sprintf("%s %d members: %d listed in the causal projection detail (not re-listed here)", tracefence.GlyphSubordinate, span.FamilyMemberCount, len(span.FamilyMemberRoster))
				}
			}
			rows = append(rows, types.AnswerBlockItem{
				Cells:       []string{runtimeTraceCausalProjectionMarkdownSafe(pointerCell), dash, dash, dash, dash, dash},
				CitationRef: -1,
			})
			continue
		}
		rows = append(rows, types.AnswerBlockItem{
			Cells: []string{
				runtimeTraceCausalProjectionMarkdownSafe(name),
				class,
				runtimeTraceCausalProjectionMarkdownSafe(host),
				costCell,
				windowShare(span, cost),
				tag,
			},
			CitationRef: -1,
		})
	}
	return columns, rows
}

func materializeRuntimeTraceMetricSnapshotBlock(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil || ctx.Mutable == nil {
		return false
	}
	if len(doc.Blocks) >= maxBlocksPerDoc {
		logging.Warning("[answer_document] runtime trace metric snapshot block skipped: document already at the %d-block cap", maxBlocksPerDoc)
		return false
	}
	if answerDocumentHasRuntimeTraceSystemBlockID(doc, "runtime_trace_metric_snapshot") {
		return false
	}
	items := runtimeTraceMetricSnapshotItems(doc, ctx)
	if len(items) == 0 {
		return false
	}
	title := "Trace 指标快照"
	if !runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx)) {
		title = "Trace Metric Snapshot"
	}
	block := types.AnswerBlock{
		ID:    "runtime_trace_metric_snapshot",
		Kind:  types.BlockBulletList,
		Title: title,
		Items: items,
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimExternalObservation,
		}},
		FacetIDs: []string{"observed_artifact_fact"},
	}
	markRuntimeTraceSystemBlock(&block)
	insertAt := answerDocumentInsertionIndexBeforeRuntimeTraceDetails(doc)
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{})
	copy(doc.Blocks[insertAt+1:], doc.Blocks[insertAt:])
	doc.Blocks[insertAt] = block
	return true
}

func runtimeTraceMetricSnapshotItems(doc *types.AnswerDocumentV2, ctx *types.BusContext) []types.AnswerBlockItem {
	if ctx == nil {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, 64))
	if len(ledger.Records) == 0 {
		return nil
	}
	visible := answerDocumentVisibleSurfaceForRuntimeTrace(doc)
	zh := runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx))
	snapCtx := newRuntimeTraceMetricSnapshotContext(ledger, runtimeTraceProjUserFocusFromBusContext(ctx), zh)
	seen := make(map[string]bool)
	type snapshotCandidate struct {
		record   types.ObservationRecord
		raw      string
		tier     int
		projIdx  int
		winStart float64
		winEnd   float64
		windowed bool
	}
	var candidates []snapshotCandidate
	hasChainCandidate := false
	for _, record := range ledger.Records {
		// The raw key=value form stays the coverage/dedupe key (typed pairs), the
		// user-facing text is the localized humanized form (§7.30 裁定5/S2). The
		// raw string itself is not shown; it stays derivable from the observation
		// record, which keeps full audit fidelity.
		raw := runtimeTraceMetricSnapshotFromObservationRecord(record)
		if raw == "" {
			continue
		}
		if runtimeTraceMetricSnapshotCoveredByAnswer(visible, record, raw) {
			continue
		}
		if seen[raw] {
			continue
		}
		seen[raw] = true
		tier, projIdx := snapCtx.candidateTier(record)
		if tier == runtimeTraceMetricSnapshotTierChain {
			hasChainCandidate = true
		}
		candidate := snapshotCandidate{record: record, raw: raw, tier: tier, projIdx: projIdx}
		// PTV5 Q3 (#68 用户裁定 2026-07-05, NEW-8 display 用途): each record's
		// own typed selected_window — the single strict parser — keys the
		// per-window grouping below. Display-only; anchors untouched.
		if ws, we, wok := types.TraceCausalProjectionSelectedWindowNote(record.RichNotes); wok {
			candidate.winStart, candidate.winEnd, candidate.windowed = ws, we, true
		}
		candidates = append(candidates, candidate)
	}
	// CMP-4a eligibility first (one face for the gate AND the render, 复核 Med
	// 2026-07-06): when ANY on-chain candidate exists, rest-tier candidates
	// never enter the snapshot at all (the customer render burned both slots
	// on usbDelayTimer/OS_DfxWatchdog while bindApplication's chain threads
	// carried full metric sets).
	eligible := candidates[:0:0]
	for _, candidate := range candidates {
		if hasChainCandidate && candidate.tier == runtimeTraceMetricSnapshotTierRest {
			continue
		}
		eligible = append(eligible, candidate)
	}
	// PTV5 Q3 + 复核 Med: the grouping keys on the ELIGIBLE set — the SAME set
	// selection and rendering read, so the two faces can never diverge. ≥2
	// distinct windows (±1ms dedupe) activates the per-window floor: every
	// window keeps at least ONE row (budget grows to the window count when it
	// exceeds the legacy 2 slots). PTV8-RCR-B (UXA 域D #16): the tree header's
	// "(按查询窗分组)" parenthetical is retired, but the header still points at
	// the snapshot for per-window metrics — the floor keeps that pointer true.
	distinctWindows := runtimeTraceMetricSnapshotDistinctWindows(func(yield func(float64, float64)) {
		for _, c := range eligible {
			if c.windowed {
				yield(c.winStart, c.winEnd)
			}
		}
	})
	multiWindow := distinctWindows >= 2
	// CMP-4a candidate priority: chain tier first, then analyzer-entity hits,
	// then the rest; stable sort keeps ledger order inside each tier. PTV5 Q3:
	// the tier stays the PRIMARY key (CMP-4a ruling untouched — it decides WHO
	// gets a slot); within a tier, multi-window snapshots order by ascending
	// window start (window-less rows last).
	sort.SliceStable(eligible, func(i, j int) bool {
		if eligible[i].tier != eligible[j].tier {
			return eligible[i].tier < eligible[j].tier
		}
		if multiWindow {
			wi, wj := eligible[i], eligible[j]
			if wi.windowed != wj.windowed {
				return wi.windowed
			}
			if wi.windowed && wj.windowed && wi.winStart != wj.winStart {
				return wi.winStart < wj.winStart
			}
		}
		return false
	})
	budget := 2
	if distinctWindows > budget {
		budget = distinctWindows
	}
	windowKey := func(c snapshotCandidate) string {
		return fmt.Sprintf("%.3f\x00%.3f", c.winStart, c.winEnd)
	}
	selected := make([]bool, len(eligible))
	picked := 0
	if multiWindow {
		// Pass 1 (per-window floor): scanning in tier-major order, the first
		// candidate of each window group is that window's tier-best row.
		groupPicked := map[string]bool{}
		for i, candidate := range eligible {
			if !candidate.windowed || groupPicked[windowKey(candidate)] {
				continue
			}
			groupPicked[windowKey(candidate)] = true
			selected[i] = true
			picked++
		}
	}
	// Pass 2: fill the remaining budget in CMP-4a order (window-less rows
	// compete here only — their absence of a window claim earns no floor).
	for i := range eligible {
		if picked >= budget {
			break
		}
		if selected[i] {
			continue
		}
		selected[i] = true
		picked++
	}
	// Render: multi-window snapshots read window-major (the grouped reading
	// the tree header points at); single-window snapshots keep the legacy
	// tier order byte-identical.
	order := make([]int, 0, picked)
	for i := range eligible {
		if selected[i] {
			order = append(order, i)
		}
	}
	if multiWindow {
		sort.SliceStable(order, func(a, b int) bool {
			ci, cj := eligible[order[a]], eligible[order[b]]
			if ci.windowed != cj.windowed {
				return ci.windowed
			}
			if ci.windowed && cj.windowed && ci.winStart != cj.winStart {
				return ci.winStart < cj.winStart
			}
			return false
		})
	}
	var out []types.AnswerBlockItem
	for _, idx := range order {
		candidate := eligible[idx]
		text := runtimeTraceMetricSnapshotDisplayText(candidate.record, zh)
		if text == "" {
			text = candidate.raw
		}
		text += snapCtx.spanMismatchNote(candidate.record, candidate.projIdx, zh)
		// PTV8-RCR-B (UXA 域C #9 / 域D #27, 2026-07-08). EVOLUTION RECORD: the
		// view token was bare on the label face — the zh face reuses the
		// registered 状态切换 display word with the token in parens (§22.2.1
		// 兜底同构); EN keeps the raw token.
		churn := "state_churn"
		if zh {
			churn = "状态切换(state_churn)"
		}
		label := strings.TrimSpace(candidate.record.Subject)
		if label == "" {
			label = churn
		} else {
			label += " " + churn
		}
		if prefix := snapCtx.artifactPrefix(candidate.projIdx); prefix != "" {
			label = prefix + " · " + label
		}
		// PTV5 Q3: the window group label leads the row (grouped reading);
		// window-less rows stay unprefixed (their absence of a window claim is
		// itself the honest state — 禁猜).
		if multiWindow && candidate.windowed {
			if zh {
				label = fmt.Sprintf("查询窗 %.3f~%.3fs · ", candidate.winStart, candidate.winEnd) + label
			} else {
				label = fmt.Sprintf("query window %.3f~%.3fs · ", candidate.winStart, candidate.winEnd) + label
			}
		}
		out = append(out, types.AnswerBlockItem{
			ID:          fmt.Sprintf("runtime_trace_metric_snapshot_%d", len(out)+1),
			Label:       label,
			Text:        text,
			CitationRef: -1,
		})
	}
	return out
}

// runtimeTraceMetricSnapshotDistinctWindows counts the DISTINCT windows the
// callback yields (±1ms per endpoint — the SAME exported tolerance authority
// as the F-2 same-window verdict, 复核 Low 2026-07-06: no re-minted literal).
// PTV5 Q3 display gate only.
func runtimeTraceMetricSnapshotDistinctWindows(iter func(yield func(float64, float64))) int {
	type win struct{ s, e float64 }
	var seenWins []win
	iter(func(s, e float64) {
		for _, w := range seenWins {
			if math.Abs(w.s-s) <= types.TraceCausalProjectionSameWindowToleranceS &&
				math.Abs(w.e-e) <= types.TraceCausalProjectionSameWindowToleranceS {
				return
			}
		}
		seenWins = append(seenWins, win{s: s, e: e})
	})
	return len(seenWins)
}

// Snapshot candidate tiers (CMP-4a, customer compare audit 2026-07-03 §7):
// precise typed lanes only — chain membership is canonical-subject equality
// against the compiled projection's WakeupPath/TreeRows; the entity tier reuses
// the R2/RF1 verbatim name/pid comparison. Tier values are the sort key.
const (
	runtimeTraceMetricSnapshotTierChain  = 0
	runtimeTraceMetricSnapshotTierEntity = 1
	runtimeTraceMetricSnapshotTierRest   = 2
)

// runtimeTraceMetricSnapshotContext carries the per-artifact projection context
// the CMP-4 snapshot selector reads: one chain-thread set per compiled
// projection (canonical WakeupPath/TreeRows subjects), the per-artifact
// projection window for the span-mismatch annotation, and the artifact label
// for the multi-artifact row prefix. Display-side selection/annotation only —
// it never feeds a hard gate.
type runtimeTraceMetricSnapshotContext struct {
	projections []types.TraceCausalProjection
	chainSets   []map[string]bool
	focus       runtimeTraceProjUserFocus
	multi       bool
}

func newRuntimeTraceMetricSnapshotContext(ledger types.ObservationLedger, focus runtimeTraceProjUserFocus, zh bool) runtimeTraceMetricSnapshotContext {
	set := types.CompileTraceCausalProjectionSet(ledger)
	out := runtimeTraceMetricSnapshotContext{
		projections: set.Projections,
		chainSets:   make([]map[string]bool, len(set.Projections)),
		focus:       focus,
		multi:       len(set.Projections) > 1,
	}
	for i, projection := range set.Projections {
		chain := map[string]bool{}
		for _, subject := range runtimeTraceCausalProjectionCleanPath(projection.WakeupPath) {
			if runtimeTraceCausalProjectionKnownSubject(subject) {
				chain[runtimeTraceCausalProjectionCanonicalNode(subject)] = true
			}
		}
		model := buildRuntimeTraceProjTreeModel(projection, nil, zh)
		for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows} {
			for _, row := range rows {
				subject := strings.TrimSpace(row.Node.Subject)
				if subject == "" || !runtimeTraceCausalProjectionKnownSubject(subject) {
					continue
				}
				chain[runtimeTraceCausalProjectionCanonicalNode(subject)] = true
			}
		}
		out.chainSets[i] = chain
	}
	return out
}

// projectionIndexFor attributes one record to a compiled projection via the
// typed artifact-identity lanes (-1 = no attribution). Identity-less records
// belong to the sole projection of a single-projection compile — exactly the
// records that compiled into it.
func (c runtimeTraceMetricSnapshotContext) projectionIndexFor(record types.ObservationRecord) int {
	for i := range c.projections {
		if types.TraceCausalProjectionRecordMatchesArtifact(record, c.projections[i]) {
			return i
		}
	}
	if len(c.projections) == 1 && types.TraceCausalProjectionRecordArtifactIdentity(record) == "" {
		return 0
	}
	return -1
}

func (c runtimeTraceMetricSnapshotContext) candidateTier(record types.ObservationRecord) (int, int) {
	projIdx := c.projectionIndexFor(record)
	subject := strings.TrimSpace(record.Subject)
	if projIdx >= 0 && subject != "" && c.chainSets[projIdx][runtimeTraceCausalProjectionCanonicalNode(subject)] {
		return runtimeTraceMetricSnapshotTierChain, projIdx
	}
	if subject != "" && runtimeTraceProjTargetMatchesUserEntities(subject, c.focus.Entities) {
		return runtimeTraceMetricSnapshotTierEntity, projIdx
	}
	return runtimeTraceMetricSnapshotTierRest, projIdx
}

// spanMismatchNote implements CMP-4b: when the snapshot thread's own observed
// state span exceeds TWICE the attributed projection window (both values
// present; exact float comparison, no tolerance band), the row says so —
// a 24.4s-sleep watchdog next to a 1.2s analysis window read as if it
// described the problem window. No projection window → no annotation.
func (c runtimeTraceMetricSnapshotContext) spanMismatchNote(record types.ObservationRecord, projIdx int, zh bool) string {
	if projIdx < 0 || projIdx >= len(c.projections) {
		return ""
	}
	windowMS := c.projections[projIdx].WindowDurationMS()
	if windowMS <= 0 {
		return ""
	}
	totalMS := runtimeTraceMetricSnapshotObservedSpanMS(record)
	if totalMS <= 0 || totalMS <= windowMS*2 {
		return ""
	}
	// CR-2 组③ P7 (F5-1 word-face family, 2026-07-12): the compared magnitude
	// is the thread's OBSERVED STATE TOTAL, not a data-coverage span.
	if zh {
		return fmt.Sprintf("(该线程观测状态合计 %.1fs,远超分析窗,仅供背景参考)", totalMS/1000)
	}
	return fmt.Sprintf(" (thread observed state total %.1fs, far beyond the analysis window — background reference only)", totalMS/1000)
}

// runtimeTraceMetricSnapshotObservedSpanMS returns the thread's own observed
// state span for the CMP-4b comparison: the typed "total" metric when present,
// else the sum of the five per-state totals the snapshot already requires.
func runtimeTraceMetricSnapshotObservedSpanMS(record types.ObservationRecord) float64 {
	values := runtimeTraceMetricSnapshotValues(record)
	if values == nil {
		return 0
	}
	if total := runtimeTraceMetricFloat(values[types.TraceNoteKeyTotal]); total > 0 {
		return total
	}
	sum := 0.0
	for _, key := range []string{types.TraceNoteKeyRunning, types.TraceNoteKeyRunnable, types.TraceNoteKeySleep, types.TraceNoteKeyDState, types.TraceNoteKeyIOWait} {
		sum += runtimeTraceMetricFloat(values[key])
	}
	return sum
}

// artifactPrefix returns the per-artifact label prefix for multi-artifact
// ledgers (CMP-4c): snapshot rows carry the same artifact basename the
// projection sections are titled with. Single-artifact renders stay unprefixed
// (byte-identity with the legacy label).
func (c runtimeTraceMetricSnapshotContext) artifactPrefix(projIdx int) string {
	if !c.multi || projIdx < 0 || projIdx >= len(c.projections) {
		return ""
	}
	label := strings.TrimSpace(c.projections[projIdx].ArtifactLabel)
	if label == "" {
		label = strings.TrimSpace(c.projections[projIdx].ArtifactPath)
	}
	return label
}

func runtimeTraceMetricSnapshotCoveredByAnswer(visible string, record types.ObservationRecord, snapshot string) bool {
	visible = strings.TrimSpace(visible)
	snapshot = strings.TrimSpace(snapshot)
	if visible == "" || snapshot == "" {
		return false
	}
	if strings.Contains(visible, snapshot) {
		return true
	}
	visibleLower := strings.ToLower(visible)
	subject := strings.ToLower(strings.TrimSpace(record.Subject))
	if subject != "" && !strings.Contains(visibleLower, subject) {
		return false
	}
	pairs := runtimeTraceMetricSnapshotPairs(snapshot)
	if len(pairs) == 0 {
		return false
	}
	for key, value := range pairs {
		if !runtimeTraceMetricPairCoveredByAnswer(visibleLower, key, value) {
			return false
		}
	}
	return true
}

func runtimeTraceMetricSnapshotPairs(snapshot string) map[string]string {
	out := map[string]string{}
	for _, token := range strings.FieldsFunc(snapshot, runtimeTraceMetricSummaryTokenSeparator) {
		key, value, ok := strings.Cut(strings.TrimSpace(token), "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.ToLower(strings.Trim(strings.TrimSpace(value), `"'()[]{}<>`))
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func runtimeTraceMetricPairCoveredByAnswer(visibleLower, key, value string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.ToLower(strings.TrimSpace(value))
	if key == "" || value == "" || !strings.Contains(visibleLower, key) {
		return false
	}
	for _, variant := range runtimeTraceMetricValueVariants(value) {
		if variant != "" && strings.Contains(visibleLower, variant) {
			return true
		}
	}
	return false
}

func runtimeTraceMetricValueVariants(value string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	add(value)
	base := value
	hadMS := false
	if strings.HasSuffix(base, "ms") {
		base = strings.TrimSpace(strings.TrimSuffix(base, "ms"))
		hadMS = true
		add(base)
	}
	if f, err := strconv.ParseFloat(base, 64); err == nil {
		for _, v := range []string{
			strconv.FormatFloat(f, 'f', 3, 64),
			strconv.FormatFloat(f, 'f', 2, 64),
			strconv.FormatFloat(f, 'f', 1, 64),
			strconv.FormatFloat(f, 'f', 0, 64),
			strconv.FormatFloat(f, 'g', -1, 64),
		} {
			add(v)
			if hadMS || strings.HasSuffix(value, "ms") {
				add(v + "ms")
			}
		}
	}
	return out
}

// runtimeTraceMetricSnapshotValues collects the complete typed state-churn
// metric set for one record (rich notes first, summary tokens as fallback).
// nil when the record is not a positive-impact deterministic state_churn row or
// any required metric is missing.
func runtimeTraceMetricSnapshotValues(record types.ObservationRecord) map[string]string {
	if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact {
		return nil
	}
	if !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
		return nil
	}
	if !runtimeTraceStateChurnHasPositiveImpact(record) {
		return nil
	}
	required := runtimeTraceMetricSnapshotRequiredKeys
	values := make(map[string]string, len(required)+2)
	keys := append([]string{types.TraceNoteKeyDominantState, types.TraceNoteKeyTotal}, required...)
	for _, key := range keys {
		if value := runtimeTraceObservationRichNoteValue(record.RichNotes, key); value != "" {
			values[key] = value
		}
	}
	runtimeTraceMergeSummaryMetricTokens(values, record.Summary, keys)
	for _, key := range required {
		if values[key] == "" {
			return nil
		}
	}
	return values
}

var runtimeTraceMetricSnapshotRequiredKeys = []string{
	types.TraceNoteKeyRunning,
	types.TraceNoteKeyRunnable,
	types.TraceNoteKeySleep,
	types.TraceNoteKeyDState,
	types.TraceNoteKeyIOWait,
	types.TraceNoteKeyFragments,
	types.TraceNoteKeySwitches,
	types.TraceNoteKeyMaxSegment,
	types.TraceNoteKeyP95Segment,
}

// runtimeTraceMetricSnapshotFromObservationRecord renders the RAW typed
// key=value snapshot line. It is no longer user-facing: it stays the precise
// coverage/dedupe key (runtimeTraceMetricSnapshotCoveredByAnswer parses these
// exact pairs) while runtimeTraceMetricSnapshotDisplayText carries the
// localized presentation (§7.30 S2).
func runtimeTraceMetricSnapshotFromObservationRecord(record types.ObservationRecord) string {
	values := runtimeTraceMetricSnapshotValues(record)
	if values == nil {
		return ""
	}
	parts := make([]string, 0, len(runtimeTraceMetricSnapshotRequiredKeys)+1)
	if values[types.TraceNoteKeyDominantState] != "" {
		parts = append(parts, types.TraceNoteKeyDominantState+"="+values[types.TraceNoteKeyDominantState])
	}
	for _, key := range []string{types.TraceNoteKeyRunning, types.TraceNoteKeyRunnable, types.TraceNoteKeySleep, types.TraceNoteKeyDState, types.TraceNoteKeyIOWait} {
		parts = append(parts, key+"="+runtimeTraceMetricWithMS(values[key]))
	}
	for _, key := range []string{types.TraceNoteKeyFragments, types.TraceNoteKeySwitches} {
		parts = append(parts, key+"="+values[key])
	}
	for _, key := range []string{types.TraceNoteKeyMaxSegment, types.TraceNoteKeyP95Segment} {
		parts = append(parts, key+"="+runtimeTraceMetricWithMS(values[key]))
	}
	return strings.Join(parts, "; ")
}

// runtimeTraceMetricSnapshotDisplayText is the humanized, localized snapshot
// line (§7.30 裁定5/S2): plain-language metric labels, per-state share of the
// state total when it is known, and an explicit selected-window basis note when
// the record also carries actual_* (aligned-window) values. Terms like
// runnable/s_sleep stay as trace terms next to their translation.
func runtimeTraceMetricSnapshotDisplayText(record types.ObservationRecord, zh bool) string {
	values := runtimeTraceMetricSnapshotValues(record)
	if values == nil {
		return ""
	}
	ms := func(key string) string { return runtimeTraceMetricWithMS(values[key]) }
	total := runtimeTraceMetricFloat(values[types.TraceNoteKeyTotal])
	share := func(key string) string {
		v := runtimeTraceMetricFloat(values[key])
		if total <= 0 || v <= 0 {
			return ""
		}
		return fmt.Sprintf("%.0f%%", v/total*100)
	}
	stateEntry := func(label, key string) string {
		entry := label + " " + ms(key)
		// H13: the share denominator is the thread's OWN observed state total,
		// not the analysis window. PTV8-RCR-B (UXA 域C #10): the basis clause
		// moved to the group head (said once); a share that rounds to 0% says
		// nothing and is dropped.
		if pct := share(key); pct != "" && pct != "0%" {
			if zh {
				entry += "(" + pct + ")"
			} else {
				entry += " (" + pct + ")"
			}
		}
		return entry
	}
	// PTV8-RCR-B (UXA 域C #10 REVISE 修正稿, 2026-07-08). EVOLUTION RECORD:
	// ten metrics used to glue into one run-on with the 13-char share
	// disclaimer repeated per state — the line now reads as two groups
	// (状态时长 / 切换特征), the share basis stated ONCE in the group head;
	// the dominant entry stays first (raw-token carrier, PTV7 alias pair) and
	// zero-share parentheticals are dropped.
	dominantEntry := ""
	if dominant := strings.TrimSpace(values[types.TraceNoteKeyDominantState]); dominant != "" {
		// PTV7 (#74): the parenthetical carries the canonical display alias
		// ONLY when it differs from the raw token (s_sleep(sleep)); an
		// identity echo (runnable(runnable)) says nothing and is dropped.
		label := runtimeTraceProjStateKindLabel(types.TraceCausalProjectionNode{StateKind: dominant}, zh)
		dominantEntry = dominant
		if label != "" && label != dominant {
			dominantEntry = dominant + "(" + label + ")"
			if !zh {
				dominantEntry = dominant + " (" + label + ")"
			}
		}
	}
	// PTV7 (#74): the per-state lane labels are the canonical state tokens on
	// both faces; the metric frame words stay localized.
	states := []string{
		stateEntry("running", types.TraceNoteKeyRunning),
		stateEntry("runnable", types.TraceNoteKeyRunnable),
		stateEntry("sleep", types.TraceNoteKeySleep),
		stateEntry("D-state", types.TraceNoteKeyDState),
		stateEntry("iowait", types.TraceNoteKeyIOWait),
	}
	// CR-2 组③ P7 (F5-1, 2026-07-12): a wakeup-lane snapshot row's per-state
	// durations are CHAIN-EPISODE-scoped (the record measures the thread's
	// states inside one chain occurrence, not the query window) — the head
	// must say so, or 「running 0.000ms」 beside 「查询窗 X–Y」 reads as a
	// full-window statistic (tieba 主线程 witness: episode running 0.000 vs
	// raw full-window 26.9ms). Precise predicate fork; state_churn records
	// (true query-window accumulation) keep the legacy head byte-for-byte.
	episodeScoped := runtimeTraceMetricSnapshotEpisodeScoped(record)
	var text string
	// A2 件4② (§29.174 UX-16②, 2026-07-21): the per-state share denominator is
	// the THREAD's own observed span — the interval its state events actually
	// cover, i.e. the thread's activity neighbourhood — which can sit far below
	// the analysis window (runnable_2:509 witness: tieba states sum ≈22ms while
	// the analysis window is 144.503ms, and the 89% share silently reads as a
	// window share). The non-episode head now disclosures the denominator's
	// identity inline; the episode-scoped head already states its own scope.
	if zh {
		head := "状态时长(括号为占该线程观测时长比例;观测窗=该线程运行邻域,非分析窗): "
		if episodeScoped {
			head = "链上发生段内状态时长(仅统计该链上发生段,非查询窗全量;括号为占该段观测时长比例): "
		}
		if dominantEntry != "" {
			head += "主导 " + dominantEntry + ";"
		}
		text = head + strings.Join(states, " · ") +
			";切换特征: " + values[types.TraceNoteKeySwitches] + " 次切换/" + values[types.TraceNoteKeyFragments] + " 段" +
			",最长单段 " + ms(types.TraceNoteKeyMaxSegment) +
			",P95 段长 " + ms(types.TraceNoteKeyP95Segment)
	} else {
		head := "state durations (parentheses = share of this thread's observed span; observed span = the thread's activity neighbourhood, not the analysis window): "
		if episodeScoped {
			head = "on-chain episode state durations (episode-scoped only, not the full query window; parentheses = share of the episode's observed span): "
		}
		if dominantEntry != "" {
			head += "dominant " + dominantEntry + "; "
		}
		text = head + strings.Join(states, " · ") +
			"; switching: " + values[types.TraceNoteKeySwitches] + " switches/" + values[types.TraceNoteKeyFragments] + " segments" +
			", longest segment " + ms(types.TraceNoteKeyMaxSegment) +
			", p95 segment " + ms(types.TraceNoteKeyP95Segment)
	}
	if runtimeTraceRecordHasActualWindowValues(record) {
		// NEW-8 (账本 §7.6): when the source observation carries the producer's
		// typed selected_window note (strict shared parser — both endpoints must
		// be legal floats), the basis line names the window endpoints inline:
		// the user panel cannot open the raw blob, so "见原始 trace_query 记录"
		// alone was a dead end. PTV6-C ruling C (#73, 用户裁定 2026-07-06): the
		// aligned actual-window VALUES themselves inline too — the intermediate
		// record file is no longer a user-facing pointer target. When none of
		// the actual_* notes parse, the basis names the dual-basis fact without
		// any deflection pointer.
		endpoints := ""
		if start, end, ok := types.TraceCausalProjectionSelectedWindowNote(record.RichNotes); ok {
			endpoints = fmt.Sprintf("%.3fs~%.3fs", start, end)
		}
		actual := runtimeTraceMetricSnapshotActualInline(record, zh)
		// PTV8-RCR-B (UXA 域C #11 + 域D #15, 2026-07-08). EVOLUTION RECORD:
		// 选定窗→查询窗 / 实际对齐窗→数据实际覆盖 (窗族终词), and the aligned
		// values leave the nested parens for a parallel ";" clause (三层嵌套
		// 拆平). Producer selected_window raw note untouched.
		var basis string
		if zh {
			basis = ";窗口基准: 查询窗"
			if endpoints != "" {
				basis += " " + endpoints
			}
			if episodeScoped {
				// CR-2 组③ P7 (F5-1): the query window is where the chain was
				// SEARCHED, not what this row's values account for.
				basis += "(检索范围,非该行统计范围)"
			}
			if actual != "" {
				basis += ";" + actual
			} else {
				basis += "(另有按实际状态段跨度统计的数值)"
			}
		} else {
			basis = "; window basis: selected window"
			if endpoints != "" {
				basis += " " + endpoints
			}
			if episodeScoped {
				basis += " (search scope, not this row's accounting scope)"
			}
			if actual != "" {
				basis += " (" + actual + ")"
			} else {
				basis += " (an actual segment-span caliber also exists)"
			}
		}
		text += basis
	}
	return text
}

// runtimeTraceMetricSnapshotActualInline renders the aligned actual-window
// values carried by the record's typed actual_* rich notes as one inline
// clause (PTV6-C ruling C): window endpoints first, then the per-state
// durations in the snapshot's own state order, then the totals — only the
// notes actually present render; "" when nothing parses. Display copy only;
// the raw notes stay verbatim on the observation record.
func runtimeTraceMetricSnapshotActualInline(record types.ObservationRecord, zh bool) string {
	value := func(key string) string {
		prefix := key + "="
		for _, note := range record.RichNotes {
			note = strings.TrimSpace(note)
			if strings.HasPrefix(note, prefix) {
				return strings.TrimSpace(strings.TrimPrefix(note, prefix))
			}
		}
		return ""
	}
	var parts []string
	statePart := func(zhLabel, enLabel, key string) {
		v := value(key)
		if v == "" {
			return
		}
		label := zhLabel
		if !zh {
			label = enLabel
		}
		parts = append(parts, label+" "+runtimeTraceMetricWithMS(v))
	}
	// PTV7 (#74): state lane labels = canonical tokens, face-invariant.
	statePart("running", "running", types.TraceNoteKeyActualRunning)
	statePart("runnable", "runnable", types.TraceNoteKeyActualRunnable)
	statePart("sleep", "sleep", types.TraceNoteKeyActualSleep)
	statePart("D-state", "D-state", types.TraceNoteKeyActualDState)
	statePart("iowait", "iowait", types.TraceNoteKeyActualIOWait)
	statePart("合计", "total", types.TraceNoteKeyActualTotalMS)
	if len(parts) == 0 {
		statePart("合计", "total", types.TraceNoteKeyActualTotal)
		statePart("影响", "impact", types.TraceNoteKeyActualImpactMS)
		if len(parts) == 0 {
			statePart("影响", "impact", types.TraceNoteKeyActualImpact)
		}
	}
	// 修正轮 Low (2026-07-06): the endpoints go through the SAME strict
	// shared parser as the selected_window note (ParseFloat both ends,
	// end>start) — a malformed note renders no endpoints, never a fabricated
	// window.
	window := ""
	if raw := value(types.TraceNoteKeyActualWindow); raw != "" {
		if start, end, ok := types.TraceCausalProjectionParseWindowValue(raw); ok {
			window = fmt.Sprintf("%.3fs~%.3fs", start, end)
		}
	}
	if len(parts) == 0 && window == "" {
		return ""
	}
	// PTV8-RCR-B (UXA 域D #15/#33 窗族): 实际对齐窗 → 数据实际覆盖.
	// EVOLUTION RECORD (CR-2 组③ P7 / F5-1 已立案复现, 2026-07-12): 数据实际
	// 覆盖 → 实际状态段跨度(活动切片,非全窗事件覆盖). The actual_window note
	// is the envelope of the scheduler-state segments that fed THIS row —
	// never a statement about how far the thread's trace data reaches
	// (donghu CompThread: 「覆盖 13762.988–13763.010」 while raw events span
	// the whole window; reading it as data coverage was the F5-1 misdirection).
	head := "实际状态段跨度"
	caliber := "(活动切片,非全窗事件覆盖)"
	if !zh {
		head = "actual segment span"
		caliber = " (active slice, not full-window event coverage)"
	}
	if window != "" {
		head += " " + window
	}
	head += caliber
	if len(parts) == 0 {
		return head
	}
	return head + ": " + strings.Join(parts, "/")
}

// runtimeTraceMetricSnapshotEpisodeScoped reports whether the snapshot record's
// per-state durations were measured inside ONE chain occurrence (the wakeup
// lanes) instead of accumulated over the query window (state_churn). Two
// precise signals (CR-2 组③ P7 / F5-1):
//   - the wakeup-lane predicates; and
//   - the actual_window note — the underlying segment envelope is minted
//     EXCLUSIVELY by the occurrence-scoped wakeup measurement lanes. The
//     chain-derived rank rows qualify for the snapshot through their summary
//     tokens while carrying a root_cause_* predicate (donghu replay witness
//     2026-07-12: the CompThread/JankManager snapshot rows rode that lane and
//     the predicate arm alone missed them). state_churn rows (true
//     query-window accumulation) never publish actual_window and keep the
//     legacy head byte-for-byte; rows with neither signal stay legacy too
//     (no typed scope proof, no scope claim).
func runtimeTraceMetricSnapshotEpisodeScoped(record types.ObservationRecord) bool {
	switch strings.TrimSpace(record.Predicate) {
	case "wakeup_causal_impact", "wakeup_causal_aggregate":
		return true
	case "state_churn":
		return false
	}
	for _, note := range record.RichNotes {
		if strings.HasPrefix(strings.TrimSpace(note), types.TraceNoteKeyActualWindow+"=") {
			return true
		}
	}
	return false
}

// runtimeTraceRecordHasActualWindowValues reports whether the record publishes
// any actual_* (aligned/underlying-window) rich note alongside its
// selected-window values — the §7.30 S1 dual-basis shape that must be labeled.
func runtimeTraceRecordHasActualWindowValues(record types.ObservationRecord) bool {
	for _, note := range record.RichNotes {
		if strings.HasPrefix(strings.TrimSpace(note), types.TraceNoteKeyActualPrefix) {
			return true
		}
	}
	return false
}

func runtimeTraceMetricFloat(value string) float64 {
	value = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), "ms"))
	if value == "" {
		return 0
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return f
}

func runtimeTraceStateChurnHasPositiveImpact(record types.ObservationRecord) bool {
	if runtimeTracePositiveMetric(record.Value) {
		return true
	}
	if runtimeTracePositiveMetric(runtimeTraceObservationRichNoteValue(record.RichNotes, types.TraceNoteKeyImpact)) {
		return true
	}
	values := map[string]string{}
	runtimeTraceMergeSummaryMetricTokens(values, record.Summary, []string{"impact"})
	return runtimeTracePositiveMetric(values[types.TraceNoteKeyImpact])
}

func runtimeTracePositiveMetric(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	value = strings.TrimSuffix(strings.ToLower(value), "ms")
	value = strings.TrimSpace(strings.TrimSuffix(value, "毫秒"))
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	v, err := strconv.ParseFloat(value, 64)
	return err == nil && v > 0
}

func runtimeTraceMetricWithMS(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	if strings.HasSuffix(lower, "ms") || strings.HasSuffix(value, "毫秒") {
		return value
	}
	return value + "ms"
}

func materializeRuntimeTraceNextStepsBlock(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil || ctx.Mutable == nil {
		return false
	}
	if len(doc.Blocks) >= maxBlocksPerDoc {
		logging.Warning("[answer_document] runtime trace next-step block skipped: document already at the %d-block cap", maxBlocksPerDoc)
		return false
	}
	if answerDocumentHasNextStepsBlock(doc) {
		return false
	}
	items := runtimeTraceNextStepItems(doc, ctx)
	if len(items) == 0 {
		return false
	}
	block := types.AnswerBlock{
		ID:    "next_steps",
		Kind:  types.BlockOrderedList,
		Items: items,
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimExternalObservation,
		}},
		FacetIDs: []string{"observed_artifact_fact"},
	}
	markRuntimeTraceSystemBlock(&block)
	insertAt := answerDocumentInsertionIndexBeforeRuntimeTraceDetails(doc)
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{})
	copy(doc.Blocks[insertAt+1:], doc.Blocks[insertAt:])
	doc.Blocks[insertAt] = block
	return true
}

func answerDocumentHasNextStepsBlock(doc *types.AnswerDocumentV2) bool {
	if doc == nil {
		return false
	}
	for _, block := range doc.Blocks {
		if answerDocumentBlockIDIsNextSteps(block.ID) {
			return true
		}
	}
	return false
}

func answerDocumentBlockIDIsNextSteps(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.NewReplacer("-", "_", " ", "_").Replace(id)
	switch id {
	case "next_step", "next_steps":
		return true
	default:
		return false
	}
}

// runtimeTraceNextStepMaxItems caps the rendered next-step list. PTS-2 (#69
// 用户条件裁定 2026-07-06, 账本 real_trace_campaign_20260705.md §7.2): on the
// COMPARISON shape the cap grows dynamically — the comparison-family rows
// (three fixed + the RTC-2 disjoint row) emit in full and the per-record rows
// keep a guaranteed floor of runtimeTraceNextStepComparisonRecordFloor slots,
// so the RTC-2 row can no longer squeeze the per-record guidance out of the
// shared budget (the former cmp6 residual). NXT (§22 D-P1, 2026-07-07): the
// undrilled-headline pointed row adds its own
// runtimeTraceNextStepUndrilledHeadlineFloor seat on top, so the hard upper
// bound is base cap + undrilled floor + per-record floor = 7 rows (reachable
// only on a comparison shape whose headline is also undrilled). Every
// non-comparison shape keeps this base cap byte-identical (the intermediate
// lanes below still read it directly).
const runtimeTraceNextStepMaxItems = 4

// runtimeTraceNextStepComparisonRecordFloor is the PTS-2 per-record slot
// guarantee on the comparison shape: 2 = one top per-record guidance row per
// trace of the dual-trace comparison form. Applied AFTER every leading lane
// (comparison rows + the coexisting recovery hints), so the reserved slots
// can only be consumed by per-record rows.
const runtimeTraceNextStepComparisonRecordFloor = 2

// runtimeTraceNextStepUndrilledHeadlineFloor is the NXT (§22 D-P1,
// real_trace_campaign_20260705.md, 2026-07-07) guaranteed seat for the
// undrilled-headline pointed drilldown row — the PTS-2 floor precedent
// applied to the headline lane. Ruling: 1 seat via DISPLACEMENT inside the
// base cap on ordinary shapes (the pointed lane runs before every generic
// lane, so a generic template row is what yields — huadong_01 shipped
// "3 通用+1 口径" with the rank=1 binder_wait headline named nowhere), and via
// cap EXTENSION only when the leading comparison family (which emits in full
// by #69 adjudication and must not be displaced either) already filled the
// base cap.
const runtimeTraceNextStepUndrilledHeadlineFloor = 1

func runtimeTraceNextStepItems(doc *types.AnswerDocumentV2, ctx *types.BusContext) []types.AnswerBlockItem {
	if doc == nil || ctx == nil {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, 64))
	if len(ledger.Records) == 0 {
		return nil
	}
	zh := runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx))
	// PTV8-RCR-B (UXA 域C #12, 2026-07-08). EVOLUTION RECORD: every item used
	// to re-print a bold 「下一步」 label under a block already titled 下一步
	// (4 条 8 个"下一步") — the per-item label is retired; the renderer's
	// block-level title carries the word.
	label := ""
	seen := make(map[string]bool)
	seenText := make(map[string]bool)
	var out []types.AnswerBlockItem
	// CMP-6 comparison lane: on a cross-trace comparison ledger (same
	// deterministic ≥2-active-projection gate as the projection
	// comparison-overview table, NEW-2) the fixed comparison-oriented
	// guidance rows LEAD the list — they are the headline next steps for this
	// question shape, and trailing placement would let generic single-trace
	// rows crowd them out of the shared cap. They pass through the same
	// verbatim display dedupe as the per-record rows. Single-projection
	// dispatches take this branch never and stay byte-identical.
	//
	// PTS-2 (#69 用户条件裁定 2026-07-06): the comparison family emits in FULL
	// (对比行全集 — no cap break inside this loop; today's family is ≤4 rows
	// anyway, the removal guards future family growth) and the per-record
	// loop below gets a dynamically extended cap so both row kinds coexist.
	// Priority order unchanged: comparison rows still lead; the
	// comparison-row ⟺ overview-table lockstep gate is untouched.
	comparisonShape := runtimeTraceNextStepComparisonShape(ledger)
	if comparisonShape {
		steps := runtimeTraceNextStepComparisonSteps(zh)
		// RTC-2 (real_trace_campaign_20260705.md §4 案 e2, 批 #67): when the
		// compiled partitions' time-base spans are pairwise disjoint, the
		// comparison lane grows ONE more guidance row — the disclosure that
		// the traces cannot be aligned on one shared timeline. Same typed
		// signal (and thus lockstep) as the overview table's disjoint note
		// row; zero emission on a single partition or any span intersection.
		if hint := runtimeTraceNextStepDisjointTimeBaseStep(ledger, zh); hint != "" {
			steps = append(steps, hint)
		}
		for _, step := range steps {
			if step == "" || seenText[step] {
				continue
			}
			seenText[step] = true
			out = append(out, types.AnswerBlockItem{
				ID:          fmt.Sprintf("runtime_trace_next_step_%d", len(out)+1),
				Label:       label,
				Text:        step,
				CitationRef: -1,
			})
		}
	}
	// CMP-C F3 (adversarial review 2026-07-04, 单边未采样引导): the comparison
	// predicate is set and the deterministic preflight census says ≥2 logical
	// trace captures are in play, but the ledger compiled to exactly ONE
	// projection — the other capture was never sampled. The directive gate and
	// the overview gate stay lockstepped by design (both key on the compiled
	// partition count); this row bridges the gap window with guidance only: it
	// names the unsampled capture when exactly one remains and never fabricates
	// comparison data or the overview table. Mutually exclusive with the
	// comparison rows above (partition count 1 vs ≥2).
	if hint := runtimeTraceNextStepUnsampledComparisonHint(ctx, ledger, zh); hint != "" &&
		len(out) < runtimeTraceNextStepMaxItems && !seenText[hint] {
		seenText[hint] = true
		out = append(out, types.AnswerBlockItem{
			ID:          fmt.Sprintf("runtime_trace_next_step_%d", len(out)+1),
			Label:       label,
			Text:        hint,
			CitationRef: -1,
		})
	}
	// NXT N1+N2 (§22 D-P1, real_trace_campaign_20260705.md 2026-07-07; RCX①
	// drill-debt 用户面 next-step 半场, §12.3-1①): when a projection's HEADLINE
	// — the node its conclusion line actually names, through the SAME
	// runtimeTraceProjLeadSelect surface — is undrilled (typed signals only:
	// the lead's rendered tree row sits on the dedicated 链上·深度未解析 lane,
	// or the lead carries the typed UndrillableReason), the list gains ONE
	// pointed drilldown row naming that subject, why it is unresolved and the
	// concrete views to run (wakeup_chain / critical_blocking_calls). The
	// huadong_01 specimen shipped a rank=1 · conf 0.92 binder_wait headline
	// with four generic/window-caliber rows and zero pointed guidance.
	// Placement: after the comparison-family lanes (their headline
	// adjudication is untouched) and BEFORE every generic lane, so the most
	// specific row never loses its seat to a template row. The floor constant
	// guarantees the seat even when the comparison family already filled the
	// base cap (see the N2 ruling on the constant).
	undrilledHeadlineRows := 0
	for _, hint := range runtimeTraceNextStepUndrilledHeadlineHints(ledger, zh) {
		if hint == "" || seenText[hint] {
			continue
		}
		if len(out) >= runtimeTraceNextStepMaxItems &&
			undrilledHeadlineRows >= runtimeTraceNextStepUndrilledHeadlineFloor {
			break
		}
		seenText[hint] = true
		undrilledHeadlineRows++
		out = append(out, types.AnswerBlockItem{
			ID:          fmt.Sprintf("runtime_trace_next_step_%d", len(out)+1),
			Label:       label,
			Text:        hint,
			CitationRef: -1,
		})
	}
	// PTV5 Q3 (#68 用户裁定 2026-07-05, 单工件多锚窗支): exactly one compiled
	// projection with ≥2 distinct typed query windows — the within-trace
	// dual-window comparison guidance (CMP-9 normalization caliber + per-window
	// causal sampling). Mutually exclusive with the ≥2-projection comparison
	// rows above by construction.
	for _, step := range runtimeTraceNextStepMultiWindowSteps(ledger, zh) {
		if len(out) >= runtimeTraceNextStepMaxItems {
			break
		}
		if step == "" || seenText[step] {
			continue
		}
		seenText[step] = true
		out = append(out, types.AnswerBlockItem{
			ID:          fmt.Sprintf("runtime_trace_next_step_%d", len(out)+1),
			Label:       label,
			Text:        step,
			CitationRef: -1,
		})
	}
	// RN-13(b) (§7.9 runnable 主导场景审计 2026-07-04): flat-fallback shape whose
	// analysis anchor is NOT the user's focused thread — the projection header
	// disclosed the mismatch (RN-13(a), same typed lane); this row is the
	// recovery guidance: re-run wakeup_chain for the user's thread to restore
	// the causal tree. Shares the verbatim display dedupe and the one item cap
	// with every other lane (coexists with the CMP-6 comparison rows).
	for _, hint := range runtimeTraceNextStepFlatAnchorRecoveryHints(ctx, ledger, zh) {
		if len(out) >= runtimeTraceNextStepMaxItems {
			break
		}
		if hint == "" || seenText[hint] {
			continue
		}
		seenText[hint] = true
		out = append(out, types.AnswerBlockItem{
			ID:          fmt.Sprintf("runtime_trace_next_step_%d", len(out)+1),
			Label:       label,
			Text:        hint,
			CitationRef: -1,
		})
	}
	// SG 批 (§10-D1② + Q4-K 修5): named holder/peer drilldown rows — when the
	// compiled projection resolved a blocking peer (lock holder via the typed
	// §7.30.3 D1 payload parse, or a binder_wait row whose peer thread is
	// named), the next-step list points at THAT thread concretely. Same
	// shared-cap + typed-key-then-verbatim dedupe discipline (R4-3/裁定5+H9)
	// as every other lane. (A2 件1: the generic s_sleep stand-down interplay
	// retired with the per-record template lane below.)
	for _, hint := range runtimeTraceNextStepResolvedPeerHints(ledger, zh) {
		if len(out) >= runtimeTraceNextStepMaxItems {
			break
		}
		if hint == "" || seenText[hint] {
			continue
		}
		seenText[hint] = true
		out = append(out, types.AnswerBlockItem{
			ID:          fmt.Sprintf("runtime_trace_next_step_%d", len(out)+1),
			Label:       label,
			Text:        hint,
			CitationRef: -1,
		})
	}
	// PTS-2 dynamic cap (#69 用户条件裁定 2026-07-06): on the comparison shape
	// the trailing lane reads an extended cap — every leading lane above has
	// already been placed, so the guaranteed floor slots can only be consumed
	// by trailing rows (强保底, not a shared trailing budget). Hard upper
	// bound = base + undrilled-headline floor + trailing floor = 7 (NXT §22
	// D-P1). A2 件1 EVOLUTION: the trailing lane's POPULATION is now the ◎
	// direction-action rows (the per-record template rows retired, §29.174
	// UX-13); the cap/floor mechanics themselves are untouched.
	recordCap := runtimeTraceNextStepMaxItems
	if comparisonShape {
		recordCap = len(out) + runtimeTraceNextStepComparisonRecordFloor
		if recordCap < runtimeTraceNextStepMaxItems {
			recordCap = runtimeTraceNextStepMaxItems
		}
	}
	// A2 件1 (§29.174 UX-13, 2026-07-21): the ◎ direction-action lane — one
	// concrete subject+value action per PUBLISHED fix-direction section, in
	// section order (see answer_document_mutation_runtime_nextstep_a2.go for
	// the retirement record and the closed action-verb table). Typed dedupe
	// key = direction+subject+value (two projections legitimately mint
	// distinct rows for one direction); the H9 verbatim display dedupe layers
	// on top like every other lane.
	for _, action := range runtimeTraceNextStepDirectionActions(ctx, zh) {
		if len(out) >= recordCap {
			break
		}
		key := "direction\x00" + action.direction + "\x00" + action.subject + "\x00" + action.value
		if seen[key] || seenText[action.text] {
			continue
		}
		seen[key] = true
		seenText[action.text] = true
		out = append(out, types.AnswerBlockItem{
			ID:          fmt.Sprintf("runtime_trace_next_step_%d", len(out)+1),
			Label:       label,
			Text:        action.text,
			CitationRef: -1,
		})
	}
	return out
}

// runtimeTraceNextStepComparisonShape gates the CMP-6 comparison-oriented
// next-step rows: the SAME deterministic gate as the projection comparison
// overview (runtimeTraceProjComparisonShape — ≥2 compiled ACTIVE per-artifact
// projections; NEW-2 §7.6 回访裁定 dropped the LLM analyzer predicate from
// both surfaces IN LOCKSTEP), evaluated against the number of per-artifact
// projections this ledger compiles to. Reusing the projection partition
// (typed artifact identity, spelling-alias merge, active check) keeps "when
// do comparison next-steps appear" identical to "when does the comparison
// overview table appear" — one gate, two display surfaces. A single-artifact
// ledger fails closed: no comparison rows, list byte-identical to the
// pre-CMP-6 output.
func runtimeTraceNextStepComparisonShape(ledger types.ObservationLedger) bool {
	return runtimeTraceProjComparisonShape(len(types.CompileTraceCausalProjectionSet(ledger).Projections))
}

// runtimeTraceNextStepComparisonSteps returns the fixed comparison-oriented
// next-step guidance rows for a typed cross-trace comparison (CMP-6): anchor
// the target span per trace before comparing, and normalize window aggregates
// by each trace's own window length before forming any cross-trace ratio
// (§7.3 CMP-9 口径). System-fixed guidance strings in the same lane as the
// per-record next-step rows; they carry no scalar claims about either trace.
func runtimeTraceNextStepComparisonSteps(zh bool) []string {
	// PTV5 C19 (#68): 『同窗』 read as one shared absolute window — two traces
	// cannot share one; the zh row says 各自同口径窗口内 (the EN row already
	// said same-caliber windows). The "running 时间" wording is the §7.2 CMP
	// design verbatim and stays. PTV5 Q3 (#68 用户裁定 2026-07-05): the third
	// row steers dual-/multi-window comparisons to run causal sampling PER
	// query window before comparing.
	if zh {
		return []string{
			"对比两 trace 各自同口径窗口内 top 运行线程与进程级 running 时间差异",
			"对齐目标 span 边界后重取两侧聚合指标(按各自窗长归一化后再对比)",
			"双窗/多窗对比时:对每个查询窗分别做同样的根因分析(wakeup_chain/root_cause_rank),再逐窗对比",
		}
	}
	return []string{
		"Compare the top running threads and per-process running time of both traces over same-caliber windows",
		"Re-anchor each trace to the target span boundaries, then re-collect the window aggregates normalized by each window's own length before comparing",
		"For dual-/multi-window comparisons: run the same root-cause analysis (wakeup_chain/root_cause_rank) per query window, then compare window by window",
	}
}

// runtimeTraceNextStepMultiWindowSteps returns the PTV5 Q3 single-artifact
// multi-anchor-window guidance rows (#68 用户裁定 2026-07-05, 对比门旁的
// 单工件多锚窗支): the ledger compiled to EXACTLY one active projection whose
// trace_query records carry ≥2 DISTINCT typed query windows — a within-trace
// dual-window comparison shape. The rows reuse the CMP-9 normalization
// caliber (relative, per-window-length normalized metrics) and steer causal
// sampling to run PER WINDOW. The preflight capture census (artifact identity
// language) is deliberately NOT consulted — windows are not captures
// (census=capture 语言不挪用). Empty on every other shape, so the existing
// gates stay byte-identical.
func runtimeTraceNextStepMultiWindowSteps(ledger types.ObservationLedger, zh bool) []string {
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 {
		return nil
	}
	windows := set.Projections[0].QueryWindows
	if len(windows) < 2 {
		return nil
	}
	// 复核 Low (2026-07-06): a cap-truncated window list renders a lower bound.
	count := fmt.Sprintf("%d", len(windows))
	if set.Projections[0].QueryWindowsTruncated {
		count = fmt.Sprintf("≥%d", len(windows))
	}
	if zh {
		return []string{
			fmt.Sprintf("本 trace 含 %s 个查询窗:窗长不同时先按各自窗长归一化(占窗比例)再跨窗对比", count),
			"双窗对比:对每个查询窗分别做同样的根因分析(wakeup_chain/root_cause_rank),再逐窗对比",
		}
	}
	return []string{
		fmt.Sprintf("This trace carries %s query windows: normalize by each window's own length (window share) before comparing across windows", count),
		"Dual-window comparison: run the same-caliber causal sampling (wakeup_chain/root_cause_rank) per query window, then compare window by window",
	}
}

// runtimeTraceNextStepDisjointTimeBaseStep returns the RTC-2 comparison-lane
// guidance row, or "" when the shape does not apply. Gate = the SAME pure
// arithmetic as the overview table's disjoint note
// (types.TraceCausalProjectionTimeBasesDisjoint over the SAME compiled
// partition set), so the two surfaces stay in lockstep by construction —
// mirroring the CMP-6 对比行⟺总览表 adjudication, this row can only appear
// when the comparison rows (and therefore the overview) appear, and the
// disjoint note row appears on the table exactly when this row appears here.
// Wording is user-facing guidance (no envelope/partition jargon) and carries
// no scalar claims about either trace.
func runtimeTraceNextStepDisjointTimeBaseStep(ledger types.ObservationLedger, zh bool) string {
	projections := types.CompileTraceCausalProjectionSet(ledger).Projections
	if !types.TraceCausalProjectionTimeBasesDisjoint(projections) {
		return ""
	}
	two := len(projections) == 2
	switch {
	case zh && two:
		return "两 trace 时间基准不相交,无法在同一时间轴直接对齐;对比请以各自窗口内相对指标为准(占窗比例/按窗长归一化)"
	case zh:
		return "各 trace 时间基准两两不相交,无法在同一时间轴直接对齐;对比请以各自窗口内相对指标为准(占窗比例/按窗长归一化)"
	case two:
		return "The two traces' time bases do not overlap and cannot be aligned directly on one shared timeline; compare relative metrics within each trace's own window (window share / normalized by window length)"
	default:
		return "The traces' time bases are pairwise disjoint and cannot be aligned directly on one shared timeline; compare relative metrics within each trace's own window (window share / normalized by window length)"
	}
}

// runtimeTraceNextStepUnsampledComparisonHint returns the CMP-C F3 single-sided
// sampling row, or "" when the shape does not apply. Gate — precise typed
// signals only: (1) the analyzer's comparison-form boolean
// (historical_regression / is_cross_component, the same predicate as the
// comparison overview), (2) the ledger compiled to EXACTLY one per-artifact
// projection, (3) the runtime-artifact preflight census counts ≥2 logical
// trace captures via types.RuntimeTracePreflightCaptureIdentityPaths — the
// SAME F1 capture-identity helper the skill-tier directive gate consumes, so
// the two gates can never disagree on what "two traces" means. The unsampled
// capture is the census identity set minus the projected artifact (same-capture
// check = merging the pair through the same helper); it is NAMED only when
// exactly one identity remains, and a multi-remainder or unattributable
// projection falls back to the generic phrase — never a guessed name.
func runtimeTraceNextStepUnsampledComparisonHint(ctx *types.BusContext, ledger types.ObservationLedger, zh bool) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	if !rm.DiagnosticProfile.HistoricalRegression && !rm.Predicates.IsCrossComponent {
		return ""
	}
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 {
		return ""
	}
	identities := types.RuntimeTracePreflightCaptureIdentityPaths(ctx.RuntimeArtifactPreflight)
	if len(identities) < 2 {
		return ""
	}
	projected := strings.TrimSpace(set.Projections[0].ArtifactPath)
	var remaining []string
	for _, identity := range identities {
		if projected != "" && len(types.TraceArtifactCaptureIdentityPaths([]string{identity, projected})) == 1 {
			// Same logical capture as the one already projected.
			continue
		}
		remaining = append(remaining, identity)
	}
	if len(remaining) == 0 {
		// Census and projection disagree on identity coverage; fail closed.
		return ""
	}
	// PTV5 C20 (#68): the artifact EXISTS (preflight census confirmed the
	// capture) — what is missing is THIS REPORT's queries against it, so the
	// wording says 本报告未取数, never "尚未采样" (read as "the trace itself
	// has no data"). PTV8-RCR-B (UXA 域D #23 本轮→本报告 + C#14 去"视图",
	// 2026-07-08): both arms speak the family words.
	if len(remaining) == 1 {
		name := runtimeTraceCaptureIdentityBasename(remaining[0])
		if name != "" {
			if zh {
				return fmt.Sprintf("另一份 trace 本报告未取数:对 %s 以同口径(同窗)执行查询后再对比", name)
			}
			return fmt.Sprintf("The other trace was not queried for this report: run the same-caliber queries (same window) on %s, then compare", name)
		}
	}
	if zh {
		return "另一份 trace 本报告未取数:对其余未取数的 trace 文件以同口径(同窗)执行查询后再对比"
	}
	return "The other trace was not queried for this report: run the same-caliber queries (same window) on the remaining trace files, then compare"
}

// runtimeTraceNextStepUndrilledHeadlineHints returns the NXT pointed
// drilldown rows (§22 D-P1, real_trace_campaign_20260705.md 2026-07-07; RCX①
// §12.3-1① 用户面 next-step 半场): one per compiled ACTIVE projection whose
// ELECTED headline is undrilled. Precise typed signals only, never prose
// matching:
//   - the headline is the runtimeTraceProjLeadSelect product on the primary /
//     on-chain-fallback lanes (the SAME single surface the conclusion line and
//     the comparison primary cell consume, so this row can never name a
//     different node than the headline it points at); the semantic lane never
//     wears the 主根因 claim and is excluded by construction;
//   - "undrilled" = the lead's rendered tree row sits on the dedicated
//     链上·深度未解析 lane (typed row Kind + Edge pair, stamped once at model
//     build — see runtimeTraceNextStepHeadlineDepthUnresolved), OR the lead
//     node carries the typed UndrillableReason (missing_wakeup);
//   - a fold roster / aggregate metric names no drillable thread and an
//     unresolved subject is never pointed at (SG precedent: soft guidance
//     never fabricates an identity).
//
// Soft guidance only — nothing gates on these rows. Multi-projection ledgers
// disambiguate with the artifact label; identical texts dedupe here and the
// caller layers the shared verbatim display dedupe on top.
func runtimeTraceNextStepUndrilledHeadlineHints(ledger types.ObservationLedger, zh bool) []string {
	set := types.CompileTraceCausalProjectionSet(ledger)
	multi := len(set.Projections) > 1
	var out []string
	seen := map[string]bool{}
	for _, projection := range set.Projections {
		if !projection.Active() {
			continue
		}
		model := buildRuntimeTraceProjTreeModel(projection, nil, zh)
		lead, lane := runtimeTraceProjLeadSelect(projection, model)
		if lead == nil ||
			(lane != runtimeTraceProjLeadLanePrimary && lane != runtimeTraceProjLeadLaneOnChainFallback) {
			continue
		}
		if lead.OnChainOverflowFold || lead.IsAggregateMetric() ||
			!runtimeTraceCausalProjectionKnownSubject(lead.Subject) {
			continue
		}
		depthUnresolved := runtimeTraceNextStepHeadlineDepthUnresolved(*lead, model)
		if !depthUnresolved && !lead.Undrillable() {
			continue
		}
		artifact := ""
		if multi {
			artifact = strings.TrimSpace(projection.ArtifactLabel)
		}
		text := runtimeTraceNextStepUndrilledHeadlineText(*lead, artifact, depthUnresolved, zh)
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, text)
	}
	return out
}

// runtimeTraceNextStepHeadlineDepthUnresolved reports whether the elected
// lead renders on the dedicated 链上·深度未解析 lane of its own projection's
// tree: a TRUNKED render (non-empty model.Target — the same typed signal
// every flat surface reads) whose lead row carries row Kind depthless PLUS
// the chain-unresolved edge — exactly the typed pair the fence edge, relation
// cell and legend consume (stamped once at model build, PTV6 #1b; the
// own-process IO caliber edge is deliberately NOT this lane). Flat renders
// return false: there the whole tree is depthless by construction and the
// dedicated flat header + RN-13(b) recovery lane own that disclosure. Node
// identity = the same node-key equality model.LeadKey is built from.
func runtimeTraceNextStepHeadlineDepthUnresolved(lead types.TraceCausalProjectionNode, model runtimeTraceProjTreeModel) bool {
	if strings.TrimSpace(model.Target) == "" {
		return false
	}
	key := runtimeTraceCausalProjectionNodeKey(lead)
	for _, row := range model.TreeRows {
		if row.Kind != runtimeTraceProjTreeRowDepthless ||
			row.Edge != runtimeTraceProjTreeEdgeChainUnresolved {
			continue
		}
		if runtimeTraceCausalProjectionNodeKey(row.Node) == key {
			return true
		}
	}
	return false
}

// runtimeTraceNextStepUndrilledHeadlineText renders ONE pointed
// undrilled-headline drilldown row: the subject (with its cause word, engine
// rank and, on multi-artifact ledgers, the artifact label), WHY it is
// unresolved (深度未解析 — the same user-facing vocabulary as the tree's
// dedicated edge — or the missing-wakeup wording the conclusion line's ⊘
// clause already speaks) and WHAT to run (the tool-visible view names
// wakeup_chain / critical_blocking_calls; zero internal pipeline names).
// Identity strings and typed enum wording only — the row carries no scalar
// claims.
func runtimeTraceNextStepUndrilledHeadlineText(lead types.TraceCausalProjectionNode, artifact string, depthUnresolved, zh bool) string {
	name := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(lead, zh))
	if name == "" {
		return ""
	}
	var quals []string
	if cause := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(lead, zh)); cause != "" && cause != name {
		quals = append(quals, cause)
	}
	if lead.Rank > 0 {
		if zh {
			quals = append(quals, fmt.Sprintf("%s#%d", tracefence.SeatChannelChainZH, lead.Rank))
		} else {
			quals = append(quals, fmt.Sprintf("%s #%d", tracefence.SeatChannelChainEN, lead.Rank))
		}
	}
	if artifact != "" {
		quals = append(quals, artifact)
	}
	subject := name
	// PTV5 C26 spacing family: a bare latin thread label followed by CJK keeps
	// the separating space; a fullwidth close-paren already separates (the SG
	// named-holder row precedent: "(持有点 %s)在重叠窗执行").
	zhTail := " "
	if len(quals) > 0 {
		zhTail = ""
		if zh {
			subject += "(" + strings.Join(quals, ",") + ")"
		} else {
			subject += " (" + strings.Join(quals, ", ") + ")"
		}
	}
	// CR-3 件② P10 (2026-07-12, 冷读案7 GPU-fence witness): when the
	// unresolved row's thread still has UNCONSUMED sched_blocked_reason
	// markers in the window, the 未解析 wording must not hide the mechanism
	// marker sitting in hand — append the typed residual disclosure.
	residualZH, residualEN := "", ""
	if depthUnresolved && lead.BlockedReasonWindowCount > 0 {
		residualZH = ",但" + runtimeTraceProjBlockedReasonResidualWord(lead, true)
		residualEN = ", yet the " + runtimeTraceProjBlockedReasonResidualWord(lead, false)
	}
	if zh {
		if depthUnresolved {
			return fmt.Sprintf("对主根因 %s%s在其发生窗执行 wakeup_chain / critical_blocking_calls 下钻:该行当前深度未解析,尚无已核实的上游因果%s", subject, zhTail, residualZH)
		}
		return fmt.Sprintf("对主根因 %s%s执行 critical_blocking_calls,并调整窗口后重试 wakeup_chain:所选窗口内无匹配唤醒记录,无法继续上溯", subject, zhTail)
	}
	if depthUnresolved {
		return fmt.Sprintf("Drill into the primary root cause %s with wakeup_chain / critical_blocking_calls in its occurrence window: its chain depth is unresolved and no verified upstream cause is attached yet%s", subject, residualEN)
	}
	return fmt.Sprintf("Run critical_blocking_calls for the primary root cause %s and retry wakeup_chain over an adjusted window: the selected window has no matching wakeup record, so the chain cannot be traced further", subject)
}

// runtimeTraceNextStepFlatAnchorRecoveryHints returns the RN-13(b) recovery
// rows: one per compiled projection whose render is the flat-fallback shape
// AND whose typed anchor-vs-entity comparison mismatched. The gate is exactly
// the RN-13(a) header lane — the hints re-run buildRuntimeTraceProjTreeModel +
// runtimeTraceProjApplyUserFocus (the single implementation of the flat-anchor
// determination), so the header note and the next-step row can never disagree
// on when the shape applies. Empty when no typed entity context exists or no
// projection is flat-mismatched; identical rosters dedupe to one row.
func runtimeTraceNextStepFlatAnchorRecoveryHints(ctx *types.BusContext, ledger types.ObservationLedger, zh bool) []string {
	focus := runtimeTraceProjUserFocusFromBusContext(ctx)
	if len(focus.Entities) == 0 {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, projection := range types.CompileTraceCausalProjectionSet(ledger).Projections {
		if !projection.Active() {
			continue
		}
		model := buildRuntimeTraceProjTreeModel(projection, nil, zh)
		runtimeTraceProjApplyUserFocus(&model, focus)
		if !model.FlatAnchorMismatch || len(model.RootFocusUserEntities) == 0 {
			continue
		}
		var text string
		if zh {
			text = fmt.Sprintf("对用户关注线程(%s)补跑 wakeup_chain 以恢复因果树",
				strings.Join(model.RootFocusUserEntities, "、"))
		} else {
			text = fmt.Sprintf("Re-run wakeup_chain for the user-focused thread (%s) to restore the causal tree",
				strings.Join(model.RootFocusUserEntities, ", "))
		}
		if seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, text)
	}
	return out
}

// runtimeTraceNextStepResolvedPeerHints returns the SG-batch named
// holder/peer drilldown rows (§10-D1② + Q4-K 修5, RN-14a 合成先例同族): one
// per DISTINCT resolved blocking peer across the compiled projections. Two
// typed admission shapes, precise signals only:
//   - lock shape: node.BlockingKind + node.BlockingPeer both set (the
//     §7.30.3 D1 payload parse already sentinel-filters the peer at compile);
//     the holding site rides along when parsed;
//   - binder shape: a critical_blocking row (exact typed Predicate) whose
//     TypeToken is binder_wait and whose Object names a real peer thread
//     (the critical_blocking publication puts the peer label in Object; the
//     unknown-thread sentinel is excluded by the exact token match).
//
// Soft guidance only — nothing gates on these rows. Dedupe follows the R4-3
// discipline: a typed identity key here (shape + canonical peer + site), the
// caller's verbatim-text layer on top.
func runtimeTraceNextStepResolvedPeerHints(ledger types.ObservationLedger, zh bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, projection := range types.CompileTraceCausalProjectionSet(ledger).Projections {
		if !projection.Active() {
			continue
		}
		for _, node := range runtimeTraceProjResolvedPeerScanNodes(projection) {
			peer, holderSite, lockShape := runtimeTraceNextStepResolvedPeer(node)
			if peer == "" {
				continue
			}
			key := strings.Join([]string{
				fmt.Sprintf("%t", lockShape),
				runtimeTraceCausalProjectionCanonicalNode(peer),
				runtimeTraceCausalProjectionCanonicalNode(holderSite),
			}, "\x00")
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, runtimeTraceNextStepResolvedPeerText(peer, holderSite, lockShape, zh))
		}
	}
	return out
}

// runtimeTraceProjResolvedPeerScanNodes walks every node bucket of one
// compiled projection in deterministic bucket-then-index order.
func runtimeTraceProjResolvedPeerScanNodes(projection types.TraceCausalProjection) []types.TraceCausalProjectionNode {
	var nodes []types.TraceCausalProjectionNode
	if projection.PrimaryRootCause != nil {
		nodes = append(nodes, *projection.PrimaryRootCause)
	}
	nodes = append(nodes, projection.PrimaryRootCauses...)
	nodes = append(nodes, projection.OnChainCauses...)
	nodes = append(nodes, projection.SupportingHops...)
	nodes = append(nodes, projection.AdjacentCauses...)
	nodes = append(nodes, projection.BackgroundCauses...)
	return nodes
}

// runtimeTraceNextStepResolvedPeer extracts the resolved blocking peer of one
// projection node, "" when the node carries none. lockShape distinguishes the
// holder wording (持有者/lock holder) from the binder wording (对端/binder
// peer).
func runtimeTraceNextStepResolvedPeer(node types.TraceCausalProjectionNode) (peer, holderSite string, lockShape bool) {
	if strings.TrimSpace(node.BlockingKind) != "" && strings.TrimSpace(node.BlockingPeer) != "" {
		// BLK §15.C ③: on a holder-subject rank node BlockingPeer is the blocked
		// WAITER, not a holder to drill into — naming it "对持有者 <waiter>"
		// reverses the direction (the q6 next-step-1 misdirection). The true
		// holder to drill into is this node's own SUBJECT. After the §15.C ①
		// single-publication fold the waiter-subject critical_blocking twin is
		// dropped, so this rank node is the SOLE carrier of the lock fact and
		// the holder drilldown must be synthesized here from the subject. A
		// sentinel/unresolved subject yields no row rather than a wrong identity.
		if node.BlockingSubjectIsHolder {
			subject := strings.TrimSpace(node.Subject)
			if !runtimeTraceCausalProjectionKnownSubject(subject) {
				return "", "", false
			}
			return subject, strings.TrimSpace(node.BlockingHolderSite), true
		}
		return strings.TrimSpace(node.BlockingPeer), strings.TrimSpace(node.BlockingHolderSite), true
	}
	if strings.TrimSpace(node.Predicate) == "critical_blocking" &&
		runtimeTraceCausalProjectionCanonicalNode(node.TypeToken) == "binder_wait" {
		object := strings.TrimSpace(node.Object)
		if object != "" && !runtimeTraceCausalProjectionUnknownSentinel(object) {
			return object, "", false
		}
	}
	return "", "", false
}

// runtimeTraceNextStepResolvedPeerText renders one named holder/peer
// drilldown row. Identity strings only (thread label + holding site) — the
// row carries no scalar claims about either side.
func runtimeTraceNextStepResolvedPeerText(peer, holderSite string, lockShape, zh bool) string {
	if zh {
		role := "对端"
		if lockShape {
			role = "持有者"
		}
		// PTV8-RCR-B (UXA 域C #15 两 verify 交集, 2026-07-08). EVOLUTION
		// RECORD: 「trace_query view=」内部调用语法离开客户面(§22.3 N1 零
		// 内部名同向),重叠窗→重叠的查询窗(窗族);持有点完整签名逐字保留
		// (C-verify: 可 grep 的硬事实,准确性>简洁;短签名提取留账待裁).
		if holderSite != "" {
			return fmt.Sprintf("对%s %s(持有点 %s)在重叠的查询窗内查看其线程时间线与唤醒链(thread_timeline/wakeup_chain)", role, peer, holderSite)
		}
		return fmt.Sprintf("对%s %s 在重叠的查询窗内查看其线程时间线与唤醒链(thread_timeline/wakeup_chain)", role, peer)
	}
	role := "binder peer"
	if lockShape {
		role = "lock holder"
	}
	if holderSite != "" {
		return fmt.Sprintf("Inspect the thread timeline and wakeup chain (thread_timeline/wakeup_chain) of the %s %s over the overlapping query window (holding site %s)", role, peer, holderSite)
	}
	return fmt.Sprintf("Inspect the thread timeline and wakeup chain (thread_timeline/wakeup_chain) of the %s %s over the overlapping query window", role, peer)
}

// runtimeTraceCaptureIdentityBasename is the display basename of a canonical
// (slash-normalised) capture identity path.
func runtimeTraceCaptureIdentityBasename(canon string) string {
	canon = strings.Trim(strings.TrimSpace(canon), "/")
	if i := strings.LastIndex(canon, "/"); i >= 0 {
		return canon[i+1:]
	}
	return canon
}

// A2 件1 (§29.174 UX-13, 2026-07-21) TOMBSTONE: the per-record next-step
// template lane (runtimeTraceNextStepFromObservationRecord, the fixed
// next_step_kind→ZH sentence table runtimeTraceNextStepChineseText — the
// former line-7088 domain — the dynamic runnable variant and the typed
// record dedupe key) retired with the batch; the ◎ direction-action lane in
// answer_document_mutation_runtime_nextstep_a2.go replaces the population.
// The engine keeps publishing next_step/next_step_kind wire notes (wire
// compatibility; registry rows demoted to display_only).

func runtimeTraceObservationRichNoteValue(notes []string, key string) string {
	prefix := strings.TrimSpace(key) + "="
	if prefix == "=" {
		return ""
	}
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if !strings.HasPrefix(note, prefix) {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(note, prefix))
	}
	return ""
}

func runtimeTraceMergeSummaryMetricTokens(values map[string]string, summary string, keys []string) {
	if values == nil || strings.TrimSpace(summary) == "" || len(keys) == 0 {
		return
	}
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key != "" {
			allowed[key] = struct{}{}
		}
	}
	for _, token := range strings.FieldsFunc(summary, runtimeTraceMetricSummaryTokenSeparator) {
		key, value, ok := strings.Cut(strings.TrimSpace(token), "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if _, exists := allowed[key]; !exists || values[key] != "" {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'()[]{}<>`)
		if value == "" {
			continue
		}
		values[key] = value
	}
}

func runtimeTraceMetricSummaryTokenSeparator(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', ';', '；', ',', '，', '、', '。':
		return true
	default:
		return false
	}
}

func materializeRuntimeTracePerfQualityBlock(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil || ctx.Mutable == nil {
		return false
	}
	if len(doc.Blocks) >= maxBlocksPerDoc {
		logging.Warning("[answer_document] runtime trace perf quality block skipped: document already at the %d-block cap", maxBlocksPerDoc)
		return false
	}
	if answerDocumentHasRuntimeTraceSystemBlockID(doc, "runtime_trace_perf_quality") {
		return false
	}
	items := runtimeTracePerfQualityItems(doc, ctx)
	if len(items) == 0 {
		return false
	}
	block := types.AnswerBlock{
		ID:    "runtime_trace_perf_quality",
		Kind:  types.BlockBulletList,
		Title: "Perf 证据质量",
		Items: items,
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimExternalObservation,
		}},
		FacetIDs: []string{"observed_artifact_fact"},
	}
	markRuntimeTraceSystemBlock(&block)
	insertAt := answerDocumentInsertionIndexBeforeRuntimeTraceDetails(doc)
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{})
	copy(doc.Blocks[insertAt+1:], doc.Blocks[insertAt:])
	doc.Blocks[insertAt] = block
	return true
}

func runtimeTracePerfQualityItems(doc *types.AnswerDocumentV2, ctx *types.BusContext) []types.AnswerBlockItem {
	if doc == nil || ctx == nil {
		return nil
	}
	visible := answerDocumentVisibleSurfaceForRuntimeTrace(doc)
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, 64))
	seen := make(map[string]bool)
	var out []types.AnswerBlockItem
	for _, record := range ledger.Records {
		text := runtimeTracePerfQualityText(record)
		if text == "" || strings.Contains(visible, text) || seen[text] {
			continue
		}
		seen[text] = true
		label := strings.TrimSpace(record.Subject)
		if label == "" {
			label = "perf sample"
		}
		out = append(out, types.AnswerBlockItem{
			ID:          fmt.Sprintf("runtime_trace_perf_quality_%d", len(out)+1),
			Label:       label,
			Text:        text,
			CitationRef: -1,
		})
		if len(out) >= 2 {
			break
		}
	}
	return out
}

func runtimeTracePerfQualityText(record types.ObservationRecord) string {
	if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
		!types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
		strings.TrimSpace(record.Predicate) != "perf_sample_top_symbol" {
		return ""
	}
	quality := runtimeTraceObservationRichNoteValue(record.RichNotes, types.TraceNoteKeyPerfQuality)
	if quality == "" {
		return ""
	}
	values := map[string]string{}
	runtimeTraceMergeSummaryMetricTokens(values, quality, []string{"source", "sample_kind", "weight_unit", "symbolization", "symbolization_status", "cpu_known", "cpu_unknown", "sample_cpu_scope", "clock", "clock_confidence", "callchain_status"})
	dso := runtimeTraceObservationRichNoteValue(record.RichNotes, types.TraceNoteKeyDSO)
	parts := make([]string, 0, 6)
	if values["source"] != "" {
		parts = append(parts, "source="+values["source"])
	}
	if values["sample_kind"] != "" {
		parts = append(parts, "sample_kind="+values["sample_kind"])
	}
	if values["weight_unit"] != "" {
		parts = append(parts, "weight_unit="+values["weight_unit"])
	}
	symbolization := firstNonEmptyRuntimeTrace(values["symbolization_status"], values["symbolization"])
	if symbolization != "" {
		parts = append(parts, "symbolization_status="+symbolization)
	}
	if values["cpu_known"] != "" {
		parts = append(parts, "cpu_known="+values["cpu_known"])
	}
	if values["cpu_unknown"] != "" {
		parts = append(parts, "cpu_unknown="+values["cpu_unknown"])
	}
	if values["sample_cpu_scope"] != "" {
		parts = append(parts, "sample_cpu_scope="+values["sample_cpu_scope"])
	}
	if values["clock"] != "" {
		parts = append(parts, "clock="+values["clock"])
	}
	if values["clock_confidence"] != "" {
		parts = append(parts, "clock_confidence="+values["clock_confidence"])
	}
	if values["callchain_status"] != "" {
		parts = append(parts, "callchain_status="+values["callchain_status"])
	}
	if dso != "" {
		parts = append(parts, "dso="+dso)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ")
}

func firstNonEmptyRuntimeTrace(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func materializeRuntimeTraceObservationBlock(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil || ctx.Mutable == nil {
		return false
	}
	// Cap headroom guard (same pattern as
	// normalizeAggregateNegativeProofSupplement): this advisory block
	// is system-fabricated and runs immediately before the
	// maxBlocksPerDoc hard gate — inserting it into an at-cap doc
	// would reject the emit with a block count the model never
	// produced. Skipping at cap is lossy-but-safe.
	if len(doc.Blocks) >= maxBlocksPerDoc {
		logging.Warning("[answer_document] runtime trace observation block skipped: document already at the %d-block cap", maxBlocksPerDoc)
		return false
	}
	perf := ctx.Mutable.PerfTrace()
	if perf == nil || !perf.IsExternalSource() {
		return false
	}
	items := runtimeTraceObservationItems(doc, perf)
	if len(items) == 0 {
		return false
	}
	if answerDocumentHasRuntimeTraceSystemBlockID(doc, "runtime_trace_facts") {
		return false
	}
	block := types.AnswerBlock{
		ID:    "runtime_trace_facts",
		Kind:  types.BlockBulletList,
		Title: "Trace 关键事实",
		Items: items,
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimExternalObservation,
		}},
		FacetIDs: []string{"observed_artifact_fact"},
	}
	markRuntimeTraceSystemBlock(&block)
	insertAt := answerDocumentInsertionIndexBeforeRuntimeTraceDetails(doc)
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{})
	copy(doc.Blocks[insertAt+1:], doc.Blocks[insertAt:])
	doc.Blocks[insertAt] = block
	return true
}

func answerDocumentInsertionIndexBeforeCaveat(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return 0
	}
	for i, existing := range doc.Blocks {
		if existing.Kind == types.BlockCaveat {
			return i
		}
	}
	return len(doc.Blocks)
}

// answerDocumentInsertionIndexBeforeRuntimeTraceDetails returns the boundary
// between decision/summary surfaces and the causal projection's drill-down
// tier. Runtime-derived blocks that help the reader decide or act (semantic
// optimization points, metric snapshot, next steps, evidence quality and key
// facts) insert here, so per-node attributes and line-by-line evidence cannot
// bury them. The exact-id classifier below intentionally follows the same
// prefix+suffix discipline as runtimeTraceCausalProjectionFamilyBlockID: a
// model-authored lookalike must not become a structural ordering signal.
func answerDocumentInsertionIndexBeforeRuntimeTraceDetails(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return 0
	}
	fallback := answerDocumentInsertionIndexBeforeCaveat(doc)
	for i := 0; i < fallback; i++ {
		if RuntimeTraceSystemBlock(doc.Blocks[i]) &&
			runtimeTraceCausalProjectionDetailBlockID(strings.TrimSpace(doc.Blocks[i].ID)) {
			return i
		}
	}
	return fallback
}

// runtimeTraceCausalProjectionDetailBlockID recognizes the LOSSLESS drill-down
// ids only. EVOLUTION RECORD (审计 #63 回裁, §29.25 处置委托 + §29.26 待主会话
// 落账, 2026-07-10): the "_detail" key-metric table left this set — per the
// §29.10-3 user ruling ("投影树(含头/覆盖句/关键指标)依次全部优先显示") it is
// part of each projection's PRIORITY unit and pairs with its lead, so it is
// neither a supplement insertion boundary, nor an eviction candidate, nor a
// tier-8 drill-down block. runtimeTraceCausalProjectionMetricBlockID keeps its
// exact classifier.
func runtimeTraceCausalProjectionDetailBlockID(id string) bool {
	rest, ok := strings.CutPrefix(id, runtimeTraceCausalProjectionBlockIDBase)
	if !ok {
		return false
	}
	switch rest {
	case "_detail_full", "_evidence", "_compare_notes", "_partition":
		return true
	}
	digits, ok := strings.CutPrefix(rest, runtimeTraceCausalProjectionArtifactBlockIDInfix)
	if !ok {
		return false
	}
	i := 0
	for i < len(digits) && digits[i] >= '0' && digits[i] <= '9' {
		i++
	}
	if i == 0 {
		return false
	}
	switch digits[i:] {
	case "_detail_full", "_evidence":
		return true
	default:
		return false
	}
}

// normalizeRuntimeTraceReportHierarchy applies one final, typed stable sort
// after every report materializer/normalizer has run. It never reads titles or
// prose.
//
// EVOLUTION RECORD (审计 #59 收窄, §29.25 处置委托 + §29.26 待主会话落账,
// 2026-07-10): the remote-introduced form re-bucketed EVERY block — model
// Summary/Decision/Scalar jumped to tier 0, all other model blocks sank below
// six system tiers, and a model-authored next_steps-shaped ID was promoted by
// bare ID (contradicting the classifier's own "a model-authored lookalike must
// not become a structural ordering signal" contract). That extended the
// §29.10-3 ruling (projection-cluster-only reorder, 纯块序重排) onto the whole
// model narrative and broke model discourse order (系统不重排模型叙事红线).
// Narrowed here: ONLY system-marked runtime-trace supplements participate in
// the tier sort; every non-system block keeps its original relative order in
// ONE body bucket (trailing model caveats keep the pre-existing
// insert-before-caveat convention and stay behind the system tiers). Within
// the system tiers, each projection lead and its own key-metric "_detail"
// table share ONE tier so the §29.10-3 pairing survives the stable sort.
// The marker prerequisite makes an exact model-authored ID inert even before
// the collision normalizer's deterministic rename.
func normalizeRuntimeTraceReportHierarchy(doc *types.AnswerDocumentV2) int {
	if doc == nil || len(doc.Blocks) < 2 {
		return 0
	}
	hasRuntimeTrace := false
	for _, block := range doc.Blocks {
		if block.SystemGeneratedKind.IsRuntimeTraceSupplement() {
			hasRuntimeTrace = true
			break
		}
	}
	if !hasRuntimeTrace {
		return 0
	}
	const tierCount = 10
	buckets := make([][]types.AnswerBlock, tierCount)
	for _, block := range doc.Blocks {
		tier := runtimeTraceReportHierarchyTier(block)
		buckets[tier] = append(buckets[tier], block)
	}
	reordered := make([]types.AnswerBlock, 0, len(doc.Blocks))
	for _, bucket := range buckets {
		reordered = append(reordered, bucket...)
	}
	moved := 0
	for i := range reordered {
		if reordered[i].ID != doc.Blocks[i].ID {
			moved++
		}
	}
	if moved > 0 {
		doc.Blocks = reordered
	}
	return moved
}

func runtimeTraceReportHierarchyTier(block types.AnswerBlock) int {
	id := strings.TrimSpace(block.ID)
	if block.SystemGeneratedKind.IsRuntimeTraceSupplement() {
		switch {
		case id == runtimeTraceCausalProjectionPartitionBlockID || block.Kind == types.BlockCaveat:
			return 9
		case id == runtimeTraceCausalProjectionCompareBlockID || runtimeTraceCausalProjectionStandaloneLeadBlockID(id):
			return 1
		case runtimeTraceCausalProjectionMetricBlockID(id):
			// §29.10-3 (审计 #63 回裁): the key-metric table shares its lead's
			// tier — the stable sort preserves the builder's paired
			// lead,_detail,lead,_detail order inside the tier.
			return 1
		case id == "runtime_trace_semantic_optimizations":
			return 2
		case answerDocumentBlockIDIsNextSteps(id):
			return 3
		case id == "runtime_trace_metric_snapshot":
			return 4
		case id == "runtime_trace_perf_quality" || id == "runtime_trace_facts":
			return 5
		case runtimeTraceCausalProjectionDetailBlockID(id):
			return 8
		default:
			return 5
		}
	}
	// 审计 #59 收窄: non-system blocks form ONE order-preserving body bucket —
	// the system never re-orders the model narrative among itself. Trailing
	// disclosure caveats keep the established end-of-report seat (the
	// insert-before-caveat convention predates this sort).
	if block.Kind == types.BlockCaveat {
		return 9
	}
	return 0
}

func runtimeTraceCausalProjectionMetricBlockID(id string) bool {
	rest, ok := strings.CutPrefix(id, runtimeTraceCausalProjectionBlockIDBase)
	if !ok {
		return false
	}
	if rest == "_detail" {
		return true
	}
	digits, ok := strings.CutPrefix(rest, runtimeTraceCausalProjectionArtifactBlockIDInfix)
	if !ok {
		return false
	}
	i := 0
	for i < len(digits) && digits[i] >= '0' && digits[i] <= '9' {
		i++
	}
	return i > 0 && digits[i:] == "_detail"
}

func runtimeTraceObservationItems(doc *types.AnswerDocumentV2, perf *types.PerfBundle) []types.AnswerBlockItem {
	if doc == nil || perf == nil {
		return nil
	}
	visible := answerDocumentVisibleSurfaceForRuntimeTrace(doc)
	seen := make(map[string]bool)
	var out []types.AnswerBlockItem
	for _, obs := range perf.Observations {
		label := runtimeTraceObservationLabel(obs)
		if label == "" {
			continue
		}
		summary := strings.TrimSpace(obs.Summary)
		if summary == "" {
			continue
		}
		if strings.Contains(visible, summary) || seen[summary] {
			continue
		}
		seen[summary] = true
		out = append(out, types.AnswerBlockItem{
			ID:          runtimeTraceObservationItemID(label, len(out)+1),
			Label:       label,
			Text:        summary,
			CitationRef: -1,
		})
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func runtimeTraceObservationLabel(obs types.PerfObservation) string {
	switch strings.TrimSpace(obs.Kind) {
	case "time_semantics":
		return "时间单位"
	case "priority_semantics":
		return "平台优先级语义"
	case "priority_semantics_normalized":
		return "优先级归一化"
	default:
		return ""
	}
}

var runtimeHarmonyPrioClassRE = regexp.MustCompile(`prio=([0-9]+)/(ohos_(?:rt|cfs))`)

// normalizePriorityInversionCandidateAnswerSurface keeps model-authored prose
// at the strength of the deterministic trace wire. A relation-only edge is a
// low-priority dependency candidate with no measured inversion impact; a
// positive ranked effective impact is a measured inversion candidate; explicit
// holder/waiter authority is confirmed. Authority is subject/edge scoped: an
// edge elsewhere in the report must never weaken an independently measured or
// confirmed claim. Rewrites are deliberately limited to a small closed phrase
// set and never touch deterministic system blocks.
func normalizePriorityInversionCandidateAnswerSurface(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	structural, measured, confirmed := runtimeTracePriorityInversionAuthorities(ctx)
	if doc == nil || len(structural)+len(measured) == 0 {
		return 0
	}
	fixed := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.SystemGeneratedKind != "" || block.Kind == types.BlockDiagram {
			continue
		}
		fixed += normalizePriorityInversionCandidateString(&block.Title, structural, measured, confirmed)
		fixed += normalizePriorityInversionCandidateString(&block.Text, structural, measured, confirmed)
		for j := range block.Items {
			item := &block.Items[j]
			fixed += normalizePriorityInversionCandidateString(&item.Label, structural, measured, confirmed)
			fixed += normalizePriorityInversionCandidateString(&item.Text, structural, measured, confirmed)
			for k := range item.Cells {
				fixed += normalizePriorityInversionCandidateString(&item.Cells[k], structural, measured, confirmed)
			}
		}
	}
	for i := range doc.Caveats {
		fixed += normalizePriorityInversionCandidateString(&doc.Caveats[i], structural, measured, confirmed)
	}
	return fixed
}

type runtimeTracePriorityInversionAuthority struct {
	subjectTokens  []string
	peerTokens     []string
	requirePeer    bool
	relationTokens []string
}

// runtimeTracePriorityInversionAuthorities returns three subject/edge-scoped
// exact wire authorities:
//   - structural: a lower-priority dependency/waker relation was observed, but
//     no ranked inversion impact was measured for that identity in this window;
//   - measured: root_cause_* published a priority_inversion_* row with a
//     positive effective_impact_ms;
//   - confirmed: a holder/waiter authority explicitly confirmed inversion.
//
// This split is load-bearing. lower_priority_waker/dependency proves a graph
// relation and a risk candidate only; it is not itself elapsed inversion time.
// Subject identity is preferred; an exact typed priority_relation token is the
// fallback for short model labels that omit the thread name. No answer prose is
// mined to create authority.
func runtimeTracePriorityInversionAuthorities(ctx *types.BusContext) (structural, measured, confirmed []runtimeTracePriorityInversionAuthority) {
	if ctx == nil {
		return nil, nil, nil
	}
	for _, result := range ctx.ToolResults {
		if result.ToolName != "trace_query" || !result.Success {
			continue
		}
		for _, record := range result.Observations {
			object := strings.TrimSpace(record.Object)
			isCandidate := runtimeTracePriorityInversionCandidateType(object)
			isConfirmed := object == "priority_inversion_confirmed" ||
				strings.TrimSpace(record.Predicate) == "priority_inversion_confirmed"
			var relation string
			for _, note := range record.RichNotes {
				note = strings.TrimSpace(note)
				switch note {
				case "priority_inversion_candidate=true":
					isCandidate = true
				case "priority_inversion_confirmed=true", "priority_inversion_authority=confirmed_holder_waiter":
					isConfirmed = true
				}
				if value, ok := strings.CutPrefix(note, "priority_relation="); ok {
					relation = strings.TrimSpace(value)
				}
				if value, ok := strings.CutPrefix(note, types.TraceNoteKeyType+"="); ok &&
					runtimeTracePriorityInversionCandidateType(strings.TrimSpace(value)) {
					isCandidate = true
				}
			}
			if !isCandidate && !isConfirmed {
				continue
			}
			authority := runtimeTracePriorityInversionAuthority{
				subjectTokens:  runtimeTracePriorityInversionIdentityTokens(record.Subject),
				relationTokens: runtimeTracePriorityInversionRelationTokens(relation),
			}
			predicate := strings.TrimSpace(record.Predicate)
			if len(authority.subjectTokens) > 0 && object != "" &&
				(predicate == "wakeup_chain_edge" || (isConfirmed && object != "priority_inversion_confirmed")) {
				authority.peerTokens = runtimeTracePriorityInversionIdentityTokens(object)
				authority.requirePeer = true
			}
			if len(authority.subjectTokens) == 0 && len(authority.relationTokens) == 0 {
				// An unbound report-level bit is not precise enough to mutate
				// unrelated model prose. Deterministic system surfaces still carry
				// the candidate semantics independently.
				continue
			}
			if isCandidate {
				if runtimeTracePriorityInversionMeasuredRecord(record) {
					measured = append(measured, authority)
				} else {
					structural = append(structural, authority)
				}
			}
			if isConfirmed {
				confirmed = append(confirmed, authority)
			}
		}
	}
	return structural, measured, confirmed
}

// runtimeTracePriorityInversionMeasuredRecord is the precise measured-impact
// gate. A generic edge bit, a candidate object without a ranked root-cause
// publication, or an absent/zero/non-finite effective value stays structural.
// Same-CPU identity is deliberately NOT part of this gate: the producer's
// broad inversion metric may count an on-chain dependency's runnable time in
// full and a cross-CPU weak-core/supply running deficit.
func runtimeTracePriorityInversionMeasuredRecord(record types.ObservationRecord) bool {
	if !strings.HasPrefix(strings.TrimSpace(record.Predicate), "root_cause_") ||
		!runtimeTracePriorityInversionCandidateType(strings.TrimSpace(record.Object)) {
		return false
	}
	for _, note := range record.RichNotes {
		value, ok := strings.CutPrefix(strings.TrimSpace(note), "effective_impact_ms=")
		if !ok {
			continue
		}
		impact, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return err == nil && !math.IsNaN(impact) && !math.IsInf(impact, 0) && impact > 0
	}
	return false
}

// runtimeTracePriorityInversionCandidateType is the DISPLAY-side inversion
// row-type family single point (UXG-1 M4, 2026-07-12): the literal token pair
// may appear together only here on the display side — everywhere else calls
// this predicate. Interlocked with the engine single point
// (tracequery.RootCauseTypeIsPriorityInversion) by
// TestUXG1InversionFamilyPredicatesInterlocked; local re-enumeration is
// intercepted by the source scan in uxg1_family_predicate_tripwire_test.go.
func runtimeTracePriorityInversionCandidateType(value string) bool {
	switch strings.TrimSpace(value) {
	case "priority_inversion_candidate", "priority_inversion_runnable_wait":
		return true
	default:
		return false
	}
}

// runtimeTracePriorityInversionIdentityTokens derives the display aliases only
// from the typed canonical thread label. The full name-pid form is always
// retained; the comm alias is admitted only by the existing strict
// name-<pure digits> parser. Thus shadowhook-task-64305 can bind the customer's
// shorter "shadowhook-task线程" wording without mining arbitrary prose.
func runtimeTracePriorityInversionIdentityTokens(label string) []string {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}
	tokens := []string{label}
	if comm, _, ok := runtimeTraceProjSplitNamePid(label); ok {
		comm = strings.TrimSpace(comm)
		if comm != "" && comm != label {
			tokens = append(tokens, comm)
		}
	}
	return tokens
}

func runtimeTracePriorityInversionRelationTokens(relation string) []string {
	switch strings.TrimSpace(relation) {
	case "lower_priority_waker":
		return []string{"lower_priority_waker", "低优先级唤醒者"}
	case "lower_priority_dependency":
		return []string{"lower_priority_dependency", "低优先级依赖"}
	default:
		return nil
	}
}

func runtimeTracePriorityInversionAuthorityMatchesLine(authority runtimeTracePriorityInversionAuthority, line string) bool {
	identityMatch := runtimeTracePriorityInversionIdentityMatchesLine(line, authority.subjectTokens)
	if identityMatch && authority.requirePeer {
		identityMatch = runtimeTracePriorityInversionIdentityMatchesLine(line, authority.peerTokens)
	}
	if identityMatch {
		return true
	}
	for _, token := range authority.relationTokens {
		if runtimeTracePriorityInversionLineContainsToken(line, token) {
			return true
		}
	}
	return false
}

func runtimeTracePriorityInversionIdentityMatchesLine(line string, tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	// Token 0 is the full typed identity. Any later token is a comm alias
	// derived by runtimeTraceProjSplitNamePid.
	if runtimeTracePriorityInversionLineContainsToken(line, tokens[0]) {
		return true
	}
	for _, alias := range tokens[1:] {
		if runtimeTracePriorityInversionLineContainsCommAlias(line, alias) {
			return true
		}
	}
	return false
}

func runtimeTracePriorityInversionLineHasAuthority(line string, authorities []runtimeTracePriorityInversionAuthority) bool {
	for _, authority := range authorities {
		if runtimeTracePriorityInversionAuthorityMatchesLine(authority, line) {
			return true
		}
	}
	return false
}

// runtimeTracePriorityInversionLineContainsToken performs a case-folded exact
// token match. ASCII alphanumeric/underscore continuations are rejected so a
// typed worker-20 authority cannot bind model text about worker-200 and a
// lower_priority_waker token cannot match lower_priority_wakerish.
func runtimeTracePriorityInversionLineContainsToken(line, token string) bool {
	line = strings.ToLower(line)
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return false
	}
	for offset := 0; offset <= len(line)-len(token); {
		rel := strings.Index(line[offset:], token)
		if rel < 0 {
			return false
		}
		start := offset + rel
		end := start + len(token)
		beforeOK := start == 0 || !runtimeTracePriorityInversionTokenContinuation(line[start-1])
		afterOK := end == len(line) || !runtimeTracePriorityInversionTokenContinuation(line[end])
		if beforeOK && afterOK {
			return true
		}
		offset = start + 1
	}
	return false
}

// runtimeTracePriorityInversionLineContainsCommAlias accepts a model's shorter
// typed-comm rendering but rejects an occurrence immediately extended into a
// different canonical comm-<pid> label. For example, worker (derived from
// worker-20) binds "worker线程" but not worker-200.
func runtimeTracePriorityInversionLineContainsCommAlias(line, alias string) bool {
	line = strings.ToLower(line)
	alias = strings.ToLower(strings.TrimSpace(alias))
	if alias == "" {
		return false
	}
	for offset := 0; offset <= len(line)-len(alias); {
		rel := strings.Index(line[offset:], alias)
		if rel < 0 {
			return false
		}
		start := offset + rel
		end := start + len(alias)
		beforeOK := start == 0 || !runtimeTracePriorityInversionTokenContinuation(line[start-1])
		afterOK := end == len(line) || !runtimeTracePriorityInversionTokenContinuation(line[end])
		canonicalSuffix := false
		if end < len(line) && line[end] == '-' {
			j := end + 1
			for j < len(line) && line[j] >= '0' && line[j] <= '9' {
				j++
			}
			canonicalSuffix = j > end+1 && (j == len(line) || !runtimeTracePriorityInversionTokenContinuation(line[j]))
		}
		if beforeOK && afterOK && !canonicalSuffix {
			return true
		}
		offset = start + 1
	}
	return false
}

func runtimeTracePriorityInversionTokenContinuation(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

func normalizePriorityInversionCandidateString(value *string, structural, measured, confirmed []runtimeTracePriorityInversionAuthority) int {
	if value == nil || *value == "" {
		return 0
	}
	before := *value
	lines := strings.Split(*value, "\n")
	for i, line := range lines {
		if runtimeTracePriorityInversionLineHasAuthority(line, confirmed) {
			lines[i] = line
			continue
		}
		if runtimeTracePriorityInversionLineHasAuthority(line, measured) {
			lines[i] = normalizeMeasuredPriorityInversionCandidateLine(line)
			continue
		}
		if runtimeTracePriorityInversionLineHasAuthority(line, structural) {
			lines[i] = normalizeStructuralPriorityInversionCandidateLine(line)
			continue
		}
		lines[i] = line
	}
	*value = strings.Join(lines, "\n")
	if *value == before {
		return 0
	}
	return 1
}

func normalizeMeasuredPriorityInversionCandidateLine(line string) string {
	// Idempotence: a measured line already carrying the candidate caliber
	// stays byte-identical. A measured candidate remains a real ranked
	// candidate even when its impact arose across CPUs.
	lower := strings.ToLower(line)
	if strings.Contains(line, "优先级反转候选") ||
		strings.Contains(lower, "priority-inversion candidate") ||
		strings.Contains(lower, "priority inversion candidate") {
		return line
	}
	replacements := []struct{ old, new string }{
		{"存在优先级反转", "存在优先级反转候选"},
		{"发生优先级反转", "出现优先级反转候选"},
		{"产生优先级反转", "产生优先级反转候选"},
		{"优先级反转：低优先级唤醒者", "优先级反转候选：低优先级唤醒者"},
		{"优先级反转:低优先级唤醒者", "优先级反转候选:低优先级唤醒者"},
		{"优先级反转是", "优先级反转候选是"},
		{"优先级反转为", "优先级反转候选为"},
		{"优先级反转导致", "优先级反转候选可能导致"},
		{"根因是优先级反转", "根因候选是优先级反转"},
		{"there is a priority inversion", "there is a priority-inversion candidate"},
		{"There is a priority inversion", "There is a priority-inversion candidate"},
		{"priority inversion exists", "a priority-inversion candidate exists"},
		{"Priority inversion exists", "A priority-inversion candidate exists"},
		{"caused a priority inversion", "produced a priority-inversion candidate"},
		{"Caused a priority inversion", "Produced a priority-inversion candidate"},
		{"priority inversion is the on-chain root cause", "priority-inversion candidate is the leading on-chain candidate"},
		{"Priority inversion is the on-chain root cause", "Priority-inversion candidate is the leading on-chain candidate"},
		{"priority inversion caused the frame", "priority-inversion candidate may explain the frame"},
		{"Priority inversion caused the frame", "Priority-inversion candidate may explain the frame"},
	}
	for _, replacement := range replacements {
		line = strings.ReplaceAll(line, replacement.old, replacement.new)
	}
	return line
}

var (
	// These two closed shapes repair the concrete overclaim seen in the
	// 20260710-163301 customer report. They run only behind the typed structural
	// authority above; unrelated prose cannot activate them.
	runtimeTraceStructuralInversionBlockingClauseZH = regexp.MustCompile(`，阻塞了高优先级的[^。；\n]*?——存在优先级反转(?:候选)?(?:（lower_priority_(?:waker|dependency)）|\(lower_priority_(?:waker|dependency)\))`)
	runtimeTraceStructuralInversionIndirectBlockZH  = regexp.MustCompile(`，[A-Za-z0-9_.:-]+ 被间接阻塞`)
)

func normalizeStructuralPriorityInversionCandidateLine(line string) string {
	// The dependency edge is useful evidence, but it carries no measured
	// inversion duration. Remove the report's specific "blocked high-priority
	// thread" leap before applying the general closed phrase table.
	line = runtimeTraceStructuralInversionBlockingClauseZH.ReplaceAllString(line,
		"；该低优先级关系仅构成依赖候选，本窗未测得优先级反转影响")
	line = runtimeTraceStructuralInversionIndirectBlockZH.ReplaceAllString(line, "")

	replacements := []struct{ old, new string }{
		{"优先级反转候选：低优先级唤醒者", "低优先级依赖候选：低优先级唤醒者（本窗未测得反转影响）"},
		{"优先级反转候选:低优先级唤醒者", "低优先级依赖候选:低优先级唤醒者（本窗未测得反转影响）"},
		{"优先级反转：低优先级唤醒者", "低优先级依赖候选：低优先级唤醒者（本窗未测得反转影响）"},
		{"优先级反转:低优先级唤醒者", "低优先级依赖候选:低优先级唤醒者（本窗未测得反转影响）"},
		{"存在优先级反转候选", "存在低优先级依赖候选，但本窗未测得优先级反转影响"},
		{"出现优先级反转候选", "出现低优先级依赖候选，但本窗未测得优先级反转影响"},
		{"产生优先级反转候选", "产生低优先级依赖候选，但本窗未测得优先级反转影响"},
		{"存在优先级反转", "存在低优先级依赖候选，但本窗未测得优先级反转影响"},
		{"发生优先级反转", "出现低优先级依赖候选，但本窗未测得优先级反转影响"},
		{"产生优先级反转", "产生低优先级依赖候选，但本窗未测得优先级反转影响"},
		// 审计 #61: the 候选-suffixed model phrasings MUST precede their bare
		// prefixes (the 存在/出现/产生 families above follow the same
		// longer-key-first discipline) — otherwise the prefix entry rewrites
		// "根因是优先级反转候选" into "结构上存在低优先级依赖候选候选" (doubled
		// 候选 on the primary answer surface).
		{"根因是优先级反转候选", "结构上存在低优先级依赖候选"},
		{"根因候选是优先级反转候选", "结构上存在低优先级依赖候选"},
		{"根因是优先级反转", "结构上存在低优先级依赖候选"},
		{"根因候选是优先级反转", "结构上存在低优先级依赖候选"},
		{"优先级反转导致", "低优先级依赖可能关联"},
		{"优先级反转候选可能导致", "低优先级依赖可能关联"},
		{"there is a priority-inversion candidate", "there is a lower-priority dependency candidate, but no priority-inversion impact was measured in this window"},
		{"There is a priority-inversion candidate", "There is a lower-priority dependency candidate, but no priority-inversion impact was measured in this window"},
		{"there is a priority inversion candidate", "there is a lower-priority dependency candidate, but no priority-inversion impact was measured in this window"},
		{"There is a priority inversion candidate", "There is a lower-priority dependency candidate, but no priority-inversion impact was measured in this window"},
		{"there is a priority inversion", "there is a lower-priority dependency candidate, but no priority-inversion impact was measured in this window"},
		{"There is a priority inversion", "There is a lower-priority dependency candidate, but no priority-inversion impact was measured in this window"},
		{"priority inversion exists", "a lower-priority dependency candidate exists, but no priority-inversion impact was measured in this window"},
		{"Priority inversion exists", "A lower-priority dependency candidate exists, but no priority-inversion impact was measured in this window"},
		{"priority inversion caused the frame", "the lower-priority dependency is structural only; no priority-inversion impact was measured in this window"},
		{"Priority inversion caused the frame", "The lower-priority dependency is structural only; no priority-inversion impact was measured in this window"},
	}
	for _, replacement := range replacements {
		line = strings.ReplaceAll(line, replacement.old, replacement.new)
	}
	return line
}

type runtimeTraceLowCoverageLeadAuthority struct {
	subjectTokens []string
}

// normalizeRuntimeTraceLowCoverageRootCauseSurface prevents model-authored
// prose from promoting a small, typed explained slice into a whole-frame
// verdict. The coverage arithmetic and comparability decision come only from
// runtimeTraceProjCoverageVerdictFor — the same helper consumed by the window
// renderer. The mutation is wording-only and subject-bound to the exact lead
// selected from that projection.
func normalizeRuntimeTraceLowCoverageRootCauseSurface(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil {
		return 0
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	set := types.CompileTraceCausalProjectionSet(ledger)
	authorities := make([]runtimeTraceLowCoverageLeadAuthority, 0, len(set.Projections))
	seen := map[string]bool{}
	for _, projection := range set.Projections {
		if !projection.Active() {
			continue
		}
		model := buildRuntimeTraceProjTreeModel(projection, nil, true)
		if !runtimeTraceProjCoverageVerdictFor(projection, model).LowCoverage() {
			continue
		}
		lead, _ := runtimeTraceProjLeadSelect(projection, model)
		if lead == nil || !types.TraceCausalProjectionKnownSubject(lead.Subject) {
			continue
		}
		tokens := runtimeTracePriorityInversionIdentityTokens(lead.Subject)
		if len(tokens) == 0 {
			continue
		}
		key := strings.Join(tokens, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		authorities = append(authorities, runtimeTraceLowCoverageLeadAuthority{subjectTokens: tokens})
	}
	return normalizeRuntimeTraceLowCoverageRootCauseSurfaceWithAuthorities(doc, authorities)
}

func normalizeRuntimeTraceLowCoverageRootCauseSurfaceForProjection(doc *types.AnswerDocumentV2,
	projection types.TraceCausalProjection, model runtimeTraceProjTreeModel) int {
	if doc == nil || !runtimeTraceProjCoverageVerdictFor(projection, model).LowCoverage() {
		return 0
	}
	lead, _ := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || !types.TraceCausalProjectionKnownSubject(lead.Subject) {
		return 0
	}
	tokens := runtimeTracePriorityInversionIdentityTokens(lead.Subject)
	if len(tokens) == 0 {
		return 0
	}
	return normalizeRuntimeTraceLowCoverageRootCauseSurfaceWithAuthorities(doc,
		[]runtimeTraceLowCoverageLeadAuthority{{subjectTokens: tokens}})
}

func normalizeRuntimeTraceLowCoverageRootCauseSurfaceWithAuthorities(doc *types.AnswerDocumentV2,
	authorities []runtimeTraceLowCoverageLeadAuthority) int {
	if doc == nil || len(authorities) == 0 {
		return 0
	}
	fixed := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.SystemGeneratedKind != "" || block.Kind == types.BlockDiagram {
			continue
		}
		fixed += normalizeRuntimeTraceLowCoverageRootCauseString(&block.Title, authorities)
		fixed += normalizeRuntimeTraceLowCoverageRootCauseString(&block.Text, authorities)
		for j := range block.Items {
			item := &block.Items[j]
			fixed += normalizeRuntimeTraceLowCoverageRootCauseString(&item.Label, authorities)
			fixed += normalizeRuntimeTraceLowCoverageRootCauseString(&item.Text, authorities)
			for k := range item.Cells {
				fixed += normalizeRuntimeTraceLowCoverageRootCauseString(&item.Cells[k], authorities)
			}
		}
	}
	for i := range doc.Caveats {
		fixed += normalizeRuntimeTraceLowCoverageRootCauseString(&doc.Caveats[i], authorities)
	}
	return fixed
}

func runtimeTraceLowCoverageLeadMatchesLine(line string, authorities []runtimeTraceLowCoverageLeadAuthority) bool {
	for _, authority := range authorities {
		if runtimeTracePriorityInversionIdentityMatchesLine(line, authority.subjectTokens) {
			return true
		}
	}
	return false
}

func normalizeRuntimeTraceLowCoverageRootCauseString(value *string, authorities []runtimeTraceLowCoverageLeadAuthority) int {
	if value == nil || *value == "" {
		return 0
	}
	before := *value
	lines := strings.Split(*value, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(line, "当前已解释部分中最大候选") ||
			strings.Contains(lower, "largest candidate within the currently explained portion") ||
			!runtimeTraceLowCoverageLeadMatchesLine(line, authorities) ||
			runtimeTraceLowCoverageRootClaimNegated(line) {
			continue
		}
		replacements := []struct{ old, new string }{
			{"是导致整帧丢帧的核心原因", "是当前已解释部分中最大候选"},
			{"导致了整帧丢帧", "是当前已解释部分中最大候选"},
			{"导致整帧丢帧", "是当前已解释部分中最大候选"},
			{"导致了这一帧丢帧", "是当前已解释部分中最大候选"},
			{"导致这一帧丢帧", "是当前已解释部分中最大候选"},
			{"整帧核心原因", "当前已解释部分中最大候选"},
			{"整帧核心根因", "当前已解释部分中最大候选"},
			{"整帧主根因", "当前已解释部分中最大候选"},
			{"本帧核心原因", "当前已解释部分中最大候选"},
			{"本帧主根因", "当前已解释部分中最大候选"},
			{"核心原因是", "当前已解释部分中最大候选是"},
			{"核心根因是", "当前已解释部分中最大候选是"},
			{"主根因是", "当前已解释部分中最大候选是"},
			{"是核心原因", "是当前已解释部分中最大候选"},
			{"是核心根因", "是当前已解释部分中最大候选"},
			{"是主根因", "是当前已解释部分中最大候选"},
			{"核心原因：", "当前已解释部分中最大候选："},
			{"核心原因:", "当前已解释部分中最大候选:"},
			{"核心根因：", "当前已解释部分中最大候选："},
			{"核心根因:", "当前已解释部分中最大候选:"},
			{"主根因：", "当前已解释部分中最大候选："},
			{"主根因:", "当前已解释部分中最大候选:"},
			{"caused the frame drop", "is the largest candidate within the currently explained portion"},
			{"Caused the frame drop", "Is the largest candidate within the currently explained portion"},
			{"caused this frame drop", "is the largest candidate within the currently explained portion"},
			{"Caused this frame drop", "Is the largest candidate within the currently explained portion"},
			{"caused the frame", "is the largest candidate within the currently explained portion"},
			{"Caused the frame", "Is the largest candidate within the currently explained portion"},
			{"is the primary root cause of the frame", "is the largest candidate within the currently explained portion"},
			{"is the main root cause of the frame", "is the largest candidate within the currently explained portion"},
			{"the primary root cause is", "the largest candidate within the currently explained portion is"},
			{"The primary root cause is", "The largest candidate within the currently explained portion is"},
			{"the main root cause is", "the largest candidate within the currently explained portion is"},
			{"The main root cause is", "The largest candidate within the currently explained portion is"},
			{"is the main root cause", "is the largest candidate within the currently explained portion"},
			{"is a primary root cause", "is the largest candidate within the currently explained portion"},
			{"primary root cause:", "largest candidate within the currently explained portion:"},
			{"Primary root cause:", "Largest candidate within the currently explained portion:"},
			{"main root cause:", "largest candidate within the currently explained portion:"},
			{"Main root cause:", "Largest candidate within the currently explained portion:"},
		}
		for _, replacement := range replacements {
			line = strings.ReplaceAll(line, replacement.old, replacement.new)
		}
		lines[i] = line
	}
	*value = strings.Join(lines, "\n")
	if *value == before {
		return 0
	}
	return 1
}

func runtimeTraceLowCoverageRootClaimNegated(line string) bool {
	lower := strings.ToLower(line)
	for _, token := range []string{
		"未导致", "没有导致", "并未导致", "不是主根因", "并非主根因", "不是核心原因", "并非核心原因",
		"did not cause", "didn't cause", "does not cause", "is not the primary root cause", "is not the main root cause",
	} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func normalizeHarmonyPriorityAnswerSurface(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || !runtimeAnswerHasHarmonyPriorityAuthority(ctx) {
		return 0
	}
	classMap := runtimeHarmonyPriorityClassMap(ctx)
	fixed := 0
	for i := range doc.Blocks {
		fixed += normalizeHarmonyPriorityString(&doc.Blocks[i].Title, "", classMap)
		fixed += normalizeHarmonyPriorityString(&doc.Blocks[i].Text, "", classMap)
		for j := range doc.Blocks[i].Items {
			item := &doc.Blocks[i].Items[j]
			authoritySurface := strings.Join([]string{item.Label, item.Text, strings.Join(item.Cells, " ")}, " ")
			fixed += normalizeHarmonyPriorityString(&item.Text, authoritySurface, classMap)
			for k := range item.Cells {
				fixed += normalizeHarmonyPriorityString(&item.Cells[k], authoritySurface, classMap)
			}
		}
	}
	for i := range doc.Caveats {
		fixed += normalizeHarmonyPriorityString(&doc.Caveats[i], "", classMap)
	}
	return fixed
}

func runtimeAnswerHasHarmonyPriorityAuthority(ctx *types.BusContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	perf := ctx.Mutable.PerfTrace()
	if perf == nil || !runtimePerfTraceSourceIsHarmony(perf.Meta.Source) {
		return false
	}
	for _, obs := range perf.Observations {
		switch strings.TrimSpace(obs.Kind) {
		case "priority_semantics", "priority_semantics_normalized":
			return true
		}
		for _, tag := range obs.Tags {
			switch strings.TrimSpace(tag) {
			case "harmony_priority", "harmony_priority_normalized":
				return true
			}
		}
	}
	return false
}

func runtimeHarmonyPriorityClassMap(ctx *types.BusContext) map[int]string {
	out := map[int]string{}
	if ctx == nil || ctx.Mutable == nil {
		return out
	}
	perf := ctx.Mutable.PerfTrace()
	if perf == nil {
		return out
	}
	addSurface := func(surface string) {
		for _, match := range runtimeHarmonyPrioClassRE.FindAllStringSubmatch(surface, -1) {
			if len(match) < 3 {
				continue
			}
			prio, err := strconv.Atoi(match[1])
			if err != nil || prio <= 0 {
				continue
			}
			out[prio] = match[2]
		}
	}
	for _, obs := range perf.Observations {
		if obs.Kind == "priority_semantics" || obs.Kind == "priority_semantics_normalized" {
			addSurface(strings.Join([]string{obs.Subject, obs.Summary, obs.Evidence, strings.Join(obs.Tags, " ")}, " "))
			continue
		}
		for _, tag := range obs.Tags {
			if tag == "harmony_priority" || tag == "harmony_priority_normalized" {
				addSurface(strings.Join([]string{obs.Subject, obs.Summary, obs.Evidence, strings.Join(obs.Tags, " ")}, " "))
				break
			}
		}
	}
	return out
}

func runtimePerfTraceSourceIsHarmony(source string) bool {
	switch strings.TrimSpace(source) {
	case "hitrace", "harmony_hitrace":
		return true
	default:
		return false
	}
}

func normalizeHarmonyPriorityString(s *string, authoritySurface string, classMap map[int]string) int {
	if s == nil || strings.TrimSpace(*s) == "" {
		return 0
	}
	rangeFixed := 0
	if normalized := normalizeHarmonyPriorityRuleRange(*s); normalized != *s {
		*s = normalized
		rangeFixed = 1
	}
	class, ok := uniqueHarmonyPriorityClass(strings.Join([]string{authoritySurface, *s}, " "), classMap)
	if !ok {
		return rangeFixed
	}
	hasPriorityAuthority := hasHarmonyPriorityClassSurface(authoritySurface, classMap) || hasHarmonyPriorityClassSurface(*s, classMap)
	lines := strings.Split(*s, "\n")
	fixed := 0
	for i, line := range lines {
		if line == "" || (harmonyPriorityLineIsRule(line) && !hasPriorityAuthority) {
			continue
		}
		before := line
		line = normalizeHarmonyPriorityRuleRange(line)
		lines[i] = normalizeHarmonyPriorityLine(line, class, classMap)
		if lines[i] != before {
			fixed++
		}
	}
	if fixed > 0 {
		*s = strings.Join(lines, "\n")
	}
	return rangeFixed + fixed
}

func uniqueHarmonyPriorityClass(surface string, classMap map[int]string) (string, bool) {
	classes := harmonyPriorityClassesInSurface(surface, classMap)
	if len(classes) == 0 {
		return "", false
	}
	class := ""
	for candidate := range classes {
		if class == "" {
			class = candidate
			continue
		}
		if class != candidate {
			return "", false
		}
	}
	return class, true
}

func hasHarmonyPriorityClassSurface(surface string, classMap map[int]string) bool {
	return len(harmonyPriorityClassesInSurface(surface, classMap)) > 0
}

func harmonyPriorityClassesInSurface(surface string, classMap map[int]string) map[string]bool {
	out := map[string]bool{}
	matches := runtimeHarmonyPrioClassRE.FindAllStringSubmatch(surface, -1)
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		prio, err := strconv.Atoi(match[1])
		if err == nil && prio > 0 {
			if class := classMap[prio]; class != "" {
				out[class] = true
				continue
			}
		}
		out[match[2]] = true
	}
	for _, match := range perfTracePrioTextRE.FindAllStringSubmatch(surface, -1) {
		if len(match) < 2 {
			continue
		}
		prio, err := strconv.Atoi(match[1])
		if err != nil || prio <= 0 {
			continue
		}
		if class := classMap[prio]; class != "" {
			out[class] = true
		}
	}
	return out
}

func harmonyPriorityLineIsRule(line string) bool {
	if runtimeHarmonyPrioClassRE.MatchString(line) {
		return false
	}
	lower := strings.ToLower(line)
	return strings.Contains(lower, "1-40") &&
		(strings.Contains(lower, "41-139") || strings.Contains(lower, "41-159")) &&
		strings.Contains(lower, "cfs") &&
		strings.Contains(lower, "rt")
}

func normalizeHarmonyPriorityRuleRange(line string) string {
	for _, old := range []string{"41-139", "41–139"} {
		line = strings.ReplaceAll(line, old, strings.ReplaceAll(old, "139", "159"))
	}
	for _, old := range []string{">139", "＞139"} {
		line = strings.ReplaceAll(line, old, strings.ReplaceAll(old, "139", "159"))
	}
	// Legacy two-band rules omitted the opaque raw scheduler-token lane. Once
	// the RT ceiling is upgraded, complete the common machine-readable rule
	// shape as well; otherwise an old answer can still imply that every value
	// above 40 is RT even though its numeric boundary was repaired.
	lower := strings.ToLower(line)
	if !strings.Contains(lower, "system_or_kernel") &&
		!strings.Contains(line, ">159") && !strings.Contains(line, "＞159") {
		for _, rtRule := range []string{"41-159=RT", "41–159=RT"} {
			if strings.Contains(line, rtRule) {
				line = strings.Replace(line, rtRule, rtRule+", >159=system_or_kernel/raw", 1)
				break
			}
		}
	}
	return line
}

func normalizeHarmonyPriorityLine(line, class string, classMap map[int]string) string {
	switch class {
	case "ohos_rt":
		line = replaceHarmonyPriorityClassPhrases(line, "CFS", "RT", "1-40", "41-159", classMap)
	case "ohos_cfs":
		line = replaceHarmonyPriorityClassPhrases(line, "RT", "CFS", "41-159", "1-40", classMap)
	}
	return line
}

func replaceHarmonyPriorityClassPhrases(line, wrongClass, correctClass, wrongRange, correctRange string, classMap map[int]string) string {
	replacements := []struct {
		old string
		new string
	}{
		{"属于 " + wrongClass + " 区间（" + wrongRange + "）", "属于 " + correctClass + " 区间（" + correctRange + "）"},
		{"属于 " + wrongClass + " 区间(" + wrongRange + ")", "属于 " + correctClass + " 区间(" + correctRange + ")"},
		{wrongClass + " 区间（" + wrongRange + "）", correctClass + " 区间（" + correctRange + "）"},
		{wrongClass + " 区间(" + wrongRange + ")", correctClass + " 区间(" + correctRange + ")"},
		{wrongClass + " range", correctClass + " range"},
		{"in " + wrongClass, "in " + correctClass},
	}
	if correctClass == "RT" {
		replacements = append(replacements,
			struct {
				old string
				new string
			}{"处于 CFS 类", "处于 RT 类"},
			struct {
				old string
				new string
			}{"处于CFS类", "处于RT类"},
		)
	}
	if correctClass == "CFS" {
		replacements = append(replacements,
			struct {
				old string
				new string
			}{"处于 RT 类", "处于 CFS 类"},
			struct {
				old string
				new string
			}{"处于RT类", "处于CFS类"},
		)
	}
	for _, repl := range replacements {
		line = strings.ReplaceAll(line, repl.old, repl.new)
	}
	line = replaceHarmonyPriorityClassNearBarePrio(line, wrongClass, correctClass, classMap)
	return line
}

func replaceHarmonyPriorityClassNearBarePrio(line, wrongClass, correctClass string, classMap map[int]string) string {
	if len(classMap) == 0 {
		return line
	}
	for prio, class := range classMap {
		want := ""
		wrongOhos := ""
		correctOhos := ""
		switch class {
		case "ohos_rt":
			want = "RT"
			wrongOhos = "ohos_cfs"
			correctOhos = "ohos_rt"
		case "ohos_cfs":
			want = "CFS"
			wrongOhos = "ohos_rt"
			correctOhos = "ohos_cfs"
		default:
			continue
		}
		if want != correctClass {
			continue
		}
		prioText := fmt.Sprintf("prio=%d", prio)
		line = strings.ReplaceAll(line, prioText+"（"+wrongClass+" 类）", prioText+"（"+correctClass+" 类）")
		line = strings.ReplaceAll(line, prioText+"("+wrongClass+" 类)", prioText+"("+correctClass+" 类)")
		line = strings.ReplaceAll(line, prioText+"（"+wrongClass+"类）", prioText+"（"+correctClass+"类）")
		line = strings.ReplaceAll(line, prioText+"("+wrongClass+"类)", prioText+"("+correctClass+"类)")
		line = strings.ReplaceAll(line, prioText+"（"+wrongClass+"，", prioText+"（"+correctClass+"，")
		line = strings.ReplaceAll(line, prioText+"（"+wrongClass+",", prioText+"（"+correctClass+",")
		line = strings.ReplaceAll(line, prioText+"("+wrongClass+",", prioText+"("+correctClass+",")
		line = strings.ReplaceAll(line, prioText+"（"+wrongClass+"、", prioText+"（"+correctClass+"、")
		line = strings.ReplaceAll(line, prioText+"/"+wrongOhos, prioText+"/"+correctOhos)
		for _, wrongRange := range harmonyPriorityRangeVariants(wrongClass) {
			for _, correctRange := range harmonyPriorityRangeVariants(correctClass) {
				line = strings.ReplaceAll(line, wrongClass+" "+wrongRange+" 波段", correctClass+" "+correctRange+" 波段")
				line = strings.ReplaceAll(line, wrongClass+" "+wrongRange+" 区间", correctClass+" "+correctRange+" 区间")
				line = strings.ReplaceAll(line, wrongClass+" "+wrongRange+" range", correctClass+" "+correctRange+" range")
			}
		}
		line = strings.ReplaceAll(line, wrongOhos+"（"+wrongClass+"）", correctOhos+"（"+correctClass+"）")
		line = strings.ReplaceAll(line, wrongOhos+"("+wrongClass+")", correctOhos+"("+correctClass+")")
		line = strings.ReplaceAll(line, wrongOhos+"（"+wrongClass+"类）", correctOhos+"（"+correctClass+"类）")
		line = strings.ReplaceAll(line, wrongOhos+"("+wrongClass+"类)", correctOhos+"("+correctClass+"类)")
		line = strings.ReplaceAll(line, wrongClass+" 类（"+prioText+"）", correctClass+" 类（"+prioText+"）")
		line = strings.ReplaceAll(line, wrongClass+"类（"+prioText+"）", correctClass+"类（"+prioText+"）")
		line = strings.ReplaceAll(line, wrongClass+" 类("+prioText+")", correctClass+" 类("+prioText+")")
		line = strings.ReplaceAll(line, wrongClass+"类("+prioText+")", correctClass+"类("+prioText+")")
		line = strings.ReplaceAll(line, prioText+" "+wrongClass+" 类", prioText+" "+correctClass+" 类")
		line = strings.ReplaceAll(line, prioText+" "+wrongClass+"类", prioText+" "+correctClass+"类")
	}
	return line
}

func harmonyPriorityRangeVariants(class string) []string {
	switch class {
	case "CFS":
		return []string{"1-40", "1–40"}
	case "RT":
		return []string{"41-159", "41–159"}
	default:
		return nil
	}
}

func runtimeTraceObservationItemID(label string, index int) string {
	base := strings.ToLower(label)
	var b strings.Builder
	for _, r := range base {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		switch label {
		case "时间单位":
			return "trace_time_semantics"
		case "平台优先级语义":
			return "trace_priority_semantics"
		case "优先级归一化":
			return "trace_priority_normalized"
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		return "trace_" + s
	}
	return fmt.Sprintf("trace_observation_%d", index)
}

func answerDocumentVisibleSurfaceForRuntimeTrace(doc *types.AnswerDocumentV2) string {
	if doc == nil {
		return ""
	}
	var b strings.Builder
	for _, block := range doc.Blocks {
		appendAnswerDocumentRuntimeSurface(&b, block.Title)
		appendAnswerDocumentRuntimeSurface(&b, types.AnswerBlockVisibleSurface(block))
	}
	for _, caveat := range doc.Caveats {
		appendAnswerDocumentRuntimeSurface(&b, caveat)
	}
	return b.String()
}

func appendAnswerDocumentRuntimeSurface(b *strings.Builder, s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	b.WriteString(s)
}

// canonicalizeSummaryLeadBlock moves the first renderable summary block in
// front of the first renderable non-summary block. This is a deterministic
// document-structure normalization, not a semantic repair: summary blocks are
// the answer lead-in everywhere the V2 renderer uses them, while tables,
// ordered lists, diagrams, and caveats remain in their relative order.
//
// Keeping this at the mutation chokepoint means full emits and patch emits get
// the same behavior. Without this, the finalizer can spend a retry round fixing
// only block order even when every row, citation, and claim_use is already
// structurally valid.
func canonicalizeSummaryLeadBlock(doc *types.AnswerDocumentV2) bool {
	if doc == nil || len(doc.Blocks) < 2 {
		return false
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
	if firstRenderable < 0 || summaryAt < 0 || summaryAt == firstRenderable {
		return false
	}
	summary := doc.Blocks[summaryAt]
	copy(doc.Blocks[firstRenderable+1:summaryAt+1], doc.Blocks[firstRenderable:summaryAt])
	doc.Blocks[firstRenderable] = summary
	return true
}

// maxBlocksPerDoc caps the number of blocks per doc. Conservative
// upper bound; production answers rarely exceed 10. Tests may
// exercise the cap by constructing a doc with > maxBlocksPerDoc
// entries.
//
// Headroom invariant (2026-06-12): the cap gate in
// validateMergedV2Doc must only ever reject counts the MODEL
// produced. Any system-side block insertion or split that runs
// before the gate (fused-diagram splits, trace/caveat/supplement/
// carrier materializers) MUST check remaining headroom under this
// cap first and degrade to a guarded no-op or unsplit pass-through
// — never push a model-fitting emit over the cap into a fabricated
// reject.
const maxBlocksPerDoc = 64
