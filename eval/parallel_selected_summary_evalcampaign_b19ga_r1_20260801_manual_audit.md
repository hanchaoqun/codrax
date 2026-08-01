# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T08:43:28Z
- sweep_start_ts: 20260801-014327
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_c2_dstate_iowait | FAIL | eval/results/real_trace_c2_dstate_iowait-20260801-014328 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 161s | 39 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B19g-a 生效：analyzer 收敛为 explain+conditional、family=generic，主体正确给出三段 0.138/0.147/0.350ms、Σ0.635ms、caller 和 D/io 分型。仍误补 root_cause_rank 并发布双轴/完整投影，因为 focused predicate 漏掉 explain+conditional。runner 唯一失败是正确总量先于清单，旧 regex 错把顺序当合同；已拆成 roster-unit 与 count/Σ 两个顺序无关的精确断言。 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-014328 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 164s | 38 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗 projection、双轴、根因排序、唤醒链、可消量全部保留，且窗口正确。模型本轮未再写“16ms 超7倍/共享锁”，但仍把 typed unproven 帧权限叙述为“帧内丢帧风险由…主导”，FRAME1 结论槽尚未融合；Cookie sleep 两账并列关系仍未披露，ARITH2 开放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
