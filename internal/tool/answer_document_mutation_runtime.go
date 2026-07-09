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
		if runtimeTraceCausalProjectionFamilyBlockID(strings.TrimSpace(block.ID)) {
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
	case "", "_detail", "_evidence", "_compare", "_compare_notes", "_coverage", "_partition":
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
	case "", "_detail", "_evidence":
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
// PSG §25(b) consumer (2026-07-08): the prose-scalar grounding gate treats
// matching blocks as system evidence surfaces (their numerals ground prose,
// their text is never scanned as model prose) — a loose prefix match there
// would let a model-authored lookalike block launder fabricated numbers
// into the evidence set, so this helper is deliberately exact.
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
			insertAt := answerDocumentInsertionIndexBeforeCaveat(doc)
			doc.Blocks = append(doc.Blocks, types.AnswerBlock{})
			copy(doc.Blocks[insertAt+1:], doc.Blocks[insertAt:])
			doc.Blocks[insertAt] = *block
			return true
		}
		return false
	}
	// The lead section is the safest minimum user-facing surface. The secondary
	// blocks carry drilldown/evidence detail, so if the document is already near
	// the block cap, degrade to the first projection section lead (never the
	// compare overview — F2a) instead of dropping the whole section.
	if len(doc.Blocks)+len(cluster) > maxBlocksPerDoc {
		lead := runtimeTraceCausalProjectionDegradeLeadBlock(cluster)
		if lead == nil || len(doc.Blocks)+1 > maxBlocksPerDoc {
			logging.Warning("[answer_document] runtime trace causal projection block skipped: document already at the %d-block cap", maxBlocksPerDoc)
			return false
		}
		cluster = []types.AnswerBlock{*lead}
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
	titleSuffix := ""
	if label := strings.TrimSpace(artifactLabel); label != "" {
		titleSuffix = " — " + label
	}
	claimUses := []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}}
	facets := []string{"observed_artifact_fact"}
	leadText := runtimeTraceProjLeadText(projection, model, lang, zh) + "\n\n" + fence
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
		lines := []string{
			"各列口径:",
			"- 窗口投影 = 该节点的状态落在分析窗内的时长;跨线程聚合行按跨线程累计计量(非墙钟,单元格已标注)。",
			"- 链上累计 = 该节点及其下钻子链沿唤醒链累计到关注线程的投影时长。",
			"- 有效归因 = 该行计入根因排序的影响时长;与窗口投影不同时,行内口径词(全额/折算/单次最大等)说明取值方式。",
			"- 实际状态 = 该状态的真实完整时长,可跨出分析窗(此时带 ⚠)。",
			"- 「—」 = 该列对此节点无值。",
			"- ⊘ = 窗口内无匹配唤醒事件(sched_wakeup),链止于此(同树内 ⊘链止)。",
			"- ⚠ = 实际状态跨出分析窗(同树内 ⚠实际Xms)。",
			"- 背景行仅作环境压力证据,不计入链上归因。",
			"- 本表只列时长与置信;每个节点的类型、因果位置、关系、影响形态、×N 成员清单与完整名称,见下方「因果投影明细」。",
		}
		if !zh {
			lines = []string{
				"Column calibers:",
				"- window projection = the duration of the node's state inside the analysis window; cross-thread aggregate rows measure a cross-thread cumulative (not wall clock; cells carry the annotation).",
				"- chain total = the projected duration this node plus its drill-down sub-chain accumulate toward the focused thread along the wakeup chain.",
				"- attribution = the impact duration this row contributes to the root-cause ranking; when it differs from the window projection, the row's caliber word (in full / discounted / single max …) says how it was taken.",
				"- actual state = the state's true full duration; it may extend beyond the analysis window (then marked ⚠).",
				"- “—” = no value in this column for this node.",
				"- ⊘ = no matching wakeup event (sched_wakeup) in the window; the chain ends there (same as the tree's ⊘chain-ends mark).",
				"- ⚠ = the actual state extends beyond the analysis window (same as the tree's ⚠actual mark).",
				"- Background rows are context-pressure evidence only, never counted into the chain attribution.",
				"- This table lists durations and confidence only; each node's type, causal position, relation, impact shape, ×N member roster and full name live in the Causal Projection Detail below.",
			}
		}
		// PTV5 C33/C34 (#68): the ×N-form and dual-seat notations get legend
		// rows exactly when the table shows them (gated flags from the same
		// detail rows the table renders) — every other render stays
		// byte-stable.
		if flags := runtimeTraceProjDetailTableLegendFlagsFor(model, zh); flags.mergedSum || flags.mergedMax || flags.mergedWindowMax || flags.mergedDedup || flags.multiSeat || flags.family || flags.selfSymptom || flags.allZeroFold || flags.stanzaChainTotal {
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
					lines = append(lines, "- ×N合计 = 同一线程同类 N 段合并为一个参赛项,数值为同线程墙钟合计(重叠段取并集;跨线程仍不可加和);×N成员最大 = 重叠无法逐段核销时取成员最大(下界)。成员清单与区分键见下方明细。")
				} else {
					lines = append(lines, "- ×N total = N same-kind segments of ONE thread merged into one contender; the value is the same-thread wall-clock total (overlaps as their interval union; across threads wall clock still never sums). ×N member max = the member MAX lower bound when overlap cannot be deducted. Member rosters and distinguishing keys live in the detail blocks.")
				}
			}
			if flags.mergedSum || flags.mergedMax || flags.mergedWindowMax || flags.mergedDedup {
				var parts []string
				if zh {
					if flags.mergedSum {
						// PTV8-RCR-B (UXA 域B 漏审 S3): the 同一(线程,原因)
						// scope clause matches the tree's sum entry (同词).
						parts = append(parts, "×N(a–b) = 同一(线程,原因)的 N 次实例合并,数值为总和")
					}
					if flags.mergedMax {
						// PTV8-RCR-B (UXA 域B #11 REVISE): canonical 墙钟跨线程不可加和.
						parts = append(parts, "×N(a–b)取最大 = 跨线程折叠,数值取成员最大(墙钟跨线程不可加和)")
					}
					if flags.mergedWindowMax {
						// §21 CWD: the cross-window MAX form gets its own gated
						// line — the sum line's 数值为总和 must never gloss it.
						parts = append(parts, "×N(a–b)跨窗取最大 = 查询窗互相重叠,数值取成员最大(互相重叠的查询窗量值不可求和)")
					}
					if flags.mergedDedup {
						parts = append(parts, "×N同值 = 同一测量重复发布,数值即那一次")
					}
					lines = append(lines, "- "+strings.Join(parts, ";")+"。")
				} else {
					if flags.mergedSum {
						parts = append(parts, "×N(a–b) = N merged instances, the value is the SUM")
					}
					if flags.mergedMax {
						parts = append(parts, "×N(a–b) max = cross-thread fold, the value is the member MAX (wall clock never sums)")
					}
					if flags.mergedWindowMax {
						parts = append(parts, "×N(a–b) cross-window max = overlapping query windows, the value is the member MAX (overlapping-window magnitudes never sum)")
					}
					if flags.mergedDedup {
						parts = append(parts, "×N same-value = one measurement published N times, the value IS that one")
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
		title := "因果投影明细(逐节点完整属性)"
		// 复核收窄: the blocks cover every DATA-bearing rendered node; folded
		// transit hops carry no data row and live on the 省略行 roster — the
		// intro must not over-promise them. PTV5 C42 (#68): 省略行 roster →
		// 省略行清单 (no half-English). PTV6-C ruling C (#73): the trailing
		// "与原始 trace_query 记录" pointer is retired — the roster is the
		// in-answer surface; the intermediate record file is not a user-facing
		// pointer target.
		intro := "每个节点一块,给出树和指标表中省略或压缩的全部属性;名称不截断;属性完全相同的同名节点共用一块(标题并列各自编号)。树中折叠的中间线程见树内省略行清单。"
		if !zh {
			title = "Causal Projection Detail (full attributes per node)"
			intro = "One block per node, carrying every attribute the tree or the key-metric table demotes or compresses; names are never truncated; identical same-name nodes share one block (evidence numbers side by side in the heading). Folded transit hops live on the tree's omitted-row roster."
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
		title := "证据索引"
		if !zh {
			title = "Evidence Index"
		}
		// NEW-9 (adversarial re-review 2026-07-04): typed truncation disclosure.
		// When any trace_query record of THIS artifact carried the producer's
		// capacity_truncated note (per-view row budget cut the result tail), the
		// index header says so instead of presenting the roster as exhaustive.
		if projection.CapacityTruncated {
			// PTV6-C ruling C (#73): the truncation disclosure states the fact
			// without deflecting to the intermediate record file — the cut
			// tails were never collected, so no coordinate exists to give.
			if zh {
				intro += " 部分查询结果超过单次返回上限:各自排序靠前的部分完整保留,超限的尾部行不在本索引内。"
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
	var out []types.AnswerBlock
	if runtimeTraceProjComparisonShape(len(set.Projections)) {
		// PTV8-LAD L6: the plural form carries the 对比注记明细 sibling when
		// the layered table notes folded past the visible cap.
		out = append(out, runtimeTraceProjCompareOverviewBlocks(set.Projections, ledger, lang, zh, focus)...)
	}
	for i, projection := range set.Projections {
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = strings.TrimSpace(projection.ArtifactPath)
		}
		section := runtimeTraceCausalProjectionClusterFor(projection, lang, focus,
			fmt.Sprintf("%s%s%d", runtimeTraceCausalProjectionBlockIDBase,
				runtimeTraceCausalProjectionArtifactBlockIDInfix, i+1), label)
		out = append(out, section...)
	}
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
	intro := "对比总览的全部注记(含总览下已显示的条目),按重要度分层排序:口径矛盾 > 窗基 > 披露。"
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
	columns := []string{"trace 文件", "主根因(根因排序#1)", "关注线程症状时长", "链上已归因(单项最大)", "背景压力"}
	if !zh {
		columns = []string{"Artifact", "Primary root cause (rank=1)", "Target symptom", "On-chain attributed (single largest)", "Background pressure"}
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
		columns = append(columns, "Projection window")
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
		if projection.WindowStartTs > 0 && projection.WindowEndTs > projection.WindowStartTs {
			window = fmt.Sprintf("%.3fs → %.3fs", projection.WindowStartTs, projection.WindowEndTs)
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
			note = "⚠ Projection window lengths differ; background pressure is normalized per window"
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
		{"关注线程症状时长", "Target symptom", symptomBases},
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
	title := "Trace 因果投影对比总览"
	text := "跨 trace 对比总览:数值来自各份 trace 独立的投影,跨线程累计值带单位标注,详情见各 trace 分段。"
	if !zh {
		title = "Trace Causal Projection Comparison Overview"
		text = "Cross-trace comparison overview: every value comes from each artifact's independent projection (structured fields); cross-thread cumulative values carry their unit annotation. Details live in the per-artifact sections."
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
				cell = fmt.Sprintf("%.3fms (direct waits inside the analysis window only; %d more target state row(s) uncounted, single largest %.3fms; the chain/self data spans multiple query windows)", symptom, excluded, excludedMax)
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
				cell = fmt.Sprintf("%.3fms (direct waits inside the analysis window only; %d more target state row(s) uncounted, single largest %.3fms)", symptom, excluded, excludedMax)
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
		base := runtimeTraceProjCompareWindowBaseFrom(projection, hopWinStart, hopWinEnd,
			hopWinStart > 0 && hopWinEnd > hopWinStart, false)
		if zh {
			return fmt.Sprintf("%.3fms(唤醒链采样到的关注线程睡眠合计,非全窗状态统计)", hopSleep), runtimeTraceProjCompareSymptomArmHop, base
		}
		return fmt.Sprintf("%.3fms (wakeup-chain-view target sleep, not a state-segment aggregate)", hopSleep), runtimeTraceProjCompareSymptomArmHop, base
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
	return "⚠ The target-symptom column mixes two calibers (whole-window state statistics / wakeup-chain sampled); the two sides cannot be read straight across"
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
	if !ok || ws <= 0 || we <= ws {
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
		node.QueryWindowStartTs > 0 && node.QueryWindowEndTs > node.QueryWindowStartTs, false)
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
		if projection.WindowStartTs <= 0 || projection.WindowEndTs <= projection.WindowStartTs {
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
	cause := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(*primary, zh))
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
			cell += fmt.Sprintf(" 单次最大 %.3fms ×%d", primary.MergedMaxMS, primary.MergedCount)
		} else {
			cell += fmt.Sprintf(" single max %.3fms ×%d", primary.MergedMaxMS, primary.MergedCount)
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
	for i := range model.Background {
		node := &model.Background[i].Node
		if !runtimeTraceProjCrossThreadAggregateType(*node) {
			continue
		}
		if v := runtimeTraceProjNodeDisplayImpact(*node); best == nil || v > bestValue {
			best, bestValue = node, v
		}
	}
	if best == nil || bestValue <= 0 {
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
		// RCM-2 D3: the ×N token + the family caliber word (shared single
		// source; unknown calibers make no claim — the count is still truth).
		cell += fmt.Sprintf(" ×%d", best.FamilyMemberCount)
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
		spans = append(spans, fmt.Sprintf("%s %.3fs→%.3fs",
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
	subject := "The two artifacts' time bases do not overlap"
	if len(projections) > 2 {
		subject = "The artifacts' time bases are pairwise disjoint"
	}
	return fmt.Sprintf("⚠ %s (%s); they cannot be aligned directly on one shared timeline — compare relative metrics within each artifact's own window",
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
	if set.UnattributedObservationCount > 0 {
		if zh {
			parts = append(parts, fmt.Sprintf("%d 条观测无法归属到任一 trace 文件,未纳入投影。", set.UnattributedObservationCount))
		} else {
			parts = append(parts, fmt.Sprintf("%d observation(s) carried no artifact identity and were left out of every projection.", set.UnattributedObservationCount))
		}
	}
	if len(set.OmittedArtifactLabels) > 0 {
		if zh {
			parts = append(parts, fmt.Sprintf("trace 文件分区数超过上限,仅保留观测最多的 %d 个;未展示: %s。",
				len(set.Projections), strings.Join(set.OmittedArtifactLabels, "、")))
		} else {
			parts = append(parts, fmt.Sprintf("Artifact partitions exceeded the cap; the %d with the most observations are shown. Omitted: %s.",
				len(set.Projections), strings.Join(set.OmittedArtifactLabels, ", ")))
		}
	}
	if len(parts) == 0 {
		return nil
	}
	title := "Trace 因果投影分区边界"
	if !zh {
		title = "Trace Causal Projection Partition Boundary"
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
	if len(reasons) == 0 {
		return nil
	}
	text := runtimeTraceCausalProjectionCoverageText(reasons, zh)
	title := "Trace 因果投影覆盖边界"
	if !zh {
		title = "Trace Causal Projection Coverage Boundary"
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
		text := "本报告已获得 trace_query 的结构化执行记录,但没有产出有数据支撑的 root_cause/wakeup_chain/semantic 行,因此未生成分层因果表。"
		if len(reasons) > 0 {
			text += " 结构化原因: " + strings.Join(reasons, "；") + "。"
		}
		text += " 这不是“没有背景影响”的结论;只表示当前证据没有给出可审计的因果/背景统计,可追问一次根因/窗口/交互统计分析(root_cause_rank、window_stats 或 interaction_stats)补齐。"
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
}

func newRuntimeTraceCausalProjectionEvidenceIndex() *runtimeTraceCausalProjectionEvidenceIndex {
	return &runtimeTraceCausalProjectionEvidenceIndex{seen: map[string]string{}}
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
	if node.StartTs > 0 && node.EndTs > node.StartTs {
		window = fmt.Sprintf("[%.3f–%.3fs]", node.StartTs, node.EndTs)
	}
	idx.order = append(idx.order, runtimeTraceCausalProjectionEvidenceEntry{
		ID:            id,
		Ref:           strings.TrimSpace(ref),
		Window:        window,
		Details:        runtimeTraceCausalProjectionAuditDetail(node, zh, idx.flatChain),
		SyntheticLine:  node.Undrillable(),
		FamilyAudit:    node.FamilyMemberCount > 1,
		SameValueAudit: len(node.SameValueMembers) > 0,
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
	if zh {
		return "正文用 E1、E2 等编号引用证据;本索引给出每条证据在 trace 中的位置(行号或时间区间)与审计字段。" +
			"审计字段为 trace_query 原文 token,便于回溯核对:tier=证据层级、causality=因果位置、rank=根因排序、confidence=置信度、predicate=判定类型、span=span 名、merged_*=合并明细、member_*=同线程家族合并明细、same_value_*=跨线程取最大折叠中同值到微秒的成员及各自行区间(供核对是否同段);其余字段同为原文 token。"
	}
	return "The answer cites evidence by the E1/E2 numbers; this index gives each entry's location in the trace (line or time span) and its audit fields. " +
		"Audit fields are raw trace_query tokens kept for cross-checking: tier = evidence tier, causality = causal position, rank = root-cause rank, confidence = confidence, predicate = judgment kind, span = span name, merged_* = merge detail, member_* = same-thread family-merge detail, same_value_* = members of a cross-thread take-MAX fold whose values tie to the µs, with each member's own line interval (to check whether they are one segment); any other field is likewise a raw token."
}

func runtimeTraceCausalProjectionPriorityCell(node types.TraceCausalProjectionNode, zh bool) string {
	switch {
	case node.Role == types.TraceCausalRolePrimaryRootCause || strings.HasPrefix(strings.TrimSpace(node.Predicate), "root_cause_primary"):
		if zh {
			return "主要关注"
		}
		return "primary focus"
	case node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span",
		strings.TrimSpace(node.Tier) == "deterministic_optimization":
		// DCS E1 display word (ledger §23.1, 2026-07-08): an on-chain
		// tier=deterministic_optimization rank row wears the SAME 确定优化
		// priority word as the semantic observation lane — one 确定性优化
		// display family, and never the on_chain 重点关注 root-cause word.
		if zh {
			return "确定优化"
		}
		return "optimize"
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
	if node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span" {
		if zh {
			return "确定性优化点"
		}
		return "semantic"
	}
	if strings.TrimSpace(node.Tier) == "deterministic_optimization" {
		// DCS E1 display word (ledger §23.1): the on-chain optimization tier
		// speaks the 确定性优化点 layer word, aligned with the semantic
		// observation lane above — never 链上 root-cause layer wording.
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
				return "优化点·" + runtimeTraceCausalProjectionCompactCellText(class, 22), ""
			}
			return "确定性优化点", ""
		}
		if class != "" {
			return "optimize·" + runtimeTraceCausalProjectionCompactCellText(class, 22), ""
		}
		return "optimization point", ""
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
	if runtimeTraceCausalProjectionInversionRow(node) {
		if zh {
			return runtimeTraceRootCauseTypeZHLabel("priority_inversion_candidate"), false
		}
		return "priority_inversion_candidate", false
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
	case "priority_inversion_runnable_wait":
		if zh {
			return "runnable调度候选", false
		}
		return "runnable scheduling candidate", false
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
		return runtimeTraceCausalProjectionTypeTokenStateWord(class, zh), false
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
	if runtimeTraceCausalProjectionUseChinese(lang) {
		return "Trace 因果投影"
	}
	return "Trace Causal Projection"
}

func runtimeTraceCausalProjectionNodeSubjectCell(node types.TraceCausalProjectionNode, zh bool) string {
	if node.IsAggregateMetric() {
		return runtimeTraceCausalProjectionCompactCellText(runtimeTraceCausalProjectionAggregateMetricName(node, zh), 44)
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
	if runtimeTraceCausalProjectionSemanticSpanRow(node) && strings.TrimSpace(node.SpanName) != "" {
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
		// lane + count + roster on THIS surface too — mirroring the tree row
		// and the lossless block; and the subject/object-less fallback speaks
		// zh on the zh panel ("trace causal node" stays EN-face only).
		if node.OnChainOverflowFold {
			if zh {
				return runtimeTraceCausalProjectionCompactCellText(
					fmt.Sprintf("其余 %d 项(链上折叠)%s", node.MergedCount, runtimeTraceProjMergedSubjectsSuffix(node, zh)), 44)
			}
			return runtimeTraceCausalProjectionCompactCellText(
				fmt.Sprintf("%d more (on-chain fold)%s", node.MergedCount, runtimeTraceProjMergedSubjectsSuffix(node, zh)), 44)
		}
		if zh {
			return "(未命名因果节点)"
		}
		return "trace causal node"
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
	case "d_state_or_io_wait":
		// PTV7 (#74): the state compound speaks the canonical tokens; the
		// peer-relation frame stays localized.
		if zh {
			return "D-state/iowait(对端未解析)"
		}
		return "D-state/iowait (peer unresolved)"
	default:
		if zh {
			return "对端线程未解析"
		}
		return "unresolved wait peer"
	}
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
			return kind
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
			return kind
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
	case "d_state_or_io_wait":
		// PTV7 (#74): same canonical compound as the unresolved arm (同形).
		if zh {
			return "D-state/iowait(对端 " + peer + ")"
		}
		return "D-state/iowait (peer " + peer + ")"
	case "io_latency":
		if zh {
			return "IO等待(对端 " + peer + ")"
		}
		return "IO wait (peer " + peer + ")"
	}
	return peer
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
		if zh {
			return "窗口IO压力(聚合)"
		}
		return "window IO pressure (aggregate)"
	case "cpu_pressure":
		if zh {
			return "CPU竞争压力(聚合)"
		}
		return "CPU contention pressure (aggregate)"
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
	replacer := strings.NewReplacer(
		"~~", "\\~\\~",
		"~", "\\~",
	)
	return replacer.Replace(raw)
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
// semantic optimization points (class_verify / jit_compile / shader_compile /
// runtime_compile spans that the projection already carries as typed
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
	if len(doc.Blocks) >= maxBlocksPerDoc {
		logging.Warning("[answer_document] runtime trace semantic optimization block skipped: document already at the %d-block cap", maxBlocksPerDoc)
		return false
	}
	if answerDocumentHasBlockID(doc, "runtime_trace_semantic_optimizations") {
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
			buildRuntimeTraceProjTreeModel(projection, evidence, zh)
			cols, sectionRows := runtimeTraceSemanticOptimizationParts(projection, evidence, zh)
			columns = cols
			label := strings.TrimSpace(projection.ArtifactLabel)
			for _, row := range sectionRows {
				if label != "" && len(row.Cells) == 5 && row.Cells[4] != "—" {
					row.Cells[4] = runtimeTraceCausalProjectionMarkdownSafe(label) + " " + row.Cells[4]
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
		buildRuntimeTraceProjTreeModel(projection, evidence, zh)
		columns, rows = runtimeTraceSemanticOptimizationParts(projection, evidence, zh)
	}
	if len(rows) == 0 {
		return false
	}
	// PTV6-C ruling C (#73): the grounding clause points at the report's own
	// evidence index (trace line/time coordinates) — never the intermediate
	// trace_query record file.
	title := "确定性优化点"
	text := "trace 中的确定性语义优化 span(类校验/JIT/着色器编译等,来自 typed semantic_class 通道):每行都是可直接落地的优化点;时长与 E# 证据均可经证据索引定位到 trace 行号区间。"
	if !zh {
		title = "Deterministic Optimization Points"
		text = "Deterministic semantic optimization spans found in the trace (class verification / JIT / shader compilation etc., from the typed semantic_class lane): each row is a directly actionable optimization point; durations and E# tags resolve to trace line spans via the evidence index."
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
	insertAt := answerDocumentInsertionIndexBeforeCaveat(doc)
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{})
	copy(doc.Blocks[insertAt+1:], doc.Blocks[insertAt:])
	doc.Blocks[insertAt] = block
	return true
}

// runtimeTraceSemanticOptimizationParts builds the ZH/EN-symmetric table rows
// for the deterministic optimization block: span name, typed semantic class,
// host thread, effective cost (EffectiveImpactMS with the display-impact
// fallback), and the shared E# evidence tag. CitationRef=-1 on every
// system-injected row (red-line invariant).
func runtimeTraceSemanticOptimizationParts(projection types.TraceCausalProjection, evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) ([]string, []types.AnswerBlockItem) {
	columns := []string{"优化点", "类别", "宿主线程", "有效成本", "证据"}
	if !zh {
		columns = []string{"Optimization point", "Class", "Host thread", "Effective cost", "Evidence"}
	}
	dash := "—"
	var rows []types.AnswerBlockItem
	for _, span := range projection.SemanticSpans {
		name := strings.TrimSpace(span.SpanName)
		if name == "" {
			name = strings.TrimSpace(span.Object)
		}
		if name == "" {
			continue
		}
		class := strings.TrimSpace(span.SemanticClass)
		if class == "" {
			class = dash
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
		tag := runtimeTraceProjEvidenceTag(span, evidence, zh)
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
					tag,
				},
				CitationRef: -1,
			})
			listed := len(span.FamilyMemberRoster)
			if listed > 3 {
				listed = 3
			}
			for _, member := range span.FamilyMemberRoster[:listed] {
				memberCell := "· 成员 " + member
				if !zh {
					memberCell = "· member " + member
				}
				rows = append(rows, types.AnswerBlockItem{
					Cells:       []string{runtimeTraceCausalProjectionMarkdownSafe(memberCell), dash, dash, dash, dash},
					CitationRef: -1,
				})
			}
			if rest := span.FamilyMemberCount - listed; rest > 0 {
				foldCell := fmt.Sprintf("· 其余 %d 项(家族折叠,成员共%d,列%d;见因果投影明细)", rest, span.FamilyMemberCount, listed)
				if !zh {
					foldCell = fmt.Sprintf("· %d more (family fold; %d members, %d listed — see the causal projection detail)", rest, span.FamilyMemberCount, listed)
				}
				rows = append(rows, types.AnswerBlockItem{
					Cells:       []string{runtimeTraceCausalProjectionMarkdownSafe(foldCell), dash, dash, dash, dash},
					CitationRef: -1,
				})
			}
			continue
		}
		rows = append(rows, types.AnswerBlockItem{
			Cells: []string{
				runtimeTraceCausalProjectionMarkdownSafe(name),
				class,
				runtimeTraceCausalProjectionMarkdownSafe(host),
				costCell,
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
	if answerDocumentHasBlockID(doc, "runtime_trace_metric_snapshot") {
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
	insertAt := answerDocumentInsertionIndexBeforeCaveat(doc)
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{})
	copy(doc.Blocks[insertAt+1:], doc.Blocks[insertAt:])
	doc.Blocks[insertAt] = block
	return true
}

func answerDocumentHasBlockID(doc *types.AnswerDocumentV2, id string) bool {
	if doc == nil {
		return false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, block := range doc.Blocks {
		if strings.TrimSpace(block.ID) == id {
			return true
		}
	}
	return false
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
				label = fmt.Sprintf("查询窗 %.3f–%.3fs · ", candidate.winStart, candidate.winEnd) + label
			} else {
				label = fmt.Sprintf("query window %.3f–%.3fs · ", candidate.winStart, candidate.winEnd) + label
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
	if zh {
		return fmt.Sprintf("(数据实际覆盖 %.1fs,远超分析窗,仅供背景参考)", totalMS/1000)
	}
	return fmt.Sprintf(" (actual data coverage %.1fs, far beyond the analysis window — background reference only)", totalMS/1000)
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
	var text string
	if zh {
		head := "状态时长(括号为占该线程观测时长比例): "
		if dominantEntry != "" {
			head += "主导 " + dominantEntry + ";"
		}
		text = head + strings.Join(states, " · ") +
			";切换特征: " + values[types.TraceNoteKeySwitches] + " 次切换/" + values[types.TraceNoteKeyFragments] + " 段" +
			",最长单段 " + ms(types.TraceNoteKeyMaxSegment) +
			",P95 段长 " + ms(types.TraceNoteKeyP95Segment)
	} else {
		head := "state durations (parentheses = share of this thread's observed span): "
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
			endpoints = fmt.Sprintf("%.3fs–%.3fs", start, end)
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
			if actual != "" {
				basis += ";" + actual
			} else {
				basis += "(另有按数据实际覆盖统计的数值)"
			}
		} else {
			basis = "; window basis: selected window"
			if endpoints != "" {
				basis += " " + endpoints
			}
			if actual != "" {
				basis += " (" + actual + ")"
			} else {
				basis += " (an aligned actual-window caliber also exists)"
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
			window = fmt.Sprintf("%.3fs–%.3fs", start, end)
		}
	}
	if len(parts) == 0 && window == "" {
		return ""
	}
	// PTV8-RCR-B (UXA 域D #15/#33 窗族): 实际对齐窗 → 数据实际覆盖.
	head := "数据实际覆盖"
	if !zh {
		head = "actual data coverage"
	}
	if window != "" {
		head += " " + window
	}
	if len(parts) == 0 {
		return head
	}
	return head + ": " + strings.Join(parts, "/")
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
	insertAt := answerDocumentInsertionIndexBeforeCaveat(doc)
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
	// as every other lane; the generic s_sleep template row stands down below
	// only when a named row actually rendered.
	namedPeerRows := false
	for _, hint := range runtimeTraceNextStepResolvedPeerHints(ledger, zh) {
		if len(out) >= runtimeTraceNextStepMaxItems {
			break
		}
		if hint == "" || seenText[hint] {
			continue
		}
		seenText[hint] = true
		namedPeerRows = true
		out = append(out, types.AnswerBlockItem{
			ID:          fmt.Sprintf("runtime_trace_next_step_%d", len(out)+1),
			Label:       label,
			Text:        hint,
			CitationRef: -1,
		})
	}
	// PTS-2 dynamic cap (#69 用户条件裁定 2026-07-06): on the comparison shape
	// the per-record rows read an extended cap — every leading lane above has
	// already been placed, so the guaranteed floor slots can only be consumed
	// by per-record rows (强保底, not a shared trailing budget). Hard upper
	// bound = base + undrilled-headline floor + per-record floor = 7 (NXT §22
	// D-P1: the pointed-row floor above participates via len(out); pre-NXT the
	// bound was base + per-record floor = 6). Non-comparison shapes keep
	// recordCap == base cap: byte-identical list behavior.
	recordCap := runtimeTraceNextStepMaxItems
	if comparisonShape {
		recordCap = len(out) + runtimeTraceNextStepComparisonRecordFloor
		if recordCap < runtimeTraceNextStepMaxItems {
			recordCap = runtimeTraceNextStepMaxItems
		}
	}
	for _, record := range ledger.Records {
		if len(out) >= recordCap {
			break
		}
		// SG 批 (§10-D1②): the generic s_sleep guidance ("investigate the peer
		// threads / binder waits / lock waits") yields to the named
		// holder/peer rows above — typed next_step_kind enum match only; rows
		// of every other kind (and legacy rows without a kind) are untouched.
		if namedPeerRows && strings.EqualFold(strings.TrimSpace(
			runtimeTraceObservationRichNoteValue(record.RichNotes, types.TraceNoteKeyNextStepKind)), "s_sleep") {
			continue
		}
		step := runtimeTraceNextStepFromObservationRecord(record, zh)
		if step == "" {
			continue
		}
		// Dedupe on the typed record payload, never on the localized rendered
		// text: a rendered-text key merges rows that carry different CPU /
		// competitor data whenever their localization collides (§7.30 裁定5
		// adversarial-review follow-up).
		key := runtimeTraceNextStepDedupeKey(record)
		if seen[key] {
			continue
		}
		seen[key] = true
		// H9 final display-layer dedupe, LAYERED ON TOP of the typed key: the
		// typed key keeps rows with different payloads apart (裁定5), but when
		// two distinct payloads still render byte-identical text the reader sees
		// pure duplicates. A verbatim match on the rendered line is a precise
		// display-only signal; the later duplicate is dropped from the LIST only
		// — both typed records stay in the ledger.
		if seenText[step] {
			continue
		}
		seenText[step] = true
		out = append(out, types.AnswerBlockItem{
			ID:          fmt.Sprintf("runtime_trace_next_step_%d", len(out)+1),
			Label:       label,
			Text:        step,
			CitationRef: -1,
		})
		if len(out) >= recordCap {
			break
		}
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
	return "The other trace was not queried this round: run the same-caliber queries (same window/same views) on the remaining trace artifacts, then compare"
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
		quals = append(quals, fmt.Sprintf("rank=%d", lead.Rank))
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
	if zh {
		if depthUnresolved {
			return fmt.Sprintf("对主根因 %s%s在其发生窗执行 wakeup_chain / critical_blocking_calls 下钻:该行当前深度未解析,尚无已核实的上游因果", subject, zhTail)
		}
		return fmt.Sprintf("对主根因 %s%s执行 critical_blocking_calls,并调整窗口后重试 wakeup_chain:所选窗口内无匹配唤醒记录,无法继续上溯", subject, zhTail)
	}
	if depthUnresolved {
		return fmt.Sprintf("Drill into the primary root cause %s with wakeup_chain / critical_blocking_calls in its occurrence window: its chain depth is unresolved and no verified upstream cause is attached yet", subject)
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

// runtimeTraceNextStepFromObservationRecord returns the localized next-step
// guidance for one typed trace_query record. English answers keep the original
// system-fixed prose verbatim; Chinese answers render the typed next_step_kind
// enum through a fixed ZH mapping (§7.30 裁定5) — the English prose is never
// passed through to a Chinese panel, and legacy records without a kind fall
// back to the generic ZH guidance instead of English.
func runtimeTraceNextStepFromObservationRecord(record types.ObservationRecord, zh bool) string {
	if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact {
		return ""
	}
	if !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
		return ""
	}
	step := trimRuntimeTraceNextStepText(runtimeTraceObservationRichNoteValue(record.RichNotes, types.TraceNoteKeyNextStep))
	if step == "" || !zh {
		return step
	}
	kind := runtimeTraceObservationRichNoteValue(record.RichNotes, types.TraceNoteKeyNextStepKind)
	if strings.EqualFold(strings.TrimSpace(kind), "runnable") {
		if dynamic := runtimeTraceNextStepRunnableChineseText(record.RichNotes); dynamic != "" {
			return dynamic
		}
	}
	return runtimeTraceNextStepChineseText(kind)
}

// runtimeTraceNextStepDedupeKey identifies one next-step guidance payload by
// its typed record carrier (kind + system prose + dynamic competitor notes) —
// never by the rendered text, which is language-dependent and can collide
// across rows that carry different CPU / competitor data.
func runtimeTraceNextStepDedupeKey(record types.ObservationRecord) string {
	return strings.Join([]string{
		runtimeTraceObservationRichNoteValue(record.RichNotes, types.TraceNoteKeyNextStepKind),
		trimRuntimeTraceNextStepText(runtimeTraceObservationRichNoteValue(record.RichNotes, types.TraceNoteKeyNextStep)),
		runtimeTraceObservationRichNoteValue(record.RichNotes, types.TraceNoteKeyRunnableCPU),
		runtimeTraceObservationRichNoteValue(record.RichNotes, types.TraceNoteKeyTopCompetitor),
	}, "\x00")
}

// runtimeTraceNextStepRunnableChineseText composes the runnable-kind ZH
// guidance from the typed state_churn rich notes (runnable_cpu /
// top_competitor), so the dynamic same-CPU competitor data carried by the
// English prose variants survives into a Chinese panel. Empty when neither
// typed note exists — the caller then falls back to the generic runnable
// guidance instead of fabricating data.
func runtimeTraceNextStepRunnableChineseText(notes []string) string {
	cpu := runtimeTraceObservationRichNoteValue(notes, types.TraceNoteKeyRunnableCPU)
	competitor := runtimeTraceObservationRichNoteValue(notes, types.TraceNoteKeyTopCompetitor)
	if cpu == "" && competitor == "" {
		return ""
	}
	// PTV8-RCR-B (UXA 域C #16 兄弟句, 2026-07-08): CJK/latin spacing (C26 先例).
	scope := "同 CPU"
	if cpu != "" {
		scope = fmt.Sprintf("同 CPU(cpu=%s)", cpu)
	}
	// PTV5 C26 (#68): "top 运行线程" spaced like the CMP-6 rows (no half-width
	// jam of latin+CJK).
	top := "top 运行线程"
	if competitor != "" {
		top += " " + competitor
	}
	return fmt.Sprintf("排查%s竞争:%s、优先级与 CPU 频率", scope, top)
}

// runtimeTraceNextStepChineseText maps the deterministic next_step_kind enum
// (published by trace_query alongside every system-fixed English next_step
// string) to its ZH guidance. Must stay in lockstep with the tracequery
// NextStepKind* constants; unknown/missing kinds take the generic guidance.
func runtimeTraceNextStepChineseText(kind string) string {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case "runnable":
		// PTV5 C26 (#68): spacing matches the dynamic variant above.
		return "排查同 CPU 竞争:top 运行线程、优先级与 CPU 频率"
	case "s_sleep":
		return "排查反复唤醒它的对端线程、binder等待、锁与条件变量等待"
	case "d_sleep_io":
		return "排查 sched_blocked_reason、块设备IO、文件系统、缺页与内存回收证据"
	case "running":
		return "排查该线程自身 trace span/帧阶段的 CPU 工作与被抢占的边界"
	case "priority_inversion":
		return "排查所依赖的低优先级线程的调度延迟,以及同窗口内的 CPU 压力"
	default:
		return "排查相邻的调度与资源事件"
	}
}

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

func trimRuntimeTraceNextStepText(s string) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if s == "" {
		return ""
	}
	const maxRunes = 360
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes-1]) + "…"
}

func materializeRuntimeTracePerfQualityBlock(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil || ctx.Mutable == nil {
		return false
	}
	if len(doc.Blocks) >= maxBlocksPerDoc {
		logging.Warning("[answer_document] runtime trace perf quality block skipped: document already at the %d-block cap", maxBlocksPerDoc)
		return false
	}
	if answerDocumentHasBlockID(doc, "runtime_trace_perf_quality") {
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
	insertAt := answerDocumentInsertionIndexBeforeCaveat(doc)
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
	for _, block := range doc.Blocks {
		if block.ID == "runtime_trace_facts" {
			return false
		}
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
	insertAt := answerDocumentInsertionIndexBeforeCaveat(doc)
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
	class, ok := uniqueHarmonyPriorityClass(strings.Join([]string{authoritySurface, *s}, " "), classMap)
	if !ok {
		return 0
	}
	hasPriorityAuthority := hasHarmonyPriorityClassSurface(authoritySurface, classMap) || hasHarmonyPriorityClassSurface(*s, classMap)
	lines := strings.Split(*s, "\n")
	fixed := 0
	for i, line := range lines {
		if line == "" || (harmonyPriorityLineIsRule(line) && !hasPriorityAuthority) {
			continue
		}
		before := line
		lines[i] = normalizeHarmonyPriorityLine(line, class, classMap)
		if lines[i] != before {
			fixed++
		}
	}
	if fixed > 0 {
		*s = strings.Join(lines, "\n")
	}
	return fixed
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
		strings.Contains(lower, "41-139") &&
		strings.Contains(lower, "cfs") &&
		strings.Contains(lower, "rt")
}

func normalizeHarmonyPriorityLine(line, class string, classMap map[int]string) string {
	switch class {
	case "ohos_rt":
		line = replaceHarmonyPriorityClassPhrases(line, "CFS", "RT", "1-40", "41-139", classMap)
	case "ohos_cfs":
		line = replaceHarmonyPriorityClassPhrases(line, "RT", "CFS", "41-139", "1-40", classMap)
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
		return []string{"41-139", "41–139"}
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
