# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T12:28:33Z
- sweep_start_ts: 20260829-052831
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260829-052833 | log_attachment,answer_contains | log_triage | 133s | 26 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | B1456 生产正证：source optional，repo reads/repomap 均为 0，终稿只保留附件内 ArkTS/Cangjie 精确首帧与调用者，不再出现无关样例 Bridge.cj:9。跨栈直接因果明确未证。analyzer 本轮仍选 call_chain，而 r942 已正确选 enumeration；没有结构拒绝或证据扩域，归为模型类型遵循波动，不加硬门。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260829-052833 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 214s | 48 | read=0,repo_map=0,list=0,trace=10,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=1 | pass | 显式窗、36 次唤醒、四跳链、链上优先级/调度/算力/D/IO、VerifyClass、实际占时/规则可消双轴、完整因果投影与自动补采全部保留；邻近/背景不入主因，帧/截止期因果未证边界明确。analyzer 有一次 causal scope 携 fact_families 的自修，trace_query 10 次且 context 48%，列 P2 成本观察，不按轮数/耗时硬截断。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
