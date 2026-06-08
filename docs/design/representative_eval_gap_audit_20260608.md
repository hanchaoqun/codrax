# Representative Eval Gap Audit - 2026-06-08

本文记录 2026-06-08 对当前代码的代表性 eval 审计。目标不是拟合单个 case，也不是增加 prompt 红线，而是从答案和运行日志中找出可泛化的系统级 gap，并拆成可执行任务。

## Scope

- Repository: `/Users/han/opt/codrax`
- Binary: 当前工作区 `make build` 产物
- Parallelism: 每批 2 个 case
- Initial summary: `eval/results/representative_eval_20260608_summary.md`
- Rerun summary: `eval/results/representative_eval_20260608_rerun_summary.md`
- Focused data fix summary: `eval/results/data_reference_projection_20260608_fix4_summary.md`
- Worktree note: eval 输出位于 `eval/results/` 和 `.codrax/data-audit/`，由现有 ignore 规则处理；代码批次按本文档的 Delivery Status 分批提交。

## Commands

```bash
make build

PARALLEL=2 RUNS=1 TIMEOUT=1200 \
  SUMMARY=eval/results/representative_eval_20260608_summary.md \
  bash eval/convergence_audit.sh

CODRAX_BIN=/Users/han/opt/codrax/codrax \
  CASES='eval/cases/data_multifile_reference_projection.case eval/cases/mr_cross_repo_compare.case' \
  PARALLEL=2 RUNS=1 TIMEOUT=1200 \
  SUMMARY=eval/results/representative_eval_20260608_rerun_summary.md \
  bash eval/convergence_audit.sh
```

第二条 rerun 是为了排除 eval harness 的共享 snapshot 二进制问题。首轮中 `data_multifile_reference_projection` 和 `mr_cross_repo_compare` 的 `read_exit:127` 来自 harness 环境，不是产品行为。

## Case Results

| Case | Result | Manual verdict | Notes |
| --- | --- | --- | --- |
| `qf_architecture` | PASS | Correct enough | 回答命中 read-mode pipeline，引用 `internal/types/enums.go` 和 `internal/orchestrator/scheduler.go`。隐藏信号：`answer_contract_violations=1`，`mermaid_source_repair_applied=1`，但最终 PASS。 |
| `qf_config_precedence` | PASS | Correct | 回答准确说明 default 50、YAML `pipeline_max_steps`、CLI `--pipeline-max-steps`，以及 default -> yaml -> CLI 优先级。隐藏信号：`answer_contract_violations=1`。 |
| `read_combo_log_current_source_explanation` | PASS | Correct | 正确区分 runtime log 事实和当前源码，引用 `internal/agent/agent.go`、`internal/llm/retryable_error.go`、`internal/orchestrator/orchestrator.go`、`internal/render/status_messages.go`。 |
| `read_combo_trace_current_source_explanation` | FAIL | Real system gap | 答案只保留外部 trace 事实，未输出当前源码 file:line。日志显示探索曾读取 `internal/tracequery/parse.go`，但 artifact-only sibling 先完成并触发 observation-only disposition。 |
| `data_multifile_reference_projection` | FAIL on rerun | Real data workflow gap | 正确答案应为 `17,0,5`，实际输出 `17,4,5,0,0,0,0,0`。系统知道 `targets.csv` 只有 3 个目标，但 terminal complete 接受了 8 个输出 slot。 |
| `mr_cross_repo_compare` | PASS on rerun | Correct but noisy | 答案正确命中 Go/Python repos，未混入 Rust unrelated symbol。指标显示 `repo_map=10`, `list_files=2`, `source_lens=4`，标记 `wide_search`。 |

## Manual Answer Audit

### 1. QF architecture and config

两个 query-finding case 的答案内容可信，引用面也合理。它们的共同问题不是正确性，而是 PASS 摘要没有显式暴露 auto-repair / contract warning。对于代表性 sweep，隐藏的 `answer_contract_violations=1` 会让人工审计低估系统不稳定性。

泛化判断：这类信号不应该变成硬失败，但应进入 advisory flags。它们是质量漂移和 prompt/schema 兼容风险的早期指标。

### 2. Log + current source

`read_combo_log_current_source_explanation` 的答案质量好。它把 runtime artifact 的“旧日志事实”和当前源码的“当前实现机制”分开：

- `Agent.Run` 在发请求前记录 `phase=llm_request`
- first-byte timeout 被 `ErrStreamFirstByteTimeout` 判定为 retryable
- orchestrator 有同类错误 retry cap
- 渲染层会把流式首包超时显示成可读状态信息

该 case 证明 mixed runtime artifact + current-source lane 可以工作，所以 trace failure 不是这一类任务整体不可行，而是 trace 路径上的组合条件被错误消解。

### 3. Trace + current source

失败答案没有当前源码 file:line，且把“当前仓库中未找到 HarmonyOS / OpenHarmony RenderService 的 atrace / hitrace 解析实现源码”当成结论。人工审计日志显示这不是事实边界：

- `internal/tracequery/parse.go:616` 识别 `raw == "print" || raw == "tracing_mark_write"`。
- `internal/tracequery/parse.go:618` 返回 `EventTraceMark`。
- `internal/tracequery/parse.go:788` 的 `parseTraceMark(fields string)` 拆分 `B|pid|name` / `E|pid` 语义。

关键日志链路：

- analyzer emitted `external_observation_policy.current_source_mode="allow"`，rationale 写明“当前请求同时要求解释源码如何解析 span”。
- analyzer 没有 emitted `current_source_explanation_profile`，`required_files` 也为空。
- explorer 的第 1 路 grep/read_file 实际读到了 `internal/tracequery/parse.go`。
- 另一个 artifact-only sibling 先调用 `emit_investigation_complete`，带 `evidence_floor_waiver.reason="external_only_trace"`。
- `HasEnoughFacts promoted by external observation sufficiency` 后，finalizer prompt 的 Runtime Grounding Disposition 渲染为 observation-only，并指示“typed request policy explicitly excludes current checkout/source evidence”。

深层根因：

1. `allow` 被当成“源码可选”，而不是“混合请求中源码 lane 仍需完成”的精确信号。
2. 没有 `CurrentSourceExplanationProfile` 时，`AssessExternalObservationSufficiency` 可把 small external observation set 提升为 enough facts。
3. parallel exploration 允许 artifact-only sibling 先完成并取消/覆盖包含 current-source evidence 的 sibling。
4. finalizer 的 runtime grounding disposition 把 external-only artifact citation policy 过度投射成 observation-only source exclusion。

泛化判断：不能靠改 prompt 强压“必须读源码”。应把“混合 runtime artifact + current-source explanation”表示成 typed lane obligation，并让 sufficiency、parallel close、finalizer disposition 共同消费同一个精确信号。

### 4. Data reference projection

fixture 语义：

- `targets.csv`: `GroupA`, `GroupX`, `GroupC`
- `labels.csv`: `A-one/A-two -> GroupA`, `Beta -> GroupB`, `Gamma alt -> GroupC`
- active observations: `GroupA=10+7`, `GroupB=4`, `GroupC=5`
- user-facing projection must follow targets order and fill missing target with zero: `17,0,5`

实际输出：`17,4,5,0,0,0,0,0`。

关键日志链路：

- round 10 `assemble_answer` declared `complete_reference=true` and claimed it would project reconcile groups into `targets.csv` order.
- terminal artifact shows `answer_len=16`, `group_count=8`，且 final answer 为 `17,4,5,0,0,0,0,0`。
- terminal reason 同时写出 `targets.csv has 3 targets(GroupA,GroupX,GroupC)` 和 `Answer "17,4,5,0,0,0,0,0" is comma-separated numeric values`，但仍 `terminal status=complete`。
- checkpoint/result 中仍可见 `status="missing_projection"`，但 completion path 没有把最终 cardinality mismatch 作为 hard block。

深层根因：

1. contribution/reconcile ledger 对贡献组校验为 pass，但 user-facing reference projection 没有独立 hard invariant。
2. `complete_reference=true` 的语义没有保证“输出 slot 数 == reference row count”。
3. 非 target 组 `GroupB` 被带入最终输出，target 中存在但 reconcile 中缺失的 `GroupX` 没有被强制输出为 0。
4. terminal complete 消费了“ledger satisfied / reconcile pass”这些上游信号，却没有复核最终 answer 的结构形状。

泛化判断：data workflow 需要把 contribution ledger 和 final reference projection 分成两个 typed validation surfaces。reconcile pass 只说明贡献账本正确，不说明最终用户投影正确。

### 5. Multi-repo compare

rerun 后答案正确，且没有混入 Rust fixture。问题主要是效率：tiny fixture 里仍出现多次 repo_map/list_files/source_inventory lens。这个不是 P1 正确性缺口，但代表 multi-repo scoped search 的可改进空间。

泛化判断：当 focus selector 已经 pin exact subrepo / active set 时，可以生成一次 deterministic scoped inventory，后续探索复用，不必反复扩大搜索。

## System-Level Gaps

### G1. Mixed external artifact + current-source explanation can collapse to observation-only

Priority: P1

Current behavior: trace case 中 analyzer 的 typed policy 为 `current_source_mode=allow`，但没有 active `CurrentSourceExplanationProfile`。下游把 external observation sufficiency 视为 enough facts，并最终渲染 observation-only disposition。

General gap: “artifact 的事实引用必须 external-only”和“请求还要求解释当前源码机制”是两条 lane。前者不应消除后者。

Do not fix with: prompt-only “看到结合当前源码就必须读源码”。这会违反硬门只吃精确信号的原则。

Preferred fix shape:

- analyzer/parser 层把 current-source bridge 表示为 typed profile 或等价 precise lane obligation。
- sufficiency 层在 required current-source lane 未完成时返回 `blocked_by_current_source`。
- parallel close 层等待或合并 required lane evidence。
- finalizer disposition 只约束 artifact-origin claims，不宣称 typed policy excludes current source，除非 `CurrentSourceLaneDecision()==excluded`。

### G2. Data final projection lacks reference-cardinality invariant

Priority: P1

Current behavior: data workflow terminal complete 接受了 8 个 output slot，虽然 reference set 只有 3 rows。

General gap: output_contract 的 final answer 应当有 machine-checkable shape。reference projection 场景至少需要：

- output slot count equals reference row count
- every output slot maps to exactly one reference key in reference order
- reference-missing contribution defaults are applied by typed rule
- non-reference contribution groups remain audit-only unless explicitly requested
- explicit `reference_path(s)` are authoritative before fallback action inputs/artifact graph candidates
- a candidate reference field must have structural value-set overlap with existing reconcile group keys before it can drive deterministic projection repair
- if contributions were computed at an aggregate grain that cannot support the requested reference projection, projection repair must force typed regrouping/recompute rather than invent per-reference values

Do not fix with: 给模型更多解释 `complete_reference=true` 的 prompt。日志显示模型已经声称要这样做，缺的是完成门的结构校验。

### G3. Eval harness shared snapshot binary can disappear under parallel workers

Priority: P2

Current behavior: convergence audit 创建相对路径 `./.codrax-convergence-<ts>` 作为 shared binary；workers 继承 `CODRAX_BIN`。某些 worker 进入 `eval/run.sh` 时该路径已不存在，脚本虽然打印 building codrax，但没有清掉 missing env override，最终 `run_read_step` 执行 missing binary，导致 `read_exit:127`。

General gap: harness 的二进制快照 lifecycle 不应依赖当前目录相对路径，也不应在 env binary missing 时半恢复。

### G4. Data-route eval log matching only picks latest log

Priority: P2

Current behavior: data route 一次 case 会产生多个 `codrax-*.log`。runner 的 log regex/metrics 使用 latest log，可能丢掉前置 route/result 行，产生 `no_log_regex:route=data` 等误报。

General gap: eval telemetry 应按 run 聚合全部 control logs，至少让 EXPECT_LOG_MATCHES 在所有本次 run logs 上匹配，并记录最终使用了哪些 log。

### G5. PASS summaries hide latent repair/contract risk

Priority: P2/P3

Current behavior: PASS case 中出现 `answer_contract_violations=1`、mermaid repair 等信号，但 summary flags 不突出。

General gap: representative sweep 不应只看 pass/fail。auto-repair、contract warning、semantic concern、context prune 等都是 drift indicators。

### G6. Multi-repo exact-scope search still over-expands

Priority: P3

Current behavior: exact multi-repo fixture 能答对，但 search cost 偏高。

General gap: 当 multi-repo focus selector 已经给出 active scopes，repo map/source inventory 应形成可复用 scoped manifest，避免重复 broad navigation。

## Commercial Delivery Design

本节把上面的 gap 转成商用级交付方案。核心原则是复用已有 typed IR，把完成条件补到系统闭环里；不把单个 eval 的文字、用户关键字、模型散文或 case 名称放进硬逻辑。

### Shared invariants

1. Hard gates only consume precise typed signals:
   - `RequestModel.CurrentSourceLaneDecision()`
   - `CurrentSourceExplanationProfile.Active()`
   - `AnswerIntentContract.Origins`
   - `ExternalObservationSufficiency.Status`
   - `RuntimeGroundingDisposition` plus typed source-lane posture
   - `OutputProjectionGraph.Status`
   - `CompletionGateGuardResult`
2. Prompt text is soft guidance only. It may explain typed state to the model, but must not become the only enforcement mechanism.
3. Runtime artifact provenance and current-source provenance remain separate:
   - external trace/log rows can support artifact observations;
   - current repo citations can support current implementation claims;
   - neither lane can masquerade as the other.
4. Data workflow terminal completion must validate the final user-facing output, not only upstream ledgers.
5. Eval harness fixes must improve reproducibility and telemetry without changing product runtime behavior.
6. Existing write-mode/read-mode red lines remain untouched: no worktree write-mode bypass, no read-mode source mutations, no broad behavior change to ordinary code questions.

### Existing components to reuse

| Need | Existing component | Use |
| --- | --- | --- |
| Current-source lane posture | `CurrentSourceLaneDecision`, `CurrentSourceExplanationProfile`, `RequiresCurrentSourceForExternalObservation` | Decide whether source is excluded, optional, or required. |
| Mixed-origin answer contract | `CompileAnswerIntentContract`, `AnswerEvidenceOrigin` | Determine whether current-source and external origins are both required before close. |
| External observation close signal | `AssessExternalObservationSufficiency` | Allow cheap close only when source is truly optional. |
| Parallel sibling waiting | `parallelExploreMustWaitForSiblingHandoffs`, `parallelExploreMixedOriginNeedsSiblingHandoffs` | Disable early convergence when required typed lanes are unfinished. |
| Runtime artifact floor waiver | `EvidenceFloorWaiver`, `RuntimeGroundingDisposition` | Waive artifact citation floors without excluding source lanes. |
| Data output readiness | `OutputProjectionGraph`, `ReferenceProjectionGap`, `CompletionGateGuardResult` | Block terminal complete on invalid final projection shape. |
| Eval metrics | `runner_lib.sh`, `eval/run.sh`, `convergence_audit.sh` | Aggregate control logs and expose advisory flags. |

### Forbidden implementation paths

- Do not inspect raw user request text for “结合当前源码”, “当前代码”, group names, case names, or other keywords in hard logic.
- Do not parse model-authored prose to decide whether a lane is complete.
- Do not create a second current-source obligation system parallel to `CurrentSourceExplanationProfile` / `AnswerIntentContract`.
- Do not make `external_only_trace` or `external_only_log` mean source exclusion unless `ExternalObservationPolicy.ExcludesCurrentSource()` or `CurrentSourceLaneDecision()==excluded`.
- Do not hard-code data field names like `GroupA`, `GroupX`, `targets.csv`, or `canonical_label`.
- Do not make eval pass/fail decisions depend on answer content outside declared case expectations and control logs.

### Batch design

#### Batch 1: Mixed external artifact + current-source lane closure

Objective: A mixed external/current-source request must not close from artifact observations alone when typed state requires current-source evidence.

Design:

- Make `CompileAnswerIntentContract` and `RequiresCurrentSourceForExternalObservation` agree on source-required mixed requests.
- Treat `CurrentSourceExplanationProfile.Active()` and typed current-source obligations as blockers for external sufficiency.
- Keep source optional for pure artifact-observation requests.
- Add unit tests that construct typed IR directly; no raw-request keyword matching.

Acceptance:

- Existing observation-only runtime tests still pass.
- Mixed runtime/current-source tests block external-only sufficiency.
- `parallelExploreMixedOriginNeedsSiblingHandoffs` returns true for mixed runtime/current-source mechanism/diagnostic outputs.

#### Batch 2: Parallel explorer and finalizer provenance boundary

Objective: Parallel exploration cannot let an artifact-only sibling erase or suppress current-source evidence, and finalizer cannot render observation-only text unless typed source policy excludes source.

Design:

- Keep early convergence enabled for source-optional external-only turns.
- Disable or delay early convergence when the compiled mixed-origin contract requires sibling handoffs.
- Merge completed sibling evidence before deciding final close whenever required lanes are in play.
- Refine `runtimeObservationOnlyForAnswerDoc` to check typed source exclusion / source lane decision, not just absence of current-source evidence.

Acceptance:

- The trace mixed-source regression prompt contains “Current repository citations may still be used” and not “This dispatch is observation-only”.
- Existing observation-only waiver tests still suppress repo enrichment.
- Parallel mixed-origin tests show current_source missing lane prevents auto-close.

#### Batch 3: Data final reference projection gate

Objective: Terminal complete must be impossible when strict reference projection output has the wrong number of items or wrong reference-key coverage.

Design:

- Extend output projection validation so final answer shape is checked against typed reference universe when `complete_reference` is required.
- Distinguish contribution/reconcile group correctness from final output projection correctness.
- Produce typed `GuardResult` with repair hint `assemble_answer` when final answer cardinality/coverage does not match reference rows.
- Keep the validation domain-neutral: reference row count, key coverage, output item count, projection kind, and typed defaults.

Acceptance:

- A result with three reference keys and eight output values is blocked.
- Missing reference key emits incomplete-reference guard instead of terminal complete.
- Non-reference contribution groups remain in audit/reconcile artifacts but do not become output members unless the output contract asks for present groups only.

#### Batch 4: Eval harness reliability and advisory telemetry

Objective: Representative eval results should be reproducible under parallel=2 and should expose latent risk signals even when PASS.

Design:

- Use absolute shared snapshot binary paths and parent-owned cleanup.
- If `CODRAX_BIN` from env is missing/non-executable, either reset to rebuilt `./codrax` or fail loud before case execution.
- Aggregate all per-run `codrax-*.log` files for `EXPECT_LOG_MATCHES_REGEX` and metrics.
- Add advisory flags for answer-contract violations, mermaid source repairs, context prunes, semantic concerns, and finalizer rewrites without turning them into product failures.

Acceptance:

- No `read_exit:127` caused by missing snapshot binary in parallel sweeps.
- Data-route cases can match `route=data` across multi-log runs.
- PASS summaries show advisory flags when hidden repairs or contract warnings occurred.

#### Batch 5: Scoped multi-repo/source-inventory efficiency

Objective: Preserve correctness while lowering broad search cost when exact active repos are already known.

Design:

- Reuse existing focus selector / source inventory typed routes as the scoped navigation manifest.
- When `source_inventory_profile` is active, make `source_inventory` the first soft repo-map route and suppress relation/call-flow first-hop primer text that comes only from broad enumeration compatibility hints.
- Avoid repeated `repo_map` / `list_files` expansion after active subrepos are fixed by steering the explorer toward one partitioned source-inventory pass per active subrepo.
- Keep this as P3 after correctness batches; do not risk broad multi-repo behavior during P1 delivery.

Acceptance:

- `mr_cross_repo_compare` remains PASS.
- Prompt/tool advisory order favors `source_inventory` for typed inventory shapes.
- Tool counts decrease or advisory `wide_search`/`contract_warning` is visible without hiding inactive-scope disclosure.

### Delivery and push protocol

1. Design batch:
   - land this document;
   - commit and push on `codex/representative-eval-gap-delivery-20260608`.
2. Code batches:
   - implement one batch at a time;
   - run focused unit tests for touched packages;
   - commit and push each batch separately;
   - update this document with status and validation evidence.
3. Eval validation:
   - after P1 code batches, run focused evals for trace mixed source and data reference projection;
   - after harness batch, rerun representative parallel-2 sweep;
   - record result paths and any residual advisory-only risks.

## Executable Task List

| ID | Priority | Task | Target areas | Validation |
| --- | --- | --- | --- | --- |
| T1 | P1 | Add a regression test for runtime trace + current-source explanation where analyzer emits `external_observation_policy.current_source_mode=allow` and a typed current-source bridge; external sufficiency must not mark the turn sufficient before current-source evidence exists. | `internal/types/external_observation_sufficiency.go`, `internal/types/request_traits.go`, analyzer IR normalization tests | `go test ./internal/types` plus focused trace eval |
| T2 | P1 | Ensure analyzer normalization creates or preserves a precise current-source lane obligation for explicit mixed artifact/source requests. If the analyzer emits only `source_scope_profile` plus `external_observation_policy.allow`, derive a typed warning/drop-safe bridge only from structured fields, not raw prose. | `internal/types/current_source_explanation_profile.go`, `internal/agent/analyzer.go`, analyzer parser tests | unit tests for log/trace/command/git mixed requests |
| T3 | P1 | Change external observation sufficiency and accepted closure so artifact-only completion cannot close a mixed turn with a required current-source lane. | `internal/types/external_observation_sufficiency.go`, `internal/orchestrator/contract_check.go`, `internal/orchestrator/explore_parallel_dispatch.go` | tests around `shouldAutoCompleteExploreWindowFromAcceptedClosure` and `AssessExternalObservationSufficiency` |
| T4 | P1 | Update parallel explorer merge/cancel behavior: when any required lane is unfinished, do not let a sibling with `external_only_trace/log` waiver cancel or overwrite sibling current-source evidence. Preserve accepted read/evidence rows from siblings before close. | `internal/orchestrator/explore_parallel_dispatch.go`, explorer handoff merge tests | regression based on `read_combo_trace_current_source_explanation` |
| T5 | P1 | Narrow Runtime Grounding Disposition: `external_only_trace/log` may waive repo citation floors for artifact-origin claims, but must not render observation-only exclusion unless `CurrentSourceLaneDecision()==excluded`. | `internal/agent/answer_document_evaluator.go`, `internal/types/evidence_floor_waiver.go`, `internal/types/answer_surface_plan.go` | finalizer prompt unit tests plus trace eval |
| T6 | P1 | Add data final projection validator: when reference projection is required, final answer cardinality and reference-key coverage must match the reference set before terminal complete. | `internal/dataworkflow/output_projection.go`, `internal/dataworkflow/completion_gate.go`, `internal/repl` data terminal path | unit test with `targets=[GroupA,GroupX,GroupC]`, answer `17,4,5,0...` must block |
| T7 | P1 | Separate contribution ledger reconciliation from target-reference projection reconciliation. Non-reference contribution groups are audit-only; missing target keys get typed default values in output order. | `internal/dataworkflow`, `internal/dataquery` assemble/reconcile actions | data workflow fixture expecting `17,0,5` |
| T8 | P2 | Fix convergence audit snapshot lifecycle: use an absolute shared binary path, clean it only in parent, and make worker scripts fail loud or reset `CODRAX_BIN` when env binary is missing. | `eval/convergence_audit.sh`, `eval/run.sh`, `eval/runner_lib.sh` | shell tests plus parallel-2 smoke run |
| T9 | P2 | Aggregate all per-run logs for EXPECT_LOG_MATCHES and telemetry, especially data route multi-log runs. | `eval/run.sh`, `eval/runner_lib.sh`, `eval/telemetry` | data route case should not falsely report missing `route=data` |
| T10 | P2/P3 | Add advisory flags for hidden repairs and contract warnings in representative summaries, without converting them to hard failures. | `eval/convergence_audit.sh`, telemetry collectors | PASS rows expose `contract_warning`, `auto_repair`, `context_prune` where applicable |
| T11 | P3 | Add scoped multi-repo source-inventory first-hop reuse after exact active repos are known. Keep it as typed soft guidance, not a hard gate. | repo map navigation policy, explorer repo-map prompt | `mr_cross_repo_compare` remains PASS; typed inventory prompt starts with `source_inventory` and no relation first-hop primer |
| T12 | P1 | Harden `assemble_answer complete_reference` so explicit reference paths are priority tier 1 and fallback inputs/artifacts cannot override them by having more rows. | `internal/dataquery/action_runner.go` | generated artifact alias test where wider input artifact shares the key field; output must use explicit reference path order/count |
| T13 | P1 | Harden data completion repair candidate selection so reference fields are chosen by typed value-set overlap with reconcile group keys; a zero-overlap field with matching cardinality must not terminate completion. | `internal/repl/data_task_workflow.go` | completion gate test with `target_id` and `canonical_label` fields; repair must pick the overlapping field without keyword matching |
| T14 | P1 | Make data eval verdicts read the latest terminal audit status, not arbitrary stdout snippets. A blocked/repair/continue terminal must fail even if logs contain the expected answer string. | `eval/run.sh`, `eval/runner_lib_test.sh` | fake blocked terminal with expected text in stdout must fail as `data_terminal_status:blocked` |
| T15 | P1 | Let `compute_contributions` accept typed grouping aliases such as `group_by_fields` without changing its single-record-set contract. | `internal/dataquery/action_runner.go` | contribution test with alias params groups by the intended structural field |
| T16 | P1 | Add relation-field inference for `enrich_records` and `join_records` based on value overlap, one-to-one/cardinality quality, and generated-index safeguards. | `internal/dataquery/action_runner.go` | zero-match enrich recovers; duplicate-label join chooses row-aligned index only when structurally derived index fields justify it |
| T17 | P1 | Prevent repeated final projection from overwriting a structurally complete `assemble_answer` result. Completion reducers must recognize typed projection artifacts and let deterministic completion override noisy evaluator repair. | `internal/dataworkflow/state_builder.go`, `internal/dataworkflow/evaluation.go`, `internal/repl/data_task_workflow.go` | post-result/evaluation tests keep completed projection from dispatching a second assemble |
| T18 | P1 | Detect reference projection gaps when the reference key set is a subset of reconcile groups but contains reference-only keys; rollups and non-reference groups must not satisfy strict target output. | `internal/repl/data_task_workflow.go` | completion gate test with `GroupA,GroupB,GroupC,all` reconcile and `targets.csv=[GroupA,GroupX,GroupC]` repairs to `targets.csv.canonical_label` |

## Suggested Rerun Matrix

After T1-T7:

```bash
CODRAX_BIN=/Users/han/opt/codrax/codrax \
  CASES='eval/cases/read_combo_trace_current_source_explanation.case eval/cases/data_multifile_reference_projection.case' \
  PARALLEL=2 RUNS=1 TIMEOUT=1200 \
  SUMMARY=eval/results/representative_eval_20260608_fixcheck_summary.md \
  bash eval/convergence_audit.sh
```

After T8-T10:

```bash
PARALLEL=2 RUNS=1 TIMEOUT=1200 \
  SUMMARY=eval/results/representative_eval_20260608_harness_fixcheck_summary.md \
  bash eval/convergence_audit.sh
```

Expected outcomes:

- `read_combo_trace_current_source_explanation`: PASS with current-source citation from `internal/tracequery/parse.go`.
- `data_multifile_reference_projection`: PASS with `17,0,5`.
- no `read_exit:127` from missing shared binary.
- data route log regex should match across run logs.
- PASS rows should show advisory repair/contract telemetry when present.

## Delivery Status

### Batch 1 - Mixed typed source-scope lane

Status: done and pushed in code batch.

Changes:

- Added `RequestModel.HasTypedCurrentSourceScopeRequest()` as a precise typed signal for external-observation turns that also carry analyzer-emitted current-source path scope.
- Wired that signal into `CurrentSourceLaneDecision`, `RequiresCurrentSourceForExternalObservation`, and `AssessExternalObservationSufficiency`.
- Added regression tests for source-scoped external trace requests so they compile `current_source + runtime_artifact` origins and block external-only sufficiency.

Validation:

```bash
go test ./internal/types
go test ./internal/orchestrator -run 'TestAcceptedClosureAutoComplete|TestParallel|MixedOrigin|RuntimeCurrentSource'
go test ./internal/agent -run 'TestAnswerDocumentEvaluator_(MixedRuntimeCurrentSourceDoesNotRenderObservationOnly|CurrentSourceExplanationProfileRendersMixedGuidance|RuntimeObservationOnlySuppressesRepoEnrichment|RuntimeObservationOnly)'
```

Result: all passed.

### Batch 4 - Scoped source-inventory navigation efficiency

Status: done and pushed in code batch.

Changes:

- Changed typed repo-map navigation policy so active `source_inventory_profile` makes `view="source_inventory"` the first soft route instead of leading with generic `task_map`/`file_map` orientation.
- Suppressed the relation/call-flow “Typed Repo Map First Hop” primer when the first typed route is a principal source-inventory route. This avoids a broad `task_map` prompt nudge caused by generic enumeration-to-implements compatibility hints.
- Kept all behavior advisory-only: no evidence gate, answer gate, active-set gate, or user-intent keyword matcher was added.

Validation:

```bash
go test ./internal/types -run TestCompileRepoMapNavigationPolicy
go test ./internal/agent -run 'TestBuildInitialInstruction_(CallChainTypedRepoMapOutranksGenericGrep|SourceInventorySuppressesRelationFirstHop)'
```

Result: all passed.

### Batch 2 - Reference projection output gate

Status: done and pushed in code batch.

Changes:

- Made `assemble_answer complete_reference` project exactly the declared reference key universe in reference order, filling missing keys with zero and excluding non-reference contribution groups from the user-facing output.
- Preserved original reconcile groups as audit ledger and added an answer-scoped final projection group so reconcile validation does not confuse reference projection output with the contribution group universe.
- Added `ActionRunner.ReferenceKeyCandidateForPath()` so completion gates can validate explicit reference contracts without inferring business semantics.
- Extended data terminal completion logic to reject reference item-count mismatches in both directions, even when a final assemble artifact is present.

Validation:

```bash
go test ./internal/dataquery
go test ./internal/dataworkflow
go test ./internal/repl
```

Result: all passed.

### Batch 3 - Eval harness reliability and advisory telemetry

Status: done and pushed in code batch.

Changes:

- Switched convergence-audit snapshot binary to an absolute path and cleared inherited EXIT traps in worker subshells so parallel workers cannot delete the parent sweep binary.
- Made `eval/run.sh` recover from a missing/non-executable env `CODRAX_BIN` by rebuilding and resetting to `./codrax` before execution.
- Aggregated every per-run `codrax-*.log` into `run-N.logs.all.log`; metrics and log regex assertions now read the aggregate log, covering data-route multi-log runs.
- Added advisory convergence flags for answer-contract warnings, renderer auto-repairs, and context pruning.
- Added a runner contract test that verifies regex assertions and metrics can span multiple logs from the same run.

Validation:

```bash
bash -n eval/run.sh eval/convergence_audit.sh eval/runner_lib.sh eval/runner_lib_test.sh
bash eval/runner_lib_test.sh
```

Result: all passed.

### Batch 6 - Reference path priority and field-overlap projection hardening

Status: done in current code batch; superseded by Batch 7 final validation.

Follow-up audit:

- A focused rerun after Batch 2 showed deterministic repair correctly detected a reference projection gap, but `assemble_answer` still selected a wider input artifact because fallback paths were pooled with explicit `reference_path`.
- Another rerun showed a stricter completion gap: a reference table contained multiple fields with the same row count, and the contract's current field had zero value-set overlap with reconcile group keys. Matching cardinality alone allowed a wrong `0,0,0` projection.
- Both issues are system-level: reference path priority and reference field selection must be typed structural rules, not prompt expectations or field-name keyword matching.

Changes:

- Split assemble reference lookup into explicit and fallback tiers. `reference_path(s)` from action params/output contract are tried first; action inputs and artifact graph aliases are used only if no explicit reference keys can be read.
- Added `ActionRunner.ReferenceKeyCandidatesForPath()` to enumerate structural key-universe candidates for all readable fields in a reference material.
- Changed data completion repair selection to annotate candidates with value-set overlap against reconcile group keys, skip zero-overlap candidates, and choose the best overlapping field by deterministic score.
- Preserved domain neutrality: the code does not look for names like `target_id`, `canonical_label`, `GroupA`, `GroupX`, or case text.

Validation:

```bash
go test ./internal/dataquery -run 'TestActionRunnerAssembleAnswer(CompletesReferenceKeys|ReferenceProjectionDrops|HonorsExplicitReferencePath|DoesNotCompleteReference)|TestActionRunnerInfersReferenceKeyCandidate'
go test ./internal/repl -run 'TestDataTaskWorkflowCompletionGate(RequiresReferenceCompleteProjection|RejectsReferenceCardinalityMismatch|InfersReferenceFromAssembleActionInput|ChoosesReferenceFieldByGroupOverlap)|TestDataTaskWorkflowCompletionGateRequiresOutputProjection'
go test ./internal/dataquery
go test ./internal/dataworkflow
go test ./internal/repl
make

CODRAX_BIN=/Users/han/opt/codrax/codrax \
  CASES='eval/cases/data_multifile_reference_projection.case' \
  PARALLEL=1 RUNS=1 TIMEOUT=1200 \
  SUMMARY=eval/results/data_reference_projection_20260608_fix4_summary.md \
  bash eval/convergence_audit.sh
```

Result: unit/package tests passed; early focused data eval passed, but later reruns exposed additional relation/final-projection reducer gaps captured in Batch 7.

### Batch 7 - Data relation quality and final projection completion hardening

Status: done in current code batch; pending commit/push after final validation.

Follow-up audit:

- A stricter focused rerun showed `enrich_records` could keep a declared relation field pair that produced zero matches even though another structural field pair had clear value overlap.
- Another rerun showed `join_records` could repair a zero-match pair into a high-fanout duplicate-label join. The root cause was wrong join grain, not missing dedupe.
- A later rerun produced the correct `17,0,5`, then a required-output fallback re-assembled over the wrong reference (`target_contributions.group_key`) and overwrote the answer with zeros.
- The final failing rerun assembled `11,17,4,5`: the reducer saw an `assemble_answer` artifact but output graph did not mark `projection_artifact_present`, and reference-gap detection ignored reference subsets when reconcile groups also contained rollups/non-reference groups.

Changes:

- Added value-overlap relation inference for `enrich_records` when declared fields produce zero matches.
- Added `join_records` relation-quality inference that can replace zero-match or row-aligned fanout joins with better structural field pairs, while guarding generated `_source_index` use behind derived-index evidence.
- Added grouping-param aliases for `compute_contributions` so model plans using `group_by_fields` still execute through the typed contribution path.
- Made `assemble_answer` replace stale final projection reconcile groups instead of preserving older answer-scoped projection rows.
- Added deterministic completion recognition for typed `assemble_answer` artifacts so post-result fallback and evaluator repair cannot overwrite a structurally complete projection.
- Extended reference-gap detection to handle reference subsets with reference-only keys, so rollups and non-reference groups do not satisfy strict reference output.
- Hardened data eval verdicts to read latest terminal audit status and strict final stdout, preventing false PASS from diagnostic text.

Validation:

```bash
go test ./internal/dataquery ./internal/dataworkflow ./internal/repl
bash eval/runner_lib_test.sh
make

CODRAX_BIN=/Users/han/opt/codrax/codrax \
  CASES='eval/cases/data_multifile_reference_projection.case' \
  PARALLEL=1 RUNS=1 TIMEOUT=1200 \
  SUMMARY=eval/results/data_reference_projection_20260608_fix12_summary.md \
  bash eval/convergence_audit.sh
```

Result: all package tests and harness tests passed. Focused data eval passed with terminal status `complete`, final stdout `17,0,5`, `reconcile=pass`, `projection_artifact_present=true`, and zero advisory flags.
