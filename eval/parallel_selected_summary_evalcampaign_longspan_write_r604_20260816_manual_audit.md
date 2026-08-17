# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T03:07:45Z
- sweep_start_ts: 20260816-200744
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_fmt_tm_year_overflow | PASS | eval/results/github_issue_fmt_tm_year_overflow-20260816-200745 | write_apply,write_patch_oracle | none | 198s | 24 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass-with-verified-replan | 第一版计划只把接收变量改为 `long long`，错误声称 RHS 会先拓宽；`make check` 精确捕获 `got -2147481749 want 2147485547`。typed verify→replan 改为 `static_cast<long long>(tm.tm_year) + 1900`，第二次 `make check` 通过，且只改被授权头文件。这是模型首轮 C++ 语义失误被通用验证闭环正确拦住，不新增语言/type 特判。 |
| 1 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260816-200745 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 241s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass-with-wording-watch | 显式窗 Trace 因果投影完整：链上 `worker-200` priority-inversion candidate 有效归因 8.300ms/累计 9.000ms，目标 sleep 10.000ms；3.500ms 调度压力保持背景，实际占时与规则可消双轴均在。模型仍把 pre-wakeup dependency 叙述成“等待 worker 完成”、并提出 PI-mutex，超过 typed 锁/完成证据；但正文末尾也披露 holder/waiter 未证，现有 prompt 已明确两项不证明，故按模型波动观察，不加 prose 硬门或系统改写。Analyzer 首轮 `causal_diagnosis+fact_families` 被精确 schema 重试一次；更重要的新系统 gap 是无锚点 source-exclude 被旧 normalize 反向改成 `allow`，触发 4187 文件 repomap 构建，记 B957。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
