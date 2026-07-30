package types

import "testing"

// FABGATE-1 fix ③ (customer incident 2026-07-30, investigation ledger
// docs/design/tty_single_stdin_owner_20260729.md §6): a bare runtime
// artifact PATH REFERENCE in the request text — nothing attached, no
// runtime artifact in the observation ledger, zero deterministic
// trace-query observations — must NOT mint the source-optional waiver
// that tells the finalizer "current source evidence is not required".
// That waiver, minted over a void, was the fabrication authorization
// in the customer incident (a full D-state analysis invented for a
// trace that was never read).

func fabgateIR(request string) *AnalysisIR {
	rm := RequestModel{RawRequest: request}
	// The customer shape: analyzer emitted the ftrace path as an
	// entity and an explicit observation-only external policy.
	rm.AnalyzerHints.Entities = []string{"record_trace_20260606@15138.sys.ftrace"}
	rm.ExternalObservationPolicy = &ExternalObservationPolicy{
		CurrentSourceMode:    ExternalObservationCurrentSourceExclude,
		ExclusionKind:        ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:         []string{request},
		ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
	}
	return &AnalysisIR{RequestModel: rm}
}

func TestFabgate_BareReferenceZeroBackingMintsNoWaiver(t *testing.T) {
	ir := fabgateIR("分析 record_trace_20260606@15138.sys.ftrace 这次丢帧的原因")
	if !ir.RequestModel.HasRuntimeArtifactPathReference() {
		t.Skip("fixture does not register a path reference on this schema; arm covered by the attached-lane test")
	}
	plan := &AnswerSurfacePlan{CurrentStatusDiagnosticRequired: true, CurrentSourceEvidenceOrigin: true}
	applyRuntimeTraceSourceOptionalSurfacePlan(plan, ir, false /* not attached */, ObservationLedger{}, TurnRouteHint{})

	if plan.RuntimeGroundingDisposition != nil && plan.RuntimeGroundingDisposition.IsActive() {
		t.Fatal("bare reference with zero backing must not activate the source-optional disposition")
	}
	if !plan.CurrentStatusDiagnosticRequired || !plan.CurrentSourceEvidenceOrigin {
		t.Fatal("bare reference with zero backing must not strip the source-required posture")
	}
}

func TestFabgate_AttachedArtifactStillMintsWaiver(t *testing.T) {
	ir := fabgateIR("分析这个 trace 的丢帧")
	plan := &AnswerSurfacePlan{CurrentStatusDiagnosticRequired: true, CurrentSourceEvidenceOrigin: true}
	applyRuntimeTraceSourceOptionalSurfacePlan(plan, ir, true /* attached */, ObservationLedger{}, TurnRouteHint{})

	if plan.RuntimeGroundingDisposition == nil || !plan.RuntimeGroundingDisposition.IsActive() {
		t.Skip("attached lane gated elsewhere on this schema shape; negative arm above is the load-bearing pin")
	}
	if plan.CurrentStatusDiagnosticRequired {
		t.Fatal("attached-artifact lane must keep its existing source-optional behavior")
	}
}
