package orchestrator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/immutablefile"
	"github.com/hanchaoqun/codrax/internal/reportartifact"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Test-only material inventory. Complete means the archive/link checks were
// complete, never that tests passed or an execution/permission was authorized.
type registrationArtifactCapture struct {
	Producer       string `json:"producer"`
	PID            int    `json:"pid"`
	RequiredPlanID string `json:"required_final_plan_id,omitempty"`
	TestFailed     bool   `json:"source_test_failed"`
}

type registrationArtifactRow struct {
	Kind            string   `json:"kind"`
	SourcePath      string   `json:"source_path"`
	ArchivePath     string   `json:"archive_path,omitempty"`
	Bytes           int      `json:"bytes"`
	SHA256          string   `json:"sha256,omitempty"`
	PlanID          string   `json:"plan_id,omitempty"`
	RunID           string   `json:"run_id,omitempty"`
	ReportID        string   `json:"report_id,omitempty"`
	CanonicalSHA256 string   `json:"canonical_sha256,omitempty"`
	Invocations     []string `json:"invocations,omitempty"`
	Error           string   `json:"error,omitempty"`
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

type registrationDeliveryLink struct {
	FinalPath       string   `json:"final_archive_path"`
	Fields          []string `json:"fields"`
	PlanID          string   `json:"report_plan_id,omitempty"`
	ReportID        string   `json:"report_id,omitempty"`
	SourcePath      string   `json:"report_source_path,omitempty"`
	ArchivePath     string   `json:"report_archive_path,omitempty"`
	SHA256          string   `json:"report_sha256,omitempty"`
	CanonicalSHA256 string   `json:"report_canonical_sha256,omitempty"`
	Status          string   `json:"status"`
	Error           string   `json:"error,omitempty"`
}

type registrationArtifactManifest struct {
	SchemaVersion int                              `json:"schema_version"`
	Capture       registrationArtifactCapture      `json:"capture"`
	SourceRoot    string                           `json:"source_root"`
	RunID         string                           `json:"run_id,omitempty"`
	ActiveBatchID string                           `json:"active_batch_id,omitempty"`
	Complete      bool                             `json:"archive_complete"`
	Artifacts     []registrationArtifactRow        `json:"artifacts"`
	Attempts      []registrationReportAttemptAudit `json:"attempts,omitempty"`
	Links         []registrationDeliveryLink       `json:"links,omitempty"`
	Issues        []string                         `json:"issues,omitempty"`
}

func registrationArchiveDigest(body []byte) string { return fmt.Sprintf("%x", sha256.Sum256(body)) }

func archiveRegistrationArtifacts(root, out string, run *types.WriteWorkflowRun, capture registrationArtifactCapture) (registrationArtifactManifest, error) {
	root, out = filepath.Clean(root), filepath.Clean(out)
	result := registrationArtifactManifest{SchemaVersion: 1, Capture: capture, SourceRoot: root}
	if !filepath.IsAbs(root) || !filepath.IsAbs(out) {
		return result, fmt.Errorf("artifact archive requires explicit absolute roots")
	}
	if rel, err := filepath.Rel(root, out); err != nil || rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return result, fmt.Errorf("artifact archive destination must be outside source root")
	}
	if run != nil {
		result.RunID, result.ActiveBatchID = run.RunID, run.ActiveBatchID
	}
	if err := os.Mkdir(out, 0700); err != nil {
		return result, err
	}
	payloads := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			result.Issues = append(result.Issues, fmt.Sprintf("read %s: %v", path, walkErr))
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			result.Issues = append(result.Issues, "refused symlink: "+path)
			return nil
		}
		kind := ""
		if strings.HasSuffix(entry.Name(), ".report.json") {
			kind = "change_report"
		} else if strings.HasSuffix(entry.Name(), ".final.json") {
			kind = "write_final_report"
		}
		if entry.IsDir() || kind == "" {
			return nil
		}
		row := registrationArtifactRow{Kind: kind, SourcePath: path}
		body, readErr := immutablefile.Read(filepath.Dir(path), filepath.Base(path))
		if readErr != nil {
			row.Error = readErr.Error()
		} else {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			row.Bytes, row.SHA256 = len(body), registrationArchiveDigest(body)
			row.ArchivePath = filepath.ToSlash(filepath.Join("originals", rel))
			destination := filepath.Join(out, filepath.FromSlash(row.ArchivePath))
			if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
				return err
			}
			if err := writeRegistrationArchiveFile(destination, body); err != nil {
				return err
			}
			payloads[path] = body
			decodeRegistrationArtifact(&row, body)
			if row.Kind == "change_report" && row.Error == "" && reportartifact.IsExactID(row.ReportID) {
				if _, err := reportartifact.LoadExact(filepath.Dir(destination), row.ReportID, row.PlanID); err != nil {
					row.Error = "archived exact identity invalid: " + err.Error()
				}
			}
		}
		if row.Error != "" {
			result.Issues = append(result.Issues, path+": "+row.Error)
		}
		result.Artifacts = append(result.Artifacts, row)
		return nil
	})
	if err != nil {
		result.Issues = append(result.Issues, err.Error())
	}
	validateRegistrationArtifactLinks(&result, payloads, run)
	result.Complete = len(result.Issues) == 0
	body, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr != nil {
		return result, marshalErr
	}
	if writeErr := writeRegistrationArchiveFile(filepath.Join(out, "index.json"), body); writeErr != nil {
		return result, writeErr
	}
	return result, err
}

func decodeRegistrationArtifact(row *registrationArtifactRow, body []byte) {
	if row.Kind == "change_report" {
		var report types.ChangeReport
		if err := json.Unmarshal(body, &report); err != nil || report.PlanID == "" {
			row.Error = fmt.Sprintf("invalid ChangeReport identity: %v", err)
			return
		}
		row.PlanID, row.ReportID = report.PlanID, filepath.Base(row.SourcePath)
		canonical, _ := json.Marshal(&report)
		row.CanonicalSHA256 = registrationArchiveDigest(canonical)
		for _, command := range report.ExecutedCommands {
			if command.InvocationID != "" {
				row.Invocations = append(row.Invocations, command.InvocationID)
			}
		}
		return
	}
	var final types.WriteFinalReport
	if err := json.Unmarshal(body, &final); err != nil || final.SchemaVersion != types.WriteFinalReportSchemaVersion || final.Kind != types.WriteOutputFinalReport || final.Plan.ID == "" {
		row.Error = fmt.Sprintf("invalid WriteFinalReport identity/schema: %v", err)
		return
	}
	row.PlanID, row.RunID, row.ReportID = final.Plan.ID, final.RunID, final.Verification.ReportID
}
