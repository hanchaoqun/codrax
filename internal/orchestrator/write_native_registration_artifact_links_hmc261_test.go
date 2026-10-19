package orchestrator

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/reportartifact"
	"github.com/hanchaoqun/codrax/internal/types"
)

func validateRegistrationArtifactLinks(result *registrationArtifactManifest, payloads map[string][]byte, run *types.WriteWorkflowRun) {
	byPath := map[string]registrationArtifactRow{}
	for _, row := range result.Artifacts {
		byPath[filepath.Clean(row.SourcePath)] = row
	}
	if run != nil {
		for _, batch := range run.Batches {
			for _, attempt := range batch.Attempts {
				if attempt.Kind != "verify" {
					continue
				}
				row, found := byPath[filepath.Join(result.SourceRoot, attempt.ReportID)]
				audit := registrationReportAttemptAudit{BatchID: batch.ID, AttemptID: attempt.ID, PlanID: attempt.PlanID, ReportID: attempt.ReportID, Retained: found && row.Error == "" && row.Kind == "change_report" && row.PlanID == attempt.PlanID && row.ReportID == attempt.ReportID}
				if attempt.VerificationResults != nil {
					audit.ExpectedCanonical = attempt.VerificationResults.ReportSHA256
					match := audit.Retained && row.PlanID == attempt.PlanID && row.CanonicalSHA256 == audit.ExpectedCanonical && audit.ExpectedCanonical != ""
					audit.CanonicalMatch = &match
				}
				if !audit.Retained || audit.CanonicalMatch != nil && !*audit.CanonicalMatch {
					result.Issues = append(result.Issues, "unavailable attempt report: "+attempt.ID+" / "+attempt.ReportID)
				}
				result.Attempts = append(result.Attempts, audit)
			}
		}
	}
	currentFound := false
	for _, row := range result.Artifacts {
		if row.Kind != "write_final_report" || row.Error != "" {
			continue
		}
		var final types.WriteFinalReport
		if err := json.Unmarshal(payloads[row.SourcePath], &final); err != nil {
			continue // Decode failure is already retained as an artifact issue.
		}
		current := result.Capture.RequiredPlanID != "" && row.SourcePath == filepath.Join(result.SourceRoot, writeWorkflowArtifactFileStem(result.Capture.RequiredPlanID)+".final.json")
		currentFound = currentFound || current
		link := registrationFinalArtifactLink(result.SourceRoot, row, final, byPath)
		if current {
			if final.Plan.ID != result.Capture.RequiredPlanID || run == nil || final.RunID != run.RunID || final.RunStatus != run.Status || final.ActiveBatchID != run.ActiveBatchID {
				link.Error = "current final report does not match captured plan/run state"
			} else if selected, ok := selectedWorkflowReportAttempt(run, link.PlanID); !ok || selected.ReportID != link.ReportID {
				link.Error = "current final report does not match owning batch selection"
			} else if selected.VerificationResults != nil && selected.VerificationResults.ReportSHA256 != "" && selected.VerificationResults.ReportSHA256 != link.CanonicalSHA256 {
				link.Error = "current final report differs from selected attempt digest"
			}
			if link.Status == "unavailable" {
				link.Error = "required final report has no retained exact verification report"
			}
		}
		if link.Error != "" {
			link.Status = "invalid"
			result.Issues = append(result.Issues, row.SourcePath+": "+link.Error)
		}
		result.Links = append(result.Links, link)
	}
	if result.Capture.RequiredPlanID != "" && !currentFound {
		result.Issues = append(result.Issues, "required final report missing or invalid: "+result.Capture.RequiredPlanID)
	}
}

func registrationFinalArtifactLink(root string, row registrationArtifactRow, final types.WriteFinalReport, byPath map[string]registrationArtifactRow) registrationDeliveryLink {
	link := registrationDeliveryLink{FinalPath: row.ArchivePath, Fields: []string{"verification.report_id", "artifacts.report_path"}, PlanID: final.Delivery.ReportPlanID, ReportID: final.Verification.ReportID, SourcePath: final.Artifacts.ReportPath, Status: "unavailable"}
	if link.PlanID == "" {
		link.PlanID = final.Plan.ID
	}
	if link.ReportID == "" && link.SourcePath == "" {
		return link // Honest pre-verification final; never materialized as a PASS.
	}
	if !reportartifact.IsExactID(link.ReportID) || !filepath.IsAbs(link.SourcePath) || filepath.Clean(link.SourcePath) != filepath.Join(root, link.ReportID) {
		link.Error = "final report path/ref is not the exact original in the captured source root"
		return link
	}
	report, found := byPath[filepath.Clean(link.SourcePath)]
	if !found || report.Kind != "change_report" || report.Error != "" || report.PlanID != link.PlanID || report.ReportID != link.ReportID {
		link.Error = "linked original is missing, invalid, or belongs to a different plan"
		return link
	}
	// The exact ID is a complete payload digest, not a current-view alias. Use
	// the frozen row's bytes, never reopen a mutable live path or choose latest.
	wantSuffix := "." + report.SHA256 + ".report.json"
	if len(link.ReportID) <= len(wantSuffix) || link.ReportID[len(link.ReportID)-len(wantSuffix):] != wantSuffix {
		link.Error = "linked original payload does not match its content address"
		return link
	}
	var owner *types.WriteFinalBatchSummary
	for i := range final.Batches {
		batch := &final.Batches[i]
		if batch.PlanID == link.PlanID {
			owner = batch
			if batch.ID == final.ActiveBatchID {
				break
			}
		}
	}
	if owner != nil && owner.VerifyRef != link.ReportID {
		link.Error = fmt.Sprintf("final snapshot owner %s selects another report", owner.ID)
		return link
	}
	link.ArchivePath, link.SHA256, link.CanonicalSHA256 = report.ArchivePath, report.SHA256, report.CanonicalSHA256
	link.Status = "matched"
	return link
}
