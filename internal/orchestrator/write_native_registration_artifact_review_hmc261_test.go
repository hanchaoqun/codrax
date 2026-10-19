package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/reportartifact"
	"github.com/hanchaoqun/codrax/internal/types"
)

func reviewHMC261Archive(t *testing.T, root string, run *types.WriteWorkflowRun, capture registrationArtifactCapture) registrationArtifactManifest {
	t.Helper()
	out := filepath.Join(t.TempDir(), "archive")
	got, err := archiveRegistrationArtifacts(root, out, run, capture)
	if err != nil {
		t.Fatalf("archive failed instead of retaining material/issue inventory: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(out, "index.json"))
	if err != nil {
		t.Fatalf("issue manifest not persisted: %v", err)
	}
	var persisted registrationArtifactManifest
	if err := json.Unmarshal(data, &persisted); err != nil || persisted.Complete != got.Complete {
		t.Fatalf("persisted manifest differs: %v %+v", err, persisted)
	}
	return got
}

func reviewHMC261Report(t *testing.T, root, planID string) *reportartifact.Receipt {
	t.Helper()
	receipt, err := reportartifact.PublishSnapshot(root, &types.ChangeReport{PlanID: planID, Passed: true})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestHMC261ReviewAttemptIdentityWithoutCountSnapshot(t *testing.T) {
	root := t.TempDir()
	receipt := reviewHMC261Report(t, root, "source-plan")
	for _, name := range []string{"same_plan", "different_plan"} {
		t.Run(name, func(t *testing.T) {
			planID := receipt.Report.PlanID
			if name == "different_plan" {
				planID = "unrelated-plan"
			}
			run := &types.WriteWorkflowRun{Batches: []types.WriteWorkflowBatch{{ID: "batch", Attempts: []types.WriteWorkflowAttempt{{ID: "attempt", Kind: "verify", PlanID: planID, ReportID: receipt.ReportID}}}}}
			got := reviewHMC261Archive(t, root, run, registrationArtifactCapture{})
			if name == "same_plan" && (!got.Complete || len(got.Attempts) != 1 || !got.Attempts[0].Retained) {
				t.Fatalf("exact valid no-count historical report lost: %+v", got)
			}
			if name == "different_plan" && got.Complete {
				t.Fatalf("different plan borrowed exact report without a count snapshot: %+v", got)
			}
		})
	}
}

func TestHMC261ReviewSourceRootSymlinkSpelling(t *testing.T) {
	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real")
	if err := os.Mkdir(realRoot, 0700); err != nil {
		t.Fatal(err)
	}
	reviewHMC261Report(t, realRoot, "source-plan")
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(realRoot, alias); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	for _, name := range []string{"plain", "trailing_separator"} {
		t.Run(name, func(t *testing.T) {
			root := alias
			if name == "trailing_separator" {
				root += string(filepath.Separator)
			}
			got := reviewHMC261Archive(t, root, nil, registrationArtifactCapture{})
			if got.Complete || len(got.Artifacts) != 0 || len(got.Issues) == 0 {
				t.Fatalf("symlink root was treated as a captured real source: %+v", got)
			}
		})
	}
}

func TestHMC261ReviewMissingRootRetainsFailureManifest(t *testing.T) {
	got := reviewHMC261Archive(t, filepath.Join(t.TempDir(), "missing"), nil, registrationArtifactCapture{RequiredPlanID: "missing-plan"})
	if got.Complete || len(got.Issues) == 0 || len(got.Artifacts) != 0 {
		t.Fatalf("missing root/required final was silently completed: %+v", got)
	}
}

func TestHMC261ReviewExactFilenameDigestCheckedWithoutFinal(t *testing.T) {
	root := t.TempDir()
	receipt := reviewHMC261Report(t, root, "source-plan")
	path := filepath.Join(root, receipt.ReportID)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Keep valid typed content/PlanID while changing bytes behind an exact ID.
	if err := os.WriteFile(path, append(body, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	got := reviewHMC261Archive(t, root, nil, registrationArtifactCapture{})
	if got.Complete || len(got.Artifacts) != 1 || got.Artifacts[0].Error == "" || got.Artifacts[0].ArchivePath == "" {
		t.Fatalf("corrupt original not retained and marked invalid: %+v", got)
	}
}

func TestHMC261ReviewFinalSourcePlanMayDifferFromRegistrationPlan(t *testing.T) {
	root := t.TempDir()
	receipt := reviewHMC261Report(t, root, "source-plan")
	run := &types.WriteWorkflowRun{RunID: "run", ActiveBatchID: "registration", Batches: []types.WriteWorkflowBatch{
		{ID: "source", PlanID: "source-plan", VerifyRef: receipt.ReportID, Attempts: []types.WriteWorkflowAttempt{{ID: "verify", Kind: "verify", PlanID: "source-plan", ReportID: receipt.ReportID}}},
		{ID: "registration", PlanID: "registration-plan"},
	}}
	final := types.BuildWriteFinalReport(types.WriteFinalReportInput{
		Run: run, Plan: &types.ChangePlan{ID: "registration-plan"}, Report: receipt.Report,
		ReportID: &receipt.ReportID, ReportPath: filepath.Join(root, receipt.ReportID),
		Delivery: types.WriteFinalDeliverySummary{FinalPlanID: "registration-plan", ReportPlanID: "source-plan", PrimarySourcePlanID: "source-plan"},
	})
	if err := types.WriteFinalReportToFile(&final, filepath.Join(root, "registration-plan.final.json")); err != nil {
		t.Fatal(err)
	}
	got := reviewHMC261Archive(t, root, run, registrationArtifactCapture{RequiredPlanID: "registration-plan"})
	if !got.Complete || len(got.Links) != 1 || got.Links[0].Status != "matched" || got.Links[0].PlanID != "source-plan" {
		t.Fatalf("valid source/report versus final-plan identity distinction lost: %+v", got)
	}
}
