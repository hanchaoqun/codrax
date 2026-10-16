package tool

import (
	"bytes"
	"encoding/json"
)

// traceCatalogPlanSchema shares the ordinary query field contract. Each plan is
// applied separately to discovered candidates, so physical sources, prior-call
// references and comparison routes cannot be part of that reusable plan.
// The field census test requires review when trace_query adds a field or an
// ordinary top-level constraint; descriptions alone are compacted here.
func traceCatalogPlanSchema(raw json.RawMessage) json.RawMessage {
	var catalog, query map[string]any
	if json.Unmarshal(raw, &catalog) != nil || json.Unmarshal((&TraceQuery{}).Parameters(), &query) != nil {
		return raw
	}
	queryProperties, ok := query["properties"].(map[string]any)
	if !ok {
		return raw
	}
	for _, key := range []string{"source", "path", "business_span_ref", "comparison", "catalog_query_ref"} {
		delete(queryProperties, key)
	}
	compact := traceCatalogCompactQueryTeaching(queryProperties).(map[string]any)
	for key, description := range map[string]string{
		"view":       "Choose the observation requested by the user. resource_stack returns native resource events with their same-capture frames; event_search locates raw events. Other views retain the ordinary trace_query contract.",
		"pid":        "Exact numeric target ID; target_scope defaults to thread (TID), not process. Use target_scope=process only for an explicit process target.",
		"thread":     "Typed thread selector under the ordinary trace_query contract. Do not substitute an event-text pattern for a requested thread identity.",
		"time_start": "Requested query start, in trace seconds; pair with time_end for an explicit window. Planning does not infer a window.",
		"time_end":   "Requested query end, in trace seconds; boundary inclusion remains the selected view's actual query contract.",
		"pattern":    "Case-insensitive literal event-text substring, not regex or a typed target selector.",
		"patterns":   "Literal event-text alternatives (OR); other supplied filters still apply together (AND).",
	} {
		if field, ok := compact[key].(map[string]any); ok {
			field["description"] = description
		}
	}
	properties := catalog["properties"].(map[string]any)
	plans := properties["queries"].(map[string]any)
	plans["items"] = map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"view"}, "properties": compact,
	}
	plans["description"] = "Expected ordinary trace_query calls for every candidate; required field: view. Supply only task-supported object/window/filter fields. Capture-local paths, references and comparison routes are not reusable plans. Register these before representative selection; planning neither executes nor supplies evidence. Re-discovering the same root adds plans without discarding current attempts."
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(catalog) != nil {
		return raw
	}
	return json.RawMessage(bytes.TrimSpace(out.Bytes()))
}

// Keep concise existing field teaching while avoiding a second copy of the
// query's large view/causal tutorials. Every non-description schema key survives.
func traceCatalogCompactQueryTeaching(value any) any {
	switch item := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(item))
		for key, child := range item {
			if key == "description" {
				if text, ok := child.(string); !ok || len(text) > 384 {
					continue
				}
			}
			out[key] = traceCatalogCompactQueryTeaching(child)
		}
		return out
	case []any:
		out := make([]any, len(item))
		for i, child := range item {
			out[i] = traceCatalogCompactQueryTeaching(child)
		}
		return out
	default:
		return value
	}
}
