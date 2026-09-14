package toolparam

import (
	"bytes"
	"encoding/json"
)

// RawToolArgumentEnvelope locates the original object behind the same unique
// envelope that Normalize already accepts. It does not unwrap or normalize the
// input. Consumers preserving opaque parameter fields can use this bounded
// path without repeating the envelope whitelist or re-marshaling its object.
func RawToolArgumentEnvelope(raw, schema json.RawMessage) (json.RawMessage, []string, bool) {
	value, ok := decodeJSONValue(raw)
	if !ok {
		return nil, nil, false
	}
	node, ok := parseSchema(schema)
	if !ok || !schemaExpectsObject(node) {
		return nil, nil, false
	}
	// Follow the same bounded string-decoding candidate used by Normalize.
	if encoded, isString := value.(string); isString && !typeAllows(node.Type, "string") {
		inner, _, decoded := DecodeJSONStringAsRaw(encoded, "object")
		if !decoded {
			return nil, nil, false
		}
		// Agent and tool boundaries may perform successive normalizations.
		// The decoded value is now a native object, so this second lookup is
		// bounded to the existing one direct/function envelope, not a walk.
		if nested, path, wrapped := RawToolArgumentEnvelope(inner, schema); wrapped {
			return nested, path, true
		}
		return inner, nil, true
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, nil, false
	}
	if _, _, admitted := unwrapToolArgumentEnvelope(object, node); !admitted {
		return nil, nil, false
	}
	key, _, direct := singleEnvelopeCarrier(object)
	path := []string{}
	container := raw
	if !direct {
		fn, ok := object["function"].(map[string]any)
		if !ok {
			return nil, nil, false
		}
		container, ok = uniqueRawEnvelopeField(raw, "function")
		if !ok {
			return nil, nil, false
		}
		key, _, ok = singleEnvelopeCarrier(fn)
		if !ok {
			return nil, nil, false
		}
		path = append(path, "function")
	}
	inner, ok := uniqueRawEnvelopeField(container, key)
	if !ok {
		return nil, nil, false
	}
	inner = bytes.TrimSpace(inner)
	var encoded string
	if json.Unmarshal(inner, &encoded) == nil {
		var decoded bool
		inner, _, decoded = DecodeJSONStringAsRaw(encoded, "object")
		if !decoded {
			return nil, nil, false
		}
	}
	if len(inner) == 0 || inner[0] != '{' || !json.Valid(inner) {
		return nil, nil, false
	}
	return inner, append(path, key), true
}

func uniqueRawEnvelopeField(raw json.RawMessage, key string) (json.RawMessage, bool) {
	properties, ok := decodeRawObjectProperties(raw)
	if !ok {
		return nil, false
	}
	var found json.RawMessage
	for _, property := range properties {
		if property.key != key {
			continue
		}
		if found != nil {
			return nil, false
		}
		found = property.value
	}
	return found, found != nil
}
