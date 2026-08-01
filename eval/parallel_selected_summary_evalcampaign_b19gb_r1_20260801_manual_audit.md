# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T09:40:36Z
- sweep_start_ts: 20260801-024035
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260801-024036 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 108s | 35 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B19g-b 的 typed target 生效：`named_target` + user_explicit PID，主值恢复为 3 次/0.635ms，不再从 entities 或 cursor 猜目标。但 analyzer 本轮将“是否/何时/原因/总量”有限事实集标为 `intent=trace, kind=mechanism, diagnostic=false`；现有窄化只覆盖 trace+conditional 和 explain+mechanism，系统仍补跑 root_cause/critical_blocking 并追加整窗因果报告。记 `EVAL-B19-FACTSET1/P0`。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-024036 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 155s | 40 | read=2,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式 114.940ms 窗的双轴占用、可消除量、根因排序、唤醒链、代表窗和 Trace 因果投影全部在场，证明 B19g-b 未伤及显式窗/自动补齐。但 model principal 仍将优先级反转候选写成结构性瓶颈、次根因/三级原因，typed 边界却为 `frame_causality=unproven, frame_evidence_status=absent`。`EVAL-B19-CAUSAL1/P1` 继续开放；不用模型原文关键词扫描做硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
