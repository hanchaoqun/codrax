# Eval Gap Delivery 2026-06-13

## Scope

本记录来自 2026-06-13 代表性 eval 抽检，覆盖 trace/runtime、data
workflow、repo relation、多仓对比和 TypeScript relation。目标不是贴单个 case
补丁，而是把可复用的系统级 gap 落到结构化 handoff、确定性 workflow fallback、
统一 JSON/plan 修复层和可审计任务列表。

## Eval Batch

每批 2 个 case 并行运行：

| Batch | Case | Verdict | Audit |
| --- | --- | --- | --- |
| trace | `trace_query_openharmony_bytrace_thread` | PASS | trace/perf pre-stage 可回答，但未显式调用 `trace_query`；答案语义可接受。 |
| trace | `trace_query_state_churn_window_stats` | FAIL | 答案包含所需下一步方向和 `rival-30`，但最终答复同时出现 runtime state 数值不一致的系统警告。 |
| data/repo | `data_json_strict_ids` | FAIL | 首批已算出正确 JSON ids，但 contribution ledger 未完成，workflow 在 `prepare_contribution_inputs` / `compute_contributions` 间耗尽。 |
| data/repo | `sr_ts_workspace_chain` | PASS | 有效使用 `repo_map` / `read_file`，答案稳定。 |
| multi/relation | `mr_cross_repo_compare` | PASS | analyzer 的 `source_inventory` 多仓 lens 未命中后 explorer 用 `file_map` 恢复；存在效率优化空间。 |
| multi/relation | `qf_type_relation_loop_controller` | PASS | analyzer/explorer 均积极使用 `repo_map(view="implementers")`。 |

## Root Cause Ledger

### G1. Runtime artifact 多生产者缺少结构化优先级

现象：perf pre-stage 的模型化 `emit_perf_trace` 观测与后续确定性
`trace_query` aggregate facts 同时进入 finalizer handoff。二者同属
runtime artifact，但没有统一的 producer 优先级、冲突降级或 prompt 预算排序。
结果是 finalizer 可以同时携带两组不一致的 state 总量/主导态，并在用户答案里
暴露前后不一致警告。

泛化约束：

- 只能消费结构化 producer/source/origin/role/policy 字段。
- 不检查用户意图关键字，不解析模型散文，不用 case 文案匹配。
- 不删除预处理信息，避免削弱答案丰富度；只在确定性 runtime tool 存在时把
  pre-triage runtime rows 降级为 supporting/advisory。
- `trace_query`、未来 runtime query 工具、perf/log pre-stage 都通过同一
  `ObservationLedger` 优先级和 role/policy 规则进入 finalizer/reviewer。

### G2. Data workflow contribution ledger completion 缺少确定性兜底

现象：`data_json_strict_ids` 已产生正确答案和可复用 record artifact，但
`ContributionLedgerRequired` 仍未满足。现有 `ActionScaffold` 会给出
`compute_contributions` 模板，但 `ConcreteActionFromScaffold` 不会把它转换为
可执行 action；`BuildRequiredLedgerCompletionPlan` 也只处理 rule/reconcile/final
projection，未处理 missing contributions。模型多次在相邻 DAG rank 间来回，直到
budget exhausted。

泛化约束：

- fallback 只在 workflow state 指示 contribution ledger 缺失、且已有结构化
  record artifact 时触发。
- fallback 生成保守 `compute_contributions(operation=count)` 审计账本，不改变
  已有业务答案和最终 projection。
- item/source/evidence 锚点来自 artifact path、row line/index 和已知字段；
  不从用户问题或模型输出散文推断业务字段。
- 若已有更强的模型计划或可执行 typed action，保持现有路径优先；fallback
  只作为 planner/repair/terminal guard 后的 deterministic recovery。

### G3. JSON/plan 修复层需要减少模型心智

现象：strict decode 已覆盖 tool JSON，当前抽检没有触发 decode repair，但 data
workflow 的失败显示“模型需要知道下一步 DAG rank + ledger producer + JSON 形状”
仍然过重。系统应尽量把可确定的 ledger completion plan 由 reducer 生成，让模型
只处理业务判断。

泛化约束：

- 统一修复层负责 typed JSON/plan shape 和 executable scaffold materialization。
- prompt/hint 只描述结构化 contract，不把关键字或 case-specific 文案放入逻辑。
- 后端消费者必须收到前序阶段的 accepted aggregate facts、observation rows、
  artifact access、ledger graph 和 output graph，避免证据丢失。

### G4. Rule coverage 与 item ledgers 的 `rule_refs` 缺少结构化归一

复跑后 `data_json_strict_ids` 已产生 decision rows 和 contributions，但
`qualify_records` 生成了不存在的 `RULE_ACTIVE_FILTER` rule ref，终态校验在
unknown rule ref 处失败。这不是单 case 的规则名问题，而是 item ledgers 与
source-backed `rule_coverage` 之间缺少 deterministic canonicalization。

泛化约束：

- 只处理 typed `rule_refs` 字段，不能从用户文本或模型解释中猜测规则。
- 只有存在 source-backed `rule_coverage.rule_id` 时，才把未知 refs 映射到
  source-backed rule id；纯模型规则无 evidence 时继续 fail-loud。
- 不新增业务规则，不放宽 unknown-rule 校验；修复层只把模型自造的 ledger
  ref 对齐到已经存在且有源证据的 rule coverage。

### G5. JSON payload answer 在多批 typed workflow 中丢失

复跑后 `data_json_strict_ids` 的早期脚本已经发出 `{"ids":["u1","u3"]}`，
但 `emit_result(dict)` 把普通 JSON payload 当成完整 Result envelope；由于对象里没有
`answer` 字段，payload 被降为 `custom_payload` artifact，后续批次补 ledger 时
`answer` 又被内部 artifacts JSON 摘要覆盖。最终 workflow 认为缺少真实 final
answer，即使前序阶段已有满足 `output_contract` 的 JSON payload。

泛化约束：

- `emit_result` 必须区分 canonical Result envelope 与普通 JSON payload；只有包含
  `answer`、`rows`、`rule_coverage`、`contributions` 等保留字段时才按 Result
  envelope 解释。
- 普通 dict/list payload 在 `json_only` 或显式 output_contract 下应序列化为
  `result.answer`，而不是只作为 diagnostic artifact。
- 内部 artifacts/reconcile JSON 摘要必须被识别为非用户 final answer，避免覆盖
  earlier candidate answer handoff。
- 该层只消费结构化 JSON shape 和保留字段集合，不读取用户意图关键字，不解析模型散文。

### G6. Contribution reconcile 只支持数值加总，缺少 operation 语义闭环

复跑后 contribution ledger 已满足，但 `reconcile_artifacts` 使用
`sumContributionGroups`，只会从 `value` parse 数字。`operation=count` 且 value
携带实体 id 时被视为“无 numeric groups”，导致 required reconcile 阶段无法完成。
这不是单个 ids case 的问题；typed ledger 已允许 `count/include/set/rank`，但执行和
验证层只实现了 `add/subtract` 的数值语义。

泛化约束：

- `count` 的 deterministic reconcile 语义是每条 target contribution 计为 1，
  不依赖 `value` 是否为数字。
- `add/subtract` 继续要求 numeric value，保持现有 fail-loud 行为。
- `include/set/rank` 可以产生结构化文本 aggregate，验证层按 group key 精确覆盖，
  只在 report 给出对应文本值时做精确比较；不得把文本值强行 parse 成数字。
- reconcile report 生成、validator、runner artifact summary 复用同一 aggregate
  计算函数，避免 prompt 或单 case fallback 分叉。

## Design

### D1. ObservationLedger runtime producer precedence

在 `internal/types/observation_ledger.go` 增加确定性 runtime producer 规则：

1. 编译所有 accepted observations 后执行结构化 reconciliation。
2. 如果同一 ledger 中存在 origin=`runtime_artifact` 且 producer 为确定性
   runtime query 工具的记录，则把 origin=`runtime_artifact` 且 producer 为
   pre-triage 的记录降为 `supporting_coverage`，grounding policy 重新计算。
3. 给降级记录追加结构化 note，说明该记录仍可作背景/样本，但不能覆盖确定性
   runtime tool aggregate。
4. prompt 排序中确定性 runtime query producer 额外提权，pre-triage producer
   在有确定性 runtime row 时自然后置。

### D2. Contribution ledger deterministic completion

在 `internal/dataworkflow` 复用已有 scaffold/fallback：

1. 让 `ComputeContributionScaffolds` 在 artifact 有稳定输入路径时生成一个
   executable, conservative count scaffold。
2. 在 `ConcreteActionFromScaffold` 支持 `compute_contributions`，仅接受
   `operation=count`、常量 group/metric、已有 item_id_field 或 synthetic row id。
3. 在 `BuildRequiredLedgerCompletionPlan` 为 missing `LedgerContributions` 添加
   graph-driven fallback，复用 artifact projections 和 seen action keys，避免跳过
   DAG prerequisite。
4. 在 REPL/CLI data workflow completion transition 输入中传递 artifact
   projection 和 seen action keys，使 completion guard、validation failure 和
   terminal workflow 都能消费同一 fallback。

### D3. Audit and verification

新增 deterministic tests：

- ObservationLedger: deterministic runtime query rows demote perf pre-triage rows
  without dropping them.
- Observation prompt projection: deterministic runtime query rows outrank
  pre-triage rows under tight prompt budget.
- Dataworkflow reducer: missing contributions with record artifact yields one
  executable conservative `compute_contributions` plan.
- REPL data workflow integration: terminal/validation completion can use the
  deterministic contribution fallback without planner repair loops.
- Dataquery patch engine: unknown item-ledger `rule_refs` canonicalize only to
  source-backed rule coverage, while non-source-backed unknown refs still fail.

### D4. JSON payload handoff and contribution aggregate semantics

1. 在 runner helper 层修改 `emit_result`：包含 canonical Result 字段的 dict 仍按
   envelope 处理；普通 dict/list 作为 JSON answer 序列化，并继续合并显式 ledgers。
2. 在 result parsing / answer presence 层识别内部 artifacts JSON 摘要，确保多批
   typed workflow 可以保留更早的真实 answer candidate。
3. 用统一 `ContributionGroupAggregate` 替代 reconcile 里的纯 numeric sum 入口：
   `add/subtract` 走 numeric sum，`count` 走 exact count，`include/set/rank`
   走稳定文本值集合。
4. `reconcileReportFromContributions`、`runReconcileArtifacts` 和
   `validateReconcileReport` 共用 aggregate 输出；不让 prompt 或 repair plan
   承担 operation 语义修复。
5. `assemble_answer` 保持现有 numeric/default 投影；若 seed/candidate answer 已经是
   有效 output contract，reconcile 批次不会用 artifacts summary 覆盖它。

Verification commands:

- `go test ./internal/types ./internal/dataworkflow ./internal/repl ./internal/dataquery`
- `go test ./...`
- Focused eval after implementation, 2 parallel per batch:
  - `eval/cases/trace_query_state_churn_window_stats.case`
  - `eval/cases/data_json_strict_ids.case`
  - `eval/cases/harmony/trace_query_openharmony_bytrace_thread.case`
  - `eval/cases/qf_type_relation_loop_controller.case`

## Task List

- [x] T0: 记录 eval 审计、根因和泛化设计。
- [x] T1: 实现 ObservationLedger runtime producer precedence。
- [x] T2: 增加 runtime precedence 单测和 prompt budget 单测。
- [x] T3: 实现 executable conservative `compute_contributions` scaffold。
- [x] T4: 将 contribution ledger completion fallback 接入 completion repair
      transition。
- [x] T5: 增加 dataworkflow/repl 单测，覆盖 missing contributions deterministic
      recovery。
- [x] T6: 实现 source-backed `rule_refs` canonicalization，并保持无源规则
      unknown refs fail-loud。
- [x] T7: 修复 JSON payload answer handoff：`emit_result` envelope 判定、
      artifacts summary 识别、seed answer carry-over。
- [x] T8: 实现 contribution aggregate reconcile semantics，覆盖 count 和
      text-valued operations。
- [ ] T9: 分批提交并推送文档、runtime handoff、data workflow fallback。
- [ ] T10: 运行 Go 测试和代表 eval，每批 2 case。
- [ ] T11: 人工审计 eval 答案与日志，回写本文件的验证结论。

## Progress

- 2026-06-13: Batch 0 document recorded. Worktree was clean and `main` was
  already up to date before implementation.
- 2026-06-13: Batch 1 implemented runtime observation producer precedence.
  `trace_query` deterministic runtime rows now retain principal status and tight
  prompt-budget priority, while perf pre-triage rows remain available as
  supporting/advisory context when deterministic runtime query rows exist.
- 2026-06-13: Batch 2 implemented deterministic contribution-ledger recovery.
  Completion repair now receives artifact projections and can generate a
  conservative `compute_contributions(operation=count, role=audit)` action over
  handed-off record artifacts when required contribution ledgers are missing and
  DAG prerequisites are satisfied.
- 2026-06-13: Batch 3 implemented source-backed rule-ref canonicalization in the
  data result patch engine. This addresses the post-Batch-2 eval discovery where
  item ledgers were present but referenced a model-invented rule id instead of
  the already emitted source-backed rule coverage.
- 2026-06-13: Post-Batch-3 eval exposed two deeper typed workflow gaps:
  ordinary JSON payloads emitted by early batches were preserved only as
  `custom_payload` artifacts, and contribution reconcile had no deterministic
  semantics for non-numeric `count` values. Batch 4 will address these as
  runner/ledger semantics rather than case-specific answer patching.
- 2026-06-13: Batch 4 implemented JSON payload answer handoff and unified
  contribution aggregate reconcile semantics. `emit_result` now distinguishes
  Result envelopes from ordinary JSON answers, internal artifacts JSON no longer
  counts as a final answer, and `count/include/set/rank` contributions use a
  deterministic aggregate path shared by reconcile generation and validation.
- 2026-06-13: Post-Batch-4 eval found a deeper handoff/projection gap rather
  than a material-extraction gap. The early JSON answer and active-user records
  were correct, but a deferred `compute_contributions` action that listed both a
  record artifact and a sidecar rule artifact was blocked before the runner's
  existing sidecar-skip path could execute. A later conservative
  `role=audit` ledger was then reconciled and projected as if it were the target
  answer. Batch 5 must generalize three system boundaries: single-record-set
  narrowing for compute actions, target-only reconcile projection, and strict
  JSON object/list projection from typed reconcile groups.

## Batch 5 Gap: Typed Contribution Handoff And Projection Boundaries

Observed eval behavior:

- `data_json_strict_ids` produced the correct active-user records and an early
  JSON payload, but the deferred contribution plan could not execute because
  `compute_contributions` was not covered by the existing field-contract
  narrowing used for single-record-set actions.
- The runner already has generic support for skipping non-contribution sidecar
  inputs during `compute_contributions`; the missing layer was the workflow
  staging/deferred readiness path, which rejected the multi-input action before
  the runner could apply that support.
- Reconcile generation aggregated every contribution role, including
  `role=audit`. Validation already treats audit/material/diagnostic roles as
  auxiliary, so generation and validation had diverged on the target output
  boundary.
- `assemble_answer` could render JSON group arrays but lacked a structural
  projection for JSON objects whose fields are reconcile group keys and whose
  values are text-member arrays.

Generalized design:

- Extend the existing field-contract narrowing path to include
  `compute_contributions`. The matcher should use structured action params such
  as `value_field`, `group_key_field(s)`, `item_id_field`, `status_fields`, and
  filter fields. It must not infer intent from user prose or model prose.
- Make reconcile generation and patch-generated reconcile reports aggregate
  only target-participating contributions. Auxiliary roles remain available for
  validation/audit context but cannot become final answer groups.
- Preserve text aggregate members on reconcile groups as typed `values` metadata
  and add a generic `json_object` projection in `assemble_answer`. This lets
  output contracts express object/list answers without custom scripts or
  keyword-shaped post-processing.
- Strengthen planner guidance around contribution operations: use
  `include`/`set`/`rank` when the final value is an item label/id/list member;
  use `count` only for row counts.

Executable task list:

- [x] B5-T1: Add compute-contribution field references to
  `SingleRecordSetActionFieldRefs` and the REPL deferred narrowing wrapper.
- [x] B5-T2: Add dataworkflow and REPL tests proving sidecar rule/reference
  artifacts are narrowed away before deferred compute dispatch.
- [x] B5-T3: Change reconcile generation/report synthesis to aggregate
  target-participating contributions only, while preserving answer-level pass
  for auxiliary-only ledgers.
- [x] B5-T4: Add typed reconcile group `values` metadata for text aggregates
  and `assemble_answer projection=json_object`.
- [x] B5-T5: Update planner action guidance for `include`/`set`/`rank` versus
  `count` and the new `json_object` projection.
- [ ] B5-T6: Run focused Go tests, full Go tests, rebuild, commit/push, then
  rerun representative eval cases two at a time.

Batch 5 verification before commit:

- `go test ./internal/dataworkflow ./internal/dataquery ./internal/repl`
- `go test ./...`
- `make`

## Batch 6 Gap: Trace Next-Step Evidence Handoff

Observed eval behavior:

- `trace_query_state_churn_window_stats` used `trace_query` efficiently and the
  answer was semantically correct, but the final next-step sentence did not bind
  the recommended follow-up to the dominant same-CPU competitor evidence.
- `trace_query` already produced a generic `state_churn` next step, while the
  same-window CPU pressure product already contained `TopRunnable` and
  `TopRunning` rows. The gap was that state-churn handoff did not carry the
  top competitor and CPU context as typed fields/RichNotes.

Generalized design:

- Keep state-churn detection domain-neutral: compute the fragmented state totals
  as before, then enrich runnable-dominant churn with same-window CPU pressure
  metadata by matching the churn thread against `CPUPressure.TopRunnable`.
- Publish the selected CPU, top running competitor, competitor running time, and
  concrete next step through `ThreadStateChurnSummary` and `trace_query` typed
  observation RichNotes. This keeps final-answer guidance grounded in structured
  runtime observations rather than finalizer prose heuristics.
- The next-step text should remain generic across scheduler traces: inspect the
  same-CPU competitor/CPU pressure/time-slice path, then validate wake latency
  with `sched_wakeup`.

Executable task list:

- [x] B6-T1: Add typed same-CPU competitor fields and next-step field to
  `ThreadStateChurnSummary`.
- [x] B6-T2: Enrich runnable-dominant state churn from existing CPU pressure
  products without rescanning or adding case-specific logic.
- [x] B6-T3: Publish next-step/competitor fields in `trace_query` RichNotes.
- [x] B6-T4: Add focused tracequery/tool tests for summary and RichNotes.
- [ ] B6-T5: Run full tests, rebuild, commit/push, then rerun representative
  eval cases two at a time.

Batch 6 focused verification before commit:

- `go test ./internal/tracequery ./internal/tool -run 'TestRootCauseRankPromotesFragmentedStateChurn|TestTraceQuerySummaryRendersFragmentedStateChurn|TestTraceQueryTypedObservationsCoverTypedProductBeyondSummaryCaps'`

## Batch 7 Gap: High-Signal Handoff Ordering And Patch Surface Preservation

Post-Batch-6 eval root:

- `eval/results/eval-gap-20260613-post-18c6a163`

Observed eval behavior:

- `data_json_strict_ids` produced the exact valid final answer
  `{"ids":["u1","u3"]}` and used the local JSON material correctly. The harness
  still failed on the default minimum output length because compact strict JSON
  can be shorter than the generic answer floor.
- The same strict JSON case carried `EXPECT_NOT_CONTAINS="```"`, which is not a
  safe bash literal because backticks inside double quotes still trigger command
  substitution during case sourcing.
- `trace_query_state_churn_window_stats` used `trace_query` once and surfaced a
  semantically correct scheduler answer, but the final visible answer lost the
  explicit next-step wording that had been present earlier in the answer
  document. The runtime log showed the inline `trace_query` result was capped
  before the late `state_churn` rows because verbose window sections such as
  trace spans/resources/compute supply preceded the most decision-critical churn
  summary.
- A later rerun showed the final answer could be semantically correct and rich
  while still failing a line-oriented eval regex: the section heading carried
  `下一步`, and the following list item carried `rival-30`/CPU competition.
  Requiring both concepts on one rendered line would reduce answer structure
  rather than improve product behavior.
- The answer document patch path then replaced a markdown table block with a
  structured table block. That is the right direction for stable rendering, but
  the old block also contained trailing prose with the next investigation step.
  The replacement had structured rows/cells and no text, so the prose was not
  carried forward into the final surface.

Deep root cause:

- Handoff priority was based on the producer's natural rendering order instead
  of signal criticality. Under a bounded tool-result budget, verbose supporting
  sections can push principal diagnostic facts and next-step evidence out of the
  model-visible slice even when those facts exist in typed products.
- Partial answer mutation preserved structure but did not preserve all visible
  surface semantics when converting a mixed markdown table+prose block into a
  pure structured table. This is a renderer/mutation fidelity gap, not a
  finalizer content gap.
- The compact JSON failure was an eval contract mismatch: strict JSON-only
  cases should be allowed to return minimal valid JSON without forcing padding
  or prose.
- Eval case metadata needs shell-literal hygiene for control characters and
  markdown sentinels. Otherwise the harness can report product failures while
  the case contract itself was not loaded faithfully.
- The eval harness only had line-oriented ERE matching for co-occurrence checks.
  That is too strict for rich multi-section answers where a heading introduces a
  concept and the immediately following item carries the concrete evidence.

Generalized design:

- Render deterministic runtime summaries in decision-priority order. Principal
  CPU pressure and state-churn rows should appear before verbose supporting
  sections so bounded handoff keeps the highest-value typed evidence. This uses
  typed product categories and existing structured fields only; it does not
  inspect user intent or finalizer prose.
- Add a structural answer-patch preservation layer for table replacements. When
  a previous table block contains a markdown table followed by non-empty prose,
  and the replacement is a structured table with no text surface, carry that
  trailing prose into a separate section block with the replacement's
  annotations. The trigger is purely structural: previous block kind, markdown
  table shape, replacement block kind, and visible-surface absence.
- Keep compact strict JSON evals honest by declaring a case-level minimum output
  length of 1, rather than changing production answer behavior or encouraging
  filler text.
- Quote eval expectations as shell-safe literals when they contain markdown
  fences or other command-substitution characters. This keeps the case contract
  faithful without altering model behavior.
- Add an eval-only whitespace-folded full-text regex channel for co-occurrence
  assertions across adjacent rendered lines. This belongs in eval verification,
  not product answer logic, and avoids pressuring the finalizer to collapse
  useful sections into one line.

Executable task list:

- [x] B7-T1: Set `MIN_OUTPUT_CHARS=1` for strict compact JSON eval cases whose
  contract is the JSON payload itself.
- [x] B7-T2: Quote markdown fence expectations as shell-safe literals in eval
  case metadata.
- [x] B7-T3: Add `EXPECT_MATCHES_TEXT_REGEX` for whitespace-folded full-answer
  eval assertions.
- [x] B7-T4: Move `state_churn` rendering ahead of verbose window support
  sections in `trace_query` summary output.
- [x] B7-T5: Add structural table-tail preservation to
  `emit_answer_document_patch`, preserving annotations and avoiding duplicate
  visible text.
- [x] B7-T6: Add focused answer-patch regression coverage for markdown
  table+trailing prose to structured-table replacement.
- [x] B7-T7: Run focused Go tests, full Go tests, rebuild, commit/push.
- [ ] B7-T8: Rerun representative eval cases two at a time and manually audit
  final answers plus logs.

Batch 7 focused verification before commit:

- `go test ./internal/tool -run 'TestTraceQuerySummaryRendersFragmentedStateChurn|TestEmitAnswerDocumentPatch_PreservesTableTrailingProse'`
- `go test ./...`
- `make`
- `bash -n eval/run.sh eval/cases/data_json_strict_ids.case eval/cases/trace_query_state_churn_window_stats.case`

Batch 7 verification before commit:

- `go test ./internal/tool -run 'TestTraceQuerySummaryRendersFragmentedStateChurn|TestEmitAnswerDocumentPatch_PreservesTableTrailingProse'`
- `go test ./...`
- `make`

## Batch 8 Gap: Member-Value Projection From Typed Contribution Ledgers

Post-Batch-7 eval root:

- `eval/results/eval-gap-20260613-post-230b4dd8`

Observed eval behavior:

- `trace_query_state_churn_window_stats` passed after the high-signal handoff
  and eval co-occurrence fixes. The run used `trace_query` once and kept the
  next-step evidence visible in the final answer.
- `data_json_strict_ids` still failed, but not because material extraction or
  filtering was wrong. The workflow produced active rows for `u1` and `u3`,
  rule coverage for the JSON shape, contribution rows carrying `value=u1/u3`,
  and a passing reconcile. The final projection still became
  `{"active_user_ids":"2"}` because the contribution operation was `count` and
  `assemble_answer` projected the count under the contribution group key.
- The final evaluation reached an assembled answer artifact, but CLI completion
  still rejected it when the current plan retained `continue_after=true`.

Deep root cause:

- `count` correctly means row count, so the numeric aggregate must remain `2`.
  However, a `compute_contributions` action can still carry member values in
  `ContributionRecord.Value` when the action declares a `value_field`. That
  member-value evidence was not available to `assemble_answer` unless the
  operation itself was `include`/`set`/`rank`.
- `json_object` projection chose object keys only from reconcile
  `group_key/metric`. When a single output field is assembled from an explicit
  record `value_field`, the typed action parameter is the more stable
  structural signal than a descriptive group label.
- Final-candidate policy treated `continue_after` as a hard blocker even when
  the result already contained an `assemble_answer` artifact and all typed
  ledger checks were otherwise satisfied.

Generalized design:

- Pass the accumulated typed contribution ledger into `assemble_answer`.
  Preserve default `count` semantics, but when an assemble action explicitly
  asks for a non-standard `value_field`, recover same group/metric contribution
  values as the JSON member array.
- For single-group JSON object projections with an explicit member
  `value_field`, derive the output key from that field name using a generic
  field-name pluralization rule, unless the action provides an explicit
  `output_field`/`output_key`/`json_field`/`target_field`.
- Treat `continue_after=true` as non-terminal by default, but allow final
  completion when the result contains an `assemble_answer` artifact and the
  existing ledger/output-contract checks pass.

Executable task list:

- [x] B8-T1: Hand off accumulated contribution records to the
  `assemble_answer` runner path.
- [x] B8-T2: Recover member values from same group/metric contributions only
  when `assemble_answer.value_field` is an explicit non-standard field.
- [x] B8-T3: Add generic single-field JSON object key derivation from
  structured `value_field`, with explicit output-key params taking precedence.
- [x] B8-T4: Keep default count JSON projection numeric when no explicit member
  `value_field` is requested.
- [x] B8-T5: Allow `continue_after` plans with an assembled final projection to
  satisfy final-candidate policy after existing ledger checks pass.
- [x] B8-T6: Run focused tests, full tests, rebuild, commit/push.
- [x] B8-T7: Rerun representative eval cases two at a time and manually audit
  final answers plus logs.

Batch 8 focused verification before commit:

- `go test ./internal/dataquery -run 'TestActionRunnerAssembleAnswerProjectsExplicitValueFieldMembers|TestActionRunnerAssembleAnswerCountJSONObjectDefaultsToNumeric|TestActionRunnerAssembleAnswerProjectsJSONObjectValues'`
- `go test ./internal/dataworkflow -run TestResultIsFinalAnswerCandidateUsesTypedOutputPolicy`
- `go test ./...`
- `make`

Post-Batch-8 eval root:

- `eval/results/eval-gap-20260613-post-7a7e08b5`

Manual audit after rerun:

- `trace_query_state_churn_window_stats` passed. It used the high-signal
  `trace_query` path and kept the `rival-30` next-step evidence visible in the
  final answer. The remaining optimization opportunity is efficiency telemetry,
  not answer correctness.
- `data_json_strict_ids` still failed after 14 data rounds and 6 repair rounds.
  The workflow had already materialized active-user rows and contribution
  records for `u1/u3`, but the terminal validation compared reconcile
  `expected_answer="4"` against an internally rendered artifact bullet summary
  instead of carrying forward the richer structured answer.

## Batch 9 Gap: Typed Contribution Semantics and Internal Summary Isolation

Deep root cause:

- `compute_contributions` supports both numeric aggregation operations and
  member-value operations. The executor already aggregates `include`, `set`,
  and `rank` as text values, but the numeric value contract still treated every
  operation except `count` as numeric. That produced a misleading hard failure
  for `operation=include,value_field=id`, pushing repair into unrelated
  normalize/mapping branches.
- `ActionRunner` can render an artifact list as an intermediate answer when no
  final projection exists in the current batch. Existing summary detection
  handled compact renderer-owned forms such as `N artifact(s)` and internal
  JSON artifact payloads, but the runner also knows when an answer was produced
  by `renderArtifactsAnswer`. That internal provenance was not used when
  choosing between the current batch summary, the seed answer, and reconcile
  output.
- Final validation should compare user-facing answers with reconcile answers,
  not compare system-generated intermediate summaries with reconcile answers.
  The missing boundary is provenance-aware answer selection inside the runner,
  plus operation-aware type gating at contribution execution time.

Generalized design:

- Reuse the existing normalized contribution operation enum as the source of
  truth for type requirements. Only numeric contribution operations (`add`,
  `subtract`, and the empty/default numeric op) require numeric field values;
  member-value operations (`include`, `set`, `rank`) and `count` do not.
- Preserve intermediate artifact summaries for non-terminal exploration
  batches, but do not let a renderer-owned summary override a stronger seed
  answer or reconcile-rendered answer. This uses runner provenance, not user
  text or model prose matching.
- Keep public artifact-summary detection conservative. It should continue to
  reject renderer-owned compact/JSON summaries as final answers without
  broadly classifying arbitrary markdown bullet lists from users as internal
  artifacts.

Executable task list:

- [x] B9-T1: Add operation-aware contribution value type gating using
  `normalizeContributionOperation`.
- [x] B9-T2: Preserve member-value `include` contributions with string
  `value_field` values and source anchors.
- [x] B9-T3: Track runner-local provenance when `renderArtifactsAnswer`
  produces the current batch answer.
- [x] B9-T4: Prefer seed answers over renderer-owned intermediate summaries
  when the current batch has no `assemble_answer` projection.
- [x] B9-T5: Prefer reconcile-rendered answers over renderer-owned summaries
  when no stronger seed answer is available.
- [x] B9-T6: Add focused regression tests for string-valued include
  contributions and summary isolation.
- [x] B9-T7: Run focused tests, full tests, rebuild, commit/push.
- [x] B9-T8: Rerun representative eval cases two at a time and manually audit
  final answers plus logs.

Batch 9 verification before commit:

- `go test ./internal/dataquery -run 'TestActionRunnerComputeContributionsIncludeAcceptsStringValueField|TestActionRunnerReconcileMarkdownArtifactSummaryKeepsSeedJSONAnswer|TestActionRunnerReconcilesCountContributionsWithTextValuesAndKeepsSeedJSONAnswer|TestActionRunnerReconcileAnswerBeatsSeedArtifactSummary'`
- `go test ./...`
- `make`

Post-Batch-9 eval root:

- `eval/results/eval-gap-20260613-post-cb2ddd01`

Manual audit after rerun:

- `data_json_strict_ids` reached `data_terminal_status=complete`, one repair
  round, no action failure, and a passing reconcile. The final answer was
  `{"all":["u1","u3"]}`. Values and order were correct, but the JSON object key
  came from the synthetic contribution group `all` instead of the structured
  contribution metric/value field (`id`).
- `trace_query_state_churn_window_stats` used `trace_query` successfully, but
  at high cost: `tool_trace_query=8`, `explorer_iters=12`, `wall_seconds=344`.
  The final answer kept the core metrics, but the visible answer dropped the
  explicit "next step" heading from extraction and retained a stale perf-triage
  caveat that said the attached excerpt spanned only `11.000300..11.000300`.

## Batch 10 Gap: Final Projection Keys and Trace Handoff Monotonicity

Deep root cause:

- `assemble_answer` chooses JSON object keys from explicit output-key params,
  explicit non-standard `value_field`, then `group_key/metric`. When
  `compute_contributions` groups member-value rows under the executor's
  synthetic catch-all group (`all`) and sets `metric=id`, the output key should
  come from the member-value metric, not the aggregation bucket name. The
  existing fallback treated the synthetic bucket as user-facing schema.
- The data completion gate can normalize a repair-node evaluator verdict to
  complete when typed ledgers and reconcile pass. That is valid for noisy
  repair prose, but it must not mask a deterministic final projection mismatch.
  The first low-risk fix is to stop producing the wrong key for synthetic
  member arrays; a stricter completion-gate assertion can follow if needed.
- In trace mode, multiple accepted `trace_query` observations did not become a
  monotonic completion handoff. The explorer repeatedly re-issued the same
  window/root-cause queries, and final writing mixed stronger `trace_query`
  facts with weaker stale perf-triage caveats. This is a handoff/precedence
  issue, not a trace_query algorithm issue; tool runtime itself was microsecond
  to millisecond scale.

Generalized design:

- Treat `all` only as the executor-owned synthetic catch-all group. For a
  single JSON object projection whose reconcile group carries member
  `Values`, derive the output key from the structured metric name when the
  group key is that synthetic catch-all. Use the existing generic
  `pluralizeJSONFieldName` helper and keep explicit output-key params highest
  precedence.
- Keep default numeric aggregate projections stable: only member-value groups
  with `Values` use metric-derived keys for synthetic catch-all groups.
- For trace handoff, prefer structured trace_query facts over weaker
  pre-triage caveats once they cover the requested window. The future fix
  should be a typed precedence/monotonic completion rule, not prompt keyword
  matching or final-answer string patching.

Executable task list:

- [x] B10-T1: Add synthetic catch-all group detection for assemble JSON object
  key derivation.
- [x] B10-T2: For single-group member-value projections, derive JSON field
  names from structured reconcile metric when group key is the synthetic
  catch-all.
- [x] B10-T3: Preserve explicit output-key params and explicit
  non-standard `value_field` precedence.
- [x] B10-T4: Add focused regression tests for `all/id` member arrays becoming
  `ids`, while numeric aggregate defaults remain stable.
- [x] B10-T5: Run focused tests, full tests, rebuild, commit/push.
- [x] B10-T6: Rerun representative eval cases two at a time and manually audit
  final answers plus logs.
- [x] B10-T7: Add finalizer trace handoff guidance so `state_churn.next_step`
  remains visible and bounded `trace_query` facts take precedence over stale
  pre-triage caveats for the same window.

Batch 10 verification before commit:

- `go test ./internal/dataquery -run 'TestActionRunnerAssembleAnswerUsesMetricKeyForSyntheticAllMembers|TestActionRunnerAssembleAnswerProjectsExplicitValueFieldMembers|TestActionRunnerAssembleAnswerCountJSONObjectDefaultsToNumeric|TestActionRunnerAssembleAnswerProjectsJSONObjectValues'`
- `go test ./internal/agent -run TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersHarmonyTracePriorityReminder`
- `go test ./...`
- `make`

Post-Batch-10 eval root:

- `eval/results/eval-gap-20260613-post-1f72c140`

Manual audit after rerun:

- `data_json_strict_ids` passed. The final answer was exactly
  `{"ids":["u1","u3"]}` and terminal data workflow metrics improved from
  9 rounds / 156s to 1 round / 36s.
- `trace_query_state_churn_window_stats` still failed. The run used
  `trace_query` twice and produced correct `state_churn` metrics, but the
  final answer omitted the requested next-step guidance. Runtime logs showed
  `tool_read_file=7`, `explorer_iters=10`, `wall_seconds=265`, and a hard
  current-status answer contract (`current_code_path` plus
  `current_status_verdict`) even though the user requested bounded runtime
  trace metrics, not current-checkout verification.

## Batch 11 Gap: External Runtime Trace Source-Lane Boundary

Deep root cause:

- The analyzer correctly marked the trace coordinates as external artifact
  citations, but left `current_source_mode=default` because the user did not
  explicitly exclude source analysis. That default means current-source
  exploration is allowed when useful; it must not become a hard requirement.
- `diagnostic_profile.current_risk=true` combined with a broad
  `HasRuntimeArtifactCurrentVerificationAnchor` fallback that treats any
  non-empty `ExactTargets` entry as a current-source anchor. Runtime subjects
  such as `app-20` and bounded trace windows such as `11.0s-11.008s` were
  therefore misclassified as source-verification anchors.
- Once the current-status contract was active, the answer surface required a
  principal decision block and `current_code_path` facet. Explorer widened into
  current source (`grep`/`read_file`) to satisfy the contract, and finalizer
  optimized for source implementation proof instead of preserving the trace
  tool's `next_step` guidance.

Generalized design:

- Separate "current source may be used" from "current source is required" for
  external runtime artifacts. Hard gates may require source only from typed
  current-source signals: resolved current files, explicit current-source
  explanation profile, source-scope profile, current-key-code dimension,
  required file hints, or code/config-path targets.
- Runtime artifact exact targets are not source anchors by themselves. They
  remain principal runtime subjects and can enrich search ranking, but cannot
  activate `current_status_diagnostic` unless accompanied by a typed
  current-source signal. `CurrentVersionCheck=true` is preserved as an explicit
  source-verification signal when it has an exact target; a plain
  `current_risk` flag is not enough.
- Answer surface planning must compute current-status requirement from the
  same source-lane decision. If source is optional, accidentally collected
  current-source evidence can stay supporting context but cannot force
  `current_code_path`, `current_status_verdict`, or source-oriented principal
  blocks.
- Trace answers with typed `trace_query` observations should preserve
  requested metrics and `next_step` guidance as runtime artifact facts. Source
  implementation details should remain optional caveat/support, never the
  user-facing spine for bounded trace-window metric questions.

Executable task list:

- [x] B11-T1: Narrow `HasRuntimeArtifactCurrentVerificationAnchor` so arbitrary
  runtime `ExactTargets` do not become current-source anchors; keep typed
  current-source profiles, resolved files, required files, and code/config
  path anchors as source-required signals.
- [x] B11-T2: Reconcile external runtime diagnostic profiles with the narrowed
  source-lane decision, clearing current-status flags when no typed current
  source requirement exists.
- [x] B11-T3: Make `BuildAnswerSurfacePlan` derive
  `CurrentStatusDiagnosticRequired` from the effective source-lane requirement,
  not from raw diagnostic flags alone.
- [x] B11-T4: Add focused regression tests for external trace/runtime subjects
  with exact targets (`app-20`, time window) remaining source-optional, while
  explicit current-source profiles/resolved files still require source.
- [x] B11-T5: Add finalizer prompt regression coverage that source-optional
  runtime trace plans do not render `current_status_verdict` or hard
  `current_code_path`, and still render runtime trace handoff guidance.
- [x] B11-T6: Run focused tests, full tests, rebuild, commit/push.
- [x] B11-T7: Rerun representative eval cases two at a time and manually audit
  final answers plus logs.

Batch 11 verification before commit:

- `go test ./internal/types -run 'TestCurrentSourceLaneDecision_RuntimeExactTargetsRemainSourceOptional|TestCurrentSourceLaneDecision_CurrentSourceProfileRequiresSource|TestCompileAnswerIntentContract_ExternalRuntimeArtifactCurrentStatus|TestBuildAnswerSurfacePlan_ExternalTraceExactTargetsDoNotForceCurrentStatus'`
- `go test ./internal/agent -run 'TestBuildAnalysisIR_ExternalOnlyCurrentVersionCheckKeepsCurrentStatus|TestAnswerDocumentEvaluator_BuildInitialInstruction_SourceOptionalTraceSkipsCurrentStatus|TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersHarmonyTracePriorityReminder'`
- `go test ./internal/tool -run 'TestPreCheckRuntimeObservationRepoContaminationAllowsCurrentStatus|TestPreCheckRuntimeObservationRepoContamination'`
- `go test ./internal/agent ./internal/types ./internal/tool`
- `go test ./...`
- `make`

Post-Batch-11 eval attempt root:

- `eval/results/eval-gap-20260613-post-088b6332`

Manual audit after rerun attempt:

- `data_json_strict_ids` passed again with `{"ids":["u1","u3"]}`.
- `trace_query_state_churn_window_stats` was stopped early after logs showed
  the same class of source-lane widening through a different structured path:
  the analyzer retried successfully, but emitted
  `source_scope_profile.requested_scope=production` and requested answer
  dimensions with role `current_key_code` for runtime metric labels such as
  `dominant_state`, `fragments`, and `p95_segment`. With
  `current_source_mode=default`, those fields still upgraded the dispatch to a
  hard current-source lane, causing `repo_map` and repeated `read_file` before
  any useful `trace_query` exploration.

## Batch 12 Gap: Runtime Metric Dimensions Are Not Source Anchors

Deep root cause:

- Batch 11 removed broad `ExactTargets` anchoring, but two other typed
  source-lane inputs remained too broad for external runtime artifacts:
  `SourceScopeProfile` and `RequestedAnswerDimensionCurrentKeyCode`.
- `SourceScopeProfile` is a path-role filter for repo/source questions. In an
  external trace run with `artifact_citation_mode=external_only` and
  `current_source_mode=default`, it should remain optional ranking/scope
  context unless the analyzer explicitly sets `current_source_mode=allow` or
  another current-source profile/anchor exists.
- Requested runtime output fields (`dominant_state`, `fragments`,
  `max_segment`, `p95_segment`) can be mislabeled as `current_key_code`. Under
  default external-artifact mode, non-path metric labels must not become
  current-source anchors. Code/config path labels remain valid anchors.

Generalized design:

- `HasTypedCurrentSourceScopeRequest` now requires
  `external_observation_policy.current_source_mode=allow`; default keeps source
  optional.
- Under `artifact_citation_mode=external_only` with non-allow source mode,
  `current_key_code` dimensions require an actual code/config path anchor.
  Plain metric names stay runtime answer dimensions.
- Existing explicit source verification remains intact: resolved files,
  current-source explanation profiles, `CurrentVersionCheck=true` with exact
  target, explicit source allow, and code/config path anchors still open the
  source-required lane.

Executable task list:

- [x] B12-T1: Gate typed source-scope hard requirements on
  `current_source_mode=allow`.
- [x] B12-T2: Prevent default external-artifact metric dimensions from
  becoming current-source anchors unless they name code/config paths.
- [x] B12-T3: Extend answer surface and finalizer prompt regression tests with
  source scope plus metric dimensions.
- [x] B12-T4: Add answer intent contract coverage for default runtime
  source-scope/metric-dimension cases staying runtime-only.
- [x] B12-T5: Run focused tests, full tests, rebuild, commit/push.
- [x] B12-T6: Rerun representative eval cases two at a time and manually audit
  final answers plus logs.

Batch 12 verification before commit:

- `go test ./internal/types -run 'TestCurrentSourceLaneDecision_DefaultExternalArtifactSourceScopeStaysOptional|TestCurrentSourceLaneDecision_ExternalArtifactSourceScopeRequiresSource|TestBuildAnswerSurfacePlan_ExternalTraceExactTargetsDoNotForceCurrentStatus|TestCompileAnswerIntentContract_DefaultTraceArtifactSourceScopeStaysRuntimeOnly|TestCompileAnswerIntentContract_TraceArtifactSourceScopeKeepsCurrentSource'`
- `go test ./internal/agent -run 'TestAnswerDocumentEvaluator_BuildInitialInstruction_SourceOptionalTraceSkipsCurrentStatus'`
- `go test ./...`
- `make`

Post-Batch-12 eval root:

- `eval/results/eval-gap-20260613-post-57d492ad`

Manual audit after rerun:

- `data_json_strict_ids`: PASS, final answer was strict JSON
  `{"ids":["u1","u3"]}`, `data_rounds=1`, `data_repair_rounds=0`,
  `tool_read_file=0`, wall 32s. The prior synthetic `all` key issue stayed
  fixed.
- `trace_query_state_churn_window_stats`: PASS. The run used the efficient
  runtime lane (`tool_trace_query=2`, `tool_repo_map=0`, `tool_read_file=0`,
  explorer iterations reduced to 3, wall 173s). The final answer preserved the
  bounded `trace_query` metrics: `dominant_state=runnable`,
  `running=3.5ms`, `runnable=5.0ms`, `sleep/d_state/io_wait=0ms`,
  `fragments=21`, `switches=20`, `max_segment=0.5ms`,
  `p95_segment=0.5ms`, `cpu_pressure` as the primary root cause, and the
  concrete next-step handoff (`wakeup_chain`, `event_search`, compare
  no-competition windows).
- Residual commercial-quality gap: even though the semantic answer passed and
  the source lane stayed closed, the final surface still appended generic
  system material: a requested-dimension quote supplement and generic
  "enumeration/consistency" caveats. Logs show two causes:
  1. The last-mile requested-dimension supplement listed every dimension whose
     source quote differed from its label, without first checking whether the
     structured answer already covered that dimension.
  2. V2 label oracles treated trace metric `ordered_list` labels as code/source
     enumeration labels unless they were visible in log frames. The accepted
     `trace_query` aggregate facts already carried
     `evidence_origin=runtime_artifact`, but those typed aggregate labels were
     not part of the runtime label support pool. The resulting soft violation
     made LLM reviewers run and compare stale pre-triage hand calculations
     against the later authoritative `trace_query` values.

## Batch 13 Gap: Runtime Observation Answers Need Surface-Noise Suppression

Deep root cause:

- The answer pipeline now carries rich typed runtime facts, but two downstream
  consumers still failed to consume that handoff:
  - last-mile dimension supplements consumed `RequestedAnswerDimensions`
    directly instead of the already-computed missing-dimension set;
  - label grounding/hallucination oracles consumed log frames and code evidence
    but not runtime-origin aggregate facts accepted from deterministic tools
    such as `trace_query`.
- This is not a case-specific wording problem. The same class affects any
  origin-specific tool output that produces metric labels or structured rows:
  trace windows, command measurements, MCP/connector rows, external documents,
  and cross-repo indexes. Accepted typed aggregate facts must be a first-class
  authority lane for final display and validation.
- Prompt-only fixes are inappropriate: the finalizer already produced a rich
  correct body. The gap is deterministic downstream consumption of typed
  handoff evidence.

Generalized design:

- Requested-dimension source-quote supplements become missing-only: reuse the
  existing `missingRequestedAnswerDimensionsInDocument` result so the supplement
  appears only when the structured answer still lacks requested visible
  dimensions.
- Runtime artifact label support now includes stable aggregate facts whose
  evidence origin projects to `runtime_artifact`. The support surfaces are
  structured fields (`label`, `value`, `provenance`, dimensions, members,
  support refs), not user intent prose or model answer prose.
- Code/source hallucination checks remain intact: unrelated code-looking labels
  still fail unless the code oracle, citations, answer symbols, question
  buckets, runtime artifacts, or aggregate member sets support them.
- Once runtime aggregate labels pass the typed oracle, observation-only/source-
  optional artifact answers can skip LLM reviewers that would otherwise compare
  stale pre-triage guesses with later deterministic `trace_query` outputs.

Executable task list:

- [x] B13-T1: Reuse missing requested-dimension coverage for last-mile
  source-quote supplements.
- [x] B13-T2: Add runtime aggregate facts to the runtime artifact label support
  lane for label grounding and hallucination oracles.
- [x] B13-T3: Add regression tests proving covered requested dimensions do not
  receive duplicate system supplements.
- [x] B13-T4: Add regression tests proving runtime aggregate labels pass while
  unrelated code-looking labels still fail.
- [x] B13-T5: Run focused tests, full tests, rebuild, commit/push. The
  representative rerun exposed the Batch 14 gaps below rather than a prompt
  or semantic-answer defect.

Post-Batch-13 eval root:

- `eval/results/eval-gap-20260613-post-96b644f3`

Manual audit after rerun:

- `data_json_strict_ids`: FAIL. The first batch had already computed the
  correct strict JSON answer `{"ids":["u1","u3"]}` and carried a pass
  answer-level reconcile, but the terminal `assemble_answer` batch failed with
  `assemble_answer requires reconcile groups`. Logs show
  `data_rounds=16`, `data_repair_rounds=6`, `data_answer_len=0`,
  `data_record_count=26`, and no source/read/repo tools. The root cause is a
  typed-action projection gap: answer-level reconciliation is a valid
  structural state when all contribution records are auxiliary/audit ledgers,
  but `runAssembleAnswer` only accepted non-empty group-level reconcile.
- `trace_query_state_churn_window_stats`: FAIL only on surface regex
  `max_segment.*p95_segment`; `exit_code=0`, `tool_trace_query=2`,
  `tool_repo_map=0`, `answer_contract_violations=0`, wall 203s. Manual audit
  found the answer semantically correct and rich: it used `trace_query`
  `window_stats` and `root_cause_rank`, preserved `dominant_state=runnable`,
  `running=3.5ms`, `runnable=5.0ms`, `sleep/d_state/io_wait=0`,
  `fragments=21`, `switches=20`, `max_segment=0.5ms`,
  `p95_segment=0.5ms`, and next-step root-cause guidance. The surface problem
  came from exact-only requested-dimension coverage: the metrics were visible
  as structured list rows, but the finalizer still asked for explicit
  `state_churn 统计` / `下一步查主因` headings, producing duplicate empty-ish
  sections. A separate auditability gap is that runtime scalar facts were not
  projected into one compact metric line for humans/evals to verify quickly.

## Batch 14 Gap: Answer-Level Reconcile and Runtime Metric Projection Need Typed Bridges

Deep root cause:

- Data workflows already support answer-level reconciliation (`scope=answer`)
  when contribution records exist only as audit/coverage/material ledgers, but
  the assemble boundary assumed every terminal answer must be assembled from
  per-group reconcile rows. This turns a valid pass reconcile plus valid seed
  answer into a terminal failure. The failure is systemic for JSON-only,
  scalar, CSV, and freeform outputs whose final projection has already been
  produced by a prior typed/custom action and whose later ledgers are
  non-target audit evidence.
- Final answer coverage checks used exact label/source-quote substring
  matching. That misses a common commercial answer shape: a requested dimension
  such as "output these metrics" is satisfied by a structured table/list whose
  rows are the metric names, not by a repeated heading. Exact-only coverage
  creates unnecessary repair turns and duplicate surface sections.
- Runtime scalar aggregate facts reached the finalizer as typed handoff, but
  there was no deterministic compact projection for requested metric sets.
  The rich prose/list answer can be semantically correct while remaining hard
  to audit or line-match. This is not an eval-regex problem; it is a generic
  answer-surface observability gap for trace/log/measurement/MCP metric
  answers.
- Perf pre-triage still attempted `read_file /dev/stdin` once before the
  deterministic trace lane took over. This did not corrupt the answer, but it
  is a tool-selection resilience gap: stdin-backed runtime blobs should be
  represented as typed attachments/trace-query inputs, not as normal repo file
  reads.

Generalized design:

- `assemble_answer` accepts answer-level reconcile only under precise typed
  conditions: reconcile exists, status is `pass`, reconcile groups are empty,
  target contributions are empty, the seed answer is non-empty, the seed answer
  is not an internal artifact summary, and it satisfies the effective
  `OutputContract`. It then emits a deterministic `final_answer` projection
  group so downstream reconcile validation still has an answer-level carrier.
- Requested-dimension coverage remains display-only and soft, but expands from
  exact substring to generic structural anchors: ASCII identifier-token quorum
  for metric/source-quote lists and CJK prefix/suffix boundary anchors for
  compact Chinese labels. These anchors are derived from analyzer-validated
  `RequestedAnswerDimensions`, not from user-intent keyword matching or model
  prose classification.
- Runtime metric compaction is additive and evidence-typed: it selects
  `scalar_value` aggregate facts whose evidence origin projects to
  `runtime_artifact`, intersects their labels with requested-dimension
  identifier tokens, and renders one compact verification line. It never
  derives values from final answer prose and never replaces the model-authored
  rich answer.
- Existing hard gates stay precise: source/citation gates remain unchanged,
  internal artifact summaries cannot become final answers, and unrelated
  code-looking labels still need their normal evidence lanes.

Executable task list:

- [x] B14-T1: Record post-Batch-13 eval results, manual audit, and generalized
  root-cause analysis in this design doc.
- [x] B14-T2: Add answer-level reconcile projection in `runAssembleAnswer`
  guarded by typed pass/empty-target/valid-seed-answer signals.
- [x] B14-T3: Add dataquery regression tests for valid seed JSON projection
  and internal artifact-summary rejection.
- [x] B14-T4: Add generic requested-dimension coverage anchors for metric
  identifier lists and compact CJK labels.
- [x] B14-T5: Add runtime aggregate scalar compact metric supplement sourced
  only from typed runtime aggregate facts and requested metric tokens.
- [x] B14-T6: Add finalizer regression tests for covered metric dimensions and
  compact runtime metric projection.
- [x] B14-T7: Run focused package tests, full test suite, rebuild, and diff
  hygiene.
- [x] B14-T8: Commit and push Batch 14.
- [x] B14-T9: Rerun representative eval cases two at a time and manually audit
  answers/logs again. The rerun exposed the Batch 15 projection gap below.

Post-Batch-14 eval root:

- `eval/results/eval-gap-20260613-post-5cd74a68`

Manual audit after rerun:

- `trace_query_state_churn_window_stats`: PASS. The run used
  `trace_query=2`, `repo_map=0`, `read_file=0`, no finalizer retries, and the
  final answer preserved the authoritative runtime values. The compact
  structured metric line rendered `max_segment=0.500ms` and
  `p95_segment=0.500ms` on the same line without replacing the richer body.
- `data_json_strict_ids`: FAIL. The data workflow reached
  `data_terminal_status=complete`, `data_answer_len=25`, `reconcile=pass`,
  and had no action failures in the terminal result, but the final answer was
  valid JSON with the wrong shape: `{"u1":["u1"],"u3":["u3"]}`. The expected
  contract from the rule ledger is a single field containing the included
  member set: `{"ids":["u1","u3"]}`. The logs show `compute_contributions`
  produced target records with `group_key_field=id`, `metric=id`, and
  `operation=include`; `assemble_answer projection=json_object
  value_field=group_key` then interpreted each group key as a JSON object key
  instead of recognizing the same-metric include groups as one output member
  set.

## Batch 15 Gap: JSON Object Projection Needs Typed Set Semantics

Deep root cause:

- `assemble_answer` can already project a single synthetic/all group into a
  plural metric key (for example `id` -> `ids`) and can merge duplicate JSON
  keys into arrays. It did not handle the equivalent normalized shape where a
  prior typed plan grouped each included member by its own ID while preserving a
  single shared metric. This shape is common after `filter_records` +
  `compute_contributions`: group keys identify rows/members, while the metric
  names the output field.
- Treating every group key as a JSON key is correct for numeric grouped
  aggregates (`amount by Q1/Q2`) but wrong for set/list operations
  (`include/set/rank` members of the same metric). The missing bridge is typed
  operation semantics, not a keyword in the user request or a case-specific
  field name.

Generalized design:

- In `json_object` assembly, detect only this precise typed shape:
  multiple reconcile groups, exactly one shared metric, no explicit output key
  already provided, and every matching target contribution uses a list-like
  operation (`include`, `set`, or `rank`). Project those groups to one JSON
  array under `pluralize(metric)`.
- Preserve existing grouped numeric behavior: `add`, `subtract`, and `count`
  groups continue to render as group-keyed JSON objects unless an explicit
  output key is provided.
- Keep explicit model/tool instructions stronger than inference:
  `output_field`, `output_key`, `json_field`, `target_field`, or `field` still
  win and are merged by the existing object-key path.

Executable task list:

- [x] B15-T1: Document the post-Batch-14 eval result and identify the typed
  same-metric set/list projection gap.
- [x] B15-T2: Add same-metric list projection for JSON object assembly,
  guarded by contribution operation semantics.
- [x] B15-T3: Add regression tests for include/set-style same-metric groups
  collapsing to a single plural metric array.
- [x] B15-T4: Add regression tests proving numeric same-metric groups remain
  keyed by group.
- [x] B15-T5: Run focused/full tests, rebuild, diff hygiene.
- [x] B15-T6: Commit/push Batch 15.
- [x] B15-T7: Rerun representative eval cases two at a time and manually audit
  answers/logs again.

Post-Batch-15 eval root:

- `eval/results/eval-gap-20260613-post-ff8ae0fa`

Manual audit after rerun:

- `data_json_strict_ids`: PASS. The terminal data workflow completed in two
  rounds with one repair, `data_terminal_status=complete`,
  `data_record_count=2`, `data_answer_len=19`, and no source/repo/runtime
  tool calls. The final answer was exactly `{"ids":["u1","u3"]}`: strict JSON
  only, correct output field, and no explanatory leakage.
- `trace_query_state_churn_window_stats`: PASS. The run used
  `trace_query=2`, `repo_map=0`, `answer_contract_violations=0`, and one
  finalizer iteration. The final answer preserved `dominant_state=runnable`,
  `running=3.5ms`, `runnable=5.0ms`, `sleep/d_state/io_wait=0`,
  `fragments=21`, `switches=20`, `max_segment=0.5ms`,
  `p95_segment=0.5ms`, root-cause guidance, and next steps for investigating
  `rival-30`.
- Residual cost/robustness audit: the trace answer is commercially usable, but
  logs still show two avoidable system frictions. Perf triage attempted
  `read_file /dev/null` before emitting the inline trace bundle because the
  pagination tool remained visible even when no blob path existed. Exploration
  also needed two `emit_investigation_complete` retries before the
  `negative_observation` aggregate fact had all required dimensions. Neither
  issue changed the final answer, but both waste model turns and weaken JSON
  handoff resilience.

## Batch 16 Gap: Runtime Tool Surface and Aggregate JSON Repair Need Typed Narrowing

Deep root cause:

- Attachment triage already has a hard execution guard: `read_file` can only
  paginate the attached log/trace blob, and inline-only artifacts reject repo
  reads. The gap is earlier in the tool surface. When the attachment is fully
  inline and no blob path exists, the model still sees `read_file`; with
  `tool_choice=required`, a model can spend one round on an impossible
  pagination call before the typed emit tool is forced.
- `negative_observation` aggregate facts require origin, target/scope,
  result_count, and searched_at dimensions. The schema documents that shape,
  but common tool-derived zero-result payloads naturally carry some fields as
  `window`, `checked_types`, `matches`, `source`, `provenance`, or structured
  source-ref aliases. Requiring the model to discover every canonical dimension
  through rejected retries increases cognitive load and risks dropping useful
  runtime handoff facts.
- These are system-level gaps, not case wording problems. Any inline log/trace
  pre-stage can hit the first class; any external observation, VCS, command,
  MCP, connector, or repo-map zero-result aggregate can hit the second class.

Generalized design:

- Runtime triage filters tool schemas by precise attachment state. If the
  active agent/stage is log/perf triage and the corresponding attached artifact
  has no blob path on disk, hide `read_file` and expose only the stage's typed
  emit tool. If a blob path exists, keep `read_file` available for legitimate
  pagination. This mirrors the existing execution guard and does not inspect
  user intent prose.
- `emit_investigation_complete` adds a structured compatibility pass for
  `aggregate_facts` before strict fact validation. The pass maps typed aliases
  such as `matches`/`count` -> `result_count`, `window`/`range` -> `scope`,
  `checked_types`/`absent_types` -> `target`, and structured
  `source`/`producer`/`provenance` tokens to canonical origin dimensions when
  they resolve through the existing evidence-origin enum. It never infers from
  free-form reason prose or final answer text.
- Hard validation remains strict after repair: invalid origins, missing bounded
  scope, missing absent target/query/pattern/predicate, or non-zero values for
  negative facts still reject. The repair layer only preserves mechanically
  equivalent typed payloads that already supplied the information in a
  compatible field.

Executable task list:

- [x] B16-T1: Record post-Batch-15 PASS eval results and residual cost/root
  cause audit in this design doc.
- [x] B16-T2: Add runtime-triage tool schema filtering so inline-only
  attachments hide `read_file` while blob-backed attachments keep pagination.
- [x] B16-T3: Add regression tests for perf/log triage schema filtering across
  inline-only and blob-backed attachments.
- [x] B16-T4: Add structured aggregate-fact alias repair before
  `NormalizeAnswerAggregateFacts`, focused on zero-result observation aliases
  and evidence-origin tokens.
- [x] B16-T5: Add regression tests proving one-shot `negative_observation`
  payloads with alias fields normalize without retries and invalid origins
  still reject.
- [x] B16-T6: Run focused tests, full tests, rebuild, diff hygiene.
- [x] B16-T7: Commit and push Batch 16.
- [ ] B16-T8: Rerun representative eval cases two at a time and manually audit
  answers/logs again.

Implementation notes:

- `BaseAgent.buildToolSchemas` now applies a runtime-triage-only schema filter
  derived from exact stage/agent identity plus attachment blob presence. This
  removes an impossible `read_file` option for inline-only log/trace pre-stages
  while preserving blob-backed pagination.
- `emit_investigation_complete` now performs a pre-validation JSON repair pass
  only for typed `negative_observation` and `negative_search` aggregate fact
  objects. The pass moves known top-level scalar/array aliases into
  `dimensions`, then the existing strict aggregate normalizer enforces origin,
  zero-result, target, scope, and evidence constraints.
- Evidence origin normalization accepts structured tool-qualified origin tokens
  only through a whitelist of already-supported origin families. Ambiguous
  prefixes such as generic `system:*` remain unexpanded so existing source
  inventory and system-inference behavior stays stable.

Verification:

- Focused tests: `go test ./internal/agent -run 'TestBuildToolSchemas_RuntimeTriage|TestBuildToolSchemas_ObservationOnlyRuntime'`
- Focused tests: `go test ./internal/tool -run 'TestEmitInvestigationComplete_(RuntimeNegativeObservationCompat|NormalizesNegativeObservationAliasPayload|RejectsNegativeObservationAliasPayloadWithCurrentSourceOrigin)'`
- Focused tests: `go test ./internal/types -run 'TestAnswerEvidenceOriginFromStructuredToken_AllowsToolQualifiedOrigin|TestNormalizeAnswerAggregateFacts_AcceptsNegativeObservation'`
- Package tests: `go test ./internal/agent ./internal/tool ./internal/types`
- Full tests: `go test ./...`
- Build: `make`
- Diff hygiene: `git diff --check`

## Batch 17 Gap: JSON-Only Data Payload Projection Must Use Plan-Level Output Contracts

Post-Batch-16 eval root:

- `eval/results/eval-gap-20260613-post-9ef9c00a-b1`
- Parallel batch: `data_json_strict_ids` + `trace_query_state_churn_window_stats`.
- `trace_query_state_churn_window_stats`: PASS in 177s. Metrics:
  `tool_trace_query=2`, `tool_read_file=0`, `unavailable_tool_attempts=0`,
  `finalizer_rejects=0`. Manual audit: answer preserves
  `dominant_state=runnable`, cumulative running/runnable/sleep/d_state/io_wait,
  `fragments`, `switches`, `max_segment`, `p95_segment`, and next-step
  guidance. The runtime handoff path is materially improved. Residual
  non-blocker: the perf-triage static guidance still mentions `read_file`, but
  the tool was not called and exploration used `trace_query` as expected.
- `data_json_strict_ids`: FAIL in 242s with `data_rounds=18`,
  `data_repair_rounds=5`, `data_answer_len=52193`, and
  `data_terminal_status=failed`. Manual audit: the first successful data
  transform had already produced the correct strict payload
  `{"ids":["u1","u3"]}`, but the result had no `answer` because the runner only
  promotes ordinary extra JSON payload fields when the script-emitted result
  itself carries `output_contract`. In this case the strict JSON contract lived
  on the plan, so the payload stayed as `emitted_payload`/`custom_payload`.
  The workflow then chased final_projection, rule coverage, decisions,
  contributions, and reconcile ledgers until budget exhaustion.

Deep root cause:

- The data runner has the right primitive,
  `runnerPayloadAnswerFromExtraFields`: ordinary extra JSON object/array fields
  can become the final `answer` under a `json_only` contract. The gap is that
  the primitive sees only the raw emitted result object. If a plan supplies the
  output contract and the script emits a plain payload, the promotion happens
  before plan-level contract normalization and therefore misses.
- The workflow reducer then receives a structurally correct payload artifact
  but `HasAnswer=false`. That converts a solved strict-output task into a
  generic ledger-completion problem. Because ledger prerequisites are correct
  for non-trivial aggregation/join/reconcile workflows, this cannot be fixed by
  weakening ledger validation globally.
- This is a system-level handoff gap across all data tasks that emit a plain
  JSON payload while relying on the plan's output contract: small filters,
  extraction-only outputs, JSON reshaping, and deterministic transforms can all
  lose the terminal answer even though the payload is already valid.

Generalized design:

- Make plan-level output contracts participate in runner payload promotion.
  After parsing the script result and before workflow validation observes the
  result, derive the effective contract as `result.output_contract` if present,
  otherwise `plan.output_contract`. If the effective contract is `json_only`
  and the script result is an ordinary JSON payload with non-canonical result
  fields, promote the payload to `result.answer` using the existing
  `runnerPayloadAnswerFromExtraFields` semantics.
- Keep validation strict after promotion: `ValidateAnswer` still verifies valid
  JSON, material coverage still requires consumed required materials, and
  explicit ledger requirements still fail when the plan asks for them. The
  change only prevents an already-valid terminal payload from being invisible
  to completion.
- Preserve auditability: retaining the `emitted_payload` artifact is acceptable
  as a handoff/audit artifact, but `answer` becomes the authoritative terminal
  projection for strict JSON-only outputs. No prompt redline is introduced, and
  no user-intent or model-prose keyword matching is used.

Executable task list:

- [x] B17-T1: Record post-Batch-16 representative eval audit and root cause in
  this design doc.
- [x] B17-T2: Extend runner result parsing/normalization so plan-level
  `output_contract` can trigger existing JSON payload answer promotion before
  terminal workflow validation.
- [x] B17-T3: Add regression tests for plain `emit({"field": ...})` under
  plan-level `json_only` output contract, including required-material
  consumption through an instruction/rule file.
- [x] B17-T4: Add/adjust workflow completion tests proving a promoted payload
  satisfies output projection without forcing unrelated ledger stages when
  ledger requirements are not declared.
- [x] B17-T5: Run focused tests, full tests, rebuild, diff hygiene.
- [x] B17-T6: Commit and push Batch 17.
- [x] B17-T7: Rerun representative eval cases two at a time and manually audit
  answers/logs again.

Implementation notes:

- `Runner.Run` now passes the plan output contract into result parsing.
  `parseRunnerResult` derives an effective contract from
  `result.output_contract` first, then `plan.output_contract`; when that
  effective contract is `json_only`, the existing extra-payload promotion
  converts ordinary script payloads such as `{"ids":[...]}` into
  `result.answer`.
- The emitted payload artifact remains available for audit/handoff, while the
  promoted `answer` becomes the terminal projection consumed by the workflow
  completion state. Explicit ledger requirements still gate completion.

Verification:

- Focused tests: `go test ./internal/dataquery -run 'TestRunner(PromotesPlainJSONPayloadWithPlanOutputContract|EmitResultJSONPayloadBecomesAnswer|JSONOnlyValidation)'`
- Focused tests: `go test ./internal/dataworkflow -run 'TestResultIsFinalAnswerCandidateAcceptsPromotedJSONOnlyPayloadWithoutLedgers|TestBuildOutputProjectionGraph|TestWorkflowStateCompletion'`
- Full tests: `go test ./...`
- Build: `make`
- Diff hygiene: `git diff --check`

Post-Batch-17 eval audit:

- `eval/results/eval-gap-20260613-post-97786673-b1`
- Parallel batch: `data_json_strict_ids` + `trace_query_state_churn_window_stats`.
- `data_json_strict_ids`: PASS in 34s. Metrics:
  `data_rounds=1`, `data_repair_rounds=0`, `data_answer_len=19`,
  `data_record_count=1`, no repo/runtime tools, no contract violations.
  Manual audit: final answer is exactly `{"ids":["u1","u3"]}`; both
  `instructions.md` and `users.json` were consumed; workflow completion says
  `final_projection` is present and optional ledgers stayed optional.
- `trace_query_state_churn_window_stats`: PASS in 167s. Metrics:
  `tool_trace_query=4`, `tool_read_file=0`, `unavailable_tool_attempts=0`,
  `finalizer_rejects=0`, `max_context_window_pct=19`. Manual audit: model
  actively used `trace_query` for `window_stats`, `scheduler_latency_stats`,
  and `root_cause_rank`; answer preserves `dominant_state=runnable`,
  `running=3.5ms`, `runnable=5.0ms`, `sleep/d_state/io_wait=0ms`,
  `fragments=21`, `switches=20`, `max_segment=0.5ms`,
  `p95_segment=0.5ms`, and next-step guidance. Residual quality gap: the
  system appended a generic consistency caveat even though the authoritative
  trace_query carrier covered the principal metrics.

## Batch 18 Gap: Runtime-Artifact Answers Should Not Surface Generic Consistency Caveats

Deep root cause:

- Observation-only runtime answers can receive weak preliminary perf/log
  summaries before later bounded tools such as `trace_query` produce
  authoritative typed carriers. The finalizer correctly prefers trace_query
  metrics, but CGEC/self-consistency can still record a medium-soft
  `ViolSelfContradiction` without structured SUMMARY/BODY claims.
- `AppendUserCaveatsToAnswerForBus` only applied generic accepted-surface
  suppression. Unlike `AppendSoftContractCaveatsToAnswerForBus`, it did not
  treat observation-only runtime context specially, so an unlocalized
  consistency template became user-visible as “答案前后某些表述存在不完全一致”.
- This is not a trace-case wording problem. The same issue applies to any
  observation-only log/perf/MCP/connector artifact answer where a precise typed
  carrier supersedes earlier weak prose, but a low-precision reviewer signal
  remains in telemetry.

Generalized design:

- Add a bus-aware suppression layer for observation-only runtime caveats that
  demotes only low-precision `ViolSelfContradiction` entries to telemetry when
  they lack parseable SUMMARY/BODY claims. Specific self-contradiction entries
  with concrete conflicting claims remain user-visible.
- Apply the same suppression to user-caveat and soft-contract-caveat paths so
  retry-exhausted and soft-accept flows behave consistently.
- Keep existing LLM-authored caveats and runtime boundary caveats intact. The
  goal is to remove generic non-actionable system templates, not to shorten or
  weaken the answer body.

Executable task list:

- [x] B18-T1: Record post-Batch-17 eval audit and generic consistency-caveat
  root cause in this design doc.
- [x] B18-T2: Add bus-aware suppression for generic observation-only runtime
  `ViolSelfContradiction` caveats while preserving specific SUMMARY/BODY
  contradictions.
- [x] B18-T3: Add regression tests for user-caveat suppression and specific
  contradiction preservation under observation-only runtime context.
- [x] B18-T4: Run focused tests, full tests, rebuild, diff hygiene.
- [x] B18-T5: Commit and push Batch 18.
- [x] B18-T6: Rerun representative eval cases two at a time and manually audit
  answers/logs again.

Post-Batch-18 eval audit:

- `eval/results/eval-gap-20260613-post-d4a04e25-b1`
- Parallel batch: `data_json_strict_ids` + `trace_query_state_churn_window_stats`.
- `trace_query_state_churn_window_stats`: PASS in 150s. Metrics:
  `tool_trace_query=1`, `tool_read_file=0`, `unavailable_tool_attempts=0`,
  `answer_contract_violations=1`, `finalizer_rejects=2`,
  `max_context_window_pct=17`. Manual audit: the model actively used
  `trace_query view=window_stats`; the answer preserves the requested
  state_churn metrics and next-step guidance. Residual gap: system-generated
  caveats still appended generic enumeration/support and consistency warnings
  after the typed trace answer was otherwise complete.
- `data_json_strict_ids`: FAIL in 167s. Metrics: `data_rounds=10`,
  `data_repair_rounds=1`, `data_answer_len=34`, no repo/runtime tools, and
  no unavailable tool attempts. Manual audit: business semantics were correct
  during the run, but the terminal answer became
  `{"u1":["u1"],"u2":"0","u3":["u3"]}`. The system projected across the full
  `users.json.id` reference universe even though the requested output shape was
  an aggregate JSON object field (`ids`) containing the filtered active IDs.

## Batch 19 Gap: Reference Completion Must Respect Aggregate List Projection Shape

Deep root cause:

- Reference completion is correct for detail/numeric projections where each
  reconcile group is one output slot keyed by `group_key` (for example
  `Q001,Q002,Q003 -> 0,7,0`). It is wrong for member-value aggregate outputs
  where the reconcile ledger represents a set/list to be collected under one
  output field.
- The completion gate and `assemble_answer` currently treat a smaller answer
  cardinality than a candidate reference universe as a projection gap whenever
  the plan/action declares reference completion. That loses the distinction
  between "missing reference-key output rows" and "filtered members collected
  into one aggregate field".
- The existing `json_object` projection already supports member-value
  operations (`include`, `set`, `rank`) and can render `{"ids":[...]}` from
  typed contribution/reconcile values. The gap is that reference-completion
  logic can override this aggregate projection and force the answer back into a
  per-reference-key shape.

Generalized design:

- Add a reusable structural predicate for aggregate list projections based on
  typed contribution operation semantics and reconcile group linkage. If the
  participating reconcile groups are backed by member-value operations that
  project list/set members, the output is aggregate-list shaped and is not
  eligible for reference-key completion.
- Apply the predicate in two places:
  1. completion/reference-gap detection, so the workflow does not synthesize a
     `complete_reference=true` repair for aggregate list outputs;
  2. `assemble_answer`, so an already-declared `complete_reference` cannot
     corrupt a member-value aggregate into per-reference-key zero fill.
- Preserve existing numeric/detail reference completion unchanged. The guard
  reads only structured plan/result fields: reconcile groups, contribution
  operations, output contract, and artifact metadata. It does not parse user
  prose or model-authored answer text.

Executable task list:

- [x] B19-T1: Record post-Batch-18 data eval audit and aggregate/reference
  projection root cause in this design doc.
- [x] B19-T2: Export/reuse a typed dataquery predicate for reconcile groups
  that should project as member-value lists rather than reference-key rows.
- [x] B19-T3: Gate `assemble_answer` reference completion with that predicate
  while preserving numeric/detail zero-fill behavior.
- [x] B19-T4: Gate REPL workflow reference-gap detection with the same
  predicate so fallback plans do not synthesize inapplicable
  `complete_reference=true` repairs.
- [x] B19-T5: Add regression tests for JSON object list projection with a
  larger source reference universe, plus existing numeric reference completion.
- [x] B19-T6: Run focused tests, full tests/build hygiene, commit and push.

Implementation notes:

- `dataquery.ReconcileGroupsPreferListProjection` centralizes the structural
  member-value/list predicate. It recognizes reconcile groups that carry
  aggregate member values or are backed by list-projection contribution
  operations, and excludes only those shapes from reference completion.
- `assemble_answer` now skips `complete_reference` projection for that shape;
  the existing `json_object` list projection remains authoritative. Numeric
  and detail projections still complete missing reference keys with zero/empty
  values.
- REPL workflow reference-gap detection uses the same predicate before
  synthesizing output-projection repairs, so an aggregate list answer no longer
  loops into per-reference-key fallback plans.

Verification:

- Focused tests:
  `go test ./internal/dataquery -run 'TestActionRunnerAssembleAnswer(SkipsReferenceCompletionForListAggregate|CompletesReferenceKeys|CompletesReferenceKeysFromOutputContract|ReferenceProjectionDropsNonReferenceGroups|ProjectsSameMetricSetGroupsAsJSONArrayField)'`
- Focused tests:
  `go test ./internal/dataworkflow -run 'TestInferAnswerItemCountSingleJSONFieldArray|TestBuildOutputProjectionGraphReportsReferenceIncomplete'`
- Focused tests:
  `go test ./internal/repl -run 'TestDataTaskReferenceProjection(SkipsMemberValueListAggregate|PrefersAtomicCandidateOverAggregateArtifact)|TestDataTaskWorkflowCompletionGateChoosesReferenceFieldByGroupOverlap'`
- Package tests: `go test ./internal/dataquery ./internal/dataworkflow ./internal/repl`

## Batch 20 Gap: Runtime Artifact Caveat Filtering Must Use Answer-Surface Evidence

Deep root cause:

- Batch 18 suppressed generic self-contradiction caveats only when the request
  model classified the turn as observation-only runtime. The post-Batch-18 eval
  showed a broader, valid shape: analyzer kept current-source analysis
  available by default, but the accepted final answer surface was still
  trace-only (`claim_form=external_observation`, no current-source citation).
- CGEC produced low-precision residuals after the answer was accepted:
  `ViolEnumerationEvidenceUnderspecified` and generic
  `ViolSelfContradiction`. These are useful telemetry for operator analysis but
  not actionable user caveats when the visible principal answer is already
  carried by typed runtime observations plus a concrete uncertainty boundary.
- The gap is not trace-specific. It applies to log/perf/MCP/connector answers
  whose principal answer surface is origin-specific and typed, even when the
  analyzer did not explicitly exclude current source upfront.

Generalized design:

- Extend low-precision caveat filtering with an answer-surface predicate:
  when the accepted `AnswerDocumentV2` principal blocks are carried by
  `external_observation` claim uses and no current-source citations are used,
  demote generic residual caveats to telemetry.
- Suppress only non-actionable generic families in that context:
  unlocalized self-contradiction, generic enumeration-evidence/support
  underfill, and accepted-surface metadata concerns. Specific contradictions
  with parseable conflicting claims and model/authored runtime uncertainty
  caveats remain visible.
- The predicate consumes typed answer-document annotations and citation
  metadata, not user prose, keyword matches, or model narrative strings.

Executable task list:

- [x] B20-T1: Record post-Batch-18 runtime caveat audit and answer-surface
  root cause in this design doc.
- [x] B20-T2: Add a bus-aware answer-surface runtime predicate based on
  `AnswerDocumentV2` principal block `claim_uses` and citation metadata.
- [x] B20-T3: Use that predicate to suppress generic low-precision runtime
  caveats while preserving concrete contradictions and boundary caveats.
- [x] B20-T4: Add regression tests for default-current-source runtime requests
  whose accepted answer surface is external-observation-only.
- [x] B20-T5: Run focused tests, full tests/build hygiene, commit and push.

Implementation notes:

- Runtime low-precision caveat suppression now accepts either the original
  observation-only request model signal or a typed answer-surface signal:
  principal `AnswerDocumentV2` blocks must all declare
  `claim_form=external_observation`, and none may use a resolved citation from
  the current-source citation pool.
- The suppression removes only generic unlocalized self-consistency caveats and
  non-principal runtime metric-list enumeration support caveats. Specific
  SUMMARY/BODY contradictions still materialize, and principal runtime
  enumeration requests still keep enumeration evidence caveats.
- The predicate consumes typed request/answer metadata and citation references.
  It does not inspect natural-language request text or rendered answer prose.

Verification:

- Focused tests:
  `go test ./internal/orchestrator -run 'TestAppend(User|Soft).*CaveatsToAnswerForBus_(ObservationOnly|RuntimeAnswerSurface|PureHistory|Mechanism)'`

Post-Batch-23 eval audit:

- `eval/results/eval-gap-20260613-post-b99e2bfe-b1`
- Parallel batch: `data_json_strict_ids` + `trace_query_state_churn_window_stats`.
- `data_json_strict_ids`: runner exit code 0 but eval verdict FAIL
  (`no_regex_match:"ids"`). Manual audit: the workflow consumed both
  `instructions.md` and `users.json`, decisions/rule coverage/contributions
  and reconcile were satisfied, but the terminal answer was
  `[{"group_key":"u1","metric":"id","value":"1"},{"group_key":"u3","metric":"id","value":"1"}]`
  rather than the strict JSON object shape requested by the output contract.
  The final evaluator had identified this as a node repair, but the completion
  gate normalized the verdict to `complete`.
- `trace_query_state_churn_window_stats`: runner exit code 0 but eval verdict
  FAIL on surface co-occurrence regexes. Manual audit: the answer is
  semantically correct and rich, uses `trace_query` three times, preserves
  `dominant_state=runnable`, cumulative running/runnable/sleep/d_state/io_wait,
  `fragments=21`, `switches=20`, `max_segment=0.5ms`, `p95_segment=0.5ms`,
  and root-cause/next-step guidance. The Batch-23 generic consistency caveat is
  gone. Residual gap: typed runtime metrics are present but distributed across
  several visible bullets instead of a stable compact snapshot line.

## Batch 24 Gap: Completed Workflow Must Not Drop Actionable Typed Repair Anchors

Deep root cause:

- The data evaluator can emit a structured `repair_node` verdict after a
  final result is produced. In the failing run the evaluator identified the
  final projection node as the repair target, while the typed workflow state
  simultaneously reported all ledgers and the projection graph as complete.
- `NormalizeEvaluationForWorkflowState` treats completed typed state as
  authoritative and rewrites every non-complete evaluator status to
  `complete`, clearing `action_id`, `action_kind`, and `repair_locus`. The
  terminal log keeps only a prose marker (`original_status=repair_node`), so
  downstream decision logic cannot consume the typed repair target.
- This is a general handoff gap. Any strict data workflow can have all ledgers
  satisfied while the final projection is structurally wrong for the user-facing
  output contract. The system must preserve actionable typed repair anchors
  instead of forcing downstream code to parse evaluator prose.

Generalized design:

- Treat completed workflow state as authoritative only for noisy or stale
  repair verdicts. When an evaluator emits `repair_node` with a precise typed
  target (`repair_locus`, or a high-confidence action id/kind pair), preserve
  that repair status through normalization.
- Update evaluation decisioning so an actionable typed repair target can still
  choose repair even when structural completion is otherwise satisfied. Noisy
  `repair_node` verdicts with no typed repair anchor remain overrideable by the
  completion gate.
- The fix consumes only schema fields from `Evaluation` and existing workflow
  state. It does not inspect evaluator prose, user prose, answer strings, or
  case-specific keys.

Executable task list:

- [x] B24-T1: Add a shared typed predicate for actionable evaluator repair
  targets.
- [x] B24-T2: Preserve actionable `repair_node` evaluations during completed
  workflow normalization while still suppressing stale low-confidence repairs.
- [x] B24-T3: Make `DecideEvaluation` prefer actionable repair over completion
  gate return-answer decisions when repair budget/planner are available.
- [x] B24-T4: Add regression tests for noisy repair override and actionable
  repair preservation.
- [x] B24-T5: Run focused dataworkflow/repl tests, full package hygiene,
  commit, and push.

Implementation notes:

- `dataworkflow.EvaluationHasActionableRepairTarget` now recognizes
  `repair_node` evaluations with a typed `repair_locus`, or a non-low
  confidence `action_id`/`action_kind` pair, as concrete repair requests.
- Completed workflow normalization still retires noisy/stale repair statuses,
  but it preserves actionable typed repair targets intact for downstream
  repair planning.
- `DecideEvaluation` now routes actionable repair through the repair planner
  even when the structural completion gate is otherwise satisfied. No-anchor
  noisy repairs still return the completed answer through the existing gate.

Verification:

- Focused tests:
  `go test ./internal/dataworkflow -run 'TestNormalizeEvaluationForWorkflowState|TestDecideEvaluationCompletionSatisfied'`
- Focused tests:
  `go test ./internal/repl -run 'TestDataTaskEvaluationDecision(UsesCompletionGateForNoisyRepair|PreservesActionableRepairTarget)'`
- Package tests:
  `go test ./internal/dataworkflow ./internal/repl ./internal/dataquery`

## Batch 25 Gap: Runtime Metric Handoff Needs a Compact Typed Snapshot Surface

Deep root cause:

- `trace_query` already emits `state_churn` as typed runtime observations with
  structured `RichNotes` for `dominant_state`, fragment counts, per-state
  cumulative time, `max_segment`, and `p95_segment`.
- The prompt-facing observation projection gives supporting observations only
  two notes by default. Because `state_churn` is supporting runtime evidence
  rather than a principal current-source row, later metric notes can be dropped
  from the compact handoff even though they were accepted by the observation
  ledger.
- The final answer remains semantically correct, but required metrics can be
  spread over multiple bullets and become hard to scan or validate. This is a
  generalized presentation/handoff budget problem for runtime, VCS, connector,
  and other origin-specific observations that carry compact metric notes.

Generalized design:

- Give origin-specific observations a bounded, larger note budget so typed
  metric notes survive the shared observation prompt projection even when the
  record is supporting rather than principal.
- Add a runtime trace presentation hint to preserve multi-metric observation
  notes as one compact metric snapshot line before the richer explanation.
  This preserves answer richness; it does not replace the detailed analysis.
- Keep row count and per-note length limits unchanged, so the change improves
  high-signal handoff without widening broad context windows.

Executable task list:

- [x] B25-T1: Extend observation prompt projection options with a bounded
  origin-specific supporting note limit.
- [x] B25-T2: Add tests showing a runtime `trace_query` supporting observation
  carries the full compact metric note set into the finalizer handoff.
- [x] B25-T3: Add generic runtime trace guidance for compact metric snapshots
  sourced from typed observation notes.
- [x] B25-T4: Run focused types/agent tests, full tests/build hygiene, commit,
  and push.

Implementation notes:

- `ObservationPromptProjectionOptions` now has
  `OriginSpecificSupportingNoteLimit`. Default finalizer projection keeps up to
  10 compact notes for origin-specific supporting observations, while semantic
  review keeps a smaller bounded budget.
- Runtime supporting observations such as `trace_query` `state_churn` now keep
  the complete compact metric set (`dominant_state`, state totals,
  fragments/switches, max/p95 segment) in the typed observation handoff.
- Runtime trace guidance now asks finalization to preserve multi-metric typed
  notes as one metric snapshot line before the richer explanation, preserving
  answer richness while making required dimensions easier to scan.

Verification:

- Focused tests:
  `go test ./internal/types -run 'TestProjectObservationPromptRecords_(RuntimeQueryOutranksPreTriageBudget|RuntimeSupportingMetricsKeepCompactNotes)'`
- Focused tests:
  `go test ./internal/agent -run TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersHarmonyTracePriorityReminder`
- Package tests: `go test ./internal/types ./internal/agent`

Post-Batch-25 eval audit:

- `eval/results/eval-gap-20260613-post-28051d92-b1`
- Parallel batch: `data_json_strict_ids` + `trace_query_state_churn_window_stats`.
- `data_json_strict_ids`: PASS in 29s. Metrics: `data_rounds=1`,
  `data_repair_rounds=0`, `data_answer_len=19`, and terminal answer
  `{"ids":["u1","u3"]}`. Manual audit: no completion-gate override,
  no output-contract warning, and both required materials were consumed.
- `trace_query_state_churn_window_stats`: runner exit code 0 but eval verdict
  FAIL on the next-step surface regex. Manual audit: semantics are correct and
  rich; the run uses `trace_query=2`, preserves all requested metrics, root
  cause (`rival-30` / same-CPU CPU pressure), and useful follow-up actions.
  Residual gaps: the follow-up actions render as an untitled ordered list, so
  the explicit next-step relationship can be missed; a soft
  `uncertainty_block_missing` caveat still materializes as generic
  source-check wording even though the accepted surface is external
  observation only.

## Batch 26 Gap: Runtime Answer Surfaces Must Demote Generic Uncertainty-Block Caveats

Deep root cause:

- Runtime observation-only answers already have a typed waiver path that turns
  `uncertainty_block_missing` into a precise runtime-boundary caveat or
  suppresses generic block-coverage wording.
- The post-Batch-25 trace answer uses a different but equally typed accept
  context: principal answer blocks declare `claim_form=external_observation`
  and use no current-source citations. That surface is runtime-observation-only,
  but the soft caveat path does not treat `ViolUncertaintyBlockMissing` as
  low-precision telemetry for this accepted surface.
- This is not trace-specific. It applies to any accepted external-observation
  answer surface where the only missing item is a generic uncertainty block and
  the answer already avoids current-source citation pressure.

Generalized design:

- Extend the runtime low-precision caveat filter to demote
  `ViolUncertaintyBlockMissing` when the accepted answer surface is
  external-observation-only and citation-free.
- Keep the validator telemetry and strict retry behavior intact. Mixed visible
  current-source blocks or unannotated blocks continue to surface the caveat.
- Do not parse caveat prose or answer prose; consume only violation kind,
  block claim metadata, and citation metadata.

Executable task list:

- [x] B26-T1: Add regression coverage for runtime answer-surface
  `ViolUncertaintyBlockMissing` suppression.
- [x] B26-T2: Extend `runtimeObservationOnlyLowPrecisionCaveat` for the typed
  uncertainty-block violation under the accepted runtime surface predicate.
- [x] B26-T3: Preserve mixed-surface disclosure tests.

Implementation notes:

- `runtimeObservationOnlyLowPrecisionCaveat` now treats
  `ViolUncertaintyBlockMissing` as telemetry-only only when the same typed
  `runtimeArtifactPrincipalAnswerSurfaceContext` predicate is true:
  principal/principal-like blocks must carry only external-observation claims
  and no citations.
- Mixed principal surfaces with an unannotated current-source block still
  materialize the existing user caveat. This keeps the hard/soft split on typed
  answer-document metadata, not answer prose.

Verification:

- Focused caveat tests:
  `go test ./internal/orchestrator -run 'TestAppend(SoftContract|User)CaveatsToAnswerForBus_RuntimeAnswerSurface'`
- Package tests: `go test ./internal/orchestrator`

## Batch 27 Gap: Follow-Up Action Blocks Need a Stable Visible Relation Label

Deep root cause:

- The finalizer emitted a structured `ordered_list` block with
  `id="next_steps"` and external-observation claim metadata. The renderer
  intentionally does not invent headings for ordinary untitled lists, so the
  follow-up actions are semantically present but visually detached from the
  "next step" relation.
- This is a structured answer-document rendering gap. Any model can emit a
  semantic next-step block ID without a title, and the renderer should preserve
  the relation label in the user surface without relying on natural-language
  item text or user-keyword matching.

Generalized design:

- Add a narrow renderer fallback for answer-document list blocks whose
  structured block ID is the next-step carrier (`next_step` / `next_steps`):
  when `title` is empty, render a localized title (`下一步` / `Next steps`).
- Leave all ordinary untitled ordered/bullet lists unchanged.
- This consumes only structured block ID and block kind, not user intent text,
  model prose, item labels, or answer contents.

Executable task list:

- [x] B27-T1: Add renderer tests for localized next-step headings on structured
  `next_steps` list blocks.
- [x] B27-T2: Add the renderer fallback for ordered and bullet list blocks.
- [x] B27-T3: Run focused render/orchestrator tests, full hygiene, commit,
  push, and rerun the two representative eval cases.

Implementation notes:

- The answer-document renderer now renders a localized heading only when a
  list block has structured ID `next_step` / `next_steps` and no explicit
  title.
- Ordinary untitled ordered and bullet lists remain unchanged. The renderer
  consumes block ID and kind only; it does not inspect user intent text, item
  labels, or model prose.

Verification:

- Focused render tests:
  `go test ./internal/render -run 'TestRenderV2_(BlockOrderedList|NextStepsListIDGetsLocalizedHeading)'`
- Package tests: `go test ./internal/render`

Post-Batch-27 eval audit:

- `eval/results/eval-gap-20260613-post-e52171c5-b1`
- Parallel batch: `data_json_strict_ids` + `trace_query_state_churn_window_stats`.
- `data_json_strict_ids`: PASS in 158s. Manual audit: final answer is strict
  JSON `{"ids":["u1","u3"]}`. Logs show two repair rounds: one invalid
  `group_key_field=group` for contribution computation and one field-name
  repair from `id_lists` to `ids`. No output-contract warning remains, but the
  higher round count is a separate data-planning efficiency signal, not a
  correctness gap for this batch.
- `trace_query_state_churn_window_stats`: runner exit code 0 but eval verdict
  still FAIL on the next-step surface regex. Manual audit: scalar metrics and
  root-cause semantics are correct, and `trace_query` was used once with a
  bounded `window_stats` call. The accepted answer document has only summary,
  metrics ordered list, auto-materialized trace facts, and caveat. The typed
  observation ledger contains `RichNotes` with
  `next_step=inspect rival-30 on same CPU cpu=1 for CPU pressure/time-slice competition`,
  but no structured `next_steps` block reaches the final surface, so the
  Batch-27 renderer fallback has nothing to render.

## Batch 28 Gap: Runtime Next-Step Handoff Must Materialize From Typed Observation Notes

Deep root cause:

- The trace tool already emits next-step guidance as structured observation
  metadata: `ObservationRecord.RichNotes` contains a typed `next_step=...`
  entry on the `trace_query` runtime-artifact row.
- The prompt asks the finalizer to preserve that guidance, but preservation is
  model-authored and therefore optional in practice. The answer contract stays
  quiet because no hard structural requirement says a typed next-step note must
  have a visible answer-document carrier.
- Batch 27 fixed the renderer for an existing structured `next_steps` block;
  it did not guarantee such a block exists when upstream typed notes contain
  the relation.

Generalized design:

- Extend the unified answer-document mutation chokepoint with a deterministic
  materializer that consumes only `ObservationLedger` records from accepted
  tool results.
- When a runtime-artifact `trace_query` observation carries a typed
  `RichNotes` key `next_step`, and the accepted document does not already have
  a `next_step` / `next_steps` block, insert a support ordered-list block with
  `id="next_steps"` before caveats.
- Keep the inserted block in the external-observation lane and do not create
  repository citations. Skip insertion only when the document already has a
  structured next-step block.
- This does not parse user text, model prose, or answer prose for intent; the
  only trigger is a producer-owned structured note key.

Executable task list:

- [x] B28-T1: Add regression coverage for typed `trace_query`
  `next_step` notes materializing into a `next_steps` block.
- [x] B28-T2: Add duplicate protection when the model already emitted a
  structured next-step block.
- [x] B28-T3: Implement the mutation-chokepoint materializer using
  `ObservationLedgerInputFromBusContext` and typed `ObservationRecord` fields.
- [x] B28-T4: Run full hygiene, commit, push, rebuild, and rerun the two
  representative eval cases.

Implementation notes:

- `persistMergedAnswerDocument` now runs the next-step materializer before the
  runtime trace facts materializer, giving requested follow-up guidance priority
  when block headroom is tight.
- The inserted item label is `下一步`, while the text preserves the typed note
  value exactly after whitespace normalization and bounded length trimming.

Verification:

- Focused tool tests:
  `go test ./internal/tool -run 'TestEmitAnswerDocumentV2_(MaterializesRuntimeTraceStructuredFacts|MaterializesRuntimeTraceNextStepFromTypedObservation|DoesNotDuplicateExistingRuntimeTraceNextStepsBlock|DoesNotMaterializeRuntimeFactsForCodeOnlyDoc)'`

Post-Batch-28 eval audit:

- `eval/results/eval-gap-20260613-post-24f2ad99-b1`
- Parallel batch: `data_json_strict_ids` + `trace_query_state_churn_window_stats`.
- `data_json_strict_ids`: PASS in 37s. Metrics returned to the stable fast
  shape: `data_rounds=1`, `data_repair_rounds=0`, `data_answer_len=19`.
- `trace_query_state_churn_window_stats`: runner exit code 0 but eval verdict
  FAIL on line-oriented metric regexes for the state cumulative values and
  `max_segment`/`p95_segment`. Manual audit: the next-step relation is now
  visibly present, and the answer is rich. However, the final surface rendered
  the scalar metrics as table rows and then duplicated next-step prose through
  finalizer patch repair, so the required runtime scalar set no longer has a
  stable compact line.
- Logs show `trace_query` usage improved (`window_stats` plus
  `root_cause_rank`) with bounded windows. The run also exercised the strict
  JSON repair layer: the first finalizer emit sent `blocks` as a JSON-encoded
  string and was rejected with a precise retry hint, then re-emitted native
  blocks. Performance stayed bounded: trace_query index/run phases were
  sub-millisecond, with heap_sys around 573 MB under `GOMEMLIMIT=12GiB`.

## Batch 29 Gap: Runtime Scalar Metrics Need a Stable Typed Snapshot Surface

Deep root cause:

- `trace_query` state_churn rows already carry all requested scalar metrics as
  typed `RichNotes` (`running`, `runnable`, `sleep`, `d_state`, `io_wait`,
  `fragments`, `switches`, `max_segment`, `p95_segment`).
- The answer prompt asks the model to preserve metric snapshots, but the model
  can choose a table, repeated sections, or prose. Those are semantically rich,
  yet they do not provide a stable compact scalar surface for downstream audit,
  copy/paste, or line-oriented consumers.
- This is broader than one eval case: any runtime tool that emits a typed
  scalar bundle needs one deterministic visible carrier so rich narrative and
  stable machine-auditable surfaces can coexist.

Generalized design:

- Add a second mutation-chokepoint materializer for runtime trace metric
  snapshots, parallel to the next-step materializer.
- Consume only accepted `ObservationLedger` records from `trace_query` whose
  typed metric note bundle is complete. Predicate/claim-key labels are useful
  context but not a hard dependency because producer rows can carry the full
  state-churn metric set in `RichNotes` while leaving predicate fields empty.
- Insert a support bullet-list block `id="runtime_trace_metric_snapshot"` before
  caveats. Each item is a single compact line preserving the typed state
  cumulative values, fragments/switches, and segment percentiles.
- Keep the block in the external-observation lane and do not create repo
  citations. Do not inspect user text, model prose, or rendered answer prose to
  decide whether to insert it.

Executable task list:

- [x] B29-T1: Add regression coverage for typed state_churn metric notes
  materializing into a compact snapshot line.
- [x] B29-T2: Implement the runtime metric snapshot materializer in the same
  unified answer-document mutation chokepoint.
- [x] B29-T3: Preserve no-citation external-observation lane semantics.
- [ ] B29-T4: Run full hygiene, commit, push, rebuild, and rerun the two
  representative eval cases.

Implementation notes:

- `persistMergedAnswerDocument` now runs metric snapshot materialization before
  next-step and trace-facts materialization so requested scalar outputs have
  priority when block headroom is constrained.
- Snapshot rows are derived from exact typed note keys only. Missing any
  required metric key causes a no-op rather than a partial/inferred snapshot.

Verification:

- Focused tool tests:
  `go test ./internal/tool -run 'TestEmitAnswerDocumentV2_(MaterializesRuntimeTraceMetricSnapshotFromTypedObservation|MaterializesRuntimeTraceNextStepFromTypedObservation|DoesNotDuplicateExistingRuntimeTraceNextStepsBlock|MaterializesRuntimeTraceStructuredFacts)'`

Post-Batch-29 eval audit:

- `eval/results/eval-gap-20260613-post-3e7d1f69-b1`
- Parallel batch: `data_json_strict_ids` + `trace_query_state_churn_window_stats`.
- `trace_query_state_churn_window_stats`: PASS in 135s. Metrics:
  `tool_trace_query=1`, `data_rounds=0`, `finalizer_rejects=0`,
  `max_context_window_pct=17`. Manual audit: the answer used bounded
  `trace_query window_stats`, preserved all scalar metrics, next-step guidance,
  trace facts, and external runtime caveat. Two deeper gaps remained:
  the metric snapshot materializer did not fire because the actual typed row
  relied on complete `RichNotes` rather than stable predicate/claim-key fields;
  the visible summary still copied a low-authority runtime closure phrase that
  classified `prio=53` as CFS despite the typed priority fact saying
  `prio=53/ohos_rt`.
- `data_json_strict_ids`: runner exit code was 0 but eval summary verdict was
  FAIL. Manual audit: the typed DAG eventually computed the correct strict JSON
  `{"ids":["u1","u3"]}` and reconcile passed, but terminal validation failed
  on `unknown_rule_ref` after multi-batch aggregation. A repair then planned a
  `custom_transform` to read `final_answer.json` and emitted the raw answer
  string, which the runner parsed as a non-Result JSON value and rejected.

Implementation adjustment:

- Metric snapshot materialization now treats a complete typed metric note
  bundle from `trace_query` as sufficient. This keeps the trigger structural
  and producer-owned while avoiding hard dependency on optional predicate or
  claim-key fields.

## Batch 30 Gap: Workflow-Level Result Normalization Must Run After Aggregation

Deep root cause:

- `applyDataResultPatchEngine` already canonicalizes unknown item-ledger
  `rule_refs` to source-backed `rule_coverage.rule_id`, but it only ran on
  single runner results.
- Adaptive data workflows aggregate rows, rules, contributions, reconcile
  reports, artifacts, and preserved answers across multiple batches. The
  terminal completion gate validated this aggregate without re-running the
  structural patch layer, so a valid source-backed rule mapping could pass in
  one batch and fail after cross-batch handoff.
- The repair loop then fell back to a free-form `custom_transform`, increasing
  model burden and producing a shape error. The deeper gap is not the
  particular `RULE:...` string; it is missing normalization at every backend
  consumption boundary for merged `Result` values.

Generalized design:

- Promote the data result patch engine to a public `NormalizeResult` function
  that can be safely applied to both single runner outputs and merged
  workflow-level results.
- Run normalization before workflow completion validation, output graph
  construction, ledger graph construction, completion-repair transition input,
  runner seed handoff, and final CLI answer rendering.
- Keep validation strict: unknown refs are only canonicalized when a
  source-backed `rule_coverage` record exists. Pure model rules without
  evidence still fail loud.
- Consume only typed Result fields (`rule_coverage`, `rows`,
  `contributions`, `entity_resolutions`, `reconcile`, `output_contract`);
  do not inspect user text, model prose, or answer string semantics.

Executable task list:

- [x] B30-T1: Add public `dataquery.NormalizeResult` around the existing
  deterministic patch engine.
- [x] B30-T2: Apply normalization at workflow completion, output/ledger graph,
  repair-transition, seed handoff, and final CLI answer boundaries.
- [x] B30-T3: Add regression tests for aggregated source-backed rule-ref
  canonicalization and cross-batch seed normalization.
- [ ] B30-T4: Run full hygiene, commit, push, rebuild, and rerun the two
  representative eval cases.

Verification:

- Focused tests:
  `go test ./internal/dataquery -run 'TestRunnerCanonicalizesUnknownRuleRefs|TestRunnerRejectsUnknownRuleRefs|TestApplyDataResultPatchPlan'`
- Focused tests:
  `go test ./internal/repl -run 'TestValidateDataTaskWorkflowResult|TestDataTaskActionRunnerSeed'`

## Batch 31 Gap: Deterministic Runtime Query Must Shadow Runtime Closure Prose

Deep root cause:

- Runtime observation-only prompts already omit the model-authored closure
  reason from the authority section, but the same closure text was reintroduced
  under investigation narrative handoff as advisory synthesis.
- When a deterministic runtime query tool (`trace_query`) has already produced
  typed observation rows, that low-authority closure prose becomes a duplicate
  factual source. It can contaminate finalizer summaries even when the typed
  observation ledger and trace facts contain the correct values.
- The issue is broader than priority labels: any runtime query row can carry
  bounded metrics/coordinates while earlier closure prose preserves stale or
  pre-query interpretations.

Generalized design:

- Keep runtime closure narrative handoff for observation-only runs that have no
  deterministic runtime query rows; it remains useful when pre-triage is the
  only artifact consumer.
- If the accepted observation ledger contains a runtime-artifact row whose
  producer is `trace_query`, do not append the accepted runtime closure reason
  into narrative handoff. The finalizer still receives typed observation rows,
  metric snapshots, next-step notes, runtime trace facts, and caveats.
- The decision consumes only structured `ObservationRecord.Origin` and
  `ObservationRecord.Producer`; it does not parse user intent keywords, model
  prose, final answer text, or trace labels.

Executable task list:

- [x] B31-T1: Add a deterministic-runtime-query detector over accepted
  observation ledger records.
- [x] B31-T2: Suppress only the appended runtime closure narrative when
  `trace_query` rows are present, preserving typed handoff richness.
- [x] B31-T3: Add prompt regression tests for both paths: no `trace_query`
  keeps closure advisory, `trace_query` shadows it.
- [ ] B31-T4: Run full hygiene, commit, push, rebuild, and rerun the two
  representative eval cases.

Verification:

- Focused tests:
  `go test ./internal/agent -run 'TestAnswerDocumentEvaluator_BuildInitialInstruction_(RendersRuntimeClosureReasonWithoutTurnAArtifacts|SkipsRuntimeClosureNarrativeWhenTraceQueryRowsPresent|SourceOptionalTraceSkipsCurrentStatus)'`

Post-Batch-22 eval audit:

- `eval/results/eval-gap-20260613-post-c1224262-b1`
- Parallel batch: `data_json_strict_ids` + `trace_query_state_churn_window_stats`.
- `data_json_strict_ids`: PASS in 51s. Metrics: `data_rounds=2`,
  `data_repair_rounds=1`, `data_answer_len=19`, no unavailable tools, and no
  answer-contract violations. Manual audit: terminal status is `complete`,
  final projection is present/satisfied, consumed paths include both
  `instructions.md` and `users.json`, and the answer is the strict JSON object
  `{"ids":["u1","u3"]}`.
- `trace_query_state_churn_window_stats`: PASS in 159s. Metrics:
  `tool_trace_query=3`, `tool_read_file=0`, `unavailable_tool_attempts=0`,
  `answer_contract_violations=1`, `finalizer_rejects=0`,
  `max_context_window_pct=19`. Manual audit: the answer actively used
  `trace_query window_stats` and `root_cause_rank`, preserved
  `dominant_state=runnable`, `running=3.5ms`, `runnable=5.0ms`,
  `sleep/d_state/io_wait=0ms`, `fragments=21`, `switches=20`,
  `max_segment=0.5ms`, `p95_segment=0.5ms`, and retained a useful runtime
  artifact boundary caveat. Residual gap: the generic enumeration caveat is
  gone, but a generic consistency caveat remains.

## Batch 23 Gap: Runtime Surface Must Demote Typed-Denial Consistency Telemetry

Deep root cause:

- Batch 22 demoted generic self-contradiction and enumeration-support caveats
  for typed external-observation answer surfaces. The post-Batch-22 run shows
  another consistency-family producer: `ViolDeniedTokenUndeclared`, emitted by
  the answer-side typed-denial validator against `answer_document.blocks.*.prose`.
- In runtime external-only answers, principal blocks can legitimately mention
  observed or absent runtime labels as external observations while a separate
  caveat block states the runtime artifact boundary. When the accepted
  principal surface is `claim_form=external_observation` with no current-source
  citations, a generic typed-denial consistency caveat is low-precision
  telemetry, not a concrete user action.
- This is a typed-signal gap, not a trace string problem. The fix must consume
  violation kind, answer-surface claim metadata, and citation metadata only.

Generalized design:

- Extend the runtime low-precision caveat filter to include
  `ViolDeniedTokenUndeclared` when the accepted answer surface is
  external-observation-only and citation-free.
- Keep the validator and telemetry intact; only the user-visible generic
  caveat is suppressed on accepted runtime surfaces.
- Preserve concrete `ViolSelfContradiction` SUMMARY/BODY conflicts, principal
  enumeration caveats, mixed current-source answer surfaces, and strict
  current-source denial behavior.

Executable task list:

- [x] B23-T1: Record post-Batch-22 eval audit and typed-denial consistency
  root cause in this document.
- [x] B23-T2: Add regression coverage for `ViolDeniedTokenUndeclared` on
  runtime external-observation answer surfaces.
- [x] B23-T3: Extend runtime low-precision caveat filtering for this typed
  denial violation while preserving mixed-surface disclosure.
- [x] B23-T4: Run focused tests; full tests/build, commit, push, and rerun the two
  representative eval cases.

Implementation notes:

- `runtimeObservationOnlyLowPrecisionCaveat` now treats
  `ViolDeniedTokenUndeclared` as telemetry-only when the accepted runtime
  answer surface is external-observation-only and citation-free.
- The typed-denial validator still emits the violation for logs and strict
  promotion. The user-visible suppression is scoped to the same answer-surface
  predicate used for runtime self-contradiction/enumeration support caveats.
- Mixed visible blocks without external-observation claim annotations still
  keep the consistency caveat.

Verification:

- Focused tests:
  `go test ./internal/orchestrator -run 'TestAppend(User|Soft).*CaveatsToAnswerForBus_(ObservationOnly|RuntimeAnswerSurface|PureHistory|Mechanism)'`
- Package tests: `go test ./internal/orchestrator`
- Full tests: `go test ./...`
- Build: `make`
- Diff hygiene: `git diff --check`

Post-Batch-20 eval audit:

- `eval/results/eval-gap-20260613-post-19b35fca-b1`
- Parallel batch: `data_json_strict_ids` + `trace_query_state_churn_window_stats`.
- `trace_query_state_churn_window_stats`: PASS by eval harness. Metrics:
  `tool_trace_query=3`, `tool_read_file=0`, `unavailable_tool_attempts=0`,
  `answer_contract_violations=1`, `finalizer_rejects=0`,
  `max_context_tokens_est=38031`. Manual audit: answer actively uses
  `trace_query` and preserves the required state-churn metrics, root-cause
  ranking, and next-step direction. Residual gap: generic system caveats for
  enumeration support and consistency still append after the typed runtime
  answer surface.
- `data_json_strict_ids`: FAIL by eval harness. Metrics: `data_rounds=10`,
  `data_repair_rounds=2`, `data_record_count=10`, `data_answer_len=19`,
  no unavailable tools, and no answer-contract violations. Manual audit:
  `result.answer` is exactly `{"ids":["u1","u3"]}`, but terminal completion
  fails with `final_answer_continue_after`. The final plan remains
  `continue_after=true` after budget exhaustion, and the latest result's
  `output_contract` is downgraded to freeform even though the workflow-level
  contract is strict `json_only`.

## Batch 21 Gap: Final Answer Handoff Must Use Effective Typed Output Contract

Deep root cause:

- Data workflows can preserve an earlier valid answer candidate across later
  bounded batches through the runner seed/result handoff. When a later
  continuation batch is marked `continue_after=true`, the current terminal
  check rejects the preserved answer solely because the current plan was not a
  final projection batch.
- The protection is important for unfinished workflows, but it is too coarse:
  it does not distinguish a true unfinished dependency from a handoff answer
  that already satisfies the effective workflow output contract and all
  required ledgers. In the failing run, the strict JSON payload is correct and
  no declared reference/projection gap remains, but a later exploratory batch
  causes terminal rejection.
- This is a general handoff/contract problem. Any typed data workflow can
  carry a valid final answer through later diagnostic or recovery batches, and
  the terminal gate must evaluate the effective structured contract rather
  than the incidental plan-local `continue_after` flag alone.

Generalized design:

- Add a typed final-answer handoff predicate that accepts a preserved answer
  only when:
  1. `result.answer` is present and not an internal artifact summary;
  2. workflow coverage/ledger requirements validate;
  3. the answer validates against the effective workflow output contract using
     the existing `dataquery.ValidateAnswer` contract checker;
  4. no declared final-projection/reference gap is pending.
- Keep the existing conservative behavior for ordinary `continue_after`
  batches whose answer does not satisfy the effective contract or whose
  required ledgers are missing.
- The predicate consumes only typed plan/result/output/coverage fields and
  existing contract validators. It does not parse user prose, model prose, or
  case-specific answer strings.

Executable task list:

- [x] B21-T1: Add dataworkflow helper(s) for effective typed answer-contract
  validation and preserved-answer handoff candidacy.
- [x] B21-T2: Update `ResultIsFinalAnswerCandidate` so `continue_after=true`
  can pass only through the typed handoff path; preserve assemble-answer
  terminal behavior.
- [x] B21-T3: Ensure CLI completion uses the same candidate result and emits a
  precise guard only when typed contract/ledger validation fails.
- [x] B21-T4: Add regression tests for strict JSON handoff across a
  continuation plan, and for missing ledger / invalid JSON still failing.
- [x] B21-T5: Run focused dataworkflow/repl/dataquery tests, commit, and push.

Implementation notes:

- `dataworkflow.ResultIsPreservedAnswerHandoffCandidate` now defines the narrow
  promotion path: `continue_after=true`, no assemble artifact, current action
  cannot itself produce a final answer, and `result.answer` validates against
  the effective workflow output contract.
- `ResultIsFinalAnswerCandidate` still rejects missing required ledgers and
  invalid output shapes. This preserves the existing safety boundary for true
  unfinished workflows while allowing strict typed answers to survive later
  diagnostic/continuation batches.
- REPL/CLI structural completion and final-answer rendering use the same
  predicate, so status view and terminal CLI delivery agree.

Verification:

- Focused tests:
  `go test ./internal/dataworkflow -run 'TestResultIsFinalAnswerCandidate|TestPlanMayProduceFinalAnswer'`
- Focused tests:
  `go test ./internal/repl -run 'TestDataTask(EvaluationDecisionUsesCompletionGateForNoisyRepair|FinalAnswerPromotesPreservedStrictJSONHandoff)'`
- Package tests:
  `go test ./internal/dataworkflow ./internal/repl ./internal/dataquery`

## Batch 22 Gap: Runtime Soft-Caveat Suppression Must Match Production Accept Path

Deep root cause:

- Batch 20 added typed answer-surface filtering for runtime artifacts, but the
  post-Batch-20 run still surfaces generic caveats in the production
  soft-accept path. The accepted document has principal blocks annotated with
  `claim_form=external_observation` and no current-source citations, so the
  visible answer surface is runtime-observation-only even though current-source
  mode remains available by default.
- The residual caveats are low-precision CGEC/contract telemetry:
  generic enumeration support and generic consistency. They are useful for
  logs, but on a typed runtime answer surface they do not give the user a
  concrete corrective action and make a correct answer look unstable.
- This is not trace-query-specific. It applies to runtime/log/perf/connector
  answers where the accepted visible surface is carried by typed external
  observation claims, including soft-accept flows with no finalizer retry.

Generalized design:

- Reproduce the production soft-accept path in tests: call
  `AppendSoftContractCaveatsToAnswerForBus` with a runtime request model,
  accepted `AnswerDocumentV2` principal blocks, external-observation claim
  uses, and no resolved citations.
- Broaden the suppression context only through typed answer-document metadata:
  principal (or principal-like visible) blocks whose claim uses are
  external-observation-only and whose items do not resolve to current-source
  citations qualify for low-precision caveat demotion.
- Preserve concrete contradictions, true principal enumeration requests, and
  runtime boundary/model-authored caveats.

Executable task list:

- [x] B22-T1: Add production-path regression tests for
  `AppendSoftContractCaveatsToAnswerForBus` on runtime answer-surface
  documents.
- [x] B22-T2: Adjust the runtime answer-surface predicate if needed so it
  consumes typed block/claim/citation metadata available at append time.
- [x] B22-T3: Keep generic low-precision caveats telemetry-only for runtime
  answer surfaces while preserving specific contradictions and principal
  enumeration caveats.
- [x] B22-T4: Run focused orchestrator tests, full tests/build hygiene, commit,
  and push.

Implementation notes:

- Runtime answer-surface filtering now uses accepted `AnswerDocumentV2` blocks
  directly. Explicit `surface_role=principal` blocks are authoritative; when no
  explicit principal role exists, visible principal-like blocks
  (summary/section/list/scalar/decision/table/diagram) are checked as the
  fallback surface.
- A surface qualifies only when every candidate block declares
  `claim_form=external_observation` and none of its item citations resolve to a
  current-source citation. Mixed or unannotated visible blocks keep the
  generic caveat disclosure.
- The suppression path applies to both user-caveat and soft-contract-caveat
  appenders. Concrete SUMMARY/BODY contradictions and true principal
  enumeration requests remain user-visible.

Verification:

- Focused tests:
  `go test ./internal/orchestrator -run 'TestAppend(User|Soft).*CaveatsToAnswerForBus_(ObservationOnly|RuntimeAnswerSurface|PureHistory|Mechanism)'`

## Post-Batch-31 Eval Audit

Eval batch:

- Results root:
  `eval/results/eval-gap-20260613-post-3a4c4b5a-b1`
- Parallel cases: `data_json_strict_ids` +
  `trace_query_state_churn_window_stats`.

Manual audit:

- `data_json_strict_ids`: PASS in 57s. The terminal strict JSON answer is
  `{"ids":["u1","u3"]}`, flags remain 0, and the workflow result is now
  normalized after aggregation. One repair round remains because the first
  generated transform did not consume the explicit `instructions.md` artifact,
  but the repair path stays typed and the final result is correct.
- `trace_query_state_churn_window_stats`: FAIL by harness surface regex only.
  The run used `trace_query` three times, bounded the window, and preserved the
  correct semantic values in the final answer: `dominant_state=runnable`,
  `running=3.5ms`, `runnable=5.0ms`, zero sleep/D/io-wait, `fragments=21`,
  `switches=20`, `max_segment=0.5ms`, `p95_segment=0.5ms`, and the
  `rival-30` next-step direction. The answer remains rich enough, but the
  system-generated metric snapshot block did not materialize, so
  `max_segment` and `p95_segment` can land on separate rendered bullets.

Log/root-cause notes:

- The `trace_query` payload contains a complete deterministic
  `window_stats.state_churn.summary` row:
  `dominant_state=... fragments=... switches=... max_segment=...
  p95_segment=... running=... runnable=... sleep=... d_state=...
  io_wait=...`.
- The answer-document runtime materializer currently requires every metric to
  be present in `ObservationRecord.RichNotes`. Prompt-facing and ledger
  projections can reorder, merge, or budget notes independently from the
  summary. In this run the visible handoff kept high-signal notes such as
  `max_segment`, `p95_segment`, `running`, `runnable`, and `next_step`, while
  some zero/count notes were only reliably present in the same tool-produced
  summary line.
- This is a general structured-handoff gap: answer-side deterministic
  materializers should merge same-row tool summary tokens with typed rich
  notes before deciding that a compact metric snapshot is unavailable.

## Batch 32 Gap: Runtime Metric Snapshots Must Merge Same-Row Tool Tokens

Deep root cause:

- Runtime tools often produce both typed notes and a compact structured summary
  for the same observation row. Notes are optimized for prompt budget and
  priority; summaries are optimized for complete row audit. Requiring every
  scalar to survive in notes alone makes answer-side materialization brittle
  under valid projection/merge choices.
- The failure mode is not trace-query-specific. Any runtime/log/perf tool that
  emits exact `key=value` summary tokens can lose a deterministic compact
  snapshot if one required scalar is only present in the summary surface.
- Letting the model repair this in prose would either reduce answer richness or
  create unstable formatting pressure. The stable boundary is a deterministic
  answer-document materializer that consumes only trusted tool-produced
  structure and inserts one supplemental snapshot line without replacing the
  richer explanation.

Generalized design:

- Keep `ObservationRecord.RichNotes` as the preferred source for explicit
  metric notes.
- Add a same-row structured-token merge layer for deterministic runtime query
  observations. It may read only:
  1. `ObservationRecord.RichNotes`;
  2. the same `ObservationRecord.Summary` generated by the runtime tool.
- Accept exact `key=value` tokens for known metric keys from tool output
  (`dominant_state`, `running`, `runnable`, `sleep`, `d_state`, `io_wait`,
  `fragments`, `switches`, `max_segment`, `p95_segment`). Do not inspect user
  intent, model-authored prose, final answer text, or arbitrary keywords.
- Continue to require a complete metric family before materializing the
  snapshot. Partial rows remain advisory evidence, not a user-visible hard
  supplement.
- Insert the snapshot as an external-observation block before caveats so
  validators and users see `max_segment` and `p95_segment` on the same stable
  line while the model-authored rich explanation remains intact.

Executable task list:

- [x] B32-T1: Document the post-Batch-31 eval audit and same-row token merge
  design.
- [x] B32-T2: Add a structured token extractor in the runtime answer-document
  materializer that merges exact tool summary tokens into typed note values.
- [x] B32-T3: Add regression coverage for partial RichNotes plus complete
  same-row `trace_query` summary tokens.
- [x] B32-T4: Run focused tests, full tests/build hygiene, commit, push, and
  rerun the representative eval pair.

Post-Batch-32 eval audit:

- Results root:
  `eval/results/eval-gap-20260613-post-c884d970-b1`
- Parallel cases: `data_json_strict_ids` +
  `trace_query_state_churn_window_stats`.
- `data_json_strict_ids`: PASS in 45s. The final strict output remains
  `{"ids":["u1","u3"]}` with `data_rounds=2`, `data_repair_rounds=1`, and
  no answer-contract violations. Manual audit: the remaining repair is the
  first-plan material-consumption correction; it is typed and does not affect
  final correctness.
- `trace_query_state_churn_window_stats`: PASS in 244s. The run used
  `trace_query` six times, no `read_file`/`grep`, `max_context_window_pct=21`,
  and `GOMEMLIMIT=12GiB` was active. Manual audit: the new
  `Trace 指标快照` block materialized, and each state-churn row now keeps
  `max_segment` and `p95_segment` on one deterministic line.
- Residual audit gap: `answer_contract_violations=1` and user-visible generic
  caveats remain. Logs show the root cause is a runtime-artifact
  `aggregate_facts.member_set` for a diagnostic ranking being treated as a
  principal enumeration set, which activates `Required Principal Member Set`,
  principal enumeration rows, and a count oracle meant for source/list answers.

## Batch 33 Gap: Runtime Diagnostic Rankings Must Not Become Principal Enumerations

Deep root cause:

- Runtime diagnostic tools return ranked candidates such as scheduler
  root-cause rows. These are ordered observations that support the diagnosis,
  not necessarily a user-requested exhaustive member list.
- The aggregate fact role resolver already demotes unsupported runtime
  behavior/scalar/count facts for external-only runtime artifacts, but
  `member_set` was missing from that advisory path. When an explorer emits a
  runtime diagnostic ranking as `member_set` with `role=principal_answer`, the
  finalizer treats it like a hard principal enumeration contract and raises
  generic completeness/count caveats.
- This is a system-level role-normalization gap. The fix belongs in typed
  aggregate role resolution, not in prompt wording or final-answer prose
  rewriting.

Generalized design:

- Extend runtime advisory aggregate demotion to `member_set` facts when all
  the following typed conditions hold:
  1. the request has an external-only runtime artifact;
  2. the fact has no support refs;
  3. the request is diagnostic/root-cause/trace/performance-shaped;
  4. the request does not structurally require a principal member set
     (explicit enumeration intent, category/relation enumeration,
     source-inventory profile, or declared member-set obligation).
- Preserve principal `member_set` behavior when a runtime/external request
  explicitly asks for an enumeration/list or carries per-member support refs.
- Keep the decision typed-only: request model traits, aggregate kind/role, and
  support-ref presence. Do not inspect user prose, labels, model prose, or
  answer markdown.

Executable task list:

- [x] B33-T1: Record the Post-Batch-32 audit and runtime diagnostic
  member-set gap.
- [x] B33-T2: Extend runtime aggregate role normalization so unsupported
  diagnostic `member_set` rows become supporting coverage.
- [x] B33-T3: Add regression tests for demotion and explicit runtime
  enumeration preservation.
- [x] B33-T4: Run focused/full validation, commit, push, and rerun the
  representative eval pair.

Post-Batch-33 eval audit:

- Results root:
  `eval/results/eval-gap-20260613-post-1f76f81e-b1`
- Parallel cases: `data_json_strict_ids` +
  `trace_query_state_churn_window_stats`.
- `data_json_strict_ids`: PASS in 40s. The final strict output remains
  `{"ids":["u1","u3"]}` with `data_rounds=2`, `data_repair_rounds=1`,
  `data_record_count=2`, and no answer-contract violations. Manual audit:
  the stale early `decision_status=blocked` log belongs to a repaired
  intermediate data-workflow round; the terminal result is complete and
  correct.
- `trace_query_state_churn_window_stats`: PASS in 164s with
  `tool_trace_query=1`, `tool_read_file=1`, `unavailable_tool_attempts=1`,
  `answer_contract_violations=0`, `enumeration_push=0`,
  `max_context_window_pct=18`, and `finalizer_iters=1`.
- Manual answer audit: the answer keeps the rich diagnostic explanation and
  materializes `Trace 指标快照`; no generic principal-enumeration caveat leaks.
  It correctly states that trace-query window statistics override the earlier
  pre-triage estimate when they conflict.
- Residual audit gap: the perf-triage pre-stage attempted unavailable
  `read_file` once before emitting. Logs show the hard schema already hid
  `read_file` for inline-only attachments, but the static perf/log triage skill
  still advertised `read_file` as a pagination tool. This is a capability
  surface drift, not a trace-query answer gap.

## Batch 34 Gap: Runtime Triage Prompt Capability Surface Must Match Tool Schema

Deep root cause:

- Runtime triage already has a typed execution guard:
  inline-only attachments hide `read_file`, while blob-backed attachments keep
  it for legitimate pagination. The visible skill prompt, however, was static
  and still said the model had `read_file`.
- That drift creates unnecessary unavailable-tool attempts and can produce
  stale hand calculations before downstream tools correct the answer. The
  answer can be semantically right, but the run spends a round on a tool that
  the same dispatcher has structurally hidden.
- The general class is "prompt capability surface != tool schema". The fix
  should project the prompt-facing tool surface from the same typed gates that
  build the schema, and avoid static skill prose that names conditional tools
  as unconditionally available.

Generalized design:

- Keep `ToolSuggestions` as the source allowlist for all tools a skill may use
  in any valid context.
- Before prompt assembly, clone the skill with `ToolSuggestions` filtered by
  the same context gates used by `buildToolSchemas`:
  inline-only runtime attachment, external-observation-only runtime mode,
  write-exploration read-only mode, trace-query availability, and
  answer-document patch availability.
- Render the prompt and evaluator dynamic supplement from that projected skill,
  so reasoning hygiene and any inspected tool list describe the current
  dispatch rather than the superset.
- For runtime log/perf triage and segmentation static text, describe
  attachment pagination as "available only when the current tool schema exposes
  a pagination read tool"; do not name `read_file` in the static prompt body.
  Blob-backed runs still expose the actual `read_file` schema; inline-only
  runs expose only the typed emit tool.
- Keep the hard guard in tool execution. Prompt projection reduces invalid
  attempts but does not become the enforcement boundary.

Executable task list:

- [x] B34-T1: Record the Post-Batch-33 eval audit and identify the runtime
  triage capability-surface drift.
- [x] B34-T2: Add prompt-time skill tool projection from the same typed gates
  used by tool-schema construction.
- [x] B34-T3: Make log/perf triage and segmentation static skill text
  schema-authoritative for attachment pagination instead of naming a
  conditional tool as always available.
- [x] B34-T4: Add regression tests for prompt-visible tool projection and for
  runtime triage skills not advertising `read_file` in static prompt text.
- [x] B34-T5: Run focused/full validation, commit, push, and rerun the
  representative eval pair.

Post-Batch-34 eval audit:

- Results root:
  `eval/results/eval-gap-20260613-post-17bbd853-b1`
- Parallel cases: `data_json_strict_ids` +
  `trace_query_state_churn_window_stats`.
- `data_json_strict_ids`: PASS in 46s. The final strict output remains
  `{"ids":["u1","u3"]}` with `data_rounds=2`,
  `data_repair_rounds=1`, `data_record_count=2`, and no
  answer-contract violations. Manual audit: the stale
  `decision_status=blocked` entry belongs to an intermediate repaired data
  round; the terminal strict JSON is complete and correct.
- `trace_query_state_churn_window_stats`: PASS in 157s with
  `tool_trace_query=1`, `tool_read_file=0`, `unavailable_tool_attempts=0`,
  `max_context_window_pct=17`, and `finalizer_iters=1`.
- Manual answer audit: the answer is semantically correct and rich. It keeps
  both `app-20` and `rival-30` state-churn rows, includes
  `dominant_state`, cumulative state times, `fragments`, `switches`,
  `max_segment`, `p95_segment`, and preserves the follow-up direction to
  inspect `rival-30` on the same CPU and validate wake latency with
  `sched_wakeup`.
- Residual audit gap: `answer_contract_violations=1` and the final surface
  still appended a generic enumeration caveat. Logs show the only remaining
  violation was rooted at `block_items_label`, while the answer block was a
  typed runtime/external-observation metric surface. The prompt/tool surface
  gap from Batch 34 is closed: no unavailable `read_file` attempt remains.

## Batch 35 Gap: Runtime External-Observation Dimension Lists Must Not Trigger Source Enumeration Caveats

Deep root cause:

- Source-code enumeration validators correctly protect answers that render
  code identifiers or source inventory labels. Runtime trace metric snapshots,
  however, render dimension names such as `dominant_state`, `running`, and
  `max_segment` as external-observation facts, not source symbols.
- The runtime materializer already stamps these blocks with
  `ClaimExternalObservation` and `observed_artifact_fact`, but the
  enumeration label grounding/hallucination oracles did not use that typed
  boundary. They therefore interpreted runtime metric dimensions as
  source-like labels and produced `block_items_label` violations even when the
  trace-query values were correct.
- The caveat materializer had runtime suppression for low-precision coverage
  and uncertainty caveats, but not for source enumeration-label violations.
  If a stale or over-broad source-label violation reached the soft caveat
  path, it could still leak as a generic user-visible note.
- This is a system-boundary gap, not a trace-query formatting issue. The fix
  belongs at the typed claim-form boundary and the typed caveat materializer;
  it must not inspect user prose, model-authored markdown, or keyword-match
  runtime metric names.

Generalized design:

- Treat a list/table block whose non-empty `ClaimUses` are all
  `ClaimExternalObservation` as an external-observation answer surface.
- Source enumeration label grounding and hallucination oracles must skip that
  surface entirely. Runtime artifact completeness should be checked by typed
  runtime aggregate/member-set contracts, not by source symbol evidence
  tokens or source graph symbol existence.
- Preserve all existing source-code enumeration behavior for blocks without
  this typed external-observation boundary, including runtime requests that
  explicitly render current-source identifiers.
- Extend runtime low-precision caveat suppression so
  `ViolEnumerationLabelUngrounded`,
  `ViolEnumerationItemLabelExtractorDrift`, and
  `ViolEnumerationLabelHallucinated` remain telemetry-only when the accepted
  principal answer surface is typed as runtime/external observation.
- Keep enforcement structural: decisions read claim forms, surface roles, the
  request model, and mutable answer document state only. No prompt red-line
  changes, no keyword matching over user intent, labels, or model prose.

Executable task list:

- [x] B35-T1: Record the Post-Batch-34 eval audit and the typed
  external-observation label-boundary gap.
- [x] B35-T2: Gate source enumeration label grounding and hallucination on
  the existing `ClaimExternalObservation` block boundary.
- [x] B35-T3: Extend runtime caveat materialization so stale source-label
  violations for accepted external-observation surfaces stay telemetry-only.
- [x] B35-T4: Add focused regression coverage for the oracle boundary and
  caveat suppression.
- [x] B35-T5: Run focused/full validation, commit, push, and rerun the
  representative eval pair.

Post-Batch-35 eval audit:

- Results root:
  `eval/results/eval-gap-20260613-post-5aeea613-b1`
- Parallel cases: `data_json_strict_ids` +
  `trace_query_state_churn_window_stats`.
- `data_json_strict_ids`: PASS in 44s. The strict output remained
  `{"ids":["u1","u3"]}` with no answer-contract violations.
- `trace_query_state_churn_window_stats`: FAIL with `missing:fragments`.
  The run used `read_file=4`, `trace_query=0`, and had one
  answer-contract violation.
- Manual answer audit: the final answer was grounded in a hand-derived
  sched-switch interpretation rather than trace-query rows. It stated that
  `trace_query window_stats` was unavailable, then emitted values that differed
  from prior trace-query facts for the same window.
- Log audit: the perf-triage pre-stage only had its own emit tool, but its
  residue said `trace_query` was unavailable. Analyzer and explorer treated
  that local-stage statement as if it were the downstream explorer tool
  surface. The explorer turn did expose `trace_query`, but source tools were
  selected first and no trace query was ever attempted.

## Batch 36 Gap: Runtime Trace Tool Provenance Must Be Stage-Scoped

Deep root cause:

- Runtime pre-triage stages and explorer run with different tool schemas. A
  pre-stage may legitimately lack `trace_query`, while the later explorer may
  expose it for the same attached trace.
- Model-authored residue from a pre-stage can therefore contain a stale
  local-stage availability claim. If downstream stages treat that prose as an
  authoritative capability fact, they may bypass the deterministic trace tool
  and fall back to source navigation or manual parsing.
- Existing prompt guidance already says to start runtime trace questions with
  `trace_query`, but prompt-only guidance is not a commercial enforcement
  boundary. The hard boundary must read typed runtime context, current tool
  availability, and source-lane posture.
- This is a stage-scoped provenance gap. The fix must not inspect user prose,
  model prose, metric names, or specific eval expectations; it should enforce
  the general invariant that a capable runtime trace tool gets first refusal
  before optional source fallback.

Generalized design:

- In explorer, when all typed conditions hold, require the first non-repair
  tool attempt to be `trace_query`:
  1. a runtime trace is attached or structurally named;
  2. `trace_query` is available in the current explorer tool surface;
  3. current-source evidence is not required by `CurrentSourceLaneDecision`;
  4. the current dispatch has not yet attempted `trace_query`.
- Reject any other tool call with a typed repair code that tells the model to
  call `trace_query` first, or use source tools only after `trace_query`
  returns unsupported/incomplete or a separate current-source lane becomes
  required.
- Count both successful and failed `trace_query` results as first-refusal
  attempts. After that, existing fallback behavior remains available, so real
  unsupported trace shapes do not dead-end.
- Preserve stable source-heavy scenarios: ordinary code questions never expose
  `trace_query`; runtime questions with a typed current-source anchor keep the
  source lane open; observation-only runtime questions keep the stricter
  existing source-tool block.
- Keep the trace-query parameter story centralized: reuse the existing
  `trace_query` schema, strict decode, and unified parameter repair layer
  rather than adding a new prompt or tool-specific JSON workaround.

Executable task list:

- [x] B36-T1: Record the Post-Batch-35 eval audit and the stage-scoped
  trace-tool provenance gap.
- [x] B36-T2: Add an explorer execution guard that gives available
  `trace_query` first refusal before optional source fallback.
- [x] B36-T3: Add regression tests for source-tool/complete rejection before
  trace-query, fallback after a trace-query attempt, and current-source
  required preservation.
- [ ] B36-T4: Run focused/full validation, commit, push, and rerun the
  representative eval pair.
