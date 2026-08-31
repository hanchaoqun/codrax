# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T05:08:42Z
- sweep_start_ts: 20260830-220840
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-220842 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 147s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 10ms 窗、4 次 typed trace_query、最终因果投影、链上 NetworkService 第一席、实际占时/规则可消双账户、确定性优化线索与背景隔离均完整；无固定时限降级。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260830-220842 | answer_regex,answer_contains | none | 470s | 45 | read=16,repo_map=2,list=0,trace=0,source_lens=0 | midloop=16,inv=2/0,fin_reject=6,unavail=0,prune=1 | pass | 最终图仅保留四阶段三条 typed precedence，表格与调度链完整且无降级。B1469 空动作集未复发，但本轮未自然形成 label/body 重叠；6 次拒绝为整块操作、错误候选端点、漏孤儿处置和陈腐 boundary，未确认新合同矛盾。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual Findings

1. Trace 人工判定通过。显式 `34579.490..34579.500s` 窗、4 次 typed `trace_query` 和最终 `Trace 因果投影` 均在；
   `NetworkService-60595 -> CookieMonsterCl-59843 -> com.baidu.tieba-59566` 已证链、5.951ms 链上第一席、目标五态账、实际占时/规则可消
   双账户、类校验语义线索和邻近/背景隔离完整。没有把背景 IO/D 升为主因，也没有按 4ms、4m、轮次、上下文比例或活动流年龄降级。
2. read 最终答案人工判定通过。Mermaid 只保留 `analyze -> Expl -> Ext -> Fin` 三条 checkout-verified precedence；四阶段输入、输出、Agent、
   状态载体表齐全，Orchestrator 调度链有源码引用；无恢复旧稿、无系统代写关系或结论。B1468 的多声明歧义错误未出现。
3. r954 没有自然形成 r953 的 label-pair + visible-body 同 occurrence 形，因此 B1469 不能记 production-positive；确定性 lease/e2e 回归与本轮
   production no-regression 共同成立。read 的 6 次拒绝分别来自模型尝试 whole add/replace、把 `Expl` 用作 analyzer 端点、漏掉两个精确孤儿处置、
   以及在已连通 participant 上保留 unproven boundary；每次错误信息与下一轮修补均可执行，未发现新的共同 action 为空或互相矛盾的合同。
4. 相比 r953，read 从 876s/7 rejects/降级收敛到 470s/6 rejects/有效结构化答案，证明新 capability 没有引入回归；但时长差受模型路径影响，
   不能单凭一次样本宣称性能提升比例。下一批应切换到异构 write/read 或其它语言/图家族，继续以一个显式窗 Trace 作为护栏，而非反复拟合同一 read case。
