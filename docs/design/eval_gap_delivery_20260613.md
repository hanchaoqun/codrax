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
- [ ] B11-T7: Rerun representative eval cases two at a time and manually audit
  final answers plus logs.

Batch 11 verification before commit:

- `go test ./internal/types -run 'TestCurrentSourceLaneDecision_RuntimeExactTargetsRemainSourceOptional|TestCurrentSourceLaneDecision_CurrentSourceProfileRequiresSource|TestCompileAnswerIntentContract_ExternalRuntimeArtifactCurrentStatus|TestBuildAnswerSurfacePlan_ExternalTraceExactTargetsDoNotForceCurrentStatus'`
- `go test ./internal/agent -run 'TestBuildAnalysisIR_ExternalOnlyCurrentVersionCheckKeepsCurrentStatus|TestAnswerDocumentEvaluator_BuildInitialInstruction_SourceOptionalTraceSkipsCurrentStatus|TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersHarmonyTracePriorityReminder'`
- `go test ./internal/tool -run 'TestPreCheckRuntimeObservationRepoContaminationAllowsCurrentStatus|TestPreCheckRuntimeObservationRepoContamination'`
- `go test ./internal/agent ./internal/types ./internal/tool`
- `go test ./...`
- `make`
