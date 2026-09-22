package types

import (
	"encoding/json"
	"math"
)

// HasSelectedWindow checks only this optional display ruler. It does not
// reject a measured value, add a window credential, or change shared interval
// arithmetic. Non-finite endpoints cannot identify an addressable query range.
func (d TraceCausalProjectionSelfRunningFoldUnmeasured) HasSelectedWindow() bool {
	w := d.SelectedWindow
	return w != nil && !math.IsInf(w.StartTs, 0) && !math.IsInf(w.EndTs, 0) &&
		TraceCausalProjectionWindowPresent(w.StartTs, w.EndTs)
}

// Preserve the source's two distinct rulers without inferring either from
// the elected board or changing the native quantity. Each compiled disclosure
// owns its pointer storage; later callers cannot mutate the ledger through it.
func traceCausalProjectionSelfRunningDisclosureScope(out *TraceCausalProjectionSelfRunningFoldUnmeasured, record ObservationRecord) {
	if record.SourceRef != (ObservationSourceRef{}) {
		ref := record.SourceRef
		if ref.ClockOffsetSec != nil {
			value := *ref.ClockOffsetSec
			ref.ClockOffsetSec = &value
		}
		if ref.ClockSlope != nil {
			value := *ref.ClockSlope
			ref.ClockSlope = &value
		}
		if ref.TraceExcerptScope != nil {
			value := *ref.TraceExcerptScope
			ref.TraceExcerptScope = &value
		}
		out.QuerySourceRef = &ref
	}
	if start, end, ok := traceCausalProjectionSelectedWindowNote(record.RichNotes); ok {
		out.SelectedWindow = &TraceCausalProjectionQueryWindow{StartTs: start, EndTs: end}
		if !out.HasSelectedWindow() {
			out.SelectedWindow = nil
		}
	}
}

// Exact display-duplicate suppression only, never a shared-measurement or
// causality authority. Different original sources, targets, selected windows,
// or quantities survive even when the subject is the same. JSON compares
// pointed-to values, not memory addresses. If optional metadata cannot be
// serialized, retain the disclosure rather than merge it under an empty key.
func traceCausalProjectionSelfRunningDisclosureKey(d TraceCausalProjectionSelfRunningFoldUnmeasured, recordID string) string {
	// A legacy row with neither source nor selected-window metadata cannot
	// be identified by its value alone across different observation records.
	// This preserves record identity, not an inferred source or permission.
	if d.QuerySourceRef != nil || d.SelectedWindow != nil {
		recordID = ""
	}
	key, err := json.Marshal(struct {
		Disclosure      TraceCausalProjectionSelfRunningFoldUnmeasured
		UnknownRecordID string
	}{d, recordID})
	if err != nil {
		return ""
	}
	return string(key)
}
