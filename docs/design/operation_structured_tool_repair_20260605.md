# Operation Structured Tool Repair — 2026-06-05

## Problem Ledger

Customer logs showed command-operation continuation reaching a valid typed
evaluator decision, then failing while planning the next command batch:

- `command evaluation status=continue_command`
- next planner LLM response had `finish_reason=tool_use`
- tool argument decoding failed with `invalid character 'ä' looking for
  beginning of value`
- the system retried with the same large operation context, around 97 KB

This is not a command-execution safety failure: no command is run when the tool
parameters are invalid. The gap is recovery quality. A malformed structured
tool argument should not make the operation lane resend a large context and
wait minutes before either recovering or degrading.

## Root Cause

Existing operation structured tools already use the shared
`unmarshalReplStructuredToolParams` compatibility layer. That layer safely
handles mechanical JSON/schema slips such as:

- strings for booleans/numbers
- singleton objects for arrays
- JSON strings containing arrays/objects
- key aliases and schema-aware scalar repair
- recoverable trailing comma / wrapper suffix issues

The logged `ä` error is outside that repair boundary. It is not JSON-shaped
and cannot be safely converted to the target schema without inventing content.
The missing layer is not "more permissive JSON repair"; it is a bounded
operation-level repair/degrade protocol for unrepairable structured tool
arguments.

## Design

### Boundary

Keep JSON normalization centralized in `internal/repl/structured_tool_params.go`
and `internal/toolparam`. Add a typed parse-failure error so callers can
distinguish schema/tool-param corruption from normal planner/evaluator errors.

### Compact Repair Turn

For operation structured tools only:

- command-operation planner (`emit_command_operation_plan`)
- command-operation evaluator (`emit_operation_evaluation`)
- provider-operation evaluator (`emit_operation_evaluation`)

When typed parsing fails after normal compatibility repair:

1. classify the failure as `StructuredToolParamFailure`
2. issue at most one compact repair LLM call
3. include only:
   - required tool name and schema
   - parse error summary
   - compact user goal / policy / latest operation records
4. require the same tool call again
5. accept only if the repaired arguments pass the same normalizer and decoder

The repair prompt is schema-level. It must not inspect model prose or use
keywords from the user request as logic.

### Degrade

If compact repair also fails, return a typed error to the operation loop. The
loop must not execute anything from the malformed response. Existing operation
budgets and final-answer fallback can then produce a partial/failure report
from already-collected records.

### Red Lines

- Do not guess arbitrary non-JSON bytes into a valid command plan.
- Do not make this path global for code/log/trace tools in this batch.
- Do not parse model prose or user keywords for hard routing.
- Do not bypass command lint, approval, risk policy, or execution budgets.
- Do not resend full saved payloads/material excerpts during repair.

## Task List

- [x] Add typed structured-tool-parameter parse failure in the REPL layer.
- [x] Add compact repair prompt for operation structured tools.
- [x] Wire compact repair into command planner.
- [x] Wire compact repair into command evaluator.
- [x] Wire compact repair into provider evaluator.
- [x] Keep existing JSON normalizer as the first-line repair path.
- [x] Add focused tests for command planner, command continuation, command
      evaluator, and provider evaluator compact repair.
- [x] Keep repair prompts model-facing and free of internal product codenames.
- [ ] Future: surface a concise REPL/CLI permanent line when compact repair
      fails repeatedly and the operation degrades to a partial report.

