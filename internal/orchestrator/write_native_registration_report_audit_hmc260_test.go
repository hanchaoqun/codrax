package orchestrator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func writeRegistrationArchiveFile(path string, body []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(body)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func TestHMC260RegistrationReportArchivePreservesVersionsAndMissingReferences(t *testing.T) {
	root, out := t.TempDir(), filepath.Join(t.TempDir(), "reports")
	report := &types.ChangeReport{PlanID: "same-plan"}
	body, _ := json.Marshal(report)
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	for _, name := range []string{"old.report.json", "new.report.json", "same-plan.report.json"} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	run := &types.WriteWorkflowRun{Batches: []types.WriteWorkflowBatch{{ID: "source", Attempts: []types.WriteWorkflowAttempt{
		{ID: "old", Kind: "verify", PlanID: report.PlanID, ReportID: "old.report.json", VerificationResults: &types.WriteWorkflowVerificationResults{ReportSHA256: digest}},
		{ID: "missing", Kind: "verify", PlanID: report.PlanID, ReportID: "missing.report.json", VerificationResults: &types.WriteWorkflowVerificationResults{ReportSHA256: digest}},
	}}}}
	got, err := archiveRegistrationArtifacts(root, out, run, registrationArtifactCapture{})
	if err != nil || len(got.Artifacts) != 3 || len(got.Attempts) != 2 {
		t.Fatalf("archive = %+v, %v", got, err)
	}
	if !got.Attempts[0].Retained || got.Attempts[0].CanonicalMatch == nil || !*got.Attempts[0].CanonicalMatch || got.Attempts[1].Retained || got.Attempts[1].CanonicalMatch == nil || *got.Attempts[1].CanonicalMatch {
		t.Fatalf("missing reference borrowed current report: %+v", got.Attempts)
	}
	if _, err := archiveRegistrationArtifacts(root, out, run, registrationArtifactCapture{}); err == nil {
		t.Fatal("existing audit was overwritten")
	}
	linked := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "old.report.json"), filepath.Join(linked, "link.report.json")); err != nil {
		t.Fatal(err)
	}
	if got, err := archiveRegistrationArtifacts(linked, filepath.Join(t.TempDir(), "refused"), nil, registrationArtifactCapture{}); err == nil && got.Complete {
		t.Fatal("symlink was archived as original")
	}
}
