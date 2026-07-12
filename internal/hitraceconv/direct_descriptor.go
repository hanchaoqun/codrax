package hitraceconv

import "math"

// directDescriptorFieldIsolated proves that one physical field range is
// valid and does not overlap any other declared field. Typed direct payload
// decoders share this relation instead of growing family-specific overlap
// rules.
func directDescriptorFieldIsolated(ev decodedEvent, selectedIndex int) bool {
	if selectedIndex < 0 || selectedIndex >= len(ev.format.Fields) {
		return false
	}
	selected := ev.format.Fields[selectedIndex]
	if selected.Offset < 0 || selected.Size <= 0 || selected.Offset > math.MaxInt-selected.Size {
		return false
	}
	selectedEnd := selected.Offset + selected.Size
	for index, other := range ev.format.Fields {
		if index == selectedIndex {
			continue
		}
		if other.Offset < 0 || other.Size <= 0 || other.Offset > math.MaxInt-other.Size {
			return false
		}
		otherEnd := other.Offset + other.Size
		if selected.Offset < otherEnd && other.Offset < selectedEnd {
			return false
		}
	}
	return true
}

func directDescriptorFixedTail(ev decodedEvent) (int, bool) {
	tail := 0
	for _, field := range ev.format.Fields {
		if field.Offset < 0 || field.Size <= 0 || field.Offset > math.MaxInt-field.Size {
			return 0, false
		}
		if end := field.Offset + field.Size; end > tail {
			tail = end
		}
	}
	return tail, true
}
