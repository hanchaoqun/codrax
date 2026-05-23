package types

import "strings"

// CurrentSourceExplanationMode names why a non-source observation needs to be
// related back to the current checkout. The enum is intentionally
// language-neutral: Go, Java, C++, ArkTS, Cangjie, logs, traces, git, command
// output, MCP resources, and connector data all use the same request lane.
type CurrentSourceExplanationMode string

const (
	CurrentSourceExplanationUnknown                 CurrentSourceExplanationMode = ""
	CurrentSourceExplanationExplainCurrentMechanism CurrentSourceExplanationMode = "explain_current_mechanism"
	CurrentSourceExplanationVerifyCurrentStatus     CurrentSourceExplanationMode = "verify_current_status"
	CurrentSourceExplanationTraceCurrentFlow        CurrentSourceExplanationMode = "trace_current_flow"
	CurrentSourceExplanationCompareWithCurrent      CurrentSourceExplanationMode = "compare_with_current_source"
	CurrentSourceExplanationLocateCurrentCode       CurrentSourceExplanationMode = "locate_current_code"
	CurrentSourceExplanationAssessCurrentImpact     CurrentSourceExplanationMode = "assess_current_impact"
	CurrentSourceExplanationOther                   CurrentSourceExplanationMode = "other"
)

func AllCurrentSourceExplanationModes() []CurrentSourceExplanationMode {
	return []CurrentSourceExplanationMode{
		CurrentSourceExplanationExplainCurrentMechanism,
		CurrentSourceExplanationVerifyCurrentStatus,
		CurrentSourceExplanationTraceCurrentFlow,
		CurrentSourceExplanationCompareWithCurrent,
		CurrentSourceExplanationLocateCurrentCode,
		CurrentSourceExplanationAssessCurrentImpact,
		CurrentSourceExplanationOther,
	}
}

func (m CurrentSourceExplanationMode) IsValid() bool {
	if m == CurrentSourceExplanationUnknown {
		return true
	}
	for _, declared := range AllCurrentSourceExplanationModes() {
		if m == declared {
			return true
		}
	}
	return false
}

func NormalizeCurrentSourceExplanationMode(s string) CurrentSourceExplanationMode {
	mode := CurrentSourceExplanationMode(strings.TrimSpace(s))
	if mode.IsValid() && mode != CurrentSourceExplanationUnknown {
		return mode
	}
	return CurrentSourceExplanationOther
}

// CurrentSourceExplanationProfile is the analyzer's soft typed request lane for
// mixed questions where an external/non-source observation should be explained,
// verified, traced, compared, located, or impact-assessed against the current
// checkout. It opens/request-ranks the current-source evidence lane; it is not a
// presentation dimension, not an evidence item, and not permission for
// deterministic answer replacement.
type CurrentSourceExplanationProfile struct {
	IsCurrentSourceExplanationRequested bool                           `json:"is_current_source_explanation_requested"`
	Modes                               []CurrentSourceExplanationMode `json:"modes,omitempty"`
	SourceQuotes                        []string                       `json:"source_quotes,omitempty"`
	TargetTerms                         []string                       `json:"target_terms,omitempty"`
	Confidence                          float64                        `json:"confidence,omitempty"`
	Rationale                           string                         `json:"rationale,omitempty"`
}

func (p *CurrentSourceExplanationProfile) Active() bool {
	return p != nil &&
		p.IsCurrentSourceExplanationRequested &&
		len(p.SourceQuotes) > 0
}

// NormalizeCurrentSourceExplanationProfile validates current-request
// provenance and dedupes advisory fields. Invalid optional input is dropped
// with warnings instead of becoming an analyze-stage retry.
func NormalizeCurrentSourceExplanationProfile(raw string, in *CurrentSourceExplanationProfile) (*CurrentSourceExplanationProfile, []string) {
	if in == nil {
		return nil, nil
	}
	var warnings []string
	if !in.IsCurrentSourceExplanationRequested {
		return nil, warnings
	}
	sourceQuotes := normalizeCurrentSourceExplanationQuotes(raw, in.SourceQuotes, &warnings)
	if len(sourceQuotes) == 0 {
		warnings = append(warnings, "current_source_explanation_profile ignored because no source_quote survived current-request provenance validation")
		return nil, warnings
	}
	modes := normalizeCurrentSourceExplanationModes(in.Modes)
	if len(modes) == 0 {
		modes = []CurrentSourceExplanationMode{CurrentSourceExplanationOther}
	}
	confidence := in.Confidence
	if confidence < 0 {
		confidence = 0
	} else if confidence > 1 {
		confidence = 1
	}
	return &CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: true,
		Modes:                               modes,
		SourceQuotes:                        sourceQuotes,
		TargetTerms:                         normalizeCurrentSourceExplanationTerms(in.TargetTerms),
		Confidence:                          confidence,
		Rationale:                           strings.TrimSpace(in.Rationale),
	}, warnings
}

func normalizeCurrentSourceExplanationQuotes(raw string, quotes []string, warnings *[]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(quotes))
	for _, quote := range quotes {
		trimmed := strings.TrimSpace(quote)
		if trimmed == "" {
			continue
		}
		if !requestedEnumerationQuotePresent(raw, trimmed) {
			if warnings != nil {
				*warnings = append(*warnings, "current_source_explanation_profile ignored unanchored source_quote "+trimmed)
			}
			continue
		}
		key := strings.ToLower(foldEnumerationBoundarySurface(trimmed))
		if key == "" {
			key = strings.ToLower(trimmed)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func normalizeCurrentSourceExplanationModes(modes []CurrentSourceExplanationMode) []CurrentSourceExplanationMode {
	seen := map[CurrentSourceExplanationMode]struct{}{}
	out := make([]CurrentSourceExplanationMode, 0, len(modes))
	for _, mode := range modes {
		normalized := NormalizeCurrentSourceExplanationMode(string(mode))
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func normalizeCurrentSourceExplanationTerms(terms []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		trimmed := strings.TrimSpace(term)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
