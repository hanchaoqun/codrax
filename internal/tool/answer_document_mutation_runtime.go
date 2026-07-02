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
	input := types.ObservationLedgerInputFromBusContext(ctx, 128)
	ledger := types.CompileObservationLedger(input)
	projection := types.CompileTraceCausalProjection(ledger)
	lang := requestedAnswerDocumentLanguage(ctx)
	cluster := runtimeTraceCausalProjectionCluster(projection, lang)
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

// runtimeTraceCausalProjectionCluster builds the presentation block cluster for
// the Trace Causal Projection section. The lead block is intentionally a compact
// user-facing overview; chain, impact, background, and evidence-index blocks
// carry the remaining dimensions so no single table becomes unreadable.
func runtimeTraceCausalProjectionCluster(projection types.TraceCausalProjection, lang string) []types.AnswerBlock {
	if !projection.Active() {
		return nil
	}
	zh := runtimeTraceCausalProjectionUseChinese(lang)
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	overviewColumns, overviewRows := runtimeTraceCausalProjectionOverviewTable(projection, evidence, zh)
	if len(overviewRows) == 0 {
		return nil
	}
	claimUses := []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}}
	facets := []string{"observed_artifact_fact"}
	table := types.AnswerBlock{
		ID:          "runtime_trace_causal_projection",
		Kind:        types.BlockTable,
		Title:       runtimeTraceCausalProjectionTitle(lang),
		Text:        runtimeTraceCausalProjectionClusterIntro(projection, lang, zh),
		Columns:     overviewColumns,
		Items:       overviewRows,
		SurfaceRole: types.SurfacePrincipal,
		ClaimUses:   claimUses,
		FacetIDs:    facets,
	}
	out := []types.AnswerBlock{table}
	if columns, rows := runtimeTraceCausalProjectionOnChainTable(projection, evidence, zh); len(rows) > 0 {
		title := "On-chain 链路拆解"
		if !zh {
			title = "On-chain Chain Breakdown"
		}
		out = append(out, types.AnswerBlock{
			ID:          "runtime_trace_causal_projection_on_chain",
			Kind:        types.BlockTable,
			Title:       title,
			Text:        runtimeTraceCausalProjectionOnChainText(zh),
			Columns:     columns,
			Items:       rows,
			SurfaceRole: types.SurfacePrincipal,
			ClaimUses:   claimUses,
			FacetIDs:    facets,
		})
	}
	if columns, rows := runtimeTraceCausalProjectionImpactTable(projection, evidence, zh); len(rows) > 0 {
		title := "影响时长拆解"
		if !zh {
			title = "Impact Duration Breakdown"
		}
		out = append(out, types.AnswerBlock{
			ID:          "runtime_trace_causal_projection_impact",
			Kind:        types.BlockTable,
			Title:       title,
			Text:        runtimeTraceCausalProjectionImpactText(zh),
			Columns:     columns,
			Items:       rows,
			SurfaceRole: types.SurfacePrincipal,
			ClaimUses:   claimUses,
			FacetIDs:    facets,
		})
	}
	if columns, rows := runtimeTraceCausalProjectionBackgroundTable(projection, evidence, zh); len(rows) > 0 {
		title := "背景支撑"
		if !zh {
			title = "Background Support"
		}
		out = append(out, types.AnswerBlock{
			ID:        "runtime_trace_causal_projection_background",
			Kind:      types.BlockTable,
			Title:     title,
			Text:      runtimeTraceCausalProjectionBackgroundText(zh),
			Columns:   columns,
			Items:     rows,
			ClaimUses: claimUses,
			FacetIDs:  facets,
		})
	}
	if body := runtimeTraceCausalProjectionWakeupDiagram(projection, zh); body != "" {
		caption := "唤醒链拓扑(实线=on-chain 主链,虚线=off-chain 背景):"
		if !zh {
			caption = "Wakeup-chain topology (solid = on-chain trunk, dashed = off-chain background):"
		}
		out = append(out, types.AnswerBlock{
			ID:          "runtime_trace_causal_projection_wakeup",
			Kind:        types.BlockDiagram,
			Text:        caption,
			Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: body},
			SurfaceRole: types.SurfacePrincipal,
			ClaimUses:   claimUses,
			FacetIDs:    facets,
		})
	}
	if body := runtimeTraceCausalProjectionSleepDiagram(projection, zh); body != "" {
		caption := "sleep 下钻树(sleep=症状必须下钻;无匹配 sched_wakeup 时显式标注无法下钻):"
		if !zh {
			caption = "Sleep drilldown (sleep = symptom to drill; explicitly marked when no matching sched_wakeup exists):"
		}
		out = append(out, types.AnswerBlock{
			ID:          "runtime_trace_causal_projection_sleep",
			Kind:        types.BlockDiagram,
			Text:        caption,
			Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: body},
			SurfaceRole: types.SurfacePrincipal,
			ClaimUses:   claimUses,
			FacetIDs:    facets,
		})
	}
	if items := runtimeTraceCausalProjectionEvidenceItems(evidence, zh); len(items) > 0 {
		title := "证据索引"
		if !zh {
			title = "Evidence Index"
		}
		out = append(out, types.AnswerBlock{
			ID:        "runtime_trace_causal_projection_evidence",
			Kind:      types.BlockBulletList,
			Title:     title,
			Text:      runtimeTraceCausalProjectionEvidenceText(zh),
			Items:     items,
			ClaimUses: claimUses,
			FacetIDs:  facets,
		})
	}
	return out
}

func runtimeTraceCausalProjectionClusterIntro(projection types.TraceCausalProjection, lang string, zh bool) string {
	intro := runtimeTraceCausalProjectionIntro(lang)
	if len(projection.WakeupPath) >= 2 {
		pathView := runtimeTraceCausalProjectionBoundedPathView(projection.WakeupPath, 8)
		chain := runtimeTraceCausalProjectionFormatPathView(pathView, zh)
		note := runtimeTraceCausalProjectionPathViewNote(pathView, zh)
		if zh {
			intro += fmt.Sprintf("\n\n唤醒链: `%s`%s — 阅读方向为上游唤醒/依赖者逐级影响目标线程;💤 sleep 行本身不是根因,其下钻根因见「下钻→」列,`⛔` 表示窗口内无匹配 sched_wakeup、无法下钻。", chain, note)
		} else {
			intro += fmt.Sprintf("\n\nWakeup chain: `%s`%s — read upstream waker/dependency to target; a 💤 sleep row is a symptom (see the drilldown column), and `⛔` marks a sleep with no matching sched_wakeup in the window (cannot drill further).", chain, note)
		}
	}
	if len(projection.BackgroundCauses) == 0 {
		if zh {
			intro += "\n\n背景层: 当前结构化 trace_query 结果没有产出可承重的 off-chain/background 行;这不等于背景没有影响,只表示本轮证据没有给出可审计的背景统计。"
		} else {
			intro += "\n\nBackground layer: the structured trace_query result did not produce load-bearing off-chain/background rows. This does not prove there was no background influence; it only means this run lacks auditable background statistics."
		}
	}
	return intro
}

type runtimeTraceCausalProjectionPathView struct {
	Prefix        []string
	Suffix        []string
	OriginalCount int
	OmittedCount  int
	RepeatPeriod  int
	RepeatCount   int
}

func runtimeTraceCausalProjectionBoundedPathView(path []string, maxDisplayNodes int) runtimeTraceCausalProjectionPathView {
	clean := runtimeTraceCausalProjectionCleanPath(path)
	view := runtimeTraceCausalProjectionPathView{OriginalCount: len(clean)}
	view.RepeatPeriod, view.RepeatCount = runtimeTraceCausalProjectionRepeatingPath(clean)
	if len(clean) == 0 {
		return view
	}
	if maxDisplayNodes < 4 {
		maxDisplayNodes = 4
	}
	if len(clean) <= maxDisplayNodes {
		view.Prefix = clean
		return view
	}
	head := maxDisplayNodes / 2
	tail := maxDisplayNodes - head - 1
	if tail < 1 {
		tail = 1
	}
	if head < 1 {
		head = 1
	}
	if head+tail >= len(clean) {
		view.Prefix = clean
		return view
	}
	view.Prefix = append([]string{}, clean[:head]...)
	view.Suffix = append([]string{}, clean[len(clean)-tail:]...)
	view.OmittedCount = len(clean) - head - tail
	return view
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

func runtimeTraceCausalProjectionFormatPathView(view runtimeTraceCausalProjectionPathView, zh bool) string {
	var parts []string
	parts = append(parts, view.Prefix...)
	if view.OmittedCount > 0 {
		if zh {
			parts = append(parts, fmt.Sprintf("…省略%d节点…", view.OmittedCount))
		} else {
			parts = append(parts, fmt.Sprintf("…%d omitted…", view.OmittedCount))
		}
	}
	parts = append(parts, view.Suffix...)
	return strings.Join(parts, " ▸ ")
}

func runtimeTraceCausalProjectionPathViewNote(view runtimeTraceCausalProjectionPathView, zh bool) string {
	if view.OmittedCount <= 0 {
		return ""
	}
	if zh {
		note := fmt.Sprintf("（原始链路共%d节点/%d跳,中间已压缩%d节点", view.OriginalCount, max(0, view.OriginalCount-1), view.OmittedCount)
		if view.RepeatPeriod > 0 && view.RepeatCount > 0 {
			note += fmt.Sprintf(",检测到%d节点循环约%d轮", view.RepeatPeriod, view.RepeatCount)
		}
		return note + ";完整链路见原始 trace_query 记录）"
	}
	note := fmt.Sprintf(" (original chain has %d nodes/%d hops; %d middle nodes compressed", view.OriginalCount, max(0, view.OriginalCount-1), view.OmittedCount)
	if view.RepeatPeriod > 0 && view.RepeatCount > 0 {
		note += fmt.Sprintf("; detected an approximately %d-node cycle repeated %d times", view.RepeatPeriod, view.RepeatCount)
	}
	return note + "; full chain remains in the original trace_query record)"
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
			text += " typed 原因: " + strings.Join(reasons, "；") + "。"
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
	ID      string
	Ref     string
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
	idx.order = append(idx.order, runtimeTraceCausalProjectionEvidenceEntry{
		ID:      id,
		Ref:     strings.TrimSpace(ref),
		Details: runtimeTraceCausalProjectionAuditDetail(node, zh),
	})
	return id
}

func runtimeTraceCausalProjectionOverviewTable(projection types.TraceCausalProjection, evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) ([]string, []types.AnswerBlockItem) {
	columns := []string{"关注", "根因/节点", "影响", "处理方向", "证据"}
	if !zh {
		columns = []string{"Focus", "Cause / node", "Impact", "Action", "Evidence"}
	}
	var rows []types.AnswerBlockItem
	for _, node := range runtimeTraceCausalProjectionOverviewNodes(projection) {
		rows = append(rows, types.AnswerBlockItem{
			Cells: []string{
				runtimeTraceCausalProjectionFocusCell(node, zh),
				runtimeTraceCausalProjectionNodeSubjectCell(node),
				runtimeTraceCausalProjectionImpactSummaryCell(node, zh),
				runtimeTraceCausalProjectionActionCell(node, zh),
				evidence.add(node, zh),
			},
			CitationRef: -1,
		})
	}
	return columns, rows
}

func runtimeTraceCausalProjectionOverviewNodes(projection types.TraceCausalProjection) []types.TraceCausalProjectionNode {
	seen := map[string]bool{}
	var out []types.TraceCausalProjectionNode
	const maxOverviewRows = 8
	add := func(nodes []types.TraceCausalProjectionNode, bucketLimit int) {
		added := 0
		for _, node := range nodes {
			if len(out) >= maxOverviewRows {
				return
			}
			key := runtimeTraceCausalProjectionNodeKey(node)
			if seen[key] {
				continue
			}
			if bucketLimit > 0 && added >= bucketLimit {
				return
			}
			seen[key] = true
			out = append(out, node)
			added++
		}
	}
	primary := runtimeTraceCausalProjectionPrimaryRoots(projection)
	add(primary, 4)
	add(projection.SemanticSpans, 2)
	add(projection.OnChainCauses, 4)
	add(projection.AdjacentCauses, 1)
	// Fill any remaining first-screen slots from the same typed surfaces. The
	// detailed on-chain/impact/background/evidence views still carry the full
	// bounded projection; this overview is intentionally a compact decision face.
	add(primary, 0)
	add(projection.SemanticSpans, 0)
	add(projection.OnChainCauses, 0)
	add(projection.AdjacentCauses, 0)
	return out
}

func runtimeTraceCausalProjectionOnChainTable(projection types.TraceCausalProjection, evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) ([]string, []types.AnswerBlockItem) {
	columns := []string{"层", "链路", "本层含义", "影响", "证据"}
	if !zh {
		columns = []string{"Layer", "Chain", "Meaning", "Impact", "Evidence"}
	}
	nodes := runtimeTraceCausalProjectionOnChainNodes(projection)
	if len(nodes) == 0 {
		return columns, nil
	}
	var rows []types.AnswerBlockItem
	pathIndex := runtimeTraceCausalProjectionLocalPathIndex(projection.WakeupPath)
	for _, node := range nodes {
		rows = append(rows, types.AnswerBlockItem{
			Cells: []string{
				runtimeTraceCausalProjectionDepthCell(node, pathIndex, zh),
				runtimeTraceCausalProjectionChainCell(node, projection.WakeupPath),
				runtimeTraceCausalProjectionImpactMeaningCell(node, zh),
				runtimeTraceCausalProjectionOnChainImpactPairCell(node, zh),
				evidence.add(node, zh),
			},
			CitationRef: -1,
		})
	}
	return columns, rows
}

func runtimeTraceCausalProjectionImpactTable(projection types.TraceCausalProjection, evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) ([]string, []types.AnswerBlockItem) {
	columns := []string{"节点", "强度", "链上累计", "本节点投影", "有效归因", "实际状态", "窗口", "证据"}
	if !zh {
		columns = []string{"Node", "Magnitude", "Chain total", "Node projection", "Attribution", "Actual state", "Window", "Evidence"}
	}
	nodes := runtimeTraceCausalProjectionImpactNodes(projection)
	if len(nodes) == 0 {
		return columns, nil
	}
	bucketMax := runtimeTraceCausalProjectionBucketMax(projection)
	var rows []types.AnswerBlockItem
	for _, node := range nodes {
		rows = append(rows, types.AnswerBlockItem{
			Cells: []string{
				runtimeTraceCausalProjectionNodeSubjectCell(node),
				runtimeTraceCausalProjectionBar(runtimeTraceCausalProjectionNodeImpact(node), bucketMax),
				runtimeTraceCausalProjectionMSCell(node.CumulativeImpactMS),
				runtimeTraceCausalProjectionMSCell(node.ImpactMS),
				runtimeTraceCausalProjectionMSCell(node.EffectiveImpactMS),
				runtimeTraceCausalProjectionMSCell(node.ActualImpactMS),
				runtimeTraceCausalProjectionWindowCell(node, zh),
				evidence.add(node, zh),
			},
			CitationRef: -1,
		})
	}
	return columns, rows
}

func runtimeTraceCausalProjectionBackgroundTable(projection types.TraceCausalProjection, evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) ([]string, []types.AnswerBlockItem) {
	columns := []string{"节点", "状态", "影响", "保守解释", "证据"}
	if !zh {
		columns = []string{"Node", "State", "Impact", "Conservative reading", "Evidence"}
	}
	if len(projection.BackgroundCauses) == 0 {
		return columns, nil
	}
	var rows []types.AnswerBlockItem
	for _, node := range projection.BackgroundCauses {
		rows = append(rows, types.AnswerBlockItem{
			Cells: []string{
				runtimeTraceCausalProjectionNodeSubjectCell(node),
				runtimeTraceCausalProjectionStateCell(node, zh),
				runtimeTraceCausalProjectionImpactSummaryCell(node, zh),
				runtimeTraceCausalProjectionBackgroundWhyCell(zh),
				evidence.add(node, zh),
			},
			CitationRef: -1,
		})
	}
	return columns, rows
}

func runtimeTraceCausalProjectionEvidenceItems(evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) []types.AnswerBlockItem {
	if evidence == nil || len(evidence.order) == 0 {
		return nil
	}
	items := make([]types.AnswerBlockItem, 0, len(evidence.order))
	for _, entry := range evidence.order {
		locator := runtimeTraceCausalProjectionEvidenceDisplayRef(entry.Ref)
		audit := runtimeTraceCausalProjectionCompactCellText(entry.Details, 72)
		if locator == "" {
			locator = "trace_query"
		}
		var text string
		if zh {
			text = fmt.Sprintf("定位: %s；审计: %s", locator, audit)
			if runtimeTraceCausalProjectionEvidenceRefShortened(entry.Ref, locator) {
				text += "；完整定位见原始 trace_query 记录"
			}
		} else {
			text = fmt.Sprintf("locator: %s; audit: %s", locator, audit)
			if runtimeTraceCausalProjectionEvidenceRefShortened(entry.Ref, locator) {
				text += "; full locator remains in the original trace_query record"
			}
		}
		items = append(items, types.AnswerBlockItem{
			ID:          strings.ToLower(entry.ID),
			Label:       entry.ID,
			Text:        text,
			CitationRef: -1,
		})
	}
	return items
}

func runtimeTraceCausalProjectionOnChainText(zh bool) string {
	if zh {
		return "只展示直接唤醒/依赖链上的节点:这一块回答“谁影响谁、每一层影响是什么”。"
	}
	return "Only direct wakeup/dependency-chain nodes are shown here; this answers who affected whom and what each layer contributed."
}

func runtimeTraceCausalProjectionImpactText(zh bool) string {
	if zh {
		return "把影响时长拆开看:累计=链上累计影响,投影=当前节点投影影响,有效=排序/归因使用的有效影响,实际=底层状态实际持续时长。"
	}
	return "Impact durations are split out: cumulative = chain cumulative impact, projected = this node's projected impact, effective = ranking/attribution impact, actual = underlying state duration."
}

func runtimeTraceCausalProjectionBackgroundText(zh bool) string {
	if zh {
		return "背景支撑只作为压力/环境证据展示,不自动等同于 on-chain 主因。"
	}
	return "Background support is shown as pressure/context evidence and is not automatically treated as an on-chain root cause."
}

func runtimeTraceCausalProjectionEvidenceText(zh bool) string {
	if zh {
		return "主表只引用短证据 ID;这里显示短定位和 typed 审计摘要,完整定位以原始 trace_query 结构化记录为准。"
	}
	return "Main tables use short evidence IDs; this index shows short locators and typed audit summaries. The original trace_query record remains the full locator authority."
}

func runtimeTraceCausalProjectionOnChainNodes(projection types.TraceCausalProjection) []types.TraceCausalProjectionNode {
	seen := map[string]bool{}
	var out []types.TraceCausalProjectionNode
	add := func(nodes []types.TraceCausalProjectionNode) {
		for _, node := range nodes {
			if strings.TrimSpace(node.ChainRelevance) != "on_chain" && !runtimeTraceCausalProjectionLocalNodeOnChain(node) {
				continue
			}
			key := runtimeTraceCausalProjectionNodeKey(node)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, node)
		}
	}
	add(runtimeTraceCausalProjectionPrimaryRoots(projection))
	add(projection.OnChainCauses)
	add(projection.SemanticSpans)
	add(projection.SupportingHops)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ChainDepth > 0 && out[j].ChainDepth > 0 && out[i].ChainDepth != out[j].ChainDepth {
			return out[i].ChainDepth < out[j].ChainDepth
		}
		return runtimeTraceCausalProjectionNodeImpact(out[i]) > runtimeTraceCausalProjectionNodeImpact(out[j])
	})
	return out
}

func runtimeTraceCausalProjectionImpactNodes(projection types.TraceCausalProjection) []types.TraceCausalProjectionNode {
	nodes := runtimeTraceCausalProjectionAllNodes(projection)
	out := nodes[:0:0]
	for _, node := range nodes {
		if runtimeTraceCausalProjectionNodeImpact(node) > 0 {
			out = append(out, node)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return runtimeTraceCausalProjectionNodeImpact(out[i]) > runtimeTraceCausalProjectionNodeImpact(out[j])
	})
	if len(out) > 24 {
		out = out[:24]
	}
	return out
}

func runtimeTraceCausalProjectionAllNodes(projection types.TraceCausalProjection) []types.TraceCausalProjectionNode {
	seen := map[string]bool{}
	var out []types.TraceCausalProjectionNode
	add := func(nodes []types.TraceCausalProjectionNode) {
		for _, node := range nodes {
			key := runtimeTraceCausalProjectionNodeKey(node)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, node)
		}
	}
	add(runtimeTraceCausalProjectionPrimaryRoots(projection))
	add(projection.SemanticSpans)
	add(projection.OnChainCauses)
	add(projection.AdjacentCauses)
	add(projection.BackgroundCauses)
	add(projection.SupportingHops)
	return out
}

func runtimeTraceCausalProjectionPriorityCell(node types.TraceCausalProjectionNode, zh bool) string {
	switch {
	case node.Role == types.TraceCausalRolePrimaryRootCause || strings.HasPrefix(strings.TrimSpace(node.Predicate), "root_cause_primary"):
		return "P0"
	case node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span":
		return "P1"
	case strings.TrimSpace(node.ChainRelevance) == "on_chain":
		return "P1"
	case strings.TrimSpace(node.ChainRelevance) == "adjacent":
		return "P2"
	default:
		return "P2"
	}
}

func runtimeTraceCausalProjectionFocusCell(node types.TraceCausalProjectionNode, zh bool) string {
	priority := runtimeTraceCausalProjectionPriorityCell(node, zh)
	layer := runtimeTraceCausalProjectionLayerCell(node, zh)
	switch {
	case priority != "" && layer != "":
		return priority + " · " + layer
	case priority != "":
		return priority
	default:
		return layer
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
		if node.ChainDepth > 0 {
			if zh {
				return fmt.Sprintf("on-chain d%d", node.ChainDepth)
			}
			return fmt.Sprintf("on-chain d%d", node.ChainDepth)
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

func runtimeTraceCausalProjectionImpactSummaryCell(node types.TraceCausalProjectionNode, zh bool) string {
	if node.CumulativeImpactMS > 0 {
		if zh {
			return fmt.Sprintf("累计 %.3fms", node.CumulativeImpactMS)
		}
		return fmt.Sprintf("cum %.3fms", node.CumulativeImpactMS)
	}
	if node.EffectiveImpactMS > 0 {
		if zh {
			return fmt.Sprintf("有效 %.3fms", node.EffectiveImpactMS)
		}
		return fmt.Sprintf("eff %.3fms", node.EffectiveImpactMS)
	}
	if node.ImpactMS > 0 {
		if zh {
			return fmt.Sprintf("投影 %.3fms", node.ImpactMS)
		}
		return fmt.Sprintf("proj %.3fms", node.ImpactMS)
	}
	if node.ActualImpactMS > 0 {
		if zh {
			return fmt.Sprintf("实际 %.3fms", node.ActualImpactMS)
		}
		return fmt.Sprintf("act %.3fms", node.ActualImpactMS)
	}
	return ""
}

func runtimeTraceCausalProjectionOnChainImpactCell(node types.TraceCausalProjectionNode) string {
	if node.CumulativeImpactMS > 0 {
		return runtimeTraceCausalProjectionMSCell(node.CumulativeImpactMS)
	}
	if node.EffectiveImpactMS > 0 {
		return runtimeTraceCausalProjectionMSCell(node.EffectiveImpactMS)
	}
	return runtimeTraceCausalProjectionMSCell(node.ImpactMS)
}

func runtimeTraceCausalProjectionLocalImpactCell(node types.TraceCausalProjectionNode) string {
	if node.ImpactMS > 0 {
		return runtimeTraceCausalProjectionMSCell(node.ImpactMS)
	}
	if node.ActualImpactMS > 0 {
		return runtimeTraceCausalProjectionMSCell(node.ActualImpactMS)
	}
	return runtimeTraceCausalProjectionMSCell(node.EffectiveImpactMS)
}

func runtimeTraceCausalProjectionOnChainImpactPairCell(node types.TraceCausalProjectionNode, zh bool) string {
	chain := runtimeTraceCausalProjectionOnChainImpactCell(node)
	local := runtimeTraceCausalProjectionLocalImpactCell(node)
	switch {
	case chain != "" && local != "":
		if zh {
			return "链 " + chain + " / 本 " + local
		}
		return "chain " + chain + " / node " + local
	case chain != "":
		if zh {
			return "链 " + chain
		}
		return "chain " + chain
	case local != "":
		if zh {
			return "本 " + local
		}
		return "node " + local
	default:
		return ""
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
				return "sleep症状·缺唤醒边"
			}
			return "sleep symptom·no wake edge"
		}
		if strings.TrimSpace(node.DrilldownTarget) != "" {
			if zh {
				return "sleep症状→查上游"
			}
			return "sleep symptom→upstream"
		}
		if zh {
			return "sleep症状"
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

func runtimeTraceCausalProjectionBackgroundWhyCell(zh bool) string {
	if zh {
		return "背景压力/环境支撑,需结合 on-chain 证据解读"
	}
	return "background pressure/context; interpret with on-chain evidence"
}

func runtimeTraceCausalProjectionDepthCell(node types.TraceCausalProjectionNode, pathIndex map[string]int, zh bool) string {
	depth := node.ChainDepth
	if depth <= 0 && len(pathIndex) > 0 {
		depth = pathIndex[runtimeTraceCausalProjectionCanonicalNode(node.Subject)]
	}
	if depth <= 0 {
		return ""
	}
	if zh {
		return fmt.Sprintf("深度%d", depth)
	}
	return fmt.Sprintf("depth %d", depth)
}

func runtimeTraceCausalProjectionUpstreamCell(node types.TraceCausalProjectionNode) string {
	subject := strings.TrimSpace(node.Subject)
	if subject == "" {
		return runtimeTraceCausalProjectionNodeSubjectCell(node)
	}
	return runtimeTraceCausalProjectionCompactCellText(subject, 24)
}

func runtimeTraceCausalProjectionDownstreamCell(node types.TraceCausalProjectionNode, path []string) string {
	if next := runtimeTraceCausalProjectionNextPathNode(path, node.Subject); next != "" {
		return runtimeTraceCausalProjectionCompactCellText(next, 24)
	}
	object := strings.TrimSpace(node.Object)
	if object == "" && (node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span") {
		object = strings.TrimSpace(node.SpanName)
	}
	if object == "" {
		return ""
	}
	limit := 24
	if node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span" {
		limit = 34
	}
	return runtimeTraceCausalProjectionCompactCellText(object, limit)
}

func runtimeTraceCausalProjectionChainCell(node types.TraceCausalProjectionNode, path []string) string {
	upstream := runtimeTraceCausalProjectionUpstreamCell(node)
	downstream := runtimeTraceCausalProjectionDownstreamCell(node, path)
	switch {
	case upstream != "" && downstream != "":
		return upstream + " ▸ " + downstream
	case upstream != "":
		return upstream
	default:
		return downstream
	}
}

func runtimeTraceCausalProjectionNextPathNode(path []string, subject string) string {
	key := runtimeTraceCausalProjectionCanonicalNode(subject)
	for i, item := range path {
		if runtimeTraceCausalProjectionCanonicalNode(item) == key && i+1 < len(path) {
			return strings.TrimSpace(path[i+1])
		}
	}
	return ""
}

func runtimeTraceCausalProjectionMSCell(value float64) string {
	if value <= 0 {
		return ""
	}
	return fmt.Sprintf("%.3fms", value)
}

func runtimeTraceCausalProjectionNodeImpact(node types.TraceCausalProjectionNode) float64 {
	v := node.CumulativeImpactMS
	if node.EffectiveImpactMS > v {
		v = node.EffectiveImpactMS
	}
	if node.ImpactMS > v {
		v = node.ImpactMS
	}
	if node.ActualImpactMS > v {
		v = node.ActualImpactMS
	}
	return v
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
	if len(parts) == 0 {
		if zh {
			return "结构化 trace_query 观测"
		}
		return "structured trace_query observation"
	}
	return strings.Join(parts, " · ")
}

func runtimeTraceCausalProjectionLocalPathIndex(path []string) map[string]int {
	if len(path) == 0 {
		return nil
	}
	out := make(map[string]int, len(path))
	for i, item := range path {
		key := runtimeTraceCausalProjectionCanonicalNode(item)
		if key != "" {
			out[key] = i + 1
		}
	}
	return out
}

func runtimeTraceCausalProjectionLocalNodeOnChain(node types.TraceCausalProjectionNode) bool {
	switch strings.TrimSpace(node.ChainRelevance) {
	case "on_chain":
		return true
	}
	switch strings.TrimSpace(node.Causality) {
	case "on_wakeup_chain", "on_dependency_chain":
		return true
	}
	return false
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

func runtimeTraceCausalProjectionStateCell(node types.TraceCausalProjectionNode, zh bool) string {
	state := strings.TrimSpace(node.StateKind)
	if node.IsSleepState() {
		label := state
		if label == "" {
			label = "sleep"
		}
		out := "💤 " + label
		if zh {
			out += " · 非根因"
		} else {
			out += " · non-root"
		}
		if node.Undrillable() {
			out += " · ⛔"
		}
		return out
	}
	if node.Undrillable() {
		if zh {
			return "💤 sleep · ⛔ 无法下钻"
		}
		return "💤 sleep · ⛔ undrillable"
	}
	return state
}

func runtimeTraceCausalProjectionBar(value, max float64) string {
	if max <= 0 || value <= 0 {
		return ""
	}
	const width = 10
	filled := int(value/max*float64(width) + 0.5)
	if filled < 1 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func runtimeTraceCausalProjectionWindowCell(node types.TraceCausalProjectionNode, zh bool) string {
	return runtimeTraceCausalProjectionWindowDetail(node, zh)
}

func runtimeTraceCausalProjectionBucketMax(projection types.TraceCausalProjection) float64 {
	max := 0.0
	consider := func(nodes []types.TraceCausalProjectionNode) {
		for _, n := range nodes {
			v := n.CumulativeImpactMS
			if n.EffectiveImpactMS > v {
				v = n.EffectiveImpactMS
			}
			if n.ImpactMS > v {
				v = n.ImpactMS
			}
			if v > max {
				max = v
			}
		}
	}
	consider(runtimeTraceCausalProjectionPrimaryRoots(projection))
	consider(projection.OnChainCauses)
	consider(projection.AdjacentCauses)
	consider(projection.BackgroundCauses)
	consider(projection.SemanticSpans)
	consider(projection.SupportingHops)
	return max
}

func runtimeTraceCausalProjectionMermaidLabel(raw string) string {
	s := strings.TrimSpace(raw)
	replacer := strings.NewReplacer(
		"\"", "'", "\n", " ", "\r", " ",
		"[", "(", "]", ")", "{", "(", "}", ")", "|", "/", "`", "'",
	)
	s = replacer.Replace(s)
	s = strings.TrimSpace(s)
	if s == "" {
		return "node"
	}
	runes := []rune(s)
	if len(runes) > 60 {
		s = string(runes[:59]) + "…"
	}
	return s
}

func runtimeTraceCausalProjectionNodeShort(node types.TraceCausalProjectionNode) string {
	label := runtimeTraceCausalProjectionNodeSubject(node)
	if node.CumulativeImpactMS > 0 {
		label += fmt.Sprintf(" %.1fms", node.CumulativeImpactMS)
	} else if node.EffectiveImpactMS > 0 {
		label += fmt.Sprintf(" %.1fms", node.EffectiveImpactMS)
	}
	return label
}

func runtimeTraceCausalProjectionWakeupDiagram(projection types.TraceCausalProjection, zh bool) string {
	pathView := runtimeTraceCausalProjectionBoundedPathView(projection.WakeupPath, 10)
	if pathView.OriginalCount < 2 {
		return ""
	}
	path := runtimeTraceCausalProjectionDiagramPathNodes(pathView, zh)
	if len(path) < 2 {
		return ""
	}
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	ids := make([]string, len(path))
	for i, node := range path {
		id := fmt.Sprintf("w%d", i)
		ids[i] = id
		label := runtimeTraceCausalProjectionMermaidLabel(node)
		if i == len(path)-1 && pathView.SuffixPathCarriesTarget() {
			if zh {
				label += " (目标)"
			} else {
				label += " (target)"
			}
		}
		fmt.Fprintf(&b, "    %s[\"%s\"]\n", id, label)
	}
	edge := "唤醒/依赖"
	if !zh {
		edge = "wakes/depends"
	}
	for i := 0; i+1 < len(path); i++ {
		fmt.Fprintf(&b, "    %s -->|%s| %s\n", ids[i], edge, ids[i+1])
	}
	target := ids[len(ids)-1]
	bgEdge := "背景"
	if !zh {
		bgEdge = "background"
	}
	for i, bg := range projection.BackgroundCauses {
		if i >= 6 {
			break
		}
		id := fmt.Sprintf("bg%d", i)
		// off-chain background: rounded node + a plain dashed edge (no edge
		// label — the mermaid-subset renderer mishandles a "-.<label>.->" dashed
		// edge whose label is CJK; the rounded shape already reads as background).
		fmt.Fprintf(&b, "    %s(\"%s %s\")\n", id, runtimeTraceCausalProjectionMermaidLabel(runtimeTraceCausalProjectionNodeShort(bg)), bgEdge)
		fmt.Fprintf(&b, "    %s -.-> %s\n", id, target)
	}
	// No classDef/class/style directives: color is HTML-only decoration the
	// terminal mermaid-ascii renderer mis-parses into a spurious "class" node,
	// and the on-chain vs off-chain distinction is already carried losslessly by
	// the table's chain column plus the node shape (rectangle vs rounded).
	return b.String()
}

func runtimeTraceCausalProjectionDiagramPathNodes(view runtimeTraceCausalProjectionPathView, zh bool) []string {
	nodes := append([]string{}, view.Prefix...)
	if view.OmittedCount > 0 {
		if zh {
			nodes = append(nodes, fmt.Sprintf("省略%d节点", view.OmittedCount))
		} else {
			nodes = append(nodes, fmt.Sprintf("%d omitted", view.OmittedCount))
		}
	}
	nodes = append(nodes, view.Suffix...)
	return nodes
}

func (view runtimeTraceCausalProjectionPathView) SuffixPathCarriesTarget() bool {
	return view.OriginalCount > 0 && (view.OmittedCount == 0 || len(view.Suffix) > 0)
}

func runtimeTraceCausalProjectionSleepDiagram(projection types.TraceCausalProjection, zh bool) string {
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	seen := map[string]bool{}
	emitted := 0
	drillEdge := "下钻到"
	undrillEdge := "无法下钻"
	rootTag := "非sleep根因"
	if !zh {
		drillEdge, undrillEdge, rootTag = "drill down", "cannot drill", "non-sleep root"
	}
	add := func(node types.TraceCausalProjectionNode) {
		if !node.IsSleepState() && !node.Undrillable() {
			return
		}
		key := runtimeTraceCausalProjectionNodeKey(node)
		if seen[key] {
			return
		}
		seen[key] = true
		sid := fmt.Sprintf("s%d", emitted)
		fmt.Fprintf(&b, "    %s{{\"💤 %s\"}}\n", sid, runtimeTraceCausalProjectionMermaidLabel(runtimeTraceCausalProjectionNodeShort(node)))
		if node.Undrillable() {
			uid := fmt.Sprintf("u%d", emitted)
			ref := runtimeTraceCausalProjectionEvidenceRef(node)
			label := "⚠ " + node.UndrillableReason + " · " + undrillEdge
			if ref != "" {
				label += " " + ref
			}
			// terminal (no out-edge) node + plain dashed edge (no CJK edge label,
			// which the mermaid-subset renderer mishandles). The [[...]] shape and
			// the ⚠ label carry "chain breaks here / cannot drill further".
			fmt.Fprintf(&b, "    %s[[\"%s\"]]\n", uid, runtimeTraceCausalProjectionMermaidLabel(label))
			fmt.Fprintf(&b, "    %s -.-> %s\n", sid, uid)
		} else if target := strings.TrimSpace(node.DrilldownTarget); target != "" {
			rid := fmt.Sprintf("r%d", emitted)
			fmt.Fprintf(&b, "    %s[\"%s · %s\"]\n", rid, runtimeTraceCausalProjectionMermaidLabel(target), rootTag)
			fmt.Fprintf(&b, "    %s -->|%s| %s\n", sid, drillEdge, rid)
		}
		emitted++
	}
	for _, n := range runtimeTraceCausalProjectionPrimaryRoots(projection) {
		add(n)
	}
	for _, n := range projection.OnChainCauses {
		add(n)
	}
	for _, n := range projection.SupportingHops {
		add(n)
	}
	if emitted == 0 {
		return ""
	}
	return b.String()
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

func runtimeTraceCausalProjectionNodeSubject(node types.TraceCausalProjectionNode) string {
	subject := strings.TrimSpace(node.Subject)
	object := strings.TrimSpace(node.Object)
	switch {
	case subject != "" && object != "":
		return subject + " -> " + object
	case subject != "":
		return subject
	case object != "":
		return object
	default:
		return "trace causal node"
	}
}

func runtimeTraceCausalProjectionNodeSubjectCell(node types.TraceCausalProjectionNode) string {
	subject := strings.TrimSpace(node.Subject)
	object := strings.TrimSpace(node.Object)
	if (node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span") && strings.TrimSpace(node.SpanName) != "" {
		object = strings.TrimSpace(node.SpanName)
	}
	objectLimit := 22
	if node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span" {
		objectLimit = 36
	}
	switch {
	case subject != "" && object != "":
		return runtimeTraceCausalProjectionCompactCellText(subject, 28) + " → " + runtimeTraceCausalProjectionCompactCellText(object, objectLimit)
	case subject != "":
		return runtimeTraceCausalProjectionCompactCellText(subject, 44)
	case object != "":
		return runtimeTraceCausalProjectionCompactCellText(object, 44)
	default:
		return "trace causal node"
	}
}

func runtimeTraceCausalProjectionCompactCellText(raw string, maxRunes int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || maxRunes <= 0 {
		return raw
	}
	runes := []rune(raw)
	if len(runes) <= maxRunes {
		return raw
	}
	if maxRunes <= 1 {
		return "…"
	}
	return string(runes[:maxRunes-1]) + "…"
}

// runtimeTraceCausalProjectionWindowDetail surfaces the O4 within/outside
// requested-window tag. It returns "" when WithinRequestedWindow is nil (no
// precise anchor available), so nodes without a resolved user window render
// byte-identically to before.
func runtimeTraceCausalProjectionWindowDetail(node types.TraceCausalProjectionNode, zh bool) string {
	if node.WithinRequestedWindow == nil {
		return ""
	}
	if *node.WithinRequestedWindow {
		if zh {
			return "落在用户请求窗口内"
		}
		return "within requested window"
	}
	if zh {
		return "下钻到请求窗口外的上游依赖"
	}
	return "upstream dependency drilled outside the requested window"
}

func runtimeTraceCausalProjectionSemanticDetail(node types.TraceCausalProjectionNode, zh bool) string {
	semanticClass := strings.TrimSpace(node.SemanticClass)
	spanName := strings.TrimSpace(node.SpanName)
	if semanticClass == "" && spanName == "" {
		return ""
	}
	var parts []string
	if semanticClass != "" {
		if zh {
			parts = append(parts, "语义类 "+semanticClass)
		} else {
			parts = append(parts, "semantic class "+semanticClass)
		}
	}
	if spanName != "" {
		parts = append(parts, "span "+spanName)
	}
	return strings.Join(parts, "，")
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
		return ref
	}
	pathPart, suffix := runtimeTraceCausalProjectionSplitLineSuffix(ref)
	if strings.TrimSpace(pathPart) == "" {
		return runtimeTraceCausalProjectionCompactCellText(ref, 56)
	}
	tail := runtimeTraceCausalProjectionPathTail(pathPart, 1)
	if tail == "" {
		return runtimeTraceCausalProjectionCompactCellText(ref, 56)
	}
	return runtimeTraceCausalProjectionCompactCellText(tail+suffix, 56)
}

func runtimeTraceCausalProjectionEvidenceRefShortened(raw, display string) bool {
	raw = strings.TrimSpace(raw)
	display = strings.TrimSpace(display)
	return raw != "" && display != "" && raw != display
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
	block := types.AnswerBlock{
		ID:    "runtime_trace_metric_snapshot",
		Kind:  types.BlockBulletList,
		Title: "Trace 指标快照",
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
	seen := make(map[string]bool)
	var out []types.AnswerBlockItem
	for _, record := range ledger.Records {
		text := runtimeTraceMetricSnapshotFromObservationRecord(record)
		if text == "" {
			continue
		}
		if runtimeTraceMetricSnapshotCoveredByAnswer(visible, record, text) {
			continue
		}
		if seen[text] {
			continue
		}
		seen[text] = true
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

func runtimeTraceMetricSnapshotFromObservationRecord(record types.ObservationRecord) string {
	if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact {
		return ""
	}
	if !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
		return ""
	}
	if !runtimeTraceStateChurnHasPositiveImpact(record) {
		return ""
	}
	required := []string{
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
	values := make(map[string]string, len(required)+1)
	keys := append([]string{"dominant_state"}, required...)
	for _, key := range keys {
		if value := runtimeTraceObservationRichNoteValue(record.RichNotes, key); value != "" {
			values[key] = value
		}
	}
	runtimeTraceMergeSummaryMetricTokens(values, record.Summary, keys)
	for _, key := range required {
		if values[key] == "" {
			return ""
		}
	}
	parts := make([]string, 0, len(required)+1)
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
	seen := make(map[string]bool)
	var out []types.AnswerBlockItem
	for _, record := range ledger.Records {
		step := runtimeTraceNextStepFromObservationRecord(record)
		if step == "" {
			continue
		}
		if seen[step] {
			continue
		}
		seen[step] = true
		out = append(out, types.AnswerBlockItem{
			ID:          fmt.Sprintf("runtime_trace_next_step_%d", len(out)+1),
			Label:       "下一步",
			Text:        step,
			CitationRef: -1,
		})
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func runtimeTraceNextStepFromObservationRecord(record types.ObservationRecord) string {
	if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact {
		return ""
	}
	if !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
		return ""
	}
	return trimRuntimeTraceNextStepText(runtimeTraceObservationRichNoteValue(record.RichNotes, "next_step"))
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
