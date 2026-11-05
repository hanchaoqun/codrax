package types

import "encoding/json"

// ObservationValueContext is producer-owned methodology/availability data
// that must travel with a numeric observation, not optional narrative notes.
// Fields names the affected operands within this record's existing source and
// query scope. It grants no source, physical-relation or causal authority.
// Missing legacy context remains missing; consumers never infer it from prose.
type ObservationValueContext struct {
	Fields string `json:"fields,omitempty"`
	Method string `json:"method,omitempty"`
	Detail string `json:"detail"`
}

func CloneObservationValueContexts(in []ObservationValueContext) []ObservationValueContext {
	return append([]ObservationValueContext(nil), in...)
}

func mergeObservationValueContexts(a, b []ObservationValueContext) []ObservationValueContext {
	var out []ObservationValueContext
	seen := make(map[ObservationValueContext]bool, len(a)+len(b))
	for _, items := range [][]ObservationValueContext{a, b} {
		for _, item := range items {
			if !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
	}
	return out
}

// Keep the complete qualification beside its value. Optional note/excerpt
// limits cannot turn a qualified estimate into an unqualified measured value.
// Renderers account for this whole string in their existing per-record budget;
// a row may be omitted, never shortened into a different measurement meaning.
func FormatObservationValueContexts(in []ObservationValueContext) string {
	if len(in) == 0 {
		return ""
	}
	raw, _ := json.Marshal(in)
	return "value_context=" + string(raw)
}
