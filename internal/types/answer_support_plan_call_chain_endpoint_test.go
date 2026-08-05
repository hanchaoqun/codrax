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

func TestCallChainOrderedEndpointProfileOwnsDirectionAndIdentityPriority(t *testing.T) {
	rm := RequestModel{
		CallChainEndpointProfile: &CallChainEndpointProfile{Source: "Source.run", Sink: "Sink.run"},
		PredicateAxis:            AxisCall,
		AnalyzerHints: AnalyzerHints{
			Kind:         string(ReqCallChain),
			ExactTargets: []string{"Sink.run", "Source.run", "Context.helper"},
		},
	}
	if source, sink, ok := CallChainOrderedEndpointHints(rm); !ok || source != "Source.run" || sink != "Sink.run" {
		t.Fatalf("ordered endpoints=%q,%q,%t", source, sink, ok)
	}
	want := []string{"Source.run", "Sink.run"}
	if got := CallChainRequestedEndpointHints(rm); !reflect.DeepEqual(got, want) {
		t.Fatalf("identity hints=%v want ordered profile pair %v", got, want)
	}
}

func TestNormalizeCallChainEndpointProfilePreservesOrderedDiscoveredEndpointWithWarning(t *testing.T) {
	profile, reason := NormalizeCallChainEndpointProfile(
		&CallChainEndpointProfile{Source: "Sink.run", Sink: "Source.run"},
		[]string{"Source.run", "Sink.run"},
	)
	if reason != "" || profile == nil || profile.Source != "Sink.run" || profile.Sink != "Source.run" {
		t.Fatalf("normalization must preserve model-authored direction: profile=%+v reason=%q", profile, reason)
	}
	if got, reason := NormalizeCallChainEndpointProfile(
		&CallChainEndpointProfile{Source: "Source.run", Sink: "Resolved.run"},
		[]string{"Source.run", "Sink.run"},
	); got == nil || got.Source != "Source.run" || got.Sink != "Resolved.run" || reason == "" {
		t.Fatalf("source-discovered endpoint must remain ordered but soft-warned: profile=%+v reason=%q", got, reason)
	}
	if got, reason := NormalizeCallChainEndpointProfile(
		&CallChainEndpointProfile{Source: "Source.run", Sink: "Rust implementation"},
		[]string{"Source.run"},
	); got != nil || reason == "" {
		t.Fatalf("free-form semantic role is not a code endpoint: profile=%+v reason=%q", got, reason)
	}
}

func TestCallChainEndpointCompatibleUsesQualifiedIdentitySegments(t *testing.T) {
	for _, tc := range []struct {
		candidate string
		endpoint  string
		want      bool
	}{
		{candidate: "Foo::run", endpoint: "Foo.run", want: true},
		{candidate: "Foo#run", endpoint: "Foo.run", want: true},
		{candidate: "run", endpoint: "Foo::run", want: true},
		{candidate: "Other::run", endpoint: "Foo.run", want: false},
		{candidate: "Foo.runWith", endpoint: "Foo.run", want: false},
		{candidate: "goroutine", endpoint: "go", want: false},
	} {
		if got := CallChainEndpointCompatible(tc.candidate, tc.endpoint); got != tc.want {
			t.Errorf("CallChainEndpointCompatible(%q,%q)=%t want %t", tc.candidate, tc.endpoint, got, tc.want)
		}
	}
}
