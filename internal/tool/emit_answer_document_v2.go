package tool

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Block-only carrier dispatch for emit_answer_document.
//
// Schema validation discipline (R2 / R12 from
// docs/migration/block_only_carrier.md §3):
//   - blocks[] MUST be present and non-empty
//   - every block.kind MUST be in AllAnswerBlockKinds()
//   - every block.id MUST be non-empty
//   - V1 fields (shape / steps / symbols / value / boolean /
//     symbols_completeness) MUST NOT appear at the top level —
//     V1-or-V2 hybrid emits are rejected to keep the carriers
//     mutually exclusive
//   - summary at top level (V1 field) MUST NOT appear

// emitAnswerDocumentV2Params mirrors AnswerDocumentV2 one-to-one for
// JSON unmarshalling. We DO NOT reuse types.AnswerDocumentV2
// directly so the tool layer can run schema-level checks (forbid V1
// fields) before constructing the typed value. CitationRef is
// transported as FlexInt to accept either int or string forms.
type emitAnswerDocumentV2Params struct {
	DocumentModel         string                             `json:"document_model"`
	Blocks                []emitAnswerBlockV2                `json:"blocks"`
	Citations             []emitAnswerCitationV2             `json:"citations,omitempty"`
	ExactResolution       *types.AnswerExactResolution       `json:"exact_resolution,omitempty"`
	MissingRequestedRoles []types.AnswerMissingRequestedRole `json:"missing_requested_roles,omitempty"`
	Caveats               []string                           `json:"caveats,omitempty"`
	Snippets              []emitCodeSnippetV2                `json:"snippets,omitempty"`
}

type emitAnswerCitationV2 struct {
	File             string              `json:"file"`
	Line             FlexInt             `json:"line"`
	Quote            string              `json:"quote,omitempty"`
	Scope            types.EvidenceScope `json:"scope,omitempty"`
	LineEnd          FlexInt             `json:"line_end,omitempty"`
	SectionPath      string              `json:"section_path,omitempty"`
	FileRoleLabel    types.FileRoleLabel `json:"file_role_label,omitempty"`
	CrossfileSummary string              `json:"crossfile_summary,omitempty"`
	NegativePattern  string              `json:"negative_pattern,omitempty"`
}

type emitCodeSnippetV2 struct {
	File      string  `json:"file"`
	StartLine FlexInt `json:"start_line"`
	EndLine   FlexInt `json:"end_line"`
	Language  string  `json:"language,omitempty"`
	Code      string  `json:"code"`
}

type emitAnswerBlockV2 struct {
	ID                      string                    `json:"id"`
	Kind                    string                    `json:"kind"`
	Title                   string                    `json:"title,omitempty"`
	Text                    string                    `json:"text,omitempty"`
	Caveat                  string                    `json:"caveat,omitempty"`
	ErrorGranularityVerdict string                    `json:"error_granularity_verdict,omitempty"`
	CurrentStatusVerdict    string                    `json:"current_status_verdict,omitempty"`
	ScopeDisclosure         string                    `json:"scope_disclosure,omitempty"`
	Columns                 []string                  `json:"columns,omitempty"`
	Items                   []emitAnswerBlockItemV2   `json:"items,omitempty"`
	Diagram                 *emitAnswerDiagramV2      `json:"diagram,omitempty"`
	ClaimUses               []types.RenderedClaimUse  `json:"claim_uses,omitempty"`
	EdgeAnchors             []types.DiagramEdgeAnchor `json:"edge_anchors,omitempty"`
	FacetIDs                []string                  `json:"facet_ids,omitempty"`
	SurfaceRole             string                    `json:"surface_role,omitempty"`
}

type emitAnswerBlockItemV2 struct {
	ID            string   `json:"id,omitempty"`
	Label         string   `json:"label,omitempty"`
	Text          string   `json:"text,omitempty"`
	Cells         []string `json:"cells,omitempty"`
	CandidateRole string   `json:"candidate_role,omitempty"`
	// CitationRef is a pointer so an ABSENT/null citation_ref (the item
	// is uncited → types.CitationRefUnset) is distinguishable from an
	// explicit citation_ref:0 (a valid zero-based index into the first
	// pooled citation). A value FlexInt here collapsed both to 0, which
	// glued citations[0] onto items the LLM left intentionally uncited.
	CitationRef *FlexInt `json:"citation_ref,omitempty"`
}

type emitAnswerDiagramV2 struct {
	Kind     string `json:"kind"`
	Language string `json:"language,omitempty"`
	Body     string `json:"body"`
}

// executeAnswerDocumentV2 handles block-only emits. It validates the
// schema, rejects mixed V1+V2 fields, and writes
// the typed AnswerDocumentV2 to MutableState via
// SetAnswerDocumentV2. The result Summary string mirrors V1's tone
// so finalize_preview / orchestrator hooks render consistent
// per-call text.
func executeAnswerDocumentV2(toolName string, ctx *types.BusContext, raw json.RawMessage, now time.Time) (types.ToolResult, error) {
	var recovery answerDocumentRecoveryReport
	// First pass: detect retired top-level fields (shape / steps /
	// symbols / value / boolean / summary / symbols_completeness).
	// The answer payload lives entirely inside blocks[]; these
	// top-level fields would silently shadow the block contract and
	// must be moved into the appropriate block kind.
	if violation := detectV1FieldsInV2Emit(raw); violation != "" {
		persistRecoveredAnswerDraft(ctx, raw, recovery, nil)
		return failEmit(toolName, now,
			"top-level field %q is not accepted; the answer is expressed through blocks[] only — move any answer payload into the appropriate block kind",
			violation)
	}

	// Flat-mode tolerance. Some LLMs emit nested arrays as JSON
	// strings ("[{...}]") instead of real arrays — typically a
	// streaming artefact where the model ran the array through
	// JSON.stringify before placing it in the tool-call. We
	// repair top-level blocks[] AND every nested array on each
	// block (items / claim_uses) so the
	// LLM gets accepted instead of forced to retry, while a WARN
	// log surfaces the recovery for operator visibility.
	if repaired, report, ok := repairBlocksAsStringDetailed(raw); ok {
		logging.Warning("[emit_answer_document] blocks[] arrived as JSON-encoded string; re-parsed via flat-mode tolerance path")
		raw = repaired
		recovery = report
	}
	raw = applyStructuredPayloadCompatWithLegacyStringFieldRepair(toolName, raw, (&EmitAnswerDocument{}).Parameters(), "exact_resolution")
	if repaired, paths, ok := repairNestedAnswerBlockFields(raw); ok {
		logging.Warning("[emit_answer_document] nested block fields normalized via local-model JSON tolerance (paths: %s)",
			strings.Join(paths, ", "))
		raw = repaired
	}
	if repaired, paths, ok := repairOrphanAnswerBlockAnnotations(raw); ok {
		logging.Warning("[emit_answer_document] orphan block annotations merged via local-model JSON tolerance (paths: %s)",
			strings.Join(paths, ", "))
		raw = repaired
	}
	if repaired, paths, ok := quarantineUnknownAnswerDocumentFields(raw, answerDocumentFullEmitQuarantineProfile); ok {
		logging.Warning("[emit_answer_document] quarantined schema-unknown answer-document field(s) without retry: %s",
			strings.Join(paths, ", "))
		raw = repaired
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var p emitAnswerDocumentV2Params
	if err := dec.Decode(&p); err != nil {
		persistRecoveredAnswerDraft(ctx, raw, recovery, nil)
		return failStrictDecode(toolName, now, err, answerDocumentV2MisplacedHints, raw)
	}

	// document_model is no longer surfaced to the LLM. The system
	// runs only one carrier so any value (or absence) the model
	// happens to emit is silently accepted — the dispatcher always
	// routes to this executor and the typed model is hardcoded
	// downstream. Future migration to a second carrier would
	// re-introduce validation here.
	if len(p.Blocks) == 0 {
		persistRecoveredAnswerDraft(ctx, raw, recovery, nil)
		return failEmitWithRepair(toolName, now, &types.ToolRepair{
			Code:   "answer_doc_blocks_required",
			Fields: []string{"blocks"},
			Hint:   "Re-emit a complete `emit_answer_document` payload with a non-empty `blocks[]` array. Preserve the same answer facts and citations; do not send only `citations[]`, metadata, or free-form prose outside the tool call.",
		},
			"blocks[] is required and must be non-empty")
	}

	// Build typed AnswerDocumentV2 — per-block validation lives in
	// NormalizeEmitAnswerBlock (shared with the patch path so a typed
	// annotation field added to AnswerBlock surfaces in BOTH paths).
	// Merged-doc invariants (block-id uniqueness, diagram payload,
	// max blocks) are enforced inside ApplyAndPersistMutation so
	// both paths share them.
	doc := &types.AnswerDocumentV2{
		DocumentModel:         "v2",
		Citations:             enrichCitationsWithEnclosingFunction(convertEmitCitationsToTyped(p.Citations), ctx),
		ExactResolution:       p.ExactResolution,
		MissingRequestedRoles: p.MissingRequestedRoles,
		Caveats:               p.Caveats,
		Snippets:              convertEmitCodeSnippetsToTyped(p.Snippets),
	}
	// entry.modelIndex, never the loop position: validation-error
	// fieldPaths must name the block's index in the MODEL's own
	// blocks[] array, not its system-shifted post-split position.
	for _, entry := range splitFusedDiagramBlocks(toolName, p.Blocks) {
		blk, err := NormalizeEmitAnswerBlock(entry.raw, fmt.Sprintf("blocks[%d]", entry.modelIndex))
		if err != nil {
			persistRecoveredAnswerDraft(ctx, raw, mergeAnswerDocumentRecoveryAttachments(recovery, doc), doc)
			return failEmit(toolName, now, "%s", err.Error())
		}
		doc.Blocks = append(doc.Blocks, blk)
	}
	if changed, fields := normalizeAnswerDocumentBlockIDSurface(doc); changed {
		logging.Warning("[emit_answer_document] id duplicate(s) normalized via transactional tolerance: %s",
			strings.Join(fields, ", "))
	}
	canonicalizeSummaryLeadBlock(doc)
	if answerDocumentRecoveryLostUnattachedBlocks(recovery) {
		persistRecoveredAnswerDraft(ctx, raw, mergeAnswerDocumentRecoveryAttachments(recovery, doc), doc)
		return failEmitWithRepair(toolName, now, answerDocumentLossyBlocksRecoveryRepair(recovery),
			"structured recovery could not preserve every visible blocks[] item (%d candidate block(s), %d structured block(s), %d recovered attachment(s)); re-emit a complete native JSON blocks[] array, not a JSON-encoded string",
			recovery.CandidateBlocks, recovery.RecoveredBlocks, len(recovery.Attachments))
	}
	visibleRecovery := mergeAnswerDocumentRecoveryAttachments(recovery, doc)
	normalizeTypedExcludedAnswerSurface(doc, ctx)

	// P1 (2026-05-10) — emit-time pre-validation chokepoint.
	// Before we persist the doc, run the structural checks and split
	// their typed hints through the central ViolationKind registry.
	// SoftByDefault hints are logged as advisories and allowed to
	// reach the post-emit caveat path; only genuinely hard hints
	// reject the tool call.
	//
	// View-aware: when no view is available (single-shot tests,
	// pre-AnalysisIR paths) the pre-check returns nil and the
	// post-emit chain in internal/orchestrator runs unchanged.
	if view := types.BuildAnswerSemanticViewForBusContext(ctx); view != nil {
		preEmitCtx := newPreEmitCheckContext(ctx)
		if fixed := carryForwardCitationsFromRejectedDraft(doc, ctx); fixed > 0 {
			logging.Warning("[emit_answer_document] restored %d citation(s) from previous answer draft", fixed)
		}
		if fixed := promoteRecoveredDiagramBlocks(doc, view, visibleRecovery); fixed > 0 {
			logging.Warning("[emit_answer_document] promoted %d recovered diagram attachment(s) into diagram block(s)", fixed)
			recovery.Attachments = filterAttachmentsAlreadyRepresentedInDocument(recovery.Attachments, doc)
			visibleRecovery.Attachments = filterAttachmentsAlreadyRepresentedInDocument(visibleRecovery.Attachments, doc)
		}
		normalizeAnswerDocumentForPreEmit(toolName, doc, view, ctx, preEmitCtx)
		if hints := runPreEmitChecksWithContext(doc, view, preEmitOracleFromCtx(ctx), preEmitCtx); len(hints) > 0 {
			if fixed := materializeRequiredCaveatWhenOnlyMissing(doc, view, hints, ctx); fixed > 0 {
				logging.Warning("[emit_answer_document] materialized %d required caveat block(s) from uncertainty contract", fixed)
				hints = runPreEmitChecksWithContext(doc, view, preEmitOracleFromCtx(ctx), preEmitCtx)
			}
			hardHints, advisoryHints := splitPreEmitHintsByGate(hints)
			if len(advisoryHints) > 0 {
				logSoftPreEmitAdvisory(toolName, "pre-emit structural", advisoryHints)
			}
			if len(hardHints) > 0 {
				rememberRejectedAnswerDocumentDraft(ctx, doc)
				persistRecoveredAnswerDraft(ctx, raw, visibleRecovery, doc)
				return failEmitWithRepair(toolName, now, emitFixHintsRepair(hardHints),
					"%s", formatEmitFixHints(hardHints))
			}
		}
	}
	if hints := preCheckModelSurfaceTerms(doc, ctx); len(hints) > 0 {
		if fixed := materializeRequiredModelSurfaceTerms(doc, ctx); fixed > 0 {
			logging.Warning("[emit_answer_document] materialized required model surface_terms into %d principal item(s)", fixed)
			hints = preCheckModelSurfaceTerms(doc, ctx)
		}
		if len(hints) > 0 {
			logSoftPreEmitAdvisory(toolName, "model-emitted surface_terms", hints)
		}
	}

	// v3 B4 (2026-05-04): route the full-emit write through the
	// unified mutation runtime — same chokepoint as the patch path,
	// merged-doc validation + persist + telemetry shared.
	mutation := types.NewReplaceAllMutation(doc)
	res, err := ApplyAndPersistMutation(ctx, toolName, mutation, nil, now)
	if err == nil && res.Success && ctx != nil && ctx.Mutable != nil {
		attachments := filterAcceptedAnswerDisplayAttachments(doc, recovery.Attachments)
		ctx.Mutable.SetAnswerDisplayAttachments(attachments)
		if len(attachments) > 0 {
			logging.Warning("[emit_answer_document] preserved %d recovered display attachment(s) after %s recovery (candidate_blocks=%d recovered_blocks=%d)",
				len(attachments), recovery.Mode, recovery.CandidateBlocks, recovery.RecoveredBlocks)
		}
	}
	return res, err
}

// detectV1FieldsInV2Emit scans the raw JSON for any top-level field
// that belongs to V1's shape-specific schema. Returns the first
// offending field name, or "" when none are present. The check is
// intentionally a flat regex-free string match because the field
// list is short and explicit.
func detectV1FieldsInV2Emit(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ""
	}
	v1Fields := []string{
		"shape",
		"steps",
		"symbols",
		"value",
		"boolean",
		"symbols_completeness",
		"summary",
	}
	for _, f := range v1Fields {
		if _, present := probe[f]; present {
			return f
		}
	}
	return ""
}

type answerDocumentRecoveryReport struct {
	Mode                  string
	Lossless              bool
	CandidateBlocks       int
	RecoveredBlocks       int
	CandidateKinds        []types.AnswerBlockKind
	RecoveredKinds        []types.AnswerBlockKind
	DroppedVisiblePayload bool
	Attachments           []types.AnswerDisplayAttachment
	Diagnostics           []string
}

func persistRecoveredAnswerDraft(ctx *types.BusContext, raw json.RawMessage, report answerDocumentRecoveryReport, doc *types.AnswerDocumentV2) {
	if ctx == nil || ctx.Mutable == nil {
		return
	}
	attachments := ctx.Mutable.AnswerDisplayAttachments()
	initialLen := len(attachments)
	appendReport := func(r answerDocumentRecoveryReport) {
		for _, att := range r.Attachments {
			attachments = appendRecoveredAttachment(attachments, att)
		}
	}
	appendDoc := func(candidate *types.AnswerDocumentV2, source string) bool {
		if candidate == nil || len(candidate.Blocks) == 0 {
			return false
		}
		appendReport(mergeAnswerDocumentRecoveryAttachments(answerDocumentRecoveryReport{
			Mode: source,
		}, candidate))
		if att, ok := answerDocumentDraftTextAttachment(candidate, source); ok {
			attachments = appendRecoveredAttachment(attachments, att)
			return true
		}
		return false
	}
	appendReport(report)
	addedDoc := false
	if len(bytes.TrimSpace(raw)) > 0 {
		if rec, ok := recoverAnswerDocumentV2FromRawCandidate(raw); ok {
			appendReport(answerDocumentRecoveryReport{Mode: rec.Mode, Attachments: rec.Attachments})
			addedDoc = appendDoc(rec.Document, rec.Mode)
		}
	}
	if !addedDoc {
		source := strings.TrimSpace(report.Mode)
		if source == "" {
			source = "rejected_answer_document"
		}
		addedDoc = appendDoc(doc, source)
	}
	if len(attachments) == initialLen {
		return
	}
	ctx.Mutable.SetAnswerDisplayAttachments(attachments)
	logging.Warning("[emit_answer_document] preserved %d recovered display attachment(s) from rejected answer draft",
		len(attachments)-initialLen)
}

func mergeAnswerDocumentRecoveryAttachments(report answerDocumentRecoveryReport, doc *types.AnswerDocumentV2) answerDocumentRecoveryReport {
	if doc == nil || len(doc.Blocks) == 0 {
		return report
	}
	for _, blk := range doc.Blocks {
		att, ok := displayAttachmentFromAnswerDiagramBlock(blk)
		if !ok {
			continue
		}
		report.Attachments = appendRecoveredAttachment(report.Attachments, att)
	}
	if len(report.Attachments) > 0 && strings.TrimSpace(report.Mode) == "" {
		report.Mode = "rejected_answer_document"
	}
	return report
}

func answerDocumentDraftTextAttachment(doc *types.AnswerDocumentV2, source string) (types.AnswerDisplayAttachment, bool) {
	body := renderRecoveredAnswerDocumentDraft(doc)
	if body == "" {
		return types.AnswerDisplayAttachment{}, false
	}
	att := types.AnswerDisplayAttachment{
		Kind:   types.AnswerDisplayAttachmentMarkdown,
		Body:   body,
		Source: "emit_answer_document.rejected_payload",
		Reason: "rejected structured answer draft contained user-visible text",
	}
	if strings.TrimSpace(source) != "" {
		att.Source = "emit_answer_document." + strings.TrimSpace(source)
	}
	att.Hash = answerDisplayAttachmentHash(att.Kind, att.Language, att.Body)
	return att, true
}

func answerDocumentRecoveryLostUnattachedBlocks(report answerDocumentRecoveryReport) bool {
	if strings.TrimSpace(report.Mode) == "" || report.Lossless {
		return false
	}
	recoveredVisible := report.RecoveredBlocks + len(report.Attachments)
	return report.CandidateBlocks > recoveredVisible
}

func answerDocumentLossyBlocksRecoveryRepair(report answerDocumentRecoveryReport) *types.ToolRepair {
	return &types.ToolRepair{
		Code:   "answer_doc_lossy_blocks_string_recovery",
		Fields: []string{"blocks"},
		Hint:   "Re-emit `emit_answer_document` with `blocks` as a complete native JSON array of block objects. Do not JSON-encode the array as a string, and preserve every model-authored visible block from the previous draft.",
		Metadata: map[string]string{
			"candidate_blocks":      strconv.Itoa(report.CandidateBlocks),
			"recovered_blocks":      strconv.Itoa(report.RecoveredBlocks),
			"recovered_attachments": strconv.Itoa(len(report.Attachments)),
			"recovery_mode":         strings.TrimSpace(report.Mode),
		},
	}
}

func renderRecoveredAnswerDocumentDraft(doc *types.AnswerDocumentV2) string {
	if doc == nil {
		return ""
	}
	var b strings.Builder
	for _, blk := range doc.Blocks {
		if blk.Kind == types.BlockDiagram {
			continue
		}
		if title := strings.TrimSpace(blk.Title); title != "" {
			if blk.Kind == types.BlockSummary {
				fmt.Fprintf(&b, "## %s\n\n", title)
			} else {
				fmt.Fprintf(&b, "### %s\n\n", title)
			}
		}
		if text := strings.TrimSpace(blk.Text); text != "" {
			b.WriteString(text)
			b.WriteString("\n\n")
		}
		for i, item := range blk.Items {
			line := recoveredAnswerItemLine(doc, blk.Kind, item, i)
			if line == "" {
				continue
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
		if len(blk.Items) > 0 {
			b.WriteByte('\n')
		}
	}
	if len(doc.Caveats) > 0 {
		b.WriteString("Caveats:\n")
		for _, caveat := range doc.Caveats {
			if caveat = strings.TrimSpace(caveat); caveat != "" {
				fmt.Fprintf(&b, "- %s\n", caveat)
			}
		}
		b.WriteByte('\n')
	}
	if len(doc.Citations) > 0 {
		b.WriteString("References:\n")
		for _, cite := range doc.Citations {
			if ref := recoveredCitationRef(cite); ref != "" {
				fmt.Fprintf(&b, "- %s\n", ref)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func recoveredAnswerItemLine(doc *types.AnswerDocumentV2, kind types.AnswerBlockKind, item types.AnswerBlockItem, idx int) string {
	label := strings.TrimSpace(item.Label)
	text := strings.TrimSpace(item.Text)
	if label == "" && text == "" {
		return ""
	}
	body := text
	if label != "" && text != "" {
		body = label + ": " + text
	} else if label != "" {
		body = label
	}
	if item.CitationRef >= 0 && item.CitationRef < len(doc.Citations) {
		if ref := recoveredCitationRef(doc.Citations[item.CitationRef]); ref != "" {
			body += " (" + ref + ")"
		}
	}
	if kind == types.BlockOrderedList {
		return fmt.Sprintf("%d. %s", idx+1, body)
	}
	return "- " + body
}

func recoveredCitationRef(c types.Citation) string {
	file := strings.TrimSpace(c.File)
	if file == "" {
		return ""
	}
	if c.Line <= 0 {
		return file
	}
	if c.LineEnd > c.Line {
		return fmt.Sprintf("%s:%d-%d", file, c.Line, c.LineEnd)
	}
	return fmt.Sprintf("%s:%d", file, c.Line)
}

func displayAttachmentFromAnswerDiagramBlock(blk types.AnswerBlock) (types.AnswerDisplayAttachment, bool) {
	if blk.Kind != types.BlockDiagram || blk.Diagram == nil {
		return types.AnswerDisplayAttachment{}, false
	}
	body := strings.TrimSpace(blk.Diagram.Body)
	if body == "" {
		return types.AnswerDisplayAttachment{}, false
	}
	lang := strings.TrimSpace(blk.Diagram.Language)
	if lang == "" {
		lang = "mermaid"
	}
	att := types.AnswerDisplayAttachment{
		Kind:     types.AnswerDisplayAttachmentDiagram,
		Title:    blk.Title,
		Language: lang,
		Body:     body,
		Source:   "emit_answer_document.rejected_payload",
		Reason:   "rejected structured answer draft contained a visible diagram",
	}
	att.Hash = answerDisplayAttachmentHash(att.Kind, att.Language, att.Body)
	return att, true
}

func rememberRejectedAnswerDocumentDraft(ctx *types.BusContext, doc *types.AnswerDocumentV2) {
	if ctx == nil || ctx.Mutable == nil || doc == nil || len(doc.Blocks) == 0 {
		return
	}
	if deduped := dedupeVisibleAnswerBlocks(doc); deduped > 0 {
		logging.Warning("[emit_answer_document] dropped %d duplicate visible block(s) before storing rejected draft", deduped)
	}
	ctx.Mutable.SetLastRejectedAnswerDocumentV2(doc)
}

func normalizeAnswerDocumentForPreEmit(toolName string, doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctx *types.BusContext, pctx *preEmitCheckContext) {
	if doc == nil || view == nil {
		return
	}
	if pctx == nil {
		pctx = newPreEmitCheckContext(ctx)
	}
	if fixed := normalizeDiagramDefinitionLabelsByEvidence(doc, pctx); fixed > 0 {
		logging.Warning("[%s] repaired %d diagram definition label(s) by evidence-defined source", toolName, fixed)
	}
	if fixed := normalizeDiagramEdgeAnchorMetadata(doc); fixed > 0 {
		logging.Warning("[%s] normalized %d diagram edge anchor metadata value(s)", toolName, fixed)
	}
	if fixed := normalizeRequiredMechanismAnchorCarriersWithContext(doc, view, ctx, pctx); fixed > 0 {
		logging.Warning("[%s] repaired %d required mechanism anchor carrier(s)", toolName, fixed)
	}
	if fixed := normalizeQualifiedItemLabelsByUniqueEnclosingFunction(doc, view); fixed > 0 {
		logging.Warning("[%s] repaired %d qualified item label(s) by graph-derived enclosing function", toolName, fixed)
	}
	if fixed := normalizeItemCitationRefsByTypedCandidateRoleWithContext(doc, view, ctx, pctx); fixed > 0 {
		logging.Warning("[%s] repaired %d item citation_ref value(s) by typed candidate-role source rows", toolName, fixed)
	}
	if fixed := normalizeItemCitationRefsByUniqueLabelCitationWithContext(doc, view, ctx, pctx); fixed > 0 {
		logging.Warning("[%s] repaired %d item citation_ref value(s) by typed label/citation corroboration", toolName, fixed)
	}
	if fixed := detachInvalidItemCitationRefsWithoutSafeCandidateWithContext(doc, view, ctx, pctx); fixed > 0 {
		logging.Warning("[%s] detached %d invalid item citation_ref value(s) with no safe replacement candidate", toolName, fixed)
		// G6 (2026-06-12 sweep): a detached reference is a visible
		// degradation, not a silent repair — the item keeps its text
		// but loses its source anchor. Disclose it as a caveat so the
		// reader knows which strength of grounding they are reading.
		if principalEnumerationPrefersZH(ctx) {
			doc.Caveats = append(doc.Caveats, fmt.Sprintf("%d 处条目的来源引用无法对应到任何已验证来源，已移除该引用（条目内容保留，但不再带源码锚点）。", fixed))
		} else {
			doc.Caveats = append(doc.Caveats, fmt.Sprintf("%d item source reference(s) could not be matched to a verified source and were removed (item text kept, but without a source anchor).", fixed))
		}
	}
	if fixed := normalizeOutOfRangeItemCitationRefsByEvidenceSurfaceWithContext(doc, view, ctx, pctx); fixed > 0 {
		logging.Warning("[%s] repaired %d out-of-range item citation_ref value(s) by evidence-surface corroboration", toolName, fixed)
	}
	if fixed := compileEnumerationDisplayTableRows(doc, ctx); fixed > 0 {
		logging.Warning("[%s] compiled %d deterministic enumeration table row(s) from accepted principal evidence handoff", toolName, fixed)
	}
	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed > 0 {
		logging.Warning("[%s] normalized %d principal enumeration block(s) from accepted evidence-rich row contract", toolName, fixed)
	}
	if fixed := normalizeCurrentSourceCitationSupplement(doc, ctx, pctx); fixed > 0 {
		logging.Warning("[%s] materialized %d current-source citation support row(s) from accepted evidence", toolName, fixed)
	}
	if fixed := normalizeAggregateNegativeProofSupplement(doc, ctx); fixed > 0 {
		logging.Warning("[%s] materialized %d typed negative-proof supplement row(s) from accepted aggregate evidence", toolName, fixed)
	}
	if fixed := compileCitationBackedTableRows(doc); fixed > 0 {
		logging.Warning("[%s] compiled %d citation-backed table row(s) from incomplete structured table carriers", toolName, fixed)
	}
	if ctx != nil {
		if fixed := normalizeAggregateMemberSetCarriers(doc, ctx); fixed > 0 {
			logging.Warning("[%s] materialized %d principal aggregate member row(s) from accepted exhaustive enumeration handoff", toolName, fixed)
		}
		if fixed := normalizePrincipalSupportMemberCarriers(doc, types.BuildAnswerSupportPlanForBusContext(ctx)); fixed > 0 {
			logging.Warning("[%s] repaired %d principal support member citation carrier(s) from visible answer surface", toolName, fixed)
		}
		if fixed := normalizePrincipalSupportSurfaceTermSupplement(doc, types.BuildAnswerSupportPlanForBusContext(ctx), ctx); fixed > 0 {
			logging.Warning("[%s] materialized %d principal support surface-term row(s) from accepted evidence handoff", toolName, fixed)
		}
	}
	if fixed := normalizeViewCompatibleAnswerDocument(doc, view); fixed > 0 {
		logging.Warning("[%s] repaired %d view-compatible typed lane field(s)", toolName, fixed)
	}
	if fixed := normalizeRuntimeObservationOnlyDecisionBlocks(doc, view, ctx); fixed > 0 {
		logging.Warning("[%s] removed %d runtime-observation-only decision block(s)", toolName, fixed)
	}
	if fixed := normalizeObservedArtifactClaimUseCarriers(doc, ctx); fixed > 0 {
		logging.Warning("[%s] repaired %d observed-artifact claim_use carrier(s)", toolName, fixed)
	}
	if fixed := normalizeCitationBackedPrincipalClaimUses(doc, view, pctx); fixed > 0 {
		logging.Warning("[%s] repaired %d citation-backed principal claim_use carrier(s)", toolName, fixed)
	}
	if fixed := normalizeClaimUseEvidenceIDsByProjection(doc, ctx); fixed > 0 {
		logging.Warning("[%s] detached %d incompatible claim_use evidence_id value(s) from citation-backed blocks", toolName, fixed)
	}
	if fixed := normalizeRuntimeArtifactCitationRefs(doc, ctx); fixed > 0 {
		logging.Warning("[%s] normalized %d runtime-artifact citation carrier(s) to observation provenance", toolName, fixed)
	}
	if fixed := normalizeExternalObservationPseudoCitations(doc, ctx); fixed > 0 {
		logging.Warning("[%s] normalized %d non-line external observation citation carrier(s) to observation provenance", toolName, fixed)
	}
	if fixed := normalizeRuntimeArtifactVisibleCitationSentinels(doc, ctx); fixed > 0 {
		logging.Warning("[%s] sanitized %d runtime-artifact visible citation sentinel(s)", toolName, fixed)
	}
	if fixed := normalizeExternalObservationVisibleCitationSentinels(doc, ctx); fixed > 0 {
		logging.Warning("[%s] sanitized %d external-observation visible citation sentinel(s)", toolName, fixed)
	}
}

func normalizeClaimUseEvidenceIDsByProjection(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil || ctx.Mutable == nil {
		return 0
	}
	pool := ctx.Mutable.EmittedEvidence()
	if len(pool) == 0 {
		return 0
	}
	byID := make(map[string]types.EvidenceItem, len(pool))
	for _, ev := range pool {
		id := strings.TrimSpace(ev.ID)
		if id != "" {
			byID[id] = ev
		}
	}
	if len(byID) == 0 {
		return 0
	}
	fixed := 0
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		if !answerBlockHasValidCitationCarrier(*block, doc) {
			continue
		}
		for ci := range block.ClaimUses {
			claim := &block.ClaimUses[ci]
			evidenceID := strings.TrimSpace(claim.EvidenceID)
			if evidenceID == "" || claim.ClaimForm == "" {
				continue
			}
			ev, ok := byID[evidenceID]
			if !ok {
				continue
			}
			projected := types.ClaimFormOf(ev)
			if projected == types.ClaimUnknown || projected == claim.ClaimForm {
				continue
			}
			claim.EvidenceID = ""
			fixed++
		}
	}
	return fixed
}

func answerBlockHasValidCitationCarrier(block types.AnswerBlock, doc *types.AnswerDocumentV2) bool {
	if doc == nil || len(doc.Citations) == 0 {
		return false
	}
	for _, item := range block.Items {
		if item.CitationRef >= 0 && item.CitationRef < len(doc.Citations) {
			return true
		}
	}
	return false
}

func carryForwardCitationsFromRejectedDraft(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil || ctx.Mutable == nil {
		return 0
	}
	maxRef := -1
	for _, block := range doc.Blocks {
		for _, item := range block.Items {
			if item.CitationRef > maxRef {
				maxRef = item.CitationRef
			}
		}
	}
	if maxRef < 0 || len(doc.Citations) > maxRef {
		return 0
	}
	prev := ctx.Mutable.LastRejectedAnswerDocumentV2()
	if prev == nil || len(prev.Citations) <= maxRef {
		prev = ctx.Mutable.AnswerDocumentV2()
	}
	if prev == nil || len(prev.Citations) <= maxRef {
		return 0
	}
	if len(doc.Citations) > 0 {
		if len(doc.Citations) > len(prev.Citations) {
			return 0
		}
		for i := range doc.Citations {
			if !equivalentAnswerCitation(doc.Citations[i], prev.Citations[i]) {
				return 0
			}
		}
	}
	needed := maxRef + 1
	doc.Citations = append(doc.Citations[:len(doc.Citations):len(doc.Citations)], prev.Citations[len(doc.Citations):needed]...)
	return needed
}

func promoteRecoveredDiagramBlocks(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, report answerDocumentRecoveryReport) int {
	if doc == nil || view == nil || !viewRequiresDiagramBlock(view) || answerDocumentHasBlockKind(doc, types.BlockDiagram) {
		return 0
	}
	// Cap headroom guard: this system-fabricated block runs before
	// the maxBlocksPerDoc hard gate — appending to an at-cap doc
	// would reject the emit with a count the model never produced.
	if len(doc.Blocks) >= maxBlocksPerDoc {
		logging.Warning("[answer_document] recovered diagram promotion skipped: document already at the %d-block cap", maxBlocksPerDoc)
		return 0
	}
	req := firstRequiredBlockRequirement(view, types.BlockDiagram)
	fixed := 0
	for _, att := range report.Attachments {
		if att.Kind != types.AnswerDisplayAttachmentDiagram {
			continue
		}
		body := strings.TrimSpace(att.Body)
		if body == "" {
			continue
		}
		lang := strings.TrimSpace(att.Language)
		if lang == "" {
			lang = "mermaid"
		}
		block := types.AnswerBlock{
			ID:      uniqueAnswerBlockID(doc, "recovered_diagram"),
			Kind:    types.BlockDiagram,
			Title:   strings.TrimSpace(att.Title),
			Diagram: &types.AnswerDiagramBlock{Kind: inferRecoveredDiagramKind(body), Language: lang, Body: body},
		}
		if len(req.FacetIDs) > 0 {
			block.FacetIDs = append([]string(nil), req.FacetIDs...)
		}
		if req.SurfaceRoleHint != "" {
			block.SurfaceRole = req.SurfaceRoleHint
		}
		doc.Blocks = append(doc.Blocks, block)
		fixed++
		break
	}
	return fixed
}

func filterAttachmentsAlreadyRepresentedInDocument(in []types.AnswerDisplayAttachment, doc *types.AnswerDocumentV2) []types.AnswerDisplayAttachment {
	if len(in) == 0 || doc == nil {
		return in
	}
	diagramBodies := make(map[string]bool)
	for _, block := range doc.Blocks {
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		body := strings.TrimSpace(block.Diagram.Body)
		if body != "" {
			diagramBodies[body] = true
		}
	}
	if len(diagramBodies) == 0 {
		return in
	}
	out := make([]types.AnswerDisplayAttachment, 0, len(in))
	for _, att := range in {
		if att.Kind == types.AnswerDisplayAttachmentDiagram && diagramBodies[strings.TrimSpace(att.Body)] {
			continue
		}
		out = append(out, att)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func viewRequiresDiagramBlock(view *types.AnswerSemanticView) bool {
	if view == nil {
		return false
	}
	for _, req := range view.RequiredBlocks {
		if req.Required && req.Kind == types.BlockDiagram && req.MinCount > 0 {
			return true
		}
	}
	return false
}

func answerDocumentHasBlockKind(doc *types.AnswerDocumentV2, kind types.AnswerBlockKind) bool {
	if doc == nil {
		return false
	}
	for _, block := range doc.Blocks {
		if block.Kind == kind {
			return true
		}
	}
	return false
}

func firstRequiredBlockRequirement(view *types.AnswerSemanticView, kind types.AnswerBlockKind) types.BlockRequirement {
	if view == nil {
		return types.BlockRequirement{}
	}
	for _, req := range view.RequiredBlocks {
		if req.Required && req.Kind == kind {
			return req
		}
	}
	return types.BlockRequirement{}
}

func inferRecoveredDiagramKind(body string) types.DiagramKind {
	lower := strings.ToLower(strings.TrimSpace(body))
	switch {
	case strings.Contains(lower, "sequencediagram"):
		return types.DiagramSequence
	case strings.Contains(lower, "flowchart") || strings.Contains(lower, "graph td") ||
		strings.Contains(lower, "graph lr") || strings.Contains(lower, "graph tb") ||
		strings.Contains(lower, "graph bt"):
		return types.DiagramFlow
	default:
		return types.DiagramArchitecture
	}
}

func uniqueAnswerBlockID(doc *types.AnswerDocumentV2, base string) string {
	if doc == nil {
		return base
	}
	used := make(map[string]bool, len(doc.Blocks))
	for _, block := range doc.Blocks {
		used[strings.TrimSpace(block.ID)] = true
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func materializeRequiredCaveatWhenOnlyMissing(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, hints []emitFixHint, ctx *types.BusContext) int {
	if doc == nil || view == nil || len(hints) == 0 || len(view.UncertaintyRules) == 0 {
		return 0
	}
	hasCaveatHint := false
	for _, hint := range hints {
		if hint.Field == "blocks[].kind=caveat" {
			hasCaveatHint = true
			break
		}
	}
	if !hasCaveatHint {
		return 0
	}
	if answerDocumentHasBlockKind(doc, types.BlockCaveat) {
		return 0
	}
	// Cap headroom guard: declining to materialize at the cap keeps
	// the model-attributable "missing caveat" hard hint standing —
	// the model can resolve it by replacing a block. Appending here
	// instead would trip the maxBlocksPerDoc gate with a
	// system-fabricated count.
	if len(doc.Blocks) >= maxBlocksPerDoc {
		logging.Warning("[answer_document] scope caveat materialization skipped: document already at the %d-block cap", maxBlocksPerDoc)
		return 0
	}
	title, text := materializedScopeCaveatCopy(ctx)
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:       uniqueAnswerBlockID(doc, "scope_caveat"),
		Kind:     types.BlockCaveat,
		Title:    title,
		Text:     text,
		FacetIDs: []string{string(types.FacetUncertaintyBoundary)},
	})
	return 1
}

func materializedScopeCaveatCopy(ctx *types.BusContext) (title string, text string) {
	lang := ""
	if ctx != nil {
		lang = ctx.Language
	}
	if answerDocumentToolLangIsZh(lang) {
		return "范围说明", "以下结论仅限于本轮已经读取并校验过的证据；未解析或未搜索到的区域不视为已经被证明。"
	}
	return "Scope note", "This answer is limited to the evidence that was read and validated in this run; unresolved or unsearched areas are not treated as proven."
}

func answerDocumentToolLangIsZh(lang string) bool {
	if strings.TrimSpace(lang) == "" {
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
}

// repairBlocksAsString detects the "blocks[] arrived as JSON
// string" failure mode and returns a re-encoded RawMessage with
// blocks[] inlined as a real array.
//
// Trigger condition: top-level object whose `blocks` field decodes
// as a JSON string AND that string itself decodes as a JSON array.
// Anything else returns (_, false) and the caller proceeds with the
// original raw JSON unchanged — a hard parse error then surfaces
// the original schema violation to the LLM via the existing
// rejection path.
//
// The repair is conservative: only `blocks` is patched; every other
// top-level key passes through verbatim. This means downstream
// V1-field detection (detectV1FieldsInV2Emit) still fires after the
// repair, so we cannot soften any of the V1↔V2 mutual-exclusion
// invariants.
// repairBlocksAsString is the layered recovery entry-point. The
// layers, in order:
//
//  1. Path A — pure-array stringify (`{"blocks":"[…]"}`); the LLM
//     stringified just the blocks array; everything else is at
//     top-level.
//  2. Path B — whole-document stringify (`{"blocks":"[…], \"citations\":[…]"}`);
//     the LLM put the entire answer body inside the blocks string.
//  3. Path C — structured control-char normalisation: Paths A/B fail
//     when raw \n / \r / \t / 0x00–0x1F appears inside a string
//     literal (strict json.Unmarshal rejects unescaped control
//     bytes in strings). The state-machine pass walks every byte,
//     re-escapes control bytes ONLY inside string scope, and retries
//     Paths A/B.
//  4. Path D (heuristic) — brace-balanced block extraction: when
//     everything above fails, walk the trimmed string and emit one
//     `{...}` element per top-level brace pair. Each candidate is
//     individually json.Unmarshal-validated before being kept; bad
//     blocks are silently dropped. Validators downstream still run
//     on the recovered set so a partial recovery does not silently
//     ship — block-coverage / claim-use / facet-coverage oracles
//     will reject if the recovered set is below the contract floor,
//     triggering the standard reject→LLM-retry path.
//
// Each layer falls through to the next on failure. Returns
// `(repaired, true)` on the first successful layer; `(nil, false)`
// when even Path D could not extract a single valid block.
func repairBlocksAsString(raw json.RawMessage) (json.RawMessage, bool) {
	patched, _, ok := repairBlocksAsStringDetailed(raw)
	return patched, ok
}

// RepairEmitAnswerDocumentMalformedParams salvages malformed outer
// emit_answer_document arguments before the tool runtime can decode
// them. It intentionally reuses the same block recovery pipeline as
// the in-tool repair path, so the agent layer does not grow a parallel
// answer-document taxonomy.
func RepairEmitAnswerDocumentMalformedParams(raw json.RawMessage) (json.RawMessage, bool) {
	patched, ok := repairBlocksAsString(raw)
	if !ok {
		return nil, false
	}
	recovered := extractLooseAnswerDocumentSiblingFields(string(raw))
	if len(recovered) == 0 {
		return patched, true
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(patched, &merged); err != nil || len(merged) == 0 {
		return patched, true
	}
	changed := false
	for k, v := range recovered {
		if _, exists := merged[k]; exists {
			continue
		}
		merged[k] = v
		changed = true
	}
	if !changed {
		return patched, true
	}
	out, err := json.Marshal(merged)
	if err != nil || !json.Valid(out) {
		return patched, true
	}
	return out, true
}

func repairBlocksAsStringDetailed(raw json.RawMessage) (json.RawMessage, answerDocumentRecoveryReport, bool) {
	if len(raw) == 0 {
		return nil, answerDocumentRecoveryReport{}, false
	}
	// Pass 1: try as-is (handles the well-formed string-wrapped
	// case where Paths A or B succeed without normalisation).
	if patched, ok := tryRepairBlocksAsString(raw); ok {
		return patched, losslessRecoveryReport("string_wrapped_blocks", patched), true
	}
	// Pass 2 (Path C): normalise control characters inside string
	// scope, then retry the structural Paths A/B. Fixes the streaming
	// bug where the LLM emits unescaped \n / \r / \t inside diagram
	// bodies / multi-line text fields.
	if normalised, changed := normalizeControlCharsInJSONStrings(string(raw)); changed {
		if patched, ok := tryRepairBlocksAsString(json.RawMessage(normalised)); ok {
			return patched, losslessRecoveryReport("control_char_normalized_blocks", patched), true
		}
	}
	// Pass 3 (Path D, heuristic): brace-balanced extraction. When
	// structural normalisation cannot rescue the payload (truncated
	// closing brackets, mid-string unescaped quotes, mixed corruption),
	// walk byte-by-byte and emit one valid `{...}` block per top-level
	// brace pair. Bad blocks drop silently; validator oracles
	// downstream catch insufficient recovery and trigger LLM retry
	// through the existing reject→retry path.
	if patched, report, ok := extractBlocksByBraceBalanceDetailed(raw); ok {
		return patched, report, true
	}
	return nil, answerDocumentRecoveryReport{}, false
}

// tryRepairBlocksAsString runs Path A + Path B against the supplied
// payload (no normalisation). Returns (patched, true) on success;
// (nil, false) on any structural mismatch — including the outer
// json.Unmarshal failing, which is the dominant failure mode when
// the outer raw itself contains unescaped control characters.
func tryRepairBlocksAsString(raw json.RawMessage) (json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, false
	}
	rawBlocks, ok := probe["blocks"]
	if !ok {
		return nil, false
	}
	if len(rawBlocks) == 0 || rawBlocks[0] != '"' {
		return nil, false
	}
	var encoded string
	if err := json.Unmarshal(rawBlocks, &encoded); err != nil {
		return nil, false
	}
	trimmed := strings.TrimSpace(encoded)
	if !strings.HasPrefix(trimmed, "[") {
		return nil, false
	}
	if patched, ok := tryRepairBlocksStringBody(probe, trimmed); ok {
		return patched, true
	}
	if repaired, changed := stripDanglingQuotesAfterCompositeValues(trimmed); changed {
		if patched, ok := tryRepairBlocksStringBody(probe, repaired); ok {
			return patched, true
		}
	}
	if normalised, changed := normalizeControlCharsInJSONStrings(trimmed); changed {
		if patched, ok := tryRepairBlocksStringBody(probe, normalised); ok {
			return patched, true
		}
		if repaired, changed := stripDanglingQuotesAfterCompositeValues(normalised); changed {
			if patched, ok := tryRepairBlocksStringBody(probe, repaired); ok {
				return patched, true
			}
		}
	}
	return nil, false
}

func tryRepairBlocksStringBody(probe map[string]json.RawMessage, trimmed string) (json.RawMessage, bool) {
	if !strings.HasPrefix(strings.TrimSpace(trimmed), "[") {
		return nil, false
	}
	// Path A: pure-array stringify. Repair just blocks; preserve all
	// other top-level keys verbatim.
	var inner []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &inner); err == nil {
		merged := cloneRawMessageMap(probe)
		merged["blocks"] = mustMarshal(inner)
		if patched, err := json.Marshal(merged); err == nil {
			return patched, true
		}
	}
	// Path A1: stringified blocks array with a missing top-level
	// block close before the next block begins. This is a common
	// model syntax slip for diagram/table blocks that contain nested
	// objects. The repair is still lossless: it only inserts the
	// missing outer `}` at a top-level array element boundary, and it
	// is accepted only if the entire result parses into block-shaped
	// objects.
	if repairedBlocks, ok := repairAnswerBlocksArraySyntax(trimmed); ok {
		merged := cloneRawMessageMap(probe)
		merged["blocks"] = repairedBlocks
		if patched, err := json.Marshal(merged); err == nil {
			return patched, true
		}
	}
	// Path B: whole-document stringify — the LLM sometimes places
	// the whole answer body inside the blocks string:
	//
	//   "[{block...}], \"citations\": [...]"
	//
	// and sometimes includes the closing object brace too:
	//
	//   "[{block...}], \"citations\": [...]}"
	//
	// Build the candidate object from the leading blocks array plus
	// sibling fields instead of blindly suffixing a brace, otherwise
	// the second shape falls through to Path D and citation objects
	// can be mistaken for block objects.
	if patched, ok := finishWholeDocumentStringRepair(probe, trimmed); ok {
		return patched, true
	}
	return nil, false
}

func cloneRawMessageMap(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in))
	for k, v := range in {
		out[k] = append(json.RawMessage(nil), v...)
	}
	return out
}

func stripDanglingQuotesAfterCompositeValues(s string) (string, bool) {
	if s == "" {
		return s, false
	}
	var out strings.Builder
	out.Grow(len(s))
	inStr := false
	esc := false
	changed := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			out.WriteByte(c)
			if esc {
				esc = false
				continue
			}
			switch c {
			case '\\':
				esc = true
			case '"':
				inStr = false
			}
			continue
		}
		if c == '"' && danglingQuoteAfterCompositeValue(s, i) {
			changed = true
			continue
		}
		out.WriteByte(c)
		if c == '"' {
			inStr = true
		}
	}
	if !changed {
		return s, false
	}
	return out.String(), true
}

func danglingQuoteAfterCompositeValue(s string, quoteAt int) bool {
	prev := byte(0)
	for i := quoteAt - 1; i >= 0; i-- {
		if isAnswerDocJSONSpace(s[i]) {
			continue
		}
		prev = s[i]
		break
	}
	if prev != ']' && prev != '}' {
		return false
	}
	next := byte(0)
	for i := quoteAt + 1; i < len(s); i++ {
		if isAnswerDocJSONSpace(s[i]) {
			continue
		}
		next = s[i]
		break
	}
	switch next {
	case '}', ']', ',':
		return true
	default:
		return false
	}
}

func losslessRecoveryReport(mode string, patched json.RawMessage) answerDocumentRecoveryReport {
	kinds := recoveredBlockKinds(patched)
	return answerDocumentRecoveryReport{
		Mode:            mode,
		Lossless:        true,
		CandidateBlocks: len(kinds),
		RecoveredBlocks: len(kinds),
		CandidateKinds:  append([]types.AnswerBlockKind(nil), kinds...),
		RecoveredKinds:  kinds,
	}
}

func recoveredBlockKinds(raw json.RawMessage) []types.AnswerBlockKind {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(probe["blocks"], &blocks); err != nil {
		return nil
	}
	out := make([]types.AnswerBlockKind, 0, len(blocks))
	for _, blk := range blocks {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(blk, &obj); err != nil {
			continue
		}
		if kind, ok := blockKindFromObject(obj); ok {
			out = append(out, kind)
		}
	}
	return out
}

// extractBlocksByBraceBalance is Path D's heuristic recovery.
//
// The input is the original raw payload (post-normalisation if Path
// C ran). The function:
//
//  1. Locates the JSON-encoded string value of the `blocks` key by
//     finding the first `"blocks"` entry at the top level of the
//     outer object. We can't rely on json.Unmarshal because the
//     payload is corrupt; we use a small state-machine to find the
//     blocks-string boundary ourselves.
//  2. Walks the inner string content (or directly the raw if blocks
//     turns out to be a real array) byte-by-byte, tracking string
//     state + brace depth. Each top-level `{...}` group at depth 0
//     becomes one element candidate.
//  3. Each candidate is independently `json.Unmarshal`-validated
//     against `map[string]json.RawMessage`. Pass = keep; fail = drop.
//  4. If at least one valid block survives, marshals the recovered
//     array as the new blocks value and returns the merged top-level
//     payload.
//
// Heuristic safety: per-block validation prevents structurally-broken
// bytes from poisoning the merged payload. The downstream block-
// coverage / claim-use / facet-coverage oracles will reject if the
// recovered set is incomplete — that triggers the existing
// reject→LLM-retry path, so a "partial-but-incomplete" recovery does
// NOT silently ship.
func extractBlocksByBraceBalance(raw json.RawMessage) (json.RawMessage, bool) {
	patched, _, ok := extractBlocksByBraceBalanceDetailed(raw)
	return patched, ok
}

func extractBlocksByBraceBalanceDetailed(raw json.RawMessage) (json.RawMessage, answerDocumentRecoveryReport, bool) {
	if len(raw) == 0 {
		return nil, answerDocumentRecoveryReport{}, false
	}
	body, leadKeys, trailKeys, ok := isolateBlocksStringBody(raw)
	if !ok {
		return nil, answerDocumentRecoveryReport{}, false
	}
	body = strings.TrimSpace(body)
	if len(body) < 2 {
		return nil, answerDocumentRecoveryReport{}, false
	}
	// Strip leading `[` if present; final `]` is balanced via
	// state-machine below.
	start := 0
	if body[0] == '[' {
		start = 1
	}
	scan := body[start:]

	var elements []json.RawMessage
	var candidateKinds []types.AnswerBlockKind
	var recoveredKinds []types.AnswerBlockKind
	var attachments []types.AnswerDisplayAttachment
	repairedMalformedCandidate := false
	depth := 0
	inStr := false
	esc := false
	elemStart := -1
	for i := 0; i < len(scan); i++ {
		c := scan[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			switch c {
			case '\\':
				esc = true
			case '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			if depth <= 0 {
				elemStart = i
				depth = 1
				continue
			}
			depth++
		case '}':
			if depth <= 0 {
				depth = 0
				elemStart = -1
				continue
			}
			depth--
			if depth == 0 && elemStart >= 0 {
				candidate := strings.TrimSpace(scan[elemStart : i+1])
				elemStart = -1
				if candidate == "" {
					continue
				}
				if kind, likely := answerBlockKindFromCandidate(candidate); likely {
					candidateKinds = append(candidateKinds, kind)
				}
				var probe map[string]json.RawMessage
				if err := json.Unmarshal([]byte(candidate), &probe); err == nil && isAnswerBlockCandidate(probe) {
					recovered := json.RawMessage(candidate)
					if annotated, ok := mergeTrailingAnswerBlockAnnotations(probe, scan[i+1:]); ok {
						recovered = annotated
					}
					elements = append(elements, recovered)
					if kind, ok := blockKindFromObject(probe); ok {
						recoveredKinds = append(recoveredKinds, kind)
					}
				} else if repaired, ok := recoverMalformedDiagramCandidateBlock(candidate); ok {
					elements = append(elements, repaired)
					recoveredKinds = append(recoveredKinds, types.BlockDiagram)
					repairedMalformedCandidate = true
				} else if att, ok := displayAttachmentFromMalformedCandidate(candidate); ok {
					attachments = appendRecoveredAttachment(attachments, att)
				}
				// Else drop silently — downstream validators catch
				// insufficient recovery and trigger LLM retry.
			}
		case '[':
			if depth > 0 {
				depth++ // nested array inside a block — count as depth
			}
			// outer-level `[` (e.g. starting nested arrays inside
			// values) is structurally fine; we only treat braces
			// as block boundaries.
		case ']':
			if depth > 0 {
				depth--
			} else {
				depth = 0
			}
		}
	}
	if len(elements) == 0 {
		return nil, answerDocumentRecoveryReport{}, false
	}
	if !containsAnswerBlockKind(recoveredKinds, types.BlockDiagram) {
		if att, ok := displayAttachmentFromText(body); ok {
			attachments = appendRecoveredAttachment(attachments, att)
		}
	}
	if recovered := extractLooseAnswerDocumentSiblingFields(body); len(recovered) > 0 {
		if trailKeys == nil {
			trailKeys = make(map[string]json.RawMessage, len(recovered))
		}
		for k, v := range recovered {
			if _, ok := leadKeys[k]; ok {
				continue
			}
			if _, ok := trailKeys[k]; ok {
				continue
			}
			trailKeys[k] = v
		}
	}
	candidateBlocks := len(candidateKinds)
	if markerBlocks := countAnswerBlockKindMarkers(body); markerBlocks > candidateBlocks {
		candidateBlocks = markerBlocks
	}
	// Reconstruct merged payload: leadKeys ∪ {"blocks": elements} ∪
	// trailKeys. Keys collide only on `blocks`; the recovered array
	// wins. Other key data is preserved byte-for-byte.
	merged := make(map[string]json.RawMessage, len(leadKeys)+len(trailKeys)+1)
	for k, v := range leadKeys {
		merged[k] = v
	}
	for k, v := range trailKeys {
		merged[k] = v
	}
	merged["blocks"] = mustMarshal(elements)
	patched, err := json.Marshal(merged)
	if err != nil {
		return nil, answerDocumentRecoveryReport{}, false
	}
	report := answerDocumentRecoveryReport{
		Mode:            "brace_balanced_blocks",
		Lossless:        !repairedMalformedCandidate && candidateBlocks == len(recoveredKinds) && len(attachments) == 0,
		CandidateBlocks: candidateBlocks,
		RecoveredBlocks: len(recoveredKinds),
		CandidateKinds:  candidateKinds,
		RecoveredKinds:  recoveredKinds,
		Attachments:     attachments,
	}
	report.DroppedVisiblePayload = len(attachments) > 0 || candidateBlocks > len(recoveredKinds)
	if candidateBlocks == 0 {
		report.CandidateBlocks = len(recoveredKinds)
		report.CandidateKinds = append([]types.AnswerBlockKind(nil), recoveredKinds...)
		report.Lossless = len(attachments) == 0
		report.DroppedVisiblePayload = len(attachments) > 0
	}
	return patched, report, true
}

func mergeTrailingAnswerBlockAnnotations(probe map[string]json.RawMessage, tail string) (json.RawMessage, bool) {
	if !isAnswerBlockCandidate(probe) {
		return nil, false
	}
	bounded := strings.TrimSpace(answerBlockAnnotationTail(tail))
	if bounded == "" {
		return nil, false
	}
	merged := cloneRawMessageMap(probe)
	changed := false
	for _, field := range []struct {
		name  string
		open  byte
		close byte
	}{
		{"facet_ids", '[', ']'},
		{"claim_uses", '[', ']'},
		{"edge_anchors", '[', ']'},
	} {
		if _, exists := merged[field.name]; exists {
			continue
		}
		raw, ok := extractLooseJSONFieldValue(bounded, field.name, field.open, field.close)
		if !ok {
			continue
		}
		merged[field.name] = raw
		changed = true
	}
	for _, field := range []string{"surface_role", "title"} {
		if _, exists := merged[field]; exists {
			continue
		}
		value, ok := extractJSONStringFieldLoose(bounded, field)
		if !ok {
			continue
		}
		raw, err := json.Marshal(value)
		if err != nil {
			continue
		}
		merged[field] = raw
		changed = true
	}
	if !changed {
		return nil, false
	}
	out, err := json.Marshal(merged)
	if err != nil || !json.Valid(out) {
		return nil, false
	}
	var wire emitAnswerBlockV2
	if err := json.Unmarshal(out, &wire); err != nil {
		return nil, false
	}
	if _, err := NormalizeEmitAnswerBlock(wire, "recovered_blocks[0]"); err != nil {
		return nil, false
	}
	return json.RawMessage(out), true
}

func answerBlockAnnotationTail(tail string) string {
	if tail == "" {
		return ""
	}
	depth := 0
	inStr := false
	esc := false
	for i := 0; i < len(tail); i++ {
		c := tail[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			switch c {
			case '\\':
				esc = true
			case '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			if depth == 0 {
				return tail[:i]
			}
			depth++
		case '[':
			depth++
		case '}', ']':
			if depth > 0 {
				depth--
			}
		}
	}
	return tail
}

func countAnswerBlockKindMarkers(s string) int {
	decoded := decodeEscapedTextLoose(s)
	compact := strings.NewReplacer(" ", "", "\n", "", "\r", "", "\t", "").Replace(decoded)
	total := 0
	for _, kind := range types.AllAnswerBlockKinds() {
		k := string(kind)
		total += strings.Count(compact, `"kind":"`+k+`"`)
	}
	return total
}

// isAnswerBlockCandidate is Path D's structural filter. The brace
// extractor scans the entire stringified payload, so top-level sibling
// objects such as citations (`{"file":"x.go","line":1}`) can appear at
// the same brace depth as real answer blocks. Keep only block-shaped
// objects here; NormalizeEmitAnswerBlock still performs the full
// validation later.
func isAnswerBlockCandidate(probe map[string]json.RawMessage) bool {
	if len(probe) == 0 {
		return false
	}
	if _, ok := probe["id"]; !ok {
		return false
	}
	if _, ok := probe["kind"]; !ok {
		return false
	}
	return true
}

func recoverMalformedDiagramCandidateBlock(candidate string) (json.RawMessage, bool) {
	kind, likely := answerBlockKindFromCandidate(candidate)
	if !likely || kind != types.BlockDiagram {
		return nil, false
	}
	body := extractDiagramBodyLoose(candidate)
	if body == "" {
		body = extractMermaidBody(candidate)
	}
	body = cleanRecoveredDiagramBody(body)
	if body == "" || !textLooksLikeMermaid(body) {
		return nil, false
	}
	id, _ := extractJSONStringFieldLoose(candidate, "id")
	id = strings.TrimSpace(id)
	if id == "" {
		id = "recovered_diagram"
	}
	title, _ := extractJSONStringFieldLoose(candidate, "title")
	block := map[string]interface{}{
		"id":    id,
		"kind":  string(types.BlockDiagram),
		"title": strings.TrimSpace(title),
		"diagram": map[string]interface{}{
			"kind":     string(inferRecoveredDiagramKind(body)),
			"language": "mermaid",
			"body":     body,
		},
	}
	if strings.TrimSpace(title) == "" {
		delete(block, "title")
	}
	raw, err := json.Marshal(block)
	if err != nil {
		return nil, false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil || !isAnswerBlockCandidate(probe) {
		return nil, false
	}
	return raw, true
}

func extractLooseAnswerDocumentSiblingFields(body string) map[string]json.RawMessage {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	out := make(map[string]json.RawMessage)
	for _, field := range []struct {
		name  string
		open  byte
		close byte
	}{
		{"citations", '[', ']'},
		{"caveats", '[', ']'},
		{"snippets", '[', ']'},
		{"missing_requested_roles", '[', ']'},
		{"exact_resolution", '{', '}'},
	} {
		if raw, ok := extractLooseJSONFieldValue(body, field.name, field.open, field.close); ok {
			if !looseAnswerDocumentSiblingFieldValid(field.name, raw) {
				continue
			}
			out[field.name] = raw
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func extractLooseJSONFieldValue(s, field string, open, close byte) (json.RawMessage, bool) {
	needle := `"` + field + `"`
	searchFrom := 0
	for {
		idx := strings.Index(s[searchFrom:], needle)
		if idx < 0 {
			return nil, false
		}
		idx += searchFrom
		cur := idx + len(needle)
		for cur < len(s) && isAnswerDocJSONSpace(s[cur]) {
			cur++
		}
		if cur >= len(s) || s[cur] != ':' {
			searchFrom = idx + len(needle)
			continue
		}
		cur++
		for cur < len(s) && isAnswerDocJSONSpace(s[cur]) {
			cur++
		}
		if cur >= len(s) || s[cur] != open {
			searchFrom = idx + len(needle)
			continue
		}
		end := findMatchingJSONClose(s, cur, open, close)
		if end < 0 {
			searchFrom = idx + len(needle)
			continue
		}
		raw := strings.TrimSpace(s[cur : end+1])
		if json.Valid([]byte(raw)) {
			return json.RawMessage(raw), true
		}
		searchFrom = end + 1
	}
}

func looseAnswerDocumentSiblingFieldValid(field string, raw json.RawMessage) bool {
	switch field {
	case "citations":
		var v []emitAnswerCitationV2
		return json.Unmarshal(raw, &v) == nil
	case "caveats":
		var v []string
		return json.Unmarshal(raw, &v) == nil
	case "snippets":
		var v []emitCodeSnippetV2
		return json.Unmarshal(raw, &v) == nil
	case "missing_requested_roles":
		var v []types.AnswerMissingRequestedRole
		return json.Unmarshal(raw, &v) == nil
	case "exact_resolution":
		var v types.AnswerExactResolution
		return json.Unmarshal(raw, &v) == nil
	default:
		return false
	}
}

func blockKindFromObject(probe map[string]json.RawMessage) (types.AnswerBlockKind, bool) {
	raw, ok := probe["kind"]
	if !ok {
		return "", false
	}
	var kind string
	if err := json.Unmarshal(raw, &kind); err != nil {
		return "", false
	}
	k := types.AnswerBlockKind(strings.TrimSpace(kind))
	if !types.IsValidAnswerBlockKind(k) {
		return "", false
	}
	return k, true
}

func answerBlockKindFromCandidate(candidate string) (types.AnswerBlockKind, bool) {
	if !strings.Contains(candidate, `"id"`) || !strings.Contains(candidate, `"kind"`) {
		return "", false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(candidate), &probe); err == nil {
		return blockKindFromObject(probe)
	}
	kind, ok := extractJSONStringFieldLoose(candidate, "kind")
	if !ok {
		return "", true
	}
	k := types.AnswerBlockKind(strings.TrimSpace(kind))
	if !types.IsValidAnswerBlockKind(k) {
		return "", true
	}
	return k, true
}

func containsAnswerBlockKind(kinds []types.AnswerBlockKind, want types.AnswerBlockKind) bool {
	for _, kind := range kinds {
		if kind == want {
			return true
		}
	}
	return false
}

func appendRecoveredAttachment(in []types.AnswerDisplayAttachment, att types.AnswerDisplayAttachment) []types.AnswerDisplayAttachment {
	if strings.TrimSpace(att.Body) == "" {
		return in
	}
	if att.Hash == "" {
		att.Hash = answerDisplayAttachmentHash(att.Kind, att.Language, att.Body)
	}
	for _, existing := range in {
		if existing.Hash != "" && existing.Hash == att.Hash {
			return in
		}
		if strings.TrimSpace(existing.Body) == strings.TrimSpace(att.Body) &&
			strings.TrimSpace(existing.Kind) == strings.TrimSpace(att.Kind) {
			return in
		}
	}
	return append(in, att)
}

func displayAttachmentFromMalformedCandidate(candidate string) (types.AnswerDisplayAttachment, bool) {
	kind, likely := answerBlockKindFromCandidate(candidate)
	if !likely && !textLooksLikeMermaid(candidate) {
		return types.AnswerDisplayAttachment{}, false
	}
	if kind != "" && kind != types.BlockDiagram && !textLooksLikeMermaid(candidate) {
		return types.AnswerDisplayAttachment{}, false
	}
	body := extractDiagramBodyLoose(candidate)
	if body == "" {
		body = extractMermaidBody(candidate)
	}
	return newRecoveredDiagramAttachment(body)
}

func displayAttachmentFromText(s string) (types.AnswerDisplayAttachment, bool) {
	if !textLooksLikeMermaid(s) {
		return types.AnswerDisplayAttachment{}, false
	}
	return newRecoveredDiagramAttachment(extractMermaidBody(s))
}

func newRecoveredDiagramAttachment(body string) (types.AnswerDisplayAttachment, bool) {
	body = cleanRecoveredDiagramBody(body)
	if body == "" || !textLooksLikeMermaid(body) {
		return types.AnswerDisplayAttachment{}, false
	}
	att := types.AnswerDisplayAttachment{
		Kind:     types.AnswerDisplayAttachmentDiagram,
		Language: "mermaid",
		Body:     body,
		Source:   "emit_answer_document.recovery",
		Reason:   "structured recovery could not preserve every visible block",
	}
	att.Hash = answerDisplayAttachmentHash(att.Kind, att.Language, att.Body)
	return att, true
}

func answerDisplayAttachmentHash(kind, lang, body string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(kind) + "\x00" + strings.TrimSpace(lang) + "\x00" + strings.TrimSpace(body)))
	return fmt.Sprintf("%x", sum[:])
}

func extractDiagramBodyLoose(candidate string) string {
	var typed struct {
		Diagram *struct {
			Body string `json:"body"`
		} `json:"diagram"`
	}
	if err := json.Unmarshal([]byte(candidate), &typed); err == nil && typed.Diagram != nil {
		if body := strings.TrimSpace(typed.Diagram.Body); body != "" {
			return body
		}
	}
	body, ok := extractJSONStringFieldLoose(candidate, "body")
	if !ok {
		return ""
	}
	return body
}

func extractJSONStringFieldLoose(s, field string) (string, bool) {
	needle := `"` + field + `"`
	searchFrom := 0
	for {
		idx := strings.Index(s[searchFrom:], needle)
		if idx < 0 {
			return "", false
		}
		idx += searchFrom
		cur := idx + len(needle)
		for cur < len(s) && isAnswerDocJSONSpace(s[cur]) {
			cur++
		}
		if cur >= len(s) || s[cur] != ':' {
			searchFrom = idx + len(needle)
			continue
		}
		cur++
		for cur < len(s) && isAnswerDocJSONSpace(s[cur]) {
			cur++
		}
		if cur >= len(s) || s[cur] != '"' {
			searchFrom = idx + len(needle)
			continue
		}
		if value, ok := readJSONStringLiteralLoose(s, cur); ok {
			return value, true
		}
		searchFrom = idx + len(needle)
	}
}

func readJSONStringLiteralLoose(s string, quoteAt int) (string, bool) {
	if quoteAt < 0 || quoteAt >= len(s) || s[quoteAt] != '"' {
		return "", false
	}
	esc := false
	for i := quoteAt + 1; i < len(s); i++ {
		c := s[i]
		if esc {
			esc = false
			continue
		}
		if c == '\\' {
			esc = true
			continue
		}
		if c != '"' {
			continue
		}
		j := i + 1
		for j < len(s) && isAnswerDocJSONSpace(s[j]) {
			j++
		}
		if j < len(s) && s[j] != ',' && s[j] != '}' {
			continue
		}
		raw := s[quoteAt : i+1]
		if value, err := strconv.Unquote(raw); err == nil {
			return value, true
		}
		if normalised, changed := normalizeControlCharsInJSONStrings(raw); changed {
			if value, err := strconv.Unquote(normalised); err == nil {
				return value, true
			}
		}
		return decodeEscapedTextLoose(strings.Trim(raw, `"`)), true
	}
	return "", false
}

func isAnswerDocJSONSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func textLooksLikeMermaid(s string) bool {
	lower := strings.ToLower(s)
	for _, marker := range []string{
		"sequencediagram",
		"flowchart ",
		"graph td",
		"graph lr",
		"graph tb",
		"graph bt",
		"statediagram",
		"classdiagram",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func extractMermaidBody(s string) string {
	lower := strings.ToLower(s)
	best := -1
	for _, marker := range []string{
		"sequencediagram",
		"flowchart ",
		"graph td",
		"graph lr",
		"graph tb",
		"graph bt",
		"statediagram",
		"classdiagram",
	} {
		if idx := strings.Index(lower, marker); idx >= 0 && (best < 0 || idx < best) {
			best = idx
		}
	}
	if best < 0 {
		return ""
	}
	return cleanRecoveredDiagramBody(s[best:])
}

func cleanRecoveredDiagramBody(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.TrimPrefix(s, "```mermaid")
	s = strings.TrimPrefix(s, "```")
	if end := strings.Index(s, "```"); end >= 0 {
		s = s[:end]
	}
	for _, field := range []string{`"edge_anchors"`, `"claim_uses"`, `"facet_ids"`, `"surface_role"`, `"citations"`, `"caveats"`, `"snippets"`} {
		if idx := strings.Index(s, field); idx > 0 {
			s = s[:idx]
		}
	}
	s = decodeEscapedTextLoose(s)
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, `"' ,}]`)
	s = strings.TrimSpace(s)
	return s
}

func decodeEscapedTextLoose(s string) string {
	replacer := strings.NewReplacer(
		`\\n`, "\n",
		`\n`, "\n",
		`\\r`, "\n",
		`\r`, "\n",
		`\\t`, "\t",
		`\t`, "\t",
		`\"`, `"`,
		`\\`, `\`,
	)
	return replacer.Replace(s)
}

// isolateBlocksStringBody finds the `blocks` string value within a
// corrupt outer JSON object and returns its (best-effort decoded)
// content along with the unaffected outer keys. Used by Path D when
// json.Unmarshal of the outer raw fails.
//
// Strategy:
//  1. Try the happy path: `json.Unmarshal(raw, &probe)`. If success
//     and `blocks` is a JSON string, decode it and return its body
//     plus all other top-level keys. (Same conditions as
//     tryRepairBlocksAsString — ensures Path D can rescue cases that
//     pass the outer parse but fail the inner array parse.)
//  2. Fallback: walk the bytes manually to extract the value of
//     `blocks` even when the outer object is malformed. Returns the
//     content between the opening / closing quotes of the blocks
//     string (or the array body if it is a real array). Other keys
//     cannot be rescued in this branch — empty maps returned.
func isolateBlocksStringBody(raw json.RawMessage) (body string, lead map[string]json.RawMessage, trail map[string]json.RawMessage, ok bool) {
	// Branch 1: outer JSON parses cleanly.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err == nil {
		rawBlocks, present := probe["blocks"]
		if !present {
			return "", nil, nil, false
		}
		// Other keys preserved.
		others := make(map[string]json.RawMessage, len(probe)-1)
		for k, v := range probe {
			if k == "blocks" {
				continue
			}
			others[k] = v
		}
		switch {
		case len(rawBlocks) == 0:
			return "", nil, nil, false
		case rawBlocks[0] == '[':
			// Already a real array — repair was not needed. Path D
			// must NOT fire on well-formed input; the contract is
			// "repair returns true ONLY when an actual structural fix
			// was made", and a clean array means no fix is needed.
			// (If the array body itself were malformed we would still
			// hit Branch 2 below via outer parse failure.)
			return "", nil, nil, false
		case rawBlocks[0] == '"':
			var encoded string
			if err := json.Unmarshal(rawBlocks, &encoded); err != nil {
				return "", nil, nil, false
			}
			return encoded, others, nil, true
		default:
			return "", nil, nil, false
		}
	}
	// Branch 2: outer JSON broken. Best-effort byte walk to find
	// `"blocks"` key + its associated value.
	bs := []byte(raw)
	idx := bytes.Index(bs, []byte(`"blocks"`))
	if idx < 0 {
		return "", nil, nil, false
	}
	// Skip past `"blocks"`, optional whitespace, ':' separator.
	cur := idx + len(`"blocks"`)
	for cur < len(bs) && (bs[cur] == ' ' || bs[cur] == '\t') {
		cur++
	}
	if cur >= len(bs) || bs[cur] != ':' {
		return "", nil, nil, false
	}
	cur++
	for cur < len(bs) && (bs[cur] == ' ' || bs[cur] == '\t') {
		cur++
	}
	if cur >= len(bs) {
		return "", nil, nil, false
	}
	switch bs[cur] {
	case '[':
		// Find balanced closing `]` via state-machine.
		depth := 0
		inStr := false
		esc := false
		closeAt := -1
		for j := cur; j < len(bs); j++ {
			c := bs[j]
			if inStr {
				if esc {
					esc = false
					continue
				}
				switch c {
				case '\\':
					esc = true
				case '"':
					inStr = false
				}
				continue
			}
			if c == '"' {
				inStr = true
				continue
			}
			if c == '[' {
				depth++
			} else if c == ']' {
				depth--
				if depth == 0 {
					closeAt = j
					break
				}
			}
		}
		if closeAt < 0 {
			// Truncated; return whatever we have.
			return string(bs[cur:]), nil, nil, true
		}
		return string(bs[cur : closeAt+1]), nil, nil, true
	case '"':
		// Find balanced closing `"` via state-machine.
		esc := false
		closeAt := -1
		for j := cur + 1; j < len(bs); j++ {
			c := bs[j]
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == '"' {
				closeAt = j
				break
			}
		}
		if closeAt < 0 {
			return "", nil, nil, false
		}
		// json.Unmarshal the quoted string to get the decoded body.
		var encoded string
		if err := json.Unmarshal(bs[cur:closeAt+1], &encoded); err != nil {
			return "", nil, nil, false
		}
		return encoded, nil, nil, true
	default:
		return "", nil, nil, false
	}
}

// finishStringWrappedRepair takes the outer `probe` (top-level keys
// of the original raw payload) and a `wrapped` candidate `{"blocks":
// ... }` byte slice. When the wrapped slice parses successfully and
// its `blocks` value is a real array, the function merges the wrapped
// fields with the outer probe (outer wins on key conflicts other
// than `blocks`) and returns the marshalled patched payload. Used by
// Path B and Path C-B which share this merge / serialise tail.
func finishStringWrappedRepair(probe map[string]json.RawMessage, wrapped []byte) (json.RawMessage, bool) {
	var fullDoc map[string]json.RawMessage
	if err := json.Unmarshal(wrapped, &fullDoc); err != nil {
		return nil, false
	}
	innerBlocks, ok := fullDoc["blocks"]
	if !ok || len(innerBlocks) == 0 || innerBlocks[0] != '[' {
		return nil, false
	}
	merged := make(map[string]json.RawMessage, len(probe)+len(fullDoc))
	for k, v := range fullDoc {
		merged[k] = v
	}
	for k, v := range probe {
		if k == "blocks" {
			continue
		}
		merged[k] = v
	}
	patched, err := json.Marshal(merged)
	if err != nil {
		return nil, false
	}
	return patched, true
}

// finishWholeDocumentStringRepair repairs the common flat-mode shape
// where the `blocks` value is a string that begins with a blocks array
// and may continue with additional top-level document fields. It keeps
// all recovered sibling fields (citations / caveats / exact_resolution /
// snippets / missing_requested_roles) in the document-level container.
func finishWholeDocumentStringRepair(probe map[string]json.RawMessage, trimmed string) (json.RawMessage, bool) {
	trimmed = strings.TrimSpace(trimmed)
	if !strings.HasPrefix(trimmed, "[") {
		return nil, false
	}
	arrayEnd := findMatchingJSONClose(trimmed, 0, '[', ']')
	if arrayEnd < 0 {
		return nil, false
	}
	blocks := strings.TrimSpace(trimmed[:arrayEnd+1])
	rest := strings.TrimSpace(trimmed[arrayEnd+1:])
	if rest == "" {
		return finishStringWrappedRepair(probe, []byte(`{"blocks": `+blocks+`}`))
	}
	if !strings.HasPrefix(rest, ",") {
		return nil, false
	}

	candidates := []string{
		`{"blocks": ` + blocks + rest,
		`{"blocks": ` + blocks + rest + `}`,
	}
	if repairedBlocks, ok := repairAnswerBlocksArraySyntax(blocks); ok {
		repaired := string(repairedBlocks)
		candidates = append([]string{
			`{"blocks": ` + repaired + rest,
			`{"blocks": ` + repaired + rest + `}`,
		}, candidates...)
	}
	for _, candidate := range candidates {
		if patched, ok := finishStringWrappedRepair(probe, []byte(candidate)); ok {
			return patched, true
		}
	}
	return nil, false
}

func repairAnswerBlocksArraySyntax(rawArray string) (json.RawMessage, bool) {
	rawArray = strings.TrimSpace(rawArray)
	if !strings.HasPrefix(rawArray, "[") {
		return nil, false
	}
	for _, candidate := range answerBlocksArraySyntaxCandidates(rawArray) {
		if candidate == rawArray {
			continue
		}
		var inner []json.RawMessage
		if err := json.Unmarshal([]byte(candidate), &inner); err != nil {
			continue
		}
		if !rawAnswerBlockArrayLooksValid(inner) {
			continue
		}
		return mustMarshal(inner), true
	}
	return nil, false
}

func answerBlocksArraySyntaxCandidates(rawArray string) []string {
	candidates := make([]string, 0, 8)
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		candidates = append(candidates, s)
	}
	add(rawArray)
	for i := 0; i < len(candidates) && len(candidates) < 16; i++ {
		base := candidates[i]
		if repaired, changed := repairAnswerBlockProseFieldQuotes(base); changed {
			add(repaired)
		}
		if repaired, changed := insertMissingTopLevelBlockObjectClosers(base); changed {
			add(repaired)
		}
		for _, candidate := range toolparam.JSONRepairCandidates(base) {
			add(candidate)
		}
	}
	return candidates
}

// repairAnswerBlockProseFieldQuotes fixes malformed JSON only when the bad
// quote sits inside a known prose carrier. Structural fields such as id/kind/
// label remain untouched so authority-bearing schema mistakes still fail loud.
func repairAnswerBlockProseFieldQuotes(s string) (string, bool) {
	if strings.TrimSpace(s) == "" {
		return s, false
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	changed := false
	for i := 0; i < len(s); {
		if s[i] != '"' {
			b.WriteByte(s[i])
			i++
			continue
		}
		key, ok := quotedObjectKeyAt(s, i)
		if !ok || !answerBlockProseQuoteRepairField(key) {
			b.WriteByte(s[i])
			i++
			continue
		}
		valueQuote := answerBlockStringFieldValueQuoteAt(s, i)
		if valueQuote < 0 {
			b.WriteByte(s[i])
			i++
			continue
		}
		b.WriteString(s[i:valueQuote])
		lit, end, litChanged, ok := repairJSONStringObjectValueLiteralQuotes(s, valueQuote)
		if !ok {
			b.WriteByte(s[i])
			i++
			continue
		}
		b.WriteString(lit)
		if litChanged {
			changed = true
		}
		i = end
	}
	if !changed {
		return s, false
	}
	return b.String(), true
}

func answerBlockProseQuoteRepairField(field string) bool {
	switch field {
	case "text", "body", "quote", "crossfile_summary":
		return true
	default:
		return false
	}
}

func answerBlockStringFieldValueQuoteAt(s string, keyQuoteAt int) int {
	if keyQuoteAt < 0 || keyQuoteAt >= len(s) || s[keyQuoteAt] != '"' {
		return -1
	}
	i := keyQuoteAt + 1
	esc := false
	for ; i < len(s); i++ {
		ch := s[i]
		if esc {
			esc = false
			continue
		}
		if ch == '\\' {
			esc = true
			continue
		}
		if ch == '"' {
			i++
			break
		}
	}
	for i < len(s) && isJSONSpaceByte(s[i]) {
		i++
	}
	if i >= len(s) || s[i] != ':' {
		return -1
	}
	i++
	for i < len(s) && isJSONSpaceByte(s[i]) {
		i++
	}
	if i >= len(s) || s[i] != '"' {
		return -1
	}
	return i
}

func repairJSONStringObjectValueLiteralQuotes(s string, quoteAt int) (string, int, bool, bool) {
	if quoteAt < 0 || quoteAt >= len(s) || s[quoteAt] != '"' {
		return "", 0, false, false
	}
	var b strings.Builder
	b.Grow(64)
	b.WriteByte('"')
	changed := false
	esc := false
	for i := quoteAt + 1; i < len(s); i++ {
		ch := s[i]
		if esc {
			b.WriteByte(ch)
			esc = false
			continue
		}
		if ch == '\\' {
			b.WriteByte(ch)
			esc = true
			continue
		}
		if ch != '"' {
			b.WriteByte(ch)
			continue
		}
		if jsonObjectStringValueCanCloseAt(s, i) {
			b.WriteByte('"')
			return b.String(), i + 1, changed, true
		}
		b.WriteByte('\\')
		b.WriteByte('"')
		changed = true
	}
	return "", 0, false, false
}

func jsonObjectStringValueCanCloseAt(s string, quoteAt int) bool {
	for i := quoteAt + 1; i < len(s); i++ {
		if isJSONSpaceByte(s[i]) {
			continue
		}
		return s[i] == ',' || s[i] == '}'
	}
	return false
}

func rawAnswerBlockArrayLooksValid(blocks []json.RawMessage) bool {
	if len(blocks) == 0 {
		return false
	}
	for _, raw := range blocks {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return false
		}
		if !isAnswerBlockCandidate(obj) {
			return false
		}
		if _, ok := blockKindFromObject(obj); !ok {
			return false
		}
	}
	return true
}

func insertMissingTopLevelBlockObjectClosers(s string) (string, bool) {
	if strings.TrimSpace(s) == "" {
		return s, false
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	inString := false
	escape := false
	arrayDepth := 0
	objectDepth := 0
	changed := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			b.WriteByte(ch)
			if escape {
				escape = false
				continue
			}
			switch ch {
			case '\\':
				escape = true
			case '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
			b.WriteByte(ch)
		case '[':
			arrayDepth++
			b.WriteByte(ch)
		case ']':
			if arrayDepth > 0 {
				arrayDepth--
			}
			b.WriteByte(ch)
		case '{':
			objectDepth++
			b.WriteByte(ch)
		case '}':
			if objectDepth > 0 {
				objectDepth--
			}
			b.WriteByte(ch)
		case ',':
			if arrayDepth == 1 && objectDepth == 1 && nextNonSpaceByteIs(s, i+1, '{') {
				b.WriteByte('}')
				objectDepth--
				changed = true
			}
			b.WriteByte(ch)
		default:
			b.WriteByte(ch)
		}
	}
	if !changed {
		return s, false
	}
	return b.String(), true
}

func nextNonSpaceByteIs(s string, start int, want byte) bool {
	for i := start; i < len(s); i++ {
		if isAnswerDocJSONSpace(s[i]) {
			continue
		}
		return s[i] == want
	}
	return false
}

func findMatchingJSONClose(s string, openAt int, open, close byte) int {
	if openAt < 0 || openAt >= len(s) || s[openAt] != open {
		return -1
	}
	depth := 0
	inStr := false
	esc := false
	for i := openAt; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			switch c {
			case '\\':
				esc = true
			case '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func normalizeControlCharsInJSONStrings(s string) (string, bool) {
	return toolparam.NormalizeControlCharsInJSONStrings(s)
}

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func convertEmitCitationsToTyped(in []emitAnswerCitationV2) []types.Citation {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.Citation, 0, len(in))
	for _, c := range in {
		out = append(out, types.Citation{
			File:             c.File,
			Line:             c.Line.Int(),
			Quote:            c.Quote,
			Scope:            c.Scope,
			LineEnd:          c.LineEnd.Int(),
			SectionPath:      c.SectionPath,
			FileRoleLabel:    c.FileRoleLabel,
			CrossfileSummary: c.CrossfileSummary,
			NegativePattern:  c.NegativePattern,
		})
	}
	return out
}

func convertEmitCodeSnippetsToTyped(in []emitCodeSnippetV2) []types.CodeSnippet {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.CodeSnippet, 0, len(in))
	for _, s := range in {
		out = append(out, types.CodeSnippet{
			File:      s.File,
			StartLine: s.StartLine.Int(),
			EndLine:   s.EndLine.Int(),
			Language:  s.Language,
			Code:      s.Code,
		})
	}
	return out
}

// repairStringWrappedArrayFields scans every top-level key of raw and repairs
// uniquely recoverable array-shape mistakes. It covers the same MiniMax
// streaming-bug pattern that repairBlocksAsString targets for the `blocks`
// field, generalised to every top-level array slot, and also accepts singleton
// object/string values for known array fields where wrapping into a one-element
// array is lossless.
//
// Layered:
//
//  1. Outer-level Path C normalisation: the outer json.Unmarshal
//     can fail when ANY string field contains raw control bytes
//     (not just array-typed fields). When the bare unmarshal fails
//     and normalisation changes the bytes, retry the unmarshal on
//     the normalised payload.
//  2. Per-field singleton object: known object-array fields that arrive as a
//     raw object are wrapped as a one-element array.
//  3. Per-field Path A: each top-level value that looks like a JSON-encoded
//     string starting with `[` gets unwrapped and parsed as a real array.
//     Success → replace; failure → fall through.
//  4. Per-field Path C: re-run the per-field parse on the normalised inner
//     string when Path A fails (the inner stringified content may itself carry
//     unescaped control chars for the same reason the outer payload did).
//  5. Per-field singleton string/object: known string-array/object-array fields
//     that arrive as a single value are wrapped as one-element arrays.
//
// Returns (raw, nil, false) when nothing needed repair. Returns
// (patched, [list of repaired field names], true) when at least one
// field was rebuilt OR when only the outer normalisation was
// applied (in that case the field list contains the sentinel
// "<outer-normalisation>"). Path D (brace-balance) is intentionally
// NOT generalised here — it is specific to the `blocks` semantics
// (object array with id contracts) and lives in
// extractBlocksByBraceBalance.
func repairStringWrappedArrayFields(raw json.RawMessage) (json.RawMessage, []string, bool) {
	if len(raw) == 0 {
		return nil, nil, false
	}
	var probe map[string]json.RawMessage
	outerNormalised := false
	if err := json.Unmarshal(raw, &probe); err != nil {
		normalised, changed := normalizeControlCharsInJSONStrings(string(raw))
		if !changed {
			return raw, nil, false
		}
		if err := json.Unmarshal([]byte(normalised), &probe); err != nil {
			return raw, nil, false
		}
		outerNormalised = true
	}

	var repaired []string
	for key, val := range probe {
		val = bytes.TrimSpace(val)
		if len(val) == 0 {
			continue
		}
		if val[0] == '{' && topLevelArrayFieldAcceptsSingletonObject(key) {
			var item json.RawMessage
			if err := json.Unmarshal(val, &item); err == nil {
				probe[key] = mustMarshal([]json.RawMessage{item})
				repaired = append(repaired, key)
			}
			continue
		}
		if val[0] != '"' {
			continue
		}
		var encoded string
		if err := json.Unmarshal(val, &encoded); err != nil {
			continue
		}
		trimmed := strings.TrimSpace(encoded)
		if strings.HasPrefix(trimmed, "[") {
			// Path A: direct array decode after the string unwrap.
			var inner []json.RawMessage
			if err := json.Unmarshal([]byte(trimmed), &inner); err == nil {
				probe[key] = mustMarshal(inner)
				repaired = append(repaired, key)
				continue
			}
			if topLevelArrayFieldContainsAnswerBlocks(key) {
				if repairedBlocks, ok := repairAnswerBlocksArraySyntax(trimmed); ok {
					probe[key] = repairedBlocks
					repaired = append(repaired, key)
					continue
				}
			}
			if repairedArray, ok := repairStringWrappedArrayJSON(trimmed); ok {
				probe[key] = repairedArray
				repaired = append(repaired, key)
				continue
			}
			if repairedArray, ok := repairStringWrappedObjectFragmentArray(trimmed); ok {
				probe[key] = repairedArray
				repaired = append(repaired, key)
				continue
			}
			// Path A2: direct array decode after trimming redundant
			// trailing JSON closers. Some local models produce a valid
			// stringified array followed by an extra ']' or '}' while the
			// outer tool-call JSON remains valid, e.g.
			// `"replace_citations":"[{...}]]"`. Only accept this when
			// the suffix after the first balanced array contains closers
			// and whitespace exclusively; comma-led whole-document
			// stringify remains Path B below.
			if trimmedArray, ok := trimRedundantTrailingClosersAfterJSONArray(trimmed); ok {
				if err := json.Unmarshal([]byte(trimmedArray), &inner); err == nil {
					probe[key] = mustMarshal(inner)
					repaired = append(repaired, key)
					continue
				}
			}
			// Path B: whole-document stringify — the LLM put MULTIPLE
			// top-level fields inside this string value (typical when
			// the blocks array is followed by replace_citations / etc
			// inside the same stringified blob). Wrap as
			// `{"<key>": <trimmed>}` and parse; the merged document
			// then has the array at `key` plus the other top-level keys
			// re-surfaced. Outer keys not in the wrapped doc are
			// preserved (the outer probe wins on conflict).
			if patched, ok := mergeWholeDocStringifyVariants(probe, key, trimmed); ok {
				probe = patched
				repaired = append(repaired, key)
				continue
			}
			// Path C inner: normalise control chars inside the inner
			// string and retry both Path A and Path B.
			if normalised, changed := normalizeControlCharsInJSONStrings(trimmed); changed {
				if err := json.Unmarshal([]byte(normalised), &inner); err == nil {
					probe[key] = mustMarshal(inner)
					repaired = append(repaired, key)
					continue
				}
				if topLevelArrayFieldContainsAnswerBlocks(key) {
					if repairedBlocks, ok := repairAnswerBlocksArraySyntax(normalised); ok {
						probe[key] = repairedBlocks
						repaired = append(repaired, key)
						continue
					}
				}
				if trimmedArray, ok := trimRedundantTrailingClosersAfterJSONArray(normalised); ok {
					if err := json.Unmarshal([]byte(trimmedArray), &inner); err == nil {
						probe[key] = mustMarshal(inner)
						repaired = append(repaired, key)
						continue
					}
				}
				if repairedArray, ok := repairStringWrappedObjectFragmentArray(normalised); ok {
					probe[key] = repairedArray
					repaired = append(repaired, key)
					continue
				}
				if patched, ok := mergeWholeDocStringifyVariants(probe, key, normalised); ok {
					probe = patched
					repaired = append(repaired, key)
					continue
				}
			}
		}
		if strings.HasPrefix(trimmed, "{") && topLevelArrayFieldAcceptsSingletonObject(key) {
			var item json.RawMessage
			if err := json.Unmarshal([]byte(trimmed), &item); err == nil {
				probe[key] = mustMarshal([]json.RawMessage{item})
				repaired = append(repaired, key)
				continue
			}
			if normalised, changed := normalizeControlCharsInJSONStrings(trimmed); changed {
				if err := json.Unmarshal([]byte(normalised), &item); err == nil {
					probe[key] = mustMarshal([]json.RawMessage{item})
					repaired = append(repaired, key)
					continue
				}
			}
		}
		if topLevelArrayFieldAcceptsSingletonString(key) && trimmed != "" {
			probe[key] = mustMarshal([]string{trimmed})
			repaired = append(repaired, key)
			continue
		}
	}

	if len(repaired) == 0 && !outerNormalised {
		return raw, nil, false
	}
	patched, err := json.Marshal(probe)
	if err != nil {
		return raw, nil, false
	}
	if len(repaired) == 0 {
		// Outer normalisation alone fixed control-char-in-string-
		// field issues; surface that so the caller can log it
		// distinctly.
		return patched, []string{"<outer-normalisation>"}, true
	}
	return patched, repaired, true
}

func repairSelectedStringWrappedArrayFields(raw json.RawMessage, fields ...string) (json.RawMessage, []string, bool) {
	if len(raw) == 0 || len(fields) == 0 {
		return raw, nil, false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return raw, nil, false
	}
	subset := make(map[string]json.RawMessage, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if val, ok := probe[field]; ok {
			subset[field] = val
		}
	}
	if len(subset) == 0 {
		return raw, nil, false
	}
	subsetRaw, err := json.Marshal(subset)
	if err != nil {
		return raw, nil, false
	}
	repairedSubset, repairedFields, ok := repairStringWrappedArrayFields(subsetRaw)
	if !ok || len(repairedFields) == 0 {
		return raw, nil, false
	}
	var repairedMap map[string]json.RawMessage
	if err := json.Unmarshal(repairedSubset, &repairedMap); err != nil {
		return raw, nil, false
	}
	var repaired []string
	for _, field := range repairedFields {
		if field == "<outer-normalisation>" {
			continue
		}
		val, ok := repairedMap[field]
		if !ok {
			continue
		}
		probe[field] = val
		repaired = append(repaired, field)
	}
	if len(repaired) == 0 {
		return raw, nil, false
	}
	patched, err := json.Marshal(probe)
	if err != nil {
		return raw, nil, false
	}
	return patched, repaired, true
}

func repairStringWrappedArrayJSON(trimmed string) (json.RawMessage, bool) {
	for _, candidate := range toolparam.JSONRepairCandidates(trimmed) {
		if candidate == trimmed {
			continue
		}
		var inner []json.RawMessage
		if err := json.Unmarshal([]byte(candidate), &inner); err != nil {
			continue
		}
		return mustMarshal(inner), true
	}
	return nil, false
}

func repairStringWrappedObjectFragmentArray(trimmed string) (json.RawMessage, bool) {
	candidate, changed := insertMissingObjectBracesInArray(trimmed)
	if !changed {
		return nil, false
	}
	candidate, _ = attachTrailingArrayObjectField(candidate, "load_bearing_summary")
	var inner []json.RawMessage
	if err := json.Unmarshal([]byte(candidate), &inner); err != nil {
		return nil, false
	}
	return mustMarshal(inner), true
}

func insertMissingObjectBracesInArray(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "[") {
		return s, false
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	inString := false
	escape := false
	arrayDepth := 0
	objectDepth := 0
	changed := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			b.WriteByte(ch)
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
			b.WriteByte(ch)
		case '[':
			arrayDepth++
			b.WriteByte(ch)
		case ']':
			if arrayDepth > 0 {
				arrayDepth--
			}
			b.WriteByte(ch)
		case '{':
			objectDepth++
			b.WriteByte(ch)
		case '}':
			if objectDepth > 0 {
				objectDepth--
			}
			b.WriteByte(ch)
		case ',':
			b.WriteByte(ch)
			if arrayDepth == 1 && objectDepth == 0 {
				j := i + 1
				for j < len(s) && isJSONSpaceByte(s[j]) {
					b.WriteByte(s[j])
					j++
				}
				if j < len(s) && s[j] == '"' {
					key, ok := quotedObjectKeyAt(s, j)
					if ok && stringWrappedArrayObjectStartKey(key) {
						b.WriteByte('{')
						objectDepth++
						changed = true
					}
				}
				i = j - 1
			}
		default:
			b.WriteByte(ch)
		}
	}
	if !changed {
		return s, false
	}
	return b.String(), true
}

func attachTrailingArrayObjectField(s, field string) (string, bool) {
	if strings.TrimSpace(field) == "" {
		return s, false
	}
	needle := `}, "` + field + `":`
	idx := strings.LastIndex(s, needle)
	if idx < 0 {
		return s, false
	}
	tail := strings.TrimSpace(s[idx+len(needle):])
	if !strings.HasSuffix(tail, "}]") {
		return s, false
	}
	return s[:idx] + `, "` + field + `":` + s[idx+len(needle):], true
}

func isJSONSpaceByte(b byte) bool {
	return b == ' ' || b == '\n' || b == '\r' || b == '\t'
}

func quotedObjectKeyAt(s string, start int) (string, bool) {
	if start < 0 || start >= len(s) || s[start] != '"' {
		return "", false
	}
	i := start + 1
	var b strings.Builder
	escape := false
	for ; i < len(s); i++ {
		ch := s[i]
		if escape {
			b.WriteByte(ch)
			escape = false
			continue
		}
		if ch == '\\' {
			escape = true
			continue
		}
		if ch == '"' {
			j := i + 1
			for j < len(s) && isJSONSpaceByte(s[j]) {
				j++
			}
			return b.String(), j < len(s) && s[j] == ':'
		}
		b.WriteByte(ch)
	}
	return "", false
}

func stringWrappedArrayObjectStartKey(key string) bool {
	switch key {
	case "id", "label", "text", "source", "subject", "anchor_kind", "anchor_symbol", "evidence_kind", "scope", "file", "line":
		return true
	default:
		return false
	}
}

func topLevelArrayFieldAcceptsSingletonObject(field string) bool {
	switch field {
	case "blocks",
		"citations",
		"snippets",
		"missing_requested_roles",
		"replace_blocks",
		"add_blocks",
		"replace_citations",
		"append_citations",
		"replace_missing_requested_roles",
		"replace_snippets":
		return true
	default:
		return false
	}
}

func topLevelArrayFieldContainsAnswerBlocks(field string) bool {
	switch field {
	case "blocks", "replace_blocks", "add_blocks":
		return true
	default:
		return false
	}
}

func topLevelArrayFieldAcceptsSingletonString(field string) bool {
	switch field {
	case "caveats",
		"replace_caveats",
		"unchanged_block_ids",
		"remove_block_ids":
		return true
	default:
		return false
	}
}

// repairStringWrappedObjectFields repairs selected top-level object fields
// that arrived as JSON-encoded strings. Unlike the array repair above this is
// intentionally field-gated: top-level free-text fields may legitimately begin
// with "{" in future schemas, while exact_resolution-style fields are known
// object carriers.
func repairStringWrappedObjectFields(raw json.RawMessage, fields ...string) (json.RawMessage, []string, bool) {
	if len(raw) == 0 || len(fields) == 0 {
		return nil, nil, false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		normalised, changed := normalizeControlCharsInJSONStrings(string(raw))
		if !changed {
			return raw, nil, false
		}
		if err := json.Unmarshal([]byte(normalised), &probe); err != nil {
			return raw, nil, false
		}
	}

	var repaired []string
	for _, field := range fields {
		if r, ok := repairObjectField(probe, field); ok {
			probe[field] = r
			repaired = append(repaired, field)
		}
	}
	if len(repaired) == 0 {
		return raw, nil, false
	}
	patched, err := json.Marshal(probe)
	if err != nil {
		return raw, nil, false
	}
	return patched, repaired, true
}

func trimRedundantTrailingClosersAfterJSONArray(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	end := findMatchingJSONClose(trimmed, 0, '[', ']')
	if end < 0 || end == len(trimmed)-1 {
		return "", false
	}
	suffix := strings.TrimSpace(trimmed[end+1:])
	if suffix == "" {
		return "", false
	}
	for _, r := range suffix {
		if r != ']' && r != '}' {
			return "", false
		}
	}
	return strings.TrimSpace(trimmed[:end+1]), true
}

func trimRedundantTrailingClosersAfterJSONObject(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "{") {
		return "", false
	}
	end := findMatchingJSONClose(trimmed, 0, '{', '}')
	if end < 0 || end == len(trimmed)-1 {
		return "", false
	}
	suffix := strings.TrimSpace(trimmed[end+1:])
	if suffix == "" {
		return "", false
	}
	for _, r := range suffix {
		if r != ']' && r != '}' {
			return "", false
		}
	}
	return strings.TrimSpace(trimmed[:end+1]), true
}

func mergeWholeDocStringifyVariants(probe map[string]json.RawMessage, key, trimmed string) (map[string]json.RawMessage, bool) {
	candidates := []string{
		`{"` + key + `": ` + trimmed + `}`,
		`{"` + key + `": ` + trimmed,
	}
	for _, candidate := range candidates {
		if patched, ok := mergeWholeDocStringify(probe, key, []byte(candidate)); ok {
			return patched, true
		}
	}
	return nil, false
}

// repairNestedAnswerBlockFields detects the same "JSON-encoded string where a
// native JSON array/object is expected" failure mode for structured fields
// inside each block. It repairs block arrays (items, columns, claim_uses,
// edge_anchors, facet_ids), item arrays (items[].cells), and block objects
// (diagram). Mirrors repairBlocksAsString
// conservatively — only known structured fields are patched; everything else
// passes through verbatim so downstream V1-field detection + schema validation
// still fire on whatever else may be wrong.
//
// Returns (raw, [list of repaired paths], true) on at least one
// repair, or (raw, nil, false) when nothing was repaired.
//
// Paths use dotted-bracket notation so the WARN log can name the
// exact site (e.g. blocks[0].items, blocks[2].claim_uses).
func repairNestedAnswerBlockFields(raw json.RawMessage) (json.RawMessage, []string, bool) {
	if len(raw) == 0 {
		return nil, nil, false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, nil, false
	}
	rawBlocks, ok := probe["blocks"]
	if !ok || len(rawBlocks) == 0 || rawBlocks[0] != '[' {
		return nil, nil, false
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(rawBlocks, &blocks); err != nil {
		return nil, nil, false
	}
	var paths []string
	repaired := false
	for i, blk := range blocks {
		var blkObj map[string]json.RawMessage
		if err := json.Unmarshal(blk, &blkObj); err != nil {
			continue
		}
		if fields, ok := repairMisplacedDiagramBlockFields(blkObj); ok {
			for _, field := range fields {
				paths = append(paths, fmt.Sprintf("blocks[%d].diagram.%s->blocks[%d].%s", i, field, i, field))
			}
			repaired = true
		}
		for _, field := range answerBlockArrayFieldNames {
			if r, ok := repairBlockArrayField(blkObj, field); ok {
				blkObj[field] = r
				paths = append(paths, fmt.Sprintf("blocks[%d].%s", i, field))
				repaired = true
			}
		}
		for _, field := range answerBlockObjectFieldNames {
			if r, ok := repairObjectField(blkObj, field); ok {
				blkObj[field] = r
				paths = append(paths, fmt.Sprintf("blocks[%d].%s", i, field))
				repaired = true
			}
		}
		if fields, ok := repairAnswerBlockItemScalarFragments(blkObj); ok {
			for _, field := range fields {
				paths = append(paths, fmt.Sprintf("blocks[%d].%s", i, field))
			}
			repaired = true
		}
		if fields, ok := repairMisplacedItemClaimUses(blkObj); ok {
			for _, field := range fields {
				paths = append(paths, fmt.Sprintf("blocks[%d].%s", i, field))
			}
			repaired = true
		}
		if fields, ok := repairAnswerBlockItemArrayFields(blkObj); ok {
			for _, field := range fields {
				paths = append(paths, fmt.Sprintf("blocks[%d].%s", i, field))
			}
			repaired = true
		}
		if fields, ok := repairAnswerBlockItemTextAliases(blkObj); ok {
			for _, field := range fields {
				paths = append(paths, fmt.Sprintf("blocks[%d].%s", i, field))
			}
			repaired = true
		}
		if fields, ok := repairAnswerBlockAnnotationShape(blkObj); ok {
			for _, field := range fields {
				paths = append(paths, fmt.Sprintf("blocks[%d].%s", i, field))
			}
			repaired = true
		}
		if patched, err := json.Marshal(blkObj); err == nil {
			blocks[i] = patched
		}
	}
	if !repaired {
		return raw, nil, false
	}
	if patchedBlocks, err := json.Marshal(blocks); err == nil {
		probe["blocks"] = patchedBlocks
	}
	out, err := json.Marshal(probe)
	if err != nil {
		return raw, nil, false
	}
	return out, paths, true
}

func repairNestedArraysAsString(raw json.RawMessage) (json.RawMessage, []string, bool) {
	return repairNestedAnswerBlockFields(raw)
}

func repairOrphanAnswerBlockAnnotations(raw json.RawMessage) (json.RawMessage, []string, bool) {
	if len(raw) == 0 {
		return nil, nil, false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, nil, false
	}
	rawBlocks, ok := probe["blocks"]
	if !ok || len(rawBlocks) == 0 || rawBlocks[0] != '[' {
		return nil, nil, false
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(rawBlocks, &blocks); err != nil || len(blocks) == 0 {
		return nil, nil, false
	}
	out := make([]json.RawMessage, 0, len(blocks))
	var paths []string
	for i, rawBlock := range blocks {
		var orphan map[string]json.RawMessage
		if err := json.Unmarshal(rawBlock, &orphan); err != nil || !answerBlockObjectIsAnnotationOnlyOrphan(orphan) || len(out) == 0 {
			out = append(out, rawBlock)
			continue
		}
		var target map[string]json.RawMessage
		if err := json.Unmarshal(out[len(out)-1], &target); err != nil || !answerBlockObjectHasIdentity(target) {
			out = append(out, rawBlock)
			continue
		}
		fields, ok := mergeAnswerBlockAnnotationOrphan(target, orphan)
		if !ok || len(fields) == 0 {
			out = append(out, rawBlock)
			continue
		}
		patched, err := json.Marshal(target)
		if err != nil {
			out = append(out, rawBlock)
			continue
		}
		out[len(out)-1] = patched
		for _, field := range fields {
			paths = append(paths, fmt.Sprintf("blocks[%d].%s->blocks[%d].%s", i, field, len(out)-1, field))
		}
	}
	if len(paths) == 0 || len(out) == len(blocks) {
		return raw, nil, false
	}
	patchedBlocks, err := json.Marshal(out)
	if err != nil {
		return raw, nil, false
	}
	probe["blocks"] = patchedBlocks
	patched, err := json.Marshal(probe)
	if err != nil {
		return raw, nil, false
	}
	return patched, paths, true
}

func answerBlockObjectIsAnnotationOnlyOrphan(block map[string]json.RawMessage) bool {
	if len(block) == 0 {
		return false
	}
	if rawNonEmptyJSONString(block["id"]) || rawNonEmptyJSONString(block["kind"]) {
		return false
	}
	found := false
	for field, raw := range block {
		switch field {
		case "claim_uses", "edge_anchors", "facet_ids", "surface_role":
			if len(bytes.TrimSpace(raw)) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				found = true
			}
		default:
			return false
		}
	}
	return found
}

func answerBlockObjectHasIdentity(block map[string]json.RawMessage) bool {
	return rawNonEmptyJSONString(block["id"]) && rawNonEmptyJSONString(block["kind"])
}

func rawNonEmptyJSONString(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	return strings.TrimSpace(value) != ""
}

func mergeAnswerBlockAnnotationOrphan(target, orphan map[string]json.RawMessage) ([]string, bool) {
	var fields []string
	for _, field := range []string{"claim_uses", "edge_anchors", "facet_ids"} {
		raw, ok := orphan[field]
		if !ok {
			continue
		}
		merged, changed, ok := mergeRawJSONArrayField(target[field], raw)
		if !ok {
			return nil, false
		}
		if changed {
			target[field] = merged
			fields = append(fields, field)
		}
	}
	if raw, ok := orphan["surface_role"]; ok {
		changed, ok := mergeRawJSONStringField(target, "surface_role", raw)
		if !ok {
			return nil, false
		}
		if changed {
			fields = append(fields, "surface_role")
		}
	}
	return fields, true
}

func mergeRawJSONArrayField(existing, incoming json.RawMessage) (json.RawMessage, bool, bool) {
	incoming = bytes.TrimSpace(incoming)
	if len(incoming) == 0 || bytes.Equal(incoming, []byte("null")) {
		return existing, false, true
	}
	var incomingItems []json.RawMessage
	if err := json.Unmarshal(incoming, &incomingItems); err != nil {
		return nil, false, false
	}
	if len(incomingItems) == 0 {
		return existing, false, true
	}
	existing = bytes.TrimSpace(existing)
	if len(existing) == 0 || bytes.Equal(existing, []byte("null")) {
		encoded, err := json.Marshal(incomingItems)
		return encoded, true, err == nil
	}
	var existingItems []json.RawMessage
	if err := json.Unmarshal(existing, &existingItems); err != nil {
		return nil, false, false
	}
	merged := append(append([]json.RawMessage(nil), existingItems...), incomingItems...)
	encoded, err := json.Marshal(merged)
	return encoded, true, err == nil
}

func mergeRawJSONStringField(target map[string]json.RawMessage, field string, incoming json.RawMessage) (bool, bool) {
	incoming = bytes.TrimSpace(incoming)
	if len(incoming) == 0 || bytes.Equal(incoming, []byte("null")) {
		return false, true
	}
	var incomingValue string
	if err := json.Unmarshal(incoming, &incomingValue); err != nil {
		return false, false
	}
	incomingValue = strings.TrimSpace(incomingValue)
	if incomingValue == "" {
		return false, true
	}
	existing, ok := target[field]
	existing = bytes.TrimSpace(existing)
	if !ok || len(existing) == 0 || bytes.Equal(existing, []byte("null")) {
		target[field] = mustMarshal(incomingValue)
		return true, true
	}
	var existingValue string
	if err := json.Unmarshal(existing, &existingValue); err != nil {
		return false, false
	}
	if strings.TrimSpace(existingValue) != incomingValue {
		return false, false
	}
	return false, true
}

// mergeWholeDocStringify is the Path B helper for
// repairStringWrappedArrayFields. It parses `wrapped` (a candidate
// `{"<key>": <trimmed>}` byte slice that, on success, supplies the
// array at `key` along with any sibling top-level fields that the
// LLM crammed inside the stringified blob), then merges the result
// back into the outer probe so the patch payload retains every
// originally-emitted top-level key.
//
// Returns (patched, true) when wrapped parses to a JSON object
// whose `key` field is a real array; (nil, false) otherwise. The
// merge rule: wrapped's keys win for `key` AND for any sibling key
// the outer probe did not already define; the outer probe wins on
// conflict for every other key (so non-target fields are not
// silently overridden by malformed inner content).
func mergeWholeDocStringify(probe map[string]json.RawMessage, key string, wrapped []byte) (map[string]json.RawMessage, bool) {
	var fullDoc map[string]json.RawMessage
	if err := json.Unmarshal(wrapped, &fullDoc); err != nil {
		return nil, false
	}
	rawArr, ok := fullDoc[key]
	if !ok || len(rawArr) == 0 || rawArr[0] != '[' {
		return nil, false
	}
	merged := make(map[string]json.RawMessage, len(probe)+len(fullDoc))
	for k, v := range probe {
		merged[k] = v
	}
	merged[key] = rawArr
	for k, v := range fullDoc {
		if k == key {
			continue
		}
		if _, exists := merged[k]; !exists {
			merged[k] = v
		}
	}
	return merged, true
}

// repairNestedArraysInPatch applies the same block nested-field repair that
// repairNestedAnswerBlockFields does for the full emit, but operates on the
// patch tool's `replace_blocks` AND `add_blocks` arrays (the emit's `blocks`
// is the singular equivalent).
func repairNestedArraysInPatch(raw json.RawMessage) (json.RawMessage, []string, bool) {
	if len(raw) == 0 {
		return nil, nil, false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, nil, false
	}
	var paths []string
	repairedAny := false
	for _, topField := range []string{"replace_blocks", "add_blocks"} {
		rawList, ok := probe[topField]
		if !ok || len(rawList) == 0 || rawList[0] != '[' {
			continue
		}
		var blocks []json.RawMessage
		if err := json.Unmarshal(rawList, &blocks); err != nil {
			continue
		}
		listChanged := false
		for i, blk := range blocks {
			var blkObj map[string]json.RawMessage
			if err := json.Unmarshal(blk, &blkObj); err != nil {
				continue
			}
			blockChanged := false
			if fields, ok := repairMisplacedDiagramBlockFields(blkObj); ok {
				for _, field := range fields {
					paths = append(paths, fmt.Sprintf("%s[%d].diagram.%s->%s[%d].%s", topField, i, field, topField, i, field))
				}
				blockChanged = true
			}
			for _, field := range answerBlockArrayFieldNames {
				if r, ok := repairBlockArrayField(blkObj, field); ok {
					blkObj[field] = r
					paths = append(paths, fmt.Sprintf("%s[%d].%s", topField, i, field))
					blockChanged = true
				}
			}
			for _, field := range answerBlockObjectFieldNames {
				if r, ok := repairObjectField(blkObj, field); ok {
					blkObj[field] = r
					paths = append(paths, fmt.Sprintf("%s[%d].%s", topField, i, field))
					blockChanged = true
				}
			}
			if fields, ok := repairAnswerBlockItemScalarFragments(blkObj); ok {
				for _, field := range fields {
					paths = append(paths, fmt.Sprintf("%s[%d].%s", topField, i, field))
				}
				blockChanged = true
			}
			if fields, ok := repairMisplacedItemClaimUses(blkObj); ok {
				for _, field := range fields {
					paths = append(paths, fmt.Sprintf("%s[%d].%s", topField, i, field))
				}
				blockChanged = true
			}
			if fields, ok := repairAnswerBlockItemArrayFields(blkObj); ok {
				for _, field := range fields {
					paths = append(paths, fmt.Sprintf("%s[%d].%s", topField, i, field))
				}
				blockChanged = true
			}
			if fields, ok := repairAnswerBlockItemTextAliases(blkObj); ok {
				for _, field := range fields {
					paths = append(paths, fmt.Sprintf("%s[%d].%s", topField, i, field))
				}
				blockChanged = true
			}
			if fields, ok := repairAnswerBlockAnnotationShape(blkObj); ok {
				for _, field := range fields {
					paths = append(paths, fmt.Sprintf("%s[%d].%s", topField, i, field))
				}
				blockChanged = true
			}
			if blockChanged {
				if patched, err := json.Marshal(blkObj); err == nil {
					blocks[i] = patched
					listChanged = true
				}
			}
		}
		if listChanged {
			if patched, err := json.Marshal(blocks); err == nil {
				probe[topField] = patched
				repairedAny = true
			}
		}
	}
	if !repairedAny {
		return raw, nil, false
	}
	out, err := json.Marshal(probe)
	if err != nil {
		return raw, nil, false
	}
	return out, paths, true
}

var answerBlockArrayFieldNames = []string{"items", "columns", "claim_uses", "edge_anchors", "facet_ids"}
var answerBlockObjectFieldNames = []string{"diagram"}
var answerBlockFieldsAllowedFromDiagram = []string{"claim_uses", "edge_anchors", "facet_ids", "surface_role"}

// repairMisplacedItemClaimUses hoists item-level claim_use(s) that local
// models sometimes emit despite the schema's block-level-only contract. This
// is a structural relocation: claim annotations are not interpreted here, only
// moved to the block's claim_uses[] pool before the normal annotation-shape
// repair canonicalizes aliases such as claimForm/facetId.
func repairMisplacedItemClaimUses(blkObj map[string]json.RawMessage) ([]string, bool) {
	if len(blkObj) == 0 {
		return nil, false
	}
	rawItems, ok := blkObj["items"]
	rawItems = bytes.TrimSpace(rawItems)
	if !ok || len(rawItems) == 0 || rawItems[0] != '[' {
		return nil, false
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(rawItems, &items); err != nil || len(items) == 0 {
		return nil, false
	}
	existing, ok := decodeClaimUseArrayRaw(blkObj["claim_uses"])
	if !ok {
		return nil, false
	}
	seen := make(map[string]bool, len(existing))
	for _, raw := range existing {
		seen[canonicalRawObjectKey(raw)] = true
	}
	changed := false
	for i := range items {
		for _, field := range []string{"claim_uses", "claim_use"} {
			raw, exists := items[i][field]
			if !exists {
				continue
			}
			claims, parsed := decodeItemClaimUseRaw(raw)
			if !parsed {
				continue
			}
			for _, claim := range claims {
				key := canonicalRawObjectKey(claim)
				if seen[key] {
					continue
				}
				seen[key] = true
				existing = append(existing, claim)
			}
			delete(items[i], field)
			changed = true
		}
	}
	if !changed {
		return nil, false
	}
	blkObj["items"] = mustMarshal(items)
	blkObj["claim_uses"] = mustMarshal(existing)
	return []string{"items[].claim_use(s)->claim_uses"}, true
}

func repairAnswerBlockItemArrayFields(blkObj map[string]json.RawMessage) ([]string, bool) {
	if len(blkObj) == 0 {
		return nil, false
	}
	rawItems, ok := blkObj["items"]
	rawItems = bytes.TrimSpace(rawItems)
	if !ok || len(rawItems) == 0 || rawItems[0] != '[' {
		return nil, false
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(rawItems, &items); err != nil || len(items) == 0 {
		return nil, false
	}
	var paths []string
	changed := false
	for i := range items {
		if r, ok := repairBlockArrayField(items[i], "cells"); ok {
			items[i]["cells"] = r
			paths = append(paths, fmt.Sprintf("items[%d].cells", i))
			changed = true
		}
	}
	if !changed {
		return nil, false
	}
	blkObj["items"] = mustMarshal(items)
	return paths, true
}

var answerBlockItemKnownFieldNames = map[string]bool{
	"id":             true,
	"label":          true,
	"text":           true,
	"cells":          true,
	"candidate_role": true,
	"citation_ref":   true,
}

// repairAnswerBlockItemTextAliases preserves user-visible item prose when a
// local model emits exactly one schema-unknown string field on an item that has
// no text/cells carrier. The field name itself is intentionally not interpreted:
// the safe structural signal is "one non-empty string payload would otherwise
// be quarantined and the row would render empty".
func repairAnswerBlockItemTextAliases(blkObj map[string]json.RawMessage) ([]string, bool) {
	if len(blkObj) == 0 {
		return nil, false
	}
	rawItems, ok := blkObj["items"]
	rawItems = bytes.TrimSpace(rawItems)
	if !ok || len(rawItems) == 0 || rawItems[0] != '[' {
		return nil, false
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(rawItems, &items); err != nil || len(items) == 0 {
		return nil, false
	}
	var paths []string
	changed := false
	for i := range items {
		if jsonStringFieldNonEmpty(items[i]["text"]) || jsonStringArrayFieldNonEmpty(items[i]["cells"]) {
			continue
		}
		aliasField := ""
		aliasText := ""
		ambiguous := false
		for field, raw := range items[i] {
			if answerBlockItemKnownFieldNames[field] {
				continue
			}
			text, ok := jsonStringFieldValue(raw)
			if !ok || strings.TrimSpace(text) == "" {
				continue
			}
			if aliasField != "" {
				ambiguous = true
				break
			}
			aliasField = field
			aliasText = strings.TrimSpace(text)
		}
		if aliasField == "" || ambiguous {
			continue
		}
		items[i]["text"] = mustMarshal(aliasText)
		delete(items[i], aliasField)
		paths = append(paths, fmt.Sprintf("items[%d].%s->text", i, aliasField))
		changed = true
	}
	if !changed {
		return nil, false
	}
	blkObj["items"] = mustMarshal(items)
	return paths, true
}

// repairAnswerBlockItemScalarFragments handles a narrow local-model
// corruption mode inside block.items[]:
//   - a complete item object is JSON-encoded as a string; or
//   - a display-rich block already has block.text OR display-rich
//     item objects, and the items[] array contains stray scalar
//     fragments around at least one valid object.
//
// The first case is lossless. The second case is intentionally gated:
// visible answer content must already be preserved by block.text or
// by kept item object(s), and any scalar being discarded must look like
// structural JSON/citation sidecar noise rather than standalone prose.
// This accepts local-model stream fragments without letting the system
// silently delete user-visible answer text.
func repairAnswerBlockItemScalarFragments(blkObj map[string]json.RawMessage) ([]string, bool) {
	if len(blkObj) == 0 {
		return nil, false
	}
	rawItems, ok := blkObj["items"]
	rawItems = bytes.TrimSpace(rawItems)
	if !ok || len(rawItems) == 0 || rawItems[0] != '[' {
		return nil, false
	}
	var rawList []json.RawMessage
	if err := json.Unmarshal(rawItems, &rawList); err != nil || len(rawList) == 0 {
		return nil, false
	}
	hasVisibleText := jsonStringFieldNonEmpty(blkObj["text"])
	kept := make([]json.RawMessage, 0, len(rawList))
	changed := false
	invalidScalar := false
	nonIgnorableScalar := false
	for _, raw := range rawList {
		trimmedRaw := bytes.TrimSpace(raw)
		if len(trimmedRaw) == 0 || bytes.Equal(trimmedRaw, []byte("null")) {
			invalidScalar = true
			changed = true
			continue
		}
		switch trimmedRaw[0] {
		case '{':
			kept = append(kept, trimmedRaw)
		case '"':
			var encoded string
			if err := json.Unmarshal(trimmedRaw, &encoded); err != nil {
				return nil, false
			}
			inner := strings.TrimSpace(encoded)
			if strings.HasPrefix(inner, "{") {
				var item json.RawMessage
				if err := json.Unmarshal([]byte(inner), &item); err == nil {
					kept = append(kept, item)
					changed = true
					continue
				}
				if normalised, normChanged := normalizeControlCharsInJSONStrings(inner); normChanged {
					if err := json.Unmarshal([]byte(normalised), &item); err == nil {
						kept = append(kept, item)
						changed = true
						continue
					}
				}
			}
			invalidScalar = true
			if !isIgnorableAnswerItemScalarFragment(inner) {
				nonIgnorableScalar = true
			}
			changed = true
		default:
			invalidScalar = true
			nonIgnorableScalar = true
			changed = true
		}
	}
	if !changed || len(kept) == 0 {
		return nil, false
	}
	if invalidScalar && !hasVisibleText {
		if nonIgnorableScalar || !answerBlockItemObjectsPreserveVisibleDisplay(kept) {
			return nil, false
		}
	}
	blkObj["items"] = mustMarshal(kept)
	return []string{"items[] scalar fragments"}, true
}

func answerBlockItemObjectsPreserveVisibleDisplay(items []json.RawMessage) bool {
	for _, raw := range items {
		var item map[string]json.RawMessage
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		if jsonStringFieldNonEmpty(item["text"]) || jsonStringFieldNonEmpty(item["label"]) || jsonStringArrayFieldNonEmpty(item["cells"]) {
			return true
		}
	}
	return false
}

func isIgnorableAnswerItemScalarFragment(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if isJSONPunctuationOnlyFragment(s) {
		return true
	}
	normalized := strings.ToLower(s)
	for _, old := range []string{"\"", "'", "{", "}", "[", "]", ",", " ", "\t", "\r", "\n"} {
		normalized = strings.ReplaceAll(normalized, old, "")
	}
	normalized = strings.TrimSpace(normalized)
	if strings.HasPrefix(normalized, "citation_ref:") || strings.HasPrefix(normalized, "citation_ref=") {
		rest := normalized[len("citation_ref:"):]
		if strings.HasPrefix(normalized, "citation_ref=") {
			rest = normalized[len("citation_ref="):]
		}
		if rest == "" {
			return true
		}
		if rest[0] == '-' || rest[0] == '+' {
			rest = rest[1:]
		}
		if rest == "" {
			return false
		}
		for _, r := range rest {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	return false
}

func isJSONPunctuationOnlyFragment(s string) bool {
	for _, r := range s {
		switch r {
		case ' ', '\t', '\r', '\n', '{', '}', '[', ']', '(', ')', ',', ':', ';', '"', '\'', '\\':
			continue
		default:
			return false
		}
	}
	return true
}

func jsonStringFieldNonEmpty(raw json.RawMessage) bool {
	text, ok := jsonStringFieldValue(raw)
	return ok && strings.TrimSpace(text) != ""
}

func jsonStringFieldValue(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '"' {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func jsonStringArrayFieldNonEmpty(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return false
	}
	var cells []string
	if err := json.Unmarshal(raw, &cells); err != nil {
		return false
	}
	for _, cell := range cells {
		if strings.TrimSpace(cell) != "" {
			return true
		}
	}
	return false
}

func decodeClaimUseArrayRaw(raw json.RawMessage) ([]json.RawMessage, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, true
	}
	if raw[0] == '"' {
		if repaired, ok := repairBlockArrayField(map[string]json.RawMessage{"claim_uses": raw}, "claim_uses"); ok {
			raw = repaired
		}
	}
	if len(raw) == 0 || raw[0] != '[' {
		return nil, false
	}
	var out []json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false
	}
	return out, true
}

func decodeItemClaimUseRaw(raw json.RawMessage) ([]json.RawMessage, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, false
	}
	if raw[0] == '"' {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return nil, false
		}
		raw = json.RawMessage(strings.TrimSpace(encoded))
	}
	switch raw[0] {
	case '[':
		var out []json.RawMessage
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, false
		}
		return out, true
	case '{':
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(raw, &probe); err != nil {
			return nil, false
		}
		return []json.RawMessage{mustMarshal(probe)}, true
	default:
		return nil, false
	}
}

func canonicalRawObjectKey(raw json.RawMessage) string {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		if b, marshalErr := json.Marshal(obj); marshalErr == nil {
			return string(b)
		}
	}
	return string(bytes.TrimSpace(raw))
}

// repairMisplacedDiagramBlockFields moves known block-level annotation fields
// that a local model placed inside the sibling diagram object back onto the
// block. This is a structural relocation only: the exact JSON value is
// preserved, conflicts are left untouched for the strict decoder, and shape
// validation still runs after the move.
func repairMisplacedDiagramBlockFields(blkObj map[string]json.RawMessage) ([]string, bool) {
	if len(blkObj) == 0 {
		return nil, false
	}
	rawDiagram, ok := blkObj["diagram"]
	rawDiagram = bytes.TrimSpace(rawDiagram)
	if !ok || len(rawDiagram) == 0 || rawDiagram[0] != '{' {
		return nil, false
	}
	var diagram map[string]json.RawMessage
	if err := json.Unmarshal(rawDiagram, &diagram); err != nil || len(diagram) == 0 {
		return nil, false
	}
	var moved []string
	for _, field := range answerBlockFieldsAllowedFromDiagram {
		rawField, ok := diagram[field]
		if !ok || len(rawField) == 0 {
			continue
		}
		if existing, exists := blkObj[field]; exists {
			if bytes.Equal(bytes.TrimSpace(existing), bytes.TrimSpace(rawField)) {
				delete(diagram, field)
				moved = append(moved, field)
			}
			continue
		}
		if !misplacedDiagramFieldShapeOK(field, rawField) {
			continue
		}
		blkObj[field] = rawField
		delete(diagram, field)
		moved = append(moved, field)
	}
	if len(moved) == 0 {
		return nil, false
	}
	blkObj["diagram"] = mustMarshal(diagram)
	return moved, true
}

func misplacedDiagramFieldShapeOK(field string, raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return false
	}
	switch field {
	case "claim_uses", "edge_anchors", "facet_ids":
		return raw[0] == '[' || raw[0] == '"' ||
			(raw[0] == '{' && answerBlockArrayFieldAcceptsSingletonObject(field))
	case "surface_role":
		return raw[0] == '"'
	default:
		return false
	}
}

// repairBlockArrayField detects uniquely recoverable array-shape
// mistakes at obj[field] and returns the re-encoded array RawMessage:
// JSON-encoded arrays, JSON-encoded singleton objects for object-array
// fields, raw singleton objects for object-array fields, and raw singleton
// strings for string-array fields. Conservative: any ambiguous shape is left
// untouched so the regular validator surfaces the real schema violation.
func repairBlockArrayField(obj map[string]json.RawMessage, field string) (json.RawMessage, bool) {
	rawField, ok := obj[field]
	rawField = bytes.TrimSpace(rawField)
	if !ok || len(rawField) == 0 {
		return nil, false
	}
	switch rawField[0] {
	case '[':
		return nil, false
	case '{':
		if answerBlockArrayFieldAcceptsSingletonObject(field) {
			var item json.RawMessage
			if err := json.Unmarshal(rawField, &item); err == nil {
				return mustMarshal([]json.RawMessage{item}), true
			}
		}
		return nil, false
	case '"':
		var encoded string
		if err := json.Unmarshal(rawField, &encoded); err != nil {
			return nil, false
		}
		trimmed := strings.TrimSpace(encoded)
		if strings.HasPrefix(trimmed, "[") {
			var inner []json.RawMessage
			if err := json.Unmarshal([]byte(trimmed), &inner); err == nil {
				return mustMarshal(inner), true
			}
			if trimmedArray, ok := trimRedundantTrailingClosersAfterJSONArray(trimmed); ok {
				if err := json.Unmarshal([]byte(trimmedArray), &inner); err == nil {
					return mustMarshal(inner), true
				}
			}
			if normalised, changed := normalizeControlCharsInJSONStrings(trimmed); changed {
				if err := json.Unmarshal([]byte(normalised), &inner); err == nil {
					return mustMarshal(inner), true
				}
				if trimmedArray, ok := trimRedundantTrailingClosersAfterJSONArray(normalised); ok {
					if err := json.Unmarshal([]byte(trimmedArray), &inner); err == nil {
						return mustMarshal(inner), true
					}
				}
			}
		}
		if strings.HasPrefix(trimmed, "{") && answerBlockArrayFieldAcceptsSingletonObject(field) {
			var item json.RawMessage
			if err := json.Unmarshal([]byte(trimmed), &item); err == nil {
				return mustMarshal([]json.RawMessage{item}), true
			}
			if normalised, changed := normalizeControlCharsInJSONStrings(trimmed); changed {
				if err := json.Unmarshal([]byte(normalised), &item); err == nil {
					return mustMarshal([]json.RawMessage{item}), true
				}
			}
		}
		if answerBlockArrayFieldAcceptsSingletonString(field) && trimmed != "" {
			return mustMarshal([]string{trimmed}), true
		}
		return nil, false
	default:
		return nil, false
	}
}

func answerBlockArrayFieldAcceptsSingletonObject(field string) bool {
	switch field {
	case "items", "claim_uses", "edge_anchors":
		return true
	default:
		return false
	}
}

func answerBlockArrayFieldAcceptsSingletonString(field string) bool {
	switch field {
	case "columns", "facet_ids":
		return true
	default:
		return false
	}
}

// repairObjectField is the object-shaped sibling of repairBlockArrayField. It
// is shared by top-level exact_resolution repair and block-level diagram
// repair. Only a successfully parsed JSON object is accepted.
func repairObjectField(obj map[string]json.RawMessage, field string) (json.RawMessage, bool) {
	rawField, ok := obj[field]
	if !ok || len(rawField) == 0 {
		return nil, false
	}
	if rawField[0] != '"' {
		return nil, false
	}
	var encoded string
	if err := json.Unmarshal(rawField, &encoded); err != nil {
		return nil, false
	}
	trimmed := strings.TrimSpace(encoded)
	if !strings.HasPrefix(trimmed, "{") {
		return nil, false
	}
	var inner map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &inner); err == nil {
		return mustMarshal(inner), true
	}
	if trimmedObject, ok := trimRedundantTrailingClosersAfterJSONObject(trimmed); ok {
		if err := json.Unmarshal([]byte(trimmedObject), &inner); err == nil {
			return mustMarshal(inner), true
		}
	}
	if normalised, changed := normalizeControlCharsInJSONStrings(trimmed); changed {
		if err := json.Unmarshal([]byte(normalised), &inner); err == nil {
			return mustMarshal(inner), true
		}
		if trimmedObject, ok := trimRedundantTrailingClosersAfterJSONObject(normalised); ok {
			if err := json.Unmarshal([]byte(trimmedObject), &inner); err == nil {
				return mustMarshal(inner), true
			}
		}
	}
	return nil, false
}

// summarizeV2Blocks builds a short " (kinds=summary,ordered_list,…)"
// snippet for the tool result Summary. Helps operators eyeball the
// shape of the V2 emit without reading the full doc.
func summarizeV2Blocks(blocks []types.AnswerBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, string(b.Kind))
	}
	return " (kinds=" + strings.Join(parts, ",") + ")"
}

// answerDocumentV2MisplacedHints lists field-name patterns
// historically seen as `unknown field` strict-decode rejects in
// V2 emit calls — typically caused by the LLM placing a sibling
// field inside a nested object whose description grew large
// enough to look like a "metadata bag". The remap rewrites the
// strict-decode error to surface the correct paths so the next
// retry sees concrete relocation guidance instead of a bare
// field-name reject.
//
// Provenance:
//   - citation_ref → claim_use / claim_uses: u3a-1 forensic
//     2026-05-04 (7 retry iters before recovery). Phase 3-C3
//     added from_node/to_node to claim_use; LLMs increasingly
//     guessed citation_ref into the same nested object.
//
// Adding new entries: confirm via grep + real-eval log that the
// LLM mistake is recurring (not a one-off), then add the pattern
// here and a regression test.
var answerDocumentV2MisplacedHints = []MisplacedFieldHint{
	{
		Field:          "citation_ref",
		ContainerNames: []string{"claim_uses"},
		CorrectPaths: []string{
			"items[i].citation_ref (anchor scalar / decision blocks via a one-element items=[{citation_ref:N}])",
		},
	},
	{
		Field:          "facet_ids",
		ContainerNames: []string{"claim_uses"},
		CorrectPaths: []string{
			"blocks[i].facet_ids (plural, block-level coverage list)",
			"blocks[i].claim_uses[j].facet_id (singular, block-level claim annotation)",
		},
	},
	// V1 carrier residue: scalar blocks used to carry value{literal,
	// citation_ref}, decision blocks used to carry boolean{decision,
	// rationale, citation_ref}. V2 carrier puts the literal / verdict
	// in block.text and the citation on a one-element items[].
	// LLMs trained on the old shape may still emit value / boolean
	// at block top level; the remap directs them to the V2 location.
	{
		Field:          "value",
		ContainerNames: []string{"block (top-level)"},
		CorrectPaths: []string{
			"blocks[i].text (put the literal here directly, e.g. text=\"42\")",
			"blocks[i].items=[{id:\"v\", citation_ref: N}] (anchor citation here)",
		},
	},
	{
		Field:          "boolean",
		ContainerNames: []string{"block (top-level)"},
		CorrectPaths: []string{
			"blocks[i].text (put the verdict + rationale here directly, e.g. text=\"yes — the function returns ...\")",
			"blocks[i].items=[{id:\"d\", citation_ref: N}] (anchor citation here)",
		},
	},
	{
		Field:          "literal",
		ContainerNames: []string{"value (the value field is rejected; this targets the nested literal)"},
		CorrectPaths: []string{
			"blocks[i].text (the literal goes directly in block.text on a scalar block)",
		},
	},
	{
		Field:          "decision",
		ContainerNames: []string{"boolean (the boolean field is rejected; this targets the nested decision)"},
		CorrectPaths: []string{
			"blocks[i].text (the verdict and rationale go directly in block.text on a decision block)",
		},
	},
	{
		Field:          "rationale",
		ContainerNames: []string{"boolean (the boolean field is rejected; this targets the nested rationale)"},
		CorrectPaths: []string{
			"blocks[i].text (the verdict and rationale go directly in block.text on a decision block)",
		},
	},
	// Phase 1-B source-fix follow-on (2026-05-04): from_node /
	// to_node previously lived inside RenderedClaimUse and were
	// removed for source-level information-density reduction.
	// LLMs trained on the old shape may still place them inside
	// claim_use; remap to the new edge_anchors[] location.
	{
		Field:          "from_node",
		ContainerNames: []string{"claim_uses"},
		CorrectPaths: []string{
			"blocks[i].edge_anchors[j].from_node",
		},
	},
	{
		Field:          "to_node",
		ContainerNames: []string{"claim_uses"},
		CorrectPaths: []string{
			"blocks[i].edge_anchors[j].to_node",
		},
	},
	// G3 (post_v2_runtime_gap_remediation, 2026-05-04). relation_kind
	// is typed-only on EdgeAnchor; LLMs that have not yet learned the
	// new schema may misplace it inside claim_use (alongside its
	// neighbour fields from_node / to_node).
	{
		Field:          "relation_kind",
		ContainerNames: []string{"claim_uses"},
		CorrectPaths: []string{
			"blocks[i].edge_anchors[j].relation_kind",
		},
	},
}

// enrichCitationsWithEnclosingFunction populates each Citation's
// EnclosingFunction field by looking up the cite's (File, Line) in
// the repo graph's per-file Symbol table. The graph is fetched from
// MutableState.SearchGraph (set once per Run by the explorer's
// pre-scan). Only ScopeLine / ScopeLineRange / ScopeSection citations
// trigger the lookup — File / Crossfile / Negative citations carry
// their own context fields and do not need a per-line enrichment.
//
// Idempotent on EnclosingFunction already populated. Returns the
// input slice mutated in place (caller still receives the slice
// header for assignment readability). Graceful degradation: any
// missing graph / file / symbol bracket leaves the field empty —
// the renderer treats empty as "no enclosing function known" and
// emits the cite without the (in `<fn>`) suffix.
func enrichCitationsWithEnclosingFunction(citations []types.Citation, ctx *types.BusContext) []types.Citation {
	if len(citations) == 0 || ctx == nil || ctx.Mutable == nil {
		return citations
	}
	rawGraph := ctx.Mutable.SearchGraph()
	if rawGraph == nil {
		return citations
	}
	graph, ok := rawGraph.(*repotypes.Graph)
	if !ok || graph == nil {
		return citations
	}
	for i := range citations {
		c := &citations[i]
		if c.EnclosingFunction != "" {
			continue
		}
		switch c.Scope {
		case types.ScopeFile, types.ScopeCrossfile, types.ScopeNegative:
			continue
		}
		if c.File == "" || c.Line <= 0 {
			continue
		}
		fi, ok := graph.FileIndex[c.File]
		if !ok || fi == nil {
			continue
		}
		c.EnclosingFunction = lookupEnclosingFunction(fi.Symbols, c.Line)
	}
	return citations
}

// lookupEnclosingFunction walks the file's Symbols and returns the
// callable identifier whose body brackets the given line. Prefers
// the smallest enclosing range (innermost function for nested
// callables), and formats methods as `Receiver.Name` so the cite
// reader sees the type-qualified handle.
//
// Empty when no callable Symbol brackets the line — module-level
// declarations, imports, comments, struct-literal lines outside
// any function. Language-agnostic: relies on Symbol.Kind values
// the per-language extractor populates ("function" / "method" are
// the universal kinds; other languages map their concept onto these
// in the repomap layer).
func lookupEnclosingFunction(symbols []repotypes.Symbol, line int) string {
	if line <= 0 {
		return ""
	}
	bestStart := 0
	var best string
	for _, sym := range symbols {
		if sym.Kind != "function" && sym.Kind != "method" {
			continue
		}
		if sym.Line <= 0 || sym.EndLine <= 0 {
			continue
		}
		if sym.Line > line || sym.EndLine < line {
			continue
		}
		if sym.Line <= bestStart && best != "" {
			continue
		}
		bestStart = sym.Line
		if sym.Receiver != "" {
			best = sym.Receiver + "." + sym.Name
		} else {
			best = sym.Name
		}
	}
	return best
}
