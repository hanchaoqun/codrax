package operation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool"
)

// CommandExecutor runs an already-approved command operation plan. It does not
// classify user intent and does not override policy decisions.
type CommandExecutor struct {
	Policy    CommandPolicy
	OutputDir string
}

func (e CommandExecutor) Execute(ctx context.Context, plan CommandOperationPlan) CommandOperationResult {
	policy := normalizeCommandPolicy(e.Policy)
	if ctx == nil {
		ctx = context.Background()
	}
	result := CommandOperationResult{
		PlanID: plan.ID,
		Status: StatusExecuted,
	}
	if plan.Status != StatusReady {
		result.Status = StatusFailed
		result.StepResults = append(result.StepResults, CommandStepResult{
			Status: StatusFailed,
			Error:  fmt.Sprintf("operation plan is not ready: %s", plan.Status),
		})
		return result
	}
	for _, step := range plan.Steps {
		stepResult := e.executeStep(ctx, plan, step, policy)
		result.StepResults = append(result.StepResults, stepResult)
		if strings.TrimSpace(stepResult.OutputPreview) != "" {
			if result.OutputPreview != "" {
				result.OutputPreview += "\n"
			}
			result.OutputPreview += stepResult.OutputPreview
		}
		if result.PayloadRef == "" && stepResult.PayloadRef != "" {
			result.PayloadRef = stepResult.PayloadRef
		}
		switch stepResult.Status {
		case StatusExecuted:
			continue
		case StatusCancelled:
			result.Status = StatusCancelled
			return result
		default:
			result.Status = StatusFailed
			return result
		}
	}
	return result
}

func (e CommandExecutor) executeStep(ctx context.Context, plan CommandOperationPlan, step CommandStep, policy CommandPolicy) CommandStepResult {
	timeout := time.Duration(step.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Duration(policy.TimeoutMS) * time.Millisecond
	}
	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, display := buildExecCommand(stepCtx, step)
	if cmd == nil {
		return CommandStepResult{
			StepID: step.ID,
			Status: StatusFailed,
			Error:  "command step has neither program nor shell command",
		}
	}
	workDir := strings.TrimSpace(step.WorkDir)
	if workDir == "" {
		workDir = plan.WorkDir
	}
	if workDir != "" {
		cmd.Dir = workDir
	}
	if len(step.Env) > 0 {
		cmd.Env = append(os.Environ(), step.Env...)
	}

	capture, err := newOperationOutputCapture(e.OutputDir, plan.ID, step.ID, policy.OutputPreviewBytes)
	if err != nil {
		return CommandStepResult{
			StepID: step.ID,
			Status: StatusFailed,
			Error:  fmt.Sprintf("prepare output capture: %v", err),
		}
	}
	defer capture.Close()
	writer := io.MultiWriter(capture)
	cmd.Stdout = writer
	cmd.Stderr = writer

	err = cmd.Run()
	preview := strings.TrimRight(capture.Preview(), "\n")
	ref := ""
	if capture.Truncated() {
		ref = capture.Path()
		preview += fmt.Sprintf("\n[operation output truncated; full output saved to %s]", ref)
	}
	if preview == "" {
		preview = fmt.Sprintf("[operation command completed with no output: %s]", display)
	}
	if errors.Is(stepCtx.Err(), context.DeadlineExceeded) {
		return CommandStepResult{
			StepID:        step.ID,
			Status:        StatusFailed,
			OutputPreview: preview,
			PayloadRef:    ref,
			Error:         fmt.Sprintf("command timed out after %s", timeout),
			TimedOut:      true,
		}
	}
	if errors.Is(stepCtx.Err(), context.Canceled) {
		return CommandStepResult{
			StepID:        step.ID,
			Status:        StatusCancelled,
			OutputPreview: preview,
			PayloadRef:    ref,
			Error:         "command cancelled",
		}
	}
	if err != nil {
		return CommandStepResult{
			StepID:        step.ID,
			Status:        StatusFailed,
			ExitCode:      exitCode(err),
			OutputPreview: preview,
			PayloadRef:    ref,
			Error:         err.Error(),
		}
	}
	return CommandStepResult{
		StepID:        step.ID,
		Status:        StatusExecuted,
		OutputPreview: preview,
		PayloadRef:    ref,
	}
}

func buildExecCommand(ctx context.Context, step CommandStep) (*exec.Cmd, string) {
	if strings.TrimSpace(step.Shell) != "" {
		cmd := tool.NewShellCommandContext(ctx, step.Shell)
		return cmd, step.Shell
	}
	if strings.TrimSpace(step.Program) == "" {
		return nil, ""
	}
	cmd := exec.CommandContext(ctx, step.Program, step.Args...)
	display := strings.TrimSpace(step.Program + " " + strings.Join(step.Args, " "))
	return cmd, display
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

type operationOutputCapture struct {
	file      *os.File
	path      string
	buf       []byte
	cap       int
	truncated bool
}

func newOperationOutputCapture(dir, planID, stepID string, capBytes int) (*operationOutputCapture, error) {
	if capBytes <= 0 {
		capBytes = DefaultCommandPolicy().OutputPreviewBytes
	}
	c := &operationOutputCapture{cap: capBytes}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return c, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(planID + ":" + stepID + ":" + time.Now().UTC().Format(time.RFC3339Nano)))
	name := fmt.Sprintf("operation-%s-%s.txt", sanitizeOperationBlobToken(planID), hex.EncodeToString(sum[:4]))
	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	c.file = file
	c.path = path
	return c, nil
}

func (c *operationOutputCapture) Write(p []byte) (int, error) {
	if c.file != nil {
		if _, err := c.file.Write(p); err != nil {
			return 0, err
		}
	}
	remaining := c.cap - len(c.buf)
	if remaining <= 0 {
		c.truncated = true
		return len(p), nil
	}
	if len(p) <= remaining {
		c.buf = append(c.buf, p...)
		return len(p), nil
	}
	c.buf = append(c.buf, p[:remaining]...)
	c.truncated = true
	return len(p), nil
}

func (c *operationOutputCapture) Preview() string { return string(c.buf) }
func (c *operationOutputCapture) Truncated() bool { return c.truncated && c.path != "" }
func (c *operationOutputCapture) Path() string    { return c.path }

func (c *operationOutputCapture) Close() error {
	if c.file == nil {
		return nil
	}
	return c.file.Close()
}

func sanitizeOperationBlobToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "operation"
	}
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "operation"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}
