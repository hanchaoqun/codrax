package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

var answerDocumentSnippetSelectorFields = stringSet("file", "start_line", "end_line", "language", "code")

// normalizeMisroutedPatchBlockOperations repairs one structurally
// unambiguous carrier mistake: answer-block operations emitted under
// replace_snippets. It never reads request/prose content and never chooses a
// visible field. Full block payloads move field-for-field. A homogeneous
// {block_id, ...explicit block fields} array is expanded from the exact prior
// block and only the explicitly submitted fields overwrite that base.
// Ambiguous, mixed, unknown, duplicated, or conflicting shapes return a
// violation so the caller can fail loudly instead of quarantining the block
// payload into an empty snippet.
func normalizeMisroutedPatchBlockOperations(raw json.RawMessage, prev *types.AnswerDocumentV2) (json.RawMessage, []string, string) {
	if prev == nil || len(bytes.TrimSpace(raw)) == 0 {
		return raw, nil, ""
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil {
		return raw, nil, ""
	}
	snippetsRaw, ok := root["replace_snippets"]
	if !ok {
		return raw, nil, ""
	}
	var entries []map[string]json.RawMessage
	if json.Unmarshal(snippetsRaw, &entries) != nil || len(entries) == 0 {
		return raw, nil, ""
	}
	blockLike := false
	for _, entry := range entries {
		for key := range entry {
			if key == "block_id" || answerDocumentBlockAllowedFields[key] {
				blockLike = true
				break
			}
		}
	}
	if !blockLike {
		return raw, nil, ""
	}
	if _, conflict := root["replace_blocks"]; conflict {
		return raw, nil, "replace_blocks is already present"
	}

	previous := make(map[string]types.AnswerBlock, len(prev.Blocks))
	duplicatePrevious := map[string]bool{}
	for _, block := range prev.Blocks {
		id := strings.TrimSpace(block.ID)
		if id == "" {
			continue
		}
		if _, exists := previous[id]; exists {
			duplicatePrevious[id] = true
		}
		previous[id] = block
	}

	mode := ""
	seen := map[string]bool{}
	replacements := make([]json.RawMessage, 0, len(entries))
	ids := make([]string, 0, len(entries))
	for i, entry := range entries {
		for key := range entry {
			if answerDocumentSnippetSelectorFields[key] {
				return raw, nil, fmt.Sprintf("replace_snippets[%d] mixes snippet field %q with a block operation", i, key)
			}
		}
		id, hasID := exactPatchCompatString(entry["id"])
		blockID, hasBlockID := exactPatchCompatString(entry["block_id"])
		entryMode := ""
		switch {
		case hasID && !hasBlockID:
			entryMode = "full"
		case hasBlockID && !hasID:
			entryMode = "partial"
			id = blockID
		default:
			return raw, nil, fmt.Sprintf("replace_snippets[%d] must carry exactly one block selector: id or block_id", i)
		}
		if mode == "" {
			mode = entryMode
		} else if mode != entryMode {
			return raw, nil, "replace_snippets mixes full-block and block_id partial operation shapes"
		}
		if id == "" || duplicatePrevious[id] {
			return raw, nil, fmt.Sprintf("block selector %q is empty or ambiguous in the previous document", id)
		}
		prior, exists := previous[id]
		if !exists {
			return raw, nil, fmt.Sprintf("block selector %q is not present in the previous document", id)
		}
		if prior.SystemGeneratedKind != types.AnswerSystemGeneratedBlockUnknown {
			return raw, nil, fmt.Sprintf("block selector %q names a system-generated block that is not model-editable", id)
		}
		if seen[id] {
			return raw, nil, fmt.Sprintf("block selector %q is duplicated", id)
		}
		seen[id] = true

		for key := range entry {
			if key == "block_id" {
				continue
			}
			if !answerDocumentBlockAllowedFields[key] {
				return raw, nil, fmt.Sprintf("replace_snippets[%d] carries unknown block field %q", i, key)
			}
		}

		candidate := make(map[string]json.RawMessage)
		if entryMode == "partial" {
			if len(entry) == 1 {
				return raw, nil, fmt.Sprintf("block_id %q has no explicit field mutation", id)
			}
			base, err := json.Marshal(emitAnswerBlockFromTyped(prior))
			if err != nil || json.Unmarshal(base, &candidate) != nil {
				return raw, nil, fmt.Sprintf("previous block %q could not be represented losslessly", id)
			}
			for key, value := range entry {
				if key != "block_id" {
					candidate[key] = append(json.RawMessage(nil), value...)
				}
			}
			candidate["id"] = mustMarshal(id)
		} else {
			for key, value := range entry {
				candidate[key] = append(json.RawMessage(nil), value...)
			}
			kind, ok := exactPatchCompatString(candidate["kind"])
			if !ok || !types.IsValidAnswerBlockKind(types.AnswerBlockKind(kind)) {
				return raw, nil, fmt.Sprintf("full block %q has no valid kind", id)
			}
		}
		encoded, err := json.Marshal(candidate)
		if err != nil {
			return raw, nil, fmt.Sprintf("block %q could not be encoded", id)
		}
		replacements = append(replacements, encoded)
		ids = append(ids, id)
	}

	// Require the resulting block carrier to be accepted byte-for-byte by the
	// canonical quarantine surface. If quarantine would drop or rewrite any
	// field, the remap is not lossless and must not occur.
	probe := map[string]json.RawMessage{"replace_blocks": mustMarshal(replacements)}
	var quarantinePaths []string
	if quarantineBlockArray(probe, "replace_blocks", "replace_blocks", &quarantinePaths) {
		sort.Strings(quarantinePaths)
		return raw, nil, "block payload contains non-canonical fields: " + strings.Join(quarantinePaths, ", ")
	}
	var decoded []emitAnswerBlockV2
	dec := json.NewDecoder(bytes.NewReader(probe["replace_blocks"]))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decoded); err != nil {
		return raw, nil, "block payload does not match the canonical block schema: " + err.Error()
	}
	if _, err := convertEmitBlocksToTyped("replace_snippets block-operation compatibility", decoded, "replace_blocks"); err != nil {
		return raw, nil, err.Error()
	}

	root["replace_blocks"] = probe["replace_blocks"]
	delete(root, "replace_snippets")
	repaired, err := json.Marshal(root)
	if err != nil || !json.Valid(repaired) {
		return raw, nil, "remapped patch could not be encoded"
	}
	sort.Strings(ids)
	return repaired, ids, ""
}

func exactPatchCompatString(raw json.RawMessage) (string, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}

func emitAnswerBlockFromTyped(block types.AnswerBlock) emitAnswerBlockV2 {
	out := emitAnswerBlockV2{
		ID:                      block.ID,
		Kind:                    string(block.Kind),
		Title:                   block.Title,
		Text:                    block.Text,
		ErrorGranularityVerdict: string(block.ErrorGranularityVerdict),
		CurrentStatusVerdict:    string(block.CurrentStatusVerdict),
		TraceCausalClaimCaliber: string(block.TraceCausalClaimCaliber),
		ScopeDisclosure:         string(block.ScopeDisclosure),
		SourceInventoryFamily:   block.SourceInventoryFamily,
		Columns:                 append([]string(nil), block.Columns...),
		ClaimUses:               append([]types.RenderedClaimUse(nil), block.ClaimUses...),
		EdgeAnchors:             append([]types.DiagramEdgeAnchor(nil), block.EdgeAnchors...),
		ParticipantBoundaries:   append([]types.DiagramParticipantBoundary(nil), block.ParticipantBoundaries...),
		RequestedRelationScope:  string(block.RequestedRelationScope),
		RelationClaims:          append([]types.AnswerRelationClaim(nil), block.RelationClaims...),
		FacetIDs:                append([]string(nil), block.FacetIDs...),
		SurfaceRole:             string(block.SurfaceRole),
	}
	if block.RuntimeWorkRelation != nil {
		out.RuntimeWorkRelation = &emitRuntimeWorkRelationReceipt{
			ObservationID: block.RuntimeWorkRelation.ObservationID,
			Conclusion:    string(block.RuntimeWorkRelation.Conclusion),
		}
	}
	if block.ConceptualTerminalResolution != nil {
		out.ConceptualTerminalResolution = &emitConceptualTerminalResolutionReceipt{
			EvidenceID: block.ConceptualTerminalResolution.EvidenceID,
			Conclusion: string(block.ConceptualTerminalResolution.Conclusion),
		}
	}
	if block.Diagram != nil {
		out.Diagram = &emitAnswerDiagramV2{
			Kind: string(block.Diagram.Kind), Language: block.Diagram.Language, Body: block.Diagram.Body,
		}
	}
	for _, item := range block.Items {
		rawItem := emitAnswerBlockItemV2{
			ID: item.ID, Label: item.Label, Text: item.Text,
			Cells: append([]string(nil), item.Cells...), CandidateRole: string(item.CandidateRole),
			SourceInventoryRowID: item.SourceInventoryRowID,
			EvidenceIDs:          append([]string(nil), item.EvidenceIDs...),
		}
		if item.CitationRef != types.CitationRefUnset {
			ref := FlexInt(item.CitationRef)
			rawItem.CitationRef = &ref
		}
		for _, ref := range item.CitationRefs {
			rawItem.CitationRefs = append(rawItem.CitationRefs, FlexInt(ref))
		}
		out.Items = append(out.Items, rawItem)
	}
	return out
}
