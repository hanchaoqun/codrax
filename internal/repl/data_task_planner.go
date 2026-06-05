package repl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
)

type DataTaskPlanner interface {
	PlanDataTask(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile) (dataquery.TaskPlan, error)
}

type DataTaskRepairPlanner interface {
	RepairDataTask(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, previous dataquery.TaskPlan, executionError string) (dataquery.TaskPlan, error)
}

type DataTaskEvaluator interface {
	EvaluateDataTask(ctx context.Context, userLine string, records []dataTaskWorkflowRecord, lang string) (dataquery.Evaluation, error)
}

type DataTaskContinuationPlanner interface {
	ContinueDataTask(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, records []dataTaskWorkflowRecord) (dataquery.TaskPlan, error)
}

type llmDataTaskPlanner struct {
	adapter   llm.Adapter
	lastTrace replLLMCallTrace
}

func NewDataTaskPlanner(adapter llm.Adapter) DataTaskPlanner {
	return &llmDataTaskPlanner{adapter: adapter}
}

var dataTaskPlanTool = llm.ToolSchema{
	Name:        "emit_data_task_plan",
	Description: "Emit a typed read-only data-task plan. This tool never executes code.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "status": {
      "type": "string",
      "enum": ["ready", "needs_clarification", "blocked", "complete", "budget_exhausted", "partial_answer_possible"],
      "description": "ready when input paths, output contract, and script are concrete; needs_clarification only for user-owned missing inputs; blocked when safe read-only data calculation cannot be attempted; complete/budget_exhausted/partial_answer_possible are terminal workflow statuses for repair/continuation turns."
    },
    "input_paths": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Repo/workspace-relative local data files or directories to copy into the isolated data workspace. Use paths from the candidate list when possible."
    },
    "output_contract": {
      "type": "object",
      "properties": {
        "format": {"type": "string", "enum": ["plain_single_line", "csv_line", "json_only", "markdown_table", "markdown", "file_path", "freeform"]},
        "explanation_allowed": {"type": "boolean"},
        "delimiter": {"type": "string"}
      },
      "required": ["format", "explanation_allowed"],
      "description": "Typed final-output contract derived from the user's requested shape. If the user says only output JSON/CSV/a single line, set explanation_allowed=false."
    },
    "coverage_contract": {
      "type": "object",
      "properties": {
        "required_materials": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "id": {"type":"string"},
              "path": {"type":"string"},
              "purpose": {"type":"string"},
              "usage_mode": {"type":"string", "enum":["script_consumed","text_evidence_consumed","planner_distilled","reference_only"], "description":"How this material is covered. Empty means script_consumed. text_evidence_consumed requires text_evidence_path. planner_distilled requires distilled_notes. reference_only should normally be optional."},
              "text_evidence_path": {"type":"string"},
              "distilled_notes": {"type":"array", "items":{"type":"string"}},
              "required": {"type":"boolean"}
            }
          },
          "description": "Model-authored task-specific materials that must be covered before the result is trustworthy. Use broad current-task purposes; do not rely on hard-coded file names or business categories. Choose usage_mode: script_consumed means the material must be listed in input_paths and directly read by the script; text_evidence_consumed means text_evidence_path is listed/read instead; planner_distilled means the material has been distilled into typed rules/constraints and distilled_notes must explain the distilled content; reference_only is advisory and should normally be optional."
        },
        "optional_materials": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "id": {"type":"string"},
              "path": {"type":"string"},
              "purpose": {"type":"string"},
              "usage_mode": {"type":"string", "enum":["script_consumed","text_evidence_consumed","planner_distilled","reference_only"]},
              "text_evidence_path": {"type":"string"},
              "distilled_notes": {"type":"array", "items":{"type":"string"}},
              "required": {"type":"boolean"}
            }
          },
          "description": "Relevant but non-blocking materials."
        },
        "validation_rules": {
          "type": "array",
          "items": {"type":"string"},
          "description": "Task-specific structural validation rules the result should satisfy."
        },
        "decision_records_required": {
          "type": "boolean",
          "description": "true when filtering, joining, aggregation, or item-level decisions must be auditable through generic result.rows decision records. These records may represent table rows, JSON items, text spans, pages, image regions, or any task-specific item."
        },
        "rule_coverage_required": {
          "type": "boolean",
          "description": "true when the result must include generic result.rule_coverage records showing which user/model rules were applied, not applicable, failed, or unresolved. Use for cleaning/filtering/calculation rules; do not encode domain-specific rule types."
        },
        "contribution_ledger_required": {
          "type": "boolean",
          "description": "true when the result must include generic result.contributions records for items contributing to totals, counts, groups, ranks, or final output elements. A contribution item can be a row, JSON item, text span, page, image region, or other task item."
        },
        "entity_resolution_required": {
          "type": "boolean",
          "description": "true when the result must include generic result.entity_resolutions records for source values mapped to canonical values, including ambiguous/unresolved cases. Use for joins, name/id/category normalization, and cross-material linking."
        },
        "reconcile_required": {
          "type": "boolean",
          "description": "true when the result must include result.reconcile with status=pass and reconcile.groups. The system recomputes numeric group totals from result.contributions when available and rejects mismatches."
        }
      },
      "description": "Generic material coverage contract. The model decides material purpose from the user goal and objective inventory; the system only verifies declared required materials are input and actually consumed. Listing a file in input_paths is not consumption; the script must read it through the provided helpers and use it for the result, validation, or audit."
    },
    "script": {
      "type": "string",
      "description": "Python calculation body executed by a bounded local data helper. Prefer csv_rows(path), tsv_rows(path), json_load(path), jsonl_rows(path), read_text(path), parse_money(value), Decimal, re, and ledger helpers add_decision(...), add_rule_coverage(...), add_contribution(...), add_resolution(...), emit_result(...). You may import only common data standard libraries such as csv/json/decimal/re/math/statistics/collections/datetime/itertools/functools/operator/string/textwrap/base64/hashlib/unicodedata/fractions/calendar, and open(path) only reads declared input files. print(...) is allowed only for small debug output; the final answer must still be emitted. Do not access os/sys/pathlib/shutil, run shell commands, use network/process libraries, dynamic eval/exec, or write files. If the user task truly requires side effects, block this data plan so the operation pipeline can handle risk/approval instead of hiding those actions inside the data script. The script must call emit_result(answer, output_contract=...) or emit({\"answer\": string, \"output_contract\": object, \"audit_summary\": string, \"rows\": [...], \"rule_coverage\": [...], \"contributions\": [...], \"entity_resolutions\": [...], \"reconcile\": {...}}). Include only the ledger fields required by coverage_contract for this task."
    },
    "goal": {
      "type": "string",
      "description": "The user's data-processing goal in one concise sentence."
    },
    "known_constraints": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Typed constraints from the user request, such as strict output-only shape, inclusion/exclusion rules, source-file constraints, or read-only requirements."
    },
    "missing_observations": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Facts still missing after this batch. Do not ask the user for facts that the script can safely compute from declared local data files."
    },
    "success_criteria": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Concrete criteria the final data result must satisfy."
    },
    "next_batch": {
      "type": "string",
      "description": "Short purpose of the next bounded data batch when continue_after=true."
    },
    "why_this_batch": {
      "type": "string",
      "description": "Why this bounded data batch is the right next step."
    },
    "continue_after": {
      "type": "boolean",
      "description": "true when this batch intentionally gathers/intermediates data and a follow-up data batch/evaluation is expected before final answer."
    },
    "questions": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id": {"type":"string"},
          "question": {"type":"string"},
          "suggestions": {"type":"array", "items":{"type":"string"}}
        }
      },
      "description": "Short user questions when status=needs_clarification."
    },
    "block_reason": {
      "type": "string",
      "description": "Reason when status=blocked."
    }
  },
  "required": ["status", "output_contract"]
}`),
}

var dataTaskEvaluationTool = llm.ToolSchema{
	Name:        "emit_data_task_evaluation",
	Description: "Emit a typed data-workflow goal evaluation. This tool never executes code.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "status": {
      "type": "string",
      "enum": ["complete", "continue_data", "needs_clarification", "blocked", "budget_exhausted", "partial_answer_possible"],
      "description": "complete when the user's data goal and output contract are satisfied; continue_data when another bounded data plan should run; needs_clarification only for user-owned missing business inputs; blocked for capability/policy barriers; budget_exhausted/partial_answer_possible when useful progress exists but no more safe bounded work should run."
    },
    "reason": {"type": "string"},
    "confidence": {"type": "string"},
    "missing_inputs": {
      "type": "array",
      "items": {"type": "string"},
      "description": "User-owned missing inputs only. Do not list facts that can be computed from available local data files."
    }
  },
  "required": ["status", "reason"]
}`),
}

const dataTaskPlannerSystemPrompt = `You are a read-only data task planner.

Convert the user's request into a typed data task plan for deterministic local data processing.

Hard rules:
- Emit only emit_data_task_plan. Do not write prose.
- This lane is for structured/semi-structured data computation: CSV/TSV/JSON/JSONL/text cleaning, filtering, joining, aggregation, item-level decisions, and strict data-shaped output.
- This is not source-code analysis, log/trace diagnosis, write-mode code editing, or computer operation.
- Use candidate files when possible. If no relevant local data file is available or the user's business rule is genuinely missing, ask a short clarification question.
- Do not make the model hand-calculate the final answer. The script must compute it.
- The final result object must include answer as a string and output_contract as an object.
- Treat each plan as one bounded data batch. For multi-step data work, set goal/success_criteria and continue_after=true, then explain next_batch and why_this_batch.
- If the user requests JSON-only, CSV-only, a single line, only a file path, only a code block, or a Markdown table, encode that in output_contract. Do not hard-code one output style.
- If explanation_allowed=false, answer must be exactly the requested final payload, with no explanatory prefix or suffix.
- Treat candidate files as an objective material inventory: path, kind, size, headers, row/line counts, and small samples. The system does not know the business role of a file. You must decide, from the user goal, which materials are required, optional, or irrelevant.
- Candidate files may include materials whose semantic content is not directly readable by the Python runner. Do not put paths with extraction_status=needs_text_extraction or extraction_status=related_text_available directly into input_paths. If such a material is required and text_evidence_paths are available, use the relevant text evidence path as the script input and keep the original non-text path in coverage_contract.required_materials only when its content must be covered. If no text evidence is available, return blocked/needs_clarification or wait for the material extraction layer instead of silently ignoring it.
- extraction_status is objective inventory metadata. "text_ready" and "extracted_text" mean the runner can read the material; "related_text_available" means the material has candidate text evidence; "needs_text_extraction" means semantic content is not yet available to the deterministic runner.
- Fill coverage_contract.required_materials with every material that must be covered to make the result trustworthy. This is generic: use it for rules/instructions, main datasets, reference materials, linked evidence, examples, schemas, or any other task-specific source. Do not hard-code one domain or file naming pattern.
- For each required material choose usage_mode. Default/empty is script_consumed. Use script_consumed when the script must directly read that material. Use text_evidence_consumed when an original material is covered by an extracted or related text evidence file; set text_evidence_path and read that path in the script. Use planner_distilled only when the material content has already been distilled into typed validation_rules/constraints for this batch; include distilled_notes. Use reference_only in optional_materials, not as a blocking requirement.
- A script_consumed material is consumed only when the script reads it through a provided helper such as csv_rows, tsv_rows, json_load, jsonl_rows, read_text, or safe open, and then uses the returned content in computation, validation, rule coverage, contribution/entity records, reconcile, or audit summary. Listing a path in input_paths or required_materials is not enough.
- If you mark a rule/instruction/example/schema/text document as required with script_consumed, read it with read_text(path) and use its content to drive rule_coverage, validation, parsing choices, or audit notes. Do not add dummy comments or assertions solely to satisfy consumption.
- Repair/continuation plans must not silently drop previously required materials. If a material is no longer required, replace the coverage_contract and explain the structural reason in validation_rules or why_this_batch.
- For filtering/joining/aggregation/item-level decisions, set coverage_contract.decision_records_required=true and emit result.rows with source/material locators and decisions. The final answer can still be a strict single line if the user requested that; decision records are for system validation and handoff.
- Choose validation fields from the task shape, not from a fixed domain. Simple sums/counts/rankings usually need contribution_ledger_required=true and reconcile_required=true. Cleaning/filtering usually also needs rule_coverage_required=true. Joins, name/ID/category normalization, or cross-material linking usually also need entity_resolution_required=true. Pure summary/extraction may only need material coverage and output_contract.
- If coverage_contract.rule_coverage_required=true, emit result.rule_coverage records with rule_id, rule_text, status, evidence_refs, and notes. These are generic rule records.
- Rule coverage records must be supported: each rule needs rule_id/rule_text, status, and either notes, evidence_refs, or linked rule_refs from rows/contributions/entity_resolutions.
- If coverage_contract.contribution_ledger_required=true, emit result.contributions records with item_id, source, source_locator, group_key, metric, value, operation, reason, evidence_refs, and rule_refs when a contribution is governed by specific rules. These explain how task items contribute to totals, counts, groups, ranks, or final output elements. Prefer add_contribution(...) so the runner owns the JSON shape. Use operation="add" or "sum" for positive numeric totals, "count" for item counts, and "subtract" for negative adjustments.
- If coverage_contract.entity_resolution_required=true, emit result.entity_resolutions records with source_value, canonical_id, canonical_label, status, candidates, evidence_refs, reason, and rule_refs when the mapping is governed by specific rules. Use this for any source-to-canonical mapping, including unresolved or ambiguous values. Prefer add_resolution(...); if candidates are present, each candidate should be an object with id/label/evidence/confidence.
- If coverage_contract.reconcile_required=true, emit result.reconcile with status="pass", expected_answer and/or actual_answer, and reconcile.groups. The system recomputes numeric group totals from result.contributions when possible; do not mark pass unless the output and contribution ledger reconcile.
- Use the canonical ledger field names exactly. Task-specific aliases may be useful inside normalized_fields or reason text, but they do not replace item_id/group_key/metric/value/source_locator in result.contributions or group_key/metric/expected/actual in result.reconcile.groups. Keep group_key as dimensions only; put the measure name in metric so the same metric is not encoded twice.
- Script sandbox: prefer the provided helpers:
  csv_rows(path), tsv_rows(path), json_load(path), jsonl_rows(path), read_text(path), parse_money(value), Decimal, re, add_decision(...), add_rule_coverage(...), add_contribution(...), add_resolution(...), emit_result(...), emit(obj).
- Do not redefine provided helper names such as csv_rows, tsv_rows, json_load, jsonl_rows, read_text, parse_money, add_decision, add_rule_coverage, add_contribution, add_resolution, emit_result, emit, or open. Use them directly.
- Candidate table metadata uses structured JSON fields. headers_json and sample_rows_json describe columns/rows; delimiter shows the real file delimiter. Do not infer a pipe delimiter from display punctuation.
- You may also import common data standard libraries only: csv, json, decimal, re, math, statistics, collections, datetime, itertools, functools, operator, string, textwrap, base64, binascii, hashlib, unicodedata, fractions, calendar.
- open(path) is read-only and works only for files listed in input_paths. Never write files, run shell commands, use network/process libraries, access os/sys/pathlib/shutil, or use dynamic eval/exec/compile.
- print(...) is allowed for small debug output only. It is not the final answer channel; the script must still call emit(obj) or set result.
- If the requested task needs file writes, network, process execution, installation, deletion, remote access, or other side effects, do not smuggle that into this data script. Return status=blocked with a concise block_reason explaining that the task requires the operation pipeline and its risk/approval policy.
- input_paths must list every path the script reads. The runner copies only those paths into an isolated temporary workspace.
- Emit item-level decision records in rows when the task includes filtering/joining/aggregation decisions. A record may represent a table row, JSON item, text span, page, image region, or other task-specific item. Do not misuse member_set/count semantics; numeric totals belong in answer/metrics/audit, not in member count fields.
`

const dataTaskEvaluationSystemPrompt = `You are a data workflow evaluator.

Decide whether the current read-only data-processing observations satisfy the user's original data goal, or whether another bounded data batch is needed.

Hard rules:
- Emit only emit_data_task_evaluation. Do not write prose.
- This is data-lane evaluation only. Do not route to source-code analysis, trace analysis, write-mode code editing, or command operation.
- Existing result.answer is the authoritative computed result for its batch; do not recalculate manually.
- If the user's strict output contract and success criteria are satisfied, emit status=complete.
- If the result is a deliberate intermediate batch, contract warnings remain, required item decisions/audit are missing, decision samples show empty/missing parsed fields, or success criteria are not covered but can still be computed from available data files, emit status=continue_data.
- If the plan required rule_coverage, contributions, entity_resolutions, or reconcile, those typed result ledgers must be present and reconcile.status must be pass before status=complete.
- Do not mark complete when reconcile failed or when contribution/entity/rule samples show missing values that are needed for the user goal.
- Do not mark a batch complete just because the script exited successfully. The result must be supported by consumed materials, decision samples, metrics, and the declared coverage contract.
- Ask for clarification only for user-owned missing business rules or missing data files that cannot be safely discovered/read from the candidate data files.
- If the task requires side effects such as network/process/file write/install/delete, emit blocked so the operation lane can own risk/approval.
- If useful progress exists but bounded workflow budget is exhausted or no safe continuation exists, emit budget_exhausted or partial_answer_possible.
- Match status to a typed enum exactly.`

const dataTaskStructuredToolRepairSystemPrompt = `You repair one malformed structured tool call for the read-only data lane.

Hard rules:
- Emit exactly one tool call using the required tool.
- Preserve the original data-task intent, output contract, and status unless the parse failure is directly caused by those fields.
- Return a single valid JSON object matching the tool schema. Arrays must be JSON arrays, objects must be JSON objects, and booleans/numbers must be typed values.
- Do not add prose, markdown, comments, code fences, or extra wrapper keys.
- Use only the compact context provided here; do not invent source-code, trace, log, or external-tool evidence.`

func (p *llmDataTaskPlanner) PlanDataTask(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile) (dataquery.TaskPlan, error) {
	return p.planDataTask(ctx, "data_task_planner", dataTaskPlannerPrompt(userLine, repoRoot, policy, candidates))
}

func (p *llmDataTaskPlanner) RepairDataTask(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, previous dataquery.TaskPlan, executionError string) (dataquery.TaskPlan, error) {
	return p.planDataTask(ctx, "data_task_repair_planner", dataTaskRepairPrompt(userLine, repoRoot, policy, candidates, previous, executionError))
}

func (p *llmDataTaskPlanner) ContinueDataTask(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, records []dataTaskWorkflowRecord) (dataquery.TaskPlan, error) {
	return p.planDataTask(ctx, "data_task_continuation_planner", dataTaskContinuationPrompt(userLine, repoRoot, policy, candidates, records))
}

func (p *llmDataTaskPlanner) EvaluateDataTask(ctx context.Context, userLine string, records []dataTaskWorkflowRecord, lang string) (dataquery.Evaluation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if p == nil || p.adapter == nil {
		return dataquery.Evaluation{}, fmt.Errorf("data task evaluator is not configured")
	}
	resp, err := p.adapter.Chat(ctx,
		[]llm.Message{
			{Role: "system", Content: dataTaskEvaluationSystemPrompt},
			{Role: "user", Content: dataTaskEvaluationPrompt(userLine, records, lang)},
		},
		[]llm.ToolSchema{dataTaskEvaluationTool},
		llm.ChatOptions{ToolChoice: "required"},
	)
	p.lastTrace = traceFromLLMResponse("data_task_evaluator", resp)
	if err != nil {
		return dataquery.Evaluation{}, err
	}
	if len(resp.ToolCalls) == 0 {
		return dataquery.Evaluation{}, fmt.Errorf("data task evaluator returned no tool_call")
	}
	call := resp.ToolCalls[0]
	if call.Name != dataTaskEvaluationTool.Name {
		return dataquery.Evaluation{}, fmt.Errorf("data task evaluator returned unexpected tool %q", call.Name)
	}
	var parsed dataTaskEvaluationDraft
	if err := unmarshalReplStructuredToolParams(dataTaskEvaluationTool, call.Params, &parsed, "data task evaluator"); err != nil {
		parseErr := err
		raw, repairErr := p.repairDataTaskStructuredToolParams(ctx, dataTaskEvaluationTool, err, dataTaskStructuredToolRepairRequest{
			Scope:   "data task evaluator",
			Context: dataTaskEvaluationPrompt(userLine, records, lang),
		})
		if repairErr != nil {
			return dataquery.Evaluation{}, repairErr
		}
		if err := unmarshalReplStructuredToolParams(dataTaskEvaluationTool, raw, &parsed, "data task evaluator"); err != nil {
			return dataquery.Evaluation{}, fmt.Errorf("%w; compact tool-param repair also failed: %v", parseErr, err)
		}
	}
	return parsed.toEvaluation(), nil
}

func (p *llmDataTaskPlanner) planDataTask(ctx context.Context, scope, prompt string) (dataquery.TaskPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if p == nil || p.adapter == nil {
		return dataquery.TaskPlan{}, fmt.Errorf("data task planner is not configured")
	}
	resp, err := p.adapter.Chat(ctx,
		[]llm.Message{
			{Role: "system", Content: dataTaskPlannerSystemPrompt},
			{Role: "user", Content: prompt},
		},
		[]llm.ToolSchema{dataTaskPlanTool},
		llm.ChatOptions{ToolChoice: "required"},
	)
	p.lastTrace = traceFromLLMResponse(scope, resp)
	if err != nil {
		return dataquery.TaskPlan{}, err
	}
	if len(resp.ToolCalls) == 0 {
		return dataquery.TaskPlan{}, fmt.Errorf("data task planner returned no tool_call")
	}
	call := resp.ToolCalls[0]
	if call.Name != dataTaskPlanTool.Name {
		return dataquery.TaskPlan{}, fmt.Errorf("data task planner returned unexpected tool %q", call.Name)
	}
	var parsed dataTaskPlanDraft
	if err := unmarshalReplStructuredToolParams(dataTaskPlanTool, call.Params, &parsed, "data task planner"); err != nil {
		parseErr := err
		raw, repairErr := p.repairDataTaskStructuredToolParams(ctx, dataTaskPlanTool, err, dataTaskStructuredToolRepairRequest{
			Scope:   scope,
			Context: prompt,
		})
		if repairErr != nil {
			return dataquery.TaskPlan{}, repairErr
		}
		if err := unmarshalReplStructuredToolParams(dataTaskPlanTool, raw, &parsed, "data task planner"); err != nil {
			return dataquery.TaskPlan{}, fmt.Errorf("%w; compact tool-param repair also failed: %v", parseErr, err)
		}
	}
	return parsed.toPlan(), nil
}

type dataTaskStructuredToolRepairRequest struct {
	Scope   string
	Context string
}

func (p *llmDataTaskPlanner) repairDataTaskStructuredToolParams(ctx context.Context, tool llm.ToolSchema, parseErr error, req dataTaskStructuredToolRepairRequest) (json.RawMessage, error) {
	var paramErr *replStructuredToolParamError
	if !errors.As(parseErr, &paramErr) {
		return nil, parseErr
	}
	if p == nil || p.adapter == nil {
		return nil, parseErr
	}
	scope := firstNonEmptyString(strings.TrimSpace(req.Scope), strings.TrimSpace(paramErr.Scope), "data structured tool")
	logging.Warning("[repl/data] structured tool params invalid tool=%s scope=%s raw_bytes=%d; attempting compact repair",
		firstNonEmptyString(tool.Name, paramErr.ToolName), scope, paramErr.RawLen)
	resp, err := p.adapter.Chat(ctx,
		[]llm.Message{
			{Role: "system", Content: dataTaskStructuredToolRepairSystemPrompt},
			{Role: "user", Content: dataTaskStructuredToolRepairPrompt(tool, paramErr, req)},
		},
		[]llm.ToolSchema{tool},
		llm.ChatOptions{ToolChoice: "required"},
	)
	p.lastTrace = traceFromLLMResponse("data_task_structured_tool_repair", resp)
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

func dataTaskStructuredToolRepairPrompt(tool llm.ToolSchema, paramErr *replStructuredToolParamError, req dataTaskStructuredToolRepairRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## repair_scope\n%s\n\n", firstNonEmptyString(strings.TrimSpace(req.Scope), strings.TrimSpace(paramErr.Scope), "data structured tool"))
	fmt.Fprintf(&b, "## required_tool\n%s\n\n", tool.Name)
	fmt.Fprintf(&b, "## parse_failure\nraw_bytes=%d\nerror=%s\nhint=%s\n\n",
		paramErr.RawLen,
		oneLineClamp(fmt.Sprint(paramErr.Err), 400),
		oneLineClamp(paramErr.Hint, 400))
	b.WriteString("## schema\n")
	b.WriteString(oneLineClamp(tool.Description, 600))
	b.WriteString("\n")
	b.WriteString(clampMultilineForRepair(string(tool.Parameters), 6000))
	b.WriteString("\n\n## compact_data_context\n")
	b.WriteString(clampMultilineForRepair(req.Context, 5000))
	return strings.TrimSpace(b.String())
}

func (p *llmDataTaskPlanner) LastReplLLMTrace() replLLMCallTrace {
	if p == nil {
		return replLLMCallTrace{}
	}
	return p.lastTrace
}

func dataTaskPlannerPrompt(userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## user_request\n%s\n\n", strings.TrimSpace(userLine))
	fmt.Fprintf(&b, "## workspace_root\n%s\n\n", strings.TrimSpace(repoRoot))
	fmt.Fprintf(&b, "## route_policy\nroute=%s data_task_kind=%s operation=%s source=%s confidence=%.2f\n\n",
		policy.Route, policy.DataTaskKind, policy.Operation, policy.Source, policy.Confidence)
	b.WriteString("## candidate_data_files\n")
	appendCandidateDataFiles(&b, candidates)
	return strings.TrimSpace(b.String())
}

func dataTaskRepairPrompt(userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, previous dataquery.TaskPlan, executionError string) string {
	var b strings.Builder
	violation := dataquery.ClassifyExecutionError(executionError)
	fmt.Fprintf(&b, "## repair_goal\nThe previous data task script failed at execution time. Emit a corrected emit_data_task_plan.\n\n")
	fmt.Fprintf(&b, "## user_request\n%s\n\n", strings.TrimSpace(userLine))
	fmt.Fprintf(&b, "## workspace_root\n%s\n\n", strings.TrimSpace(repoRoot))
	fmt.Fprintf(&b, "## route_policy\nroute=%s data_task_kind=%s operation=%s source=%s confidence=%.2f\n\n",
		policy.Route, policy.DataTaskKind, policy.Operation, policy.Source, policy.Confidence)
	fmt.Fprintf(&b, "## execution_error\n%s\n\n", strings.TrimSpace(executionError))
	violationJSON, _ := json.MarshalIndent(violation, "", "  ")
	fmt.Fprintf(&b, "## typed_repair_locus\n%s\n\n", string(violationJSON))
	prevJSON, _ := json.MarshalIndent(compactDataTaskRepairContext(previous), "", "  ")
	fmt.Fprintf(&b, "## previous_plan_compact_json\n%s\n\n", string(prevJSON))
	b.WriteString("## repair_rules\n")
	b.WriteString("- Fix the script deterministically; preserve the user's requested output contract unless it was the direct cause of the failure.\n")
	b.WriteString("- Prefer correcting code/import/helper usage over asking the user. Ask only when a user-owned business rule or missing input is genuinely required.\n")
	b.WriteString("- Keep input_paths limited to candidate files. List every file the script reads.\n")
	b.WriteString("- Preserve the previous coverage_contract unless the failed script proves a structural reason to replace it. Required materials must keep a verifiable usage_mode.\n")
	b.WriteString("- If the error says a required material was not consumed, repair one of three ways: read the material with a helper and keep usage_mode=script_consumed; if a text evidence file covers the original material, set usage_mode=text_evidence_consumed, text_evidence_path, and read that text evidence path; or if the material was already distilled into typed validation_rules/constraints for this bounded batch, set usage_mode=planner_distilled with concrete distilled_notes. Do not use reference_only for a blocking required material.\n")
	b.WriteString("- For required rule/instruction/example/schema/text materials with usage_mode=script_consumed, read_text(path) is usually the right helper. Use the content to drive the current task's generic validation or audit records; do not add dummy comments or assertions solely to satisfy consumption.\n")
	b.WriteString("- If coverage_contract.decision_records_required=true, emit result.rows. If result.rows was missing, repair by adding generic item-level decision records rather than changing the user's final output format.\n")
	b.WriteString("- If a required validation ledger is missing or reconcile failed, repair by adding/fixing the generic rule_coverage, contributions, entity_resolutions, or reconcile fields and correcting the computation. Do not remove required validation flags to bypass the contract unless the previous contract was structurally wrong for the user goal; explain that structural reason in validation_rules or why_this_batch.\n")
	b.WriteString("- Prefer runner ledger helpers for repaired structures: add_decision(...), add_rule_coverage(...), add_contribution(...), add_resolution(...), and emit_result(...). These helpers reduce JSON-shape drift.\n")
	b.WriteString("- Use canonical ledger field names exactly. Task-specific aliases do not satisfy contribution/reconcile validation: contributions need item_id/source/source_locator/evidence_refs plus group_key/metric/value/operation/reason as needed; reconcile.groups need group_key/metric and expected/actual values.\n")
	b.WriteString("- Contribution operation values are canonicalized, but use operation=\"add\" or \"sum\" for positive numeric totals, \"count\" for counts, and \"subtract\" for negative adjustments. Keep group_key as dimensions only and metric as the measure name; do not encode metric inside group_key.\n")
	b.WriteString("- Rule coverage records must be supported by notes, evidence_refs, or rule_refs on rows/contributions/entity_resolutions. Resolved entity_resolutions need canonical_id/canonical_label; unresolved or ambiguous mappings need reason, candidates, or evidence_refs. Candidate entries should be objects with id/label/evidence/confidence, not task-specific aliases.\n")
	b.WriteString("- The script must call emit(obj) or set result. Debug print is allowed only in small amounts and is not the final answer.\n")
	b.WriteString("- If the repair requires side effects such as network/process/file write/install/delete, return status=blocked instead of embedding those actions in the script; the operation pipeline owns risk and approval.\n\n")
	b.WriteString("## candidate_data_files\n")
	appendCandidateDataFiles(&b, candidates)
	return strings.TrimSpace(b.String())
}

type dataTaskRepairContext struct {
	Status              string                     `json:"status,omitempty"`
	InputPaths          []string                   `json:"input_paths,omitempty"`
	OutputContract      dataquery.OutputContract   `json:"output_contract,omitempty"`
	CoverageContract    dataquery.CoverageContract `json:"coverage_contract,omitempty"`
	Goal                string                     `json:"goal,omitempty"`
	KnownConstraints    []string                   `json:"known_constraints,omitempty"`
	MissingObservations []string                   `json:"missing_observations,omitempty"`
	SuccessCriteria     []string                   `json:"success_criteria,omitempty"`
	NextBatch           string                     `json:"next_batch,omitempty"`
	WhyThisBatch        string                     `json:"why_this_batch,omitempty"`
	ContinueAfter       bool                       `json:"continue_after,omitempty"`
	ScriptPreview       string                     `json:"script_preview,omitempty"`
}

func compactDataTaskRepairContext(plan dataquery.TaskPlan) dataTaskRepairContext {
	return dataTaskRepairContext{
		Status:              strings.TrimSpace(plan.Status),
		InputPaths:          append([]string(nil), plan.InputPaths...),
		OutputContract:      plan.OutputContract,
		CoverageContract:    plan.CoverageContract,
		Goal:                strings.TrimSpace(plan.Goal),
		KnownConstraints:    append([]string(nil), plan.KnownConstraints...),
		MissingObservations: append([]string(nil), plan.MissingObservations...),
		SuccessCriteria:     append([]string(nil), plan.SuccessCriteria...),
		NextBatch:           strings.TrimSpace(plan.NextBatch),
		WhyThisBatch:        strings.TrimSpace(plan.WhyThisBatch),
		ContinueAfter:       plan.ContinueAfter,
		ScriptPreview:       clampDataTaskWorkflowText(plan.Script, 6000),
	}
}

func dataTaskContinuationPrompt(userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, records []dataTaskWorkflowRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## continuation_goal\nThe previous data batch executed. Emit the next bounded emit_data_task_plan, or status=complete/blocked/needs_clarification when appropriate.\n\n")
	fmt.Fprintf(&b, "## user_request\n%s\n\n", strings.TrimSpace(userLine))
	fmt.Fprintf(&b, "## workspace_root\n%s\n\n", strings.TrimSpace(repoRoot))
	fmt.Fprintf(&b, "## route_policy\nroute=%s data_task_kind=%s operation=%s source=%s confidence=%.2f\n\n",
		policy.Route, policy.DataTaskKind, policy.Operation, policy.Source, policy.Confidence)
	b.WriteString("## previous_data_rounds\n")
	b.WriteString(renderDataTaskRecordsForPrompt(records))
	b.WriteString("\n\n## continuation_rules\n")
	b.WriteString("- If previous results satisfy the user's data goal and output contract, emit status=complete with a short block_reason/reason and no script.\n")
	b.WriteString("- If more data calculation is needed, emit status=ready with only the next bounded script batch.\n")
	b.WriteString("- Continue plans must preserve still-relevant coverage_contract materials; do not drop required materials just because a prior batch produced a partial answer.\n")
	b.WriteString("- Continue plans must preserve still-relevant required validation flags. If a prior result has missing rule coverage, missing contributions, missing entity resolutions, or failed reconcile, the next batch should repair the computation or emit a corrected bounded result.\n")
	b.WriteString("- Do not repeat a failed script unchanged. Do not re-run successful intermediate batches unless a fresh value is required.\n")
	b.WriteString("- Ask the user only for user-owned business inputs. Keep computing when the missing fact can be derived from available local data files.\n")
	b.WriteString("- Side effects belong to the operation lane; return blocked if they are required.\n\n")
	b.WriteString("## candidate_data_files\n")
	appendCandidateDataFiles(&b, candidates)
	return strings.TrimSpace(b.String())
}

func dataTaskEvaluationPrompt(userLine string, records []dataTaskWorkflowRecord, lang string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## language\n%s\n\n", strings.TrimSpace(lang))
	fmt.Fprintf(&b, "## user_request\n%s\n\n", strings.TrimSpace(userLine))
	b.WriteString("## data_workflow_rounds\n")
	b.WriteString(renderDataTaskRecordsForPrompt(records))
	return strings.TrimSpace(b.String())
}

func appendCandidateDataFiles(b *strings.Builder, candidates []dataquery.CandidateFile) {
	if len(candidates) == 0 {
		b.WriteString("(none discovered)\n")
		return
	}
	limit := len(candidates)
	if limit > 120 {
		limit = 120
	}
	for i := 0; i < limit; i++ {
		f := candidates[i]
		fmt.Fprintf(b, "- path=%s kind=%s size=%d", f.Path, f.Kind, f.Size)
		if strings.TrimSpace(f.ExtractionStatus) != "" {
			fmt.Fprintf(b, " extraction_status=%s", marshalInlineJSON(f.ExtractionStatus))
		}
		if len(f.TextEvidencePaths) > 0 {
			fmt.Fprintf(b, " text_evidence_paths_json=%s", marshalInlineJSON(f.TextEvidencePaths))
		}
		if f.Lines > 0 {
			fmt.Fprintf(b, " lines=%d", f.Lines)
		}
		if strings.TrimSpace(f.Delimiter) != "" {
			fmt.Fprintf(b, " delimiter=%s", marshalInlineJSON(f.Delimiter))
		}
		if len(f.Headers) > 0 {
			fmt.Fprintf(b, " headers_json=%s", marshalInlineJSON(f.Headers))
		}
		if len(f.SampleRows) > 0 {
			fmt.Fprintf(b, " sample_rows_json=%s", marshalInlineJSON(f.SampleRows))
		}
		if len(f.Sample) > 0 {
			fmt.Fprintf(b, " sample_lines_json=%s", marshalInlineJSON(f.Sample))
		}
		if strings.TrimSpace(f.InspectError) != "" {
			fmt.Fprintf(b, " inspect_error=%q", oneLineClamp(f.InspectError, 160))
		}
		b.WriteString("\n")
	}
	if len(candidates) > limit {
		fmt.Fprintf(b, "- ... %d more candidate file(s) omitted\n", len(candidates)-limit)
	}
}

func marshalInlineJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(raw)
}

type dataTaskPlanDraft struct {
	Status              flexiblePolicyString     `json:"status"`
	InputPaths          flexiblePolicyStringList `json:"input_paths"`
	OutputContract      dataTaskOutputDraft      `json:"output_contract"`
	CoverageContract    dataTaskCoverageDraft    `json:"coverage_contract"`
	Script              flexiblePolicyString     `json:"script"`
	Questions           []dataTaskQuestionDraft  `json:"questions"`
	BlockReason         flexiblePolicyString     `json:"block_reason"`
	Goal                flexiblePolicyString     `json:"goal"`
	KnownConstraints    flexiblePolicyStringList `json:"known_constraints"`
	MissingObservations flexiblePolicyStringList `json:"missing_observations"`
	SuccessCriteria     flexiblePolicyStringList `json:"success_criteria"`
	NextBatch           flexiblePolicyString     `json:"next_batch"`
	WhyThisBatch        flexiblePolicyString     `json:"why_this_batch"`
	ContinueAfter       flexiblePolicyBool       `json:"continue_after"`
}

type dataTaskOutputDraft struct {
	Format             flexiblePolicyString `json:"format"`
	ExplanationAllowed flexiblePolicyBool   `json:"explanation_allowed"`
	Delimiter          flexiblePolicyString `json:"delimiter"`
}

type dataTaskQuestionDraft struct {
	ID          flexiblePolicyString     `json:"id"`
	Question    flexiblePolicyString     `json:"question"`
	Suggestions flexiblePolicyStringList `json:"suggestions"`
}

type dataTaskCoverageDraft struct {
	RequiredMaterials          []dataTaskCoverageMaterialDraft `json:"required_materials"`
	OptionalMaterials          []dataTaskCoverageMaterialDraft `json:"optional_materials"`
	ValidationRules            flexiblePolicyStringList        `json:"validation_rules"`
	DecisionRecordsRequired    flexiblePolicyBool              `json:"decision_records_required"`
	RuleCoverageRequired       flexiblePolicyBool              `json:"rule_coverage_required"`
	ContributionLedgerRequired flexiblePolicyBool              `json:"contribution_ledger_required"`
	EntityResolutionRequired   flexiblePolicyBool              `json:"entity_resolution_required"`
	ReconcileRequired          flexiblePolicyBool              `json:"reconcile_required"`
}

type dataTaskCoverageMaterialDraft struct {
	ID               flexiblePolicyString     `json:"id"`
	Path             flexiblePolicyString     `json:"path"`
	Purpose          flexiblePolicyString     `json:"purpose"`
	UsageMode        flexiblePolicyString     `json:"usage_mode"`
	TextEvidencePath flexiblePolicyString     `json:"text_evidence_path"`
	DistilledNotes   flexiblePolicyStringList `json:"distilled_notes"`
	Required         flexiblePolicyBool       `json:"required"`
}

type dataTaskEvaluationDraft struct {
	Status        flexiblePolicyString     `json:"status"`
	Reason        flexiblePolicyString     `json:"reason"`
	Confidence    flexiblePolicyString     `json:"confidence"`
	MissingInputs flexiblePolicyStringList `json:"missing_inputs"`
}

func (d dataTaskEvaluationDraft) toEvaluation() dataquery.Evaluation {
	return dataquery.Evaluation{
		Status:        dataquery.NormalizeEvaluationStatus(string(d.Status)),
		Reason:        strings.TrimSpace(string(d.Reason)),
		Confidence:    strings.TrimSpace(string(d.Confidence)),
		MissingInputs: cleanPolicyStringList([]string(d.MissingInputs)),
	}
}

func (d dataTaskPlanDraft) toPlan() dataquery.TaskPlan {
	qs := make([]dataquery.Question, 0, len(d.Questions))
	for _, q := range d.Questions {
		if strings.TrimSpace(string(q.Question)) == "" {
			continue
		}
		qs = append(qs, dataquery.Question{
			ID:          strings.TrimSpace(string(q.ID)),
			Question:    strings.TrimSpace(string(q.Question)),
			Suggestions: []string(q.Suggestions),
		})
	}
	return dataquery.TaskPlan{
		Status:     normalizeDataTaskPlanStatus(string(d.Status)),
		InputPaths: cleanPolicyStringList([]string(d.InputPaths)),
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputFormat(strings.TrimSpace(string(d.OutputContract.Format))),
			ExplanationAllowed: bool(d.OutputContract.ExplanationAllowed),
			Delimiter:          strings.TrimSpace(string(d.OutputContract.Delimiter)),
		}.Normalize(),
		CoverageContract:    d.CoverageContract.toCoverageContract(),
		Script:              strings.TrimSpace(string(d.Script)),
		Questions:           qs,
		BlockReason:         strings.TrimSpace(string(d.BlockReason)),
		Goal:                strings.TrimSpace(string(d.Goal)),
		KnownConstraints:    cleanPolicyStringList([]string(d.KnownConstraints)),
		MissingObservations: cleanPolicyStringList([]string(d.MissingObservations)),
		SuccessCriteria:     cleanPolicyStringList([]string(d.SuccessCriteria)),
		NextBatch:           strings.TrimSpace(string(d.NextBatch)),
		WhyThisBatch:        strings.TrimSpace(string(d.WhyThisBatch)),
		ContinueAfter:       bool(d.ContinueAfter),
	}
}

func (d dataTaskCoverageDraft) toCoverageContract() dataquery.CoverageContract {
	return dataquery.CoverageContract{
		RequiredMaterials:          dataTaskCoverageMaterialsToPlan(d.RequiredMaterials, true),
		OptionalMaterials:          dataTaskCoverageMaterialsToPlan(d.OptionalMaterials, false),
		ValidationRules:            cleanPolicyStringList([]string(d.ValidationRules)),
		DecisionRecordsRequired:    bool(d.DecisionRecordsRequired),
		RuleCoverageRequired:       bool(d.RuleCoverageRequired),
		ContributionLedgerRequired: bool(d.ContributionLedgerRequired),
		EntityResolutionRequired:   bool(d.EntityResolutionRequired),
		ReconcileRequired:          bool(d.ReconcileRequired),
	}
}

func dataTaskCoverageMaterialsToPlan(in []dataTaskCoverageMaterialDraft, forceRequired bool) []dataquery.CoverageMaterial {
	out := make([]dataquery.CoverageMaterial, 0, len(in))
	seen := map[string]bool{}
	for _, m := range in {
		path := strings.TrimSpace(string(m.Path))
		purpose := strings.TrimSpace(string(m.Purpose))
		id := strings.TrimSpace(string(m.ID))
		if path == "" && purpose == "" && id == "" {
			continue
		}
		key := path + "\x00" + id + "\x00" + purpose
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, dataquery.CoverageMaterial{
			ID:               id,
			Path:             path,
			Purpose:          purpose,
			UsageMode:        dataquery.CoverageMaterialUseMode(strings.TrimSpace(string(m.UsageMode))),
			TextEvidencePath: strings.TrimSpace(string(m.TextEvidencePath)),
			DistilledNotes:   cleanPolicyStringList([]string(m.DistilledNotes)),
			Required:         forceRequired || bool(m.Required),
		})
	}
	return out
}

func normalizeDataTaskPlanStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "ready", "needs_clarification", "blocked", "complete", "budget_exhausted", "partial_answer_possible":
		return status
	case "":
		return "ready"
	default:
		return status
	}
}
