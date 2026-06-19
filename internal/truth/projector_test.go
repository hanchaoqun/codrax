package truth

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func TestProjectRuntimeFailureOverridesCoveredProof(t *testing.T) {
	ledger := Project(ProjectInput{View: writeflow.WorkflowExecutionView{
		Observation: writeflow.ObservationAuthorityView{
			State:      writeflow.ObservationAuthorityFailed,
			ReasonCode: "tests_failed",
			ReportID:   "report-1",
		},
		Proof: loopkernel.ProofCoverageAuthorityView{
			State:      loopkernel.ProofCoverageCovered,
			ReasonCode: "proof_covered",
		},
	}})
	if ledger.State != types.TruthLedgerFailed || ledger.RecommendedAction != types.TruthActionRepair {
		t.Fatalf("ledger = %+v, want failed repair", ledger)
	}
	if ledger.ReasonCode != "tests_failed" {
		t.Fatalf("reason = %q", ledger.ReasonCode)
	}
}

func TestProjectUnavailableVerificationIsUnverified(t *testing.T) {
	ledger := Project(ProjectInput{View: writeflow.WorkflowExecutionView{
		Observation: writeflow.ObservationAuthorityView{
			State:      writeflow.ObservationAuthorityUnverified,
			ReasonCode: "runner_missing",
		},
		Proof: loopkernel.ProofCoverageAuthorityView{
			State:      loopkernel.ProofCoverageUnavailable,
			ReasonCode: "proof_unavailable",
		},
	}})
	if ledger.State != types.TruthLedgerUnverified || ledger.RecommendedAction != types.TruthActionContinue {
		t.Fatalf("ledger = %+v, want unverified continue", ledger)
	}
	if ledger.Failed {
		t.Fatalf("unavailable verification must not be failed: %+v", ledger)
	}
}

func TestProjectLocalizationGapPrefersLocalize(t *testing.T) {
	ledger := Project(ProjectInput{View: writeflow.WorkflowExecutionView{
		Localization: loopkernel.LocalizationAuthorityView{
			State:               loopkernel.LocalizationAuthorityObservedOnly,
			ReasonCode:          "localization_observed_without_owner",
			SourcePaths:         []string{"pkg/a.py"},
			OwnerMissingPaths:   []string{"pkg/a.py"},
			RequiresMoreContext: true,
		},
		Proof: loopkernel.ProofCoverageAuthorityView{
			State:      loopkernel.ProofCoverageWeak,
			ReasonCode: "proof_weak",
		},
	}})
	if ledger.State != types.TruthLedgerWeak ||
		ledger.RecommendedAction != types.TruthActionLocalize ||
		ledger.ReasonCode != "localization_observed_without_owner" {
		t.Fatalf("ledger = %+v, want weak localize", ledger)
	}
}

func TestProjectPatchReviewHardBlockFails(t *testing.T) {
	plan := &types.ChangePlan{PatchReview: &types.PatchReviewRecord{
		HardBlock: true,
		Findings: []types.PatchReviewFinding{{
			Code:     "outside_scope",
			Severity: types.PatchReviewSeverityError,
			Path:     "pkg/a.py",
		}},
	}}
	ledger := Project(ProjectInput{Plan: plan})
	if ledger.State != types.TruthLedgerFailed || ledger.RecommendedAction != types.TruthActionRepair {
		t.Fatalf("ledger = %+v, want failed repair", ledger)
	}
}

func TestProjectImpactUnverifiedCreatesWeakTruth(t *testing.T) {
	plan := &types.ChangePlan{ImpactAnalysis: &types.ImpactAnalysisResult{
		VerificationTargets: []types.ImpactVerificationTarget{{
			Kind:           "dependent",
			Path:           "pkg/a.py",
			RelatedPath:    "pkg/b.py",
			CoverageStatus: "unverified",
			EvidenceRef:    "impact-1",
		}},
	}}
	ledger := Project(ProjectInput{Plan: plan})
	if ledger.State != types.TruthLedgerWeak || ledger.RecommendedAction != types.TruthActionAddProof {
		t.Fatalf("ledger = %+v, want weak add_proof", ledger)
	}
	if !truthSignalHasEvidence(ledger, "impact-1") {
		t.Fatalf("impact evidence not preserved: %+v", ledger.Signals)
	}
}

func TestProjectCoveredSignalsBecomeCoveredTruth(t *testing.T) {
	ledger := Project(ProjectInput{View: writeflow.WorkflowExecutionView{
		Observation: writeflow.ObservationAuthorityView{
			State:      writeflow.ObservationAuthorityVerified,
			ReasonCode: "tests_passed",
		},
		Proof: loopkernel.ProofCoverageAuthorityView{
			State:      loopkernel.ProofCoverageCovered,
			ReasonCode: "proof_covered",
		},
		Localization: loopkernel.LocalizationAuthorityView{
			State:      loopkernel.LocalizationAuthorityOwnerSupported,
			ReasonCode: "localization_owner_supported",
		},
	}})
	if ledger.State != types.TruthLedgerCovered ||
		ledger.RecommendedAction != types.TruthActionContinue ||
		ledger.CompletionVerdict != types.WriteWorkflowCompletionVerified {
		t.Fatalf("ledger = %+v, want covered continue verified", ledger)
	}
}

func truthSignalHasEvidence(ledger types.TruthLedger, ref string) bool {
	for _, signal := range ledger.Signals {
		for _, got := range signal.EvidenceRefs {
			if got == ref {
				return true
			}
		}
	}
	return false
}
