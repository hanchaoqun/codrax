# Selected Eval Manual Audit Scaffold

- date: 2026-07-05T01:49:31Z
- sweep_start_ts: 20260705-094930
- total cases: 3
- parallel: 3
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 3 | data_text_filter_count | PASS | eval/results/data_text_filter_count-20260705-094931 | log_regex,answer_regex | none | 37s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 2 | data_json_strict_ids | FAIL | eval/results/data_json_strict_ids-20260705-094931 | log_regex,answer_regex | none | 176s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 1 | data_basic_sum_with_rules | PASS | eval/results/data_basic_sum_with_rules-20260705-094931 | log_regex,answer_regex | none | 383s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
