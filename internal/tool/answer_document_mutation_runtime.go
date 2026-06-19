package tool

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
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
	if materializeRuntimeTraceMetricSnapshotBlock(merged, ctx) {
		logging.Info("[%s] materialized runtime trace metric snapshot from structured observation notes", toolName)
	}
	if materializeRuntimeTraceNextStepsBlock(merged, ctx) {
		logging.Info("[%s] materialized runtime trace next-step block from structured observation notes", toolName)
	}
	if materializeRuntimeTracePerfQualityBlock(merged, ctx) {
		logging.Info("[%s] materialized runtime trace perf quality block from structured observation notes", toolName)
	}
	if materializeRuntimeTraceObservationBlock(merged, ctx) {
		logging.Info("[%s] materialized runtime trace observation block from structured perf facts", toolName)
	}
	if stamped := stampReadOwnerAnchorsFromTurnA(ctx, merged); stamped > 0 {
		logging.Info("[%s] stamped %d read owner anchor(s) from typed source localization", toolName, stamped)
	}
	if stampReadNavigationCoverageFromTurnA(ctx, merged) {
		logging.Info("[%s] stamped read repo_map navigation coverage from typed TurnA observations", toolName)
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

func stampReadOwnerAnchorsFromTurnA(ctx *types.BusContext, doc *types.AnswerDocumentV2) int {
	if ctx == nil || ctx.Mutable == nil || doc == nil {
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
	items := runtimeTraceMetricSnapshotItems(ctx)
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

func runtimeTraceMetricSnapshotItems(ctx *types.BusContext) []types.AnswerBlockItem {
	if ctx == nil {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, 64))
	if len(ledger.Records) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var out []types.AnswerBlockItem
	for _, record := range ledger.Records {
		text := runtimeTraceMetricSnapshotFromObservationRecord(record)
		if text == "" {
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

func runtimeTraceMetricSnapshotFromObservationRecord(record types.ObservationRecord) string {
	if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact {
		return ""
	}
	if strings.TrimSpace(record.Producer) != "trace_query" {
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
	if strings.TrimSpace(record.Producer) != "trace_query" {
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
		strings.TrimSpace(record.Producer) != "trace_query" ||
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
