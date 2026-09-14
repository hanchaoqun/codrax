package tool

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/hanchaoqun/codrax/internal/toolparam"
)

type traceRootCauseRawProperty struct {
	key   string
	value json.RawMessage
	wire  json.RawMessage
}

// PrepareTraceRootCauseParamsForCompatibility preserves an ambiguous optional
// selection while the agent repairs unrelated answer fields. Only the exact
// root-cause field family is detached; every original occurrence is restored
// byte-for-byte afterwards, including duplicate keys. The owning tool captures
// the ambiguity before its own compatibility passes and discloses rejection of
// the selector, never the answer. No model-visible marker or call metadata is
// invented, and unambiguous arguments follow the ordinary path unchanged.
func PrepareTraceRootCauseParamsForCompatibility(toolName string, raw, schema json.RawMessage) (json.RawMessage, func(json.RawMessage) json.RawMessage) {
	if toolName != "emit_answer_document" && toolName != "emit_answer_document_patch" {
		return raw, nil
	}
	prepared, restore := prepareDirectTraceRootCauseParams(raw)
	if restore != nil {
		return prepared, restore
	}
	inspection := toolparam.InspectRawToolArgumentEnvelope(raw, schema)
	if inspection.Status == toolparam.RawToolArgumentEnvelopeEquivalent && len(inspection.Candidates) > 0 {
		// Every admitted wrapper has this exact object. Detach once from the
		// common object, not from just the first occurrence of its raw path.
		return prepareDirectTraceRootCauseParams(inspection.Candidates[0].Object)
	}
	if inner, path, ok := toolparam.RawToolArgumentEnvelope(raw, schema); ok {
		preparedInner, restoreInner := prepareDirectTraceRootCauseParams(inner)
		if restoreInner != nil {
			if preparedEnvelope, replaced := replaceTraceRootCauseEnvelopeObject(raw, path, preparedInner); replaced {
				return preparedEnvelope, restoreInner
			}
		}
	}
	return raw, nil
}

func prepareDirectTraceRootCauseParams(raw json.RawMessage) (json.RawMessage, func(json.RawMessage) json.RawMessage) {
	properties, ok := traceRootCauseRawProperties(raw)
	if !ok || traceRootCausePropertiesAmbiguity(properties) == nil {
		return raw, nil
	}
	var selected, other []traceRootCauseRawProperty
	for _, property := range properties {
		if traceRootCauseReservedParam(property.key) {
			selected = append(selected, property)
		} else {
			other = append(other, property)
		}
	}
	return traceRootCausePropertyObject(other), func(normalized json.RawMessage) json.RawMessage {
		body, valid := traceRootCauseRawProperties(normalized)
		if !valid {
			return raw
		}
		kept := make([]traceRootCauseRawProperty, 0, len(body)+len(selected))
		for _, property := range body {
			if !traceRootCauseReservedParam(property.key) {
				kept = append(kept, property)
			}
		}
		return traceRootCausePropertyObject(append(kept, selected...))
	}
}

func (l *optionalCarrierLedger) captureTraceRootCauseParamIntegrity(raw json.RawMessage) {
	if properties, ok := traceRootCauseRawProperties(raw); ok {
		l.traceRootCauseParamAmbiguity = traceRootCausePropertiesAmbiguity(properties)
	}
	if l.traceRootCauseParamAmbiguity != nil {
		return
	}
	var schema json.RawMessage
	switch l.toolName {
	case "emit_answer_document":
		schema = (&EmitAnswerDocument{}).Parameters()
	case "emit_answer_document_patch":
		schema = (&EmitAnswerDocumentPatch{}).Parameters()
	}
	prepared, bodyErr, envelopeErr := prepareAnswerArgumentEnvelope(raw, schema)
	if answerEnvelopeSelectionUnobserved(bodyErr) {
		l.traceRootCauseParamAmbiguity = bodyErr
		return
	}
	if envelopeErr != nil {
		l.traceRootCauseParamAmbiguity = envelopeErr
		return
	}
	if !bytes.Equal(prepared, raw) {
		if properties, ok := traceRootCauseRawProperties(prepared); ok {
			l.traceRootCauseParamAmbiguity = traceRootCausePropertiesAmbiguity(properties)
		}
		return
	}
	if inner, _, ok := toolparam.RawToolArgumentEnvelope(raw, schema); ok {
		if properties, valid := traceRootCauseRawProperties(inner); valid {
			l.traceRootCauseParamAmbiguity = traceRootCausePropertiesAmbiguity(properties)
		}
	}
}

func (l *optionalCarrierLedger) traceRootCauseParamIntegrityError() error {
	return l.traceRootCauseParamAmbiguity
}

// The path comes only from the normalizer's existing envelope admission.
// Preserve metadata and its string/object carrier shape until Normalize does
// the actual unwrap; no arbitrary JSON descent or new envelope is accepted.
func replaceTraceRootCauseEnvelopeObject(raw json.RawMessage, path []string, replacement json.RawMessage) (json.RawMessage, bool) {
	var encoded string
	if json.Unmarshal(raw, &encoded) == nil {
		decoded, _, ok := toolparam.DecodeJSONStringAsRaw(encoded, "object")
		if !ok {
			return raw, false
		}
		value, replaced := replaceTraceRootCauseEnvelopeObject(decoded, path, replacement)
		if !replaced {
			return raw, false
		}
		out, err := json.Marshal(string(value))
		return out, err == nil
	}
	if len(path) == 0 {
		return replacement, true
	}
	properties, ok := traceRootCauseRawProperties(raw)
	if !ok {
		return raw, false
	}
	for index, property := range properties {
		if property.key != path[0] {
			continue
		}
		value, replaced := replaceTraceRootCauseEnvelopeObject(property.value, path[1:], replacement)
		if !replaced {
			return raw, false
		}
		prefix := property.wire[:len(property.wire)-len(property.value)]
		properties[index].wire = append(append(json.RawMessage(nil), prefix...), value...)
		return traceRootCausePropertyObject(properties), true
	}
	return raw, false
}

func traceRootCausePropertiesAmbiguity(properties []traceRootCauseRawProperty) error {
	var selectors []traceRootCauseRawProperty
	var versions []json.RawMessage
	for _, property := range properties {
		switch strings.ToLower(property.key) {
		case "trace_root_causes", "replace_trace_root_causes", "root_causes":
			selectors = append(selectors, property)
		case "schema_version":
			versions = append(versions, property.value)
		}
	}
	if len(selectors) == 0 {
		return nil
	}
	submitted := false
	for _, selector := range selectors {
		submitted = submitted || traceRootCauseSelectorSubmitted(selector.value)
	}
	if !submitted {
		return nil
	}
	if len(selectors) > 1 {
		return errors.New("root-cause selection contains competing JSON carriers; no submitted value was chosen")
	}
	if len(versions) > 1 {
		return errors.New("root-cause selection contains duplicate or competing schema_version fields; no submitted value was chosen")
	}
	if len(versions) == 1 && !isExactTraceRootCauseSchemaVersion(versions[0]) {
		return errors.New("root-cause selection has a conflicting outer schema_version; no submitted value was chosen")
	}
	selection := selectors[0].value
	// Inspect the exact candidate admitted by existing bounded string decoding,
	// preserving duplicate fields before map-based normalization sees them.
	var encoded string
	if json.Unmarshal(selection, &encoded) == nil {
		for _, kind := range []string{"object", "array"} {
			if decoded, _, ok := toolparam.DecodeJSONStringAsRaw(encoded, kind); ok {
				selection = decoded
				break
			}
		}
	}
	if !traceRootCauseReportHasUniqueNestedKeys(selection) {
		return errors.New("root-cause selection contains duplicate or competing JSON fields; no submitted value was chosen")
	}
	return nil
}

func traceRootCauseReservedParam(key string) bool {
	switch strings.ToLower(key) {
	case "trace_root_causes", "replace_trace_root_causes", "root_causes", "schema_version":
		return true
	}
	return false
}

func traceRootCauseRawProperties(raw json.RawMessage) ([]traceRootCauseRawProperty, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, false
	}
	var properties []traceRootCauseRawProperty
	for decoder.More() {
		start := decoder.InputOffset()
		for start < int64(len(raw)) && (raw[start] == ',' || raw[start] == ' ' || raw[start] == '\n' || raw[start] == '\r' || raw[start] == '\t') {
			start++
		}
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return nil, false
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, false
		}
		properties = append(properties, traceRootCauseRawProperty{key: key, value: value, wire: raw[start:decoder.InputOffset()]})
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, false
	}
	return properties, true
}

func traceRootCausePropertyObject(properties []traceRootCauseRawProperty) json.RawMessage {
	var out bytes.Buffer
	out.WriteByte('{')
	for index, property := range properties {
		if index > 0 {
			out.WriteByte(',')
		}
		out.Write(property.wire)
	}
	out.WriteByte('}')
	return out.Bytes()
}
