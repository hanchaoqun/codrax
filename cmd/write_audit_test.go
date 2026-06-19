package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestResolveWriteAuditFinalReportPathAcceptsFinalReport(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "plan-audit.final.json")
	writeAuditTestFinalReport(t, finalPath, "run-audit", "plan-audit")

	got, err := resolveWriteAuditFinalReportPath(finalPath)
	if err != nil {
		t.Fatalf("resolveWriteAuditFinalReportPath(final): %v", err)
	}
	if got != finalPath {
		t.Fatalf("resolved final path = %q, want %q", got, finalPath)
	}
}

func TestResolveWriteAuditFinalReportPathFindsPlanIDSibling(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "saved-plan.json")
	plan := writeAuditTestPlan(t, planPath, "plan-from-id")
	finalPath := filepath.Join(dir, plan.ID+".final.json")
	writeAuditTestFinalReport(t, finalPath, "run-from-plan", plan.ID)

	got, err := resolveWriteAuditFinalReportPath(planPath)
	if err != nil {
		t.Fatalf("resolveWriteAuditFinalReportPath(plan): %v", err)
	}
	if got != finalPath {
		t.Fatalf("resolved final path = %q, want %q", got, finalPath)
	}
}

func TestRunWriteAuditCLIOutputsTypedAuditJSON(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "saved-plan.json")
	plan := writeAuditTestPlan(t, planPath, "plan-json")
	finalPath := filepath.Join(dir, plan.ID+".final.json")
	writeAuditTestFinalReport(t, finalPath, "run-json", plan.ID)

	var buf bytes.Buffer
	if err := runWriteAuditCLI(planPath, &buf); err != nil {
		t.Fatalf("runWriteAuditCLI: %v", err)
	}
	var got types.WriteFinalAuditSummary
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("audit output must be JSON: %v\n%s", err, buf.String())
	}
	if got.Kind != types.WriteOutputFinalAudit || got.RunID != "run-json" || got.Plan.ID != plan.ID {
		t.Fatalf("audit summary = %+v", got)
	}
	if got.Loop == nil || got.Loop.Truth.State != types.TruthLedgerCovered || len(got.Loop.EventRefs) != 1 {
		t.Fatalf("audit loop = %+v", got.Loop)
	}
	if len(got.Patch.LanguageFamilies) != 1 || got.Patch.LanguageFamilies[0] != types.VerificationLanguagePython {
		t.Fatalf("audit languages = %+v", got.Patch.LanguageFamilies)
	}
}

func TestResolveWriteAuditFinalReportPathFailsLoudWithoutSibling(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "orphan-plan.json")
	writeAuditTestPlan(t, planPath, "orphan-plan")

	_, err := resolveWriteAuditFinalReportPath(planPath)
	if err == nil {
		t.Fatal("expected missing final report error")
	}
	if !strings.Contains(err.Error(), "write final report not found") {
		t.Fatalf("missing final report error = %v", err)
	}
}

func writeAuditTestPlan(t *testing.T, path, id string) *types.ChangePlan {
	t.Helper()
	plan := &types.ChangePlan{
		ID:      id,
		Request: "test audit",
		Summary: "Test audit plan.",
		Status:  types.PlanStatusApplied,
		Changes: []types.FileChange{{
			Path:       "pkg/fix.py",
			Kind:       "modify",
			NewContent: "print('ok')\n",
			Rationale:  "Exercise audit artifact resolution.",
		}},
		TargetPaths: []string{"pkg/fix.py"},
	}
	if err := types.WritePlanToFile(plan, path); err != nil {
		t.Fatalf("WritePlanToFile: %v", err)
	}
	return plan
}

func writeAuditTestFinalReport(t *testing.T, path, runID, planID string) {
	t.Helper()
	report := types.NormalizeWriteFinalReport(types.WriteFinalReport{
		Kind:      types.WriteOutputFinalReport,
		RunID:     runID,
		RunStatus: types.WriteWorkflowRunComplete,
		Plan: types.WriteFinalPlanSummary{
			ID:     planID,
			Status: types.PlanStatusApplied,
		},
		Patch: types.WriteFinalPatchSummary{
			ChangedFiles:     []string{"pkg/fix.py"},
			LanguageFamilies: []types.VerificationLanguageFamily{types.VerificationLanguagePython},
		},
		Verification: types.WriteFinalVerificationSummary{
			Status:         types.VerificationStatusPassed,
			Passed:         true,
			RunnerFamilies: []types.VerificationLanguageFamily{types.VerificationLanguagePython},
		},
		Loop: &types.WriteFinalLoopSummary{
			RunID:      runID,
			Status:     "complete",
			EventCount: 1,
			EventRefs:  []string{"event-1"},
			Truth: types.TruthLedger{
				State:      types.TruthLedgerCovered,
				ReasonCode: "tests_passed",
			},
		},
	})
	if err := types.WriteFinalReportToFile(&report, path); err != nil {
		t.Fatalf("WriteFinalReportToFile: %v", err)
	}
}
