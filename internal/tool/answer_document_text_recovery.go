package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

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
	return AnswerDocumentTextRecovery{}, false
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
	for i, rawBlock := range p.Blocks {
		blk, err := NormalizeEmitAnswerBlock(rawBlock, fmt.Sprintf("blocks[%d]", i))
		if err != nil {
			return AnswerDocumentTextRecovery{}, err
		}
		doc.Blocks = append(doc.Blocks, blk)
	}
	canonicalizeSummaryLeadBlock(doc)
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
	canonicalizeSummaryLeadBlock(doc)
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
		ID:       id,
		Kind:     kind,
		Title:    block.Title,
		Text:     block.Text,
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
			CitationRef:   int(item.CitationRef),
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
