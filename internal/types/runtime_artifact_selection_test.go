package types

import "testing"

func TestRuntimeArtifactSelectionView_RawRequestOnlyDoesNotCreateArtifact(t *testing.T) {
	view := RuntimeArtifactSelectionViewFromAgentContext(&AgentContext{
		AnalysisIR: &AnalysisIR{RequestModel: RequestModel{
			RawRequest: "分析 /tmp/customer.systrace 里的卡顿",
		}},
	})
	if view.Active || len(view.Items) != 0 {
		t.Fatalf("raw request text must not create runtime artifact selection view: %+v", view)
	}
	if view.Policy.Kind != RuntimeArtifactAnalysisPolicyNone {
		t.Fatalf("raw-only policy kind = %q, want empty", view.Policy.Kind)
	}
}

func TestRuntimeArtifactSelectionView_SingleTraceOnlyExactPolicy(t *testing.T) {
	view := RuntimeArtifactSelectionViewFromAgentContext(&AgentContext{
		RuntimeArtifactPreflight: NormalizeRuntimeArtifactPreflightProfile(RuntimeArtifactPreflightProfile{
			Artifacts: []RuntimeArtifactPreflightArtifact{{
				Kind:    "trace",
				Source:  "/tmp/customer.systrace",
				Carrier: "request_path",
			}},
		}),
		AnalysisIR: &AnalysisIR{RequestModel: RequestModel{
			ExternalObservationPolicy: traceOnlyExternalObservationPolicy(),
		}},
	})
	if !view.Active || view.TraceCount != 1 || view.LogCount != 0 {
		t.Fatalf("trace selection counts = %+v", view)
	}
	if view.Policy.Kind != RuntimeArtifactAnalysisPolicyTraceOnlyExactArtifact {
		t.Fatalf("policy kind = %q, want %q: %+v", view.Policy.Kind, RuntimeArtifactAnalysisPolicyTraceOnlyExactArtifact, view)
	}
	if view.Policy.ActiveArtifactSource != "/tmp/customer.systrace" || view.Policy.ActiveArtifactID == "" {
		t.Fatalf("active trace identity not projected: %+v", view.Policy)
	}
}

func TestRuntimeArtifactSelectionView_MultipleTraceOnlyIsAmbiguous(t *testing.T) {
	view := RuntimeArtifactSelectionViewFromAgentContext(&AgentContext{
		RuntimeArtifactPreflight: NormalizeRuntimeArtifactPreflightProfile(RuntimeArtifactPreflightProfile{
			Artifacts: []RuntimeArtifactPreflightArtifact{
				{Kind: "trace", Source: "/tmp/a.systrace", Carrier: "request_path"},
				{Kind: "trace", Source: "/tmp/b.systrace", Carrier: "request_path"},
			},
		}),
		AnalysisIR: &AnalysisIR{RequestModel: RequestModel{
			ExternalObservationPolicy: traceOnlyExternalObservationPolicy(),
		}},
	})
	if view.TraceCount != 2 {
		t.Fatalf("trace count = %d, want 2: %+v", view.TraceCount, view)
	}
	if view.Policy.Kind != RuntimeArtifactAnalysisPolicyTraceArtifactAmbiguous {
		t.Fatalf("multi-trace policy kind = %q, want ambiguity: %+v", view.Policy.Kind, view.Policy)
	}
	if view.Policy.ActiveArtifactID != "" {
		t.Fatalf("ambiguous trace policy must not silently choose an active artifact: %+v", view.Policy)
	}
}

func TestRuntimeArtifactSelectionView_TraceAndLogStaySeparated(t *testing.T) {
	view := RuntimeArtifactSelectionViewFromAgentContext(&AgentContext{
		RuntimeArtifactPreflight: NormalizeRuntimeArtifactPreflightProfile(RuntimeArtifactPreflightProfile{
			Artifacts: []RuntimeArtifactPreflightArtifact{
				{Kind: "trace", Source: "/tmp/a.systrace", Carrier: "request_path"},
				{Kind: "log", Source: "/tmp/app.log", Carrier: "request_path"},
			},
		}),
	})
	if view.TraceCount != 1 || view.LogCount != 1 {
		t.Fatalf("runtime artifact counts = %+v, want one trace and one log", view)
	}
	if view.Policy.Kind != RuntimeArtifactAnalysisPolicySelectionAdvisory {
		t.Fatalf("mixed runtime artifacts should render advisory policy, got %+v", view.Policy)
	}
}

func TestRuntimeArtifactSelectionView_RequestModelRequiredFileHintTrace(t *testing.T) {
	view := RuntimeArtifactSelectionViewFromAgentContext(&AgentContext{
		AnalysisIR: &AnalysisIR{RequestModel: RequestModel{
			ExternalObservationPolicy: traceOnlyExternalObservationPolicy(),
			AnalyzerHints: AnalyzerHints{RequiredFileHints: []RequiredFileHint{{
				Path:       "captures/frame.tracebundle.json",
				Confidence: 0.93,
			}}},
		}},
	})
	if view.Policy.Kind != RuntimeArtifactAnalysisPolicyTraceOnlyExactArtifact {
		t.Fatalf("typed required-file trace should form exact trace-only policy: %+v", view)
	}
	if !runtimeArtifactSelectionTestHasCarrier(view.Items[0], "required_file_hint") {
		t.Fatalf("required_file_hint carrier missing: %+v", view.Items[0])
	}
}

func traceOnlyExternalObservationPolicy() *ExternalObservationPolicy {
	return &ExternalObservationPolicy{
		ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
		CurrentSourceMode:    ExternalObservationCurrentSourceExclude,
		ExclusionKind:        ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:         []string{"typed current-source exclusion"},
		Confidence:           0.9,
	}
}

func runtimeArtifactSelectionTestHasCarrier(item RuntimeArtifactSelectionItem, want string) bool {
	for _, carrier := range item.Carriers {
		if carrier == want {
			return true
		}
	}
	return false
}
