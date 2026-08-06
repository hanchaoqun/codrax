# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T00:42:01Z
- sweep_start_ts: 20260805-174200
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260805-174202 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 166s | 35 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Analyzer 首轮成功；non-bounded fact_families 由 typed compat 无损忽略且 warning 可审计。正文与系统投影均使用查询窗 20.000ms，四级唤醒链、11ms IO 主因、1ms 调度次因、实际占用/现规则可消两轴、因果投影与下钻方向齐全；主要占用表的同一 IO 账户已合为一席。附件 extent 20.020ms 仅在时间单位 caveat 出现。模型仍有“没有主动睡眠设计”的轻微超证措辞，记 P2 波动，不做 prose 硬门。 |
| 2 | data_jsonl_filter_count | FAIL | eval/results/data_jsonl_filter_count-20260805-174202 | log_regex,answer_regex | none | 222s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 过滤结果、2 条 contributions 与 filtered_count actual=2 均已精确形成，但首次 repair 把 generated reconcile 的 metric 列误作完整引用键，零交集仍补零并发布 0；下一 repair 又把旧 final_answer/projection receipt 当业务组拼成 2,0。终态图仅检查 receipt 自洽，最终错误发布 0。确认 EVAL-B148-DATASCALARREF1=P0。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
