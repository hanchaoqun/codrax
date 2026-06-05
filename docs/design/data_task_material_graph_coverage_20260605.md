# Data Workflow Contract

## Problem

The data lane is now goal-aware, but it still needs a stronger correctness
contract. A model-authored script can read only a subset of relevant materials,
repair itself after a syntax/runtime failure, silently drop important inputs,
and still produce an output that satisfies the final presentation format. That
catches syntax and shape errors, but not data correctness errors.

The fix must not encode any one business domain. Procurement, invoices,
contracts, vendors, record sets, orders, survey items, JSON records, web extracts,
and document spans are all user-task semantics. The model interprets those
semantics from the current request; Codrax provides objective material
discovery, bounded execution, consumption tracking, decision-record validation,
and output-contract validation.

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
- text line counts and short previews where cheap.

The inventory does not decide that a material is a rule file, evidence file,
reference table, source of truth, attachment, output spec, or any other
business role. It only gives the model grounded metadata. The model decides
material purpose for the current user goal.

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

### 4. Decision Records

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
`coverage_contract.decision_records_required=true`.

### 5. Output Contract Separation

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
- [ ] Add richer validation for declared `validation_rules` once real-world
      data-task evals identify stable structural checks.
