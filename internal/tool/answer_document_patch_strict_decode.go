package tool

import (
	"encoding/json"
	"sort"
	"strings"
)

// answerDocumentPatchMisplacedHintsForSchema keeps wrong-container repairs in
// the grammar of the patch operation that actually rejected the payload. The
// full-emit and patch tools share block payloads, but patch-only atomic
// operations add other valid homes for fields such as from_node. A static
// full-emit hint must not redirect a patch retry away from its live operation.
//
// The override is deliberately fail-closed: the rejected field must occur
// under exactly one top-level operation in the native JSON payload, and the
// current dispatch schema must publish at least one same-operation path for
// that exact field. No spelling similarity or answer prose participates.
func answerDocumentPatchMisplacedHintsForSchema(err error, raw, schema json.RawMessage) []MisplacedFieldHint {
	field := extractUnknownFieldName(err)
	if field == "" {
		return answerDocumentV2MisplacedHints
	}
	owner, invalidContainers, ok := patchRawFieldOwnerAndContainers(raw, field)
	if !ok {
		return answerDocumentV2MisplacedHints
	}
	validPaths := patchSchemaFieldPaths(schema, field, owner)
	if len(validPaths) == 0 {
		return answerDocumentV2MisplacedHints
	}
	hints := append([]MisplacedFieldHint(nil), answerDocumentV2MisplacedHints...)
	for i := range hints {
		if hints[i].Field != field || hints[i].CanonicalName != "" {
			continue
		}
		hints[i].ContainerNames = invalidContainers
		hints[i].CorrectPaths = validPaths
		return hints
	}
	return answerDocumentV2MisplacedHints
}

func patchRawFieldOwnerAndContainers(raw json.RawMessage, field string) (string, []string, bool) {
	var root map[string]any
	if len(raw) == 0 || field == "" || json.Unmarshal(raw, &root) != nil {
		return "", nil, false
	}
	owners := make(map[string]bool)
	containers := make(map[string]bool)
	for owner, value := range root {
		patchCollectRawFieldContainers(value, owner, field, owner, owners, containers)
	}
	if len(owners) != 1 || len(containers) == 0 {
		return "", nil, false
	}
	var owner string
	for candidate := range owners {
		owner = candidate
	}
	out := make([]string, 0, len(containers))
	for container := range containers {
		out = append(out, container)
	}
	sort.Strings(out)
	return owner, out, true
}

func patchCollectRawFieldContainers(value any, path, field, owner string, owners, containers map[string]bool) {
	switch node := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(node))
		for key := range node {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if key == field {
				owners[owner] = true
				containers[path] = true
			}
			patchCollectRawFieldContainers(node[key], patchJSONPathJoin(path, key), field, owner, owners, containers)
		}
	case []any:
		for _, child := range node {
			patchCollectRawFieldContainers(child, path+"[i]", field, owner, owners, containers)
		}
	}
}

func patchSchemaFieldPaths(schema json.RawMessage, field, owner string) []string {
	var root map[string]any
	if len(schema) == 0 || field == "" || owner == "" || json.Unmarshal(schema, &root) != nil {
		return nil
	}
	properties, _ := root["properties"].(map[string]any)
	ownerNode, ok := properties[owner]
	if !ok {
		return nil
	}
	seen := make(map[string]bool)
	patchCollectSchemaFieldPaths(ownerNode, owner, field, seen)
	out := make([]string, 0, len(seen))
	for path := range seen {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func patchCollectSchemaFieldPaths(value any, path, field string, out map[string]bool) {
	node, ok := value.(map[string]any)
	if !ok {
		return
	}
	if properties, ok := node["properties"].(map[string]any); ok {
		keys := make([]string, 0, len(properties))
		for key := range properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childPath := patchJSONPathJoin(path, key)
			if key == field {
				out[childPath] = true
			}
			patchCollectSchemaFieldPaths(properties[key], childPath, field, out)
		}
	}
	if items, ok := node["items"]; ok {
		patchCollectSchemaFieldPaths(items, path+"[i]", field, out)
	}
	for _, keyword := range []string{"oneOf", "anyOf", "allOf"} {
		branches, _ := node[keyword].([]any)
		for _, branch := range branches {
			patchCollectSchemaFieldPaths(branch, path, field, out)
		}
	}
}

func patchJSONPathJoin(base, field string) string {
	if strings.TrimSpace(base) == "" {
		return field
	}
	return base + "." + field
}
