package types

import (
	"reflect"
	"strings"
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

func TestProjectCallChainEndpointBoundarySupportPlan_KeepsOnlyBoundaryEdges(t *testing.T) {
	plan := &AnswerSupportPlan{Family: QFCallChain, Lanes: []AnswerSupportLane{
		{Kind: SupportLaneCurrentCodePath, Entries: []AnswerSupportEntry{
			{EvidenceID: "sibling", ClaimForm: ClaimCallEdge, Subject: "Source.run", Object: "Other.work"},
			{EvidenceID: "source", ClaimForm: ClaimCallEdge, Subject: "Source.run", Object: "Shared.work"},
			{EvidenceID: "sink", ClaimForm: ClaimCallEdge, Subject: "Sink.run", Object: "Shared.work"},
			{EvidenceID: "definition", ClaimForm: ClaimDefinitionFact, Subject: "Sink.run"},
		}},
		{Kind: SupportLaneUncertaintyBound, Entries: []AnswerSupportEntry{{Text: "no directed path"}}},
	}}
	boundary := &CallChainEndpointBoundary{
		Disposition:    CallChainEndpointNoDirectedPath,
		SourceEndpoint: "Source.run",
		RequestedSink:  "Sink.run",
		EvidenceCapsule: &CallChainEndpointEvidenceCapsule{
			Status:     CallChainEndpointEvidenceSharedCalleeBoundary,
			SourcePath: []CallChainEvidenceEdge{{From: "Source.run", To: "Shared.work"}},
			SinkPath:   []CallChainEvidenceEdge{{From: "Sink.run", To: "Shared.work"}},
		},
	}
	got := projectCallChainEndpointBoundarySupportPlan(plan, boundary)
	if got == nil || len(got.Lanes) != 2 || len(got.Lanes[0].Entries) != 2 {
		t.Fatalf("endpoint-boundary support projection = %+v", got)
	}
	if got.Lanes[0].Entries[0].EvidenceID != "source" || got.Lanes[0].Entries[1].EvidenceID != "sink" {
		t.Fatalf("principal path retained non-boundary rows: %+v", got.Lanes[0].Entries)
	}
	if !strings.Contains(got.Lanes[0].Guidance, "only the grounded call edges that explain that boundary") {
		t.Fatalf("boundary guidance missing: %q", got.Lanes[0].Guidance)
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
	); got == nil || !got.DiscoverTerminalActive() || got.Source != "Source.run" || got.Sink != "" || reason == "" {
		t.Fatalf("conceptual endpoint guessed by analysis must demote to static terminal discovery: profile=%+v reason=%q", got, reason)
	}
	if got, reason := NormalizeCallChainEndpointProfile(
		&CallChainEndpointProfile{Source: "Source.run", Sink: "Rust implementation"},
		[]string{"Source.run"},
	); got == nil || !got.DiscoverTerminalActive() || got.Source != "Source.run" || reason == "" {
		t.Fatalf("named source plus semantic destination role should use terminal discovery: profile=%+v reason=%q", got, reason)
	}
	if got, reason := NormalizeCallChainEndpointProfile(
		&CallChainEndpointProfile{Source: "Resolved.entry", Sink: "Resolved.run"},
		[]string{"Source.run"},
	); got == nil || !got.DiscoverPathActive() || reason == "" {
		t.Fatalf("analyzer-resolved source and sink must become authority-free path discovery: profile=%+v reason=%q", got, reason)
	}
}

func TestNormalizeCallChainEndpointProfile_UnprovenSinkKeepsRuntimeSelectionSeparate(t *testing.T) {
	profile, reason := NormalizeCallChainEndpointProfile(
		&CallChainEndpointProfile{
			Source:                      "Source.run",
			Sink:                        "ConcretePlugin.handle",
			SinkMode:                    CallChainSinkResolutionExact,
			RuntimeSelectionRequired:    true,
			RuntimeSelectionSourceQuote: "which runtime plugin is selected",
		},
		[]string{"Source.run"},
	)
	if profile == nil || !profile.DiscoverSinkActive() || !profile.RequiresRuntimeSelectionEvidence() || reason == "" {
		t.Fatalf("an independently declared runtime-selection request must retain discover semantics: profile=%+v reason=%q", profile, reason)
	}
	if profile.DiscoverTerminalActive() {
		t.Fatalf("runtime-selected implementation must not be weakened to static terminal discovery: %+v", profile)
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

func TestNormalizeCallChainEndpointProfile_DemotionPreservesIndependentRuntimeSelectionObligation(t *testing.T) {
	profile, reason := NormalizeCallChainEndpointProfile(
		&CallChainEndpointProfile{
			Source:                      "main",
			Sink:                        "ConsoleSink",
			SinkMode:                    CallChainSinkResolutionExact,
			RuntimeSelectionRequired:    true,
			RuntimeSelectionSourceQuote: "运行时具体的 sink 是如何被选择出来的",
		},
		[]string{"runtime backend path"},
	)
	if profile == nil || !profile.DiscoverPathActive() || reason == "" {
		t.Fatalf("endpoint candidates should demote to discover_path: profile=%+v reason=%q", profile, reason)
	}
	if !profile.RequiresRuntimeSelectionEvidence() ||
		profile.RuntimeSelectionSourceQuote != "运行时具体的 sink 是如何被选择出来的" {
		t.Fatalf("endpoint demotion must not erase the independent selection contract: %+v", profile)
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

func TestNormalizeCallChainEndpointProfile_DiscoverFilePathDemotesToDiscoverPath(t *testing.T) {
	profile, reason := NormalizeCallChainEndpointProfile(
		&CallChainEndpointProfile{Source: "packages/cli/src/main.ts", SinkMode: CallChainSinkResolutionDiscover},
		[]string{"packages/cli/src/main.ts"},
	)
	if profile == nil || !profile.DiscoverPathActive() || reason == "" {
		t.Fatalf("navigation path must lose endpoint authority without rejecting the role-bound investigation: profile=%+v reason=%q", profile, reason)
	}
	if !strings.Contains(reason, "not an exact current-request code identity") ||
		!strings.Contains(reason, "normalized to discover_path") {
		t.Fatalf("demotion reason must explain the precise authority boundary: %q", reason)
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
