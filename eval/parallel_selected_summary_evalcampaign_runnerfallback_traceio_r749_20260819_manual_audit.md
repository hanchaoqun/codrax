# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T00:18:27Z
- sweep_start_ts: 20260819-171826
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260819-171827 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 237s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式窗 Trace 因果投影完整保留；threadpool-400 链上 IO 等待 11ms 为首因，三个链上 runnable 各 1ms，邻近睡眠和背景 IO 压力未晋升。模型把四节点主链写成“四个 hop”（应为三条主链边；若把 irq→threadpool 算入则必须写出五节点），属于 typed 路径正确、成文边数不一致的关系表达债。 |
| 1 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-171827 | log_regex,write_apply,answer_regex,answer_contains | none | 1099s | 26 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 模型首补丁把五个换行错误折成 `[300,300,10]`；精确 probe、unittest 与 make 均正确失败，controller 进入 typed replan。apply 入口默认 900s 在重规划模型持续输出时触发 caller deadline，留下 in_progress/missing final；这是 B1202 预算错配，不是活跃字节流 4ms 降级。B1200 未获正向回放，因为 exact fallback 本轮也真实失败，旧 runner_missing 正确未被 supersede。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
