package orchestrator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This is an operator audit copy, not an execution/verification authority. It
// inventories every retained report version, not just the latest convenient
// source-report snapshot. Missing historical bytes stay missing in the index.
type registrationReportArchiveRow struct {
	Path            string   `json:"path"`
	Bytes           int      `json:"bytes"`
	SHA256          string   `json:"sha256"`
	PlanID          string   `json:"plan_id,omitempty"`
	CanonicalSHA256 string   `json:"canonical_sha256,omitempty"`
	Invocations     []string `json:"invocations,omitempty"`
	DecodeError     string   `json:"decode_error,omitempty"`
}

type registrationReportAttemptAudit struct {
	BatchID           string `json:"batch_id"`
	AttemptID         string `json:"attempt_id"`
	PlanID            string `json:"plan_id"`
	ReportID          string `json:"report_id"`
	Retained          bool   `json:"retained"`
	ExpectedCanonical string `json:"expected_canonical_sha256,omitempty"`
	CanonicalMatch    *bool  `json:"canonical_match,omitempty"`
}

type registrationReportArchive struct {
	Reports  []registrationReportArchiveRow   `json:"reports"`
	Attempts []registrationReportAttemptAudit `json:"attempts"`
}

func archiveRegistrationReports(root, out string, run *types.WriteWorkflowRun) (registrationReportArchive, error) {
	var result registrationReportArchive
	if err := os.Mkdir(out, 0700); err != nil {
		return result, err
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("report audit refuses symlink: %s", path)
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".report.json") {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("report audit requires regular file: %s (%v)", path, err)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(out, rel)
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return err
		}
		if err := writeRegistrationArchiveFile(destination, body); err != nil {
			return err
		}
		after, err := os.ReadFile(path)
		if err != nil || string(body) != string(after) {
			return fmt.Errorf("report changed during audit copy: %s (%v)", path, err)
		}
		row := registrationReportArchiveRow{Path: filepath.ToSlash(rel), Bytes: len(body), SHA256: fmt.Sprintf("%x", sha256.Sum256(body))}
		var report types.ChangeReport
		if err := json.Unmarshal(body, &report); err != nil {
			row.DecodeError = err.Error()
		} else {
			canonical, err := json.Marshal(&report)
			if err != nil {
				return err
			}
			row.PlanID = report.PlanID
			row.CanonicalSHA256 = fmt.Sprintf("%x", sha256.Sum256(canonical))
			for _, command := range report.ExecutedCommands {
				if command.InvocationID != "" {
					row.Invocations = append(row.Invocations, command.InvocationID)
				}
			}
		}
		result.Reports = append(result.Reports, row)
		return nil
	})
	if err != nil {
		return result, err
	}
	byPath := map[string]registrationReportArchiveRow{}
	for _, row := range result.Reports {
		byPath[row.Path] = row
	}
	if run != nil {
		for _, batch := range run.Batches {
			for _, attempt := range batch.Attempts {
				if attempt.Kind != "verify" {
					continue
				}
				row, found := byPath[attempt.ReportID]
				audit := registrationReportAttemptAudit{BatchID: batch.ID, AttemptID: attempt.ID, PlanID: attempt.PlanID, ReportID: attempt.ReportID, Retained: found}
				if attempt.VerificationResults != nil {
					audit.ExpectedCanonical = attempt.VerificationResults.ReportSHA256
					match := found && row.PlanID == attempt.PlanID && row.CanonicalSHA256 == audit.ExpectedCanonical && audit.ExpectedCanonical != ""
					audit.CanonicalMatch = &match
				}
				result.Attempts = append(result.Attempts, audit)
			}
		}
	}
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return result, err
	}
	return result, writeRegistrationArchiveFile(filepath.Join(out, "index.json"), body)
}

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

func preserveRegistrationReportArchive(t *testing.T, root, out string, run *types.WriteWorkflowRun) {
	t.Helper()
	archive, err := archiveRegistrationReports(root, filepath.Join(out, "history-report-artifacts"), run)
	if err != nil {
		t.Errorf("preserve complete report audit: %v", err)
		return
	}
	t.Logf("retained %d report files and %d verify-attempt references; file count is not execution/test count", len(archive.Reports), len(archive.Attempts))
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
	got, err := archiveRegistrationReports(root, out, run)
	if err != nil || len(got.Reports) != 3 || len(got.Attempts) != 2 {
		t.Fatalf("archive = %+v, %v", got, err)
	}
	if !got.Attempts[0].Retained || got.Attempts[0].CanonicalMatch == nil || !*got.Attempts[0].CanonicalMatch || got.Attempts[1].Retained || got.Attempts[1].CanonicalMatch == nil || *got.Attempts[1].CanonicalMatch {
		t.Fatalf("missing reference borrowed current report: %+v", got.Attempts)
	}
	if _, err := archiveRegistrationReports(root, out, run); err == nil {
		t.Fatal("existing audit was overwritten")
	}
	linked := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "old.report.json"), filepath.Join(linked, "link.report.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := archiveRegistrationReports(linked, filepath.Join(t.TempDir(), "refused"), nil); err == nil {
		t.Fatal("symlink was archived as original")
	}
}
