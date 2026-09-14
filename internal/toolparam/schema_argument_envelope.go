package toolparam

import (
	"encoding/json"
	"fmt"
	"sort"
)

// SchemaConsumedArgumentEnvelopeAmbiguity belongs to one schema-owned value,
// not to the root tool arguments. Path contains canonical property names and
// array coordinates ("[0]"); transport wrappers are deliberately absent.
// Envelope keeps its local raw carrier paths and every bounded alternative.
type SchemaConsumedArgumentEnvelopeAmbiguity struct {
	Path     []string
	Envelope RawToolArgumentEnvelopeInspection
}

// SchemaConsumedArgumentEnvelopeInspection is separate from root envelope
// ownership. In particular, an answer owner can handle an optional selector
// conflict without mistaking its candidates for competing answer documents.
type SchemaConsumedArgumentEnvelopeInspection struct {
	Ambiguities []SchemaConsumedArgumentEnvelopeAmbiguity
	Truncated   bool
}

func (i SchemaConsumedArgumentEnvelopeInspection) HasAmbiguity() bool {
	return len(i.Ambiguities) > 0 || i.Truncated
}

const schemaArgumentEnvelopeAmbiguityLimit = 64

// InspectSchemaConsumedArgumentEnvelopes follows the object properties and
// array items consumed by Normalize, retaining raw values until comparison.
// It does not inspect opaque objects, unknown properties, or schema-owned
// fields merely because their names resemble a transport wrapper. The root
// envelope API remains the authority for root argument ownership.
//
// Like the root whole-string guard, this also protects a decoded object before
// its first map round-trip: that pass normalizes its properties, and a later
// compatibility pass may consume its native wrapper. Waiting for the later
// pass would already have erased competing original carriers.
func InspectSchemaConsumedArgumentEnvelopes(raw, schema json.RawMessage) SchemaConsumedArgumentEnvelopeInspection {
	var inspection SchemaConsumedArgumentEnvelopeInspection
	input := raw
	if repaired, _, ok := coalesceDuplicateSchemaArrayProperties(input, schema); ok {
		input = repaired
	}
	if _, ok := decodeJSONValue(input); !ok {
		if repaired, changed := RemoveTrailingCommasBeforeJSONClosers(string(input)); changed {
			input = json.RawMessage(repaired)
		}
	}
	inspection.walkValue(input, schema, nil)
	return inspection
}

func (i *SchemaConsumedArgumentEnvelopeInspection) walkValue(raw, schema json.RawMessage, path []string) {
	if i.Truncated {
		return
	}
	node, ok := parseSchema(schema)
	if !ok {
		return
	}
	value, ok := decodeJSONValue(raw)
	if !ok {
		return
	}
	if schemaExpectsObject(node) {
		if encoded, stringValue := value.(string); stringValue && !typeAllows(node.Type, "string") {
			if decoded, _, ok := DecodeJSONStringAsRaw(encoded, "object"); ok {
				i.walkObjectEnvelope(decoded, node, path)
				return
			}
		}
		if _, objectValue := value.(map[string]any); objectValue {
			i.walkObjectEnvelope(raw, node, path)
			return
		}
	}
	if !schemaExpectsArray(node) || len(node.Items) == 0 {
		return
	}
	if _, objectValue := value.(map[string]any); objectValue && !typeAllows(node.Type, "object") && arrayItemsExpectObject(node) {
		i.walkValue(raw, node.Items, schemaEnvelopeChildPath(path, "[0]"))
		return
	}
	if encoded, stringValue := value.(string); stringValue && !typeAllows(node.Type, "string") {
		decoded, _, ok := DecodeJSONStringAsRaw(encoded, "array")
		if !ok {
			decoded, _, ok = decodeJSONStringArrayWithSchemaRaw(encoded, node)
		}
		if !ok {
			return
		}
		raw = decoded
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return
	}
	for index, item := range items {
		i.walkValue(item, node.Items, schemaEnvelopeChildPath(path, fmt.Sprintf("[%d]", index)))
		if i.Truncated {
			return
		}
	}
}

func (i *SchemaConsumedArgumentEnvelopeInspection) walkObjectEnvelope(raw json.RawMessage, node schemaNode, path []string) {
	if len(node.Properties) == 0 {
		return
	}
	envelope := inspectNativeToolArgumentEnvelope(raw, node)
	if envelope.Status == RawToolArgumentEnvelopeAmbiguous {
		// Root ambiguity is deliberately not recast as a nested value. The
		// caller must resolve root ownership before assigning child scope.
		if len(path) > 0 {
			if len(i.Ambiguities) >= schemaArgumentEnvelopeAmbiguityLimit {
				i.Truncated = true
				return
			}
			i.Ambiguities = append(i.Ambiguities, SchemaConsumedArgumentEnvelopeAmbiguity{
				Path: append([]string(nil), path...), Envelope: envelope,
			})
		}
		return
	}
	if envelope.Status == RawToolArgumentEnvelopeUnique || envelope.Status == RawToolArgumentEnvelopeEquivalent {
		raw = envelope.Candidates[0].Object
	}
	properties, ok := decodeRawObjectProperties(raw)
	if !ok {
		return
	}
	// Map values remain RawMessages. The existing key-alias selector is reused
	// verbatim; maps are never used to compare competing carrier contents.
	// Ordinary repeated schema properties retain Normalize's existing policy;
	// only the top-level native-array concatenation above is a lossless merge.
	fields := make(map[string]any, len(properties))
	for _, property := range properties {
		fields[property.key] = property.value
	}
	if aliased, _ := normalizeObjectPropertyKeys(fields, node, "$"); aliased != nil {
		fields = aliased
	}
	keys := make([]string, 0, len(node.Properties))
	for key := range node.Properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if child, exists := fields[key]; exists {
			i.walkValue(child.(json.RawMessage), node.Properties[key], schemaEnvelopeChildPath(path, key))
			if i.Truncated {
				return
			}
		}
	}
}

func schemaEnvelopeChildPath(path []string, child string) []string {
	result := make([]string, len(path)+1)
	copy(result, path)
	result[len(path)] = child
	return result
}
