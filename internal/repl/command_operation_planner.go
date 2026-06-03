package repl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/operation"
)

// CommandOperationPlanner turns a typed route=operation turn into a command
// proposal. It only plans; execution is handled by operation.CommandExecutor
// after deterministic policy evaluation and user approval.
type CommandOperationPlanner interface {
	PlanCommandOperation(ctx context.Context, userLine, repoRoot string, policy TurnPolicy) (operation.CommandOperationRequest, error)
}

type CommandOperationPlannerWithSnapshot interface {
	PlanCommandOperationWithSnapshot(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, snapshot operation.CapabilitySnapshot) (operation.CommandOperationRequest, error)
}

type llmCommandOperationPlanner struct {
	adapter llm.Adapter
}

func NewCommandOperationPlanner(adapter llm.Adapter) CommandOperationPlanner {
	return &llmCommandOperationPlanner{adapter: adapter}
}

var commandOperationPlanTool = llm.ToolSchema{
	Name:        "emit_command_operation_plan",
	Description: "Emit a typed command-operation plan draft. This tool never executes commands.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "status": {
      "type": "string",
      "enum": ["ready", "needs_clarification", "blocked"],
      "description": "ready when the command steps are concrete; needs_clarification when key details are missing; blocked when the request should not be attempted."
    },
    "risk_level": {
      "type": "string",
      "enum": ["none", "low", "medium", "high"],
      "description": "Overall risk before deterministic policy review."
    },
    "work_dir": {
      "type": "string",
      "description": "Working directory for the plan. Use the provided repo root unless the user explicitly asked for another directory."
    },
    "requires_confirmation": {
      "type": "boolean",
      "description": "true for any write, install, uninstall, network, shell-form, unknown command, destructive, or ambiguous action."
    },
    "questions": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "question": {"type": "string"},
          "suggestions": {"type": "array", "items": {"type": "string"}}
        }
      },
      "description": "Questions to ask the user when status=needs_clarification."
    },
    "block_reason": {
      "type": "string",
      "description": "Reason when status=blocked."
    },
    "steps": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "title": {"type": "string"},
          "program": {"type": "string", "description": "Executable name/path. Prefer program+args over shell."},
          "args": {"type": "array", "items": {"type": "string"}},
          "shell": {"type": "string", "description": "Only when a shell pipeline is necessary. Shell-form commands always require manual approval."},
          "work_dir": {"type": "string"},
          "env": {"type": "array", "items": {"type": "string"}},
          "timeout_ms": {"type": "integer"},
          "risk_level": {"type": "string", "enum": ["none", "low", "medium", "high"]},
          "side_effects": {"type": "array", "items": {"type":"string", "enum":["local_file_write","local_file_overwrite","local_file_delete","package_install","package_uninstall","network_read","network_submit","external_system_write","destructive"]}},
          "reason": {"type": "string"},
          "verify_hint": {"type": "string"}
        }
      },
      "description": "Concrete command steps when status=ready."
    }
  },
  "required": ["status", "risk_level", "requires_confirmation"]
}`),
}

const commandOperationPlannerSystemPrompt = `You are Codrax's command-operation planner.

Your job is to convert the user's request into a typed command plan draft. Do not execute commands.

Hard rules:
- If the request lacks key details such as source path, destination path, package name, desired version, target directory, or confirmation scope, emit status=needs_clarification with short questions and suggestions. Do not guess.
- Prefer program+args. Use shell only when a pipeline or shell builtin is necessary; shell always requires confirmation.
- Unknown programs are allowed in a ready plan, but set requires_confirmation=true and risk_level at least medium.
- Batch related commands into a small ordered plan instead of asking approval for each tiny command.
- Do not plan destructive commands unless the user explicitly asked for that destructive action. For obviously catastrophic operations, emit blocked.
- Use the supplied repo root as work_dir by default.
- This is not source-code investigation, trace analysis, or write-mode code editing. Only produce command-operation plans.
- Use capability_snapshot when present. Prefer commands shown as available. If a needed tool is absent, either ask for clarification or plan a safe check/install workflow when the user explicitly asked for installation.
- Do not invent installed tools that are absent from capability_snapshot unless the user explicitly named a custom command or requested installing it.

Risk hints:
- Read-only inspection (pwd, ls, which, version, git status/log/show/diff, grep/rg/cat/head/tail) is low risk.
- Creating a new directory with mkdir -p is low risk when no overwrite/delete is involved.
- Moving, copying, installing packages, downloading, shell pipelines, or unknown commands are medium and require confirmation.
- Deleting, overwriting, uninstalling, external writes, or network submission is high and requires confirmation.
`

func (p *llmCommandOperationPlanner) PlanCommandOperation(ctx context.Context, userLine, repoRoot string, policy TurnPolicy) (operation.CommandOperationRequest, error) {
	return p.PlanCommandOperationWithSnapshot(ctx, userLine, repoRoot, policy, operation.CapabilitySnapshot{})
}

func (p *llmCommandOperationPlanner) PlanCommandOperationWithSnapshot(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, snapshot operation.CapabilitySnapshot) (operation.CommandOperationRequest, error) {
	var zero operation.CommandOperationRequest
	if p == nil || p.adapter == nil {
		return zero, fmt.Errorf("command operation planner not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	userLine = strings.TrimSpace(userLine)
	if userLine == "" {
		return zero, fmt.Errorf("command operation planner: empty user line")
	}
	var b strings.Builder
	b.WriteString("## repo_root\n")
	b.WriteString(strings.TrimSpace(repoRoot))
	b.WriteString("\n\n## route_policy\n")
	b.WriteString(fmt.Sprintf("operation=%s operation_kind=%s risk=%s side_effects=%s target=%s requires_confirmation=%t\n",
		policy.Operation, policy.OperationKind, policy.RiskLevel, strings.Join(policy.SideEffects, ","), policy.TargetSurface, policy.RequiresConfirmation))
	if rendered := snapshot.RenderForPrompt(); strings.TrimSpace(rendered) != "" {
		b.WriteString("\n")
		b.WriteString(rendered)
		b.WriteString("\n")
	}
	b.WriteString("\n## user_request\n")
	b.WriteString(userLine)

	resp, err := p.adapter.Chat(ctx,
		[]llm.Message{
			{Role: "system", Content: commandOperationPlannerSystemPrompt},
			{Role: "user", Content: b.String()},
		},
		[]llm.ToolSchema{commandOperationPlanTool},
		llm.ChatOptions{ToolChoice: "required"},
	)
	if err != nil {
		return zero, fmt.Errorf("command operation planner llm call: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		return zero, fmt.Errorf("command operation planner: LLM returned no tool_call")
	}
	call := resp.ToolCalls[0]
	if call.Name != commandOperationPlanTool.Name {
		return zero, fmt.Errorf("command operation planner: unexpected tool %q", call.Name)
	}
	parsed, err := unmarshalCommandOperationPlan(call.Params)
	if err != nil {
		return zero, err
	}
	return parsed.toRequest(userLine, repoRoot), nil
}

type commandPlanDraft struct {
	Status               string                     `json:"status"`
	RiskLevel            string                     `json:"risk_level"`
	WorkDir              string                     `json:"work_dir"`
	RequiresConfirmation flexiblePolicyBool         `json:"requires_confirmation"`
	Questions            []commandPlanQuestionDraft `json:"questions"`
	BlockReason          string                     `json:"block_reason"`
	Steps                []commandPlanStepDraft     `json:"steps"`
}

type commandPlanQuestionDraft struct {
	ID          string                   `json:"id"`
	Question    string                   `json:"question"`
	Suggestions flexiblePolicyStringList `json:"suggestions"`
}

type commandPlanStepDraft struct {
	ID          string                   `json:"id"`
	Title       string                   `json:"title"`
	Program     string                   `json:"program"`
	Args        flexiblePolicyStringList `json:"args"`
	Shell       string                   `json:"shell"`
	WorkDir     string                   `json:"work_dir"`
	Env         flexiblePolicyStringList `json:"env"`
	TimeoutMS   flexiblePolicyInt        `json:"timeout_ms"`
	RiskLevel   string                   `json:"risk_level"`
	SideEffects flexiblePolicyStringList `json:"side_effects"`
	Reason      string                   `json:"reason"`
	VerifyHint  string                   `json:"verify_hint"`
}

type flexiblePolicyInt int

func (i *flexiblePolicyInt) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*i = 0
		return nil
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		*i = flexiblePolicyInt(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			*i = 0
			return nil
		}
		var parsed int
		if _, err := fmt.Sscanf(s, "%d", &parsed); err == nil {
			*i = flexiblePolicyInt(parsed)
			return nil
		}
	}
	return fmt.Errorf("invalid integer value %s", string(raw))
}

func unmarshalCommandOperationPlan(raw []byte) (commandPlanDraft, error) {
	var parsed commandPlanDraft
	if err := json.Unmarshal(raw, &parsed); err == nil {
		return parsed, nil
	}
	repaired, ok := repairTurnPolicyParamsJSON(raw)
	if !ok {
		return parsed, fmt.Errorf("command operation planner: unmarshal tool params: %w", json.Unmarshal(raw, &parsed))
	}
	if err := json.Unmarshal(repaired, &parsed); err != nil {
		return parsed, fmt.Errorf("command operation planner: unmarshal repaired tool params: %w", err)
	}
	logging.Warning("[repl/operation] emit_command_operation_plan params auto-repaired (LLM-corrupted JSON: structural repair)")
	return parsed, nil
}

func (d commandPlanDraft) toRequest(userLine, repoRoot string) operation.CommandOperationRequest {
	req := operation.CommandOperationRequest{
		Text:                 userLine,
		WorkDir:              plannerFirstNonEmpty(strings.TrimSpace(d.WorkDir), strings.TrimSpace(repoRoot)),
		RiskLevel:            strings.TrimSpace(d.RiskLevel),
		BlockReason:          strings.TrimSpace(d.BlockReason),
		RequiresConfirmation: bool(d.RequiresConfirmation),
	}
	if strings.TrimSpace(d.Status) == string(operation.StatusNeedsClarification) {
		for _, q := range d.Questions {
			if strings.TrimSpace(q.Question) == "" {
				continue
			}
			req.ClarifyingQuestions = append(req.ClarifyingQuestions, operation.ClarifyingQuestion{
				ID:          strings.TrimSpace(q.ID),
				Question:    strings.TrimSpace(q.Question),
				Suggestions: []string(q.Suggestions),
			})
		}
		if len(req.ClarifyingQuestions) == 0 {
			req.ClarifyingQuestions = append(req.ClarifyingQuestions, operation.ClarifyingQuestion{
				ID:       "details",
				Question: "Please provide the missing command target or constraint.",
			})
		}
		return req
	}
	if strings.TrimSpace(d.Status) == string(operation.StatusBlocked) && req.BlockReason == "" {
		req.BlockReason = "planner marked the operation blocked"
		return req
	}
	for _, step := range d.Steps {
		if strings.TrimSpace(step.Program) == "" && strings.TrimSpace(step.Shell) == "" {
			continue
		}
		req.Steps = append(req.Steps, operation.CommandStep{
			ID:          strings.TrimSpace(step.ID),
			Title:       strings.TrimSpace(step.Title),
			Program:     strings.TrimSpace(step.Program),
			Args:        []string(step.Args),
			Shell:       strings.TrimSpace(step.Shell),
			WorkDir:     strings.TrimSpace(step.WorkDir),
			Env:         []string(step.Env),
			TimeoutMS:   int(step.TimeoutMS),
			RiskLevel:   strings.TrimSpace(step.RiskLevel),
			SideEffects: []string(step.SideEffects),
			Reason:      strings.TrimSpace(step.Reason),
			VerifyHint:  strings.TrimSpace(step.VerifyHint),
		})
	}
	return req
}

func plannerFirstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
