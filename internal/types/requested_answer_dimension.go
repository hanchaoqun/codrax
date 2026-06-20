package types

import (
	"sort"
	"strings"
)

// RequestedAnswerDimensionRole is a language-neutral role for a visible answer
// dimension the user explicitly asked to see. Most roles are presentation
// metadata. A required current_key_code role can participate in source-lane
// decisions only when paired with typed source-scope and external-observation
// policy state; labels or rationale text are never inspected for hard routing.
type RequestedAnswerDimensionRole string

const (
	RequestedAnswerDimensionUnknown           RequestedAnswerDimensionRole = ""
	RequestedAnswerDimensionDiffClue          RequestedAnswerDimensionRole = "diff_clue"
	RequestedAnswerDimensionCurrentKeyCode    RequestedAnswerDimensionRole = "current_key_code"
	RequestedAnswerDimensionFunctionOrPurpose RequestedAnswerDimensionRole = "function_or_purpose"
	RequestedAnswerDimensionImpact            RequestedAnswerDimensionRole = "impact"
	RequestedAnswerDimensionComparisonAxis    RequestedAnswerDimensionRole = "comparison_axis"
	RequestedAnswerDimensionEvidenceSource    RequestedAnswerDimensionRole = "evidence_source"
	RequestedAnswerDimensionBoundary          RequestedAnswerDimensionRole = "boundary"
	RequestedAnswerDimensionStageWorkflow     RequestedAnswerDimensionRole = "stage_or_workflow"
	RequestedAnswerDimensionOther             RequestedAnswerDimensionRole = "other"
)

func AllRequestedAnswerDimensionRoles() []RequestedAnswerDimensionRole {
	return []RequestedAnswerDimensionRole{
		RequestedAnswerDimensionDiffClue,
		RequestedAnswerDimensionCurrentKeyCode,
		RequestedAnswerDimensionFunctionOrPurpose,
		RequestedAnswerDimensionImpact,
		RequestedAnswerDimensionComparisonAxis,
		RequestedAnswerDimensionEvidenceSource,
		RequestedAnswerDimensionBoundary,
		RequestedAnswerDimensionStageWorkflow,
		RequestedAnswerDimensionOther,
	}
}

func (r RequestedAnswerDimensionRole) IsValid() bool {
	if r == RequestedAnswerDimensionUnknown {
		return true
	}
	for _, declared := range AllRequestedAnswerDimensionRoles() {
		if r == declared {
			return true
		}
	}
	return false
}

func NormalizeRequestedAnswerDimensionRole(s string) RequestedAnswerDimensionRole {
	role := RequestedAnswerDimensionRole(strings.TrimSpace(s))
	if role.IsValid() && role != RequestedAnswerDimensionUnknown {
		return role
	}
	return RequestedAnswerDimensionOther
}

// RequestedAnswerDimension is one current-request-visible output axis, such as
// "diff 线索", "当前关键代码", "作用", or "影响". The label is user-facing and
// may be any language; Role is the small typed carrier downstream can rank.
type RequestedAnswerDimension struct {
	Label       string                       `json:"label"`
	Role        RequestedAnswerDimensionRole `json:"role,omitempty"`
	SourceQuote string                       `json:"source_quote,omitempty"`
	Required    bool                         `json:"required,omitempty"`
	Index       int                          `json:"index,omitempty"`
}

// RequestedAnswerDimensionProfile is analyzer-emitted presentation guidance.
// It is deliberately soft: invalid or unanchored entries are dropped, and the
// absence of the profile never blocks analysis or finalization.
type RequestedAnswerDimensionProfile struct {
	IsDimensionedAnswer bool                       `json:"is_dimensioned_answer"`
	Dimensions          []RequestedAnswerDimension `json:"dimensions,omitempty"`
	Confidence          float64                    `json:"confidence,omitempty"`
	Rationale           string                     `json:"rationale,omitempty"`
}

func (p *RequestedAnswerDimensionProfile) Active() bool {
	return p != nil && p.IsDimensionedAnswer && len(p.Dimensions) > 0
}

// NormalizeRequestedAnswerDimensionProfile validates user provenance and
// dedupes dimensions. A dimension survives when either source_quote or label is
// anchored in the current request; this keeps the profile useful when the label
// itself is the quoted phrase.
func NormalizeRequestedAnswerDimensionProfile(raw string, in *RequestedAnswerDimensionProfile) (*RequestedAnswerDimensionProfile, []string) {
	if in == nil {
		return nil, nil
	}
	var warnings []string
	if !in.IsDimensionedAnswer {
		return nil, warnings
	}
	seen := make(map[string]struct{}, len(in.Dimensions))
	out := make([]RequestedAnswerDimension, 0, len(in.Dimensions))
	for _, dim := range in.Dimensions {
		label := strings.TrimSpace(dim.Label)
		if label == "" {
			warnings = append(warnings, "requested_answer_dimensions ignored entry with empty label")
			continue
		}
		quote := strings.TrimSpace(dim.SourceQuote)
		anchored := false
		if quote != "" && requestedEnumerationQuotePresent(raw, quote) {
			anchored = true
		}
		if !anchored && requestedEnumerationQuotePresent(raw, label) {
			quote = label
			anchored = true
		}
		if !anchored {
			warnings = append(warnings, "requested_answer_dimensions ignored unanchored dimension "+label)
			continue
		}
		role := dim.Role
		if !role.IsValid() || role == RequestedAnswerDimensionUnknown {
			role = RequestedAnswerDimensionOther
		}
		key := strings.ToLower(string(role)) + "\x00" + strings.ToLower(label)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, RequestedAnswerDimension{
			Label:       label,
			Role:        role,
			SourceQuote: quote,
			Required:    dim.Required,
			Index:       dim.Index,
		})
	}
	if len(out) == 0 {
		warnings = append(warnings, "requested_answer_dimensions ignored because no dimension survived current-request provenance validation")
		return nil, warnings
	}
	sort.SliceStable(out, func(i, j int) bool {
		li := out[i].Index
		lj := out[j].Index
		if li <= 0 {
			li = i + 1
		}
		if lj <= 0 {
			lj = j + 1
		}
		return li < lj
	})
	for i := range out {
		if out[i].Index <= 0 {
			out[i].Index = i + 1
		}
	}
	confidence := in.Confidence
	if confidence < 0 {
		confidence = 0
	} else if confidence > 1 {
		confidence = 1
	}
	return &RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions:          out,
		Confidence:          confidence,
		Rationale:           strings.TrimSpace(in.Rationale),
	}, warnings
}
