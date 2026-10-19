package orchestrator

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

type registrationArchiveProcessInput struct {
	Out      string
	Producer string
	Fail     bool
}

func TestHMC261ArtifactArchiveSurvivesParentAndChildCleanup(t *testing.T) {
	for _, producer := range []string{"parent", "restored_child"} {
		for _, failed := range []bool{false, true} {
			t.Run(producer+"/failed_"+strconv.FormatBool(failed), func(t *testing.T) {
				input := registrationArchiveProcessInput{Out: t.TempDir(), Producer: producer, Fail: failed}
				body, _ := json.Marshal(input)
				exe, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(exe, "-test.run=^TestHMC261ArtifactArchiveProcessHelper$", "-test.v")
				cmd.Env = append(os.Environ(), "CODRAX_HMC261_ARCHIVE_PROCESS="+string(body))
				output, err := cmd.CombinedOutput()
				if (err != nil) != failed {
					t.Fatalf("unexpected helper result: %v\n%s", err, output)
				}
				subdir := "history-report-artifacts"
				if producer == "restored_child" {
					subdir = "restart-child-delivery-artifacts"
				}
				archiveDir := filepath.Join(input.Out, subdir)
				data, err := os.ReadFile(filepath.Join(archiveDir, "index.json"))
				var manifest registrationArtifactManifest
				if err != nil || json.Unmarshal(data, &manifest) != nil {
					t.Fatalf("no durable manifest: %v\n%s", err, output)
				}
				if !manifest.Complete || manifest.Capture.TestFailed != failed || manifest.Capture.Producer != producer || manifest.Capture.PID == os.Getpid() || len(manifest.Links) != 1 || manifest.Links[0].Status != "matched" {
					t.Fatalf("lost process/failure/link facts: %+v\n%s", manifest, output)
				}
				if _, err := os.Stat(manifest.SourceRoot); !os.IsNotExist(err) {
					t.Fatalf("fixture cleanup not exercised: %v", err)
				}
				for _, row := range manifest.Artifacts {
					data, err := os.ReadFile(filepath.Join(archiveDir, row.ArchivePath))
					if err != nil || registrationArchiveDigest(data) != row.SHA256 {
						t.Fatalf("original lost after cleanup: %+v %v", row, err)
					}
					if row.Kind == "write_final_report" {
						var final types.WriteFinalReport
						if json.Unmarshal(data, &final) != nil || final.Verification.Passed == failed {
							t.Fatal("archive completeness changed test failure into pass")
						}
					}
				}
			})
		}
	}
}

// Separate test process exercises real defer + testing.TempDir cleanup without
// any controller, model, grant or source modification. A failure remains a
// failure while its exact final/report bytes survive outside the temporary root.
func TestHMC261ArtifactArchiveProcessHelper(t *testing.T) {
	raw := os.Getenv("CODRAX_HMC261_ARCHIVE_PROCESS")
	if raw == "" {
		t.Skip("subprocess entry only")
	}
	var input registrationArchiveProcessInput
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		t.Fatal(err)
	}
	f := newRegistrationArchiveFixture(t, input.Fail)
	if input.Producer == "parent" {
		defer preserveRegistrationReportArchive(t, f.root, input.Out, f.run, &types.ChangePlan{ID: f.final.Plan.ID})
	} else {
		mu := types.NewMutableState("archive lifecycle fixture")
		mu.SetWriteWorkflowRun(f.run)
		defer preserveRegistrationChildDeliveryArchive(t, registrationRestartInput{WorkDir: filepath.Dir(f.root), AuditDir: input.Out, PlanID: f.final.Plan.ID}, mu)
	}
	if input.Fail {
		t.Error("intentional fixture failure; retain failure artifacts")
	}
}
