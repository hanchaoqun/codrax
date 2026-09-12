package orchestrator

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1575ControllerDoesNotRestoreLegacyPythonAuthority(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "applied", TargetPaths: []string{"widget.py"},
		VerificationProbes: []types.VerificationProbe{{ID: "old", Language: "python", ContractRefs: []string{"value"}, ChangedSymbolRefs: []string{"path:widget.py"}}},
		BehaviorContracts:  []types.WriteBehaviorContract{{ID: "value", Kind: types.WriteBehaviorObservable, Required: true}},
	}
	report := &types.ChangeReport{
		PlanID: plan.ID, Passed: true, VerificationStatus: types.VerificationStatusPassed,
		TestResults: []types.TestResult{{Suite: "verification_probe/python", AssertionID: "old", Passed: true}},
		ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{
			Path: "widget.py", Status: types.ChangedPathVerificationCovered, Caliber: types.ChangedPathVerificationProbe,
			Capability: types.VerificationCapabilityTargetBehavior, Runner: "verification_probe", Source: "old",
			LanguageFamilies: []types.VerificationLanguageFamily{types.VerificationLanguagePython},
		}},
		VerificationConfidence: []types.VerificationConfidenceRecord{
			{Source: "verification_probe", Category: "probe_contract_refs", Status: "satisfied", ContractRefs: []string{"value"}},
			{Source: "verification_probe", Category: "probe_changed_symbol", Status: "satisfied", ChangedSymbolRefs: []string{"path:widget.py"}},
		},
	}
	before, _ := json.Marshal(report)
	for _, p := range []*types.ChangePlan{nil, plan} {
		confidence := verifyCoverageConfidenceFromReportForPlan(p, report)
		if confidence.CoveredContracts["value"] || confidence.CoveredSymbols["path:widget.py"] || len(confidence.TargetBehaviorPaths) != 0 {
			t.Errorf("controller restored stale Python authority: %+v", confidence)
		}
		if !confidence.MissingContracts["value"] || !confidence.MissingChangedSymbol {
			t.Errorf("controller concealed withdrawn evidence: %+v", confidence)
		}
	}
	if len(verificationConfidenceRepairQueueItems(plan, report)) == 0 {
		t.Error("withdrawn observations disappeared from the repair queue")
	}
	after, _ := json.Marshal(report)
	if !bytes.Equal(before, after) {
		t.Fatal("controller rewrote persisted execution history")
	}
}

func TestB1575ControllerKeepsNativeAndSourceTextAuthority(t *testing.T) {
	plan := &types.ChangePlan{ID: "native", TargetPaths: []string{"widget.py"}, BehaviorContracts: []types.WriteBehaviorContract{{ID: "layout", Kind: types.WriteBehaviorFileLayout, Required: true}}}
	report := &types.ChangeReport{
		PlanID: plan.ID, Passed: true,
		ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{
			Path: "widget.py", Status: types.ChangedPathVerificationCovered, Caliber: types.ChangedPathVerificationProjectRunner,
			Capability: types.VerificationCapabilityTargetBehavior, Runner: "python", Source: "native_test",
			LanguageFamilies: []types.VerificationLanguageFamily{types.VerificationLanguagePython},
		}},
		VerificationConfidence: []types.VerificationConfidenceRecord{{Source: "source_text", Category: "source_contract_refs", Status: "satisfied", ContractRefs: []string{"layout"}, WitnessKind: types.WriteBehaviorWitnessSourceText}},
	}
	confidence := verifyCoverageConfidenceFromReportForPlan(plan, report)
	if len(confidence.TargetBehaviorPaths) == 0 || !confidence.CoveredContracts["layout"] {
		t.Fatalf("Python probe limit affected independent native/source-text evidence: %+v", confidence)
	}
}
