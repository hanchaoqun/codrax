# Data Workflow Contract

## Problem

The data lane is now goal-aware, but it still needs a stronger correctness
contract. A model-authored script can read only a subset of relevant materials,
repair itself after a syntax/runtime failure, silently drop important inputs,
and still produce an output that satisfies the final presentation format. That
catches syntax and shape errors, but not data correctness errors.

The fix must not encode any one business domain or one material shape. Business
roles, record types, output expectations, and material meaning are all
user-task semantics. The model interprets those semantics from the current
request; Codrax provides objective material discovery, bounded execution,
consumption tracking, decision-record validation, and output-contract
validation.

## Red Lines

- Do not classify materials by business meaning in Go code.
- Do not hard-code file names, column names, attachment concepts, business
  roles, or output shapes from any single customer case.
- Do not route source-code analysis, log/trace diagnosis, write mode, or mixed
  source questions through the data lane unless the typed route explicitly says
  data or mixed data.
- Do not hard-gate on user prose or model prose. Hard gates consume typed
  plan/result fields, file paths, runner consumption telemetry, and output
  contracts.
- Do not introduce side effects into the data runner. Side effects remain
  operation-lane work.

## Design

### 1. Material Inventory

Codrax deterministically discovers local materials and reports objective facts:

- path, size, media kind;
- tabular headers and row counts where cheap;
- JSON/JSONL top-level field samples where cheap;
- text line counts and short previews where cheap;
- non-text material status, including whether related text evidence is already
  available or extraction is required.

The inventory does not decide that a material is a rule file, evidence file,
reference table, source of truth, attachment, output spec, or any other
business role. It only gives the model grounded metadata. The model decides
material purpose for the current user goal.

Non-text materials are first-class inventory entries. Go code may report
objective media kind and extraction status, but it must not decide whether the
content is important for a task. If the model declares a material with
`extraction_status=needs_text_extraction` as required and no text evidence
exists, the workflow must either call a configured material extractor or fail
with a recoverable extraction gap. It must not silently skip the material or let
the deterministic Python runner read binary/non-text content as if it were
semantic text.

### 1.1 Multimodal Material Extraction

Multimodal extraction is a separate data-lane adjunct, not part of source-code
analysis, trace/log diagnosis, or command operation.

Trigger conditions are structural:

1. the active route is the data lane;
2. the model-authored `coverage_contract.required_materials` includes a
   candidate whose objective inventory says text extraction is required;
3. the inventory has no usable text evidence for that required material;
4. a `multimodal_material_extractor` provider is configured.

The extractor receives the specific required non-text material and emits a
normalized text evidence artifact with source path, extracted text, confidence,
and caveats. The text artifact is appended to the material inventory and the
data planner repairs/continues with that text path. The final computation still
runs through the same deterministic data runner, consumption telemetry,
decision records, contribution/entity/reconcile ledgers, and output contract.

If no extractor is configured, or the extractor cannot handle the material
type, the data workflow surfaces a typed recoverable gap. It does not guess,
skip, or ask the source-code pipeline to compensate.

### 2. Coverage Contract

The model emits a generic `coverage_contract` inside `emit_data_task_plan`.

The contract contains:

- `required_materials`: material paths that must be consumed before the answer
  can be trusted;
- `optional_materials`: useful but non-blocking materials;
- `validation_rules`: task-specific structural checks phrased by the model;
- `decision_records_required`: whether filtering, joining, aggregation,
  extraction, or item-level decisions need generic decision records.

The contract is task-specific and model-authored. Go code validates only the
structure: declared required materials must be declared as inputs and actually
read by the script.

### 3. Consumption Telemetry

The Python helper records every declared input file actually opened through
`csv_rows`, `tsv_rows`, `json_load`, `jsonl_rows`, `read_text`, or safe
`open(...)`. The runner injects `consumed_paths` into the structured result.

If a required material is not consumed, the runner fails with a typed coverage
error. Existing data repair flow then asks the model to repair the plan/script,
with compact context, rather than accepting an ungrounded answer.

### 4. Validation Contract Matrix

The data lane uses a composable validation contract. The model decides which
parts apply from the user goal; Codrax only validates typed structure and
deterministic reconciliation. This avoids fitting the system to one business
case.

Common combinations:

- simple sum/count/ranking: `ContributionLedger + Reconcile`;
- cleaning/filtering: `RuleCoverage + ContributionLedger + Reconcile`;
- multi-material join or name/entity normalization:
  `RuleCoverage + EntityResolutionLedger + ContributionLedger + Reconcile`;
- strict output shape: add `OutputContract`;
- pure summary/extraction: `MaterialCoverage + OutputContract`.

Contract fields:

- `rule_coverage_required`: the result must include `rule_coverage` records
  explaining which model-authored rules were applied, not applicable, or failed;
- `contribution_ledger_required`: the result must include `contributions`
  records for the items that contribute to totals, counts, groups, ranks, or
  output elements;
- `entity_resolution_required`: the result must include `entity_resolutions`
  records for source values mapped to canonical values, including ambiguous or
  unresolved values;
- `reconcile_required`: the result must include a `reconcile` report. When
  contributions and reconcile groups are present, Codrax recomputes group totals
  from `contributions` and rejects mismatches.

These are generic ledgers. A contribution item can be a table row, JSON item,
text span, page, image region, webpage block, or any other task item. An entity
resolution can map names, IDs, categories, labels, files, records, accounts, or
other task-specific values. Go code must not infer those meanings.

### 5. Linked Validation

The first validation batch only checked that required ledgers existed and that
numeric contribution groups reconciled. Real data tasks need one more generic
layer: the ledgers must explain each other. This is still structural; Codrax
does not decide business meaning.

The linked contract is:

- `rule_coverage` records must carry `rule_id` or `rule_text`, a `status`, and
  support. Support can be `notes`, `evidence_refs`, or structured `rule_refs`
  from decision records, contribution records, or entity-resolution records.
- `contributions` must use canonical fields. A contribution has an item/source
  anchor (`item_id`, `source`, `source_locator`, or `evidence_refs`) and an
  effect (`group_key`, `metric`, `value`, `operation`, or `reason`). Task-local
  aliases may appear in explanatory fields, but they do not replace canonical
  contribution fields.
- `entity_resolutions` must say what source value/item was resolved and how. A
  resolved mapping needs `canonical_id` or `canonical_label`; an unresolved,
  ambiguous, failed, or open mapping needs `reason`, `candidates`, or
  `evidence_refs`.
- `reconcile.groups` must use canonical `group_key`, `metric`, `expected`, and
  `actual` fields. Codrax recomputes totals from `contributions`; non-zero
  reported groups must match contribution totals, zero-valued groups may be
  reported without synthetic contribution records, and every contribution group
  must be reported.

This catches a broad class of "script ran and emitted a formatted answer, but
the audit structure is hollow" failures without hard-coding a domain. The same
contract applies to sums, counts, rankings, joins, text-span extraction, JSONL
transforms, document-derived calculations, and strict-output data answers.

### 6. Decision Records

`Result.Rows` remains the wire field for existing result shapes, but its
meaning is generic decision records:

- source material and source locator;
- decision;
- normalized fields;
- evidence refs;
- contribution value.

A decision record may represent a CSV row, Excel cell/range, JSON item, text
span, page, image region, extracted entity, or any task-specific item. The
model supplies semantics; Codrax checks only generic structural presence when
`coverage_contract.decision_records_required=true`. Newer tasks should prefer
the more specific generic ledgers above when the required validation is about
rules, entity resolution, contribution, or reconciliation.

### 7. Output Contract Separation

Computation correctness and final output shape are separate:

- coverage and decision-record checks validate whether the result is
  structurally grounded;
- `OutputContract` validates whether the final user-facing answer is a single
  line, JSON-only, Markdown table, and so on.

This prevents a case where a perfectly formatted string hides an incomplete
calculation.

## Task Checklist

- [x] Record generalized design and red lines.
- [x] Add generic material metadata to discovered data candidates.
- [x] Add `CoverageContract` / `CoverageMaterial` to `dataquery.TaskPlan`.
- [x] Expose `coverage_contract` in `emit_data_task_plan` schema and prompt.
- [x] Parse new fields through the existing REPL structured JSON repair path.
- [x] Track `consumed_paths` in the data runner.
- [x] Validate required material declaration and consumption.
- [x] Preserve coverage contracts across repair turns unless the model emits a
      replacement contract.
- [x] Require decision records only when the model sets
      `decision_records_required=true`.
- [x] Add tests for material metadata, coverage consumption, repair contract
      preservation, and JSON compatibility.
- [x] Refresh data-task user documentation.
- [x] Add `rule_coverage_required`, `contribution_ledger_required`,
      `entity_resolution_required`, and `reconcile_required` to
      `CoverageContract`.
- [x] Add typed `rule_coverage`, `contributions`, `entity_resolutions`, and
      `reconcile` fields to data results.
- [x] Implement generic runner validation for required ledgers.
- [x] Implement deterministic contribution-to-reconcile group checks.
- [x] Strengthen contribution records so required contribution ledgers must use
      canonical item/effect fields.
- [x] Strengthen reconcile groups so blank or alias-only groups cannot pass.
- [x] Add structural rule-link support through generic `rule_refs`.
- [x] Validate rule coverage support via `notes`, `evidence_refs`, or linked
      `rule_refs`.
- [x] Validate entity-resolution records for source, status, canonical value,
      or explicit unresolved/ambiguous rationale.
- [x] Validate contribution/reconcile linkage in both directions.
- [x] Allow structurally declared zero-valued reconcile groups without forcing
      fake contribution records.
- [x] Include available contribution-group diagnostics when a non-zero
      reconcile group has no matching contribution records.
- [x] Preserve direct LLM planner content and full tool parameters in debug
      logs for data/operation audit, including visible `<think>` blocks when
      the provider emits them as ordinary content.
- [x] Expose the validation matrix in `emit_data_task_plan` schema and prompt.
- [x] Teach planner/repair prompts that canonical ledger fields and `rule_refs`
      are required for linked validation.
- [x] Preserve required validation contracts across repair turns.
- [x] Include validation-ledger summaries in evaluator/continuation handoff.
- [x] Add tests for missing ledgers, failed reconcile, passing reconcile, and
      planner JSON compatibility.
- [x] Add tests for unsupported rule coverage, unknown rule refs, missing
      canonical contribution fields, blank reconcile groups, unreported
      contribution groups, and incomplete entity-resolution records.
- [x] Detect non-text materials that need extraction in the data inventory.
- [x] Keep non-text materials out of direct deterministic runner inputs.
- [x] Surface related text-evidence candidates when objective path metadata
      suggests they exist.
- [x] Add optional `multimodal_material_extractor` provider routing for data
      material extraction.
- [x] Add OpenAI-compatible optional multimodal content parts without changing
      text-only LLM requests.
- [x] Feed extracted text evidence back into the data workflow as ordinary
      text materials, then require the planner to repair/continue normally.
- [x] Document extractor configuration and non-goals.
