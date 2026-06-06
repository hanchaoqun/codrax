package repl

import (
	"bytes"
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

type DataTaskResultPatchPlanner interface {
	ProposeDataResultPatch(ctx context.Context, userLine string, previous dataquery.TaskPlan, partial dataquery.Result, violations []dataquery.DataTaskViolation, records []dataTaskWorkflowRecord, lang string) (dataquery.DataResultPatchPlan, error)
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
          "description": "true when the result must include result.reconcile with status=pass and reconcile.groups. The system recomputes numeric group totals from target result.contributions when available and rejects mismatches. Auxiliary/audit contributions may exist for material coverage or diagnostics but do not replace target metric reconciliation."
        }
      },
      "description": "Generic material coverage contract. The model decides material purpose from the user goal and objective inventory; the system only verifies declared required materials are input and actually consumed. Listing a file in input_paths is not consumption; the script must read it through the provided helpers and use it for the result, validation, or audit."
    },
    "actions": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id": {"type":"string"},
          "kind": {"type":"string", "enum":["material_inventory","inspect_material","extract_records","derive_rules","derive_fields","normalize_entities","enrich_records","join_records","compute_contributions","reconcile_artifacts","custom_transform"]},
          "purpose": {"type":"string"},
          "input_paths": {"type":"array", "items":{"type":"string"}},
          "output_artifact": {"type":"string"},
          "script": {"type":"string"},
          "params": {"type":"object", "additionalProperties":{"type":"string"}},
          "success_criteria": {"type":"array", "items":{"type":"string"}}
        },
        "required": ["kind"]
      },
      "description": "Optional adaptive Data DAG action batch. Prefer actions for multi-step data work. material_inventory discovers objective candidate metadata; inspect_material profiles specific files; extract_records converts selected CSV/TSV/JSON/JSONL/text materials into bounded generic record samples; derive_rules turns task rules/constraints into typed generic rule records; derive_fields materializes generic derived fields such as regex extracts, parsed numbers, lower/upper/trimmed strings, maps, constants, or substrings on an existing record artifact; normalize_entities emits typed source-to-canonical mappings; enrich_records applies mapping/reference records to a base record set and materializes added canonical/derived fields for later joins; join_records creates reusable joined records from two structured inputs using one or more key fields; compute_contributions reads declared local inputs and creates generic contribution records from field/filter params; reconcile_artifacts deterministically reconciles accumulated contributions; custom_transform is the bounded fallback script over declared action input_paths. Each action must be atomic and produce one reusable artifact/result. Do not put one giant end-to-end script in a single custom_transform. A batch may contain at most one scripted custom_transform; split multiple transforms into separate batches or express them as typed actions over reusable artifacts."
    },
    "script": {
      "type": "string",
      "description": "Python calculation body executed by a bounded local data helper. Prefer csv_rows(path), tsv_rows(path), json_load(path), json_records(path), jsonl_rows(path), read_text(path), parse_money(value), Decimal, re, and ledger helpers add_decision(...), add_rule_coverage(...), add_contribution(...), add_resolution(...), emit_result(...). json_records(path) reads array/object/wrapper JSON as a record list and is usually safest for generated action artifacts. You may import only common data standard libraries such as csv/json/decimal/re/math/statistics/collections/datetime/itertools/functools/operator/string/textwrap/base64/hashlib/unicodedata/fractions/calendar, and open(path) only reads declared input files. print(...) is allowed only for small debug output; the final answer must still be emitted. Do not access os/sys/pathlib/shutil, run shell commands, use network/process libraries, dynamic eval/exec, or write files. If the user task truly requires side effects, block this data plan so the operation pipeline can handle risk/approval instead of hiding those actions inside the data script. The script must call emit_result(answer, output_contract=...) or emit({\"answer\": string, \"output_contract\": object, \"audit_summary\": string, \"rows\": [...], \"rule_coverage\": [...], \"contributions\": [...], \"entity_resolutions\": [...], \"reconcile\": {...}}). Include only the ledger fields required by coverage_contract for this task."
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
      "enum": ["complete", "continue_data", "expand_graph", "repair_node", "continue_transform", "needs_clarification", "blocked", "budget_exhausted", "partial_answer_possible"],
      "description": "complete when the user's data goal and output contract are satisfied. expand_graph when more inspection/extraction action nodes are needed. repair_node when a specific prior action/node should be corrected without expanding the whole graph. continue_transform when the next bounded transform over known artifacts should run. continue_data is a legacy broad continue status; prefer the more precise graph statuses. needs_clarification is only for user-owned missing business inputs; blocked is for capability/policy barriers; budget_exhausted/partial_answer_possible when useful progress exists but no more safe bounded work should run."
    },
    "reason": {"type": "string"},
    "confidence": {"type": "string"},
    "missing_inputs": {
      "type": "array",
      "items": {"type": "string"},
      "description": "User-owned missing inputs only. Do not list facts that can be computed from available local data files."
    },
    "action_id": {
      "type": "string",
      "description": "Optional id of the action/node to repair or continue when status is repair_node or continue_transform."
    },
    "action_kind": {
      "type": "string",
      "description": "Optional action kind such as material_inventory, inspect_material, or custom_transform."
    },
    "repair_locus": {
      "type": "string",
      "description": "Optional compact typed locus for repair, such as action id, artifact id, JSON path, or script line. Do not put prose-only reasoning here."
    }
  },
  "required": ["status", "reason"]
}`),
}

var dataTaskResultPatchTool = llm.ToolSchema{
	Name:        "emit_data_result_patch",
	Description: "Emit a typed structural patch plan for an already-computed data result. This tool never changes business semantics and never executes code.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "status": {
      "type": "string",
      "enum": ["patch", "needs_recompute", "blocked"],
      "description": "patch only when the violations can be fixed by replacing existing structural scalar fields without changing business meaning; needs_recompute when the script must recompute; blocked when no safe path exists."
    },
    "patches": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "target": {"type": "string", "enum": ["result"]},
          "op": {"type": "string", "enum": ["replace"]},
          "path": {"type": "string", "description": "JSON pointer to an existing safe structural field, such as /contributions/0/operation, /contributions/0/group_key, /contributions/0/metric, /entity_resolutions/0/status, /reconcile/status only when status is structurally missing, /reconcile/groups/0/group_key, or /reconcile/groups/0/metric."},
          "value": {"description": "Replacement scalar value for the structural field."},
          "reason": {"type": "string"}
        },
        "required": ["target", "op", "path", "value", "reason"]
      }
    },
    "reason": {"type": "string"},
    "confidence": {"type": "string"}
  },
  "required": ["status", "patches", "reason"]
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
- Prefer the adaptive action workflow for non-trivial tasks: emit actions such as material_inventory, inspect_material, extract_records, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, then a small custom_transform only when generic typed actions cannot express the bounded step.
- Do not emit a giant one-shot script for tasks with many materials, uncertain schemas, unknown mapping rules, or likely multi-stage processing. Emit the next atomic action batch, set continue_after=true when more graph expansion is needed, and let the workflow feed real artifacts back before planning the next batch.
- An action is atomic: it should answer one small observation or transformation question. If one action would inspect many unrelated schemas, normalize entities, compute contributions, reconcile, and render output together, split it.
- Action outputs are reusable read-only JSON materials. A later action can list an earlier action id or output_artifact in input_paths and read it with json_load(path) or json_records(path), or let typed actions consume it as structured records. Use this for stepwise DAG convergence instead of rebuilding all raw files in each step. Previous result.artifact_access is the authoritative access catalog for generated artifacts: it lists aliases, json_shape, and access_hint. Follow it before writing script code; for array-shaped artifacts use json_records(alias) or iterate json_load(alias) as a list, and never call .get() on the top-level array. Do not ask the user to clarify the internal shape of a system-generated artifact; inspect/read that artifact or use json_records(path).
- Use material_inventory when you need an objective file/material overview before choosing inputs. Use inspect_material when you need headers/samples/details for specific materials. Use extract_records when you need bounded generic records from selected structured or text materials.
- Use derive_rules when task rules, constraints, examples, or prior distilled notes should become auditable rule_coverage records before calculation. Params can include rules_json (array of {id,text,status,notes,evidence_refs}) or rules (newline text). If later rows/contributions/entity_resolutions will cite rule_refs, give each rule a stable generic ID using "RULE_ID: rule text" or rules_json.id, then cite exactly that ID.
- Use derive_fields when an existing record artifact needs generic derived columns before filtering, joining, or aggregation. Provide input_path or one input_paths item, output_artifact, and params.field_specs_json as an array. Each spec can include source_field, target_field, operation, pattern, group, replacement, value, default, mapping, multiplier, divisor, start, length. Supported operations are copy, trim, lower, upper, regex_extract, regex_replace, parse_number, map, substring, prefix, suffix, year/extract_year, and constant. parse_number extracts a decimal token from any string; optional multiplier/divisor can normalize an objective numeric unit when the user/material defines the conversion. This action is field-level and domain-neutral; it does not decide business rules.
- Use normalize_entities when the current task needs source values mapped to canonical values. It has two generic modes: (1) params.resolutions_json or mappings_json is an array of generic entity_resolutions records; or (2) input_paths plus params.source_field/source_fields/name_fields and canonical_id_field/canonical_label_field derive mappings from structured local materials. Use filters_json/filter_field/filter_value to restrict records when needed. Do not encode domain-specific categories in the schema.
- Use enrich_records when a base record set needs generic canonical/reference fields before it can be joined, filtered, or aggregated. Provide params.base_path plus mapping_specs_json. Each mapping spec can include mapping_path/reference_path, source_field/base_field, mapping_source_fields/reference_fields, mapping_value_field/reference_value_field, target_field, match_mode exact|mapping_contains_source|source_contains_mapping|contains, and optional mapping_filters. If a field can be derived directly from the same record by regex/parse/map/trim, use derive_fields first. This is domain-neutral: it applies mappings from reference/action artifacts and produces a reusable enriched JSON table with source lineage.
- Use join_records when the current task needs a reusable joined table before filtering or aggregation. Provide two input_paths or params.left_path/right_path, plus params.left_fields/right_fields as JSON arrays or comma lists. For composite joins, use arrays of equal length. If a join key is derived or canonicalized, first use enrich_records to materialize that field on the left/right records; do not ask join_records to join on a field that is not present yet. join_records is field-level and domain-neutral; it does not decide business rules.
- Use compute_contributions for generic sums, counts, filters, grouped totals, or contribution ledgers over declared local structured/text inputs. Params are domain-neutral: value_field, group_key_field or group_key, metric, operation, item_id_field, filters_json, rule_refs, max_records, max_contributions.
- Use reconcile_artifacts after compute_contributions when reconcile_required=true or when the final scalar/list must be backed by a deterministic contribution sum.
- Use custom_transform only for bounded deterministic transforms over known input_paths that cannot be represented by the typed actions above.
- A broad custom_transform over many input_paths, required materials, or ledger outputs is not a discovery node. It is allowed only after earlier typed actions or previous rounds have already inspected/derived/normalized/enriched/joined/computed the relevant inputs. If any required input is still unprofiled or a needed field is missing, add inspect_material, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, or extract_records first and set continue_after=true.
- An actions[] batch may contain at most one scripted custom_transform. If the work needs several transforms, emit one batch, let it produce an artifact, then continue with the next batch.
- When actions are present, top-level script must be empty. For custom_transform actions, put the bounded script on that action and list every file it reads in action.input_paths.
- If the user requests JSON-only, CSV-only, a single line, only a file path, only a code block, or a Markdown table, encode that in output_contract. Do not hard-code one output style.
- If explanation_allowed=false, answer must be exactly the requested final payload, with no explanatory prefix or suffix.
- Treat candidate files as an objective material inventory: path, kind, size, headers, row/line counts, and small samples. The system does not know the business role of a file. You must decide, from the user goal, which materials are required, optional, or irrelevant.
- Candidate files may include materials whose semantic content is not directly readable by the Python runner. Do not put paths with extraction_status=needs_text_extraction or extraction_status=related_text_available directly into input_paths. If such a material is required and text_evidence_paths are available, use the relevant text evidence path as the script input and keep the original non-text path in coverage_contract.required_materials only when its content must be covered. If no text evidence is available, return blocked/needs_clarification or wait for the material extraction layer instead of silently ignoring it.
- extraction_status is objective inventory metadata. "text_ready" and "extracted_text" mean the runner can read the material; "related_text_available" means the material has candidate text evidence; "needs_text_extraction" means semantic content is not yet available to the deterministic runner.
- Fill coverage_contract.required_materials with every material that must be covered to make the result trustworthy. This is generic: use it for rules/instructions, main datasets, reference materials, linked evidence, examples, schemas, or any other task-specific source. Do not hard-code one domain or file naming pattern.
- For each required material choose usage_mode. Default/empty is script_consumed. Use script_consumed when the script must directly read that material. Use text_evidence_consumed when an original material is covered by an extracted or related text evidence file; set text_evidence_path and read that path in the script. Use planner_distilled only when the material content has already been distilled into typed validation_rules/constraints for this batch; include distilled_notes. Use reference_only in optional_materials, not as a blocking requirement.
- A script_consumed material is consumed only when the script reads it through a provided helper such as csv_rows, tsv_rows, json_load, json_records, jsonl_rows, read_text, or safe open, and then uses the returned content in computation, validation, rule coverage, contribution/entity records, reconcile, or audit summary. Listing a path in input_paths or required_materials is not enough.
- If you mark a rule/instruction/example/schema/text document as required with script_consumed, read it with read_text(path) and use its content to drive rule_coverage, validation, parsing choices, or audit notes. Do not add dummy comments or assertions solely to satisfy consumption.
- Repair/continuation plans must not silently drop previously required materials. If a material is no longer required, replace the coverage_contract and explain the structural reason in validation_rules or why_this_batch.
- For filtering/joining/aggregation/item-level decisions, set coverage_contract.decision_records_required=true and emit result.rows with source/material locators and decisions. The final answer can still be a strict single line if the user requested that; decision records are for system validation and handoff.
- Choose validation fields from the task shape, not from a fixed domain. Simple sums/counts/rankings usually need contribution_ledger_required=true and reconcile_required=true. Cleaning/filtering usually also needs rule_coverage_required=true. Joins, name/ID/category normalization, or cross-material linking usually also need entity_resolution_required=true. Pure summary/extraction may only need material coverage and output_contract.
- If coverage_contract.rule_coverage_required=true, emit result.rule_coverage records with rule_id, rule_text, status, evidence_refs, and notes. These are generic rule records.
- Rule coverage records must be supported: each rule needs rule_id/rule_text, status, and either notes, evidence_refs, or linked rule_refs from rows/contributions/entity_resolutions.
- If coverage_contract.contribution_ledger_required=true, emit result.contributions records with item_id, source, source_locator, group_key, metric, value, operation, role, reason, evidence_refs, and rule_refs when a contribution is governed by specific rules. These explain how task items contribute to totals, counts, groups, ranks, or final output elements. Prefer add_contribution(...) so the runner owns the JSON shape. Use operation="add" or "sum" for positive numeric totals, "count" for item counts, and "subtract" for negative adjustments. Use role="target" only for atomic sourced items that participate in the final answer; each target contribution must have source, source_locator, or evidence_refs. Do not add aggregate summary rows as target contributions. Put aggregate values in reconcile.groups; use role="audit" or role="intermediate" only for material coverage, samples, diagnostics, summaries, or non-output helper statistics.
- If coverage_contract.entity_resolution_required=true, emit result.entity_resolutions records with source_value, canonical_id, canonical_label, status, candidates, evidence_refs, reason, and rule_refs when the mapping is governed by specific rules. Use this for any source-to-canonical mapping, including unresolved or ambiguous values. Prefer add_resolution(...); if candidates are present, each candidate should be an object with id/label/evidence/confidence.
- If coverage_contract.reconcile_required=true, emit result.reconcile with status="pass", expected_answer and/or actual_answer, and reconcile.groups. The system recomputes numeric group totals from target result.contributions when possible; do not mark pass unless the output and target contribution ledger reconcile. For ordinary group checks, reconcile.groups must use group_key/metric matching target contributions. For a final composite output string or object, a reconcile group may use scope="answer" (or role="answer") with expected/actual equal to the final answer; this is answer-level reconciliation, not a replacement for per-group checks when the task needs them. Audit/intermediate contributions are allowed for traceability but do not satisfy final metric reconciliation.
- Use the canonical ledger field names exactly. Task-specific aliases may be useful inside normalized_fields or reason text, but they do not replace item_id/group_key/metric/value/source_locator in result.contributions or group_key/metric/expected/actual in result.reconcile.groups. Keep group_key as dimensions only; put the measure name in metric so the same metric is not encoded twice.
- Script sandbox: prefer the provided helpers:
  csv_rows(path), tsv_rows(path), json_load(path), json_records(path), jsonl_rows(path), read_text(path), parse_money(value), Decimal, re, add_decision(...), add_rule_coverage(...), add_contribution(...), add_resolution(...), emit_result(...), emit(obj).
- Do not redefine provided helper names such as csv_rows, tsv_rows, json_load, json_records, jsonl_rows, read_text, parse_money, add_decision, add_rule_coverage, add_contribution, add_resolution, emit_result, emit, or open. Use them directly.
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
- If more objective material/profile artifacts are needed before computing, emit status=expand_graph.
- If one specific prior action/node is structurally wrong but the graph shape is still right, emit status=repair_node and fill action_id/action_kind/repair_locus when available.
- If enough artifacts exist and the next step should be a bounded transform over known inputs, emit status=continue_transform.
- If the result is a deliberate intermediate batch, contract warnings remain, required item decisions/audit are missing, decision samples show empty/missing parsed fields, or success criteria are not covered but can still be computed from available data files, emit the most precise continue status above. Use continue_data only when none of expand_graph/repair_node/continue_transform fits.
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

const dataTaskResultPatchSystemPrompt = `You repair only the STRUCTURE of an already-computed read-only data result.

Hard rules:
- Emit only emit_data_result_patch. Do not write prose.
- Patch only schema/shape drift in existing result fields. Do not change the user's final answer, business rules, inclusion/exclusion decisions, canonical entity choices, source values, numeric values, or add/remove business records.
- Allowed patch operations are replace-only scalar edits on existing safe structural fields: contribution operation/group_key/metric, entity resolution status, missing reconcile status only when no differences are present, and reconcile group_key/metric.
- Do not emit remove or move. If a structural duplicate would require remove/move, emit status=needs_recompute with an empty patches array because the current safe patch layer does not preserve enough raw-result evidence to prove data equivalence.
- If a fix requires new data, changing a calculation, adding/removing rows/contributions/entities/groups, or interpreting a business rule differently, emit status=needs_recompute with an empty patches array.
- The system will re-run deterministic validators after applying patches. Passing this tool is advisory; invalid or semantic patches are rejected.
- This is data-lane repair only. Do not route to source-code analysis, trace analysis, write-mode code editing, or command operation.`

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
	basePrompt := dataTaskEvaluationPrompt(userLine, records, lang)
	resp, err := p.adapter.Chat(ctx,
		[]llm.Message{
			{Role: "system", Content: dataTaskEvaluationSystemPrompt},
			{Role: "user", Content: basePrompt},
		},
		[]llm.ToolSchema{dataTaskEvaluationTool},
		llm.ChatOptions{ToolChoice: "required"},
	)
	p.lastTrace = traceFromLLMResponse("data_task_evaluator", resp)
	if err != nil {
		return dataquery.Evaluation{}, err
	}
	if len(resp.ToolCalls) == 0 {
		retryResp, retryErr := p.adapter.Chat(ctx,
			[]llm.Message{
				{Role: "system", Content: dataTaskEvaluationSystemPrompt},
				{Role: "user", Content: dataTaskEvaluationNoToolRepairPrompt(basePrompt, resp)},
			},
			[]llm.ToolSchema{dataTaskEvaluationTool},
			llm.ChatOptions{ToolChoice: "required"},
		)
		if retryErr == nil {
			p.lastTrace = traceFromLLMResponse("data_task_evaluator_repair", retryResp)
			resp = retryResp
		}
		if retryErr != nil || len(resp.ToolCalls) == 0 {
			return fallbackDataTaskEvaluation(records, resp), nil
		}
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

func dataTaskEvaluationNoToolRepairPrompt(basePrompt string, previous llm.Response) string {
	var b strings.Builder
	b.WriteString("## parse_failure\n")
	b.WriteString("The previous evaluator response did not call the required tool. Re-emit exactly one emit_data_task_evaluation tool call. Do not write prose.\n\n")
	if content := strings.TrimSpace(previous.Content); content != "" {
		fmt.Fprintf(&b, "## previous_content_preview\n%s\n\n", clampDataTaskWorkflowText(content, 1600))
	}
	if reasoning := strings.TrimSpace(previous.ReasoningContent); reasoning != "" {
		fmt.Fprintf(&b, "## previous_reasoning_preview\n%s\n\n", clampDataTaskWorkflowText(reasoning, 1600))
	}
	b.WriteString("## original_evaluation_context\n")
	b.WriteString(basePrompt)
	return strings.TrimSpace(b.String())
}

func fallbackDataTaskEvaluation(records []dataTaskWorkflowRecord, resp llm.Response) dataquery.Evaluation {
	reason := "evaluator produced no structured tool call; continuing conservatively from deterministic workflow state"
	if content := strings.TrimSpace(resp.Content); content != "" {
		reason = "evaluator produced no structured tool call after prose response; continuing conservatively"
	}
	if len(records) == 0 {
		return dataquery.Evaluation{Status: dataquery.EvalContinueData, Confidence: "low", Reason: reason}
	}
	last := records[len(records)-1]
	if strings.TrimSpace(last.Err) != "" {
		eval := dataquery.Evaluation{Status: dataquery.EvalRepairNode, Confidence: "low", Reason: reason}
		if len(last.Plan.Actions) > 0 {
			action := last.Plan.Actions[len(last.Plan.Actions)-1]
			eval.ActionID = action.ID
			eval.ActionKind = string(action.Kind)
		}
		return eval
	}
	if last.Result != nil && len(last.Result.Artifacts) > 0 {
		return dataquery.Evaluation{Status: dataquery.EvalContinueTransform, Confidence: "low", Reason: reason}
	}
	return dataquery.Evaluation{Status: dataquery.EvalContinueData, Confidence: "low", Reason: reason}
}

func (p *llmDataTaskPlanner) ProposeDataResultPatch(ctx context.Context, userLine string, previous dataquery.TaskPlan, partial dataquery.Result, violations []dataquery.DataTaskViolation, records []dataTaskWorkflowRecord, lang string) (dataquery.DataResultPatchPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if p == nil || p.adapter == nil {
		return dataquery.DataResultPatchPlan{}, fmt.Errorf("data result patch planner is not configured")
	}
	prompt := dataTaskResultPatchPrompt(userLine, previous, partial, violations, records, lang)
	resp, err := p.adapter.Chat(ctx,
		[]llm.Message{
			{Role: "system", Content: dataTaskResultPatchSystemPrompt},
			{Role: "user", Content: prompt},
		},
		[]llm.ToolSchema{dataTaskResultPatchTool},
		llm.ChatOptions{ToolChoice: "required"},
	)
	p.lastTrace = traceFromLLMResponse("data_result_patch_planner", resp)
	if err != nil {
		return dataquery.DataResultPatchPlan{}, err
	}
	if len(resp.ToolCalls) == 0 {
		return dataquery.DataResultPatchPlan{}, fmt.Errorf("data result patch planner returned no tool_call")
	}
	call := resp.ToolCalls[0]
	if call.Name != dataTaskResultPatchTool.Name {
		return dataquery.DataResultPatchPlan{}, fmt.Errorf("data result patch planner returned unexpected tool %q", call.Name)
	}
	var parsed dataTaskResultPatchDraft
	if err := unmarshalReplStructuredToolParams(dataTaskResultPatchTool, call.Params, &parsed, "data result patch planner"); err != nil {
		parseErr := err
		raw, repairErr := p.repairDataTaskStructuredToolParams(ctx, dataTaskResultPatchTool, err, dataTaskStructuredToolRepairRequest{
			Scope:   "data result patch planner",
			Context: prompt,
		})
		if repairErr != nil {
			return dataquery.DataResultPatchPlan{}, repairErr
		}
		if err := unmarshalReplStructuredToolParams(dataTaskResultPatchTool, raw, &parsed, "data result patch planner"); err != nil {
			return dataquery.DataResultPatchPlan{}, fmt.Errorf("%w; compact tool-param repair also failed: %v", parseErr, err)
		}
	}
	return parsed.toPatchPlan(), nil
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
	prevJSON, _ := json.MarshalIndent(compactDataTaskRepairContextForViolation(previous, violation), "", "  ")
	fmt.Fprintf(&b, "## previous_plan_compact_json\n%s\n\n", string(prevJSON))
	b.WriteString("## repair_rules\n")
	b.WriteString("- typed_repair_locus.script_line is a 1-based line number in the model-authored script. typed_repair_locus.runner_line is the helper wrapper line and is usually not the line to edit. Use previous_plan_compact_json.script_line_excerpt as the primary local context when script_line is present.\n")
	b.WriteString("- If typed_repair_locus has action_id/action_kind, repair that action/node first. Do not rewrite unrelated graph nodes unless the typed failure proves the graph needs expansion.\n")
	b.WriteString("- If script_line is absent, repair from typed_repair_locus.code, repair_hint, JSON path details when present, and the compact previous plan; do not invent unrelated changes.\n")
	b.WriteString("- Fix the script deterministically; preserve the user's requested output contract unless it was the direct cause of the failure.\n")
	b.WriteString("- Prefer correcting code/import/helper usage over asking the user. Ask only when a user-owned business rule or missing input is genuinely required.\n")
	b.WriteString("- Generated action artifact structure is not user-owned missing information. If a failure shows JSON array/object shape mismatch, use artifact json_shape metadata, inspect/read the artifact, or use json_records(path) instead of asking the user.\n")
	b.WriteString("- If typed_repair_locus.code is oversized_data_plan, do not rewrite another giant one-shot script. Emit a smaller bounded batch with concrete next_batch/why_this_batch and continue_after=true when the overall data goal needs more batches.\n")
	b.WriteString("- Keep input_paths limited to candidate files. List every file the script reads.\n")
	b.WriteString("- Preserve the previous coverage_contract unless the failed script proves a structural reason to replace it. Required materials must keep a verifiable usage_mode.\n")
	b.WriteString("- If the error says a required material was not consumed, repair one of three ways: read the material with a helper and keep usage_mode=script_consumed; if a text evidence file covers the original material, set usage_mode=text_evidence_consumed, text_evidence_path, and read that text evidence path; or if the material was already distilled into typed validation_rules/constraints for this bounded batch, set usage_mode=planner_distilled with concrete distilled_notes. Do not use reference_only for a blocking required material.\n")
	b.WriteString("- For required rule/instruction/example/schema/text materials with usage_mode=script_consumed, read_text(path) is usually the right helper. Use the content to drive the current task's generic validation or audit records; do not add dummy comments or assertions solely to satisfy consumption.\n")
	b.WriteString("- If coverage_contract.decision_records_required=true, emit result.rows. If result.rows was missing, repair by adding generic item-level decision records rather than changing the user's final output format.\n")
	b.WriteString("- If a required validation ledger is missing or reconcile failed, repair by adding/fixing the generic rule_coverage, contributions, entity_resolutions, or reconcile fields and correcting the computation. Do not remove required validation flags to bypass the contract unless the previous contract was structurally wrong for the user goal; explain that structural reason in validation_rules or why_this_batch.\n")
	b.WriteString("- Prefer runner ledger helpers for repaired structures: add_decision(...), add_rule_coverage(...), add_contribution(...), add_resolution(...), and emit_result(...). These helpers reduce JSON-shape drift.\n")
	b.WriteString("- Use canonical ledger field names exactly. Task-specific aliases do not satisfy contribution/reconcile validation: contributions need item_id/source/source_locator/evidence_refs plus group_key/metric/value/operation/role/reason as needed; reconcile.groups need group_key/metric and expected/actual values.\n")
	b.WriteString("- If the final output is a composite string/object and the reconcile group checks that exact final payload, set reconcile.groups[].scope=\"answer\" or role=\"answer\" and make expected/actual equal to result.answer. Ordinary reconcile groups still need group_key/metric matching target contributions.\n")
	b.WriteString("- Contribution operation values are canonicalized, but use operation=\"add\" or \"sum\" for positive numeric totals, \"count\" for counts, and \"subtract\" for negative adjustments. Keep group_key as dimensions only and metric as the measure name; do not encode metric inside group_key. Use role=\"target\" for final answer metrics; role=\"audit\" or role=\"intermediate\" is only for material coverage, sample counts, diagnostics, or helper statistics that should not force final answer reconciliation.\n")
	b.WriteString("- Rule coverage records must be supported by notes, evidence_refs, or rule_refs on rows/contributions/entity_resolutions. Resolved entity_resolutions need canonical_id/canonical_label; unresolved or ambiguous mappings need reason, candidates, or evidence_refs. Candidate entries should be objects with id/label/evidence/confidence, not task-specific aliases.\n")
	b.WriteString("- The script must call emit(obj) or set result. Debug print is allowed only in small amounts and is not the final answer.\n")
	b.WriteString("- If the repair requires side effects such as network/process/file write/install/delete, return status=blocked instead of embedding those actions in the script; the operation pipeline owns risk and approval.\n\n")
	b.WriteString("## candidate_data_files\n")
	appendCandidateDataFiles(&b, candidates)
	return strings.TrimSpace(b.String())
}

type dataTaskRepairContext struct {
	Status              string                     `json:"status,omitempty"`
	Actions             []dataquery.DataAction     `json:"actions,omitempty"`
	ActionID            string                     `json:"action_id,omitempty"`
	ActionKind          string                     `json:"action_kind,omitempty"`
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
	ScriptLine          int                        `json:"script_line,omitempty"`
	RunnerLine          int                        `json:"runner_line,omitempty"`
	ScriptLineExcerpt   []dataTaskScriptLine       `json:"script_line_excerpt,omitempty"`
	ScriptPreview       string                     `json:"script_preview,omitempty"`
}

type dataTaskScriptLine struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

func compactDataTaskRepairContext(plan dataquery.TaskPlan) dataTaskRepairContext {
	return compactDataTaskRepairContextForViolation(plan, dataquery.DataTaskViolation{})
}

func compactDataTaskRepairContextForViolation(plan dataquery.TaskPlan, violation dataquery.DataTaskViolation) dataTaskRepairContext {
	scriptLine := violation.ScriptLine
	script := dataTaskRepairScriptForViolation(plan, violation)
	excerpt := dataTaskScriptLineExcerpt(script, scriptLine, 8, 80)
	return dataTaskRepairContext{
		Status:              strings.TrimSpace(plan.Status),
		Actions:             compactDataTaskActionsForPrompt(plan.Actions, 12, 1800),
		ActionID:            strings.TrimSpace(violation.ActionID),
		ActionKind:          strings.TrimSpace(violation.ActionKind),
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
		ScriptLine:          scriptLine,
		RunnerLine:          violation.RunnerLine,
		ScriptLineExcerpt:   excerpt,
		ScriptPreview:       clampDataTaskWorkflowText(script, 6000),
	}
}

func dataTaskRepairScriptForViolation(plan dataquery.TaskPlan, violation dataquery.DataTaskViolation) string {
	actionID := strings.TrimSpace(violation.ActionID)
	if actionID != "" {
		for _, action := range plan.Actions {
			if strings.TrimSpace(action.ID) == actionID && strings.TrimSpace(action.Script) != "" {
				return action.Script
			}
		}
	}
	return plan.Script
}

func dataTaskScriptLineExcerpt(script string, centerLine, contextLines, maxLines int) []dataTaskScriptLine {
	script = strings.ReplaceAll(script, "\r\n", "\n")
	lines := strings.Split(script, "\n")
	if len(lines) == 0 || strings.TrimSpace(script) == "" {
		return nil
	}
	if maxLines <= 0 || maxLines > 120 {
		maxLines = 80
	}
	start := 0
	end := len(lines)
	if centerLine > 0 {
		if contextLines < 0 {
			contextLines = 0
		}
		start = centerLine - contextLines - 1
		if start < 0 {
			start = 0
		}
		end = centerLine + contextLines
		if end > len(lines) {
			end = len(lines)
		}
	} else if end > maxLines {
		end = maxLines
	}
	if end-start > maxLines {
		if centerLine > 0 {
			half := maxLines / 2
			start = centerLine - half - 1
			if start < 0 {
				start = 0
			}
			end = start + maxLines
			if end > len(lines) {
				end = len(lines)
				start = end - maxLines
				if start < 0 {
					start = 0
				}
			}
		} else {
			end = start + maxLines
		}
	}
	out := make([]dataTaskScriptLine, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, dataTaskScriptLine{
			Line: i + 1,
			Text: clampDataTaskWorkflowText(lines[i], 500),
		})
	}
	return out
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
	if stateJSON := marshalDataTaskWorkflowState(records, dataquery.TaskPlan{}); stateJSON != "" {
		fmt.Fprintf(&b, "\n\n## workflow_state_json\n%s", stateJSON)
	}
	b.WriteString("\n\n## continuation_rules\n")
	b.WriteString("- If previous results satisfy the user's data goal and output contract, emit status=complete with a short block_reason/reason and no script.\n")
	b.WriteString("- If more data calculation is needed, emit status=ready with only the next bounded script batch.\n")
	b.WriteString("- Prefer extending the adaptive action graph over rewriting a large script. Use previous result.artifacts to decide the next atomic action. A failed custom_transform should usually be repaired as a smaller custom_transform or replaced by inspect_material/material_inventory first.\n")
	b.WriteString("- Follow workflow_state_json.next_stage. If material_coverage_sufficient=true, do not emit another coverage-only batch (material_inventory, inspect_material, extract_records, derive_rules) unless workflow_state_json.missing_required_materials names a specific new material. Move to derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, or one narrow custom_transform over generated artifacts.\n")
	b.WriteString("- Previous action ids and output_artifact names are reusable read-only JSON materials. Later actions can list them in input_paths and read them with json_load(path), or typed actions can consume them as structured records.\n")
	b.WriteString("- Use result.artifact_access as the access contract for generated artifacts. It gives aliases, json_shape, and access_hint. If an artifact is array-shaped, use json_records(alias) or iterate json_load(alias) as a list; do not call .get() on the top-level value.\n")
	b.WriteString("- A custom_transform that reads many materials or emits multiple ledgers must be a final bounded transform over known inputs. If prior results have not consumed/profiled every required input, emit typed prerequisite actions first instead of another broad script.\n")
	b.WriteString("- Emit at most one scripted custom_transform per actions[] batch. If multiple fallback transforms are needed, produce one artifact now and request continuation for the next batch.\n")
	b.WriteString("- Continue plans must preserve still-relevant coverage_contract materials; do not drop required materials just because a prior batch produced a partial answer.\n")
	b.WriteString("- Continue plans must preserve still-relevant required validation flags. If a prior result has missing rule coverage, missing contributions, missing entity resolutions, or failed reconcile, the next batch should repair the computation or emit a corrected bounded result.\n")
	b.WriteString("- Prompt samples are compact previews. Use *_count and *_truncated fields before deciding a prior result is missing items or groups.\n")
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
	if stateJSON := marshalDataTaskWorkflowState(records, dataquery.TaskPlan{}); stateJSON != "" {
		fmt.Fprintf(&b, "\n\n## workflow_state_json\n%s", stateJSON)
	}
	b.WriteString("\n\n## evaluator_rules\n")
	b.WriteString("- Prompt samples are compact previews. Use *_count and *_truncated fields to distinguish a sample from the full result.\n")
	b.WriteString("- Do not infer missing reconcile groups, answer items, decisions, contributions, or resolutions solely because the sample list is shorter than the total count.\n")
	b.WriteString("- Judge completion from the user goal, output_contract, counts, reconcile status, coverage warnings, and explicit failures together.\n")
	b.WriteString("- Prefer precise continue statuses: expand_graph for more inspection/extraction artifacts, repair_node for one failed/incorrect prior node, continue_transform for a bounded transform over known artifacts. Use continue_data only when no precise status fits.\n")
	b.WriteString("- If workflow_state_json.material_coverage_sufficient=true and there is still no contribution/reconcile/final answer, evaluate toward compute-stage continuation; do not ask for more material coverage unless missing_required_materials is non-empty.\n")
	return strings.TrimSpace(b.String())
}

func marshalDataTaskWorkflowState(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) string {
	raw, err := json.MarshalIndent(dataTaskWorkflowState(records, current), "", "  ")
	if err != nil {
		return ""
	}
	return string(raw)
}

func dataTaskResultPatchPrompt(userLine string, previous dataquery.TaskPlan, partial dataquery.Result, violations []dataquery.DataTaskViolation, records []dataTaskWorkflowRecord, lang string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## language\n%s\n\n", strings.TrimSpace(lang))
	fmt.Fprintf(&b, "## user_request\n%s\n\n", strings.TrimSpace(userLine))
	prevJSON, _ := json.MarshalIndent(compactDataTaskRepairContext(previous), "", "  ")
	fmt.Fprintf(&b, "## previous_plan_compact_json\n%s\n\n", string(prevJSON))
	violationsJSON, _ := json.MarshalIndent(violations, "", "  ")
	fmt.Fprintf(&b, "## typed_violations\n%s\n\n", string(violationsJSON))
	resultJSON, _ := json.MarshalIndent(compactDataTaskPatchResult(partial), "", "  ")
	fmt.Fprintf(&b, "## partial_result_compact_json\n%s\n\n", string(resultJSON))
	b.WriteString("## recent_data_rounds\n")
	b.WriteString(renderDataTaskRecordsForPrompt(records))
	b.WriteString("\n\n## patch_rules\n")
	b.WriteString("- Emit status=patch only for structural JSON-shape drift in the already-computed result.\n")
	b.WriteString("- typed_violations[].json_path points to the structured result JSON, not to a script line. Patch only the exact structural field when allowed; use needs_recompute for computation, coverage, or business-rule changes.\n")
	b.WriteString("- Do not modify answer, numeric values, source values, include/exclude decisions, canonical mapping choices, rule interpretations, or any task/business semantics.\n")
	b.WriteString("- Do not add or remove rows, rule_coverage records, contributions, entity_resolutions, reconcile groups, metrics, or consumed_paths.\n")
	b.WriteString("- Do not emit remove or move operations. If a duplicate wrapper or alias would require moving/removing fields, emit status=needs_recompute and patches=[] because this patch layer cannot prove raw-result data equivalence.\n")
	b.WriteString("- Allowed paths are limited to contribution operation/group_key/metric, entity resolution status, missing reconcile status only when no differences are present, and reconcile group_key/metric.\n")
	b.WriteString("- If the typed violation needs new computation or a business decision, emit status=needs_recompute and patches=[].\n")
	return strings.TrimSpace(b.String())
}

func compactDataTaskPatchResult(result dataquery.Result) dataTaskResultPromptView {
	view := compactDataTaskResultPromptView(result, 1200, 800, 4, 3, 5)
	if view == nil {
		return dataTaskResultPromptView{}
	}
	return *view
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
	Actions             []dataTaskActionDraft    `json:"actions"`
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

type dataTaskActionDraft struct {
	ID              flexiblePolicyString     `json:"id"`
	Kind            flexiblePolicyString     `json:"kind"`
	Purpose         flexiblePolicyString     `json:"purpose"`
	InputPaths      flexiblePolicyStringList `json:"input_paths"`
	OutputArtifact  flexiblePolicyString     `json:"output_artifact"`
	Script          flexiblePolicyString     `json:"script"`
	Params          map[string]any           `json:"params"`
	SuccessCriteria flexiblePolicyStringList `json:"success_criteria"`
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
	ActionID      flexiblePolicyString     `json:"action_id"`
	ActionKind    flexiblePolicyString     `json:"action_kind"`
	RepairLocus   flexiblePolicyString     `json:"repair_locus"`
}

type dataTaskResultPatchDraft struct {
	Status     flexiblePolicyString           `json:"status"`
	Patches    []dataTaskResultPatchItemDraft `json:"patches"`
	Reason     flexiblePolicyString           `json:"reason"`
	Confidence flexiblePolicyString           `json:"confidence"`
}

type dataTaskResultPatchItemDraft struct {
	Target flexiblePolicyString `json:"target"`
	Op     flexiblePolicyString `json:"op"`
	Path   flexiblePolicyString `json:"path"`
	Value  json.RawMessage      `json:"value"`
	Reason flexiblePolicyString `json:"reason"`
}

func (d dataTaskEvaluationDraft) toEvaluation() dataquery.Evaluation {
	return dataquery.Evaluation{
		Status:        dataquery.NormalizeEvaluationStatus(string(d.Status)),
		Reason:        strings.TrimSpace(string(d.Reason)),
		Confidence:    strings.TrimSpace(string(d.Confidence)),
		MissingInputs: cleanPolicyStringList([]string(d.MissingInputs)),
		ActionID:      strings.TrimSpace(string(d.ActionID)),
		ActionKind:    strings.TrimSpace(string(d.ActionKind)),
		RepairLocus:   strings.TrimSpace(string(d.RepairLocus)),
	}
}

func (d dataTaskResultPatchDraft) toPatchPlan() dataquery.DataResultPatchPlan {
	patches := make([]dataquery.DataResultPatch, 0, len(d.Patches))
	for _, p := range d.Patches {
		path := strings.TrimSpace(string(p.Path))
		if path == "" {
			continue
		}
		value := bytes.TrimSpace(p.Value)
		patches = append(patches, dataquery.DataResultPatch{
			Target: firstNonEmptyString(strings.TrimSpace(string(p.Target)), "result"),
			Op:     firstNonEmptyString(strings.TrimSpace(string(p.Op)), "replace"),
			Path:   path,
			Value:  append(json.RawMessage(nil), value...),
			Reason: strings.TrimSpace(string(p.Reason)),
		})
	}
	return dataquery.DataResultPatchPlan{
		Status:     strings.ToLower(strings.TrimSpace(string(d.Status))),
		Patches:    patches,
		Reason:     strings.TrimSpace(string(d.Reason)),
		Confidence: strings.TrimSpace(string(d.Confidence)),
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
		Actions:             dataTaskActionsToPlan(d.Actions),
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

func dataTaskActionsToPlan(in []dataTaskActionDraft) []dataquery.DataAction {
	out := make([]dataquery.DataAction, 0, len(in))
	for i, action := range in {
		kind := strings.TrimSpace(string(action.Kind))
		if kind == "" {
			continue
		}
		params := map[string]string{}
		for k, v := range action.Params {
			k = strings.TrimSpace(k)
			if k == "" || v == nil {
				continue
			}
			params[k] = fmt.Sprint(v)
		}
		id := strings.TrimSpace(string(action.ID))
		if id == "" {
			id = fmt.Sprintf("action_%d", i+1)
		}
		out = append(out, dataquery.DataAction{
			ID:              id,
			Kind:            dataquery.DataActionKind(kind),
			Purpose:         strings.TrimSpace(string(action.Purpose)),
			InputPaths:      cleanPolicyStringList([]string(action.InputPaths)),
			OutputArtifact:  strings.TrimSpace(string(action.OutputArtifact)),
			Script:          strings.TrimSpace(string(action.Script)),
			Params:          params,
			SuccessCriteria: cleanPolicyStringList([]string(action.SuccessCriteria)),
		})
	}
	return out
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
