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

- `required_materials`: material paths or material IDs that must be covered
  before the answer can be trusted;
- `optional_materials`: useful but non-blocking materials;
- `validation_rules`: task-specific structural checks phrased by the model;
- `decision_records_required`: whether filtering, joining, aggregation,
  extraction, or item-level decisions need generic decision records.

The contract is task-specific and model-authored. Go code validates only the
structure: declared required materials must have a verifiable usage mode.

Material usage modes are generic:

- `script_consumed`: the deterministic script must directly read the material
  through a runner helper and use the returned content. This is the default
  mode and remains the strictest mode for datasets and other raw inputs.
- `text_evidence_consumed`: the original material is covered by a separate
  runner-readable text evidence artifact. The contract must carry
  `text_evidence_path`, and the script must consume that path.
- `planner_distilled`: the model has already distilled the material content
  into typed validation rules, constraints, or notes for this bounded batch.
  The contract must carry concrete `distilled_notes`; the runner validates the
  typed declaration but does not require direct file IO for the original
  material.
- `reference_only`: advisory context that should normally live in
  `optional_materials`; it is not a blocking required-material mode.

Go code does not decide which mode a material deserves. The model chooses the
mode from the user goal and the objective material inventory; Codrax only
validates the declared mode with typed fields and runner telemetry.

### 3. Consumption Telemetry

The Python helper records every declared input file actually opened through
`csv_rows`, `tsv_rows`, `json_load`, `jsonl_rows`, `read_text`, or safe
`open(...)`. The runner injects `consumed_paths` into the structured result.

If a required material is not covered according to its usage mode, the runner
fails with a typed coverage error. Existing data repair flow then asks the
model to repair the plan/script, with compact context, rather than accepting an
ungrounded answer.

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

## 2026-06-05 Failure Audit: Ledger Contract Drift

A recent real data workflow still failed after multiple repair rounds. The
failures were not tied to one business domain. They exposed generic drift
between the model-authored script contract, runner result schema, and
deterministic validators:

- the script redefined reserved helpers instead of using the runner-provided
  helper surface;
- required materials were declared but not consumed by the script;
- `entity_resolutions.candidates` was emitted as a string array, while the
  typed result schema expects candidate objects;
- contribution records used a natural aggregation label such as `sum`, while
  the deterministic reconcile path accepted only a narrower operation set;
- `group_key` and `metric` were both free strings, so the same metric could be
  encoded twice and no longer match reconcile groups;
- repair repeatedly rewrote a large script rather than repairing the precise
  typed violation locus.

This is a system-level data-lane issue, not a single-case rule error. The data
lane can already reject bad results, but it still needs stronger typed
normalization and smaller repair targets so the same class of error does not
become a long model retry loop.

## 2026-06-05 Failure Audit: Material Coverage Ambiguity

Another real workflow exposed a separate generic issue: the model declared a
text material as required, but the script did not read it. The old contract had
only one hard interpretation: every required material must be direct script
input. That is correct for raw datasets, but too narrow for broader data work:

- a material may be directly consumed by the script;
- a non-text or derived material may be covered through extracted text
  evidence;
- a small rule/example/schema may be distilled by the model into typed
  validation rules before execution;
- a material may be reference-only and should not block computation.

The fix is not to special-case any file name or document role. The fix is to
make material usage mode a typed contract field. The model owns the semantic
choice; Codrax validates the chosen mode structurally.

## 2026-06-05 Failure Audit: Oversized Batch and Helper Contract Drift

Another real workflow exhausted multiple repair rounds without producing a
trustworthy answer. The individual failures varied, but the common mechanism
was generic:

- the model emitted a large one-shot script that tried to do discovery,
  parsing, rule application, entity resolution, contribution collection,
  reconciliation, and final rendering in one batch;
- the script used JSON-style `true`/`false`/`null` inside Python;
- identifiers were sometimes parsed as integers because the model guessed a
  field type from shape instead of preserving ID strings;
- helper calls used structurally understandable aliases such as `row_id`,
  `status`, `rule`, `op`, or `record_id`, while the runner and validator
  expected canonical wire fields;
- a contribution referenced a rule ID that had not been emitted in
  `rule_coverage`;
- entity-resolution records missed a status.

None of these are business-domain failures. They are contract-boundary
failures between planner, script helper surface, runner result schema, and
validator. The generic fix is:

1. reject oversized one-shot data plans before execution when typed structural
   signals show the plan should be staged;
2. make runner helpers own canonical JSON shape and absorb safe structural
   aliases, while still leaving business semantics to the model;
3. normalize the same safe structural aliases when a script emits raw result
   JSON instead of using helpers;
4. classify common structural failures as typed repair loci so repair prompts
   are precise and compact;
5. keep validators strict about unknown rule references and reconciliation so
   scripts cannot pass with hollow or disconnected ledgers.

The guard is intentionally structural. It reads script line count, required
material count, validation-ledger count, plan status, and `continue_after`.
It does not inspect user prose, model prose, file names, domains, or column
names. Simple one-batch data tasks continue to execute normally; complex tasks
are steered into bounded workflow batches.

## 2026-06-05 Failure Audit: CLI Workflow Drift and Strict Scalar Evals

The REPL data workflow had grown into a goal-aware loop with plan, execution,
repair, evaluation, continuation, audit artifacts, and final answer rendering.
The CLI single-shot data path was still closer to a one-plan/one-run shortcut.
That created two generic gaps:

- CLI data tasks could stop after the first script failure even when the same
  task would have repaired and completed in REPL.
- Eval infrastructure assumed explanatory answers and rejected a correct
  strict scalar answer because it had fewer than twenty non-whitespace
  characters.

The fix is not to pad strict answers or to special-case one fixture. The fix is
to share the same data workflow semantics across CLI and REPL, and to let eval
cases declare their output contract when a correct answer is intentionally
short. Strict output contracts remain user-facing product behavior; test
harness sanity checks must not force the product to emit extra prose.

CLI data completion now emits a compact `[cli/data] data task result` audit log
with round number, answer length, ledger counts, reconcile status, consumed
materials, and warnings. REPL keeps its richer `[repl/data]` audit lane. Both
are control-plane observability only; neither is used as a hard user-intent
gate.

## Data Result Patch Engine

The next stability gap is result-shape repair. The product should not ask the
model to rewrite an entire script when the script already computed the right
data but emitted a structurally drifted result. At the same time, Codrax must
not "fix" business meaning.

The patch engine is therefore deliberately narrow:

- It may patch only result structure that is unambiguous and domain-neutral:
  canonical operation aliases, duplicate metric suffixes in group keys,
  resolved status for entity-resolution records that already carry canonical
  values, and other typed shape drift that can be repaired without changing
  answer semantics.
- It must not patch the final answer, invent records, decide whether a record
  should be included, choose a canonical business entity, or reinterpret a
  user rule. Those failures remain `needs_recompute` and go back through the
  bounded workflow.
- Every applied patch is recorded in `result.result_patches` with target,
  operation, JSON path, value, and reason. The raw script result remains
  available through data audit artifacts; the patched result is the one that
  validators consume.
- Runner helper ledgers are merged before validation even when the script uses
  the lower-level `emit({...})` API. Scripts should still prefer helpers, but
  completion no longer depends on the model remembering which emit API bundles
  accumulated ledgers.
- Validators should produce typed violations with `code`, `json_path`,
  `expected_shape`, `actual_snippet`, and
  `repairability=safe_patch|needs_recompute|needs_clarification`. The legacy
  error string remains for user-facing diagnostics and existing repair prompts.

Repair layering:

1. L0: JSON unmarshal and helper-level normalization accepts safe structural
   aliases.
2. L1: deterministic result patch engine applies safe structural patches and
   records them.
3. L2: future model-authored typed patch IR may propose only structural
   patches against validator violations; Codrax applies and revalidates.
4. L3: if a violation needs business recomputation, the workflow emits a new
   bounded data plan instead of patching the result.

This is intentionally generic. It applies to tabular aggregation, JSONL
transforms, text-span extraction, entity normalization, OCR-derived evidence,
strict scalar output, and Markdown/JSON/CSV output contracts. It does not know
or encode any customer domain.

### L2 Typed Patch IR Design

L2 is the next layer after deterministic patches. It exists for result-shape
violations that are still structural but not safe enough for a local heuristic.
It must not become a way for the model to silently rewrite business meaning.

Inputs:

- validator `DataTaskViolation` objects only, including `code`, `json_path`,
  `expected_shape`, `actual_snippet`, and `repairability`;
- the bounded result excerpt around the failing JSON path;
- the immutable coverage contract and output contract;
- the patch budget and previous patch audit.

Output IR:

```json
{
  "patches": [
    {
      "target": "result",
      "op": "replace",
      "path": "/contributions/0/operation",
      "value": "add",
      "reason": "normalize aggregation alias",
      "source_violation_code": "unsupported_contribution_operation"
    }
  ],
  "requires_recompute": false,
  "confidence": "high"
}
```

Allowed patch operations:

- `replace` on an existing scalar/object field whose expected shape is known;
- `remove` only for duplicate structural wrappers that preserve the same data;
- `move` only inside the same record when aliases created an extra field and
  the canonical field is empty.

Forbidden patch operations:

- editing `answer` without a deterministic recompute from contribution or
  reconcile records;
- adding new business records, rows, contributions, resolutions, or rule
  coverage that were not present in the raw result;
- changing include/exclude decisions, canonical entity choices, rule
  interpretation, amounts, quantities, timestamps, or any user-domain value;
- weakening the coverage contract, output contract, or validation flags.

Execution:

1. Apply L0 unmarshal/helper normalization.
2. Apply L1 deterministic patch engine and record `result_patches`.
3. Run validators. If remaining violations have
   `repairability=safe_patch` and patch budget remains, ask the L2 patch model
   for typed patch IR with only the compact violation context.
4. Apply accepted patches to a copy of the result, append them to
   `result_patches`, and re-run the full validator stack.
5. If validation still fails or a patch asks for forbidden semantics, discard
   the L2 patch and return to bounded recomputation.

Audit:

- Persist raw result, L1 patches, L2 patch proposal, accepted/rejected patch
  decisions, and post-patch validator output.
- Surface only compact counts in REPL/CLI; full patch artifacts live under the
  data audit directory.
- Do not use model patch prose for hard decisions. Hard decisions consume only
  schema-valid patch IR plus deterministic validator results.

### P0 Remediation Direction

1. **Material usage modes.** Extend `CoverageMaterial` with generic
   `usage_mode`, `text_evidence_path`, and `distilled_notes` fields. Empty
   mode remains `script_consumed`.
2. **Mode-aware validation.** Direct script materials require `input_paths` and
   runner `consumed_paths`; text-evidence materials require the evidence path
   to be declared and consumed; planner-distilled materials require concrete
   notes; reference-only materials cannot be blocking required materials.
3. **Mode-aware extraction.** Non-text extraction should trigger only for
   script-consumed required materials that still need semantic text. Materials
   already covered by text evidence or planner distillation should not loop
   through extraction.
4. **Mode-aware repair.** Repair prompts should explain the precise typed
   violation and ask the model to either consume the material, consume text
   evidence, or switch to planner distillation with concrete notes.

### P1 Remediation Direction

- Connect planner-distilled materials to staged workflows: material
  summarization/distillation can become an explicit earlier batch whose typed
  output feeds later deterministic computation.
- Add compact result-level schema repair for malformed material coverage
  fields.
- Add domain-neutral evals for the three required-material coverage modes.

### P0 Remediation Direction

1. **Ledger contract consistency.** Contribution aggregation semantics must be
   represented consistently in schema, prompt, runner, and validator. Standard
   aggregation labels such as `sum` and `add` must map to one canonical
   operation before reconciliation. Unsupported operations must fail with a
   precise typed diagnostic instead of being silently skipped.
2. **Structured ledger normalization.** Data runner results should normalize
   structural variants that are unambiguous and safe: candidate strings become
   candidate objects, contribution/reconcile group keys are canonicalized
   against `metric`, and normalized results are what validators consume.
3. **Runner ledger helpers.** Scripts should be able to call generic helpers
   such as `add_contribution`, `add_resolution`, `add_decision`, and
   `emit_result`. The helper layer owns JSON shape and canonical keys; the
   model owns business interpretation.
4. **Typed violation repair.** Repair should classify failures as typed
   loci, for example `reserved_helper_redefined`,
   `required_material_not_consumed`, `result_schema_mismatch`,
   `unsupported_contribution_operation`, or `reconcile_group_mismatch`. Repair
   prompts should receive the relevant compact locus rather than the whole
   previous script as the only context.

### P1 Remediation Direction

- Split large data jobs into staged batches: material coverage, parsing and
  normalization, decision/contribution collection, reconciliation, then final
  output rendering.
- Add runner-result schema repair and raw-result audit for malformed emitted
  JSON that can be losslessly repaired.
- Add semantic smoke checks for suspicious success states, such as strict
  aggregate answers with empty contribution ledgers.
- Expand eval coverage across simple sums, counts, rankings, joins, entity
  resolution, zero-valued groups, strict output formats, and non-text material
  extraction. These evals must remain domain-neutral.

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
- [x] Normalize contribution aggregation semantics across schema, prompt,
      runner, validator, and tests.
- [x] Normalize unambiguous runner-result schema variants such as string
      entity candidates and duplicated metric suffixes.
- [x] Add generic ledger helper APIs in the Python runner so scripts do not
      hand-build every result JSON structure.
- [x] Add focused runner tests for aggregation aliases, metric-suffix
      normalization, candidate-string normalization, helper-backed ledgers,
      and unsupported contribution operations.
- [x] Add typed data violation classification and compact locus-specific
      repair prompts.
- [x] Add generic material usage modes for direct script consumption,
      text-evidence consumption, planner distillation, and reference-only
      materials.
- [x] Validate required material coverage according to usage mode instead of
      treating every required material as direct script input.
- [x] Teach the data planner schema/prompt and JSON compatibility draft about
      `usage_mode`, `text_evidence_path`, and `distilled_notes`.
- [x] Add execution preflight for oversized one-shot data plans using typed
      structural signals, and repair them as bounded batches before running
      large scripts.
- [x] Classify `oversized_data_plan` and teach repair prompts to emit smaller
      bounded batches with `continue_after=true` when needed.
- [x] Let the Python data runner accept JSON-style `true`/`false`/`null`
      constants without requiring the model to replan.
- [x] Strengthen runner ledger helpers so safe structural aliases normalize to
      canonical result fields.
- [x] Normalize safe structural aliases in raw emitted result JSON for
      decision records, rule coverage, contributions, entity resolutions, and
      reconcile groups.
- [x] Classify numeric parse failures, unknown rule references, and missing
      entity-resolution status as typed repair loci.
- [x] Add a first domain-neutral data eval fixture for strict sum output with a
      required rule material.
- [x] Route CLI single-shot data requests through the same bounded
      plan/execute/repair/evaluate/continue workflow used by REPL instead of a
      one-run shortcut.
- [x] Add compact `[cli/data] data task result` telemetry so CLI data workflow
      completion is auditable without relying on answer prose.
- [x] Let eval cases lower the minimum-output-length sanity floor for strict
      scalar or strict-format answers, instead of padding product output.
- [x] Add deterministic data result patch audit records for safe structural
      shape fixes.
- [x] Start emitting typed validator violations with JSON paths and
      repairability for representative ledger/reconcile failures.
- [x] Continue reducing whole-script rewrites by adding deterministic local
      result-patch repair for safely repairable emitted result shapes that can
      be normalized without changing business semantics.
- [x] Merge runner helper ledgers into direct `emit({...})` results before
      validation so helper-backed scripts keep audit records without hand-built
      result JSON.
- [ ] Add model-authored typed patch IR for structural result repair only,
      with patch budget, full audit, revalidation, and hard prohibition on
      business-semantic patches.
- [ ] Add domain-neutral evals for ledger normalization, material usage modes,
      contribution/reconcile consistency, schema repair, and staged data
      workflows.
