package orchestrator

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/reportartifact"
	"github.com/hanchaoqun/codrax/internal/types"
)

// writeReportRoots uses only already configured persistence roots. A durable
// workflow's original root precedes a turn-local mirror; no cwd or mtime search.
func (o *Orchestrator) writeReportRoots() []string {
	var roots []string
	if store, ok := o.writeWorkflowRunStore.(interface{ WorkflowDir() string }); ok {
		if dir := strings.TrimSpace(store.WorkflowDir()); filepath.Base(dir) == "workflows" {
			roots = append(roots, filepath.Dir(dir))
		}
	}
	if dir := o.ensureChangeReportDir(); dir != "" && (len(roots) == 0 || filepath.Clean(roots[0]) != filepath.Clean(dir)) {
		roots = append(roots, dir)
	}
	return roots
}

func (o *Orchestrator) publishChangeReportSnapshot(report *types.ChangeReport) *reportartifact.Receipt {
	planID := o.ensureChangeReportPlanID(report)
	if report == nil || planID == "" {
		return nil
	}
	report.PlanID = planID
	if report.GeneratedAt.IsZero() {
		report.GeneratedAt = time.Now()
	}
	report.EnsureVerificationStatus()
	var receipt *reportartifact.Receipt
	for _, root := range o.writeReportRoots() {
		published, err := reportartifact.PublishSnapshot(root, report)
		if err != nil {
			logging.Warning("[orchestrator] immutable ChangeReport publication failed: %v", err)
			return nil
		}
		receipt = published
	}
	return receipt
}

func (o *Orchestrator) publishChangeReportCurrentView(receipt *reportartifact.Receipt) {
	if receipt == nil || receipt.Report == nil {
		return
	}
	legacyID := writeWorkflowReportID(receipt.Report)
	for _, root := range o.writeReportRoots() {
		if err := reportartifact.PublishCurrentView(root, legacyID, receipt); err != nil {
			logging.Warning("[orchestrator] ChangeReport compatibility view unavailable (original retained): %v", err)
		} else {
			logging.Info("[orchestrator] ChangeReport saved: %s (original=%s)", filepath.Join(root, legacyID), receipt.ReportID)
		}
	}
}

func isExactReportRef(ref string) bool { return reportartifact.IsExactID(ref) }

func (o *Orchestrator) publishWorkflowReportCurrentViews(run *types.WriteWorkflowRun) {
	seen := map[string]bool{}
	for i := len(run.Batches) - 1; i >= 0; i-- {
		for j := len(run.Batches[i].Attempts) - 1; j >= 0; j-- {
			a := run.Batches[i].Attempts[j]
			if a.Kind != "verify" || seen[a.PlanID] {
				continue
			}
			seen[a.PlanID] = true
			if selected, ok := selectedWorkflowReportAttempt(run, a.PlanID); ok {
				a = selected
			} else {
				continue
			}
			if !reportartifact.IsExactID(a.ReportID) {
				continue
			}
			for _, root := range o.writeReportRoots() {
				if receipt, err := reportartifact.LoadExact(root, a.ReportID, a.PlanID); err == nil {
					o.publishChangeReportCurrentView(receipt)
					break
				}
			}
		}
	}
}

func latestBatchReportAttempt(batch types.WriteWorkflowBatch, reportID, planID string) types.WriteWorkflowAttempt {
	for i := len(batch.Attempts) - 1; i >= 0; i-- {
		a := batch.Attempts[i]
		if a.Kind == "verify" && a.ReportID == reportID && (a.PlanID == planID || a.PlanID == "") {
			a.PlanID = planID
			return a
		}
	}
	return types.WriteWorkflowAttempt{PlanID: planID, ReportID: reportID}
}

// Publication and count binding use one frozen snapshot. A failed save does not
// invent a ref, reuse an earlier success, change test status, or create an extra
// execution. The normal sticky persistence warning exposes the evidence gap.
func (o *Orchestrator) recordWorkflowVerification(run *types.WriteWorkflowRun, report *types.ChangeReport, status, reason string, observeSlice bool) {
	if run == nil || report == nil {
		return
	}
	ref := ""
	if receipt := o.publishChangeReportSnapshot(report); receipt != nil {
		report, ref = receipt.Report, receipt.ReportID
	} else {
		run.PersistenceDegraded = true
		run.PersistenceDegradedReason = types.WriteWorkflowPersistenceDegradedReportSnapshot
		appendControllerProgress(run, run.ActiveBatchID, "report_snapshot_unavailable", "verification result remains in memory; its complete report could not be retained")
	}
	updateWorkflowRunBatchVerify(run, run.ActiveBatchID, report, status, reason, ref)
	if observeSlice {
		updateWorkflowRunActiveSliceObserve(run, run.ActiveBatchID, report, status, reason, ref)
	}
}

func reportAttemptCanonicalSHA(attempt types.WriteWorkflowAttempt) string {
	s := attempt.VerificationResults
	if s == nil || s.AttemptID != attempt.ID || s.PlanID != attempt.PlanID || s.ReportID != attempt.ReportID {
		return ""
	}
	return s.ReportSHA256
}

// The summary digest checks byte lineage only. It never grants permission or
// upgrades the report's original verification/contract capabilities.
func (o *Orchestrator) loadWorkflowAttemptReport(attempt types.WriteWorkflowAttempt) (*types.ChangeReport, string) {
	receipt, reason := o.loadWorkflowAttemptReceipt(attempt)
	if receipt == nil {
		return nil, reason
	}
	return receipt.Report, reason
}

func (o *Orchestrator) loadWorkflowAttemptReceipt(attempt types.WriteWorkflowAttempt) (*reportartifact.Receipt, string) {
	ref, planID := strings.TrimSpace(attempt.ReportID), strings.TrimSpace(attempt.PlanID)
	if ref == "" {
		return nil, "durable_report_ref_missing"
	}
	digest := reportAttemptCanonicalSHA(attempt)
	if !reportartifact.IsExactID(ref) && digest == "" {
		return nil, "durable_legacy_history_unverified"
	}
	for _, root := range o.writeReportRoots() {
		var receipt *reportartifact.Receipt
		var err error
		if reportartifact.IsExactID(ref) {
			receipt, err = reportartifact.LoadExact(root, ref, planID)
		} else {
			receipt, err = reportartifact.LoadLegacy(root, ref, planID, digest)
		}
		if err == nil && receipt != nil && (digest == "" || receipt.CanonicalSHA256 == digest) {
			return receipt, ""
		}
	}
	return nil, "durable_report_unavailable"
}

func workflowReportOwningBatch(run *types.WriteWorkflowRun, planID string) *types.WriteWorkflowBatch {
	if run == nil {
		return nil
	}
	// Current selection belongs to a batch. Active batch wins when it owns
	// this plan; otherwise use its last owning batch, never global file time.
	var owner *types.WriteWorkflowBatch
	for i := range run.Batches {
		if strings.TrimSpace(run.Batches[i].PlanID) == strings.TrimSpace(planID) {
			owner = &run.Batches[i]
			if owner.ID == run.ActiveBatchID {
				break
			}
		}
	}
	return owner
}

func selectedWorkflowReportAttempt(run *types.WriteWorkflowRun, planID string) (types.WriteWorkflowAttempt, bool) {
	owner := workflowReportOwningBatch(run, planID)
	if owner != nil {
		batch := *owner
		ref := strings.TrimSpace(batch.VerifyRef)
		if ref != "" {
			for j := len(batch.Attempts) - 1; j >= 0; j-- {
				a := batch.Attempts[j]
				if a.Kind == "verify" && a.PlanID == planID && a.ReportID == ref {
					return a, true
				}
			}
			if reportartifact.IsExactID(ref) || len(batch.Attempts) > 0 {
				return types.WriteWorkflowAttempt{PlanID: planID, ReportID: ref}, true
			}
			return types.WriteWorkflowAttempt{}, false // legacy current view only
		}
		for j := len(batch.Attempts) - 1; j >= 0; j-- {
			a := batch.Attempts[j]
			if a.Kind == "verify" && strings.TrimSpace(a.PlanID) == strings.TrimSpace(planID) {
				return a, true
			}
		}
	}
	return types.WriteWorkflowAttempt{}, false
}

func (o *Orchestrator) workflowReportSource(run *types.WriteWorkflowRun, planID string, current *types.ChangeReport) (*types.ChangeReport, string) {
	if attempt, ok := selectedWorkflowReportAttempt(run, planID); ok {
		receipt, _ := o.loadWorkflowAttemptReceipt(attempt)
		if receipt == nil {
			return nil, attempt.ReportID
		}
		if reportartifact.ReceiptMatchesReport(receipt, current) {
			return current, attempt.ReportID // retain transient proof restrictions
		}
		return receipt.Report, attempt.ReportID
	}
	// Legacy/no-attempt callers keep current-view behavior, never historical
	// evidence. The actual in-memory current report can be retained for display.
	if current != nil && strings.TrimSpace(current.PlanID) == strings.TrimSpace(planID) {
		if receipt := o.publishChangeReportSnapshot(current); receipt != nil {
			return current, receipt.ReportID
		}
		return current, ""
	}
	return o.loadLegacyCurrentReport(planID), writeWorkflowArtifactFileStem(planID) + ".report.json"
}

func (o *Orchestrator) loadLegacyCurrentReport(planID string) *types.ChangeReport {
	if strings.TrimSpace(planID) == "" {
		return nil
	}
	ref := writeWorkflowArtifactFileStem(planID) + ".report.json"
	for _, root := range o.writeReportRoots() {
		if receipt, err := reportartifact.LoadLegacy(root, ref, planID, ""); err == nil {
			return receipt.Report
		}
	}
	return nil
}

func (o *Orchestrator) reportArtifactPath(ref, planID string, expected *types.ChangeReport) string {
	if ref == "" || expected == nil {
		return ""
	}
	for _, root := range o.writeReportRoots() {
		var receipt *reportartifact.Receipt
		var err error
		if reportartifact.IsExactID(ref) {
			receipt, err = reportartifact.LoadExact(root, ref, planID)
		} else {
			receipt, err = reportartifact.LoadLegacy(root, ref, planID, "")
		}
		if err != nil || !reportartifact.ReceiptMatchesReport(receipt, expected) {
			continue
		}
		return filepath.Join(root, ref)
	}
	return ""
}
