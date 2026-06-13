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
