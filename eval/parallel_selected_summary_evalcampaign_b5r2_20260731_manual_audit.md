# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T13:37:24Z
- sweep_start_ts: 20260731-063724
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_e2_cross_trace_asymmetry | PASS | eval/results/real_trace_e2_cross_trace_asymmetry-20260731-063724 | log_regex,answer_regex,answer_contains | none | 112s | 33 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | T1/T2/T4 已生效，工件范围为 144.557ms/0.556ms 且无 Hzns；但一个 VerifyClass 语义优化行仍被算作 causal row，系统追加约 110 行 Trace 因果投影。event_search typed 账为 matched_total=90/emitted=40，正文却称“CPU 频率事件共40条”；per-artifact alignment=identity 又被解释为“两文件理论同一时钟域”，跨工件关系并未被证明。 |
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260731-063724 | typed_inventory_rowset,dimension_substring,answer_contains | none | 194s | 26 | read=11,repo_map=4,list=2,trace=0,source_lens=3 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 人工读取后成员集合和计数正确（2 extend、2 foreign func、8 public class、11 package），但每类主表没有把 location/package 放在同一行，typed row oracle 正确失败。根因是 root auxiliary projection 在 256 文件预算前按路径取样，少数语言构造未进入 graph；后续 principal rows 只有 location，package 留在说明文本而非 attributes。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
