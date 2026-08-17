# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T16:50:23Z
- sweep_start_ts: 20260817-095022
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-095023 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 214s | 34 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=2,inv=1/0,fin_reject=2,unavail=0,prune=0 | partial | 显式窗、链上 8.300ms、因果投影和候选/已证边界正确；但正文仍泄漏 priority_inversion_candidate、bounded_window_candidate、lower_priority_dependency、typed 等内部枚举/管线词，且表头退化为“列2…列6”。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-095023 | answer_regex,answer_contains,mermaid_edge_count | none | 438s | 39 | read=26,repo_map=5,list=1,trace=0,source_lens=0 | midloop=11,inv=5/0,fin_reject=3,unavail=0,prune=0 | fail | B990 已把补证送到 applyStageOutput/appendStageOutputEvidenceToMutable；typed capsule 也有正确 call+argument_flow。系统 normalizer 却把唯一单边 call receipt 重复贴给两个断开组件，validator 自拒后模型删除真实交接，终图仍让 BusContext/Mutable 断开。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计

### QF：B990 生产闭环，B991 typed receipt 被跨组件重复消费

- completion 本轮直接读取 `internal/orchestrator/orchestrator.go:8442-8479` 与
  `internal/orchestrator/stage_output_evidence_ingest.go`，模型正确识别
  `Orchestrator.applyStageOutput -> appendStageOutputEvidenceToMutable` 以及完整实参
  `o.busCtx.Mutable`。不再读取 cgec/contract helper，证明 B990 的 owner-hop 排序生产生效。
- Explorer 最终提交并通过了两条 line 8479 operation row：exact call
  `Orchestrator.applyStageOutput -> appendStageOutputEvidenceToMutable` 与 argument flow
  `o.busCtx.Mutable -> appendStageOutputEvidenceToMutable`。Finalizer typed capsule 的
  `verified_component[4]`、`edge_recipe[6]`、`edge_recipe[7]` 和 copy-ready fragment 4 全都正确；
  缺边不再是采集或上下文不足。
- 模型第二次 patch 删除未证 stage→apply 边后留下两个断开的单边 call 组件：
  `runScheduler -> executeStage` 与 `applyOut -> appendFn`。旧 topology normalizer 逐组件独立找
  唯一同构 recipe，把唯一单边 call receipt
  `runReadSchedulerLoop -> executeStageRequest` 同时贴到两条边；因此 validator 报出荒谬的
  `applyOut(Orchestrator.runReadSchedulerLoop) -> appendFn(Orchestrator.executeStageRequest)` 并以
  occurrence unproven 拒绝。模型识别出系统映射异常后，为求通过删除了真实 call/argument-flow，
  最终图再次把 BusContext/Mutable 留成未证孤点。
- B991（`e4a86bb82`）把 alias-topology receipt 消费提升为 AnswerDocument 全局一次性：先统计每个
  typed component 被多少断开模型组件竞争，竞争大于一时一律不补 identity；已有完整 identity 的
  组件用 identity+relation 多重集计入占用，避免另一匿名组件再次借用；宽对称图仍走无阶乘快路。
  正负回归覆盖“双匿名不修”和“稳定 recipe 节点只修一次”，相关矩阵及完整
  `go test ./internal/tool -count=1` 通过（180.227s）。修复不改图正文、不建边，只撤销系统无权的
  metadata 猜配。

### Trace：核心因果语义稳定，用户词汇与表格质量仍未闭环

- `selected_window=1.000000..1.010000`、目标 sleep 10.000ms、worker-200 链上 runnable
  8.300ms、CPU2→CPU1 唤醒、邻近/背景不入主因和完整 Trace 因果投影均稳定。
- 模型本轮比 r629 克制：明确 lower-priority dependency 只构成调度供给候选，
  `synchronous_blocking/holder_waiter/post_wakeup_delay` 未证，不能宣称已确认锁反转。这与用户要求的
  “链上根因保留，同时不把邻近/背景晋升”一致。
- 用户可见正文仍直接出现 `priority_inversion_candidate`、`bounded_window_candidate`、
  `lower_priority_dependency`、`typed`，根因表表头还变为“列 2…列 6”。这是 JSON schema 必须使用
  枚举与最终用户文案不应照抄枚举之间的教学负担，不应通过字符串扫描硬删或 renderer 改写模型
  结论。当前记为重复生产 witness，后续从有界 customer-language alias/glossary 和表格语义字段
  教学统一处理，并用异构 Trace 回放判断模型波动比例。
- 两案活跃流均跨过 30 秒并持续收到语义字节；没有固定 4ms 降级、空答案或旧稿恢复。

## 判定

- `B990-TYPEDOWNERBRIDGECANDIDATERANK1=production-closed-r630`。
- `B991-DIAGRAMTYPEDRECEIPTSINGLECONSUMER1=implemented/global-demand+full-tool-suite-pass/pending-r631`。
- `Trace customer-language-enum/table=confirmed/P2/repeated-witness/no-prose-hard-gate`。
- `Trace explicit-window/causal projection/auto-supplement=production-positive`。
- `active-stream-4ms-degrade=forbidden/not-observed`。
- `system-answer/conclusion/relation/diagram-authorship=none`。
