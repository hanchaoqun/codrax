package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeArtifactWorkProfileSchema() map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"description": skill.AnalysisRuntimeArtifactWorkTeaching,
		"properties": map[string]any{
			"mode":                    map[string]any{"type": "string", "enum": []string{"representative", "exhaustive"}},
			"per_specification_limit": map[string]any{"type": "integer", "minimum": 0, "description": "Explicit positive representative budget per distinct target/window/query specification, not total members; required for representative and omitted or zero for exhaustive."},
			"source_quote":            map[string]any{"type": "string", "minLength": 1, "description": "Contiguous verbatim current-request phrase establishing this collection-work policy and its representative budget."},
		},
		"required": []string{"mode", "source_quote"},
		"allOf": []any{
			map[string]any{"if": map[string]any{"properties": map[string]any{"mode": map[string]any{"const": "representative"}}, "required": []string{"mode"}}, "then": map[string]any{"required": []string{"per_specification_limit"}, "properties": map[string]any{"per_specification_limit": map[string]any{"minimum": 1}}}},
			map[string]any{"if": map[string]any{"properties": map[string]any{"mode": map[string]any{"const": "exhaustive"}}, "required": []string{"mode"}}, "then": map[string]any{"properties": map[string]any{"per_specification_limit": map[string]any{"const": 0}}}},
		},
	}
}

// parseRuntimeArtifactWorkProfile validates the independent planning carrier.
// Neither attachment availability nor a prose-derived classifier substitutes
// for its declared mode. No query, directory access, or answer duty is minted.
func parseRuntimeArtifactWorkProfile(raw string, p *types.RuntimeArtifactWorkProfile) (*types.RuntimeArtifactWorkProfile, string) {
	if p == nil {
		return nil, ""
	}
	copy := types.CloneRuntimeArtifactWorkProfile(p)
	copy.SourceQuote = strings.TrimSpace(copy.SourceQuote)
	const field = "runtime_artifact_work_profile"
	if copy.Mode != types.RuntimeArtifactWorkRepresentative && copy.Mode != types.RuntimeArtifactWorkExhaustive {
		return nil, field + ".mode must be representative or exhaustive; omit this optional profile when no explicit collection-work policy was requested."
	}
	if !copy.Valid() {
		return nil, field + " requires a nonempty source_quote, a positive integer per_specification_limit for representative, and no budget for exhaustive. Repair this profile in the complete emit_analysis object; preserve target/window and unrelated decisions."
	}
	if !sourceQuotePresentInCurrentRequest(raw, copy.SourceQuote) {
		return nil, field + ".source_quote must occur verbatim in the CURRENT request, not history, tool output, or an answer. Preserve the semantic integer; no ASCII numeral is required in this quote."
	}
	return copy, ""
}
