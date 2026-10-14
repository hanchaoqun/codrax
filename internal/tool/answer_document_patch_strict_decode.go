package tool

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// failAnswerDocumentPatchStrictDecode keeps dispatch-specific repair metadata
// inside one registered reject helper. The executor resolves the raw selector
// before calling it, preserving the directly auditable reject-exit contract.
func failAnswerDocumentPatchStrictDecode(name string, now time.Time, err error, hints []MisplacedFieldHint, raw []byte, schema json.RawMessage) (types.ToolResult, error) {
	failed, decodeErr := failStrictDecode(name, now, err, hints, raw)
	if field := extractUnknownFieldName(err); field != "" && len(hints) == 0 && failed.Repair != nil {
		// A bare field name can also occur in unrelated patch operations.
		// Keep the rejected JSON coordinates, never resolve it to an
		// accepted path belonging to a different operation's schema.
		failed.Repair.Fields = nil
		if _, containers, ok := patchRawFieldOwnerAndContainers(raw, field); ok {
			for _, container := range containers {
				failed.Repair.Fields = append(failed.Repair.Fields, patchJSONPathJoin(container, field))
			}
		}
		failed.Repair.Hint = "The field is not accepted at the submitted location. Use only fields from the selected current patch branch, preserving all intended answer facts; do not borrow hidden coordinates from another operation."
	}
	failed.Repair = attachToolJSONSurfaceMetadataForSchema(name, schema, failed.Repair)
	return failed, decodeErr
}

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
		return nil
	}
	owner, invalidContainers, ok := patchRawFieldOwnerAndContainers(raw, field)
	if !ok {
		return nil
	}
	validPaths := patchSchemaFieldPaths(schema, field, owner)
	if len(validPaths) == 0 {
		return nil
	}
	// The decoder does not report the failing path. Exclude occurrences that
	// already occupy a published path instead of falsely calling them invalid.
	var rejectedContainers []string
	for _, container := range invalidContainers {
		published := false
		for _, valid := range validPaths {
			if patchJSONPathJoin(container, field) == valid {
				published = true
				break
			}
		}
		if !published {
			rejectedContainers = append(rejectedContainers, container)
		}
	}
	if len(rejectedContainers) == 0 {
		return nil
	}
	return []MisplacedFieldHint{{Field: field, ContainerNames: rejectedContainers, CorrectPaths: validPaths}}
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
