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

	// Adjacent negative controls: removing the target or asking for a call
	// relation restores the report authority.
	bus.AnalysisIR.RequestModel.RuntimeTargets = nil
	if !runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("a target-less root-cause diagnostic still needs the full trace report")
	}
	bus = runtimeConditionalFactBusForTest()
	bus.AnalysisIR.RequestModel.AnalyzerHints.Kind = string(types.ReqCallChain)
	bus.AnalysisIR.RequestModel.PredicateAxis = types.AxisCall
	if !runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("an explicit call relation still needs causal materialization")
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

	rm.DiagnosticProfile.IsDiagnostic = true
	windowStart, windowEnd := 34579.45, 34579.50
	rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &windowStart,
		TimeEnd:        &windowEnd,
		SourceQuote:    "34579.45..34579.50",
	}
	if !runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("explicit-window diagnostic explain/mechanism must retain full trace report authority")
	}

	rm.DiagnosticProfile.IsDiagnostic = false
	rm.Intent = types.IntentRootCause
	if !runtimeTraceFullReportMaterializationAllowed(bus) {
		t.Fatal("explicit-window root-cause mechanism must retain full trace report authority")
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
}

func assertFunctionsUseRuntimeFactShapeGate(t *testing.T, path string, names []string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, name := range names {
		start := strings.Index(source, "func "+name)
		if start < 0 {
			t.Fatalf("missing materializer %s in %s", name, path)
		}
		body := source[start:]
		if next := strings.Index(body[len("func "):], "\nfunc "); next >= 0 {
			body = body[:len("func ")+next]
		}
		if !strings.Contains(body, "runtimeTraceFullReportMaterializationAllowed(ctx)") {
			t.Fatalf("%s bypasses the shared narrow-fact answer-shape gate", name)
		}
	}
}
