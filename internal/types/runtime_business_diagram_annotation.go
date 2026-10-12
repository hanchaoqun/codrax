package types

import "encoding/json"

// NativeBusinessDiagramAnnotation is a current, producer-owned interval note.
// It permits display on its exact observed owner, never an edge or a claim of
// causality. Only RuntimeMeasurementContract's private collection grants it.
type NativeBusinessDiagramAnnotation struct {
	Selector           string
	Name               string
	Record             ObservationRecord
	Owner              RuntimeWakeupThread
	StartTs, EndTs     float64
	StartLine, EndLine int
	StartRef, EndRef   string
}

// NativeBusinessDiagramAnnotations returns detached values. Public JSON can
// retain a selector for a later rebind, but cannot restore this private pool.
func (c *RuntimeMeasurementContract) NativeBusinessDiagramAnnotations() []NativeBusinessDiagramAnnotation {
	if c == nil {
		return nil
	}
	return cloneNativeBusinessDiagramAnnotations(c.nativeBusinessDiagramAnnotations)
}

func cloneNativeBusinessDiagramAnnotations(in []NativeBusinessDiagramAnnotation) []NativeBusinessDiagramAnnotation {
	out := make([]NativeBusinessDiagramAnnotation, 0, len(in))
	for _, annotation := range in {
		// ObservationRecord is a public data carrier. Deep-copy its nested
		// metadata without making caller mutations affect the private pool.
		raw, err := json.Marshal(annotation.Record)
		if err != nil {
			continue
		}
		var record ObservationRecord
		if json.Unmarshal(raw, &record) != nil {
			continue
		}
		annotation.Record = record
		out = append(out, annotation)
	}
	return out
}
