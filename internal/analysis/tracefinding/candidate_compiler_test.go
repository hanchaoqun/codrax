package tracefinding

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCompileCandidateContractBindsTypedRankSeat(t *testing.T) {
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{{ID: "E1"}}}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		ArtifactPath: "traces/a.systrace", ArtifactLabel: "a.systrace",
		WindowStartTs: 1, WindowEndTs: 1.02,
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{Subject: "ui-7", TotalMS: 20},
		RankedSeats: []types.TraceCausalProjectionNode{{
			EvidenceID: "E1", Subject: "ui-7", TypeToken: "scheduler_latency",
			SpanName: "DrawFrame", BlockingKind: "monitor_contention", BlockingHolderSite: "ClassLinker lock",
			Rank: 1, Tier: "A", EffectiveImpactMS: 8.5, EffectiveImpactPublished: true,
			RankBoardParamsFingerprint: "board-1", Confidence: 0.9,
		}},
	}}}
	contract, err := CompileCandidateContract(ledger, set, "unproven")
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.Candidates) != 1 || len(contract.PrimaryCandidateIDs) != 1 {
		t.Fatalf("unexpected candidates: %+v", contract)
	}
	decision := contract.Candidates[0].Decision
	if decision.Token.Token != "scheduler_latency" || decision.SubjectRole != "target_thread" {
		t.Fatalf("unexpected decision snapshot: %+v", decision)
	}
	if decision.SubjectName != "ui-7" || decision.ResourceName != "DrawFrame" || decision.PhaseName != "DrawFrame" || decision.BlockingKind != "monitor_contention" {
		t.Fatalf("concrete trace names were not preserved: %+v", decision)
	}
	if decision.Magnitude == nil || decision.Magnitude.Value != 8.5 || decision.Magnitude.Caliber != "effective_attribution" {
		t.Fatalf("unexpected magnitude: %+v", decision.Magnitude)
	}
	if contract.FindingID == "" || contract.AnalysisKey == "" || contract.ContractHash == "" {
		t.Fatalf("missing stable contract identities: %+v", contract)
	}
}

func TestValidateRejectsCandidateFieldRewrite(t *testing.T) {
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{{ID: "E1"}}}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		RankedSeats: []types.TraceCausalProjectionNode{{EvidenceID: "E1", TypeToken: "scheduler_latency", Rank: 1, ImpactMS: 3}},
	}}}
	contract, err := CompileCandidateContract(ledger, set, "unproven")
	if err != nil {
		t.Fatal(err)
	}
	decision := contract.Candidates[0].Decision
	finding := &types.TraceFindingV1{
		SchemaVersion: types.TraceFindingSchemaVersion,
		FindingID:     contract.FindingID, AnalysisKey: contract.AnalysisKey,
		Artifact: contract.Artifact, Scope: contract.Scope,
		Revision: types.TraceFindingRevision{ContractHash: contract.ContractHash},
		Symptom:  contract.Symptom, PrimaryCause: &decision,
		EvidenceRefs: append([]string(nil), decision.EvidenceRefs...),
		Coverage:     types.TraceFindingCoverage{Complete: true},
	}
	if err := Validate(finding, contract); err != nil {
		t.Fatalf("valid candidate rejected: %v", err)
	}
	finding.PrimaryCause.SubjectRole = "invented_role"
	if err := Validate(finding, contract); err == nil {
		t.Fatal("candidate semantic rewrite was accepted")
	}
}

func TestCompileCandidateContractKeepsContextOutOfPrimaryRoster(t *testing.T) {
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{{ID: "E1"}, {ID: "E2"}, {ID: "E3"}}}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		RankedSeats: []types.TraceCausalProjectionNode{
			{EvidenceID: "E1", TypeToken: "scheduler_latency", Rank: 1, ImpactMS: 8, ChainRelevance: "on_chain"},
			{EvidenceID: "E2", TypeToken: "scheduler_latency", Rank: 2, ImpactMS: 6, ChainRelevance: "background"},
			{EvidenceID: "E3", TypeToken: "scheduler_latency", Rank: 3, ImpactMS: 4, Tier: types.TraceCausalTierContextOnly},
		},
	}}}
	contract, err := CompileCandidateContract(ledger, set, "unproven")
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.Candidates) != 2 {
		t.Fatalf("candidates=%d, want on-chain plus background contributor: %+v", len(contract.Candidates), contract.Candidates)
	}
	if len(contract.PrimaryCandidateIDs) != 1 || len(contract.ContributorCandidateIDs) != 2 {
		t.Fatalf("unexpected eligibility rosters: primary=%v contributor=%v", contract.PrimaryCandidateIDs, contract.ContributorCandidateIDs)
	}
}

func TestCandidateCausalityUnprovenNeverDefaultsToProven(t *testing.T) {
	if candidateCausalityExplicitlyProven("unproven") {
		t.Fatal("unproven must not be matched as proven")
	}
}

func TestBuildDeterministicFindingSelectsTopEligibleCandidate(t *testing.T) {
	contract := &types.TraceFindingContract{
		FindingSchemaVersion: types.TraceFindingSchemaVersion,
		FindingID:            "finding-1", AnalysisKey: "analysis-1", ContractHash: "hash-1",
		Candidates: []types.TraceFindingCandidateV1{
			{PrimaryEligible: false, Decision: types.TraceCauseDecision{CandidateID: "context", Rank: 1}},
			{PrimaryEligible: true, Decision: types.TraceCauseDecision{CandidateID: "root", Rank: 2, EvidenceRefs: []string{"E2"}}},
		},
	}
	finding := BuildDeterministicFinding(contract)
	if finding == nil || finding.PrimaryCause == nil || finding.PrimaryCause.CandidateID != "root" {
		t.Fatalf("unexpected deterministic finding: %+v", finding)
	}
	if finding.Revision.ContractHash != "hash-1" || len(finding.EvidenceRefs) != 1 || finding.EvidenceRefs[0] != "E2" {
		t.Fatalf("finding metadata drifted: %+v", finding)
	}
}

func TestBuildDeterministicFindingFailsClosedWithoutCandidates(t *testing.T) {
	finding := BuildDeterministicFinding(&types.TraceFindingContract{
		FindingSchemaVersion: types.TraceFindingSchemaVersion,
	})
	if finding == nil || finding.PrimaryCause != nil || finding.Unresolved == nil || finding.Coverage.Complete {
		t.Fatalf("empty candidate set must remain unresolved: %+v", finding)
	}
}
