package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/types"
)

type jsonFieldAlias struct {
	Alias     string
	Canonical string
}

type jsonEnumFieldNormalizer struct {
	Field     string
	Normalize func(string) (string, bool)
}

var answerClaimUseFieldAliases = []jsonFieldAlias{
	{Alias: "claimForm", Canonical: "claim_form"},
	{Alias: "facetId", Canonical: "facet_id"},
	{Alias: "evidenceId", Canonical: "evidence_id"},
}

var answerEdgeAnchorFieldAliases = []jsonFieldAlias{
	{Alias: "fromNode", Canonical: "from_node"},
	{Alias: "toNode", Canonical: "to_node"},
	{Alias: "relationKind", Canonical: "relation_kind"},
	{Alias: "claimForm", Canonical: "claim_form"},
}

var answerClaimUseEnumNormalizers = []jsonEnumFieldNormalizer{
	{Field: "claim_form", Normalize: normalizeClaimFormJSONValue},
}

var answerEdgeAnchorEnumNormalizers = []jsonEnumFieldNormalizer{
	{Field: "relation_kind", Normalize: normalizeDiagramRelationJSONValue},
	{Field: "claim_form", Normalize: normalizeClaimFormJSONValue},
}

// repairAnswerBlockAnnotationShape normalizes known annotation carriers that
// local models often emit in SDK-style camelCase. This is intentionally a
// schema-shape repair, not a semantic repair: only known field aliases are
// moved, enum values are accepted only after validating against the canonical
// enum set, and conflicting alias/canonical pairs are left untouched so the
// strict decoder still rejects them.
func repairAnswerBlockAnnotationShape(blkObj map[string]json.RawMessage) ([]string, bool) {
	if len(blkObj) == 0 {
		return nil, false
	}
	var paths []string
	if fields, ok := repairAnnotationObjectArray(blkObj, "claim_uses", answerClaimUseFieldAliases, answerClaimUseEnumNormalizers); ok {
		paths = append(paths, fields...)
	}
	if fields, ok := repairAnnotationObjectArray(blkObj, "edge_anchors", answerEdgeAnchorFieldAliases, answerEdgeAnchorEnumNormalizers); ok {
		paths = append(paths, fields...)
	}
	return paths, len(paths) > 0
}

func repairAnnotationObjectArray(obj map[string]json.RawMessage, field string, aliases []jsonFieldAlias, enumNormalizers []jsonEnumFieldNormalizer) ([]string, bool) {
	rawField, ok := obj[field]
	if !ok {
		return nil, false
	}
	rawField = bytes.TrimSpace(rawField)
	if len(rawField) == 0 || rawField[0] != '[' {
		return nil, false
	}
	var items []json.RawMessage
	if err := json.Unmarshal(rawField, &items); err != nil {
		return nil, false
	}
	var paths []string
	changedAny := false
	for i, rawItem := range items {
		rawItem = bytes.TrimSpace(rawItem)
		if len(rawItem) == 0 || rawItem[0] != '{' {
			continue
		}
		var item map[string]json.RawMessage
		if err := json.Unmarshal(rawItem, &item); err != nil {
			continue
		}
		itemChanged := false
		for _, alias := range aliases {
			if repairJSONFieldAlias(item, alias) {
				paths = append(paths, fmt.Sprintf("%s[%d].%s->%s", field, i, alias.Alias, alias.Canonical))
				itemChanged = true
			}
		}
		for _, normalizer := range enumNormalizers {
			if repairJSONStringEnumField(item, normalizer.Field, normalizer.Normalize) {
				paths = append(paths, fmt.Sprintf("%s[%d].%s", field, i, normalizer.Field))
				itemChanged = true
			}
		}
		if itemChanged {
			items[i] = mustMarshal(item)
			changedAny = true
		}
	}
	if !changedAny {
		return nil, false
	}
	obj[field] = mustMarshal(items)
	return paths, true
}

func repairJSONFieldAlias(item map[string]json.RawMessage, alias jsonFieldAlias) bool {
	rawAlias, ok := item[alias.Alias]
	if !ok {
		return false
	}
	if rawCanonical, exists := item[alias.Canonical]; exists {
		if bytes.Equal(bytes.TrimSpace(rawAlias), bytes.TrimSpace(rawCanonical)) {
			delete(item, alias.Alias)
			return true
		}
		return false
	}
	item[alias.Canonical] = rawAlias
	delete(item, alias.Alias)
	return true
}

func repairJSONStringEnumField(item map[string]json.RawMessage, field string, normalize func(string) (string, bool)) bool {
	raw, ok := item[field]
	if !ok {
		return false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	normalized, ok := normalize(value)
	if !ok || normalized == value {
		return false
	}
	item[field] = mustMarshal(normalized)
	return true
}

func normalizeClaimFormJSONValue(value string) (string, bool) {
	candidate := normalizeFlexibleEnumToken(value)
	if candidate == "" {
		return "", false
	}
	claim := types.ClaimForm(candidate)
	if !claim.IsValid() {
		return "", false
	}
	return string(claim), true
}

func normalizeDiagramRelationJSONValue(value string) (string, bool) {
	candidate := normalizeFlexibleEnumToken(value)
	if candidate == "" {
		return "", false
	}
	relation := types.DiagramRelationKind(candidate)
	if !relation.IsValid() {
		return "", false
	}
	return string(relation), true
}

func normalizeFlexibleEnumToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value) + 4)
	prevWasUnderscore := false
	prevWasLowerOrDigit := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if unicode.IsUpper(r) && b.Len() > 0 && prevWasLowerOrDigit && !prevWasUnderscore {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			prevWasUnderscore = false
			prevWasLowerOrDigit = unicode.IsLower(r) || unicode.IsDigit(r)
			continue
		}
		if b.Len() > 0 && !prevWasUnderscore {
			b.WriteByte('_')
			prevWasUnderscore = true
		}
		prevWasLowerOrDigit = false
	}
	return strings.Trim(b.String(), "_")
}
