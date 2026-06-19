package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDeriveRuntimeUnitViewsProjectsActiveSliceTruthAndObservation(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	plan.ID = "plan-runtime-units"
	plan.Approval.RiskLevel = "low"
	plan.Changes = []types.FileChange{
		{Path: "pkg/foo.py", Kind: "modify", NewContent: "foo = 1\n"},
		{Path: "pkg/bar.py", Kind: "modify", NewContent: "bar = foo + 1\n", DependsOn: []string{"pkg/foo.py"}},
	}
	plan.TargetPaths = []string{"pkg/foo.py", "pkg/bar.py"}
	plan.Slices = []types.ChangePlanSlice{
		{ID: "slice-001", ChangeIndexes: []int{0}, ContractRefs: []string{"contract:foo"}},
		{ID: "slice-002", ChangeIndexes: []int{1}, ChangedSymbolRefs: []string{"Bar.render"}, VerificationProbes: []string{"probe:bar"}},
	}
	plan.ImpactObligations = &types.ImpactObligationSet{
		PlanID: plan.ID,
		Obligations: []types.ImpactObligation{{
			Kind:          "changed_symbol",
			Obligation:    "verify_changed_symbol",
			SubjectPath:   "pkg/bar.py",
			SubjectSymbol: "Bar.render",
			SliceID:       "slice-002",
			Source:        "change_plan_slice",
		}},
	}
	plan.OwnerAnchors = []types.OwnerAnchorViewItem{{
		ID:          "owner:bar",
		Path:        "pkg/bar.py",
		OwnerSymbol: "Bar",
		Strength:    types.SourceLocalizationAnchorOwner,
		Role:        types.SourcePathRoleProduction,
	}}
	plan.PatchEffect = &types.PatchEffectRecord{
		RecordID:        "patch-effect:plan-runtime-units:slice-002:abc",
		PlanID:          plan.ID,
		SliceID:         "slice-002",
		DiffFingerprint: "abc",
		HeadRef:         "abc123",
		Files: []types.PatchEffectFile{{
			Path:       "pkg/bar.py",
			Status:     "modified",
			AddedLines: 1,
		}},
	}
	plan.PatchReview = &types.PatchReviewRecord{
		ReviewID:        "patch-review:plan-runtime-units:slice-002",
		PlanID:          plan.ID,
		SliceID:         "slice-002",
		PatchEffectID:   plan.PatchEffect.RecordID,
		DiffFingerprint: "abc",
		Findings: []types.PatchReviewFinding{{
			Code:           "changed_symbol_unverified",
			Severity:       types.PatchReviewSeverityWarning,
			Category:       types.PatchReviewCategorySemanticCoverage,
			ImpactKind:     types.PatchReviewImpactKindChangedSymbol,
			CoverageStatus: types.PatchReviewCoverageUnverified,
			Path:           "pkg/bar.py",
		}},
	}
	plan.Approval.PlanFingerprint = types.PlanFingerprint(plan)

	run := workflowExecutionRunForTest(types.WriteWorkflowBatchVerifying, plan.ID)
	run.Batches[0].ActiveSliceID = "slice-002"
	run.Batches[0].Slices = types.WriteWorkflowSlicesFromChangePlan(plan)
	run.Batches[0].Slices[0].Status = types.ChangePlanSliceVerified
	run.Batches[0].Slices[1].Status = types.ChangePlanSliceObserving
	run.Batches[0].Slices[1].ApplyRef = "refs/codrax/applied/" + plan.ID
	run.Batches[0].Slices[1].Checkpoint = &types.WriteWorkflowCheckpoint{
		Ref:       "refs/codrax/applied/" + plan.ID,
		CommitSHA: "abc123",
		Source:    "slice_apply",
	}
	run.Batches[0].Slices[1].Attempts = []types.WriteWorkflowAttempt{{
		Kind:       "observe",
		Status:     "passed",
		ReasonCode: "tests_passed",
		PlanID:     plan.ID,
		ReportID:   "report-1",
	}}

	view := DeriveWorkflowExecutionView(types.ModeApply, run, plan)
	if len(view.RuntimeUnits) != 2 {
		t.Fatalf("runtime units = %d, want 2: %+v", len(view.RuntimeUnits), view.RuntimeUnits)
	}
	if view.ActiveRuntimeUnit == nil {
		t.Fatalf("active runtime unit missing: %+v", view)
	}
	unit := *view.ActiveRuntimeUnit
	if unit.UnitID != "slice-002" || !unit.Active || unit.Index != 2 || unit.Total != 2 {
		t.Fatalf("active runtime unit not projected: %+v", unit)
	}
	if unit.Permission.State != ApprovalExecutionAutoAllowed || unit.Permission.RiskLevel != "low" || !unit.Permission.CanAutoApply {
		t.Fatalf("permission not projected from approval authority: %+v", unit.Permission)
	}
	if !unit.PatchTruth.HasActualDiff || unit.PatchTruth.PatchEffectID != plan.PatchEffect.RecordID ||
		len(unit.PatchTruth.AppliedPaths) != 1 || unit.PatchTruth.AppliedPaths[0] != "pkg/bar.py" ||
		unit.PatchTruth.CheckpointSHA != "abc123" {
		t.Fatalf("patch truth not projected: %+v", unit.PatchTruth)
	}
	if unit.Observation.State != ObservationAuthorityVerified || unit.Observation.ReportID != "report-1" {
		t.Fatalf("observation not projected from slice attempt: %+v", unit.Observation)
	}
	if unit.PatchReview.Verdict != types.PatchReviewCoverageVerdictUnverified ||
		len(unit.PatchReview.UnverifiedKinds) != 1 ||
		unit.PatchReview.UnverifiedKinds[0] != types.PatchReviewImpactKindChangedSymbol {
		t.Fatalf("patch review coverage not projected: %+v", unit.PatchReview)
	}
	if len(unit.OwnerAnchors) != 1 || unit.OwnerAnchors[0].OwnerSymbol != "Bar" {
		t.Fatalf("owner anchors not projected: %+v", unit.OwnerAnchors)
	}
	if len(unit.ImpactObligations) != 1 || unit.ImpactObligations[0].SubjectSymbol != "Bar.render" {
		t.Fatalf("impact obligations not scoped to unit: %+v", unit.ImpactObligations)
	}
	if len(unit.ChangedSymbolRefs) != 1 || unit.ChangedSymbolRefs[0] != "Bar.render" ||
		len(unit.VerificationProbes) != 1 || unit.VerificationProbes[0] != "probe:bar" {
		t.Fatalf("slice typed refs not retained: %+v", unit)
	}
}

func TestDeriveRuntimeUnitViewsKeepsPatchTruthOnMatchingSliceOnly(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	plan.ID = "plan-runtime-matching-slice"
	plan.Changes = []types.FileChange{
		{Path: "a.py", Kind: "modify", NewContent: "a = 1\n"},
		{Path: "b.py", Kind: "modify", NewContent: "b = 2\n"},
	}
	plan.TargetPaths = []string{"a.py", "b.py"}
	plan.Slices = []types.ChangePlanSlice{
		{ID: "slice-001", ChangeIndexes: []int{0}},
		{ID: "slice-002", ChangeIndexes: []int{1}},
	}
	plan.PatchEffect = &types.PatchEffectRecord{
		RecordID:        "patch-effect:plan-runtime-matching-slice:slice-002:def",
		PlanID:          plan.ID,
		SliceID:         "slice-002",
		DiffFingerprint: "def",
		Files:           []types.PatchEffectFile{{Path: "b.py", Status: "modified"}},
	}
	plan.Approval.PlanFingerprint = types.PlanFingerprint(plan)
	run := workflowExecutionRunForTest(types.WriteWorkflowBatchVerifying, plan.ID)
	run.Batches[0].ActiveSliceID = "slice-001"
	run.Batches[0].Slices = types.WriteWorkflowSlicesFromChangePlan(plan)

	units := DeriveRuntimeUnitViews(types.ModeApply, run, plan, nil)
	if len(units) != 2 {
		t.Fatalf("runtime units = %d, want 2", len(units))
	}
	if units[0].PatchTruth.HasActualDiff || units[0].PatchTruth.PatchEffectID != "" {
		t.Fatalf("first slice must not inherit second slice patch truth: %+v", units[0].PatchTruth)
	}
	if !units[1].PatchTruth.HasActualDiff || units[1].PatchTruth.PatchEffectID != plan.PatchEffect.RecordID {
		t.Fatalf("matching slice patch truth missing: %+v", units[1].PatchTruth)
	}
}

func TestActiveRuntimeUnitApplyChangesUsesUnitIndexes(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	plan.ID = "plan-runtime-apply-changes"
	plan.Changes = []types.FileChange{
		{Path: "a.py", Kind: "modify"},
		{Path: "b.py", Kind: "modify"},
		{Path: "c.py", Kind: "modify"},
	}
	plan.TargetPaths = []string{"a.py", "b.py", "c.py"}
	plan.Approval.PlanFingerprint = types.PlanFingerprint(plan)
	run := types.WriteWorkflowRun{
		RunID:         "wf-runtime-apply",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchApplying,
			PlanID:        plan.ID,
			ActiveSliceID: "unit-b",
			Slices: []types.WriteWorkflowSlice{{
				ID:            "unit-a",
				Status:        types.ChangePlanSliceVerified,
				PlanID:        plan.ID,
				ChangeIndexes: []int{0},
			}, {
				ID:              "unit-b",
				Status:          types.ChangePlanSliceApplying,
				PlanID:          plan.ID,
				ChangeIndexes:   []int{1},
				DependsOnSlices: []string{"unit-a"},
			}},
		}},
	}

	changes, active := ActiveRuntimeUnitApplyChanges(plan, &run)
	if !active || len(changes) != 1 || changes[0].Path != "b.py" {
		t.Fatalf("changes=%+v active=%v, want b.py active", changes, active)
	}
	slice, active := ActiveRuntimeUnitApplySlice(plan, &run)
	if !active || slice.ID != "unit-b" || len(slice.DependsOnSlices) != 1 || slice.DependsOnSlices[0] != "unit-a" {
		t.Fatalf("slice=%+v active=%v, want unit-b with dependency", slice, active)
	}
}

func TestNextRunnableRuntimeUnitIDSkipsUnsatisfiedDependencies(t *testing.T) {
	units := []RuntimeUnitView{{
		UnitID: "unit-a",
		Active: true,
		Status: types.ChangePlanSliceVerified,
	}, {
		UnitID:         "unit-b",
		Status:         types.ChangePlanSlicePending,
		DependsOnUnits: []string{"unit-c"},
	}, {
		UnitID: "unit-c",
		Status: types.ChangePlanSlicePending,
	}}

	if got := NextRunnableRuntimeUnitID(units); got != "unit-c" {
		t.Fatalf("next runnable = %q, want unit-c", got)
	}
	units[2].Status = types.ChangePlanSliceVerified
	if got := NextRunnableRuntimeUnitID(units); got != "unit-b" {
		t.Fatalf("next runnable after dependency verified = %q, want unit-b", got)
	}
}

func TestPreserveVerifiedRuntimeUnitsCarriesOnlyIdenticalIndependentUnits(t *testing.T) {
	priorPlan := &types.ChangePlan{
		ID: "plan-prior",
		Changes: []types.FileChange{{
			Path:       "a.py",
			Kind:       "modify",
			NewContent: "a = 1\n",
			Rationale:  "old rationale",
			Apply:      &types.FileChangeApplyRecord{Status: "applied"},
		}, {
			Path:       "b.py",
			Kind:       "modify",
			NewContent: "b = 1\n",
		}},
		Slices: []types.ChangePlanSlice{
			{ID: "slice-a", ChangeIndexes: []int{0}},
			{ID: "slice-b", ChangeIndexes: []int{1}},
		},
	}
	nextPlan := &types.ChangePlan{
		ID: "plan-next",
		Changes: []types.FileChange{{
			Path:       "a.py",
			Kind:       "modify",
			NewContent: "a = 1\n",
			Rationale:  "new rationale does not change edit bytes",
		}, {
			Path:       "b.py",
			Kind:       "modify",
			NewContent: "b = 2\n",
		}},
		Slices: []types.ChangePlanSlice{
			{ID: "next-a", ChangeIndexes: []int{0}},
			{ID: "next-b", ChangeIndexes: []int{1}},
		},
	}
	existing := []types.WriteWorkflowSlice{{
		ID:            "slice-a",
		Status:        types.ChangePlanSliceVerified,
		PlanID:        priorPlan.ID,
		ChangeIndexes: []int{0},
		Paths:         []string{"a.py"},
		ApplyRef:      "refs/codrax/applied/plan-prior",
		Checkpoint:    &types.WriteWorkflowCheckpoint{Ref: "refs/codrax/applied/plan-prior", CommitSHA: "abc"},
		Completion:    &types.WriteWorkflowCompletion{Verdict: types.WriteWorkflowCompletionVerified, ReasonCode: "tests_passed"},
		Attempts:      []types.WriteWorkflowAttempt{{Kind: "observe", Status: "passed", ReasonCode: "tests_passed"}},
	}, {
		ID:            "slice-b",
		Status:        types.ChangePlanSliceFailed,
		PlanID:        priorPlan.ID,
		ChangeIndexes: []int{1},
		Paths:         []string{"b.py"},
	}}
	derived := types.WriteWorkflowSlicesFromChangePlan(nextPlan)

	got := PreserveVerifiedRuntimeUnits(existing, priorPlan, nextPlan, derived)
	if len(got) != 2 {
		t.Fatalf("preserved slices = %+v, want two", got)
	}
	if got[0].Status != types.ChangePlanSliceVerified || got[0].ApplyRef != "refs/codrax/applied/plan-prior" ||
		got[0].Checkpoint == nil || got[0].Checkpoint.CommitSHA != "abc" ||
		got[0].Completion == nil || got[0].Completion.Verdict != types.WriteWorkflowCompletionVerified {
		t.Fatalf("identical verified unit was not preserved: %+v", got[0])
	}
	if got[0].ID != "next-a" || got[0].PlanID != "plan-next" {
		t.Fatalf("preservation should keep next-plan unit identity while carrying prior evidence: %+v", got[0])
	}
	if got[1].Status != types.ChangePlanSlicePending || got[1].ApplyRef != "" || got[1].Checkpoint != nil {
		t.Fatalf("changed failed unit must not be preserved: %+v", got[1])
	}
}

func TestPreserveVerifiedRuntimeUnitsRejectsChangedSamePathUnit(t *testing.T) {
	priorPlan := &types.ChangePlan{
		ID:      "plan-prior",
		Changes: []types.FileChange{{Path: "a.py", Kind: "modify", NewContent: "a = 1\n"}},
		Slices:  []types.ChangePlanSlice{{ID: "slice-a", ChangeIndexes: []int{0}}},
	}
	nextPlan := &types.ChangePlan{
		ID:      "plan-next",
		Changes: []types.FileChange{{Path: "a.py", Kind: "modify", NewContent: "a = 2\n"}},
		Slices:  []types.ChangePlanSlice{{ID: "slice-a", ChangeIndexes: []int{0}}},
	}
	existing := []types.WriteWorkflowSlice{{
		ID:            "slice-a",
		Status:        types.ChangePlanSliceVerified,
		PlanID:        priorPlan.ID,
		ChangeIndexes: []int{0},
		Paths:         []string{"a.py"},
		ApplyRef:      "refs/codrax/applied/plan-prior",
	}}

	got := PreserveVerifiedRuntimeUnits(existing, priorPlan, nextPlan, types.WriteWorkflowSlicesFromChangePlan(nextPlan))
	if len(got) != 1 || got[0].Status != types.ChangePlanSlicePending || got[0].ApplyRef != "" {
		t.Fatalf("changed same-path unit must remain pending, got %+v", got)
	}
}
