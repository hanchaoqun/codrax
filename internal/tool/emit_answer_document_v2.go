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
	Citations             []types.Citation                   `json:"citations,omitempty"`
	ExactResolution       *types.AnswerExactResolution       `json:"exact_resolution,omitempty"`
	MissingRequestedRoles []types.AnswerMissingRequestedRole `json:"missing_requested_roles,omitempty"`
	Caveats               []string                           `json:"caveats,omitempty"`
	Snippets              []types.CodeSnippet                `json:"snippets,omitempty"`
}

type emitAnswerBlockV2 struct {
	ID          string                    `json:"id"`
	Kind        string                    `json:"kind"`
	Title       string                    `json:"title,omitempty"`
	Text        string                    `json:"text,omitempty"`
	Items       []emitAnswerBlockItemV2   `json:"items,omitempty"`
	Diagram     *emitAnswerDiagramV2      `json:"diagram,omitempty"`
	ClaimUses   []types.RenderedClaimUse  `json:"claim_uses,omitempty"`
	EdgeAnchors []types.DiagramEdgeAnchor `json:"edge_anchors,omitempty"`
	FacetIDs    []string                  `json:"facet_ids,omitempty"`
	SurfaceRole string                    `json:"surface_role,omitempty"`
}

type emitAnswerBlockItemV2 struct {
	ID          string  `json:"id,omitempty"`
	Label       string  `json:"label,omitempty"`
	Text        string  `json:"text,omitempty"`
	CitationRef FlexInt `json:"citation_ref,omitempty"`
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
	if repaired, paths, ok := repairNestedArraysAsString(raw); ok {
		logging.Warning("[emit_answer_document] nested arrays arrived as JSON-encoded strings (paths: %s); re-parsed via flat-mode tolerance path",
			strings.Join(paths, ", "))
		raw = repaired
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var p emitAnswerDocumentV2Params
	if err := dec.Decode(&p); err != nil {
		err = RemapStrictDecodeError(err, answerDocumentV2MisplacedHints)
		persistRecoveredAnswerDisplayAttachments(ctx, recovery)
		return failEmit(toolName, now, "invalid params: %v", err)
	}

	// document_model is no longer surfaced to the LLM. The system
	// runs only one carrier so any value (or absence) the model
	// happens to emit is silently accepted — the dispatcher always
	// routes to this executor and the typed model is hardcoded
	// downstream. Future migration to a second carrier would
	// re-introduce validation here.
	if len(p.Blocks) == 0 {
		persistRecoveredAnswerDisplayAttachments(ctx, recovery)
		return failEmit(toolName, now,
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
		Citations:             enrichCitationsWithEnclosingFunction(p.Citations, ctx),
		ExactResolution:       p.ExactResolution,
		MissingRequestedRoles: p.MissingRequestedRoles,
		Caveats:               p.Caveats,
		Snippets:              p.Snippets,
	}
	for i, raw := range p.Blocks {
		blk, err := NormalizeEmitAnswerBlock(raw, fmt.Sprintf("blocks[%d]", i))
		if err != nil {
			persistRecoveredAnswerDisplayAttachments(ctx, recovery)
			return failEmit(toolName, now, "%s", err.Error())
		}
		doc.Blocks = append(doc.Blocks, blk)
	}
	canonicalizeSummaryLeadBlock(doc)

	// P1 (2026-05-10) — emit-time pre-validation chokepoint.
	// Before we persist the doc, run the four STRICT structural
	// checks (block coverage / principal claim_use / uncertainty
	// block / facet coverage). When the check fails, the LLM
	// retries WITHIN THE SAME tool dispatch (BaseAgent's
	// emitAnswerDocumentRejectSignal captures !LastToolResult.Success
	// and re-prompts) — saves a full orchestrator-level repair
	// round (~3-5min wall-clock).
	//
	// View-aware: when no view is available (single-shot tests,
	// pre-AnalysisIR paths) the pre-check returns nil and the
	// post-emit chain in internal/orchestrator runs unchanged.
	if view := types.BuildAnswerSemanticViewForBusContext(ctx); view != nil {
		if hints := runPreEmitChecks(doc, view, preEmitOracleFromCtx(ctx), ctx); len(hints) > 0 {
			persistRecoveredAnswerDisplayAttachments(ctx, recovery)
			return failEmit(toolName, now, "%s", formatEmitFixHints(hints))
		}
	}
	if hints := preCheckModelSurfaceTerms(doc, ctx); len(hints) > 0 {
		persistRecoveredAnswerDisplayAttachments(ctx, recovery)
		return failEmit(toolName, now, "%s", formatEmitFixHints(hints))
	}

	// v3 B4 (2026-05-04): route the full-emit write through the
	// unified mutation runtime — same chokepoint as the patch path,
	// merged-doc validation + persist + telemetry shared.
	mutation := types.NewReplaceAllMutation(doc)
	res, err := ApplyAndPersistMutation(ctx, toolName, mutation, nil, now)
	if err == nil && res.Success && ctx != nil && ctx.Mutable != nil {
		ctx.Mutable.SetAnswerDisplayAttachments(recovery.Attachments)
		if len(recovery.Attachments) > 0 {
			logging.Warning("[emit_answer_document] preserved %d recovered display attachment(s) after %s recovery (candidate_blocks=%d recovered_blocks=%d)",
				len(recovery.Attachments), recovery.Mode, recovery.CandidateBlocks, recovery.RecoveredBlocks)
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

func persistRecoveredAnswerDisplayAttachments(ctx *types.BusContext, report answerDocumentRecoveryReport) {
	if ctx == nil || ctx.Mutable == nil || len(report.Attachments) == 0 {
		return
	}
	ctx.Mutable.SetAnswerDisplayAttachments(report.Attachments)
	logging.Warning("[emit_answer_document] preserved %d recovered display attachment(s) from rejected %s recovery",
		len(report.Attachments), report.Mode)
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
	// Path A: pure-array stringify. Repair just blocks; preserve all
	// other top-level keys verbatim.
	var inner []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &inner); err == nil {
		probe["blocks"] = mustMarshal(inner)
		if patched, err := json.Marshal(probe); err == nil {
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
	// Path C inner pass: the outer parse succeeded, but the decoded
	// inner string itself carries unescaped control chars (typical
	// when the LLM stringifies an array containing a multi-line
	// diagram body). Re-normalise the trimmed inner string and
	// retry both Path A and Path B.
	if normalised, changed := normalizeControlCharsInJSONStrings(trimmed); changed {
		var innerC []json.RawMessage
		if err := json.Unmarshal([]byte(normalised), &innerC); err == nil {
			probe["blocks"] = mustMarshal(innerC)
			if patched, err := json.Marshal(probe); err == nil {
				return patched, true
			}
		}
		if patched, ok := finishWholeDocumentStringRepair(probe, normalised); ok {
			return patched, true
		}
	}
	return nil, false
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
			if depth == 0 {
				elemStart = i
			}
			depth++
		case '}':
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
					elements = append(elements, json.RawMessage(candidate))
					if kind, ok := blockKindFromObject(probe); ok {
						recoveredKinds = append(recoveredKinds, kind)
					}
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
		Lossless:        len(candidateKinds) == len(recoveredKinds) && len(attachments) == 0,
		CandidateBlocks: len(candidateKinds),
		RecoveredBlocks: len(recoveredKinds),
		CandidateKinds:  candidateKinds,
		RecoveredKinds:  recoveredKinds,
		Attachments:     attachments,
	}
	report.DroppedVisiblePayload = !report.Lossless || len(attachments) > 0
	if len(candidateKinds) == 0 {
		report.CandidateBlocks = len(recoveredKinds)
		report.CandidateKinds = append([]types.AnswerBlockKind(nil), recoveredKinds...)
		report.Lossless = len(attachments) == 0
		report.DroppedVisiblePayload = len(attachments) > 0
	}
	return patched, report, true
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
	for _, candidate := range candidates {
		if patched, ok := finishStringWrappedRepair(probe, []byte(candidate)); ok {
			return patched, true
		}
	}
	return nil, false
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

// normalizeControlCharsInJSONStrings is a deterministic recovery
// pass for the "raw control char inside JSON string literal" streaming
// artefact. Walks `s` byte-by-byte while tracking string-state via a
// quote / escape state machine. Inside string literals, raw \n / \r /
// \t / form-feed / backspace are rewritten to their two-char escape
// sequences (\\n / \\r / \\t / \\f / \\b); other control bytes
// (0x00–0x1F minus the named ones) are rewritten to \\u00XX. Outside
// strings the input bytes pass through verbatim so structural braces
// / brackets / commas are unaffected.
//
// Returns (rewritten, changed). When the input is already valid JSON
// (no raw control chars in strings) the rewritten value is byte-
// identical to s and changed=false; the caller can skip a redundant
// re-parse in that case.
//
// The state machine is intentionally minimal:
//   - mode == "outside" or "inside-string"
//   - inside-string: a `"` exits unless preceded by an odd-count of
//     `\` (escape char). The escape-count is tracked by counting the
//     trailing `\` of the recently-emitted bytes.
func normalizeControlCharsInJSONStrings(s string) (string, bool) {
	// Fast path: scan for any byte < 0x20 (the entire control-char
	// range JSON forbids inside string literals). When the string is
	// already control-char-free, return early — Paths A/B failed for
	// a non-control-char reason and we have nothing to add.
	hasControl := false
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 {
			hasControl = true
			break
		}
	}
	if !hasControl {
		return s, false
	}
	var out strings.Builder
	out.Grow(len(s) + 16)
	inString := false
	escapeCount := 0
	changed := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !inString {
			out.WriteByte(c)
			if c == '"' {
				inString = true
				escapeCount = 0
			}
			continue
		}
		// inside string
		if c == '\\' {
			out.WriteByte(c)
			escapeCount++
			continue
		}
		if c == '"' && escapeCount%2 == 0 {
			out.WriteByte(c)
			inString = false
			escapeCount = 0
			continue
		}
		// Reset escape counter on any non-backslash byte (the next
		// `"` after `\\` IS terminal because the second `\` already
		// consumed the first as an escape pair).
		switch c {
		case '\n':
			out.WriteString(`\n`)
			changed = true
		case '\r':
			out.WriteString(`\r`)
			changed = true
		case '\t':
			out.WriteString(`\t`)
			changed = true
		case '\f':
			out.WriteString(`\f`)
			changed = true
		case '\b':
			out.WriteString(`\b`)
			changed = true
		default:
			if c < 0x20 {
				fmt.Fprintf(&out, `\u%04x`, c)
				changed = true
			} else {
				out.WriteByte(c)
			}
		}
		escapeCount = 0
	}
	if !changed {
		return s, false
	}
	return out.String(), true
}

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// repairStringWrappedArrayFields scans every top-level key of raw
// and repairs any value that arrived as a JSON-encoded string
// wrapping a JSON array. Same MiniMax streaming-bug pattern that
// repairBlocksAsString targets for the `blocks` field, generalised
// to every top-level array slot — used by tools (patch / full emit)
// whose schema has multiple array fields any of which may be
// stringified by the LLM.
//
// Layered:
//
//  1. Outer-level Path C normalisation: the outer json.Unmarshal
//     can fail when ANY string field contains raw control bytes
//     (not just array-typed fields). When the bare unmarshal fails
//     and normalisation changes the bytes, retry the unmarshal on
//     the normalised payload.
//  2. Per-field Path A: each top-level value that looks like a
//     JSON-encoded string starting with `[` gets unwrapped and
//     parsed as a real array. Success → replace; failure → fall
//     through.
//  3. Per-field Path C: re-run the per-field parse on the
//     normalised inner string when Path A fails (the inner
//     stringified content may itself carry unescaped control chars
//     for the same reason the outer payload did).
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
		if len(val) == 0 || val[0] != '"' {
			continue
		}
		var encoded string
		if err := json.Unmarshal(val, &encoded); err != nil {
			continue
		}
		trimmed := strings.TrimSpace(encoded)
		if !strings.HasPrefix(trimmed, "[") {
			continue
		}
		// Path A: direct array decode after the string unwrap.
		var inner []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &inner); err == nil {
			probe[key] = mustMarshal(inner)
			repaired = append(repaired, key)
			continue
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
			if patched, ok := mergeWholeDocStringifyVariants(probe, key, normalised); ok {
				probe = patched
				repaired = append(repaired, key)
				continue
			}
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

// repairNestedArraysAsString detects the same "JSON-encoded string
// where an array is expected" failure mode for the nested arrays
// inside each block (items, claim_uses) and
// returns a re-encoded RawMessage with each affected array inlined
// as a real array. Mirrors repairBlocksAsString conservatively —
// only the named fields are patched; everything else passes through
// verbatim so downstream V1-field detection + schema validation
// still fire on whatever else may be wrong.
//
// Returns (raw, [list of repaired paths], true) on at least one
// repair, or (raw, nil, false) when nothing was repaired.
//
// Paths use dotted-bracket notation so the WARN log can name the
// exact site (e.g. blocks[0].items, blocks[2].claim_uses).
func repairNestedArraysAsString(raw json.RawMessage) (json.RawMessage, []string, bool) {
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
		for _, field := range []string{"items", "claim_uses"} {
			if r, ok := repairBlockArrayField(blkObj, field); ok {
				blkObj[field] = r
				paths = append(paths, fmt.Sprintf("blocks[%d].%s", i, field))
				repaired = true
			}
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

// repairNestedArraysInPatch applies the same items / claim_uses
// JSON-string-array repair that repairNestedArraysAsString does for
// the full emit, but operates on the patch tool's `replace_blocks`
// AND `add_blocks` arrays (the emit's `blocks` is the singular
// equivalent). Patch payload normally arrives well-formed, but when
// the LLM stringifies a nested array inside a block in either
// replace_blocks or add_blocks the strict decode rejects with the
// same "must be a native JSON array" error — so we apply identical
// flat-mode tolerance here.
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
			for _, field := range []string{"items", "claim_uses"} {
				if r, ok := repairBlockArrayField(blkObj, field); ok {
					blkObj[field] = r
					paths = append(paths, fmt.Sprintf("%s[%d].%s", topField, i, field))
					blockChanged = true
				}
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

// repairBlockArrayField detects a JSON-encoded string at obj[field]
// where a JSON array is expected, and returns the re-encoded array
// RawMessage. Returns (raw, false) when the field is absent OR is
// already a real array OR the string content cannot be decoded as
// an array. Conservative: any decode failure leaves the field
// untouched so the regular validator surfaces the real schema
// violation.
func repairBlockArrayField(obj map[string]json.RawMessage, field string) (json.RawMessage, bool) {
	rawField, ok := obj[field]
	if !ok || len(rawField) == 0 {
		return nil, false
	}
	if rawField[0] != '"' {
		// Already a real array (or another shape) — nothing to do.
		return nil, false
	}
	var encoded string
	if err := json.Unmarshal(rawField, &encoded); err != nil {
		return nil, false
	}
	trimmed := strings.TrimSpace(encoded)
	if !strings.HasPrefix(trimmed, "[") {
		return nil, false
	}
	var inner []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &inner); err != nil {
		return nil, false
	}
	out, err := json.Marshal(inner)
	if err != nil {
		return nil, false
	}
	return out, true
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
