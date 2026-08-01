package types

import (
	"strings"
	"testing"
)

func TestCompileAnswerClaimBindingsFromAggregateFacts_HistoryCountKeepsVCSAndMeasurement(t *testing.T) {
	rm := RequestModel{
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
			IsCountQuestion: true,
			IsScalarAnswer:  true,
		},
	}
	facts := []AnswerAggregateFact{{
		Kind:  AnswerAggregateScalar,
		Label: "system verified history count",
		Value: "3",
		Role:  AnswerAggregateRolePrincipalAnswer,
		Dimensions: []AnswerAggregateDimension{
			{Name: "origin", Value: "vcs_metadata"},
			{Name: "measurement_origin", Value: "command_measurement"},
		},
	}}

	got := CompileAnswerClaimBindingsFromAggregateFacts(facts, &rm, nil)
	assertClaimBinding(t, got, AnswerEvidenceOriginVCSMetadata, ClaimGroundingRepairable, AnswerRequestedOutputCount)
	assertClaimBinding(t, got, AnswerEvidenceOriginCommandMeasurement, ClaimGroundingHard, AnswerRequestedOutputCount)
}

func TestCompileAnswerClaimBindingsFromAggregateFacts_CurrentSourcePrincipalIsHard(t *testing.T) {
	rm := RequestModel{Intent: IntentExplain}
	facts := []AnswerAggregateFact{{
		Kind:        AnswerAggregateMemberSet,
		Label:       "current code symbols",
		Value:       "1",
		Role:        AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"Gate.Run"},
		SupportRefs: []string{"Gate.Run: internal/analysis/gate.go:42"},
	}}

	got := CompileAnswerClaimBindingsFromAggregateFacts(facts, &rm, nil)
	b := assertClaimBinding(t, got, AnswerEvidenceOriginCurrentSource, ClaimGroundingHard, AnswerRequestedOutputSummary)
	if b.TargetRef != "current code symbols" {
		t.Fatalf("target ref = %q", b.TargetRef)
	}
	if len(b.SupportRefs) != 1 {
		t.Fatalf("support refs were not preserved: %+v", b)
	}
	if !AnswerClaimBindingHasExactCurrentSourceSupport(b) {
		t.Fatalf("current-source hard binding should report exact source support: %+v", b)
	}
}

func TestCompileAnswerClaimBindingsFromAggregateFacts_HistoryRequestKeepsExactCurrentFactInSourceLane(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism},
			SourceQuotes:                        []string{"结合当前源码解释"},
			Confidence:                          0.9,
		},
	}
	facts := []AnswerAggregateFact{{
		Kind:        AnswerAggregateMemberSet,
		Label:       "current implementation paths",
		Value:       "1",
		Role:        AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"resolveCurrentImplementation"},
		SupportRefs: []string{"internal/agent/agent.go:5484"},
	}}

	got := CompileAnswerClaimBindingsFromAggregateFacts(facts, &rm, nil)
	assertClaimBinding(t, got, AnswerEvidenceOriginCurrentSource, ClaimGroundingHard, AnswerRequestedOutputMechanism)
	for _, binding := range got {
		if binding.Origin == AnswerEvidenceOriginVCSMetadata || binding.Origin == AnswerEvidenceOriginVCSDiff {
			t.Fatalf("request history shape minted a historical binding for an exact current-source fact: %+v", got)
		}
	}
}

// EVOLUTION RECORD (CSP-RM, §29.21 ruling 2026-07-10): previously this pin
// asserted a no-support-ref model fact compiled a REPAIRABLE current_source
// binding — the binding face of the terminal fallback that also fed
// CurrentSourceSatisfied pollution. Pure model claims now compile advisory
// system_inference bindings (soft grounding, illustrative ceiling, never
// citation-eligible). Facts that DECLARE exact SupportRefs keep the Hard
// current_source verification lane (see ..._CurrentSourcePrincipalIsHard).
func TestCompileAnswerClaimBindingsFromAggregateFacts_ModelClaimWithoutSupportIsAdvisory(t *testing.T) {
	rm := RequestModel{Intent: IntentExplain}
	facts := []AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "current code symbols without source refs",
		Value:   "1",
		Role:    AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Gate.Run"},
	}}

	got := CompileAnswerClaimBindingsFromAggregateFacts(facts, &rm, nil)
	b := assertClaimBinding(t, got, AnswerEvidenceOriginSystemInference, ClaimGroundingSoft, AnswerRequestedOutputSummary)
	if AnswerClaimBindingHasExactCurrentSourceSupport(b) {
		t.Fatalf("binding without support refs must not be current-source citation eligible: %+v", b)
	}
	if b.AuthorityCeiling != AuthorityIllustrative {
		t.Fatalf("model-claim binding must stay illustrative: %+v", b)
	}
	for _, binding := range got {
		if binding.Origin == AnswerEvidenceOriginCurrentSource {
			t.Fatalf("no-support model fact minted a current_source binding (CSP-RM regression): %+v", got)
		}
	}
}

// EVOLUTION RECORD (CSP-RM, §29.21 ruling 2026-07-10): a file:line spelled
// only inside a MEMBER (not declared as a SupportRef) is a model claim, not
// declared support — it used to reach a HARD current_source binding through
// the terminal fallback plus member parsing, which is exactly the
// model-assertion-outranks-witness shape the ruling closed. The member's
// coordinate still resolves through the display-side location machinery
// (aggregateMemberFactAllowsCurrentSourceLocations admits system_inference);
// declared SupportRefs keep the Hard lane byte-stable.
func TestCompileAnswerClaimBindingsFromAggregateFacts_SourceLocationMemberWithoutDeclaredSupportIsAdvisory(t *testing.T) {
	rm := RequestModel{Intent: IntentEnumerate}
	facts := []AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "source locations",
		Value:   "1",
		Role:    AnswerAggregateRolePrincipalAnswer,
		Members: []string{"internal/analysis/gate.go:42"},
	}}

	got := CompileAnswerClaimBindingsFromAggregateFacts(facts, &rm, nil)
	b := assertClaimBinding(t, got, AnswerEvidenceOriginSystemInference, ClaimGroundingSoft, AnswerRequestedOutputSummary)
	if AnswerClaimBindingHasExactCurrentSourceSupport(b) {
		t.Fatalf("member-spelled location must not count as declared exact support: %+v", b)
	}
}

func TestCompileAnswerClaimBindingsFromAggregateFacts_RuntimeArtifactDoesNotBecomeCurrentSource(t *testing.T) {
	rm := RequestModel{
		Intent: IntentRootCause,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		LogTriage: &LogBundle{Errors: []LogError{{Type: "panic"}}},
	}
	facts := []AnswerAggregateFact{{
		Kind:       AnswerAggregateScalar,
		Label:      "observed panic",
		Value:      "panic",
		Role:       AnswerAggregateRolePrincipalAnswer,
		Dimensions: []AnswerAggregateDimension{{Name: "origin", Value: "runtime_artifact"}},
	}}

	got := CompileAnswerClaimBindingsFromAggregateFacts(facts, &rm, nil)
	assertClaimBinding(t, got, AnswerEvidenceOriginRuntimeArtifact, ClaimGroundingRepairable, AnswerRequestedOutputDiagnostic)
	for _, binding := range got {
		if binding.Origin == AnswerEvidenceOriginCurrentSource {
			t.Fatalf("runtime artifact binding should not synthesize current_source: %+v", got)
		}
	}
}

func TestCompileAnswerClaimBindingsFromAggregateFacts_DiffOnlyDoesNotBecomeCurrentSource(t *testing.T) {
	rm := RequestModel{Intent: IntentExplain}
	facts := []AnswerAggregateFact{{
		Kind:  AnswerAggregateMemberSet,
		Label: "changed methods",
		Value: "1",
		Role:  AnswerAggregateRolePrincipalAnswer,
		Dimensions: []AnswerAggregateDimension{
			{Name: "origin", Value: string(AnswerEvidenceOriginVCSDiff)},
		},
		Members: []string{"oldMethod -> newMethod"},
	}}

	got := CompileAnswerClaimBindingsFromAggregateFacts(facts, &rm, nil)
	assertClaimBinding(t, got, AnswerEvidenceOriginVCSDiff, ClaimGroundingRepairable, AnswerRequestedOutputMechanism)
	for _, binding := range got {
		if binding.Origin == AnswerEvidenceOriginCurrentSource {
			t.Fatalf("diff-only aggregate must not synthesize current_source: %+v", got)
		}
	}
}

func TestCompileAnswerClaimBindingsFromAggregateFacts_NegativeObservationTargetsAbsentThing(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
	}
	facts, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:  AnswerAggregateNegativeObservation,
		Label: "history no-hit",
		Value: "0",
		Role:  AnswerAggregateRolePrincipalAnswer,
		Dimensions: []AnswerAggregateDimension{
			{Name: "origin", Value: "vcs_metadata"},
			{Name: "target", Value: "definitely_no_such_marker"},
			{Name: "commit_range", Value: "HEAD~20..HEAD"},
			{Name: "result_count", Value: "0"},
		},
	}})
	if err != nil {
		t.Fatalf("negative observation should normalize: %v", err)
	}

	got := CompileAnswerClaimBindingsFromAggregateFacts(facts, &rm, nil)
	b := assertClaimBinding(t, got, AnswerEvidenceOriginVCSMetadata, ClaimGroundingRepairable, AnswerRequestedOutputMechanism)
	if b.TargetRef != "definitely_no_such_marker @ HEAD~20..HEAD" {
		t.Fatalf("target ref = %q", b.TargetRef)
	}
}

func TestCompileRuntimeArtifactClaimBindings_LogBundleCreatesRuntimeBinding(t *testing.T) {
	rm := RequestModel{
		Intent: IntentRootCause,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		LogTriage: &LogBundle{
			Errors: []LogError{{
				Type: "runtime panic",
				Frames: []LogFrame{{
					Raw:  "panic at internal/server.go:42",
					File: "internal/server.go",
					Line: 42,
				}},
			}},
			Observations: []LogObservation{{
				Kind:      LogObservationRetryCycle,
				Subject:   "finalizer retry",
				Summary:   "finalizer retried after a schema repair",
				Evidence:  "答案待完善，正在重写",
				LineStart: 17,
			}},
		},
	}

	got := CompileAnswerClaimBindings(nil, &rm, nil)
	panicBinding := assertClaimBinding(t, got, AnswerEvidenceOriginRuntimeArtifact, ClaimGroundingRepairable, AnswerRequestedOutputDiagnostic)
	if panicBinding.Source != "log_triage" {
		t.Fatalf("runtime log binding source = %q", panicBinding.Source)
	}
	if got := AnswerClaimBindingAuthorityCeiling(panicBinding); got != AuthorityHistorical {
		t.Fatalf("runtime log binding authority = %q; want historical", got)
	}
	if panicBinding.AggregateIndex != -1 {
		t.Fatalf("runtime log binding should not point at aggregate_facts: %+v", panicBinding)
	}
	if !claimBindingHasSupport(panicBinding, "panic at internal/server.go:42") {
		t.Fatalf("runtime log frame support was not preserved: %+v", panicBinding.SupportRefs)
	}
	var sawObservation bool
	for _, binding := range got {
		if binding.TargetRef == "finalizer retry" &&
			claimBindingHasSupportContaining(binding, "log_line=17") &&
			claimBindingHasSupportContaining(binding, "finalizer retried after a schema repair") {
			sawObservation = true
		}
		if binding.Origin == AnswerEvidenceOriginCurrentSource {
			t.Fatalf("runtime log binding should not synthesize current_source: %+v", got)
		}
	}
	if !sawObservation {
		t.Fatalf("runtime log observation did not become a claim binding: %+v", got)
	}
}

func TestCompileRuntimeArtifactClaimBindings_PerfBundleCreatesRuntimeBinding(t *testing.T) {
	rm := RequestModel{
		Intent: IntentRootCause,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		PerfTrace: &PerfBundle{
			Frames: []PerfFrame{{FrameNo: 7, DurationMs: 24.5, Phase: "draw", Janky: true}},
			Janks:  []PerfJank{{StartTsMs: 100, DurationMs: 48, TriggerSpan: "RenderFrame", Reason: "heavy-compute"}},
			Stalls: []PerfStall{{StartTsMs: 120, DurationMs: 33, Kind: "io", Symbol: "ReadConfig", File: "src/app.cpp", Line: 88}},
			Observations: []PerfObservation{{
				Subject:    "GC span start",
				Summary:    "GC:Collect begins on trace line 5",
				LineStart:  5,
				DurationMs: 8,
			}},
		},
	}

	got := CompileAnswerClaimBindings(nil, &rm, nil)
	frameBinding := assertClaimBinding(t, got, AnswerEvidenceOriginRuntimeArtifact, ClaimGroundingRepairable, AnswerRequestedOutputDiagnostic)
	if frameBinding.Source != "perf_trace" {
		t.Fatalf("runtime perf binding source = %q", frameBinding.Source)
	}
	if got := AnswerClaimBindingAuthorityCeiling(frameBinding); got != AuthorityHistorical {
		t.Fatalf("runtime perf binding authority = %q; want historical", got)
	}
	if frameBinding.AggregateIndex != -1 {
		t.Fatalf("runtime perf binding should not point at aggregate_facts: %+v", frameBinding)
	}
	if !claimBindingHasSupport(frameBinding, "duration_ms=24.500 phase=draw janky=true") {
		t.Fatalf("runtime perf duration support was not preserved: %+v", frameBinding.SupportRefs)
	}
	var sawObservation bool
	for _, binding := range got {
		if binding.Origin == AnswerEvidenceOriginCurrentSource {
			t.Fatalf("runtime perf binding should not synthesize current_source: %+v", got)
		}
		if binding.TargetRef == "GC span start" && claimBindingHasSupportContaining(binding, "trace_line=5") {
			sawObservation = true
		}
	}
	if !sawObservation {
		t.Fatalf("runtime perf observation did not become a claim binding: %+v", got)
	}
}

func TestCompileRuntimeArtifactClaimBindings_PreTriageJankCauseIsCandidate(t *testing.T) {
	rm := RequestModel{
		Intent: IntentRootCause,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		PerfTrace: &PerfBundle{Janks: []PerfJank{{
			StartTsMs:       100,
			DurationMs:      86.111,
			TriggerSpan:     "H:RenderService:DoFrame",
			Reason:          "heavy-compute",
			CausalAuthority: PerfObservationAuthorityPreTriageModelExtraction,
		}}},
	}
	bindings := CompileAnswerClaimBindings(nil, &rm, nil)
	var found *AnswerClaimBinding
	for i := range bindings {
		if bindings[i].TargetRef == "H:RenderService:DoFrame" {
			found = &bindings[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("jank binding missing: %+v", bindings)
	}
	joined := strings.Join(found.SupportRefs, "\n")
	for _, want := range []string{"cause_candidate=heavy-compute", "causal_authority=pretriage_model_extraction"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("jank binding missing candidate authority %q: %+v", want, found.SupportRefs)
		}
	}
	if strings.Contains(joined, " reason=heavy-compute") {
		t.Fatalf("pretriage cause leaked as ordinary reason support: %+v", found.SupportRefs)
	}
}

func TestAnswerClaimBindingOriginSpecificHelpers(t *testing.T) {
	originOnly := []AnswerClaimBinding{
		{
			Origin:           AnswerEvidenceOriginVCSMetadata,
			RequestedOutputs: []AnswerRequestedOutput{AnswerRequestedOutputSummary},
		},
		{
			Origin:           AnswerEvidenceOriginMCPResource,
			RequestedOutputs: []AnswerRequestedOutput{AnswerRequestedOutputMechanism},
		},
	}
	if !AnswerClaimBindingsHaveOnlyOriginSpecificSupport(originOnly) {
		t.Fatalf("origin-specific bindings should be recognized: %+v", originOnly)
	}
	if AnswerClaimBindingsRequestExactOutput(originOnly) {
		t.Fatalf("summary/mechanism outputs should not be exact-value pressure: %+v", originOnly)
	}

	withCurrent := append([]AnswerClaimBinding(nil), originOnly...)
	withCurrent = append(withCurrent, AnswerClaimBinding{Origin: AnswerEvidenceOriginCurrentSource})
	if AnswerClaimBindingsHaveOnlyOriginSpecificSupport(withCurrent) {
		t.Fatalf("current_source binding should prevent origin-specific-only classification: %+v", withCurrent)
	}

	exactExternal := []AnswerClaimBinding{{
		Origin:           AnswerEvidenceOriginCommandMeasurement,
		RequestedOutputs: []AnswerRequestedOutput{AnswerRequestedOutputCount},
	}}
	if !AnswerClaimBindingsRequestExactOutput(exactExternal) {
		t.Fatalf("count output should remain exact-value pressure: %+v", exactExternal)
	}
	if AnswerClaimBindingsHaveOnlyOriginSpecificSupport([]AnswerClaimBinding{{
		Origin: AnswerEvidenceOriginSystemInference,
	}}) {
		t.Fatal("system_inference should not be treated as origin-specific evidence support")
	}
}

func assertClaimBinding(t *testing.T, bindings []AnswerClaimBinding, origin AnswerEvidenceOrigin, policy ClaimGroundingPolicy, output AnswerRequestedOutput) AnswerClaimBinding {
	t.Helper()
	for _, binding := range bindings {
		if binding.Origin != origin {
			continue
		}
		if binding.GroundingPolicy != policy {
			t.Fatalf("binding policy for %s = %s, want %s: %+v", origin, binding.GroundingPolicy, policy, binding)
		}
		if !claimBindingHasOutput(binding, output) {
			t.Fatalf("binding for %s missing output %s: %+v", origin, output, binding.RequestedOutputs)
		}
		return binding
	}
	t.Fatalf("missing binding for origin %s in %+v", origin, bindings)
	return AnswerClaimBinding{}
}

func claimBindingHasOutput(binding AnswerClaimBinding, want AnswerRequestedOutput) bool {
	for _, got := range binding.RequestedOutputs {
		if got == want {
			return true
		}
	}
	return false
}

func claimBindingHasSupport(binding AnswerClaimBinding, want string) bool {
	for _, got := range binding.SupportRefs {
		if got == want {
			return true
		}
	}
	return false
}

func claimBindingHasSupportContaining(binding AnswerClaimBinding, want string) bool {
	for _, got := range binding.SupportRefs {
		if strings.Contains(got, want) {
			return true
		}
	}
	return false
}
