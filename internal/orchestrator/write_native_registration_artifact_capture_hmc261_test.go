package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func preserveRegistrationArtifactCapture(t *testing.T, root, out, producer, planID string, run *types.WriteWorkflowRun) {
	t.Helper()
	capture := registrationArtifactCapture{Producer: producer, PID: os.Getpid(), RequiredPlanID: planID, TestFailed: t.Failed()}
	archive, err := archiveRegistrationArtifacts(root, out, run, capture)
	if err != nil {
		t.Errorf("preserve original delivery artifacts: %v", err)
	} else if !archive.Complete && planID != "" {
		t.Errorf("delivery artifact archive incomplete: %v", archive.Issues)
	} else if !archive.Complete {
		// Explicitly rejected recovery has no final-delivery obligation. Keep
		// unavailable historical links visible without changing that negative
		// control's outcome; this inventory is still marked incomplete on disk.
		t.Logf("artifact inventory incomplete (no final delivery required): %v", archive.Issues)
	}
	t.Logf("retained %d typed original artifacts and %d verify references (%s); material count is not execution/test count", len(archive.Artifacts), len(archive.Attempts), producer)
}

func preserveRegistrationReportArchive(t *testing.T, root, out string, run *types.WriteWorkflowRun, plan *types.ChangePlan) {
	t.Helper()
	planID := ""
	if plan != nil {
		planID = plan.ID
	}
	preserveRegistrationArtifactCapture(t, root, filepath.Join(out, "history-report-artifacts"), "parent", planID, run)
}

func preserveRegistrationChildDeliveryArchive(t *testing.T, input registrationRestartInput, mu *types.MutableState) {
	t.Helper()
	out := input.AuditDir
	if out == "" {
		out = input.WorkDir
	}
	planID := input.PlanID
	if input.Reject != "" {
		// Rejected recovery intentionally has no newly executed final report.
		// Preserve whatever exists without manufacturing a terminal requirement.
		planID = ""
	}
	preserveRegistrationArtifactCapture(t, filepath.Join(input.WorkDir, "plans"), filepath.Join(out, "restart-child-delivery-artifacts"), "restored_child", planID, mu.WriteWorkflowRun())
}
