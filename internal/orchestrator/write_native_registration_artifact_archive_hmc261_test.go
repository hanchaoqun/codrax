package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/reportartifact"
	"github.com/hanchaoqun/codrax/internal/types"
)

type registrationArchiveFixture struct {
	root, finalPath string
	run             *types.WriteWorkflowRun
	final           types.WriteFinalReport
	old, current    *reportartifact.Receipt
}

func newRegistrationArchiveFixture(t *testing.T, failed bool) registrationArchiveFixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "plans")
	old, err := reportartifact.PublishSnapshot(root, &types.ChangeReport{PlanID: "plan-proof", Passed: false})
	if err != nil {
		t.Fatal(err)
	}
	current, err := reportartifact.PublishSnapshot(root, &types.ChangeReport{PlanID: "plan-proof", Passed: !failed, FailureReasonCode: "current-generation"})
	if err != nil {
		t.Fatal(err)
	}
	run := &types.WriteWorkflowRun{RunID: "run-proof", Status: types.WriteWorkflowRunComplete, ActiveBatchID: "proof", Batches: []types.WriteWorkflowBatch{{ID: "proof", PlanID: "plan-proof", Status: types.WriteWorkflowBatchComplete, VerifyRef: current.ReportID, Attempts: []types.WriteWorkflowAttempt{
		{ID: "old", Kind: "verify", PlanID: "plan-proof", ReportID: old.ReportID, VerificationResults: &types.WriteWorkflowVerificationResults{ReportSHA256: old.CanonicalSHA256}},
		{ID: "current", Kind: "verify", PlanID: "plan-proof", ReportID: current.ReportID, VerificationResults: &types.WriteWorkflowVerificationResults{ReportSHA256: current.CanonicalSHA256}},
	}}}}
	final := types.BuildWriteFinalReport(types.WriteFinalReportInput{Run: run, Plan: &types.ChangePlan{ID: "plan-proof"}, Report: current.Report, ReportID: &current.ReportID, ReportPath: filepath.Join(root, current.ReportID), Delivery: types.WriteFinalDeliverySummary{ReportPlanID: "plan-proof"}})
	f := registrationArchiveFixture{root: root, finalPath: filepath.Join(root, "plan-proof.final.json"), run: run, final: final, old: old, current: current}
	f.saveFinal(t)
	return f
}

func (f registrationArchiveFixture) saveFinal(t *testing.T) {
	t.Helper()
	if err := types.WriteFinalReportToFile(&f.final, f.finalPath); err != nil {
		t.Fatal(err)
	}
}

func TestHMC261ArtifactArchiveRetainsExactBytesAndTypedLinks(t *testing.T) {
	f := newRegistrationArchiveFixture(t, false)
	before, _ := os.ReadFile(f.finalPath)
	out := filepath.Join(t.TempDir(), "archive")
	got, err := archiveRegistrationArtifacts(f.root, out, f.run, registrationArtifactCapture{Producer: "parent", PID: os.Getpid(), RequiredPlanID: f.final.Plan.ID})
	if err != nil || !got.Complete || len(got.Artifacts) != 3 || len(got.Links) != 1 || got.Links[0].Status != "matched" {
		t.Fatalf("archive=%+v err=%v", got, err)
	}
	for _, row := range got.Artifacts {
		source, readErr := os.ReadFile(row.SourcePath)
		copy, copyErr := os.ReadFile(filepath.Join(out, row.ArchivePath))
		if readErr != nil || copyErr != nil || string(copy) != string(source) || len(copy) != row.Bytes || registrationArchiveDigest(copy) != row.SHA256 {
			t.Fatalf("lost exact material: %+v %v %v", row, readErr, copyErr)
		}
	}
	after, _ := os.ReadFile(f.finalPath)
	if string(before) != string(after) || got.Links[0].SourcePath != f.final.Artifacts.ReportPath || got.Links[0].ReportID != f.current.ReportID || got.Links[0].CanonicalSHA256 != f.current.CanonicalSHA256 {
		t.Fatal("archive rewrote original path or substituted report generation")
	}
}

func TestHMC261ArtifactArchiveRecordsMissingAndInvalidMaterial(t *testing.T) {
	for _, mode := range []string{"missing_final", "missing_original", "wrong_generation", "wrong_path", "current_alias", "outside_root", "corrupt_final", "corrupt_original", "wrong_plan", "stale_run", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			f := newRegistrationArchiveFixture(t, false)
			switch mode {
			case "missing_final":
				if err := os.Remove(f.finalPath); err != nil {
					t.Fatal(err)
				}
			case "missing_original":
				if err := os.Remove(filepath.Join(f.root, f.current.ReportID)); err != nil {
					t.Fatal(err)
				}
			case "wrong_generation":
				f.final.Verification.ReportID, f.final.Artifacts.ReportPath = f.old.ReportID, filepath.Join(f.root, f.old.ReportID)
				f.final.Batches[0].VerifyRef = f.old.ReportID
				f.saveFinal(t)
			case "wrong_path":
				f.final.Artifacts.ReportPath = filepath.Join(f.root, f.old.ReportID)
				f.saveFinal(t)
			case "current_alias":
				if err := reportartifact.PublishCurrentView(f.root, "plan-proof.report.json", f.current); err != nil {
					t.Fatal(err)
				}
				f.final.Verification.ReportID, f.final.Artifacts.ReportPath = "plan-proof.report.json", filepath.Join(f.root, "plan-proof.report.json")
				f.saveFinal(t)
			case "outside_root":
				f.final.Artifacts.ReportPath = filepath.Join(t.TempDir(), f.current.ReportID)
				f.saveFinal(t)
			case "corrupt_final":
				if err := os.WriteFile(f.finalPath, []byte("{broken"), 0600); err != nil {
					t.Fatal(err)
				}
			case "corrupt_original":
				if err := os.WriteFile(filepath.Join(f.root, f.current.ReportID), []byte(`{"plan_id":"plan-proof","passed":true}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "wrong_plan":
				f.final.Delivery.ReportPlanID = "another-plan"
				f.saveFinal(t)
			case "stale_run":
				f.final.RunID = "prior-run"
				f.saveFinal(t)
			case "symlink":
				if err := os.Symlink(f.finalPath, filepath.Join(f.root, "alias.final.json")); err != nil {
					t.Fatal(err)
				}
			}
			out := filepath.Join(t.TempDir(), "archive")
			got, err := archiveRegistrationArtifacts(f.root, out, f.run, registrationArtifactCapture{Producer: "restored_child", RequiredPlanID: "plan-proof"})
			if err != nil || got.Complete || len(got.Issues) == 0 {
				t.Fatalf("invalid material became complete: %+v %v", got, err)
			}
			body, err := os.ReadFile(filepath.Join(out, "index.json"))
			var saved registrationArtifactManifest
			if err != nil || json.Unmarshal(body, &saved) != nil || saved.Complete || len(saved.Issues) == 0 {
				t.Fatal("failure evidence was not persisted")
			}
			if mode == "corrupt_final" {
				body, err := os.ReadFile(filepath.Join(out, "originals", "plan-proof.final.json"))
				if err != nil || string(body) != "{broken" {
					t.Fatal("invalid original was erased")
				}
			}
		})
	}
}

func TestHMC261ArtifactArchiveKeepsExplicitReportOwnerAndHistoricalSelection(t *testing.T) {
	f := newRegistrationArchiveFixture(t, false)
	// A delivery plan can cite a distinct report owner. Its explicit typed owner
	// must not be replaced by a same-name or current mutable plan assumption.
	f.final.Plan.ID = "delivery-plan"
	f.finalPath = filepath.Join(f.root, "delivery-plan.final.json")
	f.saveFinal(t)
	got, err := archiveRegistrationArtifacts(f.root, filepath.Join(t.TempDir(), "archive"), f.run, registrationArtifactCapture{RequiredPlanID: "delivery-plan"})
	if err != nil || !got.Complete {
		t.Fatalf("explicit report owner lost: %+v %v", got, err)
	}
	// The parent keeps its earlier valid selected generation even if another
	// immutable original is present. Neither mtime nor the last file wins.
	f.run.Batches[0].VerifyRef = f.old.ReportID
	f.final.Verification.ReportID = f.old.ReportID
	f.final.Artifacts.ReportPath = filepath.Join(f.root, f.old.ReportID)
	f.final.Batches[0].VerifyRef = f.old.ReportID
	f.saveFinal(t)
	// Remove only the unrelated prior final fixture, not any report original.
	if err := os.Remove(filepath.Join(f.root, "plan-proof.final.json")); err != nil {
		t.Fatal(err)
	}
	got, err = archiveRegistrationArtifacts(f.root, filepath.Join(t.TempDir(), "parent"), f.run, registrationArtifactCapture{Producer: "parent", RequiredPlanID: "delivery-plan"})
	if err != nil || !got.Complete || got.Links[0].ReportID != f.old.ReportID {
		t.Fatalf("selected historical generation replaced: %+v %v", got, err)
	}
}

func TestHMC261ArtifactArchiveRequiresSeparateAbsoluteDestination(t *testing.T) {
	f := newRegistrationArchiveFixture(t, false)
	for _, out := range []string{"relative", f.root, filepath.Join(f.root, "nested")} {
		if _, err := archiveRegistrationArtifacts(f.root, out, f.run, registrationArtifactCapture{}); err == nil || !strings.Contains(err.Error(), "archive") {
			t.Fatalf("invalid archive destination accepted: %q %v", out, err)
		}
	}
}
