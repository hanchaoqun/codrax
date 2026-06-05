# Data Task Goal Workflow

## Problem

The data lane currently behaves like a one-shot runner:

1. classify `route=data`;
2. ask the model for one `TaskPlan`;
3. execute the script once;
4. render either the result or the execution error.

This is not sufficient for commercial data processing. Real data tasks often
need multiple bounded steps: inspect candidate files, normalize ambiguous
columns, retry after a script error, continue after a partial result, or stop
only when the user's output contract is satisfied. A single script failure such
as `NameError: print is not defined` should not end the task when the lane still
has budget and the model can repair the plan.

## Non-goals

- Do not route source-code, log, trace, write-mode, or mixed analysis through
  the data lane.
- Do not let the data runner perform hidden side effects. File writes, network,
  subprocess execution, install/uninstall, deletion, remote access, and desktop
  operations remain operation-lane work and must use operation risk/approval.
- Do not hard-gate on user prose or model prose. Hard decisions consume typed
  route, plan status, execution status, evaluator status, budget counters, and
  output contract fields.
- Do not build a second approval system inside dataquery.

## Design

### 1. Data plans become goal-aware batches

`dataquery.TaskPlan` gains optional workflow fields:

- `goal`
- `known_constraints`
- `missing_observations`
- `success_criteria`
- `next_batch`
- `why_this_batch`
- `continue_after`

The script remains a bounded read-only calculation over declared `input_paths`.
The workflow fields are advisory/typed planning state used by the REPL data
loop, not by the Python runner.

### 2. Data runner stays deterministic and read-only

The runner should support normal low-risk data calculations:

- common read-only standard-library imports;
- `open(path)` only for declared input files;
- small bounded `print(...)` debug output;
- helpers such as `csv_rows`, `json_load`, `read_text`, `parse_money`, `emit`.

Dangerous operations are not enabled in the runner. If the task requires
side effects, the planner returns `status=blocked` with a reason that the
operation lane must handle risk/approval.

### 3. Add a data workflow loop

The REPL data dispatch becomes a bounded loop:

1. initial plan;
2. execute current plan;
3. if execution fails, call `RepairDataTask` while repair budget remains;
4. if execution succeeds and the plan requests continuation, call
   `EvaluateDataTask`;
5. evaluator can return:
   - `complete`
   - `continue_data`
   - `needs_clarification`
   - `blocked`
   - `budget_exhausted`
   - `partial_answer_possible`
6. `continue_data` calls `ContinueDataTask` to produce the next bounded batch;
7. final output still respects `OutputContract`.

This mirrors the operation lane's target-driven control flow while keeping data
execution isolated and side-effect free.

### 4. Prompt and JSON compatibility

Every new model-facing field is schema-described in `emit_data_task_plan` and
parsed through the existing REPL structured-tool compatibility layer:

- camelCase / snake_case normalization;
- strings to arrays where schema permits;
- booleans/numbers coercion;
- JSON object fragment recovery.

If the JSON cannot be repaired, the loop reports a structured planner error
instead of executing an unsafe or guessed plan.

### 5. UX

The REPL shows low-noise lane events:

- route decision;
- data plan summary;
- data repair / continuation summary when the workflow advances;
- final data result or actionable blocked/clarification message.

Debug `print` output from the Python script is not the final answer and should
not pollute strict output contracts. The final answer is rendered from the
structured `Result.Answer`.

## Task Ledger

- [x] Record design and red lines.
- [x] Expand data sandbox for common read-only calculations with bounded debug
      `print`.
- [x] Add goal-aware fields to `TaskPlan` and the LLM plan schema.
- [x] Add data evaluation status/types.
- [x] Add `RepairDataTask`, `EvaluateDataTask`, and `ContinueDataTask`
      planner interfaces.
- [x] Upgrade `dataTaskDispatch` to a bounded workflow loop.
- [x] Add tests for safe `print`, repair after script failure, continuation,
      and strict output contract preservation.
- [x] Run focused tests.
- [x] Run full test suite.
