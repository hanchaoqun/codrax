package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The native publication is already exposed through the selector roster. Do
// not spend the separate inventory metadata budget serializing its rows a
// second time. All other producer scope/quality notes remain verbatim.
func traceEventInventoryPromptNotes(record types.ObservationRecord, bridges ...traceEventPublicationBridge) []string {
	if _, valid := traceEventInventoryVerifiedPublication(record, bridges...); !valid {
		return append([]string(nil), record.RichNotes...)
	}
	var notes []string
	for _, note := range record.RichNotes {
		if !strings.HasPrefix(note, types.TraceNoteKeyRuntimeMeasurement+"=") {
			notes = append(notes, note)
		}
	}
	return notes
}

func traceEventInventoryVerifiedPublication(record types.ObservationRecord, bridges ...traceEventPublicationBridge) (types.RuntimeMeasurementPublication, bool) {
	if record.Predicate != types.TraceEventSearchInventoryPredicate {
		return types.RuntimeMeasurementPublication{}, false
	}
	if len(bridges) == 1 && bridges[0] != nil {
		publication, valid := bridges[0][traceEventPublicationRecordKey(record)]
		return publication, valid
	}
	if publication, valid := types.DecodeRuntimeMeasurementPublication(record); valid {
		return publication, true
	}
	// CompileObservationLedger fills only an absent ToolCallID. The producer's
	// publication still carries the original empty value. Undo that decoration
	// on this display-only copy, then repeat the complete exact-source, inventory
	// and table validation. No other identity/window field may be relaxed, and
	// a publication that owned a different nonempty ToolCallID still fails.
	if record.SourceRef.ToolCallID == "" {
		return types.RuntimeMeasurementPublication{}, false
	}
	record.SourceRef.ToolCallID = ""
	return types.DecodeRuntimeMeasurementPublication(record)
}
