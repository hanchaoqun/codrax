package tool

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
)

// This uses the public projected emitter schemas, not a miniature substitute.
// Expanding transport references must recover the complete executable contract.
func TestHMC278AnswerSchemaTransportRoundTrip(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"emit_answer_document":       (&EmitAnswerDocument{}).ParametersFor(nil),
		"emit_answer_document_patch": (&EmitAnswerDocumentPatch{}).ParametersFor(nil),
	} {
		t.Run(name, func(t *testing.T) {
			tools := []llm.ToolSchema{{Name: name, Parameters: raw}}
			wire := llm.CompactToolSchemas(tools)[0].Parameters
			decode := func(b []byte) map[string]any {
				var v map[string]any
				d := json.NewDecoder(bytes.NewReader(b))
				d.UseNumber()
				if err := d.Decode(&v); err != nil {
					t.Fatal(err)
				}
				return v
			}
			got := decode(wire)
			defs, _ := got["$defs"].(map[string]any)
			delete(got, "$defs")
			var expand func(any) any
			expand = func(value any) any {
				switch node := value.(type) {
				case map[string]any:
					if ref, ok := node["$ref"].(string); ok && len(node) == 1 && strings.HasPrefix(ref, "#/$defs/") {
						definition, found := defs[strings.TrimPrefix(ref, "#/$defs/")]
						if !found {
							t.Fatalf("dangling reference: %s", ref)
						}
						return expand(definition)
					}
					for key, child := range node {
						node[key] = expand(child)
					}
				case []any:
					for i, child := range node {
						node[i] = expand(child)
					}
				}
				return value
			}
			if !reflect.DeepEqual(expand(got), decode(raw)) {
				t.Fatal("wire projection changed canonical validation/teaching")
			}
			if name == "emit_answer_document_patch" && len(wire) >= len(raw)*3/4 {
				t.Fatalf("patch schema insufficiently compact: %d => %d", len(raw), len(wire))
			}
			t.Logf("%s parameters %d => %d bytes", name, len(raw), len(wire))
		})
	}
}
