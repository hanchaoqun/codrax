package types

import (
	"fmt"
	"strings"
)

// ArtifactObservationProfile is the normalized, typed observation lane
// derived from log / trace triage. It deliberately stores compact facts
// rather than raw artifact text so hard routing can consume precise
// booleans while prompts can still surface the evidence snippets.
type ArtifactObservationProfile struct {
	Source               string   `json:"source,omitempty"`
	ObservationKinds     []string `json:"observation_kind,omitempty"`
	SymptomSummary       string   `json:"symptom_summary,omitempty"`
	EvidenceSnippets     []string `json:"evidence_snippets,omitempty"`
	SubjectCandidates    []string `json:"subject_candidates,omitempty"`
	HasRetryLoop         bool     `json:"has_retry_loop,omitempty"`
	HasLineMismatch      bool     `json:"has_line_mismatch,omitempty"`
	HasCompletionRewrite bool     `json:"has_completion_rewrite,omitempty"`
	DiagnosticConfidence float64  `json:"diagnostic_confidence,omitempty"`
}

// BuildArtifactObservationProfile merges the structured log and trace
// bundles into one profile. It never scans raw log/trace text; every
// field comes from triage enums, summary fields, or typed frame/span
// structs.
func BuildArtifactObservationProfile(logBundle *LogBundle, perfBundle *PerfBundle) *ArtifactObservationProfile {
	var out ArtifactObservationProfile
	var sources []string
	if logBundle != nil {
		sources = append(sources, "log")
		mergeLogObservationProfile(&out, logBundle)
	}
	if perfBundle != nil {
		sources = append(sources, "trace")
		mergePerfObservationProfile(&out, perfBundle)
	}
	out.Source = strings.Join(dedupeStrings(sources), "+")
	out.ObservationKinds = dedupeStrings(out.ObservationKinds)
	out.EvidenceSnippets = dedupeStrings(out.EvidenceSnippets)
	out.SubjectCandidates = dedupeStrings(out.SubjectCandidates)
	if out.Source == "" &&
		len(out.ObservationKinds) == 0 &&
		out.SymptomSummary == "" &&
		len(out.EvidenceSnippets) == 0 &&
		len(out.SubjectCandidates) == 0 {
		return nil
	}
	return &out
}

// BuildArtifactObservationProfileForRequest extends the artifact/trace
// profile with the analyzer's typed diagnostic-intent lane. This lets
// no-attachment follow-up questions use the same downstream
// current-status scaffold as log/trace questions without scanning raw
// prose for diagnostic keywords.
func BuildArtifactObservationProfileForRequest(rm RequestModel) *ArtifactObservationProfile {
	profile := BuildArtifactObservationProfile(rm.LogTriage, rm.PerfTrace)
	if !rm.Predicates.IsDiagnosticQuestion && !rm.DiagnosticProfile.RequiresDiagnosticRootCause() {
		return profile
	}
	if profile == nil {
		profile = &ArtifactObservationProfile{}
	}
	profile.Source = joinProfileSource(profile.Source, "user_request")
	profile.ObservationKinds = append(profile.ObservationKinds, "request_diagnostic")
	if rm.DiagnosticProfile.IsDiagnostic {
		profile.ObservationKinds = append(profile.ObservationKinds, "diagnostic_question")
	}
	if rm.DiagnosticProfile.CurrentRisk {
		profile.ObservationKinds = append(profile.ObservationKinds, "current_risk_check")
	}
	if rm.DiagnosticProfile.HistoricalRegression {
		profile.ObservationKinds = append(profile.ObservationKinds, "historical_regression_check")
	}
	if rm.DiagnosticProfile.CurrentVersionCheck {
		profile.ObservationKinds = append(profile.ObservationKinds, "current_version_check")
	}
	if summary := strings.TrimSpace(rm.DiagnosticProfile.ObservationSummary); summary != "" {
		profile.SymptomSummary = clampProfileSnippet(summary)
		profile.EvidenceSnippets = append(profile.EvidenceSnippets, clampProfileSnippet("diagnostic observation: "+summary))
	}
	if profile.SymptomSummary == "" {
		profile.SymptomSummary = clampProfileSnippet(rm.RawRequest)
	}
	if raw := strings.TrimSpace(rm.RawRequest); raw != "" {
		profile.EvidenceSnippets = append(profile.EvidenceSnippets, clampProfileSnippet("user request: "+raw))
	}
	profile.SubjectCandidates = append(profile.SubjectCandidates,
		rm.AnalyzerHints.Entities...)
	profile.SubjectCandidates = append(profile.SubjectCandidates,
		rm.AnalyzerHints.MentionedEntities...)
	profile.SubjectCandidates = append(profile.SubjectCandidates,
		rm.AnalyzerHints.PrimaryEntities...)
	profile.SubjectCandidates = append(profile.SubjectCandidates,
		rm.AnalyzerHints.DerivedEntities...)
	profile.SubjectCandidates = append(profile.SubjectCandidates,
		rm.AnalyzerHints.ExactTargets...)
	profile.SubjectCandidates = append(profile.SubjectCandidates,
		rm.AnswerSubject.EntityAxes...)
	for _, topic := range rm.SubTopics {
		profile.SubjectCandidates = append(profile.SubjectCandidates, topic.Entities...)
		if profile.SymptomSummary == "" {
			profile.SymptomSummary = clampProfileSnippet(topic.Summary)
		}
	}
	if rm.DiagnosticProfile.Confidence > profile.DiagnosticConfidence {
		profile.DiagnosticConfidence = rm.DiagnosticProfile.Confidence
	} else if profile.DiagnosticConfidence == 0 && rm.Predicates.IsDiagnosticQuestion {
		profile.DiagnosticConfidence = 0.7
	}
	profile.ObservationKinds = dedupeStrings(profile.ObservationKinds)
	profile.EvidenceSnippets = dedupeStrings(profile.EvidenceSnippets)
	profile.SubjectCandidates = dedupeStrings(profile.SubjectCandidates)
	return profile
}

func mergeLogObservationProfile(out *ArtifactObservationProfile, bundle *LogBundle) {
	if out == nil || bundle == nil {
		return
	}
	if summary := strings.TrimSpace(bundle.Meta.Summary); summary != "" && out.SymptomSummary == "" {
		out.SymptomSummary = clampProfileSnippet(summary)
	}
	for _, sig := range bundle.Meta.Signals {
		name := strings.TrimSpace(string(sig))
		if name == "" {
			continue
		}
		out.ObservationKinds = append(out.ObservationKinds, "signal:"+name)
		out.EvidenceSnippets = append(out.EvidenceSnippets, name)
		if out.DiagnosticConfidence < 0.75 {
			out.DiagnosticConfidence = 0.75
		}
	}
	for _, typ := range LogBundleErrorTypes(bundle) {
		name := strings.TrimSpace(typ)
		if name == "" {
			continue
		}
		out.ObservationKinds = append(out.ObservationKinds, "error_type")
		out.EvidenceSnippets = append(out.EvidenceSnippets, name)
		if out.DiagnosticConfidence < 0.85 {
			out.DiagnosticConfidence = 0.85
		}
	}
	for _, obs := range bundle.Observations {
		kind := strings.TrimSpace(string(obs.Kind))
		if kind != "" {
			out.ObservationKinds = append(out.ObservationKinds, kind)
		}
		if out.SymptomSummary == "" {
			out.SymptomSummary = clampProfileSnippet(obs.Summary)
		}
		if subject := strings.TrimSpace(obs.Subject); subject != "" {
			out.SubjectCandidates = append(out.SubjectCandidates, subject)
		}
		if evidence := strings.TrimSpace(obs.Evidence); evidence != "" {
			out.EvidenceSnippets = append(out.EvidenceSnippets, clampProfileSnippet(evidence))
		} else if summary := strings.TrimSpace(obs.Summary); summary != "" {
			out.EvidenceSnippets = append(out.EvidenceSnippets, clampProfileSnippet(summary))
		}
		if obs.Diagnostic && obs.Confidence > out.DiagnosticConfidence {
			out.DiagnosticConfidence = obs.Confidence
		}
		switch obs.Kind {
		case LogObservationRetryCycle:
			out.HasRetryLoop = true
		case LogObservationLineMapping:
			out.HasLineMismatch = true
		case LogObservationTopicMismatch, LogObservationContractViolation:
			out.HasCompletionRewrite = true
		}
	}
	for _, e := range bundle.Entities {
		if e = strings.TrimSpace(e); e != "" {
			out.SubjectCandidates = append(out.SubjectCandidates, e)
		}
	}
}

func mergePerfObservationProfile(out *ArtifactObservationProfile, bundle *PerfBundle) {
	if out == nil || bundle == nil {
		return
	}
	if summary := strings.TrimSpace(bundle.Meta.Summary); summary != "" && out.SymptomSummary == "" {
		out.SymptomSummary = clampProfileSnippet(summary)
	}
	for _, sig := range bundle.Meta.Signals {
		name := strings.TrimSpace(sig)
		if name == "" {
			continue
		}
		out.ObservationKinds = append(out.ObservationKinds, "perf_signal:"+name)
		out.EvidenceSnippets = append(out.EvidenceSnippets, name)
	}
	for _, f := range bundle.Frames {
		out.ObservationKinds = append(out.ObservationKinds, "perf_frame")
		out.EvidenceSnippets = append(out.EvidenceSnippets,
			clampProfileSnippet(fmt.Sprintf("frame %d duration %.2fms", f.FrameNo, f.DurationMs)))
	}
	for _, j := range bundle.Janks {
		out.ObservationKinds = append(out.ObservationKinds, "perf_jank")
		if span := strings.TrimSpace(j.TriggerSpan); span != "" {
			out.SubjectCandidates = append(out.SubjectCandidates, span)
		}
		out.EvidenceSnippets = append(out.EvidenceSnippets,
			clampProfileSnippet(fmt.Sprintf("jank %.2fms %s", j.DurationMs, strings.TrimSpace(j.Reason))))
	}
	for _, s := range bundle.Stalls {
		out.ObservationKinds = append(out.ObservationKinds, "perf_stall")
		if sym := strings.TrimSpace(s.Symbol); sym != "" {
			out.SubjectCandidates = append(out.SubjectCandidates, sym)
		}
		out.EvidenceSnippets = append(out.EvidenceSnippets,
			clampProfileSnippet(fmt.Sprintf("stall %.2fms %s", s.DurationMs, strings.TrimSpace(s.Kind))))
	}
	if bundle.Startup != nil {
		out.ObservationKinds = append(out.ObservationKinds, "perf_startup")
		out.EvidenceSnippets = append(out.EvidenceSnippets,
			clampProfileSnippet(fmt.Sprintf("%s startup %.2fms", bundle.Startup.Mode, bundle.Startup.AppLaunchMs)))
	}
	for _, e := range bundle.Entities {
		if e = strings.TrimSpace(e); e != "" {
			out.SubjectCandidates = append(out.SubjectCandidates, e)
		}
	}
	if bundle.HasStructuredObservations() && out.DiagnosticConfidence < 0.85 {
		out.DiagnosticConfidence = 0.85
	}
}

func clampProfileSnippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 240 {
		return s
	}
	return s[:240]
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

func joinProfileSource(existing, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return strings.TrimSpace(existing)
	}
	parts := strings.Split(strings.TrimSpace(existing), "+")
	parts = append(parts, next)
	return strings.Join(dedupeStrings(parts), "+")
}
