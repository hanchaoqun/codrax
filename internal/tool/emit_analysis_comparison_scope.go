package tool

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
)

const runtimeComparisonWindowSourceRepair = "use a short literal current-user interval quote; if no interval was requested, omit endpoints and use unspecified when neither side has a requested interval. Preserve comparison path/source_quote bindings"

func runtimeArtifactComparisonSchema() map[string]any {
	side := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"path", "source_quote"}, "dependentRequired": map[string]any{"time_start": []string{"time_end"}, "time_end": []string{"time_start"}}, "properties": map[string]any{
		"path": map[string]any{"type": "string"}, "time_start": map[string]any{"type": "number", "minimum": 0}, "time_end": map[string]any{"type": "number", "minimum": 0}, "source_quote": map[string]any{"type": "string"},
	}}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"baseline", "current"}, "properties": map[string]any{"baseline": side, "current": side}, "description": "Independent source-owned baseline/current inputs. Each member carries its original path, local source_quote and optional paired time_start/time_end. Follow the containing runtime_artifact_scope_profile range and source-identity rules."}
}

func parseRuntimeArtifactComparisonScope(raw string, p *emitRuntimeArtifactScopeProfileParam) (*types.RuntimeArtifactScopeProfile, string, []string) {
	fail := func(reason string) (*types.RuntimeArtifactScopeProfile, string, []string) {
		return nil, "runtime_artifact_scope_profile.comparison: " + reason + ". Re-emit the complete object, correcting the named invalid fields while preserving valid comparison members and unrelated decisions; do not drop the independent source binding to bypass this error.", nil
	}
	if p.TimeWindows != nil {
		return fail("time_windows is for a single capture; use either source-owned comparison intervals or one shared outer time_start/time_end/source_quote, never both")
	}
	if p.Confidence == nil || *p.Confidence < 0 || *p.Confidence > 1 || !p.Comparison.Valid() {
		// Keep the original validity predicate, but identify a bad leaf rather
		// than asking the model to repeatedly adjust already-valid endpoints.
		if p.Confidence != nil && *p.Confidence >= 0 && *p.Confidence <= 1 {
			for i, side := range p.Comparison.Sides() {
				field := "runtime_artifact_scope_profile.comparison." + []string{"baseline", "current"}[i]
				if issue := runtimeWindowSourceQuoteIssue(raw, side.SourceQuote, field); issue != "" {
					return fail(issue)
				}
				if (side.TimeStart != nil || side.TimeEnd != nil) && !side.RuntimeArtifactTimeWindow.Valid() {
					return fail(runtimeWindowEndpointIssue(field))
				}
			}
		}
		return fail("both distinct paths, local source quotes and confidence in [0,1] are required; each side's optional time_start/time_end must be a complete finite ordered pair")
	}
	pair := p.Comparison.Clone()
	shared := p.TimeStart != nil || p.TimeEnd != nil
	sharedWindow := types.RuntimeArtifactTimeWindow{TimeStart: p.TimeStart, TimeEnd: p.TimeEnd, SourceQuote: strings.TrimSpace(p.SourceQuote)}
	if shared {
		if issue := runtimeWindowSourceQuoteIssue(raw, sharedWindow.SourceQuote, "runtime_artifact_scope_profile"); issue != "" {
			return fail(issue + " Use a current-user literal interval if present; otherwise omit unrequested endpoints while preserving comparison source bindings.")
		}
		if !sharedWindow.Valid() {
			return fail(runtimeWindowEndpointIssue("runtime_artifact_scope_profile"))
		}
		if pair.Baseline.HasExplicitWindow() || pair.Current.HasExplicitWindow() {
			return fail("shared outer time_start/time_end cannot be combined with side endpoints; preserve local path quotes separately")
		}
	}
	var warnings []string
	if shared {
		bound, warning, err := bindRuntimeWindowSourceWithRepair(sharedWindow, runtimeComparisonWindowSourceRepair)
		if err != nil {
			return fail(fmt.Sprintf("shared outer interval: %v", err))
		}
		sharedWindow = bound
		if warning != "" {
			warnings = append(warnings, warning)
		}
	}
	var windows []types.RuntimeArtifactTimeWindow
	sides := []*types.RuntimeArtifactComparisonSide{&pair.Baseline, &pair.Current}
	for i, side := range sides {
		side.Path, side.SourceQuote = strings.TrimSpace(side.Path), strings.TrimSpace(side.SourceQuote)
		field := "runtime_artifact_scope_profile.comparison." + []string{"baseline", "current"}[i]
		if issue := runtimeWindowSourceQuoteIssue(raw, side.SourceQuote, field); issue != "" {
			return fail(issue)
		}
		if !runtimeComparisonQuoteHasPath(side.SourceQuote, side.Path) || runtimeComparisonQuoteHasPath(side.SourceQuote, sides[1-i].Path) {
			return fail(fmt.Sprintf("side %d needs one local verbatim quote containing its own exact original path, not the other side's path; include its literal interval only when specified", i+1))
		}
		if !side.HasExplicitWindow() {
			continue
		}
		bound, warning, err := bindRuntimeWindowSourceWithRepair(side.RuntimeArtifactTimeWindow, runtimeComparisonWindowSourceRepair)
		if err != nil {
			return fail(fmt.Sprintf("side %d: %v", i+1, err))
		}
		side.RuntimeArtifactTimeWindow = bound
		windows = append(windows, bound)
		if warning != "" {
			warnings = append(warnings, warning)
		}
	}
	scope := types.RuntimeArtifactRequestedScope(p.RequestedScope)
	bounded := shared || len(windows) > 0
	if bounded && scope != types.RuntimeArtifactScopeExplicitWindow || !bounded && scope != types.RuntimeArtifactScopeUnspecified && scope != types.RuntimeArtifactScopeFullArtifact {
		return fail("use explicit_time_window for a shared literal interval or source-owned endpoints; otherwise use unspecified or full_artifact without inventing endpoints")
	}
	quote := strings.TrimSpace(p.SourceQuote)
	if quote != "" && !sourceQuotePresentInCurrentRequest(raw, quote) {
		quote = ""
	}
	if scope == types.RuntimeArtifactScopeFullArtifact && quote == "" {
		// Source identity and whole-artifact range are independent authorities.
		// The local quotes above remain valid when only the outer range quote is
		// missing; match ordinary scope normalization without dropping either input.
		scope = types.RuntimeArtifactScopeUnspecified
		warnings = append(warnings, "runtime_artifact_scope_profile.source_quote is missing or not verbatim in the current request: normalized requested_scope from full_artifact to unspecified; preserved validated comparison.baseline/current. No whole-artifact completeness or numeric-window authority is granted.")
	}
	profile := &types.RuntimeArtifactScopeProfile{RequestedScope: scope, Comparison: pair, SourceQuote: quote,
		TimeWindows: windows, Confidence: *p.Confidence, Rationale: strings.TrimSpace(p.Rationale)}
	if shared {
		profile.TimeStart, profile.TimeEnd = sharedWindow.TimeStart, sharedWindow.TimeEnd
	}
	return profile, "", warnings
}

// Exact path-token validation only. A path prefix (a.data in a.data.backup)
// is not a second source; no request-intent keywords enter this check.
func runtimeComparisonQuoteHasPath(quote, path string) bool {
	if path == "" {
		return false
	}
	boundary := func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune("`\"'<>[]()，。,;；", r) }
	for offset := 0; offset <= len(quote)-len(path); {
		i := strings.Index(quote[offset:], path)
		if i < 0 {
			return false
		}
		i += offset
		before, after := i == 0, i+len(path) == len(quote)
		if !before {
			r, _ := utf8.DecodeLastRuneInString(quote[:i])
			before = boundary(r)
		}
		if !after {
			r, _ := utf8.DecodeRuneInString(quote[i+len(path):])
			after = boundary(r)
		}
		if before && after {
			return true
		}
		offset = i + len(path)
	}
	return false
}
