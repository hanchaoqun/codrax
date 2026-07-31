package types

import "strings"

// RuntimeArtifactRequestedScope is the user-authority time/scope boundary for
// an attached runtime artifact. It is intentionally distinct from model/tool
// query windows: an explorer may narrow a query without narrowing the user's
// requested answer universe.
type RuntimeArtifactRequestedScope string

const (
	RuntimeArtifactScopeNotApplicable   RuntimeArtifactRequestedScope = "not_applicable"
	RuntimeArtifactScopeFullArtifact    RuntimeArtifactRequestedScope = "full_artifact"
	RuntimeArtifactScopeExplicitWindow  RuntimeArtifactRequestedScope = "explicit_time_window"
	RuntimeArtifactScopeBoundedSelector RuntimeArtifactRequestedScope = "bounded_selector"
	RuntimeArtifactScopeUnspecified     RuntimeArtifactRequestedScope = "unspecified"
)

func AllRuntimeArtifactRequestedScopes() []RuntimeArtifactRequestedScope {
	return []RuntimeArtifactRequestedScope{
		RuntimeArtifactScopeNotApplicable,
		RuntimeArtifactScopeFullArtifact,
		RuntimeArtifactScopeExplicitWindow,
		RuntimeArtifactScopeBoundedSelector,
		RuntimeArtifactScopeUnspecified,
	}
}

func (s RuntimeArtifactRequestedScope) IsValid() bool {
	for _, candidate := range AllRuntimeArtifactRequestedScopes() {
		if s == candidate {
			return true
		}
	}
	return false
}

// RuntimeArtifactScopeProfile is analyzer-classified current-request scope.
// SourceQuote is retained only after exact current-request validation by
// emit_analysis. Thus FullArtifact()/ExplicitTimeWindow() are precise
// downstream gates and never parse user or model prose.
type RuntimeArtifactScopeProfile struct {
	RequestedScope RuntimeArtifactRequestedScope `json:"requested_scope"`
	TimeStart      *float64                      `json:"time_start,omitempty"`
	TimeEnd        *float64                      `json:"time_end,omitempty"`
	SourceQuote    string                        `json:"source_quote,omitempty"`
	Confidence     float64                       `json:"confidence,omitempty"`
	Rationale      string                        `json:"rationale,omitempty"`
}

func (p *RuntimeArtifactScopeProfile) FullArtifact() bool {
	return p != nil &&
		p.RequestedScope == RuntimeArtifactScopeFullArtifact &&
		strings.TrimSpace(p.SourceQuote) != ""
}

func (p *RuntimeArtifactScopeProfile) ExplicitTimeWindow() (float64, float64, bool) {
	if p == nil ||
		p.RequestedScope != RuntimeArtifactScopeExplicitWindow ||
		strings.TrimSpace(p.SourceQuote) == "" ||
		p.TimeStart == nil ||
		p.TimeEnd == nil ||
		*p.TimeStart < 0 ||
		*p.TimeEnd <= *p.TimeStart {
		return 0, 0, false
	}
	return *p.TimeStart, *p.TimeEnd, true
}
