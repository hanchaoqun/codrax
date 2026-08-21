# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T18:56:58Z
- sweep_start_ts: 20260821-115656
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-115658 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 177s | 33 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 主窗、四节点唤醒链、11.000ms 链上 IO 第一席、三个互相独立的 1.000ms 调度/优先级候选、实际占时与规则可消双账户、CPU1/逐跳 CPU、背景隔离、自动补采和完整 Trace 因果投影均在；无固定 4ms/4m 降级。模型一处先称三个候选“合计上限 3.000ms”又称不可相加，typed 投影仍正确披露重叠，记软措辞波动。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260821-115658 | answer_regex,answer_contains,mermaid_edge_count | none | 897s | 66 | read=9,repo_map=4,list=0,trace=0,source_lens=0 | midloop=18,inv=6/0,fin_reject=20,unavail=1,prune=0 | fail | B1304 生产正证：已渲染同 tuple 后不再发重复 addition，且未再出现 duplicates existing anchor。新 P0 为 typed 合同自冲突：participant candidate 明确要求可见 `BusContext` 承载 from 端、canonical anchor 保留 `o.busCtx -> ctxbuilder.BuildAgentContext`，身份门随后把这一精确形拒为 edge_anchor_node_identity_conflict。其后 stale/unknown ref 失败不回传当前 live refs，模型连续猜陈腐 ref；20 次拒绝后恢复未验旧稿，答案不可交付。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
