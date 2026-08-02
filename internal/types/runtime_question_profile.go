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

// RuntimeQuestionFactFamily is the analyzer's typed declaration of which
// principal observed values a bounded runtime question actually asks for.
// Scope answers "how broad"; these values answer "which facts". Keeping the
// two axes separate prevents a finite IPC peer/waker lookup from inheriting a
// scheduler-state card merely because both questions name one runtime target.
//
// The enum is deliberately semantic and source-format independent. Consumers
// must not infer these families from request keywords, trace-query views, or
// model-authored answer prose.
type RuntimeQuestionFactFamily string

const (
	RuntimeQuestionFactTargetSchedulerState  RuntimeQuestionFactFamily = "target_scheduler_state"
	RuntimeQuestionFactTargetWaitOccurrences RuntimeQuestionFactFamily = "target_wait_occurrences"
	RuntimeQuestionFactRecordedReason        RuntimeQuestionFactFamily = "recorded_reason"
	RuntimeQuestionFactOccurrenceTime        RuntimeQuestionFactFamily = "occurrence_time"
	RuntimeQuestionFactCountOrDuration       RuntimeQuestionFactFamily = "count_or_duration"
	RuntimeQuestionFactRelationPeer          RuntimeQuestionFactFamily = "relation_peer"
	RuntimeQuestionFactTransactionID         RuntimeQuestionFactFamily = "transaction_id"
	RuntimeQuestionFactDirectWaker           RuntimeQuestionFactFamily = "direct_waker"
	RuntimeQuestionFactResourcePressure      RuntimeQuestionFactFamily = "resource_pressure"
	RuntimeQuestionFactFrequencyResidency    RuntimeQuestionFactFamily = "frequency_residency"
	RuntimeQuestionFactOtherObservedValue    RuntimeQuestionFactFamily = "other_observed_value"
)

func AllRuntimeQuestionFactFamilies() []RuntimeQuestionFactFamily {
	return []RuntimeQuestionFactFamily{
		RuntimeQuestionFactTargetSchedulerState,
		RuntimeQuestionFactTargetWaitOccurrences,
		RuntimeQuestionFactRecordedReason,
		RuntimeQuestionFactOccurrenceTime,
		RuntimeQuestionFactCountOrDuration,
		RuntimeQuestionFactRelationPeer,
		RuntimeQuestionFactTransactionID,
		RuntimeQuestionFactDirectWaker,
		RuntimeQuestionFactResourcePressure,
		RuntimeQuestionFactFrequencyResidency,
		RuntimeQuestionFactOtherObservedValue,
	}
}

func (f RuntimeQuestionFactFamily) IsValid() bool {
	for _, candidate := range AllRuntimeQuestionFactFamilies() {
		if f == candidate {
			return true
		}
	}
	return false
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
// use only the typed scope/families; they never scan the quote, raw request,
// or model prose.
type RuntimeQuestionProfile struct {
	Scope        RuntimeQuestionScope        `json:"scope"`
	FactFamilies []RuntimeQuestionFactFamily `json:"fact_families,omitempty"`
	SourceQuote  string                      `json:"source_quote,omitempty"`
	Confidence   float64                     `json:"confidence,omitempty"`
	Rationale    string                      `json:"rationale,omitempty"`
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

func (p *RuntimeQuestionProfile) RequestsTargetStatePrincipalValues() bool {
	if p == nil {
		return false
	}
	for _, family := range p.FactFamilies {
		switch family {
		case RuntimeQuestionFactTargetSchedulerState,
			RuntimeQuestionFactTargetWaitOccurrences:
			return true
		}
	}
	return false
}
