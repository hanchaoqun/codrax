package tool

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/reasoninggraph"
	"github.com/hanchaoqun/codrax/internal/sourceowner"
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

func materializeRuntimeTraceCausalProjectionBlock(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil || ctx.Mutable == nil {
		return false
	}
	if len(doc.Blocks) >= maxBlocksPerDoc {
		logging.Warning("[answer_document] runtime trace causal projection block skipped: document already at the %d-block cap", maxBlocksPerDoc)
		return false
	}
	if answerDocumentHasBlockID(doc, "runtime_trace_causal_projection") {
		return false
	}
	if answerDocumentHasBlockID(doc, "runtime_trace_causal_projection_coverage") {
		return false
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	ledger := types.CompileObservationLedger(input)
	projection := types.CompileTraceCausalProjection(ledger)
	lang := requestedAnswerDocumentLanguage(ctx)
	cluster := runtimeTraceCausalProjectionCluster(projection, lang, runtimeTraceProjUserFocusFromBusContext(ctx))
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
	// The lead overview is the safest minimum user-facing surface. The secondary
	// blocks carry drilldown/evidence detail, so if the document is already near
	// the block cap, degrade to the overview instead of dropping the whole section.
	if len(doc.Blocks)+len(cluster) > maxBlocksPerDoc {
		cluster = cluster[:1]
		if len(doc.Blocks)+len(cluster) > maxBlocksPerDoc {
			logging.Warning("[answer_document] runtime trace causal projection block skipped: document already at the %d-block cap", maxBlocksPerDoc)
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
// window-scaled bars, ⚠/⛔ inline), then ONE lossless detail table, then a
// file-grouped evidence index. Zero mermaid — the fence is byte-identical
// across HTML / markdown / terminal. Every projection node appears exactly
// twice (one tree row + one table row).
func runtimeTraceCausalProjectionCluster(projection types.TraceCausalProjection, lang string, focus runtimeTraceProjUserFocus) []types.AnswerBlock {
	if !projection.Active() {
		return nil
	}
	zh := runtimeTraceCausalProjectionUseChinese(lang)
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	model := buildRuntimeTraceProjTreeModel(projection, evidence, zh)
	runtimeTraceProjApplyUserFocus(&model, focus)
	fence := runtimeTraceProjTreeFence(model, zh)
	if fence == "" {
		return nil
	}
	claimUses := []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}}
	facets := []string{"observed_artifact_fact"}
	leadText := runtimeTraceProjLeadText(projection, model, lang, zh) + "\n\n" + fence
	out := []types.AnswerBlock{{
		ID:          "runtime_trace_causal_projection",
		Kind:        types.BlockSection,
		Title:       runtimeTraceCausalProjectionTitle(lang),
		Text:        leadText,
		SurfaceRole: types.SurfacePrincipal,
		ClaimUses:   claimUses,
		FacetIDs:    facets,
	}}
	if columns, rows := runtimeTraceProjDetailTable(model, zh); len(rows) > 0 {
		title := "因果投影明细(无损)"
		if !zh {
			title = "Causal Projection Detail (lossless)"
		}
		// Customer 2026-07-03: the legend was one run-on paragraph and
		// unreadable — itemized, one short definition per line, kept plain.
		text := strings.Join([]string{
			"口径:",
			"- 窗口投影 = 节点在用户窗口内的投影影响。",
			"- 链上累计 = 该链路向目标累计投影。",
			"- 有效归因 = 排序/归因使用的有效影响。",
			"- 实际状态 = 底层状态实际持续时长。",
			"- 「—」 = 该口径对此节点无值。",
			"- ⛔ = 窗口内无匹配 sched_wakeup(missing_wakeup),下钻链止。",
			"- ⚠ = 实际状态跨出投影窗口。",
			"- 背景行仅作压力/环境证据,不自动等同 on-chain 主因。",
		}, "\n")
		if !zh {
			text = strings.Join([]string{
				"Legend:",
				"- window projection = the node's projected impact inside the user window.",
				"- chain total = cumulative projection toward the target.",
				"- attribution = the effective impact used for ranking/attribution.",
				"- actual state = the underlying state duration.",
				"- “—” = no value for this node.",
				"- ⛔ = no matching sched_wakeup in the window (missing_wakeup); the chain ends.",
				"- ⚠ = the actual state crosses the projected window.",
				"- Background rows are pressure/context evidence only.",
			}, "\n")
		}
		out = append(out, types.AnswerBlock{
			ID:          "runtime_trace_causal_projection_detail",
			Kind:        types.BlockTable,
			Title:       title,
			Text:        text,
			Columns:     columns,
			Items:       rows,
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
		out = append(out, types.AnswerBlock{
			ID:        "runtime_trace_causal_projection_evidence",
			Kind:      types.BlockBulletList,
			Title:     title,
			Text:      intro,
			Items:     items,
			ClaimUses: claimUses,
			FacetIDs:  facets,
		})
	}
	return out
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

func runtimeTraceCausalProjectionRepeatingPath(path []string) (int, int) {
	if len(path) < 6 {
		return 0, 0
	}
	maxPeriod := len(path) / 2
	if maxPeriod > 6 {
		maxPeriod = 6
	}
	for period := 1; period <= maxPeriod; period++ {
		if len(path)/period < 3 {
			continue
		}
		matches := true
		for i := range path {
			if runtimeTraceCausalProjectionCanonicalNode(path[i]) != runtimeTraceCausalProjectionCanonicalNode(path[i%period]) {
				matches = false
				break
			}
		}
		if matches {
			return period, len(path) / period
		}
	}
	return 0, 0
}

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
		ID:        "runtime_trace_causal_projection_coverage",
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
		if zh {
			return "大 trace 的 heavy view 需要 bounded 时间/行/span/pattern 范围"
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
			return "event_search 结果达到 limit"
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
		text := "本轮已获得 trace_query 的结构化执行记录,但没有产出可承重的 root_cause/wakeup_chain/semantic rows,因此未生成分层因果表。"
		if len(reasons) > 0 {
			text += " 结构化原因: " + strings.Join(reasons, "；") + "。"
		}
		text += " 这不是“没有背景影响”的结论;只表示当前证据没有给出可审计的因果/背景统计,应按 trace_query 的有界参数继续补 root_cause_rank、window_stats 或 interaction_stats。"
		return text
	}
	text := "This run has structured trace_query execution records, but no load-bearing root_cause/wakeup_chain/semantic rows were produced, so the layered causal table was not generated."
	if len(reasons) > 0 {
		text += " Typed reason: " + strings.Join(reasons, "; ") + "."
	}
	text += " This does not prove there was no background influence; it only means this run lacks auditable causal/background statistics. Continue with bounded trace_query parameters for root_cause_rank, window_stats, or interaction_stats."
	return text
}

type runtimeTraceCausalProjectionEvidenceIndex struct {
	order []runtimeTraceCausalProjectionEvidenceEntry
	seen  map[string]string
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
	window := ""
	if node.StartTs > 0 && node.EndTs > node.StartTs {
		window = fmt.Sprintf("[%.3f–%.3fs]", node.StartTs, node.EndTs)
	}
	idx.order = append(idx.order, runtimeTraceCausalProjectionEvidenceEntry{
		ID:      id,
		Ref:     strings.TrimSpace(ref),
		Window:  window,
		Details: runtimeTraceCausalProjectionAuditDetail(node, zh),
	})
	return id
}

func runtimeTraceCausalProjectionEvidenceText(zh bool) string {
	if zh {
		return "主表只引用短证据 ID;这里显示短定位和结构化审计摘要,完整定位以原始 trace_query 结构化记录为准。"
	}
	return "Main tables use short evidence IDs; this index shows short locators and typed audit summaries. The original trace_query record remains the full locator authority."
}

func runtimeTraceCausalProjectionPriorityCell(node types.TraceCausalProjectionNode, zh bool) string {
	switch {
	case node.Role == types.TraceCausalRolePrimaryRootCause || strings.HasPrefix(strings.TrimSpace(node.Predicate), "root_cause_primary"):
		if zh {
			return "主要关注"
		}
		return "primary focus"
	case node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span":
		if zh {
			return "确定优化"
		}
		return "optimize"
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
	if node.Role == types.TraceCausalRolePrimaryRootCause || strings.HasPrefix(strings.TrimSpace(node.Predicate), "root_cause_primary") {
		if zh {
			return "主根因"
		}
		return "primary"
	}
	switch strings.TrimSpace(node.ChainRelevance) {
	case "on_chain":
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
	causeKind := strings.TrimSpace(strings.ToLower(firstNonEmptyAnswerString(node.Object, node.Predicate)))
	stateKind := strings.TrimSpace(strings.ToLower(node.StateKind))
	if stateKind == "" {
		stateKind = causeKind
	}
	if node.IsSleepState() {
		if node.Undrillable() {
			if zh {
				return "睡眠症状·缺唤醒边"
			}
			return "sleep symptom·no wake edge"
		}
		if strings.TrimSpace(node.DrilldownTarget) != "" {
			if zh {
				return "睡眠症状→查上游"
			}
			return "sleep symptom→upstream"
		}
		if zh {
			return "睡眠症状"
		}
		return "sleep symptom"
	}
	if causeKind == "compute_supply" {
		if zh {
			return "执行/算力"
		}
		return "execution/CPU"
	}
	if node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span" {
		class := strings.TrimSpace(node.SemanticClass)
		if zh {
			if class != "" {
				return "优化点·" + runtimeTraceCausalProjectionCompactCellText(class, 22)
			}
			return "确定性优化点"
		}
		if class != "" {
			return "optimize·" + runtimeTraceCausalProjectionCompactCellText(class, 22)
		}
		return "optimization point"
	}
	switch stateKind {
	case "running":
		if zh {
			return "执行/算力"
		}
		return "execution/CPU"
	case "runnable":
		if zh {
			return "调度/优先级"
		}
		return "schedule/priority"
	case "d_state", "io_wait", "d_sleep", "uninterruptible_sleep":
		if zh {
			return "阻塞/IO"
		}
		return "blocking/IO"
	}
	if zh {
		return "候选根因"
	}
	return "candidate cause"
}

func runtimeTraceCausalProjectionImpactShapeCell(node types.TraceCausalProjectionNode, zh bool) string {
	// §7.30.3 D1: lock-contention rows carry a typed BlockingKind — the
	// duration is a blocked wait on a lock, so it always renders with that
	// semantic label instead of a bare number or a generic candidate word.
	if strings.TrimSpace(node.BlockingKind) != "" {
		if zh {
			return "锁竞争·阻塞"
		}
		return "lock contention · blocked"
	}
	// §7.30.3 D3: an inversion row's impact is the R5d gated COMPOSITE
	// (runnable full + discounted weak-core running) — no single scheduler
	// state may claim it.
	if runtimeTraceCausalProjectionInversionRow(node) {
		if zh {
			return "反转影响"
		}
		return "inversion impact"
	}
	state := strings.TrimSpace(strings.ToLower(node.StateKind))
	switch state {
	case "running":
		if zh {
			return "running / CPU执行"
		}
		return "running / CPU execution"
	case "runnable":
		if zh {
			return "runnable / 等待调度"
		}
		return "runnable / waiting for CPU"
	case "sleep", "s_sleep", "sleep_wait":
		if zh {
			return "sleep / 等待唤醒"
		}
		return "sleep / waiting for wakeup"
	case "d_state", "d_sleep", "uninterruptible_sleep":
		if zh {
			return "D-state / 不可中断等待"
		}
		return "D-state / uninterruptible wait"
	case "io_wait":
		if zh {
			return "IO wait / IO等待"
		}
		return "IO wait"
	}
	causeKind := strings.TrimSpace(strings.ToLower(firstNonEmptyAnswerString(node.Object, node.Predicate)))
	switch causeKind {
	case "compute_supply":
		if zh {
			return "CPU供给候选"
		}
		return "CPU-supply candidate"
	case "priority_inversion_runnable_wait":
		if zh {
			return "runnable调度候选"
		}
		return "runnable scheduling candidate"
	case "block_io_by_inode", "io_latency":
		if zh {
			return "IO阻塞候选"
		}
		return "IO-blocking candidate"
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
		return label
	}
	// H20: rows that would fall back to the generic candidate word but carry a
	// typed aggregate-activity kind (irq_burst / irq_activity / page_cache_churn)
	// render their typed shape instead. TypeToken (the verbatim producer enum)
	// first; the same token riding the Object cause lane second. Labels live in
	// the typelabels helper file — never scattered strings.
	if label := runtimeTraceAggregateTypeShapeLabel(node.TypeToken, zh); label != "" {
		return label
	}
	if label := runtimeTraceAggregateTypeShapeLabel(causeKind, zh); label != "" {
		return label
	}
	if zh {
		return "候选影响"
	}
	return "candidate impact"
}

func runtimeTraceCausalProjectionImpactMeaningCell(node types.TraceCausalProjectionNode, zh bool) string {
	action := runtimeTraceCausalProjectionActionCell(node, zh)
	causeKind := strings.TrimSpace(strings.ToLower(firstNonEmptyAnswerString(node.Object, node.Predicate)))
	stateKind := strings.TrimSpace(strings.ToLower(node.StateKind))
	if stateKind == "" {
		stateKind = causeKind
	}
	var meaning string
	switch {
	case node.IsSleepState() && node.Undrillable():
		if zh {
			meaning = "缺唤醒边"
		} else {
			meaning = "missing wake edge"
		}
	case node.IsSleepState():
		if zh {
			meaning = "下钻上游唤醒者"
		} else {
			meaning = "drill upstream waker"
		}
	case node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span":
		if zh {
			meaning = "确定性优化 span"
		} else {
			meaning = "deterministic span"
		}
	case causeKind == "priority_inversion_runnable_wait":
		if zh {
			meaning = "疑似优先级反转"
		} else {
			meaning = "possible priority inversion"
		}
	case causeKind == "compute_supply" || stateKind == "running":
		if zh {
			meaning = "本层运行/算力占用"
		} else {
			meaning = "local execution / CPU supply"
		}
	case stateKind == "runnable":
		if zh {
			meaning = "可运行但未获 CPU"
		} else {
			meaning = "runnable but not scheduled"
		}
	case stateKind == "d_state" || stateKind == "io_wait" || stateKind == "d_sleep" || stateKind == "uninterruptible_sleep":
		if zh {
			meaning = "本层资源/IO 等待"
		} else {
			meaning = "local resource / IO wait"
		}
	default:
		if zh {
			meaning = "候选影响层"
		} else {
			meaning = "candidate impact layer"
		}
	}
	if action == "" {
		return runtimeTraceCausalProjectionCompactCellText(meaning, 42)
	}
	if meaning == "" || meaning == action {
		return runtimeTraceCausalProjectionCompactCellText(action, 42)
	}
	return runtimeTraceCausalProjectionCompactCellText(action+": "+meaning, 42)
}

func runtimeTraceCausalProjectionAuditDetail(node types.TraceCausalProjectionNode, zh bool) string {
	var parts []string
	if tier := strings.TrimSpace(node.Tier); tier != "" {
		parts = append(parts, "tier="+tier)
	}
	if causality := strings.TrimSpace(node.Causality); causality != "" {
		parts = append(parts, "causality="+causality)
	}
	if node.Rank > 0 {
		parts = append(parts, fmt.Sprintf("rank=%d", node.Rank))
	}
	if node.Confidence > 0 {
		parts = append(parts, fmt.Sprintf("confidence=%.2f", node.Confidence))
	}
	if pred := strings.TrimSpace(node.Predicate); pred != "" {
		parts = append(parts, "predicate="+pred)
	}
	// Aggregation provenance (presentation v3 §6): every merged observation id
	// stays auditable from the roster entry.
	if node.MergedCount > 1 {
		parts = append(parts, fmt.Sprintf("merged_count=%d", node.MergedCount))
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

func runtimeTraceCausalProjectionIntro(lang string) string {
	if runtimeTraceCausalProjectionUseChinese(lang) {
		return "这部分由系统根据 trace_query 的结构化证据自动提炼：优先展示直接唤醒链或依赖链上的主因层，保留共同主因和 on-chain/off-chain 分层，再给出完整链路和关键支撑节点。括号中的影响时长、排序和证据来源用于定位原始 trace 证据，不是额外推测。"
	}
	return "This section is automatically distilled from structured trace_query evidence: it prioritizes root-cause layers on the direct wakeup/dependency chain, preserves co-primary and on-chain/off-chain layering, then shows the full path and supporting hops. Impact, rank, and source details in parentheses are trace-evidence locators, not extra speculation."
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
	if (node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span") && strings.TrimSpace(node.SpanName) != "" {
		object = strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(node.SpanName, zh))
	}
	objectLimit := 22
	if node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span" {
		objectLimit = 36
	}
	switch {
	case subject != "" && object != "":
		return runtimeTraceCausalProjectionCompactCellText(subject, 28) + " / " + runtimeTraceCausalProjectionCompactCellText(object, objectLimit)
	case subject != "":
		return runtimeTraceCausalProjectionCompactCellText(subject, 44)
	case object != "":
		return runtimeTraceCausalProjectionCompactCellText(object, 44)
	default:
		return "trace causal node"
	}
}

func runtimeTraceCausalProjectionDisplayNodeName(raw string, zh bool) string {
	raw = strings.TrimSpace(raw)
	if runtimeTraceCausalProjectionUnknownSentinel(raw) {
		// Raw-string lane: no node context, so the wording stays generic.
		return runtimeTraceCausalProjectionUnresolvedPeerText("", zh)
	}
	return raw
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
		if zh {
			return "D状态/IO等待(对端未解析)"
		}
		return "D-state/IO wait (peer unresolved)"
	default:
		if zh {
			return "对端线程未解析"
		}
		return "unresolved wait peer"
	}
}

// runtimeTraceCausalProjectionUnresolvedPeerKind derives the typed kind that
// specializes the unresolved-peer wording: an EXACT typed-token match on the
// node's verbatim TypeToken ("type=" rich note), Predicate, or Object — all
// producer-side typed enums, never model prose. "" when none matches (callers
// fall back to the generic wording).
func runtimeTraceCausalProjectionUnresolvedPeerKind(node types.TraceCausalProjectionNode) string {
	for _, token := range []string{node.TypeToken, node.Predicate, node.Object} {
		switch runtimeTraceCausalProjectionCanonicalNode(token) {
		case "blocking_span":
			return "blocking_span"
		case "d_state_or_io_wait":
			return "d_state_or_io_wait"
		}
	}
	return ""
}

// runtimeTraceCausalProjectionKnownSubject reports whether a subject names a
// real (possibly partial, pid-only) thread — not empty and not the
// unknown-thread sentinel. Precise typed check, never a prose heuristic.
func runtimeTraceCausalProjectionKnownSubject(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	return !runtimeTraceCausalProjectionUnknownSentinel(raw)
}

// runtimeTraceCausalProjectionInversionRow reports whether this node publishes
// the R5d GATED COMPOSITE impact (§7.30.3 D3) — exact typed token match on the
// row's cause type. Only these rows replace the single-state tag with the
// dedicated inversion-impact label and the gated composition split.
func runtimeTraceCausalProjectionInversionRow(node types.TraceCausalProjectionNode) bool {
	return runtimeTraceCausalProjectionCanonicalNode(node.Object) == "priority_inversion_candidate"
}

// runtimeTraceCausalProjectionBlockingName renders the typed lock-contention
// semantics (§7.30.3 D1): the row is a wait ON A LOCK, and when the structured
// payload named the owner, the holder is displayed with its thread label
// (name-tid). Empty when the node carries no typed BlockingKind.
func runtimeTraceCausalProjectionBlockingName(node types.TraceCausalProjectionNode, zh bool) string {
	if strings.TrimSpace(node.BlockingKind) == "" {
		return ""
	}
	name := "锁竞争等待"
	if !zh {
		name = "lock contention wait"
	}
	if peer := strings.TrimSpace(node.BlockingPeer); peer != "" {
		if zh {
			name += "(持有者 " + peer + ")"
		} else {
			name += " (owner " + peer + ")"
		}
	}
	return name
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

func runtimeTraceCausalProjectionEvidenceRefShortened(raw, display string) bool {
	raw = strings.TrimSpace(raw)
	display = strings.TrimSpace(display)
	return raw != "" && display != "" && raw != display
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
	projection := types.CompileTraceCausalProjection(ledger)
	if len(projection.SemanticSpans) == 0 {
		return false
	}
	lang := requestedAnswerDocumentLanguage(ctx)
	zh := runtimeTraceCausalProjectionUseChinese(lang)
	// Rebuild the projection cluster's deterministic evidence numbering so the
	// E# tags in this block match the cluster's evidence index (same ledger,
	// same compile, same model walk — pure and deterministic).
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	buildRuntimeTraceProjTreeModel(projection, evidence, zh)
	columns, rows := runtimeTraceSemanticOptimizationParts(projection, evidence, zh)
	if len(rows) == 0 {
		return false
	}
	title := "确定性优化点"
	text := "trace 中的确定性语义优化 span(类校验/JIT/着色器编译等,来自 typed semantic_class 通道):每行都是可直接落地的优化点;时长与 E# 证据均可定位到原始 trace_query 结构化记录。"
	if !zh {
		title = "Deterministic Optimization Points"
		text = "Deterministic semantic optimization spans found in the trace (class verification / JIT / shader compilation etc., from the typed semantic_class lane): each row is a directly actionable optimization point; durations and E# tags locate the original structured trace_query records."
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
	seen := make(map[string]bool)
	var out []types.AnswerBlockItem
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
		text := runtimeTraceMetricSnapshotDisplayText(record, zh)
		if text == "" {
			text = raw
		}
		label := strings.TrimSpace(record.Subject)
		if label == "" {
			label = "state_churn"
		} else {
			label += " state_churn"
		}
		out = append(out, types.AnswerBlockItem{
			ID:          fmt.Sprintf("runtime_trace_metric_snapshot_%d", len(out)+1),
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
	keys := append([]string{"dominant_state", "total"}, required...)
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
	"running",
	"runnable",
	"sleep",
	"d_state",
	"io_wait",
	"fragments",
	"switches",
	"max_segment",
	"p95_segment",
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
	if values["dominant_state"] != "" {
		parts = append(parts, "dominant_state="+values["dominant_state"])
	}
	for _, key := range []string{"running", "runnable", "sleep", "d_state", "io_wait"} {
		parts = append(parts, key+"="+runtimeTraceMetricWithMS(values[key]))
	}
	for _, key := range []string{"fragments", "switches"} {
		parts = append(parts, key+"="+values[key])
	}
	for _, key := range []string{"max_segment", "p95_segment"} {
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
	total := runtimeTraceMetricFloat(values["total"])
	share := func(key string) string {
		v := runtimeTraceMetricFloat(values[key])
		if total <= 0 || v <= 0 {
			return ""
		}
		return fmt.Sprintf("%.0f%%", v/total*100)
	}
	stateEntry := func(label, key string) string {
		entry := label + " " + ms(key)
		if pct := share(key); pct != "" {
			// H13: the share denominator is the thread's OWN observed state
			// total, not the analysis window — say so, or "运行 2.891ms(占100%)"
			// reads as "the thread filled the window" (berlin customer misread).
			if zh {
				entry += "(占该线程观测时长" + pct + ")"
			} else {
				entry += " (" + pct + " of this thread's observed span)"
			}
		}
		return entry
	}
	var parts []string
	if dominant := strings.TrimSpace(values["dominant_state"]); dominant != "" {
		label := runtimeTraceProjStateKindLabel(types.TraceCausalProjectionNode{StateKind: dominant}, zh)
		entry := dominant
		if label != "" {
			entry = dominant + "(" + label + ")"
			if !zh {
				entry = dominant + " (" + label + ")"
			}
		}
		if zh {
			parts = append(parts, "主导状态 "+entry)
		} else {
			parts = append(parts, "dominant state "+entry)
		}
	}
	if zh {
		parts = append(parts,
			stateEntry("运行", "running"),
			stateEntry("可运行", "runnable"),
			stateEntry("睡眠", "sleep"),
			stateEntry("D状态", "d_state"),
			stateEntry("IO等待", "io_wait"),
			"状态段数 "+values["fragments"],
			"切换次数 "+values["switches"],
			"最长单段 "+ms("max_segment"),
			"P95段长 "+ms("p95_segment"),
		)
	} else {
		parts = append(parts,
			stateEntry("running", "running"),
			stateEntry("runnable", "runnable"),
			stateEntry("sleep", "sleep"),
			stateEntry("D-state", "d_state"),
			stateEntry("IO wait", "io_wait"),
			"state segments "+values["fragments"],
			"switches "+values["switches"],
			"longest segment "+ms("max_segment"),
			"p95 segment "+ms("p95_segment"),
		)
	}
	sep := "; "
	if zh {
		sep = ";"
	}
	text := strings.Join(parts, sep)
	if runtimeTraceRecordHasActualWindowValues(record) {
		if zh {
			text += ";窗口基准: 选定窗(实际对齐窗数值见原始 trace_query 记录)"
		} else {
			text += "; window basis: selected window (aligned actual-window values remain in the raw trace_query record)"
		}
	}
	return text
}

// runtimeTraceRecordHasActualWindowValues reports whether the record publishes
// any actual_* (aligned/underlying-window) rich note alongside its
// selected-window values — the §7.30 S1 dual-basis shape that must be labeled.
func runtimeTraceRecordHasActualWindowValues(record types.ObservationRecord) bool {
	for _, note := range record.RichNotes {
		if strings.HasPrefix(strings.TrimSpace(note), "actual_") {
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
	if runtimeTracePositiveMetric(runtimeTraceObservationRichNoteValue(record.RichNotes, "impact")) {
		return true
	}
	values := map[string]string{}
	runtimeTraceMergeSummaryMetricTokens(values, record.Summary, []string{"impact"})
	return runtimeTracePositiveMetric(values["impact"])
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

func runtimeTraceNextStepItems(doc *types.AnswerDocumentV2, ctx *types.BusContext) []types.AnswerBlockItem {
	if doc == nil || ctx == nil {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, 64))
	if len(ledger.Records) == 0 {
		return nil
	}
	zh := runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx))
	label := "下一步"
	if !zh {
		label = "Next step"
	}
	seen := make(map[string]bool)
	seenText := make(map[string]bool)
	var out []types.AnswerBlockItem
	for _, record := range ledger.Records {
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
		if len(out) >= 4 {
			break
		}
	}
	return out
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
	step := trimRuntimeTraceNextStepText(runtimeTraceObservationRichNoteValue(record.RichNotes, "next_step"))
	if step == "" || !zh {
		return step
	}
	kind := runtimeTraceObservationRichNoteValue(record.RichNotes, "next_step_kind")
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
		runtimeTraceObservationRichNoteValue(record.RichNotes, "next_step_kind"),
		trimRuntimeTraceNextStepText(runtimeTraceObservationRichNoteValue(record.RichNotes, "next_step")),
		runtimeTraceObservationRichNoteValue(record.RichNotes, "runnable_cpu"),
		runtimeTraceObservationRichNoteValue(record.RichNotes, "top_competitor"),
	}, "\x00")
}

// runtimeTraceNextStepRunnableChineseText composes the runnable-kind ZH
// guidance from the typed state_churn rich notes (runnable_cpu /
// top_competitor), so the dynamic same-CPU competitor data carried by the
// English prose variants survives into a Chinese panel. Empty when neither
// typed note exists — the caller then falls back to the generic runnable
// guidance instead of fabricating data.
func runtimeTraceNextStepRunnableChineseText(notes []string) string {
	cpu := runtimeTraceObservationRichNoteValue(notes, "runnable_cpu")
	competitor := runtimeTraceObservationRichNoteValue(notes, "top_competitor")
	if cpu == "" && competitor == "" {
		return ""
	}
	scope := "同CPU"
	if cpu != "" {
		scope = fmt.Sprintf("同CPU(cpu=%s)", cpu)
	}
	top := "top运行线程"
	if competitor != "" {
		top += " " + competitor
	}
	return fmt.Sprintf("排查%s竞争:%s、优先级与CPU频率", scope, top)
}

// runtimeTraceNextStepChineseText maps the deterministic next_step_kind enum
// (published by trace_query alongside every system-fixed English next_step
// string) to its ZH guidance. Must stay in lockstep with the tracequery
// NextStepKind* constants; unknown/missing kinds take the generic guidance.
func runtimeTraceNextStepChineseText(kind string) string {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case "runnable":
		return "排查同CPU竞争:top运行线程、优先级与CPU频率"
	case "s_sleep":
		return "排查反复唤醒它的对端线程、binder等待、锁与条件变量等待"
	case "d_sleep_io":
		return "排查 sched_blocked_reason、块设备IO、文件系统、缺页与内存回收证据"
	case "running":
		return "排查该线程自身的trace span/帧阶段的CPU工作与被抢占边界"
	case "priority_inversion":
		return "排查低优先级依赖的调度延迟与同窗口CPU压力"
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
	quality := runtimeTraceObservationRichNoteValue(record.RichNotes, "perf_quality")
	if quality == "" {
		return ""
	}
	values := map[string]string{}
	runtimeTraceMergeSummaryMetricTokens(values, quality, []string{"source", "sample_kind", "weight_unit", "symbolization", "symbolization_status", "cpu_known", "cpu_unknown", "sample_cpu_scope", "clock", "clock_confidence", "callchain_status"})
	dso := runtimeTraceObservationRichNoteValue(record.RichNotes, "dso")
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
