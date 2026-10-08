package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Visible payload is not forward-compatible metadata. These exact schema
// fields have an owner (block, item, diagram or snippet); at document scope or
// outside any recovered block that owner cannot be inferred from proximity.
// Never inspect the prose values, or descend into unknown metadata values.
var answerVisibleOwnedFields = stringSet("title", "text", "caveat", "columns", "items", "diagram", "label", "cells", "body", "code")

func answerDocumentPayloadOwnershipPaths(raw json.RawMessage, profile answerDocumentFieldQuarantineProfile) []string {
	var root map[string]json.RawMessage
	var paths []string
	if json.Unmarshal(raw, &root) != nil {
		// The malformed-envelope pre-parser uses the same check before handing a
		// reconstructed document to Execute. Do not erase an orphan first.
		if body, _, _, ok := isolateBlocksStringBody(raw); ok {
			paths = append(paths, unownedStringBlockFields(body, "blocks")...)
		}
		// Preserve document-sibling ownership too. The legacy malformed
		// envelope isolator intentionally returns only the blocks value.
		// Walking the root members prevents its reconstruction from hiding a
		// misplaced visible sibling after an otherwise complete blocks array.
		if outer := strings.TrimSpace(string(raw)); strings.HasPrefix(outer, "{") {
			paths = append(paths, unownedStringBlockFields(outer[1:], "$")...)
		}
		return uniqueSortedOwnershipPaths(paths)
	}
	for field := range root {
		if !profile.TopLevelAllowed[field] && answerVisibleOwnedFields[field] {
			paths = append(paths, "$."+field)
		}
	}
	for _, field := range profile.BlockArrayFields {
		var body string
		if json.Unmarshal(root[field], &body) == nil {
			paths = append(paths, unownedStringBlockFields(body, field)...)
		}
	}
	return uniqueSortedOwnershipPaths(paths)
}

func uniqueSortedOwnershipPaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if !seen[path] {
			seen[path] = true
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// This is a recovery-only lexical walk, not a second JSON parser. Complete
// blocks and complete field values are skipped atomically. Quoted JSON examples
// remain strings, while unknown metadata (including nested text/items keys) is
// skipped as one value. Broken candidate blocks remain the existing recovery
// pipeline's responsibility; this check does not guess through broken quotes.
func unownedStringBlockFields(body, path string) []string {
	var native []json.RawMessage
	if json.Unmarshal([]byte(body), &native) == nil {
		return nil
	}
	var paths []string
	var preceding map[string]json.RawMessage
	precedingTail := 0
	for i := 0; i < len(body); {
		if body[i] == '{' {
			end, ok := balancedJSONValueEnd(body, i)
			if !ok {
				break
			}
			var object map[string]json.RawMessage
			if json.Unmarshal([]byte(body[i:end]), &object) != nil {
				break
			}
			if _, block := blockKindFromObject(object); block {
				preceding = object
				precedingTail = end
				i = end
				continue
			}
		}
		if body[i] != '"' {
			i++
			continue
		}
		end := findJSONStringEnd(body, i)
		if end < 0 {
			break
		}
		var key string
		if json.Unmarshal([]byte(body[i:end+1]), &key) != nil {
			break
		}
		cursor := end + 1
		for cursor < len(body) && isAnswerDocJSONSpace(body[cursor]) {
			cursor++
		}
		if cursor == len(body) || body[cursor] != ':' {
			i = end + 1
			continue
		}
		cursor++
		decoder := json.NewDecoder(strings.NewReader(body[cursor:]))
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			break
		}
		if answerVisibleOwnedFields[key] {
			// Preserve the existing unambiguous adjacent annotation repair. It
			// recognizes only its narrow annotation schema, never table/list/
			// diagram payloads. Equal values prove this exact field was retained.
			owned := false
			if key == "title" && preceding != nil && preceding[key] == nil {
				if merged, ok := mergeTrailingAnswerBlockAnnotations(preceding, body[precedingTail:]); ok {
					var object map[string]json.RawMessage
					if json.Unmarshal(merged, &object) == nil {
						owned = equalOwnershipJSON(object[key], value)
					}
				}
			}
			if !owned {
				paths = append(paths, fmt.Sprintf("%s.$unowned.%s", path, key))
			}
		}
		i = cursor + int(decoder.InputOffset())
	}
	return paths
}

func equalOwnershipJSON(a, b json.RawMessage) bool {
	var left, right bytes.Buffer
	return len(a) > 0 && json.Compact(&left, a) == nil && json.Compact(&right, b) == nil && bytes.Equal(left.Bytes(), right.Bytes())
}

func recoverySiblingValueEnd(body string, start int) (int, bool) {
	end := findJSONStringEnd(body, start)
	if end < 0 {
		return 0, false
	}
	cursor := end + 1
	for cursor < len(body) && isAnswerDocJSONSpace(body[cursor]) {
		cursor++
	}
	if cursor == len(body) || body[cursor] != ':' {
		return 0, false
	}
	cursor++
	decoder := json.NewDecoder(strings.NewReader(body[cursor:]))
	var value json.RawMessage
	if decoder.Decode(&value) != nil {
		return 0, false
	}
	return cursor + int(decoder.InputOffset()), true
}

func answerDocumentPayloadOwnershipRepair(paths []string, profile answerDocumentFieldQuarantineProfile) *types.ToolRepair {
	return &types.ToolRepair{
		Code: "answer_doc_visible_payload_ownership", Fields: append([]string(nil), profile.BlockArrayFields...),
		Hint:     "Visible answer fields are outside their owning block/item/diagram. Preserve all values and the valid blocks; put each field inside the exact intended owner using the published native JSON schema. Do not guess that a trailing field belongs to the last block. If an addressable draft exists, use a local patch for the affected block; otherwise resubmit the complete document. Original payload is retained for repair.",
		Metadata: map[string]string{"unowned_field_paths": strings.Join(paths, ",")},
	}
}

func answerDocumentOwnershipAttachment(raw json.RawMessage) types.AnswerDisplayAttachment {
	att := types.AnswerDisplayAttachment{
		Kind: types.AnswerDisplayAttachmentText, Body: string(raw),
		Source: "emit_answer_document.unowned_visible_payload",
		Reason: "original model payload retained: visible fields have unresolved structured ownership",
	}
	att.Hash = answerDisplayAttachmentHash(att.Kind, att.Language, att.Body)
	return att
}

func preserveAnswerDocumentOwnershipDraft(ctx *types.BusContext, raw json.RawMessage) {
	if ctx == nil || ctx.Mutable == nil {
		return
	}
	ctx.Mutable.SetAnswerDisplayAttachments(appendRecoveredAttachment(ctx.Mutable.AnswerDisplayAttachments(), answerDocumentOwnershipAttachment(raw)))
	// Keep valid model-authored blocks available for a local correction, but
	// never publish the partial recovered structure as a successful answer.
	if rec, ok := recoverAnswerDocumentV2FromRawCandidate(raw); ok && rec.Document != nil && types.ValidateAnswerDocumentPatchBaseIdentity(rec.Document) == nil {
		rememberRejectedAnswerDocumentDraft(ctx, rec.Document)
	}
}

// Count structural candidate objects, not occurrences of "kind" in prose.
// Only direct kind members of outer candidate objects count; nested item
// objects and document-sibling metadata values cannot inflate the loss test.
// An unfinished/missing-id candidate still counts once its kind was read.
func countRecoveryBlockObjects(body string) int {
	type candidate struct{ kind bool }
	var stack []candidate
	count := 0
	for i := 0; i < len(body); {
		switch body[i] {
		case '{':
			stack = append(stack, candidate{})
		case '}':
			if len(stack) == 1 && stack[0].kind {
				count++
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case '"':
			end := findJSONStringEnd(body, i)
			if end < 0 {
				i = len(body)
				continue
			}
			var key string
			if json.Unmarshal([]byte(body[i:end+1]), &key) != nil {
				i = end + 1
				continue
			}
			cursor := end + 1
			for cursor < len(body) && isAnswerDocJSONSpace(body[cursor]) {
				cursor++
			}
			if cursor < len(body) && body[cursor] == ':' {
				cursor++
				decoder := json.NewDecoder(strings.NewReader(body[cursor:]))
				var value json.RawMessage
				if decoder.Decode(&value) == nil && (len(stack) == 0 || (len(stack) == 1 && (key == "id" || key == "kind"))) {
					if len(stack) == 1 {
						if key == "kind" {
							_, stack[0].kind = blockKindFromObject(map[string]json.RawMessage{"kind": value})
						}
					}
					i = cursor + int(decoder.InputOffset())
					continue
				}
			}
			i = end + 1
			continue
		}
		i++
	}
	if len(stack) > 0 && stack[0].kind {
		count++
	}
	return count
}
