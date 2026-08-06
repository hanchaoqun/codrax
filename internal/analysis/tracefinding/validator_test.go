package tracefinding_test

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSnapshotToken_BinderWait(t *testing.T) {
	snap, err := tracefinding.SnapshotToken("binder_wait")
	if err != nil {
		t.Fatalf("SnapshotToken: %v", err)
	}
	if snap.Token != "binder_wait" || snap.Lane == "" || snap.RegistryHash == "" {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
}

func TestValidateTraceFinding_RejectsInventedCandidate(t *testing.T) {
	snap, err := tracefinding.SnapshotToken("binder_wait")
	if err != nil {
		t.Fatal(err)
	}
	candidates := types.TraceDecisionCandidateSetV1{
		SchemaVersion:  types.TraceDecisionCandidateSetSchemaVersion,
		CandidateSetID: "set1",
		PrimaryEligible: []types.TraceCauseCandidate{{
			CandidateID:  "cand:binder_wait:1:ui",
			Token:        snap,
			EvidenceRefs: []string{"ev-1"},
		}},
		AcceptedEvidenceIDs: []string{"ev-1"},
	}
	finding := &types.TraceFindingV1{
		SchemaVersion: types.TraceFindingSchemaVersion,
		FindingID:     "f1",
		PrimaryCause: &types.TraceCauseDecision{
			CandidateID:  "cand:invented",
			Status:       types.TraceCausalSupportedCandidate,
			Token:        snap,
			EvidenceRefs: []string{"ev-1"},
		},
	}
	if err := tracefinding.ValidateTraceFinding(finding, candidates); err == nil {
		t.Fatal("expected invented candidate to fail")
	}
}

func TestValidateTraceFinding_AcceptsPrimaryFromEligible(t *testing.T) {
	snap, err := tracefinding.SnapshotToken("binder_wait")
	if err != nil {
		t.Fatal(err)
	}
	candidates := types.TraceDecisionCandidateSetV1{
		SchemaVersion:  types.TraceDecisionCandidateSetSchemaVersion,
		CandidateSetID: "set1",
		PrimaryEligible: []types.TraceCauseCandidate{{
			CandidateID:  "cand:binder_wait:1:ui",
			Token:        snap,
			EvidenceRefs: []string{"ev-1"},
		}},
		AcceptedEvidenceIDs: []string{"ev-1"},
	}
	finding := &types.TraceFindingV1{
		SchemaVersion: types.TraceFindingSchemaVersion,
		FindingID:     "f1",
		PrimaryCause: &types.TraceCauseDecision{
			CandidateID:  "cand:binder_wait:1:ui",
			Status:       types.TraceCausalSupportedCandidate,
			Token:        snap,
			EvidenceRefs: []string{"ev-1"},
		},
	}
	if err := tracefinding.ValidateTraceFinding(finding, candidates); err != nil {
		t.Fatalf("ValidateTraceFinding: %v", err)
	}
}

func TestCompileFromProjection_BuildsPrimaryEligible(t *testing.T) {
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		ArtifactLabel: "trace-a",
		WindowStartTs: 1,
		WindowEndTs:   2,
		RankedSeats: []types.TraceCausalProjectionNode{{
			Rank:                     1,
			Tier:                     "primary",
			TypeToken:                "binder_wait",
			Subject:                  "ui",
			EvidenceID:               "ev-1",
			EffectiveImpactMS:        12.5,
			EffectiveImpactPublished: true,
			RankBoardParamsFingerprint: "board1",
		}},
	}}}
	got, err := tracefinding.CompileTraceDecisionCandidateSetFromProjection(set)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(got.PrimaryEligible) == 0 {
		t.Fatalf("expected primary eligible, got %+v", got)
	}
	if got.PrimaryEligible[0].Token.Token != "binder_wait" {
		t.Fatalf("token = %q", got.PrimaryEligible[0].Token.Token)
	}
}
