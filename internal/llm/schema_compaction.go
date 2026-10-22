package llm

import (
	"bytes"
	"encoding/json"
	"sort"
)

// CompactToolSchemas is a wire-only, lossless schema projection. Large exact
// repeated schema subtrees share a local definition; field names, descriptions,
// enum values (including source/query identities), and every constraint remain
// unchanged. Canonical execution/repair schemas are never mutated. Budgeting
// and sending must consume this same projected catalog.
func CompactToolSchemas(tools []ToolSchema) []ToolSchema {
	var out []ToolSchema
	for i, tool := range tools {
		raw := compactToolParameters(tool.Parameters)
		if bytes.Equal(raw, tool.Parameters) {
			continue
		}
		if out == nil {
			out = append([]ToolSchema(nil), tools...)
		}
		out[i].Parameters = raw
	}
	if out == nil {
		return tools
	}
	return out
}

func compactToolParameters(raw json.RawMessage) json.RawMessage {
	if len(raw) < 16*1024 || !json.Valid(raw) {
		return raw
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // Do not round large integer enum/const/minimum values.
	var root map[string]any
	if dec.Decode(&root) != nil || root == nil {
		return raw
	}
	counts := map[string]int{}
	unsafeScope := false
	var collect func(map[string]any, bool)
	collect = func(node map[string]any, isRoot bool) {
		// Moving scoped or referenced schemas can change URI resolution. Leave
		// them byte-for-byte intact, including catalogs already compacted here.
		for _, key := range []string{"$schema", "$id", "id", "$ref", "$dynamicRef", "$anchor", "$dynamicAnchor", "$recursiveRef", "$recursiveAnchor", "$defs", "definitions"} {
			if _, found := node[key]; found {
				unsafeScope = true
			}
		}
		if !isRoot {
			encoded, err := json.Marshal(node)
			if err == nil && len(encoded) >= 1024 {
				counts[string(encoded)]++
			}
		}
		visitSchemaChildren(node, func(child map[string]any) map[string]any {
			collect(child, false)
			return child
		})
	}
	collect(root, true)
	if unsafeScope {
		return raw
	}
	definitions := map[string]any{}
	refs := map[string]string{}
	var rewrite func(map[string]any, bool) map[string]any
	rewrite = func(node map[string]any, isRoot bool) map[string]any {
		if !isRoot {
			encoded, _ := json.Marshal(node)
			key := string(encoded)
			if counts[key] > 1 {
				name := refs[key]
				if name == "" {
					// Stable traversal, not map iteration, assigns names. An
					// extracted parent stays whole: no speculative nested defs.
					name = "shared_schema_" + schemaDefinitionOrdinal(len(refs)+1)
					refs[key] = name
					definitions[name] = node
				}
				return map[string]any{"$ref": "#/$defs/" + name}
			}
		}
		visitSchemaChildren(node, func(child map[string]any) map[string]any {
			return rewrite(child, false)
		})
		return node
	}
	rewrite(root, true)
	if len(definitions) == 0 {
		return raw
	}
	root["$defs"] = definitions
	compressed, err := json.Marshal(root)
	if err != nil || len(compressed)+1024 >= len(raw) {
		return raw
	}
	return compressed
}

func schemaDefinitionOrdinal(n int) string {
	// json.Marshal produces the decimal integer without locale or padding.
	raw, _ := json.Marshal(n)
	return string(raw)
}

// Traverse only JSON Schema positions. const, enum, default, examples and
// unknown extension payloads are instance data and must never become schemas.
func visitSchemaChildren(node map[string]any, visit func(map[string]any) map[string]any) {
	for _, key := range []string{"properties", "patternProperties", "dependencies", "dependentSchemas"} {
		children, _ := node[key].(map[string]any)
		names := make([]string, 0, len(children))
		for name := range children {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if child, ok := children[name].(map[string]any); ok {
				children[name] = visit(child)
			}
		}
	}
	for _, key := range []string{"items", "additionalItems", "contains", "contentSchema", "additionalProperties", "unevaluatedProperties", "unevaluatedItems", "propertyNames", "not", "if", "then", "else", "allOf", "anyOf", "oneOf", "prefixItems"} {
		switch value := node[key].(type) {
		case map[string]any:
			node[key] = visit(value)
		case []any:
			for i, item := range value {
				if child, ok := item.(map[string]any); ok {
					value[i] = visit(child)
				}
			}
		}
	}
}
