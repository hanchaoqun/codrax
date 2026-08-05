package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

// AnswerDocumentTextRecovery is the display-only recovery result for
// a model-authored answer_document payload that was written as plain
// assistant text instead of executed through the tool channel.
//
// The recovered Document is suitable for deterministic rendering, but
// callers must still disclose that the normal emit-time validators did
// not run. This path is intentionally downstream-only: it preserves
// visible user answer content after the retry budget is exhausted, and
// it does not mutate MutableState or pretend a tool call succeeded.
type AnswerDocumentTextRecovery struct {
	Document    *types.AnswerDocumentV2
	Attachments []types.AnswerDisplayAttachment
	Mode        string
	Lossless    bool
	Diagnostics []string
}

// RecoverAnswerDocumentV2FromText extracts and renders the best
// answer_document-shaped payload from assistant text when the provider
// ignored tool_choice and returned the JSON in content. Recovery is
// schema-driven: it looks only for the answer_document carrier surface
// (blocks[] and sibling typed fields), never for user-question terms or
// answer-specific prose.
//
// Layers:
//   - strict-ish JSON document recovery: balanced object/array with
//     blocks[] -> typed AnswerDocumentV2 via the same block normalizer
//     used by emit_answer_document / patch.
//   - visible block salvage: when typed validation cannot keep every
//     field, preserve renderable block text/items/diagram bodies so the
//     final panel shows the model-authored answer instead of raw JSON.
func RecoverAnswerDocumentV2FromText(content string) (AnswerDocumentTextRecovery, bool) {
	candidates := answerDocumentJSONCandidates(content)
	for _, raw := range candidates {
		if rec, ok := recoverAnswerDocumentV2FromRawCandidate(raw); ok {
			return rec, true
		}
	}
	return AnswerDocumentTextRecovery{}, false
}

func answerDocumentJSONCandidates(content string) []json.RawMessage {
	content = strings.TrimSpace(content)
	if content == "" || !textContainsAnswerDocumentBlocksMarker(content) {
		return nil
	}
	var out []json.RawMessage
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || !textContainsAnswerDocumentBlocksMarker(s) {
			return
		}
		for _, existing := range out {
			if string(existing) == s {
				return
			}
		}
		out = append(out, json.RawMessage(s))
	}

	for _, fenced := range fencedJSONBodies(content) {
		add(fenced)
	}
	for _, candidate := range balancedJSONValues(content) {
		add(candidate)
	}
	if idx := strings.Index(content, "{"); idx >= 0 {
		add(content[idx:])
	}
	if idx := strings.Index(content, "["); idx >= 0 {
		add(content[idx:])
	}
	return out
}

func textContainsAnswerDocumentBlocksMarker(s string) bool {
	return strings.Contains(s, `"blocks"`) || strings.Contains(s, `\"blocks\"`)
}

func fencedJSONBodies(content string) []string {
	var out []string
	searchFrom := 0
	for {
		start := strings.Index(content[searchFrom:], "```")
		if start < 0 {
			return out
		}
		start += searchFrom
		lineEnd := strings.IndexByte(content[start+3:], '\n')
		if lineEnd < 0 {
			return out
		}
		info := strings.TrimSpace(content[start+3 : start+3+lineEnd])
		bodyStart := start + 3 + lineEnd + 1
		end := strings.Index(content[bodyStart:], "```")
		if end < 0 {
			return out
		}
		bodyEnd := bodyStart + end
		if info == "" || strings.EqualFold(info, "json") || strings.HasPrefix(strings.ToLower(info), "json ") {
			out = append(out, content[bodyStart:bodyEnd])
		}
		searchFrom = bodyEnd + 3
	}
}

func balancedJSONValues(content string) []string {
	var out []string
	for i := 0; i < len(content); i++ {
		if content[i] != '{' && content[i] != '[' {
			continue
		}
		if end, ok := balancedJSONValueEnd(content, i); ok {
			out = append(out, content[i:end])
			i = end - 1
		}
	}
	return out
}

func balancedJSONValueEnd(s string, start int) (int, bool) {
	if start < 0 || start >= len(s) || (s[start] != '{' && s[start] != '[') {
		return 0, false
	}
	var stack []byte
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
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
		case '{', '[':
			stack = append(stack, c)
		case '}', ']':
			if len(stack) == 0 {
				return 0, false
			}
			open := stack[len(stack)-1]
			if (open == '{' && c != '}') || (open == '[' && c != ']') {
				return 0, false
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}

func recoverAnswerDocumentV2FromRawCandidate(raw json.RawMessage) (AnswerDocumentTextRecovery, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return AnswerDocumentTextRecovery{}, false
	}
	for _, nested := range nestedAnswerDocumentPayloads(raw) {
		if bytes.Equal(bytes.TrimSpace(nested), raw) {
			continue
		}
		if rec, ok := recoverAnswerDocumentV2FromRawCandidate(nested); ok {
			rec.Mode = "content_json_nested_" + rec.Mode
			return rec, true
		}
	}
	if raw[0] == '[' {
		raw = json.RawMessage(append(append([]byte(`{"blocks":`), raw...), '}'))
	}
	attempts := []struct {
		raw  json.RawMessage
		mode string
	}{
		{raw: raw, mode: "content_json_document"},
	}
	if normalised, changed := normalizeControlCharsInJSONStrings(string(raw)); changed {
		attempts = append(attempts, struct {
			raw  json.RawMessage
			mode string
		}{raw: json.RawMessage(normalised), mode: "content_json_control_char_normalized"})
	}
	if repaired, changed := toolparam.RemoveTrailingCommasBeforeJSONClosers(string(raw)); changed {
		attempts = append([]struct {
			raw  json.RawMessage
			mode string
		}{{raw: json.RawMessage(repaired), mode: "content_json_trailing_comma"}}, attempts...)
		if normalised, normalisedChanged := normalizeControlCharsInJSONStrings(repaired); normalisedChanged {
			attempts = append([]struct {
				raw  json.RawMessage
				mode string
			}{{raw: json.RawMessage(normalised), mode: "content_json_trailing_comma_control_char_normalized"}}, attempts...)
		}
	}
	if repaired, report, ok := repairBlocksAsStringDetailed(raw); ok {
		mode := "content_json_" + report.Mode
		attempts = append(attempts, struct {
			raw  json.RawMessage
			mode string
		}{raw: repaired, mode: mode})
	}
	for _, attempt := range attempts {
		if repaired, _, ok := repairNestedArraysAsString(attempt.raw); ok {
			if rec, err := decodeRecoveredAnswerDocumentV2(repaired, attempt.mode+"_nested_arrays"); err == nil {
				return rec, true
			}
		}
		if rec, err := decodeRecoveredAnswerDocumentV2(attempt.raw, attempt.mode); err == nil {
			return rec, true
		}
	}
	for _, attempt := range attempts {
		if rec, ok := visibleAnswerDocumentFromRaw(attempt.raw, attempt.mode+"_visible_blocks"); ok {
			return rec, true
		}
		if repaired, report, ok := extractBlocksByBraceBalanceDetailed(attempt.raw); ok {
			rec, recovered := visibleAnswerDocumentFromRaw(repaired, "content_json_"+report.Mode+"_visible_blocks")
			if recovered {
				rec.Attachments = append(rec.Attachments, report.Attachments...)
				rec.Lossless = false
				return rec, true
			}
		}
	}
	// Last-resort, display-only salvage.  A provider can return a malformed
	// JSON envelope whose individual visible string values are still intact.
	// Keep only schema-owned user-visible fields; never mine ids, claim forms,
	// citations, or arbitrary prose for control signals.  The result stays on
	// the typed degraded lane and is explicitly disclosed as incomplete.
	if text, diagnostics, ok := recoverMalformedAnswerDocumentVisibleStrings(string(raw)); ok {
		return AnswerDocumentTextRecovery{
			Document: &types.AnswerDocumentV2{
				DocumentModel: "v2",
				Blocks: []types.AnswerBlock{{
					ID:   "recovered-visible-model-text",
					Kind: types.BlockSection,
					Text: text,
				}},
			},
			Mode:        "content_json_visible_string_salvage",
			Lossless:    false,
			Diagnostics: diagnostics,
		}, true
	}
	return AnswerDocumentTextRecovery{}, false
}

type malformedAnswerVisibleFragment struct {
	field string
	value string
}

const (
	maxMalformedAnswerVisibleFragments = 96
	maxMalformedAnswerVisibleRunes     = 200000
)

// recoverMalformedAnswerDocumentVisibleStrings extracts only values carried
// by visible answer fields after all structural JSON recovery has failed.
// This is intentionally a display salvage, not a parser and not a validator:
// it cannot create citations, typed claims, conclusions, or a successful tool
// emit.  Exact schema keys are the only selectors, so answer/user keywords
// never affect the lane.
func recoverMalformedAnswerDocumentVisibleStrings(raw string) (string, []string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !textContainsAnswerDocumentBlocksMarker(raw) {
		return "", nil, false
	}
	fragments := scanMalformedAnswerVisibleFragments(raw)
	mode := "plain_keys"
	if len(fragments) == 0 && strings.Contains(raw, `\"blocks\"`) {
		// Some providers stringify the whole tool argument and then truncate it.
		// A fully valid quoted carrier was handled by the structural lanes above;
		// this bounded replacement is only a final attempt to expose intact text.
		fragments = scanMalformedAnswerVisibleFragments(strings.ReplaceAll(raw, `\"`, `"`))
		mode = "escaped_keys"
	}
	if len(fragments) == 0 {
		return "", nil, false
	}
	var b strings.Builder
	seen := map[string]bool{}
	shownRunes := 0
	truncated := false
	for _, fragment := range fragments {
		value := strings.TrimSpace(fragment.value)
		if value == "" || seen[value] || types.IsPlaceholderLikeModelDraft(value) {
			continue
		}
		seen[value] = true
		remaining := maxMalformedAnswerVisibleRunes - shownRunes
		if remaining <= 0 {
			truncated = true
			break
		}
		runes := []rune(value)
		if len(runes) > remaining {
			value = string(runes[:remaining])
			truncated = true
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		switch fragment.field {
		case "title":
			b.WriteString("### ")
			b.WriteString(value)
		case "label":
			b.WriteString("- **")
			b.WriteString(value)
			b.WriteString("**")
		case "cells", "columns", "caveats":
			b.WriteString("- ")
			b.WriteString(value)
		default:
			b.WriteString(value)
		}
		shownRunes += len([]rune(value))
		if truncated {
			break
		}
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return "", nil, false
	}
	diagnostics := []string{
		"malformed answer_document JSON could not be structurally recovered",
		fmt.Sprintf("visible string salvage mode=%s fragments=%d", mode, len(fragments)),
	}
	if truncated {
		diagnostics = append(diagnostics, fmt.Sprintf("visible string salvage bounded at %d runes", maxMalformedAnswerVisibleRunes))
	}
	return text, diagnostics, true
}

func scanMalformedAnswerVisibleFragments(raw string) []malformedAnswerVisibleFragment {
	visibleScalar := map[string]bool{"title": true, "text": true, "label": true}
	visibleArrays := map[string]bool{"cells": true, "columns": true, "caveats": true}
	out := make([]malformedAnswerVisibleFragment, 0, 12)
	for i := 0; i < len(raw) && len(out) < maxMalformedAnswerVisibleFragments; {
		if raw[i] != '"' {
			i++
			continue
		}
		key, keyEnd, ok := scanLooseJSONString(raw, i)
		if !ok {
			i++
			continue
		}
		pos := skipJSONSpace(raw, keyEnd)
		if pos >= len(raw) || raw[pos] != ':' {
			i = keyEnd
			continue
		}
		pos = skipJSONSpace(raw, pos+1)
		if visibleScalar[key] && pos < len(raw) && raw[pos] == '"' {
			value, end, valueOK := scanLooseJSONString(raw, pos)
			if valueOK {
				out = append(out, malformedAnswerVisibleFragment{field: key, value: value})
				i = end
				continue
			}
		}
		if visibleArrays[key] && pos < len(raw) && raw[pos] == '[' {
			values, end := scanLooseJSONStringArray(raw, pos, maxMalformedAnswerVisibleFragments-len(out))
			for _, value := range values {
				out = append(out, malformedAnswerVisibleFragment{field: key, value: value})
			}
			if end > pos {
				i = end
				continue
			}
		}
		// Skip an ordinary quoted value so strings inside visible prose cannot
		// impersonate schema keys.  Broken values advance one byte and remain
		// recoverable by later real key/value pairs.
		if pos < len(raw) && raw[pos] == '"' {
			if _, end, valueOK := scanLooseJSONString(raw, pos); valueOK {
				i = end
				continue
			}
		}
		i = keyEnd
	}
	return out
}

func scanLooseJSONString(raw string, start int) (string, int, bool) {
	if start < 0 || start >= len(raw) || raw[start] != '"' {
		return "", start, false
	}
	escaped := false
	for i := start + 1; i < len(raw); i++ {
		if escaped {
			escaped = false
			continue
		}
		switch raw[i] {
		case '\\':
			escaped = true
		case '"':
			quoted := raw[start : i+1]
			var value string
			if err := json.Unmarshal([]byte(quoted), &value); err != nil {
				return "", i + 1, false
			}
			return value, i + 1, true
		}
	}
	return "", len(raw), false
}

func scanLooseJSONStringArray(raw string, start, limit int) ([]string, int) {
	if start < 0 || start >= len(raw) || raw[start] != '[' || limit <= 0 {
		return nil, start
	}
	var out []string
	for i := start + 1; i < len(raw) && len(out) < limit; {
		i = skipJSONSpace(raw, i)
		if i >= len(raw) {
			return out, i
		}
		if raw[i] == ']' {
			return out, i + 1
		}
		if raw[i] != '"' {
			i++
			continue
		}
		value, end, ok := scanLooseJSONString(raw, i)
		if !ok {
			return out, end
		}
		out = append(out, value)
		i = end
	}
	return out, len(raw)
}

func skipJSONSpace(raw string, i int) int {
	for i < len(raw) {
		switch raw[i] {
		case ' ', '\t', '\r', '\n':
			i++
		default:
			return i
		}
	}
	return i
}

func nestedAnswerDocumentPayloads(raw json.RawMessage) []json.RawMessage {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || len(obj) == 0 {
		return nil
	}
	var out []json.RawMessage
	add := func(candidate json.RawMessage) {
		candidate = bytes.TrimSpace(candidate)
		if len(candidate) == 0 || !bytes.Contains(candidate, []byte(`"blocks"`)) {
			return
		}
		for _, existing := range out {
			if bytes.Equal(existing, candidate) {
				return
			}
		}
		out = append(out, candidate)
	}
	for _, key := range []string{"arguments", "params", "input"} {
		if v, ok := obj[key]; ok {
			addNestedPayloadValue(v, add)
		}
	}
	if fn, ok := obj["function"]; ok {
		var fnObj map[string]json.RawMessage
		if err := json.Unmarshal(fn, &fnObj); err == nil {
			for _, key := range []string{"arguments", "params", "input"} {
				if v, ok := fnObj[key]; ok {
					addNestedPayloadValue(v, add)
				}
			}
		}
	}
	return out
}

func addNestedPayloadValue(raw json.RawMessage, add func(json.RawMessage)) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return
	}
	switch trimmed[0] {
	case '{', '[':
		add(trimmed)
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return
		}
		if strings.Contains(s, `"blocks"`) {
			add(json.RawMessage(strings.TrimSpace(s)))
		}
	}
}

func decodeRecoveredAnswerDocumentV2(raw json.RawMessage, mode string) (AnswerDocumentTextRecovery, error) {
	var p emitAnswerDocumentV2Params
	if err := json.Unmarshal(raw, &p); err != nil {
		return AnswerDocumentTextRecovery{}, err
	}
	if len(p.Blocks) == 0 {
		return AnswerDocumentTextRecovery{}, fmt.Errorf("blocks[] is empty")
	}
	doc := &types.AnswerDocumentV2{
		DocumentModel:         "v2",
		Citations:             convertEmitCitationsToTyped(p.Citations),
		ExactResolution:       p.ExactResolution,
		MissingRequestedRoles: p.MissingRequestedRoles,
		Caveats:               p.Caveats,
		Snippets:              convertEmitCodeSnippetsToTyped(p.Snippets),
	}
	// entry.modelIndex, never the loop position: validation-error
	// fieldPaths must name the block's index in the MODEL's own
	// blocks[] array, not its system-shifted post-split position.
	for _, entry := range splitFusedDiagramBlocks("emit_answer_document text-recovery", p.Blocks) {
		blk, err := NormalizeEmitAnswerBlock(entry.raw, fmt.Sprintf("blocks[%d]", entry.modelIndex))
		if err != nil {
			return AnswerDocumentTextRecovery{}, err
		}
		doc.Blocks = append(doc.Blocks, blk)
	}
	// Identical-duplicate dedup mirrors the main emit sequence — the
	// split's duplicate memo relies on it to collapse the visible
	// copies of a stutter pair so lossless recovery is not failed by
	// a duplicate-id reject the emit path would absorb.
	if changed, fields := normalizeAnswerDocumentBlockIDSurface(doc); changed {
		logging.Warning("[emit_answer_document text-recovery] id duplicate(s) normalized via transactional tolerance: %s",
			strings.Join(fields, ", "))
	}
	if err := validateMergedV2Doc(doc); err != nil {
		return AnswerDocumentTextRecovery{}, err
	}
	return AnswerDocumentTextRecovery{Document: doc, Mode: mode, Lossless: true}, nil
}

func visibleAnswerDocumentFromRaw(raw json.RawMessage, mode string) (AnswerDocumentTextRecovery, bool) {
	var probe struct {
		Blocks                []json.RawMessage                  `json:"blocks"`
		Citations             []types.Citation                   `json:"citations,omitempty"`
		ExactResolution       *types.AnswerExactResolution       `json:"exact_resolution,omitempty"`
		MissingRequestedRoles []types.AnswerMissingRequestedRole `json:"missing_requested_roles,omitempty"`
		Caveats               []string                           `json:"caveats,omitempty"`
		Snippets              []types.CodeSnippet                `json:"snippets,omitempty"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil || len(probe.Blocks) == 0 {
		return AnswerDocumentTextRecovery{}, false
	}
	doc := &types.AnswerDocumentV2{
		DocumentModel:         "v2",
		Citations:             probe.Citations,
		ExactResolution:       probe.ExactResolution,
		MissingRequestedRoles: probe.MissingRequestedRoles,
		Caveats:               probe.Caveats,
		Snippets:              probe.Snippets,
	}
	attachments := make([]types.AnswerDisplayAttachment, 0)
	for i, rawBlock := range probe.Blocks {
		if blk, ok := visibleAnswerBlockFromRaw(rawBlock, i); ok {
			doc.Blocks = append(doc.Blocks, blk)
			continue
		}
		if att, ok := displayAttachmentFromMalformedCandidate(string(rawBlock)); ok {
			attachments = appendRecoveredAttachment(attachments, att)
		}
	}
	if len(doc.Blocks) == 0 && len(attachments) == 0 {
		return AnswerDocumentTextRecovery{}, false
	}
	_ = validateRecoveredVisibleDoc(doc)
	return AnswerDocumentTextRecovery{
		Document:    doc,
		Attachments: attachments,
		Mode:        mode,
		Lossless:    false,
	}, true
}

func visibleAnswerBlockFromRaw(raw json.RawMessage, idx int) (types.AnswerBlock, bool) {
	var block emitAnswerBlockV2
	if err := json.Unmarshal(raw, &block); err != nil {
		return types.AnswerBlock{}, false
	}
	id := strings.TrimSpace(block.ID)
	if id == "" {
		id = fmt.Sprintf("recovered-block-%d", idx+1)
	}
	kind := types.AnswerBlockKind(strings.TrimSpace(block.Kind))
	if !types.IsValidAnswerBlockKind(kind) {
		kind = types.BlockSection
	}
	if block.Diagram != nil && strings.TrimSpace(block.Diagram.Body) != "" {
		kind = types.BlockDiagram
	}
	blk := types.AnswerBlock{
		ID:    id,
		Kind:  kind,
		Title: block.Title,
		Text:  block.Text,
		SourceInventoryFamily: types.SourceInventorySurfaceTermKey(
			block.SourceInventoryFamily,
		),
		Columns:  normalizeTableStringSlice(block.Columns),
		FacetIDs: block.FacetIDs,
	}
	if role, ok := types.NormalizeSurfaceRole(block.SurfaceRole); ok {
		blk.SurfaceRole = role
	}
	if verdict, ok := types.NormalizeErrorGranularityVerdict(block.ErrorGranularityVerdict); ok && kind == types.BlockDecision {
		blk.ErrorGranularityVerdict = verdict
	}
	if verdict, ok := types.NormalizeCurrentStatusVerdict(block.CurrentStatusVerdict); ok && kind == types.BlockDecision {
		blk.CurrentStatusVerdict = verdict
	}
	if disclosure, ok := types.NormalizeScopeDisclosureKind(block.ScopeDisclosure); ok {
		blk.ScopeDisclosure = disclosure
	}
	for _, item := range block.Items {
		candidateRole, _ := types.NormalizeAnswerCandidateRole(item.CandidateRole)
		cells := normalizeTableStringSlice(item.Cells)
		if strings.TrimSpace(item.Label) == "" && strings.TrimSpace(item.Text) == "" && len(cells) == 0 {
			continue
		}
		blk.Items = append(blk.Items, types.AnswerBlockItem{
			ID:            item.ID,
			Label:         item.Label,
			Text:          item.Text,
			Cells:         cells,
			CandidateRole: candidateRole,
			CitationRef:   citationRefFromWire(item.CitationRef),
		})
	}
	if block.Diagram != nil && strings.TrimSpace(block.Diagram.Body) != "" {
		blk.Diagram = &types.AnswerDiagramBlock{
			Kind:     types.DiagramKind(block.Diagram.Kind),
			Language: block.Diagram.Language,
			Body:     block.Diagram.Body,
		}
	}
	if blk.Kind == types.BlockDiagram && blk.Diagram == nil {
		if att, ok := displayAttachmentFromText(blk.Text); ok {
			blk.Diagram = &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: att.Language, Body: att.Body}
			blk.Text = ""
		} else {
			blk.Kind = types.BlockSection
		}
	}
	if blk.Kind == types.BlockSummary || blk.Kind == types.BlockSection || blk.Kind == types.BlockCaveat ||
		blk.Kind == types.BlockScalar || blk.Kind == types.BlockDecision || blk.Kind == types.BlockTable {
		if strings.TrimSpace(blk.Text) != "" || strings.TrimSpace(blk.Title) != "" || len(blk.Columns) > 0 || len(blk.Items) > 0 || blk.Diagram != nil {
			return blk, true
		}
		return types.AnswerBlock{}, false
	}
	if len(blk.Items) > 0 || strings.TrimSpace(blk.Text) != "" || blk.Diagram != nil || strings.TrimSpace(blk.Title) != "" {
		return blk, true
	}
	return types.AnswerBlock{}, false
}

func validateRecoveredVisibleDoc(doc *types.AnswerDocumentV2) error {
	if doc == nil {
		return fmt.Errorf("recovered doc is nil")
	}
	seen := make(map[string]int, len(doc.Blocks))
	for i := range doc.Blocks {
		id := strings.TrimSpace(doc.Blocks[i].ID)
		if id == "" {
			id = fmt.Sprintf("recovered-block-%d", i+1)
			doc.Blocks[i].ID = id
		}
		if prev, ok := seen[id]; ok {
			id = fmt.Sprintf("%s-%d", id, i+1)
			doc.Blocks[i].ID = id
			doc.Blocks[prev].ID = strings.TrimSpace(doc.Blocks[prev].ID)
		}
		seen[id] = i
		if !types.IsValidAnswerBlockKind(doc.Blocks[i].Kind) {
			doc.Blocks[i].Kind = types.BlockSection
		}
		if doc.Blocks[i].Kind != types.BlockDiagram {
			doc.Blocks[i].Diagram = nil
		}
	}
	return nil
}
