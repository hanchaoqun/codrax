package tool

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeArtifactComparisonSchema() map[string]any {
	side := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"path", "time_start", "time_end", "source_quote"}, "properties": map[string]any{
		"path": map[string]any{"type": "string"}, "time_start": map[string]any{"type": "number", "minimum": 0}, "time_end": map[string]any{"type": "number", "minimum": 0}, "source_quote": map[string]any{"type": "string"},
	}}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"baseline", "current"}, "properties": map[string]any{"baseline": side, "current": side}, "description": skill.AnalysisRuntimeComparisonTeaching}
}

func parseRuntimeArtifactComparisonScope(raw string, p *emitRuntimeArtifactScopeProfileParam) (*types.RuntimeArtifactScopeProfile, string, []string) {
	fail := func(reason string) (*types.RuntimeArtifactScopeProfile, string, []string) {
		return nil, "runtime_artifact_scope_profile.comparison: " + reason, nil
	}
	if p.RequestedScope != string(types.RuntimeArtifactScopeExplicitWindow) || p.TimeWindows != nil || p.TimeStart != nil || p.TimeEnd != nil {
		return fail("use explicit_time_window and only the two source-owned comparison members, not pooled time_windows/scalar endpoints")
	}
	if p.Confidence == nil || *p.Confidence < 0 || *p.Confidence > 1 || !p.Comparison.Valid() {
		return fail("both distinct paths, finite ordered windows, local source quotes and confidence in [0,1] are required")
	}
	pair := p.Comparison.Clone()
	var warnings []string
	sides := []*types.RuntimeArtifactComparisonSide{&pair.Baseline, &pair.Current}
	for i, side := range sides {
		side.Path, side.SourceQuote = strings.TrimSpace(side.Path), strings.TrimSpace(side.SourceQuote)
		if !sourceQuotePresentInCurrentRequest(raw, side.SourceQuote) || !runtimeComparisonQuoteHasPath(side.SourceQuote, side.Path) || runtimeComparisonQuoteHasPath(side.SourceQuote, sides[1-i].Path) {
			return fail(fmt.Sprintf("side %d needs one local verbatim quote containing its own exact original path and interval, not the other side's path", i+1))
		}
		bound, warning, err := bindRuntimeWindowSource(side.RuntimeArtifactTimeWindow)
		if err != nil {
			return fail(fmt.Sprintf("side %d: %v", i+1, err))
		}
		side.RuntimeArtifactTimeWindow = bound
		if warning != "" {
			warnings = append(warnings, warning)
		}
	}
	return &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, Comparison: pair,
		TimeWindows: []types.RuntimeArtifactTimeWindow{pair.Baseline.RuntimeArtifactTimeWindow, pair.Current.RuntimeArtifactTimeWindow}, Confidence: *p.Confidence, Rationale: strings.TrimSpace(p.Rationale)}, "", warnings
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
