package types

import "strings"

// RuntimeArtifactWorkMode describes work on collected artifacts, not selection
// of code implementations, statistical representativeness, or answer size.
type RuntimeArtifactWorkMode string

const (
	RuntimeArtifactWorkRepresentative RuntimeArtifactWorkMode = "representative"
	RuntimeArtifactWorkExhaustive     RuntimeArtifactWorkMode = "exhaustive"
)

// RuntimeArtifactWorkProfile carries the analyzer's explicit task policy.
// SourceQuote is verified at acceptance against the current request. It is an
// audit anchor, not a downstream parsing language or a source/query credential.
type RuntimeArtifactWorkProfile struct {
	Mode                  RuntimeArtifactWorkMode `json:"mode"`
	PerSpecificationLimit int                     `json:"per_specification_limit,omitempty"`
	SourceQuote           string                  `json:"source_quote"`
}

// Valid checks only accepted typed shape; it never classifies request prose.
func (p *RuntimeArtifactWorkProfile) Valid() bool {
	if p == nil || strings.TrimSpace(p.SourceQuote) == "" {
		return false
	}
	switch p.Mode {
	case RuntimeArtifactWorkRepresentative:
		return p.PerSpecificationLimit >= 1
	case RuntimeArtifactWorkExhaustive:
		return p.PerSpecificationLimit == 0
	default:
		return false
	}
}

func CloneRuntimeArtifactWorkProfile(p *RuntimeArtifactWorkProfile) *RuntimeArtifactWorkProfile {
	if p == nil {
		return nil
	}
	copy := *p
	return &copy
}
