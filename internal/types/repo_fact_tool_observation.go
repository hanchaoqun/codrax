package types

import (
	"encoding/json"
	"time"
)

// RepoFactToolObservation is a small display receipt, not a measurement or an
// execution permission. Source-scoped facts still come from the normal ledger.
type RepoFactToolObservation struct {
	ToolName          string
	RawRef            string
	ObservedAt        time.Time
	TypedRecords      int
	Origins           []AnswerEvidenceOrigin
	GroundingPolicies []ClaimGroundingPolicy
}

type repoFactToolObservation struct {
	key, value, source, evidenceRef string
	display                         RepoFactToolObservation
}

// NewToolResultRepoFact preserves the historical fact and confidence fields
// while attaching an immutable, producer-only association for prompt display.
// Only the existing typed compilers supply origin/policy metadata. In
// particular, a successful arbitrary command is not a typed measurement.
func NewToolResultRepoFact(result ToolResult, source string, confidence float64) RepoFact {
	f := RepoFact{Key: result.ToolName, Value: result.Summary, Source: source, EvidenceRef: result.RawRef, Confidence: confidence}
	if !result.Success {
		return f
	}
	display := RepoFactToolObservation{ToolName: result.ToolName, RawRef: result.RawRef, ObservedAt: result.Timestamp}
	add := func(record ObservationRecord) {
		display.TypedRecords++
		if record.Origin != AnswerEvidenceOriginUnknown {
			found := false
			for _, old := range display.Origins {
				found = found || old == record.Origin
			}
			if !found {
				display.Origins = append(display.Origins, record.Origin)
			}
		}
		if record.GroundingPolicy != ClaimGroundingUnknown {
			found := false
			for _, old := range display.GroundingPolicies {
				found = found || old == record.GroundingPolicy
			}
			if !found {
				display.GroundingPolicies = append(display.GroundingPolicies, record.GroundingPolicy)
			}
		}
	}
	seen := map[string]bool{}
	compileProducerToolResultObservations(0, result, seen, add)
	compileToolResultCarrierObservations(0, result, nil, seen, add)
	f.toolObservation = &repoFactToolObservation{f.Key, f.Value, f.Source, f.EvidenceRef, display}
	return f
}

// ToolObservation returns a defensive display copy only while the fact still
// names the producer's exact bytes and locators. Same-tool/path/value guesses
// never reconnect a legacy fact, and changing a copied fact drops association.
func (f RepoFact) ToolObservation() (RepoFactToolObservation, bool) {
	p := f.toolObservation
	if p == nil || f.Key != p.key || f.Value != p.value || f.Source != p.source || f.EvidenceRef != p.evidenceRef {
		return RepoFactToolObservation{}, false
	}
	out := p.display
	out.Origins = append([]AnswerEvidenceOrigin(nil), out.Origins...)
	out.GroundingPolicies = append([]ClaimGroundingPolicy(nil), out.GroundingPolicies...)
	return out, true
}

// UnmarshalJSON also clears a previous receipt when a caller reuses a Go
// value for restore. Ordinary copies retain the immutable display snapshot.
func (f *RepoFact) UnmarshalJSON(data []byte) error {
	type wire RepoFact
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*f = RepoFact(decoded)
	return nil
}
