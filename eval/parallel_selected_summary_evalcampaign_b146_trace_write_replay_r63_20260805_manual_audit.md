# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T00:20:52Z
- sweep_start_ts: 20260805-172051
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260805-172052 | write_apply,write_patch_oracle,answer_contains | none | 72s | 19 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 一次 source_inventory、两次精确 read 后生成单文件单行 structured patch；零 JSON/合同拒绝。apply 后 `retrun→return`，其余文件不变；post-apply Go 验证 1/1 PASS，final verdict=verified。 |
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260805-172052 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 132s | 41 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、四级唤醒链、11ms IO 主席、1ms 调度席、两轴与系统因果投影均在；但 B145 只在 rank-bearing result 内合并为 E5(+1)，独立 wakeup-only result 无 key，系统占用表仍双发 11ms。Analyzer 条件 schema 后仍一次误带 non-bounded fact_families；模型又把附件末端 2.020020 写成窗内 20.02ms，与 typed 20.000ms 冲突。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
