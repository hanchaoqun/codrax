package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/toolparam"
)

// ToolArgumentEnvelopeIntegrityError refuses ambiguous consumed wrappers before
// an ordinary tool can choose one intent through map decoding. Answer owners
// split their body and optional selector independently at their own entrypoint.
func ToolArgumentEnvelopeIntegrityError(name string, raw, schema json.RawMessage) error {
	if name == "emit_answer_document" || name == "emit_answer_document_patch" {
		return nil
	}
	inspection := toolparam.InspectRawToolArgumentEnvelope(raw, schema)
	if inspection.Status == toolparam.RawToolArgumentEnvelopeAmbiguous {
		return fmt.Errorf("ambiguous tool argument envelope at %s: resend one arguments object; no alternative was executed", inspection.ConflictPath)
	}
	nested := toolparam.InspectSchemaConsumedArgumentEnvelopes(raw, schema)
	if nested.HasAmbiguity() {
		return fmt.Errorf("ambiguous nested tool arguments at %s: resend one object at each affected location; no alternative was executed", nestedArgumentEnvelopePaths(nested))
	}
	return nil
}

// Display only schema coordinates, never competing values or model prose.
func nestedArgumentEnvelopePaths(inspection toolparam.SchemaConsumedArgumentEnvelopeInspection) string {
	paths := make([]string, 0, len(inspection.Ambiguities)+1)
	for _, ambiguity := range inspection.Ambiguities {
		path := "$"
		for _, component := range ambiguity.Path {
			if strings.HasPrefix(component, "[") {
				path += component
			} else {
				path += "." + component
			}
		}
		paths = append(paths, path)
	}
	if inspection.Truncated {
		paths = append(paths, "additional locations beyond the comparison limit")
	}
	return strings.Join(paths, ", ")
}

// An incomplete comparison cannot establish that the model omitted a selector.
// Keep this fact out of band; do not invent a selector submission or an error
// field in the model payload merely to suppress an inaccurate omission note.
type answerEnvelopeUnobservedSelection struct{ error }

func answerEnvelopeSelectionUnobserved(err error) bool {
	_, unobserved := err.(*answerEnvelopeUnobservedSelection)
	return unobserved
}

// prepareAnswerArgumentEnvelope compares raw components, not model prose or
// candidate meaning. Only components identical in every admitted alternative
// may proceed. No private sentinel is inserted into model JSON. A body conflict
// returns only a common selector for the existing rejected-emit staging lane;
// a selector conflict returns only the common body and a per-call ledger error.
func prepareAnswerArgumentEnvelope(raw, schema json.RawMessage) (json.RawMessage, error, error) {
	prepared, bodyErr, selectorErr := prepareRootAnswerArgumentEnvelope(raw, schema)
	// Resolve only the proven root transport path before inspecting the body.
	// Nested candidates are blocks/items, never competing whole documents.
	effective := prepared
	if inner, _, ok := toolparam.RawToolArgumentEnvelope(prepared, schema); ok {
		effective = inner
	}
	properties, ok := traceRootCauseRawProperties(effective)
	if !ok {
		return prepared, bodyErr, selectorErr
	}
	var body, selector []traceRootCauseRawProperty
	for _, property := range properties {
		if traceRootCauseReservedParam(property.key) {
			selector = append(selector, property)
		} else {
			body = append(body, property)
		}
	}
	// The optional selector retains its existing raw-key integrity owner. A
	// body conflict cannot discard its independently valid submission, nor
	// can it clear an existing answer/patch lease before the normal reject tail.
	nested := toolparam.InspectSchemaConsumedArgumentEnvelopes(traceRootCausePropertyObject(body), schema)
	if nested.HasAmbiguity() {
		if bodyErr == nil {
			bodyErr = fmt.Errorf("ambiguous nested answer arguments at %s: resend one object at each affected location; the previous answer is unchanged", nestedArgumentEnvelopePaths(nested))
		}
		return traceRootCausePropertyObject(selector), bodyErr, selectorErr
	}
	return prepared, bodyErr, selectorErr
}

func prepareRootAnswerArgumentEnvelope(raw, schema json.RawMessage) (json.RawMessage, error, error) {
	inspection := toolparam.InspectRawToolArgumentEnvelope(raw, schema)
	if inspection.Status == toolparam.RawToolArgumentEnvelopeEquivalent && len(inspection.Candidates) > 0 {
		return inspection.Candidates[0].Object, nil, nil
	}
	if inspection.Status != toolparam.RawToolArgumentEnvelopeAmbiguous {
		return raw, nil, nil
	}
	bodyErr := fmt.Errorf("ambiguous tool argument envelope at %s: different or undecodable answer bodies; resend one complete arguments object; the previous answer is unchanged", inspection.ConflictPath)
	selectorErr := fmt.Errorf("duplicate or competing root-cause selections across tool argument envelopes at %s; resend one complete selection", inspection.ConflictPath)
	var body, selector []traceRootCauseRawProperty
	var bodyKey, selectorKey []byte
	bodyCommon, selectorCommon := !inspection.Truncated, !inspection.Truncated
	selectorSubmitted := false
	for i, candidate := range inspection.Candidates {
		properties, ok := traceRootCauseRawProperties(candidate.Object)
		if !ok {
			bodyCommon, selectorCommon = false, false
			continue
		}
		var nextBody, nextSelector []traceRootCauseRawProperty
		for _, property := range properties {
			if traceRootCauseReservedParam(property.key) {
				nextSelector = append(nextSelector, property)
				if !strings.EqualFold(property.key, "schema_version") && traceRootCauseSelectorSubmitted(property.value) {
					selectorSubmitted = true
				}
			} else {
				nextBody = append(nextBody, property)
			}
		}
		nextBodyKey := compactAnswerEnvelopeComponent(nextBody)
		nextSelectorKey := compactAnswerEnvelopeComponent(nextSelector)
		if i == 0 {
			body, selector = nextBody, nextSelector
			bodyKey, selectorKey = nextBodyKey, nextSelectorKey
		} else {
			bodyCommon = bodyCommon && bytes.Equal(bodyKey, nextBodyKey)
			selectorCommon = selectorCommon && bytes.Equal(selectorKey, nextSelectorKey)
		}
	}
	if len(inspection.Candidates) == 0 {
		bodyCommon, selectorCommon = false, false
	}
	var kept []traceRootCauseRawProperty
	if bodyCommon {
		kept = append(kept, body...)
		bodyErr = nil
	}
	if selectorCommon {
		kept = append(kept, selector...)
		selectorErr = nil
	} else if !selectorSubmitted {
		// An absent/unreadable selector is not an invented submission. The
		// body rejection still requires a unique wrapper before any write.
		selectorErr = nil
	}
	if inspection.Truncated {
		bodyErr = fmt.Errorf("tool argument envelope at %s exceeds the comparison limit; resend one complete arguments object; the previous answer is unchanged", inspection.ConflictPath)
		if !selectorSubmitted {
			bodyErr = &answerEnvelopeUnobservedSelection{bodyErr}
		}
	}
	return traceRootCausePropertyObject(kept), bodyErr, selectorErr
}

func compactAnswerEnvelopeComponent(properties []traceRootCauseRawProperty) []byte {
	var compact bytes.Buffer
	_ = json.Compact(&compact, traceRootCausePropertyObject(properties))
	return compact.Bytes()
}
