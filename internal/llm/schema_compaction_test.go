package llm

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func largeRepeatedSchema(t *testing.T) json.RawMessage {
	t.Helper()
	child := map[string]any{"type": "object", "additionalProperties": false, "description": strings.Repeat("来源与窗口不是因果证明。", 700), "properties": map[string]any{
		"source":        map[string]any{"type": "string", "enum": []string{"query-a/window-a", "query-b/window-b"}},
		"large_integer": map[string]any{"const": json.Number("9007199254740993")},
		"literal":       map[string]any{"const": map[string]any{"$ref": "this is instance data", "description": strings.Repeat("x", 2000)}},
	}, "required": []string{"source", "large_integer"}}
	raw, err := json.Marshal(map[string]any{"type": "object", "properties": map[string]any{"add": map[string]any{"type": "array", "items": child}, "replace": map[string]any{"type": "array", "items": child}}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func decodeSchemaTest(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var node map[string]any
	if err := dec.Decode(&node); err != nil {
		t.Fatal(err)
	}
	return node
}

func TestCompactToolSchemasLosslessIdentityAndWire(t *testing.T) {
	raw := largeRepeatedSchema(t)
	original := append(json.RawMessage(nil), raw...)
	tools := []ToolSchema{{Name: "patch", Description: "preserve every field", Parameters: raw}, {Name: "small", Parameters: json.RawMessage(`{"type":"object"}`)}}
	got := CompactToolSchemas(tools)
	if len(got[0].Parameters) >= len(raw)*3/4 {
		t.Fatalf("insufficient reduction: %d => %d", len(raw), len(got[0].Parameters))
	}
	if !bytes.Equal(tools[0].Parameters, original) || !reflect.DeepEqual(tools[1], got[1]) || got[0].Description != tools[0].Description {
		t.Fatal("canonical catalog or tool description mutated")
	}
	expanded := decodeSchemaTest(t, got[0].Parameters)
	defs := expanded["$defs"].(map[string]any)
	delete(expanded, "$defs")
	var expand func(map[string]any) map[string]any
	expand = func(node map[string]any) map[string]any {
		if ref, ok := node["$ref"].(string); ok {
			if !strings.HasPrefix(ref, "#/$defs/") || len(node) != 1 {
				t.Fatalf("unexpected reference: %v", node)
			}
			return defs[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
		}
		visitSchemaChildren(node, expand)
		return node
	}
	if !reflect.DeepEqual(expand(expanded), decodeSchemaTest(t, original)) {
		t.Fatal("expanded projection differs from complete original schema")
	}
	if twice := CompactToolSchemas(got); !reflect.DeepEqual(twice, got) {
		t.Fatal("projection not idempotent")
	}
	a := &OpenAIAdapter{}
	req := a.buildRequest([]Message{{Role: "user", Content: "事实完整保留"}}, got, ChatOptions{})
	if !bytes.Equal(req.Tools[0].Function.Parameters, got[0].Parameters) {
		t.Fatal("wire and budget catalogs differ")
	}
	if EstimateRequestTokens(nil, got) >= EstimateRequestTokens(nil, tools) {
		t.Fatal("telemetry did not use actual compact catalog")
	}
	t.Logf("parameters %d => %d bytes", len(raw), len(got[0].Parameters))
}

func TestCompactToolSchemasConservativeScopes(t *testing.T) {
	for _, scope := range []string{"$schema", "$id", "$ref", "$dynamicRef", "$anchor", "$recursiveAnchor", "definitions", "$defs"} {
		t.Run(scope, func(t *testing.T) {
			root := decodeSchemaTest(t, largeRepeatedSchema(t))
			// Even a nested scoped dependency must not be moved.
			root["dependencies"] = map[string]any{"a": map[string]any{scope: "urn:local"}}
			raw, _ := json.Marshal(root)
			if got := compactToolParameters(raw); !bytes.Equal(got, raw) {
				t.Fatal("changed scoped schema")
			}
		})
	}
	for _, raw := range []json.RawMessage{nil, []byte(`{"type":"object"}`), []byte(strings.Repeat("!", 17000))} {
		if got := compactToolParameters(raw); !bytes.Equal(got, raw) {
			t.Fatal("changed small/invalid schema")
		}
	}
}

func TestCompactToolSchemasNeverMergesDifferentQueryEnums(t *testing.T) {
	root := decodeSchemaTest(t, largeRepeatedSchema(t))
	props := root["properties"].(map[string]any)
	props["replace"].(map[string]any)["description"] = "different operation, same schema is not the same identity"
	child := props["replace"].(map[string]any)["items"].(map[string]any)
	child["properties"].(map[string]any)["source"].(map[string]any)["enum"] = []string{"query-c/window-c"}
	raw, _ := json.Marshal(root)
	got := compactToolParameters(raw)
	for _, identity := range []string{"query-a/window-a", "query-b/window-b", "query-c/window-c", "9007199254740993"} {
		if !bytes.Contains(got, []byte(identity)) {
			t.Fatalf("lost exact identity/value %s", identity)
		}
	}
}
