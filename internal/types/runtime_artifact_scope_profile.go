package types

import (
	"math"
	"strings"
)

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
	TimeWindows    []RuntimeArtifactTimeWindow   `json:"time_windows,omitempty"`
	SourceQuote    string                        `json:"source_quote,omitempty"`
	Confidence     float64                       `json:"confidence,omitempty"`
	Rationale      string                        `json:"rationale,omitempty"`
}

// RuntimeArtifactTimeWindow is one ordered, request-owned member. Its quote is
// validated against the current request by emit_analysis, never parsed for
// coordinates. A member is not a capture, target, or measurement receipt.
type RuntimeArtifactTimeWindow struct {
	TimeStart   *float64 `json:"time_start,omitempty"`
	TimeEnd     *float64 `json:"time_end,omitempty"`
	SourceQuote string   `json:"source_quote,omitempty"`
}

// Valid checks the already-typed member shape, not the truth of its quote.
func (w RuntimeArtifactTimeWindow) Valid() bool {
	return strings.TrimSpace(w.SourceQuote) != "" && w.TimeStart != nil && w.TimeEnd != nil &&
		runtimeArtifactTimeWindowBoundsValid(*w.TimeStart, *w.TimeEnd)
}

func runtimeArtifactTimeWindowBoundsValid(start, end float64) bool {
	return !math.IsNaN(start) && !math.IsNaN(end) && !math.IsInf(start, 0) && !math.IsInf(end, 0) && start >= 0 && end > start
}

// Active reports whether the profile carries an anchored, structurally valid
// runtime-artifact scope. It consumes the validated enum and quote presence
// only; downstream routing does not reinterpret the quote text.
func (p *RuntimeArtifactScopeProfile) Active() bool {
	if p == nil {
		return false
	}
	switch p.RequestedScope {
	case RuntimeArtifactScopeFullArtifact, RuntimeArtifactScopeBoundedSelector:
		return strings.TrimSpace(p.SourceQuote) != ""
	case RuntimeArtifactScopeExplicitWindow:
		return p.HasExplicitTimeWindows()
	default:
		return false
	}
}

func (p *RuntimeArtifactScopeProfile) FullArtifact() bool {
	return p != nil &&
		p.RequestedScope == RuntimeArtifactScopeFullArtifact &&
		strings.TrimSpace(p.SourceQuote) != ""
}

func (p *RuntimeArtifactScopeProfile) ExplicitTimeWindow() (float64, float64, bool) {
	windows := p.explicitTimeWindows()
	if len(windows) != 1 {
		return 0, 0, false
	}
	return *windows[0].TimeStart, *windows[0].TimeEnd, true
}

// explicitTimeWindows accepts an entire valid group, never a valid subset.
// List presence prevents fallback to a legacy scalar or a derived envelope.
func (p *RuntimeArtifactScopeProfile) explicitTimeWindows() []RuntimeArtifactTimeWindow {
	if p == nil || p.RequestedScope != RuntimeArtifactScopeExplicitWindow {
		return nil
	}
	if p.TimeWindows != nil {
		if len(p.TimeWindows) == 0 || p.TimeStart != nil || p.TimeEnd != nil {
			return nil
		}
		for _, window := range p.TimeWindows {
			if !window.Valid() {
				return nil
			}
		}
		return p.TimeWindows
	}
	window := RuntimeArtifactTimeWindow{TimeStart: p.TimeStart, TimeEnd: p.TimeEnd, SourceQuote: p.SourceQuote}
	if !window.Valid() {
		return nil
	}
	return []RuntimeArtifactTimeWindow{window}
}

// ExplicitTimeWindows returns all validated members in request order. It does
// not sort, deduplicate repeated members, select the first, or form an envelope.
// Returned endpoints are independent copies, including the legacy scalar form.
func (p *RuntimeArtifactScopeProfile) ExplicitTimeWindows() []RuntimeArtifactTimeWindow {
	return cloneRuntimeArtifactTimeWindows(p.explicitTimeWindows())
}

func (p *RuntimeArtifactScopeProfile) HasExplicitTimeWindows() bool {
	return len(p.explicitTimeWindows()) > 0
}

// MatchExplicitTimeWindow returns the unique zero-based member index using the
// existing narrow principal-window precision. Duplicate or near-tolerance
// matches are ambiguous: (-1, false), never first-wins or a completeness grant.
func (p *RuntimeArtifactScopeProfile) MatchExplicitTimeWindow(start, end float64) (int, bool) {
	if !runtimeArtifactTimeWindowBoundsValid(start, end) {
		return -1, false
	}
	match := -1
	for i, window := range p.explicitTimeWindows() {
		if TraceCausalProjectionPrincipalValueSameWindow(*window.TimeStart, *window.TimeEnd, start, end) {
			if match >= 0 {
				return -1, false
			}
			match = i
		}
	}
	return match, match >= 0
}

// ContainsExplicitTimeWindow reports containment by at least one complete
// member, with the shared principal-window precision. It never spans gaps or
// unions members and does not choose an owner when containment is ambiguous.
func (p *RuntimeArtifactScopeProfile) ContainsExplicitTimeWindow(start, end float64) bool {
	if !runtimeArtifactTimeWindowBoundsValid(start, end) {
		return false
	}
	for _, window := range p.explicitTimeWindows() {
		if TraceCausalProjectionPrincipalValueWindowContains(*window.TimeStart, *window.TimeEnd, start, end) {
			return true
		}
	}
	return false
}

func CloneRuntimeArtifactScopeProfile(p *RuntimeArtifactScopeProfile) *RuntimeArtifactScopeProfile {
	if p == nil {
		return nil
	}
	out := *p
	if p.TimeStart != nil {
		value := *p.TimeStart
		out.TimeStart = &value
	}
	if p.TimeEnd != nil {
		value := *p.TimeEnd
		out.TimeEnd = &value
	}
	out.TimeWindows = cloneRuntimeArtifactTimeWindows(p.TimeWindows)
	return &out
}

func cloneRuntimeArtifactTimeWindows(in []RuntimeArtifactTimeWindow) []RuntimeArtifactTimeWindow {
	if in == nil {
		return nil
	}
	out := make([]RuntimeArtifactTimeWindow, len(in))
	copy(out, in)
	for i := range in {
		if in[i].TimeStart != nil {
			value := *in[i].TimeStart
			out[i].TimeStart = &value
		}
		if in[i].TimeEnd != nil {
			value := *in[i].TimeEnd
			out[i].TimeEnd = &value
		}
	}
	return out
}
