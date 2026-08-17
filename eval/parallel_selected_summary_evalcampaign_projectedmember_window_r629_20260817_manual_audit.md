# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T16:27:54Z
- sweep_start_ts: 20260817-092752
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-092754 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 236s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B989 生产闭环：主值严格保持 1.000000..1.010000 / 10.000ms，1.010020 只留作闭合边界证据；因果投影、worker-200 链上 8.300ms 和跨核唤醒均在。终稿仍把 CPU2 的 1.010020 switch 写成“让出 CPU1”，并泄漏 priority_inversion_candidate/lock_priority/on_chain，记残余而不影响 B989 收账。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-092754 | answer_regex,answer_contains,mermaid_edge_count | none | 386s | 43 | read=18,repo_map=1,list=1,trace=0,source_lens=0 | midloop=16,inv=3/0,fin_reject=2,unavail=0,prune=1 | fail | B988 已使 late identity 升级看到 BusContext.Mutable，但 completion 候选仍先读无关 local helper；最终图只有四阶段 precedence，Mutable/BusContext 断开，且一条系统给出的 recipe 随后被 call_edge_unproven 拒绝。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计

### Trace：B989 主值窗闭环，答案展示仍有次级残余

- Finalizer typed ledger 的 `selected_window=1.000000..1.010000`、`window_ms=10.000`、目标
  `sleep=10.000ms` 与根因板窗口完全一致；附件尾部 `1.010020` 被明确标作 extent/unit provenance，
  没有再夺取 principal value。worker-200 仍是 wakeup chain 上的首位 8.300ms 候选，CPU2→CPU1
  的 wakeup target 事实、链上根因和背景分层均保留。
- 终稿把附件闭合行 `[002] 1.010020 sched_switch` 误写成“worker-200 让出 CPU 1”，并把
  `priority_inversion_candidate`、`lock_priority`、`on_chain` 原样放进用户正文。前者是一次模型
  CPU 叙述错误，后者与此前 `bounded_window_candidate` 同属客户展示词汇债；都不能靠扫描答案
  字符串硬删。先留作跨案例 typed-context/软教学审计项，不回退已经闭环的显式窗主值隔离。
- 30 秒提示均显示“已收到模型语义输出，持续生成中”；没有活跃字节流 4ms 降级或旧稿恢复。

### QF：不是模型波动，而是 typed owner bridge 已命中后排序仍退化

- 首次 completion 强制读取 `internal/orchestrator/cgec_enforcers.go:767-791`，第二次读取
  `internal/orchestrator/contract_check_block.go:3501-3525`。前者只是本地
  `forcedReadCancelled(busCtx *types.BusContext)`，后者也是 validator 消费者，均不是用户要的
  Extractor→BusContext.Mutable→Finalizer 数据交接。
- 后期 repair hint 已包含 `Mutable, BusContext.Mutable, *MutableState, BusContext, mu, fork,
  runSingleShot, ChangePlan`，证明 B988 的 projected FileIndex 路径生产生效。问题发生在候选排序：
  `this.Mutable` 的同 owner helper 与解析器已证明的 `o.busCtx.Mutable` 跨 owner 交接同分，旧逻辑
  最终用文件名字典序选错。
- `54c81daea` 新增仅用于导航的 `carrierOwnerBridgeRank`：只有“内层成员声明 owner → 外层字段的
  exact static declared type → 外层 owner method → 完整实参仍命中原 binding”整条 parser 事实链
  存在时才提高排序。它不生成 evidence、图边、关系类型或答案。生产形覆盖全部支持语言；关系导航
  定向矩阵和完整 `go test ./internal/tool -count=1` 通过（178.405s）。
- 终稿另有一条 `Orchestrator.Run -> o.busCtx.Mutable.ChangePlan` recipe 被
  `call_edge_unproven` 拒绝的潜在 recipe/validator 自冲突。先等正确读取坐标的下一轮生产回放，若
  仍复现再独立立案，避免把两个阶段的故障混成针对本案例的特判。

## 判定

- `B988-PROJECTEDFILEINDEXMEMBERNAV1=production-positive-r629`。
- `B989-TRACEPRINCIPALWINDOWISOLATION1=production-closed-r629`。
- `B990-TYPEDOWNERBRIDGECANDIDATERANK1=implemented/all-language+full-tool-suite-pass/pending-r630`。
- `Trace customer-vocabulary/CPU-boundary narration=observed/P2/needs-heterogeneous-replay`。
- `active-stream-4ms-degrade=forbidden/not-observed`。
- `system-answer/conclusion/relation/diagram-authorship=none`。
