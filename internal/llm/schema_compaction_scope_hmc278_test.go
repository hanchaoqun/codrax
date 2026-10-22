package llm

import (
	"bytes"
	"encoding/json"
	"testing"
)

// contentSchema is a schema position, unlike const/default/examples. A
// relative reference inside it must be found before its enclosing schema is
// moved to $defs; moving the original pointer target can make it disappear.
func TestHMC278SchemaCompactionContentSchemaScope(t *testing.T) {
	for _, key := range []string{"$ref", "$id", "$dynamicRef"} {
		t.Run(key, func(t *testing.T) {
			root := decodeSchemaTest(t, largeRepeatedSchema(t))
			props := root["properties"].(map[string]any)
			for _, name := range []string{"add", "replace"} {
				child := props[name].(map[string]any)["items"].(map[string]any)
				child["contentSchema"] = map[string]any{key: "#/properties/add/items/properties/large_integer"}
			}
			raw, _ := json.Marshal(root)
			if got := CompactToolSchemas([]ToolSchema{{Name: "content", Parameters: raw}})[0].Parameters; !bytes.Equal(got, raw) {
				t.Fatal("moved a contentSchema reference/scope and its possible pointer target")
			}
		})
	}
}

func TestHMC278SchemaCompactionTupleAndInstancePositions(t *testing.T) {
	for _, location := range []string{"items", "additionalItems", "prefixItems", "allOf", "anyOf", "oneOf"} {
		t.Run(location, func(t *testing.T) {
			root := decodeSchemaTest(t, largeRepeatedSchema(t))
			child := map[string]any{"$ref": "#/properties/add/items"}
			if location == "additionalItems" {
				root[location] = child
			} else {
				root[location] = []any{child}
			}
			raw, _ := json.Marshal(root)
			if got := compactToolParameters(raw); !bytes.Equal(got, raw) {
				t.Fatal("moved tuple/combinator scoped schema")
			}
		})
	}
	for _, location := range []string{"const", "default", "examples", "enum", "x-instance-data"} {
		t.Run(location, func(t *testing.T) {
			root := decodeSchemaTest(t, largeRepeatedSchema(t))
			literal := map[string]any{"contentSchema": map[string]any{"$ref": "#/$defs/shared_schema_1"}}
			if location == "examples" || location == "enum" {
				root[location] = []any{literal}
			} else {
				root[location] = literal
			}
			raw, _ := json.Marshal(root)
			got := compactToolParameters(raw)
			if len(got) >= len(raw)*3/4 {
				t.Fatal("instance JSON incorrectly disabled schema compaction")
			}
			before, _ := json.Marshal(root[location])
			after, _ := json.Marshal(decodeSchemaTest(t, got)[location])
			if !bytes.Equal(before, after) {
				t.Fatal("instance literal interpreted as a schema reference")
			}
		})
	}
}
