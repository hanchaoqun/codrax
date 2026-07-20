package types

import (
	"sort"
	"strconv"
	"strings"
)

// RequestedAnswerDimensionRole is a language-neutral role for a visible answer
// dimension the user explicitly asked to see. Most roles are presentation
// metadata. Required current_key_code and runtime mechanism roles can
// participate in source-lane decisions only when paired with typed
// source/external-observation state; rationale text is never inspected for hard
// routing.
type RequestedAnswerDimensionRole string

const (
	RequestedAnswerDimensionUnknown           RequestedAnswerDimensionRole = ""
	RequestedAnswerDimensionDiffClue          RequestedAnswerDimensionRole = "diff_clue"
	RequestedAnswerDimensionCurrentKeyCode    RequestedAnswerDimensionRole = "current_key_code"
	RequestedAnswerDimensionFunctionOrPurpose RequestedAnswerDimensionRole = "function_or_purpose"
	RequestedAnswerDimensionImpact            RequestedAnswerDimensionRole = "impact"
	RequestedAnswerDimensionComparisonAxis    RequestedAnswerDimensionRole = "comparison_axis"
	RequestedAnswerDimensionCount             RequestedAnswerDimensionRole = "count"
	RequestedAnswerDimensionMemberSet         RequestedAnswerDimensionRole = "member_set"
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
		RequestedAnswerDimensionCount,
		RequestedAnswerDimensionMemberSet,
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

// CurrentSourceObligationSignalKind records why a source-lane obligation had
// to be preserved outside the soft presentation profile.
type CurrentSourceObligationSignalKind string

const (
	CurrentSourceObligationSignalUnknown                   CurrentSourceObligationSignalKind = ""
	CurrentSourceObligationSignalDroppedRequestedDimension CurrentSourceObligationSignalKind = "dropped_requested_dimension"
)

// CurrentSourceObligationSignal is a compact, typed routing/audit carrier for
// current-source obligations that cannot be represented by a survived
// RequestedAnswerDimension. It intentionally stores no free-form label or
// rationale, so hard gates cannot grow prose/keyword dependencies. Because no
// text survives on the carrier, anchor precision is certified at mint time
// (§29.166 OBLSWEEP-1): CurrentSourceObligationSignalsFromRequestedDimensions
// mints only from dropped dimensions whose quote/label carries a PRECISE
// current-source text anchor, which is what entitles the hard consumer faces
// (verification-anchor arm, tier-1 floor, completion landing, source-audit
// debt) to trust Active() bare.
type CurrentSourceObligationSignal struct {
	Kind  CurrentSourceObligationSignalKind `json:"kind,omitempty"`
	Role  RequestedAnswerDimensionRole      `json:"role,omitempty"`
	Index int                               `json:"index,omitempty"`
}

func (s CurrentSourceObligationSignal) Active() bool {
	return s.Kind == CurrentSourceObligationSignalDroppedRequestedDimension &&
		RequestedAnswerDimensionRoleCarriesCurrentSourceObligation(s.Role)
}

func RequestedAnswerDimensionRoleCarriesCurrentSourceObligation(role RequestedAnswerDimensionRole) bool {
	switch role {
	case RequestedAnswerDimensionCurrentKeyCode,
		RequestedAnswerDimensionFunctionOrPurpose:
		return true
	default:
		return false
	}
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

func CurrentSourceObligationSignalsFromRequestedDimensions(raw []RequestedAnswerDimension, normalized *RequestedAnswerDimensionProfile) []CurrentSourceObligationSignal {
	if len(raw) == 0 {
		return nil
	}
	var out []CurrentSourceObligationSignal
	seen := map[string]struct{}{}
	for _, dim := range raw {
		role := dim.Role
		if !role.IsValid() || role == RequestedAnswerDimensionUnknown {
			role = RequestedAnswerDimensionOther
		}
		if !dim.Required || !RequestedAnswerDimensionRoleCarriesCurrentSourceObligation(role) {
			continue
		}
		if requestedAnswerDimensionSurvivedNormalization(dim, role, normalized) {
			continue
		}
		// §29.166 OBLSWEEP-1 (same family as §29.146 UPSTREAM-3 件3 and
		// §29.151 UPTAIL-1 件3/件4): the dropped-dimension lane used to mint
		// this hard-gate carrier from a bare Required∧Role word-face that had
		// ALREADY failed current-request provenance anchoring — strictly
		// weaker-gated than the survived lane, whose identical word-face mints
		// a verification anchor only with a precise current-source text
		// anchor. The carrier deliberately stores no text, so precision cannot
		// be re-checked at consumption; this mint site is the single point. A
		// prose-only dropped dimension stays on the advisory lane (the
		// normalization warning is its only trace); a dropped dimension whose
		// quote/label carries a code/config path suffix or a parsed file:line
		// surface keeps minting, so real obligations dropped by presentation
		// normalization are still preserved. Pure relaxation: every consumer
		// face treats the signal as added obligation, never as permission.
		if !requestedAnswerDimensionHasPreciseCurrentSourceAnchor(dim) {
			continue
		}
		key := string(role) + "\x00" + normalizedDimensionIndexKey(dim.Index)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, CurrentSourceObligationSignal{
			Kind:  CurrentSourceObligationSignalDroppedRequestedDimension,
			Role:  role,
			Index: dim.Index,
		})
	}
	return out
}

func requestedAnswerDimensionSurvivedNormalization(raw RequestedAnswerDimension, role RequestedAnswerDimensionRole, normalized *RequestedAnswerDimensionProfile) bool {
	if normalized == nil || !normalized.Active() {
		return false
	}
	rawLabel := strings.TrimSpace(raw.Label)
	for _, dim := range normalized.Dimensions {
		if dim.Role != role {
			continue
		}
		if raw.Index > 0 && dim.Index == raw.Index {
			return true
		}
		if rawLabel != "" && strings.EqualFold(strings.TrimSpace(dim.Label), rawLabel) {
			return true
		}
	}
	return false
}

func normalizedDimensionIndexKey(index int) string {
	if index <= 0 {
		return ""
	}
	return strconv.Itoa(index)
}
