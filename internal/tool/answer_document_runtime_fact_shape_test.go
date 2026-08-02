package tool

import (
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeConditionalFactBusForTest() *types.BusContext {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentRootCause,
		PredicateAxis: types.AxisCondition,
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqConditional)},
		RuntimeTargets: []types.RuntimeTarget{{
			Kind:   types.RuntimeTargetKindThread,
			PID:    59566,
			Thread: "com.baidu.tieba",
			Source: "user_explicit",
		}},
	}}
	return bus
}

func TestRuntimeConditionalFactSuppressesFullTraceReportShape(t *testing.T) {
	bus := runtimeConditionalFactBusForTest()
	if runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("a typed target-state fact must not inherit the full trace report")
	}
	if !runtimeTracePrincipalValueMaterializationAllowed(bus) {
		t.Fatal("a typed target-state fact must retain narrow principal-value publication")
	}

	// Adjacent negative controls: removing the target or asking for a call
	// relation restores the report authority.
	bus.AnalysisIR.RequestModel.RuntimeTargets = nil
	if !runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("a target-less root-cause diagnostic still needs the full trace report")
	}
	if !runtimeTracePrincipalValueMaterializationAllowed(bus) {
		t.Fatal("a full trace report also permits its principal-value card")
	}
	bus = runtimeConditionalFactBusForTest()
	bus.AnalysisIR.RequestModel.AnalyzerHints.Kind = string(types.ReqCallChain)
	bus.AnalysisIR.RequestModel.PredicateAxis = types.AxisCall
	if !runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("an explicit call relation still needs causal materialization")
	}
	bus.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet}
	if runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("a declared finite call-relation fact set must stay narrow")
	}
	if !runtimeTracePrincipalValueMaterializationAllowed(bus) {
		t.Fatal("a finite call-relation fact set must retain principal typed values")
	}
}

func TestRuntimeExplainMechanismFactSuppressesFullTraceReportShape(t *testing.T) {
	bus := runtimeConditionalFactBusForTest()
	rm := &bus.AnalysisIR.RequestModel
	rm.Intent = types.IntentExplain
	rm.AnalyzerHints.Kind = string(types.ReqMechanism)
	rm.PredicateAxis = types.AxisUnknown
	rm.Predicates.IsDiagnosticQuestion = false
	rm.DiagnosticProfile.IsDiagnostic = false
	if runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("non-diagnostic explain/mechanism target fact must not inherit the full trace report")
	}
	if !runtimeTracePrincipalValueMaterializationAllowed(bus) {
		t.Fatal("non-diagnostic explain/mechanism target fact must retain narrow principal values")
	}

	windowStart, windowEnd := 34579.45, 34579.50
	rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &windowStart,
		TimeEnd:        &windowEnd,
		SourceQuote:    "34579.45..34579.50",
	}
	if !runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("exact explicit-window explain/mechanism must retain full trace report authority regardless of diagnostic-label variation")
	}

	rm.RuntimeArtifactScopeProfile.SourceQuote = ""
	if runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("unanchored explicit-window profile must not widen a focused runtime fact")
	}

	rm.RuntimeArtifactScopeProfile.SourceQuote = "34579.45..34579.50"
	rm.Intent = types.IntentRootCause
	if !runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("explicit-window root-cause mechanism must retain full trace report authority")
	}
}

func TestRuntimeExplainConditionalFactSuppressesCollectedCausalRows(t *testing.T) {
	bus := runtimeConditionalFactBusForTest()
	rm := &bus.AnalysisIR.RequestModel
	rm.Intent = types.IntentExplain
	rm.AnalyzerHints.Kind = string(types.ReqConditional)
	rm.PredicateAxis = types.AxisCondition
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "root_cause_primary",
			ClaimKey:        "root_cause_primary:runnable_delay",
			Subject:         "other-thread-9",
			Value:           "3.2",
			Unit:            "ms",
			RichNotes:       []string{"chain_relevance=on_chain", "causality=on_wakeup_chain", "impact_ms=3.2"},
		}},
	}}
	if runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("focused explain/conditional fact must not be widened by incidentally collected causal rows")
	}
	if !runtimeTracePrincipalValueMaterializationAllowed(bus) {
		t.Fatal("focused explain/conditional fact must retain deterministic principal values")
	}
}

func TestRuntimeGenericArtifactComparisonRequiresCausalRowsForFullReport(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentExplain,
		Scenario: types.ScenarioGeneric,
		Predicates: types.SemanticPredicates{
			IsCrossComponent: true,
		},
	}}
	if runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("generic artifact comparison without a root/chain carrier must stay on the requested comparison surface")
	}

	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:              "root-1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "root_cause_primary",
			ClaimKey:        "root_cause_primary:runnable_delay",
			Subject:         "ui-thread-100",
			Value:           "3.2",
			Unit:            "ms",
			RichNotes: []string{
				"chain_relevance=on_chain",
				"causality=on_wakeup_chain",
				"impact_ms=3.2",
			},
		}},
	}}
	if !runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("publication-grade typed causal rows must retain the full report for a generic family")
	}
}

// This tripwire keeps every system-authored report surface on the one shared
// answer-shape predicate. It checks wiring rather than prose: adding a new
// materializer without the gate would reintroduce the same class of narrow
// query expansion on only one output surface.
func TestRuntimeConditionalFactReportMaterializersShareShapeGate(t *testing.T) {
	assertFunctionsUseRuntimeFactShapeGate(t, "answer_document_mutation_runtime.go", []string{
		"materializeRuntimeTraceCausalProjectionBlock",
		"materializeRuntimeTraceSemanticOptimizationBlock",
		"materializeRuntimeTraceMetricSnapshotBlock",
		"materializeRuntimeTraceNextStepsBlock",
		"materializeRuntimeTracePerfQualityBlock",
		"materializeRuntimeTraceObservationBlock",
	})
	assertFunctionsUseRuntimeFactShapeGate(t, "answer_document_mutation_runtime_frequency.go", []string{
		"materializeRuntimeTraceFrequencyAuthorityCaveat",
		"materializeRuntimeTraceVsyncAuthorityCaveat",
	})
	assertFunctionsUseRuntimeFactShapeGate(t, "answer_document_mutation_runtime_wait_coverage.go", []string{
		"materializeRuntimeTraceBlockingCoverageAuthorityCaveat",
		"materializeRuntimeTraceBlockedReasonCensusCaliberCaveat",
	})
	assertFunctionsUseNamedShapeGate(
		t,
		"answer_document_mutation_runtime_wait_coverage.go",
		"materializeRuntimeTraceTargetStateAuthorityBlock",
		"runtimeTracePrincipalValueMaterializationAllowed(ctx)",
	)
}

func assertFunctionsUseRuntimeFactShapeGate(t *testing.T, path string, names []string) {
	t.Helper()
	for _, name := range names {
		assertFunctionsUseNamedShapeGate(t, path, name, "runtimeTraceFullReportMaterializationAllowed(ctx)")
	}
}

func assertFunctionsUseNamedShapeGate(t *testing.T, path, name, gate string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "func "+name)
	if start < 0 {
		t.Fatalf("missing materializer %s in %s", name, path)
	}
	body := source[start:]
	if next := strings.Index(body[len("func "):], "\nfunc "); next >= 0 {
		body = body[:len("func ")+next]
	}
	if !strings.Contains(body, gate) {
		t.Fatalf("%s bypasses answer-shape gate %s", name, gate)
	}
}
