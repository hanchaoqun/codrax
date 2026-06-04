package repl

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/operation"
)

type fakeCLICommandPlanner struct {
	req operation.CommandOperationRequest
}

func (p fakeCLICommandPlanner) PlanCommandOperation(ctx context.Context, userLine, repoRoot string, policy TurnPolicy) (operation.CommandOperationRequest, error) {
	return p.req, nil
}

func (p fakeCLICommandPlanner) PlanCommandOperationWithSnapshot(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, snapshot operation.CapabilitySnapshot) (operation.CommandOperationRequest, error) {
	return p.req, nil
}

func (p fakeCLICommandPlanner) AnswerCommandOperationRecords(ctx context.Context, userLine string, records []commandOperationResultRecord, lang string) (string, error) {
	if len(records) == 0 {
		return "no records", nil
	}
	return "final: " + strings.TrimSpace(records[len(records)-1].Result.OutputPreview), nil
}

func TestRunCommandOperationCLI_ExecutesAutoPlanAndReturnsFinalAnswer(t *testing.T) {
	t.Parallel()
	policy := operation.DefaultCommandPolicy()
	policy.DefaultWorkDir = t.TempDir()
	req := operation.CommandOperationRequest{
		Text:      "query local environment",
		WorkDir:   policy.DefaultWorkDir,
		RiskLevel: "low",
		Goal:      "query local environment",
		Steps: []operation.CommandStep{{
			ID:        "info",
			Title:     "print fixture",
			Shell:     "printf cli-operation-ok",
			RiskLevel: "low",
		}},
	}
	var progress bytes.Buffer
	answer, err := RunCommandOperationCLI(context.Background(), req.Text, TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "computer_operation",
		OperationKind:        "computer_operation",
		Source:               "current_message",
		RiskLevel:            "low",
	}, CommandOperationCLIConfig{
		Planner:       fakeCLICommandPlanner{req: req},
		Policy:        policy,
		RepoRoot:      policy.DefaultWorkDir,
		RuntimeAnchor: t.TempDir(),
		Language:      "zh",
		Progress:      &progress,
	})
	if err != nil {
		t.Fatalf("RunCommandOperationCLI returned error: %v", err)
	}
	if !strings.Contains(answer, "cli-operation-ok") {
		t.Fatalf("final answer missing command observation:\n%s", answer)
	}
	if out := progress.String(); !strings.Contains(out, "操作计划") || !strings.Contains(out, "cli-operation-ok") {
		t.Fatalf("progress should include plan and execution result, got:\n%s", out)
	}
}
