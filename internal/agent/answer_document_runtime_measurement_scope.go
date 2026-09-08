package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Only the existing capture partition identity and explicit query-window
// carrier can join different measurement rulers. Display names, timestamps
// alone, and a first record in ledger order are not source identity.
type answerDocRuntimeMeasurementScope struct {
	artifact string
	window   string
}

func answerDocRuntimeMeasurementRecordScope(record types.ObservationRecord) (answerDocRuntimeMeasurementScope, bool) {
	scope := answerDocRuntimeMeasurementScope{
		artifact: types.TraceCausalProjectionRecordArtifactIdentity(record),
		window:   strings.TrimSpace(traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeySelectedWindow)),
	}
	return scope, scope.artifact != "" && scope.window != ""
}

func answerDocRuntimeMeasurementEvidenceScope(ledger types.ObservationLedger, id string) (answerDocRuntimeMeasurementScope, bool) {
	var selected answerDocRuntimeMeasurementScope
	found := false
	if strings.TrimSpace(id) == "" {
		return selected, false
	}
	for _, record := range ledger.Records {
		if record.ID != id {
			continue
		}
		scope, ok := answerDocRuntimeMeasurementRecordScope(record)
		if !ok || (found && scope != selected) {
			return answerDocRuntimeMeasurementScope{}, false
		}
		selected, found = scope, true
	}
	return selected, found
}

func answerDocUniqueRuntimeMeasurementScope(records []types.ObservationRecord) (answerDocRuntimeMeasurementScope, bool) {
	var selected answerDocRuntimeMeasurementScope
	found := false
	for _, record := range records {
		scope, ok := answerDocRuntimeMeasurementRecordScope(record)
		if !ok {
			continue // original unscoped observations remain available, never joined
		}
		if found && scope != selected {
			return answerDocRuntimeMeasurementScope{}, false
		}
		selected, found = scope, true
	}
	return selected, found
}

func answerDocRuntimeMeasurementScopedRecords(records []types.ObservationRecord, selected answerDocRuntimeMeasurementScope) []types.ObservationRecord {
	var out []types.ObservationRecord
	for _, record := range records {
		if scope, ok := answerDocRuntimeMeasurementRecordScope(record); ok && scope == selected {
			out = append(out, record)
		}
	}
	return out
}
