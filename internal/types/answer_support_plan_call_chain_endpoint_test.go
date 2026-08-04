package types

import (
	"reflect"
	"testing"
)

func TestCallChainRequestedEndpointHints_PrefersExactTargets(t *testing.T) {
	rm := RequestModel{AnalyzerHints: AnalyzerHints{
		ExactTargets:      []string{"Controller::start", "Gate::run", "internal/agent/analyzer.go"},
		MentionedEntities: []string{"Nearby.helper", "Controller::start", "Gate::run"},
	}}
	want := []string{"Controller::start", "Gate::run"}
	if got := CallChainRequestedEndpointHints(rm); !reflect.DeepEqual(got, want) {
		t.Fatalf("endpoint hints=%v, want exact typed endpoints %v", got, want)
	}
}

func TestCallChainRequestedEndpointHints_PathOnlyExactTargetsFallBackToTypedEntities(t *testing.T) {
	rm := RequestModel{AnalyzerHints: AnalyzerHints{
		ExactTargets:      []string{"internal/agent/analyzer.go"},
		MentionedEntities: []string{"buildAnalysisIR", "gate.Run", "orchestrator.go"},
	}}
	want := []string{"buildAnalysisIR", "gate.Run"}
	if got := CallChainRequestedEndpointHints(rm); !reflect.DeepEqual(got, want) {
		t.Fatalf("endpoint hints=%v, want typed entity fallback %v", got, want)
	}
}
