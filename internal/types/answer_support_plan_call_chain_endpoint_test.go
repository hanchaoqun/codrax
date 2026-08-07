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

func TestNormalizeCallChainEndpointProfileDemotesAnalyzerDiscoveredSink(t *testing.T) {
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
	); got == nil || !got.DiscoverSinkActive() || got.Source != "Source.run" || got.Sink != "" || reason == "" {
		t.Fatalf("source-discovered endpoint must demote to discover mode: profile=%+v reason=%q", got, reason)
	}
	if got, reason := NormalizeCallChainEndpointProfile(
		&CallChainEndpointProfile{Source: "Source.run", Sink: "Rust implementation"},
		[]string{"Source.run"},
	); got == nil || !got.DiscoverSinkActive() || got.Source != "Source.run" || reason == "" {
		t.Fatalf("named source plus semantic destination role should use runtime discovery: profile=%+v reason=%q", got, reason)
	}
	if got, reason := NormalizeCallChainEndpointProfile(
		&CallChainEndpointProfile{Source: "Resolved.entry", Sink: "Resolved.run"},
		[]string{"Source.run"},
	); got == nil || !got.DiscoverPathActive() || reason == "" {
		t.Fatalf("analyzer-resolved source and sink must become authority-free path discovery: profile=%+v reason=%q", got, reason)
	}
}

func TestNormalizeCallChainEndpointProfile_RoleBoundPathCarriesNoEndpointAuthority(t *testing.T) {
	profile, reason := NormalizeCallChainEndpointProfile(
		&CallChainEndpointProfile{Source: "main", Sink: "HttpTransport", SinkMode: CallChainSinkResolutionExact},
		[]string{"@app/core"},
	)
	if profile == nil || !profile.DiscoverPathActive() || reason == "" {
		t.Fatalf("pre-scan endpoint guesses must normalize to discover_path: profile=%+v reason=%q", profile, reason)
	}
	rm := RequestModel{CallChainEndpointProfile: profile, AnalyzerHints: AnalyzerHints{Kind: string(ReqCallChain)}}
	if source, sink, ok := CallChainOrderedEndpointHints(rm); ok || source != "" || sink != "" {
		t.Fatalf("role-bound path must not mint ordered endpoint authority: %q %q %t", source, sink, ok)
	}
	if got := CallChainRequestedEndpointHints(rm); len(got) != 0 {
		t.Fatalf("role-bound path must not publish guessed endpoint hints: %v", got)
	}
	anchors := CompileRequiredMechanismAnchors(rm, AnswerContract{}, QFCallChain, nil)
	if len(anchors) != 0 {
		t.Fatalf("role-bound path must not create required answer anchors: %+v", anchors)
	}
}

func TestNormalizeCallChainEndpointProfile_DiscoverSourceRequiresCurrentRequestProvenance(t *testing.T) {
	profile, reason := NormalizeCallChainEndpointProfile(
		&CallChainEndpointProfile{Source: "Invented.entry", SinkMode: CallChainSinkResolutionDiscover},
		[]string{"plugin loading"},
	)
	if profile == nil || !profile.DiscoverPathActive() || reason == "" {
		t.Fatalf("unproven discover source must lose hard authority: profile=%+v reason=%q", profile, reason)
	}
}

func TestCallChainEndpointProfileDiscoverSinkKeepsCandidateOutOfDirectionAuthority(t *testing.T) {
	profile, reason := NormalizeCallChainEndpointProfile(
		&CallChainEndpointProfile{Source: "run_pipeline", Sink: "JsonPlugin.handle", SinkMode: CallChainSinkResolutionDiscover},
		[]string{"run_pipeline"},
	)
	if profile == nil || !profile.DiscoverSinkActive() || profile.Sink != "" || reason == "" {
		t.Fatalf("discover normalization must discard preselected candidate with an audit warning: profile=%+v reason=%q", profile, reason)
	}
	rm := RequestModel{CallChainEndpointProfile: profile}
	if source, sink, ok := CallChainOrderedEndpointHints(rm); ok || source != "" || sink != "" {
		t.Fatalf("discover profile must not mint an exact ordered pair: %q %q %t", source, sink, ok)
	}
	if got := CallChainRequestedEndpointHints(rm); !reflect.DeepEqual(got, []string{"run_pipeline"}) {
		t.Fatalf("discover presentation hints=%v, want source only", got)
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
