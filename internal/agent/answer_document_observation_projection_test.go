package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAnswerDocObservationPromptRecords_MixedRuntimeSourceUsesCompactBudget(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentRootCause,
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		LogTriage: &types.LogBundle{
			Observations: []types.LogObservation{{Kind: types.LogObservationRetryCycle, Summary: "finalizer timeout"}},
		},
		CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes: []types.CurrentSourceExplanationMode{
				types.CurrentSourceExplanationExplainCurrentMechanism,
			},
			SourceQuotes: []string{"结合当前源码"},
			Confidence:   0.9,
		},
	}}}

	got := answerDocObservationPromptRecords(ctx, answerDocProjectionBudgetRecords(14), answerDocObservationLedgerPromptLimit)
	if len(got) != answerDocMixedRuntimeSourceObservationLedgerPromptLimit {
		t.Fatalf("mixed runtime/source observation prompt records=%d, want compact limit %d", len(got), answerDocMixedRuntimeSourceObservationLedgerPromptLimit)
	}
	for _, record := range got {
		if record.Origin == types.AnswerEvidenceOriginRuntimeArtifact &&
			record.Role == types.AnswerAggregateRoleSupportingCoverage &&
			len(record.Notes) > 3 {
			t.Fatalf("mixed runtime/source support notes should be compact, got %d notes for %+v", len(record.Notes), record)
		}
	}
}

func TestAnswerDocFiniteRuntimeScopeProjectsRankingRowsOutOfFinalizerPromptOnly(t *testing.T) {
	records := []types.ObservationRecord{
		{ID: "trace_query:t#root_cause_rank:1", ClaimKey: "root_cause_primary", Subject: "target"},
		{ID: "trace_query:t#root_evidence:1", ClaimKey: "root_cause_evidence", Subject: "target"},
		{ID: "trace_query:t#target_window_states:1", ClaimKey: "target_window_states", Subject: "target"},
		{ID: "trace_query:t#frequency_limit:1", ClaimKey: "frequency_limit", Subject: "cpu=4"},
	}
	ctx := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
				Scope: types.RuntimeQuestionScopeBoundedEffectVerdict,
				FactFamilies: []types.RuntimeQuestionFactFamily{
					types.RuntimeQuestionFactTargetSchedulerState,
					types.RuntimeQuestionFactFrequencyResidency,
				},
			},
		}},
	}
	projected, _ := answerDocFinalizerObservationRecords(ctx, records)
	if len(projected) != 2 || projected[0].ID != records[2].ID || projected[1].ID != records[3].ID {
		t.Fatalf("finite prompt projection should retain direct facts only: %+v", projected)
	}
	if len(records) != 4 {
		t.Fatalf("prompt projection mutated the lossless source ledger: %+v", records)
	}

	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
	causal, _ := answerDocFinalizerObservationRecords(ctx, records)
	if len(causal) != len(records) {
		t.Fatalf("causal diagnosis lost ranking rows: %+v", causal)
	}
}

func TestExplorerRuntimeQuestionScopeWorkflowKeepsFiniteAndCausalLanesSeparate(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
			Scope: types.RuntimeQuestionScopeBoundedEffectVerdict,
			FactFamilies: []types.RuntimeQuestionFactFamily{
				types.RuntimeQuestionFactTargetSchedulerState,
				types.RuntimeQuestionFactFrequencyResidency,
			},
		},
	}}}
	finite := renderExplorerRuntimeQuestionScopeWorkflow(ctx)
	for _, want := range []string{
		"bounded_effect_verdict",
		"target_scheduler_state",
		"frequency_residency",
		"condition presence and condition-to-target binding as separate typed axes",
		"are not the default for this scope",
	} {
		if !strings.Contains(finite, want) {
			t.Fatalf("finite explorer scope guidance missing %q:\n%s", want, finite)
		}
	}
	if explorerRuntimeQuestionAllowsCausalRoster(ctx) {
		t.Fatal("finite scope unexpectedly authorizes causal roster workflow")
	}

	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
	if got := renderExplorerRuntimeQuestionScopeWorkflow(ctx); got != "" {
		t.Fatalf("causal scope should keep the established workflow without duplicate preface:\n%s", got)
	}
	if !explorerRuntimeQuestionAllowsCausalRoster(ctx) {
		t.Fatal("causal diagnosis lost broad drill workflow")
	}
}

func TestAnswerDocObservationPromptRecords_RouteBackedTypedRuntimeUsesAuthorityCompactBudget(t *testing.T) {
	ctx := &types.AgentContext{
		TurnRouteHint: types.TurnRouteHint{Source: "mixed", NeedsRepoAccess: true},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause,
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				Modes: []types.CurrentSourceExplanationMode{
					types.CurrentSourceExplanationExplainCurrentMechanism,
				},
				SourceQuotes: []string{"current parser mechanism"},
				Confidence:   0.9,
			},
		}},
	}

	got := answerDocObservationPromptRecords(ctx, answerDocProjectionBudgetRecords(14), answerDocObservationLedgerPromptLimit)
	if len(got) != answerDocMixedRuntimeSourceObservationLedgerPromptLimit {
		t.Fatalf("route-backed typed runtime/source observation prompt records=%d, want compact limit %d", len(got), answerDocMixedRuntimeSourceObservationLedgerPromptLimit)
	}
}

func TestAnswerDocObservationPromptRecords_DefaultBudgetUnchanged(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain,
	}}}

	got := answerDocObservationPromptRecords(ctx, answerDocProjectionBudgetRecords(14), answerDocObservationLedgerPromptLimit)
	if len(got) != 14 {
		t.Fatalf("ordinary observation prompt records=%d, want default full set", len(got))
	}
	runtimeSupportSeen := false
	for _, record := range got {
		if record.Origin == types.AnswerEvidenceOriginRuntimeArtifact &&
			record.Role == types.AnswerAggregateRoleSupportingCoverage {
			runtimeSupportSeen = true
			if len(record.Notes) != 5 {
				t.Fatalf("ordinary runtime support notes should keep default origin-specific budget, got %+v", record.Notes)
			}
		}
	}
	if !runtimeSupportSeen {
		t.Fatalf("test fixture did not include runtime support rows: %+v", got)
	}
}

func TestAnswerDocObservationPromptRecords_SameCaptureQuerySuppressesOnlyPreTriageNarrative(t *testing.T) {
	perf := &types.PerfBundle{Observations: []types.PerfObservation{
		{Authority: types.PerfObservationAuthorityPreTriageModelExtraction, Subject: "navigation"},
		{Authority: types.PerfObservationAuthorityDeterministicValidator, Subject: "validated"},
	}}
	ctx := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PerfTrace: perf,
		}},
	}
	const capture = "/captures/customer.systrace"
	records := []types.ObservationRecord{
		{
			ID: "perf:frame:0", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "perf_trace",
			SourceRef: types.ObservationSourceRef{CaptureIdentityPath: capture}, Subject: "measured frame",
		},
		{
			ID: "perf:observation:0", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "perf_trace",
			SourceRef: types.ObservationSourceRef{CaptureIdentityPath: capture}, Subject: "navigation",
		},
		{
			ID: "perf:observation:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "perf_trace",
			SourceRef: types.ObservationSourceRef{CaptureIdentityPath: capture}, Subject: "validated",
		},
		{
			ID: "trace_query:q#evidence_fact:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			SourceRef: types.ObservationSourceRef{CaptureIdentityPath: capture}, Subject: "exact query row",
		},
	}
	got, omitted := answerDocFinalizerObservationRecords(ctx, records)
	if omitted != 1 || len(got) != 3 {
		t.Fatalf("same-capture filter omitted=%d records=%+v", omitted, got)
	}
	joined := ""
	for _, record := range got {
		joined += record.ID + "\n"
	}
	for _, want := range []string{"perf:frame:0", "perf:observation:1", "trace_query:q#evidence_fact:1"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("required measured/deterministic row %q missing: %s", want, joined)
		}
	}
	if strings.Contains(joined, "perf:observation:0") {
		t.Fatalf("pre-triage narrative leaked after same-capture query: %s", joined)
	}
}

func TestAnswerDocObservationPromptRecords_DifferentCaptureFailsOpen(t *testing.T) {
	ctx := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{PerfTrace: &types.PerfBundle{
			Observations: []types.PerfObservation{{Authority: types.PerfObservationAuthorityPreTriageModelExtraction}},
		}}},
	}
	records := []types.ObservationRecord{
		{ID: "perf:observation:0", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "perf_trace", SourceRef: types.ObservationSourceRef{CaptureIdentityPath: "/captures/a.systrace"}},
		{ID: "trace_query:q#evidence_fact:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", SourceRef: types.ObservationSourceRef{CaptureIdentityPath: "/captures/b.systrace"}},
	}
	got, omitted := answerDocFinalizerObservationRecords(ctx, records)
	if omitted != 0 || len(got) != len(records) {
		t.Fatalf("different-capture observations must fail open, omitted=%d got=%+v", omitted, got)
	}
}

func TestRenderAnswerDocRuntimeSourceAuthority_SoftRequirementHandoff(t *testing.T) {
	ctx := &types.AgentContext{
		TurnRouteHint: types.TurnRouteHint{Source: "mixed", NeedsRepoAccess: true},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause,
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
			},
		}},
	}
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{answerDocRuntimeAuthorityRuntimeRecord()}}

	got := renderAnswerDocRuntimeSourceAuthority(ctx, ledger)
	for _, want := range []string{
		"##",
		"requirement_precision=`soft`",
		"hard_block=false",
		"caveat_only=true",
		"current_source_soft",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("soft runtime/source authority handoff missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "requirement_precision=`precise`") {
		t.Fatalf("soft authority handoff must not claim precise requirement:\n%s", got)
	}
}

func TestRenderAnswerDocRuntimeSourceAuthority_PreciseRequirementHandoff(t *testing.T) {
	ctx := &types.AgentContext{
		TurnRouteHint: types.TurnRouteHint{Source: "mixed", NeedsRepoAccess: true},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause,
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				Modes: []types.CurrentSourceExplanationMode{
					types.CurrentSourceExplanationExplainCurrentMechanism,
				},
				SourceQuotes: []string{"internal/tracequery/parse.go:42"},
				Confidence:   0.9,
			},
		}},
	}
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{answerDocRuntimeAuthorityRuntimeRecord()}}

	got := renderAnswerDocRuntimeSourceAuthority(ctx, ledger)
	for _, want := range []string{
		"requirement_precision=`precise`",
		"hard_block=true",
		"caveat_only=false",
		"current_source_precise",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("precise runtime/source authority handoff missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocRuntimeSourceAuthority_RuntimeOnlyDoesNotClaimCombinedProof(t *testing.T) {
	ctx := &types.AgentContext{
		TurnRouteHint: types.TurnRouteHint{
			Route:                     "repo",
			Source:                    "artifact",
			NeedsRepoAccess:           true,
			CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional,
		},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}},
	}
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{answerDocRuntimeAuthorityRuntimeRecord()}}

	got := renderAnswerDocRuntimeSourceAuthority(ctx, ledger)
	for _, want := range []string{
		"current_source_satisfied=false",
		"current_source_records=0",
		"runtime evidence is sufficient for the runtime artifact claim",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime-only authority handoff missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "runtime and current-source proof are both present") {
		t.Fatalf("runtime-only authority must not claim absent current-source proof:\n%s", got)
	}
}

func TestRenderAnswerDocRuntimeSourceAuthority_InactiveOmitted(t *testing.T) {
	if got := renderAnswerDocRuntimeSourceAuthority(&types.AgentContext{}, types.ObservationLedger{}); got != "" {
		t.Fatalf("inactive runtime/source authority should not render, got:\n%s", got)
	}
}

func answerDocProjectionBudgetRecords(n int) []types.ObservationRecord {
	records := make([]types.ObservationRecord, 0, n+2)
	records = append(records,
		types.ObservationRecord{
			ID:              "runtime:principal",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "log_triage",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_log", ArtifactKind: "log"},
			Span:            types.ObservationSpan{LineStart: 2, LineEnd: 3},
			Summary:         "attached log observed a finalizer timeout",
			RichNotes:       answerDocProjectionNotes("runtime-principal", 5),
		},
		types.ObservationRecord{
			ID:              "current:owner",
			Origin:          types.AnswerEvidenceOriginCurrentSource,
			Producer:        "emit_evidence",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceCurrentSource, Path: "internal/orchestrator/orchestrator.go"},
			Span:            types.ObservationSpan{LineStart: 6714},
			AnchorKind:      types.AnchorDefinition,
			EvidenceScope:   types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
			Summary:         "current source owns the retry branch",
			RichNotes:       answerDocProjectionNotes("current-owner", 5),
		},
	)
	for i := 0; i < n-2; i++ {
		records = append(records, types.ObservationRecord{
			ID:              fmt.Sprintf("runtime:support:%02d", i),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "log_triage",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingSoft,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_log", ArtifactKind: "log"},
			Span:            types.ObservationSpan{LineStart: i + 4},
			Summary:         fmt.Sprintf("supporting runtime observation %02d", i),
			RichNotes:       answerDocProjectionNotes(fmt.Sprintf("runtime-support-%02d", i), 5),
		})
	}
	return records
}

func answerDocRuntimeAuthorityRuntimeRecord() types.ObservationRecord {
	return types.ObservationRecord{
		ID:       "runtime:trace",
		Origin:   types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query",
		Role:     types.AnswerAggregateRolePrincipalAnswer,
		SourceRef: types.ObservationSourceRef{
			Kind:         types.ObservationSourceRuntimeArtifact,
			ArtifactID:   "attached_trace",
			ArtifactKind: "trace",
			PayloadRef:   "blob://trace",
		},
		Span:    types.ObservationSpan{StartTsMs: 62380029.1},
		Summary: "trace_query observed the selected runtime span",
	}
}

func answerDocProjectionNotes(prefix string, n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, strings.Repeat(prefix, 1)+fmt.Sprintf("-note-%02d", i))
	}
	return out
}
