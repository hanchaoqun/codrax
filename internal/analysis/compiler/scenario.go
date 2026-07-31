package compiler

import "github.com/hanchaoqun/codrax/internal/types"

// InferScenario is a deterministic scenario classifier used when the
// LLM did not supply rm.Scenario (or supplied ScenarioGeneric). It
// inspects Intent first and then falls back to term-graph heuristics,
// so adding LLM-side scenario tagging later never regresses the rule
// path.
//
// Priority (highest wins):
//
//  1. Active failure-scope decision answer    → generic
//  2. Intent is explicitly security-oriented  → security_audit
//  3. Intent is refactor/bugfix               → refactor_design
//  4. Intent is config_query                  → config_trace
//  5. Intent is return_value/enumerate/trace  → architecture_explain
//     (unless root_cause signals are present — see step 6)
//  6. Intent is root_cause                    → root_cause
//  7. Term graph contains perf-related terms  → performance_bottleneck
//  8. Scoped runtime-only fact/comparison      → generic
//  9. otherwise                               → architecture_explain
func InferScenario(rm types.RequestModel) types.Scenario {
	if types.IsFailureScopeDecisionAnswer(rm) {
		return types.ScenarioGeneric
	}
	if types.HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		return types.ScenarioGeneric
	}
	switch rm.Intent {
	case types.IntentConfigQuery:
		return types.ScenarioConfigTrace
	case types.IntentRootCause:
		return types.ScenarioRootCause
	case types.IntentTrace:
		if types.IsSingleTopicStructuralTrace(rm) {
			return types.ScenarioGeneric
		}
		return types.ScenarioArchitectureExplain
	case types.IntentEnumerate, types.IntentReturnValue:
		// Enumerate/return_value questions produce short answers (a
		// list of symbols or a single value) that rarely need the
		// heavy architecture_explain template with its reconcile node
		// and citation_count_ge=3 gate. Route to the lightweight
		// generic template (citation_count_ge=1).
		return types.ScenarioGeneric
	}
	if hasPerfTerms(rm.TermGraph) {
		return types.ScenarioPerformanceBottleneck
	}
	// A validated runtime-artifact scope plus explicit current-source
	// exclusion is an observation task, not a source-architecture task.
	// Keep root-cause/config/performance scenarios above this branch, and
	// let explicit trace call relations resolve their answer family
	// independently. This branch reads only typed enums/validated carriers;
	// it never scans request or answer prose. In particular, an explicit
	// trace time window remains available to the causal-report materializer
	// even though the surrounding scenario is generic.
	if rm.HasScopedRuntimeArtifactWithoutRequiredCurrentSource() {
		return types.ScenarioGeneric
	}
	// Ambiguity-sensitive escalation: when the request is
	// non-trivial AND carries at least one ambiguity clause, route
	// to a scenario whose template has a reconcile node so the DAG
	// schedules a branch-collapse step. architecture_explain is the
	// only scenario with a reconcile node that is not already
	// picked above.
	if hasReconcileWorthyAmbiguity(rm) {
		return types.ScenarioArchitectureExplain
	}
	return types.ScenarioArchitectureExplain
}

// hasReconcileWorthyAmbiguity reports whether the request has at
// least one non-trivial ambiguity clause AND the request's
// complexity is moderate or complex. Simple requests never need
// reconciliation regardless of how many ambiguities the LLM lists.
func hasReconcileWorthyAmbiguity(rm types.RequestModel) bool {
	if rm.Complexity != types.ComplexityModerate && rm.Complexity != types.ComplexityComplex {
		return false
	}
	return types.HasNonEmptyAmbiguity(rm)
}

// perfTerms are canonical IDs the normalizer emits for concepts that
// indicate the user is asking about throughput/latency/memory.
// Matching any of them flips the fallback scenario to
// performance_bottleneck. Kept deliberately narrow to avoid
// accidentally routing architecture questions through the perf
// template.
var perfTerms = map[string]bool{
	"en:performance": true,
	"en:latency":     true,
	"en:throughput":  true,
	"en:slow":        true,
	"en:bottleneck":  true,
	"zh:性能":          true,
	"zh:瓶颈":          true,
	"zh:延迟":          true,
	"zh:吞吐":          true,
}

func hasPerfTerms(tg types.TermGraph) bool {
	for _, c := range tg.Canonical {
		if perfTerms[c.ID] {
			return true
		}
	}
	return false
}
