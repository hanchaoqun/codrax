package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSourceInventoryAnswerPreEmitAuthority_ProjectsCandidateUniverseGap(t *testing.T) {
	ctx := sourceInventoryUniverseTestContext([]string{"alpha", "beta", "gamma"})
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "packages",
		Value:   "2",
		Members: []string{"alpha", "beta"},
	}}

	auth := BuildSourceInventoryAnswerPreEmitAuthority(ctx, facts)
	if !auth.Active || !auth.Blocking {
		t.Fatalf("authority should expose the existing blocking gap, got %+v", auth)
	}
	if !auth.BestUniverseGap.Blocking ||
		auth.BestUniverseGap.Role != types.AnswerCandidateRolePackage ||
		len(auth.BestUniverseGap.Missing) != 1 ||
		auth.BestUniverseGap.Missing[0].Name != "gamma" {
		t.Fatalf("best universe gap did not preserve precise sensor result: %+v", auth.BestUniverseGap)
	}
	if !sourceInventoryAnswerPreEmitReasonCodeContains(auth, "candidate_universe_blocking") {
		t.Fatalf("reason codes should include candidate universe blocker, got %+v", auth.ReasonCodes)
	}
	if !auth.Snapshot.Active {
		t.Fatalf("authority should carry the shared snapshot for downstream consumers: %+v", auth.Snapshot)
	}
}

func TestSourceInventoryAnswerPreEmitAuthority_ProjectsExactAbsenceDebt(t *testing.T) {
	mut := types.NewMutableState("source inventory absence")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:   true,
		Complete: false,
		SourceClasses: []types.SourceInventorySourceClassCount{{
			Role:     types.SourcePathRoleProduction,
			Count:    2,
			Complete: false,
		}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				Confidence:        0.9,
			},
		}},
	}

	auth := BuildSourceInventoryAnswerPreEmitAuthority(ctx, nil)
	if !auth.Active || !auth.Blocking || !auth.ExactAbsenceBlocking {
		t.Fatalf("authority should expose exact absence proof debt, got %+v", auth)
	}
	if !strings.Contains(auth.ExactAbsenceSummary, "source_classes=") ||
		!strings.Contains(auth.ExactAbsenceSummary, "principal_roles=function") {
		t.Fatalf("absence summary should preserve typed proof surface, got %q", auth.ExactAbsenceSummary)
	}
	if !sourceInventoryAnswerPreEmitReasonCodeContains(auth, "exact_absence_requires_inventory_proof") {
		t.Fatalf("reason codes should include absence blocker, got %+v", auth.ReasonCodes)
	}
	summary, blocked := preCheckSourceInventoryExactAbsenceBound(ctx)
	if !blocked || summary != auth.ExactAbsenceSummary {
		t.Fatalf("pre-emit absence gate should consume the authority view: summary=%q blocked=%v auth=%q",
			summary, blocked, auth.ExactAbsenceSummary)
	}
}

func sourceInventoryAnswerPreEmitReasonCodeContains(auth SourceInventoryAnswerPreEmitAuthority, want string) bool {
	for _, got := range auth.ReasonCodes {
		if got == want {
			return true
		}
	}
	return false
}
