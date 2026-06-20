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

// ExternalObservationCurrentSourceExclusionKind is the typed proof that an
// external observation policy's source exclusion is a user-requested boundary,
// not a model inference from unresolved artifact frames.
type ExternalObservationCurrentSourceExclusionKind string

const (
	ExternalObservationSourceExclusionNone                 ExternalObservationCurrentSourceExclusionKind = ""
	ExternalObservationSourceExclusionExplicitUserBoundary ExternalObservationCurrentSourceExclusionKind = "explicit_user_exclusion"
)

func AllExternalObservationCurrentSourceExclusionKinds() []ExternalObservationCurrentSourceExclusionKind {
	return []ExternalObservationCurrentSourceExclusionKind{
		ExternalObservationSourceExclusionExplicitUserBoundary,
	}
}

func (k ExternalObservationCurrentSourceExclusionKind) IsValid() bool {
	if k == ExternalObservationSourceExclusionNone {
		return true
	}
	for _, declared := range AllExternalObservationCurrentSourceExclusionKinds() {
		if k == declared {
			return true
		}
	}
	return false
}

func NormalizeExternalObservationCurrentSourceExclusionKind(raw string) ExternalObservationCurrentSourceExclusionKind {
	kind := ExternalObservationCurrentSourceExclusionKind(strings.TrimSpace(raw))
	if kind.IsValid() {
		return kind
	}
	return ExternalObservationSourceExclusionNone
}

// ExternalObservationArtifactCitationMode is orthogonal to current-source lane
// selection. It describes how line/row references from external observations
// should be treated when rendered as evidence. It must not be used to suppress
// current-source exploration.
type ExternalObservationArtifactCitationMode string

const (
	ExternalObservationArtifactCitationDefault            ExternalObservationArtifactCitationMode = "default"
	ExternalObservationArtifactCitationExternalOnly       ExternalObservationArtifactCitationMode = "external_only"
	ExternalObservationArtifactCitationAllowCurrentSource ExternalObservationArtifactCitationMode = "allow_current_source"
)

func AllExternalObservationArtifactCitationModes() []ExternalObservationArtifactCitationMode {
	return []ExternalObservationArtifactCitationMode{
		ExternalObservationArtifactCitationDefault,
		ExternalObservationArtifactCitationExternalOnly,
		ExternalObservationArtifactCitationAllowCurrentSource,
	}
}

func (m ExternalObservationArtifactCitationMode) IsValid() bool {
	for _, declared := range AllExternalObservationArtifactCitationModes() {
		if m == declared {
			return true
		}
	}
	return false
}

func NormalizeExternalObservationArtifactCitationMode(raw string) ExternalObservationArtifactCitationMode {
	mode := ExternalObservationArtifactCitationMode(strings.TrimSpace(raw))
	if mode.IsValid() {
		return mode
	}
	return ExternalObservationArtifactCitationDefault
}

// ExternalObservationPolicy is the typed lane for source-scope policy on
// external observations such as logs, traces, MCP resources, connector rows, or
// command output. It must not be inferred by raw keyword scans in downstream
// code. Exclusion is active only when the model gives an explicit typed
// exclusion kind plus an anchored current-request quote; this keeps accidental
// omitted source lanes from becoming a hard gate.
type ExternalObservationPolicy struct {
	CurrentSourceMode    ExternalObservationCurrentSourceMode          `json:"current_source_mode,omitempty"`
	ExclusionKind        ExternalObservationCurrentSourceExclusionKind `json:"exclusion_kind,omitempty"`
	ArtifactCitationMode ExternalObservationArtifactCitationMode       `json:"artifact_citation_mode,omitempty"`
	SourceQuotes         []string                                      `json:"source_quotes,omitempty"`
	Confidence           float64                                       `json:"confidence,omitempty"`
	Rationale            string                                        `json:"rationale,omitempty"`
}

func (p *ExternalObservationPolicy) ExcludesCurrentSource() bool {
	return p != nil &&
		p.CurrentSourceMode == ExternalObservationCurrentSourceExclude &&
		p.ExclusionKind == ExternalObservationSourceExclusionExplicitUserBoundary &&
		len(p.SourceQuotes) > 0
}

func (p *ExternalObservationPolicy) ArtifactCitationsExternalOnly() bool {
	return p != nil &&
		p.ArtifactCitationMode == ExternalObservationArtifactCitationExternalOnly
}
