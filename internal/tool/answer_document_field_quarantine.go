package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type answerDocumentFieldQuarantineProfile struct {
	TopLevelAllowed  map[string]bool
	BlockArrayFields []string
}

var answerDocumentFullEmitQuarantineProfile = answerDocumentFieldQuarantineProfile{
	TopLevelAllowed: stringSet(
		"document_model",
		"blocks",
		"citations",
		"exact_resolution",
		"missing_requested_roles",
		"caveats",
		"snippets",
		"trace_finding",
		"trace_root_causes",
	),
	BlockArrayFields: []string{"blocks"},
}

var answerDocumentPatchQuarantineProfile = answerDocumentFieldQuarantineProfile{
	TopLevelAllowed: stringSet(
		"unchanged_block_ids",
		"replace_blocks",
		"add_blocks",
		"remove_block_ids",
		"replace_citations",
		"append_citations",
		"replace_exact_resolution",
		"replace_missing_requested_roles",
		"replace_caveats",
		"replace_snippets",
		"replace_trace_finding",
		"replace_trace_root_causes",
	),
	BlockArrayFields: []string{"replace_blocks", "add_blocks"},
}

var (
	answerDocumentBlockAllowedFields = stringSet(
		"id",
		"kind",
		"title",
		"text",
		"caveat",
		"error_granularity_verdict",
		"current_status_verdict",
		"trace_causal_claim_caliber",
		"scope_disclosure",
		"source_inventory_family",
		"columns",
		"items",
		"diagram",
		"claim_uses",
		"edge_anchors",
		"participant_boundaries",
		"relation_claims",
		"facet_ids",
		"surface_role",
	)
	answerDocumentItemAllowedFields = stringSet(
		"id",
		"label",
		"text",
		"cells",
		"candidate_role",
		"source_inventory_row_id",
		"citation_ref",
	)
	answerDocumentCitationAllowedFields = stringSet(
		"file",
		"line",
		"quote",
		"scope",
		"line_end",
		"section_path",
		"file_role_label",
		"crossfile_summary",
		"negative_pattern",
	)
	answerDocumentSnippetAllowedFields = stringSet(
		"file",
		"start_line",
		"end_line",
		"language",
		"code",
	)
	answerDocumentDiagramAllowedFields = stringSet(
		"kind",
		"language",
		"body",
	)
	answerDocumentClaimUseAllowedFields = stringSet(
		"facet_id",
		"evidence_id",
		"claim_form",
	)
	answerDocumentEdgeAnchorAllowedFields = stringSet(
		"from_node",
		"to_node",
		"relation_kind",
		"claim_form",
	)
	answerDocumentParticipantBoundaryAllowedFields = stringSet(
		"participant",
		"status",
	)
	answerDocumentRelationClaimAllowedFields = stringSet(
		"authority_id",
		"member_refs",
		"physical_relation",
		"addition",
		"subtotal_value",
		"subtotal_unit",
	)
	answerDocumentExactResolutionAllowedFields = stringSet(
		"status",
		"anchor",
		"context_mode",
	)
	answerDocumentMissingRoleAllowedFields = stringSet(
		"role",
		"label",
	)
)

func stringSet(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

// quarantineUnknownAnswerDocumentFields strips schema-unknown metadata from
// known answer-document containers before strict JSON decoding. This is a
// local schema compatibility layer: it never inspects user/model prose and it
// never invents answer content. Core payload fields still go through the same
// strict decoder and semantic validators after this pass.
func quarantineUnknownAnswerDocumentFields(raw json.RawMessage, profile answerDocumentFieldQuarantineProfile) (json.RawMessage, []string, bool) {
	if len(raw) == 0 || len(profile.TopLevelAllowed) == 0 {
		return raw, nil, false
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return raw, nil, false
	}
	var paths []string
	changed := quarantineObjectUnknownFields(root, profile.TopLevelAllowed, "$", &paths)
	for _, field := range profile.BlockArrayFields {
		if quarantineBlockArray(root, field, field, &paths) {
			changed = true
		}
	}
	for _, field := range []string{"citations", "replace_citations", "append_citations"} {
		if quarantineObjectArray(root, field, field, answerDocumentCitationAllowedFields, nil, &paths) {
			changed = true
		}
	}
	for _, field := range []string{"snippets", "replace_snippets"} {
		if quarantineObjectArray(root, field, field, answerDocumentSnippetAllowedFields, nil, &paths) {
			changed = true
		}
	}
	for _, field := range []string{"missing_requested_roles", "replace_missing_requested_roles"} {
		if quarantineObjectArray(root, field, field, answerDocumentMissingRoleAllowedFields, nil, &paths) {
			changed = true
		}
	}
	for _, field := range []string{"exact_resolution", "replace_exact_resolution"} {
		if quarantineObjectField(root, field, field, answerDocumentExactResolutionAllowedFields, nil, &paths) {
			changed = true
		}
	}
	if !changed {
		return raw, nil, false
	}
	sort.Strings(paths)
	out, err := json.Marshal(root)
	if err != nil {
		return raw, nil, false
	}
	return out, paths, true
}

// answerDocumentStructuralCarrierCorruptionPaths detects the narrow case in
// which a syntactically valid JSON object still carries a serialized JSON
// boundary in one of its property names, for example `"}, {"claim_uses`.
// Such a key is not ordinary forward-compatible metadata: it proves that two
// answer objects were fused and that accepting the surviving fields would
// publish only a fragment of the model's answer.
//
// This check reads JSON property names only. It never scans the user request,
// block text, item text, diagram body, or any other model-authored prose. Quote
// and object delimiters are used because they cannot occur in a schema field
// name generated by the answer-document carrier. Brackets are deliberately
// not included so advisory metadata keys such as `field[0]` remain eligible
// for the existing compatibility quarantine.
func answerDocumentStructuralCarrierCorruptionPaths(raw json.RawMessage) []string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var root interface{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil
	}
	const maxPaths = 8
	paths := make([]string, 0, 2)
	var walk func(interface{}, string)
	walk = func(value interface{}, path string) {
		if len(paths) >= maxPaths {
			return
		}
		switch typed := value.(type) {
		case map[string]interface{}:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				childPath := path + "." + key
				if strings.ContainsAny(key, "{}\"") {
					paths = append(paths, childPath)
					if len(paths) >= maxPaths {
						return
					}
				}
				walk(typed[key], childPath)
			}
		case []interface{}:
			for i, item := range typed {
				walk(item, fmt.Sprintf("%s[%d]", path, i))
				if len(paths) >= maxPaths {
					return
				}
			}
		}
	}
	walk(root, "$")
	return paths
}

func answerDocumentStructuralCarrierCorruptionRepair(paths []string) *types.ToolRepair {
	metadata := map[string]string{
		"corrupt_field_count": strconv.Itoa(len(paths)),
	}
	if len(paths) > 0 {
		metadata["corrupt_field_paths"] = strings.Join(paths, ",")
	}
	return &types.ToolRepair{
		Code:     types.ToolRepairCodeAnswerDocStructuralCarrierCorruption,
		Fields:   append([]string(nil), paths...),
		Metadata: metadata,
		Hint:     "Re-emit the complete answer_document as native JSON. Keep each blocks[] entry as one object; never serialize `}, {` or another object boundary into a property name. Preserve every visible answer block and citation from the rejected draft.",
	}
}

// answerDocumentHasTopLevelField is used only for load-bearing block metadata
// whose location cannot be quarantined safely. Unknown advisory metadata may be
// dropped by the compatibility layer, but silently dropping a top-level
// relation_claims field guarantees a later closure-authority failure and gives
// the model no indication that the field was placed on the wrong carrier.
// This is an exact JSON-key check; it never inspects answer prose.
func answerDocumentHasTopLevelField(raw json.RawMessage, field string) bool {
	if len(raw) == 0 || strings.TrimSpace(field) == "" {
		return false
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return false
	}
	_, ok := root[field]
	return ok
}

func quarantineBlockArray(root map[string]json.RawMessage, field, path string, paths *[]string) bool {
	return quarantineObjectArray(root, field, path, answerDocumentBlockAllowedFields, quarantineBlockObject, paths)
}

func quarantineBlockObject(obj map[string]json.RawMessage, path string, paths *[]string) bool {
	changed := false
	if raw, ok := obj["claim_use"]; ok {
		if claims, parsed := decodeItemClaimUseRaw(raw); parsed {
			existing, _ := decodeClaimUseArrayRaw(obj["claim_uses"])
			seen := make(map[string]bool, len(existing))
			for _, claim := range existing {
				seen[canonicalRawObjectKey(claim)] = true
			}
			for _, claim := range claims {
				key := canonicalRawObjectKey(claim)
				if seen[key] {
					continue
				}
				seen[key] = true
				existing = append(existing, claim)
			}
			obj["claim_uses"] = mustMarshal(existing)
			delete(obj, "claim_use")
			*paths = append(*paths, path+".claim_use->claim_uses")
			changed = true
		}
	}
	if quarantineObjectUnknownFields(obj, answerDocumentBlockAllowedFields, path, paths) {
		changed = true
	}
	if quarantineObjectArray(obj, "items", path+".items", answerDocumentItemAllowedFields, nil, paths) {
		changed = true
	}
	if quarantineObjectArray(obj, "claim_uses", path+".claim_uses", answerDocumentClaimUseAllowedFields, nil, paths) {
		changed = true
	}
	if quarantineObjectArray(obj, "edge_anchors", path+".edge_anchors", answerDocumentEdgeAnchorAllowedFields, nil, paths) {
		changed = true
	}
	if quarantineObjectArray(obj, "participant_boundaries", path+".participant_boundaries", answerDocumentParticipantBoundaryAllowedFields, nil, paths) {
		changed = true
	}
	if quarantineObjectArray(obj, "relation_claims", path+".relation_claims", answerDocumentRelationClaimAllowedFields, nil, paths) {
		changed = true
	}
	if quarantineObjectField(obj, "diagram", path+".diagram", answerDocumentDiagramAllowedFields, nil, paths) {
		changed = true
	}
	return changed
}

func quarantineObjectField(
	obj map[string]json.RawMessage,
	field string,
	path string,
	allowed map[string]bool,
	after func(map[string]json.RawMessage, string, *[]string) bool,
	paths *[]string,
) bool {
	raw, ok := obj[field]
	raw = bytes.TrimSpace(raw)
	if !ok || len(raw) == 0 || raw[0] != '{' {
		return false
	}
	var child map[string]json.RawMessage
	if err := json.Unmarshal(raw, &child); err != nil {
		return false
	}
	changed := false
	if after != nil && after(child, path, paths) {
		changed = true
	}
	if quarantineObjectUnknownFields(child, allowed, path, paths) {
		changed = true
	}
	if !changed {
		return false
	}
	obj[field] = mustMarshal(child)
	return true
}

func quarantineObjectArray(
	obj map[string]json.RawMessage,
	field string,
	path string,
	allowed map[string]bool,
	after func(map[string]json.RawMessage, string, *[]string) bool,
	paths *[]string,
) bool {
	raw, ok := obj[field]
	raw = bytes.TrimSpace(raw)
	if !ok || len(raw) == 0 || raw[0] != '[' {
		return false
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return false
	}
	changed := false
	for i, rawItem := range items {
		rawItem = bytes.TrimSpace(rawItem)
		if len(rawItem) == 0 || rawItem[0] != '{' {
			continue
		}
		var item map[string]json.RawMessage
		if err := json.Unmarshal(rawItem, &item); err != nil {
			continue
		}
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		itemChanged := false
		if after != nil && after(item, itemPath, paths) {
			itemChanged = true
		}
		if quarantineObjectUnknownFields(item, allowed, itemPath, paths) {
			itemChanged = true
		}
		if !itemChanged {
			continue
		}
		items[i] = mustMarshal(item)
		changed = true
	}
	if !changed {
		return false
	}
	obj[field] = mustMarshal(items)
	return true
}

func quarantineObjectUnknownFields(obj map[string]json.RawMessage, allowed map[string]bool, path string, paths *[]string) bool {
	changed := false
	for key := range obj {
		if allowed[key] {
			continue
		}
		if preserveUnknownAnswerDocumentFieldForStrictRepair(path, key) {
			continue
		}
		delete(obj, key)
		*paths = append(*paths, path+"."+key)
		changed = true
	}
	return changed
}

func preserveUnknownAnswerDocumentFieldForStrictRepair(path, key string) bool {
	if strings.Contains(path, ".claim_uses[") {
		switch key {
		case "citation_ref", "facet_ids", "from_node", "to_node", "relation_kind",
			"claimForm", "facetId", "evidenceId":
			return true
		}
	}
	if strings.Contains(path, ".edge_anchors[") {
		switch key {
		case "claimForm", "fromNode", "toNode", "relationKind":
			return true
		}
	}
	if strings.HasPrefix(path, "blocks[") ||
		strings.HasPrefix(path, "replace_blocks[") ||
		strings.HasPrefix(path, "add_blocks[") {
		switch key {
		case "value", "boolean", "literal", "decision", "rationale":
			return true
		}
	}
	return false
}
