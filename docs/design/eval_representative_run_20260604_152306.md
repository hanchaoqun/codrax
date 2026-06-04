# Representative Eval Run

Date: 2026-06-04

Initial result root: `eval/results/representative-20260604-152306`

Valid rerun result root: `eval/results/representative-20260604-152440`

Parallel-2 rerun result root: `eval/results/representative-p2-20260604-153306`

Valid parallel-2 content-review run root: `eval/results/representative-p2-20260604-154448`

## Purpose

Run a representative sweep after recent feature work. The sweep must cover
code analysis, configuration reasoning, external log/trace observations, mixed
external+source analysis, single-repo and multi-repo routing, MCP typed-line
observations, computer-operation requests, and write-mode change planning/apply.

This document records findings as they are observed. Correctness is not judged
only by the eval PASS bit: each result must also be inspected for answer shape,
evidence separation, routing, and obvious user-facing quality issues.

## Selected Cases

| Area | Case |
|---|---|
| Single-repo architecture/code analysis | `qf_architecture` |
| Single-repo config reasoning | `read_combo_config_absent_present_mix` |
| Log-only external observation | `logtri_go` |
| Log + current source mixed analysis | `read_combo_log_current_code_boundary` |
| Trace platform/priority semantics | `trace_query_donghu_mixed_platform` |
| Trace frame flow | `trace_query_frame_timeline_flow` |
| Trace + current source mixed analysis | `read_combo_trace_current_code_boundary` |
| MCP typed-line external observation | `mcp_typed_line` |
| Multi-repo exact focus | `mr_focus_single` |
| Multi-repo comparison | `mr_cross_repo_compare` |
| Harmony source map | `harmony/arkts_repomap` |
| Diagram/handoff-style read answer | `read_combo_pipeline_sequence_table` |
| Write apply | `patch_go_typo` |
| Write plan | `patch_cpp_typo` |
| Computer operation: local system inventory | `operation_system_inventory` |
| Computer operation: web/manual retrieval | `operation_web_manual_summary` |

## Run Protocol

- Build one `codrax` snapshot before the sweep.
- Run cases in batches of four concurrent case processes.
- Run each case once (`N=1`) to maximize breadth while limiting provider cost.
- Inspect `summary.md`, `run-1.out`, and the latest debug log for every case.
- Classify provider/runtime outages as environment-blocked, not product
  correctness failures.

## Findings

### F1. Initial run was invalid because the eval binary snapshot moved provider config anchoring

The first run used a binary snapshot located under
`eval/results/representative-20260604-152306/codrax-snapshot`. Codrax anchors
`providers.yaml` lookup to the executable directory, so every case looked for:

`eval/results/representative-20260604-152306/providers.yaml`

and failed before reaching any agent/tool stage. This was an eval harness/run
protocol problem, not a product behavior result.

Generalized fix:

- keep using snapshot binaries for concurrency safety;
- when `providers.yaml` exists at the repository root, pass it explicitly via
  `--providers "$ROOT/providers.yaml"` for read/plan/apply eval dispatches;
- classify `providers.yaml: llm.default.provider is required` as
  `BLOCKED_PROVIDER provider_unconfigured` instead of a normal feature FAIL.

### F2. Valid rerun was blocked by provider balance

The rerun used the repository-root binary/config path and all 16 representative
cases reached LLM dispatch. Every case was classified as:

`BLOCKED_PROVIDER insufficient_balance`

No case produced a user answer, so answer-quality manual inspection could not
be performed in this sweep. This is an environment/provider outage, not a
feature verdict. The selected matrix remains useful and should be rerun once
provider balance is restored.

### F3. Parallel-2 rerun produced the same provider-blocked result

The same 16-case matrix was rerun with two concurrent case processes instead
of four. All 16 cases again classified as:

`BLOCKED_PROVIDER insufficient_balance`

This rules out case-level concurrency as the cause of the missing answers in
this sweep. No answer-quality manual inspection was possible because no case
produced a final answer.

### F4. Provider 402 is a matrix-level environment blocker, not a per-case product verdict

The repeated result across parallelism 4 and parallelism 2 is a deterministic
provider outage:

`LLM API error (status 402): insufficient_balance_error`

Observed impact:

- every representative case reached LLM dispatch and then failed on provider
  balance;
- no case produced a final answer, so answer quality could not be inspected;
- the runner still paid the cost of repo indexing, agent retries, extract/final
  fallback attempts, and write-mode setup before each case became blocked.

Generalized plan:

- add a representative-sweep preflight that performs a minimal LLM health check
  against the same provider/model configuration before launching the full
  matrix;
- if the preflight returns deterministic provider-auth/balance/config errors,
  stop the sweep early and emit one matrix-level `BLOCKED_PROVIDER` report;
- keep per-case provider classification for cases started before the outage is
  detected, but do not confuse it with product FAIL;
- when the provider recovers, rerun the same matrix and perform manual answer
  inspection on every non-blocked result.

This should remain eval-runner behavior only. Product runtime should still
surface provider failures clearly to the user, but eval should avoid spending
minutes on known-no-signal matrix runs.

Code-level status:

- `eval_detect_provider_blocked` already classifies timestamped control-plane
  402 / `insufficient_balance_error` log lines as
  `BLOCKED_PROVIDER insufficient_balance`.
- The detector deliberately ignores model/customer content lines such as
  `DEBUG [diag ...] ASSISTANT content`, so a customer log that mentions a 402
  string does not contaminate an eval verdict.
- `eval/run.sh` applies this classification after each case run and writes
  `BLOCKED_PROVIDER ...` instead of `FAIL`.
- The missing layer is sweep-level preflight: the matrix still starts every
  case, spends indexing/agent/retry time, and only then discovers the same
  deterministic provider blocker repeatedly.

Commercial-grade eval handling should add a small provider preflight before
launching a representative matrix:

1. Use the same binary, providers config, default/per-agent model route, CA,
   OAuth cache, and environment variables as the target sweep.
2. Send one minimal, low-token health request that exercises the same LLM
   adapter path but does not invoke repository indexing or tools.
3. Classify deterministic provider/config/auth/balance failures into a
   matrix-level status:
   - `provider_unconfigured`
   - `provider_auth_failed`
   - `insufficient_balance`
   - `model_unavailable`
   - `network_or_timeout`
4. Stop the matrix early for deterministic blockers such as 402 balance or
   missing provider config, and write one top-level report that all selected
   cases are `BLOCKED_PROVIDER` by environment.
5. For transient network/timeouts, allow an operator-controlled retry budget
   before marking the sweep blocked.
6. Keep product runtime behavior separate: normal Codrax usage should report
   the provider failure to the user, while eval avoids treating it as product
   correctness signal.

Task notes:

- Add `eval/provider_preflight.sh` or an equivalent helper in
  `eval/runner_lib.sh`.
- Add runner-lib tests for 402, missing provider config, model-list parse
  errors, OAuth failures, and quoted customer text containing a fake 402.
- Teach representative sweep scripts to run preflight once before spawning
  parallel jobs.
- Keep per-case provider-blocked classification as a fallback for outages that
  begin after the sweep starts.

### F5. Valid parallel-2 run produced 13 PASS / 3 FAIL, but manual review found hidden operation-route regressions

After provider availability recovered, the same 16-case matrix was rerun with
parallelism 2. Verdict distribution:

- PASS: 13
- FAIL: 3
- BLOCKED_PROVIDER: 0

Automated FAILs:

- `read_combo_config_absent_present_mix`: answer content was correct and
  explicitly said `explore_per_tool_default_cap` is "an independent/different"
  config item, but the regex only accepted a narrower wording such as
  "不是/separate/distinct". This is an eval assertion gap.
- `read_combo_trace_current_code_boundary`: answer used current-source anchors
  under `internal/types/...`, while the regex only accepted
  `internal/(analysis|tool|agent|orchestrator)/...`. This is an eval assertion
  gap unless the case explicitly requires those packages.
- `read_combo_pipeline_sequence_table`: answer incorrectly collapsed read mode
  to only `analyze -> finalize` even though exploration had collected the
  full `analyze -> explore -> extract -> finalize` evidence. This is a real
  handoff/finalizer compression gap.

Manual-review failures among PASS cases:

- `operation_system_inventory`: PASS regex matched OS/CPU/memory/GPU words, but
  the answer did not perform computer operation. It fell back to code-repo
  reasoning and said the current machine's real hardware information could not
  be obtained. This is a real operation routing/execution gap.
- `operation_web_manual_summary`: PASS regex matched manual/config/URL words,
  but the answer did not fetch `http://codrax.net/`; it searched local repo
  files, concluded the domain is not accessible, and summarized local docs.
  This is a real operation/web retrieval routing gap.
- `mcp_typed_line`: final answer was factually correct, but the run read MCP
  resources twice and then read three current-source files to satisfy citation
  pressure. The final answer also included system-generated dimension/support
  caveat text. This is an external-observation handoff/UX efficiency gap.

Generalized analysis direction:

- Operation requests in CLI eval are still able to fall through into the
  normal repo-analysis pipeline. The typed operation route must be available
  and preferred for explicit computer-operation goals in CLI as well as REPL,
  without keyword hard routing.
- External observation-only MCP answers should not be forced into current
  source citation repair when the user did not ask for source analysis. Runtime
  and MCP line-backed observations need a citation-equivalent lane that
  satisfies finalizer coverage without synthetic source reads.
- Finalizer must preserve explicitly requested stage enumerations from
  exploration/handoff; compression into a smaller conceptual summary should not
  drop required stages.

## Follow-up Task Ledger

- [x] Add operation representative cases:
  - `operation_system_inventory`
  - `operation_web_manual_summary`
- [x] Fix eval provider-blocked detection for missing provider config.
- [x] Fix eval runner to pass repository-root `providers.yaml` to snapshot
  binaries.
- [ ] Rerun the same 16-case matrix after provider balance is restored.
- [x] Rerun the same 16-case matrix with parallelism 2 to check whether the
  provider blockage was concurrency-sensitive.
- [ ] Add a representative-sweep provider health preflight so deterministic
  402/config/auth outages stop the matrix before case-level retries waste time.
- [ ] Fix or relax eval matchers that reject correct wording/path variants:
  `read_combo_config_absent_present_mix`,
  `read_combo_trace_current_code_boundary`.
- [ ] Investigate and fix finalizer compression dropping `explore`/`extract`
  in `read_combo_pipeline_sequence_table`.
- [ ] Investigate CLI operation routing so operation cases execute operation
  workflows rather than falling back to repo analysis.
- [ ] Investigate MCP/external-observation citation pressure that triggers
  unnecessary current-source reads for `mcp_typed_line`.
- [ ] Perform manual answer inspection for every non-blocked result:
  - verify factual answer content, not just PASS/FAIL;
  - check source vs external-observation citation separation;
  - check operation final reports are not raw execution dumps;
  - check write-mode plan/apply artifacts are scoped and test-aware.

## Validation

- `bash eval/runner_lib_test.sh` passed after adding the provider-unconfigured
  fixture.
- A snapshot-binary smoke run with explicit repo-root provider config now
  classifies as `BLOCKED_PROVIDER insufficient_balance`, not as a missing
  provider configuration failure.
