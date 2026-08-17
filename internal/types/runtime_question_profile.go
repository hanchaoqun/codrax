package types

import "strings"

// RuntimeQuestionScope is the analyzer's typed declaration of what kind of
// answer a runtime artifact question requests. It is intentionally orthogonal
// to artifact range, target identity, relation shape, and legacy labels:
// those labels varied across identical eval replays and must not silently
// widen a finite fact lookup into a causal performance report.
type RuntimeQuestionScope string

const (
	RuntimeQuestionScopeNotApplicable        RuntimeQuestionScope = "not_applicable"
	RuntimeQuestionScopeBoundedFactSet       RuntimeQuestionScope = "bounded_fact_set"
	RuntimeQuestionScopeBoundedEffectVerdict RuntimeQuestionScope = "bounded_effect_verdict"
	RuntimeQuestionScopeCausalDiagnosis      RuntimeQuestionScope = "causal_diagnosis"
	RuntimeQuestionScopeRelationAnalysis     RuntimeQuestionScope = "relation_analysis"
	RuntimeQuestionScopeSystemOverview       RuntimeQuestionScope = "system_overview"
	RuntimeQuestionScopeUnspecified          RuntimeQuestionScope = "unspecified"
)

func AllRuntimeQuestionScopes() []RuntimeQuestionScope {
	return []RuntimeQuestionScope{
		RuntimeQuestionScopeNotApplicable,
		RuntimeQuestionScopeBoundedFactSet,
		RuntimeQuestionScopeBoundedEffectVerdict,
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
	// RuntimeQuestionFactIOLatency is the finite latency-caliber lane for
	// storage requests/operations. It covers exact issue-to-complete request
	// residence and, when the completion-to-issuer wake closure is proven, the
	// issuer's completion-closed blocked wall clock. The two rulers remain
	// separate and non-additive. This is intentionally distinct from generic
	// scheduler wait occurrences and from aggregate resource pressure.
	RuntimeQuestionFactIOLatency          RuntimeQuestionFactFamily = "io_latency"
	RuntimeQuestionFactResourcePressure   RuntimeQuestionFactFamily = "resource_pressure"
	RuntimeQuestionFactFrequencyResidency RuntimeQuestionFactFamily = "frequency_residency"
	RuntimeQuestionFactOtherObservedValue RuntimeQuestionFactFamily = "other_observed_value"
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
		RuntimeQuestionFactIOLatency,
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

// BoundedEffectVerdict reports a finite condition/constraint-to-target verdict.
// The constraining mechanism may be named or may remain unresolved. Unlike a
// causal diagnosis, this shape does not request a root-cause roster, wakeup
// chain, or system-wide causal projection. It keeps the requested observed
// fact families beside the model-owned yes/no/mixed/unproven verdict. The
// requested answer role paired with this scope is target_effect_verdict;
// causal_attribution is reserved for full cause/mechanism discovery.
func (p *RuntimeQuestionProfile) BoundedEffectVerdict() bool {
	return p != nil && p.Scope == RuntimeQuestionScopeBoundedEffectVerdict
}

// CarriesBoundedFactFamilies identifies the two finite runtime answer shapes
// whose exact principal-value cards are selected by FactFamilies.
func (p *RuntimeQuestionProfile) CarriesBoundedFactFamilies() bool {
	return p != nil && (p.BoundedFactSet() || p.BoundedEffectVerdict())
}

// RequestsFactFamily reports whether a finite runtime answer explicitly owns
// one semantic fact family. Consumers use this typed declaration instead of
// inferring scope from request words, query view names, or model prose.
func (p *RuntimeQuestionProfile) RequestsFactFamily(want RuntimeQuestionFactFamily) bool {
	if p == nil || !p.CarriesBoundedFactFamilies() || !want.IsValid() {
		return false
	}
	for _, family := range p.FactFamilies {
		if family == want {
			return true
		}
	}
	return false
}

// RuntimeObservationRecordFactFamilies maps deterministic trace-query record
// shapes to the finite semantic fact families they can directly carry.  This
// is a typed prompt-projection index only: it never changes ledger truth,
// causal rank, wakeup-chain membership, or a model-owned verdict.  Keeping the
// mapping here gives every bounded prompt consumer one source of truth instead
// of re-deriving relevance from request/answer prose or tool-view names.
func RuntimeObservationRecordFactFamilies(record ObservationRecord) []RuntimeQuestionFactFamily {
	predicate := strings.ToLower(strings.TrimSpace(record.Predicate))
	add := func(out []RuntimeQuestionFactFamily, family RuntimeQuestionFactFamily) []RuntimeQuestionFactFamily {
		for _, existing := range out {
			if existing == family {
				return out
			}
		}
		return append(out, family)
	}
	var out []RuntimeQuestionFactFamily
	switch predicate {
	case "target_window_states", "state_churn", "state_drilldown":
		out = add(out, RuntimeQuestionFactTargetSchedulerState)
	case "target_window_wait_occurrences":
		out = add(out, RuntimeQuestionFactTargetWaitOccurrences)
		out = add(out, RuntimeQuestionFactCountOrDuration)
	case "target_window_wait_occurrence":
		out = add(out, RuntimeQuestionFactTargetWaitOccurrences)
		out = add(out, RuntimeQuestionFactOccurrenceTime)
		out = add(out, RuntimeQuestionFactCountOrDuration)
	case "blocked_reason_census":
		out = add(out, RuntimeQuestionFactRecordedReason)
		out = add(out, RuntimeQuestionFactCountOrDuration)
	case "io_latency", "io_latency_coverage", "storage_latency_by_layer", "block_io_by_inode":
		out = add(out, RuntimeQuestionFactIOLatency)
	case "cpu_frequency_limit", "frequency_tier_census":
		out = add(out, RuntimeQuestionFactFrequencyResidency)
	case "ipc_request_census":
		out = add(out, RuntimeQuestionFactCountOrDuration)
	case "ipc_request_edge":
		out = add(out, RuntimeQuestionFactRelationPeer)
		out = add(out, RuntimeQuestionFactTransactionID)
	case "wakeup_edge_census", "wakeup_chain_edge":
		out = add(out, RuntimeQuestionFactRelationPeer)
		out = add(out, RuntimeQuestionFactDirectWaker)
	}
	// The typed IO-pressure producer deliberately keeps its signal token in
	// Predicate for historical projection compatibility.  The exact type note
	// and producer-owned row ID identify that family without inspecting prose.
	if traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyType) == "io_pressure" &&
		strings.Contains(strings.ToLower(strings.TrimSpace(record.ID)), "#io_pressure:") {
		out = add(out, RuntimeQuestionFactResourcePressure)
	}
	if traceObservationDimension(record) == TraceObservationDimensionResourcePressure {
		out = add(out, RuntimeQuestionFactResourcePressure)
	}
	return out
}

// RuntimeObservationRecordMatchesRequestedFactFamily reports whether a
// deterministic observation is a direct carrier for at least one fact family
// selected by a finite runtime-question profile.  It is suitable only for
// soft prompt ordering/presentation; it is not an answer or completion gate.
func RuntimeObservationRecordMatchesRequestedFactFamily(record ObservationRecord, profile *RuntimeQuestionProfile) bool {
	if profile == nil || !profile.CarriesBoundedFactFamilies() {
		return false
	}
	for _, family := range RuntimeObservationRecordFactFamilies(record) {
		if profile.RequestsFactFamily(family) {
			return true
		}
	}
	return false
}

// SuppressesRootCauseRankingPrompt reports that the current typed answer
// breadth does not authorize a root-cause roster as model-facing context.
// The underlying observation ledger remains lossless for audit and later
// deterministic consumers; this method controls only prompt projection.
//
// Keeping this decision on the typed profile is important: consumers must not
// infer it from request keywords, trace-query view names, investigation prose,
// or a draft answer. An explicitly causal diagnosis keeps the full ranking
// surface. Finite fact/effect, relation-only, and overview questions receive
// their own requested evidence without an exploration-time ranking becoming a
// de facto conclusion.
func (p *RuntimeQuestionProfile) SuppressesRootCauseRankingPrompt() bool {
	if p == nil {
		return false
	}
	switch p.Scope {
	case RuntimeQuestionScopeBoundedFactSet,
		RuntimeQuestionScopeBoundedEffectVerdict,
		RuntimeQuestionScopeRelationAnalysis,
		RuntimeQuestionScopeSystemOverview:
		return true
	default:
		return false
	}
}

// RequestsTraceWaitEvidencePrompt reports whether a typed runtime question
// needs the detailed kernel-wait / wakeup evidence feed. Causal, relation, and
// overview scopes retain that feed. A finite scope receives it only when one
// of its declared fact families actually asks for wait occurrences, recorded
// reasons/times, peers, transactions, or direct wakers. Count/duration alone
// is intentionally insufficient: it may refer to running time or frequency
// residency and must not pull an unrelated wakeup/root-cause appendix into the
// prompt.
func (p *RuntimeQuestionProfile) RequestsTraceWaitEvidencePrompt() bool {
	if p == nil {
		return true
	}
	switch p.Scope {
	case RuntimeQuestionScopeCausalDiagnosis,
		RuntimeQuestionScopeRelationAnalysis,
		RuntimeQuestionScopeSystemOverview,
		RuntimeQuestionScopeUnspecified:
		return true
	case RuntimeQuestionScopeBoundedFactSet, RuntimeQuestionScopeBoundedEffectVerdict:
		for _, family := range p.FactFamilies {
			switch family {
			case RuntimeQuestionFactTargetWaitOccurrences,
				RuntimeQuestionFactRecordedReason,
				RuntimeQuestionFactOccurrenceTime,
				RuntimeQuestionFactRelationPeer,
				RuntimeQuestionFactTransactionID,
				RuntimeQuestionFactDirectWaker,
				RuntimeQuestionFactIOLatency:
				return true
			}
		}
		return false
	default:
		return false
	}
}

// RequestsTargetWaitOccurrences resolves the typed semantic closure for a
// bounded target-wait question. The analyzer should emit the dedicated
// target_wait_occurrences family directly, but model emissions can describe
// the same finite request as the conjunction of target state, count/duration,
// and a recorded reason or occurrence timestamps. That conjunction is precise
// enough to authorize an exact engine-paired wait roster when one exists. It
// does not inspect request/answer prose and does not widen state-only,
// count-only, or relation-only questions.
func (p *RuntimeQuestionProfile) RequestsTargetWaitOccurrences() bool {
	if p == nil || !p.CarriesBoundedFactFamilies() {
		return false
	}
	has := make(map[RuntimeQuestionFactFamily]bool, len(p.FactFamilies))
	for _, family := range p.FactFamilies {
		has[family] = true
	}
	if has[RuntimeQuestionFactTargetWaitOccurrences] {
		return true
	}
	return has[RuntimeQuestionFactTargetSchedulerState] &&
		has[RuntimeQuestionFactCountOrDuration] &&
		(has[RuntimeQuestionFactRecordedReason] || has[RuntimeQuestionFactOccurrenceTime])
}

// RequestsBlockedReasonCensus is the narrow typed authorization for a
// bounded answer to expose the kernel blocked-reason inventory. Requiring
// both axes prevents a reason-only question from inheriting extra numeric
// cards and a count-only question from inheriting kernel-callsite detail.
func (p *RuntimeQuestionProfile) RequestsBlockedReasonCensus() bool {
	if p == nil || !p.CarriesBoundedFactFamilies() {
		return false
	}
	hasReason, hasCountOrDuration := false, false
	for _, family := range p.FactFamilies {
		switch family {
		case RuntimeQuestionFactRecordedReason:
			hasReason = true
		case RuntimeQuestionFactCountOrDuration:
			hasCountOrDuration = true
		}
	}
	return hasReason && hasCountOrDuration
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
