package types

import (
	"fmt"
	"strings"
	"testing"
)

func TestWriteContextPackFromWriteAnalysisIRPreservesP0Constraints(t *testing.T) {
	ir := &WriteAnalysisIR{
		Request: WriteRequestModel{
			Task: WriteTask{Summary: "add approval gate"},
			Risk: WriteRiskProfile{
				AffectsPublicAPI:   true,
				ChangesBuildSystem: true,
				Overall:            RiskBandHigh,
			},
			ScopeAnchors: []string{"internal/orchestrator/stage_hooks.go"},
			Constraints: []WriteConstraint{{
				Kind:   "preserve_read_mode",
				Target: "scheduler",
				Note:   "do not change read scheduler byte identity",
			}},
			ExpectedOutcomes: []string{"plan-file apply passes approval gate"},
			BehaviorContracts: []WriteBehaviorContract{{
				ID:       "approval-gate",
				Kind:     WriteBehaviorInvariant,
				Polarity: WriteBehaviorPolarityExpected,
				Operator: WriteBehaviorOpSatisfies,
				Expected: "all writes pass apply-pre approval gate",
				Comparator: &WriteBehaviorComparator{
					Subject:     "saved plan apply",
					Operator:    WriteBehaviorOpSatisfies,
					Expected:    "same apply-pre approval gate",
					Relation:    WriteBehaviorComparatorRegressionBaseline,
					EvidenceRef: "design:approval",
				},
				Required: true,
				Source:   "write_analyzer",
			}, {
				ID:       "repr-unit-placement",
				Kind:     WriteBehaviorStdout,
				Polarity: WriteBehaviorPolarityExpected,
				Operator: WriteBehaviorOpContains,
				Subject:  "DataArray repr line",
				Expected: ", in mm",
				Placement: &WriteRenderedTextPlacement{
					Surface:   WriteRenderedTextSurfaceRepr,
					Anchor:    "rainfall",
					Expected:  ", in mm",
					Relation:  WriteRenderedTextBetweenAnchorAndDelimiter,
					Delimiter: "float32",
				},
				Required: true,
				Source:   "write_analyzer",
			}},
		},
		RepoFacts: WriteRepoFacts{TestRunner: "go"},
	}
	pack := WriteContextPackFromWriteAnalysisIR(ir)
	view := pack.View(WriteConsumerPlanner, 10)
	if len(view.Items) == 0 {
		t.Fatal("expected planner items")
	}
	if view.Items[0].Priority != WriteContextP0 {
		t.Fatalf("first item priority = %s, want p0; items=%+v", view.Items[0].Priority, view.Items)
	}
	if !writeContextViewContains(view, "constraint", "do not change read scheduler") {
		t.Fatalf("constraint missing from view: %+v", view.Items)
	}
	if !writeContextViewContains(view, "risk_profile", "changes_build_system=true") {
		t.Fatalf("risk profile missing from view: %+v", view.Items)
	}
	if !writeContextViewContains(view, "behavior_contract", "id=approval-gate") {
		t.Fatalf("behavior contract missing from view: %+v", view.Items)
	}
	if !writeContextViewContains(view, "behavior_contract", "polarity=expected") {
		t.Fatalf("behavior contract polarity missing from view: %+v", view.Items)
	}
	if !writeContextViewContains(view, "behavior_contract", "comparator_subject=saved plan apply") {
		t.Fatalf("behavior contract comparator subject missing from view: %+v", view.Items)
	}
	if !writeContextViewContains(view, "behavior_contract", "comparator_relation=regression_baseline") {
		t.Fatalf("behavior contract comparator relation missing from view: %+v", view.Items)
	}
	if !writeContextViewContains(view, "behavior_contract", "placement_relation=between_anchor_and_delimiter") ||
		!writeContextViewContains(view, "behavior_contract", "placement_anchor=rainfall") {
		t.Fatalf("placement contract fields missing from view: %+v", view.Items)
	}
}

func TestWriteContextPackFromWriteAnalysisIR_ObservedContractNotP0(t *testing.T) {
	ir := &WriteAnalysisIR{
		Request: WriteRequestModel{
			Task: WriteTask{Summary: "fix observed crash"},
			BehaviorContracts: []WriteBehaviorContract{{
				ID:       "observed-crash",
				Kind:     WriteBehaviorException,
				Polarity: WriteBehaviorPolarityObserved,
				Operator: WriteBehaviorOpRaises,
				Subject:  "update_normal",
				Expected: "ZeroDivisionError",
				Required: true,
				Source:   "write_analyzer",
			}},
		},
	}
	pack := WriteContextPackFromWriteAnalysisIR(ir)
	view := pack.View(WriteConsumerPlanner, 10)
	var found *WriteContextItem
	for i := range view.Items {
		if strings.Contains(view.Items[i].Text, "id=observed-crash") {
			found = &view.Items[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("observed contract should remain visible in handoff: %+v", view.Items)
	}
	if found.Priority == WriteContextP0 {
		t.Fatalf("observed pre-fix fact must not become a P0 completion target: %+v", *found)
	}
	if !strings.Contains(found.Text, "polarity=observed") {
		t.Fatalf("observed polarity missing from handoff text: %+v", *found)
	}
}

func TestWriteContextPackFromWriteAnalysisIR_SatisfiesContractIsSoft(t *testing.T) {
	ir := &WriteAnalysisIR{
		Request: WriteRequestModel{
			Task: WriteTask{Summary: "fix soft behavior"},
			BehaviorContracts: []WriteBehaviorContract{{
				ID:       "soft-empty-array",
				Kind:     WriteBehaviorObservable,
				Polarity: WriteBehaviorPolarityExpected,
				Operator: WriteBehaviorOpSatisfies,
				Subject:  "Array([])",
				Expected: "creates an empty array with shape text",
				Required: true,
				Source:   "write_analyzer",
			}},
		},
	}
	pack := WriteContextPackFromWriteAnalysisIR(ir)
	view := pack.View(WriteConsumerPlanner, 10)
	var found *WriteContextItem
	for i := range view.Items {
		if strings.Contains(view.Items[i].Text, "id=soft-empty-array") {
			found = &view.Items[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("soft contract should remain visible in handoff: %+v", view.Items)
	}
	if found.Priority == WriteContextP0 {
		t.Fatalf("satisfies contract must not become a P0 hard target: %+v", *found)
	}
	if !strings.Contains(found.Text, "soft_required=true") {
		t.Fatalf("soft_required marker missing from handoff text: %+v", *found)
	}
}

func TestWriteContextPackFromWriteAnalysisIR_ExactContractIsP0(t *testing.T) {
	ir := &WriteAnalysisIR{
		Request: WriteRequestModel{
			Task: WriteTask{Summary: "fix exact behavior"},
			BehaviorContracts: []WriteBehaviorContract{{
				ID:       "exact-empty-array-shape",
				Kind:     WriteBehaviorInvariant,
				Polarity: WriteBehaviorPolarityExpected,
				Operator: WriteBehaviorOpEquals,
				Subject:  "Array([]).shape",
				Expected: "(0,)",
				Required: true,
				Source:   "write_analyzer",
			}},
		},
	}
	pack := WriteContextPackFromWriteAnalysisIR(ir)
	view := pack.View(WriteConsumerPlanner, 10)
	var found *WriteContextItem
	for i := range view.Items {
		if strings.Contains(view.Items[i].Text, "id=exact-empty-array-shape") {
			found = &view.Items[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("exact contract should be visible in handoff: %+v", view.Items)
	}
	if found.Priority != WriteContextP0 {
		t.Fatalf("exact contract should be P0 hard target: %+v", *found)
	}
	if !strings.Contains(found.Text, "hard_required=true") {
		t.Fatalf("hard_required marker missing from handoff text: %+v", *found)
	}
}

func TestWriteContextPackFromExplorationHandoffPrioritizesEvidence(t *testing.T) {
	handoff := WriteExplorationHandoff{
		BatchID:          "batch-1",
		Goal:             "thread handoff into planner",
		TargetFiles:      []string{"internal/agent/planner.go"},
		RelevantSymbols:  []string{"BuildInitialInstruction"},
		ExistingPatterns: []string{"planner sections are pure data"},
		Invariants:       []string{"do not bypass ChangePlan validation"},
		TestSurface:      []string{"go test ./internal/agent"},
		RiskNotes:        []string{"prompt ordering can hide approval constraints"},
		Unknowns:         []string{"which planner view should render first"},
		EvidenceRefs: []WriteExplorationEvidenceRef{{
			ID:        "ev1",
			Kind:      "mechanism",
			Source:    "internal/agent/planner.go",
			LineStart: 105,
			Subject:   "BuildInitialInstruction",
			Summary:   "composes planner sections",
		}},
	}
	pack := WriteContextPackFromExplorationHandoff(handoff)
	planner := pack.View(WriteConsumerPlanner, 20)
	verifier := pack.View(WriteConsumerVerifier, 20)
	if !writeContextViewContains(planner, "risk_note", "prompt ordering") {
		t.Fatalf("risk note missing from planner view: %+v", planner.Items)
	}
	if !writeContextViewContains(planner, "evidence_ref", "BuildInitialInstruction @ internal/agent/planner.go:105") {
		t.Fatalf("evidence ref missing from planner view: %+v", planner.Items)
	}
	if !writeContextViewContains(planner, "pattern_hint", "planner sections") {
		t.Fatalf("pattern hint missing from planner view: %+v", planner.Items)
	}
	if writeContextViewContains(verifier, "pattern_hint", "planner sections") {
		t.Fatalf("verifier view should not receive planner-only pattern hints: %+v", verifier.Items)
	}
	if !writeContextViewContains(verifier, "test_surface", "go test ./internal/agent") {
		t.Fatalf("verifier test surface missing: %+v", verifier.Items)
	}
}

func TestWriteContextPackFromExplorationHandoffCarriesConventions(t *testing.T) {
	handoff := WriteExplorationHandoff{
		BatchID:          "batch-1",
		Goal:             "repair planner",
		ExistingPatterns: []string{"planner sections are pure data"},
		EvidenceRefs: []WriteExplorationEvidenceRef{{
			ID:        "ev-mechanism",
			Kind:      string(EvidenceMechanism),
			Source:    "internal/agent/planner.go",
			LineStart: 105,
			Subject:   "BuildInitialInstruction",
			Summary:   "planner composes sections from typed inputs",
		}},
	}

	pack := WriteContextPackFromExplorationHandoff(handoff)
	planner := pack.View(WriteConsumerPlanner, 20)
	verifier := pack.View(WriteConsumerVerifier, 20)
	if !writeContextViewContains(planner, "pattern_hint", "planner sections are pure data") {
		t.Fatalf("existing pattern hint should remain visible: %+v", planner.Items)
	}
	if !writeContextViewContains(planner, "convention", "category=local_pattern") {
		t.Fatalf("planner view missing local convention: %+v", planner.Items)
	}
	if !writeContextViewContains(verifier, "convention", "category=mechanism") {
		t.Fatalf("verifier view missing evidence-backed convention: %+v", verifier.Items)
	}
	if writeContextViewContains(verifier, "pattern_hint", "planner sections") {
		t.Fatalf("legacy pattern hints should remain planner-only: %+v", verifier.Items)
	}
}

func TestWriteContextPackFromChangePlanCarriesImpactObligations(t *testing.T) {
	plan := &ChangePlan{
		ID:          "plan-impact",
		Summary:     "repair caller helper contract",
		TargetPaths: []string{"caller.py", "helper.py"},
		Changes: []FileChange{{
			Path:      "caller.py",
			Kind:      "modify",
			DependsOn: []string{"helper.py"},
		}},
		VerificationProbes: []VerificationProbe{{
			ID:                "probe-contract",
			ContractRefs:      []string{"contract-1"},
			ChangedSymbolRefs: []string{"pkg.symbol"},
		}},
	}

	pack := WriteContextPackFromChangePlan(plan)
	planner := pack.View(WriteConsumerPlanner, 20)
	verifier := pack.View(WriteConsumerVerifier, 20)
	if !writeContextViewContains(planner, "impact_obligation", "relation=depends_on") {
		t.Fatalf("planner view missing dependency impact obligation: %+v", planner.Items)
	}
	if !writeContextViewContains(verifier, "impact_obligation", "contract_ref=contract-1") {
		t.Fatalf("verifier view missing contract impact obligation: %+v", verifier.Items)
	}
	if !writeContextViewContains(verifier, "impact_obligation", "symbol=pkg.symbol") {
		t.Fatalf("verifier view missing symbol impact obligation: %+v", verifier.Items)
	}
}

func TestWriteContextPackFromChangePlanCarriesPatchEffect(t *testing.T) {
	plan := &ChangePlan{
		ID:      "plan-effect",
		Summary: "repair applied behavior",
		PatchEffect: &PatchEffectRecord{
			RecordID:        "patch-effect:plan-effect:slice-1:abcdef123456",
			DiffFingerprint: "abcdef1234567890",
			Files: []PatchEffectFile{{
				Path:         "pkg/a.py",
				Status:       "modified",
				Language:     "py",
				PathRole:     SourcePathRoleProduction,
				AddedLines:   2,
				RemovedLines: 1,
				Hunks:        []PatchEffectHunk{{OldStart: 10, OldLines: 2, NewStart: 10, NewLines: 3}},
			}},
		},
		PatchReview: &PatchReviewRecord{
			ReviewID: "patch-review:plan-effect:slice-1",
			Findings: []PatchReviewFinding{{
				Code:           "changed_symbol_without_probe_coverage",
				Severity:       PatchReviewSeverityWarning,
				Category:       PatchReviewCategorySemanticCoverage,
				ImpactKind:     PatchReviewImpactKindChangedSymbol,
				Path:           "pkg/a.py",
				SubjectSymbol:  "pkg.A",
				CoverageStatus: PatchReviewCoverageUnverified,
				SourceStage:    "convention_graph",
				LineStart:      10,
				LineEnd:        12,
				ContextSummary: "axis converters validate input before conversion",
			}, {
				Code:     "scope_warning_without_evidence_projection",
				Severity: PatchReviewSeverityInfo,
				Category: PatchReviewCategoryScope,
				Path:     "pkg/a.py",
			}},
		},
		ImpactAnalysis: &ImpactAnalysisResult{
			ResultID: "impact-analysis:plan-effect:patch-effect",
			ChangedSurfaces: []ImpactChangedSurface{{
				Kind:        "symbol",
				Path:        "pkg/a.py",
				Symbol:      "pkg.A",
				Strength:    string(ImpactObligationStrengthPrecise),
				EvidenceRef: "pkg/a.py:10",
			}},
			VerificationTargets: []ImpactVerificationTarget{{
				Kind:           "changed_symbol",
				Path:           "pkg/a.py",
				Symbol:         "pkg.A",
				Priority:       20,
				CoverageStatus: "unverified",
				EvidenceRef:    "pkg/a.py:10",
			}},
			ObligationSet: &ImpactObligationSet{
				PlanID: "plan-effect",
				Obligations: []ImpactObligation{{
					Kind:          "changed_symbol",
					Relation:      "patch_hunk",
					Obligation:    "verify_changed_symbol",
					SubjectPath:   "pkg/a.py",
					SubjectSymbol: "pkg.A",
					EvidenceRef:   "pkg/a.py:10",
				}},
			},
		},
	}

	pack := WriteContextPackFromChangePlan(plan)
	planner := pack.View(WriteConsumerPlanner, 20)
	verifier := pack.View(WriteConsumerVerifier, 20)
	if !writeContextViewContains(planner, "patch_effect", "path=pkg/a.py") ||
		!writeContextViewContains(planner, "patch_effect", "added=2") ||
		!writeContextViewContains(planner, "patch_effect", "fingerprint=abcdef123456") {
		t.Fatalf("planner view missing applied patch effect: %+v", planner.Items)
	}
	if !writeContextViewContains(verifier, "patch_effect", "role=production") ||
		!writeContextViewContains(verifier, "patch_effect", "hunks=1") {
		t.Fatalf("verifier view missing applied patch effect: %+v", verifier.Items)
	}
	if !writeContextViewContains(planner, "patch_review_finding", "code=changed_symbol_without_probe_coverage") ||
		!writeContextViewContains(planner, "patch_review_finding", "impact_kind=changed_symbol") ||
		!writeContextViewContains(verifier, "patch_review_finding", "coverage_status=unverified") ||
		!writeContextViewContains(verifier, "patch_review_finding", "symbol=pkg.A") {
		t.Fatalf("patch review finding missing from context pack: planner=%+v verifier=%+v", planner.Items, verifier.Items)
	}
	if !writeContextViewContains(planner, "patch_review_evidence", "source_stage=convention_graph") ||
		!writeContextViewContains(planner, "patch_review_evidence", "line_start=10") ||
		!writeContextViewContains(verifier, "patch_review_evidence", "context_summary=axis converters validate input before conversion") {
		t.Fatalf("patch review evidence missing from context pack: planner=%+v verifier=%+v", planner.Items, verifier.Items)
	}
	if writeContextViewKindCount(planner, "patch_review_evidence") != 1 {
		t.Fatalf("unexpected patch review evidence noise: %+v", planner.Items)
	}
	if !writeContextViewContains(planner, "impact_changed_surface", "symbol=pkg.A") ||
		!writeContextViewContains(verifier, "impact_verification_target", "priority=20") ||
		!writeContextViewContains(verifier, "impact_verification_target", "coverage_status=unverified") {
		t.Fatalf("impact analysis projection missing from context pack: planner=%+v verifier=%+v", planner.Items, verifier.Items)
	}
}

func TestWriteContextPackFromPlanContextCoverageExcludesPlanSelfContext(t *testing.T) {
	prior := []WriteContextPack{{
		PackID:      "exploration-handoff",
		BatchID:     "batch-1",
		SourceStage: "explore",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "bug.py",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "helper.py",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "tests/test_bug.py",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}, {
		PackID:      "change-plan",
		BatchID:     "batch-1",
		SourceStage: "plan",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "self.py",
			SourceStage: "plan",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}}
	plan := &ChangePlan{
		ID:          "plan-1",
		Summary:     "repair bug",
		TargetPaths: []string{"bug.py"},
		Changes:     []FileChange{{Path: "bug.py", Kind: "modify"}},
	}

	pack := WriteContextPackFromPlanContextCoverage("batch-1", "repair", prior, plan)
	view := pack.View(WriteConsumerPlanner, 10)
	if !writeContextViewContains(view, "plan_context_coverage", "covered=1/2") ||
		!writeContextViewContains(view, "plan_context_coverage", "ratio=0.5000") ||
		!writeContextViewContains(view, "plan_context_coverage", "uncovered_paths=[helper.py]") {
		t.Fatalf("coverage summary should be based on prior non-test context only: %+v", view.Items)
	}
	if !writeContextViewContains(view, "plan_context_uncovered_path", "helper.py") {
		t.Fatalf("missing uncovered production path: %+v", view.Items)
	}
	if writeContextViewContains(view, "plan_context_coverage", "tests/test_bug.py") ||
		writeContextViewContains(view, "plan_context_coverage", "self.py") {
		t.Fatalf("coverage must exclude test paths and plan-authored self context: %+v", view.Items)
	}
}

func TestWriteContextPackFromPlanContextCoverageReportsMissingPriorContext(t *testing.T) {
	plan := &ChangePlan{
		ID:          "plan-1",
		Summary:     "repair bug",
		TargetPaths: []string{"pkg/a.py", "pkg/tests/test_a.py"},
		Changes: []FileChange{
			{Path: "pkg/a.py", Kind: "modify"},
			{Path: "pkg/b.py", Kind: "modify"},
			{Path: "pkg/tests/test_a.py", Kind: "modify"},
		},
	}

	pack := WriteContextPackFromPlanContextCoverage("batch-1", "repair", nil, plan)
	view := pack.View(WriteConsumerPlanner, 10)
	if !writeContextViewContains(view, "plan_context_coverage", "covered=0/0") ||
		!writeContextViewContains(view, "plan_context_coverage", "missing_source_paths=[pkg/a.py,pkg/b.py]") {
		t.Fatalf("coverage summary should report source paths when prior context is absent: %+v", view.Items)
	}
	if !writeContextViewContains(view, "plan_context_missing_source_path", "pkg/a.py") ||
		!writeContextViewContains(view, "plan_context_missing_source_path", "pkg/b.py") {
		t.Fatalf("missing source paths should be typed context items: %+v", view.Items)
	}
	if writeContextViewContains(view, "plan_context_missing_source_path", "pkg/tests/test_a.py") {
		t.Fatalf("test paths should not be reported as missing source context: %+v", view.Items)
	}
}

func TestWriteContextPackFromPlanContextCoverageReportsScopeOnlyOwnerGap(t *testing.T) {
	prior := []WriteContextPack{{
		PackID:      "write-analysis",
		BatchID:     "batch-1",
		SourceStage: "write_analysis",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "scope_anchor",
			Text:        "pkg/bug.py",
			SourceStage: "write_analysis",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner, WriteConsumerController},
		}},
	}}
	plan := &ChangePlan{
		ID:          "plan-1",
		Summary:     "repair bug",
		TargetPaths: []string{"pkg/bug.py"},
		Changes:     []FileChange{{Path: "pkg/bug.py", Kind: "modify"}},
	}

	pack := WriteContextPackFromPlanContextCoverage("batch-1", "repair", prior, plan)
	view := pack.View(WriteConsumerPlanner, 10)
	if !writeContextViewContains(view, "plan_context_coverage", "covered=1/1") {
		t.Fatalf("scope path should still cover file-level localization: %+v", view.Items)
	}
	if !writeContextViewContains(view, "plan_context_owner_depth", "owner_anchor_missing_paths=[pkg/bug.py]") ||
		!writeContextViewContains(view, "plan_context_owner_gap_path", "pkg/bug.py") {
		t.Fatalf("scope-only context must report missing owner-depth evidence: %+v", view.Items)
	}
	if !writeContextViewContains(view, "localization_requirement", "path=pkg/bug.py") ||
		!writeContextViewContains(view, "localization_requirement", "reason=plan_source_path_without_owner_anchor") {
		t.Fatalf("scope-only context must project a typed localization requirement: %+v", view.Items)
	}
	if got := WritePlanSourcePathsWithoutOwnerAnchor(prior, plan); strings.Join(got, ",") != "pkg/bug.py" {
		t.Fatalf("owner-gap paths = %+v, want pkg/bug.py", got)
	}
}

func TestWritePlanSourcePathsWithoutOwnerAnchorSatisfiedByOwnerAnchor(t *testing.T) {
	ref := WriteExplorationEvidenceRef{ID: "ev-owner", Source: "pkg/bug.py", LineStart: 12, OwnerSymbol: "Owner.fix"}
	prior := []WriteContextPack{{
		PackID:      "exploration-handoff",
		BatchID:     "batch-1",
		SourceStage: "explore",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "localization_anchor",
			Text:        "path=pkg/bug.py owner=Owner.fix",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner, WriteConsumerController},
			EvidenceRef: &ref,
			LocalizationAnchor: &SourceLocalizationAnchor{
				Path:        "pkg/bug.py",
				Role:        SourcePathRoleProduction,
				Kind:        SourceLocalizationAnchorGroundedEvidence,
				Strength:    SourceLocalizationAnchorOwner,
				EvidenceRef: &ref,
				OwnerSymbol: "Owner.fix",
			},
		}},
	}}
	plan := &ChangePlan{
		ID:          "plan-1",
		Summary:     "repair bug",
		TargetPaths: []string{"pkg/bug.py"},
		Changes:     []FileChange{{Path: "pkg/bug.py", Kind: "modify"}},
	}

	if got := WritePlanSourcePathsWithoutOwnerAnchor(prior, plan); len(got) != 0 {
		t.Fatalf("owner evidence should satisfy owner-depth localization, got %+v", got)
	}
	pack := WriteContextPackFromPlanContextCoverage("batch-1", "repair", prior, plan)
	if writeContextViewContains(pack.View(WriteConsumerPlanner, 10), "plan_context_owner_gap_path", "pkg/bug.py") {
		t.Fatalf("owner-depth gap should not be reported when evidence anchor exists: %+v", pack.Items)
	}
}

func TestWritePlanSourcePathsWithoutOwnerAnchorNotSatisfiedBySupportingAnchor(t *testing.T) {
	ref := WriteExplorationEvidenceRef{ID: "ev-support", Source: "pkg/bug.py", LineStart: 12, OwnerSymbol: "Owner.fix"}
	prior := []WriteContextPack{{
		PackID:      "exploration-handoff",
		BatchID:     "batch-1",
		SourceStage: "explore",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "localization_anchor",
			Text:        "path=pkg/bug.py support=Owner.fix",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner, WriteConsumerController},
			EvidenceRef: &ref,
			LocalizationAnchor: &SourceLocalizationAnchor{
				Path:        "pkg/bug.py",
				Role:        SourcePathRoleProduction,
				Kind:        SourceLocalizationAnchorGroundedEvidence,
				Strength:    SourceLocalizationAnchorSupporting,
				EvidenceRef: &ref,
				OwnerSymbol: "Owner.fix",
			},
		}},
	}}
	plan := &ChangePlan{
		ID:          "plan-1",
		Summary:     "repair bug",
		TargetPaths: []string{"pkg/bug.py"},
		Changes:     []FileChange{{Path: "pkg/bug.py", Kind: "modify"}},
	}

	if got := WritePlanSourcePathsWithoutOwnerAnchor(prior, plan); strings.Join(got, ",") != "pkg/bug.py" {
		t.Fatalf("supporting anchor must not satisfy owner-depth localization, got %+v", got)
	}
	pack := WriteContextPackFromPlanContextCoverage("batch-1", "repair", prior, plan)
	if !writeContextViewContains(pack.View(WriteConsumerPlanner, 10), "plan_context_owner_gap_path", "pkg/bug.py") {
		t.Fatalf("supporting-only anchor must report owner-depth gap: %+v", pack.Items)
	}
}

func TestWriteContextPackFromPlannerToolResultsProjectsReadFileObservation(t *testing.T) {
	results := []ToolResult{{
		ToolName: "read_file",
		Success:  true,
		RawRef:   "blob://read",
		Observations: []ObservationRecord{{
			ID:              "read_file:pkg/bug.py:10:20",
			Origin:          AnswerEvidenceOriginCurrentSource,
			Producer:        "read_file",
			SourceRef:       ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "pkg/bug.py", RawRef: "blob://read"},
			Span:            ObservationSpan{LineStart: 10, LineEnd: 20},
			EvidenceKind:    EvidenceDirect,
			EvidenceScope:   ScopeLineRange,
			GroundingStatus: GroundingGrounded,
			Summary:         "read_file observed current source bytes",
		}},
	}}

	pack := WriteContextPackFromPlannerToolResults("batch-1", "repair", results)
	view := pack.View(WriteConsumerPlanner, 10)
	if !writeContextViewContains(view, "localization_anchor", "path=pkg/bug.py") ||
		!writeContextViewContains(view, "localization_anchor", "kind=read_file") ||
		!writeContextViewContains(view, "localization_anchor", "strength=observed") {
		t.Fatalf("planner read_file observation not projected as observed localization anchor: %+v", view.Items)
	}
	if got := WritePlanSourcePathsWithoutOwnerAnchor([]WriteContextPack{pack}, &ChangePlan{
		ID:          "plan-1",
		TargetPaths: []string{"pkg/bug.py"},
		Changes:     []FileChange{{Path: "pkg/bug.py", Kind: "modify"}},
	}); len(got) != 0 {
		t.Fatalf("observed read_file-only context should not be treated as path-covered owner gap input; got %+v", got)
	}
}

func TestWriteContextPackFromRepoMapNavigationCoverageProjectsP1State(t *testing.T) {
	coverage := RepoMapNavigationCoverage{
		State:          RepoMapNavigationCoveragePartial,
		ReasonCode:     "repo_map_navigation_partial",
		RequiredRoutes: []RepoMapNavigationRoute{RepoMapNavigationRouteTaskMap, RepoMapNavigationRouteRelationMap},
		CoveredRoutes:  []RepoMapNavigationRoute{RepoMapNavigationRouteTaskMap},
		MissingRoutes:  []RepoMapNavigationRoute{RepoMapNavigationRouteRelationMap},
		ObservedRoutes: []RepoMapNavigationRoute{RepoMapNavigationRouteOverview, RepoMapNavigationRouteTaskMap},
		EvidenceRefs:   []string{"blob://repo-map-task"},
	}
	pack := WriteContextPackFromRepoMapNavigationCoverage("batch-1", "repair", coverage)
	view := pack.View(WriteConsumerPlanner, 10)
	if !writeContextViewContains(view, "repo_map_navigation_coverage", "state=partial") ||
		!writeContextViewContains(view, "repo_map_navigation_coverage", "missing=relation_map") ||
		!writeContextViewContains(view, "repo_map_navigation_coverage", "covered=task_map") {
		t.Fatalf("navigation coverage context missing typed state: %+v", view.Items)
	}
	if view.Items[0].Priority != WriteContextP1 {
		t.Fatalf("navigation coverage should be P1, got %+v", view.Items[0])
	}
	if view.Items[0].EvidenceRef == nil || view.Items[0].EvidenceRef.ID != "blob://repo-map-task" {
		t.Fatalf("navigation coverage should carry evidence ref: %+v", view.Items[0])
	}
}

func TestWriteContextPackFromPlanContextCoverageNormalizesSymbolAnchorsToFiles(t *testing.T) {
	prior := []WriteContextPack{{
		PackID:      "exploration-handoff",
		BatchID:     "batch-1",
		SourceStage: "explore",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "src/_pytest/python_api.py:RaisesContext",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}}
	plan := &ChangePlan{
		ID:          "plan-1",
		Summary:     "repair bug",
		TargetPaths: []string{"src/_pytest/python_api.py"},
		Changes:     []FileChange{{Path: "src/_pytest/python_api.py", Kind: "modify"}},
	}

	pack := WriteContextPackFromPlanContextCoverage("batch-1", "repair", prior, plan)
	view := pack.View(WriteConsumerPlanner, 10)
	if !writeContextViewContains(view, "plan_context_coverage", "covered=1/1") ||
		writeContextViewContains(view, "plan_context_uncovered_path", "src/_pytest/python_api.py") {
		t.Fatalf("symbol-qualified context anchors should cover the changed source file: %+v", view.Items)
	}
}

func TestWriteExplorationHandoffCarriesSourceLocalizationAnchors(t *testing.T) {
	turnA := TurnAArtifacts{
		ReadFiles: []string{"src/read_only.py"},
		EvidenceItems: []EvidenceItem{{
			ID:              "ev-owner",
			Kind:            EvidenceDirect,
			Source:          "src/owner.py",
			LineStart:       12,
			Subject:         "Owner",
			AnchorSymbol:    "Owner.handle",
			GroundingStatus: GroundingGrounded,
			Scope:           ScopeLine,
		}},
	}

	handoff := WriteExplorationHandoffFromTurnA(WriteExplorationRequest{BatchID: "batch-1", Goal: "fix"}, turnA)
	if handoff.SourceLocalization == nil {
		t.Fatalf("source localization not carried")
	}
	if !sourceLocalizationTestHasAnchor(*handoff.SourceLocalization, "src/owner.py", SourceLocalizationAnchorGroundedEvidence, SourceLocalizationAnchorOwner) {
		t.Fatalf("owner anchor missing from handoff: %+v", handoff.SourceLocalization.Anchors)
	}
}

func TestWriteContextPackFromExplorationHandoffProjectsLocalizationAnchors(t *testing.T) {
	ref := WriteExplorationEvidenceRef{ID: "ev-owner", Source: "src/owner.py", LineStart: 12, Subject: "Owner.handle", OwnerSymbol: "Owner", AnchorSymbol: "if"}
	pack := WriteContextPackFromExplorationHandoff(WriteExplorationHandoff{
		BatchID:     "batch-1",
		Goal:        "fix",
		TargetFiles: []string{"src/read_only.py", "src/owner.py"},
		SourceLocalization: &SourceLocalizationReview{
			Anchors: []SourceLocalizationAnchor{{
				Path:         "src/owner.py",
				Role:         SourcePathRoleProduction,
				SourceStage:  "read_turn_a",
				Kind:         SourceLocalizationAnchorGroundedEvidence,
				Strength:     SourceLocalizationAnchorOwner,
				EvidenceRef:  &ref,
				Subject:      "Owner.handle",
				OwnerSymbol:  "Owner",
				AnchorSymbol: "if",
			}},
		},
	})

	view := pack.View(WriteConsumerPlanner, 20)
	if !writeContextViewContains(view, "localization_anchor", "path=src/owner.py") ||
		!writeContextViewContains(view, "localization_anchor", "strength=owner") ||
		!writeContextViewContains(view, "localization_anchor", "owner=Owner") ||
		!writeContextViewContains(view, "localization_anchor", "anchor=if") {
		t.Fatalf("localization anchor not rendered: %+v", view.Items)
	}
	var foundTyped bool
	for _, item := range view.Items {
		if item.Kind == "localization_anchor" && item.LocalizationAnchor != nil && item.LocalizationAnchor.Path == "src/owner.py" &&
			item.LocalizationAnchor.OwnerSymbol == "Owner" && item.EvidenceRef != nil && item.EvidenceRef.OwnerSymbol == "Owner" {
			foundTyped = true
		}
	}
	if !foundTyped {
		t.Fatalf("typed localization anchor missing: %+v", view.Items)
	}
}

func TestWritePlanSourcePathsOutsidePriorContextPrefersTypedOwnerAnchors(t *testing.T) {
	prior := []WriteContextPack{{
		PackID:      "exploration-handoff",
		BatchID:     "batch-1",
		SourceStage: "explore",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "pkg/read_only.py",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP1,
			Kind:        "localization_anchor",
			Text:        "path=pkg/owner.py strength=owner",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
			LocalizationAnchor: &SourceLocalizationAnchor{
				Path:     "pkg/owner.py",
				Role:     SourcePathRoleProduction,
				Kind:     SourceLocalizationAnchorGroundedEvidence,
				Strength: SourceLocalizationAnchorOwner,
			},
		}},
	}}
	plan := &ChangePlan{
		ID:          "plan-1",
		TargetPaths: []string{"pkg/owner.py", "pkg/read_only.py"},
		Changes: []FileChange{
			{Path: "pkg/owner.py", Kind: "modify"},
			{Path: "pkg/read_only.py", Kind: "modify"},
		},
	}

	got := WritePlanSourcePathsOutsidePriorContext(prior, plan)
	if strings.Join(got, ",") != "pkg/read_only.py" {
		t.Fatalf("typed owner anchors should outrank broad target_file rows, got %+v", got)
	}
}

func TestWritePlanSourcePathsOutsidePriorContextObservedAnchorIsNotOwnerProof(t *testing.T) {
	prior := []WriteContextPack{{
		PackID:      "exploration-handoff",
		BatchID:     "batch-1",
		SourceStage: "explore",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "pkg/read_only.py",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP2,
			Kind:        "localization_anchor",
			Text:        "path=pkg/read_only.py strength=observed",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
			LocalizationAnchor: &SourceLocalizationAnchor{
				Path:     "pkg/read_only.py",
				Role:     SourcePathRoleProduction,
				Kind:     SourceLocalizationAnchorReadFile,
				Strength: SourceLocalizationAnchorObserved,
			},
		}},
	}}
	plan := &ChangePlan{
		ID:          "plan-1",
		TargetPaths: []string{"pkg/read_only.py"},
		Changes:     []FileChange{{Path: "pkg/read_only.py", Kind: "modify"}},
	}

	got := WritePlanSourcePathsOutsidePriorContext(prior, plan)
	if strings.Join(got, ",") != "pkg/read_only.py" {
		t.Fatalf("observed read-file anchors must not satisfy owner localization, got %+v", got)
	}
}

func TestWritePlanSourcePathsOutsidePriorContext(t *testing.T) {
	prior := []WriteContextPack{{
		PackID:      "write-analysis",
		BatchID:     "batch-1",
		SourceStage: "write_analysis",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "scope_anchor",
			Text:        "pkg/owner.py",
			SourceStage: "write_analysis",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "pkg/covered",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}}

	plan := &ChangePlan{
		ID:          "plan-1",
		TargetPaths: []string{"pkg/owner.py", "pkg/caller.py", "tests/test_owner.py", "pkg/covered/helper.py"},
		Changes: []FileChange{
			{Path: "pkg/owner.py", Kind: "modify"},
			{Path: "pkg/caller.py", Kind: "modify"},
			{Path: "tests/test_owner.py", Kind: "modify"},
			{Path: "pkg/covered/helper.py", Kind: "modify"},
		},
	}

	got := WritePlanSourcePathsOutsidePriorContext(prior, plan)
	if strings.Join(got, ",") != "pkg/caller.py" {
		t.Fatalf("missing localization paths = %+v, want [pkg/caller.py]", got)
	}
	if got := WritePlanSourcePathsOutsidePriorContext(nil, plan); len(got) != 0 {
		t.Fatalf("no prior context should not create routing-grade missing paths: %+v", got)
	}
	testOnly := &ChangePlan{ID: "plan-test", TargetPaths: []string{"tests/test_owner.py"}, Changes: []FileChange{{Path: "tests/test_owner.py", Kind: "modify"}}}
	if got := WritePlanSourcePathsOutsidePriorContext(prior, testOnly); len(got) != 0 {
		t.Fatalf("test-only changes should be excluded from localization miss routing: %+v", got)
	}
}

func TestWriteContextPackFromChangePlanCarriesSourceLocalizationReview(t *testing.T) {
	ref := WriteExplorationEvidenceRef{ID: "ev-owner", Source: "pkg/owner.py", LineStart: 12, Subject: "Owner", AnchorSymbol: "Owner.handle"}
	plan := &ChangePlan{
		ID:          "plan-loc",
		Summary:     "repair owner",
		TargetPaths: []string{"pkg/caller.py"},
		Changes:     []FileChange{{Path: "pkg/caller.py", Kind: "modify"}},
		LocalizationReview: &SourceLocalizationReview{
			Status:            SourceLocalizationMissing,
			Source:            "write_plan_context",
			PlanID:            "plan-loc",
			SourcePaths:       []string{"pkg/caller.py"},
			PriorContextPaths: []string{"pkg/owner.py"},
			MissingPaths:      []string{"pkg/caller.py"},
			ReasonCodes:       []string{"plan_source_paths_missing_prior_context"},
			Anchors: []SourceLocalizationAnchor{{
				Path:         "pkg/owner.py",
				Role:         SourcePathRoleProduction,
				SourceStage:  "read_turn_a",
				Kind:         SourceLocalizationAnchorGroundedEvidence,
				Strength:     SourceLocalizationAnchorOwner,
				EvidenceRef:  &ref,
				Subject:      "Owner",
				AnchorSymbol: "Owner.handle",
			}},
		},
	}

	pack := WriteContextPackFromChangePlan(plan)
	view := pack.View(WriteConsumerPlanner, 20)
	if !writeContextViewContains(view, "source_localization_review", "status=missing") ||
		!writeContextViewContains(view, "source_localization_missing_path", "pkg/caller.py") ||
		!writeContextViewContains(view, "localization_anchor", "path=pkg/owner.py") ||
		!writeContextViewContains(view, "localization_anchor", "anchor=Owner.handle") {
		t.Fatalf("source localization review not rendered into context pack: %+v", view.Items)
	}
	var foundTyped bool
	for _, item := range view.Items {
		if item.Kind == "localization_anchor" &&
			item.SourceStage == "plan_localization" &&
			item.LocalizationAnchor != nil &&
			item.LocalizationAnchor.Path == "pkg/owner.py" &&
			item.EvidenceRef != nil &&
			item.EvidenceRef.ID == "ev-owner" {
			foundTyped = true
		}
	}
	if !foundTyped {
		t.Fatalf("typed localization anchor not projected from plan review: %+v", view.Items)
	}
}

func TestWriteContextPackFromChangeReportCarriesVerifyFailure(t *testing.T) {
	report := &ChangeReport{
		PlanID:                "plan-1",
		Passed:                false,
		BuildFailed:           true,
		FailureSummary:        "compile failed",
		FailureSummaryBlobRef: "/tmp/codrax/blob/run-tests-plan-1.txt",
		FailureReasonCode:     "pytest_import_startup_error",
		TestResults: []TestResult{{
			Kind:          TestResultKindBuildError,
			AssertionID:   "internal/foo.go:10",
			Suite:         "build",
			FailureDetail: "undefined: Foo",
			BuildErrors: []BuildError{{
				File:    "internal/foo.go",
				Line:    10,
				Symbol:  "Foo",
				Message: "undefined: Foo",
			}},
		}},
		ExecutedCommands: []ExecutedCommand{{
			Runner:     "python",
			Framework:  "pytest",
			WorkingDir: ".",
			Command:    "pytest",
			ExitCode:   1,
			Outcome:    "parser_error",
			ReasonCode: "pytest_import_startup_error",
		}},
		VerificationDiagnostics: []VerificationDiagnostic{{
			Source:     "pre_suite_verification_probe",
			Category:   "probe_authoring",
			Severity:   "warning",
			ReasonCode: "verification_probe_name_error",
			Runner:     "verification_probe",
			Outcome:    "parser_error",
		}},
		VerificationConfidence: []VerificationConfidenceRecord{{
			Source:     "verification_probe",
			Category:   "probe_contract_refs",
			Status:     "missing",
			Severity:   "warning",
			ReasonCode: "verification_probe_missing_required_contract_ref",
		}},
		RegressionAssertions: []string{"TestExisting"},
	}
	pack := WriteContextPackFromChangeReport(report)
	view := pack.View(WriteConsumerPlanner, 10)
	for _, want := range []struct {
		kind string
		text string
	}{
		{"build_failure", "compile failed"},
		{"failure_summary_blob_ref", "/tmp/codrax/blob/run-tests-plan-1.txt"},
		{"failure_reason_code", "pytest_import_startup_error"},
		{"verification_diagnostic", "category=probe_authoring"},
		{"verification_diagnostic", "reason_code=verification_probe_name_error"},
		{"verification_confidence", "category=probe_contract_refs"},
		{"verification_confidence", "reason_code=verification_probe_missing_required_contract_ref"},
		{"failure_signal", "assertion=internal/foo.go:10"},
		{"failed_test", "undefined: Foo"},
		{"build_error", "internal/foo.go:10 Foo undefined: Foo"},
		{"executed_command", "reason_code=pytest_import_startup_error"},
		{"regression_assertion", "TestExisting"},
	} {
		if !writeContextViewContains(view, want.kind, want.text) {
			t.Fatalf("missing %s/%q from verify context: %+v", want.kind, want.text, view.Items)
		}
	}
}

func TestWriteContextPackFromChangeReportCarriesNoTestsToController(t *testing.T) {
	report := &ChangeReport{PlanID: "plan-no-tests", Passed: true, NoTestsRunners: []string{"python"}}
	pack := WriteContextPackFromChangeReport(report)
	view := pack.View(WriteConsumerController, 10)
	if !writeContextViewContains(view, "no_tests_runner", "python") {
		t.Fatalf("controller must receive no-tests context: %+v", view.Items)
	}
}

func TestWriteContextPackFromChangeReportDedupesVerifyFailureIdentity(t *testing.T) {
	first := WriteContextPackFromChangeReport(&ChangeReport{
		PlanID:         "plan-1",
		Passed:         false,
		FailureKind:    FailureKindTestsFailed,
		FailureSummary: "first run failed",
		TestResults: []TestResult{{
			AssertionID:   "TestA",
			Suite:         "pkg",
			Passed:        false,
			FailureDetail: "want 1 got 2",
			BuildErrors: []BuildError{{
				File:    "src/x.go",
				Line:    7,
				Symbol:  "Foo",
				Message: "undefined: Foo",
			}},
		}},
		ExecutedCommands: []ExecutedCommand{{
			Runner:     "go",
			WorkingDir: ".",
			Command:    "go test ./...",
			ExitCode:   1,
			Outcome:    "executed",
		}},
	})
	second := WriteContextPackFromChangeReport(&ChangeReport{
		PlanID:         "plan-1",
		Passed:         false,
		FailureKind:    FailureKindTestsFailed,
		FailureSummary: "second run failed with the same shape",
		TestResults: []TestResult{{
			AssertionID:   "TestA",
			Suite:         "pkg",
			Passed:        false,
			FailureDetail: "want 1 got 3",
			BuildErrors: []BuildError{{
				File:    "src/x.go",
				Line:    7,
				Symbol:  "Foo",
				Message: "still undefined: Foo",
			}},
		}},
		ExecutedCommands: []ExecutedCommand{{
			Runner:     "go",
			WorkingDir: ".",
			Command:    "go test ./...",
			ExitCode:   1,
			Outcome:    "executed",
		}},
	})
	merged := MergeWriteContextPacks("batch-1", "repair", first, second)
	view := merged.View(WriteConsumerPlanner, 20)
	for _, kind := range []string{"failed_test", "build_error", "executed_command"} {
		if got := writeContextViewKindCount(view, kind); got != 1 {
			t.Fatalf("%s should dedupe by typed identity, got %d: %+v", kind, got, view.Items)
		}
	}
}

func TestWriteContextPackPlannerViewPrioritizesVerifyFailureBeforeStaleP1Facts(t *testing.T) {
	oldFacts := WriteContextPack{
		PackID:      "old-exploration",
		BatchID:     "batch-1",
		SourceStage: "explore",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "internal/old.go",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}
	failure := WriteContextPackFromChangeReport(&ChangeReport{
		PlanID:         "plan-1",
		Passed:         false,
		FailureKind:    FailureKindTestsFailed,
		FailureSummary: "red test",
		TestResults: []TestResult{{
			AssertionID: "TestA",
			Suite:       "pkg",
			Passed:      false,
		}},
	})
	view := MergeWriteContextPacks("batch-1", "repair", oldFacts, failure).View(WriteConsumerPlanner, 2)
	if len(view.Items) == 0 || view.Items[0].Kind != "verify_failure" {
		t.Fatalf("planner replan view should lead with verify failure before stale P1 facts: %+v", view.Items)
	}
}

func TestNormalizeWriteContextPackMergesDuplicateConsumerViews(t *testing.T) {
	pack := NormalizeWriteContextPack(WriteContextPack{
		PackID: "pack-1",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "internal/write.go",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "internal/write.go",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerVerifier},
		}},
	})
	if len(pack.Items) != 1 {
		t.Fatalf("duplicate typed fact should merge, got %+v", pack.Items)
	}
	if !writeContextConsumersContain(pack.Items[0].Consumers, WriteConsumerPlanner) ||
		!writeContextConsumersContain(pack.Items[0].Consumers, WriteConsumerVerifier) {
		t.Fatalf("duplicate merge should union consumers, got %+v", pack.Items[0].Consumers)
	}
	if got := len(pack.View(WriteConsumerPlanner, 10).Items); got != 1 {
		t.Fatalf("planner view should see merged fact once, got %d", got)
	}
	if got := len(pack.View(WriteConsumerVerifier, 10).Items); got != 1 {
		t.Fatalf("verifier view should see merged fact once, got %d", got)
	}
}

func TestWriteContextPackScopedViewCarriesP0ButFiltersStaleBatchEvidence(t *testing.T) {
	oldFailure := WriteContextPack{
		PackID:      "old-failure",
		BatchID:     "batch-1",
		SourceStage: "verify",
		Items: []WriteContextItem{{
			Priority:    WriteContextP0,
			Kind:        "constraint",
			Text:        "do not touch persistence",
			SourceStage: "write_analysis",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP2,
			Kind:        "verify_failure",
			Text:        "batch-1 red test",
			SourceStage: "verify",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}
	newTarget := WriteContextPack{
		PackID:      "new-target",
		BatchID:     "batch-2",
		SourceStage: "explore",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "pkg/new.py",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}
	view := MergeWriteContextPacks("batch-1", "repair", oldFailure, newTarget).
		ViewForScope(WriteConsumerPlanner, 10, "batch-2", "")
	if !writeContextViewContains(view, "constraint", "do not touch persistence") {
		t.Fatalf("P0 global safety context should carry across batch scope: %+v", view.Items)
	}
	if !writeContextViewContains(view, "target_file", "pkg/new.py") {
		t.Fatalf("active batch context should be visible: %+v", view.Items)
	}
	if writeContextViewContains(view, "verify_failure", "batch-1 red test") {
		t.Fatalf("stale batch verify failure leaked into scoped view: %+v", view.Items)
	}
}

func TestWriteContextPackScopedViewFiltersStaleItems(t *testing.T) {
	pack := NormalizeWriteContextPack(WriteContextPack{
		PackID:  "mixed",
		BatchID: "batch-1",
		Items: []WriteContextItem{{
			Priority:  WriteContextP2,
			Kind:      "verify_failure",
			Text:      "old failure",
			Consumers: []WriteContextConsumer{WriteConsumerPlanner},
			Stale:     true,
		}, {
			Priority:  WriteContextP0,
			Kind:      "constraint",
			Text:      "hard boundary",
			Consumers: []WriteContextConsumer{WriteConsumerPlanner},
			Stale:     true,
		}},
	})
	view := pack.ViewForScope(WriteConsumerPlanner, 10, "batch-1", "")
	if writeContextViewContains(view, "verify_failure", "old failure") {
		t.Fatalf("stale non-P0 item should not render: %+v", view.Items)
	}
	if !writeContextViewContains(view, "constraint", "hard boundary") {
		t.Fatalf("stale P0 safety boundary should still render: %+v", view.Items)
	}
}

func TestWriteContextPackWithScopeRebasesArtifactPackToWorkflowBatch(t *testing.T) {
	reportPack := WriteContextPackFromChangeReport(&ChangeReport{
		PlanID:         "plan-1",
		Passed:         false,
		FailureKind:    FailureKindTestsFailed,
		FailureSummary: "red test",
	})
	scoped := reportPack.WithScope("batch-1", "slice-2")
	view := scoped.ViewForScope(WriteConsumerPlanner, 10, "batch-1", "slice-2")
	if !writeContextViewContains(view, "verify_failure", "red test") {
		t.Fatalf("scoped report pack should remain visible to active batch: %+v", view.Items)
	}
	if got := scoped.BatchID; got != "batch-1" {
		t.Fatalf("scoped pack batch id = %q, want batch-1", got)
	}
	for _, item := range scoped.Items {
		if item.BatchID != "batch-1" || item.SliceID != "slice-2" {
			t.Fatalf("item scope not rebound: %+v", item)
		}
	}
}

func TestWriteContextPackPlannerLimitedViewRetainsVerifyFailureLane(t *testing.T) {
	manyP0 := WriteContextPack{
		PackID:      "risk",
		BatchID:     "batch-1",
		SourceStage: "risk",
	}
	for i := 0; i < 8; i++ {
		manyP0.Items = append(manyP0.Items, WriteContextItem{
			Priority:    WriteContextP0,
			Kind:        "constraint",
			Text:        fmt.Sprintf("hard constraint %d", i),
			SourceStage: "risk",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		})
	}
	failure := WriteContextPackFromChangeReport(&ChangeReport{
		PlanID:         "plan-1",
		Passed:         false,
		FailureKind:    FailureKindTestsFailed,
		FailureSummary: "red test",
		TestResults: []TestResult{{
			AssertionID: "TestRepair",
			Suite:       "pkg",
			Passed:      false,
		}},
	})
	view := MergeWriteContextPacks("batch-1", "repair", manyP0, failure).View(WriteConsumerPlanner, 5)
	if len(view.Items) != 5 {
		t.Fatalf("limited view should remain bounded, got %d: %+v", len(view.Items), view.Items)
	}
	if !writeContextViewContains(view, "verify_failure", "red test") {
		t.Fatalf("planner limited view should retain typed verify failure lane: %+v", view.Items)
	}
	if got := writeContextViewPriorityCount(view, WriteContextP0); got < 3 {
		t.Fatalf("planner limited view should retain core p0 context, got %d: %+v", got, view.Items)
	}
}

func TestWriteContextPackPlannerViewRanksOwnerAnchorBeforeBroadTargets(t *testing.T) {
	pack := WriteContextPack{
		PackID:  "localization",
		BatchID: "batch-1",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "pkg/broad.py",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP1,
			Kind:        "localization_anchor",
			Text:        "path=pkg/owner.py strength=owner",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
			LocalizationAnchor: &SourceLocalizationAnchor{
				Path:         "pkg/owner.py",
				Kind:         SourceLocalizationAnchorGroundedEvidence,
				Strength:     SourceLocalizationAnchorOwner,
				AnchorSymbol: "Owner.handle",
			},
		}, {
			Priority:    WriteContextP1,
			Kind:        "evidence_ref",
			Text:        "pkg/support.py:9",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}

	view := pack.View(WriteConsumerPlanner, 3)
	if len(view.Items) != 3 {
		t.Fatalf("items = %+v", view.Items)
	}
	if view.Items[0].Kind != "localization_anchor" || view.Items[0].LocalizationAnchor == nil ||
		view.Items[0].LocalizationAnchor.Path != "pkg/owner.py" {
		t.Fatalf("owner anchor should rank first in planner view: %+v", view.Items)
	}
}

func TestWriteContextPackPlannerLimitedViewRetainsOwnerAnchor(t *testing.T) {
	pack := WriteContextPack{
		PackID:  "limited-localization",
		BatchID: "batch-1",
	}
	for i := 0; i < 6; i++ {
		pack.Items = append(pack.Items, WriteContextItem{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        fmt.Sprintf("pkg/broad_%d.py", i),
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		})
	}
	pack.Items = append(pack.Items, WriteContextItem{
		Priority:    WriteContextP1,
		Kind:        "localization_anchor",
		Text:        "path=pkg/owner.py strength=owner",
		SourceStage: "explore",
		Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		LocalizationAnchor: &SourceLocalizationAnchor{
			Path:         "pkg/owner.py",
			Kind:         SourceLocalizationAnchorGroundedEvidence,
			Strength:     SourceLocalizationAnchorOwner,
			AnchorSymbol: "Owner.handle",
		},
	})

	view := pack.View(WriteConsumerPlanner, 3)
	if len(view.Items) != 3 {
		t.Fatalf("limited items = %+v", view.Items)
	}
	if !writeContextViewContains(view, "localization_anchor", "pkg/owner.py") {
		t.Fatalf("limited planner view should retain owner anchor: %+v", view.Items)
	}
}

func TestNormalizeWriteContextPackRetainsLateVerifyFailureWhenPackFull(t *testing.T) {
	pack := WriteContextPack{
		PackID:  "merged",
		BatchID: "batch-1",
	}
	for i := 0; i < writeContextPackMaxItems; i++ {
		pack.Items = append(pack.Items, WriteContextItem{
			Priority:    WriteContextP2,
			Kind:        "impact_obligation",
			Text:        fmt.Sprintf("ordinary obligation %02d", i),
			SourceStage: "impact_analysis",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		})
	}
	pack.Items = append(pack.Items,
		WriteContextItem{
			Priority:    WriteContextP2,
			Kind:        "verify_failure",
			Text:        "verification failed after apply",
			SourceStage: "verify",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		},
		WriteContextItem{
			Priority:    WriteContextP2,
			Kind:        "failed_test",
			Text:        "TestRepair expected durable handoff",
			SourceStage: "verify",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		},
	)

	got := NormalizeWriteContextPack(pack)
	if len(got.Items) != writeContextPackMaxItems {
		t.Fatalf("normalized pack should remain bounded, got %d", len(got.Items))
	}
	view := got.View(WriteConsumerPlanner, writeContextPackMaxItems)
	if !writeContextViewContains(view, "verify_failure", "verification failed") {
		t.Fatalf("late verify failure should displace ordinary P2 context: %+v", got.Items)
	}
	if !writeContextViewContains(view, "failed_test", "TestRepair") {
		t.Fatalf("late failed test should displace ordinary P2 context: %+v", got.Items)
	}
}

func TestWriteContextPackGovernanceMetadata(t *testing.T) {
	pack := NormalizeWriteContextPack(WriteContextPack{
		PackID:  "governance",
		BatchID: "batch-1",
		Items: []WriteContextItem{{
			Priority:    WriteContextP2,
			Kind:        "verification_diagnostic",
			Text:        "reason_code=python_static_undefined_name path=pkg/a.py",
			SourceStage: "verify",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
			Stale:       true,
			StaleReason: "superseded_by_replan",
		}},
	})
	if len(pack.Items) != 1 {
		t.Fatalf("expected one normalized item, got %+v", pack.Items)
	}
	item := pack.Items[0]
	if item.CostUnits <= 0 || item.Fingerprint == "" {
		t.Fatalf("governance cost/fingerprint missing: %+v", item)
	}
	if item.ExpiresWithBatchID != "batch-1" {
		t.Fatalf("batch expiry metadata missing: %+v", item)
	}
	if item.StaleReason != "superseded_by_replan" {
		t.Fatalf("stale reason not normalized: %+v", item)
	}
	view := pack.ViewForScope(WriteConsumerPlanner, 10, "batch-1", "")
	if writeContextViewContains(view, "verification_diagnostic", "python_static_undefined_name") {
		t.Fatalf("stale non-P0 governance item should not render: %+v", view.Items)
	}
	if view.TotalItems != 1 || view.VisibleItems != 0 || view.CostUnits != 0 {
		t.Fatalf("view governance counters wrong: %+v", view)
	}
}

func TestWriteContextPackBudgetedViewRetainsSafetyAndFailure(t *testing.T) {
	pack := WriteContextPack{
		PackID:  "budgeted",
		BatchID: "batch-1",
		Items: []WriteContextItem{{
			Priority:    WriteContextP0,
			Kind:        "constraint",
			Text:        strings.Repeat("keep safety boundary ", 20),
			SourceStage: "write_analysis",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP3,
			Kind:        "pattern_hint",
			Text:        strings.Repeat("nice style hint ", 20),
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP2,
			Kind:        "verify_failure",
			Text:        "red test failure",
			SourceStage: "verify",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}
	view := pack.ViewForScopeWithBudget(WriteConsumerPlanner, 10, "batch-1", "", 1)
	if !writeContextViewContains(view, "constraint", "keep safety boundary") {
		t.Fatalf("budgeted view should retain P0 safety item: %+v", view.Items)
	}
	if !writeContextViewContains(view, "verify_failure", "red test failure") {
		t.Fatalf("budgeted view should retain planner verify-failure must-carry: %+v", view.Items)
	}
	if writeContextViewContains(view, "pattern_hint", "nice style hint") {
		t.Fatalf("budgeted view should drop low-priority style hint first: %+v", view.Items)
	}
	if !view.Truncated || view.DroppedItems == 0 || view.BudgetUnits != 1 {
		t.Fatalf("budgeted view counters wrong: %+v", view)
	}
}

func TestWriteContextPackViewBoundsAndDefensiveCopy(t *testing.T) {
	pack := WriteContextPack{PackID: "x"}
	for i := 0; i < 20; i++ {
		priority := WriteContextP3
		if i == 10 {
			priority = WriteContextP0
		}
		pack.Items = append(pack.Items, WriteContextItem{
			Priority: priority,
			Kind:     "k",
			Text:     strings.Repeat("x", writeContextPackTextLen+20),
			Consumers: []WriteContextConsumer{
				WriteConsumerPlanner,
				WriteConsumerPlanner,
				"bogus",
			},
		})
	}
	view := pack.View(WriteConsumerPlanner, 3)
	if len(view.Items) != 2 {
		t.Fatalf("dedupe should leave one p0 and one p3 item before limit, got %d: %+v", len(view.Items), view.Items)
	}
	if view.Items[0].Priority != WriteContextP0 {
		t.Fatalf("p0 item should sort first, got %+v", view.Items[0])
	}
	if len(view.Items[0].Text) > writeContextPackTextLen+3 || !strings.HasSuffix(view.Items[0].Text, "...") {
		t.Fatalf("text not trimmed: %d", len(view.Items[0].Text))
	}
	view.Items[0].Text = "mutated"
	again := pack.View(WriteConsumerPlanner, 3)
	if again.Items[0].Text == "mutated" {
		t.Fatalf("view mutation leaked back into pack")
	}
}

func writeContextConsumersContain(consumers []WriteContextConsumer, want WriteContextConsumer) bool {
	for _, c := range consumers {
		if c == want {
			return true
		}
	}
	return false
}

func writeContextViewPriorityCount(view WriteContextView, priority WriteContextPriority) int {
	count := 0
	for _, item := range view.Items {
		if item.Priority == priority {
			count++
		}
	}
	return count
}

func writeContextViewKindCount(view WriteContextView, kind string) int {
	count := 0
	for _, item := range view.Items {
		if item.Kind == kind {
			count++
		}
	}
	return count
}

func writeContextViewContains(view WriteContextView, kind, substring string) bool {
	for _, item := range view.Items {
		if item.Kind == kind && strings.Contains(item.Text, substring) {
			return true
		}
	}
	return false
}
