package types

// RuntimeQuestionScope is the analyzer's typed declaration of what kind of
// answer a runtime artifact question requests. It is intentionally orthogonal
// to artifact range, target identity, relation shape, and legacy labels:
// those labels varied across identical eval replays and must not silently
// widen a finite fact lookup into a causal performance report.
type RuntimeQuestionScope string

const (
	RuntimeQuestionScopeNotApplicable    RuntimeQuestionScope = "not_applicable"
	RuntimeQuestionScopeBoundedFactSet   RuntimeQuestionScope = "bounded_fact_set"
	RuntimeQuestionScopeCausalDiagnosis  RuntimeQuestionScope = "causal_diagnosis"
	RuntimeQuestionScopeRelationAnalysis RuntimeQuestionScope = "relation_analysis"
	RuntimeQuestionScopeSystemOverview   RuntimeQuestionScope = "system_overview"
	RuntimeQuestionScopeUnspecified      RuntimeQuestionScope = "unspecified"
)

func AllRuntimeQuestionScopes() []RuntimeQuestionScope {
	return []RuntimeQuestionScope{
		RuntimeQuestionScopeNotApplicable,
		RuntimeQuestionScopeBoundedFactSet,
		RuntimeQuestionScopeCausalDiagnosis,
		RuntimeQuestionScopeRelationAnalysis,
		RuntimeQuestionScopeSystemOverview,
		RuntimeQuestionScopeUnspecified,
	}
}

func (s RuntimeQuestionScope) IsValid() bool {
	for _, candidate := range AllRuntimeQuestionScopes() {
		if s == candidate {
			return true
		}
	}
	return false
}

// RuntimeQuestionProfile carries the current request's runtime answer scope.
// SourceQuote is an exact request anchor validated by emit_analysis. Consumers
// use only Scope; they never scan the quote, raw request, or model prose.
type RuntimeQuestionProfile struct {
	Scope       RuntimeQuestionScope `json:"scope"`
	SourceQuote string               `json:"source_quote,omitempty"`
	Confidence  float64              `json:"confidence,omitempty"`
	Rationale   string               `json:"rationale,omitempty"`
}

func (p *RuntimeQuestionProfile) BoundedFactSet() bool {
	return p != nil && p.Scope == RuntimeQuestionScopeBoundedFactSet
}

func (p *RuntimeQuestionProfile) RequiresFullReport() bool {
	if p == nil {
		return false
	}
	switch p.Scope {
	case RuntimeQuestionScopeCausalDiagnosis,
		RuntimeQuestionScopeRelationAnalysis,
		RuntimeQuestionScopeSystemOverview:
		return true
	default:
		return false
	}
}
