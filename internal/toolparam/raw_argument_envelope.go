package toolparam

import (
	"bytes"
	"encoding/json"
)

// RawToolArgumentEnvelopeStatus describes ownership of the argument object,
// not whether that object's tool-specific fields or optional carriers are valid.
type RawToolArgumentEnvelopeStatus string

const (
	RawToolArgumentEnvelopeNone       RawToolArgumentEnvelopeStatus = "none"
	RawToolArgumentEnvelopeUnique     RawToolArgumentEnvelopeStatus = "unique"
	RawToolArgumentEnvelopeEquivalent RawToolArgumentEnvelopeStatus = "equivalent"
	RawToolArgumentEnvelopeAmbiguous  RawToolArgumentEnvelopeStatus = "ambiguous"
)

// RawToolArgumentEnvelopeCandidate retains one original consumed carrier.
// Object is the exact candidate selected by the existing bounded string decoder
// (or the native object), before map decoding can erase duplicate properties.
// A nil Object means this alternative could not be admitted as an argument
// object; callers must not drop it when comparing competing alternatives.
type RawToolArgumentEnvelopeCandidate struct {
	Raw    json.RawMessage
	Object json.RawMessage
	Path   []string
}

// RawToolArgumentEnvelopeInspection is an out-of-band integrity result. It is
// never added to model-authored JSON. Truncated always implies Ambiguous: the
// retained prefix cannot establish agreement among all original alternatives.
type RawToolArgumentEnvelopeInspection struct {
	Status       RawToolArgumentEnvelopeStatus
	ConflictPath string
	Candidates   []RawToolArgumentEnvelopeCandidate
	Truncated    bool
}

const rawToolArgumentEnvelopeCandidateLimit = 64

// InspectRawToolArgumentEnvelope checks only the argument wrapper path already
// consumed by Normalize: one direct carrier, or a function carrier when the
// direct arm is not selected. Schema-owned properties inhibit envelope use;
// unrelated repeated metadata and an unconsumed function are not conflicts.
// The map view is used only for the existing admission/precedence predicates.
// Every consumed occurrence is read from raw JSON before any last-wins value
// can gain repair authority. This inspection is independent of repair mode.
func InspectRawToolArgumentEnvelope(raw, schema json.RawMessage) RawToolArgumentEnvelopeInspection {
	none := RawToolArgumentEnvelopeInspection{Status: RawToolArgumentEnvelopeNone}
	node, ok := parseSchema(schema)
	if !ok || !schemaExpectsObject(node) || len(node.Properties) == 0 {
		return none
	}
	input := raw
	value, ok := decodeJSONValue(input)
	if !ok {
		// The ordinary top-level normalizer has this same single syntax arm.
		// Keep its raw candidate so repeated wrapper keys survive the repair.
		if repaired, changed := RemoveTrailingCommasBeforeJSONClosers(string(input)); changed {
			input = json.RawMessage(repaired)
			value, ok = decodeJSONValue(input)
		}
	}
	if !ok {
		return none
	}
	if encoded, isString := value.(string); isString && !typeAllows(node.Type, "string") {
		inner, _, decoded := DecodeJSONStringAsRaw(encoded, "object")
		if !decoded {
			return none
		}
		// Existing successive agent/tool compatibility passes permit a whole
		// object string followed by one native direct/function envelope. This
		// is not a recursive search through arbitrary JSON descendants.
		if nested := inspectNativeToolArgumentEnvelope(inner, node); nested.Status != RawToolArgumentEnvelopeNone {
			return nested
		}
		return RawToolArgumentEnvelopeInspection{Status: RawToolArgumentEnvelopeUnique,
			Candidates: []RawToolArgumentEnvelopeCandidate{{Raw: append(json.RawMessage(nil), raw...), Object: inner}}}
	}
	return inspectNativeToolArgumentEnvelope(input, node)
}

func inspectNativeToolArgumentEnvelope(raw json.RawMessage, node schemaNode) RawToolArgumentEnvelopeInspection {
	none := RawToolArgumentEnvelopeInspection{Status: RawToolArgumentEnvelopeNone}
	value, ok := decodeJSONValue(raw)
	object, isObject := value.(map[string]any)
	if !ok || !isObject || hasSchemaPropertyAtEnvelope(object, node) || !envelopeHasOnlyKnownKeys(object) {
		return none
	}
	properties, ok := decodeRawObjectProperties(raw)
	if !ok {
		return none
	}
	inspection := none
	if key, _, direct := singleEnvelopeCarrier(object); direct {
		for _, property := range properties {
			if property.key == key {
				appendRawEnvelopeCandidate(&inspection, rawEnvelopeCandidate(property.value, []string{key}, node))
			}
		}
		return finalizeRawEnvelopeInspection(inspection, propertyPath("$", key))
	}
	// The existing direct arm takes precedence even when function metadata is
	// also present. Only the fallback arm consumes function, including each
	// repeated function property's own bounded argument path.
	functionCount := 0
	conflictPath := "$.function"
	for _, property := range properties {
		if property.key != "function" {
			continue
		}
		functionCount++
		fnValue, valid := decodeJSONValue(property.value)
		fn, nativeObject := fnValue.(map[string]any)
		if !valid || !nativeObject || hasSchemaPropertyAtEnvelope(fn, node) || !envelopeHasOnlyKnownKeys(fn) {
			appendRawEnvelopeCandidate(&inspection, RawToolArgumentEnvelopeCandidate{Raw: property.value, Path: []string{"function"}})
			continue
		}
		key, _, selected := singleEnvelopeCarrier(fn)
		fnProperties, rawOK := decodeRawObjectProperties(property.value)
		if !selected || !rawOK {
			appendRawEnvelopeCandidate(&inspection, RawToolArgumentEnvelopeCandidate{Raw: property.value, Path: []string{"function"}})
			continue
		}
		conflictPath = propertyPath("$.function", key)
		for _, field := range fnProperties {
			if field.key == key {
				appendRawEnvelopeCandidate(&inspection, rawEnvelopeCandidate(field.value, []string{"function", key}, node))
			}
		}
	}
	if functionCount > 1 {
		conflictPath = "$.function"
	}
	return finalizeRawEnvelopeInspection(inspection, conflictPath)
}

func rawEnvelopeCandidate(raw json.RawMessage, path []string, node schemaNode) RawToolArgumentEnvelopeCandidate {
	candidate := RawToolArgumentEnvelopeCandidate{Raw: raw, Path: path}
	value, ok := decodeJSONValue(raw)
	if !ok {
		return candidate
	}
	object := bytes.TrimSpace(raw)
	if encoded, isString := value.(string); isString {
		var decoded bool
		object, _, decoded = DecodeJSONStringAsRaw(encoded, "object")
		if !decoded {
			return candidate
		}
		value, ok = decodeJSONValue(object)
	}
	fields, isObject := value.(map[string]any)
	if ok && isObject && schemaPropertyHitCount(fields, node) > 0 {
		candidate.Object = object
	}
	return candidate
}

func appendRawEnvelopeCandidate(inspection *RawToolArgumentEnvelopeInspection, candidate RawToolArgumentEnvelopeCandidate) {
	if len(inspection.Candidates) >= rawToolArgumentEnvelopeCandidateLimit {
		inspection.Truncated = true
		return
	}
	inspection.Candidates = append(inspection.Candidates, candidate)
}

func finalizeRawEnvelopeInspection(inspection RawToolArgumentEnvelopeInspection, conflictPath string) RawToolArgumentEnvelopeInspection {
	if inspection.Truncated {
		inspection.Status, inspection.ConflictPath = RawToolArgumentEnvelopeAmbiguous, conflictPath
		return inspection
	}
	valid := 0
	for _, candidate := range inspection.Candidates {
		if candidate.Object != nil {
			valid++
		}
	}
	if valid == 0 {
		return RawToolArgumentEnvelopeInspection{Status: RawToolArgumentEnvelopeNone}
	}
	if len(inspection.Candidates) == 1 {
		inspection.Status = RawToolArgumentEnvelopeUnique
		return inspection
	}
	inspection.Status, inspection.ConflictPath = RawToolArgumentEnvelopeAmbiguous, conflictPath
	if valid != len(inspection.Candidates) {
		return inspection
	}
	// Whitespace outside JSON values is transport syntax. Do not use maps,
	// float coercion, sorted fields, or decoded schema values for agreement:
	// those could erase competing inner keys, numeric identities or prose.
	var first bytes.Buffer
	if json.Compact(&first, inspection.Candidates[0].Object) != nil {
		return inspection
	}
	for _, candidate := range inspection.Candidates[1:] {
		var next bytes.Buffer
		if json.Compact(&next, candidate.Object) != nil || !bytes.Equal(first.Bytes(), next.Bytes()) {
			return inspection
		}
	}
	inspection.Status, inspection.ConflictPath = RawToolArgumentEnvelopeEquivalent, ""
	return inspection
}

// RawToolArgumentEnvelope is the compatibility facade for callers needing one
// proven argument value. Equivalent repetitions contain exactly the same raw
// object facts; consumers needing every original carrier use the inspection.
func RawToolArgumentEnvelope(raw, schema json.RawMessage) (json.RawMessage, []string, bool) {
	inspection := InspectRawToolArgumentEnvelope(raw, schema)
	if inspection.Status != RawToolArgumentEnvelopeUnique && inspection.Status != RawToolArgumentEnvelopeEquivalent {
		return nil, nil, false
	}
	candidate := inspection.Candidates[0]
	return candidate.Object, candidate.Path, true
}
