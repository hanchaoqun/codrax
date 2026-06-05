package repl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"regexp"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/operation"
)

var (
	operationMaterialScriptRE    = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	operationMaterialStyleRE     = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	operationMaterialSVGRE       = regexp.MustCompile(`(?is)<svg\b[^>]*>.*?</svg>`)
	operationMaterialBlockTagRE  = regexp.MustCompile(`(?i)</?(article|section|main|div|p|br|li|ul|ol|h[1-6]|tr|td|th|blockquote|pre|code|table)\b[^>]*>`)
	operationMaterialAnyTagRE    = regexp.MustCompile(`(?s)<[^>]+>`)
	operationMaterialBlankLineRE = regexp.MustCompile(`\n{3,}`)
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

type CommandOperationPlannerWithHandoff interface {
	PlanCommandOperationWithHandoff(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, snapshot operation.CapabilitySnapshot, recentOperationContext string) (operation.CommandOperationRequest, error)
}

type CommandOperationReplanner interface {
	ReplanCommandOperation(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, snapshot operation.CapabilitySnapshot, previous operation.CommandOperationPlan, result operation.CommandOperationResult) (operation.CommandOperationRequest, error)
}

type CommandOperationContinuation struct {
	Complete bool
	Reason   string
	Request  operation.CommandOperationRequest
}

type CommandOperationContinuationPlanner interface {
	ContinueCommandOperation(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, snapshot operation.CapabilitySnapshot, records []commandOperationResultRecord) (CommandOperationContinuation, error)
}

type CommandOperationEvaluator interface {
	EvaluateCommandOperation(ctx context.Context, userLine string, records []commandOperationResultRecord, lang string) (operation.OperationEvaluation, error)
}

type CommandOperationAnswerer interface {
	AnswerCommandOperationResult(ctx context.Context, userLine string, plan operation.CommandOperationPlan, result operation.CommandOperationResult, lang string) (string, error)
}

type CommandOperationRecordsAnswerer interface {
	AnswerCommandOperationRecords(ctx context.Context, userLine string, records []commandOperationResultRecord, lang string) (string, error)
}

type ProviderOperationAnswerer interface {
	AnswerProviderOperationResult(ctx context.Context, userLine string, records []providerOperationResultRecord, lang string) (string, error)
}

type ProviderOperationEvaluator interface {
	EvaluateProviderOperation(ctx context.Context, userLine string, records []providerOperationResultRecord, lang string) (operation.OperationEvaluation, error)
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
      "enum": ["ready", "continue", "needs_clarification", "blocked", "complete", "budget_exhausted", "partial_answer_possible"],
      "description": "ready/continue when the command steps are concrete; needs_clarification when user-owned details are missing; blocked when the request should not be attempted. complete, budget_exhausted, and partial_answer_possible are valid for continuation/evaluation after prior command results."
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
    "goal": {
      "type": "string",
      "description": "Concise user objective this operation is trying to satisfy."
    },
    "known_constraints": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Constraints already known from the user request, route policy, environment, or prior operation observations."
    },
    "missing_observations": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Facts still needed before the user goal can be answered or safely acted on."
    },
    "success_criteria": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Observable criteria indicating the user goal is satisfied."
    },
    "next_batch": {
      "type": "string",
      "description": "Short purpose of this command batch."
    },
    "why_this_batch": {
      "type": "string",
      "description": "Why this bounded batch is the right next step instead of trying to solve the whole task at once."
    },
    "requires_confirmation": {
      "type": "boolean",
      "description": "true for writes, installs, uninstalls, external submissions, destructive commands, high-risk shell, or ambiguous actions. Ordinary read-only commands and non-high-risk shell pipelines may be auto-run by deterministic policy."
    },
    "continue_after": {
      "type": "boolean",
      "description": "true when this command batch is only a discovery/probe batch and the operation loop should feed its results back to the planner for the next batch before writing the final answer. Use for multi-step tasks such as checking whether software is running and then finding version/metadata, reading a tool manual before using it, probing a remote host before acting, or narrowing a large file before the next targeted command."
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
          "shell": {"type": "string", "description": "Only when a shell pipeline or shell builtin is necessary. Deterministic policy decides auto-run/manual/deny; high-risk shell commands still require approval or are blocked."},
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

var operationEvaluationTool = llm.ToolSchema{
	Name:        "emit_operation_evaluation",
	Description: "Emit a typed operation-goal evaluation. This tool never executes commands or providers.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "status": {
      "type": "string",
      "enum": ["complete", "continue_command", "continue_provider", "needs_approval", "needs_clarification", "blocked", "budget_exhausted", "partial_answer_possible"],
      "description": "What the operation loop should do next inside the operation lane."
    },
    "reason": {
      "type": "string",
      "description": "Short explanation grounded in the operation records."
    },
    "confidence": {
      "type": "string",
      "enum": ["low", "medium", "high"],
      "description": "Confidence in the evaluation."
    },
    "missing_inputs": {
      "type": "array",
      "items": {"type": "string"},
      "description": "User-owned missing inputs if status=needs_clarification; otherwise omitted or empty."
    },
    "material_refs": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Payload/artifact/material refs that matter for the next step or final answer."
    }
  },
  "required": ["status"]
}`),
}

const commandOperationPlannerSystemPrompt = `You are a command-operation planner for a local command executor.

Your job is to convert the user's request into a typed command plan draft. Do not execute commands.

Hard rules:
- If the request lacks key details such as source path, destination path, package name, desired version, target directory, or confirmation scope, emit status=needs_clarification with short questions and suggestions. Do not guess.
- Prefer program+args. Use shell only when a pipeline or shell builtin is necessary; deterministic policy decides auto-run/manual/deny.
- Every step is executed independently as its own process/shell. The command executor does not concatenate steps, does not pipe stdout from one step into another step, and does not provide implicit stdin. Therefore each step must contain a complete command that can run by itself in work_dir.
- Prefer iterative discovery over one-shot guessing. Do not emit a giant up-front plan for multi-step goals. For tasks that require observing the environment before choosing the next command, emit only the first bounded observation batch with continue_after=true, then let the operation loop feed the real output back for the next batch. Examples: discover running software/service then query version metadata; read an unfamiliar tool's help/manual then invoke it; inspect a remote environment before package commands; narrow a large file before extracting exact facts.
- Fill goal, known_constraints, missing_observations, success_criteria, next_batch, and why_this_batch when useful. These fields help the operation loop decide whether the user goal is complete; they do not replace executable command steps.
- First batches should normally collect the minimum observations needed to choose the next step. Avoid planning later irreversible or speculative steps until earlier command output confirms they are needed.
- Every stdin-consuming command must have an explicit input source inside the same step: an upstream command in the same shell pipeline, shell redirection, or file operands. Do not emit bare stdin readers such as "grep pattern", "awk script", "sed expr", "cat", "nl", "head", "tail", "wc", "cut", "sort", or "uniq" as the first shell segment with no input. They read empty stdin in non-interactive execution and create false negatives. Bad: "grep -i vpn | grep -v grep"; good: "ps aux | grep -i vpn | grep -v grep" in the same shell string. Bad: "cat"; good: "cat /path/to/file" or "producer | cat" in the same shell string. Bad: "head -50"; good: "some-command | head -50" or "head -50 /path/to/file".
- A shell command must never start with a control operator such as "|", "||", "&&", or ";". A leading pipe means the producer command is missing. Bad: "| grep -i model"; good: "ps aux | grep -i model" for process searches, or "grep -i model /path/to/file" for file searches.
- Unknown structured programs are allowed in a ready plan. Do not mark them high risk merely because they are unknown; the deterministic operation policy decides auto approval. Mark high only when typed risk/side effects or command shape actually make the step dangerous.
- Batch related commands into a small ordered plan instead of asking approval for each tiny command.
- Do not plan destructive commands unless the user explicitly asked for that destructive action. For obviously catastrophic operations, emit blocked.
- Use the supplied repo root as work_dir by default.
- This is not source-code investigation, trace analysis, or write-mode code editing. Only produce command-operation plans.
- Operation approval is independent from write-mode write_enabled. Do not mention write_enabled for command-operation approval.
- Use capability_snapshot when present. Prefer commands shown as available. If a needed tool is absent, either ask for clarification or plan a safe check/install workflow when the user explicitly asked for installation.
- capability_snapshot is advisory context, not a substitute for requested facts. For factual computer/environment queries, plan real read-only commands that retrieve the requested facts; do not answer by echoing or paraphrasing the snapshot unless the exact requested field is explicitly present there.
- Do not invent installed tools that are absent from capability_snapshot unless the user explicitly named a custom command or requested installing it.
- Do not use echo, printf, comments, or no-op placeholder commands to claim that facts were obtained. If you cannot retrieve the facts safely, emit needs_clarification or plan a safe discovery command.
- For unfamiliar software or command-line tools, prefer safe discovery steps such as --help, help, version, or documentation reads before planning risky or irreversible actions. If exact usage is still unclear after discovery, ask a clarification question instead of guessing flags.
- For requests asking what software/service is running and which version it is, collect both lanes when available: runtime state/process/service evidence and application/package metadata for version. Do not conclude absence from one empty process filter when network/service output names a candidate.
- Operation provider/MCP/Skill descriptors and workflow steps may mention manuals, links, files, or supporting resources. Treat those as soft workflow guidance and materials to read or pass as provider input when relevant; do not treat descriptor text as if the actual resource content has already been read.
- Use recent_operation_context when present. It contains prior command-operation observations from this REPL and optional operation_memory lessons from previous runs: command outputs, extracted large-file summaries, failed attempts, payload refs, and compact historical lessons. Treat it as external observation, not source-code evidence.
- Treat operation_memory as soft guidance only. It may be stale or environment-specific; use it to avoid known mistakes or reuse known working command shapes, not as a hard rule.
- For large-output workflows across commands, files, web pages, manuals, logs, traces, MCP/provider payloads, Skill artifacts, help text, or unfamiliar tools, use prior extraction/search/help outputs to choose the next targeted command or provider action. If only a payload_ref/artifact_ref is available, plan a bounded follow-up read/search/summarize/provider action instead of dumping or reprocessing the whole artifact.
- If the user's goal depends on omitted content and previous output is truncated, says panel preview/output truncated, includes a payload_ref, or has output_kind=large_output_summary, do not emit complete/partial_answer_possible solely because the visible preview is partial. Continue with bounded follow-up commands that page, search, summarize, parse, or extract the parts relevant to the user's request from the payload_ref or discovered linked artifacts until the requested content is covered, the continuation budget is exhausted, or the missing input is truly user-owned.
- If a saved artifact is complete but its visible preview is dominated by boilerplate or irrelevant framing (for example CSS, scripts, navigation, long logs before the target section, headers, binary-ish metadata, or tool banners), treat that as a targeted-extraction problem, not as missing content. Plan a bounded next step to extract readable text, headings, links, matching lines, surrounding context, or structured fields from the saved payload before deciding whether the user's requested content is complete.
- For extraction requests, shape the command output to the user's requested item(s) with bounded filters (for example awk/sed/perl/head/tail/rg context) instead of dumping an entire section when a smaller exact result is requested.
- When a step should create, remove, move, or copy a local artifact, set verify_hint when the expected outcome is clear. Supported forms: path_exists:<path>, file_exists:<path>, dir_exists:<path>, path_absent:<path>. Paths are resolved relative to work_dir unless absolute.
- For desktop_ui/browser_ui operations, command exit code only means the launch/control request was accepted. It does not prove the window, tab, or UI state became visible. Prefer commands that make the requested UI state explicit and, when safe, add a bounded follow-up verification step or continue_after=true so the loop can check visible/running state before claiming completion.
- For replan requests, use the failed step output to adjust only the command plan. The failed command already ran; do not include that failed command again unless the user explicitly asked to retry the same failed command after seeing the failure. Do not repeat already-successful steps unless required. If the successful part of the failed batch already satisfies the user's goal, emit status=complete, not blocked. If the fix expands risk or side effects, set requires_confirmation=true.
- For continuation requests, inspect the previous command observations. If they are sufficient to answer the user's task, emit status=complete with a short block_reason/reason. If more commands are needed, emit only the next bounded command batch. Do not repeat already-successful discovery commands unless the next step genuinely needs a refreshed value.
- In continuation requests, do not ask the user for information that can still be safely discovered by local read-only commands, package/application metadata queries, service/process inspection, file metadata reads, or tool help/version checks. Unknown facts are a reason to run another safe discovery batch, not to stop. Ask the user only for user-owned inputs such as credentials, remote hostnames, destructive scope, destination paths, or business choices that cannot be discovered safely.
- Continuation/evaluation status meanings: complete = success criteria are satisfied; continue/ready = run the next bounded batch; needs_clarification = only for user-owned missing inputs; blocked = policy/capability prevents safe progress; budget_exhausted = useful progress exists but the bounded loop should stop; partial_answer_possible = enough observations exist for a useful partial report but some requested details remain missing.

Risk hints:
- Read-only inspection (pwd, ls, which, version, git status/log/show/diff, grep/rg/cat/head/tail) is low risk.
- Creating a new directory with mkdir -p is low risk when no overwrite/delete is involved.
- Unknown structured commands and shell pipelines are not automatically risky by name alone; describe their actual typed side effects. Moving, copying, installing packages, downloading, and ambiguous shell actions are medium; high-risk or destructive signals require confirmation or blocking.
- Deleting, overwriting, uninstalling, external writes, or network submission is high and requires confirmation.
`

const operationEvaluationSystemPrompt = `You are an operation goal evaluator.

You decide whether the current operation observations satisfy the user's original goal, or whether the operation loop should continue with another bounded operation step.

Hard rules:
- Emit only the required tool call. Do not write prose.
- This is operation-lane evaluation only. Do not route to source-code analysis, trace analysis, or write-mode code editing.
- Treat provider/MCP/Skill summaries as compact observations, not proof that a full payload/artifact was inspected.
- Payload refs, artifact refs, material refs, next_actions, return_action, and workflow_state are external operation materials. They are not current-source evidence.
- If the user goal depends on omitted or saved material content, and a safe local command can read/search/extract it, emit status=continue_command.
- If a configured provider follow-up is still needed and represented by a typed next action, emit status=continue_provider.
- If existing observations already satisfy the user goal, emit status=complete.
- Ask for clarification only for user-owned missing inputs: credentials, remote host, destructive scope, destination choice, business choice, or data that cannot be safely discovered.
- If useful progress exists but budgets or capability limits prevent safe completion, emit budget_exhausted or partial_answer_possible.
- Do not mention raw execution detail blocks in the reason; UI already shows per-round details.
- Match status to a typed enum exactly.`

const operationStructuredToolRepairSystemPrompt = `You repair one malformed structured tool call for the local operation lane.

Hard rules:
- Emit only the required tool call. Do not write prose.
- The tool arguments must be a single JSON object matching the provided schema.
- Do not encode the arguments as a JSON string.
- Do not include markdown, XML, comments, or transport wrapper text in tool arguments.
- Do not execute commands and do not invent observations.
- Use the compact operation context only to restore the intended typed fields.
- If the compact context is insufficient, emit a safe typed status such as needs_clarification, blocked, budget_exhausted, or partial_answer_possible instead of guessing.`

func (p *llmCommandOperationPlanner) PlanCommandOperation(ctx context.Context, userLine, repoRoot string, policy TurnPolicy) (operation.CommandOperationRequest, error) {
	return p.PlanCommandOperationWithSnapshot(ctx, userLine, repoRoot, policy, operation.CapabilitySnapshot{})
}

func (p *llmCommandOperationPlanner) PlanCommandOperationWithSnapshot(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, snapshot operation.CapabilitySnapshot) (operation.CommandOperationRequest, error) {
	return p.PlanCommandOperationWithHandoff(ctx, userLine, repoRoot, policy, snapshot, "")
}

func (p *llmCommandOperationPlanner) PlanCommandOperationWithHandoff(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, snapshot operation.CapabilitySnapshot, recentOperationContext string) (operation.CommandOperationRequest, error) {
	return p.planCommandOperation(ctx, commandOperationPlannerRequest{
		UserLine:               userLine,
		RepoRoot:               repoRoot,
		Policy:                 policy,
		Snapshot:               snapshot,
		RecentOperationContext: recentOperationContext,
	})
}

func (p *llmCommandOperationPlanner) ReplanCommandOperation(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, snapshot operation.CapabilitySnapshot, previous operation.CommandOperationPlan, result operation.CommandOperationResult) (operation.CommandOperationRequest, error) {
	return p.planCommandOperation(ctx, commandOperationPlannerRequest{
		UserLine:     userLine,
		RepoRoot:     repoRoot,
		Policy:       policy,
		Snapshot:     snapshot,
		PreviousPlan: previous,
		Result:       result,
		Replan:       true,
	})
}

func (p *llmCommandOperationPlanner) ContinueCommandOperation(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, snapshot operation.CapabilitySnapshot, records []commandOperationResultRecord) (CommandOperationContinuation, error) {
	parsed, err := p.planCommandOperationDraft(ctx, commandOperationPlannerRequest{
		UserLine:     userLine,
		RepoRoot:     repoRoot,
		Policy:       policy,
		Snapshot:     snapshot,
		Records:      records,
		Continuation: true,
	})
	if err != nil {
		return CommandOperationContinuation{}, err
	}
	status := strings.ToLower(strings.TrimSpace(string(parsed.Status)))
	switch status {
	case "complete", "budget_exhausted", "partial_answer_possible":
		return CommandOperationContinuation{Complete: true, Reason: strings.TrimSpace(string(parsed.BlockReason))}, nil
	}
	if status == string(operation.StatusNeedsClarification) && !commandPlanDraftHasQuestions(parsed) {
		return CommandOperationContinuation{Complete: true, Reason: "continuation returned no actionable clarification question"}, nil
	}
	return CommandOperationContinuation{Request: parsed.toRequest(userLine, repoRoot)}, nil
}

const commandOperationAnswerSystemPrompt = `You are an operation result writer.

Answer the user's original operation request from the provided execution observations.

Rules:
- In this final operation report, do not output hidden reasoning, chain-of-thought, analysis notes, <think> blocks, or meta commentary about how you will answer.
- Output only the final user-facing report plus concise follow-up notes when needed.
- Write the user-facing final report first. Do not merely restate the raw command/provider output.
- State whether the task succeeded, failed, was blocked, or needs follow-up.
- Use only the provided operation observations. Do not invent source-code evidence.
- Mention artifact refs, payload refs, verification results, and follow-up actions when they matter.
- If verification_status is unknown for ui_visibility, do not claim the desktop/browser window definitely opened or became visible. Say that the command request was sent/accepted and visibility was not verified, then provide the safest next check or follow-up.
- Treat payload refs, artifact refs, material refs, and workflow actions as external operation materials. A command/provider/Skill/MCP summary is only a compact observation; do not imply full material content was inspected unless the observations actually include the extracted user-relevant content.
- Keep raw logs/large output summarized; the UI will show execution details separately.
- Do not include execution-detail headings such as "Operation plan ... completed" or "操作计划 ... 已执行完成"; per-round execution details are already shown outside the final report.
- Match the requested language. If language=zh, every user-visible sentence must be Chinese except command names, file paths, product names, and raw command output snippets.
`

func (p *llmCommandOperationPlanner) AnswerCommandOperationResult(ctx context.Context, userLine string, plan operation.CommandOperationPlan, result operation.CommandOperationResult, lang string) (string, error) {
	return p.AnswerCommandOperationRecords(ctx, userLine, []commandOperationResultRecord{{Plan: plan, Result: result}}, lang)
}

func (p *llmCommandOperationPlanner) AnswerCommandOperationRecords(ctx context.Context, userLine string, records []commandOperationResultRecord, lang string) (string, error) {
	if p == nil || p.adapter == nil {
		return "", fmt.Errorf("command operation answerer not configured")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## language\n%s\n\n", strings.TrimSpace(lang))
	fmt.Fprintf(&b, "## user_request\n%s\n\n", strings.TrimSpace(userLine))
	status, rounds := commandOperationTerminalState(records)
	fmt.Fprintf(&b, "## terminal_operation_state\nstatus=%s\nrounds=%d\n", status, rounds)
	switch status {
	case operation.StatusBudgetExhausted:
		b.WriteString("constraint=The operation budget was reached before the user goal was fully satisfied. Do not call the task fully complete; write a partial/budget-limited report.\n")
	case operation.StatusPartialAnswer:
		b.WriteString("constraint=Only a partial answer is possible from the collected observations. Do not call the task fully complete.\n")
	case operation.StatusFailed:
		b.WriteString("constraint=The latest operation failed. Do not call the task complete; explain the failure and any usable partial observations.\n")
	case operation.StatusBlocked:
		b.WriteString("constraint=The operation was blocked. Do not call the task complete; explain the block and any safe next step.\n")
	case operation.StatusNeedsClarification:
		b.WriteString("constraint=The operation needs user-owned clarification before it can continue. Do not invent missing details.\n")
	}
	b.WriteString("\n")
	b.WriteString("## command_operation_rounds\n")
	b.WriteString(renderCommandOperationRecordsForPrompt(records))
	resp, err := p.adapter.Chat(ctx,
		[]llm.Message{
			{Role: "system", Content: commandOperationAnswerSystemPrompt},
			{Role: "user", Content: b.String()},
		},
		nil,
		llm.ChatOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("command operation answer llm call: %w", err)
	}
	return strings.TrimSpace(resp.Content), nil
}

func (p *llmCommandOperationPlanner) AnswerProviderOperationResult(ctx context.Context, userLine string, records []providerOperationResultRecord, lang string) (string, error) {
	if p == nil || p.adapter == nil {
		return "", fmt.Errorf("provider operation answerer not configured")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## language\n%s\n\n", strings.TrimSpace(lang))
	fmt.Fprintf(&b, "## user_request\n%s\n\n", strings.TrimSpace(userLine))
	b.WriteString("## provider_operation_results\n")
	b.WriteString(renderProviderOperationResultsForPrompt(records))
	resp, err := p.adapter.Chat(ctx,
		[]llm.Message{
			{Role: "system", Content: commandOperationAnswerSystemPrompt},
			{Role: "user", Content: b.String()},
		},
		nil,
		llm.ChatOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("provider operation answer llm call: %w", err)
	}
	return strings.TrimSpace(resp.Content), nil
}

func (p *llmCommandOperationPlanner) EvaluateProviderOperation(ctx context.Context, userLine string, records []providerOperationResultRecord, lang string) (operation.OperationEvaluation, error) {
	if p == nil || p.adapter == nil {
		return operation.OperationEvaluation{}, fmt.Errorf("provider operation evaluator not configured")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## language\n%s\n\n", strings.TrimSpace(lang))
	fmt.Fprintf(&b, "## user_request\n%s\n\n", strings.TrimSpace(userLine))
	b.WriteString("## provider_operation_results\n")
	b.WriteString(renderProviderOperationResultsForPrompt(records))
	resp, err := p.adapter.Chat(ctx,
		[]llm.Message{
			{Role: "system", Content: operationEvaluationSystemPrompt},
			{Role: "user", Content: b.String()},
		},
		[]llm.ToolSchema{operationEvaluationTool},
		llm.ChatOptions{ToolChoice: "required"},
	)
	if err != nil {
		return operation.OperationEvaluation{}, fmt.Errorf("provider operation evaluation llm call: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		return operation.OperationEvaluation{}, fmt.Errorf("provider operation evaluator: LLM returned no tool_call")
	}
	call := resp.ToolCalls[0]
	if call.Name != operationEvaluationTool.Name {
		return operation.OperationEvaluation{}, fmt.Errorf("provider operation evaluator: unexpected tool %q", call.Name)
	}
	parsed, err := unmarshalOperationEvaluation(call.Params)
	if err != nil {
		repaired, repairErr := p.repairOperationEvaluation(ctx, err, operationStructuredToolRepairRequest{
			Scope:          "provider operation evaluator",
			UserLine:       userLine,
			Language:       lang,
			ProviderRecord: records,
		})
		if repairErr != nil {
			return operation.OperationEvaluation{}, repairErr
		}
		parsed = repaired
	}
	return parsed.toEvaluation(), nil
}

func (p *llmCommandOperationPlanner) EvaluateCommandOperation(ctx context.Context, userLine string, records []commandOperationResultRecord, lang string) (operation.OperationEvaluation, error) {
	if p == nil || p.adapter == nil {
		return operation.OperationEvaluation{}, fmt.Errorf("command operation evaluator not configured")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## language\n%s\n\n", strings.TrimSpace(lang))
	fmt.Fprintf(&b, "## user_request\n%s\n\n", strings.TrimSpace(userLine))
	b.WriteString("## command_operation_rounds\n")
	b.WriteString(renderCommandOperationRecordsForPrompt(records))
	resp, err := p.adapter.Chat(ctx,
		[]llm.Message{
			{Role: "system", Content: operationEvaluationSystemPrompt},
			{Role: "user", Content: b.String()},
		},
		[]llm.ToolSchema{operationEvaluationTool},
		llm.ChatOptions{ToolChoice: "required"},
	)
	if err != nil {
		return operation.OperationEvaluation{}, fmt.Errorf("command operation evaluation llm call: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		return operation.OperationEvaluation{}, fmt.Errorf("command operation evaluator: LLM returned no tool_call")
	}
	call := resp.ToolCalls[0]
	if call.Name != operationEvaluationTool.Name {
		return operation.OperationEvaluation{}, fmt.Errorf("command operation evaluator: unexpected tool %q", call.Name)
	}
	parsed, err := unmarshalOperationEvaluation(call.Params)
	if err != nil {
		repaired, repairErr := p.repairOperationEvaluation(ctx, err, operationStructuredToolRepairRequest{
			Scope:         "command operation evaluator",
			UserLine:      userLine,
			Language:      lang,
			CommandRecord: records,
		})
		if repairErr != nil {
			return operation.OperationEvaluation{}, repairErr
		}
		parsed = repaired
	}
	return parsed.toEvaluationWithSource(operation.MaterialSourceCommand), nil
}

type commandOperationPlannerRequest struct {
	UserLine               string
	RepoRoot               string
	Policy                 TurnPolicy
	Snapshot               operation.CapabilitySnapshot
	RecentOperationContext string
	PreviousPlan           operation.CommandOperationPlan
	Result                 operation.CommandOperationResult
	Records                []commandOperationResultRecord
	Replan                 bool
	Continuation           bool
}

type operationEvaluationDraft struct {
	Status       flexiblePolicyString     `json:"status"`
	Reason       flexiblePolicyString     `json:"reason"`
	Confidence   flexiblePolicyString     `json:"confidence"`
	MissingInput flexiblePolicyStringList `json:"missing_inputs"`
	MaterialRefs flexiblePolicyStringList `json:"material_refs"`
}

func unmarshalOperationEvaluation(raw []byte) (operationEvaluationDraft, error) {
	var parsed operationEvaluationDraft
	if err := unmarshalReplStructuredToolParams(operationEvaluationTool, raw, &parsed, "operation evaluator"); err != nil {
		return parsed, err
	}
	return parsed, nil
}

func (d operationEvaluationDraft) toEvaluation() operation.OperationEvaluation {
	return d.toEvaluationWithSource(operation.MaterialSourceProvider)
}

func (d operationEvaluationDraft) toEvaluationWithSource(source string) operation.OperationEvaluation {
	status := operation.NormalizeEvaluationStatus(string(d.Status))
	materials := make([]operation.OperationMaterial, 0, len(d.MaterialRefs))
	for _, ref := range d.MaterialRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		materials = append(materials, operation.OperationMaterial{
			Source:          firstNonEmptyString(source, operation.MaterialSourceProvider),
			Kind:            operation.MaterialKindPayloadRef,
			Role:            operation.MaterialRoleSavedPayload,
			Ref:             ref,
			CompletePreview: false,
		})
	}
	return operation.OperationEvaluation{
		Status:        status,
		Reason:        strings.TrimSpace(string(d.Reason)),
		Confidence:    strings.TrimSpace(string(d.Confidence)),
		MissingInputs: []string(d.MissingInput),
		Materials:     materials,
	}
}

func (p *llmCommandOperationPlanner) planCommandOperation(ctx context.Context, req commandOperationPlannerRequest) (operation.CommandOperationRequest, error) {
	parsed, err := p.planCommandOperationDraft(ctx, req)
	if err != nil {
		return operation.CommandOperationRequest{}, err
	}
	if shouldRetryInitialCommandPlanWithoutRecentContext(req, parsed) {
		retryReq := req
		retryReq.RecentOperationContext = ""
		parsed, err = p.planCommandOperationDraft(ctx, retryReq)
		if err != nil {
			return operation.CommandOperationRequest{}, err
		}
	}
	return parsed.toRequest(req.UserLine, req.RepoRoot), nil
}

func shouldRetryInitialCommandPlanWithoutRecentContext(req commandOperationPlannerRequest, parsed commandPlanDraft) bool {
	if req.Replan || req.Continuation || strings.TrimSpace(req.RecentOperationContext) == "" {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(string(parsed.Status)))
	if status != string(operation.StatusBlocked) {
		return false
	}
	if commandPlanDraftHasQuestions(parsed) || len(parsed.Steps) > 0 {
		return false
	}
	if bool(parsed.RequiresConfirmation) {
		return false
	}
	if commandPlannerRiskRank(strings.TrimSpace(string(parsed.RiskLevel))) >= commandPlannerRiskRank("medium") {
		return false
	}
	return true
}

func commandPlannerRiskRank(risk string) int {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	case "none", "":
		return 1
	default:
		return 3
	}
}

func (p *llmCommandOperationPlanner) planCommandOperationDraft(ctx context.Context, req commandOperationPlannerRequest) (commandPlanDraft, error) {
	var zeroDraft commandPlanDraft
	if p == nil || p.adapter == nil {
		return zeroDraft, fmt.Errorf("command operation planner not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	userLine := strings.TrimSpace(req.UserLine)
	if userLine == "" {
		return zeroDraft, fmt.Errorf("command operation planner: empty user line")
	}
	var b strings.Builder
	b.WriteString("## repo_root\n")
	b.WriteString(strings.TrimSpace(req.RepoRoot))
	b.WriteString("\n\n## route_policy\n")
	b.WriteString(fmt.Sprintf("operation=%s operation_kind=%s risk=%s side_effects=%s target=%s requires_confirmation=%t\n",
		req.Policy.Operation, req.Policy.OperationKind, req.Policy.RiskLevel, strings.Join(req.Policy.SideEffects, ","), req.Policy.TargetSurface, req.Policy.RequiresConfirmation))
	if rendered := req.Snapshot.RenderForPrompt(); strings.TrimSpace(rendered) != "" {
		b.WriteString("\n")
		b.WriteString(rendered)
		b.WriteString("\n")
	}
	if strings.TrimSpace(req.RecentOperationContext) != "" {
		b.WriteString("\n## recent_operation_context\n")
		b.WriteString("Recent command-operation observations from this REPL plus operation_memory lessons from previous runs. Use only when relevant to the current request. Payload refs indicate full bounded artifacts; plan targeted follow-up reads/searches when the preview is insufficient. Historical lessons are soft guidance, not source evidence.\n")
		b.WriteString("For an initial user request, recent_operation_context is never enough by itself to emit status=complete or status=blocked. Use it only to avoid repeated mistakes and choose the next current command. Current user constraints override all recent context; if the user says not to use a tool/service, do not plan commands that invoke that tool/service.\n")
		b.WriteString(strings.TrimSpace(req.RecentOperationContext))
		b.WriteString("\n")
	}
	if req.Replan {
		b.WriteString("\n## replan_context\n")
		b.WriteString("The previous approved command plan failed. Produce a revised typed plan using the same schema.\n")
		b.WriteString("Do not include the failed command again; it already ran. Plan only the corrective next step unless a real retry is required.\n")
		b.WriteString("If one or more previous steps succeeded and those observations already satisfy the user's goal, emit status=complete with a short block_reason/reason. Do not use status=blocked for completed/no-more-work states; blocked is only for policy or capability barriers.\n")
		b.WriteString("If failure_class=invalid_plan and the error says a shell command has no explicit input source, repair by rewriting that same step as one complete standalone command with its own producer, file operand, or redirection. The command executor will not pipe output from another step into it. Examples: use \"ps aux | grep ...\" for process searches; use \"grep pattern /path/to/file\" or \"producer | grep pattern\" for text searches; use \"cat /path/to/file\" or \"producer | cat\" for cat/nl; use \"producer | head -50\" or \"head -50 /path/to/file\" for head/tail/wc/cut/sort/uniq.\n")
		b.WriteString("If failure_class=invalid_plan and the error says the shell command starts with \"|\", \"||\", \"&&\", or \";\", the command is missing the segment before the operator. Rewrite the whole command with a real producer before the operator; do not keep the leading operator.\n")
		b.WriteString("If failure_class=invalid_plan and the error says a program/shell field contains structured JSON, repair by moving those JSON members into typed step fields such as program, args, shell, title, risk_level, and side_effects. Do not place JSON arrays or objects inside a command string.\n")
		b.WriteString(renderCommandPlanForPrompt(req.PreviousPlan))
		b.WriteString("\n")
		b.WriteString(renderCommandResultForPrompt(req.Result))
		b.WriteString("\n")
	}
	if req.Continuation {
		b.WriteString("\n## continuation_context\n")
		b.WriteString("The previous command batch executed. Decide whether the observations are sufficient. If yes, emit status=complete. If not, emit a ready plan containing only the next bounded command batch. Do not ask the user for facts that can still be discovered by safe read-only commands; keep investigating until the task is answered, the bounded continuation budget is exhausted, or the missing input is truly user-owned.\n")
		b.WriteString("For command, file, web page, manual, log, trace, MCP/provider payload, Skill artifact, help-text, or large-output reading tasks, a truncated preview, payload_ref/artifact_ref, or output_kind=large_output_summary is not by itself a reason to finalize as partial. If the requested answer depends on omitted content, continue with a bounded follow-up read/search/page/parse/extract or provider workflow action over the saved payload or discovered linked artifacts.\n")
		b.WriteString("If the saved payload is complete but the preview is mostly boilerplate, framing, headers, CSS/scripts/navigation, long logs before the target section, binary-ish metadata, or tool banners, continue by extracting the user-relevant text, links, matching lines, surrounding context, or structured fields before deciding whether the task is complete.\n")
		b.WriteString(renderCommandOperationRecordsForPrompt(req.Records))
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
		return zeroDraft, fmt.Errorf("command operation planner llm call: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		return zeroDraft, fmt.Errorf("command operation planner: LLM returned no tool_call")
	}
	call := resp.ToolCalls[0]
	if call.Name != commandOperationPlanTool.Name {
		return zeroDraft, fmt.Errorf("command operation planner: unexpected tool %q", call.Name)
	}
	parsed, err := unmarshalCommandOperationPlan(call.Params)
	if err != nil {
		repaired, repairErr := p.repairCommandOperationPlan(ctx, err, req)
		if repairErr != nil {
			return zeroDraft, repairErr
		}
		return repaired, nil
	}
	return parsed, nil
}

type operationStructuredToolRepairRequest struct {
	Scope          string
	UserLine       string
	Language       string
	PlannerRequest commandOperationPlannerRequest
	CommandRecord  []commandOperationResultRecord
	ProviderRecord []providerOperationResultRecord
}

func (p *llmCommandOperationPlanner) repairCommandOperationPlan(ctx context.Context, parseErr error, req commandOperationPlannerRequest) (commandPlanDraft, error) {
	raw, err := p.repairOperationStructuredToolParams(ctx, commandOperationPlanTool, parseErr, operationStructuredToolRepairRequest{
		Scope:          "command operation planner",
		UserLine:       req.UserLine,
		PlannerRequest: req,
	})
	if err != nil {
		return commandPlanDraft{}, err
	}
	parsed, err := unmarshalCommandOperationPlan(raw)
	if err != nil {
		return commandPlanDraft{}, fmt.Errorf("%w; compact tool-param repair also failed: %v", parseErr, err)
	}
	return parsed, nil
}

func (p *llmCommandOperationPlanner) repairOperationEvaluation(ctx context.Context, parseErr error, req operationStructuredToolRepairRequest) (operationEvaluationDraft, error) {
	raw, err := p.repairOperationStructuredToolParams(ctx, operationEvaluationTool, parseErr, req)
	if err != nil {
		return operationEvaluationDraft{}, err
	}
	parsed, err := unmarshalOperationEvaluation(raw)
	if err != nil {
		return operationEvaluationDraft{}, fmt.Errorf("%w; compact tool-param repair also failed: %v", parseErr, err)
	}
	return parsed, nil
}

func (p *llmCommandOperationPlanner) repairOperationStructuredToolParams(ctx context.Context, tool llm.ToolSchema, parseErr error, req operationStructuredToolRepairRequest) (json.RawMessage, error) {
	var paramErr *replStructuredToolParamError
	if !errors.As(parseErr, &paramErr) {
		return nil, parseErr
	}
	if p == nil || p.adapter == nil {
		return nil, parseErr
	}
	scope := firstNonEmptyString(strings.TrimSpace(req.Scope), strings.TrimSpace(paramErr.Scope), "operation structured tool")
	logging.Warning("[repl/operation] structured tool params invalid tool=%s scope=%s raw_bytes=%d; attempting compact repair",
		firstNonEmptyString(tool.Name, paramErr.ToolName), scope, paramErr.RawLen)
	resp, err := p.adapter.Chat(ctx,
		[]llm.Message{
			{Role: "system", Content: operationStructuredToolRepairSystemPrompt},
			{Role: "user", Content: operationStructuredToolRepairPrompt(tool, paramErr, req)},
		},
		[]llm.ToolSchema{tool},
		llm.ChatOptions{ToolChoice: "required"},
	)
	if err != nil {
		return nil, fmt.Errorf("%w; compact tool-param repair llm call failed: %v", parseErr, err)
	}
	if len(resp.ToolCalls) == 0 {
		return nil, fmt.Errorf("%w; compact tool-param repair returned no tool_call", parseErr)
	}
	call := resp.ToolCalls[0]
	if call.Name != tool.Name {
		return nil, fmt.Errorf("%w; compact tool-param repair returned unexpected tool %q", parseErr, call.Name)
	}
	return call.Params, nil
}

func operationStructuredToolRepairPrompt(tool llm.ToolSchema, paramErr *replStructuredToolParamError, req operationStructuredToolRepairRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## repair_scope\n%s\n\n", firstNonEmptyString(strings.TrimSpace(req.Scope), strings.TrimSpace(paramErr.Scope), "operation structured tool"))
	fmt.Fprintf(&b, "## required_tool\n%s\n\n", tool.Name)
	fmt.Fprintf(&b, "## parse_failure\nraw_bytes=%d\nerror=%s\nhint=%s\n\n",
		paramErr.RawLen,
		oneLineClamp(fmt.Sprint(paramErr.Err), 400),
		oneLineClamp(paramErr.Hint, 400))
	b.WriteString("## schema\n")
	b.WriteString(oneLineClamp(tool.Description, 600))
	b.WriteString("\n")
	b.WriteString(clampMultilineForRepair(string(tool.Parameters), 6000))
	b.WriteString("\n\n## compact_operation_context\n")
	b.WriteString(operationStructuredToolRepairContext(req))
	return strings.TrimSpace(b.String())
}

func operationStructuredToolRepairContext(req operationStructuredToolRepairRequest) string {
	if len(req.CommandRecord) > 0 {
		return operationCommandRecordsRepairContext(req.UserLine, req.Language, req.CommandRecord)
	}
	if len(req.ProviderRecord) > 0 {
		return operationProviderRecordsRepairContext(req.UserLine, req.Language, req.ProviderRecord)
	}
	return operationPlannerRepairContext(req.PlannerRequest)
}

func operationPlannerRepairContext(req commandOperationPlannerRequest) string {
	var b strings.Builder
	mode := "initial"
	if req.Replan {
		mode = "replan"
	} else if req.Continuation {
		mode = "continuation"
	}
	fmt.Fprintf(&b, "mode=%s\n", mode)
	fmt.Fprintf(&b, "repo_root=%s\n", strings.TrimSpace(req.RepoRoot))
	fmt.Fprintf(&b, "route_policy operation=%s operation_kind=%s risk=%s target=%s requires_confirmation=%t side_effects=%s\n",
		req.Policy.Operation, req.Policy.OperationKind, req.Policy.RiskLevel, req.Policy.TargetSurface, req.Policy.RequiresConfirmation, strings.Join(req.Policy.SideEffects, ","))
	if rendered := req.Snapshot.RenderForPrompt(); strings.TrimSpace(rendered) != "" {
		fmt.Fprintf(&b, "capability_snapshot=%q\n", oneLineClamp(rendered, 800))
	}
	if strings.TrimSpace(req.RecentOperationContext) != "" {
		fmt.Fprintf(&b, "recent_operation_context_summary=%q\n", oneLineClamp(req.RecentOperationContext, 360))
	}
	if req.Replan {
		b.WriteString(operationPlanRepairContext(req.PreviousPlan))
		b.WriteString("\n")
		b.WriteString(operationResultRepairContext(req.Result))
		b.WriteString("\n")
	}
	if req.Continuation {
		b.WriteString(operationCommandRecordsRepairContext(req.UserLine, "", req.Records))
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "user_request=%q\n", oneLineClamp(req.UserLine, 1200))
	return strings.TrimRight(b.String(), "\n")
}

func operationCommandRecordsRepairContext(userLine, lang string, records []commandOperationResultRecord) string {
	var b strings.Builder
	if strings.TrimSpace(lang) != "" {
		fmt.Fprintf(&b, "language=%s\n", strings.TrimSpace(lang))
	}
	fmt.Fprintf(&b, "user_request=%q\n", oneLineClamp(userLine, 1200))
	start := len(records) - 3
	if start < 0 {
		start = 0
	}
	if start > 0 {
		fmt.Fprintf(&b, "omitted_older_rounds=%d\n", start)
	}
	for i := start; i < len(records); i++ {
		rec := records[i]
		fmt.Fprintf(&b, "round[%d]\n", i+1)
		b.WriteString(operationPlanRepairContext(rec.Plan))
		b.WriteString("\n")
		b.WriteString(operationResultRepairContext(rec.Result))
		if rec.Evaluation != nil {
			fmt.Fprintf(&b, "\nevaluation status=%s confidence=%q reason=%q material_refs=%q",
				rec.Evaluation.Status,
				oneLineClamp(rec.Evaluation.Confidence, 80),
				oneLineClamp(rec.Evaluation.Reason, 320),
				operation.RenderMaterialsInline(rec.Evaluation.Materials, 6))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func operationProviderRecordsRepairContext(userLine, lang string, records []providerOperationResultRecord) string {
	var b strings.Builder
	if strings.TrimSpace(lang) != "" {
		fmt.Fprintf(&b, "language=%s\n", strings.TrimSpace(lang))
	}
	fmt.Fprintf(&b, "user_request=%q\n", oneLineClamp(userLine, 1200))
	start := len(records) - 3
	if start < 0 {
		start = 0
	}
	if start > 0 {
		fmt.Fprintf(&b, "omitted_older_provider_rounds=%d\n", start)
	}
	for i := start; i < len(records); i++ {
		rec := records[i]
		result := rec.Result
		materials := operation.MaterialsFromProviderResult(
			result.Provider,
			result.Tool,
			result.Summary,
			result.PayloadRef,
			result.ArtifactRefs,
			result.Observations,
			result.NextActions,
			result.ReturnAction,
			result.WorkflowState,
		)
		fmt.Fprintf(&b, "provider_round[%d] provider=%q tool=%q kind=%q target=%q status=%s observations=%d summary=%q error=%q payload_ref=%s artifact_refs=%s material_refs=%q next_actions=%d return_action=%q workflow_state=%q\n",
			i+1,
			oneLineClamp(result.Provider, 120),
			oneLineClamp(result.Tool, 120),
			oneLineClamp(firstNonEmptyString(rec.Plan.Kind, rec.Request.OperationKind, rec.Request.Operation), 80),
			oneLineClamp(firstNonEmptyString(rec.Plan.TargetSurface, rec.Request.TargetSurface), 80),
			result.Status,
			result.Observations,
			oneLineClamp(result.Summary, 600),
			oneLineClamp(result.Error, 240),
			result.PayloadRef,
			oneLineClamp(strings.Join(result.ArtifactRefs, ","), 500),
			operation.RenderMaterialsInline(materials, 8),
			len(result.NextActions),
			oneLineClamp(result.ReturnAction.Compact(), 260),
			oneLineClamp(result.WorkflowState.Compact(), 260))
	}
	return strings.TrimRight(b.String(), "\n")
}

func operationPlanRepairContext(plan operation.CommandOperationPlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "plan id=%s status=%s risk=%s approval=%s work_dir=%s\n",
		plan.ID, plan.Status, plan.RiskLevel, plan.ApprovalMode, plan.WorkDir)
	if strings.TrimSpace(plan.Goal) != "" {
		fmt.Fprintf(&b, "  goal=%q\n", oneLineClamp(plan.Goal, 240))
	}
	if len(plan.KnownConstraints) > 0 {
		fmt.Fprintf(&b, "  known_constraints=%q\n", oneLineClamp(strings.Join(plan.KnownConstraints, " | "), 360))
	}
	if len(plan.MissingObservations) > 0 {
		fmt.Fprintf(&b, "  missing_observations=%q\n", oneLineClamp(strings.Join(plan.MissingObservations, " | "), 360))
	}
	if len(plan.SuccessCriteria) > 0 {
		fmt.Fprintf(&b, "  success_criteria=%q\n", oneLineClamp(strings.Join(plan.SuccessCriteria, " | "), 360))
	}
	if strings.TrimSpace(plan.NextBatch) != "" {
		fmt.Fprintf(&b, "  next_batch=%q\n", oneLineClamp(plan.NextBatch, 240))
	}
	if strings.TrimSpace(plan.WhyThisBatch) != "" {
		fmt.Fprintf(&b, "  why_this_batch=%q\n", oneLineClamp(plan.WhyThisBatch, 300))
	}
	for i, step := range plan.Steps {
		if i >= 6 {
			fmt.Fprintf(&b, "  omitted_steps=%d\n", len(plan.Steps)-i)
			break
		}
		fmt.Fprintf(&b, "  step[%d] id=%s title=%q cmd=%q risk=%s side_effects=%s\n",
			i+1, step.ID, oneLineClamp(step.Title, 120), oneLineClamp(commandOperationStepCommand(step), 260), step.RiskLevel, strings.Join(step.SideEffects, ","))
	}
	return strings.TrimRight(b.String(), "\n")
}

func operationResultRepairContext(result operation.CommandOperationResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "result plan_id=%s status=%s output_preview=%q payload_ref=%s\n",
		result.PlanID, result.Status, oneLineClamp(result.OutputPreview, 600), result.PayloadRef)
	for i, step := range result.StepResults {
		if i >= 8 {
			fmt.Fprintf(&b, "  omitted_step_results=%d\n", len(result.StepResults)-i)
			break
		}
		summary := operation.SummarizeStepOutput(step)
		fmt.Fprintf(&b, "  result[%d] step_id=%s status=%s exit_code=%d timed_out=%t failure_class=%s verification=%s output_kind=%s output_summary=%q error=%q output_preview=%q payload_ref=%s\n",
			i+1,
			step.StepID,
			step.Status,
			step.ExitCode,
			step.TimedOut,
			step.FailureClass,
			step.Verification.Status,
			summary.Kind,
			oneLineClamp(summary.Summary, 260),
			oneLineClamp(step.Error, 240),
			oneLineClamp(step.OutputPreview, 600),
			step.PayloadRef)
	}
	return strings.TrimRight(b.String(), "\n")
}

func clampMultilineForRepair(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "\n...[compact repair context truncated]"
}

func renderCommandPlanForPrompt(plan operation.CommandOperationPlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "previous_plan id=%s status=%s risk=%s approval=%s work_dir=%s\n",
		plan.ID, plan.Status, plan.RiskLevel, plan.ApprovalMode, plan.WorkDir)
	if strings.TrimSpace(plan.Goal) != "" {
		fmt.Fprintf(&b, "goal=%q\n", plan.Goal)
	}
	if len(plan.KnownConstraints) > 0 {
		fmt.Fprintf(&b, "known_constraints=%q\n", strings.Join(plan.KnownConstraints, " | "))
	}
	if len(plan.MissingObservations) > 0 {
		fmt.Fprintf(&b, "missing_observations=%q\n", strings.Join(plan.MissingObservations, " | "))
	}
	if len(plan.SuccessCriteria) > 0 {
		fmt.Fprintf(&b, "success_criteria=%q\n", strings.Join(plan.SuccessCriteria, " | "))
	}
	if strings.TrimSpace(plan.NextBatch) != "" {
		fmt.Fprintf(&b, "next_batch=%q\n", plan.NextBatch)
	}
	if strings.TrimSpace(plan.WhyThisBatch) != "" {
		fmt.Fprintf(&b, "why_this_batch=%q\n", plan.WhyThisBatch)
	}
	for i, step := range plan.Steps {
		cmd := strings.TrimSpace(step.Program + " " + strings.Join(step.Args, " "))
		if strings.TrimSpace(step.Shell) != "" {
			cmd = "shell: " + strings.TrimSpace(step.Shell)
		}
		fmt.Fprintf(&b, "step[%d] id=%s status_planned title=%q cmd=%q risk=%s side_effects=%s\n",
			i+1, step.ID, step.Title, cmd, step.RiskLevel, strings.Join(step.SideEffects, ","))
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderCommandResultForPrompt(result operation.CommandOperationResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "previous_result plan_id=%s status=%s\n", result.PlanID, result.Status)
	if strings.TrimSpace(result.OutputPreview) != "" {
		fmt.Fprintf(&b, "result_summary=%q\n", oneLineClamp(result.OutputPreview, 2000))
	}
	if strings.TrimSpace(result.PayloadRef) != "" {
		fmt.Fprintf(&b, "result_payload_ref=%s\n", result.PayloadRef)
	}
	if rendered := renderCommandPayloadMaterialExcerpts(commandOperationResultPayloadRefs(result)); rendered != "" {
		b.WriteString(rendered)
		b.WriteString("\n")
	}
	for i, step := range result.StepResults {
		summary := operation.SummarizeStepOutput(step)
		preview := strings.TrimSpace(step.OutputPreview)
		if len(preview) > 2000 {
			preview = preview[:2000] + "...[truncated]"
		}
		fmt.Fprintf(&b, "result[%d] step_id=%s status=%s exit_code=%d timed_out=%t failure_class=%s verification_status=%s verification_summary=%q output_kind=%s output_summary=%q output_lines=%d output_bytes=%d error=%q output=%q payload_ref=%s\n",
			i+1, step.StepID, step.Status, step.ExitCode, step.TimedOut, step.FailureClass, step.Verification.Status, step.Verification.Summary, summary.Kind, summary.Summary, summary.Lines, summary.Bytes, step.Error, preview, step.PayloadRef)
		if rendered := operation.RenderMaterialsForPrompt(operation.MaterialsFromCommandStep(step), 4); rendered != "" {
			b.WriteString(indentPromptBlock(rendered, "  "))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func commandOperationResultPayloadRefs(result operation.CommandOperationResult) []string {
	seen := map[string]bool{}
	var refs []string
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			return
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	add(result.PayloadRef)
	for _, step := range result.StepResults {
		add(step.PayloadRef)
	}
	return refs
}

func renderCommandPayloadMaterialExcerpts(refs []string) string {
	var b strings.Builder
	for _, ref := range refs {
		excerpt := commandPayloadMaterialExcerpt(ref)
		if excerpt == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(excerpt)
	}
	return strings.TrimRight(b.String(), "\n")
}

func commandPayloadMaterialExcerpt(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	data, err := os.ReadFile(ref)
	if err != nil || len(data) == 0 {
		return ""
	}
	const maxRead = 256 * 1024
	truncated := false
	if len(data) > maxRead {
		data = data[:maxRead]
		truncated = true
	}
	if bytes.Contains(data, []byte{0}) {
		return fmt.Sprintf("payload_material_excerpt ref=%s kind=binary skipped=true\n", ref)
	}
	raw := string(data)
	kind := "text"
	text := raw
	if looksLikeHTML(raw) {
		kind = "html_text"
		text = extractHTMLVisibleText(raw)
	}
	text = compactMaterialText(text)
	if text == "" {
		return ""
	}
	const maxRunes = 4000
	runes := []rune(text)
	if len(runes) > maxRunes {
		text = string(runes[:maxRunes]) + "\n...[material excerpt truncated]"
	}
	return fmt.Sprintf("payload_material_excerpt ref=%s kind=%s source_truncated=%t\n%s", ref, kind, truncated, text)
}

func commandPayloadHasMaterialExcerpt(ref string) bool {
	excerpt := commandPayloadMaterialExcerpt(ref)
	return excerpt != "" && !strings.Contains(excerpt, "skipped=true")
}

func looksLikeHTML(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "<html") || strings.Contains(lower, "<article") || strings.Contains(lower, "<body") || strings.Contains(lower, "<!doctype html")
}

func extractHTMLVisibleText(s string) string {
	s = operationMaterialScriptRE.ReplaceAllString(s, "\n")
	s = operationMaterialStyleRE.ReplaceAllString(s, "\n")
	s = operationMaterialSVGRE.ReplaceAllString(s, "\n")
	s = operationMaterialBlockTagRE.ReplaceAllString(s, "\n")
	s = operationMaterialAnyTagRE.ReplaceAllString(s, " ")
	return html.UnescapeString(s)
}

func compactMaterialText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(operationMaterialBlankLineRE.ReplaceAllString(strings.Join(out, "\n"), "\n\n"))
}

func renderCommandOperationRecordsForPrompt(records []commandOperationResultRecord) string {
	var b strings.Builder
	for i, rec := range records {
		fmt.Fprintf(&b, "round[%d]\n", i+1)
		b.WriteString(renderCommandPlanForPrompt(rec.Plan))
		b.WriteString("\n")
		b.WriteString(renderCommandResultForPrompt(rec.Result))
		if rec.Evaluation != nil {
			fmt.Fprintf(&b, "\noperation_evaluation status=%s confidence=%q reason=%q missing_inputs=%q materials=%q\n",
				rec.Evaluation.Status,
				oneLineClamp(rec.Evaluation.Confidence, 80),
				oneLineClamp(rec.Evaluation.Reason, 260),
				strings.Join(rec.Evaluation.MissingInputs, " | "),
				operation.RenderMaterialsInline(rec.Evaluation.Materials, 6))
		}
		if i < len(records)-1 {
			b.WriteString("\n\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderProviderOperationResultsForPrompt(records []providerOperationResultRecord) string {
	var b strings.Builder
	for i, rec := range records {
		result := rec.Result
		materials := operation.MaterialsFromProviderResult(
			result.Provider,
			result.Tool,
			result.Summary,
			result.PayloadRef,
			result.ArtifactRefs,
			result.Observations,
			result.NextActions,
			result.ReturnAction,
			result.WorkflowState,
		)
		fmt.Fprintf(&b, "provider_result[%d] provider=%q tool=%q kind=%q target=%q status=%s observations=%d verification_status=%s verification_summary=%q summary=%q error=%q payload_ref=%s artifact_refs=%s material_refs=%q next_actions=%d return_action=%q workflow_state=%q diagnostics=%q\n",
			i+1,
			result.Provider,
			result.Tool,
			firstNonEmptyString(rec.Plan.Kind, rec.Request.OperationKind, rec.Request.Operation),
			firstNonEmptyString(rec.Plan.TargetSurface, rec.Request.TargetSurface),
			result.Status,
			result.Observations,
			result.VerificationStatus,
			result.VerificationSummary,
			oneLineClamp(result.Summary, 2000),
			oneLineClamp(result.Error, 1000),
			result.PayloadRef,
			strings.Join(result.ArtifactRefs, ","),
			operation.RenderMaterialsInline(materials, 8),
			len(result.NextActions),
			result.ReturnAction.Compact(),
			result.WorkflowState.Compact(),
			strings.Join(result.WorkflowDiagnostics, "; "))
		if rendered := operation.RenderMaterialsForPrompt(materials, 8); rendered != "" {
			b.WriteString(indentPromptBlock(rendered, "  "))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func indentPromptBlock(text, prefix string) string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

type commandPlanDraft struct {
	Status               flexiblePolicyString       `json:"status"`
	RiskLevel            flexiblePolicyString       `json:"risk_level"`
	WorkDir              flexiblePolicyString       `json:"work_dir"`
	Goal                 flexiblePolicyString       `json:"goal"`
	KnownConstraints     flexiblePolicyStringList   `json:"known_constraints"`
	MissingObservations  flexiblePolicyStringList   `json:"missing_observations"`
	SuccessCriteria      flexiblePolicyStringList   `json:"success_criteria"`
	NextBatch            flexiblePolicyString       `json:"next_batch"`
	WhyThisBatch         flexiblePolicyString       `json:"why_this_batch"`
	RequiresConfirmation flexiblePolicyBool         `json:"requires_confirmation"`
	ContinueAfter        flexiblePolicyBool         `json:"continue_after"`
	Questions            []commandPlanQuestionDraft `json:"questions"`
	BlockReason          flexiblePolicyString       `json:"block_reason"`
	Steps                flexibleCommandPlanSteps   `json:"steps"`
}

type commandPlanQuestionDraft struct {
	ID          flexiblePolicyString     `json:"id"`
	Question    flexiblePolicyString     `json:"question"`
	Suggestions flexiblePolicyStringList `json:"suggestions"`
}

type commandPlanStepDraft struct {
	ID          flexiblePolicyString     `json:"id"`
	Title       flexiblePolicyString     `json:"title"`
	Program     flexiblePolicyString     `json:"program"`
	Args        flexiblePolicyStringList `json:"args"`
	Shell       flexiblePolicyString     `json:"shell"`
	WorkDir     flexiblePolicyString     `json:"work_dir"`
	Env         flexiblePolicyStringList `json:"env"`
	TimeoutMS   flexiblePolicyInt        `json:"timeout_ms"`
	RiskLevel   flexiblePolicyString     `json:"risk_level"`
	SideEffects flexiblePolicyStringList `json:"side_effects"`
	Reason      flexiblePolicyString     `json:"reason"`
	VerifyHint  flexiblePolicyString     `json:"verify_hint"`
}

type flexibleCommandPlanSteps []commandPlanStepDraft

func (s *flexibleCommandPlanSteps) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*s = nil
		return nil
	}
	var arr []commandPlanStepDraft
	if err := json.Unmarshal(raw, &arr); err == nil {
		*s = cleanCommandPlanSteps(arr)
		return nil
	}
	var obj commandPlanStepDraft
	if err := json.Unmarshal(raw, &obj); err == nil {
		*s = cleanCommandPlanSteps([]commandPlanStepDraft{obj})
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			*s = nil
			return nil
		}
		if strings.HasPrefix(text, "[") {
			var nested []commandPlanStepDraft
			if err := json.Unmarshal([]byte(text), &nested); err == nil {
				*s = cleanCommandPlanSteps(nested)
				return nil
			}
		}
		if strings.HasPrefix(text, "{") {
			var nested commandPlanStepDraft
			if err := json.Unmarshal([]byte(text), &nested); err == nil {
				*s = cleanCommandPlanSteps([]commandPlanStepDraft{nested})
				return nil
			}
		}
		*s = []commandPlanStepDraft{{
			ID:        "step-1",
			Title:     "command",
			Shell:     flexiblePolicyString(text),
			RiskLevel: "medium",
		}}
		return nil
	}
	return fmt.Errorf("invalid command steps value %s", string(raw))
}

func cleanCommandPlanSteps(in []commandPlanStepDraft) []commandPlanStepDraft {
	out := make([]commandPlanStepDraft, 0, len(in))
	for _, step := range in {
		if strings.TrimSpace(string(step.ID)) == "" && strings.TrimSpace(string(step.Title)) == "" &&
			strings.TrimSpace(string(step.Program)) == "" && strings.TrimSpace(string(step.Shell)) == "" {
			continue
		}
		out = append(out, step)
	}
	return out
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
	if err := unmarshalReplStructuredToolParams(commandOperationPlanTool, raw, &parsed, "command operation planner"); err != nil {
		return parsed, err
	}
	return parsed, nil
}

func commandPlanDraftHasQuestions(d commandPlanDraft) bool {
	for _, q := range d.Questions {
		if strings.TrimSpace(string(q.Question)) != "" {
			return true
		}
	}
	return false
}

func (d commandPlanDraft) toRequest(userLine, repoRoot string) operation.CommandOperationRequest {
	req := operation.CommandOperationRequest{
		Text:                 userLine,
		WorkDir:              plannerFirstNonEmpty(strings.TrimSpace(string(d.WorkDir)), strings.TrimSpace(repoRoot)),
		RiskLevel:            strings.TrimSpace(string(d.RiskLevel)),
		BlockReason:          strings.TrimSpace(string(d.BlockReason)),
		Goal:                 strings.TrimSpace(string(d.Goal)),
		KnownConstraints:     []string(d.KnownConstraints),
		MissingObservations:  []string(d.MissingObservations),
		SuccessCriteria:      []string(d.SuccessCriteria),
		NextBatch:            strings.TrimSpace(string(d.NextBatch)),
		WhyThisBatch:         strings.TrimSpace(string(d.WhyThisBatch)),
		RequiresConfirmation: bool(d.RequiresConfirmation),
		ContinueAfter:        bool(d.ContinueAfter),
	}
	switch strings.TrimSpace(string(d.Status)) {
	case "complete":
		req.TerminalStatus = operation.StatusComplete
		return req
	case "budget_exhausted":
		req.TerminalStatus = operation.StatusBudgetExhausted
		return req
	case "partial_answer_possible":
		req.TerminalStatus = operation.StatusPartialAnswer
		return req
	}
	if strings.TrimSpace(string(d.Status)) == string(operation.StatusNeedsClarification) {
		for _, q := range d.Questions {
			if strings.TrimSpace(string(q.Question)) == "" {
				continue
			}
			req.ClarifyingQuestions = append(req.ClarifyingQuestions, operation.ClarifyingQuestion{
				ID:          strings.TrimSpace(string(q.ID)),
				Question:    strings.TrimSpace(string(q.Question)),
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
	if strings.TrimSpace(string(d.Status)) == string(operation.StatusBlocked) && req.BlockReason == "" {
		req.BlockReason = "planner marked the operation blocked"
		return req
	}
	for _, step := range d.Steps {
		if strings.TrimSpace(string(step.Program)) == "" && strings.TrimSpace(string(step.Shell)) == "" {
			continue
		}
		req.Steps = append(req.Steps, operation.CommandStep{
			ID:          strings.TrimSpace(string(step.ID)),
			Title:       strings.TrimSpace(string(step.Title)),
			Program:     strings.TrimSpace(string(step.Program)),
			Args:        []string(step.Args),
			Shell:       strings.TrimSpace(string(step.Shell)),
			WorkDir:     strings.TrimSpace(string(step.WorkDir)),
			Env:         []string(step.Env),
			TimeoutMS:   int(step.TimeoutMS),
			RiskLevel:   strings.TrimSpace(string(step.RiskLevel)),
			SideEffects: []string(step.SideEffects),
			Reason:      strings.TrimSpace(string(step.Reason)),
			VerifyHint:  strings.TrimSpace(string(step.VerifyHint)),
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
