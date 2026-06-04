# Representative Eval Run

Date: 2026-06-04

Initial result root: `eval/results/representative-20260604-152306`

Valid rerun result root: `eval/results/representative-20260604-152440`

Parallel-2 rerun result root: `eval/results/representative-p2-20260604-153306`

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
