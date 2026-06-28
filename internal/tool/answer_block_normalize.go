// Package tool — answer_block_normalize.go
//
// G2 (post_v2_runtime_gap_remediation, 2026-05-04). Single source-of-
// truth converter from the JSON-shape emitAnswerBlockV2 to the typed
// types.AnswerBlock. Both full emit
// (executeAnswerDocumentV2 in emit_answer_document_v2.go) and patch
// emit (convertEmitBlocksToTyped in emit_answer_document_patch.go)
// MUST go through this normalizer so a typed annotation field added
// to AnswerBlock is automatically picked up by both paths.
//
// Pre-G2 the two callers maintained PARALLEL copies of the same
// per-block validation + conversion loop. The patch copy
// silently dropped EdgeAnchors (a typed annotation field added by
// the Phase 1-B source-fix); the full-emit copy included it. That is
// EXACTLY the failure mode G2 prevents — a single refactor surface
// keeps the parallel paths in lock-step.
//
// Red lines:
//   - Every exported type-asserted field on AnswerBlock MUST flow
//     through here. The TestNormalizeEmitAnswerBlock_AllFieldsPropagate
//     reflection test pins this so a future field addition fails the
//     test until it is wired.
//   - Error messages name the offending field via fieldPath so the
//     LLM-facing failEmit / err message routes (full + patch) surface
//     identical wording for the same fault.

package tool

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// NormalizeEmitAnswerBlock converts one emitAnswerBlockV2 record into
// the typed types.AnswerBlock, applying every per-block validation
// the V2 carrier requires (kind whitelist, surface_role enum, diagram
// body presence, item conversion). fieldPath is the JSON-pointer-
// style prefix the caller passes for error messages — e.g.
// "blocks[3]" (full emit) or "replace_blocks[1]" (patch emit).
//
// Returns (block, nil) on success; (zero-value, err) on the first
// detected validation failure. The caller is responsible for
// failEmit-style result wrapping (full emit) or fmt.Errorf wrapping
// (patch emit) — this function only carries the typed projection
// + per-record validation.
func NormalizeEmitAnswerBlock(raw emitAnswerBlockV2, fieldPath string) (types.AnswerBlock, error) {
	if strings.TrimSpace(raw.ID) == "" {
		return types.AnswerBlock{}, fmt.Errorf("%s: id is required and must be non-empty", fieldPath)
	}
	kind := types.AnswerBlockKind(raw.Kind)
	if !types.IsValidAnswerBlockKind(kind) {
		return types.AnswerBlock{}, fmt.Errorf("%s: kind=%q is not a valid block kind; allowed values: %v",
			fieldPath, raw.Kind, types.AllAnswerBlockKinds())
	}
	if caveat := strings.TrimSpace(raw.Caveat); caveat != "" {
		if kind != types.BlockCaveat {
			return types.AnswerBlock{}, fmt.Errorf("%s: caveat is only accepted as a local-model compatibility alias on kind=caveat blocks; use block.text for the visible prose",
				fieldPath)
		}
		if strings.TrimSpace(raw.Text) == "" {
			raw.Text = raw.Caveat
		}
	}
	blk := types.AnswerBlock{
		ID:          raw.ID,
		Kind:        kind,
		Title:       raw.Title,
		Text:        raw.Text,
		Columns:     normalizeTableStringSlice(raw.Columns),
		ClaimUses:   raw.ClaimUses,
		EdgeAnchors: raw.EdgeAnchors,
		FacetIDs:    raw.FacetIDs,
		SurfaceRole: types.SurfaceRole(raw.SurfaceRole),
	}
	if raw.ErrorGranularityVerdict != "" {
		verdict, ok := types.NormalizeErrorGranularityVerdict(raw.ErrorGranularityVerdict)
		if !ok || verdict == types.ErrorGranularityUnknown {
			return types.AnswerBlock{}, fmt.Errorf("%s: error_granularity_verdict=%q is not a valid error granularity verdict",
				fieldPath, raw.ErrorGranularityVerdict)
		}
		if kind != types.BlockDecision {
			return types.AnswerBlock{}, fmt.Errorf("%s: error_granularity_verdict is only valid on kind=decision blocks",
				fieldPath)
		}
		blk.ErrorGranularityVerdict = verdict
	}
	if raw.CurrentStatusVerdict != "" {
		verdict, ok := types.NormalizeCurrentStatusVerdict(raw.CurrentStatusVerdict)
		if !ok || verdict == types.CurrentStatusUnknown {
			return types.AnswerBlock{}, fmt.Errorf("%s: current_status_verdict=%q is not a valid current status verdict",
				fieldPath, raw.CurrentStatusVerdict)
		}
		if kind != types.BlockDecision {
			return types.AnswerBlock{}, fmt.Errorf("%s: current_status_verdict is only valid on kind=decision blocks",
				fieldPath)
		}
		blk.CurrentStatusVerdict = verdict
	}
	if raw.ScopeDisclosure != "" {
		disclosure, ok := types.NormalizeScopeDisclosureKind(raw.ScopeDisclosure)
		if !ok || disclosure == types.ScopeDisclosureUnknown {
			return types.AnswerBlock{}, fmt.Errorf("%s: scope_disclosure=%q is not a valid scope disclosure kind",
				fieldPath, raw.ScopeDisclosure)
		}
		blk.ScopeDisclosure = disclosure
	}
	if blk.SurfaceRole != "" {
		if _, ok := types.NormalizeSurfaceRole(string(blk.SurfaceRole)); !ok {
			return types.AnswerBlock{}, fmt.Errorf("%s: surface_role=%q is not a valid surface role",
				fieldPath, raw.SurfaceRole)
		}
	}
	if len(raw.Items) > 0 {
		blk.Items = make([]types.AnswerBlockItem, 0, len(raw.Items))
		for _, it := range raw.Items {
			candidateRole, ok := types.NormalizeAnswerCandidateRole(it.CandidateRole)
			if !ok {
				candidateRole = types.AnswerCandidateRoleOther
			}
			blk.Items = append(blk.Items, types.AnswerBlockItem{
				ID:            it.ID,
				Label:         it.Label,
				Text:          it.Text,
				Cells:         normalizeTableStringSlice(it.Cells),
				CandidateRole: candidateRole,
				CitationRef:   citationRefFromWire(it.CitationRef),
			})
		}
	}
	if raw.Diagram != nil {
		if blk.Kind != types.BlockDiagram {
			// A non-empty typed diagram sibling is a precise schema signal:
			// the model already chose the diagram carrier and only left the
			// discriminator stale. Correct the discriminator locally instead
			// of spending a finalizer retry on a lossless shape repair. Do
			// not infer diagrams from prose/text here; only an explicit
			// raw.Diagram payload is eligible.
			blk.Kind = types.BlockDiagram
		}
		normalizeEmitAnswerDiagram(raw.Diagram)
		diag := &types.AnswerDiagramBlock{
			Kind:     types.DiagramKind(raw.Diagram.Kind),
			Language: raw.Diagram.Language,
			Body:     raw.Diagram.Body,
		}
		if strings.TrimSpace(diag.Body) == "" {
			return types.AnswerBlock{}, fmt.Errorf("%s: diagram.body is empty — set diagram.body to the raw Mermaid source (the part inside the ```mermaid fences; the renderer adds the fences itself). diagram.body is the only place the diagram source lives — do not put it in the block-level text field", fieldPath)
		}
		blk.Diagram = diag
	} else if blk.Kind == types.BlockDiagram {
		return types.AnswerBlock{}, fmt.Errorf("%s: kind=diagram requires the sibling `diagram` object {kind: <flow|sequence|architecture|call_dag>, language: \"mermaid\", body: <raw mermaid source>}. If the diagram body is currently in the block-level `text` field, move it into `diagram.body` and set diagram.kind to the SEMANTIC family the contract names (NOT the Mermaid keyword)", fieldPath)
	}
	return blk, nil
}

// citationRefFromWire converts the optional wire citation_ref pointer
// into the typed item index. A nil pointer means the field was absent
// or null in the LLM JSON — i.e. the item is uncited — and maps to
// types.CitationRefUnset (-1), NOT to index 0. An explicit value
// (including a deliberate 0, the first pooled citation) is preserved.
// This is the single decode contract both NormalizeEmitAnswerBlock and
// the text-recovery path share.
func citationRefFromWire(p *FlexInt) int {
	if p == nil {
		return types.CitationRefUnset
	}
	return int(*p)
}

func normalizeTableStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.TrimSpace(s)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeEmitAnswerDiagram(diag *emitAnswerDiagramV2) {
	if diag == nil {
		return
	}
	diag.Body = stripOuterDiagramFence(diag.Body)
	diag.Body = mermaidcompat.NormalizeSourceForMarkdown(diag.Body)
	family := types.MermaidBodySyntaxFamily(diag.Body)
	if family == types.MermaidSyntaxUnknown || family == types.MermaidSyntaxUnsupported {
		return
	}
	if strings.TrimSpace(diag.Language) == "" {
		diag.Language = "mermaid"
	}
	if !strings.EqualFold(strings.TrimSpace(diag.Language), "mermaid") {
		return
	}
	kind := types.DiagramKind(strings.TrimSpace(diag.Kind))
	if types.DiagramKindAllowsMermaidSyntax(kind, family) {
		return
	}
	switch family {
	case types.MermaidSyntaxSequence:
		diag.Kind = string(types.DiagramSequence)
	case types.MermaidSyntaxFlow:
		diag.Kind = string(types.DiagramFlow)
	}
}

func stripOuterDiagramFence(body string) string {
	out := body
	for i := 0; i < 4; i++ {
		trimmed := strings.TrimSpace(out)
		if !strings.HasPrefix(trimmed, "```") {
			return out
		}
		lines := strings.Split(trimmed, "\n")
		if len(lines) < 3 {
			return out
		}
		if strings.TrimSpace(lines[len(lines)-1]) != "```" {
			return out
		}
		info := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[0]), "```")))
		switch info {
		case "", "mermaid", "text":
		default:
			return out
		}
		next := strings.Join(lines[1:len(lines)-1], "\n")
		if strings.TrimSpace(next) == strings.TrimSpace(out) {
			return out
		}
		out = next
	}
	return out
}

// splitFusedDiagramBlocks expands every FUSED block — a valid
// non-diagram kind that carries BOTH visible rows (items / columns)
// AND a renderable diagram payload — into two sibling blocks before
// NormalizeEmitAnswerBlock's discriminator repair runs. Without the
// split, the repair overwrites the declared kind to diagram and the
// renderer (which dispatches on Kind alone) silently drops the rows —
// the model's table content evaporates even though it was emitted
// correctly (2026-06-12 read_combo_pipeline_sequence_table forensics).
//
// Partition: the visible half keeps the declared kind plus
// id/title/text/columns/items/claim_uses/facet_ids/surface_role and
// every verdict field; the diagram half carries ONLY the diagram
// payload + edge_anchors under a derived unique id, inserted
// immediately after the visible half so narrative order survives.
//
// Trigger is a precise typed signal (no prose inspection):
// diagram payload present AND body non-empty even after the full
// diagram normalization pipeline (fence strip + mermaidcompat
// passes) AND declared kind is a valid non-diagram kind AND rows
// present. Anything else passes through untouched — in particular a
// body that normalizes to EMPTY (blank, fence-only, hidden-markers-
// only) keeps the existing single-block hard-reject path so the
// model gets one retryable "diagram.body is empty" error at its OWN
// blocks[] index instead of a system-authored entry failing on its
// behalf.
//
// At most maxBlocksPerDoc − len(blocks) fused blocks split; the rest
// pass through unsplit to the lossy discriminator-repair path (see
// the budget comment in the body) so the split never pushes a
// within-cap emit over the block cap.
//
// Return entries carry the MODEL's original blocks[] index so callers
// build validation-error fieldPaths against the model's own emission,
// not the post-split physical layout — a system-inserted diagram half
// must never shift the index a retry hint points at. A split-out
// diagram half inherits its source block's index: its payload is the
// model's, so an error on it is an error in that model block's
// diagram object.
//
// Doctrine note: this mirrors the no-rewrite rule from
// answer_document_table_compile.go — the system never rewrites
// model-authored surface content; it only re-homes the two payloads
// the model fused so BOTH stay visible.
func compactNativeDisplayOnlyBlockFragments(blocks []emitAnswerBlockV2) ([]splitEmitBlockEntry, []string) {
	out := make([]splitEmitBlockEntry, 0, len(blocks))
	var fields []string
	for i, b := range blocks {
		text, ok := nativeDisplayOnlyBlockFragmentText(b)
		if ok && len(out) > 0 && canAbsorbNativeDisplayOnlyBlockFragment(out[len(out)-1].raw) {
			prev := &out[len(out)-1]
			prev.raw.Text = joinAnswerBlockDisplayText(prev.raw.Text, text)
			fields = append(fields, fmt.Sprintf("blocks[%d].text→blocks[%d].text", i, prev.modelIndex))
			continue
		}
		out = append(out, splitEmitBlockEntry{raw: b, modelIndex: i})
	}
	return out, fields
}

func nativeDisplayOnlyBlockFragmentText(raw emitAnswerBlockV2) (string, bool) {
	if strings.TrimSpace(raw.ID) != "" || strings.TrimSpace(raw.Kind) != "" {
		return "", false
	}
	if len(raw.Items) > 0 || len(raw.Columns) > 0 || raw.Diagram != nil ||
		len(raw.ClaimUses) > 0 || len(raw.EdgeAnchors) > 0 || len(raw.FacetIDs) > 0 ||
		strings.TrimSpace(raw.SurfaceRole) != "" ||
		strings.TrimSpace(raw.Caveat) != "" ||
		strings.TrimSpace(raw.ErrorGranularityVerdict) != "" ||
		strings.TrimSpace(raw.CurrentStatusVerdict) != "" ||
		strings.TrimSpace(raw.ScopeDisclosure) != "" {
		return "", false
	}
	text := strings.TrimSpace(raw.Text)
	if text == "" {
		return "", false
	}
	if title := strings.TrimSpace(raw.Title); title != "" {
		text = title + "\n\n" + text
	}
	return text, true
}

func canAbsorbNativeDisplayOnlyBlockFragment(prev emitAnswerBlockV2) bool {
	if strings.TrimSpace(prev.ID) == "" {
		return false
	}
	kind := types.AnswerBlockKind(strings.TrimSpace(prev.Kind))
	if kind == types.BlockDiagram || !types.IsValidAnswerBlockKind(kind) {
		return false
	}
	return true
}

func joinAnswerBlockDisplayText(existing, extra string) string {
	existing = strings.TrimSpace(existing)
	extra = strings.TrimSpace(extra)
	switch {
	case existing == "":
		return extra
	case extra == "":
		return existing
	case strings.Contains(existing, extra):
		return existing
	default:
		return existing + "\n\n" + extra
	}
}

func splitFusedDiagramBlocks(logLabel string, blocks []emitAnswerBlockV2) []splitEmitBlockEntry {
	entries := make([]splitEmitBlockEntry, 0, len(blocks))
	for i, b := range blocks {
		entries = append(entries, splitEmitBlockEntry{raw: b, modelIndex: i})
	}
	return splitFusedDiagramBlockEntries(logLabel, entries)
}

func splitFusedDiagramBlockEntries(logLabel string, entries []splitEmitBlockEntry) []splitEmitBlockEntry {
	fused := 0
	for _, entry := range entries {
		if isFusedDiagramBlock(entry.raw) {
			fused++
		}
	}
	out := make([]splitEmitBlockEntry, 0, len(entries)+fused)
	if fused == 0 {
		return append(out, entries...)
	}
	// Cap budget: every split adds exactly one block to the FINAL
	// document, so the projected final count is len(blocks) + splits.
	// Splitting must never push an emit that fits maxBlocksPerDoc over
	// it — the merged-doc hard gate would then reject with a block
	// count the model never emitted (system-fabricated failure,
	// mis-attributed to the model). Fused blocks beyond the budget
	// pass through unsplit and take the pre-split path instead: the
	// discriminator repair accepts them lossily, which was the
	// behavior for every fused block before the split existed. The
	// budget is position-independent — an at-cap emit is protected
	// wherever the fused block sits, not only in the last slot.
	budget := maxBlocksPerDoc - len(entries)
	used := make(map[string]bool, len(entries)+fused)
	for _, entry := range entries {
		used[strings.TrimSpace(entry.raw.ID)] = true
	}
	// Duplicates get identical treatment (mirror of the patch split's
	// memo): if a stutter-emitted copy of an already-split block were
	// classified differently at the budget boundary, the copies would
	// stop being dedup-equal and the downstream identical-duplicate
	// tolerance (normalizeAnswerDocumentBlockIDSurface) could no
	// longer absorb them — a fabricated duplicate-id reject for an
	// emit that dedups cleanly as emitted. The memo compares on the
	// SAME canonical identity that dedup uses (the normalized typed
	// projection), so twins differing only in normalization-erased
	// surface (outer fences, id whitespace, cell padding) match too.
	// A duplicate of a split block emits the same visible half only
	// (no second system half, no second budget charge); a duplicate
	// of a passed-through block passes through.
	var seen []fusedSeen
	split, dupsOfSplit := 0, 0
	for _, entry := range entries {
		i, b := entry.modelIndex, entry.raw
		if !isFusedDiagramBlock(b) {
			out = append(out, entry)
			continue
		}
		canonical, canonicalOK := canonicalFusedBlock(b)
		dup := false
		for _, s := range seen {
			if matchesSeenFused(s, b, canonical, canonicalOK) {
				dup = true
				if s.didSplit {
					dupsOfSplit++
					visible := b
					visible.Diagram = nil
					visible.EdgeAnchors = nil
					out = append(out, splitEmitBlockEntry{raw: visible, modelIndex: i})
				} else {
					out = append(out, splitEmitBlockEntry{raw: b, modelIndex: i})
				}
				break
			}
		}
		if dup {
			continue
		}
		if split >= budget {
			seen = append(seen, fusedSeen{raw: b, canonical: canonical, canonicalOK: canonicalOK, didSplit: false})
			out = append(out, splitEmitBlockEntry{raw: b, modelIndex: i})
			continue
		}
		seen = append(seen, fusedSeen{raw: b, canonical: canonical, canonicalOK: canonicalOK, didSplit: true})
		visible := b
		visible.Diagram = nil
		visible.EdgeAnchors = nil
		diagramHalf := emitAnswerBlockV2{
			ID:          deriveSplitDiagramBlockID(b.ID, used),
			Kind:        string(types.BlockDiagram),
			Diagram:     b.Diagram,
			EdgeAnchors: b.EdgeAnchors,
		}
		out = append(out,
			splitEmitBlockEntry{raw: visible, modelIndex: i},
			splitEmitBlockEntry{raw: diagramHalf, modelIndex: i})
		split++
	}
	if split > 0 {
		logging.Warning("[%s] split %d fused diagram block(s): declared kind and visible rows preserved alongside the diagram payload", logLabel, split)
	}
	if skipped := fused - split - dupsOfSplit; skipped > 0 {
		logging.Warning("[%s] %d fused diagram block(s) passed through unsplit near the %d-block cap: discriminator repair keeps the diagram and drops the rows (lossy accept) instead of splitting past the cap", logLabel, skipped, maxBlocksPerDoc)
	}
	return out
}

// splitEmitBlockEntry pairs a (possibly split) raw block with the
// index of the MODEL block it derives from. Callers MUST build
// validation-error fieldPaths from modelIndex, never from the entry's
// position in the returned slice — positions after a split are
// system-shifted and point retry hints at the wrong block of the
// model's emission.
type splitEmitBlockEntry struct {
	raw        emitAnswerBlockV2
	modelIndex int
}

// fusedSeen records one processed fused entry for the duplicate memo.
// canonical is the entry's NormalizeEmitAnswerBlock projection — the
// SAME identity the downstream identical-duplicate dedup
// (normalizePatchBlockList) compares — so a stutter pair that differs
// only in what normalization erases (outer ``` fences, id whitespace,
// cell padding) still memo-matches. canonicalOK=false (the probe
// conversion errored) falls back to raw comparison.
type fusedSeen struct {
	raw         emitAnswerBlockV2
	canonical   types.AnswerBlock
	canonicalOK bool
	didSplit    bool
}

// canonicalFusedBlock projects a fused entry onto the downstream
// dedup's identity. The Diagram payload is copied first:
// NormalizeEmitAnswerBlock normalizes it IN PLACE and the model's raw
// entry must reach the eventual conversion untouched.
func canonicalFusedBlock(b emitAnswerBlockV2) (types.AnswerBlock, bool) {
	if b.Diagram != nil {
		d := *b.Diagram
		b.Diagram = &d
	}
	blk, err := NormalizeEmitAnswerBlock(b, "fused-memo-probe")
	if err != nil {
		return types.AnswerBlock{}, false
	}
	return blk, true
}

// matchesSeenFused reports whether b is a duplicate of s under the
// canonical identity (falling back to raw identity when either
// projection failed).
func matchesSeenFused(s fusedSeen, b emitAnswerBlockV2, canonical types.AnswerBlock, canonicalOK bool) bool {
	if s.canonicalOK && canonicalOK {
		return reflect.DeepEqual(s.canonical, canonical)
	}
	return reflect.DeepEqual(s.raw, b)
}

// isFusedDiagramBlock reports whether raw fuses a visible-row block
// with a diagram payload. All conjuncts are typed-field checks.
func isFusedDiagramBlock(raw emitAnswerBlockV2) bool {
	if raw.Diagram == nil || strings.TrimSpace(raw.Diagram.Body) == "" {
		return false
	}
	// The split must never manufacture a system block that fails
	// validation. NormalizeEmitAnswerBlock rejects "diagram.body is
	// empty" on the body AFTER the full normalizeEmitAnswerDiagram
	// pipeline (fence strip + mermaidcompat passes such as
	// hidden-marker line removal), so the split predicate must judge
	// emptiness on the SAME pipeline — a fence-only or
	// markers-only body that empties under normalization would
	// otherwise split into a system half that fails on an entry the
	// model never authored. Run the pipeline on a COPY:
	// normalizeEmitAnswerDiagram mutates its argument in place and
	// the model's payload must reach the eventual normalize pass
	// untouched. Keeping the block whole routes the same reject to
	// the model's own blocks[] index via the discriminator repair.
	probe := *raw.Diagram
	normalizeEmitAnswerDiagram(&probe)
	if strings.TrimSpace(probe.Body) == "" {
		return false
	}
	kind := types.AnswerBlockKind(strings.TrimSpace(raw.Kind))
	if kind == types.BlockDiagram || !types.IsValidAnswerBlockKind(kind) {
		return false
	}
	return len(raw.Items) > 0 || len(raw.Columns) > 0
}

// deriveSplitDiagramBlockID returns a block id unique within the
// emit for the split-out diagram half. The suffixed form keeps the
// pairing visible in logs and downstream telemetry.
func deriveSplitDiagramBlockID(baseID string, used map[string]bool) string {
	candidate := peekSplitDiagramBlockID(baseID, used)
	used[candidate] = true
	return candidate
}

// peekSplitDiagramBlockID returns the id deriveSplitDiagramBlockID
// WOULD assign, without committing it to the used map. The patch
// split peeks so its exhaustion and budget gates can inspect the
// candidate before committing it — committing on a pass-through
// would burn the id and shift a later fused entry's derivation.
func peekSplitDiagramBlockID(baseID string, used map[string]bool) string {
	base := strings.TrimSpace(baseID)
	if base == "" {
		base = "block"
	}
	candidate := base + "_diagram"
	for i := 2; used[candidate] && i < 100; i++ {
		candidate = fmt.Sprintf("%s_diagram%d", base, i)
	}
	return candidate
}

// splitFusedDiagramPatchBlocks applies the fused-block split to the
// patch emit's two raw lists. replace_blocks merges strictly one
// block per replaced id, so the diagram half of a fused REPLACE
// entry cannot stay in replace_blocks — it lands in add_blocks (the
// merged doc places adds at the tail; losing adjacency beats losing
// the payload). A fused ADD entry keeps its slot as the visible half.
//
// EVERY system-derived diagram half — replace- and add-derived alike
// — is appended AFTER the model's own add_blocks entries. Index
// attribution depends on it: convertEmitBlocksToTyped builds
// `add_blocks[i]` validation-error fieldPaths from list position, so
// the model's entries must occupy exactly the indices the model
// emitted them at. System halves sit at indices >= the model's count
// and cannot fail validation by construction: isFusedDiagramBlock
// judges body emptiness on the SAME normalization pipeline
// NormalizeEmitAnswerBlock later applies, so a half that would
// reject "diagram.body is empty" is never manufactured. The cost is
// tail-of-tail placement for add-derived halves in the merged doc;
// adjacency was already sacrificed for replace-derived halves.
//
// Derived-id collision discipline (prev-aware, 2026-06-12): the
// derived `<id>_diagram` id may collide with EXACTLY ONE thing — a
// prev-doc kind=diagram block the model has not claimed via any
// patch op (typically, but not provably, a prior split's persisted
// half: provenance is not tracked, so a model-authored kind=diagram
// block whose id matches the derivation is refreshed the same way).
// That collision is the intended steady-state refresh: the half
// lands in add_blocks, the add→replace tolerance demotes it, and the
// persisted diagram half is replaced in place (count-neutral — no
// cap budget charged). Every other id is seeded into the collision
// set so the derivation suffix-numbers past it:
//   - the patch's own replace/add ids   (within-patch uniqueness)
//   - prev block ids with Kind!=diagram (a collision would demote
//     the half onto model-authored non-diagram content — silent
//     clobber, the no-rewrite doctrine's hardest violation)
//   - unchanged_block_ids               (R16 byte-identical promise:
//     Apply consults replacedBy before the unchanged passthrough, so
//     a demoted half would override the model's declaration)
//   - remove_block_ids                  (the tolerance would convert
//     the remove+add pair into a replace and silently DROP the
//     model's remove op)
//
// budget caps how many block-ADDING splits may run: a fresh-id split
// adds exactly one block to the MERGED document, so the caller
// passes the remaining headroom under maxBlocksPerDoc (see
// fusedPatchSplitBudget); refresh splits are count-neutral and run
// budget-free. Fused entries beyond the budget pass through unsplit
// and take the discriminator-repair (lossy-accept) path — same
// fallback, and same rationale, as the full-emit split: the system
// must never fabricate a cap reject for a patch whose merged count
// was within the cap as the model emitted it.
func splitFusedDiagramPatchBlocks(logLabel string, budget int, prev *types.AnswerDocumentV2, removeIDs, unchangedIDs []string, replaceBlocks, addBlocks []emitAnswerBlockV2) ([]emitAnswerBlockV2, []emitAnswerBlockV2) {
	fused := 0
	for _, b := range replaceBlocks {
		if isFusedDiagramBlock(b) {
			fused++
		}
	}
	for _, b := range addBlocks {
		if isFusedDiagramBlock(b) {
			fused++
		}
	}
	if fused == 0 {
		return replaceBlocks, addBlocks
	}
	used := make(map[string]bool, len(replaceBlocks)+len(addBlocks)+fused)
	for _, b := range replaceBlocks {
		used[strings.TrimSpace(b.ID)] = true
	}
	for _, b := range addBlocks {
		used[strings.TrimSpace(b.ID)] = true
	}
	for _, id := range removeIDs {
		used[strings.TrimSpace(id)] = true
	}
	for _, id := range unchangedIDs {
		used[strings.TrimSpace(id)] = true
	}
	// refreshable: prev kind=diagram ids not claimed by any op above
	// — the only ids a derived half may legitimately collide with.
	// Computed AFTER the seeding so a claimed diagram id is excluded.
	refreshable := map[string]bool{}
	if prev != nil {
		for _, pb := range prev.Blocks {
			id := strings.TrimSpace(pb.ID)
			if id == "" {
				continue
			}
			if pb.Kind == types.BlockDiagram && !used[id] {
				refreshable[id] = true
				continue
			}
			used[id] = true
		}
	}
	outReplace := make([]emitAnswerBlockV2, 0, len(replaceBlocks))
	outAdd := make([]emitAnswerBlockV2, 0, len(addBlocks)+fused)
	systemHalves := make([]emitAnswerBlockV2, 0, fused)
	// Duplicates get identical treatment: the downstream
	// identical-duplicate tolerance (normalizePatchBlockList) can only
	// absorb a model stutter-emit when both copies stay dedup-equal
	// through the split. Classifying the copies differently (first
	// splits, second straddles the budget/refresh boundary and passes
	// through) would de-identicalize them and turn a cleanly-merging
	// patch into a fabricated "duplicated" hard reject. The memo
	// compares on the SAME canonical identity that dedup uses (the
	// normalized typed projection), so twins differing only in
	// normalization-erased surface (outer fences, id whitespace, cell
	// padding) match too. It replays the first occurrence's outcome:
	// split duplicates emit the same visible half with NO second
	// system half and NO second budget charge; pass-through
	// duplicates pass through.
	var seen []fusedSeen
	split, refreshes, dupsOfSplit := 0, 0, 0
	// splitHalf decides one fused entry's fate. half non-nil → split
	// (emit the visible half + record the system half); passThrough →
	// emit the entry unchanged; both nil/false → duplicate of an
	// already-split entry (emit the visible half only — lossless, the
	// first occurrence's split carries the payload and the downstream
	// identical-duplicate tolerance collapses the visible copies).
	splitHalf := func(b emitAnswerBlockV2) (half *emitAnswerBlockV2, passThrough bool) {
		canonical, canonicalOK := canonicalFusedBlock(b)
		for _, s := range seen {
			if matchesSeenFused(s, b, canonical, canonicalOK) {
				if s.didSplit {
					dupsOfSplit++
				}
				return nil, !s.didSplit
			}
		}
		// Refresh-first: walk the whole suffix chain for a persisted
		// half to refresh — an earlier turn's remove/unchanged claim
		// can have landed the half at a suffixed id, and minting the
		// lower suffix as a fresh id would strand the persisted half
		// as a stale duplicate (and re-open the at-cap row loss the
		// refresh exemption closes). Refresh is count-neutral and
		// budget-free. The walk covers the same 99-candidate id space
		// peekSplitDiagramBlockID mints from (_diagram, _diagram2 ..
		// _diagram99) so the two bounds cannot drift.
		base := strings.TrimSpace(b.ID)
		if base == "" {
			base = "block"
		}
		target := ""
		isRefresh := false
		for i := 1; i < 100; i++ {
			candidate := base + "_diagram"
			if i > 1 {
				candidate = fmt.Sprintf("%s_diagram%d", base, i)
			}
			if refreshable[candidate] && !used[candidate] {
				target = candidate
				isRefresh = true
				break
			}
		}
		if !isRefresh {
			target = peekSplitDiagramBlockID(b.ID, used)
			// Suffix space exhausted (every candidate claimed): a
			// committed collision would fabricate a "duplicated"
			// reject for ids the model never emitted — fall back to
			// the lossy-accept pass-through instead.
			if used[target] {
				seen = append(seen, fusedSeen{raw: b, canonical: canonical, canonicalOK: canonicalOK, didSplit: false})
				return nil, true
			}
			if split >= budget {
				seen = append(seen, fusedSeen{raw: b, canonical: canonical, canonicalOK: canonicalOK, didSplit: false})
				return nil, true
			}
		}
		used[target] = true
		if isRefresh {
			refreshes++
		} else {
			split++
		}
		seen = append(seen, fusedSeen{raw: b, canonical: canonical, canonicalOK: canonicalOK, didSplit: true})
		return &emitAnswerBlockV2{
			ID:          target,
			Kind:        string(types.BlockDiagram),
			Diagram:     b.Diagram,
			EdgeAnchors: b.EdgeAnchors,
		}, false
	}
	for _, b := range replaceBlocks {
		if !isFusedDiagramBlock(b) {
			outReplace = append(outReplace, b)
			continue
		}
		half, passThrough := splitHalf(b)
		if passThrough {
			outReplace = append(outReplace, b)
			continue
		}
		visible := b
		visible.Diagram = nil
		visible.EdgeAnchors = nil
		outReplace = append(outReplace, visible)
		if half != nil {
			systemHalves = append(systemHalves, *half)
		}
	}
	for _, b := range addBlocks {
		if !isFusedDiagramBlock(b) {
			outAdd = append(outAdd, b)
			continue
		}
		half, passThrough := splitHalf(b)
		if passThrough {
			outAdd = append(outAdd, b)
			continue
		}
		visible := b
		visible.Diagram = nil
		visible.EdgeAnchors = nil
		outAdd = append(outAdd, visible)
		if half != nil {
			systemHalves = append(systemHalves, *half)
		}
	}
	outAdd = append(outAdd, systemHalves...)
	if split+refreshes > 0 {
		logging.Warning("[%s] split %d fused diagram block(s) across patch ops (%d refreshing a persisted diagram half): declared kind and visible rows preserved alongside the diagram payload", logLabel, split+refreshes, refreshes)
	}
	if skipped := fused - split - refreshes - dupsOfSplit; skipped > 0 {
		logging.Warning("[%s] %d fused diagram block(s) in patch ops passed through unsplit (no headroom under the %d-block cap, or derived-id suffix space exhausted): discriminator repair keeps the diagram and drops the rows (lossy accept) instead of fabricating a reject", logLabel, skipped, maxBlocksPerDoc)
	}
	return outReplace, outAdd
}

// fusedPatchSplitBudget projects the MERGED document's block count
// for the patch as emitted (before any splits) and returns the
// remaining headroom under maxBlocksPerDoc — the number of
// block-ADDING (fresh-id) fused splits the patch can absorb without
// the merged-doc hard gate rejecting a count the model never
// produced. Refresh-classified splits (derived id collides with an
// unclaimed prev kind=diagram block) are count-neutral and exempt
// from this budget — see splitFusedDiagramPatchBlocks.
//
// The projection mirrors — or over-estimates, never under-estimates —
// the deterministic prev-id tolerance
// (normalizeAnswerDocumentPatchBlockOps) and ApplyAnswerDocumentV2Patch
// for every patch shape those layers ACCEPT:
//
//   - replace of an existing prev id        → count unchanged
//   - replace of a non-prev id              → promoted to add (+1)
//   - add of a new id                       → +1
//   - add of an existing prev id            → demoted to replace (+0)
//   - remove of a prev id                   → −1, EXCEPT when the same
//     id is also in add_blocks — the tolerance turns that pair into a
//     replace and drops the remove (net 0)
//
// Patch shapes outside the accepted set (overlapping ops, unknown
// remove ids) are rejected by validatePatchStructure before the cap
// is ever consulted, so their projection is irrelevant. Identical-
// duplicate replace/add entries are deliberately over-counted per
// entry: normalizeAnswerDocumentPatchIDSurface dedups them AFTER
// this budget is computed, so the actual merged count only ever
// comes out lower. The invariant is one-directional: this function
// must never under-estimate the merged count (a too-small budget
// degrades to the lossy-accept pass-through — the pre-split
// behavior — never to a fabricated reject).
func fusedPatchSplitBudget(prev *types.AnswerDocumentV2, removeIDs []string, replaceBlocks, addBlocks []emitAnswerBlockV2) int {
	if prev == nil {
		// No merge base: the patch will be rejected upstream for
		// other reasons; an unbounded budget keeps the split's
		// payload-preserving behavior for any recovery path.
		return maxBlocksPerDoc
	}
	prevIDs := make(map[string]bool, len(prev.Blocks))
	for _, b := range prev.Blocks {
		prevIDs[strings.TrimSpace(b.ID)] = true
	}
	addIDs := make(map[string]bool, len(addBlocks))
	adds := 0
	for _, b := range addBlocks {
		id := strings.TrimSpace(b.ID)
		addIDs[id] = true
		if !prevIDs[id] {
			adds++
		}
	}
	removes := 0
	seenRemove := make(map[string]bool, len(removeIDs))
	for _, raw := range removeIDs {
		id := strings.TrimSpace(raw)
		if seenRemove[id] {
			continue
		}
		seenRemove[id] = true
		if prevIDs[id] && !addIDs[id] {
			removes++
		}
	}
	promotions := 0
	for _, b := range replaceBlocks {
		if !prevIDs[strings.TrimSpace(b.ID)] {
			promotions++
		}
	}
	projected := len(prev.Blocks) - removes + adds + promotions
	return maxBlocksPerDoc - projected
}
