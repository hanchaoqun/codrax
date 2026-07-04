package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// semantic_ruling_pins_test.go — RN-16 (§7.9) layer 2, display side: the
// causal-token registry (internal/tracequery/causal_token_registry.go) is the
// single source of truth; these pins lock the tool-side consumers to it.
// Family index lives in internal/tracequery/semantic_ruling_pins_test.go.

// TestSemanticRulingPin_CrossThreadDisplaySetMatchesRegistry pins the second
// half of the two-set interlock: for EVERY registered token, the display
// classifier runtimeTraceProjCrossThreadAggregateType (which drives bar-scale
// exclusion and the "(跨线程累计,非墙钟)" + CMP-9 normalized-density
// annotation) agrees exactly with the registry's cross_thread_cpu_ms row set,
// modulo the documented exception list. A token can therefore never gain or
// lose cross-thread semantics on the display face without a registry (and
// golden) edit — and vice versa.
func TestSemanticRulingPin_CrossThreadDisplaySetMatchesRegistry(t *testing.T) {
	for _, token := range tracequery.CausalTokenUniverse() {
		spec, _ := tracequery.CausalTokenSpecFor(token)
		want := spec.RowToken &&
			spec.Additivity == tracequery.CausalAdditivityCrossThreadCPUms &&
			!tracequery.CausalTokenCrossThreadRowException(token)

		aggregate := types.TraceCausalProjectionNode{
			SubjectKind: types.TraceCausalSubjectKindAggregateMetric,
			TypeToken:   token,
		}
		if got := runtimeTraceProjCrossThreadAggregateType(aggregate); got != want {
			t.Errorf("token %q (TypeToken lane): display cross-thread classifier = %v, registry says %v — the two sets are interlocked, change both or neither", token, got, want)
		}

		// Object-fallback lane (nodes whose TypeToken is empty).
		objectLane := types.TraceCausalProjectionNode{
			SubjectKind: types.TraceCausalSubjectKindAggregateMetric,
			Object:      token,
		}
		if got := runtimeTraceProjCrossThreadAggregateType(objectLane); got != want {
			t.Errorf("token %q (Object fallback lane): display cross-thread classifier = %v, registry says %v", token, got, want)
		}

		// Without the typed aggregate-metric marker the classifier must stay
		// false for every token: rows with a real thread subject keep their
		// wall-clock bar treatment (CMP-3 — the marker is one of the two
		// required precise signals).
		threadRow := types.TraceCausalProjectionNode{TypeToken: token}
		if runtimeTraceProjCrossThreadAggregateType(threadRow) {
			t.Errorf("token %q: classifier fired without the typed aggregate-metric subject_kind marker", token)
		}
	}
}

// TestSemanticRulingPin_ZhLabelColumnMatchesRegistry pins the registry's
// LabelZhRef column to the actual display helpers (labels are deliberately
// NOT migrated into the registry — this pin is the lockstep):
//   - CausalZhLabelRefRootCauseType   → runtimeTraceRootCauseTypeZHLabel owns
//     a non-empty zh label for the token;
//   - CausalZhLabelRefSupplyPressure  → the token renders through
//     runtimeTraceSupplyPressureDisplayLabel (§7.5 demand-backlog relabel);
//   - ""                              → no zh label helper: the token renders
//     verbatim.
func TestSemanticRulingPin_ZhLabelColumnMatchesRegistry(t *testing.T) {
	for _, token := range tracequery.CausalTokenUniverse() {
		spec, _ := tracequery.CausalTokenSpecFor(token)
		got := runtimeTraceRootCauseTypeZHLabel(token)
		switch spec.LabelZhRef {
		case tracequery.CausalZhLabelRefRootCauseType:
			if got == "" {
				t.Errorf("token %q: registry says its zh label is owned by runtimeTraceRootCauseTypeZHLabel but the helper returns \"\"", token)
			}
		case tracequery.CausalZhLabelRefSupplyPressure:
			if got != runtimeTraceSupplyPressureDisplayLabel(true) {
				t.Errorf("token %q: registry says its zh label is owned by runtimeTraceSupplyPressureDisplayLabel, helper returned %q", token, got)
			}
		case "":
			if got != "" {
				t.Errorf("token %q: registry says no zh label helper (verbatim) but runtimeTraceRootCauseTypeZHLabel returns %q — update the registry LabelZhRef column (and golden)", token, got)
			}
		default:
			t.Errorf("token %q: unknown LabelZhRef %q", token, spec.LabelZhRef)
		}
	}
}
