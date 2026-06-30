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
