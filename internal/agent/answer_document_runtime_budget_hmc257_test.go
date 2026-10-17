package agent

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC257RuntimeOnlyFactsKeepObservationBudget(t *testing.T) {
	for _, scope := range []types.RuntimeQuestionScope{
		types.RuntimeQuestionScopeBoundedFactSet, types.RuntimeQuestionScopeCausalDiagnosis,
		types.RuntimeQuestionScopeRelationAnalysis, types.RuntimeQuestionScopeSystemOverview,
	} {
		t.Run(string(scope), func(t *testing.T) {
			var records []types.ObservationRecord
			for _, record := range answerDocProjectionBudgetRecords(15) {
				if record.Origin == types.AnswerEvidenceOriginRuntimeArtifact {
					records = append(records, record)
				}
			}
			ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
				Intent:                 types.IntentExplain,
				RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: scope},
				DiagnosticProfile:      types.DiagnosticIntentProfile{IsDiagnostic: true},
			}}}
			ledger := types.ObservationLedger{Records: records}
			authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, ledger)
			if !authority.HasRuntimeSourceHandoffSurface() || authority.CurrentSourceRecordCount != 0 || authority.CurrentSourceExplanationRequested {
				t.Fatalf("runtime-only handoff fixture changed: %+v", authority)
			}
			got := answerDocObservationPromptProjectionOptions(ctx, ledger, 32)
			if want := types.DefaultObservationPromptProjectionOptions(32); !reflect.DeepEqual(got, want) {
				t.Errorf("runtime material domain must not shrink observation budget: got %+v want %+v", got, want)
			}
			projected := answerDocObservationPromptRecords(ctx, records, 32)
			if len(projected) != len(records) {
				t.Fatalf("runtime-only native roster truncated: got %d want %d", len(projected), len(records))
			}
			for _, record := range projected {
				if strings.HasPrefix(record.ID, "runtime:support:") && len(record.Notes) != 5 {
					t.Errorf("native supporting facts truncated for %s: %+v", record.ID, record.Notes)
				}
			}
		})
	}
}

func TestHMC257ExplicitCurrentVersionKeepsMixedObservationBudget(t *testing.T) {
	rm := types.RequestModel{
		Intent:                 types.IntentExplain,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeSystemOverview},
		DiagnosticProfile:      types.DiagnosticIntentProfile{CurrentVersionCheck: true},
	}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{answerDocRuntimeAuthorityRuntimeRecord()}}
	if got := answerDocObservationPromptProjectionOptions(ctx, ledger, 32); got.Limit != answerDocMixedRuntimeSourceObservationLedgerPromptLimit {
		t.Fatalf("explicit current-version verification remains a competing source lane: %+v", got)
	}
	ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
		CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
		ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary, SourceQuotes: []string{"do not analyze current source"},
	}
	if got := answerDocObservationPromptProjectionOptions(ctx, ledger, 32); got.Limit != 32 {
		t.Fatalf("explicit source exclusion must not create competing source budget: %+v", got)
	}
}
