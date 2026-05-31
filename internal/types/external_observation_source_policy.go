package types

import "strings"

// ExternalObservationCurrentSourceMode is the analyzer's typed decision about
// whether a non-current-source observation should be paired with current
// checkout evidence. The default is intentionally mixed: Codrax is primarily a
// source-analysis system, so external observations do not suppress source
// exploration unless the analyzer emits an anchored exclusion profile.
type ExternalObservationCurrentSourceMode string

const (
	ExternalObservationCurrentSourceDefault ExternalObservationCurrentSourceMode = "default"
	ExternalObservationCurrentSourceAllow   ExternalObservationCurrentSourceMode = "allow"
	ExternalObservationCurrentSourceExclude ExternalObservationCurrentSourceMode = "exclude"
)

func AllExternalObservationCurrentSourceModes() []ExternalObservationCurrentSourceMode {
	return []ExternalObservationCurrentSourceMode{
		ExternalObservationCurrentSourceDefault,
		ExternalObservationCurrentSourceAllow,
		ExternalObservationCurrentSourceExclude,
	}
}

func (m ExternalObservationCurrentSourceMode) IsValid() bool {
	for _, declared := range AllExternalObservationCurrentSourceModes() {
		if m == declared {
			return true
		}
	}
	return false
}

func NormalizeExternalObservationCurrentSourceMode(raw string) ExternalObservationCurrentSourceMode {
	mode := ExternalObservationCurrentSourceMode(strings.TrimSpace(raw))
	if mode.IsValid() {
		return mode
	}
	return ExternalObservationCurrentSourceDefault
}

// ExternalObservationPolicy is the typed lane for source-scope policy on
// external observations such as logs, traces, MCP resources, connector rows, or
// command output. It must not be inferred by raw keyword scans in downstream
// code. Exclusion is active only when the model gives an anchored current
// request quote; this keeps accidental omitted source lanes from becoming a
// hard gate.
type ExternalObservationPolicy struct {
	CurrentSourceMode ExternalObservationCurrentSourceMode `json:"current_source_mode,omitempty"`
	SourceQuotes      []string                             `json:"source_quotes,omitempty"`
	Confidence        float64                              `json:"confidence,omitempty"`
	Rationale         string                               `json:"rationale,omitempty"`
}

func (p *ExternalObservationPolicy) ExcludesCurrentSource() bool {
	return p != nil &&
		p.CurrentSourceMode == ExternalObservationCurrentSourceExclude &&
		len(p.SourceQuotes) > 0
}
