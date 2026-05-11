package types

import "testing"

func TestBuildArtifactObservationProfile_PreservesNonExceptionLogObservations(t *testing.T) {
	profile := BuildArtifactObservationProfile(&LogBundle{
		Meta: LogMeta{Summary: "answer finalizer looped while line anchors drifted"},
		Observations: []LogObservation{
			{
				Kind:       LogObservationRetryCycle,
				Subject:    "finalizer",
				Summary:    "finalizer retried the same repair",
				Evidence:   "retry #2 selected finalizer_only",
				Diagnostic: true,
				Confidence: 0.91,
			},
			{
				Kind:       LogObservationLineMapping,
				Subject:    "line anchors",
				Summary:    "attached log line numbers differed from current source",
				Diagnostic: true,
				Confidence: 0.87,
			},
			{
				Kind:       LogObservationTopicMismatch,
				Subject:    "answer topic",
				Summary:    "draft answered the previous question",
				Diagnostic: true,
				Confidence: 0.89,
			},
		},
	}, nil)
	if profile == nil {
		t.Fatal("profile = nil")
	}
	if !profile.HasRetryLoop || !profile.HasLineMismatch || !profile.HasCompletionRewrite {
		t.Fatalf("non-exception observations not projected: %+v", profile)
	}
	if profile.DiagnosticConfidence < 0.91 {
		t.Fatalf("DiagnosticConfidence = %.2f, want >= 0.91", profile.DiagnosticConfidence)
	}
}

func TestBuildArtifactObservationProfile_PreservesErrorMessagesAsTypedEvidence(t *testing.T) {
	profile := BuildArtifactObservationProfile(&LogBundle{
		Errors: []LogError{{
			Type:    "panic",
			Message: "index out of bounds: index=5, size=3",
			Cause: &LogError{
				Type:    "Error",
				Message: "Cangjie native call failed: index out of bounds",
			},
		}},
	}, nil)
	if profile == nil {
		t.Fatal("profile nil")
	}
	var sawKind, sawRootMessage, sawCauseMessage bool
	for _, kind := range profile.ObservationKinds {
		if kind == ObservationKindErrorMessage {
			sawKind = true
		}
	}
	for _, snippet := range profile.EvidenceSnippets {
		if snippet == "index out of bounds: index=5, size=3" {
			sawRootMessage = true
		}
		if snippet == "Cangjie native call failed: index out of bounds" {
			sawCauseMessage = true
		}
	}
	if !sawKind || !sawRootMessage || !sawCauseMessage {
		t.Fatalf("error messages not surfaced as typed evidence: %+v", profile)
	}
}

func TestBuildArtifactObservationProfile_PreservesTraceSignals(t *testing.T) {
	profile := BuildArtifactObservationProfile(nil, &PerfBundle{
		Meta: PerfMeta{Source: "hitrace", Summary: "main thread stalls during scroll"},
		Janks: []PerfJank{{
			DurationMs:  48.5,
			TriggerSpan: "RenderList",
			Reason:      "heavy-compute",
		}},
		Stalls: []PerfStall{{
			DurationMs: 125.0,
			Kind:       "io",
			Symbol:     "LoadAvatar",
		}},
	})
	if profile == nil {
		t.Fatal("profile = nil")
	}
	if profile.Source != "trace" {
		t.Fatalf("Source = %q, want trace", profile.Source)
	}
	if profile.DiagnosticConfidence < 0.85 {
		t.Fatalf("DiagnosticConfidence = %.2f, want >= 0.85", profile.DiagnosticConfidence)
	}
	if !ObservationKindsContain(profile.ObservationKinds, ObservationKindPerfJank) ||
		!ObservationKindsContain(profile.ObservationKinds, ObservationKindPerfStall) {
		t.Fatalf("trace observations missing: %+v", profile.ObservationKinds)
	}
	if !containsProfileString(profile.SubjectCandidates, "RenderList") ||
		!containsProfileString(profile.SubjectCandidates, "LoadAvatar") {
		t.Fatalf("trace subjects missing: %+v", profile.SubjectCandidates)
	}
}

func TestBuildArtifactObservationProfileForRequest_NoAttachmentUsesTypedDiagnosticFlags(t *testing.T) {
	profile := BuildArtifactObservationProfileForRequest(RequestModel{
		RawRequest: "confirm whether the issue observed in the last run still exists in the current checkout",
		Predicates: SemanticPredicates{IsDiagnosticQuestion: false},
		DiagnosticProfile: DiagnosticIntentProfile{
			HistoricalRegression: true,
			CurrentVersionCheck:  true,
			ObservationSummary:   "previous final answer drifted off topic",
			Confidence:           0.93,
		},
		AnalyzerHints: AnalyzerHints{
			Entities:          []string{"Analyzer"},
			MentionedEntities: []string{"Finalizer"},
			ExactTargets:      []string{"emit_evidence"},
		},
	})
	if profile == nil {
		t.Fatal("profile = nil")
	}
	if profile.Source != "user_request" {
		t.Fatalf("Source = %q, want user_request", profile.Source)
	}
	if !ObservationKindsContain(profile.ObservationKinds, ObservationKindHistoricalRegressionCheck) ||
		!ObservationKindsContain(profile.ObservationKinds, ObservationKindCurrentVersionCheck) {
		t.Fatalf("typed diagnostic flags not projected: %+v", profile.ObservationKinds)
	}
	if profile.SymptomSummary != "previous final answer drifted off topic" {
		t.Fatalf("SymptomSummary = %q", profile.SymptomSummary)
	}
	if !containsProfileString(profile.SubjectCandidates, "Analyzer") ||
		!containsProfileString(profile.SubjectCandidates, "Finalizer") ||
		!containsProfileString(profile.SubjectCandidates, "emit_evidence") {
		t.Fatalf("subject candidates missing analyzer/profile surfaces: %+v", profile.SubjectCandidates)
	}
	if profile.DiagnosticConfidence != 0.93 {
		t.Fatalf("DiagnosticConfidence = %.2f, want 0.93", profile.DiagnosticConfidence)
	}
}

func containsProfileString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
