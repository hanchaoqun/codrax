package types

import "encoding/json"

// RuntimeArtifactSourceIdentitySet is a system-compiled source receipt, not a
// model claim field. Source metadata is captured before record deduplication:
// independently proving a claim must neither mint nor erase a physical source.
type RuntimeArtifactSourceIdentitySet struct {
	Sources []ObservationSourceRef `json:"sources,omitempty"`
}

func compileRuntimeArtifactSourceIdentities(records []ObservationRecord) *RuntimeArtifactSourceIdentitySet {
	out := &RuntimeArtifactSourceIdentitySet{}
	seen := map[string]bool{}
	for _, record := range records {
		if !runtimeArtifactRecordHasObservedSource(record) {
			continue
		}
		ref := record.SourceRef
		// Compare pointed-to metadata by value, never by allocation address.
		// Unsupported optional metadata is retained, not merged under an empty key.
		if data, err := json.Marshal(ref); err == nil {
			key := string(data)
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		if ref.ClockOffsetSec != nil {
			v := *ref.ClockOffsetSec
			ref.ClockOffsetSec = &v
		}
		if ref.ClockSlope != nil {
			v := *ref.ClockSlope
			ref.ClockSlope = &v
		}
		if ref.TraceExcerptScope != nil {
			v := *ref.TraceExcerptScope
			ref.TraceExcerptScope = &v
		}
		out.Sources = append(out.Sources, ref)
	}
	return out
}

func runtimeArtifactObservedSources(ledger ObservationLedger) []ObservationSourceRef {
	if ledger.RuntimeArtifactSources != nil {
		return ledger.RuntimeArtifactSources.Sources
	}
	return compileRuntimeArtifactSourceIdentities(ledger.Records).Sources
}
