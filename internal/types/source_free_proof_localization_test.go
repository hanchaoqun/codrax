package types

import (
	"reflect"
	"testing"
)

func TestSourceFreeProofLocalizationKeepsVerificationTargets(t *testing.T) {
	native, report := nativeRegistrationProofFixture(t)
	probe := proofPersistenceFixture()
	PreserveProofProbeOnlyPlanIdentity(probe)
	for _, plan := range []*ChangePlan{native, probe} {
		t.Run(plan.PersistenceKind, func(t *testing.T) {
			before := append([]string(nil), plan.TargetPaths...)
			review := SourceLocalizationReviewFromWritePlanContext("proof", "verify", nil, plan)
			if review.Status != SourceLocalizationUnknown || len(review.SourcePaths) != 0 || len(review.OwnerMissingPaths) != 0 {
				t.Fatalf("verification targets became new source-edit debt: %+v", review)
			}
			requirements := LocalizationRequirementsFromWritePlanContext("proof", "", WriteConsumerPlanner, nil, plan, 0)
			if len(requirements.Items) != 0 || !reflect.DeepEqual(before, plan.TargetPaths) || len(writeContextCoveragePlanPaths(plan)) == 0 {
				t.Fatalf("verification scope was changed: %+v %+v", plan, requirements)
			}
		})
	}
	// No localization debt is not proof. The exact execution receipt is still
	// required, and the retained source delivery's independent debt is kept.
	native.LocalizationReview = &SourceLocalizationReview{Status: SourceLocalizationUnknown}
	if !BehaviorContractRefHasVerificationWitness(native, report, "value") {
		t.Fatal("fixture did not supply actual contract witness")
	}
	source := &ChangePlan{ID: native.NativeTestRegistration.Delivery.SourcePlanID, TargetPaths: []string{"packages/widget/value.py"}, LocalizationReview: &SourceLocalizationReview{Status: SourceLocalizationWeak}}
	sourceReview := SourceLocalizationReviewFromWritePlanContext("source", "edit", nil, source)
	if sourceReview.Status != SourceLocalizationWeak || source.LocalizationReview.Status != SourceLocalizationWeak || len(sourceReview.OwnerMissingPaths) == 0 {
		t.Fatal("registration erased the source delivery's independent localization debt")
	}
	report.ExistingTestExecutions = nil
	if BehaviorContractRefHasVerificationWitness(native, report, "value") || BuildVerificationProofLedger(native, report, nil).State == VerificationProofLedgerVerified {
		t.Fatal("source-free localization bypassed missing execution proof")
	}
}

func TestSourceFreeProofLocalizationRejectsShapeShortcuts(t *testing.T) {
	for _, mutate := range []func(*ChangePlan){
		func(p *ChangePlan) { p.PersistenceKind = "" },
		func(p *ChangePlan) { p.Status = PlanStatusPending },
		func(p *ChangePlan) { p.VerificationProbes = nil },
		func(p *ChangePlan) { p.Changes = []FileChange{{Path: "src/widget.ts", Kind: "modify"}} },
	} {
		plan := proofPersistenceFixture()
		PreserveProofProbeOnlyPlanIdentity(plan)
		mutate(plan)
		review := SourceLocalizationReviewFromWritePlanContext("source", "edit", nil, plan)
		if review.Status != SourceLocalizationWeak || len(review.OwnerMissingPaths) == 0 {
			t.Fatalf("invalid proof shape bypassed source localization: %+v %+v", plan, review)
		}
	}
}
