# Data Task Adaptive DAG Runtime

## Problem

The data lane was upgraded from one-shot execution to a repairable workflow,
but the core execution unit was still too large: a model-authored Python script
could try to inspect materials, interpret rules, normalize entities, compute
contributions, reconcile totals, and render the final answer in one batch.

That architecture can detect many failures, but it does not reliably converge.
Each repair round rewrites a large procedural script, so a local problem such
as an undeclared input path, a helper misuse, or an oversized batch can cause
the model to keep reshaping the same monolith instead of making progress
toward the user's goal.

The general fix is not a domain-specific prompt or more repair retries. The
data lane needs an adaptive, goal-driven DAG runtime:

```text
user goal
  -> model proposes the next atomic action batch
  -> system executes typed actions
  -> actions produce structured artifacts
  -> evaluator checks progress toward the goal
  -> graph expands, repairs one node, or finishes
```

The DAG is not a fixed pipeline. It grows from observations. A material profile
may reveal that an entity-resolution action is needed; a failed join may reveal
that another inspection node is needed; a reconciliation mismatch may repair
only the contribution node. The system should not ask the model to rewrite an
entire end-to-end script when only one node failed.

## Red Lines

- Do not hard-code business domains, file names, column names, output shapes,
  or customer-specific task semantics.
- Do not hard-gate on user prose or model prose. Hard decisions consume typed
  route, plan status, action kinds, material paths, runner telemetry,
  validator violations, budgets, and output contracts.
- Do not route source-code analysis, trace/log diagnosis, write mode, or mixed
  source questions through the data DAG unless the typed route says data or
  mixed data.
- Do not introduce side effects into data actions. Side effects remain
  operation-lane work with operation risk/approval.
- Do not remove the existing deterministic runner. It becomes the
  `custom_transform` fallback node, not the default architecture for complex
  tasks.

## Architecture

### 1. Typed Data Actions

`TaskPlan` now supports an optional `actions[]` batch. Initial action kinds:

- `material_inventory`: discover objective local material metadata;
- `inspect_material`: inspect concrete materials and emit profiles;
- `custom_transform`: run a bounded Python transform over declared input files.

Each action is atomic: it should answer one small observation or perform one
small transformation. Multi-stage tasks must split into multiple actions and
set `continue_after=true` when more graph expansion is expected.

### 2. Data Artifacts

`Result` now carries `artifacts[]`. Artifacts are structured, reusable outputs
from action nodes:

- id / kind;
- source paths;
- summary;
- fields;
- headers / samples / row counts;
- child artifacts for inventories or grouped profiles.

Planner, continuation, and evaluator prompts receive compact artifact previews
and counts. Full results remain in the existing audit files/logs.

### 3. Execution Semantics

When `actions[]` is present, REPL and CLI execute the action runner. When no
actions are present, existing `Runner` behavior remains unchanged.

`custom_transform` delegates to the existing safe runner with the action's
own `input_paths` and bounded script. This preserves the current sandbox,
helpers, coverage validation, ledgers, reconcile, output-contract handling,
and patch engine.

### 4. Staging Guard

The old monolithic guard remains for top-level scripts. Action plans add a
node-level guard: a single `custom_transform` script that is too large is
rejected as an oversized atomic action. The repair instruction asks the model
to split the DAG into smaller typed actions rather than rewriting another large
script.

### 5. Typed Repair Loci

Execution failures continue to be classified into typed violations. The first
new action-oriented violation is:

- `undeclared_input_path`: the script/custom transform read a path that was not
  declared in that action's `input_paths`.

The repair hint tells the planner to add the path only if the current action
should consume it; otherwise it should split a prior inspect/extract action or
remove the read. This keeps repair local to the graph node.

## Delivery Ledger

### Batch 1: Action Runtime Foundation

- [x] Record this adaptive DAG design and task ledger.
- [x] Add `DataAction` and `DataArtifact` typed structures.
- [x] Add `actions[]` to `TaskPlan` and `artifacts[]` to `Result`.
- [x] Implement `ActionRunner` with `material_inventory`,
      `inspect_material`, and `custom_transform`.
- [x] Wire REPL data dispatch to execute actions before falling back to the
      existing script runner.
- [x] Wire CLI data dispatch the same way, preserving CLI/REPL semantics.
- [x] Add action-aware prompt context, runner detail, and REPL audit preview.
- [x] Add `undeclared_input_path` typed repair classification.
- [x] Add focused unit tests for action execution and planner JSON
      compatibility.

### Batch 2: Adaptive Planner Contract

- [x] Make the data planner prefer action batches for complex tasks and use
      top-level `script` only for simple bounded transforms.
- [x] Add stronger continuation rules that consume `result.artifacts` as graph
      state rather than script prose.
- [x] Add evaluator checks for artifact progress:
      `complete`, `expand_graph`, `repair_node`, `continue_transform`,
      `blocked`, and `partial_answer_possible`.
- [x] Route evaluator statuses through distinct REPL/CLI branches:
      graph expansion and transform continuation use continuation planning,
      while node repair uses the repair planner with typed action/locus context.
- [ ] Add finer DAG-node budget accounting so one bad node cannot consume the
      entire data workflow budget.

### Batch 3: More Deterministic Atomic Ops

- [ ] Add `extract_records` for converting a profiled material into generic
      records.
- [ ] Add `derive_rules` for converting planner-distilled rules into typed
      rule artifacts.
- [ ] Add `normalize_entities` for generic source-to-canonical mapping
      artifacts.
- [ ] Add `compute_contributions` for generic contribution tables.
- [ ] Add `reconcile_artifacts` for deterministic artifact-level reconciliation.
- [ ] Keep all ops domain-neutral: no business-specific fields in Go logic.

### Batch 4: Node-Level Repair

- [ ] Return typed node failures with action id, action kind, script line,
      JSON path, and repairability.
- [ ] Repair only the failed node when possible.
- [ ] Escalate to graph expansion when a transform reads undeclared material or
      lacks schema knowledge.
- [ ] Use result patching only for structural JSON drift, never for business
      semantics.

### Batch 5: Eval and UX Hardening

- [ ] Add evals for non-procurement data tasks: CSV sum, JSONL filtering,
      multi-file join, text extraction summary, strict JSON-only output, and
      action-only inventory/inspection.
- [ ] Add regression checks that source, trace/log, operation, and write-mode
      routes do not see the data DAG unless typed routing selects it.
- [ ] Keep REPL previews compact and full artifacts/logs auditable.
- [ ] Keep CLI progress on stderr and final answer on stdout.
