# Representative Eval Gap Audit - 2026-06-09

## Scope

本轮目标是读取当前代码后，挑选具有代表性的 eval cases，以 `PARALLEL=2` 运行验证，并人工审计最终答案与日志。审计原则：

- 不按单个 case 的字面值做补丁。
- 不把用户意图或模型输出散文做关键字匹配进逻辑。
- 不通过 prompt 红线规避问题。
- 系统级修复必须依赖 typed contract、artifact schema、ledger、reference metadata、deterministic validator 等结构化信号。

## Selected Cases

| case | 代表路径 | 选择原因 |
|---|---|---|
| `qf_architecture` | 普通 read-mode 架构问答 | 验证 analyze/explore/extract/finalize 主管道与条件预阶段的 grounded answer surface。 |
| `read_combo_trace_current_source_explanation` | runtime artifact + current source | 验证 trace 运行时事实与当前源码解释的 evidence boundary。 |
| `data_multifile_reference_projection` | data lane 多文件 DAG | 验证 labels/observations/targets 的 reference-complete projection、贡献记录、对账和纯数字输出。 |
| `mr_cross_repo_compare` | multi-repo scope routing | 验证两个显式子仓的 scope 保持、分桶回答和第三仓排除。 |

Run command:

```bash
CODRAX_BIN=/Users/han/opt/codrax/codrax \
CASES='eval/cases/qf_architecture.case eval/cases/read_combo_trace_current_source_explanation.case eval/cases/data_multifile_reference_projection.case eval/cases/mr_cross_repo_compare.case' \
PARALLEL=2 RUNS=1 TIMEOUT=1200 \
SUMMARY=eval/results/representative_eval_20260609_audit_summary.md \
bash eval/convergence_audit.sh
```

Build command before the run:

```bash
make
```

## Summary

Summary path: `eval/results/representative_eval_20260609_audit_summary.md`

| case | verdict | flags | manual audit |
|---|---|---|---|
| `qf_architecture` | PASS | `auto_repair` | Final answer is semantically acceptable. Mermaid source repair normalized a diagram. No correctness failure observed. |
| `read_combo_trace_current_source_explanation` | PASS | `contract_warning context_prune` | Answer covers trace span parsing, jank threshold, coverage/residue, and evidence boundary. However, generic user-visible warning text was appended despite PASS. |
| `data_multifile_reference_projection` | FAIL | `verdict` | Final answer was `17,4,5,0`; expected `17,0,5`. Terminal JSON status was `repair_node`. |
| `mr_cross_repo_compare` | PASS | `contract_warning` | Answer is bucketed by sub-repo and excludes the Rust fixture. Contract warning came from facet coverage/system supplement path, not a direct semantic failure. |

## Manual Audit

### qf_architecture

Result directory: `eval/results/qf_architecture-20260609-085217`

The final answer names the two conditional pre-stages (`StageLogTriage`, `StagePerfTriage`) and the four unconditional main stages (`StageAnalyze`, `StageExplore`, `StageExtract`, `StageFinalize`). It correctly describes the fixed main order and the `StageOutput`/`BusContext` handoff.

Log observations:

- `mermaid_source_repair_applied=1`.
- Answer contract violations were zero after rendering.
- The repair was deterministic diagram normalization, not a semantic repair.

Audit conclusion: PASS is credible. No action item beyond keeping diagram auto-repair visible as low-severity metadata.

### read_combo_trace_current_source_explanation

Result directory: `eval/results/read_combo_trace_current_source_explanation-20260609-085217`

The final answer correctly separates:

- runtime artifact facts: `RenderService DoFrame`, PID/time coordinates, 86.111ms duration;
- current-source facts: trace mark parsing, duration derivation, jank threshold, coverage/residue model;
- evidence boundary: one span can show a janky frame but cannot prove systemic jank.

Log observations:

- `answer_contract_violations=5`, `tool_history_prunes=2`.
- CGEC summary reported violations by `answer_claim_form_support`, `answer_prose_density`, and `inline_identifier`.
- The final answer still included a generic user-visible caveat: "答案在某些维度的覆盖度可能不充分...".

Audit conclusion: semantic answer is good, but the answer surface is not commercial-grade. Advisory contract warnings should not leak as vague generic caveats in a PASS answer. If a warning is critical, it should drive a typed retry or a specific caveat; if advisory, it should stay in operator telemetry.

### data_multifile_reference_projection

Result directory: `eval/results/data_multifile_reference_projection-20260609-085455`

Fixture facts:

```text
labels.csv:
A-one -> GroupA
A-two -> GroupA
Beta -> GroupB
Gamma alt -> GroupC

observations.csv active rows:
r1 A-one 10
r2 A-two 7
r4 Beta 4
r5 Gamma alt 5
r6 unmapped 11

targets.csv:
T1 GroupA
T2 GroupX
T3 GroupC
```

Correct result:

```text
17,0,5
```

Observed final output:

```text
17,4,5,0
```

The wrong answer came from projecting four reconcile/reference groups instead of the three rows in `targets.csv`. `GroupB=4` is a valid contribution group but not a target, so it must not appear in the final answer. The `unmapped` row must not create an output slot. `GroupX` has no included records and must be zero.

Key terminal artifact: `.codrax/data-audit/20260609-091117-102326-39412-terminal.json`

Terminal JSON:

```json
{
  "status": "repair_node",
  "data_rounds": 17,
  "repair_rounds": 6,
  "result_summary": "answer_len=8 decisions=5 rules=9 contributions=5 resolutions=23 consumed=15 warnings=0 reconcile=\"pass\""
}
```

Important action chain:

```text
join_obs_labels -> filter_active -> derive_value_num -> compute_value_sum -> continue_reconcile -> complete_output_contract_answer
```

The final `assemble_answer` action used:

```json
{
  "complete_reference": "true",
  "order_by": "group_key",
  "reference_key_field": "canonical_label",
  "reference_path": "coverage_records.json",
  "value_field": "actual"
}
```

This is the core bug. `coverage_records.json` is a broad coverage artifact; it is not the final output reference. The final output reference is `targets.csv#records` or `targets.csv` with `canonical_label`.

Deep root cause:

- The typed action runner can project correctly when the explicit reference contract is supplied. Existing unit tests already cover explicit reference projection yielding `17,0,5`.
- The workflow-level fallback selected a broader inferred reference candidate because current reconcile groups already contained non-target groups. Candidate scoring favored overlap with current groups instead of preserving the user's final output reference contract.
- `reconcile=pass` was self-consistency only. It validated computed groups against themselves, not final projection against the reference output universe.
- Terminal status was inconsistent: logs printed `terminal status=complete`, while terminal JSON status and eval verdict reported `repair_node`.
- Data metrics in the summary are mostly zero, so the eval table hides 17 data rounds, 6 repair rounds, and repeated action failures.
- `route=data` was present in stdout model text but absent as a structured log field, causing an extra `no_log_regex:route=data` verdict reason.

Audit conclusion: this is a systemic data workflow gap around final-reference authority, terminal status semantics, and data-lane observability. It should be fixed through typed contracts and validators, not by special-casing target names or output numbers.

### mr_cross_repo_compare

Result directory: `eval/results/mr_cross_repo_compare-20260609-085737`

The final answer is usable:

- `repo-greet-go` bucket includes `UserService`, `GreetServiceImpl`, `NewGreetServiceImpl`.
- `repo-tools-py` bucket includes `process_request` and `echo`.
- The third Rust fixture identifier is absent.

Log observations:

- `answer_contract_violations=1`, later normalized by deterministic answer-document metadata/row contract repair.
- No semantic hallucination observed.

Audit conclusion: PASS is credible. The remaining gap is answer surface noise: system supplement tables can duplicate already clear prose and should be governed by a typed "needed for repair/completeness" condition.

## System-Level Gaps

### G1. Final Reference Authority Can Drift To Broad Coverage Artifacts

The data workflow can lose the intended final output reference after intermediate contributions and reconciliation are available. When fallback projection infers a reference from current group overlap, a broad artifact such as `coverage_records.json` can beat the explicit target/reference table. This causes extra non-reference groups to appear in the final answer.

Generic fix direction: carry final reference authority as typed `OutputContract.complete_reference`, `reference_path`, and `reference_key_field`; prefer explicit contract/reference material over inferred overlap. Inference can remain a fallback, but it must not override a known final reference contract.

### G2. Reconcile Pass Is Not Final Projection Pass

`reconcile=pass` can mean "computed groups are internally consistent" while the final answer is still projected over the wrong key universe. The terminal summary currently presents this as healthy enough to confuse operators.

Generic fix direction: add a separate final projection validator that checks answer item count, projection metadata, and reference path/field against the typed output contract. This validator should not compare against hard-coded numeric expectations; it should validate structural projection completeness and ordering.

### G3. Terminal Status Has Conflicting Sources Of Truth

The data lane emitted terminal JSON `status=repair_node`, but logs also printed `terminal status=complete`. Eval trusted the JSON and failed, but operators see conflicting signals.

Generic fix direction: define a single terminal status source. CLI log, terminal JSON, and eval should all carry the same typed status and reason. "Workflow stopped" is not the same as "user goal complete".

### G4. Data Eval Metrics Are Blind To Data-Lane Runtime

The convergence summary reports zero analyzer/explorer/finalizer metrics for the data lane, which is expected, but it also hides data-specific signals: data rounds, repair rounds, action failure signatures, terminal path, terminal status, and final projected answer.

Generic fix direction: add data-lane metric extraction and summary columns. This keeps read-mode metrics intact while making data workflows auditable.

### G5. Artifact Schema Alias Hygiene Is Not Stable Enough

Several generated artifacts were record-shaped arrays but were treated as `node_class=artifact` or `json_shape=unknown` in later typed actions. The planner then oscillated between `qualify_records`, `extract_fields`, `derive_fields`, and custom transform repair.

Generic fix direction: generated typed-action outputs that are arrays of records must register stable record aliases and access hints. Downstream typed actions should consume those aliases deterministically instead of rediscovering shape from prose or diagnostics.

### G6. Advisory Contract Warnings Leak Into User-Facing Answers

PASS answers can still include generic warning paragraphs such as "coverage may be insufficient". This reduces commercial delivery quality and can make a good answer look untrustworthy without telling the user what is actually wrong.

Generic fix direction: separate operator telemetry from user-visible caveats. Critical contract violations should cause typed retry or specific caveats; advisory warnings should remain in logs/summary unless mapped to a precise, evidence-backed limitation.

### G7. Eval Assertions Mix Business Failure With Instrumentation Failure

The data case failed both because the answer was wrong and because `route=data` was not logged in the expected channel. Mixing these reasons makes triage noisier.

Generic fix direction: log route/classifier decisions as structured records and split eval verdict reasons by semantic output, terminal contract, and telemetry/log expectation.

### G8. Rich Upstream Evidence Can Be Lost Before Answer-Surface Compilation

Some PASS answers already contain the right principal members and evidence in prose, but downstream system supplements can still duplicate them because the final compiler only recognizes table/list carriers. This is not a prompt wording issue: the earlier stages have already produced typed aggregate facts, member notes, support refs, and evidence summaries, but the answer-surface backend must consume those structured handoff signals consistently across prose, table, and list carriers.

Generic fix direction: keep rich upstream evidence in typed handoff channels (`aggregate_facts`, `member_notes`, `support_refs`, evidence summaries, artifact lineage, and output-contract metadata) and let deterministic answer compilers/validators consume those signals. Do not infer principal coverage from user keywords or model intent prose; use row candidates, citation/location compatibility, structured facets, and authored carrier blocks.

### G9. Final Projection Value Binding Can Be Polluted By Output Groups

A later `assemble_answer` repair can inherit a prior `final_answer/projection` reconcile group. If that output group participates in business metric inference, the runner may fail to identify the single computed metric and fall back to an unrelated default metric. The reference-complete projection then preserves the correct key universe but fills every key as zero, while `reference_projected=true` metadata makes the completion gate think the answer is structurally complete.

Generic fix direction: final-output projection groups are audit/output lineage, not business contribution groups. Runner metric inference and workflow validators must exclude typed `final_answer/projection` groups when binding reference keys to reconcile values. Completion should require both projection shape metadata and typed value binding against reconcile groups when a values-style strict output is verifiable. Fallback projection plans should also carry structural input aliases for the reference, reconcile, and contribution artifacts so rich upstream evidence remains consumable by backend actions.

### G10. Stage-Like Architecture Questions Can Drift Across Topology Namespaces

The `qf_architecture` rerun answered the dataworkflow finite-state stages (`StageCoverRequiredMaterials` through `StageComplete`) instead of the read-mode orchestrator pipeline stages (`StageAnalyze`, `StageExplore`, `StageExtract`, `StageFinalize`) and conditional pre-stages. The analyzer had already identified a stage-like enumeration, but the typed handoff did not include the canonical stage-binding/topology authority, so exploration followed the first same-shaped enum family it saw. The existing quality gate caught same-area subtopic shape drift, but not the deeper namespace-authority issue.

Generic fix direction: stage/topology architecture answers need a typed authority handoff when multiple stage-like enum families exist. The system should expose canonical stage-binding/topology files as source authority and add them as soft disambiguation reads for stage-like architecture enumerations. This must not hard-code a case answer or parse model prose; it should consume typed RequestModel/entity shape plus repo structure, and final-answer checks should compare structured carriers against typed authority anchors rather than free-form text.

### G11. Explicit Contribution Group Fields Must Not Fall Back To A Synthetic Aggregate

The data rerun computed contributions with `group_key_field=canonical_label`, but the chosen mixed-source `coverage_records.json` rows that matched `active=true` were observation rows and did not carry `canonical_label`. `compute_contributions` treated the empty per-row group key as `all`, producing a self-consistent aggregate `37` instead of failing the field/materialization contract. Downstream reconcile and assemble were then internally consistent while structurally unable to project per reference key.

Generic fix direction: a typed action that receives an explicit grouping field must preserve that grouping granularity. If matched target rows do not have non-empty values for the explicit group field, the action should raise a typed dependency/field contract violation and route to existing join/enrich/materialization repair paths. A synthetic fallback group is valid only when the action explicitly requests a constant group or omits grouping entirely; it must not mask missing reference/materialized fields.

## Executable Task List

### Batch A - Data Reference Projection Correctness

1. Add workflow-level tests for reference fallback priority.
   - Target: `internal/repl/data_task_workflow.go`, `internal/dataworkflow/fallback_plan.go`.
   - Scenario: reconcile groups include `GroupA`, `GroupB`, `GroupC`, and `GroupX`; output contract or explicit reference material defines `targets.csv.canonical_label = [GroupA, GroupX, GroupC]`.
   - Expected: fallback plan emits `assemble_answer` with `reference_path=targets.csv` or `targets.csv#records`, not `coverage_records.json`.

2. Preserve explicit output reference contract across repair batches.
   - Target: `dataTaskOutputReferenceProjectionGap`, `BuildRequiredOutputProjectionPlan`.
   - Behavior: if `OutputContract.CompleteReference` plus `ReferencePath/ReferenceKeyField` are set, use that candidate first and allow projection to drop non-reference reconcile groups.
   - Validation: no keyword matching; rely only on typed contract fields.

3. Detect projected-reference metadata mismatch.
   - Target: `dataTaskResultHasReferenceProjection`, `CompletionGateGuardResult`.
   - Behavior: if a projection artifact says `reference_projected=true` but its `reference_path/reference_key_field` differ from the typed output contract, treat it as incomplete projection and repair with the contract reference.

4. Add end-to-end regression for the fixture shape.
   - Target: `eval/cases/data_multifile_reference_projection.case` plus Go unit tests near existing assemble/projection tests.
   - Expected: final answer `17,0,5`, terminal JSON status `complete`, contribution/reconcile ledgers present.

### Batch B - Data Terminal And Eval Observability

5. Unify data terminal status semantics.
   - Target: data workflow terminal writer in `internal/repl/data_task_workflow.go`.
   - Behavior: terminal JSON status, CLI log `terminal status=...`, and eval verdict read the same status value.
   - Validation: a `repair_node` terminal cannot be logged as `complete`.

6. Add data-lane metrics to eval summaries.
   - Target: `eval/run.sh`, `eval/convergence_audit.sh`.
   - Metrics: `data_rounds`, `repair_rounds`, `data_terminal_status`, `data_action_failures`, `data_final_answer_len`, terminal path.
   - Validation: data cases no longer show all-zero mechanism metrics without data-specific counters.

7. Log route decisions structurally.
   - Target: classifier/data route logging path.
   - Behavior: emit a stable log line or structured field for `route=data`, independent of model stdout.
   - Validation: eval log regex can assert route without reading model thought text.

### Batch C - Artifact Schema And Typed Action Stability

8. Stabilize record aliases for generated artifacts.
   - Target: `internal/dataquery/action_runner.go`, `internal/dataworkflow/artifact_schema.go`, `internal/dataworkflow/artifact_access_view.go`.
   - Behavior: typed actions that output JSON arrays of objects register record-shaped aliases usable by later typed actions.
   - Validation: `enrich_records` output can be consumed by `qualify_records`, `derive_fields`, and `compute_contributions` without `node_class=artifact json_shape=unknown`.

9. Make zero-row extraction diagnostics actionable without oscillation.
   - Target: `internal/dataworkflow/field_contract_issue.go`, `schema_violation.go`.
   - Behavior: when `extract_fields` returns zero rows because required fields are missing on mixed-source rows, recommend a source-narrowing or reference join path based on typed artifact fields.
   - Validation: repeated repair signatures stop after bounded attempts and transition to a deterministic fallback or a precise blocked state.

### Batch D - Answer Surface Quality

10. Keep generic contract warnings out of user-visible PASS answers.
    - Target: `internal/orchestrator/contract_check*.go`, answer rendering path.
    - Behavior: advisory warnings stay in telemetry; user-visible caveats must be specific and tied to exact typed limitations.
    - Validation: `read_combo_trace_current_source_explanation` final answer has no generic "coverage may be insufficient" paragraph when answer contract is passable.

11. Gate system supplement tables by actual need.
    - Target: answer-document post-processing and principal enum compile path.
    - Behavior: supplement tables appear only when they repair a missing typed surface or materially improve completeness, not as unconditional duplication.
    - Validation: `mr_cross_repo_compare` remains correct without unnecessary duplicate system sections unless the contract requires them.

12. Preserve rich upstream evidence through backend answer compilation.
    - Target: `internal/tool/answer_document_principal_enum_compile.go`, aggregate fact/evidence handoff consumers.
    - Behavior: deterministic compilers recognize typed member rows, support refs, locations, and authored prose/table/list carriers before adding supplements.
    - Validation: member notes or evidence summaries are consumed as structured handoff material; no prompt-only instruction or keyword match decides coverage.

13. Bind final reference projection values to typed reconcile groups.
    - Target: `internal/dataquery/action_runner.go`, `internal/repl/data_task_workflow.go`, `internal/dataworkflow/fallback_plan.go`.
    - Behavior: ignore typed final-output projection groups during business metric inference; reject `reference_projected=true` terminal answers when strict values output contradicts typed reconcile groups; include reference/reconcile/contribution aliases in deterministic projection fallback plans.
    - Validation: stale `final_answer/projection` groups cannot force zero-filled complete-reference output; value-mismatched projection metadata triggers deterministic `assemble_answer` repair.

### Batch E - Topology Authority And Group-Field Contracts

14. Handoff canonical read-mode stage/topology authority for stage-like architecture enumerations.
    - Target: `internal/types/stage_binding.go`, `internal/agent/analyzer.go`, answer-surface validators as needed.
    - Behavior: expose stage-binding/topology authority files through typed code, and merge them as soft required-file candidates when the analyzer emits a stage-like architecture enumeration that could otherwise drift to a sibling stage enum family.
    - Validation: architecture answers about pipeline stages inspect canonical `PipelineStage`/`StageBinding` authority before accepting a same-shaped alternate stage namespace.

15. Enforce explicit contribution group-field non-empty values.
    - Target: `internal/dataquery/action_runner.go`, `internal/dataworkflow/*failure*`, `internal/repl/data_task_workflow.go`.
    - Behavior: `compute_contributions` must fail with typed dependency/field contract when explicit group fields are absent or empty on matched target rows; repair should reuse `enrich_records`/`join_records` fallback mechanisms.
    - Validation: mixed-source record samples cannot collapse grouped contributions into `group_key=all` unless a constant group was explicitly requested.

### Batch F - Verification Sweep

16. Run focused unit tests after each batch.
    - `go test ./internal/dataquery ./internal/dataworkflow ./internal/repl`
    - Add narrower `-run` invocations for projection, terminal status, and artifact schema tests.

17. Re-run representative eval with `PARALLEL=2`.
    - Same four cases as this audit.
    - Pass criteria: all four PASS; data lane terminal status complete; no wrong reference projection; trace/multi-repo warnings do not leak generic caveats.

18. Run a broader data-focused eval slice.
    - Include data cases with missing reference keys, extra contribution groups, unmapped source rows, numeric parsing, and multi-file joins.
    - Pass criteria: no final projection uses a broad coverage artifact when an explicit reference contract exists.

## Commercial Delivery Bar

The fix is only commercially acceptable when:

- final data answer is correct and reference-complete;
- terminal status is single-source and machine-verifiable;
- operator logs explain the failing typed action and repair reason without requiring manual archaeology through huge logs;
- PASS answers do not carry vague user-visible warnings;
- upstream aggregate facts, evidence summaries, support refs, and artifact lineage remain available to backend compilers/validators;
- all changes are covered by structural tests and representative eval;
- no task uses domain-specific constants, target names, keyword intent matching, or model prose as a hard gate.

## Current Rerun Audit - 2026-06-09 11:30 CST

This rerun validates the code after the delivery commits through `22709693 ux: highlight multi-repo startup guardrails`.

Initial sandbox run failed all four cases with environment errors only: DNS lookup for `api.minimaxi.com` failed, and the sandbox could not write the user-level repomap cache. The same command was then rerun with approved escalation so provider access and cache writes were available.

Run command:

```bash
CODRAX_BIN=/Users/han/opt/codrax/codrax \
CASES='eval/cases/qf_architecture.case eval/cases/read_combo_trace_current_source_explanation.case eval/cases/data_multifile_reference_projection.case eval/cases/mr_cross_repo_compare.case' \
PARALLEL=2 RUNS=1 TIMEOUT=1200 \
SUMMARY=eval/results/representative_eval_20260609_current_summary.md \
bash eval/convergence_audit.sh
```

Summary path: `eval/results/representative_eval_20260609_current_summary.md`

| case | verdict | data status | notable flags | manual audit |
|---|---|---|---|---|
| `qf_architecture` | PASS | - | `contract_warning auto_repair` | Answer now names `log_triage`, `perf_triage`, and `analyze -> explore -> extract -> finalize`, and cites `internal/types/stage_binding.go`. PASS is credible. |
| `read_combo_trace_current_source_explanation` | PASS | - | `contract_warning context_prune` | Answer correctly explains trace parsing, `PerfFrameBudget60HzMs=16.67`, `86.111ms`, and the external-source evidence boundary. PASS is credible. |
| `data_multifile_reference_projection` | PASS | `complete` | none | Final output is `17,0,5`; terminal JSON reports complete, `data_rounds=8`, `repair_rounds=0`, `answer_len=6`. Prior final-reference and value-binding bugs are closed for this fixture. |
| `mr_cross_repo_compare` | PASS | - | `wide_search contract_warning` | Answer separates `repo-greet-go` and `repo-tools-py`, excludes the Rust fixture, and is semantically correct. PASS is credible. |

### Verified Closed Gaps

- G1/G2/G9: final reference authority, projection pass, and value binding are now working for the representative data fixture. The final `assemble_answer` uses `targets.csv` plus `canonical_label`, and the answer is projected in target order.
- G4/G7: the eval summary now carries data-lane status, rounds, repair rounds, answer length, and structured `route=data` logs.
- G10: read-mode topology authority reaches the answer. The architecture answer no longer drifts to dataworkflow stages.
- G11: explicit contribution grouping no longer collapses this fixture to a synthetic aggregate.
- G6: the old vague generic caveat is not present in the current trace final answer; the remaining caveat is specific to the external trace/source boundary.

### Current Manual Audit Notes

`qf_architecture` is semantically correct, but the final answer still appends a system supplement table after the model already covers stage bindings. This is not a correctness failure; it is an answer-surface necessity gap. System supplements should be rendered only when they repair or complete a typed surface that the authored answer did not already cover.

`read_combo_trace_current_source_explanation` needed an extractor retry. The first `emit_hypothesis_verdict` used file:line citations and was rejected as not grounded in the accepted investigation snapshot; the second attempt used `evidence_id` and succeeded. The final answer is good, but accepted evidence IDs should be a first-class backend handoff channel so extractor verdicts do not pay an avoidable retry.

`data_multifile_reference_projection` is correct, but the complete terminal log still includes an old nonterminal `last_error` about a prior dependency-rank violation. This makes complete terminal artifacts look partially failed and can mislead audit tooling. The same run also shows early rejected/deferred actions for cross-rank batches and invalid field references before deterministic recovery.

`mr_cross_repo_compare` is correct, but logs show scope efficiency issues: the analyzer first requested `source_inventory` where unavailable, a multi-repo parent graph compatibility fallback was used, and one source-inventory result appeared ambiguous enough that the model called `list_files` to verify subrepo contents. The final answer also repeats the same identifiers in prose, item rows, and a comparison table.

## Current System-Level Gaps

### G12. Complete Terminal Artifacts Retain Stale Nonterminal Errors

The data lane can finish with `status=complete` while `last_error` still points to a prior planning repair such as a dependency-rank violation. That error is useful history, but as a top-level `last_error` on a complete terminal it reads like the final state failed.

Generic fix direction: terminal status should expose final-state error separately from prior repair lineage. For complete terminals, `last_error` should be empty unless the final state itself is degraded; historical errors belong in a bounded `prior_errors` or `last_nonterminal_error` field with round/action metadata.

### G13. Data Action Admission Allows Avoidable Cross-Rank And Field-Contract Attempts

The data workflow recovered deterministically, but it still admitted candidate plans that crossed dependent DAG ranks or referenced fields not present on the selected artifact. These are typed structural issues detectable before execution.

Generic fix direction: add a deterministic action pre-admission layer that validates batch rank boundaries and artifact field contracts from the action graph and artifact schema. Invalid plans should be rejected before execution with repair hints, not executed and then rediscovered from model text.

### G14. Evidence-ID Handoff Is Not Preferred For Verdict Grounding

Accepted investigation evidence already carried stable `evidence_id` anchors, but the extractor first tried file:line citations that were outside the accepted snapshot contract. The backend repaired this, but at the cost of one extra LLM turn.

Generic fix direction: expose accepted evidence IDs as the preferred verdict grounding channel when the prior snapshot contains them, and let validators resolve IDs to citations deterministically. This is typed handoff, not prompt-only advice and not parsing answer prose.

### G15. Answer Supplement Necessity And Carrier Coverage Are Too Coarse

PASS answers can duplicate facts by combining authored prose, item rows, comparison tables, and system supplements. The issue is not semantic correctness; it is commercial answer quality and cognitive load.

Generic fix direction: answer-document compilers should compute typed carrier coverage over member rows, citation refs, locations, and facet IDs. Supplements should render only for missing or repaired typed coverage. Model prose may remain the visible surface, but hard coverage decisions must use structured row/citation compatibility, not user keywords or free-form model intent.

### G16. Multi-Repo Scoped Inventory Still Falls Back Too Broadly

The multi-repo run selected the correct two subrepos, but later `repo_map`/inventory flow still emitted parent-graph fallback and an ambiguous scoped inventory path. This increases `wide_search` cost and risks cross-subrepo contamination on larger fixtures.

Generic fix direction: preserve focus-selector results as typed active-subrepo scope throughout analyzer/explorer tool contexts. Scoped inventory calls should resolve against the exact active `root_rel` and should avoid primary-subrepo compatibility fallback when the requested subrepo is explicit and present.

### G17. Final Assembly Handoff Contains Excess Alias Fanout

The successful data `assemble_answer` retained a very large `input_paths` set with many alias variants. This preserves evidence but makes terminal artifacts and backend consumption noisy.

Generic fix direction: normalize data artifact aliases into canonical handoff paths while retaining lineage in machine-readable metadata. Backend actions should consume canonical aliases plus lineage, not long duplicate path lists.

## Current Executable Task List

### Batch G - Terminal Error Lineage

1. Add typed terminal error lineage fields.
   - Preserve final `status`, final `reason`, and final-state `last_error`.
   - Move previous nonterminal errors into bounded lineage metadata.
   - Keep terminal JSON and CLI log synchronized.

2. Add regression tests.
   - Complete terminal after earlier rejected/deferred plan has empty final `last_error`.
   - Non-complete terminal still exposes the actionable final error.

### Batch H - Data Action Pre-Admission

3. Add structural batch rank validation before executing data actions.
   - Reject batches that cross dependent DAG ranks while required validation stages remain.
   - Emit typed repair hints for the next executable rank.

4. Add field-contract validation before executing action batches.
   - Use artifact schema fields, not model prose.
   - Reject missing `status_fields`, explicit group fields, and value fields before action execution.

### Batch I - Evidence-ID Handoff

5. Surface accepted evidence IDs as preferred verdict anchors.
   - Build a typed snapshot of evidence IDs available to extractor.
   - Allow `emit_hypothesis_verdict` to resolve IDs deterministically to citations.

6. Add verdict grounding tests.
   - Confirmed verdicts with accepted evidence IDs pass without a repair retry.
   - Unknown evidence IDs remain rejected.

### Batch J - Answer Carrier Coverage

7. Add typed carrier coverage for authored answer blocks.
   - Compare member labels, roles, citations, and locations across prose/list/table carriers.
   - Suppress duplicate supplements when typed coverage is already complete.

8. Add regression tests for architecture and multi-repo answers.
   - Stage-binding supplement is omitted when authored blocks already cover all required stage rows.
   - Multi-repo comparison keeps bucketed content without repeating the same identifier set unnecessarily.

### Batch K - Multi-Repo Scope Handoff

9. Persist active subrepo focus as typed scope metadata for analyzer/explorer tools.
   - Require exact explicit `root_rel` resolution for scoped inventory.
   - Avoid parent/primary compatibility fallback when an explicit active subrepo exists.

10. Add scoped inventory tests.
    - `repo-tools-py` inventory cannot return `repo-greet-go` files.
    - Explicit two-subrepo comparison excludes inactive third subrepo unless requested.

### Batch L - Alias Normalization

11. Normalize data action handoff aliases.
    - Keep canonical artifact paths in action params.
    - Preserve duplicate aliases in lineage metadata for audit/debug only.

12. Add alias handoff tests.
    - `assemble_answer` can consume canonical reference/reconcile/contribution aliases.
    - Terminal artifacts stay compact while lineage remains available.

### Batch M - Final Verification

13. Run focused unit tests after each implementation batch.
14. Rebuild `codrax`.
15. Re-run the same four representative eval cases with `PARALLEL=2`.
16. Manually audit final answers and terminal logs again.

Commercial acceptance for this delta requires all current representative cases to remain PASS, data terminal `last_error` to reflect only final-state errors, no avoidable data action execution for typed rank/field violations, extractor verdicts to prefer accepted evidence IDs without a repair retry, and no duplicate answer supplements when typed authored coverage is complete.

## Final Verification Rerun After Batches G-I

Summary path: `eval/results/representative_eval_20260609_final_summary.md`

| case | verdict | flags | manual audit |
|---|---|---|---|
| `qf_architecture` | PASS | none | The final answer stayed semantically correct and no representative-stage regression was observed. |
| `read_combo_trace_current_source_explanation` | PASS | `finalizer contract_warning auto_repair` | The answer remained grounded. Batch I reduced one class of avoidable verdict-citation failures, but advisory answer-contract warnings can still appear in telemetry. |
| `data_multifile_reference_projection` | FAIL | `verdict` | Terminal status was `complete` with clean final `last_error`, but the final answer was `20,0,5` instead of `17,0,5`. |
| `mr_cross_repo_compare` | PASS | none | Scope and final answer stayed correct in this representative run. |

### data_multifile_reference_projection - New Root Cause

Batch G fixed the stale terminal-error surface: the final terminal artifact had `status=complete`, empty final `last_error`, empty `last_nonterminal_error`, and no failed data actions. The remaining failure is semantic and upstream of projection.

The initial typed action batch consumed `instructions.md`, `targets.csv`, `observations.csv`, and `labels.csv`. It also declared decision, contribution, and reconcile ledgers, but did not declare `coverage_contract.rule_coverage_required=true` and did not materialize a `derive_rules` ledger. The batch then ran:

```text
extract_records(instructions.md)
extract_records(targets.csv)
join_records(observations.csv, labels.csv)
```

The join action's model-authored purpose said "join and filter", but the typed `join_records` parameters only expressed an inner join on `raw_label`. Because no typed `filter_records` or `qualify_records` action ran before `compute_contributions`, an inactive source row remained in the contribution set. Reconcile and final projection were internally consistent over the polluted contribution ledger, producing `20,0,5`.

This is not a projection bug and not a duplicate/dedupe bug. It is a system handoff gap: rule-bearing materials can be consumed as ordinary record/text artifacts without becoming a required rule coverage ledger, so downstream validators cannot prove that inclusion/exclusion rules were applied to decision and contribution records.

### G18. Rule-Bearing Material Does Not Automatically Become Required Rule Coverage

Required text/rule materials can be part of a strict-output aggregation plan while `rule_coverage_required` remains false. Once that happens, typed DAG staging may allow join/compute/reconcile actions to run before any source-backed rule ledger exists. The model can still mention rules in `purpose` or `success_criteria`, but hard logic cannot rely on that prose.

Generic fix direction: when a complex strict-output or aggregation workflow declares required text/constraint materials and also requires decision/contribution/reconcile ledgers, the workflow contract must require rule coverage. Existing stage topology and deterministic fallback can then force a `derive_rules` batch before business computations and require item ledgers to link to source-backed rule IDs. The hard decision uses coverage materials, usage modes, ledger requirements, and rule-coverage records; it must not parse user intent keywords, action purpose text, or model-authored answer prose.

### Batch N - Rule Materialization Handoff

17. Add contract normalization for rule-bearing materials.
    - Detect required text/constraint materials through the existing coverage-material path/usage classifier.
    - When a complex strict-output or aggregation plan also requires decision/contribution/reconcile ledgers, set `rule_coverage_required=true`.
    - Preserve existing explicit rule coverage and validation-rule behavior.

18. Add completion/staging regressions.
    - A strict aggregation with required rule material cannot proceed to compute/reconcile completion with zero rule coverage.
    - The deterministic fallback emits a typed `derive_rules` batch for the missing rule ledger.
    - A plan that already has `derive_rules` or source-backed rule records remains accepted.

19. Re-run the focused data tests and the representative eval sweep after Batch N.

### G19. Current Completion Gate Is Not Applied Before Noisy Repair Status

Batch N's focused rerun produced the correct final answer `17,0,5` with rule
coverage, decision records, contribution ledger, reconcile report, and final
projection all present. The terminal still reported `repair_node` because the
evaluation path propagated an older action-dependency violation after later
typed actions had superseded it. The model's evaluation reason even described
the old workflow violation as stale, but that prose is not allowed to be a hard
gate and must not be parsed.

The systemic gap is priority ordering: the evaluator decision wrapper computes
the completion gate only when the model status is already `complete`. When the
model returns `repair_node`, a stale historical violation can outrank the current
typed completion state. That makes auditability worse and can fail a commercially
deliverable result even when the latest result satisfies the ledger graph,
output projection graph, reference projection contract, and validation gate.

Generic fix direction: every evaluation decision must derive the current
completion guard from typed state before interpreting the model status. For
noisy or terminal statuses such as `repair_node`, `blocked`, budget exhausted,
or needs-clarification, an empty current completion gate plus a typed terminal
answer artifact returns the answer and keeps older repair signals only as
lineage. Explicit `continue_data` remains a forward-progress signal and is not
short-circuited. If the gate is not empty, preserve the existing repair and
fallback behavior. The hard decision must use only typed validation,
ledger/output graphs, and result structure; it must not read user-intent
keywords, action-purpose prose, or model-authored evaluation prose.

### Batch O - Completion-Gate Precedence

20. Always compute the current completion gate before evaluation-status routing.
    - Use `dataTaskWorkflowCompletionGateGuardResultWithRepo` for complete,
      repair, clarification, blocked, and budget statuses.
    - Treat an empty current completion gate as `completion_satisfied` for
      noisy/terminal statuses only when the result has a typed terminal answer
      artifact.
    - Preserve the stricter structural helper as an additional success path,
      not the sole noisy-repair override.

21. Keep stale repair information auditable but non-terminal.
    - A superseded action-dependency violation remains visible in lineage and
      prior errors.
    - It cannot set terminal `repair_node` when the latest typed completion
      gate is satisfied.
    - Explicit `continue_data` continues to schedule the continuation planner.
    - Incomplete latest results still follow existing repair/fallback paths.

22. Add regression coverage.
    - A `repair_node` evaluation with a current satisfied completion gate
      returns the final answer.
    - A non-empty completion gate still drives the existing repair/fallback
      decision.
    - No regression depends on case labels, status words in user data, or model
      evaluation prose.

### G20. Multi-Source Aggregate Artifacts Can Win Reference-Universe Inference

After Batch O, the focused data rerun reached terminal `complete`, but the final
answer was `17,4,5,0`. The reference projection repair selected
`data_records.canonical_label` as the complete reference universe. That artifact
was a multi-source extracted union across source, lookup, and target materials,
so its key set included ordinary business groups as well as target keys. The
correct output surface was the single-source reference material's key universe.

This is a structural inference gap. A combined record artifact is useful for
calculation and lineage, but it is a weaker default reference universe than a
single-source material or explicitly declared reference path. Choosing the
combined artifact over an atomic material lets projection include keys that were
valid intermediate business groups but not members of the requested output
surface.

Generic fix direction: reference-universe inference should classify candidate
paths by artifact lineage. Explicit reference paths remain strongest. Among
inferred candidates, single-source material/artifact candidates should be tried
before multi-source aggregate artifacts. Multi-source artifacts remain available
as fallback when no atomic candidate can satisfy the typed output graph. This
uses source-path cardinality and artifact aliases only; it must not inspect
material names, field-name keywords, user prose, rule text, or model evaluation
prose.

### Batch P - Reference Candidate Lineage Priority

23. Split inferred reference-candidate paths by lineage.
    - Mark artifact IDs and aliases as aggregate when their artifact has more
      than one source path or multiple child material projections.
    - Keep source materials and single-source child artifacts in the primary
      candidate bucket.
    - Use aggregate artifacts only as fallback when no primary candidate
      satisfies the projection gap.

24. Preserve explicit contracts.
    - `output_contract.reference_path` and action `reference_path` remain
      authoritative.
    - Existing explicit-reference runner behavior is unchanged.
    - Existing overlap/cardinality scoring remains inside each lineage bucket.

25. Add regression coverage.
    - A multi-source combined record artifact with broader key overlap cannot
      outrank a single-source reference material candidate.
    - If no single-source candidate exists, the aggregate artifact fallback
      remains available.
    - Tests assert lineage and key-set behavior without file-name keywords or
      business-value constants in production logic.

### G21. Reference Projection Consumption Is Not Handed Off To Coverage Validation

The Batch P rerun showed `assemble_answer` planning with `reference_path` set to
the atomic reference material, but execution still failed coverage validation:
the required reference material was reported as not consumed. The action had
structurally used the reference path to project the output, yet the runner did
not add that reference path to the result's consumed-material ledger.

This is a handoff gap between deterministic action execution and backend
coverage validation. Reference projection is a real typed material read, not
just output metadata. If the runner records projection metadata but omits the
reference material from `ConsumedPaths`, downstream validators see a false
coverage miss and may force unnecessary repair loops.

Generic fix direction: when `assemble_answer` completes reference projection,
the artifact and result must carry the actual reference path as consumed source
material. This is based on the action's typed `reference_path` and the
structural candidate used by the runner; it must not parse script text, field
names, material names, user prose, or model repair prose.

### Batch Q - Assemble Reference Consumption Handoff

26. Mark reference-projection inputs as consumed.
    - Attach the selected reference path to the `assemble_answer` artifact's
      `source_paths`.
    - Merge those source paths into result `ConsumedPaths`.
    - Preserve no-reference assembly behavior.

27. Add runner regression coverage.
    - An `assemble_answer` action with a required reference material satisfies
      coverage validation after projecting with that reference path.
    - The final answer and reference projection metadata remain unchanged.

### G22. Noisy Repair Can Bypass Deterministic Completion Fallback

The Batch Q rerun reached a state with contribution records present and the
completion ledger graph requiring only `reconcile_artifacts`. The evaluator
returned `repair_node` for an older local node issue, so the decision layer went
to the repair planner instead of using the deterministic completion fallback for
the currently missing ledger. The run exhausted repair budget while the typed
next step was already known.

This is a priority gap, not a planner prompt gap. When the current completion
gate is non-empty and can build a deterministic fallback from the typed ledger
or output graph, that fallback is stronger than a noisy model repair status.
Repair planning should remain available only when no deterministic current-state
fallback exists.

Generic fix direction: compute completion fallback for every evaluation status
and let `repair_node` consume it before model repair planning. The fallback is
derived from typed validation/ledger/output graphs, so it does not parse user
intent, action prose, model repair prose, or data-value keywords.

### Batch R - Completion Fallback Before Repair Planner

28. Compute completion fallback independently of model status.
    - Build ledger/output fallback whenever the current completion guard is
      non-empty and deterministic fallback is available.
    - Keep existing `complete` behavior unchanged.

29. Route noisy repair through deterministic fallback first.
    - In `repair_node`, prefer completion fallback, then repair fallback, then
      repair planner.
    - Keep actual incomplete gates without fallback on the existing repair path.

30. Add decision regressions.
    - A `repair_node` evaluation with missing reconcile but available typed
      reconcile fallback returns the fallback plan.
    - Completion fallback still preserves the guard reason and source.

### G23. Completed Typed Workflow State Does Not Retire Stale Violations

The Batch R focused rerun produced the correct final answer `17,0,5`. The
latest ledger graph reported `next_stage=complete`, every required ledger was
satisfied, and the output projection graph was `satisfied` with a present answer
and complete reference projection. The terminal still ended as `repair_node`
because `NormalizeEvaluationForWorkflowState` applied historical
`workflow_violations` before checking the current typed completion state. Those
violations were real earlier in the run, but later typed actions had superseded
them.

This is a stale-lineage priority gap. A hard gate over current typed blockers is
still necessary, but historical violations must not outrank a later state whose
ledger graph and output projection graph prove completion. The fix must not
weaken `GateEvaluationWithWorkflowViolations` globally; the raw gate remains
correct for incomplete states. The retirement decision belongs at the
workflow-state normalization boundary and must use only typed stage, ledger, and
output projection fields.

Generic fix direction: add a reusable typed completion predicate for
`WorkflowStateView`. When `next_stage=complete`, no required ledger is
incomplete, and the output projection graph is satisfied with a present answer,
evaluation normalization should treat historical workflow violations as lineage
instead of terminal blockers. Incomplete states still use the existing violation
gate. This does not parse model prose, action purpose text, user intent, file
names, field-name keywords, or business values.

### Batch S - Typed Completion Retires Stale Violation Lineage

31. Add a workflow-state completion predicate.
    - Require `next_stage=complete` from typed state.
    - Require no incomplete required ledgers.
    - Require a satisfied output projection graph with an answer present.
    - Allow legacy state snapshots only when they still have a typed answer
      signal and no incomplete ledgers.

32. Apply the predicate before stale violation gating.
    - `NormalizeEvaluationForWorkflowState` should normalize noisy statuses to
      `complete` when current typed state is complete.
    - `ConservativeEvaluationFromWorkflowState` should not let the raw
      violation gate override a completed current state.
    - Incomplete states retain existing hard violation behavior.

33. Add regression coverage.
    - A complete workflow state with old action-dependency violations
      normalizes to `complete`.
    - The same violation still gates an incomplete state to `repair_node`.
    - Conservative fallback over a complete state ignores stale violations.

### G24. Terminal Journal Preserves Stale Blocked Decision After Completion

The Batch S focused rerun ended with the correct final answer `17,0,5`.
Terminal JSON showed all current completion signals satisfied:

- ledger graph `next_stage=complete`;
- all required ledgers present and `satisfied`;
- output projection graph `status=satisfied`, `answer_present=true`,
  `projection_artifact_present=true`, and `reference_complete=true`;
- result summary `answer_len=6`, `contributions=4`, `reconcile="pass"`.

The eval still failed with `data_terminal_status:blocked`. The terminal journal
preserved an older `decision.status=blocked` and top-level `last_error` from a
prior plan-admission failure where `assemble_answer` had been attempted before
the workflow stage allowed it. That blocked decision was useful lineage, but the
latest typed snapshot had already completed after a later deterministic
`assemble_answer` repair.

This is a second status-source priority gap. Evaluation normalization and the
terminal journal are separate consumers of workflow state. The journal has a
valid guardrail that prevents callers from writing `complete` over a genuinely
blocked state, but the guardrail must also consult the current typed
ledger/output snapshot. Otherwise old blocked decisions can override completed
terminal artifacts even when the answer and output projection graph are valid.

Generic fix direction: terminal journal status resolution should preserve a
non-complete base decision only when the current typed snapshot is not complete.
When ledger and output graphs prove completion, the terminal decision should be
`complete`, final `last_error` should be empty, and historical plan-admission or
field-contract errors should remain in bounded `prior_errors`,
`workflow_violations`, and `last_nonterminal_error` lineage. This uses typed
snapshot fields only; it must not parse answer prose, model evaluation text,
action purpose strings, field-name keywords, or business values.

### Batch T - Terminal Journal Completion Authority

34. Reuse the typed completion predicate for journal snapshots.
    - Require complete ledger/output graph before retiring a blocked base
      decision.
    - Preserve existing journal protection for incomplete snapshots.

35. Normalize terminal decision fields for completed snapshots.
    - Final status and decision status become `complete`.
    - Final `last_error` is empty.
    - Stale base decision reasons, violations, and next-actions are removed from
      the terminal decision and retained in lineage fields.

36. Add journal regression coverage.
    - A completed snapshot with stale blocked decision writes terminal
      `complete`.
    - Incomplete snapshots still preserve blocked/failed final status.

## Final Verification After Batch T

Representative sweep command:

```bash
CODRAX_BIN=/Users/han/opt/codrax/codrax \
CASES='eval/cases/qf_architecture.case eval/cases/read_combo_trace_current_source_explanation.case eval/cases/data_multifile_reference_projection.case eval/cases/mr_cross_repo_compare.case' \
PARALLEL=2 RUNS=1 TIMEOUT=1200 \
SUMMARY=eval/results/representative_eval_20260609_final_summary.md \
bash eval/convergence_audit.sh
```

Summary path: `eval/results/representative_eval_20260609_final_summary.md`

| case | verdict | data status | flags | manual audit |
|---|---|---|---|---|
| `qf_architecture` | PASS | - | `finalizer explorer_long contract_warning` | Semantically correct. It names conditional pre-stages, the four core read-mode stages, stage responsibilities, topology handoff, and cites canonical stage enum/binding sources. Remaining flags are answer-surface/efficiency telemetry; no stage-namespace regression observed. |
| `read_combo_trace_current_source_explanation` | PASS | - | `finalizer contract_warning auto_repair` | Semantically correct. It explains `ParseLine`, `classifyEventType`, `parseTraceMark`, 16.67ms frame budget, 86.111ms duration, and the external-runtime evidence boundary. The finalizer needed a structured block repair; final answer is grounded. |
| `data_multifile_reference_projection` | PASS | `complete` | none | Final answer is `17,0,5`; terminal JSON status is `complete`; ledger graph next stage is `complete`; output projection graph is `satisfied`; final `last_error` is empty and old plan-admission errors remain only as lineage. |
| `mr_cross_repo_compare` | PASS | - | none | Answer remains bucketed by `repo-greet-go` and `repo-tools-py`, lists the expected exported identifiers, and excludes unrelated sibling repo content. |

Data terminal artifact:
`.codrax/data-audit/20260609-154805-223668-39537-terminal.json`

Terminal JSON audit:

```json
{
  "status": "complete",
  "data_rounds": 12,
  "repair_rounds": 1,
  "result_summary": "answer_len=6 decisions=8 rules=9 contributions=3 resolutions=6 consumed=11 warnings=0 reconcile=\"pass\"",
  "decision": {"status": "complete", "reason_code": "complete"},
  "ledger_next_stage": "complete",
  "output_status": "satisfied"
}
```

Closed systemic issues in this final rerun:

- rule-bearing materials are represented as required rule coverage before
  downstream contribution/reconcile/final projection completion;
- reference projection uses atomic/reference lineage before aggregate fallback;
- `assemble_answer` hands reference material consumption to coverage validators;
- deterministic completion fallback is preferred over noisy repair planning;
- completed workflow state retires stale workflow violations in evaluation
  normalization;
- terminal journal status and decision use current typed ledger/output
  completion rather than stale blocked lineage.

Residual telemetry notes:

- `qf_architecture` still reports read-mode answer-surface telemetry
  (`contract_warning`) and an efficiency flag (`explorer_long`). Manual audit
  found no semantic stage/topology failure.
- `read_combo_trace_current_source_explanation` still needed a finalizer
  structured-block repair. Manual audit found the final answer grounded and the
  visible caveat specific to external trace/source boundaries, not generic.

## Current Representative Rerun - 2026-06-09 16:07 CST

After the previous delivery batches, the same four representative cases were
run again with `PARALLEL=2`.

Run command:

```bash
CODRAX_BIN=/Users/han/opt/codrax/codrax \
CASES='eval/cases/qf_architecture.case eval/cases/read_combo_trace_current_source_explanation.case eval/cases/data_multifile_reference_projection.case eval/cases/mr_cross_repo_compare.case' \
PARALLEL=2 RUNS=1 TIMEOUT=1200 \
SUMMARY=eval/results/representative_eval_20260609_current2_summary.md \
bash eval/convergence_audit.sh
```

Summary path: `eval/results/representative_eval_20260609_current2_summary.md`

| case | verdict | data status | flags | manual audit |
|---|---|---|---|---|
| `qf_architecture` | FAIL | - | `verdict explorer_long contract_warning context_prune` | The answer is semantically correct about the read-mode pipeline order and stage/agent bindings, but the rendered surface does not reliably carry the typed analyzer responsibility artifacts such as `AnalysisIR`, `TaskGraph`, `EvidencePlan`, and `AnswerContract`. |
| `read_combo_trace_current_source_explanation` | PASS | - | `finalizer contract_warning` | The answer is rich and semantically correct. It repeats the parse/jank/boundary content across summary, numbered steps, conclusion, trace facts, and requested-dimension supplement, but this preserves the user-requested dimensions and should not be weakened by simple dedupe. |
| `data_multifile_reference_projection` | PASS | `complete` | none | Final output is `17,0,5`; terminal status and decision are `complete`; final `last_error` is empty. A prior cross-rank planning issue remains only as `last_nonterminal_error` lineage. |
| `mr_cross_repo_compare` | PASS | - | `contract_warning` | Scope routing stayed correct and the principal prose separates the two subrepos. Logs still show a brief scoped-inventory ambiguity that required `list_files` confirmation. |

### G25. Read-Mode Stage Responsibility Artifacts Are Not First-Class Handoff

`qf_architecture` failed only the verdict regex:

```text
no_regex_match:(classif|分类|hypothes|假设|意图|理解|推断|识别|解析|静态分析|[Tt]ask.?[Gg]raph|任务图|AnalysisIR)
```

The final answer already named `StageAnalyze -> StageExplore -> StageExtract ->
StageFinalize`, the conditional pre-stages, `dispatchStage`, `StageOutput`,
`BusContext`, and the verified stage/agent/skill supplement. The missing piece
was not a wrong stage namespace; it was loss of the analyze-stage responsibility
and primary artifacts on the answer surface. The deterministic supplement had
authority for actor bindings, but not for typed responsibilities or outputs.

Deep root cause: `StageBinding` is the shared topology authority consumed by
orchestrator and finalizer supplement paths, yet it only carried
stage/agent/skill/terminal. Responsibility-level facts such as analyzer
classification, `AnalysisIR`, `TaskGraph`, `EvidencePlan`, and `AnswerContract`
lived in architecture docs and scattered comments, not in the same typed
handoff record used by backend answer compilation.

Generic fix direction: extend the stage topology authority with stage
responsibility and primary-artifact metadata, then render that metadata only
when stage-binding authority is already grounded or requested through a typed
stage-workflow dimension. This keeps the fix code-driven and general across
read-mode architecture questions. It does not parse user intent keywords, eval
regexes, or model-authored prose, and it does not suppress rich authored
answers.

### G26. Rich Correct Answers Must Not Be Treated As Defective Solely For Repetition

The trace answer repeats the same semantic material in multiple carriers:
principal summary, dimension headings, Step 1-5 explanation, conclusion, trace
facts, and the requested-dimension source-quote supplement. The answer is
semantically correct and preserves the explicit user dimensions. Reducing it by
simple dedupe would weaken user-visible richness and could remove useful
evidence boundary detail.

Generic fix direction: answer-surface work should separate semantic/citation
correctness from style density. A later carrier-coverage system may detect true
contradictions or missing typed rows, but it must not delete rich, supported
content merely because it is repeated across compatible carriers. Hard decisions
remain typed: block structure, citations, requested dimensions, evidence
lineage, and stage authority records, not user/model prose keywords.

## Current Executable Tasks

### Batch U - Read-Mode Stage Responsibility Handoff

1. Extend the canonical stage authority record with responsibility and
   primary-artifact metadata.
   - Keep one source of truth for stage, agent, skill, terminal status,
     responsibility, and artifacts.
   - Include `AnalysisIR`, `TaskGraph`, `EvidencePlan`, and `AnswerContract`
     on `StageAnalyze`.

2. Render responsibility metadata in the verified stage-binding supplement.
   - Render only when the existing grounded-source gate already allows the
     supplement.
   - Preserve the model-authored answer above the supplement.
   - Do not suppress requested-dimension supplements or rich Step sections.

3. Add regression coverage.
   - `ReadModeMainStageBindings` must carry non-empty responsibilities and
     artifacts.
   - The stage-binding supplement must include `AnalysisIR`, `TaskGraph`,
     `EvidencePlan`, and `AnswerContract`.
   - The no-grounded-source gate must continue to prevent ungrounded
     supplements.

4. Rebuild and rerun validation.
   - Run focused unit tests for `internal/types` and `internal/agent`.
   - Rebuild `codrax`.
   - Rerun `qf_architecture`, then the full four-case representative sweep with
     `PARALLEL=2`.

Batch U acceptance criteria:

- `qf_architecture` PASS without relying on prompt changes or eval-regex
  special-casing;
- trace answer richness is not reduced;
- no production branch parses model prose, user-prose keywords, file-name
  constants, or business values as hard gates.

### G27. Parenthetical Category Qualifiers Can Hide Existing Architecture Carriers

The first Batch U focused rerun fixed the original `AnalysisIR` / `TaskGraph`
visibility gap, but `qf_architecture` still failed on:

```text
banned:系统按已验证证据补充
```

Manual audit showed the failing surface was not the new stage-binding table. It
was the older principal-enumeration supplement:

```text
系统按已验证证据补充缺失成员：条件性前置 stage（ADVISORY）（1）
FallbackResetTarget @ internal/types/context.go:2709
```

The authored answer already had a section titled `条件性前置 stage（按需执行）`
and explained `StageLogTriage`, `StagePerfTriage`, and
`StageMultiRepoFocus`. The compiler failed to treat that section as the same
category because the set label used a different parenthetical qualifier
(`ADVISORY`). It then appended a competing system table and, worse, pulled in
`FallbackResetTarget`, a reset-depth enum from a different namespace, as a
missing member.

Deep root cause: principal-enumeration carrier coverage was too table/list
centric and too literal about parenthetical label qualifiers. Architecture
explain answers often use prose sections as the natural carrier for typed
category coverage; in that mode, bracketed qualifiers should refine the label,
not split the namespace into a new missing-member table.

Generic fix direction: for architecture-explain answer surfaces, allow
summary/section blocks with the same parenthetical-stripped category label to
act as authored carriers. This suppresses competing system supplements when a
rich prose section already covers the category. Direct enumeration questions
still keep strict row supplements. The decision uses typed intent/scenario,
block kind, and deterministic label-normalization; it does not inspect model
rationale, eval banned strings, file names, or business values as hard gates.

### Final Verification After Batch U

Focused qf verification:

```bash
CODRAX_BIN=/Users/han/opt/codrax/codrax \
CASES='eval/cases/qf_architecture.case' \
PARALLEL=1 RUNS=1 TIMEOUT=1200 \
SUMMARY=eval/results/representative_eval_20260609_qf_after_stage_resp2_summary.md \
bash eval/convergence_audit.sh
```

Result: `qf_architecture` PASS, no flags.

Representative sweep:

```bash
CODRAX_BIN=/Users/han/opt/codrax/codrax \
CASES='eval/cases/qf_architecture.case eval/cases/read_combo_trace_current_source_explanation.case eval/cases/data_multifile_reference_projection.case eval/cases/mr_cross_repo_compare.case' \
PARALLEL=2 RUNS=1 TIMEOUT=1200 \
SUMMARY=eval/results/representative_eval_20260609_after_stage_resp_summary.md \
bash eval/convergence_audit.sh
```

Summary path: `eval/results/representative_eval_20260609_after_stage_resp_summary.md`

| case | verdict | data status | flags | manual audit |
|---|---|---|---|---|
| `qf_architecture` | PASS | - | `contract_warning auto_repair` | The answer keeps stage responsibilities visible and includes `AnalysisIR`, `TaskGraph`, `EvidencePlan`, and `AnswerContract`. The stale `FallbackResetTarget` supplement is gone. |
| `read_combo_trace_current_source_explanation` | PASS | - | `finalizer contract_warning context_prune` | The answer remains rich and semantically correct. Repeated carriers preserve requested dimensions rather than weakening the answer. |
| `data_multifile_reference_projection` | PASS | `complete` | none | Final answer is `17,0,5`; terminal status/decision are complete; `last_error` is empty; previous rank violation remains lineage only. |
| `mr_cross_repo_compare` | PASS | - | `contract_warning` | The final answer remains separated by subrepo and names the expected exported identifiers. |

Closed by this delta:

- G25: stage responsibilities and primary artifacts now live in typed stage
  authority and reach final answer surfaces.
- G26: trace richness was preserved; no dedupe/trim fix was introduced.
- G27: architecture prose sections with equivalent parenthetical-stripped labels
  now suppress competing principal-enum supplements.
