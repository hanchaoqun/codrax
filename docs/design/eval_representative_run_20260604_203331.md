# Representative Eval Sweep — 2026-06-04 20:33

## Goal

Run a small but representative real-model eval sweep after recent feature work.
Do not rely only on PASS/FAIL verdicts: inspect answer content, logs, tool use,
and whether the result satisfies the user intent.

Concurrency target: 2 cases at a time.

## Selected Cases

| Area | Case | Purpose |
| --- | --- | --- |
| Code analysis | `qf_architecture` | Read-mode architecture explanation over current code. |
| External log | `logtri_go` | Runtime log triage with file/line grounding. |
| Log + source | `read_combo_log_current_source_explanation` | Mixed external observation and current-source explanation. |
| Trace platform | `trace_query_donghu_mixed_platform` | Donghu/Harmony priority/time semantics and mixed framework surfaces. |
| Trace frame flow | `trace_query_frame_timeline_flow` | FrameTimeline/frame_flow deterministic trace views. |
| Trace + source | `read_combo_trace_current_code_dimensions` | Trace artifact facts plus current-code dimensions without source/trace line confusion. |
| MCP external observation | `mcp_typed_line` | Typed MCP line-backed observations and source-lane discipline. |
| Multi-repo focused | `mr_focus_single` | No explicit focus: selector/focus must find the right subrepo for a precise fact. |
| Multi-repo comparison | `mr_cross_repo_compare` | Two named subrepos, symmetric comparison, no third-repo pollution. |
| Computer operation | `operation_system_inventory` | Operation route, command execution, final report quality. |
| Web operation | `operation_web_manual_summary` | Operation material retrieval, pagination/extraction, final summary. |
| Write mode | `patch_go_typo` | Plan/apply typo fix with precise patch output. |

## Run Results

Valid sweep root timestamp: `20260604-203424`.

Ignore the earlier `20260604-203400` result directories from the failed shell
driver attempt. The valid run used the copied binary
`.codrax-representative-20260604-203424` and executed at most two cases in
parallel.

| Case | Verdict | Manual check | Result directory | Key observations |
| --- | --- | --- | --- | --- |
| `qf_architecture` | PASS | OK | `eval/results/qf_architecture-20260604-203424` | Answer is grounded and useful. It used `read_file=8`, `repo_map=1`, `explorer_iters=10`. Process output still contains model `<think>` before final delimiter, but final answer is clean. |
| `logtri_go` | PASS | **BAD** | `eval/results/logtri_go-20260604-203424` | Final answer overclaims the panic cause from drifted stack-frame lines. It says `buildAnalysisIR` at `internal/agent/analyzer.go:250`, but the current checkout line 250 is in `prependAnswerPitfalls`; the model even noticed the mismatch during exploration, then still produced a current-source causal story. |
| `read_combo_log_current_source_explanation` | PASS | OK | `eval/results/read_combo_log_current_source_explanation-20260604-203810` | Mixed log + current source answer is mostly correct and cites current code separately from the external observation. It is relatively heavy (`read_file=13`, `repo_map=2`, `max_context_tokens_est=88239`) but acceptable for this case. |
| `trace_query_donghu_mixed_platform` | PASS | OK with attribution caveat | `eval/results/trace_query_donghu_mixed_platform-20260604-203810` | Answer preserves Donghu/Harmony priority semantics. Metrics show `tool_trace_query=0`; the answer text says "trace_query 识别到" because the user framed the question that way, but the evidence actually came from structured perf/trace pre-stage observations. |
| `trace_query_frame_timeline_flow` | PASS | OK | `eval/results/trace_query_frame_timeline_flow-20260604-204422` | Uses `trace_query=2`, `read_file=3`, and returns frame flow details with line-backed trace evidence. |
| `read_combo_trace_current_code_dimensions` | PASS | OK with minor overstatement risk | `eval/results/read_combo_trace_current_code_dimensions-20260604-204422` | Keeps trace lines and current-source thresholds mostly separate. Minor risk: wording can imply the current checkout itself "still exposes" the runtime issue when only threshold behavior was proven. |
| `mcp_typed_line` | PASS | ISSUE | `eval/results/mcp_typed_line-20260604-204911` | Efficient (`mcp_tool_calls=1`, no repo reads) and answer content is correct, but final answer includes noisy system supplement / missing-anchor caveat. Logs show `aggregate_facts[0]#current_source` even though the facts came from `mcp_resource`. |
| `mr_focus_single` | PASS | ISSUE | `eval/results/mr_focus_single-20260604-204911` | Exact focus pre-scan correctly selects `repo-stub-rust`; answer finds `repo-stub-rust/src/lib.rs:8` returning `42`. Final answer still appends an inactive-scope caveat for unrelated repos because the exact scalar principal answer was not recognized as a populated principal slate. |
| `mr_cross_repo_compare` | PASS | OK | `eval/results/mr_cross_repo_compare-20260604-205059` | Correctly compares the two named subrepos and avoids third-repo pollution. |
| `operation_system_inventory` | PASS | OK | `eval/results/operation_system_inventory-20260604-205059` | Operation route successfully returns system, memory, CPU, and GPU facts. |
| `operation_web_manual_summary` | PASS | **BAD** | `eval/results/operation_web_manual_summary-20260604-205259` | Operation loop found the real manual links (`codrax_authoritative_guide.html`, `user_guide.html`) but finalized before fetching them, returning a partial answer and asking the user to continue. This does not satisfy the user goal despite PASS. |
| `patch_go_typo` | PASS | OK | `eval/results/patch_go_typo-20260604-205259` | Write-mode typo patch completed through plan/apply verification in the eval fixture. |

## Issues Found

### EV-20260604-01 — Log stack-frame drift can still become a current-source causal claim

Severity: P0.

Evidence:

- Result: `eval/results/logtri_go-20260604-203424/run-1.out`.
- Log: `eval/results/logtri_go-20260604-203424/run-1.logs/codrax-20260604-203425-000-43978.log`.
- Current code has `renderRuntimeFrameDriftWarning` in `internal/context/builder.go`, and it explicitly warns that drifted frames must be treated as opaque runtime observations.
- The explorer observed that current `internal/agent/analyzer.go:250` is not `buildAnalysisIR`, and `buildAnalysisIR` is now near line 1716, yet the final answer still used the drifted stack frame as if it described current source.

Impact:

PASS is misleading: the answer can be confidently wrong for crash/root-cause
questions where the runtime binary and current checkout line map diverge.

Generalized fix direction:

- Promote runtime frame drift from a prompt warning to a typed answer-contract
  boundary. Drifted frames are valid runtime observations, but they must not
  create current-source causal anchors unless separate current-source evidence
  proves the connection.
- Finalizer/evaluator should consume this typed boundary, not the prose warning.
- Strengthen eval expectations so `logtri_go` fails if it attributes the current
  line content to the runtime stack when drift is detected.

### EV-20260604-02 — Operation route can stop with known next material unread

Severity: P0.

Evidence:

- Result: `eval/results/operation_web_manual_summary-20260604-205259/run-1.out`.
- Log: `eval/results/operation_web_manual_summary-20260604-205259/run-1.logs/codrax-20260604-205259-000-55488.log`.
- The operation loop executed four bounded batches and extracted the real links
  from the homepage:
  `./codrax_authoritative_guide.html` and `./user_guide.html`.
- It then finalized with "部分成功 / 需要补充操作" instead of running a fifth safe
  fetch/extract batch.
- `internal/repl/command_operation_cli.go` has continuation/replan loops and a
  fixed `commandOperationMaxCommandRounds=5`, but the command path still relies
  primarily on planner continuation and final answer generation.
- `internal/operation` has evaluator concepts, and
  `internal/repl/command_operation_planner.go` already renders
  `operation_materials` / `payload_material_excerpt`; however command CLI does
  not have an independent evaluator gate that blocks premature finalization
  when material coverage is obviously incomplete.

Impact:

The operation route may produce a polished partial answer for a task whose next
safe action is already known. This affects web pages, local files, MCP/Skill
payloads, downloaded documents, command output refs, and any task requiring
"read material -> extract -> continue".

Generalized fix direction:

- Add/route through a unified operation evaluator before final answer emission.
  It should read typed records, success criteria, material refs, discovered next
  links/actions, failures, and budget state.
- If a safe next action exists and budget remains, evaluator returns
  `continue`; if budget is exhausted, return `partial_answer_possible` with an
  explicit reason.
- Make operation command-round budget configurable in YAML and document the
  default.

### EV-20260604-03 — MCP aggregate facts can be mis-originated as current source

Severity: P1.

Evidence:

- Result: `eval/results/mcp_typed_line-20260604-204911/run-1.out`.
- Log: `eval/results/mcp_typed_line-20260604-204911/run-1.logs/codrax-20260604-204911-000-51565.log`.
- Typed MCP observations were present as `mcp_resource` rows, but the answer
  dynamic trace showed `aggregate_facts[0]#current_source`.
- `internal/types/answer_claim_binding.go` currently defaults aggregate facts
  with no origin to `current_source`.
- `internal/types/answer_aggregate_fact.go` can normalize explicit origin
  dimensions, but it cannot infer origin from surrounding external-observation
  sufficiency / MCP result context when the model omits the dimension.

Impact:

External-only answers may get noisy "system supplemented" caveats or repair
pressure meant for current-source citation lanes. The content can remain right,
but UX and handoff are muddier than necessary.

Generalized fix direction:

- Add origin-aware aggregate-fact defaulting. When the active lane is
  external/MCP/runtime and no typed current-source profile is required,
  aggregate facts without explicit origin should inherit the active external
  origin or become supporting external observations, not current-source
  principals.
- Keep mixed source+external behavior unchanged: explicit current-source facts
  still use current-source citation requirements.

### EV-20260604-04 — Multi-repo inactive-scope disclosure false positive for exact scalar answers

Severity: P1.

Evidence:

- Result: `eval/results/mr_focus_single-20260604-204911/run-1.out`.
- Log: `eval/results/mr_focus_single-20260604-204911/run-1.logs/codrax-20260604-204911-000-51593.log`.
- Pre-scan selected the right subrepo: `multi-repo: exact focus pre-scan selected 1 sub-repo(s): repo-stub-rust`.
- Final answer correctly found `repo-stub-rust/src/lib.rs:8`, but appended an
  inactive-scope caveat for `repo-greet-go,repo-tools-py`.
- `internal/types/inactive_scope_disclosure.go` has
  `answerDocumentPrincipalSlateIsEmpty`, but it only checks list/table blocks
  and misses principal summary/scalar exact answers.

Impact:

Correct exact answers in multi-repo mode can look artificially incomplete,
weakening trust and pushing users to inspect unrelated repos.

Generalized fix direction:

- Treat principal scalar/summary/exact-resolution blocks as a populated
  principal slate.
- If exact focus pre-scan selected the target repo and the final answer has a
  concrete current-source anchor, suppress inactive-scope disclosure unless the
  answer explicitly reports absence/unknown.

### EV-20260604-05 — Trace tool-origin wording can overstate which tool ran

Severity: P2.

Evidence:

- Result: `eval/results/trace_query_donghu_mixed_platform-20260604-203810/run-1.out`.
- Metrics show `tool_trace_query=0`.
- Final answer says `trace_query` identified or normalized facts because the
  user named `trace_query`, while the actual facts came from attached trace /
  perf-triage structured observations.

Impact:

The technical conclusion is correct, but the provenance wording is misleading.
This matters when users compare trace_query deterministic outputs versus
LLM-assisted perf triage.

Generalized fix direction:

- Add tool-origin neutral wording in finalizer prompt/typed bindings:
  distinguish `trace_query` result, `perf_triage` bundle, attached-trace
  observation, and model-authored closure fact.
- Do not claim a tool ran unless a corresponding tool result exists.

### EV-20260604-06 — Eval harness PASS is insufficient for recent capabilities

Severity: P1.

Evidence:

- `logtri_go` and `operation_web_manual_summary` both PASS while manual quality
  is BAD.
- `operation_*` cases report many stage metrics as zero because they bypass the
  4-stage read pipeline; PASS alone cannot validate operation task completion.

Impact:

Green eval dashboards can hide serious answer-quality and task-completion
regressions.

Generalized fix direction:

- Add semantic/manual-oracle checks for representative cases:
  - logtri drift: answer must state frame/current-code line drift and avoid
    unsupported current-source causal claims.
  - web/material operation: answer must include content from the actual manual
    pages, not only the homepage, unless evaluator reports budget exhaustion.
  - MCP line facts: no current-source supplement when no current-source lane is
    requested.
- Track operation-specific metrics: command rounds, material refs consumed,
  final evaluator status, partial-vs-complete.

### EV-20260604-07 — UTF-8/truncated-output extraction fragility during manual audit

Severity: P2.

Evidence:

- Some ad-hoc shell extraction commands over eval output hit invalid/truncated
  multibyte boundaries.

Impact:

This does not affect Codrax runtime behavior directly, but slows reliable eval
inspection.

Generalized fix direction:

- Add a small eval inspection helper that decodes with replacement and extracts
  final answer / rendered panels safely, rather than relying on raw `sed`/`awk`
  over possibly truncated terminal output.

## Root Cause Analysis

### Root Causes by System Layer

1. Runtime/current-source boundary is still partly prompt-driven.

   The frame drift renderer in `internal/context/builder.go` gives good textual
   guidance, but the answer contract does not yet carry a strong typed
   "drifted runtime frame cannot become current-source causal proof" boundary.
   The model can notice the drift during exploration and still synthesize an
   overconfident current-source explanation later.

2. Operation command path lacks a final material-coverage evaluator.

   `internal/repl/command_operation_cli.go` has a bounded multi-round command
   loop, replan, continuation, lint, and large-output payload refs. But it still
   allows the planner/final answerer to decide that "partial answer" is enough
   even when a safe, known next action exists. This is an architecture gap, not
   an HTML-specific case: any material-producing command, MCP provider, Skill
   provider, or downloaded file can exhibit the same failure.

3. Aggregate fact origin defaults are too source-centric.

   `CompileAnswerClaimBindingsFromAggregateFacts` defaults originless facts to
   `current_source`. That is safe for code enumeration, but wrong for an
   external-only lane where MCP/runtime/log facts are already sufficient and no
   current-source profile is active.

4. Multi-repo inactive-scope logic detects "empty answer" with a narrow shape.

   `answerDocumentPrincipalSlateIsEmpty` only recognizes principal list/table
   members. Exact scalar answers can be structurally complete while looking
   empty to this helper, causing an unnecessary inactive-scope caveat.

5. Eval result semantics are still too binary.

   PASS means the harness accepted the run; it does not necessarily mean the
   answer satisfied user intent or avoided provenance mistakes. This is
   especially visible for operation routes and external-observation cases.

## Generalized Task List

### P0

- Add a typed runtime-frame drift boundary to the answer contract and finalizer
  inputs. A drifted runtime frame may be cited as an external observation, but
  current-source causal claims require separate current-source evidence.
- Add a command-operation unified evaluator before final answer generation. It
  should decide `complete`, `continue`, `needs_clarification`, `blocked`,
  `budget_exhausted`, or `partial_answer_possible` from typed operation records,
  material coverage, and known next actions.
- Strengthen eval cases for `logtri_go` and `operation_web_manual_summary` so
  answer quality failures cannot pass only because process exit/verdict is OK.

### P1

- Make aggregate-fact origin defaulting lane-aware. External-only MCP/runtime
  closures should not synthesize `current_source` claim bindings unless a typed
  current-source lane is required or the fact explicitly declares current-source
  support.
- Fix inactive-scope principal slate detection to include scalar/summary/exact
  answers, and suppress inactive-scope caveats when exact focus pre-scan found a
  concrete answer anchor.
- Add operation-specific eval metrics: command rounds, material refs discovered,
  material refs consumed, evaluator terminal status, and partial/completion
  reason.

### P2

- Add provenance-neutral trace wording so final answers do not say `trace_query`
  ran when facts came from perf-triage or attached-trace pre-stage.
- Add eval inspection helpers that safely extract final answers and logs with
  invalid UTF-8 replacement.

## Next Verification Targets

- Re-run `logtri_go` after typed drift-boundary work and manually verify that
  the answer says the stack frame is from an observed runtime binary and that
  current checkout line 250 cannot be used as the direct root cause.
- Re-run `operation_web_manual_summary` after unified evaluator work and verify
  that the operation route fetches the discovered manual pages before answering,
  or explicitly reports budget exhaustion with the safe next action listed.
- Re-run `mcp_typed_line` after origin-defaulting work and verify there is no
  current-source supplement/caveat when only MCP typed rows are needed.
- Re-run `mr_focus_single` after inactive-scope fix and verify the exact scalar
  answer has no unrelated inactive-repo caveat.
